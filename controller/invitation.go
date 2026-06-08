package controller

import (
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

const (
	invitationKeyMaxLength       = 32
	invitationMinRandomKeyLength = 8
	invitationBulkCreateMaxCount = 100000
)

type createInvitationRequest struct {
	Name        string `json:"name"`
	ExpiredTime int64  `json:"expired_time"`
	Count       int    `json:"count"`
	KeyPrefix   string `json:"key_prefix"`
}

func (r createInvitationRequest) effectiveCount() int {
	if r.Count <= 0 {
		return 1
	}
	return r.Count
}

func GetAllInvitationCodes(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	codes, total, err := model.GetAllInvitationCodes(pageInfo.GetStartIdx(), pageInfo.GetPageSize())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(codes)
	common.ApiSuccess(c, pageInfo)
}

func SearchInvitationCodes(c *gin.Context) {
	keyword := c.Query("keyword")
	pageInfo := common.GetPageQuery(c)
	codes, total, err := model.SearchInvitationCodes(keyword, pageInfo.GetStartIdx(), pageInfo.GetPageSize())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(codes)
	common.ApiSuccess(c, pageInfo)
}

func GetInvitationCode(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	code, err := model.GetInvitationCodeById(id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    code,
	})
}

func AddInvitationCode(c *gin.Context) {
	req := createInvitationRequest{}
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	if utf8.RuneCountInString(req.Name) == 0 || utf8.RuneCountInString(req.Name) > 20 {
		common.ApiErrorMsg(c, "名称长度必须在 1 到 20 个字符之间")
		return
	}
	count := req.effectiveCount()
	if count <= 0 {
		common.ApiErrorMsg(c, "生成数量必须大于 0")
		return
	}
	if count > invitationBulkCreateMaxCount {
		common.ApiErrorMsg(c, "单次最多创建 100000 个邀请码")
		return
	}
	if valid, msg := validateExpiredTime(c, req.ExpiredTime); !valid {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": msg})
		return
	}
	keyPrefix := strings.TrimSpace(req.KeyPrefix)
	prefixLen := utf8.RuneCountInString(keyPrefix)
	if prefixLen > invitationKeyMaxLength-invitationMinRandomKeyLength {
		common.ApiErrorMsg(c, "邀请码前缀过长")
		return
	}

	randomKeyLength := invitationKeyMaxLength - prefixLen
	keys := make([]string, 0, count)
	for i := 0; i < count; i++ {
		randomPart, err := common.GenerateRandomCharsKey(randomKeyLength)
		if err != nil {
			common.ApiError(c, err)
			return
		}
		key := keyPrefix + randomPart
		cleanCode := model.InvitationCode{
			UserId:      c.GetInt("id"),
			Name:        req.Name,
			Key:         key,
			CreatedTime: common.GetTimestamp(),
			ExpiredTime: req.ExpiredTime,
		}
		if err := cleanCode.Insert(); err != nil {
			common.SysError("failed to insert invitation code: " + err.Error())
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "邀请码创建失败",
				"data":    keys,
			})
			return
		}
		keys = append(keys, key)
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    keys,
		"keys":    keys,
	})
}

func DeleteInvitationCode(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	if err := model.DeleteInvitationCodeById(id); err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
}

func UpdateInvitationCode(c *gin.Context) {
	statusOnly := c.Query("status_only")
	code := model.InvitationCode{}
	if err := c.ShouldBindJSON(&code); err != nil {
		common.ApiError(c, err)
		return
	}
	cleanCode, err := model.GetInvitationCodeById(code.Id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if statusOnly == "" {
		if valid, msg := validateExpiredTime(c, code.ExpiredTime); !valid {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": msg})
			return
		}
		cleanCode.Name = code.Name
		cleanCode.ExpiredTime = code.ExpiredTime
	} else {
		cleanCode.Status = code.Status
	}
	if err := cleanCode.Update(); err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    cleanCode,
	})
}

func DeleteInvalidInvitationCodes(c *gin.Context) {
	rows, err := model.DeleteInvalidInvitationCodes()
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
