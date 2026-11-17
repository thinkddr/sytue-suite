// SPDX-License-Identifier: Apache-2.0
package notas

import (
	"context"
	"errors"
	"runtime"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/ajustes"
	"git.sytue.eu/sytue/one/apps/nucleo/auth"
	"git.sytue.eu/sytue/one/apps/nucleo/auth/authtest"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

func localWorkFixture(t *testing.T) *LocalWork {
	t.Helper()
	store, err := offline.OpenWithSecrets(t.TempDir(), new(authtest.Secrets))
	if err != nil {
		t.Fatal(err)
	}
	work, err := NewLocalWork(t.Context(), storage.Scope{Tenant: "acme", Subject: "alice", Product: "notes"}, store, func(context.Context) (func(), error) { return func() {}, nil })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(work.Close)
	return work
}

func TestLocalWorkDrainIsReversibleAndWaitsForCleanup(t *testing.T) {
	w := localWorkFixture(t)
	ctx, done, err := w.Start()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(done)
	canceled, cancel := context.WithCancel(t.Context())
	cancel()
	if err := w.Drain(canceled); !errors.Is(err, context.Canceled) {
		t.Fatal("drain skipped admitted cleanup", err)
	}
	if ctx.Err() == nil {
		t.Fatal("drain did not cancel admitted operations")
	}
	if err := w.Resume(); !errors.Is(err, offline.ErrWorkPaused) {
		t.Fatal("resumed before old work joined", err)
	}
	done()
	if err := w.Drain(t.Context()); err != nil {
		t.Fatal(err)
	}
	calls := 0
	w.OnResume(func() { calls++ })
	_, old := w.Context()
	if err := w.Resume(); err != nil {
		t.Fatal(err)
	}
	next, revision := w.Context()
	if next.Err() != nil || old == revision || calls != 1 {
		t.Fatal("resume did not replace canceled phase", next.Err(), old, revision, calls)
	}
	if ctx.Err() == nil {
		t.Fatal("resume revived an old request")
	}
	if err := w.Resume(); err != nil || calls != 1 {
		t.Fatal("duplicate resume restarted live work", err, calls)
	}
	w.Stop()
	if _, _, err := w.Start(); err == nil {
		t.Fatal("retired work admitted another request")
	}
	if err := w.Resume(); err == nil {
		t.Fatal("retired work resumed")
	}
}

func TestLocalWorkRetainsUnwrittenDraftAndCountsOneSource(t *testing.T) {
	w := localWorkFixture(t)
	if err := w.journal.Write(w.Owner(), EspacioBorradores, "nb1", "old"); err != nil {
		t.Fatal(err)
	}
	if err := w.journal.Write("bob/acme", EspacioBorradores, "foreign", "untouched"); err != nil {
		t.Fatal(err)
	}
	canceled, cancel := context.WithCancel(t.Context())
	cancel()
	if err := w.Write(canceled, EspacioBorradores, "nb1", "new"); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if p, err := w.Pending(t.Context()); err != nil || p.Count != 1 || p.Scope != w.scope {
		t.Fatal("unwritten draft double counted or other owner included", p, err)
	}
	if err := w.Drain(t.Context()); err != nil {
		t.Fatal(err)
	}
	var text string
	if found, err := w.journal.Read(w.Owner(), EspacioBorradores, "nb1", &text); err != nil || !found || text != "new" {
		t.Fatal("drain lost accepted last edit", text, found, err)
	}
	if p, err := w.Pending(t.Context()); err != nil || p.Count != 1 {
		t.Fatal("durable source no longer counted", p, err)
	}
	if found, err := w.journal.Read("bob/acme", EspacioBorradores, "foreign", &text); err != nil || !found || text != "untouched" {
		t.Fatal("foreign source changed", found, err)
	}
}

func TestLocalWorkDoesNotTurnFailedCaptureIntoDuplicateAcceptedWork(t *testing.T) {
	w := localWorkFixture(t)
	canceled, cancel := context.WithCancel(t.Context())
	cancel()
	if err := w.Write(canceled, EspacioCapturas, "unaccepted", map[string]string{"title": "draft stays in capture field"}); err == nil {
		t.Fatal("canceled capture write succeeded")
	}
	if pending, err := w.Pending(t.Context()); err != nil || pending.Count != 0 {
		t.Fatal("failed capture accepted another queued copy", pending, err)
	}
	if err := w.Write(t.Context(), "foreign-namespace", "id", "body"); err == nil {
		t.Fatal("outside local Notes namespaces accepted")
	}
	if _, err := NewLocalWork(t.Context(), storage.Scope{Tenant: "acme", Subject: "alice", Product: "mail"}, w.journal, func(context.Context) (func(), error) { return func() {}, nil }); err == nil {
		t.Fatal("foreign product bound to Notes controller")
	}
}

func TestLocalWorkKeepsAcquiredLeaseUntilFailedDraftIsDurable(t *testing.T) {
	w := localWorkFixture(t)
	releases := 0
	w.acquire = func(context.Context) (func(), error) { return func() { releases++ }, nil }
	canceled, cancel := context.WithCancel(t.Context())
	cancel()
	if err := w.Write(canceled, EspacioBorradores, "draft", "accepted but not durable"); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if releases != 0 {
		t.Fatal("failed accepted write released its shared native protection")
	}
	if err := w.Write(canceled, EspacioBorradores, "draft", "latest accepted text"); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if releases != 1 {
		t.Fatal("one retained draft kept unnecessary extra leases", releases)
	}
	if err := w.Drain(t.Context()); err != nil {
		t.Fatal(err)
	}
	if releases != 2 {
		t.Fatal("durable draft did not release each exact acquired lease", releases)
	}
	var text string
	if found, err := w.journal.Read(w.Owner(), EspacioBorradores, "draft", &text); err != nil || !found || text != "latest accepted text" {
		t.Fatal("lease released before latest source was durable", text, found, err)
	}
}

func TestLocalWorkDoesNotAcceptDraftWhenOriginalSessionLeaseIsDenied(t *testing.T) {
	w := localWorkFixture(t)
	denied := errors.New("original native witness is paused")
	w.acquire = func(context.Context) (func(), error) { return nil, denied }
	if err := w.Write(t.Context(), EspacioBorradores, "draft", "not admitted"); !errors.Is(err, ErrLocalAdmission) || !errors.Is(err, denied) {
		t.Fatal("admission failure lost its classification", err)
	}
	if pending, err := w.Pending(t.Context()); err != nil || pending.Count != 0 {
		t.Fatal("new draft accepted without its original session", pending, err)
	}
}

func TestLocalWorkFailedDraftKeepsRealSessionExclusiveLockBlocked(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("real remote Linux session-lock probe; native Apple/Binder probes are separate")
	}
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	identity := auth.IdentitySnapshot{Issuer: "https://id.example.test", Client: "notes", Subject: "alice", Organization: "acme"}
	storeID := "sesion-bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	if err := ajustes.PonerAccesoSesionCon(t.Context(), storeID, identity.IdentidadSesion(), true); err != nil {
		t.Fatal(err)
	}
	access, err := ajustes.LeerAccesoSesionCon(t.Context(), storeID)
	if err != nil {
		t.Fatal(err)
	}
	witness := auth.TestigoTrabajo{Almacen: storeID, Acceso: access}
	w := localWorkFixture(t)
	writeCtx, cancelWrite := context.WithCancel(t.Context())
	defer cancelWrite()
	w.acquire = func(ctx context.Context) (func(), error) {
		release, err := witness.Compartido(ctx)
		if err == nil {
			cancelWrite()
		}
		return release, err
	}
	if err := w.Write(writeCtx, EspacioBorradores, "draft", "accepted under original OS session lease"); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	blocked, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()
	if release, err := witness.Exclusivo(blocked); err == nil {
		release()
		t.Fatal("another app could revoke the session while its accepted draft was only in RAM")
	}
	if err := w.Drain(t.Context()); err != nil {
		t.Fatal(err)
	}
	release, err := witness.Exclusivo(t.Context())
	if err != nil {
		t.Fatal("durable source left session lock blocked", err)
	}
	release()
}
