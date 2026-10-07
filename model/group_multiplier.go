package model

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"maps"
	"math"
	"sync"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const GroupMultiplierPoliciesOption = "GroupMultiplierPolicies"

const (
	MultiplierFixed       = "fixed"
	MultiplierBalance     = "balance"
	MultiplierConcurrency = "concurrency"
)

type ConcurrencyTier struct {
	Minimum    int64   `json:"minimum"`
	Multiplier float64 `json:"multiplier"`
}

type GroupMultiplierPolicy struct {
	Mode  string            `json:"mode"`
	Tiers []ConcurrencyTier `json:"tiers"`
}

var ErrGroupMultiplierConflict = errors.New("倍率配置已变化，请刷新后重试")

var groupMultiplierCache = struct {
	sync.RWMutex
	policies map[string]GroupMultiplierPolicy
}{policies: map[string]GroupMultiplierPolicy{}}

var groupMultiplierWriteMu sync.Mutex

func (p GroupMultiplierPolicy) Validate() error {
	switch p.Mode {
	case MultiplierFixed, MultiplierBalance, MultiplierConcurrency:
	default:
		return errors.New("无效的倍率模式")
	}
	if len(p.Tiers) > 32 || (p.Mode == MultiplierConcurrency && len(p.Tiers) == 0) {
		return errors.New("并发模式需要 1 到 32 个阶梯")
	}
	for i, tier := range p.Tiers {
		if tier.Minimum < 0 || tier.Minimum > 1_000_000 || (i == 0 && tier.Minimum != 0) || (i > 0 && tier.Minimum <= p.Tiers[i-1].Minimum) {
			return errors.New("并发阶梯必须从 0 开始，阈值递增且不超过 1000000")
		}
		if math.IsNaN(tier.Multiplier) || math.IsInf(tier.Multiplier, 0) || tier.Multiplier <= 0 || tier.Multiplier > 1000 || (i > 0 && tier.Multiplier < p.Tiers[i-1].Multiplier) {
			return errors.New("阶梯系数必须大于 0、不超过 1000，且不能随并发增加而降低")
		}
	}
	return nil
}

func (p GroupMultiplierPolicy) TierAt(concurrency int64) ConcurrencyTier {
	selected := ConcurrencyTier{Multiplier: 1}
	for _, tier := range p.Tiers {
		if tier.Minimum > concurrency {
			break
		}
		selected = tier
	}
	return selected
}

func parseGroupMultiplierPolicies(value string) (map[string]GroupMultiplierPolicy, error) {
	var policies map[string]GroupMultiplierPolicy
	if err := common.UnmarshalJsonStr(value, &policies); err != nil {
		return nil, err
	}
	if policies == nil {
		return nil, errors.New("倍率配置必须是对象")
	}
	for group, policy := range policies {
		if policy.Tiers == nil {
			policy.Tiers = []ConcurrencyTier{}
			policies[group] = policy
		}
		if group == "" {
			return nil, errors.New("分组不能为空")
		}
		if err := policy.Validate(); err != nil {
			return nil, fmt.Errorf("%s: %w", group, err)
		}
	}
	return policies, nil
}

func loadGroupMultiplierPolicies(value string) error {
	policies, err := parseGroupMultiplierPolicies(value)
	if err != nil {
		return err
	}
	groupMultiplierCache.Lock()
	groupMultiplierCache.policies = policies
	groupMultiplierCache.Unlock()
	return nil
}

func GroupMultiplierPolicyVersion(policy *GroupMultiplierPolicy) string {
	encoded, _ := common.Marshal(policy)
	return fmt.Sprintf("%x", sha256.Sum256(encoded))
}

// Missing entries preserve the old switch and all existing rule semantics.
func GetGroupMultiplierPolicy(group string) (GroupMultiplierPolicy, string) {
	groupMultiplierCache.RLock()
	policy, exists := groupMultiplierCache.policies[group]
	groupMultiplierCache.RUnlock()
	if !exists {
		mode := MultiplierFixed
		if common.DynamicRatioEnabled {
			mode = MultiplierBalance
		}
		return GroupMultiplierPolicy{Mode: mode, Tiers: []ConcurrencyTier{}}, GroupMultiplierPolicyVersion(nil)
	}
	policy.Tiers = append([]ConcurrencyTier{}, policy.Tiers...)
	return policy, GroupMultiplierPolicyVersion(&policy)
}

// Save one group under the shared option row lock. The version protects an
// administrator's draft; the transaction also preserves other groups' edits.
func SaveGroupMultiplierPolicy(group string, policy GroupMultiplierPolicy, expectedVersion string) error {
	if policy.Tiers == nil {
		policy.Tiers = []ConcurrencyTier{}
	}
	if err := policy.Validate(); err != nil {
		return err
	}
	groupMultiplierWriteMu.Lock()
	defer groupMultiplierWriteMu.Unlock()
	var encoded []byte
	err := DB.Transaction(func(tx *gorm.DB) error {
		row := Option{Key: GroupMultiplierPoliciesOption, Value: "{}"}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; err != nil {
			return err
		}
		if err := lockForUpdate(tx).Scopes(WithOptionKey(row.Key)).First(&row).Error; err != nil {
			return err
		}
		policies, err := parseGroupMultiplierPolicies(row.Value)
		if err != nil {
			return err
		}
		var previous *GroupMultiplierPolicy
		if p, exists := policies[group]; exists {
			previous = &p
		}
		if expectedVersion != GroupMultiplierPolicyVersion(previous) {
			return ErrGroupMultiplierConflict
		}
		policies = maps.Clone(policies)
		policies[group] = policy
		encoded, err = common.Marshal(policies)
		if err != nil {
			return err
		}
		return tx.Model(&Option{}).Scopes(WithOptionKey(row.Key)).Update("value", string(encoded)).Error
	})
	if err != nil {
		return err
	}
	return updateOptionMap(GroupMultiplierPoliciesOption, string(encoded))
}
