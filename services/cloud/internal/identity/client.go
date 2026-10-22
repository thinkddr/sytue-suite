// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package identity integra Sytue Cloud con la API administrativa de Sytue ID.
// Reenvía el access token del administrador: no introduce una credencial global
// ni amplía privilegios, y Sytue ID vuelve a aplicar tenant, rol y permisos.
package identity

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const maxErrorBody = 8 << 10

type Client struct {
	baseURL string
	http    *http.Client
}

type Member struct {
	Sub  string `json:"user_id"`
	Role string `json:"role"`
}

type APIError struct {
	Status int
	Body   string
}

func (e *APIError) Error() string { return fmt.Sprintf("identity: HTTP %d", e.Status) }

func NewClient(baseURL string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		http:    &http.Client{Timeout: 10 * time.Second},
	}
}

func (c *Client) ResolveMember(ctx context.Context, bearer, email string) (Member, bool, error) {
	var out Member
	err := c.doJSON(ctx, bearer, "/api/v1/admin/members:resolve", map[string]string{"email": email}, &out)
	var apiErr *APIError
	if errors.As(err, &apiErr) && apiErr.Status == http.StatusNotFound {
		return Member{}, false, nil
	}
	return out, err == nil, err
}

func (c *Client) Invite(ctx context.Context, bearer, email, role string) (time.Time, error) {
	var out struct {
		ExpiresAt time.Time `json:"expires_at"`
	}
	err := c.doJSON(ctx, bearer, "/api/v1/admin/invitations", map[string]string{
		"email": email, "role": role,
	}, &out)
	return out.ExpiresAt, err
}

func (c *Client) RevokeInvitation(ctx context.Context, bearer, email string) error {
	return c.doJSON(ctx, bearer, "/api/v1/admin/invitations:revoke",
		map[string]string{"email": email}, nil)
}

func (c *Client) doJSON(ctx context.Context, bearer, path string, body, out any) error {
	payload, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("identity: codificar petición: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("identity: crear petición: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(bearer))
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("identity: petición: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorBody))
		return &APIError{Status: resp.StatusCode, Body: string(raw)}
	}
	if out == nil || resp.StatusCode == http.StatusNoContent {
		return nil
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(out); err != nil {
		return fmt.Errorf("identity: decodificar respuesta: %w", err)
	}
	return nil
}
