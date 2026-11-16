// SPDX-License-Identifier: Apache-2.0
package matrix

import (
	"context"
	"errors"
	"testing"
	"time"

	"maunium.net/go/mautrix/event"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

func TestStoragePublicationBindingPreservesPolicyAndRejectsRebinding(t *testing.T) {
	scope := storage.Scope{Tenant: "acme", Subject: "alice", Product: "photos"}
	c := &Client{cfg: Config{Subject: scope.Subject, Tenant: scope.Tenant, Bootstrap: Bootstrap{StorageProduct: scope.Product, StorageIdentityVersion: storage.IsolatedIdentity, StorageRoomID: "!photos:matrix.test", StorageRoomIDs: []string{"!photos:matrix.test"}}}}
	c.session.DeviceID = "DEVICE"
	target := storage.PublicationTarget{RoomID: "!photos:matrix.test", DeviceID: "DEVICE"}
	ctx, err := c.bindStoragePublication(t.Context(), scope, target)
	if err != nil {
		t.Fatal(err)
	}
	proof := ctx.Value(storagePublicationContext{}).(storagePublication)
	calls := 0
	denied := errors.New("authority revoked")
	proof.policy = func(context.Context) error { calls++; return denied }
	ctx = context.WithValue(ctx, storagePublicationContext{}, proof)
	again, err := c.bindStoragePublication(ctx, scope, target)
	if err != nil {
		t.Fatal(err)
	}
	if err := storagePublicationPolicy(again); !errors.Is(err, denied) || calls != 1 {
		t.Fatal("binding removed the original authorization", err, calls)
	}
	for _, change := range []string{"scope", "device", "client", "version"} {
		bad := proof
		switch change {
		case "scope":
			bad.scope.Subject = "bob"
		case "device":
			bad.target.DeviceID = "OTHER"
		case "client":
			bad.client = &Client{}
		case "version":
			bad.identity = storage.LegacyIdentity
		}
		if _, err := c.bindStoragePublication(context.WithValue(t.Context(), storagePublicationContext{}, bad), scope, target); !errors.Is(err, ErrIdentity) {
			t.Fatal("rebound captured publication", change, err)
		}
	}
}

func TestStoragePublicationPolicyRunsBeforeStateLockOrCryptoIO(t *testing.T) {
	c := &Client{}
	denied := errors.New("authority revoked")
	ctx := context.WithValue(t.Context(), storagePublicationContext{}, storagePublication{policy: func(context.Context) error {
		c.stateMu.Lock()
		defer c.stateMu.Unlock()
		return denied
	}})
	done := make(chan error, 1)
	go func() {
		_, err := c.sendEventWithTxn(ctx, "!private:matrix.test", event.EventMessage, nil, "txn")
		done <- err
	}()
	select {
	case err := <-done:
		if !errors.Is(err, denied) {
			t.Fatal("policy was not consulted before crypto", err)
		}
	case <-time.After(time.Second):
		t.Fatal("authorization callback was invoked under the state lock")
	}
}
