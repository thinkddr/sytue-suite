// SPDX-License-Identifier: Apache-2.0
package store

import (
	"context"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/services/id/internal/domain"
)

// MergeIdentityOutcome waits for an in-flight identity update before deciding
// whether its lost COMMIT acknowledgement permits reopening the source.
func (s *Store) MergeIdentityOutcome(ctx context.Context, sourceID, targetID string) (committed bool, err error) {
	err = s.InTx(ctx, func(q pg.Querier) error {
		var status, actualTarget string
		if err := q.QueryRow(ctx, `SELECT status,coalesce(merged_into_org_id::text,'') FROM organizations WHERE id=$1 FOR SHARE`, sourceID).Scan(&status, &actualTarget); err != nil {
			return err
		}
		if status != "merged" {
			return nil
		}
		if actualTarget != targetID {
			return domain.ErrConflict
		}
		committed = true
		return nil
	})
	return committed, errf("MergeIdentityOutcome", err)
}
