// SPDX-License-Identifier: Apache-2.0
package notes

import (
	"bytes"
	"context"
	"encoding/json"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/buscar"
	"git.sytue.eu/sytue/one/apps/nucleo/htmltext"
	"git.sytue.eu/sytue/one/apps/nucleo/notes/model"
)

func (r *Repository) notebooks(ctx context.Context) (map[string]api.Notebook, error) {
	versions, err := r.selected("notebook")
	if err != nil {
		return nil, err
	}
	out := map[string]api.Notebook{}
	for _, version := range versions {
		if version.Commit.State != "live" {
			continue
		}
		raw, err := r.readJSON(ctx, version)
		if err != nil {
			return nil, err
		}
		var nb api.Notebook
		err = json.Unmarshal(raw, &nb)
		clear(raw)
		if err != nil || nb.ID == "" {
			return nil, ErrSource
		}
		out[nb.ID] = nb
	}
	return out, nil
}

func visibleNotebook(notebooks map[string]api.Notebook, id string) bool {
	seen := map[string]bool{}
	for id != "" {
		if seen[id] {
			return false
		}
		seen[id] = true
		nb, ok := notebooks[id]
		if !ok {
			return false
		}
		if nb.ParentID == nil {
			return true
		}
		id = *nb.ParentID
	}
	return false
}

func (r *Repository) ListNotebooks(parent context.Context) ([]api.Notebook, error) {
	ctx, done, err := r.start(parent)
	if err != nil {
		return nil, err
	}
	defer done()
	books, err := r.notebooks(ctx)
	if err != nil {
		return nil, err
	}
	out := []api.Notebook{}
	for _, nb := range books {
		if visibleNotebook(books, nb.ID) {
			out = append(out, nb)
		}
	}
	slices.SortStableFunc(out, func(a, b api.Notebook) int {
		if a.Position < b.Position {
			return -1
		}
		if a.Position > b.Position {
			return 1
		}
		if a.Title < b.Title {
			return -1
		}
		if a.Title > b.Title {
			return 1
		}
		if a.ID < b.ID {
			return -1
		}
		if a.ID > b.ID {
			return 1
		}
		return 0
	})
	return out, nil
}

func (r *Repository) GetNotebook(parent context.Context, id string) (api.Notebook, error) {
	ctx, done, err := r.start(parent)
	if err != nil {
		return api.Notebook{}, err
	}
	defer done()
	books, err := r.notebooks(ctx)
	if err != nil {
		return api.Notebook{}, err
	}
	if !visibleNotebook(books, id) {
		return api.Notebook{}, ErrNotFound
	}
	return books[id], nil
}

func (r *Repository) ListNotes(parent context.Context, notebook string) ([]api.Note, error) {
	ctx, done, err := r.start(parent)
	if err != nil {
		return nil, err
	}
	defer done()
	if _, err := r.GetNotebook(ctx, notebook); err != nil {
		return nil, err
	}
	_, notes, err := r.visibleNotes(ctx)
	if err != nil {
		return nil, err
	}
	out := []api.Note{}
	for _, note := range notes {
		if note.NotebookID == notebook {
			out = append(out, note)
		}
	}

	sortNotes(out)
	return out, nil
}

func sortNotes(notes []api.Note) {
	slices.SortStableFunc(notes, func(a, b api.Note) int {
		if a.Pinned != b.Pinned {
			if a.Pinned {
				return -1
			}
			return 1
		}
		if a.Col < b.Col {
			return -1
		}
		if a.Col > b.Col {
			return 1
		}
		if a.Position < b.Position {
			return -1
		}
		if a.Position > b.Position {
			return 1
		}
		if a.CreatedAt.Before(b.CreatedAt) {
			return -1
		}
		if a.CreatedAt.After(b.CreatedAt) {
			return 1
		}
		if a.ID < b.ID {
			return -1
		}
		if a.ID > b.ID {
			return 1
		}
		return 0
	})
}

func rawObject(raw []byte) (map[string]json.RawMessage, error) {
	var value map[string]json.RawMessage
	if json.Unmarshal(raw, &value) != nil || value == nil {
		return nil, ErrSource
	}
	return value, nil
}

func put(value map[string]json.RawMessage, key string, input any) error {
	raw, err := json.Marshal(input)
	if err == nil {
		value[key] = raw
	}
	return err
}

func (r *Repository) CreateNotebook(ctx context.Context, in api.NotebookInput) (api.Notebook, error) {
	raw, err := r.mutate(ctx, "create-notebook", in, func(ctx context.Context) (string, string, string, []byte, error) {
		in, err := normalizeNotebookInput(in)
		if err != nil {
			return "", "", "", nil, err
		}
		if (in.Visibility != "" && in.Visibility != "private" && in.Visibility != "personal") || in.RoomID != "" {
			return "", "", "", nil, ErrShared
		}
		if strings.TrimSpace(in.Title) == "" {
			return "", "", "", nil, ErrSource
		}
		if in.ParentID != nil {
			if _, err := r.GetNotebook(ctx, *in.ParentID); err != nil {
				return "", "", "", nil, err
			}
		}
		if in.Encrypted {
			if _, ok := api.CryptoDe(in.Crypto); !ok {
				return "", "", "", nil, ErrSource
			}
		}
		now := time.Now().UTC()
		nb := api.Notebook{ID: uuid.NewString(), OwnerSub: r.cfg.Scope.Subject, ParentID: in.ParentID, Title: in.Title, Emoji: in.Emoji, Color: in.Color, Layout: in.Layout, Visibility: in.Visibility, Encrypted: in.Encrypted, Crypto: in.Crypto, RoomID: in.RoomID, UpdatedAt: now}
		if in.Position != nil {
			nb.Position = *in.Position
		}
		if in.Indexable != nil {
			nb.Indexable = *in.Indexable
		}
		encoded, err := encodeNewNotebook(nb, now)
		return "notebook", nb.ID, "live", encoded, err
	})
	defer clear(raw)
	var result api.Notebook
	if err == nil {
		err = json.Unmarshal(raw, &result)
	}
	return result, err
}

func (r *Repository) UpdateNotebook(ctx context.Context, id string, in api.NotebookInput) (api.Notebook, error) {
	raw, err := r.mutate(ctx, "update-notebook", struct {
		ID       string
		Input    api.NotebookInput
		Expected *time.Time
	}{id, in, in.ExpectedUpdatedAt}, func(ctx context.Context) (string, string, string, []byte, error) {
		return r.prepareNotebookUpdate(ctx, id, in)
	})
	defer clear(raw)
	var result api.Notebook
	if err == nil {
		err = json.Unmarshal(raw, &result)
	}
	return result, err
}

func bytesEqualJSON(a, b json.RawMessage) bool {
	if !json.Valid(a) || !json.Valid(b) {
		return false
	}
	var x, y any
	dx := json.NewDecoder(strings.NewReader(string(a)))
	dx.UseNumber()
	dy := json.NewDecoder(strings.NewReader(string(b)))
	dy.UseNumber()
	if dx.Decode(&x) != nil || dy.Decode(&y) != nil {
		return false
	}
	ax, _ := json.Marshal(x)
	by, _ := json.Marshal(y)
	return bytes.Equal(ax, by)
}

func (r *Repository) DeleteNotebook(ctx context.Context, id string) error {
	raw, err := r.mutate(ctx, "delete-notebook", id, func(ctx context.Context) (string, string, string, []byte, error) {
		if _, err := r.GetNotebook(ctx, id); err != nil {
			return "", "", "", nil, err
		}
		old, err := r.ReadRaw(ctx, "notebook", id)
		if err != nil {
			return "", "", "", nil, err
		}
		defer clear(old)
		fields, err := rawObject(old)
		if err != nil {
			return "", "", "", nil, err
		}
		_ = put(fields, "deleted_at", time.Now().UTC())
		encoded, err := json.Marshal(fields)
		return "notebook", id, "trash", encoded, err
	})
	clear(raw)
	return err
}

func (r *Repository) CreateNote(ctx context.Context, notebook string, in api.NoteInput) (api.Note, error) {
	return r.saveNote(ctx, "", notebook, in)
}

func (r *Repository) UpdateNote(ctx context.Context, id, notebook string, in api.NoteInput) (api.Note, error) {
	return r.saveNote(ctx, id, notebook, in)
}

func (r *Repository) saveNote(ctx context.Context, id, notebook string, in api.NoteInput) (api.Note, error) {
	return r.saveNoteAcknowledged(ctx, id, notebook, in, nil)
}

func (r *Repository) saveNoteAcknowledged(ctx context.Context, id, notebook string, in api.NoteInput, acknowledge func(context.Context, api.Note) error) (api.Note, error) {
	op := "update-note"
	if id == "" {
		op = "create-note"
	}
	request := noteMutationRequest{id, notebook, in, in.ExpectedUpdatedAt}
	prepare := func(ctx context.Context) (string, string, string, []byte, error) {
		return r.prepareNote(ctx, id, notebook, in)
	}
	var raw []byte
	var err error
	if acknowledge == nil {
		raw, err = r.mutate(ctx, op, request, prepare)
	} else {
		raw, err = r.engine.MutateAcknowledged(ctx, op, request, prepare, func(ctx context.Context, exact []byte) error {
			var accepted api.Note
			if json.Unmarshal(exact, &accepted) != nil || accepted.ID != id || accepted.NotebookID != notebook {
				return ErrSource
			}
			return acknowledge(ctx, accepted)
		})
	}
	defer clear(raw)
	var result api.Note
	if err == nil {
		err = json.Unmarshal(raw, &result)
	}
	return result, err
}

func (r *Repository) markNote(ctx context.Context, id string, trash bool) error {
	raw, err := r.mutate(ctx, "trash-note", struct {
		ID    string
		Trash bool
	}{id, trash}, func(ctx context.Context) (string, string, string, []byte, error) {
		version, err := r.readyVersion("note", id)
		if err != nil {
			return "", "", "", nil, err
		}
		if trash && version.Commit.State != "live" || !trash && version.Commit.State != "trash" {
			return "", "", "", nil, ErrNotFound
		}
		old, err := r.readJSON(ctx, version)
		if err != nil {
			return "", "", "", nil, err
		}
		defer clear(old)
		var note api.Note
		var purga struct {
			PurgedAt *time.Time `json:"purged_at"`
		}
		if json.Unmarshal(old, &note) != nil || json.Unmarshal(old, &purga) != nil {
			return "", "", "", nil, ErrSource
		}
		if purga.PurgedAt != nil {
			return "", "", "", nil, ErrNotFound
		}
		if _, err := r.GetNotebook(ctx, note.NotebookID); err != nil {
			return "", "", "", nil, err
		}
		fields, err := rawObject(old)
		if err != nil {
			return "", "", "", nil, err
		}
		var deleted any
		state := "live"
		if trash {
			deleted = time.Now().UTC()
			state = "trash"
		}
		_ = put(fields, "deleted_at", deleted)
		_ = put(fields, "updated_at", time.Now().UTC())
		encoded, err := json.Marshal(fields)
		return "note", id, state, encoded, err
	})
	clear(raw)
	return err
}

func (r *Repository) DeleteNote(ctx context.Context, id string) error {
	return r.markNote(ctx, id, true)
}

func (r *Repository) RestoreNote(ctx context.Context, id string) error {
	return r.markNote(ctx, id, false)
}

// ListTrash devuelve la papelera con lo que pinta la fila: de qué cuaderno viene y
// cuándo se borró. El almacenamiento privado no tiene recolector, así que `PurgeAt`
// va siempre nil («no se borra automáticamente»), que es lo que de verdad pasa.
func (r *Repository) ListTrash(parent context.Context, limit int) ([]api.TrashedNote, error) {
	ctx, done, err := r.start(parent)
	if err != nil {
		return nil, err
	}
	defer done()
	versions, err := r.selected("note")
	if err != nil {
		return nil, err
	}
	cuadernos, err := r.notebooks(ctx)
	if err != nil {
		return nil, err
	}
	out := []api.TrashedNote{}
	for _, version := range versions {
		if version.Commit.State != "trash" {
			continue
		}
		raw, err := r.readJSON(ctx, version)
		if err != nil {
			return nil, err
		}
		var note api.Note
		var borrado struct {
			DeletedAt *time.Time `json:"deleted_at"`
			PurgedAt  *time.Time `json:"purged_at"`
		}
		err = json.Unmarshal(raw, &note)
		if err == nil {
			err = json.Unmarshal(raw, &borrado)
		}
		clear(raw)
		if err != nil {
			return nil, ErrSource
		}
		if borrado.PurgedAt != nil {
			continue // borrada para siempre (PurgeNote)
		}
		item := api.TrashedNote{Note: note, DeletedAt: note.UpdatedAt}
		if borrado.DeletedAt != nil {
			item.DeletedAt = *borrado.DeletedAt
		}
		if nb, ok := cuadernos[note.NotebookID]; ok {
			item.NotebookTitle = nb.Title
		}
		out = append(out, item)
	}
	slices.SortStableFunc(out, func(a, b api.TrashedNote) int {
		if a.UpdatedAt.After(b.UpdatedAt) {
			return -1
		}
		if a.UpdatedAt.Before(b.UpdatedAt) {
			return 1
		}
		if a.ID < b.ID {
			return -1
		}
		if a.ID > b.ID {
			return 1
		}
		return 0
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (r *Repository) Search(parent context.Context, query string, limit int, archived bool) ([]api.NoteHit, error) {
	ctx, done, err := r.start(parent)
	if err != nil {
		return nil, err
	}
	defer done()
	books, err := r.notebooks(ctx)
	if err != nil {
		return nil, err
	}
	versions, err := r.selected("note")
	if err != nil {
		return nil, err
	}
	out := []api.NoteHit{}
	for _, version := range versions {
		if version.Commit.State != "live" {
			continue
		}
		raw, err := r.readJSON(ctx, version)
		if err != nil {
			return nil, err
		}
		var note api.Note
		err = json.Unmarshal(raw, &note)
		clear(raw)
		if err != nil {
			return nil, ErrSource
		}
		nb, ok := books[note.NotebookID]
		if !ok || !visibleNotebook(books, note.NotebookID) || (!archived && nb.ArchivedAt != nil) || nb.Encrypted || !noteIndexable(nb, note) {
			continue
		}
		text := htmltext.Text(note.Content)
		if tag, exact := model.HashtagDeConsulta(query); exact {
			key := buscar.Plegar(tag)
			if !slices.ContainsFunc(model.Hashtags(note.Title+"\n"+text), func(value string) bool { return buscar.Plegar(value) == key }) {
				continue
			}
		} else if !buscar.Contiene(note.Title+" "+text, query) {
			continue
		}
		out = append(out, api.NoteHit{ID: note.ID, NotebookID: note.NotebookID, Notebook: nb.Title, Title: note.Title, Snippet: text, Kind: note.Kind, UpdatedAt: note.UpdatedAt})
	}
	sortHits(out)
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (r *Repository) prepareNotebookUpdate(ctx context.Context, id string, in api.NotebookInput) (string, string, string, []byte, error) {
	nb, err := r.GetNotebook(ctx, id)
	if err != nil {
		return "", "", "", nil, err
	}
	in, err = normalizeNotebookInput(in)
	if err != nil {
		return "", "", "", nil, err
	}
	if err := checkNotebookUpdate(nb, in); err != nil {
		return "", "", "", nil, err
	}
	books, err := r.notebooks(ctx)
	if err != nil {
		return "", "", "", nil, err
	}
	seen := map[string]bool{}
	for parent := in.ParentID; parent != nil; {
		if seen[*parent] {
			return "", "", "", nil, ErrSource
		}
		seen[*parent] = true
		if *parent == id {
			return "", "", "", nil, ErrSource
		}
		p, ok := books[*parent]
		if !ok {
			return "", "", "", nil, ErrNotFound
		}
		parent = p.ParentID
	}
	old, err := r.ReadRaw(ctx, "notebook", id)
	if err != nil {
		return "", "", "", nil, err
	}
	defer clear(old)
	fields, err := rawObject(old)
	if err != nil {
		return "", "", "", nil, err
	}
	for key, value := range map[string]any{"parent_id": in.ParentID, "title": in.Title, "emoji": in.Emoji, "color": in.Color, "layout": in.Layout, "visibility": in.Visibility, "encrypted": in.Encrypted, "crypto": in.Crypto, "room_id": in.RoomID, "updated_at": time.Now().UTC()} {
		if err := put(fields, key, value); err != nil {
			return "", "", "", nil, err
		}
	}
	if in.Position != nil {
		_ = put(fields, "position", *in.Position)
	}
	if in.Indexable != nil {
		_ = put(fields, "indexable", *in.Indexable)
	}
	encoded, err := json.Marshal(fields)
	return "notebook", id, "live", encoded, err
}

func checkNoteInput(nb api.Notebook, in api.NoteInput) error {
	if in.Visibility != "" && in.Visibility != "inherit" && in.Visibility != "private" && in.Visibility != "personal" {
		return ErrShared
	}
	if nb.Encrypted && (in.ContentCipher == "" || in.CipherVersion != 2 || !bytesEqualJSON(nb.Crypto, in.ExpectedCrypto)) {
		return ErrConflict
	}
	if in.ContentCipher != "" && (in.Title != "" || in.Content != "") {
		return ErrSource
	}
	return nil
}

func (r *Repository) noteFields(ctx context.Context, id string, in api.NoteInput, now time.Time) (map[string]json.RawMessage, string, error) {
	if id == "" {
		target := uuid.NewString()
		fields := map[string]json.RawMessage{}
		_ = put(fields, "id", target)
		_ = put(fields, "owner_sub", r.cfg.Scope.Subject)
		_ = put(fields, "created_at", now)
		_ = put(fields, "width", 260)
		return fields, target, nil
	}
	version, err := r.readyVersion("note", id)
	if err != nil {
		return nil, "", err
	}
	if version.Commit.State != "live" {
		return nil, "", ErrNotFound
	}
	old, err := r.readJSON(ctx, version)
	if err != nil {
		return nil, "", err
	}
	defer clear(old)
	var note api.Note
	if json.Unmarshal(old, &note) != nil {
		return nil, "", ErrSource
	}
	if in.ExpectedUpdatedAt != nil && !note.UpdatedAt.Equal(*in.ExpectedUpdatedAt) {
		return nil, "", ErrConflict
	}
	fields, err := rawObject(old)
	return fields, id, err
}

func applyNoteInput(fields map[string]json.RawMessage, notebook string, in api.NoteInput, now time.Time) error {
	for key, value := range map[string]any{"notebook_id": notebook, "kind": in.Kind, "title": in.Title, "content": in.Content, "content_cipher": in.ContentCipher, "data": in.Data, "links": in.Links, "updated_at": now, "visibility": in.Visibility} {
		if err := put(fields, key, value); err != nil {
			return err
		}
	}
	for key, value := range map[string]*int{"x": in.X, "y": in.Y, "col": in.Col, "position": in.Position, "width": in.Width} {
		if value != nil {
			_ = put(fields, key, *value)
		}
	}
	for key, value := range map[string]*bool{"collapsed": in.Collapsed, "checked": in.Checked, "pinned": in.Pinned, "indexable": in.Indexable} {
		if value != nil {
			_ = put(fields, key, *value)
		}
	}
	_ = put(fields, "group_id", in.GroupID)
	applyTaskState(fields, in, now)
	return nil
}

func applyTaskState(fields map[string]json.RawMessage, in api.NoteInput, now time.Time) {
	if in.Checked != nil {
		if *in.Checked {
			_ = put(fields, "checked_at", now)
			_ = put(fields, "cancelled_at", nil)
		} else {
			_ = put(fields, "checked_at", nil)
		}
	}
	if in.Cancelled != nil {
		if *in.Cancelled {
			_ = put(fields, "cancelled_at", now)
			_ = put(fields, "checked", false)
			_ = put(fields, "checked_at", nil)
		} else {
			_ = put(fields, "cancelled_at", nil)
		}
	}
}

func (r *Repository) prepareNote(ctx context.Context, id, notebook string, in api.NoteInput) (string, string, string, []byte, error) {
	nb, err := r.GetNotebook(ctx, notebook)
	if err != nil {
		return "", "", "", nil, err
	}
	if err := checkNoteInput(nb, in); err != nil {
		return "", "", "", nil, err
	}
	in, err = normalizeNoteInput(in, nb.Encrypted, r.local)
	if err != nil {
		return "", "", "", nil, err
	}
	now := time.Now().UTC()
	fields, target, err := r.noteFields(ctx, id, in, now)
	if err != nil {
		return "", "", "", nil, err
	}
	if err := r.checkNoteRelations(ctx, target, notebook, in.GroupID); err != nil {
		return "", "", "", nil, err
	}
	if in.Kind == "" {
		in.Kind = "text"
	}
	if err := applyNoteInput(fields, notebook, in, now); err != nil {
		return "", "", "", nil, err
	}
	encoded, err := json.Marshal(fields)
	return "note", target, "live", encoded, err
}

func checkNotebookUpdate(nb api.Notebook, in api.NotebookInput) error {
	if (in.Visibility != "" && in.Visibility != "private" && in.Visibility != "personal") || in.RoomID != "" {
		return ErrShared
	}
	if nb.Encrypted != in.Encrypted || nb.Encrypted && !bytesEqualJSON(nb.Crypto, in.Crypto) {
		return ErrConflict
	}
	if in.ExpectedUpdatedAt != nil && !nb.UpdatedAt.Equal(*in.ExpectedUpdatedAt) {
		return ErrConflict
	}
	return nil
}

func encodeNewNotebook(nb api.Notebook, created time.Time) ([]byte, error) {
	raw, err := json.Marshal(nb)
	if err != nil {
		return nil, err
	}
	fields, err := rawObject(raw)
	if err != nil {
		return nil, err
	}
	if err := put(fields, "created_at", created); err != nil {
		return nil, err
	}
	return json.Marshal(fields)
}
