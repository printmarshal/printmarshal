package reports

import (
	"context"
	"strings"
	"testing"
)

// The security area is implemented, so the unavailable report must not claim
// otherwise, and it must say where assessments can be found today.
func TestComplianceReasonIsTrue(t *testing.T) {
	st := openStore(t)
	c, err := New(st).Compliance(context.Background(), Params{})
	if err != nil {
		t.Fatal(err)
	}
	if c.Available {
		t.Fatalf("compliance report = %+v, want unavailable", c)
	}
	reason := strings.ToLower(c.Reason)
	if strings.Contains(reason, "has not landed") {
		t.Errorf("reason still says the security feature has not landed: %q", c.Reason)
	}
	if !strings.Contains(reason, "under security") {
		t.Errorf("reason does not point to Security: %q", c.Reason)
	}
	if !strings.Contains(reason, "assessment history") {
		t.Errorf("reason does not say what is missing: %q", c.Reason)
	}
}
