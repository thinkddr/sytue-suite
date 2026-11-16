// SPDX-License-Identifier: Apache-2.0
package objectstore

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"filippo.io/age"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/agecipher"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
)

type privateIngressObjectsFixture struct {
	body             []byte
	key              string
	corrupt, failPut bool
}

type blockedIngressInput struct {
	started, closed chan struct{}
	start, close    sync.Once
}

func (b *blockedIngressInput) Read([]byte) (int, error) {
	b.start.Do(func() { close(b.started) })
	<-b.closed
	return 0, io.ErrClosedPipe
}
func (b *blockedIngressInput) Close() error { b.close.Do(func() { close(b.closed) }); return nil }

func TestPrivateIngressCancellationInterruptsBlockedInputAndRemovesCipherSpool(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("TMPDIR", dir)
	identity, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatal(err)
	}
	plan := ingress.KeyPlan{Version: 1, Scope: storage.Scope{Tenant: "acme", Subject: "alice", Product: "calendar"}, Epoch: "11111111-1111-4111-8111-111111111111", Generation: 1, Profile: ingress.Profile, Recipient: identity.Recipient().String()}
	plan.KeyID = ingress.PublicFingerprint(plan.Recipient)
	ctx, cancel := context.WithTimeout(tenancy.Into(t.Context(), tenancy.Tenant{Slug: plan.Scope.Tenant}), 5*time.Second)
	defer cancel()
	source := &blockedIngressInput{started: make(chan struct{}), closed: make(chan struct{})}
	done := make(chan error, 1)
	objects := &privateIngressObjectsFixture{}
	go func() {
		_, _, err := SealPrivateIngress(ctx, objects, plan, "task_01", "calendar.receive", []byte(`{}`), source, 10)
		done <- err
	}()
	select {
	case <-source.started:
	case <-ctx.Done():
		t.Fatal("cipher input never started", ctx.Err())
	}
	cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("cancelled blocked input became a durable task")
		}
	case <-time.After(time.Second):
		_ = source.Close()
		t.Fatal("cancellation left blocked ingress running")
	}
	if objects.key != "" {
		t.Fatal("cancelled input reached object storage")
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 0 {
		t.Fatal("cancelled input left ciphertext spool", err)
	}
}

func (f *privateIngressObjectsFixture) Put(_ context.Context, _ tenancy.Tenant, key string, source io.Reader, size int64, options PutOptions) error {
	if f.failPut {
		return errors.New("synthetic unavailable object store")
	}
	if options.ContentType != "application/octet-stream" || options.Comprimir {
		return ingress.ErrContract
	}
	file, ok := source.(*os.File)
	if !ok {
		return errors.New("expected ciphertext-only bounded spool")
	}
	stat, err := file.Stat()
	if err != nil {
		return err
	}
	if stat.Mode().Perm() != 0o600 {
		return errors.New("ciphertext spool is not owner-only")
	}
	raw, err := io.ReadAll(io.LimitReader(source, size+1))
	if err != nil {
		return err
	}
	if int64(len(raw)) != size {
		return ingress.ErrContract
	}
	if !bytes.HasPrefix(raw, []byte("age-encryption.org/v1")) || bytes.Contains(raw, []byte("PRIVATE-BODY-MARKER")) || bytes.Contains(raw, []byte("PRIVATE-METADATA-MARKER")) {
		return errors.New("plaintext reached durable spool")
	}
	f.body = raw
	f.key = key
	return nil
}

func (f *privateIngressObjectsFixture) Get(_ context.Context, _ tenancy.Tenant, key string) (io.ReadCloser, error) {
	if f.key != key {
		return nil, ingress.ErrContract
	}
	raw := bytes.Clone(f.body)
	if f.corrupt {
		raw[len(raw)-1] ^= 1
	}
	return io.NopCloser(bytes.NewReader(raw)), nil
}

func TestPrivateIngressObjectIsCiphertextOnlyAndReadbackIsRequired(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("TMPDIR", dir)
	scope := storage.Scope{Tenant: "acme", Subject: "owner", Product: "mail"}
	ctx := tenancy.Into(t.Context(), tenancy.Tenant{Slug: scope.Tenant})
	identity, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatal(err)
	}
	plan := ingress.KeyPlan{Version: 1, Scope: scope, Epoch: "11111111-1111-4111-8111-111111111111", Generation: 1, Profile: ingress.Profile, Recipient: identity.Recipient().String()}
	plan.KeyID = ingress.PublicFingerprint(plan.Recipient)
	metadata := []byte(`{"id":"PRIVATE-METADATA-MARKER"}`)
	body := "PRIVATE-BODY-MARKER original MIME"
	for _, fault := range []string{"valid", "corrupt", "upload"} {
		t.Run(fault, func(t *testing.T) {
			objects := &privateIngressObjectsFixture{corrupt: fault == "corrupt", failPut: fault == "upload"}
			task, id, err := SealPrivateIngress(ctx, objects, plan, "task_1", "mail.receive", metadata, strings.NewReader(body), int64(len(body)))
			if fault == "valid" {
				if err != nil || id == "" {
					t.Fatal(err)
				}
				meta, plain, err := agecipher.Open(ctx, bytes.NewReader(objects.body), task, identity)
				if err != nil {
					t.Fatal(err)
				}
				defer clear(meta)
				raw, err := io.ReadAll(plain)
				defer clear(raw)
				if err != nil || !bytes.Equal(meta, metadata) || string(raw) != body {
					t.Fatal("durable ciphertext did not recover original", err)
				}
			} else if err == nil || id != "" {
				t.Fatal("unverified upload became a sealed task", fault)
			}
			entries, err := os.ReadDir(dir)
			if err != nil || len(entries) != 0 {
				t.Fatal("temporary ciphertext not cleaned", err)
			}
		})
	}
}
