package core

import "testing"

func TestVersion(t *testing.T) {
	if Version != "2.0.0" {
		t.Fatalf("Version = %q, quiero %q", Version, "2.0.0")
	}
}
