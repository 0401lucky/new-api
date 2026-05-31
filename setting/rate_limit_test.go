package setting

import "testing"

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
