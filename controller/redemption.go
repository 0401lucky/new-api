package controller

import (
	"crypto/rand"
	"math/big"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/gin-gonic/gin"
)

const (
	redemptionKeyMaxLength       = 32
	redemptionMinRandomKeyLength = 8
	redemptionBulkCreateMaxCount = 100000
)

type createRedemptionRequest struct {
	Name               string `json:"name"`
	Quota              int    `json:"quota"`
	ExpiredTime        int64  `json:"expired_time"`
	Count              int    `json:"count"`
	KeyPrefix          string `json:"key_prefix"`
	RandomQuotaEnabled bool   `json:"random_quota_enabled"`
	QuotaMin           *int   `json:"quota_min"`
	QuotaMax           *int   `json:"quota_max"`
}

func (r createRedemptionRequest) effectiveCount() int {
	if r.Count <= 0 {
		return 1
	}
	return r.Count
}

func (r createRedemptionRequest) randomQuotaMode() bool {
	return r.RandomQuotaEnabled || (r.QuotaMin != nil && r.QuotaMax != nil)
}

func GetAllRedemptions(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	redemptions, total, err := model.GetAllRedemptions(pageInfo.GetStartIdx(), pageInfo.GetPageSize())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(redemptions)
	common.ApiSuccess(c, pageInfo)
	return
}

func SearchRedemptions(c *gin.Context) {
	keyword := c.Query("keyword")
	status := c.Query("status")
	pageInfo := common.GetPageQuery(c)
	redemptions, total, err := model.SearchRedemptions(keyword, status, pageInfo.GetStartIdx(), pageInfo.GetPageSize())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(redemptions)
	common.ApiSuccess(c, pageInfo)
	return
}

func GetRedemption(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	redemption, err := model.GetRedemptionById(id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    redemption,
	})
	return
}

func AddRedemption(c *gin.Context) {
	if !operation_setting.IsPaymentComplianceConfirmed() {
		common.ApiErrorT(c, "Payment, redemption, subscription, and invitation reward features are disabled. The administrator must confirm compliance terms before enabling them.")
		return
	}

	req := createRedemptionRequest{}
	err := c.ShouldBindJSON(&req)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if utf8.RuneCountInString(req.Name) == 0 || utf8.RuneCountInString(req.Name) > 20 {
		common.ApiErrorT(c, "Redemption code name length must be between 1-20")
		return
	}
	count := req.effectiveCount()
	if count <= 0 {
		common.ApiErrorT(c, "Redemption code count must be greater than 0")
		return
	}
	if count > redemptionBulkCreateMaxCount {
		common.ApiErrorT(c, "Maximum {{count}} redemption codes can be generated at once", map[string]any{"count": redemptionBulkCreateMaxCount})
		return
	}
	if err := validateExpiredTime(req.ExpiredTime); err != nil {
		common.ApiError(c, err)
		return
	}
	if !req.randomQuotaMode() && req.Quota <= 0 {
		common.ApiErrorMsg(c, "额度必须大于 0")
		return
	}
	if !req.randomQuotaMode() {
		if err := common.ValidateWalletQuota(req.Quota); err != nil {
			common.ApiError(c, err)
			return
		}
	}

	randomQuotaMode := req.randomQuotaMode()
	if randomQuotaMode {
		if req.QuotaMin == nil || req.QuotaMax == nil {
			common.ApiErrorMsg(c, "随机额度模式需要填写最小额度和最大额度")
			return
		}
		if *req.QuotaMin <= 0 || *req.QuotaMax <= 0 {
			common.ApiErrorMsg(c, "随机额度范围必须大于 0")
			return
		}
		if *req.QuotaMin > *req.QuotaMax {
			common.ApiErrorMsg(c, "随机额度最小值不能大于最大值")
			return
		}
		if err := common.ValidateWalletQuota(*req.QuotaMax); err != nil {
			common.ApiError(c, err)
			return
		}
	}

	keyPrefix := strings.TrimSpace(req.KeyPrefix)
	prefixLen := utf8.RuneCountInString(keyPrefix)
	if prefixLen > redemptionKeyMaxLength-redemptionMinRandomKeyLength {
		common.ApiErrorMsg(c, "兑换码前缀过长")
		return
	}
	randomKeyLength := redemptionKeyMaxLength - prefixLen
	var keys []string
	for i := 0; i < count; i++ {
		randomPart, err := common.GenerateRandomCharsKey(randomKeyLength)
		if err != nil {
			common.ApiError(c, err)
			return
		}
		key := keyPrefix + randomPart
		quota := req.Quota
		if randomQuotaMode {
			quota, err = cryptoRandIntInclusive(*req.QuotaMin, *req.QuotaMax)
			if err != nil {
				common.ApiError(c, err)
				return
			}
		}
		cleanRedemption := model.Redemption{
			UserId:      c.GetInt("id"),
			Name:        req.Name,
			Key:         key,
			CreatedTime: common.GetTimestamp(),
			Quota:       quota,
			ExpiredTime: req.ExpiredTime,
		}
		err = cleanRedemption.Insert()
		if err != nil {
			common.SysError(common.LogText("failed to insert redemption: %s", err.Error()))
			common.ApiErrorStatus(c, http.StatusOK, common.NewMessage("Failed to create redemption code, please try again later"), gin.H{"data": keys})
			return
		}
		keys = append(keys, key)
	}
	auditParams := map[string]any{
		"name":  req.Name,
		"count": count,
	}
	if randomQuotaMode {
		auditParams["quota_min"] = logger.FormatQuota(*req.QuotaMin)
		auditParams["quota_max"] = logger.FormatQuota(*req.QuotaMax)
	} else {
		auditParams["quota"] = logger.FormatQuota(req.Quota)
	}
	recordManageAudit(c, "redemption.create", auditParams)
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    keys,
		"keys":    keys,
	})
	return
}

func cryptoRandIntInclusive(min int, max int) (int, error) {
	if min > max {
		return 0, strconv.ErrSyntax
	}
	n, err := rand.Int(rand.Reader, big.NewInt(int64(max-min+1)))
	if err != nil {
		return 0, err
	}
	return min + int(n.Int64()), nil
}

func DeleteRedemption(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	err := model.DeleteRedemptionById(id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
	return
}

func UpdateRedemption(c *gin.Context) {
	statusOnly := c.Query("status_only")
	redemption := model.Redemption{}
	err := c.ShouldBindJSON(&redemption)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	cleanRedemption, err := model.GetRedemptionById(redemption.Id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if statusOnly == "" {
		if redemption.Quota <= 0 {
			common.ApiErrorT(c, "Redemption quota must be positive")
			return
		}
		if err := common.ValidateWalletQuota(redemption.Quota); err != nil {
			common.ApiError(c, err)
			return
		}
		if err := validateExpiredTime(redemption.ExpiredTime); err != nil {
			common.ApiError(c, err)
			return
		}
		// If you add more fields, please also update redemption.Update()
		cleanRedemption.Name = redemption.Name
		cleanRedemption.Quota = redemption.Quota
		cleanRedemption.ExpiredTime = redemption.ExpiredTime
	}
	if statusOnly != "" {
		cleanRedemption.Status = redemption.Status
	}
	err = cleanRedemption.Update()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    cleanRedemption,
	})
	return
}

func DeleteInvalidRedemption(c *gin.Context) {
	rows, err := model.DeleteInvalidRedemptions()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    rows,
	})
	return
}

func DeleteValidRedemptions(c *gin.Context) {
	rows, err := model.DeleteValidRedemptions()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    rows,
	})
}

func validateExpiredTime(expired int64) error {
	if expired != 0 && expired < common.GetTimestamp() {
		return common.NewMessage("Expiration time cannot be earlier than current time")
	}
	return nil
}

func DeleteRedemptionBatch(c *gin.Context) {
	var request struct {
		Ids []int `json:"ids" binding:"required,min=1,max=1000,dive,gt=0"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	count, err := model.BatchDeleteRedemptions(request.Ids)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	recordManageAudit(c, "redemption.delete_batch", map[string]any{
		"count":                    count,
		"total":                    len(request.Ids),
		"requested_redemption_ids": request.Ids,
	})
	common.ApiSuccess(c, count)
}
