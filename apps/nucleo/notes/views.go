// SPDX-License-Identifier: Apache-2.0
package notes

import (
	"cmp"
	"context"
	"encoding/json"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/buscar"
	"git.sytue.eu/sytue/one/apps/nucleo/htmltext"
	"git.sytue.eu/sytue/one/apps/nucleo/notes/model"
)

var _ api.NotesViewsData = (*Repository)(nil)

// eachLiveJSON reads authenticated current versions only. No auxiliary view
// consults the retained plaintext source or materializes an on-disk index.
func (r *Repository) eachLiveJSON(ctx context.Context, kind string, visit func([]byte) error) error {
	versions, err := r.selected(kind)
	if err != nil {
		return err
	}
	for _, v := range versions {
		if v.Commit.State != "live" {
			continue
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		raw, err := r.readJSON(ctx, v)
		if err != nil {
			return err
		}
		err = visit(raw)
		clear(raw)
		if err != nil {
			return err
		}
	}
	return ctx.Err()
}

func (r *Repository) visibleNotes(ctx context.Context) (map[string]api.Notebook, map[string]api.Note, error) {
	books, err := r.notebooks(ctx)
	if err != nil {
		return nil, nil, err
	}
	notes := map[string]api.Note{}
	inheritedLinks := map[string]bool{}
	err = r.eachLiveJSON(ctx, "note", func(raw []byte) error {
		var note api.Note
		if json.Unmarshal(raw, &note) != nil || note.ID == "" {
			return ErrSource
		}
		if visibleNotebook(books, note.NotebookID) {
			notes[note.ID] = note
			fields, err := rawObject(raw)
			if err != nil {
				return err
			}
			_, present := fields["links"]
			inheritedLinks[note.ID] = !present
		}
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	err = r.eachLiveJSON(ctx, "note_link", func(raw []byte) error {
		var link struct {
			NoteID string `json:"note_id"`
			api.NoteLink
		}
		if json.Unmarshal(raw, &link) != nil || link.NoteID == "" {
			return ErrSource
		}
		if note, ok := notes[link.NoteID]; ok && inheritedLinks[link.NoteID] {
			note.Links = append(note.Links, link.NoteLink)
			notes[link.NoteID] = note
		}
		return nil
	})
	if err == nil {
		err = r.estadosDeNotas(ctx, notes)
	}
	return books, notes, err
}

func (r *Repository) PinnedNotes(parent context.Context) ([]api.NoteHit, error) {
	ctx, done, err := r.start(parent)
	if err != nil {
		return nil, err
	}
	defer done()
	books, notes, err := r.visibleNotes(ctx)
	if err != nil {
		return nil, err
	}
	out := []api.NoteHit{}
	for _, note := range notes {
		nb := books[note.NotebookID]
		if !note.Pinned || nb.ArchivedAt != nil || nb.Encrypted {
			continue
		}
		text := []rune(htmltext.Text(note.Content))
		out = append(out, api.NoteHit{ID: note.ID, NotebookID: note.NotebookID, Notebook: nb.Title, Title: note.Title, Snippet: string(text[:min(len(text), 200)]), Kind: note.Kind, UpdatedAt: note.UpdatedAt})
	}
	sortHits(out)
	return out[:min(len(out), 100)], ctx.Err()
}

func sortHits(out []api.NoteHit) {
	slices.SortFunc(out, func(a, b api.NoteHit) int { return cmp.Or(b.UpdatedAt.Compare(a.UpdatedAt), cmp.Compare(a.ID, b.ID)) })
}

func (r *Repository) ListTags(parent context.Context) ([]api.Tag, error) {
	ctx, done, err := r.start(parent)
	if err != nil {
		return nil, err
	}
	defer done()
	tags := map[string]api.Tag{}
	if err := r.eachLiveJSON(ctx, "tag", func(raw []byte) error {
		var tag api.Tag
		if json.Unmarshal(raw, &tag) != nil || tag.ID == "" {
			return ErrSource
		}
		tag.States = []api.TagState{}
		tags[tag.ID] = tag
		return nil
	}); err != nil {
		return nil, err
	}
	if err := r.eachLiveJSON(ctx, "tag_state", func(raw []byte) error {
		var state api.TagState
		if json.Unmarshal(raw, &state) != nil || state.ID == "" {
			return ErrSource
		}
		tag, ok := tags[state.TagID]
		if !ok {
			return ErrSource
		}
		tag.States = append(tag.States, state)
		tags[tag.ID] = tag
		return nil
	}); err != nil {
		return nil, err
	}
	out := make([]api.Tag, 0, len(tags))
	for _, tag := range tags {
		slices.SortFunc(tag.States, func(a, b api.TagState) int {
			return cmp.Or(cmp.Compare(a.Position, b.Position), cmp.Compare(a.ID, b.ID))
		})
		out = append(out, tag)
	}
	slices.SortFunc(out, func(a, b api.Tag) int { return cmp.Or(cmp.Compare(a.Position, b.Position), cmp.Compare(a.ID, b.ID)) })
	return out, ctx.Err()
}

func (r *Repository) LinkedNotes(parent context.Context, kind, id string, limit int) ([]api.Note, error) {
	if !model.ValidTargetKind(kind) || strings.TrimSpace(id) == "" || utf8.RuneCountInString(id) > 400 {
		return nil, ErrSource
	}
	ctx, done, err := r.start(parent)
	if err != nil {
		return nil, err
	}
	defer done()
	_, notes, err := r.visibleNotes(ctx)
	if err != nil {
		return nil, err
	}
	matched := map[string]bool{}
	for _, note := range notes {
		for _, link := range note.Links {
			if link.TargetKind == kind && link.TargetID == id {
				matched[note.ID] = true
			}
		}
	}
	out := []api.Note{}
	for noteID := range matched {
		if note, ok := notes[noteID]; ok {
			out = append(out, note)
		}
	}
	slices.SortFunc(out, func(a, b api.Note) int { return cmp.Or(b.UpdatedAt.Compare(a.UpdatedAt), cmp.Compare(a.ID, b.ID)) })
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	return out[:min(len(out), limit)], ctx.Err()
}

func (r *Repository) Hashtags(parent context.Context) ([]api.Hashtag, error) {
	ctx, done, err := r.start(parent)
	if err != nil {
		return nil, err
	}
	defer done()
	books, notes, err := r.visibleNotes(ctx)
	if err != nil {
		return nil, err
	}
	counts := map[string]api.Hashtag{}
	// Stable order also makes the displayed spelling deterministic across devices.
	ordered := make([]api.Note, 0, len(notes))
	for _, note := range notes {
		ordered = append(ordered, note)
	}
	slices.SortFunc(ordered, func(a, b api.Note) int { return cmp.Or(b.UpdatedAt.Compare(a.UpdatedAt), cmp.Compare(a.ID, b.ID)) })
	for _, note := range ordered {
		nb := books[note.NotebookID]
		if nb.ArchivedAt != nil || nb.Encrypted || !noteIndexable(nb, note) {
			continue
		}
		seen := map[string]bool{}
		for _, tag := range model.Hashtags(note.Title + "\n" + htmltext.Text(note.Content)) {
			key := buscar.Plegar(tag)
			if seen[key] {
				continue
			}
			seen[key] = true
			value := counts[key]
			if value.Count == 0 {
				value.Tag = tag
			}
			value.Count++
			counts[key] = value
		}
	}
	out := make([]api.Hashtag, 0, len(counts))
	for _, value := range counts {
		out = append(out, value)
	}
	slices.SortFunc(out, func(a, b api.Hashtag) int {
		return cmp.Or(cmp.Compare(b.Count, a.Count), cmp.Compare(buscar.Plegar(a.Tag), buscar.Plegar(b.Tag)))
	})
	return out[:min(len(out), 200)], ctx.Err()
}

func noteIndexable(nb api.Notebook, note api.Note) bool {
	if note.Indexable != nil {
		return *note.Indexable
	}
	return nb.Indexable
}

func (r *Repository) ListReminders(parent context.Context) ([]api.NoteReminder, error) {
	ctx, done, err := r.start(parent)
	if err != nil {
		return nil, err
	}
	defer done()
	_, notes, err := r.visibleNotes(ctx)
	if err != nil {
		return nil, err
	}
	out := []api.NoteReminder{}
	err = r.eachLiveJSON(ctx, "reminder", func(raw []byte) error {
		var reminder struct {
			api.NoteReminder
			FiredAt *time.Time `json:"fired_at"`
			Owner   string     `json:"owner_sub"`
		}
		if json.Unmarshal(raw, &reminder) != nil || reminder.NoteID == "" || reminder.Owner != r.cfg.Scope.Subject {
			return ErrSource
		}
		note, ok := notes[reminder.NoteID]
		if !ok || reminder.FiredAt != nil && (note.Kind != "todo" || note.Checked || !reminder.Repite()) {
			return nil
		}
		reminder.NotebookID = note.NotebookID
		out = append(out, reminder.NoteReminder)
		return nil
	})
	if err != nil {
		return nil, err
	}
	slices.SortFunc(out, func(a, b api.NoteReminder) int {
		return cmp.Or(a.DueAt.Compare(b.DueAt), cmp.Compare(a.NoteID, b.NoteID))
	})
	return out[:min(len(out), 500)], ctx.Err()
}
