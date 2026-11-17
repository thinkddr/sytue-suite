// SPDX-License-Identifier: Apache-2.0
package records

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/auth/authtest"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

type commitmentReadProbe struct {
	*binaryReadProbe
	digest    string
	downloads int
	err       error
}

func (p *commitmentReadProbe) StorageFileCommitment(context.Context, storage.Reference, string, string) (storage.FileRequest, string, error) {
	return p.info, p.digest, p.err
}

func (p *commitmentReadProbe) DownloadStorageFile(ctx context.Context, gateway *api.Drive, ref storage.Reference, object, revision string, out io.Writer) (storage.FileRequest, error) {
	p.downloads++
	return p.binaryReadProbe.DownloadStorageFile(ctx, gateway, ref, object, revision, out)
}

func TestBinaryReopenUsesAuthenticatedFullDigestWithoutFetchingMedia(t *testing.T) {
	for _, scenario := range []string{"valid", "digest", "metadata", "size", "object", "revision", "scope", "room", "sdk error", "canceled"} {
		t.Run(scenario, func(t *testing.T) {
			r, base, version, row := binaryReaderFixture(t)
			bindCommitmentFixture(r, &version)
			probe := &commitmentReadProbe{binaryReadProbe: base, digest: row.SHA256}
			r.cfg.Crypto = probe
			ctx := t.Context()
			switch scenario {
			case "digest":
				probe.digest = hashBytes([]byte("other"))
			case "metadata":
				probe.info.Metadata = []byte(`{"other":true}`)
			case "size":
				probe.info.SizeBytes++
			case "object":
				probe.info.ObjectID = "other"
			case "revision":
				probe.info.RevisionID = "other"
			case "scope":
				r.cfg.Scope.Subject = "other"
			case "room":
				version.Commit.Content.Reference.RoomID = "!other:test"
			case "sdk error":
				probe.err = ErrSource
			case "canceled":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			got, err := r.verifyBinaryCommitment(ctx, version)
			if scenario == "valid" {
				if err != nil || got != row {
					t.Fatal(err)
				}
			} else if err == nil {
				t.Fatal("changed commitment accepted")
			}
			if probe.downloads != 0 {
				t.Fatal("commitment check downloaded provider content")
			}
		})
	}
}

type legacyBinaryReadProbe struct {
	Crypto
	reader    *binaryReadProbe
	downloads int
}

func (p *legacyBinaryReadProbe) StorageFileInfo(ctx context.Context, ref storage.Reference, object, revision string) (storage.FileRequest, error) {
	return p.reader.StorageFileInfo(ctx, ref, object, revision)
}

func (p *legacyBinaryReadProbe) DownloadStorageFile(ctx context.Context, gateway *api.Drive, ref storage.Reference, object, revision string, out io.Writer) (storage.FileRequest, error) {
	p.downloads++
	return p.reader.DownloadStorageFile(ctx, gateway, ref, object, revision, out)
}

func TestBinaryCommitmentFallsBackToFullReadForExistingAdapters(t *testing.T) {
	r, base, version, row := binaryReaderFixture(t)
	bindCommitmentFixture(r, &version)
	probe := &legacyBinaryReadProbe{reader: base}
	r.cfg.Crypto = probe
	got, err := r.verifyBinaryCommitment(t.Context(), version)
	if err != nil || got != row || probe.downloads != 1 {
		t.Fatal("legacy adapter skipped full verification", err)
	}
	base.body[0] ^= 1
	if _, err := r.verifyBinaryCommitment(t.Context(), version); !errors.Is(err, ErrSource) {
		t.Fatal("legacy full-read fallback accepted corruption", err)
	}
}

type binaryAuditProbe struct {
	*commitmentReadProbe
	binding storage.Binding
	entries []storage.RecordEntry
}

func (p *binaryAuditProbe) StorageBindings(context.Context) (storage.Binding, error) {
	return p.binding, nil
}

func (p *binaryAuditProbe) StorageObjectHistory(context.Context, string, string, int) (storage.ObjectPage, error) {
	return storage.ObjectPage{Entries: p.entries}, nil
}

func TestBinaryAuditChecksActualBodiesAfterMetadataOpen(t *testing.T) {
	for _, scenario := range []string{"valid", "corrupt", "missing"} {
		t.Run(scenario, func(t *testing.T) {
			r, base, entry := binaryCatalogFixture(t)
			r.ready = true
			journal, err := offline.OpenWithSecrets(t.TempDir(), &authtest.Secrets{})
			if err != nil {
				t.Fatal(err)
			}
			r.cfg.Journal = journal
			entry.Version.Commit.Kind = storage.ObjectKind
			row := binaryRow(mustBinaryDescription(t, r, entry.Version))
			payload, err := json.Marshal(entry.Version.Commit)
			if err != nil {
				t.Fatal(err)
			}
			probe := &binaryAuditProbe{commitmentReadProbe: &commitmentReadProbe{binaryReadProbe: base, digest: row.SHA256}, binding: storage.Binding{Scope: r.cfg.Scope, Target: storage.PublicationTarget{RoomID: entry.Version.Reference.RoomID, DeviceID: "TEST"}, Rooms: []string{entry.Version.Reference.RoomID}}, entries: []storage.RecordEntry{{Reference: entry.Version.Reference, Record: storage.Record{Version: 1, Scope: r.cfg.Scope, ObjectID: entry.Version.ObjectID, RevisionID: entry.Version.RevisionID, Role: "index", Payload: payload}}}}
			r.cfg.Crypto = probe
			r.binding = probe.binding
			switch scenario {
			case "corrupt":
				base.body[0] ^= 1
			case "missing":
				base.body = nil
			}
			if _, err := r.verifyBinaryCommitment(t.Context(), entry.Version); err != nil {
				t.Fatal("manifest should authenticate independently of availability", err)
			}
			result, err := r.AuditBinaries(t.Context())
			if scenario == "valid" {
				if err != nil || result.Objects != 1 || result.Bytes != row.Size {
					t.Fatal("full audit did not cover retained binary", err)
				}
			} else if !errors.Is(err, ErrSource) {
				t.Fatal("availability audit accepted missing/corrupt provider media", err)
			}
			if probe.downloads != 1 {
				t.Fatal("audit used only manifest metadata")
			}
			r.Close()
		})
	}
}

func mustBinaryDescription(t *testing.T, r *Repository, version storage.ObjectVersion) entity {
	t.Helper()
	value, err := r.description(version)
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func bindCommitmentFixture(r *Repository, version *storage.ObjectVersion) {
	version.Reference = storage.Reference{RoomID: version.Commit.Content.Reference.RoomID, EventID: "$index"}
	r.binding = storage.Binding{Scope: r.cfg.Scope, Target: storage.PublicationTarget{RoomID: version.Reference.RoomID, DeviceID: "TEST"}, Rooms: []string{version.Reference.RoomID}}
}
