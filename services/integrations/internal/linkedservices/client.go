// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package linkedservices

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var (
	ErrInvalid      = errors.New("linkedservices: invalid request")
	ErrAuth         = errors.New("linkedservices: reconnect account")
	ErrNotFound     = errors.New("linkedservices: not found")
	ErrConflict     = errors.New("linkedservices: resource changed")
	ErrReadOnly     = errors.New("linkedservices: unsupported edit")
	ErrUnavailable  = errors.New("linkedservices: provider unavailable")
	ErrWriteUnknown = errors.New("linkedservices: write outcome unknown")
)

const (
	googleCalendar = "https://www.googleapis.com/calendar/v3"
	googlePeople   = "https://people.googleapis.com/v1"
	graph          = "https://graph.microsoft.com/v1.0/me"
	maxResponse    = 8 << 20
)

// Client receives the gateway's authenticated egress client. It cannot select
// another API origin or use the default transport. In particular DAV hrefs and
// provider pagination links cannot send the account token to another host.
type Client struct {
	provider string
	http     *http.Client
}

func NewClient(provider string, transport *http.Client) (*Client, error) {
	if transport == nil || transport.Transport == nil || (provider != "gmail" && provider != "microsoft" && provider != "yahoo") {
		return nil, ErrInvalid
	}
	guarded := *transport
	guarded.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if !allowedURL(provider, req.URL) || len(via) >= 5 {
			return ErrUnavailable
		}
		if transport.CheckRedirect != nil {
			return transport.CheckRedirect(req, via)
		}
		return nil
	}
	return &Client{provider: provider, http: &guarded}, nil
}

func allowedURL(provider string, u *url.URL) bool {
	if u == nil || u.Scheme != "https" || u.User != nil || u.Port() != "" || u.Fragment != "" {
		return false
	}
	switch provider {
	case "gmail":
		return u.Host == "www.googleapis.com" && strings.HasPrefix(u.Path, "/calendar/v3/") || u.Host == "people.googleapis.com" && strings.HasPrefix(u.Path, "/v1/")
	case "microsoft":
		return u.Host == "graph.microsoft.com" && strings.HasPrefix(u.Path, "/v1.0/me/")
	case "yahoo":
		return u.Host == "caldav.calendar.yahoo.com" || u.Host == "carddav.address.yahoo.com"
	default:
		return false
	}
}

func (c *Client) request(ctx context.Context, method, target, contentType string, body []byte, headers http.Header) ([]byte, http.Header, error) {
	u, err := url.Parse(target)
	if err != nil || !allowedURL(c.provider, u) || len(body) > maxResponse {
		return nil, nil, ErrInvalid
	}
	req, err := http.NewRequestWithContext(ctx, method, target, bytes.NewReader(body))
	if err != nil {
		return nil, nil, ErrInvalid
	}
	req.Header = headers.Clone()
	if req.Header == nil {
		req.Header = make(http.Header)
	}
	if req.Header.Get("Accept") == "" {
		req.Header.Set("Accept", "application/json")
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	mutating := method != http.MethodGet && method != "PROPFIND" && method != "REPORT"
	res, err := c.http.Do(req)
	if err != nil {
		if mutating {
			return nil, nil, ErrWriteUnknown
		}
		return nil, nil, ErrUnavailable
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, maxResponse+1))
	if err != nil || len(raw) > maxResponse {
		if mutating {
			return nil, nil, ErrWriteUnknown
		}
		return nil, nil, ErrUnavailable
	}
	if err := c.responseError(res.StatusCode, raw, mutating); err != nil {
		return nil, nil, err
	}
	return raw, res.Header, nil
}

func (c *Client) responseError(status int, raw []byte, mutating bool) error {
	switch status {
	case http.StatusUnauthorized, http.StatusForbidden:
		return ErrAuth
	case http.StatusNotFound:
		return ErrNotFound
	case http.StatusConflict, http.StatusPreconditionFailed:
		return ErrConflict
	}
	if status >= 200 && status < 300 {
		return nil
	}
	if status == http.StatusBadRequest && c.provider == "gmail" {
		var wire struct {
			Error struct {
				Status string `json:"status"`
			} `json:"error"`
		}
		if json.Unmarshal(raw, &wire) == nil && wire.Error.Status == "FAILED_PRECONDITION" {
			return ErrConflict
		}
	}
	if mutating && status >= 500 {
		return ErrWriteUnknown
	}
	return ErrUnavailable
}

func (c *Client) json(ctx context.Context, method, target string, body, out any, headers http.Header) error {
	var raw []byte
	var err error
	if body != nil {
		raw, err = json.Marshal(body)
		if err != nil {
			return ErrInvalid
		}
	}
	raw, _, err = c.request(ctx, method, target, "application/json", raw, headers)
	if err != nil {
		return err
	}
	if out != nil && json.Unmarshal(raw, out) != nil {
		if method != http.MethodGet {
			return ErrWriteUnknown
		}
		return ErrUnavailable
	}
	return nil
}

func validID(id string) bool {
	return id != "" && id != "." && id != ".." && len(id) <= 2048 && !strings.ContainsAny(id, "\r\n\x00")
}

func query(target string, values url.Values) string { return target + "?" + values.Encode() }

func (c *Client) nextURL(cursor, fallback, prefix string) (string, error) {
	if cursor == "" {
		return fallback, nil
	}
	if len(cursor) > 16384 {
		return "", ErrInvalid
	}
	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return "", ErrInvalid
	}
	u, err := url.Parse(string(raw))
	if err != nil || !allowedURL(c.provider, u) || !strings.HasPrefix(string(raw), prefix+"?") {
		return "", ErrInvalid
	}
	return string(raw), nil
}

func (c *Client) cursor(next, prefix string) (string, error) {
	if next == "" {
		return "", nil
	}
	value := base64.RawURLEncoding.EncodeToString([]byte(next))
	_, err := c.nextURL(value, "", prefix)
	return value, err
}

func conditional(etag string) (http.Header, error) {
	if etag == "" || len(etag) > 4096 || strings.ContainsAny(etag, "\r\n\x00") || etag == "*" {
		return nil, ErrConflict
	}
	return http.Header{"If-Match": {etag}}, nil
}

// Bounds prevent accidental unbounded provider scans. Callers can move through
// a calendar by requesting another visible window and its page cursor.
func eventWindow(from, to time.Time) bool {
	return !from.IsZero() && to.After(from) && to.Sub(from) <= 93*24*time.Hour
}

func safeProviderURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.User != nil {
		return ""
	}
	for _, domain := range []string{"google.com", "microsoft.com", "office.com", "office365.com", "live.com", "onenote.com", "yahoo.com"} {
		if u.Hostname() == domain || strings.HasSuffix(u.Hostname(), "."+domain) {
			return raw
		}
	}
	return ""
}
