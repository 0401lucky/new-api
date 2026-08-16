/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
package controller

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetUserIncludesActiveTemporaryQuota(t *testing.T) {
	db := setupManageUserTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Checkin{}))

	user := model.User{
		Username: "temporary-quota-user",
		Password: "password",
		Role:     common.RoleCommonUser,
		Status:   common.UserStatusEnabled,
		Group:    "default",
		AffCode:  "temporary-quota-aff",
	}
	require.NoError(t, db.Create(&user).Error)
	expiresAt := common.NowInCheckinTimezone().Add(24 * time.Hour).Unix()
	require.NoError(t, db.Create(&model.Checkin{
		UserId:         user.Id,
		CheckinDate:    "2026-01-01",
		QuotaAwarded:   5000,
		QuotaType:      model.CheckinQuotaTypeTemporary,
		QuotaRemaining: 3000,
		QuotaExpiresAt: expiresAt,
	}).Error)

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Set("role", common.RoleRootUser)
	ctx.Params = gin.Params{{Key: "id", Value: strconv.Itoa(user.Id)}}
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/user/"+strconv.Itoa(user.Id), nil)

	GetUser(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	var response struct {
		Success bool `json:"success"`
		Data    struct {
			TemporaryQuota          int    `json:"temporary_quota"`
			TemporaryQuotaExpiresAt int64  `json:"temporary_quota_expires_at"`
			ExpiresAtDisplay        string `json:"temporary_quota_expires_at_display"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.True(t, response.Success)
	assert.Equal(t, 3000, response.Data.TemporaryQuota)
	assert.Equal(t, expiresAt, response.Data.TemporaryQuotaExpiresAt)
	assert.NotEmpty(t, response.Data.ExpiresAtDisplay)
}

func TestGetUserReturnsZeroTemporaryQuotaWhenNone(t *testing.T) {
	db := setupManageUserTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Checkin{}))

	user := model.User{
		Username: "no-temporary-quota-user",
		Password: "password",
		Role:     common.RoleCommonUser,
		Status:   common.UserStatusEnabled,
		Group:    "default",
		AffCode:  "no-temporary-quota-aff",
	}
	require.NoError(t, db.Create(&user).Error)

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Set("role", common.RoleRootUser)
	ctx.Params = gin.Params{{Key: "id", Value: strconv.Itoa(user.Id)}}
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/user/"+strconv.Itoa(user.Id), nil)

	GetUser(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	var response struct {
		Success bool `json:"success"`
		Data    struct {
			TemporaryQuota int `json:"temporary_quota"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.True(t, response.Success)
	assert.Zero(t, response.Data.TemporaryQuota)
}
