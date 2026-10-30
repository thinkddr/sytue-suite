// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package matrix

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"net/url"
	"time"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/id"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/auth"
)

// Login opens the system browser using Matrix SSO and exchanges its one-use login
// token directly with the homeserver. The random callback path binds the local
// return; Whoami pins both person and device before any credentials are persisted.
func Login(ctx context.Context, cfg Config, openBrowser func(string) error) (*Client, error) {
	c, err := newClient(cfg)
	if err != nil {
		return nil, err
	}
	if openBrowser == nil {
		return nil, errors.New("matrix: browser is unavailable")
	}
	flow, err := c.cli.GetLoginFlows(ctx)
	if err != nil {
		return nil, err
	}
	if !flow.HasFlow(mautrix.AuthTypeSSO) || !flow.HasFlow(mautrix.AuthTypeToken) {
		return nil, errors.New("matrix: SSO token login is unavailable")
	}
	waitCtx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	nonce := rand.Text()
	query, err := auth.WaitLoopback(waitCtx, auth.LoopbackCallback{
		Address: "127.0.0.1:0", Path: "/matrix/callback/" + nonce,
		Open: func(redirect string) error {
			parts := []any{"v3", "login", "sso", "redirect"}
			if cfg.Bootstrap.SSOProviderID != "" {
				parts = append(parts, cfg.Bootstrap.SSOProviderID)
			}
			target, parseErr := url.Parse(c.cli.BuildClientURL(parts...))
			if parseErr != nil {
				return parseErr
			}
			values := target.Query()
			values.Set("redirectUrl", redirect)
			target.RawQuery = values.Encode()
			return openBrowser(target.String())
		},
		Validate: func(q url.Values) error {
			if len(q["loginToken"]) != 1 || q.Get("loginToken") == "" || len(q.Get("loginToken")) > 4096 {
				return ErrIdentity
			}
			return nil
		},
		Success: "Sytue: return to the app to finish signing in.",
		Failure: "Sytue: sign-in could not be completed. Return to the app and try again.",
	})
	if err != nil {
		return nil, err
	}
	return c.finishLogin(ctx, query.Get("loginToken"))
}

// LoginNative wraps Matrix SSO in the first-party PKCE handoff. No bearer Matrix
// credential crosses a custom-scheme URL on mobile. The browser callback broker
// also binds the ticket to this Sytue actor/tenant before releasing a one-use token.
func LoginNative(ctx context.Context, cfg Config, rooms *api.Rooms, await func(context.Context, api.MatrixNativeAuthorization) (api.MatrixNativeCallback, error)) (*Client, error) {
	c, err := newClient(cfg)
	if err != nil {
		return nil, err
	}
	if rooms == nil || await == nil {
		return nil, api.ErrMatrixSSO
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	product := cfg.Bootstrap.StorageProduct
	if product == "" {
		product = "rooms"
	}
	authorization, err := rooms.MatrixNativeStartFor(ctx, product)
	if err != nil {
		return nil, err
	}
	if !authorization.ForProduct(cfg.SytueApex, product) {
		return nil, api.ErrMatrixSSO
	}
	callback, err := await(ctx, authorization)
	if err != nil {
		return nil, err
	}
	token, err := rooms.MatrixNativeFinish(ctx, authorization, callback)
	if err != nil {
		return nil, err
	}
	return c.finishLogin(ctx, token)
}

func (c *Client) finishLogin(ctx context.Context, loginToken string) (*Client, error) {
	cfg := c.cfg
	var err error
	// Preserve an existing device only when its identity comes from this account's
	// keychain, so reauthentication does not replace its encryption keys.
	device := id.DeviceID("SYTUE_" + rand.Text())
	newDevice := true
	if saved, readErr := c.readSession(); readErr == nil {
		device = saved.DeviceID
		newDevice = false
	} else if !errors.Is(readErr, ErrLoginRequired) {
		return nil, readErr
	}
	response, err := c.cli.Login(ctx, &mautrix.ReqLogin{Type: mautrix.AuthTypeToken, Token: loginToken, DeviceID: device, InitialDeviceDisplayName: cfg.DeviceName, StoreCredentials: true})
	if err != nil {
		return nil, err
	}
	if response.UserID != id.UserID(cfg.Bootstrap.MatrixUserID) || response.DeviceID != device {
		_, _ = c.cli.Logout(ctx)
		return nil, ErrIdentity
	}
	who, err := c.cli.Whoami(ctx)
	if err != nil {
		return nil, err
	}
	if who.UserID != response.UserID || who.DeviceID != device {
		_, _ = c.cli.Logout(ctx)
		return nil, ErrIdentity
	}
	c.session = credentials{Homeserver: c.cfg.Bootstrap.HomeserverURL, UserID: who.UserID, DeviceID: device, AccessToken: response.AccessToken, Subject: cfg.Subject, Tenant: cfg.Tenant}
	if err = c.loadMaster(newDevice); err != nil {
		_, _ = c.cli.Logout(ctx)
		return nil, err
	}
	encoded, err := json.Marshal(c.session) //nolint:gosec // G117: this envelope is written only to the OS keychain, never to logs, files or the server.
	if err != nil {
		return nil, err
	}
	defer clear(encoded)
	if err = c.cfg.Secrets.Write(c.sessionKey(), encoded); err != nil {
		_, _ = c.cli.Logout(ctx)
		return nil, ErrSecureStore
	}
	if err = c.initialize(ctx, newDevice); err != nil {
		_ = c.Close()
		return nil, err
	}
	return c, nil
}
