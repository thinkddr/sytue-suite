// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Handlers HTTP de las UNIDADES DE EQUIPO (team/shared drives). Espejan el
// contrato del resto de la API (sobre JSON canónico httpx, errores de dominio →
// códigos vía toAPIError). La VERJA de acceso deny-by-default y por rol la impone
// el servicio (handlers_teamdrive equivalentes en service/teamdrive.go): un
// no-miembro recibe ErrNotFound → 404; un rol insuficiente, ErrPermission → 403;
// quitar/degradar al último gestor, ErrLastManager → 409. Sin cambios en las rutas
// personales: el contenido de una unidad se sirve SIEMPRE por estas rutas.
package drivehttp

import (
	"errors"
	"io"
	"mime"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/drive/internal/domain"
)

// teamDriveView es una unidad de equipo más el ROL del usuario que la consulta (el
// cliente pinta/oculta acciones según el rol). Embebe domain.TeamDrive (que ya
// omite tenant y carpeta raíz de la serialización).
type teamDriveView struct {
	domain.TeamDrive
	Role domain.TeamRole `json:"role"`
}

// parseNamedID extrae y valida un UUID de un parámetro de ruta con nombre propio
// (para rutas con más de un id, p. ej. {id}/files/{fileID}).
func parseNamedID(r *http.Request, name string) (uuid.UUID, error) {
	id, err := uuid.Parse(chi.URLParam(r, name))
	if err != nil {
		return uuid.Nil, httpx.ErrBadRequest.WithMessage("Invalid id.")
	}
	return id, nil
}

// optionalFolderQuery lee el parámetro ?folder= (uuid.Nil si ausente = raíz de la
// unidad). Un valor presente pero inválido es un 400.
func optionalFolderQuery(r *http.Request) (uuid.UUID, error) {
	raw := strings.TrimSpace(r.URL.Query().Get("folder"))
	if raw == "" {
		return uuid.Nil, nil
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		return uuid.Nil, httpx.ErrBadRequest.WithMessage("Invalid folder.")
	}
	return id, nil
}

// ---------------------------------------------------------------------------
// Unidades: crear, listar, obtener, renombrar/archivar
// ---------------------------------------------------------------------------

type createTeamDriveRequest struct {
	Name string `json:"name"`
}

// CreateTeamDrive crea una unidad de equipo (el creador queda como manager).
func (h *Handlers) CreateTeamDrive(w http.ResponseWriter, r *http.Request) {
	var req createTeamDriveRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("A drive name is required."))
		return
	}
	d, err := h.svc.CreateTeamDrive(r.Context(), strings.TrimSpace(req.Name), subFromCtx(r))
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	// El creador es siempre manager (lo garantiza el store).
	httpx.JSON(w, r, http.StatusCreated, teamDriveView{TeamDrive: d, Role: domain.TeamManager})
}

// ListTeamDrives lista las unidades de las que el usuario es miembro, con su rol.
func (h *Handlers) ListTeamDrives(w http.ResponseWriter, r *http.Request) {
	drives, err := h.svc.ListTeamDrives(r.Context(), subFromCtx(r))
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	items := make([]teamDriveView, 0, len(drives))
	for _, m := range drives {
		items = append(items, teamDriveView{TeamDrive: m.TeamDrive, Role: m.Role})
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

// GetTeamDrive devuelve una unidad y el rol del usuario (404 si no es miembro).
func (h *Handlers) GetTeamDrive(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	d, role, err := h.svc.GetTeamDrive(r.Context(), id, subFromCtx(r))
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, teamDriveView{TeamDrive: d, Role: role})
}

type updateTeamDriveRequest struct {
	Name     *string `json:"name,omitempty"`
	Archived *bool   `json:"archived,omitempty"`
}

// UpdateTeamDrive renombra o (des)archiva una unidad (solo manager; operaciones
// mutuamente exclusivas).
func (h *Handlers) UpdateTeamDrive(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var req updateTeamDriveRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	sub := subFromCtx(r)
	switch {
	case req.Name != nil && req.Archived == nil:
		d, err := h.svc.RenameTeamDrive(r.Context(), id, sub, strings.TrimSpace(*req.Name))
		if err != nil {
			httpx.Fail(w, r, toAPIError(err))
			return
		}
		httpx.JSON(w, r, http.StatusOK, teamDriveView{TeamDrive: d, Role: domain.TeamManager})
	case req.Archived != nil && req.Name == nil:
		d, err := h.svc.SetTeamDriveArchived(r.Context(), id, sub, *req.Archived)
		if err != nil {
			httpx.Fail(w, r, toAPIError(err))
			return
		}
		httpx.JSON(w, r, http.StatusOK, teamDriveView{TeamDrive: d, Role: domain.TeamManager})
	default:
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Give exactly one: name (rename) or archived (archive)."))
	}
}

// ---------------------------------------------------------------------------
// Miembros: listar, añadir/cambiar rol, quitar (solo manager)
// ---------------------------------------------------------------------------

// ListTeamDriveMembers lista los miembros de una unidad (solo manager).
func (h *Handlers) ListTeamDriveMembers(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	members, err := h.svc.ListTeamDriveMembers(r.Context(), id, subFromCtx(r))
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	if members == nil {
		members = []domain.TeamMember{}
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"members": members})
}

type putMemberRequest struct {
	Subject string `json:"subject"`
	Role    string `json:"role"`
}

// PutTeamDriveMember añade un miembro o cambia su rol (solo manager). Upsert.
func (h *Handlers) PutTeamDriveMember(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var req putMemberRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	req.Subject = strings.TrimSpace(req.Subject)
	if req.Subject == "" {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("The member subject is required."))
		return
	}
	role := domain.TeamRole(req.Role)
	if !domain.ValidTeamRole(role) {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("The role must be viewer, editor or manager."))
		return
	}
	m, err := h.svc.PutTeamDriveMember(r.Context(), id, subFromCtx(r), req.Subject, role)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, m)
}

// RemoveTeamDriveMember quita a un miembro (solo manager). El subject viaja en la
// ruta (percent-encoded por el cliente).
func (h *Handlers) RemoveTeamDriveMember(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	subject := strings.TrimSpace(chi.URLParam(r, "subject"))
	if subject == "" {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("The member subject is missing."))
		return
	}
	if err := h.svc.RemoveTeamDriveMember(r.Context(), id, subFromCtx(r), subject); err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.NoContent(w)
}

// ---------------------------------------------------------------------------
// Contenido dentro de una unidad: navegar, crear carpeta, subir, descargar, borrar
// ---------------------------------------------------------------------------

// ListTeamDriveChildren lista subcarpetas + página de ficheros de una carpeta de la
// unidad (?folder=<id> vacío = raíz), con el rol del usuario. Cualquier miembro lee.
func (h *Handlers) ListTeamDriveChildren(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	folderID, err := optionalFolderQuery(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	cursor := strings.TrimSpace(r.URL.Query().Get("cursor"))
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	listing, err := h.svc.TeamListChildren(r.Context(), id, subFromCtx(r), folderID, cursor, limit)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	folders := listing.Subfolders
	if folders == nil {
		folders = []domain.Folder{}
	}
	files := listing.Files
	if files == nil {
		files = []domain.File{}
	}
	resp := map[string]any{
		"role":      listing.Role,
		"can_write": listing.Role.CanWrite(),
		"folders":   folders,
		"files":     files,
	}
	if listing.NextCursor != "" {
		resp["next_cursor"] = listing.NextCursor
	}
	httpx.JSON(w, r, http.StatusOK, resp)
}

type createTeamFolderRequest struct {
	ParentID string `json:"parent_id,omitempty"`
	Name     string `json:"name"`
}

// CreateTeamDriveFolder crea una subcarpeta dentro de la unidad (editor+).
func (h *Handlers) CreateTeamDriveFolder(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var req createTeamFolderRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	parentID := uuid.Nil
	if strings.TrimSpace(req.ParentID) != "" {
		parentID, err = uuid.Parse(req.ParentID)
		if err != nil {
			httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid parent_id."))
			return
		}
	}
	f, err := h.svc.TeamCreateFolder(r.Context(), id, subFromCtx(r), parentID, req.Name)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.JSON(w, r, http.StatusCreated, f)
}

// UploadTeamDriveFile sube un fichero a una carpeta de la unidad (?folder= vacío =
// raíz), editor+. Mismo contrato de cuerpo/nombre que UploadFile.
func (h *Handlers) UploadTeamDriveFile(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	folderID, err := optionalFolderQuery(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	name := fileNameFromRequest(r)
	if name == "" {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("The file name is missing (X-File-Name header or ?name=)."))
		return
	}
	contentType := r.Header.Get("Content-Type")
	if contentType == "" || strings.HasPrefix(contentType, "multipart/") {
		contentType = mime.TypeByExtension(filepath.Ext(name))
	}
	body := http.MaxBytesReader(w, r.Body, h.cfg.MaxUploadBytes+1)
	defer body.Close()

	f, err := h.svc.TeamUploadFile(r.Context(), id, subFromCtx(r), folderID, name, contentType, body)
	if err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			httpx.Fail(w, r, httpx.NewError(http.StatusRequestEntityTooLarge, "file_too_large", "The file is larger than the maximum allowed size."))
			return
		}
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.JSON(w, r, http.StatusCreated, f)
}

// DownloadTeamDriveFile transmite el contenido de un fichero de la unidad (viewer+).
// Disposición SEGURA (attachment + nosniff), como DownloadFile.
func (h *Handlers) DownloadTeamDriveFile(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	fileID, err := parseNamedID(r, "fileID")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	f, rc, err := h.svc.TeamOpenFileContent(r.Context(), id, subFromCtx(r), fileID)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	defer rc.Close()

	w.Header().Set("Content-Type", f.ContentType)
	w.Header().Set("Content-Length", strconv.FormatInt(f.SizeBytes, 10))
	w.Header().Set("Content-Disposition", "attachment; filename*=UTF-8''"+escapeFilename(f.Name))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if f.Checksum != "" {
		w.Header().Set("ETag", `"`+f.Checksum+`"`)
	}
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, rc)
}

// DeleteTeamDriveFolder envía a la papelera de la unidad una carpeta (editor+).
func (h *Handlers) DeleteTeamDriveFolder(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	folderID, err := parseNamedID(r, "folderID")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if err := h.svc.TeamDeleteFolder(r.Context(), id, subFromCtx(r), folderID); err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.NoContent(w)
}

// DeleteTeamDriveFile envía a la papelera de la unidad un fichero (editor+).
func (h *Handlers) DeleteTeamDriveFile(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	fileID, err := parseNamedID(r, "fileID")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if err := h.svc.TeamDeleteFile(r.Context(), id, subFromCtx(r), fileID); err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.NoContent(w)
}
