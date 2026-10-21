// SPDX-License-Identifier: Apache-2.0
package service

import (
	"context"
	"slices"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/services/rooms/internal/media/domain"
)

// FreshDocumentCallContext binds every document-room HTTP action to current ID
// metadata and drops stale builtin administration roles before moderation.
func (s *Service) FreshDocumentCallContext(ctx context.Context, id uuid.UUID) (context.Context, error) {
	if s.encryption == nil {
		return ctx, nil
	}
	room, err := s.store.GetRoom(ctx, id)
	if err != nil {
		return ctx, err
	}
	if documentRef(room) == "" {
		return ctx, nil
	}
	claims, ok := authn.FromContext(ctx)
	if !ok {
		return ctx, domain.ErrPermission
	}
	ctx, err = s.encryptedDocumentContext(ctx, room, claims.Subject)
	if err != nil {
		return ctx, err
	}
	proof, _ := domain.DocumentCallAccessFrom(ctx)
	return currentDocumentIdentity(ctx, claims, proof), nil
}

func currentDocumentIdentity(ctx context.Context, claims authn.Claims, proof domain.DocumentCallAccess) context.Context {
	claims.Roles = slices.DeleteFunc(slices.Clone(claims.Roles), func(role string) bool { return role == "admin" || role == "owner" })
	if proof.OrganizationAdmin {
		claims.Roles = append(claims.Roles, "admin")
	}
	return authn.Into(ctx, claims)
}

func currentDocumentAdministrator(ctx context.Context, room domain.Room, fallback bool) bool {
	proof, ok := domain.DocumentCallAccessFrom(ctx)
	if !ok || documentRef(room) == "" {
		return fallback
	}
	return proof.OrganizationAdmin
}
