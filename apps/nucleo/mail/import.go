// SPDX-License-Identifier: Apache-2.0
package mail

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"strings"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/mail/limits"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

const (
	migrationKindV2 = "org.sytue.mail.migration.v2"
	importNamespace = "mail-private-source-import-v1"
)

// Source is a consistent retained HISTORICAL snapshot. It says nothing about
// later SMTP/IMAP deliveries, queue execution or verified retirement of originals.
// The original authenticated API client implements it; the private facade cannot.
type Source interface {
	PrivateStorageCapabilities(context.Context, storage.Scope) (storage.AppMigrationState, error)
	PrivateStorageState(context.Context, storage.Scope) (storage.LegacyMigrationState, error)
	PrivateStorageSourcePage(context.Context, storage.Scope, string, storage.LegacySourceCursor) (storage.StreamSourcePage, error)
	OpenPrivateStorageSource(context.Context, storage.Scope, string, storage.StreamSourceRow) (io.ReadCloser, error)
}

// ImportedSource authenticates every original's exact bytes and links its
// operational projection. Unimplemented domain rows remain available as private
// archived originals instead of disappearing during migration.
type ImportedSource struct {
	Source     storage.StreamSourceRow `json:"source"`
	Archive    storage.Reference       `json:"archive"`
	Projection *storage.Reference      `json:"projection,omitempty"`
}
type importIntent struct {
	Version int                          `json:"v"`
	State   storage.LegacyMigrationState `json:"state"`
	Target  storage.PublicationTarget    `json:"target"`
	Items   []importItem                 `json:"items"`
}
type importItem struct {
	Source     storage.StreamSourceRow `json:"source"`
	Archive    *storage.Reference      `json:"archive,omitempty"`
	Projection *storage.Reference      `json:"projection,omitempty"`
	Staged     *offline.StagedFile     `json:"staged,omitempty"`
	Revision   string                  `json:"revision,omitempty"`
	File       *storage.Reference      `json:"file,omitempty"`
}

func sameSource(a, b storage.LegacyMigrationState) bool {
	return a.SameRetainedSource(b)
}

func sourceArchive(row storage.StreamSourceRow) (string, string) {
	if row.Kind == "mime_message" {
		return "mime", row.ID
	}
	return "source", digest([]byte(row.Kind + "\x00" + row.ID))
}

func sourceKind(kind string) bool {
	switch kind {
	case "account", "alias", "mailbox", "message", "thread", "outbound", "label", "filter", "vacation", "vacation_log", "acl", "request", "smime_identity", "mime_message", "mime_outbound":
		return true
	default:
		return false
	}
}
func privateBody(row storage.StreamSourceRow) bool { return strings.HasPrefix(row.Kind, "mime_") }
func checkSourcePlan(state storage.LegacyMigrationState, items []importItem) error {
	var rows, bodies int64
	last := storage.LegacySourceCursor{}
	for _, item := range items {
		row := item.Source
		if !sourceKind(row.Kind) || row.Validate() != nil || last.Kind != "" && (row.Kind < last.Kind || row.Kind == last.Kind && row.ID <= last.ID) {
			return ErrRecord
		}
		if privateBody(row) {
			bodies++
		} else {
			rows++
		}
		last = storage.LegacySourceCursor{Kind: row.Kind, ID: row.ID}
	}
	if rows != state.Rows || bodies != state.Blobs || rows == 0 {
		return ErrNotReady
	}
	available := map[storage.LegacySourceCursor]bool{}
	for _, item := range items {
		available[storage.LegacySourceCursor{Kind: item.Source.Kind, ID: item.Source.ID}] = true
	}
	for _, item := range items {
		kind := item.Source.Kind
		switch kind {
		case "message", "outbound":
			kind = "mime_" + kind
		case "mime_message", "mime_outbound":
			kind = strings.TrimPrefix(kind, "mime_")
		default:
			continue
		}
		if !available[storage.LegacySourceCursor{Kind: kind, ID: item.Source.ID}] {
			return ErrNotReady
		}
	}
	return nil
}

func (r *Repository) verifyArchive(ctx context.Context, s storage.AtomicCatalogSnapshot, row storage.StreamSourceRow, ref storage.Reference) ([]byte, error) {
	entry, ok := s.Entries[ref]
	if !ok {
		return nil, ErrNotReady
	}
	version, found, err := storage.AcceptedAtomicObject(s, r.cfg.Scope, entry.Record.ObjectID, "", &ref)
	if err != nil || !found {
		return nil, ErrRecord
	}
	typ, id := sourceArchive(row)
	h, err := decodeHeader(version)
	if err != nil || h.Type != typ || h.ID != id {
		return nil, ErrRecord
	}
	if privateBody(row) {
		return nil, r.verifyArchiveStream(ctx, version, row)
	}
	raw, err := r.readDocument(ctx, version)
	if err != nil {
		return nil, err
	}
	if int64(len(raw)) != row.Size || digest(raw) != row.SHA256 {
		clear(raw)
		return nil, ErrRecord
	}
	return raw, nil
}

type sourceHashWriter struct {
	hash    io.Writer
	size, n int64
}

func (w *sourceHashWriter) Write(p []byte) (int, error) {
	if int64(len(p)) > w.size-w.n {
		return 0, ErrRecord
	}
	n, err := w.hash.Write(p)
	w.n += int64(n)
	return n, err
}

func (r *Repository) verifyArchiveStream(ctx context.Context, version storage.ObjectVersion, row storage.StreamSourceRow) error {
	pointer := version.Commit.Content
	if version.Commit.State != "live" || pointer == nil || pointer.Kind != "file" {
		return ErrRecord
	}
	info, err := r.cfg.Files.Info(ctx, pointer.Reference, version.ObjectID, pointer.RevisionID)
	if err != nil {
		return err
	}
	if info.Validate(r.cfg.Scope) != nil || info.ObjectID != version.ObjectID || info.RevisionID != pointer.RevisionID || info.SizeBytes != row.Size {
		return ErrRecord
	}
	hash := sha256.New()
	writer := &sourceHashWriter{hash: hash, size: row.Size}
	actual, err := r.cfg.Files.Download(ctx, pointer.Reference, version.ObjectID, pointer.RevisionID, writer)
	if err != nil {
		return err
	}
	before, _ := json.Marshal(info)
	after, _ := json.Marshal(actual)
	if writer.n != row.Size || hex.EncodeToString(hash.Sum(nil)) != row.SHA256 || !bytes.Equal(before, after) {
		return ErrRecord
	}
	return nil
}

func importMIME(items []ImportedSource) map[string]storage.Reference {
	out := map[string]storage.Reference{}
	for _, item := range items {
		if item.Source.Kind == "mime_message" {
			out[item.Source.ID] = item.Archive
		}
	}
	return out
}

type projection struct {
	typ, id string
	payload []byte
}

func projectSource(row storage.StreamSourceRow, raw []byte, mimes map[string]storage.Reference) (projection, error) {
	var out projection
	switch row.Kind {
	case "mailbox", "label":
		var value struct {
			ID string `json:"id"`
		}
		if json.Unmarshal(raw, &value) != nil || value.ID != row.ID {
			return out, ErrRecord
		}
		return projection{typ: row.Kind, id: row.ID, payload: bytes.Clone(raw)}, nil
	case "account":
		var value struct {
			ID        string `json:"id"`
			Signature string `json:"signature"`
		}
		if json.Unmarshal(raw, &value) != nil || value.ID != row.ID {
			return out, ErrRecord
		}
		payload, _ := json.Marshal(value.Signature)
		return projection{typ: "signature", id: "signature", payload: payload}, nil
	case "message":
		var fields map[string]json.RawMessage
		if json.Unmarshal(raw, &fields) != nil {
			return out, ErrRecord
		}
		var id string
		if json.Unmarshal(fields["id"], &id) != nil || id != row.ID {
			return out, ErrRecord
		}
		// Retain soft-deleted originals without resurfacing them in the live inbox.
		if deleted, ok := fields["deleted_at"]; ok && !bytes.Equal(deleted, []byte("null")) {
			return out, nil
		}
		mime, ok := mimes[row.ID]
		if !ok {
			return out, ErrNotReady
		}
		for _, rename := range [][2]string{{"rfc822_msg_id", "message_id"}, {"from_addr", "from"}, {"to_addrs", "to"}, {"cc_addrs", "cc"}, {"bcc_addrs", "bcc"}, {"snippet", "preview"}, {"size_bytes", "size"}, {"flags", "keywords"}, {"has_attach", "has_attachment"}, {"sent_at", "date"}} {
			if value, ok := fields[rename[0]]; ok {
				fields[rename[1]] = value
				delete(fields, rename[0])
			}
		}
		// SQL stores a missing thread as null; the existing client uses an empty ID.
		if bytes.Equal(fields["thread_id"], []byte("null")) {
			fields["thread_id"] = json.RawMessage(`""`)
		}
		envelope, err := json.Marshal(fields)
		if err != nil {
			return out, err
		}
		defer clear(envelope)
		payload, err := json.Marshal(MessageDocument{Kind: messageKind, Envelope: envelope, MIME: mime})
		return projection{typ: "message", id: row.ID, payload: payload}, err
	default:
		if !privateBody(row) && !json.Valid(raw) {
			return out, ErrRecord
		}
		return out, nil
	}
}

// ImportRetainedSource is explicit and resumable. It freezes no account and
// removes no original. The complete descriptor plan is encrypted locally BEFORE
// publishing anything; retry never re-enumerates SQL pages after effects exist.
// Source files precede projections; all bytes/projections are checked before the
// sole completion receipt is published. Target changes or any mismatch fail shut.
func (r *Repository) ImportRetainedSource(parent context.Context, source Source) error {
	if source == nil {
		return ErrRecord
	}
	err := r.run(parent, false, func(ctx context.Context, s storage.AtomicCatalogSnapshot) error {
		r.ready = false
		if err := r.checkSourceState(ctx, source); err != nil {
			return err
		}
		if _, err := r.cfg.Crypto.PrepareStorage(ctx, r.cfg.Scope); err != nil {
			return err
		}
		if err := r.cfg.Files.Prepare(ctx); err != nil {
			return err
		}
		s, err := storage.LoadAtomicCatalog(ctx, r.cfg.Crypto, r.cfg.Scope)
		if err != nil {
			return err
		}
		owner := r.cfg.Scope.Tenant + "\x00" + r.cfg.Scope.Subject + "\x00mail"
		key := r.cfg.MigrationState.Epoch
		return r.cfg.Journal.Exclusive(ctx, owner, importNamespace, key, func() error { return r.importSnapshot(ctx, source, s, owner, key) })
	})
	if err != nil {
		return err
	}
	return r.Initialize(parent)
}

func (r *Repository) checkSourceState(ctx context.Context, source Source) error {
	current, err := source.PrivateStorageCapabilities(ctx, r.cfg.Scope)
	if err != nil {
		return err
	}
	maximum, err := limits.MessageBytes(current.MaxBodyBytes)
	if err != nil || current.Validate(r.cfg.Scope) != nil || current.Shared || !current.PrivateIngressReady || maximum != r.messageLimit() || !sameSource(current.LegacyMigrationState, r.cfg.MigrationState) {
		return ErrNotReady
	}
	return nil
}

func (r *Repository) importSnapshot(ctx context.Context, source Source, s storage.AtomicCatalogSnapshot, owner, key string) error {
	if len(heads(s, objectID("migration", key))) > 0 {
		return nil
	}
	work, err := r.loadImportIntent(ctx, source, s, owner, key)
	if err != nil {
		return err
	}
	if err := r.cfg.Crypto.CheckStoragePublicationTarget(work.Target); err != nil {
		return err
	}
	s, err = r.importArchives(ctx, source, s, owner, key, &work)
	if err != nil {
		return err
	}
	receipt, s, err := r.importProjections(ctx, s, owner, key, &work)
	if err != nil {
		return err
	}
	if err := r.validateSourceReceipt(ctx, s, receipt); err != nil {
		return err
	}
	if err := currentImportHeads(s, receipt.Records); err != nil {
		return err
	}
	if err := r.checkSourceState(ctx, source); err != nil {
		return err
	}
	payload, err := json.Marshal(receipt)
	if err != nil {
		return err
	}
	defer clear(payload)
	if err := r.writeDocument(ctx, s, "migration", key, payload, "live"); err != nil {
		return err
	}
	if err := r.checkSourceState(ctx, source); err != nil {
		return err
	}
	return r.cfg.Journal.Delete(owner, importNamespace, key)
}

func (r *Repository) loadImportIntent(ctx context.Context, source Source, s storage.AtomicCatalogSnapshot, owner, key string) (importIntent, error) {
	var work importIntent
	found, err := r.cfg.Journal.Read(owner, importNamespace, key, &work)
	if err != nil {
		return work, err
	}
	if !found {
		if len(s.Entries) != 0 {
			return work, ErrConflict
		}
		work = importIntent{Version: 1, State: r.cfg.MigrationState, Target: s.Binding.Target, Items: []importItem{}}
		work.Items, err = r.enumerateSource(ctx, source, key)
		if err != nil {
			return work, err
		}
		if err := r.cfg.Journal.Write(owner, importNamespace, key, work); err != nil {
			return work, err
		}
	}
	if work.Version != 1 || !sameSource(work.State, r.cfg.MigrationState) || checkSourcePlan(work.State, work.Items) != nil || work.Target != s.Binding.Target {
		return work, ErrRecord
	}
	return work, nil
}

func (r *Repository) enumerateSource(ctx context.Context, source Source, epoch string) ([]importItem, error) {
	items := []importItem{}
	after := storage.LegacySourceCursor{}
	for {
		page, err := source.PrivateStorageSourcePage(ctx, r.cfg.Scope, epoch, after)
		if err != nil {
			return nil, err
		}
		if page.Validate(r.cfg.Scope, epoch, after) != nil {
			return nil, ErrRecord
		}
		for _, row := range page.Rows {
			items = append(items, importItem{Source: row})
		}
		if int64(len(items)) > r.cfg.MigrationState.Rows+r.cfg.MigrationState.Blobs {
			return nil, ErrRecord
		}
		if page.Next == nil {
			break
		}
		after = *page.Next
	}
	if err := checkSourcePlan(r.cfg.MigrationState, items); err != nil {
		return nil, err
	}
	return items, nil
}

const importStageNamespace = "mail-private-source-stage-v1"

func (r *Repository) importArchives(ctx context.Context, source Source, s storage.AtomicCatalogSnapshot, owner, key string, work *importIntent) (storage.AtomicCatalogSnapshot, error) {
	for i := range work.Items {
		item := &work.Items[i]
		if item.Archive != nil {
			raw, err := r.verifyArchive(ctx, s, item.Source, *item.Archive)
			clear(raw)
			if err != nil {
				return s, err
			}
			continue
		}
		if err := r.stageImportSource(ctx, source, owner, key, work, item); err != nil {
			return s, err
		}
		if err := r.publishImportArchive(ctx, s, owner, key, work, item); err != nil {
			return s, err
		}
		var err error
		s, err = storage.LoadAtomicCatalog(ctx, r.cfg.Crypto, r.cfg.Scope)
		if err != nil {
			return s, err
		}
		if s.Binding.Target != work.Target {
			return s, ErrRecord
		}
		raw, err := r.verifyArchive(ctx, s, item.Source, *item.Archive)
		clear(raw)
		if err != nil {
			return s, err
		}
		if err := r.finishImportArchive(ctx, owner, key, work, item); err != nil {
			return s, err
		}
	}
	return s, nil
}

func (r *Repository) stageImportSource(ctx context.Context, source Source, owner, key string, work *importIntent, item *importItem) error {
	if item.Staged == nil {
		body, err := source.OpenPrivateStorageSource(ctx, r.cfg.Scope, key, item.Source)
		if err != nil {
			return err
		}
		staged, captureErr := r.cfg.Journal.StageFile(ctx, owner, importStageNamespace, body, item.Source.Size)
		closeErr := body.Close()
		if err := errors.Join(captureErr, closeErr); err != nil {
			if captureErr == nil {
				_ = r.cfg.Journal.DeleteStagedFile(owner, importStageNamespace, staged)
			}
			return err
		}
		if staged.SizeBytes != item.Source.Size || staged.SHA256 != item.Source.SHA256 {
			_ = r.cfg.Journal.DeleteStagedFile(owner, importStageNamespace, staged)
			return ErrRecord
		}
		item.Staged = &staged
		item.Revision = uuid.NewString()
		if err := r.cfg.Journal.Write(owner, importNamespace, key, work); err != nil {
			_ = r.cfg.Journal.DeleteStagedFile(owner, importStageNamespace, staged)
			return err
		}
	}
	if !storage.Identifier(item.Revision) || item.Staged.SizeBytes != item.Source.Size || item.Staged.SHA256 != item.Source.SHA256 {
		return ErrRecord
	}
	return nil
}

func importArchiveJob(epoch string, item *importItem) string {
	return "mail_source_" + digest([]byte(epoch+"\x00"+item.Source.Kind+"\x00"+item.Source.ID)) + "_" + item.Revision
}

func (r *Repository) publishImportArchive(ctx context.Context, s storage.AtomicCatalogSnapshot, owner, key string, work *importIntent, item *importItem) error {
	typ, id := sourceArchive(item.Source)
	object := objectID(typ, id)
	job := importArchiveJob(key, item)
	if item.File == nil {
		reader, err := r.cfg.Journal.OpenStagedFile(ctx, owner, importStageNamespace, *item.Staged)
		if err != nil {
			return err
		}
		ref, uploadErr := r.cfg.Files.Upload(ctx, job, work.Target, storage.FileRequest{ObjectID: object, RevisionID: item.Revision, SizeBytes: item.Source.Size, Metadata: []byte(`{"kind":"org.sytue.mail.source.v1"}`)}, reader)
		_ = reader.Close()
		if uploadErr != nil {
			return uploadErr
		}
		if !ref.Valid() || ref.RoomID != work.Target.RoomID {
			return ErrRecord
		}
		item.File = &ref
		if err := r.cfg.Journal.Write(owner, importNamespace, key, work); err != nil {
			return err
		}
	}
	info, sum, err := r.cfg.Files.Commitment(ctx, *item.File, object, item.Revision)
	if err != nil {
		return err
	}
	if info.Validate(r.cfg.Scope) != nil || info.ObjectID != object || info.RevisionID != item.Revision || info.SizeBytes != item.Source.Size || sum != item.Source.SHA256 {
		return ErrRecord
	}
	metadata, _ := json.Marshal(header{Kind: kind, Type: typ, ID: id})
	commit := storage.ObjectCommit{Kind: storage.ObjectKind, State: "live", Metadata: metadata, Content: &storage.ContentPointer{Kind: "file", RevisionID: item.Revision, Reference: *item.File}}
	if err := expectedImportHead(s, object, item.Revision+"_index", commit); err != nil {
		return err
	}
	index, err := r.cfg.Crypto.PutStorageObjectAt(ctx, work.Target, job+"_index", object, item.Revision+"_index", commit)
	if err != nil {
		return err
	}
	expected, _ := json.Marshal(commit)
	actual, _ := json.Marshal(index.Commit)
	if index.ObjectID != object || index.RevisionID != item.Revision+"_index" || !index.Reference.Valid() || index.Reference.RoomID != work.Target.RoomID || !bytes.Equal(expected, actual) {
		return ErrRecord
	}
	item.Archive = &index.Reference
	return r.cfg.Journal.Write(owner, importNamespace, key, work)
}

func expectedImportHead(s storage.AtomicCatalogSnapshot, object, revision string, commit storage.ObjectCommit) error {
	versions := heads(s, object)
	if len(versions) == 0 {
		return nil
	}
	if len(versions) != 1 || versions[0].RevisionID != revision {
		return ErrConflict
	}
	expected, _ := json.Marshal(commit)
	actual, _ := json.Marshal(versions[0].Commit)
	if !bytes.Equal(expected, actual) {
		return ErrConflict
	}
	return nil
}

func (r *Repository) finishImportArchive(ctx context.Context, owner, key string, work *importIntent, item *importItem) error {
	if err := r.cfg.Files.Acknowledge(ctx, importArchiveJob(key, item), *item.Archive); err != nil && !errors.Is(err, storage.ErrUploadMissing) {
		return err
	}
	if item.Staged != nil {
		if err := r.cfg.Journal.DeleteStagedFile(owner, importStageNamespace, *item.Staged); err != nil {
			return err
		}
		item.Staged = nil
	}
	return r.cfg.Journal.Write(owner, importNamespace, key, work)
}

func (r *Repository) importProjections(ctx context.Context, s storage.AtomicCatalogSnapshot, owner, key string, work *importIntent) (ImportReceipt, storage.AtomicCatalogSnapshot, error) {
	receipt := ImportReceipt{Kind: migrationKindV2, Epoch: key, Records: []storage.Reference{}, Sources: []ImportedSource{}}
	for _, item := range work.Items {
		receipt.Sources = append(receipt.Sources, ImportedSource{Source: item.Source, Archive: *item.Archive})
	}
	mimes := importMIME(receipt.Sources)
	for i := range work.Items {
		item := &work.Items[i]
		var err error
		s, err = r.importProjection(ctx, s, owner, key, work, item, mimes)
		if err != nil {
			return receipt, s, err
		}
		receipt.Sources[i].Projection = item.Projection
		receipt.Records = append(receipt.Records, *item.Archive)
		if item.Projection != nil {
			receipt.Records = append(receipt.Records, *item.Projection)
		}
	}
	return receipt, s, nil
}

func (r *Repository) importProjection(ctx context.Context, s storage.AtomicCatalogSnapshot, owner, key string, work *importIntent, item *importItem, mimes map[string]storage.Reference) (storage.AtomicCatalogSnapshot, error) {
	raw, err := r.verifyArchive(ctx, s, item.Source, *item.Archive)
	if err != nil {
		return s, err
	}
	projected, err := projectSource(item.Source, raw, mimes)
	clear(raw)
	if err != nil {
		return s, err
	}
	defer clear(projected.payload)
	if projected.typ == "" {
		if item.Projection != nil {
			return s, ErrRecord
		}
		return s, nil
	}
	if item.Projection != nil {
		return s, nil
	}
	if err := r.checkImportContent(ctx, s, projected.typ, projected.id, projected.payload); err != nil {
		return s, err
	}
	if err := r.writeDocument(ctx, s, projected.typ, projected.id, projected.payload, "live"); err != nil {
		return s, err
	}
	s, err = storage.LoadAtomicCatalog(ctx, r.cfg.Crypto, r.cfg.Scope)
	if err != nil {
		return s, err
	}
	if s.Binding.Target != work.Target {
		return s, ErrRecord
	}
	versions := heads(s, objectID(projected.typ, projected.id))
	if len(versions) != 1 {
		return s, ErrConflict
	}
	item.Projection = &versions[0].Reference
	return s, r.cfg.Journal.Write(owner, importNamespace, key, work)
}

func (r *Repository) validateSourceReceipt(ctx context.Context, s storage.AtomicCatalogSnapshot, receipt ImportReceipt) error {
	if receipt.Kind != migrationKindV2 || receipt.Epoch != r.cfg.MigrationState.Epoch {
		return ErrRecord
	}
	items := make([]importItem, len(receipt.Sources))
	expected := map[storage.Reference]bool{}
	sizes := map[string]int64{}
	for i, item := range receipt.Sources {
		items[i] = importItem{Source: item.Source}
		if !item.Archive.Valid() || expected[item.Archive] {
			return ErrRecord
		}
		expected[item.Archive] = true
		if item.Source.Kind == "mime_message" {
			sizes[item.Source.ID] = item.Source.Size
		}
	}
	if err := checkSourcePlan(r.cfg.MigrationState, items); err != nil {
		return err
	}
	mimes := importMIME(receipt.Sources)
	_, refs, mailboxes, err := r.receiptInventory(s, receipt)
	if err != nil {
		return err
	}
	if len(mailboxes) == 0 {
		return ErrNotReady
	}
	for _, item := range receipt.Sources {
		if err := r.validateSourceMapping(ctx, s, item, mimes, sizes, refs, mailboxes); err != nil {
			return err
		}
		if item.Projection != nil {
			if expected[*item.Projection] {
				return ErrRecord
			}
			expected[*item.Projection] = true
		}
	}
	if len(expected) != len(receipt.Records) {
		return ErrRecord
	}
	for _, ref := range receipt.Records {
		if !expected[ref] {
			return ErrRecord
		}
		delete(expected, ref)
	}
	if len(expected) != 0 {
		return ErrRecord
	}
	return nil
}

func (r *Repository) validateSourceMapping(ctx context.Context, s storage.AtomicCatalogSnapshot, item ImportedSource, mimes map[string]storage.Reference, sizes map[string]int64, refs map[storage.Reference]bool, mailboxes map[string]bool) error {
	raw, err := r.verifyArchive(ctx, s, item.Source, item.Archive)
	if err != nil {
		return err
	}
	projected, err := projectSource(item.Source, raw, mimes)
	clear(raw)
	if err != nil {
		return err
	}
	defer clear(projected.payload)
	if projected.typ == "" {
		if item.Projection != nil {
			return ErrRecord
		}
		return nil
	}
	if item.Projection == nil || !item.Projection.Valid() {
		return ErrRecord
	}
	if projected.typ == "message" {
		if err := validateProjectedMIME(projected, sizes); err != nil {
			return err
		}
	}
	entry, ok := s.Entries[*item.Projection]
	if !ok {
		return ErrNotReady
	}
	version, found, err := storage.AcceptedAtomicObject(s, r.cfg.Scope, entry.Record.ObjectID, "", item.Projection)
	if err != nil || !found {
		return ErrRecord
	}
	h, err := decodeHeader(version)
	if err != nil || h.Type != projected.typ || h.ID != projected.id {
		return ErrRecord
	}
	actual, err := r.readDocument(ctx, version)
	if err != nil {
		return err
	}
	defer clear(actual)
	if !sameProjectionPayload(projected.typ, actual, projected.payload) {
		return ErrRecord
	}
	return r.validateImportedDocument(s, h, actual, refs, mailboxes)
}

// Rewriting a message's SDK MIME reference may reorder its outer JSON fields.
// Compare that object using RawMessage values; the envelope is base64 bytes and
// remains exact, including original whitespace, extension fields and integers.
// Every other archived projection still requires its original byte sequence.
func sameProjectionPayload(typ string, actual, expected []byte) bool {
	if bytes.Equal(actual, expected) {
		return true
	}
	if typ != "message" {
		return false
	}
	var left, right map[string]json.RawMessage
	if json.Unmarshal(actual, &left) != nil || json.Unmarshal(expected, &right) != nil || left == nil || right == nil {
		return false
	}
	a, err := json.Marshal(left)
	if err != nil {
		return false
	}
	defer clear(a)
	b, err := json.Marshal(right)
	if err != nil {
		return false
	}
	defer clear(b)
	return bytes.Equal(a, b)
}

func validateProjectedMIME(projected projection, sizes map[string]int64) error {
	message, document, err := decodeMessage(projected.payload, projected.id)
	defer clear(document.Envelope)
	if err != nil {
		return err
	}
	size, ok := sizes[projected.id]
	if !ok || size != message.Size {
		return ErrRecord
	}
	return nil
}

func (r *Repository) checkImportContent(ctx context.Context, s storage.AtomicCatalogSnapshot, typ, id string, payload []byte) error {
	versions := heads(s, objectID(typ, id))
	if len(versions) == 0 {
		return nil
	}
	if len(versions) != 1 {
		return ErrConflict
	}
	raw, err := r.readDocument(ctx, versions[0])
	if err != nil {
		return err
	}
	same := bytes.Equal(raw, payload)
	clear(raw)
	if !same {
		return ErrConflict
	}
	return nil
}

func currentImportHeads(s storage.AtomicCatalogSnapshot, refs []storage.Reference) error {
	if len(s.Heads) != len(refs) {
		return ErrConflict
	}
	expected := map[storage.Reference]bool{}
	for _, ref := range refs {
		expected[ref] = true
	}
	for _, head := range s.Heads {
		if !expected[head.Reference] {
			return ErrConflict
		}
		delete(expected, head.Reference)
	}
	if len(expected) != 0 {
		return ErrConflict
	}
	return nil
}
