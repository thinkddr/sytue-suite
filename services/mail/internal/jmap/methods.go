// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package jmap

import (
	"encoding/json"
	"net/http"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/services/mail/internal/domain"
)

// rawArgs son los argumentos de un método JMAP sin tipar (map[string]any tras el
// decode del sobre). toRawArgs los normaliza a JSON crudo para deserializar en
// structs tipados por método.
type rawArgs []byte

func toRawArgs(args any) rawArgs {
	if args == nil {
		return nil
	}
	b, err := json.Marshal(args)
	if err != nil {
		return nil
	}
	return b
}

func (a rawArgs) into(dst any) {
	if len(a) == 0 {
		return
	}
	_ = json.Unmarshal(a, dst)
}

// --- Mailbox/get (RFC 8621 §2.3) ---

// LOS NOMBRES DE JMAP SON LOS DEL RFC, NO LOS DE NUESTRA API REST.
//
// Esto devolvía `boxes` tal cual, o sea `domain.Mailbox` con sus etiquetas JSON:
// `total_emails` y `unread_emails`. El RFC 8621 §2.3 los llama `totalEmails` y
// `unreadEmails`, que es lo que lee cualquier cliente JMAP —el nuestro incluido—.
//
// El resultado no se parecía a un fallo: el cliente hace `m.totalEmails ?? 0`, así
// que en vez de un hueco enseñaba un CERO creíble. Toda la barra lateral de Sytue
// Mail mostraba 0 mensajes y ningún no leído, con la bandeja llena. Medido el
// 2026-09-11: la cuenta de operaciones tenía 7 mensajes vivos en Enviados y la
// interfaz decía 0; en la base de datos, otra cuenta tenía 15 en Recibidos.
//
// La API REST (`GET /api/v1/mailboxes`) se queda en snake_case, que es lo coherente
// con el resto de sus respuestas: quien decide la forma de JMAP es el handler de
// JMAP, no el modelo.
func (a *API) mailboxGet(r *http.Request, acc domain.Account) (any, error) {
	boxes, err := a.store.ListMailboxes(r.Context(), acc.ID)
	if err != nil {
		return nil, err
	}
	list := make([]map[string]any, 0, len(boxes))
	for _, b := range boxes {
		m := map[string]any{
			"id":            b.ID.String(),
			"name":          b.Name,
			"totalEmails":   b.TotalCount,
			"unreadEmails":  b.UnreadCount,
			"totalThreads":  b.TotalCount,
			"unreadThreads": b.UnreadCount,
			"parentId":      nil,
		}
		if b.Role != "" {
			m["role"] = string(b.Role)
		}
		if b.ParentID != nil {
			m["parentId"] = b.ParentID.String()
		}
		list = append(list, m)
	}
	return map[string]any{
		"accountId": acc.ID.String(),
		"state":     "0",
		"list":      list,
		"notFound":  []string{},
	}, nil
}

// --- Mailbox/set (RFC 8621 §2.5): create/update/destroy de carpetas ---

type mailboxSetArgs struct {
	Create  map[string]mailboxCreate `json:"create"`
	Update  map[string]mailboxUpdate `json:"update"`
	Destroy []string                 `json:"destroy"`
}

type mailboxCreate struct {
	Name     string  `json:"name"`
	ParentID *string `json:"parentId"`
}

type mailboxUpdate struct {
	Name *string `json:"name"`
}

func (a *API) mailboxSet(r *http.Request, acc domain.Account, raw rawArgs) (any, error) {
	var args mailboxSetArgs
	raw.into(&args)

	created := map[string]any{}
	notCreated := map[string]any{}
	updated := map[string]any{}
	notUpdated := map[string]any{}
	destroyed := []string{}
	notDestroyed := map[string]any{}

	for cid, c := range args.Create {
		var parent *uuid.UUID
		if c.ParentID != nil {
			if id, err := uuid.Parse(*c.ParentID); err == nil {
				parent = &id
			}
		}
		mb, err := a.store.CreateMailbox(r.Context(), acc.ID, parent, c.Name)
		if err != nil {
			notCreated[cid] = setError(err)
			continue
		}
		created[cid] = map[string]any{"id": mb.ID.String()}
	}

	for id, u := range args.Update {
		mid, err := uuid.Parse(id)
		if err != nil {
			notUpdated[id] = map[string]any{"type": "notFound"}
			continue
		}
		if u.Name != nil {
			if err := a.store.RenameMailbox(r.Context(), acc.ID, mid, *u.Name); err != nil {
				notUpdated[id] = setError(err)
				continue
			}
		}
		updated[id] = nil
	}

	for _, id := range args.Destroy {
		mid, err := uuid.Parse(id)
		if err != nil {
			notDestroyed[id] = map[string]any{"type": "notFound"}
			continue
		}
		if err := a.store.DeleteMailbox(r.Context(), acc.ID, mid); err != nil {
			notDestroyed[id] = setError(err)
			continue
		}
		destroyed = append(destroyed, id)
	}

	return map[string]any{
		"accountId":    acc.ID.String(),
		"oldState":     "0",
		"newState":     "0",
		"created":      created,
		"notCreated":   notCreated,
		"updated":      updated,
		"notUpdated":   notUpdated,
		"destroyed":    destroyed,
		"notDestroyed": notDestroyed,
	}, nil
}

// --- Email/get (RFC 8621 §4.2) ---

type emailGetArgs struct {
	IDs []string `json:"ids"`
}

func (a *API) emailGet(r *http.Request, acc domain.Account, raw rawArgs) (any, error) {
	var args emailGetArgs
	raw.into(&args)

	ids := make([]uuid.UUID, 0, len(args.IDs))
	notFound := []string{}
	for _, s := range args.IDs {
		if id, err := uuid.Parse(s); err == nil {
			ids = append(ids, id)
		} else {
			notFound = append(notFound, s)
		}
	}
	msgs, err := a.store.GetMessages(r.Context(), ids)
	if err != nil {
		return nil, err
	}
	// Gate de lectura por mensaje: dueño o grantee con `read` (compartido/
	// delegado). Un id ajeno sin ACL se reporta como notFound (anti-enumeración),
	// igual que getMessage/getMessageRaw.
	visible := msgs[:0]
	for _, m := range msgs {
		switch err := a.requireMessageRead(r, acc.ID, m); {
		case err == nil:
			visible = append(visible, m)
		case isDomain(err, domain.ErrNotFound):
			// ajeno: se omite y cae en notFound
		default:
			return nil, err
		}
	}
	msgs = visible
	found := make(map[uuid.UUID]bool, len(msgs))
	for _, m := range msgs {
		found[m.ID] = true
	}
	for _, id := range ids {
		if !found[id] {
			notFound = append(notFound, id.String())
		}
	}
	return map[string]any{
		"accountId": acc.ID.String(),
		"state":     "0",
		"list":      msgs,
		"notFound":  notFound,
	}, nil
}

// --- Email/query (RFC 8621 §4.4): búsqueda + paginación ---

func (a *API) emailQuery(r *http.Request, acc domain.Account, raw rawArgs) (any, error) {
	filter := parseEmailQueryFilter(raw)
	msgs, err := a.store.SearchMessages(r.Context(), acc.ID, filter)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(msgs))
	for _, m := range msgs {
		ids = append(ids, m.ID.String())
	}
	return map[string]any{
		"accountId":           acc.ID.String(),
		"queryState":          "0",
		"canCalculateChanges": false,
		"position":            filter.Offset,
		"ids":                 ids,
	}, nil
}

// --- Email/set (RFC 8621 §4.6): update (flags/mover) y destroy ---
//
// Los cambios se expresan como patch JMAP: keywords completos ("keywords") o
// parciales ("keywords/$seen": true), y el movimiento de carpeta con "mailboxIds"
// (completo) o "mailboxIds/<id>": true (parcial). El envío de correo se hace por
// el endpoint REST /send o por SMTP submission (no por Email/set en esta versión).

func (a *API) emailSet(r *http.Request, acc domain.Account, raw rawArgs) (any, error) {
	// Deserializar de forma laxa para soportar los patches con "/" en la clave.
	var rawSet struct {
		Update  map[string]map[string]json.RawMessage `json:"update"`
		Destroy []string                              `json:"destroy"`
	}
	raw.into(&rawSet)

	updated := map[string]any{}
	notUpdated := map[string]any{}
	destroyed := []string{}
	notDestroyed := map[string]any{}

	for id, patch := range rawSet.Update {
		mid, err := uuid.Parse(id)
		if err != nil {
			notUpdated[id] = map[string]any{"type": "notFound"}
			continue
		}
		if err := a.applyEmailPatch(r, acc.ID, mid, patch); err != nil {
			notUpdated[id] = setError(err)
			continue
		}
		updated[id] = nil
	}

	for _, id := range rawSet.Destroy {
		mid, err := uuid.Parse(id)
		if err != nil {
			notDestroyed[id] = map[string]any{"type": "notFound"}
			continue
		}
		if err := a.store.DeleteMessage(r.Context(), acc.ID, mid); err != nil {
			notDestroyed[id] = setError(err)
			continue
		}
		destroyed = append(destroyed, id)
	}

	return map[string]any{
		"accountId":    acc.ID.String(),
		"oldState":     "0",
		"newState":     "0",
		"updated":      updated,
		"notUpdated":   notUpdated,
		"destroyed":    destroyed,
		"notDestroyed": notDestroyed,
	}, nil
}

// applyEmailPatch aplica un patch JMAP de Email/set a un mensaje: keywords
// (flags) tanto completos como parciales, y mover de mailbox (mailboxIds).
// accountID es la cuenta del llamante: el store solo toca mensajes/buzones suyos.
//
//nolint:gocognit // aplicación de PatchObject JMAP (RFC 8620 §5.3): un case por propiedad parcheable.
func (a *API) applyEmailPatch(r *http.Request, accountID, id uuid.UUID, patch map[string]json.RawMessage) error {
	for key, val := range patch {
		switch {
		case key == "keywords":
			var kw map[string]bool
			if err := json.Unmarshal(val, &kw); err != nil {
				return err
			}
			for k, on := range kw {
				if err := a.store.SetFlags(r.Context(), accountID, id, keywordToFlag(k), on); err != nil {
					return err
				}
			}
		case len(key) > len("keywords/") && key[:len("keywords/")] == "keywords/":
			var on bool
			_ = json.Unmarshal(val, &on)
			if err := a.store.SetFlags(r.Context(), accountID, id, keywordToFlag(key[len("keywords/"):]), on); err != nil {
				return err
			}
		case key == "mailboxIds":
			var ids map[string]bool
			if err := json.Unmarshal(val, &ids); err != nil {
				return err
			}
			for mbID, on := range ids {
				if !on {
					continue
				}
				dest, err := uuid.Parse(mbID)
				if err != nil {
					return domain.ErrNotFound
				}
				if err := a.store.MoveMessage(r.Context(), accountID, id, dest); err != nil {
					return err
				}
			}
		case len(key) > len("mailboxIds/") && key[:len("mailboxIds/")] == "mailboxIds/":
			var on bool
			_ = json.Unmarshal(val, &on)
			if !on {
				continue
			}
			dest, err := uuid.Parse(key[len("mailboxIds/"):])
			if err != nil {
				return domain.ErrNotFound
			}
			if err := a.store.MoveMessage(r.Context(), accountID, id, dest); err != nil {
				return err
			}
		}
	}
	return nil
}

// setError traduce un error de dominio a un SetError JMAP (RFC 8620 §5.3).
func setError(err error) map[string]any {
	switch {
	case isDomain(err, domain.ErrNotFound):
		return map[string]any{"type": "notFound"}
	case isDomain(err, domain.ErrMailboxExists):
		return map[string]any{"type": "alreadyExists"}
	case isDomain(err, domain.ErrMailboxNotEmpty):
		return map[string]any{"type": "mailboxHasEmail", "description": "El buzón no está vacío."}
	case isDomain(err, domain.ErrSpecialMailbox):
		return map[string]any{"type": "forbidden", "description": "Buzón de sistema no modificable."}
	case isDomain(err, domain.ErrInvalidAddress):
		return map[string]any{"type": "invalidProperties"}
	default:
		return map[string]any{"type": "serverFail", "description": err.Error()}
	}
}

// keywordToFlag traduce un keyword JMAP ($seen, $flagged, ...) al flag IMAP
// equivalente (\Seen, ...). Los keywords no estándar se pasan tal cual.
func keywordToFlag(kw string) string {
	switch kw {
	case "$seen":
		return domain.FlagSeen
	case "$flagged":
		return domain.FlagFlagged
	case "$draft":
		return domain.FlagDraft
	case "$answered":
		return domain.FlagAnswered
	case "$deleted":
		return domain.FlagDeleted
	default:
		return kw
	}
}
