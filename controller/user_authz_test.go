package controller

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service/authz"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupUserAuthzControllerTest(t *testing.T) *gorm.DB {
	t.Helper()
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.CasbinRule{}, &model.AuthzRole{}, &model.Option{}))

	wasMaster := common.IsMasterNode
	common.IsMasterNode = true
	t.Cleanup(func() {
		common.IsMasterNode = wasMaster
	})
	require.NoError(t, authz.Init(db))
	return db
}

func createUserWithAdminPermissions(t *testing.T, db *gorm.DB, username string, role int) model.User {
	t.Helper()
	user := model.User{
		Username: username,
		Password: "hashed-password",
		Role:     role,
		Status:   common.UserStatusEnabled,
		AffCode:  "aff-" + username,
	}
	settings := user.GetSetting()
	settings.AdminPermissions = map[string]map[string]bool{
		authz.ResourceChannel: {
			authz.ActionRead:           true,
			authz.ActionOperate:        true,
			authz.ActionWrite:          false,
			authz.ActionSensitiveWrite: true,
			authz.ActionSecretView:     false,
		},
	}
	user.SetSetting(settings)
	require.NoError(t, db.Create(&user).Error)
	require.NoError(t, authz.SetUserPermissions(user.Id, authz.PermissionsMap(settings.AdminPermissions)))
	return user
}

func countUserAuthorizationRules(t *testing.T, db *gorm.DB, userID int) int64 {
	t.Helper()
	var count int64
	require.NoError(t, db.Model(&model.CasbinRule{}).
		Where("ptype = ? AND v0 = ?", "p", authz.UserSubject(userID)).
		Count(&count).Error)
	return count
}

func TestManageUserDemoteClearsLegacyAndCasbinPermissions(t *testing.T) {
	db := setupUserAuthzControllerTest(t)
	user := createUserWithAdminPermissions(t, db, "demote-admin", common.RoleAdminUser)
	require.Positive(t, countUserAuthorizationRules(t, db, user.Id))

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Set("role", common.RoleRootUser)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/user/manage", strings.NewReader("{\"id\":"+strconv.Itoa(user.Id)+",\"action\":\"demote\"}"))

	ManageUser(ctx)

	assert.Equal(t, http.StatusOK, recorder.Code)
	var updated model.User
	require.NoError(t, db.Unscoped().First(&updated, user.Id).Error)
	assert.Equal(t, common.RoleCommonUser, updated.Role)
	assert.Empty(t, updated.GetSetting().AdminPermissions)
	assert.Zero(t, countUserAuthorizationRules(t, db, user.Id))
}

func TestNonRootUpdateIgnoresReturnedAdminPermissions(t *testing.T) {
	db := setupUserAuthzControllerTest(t)
	user := createUserWithAdminPermissions(t, db, "non-root-edit", common.RoleAdminUser)
	original := user.GetSetting().AdminPermissions

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Set("role", common.RoleAdminUser)
	touched, err := updateAdminPermissionsForUserInTx(ctx, db, user.Id, user.Role, map[string]map[string]bool{
		authz.ResourceChannel: {authz.ActionSensitiveWrite: false},
	})

	require.NoError(t, err)
	assert.False(t, touched)
	var updated model.User
	require.NoError(t, db.First(&updated, user.Id).Error)
	assert.Equal(t, original, updated.GetSetting().AdminPermissions)
}

func TestDeleteSelfClearsCasbinPermissions(t *testing.T) {
	db := setupUserAuthzControllerTest(t)
	user := createUserWithAdminPermissions(t, db, "delete-self", common.RoleCommonUser)
	require.Positive(t, countUserAuthorizationRules(t, db, user.Id))

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Set("id", user.Id)
	DeleteSelf(ctx)

	assert.Equal(t, http.StatusOK, recorder.Code)
	var deleted model.User
	require.NoError(t, db.Unscoped().First(&deleted, user.Id).Error)
	assert.True(t, deleted.DeletedAt.Valid)
	assert.Zero(t, countUserAuthorizationRules(t, db, user.Id))
}
