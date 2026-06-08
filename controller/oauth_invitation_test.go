package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/oauth"
	"github.com/stretchr/testify/require"
)

func TestLinuxDOExistingUsernameBypassesInvitationCode(t *testing.T) {
	db := setupModelListControllerTestDB(t)

	originalInvitationCodeEnabled := common.InvitationCodeEnabled
	originalRegisterEnabled := common.RegisterEnabled
	common.InvitationCodeEnabled = true
	common.RegisterEnabled = false
	t.Cleanup(func() {
		common.InvitationCodeEnabled = originalInvitationCodeEnabled
		common.RegisterEnabled = originalRegisterEnabled
	})

	require.NoError(t, db.Create(&model.User{
		Id:          2001,
		Username:    "linuxdo-user",
		DisplayName: "LinuxDO User",
		Group:       "default",
		Role:        common.RoleCommonUser,
		Status:      common.UserStatusEnabled,
	}).Error)

	user, err := findOrCreateOAuthUser(nil, &oauth.LinuxDOProvider{}, &oauth.OAuthUser{
		ProviderUserID: "123456",
		Username:       "linuxdo-user",
		DisplayName:    "LinuxDO User",
		Extra: map[string]any{
			"trust_level": common.LinuxDOMinimumTrustLevel,
		},
	}, nil)

	require.NoError(t, err)
	require.Equal(t, 2001, user.Id)
	require.Equal(t, "123456", user.LinuxDOId)

	var persisted model.User
	require.NoError(t, db.First(&persisted, 2001).Error)
	require.Equal(t, "123456", persisted.LinuxDOId)
}

func TestLinuxDOExistingProviderIDBypassesInvitationCode(t *testing.T) {
	db := setupModelListControllerTestDB(t)

	originalInvitationCodeEnabled := common.InvitationCodeEnabled
	originalRegisterEnabled := common.RegisterEnabled
	common.InvitationCodeEnabled = true
	common.RegisterEnabled = false
	t.Cleanup(func() {
		common.InvitationCodeEnabled = originalInvitationCodeEnabled
		common.RegisterEnabled = originalRegisterEnabled
	})

	require.NoError(t, db.Create(&model.User{
		Id:          2002,
		Username:    "bound-linuxdo-user",
		DisplayName: "Bound LinuxDO User",
		Group:       "default",
		Role:        common.RoleCommonUser,
		Status:      common.UserStatusEnabled,
		LinuxDOId:   "654321",
	}).Error)

	user, err := findOrCreateOAuthUser(nil, &oauth.LinuxDOProvider{}, &oauth.OAuthUser{
		ProviderUserID: "654321",
		Username:       "bound-linuxdo-user",
		DisplayName:    "Bound LinuxDO User",
		Extra: map[string]any{
			"trust_level": common.LinuxDOMinimumTrustLevel,
		},
	}, nil)

	require.NoError(t, err)
	require.Equal(t, 2002, user.Id)
	require.Equal(t, "654321", user.LinuxDOId)
}
