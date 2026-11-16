// SPDX-License-Identifier: Apache-2.0
package matrix

import (
	"context"
	"errors"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

func TestIngressBackupMutexCancellationDoesNotWaitForForeignIO(t *testing.T) {
	c := &Client{}
	c.backupMu.Lock()
	defer c.backupMu.Unlock()
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() {
		release, err := c.lockBackup(ctx)
		if release != nil {
			release()
		}
		done <- err
	}()
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatal("cancel did not interrupt mutex wait", err)
		}
	case <-time.After(time.Second):
		t.Fatal("cancel blocked behind unrelated backup")
	}
	for _, call := range []func(context.Context) error{c.Backup, c.RestoreBackup} {
		if err := call(ctx); !errors.Is(err, context.Canceled) {
			t.Fatal("backup path ignored cancellation", err)
		}
	}
}

func TestIngressContextIsBoundToExactClientScopeRoomAndDevice(t *testing.T) {
	scope := storage.Scope{Tenant: "acme", Subject: "alice", Product: "mail"}
	c := &Client{cfg: Config{Subject: scope.Subject, Tenant: scope.Tenant, Bootstrap: Bootstrap{StorageProduct: scope.Product, StorageIdentityVersion: storage.IsolatedIdentity, StorageRoomID: "!inbox:matrix.test", StorageRoomIDs: []string{"!inbox:matrix.test"}}}}
	c.session.DeviceID = "OWNER_DEVICE"
	target := storage.PublicationTarget{RoomID: "!inbox:matrix.test", DeviceID: "OWNER_DEVICE"}
	proof := storagePublication{client: c, scope: scope, target: target, identity: storage.IsolatedIdentity}
	ctx := context.WithValue(t.Context(), storagePublicationContext{}, proof)
	if err := c.validateStoragePublication(ctx, target.RoomID); err != nil {
		t.Fatal(err)
	}
	for _, fault := range []string{"client", "subject", "tenant", "product", "room", "device", "identity-version"} {
		changed := proof
		switch fault {
		case "client":
			changed.client = &Client{}
		case "subject":
			changed.scope.Subject = "other"
		case "tenant":
			changed.scope.Tenant = "other"
		case "product":
			changed.scope.Product = "calendar"
		case "room":
			changed.target.RoomID = "!other:matrix.test"
		case "device":
			changed.target.DeviceID = "OTHER_DEVICE"
		case "identity-version":
			changed.identity = storage.LegacyIdentity
		}
		bad := context.WithValue(t.Context(), storagePublicationContext{}, changed)
		if err := c.validateStoragePublication(bad, target.RoomID); !errors.Is(err, ErrIdentity) {
			t.Fatal("unbound ingress context accepted", fault, err)
		}
	}
}
