package service

import (
	"strings"
	"testing"
)

func TestNewAgentID(t *testing.T) {
	first := newAgentID()
	second := newAgentID()
	if !strings.HasPrefix(first, "agt_") || len(first) != 40 || first == second {
		t.Fatalf("invalid generated ids: %q, %q", first, second)
	}
}
