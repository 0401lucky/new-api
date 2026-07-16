package controller

import (
	"crypto/rand"
	"math/big"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
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
	pageInfo := common.GetPageQuery(c)
	redemptions, total, err := model.SearchRedemptions(keyword, pageInfo.GetStartIdx(), pageInfo.GetPageSize())
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
		common.ApiErrorI18n(c, i18n.MsgPaymentComplianceRequired)
		return
	}

	req := createRedemptionRequest{}
	err := c.ShouldBindJSON(&req)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if utf8.RuneCountInString(req.Name) == 0 || utf8.RuneCountInString(req.Name) > 20 {
		common.ApiErrorI18n(c, i18n.MsgRedemptionNameLength)
		return
	}
	count := req.effectiveCount()
	if count <= 0 {
		common.ApiErrorI18n(c, i18n.MsgRedemptionCountPositive)
		return
	}
	if count > redemptionBulkCreateMaxCount {
		common.ApiErrorI18n(c, i18n.MsgRedemptionCountMax)
		return
	}
	if valid, msg := validateExpiredTime(c, req.ExpiredTime); !valid {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": msg})
		return
	}
	if !req.randomQuotaMode() && req.Quota <= 0 {
		common.ApiErrorMsg(c, "额度必须大于 0")
		return
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
			common.SysError("failed to insert redemption: " + err.Error())
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": i18n.T(c, i18n.MsgRedemptionCreateFailed),
				"data":    keys,
			})
			return
		}
		keys = append(keys, key)
	}
	auditParams := map[string]interface{}{
		"name":  req.Name,
		"count": count,
	}
	if randomQuotaMode {
		auditParams["quota_min"] = logger.LogQuota(*req.QuotaMin)
		auditParams["quota_max"] = logger.LogQuota(*req.QuotaMax)
	} else {
		auditParams["quota"] = logger.LogQuota(req.Quota)
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
		if valid, msg := validateExpiredTime(c, redemption.ExpiredTime); !valid {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": msg})
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

func validateExpiredTime(c *gin.Context, expired int64) (bool, string) {
	if expired != 0 && expired < common.GetTimestamp() {
		return false, i18n.T(c, i18n.MsgRedemptionExpireTimeInvalid)
	}
	return true, ""
}
