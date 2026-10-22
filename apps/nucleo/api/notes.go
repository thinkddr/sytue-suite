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
	"sort"
	"strconv"
	"time"
)

// Notes es el cliente de Sytue Notepads: puerto de packages/api-clients/src/notes,
// contrastado con services/notes (que es quien manda: el escritorio Fyne leía `body`
// donde el servicio escribe `content`, y sus notas salían sin texto).
type Notes struct{ c *Client }

// NewNotes crea el cliente sobre la raíz del servicio (ServiceURL(apex, "notes")).
func NewNotes(servicio string, s Session, h *http.Client) *Notes {
	return &Notes{c: New(servicio+"/api/v1", s, h)}
}

// Notebook es un cuaderno.
type Notebook struct {
	DailyDate string `json:"daily_date,omitempty"`
	// ArchivedAt dice que está en el Archivo: fuera del árbol y de la búsqueda.
	ArchivedAt *time.Time `json:"archived_at,omitempty"`
	ID         string     `json:"id"`
	// OwnerSub es su dueño: el único que lo comparte con personas.
	OwnerSub   string  `json:"owner_sub,omitempty"`
	ParentID   *string `json:"parent_id,omitempty"`
	Title      string  `json:"title"`
	Emoji      string  `json:"emoji,omitempty"`
	Color      string  `json:"color"`
	Layout     string  `json:"layout"`
	Visibility string  `json:"visibility"`
	Indexable  bool    `json:"indexable"`
	Encrypted  bool    `json:"encrypted"`
	// Crypto son los parámetros de derivación de la clave (`CryptoParams` del
	// servicio). Viajan CRUDOS: este cliente no los interpreta, pero tiene que
	// devolverlos tal cual al guardar los ajustes o el cuaderno dejaría de estar
	// cifrado y sus notas no se podrían volver a leer.
	Crypto      json.RawMessage `json:"crypto,omitempty"`
	RoomID      string          `json:"room_id,omitempty"`
	PublicToken string          `json:"public_token,omitempty"`
	Position    int             `json:"position"`
	NoteCount   int             `json:"note_count"`
	UpdatedAt   time.Time       `json:"updated_at"`
	// ReadOnly: quien pregunta lo lee pero no escribe sus notas (un cuaderno público de
	// otra persona). EditRequested: ya lo ha pedido (la franja de solo lectura).
	ReadOnly      bool `json:"read_only,omitempty"`
	EditRequested bool `json:"edit_requested,omitempty"`
}

// Peticiones es la ruta de «pedir acceso de edición» de un cuaderno.
func (n *Notes) Peticiones(id string) Peticiones {
	return NuevasPeticiones(n.c, "/notebooks/"+url.PathEscape(id)+"/edit-requests")
}

// PersonasDeCuaderno es la ruta de «compartir con una persona» de un cuaderno (su dueño).
func (n *Notes) PersonasDeCuaderno(id string) Personas {
	return NuevasPersonas(n.c, "/notebooks/"+url.PathEscape(id)+"/people")
}

// PersonasDeNota es la de una nota suelta (quien la escribió).
func (n *Notes) PersonasDeNota(id string) Personas {
	return NuevasPersonas(n.c, "/notes/"+url.PathEscape(id)+"/people")
}

// NotaCompartida es una nota de «Compartido conmigo»: el nivel con que me la dieron y
// quién (su `sub`).
type NotaCompartida struct {
	Note
	Permiso string `json:"permission"`
	Por     string `json:"shared_by"`
}

// Compartidas son las notas sueltas que otras personas han compartido conmigo. Los
// cuadernos compartidos no van aquí: salen en ListNotebooks.
func (n *Notes) Compartidas(ctx context.Context) ([]NotaCompartida, error) {
	r, err := JSON[struct {
		Notes []NotaCompartida `json:"notes"`
	}](ctx, n.c, Request{Path: "/shared/notes"})
	return r.Notes, err
}

// NoteLink es un enlace de una nota a otra cosa de la suite.
type NoteLink struct {
	TargetKind string `json:"target_kind"`
	TargetID   string `json:"target_id"`
	Label      string `json:"label,omitempty"`
	URL        string `json:"url,omitempty"`
}

// Note es una nota. Content es HTML ya saneado por el servicio, o texto plano según el
// tipo; en un cuaderno cifrado viene vacío y lo que hay es ContentCipher.
type Note struct {
	ID string `json:"id"`
	// OwnerSub es quien la escribió: la única que la comparte con personas.
	OwnerSub      string          `json:"owner_sub,omitempty"`
	NotebookID    string          `json:"notebook_id"`
	Kind          string          `json:"kind"`
	Title         string          `json:"title,omitempty"`
	Content       string          `json:"content,omitempty"`
	ContentCipher string          `json:"content_cipher,omitempty"`
	Data          json.RawMessage `json:"data,omitempty"`
	DataJSON      string          `json:"data_json,omitempty"`
	X             int             `json:"x"`
	Y             int             `json:"y"`
	Col           int             `json:"col"`
	Position      int             `json:"position"`
	Width         int             `json:"width"`
	GroupID       *string         `json:"group_id,omitempty"`
	Collapsed     bool            `json:"collapsed"`
	Checked       bool            `json:"checked"`
	// CheckedAt es cuándo se completó; CancelledAt, cuándo se canceló (tachada y
	// fuera del traslado). Nunca las dos.
	CheckedAt   *time.Time  `json:"checked_at,omitempty"`
	CancelledAt *time.Time  `json:"cancelled_at,omitempty"`
	Pinned      bool        `json:"pinned"`
	Visibility  string      `json:"visibility,omitempty"`
	Indexable   *bool       `json:"indexable,omitempty"`
	States      []NoteState `json:"states,omitempty"`
	Links       []NoteLink  `json:"links,omitempty"`
	CreatedAt   time.Time   `json:"created_at"`
	UpdatedAt   time.Time   `json:"updated_at"`
	// RepeatedInto solo viene al editar una tarea: la siguiente que creó al marcarla
	// hecha, o la que retiró al desmarcarla. Si viene, se vuelven a pedir notas y
	// recordatorios.
	RepeatedInto *string `json:"repeated_into,omitempty"`
}

// NoteState es un estado de etiqueta marcado en una nota. El servicio lo manda con el
// nombre y el color dentro para que pintarlo no cueste una segunda consulta.
type NoteState struct {
	TagID   string `json:"tag_id"`
	TagName string `json:"tag_name"`
	StateID string `json:"state_id"`
	Name    string `json:"name"`
	Emoji   string `json:"emoji,omitempty"`
	Color   string `json:"color"`
}

// Tag es una etiqueta: un eje («Prioridad») con estados excluyentes entre sí.
type Tag struct {
	ID       string     `json:"id"`
	Name     string     `json:"name"`
	Position int        `json:"position"`
	States   []TagState `json:"states"`
}

// TagState es uno de los valores de una etiqueta.
type TagState struct {
	ID       string `json:"id"`
	TagID    string `json:"tag_id"`
	Name     string `json:"name"`
	Emoji    string `json:"emoji,omitempty"`
	Color    string `json:"color"`
	Shortcut string `json:"shortcut,omitempty"`
	Position int    `json:"position"`
}

// NoteInput es el cuerpo de crear y de actualizar. Los punteros son «no lo toques»; lo
// que NO es puntero se SUSTITUYE al actualizar —tipo, título, contenido, datos,
// enlaces—, así que una actualización parte siempre de Note.Input(), nunca de cero.
type NoteInput struct {
	CipherVersion  int             `json:"cipher_version,omitempty"`
	ExpectedCrypto json.RawMessage `json:"expected_crypto,omitempty"`
	Kind           string          `json:"kind,omitempty"`
	Title          string          `json:"title"`
	Content        string          `json:"content"`
	ContentCipher  string          `json:"content_cipher,omitempty"`
	Data           json.RawMessage `json:"data,omitempty"`
	X              *int            `json:"x,omitempty"`
	Y              *int            `json:"y,omitempty"`
	Col            *int            `json:"col,omitempty"`
	Position       *int            `json:"position,omitempty"`
	Width          *int            `json:"width,omitempty"`
	GroupID        *string         `json:"group_id,omitempty"`
	Collapsed      *bool           `json:"collapsed,omitempty"`
	Checked        *bool           `json:"checked,omitempty"`
	// Cancelled cancela (true) o reactiva (false) una tarea; nil la deja como está.
	Cancelled  *bool      `json:"cancelled,omitempty"`
	Pinned     *bool      `json:"pinned,omitempty"`
	Visibility string     `json:"visibility,omitempty"`
	Indexable  *bool      `json:"indexable,omitempty"`
	Links      []NoteLink `json:"links,omitempty"`
}

// ConClientID marca la entrada con `data.client_id`, conservando lo que ya traiga
// `data`. Con la misma marca, el servicio devuelve la nota que ya creó en vez de otra:
// es lo que deja reintentar una captura hecha sin conexión sin duplicarla.
func (in NoteInput) ConClientID(id string) NoteInput {
	d := map[string]any{}
	_ = json.Unmarshal(in.Data, &d) // un `data` que no es objeto se sustituye
	if d == nil {
		d = map[string]any{}
	}
	d["client_id"] = id
	in.Data, _ = json.Marshal(d)
	return in
}

// ClientID es el `data.client_id` de la entrada, o "".
func (in NoteInput) ClientID() string {
	var d struct {
		ClientID string `json:"client_id"`
	}
	_ = json.Unmarshal(in.Data, &d)
	return d.ClientID
}

// Input devuelve la nota ENTERA como entrada de actualización: se cambia lo que se haya
// editado y el resto vuelve tal cual. Mandar solo el título borraría el contenido.
func (n Note) Input() NoteInput {
	data, version := n.Data, 0
	if n.ContentCipher != "" {
		data, version = DatosPublicosNotas(data), 2
	}
	return NoteInput{
		CipherVersion: version,
		Kind:          n.Kind, Title: n.Title, Content: n.Content, ContentCipher: n.ContentCipher, Data: data,
		X: &n.X, Y: &n.Y, Col: &n.Col, Position: &n.Position, Width: &n.Width, GroupID: n.GroupID,
		Collapsed: &n.Collapsed, Checked: &n.Checked, Pinned: &n.Pinned,
		Visibility: n.Visibility, Indexable: n.Indexable, Links: n.Links,
	}
}

// NoteHit es un resultado de búsqueda.
type NoteHit struct {
	ID         string    `json:"id"`
	NotebookID string    `json:"notebook_id"`
	Notebook   string    `json:"notebook"`
	Title      string    `json:"title"`
	Snippet    string    `json:"snippet"`
	Kind       string    `json:"kind"`
	UpdatedAt  time.Time `json:"updated_at"`
	// Headings son los títulos de dentro de la nota, para enlazar a uno.
	Headings []string `json:"headings,omitempty"`
}

// ListNotebooks lista los cuadernos de la persona.
func (n *Notes) ListNotebooks(ctx context.Context) ([]Notebook, error) {
	r, err := JSON[struct {
		Notebooks []Notebook `json:"notebooks"`
	}](ctx, n.c, Request{Path: "/notebooks"})
	return r.Notebooks, err
}

// DailyNotebook abre el diario del día civil elegido. Carry traslada los pendientes
// independientes de diarios anteriores; los clientes solo lo piden para hoy.
func (n *Notes) DailyNotebook(ctx context.Context, date, title string, carry bool) (Notebook, error) {
	return PostJSON[Notebook](ctx, n.c, "/daily", map[string]any{
		"date": date, "title": title, "carry": carry,
	})
}

// Estados de una tarea en ListTasks.
const (
	TareaAbierta   = "open"
	TareaHecha     = "done"
	TareaCancelada = "cancelled"
)

// Tarea es una tarea de la vista «Tareas», con lo justo de su cuaderno.
type Tarea struct {
	Note
	Notebook  string `json:"notebook"`
	DailyDate string `json:"daily_date,omitempty"`
}

// ListTasks devuelve las tareas de quien pregunta en todos sus cuadernos, en un estado.
func (n *Notes) ListTasks(ctx context.Context, estado string) ([]Tarea, error) {
	r, err := JSON[struct {
		Tasks []Tarea `json:"tasks"`
	}](ctx, n.c, Request{Path: "/tasks", Query: url.Values{"state": {estado}}})
	return r.Tasks, err
}

// PostponeTask lleva la tarea, con lo que cuelga de ella, al diario de `date`.
func (n *Notes) PostponeTask(ctx context.Context, id, date, title string) (Notebook, error) {
	return PostJSON[Notebook](ctx, n.c, "/notes/"+url.PathEscape(id)+"/postpone", map[string]any{
		"date": date, "title": title,
	})
}

// DailyDay es un día del diario con sus cuentas (`GET /daily`). Notes solo viaja si se
// pidió (vistas Semana, Días y «Este día»).
type DailyDay struct {
	Date       string `json:"date"`
	NotebookID string `json:"notebook_id"`
	Total      int    `json:"total"`
	Pending    int    `json:"pending"`
	Notes      []Note `json:"notes,omitempty"`
}

// DailyRange son los días con notas de [from, to] (incluidos): los puntos de la barra
// de semana y, con notas, las vistas Semana y Días.
func (n *Notes) DailyRange(ctx context.Context, from, to string, notas bool) ([]DailyDay, error) {
	q := url.Values{"from": {from}, "to": {to}}
	if notas {
		q.Set("notes", "1")
	}
	r, err := JSON[struct {
		Days []DailyDay `json:"days"`
	}](ctx, n.c, Request{Path: "/daily", Query: q})
	return r.Days, err
}

// ThisDay es «Este día»: el mismo día y mes de años anteriores, del más reciente atrás.
func (n *Notes) ThisDay(ctx context.Context, date string) ([]DailyDay, error) {
	r, err := JSON[struct {
		Days []DailyDay `json:"days"`
	}](ctx, n.c, Request{Path: "/daily/this-day", Query: url.Values{"date": {date}}})
	return r.Days, err
}

// NotebookInput es lo que se manda al crear o al cambiar un cuaderno: el
// `NotebookInput` de services/notes, ENTERO.
//
// Entero porque el PUT del servicio SUSTITUYE. Hasta el 2026-09-22 aquí faltaban
// `encrypted`, `crypto` y `room_id`, así que guardar los ajustes de un cuaderno cifrado
// desde este cliente lo DESCIFRABA en el servidor —`encrypted` a false y `crypto` a
// vacío— y sus notas, que siguen siendo criptogramas, ya no las podía abrir nadie. Lo
// vigila `TestLosAjustesNoDescifranElCuaderno`.
//
// Cifrar un cuaderno NUEVO no se pide desde aquí: la clave se deriva en el cliente de
// quien lo cifra con el motor de criptografía de la web, que este cliente no tiene.
// Conservar el cifrado que ya hay, sí.
type NotebookInput struct {
	ParentID   *string         `json:"parent_id"`
	Title      string          `json:"title"`
	Emoji      string          `json:"emoji"`
	Color      string          `json:"color"`
	Layout     string          `json:"layout"`
	Visibility string          `json:"visibility"`
	Indexable  *bool           `json:"indexable"`
	Encrypted  bool            `json:"encrypted"`
	Crypto     json.RawMessage `json:"crypto,omitempty"`
	RoomID     string          `json:"room_id"`
	Position   *int            `json:"position,omitempty"`
}

// Input devuelve el cuaderno ENTERO como entrada de actualización, por lo mismo que
// Note.Input: el servicio sustituye, no parchea, y mandar solo el nombre le quitaría el
// color, el icono, la disposición y —lo grave— el cifrado.
func (nb Notebook) Input() NotebookInput {
	indexable := nb.Indexable
	return NotebookInput{
		ParentID: nb.ParentID, Title: nb.Title, Emoji: nb.Emoji, Color: nb.Color,
		Layout: nb.Layout, Visibility: nb.Visibility, Indexable: &indexable,
		Encrypted: nb.Encrypted, Crypto: nb.Crypto, RoomID: nb.RoomID,
	}
}

// CreateNotebook crea un cuaderno.
func (n *Notes) CreateNotebook(ctx context.Context, in NotebookInput) (Notebook, error) {
	return PostJSON[Notebook](ctx, n.c, "/notebooks", in)
}

// UpdateNotebook sustituye un cuaderno. `in` sale de Notebook.Input().
func (n *Notes) UpdateNotebook(ctx context.Context, id string, in NotebookInput) (Notebook, error) {
	cuerpo, err := json.Marshal(in)
	if err != nil {
		return Notebook{}, err
	}
	return JSON[Notebook](ctx, n.c, Request{
		Method: http.MethodPut, Path: "/notebooks/" + url.PathEscape(id), Body: cuerpo,
		Header: http.Header{"Content-Type": {"application/json"}},
	})
}

// DeleteNotebook manda el cuaderno y su rama a la papelera.
func (n *Notes) DeleteNotebook(ctx context.Context, id string) error {
	return n.sinCuerpo(ctx, http.MethodDelete, "/notebooks/"+url.PathEscape(id), nil)
}

// ArchiveNotebook archiva el cuaderno con su rama (true) o la devuelve al árbol
// (false). Solo su dueño; un diario no se archiva.
func (n *Notes) ArchiveNotebook(ctx context.Context, id string, archivar bool) error {
	metodo := http.MethodPost
	if !archivar {
		metodo = http.MethodDelete
	}
	return n.sinCuerpo(ctx, metodo, "/notebooks/"+url.PathEscape(id)+"/archive", nil)
}

// PinnedNotes son las notas fijadas de todos los cuadernos visibles (vista «Fijadas»),
// con la forma de un resultado de búsqueda.
func (n *Notes) PinnedNotes(ctx context.Context) ([]NoteHit, error) {
	r, err := JSON[struct {
		Notes []NoteHit `json:"notes"`
	}](ctx, n.c, Request{Path: "/notes/pinned"})
	return r.Notes, err
}

// FormatosExport son los de services/notes (`domain.FormatoDe`), en el orden en que la
// web los ofrece: el nativo de Notepads es el HTML, y los demás son salidas hacia fuera.
var FormatosExport = []string{"md", "txt", "html", "onex", "pdf"}

// ExportNotebook baja un cuaderno —o una sola de sus notas, con `nota`— en uno de los
// formatos de FormatosExport. Devuelve el cuerpo SIN LEER y el nombre que propone el
// servidor; quien llama lo cierra.
//
// Exportar es un DERECHO (RGPD art. 20) y por eso está en la barra y no escondido en
// los ajustes, igual que en la web.
func (n *Notes) ExportNotebook(ctx context.Context, cuaderno, nota, formato string) (io.ReadCloser, string, error) {
	q := url.Values{"format": {formato}}
	if nota != "" {
		q.Set("note", nota)
	}
	// no-store: una exportación no va a la caché «red primero» (en claro y hasta 50 MB),
	// ni se sirve una vieja sin red como si fuera nueva.
	resp, err := n.c.Do(ctx, Request{
		Path: "/notebooks/" + url.PathEscape(cuaderno) + "/export", Query: q,
		Header: http.Header{"Cache-Control": {"no-store"}},
	})
	if err != nil {
		return nil, "", err
	}
	return resp.Body, nombreAdjunto(resp.Header.Get("Content-Disposition"), "cuaderno."+formato), nil
}

// ListNotes lista las notas de un cuaderno.
func (n *Notes) ListNotes(ctx context.Context, cuaderno string) ([]Note, error) {
	return n.listNotes(ctx, cuaderno, nil)
}

// ListNotesForEncryption exige lectura fresca antes de un CAS de contenido.
func (n *Notes) ListNotesForEncryption(ctx context.Context, cuaderno string) ([]Note, error) {
	return n.listNotes(ctx, cuaderno, http.Header{"Cache-Control": {"no-store"}})
}

func (n *Notes) listNotes(ctx context.Context, cuaderno string, header http.Header) ([]Note, error) {
	var notes []Note
	cursor := ""
	seen := map[string]bool{}
	legacyRead := false
	for {
		path := "/notebooks/" + url.PathEscape(cuaderno) + "/notes/page"
		limit := "100"
		if legacyRead {
			path = "/notebooks/" + url.PathEscape(cuaderno) + "/notes"
			limit = "1"
		}
		query := url.Values{"limit": {limit}, "raw_data": {"1"}}
		if cursor != "" {
			query.Set("cursor", cursor)
		}
		page, err := JSON[struct {
			Notes      []Note `json:"notes"`
			NextCursor string `json:"next_cursor"`
		}](ctx, n.c, Request{Path: path, Query: query, Header: header})
		if err != nil {
			var response *Error
			if !legacyRead && errors.As(err, &response) && response.Status == http.StatusNotFound {
				// Restart instead of mixing modern pages with an older backend
				// that might ignore the cursor and return the whole legacy list.
				legacyRead, notes, cursor, seen = true, nil, "", map[string]bool{}
				continue
			}
			return nil, err
		}
		if err := restoreNotePageData(page.Notes); err != nil {
			return nil, err
		}
		notes = append(notes, page.Notes...)
		if page.NextCursor == "" {
			break
		}
		if seen[page.NextCursor] || len(page.Notes) == 0 {
			return nil, errors.New("api: invalid notes cursor")
		}
		seen[page.NextCursor] = true
		cursor = page.NextCursor
	}
	sort.SliceStable(notes, func(i, j int) bool {
		a, b := notes[i], notes[j]
		if a.Pinned != b.Pinned {
			return a.Pinned
		}
		if a.Col != b.Col {
			return a.Col < b.Col
		}
		if a.Position != b.Position {
			return a.Position < b.Position
		}
		if !a.CreatedAt.Equal(b.CreatedAt) {
			return a.CreatedAt.Before(b.CreatedAt)
		}
		return a.ID < b.ID
	})
	return notes, nil
}

func restoreNotePageData(notes []Note) error {
	for i := range notes {
		if raw := notes[i].DataJSON; raw != "" {
			if !json.Valid([]byte(raw)) {
				return errors.New("api: invalid note data JSON")
			}
			notes[i].Data = json.RawMessage(raw)
			notes[i].DataJSON = ""
		}
	}
	return nil
}

// GetNotebook obtiene parámetros actuales antes de escribir desde la vista de tareas.
func (n *Notes) GetNotebook(ctx context.Context, id string) (Notebook, error) {
	return JSON[Notebook](ctx, n.c, Request{Path: "/notebooks/" + url.PathEscape(id), Header: http.Header{"Cache-Control": {"no-store"}}})
}

// CreateNote crea una nota en un cuaderno.
func (n *Notes) CreateNote(ctx context.Context, cuaderno string, in NoteInput) (Note, error) {
	path := "/notebooks/" + url.PathEscape(cuaderno) + "/notes"
	if in.encryptedWrite() {
		path = "/notebooks/" + url.PathEscape(cuaderno) + "/encrypted-notes"
	}
	return PostJSON[Note](ctx, n.c, path, in)
}

// Older servers ignore encrypted-write preconditions on their legacy endpoints.
// A separate route fails closed with 404 instead of accepting an unsafe write;
// even an incomplete encrypted input must never fall back to a plaintext route.
func (in NoteInput) encryptedWrite() bool {
	return in.CipherVersion != 0 || len(in.ExpectedCrypto) != 0 || in.ContentCipher != ""
}

// UpdateNote sustituye una nota. `in` sale de Note.Input().
func (n *Notes) UpdateNote(ctx context.Context, id, cuaderno string, in NoteInput) (Note, error) {
	cuerpo, err := json.Marshal(struct {
		NoteInput
		NotebookID string `json:"notebook_id"`
	}{in, cuaderno})
	if err != nil {
		return Note{}, err
	}
	path := "/notes/" + url.PathEscape(id)
	if in.encryptedWrite() {
		path += "/encrypted"
	}
	return JSON[Note](ctx, n.c, Request{
		Method: http.MethodPut, Path: path, Body: cuerpo,
		Header: http.Header{"Content-Type": {"application/json"}},
	})
}

// DeleteNote manda una nota a la papelera.
func (n *Notes) DeleteNote(ctx context.Context, id string) error {
	return n.sinCuerpo(ctx, http.MethodDelete, "/notes/"+url.PathEscape(id), nil)
}

// RestoreNote la saca de la papelera y la devuelve a su cuaderno.
func (n *Notes) RestoreNote(ctx context.Context, id string) error {
	return n.sinCuerpo(ctx, http.MethodPost, "/notes/"+url.PathEscape(id)+"/restore", nil)
}

// PurgeNote la borra para siempre. No hay vuelta atrás: quien la llame pide confirmación.
func (n *Notes) PurgeNote(ctx context.Context, id string) error {
	return n.sinCuerpo(ctx, http.MethodDelete, "/notes/"+url.PathEscape(id)+"/purge", nil)
}

// MoveNoteToNotebook cambia la nota de cuaderno (con las que tenga agrupadas). El destino
// decide dónde cae, así que NO se manda posición: mandarla la colocaría encima de otra.
func (n *Notes) MoveNoteToNotebook(ctx context.Context, id, cuaderno string) error {
	cuerpo, err := json.Marshal(map[string]string{"notebook_id": cuaderno})
	if err != nil {
		return err
	}
	return n.sinCuerpo(ctx, http.MethodPost, "/notes/"+url.PathEscape(id)+"/move", cuerpo)
}

// NotePosition es dónde queda una nota dentro de SU cuaderno: el cuerpo de `moveNote` de
// la web. `GroupID` nil la saca de la nota que la agrupaba, así que quien solo la cambie
// de sitio manda el que ya tenía: el servicio lee el campo ausente igual que un null.
type NotePosition struct {
	X        int     `json:"x"`
	Y        int     `json:"y"`
	Col      int     `json:"col"`
	Position int     `json:"position"`
	GroupID  *string `json:"group_id"`
}

// MoveNote coloca una nota dentro de su cuaderno: arrastrarla, cambiarla de columna,
// meterla dentro de otra o sacarla de su grupo.
func (n *Notes) MoveNote(ctx context.Context, id string, p NotePosition) error {
	cuerpo, err := json.Marshal(p)
	if err != nil {
		return err
	}
	return n.sinCuerpo(ctx, http.MethodPost, "/notes/"+url.PathEscape(id)+"/move", cuerpo)
}

// SetNoteState marca un estado de etiqueta en una nota; con `estado` vacío lo quita. Los
// estados de una misma etiqueta son excluyentes: poner uno retira el anterior.
func (n *Notes) SetNoteState(ctx context.Context, nota, etiqueta, estado string) error {
	in := struct {
		TagID   string  `json:"tag_id"`
		StateID *string `json:"state_id"`
	}{TagID: etiqueta}
	if estado != "" {
		in.StateID = &estado
	}
	cuerpo, err := json.Marshal(in)
	if err != nil {
		return err
	}
	return n.sinCuerpo(ctx, http.MethodPost, "/notes/"+url.PathEscape(nota)+"/state", cuerpo)
}

// sinCuerpo hace una petición cuya respuesta no se mira (204 No Content) y cierra el
// cuerpo: sin cerrarlo la conexión no vuelve al pool y se abre otra en cada gesto.
func (n *Notes) sinCuerpo(ctx context.Context, metodo, ruta string, cuerpo []byte) error {
	r := Request{Method: metodo, Path: ruta, Body: cuerpo}
	if cuerpo != nil {
		r.Header = http.Header{"Content-Type": {"application/json"}}
	}
	resp, err := n.c.Do(ctx, r)
	if err != nil {
		return err
	}
	return resp.Body.Close()
}

// ListTrash lista la papelera de la persona.
func (n *Notes) ListTrash(ctx context.Context, limite int) ([]Note, error) {
	r, err := JSON[struct {
		Notes []Note `json:"notes"`
	}](ctx, n.c, Request{Path: "/notes/trash", Query: url.Values{"limit": {strconv.Itoa(limite)}}})
	return r.Notes, err
}

// ListTags lista las etiquetas de la persona, con sus estados.
func (n *Notes) ListTags(ctx context.Context) ([]Tag, error) {
	r, err := JSON[struct {
		Tags []Tag `json:"tags"`
	}](ctx, n.c, Request{Path: "/tags"})
	return r.Tags, err
}

// TagStateInput es un estado propuesto al guardar una etiqueta. `ID` nil es un estado
// NUEVO; con el suyo, se conserva —y con él las notas que ya lo llevan—.
type TagStateInput struct {
	ID       *string `json:"id,omitempty"`
	Name     string  `json:"name"`
	Emoji    string  `json:"emoji"`
	Color    string  `json:"color"`
	Shortcut string  `json:"shortcut"`
}

// TagInput es el cuerpo de crear o cambiar una etiqueta con TODOS sus estados: el que
// no venga se borra.
type TagInput struct {
	Name   string          `json:"name"`
	States []TagStateInput `json:"states"`
}

// SaveTag crea una etiqueta (`id` vacío) o la sustituye.
func (n *Notes) SaveTag(ctx context.Context, id string, in TagInput) (Tag, error) {
	if id == "" {
		return PostJSON[Tag](ctx, n.c, "/tags", in)
	}
	cuerpo, err := json.Marshal(in)
	if err != nil {
		return Tag{}, err
	}
	return JSON[Tag](ctx, n.c, Request{
		Method: http.MethodPut, Path: "/tags/" + url.PathEscape(id), Body: cuerpo,
		Header: http.Header{"Content-Type": {"application/json"}},
	})
}

// DeleteTag borra una etiqueta, sus estados y sus marcas en todas las notas.
func (n *Notes) DeleteTag(ctx context.Context, id string) error {
	return n.sinCuerpo(ctx, http.MethodDelete, "/tags/"+url.PathEscape(id), nil)
}

// LinkedNotes son las notas que cuelgan de un objeto de la suite. Con kind "note" es la
// VUELTA del enlace —qué apunta a esta nota—, que es lo que enseña el panel de
// retroenlaces: un enlace que solo se ve desde el extremo que lo creó no es una red.
func (n *Notes) LinkedNotes(ctx context.Context, kind, id string, limite int) ([]Note, error) {
	// JSON y no Consulta: lo que viaja son dos identificadores, no un término de
	// búsqueda que pueda pasarse de largo en la URL.
	r, err := JSON[struct {
		Notes []Note `json:"notes"`
	}](ctx, n.c, Request{Path: "/notes/linked", Query: url.Values{
		"target_kind": {kind}, "target_id": {id}, "limit": {strconv.Itoa(limite)},
	}})
	return r.Notes, err
}

// Search busca en las notas de la persona.
func (n *Notes) Search(ctx context.Context, q string, limite int, archivados bool) ([]NoteHit, error) {
	consulta := url.Values{"q": {q}, "limit": {strconv.Itoa(limite)}}
	if archivados {
		consulta.Set("archived", "1") // lo archivado solo sale si se pide
	}
	r, err := Consulta[struct {
		Notes []NoteHit `json:"notes"`
	}](ctx, n.c, Request{Path: "/notes/search", Query: consulta})
	return r.Notes, err
}

// Hashtag es un `#hashtag` escrito en las notas (sin el `#`) y cuántas notas lo llevan.
type Hashtag struct {
	Tag   string `json:"tag"`
	Count int    `json:"count"`
}

// Hashtags son los hashtags de las notas visibles, del más usado al menos. Las notas
// de uno se piden a Search con `#palabra`.
func (n *Notes) Hashtags(ctx context.Context) ([]Hashtag, error) {
	r, err := JSON[struct {
		Hashtags []Hashtag `json:"hashtags"`
	}](ctx, n.c, Request{Path: "/hashtags"})
	return r.Hashtags, err
}

// VistaPreviaVolcado es lo que ENTRARÍA al importar un `.onex`: el cuaderno y las notas
// ya saneados como quedarían, y cuántas se quedarían fuera. No crea nada.
type VistaPreviaVolcado struct {
	Notebook    Notebook `json:"notebook"`
	Notas       []Note   `json:"notas"`
	Descartadas int      `json:"descartadas"`
}

// Importacion es lo que devuelve importar: el cuaderno NUEVO y el recuento.
type Importacion struct {
	Notebook    Notebook `json:"notebook"`
	Importadas  int      `json:"importadas"`
	Descartadas int      `json:"descartadas"`
}

// MaxVolcado es el tope del servicio para un `.onex` (32 MiB): se comprueba antes de
// mandarlo para no subir lo que va a rechazar.
const MaxVolcado = 32 << 20

// PreviewNotebook lee un `.onex` y devuelve lo que entraría, sin crear nada: el ensayo
// de la importación. Comparte límite y lectura con ImportNotebook.
func (n *Notes) PreviewNotebook(ctx context.Context, volcado []byte) (VistaPreviaVolcado, error) {
	return JSON[VistaPreviaVolcado](ctx, n.c, Request{
		Method: http.MethodPost, Path: "/notebooks/preview", Body: volcado,
		Header: http.Header{"Content-Type": {"application/json"}},
	})
}

// ImportNotebook crea un cuaderno NUEVO a partir de un `.onex`. No sobrescribe ninguno.
func (n *Notes) ImportNotebook(ctx context.Context, volcado []byte) (Importacion, error) {
	return JSON[Importacion](ctx, n.c, Request{
		Method: http.MethodPost, Path: "/notebooks/import", Body: volcado,
		Header: http.Header{"Content-Type": {"application/json"}},
	})
}

// NoteCipherMigration sustituye únicamente el contenido bajo CAS, sin repetir una
// escritura incierta ni sobrescribir cambios de otro cliente.
type NoteCipherMigration struct {
	CipherVersion     int             `json:"cipher_version"`
	ExpectedUpdatedAt time.Time       `json:"expected_updated_at"`
	ExpectedCrypto    json.RawMessage `json:"expected_crypto"`
	ContentCipher     string          `json:"content_cipher"`
	Data              json.RawMessage `json:"data,omitempty"`
}

func (n *Notes) MigrateEncryptedContent(ctx context.Context, id string, in NoteCipherMigration) (Note, error) {
	return PostJSON[Note](ctx, n.c, "/notes/"+url.PathEscape(id)+"/encrypted-content", in)
}
