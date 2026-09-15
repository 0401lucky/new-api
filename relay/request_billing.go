package relay

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/gin-gonic/gin"
	"github.com/samber/lo"
)

// PrepareRequestBilling estimates and reserves one request's charge. Transports
// provide the current request body through BodyStorage or BillingRequestInput;
// channel retries retain the resulting billing session and pricing snapshot.
func PrepareRequestBilling(c *gin.Context, info *relaycommon.RelayInfo) *types.NewAPIError {
	// ChannelMeta is only initialized after a channel is selected, so the group
	// and channel of the prompt check must come from the request context.
	usingGroup := common.GetContextKeyString(c, constant.ContextKeyUsingGroup)
	if usingGroup == "" {
		usingGroup = c.GetString("group")
	}
	channelID := common.GetContextKeyInt(c, constant.ContextKeyChannelId)
	needSensitiveCheck := setting.ShouldCheckPromptForRequest(info.OriginModelName, usingGroup, channelID)
	meta := &types.TokenCountMeta{TokenType: types.TokenTypeTokenizer}
	if info.Request != nil && (needSensitiveCheck || constant.CountToken) {
		meta = info.Request.GetTokenCountMeta()
	} else {
		// Avoid building CombineText when only the pricing quantities are needed.
		switch request := info.Request.(type) {
		case *dto.GeneralOpenAIRequest:
			meta.MaxTokens = int(max(lo.FromPtr(request.MaxTokens), lo.FromPtr(request.MaxCompletionTokens)))
		case *dto.OpenAIResponsesRequest:
			meta.MaxTokens = int(lo.FromPtr(request.MaxOutputTokens))
		case *dto.ClaudeRequest:
			meta.MaxTokens = int(lo.FromPtr(request.MaxTokens))
		case *dto.ImageRequest:
			meta = request.GetTokenCountMeta()
		}
	}

	if needSensitiveCheck {
		service.StartRecentCallCaptureFromContext(c, info)
		verdict := service.CheckPromptText(c.Request.Context(), meta.CombineText)
		if len(verdict.Matches) > 0 {
			logger.LogWarn(c, fmt.Sprintf("prompt check matched: action=%s, score=%d, reason=%s", verdict.Action, verdict.Score, verdict.Reason))
			recentVerdict := verdict
			if verdict.Mode == setting.PromptCheckModeMonitor && verdict.Action == service.PromptCheckActionAllow {
				recentVerdict.Action = "monitor"
			}
			service.RecentCallsCache().UpsertPromptCheckByContext(c, recentVerdict)
			service.RecordPromptCheckLog(c, info, verdict)
		}
		if verdict.Action == service.PromptCheckActionWarn {
			c.Header("X-Prompt-Check-Warning", verdict.Reason)
		}
		if verdict.Action == service.PromptCheckActionBlock {
			service.RecentCallsCache().UpsertErrorByContext(c, verdict.Reason, "prompt_check", string(types.ErrorCodePromptBlocked), http.StatusBadRequest)
			return types.NewErrorWithStatusCode(
				errors.New("request contains content blocked by prompt check"),
				types.ErrorCodePromptBlocked,
				http.StatusBadRequest,
				types.ErrOptionWithSkipRetry(),
			)
		}
	}

	tokens, err := service.EstimateRequestToken(c, meta, info)
	if err != nil {
		return types.NewError(err, types.ErrorCodeCountTokenFailed)
	}
	info.SetEstimatePromptTokens(tokens)

	priceData, err := helper.ModelPriceHelper(c, info, tokens, meta)
	if err != nil {
		return types.NewError(err, types.ErrorCodeModelPriceError, types.ErrOptionWithStatusCode(http.StatusBadRequest))
	}
	if priceData.FreeModel {
		logger.LogInfo(c, fmt.Sprintf("模型 %s 免费，跳过预扣费", info.OriginModelName))
		return nil
	}
	return service.PreConsumeBilling(c, priceData.QuotaToPreConsume, info)
}

// RefundFailedRequestBilling applies the common final-failure policy after all
// eligible attempts have ended. A settled BillingSession never refunds again.
func RefundFailedRequestBilling(c *gin.Context, info *relaycommon.RelayInfo, apiErr *types.NewAPIError) *types.NewAPIError {
	if apiErr == nil {
		return nil
	}
	apiErr = service.NormalizeViolationFeeError(apiErr)
	if info.Billing != nil {
		info.Billing.Refund(c)
	}
	service.ChargeViolationFeeIfNeeded(c, info, apiErr)
	return apiErr
}
