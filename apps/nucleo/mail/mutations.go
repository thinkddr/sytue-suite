// SPDX-License-Identifier: Apache-2.0
package mail

import (
	"bytes"
	"context"
	"encoding/json"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

func validName(value string) bool {
	return utf8.ValidString(value) && strings.TrimSpace(value) != "" && len(value) <= 255 && !strings.ContainsAny(value, "\x00\r\n")
}

func (r *Repository) CreateMailbox(parent context.Context, name string) (api.Mailbox, error) {
	value := api.Mailbox{ID: uuid.NewString(), Name: name}
	if !validName(name) {
		return api.Mailbox{}, ErrRecord
	}
	err := r.run(parent, true, func(ctx context.Context, s storage.AtomicCatalogSnapshot) error {
		values, err := r.list(ctx, s, "mailbox")
		if err != nil {
			return err
		}
		defer clearDocuments(values)
		for _, data := range values {
			var box api.Mailbox
			if json.Unmarshal(data, &box) != nil {
				return ErrRecord
			}
			if strings.EqualFold(box.Name, name) {
				return ErrConflict
			}
		}
		payload, _ := json.Marshal(value)
		defer clear(payload)
		return r.writeDocument(ctx, s, "mailbox", value.ID, payload, "live")
	})
	return value, err
}

func patchField(raw []byte, name string, value any) ([]byte, error) {
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil {
		return nil, ErrRecord
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	fields[name] = encoded
	return json.Marshal(fields)
}

func (r *Repository) RenameMailbox(parent context.Context, id, name string) (api.MailboxSetError, error) {
	if !validName(name) {
		return "invalidProperties", nil
	}
	var refusal api.MailboxSetError
	err := r.run(parent, true, func(ctx context.Context, s storage.AtomicCatalogSnapshot) error {
		raw, err := r.document(ctx, s, "mailbox", id)
		if err != nil {
			return err
		}
		defer clear(raw)
		var box api.Mailbox
		if json.Unmarshal(raw, &box) != nil || box.ID != id {
			return ErrRecord
		}
		if box.Role != "" {
			refusal = "forbidden"
			return nil
		}
		if box.Name == name {
			return nil
		}
		changed, err := patchField(raw, "name", name)
		if err != nil {
			return err
		}
		defer clear(changed)
		return r.writeDocument(ctx, s, "mailbox", id, changed, "live")
	})
	return refusal, err
}

func (r *Repository) DeleteMailbox(parent context.Context, id string) (api.MailboxSetError, error) {
	var refusal api.MailboxSetError
	err := r.run(parent, true, func(ctx context.Context, s storage.AtomicCatalogSnapshot) error {
		raw, err := r.document(ctx, s, "mailbox", id)
		if err != nil {
			return err
		}
		defer clear(raw)
		var box api.Mailbox
		if json.Unmarshal(raw, &box) != nil || box.ID != id {
			return ErrRecord
		}
		if box.Role != "" {
			refusal = "forbidden"
			return nil
		}
		messages, err := r.messages(ctx, s)
		if err != nil {
			return err
		}
		for _, message := range messages {
			if message.MailboxID == id {
				refusal = "mailboxHasEmail"
				return nil
			}
		}
		// PostgreSQL cascades rules targeting a deleted mailbox. The private
		// multi-object transaction is not integrated yet, so never leave a live
		// rule pointing at a deleted folder or silently drop that rule.
		filters, err := r.list(ctx, s, "filter")
		if err != nil {
			return err
		}
		defer clearDocuments(filters)
		for _, raw := range filters {
			var filter api.MailFilter
			if json.Unmarshal(raw, &filter) != nil {
				return ErrRecord
			}
			if filter.MoveTo == id {
				return api.ErrPrivateMailOperation
			}
		}
		return r.writeDocument(ctx, s, "mailbox", id, []byte(`null`), "deleted")
	})
	return refusal, err
}

func (r *Repository) changeMessage(parent context.Context, ids []string, neededType, neededID string, change func(api.MailMessage) (api.MailMessage, error)) error {
	// The shared multi-object transaction protocol still needs integration.
	// A requested batch is never partially applied one message at a time.
	if len(ids) != 1 {
		return api.ErrPrivateMailOperation
	}
	return r.run(parent, true, func(ctx context.Context, s storage.AtomicCatalogSnapshot) error {
		if neededType != "" {
			needed, err := r.document(ctx, s, neededType, neededID)
			clear(needed)
			if err != nil {
				return err
			}
		}
		raw, err := r.document(ctx, s, "message", ids[0])
		if err != nil {
			return err
		}
		defer clear(raw)
		before, document, err := decodeMessage(raw, ids[0])
		defer clear(document.Envelope)
		if err != nil {
			return err
		}
		after, err := change(before)
		if err != nil {
			return err
		}
		original, _ := json.Marshal(before)
		updated, _ := json.Marshal(after)
		if bytes.Equal(original, updated) {
			return nil
		}
		// Patch only the deliberately changed fields of the exact envelope.
		envelope := bytes.Clone(document.Envelope)
		defer func() { clear(envelope) }()
		for _, field := range []struct {
			name  string
			value any
		}{{"mailbox_id", after.MailboxID}, {"keywords", after.Keywords}} {
			next, err := patchField(envelope, field.name, field.value)
			if err != nil {
				return err
			}
			clear(envelope)
			envelope = next
		}
		document.Envelope = envelope
		payload, err := json.Marshal(document)
		if err != nil {
			return err
		}
		defer clear(payload)
		return r.writeDocument(ctx, s, "message", ids[0], payload, "live")
	})
}

func (r *Repository) SetFlagMany(ctx context.Context, ids []string, flag string, add bool) error {
	if flag == "" || len(flag) > 255 || strings.ContainsAny(flag, "\x00\r\n") {
		return ErrRecord
	}
	return r.changeMessage(ctx, ids, "", "", func(message api.MailMessage) (api.MailMessage, error) {
		flags := make([]string, 0, len(message.Keywords)+1)
		for _, current := range message.Keywords {
			if !strings.EqualFold(current, flag) {
				flags = append(flags, current)
			}
		}
		if add {
			flags = append(flags, flag)
		}
		message.Keywords = flags
		return message, nil
	})
}

func (r *Repository) MoveMany(ctx context.Context, ids []string, mailbox string) error {
	return r.changeMessage(ctx, ids, "mailbox", mailbox, func(message api.MailMessage) (api.MailMessage, error) {
		message.MailboxID = mailbox
		return message, nil
	})
}

func (r *Repository) Delete(parent context.Context, id string) error {
	return r.run(parent, true, func(ctx context.Context, s storage.AtomicCatalogSnapshot) error {
		raw, err := r.document(ctx, s, "message", id)
		if err != nil {
			return err
		}
		clear(raw)
		return r.writeDocument(ctx, s, "message", id, []byte(`null`), "deleted")
	})
}

func (r *Repository) SetSignature(parent context.Context, value string) error {
	if len(value) > 8<<10 {
		return ErrRecord
	}
	return r.run(parent, true, func(ctx context.Context, s storage.AtomicCatalogSnapshot) error {
		payload, _ := json.Marshal(value)
		defer clear(payload)
		return r.writeDocument(ctx, s, "signature", "signature", payload, "live")
	})
}

func (r *Repository) Changes(parent context.Context, since string) (api.MailCambios, error) {
	var out api.MailCambios
	err := r.run(parent, true, func(ctx context.Context, s storage.AtomicCatalogSnapshot) error {
		references := make([]string, 0, len(s.Heads))
		for _, version := range s.Heads {
			references = append(references, version.Reference.RoomID+"\x00"+version.Reference.EventID)
		}
		slices.Sort(references)
		raw, _ := json.Marshal(references)
		out.State = digest(append([]byte(r.cfg.Scope.Tenant+"\x00"+r.cfg.Scope.Subject+"\x00mail\x00"), raw...))
		if since == "" || since == out.State {
			return nil
		}
		values, err := r.messages(ctx, s)
		if err != nil {
			return err
		}
		out.Changed = values
		for _, version := range s.Heads {
			h, err := decodeHeader(version)
			if err != nil {
				return err
			}
			if h.Type == "message" && version.Commit.State == "deleted" {
				out.Destroyed = append(out.Destroyed, h.ID)
			}
		}
		return nil
	})
	return out, err
}

func (r *Repository) CreateLabel(parent context.Context, name, color string) (api.MailLabel, error) {
	value := api.MailLabel{ID: uuid.NewString(), Name: name, Color: color}
	if !validName(name) || color != "" && !storage.Identifier(color) {
		return api.MailLabel{}, ErrRecord
	}
	err := r.run(parent, true, func(ctx context.Context, s storage.AtomicCatalogSnapshot) error {
		values, err := r.list(ctx, s, "label")
		if err != nil {
			return err
		}
		defer clearDocuments(values)
		for _, data := range values {
			var label api.MailLabel
			if json.Unmarshal(data, &label) != nil {
				return ErrRecord
			}
			if strings.EqualFold(label.Name, name) {
				return ErrConflict
			}
		}
		payload, _ := json.Marshal(value)
		defer clear(payload)
		return r.writeDocument(ctx, s, "label", value.ID, payload, "live")
	})
	return value, err
}

func (r *Repository) UpdateLabel(parent context.Context, id, name, color string) error {
	if !validName(name) || color != "" && !storage.Identifier(color) {
		return ErrRecord
	}
	return r.run(parent, true, func(ctx context.Context, s storage.AtomicCatalogSnapshot) error {
		raw, err := r.document(ctx, s, "label", id)
		if err != nil {
			return err
		}
		defer clear(raw)
		next, err := patchField(raw, "name", name)
		if err != nil {
			return err
		}
		defer clear(next)
		payload, err := patchField(next, "color", color)
		if err != nil {
			return err
		}
		defer clear(payload)
		return r.writeDocument(ctx, s, "label", id, payload, "live")
	})
}

func (r *Repository) DeleteLabel(parent context.Context, id string) error {
	return r.run(parent, true, func(ctx context.Context, s storage.AtomicCatalogSnapshot) error {
		raw, err := r.document(ctx, s, "label", id)
		if err != nil {
			return err
		}
		clear(raw)
		messages, err := r.messages(ctx, s)
		if err != nil {
			return err
		}
		for _, message := range messages {
			if message.Has(id) {
				return api.ErrPrivateMailOperation
			}
		}
		return r.writeDocument(ctx, s, "label", id, []byte(`null`), "deleted")
	})
}

func (r *Repository) ApplyLabel(ctx context.Context, label string, ids []string, add bool) error {
	return r.changeMessage(ctx, ids, "label", label, func(message api.MailMessage) (api.MailMessage, error) {
		flags := make([]string, 0, len(message.Keywords)+1)
		for _, flag := range message.Keywords {
			if !strings.EqualFold(flag, label) {
				flags = append(flags, flag)
			}
		}
		if add {
			flags = append(flags, label)
		}
		message.Keywords = flags
		return message, nil
	})
}
