// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration && cgo

package matrix

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"testing"
	"time"

	"maunium.net/go/mautrix/event"
	matrixid "maunium.net/go/mautrix/id"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	privatedrive "git.sytue.eu/sytue/one/apps/nucleo/drive"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

type syntheticDriveSource struct {
	State        storage.LegacyMigrationState `json:"state"`
	Rows         []storage.LegacySourceRow    `json:"rows"`
	Blobs        map[string][]byte            `json:"blobs"`
	Root         string                       `json:"root"`
	File         string                       `json:"file"`
	Folder       string                       `json:"folder"`
	Trash        string                       `json:"trash"`
	pages, opens int
}

func (s *syntheticDriveSource) PrivateStorageState(_ context.Context, scope storage.Scope) (storage.LegacyMigrationState, error) {
	if s.State.Validate(scope) != nil {
		return storage.LegacyMigrationState{}, privatedrive.ErrSource
	}
	return s.State, nil
}

func (*syntheticDriveSource) BeginPrivateStorageMigration(context.Context, storage.Scope, string) (storage.LegacyMigrationState, error) {
	return storage.LegacyMigrationState{}, errors.New("synthetic source is already frozen")
}

func (s *syntheticDriveSource) PrivateStorageSourcePage(_ context.Context, scope storage.Scope, epoch string, after storage.LegacySourceCursor) (storage.LegacySourcePage, error) {
	s.pages++
	page := storage.LegacySourcePage{Scope: scope, Epoch: epoch, Rows: []storage.LegacySourceRow{}}
	if scope != s.State.Scope || epoch != s.State.Epoch {
		return page, privatedrive.ErrSource
	}
	for _, row := range s.Rows {
		if row.Kind < after.Kind || row.Kind == after.Kind && row.ID <= after.ID {
			continue
		}
		if len(page.Rows) == 2 {
			last := page.Rows[1]
			page.Next = &storage.LegacySourceCursor{Kind: last.Kind, ID: last.ID}
			break
		}
		page.Rows = append(page.Rows, storage.LegacySourceRow{Kind: row.Kind, ID: row.ID, Payload: bytes.Clone(row.Payload)})
	}
	return page, nil
}

func (s *syntheticDriveSource) OpenPrivateStorageSource(_ context.Context, scope storage.Scope, epoch, kind, id string, size int64) (io.ReadCloser, error) {
	s.opens++
	body, ok := s.Blobs[kind+"/"+id]
	if !ok || scope != s.State.Scope || epoch != s.State.Epoch || int64(len(body)) != size {
		return nil, privatedrive.ErrSource
	}
	return io.NopCloser(bytes.NewReader(body)), nil
}

// Both failures occur AFTER Synapse has accepted the authenticated SDK write.
type lostDriveImporterACK struct {
	*Client
	contentLost, catalogLost bool
	contents                 map[string]storage.Reference
	catalogs                 map[string]storage.Reference
	writes                   int
	uploadBlocked            bool
}

func (s *lostDriveImporterACK) UploadStorageFileAt(ctx context.Context, gateway *api.Drive, journal *offline.Store, job string, target storage.PublicationTarget, request storage.FileRequest, source io.ReaderAt) (storage.Reference, error) {
	if s.uploadBlocked {
		s.uploadBlocked = false
		return storage.Reference{}, errors.New("artificial stop after encrypted source capture")
	}
	return s.Client.UploadStorageFileAt(ctx, gateway, journal, job, target, request, source)
}

func (s *lostDriveImporterACK) PutStorageRecordAt(ctx context.Context, target storage.PublicationTarget, txn string, record storage.Record) (storage.Reference, error) {
	s.writes++
	ref, err := s.Client.PutStorageRecordAt(ctx, target, txn, record)
	var item struct {
		Kind string `json:"kind"`
	}
	if err == nil && record.Role == "content" && json.Unmarshal(record.Payload, &item) == nil && item.Kind == "org.sytue.drive.entry.v1" {
		if previous, exists := s.contents[txn]; exists && previous != ref {
			return storage.Reference{}, errors.New("content retry duplicated accepted event")
		}
		s.contents[txn] = ref
		if !s.contentLost {
			s.contentLost = true
			return ref, errors.New("artificial lost Drive content ACK")
		}
	}
	return ref, err
}

func (s *lostDriveImporterACK) PutStorageObjectAt(ctx context.Context, target storage.PublicationTarget, txn, object, revision string, commit storage.ObjectCommit) (storage.ObjectVersion, error) {
	s.writes++
	version, err := s.Client.PutStorageObjectAt(ctx, target, txn, object, revision, commit)
	if err == nil {
		if previous, exists := s.catalogs[txn]; exists && previous != version.Reference {
			return storage.ObjectVersion{}, errors.New("catalog retry duplicated accepted event")
		}
		s.catalogs[txn] = version.Reference
		var item struct {
			Kind string `json:"kind"`
		}
		if !s.catalogLost && json.Unmarshal(commit.Metadata, &item) == nil && item.Kind == "org.sytue.drive.item.v1" {
			s.catalogLost = true
			return version, errors.New("artificial lost Drive catalog ACK")
		}
	}
	return version, err
}

func exercisePrivateDriveImporter(ctx context.Context, t *testing.T, client *Client, gateway *api.Drive, journal *offline.Store, rotated Bootstrap, opaque *fileGatewayFixture, scenario string) {
	t.Helper()
	destination := os.Getenv("SYTUE_DRIVE_PRIVATE_VECTOR")
	if destination == "" {
		t.Skip("requires synthetic real PostgreSQL export on remote host")
	}
	raw, err := os.ReadFile(destination)
	if err != nil {
		t.Fatal(err)
	}
	defer clear(raw)
	var source syntheticDriveSource
	if json.Unmarshal(raw, &source) != nil || source.State.Scope != client.storageScope() || len(source.Rows) != 9 || len(source.Blobs) != 3 {
		t.Fatal("invalid synthetic PG bridge")
	}
	original, _ := json.Marshal(source)
	// A consistent metadata snapshot must not bless a different binary source.
	var changed syntheticDriveSource
	if json.Unmarshal(original, &changed) != nil {
		t.Fatal("invalid synthetic vector")
	}
	changed.Blobs["file/"+changed.File][0] ^= 1
	probe, err := privatedrive.New(privatedrive.Config{Context: ctx, Scope: client.storageScope(), Crypto: client, Gateway: gateway, Source: &changed, Journal: journal})
	if err != nil {
		t.Fatal(err)
	}
	if err := probe.Initialize(ctx); err == nil {
		probe.Close()
		t.Fatal("changed original accepted before migration")
	}
	probe.Close()
	faults := &lostDriveImporterACK{Client: client, contents: map[string]storage.Reference{}, catalogs: map[string]storage.Reference{}}
	config := privatedrive.Config{Context: ctx, Scope: client.storageScope(), Crypto: faults, Gateway: gateway, Source: &source, Journal: journal}
	open := func() *privatedrive.Repository {
		t.Helper()
		r, err := privatedrive.New(config)
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	repository := open()
	if _, err := repository.Root(ctx); !errors.Is(err, privatedrive.ErrNotReady) {
		t.Fatal("app exposed partial migration", err)
	}
	if err := repository.Initialize(ctx); err == nil || !faults.contentLost {
		t.Fatal("content ACK loss did not interrupt migration", err)
	}
	repository.Close()
	beforeRotation := client.cfg.Bootstrap
	if err := client.UpdateBindings(rotated); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := client.UpdateBindings(beforeRotation); err != nil {
			t.Error(err)
		}
	}()
	repository = open()
	if err := repository.Initialize(ctx); err == nil || !faults.catalogLost {
		t.Fatal("catalog ACK loss did not interrupt migration", err)
	}
	repository.Close()
	repository = open()
	defer func() { repository.Close() }()
	if err := repository.Initialize(ctx); err != nil {
		t.Fatal("real source migration did not resume", err)
	}
	personal, err := gateway.WithPrivateData(repository)
	if err != nil {
		t.Fatal("private native API facade", err)
	}
	root, err := personal.Root(ctx)
	if err != nil || root.ID != source.Root {
		t.Fatal("private root lost", root, err)
	}
	children, err := personal.Children(ctx, source.Root, "")
	if err != nil || len(children.Folders) != 1 || children.Folders[0].ID != source.Folder || len(children.Files) != 1 || children.Files[0].ID != source.File {
		t.Fatal("private hierarchy lost", children, err)
	}
	if children.Files[0].Label == nil || !children.Files[0].Label.BlockExternalShare {
		t.Fatal("private migration lost the SQL sensitivity restriction")
	}
	for _, row := range source.Rows {
		data, err := repository.ReadRaw(ctx, row.Kind, row.ID)
		if err != nil || !bytes.Equal(data, row.Payload) {
			t.Fatalf("metadata or domain integer changed for %s/%s: %v", row.Kind, row.ID, err)
		}
		clear(data)
	}
	body, err := personal.Download(ctx, source.File)
	if err != nil {
		t.Fatal(err)
	}
	output, err := io.ReadAll(body)
	_ = body.Close()
	if err != nil || !bytes.Equal(output, source.Blobs["file/"+source.File]) {
		t.Fatal("private app download changed", err)
	}
	clear(output)
	if _, err := repository.Download(ctx, source.Trash); !errors.Is(err, privatedrive.ErrNotFound) {
		t.Fatal("trash exposed as live file", err)
	}
	pages, opens := source.pages, source.opens
	unread, err := repository.Download(ctx, source.File)
	if err != nil {
		t.Fatal(err)
	}
	closed := make(chan struct{})
	go func() { repository.Close(); close(closed) }()
	select {
	case <-closed:
	case <-time.After(3 * time.Second):
		t.Fatal("repository close deadlocked on an unread private download")
	}
	_ = unread.Close()
	repository = open()
	if err := repository.Initialize(ctx); err != nil {
		t.Fatal("completed private repository did not reopen", err)
	}
	if source.pages != pages || source.opens != opens {
		t.Fatal("verified completion redundantly reimported original", source.pages, source.opens)
	}
	retained, _ := json.Marshal(source)
	if !bytes.Equal(retained, original) {
		t.Fatal("migration edited or removed its originals")
	}
	switch scenario {
	case "labels":
		repository = exercisePrivateDriveLabels(ctx, t, repository, open, faults, &source, opaque)
	case "thumbnails":
		repository = exercisePrivateDriveThumbnails(ctx, t, repository, open, faults, &source, opaque)
	case "":
		repository = exercisePrivateDriveMetadata(ctx, t, repository, open, faults, &source, opaque, journal)
		exercisePrivateDriveQueries(ctx, t, repository, &source, opaque)
		repository = exercisePrivateDriveFavorites(ctx, t, repository, open, faults, &source, opaque)
		exercisePrivateDriveRefresh(ctx, t, repository, open, &source)
		repository = exercisePrivateDriveZip(ctx, t, repository, open, &source)
	default:
		t.Fatal("unknown private Drive scenario", scenario)
	}
	if source.pages != pages || source.opens != opens {
		t.Fatal("private mutation read legacy storage")
	}
	for _, reference := range faults.catalogs {
		stored, err := client.cli.GetEvent(ctx, matrixid.RoomID(reference.RoomID), matrixid.EventID(reference.EventID))
		if err != nil || stored.Type != event.EventEncrypted {
			t.Fatal("Synapse stored a readable private index", err)
		}
		wire, err := json.Marshal(stored)
		if err != nil || bytes.Contains(wire, []byte("Private synthetic")) || bytes.Contains(wire, []byte("Private metadata")) || bytes.Contains(wire, []byte("900719925474099512345")) {
			t.Fatal("private metadata reached the homeserver", err)
		}
	}
	exportPrivateDriveWebVector(ctx, t, client, repository, &source, opaque)
	opaque.mu.Lock()
	defer opaque.mu.Unlock()
	for _, ciphertext := range opaque.data {
		if bytes.Contains(ciphertext, []byte("Private synthetic")) {
			t.Fatal("object provider received readable private upload or metadata")
		}
		for _, body := range source.Blobs {
			if len(body) > 0 && bytes.Contains(ciphertext, body[:min(len(body), 64)]) {
				t.Fatal("object provider received a readable original")
			}
		}
	}
	t.Logf("real PostgreSQL -> native Drive -> maintained Matrix SDK: %d rows, %d binaries, interrupted ACK recovery, hierarchy, exact JSON, download, trash and reopen", len(source.Rows), len(source.Blobs))
}

func exercisePrivateDriveZip(ctx context.Context, t *testing.T, repository *privatedrive.Repository, open func() *privatedrive.Repository, source *syntheticDriveSource) *privatedrive.Repository {
	t.Helper()
	if err := repository.Move(ctx, api.DriveFile, source.File, source.Folder); err != nil {
		t.Fatal(err)
	}
	folder, err := repository.Folder(ctx, source.Folder)
	if err != nil {
		t.Fatal(err)
	}
	file, err := repository.File(ctx, source.File)
	if err != nil {
		t.Fatal(err)
	}
	var expected []byte
	for _, candidate := range source.Blobs {
		sum := sha256.Sum256(candidate)
		if int64(len(candidate)) == file.SizeBytes && hex.EncodeToString(sum[:]) == file.Checksum {
			expected = candidate
			break
		}
	}
	if expected == nil {
		t.Fatal("missing independent oracle for the restored current private revision")
	}
	body, size, err := repository.Zip(ctx, []string{source.File, source.File}, []string{source.Folder, source.Folder}, "private.zip")
	if err != nil || size != int64(len(expected)) {
		t.Fatal("private archive failed to capture a deduplicated selection", size, err)
	}
	if err := repository.Rename(ctx, api.DriveFile, source.File, "Private metadata changed after ZIP capture.txt"); err != nil {
		_ = body.Close()
		t.Fatal(err)
	}
	data, err := io.ReadAll(body)
	_ = body.Close()
	if err != nil {
		t.Fatal("private archive stream failed", err)
	}
	defer clear(data)
	archive, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil || len(archive.File) != 1 || archive.File[0].Name != folder.Name+"/"+file.Name {
		t.Fatal("private archive lost its snapshot or exposed hidden/duplicate entries", err)
	}
	entry, err := archive.File[0].Open()
	if err != nil {
		t.Fatal(err)
	}
	content, err := io.ReadAll(entry)
	_ = entry.Close()
	if err != nil || !bytes.Equal(content, expected) {
		t.Fatal("private archive changed decrypted bytes", err)
	}
	clear(content)
	if err := repository.Rename(ctx, api.DriveFile, source.File, file.Name); err != nil {
		t.Fatal(err)
	}
	if err := repository.Move(ctx, api.DriveFile, source.File, source.Root); err != nil {
		t.Fatal(err)
	}
	unread, _, err := repository.Zip(ctx, []string{source.File}, nil, "unread.zip")
	if err != nil {
		t.Fatal(err)
	}
	closed := make(chan struct{})
	go func() { repository.Close(); close(closed) }()
	select {
	case <-closed:
	case <-time.After(3 * time.Second):
		t.Fatal("private archive blocked repository close without a consumer")
	}
	_ = unread.Close()
	repository = open()
	if err := repository.Initialize(ctx); err != nil {
		t.Fatal(err)
	}
	t.Log("private ZIP streams exact decrypted bytes on device, pins captured revisions across rename, deduplicates overlapping selection, excludes trash and drains on close")
	return repository
}

func exercisePrivateDriveRefresh(ctx context.Context, t *testing.T, writer *privatedrive.Repository, open func() *privatedrive.Repository, source *syntheticDriveSource) {
	t.Helper()
	reader := open()
	defer reader.Close()
	if err := reader.Initialize(ctx); err != nil {
		t.Fatal(err)
	}
	before, err := reader.File(ctx, source.File)
	if err != nil {
		t.Fatal(err)
	}
	name := "Private metadata refreshed from another app.txt"
	if err := writer.Rename(ctx, api.DriveFile, source.File, name); err != nil {
		t.Fatal(err)
	}
	stale, err := reader.File(ctx, source.File)
	if err != nil || stale.Name != before.Name {
		t.Fatal("independent private projection did not retain its snapshot", stale, err)
	}
	if err := reader.Refresh(ctx); err != nil {
		t.Fatal("private refresh failed", err)
	}
	current, err := reader.File(ctx, source.File)
	if err != nil || current.Name != name || current.Label == nil || !current.Label.BlockExternalShare {
		t.Fatal("private refresh lost an authenticated update or sensitivity", current, err)
	}
	if err := writer.Rename(ctx, api.DriveFile, source.File, before.Name); err != nil {
		t.Fatal(err)
	}
	t.Log("explicit private refresh observes another app's authenticated update without reopening legacy source")
}

func exercisePrivateDriveFavorites(ctx context.Context, t *testing.T, repository *privatedrive.Repository, open func() *privatedrive.Repository, faults *lostDriveImporterACK, source *syntheticDriveSource, opaque *fileGatewayFixture) *privatedrive.Repository {
	t.Helper()
	opaque.mu.Lock()
	gets, puts := opaque.gets, opaque.puts
	opaque.mu.Unlock()
	var imported storage.LegacySourceRow
	for _, row := range source.Rows {
		if row.Kind == "favorite" {
			imported = row
		}
	}
	if imported.ID == "" {
		t.Fatal("missing real PostgreSQL favorite")
	}
	check := func(ids ...string) {
		t.Helper()
		page, err := repository.Favorites(ctx)
		if err != nil || len(page) != len(ids) {
			t.Fatal("private favorite list changed", page, ids, err)
		}
		for i, id := range ids {
			if page[i].ID != id || page[i].Path == "" || id == source.File && (page[i].Label == nil || !page[i].Label.BlockExternalShare) {
				t.Fatal("private favorite lost order, path or label", page[i], id)
			}
		}
	}
	check(source.File)
	writes := faults.writes
	if err := repository.SetFavorite(ctx, api.DriveFile, source.File, true); err != nil || faults.writes != writes {
		t.Fatal("repeated private favorite wrote again", err)
	}
	for _, stage := range []string{"content", "catalog", "complete"} {
		faults.contentLost, faults.catalogLost = stage != "content", stage != "catalog"
		err := repository.SetFavorite(ctx, api.DriveFile, source.File, false)
		if (err != nil) != (stage != "complete") {
			t.Fatal("private favorite retry did not preserve accepted events", stage, err)
		}
		repository.Close()
		repository = open()
		if err := repository.Initialize(ctx); err != nil {
			t.Fatal("private favorite did not reopen", err)
		}
	}
	check()
	if err := repository.SetFavorite(ctx, api.DriveFolder, source.Folder, true); err != nil {
		t.Fatal(err)
	}
	if err := repository.SetFavorite(ctx, api.DriveFile, source.File, true); err != nil {
		t.Fatal(err)
	}
	check(source.Folder, source.File)
	raw, err := repository.ReadRaw(ctx, "favorite", imported.ID)
	var original, updated map[string]json.RawMessage
	if err != nil || json.Unmarshal(imported.Payload, &original) != nil || json.Unmarshal(raw, &updated) != nil {
		t.Fatal("favorite identity or original JSON lost", err)
	}
	clear(raw)
	for key, value := range original {
		if key != "created_at" && key != "updated_at" && key != "deleted_at" && !bytes.Equal(value, updated[key]) {
			t.Fatal("private favorite lost an original field", key)
		}
	}
	if err := repository.Move(ctx, api.DriveFile, source.File, source.Folder); err != nil {
		t.Fatal(err)
	}
	if err := repository.Delete(ctx, api.DriveFolder, source.Folder); err != nil {
		t.Fatal(err)
	}
	check()
	writes = faults.writes
	if err := repository.SetFavorite(ctx, api.DriveFile, source.File, true); !errors.Is(err, privatedrive.ErrNotFound) || faults.writes != writes {
		t.Fatal("favorite accepted an inherited trash target", err)
	}
	if err := repository.Restore(ctx, api.DriveFolder, source.Folder); err != nil {
		t.Fatal(err)
	}
	check(source.Folder, source.File)
	if err := repository.Move(ctx, api.DriveFile, source.File, source.Root); err != nil {
		t.Fatal(err)
	}
	writes = faults.writes
	if err := repository.SetFavorite(ctx, api.DriveFolder, source.Root, true); !errors.Is(err, privatedrive.ErrRootImmutable) || faults.writes != writes {
		t.Fatal("private favorite accepted the root", err)
	}
	opaque.mu.Lock()
	afterGets, afterPuts := opaque.gets, opaque.puts
	opaque.mu.Unlock()
	if gets != afterGets || puts != afterPuts {
		t.Fatal("private stars transferred binary media")
	}
	t.Log("private favorites retain PostgreSQL identity, ordering and sensitivity, survive both lost ACKs and reopen, hide inherited trash and use no media transfer")
	return repository
}

func exercisePrivateDriveQueries(ctx context.Context, t *testing.T, repository *privatedrive.Repository, source *syntheticDriveSource, opaque *fileGatewayFixture) {
	t.Helper()
	opaque.mu.Lock()
	gets, puts := opaque.gets, opaque.puts
	opaque.mu.Unlock()
	search := func(want bool) {
		t.Helper()
		found, err := repository.Search(ctx, "  PRÍVATE MÉTADATA RENAMED  ")
		if err != nil {
			t.Fatal(err)
		}
		present := false
		for _, file := range found.Files {
			if file.Label == nil || !file.Label.BlockExternalShare || file.Path == "" {
				t.Fatal("private search lost label or path", file.ID)
			}
			if file.ID == source.File {
				present = true
			}
			if file.ID == source.Trash {
				t.Fatal("private search exposed trash")
			}
		}
		if present != want {
			t.Fatal("private search lost accent folding or inherited visibility", present, want)
		}
	}
	recent := func(want bool) {
		t.Helper()
		page, next, err := repository.Recent(ctx, "")
		if err != nil || len(page) == 0 || next != "" {
			t.Fatal("private recent page changed", len(page), next, err)
		}
		present := false
		for _, file := range page {
			if file.ID == source.File {
				present = true
			}
			if file.ID == source.Trash {
				t.Fatal("private recent exposed trash")
			}
		}
		if present != want {
			t.Fatal("private recent lost inherited visibility", present, want)
		}
	}
	trash := func(folder bool) {
		t.Helper()
		page, err := repository.Trash(ctx)
		if err != nil || page.RetentionSeconds != 0 {
			t.Fatal("private trash claimed an inactive cleanup policy", err)
		}
		foundFolder, foundFile := false, false
		for _, item := range page.Folders {
			if item.ID == source.Folder {
				foundFolder = true
			}
		}
		for _, item := range page.Files {
			if item.ID == source.Trash {
				foundFile = true
				if item.Path == "" {
					t.Fatal("private trash lost its path")
				}
			}
		}
		if foundFolder != folder || foundFile == folder {
			t.Fatal("private trash lost top-level subtree visibility", foundFolder, foundFile, folder)
		}
	}
	search(true)
	recent(true)
	trash(false)
	if err := repository.Move(ctx, api.DriveFile, source.File, source.Folder); err != nil {
		t.Fatal(err)
	}
	if err := repository.Delete(ctx, api.DriveFolder, source.Folder); err != nil {
		t.Fatal(err)
	}
	search(false)
	recent(false)
	trash(true)
	if err := repository.Restore(ctx, api.DriveFolder, source.Folder); err != nil {
		t.Fatal(err)
	}
	search(true)
	recent(true)
	trash(false)
	if err := repository.Move(ctx, api.DriveFile, source.File, source.Root); err != nil {
		t.Fatal(err)
	}
	opaque.mu.Lock()
	afterGets, afterPuts := opaque.gets, opaque.puts
	opaque.mu.Unlock()
	if gets != afterGets || puts != afterPuts {
		t.Fatal("private metadata queries transferred binary media")
	}
	t.Log("private search/recent/trash use device-side paths and accent folding, retain sensitivity snapshots and hide descendants across folder trash/restore without media GET/PUT")
}

func exercisePrivateDriveMetadata(ctx context.Context, t *testing.T, repository *privatedrive.Repository, open func() *privatedrive.Repository, faults *lostDriveImporterACK, source *syntheticDriveSource, opaque *fileGatewayFixture, journal *offline.Store) *privatedrive.Repository {
	t.Helper()
	opaque.mu.Lock()
	puts := opaque.puts
	opaque.mu.Unlock()
	if err := repository.Rename(ctx, api.DriveFolder, source.Root, "changed root"); !errors.Is(err, privatedrive.ErrRootImmutable) {
		t.Fatal("private root renamed", err)
	}
	if err := repository.Rename(ctx, api.DriveFile, source.File, "../invalid"); !errors.Is(err, privatedrive.ErrInvalidName) {
		t.Fatal("invalid private name accepted", err)
	}
	faults.contentLost, faults.catalogLost = false, false
	const name = "Private metadata renamed.bin"
	if err := repository.Rename(ctx, api.DriveFile, source.File, name); err == nil || !faults.contentLost {
		t.Fatal("metadata content ACK loss not exercised", err)
	}
	checkPrivateMetadataJournalBinding(ctx, t, repository, faults, source, journal, name)
	repository.Close()
	bindings := faults.cfg.Bootstrap
	rotated := bindings
	for _, room := range rotated.StorageRoomIDs {
		if room != rotated.StorageRoomID {
			rotated.StorageRoomID = room
			break
		}
	}
	if rotated.StorageRoomID == bindings.StorageRoomID {
		t.Fatal("metadata test requires two retained rooms")
	}
	if err := faults.UpdateBindings(rotated); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := faults.UpdateBindings(bindings); err != nil {
			t.Error(err)
		}
	}()
	repository = open()
	if err := repository.Initialize(ctx); err != nil {
		t.Fatal(err)
	}
	if err := repository.Rename(ctx, api.DriveFile, source.File, name); err == nil || !faults.catalogLost {
		t.Fatal("metadata index ACK loss not exercised", err)
	}
	repository.Close()
	repository = open()
	if err := repository.Initialize(ctx); err != nil {
		t.Fatal(err)
	}
	if err := repository.Rename(ctx, api.DriveFile, source.File, name); err != nil {
		t.Fatal("private rename did not resume with its original target", err)
	}
	renamed, err := repository.File(ctx, source.File)
	if err != nil || renamed.Name != name {
		t.Fatal("private rename not visible", renamed, err)
	}
	if err := repository.Move(ctx, api.DriveFolder, source.Folder, source.Folder); !errors.Is(err, privatedrive.ErrCycle) {
		t.Fatal("private folder cycle accepted", err)
	}
	if err := repository.Move(ctx, api.DriveFile, source.File, source.Folder); err != nil {
		t.Fatal(err)
	}
	children, err := repository.Children(ctx, source.Folder, "")
	if err != nil || len(children.Files) != 1 || children.Files[0].ID != source.File {
		t.Fatal("private move lost hierarchy", children, err)
	}
	if err := repository.Delete(ctx, api.DriveFolder, source.Folder); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.File(ctx, source.File); !errors.Is(err, privatedrive.ErrNotFound) {
		t.Fatal("trashed folder exposed its live descendant", err)
	}
	if _, err := repository.Download(ctx, source.File); !errors.Is(err, privatedrive.ErrNotFound) {
		t.Fatal("trashed subtree still downloadable", err)
	}
	if err := repository.Restore(ctx, api.DriveFolder, source.Folder); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.File(ctx, source.Trash); !errors.Is(err, privatedrive.ErrNotFound) {
		t.Fatal("parent restore resurrected an independently trashed child", err)
	}
	if err := repository.Delete(ctx, api.DriveFile, source.File); err != nil {
		t.Fatal(err)
	}
	if err := repository.Rename(ctx, api.DriveFile, source.File, "rename in trash"); !errors.Is(err, privatedrive.ErrNotFound) {
		t.Fatal("trashed file renamed", err)
	}
	if err := repository.Restore(ctx, api.DriveFile, source.File); err != nil {
		t.Fatal(err)
	}
	if err := repository.Move(ctx, api.DriveFile, source.File, source.Root); err != nil {
		t.Fatal(err)
	}
	body, err := repository.Download(ctx, source.File)
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(body)
	_ = body.Close()
	if err != nil || !bytes.Equal(data, source.Blobs["file/"+source.File]) {
		t.Fatal("metadata edit changed the retained file", err)
	}
	clear(data)
	opaque.mu.Lock()
	after := opaque.puts
	opaque.mu.Unlock()
	if puts != after {
		t.Fatal("metadata-only edit uploaded media again", puts, after)
	}
	repository = exercisePrivateFolderCreation(ctx, t, repository, open, faults, source)
	repository = exercisePrivateFileCopies(ctx, t, repository, open, faults, source, opaque)
	repository = exercisePrivateFileUploads(ctx, t, repository, open, faults, source, journal, opaque)
	t.Log("private rename/move/trash/restore, durable ACK recovery across rooms, atomic subtree visibility and original media reuse verified")
	return repository
}

func exercisePrivateFileUploads(ctx context.Context, t *testing.T, repository *privatedrive.Repository, open func() *privatedrive.Repository, faults *lostDriveImporterACK, source *syntheticDriveSource, journal *offline.Store, opaque *fileGatewayFixture) *privatedrive.Repository {
	t.Helper()
	data := bytes.Repeat([]byte("Private synthetic new upload with non-seekable input\n"), 7000)
	defer clear(data)
	const name = "Private synthetic upload.txt"
	upload := func(content []byte) (api.File, error) {
		return repository.Upload(ctx, source.Root, name, "text/plain", struct{ io.Reader }{bytes.NewReader(content)}, int64(len(content)))
	}
	faults.contentLost, faults.catalogLost = false, false
	if _, err := upload(data); err == nil || !faults.contentLost {
		t.Fatal("upload content ACK loss not exercised", err)
	}
	repository.Close()
	repository = open()
	if err := repository.Initialize(ctx); err != nil {
		t.Fatal(err)
	}
	changed := bytes.Clone(data)
	changed[0] ^= 1
	writes := faults.writes
	if _, err := upload(changed); !errors.Is(err, privatedrive.ErrSource) || faults.writes != writes {
		t.Fatal("changed upload retry accepted or published", err)
	}
	clear(changed)
	if _, err := upload(data); err == nil || !faults.catalogLost {
		t.Fatal("upload index ACK loss not exercised", err)
	}
	repository.Close()
	repository = open()
	if err := repository.Initialize(ctx); err != nil {
		t.Fatal(err)
	}
	resumed, err := repository.ResumeUploads(ctx)
	if err != nil || len(resumed) != 1 {
		t.Fatal("pending private upload did not resume without original input", len(resumed), err)
	}
	file := resumed[0]
	if file.Name != name || file.SizeBytes != int64(len(data)) || file.Checksum == "" || file.Version != 1 {
		t.Fatal("private upload did not resume", file, err)
	}
	body, err := repository.Download(ctx, file.ID)
	if err != nil {
		t.Fatal(err)
	}
	got, err := io.ReadAll(body)
	_ = body.Close()
	if err != nil || !bytes.Equal(got, data) {
		t.Fatal("private uploaded bytes changed", err)
	}
	clear(got)
	replacement := []byte("Private synthetic replacement content\n")
	repository = exerciseCapturedUploadResume(ctx, t, repository, open, faults, source.Root, journal)
	updated, err := repository.Upload(ctx, source.Root, name, "text/plain", struct{ io.Reader }{bytes.NewReader(replacement)}, int64(len(replacement)))
	if err != nil || updated.ID != file.ID || updated.Version != 2 {
		t.Fatal("private replacement did not retain file identity and advance version", updated, err)
	}
	versions, next, err := repository.Versions(ctx, file.ID, "")
	if err != nil || next != "" || len(versions) != 2 || versions[0].Version != 2 || !versions[0].IsCurrent || versions[1].Version != 1 || versions[1].IsCurrent {
		t.Fatal("private version history lost previous upload", versions, next, err)
	}
	body, err = repository.DownloadVersion(ctx, file.ID, 1)
	if err != nil {
		t.Fatal(err)
	}
	got, err = io.ReadAll(body)
	_ = body.Close()
	if err != nil || !bytes.Equal(got, data) {
		t.Fatal("overwriting changed archived private bytes", err)
	}
	clear(got)
	if _, err := repository.DownloadVersion(ctx, file.ID, 2); !errors.Is(err, privatedrive.ErrNotFound) {
		t.Fatal("current content exposed as archived version", err)
	}
	if err := repository.Rename(ctx, api.DriveFile, file.ID, "Private synthetic renamed upload.txt"); err != nil {
		t.Fatal(err)
	}
	versions, _, err = repository.Versions(ctx, file.ID, "")
	if err != nil || len(versions) != 2 {
		t.Fatal("metadata edit manufactured a content version", versions, err)
	}
	legacy, _, err := repository.Versions(ctx, source.File, "")
	if err != nil || len(legacy) != 2 || legacy[0].Version != 2 || legacy[1].Version != 1 {
		t.Fatal("legacy private versions lost", legacy, err)
	}
	body, err = repository.DownloadVersion(ctx, source.File, 1)
	if err != nil {
		t.Fatal(err)
	}
	got, err = io.ReadAll(body)
	_ = body.Close()
	var originalVersion []byte
	for key, value := range source.Blobs {
		if len(key) > 8 && key[:8] == "version/" {
			originalVersion = value
		}
	}
	if err != nil || !bytes.Equal(got, originalVersion) {
		t.Fatal("legacy private archived content changed", err)
	}
	clear(got)
	if err := repository.Delete(ctx, api.DriveFile, file.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.DownloadVersion(ctx, file.ID, 1); !errors.Is(err, privatedrive.ErrNotFound) {
		t.Fatal("trash exposed archived content", err)
	}
	if err := repository.Restore(ctx, api.DriveFile, file.ID); err != nil {
		t.Fatal(err)
	}
	repository = exercisePrivateVersionRestore(ctx, t, repository, open, faults, file.ID, data, replacement, opaque)
	if err := repository.RestoreVersion(ctx, source.File, 1); err != nil {
		t.Fatal("legacy seeded version could not be restored", err)
	}
	seeded, err := repository.File(ctx, source.File)
	if err != nil || seeded.Version != 3 {
		t.Fatal("restoring legacy version lost lineage or counter", seeded, err)
	}
	owner := source.State.Scope.Tenant + "\x00" + source.State.Scope.Subject + "\x00drive"
	rows, err := journal.List(owner, "drive-private-upload-source-v1")
	if err != nil || len(rows) != 0 {
		t.Fatal("confirmed upload retained temporary source", len(rows), err)
	}
	empty, err := repository.Upload(ctx, source.Root, "Private synthetic empty.txt", "", bytes.NewReader(nil), 0)
	if err != nil || empty.SizeBytes != 0 {
		t.Fatal("empty private upload rejected", empty, err)
	}
	body, err = repository.Download(ctx, empty.ID)
	if err != nil {
		t.Fatal(err)
	}
	got, err = io.ReadAll(body)
	_ = body.Close()
	if err != nil || len(got) != 0 {
		t.Fatal("empty private file changed", err)
	}
	repository.Close()
	repository = open()
	if err := repository.Initialize(ctx); err != nil {
		t.Fatal("private upload/version history did not reopen", err)
	}
	body, err = repository.Download(ctx, source.File)
	if err != nil {
		t.Fatal(err)
	}
	got, err = io.ReadAll(body)
	_ = body.Close()
	if err != nil || !bytes.Equal(got, originalVersion) {
		t.Fatal("restored legacy lineage changed after reopen", err)
	}
	clear(got)
	t.Log("private uploads retain only encrypted bounded source chunks, reject changed retries before publication and recover lost ACKs/restarts; replacements preserve identity and archived versions, metadata edits do not create versions, empty input and post-commit source cleanup verified")
	return repository
}

func exerciseCapturedUploadResume(ctx context.Context, t *testing.T, repository *privatedrive.Repository, open func() *privatedrive.Repository, faults *lostDriveImporterACK, root string, journal *offline.Store) *privatedrive.Repository {
	t.Helper()
	input := []byte("Private synthetic captured input whose original is unavailable\n")
	expected := bytes.Clone(input)
	defer clear(expected)
	faults.uploadBlocked = true
	if _, err := repository.Upload(ctx, root, "Private synthetic resumed.txt", "text/plain", struct{ io.Reader }{bytes.NewReader(input)}, int64(len(input))); err == nil || faults.uploadBlocked {
		t.Fatal("capture was not stopped before its first SDK upload", err)
	}
	// Lose access to the original bytes before reopening. Only the encrypted
	// captured source is available to the production resume operation.
	clear(input)
	scope := faults.storageScope()
	owner := scope.Tenant + "\x00" + scope.Subject + "\x00" + scope.Product
	const bad = "upload_0000000000000000000000000000000000000000000000000000000000000000"
	if err := journal.Write(owner, "drive-private-migration-v1", bad, map[string]int{"v": 1}); err != nil {
		t.Fatal(err)
	}
	repository.Close()
	repository = open()
	if err := repository.Initialize(ctx); err != nil {
		t.Fatal(err)
	}
	files, err := repository.ResumeUploads(ctx)
	if !errors.Is(err, privatedrive.ErrSource) || len(files) != 1 {
		t.Fatal("bad intent blocked unrelated captured upload", len(files), err)
	}
	if err := journal.Delete(owner, "drive-private-migration-v1", bad); err != nil {
		t.Fatal(err)
	}
	body, err := repository.Download(ctx, files[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	got, err := io.ReadAll(body)
	_ = body.Close()
	if err != nil || !bytes.Equal(got, expected) {
		t.Fatal("upload resume used changed or unavailable original", err)
	}
	clear(got)
	files, err = repository.ResumeUploads(ctx)
	if err != nil || len(files) != 0 {
		t.Fatal("completed upload resumed twice", len(files), err)
	}
	t.Log("captured private upload resumes after restart without original input; one damaged intent does not block another upload and completed work does not repeat")
	return repository
}

func exercisePrivateVersionRestore(ctx context.Context, t *testing.T, repository *privatedrive.Repository, open func() *privatedrive.Repository, faults *lostDriveImporterACK, id string, first, second []byte, opaque *fileGatewayFixture) *privatedrive.Repository {
	t.Helper()
	restore := func() error {
		opaque.mu.Lock()
		gets, puts := opaque.gets, opaque.puts
		opaque.mu.Unlock()
		err := repository.RestoreVersion(ctx, id, 1)
		opaque.mu.Lock()
		afterGets, afterPuts := opaque.gets, opaque.puts
		opaque.mu.Unlock()
		if gets != afterGets || puts != afterPuts {
			t.Fatal("private version restore transferred media")
		}
		return err
	}
	faults.contentLost, faults.catalogLost = false, false
	if err := restore(); err == nil || !faults.contentLost {
		t.Fatal("version restore content ACK loss not exercised", err)
	}
	repository.Close()
	repository = open()
	if err := repository.Initialize(ctx); err != nil {
		t.Fatal(err)
	}
	if err := restore(); err == nil || !faults.catalogLost {
		t.Fatal("version restore catalog ACK loss not exercised", err)
	}
	repository.Close()
	repository = open()
	if err := repository.Initialize(ctx); err != nil {
		t.Fatal(err)
	}
	if err := restore(); err != nil {
		t.Fatal("private version restore did not resume", err)
	}
	versions, _, err := repository.Versions(ctx, id, "")
	if err != nil || len(versions) != 3 || versions[0].Version != 3 || !versions[0].IsCurrent {
		t.Fatal("restoration did not preserve complete version history", versions, err)
	}
	for _, n := range []int{0, 2} {
		var body io.ReadCloser
		if n == 0 {
			body, err = repository.Download(ctx, id)
		} else {
			body, err = repository.DownloadVersion(ctx, id, n)
		}
		if err != nil {
			t.Fatal(err)
		}
		got, readErr := io.ReadAll(body)
		_ = body.Close()
		want := first
		if n == 2 {
			want = second
		}
		if readErr != nil || !bytes.Equal(got, want) {
			t.Fatal("restored or retained content changed", n, readErr)
		}
		clear(got)
	}
	writes := faults.writes
	if err := repository.RestoreVersion(ctx, id, 99); !errors.Is(err, privatedrive.ErrNotFound) || faults.writes != writes {
		t.Fatal("unknown archived version published", err)
	}
	t.Log("private version restore creates one new version without media transfer, keeps displaced content and survives lost content/index ACKs and restarts")
	return repository
}

func exercisePrivateFileCopies(ctx context.Context, t *testing.T, repository *privatedrive.Repository, open func() *privatedrive.Repository, faults *lostDriveImporterACK, source *syntheticDriveSource, opaque *fileGatewayFixture) *privatedrive.Repository {
	t.Helper()
	opaque.mu.Lock()
	gets, puts := opaque.gets, opaque.puts
	opaque.mu.Unlock()
	first, err := repository.CopyFile(ctx, source.File)
	if err != nil || first.ID == source.File || first.FolderID != source.Root || first.Version != 1 || first.Checksum == "" {
		t.Fatal("private file reference copy failed", first, err)
	}
	if first.Label == nil || !first.Label.BlockExternalShare {
		t.Fatal("private reference copy lost the sensitivity restriction")
	}
	second, err := repository.CopyFile(ctx, source.File)
	if err != nil || second.ID == first.ID || second.Name != first.Name+" (2)" {
		t.Fatal("private copy name or identity duplicated", second, err)
	}
	nested, err := repository.CopyFile(ctx, first.ID)
	if err != nil {
		t.Fatal("copy of a private copy failed", err)
	}
	opaque.mu.Lock()
	nowGets, nowPuts := opaque.gets, opaque.puts
	opaque.mu.Unlock()
	if gets != nowGets || puts != nowPuts {
		t.Fatal("reference copies transferred media", gets, nowGets, puts, nowPuts)
	}
	if err := repository.Rename(ctx, api.DriveFile, first.ID, "Private synthetic renamed copy"); err != nil {
		t.Fatal("copy content identity changed during rename", err)
	}
	if err := repository.Delete(ctx, api.DriveFile, source.File); err != nil {
		t.Fatal(err)
	}
	if err := repository.Delete(ctx, api.DriveFile, first.ID); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{second.ID, nested.ID} {
		body, err := repository.Download(ctx, id)
		if err != nil {
			t.Fatal("source deletion broke an independent copy", err)
		}
		data, err := io.ReadAll(body)
		_ = body.Close()
		if err != nil || !bytes.Equal(data, source.Blobs["file/"+source.File]) {
			t.Fatal("private copy changed original bytes", err)
		}
		clear(data)
	}
	if err := repository.Restore(ctx, api.DriveFile, source.File); err != nil {
		t.Fatal(err)
	}
	faults.contentLost, faults.catalogLost = false, false
	if _, err := repository.CopyFile(ctx, source.File); err == nil || !faults.contentLost {
		t.Fatal("copy content ACK loss not exercised", err)
	}
	repository.Close()
	repository = open()
	if err := repository.Initialize(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.CopyFile(ctx, source.File); err == nil || !faults.catalogLost {
		t.Fatal("copy index ACK loss not exercised", err)
	}
	repository.Close()
	repository = open()
	if err := repository.Initialize(ctx); err != nil {
		t.Fatal(err)
	}
	recovered, err := repository.CopyFile(ctx, source.File)
	if err != nil || recovered.ID == source.File || recovered.ID == second.ID {
		t.Fatal("private copy did not resume its saved identity", recovered, err)
	}
	t.Log("private copies reuse authenticated immutable content without media GET/PUT; nested copies survive source deletion and lost ACKs/restart")
	return repository
}

func exercisePrivateFolderCreation(ctx context.Context, t *testing.T, repository *privatedrive.Repository, open func() *privatedrive.Repository, faults *lostDriveImporterACK, source *syntheticDriveSource) *privatedrive.Repository {
	t.Helper()
	const name = "Private synthetic new folder"
	faults.contentLost, faults.catalogLost = false, false
	if _, err := repository.CreateFolder(ctx, source.Root, name); err == nil || !faults.contentLost {
		t.Fatal("folder creation content ACK loss not exercised", err)
	}
	repository.Close()
	repository = open()
	if err := repository.Initialize(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.CreateFolder(ctx, source.Root, name); err == nil || !faults.catalogLost {
		t.Fatal("folder creation index ACK loss not exercised", err)
	}
	repository.Close()
	repository = open()
	if err := repository.Initialize(ctx); err != nil {
		t.Fatal(err)
	}
	folder, err := repository.CreateFolder(ctx, source.Root, name)
	if err != nil || folder.Name != name || folder.IsRoot || folder.ParentID == nil || *folder.ParentID != source.Root {
		t.Fatal("private folder creation did not resume", folder, err)
	}
	if _, err := repository.CreateFolder(ctx, source.Root, name); !errors.Is(err, privatedrive.ErrConflict) {
		t.Fatal("duplicate private folder name accepted", err)
	}
	if err := repository.Rename(ctx, api.DriveFolder, folder.ID, "Private synthetic renamed folder"); err != nil {
		t.Fatal(err)
	}
	child, err := repository.CreateFolder(ctx, folder.ID, "Private synthetic child")
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.Move(ctx, api.DriveFolder, folder.ID, child.ID); !errors.Is(err, privatedrive.ErrCycle) {
		t.Fatal("private ancestor moved below its child", err)
	}
	children, err := repository.Children(ctx, folder.ID, "")
	if err != nil || len(children.Folders) != 1 || children.Folders[0].ID != child.ID {
		t.Fatal("new private folder hierarchy lost", children, err)
	}
	if err := repository.Delete(ctx, api.DriveFolder, folder.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.Folder(ctx, child.ID); !errors.Is(err, privatedrive.ErrNotFound) {
		t.Fatal("new folder ignored inherited trash", err)
	}
	if err := repository.Restore(ctx, api.DriveFolder, folder.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.Folder(ctx, child.ID); err != nil {
		t.Fatal("private child not restored with parent", err)
	}
	t.Log("private folder creation survives lost content/index ACKs and restart without duplicating identity; names, nested hierarchy and ancestor cycles verified")
	return repository
}

func checkPrivateMetadataJournalBinding(ctx context.Context, t *testing.T, repository *privatedrive.Repository, faults *lostDriveImporterACK, source *syntheticDriveSource, journal *offline.Store, name string) {
	t.Helper()
	scope := faults.storageScope()
	owner := scope.Tenant + "\x00" + scope.Subject + "\x00" + scope.Product
	rows, err := journal.List(owner, "drive-private-migration-v1")
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range rows {
		var work map[string]json.RawMessage
		if json.Unmarshal(row.Value, &work) != nil {
			t.Fatal("invalid private mutation journal")
		}
		mutation, exists := work["mutation"]
		if !exists {
			continue
		}
		original := bytes.Clone(row.Value)
		var item map[string]json.RawMessage
		if json.Unmarshal(mutation, &item) != nil {
			t.Fatal("invalid private mutation intent")
		}
		var binary struct {
			ObjectID  string            `json:"object_id"`
			Reference storage.Reference `json:"reference"`
		}
		if json.Unmarshal(item["binary"], &binary) != nil || binary.ObjectID == "" {
			t.Fatal("missing private source body")
		}
		binary.ObjectID += "changed"
		item["binary"], err = json.Marshal(binary)
		if err != nil {
			t.Fatal(err)
		}
		work["mutation"], err = json.Marshal(item)
		if err != nil {
			t.Fatal(err)
		}
		if err := journal.Write(owner, "drive-private-migration-v1", row.ID, work); err != nil {
			t.Fatal(err)
		}
		before := faults.writes
		err = repository.Rename(ctx, api.DriveFile, source.File, name)
		if !errors.Is(err, privatedrive.ErrSource) || faults.writes != before {
			t.Fatal("changed journal body reached a private publication", err, faults.writes-before)
		}
		err = journal.Write(owner, "drive-private-migration-v1", row.ID, json.RawMessage(original))
		clear(original)
		if err != nil {
			t.Fatal(err)
		}
		return
	}
	t.Fatal("metadata intent was not durable before the first publication")
}
