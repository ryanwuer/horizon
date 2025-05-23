package admission

import (
	"context"
	"encoding/json"

	"k8s.io/apimachinery/pkg/util/runtime"

	herrors "github.com/horizoncd/horizon/core/errors"
	"github.com/horizoncd/horizon/pkg/admission/models"
	perror "github.com/horizoncd/horizon/pkg/errors"
	"github.com/horizoncd/horizon/pkg/util/log"
	jsonpatch "gopkg.in/evanphx/json-patch.v5"
)

var (
	mutatingWebhooks   []Webhook
	validatingWebhooks []Webhook
)

func Register(kind models.Kind, webhook Webhook) {
	switch kind {
	case models.KindMutating:
		mutatingWebhooks = append(mutatingWebhooks, webhook)
	case models.KindValidating:
		validatingWebhooks = append(validatingWebhooks, webhook)
	}
}

type validateResult struct {
	req  Request
	err  error
	resp *Response
}

type Request struct {
	Operation   models.Operation       `json:"operation"`
	Resource    string                 `json:"resource"`
	Name        string                 `json:"name"`
	SubResource string                 `json:"subResource"`
	Version     string                 `json:"version"`
	Object      interface{}            `json:"object"`
	OldObject   interface{}            `json:"oldObject"`
	Options     map[string]interface{} `json:"options,omitempty"`
}

type Response struct {
	Allowed   *bool  `json:"allowed"`
	Result    string `json:"result,omitempty"`
	Patch     []byte `json:"patch,omitempty"`
	PatchType string `json:"patchType,omitempty"`
}

type Webhook interface {
	Handle(context.Context, *Request) (*Response, error)
	IgnoreError() bool
	Interest(*Request) bool
}

func Mutating(ctx context.Context, request *Request) (*Request, error) {
	ctx, cancelFunc := context.WithCancel(ctx)
	defer cancelFunc()
	if request.Object == nil {
		return request, nil
	}
	for _, webhook := range mutatingWebhooks {
		if !webhook.Interest(request) {
			continue
		}
		response, err := webhook.Handle(ctx, request)
		if err = loggingError(ctx, err, webhook); err != nil {
			return nil, err
		}
		if response != nil && response.Allowed != nil && !*response.Allowed {
			log.Infof(ctx,
				"request (resource: %s, resourceName: %s, subresource: %s, operation: %s) denied by webhook: %s",
				request.Resource, request.Name, request.SubResource,
				request.Operation, response.Result)
			return nil, perror.Wrapf(herrors.ErrForbidden, "request denied by webhook: %s", response.Result)
		}
		if response != nil && response.Patch != nil {
			request.Object, err = jsonPatch(request.Object, response.Patch)
			if err = loggingError(ctx, err, webhook); err != nil {
				return nil, err
			}
		}
	}
	return request, nil
}

func Validating(ctx context.Context, request *Request) error {
	if len(validatingWebhooks) < 1 {
		return nil
	}
	ctx, cancelFunc := context.WithCancel(ctx)
	defer cancelFunc()
	finishedCount := 0
	resCh := make(chan validateResult)
	for _, webhook := range validatingWebhooks {
		go func(webhook Webhook) {
			defer runtime.HandleCrash()
			if !webhook.Interest(request) {
				resCh <- validateResult{*request, nil, nil}
				return
			}
			response, err := webhook.Handle(ctx, request)
			if err != nil {
				if webhook.IgnoreError() {
					log.Errorf(ctx, "failed to admit request: %v", err)
					resCh <- validateResult{*request, nil, nil}
					return
				}
				resCh <- validateResult{*request, err, nil}
				return
			}
			if response == nil || response.Allowed == nil {
				if webhook.IgnoreError() {
					log.Errorf(ctx, "failed to admit request: response is nil or allowed is nil")
					resCh <- validateResult{*request, nil, nil}
					return
				}
				resCh <- validateResult{*request, perror.New("response is nil or allowed is nil"), nil}
				return
			}
			resCh <- validateResult{*request, nil, response}
		}(webhook)
	}

	for res := range resCh {
		finishedCount++
		if res.err != nil {
			return res.err
		}
		if res.resp != nil && res.resp.Allowed != nil && !*res.resp.Allowed {
			log.Infof(ctx,
				"request (resource: %s, resourceName: %s, subresource: %s, operation: %s) denied by webhook: %s",
				res.req.Resource, res.req.Name, res.req.SubResource,
				res.req.Operation, res.resp.Result)
			return perror.Wrapf(herrors.ErrForbidden, "request denied by webhook: %s", res.resp.Result)
		}
		if finishedCount >= len(validatingWebhooks) {
			close(resCh)
			break
		}
	}

	return nil
}

func loggingError(ctx context.Context, err error, webhook Webhook) error {
	if err != nil {
		if webhook.IgnoreError() {
			log.Warningf(ctx, "failed to admit request: %v", err.Error())
			return nil
		}
		log.Errorf(ctx, "failed to admit request: %v", err.Error())
		return err
	}
	return nil
}

func jsonPatch(obj interface{}, patchJSON []byte) (interface{}, error) {
	objJSON, err := json.Marshal(obj)
	if err != nil {
		return nil, err
	}
	patch, err := jsonpatch.DecodePatch(patchJSON)
	if err != nil {
		return nil, err
	}

	objPatched, err := patch.Apply(objJSON)
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(objPatched, &obj)
	if err != nil {
		return nil, err
	}
	return obj, nil
}
