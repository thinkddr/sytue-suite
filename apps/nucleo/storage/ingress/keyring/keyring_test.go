// SPDX-License-Identifier: Apache-2.0
package keyring

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"testing"

	"filippo.io/age"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/agecipher"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/pqc"
)

type testSecrets struct {
	values  map[string][]byte
	writes  int
	missing error
	replace func([]byte) []byte
}

func (s *testSecrets) Read(_ context.Context, name string) ([]byte, error) {
	value, found := s.values[name]
	if !found {
		return nil, s.missing
	}
	if s.replace != nil {
		return s.replace(bytes.Clone(value)), nil
	}
	return bytes.Clone(value), nil
}

func (s *testSecrets) Write(_ context.Context, name string, raw []byte) error {
	s.writes++
	s.values[name] = bytes.Clone(raw)
	return nil
}

func fixtureScope() storage.Scope {
	return storage.Scope{Tenant: "tenant", Subject: "owner", Product: "mail"}
}

const fixtureEpoch = "00000000-0000-4000-8000-000000000001"

func TestSharedIngressKeyringKeepsRawSSSSAndExactHybridBody(t *testing.T) {
	secrets := &testSecrets{values: map[string][]byte{}, missing: errors.New("missing SSSS")}
	scope := fixtureScope()
	plan, err := Create(t.Context(), secrets, scope, fixtureEpoch, 1)
	if err != nil || plan.Profile != pqc.HybridProfile {
		t.Fatal("hybrid recipient not enrolled", err)
	}
	name := SecretName(scope, fixtureEpoch, 1, plan.KeyID)
	raw := secrets.values[name]
	// Compatibility: native SSSS stores a JSON object directly, never a quoted
	// base64 value or the separate org.sytue.vault envelope.
	var original map[string]json.RawMessage
	if json.Unmarshal(raw, &original) != nil || len(original) != 6 || string(original["v"]) != "1" || original["identity"] == nil || original["payload"] != nil {
		t.Fatal("native SSSS wire changed")
	}
	restored, err := Recover(t.Context(), secrets, plan)
	if err != nil || restored != plan || secrets.writes != 1 {
		t.Fatal("recovery changed key or wrote another candidate", err)
	}
	body := bytes.Repeat([]byte("Exact MIME\r\nPrivate: 9007199254740993\r\n"), 4096)
	metadata := []byte(`{"from":"sender@example.test","integer":900719925474099312345}`)
	var encrypted bytes.Buffer
	task, err := agecipher.Seal(t.Context(), &encrypted, plan, "delivery", "mail.receive", metadata, bytes.NewReader(body), int64(len(body)))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(encrypted.Bytes(), body) || bytes.Contains(encrypted.Bytes(), metadata) {
		t.Fatal("private data in ciphertext")
	}
	got, reader, err := Open(t.Context(), secrets, task, bytes.NewReader(encrypted.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	defer clear(got)
	actual, err := io.ReadAll(reader)
	defer clear(actual)
	if err != nil || !bytes.Equal(got, metadata) || !bytes.Equal(actual, body) {
		t.Fatal("shared ingress changed original", err)
	}
}

func TestSharedIngressRecoveryNeverManufacturesMissingOrChangedKeys(t *testing.T) {
	missing := errors.New("recovery required")
	secrets := &testSecrets{values: map[string][]byte{}, missing: missing}
	plan, err := Create(t.Context(), secrets, fixtureScope(), fixtureEpoch, 1)
	if err != nil {
		t.Fatal(err)
	}
	writes := secrets.writes
	other := plan
	other.Generation++
	if _, err = Recover(t.Context(), secrets, other); !errors.Is(err, missing) || secrets.writes != writes {
		t.Fatal("missing recovery generated a replacement")
	}
	for _, field := range []string{"v", "scope", "epoch", "generation", "key_id", "identity"} {
		t.Run(field, func(t *testing.T) {
			secrets.replace = func(raw []byte) []byte {
				defer clear(raw)
				var value map[string]json.RawMessage
				_ = json.Unmarshal(raw, &value)
				value[field] = json.RawMessage(`"foreign"`)
				result, _ := json.Marshal(value)
				return result
			}
			if _, err := Recover(t.Context(), secrets, plan); err == nil || secrets.writes != writes {
				t.Fatal("substituted secret accepted")
			}
		})
	}
	secrets.replace = nil
	changed := plan
	changed.Recipient = plan.Recipient + "x"
	if _, err = Recover(t.Context(), secrets, changed); !errors.Is(err, ingress.ErrContract) {
		t.Fatal("changed recipient accepted")
	}
}

func TestSharedIngressExplicitHistoricalProfileStillRecoversWithoutUpgrade(t *testing.T) {
	identity, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatal(err)
	}
	scope := fixtureScope()
	plan := ingress.KeyPlan{Version: 1, Scope: scope, Epoch: fixtureEpoch, Generation: 1, Profile: pqc.LegacyProfile, Recipient: identity.Recipient().String()}
	plan.KeyID = ingress.PublicFingerprint(plan.Recipient)
	raw, _ := json.Marshal(Secret{Version: 1, Scope: scope, Epoch: fixtureEpoch, Generation: 1, KeyID: plan.KeyID, Identity: identity.String()})
	defer clear(raw)
	secrets := &testSecrets{values: map[string][]byte{SecretName(scope, fixtureEpoch, 1, plan.KeyID): raw}}
	actual, err := Recover(t.Context(), secrets, plan)
	if err != nil || actual != plan || secrets.writes != 0 {
		t.Fatal("explicit historical recipient was replaced", err)
	}
}
