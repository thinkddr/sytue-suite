// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/docs/internal/domain"
	"git.sytue.eu/sytue/one/services/docs/internal/plantillas"
)

// plantillasOrgStore es lo que la biblioteca de plantillas de la organización necesita
// del store, aparte del contrato documental (como auditoriaStore).
type plantillasOrgStore interface {
	MarcarPlantillaOrg(ctx context.Context, docID string, kind domain.Kind, nombre, descripcion, sub, versionID string) (bool, error)
	PublicarPlantillaOrg(ctx context.Context, docID, sub, versionID string) (int, error)
	QuitarPlantillaOrg(ctx context.Context, docID, sub string) error
	RegistrarUsoPlantillaOrg(ctx context.Context, docID, sub string, versionNo int) error
	ListarPlantillasOrg(ctx context.Context, kind domain.Kind) ([]domain.PlantillaOrg, error)
	GetPlantillaOrg(ctx context.Context, docID string) (*domain.PlantillaOrg, error)
	MarcarBloqueOrg(ctx context.Context, docID string, bloque bool) error
	RegistroPlantillaOrg(ctx context.Context, docID string, limite int) ([]domain.EntradaRegistroPlantilla, error)
}

const (
	maxNombrePlantilla      = 120
	maxDescripcionPlantilla = 300
	prefijoPlantillaOrg     = domain.PrefijoPlantillaOrg
)

// esAdminOrg: ¿administra la organización de la sesión? Las plantillas de la
// organización las marca y las quita quien la administra (los mismos roles que la
// política de firmas), además de poder editar el documento.
func esAdminOrg(r *http.Request) bool {
	c, ok := authn.FromContext(r.Context())
	return ok && c.HasAnyRole("owner", "admin")
}

// plantillasOrgEnGaleria son las de la organización para GET /templates.
func (h *Handler) plantillasOrgEnGaleria(r *http.Request) []domain.PlantillaOrg {
	if h.plantillasOrg == nil {
		return []domain.PlantillaOrg{}
	}
	lista, err := h.plantillasOrg.ListarPlantillasOrg(r.Context(), "")
	if err != nil {
		// La galería de Sytue sigue ofreciéndose aunque la de la organización falle.
		return []domain.PlantillaOrg{}
	}
	// Los bloques se insertan desde la cinta (bloques.go): no son para crear documentos.
	return slices.DeleteFunc(lista, func(p domain.PlantillaOrg) bool { return p.Block })
}

type plantillaOrgReq struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Label       string `json:"label,omitempty"`
	// Block: ofrecerla como bloque para insertar en vez de como plantilla para crear.
	// Ausente = no se toca (renombrar no cambia qué es).
	Block *bool `json:"block,omitempty"`
}

func (q *plantillaOrgReq) validar(titulo string) error {
	q.Name = strings.TrimSpace(q.Name)
	if q.Name == "" {
		q.Name = strings.TrimSpace(titulo)
	}
	q.Description = strings.TrimSpace(q.Description)
	if q.Name == "" || utf8.RuneCountInString(q.Name) > maxNombrePlantilla ||
		utf8.RuneCountInString(q.Description) > maxDescripcionPlantilla {
		return httpx.ErrBadRequest.WithMessage("Invalid template name or description.")
	}
	return nil
}

// marcarPlantillaOrg — PUT /org-templates/{id}: marca el documento como plantilla de
// la organización (o cambia su nombre y descripción si ya lo era). Exige administrar la
// organización Y poder editar el documento: la primera versión es una instantánea de su
// contenido, y un viewer no debe poder publicar lo que no puede cambiar.
func (h *Handler) marcarPlantillaOrg(w http.ResponseWriter, r *http.Request) {
	subj, ok := subjectFrom(r)
	if !ok {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	if !esAdminOrg(r) {
		httpx.Fail(w, r, httpx.ErrForbidden)
		return
	}
	var req plantillaOrgReq
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	doc, _, err := h.svc.Authorize(r.Context(), docID(r), subj, domain.RoleEditor)
	if err != nil {
		fail(w, r, err)
		return
	}
	if err := req.validar(doc.Title); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	// La instantánea va ANTES de la marca y solo si es nueva: renombrar no publica.
	versionID := ""
	if _, err := h.plantillasOrg.GetPlantillaOrg(r.Context(), doc.ID); err != nil {
		if !errors.Is(err, domain.ErrNotFound) {
			fail(w, r, err)
			return
		}
		v, err := h.instantanea(r, subj, doc.ID, req.Label)
		if err != nil {
			fail(w, r, err)
			return
		}
		versionID = v.ID
	}
	nueva, err := h.plantillasOrg.MarcarPlantillaOrg(r.Context(), doc.ID, doc.Kind, req.Name, req.Description, subj.Sub, versionID)
	if err != nil {
		fail(w, r, err)
		return
	}
	if req.Block != nil {
		if err := h.plantillasOrg.MarcarBloqueOrg(r.Context(), doc.ID, *req.Block); err != nil {
			fail(w, r, err)
			return
		}
	}
	p, err := h.plantillasOrg.GetPlantillaOrg(r.Context(), doc.ID)
	if err != nil {
		fail(w, r, err)
		return
	}
	status := http.StatusOK
	if nueva {
		status = http.StatusCreated
	}
	httpx.JSON(w, r, status, map[string]any{"template": p})
}

func (h *Handler) instantanea(r *http.Request, subj domain.Subject, id, etiqueta string) (*domain.Version, error) {
	etiqueta = strings.TrimSpace(etiqueta)
	if etiqueta == "" {
		etiqueta = "template"
	}
	return h.svc.Snapshot(r.Context(), subj, id, &etiqueta, false,
		h.inline, h.hydrateFromS3, h.uploadSnapshot, snapshotKey)
}

// quitarPlantillaOrg — DELETE /org-templates/{id}: quita la marca. No borra el documento
// ni sus versiones, y los documentos ya creados desde la plantilla no se enteran.
func (h *Handler) quitarPlantillaOrg(w http.ResponseWriter, r *http.Request) {
	subj, ok := subjectFrom(r)
	if !ok {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	if !esAdminOrg(r) {
		httpx.Fail(w, r, httpx.ErrForbidden)
		return
	}
	if err := h.plantillasOrg.QuitarPlantillaOrg(r.Context(), docID(r), subj.Sub); err != nil {
		fail(w, r, err)
		return
	}
	httpx.NoContent(w)
}

// verPlantillaOrg — GET /org-templates/{id}: la ficha de la plantilla y sus versiones,
// para quien la usa (cualquier miembro) y para el menú del editor.
func (h *Handler) verPlantillaOrg(w http.ResponseWriter, r *http.Request) {
	if _, ok := subjectFrom(r); !ok {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	p, err := h.plantillasOrg.GetPlantillaOrg(r.Context(), docID(r))
	if err != nil {
		fail(w, r, err)
		return
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"template": p, "can_manage": esAdminOrg(r)})
}

// publicarPlantillaOrg — POST /org-templates/{id}/versions: saca una instantánea del
// documento y la deja como versión vigente. Lo hace quien puede EDITAR el documento (no
// hace falta administrar la organización: cambiar lo ya marcado es trabajo de quien lo
// mantiene), y hasta que se publica los miembros siguen recibiendo la versión anterior.
func (h *Handler) publicarPlantillaOrg(w http.ResponseWriter, r *http.Request) {
	subj, ok := subjectFrom(r)
	if !ok {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	if _, err := h.plantillasOrg.GetPlantillaOrg(r.Context(), docID(r)); err != nil {
		fail(w, r, err)
		return
	}
	var req plantillaOrgReq
	if r.ContentLength != 0 {
		if err := httpx.DecodeJSON(w, r, &req); err != nil {
			httpx.Fail(w, r, err)
			return
		}
	}
	v, err := h.instantanea(r, subj, docID(r), req.Label) // exige editor
	if err != nil {
		fail(w, r, err)
		return
	}
	no, err := h.plantillasOrg.PublicarPlantillaOrg(r.Context(), docID(r), subj.Sub, v.ID)
	if err != nil {
		fail(w, r, err)
		return
	}
	httpx.JSON(w, r, http.StatusCreated, map[string]any{"version": no, "version_id": v.ID})
}

// registroPlantillaOrg — GET /org-templates/{id}/log: la auditoría de la plantilla,
// solo para quien administra la organización.
func (h *Handler) registroPlantillaOrg(w http.ResponseWriter, r *http.Request) {
	if _, ok := subjectFrom(r); !ok {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	if !esAdminOrg(r) {
		httpx.Fail(w, r, httpx.ErrForbidden)
		return
	}
	limite, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limite <= 0 {
		limite = 100
	}
	log, err := h.plantillasOrg.RegistroPlantillaOrg(r.Context(), docID(r), limite)
	if err != nil {
		fail(w, r, err)
		return
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"entries": log})
}

// desdePlantillaOrg resuelve «crear desde la plantilla de la organización»: el tipo, el
// título y las operaciones que copian el contenido de la versión elegida (la vigente por
// defecto) al documento nuevo, en el servidor y sin pasar por el cliente. Lo que se copia
// es SOLO el contenido: ni permisos, ni comentarios, ni versiones, ni la protección.
func (h *Handler) desdePlantillaOrg(r *http.Request, idDoc, idVersion string) (plantillas.Info, int, []domain.Op, error) {
	ctx := r.Context()
	if h.plantillasOrg == nil {
		return plantillas.Info{}, 0, nil, domain.ErrNotFound
	}
	p, err := h.plantillasOrg.GetPlantillaOrg(ctx, idDoc)
	if err != nil {
		return plantillas.Info{}, 0, nil, err
	}
	if len(p.Versions) == 0 {
		return plantillas.Info{}, 0, nil, domain.ErrNotFound
	}
	elegida := p.Versions[0]
	if idVersion != "" {
		elegida = domain.VersionPlantilla{}
		for _, v := range p.Versions {
			if v.VersionID == idVersion {
				elegida = v
			}
		}
		if elegida.VersionID == "" {
			return plantillas.Info{}, 0, nil, domain.ErrNotFound
		}
	}
	destino, _, err := h.store.SnapshotState(ctx, elegida.SnapshotID, h.hydrateFromS3)
	if err != nil {
		return plantillas.Info{}, 0, nil, err
	}
	domain.RedactarEstado(destino)
	tok, err := domain.NewToken()
	if err != nil {
		return plantillas.Info{}, 0, nil, err
	}
	ops := domain.NewState().Diff(destino, "plantilla:"+tok[:16], 0, 0)
	info := plantillas.Info{Key: prefijoPlantillaOrg + p.DocumentID, Kind: p.Kind, Name: p.Name, Description: p.Description}
	return info, elegida.No, ops, nil
}
