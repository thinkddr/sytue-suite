// SPDX-License-Identifier: Apache-2.0
package service

import (
	"context"
	"encoding/json"

	"git.sytue.eu/sytue/one/apps/nucleo/resource"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
	"git.sytue.eu/sytue/one/services/rooms/internal/store"
)

func (s *Service) resourceSessionScope(ctx context.Context, actor, product string) (storage.Scope, string, error) {
	client, err := resourceProductClient(ctx, actor, product)
	if err != nil {
		return storage.Scope{}, "", err
	}
	if err := s.matrixNativeActor(ctx, actor); err != nil {
		return storage.Scope{}, "", err
	}
	tenant := tenancy.MustFrom(ctx)
	if tenant.Personal() && (product == "erp" || product == "bases") {
		return storage.Scope{}, "", domain.ErrForbidden
	}
	return storage.Scope{Tenant: tenant.Slug, Subject: actor, Product: product}, client, nil
}

func (s *Service) MatrixResourceWebConsent(ctx context.Context, actor, product string) (store.MatrixResourceNativeGrant, error) {
	scope, client, err := s.resourceSessionScope(ctx, actor, product)
	if err != nil {
		return store.MatrixResourceNativeGrant{}, err
	}
	if client == "sytue-desktop" || client == "sytue-mobile" {
		return store.MatrixResourceNativeGrant{}, domain.ErrForbidden
	}
	return s.pstore.CreateMatrixResourceWebConsent(ctx, scope.Subject, scope.Product, client)
}

func (s *Service) MatrixResourceSessionChallenge(ctx context.Context, actor, product, grant string, recipient resource.SignedRecipient) (resource.SessionChallenge, error) {
	scope, client, err := s.resourceSessionScope(ctx, actor, product)
	if err != nil {
		return resource.SessionChallenge{}, err
	}
	return s.pstore.ResourceSessionChallenge(ctx, scope, grant, client, recipient)
}

func (s *Service) ConfirmMatrixResourceSession(ctx context.Context, actor, product string, proof resource.SignedSessionEnrollment) (resource.SessionRecord, error) {
	scope, client, err := s.resourceSessionScope(ctx, actor, product)
	if err != nil {
		return resource.SessionRecord{}, err
	}
	if _, err := s.pstore.ConfirmResourceSession(ctx, scope, client, proof); err != nil {
		return resource.SessionRecord{}, err
	}
	return s.MatrixResourceSession(ctx, actor, product, proof.Binding.DeviceID)
}

func (s *Service) MatrixResourceSession(ctx context.Context, actor, product, device string) (resource.SessionRecord, error) {
	scope, _, err := s.resourceSessionScope(ctx, actor, product)
	if err != nil {
		return resource.SessionRecord{}, err
	}
	provider, err := s.pstore.ResourceSessionProvider()
	if err != nil {
		return resource.SessionRecord{}, err
	}
	session, err := provider.Resolve(ctx, scope, device)
	if err != nil {
		return resource.SessionRecord{}, err
	}
	var out resource.SessionRecord
	if json.Unmarshal(session.PublicRecordBytes(), &out) != nil {
		return resource.SessionRecord{}, domain.ErrMatrixUnavailable
	}
	return out, nil
}

// MatrixResourceBootstrap returns routing only. It initializes no Rooms channel,
// draft, storage room, role, product consent or encryption device.
func (s *Service) MatrixResourceBootstrap(ctx context.Context, actor, product string) (MatrixBootstrap, error) {
	if _, _, err := s.resourceSessionScope(ctx, actor, product); err != nil {
		return MatrixBootstrap{}, err
	}
	return s.matrixBootstrapIdentity(ctx, actor)
}
