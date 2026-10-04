package graphql

import (
	"io"
	"net/http"
	"os"
	"regexp"
	"sync"
	"testing"
	"time"
)

const (
	DefaultClientVersion = "v1.0.5101"
	RESTClient           = "monarch-core-web-app-rest"
	GraphQLClient        = "monarch-core-web-app-graphql"
)

var (
	webAppURL          = "https://app.monarch.com/"
	appVersionPattern  = regexp.MustCompile(`__APP_VERSION__\s*=\s*"([^"]+)"`)
	clientVersionOnce  sync.Once
	clientVersionValue string
)

// ClientVersion reports the Monarch web app version; Monarch rejects logins from clients that omit or lag it.
func ClientVersion() string {
	if v := os.Getenv("MONARCH_CLIENT_VERSION"); v != "" {
		return v
	}
	clientVersionOnce.Do(func() {
		clientVersionValue = DefaultClientVersion
		if testing.Testing() {
			return
		}
		if v := fetchWebAppVersion(); v != "" {
			clientVersionValue = v
		}
	})
	return clientVersionValue
}

func fetchWebAppVersion() string {
	req, err := http.NewRequest("GET", webAppURL, nil)
	if err != nil {
		return ""
	}
	req.Header.Set("User-Agent", UserAgent())
	resp, err := (&http.Client{Timeout: 5 * time.Second}).Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ""
	}
	page, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return ""
	}
	if m := appVersionPattern.FindSubmatch(page); m != nil {
		return string(m[1])
	}
	return ""
}

func SetClientHeaders(h http.Header, client string) {
	h.Set("Client-Platform", "web")
	h.Set("User-Agent", UserAgent())
	h.Set("Monarch-Client", client)
	h.Set("Monarch-Client-Version", ClientVersion())
}
