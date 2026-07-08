package controller

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
)

const (
	adminPermissionResourceChannel      = "channel"
	adminPermissionActionRead           = "read"
	adminPermissionActionOperate        = "operate"
	adminPermissionActionWrite          = "write"
	adminPermissionActionSensitiveWrite = "sensitive_write"
	adminPermissionActionSecretView     = "secret_view"
)

type adminPermissionActionDef struct {
	Action         string `json:"action"`
	LabelKey       string `json:"label_key"`
	DescriptionKey string `json:"description_key"`
}

type adminPermissionResourceDef struct {
	Resource string                     `json:"resource"`
	LabelKey string                     `json:"label_key"`
	Actions  []adminPermissionActionDef `json:"actions"`
}

type adminPermissionRoleDef struct {
	Key       string                    `json:"key"`
	Name      string                    `json:"name"`
	BuiltIn   bool                      `json:"built_in"`
	Superuser bool                      `json:"superuser"`
	Grants    dto.AdminPermissionMatrix `json:"grants"`
}

var adminPermissionResources = []adminPermissionResourceDef{
	{
		Resource: adminPermissionResourceChannel,
		LabelKey: "Admin Channel Permissions",
		Actions: []adminPermissionActionDef{
			{
				Action:         adminPermissionActionRead,
				LabelKey:       "Read channels",
				DescriptionKey: "Channel Management",
			},
			{
				Action:         adminPermissionActionOperate,
				LabelKey:       "Operate channels",
				DescriptionKey: "Test channels, refresh balances, and enable/disable individual, batch, or tagged channels.",
			},
			{
				Action:         adminPermissionActionWrite,
				LabelKey:       "Update Channel",
				DescriptionKey: "Create Channel",
			},
			{
				Action:         adminPermissionActionSensitiveWrite,
				LabelKey:       "Edit sensitive channel settings",
				DescriptionKey: "Edit sensitive channel settings",
			},
			{
				Action:         adminPermissionActionSecretView,
				LabelKey:       "View channel secrets",
				DescriptionKey: "View channel secrets",
			},
		},
	},
}

func cloneAdminPermissionMatrix(source dto.AdminPermissionMatrix) dto.AdminPermissionMatrix {
	cloned := make(dto.AdminPermissionMatrix, len(source))
	for resource, actions := range source {
		cloned[resource] = make(map[string]bool, len(actions))
		for action, enabled := range actions {
			cloned[resource][action] = enabled
		}
	}
	return cloned
}

func defaultAdminPermissionMatrix(role int) dto.AdminPermissionMatrix {
	enabled := role >= common.RoleAdminUser
	matrix := dto.AdminPermissionMatrix{}
	for _, resource := range adminPermissionResources {
		matrix[resource.Resource] = map[string]bool{}
		for _, action := range resource.Actions {
			matrix[resource.Resource][action.Action] = enabled
		}
	}
	return matrix
}

func normalizeAdminPermissionMatrix(role int, value dto.AdminPermissionMatrix) dto.AdminPermissionMatrix {
	normalized := defaultAdminPermissionMatrix(role)
	if role < common.RoleAdminUser || len(value) == 0 {
		return normalized
	}
	for _, resource := range adminPermissionResources {
		for _, action := range resource.Actions {
			if actions, ok := value[resource.Resource]; ok {
				if enabled, ok := actions[action.Action]; ok {
					normalized[resource.Resource][action.Action] = enabled
				}
			}
		}
	}
	return normalized
}

func getUserAdminPermissionMatrix(user *model.User) dto.AdminPermissionMatrix {
	if user == nil {
		return defaultAdminPermissionMatrix(common.RoleCommonUser)
	}
	settings := user.GetSetting()
	if len(settings.AdminPermissions) == 0 {
		return defaultAdminPermissionMatrix(user.Role)
	}
	return normalizeAdminPermissionMatrix(user.Role, settings.AdminPermissions)
}

func fillUserAdminPermissions(user *model.User) {
	if user == nil {
		return
	}
	user.AdminPermissions = getUserAdminPermissionMatrix(user)
}

func saveUserAdminPermissions(userID int, role int, permissions dto.AdminPermissionMatrix) error {
	user, err := model.GetUserById(userID, true)
	if err != nil {
		return err
	}
	settings := user.GetSetting()
	if role >= common.RoleAdminUser {
		settings.AdminPermissions = normalizeAdminPermissionMatrix(role, permissions)
	} else {
		settings.AdminPermissions = nil
	}
	user.SetSetting(settings)
	return user.Update(false)
}

func GetAuthzCatalog(c *gin.Context) {
	common.ApiSuccess(c, gin.H{
		"resources": adminPermissionResources,
		"roles": []adminPermissionRoleDef{
			{
				Key:       "admin",
				Name:      "Admin",
				BuiltIn:   true,
				Superuser: false,
				Grants:    cloneAdminPermissionMatrix(defaultAdminPermissionMatrix(common.RoleAdminUser)),
			},
			{
				Key:       "root",
				Name:      "Root",
				BuiltIn:   true,
				Superuser: true,
				Grants:    cloneAdminPermissionMatrix(defaultAdminPermissionMatrix(common.RoleRootUser)),
			},
		},
	})
}
