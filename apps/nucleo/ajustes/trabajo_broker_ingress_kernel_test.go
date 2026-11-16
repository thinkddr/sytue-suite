// SPDX-License-Identifier: Apache-2.0
//go:build linux

package ajustes

import (
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

// Signing retains a second genuine shared kernel lease while the ingress
// worker owns its first lease. A concurrent logout writer must not turn this
// into a recursive-reader deadlock, nor acquire until both leases are gone.
func TestWorkBrokerIngressNestedSigningLeaseWhileLogoutWriterWaits(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ingress-session-inode")
	opened := make(chan struct{}, 3)
	open := func(context.Context, string) (*os.File, error) {
		f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
		if err == nil {
			opened <- struct{}{}
		}
		return f, err
	}
	var confirmations atomic.Int32
	client := kernelBroker(t, &confirmations)
	parent, _, err := client.cerrojo(t.Context(), kernelRequest(false), open)
	if err != nil {
		t.Fatal(err)
	}
	defer parent()
	<-opened
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	writer := make(chan func(), 1)
	failed := make(chan error, 1)
	go func() {
		release, _, err := client.cerrojo(ctx, kernelRequest(true), open)
		if err != nil {
			failed <- err
			return
		}
		writer <- release
	}()
	<-opened
	select {
	case release := <-writer:
		release()
		t.Fatal("logout passed active worker lease")
	case err := <-failed:
		t.Fatal(err)
	case <-time.After(35 * time.Millisecond):
	}
	nestedCtx, nestedCancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer nestedCancel()
	child, _, err := client.cerrojo(nestedCtx, kernelRequest(false), open)
	if err != nil {
		t.Fatal("SDK signing lease blocked behind logout while its parent lease remained held", err)
	}
	defer child()
	<-opened
	if confirmations.Load() != 2 {
		t.Fatal("nested signing did not retain a genuine confirmed kernel lease", confirmations.Load())
	}
	parent()
	select {
	case release := <-writer:
		release()
		t.Fatal("logout passed live SDK signing child lease")
	case err := <-failed:
		t.Fatal(err)
	case <-time.After(35 * time.Millisecond):
	}
	child()
	select {
	case release := <-writer:
		release()
	case err := <-failed:
		t.Fatal(err)
	case <-ctx.Done():
		t.Fatal("logout did not acquire after both real leases released")
	}
	if confirmations.Load() != 3 {
		t.Fatal("writer or nested signer skipped broker confirmation", confirmations.Load())
	}
}
