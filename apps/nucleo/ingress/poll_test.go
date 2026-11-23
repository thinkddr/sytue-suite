// SPDX-License-Identifier: Apache-2.0
package ingress

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	contract "git.sytue.eu/sytue/one/apps/nucleo/storage/ingress"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
)

type pollingWorkerSource struct {
	*workerSource
	after   []string
	foreign bool
}

func (s *pollingWorkerSource) Pending(_ context.Context, after string, _ int) (contract.PendingPage, error) {
	s.after = append(s.after, after)
	page := contract.PendingPage{Version: 1, Scope: s.task.Scope, Epoch: s.task.Epoch, Tasks: []contract.Task{}}
	if s.foreign {
		page.Scope.Subject = "other"
		return page, nil
	}
	if after == "" {
		task := s.task
		task.ID = "blocked"
		task.Cipher.ID = task.ID
		page.Tasks = []contract.Task{task}
		page.Next = task.ID
	} else {
		page.Tasks = []contract.Task{s.task}
	}
	return page, nil
}

func (s *pollingWorkerSource) Claim(ctx context.Context, scope storage.Scope, epoch, id string, target storage.PublicationTarget, token string, authority matrixproof.Authority) (contract.Claim, error) {
	if id == "blocked" {
		return contract.Claim{}, contract.ErrClaim
	}
	return s.workerSource.Claim(ctx, scope, epoch, id, target, token, authority)
}

func TestWorkerPollRetainsBlockedIntentAndAdvancesWithoutNestedSessionPermission(t *testing.T) {
	f := newWorkerFixture(t)
	source := &pollingWorkerSource{workerSource: f.source}
	f.cfg.Source = source
	active := 0
	f.cfg.Lease = func(ctx context.Context) (func(), error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if active != 0 {
			return nil, offline.ErrWorkLease
		}
		active++
		return func() { active-- }, nil
	}
	worker := f.open(t)
	first, err := worker.Poll(t.Context())
	if !errors.Is(err, contract.ErrClaim) || first.Attempted != 1 || first.Completed != 0 {
		t.Fatal(first, err)
	}
	var retained intent
	found, err := worker.cfg.Journal.Read(worker.owner(), storage.PrivateIngressNamespace, "blocked", &retained)
	if err != nil || !found || retained.ID != "blocked" || retained.Claim != nil {
		t.Fatal("blocked task lost its exact original intent", retained, err)
	}
	second, err := worker.Poll(t.Context())
	if !errors.Is(err, contract.ErrClaim) || second.Completed != 1 || second.Attempted != 2 {
		t.Fatal("blocked first page starved an independent delivery", second, err)
	}
	if len(source.after) != 2 || source.after[0] != "" || source.after[1] != "blocked" || f.executor.executions != 1 || f.source.acks != 1 || active != 0 {
		t.Fatal("poll did not retain one genuine session permission and cursor", source.after, active)
	}
	if err := worker.Drain(t.Context()); err != nil {
		t.Fatal(err)
	}
	before := len(source.after)
	if _, err := worker.Poll(t.Context()); !errors.Is(err, offline.ErrWorkPaused) {
		t.Fatal("drained worker admitted a discovery request", err)
	}
	if len(source.after) != before {
		t.Fatal("drained worker read another pending page")
	}
}

func TestWorkerPollRejectsForeignScopeBeforeAnyTaskOrWALMutation(t *testing.T) {
	f := newWorkerFixture(t)
	source := &pollingWorkerSource{workerSource: f.source, foreign: true}
	f.cfg.Source = source
	worker := f.open(t)
	if _, err := worker.Poll(t.Context()); !errors.Is(err, contract.ErrContract) {
		t.Fatal("foreign discovery response accepted", err)
	}
	pending, err := worker.Pending(t.Context())
	if err != nil || pending.Count != 0 || f.source.claims != 0 || f.executor.executions != 0 {
		t.Fatal("invalid discovery caused task effects", pending, err)
	}
}

type acknowledgedPollingSource struct{ *workerSource }

func (s acknowledgedPollingSource) Pending(context.Context, string, int) (contract.PendingPage, error) {
	page := contract.PendingPage{Version: 1, Scope: s.task.Scope, Epoch: s.task.Epoch, Tasks: []contract.Task{}}
	if !s.acknowledged {
		page.Tasks = append(page.Tasks, s.task)
	}
	return page, nil
}

func TestWorkerPollReplaysLostAcceptedACKEvenWhenRemotePendingIsEmpty(t *testing.T) {
	f := newWorkerFixture(t)
	f.source.loseACK = true
	f.source.rejectCompletedClaim = true
	f.cfg.Source = acknowledgedPollingSource{f.source}
	worker := f.open(t)
	first, err := worker.Poll(t.Context())
	if !errors.Is(err, io.ErrUnexpectedEOF) || first.Attempted != 1 || first.Completed != 0 {
		t.Fatal(first, err)
	}
	var work intent
	found, err := worker.cfg.Journal.Read(worker.owner(), storage.PrivateIngressNamespace, f.source.task.ID, &work)
	if err != nil || !found || work.Claim == nil || work.Commit == nil {
		t.Fatal("lost accepted ACK did not retain original commit", work, err)
	}
	work.Claim.ExpiresAt = time.Now().Add(-time.Minute)
	if err := worker.save(&work); err != nil {
		t.Fatal(err)
	}
	claims := f.source.claims
	worker.Close()
	worker = f.open(t)
	second, err := worker.Poll(t.Context())
	if err != nil || second.Completed != 1 || second.Attempted != 1 {
		t.Fatal("remote absence hid retained accepted ACK", second, err)
	}
	pending, err := worker.Pending(t.Context())
	if err != nil || pending.Count != 0 || f.source.claims != claims || f.source.acks != 2 || f.executor.executions != 1 {
		t.Fatal("accepted ACK retry changed claim or republished", pending, err, f.source.claims, f.executor.executions)
	}
}

// Tras rotar la clave de entrada (ADR 0087), la bóveda no reclama las entregas
// cifradas para otra clave: se quedan en la cola, sin tocarlas.
func TestWorkerPollNoReclamaEntregasDeOtraClave(t *testing.T) {
	f := newWorkerFixture(t)
	source := &pollingWorkerSource{workerSource: f.source}
	f.cfg.Source = source
	f.cfg.KeyID = f.source.task.KeyID
	ajena := source.task
	ajena.KeyID = "0000000000000000000000000000000000000000000000000000000000000000"
	source.task = ajena
	worker := f.open(t)
	for range 2 {
		got, err := worker.Poll(t.Context())
		if err != nil || got.Attempted != 0 || f.source.claims != 0 {
			t.Fatal("se reclamó una entrega de otra clave", got, err, f.source.claims)
		}
	}
	ids, err := worker.cfg.Journal.RecordIDs(worker.owner(), storage.PrivateIngressNamespace)
	if err != nil || len(ids) != 0 {
		t.Fatal("una entrega de otra clave dejó intención", ids, err)
	}
	// Y la de su clave sí se procesa.
	source.task = f.source.task
	source.task.KeyID = f.cfg.KeyID
	if got, _ := worker.Poll(t.Context()); got.Attempted == 0 {
		t.Fatal("la entrega de su clave no se intentó")
	}
}
