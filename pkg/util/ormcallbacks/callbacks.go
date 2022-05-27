package callbacks

import (
	"g.hz.netease.com/horizon/core/common"
	"g.hz.netease.com/horizon/pkg/util/log"
	"gorm.io/gorm"
)

const (
	_createdBy = "created_by"
	_updatedBy = "updated_by"
)

// addCreatedByUpdatedByForCreateCallback will set `created_by` and `updated_by` when creating records if fields exist
func addCreatedByUpdatedByForCreateCallback(db *gorm.DB) {
	field := db.Statement.Schema.LookUpField(_createdBy)
	if field != nil {
		currentUser, err := common.FromContext(db.Statement.Context)
		if err != nil {
			db.Error = err
			return
		}
		db.Statement.SetColumn(_createdBy, currentUser.GetID(), true)
	}

	field = db.Statement.Schema.LookUpField(_updatedBy)
	if field != nil {
		currentUser, err := common.FromContext(db.Statement.Context)
		if err != nil {
			db.Error = err
			return
		}
		db.Statement.SetColumn(_updatedBy, currentUser.GetID(), true)
	}
}

// addUpdatedByForUpdateDeleteCallback will set `updated_by` when updating or deleting records if fields exist
func addUpdatedByForUpdateDeleteCallback(db *gorm.DB) {
	field := db.Statement.Schema.LookUpField(_updatedBy)
	if field != nil {
		currentUser, err := common.FromContext(db.Statement.Context)
		if err != nil {
			log.Errorf(db.Statement.Context, "context in gorm is: %+v", db.Statement.Context)
			db.Error = err
			return
		}
		db.Statement.SetColumn(_updatedBy, currentUser.GetID())
	}
}

func RegisterCustomCallbacks(db *gorm.DB) {
	_ = db.Callback().Create().After("gorm:before_create").Before("gorm:create").
		Register("add_created_by", addCreatedByUpdatedByForCreateCallback)

	_ = db.Callback().Update().After("gorm:before_update").Before("gorm:update").
		Register("add_updated_by", addUpdatedByForUpdateDeleteCallback)

	_ = db.Callback().Delete().After("gorm:before_delete").Before("gorm:delete").
		Register("add_updated_by", addUpdatedByForUpdateDeleteCallback)
}
