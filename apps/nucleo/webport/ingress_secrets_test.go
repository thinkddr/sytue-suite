// SPDX-License-Identifier: Apache-2.0
package webport

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/keyring"
)

func TestIngressSecretsUseCentralIsolatedOwnerAndExactRawJSON(t *testing.T) {
	identity := fixtureIdentity()
	legacy, err := OpenStorage(t.Context(), sourcePort{id: "legacy", identity: identity}, NewSources())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = legacy.IngressSecrets(); !errors.Is(err, ErrRejected) {
		t.Fatal("legacy port got ingress key access")
	}
	identity.Binding.MatrixUserID = storage.IsolatedMatrixUserPrefix(identity.Scope) + "example.test"
	name := "org.sytue.ingress.key.v1." + string(bytes.Repeat([]byte("a"), 64))
	original := []byte(`{"v":1,"exact":900719925474099312345}`)
	var saved []byte
	port := storagePort{sourcePort: sourcePort{id: "isolated", identity: identity}, handle: func(method Method, raw []byte) ([]byte, error) {
		switch method {
		case IngressSecretWrite:
			var input struct {
				Name  string
				Value []byte
			}
			if json.Unmarshal(raw, &input) != nil || input.Name != name {
				return nil, ErrRejected
			}
			saved = bytes.Clone(input.Value)
			clear(input.Value)
			return nil, nil
		case IngressSecretRead:
			if saved == nil {
				return []byte{0}, nil
			}
			return append([]byte{1}, saved...), nil
		default:
			return nil, ErrRejected
		}
	}}
	repository, err := OpenStorage(t.Context(), port, NewSources())
	if err != nil {
		t.Fatal(err)
	}
	secrets, err := repository.IngressSecrets()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = secrets.Read(t.Context(), name); !errors.Is(err, keyring.ErrRecoveryRequired) {
		t.Fatal("missing recovery was hidden")
	}
	if err = secrets.Write(t.Context(), name, original); err != nil {
		t.Fatal(err)
	}
	actual, err := secrets.Read(t.Context(), name)
	if err != nil || !bytes.Equal(original, actual) || !bytes.Equal(original, saved) {
		t.Fatal("SSSS JSON was re-encoded", err)
	}
	clear(actual)
	clear(saved)
	if err = secrets.Write(t.Context(), "org.sytue.vault.other", original); !errors.Is(err, ErrRejected) {
		t.Fatal("other SSSS namespace admitted")
	}
}

type changedIdentityPort struct {
	identity Identity
	changed  bool
	returned []byte
}

func (p *changedIdentityPort) ID() string { return "port" }
func (p *changedIdentityPort) Current(context.Context) (Identity, error) {
	identity := p.identity
	if p.changed {
		identity.Binding.DeviceID = "OTHER"
	}
	return identity, nil
}

func (p *changedIdentityPort) Call(context.Context, storage.Scope, Method, []byte) ([]byte, error) {
	p.changed = true
	p.returned = []byte("PRIVATE-LATE")
	return p.returned, nil
}

func TestOwnedStorageOutputErasedIfCentralDeviceChangesDuringIO(t *testing.T) {
	port := &changedIdentityPort{identity: fixtureIdentity()}
	repository, err := OpenStorage(t.Context(), port, NewSources())
	if err != nil {
		t.Fatal(err)
	}
	if raw, err := repository.callOwned(t.Context(), StorageBindings, nil); raw != nil || !errors.Is(err, ErrRejected) {
		t.Fatal("changed owner returned IO")
	}
	if !bytes.Equal(port.returned, make([]byte, len(port.returned))) {
		t.Fatal("rejected raw output was not erased")
	}
}
