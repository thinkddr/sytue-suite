// SPDX-License-Identifier: Apache-2.0
package agecipher

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"testing"

	"filippo.io/age"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress"
)

func fixturePlan(t *testing.T) (ingress.KeyPlan, *age.X25519Identity) {
	t.Helper()
	identity, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatal(err)
	}
	public := identity.Recipient().String()
	return ingress.KeyPlan{Version: 1, Scope: storage.Scope{Tenant: "artificial", Subject: "owner", Product: "mail"}, Epoch: "11111111-1111-4111-8111-111111111111", Generation: 1, KeyID: ingress.PublicFingerprint(public), Profile: ingress.Profile, Recipient: public}, identity
}

func fixtureCipher(t *testing.T) (ingress.Task, *age.X25519Identity, []byte, []byte, []byte) {
	t.Helper()
	plan, identity := fixturePlan(t)
	metadata := []byte(`{"mailbox_id":"private-inbox","future_integer":9007199254740993}`)
	body := bytes.Repeat([]byte("artificial SMTP body, retained exactly / 漢字 é\r\n"), 50000)
	var encrypted bytes.Buffer
	task, err := Seal(t.Context(), &encrypted, plan, "delivery_01", "mail.receive", metadata, bytes.NewReader(body), int64(len(body)))
	if err != nil {
		t.Fatal(err)
	}
	return task, identity, encrypted.Bytes(), body, metadata
}

func TestAgeIngressStreamsExactBodyAndMetadataWithMaintainedReaderAt(t *testing.T) {
	task, identity, encrypted, body, metadata := fixtureCipher(t)
	if bytes.Contains(encrypted, []byte("artificial SMTP body")) || bytes.Contains(encrypted, []byte("private-inbox")) || bytes.Contains(encrypted, []byte("9007199254740993")) {
		t.Fatal("ingress persisted plaintext")
	}
	actual, reader, err := Open(t.Context(), bytes.NewReader(encrypted), task, identity)
	if err != nil {
		t.Fatal(err)
	}
	defer clear(actual)
	if !bytes.Equal(actual, metadata) || reader.Size() != int64(len(body)) {
		t.Fatal("private frame changed")
	}
	for _, offset := range []int64{0, 65530, 1 << 20, int64(len(body) - 200)} {
		var block [200]byte
		n, err := reader.ReadAt(block[:], offset)
		if err != nil || n != len(block) || !bytes.Equal(block[:], body[offset:offset+int64(n)]) {
			t.Fatal("random-access body changed", offset, err)
		}
	}
	digest := sha256.New()
	if _, err := io.Copy(digest, reader); err != nil {
		t.Fatal(err)
	}
	expected := sha256.Sum256(body)
	if !bytes.Equal(digest.Sum(nil), expected[:]) {
		t.Fatal("streamed MIME changed")
	}
}

func TestAgeIngressRejectsRealCipherCorruptionTruncationAndChangedBinding(t *testing.T) {
	task, identity, encrypted, _, _ := fixtureCipher(t)
	for _, fault := range []string{"cipher-corruption", "cipher-truncation", "cipher-trailing", "scope", "epoch", "key-generation", "task-id"} {
		t.Run(fault, func(t *testing.T) {
			input := bytes.Clone(encrypted)
			changed := task
			switch fault {
			case "cipher-corruption":
				input[len(input)/2] ^= 1
			case "cipher-truncation":
				input = input[:len(input)-1]
			case "cipher-trailing":
				input = append(input, 0)
			case "scope":
				changed.Scope.Subject = "other"
			case "epoch":
				changed.Epoch = "22222222-2222-4222-8222-222222222222"
			case "key-generation":
				changed.KeyGeneration++
			case "task-id":
				changed.ID = "other_delivery"
				changed.Cipher.ID = changed.ID
			}
			// Recompute the public descriptor: ciphertext hashing alone cannot
			// protect against substitution or replace the maintained AEAD checks.
			sum := sha256.Sum256(input)
			changed.Cipher.SHA256 = hex.EncodeToString(sum[:])
			changed.Cipher.Size = int64(len(input))
			metadata, reader, err := Open(t.Context(), bytes.NewReader(input), changed, identity)
			if err == nil || metadata != nil || reader != nil {
				t.Fatal("invalid input reached executor", fault)
			}
		})
	}
}

type inputSpy struct{ reads int }

func (s *inputSpy) ReadAt([]byte, int64) (int, error) { s.reads++; return 0, io.EOF }

func TestAgeIngressDeniesWrongKeyAndCancelledWorkBeforeIO(t *testing.T) {
	task, _, _, _, _ := fixtureCipher(t)
	_, wrong := fixturePlan(t)
	input := &inputSpy{}
	if _, _, err := Open(t.Context(), input, task, wrong); err == nil || input.reads != 0 {
		t.Fatal("wrong key reached source")
	}
	plan, key := fixturePlan(t)
	task.KeyID = plan.KeyID
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, _, err := Open(ctx, input, task, key); !errors.Is(err, context.Canceled) || input.reads != 0 {
		t.Fatal("cancelled job reached source", err)
	}
	var out bytes.Buffer
	if _, err := Seal(ctx, &out, plan, "delivery", "mail.receive", []byte(`{}`), bytes.NewReader(nil), 0); !errors.Is(err, context.Canceled) || out.Len() != 0 {
		t.Fatal("cancelled sealing wrote a spool", err)
	}
}

func TestAgeIngressRejectsShortOrExtraOriginalWithoutACommittedDescriptor(t *testing.T) {
	plan, _ := fixturePlan(t)
	for _, size := range []int64{2, 4} {
		var out bytes.Buffer
		task, err := Seal(t.Context(), &out, plan, "delivery", "mail.receive", []byte(`{}`), bytes.NewReader([]byte("abc")), size)
		if err == nil || task != (ingress.Task{}) {
			t.Fatal("incomplete input became a durable delivery")
		}
	}
}
