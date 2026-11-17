// SPDX-License-Identifier: Apache-2.0
package service

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"net/url"
	"slices"
	"strings"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
)

type MatrixNativeStart struct {
	Purpose          string                  `json:"purpose,omitempty"`
	ResourceApproved bool                    `json:"resource_approved"`
	NativeConsumer   string                  `json:"native_consumer,omitempty"`
	IdentityVersion  storage.IdentityVersion `json:"identity_version,omitempty"`
	Product          string                  `json:"product"`
	Tenant           string                  `json:"tenant"`
	AuthorizeURL     string                  `json:"authorize_url"`
	State            string                  `json:"state"`
}
type MatrixNativePreview struct {
	MatrixNativeStart
	Actor         string `json:"actor"`
	HomeserverURL string `json:"homeserver_url"`
	SSOProviderID string `json:"sso_provider_id"`
}
type MatrixNativeAccepted struct {
	NativeConsumer  string                  `json:"native_consumer,omitempty"`
	IdentityVersion storage.IdentityVersion `json:"identity_version,omitempty"`
	Product         string                  `json:"product"`
	Ticket          string                  `json:"ticket"`
	RedirectURL     string                  `json:"redirect_url"`
}

func matrixOpaque(value string) bool {
	raw, err := base64.RawURLEncoding.DecodeString(value)
	return err == nil && len(raw) == 32 && base64.RawURLEncoding.EncodeToString(raw) == value
}

func matrixPKCEChallenge(verifier string) (string, error) {
	if len(verifier) < 43 || len(verifier) > 128 {
		return "", domain.ErrInvalidInput
	}
	for _, r := range verifier {
		if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9') && !strings.ContainsRune("-._~", r) {
			return "", domain.ErrInvalidInput
		}
	}
	digest := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(digest[:]), nil
}

func (s *Service) matrixNativeActor(ctx context.Context, actor string) error {
	if s.matrix == nil || s.pstore == nil {
		return domain.ErrMatrixUnavailable
	}
	if _, err := s.matrix.UserID(actor); err != nil {
		return domain.ErrForbidden
	}
	orgs, personal, err := s.matrixIdentity(ctx, actor)
	if err != nil {
		return err
	}
	tenant := tenancy.MustFrom(ctx)
	if !slices.Contains(orgs, tenant.Slug) && (!personal || !tenant.Personal()) {
		return domain.ErrForbidden
	}
	return s.pstore.MatrixActorAllowed(ctx, actor)
}

func (s *Service) matrixNativeConsentContext(ctx context.Context, actor, state string) (context.Context, error) {
	if s.pstore == nil || s.matrix == nil {
		return nil, domain.ErrMatrixUnavailable
	}
	tenant, err := s.pstore.MatrixNativeConsentTenant(ctx, actor, state)
	if err != nil {
		return nil, err
	}
	selected := tenancy.Into(ctx, tenant)
	if err := s.matrixNativeActor(selected, actor); err != nil {
		return nil, err
	}
	return selected, nil
}

func (s *Service) matrixNativePublicURL(path, state string) (string, error) {
	base, err := url.Parse(s.cfg.PublicURL)
	if err != nil || base.Scheme != "https" || base.Host == "" || base.User != nil || base.RawQuery != "" || base.Fragment != "" {
		return "", domain.ErrMatrixUnavailable
	}
	base.Path = "/rooms/matrix/native/" + path
	base.RawPath = ""
	base.RawQuery = url.Values{"state": {state}}.Encode()
	return base.String(), nil
}

func (s *Service) StartMatrixNative(ctx context.Context, actor, challenge, method, product string) (MatrixNativeStart, error) {
	return s.StartMatrixNativeForIdentity(ctx, actor, challenge, method, product, storage.LegacyIdentity)
}

func (s *Service) StartMatrixNativeForIdentity(ctx context.Context, actor, challenge, method, product string, version storage.IdentityVersion) (MatrixNativeStart, error) {
	return s.StartMatrixNativeForConsumer(ctx, actor, challenge, method, product, "", version)
}

func (s *Service) StartMatrixNativeForConsumer(ctx context.Context, actor, challenge, method, product, consumer string, version storage.IdentityVersion) (MatrixNativeStart, error) {
	version = version.Normalized()
	if product == "" {
		product = "rooms"
	}
	if !version.Valid() || method != "S256" || !matrixOpaque(challenge) || storage.NativeReturnApp(product, consumer) == "" {
		return MatrixNativeStart{}, domain.ErrInvalidInput
	}
	if err := s.matrixNativeActor(ctx, actor); err != nil {
		return MatrixNativeStart{}, err
	}
	if _, err := s.matrixStorageIdentity(ctx, storage.Scope{Tenant: tenancy.MustFrom(ctx).Slug, Subject: actor, Product: product}, version); err != nil {
		return MatrixNativeStart{}, err
	}
	state, err := s.pstore.CreateMatrixNativeConsentForConsumer(ctx, actor, challenge, product, consumer, version)
	if err != nil {
		return MatrixNativeStart{}, err
	}
	target, err := s.matrixNativePublicURL("start", state)
	if err != nil {
		return MatrixNativeStart{}, err
	}
	return MatrixNativeStart{NativeConsumer: consumer, AuthorizeURL: target, State: state, Product: product, IdentityVersion: version, Tenant: tenancy.MustFrom(ctx).Slug}, nil
}

func (s *Service) PreviewMatrixNative(ctx context.Context, actor, state string) (MatrixNativePreview, error) {
	if !matrixOpaque(state) {
		return MatrixNativePreview{}, domain.ErrInvalidInput
	}
	ctx, err := s.matrixNativeConsentContext(ctx, actor, state)
	if err != nil {
		return MatrixNativePreview{}, err
	}
	destination, err := s.pstore.PreviewMatrixNativeDestination(ctx, actor, state)
	if err != nil {
		return MatrixNativePreview{}, err
	}
	callback, err := s.matrixNativePublicURL("callback", state)
	if err != nil {
		return MatrixNativePreview{}, err
	}
	authorize, err := url.Parse(s.cfg.MatrixHomeserverURL)
	if err != nil || authorize.Scheme != "https" || authorize.Host != s.cfg.MatrixServerName || authorize.User != nil || authorize.RawQuery != "" || authorize.Fragment != "" {
		return MatrixNativePreview{}, domain.ErrMatrixUnavailable
	}
	provider := s.cfg.MatrixSSOProviderID
	if destination.IdentityVersion == storage.IsolatedIdentity {
		provider, err = s.matrix.StorageSSOProviderID(destination.Product)
		if err != nil {
			return MatrixNativePreview{}, err
		}
	}
	authorize.Path = "/_matrix/client/v3/login/sso/redirect/" + provider
	query := url.Values{"redirectUrl": {callback}}
	if destination.IdentityVersion == storage.IsolatedIdentity {
		query.Set("sytue_tenant", tenancy.MustFrom(ctx).Slug)
		query.Set("sytue_subject", actor)
	}
	authorize.RawQuery = query.Encode()
	return MatrixNativePreview{MatrixNativeStart: MatrixNativeStart{Purpose: destination.Purpose, ResourceApproved: destination.ResourceApproved, NativeConsumer: destination.NativeConsumer, Product: destination.Product, IdentityVersion: destination.IdentityVersion, AuthorizeURL: authorize.String(), State: state, Tenant: tenancy.MustFrom(ctx).Slug}, Actor: actor, HomeserverURL: s.cfg.MatrixHomeserverURL, SSOProviderID: provider}, nil
}

func (s *Service) AcceptMatrixNative(ctx context.Context, actor, state, loginToken string) (MatrixNativeAccepted, error) {
	if !matrixOpaque(state) || len(loginToken) < 16 || len(loginToken) > 8192 || strings.ContainsAny(loginToken, "\r\n\x00") {
		return MatrixNativeAccepted{}, domain.ErrInvalidInput
	}
	ctx, err := s.matrixNativeConsentContext(ctx, actor, state)
	if err != nil {
		return MatrixNativeAccepted{}, err
	}
	ticket, destination, err := s.pstore.AcceptMatrixNativeConsentWithDestination(ctx, actor, state, loginToken)
	if err != nil {
		return MatrixNativeAccepted{}, err
	}
	native := storage.NativeReturnApp(destination.Product, destination.NativeConsumer)
	if native == "" {
		return MatrixNativeAccepted{}, domain.ErrForbidden
	}
	target := url.URL{Scheme: "sytue-" + native, Host: "matrix", Path: "/callback", RawQuery: url.Values{"state": {state}, "ticket": {ticket}}.Encode()}
	return MatrixNativeAccepted{NativeConsumer: destination.NativeConsumer, Product: destination.Product, IdentityVersion: destination.IdentityVersion, Ticket: ticket, RedirectURL: target.String()}, nil
}

func (s *Service) FinishMatrixNative(ctx context.Context, actor, state, ticket, verifier string) (string, error) {
	return s.FinishMatrixNativeForIdentity(ctx, actor, state, ticket, verifier, "", storage.LegacyIdentity)
}

func (s *Service) FinishMatrixNativeForIdentity(ctx context.Context, actor, state, ticket, verifier, product string, version storage.IdentityVersion) (string, error) {
	return s.FinishMatrixNativeForConsumer(ctx, actor, state, ticket, verifier, product, "", version)
}

func (s *Service) FinishMatrixNativeForConsumer(ctx context.Context, actor, state, ticket, verifier, product, consumer string, version storage.IdentityVersion) (string, error) {
	if consumer != "" && storage.NativeReturnApp(product, consumer) == "" {
		return "", domain.ErrInvalidInput
	}
	version = version.Normalized()
	if !version.Valid() {
		return "", domain.ErrInvalidInput
	}
	if !matrixOpaque(state) || !matrixOpaque(ticket) {
		return "", domain.ErrInvalidInput
	}
	challenge, err := matrixPKCEChallenge(verifier)
	if err != nil {
		return "", err
	}
	ctx, err = s.matrixNativeConsentContext(ctx, actor, state)
	if err != nil {
		return "", err
	}
	return s.pstore.FinishMatrixNativeConsentForConsumer(ctx, actor, state, ticket, challenge, product, consumer, version)
}
