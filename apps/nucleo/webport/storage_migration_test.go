// SPDX-License-Identifier: Apache-2.0
package webport

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

type migrationPrincipal struct{}

func TestMigrationStorageKeepsReaderReadOnlyAndRequiresOriginalOpaqueWriterPolicy(t *testing.T) {
	identity := fixtureIdentity()
	binding := storage.Binding{Scope: identity.Scope, Target: storage.PublicationTarget{RoomID: "!legacy:test", DeviceID: identity.Binding.DeviceID}, Rooms: []string{"!legacy:test"}}
	policies := NewPolicies()
	writerCalls, writes, checks := 0, 0, 0
	var writerPort storagePort
	var writtenRecord storage.Record
	readerPort := storagePort{sourcePort: sourcePort{id: strings.Repeat("a", 64), identity: identity}, handle: func(method Method, _ []byte) ([]byte, error) {
		if method == StoragePrepare || method == StorageBindings {
			return json.Marshal(binding)
		}
		return nil, ErrRejected
	}}
	writerPort = storagePort{sourcePort: sourcePort{id: strings.Repeat("b", 64), identity: identity}, handle: func(method Method, raw []byte) ([]byte, error) {
		writerCalls++
		switch method {
		case Authorize:
			return nil, nil
		case StoragePrepare, StorageBindings:
			return json.Marshal(binding)
		case StorageRecoverable:
			return nil, nil
		case StoragePutRecord:
			var input struct {
				Authorization string
				Target        storage.PublicationTarget
				Transaction   string
				Record        storage.Record
			}
			if json.Unmarshal(raw, &input) != nil || !ValidID(input.Authorization) {
				return nil, ErrRejected
			}
			caller := context.WithValue(t.Context(), migrationPrincipal{}, "replacement")
			if err := policies.Authorize(caller, writerPort, identity, input.Authorization, input.Target, input.Transaction); err != nil {
				return nil, err
			}
			writes++
			writtenRecord = input.Record
			return json.Marshal(storage.Reference{RoomID: binding.Target.RoomID, EventID: "$accepted"})
		case StorageReadRecord:
			return json.Marshal(writtenRecord)
		default:
			return nil, ErrRejected
		}
	}}
	sources := NewSources()
	reader, _ := OpenStorage(t.Context(), readerPort, sources, policies)
	writer, _ := OpenStorage(t.Context(), writerPort, sources, policies)
	combined, err := NewMigrationStorage(reader, writer)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = combined.PrepareStorage(t.Context(), identity.Scope); err != nil || writerCalls != 0 {
		t.Fatal("reader preparation touched writer", err, writerCalls)
	}
	record := storage.Record{Version: 1, Scope: identity.Scope, ObjectID: "object", RevisionID: "revision", Role: "content", Payload: []byte("{}")}
	if _, err = combined.PutStorageRecordAt(t.Context(), binding.Target, "stable", record); !errors.Is(err, ErrRejected) || writes != 0 {
		t.Fatal("unbound writer admitted", err)
	}
	original := context.WithValue(t.Context(), migrationPrincipal{}, "original")
	bound, err := combined.RequireAuthorizedStoragePublication(original, identity.Scope, binding.Target, func(ctx context.Context) error {
		checks++
		if ctx.Value(migrationPrincipal{}) != "original" {
			return ErrRejected
		}
		return ctx.Err()
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = combined.PutStorageRecordAt(bound, binding.Target, "stable", record); err != nil || writes != 1 || checks < 2 {
		t.Fatal("original opaque writer policy missing", err, writes, checks)
	}
	readonly, err := NewMigrationStorage(reader, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = readonly.PrepareStorage(t.Context(), identity.Scope); err != nil {
		t.Fatal(err)
	}
	if _, err = readonly.RequireAuthorizedStoragePublication(original, identity.Scope, binding.Target, func(context.Context) error { return nil }); !errors.Is(err, ErrRejected) {
		t.Fatal("missing migration writer fell back", err)
	}
	foreign := identity
	foreign.Scope.Subject = "foreign"
	other, _ := OpenStorage(t.Context(), sourcePort{id: strings.Repeat("c", 64), identity: foreign}, sources, policies)
	if _, err = NewMigrationStorage(reader, other); !errors.Is(err, ErrRejected) {
		t.Fatal("foreign writer mixed with source", err)
	}
}

func TestStorageCopyToUsesOriginalReaderAndExactDestinationCommitmentAcrossIdentities(t *testing.T) {
	previousIdentity := fixtureIdentity()
	previousIdentity.Binding.DeviceID = "LEGACY"
	nextIdentity := previousIdentity
	nextIdentity.Binding.MatrixUserID = storage.IsolatedMatrixUserPrefix(nextIdentity.Scope) + "test:matrix.test"
	nextIdentity.Binding.DeviceID = "ISOLATED"
	sourceRef := storage.Reference{RoomID: "!legacy:test", EventID: "$file"}
	target := storage.PublicationTarget{RoomID: "!isolated:test", DeviceID: nextIdentity.Binding.DeviceID}
	original := bytes.Repeat([]byte("exact original MIME\r\n"), 20000)
	sum := sha256.Sum256(original)
	checksum := hex.EncodeToString(sum[:])
	request := storage.FileRequest{ObjectID: "object", RevisionID: "revision", SizeBytes: int64(len(original)), Metadata: []byte(`{"name":"private","integer":900719925474099312345}`)}
	sources := NewSources()
	closed := false
	reads := 0
	uploads := 0
	previousPort := storagePort{sourcePort: sourcePort{id: strings.Repeat("a", 64), identity: previousIdentity}, handle: func(method Method, raw []byte) ([]byte, error) {
		switch method {
		case Authorize:
			return nil, nil
		case StorageFileDownload:
			return json.Marshal(map[string]any{"stream": strings.Repeat("c", 64), "request": request, "sha256": checksum})
		case StorageFileRead:
			reads++
			var value struct {
				Offset int64
				Length int
			}
			if json.Unmarshal(raw, &value) != nil || value.Length > ChunkBytes {
				return nil, ErrRejected
			}
			return bytes.Clone(original[value.Offset : value.Offset+int64(value.Length)]), nil
		case StorageFileClose:
			closed = true
			return nil, nil
		default:
			return nil, ErrRejected
		}
	}}
	var nextPort storagePort
	nextPort = storagePort{sourcePort: sourcePort{id: strings.Repeat("b", 64), identity: nextIdentity}, handle: func(method Method, raw []byte) ([]byte, error) {
		switch method {
		case Authorize:
			return nil, nil
		case StorageBindings, StoragePrepare:
			return json.Marshal(storage.Binding{Scope: nextIdentity.Scope, Target: target, Rooms: []string{target.RoomID}})
		case StorageFileUpload:
			uploads++
			var input struct {
				Source  string
				Request storage.FileRequest
			}
			if json.Unmarshal(raw, &input) != nil || !ValidID(input.Source) {
				return nil, ErrRejected
			}
			for offset := int64(0); offset < input.Request.SizeBytes; {
				length := int(min(int64(ChunkBytes), input.Request.SizeBytes-offset))
				value, err := sources.Read(t.Context(), nextPort, nextIdentity, input.Source, offset, length)
				if err != nil {
					return nil, err
				}
				if !bytes.Equal(value, original[offset:offset+int64(length)]) {
					return nil, ErrRejected
				}
				clear(value)
				offset += int64(length)
			}
			return json.Marshal(storage.Reference{RoomID: target.RoomID, EventID: "$copied"})
		case StorageFileCommitment:
			return json.Marshal(map[string]any{"request": request, "sha256": checksum})
		default:
			return nil, ErrRejected
		}
	}}
	previous, _ := OpenStorage(t.Context(), previousPort, sources)
	next, _ := OpenStorage(t.Context(), nextPort, sources)
	if _, err := next.StorageBindings(t.Context()); err != nil {
		t.Fatal(err)
	}
	copied, err := previous.CopyTo(t.Context(), next, sourceRef, "object", "revision", "job", target, request)
	if err != nil || copied.RoomID != target.RoomID || !closed || reads < 2 {
		t.Fatal("cross-SDK exact copy failed", copied, err, closed, reads)
	}
	request.Previous = []string{"$old-parent"}
	if _, err := previous.CopyTo(t.Context(), next, sourceRef, "object", "revision", "job", target, request); !errors.Is(err, ErrRejected) || uploads != 1 {
		t.Fatal("cross-account manifest retained a legacy parent", err, uploads)
	}
	if _, err := next.CopyTo(t.Context(), previous, sourceRef, "object", "revision", "job", target, request); !errors.Is(err, ErrRejected) {
		t.Fatal("reverse identity copy admitted", err)
	}
	clear(original)
}
