// SPDX-License-Identifier: Apache-2.0
package records

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

type binaryReadProbe struct {
	Crypto
	info storage.FileRequest
	body []byte
}

func (p *binaryReadProbe) StorageFileInfo(context.Context, storage.Reference, string, string) (storage.FileRequest, error) {
	return p.info, nil
}

func (p *binaryReadProbe) DownloadStorageFile(_ context.Context, _ *api.Drive, _ storage.Reference, _, _ string, out io.Writer) (storage.FileRequest, error) {
	_, err := out.Write(p.body)
	return p.info, err
}

func binaryReaderFixture(t *testing.T) (*Repository, *binaryReadProbe, storage.ObjectVersion, SnapshotRow) {
	t.Helper()
	body := []byte{0xff, 0xd8, 0, 1, 3, 5, 0xfe, 0xff}
	probe := &binaryReadProbe{body: body}
	r := &Repository{cfg: Config{Scope: storage.Scope{Tenant: "acme", Subject: "alice", Product: "photos"}, Product: "photos", EntityKind: "photos.entity", Crypto: probe, BinaryKind: func(kind string) bool { return kind == "original" }, ValidateBinary: func(_ context.Context, row SnapshotRow) error { return row.Validate() }}}
	row := SnapshotRow{Kind: "original", ID: "asset", Size: int64(len(body)), SHA256: hashBytes(body)}
	value := entity{Kind: r.cfg.EntityKind, Entity: row.Kind, ID: row.ID, SHA256: row.SHA256, Binary: true, Size: row.Size}
	metadata, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	object := r.object(row.Kind, row.ID)
	probe.info = storage.FileRequest{ObjectID: object, RevisionID: "revision", SizeBytes: row.Size, Metadata: metadata}
	version := storage.ObjectVersion{ObjectID: object, Commit: storage.ObjectCommit{State: "live", Metadata: metadata, Content: &storage.ContentPointer{Kind: "file", RevisionID: "revision", Reference: storage.Reference{RoomID: "!photos:test", EventID: "$file"}}}}
	return r, probe, version, row
}

func TestBinarySnapshotNeedsExplicitHandlerAndExactBlobCount(t *testing.T) {
	now := time.Now().UTC()
	scope := storage.Scope{Tenant: "acme", Subject: "alice", Product: "photos"}
	state := SnapshotState{Version: 1, LegacyMigrationState: storage.LegacyMigrationState{Scope: scope, Mode: "frozen", Epoch: "12345678-1234-1234-1234-123456789abc", StartedAt: &now, Rows: 2, Blobs: 1}}
	r := &Repository{cfg: Config{Scope: scope}}
	if err := r.validateState(state); !errors.Is(err, ErrSource) {
		t.Fatal("JSON consumer accepted binary source", err)
	}
	r.cfg.BinaryKind = func(kind string) bool { return kind == "original" }
	if err := r.validateState(state); !errors.Is(err, ErrSource) {
		t.Fatal("missing binary domain validator accepted", err)
	}
	r.cfg.ValidateBinary = func(context.Context, SnapshotRow) error { return nil }
	if err := r.validateState(state); err != nil {
		t.Fatal(err)
	}
	state.Blobs = 3
	if err := r.validateState(state); !errors.Is(err, ErrSource) {
		t.Fatal("more blobs than rows accepted", err)
	}
}

func TestBinaryReadAuthenticatesHeaderLengthHashAndDomain(t *testing.T) {
	for _, corruption := range []string{"none", "body", "truncated", "long", "metadata", "identity", "domain"} {
		t.Run(corruption, func(t *testing.T) {
			r, probe, version, row := binaryReaderFixture(t)
			switch corruption {
			case "body":
				probe.body[0] ^= 1
			case "truncated":
				probe.body = probe.body[:len(probe.body)-1]
			case "long":
				probe.body = append(probe.body, 7)
			case "metadata":
				probe.info.Metadata = []byte(`{"different":"authenticated metadata"}`)
			case "identity":
				probe.info.RevisionID = "other"
			case "domain":
				r.cfg.ValidateBinary = func(context.Context, SnapshotRow) error { return ErrShared }
			}
			var output bytes.Buffer
			got, err := r.readBinary(t.Context(), version, &output)
			if corruption == "none" {
				if err != nil || got != row || !bytes.Equal(output.Bytes(), probe.body) {
					t.Fatal("raw media changed", err)
				}
			} else if err == nil {
				t.Fatal("changed binary accepted")
			}
		})
	}
}

func TestBinaryFlagCannotReinterpretLargeJSONOrBeRemoved(t *testing.T) {
	r, _, version, _ := binaryReaderFixture(t)
	if _, err := r.readJSON(t.Context(), version); !errors.Is(err, ErrSource) {
		t.Fatal("binary passed through JSON reader", err)
	}
	var value entity
	if err := json.Unmarshal(version.Commit.Metadata, &value); err != nil {
		t.Fatal(err)
	}
	value.Binary, value.Size = false, 0
	version.Commit.Metadata, _ = json.Marshal(value)
	if _, err := r.description(version); !errors.Is(err, ErrSource) {
		t.Fatal("binary kind reinterpreted as JSON", err)
	}
	r.cfg.BinaryKind = nil
	if _, err := r.description(version); err != nil {
		t.Fatal("legacy large JSON descriptor rejected", err)
	}
}

func TestJSONOnlyReceiptAndEntityEncodingsRemainUnchanged(t *testing.T) {
	raw, err := json.Marshal(snapshotReceipt{Kind: "receipt", Epoch: "epoch", Rows: 1, Inventory: "hash"})
	if err != nil || string(raw) != `{"kind":"receipt","epoch":"epoch","rows":1,"inventory":"hash"}` {
		t.Fatal("legacy receipt changed", string(raw), err)
	}
	raw, err = json.Marshal(entity{Kind: "entity", Entity: "note", ID: "id", SHA256: "hash"})
	if err != nil || string(raw) != `{"kind":"entity","entity":"note","id":"id","sha256":"hash"}` {
		t.Fatal("legacy entity changed", string(raw), err)
	}
}

func TestBinaryCatalogChecksCloseAndReadinessBeforeCrypto(t *testing.T) {
	r := &Repository{cfg: Config{Context: t.Context()}, gate: offline.NewWorkGate(t.Context())}
	view := catalogView{r: r, requireReady: true}
	if _, err := view.BinaryInfo(t.Context(), Entry{}); !errors.Is(err, ErrNotReady) {
		t.Fatal("binary header bypassed readiness", err)
	}
	if _, err := r.ReadBinary(t.Context(), "original", "asset", io.Discard); !errors.Is(err, ErrNotReady) {
		t.Fatal("binary body bypassed readiness", err)
	}
	r.Close()
	if _, err := view.BinaryInfo(t.Context(), Entry{}); !errors.Is(err, ErrClosed) {
		t.Fatal("old binary view bypassed close", err)
	}
}
