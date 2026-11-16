// SPDX-License-Identifier: Apache-2.0
package service

import (
	"context"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/drive/internal/domain"
	"git.sytue.eu/sytue/one/services/drive/internal/store"
)

func (s *Service) CipherUsage(ctx context.Context, product string) (storage.CipherUsage, error) {
	tenant, err := tenancy.From(ctx)
	if err != nil {
		return storage.CipherUsage{}, err
	}
	scope := storage.Scope{Tenant: tenant.Slug, Subject: store.OwnerFrom(ctx), Product: product}
	if !scope.Valid() {
		return storage.CipherUsage{}, domain.ErrPermission
	}
	reader, ok := s.cipher.(interface {
		CipherUsage(context.Context, string) (storage.CipherUsage, error)
	})
	if !ok {
		return storage.CipherUsage{}, domain.ErrNotFound
	}
	value, err := reader.CipherUsage(ctx, product)
	if err != nil {
		return storage.CipherUsage{}, err
	}
	if !value.Valid(scope) {
		return storage.CipherUsage{}, domain.ErrPermission
	}
	return value, nil
}
