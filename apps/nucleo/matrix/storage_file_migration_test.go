// SPDX-License-Identifier: Apache-2.0
package matrix

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/auth/authtest"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

func migrationFileTarget(t *testing.T, source storageFileIO) (storageFileIO, *fileRecordFixture) {
	t.Helper()
	journal, err := offline.OpenWithSecrets(t.TempDir(), &authtest.Secrets{})
	if err != nil {
		t.Fatal(err)
	}
	records := newFileRecordFixture()
	records.target = storage.PublicationTarget{RoomID: "!isolated:matrix.test", DeviceID: "ISOLATED"}
	records.allowed = []string{records.target.RoomID}
	return storageFileIO{scope: source.scope, gateway: source.gateway, records: records, journal: journal}, records
}

func TestStorageFileMigrationReencryptsResumesAndKeepsOriginal(t *testing.T) {
	source, gateway, oldRecords, _ := fileFixture(t)
	original := patternedSource{size: storage.MaxCipherBlockBytes + 41, pattern: []byte("private retained file version; ")}
	request := fileRequest(original.size)
	old, err := source.upload(t.Context(), "original", request, original)
	if err != nil {
		t.Fatal(err)
	}
	target, records := migrationFileTarget(t, source)
	records.loseRoot = true
	if _, err := target.migrateContent(t.Context(), source, "migration", old, request.ObjectID, request.RevisionID); err == nil {
		t.Fatal("lost migration root acknowledgment accepted")
	}
	beforePuts := gateway.puts
	next, err := target.migrateContent(t.Context(), source, "migration", old, request.ObjectID, request.RevisionID)
	if err != nil {
		t.Fatal(err)
	}
	if next.RoomID != records.target.RoomID || gateway.puts != beforePuts || len(records.records) != 1 || len(oldRecords.records) != 1 {
		t.Fatal("migration retry changed its target, repeated ciphertext, or changed the original")
	}
	from, err := source.root(t.Context(), old, request.ObjectID, request.RevisionID)
	if err != nil {
		t.Fatal(err)
	}
	to, err := target.root(t.Context(), next, request.ObjectID, request.RevisionID)
	if err != nil || to.SHA256 != from.SHA256 || !bytes.Equal(to.Request.Metadata, request.Metadata) {
		t.Fatal("migration changed exact metadata or source commitment", err)
	}
	for i, leaf := range from.Root.Nodes {
		replacement := to.Root.Nodes[i].Block
		if replacement.File.Key.Key == leaf.Block.File.Key.Key || replacement.Blob.ID == leaf.Block.Blob.ID || replacement.Blob.SHA256 == leaf.Block.Blob.SHA256 {
			t.Fatal("migration reused the old content key or ciphertext")
		}
	}
	expected, actual := sha256.New(), sha256.New()
	if _, err := io.Copy(expected, io.NewSectionReader(original, 0, original.size)); err != nil {
		t.Fatal(err)
	}
	if _, err := target.download(t.Context(), next, request.ObjectID, request.RevisionID, actual); err != nil || !bytes.Equal(expected.Sum(nil), actual.Sum(nil)) {
		t.Fatal("migrated content differs from retained original", err)
	}
	var state storageFileUpload
	found, err := target.journal.Read(target.journalOwner(), storage.PendingUploadsNamespace, "migration", &state)
	if err != nil || !found || !state.Verified || state.Published == nil || *state.Published != next {
		t.Fatal("migration dropped its receipt before catalog publication", err)
	}
	if _, err := source.download(t.Context(), old, request.ObjectID, request.RevisionID, io.Discard); err != nil {
		t.Fatal("migration damaged original", err)
	}
}

func TestStorageFileMigrationChecksCommitmentAndParentLinksBeforePublication(t *testing.T) {
	for _, fault := range []string{"commitment", "parents", "different-scope", "retained-commitment"} {
		t.Run(fault, func(t *testing.T) {
			source, gateway, records, _ := fileFixture(t)
			data := []byte("immutable original")
			request := fileRequest(int64(len(data)))
			old, err := source.upload(t.Context(), "original", request, bytes.NewReader(data))
			if err != nil {
				t.Fatal(err)
			}
			target, targetRecords := migrationFileTarget(t, source)
			if fault == "retained-commitment" {
				targetRecords.loseRoot = true
				if _, err := target.migrateContent(t.Context(), source, "migration", old, request.ObjectID, request.RevisionID); err == nil {
					t.Fatal("fixture did not interrupt")
				}
			}
			rec := records.records[old]
			var file storageFile
			if err := json.Unmarshal(rec.Payload, &file); err != nil {
				t.Fatal(err)
			}
			switch fault {
			case "commitment", "retained-commitment":
				file.SHA256 = digestBytes([]byte("other content"))
			case "parents":
				file.Request.Previous, rec.Previous = []string{"$old-parent"}, []string{"$old-parent"}
			case "different-scope":
				target.scope.Tenant = "beta"
			}
			rec.Payload, err = json.Marshal(file)
			if err != nil {
				t.Fatal(err)
			}
			records.records[old] = rec
			puts, events := gateway.puts, len(targetRecords.records)
			if _, err := target.migrateContent(t.Context(), source, "migration", old, request.ObjectID, request.RevisionID); err == nil {
				t.Fatal("invalid source accepted", fault)
			}
			if gateway.puts != puts || len(targetRecords.records) != events {
				t.Fatal("source was rejected after destination publication", fault)
			}
		})
	}
}

func TestStorageFileReaderAtUsesOneBlockAndClearsItOnCancelAndClose(t *testing.T) {
	source, gateway, _, _ := fileFixture(t)
	input := patternedSource{size: storage.MaxCipherBlockBytes + 17, pattern: []byte("secret block ")}
	request := fileRequest(input.size)
	ref, err := source.upload(t.Context(), "reader", request, input)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	r := &storageFileReaderAt{ctx: ctx, source: source, reference: ref, request: request, part: -1}
	defer r.Close()
	before := gateway.gets
	for i := 0; i < 3; i++ {
		out, want := make([]byte, 13), make([]byte, 13)
		if _, err := r.ReadAt(out, int64(i)); err != nil {
			t.Fatal(err)
		}
		_, _ = input.ReadAt(want, int64(i))
		if !bytes.Equal(out, want) {
			t.Fatal("random access changed bytes")
		}
	}
	if gateway.gets-before != 1 {
		t.Fatal("small reads repeatedly downloaded the same encrypted block")
	}
	retained := r.block
	if _, err := r.ReadAt(make([]byte, 20), storage.MaxCipherBlockBytes-3); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(retained, make([]byte, len(retained))) || len(r.block) != 17 {
		t.Fatal("reader retained more than one plaintext block")
	}
	retained = r.block
	cancel()
	if _, err := r.ReadAt(make([]byte, 1), 0); !errors.Is(err, context.Canceled) || r.block != nil || !bytes.Equal(retained, make([]byte, len(retained))) {
		t.Fatal("cancelled reader kept plaintext", err)
	}
	r.Close()
	if _, err := r.ReadAt(make([]byte, 1), 0); !errors.Is(err, ErrClosed) {
		t.Fatal("closed reader still reads", err)
	}
}

func TestStorageFileMigrationPinsProductTenantOwnerAndIdentityBeforeIO(t *testing.T) {
	server := httptest.NewTLSServer(http.NotFoundHandler())
	defer server.Close()
	cfg := storageConfig(t, server, "drive")
	for _, fault := range []string{"valid", "product", "tenant", "subject", "home", "version", "uid", "same"} {
		t.Run(fault, func(t *testing.T) {
			c := &Client{cfg: cfg}
			next := cfg
			next.Bootstrap.StorageIdentityVersion = storage.IsolatedIdentity
			next.Bootstrap.MatrixUserID = storage.IsolatedMatrixUserPrefix(c.storageScope()) + "matrix.test"
			switch fault {
			case "product":
				next.Bootstrap.StorageProduct = "mail"
			case "tenant":
				next.Tenant = "beta"
			case "subject":
				next.Subject = "bob"
			case "home":
				next.Bootstrap.HomeserverURL = "https://foreign.invalid"
			case "version":
				next.Bootstrap.StorageIdentityVersion = storage.LegacyIdentity
			case "uid":
				next.Bootstrap.MatrixUserID = cfg.Bootstrap.MatrixUserID
			}
			destination := &Client{cfg: next}
			if fault == "same" {
				destination = c
			}
			_, err := c.fileMigrationScope(destination)
			if fault == "valid" && err != nil {
				t.Fatal(err)
			}
			if fault != "valid" {
				if !errors.Is(err, ErrIdentity) {
					t.Fatal("migration crossed identity", fault, err)
				}
				if _, err := c.MigrateStorageFileContent(t.Context(), destination, nil, nil, "migration", EventRef{}, "object", "revision"); !errors.Is(err, ErrIdentity) {
					t.Fatal("identity check ran after IO", err)
				}
			}
		})
	}
}

func TestStorageFileMigrationExplicitTargetPinsFirstManifestAfterRotation(t *testing.T) {
	source, gateway, _, _ := fileFixture(t)
	original := []byte("private file content stays on the journaled destination")
	request := fileRequest(int64(len(original)))
	ref, err := source.upload(t.Context(), "original-at", request, bytes.NewReader(original))
	if err != nil {
		t.Fatal(err)
	}
	target, records := migrationFileTarget(t, source)
	wanted := records.target
	records.target.RoomID = "!rotated:matrix.test"
	records.allowed = append(records.allowed, records.target.RoomID)
	target.records = storageFileTarget{storageRecordIO: records, storagePublicationIO: records, target: wanted}
	records.loseRoot = true
	if _, err := target.migrateContent(t.Context(), source, "migration-at", ref, request.ObjectID, request.RevisionID); err == nil {
		t.Fatal("missing initial receipt was accepted")
	}
	before := gateway.puts
	copied, err := target.migrateContent(t.Context(), source, "migration-at", ref, request.ObjectID, request.RevisionID)
	if err != nil || copied.RoomID != wanted.RoomID || gateway.puts != before {
		t.Fatal("migration followed a new default or duplicated ciphertext", copied, err)
	}
}

func TestStorageFileMigrationAtRejectsEndpointBeforePrivateKeysAndHTTP(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Error("invalid migration target reached HTTP")
	}))
	defer server.Close()
	cfg := storageConfig(t, server, "drive")
	source, err := newClient(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = source.Close() }()
	next := cfg
	next.Bootstrap.StorageIdentityVersion = storage.IsolatedIdentity
	next.Bootstrap.MatrixUserID = storage.IsolatedMatrixUserPrefix(source.storageScope()) + "matrix.test"
	next.StoreDir = t.TempDir()
	destination, err := newClient(next)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = destination.Close() }()
	destination.session.DeviceID = "DESTINATION"
	for _, target := range []storage.PublicationTarget{{RoomID: next.Bootstrap.StorageRoomID, DeviceID: "FOREIGN"}, {RoomID: "!foreign:matrix.test", DeviceID: "DESTINATION"}} {
		if _, err := source.MigrateStorageFileContentAt(t.Context(), destination, nil, nil, "invalid-target", target, EventRef{}, "object", "revision"); !errors.Is(err, ErrIdentity) {
			t.Fatal("invalid endpoint reached keys or encrypted state", err)
		}
	}
}
