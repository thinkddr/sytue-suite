// SPDX-License-Identifier: Apache-2.0
package matrix

import (
	"context"

	"git.sytue.eu/sytue/one/apps/nucleo/resource"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// PrepareRecord shares the private application record codec with all adapters.
// Scope comes from this enrolled producer; another personal vault is not used.
func (r *ResourceRoom) PrepareRecord(ctx context.Context, header resource.Publication, record storage.Record) (*PreparedResourcePublication, error) {
	if r == nil || record.Validate(r.ActorScope()) != nil {
		return nil, resource.ErrInvalid
	}
	raw, err := resource.EncodeRecord(record)
	if err != nil {
		return nil, err
	}
	defer clear(raw)
	return r.Prepare(ctx, header, raw)
}

// ReadRecordCommitted opens through this actual SDK and current resource lease.
// The original author's scope is retained as authenticated record context only.
func (r *ResourceRoom) ReadRecordCommitted(ctx context.Context, proof resource.VerifiedCommittedRead, object, revision, role string) (storage.Record, error) {
	if r == nil {
		return storage.Record{}, resource.ErrInvalid
	}
	return resource.ReadCommittedRecord(ctx, r, proof, r.access, r.fence, object, revision, role)
}
