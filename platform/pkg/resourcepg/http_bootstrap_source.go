// SPDX-License-Identifier: Apache-2.0
package resourcepg

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

	"git.sytue.eu/sytue/one/apps/nucleo/resource"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
)

var ErrOriginSourceUnavailable = errors.New("resourcepg: original resource source unavailable")

// OriginSourceEndpoint is created by service composition from a fixed backend
// catalog/configuration. No user request selects its URL, credential or service.
// The same bounded JSON transport serves public source and session metadata.
type OriginSourceEndpoint struct {
	serviceID string
	base      string
	secret    string
	client    *http.Client
}

func NewOriginSourceEndpoint(serviceID, base, secret string, transport http.RoundTripper) (*OriginSourceEndpoint, error) {
	endpoint, err := url.Parse(base)
	if err != nil || resource.OriginServiceID(serviceID) != serviceID || serviceID == "" || !httpx.ValidMetadataSecret(secret) || endpoint == nil || endpoint.Host == "" || endpoint.User != nil || (endpoint.Scheme != "http" && endpoint.Scheme != "https") || (endpoint.Path != "" && endpoint.Path != "/") || endpoint.RawQuery != "" || endpoint.Fragment != "" {
		return nil, resource.ErrInvalid
	}
	if transport == nil {
		transport = http.DefaultTransport
	}
	return &OriginSourceEndpoint{serviceID: serviceID, base: strings.TrimRight(base, "/"), secret: secret, client: &http.Client{Transport: transport, Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}

func (e *OriginSourceEndpoint) post(ctx context.Context, path string, body, result any, maxBytes int64) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if e == nil || e.client == nil {
		return ErrOriginSourceUnavailable
	}
	raw, err := httpx.EncodeJSONLimit(body, 32768)
	if err != nil {
		return resource.ErrInvalid
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, e.base+path, bytes.NewReader(raw))
	if err != nil {
		return ErrOriginSourceUnavailable
	}
	request.Header.Set("Authorization", "Bearer "+e.secret)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	response, err := e.client.Do(request)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return ErrOriginSourceUnavailable
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusForbidden || response.StatusCode == http.StatusNotFound {
		return resource.ErrRevoked
	}
	if response.StatusCode != http.StatusOK {
		return ErrOriginSourceUnavailable
	}
	raw, err = io.ReadAll(io.LimitReader(response.Body, maxBytes+1))
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return ErrOriginSourceUnavailable
	}
	if int64(len(raw)) > maxBytes {
		return ErrOriginSourceUnavailable
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(result) != nil {
		return ErrOriginSourceUnavailable
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		return ErrOriginSourceUnavailable
	}
	return ctx.Err()
}

type BootstrapHTTPSource struct {
	endpoint *OriginSourceEndpoint
	query    resource.VerifiedBootstrapSourceQuery
}

var _ resource.OriginalBootstrapSource = (*BootstrapHTTPSource)(nil)

func (e *OriginSourceEndpoint) BootstrapSource(query resource.VerifiedBootstrapSourceQuery) (*BootstrapHTTPSource, error) {
	q := query.Value()
	if e == nil || !q.Valid() || resource.OriginServiceID(q.Resource.Product) != e.serviceID || len(query.PublicQueryBytes()) == 0 {
		return nil, resource.ErrInvalid
	}
	return &BootstrapHTTPSource{endpoint: e, query: query}, nil
}

func (s *BootstrapHTTPSource) BootstrapSource(ctx context.Context, key resource.Resource) (resource.BootstrapSourceSnapshot, error) {
	var out resource.BootstrapSourceSnapshot
	if s == nil || s.endpoint == nil || key != s.query.Value().Resource {
		return out, resource.ErrInvalid
	}
	if err := s.query.Revalidate(ctx); err != nil {
		return out, err
	}
	// Public query bytes were canonicalized only after signature/current-session
	// verification. Transporting them does not turn JSON into a Verified value.
	nonce, err := httpx.FreshMetadataNonce()
	if err != nil {
		return resource.BootstrapSourceSnapshot{}, err
	}
	var signed resource.SignedBootstrapSourceQuery
	if json.Unmarshal(s.query.PublicQueryBytes(), &signed) != nil {
		return out, resource.ErrInvalid
	}
	request := httpx.ResourceBootstrapSourceRequest{Version: 1, Nonce: nonce, Query: signed}
	var response httpx.ResourceBootstrapSourceResponse
	if err := s.endpoint.post(ctx, httpx.ResourceBootstrapSourcePath, request, &response, 32768); err != nil {
		return resource.BootstrapSourceSnapshot{}, err
	}
	mac := response.MAC
	response.MAC = ""
	if response.Version != 1 || response.Nonce != nonce || httpx.VerifyMetadataMAC(s.endpoint.secret, httpx.ResourceBootstrapSourceProofDomain, request, response, mac) != nil {
		return out, resource.ErrRevoked
	}
	out = response.Snapshot
	if !out.Valid() || out.Resource != key || out.OwnerScope != s.query.Value().OwnerScope {
		return resource.BootstrapSourceSnapshot{}, ErrOriginSourceUnavailable
	}
	if err := s.query.Revalidate(ctx); err != nil {
		return resource.BootstrapSourceSnapshot{}, err
	}
	return out, nil
}

func (s *BootstrapHTTPSource) CheckBootstrapSource(ctx context.Context, expected resource.BootstrapSourceSnapshot) error {
	actual, err := s.BootstrapSource(ctx, expected.Resource)
	if err != nil {
		return err
	}
	if actual != expected {
		return resource.ErrRevoked
	}
	return ctx.Err()
}
