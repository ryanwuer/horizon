// Copyright © 2023 Horizoncd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package kservice

import (
	"context"
	"strconv"

	"github.com/argoproj/argo-cd/pkg/apis/application/v1alpha1"
	herrors "github.com/horizoncd/horizon/core/errors"
	perror "github.com/horizoncd/horizon/pkg/errors"
	"github.com/horizoncd/horizon/pkg/util/kube"
	"github.com/horizoncd/horizon/pkg/util/log"
	"github.com/horizoncd/horizon/pkg/workload"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/kubectl/pkg/polymorphichelpers"
	servicev1 "knative.dev/serving/pkg/apis/serving/v1"
)

const (
	StepTotalAnnotationKey = "cloudnative.music.netease.com/step-total"
)

var (
	GVRService = schema.GroupVersionResource{
		Group:    "serving.knative.dev",
		Version:  "v1",
		Resource: "services",
	}
	GVRPod = schema.GroupVersionResource{
		Group:    "",
		Version:  "v1",
		Resource: "pods",
	}
)

func init() {
	workload.Register(ability, GVRService, GVRPod)
}

// please refer to github.com/horizoncd/horizon/pkg/cluster/cd/workload/workload.go
var ability = &service{}

type service struct{}

func (*service) MatchGK(gk schema.GroupKind) bool {
	return gk.Group == "serving.knative.dev" && gk.Kind == "Service"
}

func (*service) getService(node *v1alpha1.ResourceNode,
	factory dynamicinformer.DynamicSharedInformerFactory) (*servicev1.Service, error) {
	obj, err := factory.ForResource(GVRService).Lister().ByNamespace(node.Namespace).
		Get(node.Name)
	if err != nil {
		return nil, perror.Wrapf(
			herrors.NewErrGetFailed(herrors.ResourceInK8S,
				"failed to get deployment in k8s"),
			"failed to get deployment in k8s: deployment = %s, err = %v", node.Name, err)
	}

	un, ok := obj.(*unstructured.Unstructured)
	if !ok {
		return nil, perror.Wrapf(herrors.ErrParamInvalid, "failed to convert obj to unstructured")
	}

	var ksvc *servicev1.Service
	err = runtime.DefaultUnstructuredConverter.FromUnstructured(un.UnstructuredContent(), &ksvc)
	if err != nil {
		return nil, err
	}
	return ksvc, nil
}

func (*service) getServiceByNode(node *v1alpha1.ResourceNode, client *kube.Client) (*servicev1.Service, error) {
	return GetServiceByNode(node, client)
}

func (s *service) ListPods(node *v1alpha1.ResourceNode,
	factory dynamicinformer.DynamicSharedInformerFactory) ([]corev1.Pod, error) {
	instance, err := s.getService(node, factory)
	if err != nil {
		return nil, err
	}

	selector := labels.SelectorFromSet(instance.Spec.Template.ObjectMeta.Labels)
	objs, err := factory.ForResource(GVRPod).Lister().ByNamespace(node.Namespace).List(selector)
	if err != nil {
		return nil, err
	}

	pods := workload.ObjIntoPod(objs...)

	return pods, nil
}

func (s *service) IsHealthy(node *v1alpha1.ResourceNode,
	client *kube.Client) (bool, error) {
	instance, err := s.getServiceByNode(node, client)
	if err != nil {
		return true, err
	}

	if instance == nil {
		return true, nil
	}

	labels := polymorphichelpers.MakeLabels(instance.Spec.Template.ObjectMeta.Labels)
	pods, err := client.Basic.CoreV1().Pods(instance.Namespace).
		List(context.TODO(), metav1.ListOptions{LabelSelector: labels, ResourceVersion: "0"})
	if err != nil {
		return true, err
	}
	log.Debugf(context.TODO(), "[knative service: %v] pods count = %v", instance.Name, len(pods.Items))

	annos := instance.Spec.Template.ObjectMeta.Annotations
	min, _ := strconv.Atoi(annos["autoscaling.knative.dev/minScale"])
	max, _ := strconv.Atoi(annos["autoscaling.knative.dev/maxScale"])
	log.Debugf(context.TODO(), "[knative service: %v] minScale = %v, maxScale = %v", instance.Name, min, max)

	count := 0

OUTTER:
	for _, pod := range pods.Items {
		m := make(map[string]string)
		for _, container := range pod.Spec.Containers {
			m[container.Name] = container.Image
		}

		for _, container := range instance.Spec.Template.Spec.Containers {
			if image := m[container.Name]; image != container.Image {
				log.Debugf(context.TODO(),
					"[knative service: %v] expect container(%v)'s image = %v, pod(%v) container(%v)'s image = %v",
					instance.Name, container.Name, image, pod.Name, container.Name, container.Image)
				continue OUTTER
			}
		}

		for k, v := range instance.Spec.Template.ObjectMeta.Annotations {
			if pod.Annotations[k] != v {
				log.Debugf(context.TODO(),
					"[knative service: %v] expect annotation(%v) = %v, pod(%v) annotation(%v) = %v",
					instance.Name, k, v,
					pod.Name, k, pod.Annotations[k])
				continue OUTTER
			}
		}
		count++
		if count > max {
			break
		}
	}
	log.Debugf(context.TODO(),
		"[knative service: %v] count = %v, min = %v, max = %v",
		instance.Name, count, min, max)
	return count >= min && count <= max, nil
}

func (s *service) GetSteps(node *v1alpha1.ResourceNode, client *kube.Client) (*workload.Step, error) {
	instance, err := s.getServiceByNode(node, client)
	if err != nil {
		return nil, err
	}

	// 1. get step total
	stepTotal, ok := instance.Spec.Template.Annotations[StepTotalAnnotationKey]
	if !ok {
		return nil, nil
	}
	stepTotalInt, err := strconv.Atoi(stepTotal)
	if err != nil {
		return nil, perror.Wrapf(herrors.ErrParamInvalid, "step total is not a number")
	}

	// 2. get current traffic percent
	if instance.Status.Traffic == nil {
		return nil, nil
	}
	var latestRevisionPercent int
	for _, traffic := range instance.Status.Traffic {
		if traffic.LatestRevision != nil && *traffic.LatestRevision {
			latestRevisionPercent = int(*traffic.Percent)
		}
	}

	// 3. build traffic percent array
	trafficPercentArray, err := buildTrafficPercentArray(stepTotalInt)
	if err != nil {
		return nil, err
	}

	// 4. get step index
	stepIndex := 0
	sumPercent := 0
	for i, trafficPercent := range trafficPercentArray {
		sumPercent += trafficPercent
		if sumPercent == latestRevisionPercent {
			stepIndex = i
			break
		}
	}

	return &workload.Step{
		Index:    stepIndex,
		Total:    stepTotalInt,
		Replicas: trafficPercentArray,
		StepType: workload.StepTypePercent,
	}, nil
}

func (*service) Action(actionName string, un *unstructured.Unstructured) (*unstructured.Unstructured, error) {
	var instance *servicev1.Service
	err := runtime.DefaultUnstructuredConverter.FromUnstructured(un.UnstructuredContent(), &instance)
	if err != nil {
		return un, perror.Wrapf(herrors.ErrParamInvalid, "convert to service failed: %v", err)
	}
	spec, ok := un.Object["spec"].(map[string]interface{})
	if !ok {
		return nil, perror.Wrapf(herrors.ErrParamInvalid, "spec not found")
	}

	switch actionName {
	case "promote":
		if instance.Status.Traffic == nil {
			return nil, perror.Wrapf(herrors.ErrParamInvalid, "traffic is nil")
		}
		if len(instance.Status.Traffic)!= 2 {
			return nil, perror.Wrapf(herrors.ErrParamInvalid, "traffic length is not 2")
		}

		stepTotal, ok := instance.Spec.Template.Annotations[StepTotalAnnotationKey]
		if !ok {
			return nil, nil
		}
		stepTotalInt, err := strconv.Atoi(stepTotal)
		if err != nil {
			return nil, perror.Wrapf(herrors.ErrParamInvalid, "step total is not a number")
		}

		var latestRevisionPercent int
		for _, traffic := range instance.Status.Traffic {
			if traffic.LatestRevision != nil && *traffic.LatestRevision {
				latestRevisionPercent = int(*traffic.Percent)
			}
		}

		trafficPercentArray, err := buildTrafficPercentArray(stepTotalInt)
		if err != nil {
			return nil, err
		}

		sumPercent := int64(0)
		for _, trafficPercent := range trafficPercentArray {
			sumPercent += int64(trafficPercent)
			if sumPercent > int64(latestRevisionPercent) {
				break
			}
		}

		isLatestRevision := true
		notLatestRevision := false
		oldRevisionPercent := 100 - sumPercent
		updatedTraffic := []servicev1.TrafficTarget{
			{
				LatestRevision: &isLatestRevision, 
				Percent:        &sumPercent,  
			},
		}
		if oldRevisionPercent != 0 {
			updatedTraffic = append(updatedTraffic, servicev1.TrafficTarget{
				LatestRevision: &notLatestRevision, 
				Percent:        &oldRevisionPercent,  
				RevisionName:   instance.Status.Traffic[1].RevisionName,
			})
		}
		spec["traffic"] = updatedTraffic
	default:
		return nil, perror.Wrapf(herrors.ErrParamInvalid, "unsupported action: %v", actionName)
	}
	return un, nil
}

// buildTrafficPercentArray build traffic percent array
// 1. first step is always 5%
// 2. the rest steps split 95% evenly
func buildTrafficPercentArray(stepTotal int) ([]int, error) {
	if stepTotal <= 0 {
		return nil, perror.Wrapf(herrors.ErrParamInvalid, "step total is not a positive number")
	}

	if stepTotal == 1 {
		return []int{100}, nil
	}

	trafficPercentArray := make([]int, stepTotal)
	trafficPercentArray[0] = 5
	restPercent := 95
	for i := 1; i < stepTotal; i++ {
		trafficPercentArray[i] = restPercent / (stepTotal - 1)
	}
	// fix the last step to make the sum of the array equal to 95%
	trafficPercentArray[stepTotal-1] = restPercent - (stepTotal-2)*trafficPercentArray[stepTotal-2]

	return trafficPercentArray, nil
}

func GetServiceByNode(node *v1alpha1.ResourceNode, client *kube.Client) (*servicev1.Service, error) {
	gvr := schema.GroupVersionResource{
		Group:    "serving.knative.dev",
		Version:  node.Version,
		Resource: "services",
	}

	un, err := client.Dynamic.Resource(gvr).Namespace(node.Namespace).
		Get(context.TODO(), node.Name, metav1.GetOptions{})
	if err != nil {
		return nil, perror.Wrapf(
			herrors.NewErrGetFailed(herrors.ResourceInK8S,
				"failed to get deployment in k8s"),
			"failed to get deployment in k8s: deployment = %s, err = %v", node.Name, err)
	}

	var ksvc *servicev1.Service
	err = runtime.DefaultUnstructuredConverter.FromUnstructured(un.UnstructuredContent(), &ksvc)
	if err != nil {
		return nil, err
	}
	return ksvc, nil
}