// SPDX-License-Identifier: Apache-2.0
package service

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"net/url"
	"slices"
	"strings"

	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
)

type MatrixNativeStart struct {
	AuthorizeURL string `json:"authorize_url"`
	State        string `json:"state"`
}
type MatrixNativePreview struct {
	MatrixNativeStart
	Actor  string `json:"actor"`
	Tenant string `json:"tenant"`
}
type MatrixNativeAccepted struct {
	Ticket      string `json:"ticket"`
	RedirectURL string `json:"redirect_url"`
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

func (s *Service) StartMatrixNative(ctx context.Context, actor, challenge, method string) (MatrixNativeStart, error) {
	if method != "S256" || !matrixOpaque(challenge) {
		return MatrixNativeStart{}, domain.ErrInvalidInput
	}
	if err := s.matrixNativeActor(ctx, actor); err != nil {
		return MatrixNativeStart{}, err
	}
	if _, err := s.matrix.EnsureUser(ctx, actor); err != nil {
		return MatrixNativeStart{}, err
	}
	state, err := s.pstore.CreateMatrixNativeConsent(ctx, actor, challenge)
	if err != nil {
		return MatrixNativeStart{}, err
	}
	target, err := s.matrixNativePublicURL("start", state)
	if err != nil {
		return MatrixNativeStart{}, err
	}
	return MatrixNativeStart{AuthorizeURL: target, State: state}, nil
}

func (s *Service) PreviewMatrixNative(ctx context.Context, actor, state string) (MatrixNativePreview, error) {
	if !matrixOpaque(state) {
		return MatrixNativePreview{}, domain.ErrInvalidInput
	}
	if err := s.matrixNativeActor(ctx, actor); err != nil {
		return MatrixNativePreview{}, err
	}
	if err := s.pstore.PreviewMatrixNativeConsent(ctx, actor, state); err != nil {
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
	authorize.Path = "/_matrix/client/v3/login/sso/redirect/" + s.cfg.MatrixSSOProviderID
	authorize.RawQuery = url.Values{"redirectUrl": {callback}}.Encode()
	return MatrixNativePreview{MatrixNativeStart: MatrixNativeStart{AuthorizeURL: authorize.String(), State: state}, Actor: actor, Tenant: tenancy.MustFrom(ctx).Slug}, nil
}

func (s *Service) AcceptMatrixNative(ctx context.Context, actor, state, loginToken string) (MatrixNativeAccepted, error) {
	if !matrixOpaque(state) || len(loginToken) < 16 || len(loginToken) > 8192 || strings.ContainsAny(loginToken, "\r\n\x00") {
		return MatrixNativeAccepted{}, domain.ErrInvalidInput
	}
	if err := s.matrixNativeActor(ctx, actor); err != nil {
		return MatrixNativeAccepted{}, err
	}
	ticket, err := s.pstore.AcceptMatrixNativeConsent(ctx, actor, state, loginToken)
	if err != nil {
		return MatrixNativeAccepted{}, err
	}
	target := url.URL{Scheme: "sytue-rooms", Host: "matrix", Path: "/callback", RawQuery: url.Values{"state": {state}, "ticket": {ticket}}.Encode()}
	return MatrixNativeAccepted{Ticket: ticket, RedirectURL: target.String()}, nil
}

func (s *Service) FinishMatrixNative(ctx context.Context, actor, state, ticket, verifier string) (string, error) {
	if !matrixOpaque(state) || !matrixOpaque(ticket) {
		return "", domain.ErrInvalidInput
	}
	challenge, err := matrixPKCEChallenge(verifier)
	if err != nil {
		return "", err
	}
	if err := s.matrixNativeActor(ctx, actor); err != nil {
		return "", err
	}
	return s.pstore.FinishMatrixNativeConsent(ctx, actor, state, ticket, challenge)
}
