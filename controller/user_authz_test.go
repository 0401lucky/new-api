package controller

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
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
	db := setupManageUserTestDB(t)
	require.NoError(t, authz.Init(db))
	user := createUserWithAdminPermissions(t, db, "demote-admin", common.RoleAdminUser)
	require.Positive(t, countUserAuthorizationRules(t, db, user.Id))

	body := "{\"id\":" + strconv.Itoa(user.Id) + ",\"action\":\"demote\"}"
	identity, proof := manageUserProof(t, db, service.VerificationOperation{
		Scope:   service.VerificationScopeAdminUserManage,
		Context: []byte("{\"user_id\":" + strconv.Itoa(user.Id) + ",\"action\":\"demote\"}"),
	})
	recorder := performVerifiedManageUserRequest(t, body, identity, proof)

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
	user, identity := setupSecurityEnrollmentTest(t)
	db := model.DB
	require.NoError(t, authz.SetUserPermissions(user.Id, authz.PermissionsMap{
		authz.ResourceChannel: {authz.ActionSensitiveWrite: true},
	}))
	require.Positive(t, countUserAuthorizationRules(t, db, user.Id))

	proof := issueSecurityEnrollmentProof(t, identity, service.VerificationOperation{Scope: service.VerificationScopeAccountDelete}, "password")
	recorder := securityEnrollmentRequest("DELETE", "/api/user/self", "", proof, identity, DeleteSelf)

	assert.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Body.String(), `"success":true`)
	var deleted model.User
	require.NoError(t, db.Unscoped().First(&deleted, user.Id).Error)
	assert.True(t, deleted.DeletedAt.Valid)
	assert.Zero(t, countUserAuthorizationRules(t, db, user.Id))
}

func TestAdminUserDeletionClearsTokensAndPermissionsAtomically(t *testing.T) {
	for _, hardDelete := range []bool{false, true} {
		for _, failTokenDelete := range []bool{false, true} {
			t.Run(fmt.Sprintf("hard=%v/rollback=%v", hardDelete, failTokenDelete), func(t *testing.T) {
				_, identity, target := setupAdminUserTest(t)
				db := model.DB
				require.NoError(t, db.AutoMigrate(&model.ExternalIdentityClaim{}, &model.Token{}))
				require.NoError(t, authz.SetUserPermissions(target.Id, authz.PermissionsMap{
					authz.ResourceChannel: {authz.ActionRead: true},
				}))
				raw, token := createScopedAccessToken(t, target.Id, 0, "profile:read")
				proof := issueSecurityEnrollmentProof(t, identity, service.VerificationOperation{
					Scope:   service.VerificationScopeAdminUserDelete,
					Context: []byte(fmt.Sprintf(`{"user_id":%d}`, target.Id)),
				}, service.VerificationMethodPassword)
				if failTokenDelete {
					const callback = "test:reject-access-token-delete"
					require.NoError(t, db.Callback().Delete().Before("gorm:delete").Register(callback, func(tx *gorm.DB) {
						if tx.Statement.Table == token.TableName() {
							tx.AddError(errors.New("token deletion failed"))
						}
					}))
					t.Cleanup(func() { db.Callback().Delete().Remove(callback) })
				}
				method, path, handler := http.MethodPost, "/api/user/manage", gin.HandlerFunc(ManageUser)
				body := fmt.Sprintf(`{"id":%d,"action":"delete"}`, target.Id)
				var params gin.Params
				if hardDelete {
					method, path, handler = http.MethodDelete, "/api/user/:id", DeleteUser
					params = gin.Params{{Key: "id", Value: strconv.Itoa(target.Id)}}
				}
				response := adminUserRequest(method, path, body, proof, identity, common.RoleRootUser, params, handler)
				var result securityEnrollmentResponse
				require.NoError(t, common.Unmarshal(response.Body.Bytes(), &result))
				assert.Equal(t, !failTokenDelete, result.Success, response.Body.String())
				found, err := model.FindUserAccessTokenByHash(model.AccessTokenFingerprint(raw))
				require.NoError(t, err)
				var activeUsers int64
				require.NoError(t, db.Model(&model.User{}).Where("id = ?", target.Id).Count(&activeUsers).Error)
				if failTokenDelete {
					require.NotNil(t, found)
					assert.Equal(t, token.Id, found.Id)
					assert.EqualValues(t, 1, activeUsers)
					assert.Positive(t, countUserAuthorizationRules(t, db, target.Id))
					return
				}
				assert.Nil(t, found)
				assert.Zero(t, activeUsers)
				assert.Zero(t, countUserAuthorizationRules(t, db, target.Id))
			})
		}
	}
}
