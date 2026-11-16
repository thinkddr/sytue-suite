// SPDX-License-Identifier: Apache-2.0
package records

import (
	"context"
	"strings"

	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

var _ offline.PrivateWork = (*Repository)(nil)

func (r *Repository) Drain(ctx context.Context) error { return r.gate.Drain(ctx) }
func (r *Repository) Resume() error                   { return r.gate.Resume() }

// Pending reads durable ownership, not a best-effort in-memory upload counter.
// Import journals remain pending until their exact accepted commit is reconciled.
func (r *Repository) Pending(ctx context.Context) (offline.PendingWork, error) {
	result := offline.PendingWork{Scope: r.cfg.Scope}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	if !r.cfg.Journal.Encrypted() {
		return result, offline.ErrEncryptedStore
	}
	operations := map[string]bool{}
	for _, namespace := range []string{r.namespace(), storage.PendingUploadsNamespace, storage.IdentityJournalNamespace(r.cfg.Product)} {
		ids, err := r.cfg.Journal.RecordIDs(r.owner(), namespace)
		if err != nil {
			return result, err
		}
		for _, id := range ids {
			if namespace == storage.IdentityJournalNamespace(r.cfg.Product) {
				operations["identity-cutover"] = true
				continue
			}
			switch {
			case id == "snapshot" || strings.HasPrefix(id, "import_"):
				operations["snapshot"] = true
			default:
				key := strings.TrimSuffix(strings.TrimSuffix(id, "_file"), "_publication")
				operations[key] = true
			}
		}
	}
	result.Count = len(operations)
	return result, nil
}
