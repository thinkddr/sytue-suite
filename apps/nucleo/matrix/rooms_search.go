// SPDX-License-Identifier: Apache-2.0
package matrix

import (
	"context"
	"errors"
	"slices"
	"strings"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	roomsearch "git.sytue.eu/sytue/one/apps/nucleo/rooms/search"
)

type searchCandidate struct {
	metadata roomsearch.Metadata
	message  api.RoomsMessage
	content  MessageContent
	client   *Client
	room     string
}

func searchMessage(m roomsearch.Metadata) api.RoomsMessage {
	return api.RoomsMessage{ID: m.ID, ChannelID: m.ChannelID, ParentID: m.ParentID, AuthorSub: m.AuthorSub, AuthorName: m.AuthorName, CreatedAt: m.CreatedAt, EditedAt: m.EditedAt, MatrixEvent: m.MatrixEvent, Type: m.Type}
}

func searchDocument(m roomsearch.Metadata, message api.RoomsMessage) roomsearch.Document {
	d := m.Document()
	d.Body = message.Body
	d.HasFile = len(message.Attachments) > 0
	return d
}

// searchBatch decrypts only a bounded page without filling the persistent view
// caches with an entire conversation history. Any unreadable authorized event
// fails the operation instead of pretending that an unsearched body did not match.
func (r *RoomsService) searchBatch(ctx context.Context, rows []roomsearch.Metadata, epoch uint64) ([]searchCandidate, error) {
	if r.revision() != epoch {
		return nil, ErrIdentity
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	out := make([]searchCandidate, len(rows))
	groups := map[string][]int{}
	for i, row := range rows {
		if row.MatrixEvent == nil {
			return nil, ErrUnsupported
		}
		groups[row.ChannelID] = append(groups[row.ChannelID], i)
	}
	for channel, indexes := range groups {
		c, room, err := r.connection(ctx, channel)
		if err != nil {
			return nil, err
		}
		refs := make([]EventRef, len(indexes))
		for i, index := range indexes {
			if rows[index].MatrixEvent.RoomID != room {
				return nil, ErrIdentity
			}
			refs[i] = *rows[index].MatrixEvent
		}
		results, err := c.ReadMany(ctx, room, refs)
		if err != nil {
			return nil, err
		}
		for i, index := range indexes {
			if results[i].Err != nil {
				return nil, results[i].Err
			}
			message, content, err := decodeDecryptedMessage(searchMessage(rows[index]), c, results[i].Event)
			if err != nil {
				return nil, err
			}
			out[index] = searchCandidate{metadata: rows[index], message: message, content: content, client: c, room: room}
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if r.revision() != epoch || c.closed.Load() {
			return nil, ErrIdentity
		}
	}
	return out, nil
}

func (r *RoomsService) visibleSearchCandidates(ctx context.Context, q roomsearch.Query, candidates []searchCandidate, epoch uint64) ([]searchCandidate, error) {
	if len(candidates) == 0 {
		return nil, nil
	}
	ids := make([]roomsearch.Identity, len(candidates))
	old := make(map[roomsearch.Identity]searchCandidate, len(candidates))
	for i, candidate := range candidates {
		ids[i] = candidate.metadata.Identity()
		old[ids[i]] = candidate
	}
	rows, err := r.MessageMetadataVisible(ctx, ids)
	if err != nil {
		return nil, err
	}
	if r.revision() != epoch {
		return nil, ErrIdentity
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	out := []searchCandidate{}
	for _, row := range rows {
		candidate, ok := old[row.Identity()]
		if !ok || !q.MetadataMatches(row.Document()) {
			continue
		}
		if row.MatrixEvent == nil {
			return nil, ErrUnsupported
		}
		if candidate.metadata.MatrixEvent == nil || *candidate.metadata.MatrixEvent != *row.MatrixEvent || candidate.metadata.ChannelID != row.ChannelID || candidate.metadata.AuthorSub != row.AuthorSub {
			changed, err := r.searchBatch(ctx, []roomsearch.Metadata{row}, epoch)
			if err != nil {
				return nil, err
			}
			candidate = changed[0]
		}
		candidate.metadata = row
		candidate.message.AuthorName = row.AuthorName
		candidate.message.EditedAt = row.EditedAt
		if q.Matches(searchDocument(row, candidate.message)) {
			out = append(out, candidate)
		}
	}
	slices.SortFunc(out, func(a, b searchCandidate) int { return roomsearch.Compare(a.metadata, b.metadata) })
	if r.revision() != epoch {
		return nil, ErrIdentity
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	return out, nil
}

//nolint:gocognit // paged search re-checks identity and epoch on every page; the ordering of those fail-closed checks is the logic and is kept in one place
func (r *RoomsService) Search(ctx context.Context, raw string, limit int) ([]api.RoomsMessage, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, errors.New("rooms: search query is required")
	}
	if limit <= 0 || limit > 100 {
		limit = 30
	}
	q := roomsearch.Parse(raw)
	epoch := r.revision()
	selected := []searchCandidate{}
	initial, err := r.Client(ctx)
	if err != nil {
		return nil, err
	}
	if initial == nil {
		return nil, ErrIdentity
	}
	cursor := ""
	for {
		if err := r.checkSearchIdentity(ctx, initial, epoch); err != nil {
			return nil, err
		}
		page, err := r.MessageMetadata(ctx, cursor, 100)
		if err != nil {
			return nil, err
		}
		if err := r.checkSearchIdentity(ctx, initial, epoch); err != nil {
			return nil, err
		}
		matched, err := r.searchPageCandidates(ctx, q, page.Messages, epoch)
		if err != nil {
			return nil, err
		}
		selected = append(selected, matched...)
		if len(selected) >= limit || page.Next == "" {
			selected, err = r.visibleSearchCandidates(ctx, q, selected, epoch)
			if err != nil {
				return nil, err
			}
			if len(selected) >= limit || page.Next == "" {
				selected = selected[:min(limit, len(selected))]
				stable, err := r.verifyFinalSearchCandidates(ctx, q, selected)
				if err != nil {
					return nil, err
				}
				if !stable {
					selected = nil
					cursor = ""
					continue
				}
				if err := r.checkSearchIdentity(ctx, initial, epoch); err != nil {
					return nil, err
				}
				return r.rememberSearchCandidates(ctx, selected, initial, epoch)
			}
		}
		if page.Next == cursor {
			return nil, ErrIdentity
		}
		cursor = page.Next
	}
}

func (r *RoomsService) searchPageCandidates(ctx context.Context, q roomsearch.Query, messages []roomsearch.Metadata, epoch uint64) ([]searchCandidate, error) {
	metadata := make([]roomsearch.Metadata, 0, len(messages))
	for _, row := range messages {
		if q.MetadataMatches(row.Document()) {
			metadata = append(metadata, row)
		}
	}
	batch, err := r.searchBatch(ctx, metadata, epoch)
	if err != nil {
		return nil, err
	}
	var matched []searchCandidate
	for _, candidate := range batch {
		if q.Matches(searchDocument(candidate.metadata, candidate.message)) {
			matched = append(matched, candidate)
		}
	}
	return matched, nil
}

func (r *RoomsService) verifyFinalSearchCandidates(ctx context.Context, q roomsearch.Query, selected []searchCandidate) (bool, error) {
	if len(selected) == 0 {
		return true, nil
	}
	identities := make([]roomsearch.Identity, len(selected))
	for i, item := range selected {
		identities[i] = item.metadata.Identity()
	}
	latest, err := r.MessageMetadataVisible(ctx, identities)
	if err != nil {
		return false, err
	}
	visible := map[roomsearch.Identity]roomsearch.Metadata{}
	for _, row := range latest {
		visible[row.Identity()] = row
	}
	for _, item := range selected {
		current, ok := visible[item.metadata.Identity()]
		if !ok || current.MatrixEvent == nil || item.metadata.MatrixEvent == nil || *current.MatrixEvent != *item.metadata.MatrixEvent || current.ChannelID != item.metadata.ChannelID || current.AuthorSub != item.metadata.AuthorSub || !q.MetadataMatches(current.Document()) {
			return false, nil
		}
	}
	return true, nil
}

func (r *RoomsService) checkSearchIdentity(ctx context.Context, initial *Client, epoch uint64) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if r.revision() != epoch || initial.closed.Load() {
		return ErrIdentity
	}
	current, err := r.Client(ctx)
	if err != nil {
		return err
	}
	if current != initial {
		return ErrIdentity
	}
	return nil
}

func (r *RoomsService) rememberSearchCandidates(ctx context.Context, selected []searchCandidate, initial *Client, epoch uint64) ([]api.RoomsMessage, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.epoch != epoch || initial.closed.Load() {
		return nil, ErrIdentity
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	messages := make([]api.RoomsMessage, len(selected))
	for _, item := range selected {
		if item.client != initial {
			return nil, ErrIdentity
		}
	}
	for i, item := range selected {
		messages[i] = item.message
		for _, file := range item.content.Attachments {
			r.files[attachmentKey{initial, item.room, file.FileID}] = cachedAttachment{file: file, channel: item.message.ChannelID}
		}
		r.messages[item.message.ID] = cachedMessage{value: item.message, client: initial, room: item.room}
	}
	return messages, nil
}
