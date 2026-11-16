// SPDX-License-Identifier: Apache-2.0
package store

import (
	"context"
	"fmt"

	"github.com/google/uuid"
)

// RecoverableMergeIntent contains only the original routing needed to resume one
// authorized pending/running job. Discovery does not claim it from an age.
type RecoverableMergeIntent struct{ ID, SourceOrgID, TargetOrgID, SourceSlug, TargetSlug string }

func (s *Store) RecoverableMergeIntentsPage(ctx context.Context, after string) ([]RecoverableMergeIntent, error) {
	var cursor *uuid.UUID
	if after != "" {
		id, err := uuid.Parse(after)
		if err != nil {
			return nil, fmt.Errorf("merge: invalid recovery cursor")
		}
		cursor = &id
	}
	rows, err := s.DB().Query(ctx, `SELECT m.id::text,m.source_org_id::text,m.target_org_id::text,source.slug,target.slug
 FROM org_merges m JOIN organizations source ON source.id=m.source_org_id JOIN organizations target ON target.id=m.target_org_id
 WHERE m.status IN('pending','running') AND ($1::uuid IS NULL OR m.id>$1::uuid) ORDER BY m.id LIMIT 32`, cursor)
	if err != nil {
		return nil, errf("RecoverableMergeIntentsPage", err)
	}
	defer rows.Close()
	intents := make([]RecoverableMergeIntent, 0, 32)
	for rows.Next() {
		var intent RecoverableMergeIntent
		if err := rows.Scan(&intent.ID, &intent.SourceOrgID, &intent.TargetOrgID, &intent.SourceSlug, &intent.TargetSlug); err != nil {
			return nil, err
		}
		intents = append(intents, intent)
	}
	return intents, errf("RecoverableMergeIntentsPage", rows.Err())
}
