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

package envtemplate

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/horizoncd/horizon/core/common"
	"github.com/horizoncd/horizon/core/controller/build"
	"github.com/horizoncd/horizon/pkg/application/gitrepo"
	applicationmanager "github.com/horizoncd/horizon/pkg/application/manager"
	envmanager "github.com/horizoncd/horizon/pkg/environment/manager"
	eventmodels "github.com/horizoncd/horizon/pkg/event/models"
	eventservice "github.com/horizoncd/horizon/pkg/event/service"
	"github.com/horizoncd/horizon/pkg/param"
	templateschema "github.com/horizoncd/horizon/pkg/templaterelease/schema"
	"github.com/horizoncd/horizon/pkg/util/errors"
	"github.com/horizoncd/horizon/pkg/util/jsonschema"
	"github.com/horizoncd/horizon/pkg/util/sets"
)

type Controller interface {
	UpdateEnvTemplate(ctx context.Context, applicationID uint, env string, r *UpdateEnvTemplateRequest) error
	UpdateEnvTemplateV2(ctx context.Context, applicationID uint, env string, r *UpdateEnvTemplateRequest) error
	GetEnvTemplate(ctx context.Context, applicationID uint, env string) (*GetEnvTemplateResponse, error)
}

type controller struct {
	applicationGitRepo   gitrepo.ApplicationGitRepo
	templateSchemaGetter templateschema.Getter
	applicationMgr       applicationmanager.Manager
	envMgr               envmanager.Manager
	buildSchema          *build.Schema
	eventSvc             eventservice.Service
}

func NewController(param *param.Param) Controller {
	return &controller{
		applicationGitRepo:   param.ApplicationGitRepo,
		templateSchemaGetter: param.TemplateSchemaGetter,
		applicationMgr:       param.ApplicationMgr,
		envMgr:               param.EnvMgr,
		buildSchema:          param.BuildSchema,
		eventSvc:             param.EventSvc,
	}
}
func (c *controller) UpdateEnvTemplateV2(ctx context.Context, applicationID uint, env string,
	r *UpdateEnvTemplateRequest) (err error) {
	const op = "env template controller: update env templates"

	// defer the event recording function
	defer func() {
		extraStr := ""
		if err != nil {
			extra := map[string]string{
				"error": err.Error(),
			}
			extraBytes, _ := json.Marshal(extra)
			extraStr = string(extraBytes)
		}
		// record event
		c.eventSvc.CreateEventIgnoreError(ctx, common.ResourceApplication, applicationID,
			eventmodels.ApplicationUpdated, &extraStr)
	}()

	// 1. get application
	application, err := c.applicationMgr.GetByID(ctx, applicationID)
	if err != nil {
		return errors.E(op, err)
	}

	// 2. validate schema
	schema, err := c.templateSchemaGetter.GetTemplateSchema(ctx, application.Template, application.TemplateRelease, nil)
	if err != nil {
		return errors.E(op, http.StatusBadRequest, err)
	}
	if err = jsonschema.Validate(schema.Application.JSONSchema, r.Application, false); err != nil {
		return errors.E(op, http.StatusBadRequest, err)
	}
	if c.buildSchema != nil && c.buildSchema.JSONSchema != nil && r.Pipeline != nil {
		if err = jsonschema.Validate(c.buildSchema.JSONSchema, r.Pipeline, false); err != nil {
			return errors.E(op, http.StatusBadRequest, err)
		}
	}

	// 3.1 update application's git repo if env is empty
	updateReq := gitrepo.CreateOrUpdateRequest{
		Version:      common.MetaVersion2,
		Environment:  env,
		BuildConf:    r.Pipeline,
		TemplateConf: r.Application,
	}
	if env == "" {
		if err = c.applicationGitRepo.CreateOrUpdateApplication(ctx, application.Name, updateReq); err != nil {
			return errors.E(op, err)
		}
		return nil
	}

	// 3.2 check env exists
	if err = c.checkEnvExists(ctx, env); err != nil {
		return errors.E(op, err)
	}
	// 4. update application env template in git repo
	err = c.applicationGitRepo.CreateOrUpdateApplication(ctx, application.Name, updateReq)
	return err
}

func (c *controller) UpdateEnvTemplate(ctx context.Context,
	applicationID uint, env string, r *UpdateEnvTemplateRequest) (err error) {
	const op = "env template controller: update env templates"

	// defer the event recording function
	defer func() {
		extraStr := ""
		if err != nil {
			extra := map[string]string{
				"error": err.Error(),
			}
			extraBytes, _ := json.Marshal(extra)
			extraStr = string(extraBytes)
		}
		// record event
		c.eventSvc.CreateEventIgnoreError(ctx, common.ResourceApplication, applicationID,
			eventmodels.ApplicationUpdated, &extraStr)
	}()

	// 1. get application
	application, err := c.applicationMgr.GetByID(ctx, applicationID)
	if err != nil {
		return errors.E(op, err)
	}

	// 2. validate schema
	schema, err := c.templateSchemaGetter.GetTemplateSchema(ctx, application.Template, application.TemplateRelease, nil)
	if err != nil {
		return errors.E(op, http.StatusBadRequest, err)
	}
	if err = jsonschema.Validate(schema.Application.JSONSchema, r.Application, false); err != nil {
		return errors.E(op, http.StatusBadRequest, err)
	}
	if err = jsonschema.Validate(schema.Pipeline.JSONSchema, r.Pipeline, true); err != nil {
		return errors.E(op, http.StatusBadRequest, err)
	}

	// 3.1 update application's git repo if env is empty
	updateReq := gitrepo.CreateOrUpdateRequest{
		Environment:  env,
		BuildConf:    r.Pipeline,
		TemplateConf: r.Application,
	}
	if env == "" {
		if err = c.applicationGitRepo.CreateOrUpdateApplication(ctx, application.Name, updateReq); err != nil {
			return errors.E(op, err)
		}
		return nil
	}

	// 3.2 check env exists
	if err = c.checkEnvExists(ctx, env); err != nil {
		return errors.E(op, err)
	}
	// 4. update application env template in git repo
	err = c.applicationGitRepo.CreateOrUpdateApplication(ctx, application.Name, updateReq)
	return err
}

func (c *controller) GetEnvTemplate(ctx context.Context,
	applicationID uint, env string) (*GetEnvTemplateResponse, error) {
	const op = "env template controller: get env templates"

	// 1. get application
	application, err := c.applicationMgr.GetByID(ctx, applicationID)
	if err != nil {
		return nil, errors.E(op, err)
	}

	var pipelineJSONBlob, applicationJSONBlob map[string]interface{}
	// 2.1 get application's git repo if env is empty
	var repoFile *gitrepo.GetResponse
	if env == "" {
		repoFile, err = c.applicationGitRepo.GetApplication(ctx, application.Name, env)
		if repoFile != nil {
			pipelineJSONBlob = repoFile.BuildConf
			applicationJSONBlob = repoFile.TemplateConf
		}
	} else {
		// 2.2 check env exists
		if err := c.checkEnvExists(ctx, env); err != nil {
			return nil, errors.E(op, err)
		}
		// 3. get application env template
		repoFile, err = c.applicationGitRepo.GetApplication(ctx, application.Name, env)
		if repoFile != nil {
			pipelineJSONBlob = repoFile.BuildConf
			applicationJSONBlob = repoFile.TemplateConf
		}
	}

	if err != nil {
		return nil, errors.E(op, err)
	}
	return &GetEnvTemplateResponse{
		EnvTemplate: &EnvTemplate{
			Application: applicationJSONBlob,
			Pipeline:    pipelineJSONBlob,
		},
	}, nil
}

func (c *controller) checkEnvExists(ctx context.Context, envName string) error {
	const op = "env template controller: check env exists"

	envs, err := c.envMgr.ListAllEnvironment(ctx)
	if err != nil {
		return err
	}
	envSet := sets.NewString()
	for _, env := range envs {
		envSet.Insert(env.Name)
	}
	if !envSet.Has(envName) {
		return errors.E(op, http.StatusNotFound, fmt.Sprintf("environment %s is not exists", envName))
	}
	return nil
}
