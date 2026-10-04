package auth

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/pquerna/otp/totp"

	"github.com/thedavidweng/monarchmoney-cli/internal/errors"
	"github.com/thedavidweng/monarchmoney-cli/internal/graphql"
)

var (
	loginEndpoint        = "https://api.monarch.com/auth/login/"
	maxLoginResponseSize = int64(1 << 20)
	newLoginHTTPClient   = func() *http.Client {
		return &http.Client{
			Timeout:       10 * time.Second,
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse },
		}
	}
)

type loginRequest struct {
	Username      string `json:"username"`
	Password      string `json:"password"`
	SupportsMFA   bool   `json:"supports_mfa"`
	TrustedDevice bool   `json:"trusted_device"`
	TOTP          string `json:"totp,omitempty"`
}

type loginResponse struct {
	Token string `json:"token"`
}

// Authenticate logs in through Monarch's REST endpoint, not GraphQL.
func Authenticate(email, password, mfaCode, mfaSecret string) (*Session, error) {
	if mfaSecret != "" {
		code, err := totp.GenerateCode(mfaSecret, time.Now())
		if err != nil {
			return nil, errors.New(errors.InternalError, "failed to generate MFA code", errors.CatInternal, false, err)
		}
		mfaCode = code
	}

	reqBody := loginRequest{
		Username:      email,
		Password:      password,
		SupportsMFA:   true,
		TrustedDevice: true,
		TOTP:          mfaCode,
	}
	body, _ := json.Marshal(reqBody)

	req, err := http.NewRequest("POST", loginEndpoint, bytes.NewBuffer(body))
	if err != nil {
		return nil, errors.New(errors.InternalError, "failed to create login request", errors.CatInternal, false, err)
	}
	req.Header.Set("Content-Type", "application/json")
	graphql.SetClientHeaders(req.Header, graphql.RESTClient)

	client := newLoginHTTPClient()
	resp, err := client.Do(req)
	if err != nil {
		return nil, errors.New(errors.NetworkUnreachable, "failed to reach Monarch API", errors.CatNetwork, true, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 403 || resp.StatusCode == 401 {
		return nil, classifyLoginRejection(resp.StatusCode, decodeLoginError(resp.Body), mfaCode != "")
	}

	if resp.StatusCode != 200 {
		if apiErr := decodeLoginError(resp.Body); apiErr.Detail != "" {
			return nil, errors.New(errors.APIError, apiErr.Detail, errors.CatAPI, false, nil)
		}
		return nil, errors.New(errors.APIError, fmt.Sprintf("API returned status %d", resp.StatusCode), errors.CatAPI, false, nil)
	}

	var loginResp loginResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxLoginResponseSize)).Decode(&loginResp); err != nil {
		return nil, errors.New(errors.APISchemaChanged, "failed to parse login response", errors.CatAPI, false, err)
	}

	return &Session{
		Email:     email,
		Token:     loginResp.Token,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}, nil
}

type loginError struct {
	Detail    string `json:"detail"`
	ErrorCode string `json:"error_code"`
}

func decodeLoginError(body io.Reader) loginError {
	var apiErr loginError
	_ = json.NewDecoder(io.LimitReader(body, maxLoginResponseSize)).Decode(&apiErr)
	return apiErr
}

func (e loginError) describe() string {
	switch {
	case e.Detail != "" && e.ErrorCode != "":
		return fmt.Sprintf("%s (%s)", e.Detail, e.ErrorCode)
	case e.Detail != "":
		return e.Detail
	default:
		return e.ErrorCode
	}
}

func (e loginError) mentions(words ...string) bool {
	text := strings.ToLower(e.Detail + " " + e.ErrorCode)
	for _, w := range words {
		if strings.Contains(text, w) {
			return true
		}
	}
	return false
}

// Monarch answers 401/403 for MFA, email verification, and CAPTCHA challenges alike; only an
// explicit MFA signal (or an empty body, the historical MFA response) should trigger an MFA prompt.
func classifyLoginRejection(status int, apiErr loginError, mfaAttempted bool) error {
	reason := apiErr.describe()
	withReason := func(msg string) string {
		if reason == "" {
			return msg
		}
		return msg + ": " + reason
	}

	switch {
	case apiErr.mentions("captcha"):
		return errors.New(errors.AuthRequired, withReason("Monarch blocked programmatic login with a CAPTCHA challenge"), errors.CatAuth, false, nil)
	case apiErr.mentions("email", "verification code", "verify your device", "otp") && !apiErr.mentions("totp"):
		return errors.New(errors.AuthRequired, withReason("Monarch requires email verification, which this CLI does not support; enable authenticator-app MFA in Monarch settings and use --mfa-secret"), errors.CatAuth, false, nil)
	case reason == "" || apiErr.mentions("mfa", "multi-factor", "multi factor", "two-factor", "2fa", "totp", "authenticator"):
		if mfaAttempted {
			return errors.New(errors.AuthMFAInvalid, withReason("invalid credentials or MFA code"), errors.CatAuth, false, nil)
		}
		return errors.New(errors.AuthMFARequired, withReason("MFA code required"), errors.CatAuth, false, nil)
	default:
		return errors.New(errors.AuthRequired, withReason(fmt.Sprintf("Monarch rejected the login (HTTP %d)", status)), errors.CatAuth, false, nil)
	}
}
