// SPDX-License-Identifier: Apache-2.0
package ajustes

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

func nativeWorkFixture(t *testing.T, report func(string, pedidoTrabajoBroker) *registroTrabajoBroker) (*TrabajoNativo, storage.Scope, *atomic.Int32) {
	t.Helper()
	p, access := brokerFixture()
	var calls atomic.Int32
	client := readyWorkBroker(func(_ context.Context, action, raw string) (string, error) {
		calls.Add(1)
		var got pedidoTrabajoBroker
		if err := json.Unmarshal([]byte(raw), &got); err != nil {
			return "", err
		}
		state := &registroTrabajoBroker{Incierto: action == "iniciar"}
		if report != nil {
			state = report(action, got)
		}
		return brokerResponse(t, action, got, access, state), nil
	})
	work, native, err := nuevoTrabajoNativo(t.Context(), client, p.Cuenta, access)
	if err != nil || !native {
		t.Fatal(err, native)
	}
	file := filepath.Join(t.TempDir(), "broker-inode")
	work.abrir = func(context.Context, string) (*os.File, error) {
		return os.OpenFile(file, os.O_CREATE|os.O_RDWR, 0o600)
	}
	return work, *p.Ambito, &calls
}

func TestNativeWorkKnownOldAndFailureNeverCreateAUsableHandle(t *testing.T) {
	p, access := brokerFixture()
	for _, old := range []bool{true, false} {
		client := clienteTrabajoBroker{disponible: func() (bool, error) {
			if old {
				return false, nil
			}
			return false, ErrProtocoloTrabajoSesion
		}, pedir: func(context.Context, string, string) (string, error) { t.Fatal("unexpected IO"); return "", nil }}
		work, native, err := nuevoTrabajoNativo(t.Context(), client, p.Cuenta, access)
		if work != nil || native == old || old && err != nil || !old && !errors.Is(err, ErrProtocoloTrabajoSesion) {
			t.Fatal("unsupported or failed broker exposed work handle", native, err)
		}
	}
}

func TestNativeWorkRegistersBeforeIOAndReportsWithoutNewLease(t *testing.T) {
	var begun, finished bool
	work, scope, calls := nativeWorkFixture(t, func(action string, p pedidoTrabajoBroker) *registroTrabajoBroker {
		switch action {
		case "iniciar":
			begun = true
			return &registroTrabajoBroker{Incierto: true}
		case "terminar":
			if !begun || p.Pendientes == nil || *p.Pendientes != 3 || p.Fallo {
				t.Fatal("report changed owner result")
			}
			finished = true
			return &registroTrabajoBroker{Pendientes: 3}
		default:
			return &registroTrabajoBroker{}
		}
	})
	lease, err := work.Arrendar(t.Context(), false)
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Cerrar()
	op, err := work.Iniciar(t.Context(), scope, lease)
	if err != nil || !begun || op == nil {
		t.Fatal("WAL authorized before dirty registration", err)
	}
	before := calls.Load()
	state, err := op.Terminar(t.Context(), 3, false)
	if err != nil || !finished || state.Pendientes != 3 || calls.Load() != before+1 {
		t.Fatal("report acquired another lease or lost its count", err)
	}
}

func TestNativeWorkFailedReportRetainsDirtyInsteadOfZero(t *testing.T) {
	work, scope, _ := nativeWorkFixture(t, func(action string, p pedidoTrabajoBroker) *registroTrabajoBroker {
		if action == "terminar" && (!p.Fallo || p.Pendientes != nil) {
			t.Fatal("failed scan was encoded as successful zero")
		}
		return &registroTrabajoBroker{Incierto: true}
	})
	lease, err := work.Arrendar(t.Context(), false)
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Cerrar()
	op, err := work.Iniciar(t.Context(), scope, lease)
	if err != nil {
		t.Fatal(err)
	}
	state, err := op.Terminar(t.Context(), 0, true)
	if err != nil || !state.Incierto {
		t.Fatal("failed report became clean", err)
	}
}

func TestNativeWorkRejectsMissingForeignExclusiveAndClosedSHBeforeIO(t *testing.T) {
	work, scope, calls := nativeWorkFixture(t, nil)
	other, _, _ := nativeWorkFixture(t, nil)
	for _, lease := range []*ArrendamientoTrabajoNativo{nil, {trabajo: other, soltar: func() {}}, {trabajo: work, exclusivo: true, soltar: func() {}}, {trabajo: work, cerrado: true, soltar: func() {}}} {
		before := calls.Load()
		if _, err := work.Iniciar(t.Context(), scope, lease); !errors.Is(err, ErrProtocoloTrabajoSesion) || calls.Load() != before {
			t.Fatal("invalid lease admitted dirty registration", err)
		}
	}
	lease, err := work.Arrendar(t.Context(), false)
	if err != nil {
		t.Fatal(err)
	}
	op, err := work.Iniciar(t.Context(), scope, lease)
	if err != nil {
		t.Fatal(err)
	}
	lease.Cerrar()
	before := calls.Load()
	if _, err := op.Terminar(t.Context(), 0, false); !errors.Is(err, ErrProtocoloTrabajoSesion) || calls.Load() != before {
		t.Fatal("report after kernel release erased retained work", err)
	}
}

func TestNativeWorkReconciliationAndDiscardRequireExactOpenEX(t *testing.T) {
	work, scope, calls := nativeWorkFixture(t, func(action string, p pedidoTrabajoBroker) *registroTrabajoBroker {
		switch action {
		case "reconciliar":
			return &registroTrabajoBroker{Pendientes: *p.Pendientes}
		case "desechar":
			return &registroTrabajoBroker{Pendientes: 4, Incierto: true, Desecho: "55555555555555555555555555555555", TodoConDesecho: true}
		case "confirmar-desecho":
			return &registroTrabajoBroker{Desecho: p.Desecho, DesechoAplicado: true}
		default:
			return &registroTrabajoBroker{}
		}
	})
	other, _, _ := nativeWorkFixture(t, nil)
	for _, lease := range []*ArrendamientoTrabajoNativo{nil, {trabajo: other, exclusivo: true, permiso: "44444444444444444444444444444444", soltar: func() {}}, {trabajo: work, soltar: func() {}}} {
		before := calls.Load()
		if _, err := work.Reconciliar(t.Context(), scope, lease, 0); !errors.Is(err, ErrProtocoloTrabajoSesion) {
			t.Fatal(err)
		}
		if _, err := work.PedirDesecho(t.Context(), lease); !errors.Is(err, ErrProtocoloTrabajoSesion) {
			t.Fatal(err)
		}
		if _, err := work.ConfirmarDesecho(t.Context(), scope, lease, "55555555555555555555555555555555", 0, false); !errors.Is(err, ErrProtocoloTrabajoSesion) {
			t.Fatal(err)
		}
		if calls.Load() != before {
			t.Fatal("foreign or absent EX proof performed IO")
		}
	}
	lease, err := work.Arrendar(t.Context(), true)
	if err != nil {
		t.Fatal(err)
	}
	if state, err := work.Reconciliar(t.Context(), scope, lease, 2); err != nil || state.Pendientes != 2 {
		t.Fatal(err)
	}
	state, err := work.PedirDesecho(t.Context(), lease)
	if err != nil || state.Pendientes != 4 || !state.Incierto || !state.TodoConDesecho {
		t.Fatal("tombstone request pretended another UID erased WAL", err)
	}
	before := calls.Load()
	if _, err := work.ConfirmarDesecho(t.Context(), scope, lease, state.Desecho, 1, false); !errors.Is(err, ErrProtocoloTrabajoSesion) || calls.Load() != before {
		t.Fatal("cleanup ACK accepted remaining data", err)
	}
	if state, err := work.ConfirmarDesecho(t.Context(), scope, lease, state.Desecho, 0, false); err != nil || !state.DesechoAplicado {
		t.Fatal("verified cleanup was not acknowledged", err)
	}
	lease.Cerrar()
	before = calls.Load()
	if _, err := work.Reconciliar(t.Context(), scope, lease, 0); !errors.Is(err, ErrProtocoloTrabajoSesion) || calls.Load() != before {
		t.Fatal("released EX proof authorized cleanup", err)
	}
}

func TestNativeWorkCloseIsIdempotentWithoutHoldingMutexDuringRelease(t *testing.T) {
	var calls atomic.Int32
	lease := &ArrendamientoTrabajoNativo{}
	lease.soltar = func() {
		lease.mu.Lock()
		defer lease.mu.Unlock()
		calls.Add(1)
		if !lease.cerrado {
			t.Error("release ran before lease was closed")
		}
	}
	var group sync.WaitGroup
	for range 8 {
		group.Go(lease.Cerrar)
	}
	group.Wait()
	if calls.Load() != 1 {
		t.Fatal("kernel description released more than once", calls.Load())
	}
}

func TestNativeWorkPausedRepliesCannotAdmitNewSharedIO(t *testing.T) {
	for _, action := range []string{"confirmar", "iniciar"} {
		p, access := brokerFixture()
		access.Suspendido = true
		if action == "confirmar" {
			p.Ambito = nil
			exclusive := false
			p.Exclusiva = &exclusive
		}
		client := readyWorkBroker(func(context.Context, string, string) (string, error) {
			return brokerResponse(t, action, p, access, &registroTrabajoBroker{Incierto: true}), nil
		})
		if _, _, err := client.aplicar(t.Context(), action, p); !errors.Is(err, ErrAccesoSesion) {
			t.Fatal("paused native reply admitted new SH work", action, err)
		}
	}
}
