// SPDX-License-Identifier: Apache-2.0
package mail

import (
	"context"
	"errors"

	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

var _ offline.PrivateWork = (*Repository)(nil)

func (r *Repository) start(parent context.Context) (context.Context, func(), error) {
	ctx, done, err := r.gate.StartBound(parent, r.cfg.BindPublicationContext)
	if errors.Is(err, offline.ErrWorkClosed) {
		return nil, nil, ErrClosed
	}
	if err != nil {
		return nil, nil, err
	}
	return ctx, done, nil
}

func (r *Repository) Drain(ctx context.Context) error { return r.gate.Drain(ctx) }
func (r *Repository) Resume() error                   { return r.gate.Resume() }

func (r *Repository) Pending(ctx context.Context) (offline.PendingWork, error) {
	return PendingForScope(ctx, r.cfg.Scope, r.cfg.Journal)
}

// PendingForScope inventories this exact application's encrypted journals even
// before a deferred repository exists. Shared source/receiver wrappers use it
// once per owner instead of counting the same records through several readers.
func PendingForScope(ctx context.Context, scope storage.Scope, journal *offline.Store) (offline.PendingWork, error) {
	out := offline.PendingWork{Scope: scope}
	if err := ctx.Err(); err != nil {
		return out, err
	}
	if !scope.Valid() || scope.Product != "mail" {
		return out, ErrRecord
	}
	if !journal.Encrypted() {
		return out, offline.ErrEncryptedStore
	}
	owner := scope.Tenant + "\x00" + scope.Subject + "\x00mail"
	namespaces := []string{storage.PendingUploadsNamespace}
	for _, namespace := range storage.PrivateJournalNamespaces("mail") {
		if namespace != storage.IdentityJournalNamespace("mail") {
			namespaces = append(namespaces, namespace)
		}
	}
	count, err := journal.CountOwner(owner, namespaces...)
	if err != nil {
		return out, err
	}
	identity, err := journal.CountOwner(owner, storage.IdentityJournalNamespace("mail"))
	if identity > 0 {
		count++
	}
	out.Count = count
	return out, err
}
