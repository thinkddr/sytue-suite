// SPDX-License-Identifier: Apache-2.0
package cifrado

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	mx "git.sytue.eu/sytue/one/apps/nucleo/matrix"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
)

func TestPanelWaitClosedJoinsApplicationDrainBeforeStoreReuse(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	t.Cleanup(func() {
		select {
		case <-release:
		default:
			close(release)
		}
	})
	panel := &Panel{AlCerrar: func() func() { return func() { close(entered); <-release } }}
	panel.Cerrar()
	<-entered
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := panel.WaitClosed(ctx); !errors.Is(err, context.Canceled) {
		t.Fatal("cancelled waiter did not return", err)
	}
	done := make(chan error, 1)
	go func() { done <- panel.WaitClosed(t.Context()) }()
	select {
	case err := <-done:
		t.Fatal("SDK store could reopen before its application drained", err)
	case <-time.After(30 * time.Millisecond):
	}
	close(release)
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("completed drain did not release store")
	}
}

func TestPanelWaitClosedClosesSDKWaitingForItsUICallback(t *testing.T) {
	client := &mx.Client{}
	panel := &Panel{clients: map[*mx.Client]bool{client: true}}
	panel.Cerrar()
	if err := panel.WaitClosed(t.Context()); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	if err := client.WaitReady(ctx); !errors.Is(err, mx.ErrClosed) {
		t.Fatal("uninstalled SDK retained its native store after shutdown", err)
	}
}

func TestPanelRetireUnregistersDetachedWorkOnce(t *testing.T) {
	var unregistered atomic.Int32
	worker := &PanelWorkers{finite: offline.NewWorkGate(t.Context()), syncer: offline.NewWorkGate(t.Context()), unregister: func() { unregistered.Add(1) }}
	panel := &Panel{}
	panel.workers.Store(worker)
	panel.Cerrar()
	if err := panel.Retire(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := panel.Retire(t.Context()); err != nil {
		t.Fatal(err)
	}
	if unregistered.Load() != 1 || panel.workers.Load() != nil {
		t.Fatal("a retired view retained its private work registration")
	}
}
