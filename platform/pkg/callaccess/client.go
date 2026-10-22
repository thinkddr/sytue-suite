// SPDX-License-Identifier: Apache-2.0
// Package callaccess coordinates document ACL commits with encrypted call revocation.
package callaccess

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// ErrUnavailable means a revocation could not be acknowledged safely.
var ErrUnavailable = errors.New("callaccess: synchronous document call revocation is unavailable")

// Revoker closes document calls before an access change commits.
type Revoker interface {
	RevokeDocumentCalls(context.Context, string, string, int64) error
}

// Client authenticates private Rooms callbacks without following redirects.
type Client struct {
	base, secret string
	http         *http.Client
}

// New accepts a configured internal service origin and a dedicated credential.
func New(base, secret string) *Client {
	parsed, err := url.Parse(base)
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || strings.Trim(parsed.Path, "/") != "" || (parsed.Scheme != "http" && parsed.Scheme != "https") || secret == "" {
		return nil
	}
	baseTransport, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		return nil
	}
	transport := baseTransport.Clone()
	transport.Proxy = nil
	return &Client{base: strings.TrimRight(base, "/"), secret: secret, http: &http.Client{Transport: transport, Timeout: 45 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
}

// FenceReader also repairs the safe overblocking left by a rolled-back Docs
// transaction after Rooms already acknowledged its newer fence.
type FenceReader interface {
	MinimumDocumentVersion(context.Context, string) (int64, error)
}

// RevokeDocumentCalls advances a document fence and closes its active calls.
func (c *Client) RevokeDocumentCalls(ctx context.Context, id, tenant string, version int64) error {
	return c.request(ctx, http.MethodPost, "/internal/rooms/document-access", struct {
		ID      string `json:"document_id"`
		Tenant  string `json:"tenant"`
		Version int64  `json:"version"`
	}{id, tenant, version}, nil)
}

// MinimumDocumentVersion reads the minimum assertion version Rooms will accept.
func (c *Client) MinimumDocumentVersion(ctx context.Context, id string) (int64, error) {
	var out struct {
		Version int64 `json:"version"`
	}
	if err := c.request(ctx, http.MethodGet, "/internal/rooms/document-access/"+url.PathEscape(id), nil, &out); err != nil {
		return 0, err
	}
	if out.Version < 0 {
		return 0, ErrUnavailable
	}
	return out.Version, nil
}

func (c *Client) request(ctx context.Context, method, path string, body, out any) error {
	if c == nil {
		return ErrUnavailable
	}
	var payload io.Reader = http.NoBody
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return err
		}
		payload = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, payload)
	if err != nil {
		return ErrUnavailable
	}
	req.Header.Set("Authorization", "Bearer "+c.secret)
	req.Header.Set("Content-Type", "application/json")
	res, err := c.http.Do(req)
	if err != nil {
		return ErrUnavailable
	}
	defer res.Body.Close()
	if out == nil {
		_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 4096))
		if res.StatusCode != http.StatusNoContent {
			return ErrUnavailable
		}
		return nil
	}
	if res.StatusCode != http.StatusOK {
		return ErrUnavailable
	}
	if json.NewDecoder(io.LimitReader(res.Body, 8192)).Decode(out) != nil {
		return ErrUnavailable
	}
	return nil
}
