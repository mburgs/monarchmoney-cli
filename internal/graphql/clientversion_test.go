package graphql

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

func TestFetchWebAppVersion(t *testing.T) {
	original := webAppURL
	defer func() { webAppURL = original }()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`<script id="global-vars">window.__APP_VERSION__="v1.0.9999";</script>`))
	}))
	defer srv.Close()
	webAppURL = srv.URL

	if got := fetchWebAppVersion(); got != "v1.0.9999" {
		t.Fatalf("fetchWebAppVersion() = %q, want v1.0.9999", got)
	}
}

func TestSetClientHeaders(t *testing.T) {
	t.Setenv("MONARCH_CLIENT_VERSION", "v1.0.1234")
	t.Setenv("MONARCH_DEVICE_UUID", "11111111-2222-3333-4444-555555555555")
	h := http.Header{}
	SetClientHeaders(h, RESTClient)

	want := map[string]string{
		"Client-Platform":        "web",
		"Monarch-Client":         RESTClient,
		"Monarch-Client-Version": "v1.0.1234",
		"Device-UUID":            "11111111-2222-3333-4444-555555555555",
	}
	for k, v := range want {
		if got := h.Get(k); got != v {
			t.Errorf("%s = %q, want %q", k, got, v)
		}
	}
}

func TestLoadOrCreateDeviceUUIDPersists(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "device_uuid")
	first := loadOrCreateDeviceUUID(path)
	if first == "" {
		t.Fatal("loadOrCreateDeviceUUID() returned empty id")
	}
	if second := loadOrCreateDeviceUUID(path); second != first {
		t.Fatalf("second call = %q, want persisted %q", second, first)
	}
}
