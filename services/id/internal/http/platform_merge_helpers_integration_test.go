// SPDX-License-Identifier: Apache-2.0
//go:build integration

package http

import (
	"context"

	"git.sytue.eu/sytue/one/services/id/internal/merge"
)

// executeMerge also owns every SQL connection in direct integration callers.
func (s *Server) executeMerge(ctx context.Context, mergeID, sourceSlug, targetSlug, sourceOrgID, targetOrgID string) error {
	owner, err := merge.AcquireOwnership(ctx, s.cfg.DatabaseDSN, mergeID)
	if err != nil {
		return err
	}
	defer closeMergeOwnership(ctx, s.log, owner)
	return (&mergeExecution{Server: s, store: s.store.WithDatabase(owner.Identity()), owner: owner}).executeMerge(ctx, mergeID, sourceSlug, targetSlug, sourceOrgID, targetOrgID)
}

// Tests and maintenance callers may settle an already prepared admission.
func (s *Server) settleFailedMergeAdmission(ctx context.Context, admission *merge.Admission, sourceOrgID, targetOrgID string, cause error) error {
	return (&mergeExecution{Server: s, store: s.store}).settleFailedMergeAdmission(ctx, admission, sourceOrgID, targetOrgID, cause)
}
