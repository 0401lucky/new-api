package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/oauth"
	"github.com/gin-contrib/sessions"
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

func TestLinuxDOExistingUsernameMatchIgnoresCaseBypassesInvitationCode(t *testing.T) {
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
		Id:          2003,
		Username:    "LinuxDO-User",
		DisplayName: "LinuxDO User",
		Group:       "default",
		Role:        common.RoleCommonUser,
		Status:      common.UserStatusEnabled,
	}).Error)

	user, err := findOrCreateOAuthUser(nil, &oauth.LinuxDOProvider{}, &oauth.OAuthUser{
		ProviderUserID: "987654",
		Username:       "linuxdo-user",
		DisplayName:    "LinuxDO User",
		Extra: map[string]any{
			"trust_level": common.LinuxDOMinimumTrustLevel,
		},
	}, nil)

	require.NoError(t, err)
	require.Equal(t, 2003, user.Id)
	require.Equal(t, "987654", user.LinuxDOId)

	var persisted model.User
	require.NoError(t, db.First(&persisted, 2003).Error)
	require.Equal(t, "987654", persisted.LinuxDOId)
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

func TestLinuxDONewUserRequiresInvitationCode(t *testing.T) {
	setupModelListControllerTestDB(t)

	originalInvitationCodeEnabled := common.InvitationCodeEnabled
	originalRegisterEnabled := common.RegisterEnabled
	common.InvitationCodeEnabled = true
	common.RegisterEnabled = true
	t.Cleanup(func() {
		common.InvitationCodeEnabled = originalInvitationCodeEnabled
		common.RegisterEnabled = originalRegisterEnabled
	})

	user, err := findOrCreateOAuthUser(nil, &oauth.LinuxDOProvider{}, &oauth.OAuthUser{
		ProviderUserID: "112233",
		Username:       "new-linuxdo-user",
		DisplayName:    "New LinuxDO User",
		Extra: map[string]any{
			"trust_level": common.LinuxDOMinimumTrustLevel,
		},
	}, newOAuthInvitationTestSession(nil))

	var requiredErr *OAuthInvitationCodeRequiredError
	require.ErrorAs(t, err, &requiredErr)
	require.Nil(t, user)
}

type oauthInvitationTestSession struct {
	values map[interface{}]interface{}
}

func newOAuthInvitationTestSession(values map[interface{}]interface{}) *oauthInvitationTestSession {
	if values == nil {
		values = map[interface{}]interface{}{}
	}
	return &oauthInvitationTestSession{values: values}
}

func (s *oauthInvitationTestSession) ID() string {
	return ""
}

func (s *oauthInvitationTestSession) Get(key interface{}) interface{} {
	return s.values[key]
}

func (s *oauthInvitationTestSession) Set(key interface{}, val interface{}) {
	s.values[key] = val
}

func (s *oauthInvitationTestSession) Delete(key interface{}) {
	delete(s.values, key)
}

func (s *oauthInvitationTestSession) Clear() {
	s.values = map[interface{}]interface{}{}
}

func (s *oauthInvitationTestSession) AddFlash(value interface{}, vars ...string) {
}

func (s *oauthInvitationTestSession) Flashes(vars ...string) []interface{} {
	return nil
}

func (s *oauthInvitationTestSession) Options(options sessions.Options) {
}

func (s *oauthInvitationTestSession) Save() error {
	return nil
}
