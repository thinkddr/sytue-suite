// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drive

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"hash"
	"io"
	"slices"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

const (
	migrationNamespace = "drive-private-migration-v1"
	migrationKind      = "org.sytue.drive.migration.v1"
)

type beginWork struct {
	Scope  storage.Scope             `json:"scope"`
	Epoch  string                    `json:"epoch"`
	Target storage.PublicationTarget `json:"target"`
}

type fileWork struct {
	Revision     string              `json:"revision"`
	File         *storage.Reference  `json:"file,omitempty"`
	Catalog      *storage.Reference  `json:"catalog,omitempty"`
	SHA256       string              `json:"sha256,omitempty"`
	Acknowledged bool                `json:"acknowledged,omitempty"`
	Parents      []storage.Reference `json:"parents,omitempty"`
	ParentsSet   bool                `json:"parents_set,omitempty"`
}

type rowWork struct {
	V          int                       `json:"v"`
	Scope      storage.Scope             `json:"scope"`
	Epoch      string                    `json:"epoch"`
	SourceKind string                    `json:"source_kind"`
	SourceID   string                    `json:"source_id"`
	RawSHA256  string                    `json:"raw_sha256"`
	Revision   string                    `json:"revision"`
	Target     storage.PublicationTarget `json:"target"`
	Bodies     map[string]*fileWork      `json:"bodies"`
	Content    *storage.Reference        `json:"content,omitempty"`
	Catalog    *storage.Reference        `json:"catalog,omitempty"`
	Parents    []storage.Reference       `json:"parents,omitempty"`
	ParentsSet bool                      `json:"parents_set,omitempty"`
	Mutation   *metadataIntent           `json:"mutation,omitempty"`
	Creation   *creationIntent           `json:"creation,omitempty"`
	Favorite   *favoriteIntent           `json:"favorite,omitempty"`
}

type completion struct {
	Kind            string `json:"kind"`
	Epoch           string `json:"epoch"`
	Rows            int64  `json:"rows"`
	Blobs           int64  `json:"blobs"`
	InventorySHA256 string `json:"inventory_sha256"`
}

// Initialize imports a retained source through the maintained E2E SDK. It never
// deletes the snapshot, originals, revisions, or source backups. Applications
// expose private data only after this complete inventory has been verified.
func (r *Repository) Initialize(parent context.Context) error {
	ctx, done, err := r.start(parent)
	if err != nil {
		return err
	}
	defer done()
	return r.cfg.Journal.Exclusive(ctx, r.owner(), migrationNamespace, "initialize", func() error { return r.initialize(ctx) })
}

func (r *Repository) prepareMigrationState(ctx context.Context) (storage.Binding, storage.LegacyMigrationState, context.Context, error) {
	// Preparation precedes every source-side effect, including its write fence.
	binding, err := r.cfg.Crypto.PrepareStorage(ctx, r.cfg.Scope)
	if err != nil {
		return storage.Binding{}, storage.LegacyMigrationState{}, nil, err
	}
	if !binding.Valid() || binding.Scope != r.cfg.Scope {
		return storage.Binding{}, storage.LegacyMigrationState{}, nil, ErrSource
	}
	state, err := r.cfg.Source.PrivateStorageState(ctx, r.cfg.Scope)
	if err != nil {
		return storage.Binding{}, storage.LegacyMigrationState{}, nil, err
	}
	if state.Validate(r.cfg.Scope) != nil {
		return storage.Binding{}, storage.LegacyMigrationState{}, nil, ErrSource
	}
	if err := r.refresh(ctx); err != nil {
		return storage.Binding{}, storage.LegacyMigrationState{}, nil, err
	}
	ctx, err = r.observeCurrent(ctx)
	if err != nil {
		return storage.Binding{}, storage.LegacyMigrationState{}, nil, err
	}
	return binding, state, ctx, nil
}

func (r *Repository) initialize(ctx context.Context) error {
	binding, state, ctx, err := r.prepareMigrationState(ctx)
	if err != nil {
		return err
	}
	identity, versioned := r.cfg.Crypto.(interface {
		StorageIdentityVersion() storage.IdentityVersion
	})
	isolated := versioned && identity.StorageIdentityVersion() == storage.IsolatedIdentity
	if r.cfg.DirectImport {
		if !isolated {
			return ErrSource
		}
		return r.initializeDirect(ctx, binding, state)
	}
	if isolated && state.Mode == "legacy" {
		return ErrNotReady
	}
	if state.Mode == "legacy" {
		state, err = r.startLegacyMigration(ctx, binding)
		if err != nil {
			return err
		}
	}
	// Lo que dejaron dos importaciones a la vez (dos copias de una fila, dos
	// recibos) se junta antes de mirar el recibo: si no, no abriría nunca.
	if err := r.mergeImportCopies(ctx, binding, state.Epoch); err != nil {
		return err
	}
	found, err := r.completed(ctx, state)
	if err != nil {
		return err
	}
	if found {
		return r.finishInitialization(ctx, state)
	}
	if isolated {
		return ErrNotReady
	}
	if state.Mode != "frozen" {
		return refuse("source_mode", ErrSource)
	}
	return r.importFrozen(ctx, binding, state)
}

// startLegacyMigration empieza a migrar una fuente todavía en `legacy`: con la
// autorización vigente de la lectura histórica, la congela en una época nueva.
func (r *Repository) startLegacyMigration(ctx context.Context, binding storage.Binding) (storage.LegacyMigrationState, error) {
	operation, err := r.historicalInitialization(ctx)
	if err != nil {
		return storage.LegacyMigrationState{}, err
	}
	if err := operation.authorizeCurrent(ctx); err != nil {
		return storage.LegacyMigrationState{}, err
	}
	return r.beginMigration(ctx, binding)
}

// importFrozen importa la fuente ya congelada, junta las copias repetidas que deje
// otra importación a la vez, publica el recibo y termina de inicializar.
func (r *Repository) importFrozen(ctx context.Context, binding storage.Binding, state storage.LegacyMigrationState) error {
	operation, err := r.historicalInitialization(ctx)
	if err != nil {
		return err
	}
	ctx, err = operation.publicationContext(ctx, binding, state)
	if err != nil {
		return err
	}
	receipt, err := r.importSnapshot(ctx, binding, state)
	if err != nil {
		return err
	}
	if err := r.mergeImportCopies(ctx, binding, state.Epoch); err != nil {
		return err
	}
	if err := r.publishCompletion(ctx, binding, receipt); err != nil {
		return err
	}
	return r.finishInitialization(ctx, state)
}

func (r *Repository) beginMigration(ctx context.Context, binding storage.Binding) (storage.LegacyMigrationState, error) {
	// La fuente volvió a `legacy` (se soltó) con la bóveda llena: importar encima
	// mezclaría dos épocas. Se sale vaciando la bóveda (la puerta lo ofrece).
	if r.hasPrivateItems() {
		return storage.LegacyMigrationState{}, refuse("begin_items", ErrSource)
	}
	key := "begin_" + digest([]byte(binding.Target.DeviceID))
	var work beginWork
	found, err := r.cfg.Journal.Read(r.owner(), migrationNamespace, key, &work)
	if err != nil {
		return storage.LegacyMigrationState{}, err
	}
	if !found {
		work = beginWork{Scope: r.cfg.Scope, Epoch: uuid.NewString(), Target: binding.Target}
		if err := r.cfg.Journal.Write(r.owner(), migrationNamespace, key, work); err != nil {
			return storage.LegacyMigrationState{}, err
		}
	}
	if work.Scope != r.cfg.Scope || !storage.MigrationEpoch(work.Epoch) || work.Target.DeviceID != binding.Target.DeviceID {
		return storage.LegacyMigrationState{}, ErrSource
	}
	if err := r.cfg.Crypto.CheckStoragePublicationTarget(work.Target); err != nil {
		return storage.LegacyMigrationState{}, err
	}
	state, err := r.cfg.Source.BeginPrivateStorageMigration(ctx, r.cfg.Scope, work.Epoch)
	if err != nil {
		return storage.LegacyMigrationState{}, err
	}
	if state.Validate(r.cfg.Scope) != nil || state.Mode != "frozen" || state.Epoch != work.Epoch {
		return storage.LegacyMigrationState{}, ErrSource
	}
	return state, nil
}

type migrationInventory struct {
	state       storage.LegacyMigrationState
	cursor      storage.LegacySourceCursor
	sum         hash.Hash
	rows, blobs int64
}

func (r *Repository) importSnapshot(ctx context.Context, binding storage.Binding, state storage.LegacyMigrationState) (completion, error) {
	inventory := migrationInventory{state: state, sum: sha256.New()}
	_, _ = inventory.sum.Write([]byte("org.sytue.drive.inventory.v1\x00" + state.Epoch + "\x00"))
	for {
		page, err := r.cfg.Source.PrivateStorageSourcePage(ctx, r.cfg.Scope, state.Epoch, inventory.cursor)
		if err != nil {
			return completion{}, err
		}
		if page.Validate(r.cfg.Scope, state.Epoch) != nil {
			return completion{}, ErrSource
		}
		if err := r.importPage(ctx, binding, &inventory, page); err != nil {
			return completion{}, err
		}
		if page.Next == nil {
			break
		}
		if *page.Next != inventory.cursor {
			return completion{}, ErrSource
		}
	}
	if inventory.rows != state.Rows || inventory.blobs != state.Blobs {
		return completion{}, refuse("inventory", ErrSource)
	}
	return completion{Kind: migrationKind, Epoch: state.Epoch, Rows: inventory.rows, Blobs: inventory.blobs, InventorySHA256: hex.EncodeToString(inventory.sum.Sum(nil))}, nil
}

func (r *Repository) importPage(ctx context.Context, binding storage.Binding, inventory *migrationInventory, page storage.LegacySourcePage) error {
	for _, row := range page.Rows {
		cursor := inventory.cursor
		if cursor.Kind != "" && (row.Kind < cursor.Kind || row.Kind == cursor.Kind && row.ID <= cursor.ID) {
			return ErrSource
		}
		if err := r.importRow(ctx, binding, inventory.state.Epoch, row); err != nil {
			return err
		}
		inventoryRow(inventory.sum, row)
		inventory.rows++
		if binaryKind(row.Kind) {
			inventory.blobs++
		}
		if inventory.rows > inventory.state.Rows || inventory.blobs > inventory.state.Blobs {
			return refuse("inventory", ErrSource)
		}
		inventory.cursor = storage.LegacySourceCursor{Kind: row.Kind, ID: row.ID}
	}
	return nil
}

func inventoryRow(out hash.Hash, row storage.LegacySourceRow) {
	_, _ = out.Write([]byte(row.Kind + "\x00" + row.ID + "\x00" + digest(row.Payload) + "\x00"))
}

func (r *Repository) refresh(ctx context.Context) error {
	snapshot, heads, err := r.load(ctx)
	if err != nil {
		return err
	}
	return r.installObservation(ctx, snapshot, heads)
}

func (r *Repository) installObservation(ctx context.Context, snapshot storage.AtomicCatalogSnapshot, heads map[string][]storage.ObjectVersion) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		clearObservation(&observation{snapshot: snapshot})
		return ErrClosed
	}
	if err := ctx.Err(); err != nil {
		clearObservation(&observation{snapshot: snapshot})
		return err
	}
	previous := r.observation
	r.binding, r.entries, r.heads, r.snapshot = snapshot.Binding, snapshot.Entries, heads, snapshot
	r.observation = &observation{snapshot: snapshot}
	if previous != nil {
		previous.retired = true
		if previous.refs == 0 {
			clearObservation(previous)
		}
	}
	return nil
}

func (r *Repository) markReady(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return ErrClosed
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	r.ready = true
	return nil
}

func (r *Repository) hasPrivateItems() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, versions := range r.heads {
		for _, version := range versions {
			var item struct {
				Kind string `json:"kind"`
			}
			if json.Unmarshal(version.Commit.Metadata, &item) == nil && (item.Kind == itemKind || item.Kind == migrationKind || item.Kind == bodyKind) {
				return true
			}
		}
	}
	return false
}

func (r *Repository) versions(object string) []storage.ObjectVersion {
	r.mu.Lock()
	defer r.mu.Unlock()
	versions := make([]storage.ObjectVersion, 0, len(r.heads[object]))
	for _, version := range r.heads[object] {
		version.Commit.Metadata = bytes.Clone(version.Commit.Metadata)
		version.Commit.Parents = slices.Clone(version.Commit.Parents)
		if version.Commit.Content != nil {
			content := *version.Commit.Content
			version.Commit.Content = &content
		}
		versions = append(versions, version)
	}
	return versions
}

func (r *Repository) completed(ctx context.Context, state storage.LegacyMigrationState) (bool, error) {
	versions := r.versions("drive_migration_" + digest([]byte(state.Epoch)))
	if len(versions) == 0 {
		return false, nil
	}
	// Dos importaciones que terminan a la vez dejan dos recibos. Valen si todos
	// dicen lo mismo (época, filas, ficheros e inventario) y cada uno es válido:
	// no hay nada que elegir. Inventarios distintos son lecturas distintas de la
	// fuente: conflicto, como siempre.
	if len(versions) > 16 {
		return false, refuse("receipt", ErrConflict)
	}
	receipts := make([]completion, len(versions))
	for i, version := range versions {
		if json.Unmarshal(version.Commit.Metadata, &receipts[i]) != nil {
			return false, ErrSource
		}
		if receipts[i] != receipts[0] {
			return false, refuse("receipt", ErrConflict)
		}
	}
	for i, version := range versions {
		if err := r.checkReceipt(ctx, state, version, receipts[i]); err != nil {
			return false, err
		}
	}
	return true, nil
}

// checkReceipt comprueba un recibo de la época: que diga lo mismo que la fuente
// congelada y que su registro cifrado sea exactamente el que publica la cabeza.
func (r *Repository) checkReceipt(ctx context.Context, state storage.LegacyMigrationState, version storage.ObjectVersion, receipt completion) error {
	if receipt.Kind != migrationKind || receipt.Epoch != state.Epoch || receipt.Rows != state.Rows || receipt.Blobs != state.Blobs || !validHash(receipt.InventorySHA256) {
		return ErrSource
	}
	pointer := version.Commit.Content
	if version.Commit.State != "live" || pointer == nil || pointer.Kind != "record" {
		return ErrSource
	}
	record, err := r.cfg.Crypto.ReadStorageRecord(ctx, pointer.Reference, version.ObjectID, "content")
	if err != nil {
		return err
	}
	valid := record.Validate(r.cfg.Scope) == nil && record.ObjectID == version.ObjectID && record.RevisionID == pointer.RevisionID && record.Role == "content" && bytes.Equal(record.Payload, version.Commit.Metadata)
	clear(record.Payload)
	if !valid {
		return ErrSource
	}
	return nil
}

func (r *Repository) importRow(ctx context.Context, binding storage.Binding, epoch string, row storage.LegacySourceRow) error {
	ctx, release, err := r.temporaryObservation(ctx)
	if err != nil {
		return err
	}
	defer release()
	item, err := summarize(row, epoch)
	if err != nil {
		return err
	}
	object := objectID(row.Kind, row.ID)
	workID := "row_" + digest([]byte(epoch+"\x00"+row.Kind+"\x00"+row.ID+"\x00"+binding.Target.DeviceID))
	view, err := r.currentObservation(ctx)
	if err != nil {
		return err
	}
	var existing []storage.ObjectVersion
	for _, version := range view.snapshot.Heads {
		if version.ObjectID == object {
			existing = append(existing, version)
		}
	}
	// Las copias idénticas de otra importación ya se juntaron (mergeImportCopies):
	// varias cabezas aquí son revisiones distintas de la fila, un conflicto de verdad.
	if len(existing) > 1 {
		return refuse("row_heads", ErrConflict)
	}
	if len(existing) == 1 {
		return r.confirmImportedRow(ctx, workID, item, row, existing[0])
	}
	work, err := r.rowIntent(ctx, workID, item, binding.Target)
	if err != nil {
		return err
	}
	data, err := r.importEntry(ctx, workID, &work, item, row.Payload)
	if err != nil {
		return err
	}
	defer clear(data.Raw)
	if err := r.publishEntry(ctx, workID, &work, item, data); err != nil {
		return err
	}
	stored, err := r.cfg.Crypto.StorageObject(ctx, *work.Catalog, object)
	if err != nil {
		return err
	}
	fresh, releaseFresh, err := r.temporaryObservation(ctx)
	if err != nil {
		return err
	}
	defer releaseFresh()
	return r.confirmImportedRow(fresh, workID, item, row, stored)
}

func (r *Repository) confirmImportedRow(ctx context.Context, workID string, item summary, row storage.LegacySourceRow, version storage.ObjectVersion) error {
	prior, data, raw, err := r.readEntry(ctx, version)
	defer clear(raw)
	if err != nil {
		return err
	}
	if prior.Epoch != item.Epoch || !bytes.Equal(raw, row.Payload) {
		return refuse("row_mismatch", ErrSource)
	}
	if data.Binary != nil {
		if err := r.readBody(ctx, data.observation, *data.Binary, prior, "binary", io.Discard); err != nil {
			return err
		}
	}
	// An index accepted immediately before losing its ACK is completion proof
	// only after the exact row and its replacement have been read back.
	var work rowWork
	found, err := r.cfg.Journal.Read(r.owner(), migrationNamespace, workID, &work)
	if err != nil {
		return err
	}
	if !found {
		return nil
	}
	if !work.matches(r.cfg.Scope, item) {
		return ErrSource
	}
	return r.cfg.Journal.Delete(r.owner(), migrationNamespace, workID)
}

func (work rowWork) matches(scope storage.Scope, item summary) bool {
	if work.Favorite != nil && (work.Mutation != nil || work.Creation != nil) {
		return false
	}
	return work.V == 1 && work.Scope == scope && work.Epoch == item.Epoch && work.SourceKind == item.SourceKind && work.SourceID == item.SourceID && work.RawSHA256 == item.RawSHA256 && storage.Identifier(work.Revision) && work.Bodies != nil
}

func (r *Repository) rowIntent(ctx context.Context, id string, item summary, target storage.PublicationTarget) (rowWork, error) {
	var work rowWork
	found, err := r.cfg.Journal.Read(r.owner(), migrationNamespace, id, &work)
	if err != nil {
		return rowWork{}, err
	}
	if !found {
		work = rowWork{V: 1, Scope: r.cfg.Scope, Epoch: item.Epoch, SourceKind: item.SourceKind, SourceID: item.SourceID, RawSHA256: item.RawSHA256, Revision: uuid.NewString(), Target: target, Bodies: map[string]*fileWork{}}
		if err := r.saveWork(ctx, id, work); err != nil {
			return rowWork{}, err
		}
	}
	if !work.matches(r.cfg.Scope, item) {
		return rowWork{}, ErrSource
	}
	if err := r.cfg.Crypto.CheckStoragePublicationTarget(work.Target); err != nil {
		return rowWork{}, err
	}
	return work, nil
}

func (r *Repository) importEntry(ctx context.Context, id string, work *rowWork, item summary, raw []byte) (entry, error) {
	data := entry{Kind: entryKind, SourceKind: item.SourceKind, SourceID: item.SourceID}
	if binaryKind(item.SourceKind) {
		source, err := openSource(ctx, r.cfg.Source, r.cfg.Scope, item.Epoch, item.SourceKind, item.SourceID, item.SizeBytes, item.Checksum)
		if err != nil {
			return entry{}, err
		}
		defer func() { _ = source.Close() }()
		pointer, err := r.ensureBody(ctx, id, work, item, "binary", source, item.SizeBytes, source.digest)
		if err != nil {
			return entry{}, err
		}
		data.Binary = &pointer
	}
	if len(raw) <= inlineSourceBytes {
		data.Raw = bytes.Clone(raw)
		return data, nil
	}
	raw = bytes.Clone(raw)
	defer clear(raw)
	pointer, err := r.ensureBody(ctx, id, work, item, "metadata", bytes.NewReader(raw), int64(len(raw)), func() (string, error) { return digest(raw), nil })
	if err != nil {
		return entry{}, err
	}
	data.Metadata = &pointer
	return data, nil
}

func (r *Repository) publishEntry(ctx context.Context, id string, work *rowWork, item summary, data entry) error {
	payload, err := json.Marshal(data)
	if err != nil {
		return err
	}
	defer clear(payload)
	object := objectID(item.SourceKind, item.SourceID)
	if work.Content == nil {
		ref, err := r.putRecord(ctx, work.Target, work.Revision+"_content", storage.Record{Version: 1, Scope: r.cfg.Scope, ObjectID: object, RevisionID: work.Revision, Role: "content", Payload: payload})
		if err != nil {
			return err
		}
		work.Content = &ref
		if err := r.saveWork(ctx, id, *work); err != nil {
			return err
		}
	}
	if work.Catalog != nil {
		return nil
	}
	metadata, err := json.Marshal(item)
	if err != nil {
		return err
	}
	state := "live"
	if item.DeletedAt != nil {
		state = "trash"
	}
	if !work.ParentsSet {
		work.Parents, err = r.parents(object)
		if err != nil {
			return err
		}
		work.ParentsSet = true
		if err := r.saveWork(ctx, id, *work); err != nil {
			return err
		}
	}
	version, err := r.putObject(ctx, work.Target, work.Revision+"_index", object, work.Revision+"_index", storage.ObjectCommit{Kind: storage.ObjectKind, State: state, Metadata: metadata, Content: &storage.ContentPointer{Kind: "record", RevisionID: work.Revision, Reference: *work.Content}, Parents: work.Parents})
	if err != nil {
		return err
	}
	work.Catalog = &version.Reference
	return r.saveWork(ctx, id, *work)
}

func (r *Repository) saveWork(ctx context.Context, id string, work rowWork) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := r.cfg.Crypto.CheckStoragePublicationTarget(work.Target); err != nil {
		return err
	}
	return r.cfg.Journal.Write(r.owner(), migrationNamespace, id, work)
}

func (r *Repository) ensureBody(ctx context.Context, id string, work *rowWork, item summary, purpose string, source io.ReaderAt, size int64, sourceDigest func() (string, error)) (bodyPointer, error) {
	object := bodyID(item.SourceKind, item.SourceID, purpose)
	file := work.Bodies[purpose]
	if file == nil {
		file = &fileWork{Revision: uuid.NewString()}
		work.Bodies[purpose] = file
		if err := r.saveWork(ctx, id, *work); err != nil {
			return bodyPointer{}, err
		}
	}
	if !storage.Identifier(file.Revision) {
		return bodyPointer{}, ErrSource
	}
	job := bodyJob(*work, purpose, file.Revision)
	if file.File == nil {
		if err := r.uploadBody(ctx, id, job, work, file, item, purpose, source, size, sourceDigest); err != nil {
			return bodyPointer{}, err
		}
	}
	if !file.File.Valid() || !validHash(file.SHA256) {
		return bodyPointer{}, ErrSource
	}
	if file.Catalog == nil {
		if err := r.publishBody(ctx, id, work, file, item, purpose, size); err != nil {
			return bodyPointer{}, err
		}
	}
	pointer := bodyPointer{ObjectID: object, Reference: *file.Catalog}
	if !file.Acknowledged {
		err := r.cfg.Crypto.AcknowledgeStorageFile(ctx, r.cfg.Journal, job, *file.Catalog)
		if errors.Is(err, storage.ErrUploadMissing) {
			err = r.readBody(ctx, nil, pointer, item, purpose, io.Discard)
		}
		if err != nil {
			return bodyPointer{}, err
		}
		file.Acknowledged = true
		if err := r.saveWork(ctx, id, *work); err != nil {
			return bodyPointer{}, err
		}
	}
	return pointer, nil
}

func bodyJob(work rowWork, purpose, revision string) string {
	object := bodyID(work.SourceKind, work.SourceID, purpose)
	return "body_" + digest([]byte(work.Epoch+"\x00"+object+"\x00"+revision))
}

func (r *Repository) uploadBody(ctx context.Context, id, job string, work *rowWork, file *fileWork, item summary, purpose string, source io.ReaderAt, size int64, sourceDigest func() (string, error)) error {
	metadata, err := json.Marshal(map[string]string{"kind": bodyKind, "source_kind": item.SourceKind, "source_id": item.SourceID, "purpose": purpose, "epoch": item.Epoch})
	if err != nil {
		return err
	}
	object := bodyID(item.SourceKind, item.SourceID, purpose)
	ref, err := r.cfg.Crypto.UploadStorageFileAt(ctx, r.cfg.Gateway, r.cfg.Journal, job, work.Target, storage.FileRequest{ObjectID: object, RevisionID: file.Revision, SizeBytes: size, Metadata: metadata}, source)
	if err != nil {
		return err
	}
	sum, err := sourceDigest()
	if err != nil || !validHash(sum) {
		return ErrSource
	}
	file.File, file.SHA256 = &ref, sum
	return r.saveWork(ctx, id, *work)
}

func (r *Repository) publishBody(ctx context.Context, id string, work *rowWork, file *fileWork, item summary, purpose string, size int64) error {
	object := bodyID(item.SourceKind, item.SourceID, purpose)
	metadata, err := json.Marshal(bodySummary{Kind: bodyKind, SourceKind: item.SourceKind, SourceID: item.SourceID, Epoch: item.Epoch, Purpose: purpose, SHA256: file.SHA256, SizeBytes: size})
	if err != nil {
		return err
	}
	if !file.ParentsSet {
		file.Parents, err = r.parents(object)
		if err != nil {
			return err
		}
		file.ParentsSet = true
		if err := r.saveWork(ctx, id, *work); err != nil {
			return err
		}
	}
	version, err := r.putObject(ctx, work.Target, file.Revision+"_index", object, file.Revision+"_index", storage.ObjectCommit{Kind: storage.ObjectKind, State: "live", Metadata: metadata, Content: &storage.ContentPointer{Kind: "file", RevisionID: file.Revision, Reference: *file.File}, Parents: file.Parents})
	if err != nil {
		return err
	}
	file.Catalog = &version.Reference
	return r.saveWork(ctx, id, *work)
}

func (r *Repository) publishCompletion(ctx context.Context, binding storage.Binding, receipt completion) error {
	object := "drive_migration_" + digest([]byte(receipt.Epoch))
	key := "completion_" + digest([]byte(receipt.Epoch+"\x00"+binding.Target.DeviceID))
	var work rowWork
	found, err := r.cfg.Journal.Read(r.owner(), migrationNamespace, key, &work)
	if err != nil {
		return err
	}
	if !found {
		work = rowWork{V: 1, Scope: r.cfg.Scope, Epoch: receipt.Epoch, Revision: uuid.NewString(), Target: binding.Target, Bodies: map[string]*fileWork{}}
		if err := r.saveWork(ctx, key, work); err != nil {
			return err
		}
	}
	if work.V != 1 || work.Scope != r.cfg.Scope || work.Epoch != receipt.Epoch || !storage.Identifier(work.Revision) {
		return ErrSource
	}
	payload, err := json.Marshal(receipt)
	if err != nil {
		return err
	}
	if work.Content == nil {
		ref, err := r.putRecord(ctx, work.Target, work.Revision+"_content", storage.Record{Version: 1, Scope: r.cfg.Scope, ObjectID: object, RevisionID: work.Revision, Role: "content", Payload: payload})
		if err != nil {
			return err
		}
		work.Content = &ref
		if err := r.saveWork(ctx, key, work); err != nil {
			return err
		}
	}
	if !work.ParentsSet {
		work.Parents, err = r.parents(object)
		if err != nil {
			return err
		}
		work.ParentsSet = true
		if err := r.saveWork(ctx, key, work); err != nil {
			return err
		}
	}
	_, err = r.putObject(ctx, work.Target, work.Revision+"_index", object, work.Revision+"_index", storage.ObjectCommit{Kind: storage.ObjectKind, State: "live", Metadata: payload, Content: &storage.ContentPointer{Kind: "record", RevisionID: work.Revision, Reference: *work.Content}, Parents: work.Parents})
	if err != nil {
		return err
	}
	return r.cfg.Journal.Delete(r.owner(), migrationNamespace, key)
}
