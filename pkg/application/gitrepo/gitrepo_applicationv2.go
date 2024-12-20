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

package gitrepo

import (
	"bytes"
	"context"
	"fmt"
	"sync"

	pkgcommon "github.com/horizoncd/horizon/pkg/common"
	"github.com/horizoncd/horizon/pkg/config/template"
	trmodels "github.com/horizoncd/horizon/pkg/templaterelease/models"

	"github.com/horizoncd/horizon/core/common"
	herrors "github.com/horizoncd/horizon/core/errors"
	gitlablib "github.com/horizoncd/horizon/lib/gitlab"
	perror "github.com/horizoncd/horizon/pkg/errors"
	"github.com/horizoncd/horizon/pkg/util/angular"
	"github.com/horizoncd/horizon/pkg/util/log"
	"github.com/horizoncd/horizon/pkg/util/wlog"
	"github.com/xanzy/go-gitlab"
	goyaml "gopkg.in/yaml.v3"
	"sigs.k8s.io/yaml"
)

const (
	_filePathManifest = "manifest.yaml"

	_filePathApplication = "application.yaml"
	_filePathPipeline    = "pipeline.yaml"

	_applications          = "applications"
	_recyclingApplications = "recycling-applications"
)

type CreateOrUpdateRequest struct {
	Version string

	Environment  string
	BuildConf    map[string]interface{}
	TemplateConf map[string]interface{}
}

type ApplicationTemplate struct {
	Name    string
	Release string
}

type UpgradeValuesParam struct {
	Application   string
	TargetRelease *trmodels.TemplateRelease
	BuildConfig   *template.BuildConfig
}

type GetResponse struct {
	Manifest     map[string]interface{}
	BuildConf    map[string]interface{}
	TemplateConf map[string]interface{}
}

//go:generate mockgen -source=$GOFILE -destination=../../../mock/pkg/application/gitrepo/gitrepo_applicationv2_mock.go -package=mock_gitrepo
type ApplicationGitRepo interface {
	CreateOrUpdateApplication(ctx context.Context, application string, request CreateOrUpdateRequest) error
	GetApplication(ctx context.Context, application, environment string) (*GetResponse, error)
	// HardDeleteApplication hard delete an application by the specified application name
	HardDeleteApplication(ctx context.Context, application string) error
	Upgrade(ctx context.Context, param *UpgradeValuesParam) error
}

type appGitopsRepo struct {
	gitlabLib                  gitlablib.Interface
	applicationsGroup          *gitlab.Group
	recyclingApplicationsGroup *gitlab.Group
	defaultBranch              string
	defaultVisibility          string
}

type ApplicationGitRepoConfig struct {
	RootGroup         *gitlab.Group
	DefaultBranch     string
	DefaultVisibility string
}

var _ ApplicationGitRepo = &appGitopsRepo{}

func NewApplicationGitlabRepo(ctx context.Context, gitlabLib gitlablib.Interface,
	config ApplicationGitRepoConfig) (ApplicationGitRepo, error) {
	applicationsGroup, err := gitlabLib.GetCreatedGroup(ctx, config.RootGroup.ID,
		config.RootGroup.FullPath, _applications, config.DefaultVisibility)
	if err != nil {
		return nil, err
	}
	recyclingApplicationsGroup, err := gitlabLib.GetCreatedGroup(ctx, config.RootGroup.ID,
		config.RootGroup.FullPath, _recyclingApplications, config.DefaultVisibility)
	if err != nil {
		return nil, err
	}
	return &appGitopsRepo{
		gitlabLib:                  gitlabLib,
		applicationsGroup:          applicationsGroup,
		recyclingApplicationsGroup: recyclingApplicationsGroup,
		defaultBranch:              config.DefaultBranch,
		defaultVisibility:          config.DefaultVisibility,
	}, nil
}

func (g appGitopsRepo) CreateOrUpdateApplication(ctx context.Context,
	application string, req CreateOrUpdateRequest) error {
	const op = "gitlab repo: create or update application"
	defer wlog.Start(ctx, op).StopPrint()

	currentUser, err := common.UserFromContext(ctx)
	if err != nil {
		return err
	}

	environmentRepoName := common.ApplicationRepoDefaultEnv
	if req.Environment != "" {
		environmentRepoName = req.Environment
	}

	var envProjectExists = false
	pid := fmt.Sprintf("%v/%v/%v", g.applicationsGroup.FullPath, application, environmentRepoName)
	var project *gitlab.Project
	project, err = g.gitlabLib.GetProject(ctx, pid)
	if err != nil {
		if _, ok := perror.Cause(err).(*herrors.HorizonErrNotFound); !ok {
			return err
		}
		// if not found, test application group exist
		gid := fmt.Sprintf("%v/%v", g.applicationsGroup.FullPath, application)
		parentGroup, err := g.gitlabLib.GetGroup(ctx, gid)
		if err != nil {
			if _, ok := perror.Cause(err).(*herrors.HorizonErrNotFound); !ok {
				return err
			}
			parentGroup, err = g.gitlabLib.CreateGroup(ctx, application, application,
				&g.applicationsGroup.ID, g.defaultVisibility)
			if err != nil {
				return err
			}
		}
		project, err = g.gitlabLib.CreateProject(ctx, environmentRepoName, parentGroup.ID, g.defaultVisibility)
		if err != nil {
			return err
		}
	} else {
		envProjectExists = true
	}
	if project.DefaultBranch != g.defaultBranch {
		return perror.Wrap(herrors.ErrGitLabDefaultBranchNotMatch,
			fmt.Sprintf("expect %s, not got %s", g.defaultBranch, project.DefaultBranch))
	}

	// 2. if env template repo exists, the gitlab action is update, else the action is create
	var action = gitlablib.FileCreate
	if envProjectExists {
		action = gitlablib.FileUpdate
	}

	// 3. write files
	var templateConfYaml, buildConfYaml, manifestYaml []byte
	if req.TemplateConf != nil {
		templateConfYaml, err = yaml.Marshal(req.TemplateConf)
		if err != nil {
			log.Warningf(ctx, "templateConf marshal error, %v", req.TemplateConf)
			return perror.Wrap(herrors.ErrParamInvalid, err.Error())
		}
	}
	if req.BuildConf != nil {
		buildConfYaml, err = yaml.Marshal(req.BuildConf)
		if err != nil {
			log.Warningf(ctx, "buildConf marshal error, %v", req.BuildConf)
			return perror.Wrap(herrors.ErrParamInvalid, err.Error())
		}
	}
	if req.Version != "" {
		manifest := pkgcommon.Manifest{Version: req.Version}
		manifestYaml, err = yaml.Marshal(manifest)
		if err != nil {
			log.Warningf(ctx, "Manifest marshal error, %+v", manifest)
			return perror.Wrap(herrors.ErrParamInvalid, err.Error())
		}
	}

	actions := func() []gitlablib.CommitAction {
		actions := make([]gitlablib.CommitAction, 0)
		if req.BuildConf != nil {
			actions = append(actions, gitlablib.CommitAction{
				Action:   action,
				FilePath: _filePathPipeline,
				Content:  string(buildConfYaml),
			})
		}
		if req.TemplateConf != nil {
			actions = append(actions, gitlablib.CommitAction{
				Action:   action,
				FilePath: _filePathApplication,
				Content:  string(templateConfYaml),
			})
		}
		if req.Version != "" {
			actions = append(actions, gitlablib.CommitAction{
				Action:   action,
				FilePath: _filePathManifest,
				Content:  string(manifestYaml),
			})
		}
		return actions
	}()

	commitMsg := angular.CommitMessage("application", angular.Subject{
		Operator:    currentUser.GetName(),
		Action:      fmt.Sprintf("%s application %s configure", string(action), environmentRepoName),
		Application: angular.StringPtr(application),
	}, struct {
		Application map[string]interface{} `json:"application"`
		Pipeline    map[string]interface{} `json:"pipeline"`
	}{
		Application: req.TemplateConf,
		Pipeline:    req.BuildConf,
	})
	if _, err := g.gitlabLib.WriteFiles(ctx, pid, g.defaultBranch, commitMsg, nil, actions); err != nil {
		return err
	}
	return nil
}

func (g appGitopsRepo) GetApplication(ctx context.Context, application, environment string) (*GetResponse, error) {
	const op = "gitlab repo: get application"
	defer wlog.Start(ctx, op).StopPrint()

	// 1. get data from gitlab
	gid := fmt.Sprintf("%v/%v", g.applicationsGroup.FullPath, application)
	pid := fmt.Sprintf("%v/%v", gid, func() string {
		if environment == "" {
			return common.ApplicationRepoDefaultEnv
		}
		return environment
	}())

	// if env template not exist, use the default one
	_, err := g.gitlabLib.GetProject(ctx, pid)
	if err != nil {
		if _, ok := perror.Cause(err).(*herrors.HorizonErrNotFound); ok {
			pid = fmt.Sprintf("%v/%v", gid, common.ApplicationRepoDefaultEnv)
		}
	}

	manifestBytes, err1 := g.gitlabLib.GetFile(ctx, pid, g.defaultBranch, _filePathManifest)
	buildConfBytes, err2 := g.gitlabLib.GetFile(ctx, pid, g.defaultBranch, _filePathPipeline)
	templateConfBytes, err3 := g.gitlabLib.GetFile(ctx, pid, g.defaultBranch, _filePathApplication)
	for _, err := range []error{err1, err2, err3} {
		if err != nil {
			if _, ok := perror.Cause(err).(*herrors.HorizonErrNotFound); !ok {
				return nil, err
			}
		}
	}

	// 2. process data
	res := GetResponse{}
	TransformData := func(bytes []byte) (map[string]interface{}, error) {
		var entity map[string]interface{}
		err = yaml.Unmarshal(bytes, &entity)
		if err != nil {
			return nil, perror.Wrap(herrors.ErrParamInvalid, err.Error())
		}
		return entity, nil
	}

	if manifestBytes != nil {
		entity, err := TransformData(manifestBytes)
		if err != nil {
			return nil, err
		}
		res.Manifest = entity
	}

	if buildConfBytes != nil {
		entity, err := TransformData(buildConfBytes)
		if err != nil {
			return nil, err
		}
		res.BuildConf = entity
	}

	if templateConfBytes != nil {
		entity, err := TransformData(templateConfBytes)
		if err != nil {
			return nil, err
		}
		res.TemplateConf = entity
	}
	return &res, nil
}

func (g appGitopsRepo) HardDeleteApplication(ctx context.Context, application string) error {
	const op = "gitlab repo: hard delete application"
	defer wlog.Start(ctx, op).StopPrint()

	gid := fmt.Sprintf("%v/%v", g.applicationsGroup.FullPath, application)
	return g.gitlabLib.DeleteGroup(ctx, gid)
}

func (g appGitopsRepo) Upgrade(ctx context.Context, param *UpgradeValuesParam) error {
	const op = "git repo: upgrade application"
	defer wlog.Start(ctx, op).StopPrint()
	currentUser, err := common.UserFromContext(ctx)
	if err != nil {
		return err
	}
	// 1. get projects of application
	gid := fmt.Sprintf("%v/%v", g.applicationsGroup.FullPath, param.Application)
	projects, err := g.gitlabLib.ListGroupProjects(ctx, gid, 1, 10)
	if err != nil {
		return err
	}
	type upgradeValueBytes struct {
		fileName      string
		sourceBytes   []byte
		upgradedBytes []byte
		err           error
	}
	// 2. iterate projects and upgrade
	for _, project := range projects {
		pid := fmt.Sprintf("%v/%v", gid, project.Name)
		cases := []*upgradeValueBytes{
			{
				fileName: common.GitopsFileApplication,
			}, {
				fileName: common.GitopsAppPipeline,
			}, {
				fileName: common.GitopsFileManifest,
			},
		}

		// 1. read files
		var wgReadFile sync.WaitGroup
		wgReadFile.Add(len(cases))
		for i := range cases {
			i := i
			go func() {
				defer wgReadFile.Done()
				cases[i].sourceBytes, cases[i].err = g.gitlabLib.GetFile(ctx, pid, g.defaultBranch, cases[i].fileName)
			}()
		}
		wgReadFile.Wait()
		for _, oneCase := range cases {
			if oneCase.fileName != common.GitopsFileManifest {
				if oneCase.err != nil {
					log.Errorf(ctx, "get application value file error, file = %s, err = %s",
						oneCase.fileName, oneCase.err.Error())
					return oneCase.err
				}
			} else {
				if oneCase.err != nil {
					if _, ok := perror.Cause(oneCase.err).(*herrors.HorizonErrNotFound); !ok {
						log.Errorf(ctx, "get application value file error, file = %s, err = %s",
							oneCase.fileName, oneCase.err.Error())
						return oneCase.err
					}
				}
			}
		}

		updateApplicationValue := func(sourceBytes []byte) ([]byte, error) {
			if sourceBytes == nil {
				return nil, nil
			}

			var (
				inMap         map[string]interface{}
				upgradedBytes []byte
				err           error
			)
			err = yaml.Unmarshal(sourceBytes, &inMap)
			if err != nil {
				return nil, perror.Wrapf(herrors.ErrParamInvalid,
					"yaml Unmarshal err, file = %s, err = %s", common.GitopsFileApplication, err.Error())
			}
			// convert params to envs
			func() {
				appMap, ok := inMap["app"].(map[string]interface{})
				if !ok {
					return
				}
				paramsMap, ok := appMap["params"].(map[string]interface{})
				if ok {
					var envsArray []map[string]interface{}
					for k, v := range paramsMap {
						// v could be int, convert to string
						strV := fmt.Sprintf("%v", v)
						envsArray = append(envsArray, map[string]interface{}{
							"name":  k,
							"value": strV,
						})
					}
					delete(appMap, "params")
					appMap["envs"] = envsArray
				}
			}()
			marshal(&upgradedBytes, &err, &inMap)
			return upgradedBytes, err
		}
		updatePipelineValue := func(sourceBytes []byte) ([]byte, error) {
			if sourceBytes == nil {
				return nil, nil
			}

			var (
				inMap         map[string]interface{}
				upgradedBytes []byte
				err           error
			)
			err = yaml.Unmarshal(sourceBytes, &inMap)
			if err != nil {
				return nil, perror.Wrapf(herrors.ErrParamInvalid,
					"yaml Unmarshal err, file = %s, err = %s", common.GitopsAppPipeline, err.Error())
			}
			antScript, ok := inMap["buildxml"]
			if !ok {
				return nil, perror.Wrapf(herrors.ErrParamInvalid,
					"value parent err, file = %s", common.GitopsFilePipeline)
			}
			retMap := map[string]interface{}{
				"buildInfo": map[string]interface{}{
					"buildTool": "ant",
					"buildxml":  antScript,
				},
				"buildType":   "netease-normal",
				"environment": param.BuildConfig.Environment,
				"language":    param.BuildConfig.Language,
			}
			marshal(&upgradedBytes, &err, &retMap)
			return upgradedBytes, err
		}
		assembleManifest := func() ([]byte, error) {
			var (
				upgradedBytes []byte
				err           error
			)
			marshal(&upgradedBytes, &err, &pkgcommon.Manifest{Version: common.MetaVersion2})
			if err != nil {
				log.Errorf(ctx, "marshal manifest error, err = %s", err.Error())
			}
			return upgradedBytes, nil
		}

		// 2. upgrade value bytes
		var wgUpdateValue sync.WaitGroup
		wgUpdateValue.Add(len(cases))
		for i := range cases {
			i := i
			switch cases[i].fileName {
			case common.GitopsFileManifest:
				go func() {
					defer wgUpdateValue.Done()
					cases[i].upgradedBytes, _ = assembleManifest()
				}()
			case common.GitopsAppPipeline:
				go func() {
					defer wgUpdateValue.Done()
					cases[i].upgradedBytes, cases[i].err = updatePipelineValue(cases[i].sourceBytes)
				}()
			case common.GitopsFileApplication:
				go func() {
					defer wgUpdateValue.Done()
					cases[i].upgradedBytes, cases[i].err = updateApplicationValue(cases[i].sourceBytes)
				}()
			}
		}
		wgUpdateValue.Wait()

		// 3. write files
		var gitActions []gitlablib.CommitAction
		for _, oneCase := range cases {
			if oneCase.fileName != common.GitopsFileManifest {
				if oneCase.err != nil {
					return oneCase.err
				}
				if oneCase.sourceBytes != nil {
					gitActions = append(gitActions, gitlablib.CommitAction{
						Action:   gitlablib.FileUpdate,
						FilePath: oneCase.fileName,
						Content:  string(oneCase.upgradedBytes),
					})
				}
			} else {
				if oneCase.err != nil {
					gitActions = append(gitActions, gitlablib.CommitAction{
						Action:   gitlablib.FileCreate,
						FilePath: oneCase.fileName,
						Content:  string(oneCase.upgradedBytes),
					})
				} else {
					gitActions = append(gitActions, gitlablib.CommitAction{
						Action:   gitlablib.FileUpdate,
						FilePath: oneCase.fileName,
						Content:  string(oneCase.upgradedBytes),
					})
				}
			}
		}
		commitMsg := angular.CommitMessage("application", angular.Subject{
			Operator: currentUser.GetName(),
			Action:   "upgrade application",
		}, struct {
			TargetTemplate ApplicationTemplate `json:"targetTemplate"`
		}{
			TargetTemplate: ApplicationTemplate{
				Name:    param.TargetRelease.TemplateName,
				Release: param.TargetRelease.Name,
			},
		})
		_, err := g.gitlabLib.WriteFiles(ctx, pid, g.defaultBranch, commitMsg, nil, gitActions)
		if err != nil {
			return err
		}
	}

	return nil
}

func marshal(b *[]byte, err *error, data interface{}) {
	buf := bytes.NewBuffer(make([]byte, 0))
	encoder := goyaml.NewEncoder(buf)
	encoder.SetIndent(2)
	*err = encoder.Encode(data)
	if (*err) != nil {
		*err = perror.Wrap(herrors.ErrParamInvalid, (*err).Error())
	} else {
		*b = buf.Bytes()
	}
}
