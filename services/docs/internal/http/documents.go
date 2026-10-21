// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/docs/internal/domain"
	"git.sytue.eu/sytue/one/services/docs/internal/drive"
)

type createDocumentReq struct {
	Kind  domain.Kind `json:"kind"`
	Title string      `json:"title"`
	// SourceRef ancla el documento al fichero del que sale («drive:<uuid>»), para
	// que abrir ese mismo fichero otra vez vuelva a él en vez de crear otra copia.
	SourceRef string `json:"source_ref,omitempty"`
	// Template es la plantilla de la galería («carta», «presupuesto»…) de la que
	// nace el documento; manda sobre `kind`. Locale es el idioma de su texto.
	Template string `json:"template,omitempty"`
	Locale   string `json:"locale,omitempty"`
	// TemplateVersion elige la versión publicada de una plantilla de la organización
	// («org:<id>»); vacía = la vigente.
	TemplateVersion string `json:"template_version,omitempty"`
	// Theme es el tema (del catálogo común o «marca») que se aplica al contenido de la
	// plantilla al crear; BrandVersion elige una versión de la marca (0 = la vigente).
	// Sin plantilla no hay contenido que estilar y se ignora.
	Theme        string `json:"theme,omitempty"`
	BrandVersion int    `json:"brand_version,omitempty"`
}

type documentResp struct {
	Document *domain.Document `json:"document"`
	Role     domain.Role      `json:"role,omitempty"`
	// EditRequested: quien solo puede leer ya ha pedido editar (la franja lo dice).
	EditRequested bool `json:"edit_requested,omitempty"`
}

func (h *Handler) createDocument(w http.ResponseWriter, r *http.Request) {
	subj, ok := subjectFrom(r)
	if !ok {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	var req createDocumentReq
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}

	// El documento nace CON su fichero en la unidad de la persona (hogar_en_drive.go).
	// Se valida el tipo y se fija el título antes, porque el nombre del fichero se
	// decide aquí: dejar que lo validara `svc.Create` habría significado crear el
	// fichero primero y descubrir después que el tipo no existía.
	sourceRef := strings.TrimSpace(req.SourceRef)
	titulo := strings.TrimSpace(req.Title)

	// Desde una plantilla, el documento nace con su contenido (creacion.go): el tipo y,
	// si no se pide otro, el título son los de la plantilla; con `theme`, ya tematizado.
	sem, err := h.semillaDeCreacion(r, req, sourceRef, titulo)
	if err != nil {
		fail(w, r, err)
		return
	}
	req.Kind, titulo = sem.kind, sem.titulo
	semilla, estado := sem.ops, sem.estado
	if titulo == "" {
		titulo = "Sin título"
	}
	if !req.Kind.Valid() {
		fail(w, r, domain.ErrInvalidKind)
		return
	}
	// Con `source_ref` ya puesto el documento VIENE de un fichero que existe (es
	// «abrir con Sytue Docs»): crearle otro sería duplicar el original y romper la
	// idempotencia que da el índice único de la migración 0006.
	if sourceRef == "" && h.drive != nil {
		ref, tituloFinal, err := h.crearFicheroEnDrive(r, req.Kind, titulo, estado)
		if err != nil {
			errorDeDrive(w, r, err)
			return
		}
		sourceRef = ref
		// El título que se guarda es el que quedó tras desambiguar en la unidad, para
		// que el documento y su fichero se llamen igual. Si no hizo falta desambiguar
		// —el caso normal— vuelve idéntico.
		titulo = tituloFinal
	}

	doc, creado, err := h.svc.CreateSeeded(r.Context(), subj, req.Kind, titulo, sourceRef, semilla)
	if err != nil {
		fail(w, r, err)
		return
	}
	if sem.usoDoc != "" && creado {
		if err := h.plantillasOrg.RegistrarUsoPlantillaOrg(r.Context(), sem.usoDoc, subj.Sub, sem.usoVersion); err != nil {
			slog.WarnContext(r.Context(), "docs: no se pudo registrar el uso de la plantilla", "error", err)
		}
	}
	// 200 cuando el documento ya existía (mismo fichero de origen, misma persona):
	// quien llama distingue así si tiene que sembrar el contenido o si sembrarlo
	// pisaría lo que ya hay escrito.
	status := http.StatusCreated
	if !creado {
		status = http.StatusOK
	}
	httpx.JSON(w, r, status, documentResp{Document: doc, Role: domain.RoleOwner})
}

func (h *Handler) listDocuments(w http.ResponseWriter, r *http.Request) {
	subj, ok := subjectFrom(r)
	if !ok {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	f := domain.ListFilter{
		Kind:     domain.Kind(q.Get("kind")),
		OwnerSub: q.Get("owner"),
		Query:    q.Get("q"),
		Limit:    limit,
		// ?starred=1 — solo los destacados por quien pregunta (columna starred_by).
		StarredOnly: q.Get("starred") == "1" || q.Get("starred") == "true",
		// ?source_ref=drive:<uuid> — el documento que nació de ese fichero, si lo
		// hay. Con esto, «Abrir con Sytue Docs» sabe si ya existe SIN descargar el
		// fichero entero para descubrir que sobraba.
		SourceRef: q.Get("source_ref"),
	}
	if c := q.Get("cursor"); c != "" {
		at, id, err := parseListCursor(c)
		if err != nil {
			httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid pagination cursor."))
			return
		}
		f.BeforeUpdatedAt = &at
		f.BeforeID = id
	}
	docs, err := h.svc.List(r.Context(), subj, f)
	if err != nil {
		fail(w, r, err)
		return
	}
	// Y los de OTRAS organizaciones en los que canjeó un enlace (ADR 0052), con los
	// mismos filtros y el mismo cursor: se mezclan por fecha y se corta la página.
	if h.canje != nil {
		if fuera, err := h.canje.DeFuera(r.Context(), subj.Sub, f); err == nil && len(fuera) > 0 {
			docs = append(docs, fuera...)
			slices.SortFunc(docs, func(a, b domain.Document) int {
				if c := b.UpdatedAt.Compare(a.UpdatedAt); c != 0 {
					return c
				}
				return strings.Compare(b.ID, a.ID)
			})
			if len(docs) > limit {
				docs = docs[:limit]
			}
		}
	}
	resp := map[string]any{"documents": docs}
	// Página llena => puede haber más: cursor keyset al último documento.
	if len(docs) == limit {
		last := docs[len(docs)-1]
		resp["next_cursor"] = last.UpdatedAt.UTC().Format(time.RFC3339Nano) + "~" + last.ID
	}
	httpx.JSON(w, r, http.StatusOK, resp)
}

// parseListCursor decodifica el cursor keyset del listado:
// "<updated_at RFC3339Nano>~<uuid>" (lo que emite next_cursor). Valida el uuid
// aquí para que un cursor corrupto sea un 400, no un error de cast en la BD.
func parseListCursor(c string) (time.Time, string, error) {
	ts, id, ok := strings.Cut(c, "~")
	if !ok {
		return time.Time{}, "", fmt.Errorf("cursor sin separador")
	}
	at, err := time.Parse(time.RFC3339Nano, ts)
	if err != nil {
		return time.Time{}, "", err
	}
	if _, err := uuid.Parse(id); err != nil {
		return time.Time{}, "", err
	}
	return at, id, nil
}

func (h *Handler) getDocument(w http.ResponseWriter, r *http.Request) {
	subj, ok := subjectFrom(r)
	if !ok {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	doc, role, err := h.svc.Authorize(r.Context(), docID(r), subj, domain.RoleViewer)
	if err != nil {
		fail(w, r, err)
		return
	}
	resp := documentResp{Document: doc, Role: role}
	if !role.CanWrite() {
		resp.EditRequested = h.pedidaPor(r.Context(), doc.ID, subj.Sub)
	}
	httpx.JSON(w, r, http.StatusOK, resp)
}

type renameReq struct {
	Title string `json:"title"`
}

func (h *Handler) renameDocument(w http.ResponseWriter, r *http.Request) {
	subj, ok := subjectFrom(r)
	if !ok {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	var req renameReq
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if err := h.svc.Rename(r.Context(), subj, docID(r), req.Title); err != nil {
		fail(w, r, err)
		return
	}
	httpx.NoContent(w)
}

func (h *Handler) deleteDocument(w http.ResponseWriter, r *http.Request) {
	subj, ok := subjectFrom(r)
	if !ok {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	// Su fichero va a la papelera de Drive ANTES que el documento (papelera.go): si
	// Drive falla, no se borra nada y la persona lo ve.
	doc, _, err := h.svc.Authorize(r.Context(), docID(r), subj, domain.RoleOwner)
	if err != nil {
		fail(w, r, err)
		return
	}
	if err := h.ficheroDelDocumento(r.Context(), doc, drive.AccionTirar); err != nil {
		errorDePapelera(w, r, err)
		return
	}
	if err := h.svc.Delete(r.Context(), subj, docID(r)); err != nil {
		fail(w, r, err)
		return
	}
	httpx.NoContent(w)
}

type starReq struct {
	Starred bool `json:"starred"`
}

func (h *Handler) starDocument(w http.ResponseWriter, r *http.Request) {
	subj, ok := subjectFrom(r)
	if !ok {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	var req starReq
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if err := h.svc.Star(r.Context(), subj, docID(r), req.Starred); err != nil {
		fail(w, r, err)
		return
	}
	httpx.NoContent(w)
}
