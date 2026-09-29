package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadIgnoresLegacyHubEndpoint(t *testing.T) {
	old := BuiltinHubEndpoint
	BuiltinHubEndpoint = "wss://pingo.test.invalid/ws"
	defer func() { BuiltinHubEndpoint = old }()

	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("hub:\n  endpoint: \"wss://hub.example.com/ws\"\nagents: []\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Hub.Endpoint != BuiltinHubEndpoint {
		t.Fatalf("expected builtin endpoint, got %q", cfg.Hub.Endpoint)
	}
}
