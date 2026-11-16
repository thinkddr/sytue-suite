// SPDX-License-Identifier: Apache-2.0
package service

import (
	"context"
	"slices"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/platform/pkg/authn"
	suiteconfig "git.sytue.eu/sytue/one/platform/pkg/config"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
	"git.sytue.eu/sytue/one/services/rooms/internal/store"
)

func resourceNativeAudience(ctx context.Context, actor string) (string, error) {
	client, ok := authn.FirstPartyNativeClient(ctx, actor)
	if !ok {
		return "", domain.ErrForbidden
	}
	return client, nil
}

func (s *Service) StartMatrixResourceNative(ctx context.Context, actor, challenge, method, product string) (MatrixNativeStart, error) {
	client, err := resourceNativeAudience(ctx, actor)
	if err != nil {
		return MatrixNativeStart{}, err
	}
	if method != "S256" || !matrixOpaque(challenge) || !storage.Product(product) {
		return MatrixNativeStart{}, domain.ErrInvalidInput
	}
	if err := s.matrixNativeActor(ctx, actor); err != nil {
		return MatrixNativeStart{}, err
	}
	tenant := tenancy.MustFrom(ctx)
	if tenant.Personal() && (product == "erp" || product == "bases") {
		return MatrixNativeStart{}, domain.ErrForbidden
	}
	if _, err := s.matrix.EnsureUser(ctx, actor); err != nil {
		return MatrixNativeStart{}, err
	}
	state, err := s.pstore.CreateMatrixResourceNativeConsent(ctx, actor, challenge, product, client)
	if err != nil {
		return MatrixNativeStart{}, err
	}
	target, err := s.matrixNativePublicURL("start", state)
	if err != nil {
		return MatrixNativeStart{}, err
	}
	return MatrixNativeStart{Purpose: "resources:" + product, Product: product, IdentityVersion: storage.LegacyIdentity, Tenant: tenant.Slug, State: state, AuthorizeURL: target}, nil
}

// Approval is only through the authenticated Rooms browser consent surface.
// Native shared-client tokens cannot approve their own purpose invisibly.
func (s *Service) ApproveMatrixResourceNative(ctx context.Context, actor, state string) error {
	claims, ok := authn.FromContext(ctx)
	if !ok || claims.Subject != actor || claims.Delegado || !slices.Contains(claims.Audience, suiteconfig.FirstPartyProductClientID("rooms")) || slices.Contains(claims.Audience, "sytue-desktop") || slices.Contains(claims.Audience, "sytue-mobile") {
		return domain.ErrForbidden
	}
	if !matrixOpaque(state) {
		return domain.ErrInvalidInput
	}
	selected, err := s.matrixNativeConsentContext(ctx, actor, state)
	if err != nil {
		return err
	}
	return s.pstore.ApproveMatrixResourceNativeConsent(selected, actor, state)
}

func (s *Service) FinishMatrixResourceNative(ctx context.Context, actor, state, ticket, verifier string) (store.MatrixResourceNativeGrant, error) {
	var empty store.MatrixResourceNativeGrant
	client, err := resourceNativeAudience(ctx, actor)
	if err != nil {
		return empty, err
	}
	if !matrixOpaque(state) || !matrixOpaque(ticket) {
		return empty, domain.ErrInvalidInput
	}
	challenge, err := matrixPKCEChallenge(verifier)
	if err != nil {
		return empty, err
	}
	selected, err := s.matrixNativeConsentContext(ctx, actor, state)
	if err != nil {
		return empty, err
	}
	return s.pstore.FinishMatrixResourceNativeConsent(selected, actor, state, ticket, challenge, client)
}

// resourceProductClient uses only verified claims and the current shared client
// catalog. A legacy service audience or another app's client is not authority.
func resourceProductClient(ctx context.Context, actor, product string) (string, error) {
	if !storage.Product(product) {
		return "", domain.ErrForbidden
	}
	client, ok := authn.FirstPartyProductClient(ctx, actor, product)
	if !ok {
		return "", domain.ErrForbidden
	}
	return client, nil
}
