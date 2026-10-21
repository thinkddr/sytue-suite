// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Mail es el cliente de Sytue Mail sobre su API REST (services/mail/internal/jmap,
// `/api/v1`). El servicio guarda el mensaje CRUDO y nada más: el cuerpo y los adjuntos se
// sacan de Raw con internal/eml, y para enviar se le manda un mensaje ya compuesto.
type Mail struct {
	c *Client
	// jmap va a la raíz del servicio: lo poco que solo existe en JMAP (renombrar y
	// borrar carpetas) se pide en `POST /jmap`.
	jmap *Client
}

// NewMail crea el cliente sobre la raíz del servicio (ServiceURL(apex, "mail")).
func NewMail(servicio string, s Session, h *http.Client) *Mail {
	return &Mail{c: New(servicio+"/api/v1", s, h), jmap: New(servicio, s, h)}
}

// Las marcas de un mensaje, con el nombre de IMAP que usa el servicio.
const (
	MailFlagSeen    = `\Seen`
	MailFlagFlagged = `\Flagged`
)

// Mailbox es una carpeta de correo. Role dice si es especial: "inbox", "sent", "drafts",
// "trash", "junk", "archive", o "" si es de la persona.
type Mailbox struct {
	ID           string  `json:"id"`
	ParentID     *string `json:"parent_id,omitempty"`
	Name         string  `json:"name"`
	Role         string  `json:"role,omitempty"`
	TotalEmails  int     `json:"total_emails"`
	UnreadEmails int     `json:"unread_emails"`
}

// MailInboxUnread returns the server's authoritative inbox count. Installations
// without an inbox role use their complete mailbox inventory as the fallback.
func MailInboxUnread(mailboxes []Mailbox) int {
	total := 0
	for _, mailbox := range mailboxes {
		if mailbox.Role == "inbox" {
			return mailbox.UnreadEmails
		}
		total += mailbox.UnreadEmails
	}
	return total
}

// MailMessage es el SOBRE de un correo: cabeceras indexadas y estado, sin el cuerpo.
type MailMessage struct {
	ID        string `json:"id"`
	MailboxID string `json:"mailbox_id"`
	ThreadID  string `json:"thread_id"`
	// ThreadCount son los mensajes del hilo en TODA la cuenta; solo lo trae el listado.
	ThreadCount   int       `json:"thread_count,omitempty"`
	MessageID     string    `json:"message_id"`
	InReplyTo     string    `json:"in_reply_to,omitempty"`
	Subject       string    `json:"subject"`
	From          string    `json:"from"`
	FromName      string    `json:"from_name,omitempty"`
	To            []string  `json:"to"`
	Cc            []string  `json:"cc,omitempty"`
	Bcc           []string  `json:"bcc,omitempty"`
	ReplyTo       []string  `json:"reply_to,omitempty"`
	Preview       string    `json:"preview,omitempty"`
	Size          int64     `json:"size"`
	Keywords      []string  `json:"keywords,omitempty"`
	HasAttachment bool      `json:"has_attachment"`
	ReceivedAt    time.Time `json:"received_at"`
	Date          time.Time `json:"date"`
}

// Has dice si el mensaje lleva una marca; como el servicio, sin distinguir mayúsculas.
func (m MailMessage) Has(marca string) bool {
	return slices.ContainsFunc(m.Keywords, func(k string) bool { return strings.EqualFold(k, marca) })
}

// Sender es quien escribe, como se enseña en la lista: su nombre, o su dirección.
func (m MailMessage) Sender() string {
	if m.FromName != "" {
		return m.FromName
	}
	return m.From
}

// Address es la dirección principal de la cuenta: el «De» de lo que se envía. No es el
// correo con el que se entra en Sytue ID, que puede ser otro.
func (m *Mail) Address(ctx context.Context) (string, error) {
	r, err := JSON[struct {
		Primary string `json:"primary"`
	}](ctx, m.c, Request{Path: "/aliases"})
	return r.Primary, err
}

// MailAlias es otra dirección que entrega a la misma cuenta (y desde la que se puede
// enviar).
type MailAlias struct {
	ID      string `json:"id"`
	Address string `json:"address"`
}

// Aliases es la dirección principal y los alias de la cuenta: las direcciones «De» que
// ofrece el redactor, además de los buzones compartidos con permiso de envío.
func (m *Mail) Aliases(ctx context.Context) (principal string, alias []MailAlias, err error) {
	r, err := JSON[struct {
		Primary string      `json:"primary"`
		Aliases []MailAlias `json:"aliases"`
	}](ctx, m.c, Request{Path: "/aliases"})
	return r.Primary, r.Aliases, err
}

// CreateAlias da de alta un alias. El servicio lo rechaza con código propio si la
// dirección está reservada (400 `alias_reserved`) o si ya hay uno (409 `alias_limit`):
// se leen con Error.Code().
func (m *Mail) CreateAlias(ctx context.Context, direccion string) (MailAlias, error) {
	r, err := m.peticion(http.MethodPost, "/aliases", map[string]string{"address": direccion})
	if err != nil {
		return MailAlias{}, err
	}
	return JSON[MailAlias](ctx, m.c, r)
}

// DeleteAlias quita un alias.
func (m *Mail) DeleteAlias(ctx context.Context, id string) error {
	return m.sinCuerpo(ctx, http.MethodDelete, "/aliases/"+url.PathEscape(id), nil)
}

// CreateMailbox crea una carpeta de la persona. El servicio rechaza el nombre repetido
// (409) y el que no vale (400); renombrar y borrar NO están en REST y van por JMAP
// (RenameMailbox, DeleteMailbox).
func (m *Mail) CreateMailbox(ctx context.Context, nombre string) (Mailbox, error) {
	r, err := m.peticion(http.MethodPost, "/mailboxes", map[string]any{"name": nombre})
	if err != nil {
		return Mailbox{}, err
	}
	return JSON[Mailbox](ctx, m.c, r)
}

// MailboxSetError es el motivo por el que el servicio NO aplicó un cambio de carpeta:
// "forbidden" (es de sistema), "mailboxHasEmail" (tiene correo), "alreadyExists",
// "invalidProperties" o "notFound". Son respuestas que la pantalla explica, no averías:
// una avería (red, 5xx) llega como error aparte.
type MailboxSetError string

// RenameMailbox renombra una carpeta de la persona (`Mailbox/set` update). Devuelve ""
// si se aplicó.
func (m *Mail) RenameMailbox(ctx context.Context, id, nombre string) (MailboxSetError, error) {
	return m.mailboxSet(ctx, map[string]any{"update": map[string]any{id: map[string]string{"name": nombre}}}, "notUpdated", id)
}

// DeleteMailbox borra una carpeta de la persona que esté VACÍA (`Mailbox/set` destroy):
// con correo dentro, el servicio la niega con "mailboxHasEmail".
func (m *Mail) DeleteMailbox(ctx context.Context, id string) (MailboxSetError, error) {
	return m.mailboxSet(ctx, map[string]any{"destroy": []string{id}}, "notDestroyed", id)
}

// mailboxSet hace un `Mailbox/set` y lee el motivo del rechazo de `id` en `cubo`.
//
// ponytail: sin `accountId` ni la sesión JMAP que lo diría: el servicio toma la cuenta
// del token y no lo lee. Si algún día hay más de una cuenta por persona, se pide
// `/jmap/session` y se manda su `primaryAccounts`.
func (m *Mail) mailboxSet(ctx context.Context, args map[string]any, cubo, id string) (MailboxSetError, error) {
	cuerpo, err := json.Marshal(map[string]any{
		"using":       []string{"urn:ietf:params:jmap:core", "urn:ietf:params:jmap:mail"},
		"methodCalls": []any{[]any{"Mailbox/set", args, "0"}},
	})
	if err != nil {
		return "", err
	}
	r, err := JSON[struct {
		MethodResponses []json.RawMessage `json:"methodResponses"` //nolint:tagliatelle // JMAP (RFC 8620) es camelCase
	}](ctx, m.jmap, Request{
		Method: http.MethodPost, Path: "/jmap", Body: cuerpo,
		Header: http.Header{"Content-Type": {"application/json"}},
	})
	if err != nil {
		return "", err
	}
	if len(r.MethodResponses) == 0 {
		return "", errors.New("mail: Mailbox/set sin respuesta")
	}
	var resp []json.RawMessage
	if err := json.Unmarshal(r.MethodResponses[0], &resp); err != nil || len(resp) < 2 {
		return "", errors.New("mail: Mailbox/set con respuesta rara")
	}
	var nombre string
	_ = json.Unmarshal(resp[0], &nombre)
	var res map[string]json.RawMessage
	_ = json.Unmarshal(resp[1], &res)
	if nombre == "error" {
		var e struct{ Type, Description string }
		_ = json.Unmarshal(resp[1], &e)
		return "", errors.New("mail: Mailbox/set: " + e.Type + " " + e.Description)
	}
	var rechazos map[string]struct {
		Type string `json:"type"`
	}
	_ = json.Unmarshal(res[cubo], &rechazos)
	if x, hay := rechazos[id]; hay {
		if x.Type == "" {
			return "serverFail", nil
		}
		return MailboxSetError(x.Type), nil
	}
	return "", nil
}

// MailFilter es una regla de servidor: condiciones AND de subcadena (remitente,
// asunto) y acciones (mover, leído, destacado). La aplica la entrega entrante.
type MailFilter struct {
	ID           string `json:"id,omitempty"`
	Name         string `json:"name"`
	Enabled      bool   `json:"enabled"`
	Priority     int    `json:"priority"`
	MatchFrom    string `json:"match_from"`
	MatchSubject string `json:"match_subject"`
	// MoveTo es la carpeta de destino; vacía, no mueve.
	MoveTo      string `json:"move_to,omitempty"`
	MarkRead    bool   `json:"mark_read"`
	MarkFlagged bool   `json:"mark_flagged"`
}

// cuerpoFiltro es lo que acepta el servicio: `move_to` null para no mover.
func cuerpoFiltro(f MailFilter) map[string]any {
	c := map[string]any{
		"name": f.Name, "enabled": f.Enabled, "priority": f.Priority,
		"match_from": f.MatchFrom, "match_subject": f.MatchSubject,
		"mark_read": f.MarkRead, "mark_flagged": f.MarkFlagged, "move_to": nil,
	}
	if f.MoveTo != "" {
		c["move_to"] = f.MoveTo
	}
	return c
}

// Filters lista las reglas, en su orden de evaluación.
func (m *Mail) Filters(ctx context.Context) ([]MailFilter, error) {
	r, err := JSON[struct {
		Filters []MailFilter `json:"filters"`
	}](ctx, m.c, Request{Path: "/filters"})
	return r.Filters, err
}

// CreateFilter crea una regla; el servicio exige al menos una condición.
func (m *Mail) CreateFilter(ctx context.Context, f MailFilter) (MailFilter, error) {
	r, err := m.peticion(http.MethodPost, "/filters", cuerpoFiltro(f))
	if err != nil {
		return MailFilter{}, err
	}
	return JSON[MailFilter](ctx, m.c, r)
}

// UpdateFilter SUSTITUYE la regla entera: para activarla o desactivarla se manda con
// todos sus campos.
func (m *Mail) UpdateFilter(ctx context.Context, f MailFilter) error {
	return m.sinCuerpo(ctx, http.MethodPut, "/filters/"+url.PathEscape(f.ID), cuerpoFiltro(f))
}

// DeleteFilter borra una regla.
func (m *Mail) DeleteFilter(ctx context.Context, id string) error {
	return m.sinCuerpo(ctx, http.MethodDelete, "/filters/"+url.PathEscape(id), nil)
}

// SmimeAnchor es el certificado (PEM) de la autoridad S/MIME de la organización: lo
// que se instala UNA vez en un dispositivo para que la firma del correo salga «de
// confianza». Un 404 es que la organización aún no tiene autoridad (se crea con el
// primer correo firmado), no una avería.
func (m *Mail) SmimeAnchor(ctx context.Context) ([]byte, error) {
	resp, err := m.c.Do(ctx, Request{Path: "/smime/anchor", Header: http.Header{"Accept": {"application/x-pem-file"}}})
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(io.LimitReader(resp.Body, MaxMailBody))
}

// MaxSmimeIdentity es lo que el servicio admite del PEM propio.
const MaxSmimeIdentity = 128 << 10

// SetSmimeIdentity instala el certificado S/MIME PROPIO del buzón: certificado, cadena
// y clave en un PEM (lo que sale de `openssl pkcs12 -in fichero.p12 -nodes`). El
// motivo de un rechazo viene en Error.Message().
func (m *Mail) SetSmimeIdentity(ctx context.Context, pem string) error {
	if len(pem) > MaxSmimeIdentity {
		return ErrMailTooLarge
	}
	resp, err := m.c.Do(ctx, Request{
		Method: http.MethodPut, Path: "/smime/identity", Body: []byte(pem),
		Header: http.Header{"Content-Type": {"application/x-pem-file"}},
	})
	if err != nil {
		return err
	}
	return resp.Body.Close()
}

// MailShared es un buzón ajeno al que se tiene acceso: uno compartido de la
// organización (Kind "shared": soporte@, ventas@) o el de otra persona que lo ha
// delegado (Kind "user"). Permission es "read", "send" o "manage".
type MailShared struct {
	AccountID   string `json:"account_id"`
	Address     string `json:"address"`
	DisplayName string `json:"display_name,omitempty"`
	Kind        string `json:"kind"`
	Permission  string `json:"permission"`
	// EditRequested: en "read", ya ha pedido acceso (la franja lo dice).
	EditRequested bool `json:"edit_requested,omitempty"`
}

// PeticionesBuzon es la ruta de «pedir acceso» de un buzón compartido o delegado.
func (m *Mail) PeticionesBuzon(cuenta string) Peticiones {
	return NuevasPeticiones(m.c, "/shared-mailboxes/"+url.PathEscape(cuenta)+"/edit-requests")
}

// MailGrant es una entrada de acceso: quién (su dirección) y con qué permiso.
type MailGrant struct {
	Grantee    string `json:"grantee"`
	Permission string `json:"permission"`
}

// SharedMailboxes lista los buzones ajenos a los que se tiene acceso.
func (m *Mail) SharedMailboxes(ctx context.Context) ([]MailShared, error) {
	r, err := JSON[struct {
		Shared []MailShared `json:"shared_mailboxes"`
	}](ctx, m.c, Request{Path: "/shared-mailboxes"})
	return r.Shared, err
}

// CreateSharedMailbox crea un buzón compartido de la organización; quien lo crea lo
// gestiona. La dirección tiene que ser de un dominio del servicio.
func (m *Mail) CreateSharedMailbox(ctx context.Context, direccion, nombre string) (MailShared, error) {
	r, err := m.peticion(http.MethodPost, "/shared-mailboxes", map[string]string{"address": direccion, "display_name": nombre})
	if err != nil {
		return MailShared{}, err
	}
	return JSON[MailShared](ctx, m.c, r)
}

// SharedFolders lista las carpetas de un buzón ajeno (con permiso de lectura).
func (m *Mail) SharedFolders(ctx context.Context, cuenta string) ([]Mailbox, error) {
	r, err := JSON[struct {
		Mailboxes []Mailbox `json:"mailboxes"`
	}](ctx, m.c, Request{Path: "/shared-mailboxes/" + url.PathEscape(cuenta) + "/mailboxes"})
	return r.Mailboxes, err
}

// SharedMessages es una página de una carpeta de un buzón ajeno, como Messages.
func (m *Mail) SharedMessages(ctx context.Context, cuenta, carpeta, cursor string, limite int) ([]MailMessage, string, error) {
	return m.pagina(ctx, "/shared-mailboxes/"+url.PathEscape(cuenta)+"/mailboxes/"+url.PathEscape(carpeta)+"/messages", cursor, limite)
}

// Grants es quién tiene acceso a un buzón compartido (hace falta gestionarlo).
func (m *Mail) Grants(ctx context.Context, cuenta string) ([]MailGrant, error) {
	return m.accesos(ctx, "/shared-mailboxes/"+url.PathEscape(cuenta)+"/grants")
}

// Grant da o cambia el acceso de alguien a un buzón compartido.
func (m *Mail) Grant(ctx context.Context, cuenta, quien, permiso string) error {
	return m.sinCuerpo(ctx, http.MethodPost, "/shared-mailboxes/"+url.PathEscape(cuenta)+"/grants",
		map[string]any{"grantee": quien, "permission": permiso})
}

// Revoke quita el acceso de alguien a un buzón compartido.
func (m *Mail) Revoke(ctx context.Context, cuenta, quien string) error {
	return m.sinCuerpo(ctx, http.MethodPost, "/shared-mailboxes/"+url.PathEscape(cuenta)+"/grants",
		map[string]any{"grantee": quien, "delete": true})
}

// Delegations es a quién se ha delegado el buzón PROPIO.
func (m *Mail) Delegations(ctx context.Context) ([]MailGrant, error) {
	return m.accesos(ctx, "/delegation")
}

// Delegate delega el buzón propio en alguien: "read" o "send" (en su nombre).
func (m *Mail) Delegate(ctx context.Context, quien, permiso string) error {
	return m.sinCuerpo(ctx, http.MethodPost, "/delegation", map[string]any{"grantee": quien, "permission": permiso})
}

// Undelegate retira una delegación del buzón propio.
func (m *Mail) Undelegate(ctx context.Context, quien string) error {
	return m.sinCuerpo(ctx, http.MethodPost, "/delegation", map[string]any{"grantee": quien, "delete": true})
}

func (m *Mail) accesos(ctx context.Context, ruta string) ([]MailGrant, error) {
	r, err := JSON[struct {
		Grants []MailGrant `json:"grants"`
	}](ctx, m.c, Request{Path: ruta})
	return r.Grants, err
}

// Mailboxes lista las carpetas.
func (m *Mail) Mailboxes(ctx context.Context) ([]Mailbox, error) {
	r, err := JSON[struct {
		Mailboxes []Mailbox `json:"mailboxes"`
	}](ctx, m.c, Request{Path: "/mailboxes"})
	return r.Mailboxes, err
}

// Messages devuelve una página de una carpeta, de más nuevo a más viejo. cursor vacío es
// la primera; siguiente vacío, la última.
func (m *Mail) Messages(ctx context.Context, carpeta, cursor string, limite int) (mensajes []MailMessage, siguiente string, err error) {
	return m.pagina(ctx, "/mailboxes/"+url.PathEscape(carpeta)+"/messages", cursor, limite)
}

func (m *Mail) pagina(ctx context.Context, ruta, cursor string, limite int) (mensajes []MailMessage, siguiente string, err error) {
	q := url.Values{"limit": {strconv.Itoa(limite)}}
	if cursor != "" {
		q.Set("cursor", cursor)
	}
	r, err := JSON[struct {
		Messages   []MailMessage `json:"messages"`
		NextCursor string        `json:"next_cursor"`
	}](ctx, m.c, Request{Path: ruta, Query: q})
	return r.Messages, r.NextCursor, err
}

// Search busca en todo el correo, con los operadores del servicio («from:ana»,
// «has:attachment»). Pagina por posición, como `Email/query`: `desde` es cuántos
// resultados saltar, así que la página siguiente pide `desde` + los ya recibidos, y
// una página más corta que `limite` es la última. El servicio no da más de 50 de golpe.
func (m *Mail) Search(ctx context.Context, q string, desde, limite int) ([]MailMessage, error) {
	query := url.Values{"q": {q}, "limit": {strconv.Itoa(limite)}}
	if desde > 0 {
		query.Set("position", strconv.Itoa(desde))
	}
	r, err := JSON[struct {
		Messages []MailMessage `json:"messages"`
	}](ctx, m.c, Request{Path: "/messages/search", Query: query})
	return r.Messages, err
}

// Llegadas escucha el aviso de correo nuevo (`GET /jmap/eventos`, el mismo flujo que
// la web abre con `EventSource`) y llama a `alLlegar` con cada `llegada`. ES UN AVISO,
// NO UN CANAL DE DATOS: quien llama pregunta después por el delta de siempre, así que
// si el flujo se cae no hay nada que reparar. No vuelve hasta que se corta o muere
// `ctx`; para mantenerlo vivo, Escuchar.
func (m *Mail) Llegadas(ctx context.Context, alLlegar func()) error {
	resp, err := m.jmap.Do(ctx, Request{Path: "/jmap/eventos", Header: http.Header{"Accept": {"text/event-stream"}}})
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	return leerSSE(resp.Body, func(evento, _ string) {
		if evento == "llegada" {
			alLlegar()
		}
	})
}

// Raw es el .eml del mensaje. Cerrarlo es cosa de quien llama.
func (m *Mail) Raw(ctx context.Context, id string) (io.ReadCloser, error) {
	resp, err := m.c.Do(ctx, Request{Path: "/messages/" + url.PathEscape(id) + "/raw", Header: http.Header{"Accept": {"message/rfc822"}}})
	if err != nil {
		return nil, err
	}
	return resp.Body, nil
}

// MaxMailBody es lo que el servicio admite en el cuerpo JSON de una petición: /send y
// /drafts pasan por httpx.DecodeJSON, que corta en 1 MiB. Lo que no cabe se rechaza
// AQUÍ, con un error que se puede explicar, y no con el «400 body_too_large» del
// servicio. Un mensaje que no cabe no llega a esto: va subido aparte (conMensaje).
const MaxMailBody = 1 << 20

// ErrMailTooLarge: el mensaje compuesto no cabe en lo que el servicio admite.
var ErrMailTooLarge = errors.New("api: el mensaje no cabe en una petición de Sytue Mail")

func (m *Mail) peticion(metodo, ruta string, cuerpo any) (Request, error) {
	r := Request{Method: metodo, Path: ruta}
	if cuerpo != nil {
		datos, err := json.Marshal(cuerpo)
		if err != nil {
			return r, err
		}
		if len(datos) > MaxMailBody {
			return r, ErrMailTooLarge
		}
		r.Body, r.Header = datos, http.Header{"Content-Type": {"application/json"}}
	}
	return r, nil
}

// conMensaje es la petición de /send o /drafts con el mensaje compuesto dentro (`raw`)
// o, si con él el JSON no cabe en los 1 MiB del servicio —un adjunto de 800 KB en base64
// ya no cabía—, subido antes por `/jmap/upload` y referenciado (`raw_blob_id`).
func (m *Mail) conMensaje(ctx context.Context, metodo, ruta string, cuerpo map[string]any, crudo []byte) (Request, error) {
	cuerpo["raw"] = string(crudo)
	r, err := m.peticion(metodo, ruta, cuerpo)
	if !errors.Is(err, ErrMailTooLarge) {
		return r, err
	}
	// El servicio toma la cuenta del token: el `{accountId}` de la ruta no se mira.
	resp, err := m.jmap.Do(ctx, Request{
		Method: http.MethodPost, Path: "/jmap/upload/me", Body: crudo,
		Header: http.Header{"Content-Type": {"message/rfc822"}},
	})
	if err != nil {
		return r, err
	}
	defer func() { _ = resp.Body.Close() }()
	var subido struct {
		BlobID string `json:"blobId"` //nolint:tagliatelle // JMAP (RFC 8620) es camelCase
	}
	if err := json.NewDecoder(resp.Body).Decode(&subido); err != nil || subido.BlobID == "" {
		return r, errors.Join(errors.New("api: la subida del mensaje no devolvió blobId"), err)
	}
	delete(cuerpo, "raw")
	cuerpo["raw_blob_id"] = subido.BlobID
	return m.peticion(metodo, ruta, cuerpo)
}

func (m *Mail) sinCuerpo(ctx context.Context, metodo, ruta string, cuerpo any) error {
	r, err := m.peticion(metodo, ruta, cuerpo)
	if err != nil {
		return err
	}
	resp, err := m.c.Do(ctx, r)
	if err != nil {
		return err
	}
	return resp.Body.Close()
}

// SetFlag pone o quita una marca (MailFlagSeen, MailFlagFlagged).
func (m *Mail) SetFlag(ctx context.Context, id, marca string, poner bool) error {
	return m.sinCuerpo(ctx, http.MethodPost, "/messages/"+url.PathEscape(id)+"/flags", map[string]any{"flag": marca, "add": poner})
}

// Move lleva un mensaje a otra carpeta.
func (m *Mail) Move(ctx context.Context, id, carpeta string) error {
	return m.sinCuerpo(ctx, http.MethodPost, "/messages/"+url.PathEscape(id)+"/move", map[string]string{"mailbox_id": carpeta})
}

// Delete borra un mensaje PARA SIEMPRE. «Eliminar» en la interfaz es Move a la papelera.
func (m *Mail) Delete(ctx context.Context, id string) error {
	return m.sinCuerpo(ctx, http.MethodDelete, "/messages/"+url.PathEscape(id), nil)
}

// MailEnvio es lo que contesta `POST /send`. Con ventana de deshacer el correo queda
// RETENIDO en la cola hasta `ReleaseAt` y `ID` es lo que cancela CancelOutbound; sin
// ventana sale ya y no hay id que cancelar.
type MailEnvio struct {
	Status    string    `json:"status"` // queued | undo | scheduled
	ID        string    `json:"id"`
	ReleaseAt time.Time `json:"release_at"`
}

// MaxUndoSeconds es el tope que aplica el servicio (`maxUndoSeconds` en
// services/mail/internal/jmap/handlers.go): pedir más lo recorta allí, sin error.
const MaxUndoSeconds = 30

// Send encola un mensaje ya compuesto (eml.Componer). `de` vacío es la dirección
// principal; otra tiene que ser un alias o un buzón con permiso, o el servicio contesta 403.
//
// Con `undoSegundos` > 0 el envío se RETIENE esa ventana antes de salir: es lo que hace
// que «Deshacer» pueda existir. Cero es salir ya.
func (m *Mail) Send(ctx context.Context, de string, destinatarios []string, crudo []byte, undoSegundos int) (MailEnvio, error) {
	cuerpo := map[string]any{"from": de, "recipients": destinatarios}
	if undoSegundos > 0 {
		cuerpo["undo_seconds"] = undoSegundos
	}
	r, err := m.conMensaje(ctx, http.MethodPost, "/send", cuerpo, crudo)
	if err != nil {
		return MailEnvio{}, err
	}
	return JSON[MailEnvio](ctx, m.c, r)
}

// Schedule encola un mensaje ya compuesto para que salga a una hora futura: el mismo
// diferido que el «deshacer», con otra espera (`send_at`). Hasta entonces se ve en
// Scheduled y se cancela con CancelOutbound.
func (m *Mail) Schedule(ctx context.Context, de string, destinatarios []string, crudo []byte, cuando time.Time) (MailEnvio, error) {
	r, err := m.conMensaje(ctx, http.MethodPost, "/send", map[string]any{
		"from": de, "recipients": destinatarios,
		"send_at": cuando.UTC().Format(time.RFC3339),
	}, crudo)
	if err != nil {
		return MailEnvio{}, err
	}
	return JSON[MailEnvio](ctx, m.c, r)
}

// ErrDemasiadoTarde: el correo ya ha salido, así que no se puede deshacer. El servicio lo
// dice con un 409 y es la carrera que importa: la alternativa sería creerse que se
// deshizo algo que ya está en el buzón de la otra persona.
var ErrDemasiadoTarde = errors.New("api: el correo ya ha salido")

// CancelOutbound deshace un envío retenido (deshacer o programado). Es idempotente-
// consciente: si ya salió devuelve ErrDemasiadoTarde, nunca un doble efecto.
func (m *Mail) CancelOutbound(ctx context.Context, id string) error {
	err := m.sinCuerpo(ctx, http.MethodPost, "/outbound/"+url.PathEscape(id)+"/cancel", nil)
	var deAPI *Error
	if errors.As(err, &deAPI) && deAPI.Status == http.StatusConflict {
		return ErrDemasiadoTarde
	}
	return err
}

// MailBaja dice qué métodos de baja ofrece un mensaje de lista (RFC 2369
// List-Unsubscribe + RFC 8058 one-click). El servicio NO devuelve la URL ni la dirección
// —solo si las hay— y es él quien ejecuta la baja: así un cliente no puede inducir una
// petición a una URL cualquiera.
type MailBaja struct {
	OneClick bool `json:"one_click"`
	Mailto   bool `json:"mailto"`
}

// Hay dice si se puede ofrecer «Darse de baja».
func (b MailBaja) Hay() bool { return b.OneClick || b.Mailto }

// Unsubscribe pregunta qué métodos de baja ofrece un mensaje.
func (m *Mail) Unsubscribe(ctx context.Context, id string) (MailBaja, error) {
	return JSON[MailBaja](ctx, m.c, Request{Path: "/messages/" + url.PathEscape(id) + "/unsubscribe"})
}

// Los tres resultados de una baja: hecha (one-click), pedida por correo, o fallida.
const (
	BajaHecha   = "ok"
	BajaEnviada = "sent"
	BajaFallida = "failed"
)

// DoUnsubscribe ejecuta la baja. El servicio elige el método seguro —one-click https
// validado contra SSRF, o el correo de baja desde la dirección de la persona— y contesta
// 200 con el resultado aunque no lo consiga: quien lee tiene que saber en qué quedó.
func (m *Mail) DoUnsubscribe(ctx context.Context, id string) (string, error) {
	r, err := JSON[struct {
		Status string `json:"status"`
	}](ctx, m.c, Request{Method: http.MethodPost, Path: "/messages/" + url.PathEscape(id) + "/unsubscribe"})
	return r.Status, err
}

// SaveDraft guarda un borrador ya compuesto (eml.Componer con Borrador) en la carpeta de
// borradores, sin enviar nada. Con `id`, SUSTITUYE a ese borrador y el id CAMBIA: hay que
// quedarse con el del mensaje devuelto. Descartarlo es Delete.
func (m *Mail) SaveDraft(ctx context.Context, id, de string, crudo []byte) (MailMessage, error) {
	metodo, ruta := http.MethodPost, "/drafts"
	if id != "" {
		metodo, ruta = http.MethodPut, "/drafts/"+url.PathEscape(id)
	}
	r, err := m.conMensaje(ctx, metodo, ruta, map[string]any{"from": de}, crudo)
	if err != nil {
		return MailMessage{}, err
	}
	return JSON[MailMessage](ctx, m.c, r)
}

// Thread es la conversación ENTERA, de más nuevo a más viejo y atravesando carpetas: lo
// que tú contestaste vive en Enviados y la lista de una carpeta no lo trae.
func (m *Mail) Thread(ctx context.Context, hilo string) ([]MailMessage, error) {
	r, err := JSON[struct {
		Messages []MailMessage `json:"messages"`
	}](ctx, m.c, Request{Path: "/threads/" + url.PathEscape(hilo) + "/messages"})
	return r.Messages, err
}

// MailCambios es el DELTA de la cuenta desde un estado (`GET /changes`): los sobres que
// han cambiado —llegados, movidos, marcados—, los ids que ya no existen y el estado
// desde el que preguntar la próxima vez. Es el Email/changes de JMAP sobre REST.
type MailCambios struct {
	Changed   []MailMessage `json:"changed"`
	Destroyed []string      `json:"destroyed"`
	State     string        `json:"state"`
	// HasMore dice que el servidor ha cortado la página: hay que volver a preguntar YA
	// y no esperar al siguiente sondeo, o una ráfaga grande gotearía una página por
	// intervalo.
	HasMore bool `json:"has_more"`
}

// Changes pregunta qué ha cambiado desde `desde`. Con `desde` vacío es el ARRANQUE: el
// servidor contesta su estado actual sin cambios, y a partir de ahí cada vuelta trae
// solo el delta. Así se refresca la bandeja abierta sin volver a pedirla entera.
func (m *Mail) Changes(ctx context.Context, desde string) (MailCambios, error) {
	q := url.Values{}
	if desde != "" {
		q.Set("since", desde)
	}
	return JSON[MailCambios](ctx, m.c, Request{Path: "/changes", Query: q})
}

// MoveMany lleva varios mensajes —una conversación— a otra carpeta en UNA operación.
func (m *Mail) MoveMany(ctx context.Context, ids []string, carpeta string) error {
	return m.sinCuerpo(ctx, http.MethodPost, "/messages/batch/move", map[string]any{"ids": ids, "mailbox_id": carpeta})
}

// SetFlagMany pone o quita una marca a varios mensajes en UNA operación.
func (m *Mail) SetFlagMany(ctx context.Context, ids []string, marca string, poner bool) error {
	return m.sinCuerpo(ctx, http.MethodPost, "/messages/batch/flags", map[string]any{"ids": ids, "flag": marca, "add": poner})
}

// MailScheduled es un envío PROGRAMADO que todavía no ha salido. Es el mismo diferido
// que el «deshacer», con otra espera: `release_at` dice cuándo sale.
type MailScheduled struct {
	ID         string    `json:"id"`
	From       string    `json:"from"`
	Recipients []string  `json:"recipients"`
	Subject    string    `json:"subject"`
	ReleaseAt  time.Time `json:"release_at"`
	CreatedAt  time.Time `json:"created_at"`
}

// Scheduled lista los envíos programados pendientes, del que sale antes al que sale
// después. Se cancelan con CancelOutbound, que es la misma cola.
func (m *Mail) Scheduled(ctx context.Context) ([]MailScheduled, error) {
	r, err := JSON[struct {
		Scheduled []MailScheduled `json:"scheduled"`
	}](ctx, m.c, Request{Path: "/scheduled"})
	return r.Scheduled, err
}

// Reschedule cambia la hora de un programado. El instante tiene que estar en el futuro:
// el servicio rechaza lo demás.
func (m *Mail) Reschedule(ctx context.Context, id string, cuando time.Time) error {
	return m.sinCuerpo(ctx, http.MethodPost, "/scheduled/"+url.PathEscape(id)+"/reschedule",
		map[string]any{"send_at": cuando.UTC().Format(time.RFC3339)})
}

// MailQuota es el espacio del buzón: cuánto se ha gastado y cuánto hay. Un límite a
// cero es «sin cuota configurada», y entonces no hay nada que medir.
type MailQuota struct {
	UsedBytes  int64 `json:"used_bytes"`
	QuotaBytes int64 `json:"quota_bytes"`
}

// Quota lee el espacio del buzón (el medidor del pie del menú). El uso es el contador
// agregado que el servicio mantiene, no una suma por mensaje.
func (m *Mail) Quota(ctx context.Context) (MailQuota, error) {
	return JSON[MailQuota](ctx, m.c, Request{Path: "/quota"})
}

// MailLabel es una etiqueta de la persona. Un mensaje la lleva como keyword: su ID.
type MailLabel struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Color        string `json:"color,omitempty"`
	TotalEmails  int    `json:"total_emails"`
	UnreadEmails int    `json:"unread_emails"`
}

// Labels lista las etiquetas.
func (m *Mail) Labels(ctx context.Context) ([]MailLabel, error) {
	r, err := JSON[struct {
		Labels []MailLabel `json:"labels"`
	}](ctx, m.c, Request{Path: "/labels"})
	return r.Labels, err
}

// CreateLabel crea una etiqueta. `color` es un token de la paleta cerrada
// (theme.EtiquetasColores), no un hex: es lo que el servicio valida.
func (m *Mail) CreateLabel(ctx context.Context, nombre, color string) (MailLabel, error) {
	r, err := m.peticion(http.MethodPost, "/labels", map[string]any{"name": nombre, "color": color})
	if err != nil {
		return MailLabel{}, err
	}
	return JSON[MailLabel](ctx, m.c, r)
}

// UpdateLabel cambia el nombre y el color de una etiqueta. Los dos a la vez: el servicio
// SUSTITUYE, así que mandar solo uno borraría el otro.
func (m *Mail) UpdateLabel(ctx context.Context, id, nombre, color string) error {
	return m.sinCuerpo(ctx, http.MethodPut, "/labels/"+url.PathEscape(id), map[string]any{"name": nombre, "color": color})
}

// DeleteLabel borra una etiqueta. Los mensajes que la llevaban se quedan donde estaban:
// una etiqueta no es una carpeta.
func (m *Mail) DeleteLabel(ctx context.Context, id string) error {
	return m.sinCuerpo(ctx, http.MethodDelete, "/labels/"+url.PathEscape(id), nil)
}

// ApplyLabel pone o quita una etiqueta a varios mensajes.
func (m *Mail) ApplyLabel(ctx context.Context, etiqueta string, ids []string, poner bool) error {
	return m.sinCuerpo(ctx, http.MethodPost, "/labels/"+url.PathEscape(etiqueta)+"/apply", map[string]any{"message_ids": ids, "add": poner})
}

// LabelMessages pagina los mensajes de una etiqueta, con el contrato de Messages.
func (m *Mail) LabelMessages(ctx context.Context, etiqueta, cursor string, limite int) (mensajes []MailMessage, siguiente string, err error) {
	return m.pagina(ctx, "/labels/"+url.PathEscape(etiqueta)+"/messages", cursor, limite)
}

// Signature es la firma de la cuenta: texto plano, va al final de lo que se escribe.
func (m *Mail) Signature(ctx context.Context) (string, error) {
	r, err := JSON[struct {
		Signature string `json:"signature"`
	}](ctx, m.c, Request{Path: "/signature"})
	return r.Signature, err
}

// SetSignature guarda la firma (el servicio rechaza más de 8 KiB, sin truncar).
func (m *Mail) SetSignature(ctx context.Context, firma string) error {
	return m.sinCuerpo(ctx, http.MethodPut, "/signature", map[string]string{"signature": firma})
}

// MailVacation es la respuesta automática. Sin StartsAt empieza ya; sin EndsAt, no acaba.
type MailVacation struct {
	Enabled  bool       `json:"enabled"`
	Subject  string     `json:"subject"`
	Body     string     `json:"body"`
	StartsAt *time.Time `json:"starts_at,omitempty"`
	EndsAt   *time.Time `json:"ends_at,omitempty"`
}

// Vacation lee la respuesta automática.
func (m *Mail) Vacation(ctx context.Context) (MailVacation, error) {
	return JSON[MailVacation](ctx, m.c, Request{Path: "/vacation"})
}

// SetVacation la SUSTITUYE entera. El PUT quiere las fechas como texto RFC 3339 —vacío:
// sin límite— y no admite campos de más, así que no vale mandar lo mismo que se leyó.
func (m *Mail) SetVacation(ctx context.Context, v MailVacation) error {
	fecha := func(t *time.Time) string {
		if t == nil {
			return ""
		}
		return t.UTC().Format(time.RFC3339)
	}
	return m.sinCuerpo(ctx, http.MethodPut, "/vacation", map[string]any{
		"enabled": v.Enabled, "subject": v.Subject, "body": v.Body, "starts_at": fecha(v.StartsAt), "ends_at": fecha(v.EndsAt),
	})
}
