// SPDX-License-Identifier: Apache-2.0
//go:build integration && cgo

// Package nativestoragetest opens actual maintained SDK devices against an
// explicitly disposable Synapse. It bridges only the initial SSO exchange.
package nativestoragetest

import (
	"context"
	"errors"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"testing"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/id"

	"git.sytue.eu/sytue/one/apps/nucleo/auth/authtest"
	mx "git.sytue.eu/sytue/one/apps/nucleo/matrix"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof/matrixprooftest"
)

type Config struct {
	Scope              storage.Scope
	Origin             string
	User, Controller   *mautrix.Client
	Room               id.RoomID
	Password, Recovery string
	Intercept          func(http.ResponseWriter, *http.Request, *httputil.ReverseProxy) bool
}

type Device struct {
	Client   *mx.Client
	Recovery string
	Room     id.RoomID
	Config   mx.Config
}

type canonicalTransport struct {
	origin, proxy *url.URL
	base          http.RoundTripper
}

func (t canonicalTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if request.URL.Scheme != t.origin.Scheme || request.URL.Host != t.origin.Host || request.URL.User != nil {
		return nil, errors.New("native SDK fixture refused another origin")
	}
	forward := request.Clone(request.Context())
	target := *request.URL
	target.Scheme, target.Host = t.proxy.Scheme, t.proxy.Host
	forward.URL = &target
	forward.Host = t.proxy.Host
	return t.base.RoundTrip(forward)
}

// Open never acknowledges recovery on behalf of the caller or constructs a
// verified status. Registration, cross-signing, backup and SSSS use the SDK.
func Open(ctx context.Context, t testing.TB, cfg Config) (*Device, error) {
	t.Helper()
	if !cfg.Scope.Valid() || cfg.User == nil || cfg.Controller == nil || cfg.User.UserID == "" || cfg.Controller.UserID == "" || cfg.Room == "" || cfg.Password == "" {
		return nil, errors.New("disposable native storage configuration is required")
	}
	bridge, err := matrixprooftest.NewLoginProxy(t, matrixprooftest.LoginProxyConfig{SynapseURL: cfg.Origin, UserID: cfg.User.UserID.String(), Password: cfg.Password, RoomID: cfg.Room.String(), Intercept: cfg.Intercept})
	if err != nil {
		return nil, err
	}
	// Keep the actual disposable server's host/port as its stable identity.
	// A per-device TLS bridge must neither split one server nor unify two.
	canonical, err := matrixprooftest.DisposableHTTPOrigin(cfg.Origin)
	if err != nil {
		return nil, err
	}
	canonical.Scheme = "https"
	proxyOrigin, err := url.Parse(bridge.Server.URL)
	if err != nil {
		return nil, err
	}
	transport := bridge.Server.Client()
	bound := *transport
	bound.Transport = canonicalTransport{origin: canonical, proxy: proxyOrigin, base: transport.Transport}
	version := storage.LegacyIdentity
	if strings.HasPrefix(cfg.User.UserID.String(), storage.IsolatedMatrixUserPrefix(cfg.Scope)) {
		version = storage.IsolatedIdentity
	}
	configuration := mx.Config{Bootstrap: mx.Bootstrap{Actor: cfg.Scope.Subject, Tenant: cfg.Scope.Tenant, HomeserverURL: canonical.String(), MatrixUserID: cfg.User.UserID.String(), ControllerUserID: cfg.Controller.UserID.String(), Encryption: string(id.AlgorithmMegolmV1), StorageProduct: cfg.Scope.Product, StorageIdentityVersion: version, StorageRoomID: cfg.Room.String(), StorageRoomIDs: []string{cfg.Room.String()}}, Subject: cfg.Scope.Subject, Tenant: cfg.Scope.Tenant, StoreDir: t.TempDir(), Secrets: &authtest.Secrets{}, HTTPClient: &bound}
	client, err := mx.Login(ctx, configuration, func(raw string) error { return bridge.OpenURL(ctx, raw) })
	if err != nil {
		return nil, err
	}
	t.Cleanup(func() { _ = client.Close() })
	recovery := cfg.Recovery
	if recovery == "" {
		recovery, err = client.BootstrapRecovery(ctx, func(challenge *mautrix.RespUserInteractive) any {
			return map[string]any{"type": "m.login.password", "identifier": map[string]string{"type": "m.id.user", "user": cfg.User.UserID.String()}, "password": cfg.Password, "session": challenge.Session}
		})
	} else {
		err = client.Recover(ctx, recovery)
	}
	if err != nil {
		return nil, err
	}
	go func() { _ = client.Sync(ctx, func(mx.DecryptedEvent, error) {}) }()
	if err := client.WaitReady(ctx); err != nil {
		return nil, err
	}
	return &Device{Client: client, Recovery: recovery, Room: cfg.Room, Config: configuration}, nil
}
