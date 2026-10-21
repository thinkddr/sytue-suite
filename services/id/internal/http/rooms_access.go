// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/services/id/internal/store"
)

// RoomsAccessGuard revokes independent Matrix sessions before ID confirms a
// suspension or an organization departure. ID never holds Matrix message keys
// or the Synapse administrator token.
func RoomsAccessGuard(baseURL, secret string) (store.ExternalAccessGuard, error) {
	if baseURL == "" && secret == "" {
		return store.ExternalAccessGuard{}, nil
	}
	u, err := url.Parse(baseURL)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" ||
		(u.Scheme != "https" && u.Scheme != "http") || secret == "" {
		return store.ExternalAccessGuard{}, errors.New("rooms lifecycle URL and secret must be configured together")
	}
	client := &http.Client{
		Timeout:       10 * time.Second,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse },
	}
	call := func(ctx context.Context, path string, body any) error {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(baseURL, "/")+path, bytes.NewReader(b))
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+secret)
		req.Header.Set("Content-Type", "application/json")
		res, err := client.Do(req)
		if err != nil {
			return errors.New("rooms encrypted access could not be revoked")
		}
		defer res.Body.Close()
		if res.StatusCode != http.StatusNoContent {
			return errors.New("rooms encrypted access could not be revoked")
		}
		return nil
	}
	return store.ExternalAccessGuard{
		Apply: func(ctx context.Context, change store.MatrixAccessChange) error {
			path := "/internal/matrix/account-access"
			if change.Tenant != "" {
				path = "/internal/matrix/tenant-access"
			}
			return call(ctx, path, change)
		},
	}, nil
}
