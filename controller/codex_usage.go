package controller

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel/codex"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type codexWhamContext struct {
	channel  *model.Channel
	oauthKey *codex.OAuthKey
	client   *http.Client
}

func newCodexWhamContext(channelId int) (*codexWhamContext, error) {
	ch, err := model.GetChannelById(channelId, true)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("channel not found")
	}
	if err != nil {
		return nil, err
	}
	if ch == nil {
		return nil, fmt.Errorf("channel not found")
	}
	if ch.Type != constant.ChannelTypeCodex {
		return nil, fmt.Errorf("channel type is not Codex")
	}
	if ch.ChannelInfo.IsMultiKey {
		return nil, fmt.Errorf("multi-key channel is not supported")
	}

	oauthKey, err := codex.ParseOAuthKey(strings.TrimSpace(ch.Key))
	if err != nil {
		common.SysError("failed to parse oauth key: " + err.Error())
		return nil, fmt.Errorf("解析凭证失败，请检查渠道配置")
	}
	accessToken := strings.TrimSpace(oauthKey.AccessToken)
	accountID := strings.TrimSpace(oauthKey.AccountID)
	if accessToken == "" {
		return nil, fmt.Errorf("codex channel: access_token is required")
	}
	if accountID == "" {
		return nil, fmt.Errorf("codex channel: account_id is required")
	}

	client, err := service.NewProxyHttpClient(ch.GetSetting().Proxy)
	if err != nil {
		return nil, err
	}

	return &codexWhamContext{
		channel:  ch,
		oauthKey: oauthKey,
		client:   client,
	}, nil
}

func refreshCodexWhamToken(c *gin.Context, wham *codexWhamContext) bool {
	if wham == nil || wham.channel == nil || wham.oauthKey == nil || strings.TrimSpace(wham.oauthKey.RefreshToken) == "" {
		return false
	}
	refreshCtx, refreshCancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer refreshCancel()

	res, refreshErr := service.RefreshCodexOAuthTokenWithProxy(refreshCtx, wham.oauthKey.RefreshToken, wham.channel.GetSetting().Proxy)
	if refreshErr != nil {
		return false
	}
	wham.oauthKey.AccessToken = res.AccessToken
	wham.oauthKey.RefreshToken = res.RefreshToken
	wham.oauthKey.LastRefresh = time.Now().Format(time.RFC3339)
	wham.oauthKey.Expired = res.ExpiresAt.Format(time.RFC3339)
	if strings.TrimSpace(wham.oauthKey.Type) == "" {
		wham.oauthKey.Type = "codex"
	}

	encoded, encErr := common.Marshal(wham.oauthKey)
	if encErr != nil {
		common.SysError(fmt.Sprintf("failed to marshal refreshed codex oauth key for channel %d: %v", wham.channel.Id, encErr))
		return true
	}
	if dbErr := model.DB.Model(&model.Channel{}).Where("id = ?", wham.channel.Id).Update("key", string(encoded)).Error; dbErr != nil {
		common.SysError(fmt.Sprintf("failed to persist refreshed codex oauth key for channel %d: %v", wham.channel.Id, dbErr))
		return true
	}
	model.InitChannelCache()
	service.ResetProxyClientCache()
	return true
}

func fetchCodexWhamWithRefresh(
	c *gin.Context,
	method string,
	endpoints []string,
) (statusCode int, body []byte, err error) {
	channelId, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return 0, nil, fmt.Errorf("invalid channel id: %w", err)
	}
	wham, err := newCodexWhamContext(channelId)
	if err != nil {
		return 0, nil, err
	}

	refreshed := false
	for index, endpoint := range endpoints {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
		statusCode, body, err = service.FetchCodexWham(
			ctx,
			wham.client,
			wham.channel.GetBaseURL(),
			wham.oauthKey.AccessToken,
			wham.oauthKey.AccountID,
			method,
			endpoint,
		)
		cancel()
		if err != nil {
			return statusCode, body, err
		}

		if (statusCode == http.StatusUnauthorized || statusCode == http.StatusForbidden) && !refreshed {
			refreshed = refreshCodexWhamToken(c, wham)
			if refreshed {
				ctx2, cancel2 := context.WithTimeout(c.Request.Context(), 15*time.Second)
				statusCode, body, err = service.FetchCodexWham(
					ctx2,
					wham.client,
					wham.channel.GetBaseURL(),
					wham.oauthKey.AccessToken,
					wham.oauthKey.AccountID,
					method,
					endpoint,
				)
				cancel2()
				if err != nil {
					return statusCode, body, err
				}
			}
		}

		if statusCode != http.StatusNotFound || index == len(endpoints)-1 {
			return statusCode, body, nil
		}
	}
	return statusCode, body, nil
}

func writeCodexWhamResponse(c *gin.Context, statusCode int, body []byte) {
	var payload any
	if common.Unmarshal(body, &payload) != nil {
		payload = string(body)
	}

	ok := statusCode >= 200 && statusCode < 300
	resp := gin.H{
		"success":         ok,
		"message":         "",
		"upstream_status": statusCode,
		"data":            payload,
	}
	if !ok {
		resp["message"] = fmt.Sprintf("upstream status: %d", statusCode)
	}
	c.JSON(http.StatusOK, resp)
}

func GetCodexChannelUsage(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()

	channelId, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, fmt.Errorf("invalid channel id: %w", err))
		return
	}
	wham, err := newCodexWhamContext(channelId)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
		return
	}

	statusCode, body, err := service.FetchCodexWhamUsage(ctx, wham.client, wham.channel.GetBaseURL(), wham.oauthKey.AccessToken, wham.oauthKey.AccountID)
	if err != nil {
		common.SysError("failed to fetch codex usage: " + err.Error())
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "获取用量信息失败，请稍后重试"})
		return
	}

	if (statusCode == http.StatusUnauthorized || statusCode == http.StatusForbidden) && refreshCodexWhamToken(c, wham) {
		ctx2, cancel2 := context.WithTimeout(c.Request.Context(), 15*time.Second)
		defer cancel2()
		statusCode, body, err = service.FetchCodexWhamUsage(ctx2, wham.client, wham.channel.GetBaseURL(), wham.oauthKey.AccessToken, wham.oauthKey.AccountID)
		if err != nil {
			common.SysError("failed to fetch codex usage after refresh: " + err.Error())
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "获取用量信息失败，请稍后重试"})
			return
		}
	}

	writeCodexWhamResponse(c, statusCode, body)
}

func GetCodexUsageResetCredits(c *gin.Context) {
	statusCode, body, err := fetchCodexWhamWithRefresh(c, http.MethodGet, []string{
		"rate_limit_reset_credits",
		"usage/reset-credits",
		"rate-limit-reset-credits",
	})
	if err != nil {
		common.SysError("failed to fetch codex reset credits: " + err.Error())
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "获取重置次数失败，请稍后重试"})
		return
	}
	writeCodexWhamResponse(c, statusCode, body)
}

func ResetCodexChannelUsage(c *gin.Context) {
	statusCode, body, err := fetchCodexWhamWithRefresh(c, http.MethodPost, []string{
		"usage/reset",
		"rate_limit_reset",
		"rate-limit-reset",
	})
	if err != nil {
		common.SysError("failed to reset codex usage: " + err.Error())
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "重置用量失败，请稍后重试"})
		return
	}
	writeCodexWhamResponse(c, statusCode, body)
}
