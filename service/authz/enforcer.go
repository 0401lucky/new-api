package authz

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/casbin/casbin/v2"
	casbinmodel "github.com/casbin/casbin/v2/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	enforcerMu sync.RWMutex
	enforcer   *casbin.SyncedEnforcer
)

const modelText = `
[request_definition]
r = sub, obj, act

[policy_definition]
p = sub, obj, act, eft

[policy_effect]
e = some(where (p.eft == allow))

[matchers]
m = r.sub == p.sub && r.obj == p.obj && r.act == p.act && p.eft == "allow"
`

func Init(db *gorm.DB) error {
	if common.IsMasterNode {
		if err := seedBuiltInRoles(db); err != nil {
			return err
		}
		if err := resetBuiltInRolePolicies(db); err != nil {
			return err
		}
	}

	m, err := casbinmodel.NewModelFromString(modelText)
	if err != nil {
		return err
	}
	e, err := casbin.NewSyncedEnforcer(m, newGormAdapter(db))
	if err != nil {
		return err
	}
	e.EnableAutoSave(true)

	enforcerMu.Lock()
	enforcer = e
	enforcerMu.Unlock()

	if !common.IsMasterNode {
		return nil
	}
	if err := seedDefaultPolicies(); err != nil {
		return err
	}
	return migrateLegacyAdminPermissions(db)
}

// migrateLegacyAdminPermissions 将旧版 User.Setting 中的权限矩阵同步为
// Casbin 覆盖规则。迁移保持幂等，并与控制器的双写逻辑配合，避免升级后
// 既有管理员被静默降权，或重启时用旧设置覆盖新授权。
func migrateLegacyAdminPermissions(db *gorm.DB) error {
	if !db.Migrator().HasTable(&model.User{}) || !db.Migrator().HasTable(&model.Option{}) {
		return nil
	}
	const migrationKey = "AuthzLegacyAdminPermissionsMigratedV1"
	var marker model.Option
	if err := db.Where("key = ?", migrationKey).First(&marker).Error; err == nil {
		return nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	var users []model.User
	if err := db.Where("role = ?", common.RoleAdminUser).Find(&users).Error; err != nil {
		return err
	}
	for i := range users {
		legacy := users[i].GetSetting().AdminPermissions
		permissions := make(PermissionsMap, len(registry))
		if len(legacy) == 0 {
			// 旧版本未配置矩阵的管理员默认拥有全部渠道权限。
			for _, resource := range registry {
				permissions[resource.Resource] = make(map[string]bool, len(resource.Actions))
				for _, action := range resource.Actions {
					permissions[resource.Resource][action.Action] = true
				}
			}
		} else {
			for resource, actions := range legacy {
				permissions[resource] = make(map[string]bool, len(actions))
				for action, allowed := range actions {
					permissions[resource][action] = allowed
				}
			}
		}
		if err := SetUserPermissions(users[i].Id, permissions); err != nil {
			return err
		}
	}
	return db.Clauses(clause.OnConflict{DoNothing: true}).Create(&model.Option{Key: migrationKey, Value: "true"}).Error
}

func currentEnforcer() *casbin.SyncedEnforcer {
	enforcerMu.RLock()
	defer enforcerMu.RUnlock()
	return enforcer
}

func ReloadPolicy() error {
	enforcerMu.Lock()
	defer enforcerMu.Unlock()
	if enforcer == nil {
		return fmt.Errorf("authz enforcer is not initialized")
	}
	return enforcer.LoadPolicy()
}

// StartPolicySync periodically reloads the authorization policy from the database.
// The enforcer keeps an in-memory snapshot, and permission changes are written
// straight to the DB (see SetUserPermissionsInTx) with only the local node's
// snapshot refreshed afterwards. Without this loop other instances in a
// multi-node deployment would keep serving stale permissions (including not
// honoring a revoked grant) until restart. Mirrors model.SyncOptions polling.
func StartPolicySync(frequency int) {
	if frequency <= 0 {
		return
	}
	for {
		time.Sleep(time.Duration(frequency) * time.Second)
		if err := ReloadPolicy(); err != nil {
			common.SysError("failed to reload authz policy: " + err.Error())
		}
	}
}
