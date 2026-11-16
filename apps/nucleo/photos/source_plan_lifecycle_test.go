// SPDX-License-Identifier: Apache-2.0
package photos

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestPhotosRepositoryDrainsItsSharedSourcePlanWithoutCountingWALTwice(t *testing.T) {
	plans, _, _, _ := archivePublishFixture(t)
	source := &PlannedSource{scope: plans.cfg.Scope, plans: plans}
	repository, err := New(Config{Context: t.Context(), Scope: plans.cfg.Scope, Crypto: plans.cfg.Crypto, Gateway: plans.cfg.Gateway, Journal: plans.cfg.Journal, Source: source})
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	work, done, err := plans.gate.Start(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer done()
	result := make(chan error, 1)
	go func() { result <- repository.Drain(t.Context()) }()
	select {
	case <-work.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("repository drain left source-plan publication running")
	}
	select {
	case err := <-result:
		t.Fatal("source-plan drain returned before its writer joined", err)
	default:
	}
	done()
	if err := <-result; err != nil {
		t.Fatal(err)
	}
	if err := repository.Resume(); err != nil {
		t.Fatal(err)
	}
	_, resumed, err := plans.gate.Start(t.Context())
	if err != nil {
		t.Fatal("source plan remained paused after repository resumed", err)
	}
	resumed()
	if err := plans.cfg.Journal.Write(plans.owner(), "photos-private-v1", sourcePlanJob(plannedEpoch), map[string]string{"retained": "synthetic pending source plan"}); err != nil {
		t.Fatal(err)
	}
	pending, err := repository.Pending(t.Context())
	if err != nil || pending.Scope != plans.cfg.Scope || pending.Count != 1 {
		t.Fatal("shared source-plan WAL was omitted or counted twice", pending, err)
	}
}

func TestPhotosRepositoryDrainFailureStillCancelsBothGates(t *testing.T) {
	plans, _, _, _ := archivePublishFixture(t)
	repository, err := New(Config{Context: t.Context(), Scope: plans.cfg.Scope, Crypto: plans.cfg.Crypto, Gateway: plans.cfg.Gateway, Journal: plans.cfg.Journal, Source: &PlannedSource{scope: plans.cfg.Scope, plans: plans}})
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	source, sourceDone, err := plans.gate.Start(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer sourceDone()
	data, dataDone, err := repository.engine.Start(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer dataDone()
	deadline, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancel()
	if err := repository.Drain(deadline); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("unjoined source plan did not block retirement", err)
	}
	if source.Err() == nil || data.Err() == nil {
		t.Fatal("failed drain left one gate running", source.Err(), data.Err())
	}
}
