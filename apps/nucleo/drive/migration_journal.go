// SPDX-License-Identifier: Apache-2.0
package drive

import (
	"context"
	"errors"
	"strings"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	catalogtransfer "git.sytue.eu/sytue/one/apps/nucleo/storage/transfer"
)

const identityHistoryNamespace = "drive-private-cutover-history-v1"

func (r *Repository) finishInitialization(ctx context.Context, state storage.LegacyMigrationState) error {
	complete, err := r.completed(ctx, state)
	if err != nil {
		return err
	}
	if !complete {
		return ErrNotReady
	}
	ids, err := r.cfg.Journal.RecordIDs(r.owner(), migrationNamespace)
	if err != nil {
		return err
	}
	for _, id := range ids {
		if strings.HasPrefix(id, adoptPrefix) {
			// La adopción de la copia del servidor (AdoptRetainedSource) ya se
			// cumplió: el recibo está autenticado.
			if err := r.cfg.Journal.Delete(r.owner(), migrationNamespace, id); err != nil {
				return err
			}
			continue
		}
		if !strings.HasPrefix(id, "begin_") {
			continue
		}
		var work beginWork
		found, err := r.cfg.Journal.Read(r.owner(), migrationNamespace, id, &work)
		if err != nil {
			return err
		}
		if !found || work.Scope != r.cfg.Scope || !storage.MigrationEpoch(work.Epoch) || !work.Target.Valid() || id != "begin_"+digest([]byte(work.Target.DeviceID)) {
			return ErrSource
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		// A begin intent contains only routing. Its fulfilled snapshot is now
		// authenticated, even if this device lost the race to another epoch.
		if err := r.cfg.Journal.Delete(r.owner(), migrationNamespace, id); err != nil {
			return err
		}
	}
	return r.markReady(ctx)
}

// AcknowledgeIdentityCutover archives the completed transfer map only AFTER the
// durable backend returned isolated routing and the exact authenticated receipt.
// Before this acknowledgment the map remains pending and blocks destructive
// logout. Archived maps remain encrypted and protected across other logins.
func (r *Repository) AcknowledgeIdentityCutover(parent context.Context, state storage.IdentityCutoverState) error {
	ctx, done, err := r.start(parent)
	if err != nil {
		return err
	}
	defer done()
	if state.Validate(r.cfg.Scope) != nil || state.Phase != "isolated" || state.Receipt == nil {
		return ErrSource
	}
	return r.cfg.Journal.Exclusive(ctx, r.owner(), migrationNamespace, "initialize", func() error {
		return r.archiveAcknowledgedCutover(ctx, state)
	})
}

func (r *Repository) archiveAcknowledgedCutover(ctx context.Context, state storage.IdentityCutoverState) error {
	receipt, err := r.PrivateMigrationReceipt(ctx)
	if err != nil {
		return err
	}
	err = catalogtransfer.RetireAcknowledged(ctx, r.cfg.Journal, r.owner(), identityMigrationNamespace, identityHistoryNamespace, r.cfg.Scope, state, receipt)
	if errors.Is(err, catalogtransfer.ErrSource) {
		return ErrSource
	}
	return err
}
