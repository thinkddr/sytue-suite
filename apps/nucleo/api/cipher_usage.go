// SPDX-License-Identifier: Apache-2.0
package api

import (
	"context"
	"net/http"
	"net/url"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

func (d *Drive) CipherUsage(ctx context.Context, scope storage.Scope) (storage.CipherUsage, error) {
	encoded, err := storage.EncodeScopeHeader(scope)
	if err != nil {
		return storage.CipherUsage{}, err
	}
	value, err := JSON[storage.CipherUsage](ctx, d.c, Request{Path: "/cipher/usage/" + url.PathEscape(scope.Product), Header: http.Header{storage.ScopeHeader: {encoded}}})
	if err != nil {
		return storage.CipherUsage{}, err
	}
	if !value.Valid(scope) {
		return storage.CipherUsage{}, storage.ErrRecord
	}
	return value, nil
}
