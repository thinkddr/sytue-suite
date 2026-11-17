// SPDX-License-Identifier: Apache-2.0
package webport

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"sync"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

type sourcePort struct {
	id       string
	identity Identity
}

func (p sourcePort) ID() string                                    { return p.id }
func (p sourcePort) Current(ctx context.Context) (Identity, error) { return p.identity, ctx.Err() }
func (p sourcePort) Call(ctx context.Context, scope storage.Scope, method Method, raw []byte) ([]byte, error) {
	if scope == p.identity.Scope && method == Authorize && len(raw) == 0 {
		return nil, ctx.Err()
	}
	return nil, ErrRejected
}

func fixtureIdentity() Identity {
	return Identity{Version: Version, Binding: Binding{MatrixUserID: "@owner:example.test", DeviceID: "DEVICE"}, Scope: storage.Scope{Tenant: "tenant", Subject: "owner", Product: "mail"}}
}

type heldReader struct {
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func (r *heldReader) ReadAt(p []byte, _ int64) (int, error) {
	r.once.Do(func() { close(r.entered) })
	<-r.release
	return copy(p, "secret"), nil
}

func TestScopedSourcesRetainReaderUntilCancelAndDrain(t *testing.T) {
	sources := NewSources()
	identity := fixtureIdentity()
	port := sourcePort{id: "port", identity: identity}
	reader := &heldReader{entered: make(chan struct{}), release: make(chan struct{})}
	source, release, err := sources.Add(context.Background(), port, identity, reader, 6)
	if err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	go func() {
		data, e := sources.Read(context.Background(), port, identity, source, 0, 6)
		if data != nil {
			e = errors.New("canceled reader returned plaintext")
		}
		result <- e
	}()
	<-reader.entered
	released := make(chan struct{})
	go func() { release(); close(released) }()
	select {
	case <-released:
		t.Fatal("release did not join actual reader")
	case <-time.After(20 * time.Millisecond):
	}
	if _, err = sources.Read(context.Background(), port, identity, source, 0, 6); !errors.Is(err, ErrRejected) {
		t.Fatal("retired source admitted")
	}
	close(reader.release)
	if err = <-result; !errors.Is(err, context.Canceled) {
		t.Fatalf("late result %v", err)
	}
	<-released
}

func TestScopedSourcesRejectOwnerPortAndRange(t *testing.T) {
	sources := NewSources()
	identity := fixtureIdentity()
	port := sourcePort{id: "port", identity: identity}
	source, release, err := sources.Add(context.Background(), port, identity, bytes.NewReader([]byte("secret")), 6)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	foreign := identity
	foreign.Scope.Subject = "other"
	for _, attempt := range []struct {
		port     sourcePort
		identity Identity
		offset   int64
		length   int
	}{
		{sourcePort{id: "different", identity: identity}, identity, 0, 6}, {port, foreign, 0, 6}, {port, identity, -1, 6}, {port, identity, 0, 7}, {port, identity, 0, ChunkBytes + 1},
	} {
		if raw, err := sources.Read(context.Background(), attempt.port, attempt.identity, source, attempt.offset, attempt.length); !errors.Is(err, ErrRejected) || raw != nil {
			t.Fatal("foreign/range source read admitted")
		}
	}
	raw, err := sources.Read(context.Background(), port, identity, source, 0, 6)
	if err != nil || string(raw) != "secret" {
		t.Fatal("valid reader rejected")
	}
	clear(raw)
}

type storagePort struct {
	sourcePort
	handle func(Method, []byte) ([]byte, error)
}

func (p storagePort) Call(_ context.Context, scope storage.Scope, method Method, raw []byte) ([]byte, error) {
	if scope != p.identity.Scope {
		return nil, ErrRejected
	}
	return p.handle(method, raw)
}

func TestStorageFileBoundaryKeepsExactBytesAndFailsBadCommitment(t *testing.T) {
	identity := fixtureIdentity()
	ref := storage.Reference{RoomID: "!room:test", EventID: "$file"}
	original := bytes.Repeat([]byte("exact MIME\r\n"), ChunkBytes/8)
	request := storage.FileRequest{ObjectID: "object", RevisionID: "revision", Metadata: []byte("{}"), SizeBytes: int64(len(original))}
	sumBytes := sha256.Sum256(original)
	sum := hex.EncodeToString(sumBytes[:])
	for _, wrong := range []bool{false, true} {
		t.Run(map[bool]string{false: "exact", true: "bad-digest"}[wrong], func(t *testing.T) {
			closed := false
			port := storagePort{sourcePort: sourcePort{id: "port", identity: identity}, handle: func(method Method, raw []byte) ([]byte, error) {
				switch method {
				case StorageFileDownload:
					checksum := sum
					if wrong {
						checksum = string(bytes.Repeat([]byte("0"), 64))
					}
					return json.Marshal(map[string]any{"stream": string(bytes.Repeat([]byte("a"), 64)), "request": request, "sha256": checksum})
				case StorageFileRead:
					var input struct {
						Offset int64
						Length int
					}
					if json.Unmarshal(raw, &input) != nil {
						return nil, ErrRejected
					}
					return bytes.Clone(original[input.Offset : input.Offset+int64(input.Length)]), nil
				case StorageFileClose:
					closed = true
					return nil, nil
				default:
					return nil, ErrRejected
				}
			}}
			repository, err := OpenStorage(context.Background(), port, NewSources())
			if err != nil {
				t.Fatal(err)
			}
			var output bytes.Buffer
			got, err := repository.Download(context.Background(), ref, "object", "revision", &output)
			if wrong {
				if !errors.Is(err, ErrRejected) {
					t.Fatal("bad commitment accepted")
				}
			} else if err != nil || got.SizeBytes != request.SizeBytes || !bytes.Equal(output.Bytes(), original) {
				t.Fatal("exact file changed", err)
			}
			if !closed {
				t.Fatal("file stream leaked")
			}
			clear(output.Bytes())
		})
	}
}

func TestSourceUnexpectedEOFIsRejected(t *testing.T) {
	sources := NewSources()
	identity := fixtureIdentity()
	port := sourcePort{id: "port", identity: identity}
	source, release, err := sources.Add(context.Background(), port, identity, bytes.NewReader([]byte("x")), 10)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if raw, err := sources.Read(context.Background(), port, identity, source, 0, 10); raw != nil || !errors.Is(err, io.EOF) {
		t.Fatal("truncated source accepted")
	}
}

func TestStorageBindingsReopenPinsOnlyAuthenticatedTargets(t *testing.T) {
	identity := fixtureIdentity()
	binding := storage.Binding{Scope: identity.Scope, Target: storage.PublicationTarget{RoomID: "!room:test", DeviceID: identity.Binding.DeviceID}, Rooms: []string{"!room:test"}}
	port := storagePort{sourcePort: sourcePort{id: "port", identity: identity}, handle: func(method Method, _ []byte) ([]byte, error) {
		if method != StorageBindings {
			return nil, ErrRejected
		}
		return json.Marshal(binding)
	}}
	repository, err := OpenStorage(context.Background(), port, NewSources())
	if err != nil {
		t.Fatal(err)
	}
	if repository.CheckStoragePublicationTarget(binding.Target) == nil {
		t.Fatal("uncaptured routing admitted")
	}
	got, err := repository.StorageBindings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err = repository.CheckStoragePublicationTarget(binding.Target); err != nil {
		t.Fatal("authenticated reopened route missing", err)
	}
	got.Rooms[0] = "!foreign:test"
	if err = repository.CheckStoragePublicationTarget(binding.Target); err != nil {
		t.Fatal("caller mutated pinned route", err)
	}
	if repository.CheckStoragePublicationTarget(storage.PublicationTarget{RoomID: "!foreign:test", DeviceID: identity.Binding.DeviceID}) == nil {
		t.Fatal("foreign room admitted")
	}
	if repository.CheckStoragePublicationTarget(storage.PublicationTarget{RoomID: "!room:test", DeviceID: "OTHER"}) == nil {
		t.Fatal("foreign device admitted")
	}
}

func TestStorageIdentityUsesCentralNamespaceWithoutLegacyUpgrade(t *testing.T) {
	for _, input := range []struct {
		user    string
		version storage.IdentityVersion
	}{
		{"@owner:example.test", storage.LegacyIdentity},
		{"@sytue_storage_mail_other_owner:example.test", storage.LegacyIdentity},
		{"@sytue_storage_drive_tenant_owner:example.test", storage.LegacyIdentity},
		{"@sytue_storage_mail_tenant_owner:example.test", storage.IsolatedIdentity},
	} {
		identity := fixtureIdentity()
		identity.Binding.MatrixUserID = input.user
		repository, err := OpenStorage(context.Background(), sourcePort{id: "port", identity: identity}, NewSources())
		if err != nil {
			t.Fatal(err)
		}
		if repository.StorageIdentityVersion() != input.version {
			t.Fatalf("central namespace %s became identity %d", input.user, repository.StorageIdentityVersion())
		}
	}
}

func TestStorageRecordReadbackNeverAcknowledgesSubstitutedOriginalGoIntent(t *testing.T) {
	identity := fixtureIdentity()
	target := storage.PublicationTarget{RoomID: "!private:test", DeviceID: identity.Binding.DeviceID}
	ref := storage.Reference{RoomID: target.RoomID, EventID: "$accepted"}
	for _, fault := range []string{"exact", "revision", "payload", "previous", "scope", "ref", "read-failure", "cancelled"} {
		t.Run(fault, func(t *testing.T) {
			original := storage.Record{Version: 1, Scope: identity.Scope, ObjectID: "object", RevisionID: "revision", Role: "content", Previous: []string{"$parent"}, Payload: []byte(`{"integer":900719925474099312345}`)}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			writes, reads := 0, 0
			port := storagePort{sourcePort: sourcePort{id: "record-port", identity: identity}, handle: func(method Method, raw []byte) ([]byte, error) {
				switch method {
				case StoragePrepare:
					return json.Marshal(storage.Binding{Scope: identity.Scope, Target: target, Rooms: []string{target.RoomID}})
				case StoragePutRecord:
					writes++
					var input struct {
						Target      storage.PublicationTarget
						Transaction string
						Record      storage.Record
					}
					if json.Unmarshal(raw, &input) != nil || input.Target != target || input.Transaction != "same_intent" || !original.MatchesExact(input.Record) {
						t.Fatal("changed immutable Go intent")
					}
					if fault == "cancelled" {
						cancel()
					}
					result := ref
					if fault == "ref" {
						result.RoomID = "!foreign:test"
					}
					return json.Marshal(result)
				case StorageReadRecord:
					reads++
					if fault == "read-failure" {
						return nil, ErrRejected
					}
					var input struct {
						Reference    storage.Reference
						Object, Role string
					}
					if json.Unmarshal(raw, &input) != nil || input.Reference != ref || input.Object != original.ObjectID || input.Role != original.Role {
						t.Fatal("readback used a different receipt")
					}
					actual := original
					switch fault {
					case "revision":
						actual.RevisionID = "other"
					case "payload":
						actual.Payload = []byte(`{"integer":900719925474099300000}`)
					case "previous":
						actual.Previous = []string{"$different"}
					case "scope":
						actual.Scope.Subject = "foreign"
					}
					return json.Marshal(actual)
				default:
					return nil, ErrRejected
				}
			}}
			repository, err := OpenStorage(ctx, port, NewSources())
			if err != nil {
				t.Fatal(err)
			}
			if _, err = repository.PrepareStorage(ctx, identity.Scope); err != nil {
				t.Fatal(err)
			}
			got, err := repository.PutStorageRecordAt(ctx, target, "same_intent", original)
			if fault == "exact" {
				if err != nil || got != ref || writes != 1 || reads != 1 {
					t.Fatal("valid original Go readback rejected", err, writes, reads)
				}
			} else if err == nil {
				t.Fatal("substituted intent acknowledged", fault)
			}
			if string(original.Payload) != `{"integer":900719925474099312345}` {
				t.Fatal("caller payload erased or changed")
			}
		})
	}
}

func TestStorageCatalogReadbackReturnsOnlyOriginalGoCommit(t *testing.T) {
	identity := fixtureIdentity()
	target := storage.PublicationTarget{RoomID: "!private:test", DeviceID: identity.Binding.DeviceID}
	ref := storage.Reference{RoomID: target.RoomID, EventID: "$index"}
	for _, fault := range []string{"exact", "revision", "metadata", "state", "parent", "reference", "read-failure"} {
		t.Run(fault, func(t *testing.T) {
			original := storage.ObjectCommit{Kind: storage.ObjectKind, State: "deleted", Metadata: []byte(`{"integer":900719925474099312345}`), Parents: []storage.Reference{{RoomID: target.RoomID, EventID: "$parent"}}}
			reads := 0
			port := storagePort{sourcePort: sourcePort{id: "object-port", identity: identity}, handle: func(method Method, raw []byte) ([]byte, error) {
				switch method {
				case StoragePrepare:
					return json.Marshal(storage.Binding{Scope: identity.Scope, Target: target, Rooms: []string{target.RoomID}})
				case StoragePutObject:
					var input struct {
						Target                        storage.PublicationTarget
						Transaction, Object, Revision string
						Commit                        storage.ObjectCommit
					}
					if json.Unmarshal(raw, &input) != nil || input.Target != target || input.Transaction != "same_intent" || input.Object != "object" || input.Revision != "revision" || !original.MatchesExact(input.Commit) {
						t.Fatal("changed original Go catalog intent")
					}
					// Even plausible public returned metadata is not the actual SDK readback.
					return json.Marshal(storage.ObjectVersion{Reference: ref, ObjectID: "object", RevisionID: "revision", Commit: storage.ObjectCommit{Kind: storage.ObjectKind, State: "deleted", Metadata: []byte(`{"unverified":true}`)}})
				case StorageReadObject:
					reads++
					if fault == "read-failure" {
						return nil, ErrRejected
					}
					var input struct {
						Reference storage.Reference
						Object    string
					}
					if json.Unmarshal(raw, &input) != nil || input.Reference != ref || input.Object != "object" {
						t.Fatal("catalog readback changed original receipt")
					}
					actual := storage.ObjectVersion{Reference: ref, ObjectID: "object", RevisionID: "revision", Commit: original}
					switch fault {
					case "revision":
						actual.RevisionID = "other"
					case "metadata":
						actual.Commit.Metadata = []byte(`{"integer":900719925474099300000}`)
					case "state":
						actual.Commit.State = "live"
						actual.Commit.Content = &storage.ContentPointer{Kind: "record", RevisionID: "content", Reference: storage.Reference{RoomID: target.RoomID, EventID: "$content"}}
					case "parent":
						actual.Commit.Parents = nil
					case "reference":
						actual.Reference.EventID = "$other"
					}
					return json.Marshal(actual)
				default:
					return nil, ErrRejected
				}
			}}
			repository, err := OpenStorage(t.Context(), port, NewSources())
			if err != nil {
				t.Fatal(err)
			}
			if _, err = repository.PrepareStorage(t.Context(), identity.Scope); err != nil {
				t.Fatal(err)
			}
			result, err := repository.PutStorageObjectAt(t.Context(), target, "same_intent", "object", "revision", original)
			if fault == "exact" {
				if err != nil || reads != 1 || !original.MatchesExact(result.Commit) || result.Reference != ref {
					t.Fatal("verified Go commit changed or erased", err, reads)
				}
			} else if err == nil {
				t.Fatal("substituted catalog acknowledged", fault)
			}
			if string(original.Metadata) != `{"integer":900719925474099312345}` {
				t.Fatal("caller metadata changed or erased")
			}
		})
	}
}
