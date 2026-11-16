// SPDX-License-Identifier: Apache-2.0
package drive

import (
	"context"
	"strings"

	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

var _ offline.PrivateWork = (*Repository)(nil)

func (r *Repository) Drain(ctx context.Context) error { return r.gate.Drain(ctx) }
func (r *Repository) Resume() error                   { return r.gate.Resume() }

// Pending remains readable after Drain. It never acquires a shared session lease
// while logout holds its exclusive lease, nor treats a read failure as no work.
func (r *Repository) Pending(ctx context.Context) (offline.PendingWork, error) {
	result := offline.PendingWork{Scope: r.cfg.Scope}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	operations, jobs := map[string]bool{}, map[string]string{}
	ids, err := r.cfg.Journal.RecordIDs(r.owner(), migrationNamespace)
	if err != nil {
		return result, err
	}
	for _, id := range ids {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		logical := id
		if strings.HasPrefix(id, "begin_") || strings.HasPrefix(id, "row_") || strings.HasPrefix(id, "completion_") {
			logical = "snapshot"
		}
		operations[logical] = true
		if strings.HasPrefix(id, "begin_") {
			continue
		}
		if err := r.pendingBodyJobs(id, logical, jobs); err != nil {
			return result, err
		}
	}
	transfers, err := r.cfg.Journal.RecordIDs(r.owner(), identityMigrationNamespace)
	if err != nil {
		return result, err
	}
	if len(transfers) > 0 {
		operations["identity"] = true
		for _, id := range transfers {
			jobs[id] = "identity"
		}
	}
	uploads, err := r.cfg.Journal.RecordIDs(r.owner(), storage.PendingUploadsNamespace)
	if err != nil {
		return result, err
	}
	for _, id := range uploads {
		job := strings.TrimSuffix(id, "_publication")
		logical, known := jobs[job]
		if !known {
			logical = "sdk_" + job
		}
		operations[logical] = true
	}
	result.Count = len(operations)
	return result, ctx.Err()
}

func (r *Repository) pendingBodyJobs(id, logical string, jobs map[string]string) error {
	var work rowWork
	found, err := r.cfg.Journal.Read(r.owner(), migrationNamespace, id, &work)
	if err != nil {
		return err
	}
	if !found || work.V != 1 || work.Scope != r.cfg.Scope {
		return ErrSource
	}
	defer func() {
		if work.Creation != nil {
			clear(work.Creation.Raw)
		}
		if work.Mutation != nil {
			clear(work.Mutation.Raw)
		}
	}()
	for purpose, file := range work.Bodies {
		if file == nil || !storage.Identifier(file.Revision) {
			return ErrSource
		}
		jobs[bodyJob(work, purpose, file.Revision)] = logical
	}
	return nil
}
