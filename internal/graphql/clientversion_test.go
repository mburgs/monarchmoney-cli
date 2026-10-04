package graphql

import (
	"net/http"
	"net/http/httptest"
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
	h := http.Header{}
	SetClientHeaders(h, RESTClient)

	want := map[string]string{
		"Client-Platform":        "web",
		"Monarch-Client":         RESTClient,
		"Monarch-Client-Version": "v1.0.1234",
	}
	for k, v := range want {
		if got := h.Get(k); got != v {
			t.Errorf("%s = %q, want %q", k, got, v)
		}
	}
}
