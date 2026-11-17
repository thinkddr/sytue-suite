// SPDX-License-Identifier: Apache-2.0
package service

import (
	"context"
	"slices"
	"strings"

	"github.com/google/uuid"

	roomsearch "git.sytue.eu/sytue/one/apps/nucleo/rooms/search"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
)

func (s *Service) MessageMetadata(ctx context.Context, actor, cursor string, limit int) (roomsearch.Page, error) {
	tenant := tenancy.MustFrom(ctx)
	if err := s.requireSearchIdentity(ctx, actor); err != nil {
		return roomsearch.Page{}, err
	}
	before, err := roomsearch.DecodeCursor(cursor, tenant.Slug, actor)
	if err != nil {
		return roomsearch.Page{}, domain.ErrInvalidInput
	}
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	rows, err := s.store.MessageMetadata(ctx, actor, before, limit+1, nil)
	if err != nil {
		return roomsearch.Page{}, err
	}
	if s.pstore != nil {
		external, err := s.pstore.ExternalMessageMetadata(ctx, actor, tenant.Slug, before, limit+1, nil)
		if err != nil {
			return roomsearch.Page{}, err
		}
		rows = append(rows, external...)
	}
	slices.SortFunc(rows, roomsearch.Compare)
	page := roomsearch.Page{Messages: rows}
	if len(rows) > limit {
		page.Messages = rows[:limit]
		page.Next = roomsearch.EncodeCursor(rows[limit-1], tenant.Slug, actor)
	}
	if page.Messages == nil {
		page.Messages = []roomsearch.Metadata{}
	}
	if err := s.requireSearchIdentity(ctx, actor); err != nil {
		return roomsearch.Page{}, err
	}
	return page, nil
}

// Revalidation runs through the same access predicate after local decryption.
// Callers send only identities, never search terms or decrypted content.
func (s *Service) MessageMetadataVisible(ctx context.Context, actor string, identities []roomsearch.Identity) ([]roomsearch.Metadata, error) {
	tenant := tenancy.MustFrom(ctx)
	if err := s.requireSearchIdentity(ctx, actor); err != nil {
		return nil, err
	}
	if len(identities) == 0 || len(identities) > 200 {
		return nil, domain.ErrInvalidInput
	}
	selected := map[string][]uuid.UUID{}
	seen := map[roomsearch.Identity]bool{}
	for _, ref := range identities {
		id, err := uuid.Parse(ref.ID)
		if err != nil || id == uuid.Nil || id.String() != ref.ID || strings.TrimSpace(ref.Tenant) == "" || len(ref.Tenant) > 253 || seen[ref] {
			return nil, domain.ErrInvalidInput
		}
		seen[ref] = true
		selected[ref.Tenant] = append(selected[ref.Tenant], id)
	}
	out := []roomsearch.Metadata{}
	if own := selected[tenant.Slug]; len(own) > 0 {
		rows, err := s.store.MessageMetadata(ctx, actor, nil, len(identities), own)
		if err != nil {
			return nil, err
		}
		out = append(out, rows...)
	}
	if s.pstore != nil {
		rows, err := s.pstore.ExternalMessageMetadata(ctx, actor, tenant.Slug, nil, len(identities), selected)
		if err != nil {
			return nil, err
		}
		out = append(out, rows...)
	}
	slices.SortFunc(out, roomsearch.Compare)
	if err := s.requireSearchIdentity(ctx, actor); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Service) requireSearchIdentity(ctx context.Context, actor string) error {
	if actor == "" {
		return domain.ErrForbidden
	}
	active, err := s.store.SearchIdentityActive(ctx, actor)
	if err != nil {
		return err
	}
	if !active {
		return domain.ErrForbidden
	}
	return nil
}
