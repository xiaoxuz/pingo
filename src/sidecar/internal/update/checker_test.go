package update

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCheckFindsNewerVersion(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"version":"0.2.0","channel":"stable","package_url":"https://updates.test.invalid/pingo.tar.gz","sha256":"` + stringOf64Zeros + `","notes":"Release"}`))
	}))
	defer server.Close()
	checker := NewChecker("0.1.0", "stable", server.URL, nil)
	if err := checker.Check(context.Background()); err != nil {
		t.Fatal(err)
	}
	state := checker.State()
	if !state.Available || state.LatestVersion != "0.2.0" || state.PackageURL == "" {
		t.Fatalf("state: %+v", state)
	}
}

func TestCheckAcceptsHTTPPackageURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"version":"0.2.0","channel":"stable","package_url":"http://10.0.0.8:18080/pingo.tar.gz","sha256":"` + stringOf64Zeros + `"}`))
	}))
	defer server.Close()
	checker := NewChecker("0.1.0", "stable", server.URL, nil)
	if err := checker.Check(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !checker.State().Available {
		t.Fatalf("state: %+v", checker.State())
	}
}

func TestVersionComparison(t *testing.T) {
	if !newer("0.10.0", "0.9.9") || newer("0.9.0", "0.10.0") || newer("0.1.0", "0.1.0") {
		t.Fatal("semantic version comparison failed")
	}
}

const stringOf64Zeros = "0000000000000000000000000000000000000000000000000000000000000000"
