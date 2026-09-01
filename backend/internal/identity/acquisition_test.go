package identity

import "testing"

func TestAcqTypeFilter(t *testing.T) {
	if got := AcqTypeFilter(""); got != nil {
		t.Fatalf("empty filter must be nil, got %#v", got)
	}
	kol := AcqTypeFilter("kol")
	if len(kol) != 2 || kol[0] != AcqKOL1 || kol[1] != AcqKOL2 {
		t.Fatalf("kol filter: %#v", kol)
	}
	agent := AcqTypeFilter("agent")
	if len(agent) != 1 || agent[0] != AcqAgent {
		t.Fatalf("agent filter: %#v", agent)
	}
}
