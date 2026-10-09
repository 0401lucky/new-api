package controller

import (
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/service/authz"
	"github.com/gin-gonic/gin"
)

func GetUserActivity(c *gin.Context) {
	filter := model.UserActivityFilter{
		Keyword: c.Query("keyword"), Group: c.Query("group"), Activity: c.Query("activity"),
		SortBy: c.Query("sort_by"), SortDesc: c.Query("sort_order") == "desc",
	}
	if !slices.Contains([]string{"", "active", "inactive", "very_inactive", "never_requested", "unknown", "cleanup"}, filter.Activity) {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	if value := c.Query("role"); value != "" {
		role, err := strconv.Atoi(value)
		if err != nil || !common.IsValidateRole(role) {
			common.ApiErrorI18n(c, i18n.MsgInvalidParams)
			return
		}
		filter.Role = &role
	}
	if value := c.Query("status"); value != "" {
		status, err := strconv.Atoi(value)
		if err != nil || !slices.Contains([]int{common.UserStatusEnabled, common.UserStatusDisabled}, status) {
			common.ApiErrorI18n(c, i18n.MsgInvalidParams)
			return
		}
		filter.Status = &status
	}
	result, err := model.GetUserActivity(filter, common.GetPageQuery(c), common.GetTimestamp())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, result)
}

func BatchDeleteInactiveUsers(c *gin.Context) {
	var request service.AdminUserBatchDeleteContext
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	// The proof is bound to the exact sorted selection, the operator's current
	// session and this scope. Consumption enforces expiry and single use.
	authorization := requireAdminUserProof(c, service.VerificationScopeAdminUserBatchDelete, request)
	if authorization == nil {
		return
	}
	deleted, err := model.DeleteInactiveUsers(request.UserIDs, authz.ClearUserAuthorizationInTx)
	if err != nil {
		if errors.Is(err, model.ErrInactiveUserSelection) {
			c.JSON(http.StatusConflict, gin.H{"success": false, "message": err.Error()})
		} else {
			common.ApiError(c, err)
		}
		return
	}
	if err := authz.ReloadPolicy(); err != nil {
		common.SysError("reload authorization after inactive user cleanup: " + err.Error())
	}
	for _, user := range deleted {
		if err := model.InvalidateUserCache(user.UserID); err != nil {
			common.SysError(fmt.Sprintf("invalidate deleted user %d cache: %v", user.UserID, err))
		}
		if err := model.InvalidateUserTokensCache(user.UserID); err != nil {
			common.SysError(fmt.Sprintf("invalidate deleted user %d token cache: %v", user.UserID, err))
		}
		recordManageAuditFor(c, user.UserID, "user.delete", map[string]any{
			"id": user.UserID, "username": user.Username, "reason": "inactive_30_days",
			"verification_method": authorization.Method, "revoked_access_tokens": user.RevokedAccessTokens,
		})
	}
	common.ApiSuccess(c, gin.H{"deleted": len(deleted)})
}
