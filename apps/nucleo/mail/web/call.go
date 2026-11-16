// SPDX-License-Identifier: Apache-2.0
package web

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"slices"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/eml"
	privateMail "git.sytue.eu/sytue/one/apps/nucleo/mail"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/webport"
)

// Call is a domain operation, never a route, arbitrary method or HTTP fallback.
// Payload contains an immutable MIME original when admitting a draft/send.
type (
	Call struct {
		Action string          `json:"action"`
		Params json.RawMessage `json:"params"`
	}
	Reply struct {
		Data []byte
		Body []byte
	}
)

func parameters[T any](raw []byte) (T, error) {
	var out T
	if len(raw) == 0 {
		raw = []byte("{}")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&out) != nil || decoder.Decode(new(any)) != io.EOF {
		return out, webport.ErrRejected
	}
	return out, nil
}

// Perform keeps domain work AND its final serialization inside the original
// action gate. Pause cannot acknowledge while an admitted reply is being copied.
func (s *Session) Perform(ctx context.Context, call Call, payload []byte, consume func(Reply) error) error {
	if consume == nil {
		return webport.ErrRejected
	}
	return s.run(ctx, func(ctx context.Context, repository *privateMail.Repository) error {
		value, body, err := invoke(ctx, repository, call, payload)
		defer clear(body)
		if raw, ok := value.(json.RawMessage); ok {
			defer clear(raw)
		}
		if err != nil {
			return err
		}
		data, err := json.Marshal(value)
		if err != nil {
			return err
		}
		defer clear(data)
		if err := ctx.Err(); err != nil {
			return err
		}
		return consume(Reply{Data: data, Body: body})
	})
}

func invoke(ctx context.Context, r *privateMail.Repository, call Call, payload []byte) (any, []byte, error) {
	if call.Action != "draft.prepare" && call.Action != "outbound.prepare" && len(payload) != 0 {
		return nil, nil, webport.ErrRejected
	}
	switch call.Action {
	case "mailboxes", "aliases", "aliases.refresh", "quota", "alias.create", "alias.delete", "mailbox.create", "mailbox.rename", "mailbox.delete":
		return invokeRoutingAndMailboxes(ctx, r, call)
	case "messages", "label.messages", "search", "thread", "mime.view", "mime.attachment", "raw", "mime.parse":
		return invokeMessagesAndSearch(ctx, r, call)
	case "messages.flag", "messages.move", "message.delete", "changes", "signature", "signature.set":
		return invokeMessageMutations(ctx, r, call)
	case "labels", "label.create", "label.update", "label.delete", "label.apply", "filters", "filter.create", "filter.update", "filter.delete", "vacation", "vacation.set":
		return invokeLabelsFiltersVacation(ctx, r, call)
	case "templates", "template.save", "template.delete":
		return invokeTemplates(ctx, r, call)
	case "mime.compose":
		return invokeCompose(ctx, r, call)
	case "draft.prepare", "draft.commit", "draft.pending":
		return invokeDrafts(ctx, r, call, payload)
	default:
		return invokeOutboundAndRetained(ctx, r, call, payload)
	}
}

func invokeRoutingAndMailboxes(ctx context.Context, r *privateMail.Repository, call Call) (any, []byte, error) {
	switch call.Action {
	case "mailboxes":
		if _, err := parameters[struct{}](call.Params); err != nil {
			return nil, nil, err
		}
		value, err := r.Mailboxes(ctx)
		return value, nil, err
	case "aliases":
		if _, err := parameters[struct{}](call.Params); err != nil {
			return nil, nil, err
		}
		primary, aliases, err := r.Aliases(ctx)
		return struct {
			Primary string          `json:"primary"`
			Aliases []api.MailAlias `json:"aliases"`
		}{primary, aliases}, nil, err
	case "aliases.refresh", "quota":
		if _, err := parameters[struct{}](call.Params); err != nil {
			return nil, nil, err
		}
		if call.Action == "quota" {
			value, err := r.Quota(ctx)
			return value, nil, err
		}
		value, err := r.RefreshRouting(ctx)
		return value, nil, err
	case "alias.create":
		input, err := parameters[struct {
			Address string `json:"address"`
		}](call.Params)
		if err != nil {
			return nil, nil, err
		}
		value, err := r.CreateRoutingAlias(ctx, input.Address)
		return value, nil, err
	case "alias.delete":
		input, err := parameters[struct {
			ID string `json:"id"`
		}](call.Params)
		if err != nil {
			return nil, nil, err
		}
		return struct{}{}, nil, r.DeleteRoutingAlias(ctx, input.ID)
	case "mailbox.create":
		input, err := parameters[struct {
			Name string `json:"name"`
		}](call.Params)
		if err != nil {
			return nil, nil, err
		}
		value, err := r.CreateMailbox(ctx, input.Name)
		return value, nil, err
	default:
		input, err := parameters[struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		}](call.Params)
		if err != nil {
			return nil, nil, err
		}
		var reason api.MailboxSetError
		if call.Action == "mailbox.rename" {
			reason, err = r.RenameMailbox(ctx, input.ID, input.Name)
		} else {
			reason, err = r.DeleteMailbox(ctx, input.ID)
		}
		return struct {
			Reason api.MailboxSetError `json:"reason"`
		}{reason}, nil, err
	}
}

func invokeMessagesAndSearch(ctx context.Context, r *privateMail.Repository, call Call) (any, []byte, error) {
	switch call.Action {
	case "messages", "label.messages":
		input, err := parameters[struct {
			ID     string `json:"id"`
			Cursor string `json:"cursor"`
			Limit  int    `json:"limit"`
		}](call.Params)
		if err != nil {
			return nil, nil, err
		}
		var values []api.MailMessage
		var cursor string
		if call.Action == "messages" {
			values, cursor, err = r.Messages(ctx, input.ID, input.Cursor, input.Limit)
		} else {
			values, cursor, err = r.LabelMessages(ctx, input.ID, input.Cursor, input.Limit)
		}
		return struct {
			Messages []api.MailMessage `json:"messages"`
			Cursor   string            `json:"cursor"`
		}{values, cursor}, nil, err
	case "search":
		input, err := parameters[struct {
			Query  string `json:"query"`
			Offset int    `json:"offset"`
			Limit  int    `json:"limit"`
		}](call.Params)
		if err != nil {
			return nil, nil, err
		}
		value, err := r.Search(ctx, input.Query, input.Offset, input.Limit)
		return value, nil, err
	case "thread":
		input, err := parameters[struct {
			ID string `json:"id"`
		}](call.Params)
		if err != nil {
			return nil, nil, err
		}
		value, err := r.Thread(ctx, input.ID)
		return value, nil, err
	case "mime.view", "mime.attachment":
		input, err := parameters[struct {
			ID    string `json:"id"`
			Index int    `json:"index"`
		}](call.Params)
		if err != nil {
			return nil, nil, err
		}
		index := -1
		if call.Action == "mime.attachment" {
			if input.Index < 0 {
				return nil, nil, webport.ErrRejected
			}
			index = input.Index
		}
		return readMessageView(ctx, r, input.ID, index)
	default:
		return invokeRawOrParse(ctx, r, call)
	}
}

func invokeRawOrParse(ctx context.Context, r *privateMail.Repository, call Call) (any, []byte, error) {
	input, err := parameters[struct {
		ID string `json:"id"`
	}](call.Params)
	if err != nil {
		return nil, nil, err
	}
	reader, err := r.Raw(ctx, input.ID)
	if err != nil {
		return nil, nil, err
	}
	if call.Action == "raw" {
		body, readErr := io.ReadAll(reader)
		closeErr := reader.Close()
		if err := errors.Join(readErr, closeErr); err != nil {
			clear(body)
			return nil, nil, err
		}
		return struct{}{}, body, nil
	}
	parsed, readErr := eml.LeerConLimite(ctx, reader, r.MessageByteLimit())
	closeErr := reader.Close()
	defer func() {
		for _, part := range parsed.Adjuntos {
			clear(part.Datos)
		}
	}()
	if err := errors.Join(readErr, closeErr); err != nil {
		return nil, nil, err
	}
	// Marshal before clearing attachment buffers; no MIME parser runs on the BFF.
	raw, err := json.Marshal(parsed)
	if err != nil {
		return nil, nil, err
	}
	return json.RawMessage(raw), nil, nil
}

func invokeMessageMutations(ctx context.Context, r *privateMail.Repository, call Call) (any, []byte, error) {
	switch call.Action {
	case "messages.flag":
		input, err := parameters[struct {
			IDs  []string `json:"ids"`
			Flag string   `json:"flag"`
			Add  bool     `json:"add"`
		}](call.Params)
		if err != nil {
			return nil, nil, err
		}
		return struct{}{}, nil, r.SetFlagMany(ctx, input.IDs, input.Flag, input.Add)
	case "messages.move":
		input, err := parameters[struct {
			IDs     []string `json:"ids"`
			Mailbox string   `json:"mailbox"`
		}](call.Params)
		if err != nil {
			return nil, nil, err
		}
		return struct{}{}, nil, r.MoveMany(ctx, input.IDs, input.Mailbox)
	case "message.delete":
		input, err := parameters[struct {
			ID string `json:"id"`
		}](call.Params)
		if err != nil {
			return nil, nil, err
		}
		return struct{}{}, nil, r.Delete(ctx, input.ID)
	case "changes":
		input, err := parameters[struct {
			Since string `json:"since"`
		}](call.Params)
		if err != nil {
			return nil, nil, err
		}
		value, err := r.Changes(ctx, input.Since)
		return value, nil, err
	case "signature":
		if _, err := parameters[struct{}](call.Params); err != nil {
			return nil, nil, err
		}
		value, err := r.Signature(ctx)
		return value, nil, err
	default:
		input, err := parameters[struct {
			Value string `json:"value"`
		}](call.Params)
		if err != nil {
			return nil, nil, err
		}
		return struct{}{}, nil, r.SetSignature(ctx, input.Value)
	}
}

func invokeLabelsFiltersVacation(ctx context.Context, r *privateMail.Repository, call Call) (any, []byte, error) {
	switch call.Action {
	case "labels":
		if _, err := parameters[struct{}](call.Params); err != nil {
			return nil, nil, err
		}
		value, err := r.Labels(ctx)
		return value, nil, err
	case "label.create":
		input, err := parameters[struct {
			Name  string `json:"name"`
			Color string `json:"color"`
		}](call.Params)
		if err != nil {
			return nil, nil, err
		}
		value, err := r.CreateLabel(ctx, input.Name, input.Color)
		return value, nil, err
	case "label.update":
		input, err := parameters[struct {
			ID    string `json:"id"`
			Name  string `json:"name"`
			Color string `json:"color"`
		}](call.Params)
		if err != nil {
			return nil, nil, err
		}
		return struct{}{}, nil, r.UpdateLabel(ctx, input.ID, input.Name, input.Color)
	case "label.delete":
		input, err := parameters[struct {
			ID string `json:"id"`
		}](call.Params)
		if err != nil {
			return nil, nil, err
		}
		return struct{}{}, nil, r.DeleteLabel(ctx, input.ID)
	case "label.apply":
		input, err := parameters[struct {
			ID  string   `json:"id"`
			IDs []string `json:"ids"`
			Add bool     `json:"add"`
		}](call.Params)
		if err != nil {
			return nil, nil, err
		}
		return struct{}{}, nil, r.ApplyLabel(ctx, input.ID, input.IDs, input.Add)
	default:
		return invokeFiltersAndVacation(ctx, r, call)
	}
}

func invokeFiltersAndVacation(ctx context.Context, r *privateMail.Repository, call Call) (any, []byte, error) {
	switch call.Action {
	case "filters":
		if _, err := parameters[struct{}](call.Params); err != nil {
			return nil, nil, err
		}
		values, err := r.Filters(ctx)
		if err != nil {
			return nil, nil, err
		}
		editors := make([]filterEditor, 0, len(values))
		for _, value := range values {
			editors = append(editors, editFilter(value))
		}
		return editors, nil, nil
	case "filter.create", "filter.update":
		editor, err := parameters[filterEditor](call.Params)
		if err != nil {
			return nil, nil, err
		}
		input, err := editor.value()
		if err != nil {
			return nil, nil, err
		}
		if call.Action == "filter.create" {
			if input.PrivateRevision != "" {
				return nil, nil, webport.ErrRejected
			}
			value, err := r.CreateFilter(ctx, input)
			return editFilter(value), nil, err
		}
		return struct{}{}, nil, r.UpdateFilter(ctx, input)
	case "filter.delete":
		input, err := parameters[struct {
			ID string `json:"id"`
		}](call.Params)
		if err != nil {
			return nil, nil, err
		}
		return struct{}{}, nil, r.DeleteFilter(ctx, input.ID)
	case "vacation":
		if _, err := parameters[struct{}](call.Params); err != nil {
			return nil, nil, err
		}
		value, err := r.Vacation(ctx)
		return editVacation(value), nil, err
	default:
		editor, err := parameters[vacationEditor](call.Params)
		if err != nil {
			return nil, nil, err
		}
		input, err := editor.value()
		if err != nil {
			return nil, nil, err
		}
		return struct{}{}, nil, r.SetVacation(ctx, input)
	}
}

func invokeTemplates(ctx context.Context, r *privateMail.Repository, call Call) (any, []byte, error) {
	switch call.Action {
	case "templates":
		if _, err := parameters[struct{}](call.Params); err != nil {
			return nil, nil, err
		}
		value, err := r.Templates(ctx)
		return value, nil, err
	case "template.save":
		input, err := parameters[struct {
			ID   string `json:"id"`
			Name string `json:"name"`
			Body string `json:"body"`
		}](call.Params)
		if err != nil {
			return nil, nil, err
		}
		value, err := r.SaveTemplate(ctx, input.ID, input.Name, input.Body)
		return value, nil, err
	default:
		input, err := parameters[struct {
			ID string `json:"id"`
		}](call.Params)
		if err != nil {
			return nil, nil, err
		}
		return struct{}{}, nil, r.DeleteTemplate(ctx, input.ID)
	}
}

func invokeCompose(ctx context.Context, r *privateMail.Repository, call Call) (any, []byte, error) {
	input, err := parameters[struct {
		From        string   `json:"from"`
		FromName    string   `json:"from_name"`
		To          []string `json:"to"`
		Cc          []string `json:"cc"`
		Bcc         []string `json:"bcc"`
		Subject     string   `json:"subject"`
		Text        string   `json:"text"`
		HTML        string   `json:"html"`
		InReplyTo   string   `json:"in_reply_to"`
		References  []string `json:"references"`
		Draft       bool     `json:"draft"`
		Attachments []struct {
			Name string `json:"name"`
			Type string `json:"type"`
			Body []byte `json:"body"`
			CID  string `json:"cid"`
		} `json:"attachments"`
	}](call.Params)
	if err != nil {
		return nil, nil, err
	}
	defer func() {
		for _, attachment := range input.Attachments {
			clear(attachment.Body)
		}
	}()
	primary, aliases, err := r.Aliases(ctx)
	if err != nil {
		return nil, nil, err
	}
	if input.From == "" {
		input.From = primary
	}
	if !strings.EqualFold(primary, input.From) && !slices.ContainsFunc(aliases, func(alias api.MailAlias) bool { return strings.EqualFold(alias.Address, input.From) }) {
		return nil, nil, api.ErrPrivateMailOperation
	}
	outgoing := eml.Salida{De: input.From, DeNombre: input.FromName, Asunto: input.Subject, Texto: input.Text, HTML: input.HTML, EnRespuestaA: input.InReplyTo, Referencias: input.References, Borrador: input.Draft}
	outgoing.Para, err = eml.Direcciones(strings.Join(input.To, ","))
	if err != nil {
		return nil, nil, err
	}
	outgoing.Cc, err = eml.Direcciones(strings.Join(input.Cc, ","))
	if err != nil {
		return nil, nil, err
	}
	outgoing.Cco, err = eml.Direcciones(strings.Join(input.Bcc, ","))
	if err != nil {
		return nil, nil, err
	}
	for _, attachment := range input.Attachments {
		outgoing.Adjuntos = append(outgoing.Adjuntos, eml.Adjunto{Nombre: attachment.Name, Tipo: attachment.Type, Datos: attachment.Body, CID: attachment.CID})
	}
	body, err := eml.Componer(outgoing, time.Now())
	if err == nil && int64(len(body)) > r.MessageByteLimit() {
		clear(body)
		return nil, nil, api.ErrMailTooLarge
	}
	return struct {
		From       string   `json:"from"`
		Recipients []string `json:"recipients"`
	}{input.From, outgoing.Destinatarios()}, body, err
}

func invokeDrafts(ctx context.Context, r *privateMail.Repository, call Call, payload []byte) (any, []byte, error) {
	switch call.Action {
	case "draft.prepare":
		input, err := parameters[struct {
			Editor   string `json:"editor"`
			Revision string `json:"revision"`
			ID       string `json:"id"`
			From     string `json:"from"`
		}](call.Params)
		if err != nil {
			return nil, nil, err
		}
		value, err := r.PrepareDraftRevision(ctx, api.MailDraftRevision{Editor: input.Editor, Revision: input.Revision, ID: input.ID, From: input.From, MIME: payload})
		return value, nil, err
	case "draft.commit":
		input, err := parameters[struct {
			Attempt api.MailDraftAttempt `json:"attempt"`
		}](call.Params)
		if err != nil {
			return nil, nil, err
		}
		value, err := r.CommitDraftRevision(ctx, input.Attempt)
		return value, nil, err
	default:
		if _, err := parameters[struct{}](call.Params); err != nil {
			return nil, nil, err
		}
		value, err := r.PendingDraftRevisions(ctx)
		return value, nil, err
	}
}

func invokeOutboundAndRetained(ctx context.Context, r *privateMail.Repository, call Call, payload []byte) (any, []byte, error) {
	switch call.Action {
	case "outbound.prepare":
		input, err := parameters[struct {
			Operation  string    `json:"operation"`
			DraftID    string    `json:"draft_id"`
			From       string    `json:"from"`
			Recipients []string  `json:"recipients"`
			Consent    string    `json:"consent"`
			ReleaseAt  time.Time `json:"release_at"`
		}](call.Params)
		if err != nil {
			return nil, nil, err
		}
		value, err := r.PrepareOutbound(ctx, api.MailOutgoing{Operation: input.Operation, DraftID: input.DraftID, From: input.From, Recipients: input.Recipients, Consent: input.Consent, ReleaseAt: input.ReleaseAt, MIME: payload})
		return value, nil, err
	case "outbound.commit":
		input, err := parameters[struct {
			Attempt api.MailOutgoingAttempt `json:"attempt"`
		}](call.Params)
		if err != nil {
			return nil, nil, err
		}
		value, err := r.CommitOutbound(ctx, input.Attempt)
		return value, nil, err
	case "outbound.pending":
		if _, err := parameters[struct{}](call.Params); err != nil {
			return nil, nil, err
		}
		value, err := r.PendingOutbound(ctx)
		return value, nil, err
	case "outbound.cancel-attempt":
		input, err := parameters[struct {
			Attempt api.MailOutgoingAttempt `json:"attempt"`
		}](call.Params)
		if err != nil {
			return nil, nil, err
		}
		return struct{}{}, nil, r.CancelPrivateOutbound(ctx, input.Attempt)
	default:
		return invokeScheduledAndRetained(ctx, r, call)
	}
}

func invokeScheduledAndRetained(ctx context.Context, r *privateMail.Repository, call Call) (any, []byte, error) {
	switch call.Action {
	case "scheduled":
		if _, err := parameters[struct{}](call.Params); err != nil {
			return nil, nil, err
		}
		value, err := r.Scheduled(ctx)
		return value, nil, err
	case "scheduled.cancel":
		input, err := parameters[struct {
			ID string `json:"id"`
		}](call.Params)
		if err != nil {
			return nil, nil, err
		}
		return struct{}{}, nil, r.CancelOutbound(ctx, input.ID)
	case "scheduled.reschedule":
		input, err := parameters[struct {
			ID   string    `json:"id"`
			When time.Time `json:"when"`
		}](call.Params)
		if err != nil {
			return nil, nil, err
		}
		return struct{}{}, nil, r.Reschedule(ctx, input.ID, input.When)
	case "retained.outbound":
		if _, err := parameters[struct{}](call.Params); err != nil {
			return nil, nil, err
		}
		value, err := r.RetainedOutbounds(ctx)
		return value, nil, err
	case "retained.outbound.prepare":
		input, err := parameters[struct {
			ID      string `json:"id"`
			Consent string `json:"consent"`
		}](call.Params)
		if err != nil {
			return nil, nil, err
		}
		value, err := r.PrepareRetainedOutbound(ctx, input.ID, input.Consent)
		return value, nil, err
	case "retained.page":
		input, err := parameters[struct {
			After storage.LegacySourceCursor `json:"after"`
			Limit int                        `json:"limit"`
		}](call.Params)
		if err != nil {
			return nil, nil, err
		}
		value, err := r.RetainedSourcePage(ctx, input.After, input.Limit)
		return value, nil, err
	default:
		return nil, nil, webport.ErrRejected
	}
}
