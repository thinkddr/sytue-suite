// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package matrix

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

type Device struct {
	ID, Name string
	Current  bool
}

func (c *Client) Devices(ctx context.Context) ([]Device, error) {
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()
	if c.closed.Load() {
		return nil, ErrClosed
	}
	response, err := c.cli.GetDevicesInfo(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Device, 0, len(response.Devices))
	for _, device := range response.Devices {
		out = append(out, Device{ID: device.DeviceID.String(), Name: device.DisplayName, Current: device.DeviceID == c.cli.DeviceID})
	}
	return out, nil
}

// UIASSOURL uses the homeserver's official browser UIA fallback. The UI never asks
// for a Matrix password or invents another authentication exchange.
func (c *Client) UIASSOURL(response *mautrix.RespUserInteractive) (string, error) {
	if response == nil || response.Session == "" || !response.HasSingleStageFlow(mautrix.AuthTypeSSO) {
		return "", ErrUnsupported
	}
	target, err := url.Parse(c.session.Homeserver)
	if err != nil || target.Scheme != "https" || target.Host == "" {
		return "", ErrIdentity
	}
	target.Path = "/_matrix/client/v3/auth/m.login.sso/fallback/web"
	target.RawQuery = url.Values{"session": {response.Session}}.Encode()
	return target.String(), nil
}

func (c *Client) RevokeDevice(ctx context.Context, device string, uia mautrix.UIACallback) error {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	if c.closed.Load() {
		return ErrClosed
	}
	if device == "" || id.DeviceID(device) == c.cli.DeviceID {
		return ErrIdentity
	}
	target := c.cli.BuildClientURL("v3", "devices", id.DeviceID(device))
	raw, err := c.cli.MakeFullRequest(ctx, mautrix.FullRequest{Method: http.MethodDelete, URL: target, RequestJSON: &mautrix.ReqDeleteDevice[any]{}})
	if err == nil {
		return nil
	}
	var httpErr mautrix.HTTPError
	if !errors.As(err, &httpErr) || !httpErr.IsStatus(http.StatusUnauthorized) || uia == nil {
		return err
	}
	var challenge mautrix.RespUserInteractive
	if json.Unmarshal(raw, &challenge) != nil {
		return err
	}
	authentication := uia(&challenge)
	if authentication == nil {
		return context.Canceled
	}
	return c.cli.DeleteDevice(ctx, id.DeviceID(device), &mautrix.ReqDeleteDevice[any]{Auth: authentication})
}

func (c *Client) StartVerification(ctx context.Context) (id.VerificationTransactionID, error) {
	return c.StartVerificationWith(ctx, "")
}

func (c *Client) StartVerificationWith(ctx context.Context, user string) (id.VerificationTransactionID, error) {
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()
	if c.closed.Load() {
		return "", ErrClosed
	}
	if c.verify == nil {
		return "", ErrUnsupported
	}
	if user == "" {
		user = c.cli.UserID.String()
	}
	if user == c.cfg.Bootstrap.ControllerUserID {
		return "", ErrIdentity
	}
	return c.verify.StartVerification(ctx, id.UserID(user))
}

func (c *Client) AcceptVerification(ctx context.Context, txn id.VerificationTransactionID) error {
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()
	if c.closed.Load() {
		return ErrClosed
	}
	if c.verify == nil {
		return ErrUnsupported
	}
	return c.verify.AcceptVerification(ctx, txn)
}

func (c *Client) StartSAS(ctx context.Context, txn id.VerificationTransactionID) error {
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()
	if c.closed.Load() {
		return ErrClosed
	}
	if c.verify == nil {
		return ErrUnsupported
	}
	return c.verify.StartSAS(ctx, txn)
}

func (c *Client) ConfirmSAS(ctx context.Context, txn id.VerificationTransactionID) error {
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()
	if c.closed.Load() {
		return ErrClosed
	}
	if c.verify == nil {
		return ErrUnsupported
	}
	return c.verify.ConfirmSAS(ctx, txn)
}

func (c *Client) CancelVerification(ctx context.Context, txn id.VerificationTransactionID, mismatch bool) error {
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()
	if c.closed.Load() {
		return ErrClosed
	}
	if c.verify == nil {
		return ErrUnsupported
	}
	code := event.VerificationCancelCodeUser
	if mismatch {
		code = event.VerificationCancelCodeSASMismatch
	}
	return c.verify.CancelVerification(ctx, txn, code, "")
}
