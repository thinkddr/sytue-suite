// SPDX-License-Identifier: Apache-2.0
package mail

import (
	"context"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/transfer"
)

func (r *Repository) privateMigrationReceipt(ctx context.Context, snapshot storage.AtomicCatalogSnapshot) (storage.Reference, error) {
	if r.cfg.Crypto.StorageIdentityVersion() != storage.IsolatedIdentity {
		return storage.Reference{}, ErrRecord
	}
	if _, err := r.completedReceipt(ctx, snapshot); err != nil {
		return storage.Reference{}, err
	}
	versions := heads(snapshot, objectID("migration", r.cfg.MigrationState.Epoch))
	if len(versions) != 1 {
		return storage.Reference{}, ErrConflict
	}
	return versions[0].Reference, nil
}

func (r *Repository) PrivateMigrationReceipt(parent context.Context) (storage.Reference, error) {
	var receipt storage.Reference
	err := r.run(parent, true, func(ctx context.Context, snapshot storage.AtomicCatalogSnapshot) error {
		var err error
		receipt, err = r.privateMigrationReceipt(ctx, snapshot)
		return err
	})
	return receipt, err
}

// AcknowledgeIdentityCutover first reopens every referenced MIME/document from
// the SDK, then archives the exact transfer WAL before retiring active markers.
func (r *Repository) AcknowledgeIdentityCutover(parent context.Context, state storage.IdentityCutoverState) error {
	if state.Validate(r.cfg.Scope) != nil || state.Phase != "isolated" || state.Receipt == nil {
		return ErrRecord
	}
	ctx, done, err := r.start(parent)
	if err != nil {
		return err
	}
	defer done()
	return r.runAdmitted(ctx, true, func(ctx context.Context, snapshot storage.AtomicCatalogSnapshot) error {
		receipt, err := r.privateMigrationReceipt(ctx, snapshot)
		if err != nil {
			return err
		}
		if receipt != *state.Receipt {
			return ErrRecord
		}
		if err := r.cfg.Crypto.CheckStoragePublicationTarget(snapshot.Binding.Target); err != nil {
			return err
		}
		owner := r.cfg.Scope.Tenant + "\x00" + r.cfg.Scope.Subject + "\x00mail"
		return r.cfg.Journal.Exclusive(ctx, owner, storage.IdentityJournalNamespace("mail"), "catalog", func() error {
			return transfer.RetireAcknowledged(ctx, r.cfg.Journal, owner, storage.IdentityJournalNamespace("mail"), storage.IdentityHistoryNamespace("mail"), r.cfg.Scope, state, receipt)
		})
	})
}
