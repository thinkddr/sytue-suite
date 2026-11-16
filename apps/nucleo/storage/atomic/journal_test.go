// SPDX-License-Identifier: Apache-2.0
//go:build !js

package atomic

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/auth/authtest"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

func atomicWALMutations(count int) []Mutation {
	values := make([]Mutation, count)
	sum := sha256.Sum256([]byte("artificial exact payload"))
	for i := range values {
		commit := storage.ObjectCommit{Kind: storage.ObjectKind, State: "live", Content: &storage.ContentPointer{Kind: "record", RevisionID: "content_revision", Reference: storage.Reference{RoomID: "!private:test", EventID: "$content"}}}
		commit.Metadata = []byte(`{"private":"artificial atomic WAL marker","unknown":9007199254740993}`)
		values[i] = Mutation{ObjectID: fmt.Sprint("object_", i), Commit: commit, PayloadSHA256: hex.EncodeToString(sum[:])}
	}
	return values
}

func TestAtomicWorkInterruptedCaptureRetainsPendingAndResumesExactPlanAfterSealedStoreReopen(t *testing.T) {
	dir := t.TempDir()
	secrets := new(authtest.Secrets)
	journal, err := offline.OpenWithSecrets(dir, secrets)
	if err != nil {
		t.Fatal(err)
	}
	scope := storage.Scope{Tenant: "acme", Subject: "alice", Product: "mail"}
	target := storage.PublicationTarget{RoomID: "!private:test", DeviceID: "ORIGINAL_A"}
	values := atomicWALMutations(257)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	observed := make(chan error, 1)
	go func() {
		ticker := time.NewTicker(time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				observed <- ctx.Err()
				return
			case <-ticker.C:
				var root Work
				found, e := journal.Read(atomicOwner(scope), storage.AtomicJournalNamespace(scope.Product), atomicOperationKey("operation"), &root)
				if e != nil {
					observed <- e
					cancel()
					return
				}
				if found && root.State == StateStaging {
					cancel()
					observed <- nil
					return
				}
			}
		}
	}()
	if work, err := Capture(ctx, journal, scope, "operation", target, values); !errors.Is(err, context.Canceled) || work.Version != 0 {
		t.Fatal("interrupted capture became complete", err)
	}
	if err := <-observed; err != nil && !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	pending, err := Load(t.Context(), journal, scope, "operation")
	if err != nil || pending.State != StateStaging {
		t.Fatal("durable staging root lost", err, pending.State)
	}
	if count, err := journal.CountOwner(atomicOwner(scope), storage.PrivateJournalNamespaces(scope.Product)...); err != nil || count == 0 {
		t.Fatal("partial staging reported clean shutdown", count, err)
	}
	changed := slices.Clone(values)
	changed[0] = cloneMutation(changed[0])
	changed[0].Commit.Metadata = []byte(`{"changed":true}`)
	if _, err := Capture(t.Context(), journal, scope, "operation", target, changed); err == nil {
		t.Fatal("changed input replaced a retained partial plan")
	}
	reopened, err := offline.OpenWithSecrets(dir, secrets)
	if err != nil {
		t.Fatal(err)
	}
	captured, err := Capture(t.Context(), reopened, scope, "operation", target, values)
	if err != nil || captured.State != StateCaptured || captured.BatchID != pending.BatchID || captured.Count != 257 {
		t.Fatal("resume changed captured membership", err)
	}
	for i := range values {
		member, err := ReadMember(t.Context(), reopened, captured, uint64(i))
		if err != nil || !member.matches(captured, uint64(i), values[i]) {
			t.Fatal("captured exact member unavailable", i, err)
		}
	}
	if count, err := reopened.CountOwner(atomicOwner(scope), storage.AtomicJournalNamespace(scope.Product)); err != nil || count != 258 {
		t.Fatal("root/member inventory lost", count, err)
	}
	if count, err := reopened.CountOwner("acme\x00bob\x00mail", storage.AtomicJournalNamespace(scope.Product)); err != nil || count != 0 {
		t.Fatal("WAL crossed physical account", count, err)
	}
	if err := filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() {
			raw, e := os.ReadFile(path)
			if e != nil {
				return e
			}
			if bytes.Contains(raw, []byte("artificial atomic WAL marker")) {
				t.Fatal("WAL stored plaintext")
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestAtomicWorkCapturedMissingMemberNeverRebuildsOrRetargets(t *testing.T) {
	journal, err := offline.OpenWithSecrets(t.TempDir(), new(authtest.Secrets))
	if err != nil {
		t.Fatal(err)
	}
	scope := storage.Scope{Tenant: "acme", Subject: "alice", Product: "mail"}
	target := storage.PublicationTarget{RoomID: "!private:test", DeviceID: "ORIGINAL_A"}
	values := atomicWALMutations(2)
	work, err := Capture(t.Context(), journal, scope, "operation", target, values)
	if err != nil {
		t.Fatal(err)
	}
	other := target
	other.DeviceID = "CURRENT_B"
	if _, err := Capture(t.Context(), journal, scope, "operation", other, values); err == nil {
		t.Fatal("current device retargeted original writer")
	}
	if err := journal.Delete(atomicOwner(scope), storage.AtomicJournalNamespace(scope.Product), atomicMemberKey("operation", 1)); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadMember(t.Context(), journal, work, 1); err == nil {
		t.Fatal("missing staged intent was ignored")
	}
	if _, err := Capture(t.Context(), journal, scope, "operation", target, values); err == nil {
		t.Fatal("captured plan recreated lost member data")
	}
	if count, err := journal.CountOwner(atomicOwner(scope), storage.AtomicJournalNamespace(scope.Product)); err != nil || count != 2 {
		t.Fatal("corrupt work discarded or repaired silently", count, err)
	}
}

func TestAtomicWorkChangedInputCannotReplaceUnstagedMembers(t *testing.T) {
	journal, err := offline.OpenWithSecrets(t.TempDir(), new(authtest.Secrets))
	if err != nil {
		t.Fatal(err)
	}
	scope := storage.Scope{Tenant: "acme", Subject: "alice", Product: "mail"}
	target := storage.PublicationTarget{RoomID: "!private:test", DeviceID: "ORIGINAL_A"}
	values := atomicWALMutations(2)
	fingerprint, err := atomicMutationFingerprint(values)
	if err != nil {
		t.Fatal(err)
	}
	root := Work{Version: 1, Scope: scope, Target: target, Operation: "operation", BatchID: "retained_batch", Fingerprint: fingerprint, Count: 2, State: StateStaging}
	if err := journal.Write(atomicOwner(scope), storage.AtomicJournalNamespace(scope.Product), atomicOperationKey(root.Operation), root); err != nil {
		t.Fatal(err)
	}
	changed := slices.Clone(values)
	changed[1] = cloneMutation(values[1])
	changed[1].Commit.Metadata = []byte(`{"different":9007199254740995}`)
	if _, err := Capture(t.Context(), journal, scope, root.Operation, target, changed); err == nil {
		t.Fatal("changed input completed old root with newly chosen unstaged content")
	}
	if count, err := journal.CountOwner(atomicOwner(scope), storage.AtomicJournalNamespace(scope.Product)); err != nil || count != 1 {
		t.Fatal("rejected input changed retained root or members", count, err)
	}
	retained, err := Load(t.Context(), journal, scope, root.Operation)
	if err != nil || retained != root {
		t.Fatal("rejected input altered original staging plan", err)
	}
}
