package discovery

import "testing"

func TestExpandTargets(t *testing.T) {
	addrs, skipped, err := ExpandTargets([]string{"10.0.0.0/30", "10.0.1.5", "10.0.2.1-3", "10.0.3.250-10.0.3.252", "10.0.0.1"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"10.0.0.1", "10.0.0.2", "10.0.1.5", "10.0.2.1", "10.0.2.2", "10.0.2.3", "10.0.3.250", "10.0.3.251", "10.0.3.252"}
	if len(addrs) != len(want) || skipped != 0 {
		t.Fatalf("addrs = %v skipped=%d", addrs, skipped)
	}
	for i := range want {
		if addrs[i] != want[i] {
			t.Errorf("addrs[%d] = %s want %s", i, addrs[i], want[i])
		}
	}
	addrs, skipped, err = ExpandTargets([]string{"192.168.0.0/24"}, 10)
	if err != nil || len(addrs) != 10 || skipped != 244 {
		t.Errorf("cap: addrs=%d skipped=%d err=%v", len(addrs), skipped, err)
	}
	if _, _, err := ExpandTargets([]string{"10.0.0.5-1"}, 0); err == nil {
		t.Error("descending range should fail")
	}
}

func TestMergeCandidates(t *testing.T) {
	out := MergeCandidates(
		[]Candidate{{Address: "10.0.0.5", Methods: []string{"mdns"}, TXT: map[string]string{"ty": "X"}}},
		[]Candidate{{Address: "10.0.0.5", Methods: []string{"scan"}}, {Address: "10.0.0.6", Methods: []string{"scan"}}},
	)
	if len(out) != 2 || len(out[0].Methods) != 2 || out[0].TXT["ty"] != "X" {
		t.Errorf("merge = %+v", out)
	}
}
