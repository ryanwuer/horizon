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

package models

import (
	prmodels "github.com/horizoncd/horizon/pkg/pr/models"
	"github.com/horizoncd/horizon/pkg/server/global"
	"github.com/horizoncd/horizon/pkg/tag/models"
)

type Cluster struct {
	global.Model

	ApplicationID   uint
	Name            string
	EnvironmentName string
	RegionName      string
	Description     string
	Type            *string `gorm:"->"`
	GitURL          string
	GitSubfolder    string
	GitRef          string
	GitRefType      string
	Image           string
	Template        string
	TemplateRelease string
	Status          string
	CreatedBy       uint
	UpdatedBy       uint
	ExpireSeconds   uint
}

type ClusterWithTags struct {
	*Cluster
	Tags []*models.Tag
}

type ClusterWithRegion struct {
	*Cluster

	RegionDisplayName string
}

type ClusterEventExtra struct {
	Pipelinerun *prmodels.Pipelinerun `json:"pipelinerun"`
}
