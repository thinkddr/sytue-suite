// SPDX-License-Identifier: Apache-2.0
package service

import (
	"context"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/publication"
	"git.sytue.eu/sytue/one/platform/pkg/log"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/rooms/internal/store"
)

type publicationReconciliationStore interface {
	PendingMatrixPublicationOrigins(context.Context, *store.MatrixPublicationCursor, int) ([]store.MatrixPublicationPending, *store.MatrixPublicationCursor, error)
	AcceptMatrixPublicationOrigin(context.Context, storage.Scope, string, publication.OriginEvent) error
}
type publicationOriginSource interface {
	RetainedPublicationOrigin(context.Context, storage.Scope, storage.Reference, string) (publication.OriginEvent, error)
}

// Only positive, exact readback can acknowledge an origin. Absence, expiry,
// transport errors and untrusted responses leave the durable hold untouched.
func reconcileMatrixPublications(ctx context.Context, st publicationReconciliationStore, source publicationOriginSource, after *store.MatrixPublicationCursor) (*store.MatrixPublicationCursor, int, error) {
	pending, next, err := st.PendingMatrixPublicationOrigins(ctx, after, 16)
	if err != nil {
		return after, 0, err
	}
	accepted := 0
	for _, value := range pending {
		if err := ctx.Err(); err != nil {
			return next, accepted, err
		}
		tenant, err := tenancy.New(value.Cursor.Scope.Tenant)
		if err != nil {
			continue
		}
		scoped := tenancy.Into(ctx, tenant)
		origin, err := source.RetainedPublicationOrigin(scoped, value.Cursor.Scope, value.Reference, value.MatrixUserID)
		if err != nil || origin.Reference != value.Reference || origin.Sender != value.MatrixUserID || origin.Type != "m.room.encrypted" || !origin.Valid() {
			continue
		}
		if err := st.AcceptMatrixPublicationOrigin(scoped, value.Cursor.Scope, origin.Sender, origin); err != nil {
			continue
		}
		accepted++
	}
	return next, accepted, nil
}

// RunMatrixPublicationReconciliation repairs lost after-persist notifications.
// It never retries a send or treats a 404 as proof that a send is absent.
func (s *Service) RunMatrixPublicationReconciliation(ctx context.Context) {
	if s == nil || s.pstore == nil || s.matrix == nil || s.cfg.MatrixPublicationSecret == "" {
		return
	}
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	var cursor *store.MatrixPublicationCursor
	for {
		round, cancel := context.WithTimeout(ctx, 20*time.Second)
		next, _, err := reconcileMatrixPublications(round, s.pstore, s.matrix, cursor)
		cancel()
		cursor = next
		if err != nil && ctx.Err() == nil {
			log.From(ctx).Warn("Matrix publication reconciliation unavailable")
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
