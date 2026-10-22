// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package noteshttp

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/hubnotify"
	"git.sytue.eu/sytue/one/platform/pkg/log"
	"git.sytue.eu/sytue/one/services/notes/internal/config"
	"git.sytue.eu/sytue/one/services/notes/internal/store"

	"git.sytue.eu/sytue/one/services/notes/internal/domain"
)

// Handlers sirve la API REST de Sytue Notepads sobre el store.
type Handlers struct {
	store *store.Store
	cfg   config.Config
	// hub avisa de las peticiones de edición (peticiones.go). nil: sin aviso.
	hub *hubnotify.Client
	// avisados frena el aviso de «compartido contigo»: uno por recurso y persona cada
	// VentanaAviso, por mucho que se retire y se vuelva a compartir (personas.go).
	avisados *hubnotify.UnaVez
}

// sub devuelve el sujeto del token. RequireAuth garantiza que hay claims, así que
// una cadena vacía aquí solo puede venir de un token sin `sub`, que no autoriza
// nada: todas las consultas lo comparan contra owner_sub.
func sub(r *http.Request) string {
	if c, ok := authn.FromContext(r.Context()); ok {
		return c.Subject
	}
	return ""
}

// traducir mapea los errores de dominio al sobre canónico de la API. Un recurso
// que no existe y uno al que no se tiene acceso se contestan IGUAL (404): decir
// «existe pero no puedes» ya filtra la existencia.
func traducir(err error) error {
	switch {
	case errors.Is(err, domain.ErrNotFound), errors.Is(err, domain.ErrForbidden):
		return httpx.ErrNotFound.Wrap(err)
	case errors.Is(err, domain.ErrSoloDueno):
		return httpx.ErrForbidden.WithCode("notebook_owner_only").
			WithMessage("Only the notebook's owner or an organisation administrator can move, publish, hide or encrypt it.").Wrap(err)
	case errors.Is(err, domain.ErrEncrypted):
		return httpx.ErrBadRequest.WithCode("notebook_encrypted").
			WithMessage("This notebook is encrypted; content must be sent encrypted.").Wrap(err)
	case errors.Is(err, domain.ErrEncryptedClientUpgrade):
		return httpx.NewError(http.StatusConflict, "encrypted_client_upgrade_required", "Update the application before editing encrypted notes.").Wrap(err)
	case errors.Is(err, domain.ErrBackendWriteUpgrade):
		return httpx.NewError(http.StatusServiceUnavailable, "notes_backend_upgrade_required", "Notes needs a backend update before changes can be saved. Your existing notes are preserved.").Wrap(err)
	case errors.Is(err, domain.ErrEncryptedMigrationRequired):
		return httpx.NewError(http.StatusConflict, "encrypted_content_migration_required", "Unlock and migrate the encrypted note before editing it.").Wrap(err)
	case errors.Is(err, domain.ErrNoteChanged):
		return httpx.NewError(http.StatusConflict, "note_changed", "The note changed. Reload it before migrating.").Wrap(err)
	case errors.Is(err, domain.ErrNotebookCryptoChanged):
		return httpx.NewError(http.StatusConflict, "notebook_crypto_changed", "The notebook encryption changed. Unlock it again before migrating.").Wrap(err)
	case errors.Is(err, domain.ErrNotePageTooLarge):
		return httpx.NewError(http.StatusRequestEntityTooLarge, "note_page_too_large", "A note exceeds the response size limit. Export the original or reduce its attachments before retrying.").Wrap(err)
	case errors.Is(err, domain.ErrNotePaginationRequired):
		return httpx.NewError(http.StatusConflict, "note_pagination_required", "Update the application to read this notebook in bounded pages.").Wrap(err)
	case errors.Is(err, domain.ErrInvalidInput):
		return httpx.ErrBadRequest.Wrap(err)
	default:
		return err
	}
}

// idDeRuta lee el `{id}` de la ruta. Sin parámetro para el nombre del segmento:
// las diez llamadas pasaban "id", así que era una flexibilidad que solo obligaba a
// mirar cada llamada para saber qué se estaba leyendo.
func idDeRuta(r *http.Request) (uuid.UUID, error) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		return uuid.Nil, httpx.ErrBadRequest.WithCode("invalid_id").Wrap(err)
	}
	return id, nil
}

// ---------------------------------------------------------------------------
// Cuadernos
// ---------------------------------------------------------------------------

// ListNotebooks devuelve el árbol completo visible para quien pregunta.
func (h *Handlers) ListNotebooks(w http.ResponseWriter, r *http.Request) {
	nbs, err := h.store.ListNotebooks(r.Context(), sub(r))
	if err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}
	if nbs == nil {
		nbs = []domain.Notebook{}
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"notebooks": nbs})
}

// CreateNotebook crea un cuaderno.
func (h *Handlers) CreateNotebook(w http.ResponseWriter, r *http.Request) {
	var in domain.NotebookInput
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if err := in.Normalize(); err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}
	nb, err := h.store.CreateNotebook(r.Context(), sub(r), in)
	if err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}
	httpx.JSON(w, r, http.StatusCreated, nb)
}

// GetNotebook devuelve un cuaderno.
func (h *Handlers) GetNotebook(w http.ResponseWriter, r *http.Request) {
	id, err := idDeRuta(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	nb, err := h.store.GetNotebook(r.Context(), id, sub(r))
	if err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, nb)
}

// UpdateNotebook actualiza un cuaderno (incluida su visibilidad y su cifrado).
func (h *Handlers) UpdateNotebook(w http.ResponseWriter, r *http.Request) {
	id, err := idDeRuta(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var in domain.NotebookInput
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if err := in.Normalize(); err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}
	// Quien administra la organización (roles `owner` o `admin`) decide, como su dueño,
	// dónde cuelga y quién ve un cuaderno de organización.
	c, _ := authn.FromContext(r.Context())
	nb, err := h.store.ActualizarCuaderno(r.Context(), id, sub(r), c.HasAnyRole("owner", "admin"), in)
	if err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, nb)
}

// DeleteNotebook manda el cuaderno (y su rama) a la papelera.
func (h *Handlers) DeleteNotebook(w http.ResponseWriter, r *http.Request) {
	id, err := idDeRuta(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if err := h.store.DeleteNotebook(r.Context(), id, sub(r)); err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}
	httpx.NoContent(w)
}

// ---------------------------------------------------------------------------
// Notas
// ---------------------------------------------------------------------------

// ListNotes devuelve las notas de un cuaderno.
func (h *Handlers) ListNotes(w http.ResponseWriter, r *http.Request) {
	id, err := idDeRuta(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if r.URL.Query().Has("limit") || r.URL.Query().Has("cursor") || r.URL.Query().Has("raw_data") {
		h.listNotesPage(w, r, id)
		return
	}
	page, err := h.store.ListNotesLegacyBounded(r.Context(), id, sub(r))
	if err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}
	h.writeNotePage(w, r, page, false)
}

// CreateNote crea una nota dentro de un cuaderno.
func (h *Handlers) CreateNote(w http.ResponseWriter, r *http.Request) {
	h.createNote(w, r, false)
}

// CreateEncryptedNote is a write-version gate: older backends do not expose it.
func (h *Handlers) CreateEncryptedNote(w http.ResponseWriter, r *http.Request) {
	h.createNote(w, r, true)
}

func (h *Handlers) createNote(w http.ResponseWriter, r *http.Request, encryptedOnly bool) {
	id, err := idDeRuta(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var in domain.NoteInput
	if err := httpx.DecodeJSONLimit(w, r, &in, domain.MaxNoteRequestBody); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if encryptedOnly && in.CipherVersion != domain.EncryptedNoteWriteVersion {
		httpx.Fail(w, r, traducir(domain.ErrEncryptedClientUpgrade))
		return
	}
	// El cuaderno decide la forma del contenido: si está cifrado, el texto en claro
	// se rechaza. Se lee ANTES de normalizar porque es lo que fija la regla.
	nb, err := h.store.GetNotebook(r.Context(), id, sub(r))
	if err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}
	if encryptedOnly && !nb.Encrypted {
		httpx.Fail(w, r, traducir(domain.ErrNotebookCryptoChanged))
		return
	}
	if err := in.Normalize(nb.Encrypted); err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}
	n, err := h.store.CreateNote(r.Context(), id, sub(r), in)
	if err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}
	h.avisarMenciones(r, n, "")
	httpx.JSON(w, r, http.StatusCreated, n)
}

// UpdateNote actualiza una nota.
func (h *Handlers) UpdateNote(w http.ResponseWriter, r *http.Request) {
	h.updateNote(w, r, false)
}

// UpdateEncryptedNote never falls back to an unversioned write handler.
func (h *Handlers) UpdateEncryptedNote(w http.ResponseWriter, r *http.Request) {
	h.updateNote(w, r, true)
}

func (h *Handlers) updateNote(w http.ResponseWriter, r *http.Request, encryptedOnly bool) {
	id, err := idDeRuta(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var body struct {
		domain.NoteInput
		// NotebookID lo siguen mandando los clientes y ya no se usa: el cifrado se lee
		// del cuaderno de la NOTA (EscrituraDeNota), no del que diga el cliente. Quien
		// la tiene solo por compartida (0010) no puede leer ese cuaderno.
		NotebookID uuid.UUID `json:"notebook_id"`
	}
	if err := httpx.DecodeJSONLimit(w, r, &body, domain.MaxNoteRequestBody); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if encryptedOnly && body.CipherVersion != domain.EncryptedNoteWriteVersion {
		httpx.Fail(w, r, traducir(domain.ErrEncryptedClientUpgrade))
		return
	}
	cifrado, tipo, porCuaderno, err := h.store.EscrituraDeNota(r.Context(), id, sub(r))
	if err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}
	if encryptedOnly && !cifrado {
		httpx.Fail(w, r, traducir(domain.ErrNotebookCryptoChanged))
		return
	}
	if !porCuaderno {
		// Quien la tiene solo por compartida no cambia el tipo: se valida —y se sanea—
		// con el que tiene, que es con el que se va a pintar.
		body.Kind = tipo
	}
	if err := body.Normalize(cifrado); err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}
	n, antes, err := h.store.ActualizarNota(r.Context(), id, sub(r), body.NoteInput)
	if err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}
	// Una tarea que repite, recién hecha, deja preparada la siguiente; desmarcarla la
	// retira si nadie la ha tocado (recordatorios.go). Solo en el PASO: reenviar
	// `checked: true` al cambiar el texto no crea otra. La nota ya está guardada: si
	// esto falla, se anota y la respuesta sigue siendo la de la nota, que es lo pedido.
	h.avisarMenciones(r, n, textoDeMenciones(antes.Titulo, antes.Contenido))
	if n.Kind == domain.KindTodo && antes.Checked != n.Checked {
		if n.Checked {
			sig, err := h.store.RepetirAlCompletar(r.Context(), id, time.Now())
			if err != nil {
				log.From(r.Context()).Warn("repetir la tarea completada", "error", err)
			} else if sig != nil {
				n.RepeatedInto = &sig.ID
			}
		} else if n.RepeatedInto, err = h.store.DeshacerRepeticion(r.Context(), id); err != nil {
			log.From(r.Context()).Warn("deshacer la repetición de la tarea", "error", err)
		}
	}
	httpx.JSON(w, r, http.StatusOK, n)
}

// MoveNote reubica una nota. Endpoint aparte del de edición porque arrastrar es
// la escritura más frecuente del producto y no tiene por qué reenviar el
// contenido —ni volver a saneárselo— en cada soltar.
func (h *Handlers) MoveNote(w http.ResponseWriter, r *http.Request) {
	id, err := idDeRuta(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var in struct {
		X        int        `json:"x"`
		Y        int        `json:"y"`
		Col      int        `json:"col"`
		Position int        `json:"position"`
		GroupID  *uuid.UUID `json:"group_id"`
		// Con `notebook_id` la nota cambia de CUADERNO (y solo eso): la posición
		// la decide el destino. Sin él, es el movimiento de siempre dentro del suyo.
		NotebookID *uuid.UUID `json:"notebook_id"`
	}
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if in.NotebookID != nil {
		if err := h.store.MoveNoteToNotebook(r.Context(), id, sub(r), *in.NotebookID); err != nil {
			httpx.Fail(w, r, traducir(err))
			return
		}
		httpx.NoContent(w)
		return
	}
	if err := h.store.MoveNote(r.Context(), id, sub(r), in.X, in.Y, in.Col, in.Position, in.GroupID); err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}
	httpx.NoContent(w)
}

// DeleteNote manda la nota a la papelera.
func (h *Handlers) DeleteNote(w http.ResponseWriter, r *http.Request) {
	h.papelera(w, r, h.store.DeleteNote)
}

// RestoreNote la saca de la papelera.
func (h *Handlers) RestoreNote(w http.ResponseWriter, r *http.Request) {
	h.papelera(w, r, h.store.RestoreNote)
}

// PurgeNote la borra definitivamente.
func (h *Handlers) PurgeNote(w http.ResponseWriter, r *http.Request) {
	h.papelera(w, r, h.store.PurgeNote)
}

func (h *Handlers) papelera(w http.ResponseWriter, r *http.Request, fn func(context.Context, uuid.UUID, string) error) {
	id, err := idDeRuta(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if err := fn(r.Context(), id, sub(r)); err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}
	httpx.NoContent(w)
}

// ListTrash lista la papelera de quien pregunta.
func (h *Handlers) ListTrash(w http.ResponseWriter, r *http.Request) {
	notes, err := h.store.ListTrash(r.Context(), sub(r), limiteDeQuery(r, 100))
	if err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}
	if notes == nil {
		notes = []domain.Note{}
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"notes": notes})
}

// ---------------------------------------------------------------------------
// Búsqueda y referencias cruzadas
// ---------------------------------------------------------------------------

// Search busca en las notas indexables visibles. Sin `q`, devuelve las recientes:
// es lo que alimenta tanto el carril de actividad del Hub como la vista de inicio.
func (h *Handlers) Search(w http.ResponseWriter, r *http.Request) {
	// De `httpx.Consulta` y no de la URL: con QUERY —lo que manda el cliente nativo—
	// el término viaja en el CUERPO, y leyendo la URL la búsqueda llegaba vacía y
	// contestaba con lo reciente.
	p, err := httpx.Consulta(w, r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	q := p.Get("q")
	limit := limiteDe(p.Get("limit"), 20)
	// `archived=1` es el filtro opcional de la app; sin él lo archivado no sale.
	archivados := p.Get("archived") == "1"
	var hits []store.Hit
	if q == "" {
		hits, err = h.store.RecentNotes(r.Context(), sub(r), limit, archivados)
	} else {
		hits, err = h.store.SearchNotes(r.Context(), sub(r), q, limit, archivados)
	}
	if err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}
	if hits == nil {
		hits = []store.Hit{}
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"notes": hits})
}

// Hashtags lista los `#hashtags` de las notas visibles con cuántas notas lleva cada
// uno. Para ver las de uno se busca `#palabra` en Search: no hay ruta aparte.
func (h *Handlers) Hashtags(w http.ResponseWriter, r *http.Request) {
	tags, err := h.store.Hashtags(r.Context(), sub(r))
	if err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"hashtags": tags})
}

// LinkedNotes es el OTRO EXTREMO de la referencia bidireccional: dado un objeto de
// la suite —`?target_kind=calendar_event&target_id=<uuid>`— devuelve las notas que
// cuelgan de él. Lo consume la ficha del evento en Sytue Calendar con el token de
// quien mira, así que la autorización sigue siendo la de Notes.
func (h *Handlers) LinkedNotes(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	notes, err := h.store.NotesForTarget(r.Context(), sub(r),
		q.Get("target_kind"), q.Get("target_id"), limiteDeQuery(r, 50))
	if err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}
	if notes == nil {
		notes = []domain.Note{}
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"notes": notes})
}

// ---------------------------------------------------------------------------
// Etiquetas
// ---------------------------------------------------------------------------

// ListTags devuelve las etiquetas de quien pregunta, con sus estados.
func (h *Handlers) ListTags(w http.ResponseWriter, r *http.Request) {
	tags, err := h.store.ListTags(r.Context(), sub(r))
	if err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}
	if tags == nil {
		tags = []domain.Tag{}
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"tags": tags})
}

// SaveTag crea (POST) o actualiza (PUT /{id}) una etiqueta con sus estados.
func (h *Handlers) SaveTag(w http.ResponseWriter, r *http.Request) {
	var id *uuid.UUID
	if raw := chi.URLParam(r, "id"); raw != "" {
		parsed, err := uuid.Parse(raw)
		if err != nil {
			httpx.Fail(w, r, httpx.ErrBadRequest.WithCode("invalid_id").Wrap(err))
			return
		}
		id = &parsed
	}
	var in domain.TagInput
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if err := in.Normalize(); err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}
	tag, err := h.store.SaveTag(r.Context(), sub(r), id, in)
	if err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, tag)
}

// DeleteTag borra una etiqueta con sus estados y marcas.
func (h *Handlers) DeleteTag(w http.ResponseWriter, r *http.Request) {
	id, err := idDeRuta(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if err := h.store.DeleteTag(r.Context(), id, sub(r)); err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}
	httpx.NoContent(w)
}

// SetNoteState marca (o quita, con `state_id` nulo) un estado en una nota.
func (h *Handlers) SetNoteState(w http.ResponseWriter, r *http.Request) {
	id, err := idDeRuta(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var in struct {
		TagID   uuid.UUID  `json:"tag_id"`
		StateID *uuid.UUID `json:"state_id"`
	}
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if err := h.store.SetNoteState(r.Context(), id, in.TagID, in.StateID, sub(r)); err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}
	httpx.NoContent(w)
}

// ---------------------------------------------------------------------------
// Enlace público (sin sesión)
// ---------------------------------------------------------------------------

// PublicNotebook sirve, SIN SESIÓN, un cuaderno publicado y sus notas. La
// credencial es el token del enlace; por eso la respuesta no lleva el token de
// vuelta ni identifica a nadie.
func (h *Handlers) PublicNotebook(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	if len(token) < 32 || len(token) > 128 {
		httpx.Fail(w, r, httpx.ErrNotFound)
		return
	}
	nb, notes, err := h.store.PublicNotebook(r.Context(), token)
	if err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}
	// Un cuaderno público NO se indexa por buscadores: es «cualquiera con el
	// enlace», no «cualquiera».
	w.Header().Set("X-Robots-Tag", "noindex, nofollow")
	httpx.JSON(w, r, http.StatusOK, map[string]any{"notebook": nb, "notes": notes})
}

// limiteDeQuery lee `?limit=` con un tope. Sin tope, una URL a mano encarga a
// PostgreSQL una consulta arbitrariamente grande.
func limiteDeQuery(r *http.Request, porDefecto int) int {
	return limiteDe(r.URL.Query().Get("limit"), porDefecto)
}

func limiteDe(raw string, porDefecto int) int {
	if raw == "" {
		return porDefecto
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return porDefecto
	}
	if n > 100 {
		return 100
	}
	return n
}
