// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package office

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/buscar"
	"git.sytue.eu/sytue/one/apps/nucleo/coedicion"
	"git.sytue.eu/sytue/one/apps/nucleo/office/ancla"
	"git.sytue.eu/sytue/one/apps/nucleo/office/model"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/records"
)

// Config configures the encrypted local repository for Sytue Docs, Calcus, or Diapos.
type Config = records.Config

// Repository stores private office documents, spreadsheets, or presentations
// in the shared encrypted records engine.
type Repository struct {
	core  *records.Repository
	scope storage.Scope
	// Idioma es la lengua que se declara en el PDF accesible exportado (PDF/UA pide el
	// idioma del documento); vacío, «es», lo de siempre. Lo pone quien abre el repositorio
	// con el idioma de la app (O-11).
	Idioma string
}

var (
	_ api.DocsData   = (*Repository)(nil)
	_ api.SlidesData = (*Repository)(nil)
)

// New creates an encrypted office repository for "docs", "sheets", or "slides".
func New(cfg Config) (*Repository, error) {
	configured, err := recordsConfiguration(cfg)
	if err != nil {
		return nil, err
	}
	core, err := records.New(configured)
	if err != nil {
		return nil, err
	}
	return &Repository{core: core, scope: configured.Scope}, nil
}

// NewHistoricalSource validates historical source snapshots during identity
// cutover using the exact same office partition and binary rules.
func NewHistoricalSource(cfg Config) (*records.HistoricalSource, error) {
	configured, err := recordsConfiguration(cfg)
	if err != nil {
		return nil, err
	}
	return records.NewHistoricalSource(configured)
}

// NewDocsSource wraps a Docs HTTP client for one-way migration of "docs".
func NewDocsSource(client *api.Docs) (records.Source, error) {
	if client == nil {
		return nil, model.ErrSource
	}
	return records.NewAppSource(client)
}

// NewSheetsSource wraps a Docs HTTP client for one-way migration of "sheets".
func NewSheetsSource(client *api.Docs) (records.Source, error) {
	if client == nil {
		return nil, model.ErrSource
	}
	return records.NewAppSource(client.SheetsSource())
}

// NewSlidesSource wraps a Slides HTTP client for one-way migration of "slides".
func NewSlidesSource(client *api.Slides) (records.Source, error) {
	if client == nil {
		return nil, model.ErrSource
	}
	return records.NewAppSource(client)
}

func recordsConfiguration(cfg Config) (Config, error) {
	scope := cfg.Scope
	if _, ok := model.ProductKind(scope.Product); !ok || !scope.Valid() {
		return cfg, model.ErrSource
	}
	cfg.Product = scope.Product
	cfg.EntityKind = "org.sytue." + scope.Product + ".entity.v1"
	cfg.ReceiptKind = "org.sytue." + scope.Product + ".snapshot.v1"
	cfg.BinaryKind = func(kind string) bool {
		return kind == model.BinarySourceKind
	}
	cfg.ValidateBinary = func(_ context.Context, row records.SnapshotRow) error {
		if row.Kind != model.BinarySourceKind || !storage.Identifier(row.ID) {
			return model.ErrSource
		}
		return row.Validate()
	}
	cfg.ValidateRow = func(_ context.Context, kind, id string, raw []byte) error {
		return model.ValidateSourceRow(scope, kind, id, raw)
	}
	cfg.ValidateCatalog = func(ctx context.Context, catalog records.Catalog) error {
		return validateOfficeCatalog(ctx, scope, catalog)
	}
	return cfg, nil
}

func validateOfficeCatalog(ctx context.Context, scope storage.Scope, catalog records.Catalog) error {
	entries, err := catalog.Entries()
	if err != nil {
		return err
	}
	expectedBinaries := make(map[string]int64)
	actualBinaries := make(map[string]int64)
	for _, entry := range entries {
		if entry.Binary {
			if err := recordCatalogBinary(ctx, catalog, entry, actualBinaries); err != nil {
				return err
			}
			continue
		}
		if err := validateCatalogJSONEntry(ctx, scope, catalog, entry, expectedBinaries); err != nil {
			return err
		}
	}
	if len(actualBinaries) != len(expectedBinaries) {
		return model.ErrSource
	}
	for id, wantSize := range expectedBinaries {
		gotSize, ok := actualBinaries[id]
		if !ok || gotSize != wantSize {
			return model.ErrSource
		}
	}
	return ctx.Err()
}

func recordCatalogBinary(ctx context.Context, catalog records.Catalog, entry records.Entry, actual map[string]int64) error {
	if entry.Kind != model.BinarySourceKind {
		return model.ErrSource
	}
	binaryCatalog, ok := catalog.(records.BinaryCatalog)
	if !ok {
		return model.ErrSource
	}
	descriptor, err := binaryCatalog.BinaryInfo(ctx, entry)
	if err != nil {
		return err
	}
	actual[entry.ID] = descriptor.Size
	return nil
}

func validateCatalogJSONEntry(ctx context.Context, scope storage.Scope, catalog records.Catalog, entry records.Entry, expected map[string]int64) error {
	raw, err := catalog.ReadJSON(ctx, entry)
	if err != nil {
		return err
	}
	defer clear(raw)
	if _, err := model.PartitionSourceRow(scope, entry.Kind, entry.ID, raw); err != nil {
		return err
	}
	if entry.Kind != "snapshot" && entry.Kind != "blob" {
		return nil
	}
	var object struct {
		Key  string `json:"blob_key"`
		Size int64  `json:"size_bytes"`
	}
	if json.Unmarshal(raw, &object) != nil {
		return model.ErrSource
	}
	if object.Key == "" {
		if entry.Kind == "blob" {
			return model.ErrSource
		}
		return nil
	}
	binID, err := model.BinarySourceID(entry.Kind, entry.ID)
	if err != nil {
		return err
	}
	expected[binID] = object.Size
	return nil
}

// PrivateStorageScope returns the account scope bound to this repository.
func (r *Repository) PrivateStorageScope() storage.Scope {
	if r == nil {
		return storage.Scope{}
	}
	return r.scope
}

// Initialize opens an authenticated, completely imported office catalog.
func (r *Repository) Initialize(ctx context.Context) error {
	if r == nil || r.core == nil {
		return model.ErrSource
	}
	return r.core.Initialize(ctx)
}

// ImportLegacySnapshot imports the retained office snapshot.
func (r *Repository) ImportLegacySnapshot(ctx context.Context) error {
	if r == nil || r.core == nil {
		return model.ErrSource
	}
	return r.core.ImportLegacySnapshot(ctx)
}

// RefreshPrivateData refreshes the office catalog from the encrypted room.
func (r *Repository) RefreshPrivateData(ctx context.Context) error {
	if r == nil || r.core == nil {
		return model.ErrSource
	}
	return r.core.RefreshPrivateData(ctx)
}

// IdentityCutover binds the historical identity migration for this office product.
func (r *Repository) IdentityCutover(cfg records.IdentityCutoverConfig) (*records.IdentityCutover, error) {
	if r == nil || r.core == nil || cfg.Scope != r.scope || cfg.Isolated != nil {
		return nil, model.ErrSource
	}
	cfg.Isolated = r.core
	return records.NewIdentityCutover(cfg)
}

// Drain waits for active work to finish and blocks new work.
func (r *Repository) Drain(ctx context.Context) error {
	if r == nil || r.core == nil {
		return model.ErrSource
	}
	return r.core.Drain(ctx)
}

// Resume reopens the work gate after a cancelled drain.
func (r *Repository) Resume() error {
	if r == nil || r.core == nil {
		return model.ErrSource
	}
	return r.core.Resume()
}

// Pending reports whether durable journal mutations are awaiting publication.
func (r *Repository) Pending(ctx context.Context) (offline.PendingWork, error) {
	if r == nil || r.core == nil {
		return offline.PendingWork{}, model.ErrSource
	}
	return r.core.Pending(ctx)
}

// Close releases all decrypted catalog state and closes the work gate.
func (r *Repository) Close() {
	if r != nil && r.core != nil {
		r.core.Close()
	}
}

type storedDocument struct {
	ID                string     `json:"id"`
	Tenant            string     `json:"tenant"`
	Kind              string     `json:"kind"`
	Title             string     `json:"title"`
	OwnerSub          string     `json:"owner_sub"`
	Seq               int64      `json:"seq"`
	CurrentSnapshotID *string    `json:"current_snapshot_id,omitempty"`
	Starred           bool       `json:"starred,omitempty"`
	SourceRef         *string    `json:"source_ref,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
	DeletedAt         *time.Time `json:"deleted_at,omitempty"`
}

func (d storedDocument) toAPIDocument() api.Document {
	var sourceRef string
	if d.SourceRef != nil {
		sourceRef = *d.SourceRef
	}
	return api.Document{
		ID:        d.ID,
		Kind:      api.DocKind(d.Kind),
		Title:     d.Title,
		OwnerSub:  d.OwnerSub,
		Seq:       d.Seq,
		Starred:   d.Starred,
		CreatedAt: d.CreatedAt,
		UpdatedAt: d.UpdatedAt,
		SourceRef: sourceRef,
		DeletedAt: d.DeletedAt,
	}
}

type storedSnapshot struct {
	ID         string          `json:"id"`
	DocumentID string          `json:"document_id"`
	Tenant     string          `json:"tenant"`
	Seq        int64           `json:"seq"`
	State      json.RawMessage `json:"state,omitempty"`
	BlobKey    *string         `json:"blob_key,omitempty"`
	SizeBytes  int64           `json:"size_bytes"`
}

type storedOperation struct {
	DocumentID string          `json:"document_id"`
	Seq        int64           `json:"seq"`
	Tenant     string          `json:"tenant"`
	Actor      string          `json:"actor"`
	AuthorSub  string          `json:"author_sub"`
	Lamport    int64           `json:"lamport"`
	Op         json.RawMessage `json:"op"`
	CreatedAt  time.Time       `json:"created_at,omitempty"`
}

type storedVersion struct {
	ID         string    `json:"id"`
	DocumentID string    `json:"document_id"`
	Tenant     string    `json:"tenant"`
	SnapshotID string    `json:"snapshot_id"`
	Label      string    `json:"label"`
	AuthorSub  string    `json:"author_sub"`
	Seq        int64     `json:"seq"`
	Auto       bool      `json:"auto,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

type storedComment struct {
	ID         string          `json:"id"`
	DocumentID string          `json:"document_id"`
	Tenant     string          `json:"tenant"`
	ParentID   *string         `json:"parent_id,omitempty"`
	AuthorSub  string          `json:"author_sub"`
	Anchor     json.RawMessage `json:"anchor,omitempty"`
	Body       string          `json:"body"`
	Resolved   bool            `json:"resolved"`
	CreatedAt  time.Time       `json:"created_at"`
	UpdatedAt  time.Time       `json:"updated_at"`
	DeletedAt  *time.Time      `json:"deleted_at,omitempty"`
}

type storedPermission struct {
	DocumentID    string     `json:"document_id"`
	Tenant        string     `json:"tenant"`
	PrincipalKind string     `json:"principal_kind"`
	PrincipalID   string     `json:"principal_id"`
	Role          string     `json:"role"`
	GrantedBy     string     `json:"granted_by"`
	DeletedAt     *time.Time `json:"deleted_at,omitempty"`
}

type storedShareLink struct {
	ID         string     `json:"id"`
	DocumentID string     `json:"document_id"`
	Tenant     string     `json:"tenant"`
	Role       string     `json:"role"`
	CreatedBy  string     `json:"created_by"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
	RevokedAt  *time.Time `json:"revoked_at,omitempty"`
	DeletedAt  *time.Time `json:"deleted_at,omitempty"`
}

func notFoundError(id string) error {
	return &api.Error{Status: http.StatusNotFound, URL: "/documents/" + id}
}

func sourceObjectID(kind string, fields map[string]any) (string, error) {
	primary := make(map[string]json.RawMessage, len(fields))
	for k, v := range fields {
		raw, err := json.Marshal(v)
		if err != nil {
			return "", err
		}
		primary[k] = raw
	}
	return model.SourceObjectID(kind, primary)
}

func idSourceObjectID(kind, id string) (string, error) {
	return sourceObjectID(kind, map[string]any{"id": id})
}

func opSourceObjectID(docID string, seq int64) (string, error) {
	return sourceObjectID("operation", map[string]any{"document_id": docID, "seq": seq})
}

func permSourceObjectID(docID, kind, principal string) (string, error) {
	return sourceObjectID("permission", map[string]any{
		"document_id":    docID,
		"principal_kind": kind,
		"principal_id":   principal,
	})
}

func (r *Repository) readDocumentRow(ctx context.Context, id string) (storedDocument, map[string]any, string, error) {
	objID, err := idSourceObjectID("document", id)
	if err != nil {
		return storedDocument{}, nil, "", notFoundError(id)
	}
	raw, err := r.core.ReadRaw(ctx, "document", objID)
	if errors.Is(err, records.ErrNotFound) {
		return storedDocument{}, nil, "", notFoundError(id)
	}
	if err != nil {
		return storedDocument{}, nil, "", err
	}
	defer clear(raw)
	var doc storedDocument
	if err := json.Unmarshal(raw, &doc); err != nil {
		return storedDocument{}, nil, "", err
	}
	var rawMap map[string]any
	if err := json.Unmarshal(raw, &rawMap); err != nil {
		return storedDocument{}, nil, "", err
	}
	if doc.DeletedAt != nil {
		return storedDocument{}, nil, "", notFoundError(id)
	}
	return doc, rawMap, objID, nil
}

func (r *Repository) loadSnapshotState(ctx context.Context, snapID string) (*model.State, error) {
	snapObjID, err := idSourceObjectID("snapshot", snapID)
	if err != nil {
		return nil, err
	}
	raw, err := r.core.ReadRaw(ctx, "snapshot", snapObjID)
	if err != nil {
		return nil, err
	}
	defer clear(raw)
	var snap storedSnapshot
	if err := json.Unmarshal(raw, &snap); err != nil {
		return nil, err
	}
	if snap.BlobKey != nil && *snap.BlobKey != "" {
		binID, err := model.BinarySourceID("snapshot", snapObjID)
		if err != nil {
			return nil, err
		}
		var buf bytes.Buffer
		if _, err := r.core.ReadBinary(ctx, model.BinarySourceKind, binID, &buf); err != nil {
			clear(buf.Bytes())
			return nil, err
		}
		defer clear(buf.Bytes())
		return model.StateFromJSON(buf.Bytes())
	}
	if len(snap.State) == 0 {
		return model.NewState(), nil
	}
	return model.StateFromJSON(snap.State)
}

func (r *Repository) loadDocumentState(ctx context.Context, doc storedDocument) (*model.State, error) {
	state := model.NewState()
	if doc.CurrentSnapshotID != nil && *doc.CurrentSnapshotID != "" {
		loaded, err := r.loadSnapshotState(ctx, *doc.CurrentSnapshotID)
		if err != nil {
			return nil, err
		}
		state = loaded
	}
	ops, err := r.readOperationsForDocument(ctx, doc.ID, state.Seq, doc.Seq)
	if err != nil {
		return nil, err
	}
	for _, opRow := range ops {
		var op model.Op
		if err := json.Unmarshal(opRow.Op, &op); err != nil {
			return nil, err
		}
		if _, err := state.Apply(op); err != nil {
			return nil, err
		}
		state.Seq = opRow.Seq
	}
	if doc.Seq > state.Seq {
		state.Seq = doc.Seq
	}
	return state, nil
}

func (r *Repository) readOperationsForDocument(ctx context.Context, docID string, afterSeq, maxSeq int64) ([]storedOperation, error) {
	versions, err := r.core.Selected("operation")
	if err != nil {
		return nil, err
	}
	var ops []storedOperation
	for _, version := range versions {
		if version.Commit.State != "live" {
			continue
		}
		raw, err := r.core.ReadJSON(ctx, version)
		if err != nil {
			return nil, err
		}
		var opRow storedOperation
		err = json.Unmarshal(raw, &opRow)
		clear(raw)
		if err != nil {
			return nil, err
		}
		if opRow.DocumentID != docID || opRow.Seq <= afterSeq || (maxSeq > 0 && opRow.Seq > maxSeq) {
			continue
		}
		ops = append(ops, opRow)
	}
	slices.SortFunc(ops, func(a, b storedOperation) int {
		return cmp.Compare(a.Seq, b.Seq)
	})
	return ops, nil
}

func (r *Repository) writeInlineSnapshot(ctx context.Context, docID string, seq int64, state *model.State) (string, error) {
	stateBytes, err := state.JSON()
	if err != nil {
		return "", err
	}
	snapID := uuid.NewString()
	snapObjID, err := idSourceObjectID("snapshot", snapID)
	if err != nil {
		return "", err
	}
	snap := storedSnapshot{
		ID:         snapID,
		DocumentID: docID,
		Tenant:     r.scope.Tenant,
		Seq:        seq,
		State:      stateBytes,
		SizeBytes:  int64(len(stateBytes)),
	}
	raw, err := r.core.Mutate(ctx, "snapshot-write", snap, func(_ context.Context) (string, string, string, []byte, error) {
		encoded, err := json.Marshal(snap)
		return "snapshot", snapObjID, "live", encoded, err
	})
	clear(raw)
	if err != nil {
		return "", err
	}
	return snapID, nil
}

// ListDocuments lists live documents matching f.
func (r *Repository) ListDocuments(ctx context.Context, f api.DocsFilter) ([]api.Document, string, error) {
	versions, err := r.core.Selected("document")
	if err != nil {
		return nil, "", err
	}
	query := strings.TrimSpace(f.Query)
	var docs []api.Document
	for _, version := range versions {
		if version.Commit.State != "live" {
			continue
		}
		raw, err := r.core.ReadJSON(ctx, version)
		if err != nil {
			return nil, "", err
		}
		var d storedDocument
		err = json.Unmarshal(raw, &d)
		clear(raw)
		if err != nil {
			return nil, "", err
		}
		if matchesDocumentFilter(d, f, query) {
			docs = append(docs, d.toAPIDocument())
		}
	}
	slices.SortFunc(docs, func(a, b api.Document) int {
		if c := b.UpdatedAt.Compare(a.UpdatedAt); c != 0 {
			return c
		}
		return cmp.Compare(a.ID, b.ID)
	})
	return paginateDocuments(docs, f.Cursor, f.Limit), "", nil
}

func matchesDocumentFilter(d storedDocument, f api.DocsFilter, query string) bool {
	if d.DeletedAt != nil {
		return false
	}
	if f.Kind != "" && api.DocKind(d.Kind) != f.Kind {
		return false
	}
	if f.Starred && !d.Starred {
		return false
	}
	if query != "" && !buscar.Contiene(d.Title, query) {
		return false
	}
	return true
}

func paginateDocuments(docs []api.Document, cursor string, limit int) []api.Document {
	if cursor != "" {
		idx := slices.IndexFunc(docs, func(d api.Document) bool { return d.ID == cursor })
		if idx >= 0 {
			docs = docs[idx+1:]
		}
	}
	if limit <= 0 {
		limit = 50
	} else if limit > 200 {
		limit = 200
	}
	if len(docs) > limit {
		docs = docs[:limit]
	}
	return docs
}

// CreateDocumentFromSource creates or reuses a document anchored to fuente.
func (r *Repository) CreateDocumentFromSource(ctx context.Context, kind api.DocKind, titulo, fuente string) (api.Document, bool, error) {
	wantKind, ok := model.ProductKind(r.scope.Product)
	if !ok {
		return api.Document{}, false, model.ErrSource
	}
	if kind == "" {
		kind = api.DocKind(wantKind)
	}
	if string(kind) != string(wantKind) {
		return api.Document{}, false, model.ErrInvalidKind
	}
	fuente = strings.TrimSpace(fuente)
	if fuente != "" {
		existing, found, err := r.DocumentBySource(ctx, fuente)
		if err != nil || found {
			return existing, false, err
		}
	}
	now := time.Now().UTC()
	id := uuid.NewString()
	objID, err := idSourceObjectID("document", id)
	if err != nil {
		return api.Document{}, false, err
	}
	var sourcePtr *string
	if fuente != "" {
		sourcePtr = &fuente
	}
	doc := storedDocument{
		ID:        id,
		Tenant:    r.scope.Tenant,
		Kind:      string(kind),
		Title:     titulo,
		OwnerSub:  r.scope.Subject,
		Seq:       0,
		Starred:   false,
		SourceRef: sourcePtr,
		CreatedAt: now,
		UpdatedAt: now,
	}
	raw, err := r.core.Mutate(ctx, "document-create", doc, func(_ context.Context) (string, string, string, []byte, error) {
		encoded, err := json.Marshal(doc)
		return "document", objID, "live", encoded, err
	})
	clear(raw)
	if err != nil {
		return api.Document{}, false, err
	}
	return doc.toAPIDocument(), true, nil
}

// DocumentBySource finds a live document created from fuente.
func (r *Repository) DocumentBySource(ctx context.Context, fuente string) (api.Document, bool, error) {
	fuente = strings.TrimSpace(fuente)
	if fuente == "" {
		return api.Document{}, false, nil
	}
	versions, err := r.core.Selected("document")
	if err != nil {
		return api.Document{}, false, err
	}
	for _, version := range versions {
		if version.Commit.State != "live" {
			continue
		}
		raw, err := r.core.ReadJSON(ctx, version)
		if err != nil {
			return api.Document{}, false, err
		}
		var d storedDocument
		err = json.Unmarshal(raw, &d)
		clear(raw)
		if err != nil {
			return api.Document{}, false, err
		}
		if d.DeletedAt == nil && d.SourceRef != nil && *d.SourceRef == fuente {
			return d.toAPIDocument(), true, nil
		}
	}
	return api.Document{}, false, nil
}

// GetDocument returns a live document and the caller's role ("owner").
func (r *Repository) GetDocument(ctx context.Context, id string) (api.Document, string, error) {
	doc, _, _, err := r.readDocumentRow(ctx, id)
	if err != nil {
		return api.Document{}, "", err
	}
	return doc.toAPIDocument(), "owner", nil
}

// ImportDocument converts an external office file into the internal model.
func (r *Repository) ImportDocument(_ context.Context, nombre string, datos []byte, comentarios bool) (api.Importado, error) {
	var (
		res *model.ImportResult
		err error
	)
	if comentarios {
		res, err = model.ImportWithComments(nombre, datos)
	} else {
		res, err = model.Import(nombre, datos)
	}
	if err != nil {
		return api.Importado{}, err
	}
	out := api.Importado{
		Kind:      api.DocKind(res.Kind),
		Title:     res.Title,
		Content:   res.Content,
		Template:  res.Template,
		Truncated: res.Truncated,
	}
	if len(res.Comments) > 0 {
		out.Comments = make([]api.ImportadoComentario, len(res.Comments))
		for i, c := range res.Comments {
			out.Comments[i] = api.ImportadoComentario{
				ID:       c.ID,
				ParentID: c.ParentID,
				Text:     c.Text,
				Quote:    c.Quote,
				Anchored: c.Anchored,
				Resolved: c.Resolved,
			}
			if c.Cell != nil {
				out.Comments[i].Cell = &api.ImportadoCelda{Hoja: c.Cell.Hoja, Fila: c.Cell.Fila, Col: c.Cell.Col, Text: c.Cell.Text}
			}
		}
	}
	return out, nil
}

// ImportComments stores imported comments on document id.
func (r *Repository) ImportComments(ctx context.Context, id string, comentarios []api.ImportadoComentario) ([]string, error) {
	if _, _, _, err := r.readDocumentRow(ctx, id); err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	idMap := make(map[string]string, len(comentarios))
	var anchored []string
	for _, c := range comentarios {
		if c.ParentID != "" {
			continue
		}
		cid, wasAnchored, err := r.storeImportedRootComment(ctx, id, c, now)
		if err != nil {
			return nil, err
		}
		if cid == "" {
			continue
		}
		idMap[c.ID] = cid
		if wasAnchored {
			anchored = append(anchored, c.ID)
		}
	}
	for _, c := range comentarios {
		if c.ParentID == "" {
			continue
		}
		parentID, ok := idMap[c.ParentID]
		if !ok {
			continue
		}
		if err := r.storeImportedReplyComment(ctx, id, parentID, c, now); err != nil {
			return nil, err
		}
	}
	return anchored, nil
}

// maxCuerpoImportado y maxCitaImportada son los recortes del servicio docs al importar
// comentarios de un fichero (`guardarImportado`).
const (
	maxCuerpoImportado = 20000
	maxCitaImportada   = 1000
)

// recortarRunas deja s en n runas como mucho.
func recortarRunas(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n])
}

func (r *Repository) storeImportedRootComment(ctx context.Context, docID string, c api.ImportadoComentario, now time.Time) (string, bool, error) {
	body := strings.TrimSpace(recortarRunas(c.Text, maxCuerpoImportado))
	// Las mismas reglas que `guardarImportado` del servicio docs: un ancla de marca necesita un
	// id de marca válido, y la de una celda que no lo es se queda en el texto citado.
	if body == "" || (c.Anchored && !ancla.EsMarca(c.ID)) {
		return "", false, nil
	}
	cid := uuid.NewString()
	if parsed, err := uuid.Parse(c.ID); err == nil && parsed != uuid.Nil {
		cid = parsed.String()
	}
	var anchor json.RawMessage
	var datosAncla map[string]string
	switch {
	case c.Anchored:
		datosAncla = map[string]string{"mark": c.ID, "text": recortarRunas(c.Quote, maxCitaImportada)}
	case c.Cell != nil:
		// El comentario de una celda de un .xlsx/.ods, anclado a ella.
		datosAncla = ancla.DeCelda(c.Cell.Hoja, c.Cell.Fila, c.Cell.Col, c.Cell.Text)
	}
	if datosAncla != nil {
		rawAnchor, err := json.Marshal(datosAncla)
		if err != nil {
			return "", false, err
		}
		if ancla.Validar(rawAnchor) == "" {
			anchor = rawAnchor
		}
	}
	row := storedComment{
		ID:         cid,
		DocumentID: docID,
		Tenant:     r.scope.Tenant,
		AuthorSub:  r.scope.Subject,
		Anchor:     anchor,
		Body:       body,
		Resolved:   c.Resolved,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	if err := r.putCommentRow(ctx, "comment-import-root", row); err != nil {
		return "", false, err
	}
	return cid, c.Anchored, nil
}

func (r *Repository) storeImportedReplyComment(ctx context.Context, docID, parentID string, c api.ImportadoComentario, now time.Time) error {
	body := strings.TrimSpace(recortarRunas(c.Text, maxCuerpoImportado))
	if body == "" {
		return nil
	}
	row := storedComment{
		ID:         uuid.NewString(),
		DocumentID: docID,
		Tenant:     r.scope.Tenant,
		ParentID:   &parentID,
		AuthorSub:  r.scope.Subject,
		Body:       body,
		Resolved:   c.Resolved,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	return r.putCommentRow(ctx, "comment-import-reply", row)
}

func (r *Repository) putCommentRow(ctx context.Context, op string, row storedComment) error {
	objID, err := idSourceObjectID("comment", row.ID)
	if err != nil {
		return err
	}
	state := "live"
	if row.DeletedAt != nil {
		state = "trash"
	}
	raw, err := r.core.Mutate(ctx, op, row, func(_ context.Context) (string, string, string, []byte, error) {
		encoded, err := json.Marshal(row)
		return "comment", objID, state, encoded, err
	})
	clear(raw)
	return err
}

// RenameDocument updates the title of document id.
func (r *Repository) RenameDocument(ctx context.Context, id, titulo string) error {
	raw, err := r.core.Mutate(ctx, "document-rename", []string{id, titulo}, func(ctx context.Context) (string, string, string, []byte, error) {
		_, rawMap, objID, err := r.readDocumentRow(ctx, id)
		if err != nil {
			return "", "", "", nil, err
		}
		rawMap["title"] = titulo
		rawMap["updated_at"] = time.Now().UTC()
		encoded, err := json.Marshal(rawMap)
		return "document", objID, "live", encoded, err
	})
	clear(raw)
	return err
}

// DeleteDocument moves document id to the trash.
func (r *Repository) DeleteDocument(ctx context.Context, id string) error {
	raw, err := r.core.Mutate(ctx, "document-delete", id, func(ctx context.Context) (string, string, string, []byte, error) {
		_, rawMap, objID, err := r.readDocumentRow(ctx, id)
		if err != nil {
			return "", "", "", nil, err
		}
		now := time.Now().UTC()
		rawMap["deleted_at"] = now
		rawMap["updated_at"] = now
		encoded, err := json.Marshal(rawMap)
		return "document", objID, "trash", encoded, err
	})
	clear(raw)
	return err
}

// StarDocument updates the starred flag on document id.
func (r *Repository) StarDocument(ctx context.Context, id string, destacado bool) error {
	raw, err := r.core.Mutate(ctx, "document-star", struct {
		ID      string
		Starred bool
	}{id, destacado}, func(ctx context.Context) (string, string, string, []byte, error) {
		_, rawMap, objID, err := r.readDocumentRow(ctx, id)
		if err != nil {
			return "", "", "", nil, err
		}
		rawMap["starred"] = destacado
		encoded, err := json.Marshal(rawMap)
		return "document", objID, "live", encoded, err
	})
	clear(raw)
	return err
}

// DocumentContent materializes the current CRDT state for clave on document id.
func (r *Repository) DocumentContent(ctx context.Context, id, clave string) (api.DocContent, error) {
	doc, _, _, err := r.readDocumentRow(ctx, id)
	if err != nil {
		return api.DocContent{}, err
	}
	state, err := r.loadDocumentState(ctx, doc)
	if err != nil {
		return api.DocContent{}, err
	}
	var elementos []json.RawMessage
	if len(state.Elements) > 0 {
		elementos = make([]json.RawMessage, 0, len(state.Elements))
		for _, el := range state.Elements {
			rawEl, err := json.Marshal(el)
			if err != nil {
				return api.DocContent{}, err
			}
			elementos = append(elementos, rawEl)
		}
	}
	out := api.DocContent{
		MaxLamport: state.Lamport,
		Elementos:  elementos,
		Seq:        doc.Seq,
	}
	if reg, ok := state.Registers[clave]; ok {
		out.Dot = api.DocDot{Actor: reg.Dot.Actor, Counter: reg.Dot.Counter}
		out.Lamport = reg.Lamport
		_ = json.Unmarshal(reg.Value, &out.Value)
	}
	return out, nil
}

func (r *Repository) applyAndPersistOps(ctx context.Context, id string, ops []model.Op, actor string, baseSeq int64, checkKey string) (int64, error) {
	doc, rawMap, docObjID, err := r.readDocumentRow(ctx, id)
	if err != nil {
		return 0, err
	}
	if baseSeq > 0 && doc.Seq > baseSeq && checkKey != "" {
		return 0, &api.Error{Status: http.StatusConflict, URL: "/documents/" + id + "/ops"}
	}
	state, err := r.loadDocumentState(ctx, doc)
	if err != nil {
		return 0, err
	}
	if actor == "" {
		actor = r.scope.Subject
	}
	seq := doc.Seq
	now := time.Now().UTC()
	for _, op := range ops {
		var err error
		seq, err = r.appendSingleOp(ctx, id, state, op, actor, seq, now)
		if err != nil {
			return 0, err
		}
	}
	snapID, err := r.writeInlineSnapshot(ctx, id, seq, state)
	if err != nil {
		return 0, err
	}
	rawMap["seq"] = seq
	rawMap["current_snapshot_id"] = snapID
	rawMap["updated_at"] = now
	raw, err := r.core.Mutate(ctx, "document-seq", struct {
		ID     string
		Seq    int64
		SnapID string
	}{id, seq, snapID}, func(_ context.Context) (string, string, string, []byte, error) {
		encoded, err := json.Marshal(rawMap)
		return "document", docObjID, "live", encoded, err
	})
	clear(raw)
	return seq, err
}

func (r *Repository) appendSingleOp(ctx context.Context, docID string, state *model.State, op model.Op, actor string, seq int64, now time.Time) (int64, error) {
	if op.Dot.Actor == "" {
		op.Dot.Actor = actor
	}
	if op.Lamport <= state.Lamport {
		op.Lamport = state.Lamport + 1
	}
	if op.Dot.Counter == 0 {
		op.Dot.Counter = op.Lamport
	}
	if _, err := state.Apply(op); err != nil {
		return 0, err
	}
	seq++
	state.Seq = seq
	rawOp, err := json.Marshal(op)
	if err != nil {
		return 0, err
	}
	opObjID, err := opSourceObjectID(docID, seq)
	if err != nil {
		return 0, err
	}
	opRow := storedOperation{
		DocumentID: docID,
		Seq:        seq,
		Tenant:     r.scope.Tenant,
		Actor:      op.Dot.Actor,
		AuthorSub:  r.scope.Subject,
		Lamport:    int64(op.Lamport), //nolint:gosec // G115: Lamport clock is bounded well below MaxInt64
		Op:         rawOp,
		CreatedAt:  now,
	}
	raw, err := r.core.Mutate(ctx, "operation-append", opRow, func(_ context.Context) (string, string, string, []byte, error) {
		encoded, err := json.Marshal(opRow)
		return "operation", opObjID, "live", encoded, err
	})
	clear(raw)
	return seq, err
}

// SaveDocumentContent writes an LWW register update for clave on document id.
func (r *Repository) SaveDocumentContent(ctx context.Context, id, clave, valor string, dot api.DocDot, lamport uint64, autoritativo bool, baseSeq int64) (int64, error) {
	rawValue, err := json.Marshal(valor)
	if err != nil {
		return 0, err
	}
	if dot.Actor == "" {
		dot.Actor = r.scope.Subject
	}
	var ops []model.Op
	if autoritativo {
		rawLamport, err := json.Marshal(lamport)
		if err != nil {
			return 0, err
		}
		ops = append(ops, model.Op{
			Kind:    model.OpSet,
			Dot:     model.Dot{Actor: dot.Actor, Counter: dot.Counter},
			Key:     coedicion.ClaveLWW(clave),
			Value:   rawLamport,
			Lamport: lamport,
		})
	}
	ops = append(ops, model.Op{
		Kind:    model.OpSet,
		Dot:     model.Dot{Actor: dot.Actor, Counter: dot.Counter},
		Key:     clave,
		Value:   rawValue,
		Lamport: lamport,
	})
	return r.applyAndPersistOps(ctx, id, ops, dot.Actor, baseSeq, clave)
}

// SaveDocumentCoedition writes RGA character operations and the companion register.
func (r *Repository) SaveDocumentCoedition(ctx context.Context, id, clave, valor string, dot api.DocDot, lamport uint64, autoritativo bool, chars []coedicion.Op) (int64, error) {
	if dot.Actor == "" {
		dot.Actor = r.scope.Subject
	}
	ops := make([]model.Op, 0, len(chars)+2)
	for _, c := range chars {
		rawC, err := json.Marshal(c)
		if err != nil {
			return 0, err
		}
		var op model.Op
		if err := json.Unmarshal(rawC, &op); err != nil {
			return 0, err
		}
		if op.Lamport == 0 {
			op.Lamport = max(lamport, 1)
		}
		ops = append(ops, op)
	}
	if autoritativo {
		rawLamport, err := json.Marshal(lamport)
		if err != nil {
			return 0, err
		}
		ops = append(ops, model.Op{
			Kind:    model.OpSet,
			Dot:     model.Dot{Actor: dot.Actor, Counter: dot.Counter},
			Key:     coedicion.ClaveLWW(clave),
			Value:   rawLamport,
			Lamport: lamport,
		})
	}
	rawValue, err := json.Marshal(valor)
	if err != nil {
		return 0, err
	}
	ops = append(ops, model.Op{
		Kind:    model.OpSet,
		Dot:     model.Dot{Actor: dot.Actor, Counter: dot.Counter},
		Key:     clave,
		Value:   rawValue,
		Lamport: lamport,
	})
	return r.applyAndPersistOps(ctx, id, ops, dot.Actor, 0, "")
}

// DocumentOpsSince returns sequenced operations after desde.
func (r *Repository) DocumentOpsSince(ctx context.Context, id string, desde int64) ([]api.DocOpSecuenciada, error) {
	if _, _, _, err := r.readDocumentRow(ctx, id); err != nil {
		return nil, err
	}
	opRows, err := r.readOperationsForDocument(ctx, id, desde, 0)
	if err != nil {
		return nil, err
	}
	ops := make([]api.DocOpSecuenciada, len(opRows))
	for i, opRow := range opRows {
		ops[i] = api.DocOpSecuenciada{
			Seq:   opRow.Seq,
			Actor: opRow.Actor,
			Op:    opRow.Op,
		}
	}
	return ops, nil
}

// DocumentPresence returns the local owner as the sole active member in private mode.
func (r *Repository) DocumentPresence(ctx context.Context, id, conn string) ([]api.DocMiembro, error) {
	if _, _, _, err := r.readDocumentRow(ctx, id); err != nil {
		return nil, err
	}
	return []api.DocMiembro{{Conn: conn, Sub: r.scope.Subject}}, nil
}

// SaveDocumentChecked saves content only if base matches the current head.
func (r *Repository) SaveDocumentChecked(ctx context.Context, id, clave, valor string, base api.DocContent, dot api.DocDot, autoritativo bool) (api.DocContent, error) {
	ahora, err := r.DocumentContent(ctx, id, clave)
	if err != nil {
		return base, err
	}
	if ahora.Dot != base.Dot || ahora.Lamport != base.Lamport {
		return base, api.ErrDocConflicto
	}
	lamport := max(ahora.MaxLamport, base.MaxLamport) + 1
	if dot.Actor == "" {
		dot.Actor = r.scope.Subject
	}
	if dot.Counter == 0 {
		dot.Counter = lamport
	}
	seq, err := r.SaveDocumentContent(ctx, id, clave, valor, dot, lamport, autoritativo, base.Seq)
	var httpErr *api.Error
	if errors.As(err, &httpErr) && httpErr.Status == http.StatusConflict {
		return base, api.ErrDocConflicto
	}
	if err != nil {
		return base, err
	}
	return api.DocContent{Value: valor, Dot: dot, Lamport: lamport, MaxLamport: lamport, Seq: seq}, nil
}

// DocumentVersions lists saved versions for document id.
func (r *Repository) DocumentVersions(ctx context.Context, id string) ([]api.DocVersion, error) {
	if _, _, _, err := r.readDocumentRow(ctx, id); err != nil {
		return nil, err
	}
	entries, err := r.core.Selected("version")
	if err != nil {
		return nil, err
	}
	var versions []api.DocVersion
	for _, entry := range entries {
		if entry.Commit.State != "live" {
			continue
		}
		raw, err := r.core.ReadJSON(ctx, entry)
		if err != nil {
			return nil, err
		}
		var v storedVersion
		err = json.Unmarshal(raw, &v)
		clear(raw)
		if err != nil {
			return nil, err
		}
		if v.DocumentID != id {
			continue
		}
		versions = append(versions, api.DocVersion{
			ID:        v.ID,
			Label:     v.Label,
			AuthorSub: v.AuthorSub,
			Seq:       v.Seq,
			Auto:      v.Auto,
			CreatedAt: v.CreatedAt,
		})
	}
	slices.SortFunc(versions, func(a, b api.DocVersion) int {
		if c := b.CreatedAt.Compare(a.CreatedAt); c != 0 {
			return c
		}
		return cmp.Compare(b.Seq, a.Seq)
	})
	return versions, nil
}

// CreateDocumentVersion snapshots and labels the current state of document id.
func (r *Repository) CreateDocumentVersion(ctx context.Context, id, label string) (api.DocVersion, error) {
	doc, _, _, err := r.readDocumentRow(ctx, id)
	if err != nil {
		return api.DocVersion{}, err
	}
	state, err := r.loadDocumentState(ctx, doc)
	if err != nil {
		return api.DocVersion{}, err
	}
	snapID, err := r.writeInlineSnapshot(ctx, id, doc.Seq, state)
	if err != nil {
		return api.DocVersion{}, err
	}
	now := time.Now().UTC()
	verID := uuid.NewString()
	verObjID, err := idSourceObjectID("version", verID)
	if err != nil {
		return api.DocVersion{}, err
	}
	v := storedVersion{
		ID:         verID,
		DocumentID: id,
		Tenant:     r.scope.Tenant,
		SnapshotID: snapID,
		Label:      strings.TrimSpace(label),
		AuthorSub:  r.scope.Subject,
		Seq:        doc.Seq,
		Auto:       false,
		CreatedAt:  now,
	}
	raw, err := r.core.Mutate(ctx, "version-create", v, func(_ context.Context) (string, string, string, []byte, error) {
		encoded, err := json.Marshal(v)
		return "version", verObjID, "live", encoded, err
	})
	clear(raw)
	if err != nil {
		return api.DocVersion{}, err
	}
	return api.DocVersion{
		ID:        v.ID,
		Label:     v.Label,
		AuthorSub: v.AuthorSub,
		Seq:       v.Seq,
		Auto:      v.Auto,
		CreatedAt: v.CreatedAt,
	}, nil
}

// RestoreDocumentVersion restores the content register from versionID on document id.
func (r *Repository) RestoreDocumentVersion(ctx context.Context, id, versionID string) error {
	doc, _, _, err := r.readDocumentRow(ctx, id)
	if err != nil {
		return err
	}
	verObjID, err := idSourceObjectID("version", versionID)
	if err != nil {
		return notFoundError(id)
	}
	rawVer, err := r.core.ReadRaw(ctx, "version", verObjID)
	if errors.Is(err, records.ErrNotFound) {
		return notFoundError(id)
	}
	if err != nil {
		return err
	}
	var ver storedVersion
	err = json.Unmarshal(rawVer, &ver)
	clear(rawVer)
	if err != nil {
		return err
	}
	if ver.DocumentID != id {
		return notFoundError(id)
	}
	snapState, err := r.loadSnapshotState(ctx, ver.SnapshotID)
	if err != nil {
		return err
	}
	curState, err := r.loadDocumentState(ctx, doc)
	if err != nil {
		return err
	}
	key := api.DocContentKey(api.DocKind(doc.Kind))
	var value string
	if reg, ok := snapState.Registers[key]; ok {
		_ = json.Unmarshal(reg.Value, &value)
	}
	lam := curState.Lamport + 1
	_, err = r.SaveDocumentContent(ctx, id, key, value, api.DocDot{Actor: r.scope.Subject, Counter: lam}, lam, doc.Kind == string(api.DocText), 0)
	return err
}

// ExportDocument exports document id to formato.
func (r *Repository) ExportDocument(ctx context.Context, id, formato string, plantilla bool, accesible *bool) ([]byte, error) {
	doc, _, _, err := r.readDocumentRow(ctx, id)
	if err != nil {
		return nil, err
	}
	state, err := r.loadDocumentState(ctx, doc)
	if err != nil {
		return nil, err
	}
	kind := model.Kind(doc.Kind)
	format := model.BlobFormat(formato)
	if format == model.FormatPDF && !plantilla && (accesible == nil || *accesible) {
		data, _, err := model.ExportPDFAccesible(kind, doc.Title, cmp.Or(r.Idioma, "es"), state)
		return data, err
	}
	comments, err := r.loadModelComments(ctx, id)
	if err != nil {
		return nil, err
	}
	data, _, _, err := model.ExportWithComments(kind, doc.Title, state, format, comments)
	if err != nil {
		return nil, err
	}
	if plantilla {
		data, _, _, err = model.ComoPlantilla(kind, format, data)
		if err != nil {
			return nil, err
		}
	}
	return data, nil
}

func (r *Repository) loadModelComments(ctx context.Context, docID string) ([]model.Comment, error) {
	rows, err := r.readCommentsForDocument(ctx, docID, true)
	if err != nil {
		return nil, err
	}
	comments := make([]model.Comment, len(rows))
	for i, c := range rows {
		comments[i] = model.Comment{
			ID:         c.ID,
			DocumentID: c.DocumentID,
			ParentID:   c.ParentID,
			AuthorSub:  c.AuthorSub,
			Anchor:     c.Anchor,
			Body:       c.Body,
			Resolved:   c.Resolved,
			CreatedAt:  c.CreatedAt,
			UpdatedAt:  c.UpdatedAt,
		}
	}
	return comments, nil
}

// DocumentSharing returns retained ACL permissions and active share links for document id.
func (r *Repository) DocumentSharing(ctx context.Context, id string) ([]api.DocPermission, []api.DocShare, error) {
	if _, _, _, err := r.readDocumentRow(ctx, id); err != nil {
		return nil, nil, err
	}
	perms, err := r.readDocumentPermissions(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	shares, err := r.readDocumentShareLinks(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	return perms, shares, nil
}

func (r *Repository) readDocumentPermissions(ctx context.Context, docID string) ([]api.DocPermission, error) {
	entries, err := r.core.Selected("permission")
	if err != nil {
		return nil, err
	}
	var perms []api.DocPermission
	for _, entry := range entries {
		if entry.Commit.State != "live" {
			continue
		}
		raw, err := r.core.ReadJSON(ctx, entry)
		if err != nil {
			return nil, err
		}
		var p storedPermission
		err = json.Unmarshal(raw, &p)
		clear(raw)
		if err != nil {
			return nil, err
		}
		if p.DocumentID == docID && p.DeletedAt == nil {
			perms = append(perms, api.DocPermission{
				PrincipalKind: p.PrincipalKind,
				PrincipalID:   p.PrincipalID,
				Role:          p.Role,
			})
		}
	}
	return perms, nil
}

func (r *Repository) readDocumentShareLinks(ctx context.Context, docID string) ([]api.DocShare, error) {
	entries, err := r.core.Selected("share_link")
	if err != nil {
		return nil, err
	}
	var shares []api.DocShare
	for _, entry := range entries {
		if entry.Commit.State != "live" {
			continue
		}
		raw, err := r.core.ReadJSON(ctx, entry)
		if err != nil {
			return nil, err
		}
		var s storedShareLink
		err = json.Unmarshal(raw, &s)
		clear(raw)
		if err != nil {
			return nil, err
		}
		if s.DocumentID == docID && s.RevokedAt == nil && s.DeletedAt == nil {
			shares = append(shares, api.DocShare{
				ID:        s.ID,
				Role:      s.Role,
				ExpiresAt: s.ExpiresAt,
			})
		}
	}
	return shares, nil
}

// GrantDocumentPermission records a permission entry on document id.
func (r *Repository) GrantDocumentPermission(ctx context.Context, id, kind, principal, role string) error {
	if _, _, _, err := r.readDocumentRow(ctx, id); err != nil {
		return err
	}
	objID, err := permSourceObjectID(id, kind, principal)
	if err != nil {
		return err
	}
	p := storedPermission{
		DocumentID:    id,
		Tenant:        r.scope.Tenant,
		PrincipalKind: kind,
		PrincipalID:   principal,
		Role:          role,
		GrantedBy:     r.scope.Subject,
	}
	raw, err := r.core.Mutate(ctx, "permission-grant", p, func(_ context.Context) (string, string, string, []byte, error) {
		encoded, err := json.Marshal(p)
		return "permission", objID, "live", encoded, err
	})
	clear(raw)
	return err
}

// RevokeDocumentPermission marks a permission entry as deleted on document id.
func (r *Repository) RevokeDocumentPermission(ctx context.Context, id, kind, principal string) error {
	if _, _, _, err := r.readDocumentRow(ctx, id); err != nil {
		return err
	}
	objID, err := permSourceObjectID(id, kind, principal)
	if err != nil {
		return err
	}
	raw, err := r.core.Mutate(ctx, "permission-revoke", []string{id, kind, principal}, func(ctx context.Context) (string, string, string, []byte, error) {
		existing, err := r.core.ReadRaw(ctx, "permission", objID)
		if err != nil {
			return "", "", "", nil, err
		}
		defer clear(existing)
		var rawMap map[string]any
		if err := json.Unmarshal(existing, &rawMap); err != nil {
			return "", "", "", nil, err
		}
		rawMap["deleted_at"] = time.Now().UTC()
		encoded, err := json.Marshal(rawMap)
		return "permission", objID, "trash", encoded, err
	})
	clear(raw)
	return err
}

// CreateDocumentShare is unsupported in isolated private mode.
func (r *Repository) CreateDocumentShare(_ context.Context, _, _ string) (api.DocShare, error) {
	return api.DocShare{}, api.ErrPrivateDataOnly
}

// RevokeDocumentShare revokes a retained share link on document id.
func (r *Repository) RevokeDocumentShare(ctx context.Context, id, enlace string) error {
	if _, _, _, err := r.readDocumentRow(ctx, id); err != nil {
		return err
	}
	objID, err := idSourceObjectID("share_link", enlace)
	if err != nil {
		return err
	}
	raw, err := r.core.Mutate(ctx, "share-revoke", []string{id, enlace}, func(ctx context.Context) (string, string, string, []byte, error) {
		existing, err := r.core.ReadRaw(ctx, "share_link", objID)
		if err != nil {
			return "", "", "", nil, err
		}
		defer clear(existing)
		var rawMap map[string]any
		if err := json.Unmarshal(existing, &rawMap); err != nil {
			return "", "", "", nil, err
		}
		now := time.Now().UTC()
		rawMap["revoked_at"] = now
		rawMap["deleted_at"] = now
		encoded, err := json.Marshal(rawMap)
		return "share_link", objID, "trash", encoded, err
	})
	clear(raw)
	return err
}

func (r *Repository) readCommentsForDocument(ctx context.Context, docID string, conResueltos bool) ([]storedComment, error) {
	entries, err := r.core.Selected("comment")
	if err != nil {
		return nil, err
	}
	var out []storedComment
	for _, entry := range entries {
		if entry.Commit.State != "live" {
			continue
		}
		raw, err := r.core.ReadJSON(ctx, entry)
		if err != nil {
			return nil, err
		}
		var c storedComment
		err = json.Unmarshal(raw, &c)
		clear(raw)
		if err != nil {
			return nil, err
		}
		if c.DocumentID != docID || c.DeletedAt != nil || (!conResueltos && c.Resolved) {
			continue
		}
		out = append(out, c)
	}
	return out, nil
}

// DocumentComments lists comments on document id.
func (r *Repository) DocumentComments(ctx context.Context, id string, conResueltos bool) ([]api.DocComment, error) {
	if _, _, _, err := r.readDocumentRow(ctx, id); err != nil {
		return nil, err
	}
	rows, err := r.readCommentsForDocument(ctx, id, conResueltos)
	if err != nil {
		return nil, err
	}
	comments := make([]api.DocComment, len(rows))
	for i, c := range rows {
		var parentID string
		if c.ParentID != nil {
			parentID = *c.ParentID
		}
		comments[i] = api.DocComment{
			ID:        c.ID,
			ParentID:  parentID,
			AuthorSub: c.AuthorSub,
			Body:      c.Body,
			Resolved:  c.Resolved,
			CreatedAt: c.CreatedAt,
			UpdatedAt: c.UpdatedAt,
			Anchor:    c.Anchor,
		}
	}
	slices.SortFunc(comments, func(a, b api.DocComment) int {
		if c := a.CreatedAt.Compare(b.CreatedAt); c != 0 {
			return c
		}
		return cmp.Compare(a.ID, b.ID)
	})
	return comments, nil
}

// CreateDocumentComment adds a comment or reply to document id.
func (r *Repository) CreateDocumentComment(ctx context.Context, id, padre, cuerpo string) (api.DocComment, error) {
	return r.createComment(ctx, id, padre, cuerpo, nil)
}

// CreateAnchoredDocumentComment adds an anchored comment to document id.
func (r *Repository) CreateAnchoredDocumentComment(ctx context.Context, id, cuerpo, marca, texto string) (api.DocComment, error) {
	anchor, err := json.Marshal(map[string]string{"mark": marca, "text": texto})
	if err != nil {
		return api.DocComment{}, err
	}
	return r.createComment(ctx, id, "", cuerpo, anchor)
}

// CreateCellDocumentComment adds a comment anchored to a spreadsheet cell to document id.
func (r *Repository) CreateCellDocumentComment(ctx context.Context, id, cuerpo string, a api.AnclaCelda) (api.DocComment, error) {
	anchor, err := json.Marshal(a)
	if err != nil {
		return api.DocComment{}, err
	}
	return r.createComment(ctx, id, "", cuerpo, anchor)
}

func (r *Repository) createComment(ctx context.Context, id, padre, cuerpo string, anchor json.RawMessage) (api.DocComment, error) {
	cuerpo = strings.TrimSpace(cuerpo)
	if cuerpo == "" {
		return api.DocComment{}, api.ErrComentarioVacio
	}
	// La misma regla que el servicio docs: un ancla que él rechazaría tampoco se guarda en
	// local, o el documento no podría compartirse ni sincronizarse tal cual.
	if motivo := ancla.Validar(anchor); motivo != "" {
		return api.DocComment{}, fmt.Errorf("%w: %s", api.ErrAnclaNoValida, motivo)
	}
	if _, _, _, err := r.readDocumentRow(ctx, id); err != nil {
		return api.DocComment{}, err
	}
	var parentPtr *string
	if padre = strings.TrimSpace(padre); padre != "" {
		parentPtr = &padre
	}
	now := time.Now().UTC()
	cid := uuid.NewString()
	row := storedComment{
		ID:         cid,
		DocumentID: id,
		Tenant:     r.scope.Tenant,
		ParentID:   parentPtr,
		AuthorSub:  r.scope.Subject,
		Anchor:     anchor,
		Body:       cuerpo,
		Resolved:   false,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	if err := r.putCommentRow(ctx, "comment-create", row); err != nil {
		return api.DocComment{}, err
	}
	return api.DocComment{
		ID:        cid,
		ParentID:  padre,
		AuthorSub: r.scope.Subject,
		Body:      cuerpo,
		Resolved:  false,
		CreatedAt: now,
		UpdatedAt: now,
		Anchor:    anchor,
	}, nil
}

// ResolveDocumentComment marks comment comentario on document id as resolved or unresolved.
func (r *Repository) ResolveDocumentComment(ctx context.Context, id, comentario string, resuelto bool) error {
	if _, _, _, err := r.readDocumentRow(ctx, id); err != nil {
		return err
	}
	objID, err := idSourceObjectID("comment", comentario)
	if err != nil {
		return notFoundError(id)
	}
	raw, err := r.core.Mutate(ctx, "comment-resolve", struct {
		DocID    string
		Comment  string
		Resolved bool
	}{id, comentario, resuelto}, func(ctx context.Context) (string, string, string, []byte, error) {
		existing, err := r.core.ReadRaw(ctx, "comment", objID)
		if errors.Is(err, records.ErrNotFound) {
			return "", "", "", nil, notFoundError(id)
		}
		if err != nil {
			return "", "", "", nil, err
		}
		defer clear(existing)
		var c storedComment
		if err := json.Unmarshal(existing, &c); err != nil {
			return "", "", "", nil, err
		}
		if c.DocumentID != id || c.DeletedAt != nil {
			return "", "", "", nil, notFoundError(id)
		}
		var rawMap map[string]any
		if err := json.Unmarshal(existing, &rawMap); err != nil {
			return "", "", "", nil, err
		}
		rawMap["resolved"] = resuelto
		rawMap["updated_at"] = time.Now().UTC()
		encoded, err := json.Marshal(rawMap)
		return "comment", objID, "live", encoded, err
	})
	clear(raw)
	return err
}

// DeleteDocumentComment deletes comment comentario and its direct replies on document id.
func (r *Repository) DeleteDocumentComment(ctx context.Context, id, comentario string) error {
	if _, _, _, err := r.readDocumentRow(ctx, id); err != nil {
		return err
	}
	rows, err := r.readCommentsForDocument(ctx, id, true)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	for _, c := range rows {
		if c.ID == comentario || (c.ParentID != nil && *c.ParentID == comentario) {
			c.DeletedAt = &now
			c.UpdatedAt = now
			if err := r.putCommentRow(ctx, "comment-delete", c); err != nil {
				return err
			}
		}
	}
	return nil
}

func toSlidesDocument(d api.Document) api.SlidesDocument {
	return api.SlidesDocument{
		ID:        d.ID,
		Kind:      string(d.Kind),
		Title:     d.Title,
		OwnerSub:  d.OwnerSub,
		Seq:       d.Seq,
		Starred:   d.Starred,
		CreatedAt: d.CreatedAt,
		UpdatedAt: d.UpdatedAt,
		SourceRef: d.SourceRef,
	}
}

// ListSlides lists slide presentations matching q.
func (r *Repository) ListSlides(ctx context.Context, q, cursor string) ([]api.SlidesDocument, string, error) {
	docs, next, err := r.ListDocuments(ctx, api.DocsFilter{
		Kind:   api.DocSlides,
		Query:  q,
		Cursor: cursor,
		Limit:  api.SlidesPagina,
	})
	if err != nil {
		return nil, "", err
	}
	out := make([]api.SlidesDocument, len(docs))
	for i, d := range docs {
		out[i] = toSlidesDocument(d)
	}
	return out, next, nil
}

// CreateSlidePresentation creates a new slide presentation with titulo.
func (r *Repository) CreateSlidePresentation(ctx context.Context, titulo string) (api.SlidesDocument, error) {
	doc, _, err := r.CreateDocumentFromSource(ctx, api.DocSlides, titulo, "")
	if err != nil {
		return api.SlidesDocument{}, err
	}
	return toSlidesDocument(doc), nil
}

// GetSlidePresentation returns a slide presentation and the caller's role.
func (r *Repository) GetSlidePresentation(ctx context.Context, id string) (api.SlidesDocument, string, error) {
	doc, role, err := r.GetDocument(ctx, id)
	if err != nil {
		return api.SlidesDocument{}, "", err
	}
	return toSlidesDocument(doc), role, nil
}

// RenameSlidePresentation renames slide presentation id.
func (r *Repository) RenameSlidePresentation(ctx context.Context, id, titulo string) error {
	return r.RenameDocument(ctx, id, titulo)
}

// DeleteSlidePresentation moves slide presentation id to the trash.
func (r *Repository) DeleteSlidePresentation(ctx context.Context, id string) error {
	return r.DeleteDocument(ctx, id)
}

// SlideContent reads the current slide presentation model for id.
func (r *Repository) SlideContent(ctx context.Context, id string) (api.SlidesContent, error) {
	c, err := r.DocumentContent(ctx, id, api.SlidesContentKey)
	if err != nil {
		return api.SlidesContent{}, err
	}
	return api.SlidesContent{
		Modelo:  c.Value,
		Dot:     api.SlidesDot(c.Dot),
		Lamport: c.MaxLamport,
		Seq:     c.Seq,
	}, nil
}

// SaveSlideContent saves the slide presentation model for id if base matches.
func (r *Repository) SaveSlideContent(ctx context.Context, id string, base api.SlidesContent, actor string, contador uint64, modelo string) (api.SlidesContent, error) {
	if len(modelo) > 1<<20 {
		return api.SlidesContent{}, api.ErrSlidesTooBig
	}
	ahora, err := r.SlideContent(ctx, id)
	if err != nil {
		return api.SlidesContent{}, err
	}
	if ahora.Dot != base.Dot {
		return ahora, api.ErrSlidesConflict
	}
	lamport := max(ahora.Lamport, base.Lamport) + 1
	if actor == "" {
		actor = r.scope.Subject
	}
	if contador == 0 {
		contador = lamport
	}
	dot := api.DocDot{Actor: actor, Counter: contador}
	seq, err := r.SaveDocumentContent(ctx, id, api.SlidesContentKey, modelo, dot, lamport, false, base.Seq)
	var httpErr *api.Error
	if errors.As(err, &httpErr) && httpErr.Status == http.StatusConflict {
		if latest, readErr := r.SlideContent(ctx, id); readErr == nil {
			return latest, api.ErrSlidesConflict
		}
		return api.SlidesContent{}, api.ErrSlidesConflict
	}
	if err != nil {
		return api.SlidesContent{}, err
	}
	return api.SlidesContent{
		Modelo:  modelo,
		Dot:     api.SlidesDot(dot),
		Lamport: lamport,
		Seq:     seq,
	}, nil
}
