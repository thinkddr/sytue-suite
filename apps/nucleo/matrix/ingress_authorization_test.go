// SPDX-License-Identifier: Apache-2.0
package matrix

import (
	"context"
	"errors"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
)

func TestIngressSignatureRequiresPersistentLeaseBeforeSDKAccess(t *testing.T) {
	scope := storage.Scope{Tenant: "fixture", Subject: "owner", Product: "mail"}
	target := storage.PublicationTarget{RoomID: "!private:matrix.test", DeviceID: "OWNER"}
	intent, err := matrixproof.IntentFor(scope, 1, "11111111-1111-4111-8111-111111111111", "key.enroll", target, map[string]string{"recipient": "synthetic"})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	action := matrixproof.Action{Version: 1, Generation: 1, Scope: scope, Epoch: intent.Epoch, Kind: intent.Kind, Target: target, RootKey: "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA", RequestSHA256: intent.RequestSHA256, Nonce: "challenge", IssuedAt: now.UnixMilli(), ExpiresAt: now.Add(time.Minute).UnixMilli()}
	c := &Client{}
	if _, err := c.SignIngressAction(t.Context(), nil, intent, action); !errors.Is(err, offline.ErrWorkLease) {
		t.Fatal("missing lease reached SDK", err)
	}
	denied := errors.New("synthetic revoked session witness")
	lease := func(context.Context) (func(), error) { return nil, denied }
	if _, err := c.SignIngressAction(t.Context(), lease, intent, action); !errors.Is(err, denied) {
		t.Fatal("revoked witness reached SDK", err)
	}
	calls := 0
	lease = func(context.Context) (func(), error) { return func() { calls++ }, nil }
	if _, err := c.SignIngressAction(t.Context(), lease, intent, action); !errors.Is(err, ErrIdentity) || calls != 1 {
		t.Fatal("failed SDK admission retained lease", err, calls)
	}
}

func TestStorageRootConsentRequiresPersistentLeaseBeforeSDKAccess(t *testing.T) {
	c := &Client{}
	scope := storage.Scope{Tenant: "acme", Subject: "owner", Product: "mail"}
	target := storage.PublicationTarget{RoomID: "!private:matrix.test", DeviceID: "DEVICE"}
	if _, err := c.StorageRootConsentRequest(t.Context(), nil, scope, target, "root.enroll", nil); !errors.Is(err, offline.ErrWorkLease) {
		t.Fatal("missing root-consent lease reached SDK", err)
	}
	denied := errors.New("synthetic revoked session witness")
	lease := func(context.Context) (func(), error) { return nil, denied }
	if _, err := c.StorageRootConsentRequest(t.Context(), lease, scope, target, "root.enroll", nil); !errors.Is(err, denied) {
		t.Fatal("revoked root-consent lease reached SDK", err)
	}
}
