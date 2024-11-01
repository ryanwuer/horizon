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

package cluster

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/horizoncd/horizon/core/common"
	"github.com/horizoncd/horizon/pkg/server/route"
)

// RegisterRoute register routes
func (a *API) RegisterRoute(engine *gin.Engine) {
	apiGroup := engine.Group("/apis/core/v1")
	var routes = route.Routes{
		{
			Method:      http.MethodPost,
			Pattern:     fmt.Sprintf("/applications/:%v/clusters", common.ParamApplicationID),
			HandlerFunc: a.Create,
		}, {
			Method:      http.MethodGet,
			Pattern:     fmt.Sprintf("/applications/:%v/clusters", common.ParamApplicationID),
			HandlerFunc: a.ListByApplication,
		}, {
			Method:      http.MethodGet,
			Pattern:     "/clusters",
			HandlerFunc: a.List,
		}, {
			Method:      http.MethodPut,
			Pattern:     fmt.Sprintf("/clusters/:%v", common.ParamClusterID),
			HandlerFunc: a.Update,
		}, {
			Method:      http.MethodGet,
			Pattern:     fmt.Sprintf("/clusters/:%v", common.ParamClusterID),
			HandlerFunc: a.Get,
		}, {
			Method:      http.MethodDelete,
			Pattern:     fmt.Sprintf("/clusters/:%v", common.ParamClusterID),
			HandlerFunc: a.Delete,
		}, {
			Method:      http.MethodPost,
			Pattern:     fmt.Sprintf("/clusters/:%v/builddeploy", common.ParamClusterID),
			HandlerFunc: a.BuildDeploy,
		}, {
			Method:      http.MethodGet,
			Pattern:     fmt.Sprintf("/clusters/:%v/diffs", common.ParamClusterID),
			HandlerFunc: a.GetDiff,
		}, {
			Method:      http.MethodGet,
			Pattern:     fmt.Sprintf("/clusters/:%v/status", common.ParamClusterID),
			HandlerFunc: a.ClusterStatus,
		}, {
			Method:      http.MethodPost,
			Pattern:     fmt.Sprintf("/clusters/:%v/restart", common.ParamClusterID),
			HandlerFunc: a.Restart,
		}, {
			Method:      http.MethodPost,
			Pattern:     fmt.Sprintf("/clusters/:%v/deploy", common.ParamClusterID),
			HandlerFunc: a.Deploy,
		}, {
			Method:      http.MethodPost,
			Pattern:     fmt.Sprintf("/clusters/:%v/rollback", common.ParamClusterID),
			HandlerFunc: a.Rollback,
		}, {
			Method:      http.MethodPost,
			Pattern:     fmt.Sprintf("/clusters/:%v/next", common.ParamClusterID),
			HandlerFunc: a.Next,
		}, {
			Method:      http.MethodGet,
			Pattern:     fmt.Sprintf("/clusters/:%v/containerlog", common.ParamClusterID),
			HandlerFunc: a.GetContainerLog,
		}, {
			Method: http.MethodPost,
			// Deprecated
			Pattern:     fmt.Sprintf("/clusters/:%v/online", common.ParamClusterID),
			HandlerFunc: a.Online,
		}, {
			Method: http.MethodPost,
			// Deprecated
			Pattern:     fmt.Sprintf("/clusters/:%v/offline", common.ParamClusterID),
			HandlerFunc: a.Offline,
		}, {
			Method:      http.MethodPost,
			Pattern:     fmt.Sprintf("/clusters/:%v/exec", common.ParamClusterID),
			HandlerFunc: a.Exec,
		}, {
			Method:      http.MethodGet,
			Pattern:     fmt.Sprintf("/clusters/:%v/dashboards", common.ParamClusterID),
			HandlerFunc: a.GetGrafanaDashBoard,
		}, {
			Method:      http.MethodGet,
			Pattern:     fmt.Sprintf("/clusters/:%v/pod", common.ParamClusterID),
			HandlerFunc: a.GetClusterPod,
		}, {
			Method:      http.MethodDelete,
			Pattern:     fmt.Sprintf("/clusters/:%v/pods", common.ParamClusterID),
			HandlerFunc: a.DeleteClusterPods,
		}, {
			Method:      http.MethodPost,
			Pattern:     fmt.Sprintf("/clusters/:%v/free", common.ParamClusterID),
			HandlerFunc: a.Free,
		}, {
			Method:      http.MethodPost,
			Pattern:     fmt.Sprintf("/clusters/:%v/maintain", common.ParamClusterID),
			HandlerFunc: a.Maintain,
		}, {
			Method:      http.MethodGet,
			Pattern:     fmt.Sprintf("/clusters/:%v/events", common.ParamClusterID),
			HandlerFunc: a.PodEvents,
		}, {
			Method:      http.MethodGet,
			Pattern:     fmt.Sprintf("/clusters/:%v/outputs", common.ParamClusterID),
			HandlerFunc: a.GetOutput,
		}, {
			Method:      http.MethodPost,
			Pattern:     fmt.Sprintf("/clusters/:%v/promote", common.ParamClusterID),
			HandlerFunc: a.Promote,
		}, {
			Method:      http.MethodPost,
			Pattern:     fmt.Sprintf("/clusters/:%v/pause", common.ParamClusterID),
			HandlerFunc: a.Pause,
		}, {
			Method:      http.MethodPost,
			Pattern:     fmt.Sprintf("/clusters/:%v/resume", common.ParamClusterID),
			HandlerFunc: a.Resume,
		}, {
			Method:      http.MethodGet,
			Pattern:     fmt.Sprintf("/clusters/:%v/containers", common.ParamClusterID),
			HandlerFunc: a.GetContainers,
		}, {
			Method: http.MethodPost,
			// Deprecated
			Pattern:     fmt.Sprintf("/clusters/:%v/upgrade", common.ParamClusterID),
			HandlerFunc: a.Upgrade,
		},
	}

	frontGroup := engine.Group("/apis/front/v1/clusters")
	var frontRoutes = route.Routes{
		{
			Method:      http.MethodGet,
			Pattern:     fmt.Sprintf("/:%v", common.ParamClusterName),
			HandlerFunc: a.GetByName,
		},
		{
			Method:      http.MethodGet,
			Pattern:     "/searchclusters",
			HandlerFunc: a.List,
		},
		{
			Method:      http.MethodGet,
			Pattern:     "/searchmyclusters",
			HandlerFunc: a.ListSelf,
		},
	}

	internalGroup := engine.Group("/apis/internal/v1/clusters")
	var internalRoutes = route.Routes{
		{
			Method:      http.MethodPost,
			Pattern:     fmt.Sprintf("/:%v/deploy", common.ParamClusterID),
			HandlerFunc: a.InternalDeploy,
		},
	}
	// TODO use middleware to auth token
	internalGroup.Use()

	route.RegisterRoutes(apiGroup, routes)
	route.RegisterRoutes(frontGroup, frontRoutes)
	route.RegisterRoutes(internalGroup, internalRoutes)
}
