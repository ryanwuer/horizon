package manager

import (
	"context"
	"net/http"

	"g.hz.netease.com/horizon/core/common"
	"g.hz.netease.com/horizon/lib/q"
	"g.hz.netease.com/horizon/pkg/cluster/dao"
	"g.hz.netease.com/horizon/pkg/cluster/models"
	"g.hz.netease.com/horizon/pkg/util/errors"

	"gorm.io/gorm"
)

var (
	// Mgr is the global cluster manager
	Mgr = New()
)

const _errCodeClusterNotFound = errors.ErrorCode("ClusterNotFound")

type Manager interface {
	Create(ctx context.Context, cluster *models.Cluster) (*models.Cluster, error)
	GetByID(ctx context.Context, id uint) (*models.Cluster, error)
	UpdateByID(ctx context.Context, id uint, cluster *models.Cluster) (*models.Cluster, error)
	ListByApplicationAndEnv(ctx context.Context, applicationID uint, environment,
		filter string, query *q.Query) (int, []*models.ClusterWithEnvAndRegion, error)
	CheckClusterExists(ctx context.Context, cluster string) (bool, error)
}

func New() Manager {
	return &manager{
		dao: dao.NewDAO(),
	}
}

type manager struct {
	dao dao.DAO
}

func (m *manager) Create(ctx context.Context, cluster *models.Cluster) (*models.Cluster, error) {
	return m.dao.Create(ctx, cluster)
}

func (m *manager) GetByID(ctx context.Context, id uint) (*models.Cluster, error) {
	const op = "cluster manager: get by name"
	cluster, err := m.dao.GetByID(ctx, id)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, errors.E(op, http.StatusNotFound, _errCodeClusterNotFound)
		}
		return nil, errors.E(op, err)
	}
	return cluster, nil
}

func (m *manager) UpdateByID(ctx context.Context, id uint, cluster *models.Cluster) (*models.Cluster, error) {
	return m.dao.UpdateByID(ctx, id, cluster)
}

func (m *manager) ListByApplicationAndEnv(ctx context.Context, applicationID uint, environment,
	filter string, query *q.Query) (int, []*models.ClusterWithEnvAndRegion, error) {
	if query == nil {
		query = &q.Query{
			PageNumber: common.DefaultPageNumber,
			PageSize:   common.DefaultPageSize,
		}
	}
	if query.PageNumber < 1 {
		query.PageNumber = common.DefaultPageNumber
	}
	if query.PageSize < 1 {
		query.PageSize = common.DefaultPageSize
	}
	return m.dao.ListByApplicationAndEnv(ctx, applicationID, environment, filter, query)
}

func (m *manager) CheckClusterExists(ctx context.Context, cluster string) (bool, error) {
	return m.dao.CheckClusterExists(ctx, cluster)
}
