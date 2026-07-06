package setting

import "testing"

func TestParseModelRequestRateLimitGroup(t *testing.T) {
	tests := []struct {
		name     string
		raw      string
		group    string
		expected [3]int
	}{
		{
			name:     "legacy two value entries default concurrency to zero",
			raw:      `{"default":[30,20]}`,
			group:    "default",
			expected: [3]int{30, 20, 0},
		},
		{
			name:     "three value entries include concurrency",
			raw:      `{"vip":[60,50,5]}`,
			group:    "vip",
			expected: [3]int{60, 50, 5},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			limits, err := ParseModelRequestRateLimitGroup(test.raw)
			if err != nil {
				t.Fatalf("ParseModelRequestRateLimitGroup error: %v", err)
			}
			if limits[test.group] != test.expected {
				t.Fatalf("expected %v, got %v", test.expected, limits[test.group])
			}
		})
	}
}

func TestParseModelRequestRateLimitGroupRejectsInvalidValues(t *testing.T) {
	tests := []string{
		`{"default":[30]}`,
		`{"default":[30,20,3,1]}`,
		`{"default":[30,0,3]}`,
		`{"default":[30,20,-1]}`,
		`{"default":[2147483648,20,1]}`,
	}

	for _, raw := range tests {
		t.Run(raw, func(t *testing.T) {
			if _, err := ParseModelRequestRateLimitGroup(raw); err == nil {
				t.Fatal("expected invalid group rate limit to return error")
			}
		})
	}
}

func TestParseModelRequestRateLimitExemptUserIDs(t *testing.T) {
	ids, err := ParseModelRequestRateLimitExemptUserIDs("1,2\n3 4\t5")
	if err != nil {
		t.Fatalf("ParseModelRequestRateLimitExemptUserIDs error: %v", err)
	}
	for _, id := range []int{1, 2, 3, 4, 5} {
		if _, ok := ids[id]; !ok {
			t.Fatalf("expected id %d to be parsed", id)
		}
	}

	if _, err := ParseModelRequestRateLimitExemptUserIDs("1,abc"); err == nil {
		t.Fatal("expected invalid id to return error")
	}
	if _, err := ParseModelRequestRateLimitExemptUserIDs("0"); err == nil {
		t.Fatal("expected non-positive id to return error")
	}
}

func TestIsModelRequestRateLimitExemptUser(t *testing.T) {
	oldIDs := ModelRequestRateLimitExemptUserIDs
	defer func() {
		ModelRequestRateLimitMutex.Lock()
		ModelRequestRateLimitExemptUserIDs = oldIDs
		ModelRequestRateLimitMutex.Unlock()
	}()

	if err := UpdateModelRequestRateLimitExemptUserIDs("10,20"); err != nil {
		t.Fatalf("UpdateModelRequestRateLimitExemptUserIDs error: %v", err)
	}
	if !IsModelRequestRateLimitExemptUser(10) {
		t.Fatal("expected user 10 to be exempt")
	}
	if IsModelRequestRateLimitExemptUser(11) {
		t.Fatal("expected user 11 not to be exempt")
	}
}
