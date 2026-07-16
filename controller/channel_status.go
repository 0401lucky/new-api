package controller

import (
	"errors"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"gorm.io/gorm"
)

type channelStatusRequest struct {
	Status int `json:"status"`
}

type channelBatchStatusRequest struct {
	Ids    []int `json:"ids"`
	Status int   `json:"status"`
}

func isManualChannelStatus(status int) bool {
	return status == common.ChannelStatusEnabled || status == common.ChannelStatusManuallyDisabled
}

func updateSingleChannelStatus(id int, status int) (bool, string, error) {
	channel, err := model.GetChannelById(id, true)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, "渠道不存在", nil
	}
	if err != nil {
		return false, "", err
	}
	if channel.Status == status {
		return true, "", nil
	}

	reason := ""
	if status == common.ChannelStatusManuallyDisabled {
		reason = "手动禁用"
	}
	if !model.UpdateChannelStatus(id, "", status, reason) {
		return false, "更新渠道状态失败", nil
	}
	return true, "", nil
}
