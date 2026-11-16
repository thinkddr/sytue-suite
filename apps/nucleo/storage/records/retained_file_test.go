// SPDX-License-Identifier: Apache-2.0
package records

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"slices"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/auth/authtest"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

type retainedFileProbe struct {
	*commitmentReadProbe
	record          storage.Record
	link, root      storage.Reference
	target          storage.PublicationTarget
	acknowledgments int
}

func (p *retainedFileProbe) ReadStorageRecord(ctx context.Context, ref storage.Reference, object, role string) (storage.Record, error) {
	if err := ctx.Err(); err != nil {
		return storage.Record{}, err
	}
	if ref != p.link || object != p.record.ObjectID || role != "content" {
		return storage.Record{}, ErrSource
	}
	result := p.record
	result.Payload = slices.Clone(result.Payload)
	return result, nil
}

func (p *retainedFileProbe) StorageFileCommitment(ctx context.Context, ref storage.Reference, object, revision string) (storage.FileRequest, string, error) {
	if ref != p.root || object != p.info.ObjectID || revision != p.info.RevisionID {
		return storage.FileRequest{}, "", ErrSource
	}
	return p.commitmentReadProbe.StorageFileCommitment(ctx, ref, object, revision)
}

func (p *retainedFileProbe) CheckStoragePublicationTarget(target storage.PublicationTarget) error {
	if target != p.target {
		return ErrSource
	}
	return nil
}

func (p *retainedFileProbe) AcknowledgeStorageFileLink(context.Context, *offline.Store, string, storage.Reference) error {
	p.acknowledgments++
	return nil
}

func retainedReaderFixture(t *testing.T) (*Repository, *retainedFileProbe, RetainedFile) {
	t.Helper()
	r, base, version, row := binaryReaderFixture(t)
	r.cfg.Context = t.Context()
	r.gate = offline.NewWorkGate(t.Context())
	r.ready = true
	r.cfg.ValidateRow = func(context.Context, string, string, []byte) error { return nil }
	journal, err := offline.OpenWithSecrets(t.TempDir(), &authtest.Secrets{})
	if err != nil {
		t.Fatal(err)
	}
	r.cfg.Journal = journal
	target := storage.PublicationTarget{RoomID: "!photos:test", DeviceID: "PRODUCER"}
	file := RetainedFile{Version: 1, Scope: r.cfg.Scope, Target: target, Row: row, Revision: "revision", Link: storage.Reference{RoomID: target.RoomID, EventID: "$link"}}
	r.binding = storage.Binding{Scope: r.cfg.Scope, Target: target, Rooms: []string{target.RoomID}}
	header := r.retainedFileHeader(publication{Target: target, Kind: row.Kind, ID: row.ID, Size: row.Size, Input: row.SHA256})
	base.info.Metadata, err = json.Marshal(header)
	if err != nil {
		t.Fatal(err)
	}
	pointer := *version.Commit.Content
	payload, err := json.Marshal(storage.FileLink{Kind: storage.FileLinkKind, Content: pointer})
	if err != nil {
		t.Fatal(err)
	}
	probe := &retainedFileProbe{commitmentReadProbe: &commitmentReadProbe{binaryReadProbe: base, digest: row.SHA256}, record: storage.Record{Version: 1, Scope: r.cfg.Scope, ObjectID: base.info.ObjectID, RevisionID: file.Revision, Role: "content", Payload: payload}, link: file.Link, root: pointer.Reference, target: target}
	r.cfg.Crypto = probe
	t.Cleanup(r.Close)
	return r, probe, file
}

func TestRetainedFileAuthenticatesOriginalTargetAndDescriptorWithoutProviderReads(t *testing.T) {
	for _, change := range []string{"valid", "scope", "device", "room", "id", "revision", "size", "digest", "header", "link", "link root"} {
		t.Run(change, func(t *testing.T) {
			r, probe, file := retainedReaderFixture(t)
			switch change {
			case "scope":
				file.Scope.Subject = "other"
			case "device":
				file.Target.DeviceID = "OTHER"
			case "room":
				file.Target.RoomID = "!other:test"
			case "id":
				file.Row.ID = "other"
			case "revision":
				file.Revision = "other"
			case "size":
				file.Row.Size++
			case "digest":
				probe.digest = hashBytes([]byte("other"))
			case "header":
				probe.info.Metadata = []byte(`{"different":true}`)
			case "link":
				file.Link.EventID = "$other"
			case "link root":
				probe.root.EventID = "$other"
			}
			err := r.ValidateRetainedFile(t.Context(), file)
			if change == "valid" {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil {
				t.Fatal("changed retained file became visible")
			}
			if probe.downloads != 0 {
				t.Fatal("metadata validation downloaded provider media", probe.downloads)
			}
		})
	}
}

func TestRetainedFileReadChecksActualBytesAndCancellation(t *testing.T) {
	for _, change := range []string{"valid", "body", "cancel"} {
		t.Run(change, func(t *testing.T) {
			r, probe, file := retainedReaderFixture(t)
			ctx := t.Context()
			if change == "body" {
				probe.body[0] ^= 1
			}
			if change == "cancel" {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			var out bytes.Buffer
			err := r.ReadRetainedFile(ctx, file, &out)
			if change == "valid" {
				if err != nil || !bytes.Equal(out.Bytes(), probe.body) {
					t.Fatal("retained image changed", err)
				}
			} else if err == nil {
				t.Fatal("invalid read succeeded")
			}
			if change == "cancel" && probe.downloads != 0 {
				t.Fatal("canceled reader fetched a body")
			}
		})
	}
}

func TestRetainedFileCatalogValidatesBeforeReadyAndCannotAcknowledgeBeforeIndex(t *testing.T) {
	r, probe, file := retainedReaderFixture(t)
	r.ready = false
	if err := (catalogView{r: r}).ValidateRetainedFile(t.Context(), file); err != nil {
		t.Fatal(err)
	}
	if err := r.ValidateRetainedFile(t.Context(), file); !errors.Is(err, ErrNotReady) {
		t.Fatal("public validation bypassed ready gate", err)
	}
	r.ready = true
	selector := func([]byte) ([]RetainedFile, error) { return []RetainedFile{file}, nil }
	if err := r.AcknowledgeRetainedFiles(t.Context(), "recipe", []RetainedFile{file}, storage.ObjectVersion{}, selector); err == nil {
		t.Fatal("unpublished file was acknowledged")
	}
	if probe.acknowledgments != 0 {
		t.Fatal("ACK ran before an accepted index", probe.acknowledgments)
	}
	if probe.downloads != 0 {
		t.Fatal("open/failed ACK fetched provider images")
	}
	if err := r.ReadRetainedFile(t.Context(), file, io.Discard); err != nil {
		t.Fatal(err)
	}
}

func TestRetainedFileCatalogReadsBeforeReadyWithTheSameByteAndScopeGuards(t *testing.T) {
	for _, change := range []string{"valid", "scope", "target", "revision", "size", "digest", "body", "cancel", "closed"} {
		t.Run(change, func(t *testing.T) {
			r, probe, file := retainedReaderFixture(t)
			r.ready = false
			if err := r.ReadRetainedFile(t.Context(), file, io.Discard); !errors.Is(err, ErrNotReady) {
				t.Fatal("public read bypassed readiness", err)
			}
			ctx := t.Context()
			switch change {
			case "scope":
				file.Scope.Subject = "other"
			case "target":
				file.Target.DeviceID = "OTHER"
			case "revision":
				file.Revision = "other"
			case "size":
				file.Row.Size++
			case "digest":
				file.Row.SHA256 = hashBytes([]byte("other"))
			case "body":
				probe.body[0] ^= 1
			case "cancel":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			case "closed":
				r.Close()
			}
			var out bytes.Buffer
			err := (catalogView{r: r}).ReadRetainedFile(ctx, file, &out)
			if change == "valid" {
				if err != nil || !bytes.Equal(out.Bytes(), probe.body) {
					t.Fatal("pre-ready catalog reader changed retained bytes", err)
				}
			} else if err == nil {
				t.Fatal("invalid pre-ready retained read succeeded", change)
			}
		})
	}
}
