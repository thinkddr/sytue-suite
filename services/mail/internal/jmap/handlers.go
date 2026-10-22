// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package jmap

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/mail/internal/blobs"
	"git.sytue.eu/sytue/one/services/mail/internal/domain"
	"git.sytue.eu/sytue/one/services/mail/internal/mime"
	"git.sytue.eu/sytue/one/services/mail/internal/smime"
)

// account resuelve la cuenta del usuario autenticado en el tenant del token.
//
// Y la MANTIENE al día: si Sytue ID dice que la dirección principal de esta
// persona es otra —le pasa a toda la plantilla el día que su organización
// conecta su dominio propio—, la cuenta se muda y la anterior se conserva como
// alias. Sin esto, la cuenta se quedaba con la dirección del alta para siempre y
// la nueva no existía en Sytue Mail: el MX contestaba 550 al correo dirigido al
// dominio recién conectado, mientras la consola de Sytue ID lo daba por hecho.
//
// Solo muda el claim `mailbox` —el que Sytue ID firma con la dirección que él
// asigna— y NUNCA `MailAddress()`, que cae al correo de registro cuando no hay
// buzón: eso metería el gmail de alguien en nuestro espacio de direcciones.
func (a *API) account(r *http.Request) (domain.Account, error) {
	claims, ok := authn.FromContext(r.Context())
	if !ok {
		return domain.Account{}, httpx.ErrUnauthorized
	}
	// MailAddress y no Email: la dirección del buzón es la que Sytue ID asignó en el
	// dominio de la organización, no el correo con el que el usuario se registró
	// (ver platform/pkg/authn.Claims).
	// La cuota del buzón: la del plan si es una cuenta personal (viene en el
	// token, y manda sobre la fila), la del servicio si es una organización.
	cuota, fijar := a.cfg.DefaultMailboxQuotaBytes, false
	if claims.MailboxBytes > 0 {
		cuota, fijar = claims.MailboxBytes, true
	}
	acc, err := a.store.EnsureAccountConCuota(r.Context(), claims.Subject, claims.MailAddress(), claims.Name, cuota, fijar)
	if err != nil || claims.Mailbox == "" || strings.EqualFold(claims.Mailbox, acc.Address) {
		return acc, err
	}
	return a.store.MoveAccountAddress(r.Context(), acc, claims.Mailbox)
}

// orgMailDomains son los dominios propios que Sytue ID dice que tiene la
// organización del llamante. Van en el token porque aquí no hay forma de
// deducirlos: de una dirección se sabe su dominio, no los otros que esa
// organización tiene conectados (ver domain.AliasDomainAllowed).
func orgMailDomains(r *http.Request) []string {
	claims, ok := authn.FromContext(r.Context())
	if !ok {
		return nil
	}
	return claims.MailDomains
}

func (a *API) listMailboxes(w http.ResponseWriter, r *http.Request) {
	acc, err := a.account(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	boxes, err := a.store.ListMailboxes(r.Context(), acc.ID)
	if err != nil {
		httpx.Fail(w, r, mapErr(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"mailboxes": boxes})
}

type createMailboxReq struct {
	Name     string  `json:"name"`
	ParentID *string `json:"parent_id,omitempty"`
}

func (a *API) createMailbox(w http.ResponseWriter, r *http.Request) {
	acc, err := a.account(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var body createMailboxReq
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var parent *uuid.UUID
	if body.ParentID != nil {
		id, err := uuid.Parse(*body.ParentID)
		if err != nil {
			httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid parent_id."))
			return
		}
		parent = &id
	}
	mb, err := a.store.CreateMailbox(r.Context(), acc.ID, parent, body.Name)
	if err != nil {
		httpx.Fail(w, r, mapErr(err))
		return
	}
	httpx.JSON(w, r, http.StatusCreated, mb)
}

// listMessages pagina el buzón por KEYSET: `cursor` es "received_at,id"
// (RFC 3339 + uuid) del último mensaje visto; vacío = primera página. La
// respuesta trae `next_cursor` para pedir la siguiente ("" = no hay más).
// Sustituye al antiguo `offset` (O(n) en buzones grandes), que se ignora.
func (a *API) listMessages(w http.ResponseWriter, r *http.Request) {
	mailboxID, err := uuid.Parse(chi.URLParam(r, "mailboxID"))
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid mailboxID."))
		return
	}
	// El buzón debe ser del llamante (mismo check que listSharedMessages); un
	// buzón ajeno del tenant responde 404 (anti-enumeración). La lectura
	// compartida/delegada va por /shared-mailboxes/... con su ACL.
	acc, err := a.account(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	owned, err := a.store.MailboxInAccount(r.Context(), mailboxID, acc.ID)
	if err != nil {
		httpx.Fail(w, r, mapErr(err))
		return
	}
	if !owned {
		httpx.Fail(w, r, httpx.ErrNotFound)
		return
	}
	limit := queryInt(r, 50)
	if limit <= 0 || limit > 200 {
		limit = 50 // mismo clamp que el store: len(msgs)==limit detecta página llena
	}
	before, beforeID, err := parseCursor(r.URL.Query().Get("cursor"))
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid cursor."))
		return
	}
	msgs, err := a.store.ListMessages(r.Context(), mailboxID, limit, before, beforeID)
	if err != nil {
		httpx.Fail(w, r, mapErr(err))
		return
	}
	next := ""
	if len(msgs) == limit {
		last := msgs[len(msgs)-1]
		next = formatCursor(last.ReceivedAt, last.ID)
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"messages": msgs, "limit": limit, "next_cursor": next})
}

// listThreadMessages devuelve el hilo COMPLETO, atravesando carpetas: la vista de
// conversación del cliente pliega la lista de la carpeta, así que sin esto un
// hilo abierto desde Recibidos no enseñaba lo que tú respondiste (vive en
// Enviados) ni lo que ya archivaste.
//
// Sin paginación (un hilo son decenas de mensajes; el store impone un tope duro).
// Owner-scoped en el store por account_id: un hilo ajeno sale VACÍO, no 404 ni
// 403 —no confirma su existencia—.
func (a *API) listThreadMessages(w http.ResponseWriter, r *http.Request) {
	acc, err := a.account(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "threadID"))
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid threadID."))
		return
	}
	msgs, err := a.store.ListThreadMessages(r.Context(), acc.ID, id, queryInt(r, 0))
	if err != nil {
		httpx.Fail(w, r, mapErr(err))
		return
	}
	if msgs == nil {
		msgs = []domain.Message{}
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"messages": msgs})
}

// maxBusquedaSuite acota lo que el buscador global puede pedir de una vez: pinta
// unos pocos resultados por producto y el Hub pagina sobre lo agregado.
const maxBusquedaSuite = 50

// searchMessages — GET /api/v1/messages/search?q=&limit=&position= — los correos
// del usuario que casan con `q`, para el buscador global de la suite y el de las
// apps nativas, que pagina: `position` es el desplazamiento de `Email/query`
// (cuántos saltar), así que la página siguiente es `position` + lo ya recibido.
//
// Es la MISMA búsqueda que Email/query —texto libre con sus operadores (`from:`,
// `has:attachment`…)— y por eso reutiliza su traducción del filtro en vez de
// escribir otra: dos lecturas de `from:ana` acabarían dando resultados distintos
// según desde dónde se buscara. Owner-scoped: solo la cuenta del token.
func (a *API) searchMessages(w http.ResponseWriter, r *http.Request) {
	acc, err := a.account(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		httpx.JSON(w, r, http.StatusOK, map[string]any{"messages": []domain.Message{}})
		return
	}
	limit := queryInt(r, 20)
	if limit <= 0 || limit > maxBusquedaSuite {
		limit = maxBusquedaSuite
	}
	args, err := json.Marshal(map[string]any{
		"filter": map[string]string{"text": q}, "limit": limit, "position": queryIntNamed(r, "position", 0),
	})
	if err != nil {
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	}
	msgs, err := a.store.SearchMessages(r.Context(), acc.ID, parseEmailQueryFilter(rawArgs(args)))
	if err != nil {
		httpx.Fail(w, r, mapErr(err))
		return
	}
	if msgs == nil {
		msgs = []domain.Message{}
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"messages": msgs})
}

func (a *API) getMessage(w http.ResponseWriter, r *http.Request) {
	acc, err := a.account(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "messageID"))
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid messageID."))
		return
	}
	m, err := a.store.GetMessage(r.Context(), id)
	if err != nil {
		httpx.Fail(w, r, mapErr(err))
		return
	}
	if err := a.requireMessageRead(r, acc.ID, m); err != nil {
		httpx.Fail(w, r, mapErr(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, m)
}

// getMessageRaw streamea el .eml crudo desde S3 (descarga del mensaje completo).
func (a *API) getMessageRaw(w http.ResponseWriter, r *http.Request) {
	acc, err := a.account(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "messageID"))
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid messageID."))
		return
	}
	m, err := a.store.GetMessage(r.Context(), id)
	if err != nil {
		httpx.Fail(w, r, mapErr(err))
		return
	}
	if err := a.requireMessageRead(r, acc.ID, m); err != nil {
		httpx.Fail(w, r, mapErr(err))
		return
	}
	t, err := tenancy.From(r.Context())
	if err != nil {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	blob, err := a.blobsGet(r, t, m.BlobKey)
	if err != nil {
		httpx.Fail(w, r, mapErr(err))
		return
	}
	defer blob.Close()
	w.Header().Set("Content-Type", "message/rfc822")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", m.ID.String()+".eml"))
	_, _ = io.Copy(w, blob)
}

type setFlagsReq struct {
	Flag string `json:"flag"`
	Add  bool   `json:"add"`
}

func (a *API) setFlags(w http.ResponseWriter, r *http.Request) {
	acc, err := a.account(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "messageID"))
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid messageID."))
		return
	}
	var body setFlagsReq
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if body.Flag == "" {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("flag requerido."))
		return
	}
	if err := a.store.SetFlags(r.Context(), acc.ID, id, body.Flag, body.Add); err != nil {
		httpx.Fail(w, r, mapErr(err))
		return
	}
	httpx.NoContent(w)
}

type moveReq struct {
	MailboxID string `json:"mailbox_id"`
}

func (a *API) moveMessage(w http.ResponseWriter, r *http.Request) {
	acc, err := a.account(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "messageID"))
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid messageID."))
		return
	}
	var body moveReq
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	dest, err := uuid.Parse(body.MailboxID)
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid mailbox_id."))
		return
	}
	if err := a.store.MoveMessage(r.Context(), acc.ID, id, dest); err != nil {
		httpx.Fail(w, r, mapErr(err))
		return
	}
	httpx.NoContent(w)
}

func (a *API) deleteMessage(w http.ResponseWriter, r *http.Request) {
	acc, err := a.account(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "messageID"))
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid messageID."))
		return
	}
	if err := a.store.DeleteMessage(r.Context(), acc.ID, id); err != nil {
		httpx.Fail(w, r, mapErr(err))
		return
	}
	httpx.NoContent(w)
}

type sendReq struct {
	From       string   `json:"from"`
	Recipients []string `json:"recipients"`
	// Raw es el mensaje RFC 5322 completo (base64 o texto) construido por el
	// cliente. El servidor lo firma con DKIM y lo encola.
	Raw string `json:"raw"`
	// RawBlobID es el mismo mensaje ya subido por /jmap/upload, para el que no cabe
	// en este cuerpo JSON (httpx.DecodeJSON corta en 1 MiB: un adjunto de 800 KB en
	// base64 ya no cabía). Si viene, manda sobre Raw.
	RawBlobID string `json:"raw_blob_id,omitempty"`
	// SendAt (RFC3339) programa el envío para esa hora; UndoSeconds difiere el
	// envío una ventana de gracia (deshacer). Ambos vacíos/cero = envío inmediato.
	SendAt      string `json:"send_at,omitempty"`
	UndoSeconds int    `json:"undo_seconds,omitempty"`
}

// maxUndoSeconds acota la ventana de deshacer (Gmail ofrece 5/10/20/30 s). El
// cliente elige; el servidor la limita para que un valor absurdo no retenga un
// correo indefinidamente.
const maxUndoSeconds = 30

func (a *API) sendMessage(w http.ResponseWriter, r *http.Request) {
	acc, err := a.account(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var body sendReq
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	from := body.From
	if from == "" {
		from = acc.Address
	}
	if len(body.Recipients) == 0 || (body.Raw == "" && body.RawBlobID == "") {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("recipients and raw are required."))
		return
	}
	crudo, soltar, err := a.rawDe(r, acc, body)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	defer soltar()
	// Enviar-como / enviar-en-nombre-de seguro (frontera de confianza; el cliente NO
	// forja el From). El From debe ser una identidad AUTORIZADA del usuario:
	//   - su dirección primaria, o un alias VERIFICADO suyo (sin cabecera Sender);
	//   - un buzón COMPARTIDO con permiso send (enviar-COMO: From = la compartida);
	//   - un buzón DELEGADO con permiso send (enviar-EN-NOMBRE-DE: From = el
	//     directivo, y el servidor sella Sender = el remitente real, el asistente).
	// Cualquier otra dirección ⇒ 403, sin encolar. La copia en Enviados y la firma
	// DKIM se resuelven contra la cuenta dueña del From (LookupAddress en Submit).
	// Esto autoriza el `from` del SOBRE; que la cabecera `From:` del mensaje sea de
	// esa misma cuenta lo exige delivery.Outbound (exigirFromDeLaCuenta), que es
	// por donde pasa también el SMTP.
	raw := crudo
	if !strings.EqualFold(from, acc.Address) {
		owned, err := a.store.AliasBelongsTo(r.Context(), acc.ID, from)
		if err != nil {
			httpx.Fail(w, r, mapErr(err))
			return
		}
		if !owned {
			ident, err := a.store.GrantedIdentityByAddress(r.Context(), acc.Subject, from)
			if err != nil {
				if errors.Is(err, domain.ErrNotFound) {
					httpx.Fail(w, r, httpx.ErrForbidden.WithMessage("You cannot send as that address."))
					return
				}
				httpx.Fail(w, r, mapErr(err))
				return
			}
			if !ident.Permission.AtLeast(domain.PermSend) {
				httpx.Fail(w, r, httpx.ErrForbidden.WithMessage("You do not have permission to send as that address."))
				return
			}
			// Delegación (cuenta personal de otro): sellar el remitente real en Sender.
			// Un buzón compartido (kind=shared) ES la identidad: no lleva Sender.
			if ident.Kind == domain.KindUser {
				raw = mime.EnsureSenderHeader(raw, acc.Address)
			}
		}
	}

	releaseAt, hold, err := resolveRelease(body.SendAt, body.UndoSeconds, time.Now())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if hold == domain.HoldNone {
		if err := a.outbound.SubmitRaw(r.Context(), from, body.Recipients, raw); err != nil {
			httpx.Fail(w, r, mapErr(err))
			return
		}
		httpx.JSON(w, r, http.StatusAccepted, map[string]any{"status": "queued"})
		return
	}
	id, err := a.outbound.Schedule(r.Context(), from, body.Recipients, raw, releaseAt, hold)
	if err != nil {
		httpx.Fail(w, r, mapErr(err))
		return
	}
	httpx.JSON(w, r, http.StatusAccepted, map[string]any{
		"status":     hold,
		"id":         id,
		"release_at": releaseAt.UTC().Format(time.RFC3339),
	})
}

// rawDe es el mensaje de /send y /drafts: el del cuerpo o, con `raw_blob_id`, el que
// se subió antes por /jmap/upload. El blob tiene que ser de ESTA cuenta (un id ajeno
// enviaría el correo de otra persona) y se lee con el tope de MaxMessageBytes.
// `soltar` borra el blob de subida, que ya no hace falta: el envío lo lleva en la
// cola y el borrador en su clave definitiva. Se llama también si algo falla después,
// porque el cliente lo volverá a subir al reintentar.
func (a *API) rawDe(r *http.Request, acc domain.Account, body sendReq) (raw []byte, soltar func(), err error) {
	soltar = func() {}
	if body.RawBlobID == "" {
		if body.Raw == "" {
			return nil, soltar, httpx.ErrBadRequest.WithMessage("raw es obligatorio.")
		}
		return []byte(body.Raw), soltar, nil
	}
	key, ok := blobKey(body.RawBlobID)
	if !ok || !strings.HasPrefix(key, "blobs/"+acc.ID.String()+"/") {
		return nil, soltar, httpx.ErrBadRequest.WithMessage("Invalid raw_blob_id.")
	}
	t, err := tenancy.From(r.Context())
	if err != nil {
		return nil, soltar, httpx.ErrUnauthorized
	}
	if a.blobs == nil {
		return nil, soltar, httpx.ErrInternal.WithMessage("almacenamiento no disponible.")
	}
	rc, err := a.blobs.Get(r.Context(), t, key)
	if err != nil {
		return nil, soltar, httpx.ErrNotFound.WithMessage("The uploaded message does not exist.")
	}
	defer func() { _ = rc.Close() }()
	raw, err = io.ReadAll(io.LimitReader(rc, a.cfg.MaxMessageBytes+1))
	if err != nil {
		return nil, soltar, httpx.ErrInternal.Wrap(err)
	}
	if int64(len(raw)) > a.cfg.MaxMessageBytes {
		return nil, soltar, httpx.ErrBadRequest.WithMessage("The message is too large.")
	}
	ctx := context.WithoutCancel(r.Context())
	return raw, func() { _ = a.blobs.Delete(ctx, t, key) }, nil
}

// resolveRelease decide el instante de liberación y el tipo de retención a partir
// de send_at (programado) / undo_seconds (deshacer). Prioriza send_at. Un send_at
// en el pasado o mal formado es un error de cliente. undo_seconds se acota a
// [1, maxUndoSeconds]. Sin ninguno → envío inmediato (hold vacío).
func resolveRelease(sendAt string, undoSeconds int, now time.Time) (time.Time, string, error) {
	if sendAt != "" {
		t, err := time.Parse(time.RFC3339, sendAt)
		if err != nil {
			return time.Time{}, "", httpx.ErrBadRequest.WithMessage("send_at must be an RFC3339 date.")
		}
		if !t.After(now) {
			return time.Time{}, "", httpx.ErrBadRequest.WithMessage("send_at must be in the future.")
		}
		return t.UTC(), domain.HoldScheduled, nil
	}
	if undoSeconds > 0 {
		if undoSeconds > maxUndoSeconds {
			undoSeconds = maxUndoSeconds
		}
		return now.Add(time.Duration(undoSeconds) * time.Second).UTC(), domain.HoldUndo, nil
	}
	return time.Time{}, domain.HoldNone, nil
}

// --- Borradores (Drafts) ---
//
// Un borrador es un mensaje en la carpeta role=drafts con el keyword \Draft. Se
// reutiliza la misma composición RFC 5322 del cliente que /send (campo raw), pero
// en lugar de firmar+encolar el envío se guarda el .eml en S3 y su sobre en PG
// (store.InsertMessage), igual que la copia en Sent de un saliente. Listar los
// borradores = listar el buzón Drafts (GET /mailboxes/{id}/messages).

// createDraft guarda un nuevo borrador en la carpeta Drafts.
func (a *API) createDraft(w http.ResponseWriter, r *http.Request) {
	acc, err := a.account(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var body sendReq // {from, recipients, raw | raw_blob_id}: mismo contrato que /send.
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	crudo, soltar, err := a.rawDe(r, acc, body)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	defer soltar()
	m, err := a.storeDraft(r, acc, crudo)
	if err != nil {
		httpx.Fail(w, r, mapErr(err))
		return
	}
	httpx.JSON(w, r, http.StatusCreated, m)
}

// updateDraft reemplaza un borrador: guarda la versión nueva y descarta la vieja.
// El id cambia (el cliente adopta el devuelto); así no se necesita mutar el .eml
// en S3 in situ ni un UPDATE parcial del sobre. No es atómico, pero es un
// borrador: si el descarte del viejo fallara tras crear el nuevo, a lo sumo queda
// un duplicado, nunca se pierde contenido.
func (a *API) updateDraft(w http.ResponseWriter, r *http.Request) {
	acc, err := a.account(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	oldID, err := uuid.Parse(chi.URLParam(r, "messageID"))
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid messageID."))
		return
	}
	var body sendReq
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	crudo, soltar, err := a.rawDe(r, acc, body)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	defer soltar()
	m, err := a.storeDraft(r, acc, crudo)
	if err != nil {
		httpx.Fail(w, r, mapErr(err))
		return
	}
	if err := a.store.DeleteMessage(r.Context(), acc.ID, oldID); err != nil && !errors.Is(err, domain.ErrNotFound) {
		httpx.Fail(w, r, mapErr(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, m)
}

// storeDraft compone el sobre a partir del raw RFC 5322, sube el .eml a S3 y lo
// inserta en la carpeta Drafts con los keywords \Draft + \Seen. Es el camino de
// la copia en Sent de un saliente (delivery.Outbound.Submit) pero SIN DKIM ni
// cola: nada se envía.
func (a *API) storeDraft(r *http.Request, acc domain.Account, raw []byte) (domain.Message, error) {
	if a.blobs == nil {
		return domain.Message{}, httpx.ErrInternal.WithMessage("almacenamiento no disponible.")
	}
	t, err := tenancy.From(r.Context())
	if err != nil {
		return domain.Message{}, httpx.ErrUnauthorized
	}
	parsed, err := mime.Parse(bytes.NewReader(raw))
	if err != nil {
		return domain.Message{}, err
	}
	drafts, err := a.store.MailboxByRole(r.Context(), acc.ID, domain.RoleDrafts)
	if err != nil {
		return domain.Message{}, err
	}
	blobKey, err := blobs.GuardarMensaje(r.Context(), a.store, a.blobs, t, parsed.Raw)
	if err != nil {
		return domain.Message{}, err
	}
	env := parsed.Envelope
	env.AccountID = acc.ID
	env.MailboxID = drafts.ID
	env.BlobKey = blobKey
	env.SizeBytes = int64(len(parsed.Raw))
	env.Flags = []string{domain.FlagDraft, domain.FlagSeen}
	if err := a.store.InsertMessage(r.Context(), &env); err != nil {
		// ponytail: blob huérfano si el insert falla lo recoge el GC diferido de S3
		// (igual que en DeleteMessage); no exponemos Delete en blobStore por esto.
		return domain.Message{}, err
	}
	return env, nil
}

// --- helpers ---

// blobsGet obtiene el lector del blob; encapsula el acceso al objectstore desde
// el store (que expone el cliente vía la API; aquí lo tomamos del API struct).
func (a *API) blobsGet(r *http.Request, t tenancy.Tenant, key string) (io.ReadCloser, error) {
	if a.blobs == nil {
		return nil, httpx.ErrInternal.WithMessage("almacenamiento no disponible.")
	}
	return a.blobs.Get(r.Context(), t, key)
}

func errUnsupportedMethod(method string) error {
	return fmt.Errorf("método JMAP no soportado: %s", method)
}

// parseCursor descompone el cursor keyset "received_at,id" (RFC 3339 con nanos
// + uuid). Cadena vacía = primera página (cero, uuid.Nil).
func parseCursor(s string) (time.Time, uuid.UUID, error) {
	if s == "" {
		return time.Time{}, uuid.Nil, nil
	}
	ts, rest, ok := strings.Cut(s, ",")
	if !ok {
		return time.Time{}, uuid.Nil, errors.New("cursor sin separador")
	}
	t, err := time.Parse(time.RFC3339Nano, ts)
	if err != nil {
		return time.Time{}, uuid.Nil, err
	}
	id, err := uuid.Parse(rest)
	if err != nil {
		return time.Time{}, uuid.Nil, err
	}
	return t, id, nil
}

// formatCursor es el inverso de parseCursor (round-trip exacto: Postgres
// persiste received_at con precisión de microsegundos y RFC3339Nano la
// conserva). Se emite en UTC para que el cursor no lleve '+' de offset horario
// (que sin URL-encoding se decodificaría como espacio en la query).
func formatCursor(t time.Time, id uuid.UUID) string {
	return t.UTC().Format(time.RFC3339Nano) + "," + id.String()
}

func queryInt(r *http.Request, def int) int { return queryIntNamed(r, "limit", def) }

// queryIntNamed es un entero no negativo de la query; cualquier otra cosa es `def`.
func queryIntNamed(r *http.Request, nombre string, def int) int {
	v := r.URL.Query().Get(nombre)
	if v == "" || len(v) > 9 { // nueve cifras: ni desborda ni es una página real
		return def
	}
	n := 0
	for _, c := range v {
		if c < '0' || c > '9' {
			return def
		}
		n = n*10 + int(c-'0')
	}
	return n
}

// mapErr traduce errores de dominio a *httpx.Error canónicos.
func mapErr(err error) error {
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return httpx.ErrNotFound
	case errors.Is(err, domain.ErrScheduleGone):
		return httpx.ErrConflict.WithMessage("That message has already been released or does not exist; it cannot be cancelled or rescheduled.")
	case errors.Is(err, domain.ErrMailboxExists):
		return httpx.ErrConflict.WithMessage("That mailbox already exists.")
	case errors.Is(err, domain.ErrLabelExists):
		return httpx.ErrConflict.WithMessage("La etiqueta ya existe.")
	case errors.Is(err, domain.ErrAliasExists):
		return httpx.ErrConflict.WithMessage("That alias address is already in use.")
	case errors.Is(err, domain.ErrAliasReservado):
		return httpx.ErrBadRequest.WithCode("alias_reserved").WithMessage("That address is reserved: it would look like it comes from the service itself.")
	case errors.Is(err, domain.ErrBuzonesLimite):
		return httpx.ErrConflict.WithCode("shared_mailbox_limit").WithMessage("Your organisation has reached its limit of shared mailboxes.")
	case errors.Is(err, domain.ErrAliasLimite):
		return httpx.ErrConflict.WithCode("alias_limit").WithMessage("Your account can have one alias. Remove the existing one first.")
	case errors.Is(err, domain.ErrInvalidAddress):
		return httpx.ErrBadRequest.WithMessage("Invalid address or name.")
	case errors.Is(err, domain.ErrInvalidFilter):
		return httpx.ErrBadRequest.WithMessage("Invalid filter rule: give at least one condition and one action.")
	case errors.Is(err, domain.ErrQuotaExceeded):
		return httpx.NewError(http.StatusInsufficientStorage, "quota_exceeded", "Mailbox quota exceeded.")
	case errors.Is(err, domain.ErrMessageTooLarge):
		return httpx.ErrBadRequest.WithMessage("Mensaje demasiado grande.")
	case errors.Is(err, domain.ErrFromNoAutorizado):
		return httpx.ErrForbidden.WithCode("from_not_allowed").WithMessage("The From header must be exactly one address of the sending mailbox.")
	default:
		return err
	}
}

// getSMIMEAnchor sirve el certificado de la autoridad S/MIME de la organización.
//
//	GET /api/v1/smime/anchor  →  application/x-pem-file
//
// Se descarga y se instala una vez por dispositivo; a partir de ahí el correo
// firmado de la organización se ve como de confianza en vez de con la
// advertencia de «emisor desconocido». Es exactamente lo que el paquete smime
// dice que hay que distribuir.
//
// PEM Y NO JSON: el fichero se abre con el llavero del sistema, que es quien lo
// instala. Envolverlo en JSON obligaría a quien lo descarga a extraerlo a mano.
func (a *API) getSMIMEAnchor(w http.ResponseWriter, r *http.Request) {
	if a.smime == nil {
		httpx.Fail(w, r, httpx.ErrUnavailable.WithMessage("Este despliegue no tiene S/MIME configurado."))
		return
	}
	t, err := tenancy.From(r.Context())
	if err != nil {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	pem, err := a.smime.AnchorPEM(r.Context(), t.Slug)
	if errors.Is(err, smime.ErrNoMaterial) {
		// Una organización que aún no ha enviado ningún correo no tiene
		// autoridad todavía: se crea al firmar el primero. Eso es un 404 —no hay
		// nada que descargar—, no un 500: `mapErr` no conoce este error y lo
		// traducía a «error interno», que hace buscar una avería donde solo
		// falta usar el producto.
		httpx.Fail(w, r, httpx.ErrNotFound.WithMessage(
			"Esta organización todavía no tiene autoridad S/MIME: se crea con el primer correo firmado."))
		return
	}
	if err != nil {
		httpx.Fail(w, r, mapErr(err))
		return
	}
	w.Header().Set("Content-Type", "application/x-pem-file")
	w.Header().Set("Content-Disposition", `attachment; filename="sytue-smime-ca.pem"`)
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte(pem))
}

// putSMIMEIdentity instala en el buzón un certificado emitido por una CA
// PÚBLICA, con su cadena y su clave privada.
//
//	PUT /api/v1/smime/identity   (cuerpo: PEM)
//
// POR QUÉ EXISTE: la autoridad de la organización consigue que TODO el correo
// salga firmado, pero quien lo recibe fuera no tiene su ancla y ve un aviso —o,
// si la instaló, un fallo de revocación—. Con un certificado de una CA que ya
// viene de fábrica en los sistemas, la firma se valida SOLA en el móvil de
// cualquiera sin instalar ningún perfil, que es el objetivo de firmar.
//
// El cuerpo es el PEM tal cual sale de `openssl pkcs12 -in fichero.p12 -nodes`:
// certificado, cadena y clave en un solo texto. Sin JSON, para no obligar a
// escapar saltos de línea en un material que se maneja como fichero.
//
// Cada cual instala el SUYO: la cuenta sale del token y la dirección del buzón
// de la cuenta, así que no hay forma de instalarle un certificado a otro.
func (a *API) putSMIMEIdentity(w http.ResponseWriter, r *http.Request) {
	if a.smime == nil {
		httpx.Fail(w, r, httpx.ErrUnavailable.WithMessage("Este despliegue no tiene S/MIME configurado."))
		return
	}
	t, err := tenancy.From(r.Context())
	if err != nil {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	acc, err := a.account(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	// 128 KiB: un material S/MIME son unos pocos KB, y el tope evita que un PUT
	// mal dirigido se lleve memoria del servicio.
	cuerpo, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 128<<10))
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("No se pudo leer el certificado enviado."))
		return
	}
	certPEM, keyPEM := smime.SplitBundle(string(cuerpo))
	if certPEM == "" || keyPEM == "" {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage(
			"El cuerpo tiene que ser un PEM con el certificado, su cadena y la clave privada."))
		return
	}
	id, err := a.smime.Import(r.Context(), t.Slug, acc.ID, acc.Address, certPEM, keyPEM)
	if errors.Is(err, smime.ErrMaterialInvalido) {
		// El material es de quien lo manda: se le dice QUÉ está mal, porque es lo
		// único con lo que puede arreglarlo. Un 500 mandaría a buscar una avería
		// que no existe.
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage(err.Error()))
		return
	}
	if err != nil {
		httpx.Fail(w, r, mapErr(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{
		"address":   id.Address,
		"issuer":    id.Certificate.Issuer.CommonName,
		"not_after": id.NotAfter,
	})
}

// getSMIMEAnchorPublico sirve el ancla de una organización SIN autenticación.
//
//	GET /.well-known/smime/{org}.pem  →  application/x-pem-file
//
// Existe para quien recibe el correo, que no tiene cuenta: sin esto la firma
// solo es verificable dentro de la organización. Ver la nota de la ruta.
//
// TODOS LOS CAMINOS QUE NO SIRVEN UN CERTIFICADO CONTESTAN IGUAL: slug inválido,
// organización inexistente y organización sin autoridad dan el mismo 404 con el
// mismo texto. Distinguirlos convertiría este endpoint en una forma de averiguar
// qué organizaciones hay en la instalación.
func (a *API) getSMIMEAnchorPublico(w http.ResponseWriter, r *http.Request) {
	noHay := func() {
		httpx.Fail(w, r, httpx.ErrNotFound.WithMessage("No hay certificado publicado para esa organización."))
	}
	if a.smime == nil {
		noHay()
		return
	}
	org := chi.URLParam(r, "org")
	if !tenancy.ValidSlug(org) {
		noHay()
		return
	}
	pem, err := a.smime.AnchorPEM(r.Context(), org)
	if err != nil || pem == "" {
		noHay()
		return
	}
	w.Header().Set("Content-Type", "application/x-pem-file")
	w.Header().Set("Content-Disposition", `attachment; filename="sytue-smime-ca.pem"`)
	// Cacheable: es un certificado que dura años y lo pide gente de fuera. Una
	// hora basta para que una rotación se note el mismo día.
	w.Header().Set("Cache-Control", "public, max-age=3600")
	_, _ = w.Write([]byte(pem))
}
