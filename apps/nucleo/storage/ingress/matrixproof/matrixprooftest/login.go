// SPDX-License-Identifier: Apache-2.0
package matrixprooftest

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/id"
)

type LoginProxyConfig struct {
	SynapseURL, UserID, Password, RoomID string
	Intercept                            func(http.ResponseWriter, *http.Request, *httputil.ReverseProxy) bool
}

// LoginProxy bridges only initial SSO/token exchange on a disposable Synapse.
// Device registration, Whoami, crypto, SSSS, backup and recovery remain real SDK
// operations. No Matrix client is imported, so SDK tests can use the same seam.
type LoginProxy struct {
	Server   *httptest.Server
	nonce    string
	consumed atomic.Bool
}

func NewLoginProxy(t testing.TB, cfg LoginProxyConfig) (*LoginProxy, error) {
	t.Helper()
	endpoint, err := DisposableHTTPOrigin(cfg.SynapseURL)
	if err != nil {
		return nil, err
	}
	if cfg.UserID == "" || cfg.Password == "" {
		return nil, errors.New("disposable login identity is required")
	}
	proxy := &LoginProxy{nonce: rand.Text()}
	forward := httputil.NewSingleHostReverseProxy(endpoint)
	proxy.Server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/_matrix/client/v3/login" {
			if r.Method == http.MethodGet {
				_ = json.NewEncoder(w).Encode(map[string]any{"flows": []map[string]string{{"type": "m.login.sso"}, {"type": "m.login.token"}}})
				return
			}
			if r.Method == http.MethodPost {
				proxy.login(w, r, cfg, endpoint.String())
				return
			}
		}
		if cfg.Intercept != nil && cfg.Intercept(w, r, forward) {
			return
		}
		forward.ServeHTTP(w, r)
	}))
	t.Cleanup(proxy.Server.Close)
	return proxy, nil
}

func (p *LoginProxy) login(w http.ResponseWriter, r *http.Request, cfg LoginProxyConfig, endpoint string) {
	var body struct {
		Type, Token string
		DeviceID    id.DeviceID `json:"device_id"`
	}
	if json.NewDecoder(r.Body).Decode(&body) != nil || body.Type != "m.login.token" || body.Token != p.nonce || body.DeviceID == "" || !p.consumed.CompareAndSwap(false, true) {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	login, err := mautrix.NewClient(endpoint, id.UserID(cfg.UserID), "")
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	registered, err := login.Login(r.Context(), &mautrix.ReqLogin{Type: mautrix.AuthTypePassword, Identifier: mautrix.UserIdentifier{Type: mautrix.IdentifierTypeUser, User: cfg.UserID}, Password: cfg.Password, DeviceID: body.DeviceID})
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	login.UserID, login.DeviceID, login.AccessToken = registered.UserID, registered.DeviceID, registered.AccessToken
	if cfg.RoomID != "" {
		if _, err := login.JoinRoom(r.Context(), cfg.RoomID, nil); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
	}
	_ = json.NewEncoder(w).Encode(registered) //nolint:gosec // G117: exact protocol response from disposable Synapse; this test seam never uses provider or production credentials.
}

// OpenURL returns a token only to the SDK's ephemeral loopback callback. Its
// random callback path remains generated and checked by auth.WaitLoopback.
func (p *LoginProxy) OpenURL(ctx context.Context, raw string) error {
	target, err := url.Parse(raw)
	if err != nil {
		return err
	}
	callback, err := url.Parse(target.Query().Get("redirectUrl"))
	if err != nil || callback.Scheme != "http" || callback.Hostname() != "127.0.0.1" || !strings.HasPrefix(callback.Path, "/matrix/callback/") || callback.User != nil {
		return errors.New("invalid fixture callback")
	}
	query := callback.Query()
	query.Set("loginToken", p.nonce)
	callback.RawQuery = query.Encode()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, callback.String(), http.NoBody)
	if err != nil {
		return err
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return err
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("fixture callback status %d", response.StatusCode)
	}
	return nil
}

// ArtificialNativeLoginToken is ONLY the disposable SSO seam's random nonce.
// It is consumed by the SDK login bridge, never a Matrix/provider credential.
func (p *LoginProxy) ArtificialNativeLoginToken() string { return p.nonce }
