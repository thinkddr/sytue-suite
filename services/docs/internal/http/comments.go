// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"cmp"
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/apps/nucleo/office/ancla"
	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/hubnotify"
	"git.sytue.eu/sytue/one/platform/pkg/peticiones"
	"git.sytue.eu/sytue/one/platform/pkg/validation"
	"git.sytue.eu/sytue/one/services/docs/internal/domain"
)

func (h *Handler) listComments(w http.ResponseWriter, r *http.Request) {
	subj, ok := subjectFrom(r)
	if !ok {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	if _, _, err := h.svc.Authorize(r.Context(), docID(r), subj, domain.RoleViewer); err != nil {
		fail(w, r, err)
		return
	}
	includeResolved := r.URL.Query().Get("resolved") == "true"
	comments, err := h.store.ListComments(r.Context(), docID(r), includeResolved)
	if err != nil {
		fail(w, r, err)
		return
	}
	h.conNombres(r, comments)
	httpx.JSON(w, r, http.StatusOK, map[string]any{"comments": comments})
}

// conNombres pone a cada comentario el nombre de su autor, del directorio de Sytue ID
// con la sesión de quien lee (la misma verja de organización). Sin esto la web y Fyne
// enseñaban el principio del `sub`. Un autor que no se resuelve se queda sin nombre.
//
// ponytail: una petición por autor distinto y por listado; un lote en directory:resolve
// si los hilos crecen.
func (h *Handler) conNombres(r *http.Request, cs []domain.Comment) {
	token := authn.Token(r)
	if h.idBaseURL == "" || token == "" {
		return
	}
	base := strings.TrimRight(h.idBaseURL, "/")
	nombres := map[string]string{}
	for i := range cs {
		sub := cs[i].AuthorSub
		if _, visto := nombres[sub]; !visto {
			correo, nombre, _ := peticiones.Persona(r.Context(), base, token, sub)
			nombres[sub] = cmp.Or(nombre, correo)
		}
		cs[i].AuthorName = nombres[sub]
	}
}

type createCommentReq struct {
	ParentID *string         `json:"parent_id,omitempty"`
	Anchor   json.RawMessage `json:"anchor,omitempty"`
	Body     string          `json:"body"`
}

func (h *Handler) createComment(w http.ResponseWriter, r *http.Request) {
	subj, ok := subjectFrom(r)
	if !ok {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	// Comentar requiere rol commenter (lectura + comentarios).
	if _, _, err := h.svc.Authorize(r.Context(), docID(r), subj, domain.RoleCommenter); err != nil {
		fail(w, r, err)
		return
	}
	var req createCommentReq
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if strings.TrimSpace(req.Body) == "" {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("The comment cannot be empty."))
		return
	}
	if motivo := ancla.Validar(req.Anchor); motivo != "" {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage(motivo))
		return
	}
	c, err := h.store.CreateComment(r.Context(), domain.CreateCommentParams{
		DocumentID: docID(r),
		ParentID:   req.ParentID,
		AuthorSub:  subj.Sub,
		Anchor:     []byte(req.Anchor),
		Body:       strings.TrimSpace(req.Body),
	})
	if err != nil {
		fail(w, r, err)
		return
	}
	h.avisarComentario(r.Context(), subj.Sub, c)
	uno := []domain.Comment{*c}
	h.conNombres(r, uno)
	c = &uno[0]
	httpx.JSON(w, r, http.StatusCreated, map[string]any{"comment": c})
}

// avisarComentario deja el comentario en la campana de quien tiene que enterarse: el
// dueño del documento y, si es una respuesta, quienes escriben en ese hilo. Nunca
// quien lo escribe. Y a quien se menciona con su dirección (`@ana@empresa.es`), el
// aviso de la mención.
func (h *Handler) avisarComentario(ctx context.Context, autor string, c *domain.Comment) {
	if h.hubNotifier == nil || c == nil {
		return
	}
	doc, err := h.store.GetDocument(ctx, c.DocumentID)
	if err != nil || doc == nil {
		return
	}
	cuerpo := validation.TruncateRunes(c.Body, 300)
	quienes := []string{doc.OwnerSub}
	if c.ParentID != nil {
		if hilo, err := h.store.ListComments(ctx, c.DocumentID, true); err == nil {
			for _, o := range hilo {
				if o.ID == *c.ParentID || (o.ParentID != nil && *o.ParentID == *c.ParentID) {
					quienes = append(quienes, o.AuthorSub)
				}
			}
		}
	}
	vistos := map[string]bool{autor: true, "": true}
	for _, q := range quienes {
		if !vistos[q] {
			vistos[q] = true
			h.avisar(ctx, q, doc, "docs.comment", tituloComentario(doc), cuerpo)
		}
	}
	for _, e := range hubnotify.Mencionados(c.Body) {
		h.avisar(ctx, hubnotify.PorCorreo(e), doc, "docs.mention", tituloMencion(doc), cuerpo)
	}
}

type resolveCommentReq struct {
	Resolved bool `json:"resolved"`
}

func (h *Handler) resolveComment(w http.ResponseWriter, r *http.Request) {
	subj, ok := subjectFrom(r)
	if !ok {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	if _, _, err := h.svc.Authorize(r.Context(), docID(r), subj, domain.RoleCommenter); err != nil {
		fail(w, r, err)
		return
	}
	var req resolveCommentReq
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if err := h.store.ResolveComment(r.Context(), docID(r), chi.URLParam(r, "commentId"), subj.Sub, req.Resolved); err != nil {
		fail(w, r, err)
		return
	}
	httpx.NoContent(w)
}

// deleteComment: borrar es de quien lo escribió y de quien modera. Un comentarista solo
// borra los suyos; un editor o el propietario, cualquiera (también las respuestas y los
// importados de Word, que son de quien importó).
func (h *Handler) deleteComment(w http.ResponseWriter, r *http.Request) {
	subj, ok := subjectFrom(r)
	if !ok {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	_, rol, err := h.svc.Authorize(r.Context(), docID(r), subj, domain.RoleCommenter)
	if err != nil {
		fail(w, r, err)
		return
	}
	if err := h.store.DeleteComment(r.Context(), docID(r), chi.URLParam(r, "commentId"), subj.Sub, rol.AtLeast(domain.RoleEditor)); err != nil {
		fail(w, r, err)
		return
	}
	httpx.NoContent(w)
}

// importedComment es un comentario de un fichero importado (domain.ImportedComment), tal como
// lo reenvía el cliente que crea el documento.
type importedComment struct {
	ID       string `json:"id"`
	ParentID string `json:"parent_id"`
	Text     string `json:"text"`
	Quote    string `json:"quote"`
	Anchored bool   `json:"anchored"`
	Resolved bool   `json:"resolved"`
	// Cell es la celda de un comentario de una hoja (.xlsx/.ods): su ancla.
	Cell *celdaImportada `json:"cell,omitempty"`
}

// celdaImportada es el ancla de un comentario de una celda: la hoja (su id) y la identidad de su
// fila y de su columna (sheets/anclas.ts), más la referencia que se ve.
type celdaImportada struct {
	Hoja string `json:"hoja"`
	Fila string `json:"fila"`
	Col  string `json:"col"`
	Text string `json:"text"`
}

// anclaDeCelda es el ancla de un comentario de una celda importado, o nil si no es válida.
func anclaDeCelda(c *celdaImportada) map[string]string {
	if c == nil {
		return nil
	}
	return ancla.DeCelda(c.Hoja, c.Fila, c.Col, c.Text)
}

// maxComentariosImportados acota lo que se guarda de una vez de un fichero.
const maxComentariosImportados = 1000

// importComments guarda los comentarios de un fichero recién importado (ImportWithComments)
// en el documento que el cliente acaba de crear. Regla de autoría del dueño: los guarda quien
// importa, sin usuarios inventados; el autor y la fecha originales ya van delante del cuerpo
// (`text`). Las respuestas se guardan tras sus raíces y no avisan a nadie. Contesta con los
// ids de los comentarios con ancla que SÍ quedaron guardados: el cliente quita del HTML los
// marks de los que no (un mark sin comentario queda resaltado para siempre).
func (h *Handler) importComments(w http.ResponseWriter, r *http.Request) {
	subj, ok := subjectFrom(r)
	if !ok {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	if _, _, err := h.svc.Authorize(r.Context(), docID(r), subj, domain.RoleCommenter); err != nil {
		fail(w, r, err)
		return
	}
	var req struct {
		Comments []importedComment `json:"comments"`
	}
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if len(req.Comments) > maxComentariosImportados {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Too many comments to import."))
		return
	}
	// Raíces primero: una respuesta necesita el id guardado de su padre.
	orden := make([]importedComment, 0, len(req.Comments))
	for _, quienes := range []bool{false, true} {
		for _, c := range req.Comments {
			if (c.ParentID != "") == quienes {
				orden = append(orden, c)
			}
		}
	}
	guardado := map[string]string{} // id local -> id guardado
	var anclados []string
	for _, c := range orden {
		id, err := h.guardarImportado(r, subj.Sub, c, guardado)
		if err != nil {
			fail(w, r, err)
			return
		}
		if id != "" {
			guardado[c.ID] = id
			if c.Anchored {
				anclados = append(anclados, c.ID)
			}
		}
	}
	httpx.JSON(w, r, http.StatusCreated, map[string]any{"saved": len(guardado), "anchored": anclados})
}

// guardarImportado guarda un comentario importado y devuelve su id guardado, o "" si no se
// guarda (vacío, id de marca no válido o su padre no se guardó).
func (h *Handler) guardarImportado(r *http.Request, sub string, c importedComment, guardado map[string]string) (string, error) {
	body := strings.TrimSpace(validation.TruncateRunes(c.Text, 20000))
	if body == "" || (c.Anchored && !ancla.EsMarca(c.ID)) {
		return "", nil
	}
	p := domain.CreateCommentParams{DocumentID: docID(r), AuthorSub: sub, Body: body}
	if c.ParentID != "" {
		padre, ok := guardado[c.ParentID]
		if !ok {
			return "", nil // sin hilo al que colgarla
		}
		p.ParentID = &padre
	}
	anchor := map[string]string{"text": validation.TruncateRunes(c.Quote, 1000)}
	if c.Anchored {
		anchor["mark"] = c.ID
	} else if celda := anclaDeCelda(c.Cell); celda != nil && c.ParentID == "" {
		anchor = celda
	}
	p.Anchor, _ = json.Marshal(anchor)
	saved, err := h.store.CreateComment(r.Context(), p)
	if err != nil {
		return "", err
	}
	if c.Resolved {
		if err := h.store.ResolveComment(r.Context(), docID(r), saved.ID, sub, true); err != nil {
			return "", err
		}
	}
	return saved.ID, nil
}
