// SPDX-License-Identifier: Apache-2.0
package webport

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	worker "git.sytue.eu/sytue/one/apps/nucleo/ingress"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/agecipher"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/keyring"
)

var (
	_ worker.Device           = (*Storage)(nil)
	_ worker.EnrollmentDevice = (*Storage)(nil)
	_ worker.HTTPActionSigner = (*Storage)(nil)
)

func TestIngressDeviceUsesSharedHybridKeyringForFullSizeAuthorizedBody(t *testing.T) {
	identity := fixtureIdentity()
	identity.Binding.MatrixUserID = storage.IsolatedMatrixUserPrefix(identity.Scope) + "test:matrix.test"
	binding := storage.Binding{Scope: identity.Scope, Target: storage.PublicationTarget{RoomID: "!private:test", DeviceID: identity.Binding.DeviceID}, Rooms: []string{"!private:test"}}
	secrets := map[string][]byte{}
	writes := 0
	recoverable := false
	t.Cleanup(func() {
		for _, raw := range secrets {
			clear(raw)
		}
	})
	port := storagePort{sourcePort: sourcePort{id: strings.Repeat("a", 64), identity: identity}, handle: func(method Method, raw []byte) ([]byte, error) {
		switch method {
		case StoragePrepare, StorageBindings:
			return json.Marshal(binding)
		case StorageRecoverable:
			if !recoverable {
				return nil, ErrRejected
			}
			return nil, nil
		case IngressSecretWrite:
			var value struct {
				Name  string
				Value []byte
			}
			if json.Unmarshal(raw, &value) != nil {
				return nil, ErrRejected
			}
			writes++
			secrets[value.Name] = bytes.Clone(value.Value)
			clear(value.Value)
			return nil, nil
		case IngressSecretRead:
			var value struct{ Name string }
			if json.Unmarshal(raw, &value) != nil {
				return nil, ErrRejected
			}
			if saved, found := secrets[value.Name]; found {
				return append([]byte{1}, saved...), nil
			}
			return []byte{0}, nil
		default:
			return nil, ErrRejected
		}
	}}
	device, err := OpenStorage(t.Context(), port, NewSources())
	if err != nil {
		t.Fatal(err)
	}
	epoch := "018f3340-7b12-7000-8000-000000000001"
	if _, err = device.CreateIngressKey(t.Context(), identity.Scope, epoch, 1); err == nil || writes != 0 {
		t.Fatal("unrecoverable host generated a key", err)
	}
	recoverable = true
	plan, err := device.CreateIngressKey(t.Context(), identity.Scope, epoch, 1)
	if err != nil || plan.Profile != "age-MLKEM768-X25519-v1" || writes != 1 {
		t.Fatal("hybrid candidate changed", err, plan.Profile, writes)
	}
	actual, err := device.RecoverIngressEnrollment(t.Context(), plan)
	if err != nil || actual != plan {
		t.Fatal("same keyring recovery changed", err)
	}
	metadata := []byte(`{"subject":"original","integer":900719925474099312345}`)
	body := bytes.Repeat([]byte("MIME exact\r\n"), 100000)
	var cipher bytes.Buffer
	task, err := agecipher.Seal(t.Context(), &cipher, plan, "device-full-body", "mail.receive", metadata, bytes.NewReader(body), int64(len(body)))
	if err != nil {
		t.Fatal(err)
	}
	restored, reader, err := device.OpenIngressBody(t.Context(), task, bytes.NewReader(cipher.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	original, err := io.ReadAll(reader)
	if err != nil || !bytes.Equal(restored, metadata) || !bytes.Equal(original, body) {
		t.Fatal("full-size original changed", err)
	}
	clear(original)
	clear(restored)
	clear(body)
	clear(cipher.Bytes())
	for name, raw := range secrets {
		clear(raw)
		delete(secrets, name)
	}
	if _, err = device.RecoverIngressEnrollment(t.Context(), plan); !errors.Is(err, keyring.ErrRecoveryRequired) || writes != 1 {
		t.Fatal("missing recovery generated replacement", err)
	}
}

func TestIngressDeviceRejectsLegacyBeforeSecretOrKeyIO(t *testing.T) {
	identity := fixtureIdentity()
	calls := 0
	port := storagePort{sourcePort: sourcePort{id: "port", identity: identity}, handle: func(Method, []byte) ([]byte, error) { calls++; return nil, ErrRejected }}
	device, err := OpenStorage(context.Background(), port, NewSources())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = device.CreateIngressKey(t.Context(), identity.Scope, "018f3340-7b12-7000-8000-000000000001", 1); !errors.Is(err, ErrRejected) || calls != 0 {
		t.Fatal("legacy identity reached ingress key IO", err, calls)
	}
}
