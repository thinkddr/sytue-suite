// SPDX-License-Identifier: Apache-2.0
//go:build linux

package ajustes

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

type ledgerFixture struct {
	mu             sync.Mutex
	own            registroTrabajoBroker
	operations     int
	foreign        int
	foreignDiscard bool
	starts         int
}

func (l *ledgerFixture) reply(action string, p pedidoTrabajoBroker) *registroTrabajoBroker {
	l.mu.Lock()
	defer l.mu.Unlock()
	switch action {
	case "iniciar":
		if l.own.Desecho != "" && !l.own.DesechoAplicado {
			return &registroTrabajoBroker{}
		}
		l.operations++
		l.starts++
	case "terminar":
		if !p.Fallo {
			l.operations--
			if *p.Pendientes > l.own.Pendientes {
				l.own.Pendientes = *p.Pendientes
			}
		}
	case "reconciliar":
		l.operations = 0
		l.own.Pendientes = *p.Pendientes
	case "desechar":
		if l.own.Pendientes > 0 || l.operations > 0 {
			l.own.Desecho = "55555555555555555555555555555555"
			l.own.DesechoAplicado = false
		}
		l.foreignDiscard = l.foreign > 0
	case "confirmar-desecho":
		l.own.Pendientes = 0
		l.operations = 0
		l.own.DesechoAplicado = true
	}
	l.own.Incierto = l.operations > 0 || (l.own.Desecho != "" && !l.own.DesechoAplicado)
	result := l.own
	if (action == "resumir" && p.Ambito == nil) || action == "desechar" {
		result.Pendientes += l.foreign
		result.TodoConDesecho = (l.own.Pendientes == 0 && !l.own.Incierto || l.own.Desecho != "" && !l.own.DesechoAplicado) && (l.foreign == 0 || l.foreignDiscard)
	}
	return &result
}

func engineWorkFixture(t *testing.T) (*TrabajoNativo, storage.Scope, *ledgerFixture) {
	t.Helper()
	ledger := &ledgerFixture{}
	work, scope, _ := nativeWorkFixture(t, ledger.reply)
	return work, scope, ledger
}

func TestNativeLeaseAppliesDurableTombstoneBeforeFirstWALAccess(t *testing.T) {
	work, scope, ledger := engineWorkFixture(t)
	ledger.own = registroTrabajoBroker{Pendientes: 3, Incierto: true, Desecho: "55555555555555555555555555555555"}
	dropped := false
	metadata := MetadatosTrabajoNativo{Contar: func(got storage.Scope) (int, error) {
		if got != scope || !dropped {
			t.Fatal("WAL metadata opened before requested cleanup")
		}
		return 0, nil
	}, Desechar: func(_ context.Context, got storage.Scope) error {
		if got != scope {
			t.Fatal("discard crossed scope")
		}
		dropped = true
		return nil
	}}
	acquire := work.ArrendamientoPrivado(scope, metadata)
	done, err := acquire(t.Context())
	if err != nil || !dropped || !ledger.own.DesechoAplicado || ledger.starts != 1 {
		t.Fatal("WAL was not gated by cleanup ACK", err)
	}
	done()
}

func TestNativeLeaseFailedCleanupNeverOpensWALOrAcknowledgesZero(t *testing.T) {
	work, scope, ledger := engineWorkFixture(t)
	ledger.own = registroTrabajoBroker{Pendientes: 2, Incierto: true, Desecho: "55555555555555555555555555555555"}
	failure := errors.New("local storage unavailable")
	metadata := MetadatosTrabajoNativo{Contar: func(storage.Scope) (int, error) { t.Fatal("counted after failed deletion"); return 0, nil }, Desechar: func(context.Context, storage.Scope) error { return failure }}
	if release, err := work.ArrendamientoPrivado(scope, metadata)(t.Context()); release != nil || !errors.Is(err, failure) {
		t.Fatal("failed tombstone applied as zero", err)
	}
	if ledger.starts != 0 || ledger.own.DesechoAplicado || ledger.own.Pendientes != 2 {
		t.Fatal("cleanup failure lost retained work")
	}
}

func TestNativeLeaseNestedReleaseDoesNotWaitForItsOwnSHOrCanceledContext(t *testing.T) {
	work, scope, ledger := engineWorkFixture(t)
	metadata := MetadatosTrabajoNativo{Contar: func(storage.Scope) (int, error) { return 0, nil }, Desechar: func(context.Context, storage.Scope) error { t.Fatal("implicit discard"); return nil }}
	acquire := work.ArrendamientoPrivado(scope, metadata)
	ctx, cancel := context.WithCancel(t.Context())
	first, err := acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	second, err := acquire(ctx)
	if err != nil {
		first()
		t.Fatal(err)
	}
	cancel()
	finished := make(chan struct{})
	go func() { first(); close(finished) }()
	select {
	case <-finished:
	case <-time.After(250 * time.Millisecond):
		second()
		t.Fatal("nested release tried EX while still holding its own SH")
	}
	second()
	if ledger.operations != 0 || ledger.own.Incierto {
		t.Fatal("canceled parent prevented durable reports")
	}
}

func TestNativeLeaseScanFailureRetainsUnknownWorkUntilVerifiedEX(t *testing.T) {
	work, scope, ledger := engineWorkFixture(t)
	bad := false
	failure := errors.New("metadata scan failed")
	metadata := MetadatosTrabajoNativo{Contar: func(storage.Scope) (int, error) {
		if bad {
			return 0, failure
		}
		return 0, nil
	}, Desechar: func(context.Context, storage.Scope) error { t.Fatal("implicit discard"); return nil }}
	done, err := work.ArrendamientoPrivado(scope, metadata)(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	bad = true
	done()
	if !ledger.own.Incierto || ledger.operations != 1 {
		t.Fatal("failed report cleared durable dirty state")
	}
	if release, err := work.PrepararCierre(t.Context(), []storage.Scope{scope}, metadata, false); release != nil || !errors.Is(err, failure) {
		t.Fatal("logout ignored metadata failure", err)
	}
	bad = false
	if err := work.PrepararAmbito(t.Context(), scope, metadata); err != nil || ledger.own.Incierto || ledger.operations != 0 {
		t.Fatal("verified recovery did not reconcile retained work", err)
	}
}

func TestNativeClosePreservesForeignUIDWorkAndRequiresExplicitDiscard(t *testing.T) {
	work, scope, ledger := engineWorkFixture(t)
	ledger.foreign = 4
	metadata := MetadatosTrabajoNativo{Contar: func(storage.Scope) (int, error) { return 0, nil }, Desechar: func(context.Context, storage.Scope) error { t.Fatal("another UID's WAL deleted locally"); return nil }}
	if release, err := work.PrepararCierre(t.Context(), []storage.Scope{scope}, metadata, false); release != nil || !errors.Is(err, ErrTrabajoNativoPendiente) {
		t.Fatal("foreign pending work was treated as zero", err)
	}
	release, err := work.PrepararCierre(t.Context(), []storage.Scope{scope}, metadata, true)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if ledger.foreign != 4 || !ledger.foreignDiscard {
		t.Fatal("discard pretended the other UID applied its tombstone")
	}
}

func TestNativeLeaseInvalidMetadataFailsBeforeBrokerOrWALIO(t *testing.T) {
	work, scope, ledger := engineWorkFixture(t)
	for _, metadata := range []MetadatosTrabajoNativo{{}, {Contar: func(storage.Scope) (int, error) { t.Fatal("invalid callback performed IO"); return 0, nil }}} {
		if done, err := work.ArrendamientoPrivado(scope, metadata)(t.Context()); done != nil || !errors.Is(err, ErrProtocoloTrabajoSesion) {
			t.Fatal("incomplete metadata config admitted WAL", err)
		}
	}
	if ledger.starts != 0 {
		t.Fatal("invalid config started work")
	}
}

func TestNativeLeaseVerifiesAbsenceBeforeTombstoneAcknowledgement(t *testing.T) {
	work, scope, ledger := engineWorkFixture(t)
	ledger.own = registroTrabajoBroker{Pendientes: 2, Incierto: true, Desecho: "55555555555555555555555555555555"}
	metadata := MetadatosTrabajoNativo{Contar: func(storage.Scope) (int, error) { return 2, nil }, Desechar: func(context.Context, storage.Scope) error { return nil }}
	if release, err := work.ArrendamientoPrivado(scope, metadata)(t.Context()); release != nil || !errors.Is(err, ErrTrabajoNativoPendiente) {
		t.Fatal("partial deletion acknowledged as zero", err)
	}
	if ledger.starts != 0 || ledger.own.DesechoAplicado || ledger.own.Pendientes != 2 {
		t.Fatal("WAL opened despite retained data")
	}
}

func TestNativeLeaseExclusiveRecoveryPreservesActualPendingCount(t *testing.T) {
	work, scope, ledger := engineWorkFixture(t)
	ledger.operations = 1
	metadata := MetadatosTrabajoNativo{Contar: func(storage.Scope) (int, error) { return 5, nil }, Desechar: func(context.Context, storage.Scope) error { t.Fatal("recovery implicitly discarded work"); return nil }}
	if err := work.PrepararAmbito(t.Context(), scope, metadata); err != nil {
		t.Fatal(err)
	}
	if ledger.own.Incierto || ledger.own.Pendientes != 5 {
		t.Fatal("EX recovery fabricated a zero count")
	}
}

func TestNativeCloseNeverDeletesOwnWALBeforeCredentialCommit(t *testing.T) {
	work, scope, ledger := engineWorkFixture(t)
	metadata := MetadatosTrabajoNativo{Contar: func(storage.Scope) (int, error) { return 3, nil }, Desechar: func(context.Context, storage.Scope) error {
		t.Fatal("prepared close erased WAL before revocation succeeded")
		return nil
	}}
	release, err := work.PrepararCierre(t.Context(), []storage.Scope{scope}, metadata, true)
	if err != nil {
		t.Fatal(err)
	}
	release() // The caller's revocation failed; credentials and files remain.
	if ledger.own.Pendientes != 3 || ledger.own.Desecho == "" || ledger.own.DesechoAplicado {
		t.Fatal("uncommitted close changed retained work")
	}
}

func TestNativeLeaseAppliesLaterDiscardWithoutReopeningTheApplication(t *testing.T) {
	work, scope, ledger := engineWorkFixture(t)
	pending := 0
	deleted := 0
	metadata := MetadatosTrabajoNativo{Contar: func(storage.Scope) (int, error) { return pending, nil }, Desechar: func(context.Context, storage.Scope) error { pending = 0; deleted++; return nil }}
	acquire := work.ArrendamientoPrivado(scope, metadata)
	done, err := acquire(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	done()
	pending = 3
	release, err := work.PrepararCierre(t.Context(), []storage.Scope{scope}, metadata, true)
	if err != nil {
		t.Fatal(err)
	}
	release() // Credential revocation failed and the old application resumed.
	if pending != 3 || deleted != 0 {
		t.Fatal("prepared close erased data before revocation")
	}
	done, err = acquire(t.Context())
	if err != nil {
		t.Fatal("retained cycle did not apply its authorized tombstone", err)
	}
	if deleted != 1 || !ledger.own.DesechoAplicado || pending != 0 {
		t.Fatal("resumed WAL preceded tombstone ACK")
	}
	done()
}

func TestNativeCloseIndividualSessionDoesNotInventAStorageTenant(t *testing.T) {
	work, _, _ := engineWorkFixture(t)
	metadata := MetadatosTrabajoNativo{Contar: func(storage.Scope) (int, error) { t.Fatal("tenant invented for session-only logout"); return 0, nil }, Desechar: func(context.Context, storage.Scope) error { t.Fatal("unbound scope discarded"); return nil }}
	release, err := work.PrepararCierre(t.Context(), nil, metadata, false)
	if err != nil {
		t.Fatal(err)
	}
	release()
}
