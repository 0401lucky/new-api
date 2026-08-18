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
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetUserByLinuxDOId(t *testing.T) {
	db := setupManageUserTestDB(t)

	user := model.User{
		Username:    "linuxdo-bind-user",
		Password:    "password123",
		DisplayName: "LinuxDO Bind User",
		Role:        common.RoleCommonUser,
		Status:      common.UserStatusEnabled,
		Group:       "default",
		AffCode:     "linuxdo-bind-aff",
		LinuxDOId:   "23456",
		Quota:       114514,
		Email:       "bind-secret@example.com",
	}
	require.NoError(t, db.Create(&user).Error)

	run := func(query string) *httptest.ResponseRecorder {
		gin.SetMode(gin.TestMode)
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequest(http.MethodGet, "/api/user/by_linuxdo"+query, nil)
		GetUserByLinuxDOId(ctx)
		return recorder
	}

	t.Run("found returns trimmed binding fields only", func(t *testing.T) {
		recorder := run("?linux_do_id=23456")

		require.Equal(t, http.StatusOK, recorder.Code)
		var response struct {
			Success bool `json:"success"`
			Data    struct {
				Id          int    `json:"id"`
				Username    string `json:"username"`
				DisplayName string `json:"display_name"`
				LinuxDOId   string `json:"linux_do_id"`
				Quota       int    `json:"quota"`
				Status      int    `json:"status"`
				Role        int    `json:"role"`
			} `json:"data"`
		}
		require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
		assert.True(t, response.Success)
		assert.Equal(t, user.Id, response.Data.Id)
		assert.Equal(t, "linuxdo-bind-user", response.Data.Username)
		assert.Equal(t, "LinuxDO Bind User", response.Data.DisplayName)
		assert.Equal(t, "23456", response.Data.LinuxDOId)
		assert.Equal(t, 114514, response.Data.Quota)
		assert.Equal(t, common.UserStatusEnabled, response.Data.Status)
		assert.Equal(t, common.RoleCommonUser, response.Data.Role)

		// 精简响应契约：敏感字段绝不能出现在响应体里
		assert.NotContains(t, recorder.Body.String(), "password")
		assert.NotContains(t, recorder.Body.String(), "bind-secret@example.com")
	})

	t.Run("unknown linux_do_id returns failure", func(t *testing.T) {
		recorder := run("?linux_do_id=99999")

		require.Equal(t, http.StatusOK, recorder.Code)
		var response struct {
			Success bool `json:"success"`
		}
		require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
		assert.False(t, response.Success)
	})

	t.Run("missing linux_do_id returns failure", func(t *testing.T) {
		recorder := run("")

		require.Equal(t, http.StatusOK, recorder.Code)
		var response struct {
			Success bool `json:"success"`
		}
		require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
		assert.False(t, response.Success)
	})
}
