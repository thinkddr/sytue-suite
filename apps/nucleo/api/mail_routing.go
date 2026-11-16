// SPDX-License-Identifier: Apache-2.0
package api

import (
	"context"
	"net/http"
	"net/url"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// MailRoutingSnapshot contains public delivery addresses, never message bodies
// or private mailbox content. The original service remains their authority.
type MailRoutingSnapshot struct {
	Primary string      `json:"primary"`
	Aliases []MailAlias `json:"aliases"`
}

func (m *Mail) routingRequest(scope storage.Scope, method, path string, body any) (Request, error) {
	if m == nil || m.c == nil || m.data != nil {
		return Request{}, ErrPrivateMailOperation
	}
	scoped, err := privateAppRequest(scope, "mail", path)
	if err != nil {
		return Request{}, err
	}
	request, err := m.peticion(method, path, body)
	if err != nil {
		return Request{}, err
	}
	if request.Header == nil {
		request.Header = scoped.Header
	} else {
		request.Header.Set(storage.ScopeHeader, scoped.Header.Get(storage.ScopeHeader))
	}
	return request, nil
}

func (m *Mail) RoutingMetadata(ctx context.Context, scope storage.Scope) (MailRoutingSnapshot, error) {
	request, err := m.routingRequest(scope, http.MethodGet, "/aliases", nil)
	if err != nil {
		return MailRoutingSnapshot{}, err
	}
	return JSON[MailRoutingSnapshot](ctx, m.c, request)
}

func (m *Mail) CreateRoutingAlias(ctx context.Context, scope storage.Scope, address string) (MailAlias, error) {
	request, err := m.routingRequest(scope, http.MethodPost, "/aliases", map[string]string{"address": address})
	if err != nil {
		return MailAlias{}, err
	}
	return JSON[MailAlias](ctx, m.c, request)
}

func (m *Mail) DeleteRoutingAlias(ctx context.Context, scope storage.Scope, id string) error {
	if !storage.MigrationEpoch(id) {
		return storage.ErrRecord
	}
	request, err := m.routingRequest(scope, http.MethodDelete, "/aliases/"+url.PathEscape(id), nil)
	if err != nil {
		return err
	}
	response, err := m.c.Do(ctx, request)
	if err != nil {
		return err
	}
	return response.Body.Close()
}

func (m *Mail) RoutingQuota(ctx context.Context, scope storage.Scope) (MailQuota, error) {
	request, err := m.routingRequest(scope, http.MethodGet, "/quota", nil)
	if err != nil {
		return MailQuota{}, err
	}
	return JSON[MailQuota](ctx, m.c, request)
}
