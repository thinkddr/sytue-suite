// SPDX-License-Identifier: Apache-2.0
package mail

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	netmail "net/mail"
	"slices"
	"strings"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/eml"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

func identityAddress(value string) bool {
	parsed, err := netmail.ParseAddress(value)
	return err == nil && parsed.Address == value && !strings.ContainsAny(value, "\x00\r\n")
}

// Aliases reads only the authenticated historical account and alias originals.
// It never infers the mailbox address from an OIDC login or a session label.
func (r *Repository) Aliases(parent context.Context) (string, []api.MailAlias, error) {
	var primary string
	var aliases []api.MailAlias
	err := r.run(parent, true, func(ctx context.Context, s storage.AtomicCatalogSnapshot) error {
		var err error
		primary, aliases, err = r.aliases(ctx, s)
		return err
	})
	return primary, aliases, err
}

func (r *Repository) aliases(ctx context.Context, s storage.AtomicCatalogSnapshot) (string, []api.MailAlias, error) {
	if value, found, err := r.routingProjection(ctx, s); err != nil || found {
		return value.Primary, value.Aliases, err
	}
	receipt, err := r.loadImportReceipt(ctx, s)
	if err != nil {
		return "", nil, err
	}
	var primary string
	var aliases []api.MailAlias
	seen := map[string]bool{}
	for _, item := range receipt.Sources {
		if item.Source.Kind != "account" && item.Source.Kind != "alias" {
			continue
		}
		raw, err := r.verifyArchive(ctx, s, item.Source, item.Archive)
		if err != nil {
			return "", nil, err
		}
		var value api.MailAlias
		err = json.Unmarshal(raw, &value)
		clear(raw)
		if err != nil || value.ID != item.Source.ID || !identityAddress(value.Address) || seen[strings.ToLower(value.Address)] {
			return "", nil, ErrRecord
		}
		seen[strings.ToLower(value.Address)] = true
		if item.Source.Kind == "account" {
			if primary != "" {
				return "", nil, ErrRecord
			}
			primary = value.Address
		} else {
			aliases = append(aliases, value)
		}
	}
	if primary == "" {
		return "", nil, ErrNotReady
	}
	slices.SortFunc(aliases, func(a, b api.MailAlias) int { return strings.Compare(a.ID, b.ID) })
	return primary, aliases, nil
}

func (r *Repository) roleMailbox(ctx context.Context, s storage.AtomicCatalogSnapshot, role string) (string, error) {
	values, err := r.list(ctx, s, "mailbox")
	if err != nil {
		return "", err
	}
	defer clearDocuments(values)
	var id string
	for _, raw := range values {
		var box api.Mailbox
		if json.Unmarshal(raw, &box) != nil {
			return "", ErrRecord
		}
		if box.Role == role {
			if id != "" {
				return "", ErrConflict
			}
			id = box.ID
		}
	}
	if id == "" {
		return "", ErrNotReady
	}
	return id, nil
}

// SaveDraft retains exact composed MIME through the same SDK/WAL path as ingress.
// A private edit keeps its object id; a new revision replaces its visible content.
func (r *Repository) SaveDraft(parent context.Context, id, from string, raw []byte) (api.MailMessage, error) {
	if int64(len(raw)) > r.messageLimit() {
		return api.MailMessage{}, api.ErrMailTooLarge
	}
	if len(raw) == 0 || id != "" && !storage.Identifier(id) {
		return api.MailMessage{}, ErrRecord
	}
	raw = bytes.Clone(raw)
	defer clear(raw)
	var result api.MailMessage
	err := r.run(parent, true, func(ctx context.Context, s storage.AtomicCatalogSnapshot) error {
		work, err := r.prepareDraftFromRaw(ctx, s, id, from, raw)
		if err != nil {
			return err
		}
		defer clear(work.envelope)
		result = work.message
		if work.retry && work.operation == "" {
			return nil
		}
		return r.storeMessage(ctx, s, work.operation, ingestMessage{ID: result.ID, MailboxID: result.MailboxID, Size: int64(len(raw))}, work.envelope, bytes.NewReader(raw), int64(len(raw)))
	})
	return result, err
}

// prepareDraftFromRaw shares sender authorization, MIME parsing and projection
// between ordinary saves and the durable editor's immutable revisions.
func (r *Repository) prepareDraftFromRaw(ctx context.Context, s storage.AtomicCatalogSnapshot, id, from string, raw []byte) (draftWrite, error) {
	primary, aliases, err := r.aliases(ctx, s)
	if err != nil {
		return draftWrite{}, err
	}
	if from == "" {
		from = primary
	}
	if !strings.EqualFold(from, primary) && !slices.ContainsFunc(aliases, func(a api.MailAlias) bool { return strings.EqualFold(a.Address, from) }) {
		return draftWrite{}, api.ErrPrivateMailOperation
	}
	parsed, err := eml.Leer(bytes.NewReader(raw)) //nolint:contextcheck // eml.Leer parses in-memory bytes; there is no I/O to cancel
	defer func() {
		for _, part := range parsed.Adjuntos {
			clear(part.Datos)
		}
	}()
	if err != nil || !strings.EqualFold(parsed.De, from) {
		return draftWrite{}, ErrRecord
	}
	box, err := r.roleMailbox(ctx, s, "drafts")
	if err != nil {
		return draftWrite{}, err
	}
	return r.prepareDraft(ctx, s, id, box, parsed, raw)
}

type draftWrite struct {
	message   api.MailMessage
	envelope  []byte
	operation string
	retry     bool
}

func draftMessage(id, box string, parsed eml.Mensaje, size int64) api.MailMessage {
	preview := []rune(strings.Join(strings.Fields(parsed.Texto), " "))
	return api.MailMessage{
		ID: id, MailboxID: box, ThreadID: id, MessageID: parsed.ID,
		InReplyTo: strings.Trim(parsed.EnRespuestaA, "<>"), Subject: parsed.Asunto,
		From: parsed.De, FromName: parsed.DeNombre, To: parsed.Para,
		Cc: parsed.Cc, Bcc: parsed.Cco, ReplyTo: parsed.ResponderA,
		Preview: string(preview[:min(280, len(preview))]), Size: size,
		Keywords: []string{`\Draft`, api.MailFlagSeen}, HasAttachment: len(parsed.Adjuntos) > 0,
		ReceivedAt: parsed.Fecha.UTC(), Date: parsed.Fecha.UTC(),
	}
}

func (r *Repository) prepareDraft(ctx context.Context, s storage.AtomicCatalogSnapshot, id, box string, parsed eml.Mensaje, raw []byte) (draftWrite, error) {
	if id == "" {
		id = "draft_" + digest(raw)
	}
	work := draftWrite{message: draftMessage(id, box, parsed, int64(len(raw)))}
	existing, err := r.document(ctx, s, "message", id)
	switch {
	case err == nil:
		defer clear(existing)
		if err := r.updateDraft(ctx, s, &work, existing, raw); err != nil {
			return draftWrite{}, err
		}
	case errors.Is(err, ErrNotFound):
		work.message.ThreadID, err = r.draftThread(ctx, s, id, parsed)
		if err != nil {
			return draftWrite{}, err
		}
		work.envelope, err = draftEnvelope(work.message)
		if err != nil {
			return draftWrite{}, err
		}
	default:
		return draftWrite{}, err
	}
	if work.retry {
		return work, nil
	}
	parents, err := versionReferences(s, objectID("message", id))
	if err != nil {
		clear(work.envelope)
		return draftWrite{}, err
	}
	encoded, _ := json.Marshal(parents)
	work.operation = "draft_" + digest(append(append([]byte(id+"\x00"+digest(raw)+"\x00"), encoded...), work.envelope...))
	return work, nil
}

func (r *Repository) draftThread(ctx context.Context, s storage.AtomicCatalogSnapshot, fallback string, parsed eml.Mensaje) (string, error) {
	references := map[string]bool{}
	for _, value := range append(slices.Clone(parsed.Referencias), parsed.EnRespuestaA) {
		if id := strings.Trim(strings.TrimSpace(value), "<>"); id != "" {
			references[id] = true
		}
	}
	if len(references) == 0 {
		return fallback, nil
	}
	messages, err := r.messages(ctx, s)
	if err != nil {
		return "", err
	}
	var thread string
	for _, message := range messages {
		if !references[strings.Trim(strings.TrimSpace(message.MessageID), "<>")] {
			continue
		}
		if thread != "" && thread != message.ThreadID {
			return "", ErrConflict
		}
		thread = message.ThreadID
	}
	if thread == "" {
		return fallback, nil
	}
	return thread, nil
}

func (r *Repository) updateDraft(ctx context.Context, s storage.AtomicCatalogSnapshot, work *draftWrite, existing, raw []byte) error {
	previous, doc, err := decodeMessage(existing, work.message.ID)
	defer clear(doc.Envelope)
	if err != nil {
		return err
	}
	if previous.MailboxID != work.message.MailboxID || !previous.Has(`\Draft`) {
		return api.ErrPrivateMailOperation
	}
	work.message.ThreadID, work.message.ReceivedAt = previous.ThreadID, previous.ReceivedAt
	fields, err := draftEnvelope(work.message)
	if err != nil {
		return err
	}
	defer clear(fields)
	work.envelope, err = mergeDraftEnvelope(doc.Envelope, fields)
	if err != nil {
		return err
	}
	if !bytes.Equal(doc.Envelope, work.envelope) {
		return nil
	}
	equal, err := r.sameDraftMIME(ctx, s, doc, digest(raw), work.message.ID)
	if err != nil {
		clear(work.envelope)
		return err
	}
	if equal {
		// Keeping this operation drains a final accepted ACK from the retained WAL.
		work.operation, work.retry = doc.Operation, true
	}
	return nil
}

func mergeDraftEnvelope(original, fields []byte) ([]byte, error) {
	var updates map[string]json.RawMessage
	if json.Unmarshal(fields, &updates) != nil {
		return nil, ErrRecord
	}
	envelope := bytes.Clone(original)
	for name, value := range updates {
		next, err := patchField(envelope, name, value)
		clear(envelope)
		if err != nil {
			return nil, err
		}
		envelope = next
	}
	return envelope, nil
}

func (r *Repository) sameDraftMIME(ctx context.Context, s storage.AtomicCatalogSnapshot, doc MessageDocument, checksum, id string) (bool, error) {
	body, err := r.readMIMEVersion(s, doc.MIME, id)
	if err != nil {
		return false, err
	}
	pointer := body.Commit.Content
	if pointer == nil || pointer.Kind != "file" {
		return false, ErrRecord
	}
	_, sum, err := r.cfg.Files.Commitment(ctx, pointer.Reference, body.ObjectID, pointer.RevisionID)
	return sum == checksum, err
}

// Include empty indexed fields so clearing recipients or subject also replaces
// their prior values. Unknown extension fields remain in the original envelope.
func draftEnvelope(message api.MailMessage) ([]byte, error) {
	raw, err := json.Marshal(message)
	if err != nil {
		return nil, err
	}
	defer clear(raw)
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil {
		return nil, ErrRecord
	}
	for _, field := range []string{"thread_count", "in_reply_to", "from_name", "cc", "bcc", "reply_to", "preview", "keywords"} {
		if _, ok := fields[field]; !ok {
			fields[field] = json.RawMessage("null")
		}
	}
	return json.Marshal(fields)
}
