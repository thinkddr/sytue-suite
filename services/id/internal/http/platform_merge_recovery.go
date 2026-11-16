// SPDX-License-Identifier: Apache-2.0
package http

import (
	"context"
	"sync"
	"time"
)

type mergeRunnerState struct {
	once sync.Once
	wake chan struct{}
}

func newMergeRunnerState() *mergeRunnerState { return &mergeRunnerState{wake: make(chan struct{}, 1)} }

// RunMergeRecovery discovers authorized pending/running jobs. Every replica has
// one serial traversal; physical global database locks decide ownership, never
// a polling interval or an age. HTTP requests only coalesce a wakeup.
func (s *Server) RunMergeRecovery(ctx context.Context) {
	if s.mergeRunner == nil {
		s.log.Error("merge: runner state not initialized")
		return
	}
	s.mergeRunner.once.Do(func() { s.runMergeRecovery(ctx) })
}

func (s *Server) runMergeRecovery(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		if err := s.RecoverOrgMerges(ctx); err != nil && ctx.Err() == nil {
			s.log.Error("merge: recovery discovery failed", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		case <-s.mergeRunner.wake:
		}
	}
}

// RecoverOrgMerges traverses bounded pages and runs at most one job at a
// time in this recovery loop. A live competing owner leaves the job untouched.
func (s *Server) RecoverOrgMerges(ctx context.Context) error {
	after := ""
	for {
		intents, err := s.store.RecoverableMergeIntentsPage(ctx, after)
		if err != nil {
			return err
		}
		if len(intents) == 0 {
			return nil
		}
		for _, intent := range intents {
			jobCtx, cancel := context.WithTimeout(ctx, mergeTimeout)
			s.runOwnedMerge(jobCtx, intent.ID, intent.SourceSlug, intent.TargetSlug, intent.SourceOrgID, intent.TargetOrgID)
			cancel()
			if ctx.Err() != nil {
				return ctx.Err()
			}
		}
		after = intents[len(intents)-1].ID
	}
}
