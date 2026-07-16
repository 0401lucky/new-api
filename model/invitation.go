package model

import (
	"errors"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

var (
	ErrInvitationCodeRequired = errors.New("请填写邀请码")
	ErrInvitationCodeInvalid  = errors.New("无效的邀请码")
)

type InvitationCode struct {
	Id          int            `json:"id"`
	UserId      int            `json:"user_id"`
	Key         string         `json:"key" gorm:"type:char(32);uniqueIndex"`
	Status      int            `json:"status" gorm:"default:1"`
	Name        string         `json:"name" gorm:"index"`
	CreatedTime int64          `json:"created_time" gorm:"bigint"`
	UsedTime    int64          `json:"used_time" gorm:"bigint"`
	UsedUserId  int            `json:"used_user_id"`
	DeletedAt   gorm.DeletedAt `gorm:"index"`
	ExpiredTime int64          `json:"expired_time" gorm:"bigint"`
	Count       int            `json:"count" gorm:"-:all"`
}

func GetAllInvitationCodes(startIdx int, num int) (codes []*InvitationCode, total int64, err error) {
	query := DB.Model(&InvitationCode{})
	if err = query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	err = query.Order("id desc").Limit(num).Offset(startIdx).Find(&codes).Error
	return codes, total, err
}

func SearchInvitationCodes(keyword string, startIdx int, num int) (codes []*InvitationCode, total int64, err error) {
	keyword = strings.TrimSpace(keyword)
	query := DB.Model(&InvitationCode{})
	if keyword != "" {
		if id, convErr := strconv.Atoi(keyword); convErr == nil {
			query = query.Where("id = ? OR name LIKE ? OR "+commonKeyCol+" LIKE ?", id, keyword+"%", keyword+"%")
		} else {
			query = query.Where("name LIKE ? OR "+commonKeyCol+" LIKE ?", keyword+"%", keyword+"%")
		}
	}
	if err = query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	err = query.Order("id desc").Limit(num).Offset(startIdx).Find(&codes).Error
	return codes, total, err
}

func GetInvitationCodeById(id int) (*InvitationCode, error) {
	if id == 0 {
		return nil, errors.New("id 为空！")
	}
	code := InvitationCode{Id: id}
	err := DB.First(&code, "id = ?", id).Error
	return &code, err
}

func UseInvitationCodeWithTx(tx *gorm.DB, key string, userId int) error {
	key = strings.TrimSpace(key)
	if key == "" {
		return ErrInvitationCodeRequired
	}
	if userId == 0 {
		return errors.New("无效的 user id")
	}

	code := &InvitationCode{}
	if err := lockForUpdate(tx).Where(commonKeyCol+" = ?", key).First(code).Error; err != nil {
		return ErrInvitationCodeInvalid
	}
	if code.Status == common.InvitationCodeStatusUsed {
		return errors.New("该邀请码已被使用")
	}
	if code.Status != common.InvitationCodeStatusEnabled {
		return errors.New("该邀请码已禁用")
	}
	if code.ExpiredTime != 0 && code.ExpiredTime < common.GetTimestamp() {
		return errors.New("该邀请码已过期")
	}

	now := common.GetTimestamp()
	result := tx.Model(&InvitationCode{}).
		Where("id = ? AND status = ? AND (expired_time = ? OR expired_time >= ?)", code.Id, common.InvitationCodeStatusEnabled, 0, now).
		Updates(map[string]interface{}{
			"used_time":    now,
			"status":       common.InvitationCodeStatusUsed,
			"used_user_id": userId,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return errors.New("该邀请码已被使用")
	}
	return nil
}

func (code *InvitationCode) Insert() error {
	return DB.Create(code).Error
}

func (code *InvitationCode) Update() error {
	return DB.Model(code).Select("name", "status", "expired_time").Updates(code).Error
}

func (code *InvitationCode) Delete() error {
	return DB.Delete(code).Error
}

func DeleteInvitationCodeById(id int) error {
	if id == 0 {
		return errors.New("id 为空！")
	}
	code := InvitationCode{Id: id}
	if err := DB.Where(code).First(&code).Error; err != nil {
		return err
	}
	return code.Delete()
}

func DeleteInvalidInvitationCodes() (int64, error) {
	return deleteInvalidInvitationCodesAt(common.GetTimestamp())
}

func deleteInvalidInvitationCodesAt(now int64) (int64, error) {
	result := DB.Where(
		"status IN ? OR (status = ? AND expired_time != 0 AND expired_time < ?)",
		[]int{common.InvitationCodeStatusUsed, common.InvitationCodeStatusDisabled},
		common.InvitationCodeStatusEnabled,
		now,
	).Delete(&InvitationCode{})
	return result.RowsAffected, result.Error
}

func DeleteValidInvitationCodes() (int64, error) {
	return deleteValidInvitationCodesAt(common.GetTimestamp())
}

func deleteValidInvitationCodesAt(now int64) (int64, error) {
	result := DB.Where(
		"status = ? AND (expired_time = ? OR expired_time >= ?)",
		common.InvitationCodeStatusEnabled,
		0,
		now,
	).Delete(&InvitationCode{})
	return result.RowsAffected, result.Error
}
