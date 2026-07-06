package setting

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"sync"
	"unicode"

	"github.com/QuantumNous/new-api/common"
)

var ModelRequestRateLimitEnabled = false
var ModelRequestRateLimitDurationMinutes = 1
var ModelRequestRateLimitCount = 0
var ModelRequestRateLimitSuccessCount = 1000
var ModelRequestRateLimitConcurrencyCount = 0
var ModelRequestRateLimitGroup = map[string][3]int{}
var ModelRequestRateLimitExemptUserIDs = map[int]struct{}{}
var ModelRequestRateLimitMutex sync.RWMutex

func ModelRequestRateLimitGroup2JSONString() string {
	ModelRequestRateLimitMutex.RLock()
	defer ModelRequestRateLimitMutex.RUnlock()

	jsonBytes, err := common.Marshal(ModelRequestRateLimitGroup)
	if err != nil {
		common.SysLog("error marshalling model ratio: " + err.Error())
	}
	return string(jsonBytes)
}

func UpdateModelRequestRateLimitGroupByJSONString(jsonStr string) error {
	limits, err := ParseModelRequestRateLimitGroup(jsonStr)
	if err != nil {
		return err
	}

	ModelRequestRateLimitMutex.Lock()
	defer ModelRequestRateLimitMutex.Unlock()

	ModelRequestRateLimitGroup = limits
	return nil
}

func ParseModelRequestRateLimitExemptUserIDs(raw string) (map[int]struct{}, error) {
	result := make(map[int]struct{})
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return result, nil
	}

	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || unicode.IsSpace(r)
	})
	for _, part := range parts {
		id, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil || id <= 0 {
			return nil, fmt.Errorf("invalid userId %q", part)
		}
		result[id] = struct{}{}
	}
	return result, nil
}

func UpdateModelRequestRateLimitExemptUserIDs(raw string) error {
	ids, err := ParseModelRequestRateLimitExemptUserIDs(raw)
	if err != nil {
		return err
	}
	ModelRequestRateLimitMutex.Lock()
	defer ModelRequestRateLimitMutex.Unlock()
	ModelRequestRateLimitExemptUserIDs = ids
	return nil
}

func IsModelRequestRateLimitExemptUser(userID int) bool {
	if userID <= 0 {
		return false
	}
	ModelRequestRateLimitMutex.RLock()
	defer ModelRequestRateLimitMutex.RUnlock()
	_, ok := ModelRequestRateLimitExemptUserIDs[userID]
	return ok
}

func GetGroupRateLimit(group string) (totalCount, successCount, concurrencyCount int, found bool) {
	ModelRequestRateLimitMutex.RLock()
	defer ModelRequestRateLimitMutex.RUnlock()

	if ModelRequestRateLimitGroup == nil {
		return 0, 0, 0, false
	}

	limits, found := ModelRequestRateLimitGroup[group]
	if !found {
		return 0, 0, 0, false
	}
	return limits[0], limits[1], limits[2], true
}

func CheckModelRequestRateLimitGroup(jsonStr string) error {
	_, err := ParseModelRequestRateLimitGroup(jsonStr)
	return err
}

func ParseModelRequestRateLimitGroup(jsonStr string) (map[string][3]int, error) {
	raw := strings.TrimSpace(jsonStr)
	if raw == "" {
		return map[string][3]int{}, nil
	}

	parsed := make(map[string][]int)
	err := common.UnmarshalJsonStr(raw, &parsed)
	if err != nil {
		return nil, err
	}

	result := make(map[string][3]int, len(parsed))
	for group, limits := range parsed {
		if len(limits) != 2 && len(limits) != 3 {
			return nil, fmt.Errorf("group %s must use [maxRequests, maxSuccess] or [maxRequests, maxSuccess, maxConcurrency]", group)
		}
		normalized := [3]int{limits[0], limits[1], 0}
		if len(limits) == 3 {
			normalized[2] = limits[2]
		}

		if normalized[0] < 0 || normalized[1] < 1 || normalized[2] < 0 {
			return nil, fmt.Errorf("group %s has invalid rate limit values: [%d, %d, %d]", group, normalized[0], normalized[1], normalized[2])
		}
		if normalized[0] > math.MaxInt32 || normalized[1] > math.MaxInt32 || normalized[2] > math.MaxInt32 {
			return nil, fmt.Errorf("group %s [%d, %d, %d] has max rate limits value 2147483647", group, normalized[0], normalized[1], normalized[2])
		}
		result[group] = normalized
	}
	return result, nil
}
