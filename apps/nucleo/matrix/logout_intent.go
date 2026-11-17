// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package matrix

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"

	"maunium.net/go/mautrix"
)

var errNoLogoutIntent = errors.New("matrix: no pending logout")

type logoutIntent struct {
	Version int         `json:"version"`
	Account string      `json:"account"`
	Session credentials `json:"session"`
}

// Revocation survives ordinary removal of rooms-matrix-* encryption secrets.
// This OS-keychain entry holds only the retired token and its identity, no key
// capable of decrypting content. Every caller must hold the account device lease.
func (c *Client) logoutIntentKey() string { return "matrix-logout-" + c.account }

func (c *Client) readLogoutIntent() (*credentials, error) {
	raw, err := c.cfg.Secrets.Read(c.logoutIntentKey())
	if err != nil {
		return nil, fmt.Errorf("%w: logout intent", ErrSecureStore)
	}
	defer clear(raw)
	if len(raw) == 0 {
		return nil, errNoLogoutIntent
	}
	var intent logoutIntent
	if len(raw) > 64<<10 || json.Unmarshal(raw, &intent) != nil || intent.Version != 1 {
		return nil, fmt.Errorf("%w: logout intent", ErrSecureStore)
	}
	if intent.Account != c.account || !c.validSession(intent.Session) {
		return nil, ErrIdentity
	}
	return &intent.Session, nil
}

func (c *Client) saveLogoutIntent(session credentials) error {
	//nolint:gosec // G117: the envelope goes only to the app-private OS keychain.
	raw, err := json.Marshal(logoutIntent{Version: 1, Account: c.account, Session: session})
	if err != nil {
		return err
	}
	defer clear(raw)
	if err = c.cfg.Secrets.Write(c.logoutIntentKey(), raw); err != nil {
		return fmt.Errorf("%w: logout intent", ErrSecureStore)
	}
	return nil
}

// A crash after writing an intent must not resurrect the retired local device.
// A different saved session is never removed, even when its device ID is equal.
func (c *Client) resumeLogout(ctx context.Context) error {
	pending, err := c.readLogoutIntent()
	if errors.Is(err, errNoLogoutIntent) {
		return nil
	}
	if err != nil {
		return err
	}
	saved, err := c.readSession()
	if err != nil && !errors.Is(err, ErrLoginRequired) {
		return err
	}
	if err == nil && saved != *pending {
		return ErrIdentity
	}
	return c.finishLogout(ctx, *pending)
}

func (c *Client) finishLogout(ctx context.Context, session credentials) error {
	// ssssSecretKey se borra desde el 2026-11-01: el secreto del bóveda SSSS
	// (ssss_vault.go, fusión con sytue-native) no debe sobrevivir al logout.
	var leafErr error
	if c.cfg.ResourceProduct != "" {
		leafErr = c.cfg.Secrets.Delete(c.resourceLeafKey(session.DeviceID))
	}
	localErr := errors.Join(leafErr, c.cfg.Secrets.Delete(c.sessionKey()), c.cfg.Secrets.Delete(c.masterKey()), c.cfg.Secrets.Delete(c.backupSecretKey()), c.cfg.Secrets.Delete(c.storageAuthSecretKey()), c.cfg.Secrets.Delete(c.storagePQSecretKey()), c.cfg.Secrets.Delete(c.recoverySetupKey()), c.cfg.Secrets.Delete(c.ssssSecretKey()), os.RemoveAll(c.directory))
	remoteErr := c.logoutSession(ctx, session)
	if localErr != nil || remoteErr != nil {
		return errors.Join(localErr, remoteErr)
	}
	if err := c.cfg.Secrets.Delete(c.logoutIntentKey()); err != nil {
		return fmt.Errorf("%w: logout receipt", ErrSecureStore)
	}
	return nil
}

func (c *Client) logoutSession(ctx context.Context, session credentials) error {
	client, err := mautrix.NewClient(session.Homeserver, session.UserID, session.AccessToken)
	if err != nil {
		return err
	}
	client.DeviceID, client.Client, client.Log, client.UserAgent = session.DeviceID, c.cli.Client, c.cli.Log, c.cli.UserAgent
	_, err = client.Logout(ctx)
	var httpErr mautrix.HTTPError
	if errors.Is(err, mautrix.MUnknownToken) && errors.As(err, &httpErr) && httpErr.IsStatus(http.StatusUnauthorized) && httpErr.RespError != nil {
		// An expired/soft-logged-out session is not proof of device deletion.
		value, present := httpErr.RespError.ExtraData["soft_logout"]
		soft, valid := value.(bool)
		if !present || (valid && !soft) {
			return nil
		}
	}
	return err
}
