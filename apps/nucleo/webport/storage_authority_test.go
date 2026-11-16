// SPDX-License-Identifier: Apache-2.0
package webport

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
)

func TestAuthorityRequiresOwnVerifiedSDKRootAndActualIsolatedIdentity(t *testing.T) {
	identity := fixtureIdentity()
	identity.Binding.MatrixUserID = storage.IsolatedMatrixUserPrefix(identity.Scope) + "owner:matrix.test"
	public, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	root := base64.RawStdEncoding.EncodeToString(public)
	authority := matrixproof.Authority{Version: 1, Scope: identity.Scope, UserID: identity.Binding.MatrixUserID, RootKey: root, Generation: 1, Status: "active"}
	reads := 0
	port := storagePort{sourcePort: sourcePort{id: strings.Repeat("a", 64), identity: identity}, handle: func(method Method, _ []byte) ([]byte, error) {
		switch method {
		case StoragePrepare:
			return json.Marshal(storage.Binding{Scope: identity.Scope, Target: storage.PublicationTarget{RoomID: "!own:test", DeviceID: identity.Binding.DeviceID}, Rooms: []string{"!own:test"}})
		case StorageRecoverable:
			return nil, nil
		case IdentityRoot:
			reads++
			return json.Marshal(map[string]string{"root_key": root})
		default:
			return nil, ErrRejected
		}
	}}
	owner, _ := OpenStorage(t.Context(), port, NewSources())
	if err = owner.RequireStorageAuthority(t.Context(), authority); err != nil || reads != 1 {
		t.Fatal("own root rejected", err)
	}
	other := authority
	other.UserID = storage.IsolatedMatrixUserPrefix(identity.Scope) + "foreign:matrix.test"
	if err = owner.RequireStorageAuthority(t.Context(), other); !errors.Is(err, ErrRejected) || reads != 1 {
		t.Fatal("foreign SDK user authority adopted", err)
	}
	other = authority
	other.Status = "revoked"
	if err = owner.RequireStorageAuthority(t.Context(), other); !errors.Is(err, ErrRejected) {
		t.Fatal("revoked authority adopted", err)
	}
	next, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	root = base64.RawStdEncoding.EncodeToString(next)
	if err = owner.RequireStorageAuthority(t.Context(), authority); !errors.Is(err, ErrRejected) {
		t.Fatal("host root swap adopted server root", err)
	}
}
