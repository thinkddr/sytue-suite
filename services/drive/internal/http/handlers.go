// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drivehttp

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/log"
	"git.sytue.eu/sytue/one/platform/pkg/peticiones"
	"git.sytue.eu/sytue/one/services/drive/internal/config"
	"git.sytue.eu/sytue/one/services/drive/internal/domain"
	"git.sytue.eu/sytue/one/services/drive/internal/service"
	"git.sytue.eu/sytue/one/services/drive/internal/store"
	"git.sytue.eu/sytue/one/services/drive/internal/wopi"
)

// driveService es la superficie de casos de uso que consumen los handlers HTTP.
// La satisface *service.Service (producción) y un fake en tests, de modo que los
// handlers se ejercitan sin PostgreSQL ni S3. No cambia la firma pública de
// NewRouter (que sigue recibiendo un *service.Service).
type driveService interface {
	// Almacén S3 de terceros: si el tenant lo tiene, y olvidar su caché al cambiarlo.
	ExternalStorageEnabled(ctx context.Context) bool
	ForgetExternalStorage(tenant string)
	ExternalInstances(ctx context.Context) (service.ExternalAccounts, error)
	ProbeExternal(ctx context.Context, cfg store.ExternalStorage, secret string) string
	ConnectNAS(ctx context.Context, req service.NASRequest, actor string) (service.NASStatus, error)
	NASStatus(ctx context.Context) (service.NASStatus, error)
	TeardownNAS(ctx context.Context, e store.ExternalStorage) error
	Root(ctx context.Context) (domain.Folder, error)
	GetFolder(ctx context.Context, id uuid.UUID) (domain.Folder, error)
	ListSubfolders(ctx context.Context, id uuid.UUID) ([]domain.Folder, error)
	ListFilesPage(ctx context.Context, folderID uuid.UUID, cursor string, limit int) (service.FilePage, error)
	CreateFolder(ctx context.Context, parentID uuid.UUID, name, ownerSub string, isGroup bool) (domain.Folder, error)
	RenameFolder(ctx context.Context, id uuid.UUID, name string) (domain.Folder, error)
	MoveFolder(ctx context.Context, id, newParent uuid.UUID) (domain.Folder, error)
	DeleteFolder(ctx context.Context, id uuid.UUID) error
	UploadFile(ctx context.Context, folderID uuid.UUID, name, contentType, ownerSub string, body io.Reader) (domain.File, error)
	PresignUpload(ctx context.Context, path string, size int64, contentType, ownerSub string) (service.PresignResult, error)
	FinalizeUpload(ctx context.Context, handle string) (domain.File, error)
	GetFile(ctx context.Context, id uuid.UUID) (domain.File, error)
	OpenFileContent(ctx context.Context, id uuid.UUID) (domain.File, io.ReadCloser, error)
	Thumbnail(ctx context.Context, id uuid.UUID) (io.ReadCloser, string, string, error)
	PresignDownload(ctx context.Context, id uuid.UUID) (*url.URL, error)
	RenameFile(ctx context.Context, id uuid.UUID, name string) (domain.File, error)
	MoveFile(ctx context.Context, id, newFolder uuid.UUID) (domain.File, error)
	SetFileLabel(ctx context.Context, id uuid.UUID, label domain.Label, perm domain.Permission) (domain.File, error)
	DeleteFile(ctx context.Context, id uuid.UUID) error
	CopyFile(ctx context.Context, srcID, destFolderID uuid.UUID, actorSub string, perm domain.Permission) (domain.File, error)
	CopyFolder(ctx context.Context, srcID uuid.UUID, destParentID *uuid.UUID, actorSub string, perm domain.Permission) (domain.Folder, error)
	Recent(ctx context.Context, cursor string, limit int) (service.RecentPage, error)
	Favorites(ctx context.Context, subject string) ([]service.Entry, error)
	SetFavorite(ctx context.Context, subject string, fileID, folderID *uuid.UUID, on bool) error
	ListTrash(ctx context.Context) ([]domain.Folder, []domain.File, error)
	RestoreFile(ctx context.Context, id uuid.UUID) (domain.File, error)
	RestoreFolder(ctx context.Context, id uuid.UUID) (domain.Folder, error)
	PurgeFile(ctx context.Context, id uuid.UUID) error
	PurgeFolder(ctx context.Context, id uuid.UUID) error
	Search(ctx context.Context, query string, limit int) ([]service.Entry, error)
	FilePath(ctx context.Context, id uuid.UUID) (string, error)
	Quota(ctx context.Context) (domain.Quota, error)
	SetQuotaLimit(ctx context.Context, limit int64) (domain.Quota, error)
	CreateShare(ctx context.Context, in service.CreateShareInput) (domain.Share, error)
	ListFileShares(ctx context.Context, id uuid.UUID) ([]domain.Share, error)
	ListFolderShares(ctx context.Context, id uuid.UUID) ([]domain.Share, error)
	RevokeShare(ctx context.Context, id uuid.UUID) error
	PublicResolve(ctx context.Context, token, proof string) (domain.Share, string, error)
	PublicVerify(ctx context.Context, token, password string) (string, error)
	PublicBrowse(ctx context.Context, share domain.Share, tenantSlug, subpath string) (service.PublicNode, error)
	SharedWithMe(ctx context.Context, subject string, groups []string) ([]service.SharedEntry, error)
	PedirEdicion(ctx context.Context, recurso uuid.UUID, groups []string, p peticiones.Peticion, token string) (bool, error)
	PeticionesEdicion(ctx context.Context, recurso uuid.UUID, sub string) ([]peticiones.Peticion, error)
	ResolverPeticion(ctx context.Context, recurso uuid.UUID, sub, sujeto string, concede bool) error
	PedidasPor(ctx context.Context, sub string, recursos []uuid.UUID) (map[uuid.UUID]bool, error)
	SharedBrowse(ctx context.Context, subject string, groups []string, rootID uuid.UUID, subpath string) (service.PublicNode, domain.Permission, error)
	SharedUpload(ctx context.Context, subject string, groups []string, rootID uuid.UUID, subpath, name, contentType, ownerSub string, body io.Reader) (domain.File, error)
	FileVersions(ctx context.Context, fileID uuid.UUID, cursor string, limit int) (service.VersionPage, error)
	OpenVersionContent(ctx context.Context, fileID uuid.UUID, version int) (domain.FileVersion, io.ReadCloser, error)
	RestoreVersion(ctx context.Context, fileID uuid.UUID, version int, actorSub string, perm domain.Permission) (domain.File, error)
	BatchMove(ctx context.Context, fileIDs, folderIDs []uuid.UUID, targetFolderID uuid.UUID) (service.BatchResult, error)
	BatchTrash(ctx context.Context, fileIDs, folderIDs []uuid.UUID) (service.BatchResult, error)
	ZipEntries(ctx context.Context, fileIDs, folderIDs []uuid.UUID) ([]service.ZipEntry, error)
	StreamZip(ctx context.Context, w io.Writer, entries []service.ZipEntry) error

	// Unidades de equipo (team/shared drives). Verja deny-by-default y por rol
	// dentro del servicio: un no-miembro recibe ErrNotFound; un rol insuficiente,
	// ErrPermission. Ver handlers_teamdrive.go.
	CreateTeamDrive(ctx context.Context, name, createdBy string) (domain.TeamDrive, error)
	ListTeamDrives(ctx context.Context, subject string) ([]service.TeamDriveMembership, error)
	GetTeamDrive(ctx context.Context, driveID uuid.UUID, subject string) (domain.TeamDrive, domain.TeamRole, error)
	RenameTeamDrive(ctx context.Context, driveID uuid.UUID, subject, name string) (domain.TeamDrive, error)
	SetTeamDriveArchived(ctx context.Context, driveID uuid.UUID, subject string, archived bool) (domain.TeamDrive, error)
	ListTeamDriveMembers(ctx context.Context, driveID uuid.UUID, subject string) ([]domain.TeamMember, error)
	PutTeamDriveMember(ctx context.Context, driveID uuid.UUID, actorSub, subject string, role domain.TeamRole) (domain.TeamMember, error)
	RemoveTeamDriveMember(ctx context.Context, driveID uuid.UUID, actorSub, subject string) error
	TeamListChildren(ctx context.Context, driveID uuid.UUID, subject string, folderID uuid.UUID, cursor string, limit int) (service.TeamListing, error)
	TeamCreateFolder(ctx context.Context, driveID uuid.UUID, subject string, parentID uuid.UUID, name string) (domain.Folder, error)
	TeamUploadFile(ctx context.Context, driveID uuid.UUID, subject string, folderID uuid.UUID, name, contentType string, body io.Reader) (domain.File, error)
	TeamOpenFileContent(ctx context.Context, driveID uuid.UUID, subject string, fileID uuid.UUID) (domain.File, io.ReadCloser, error)
	TeamDeleteFolder(ctx context.Context, driveID uuid.UUID, subject string, folderID uuid.UUID) error
	TeamDeleteFile(ctx context.Context, driveID uuid.UUID, subject string, fileID uuid.UUID) error

	// Adjuntos de canal de Sytue Rooms (ámbito de canal, mig 0014). Verja
	// deny-by-default por MEMBRESÍA del canal, resuelta contra services/rooms en cada
	// petición: no-miembro o fichero de otro canal ⇒ ErrNotFound → 404. Ver
	// handlers_channelfiles.go y service/channelfiles.go.
	ChannelUploadFile(ctx context.Context, channelID uuid.UUID, subject, name, contentType string, body io.Reader) (domain.File, error)
	ChannelFile(ctx context.Context, channelID, fileID uuid.UUID, subject string) (domain.File, error)
	ChannelFileContent(ctx context.Context, channelID, fileID uuid.UUID, subject string) (domain.File, io.ReadCloser, error)
	ChannelThumbnail(ctx context.Context, channelID, fileID uuid.UUID, subject string) (io.ReadCloser, string, string, error)
	CopyChannelFileToMyDrive(ctx context.Context, channelID, fileID uuid.UUID, subject string) (domain.File, error)
}

// Handlers agrupa los manejadores de la API REST de Drive.
type Handlers struct {
	svc    driveService
	cipher cipherBlobService
	cfg    config.Config
	// wopi acuña los tokens de acceso del host WOPI. Nil ⇒ el endpoint de acuñación
	// responde 404, igual que si el protocolo no existiera (sin WOPI_KEY no se monta).
	wopi *wopi.Minter
	// editorReg entrega el catálogo del editor de CADA organización. El editor era una
	// variable de entorno del servicio, así que conectar el Collabora de una empresa
	// obligaba a editar el despliegue y todas las organizaciones compartían el mismo.
	editorReg *wopi.Registry
	// editors persiste ese ajuste. Nil ⇒ el plano de administración responde 404.
	editors wopiEditorStore
	// bases exporta una vista de Sytue Bases a una carpeta (handlers_bases.go). Nil ⇒ 404.
	bases basesExporter
	// external persiste el almacén S3 de terceros de cada organización (mig 0023).
	// Nil ⇒ su plano de administración responde 404.
	external externalStorageStore
	// externalUsage guarda lo que las otras apps de la suite ocupan en la cuota
	// (mig 0026). Nil ⇒ la puerta interna no se monta (ver server.go).
	externalUsage externalUsageStore
}

// Asegura en compilación que *service.Service satisface driveService.
var _ driveService = (*service.Service)(nil)

// toAPIError traduce errores de dominio a *httpx.Error canónicos.
func toAPIError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, domain.ErrNotFound):
		return httpx.ErrNotFound
	case errors.Is(err, domain.ErrConflict):
		return httpx.ErrConflict.WithMessage("A resource with that name already exists.")
	case errors.Is(err, domain.ErrInvalidName):
		return httpx.ErrBadRequest.WithMessage("Invalid name or parameter.")
	case errors.Is(err, domain.ErrInvalidParent):
		return httpx.ErrBadRequest.WithMessage("Invalid destination folder.")
	case errors.Is(err, service.ErrExternalStorageOff):
		return httpx.ErrUnavailable.WithCode("external_storage_unavailable").
			WithMessage("Connected storage is unavailable. Try again later.").Wrap(err)
	case errors.Is(err, service.ErrPresignUnsupported):
		// El almacén de terceros del tenant (WebDAV) no firma URLs: la UI cae a
		// la subida por Drive al ver cualquier error que no sea de cuota.
		return httpx.ErrUnprocessable.WithMessage("Direct upload is not available for this storage.")
	case errors.Is(err, domain.ErrQuotaExceeded):
		return httpx.NewError(http.StatusInsufficientStorage, "quota_exceeded", "Storage quota exceeded.")
	case errors.Is(err, domain.ErrCycle):
		return httpx.ErrBadRequest.WithMessage("A folder cannot be moved into itself.")
	case errors.Is(err, domain.ErrRootImmutable):
		return httpx.ErrBadRequest.WithMessage("The root folder cannot be modified.")
	case errors.Is(err, domain.ErrBatchTooLarge):
		return httpx.ErrBadRequest.WithMessage("The selection is too large. Make it smaller and try again.")
	case errors.Is(err, domain.ErrUnsupportedMedia):
		return httpx.NewError(http.StatusUnsupportedMediaType, "unsupported_media", "A thumbnail cannot be generated for this file.")
	case errors.Is(err, domain.ErrShareExpired):
		return httpx.ErrNotFound.WithMessage("This link does not exist or has expired.")
	case errors.Is(err, domain.ErrPasswordRequired), errors.Is(err, domain.ErrWrongPassword):
		return httpx.NewError(http.StatusUnauthorized, "password_required", "This link requires a password.")
	case errors.Is(err, domain.ErrLabelBlocksSharing):
		// 403 con el MOTIVO y la salida: la UI explica por qué no hay enlace y qué sí
		// se puede hacer, en vez de dar un «no» sin razón. El nombre de la etiqueta lo
		// tiene ya el cliente (viaja en el propio fichero).
		return httpx.ErrForbidden.WithMessage(
			"The sensitivity label on this file prevents sharing it outside the organisation. " +
				"You can still share it with people or groups inside your organisation.",
		)
	case errors.Is(err, domain.ErrPermission):
		return httpx.ErrForbidden
	case errors.Is(err, domain.ErrLastManager):
		return httpx.ErrConflict.WithMessage("That would leave the drive without a manager.")
	case errors.Is(err, domain.ErrLegalHold):
		return httpx.ErrConflict.WithMessage("This file is under legal hold and cannot be permanently deleted.")
	default:
		return httpx.ErrInternal.Wrap(err)
	}
}

// parseID extrae y valida el UUID del parámetro de ruta "id".
func parseID(r *http.Request) (uuid.UUID, error) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		return uuid.Nil, httpx.ErrBadRequest.WithMessage("Invalid id.")
	}
	return id, nil
}

// parseVersion extrae y valida el número de versión del parámetro de ruta.
func parseVersion(r *http.Request) (int, error) {
	n, err := strconv.Atoi(chi.URLParam(r, "version"))
	if err != nil || n <= 0 {
		return 0, httpx.ErrBadRequest.WithMessage("Invalid version number.")
	}
	return n, nil
}

// subFromCtx devuelve el subject del usuario autenticado.
func subFromCtx(r *http.Request) string {
	if c, ok := authn.FromContext(r.Context()); ok {
		return c.Subject
	}
	return ""
}

// jwtGroups extrae el claim "groups" del JWT Bearer YA VERIFICADO por RequireAuth.
// authn.Claims no expone grupos (y platform/pkg/authn queda fuera de este cambio),
// así que releemos el payload del MISMO token —cuya firma/iss/exp ya validó el
// middleware antes de llegar aquí— para leer un claim adicional. No re-verifica (no
// hace falta: el handler solo corre tras RequireAuth) y "groups" no es más sensible
// que sub/roles, que authn ya toma del mismo token. Si el claim falta (Sytue ID aún
// no lo emite) o no hay token, devuelve nil: los shares de grupo no conceden acceso
// (deny-by-default), correcto hasta que ID emita grupos en el token.
func jwtGroups(r *http.Request) []string {
	const prefix = "Bearer "
	h := r.Header.Get("Authorization")
	if len(h) <= len(prefix) || !strings.EqualFold(h[:len(prefix)], prefix) {
		return nil
	}
	parts := strings.Split(strings.TrimSpace(h[len(prefix):]), ".")
	if len(parts) != 3 {
		return nil
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil
	}
	var claims struct {
		Groups any `json:"groups"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil
	}
	return toStringClaim(claims.Groups)
}

// toStringClaim normaliza un claim que puede venir como []any, []string o string
// separado por espacios (mismo criterio que authn.toStringSlice, no exportado).
func toStringClaim(v any) []string {
	switch t := v.(type) {
	case []any:
		out := make([]string, 0, len(t))
		for _, e := range t {
			if s, ok := e.(string); ok && s != "" {
				out = append(out, s)
			}
		}
		return out
	case string:
		return strings.Fields(t)
	default:
		return nil
	}
}

// ---------------------------------------------------------------------------
// Carpetas
// ---------------------------------------------------------------------------

// GetRoot devuelve (o crea) la carpeta raíz del tenant.
func (h *Handlers) GetRoot(w http.ResponseWriter, r *http.Request) {
	root, err := h.svc.Root(r.Context())
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, root)
}

// GetFolder devuelve una carpeta por ID.
func (h *Handlers) GetFolder(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	f, err := h.svc.GetFolder(r.Context(), id)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, f)
}

// listResponse es la vista de los hijos de una carpeta (y de la papelera, que
// no pagina y omite next_cursor).
type listResponse struct {
	Folders    []domain.Folder `json:"folders"`
	Files      []domain.File   `json:"files"`
	NextCursor string          `json:"next_cursor,omitempty"`
}

// ListChildren lista subcarpetas y ficheros de una carpeta. Los ficheros van
// con paginación keyset por (lower(name), id) — ?cursor=<opaco>&limit=N y
// next_cursor en la respuesta cuando hay más (CONVENTIONS §0.8: sin OFFSET ni
// listados sin LIMIT). Las subcarpetas solo viajan en la primera página.
func (h *Handlers) ListChildren(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	cursor := strings.TrimSpace(r.URL.Query().Get("cursor"))
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 {
		// ponytail: 500 (el máximo del store) por defecto, no 100: colchón para
		// clientes que aún no siguen next_cursor; bajar cuando el front pagine.
		limit = 500
	}
	folders := []domain.Folder{}
	if cursor == "" {
		folders, err = h.svc.ListSubfolders(r.Context(), id)
		if err != nil {
			httpx.Fail(w, r, toAPIError(err))
			return
		}
		if folders == nil {
			folders = []domain.Folder{}
		}
	}
	page, err := h.svc.ListFilesPage(r.Context(), id, cursor, limit)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	files := page.Files
	if files == nil {
		files = []domain.File{}
	}
	httpx.JSON(w, r, http.StatusOK, listResponse{Folders: folders, Files: files, NextCursor: page.NextCursor})
}

// ListFolderFiles lista los ficheros de una carpeta con paginación keyset
// (?cursor=<opaco>&limit=N). Devuelve `next_cursor` cuando hay más resultados;
// un cursor vacío o ausente pide la primera página. Pensado para carpetas con
// muchos ficheros (CONVENTIONS §0.8: keyset, sin OFFSET).
func (h *Handlers) ListFolderFiles(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	cursor := strings.TrimSpace(r.URL.Query().Get("cursor"))
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	page, err := h.svc.ListFilesPage(r.Context(), id, cursor, limit)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	files := page.Files
	if files == nil {
		files = []domain.File{}
	}
	resp := map[string]any{"files": files}
	if page.NextCursor != "" {
		resp["next_cursor"] = page.NextCursor
	}
	httpx.JSON(w, r, http.StatusOK, resp)
}

type createFolderRequest struct {
	ParentID    string `json:"parent_id"`
	Name        string `json:"name"`
	GroupFolder bool   `json:"group_folder"`
}

// CreateFolder crea una subcarpeta.
func (h *Handlers) CreateFolder(w http.ResponseWriter, r *http.Request) {
	var req createFolderRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	parent, err := uuid.Parse(req.ParentID)
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid parent_id."))
		return
	}
	f, err := h.svc.CreateFolder(r.Context(), parent, req.Name, subFromCtx(r), req.GroupFolder)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.JSON(w, r, http.StatusCreated, f)
}

type updateFolderRequest struct {
	Name        *string `json:"name,omitempty"`
	NewParentID *string `json:"new_parent_id,omitempty"`
}

// UpdateFolder renombra o mueve una carpeta (operaciones mutuamente exclusivas).
func (h *Handlers) UpdateFolder(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var req updateFolderRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	switch {
	case req.Name != nil && req.NewParentID == nil:
		f, err := h.svc.RenameFolder(r.Context(), id, *req.Name)
		if err != nil {
			httpx.Fail(w, r, toAPIError(err))
			return
		}
		httpx.JSON(w, r, http.StatusOK, f)
	case req.NewParentID != nil && req.Name == nil:
		np, err := uuid.Parse(*req.NewParentID)
		if err != nil {
			httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid new_parent_id."))
			return
		}
		f, err := h.svc.MoveFolder(r.Context(), id, np)
		if err != nil {
			httpx.Fail(w, r, toAPIError(err))
			return
		}
		httpx.JSON(w, r, http.StatusOK, f)
	default:
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Give exactly one: name (rename) or new_parent_id (move)."))
	}
}

// DeleteFolder borra una carpeta y su subárbol.
func (h *Handlers) DeleteFolder(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if err := h.svc.DeleteFolder(r.Context(), id); err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.NoContent(w)
}

// ---------------------------------------------------------------------------
// Ficheros
// ---------------------------------------------------------------------------

// UploadFile sube un fichero a una carpeta. Acepta el cuerpo crudo
// (application/octet-stream) con el nombre en la cabecera X-File-Name o el
// parámetro de query `name`. El Content-Type del cuerpo se conserva.
func (h *Handlers) UploadFile(w http.ResponseWriter, r *http.Request) {
	folderID, err := parseID(r)
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

	f, err := h.svc.UploadFile(r.Context(), folderID, name, contentType, subFromCtx(r), body)
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

type presignRequest struct {
	Path        string `json:"path"`
	Size        int64  `json:"size"`
	ContentType string `json:"content_type"`
}

type presignResponse struct {
	URL       string `json:"url"`
	Handle    string `json:"handle"`
	ExpiresAt string `json:"expires_at"`
}

// Presign autoriza una subida directa a S3: comprueba permiso (acceso a la carpeta
// destino) y cuota, y devuelve una URL PUT prefirmada acotada (clave+método+
// caducidad) + un handle firmado para finalize. Los bytes van del cliente a S3 sin
// pasar por el app-server (descarga la subida de ficheros grandes). POST con cuerpo
// JSON; autenticado por Bearer (sin cookies → sin CSRF en el servicio). Tras subir,
// el cliente llama a Finalize. Es una vía ADICIONAL: la subida por PUT /content
// sigue como fallback. POST /files/presign.
func (h *Handlers) Presign(w http.ResponseWriter, r *http.Request) {
	var req presignRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if strings.TrimSpace(req.Path) == "" {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("The destination path is missing."))
		return
	}
	res, err := h.svc.PresignUpload(r.Context(), req.Path, req.Size, req.ContentType, subFromCtx(r))
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, presignResponse{
		URL:       res.URL,
		Handle:    res.Handle,
		ExpiresAt: res.ExpiresAt.UTC().Format(time.RFC3339),
	})
}

type finalizeRequest struct {
	Handle string `json:"handle"`
}

// Finalize confirma una subida directa: verifica en S3 (HEAD: existe + tamaño),
// registra el metadato reservando cuota y ata el blob. Idempotente (reintento →
// mismo fichero). POST /files/finalize.
func (h *Handlers) Finalize(w http.ResponseWriter, r *http.Request) {
	var req finalizeRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if strings.TrimSpace(req.Handle) == "" {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("The upload handle is missing."))
		return
	}
	f, err := h.svc.FinalizeUpload(r.Context(), req.Handle)
	if err != nil {
		httpx.Fail(w, r, finalizeError(err))
		return
	}
	httpx.JSON(w, r, http.StatusCreated, f)
}

// finalizeError mapea los errores propios de la finalización a códigos HTTP; el
// resto (cuota, conflicto de nombre, carpeta inválida...) cae en toAPIError.
func finalizeError(err error) error {
	switch {
	case errors.Is(err, service.ErrPresignHandleInvalid):
		return httpx.ErrBadRequest.WithMessage("The upload authorisation is not valid or has expired.")
	case errors.Is(err, service.ErrUploadNotFound):
		return httpx.NewError(http.StatusConflict, "upload_not_found", "The upload did not finish. Try again.")
	case errors.Is(err, service.ErrUploadSizeMismatch):
		return httpx.ErrBadRequest.WithMessage("The uploaded size does not match what was authorised.")
	default:
		return toAPIError(err)
	}
}

// fileNameFromRequest extrae el nombre del fichero de la cabecera o el query.
//
// `?name=` PRIMERO. Una cabecera HTTP solo puede llevar bytes, y un nombre de
// fichero es Unicode: quien lo mete en `X-File-Name` acaba mandando la tilde en
// Latin-1 —un solo byte— y aquí llega una cadena que no es UTF-8 válido. El
// query no tiene ese problema porque va escapado. Se sigue aceptando la
// cabecera porque hay clientes ya instalados que la usan, pero cuando vienen
// las dos manda la que no rompe.
func fileNameFromRequest(r *http.Request) string {
	if n := strings.TrimSpace(r.URL.Query().Get("name")); n != "" {
		return n
	}
	return repararUTF8(strings.TrimSpace(r.Header.Get("X-File-Name")))
}

// repararUTF8 devuelve un nombre que se puede guardar.
//
// POR QUÉ HACE FALTA. Una cadena inválida no se queda en un nombre feo: la
// columna es `text` y Postgres RECHAZA el byte suelto («invalid byte sequence
// for encoding UTF8»), así que la subida entera se cae con un 500 que no dice
// nada. Y los tests no lo veían porque el almacén de los tests es de mentira y
// se traga cualquier cosa; solo aparecía contra la base de datos de verdad.
//
// Se reinterpreta como Latin-1 porque es EXACTAMENTE lo que produce el viaje
// por la cabecera: cada rúnica de U+0080 a U+00FF sale como su byte suelto, así
// que leerla de vuelta recupera el nombre original —«título», no «t?tulo»—. Lo
// que no cabía en Latin-1 nunca llegó a salir del cliente, así que aquí no hay
// nada que recuperar de ello.
func repararUTF8(s string) string {
	if utf8.ValidString(s) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s) * 2)
	for i := 0; i < len(s); i++ {
		b.WriteRune(rune(s[i]))
	}
	return b.String()
}

// GetFile devuelve el metadato de un fichero.
func (h *Handlers) GetFile(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	f, err := h.svc.GetFile(r.Context(), id)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	// La RUTA acompaña al metadato: el explorador navega por ruta, no por id, así que sin
	// ella un enlace canónico /files/{id} —el que componen la búsqueda federada del Hub y
	// «Compartir a Rooms»— no puede llevar a ninguna parte. Es aditiva: quien ya consumía
	// esta respuesta no se entera.
	//
	// Un fallo al resolverla NO tumba la respuesta: el metadato es válido igual y perderlo
	// entero por no poder situar el fichero sería peor que devolverlo sin ruta.
	path, perr := h.svc.FilePath(r.Context(), id)
	if perr != nil {
		httpx.JSON(w, r, http.StatusOK, f)
		return
	}
	httpx.JSON(w, r, http.StatusOK, struct {
		domain.File
		Path string `json:"path"`
	}{File: f, Path: path})
}

// DownloadFile transmite el contenido del fichero desde S3. Con ?presign=1
// responde una redirección a una URL prefirmada (descarga directa de S3).
func (h *Handlers) DownloadFile(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if r.URL.Query().Get("presign") == "1" {
		u, err := h.svc.PresignDownload(r.Context(), id)
		switch {
		case err == nil:
			http.Redirect(w, r, u.String(), http.StatusTemporaryRedirect)
			return
		case errors.Is(err, service.ErrPresignUnsupported):
			// Sin URL firmada (WebDAV del tenant): se sirve el contenido desde aquí.
		default:
			httpx.Fail(w, r, toAPIError(err))
			return
		}
	}

	f, rc, err := h.svc.OpenFileContent(r.Context(), id)
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

// Thumbnail sirve una miniatura pequeña (JPEG/PNG) de un fichero de imagen ráster,
// generándola perezosamente y cacheando la derivada (el servicio la gatea por RLS:
// un fichero inaccesible/ajeno → 404; un tipo no soportado → 415). Disposición
// SEGURA para pintarla en un <img>: inline + nosniff + CSP sandbox (una miniatura
// nunca ejecuta script). Cacheable con ETag (deriva del checksum): responde 304 a
// una revalidación con If-None-Match que coincida.
func (h *Handlers) Thumbnail(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	rc, contentType, etag, err := h.svc.Thumbnail(r.Context(), id)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	defer rc.Close()

	w.Header().Set("ETag", etag)
	w.Header().Set("Cache-Control", "private, max-age=300")
	if match := r.Header.Get("If-None-Match"); match != "" && match == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Disposition", "inline")
	w.Header().Set("Content-Security-Policy", "sandbox")
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, rc)
}

type updateFileRequest struct {
	Name        *string `json:"name,omitempty"`
	NewFolderID *string `json:"new_folder_id,omitempty"`
}

// UpdateFile renombra o mueve un fichero.
func (h *Handlers) UpdateFile(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var req updateFileRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	switch {
	case req.Name != nil && req.NewFolderID == nil:
		f, err := h.svc.RenameFile(r.Context(), id, *req.Name)
		if err != nil {
			httpx.Fail(w, r, toAPIError(err))
			return
		}
		httpx.JSON(w, r, http.StatusOK, f)
	case req.NewFolderID != nil && req.Name == nil:
		nf, err := uuid.Parse(*req.NewFolderID)
		if err != nil {
			httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid new_folder_id."))
			return
		}
		f, err := h.svc.MoveFile(r.Context(), id, nf)
		if err != nil {
			httpx.Fail(w, r, toAPIError(err))
			return
		}
		httpx.JSON(w, r, http.StatusOK, f)
	default:
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Give exactly one: name (rename) or new_folder_id (move)."))
	}
}

// DeleteFile borra un fichero.
func (h *Handlers) DeleteFile(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if err := h.svc.DeleteFile(r.Context(), id); err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.NoContent(w)
}

// copyRequest es el cuerpo (OPCIONAL) de una copia server-side: el destino. Si se
// omite, la copia va a la misma carpeta del origen (fichero) o al padre del origen
// (carpeta) — paridad Drive: la copia aparece junto al original.
type copyRequest struct {
	// DestFolderID: carpeta destino de una copia de FICHERO (vacío = misma carpeta).
	DestFolderID string `json:"dest_folder_id,omitempty"`
	// DestParentID: carpeta padre destino de una copia de CARPETA (vacío = mismo padre).
	DestParentID string `json:"dest_parent_id,omitempty"`
}

// decodeCopyBody lee el cuerpo de copia si viene (es opcional: una copia «en el
// sitio» no lleva cuerpo). DecodeJSON rechaza el cuerpo vacío, así que solo se
// invoca cuando hay contenido declarado.
func decodeCopyBody(w http.ResponseWriter, r *http.Request) (copyRequest, error) {
	var req copyRequest
	if r.ContentLength > 0 {
		if err := httpx.DecodeJSON(w, r, &req); err != nil {
			return copyRequest{}, err
		}
	}
	return req, nil
}

// CopyFile hace «una copia» server-side de un fichero (nueva fila que referencia el
// MISMO blob; sin duplicar bytes). POST /files/{id}/copy (CSRF: mutación). El
// miembro del tenant es editor en «Mi unidad» (como en RestoreFileVersion), así que
// concedemos PermWrite; la verja viewer/editor la impone el servicio. Cuerpo
// opcional {dest_folder_id}; sin él, copia en la misma carpeta.
func (h *Handlers) CopyFile(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	req, err := decodeCopyBody(w, r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	dest := uuid.Nil
	if req.DestFolderID != "" {
		dest, err = uuid.Parse(req.DestFolderID)
		if err != nil {
			httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid dest_folder_id."))
			return
		}
	}
	f, err := h.svc.CopyFile(r.Context(), id, dest, subFromCtx(r), domain.PermWrite)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.JSON(w, r, http.StatusCreated, f)
}

// CopyFolder copia una carpeta y su subárbol server-side (recursivo; cada fichero
// referencia su MISMO blob). POST /folders/{id}/copy (CSRF). Cuerpo opcional
// {dest_parent_id}; sin él, la copia es hermana del origen. Mismo gating que CopyFile.
func (h *Handlers) CopyFolder(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	req, err := decodeCopyBody(w, r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var destParent *uuid.UUID
	if req.DestParentID != "" {
		p, err := uuid.Parse(req.DestParentID)
		if err != nil {
			httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid dest_parent_id."))
			return
		}
		destParent = &p
	}
	f, err := h.svc.CopyFolder(r.Context(), id, destParent, subFromCtx(r), domain.PermWrite)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.JSON(w, r, http.StatusCreated, f)
}

// ---------------------------------------------------------------------------
// Operaciones en lote (mover/papelera N) y descarga ZIP
// ---------------------------------------------------------------------------

// maxBatchItems acota el nº de identificadores aceptados en una operación en lote
// (defensa contra peticiones abusivas; el árbol del ZIP tiene su propia cota).
const maxBatchItems = 1000

// batchRequest son los objetivos de una operación en lote: ficheros y carpetas por
// ID, más (solo en «mover») la carpeta destino (vacía = raíz «Mi unidad»).
type batchRequest struct {
	FileIDs        []string `json:"file_ids"`
	FolderIDs      []string `json:"folder_ids"`
	TargetFolderID string   `json:"target_folder_id,omitempty"`
}

// parseIDs valida una lista de UUID (ignora vacíos). Falla si alguno es inválido.
func parseIDs(raw []string) ([]uuid.UUID, error) {
	out := make([]uuid.UUID, 0, len(raw))
	for _, s := range raw {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		id, err := uuid.Parse(s)
		if err != nil {
			return nil, httpx.ErrBadRequest.WithMessage("Invalid id in the selection.")
		}
		out = append(out, id)
	}
	return out, nil
}

// batchTargets parsea ficheros+carpetas de una petición de lote, exigiendo al menos
// un objetivo y respetando el tope de tamaño.
func batchTargets(fileRaw, folderRaw []string) (fileIDs, folderIDs []uuid.UUID, err error) {
	if fileIDs, err = parseIDs(fileRaw); err != nil {
		return nil, nil, err
	}
	if folderIDs, err = parseIDs(folderRaw); err != nil {
		return nil, nil, err
	}
	switch n := len(fileIDs) + len(folderIDs); {
	case n == 0:
		return nil, nil, httpx.ErrBadRequest.WithMessage("Select at least one item.")
	case n > maxBatchItems:
		return nil, nil, httpx.ErrBadRequest.WithMessage("Too many items in the selection.")
	}
	return fileIDs, folderIDs, nil
}

// BatchMove mueve N ficheros/carpetas bajo una carpeta destino en UNA operación.
// target_folder_id vacío = raíz «Mi unidad». Mutación por POST (CSRF).
func (h *Handlers) BatchMove(w http.ResponseWriter, r *http.Request) {
	var req batchRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	fileIDs, folderIDs, err := batchTargets(req.FileIDs, req.FolderIDs)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	target, err := h.resolveTargetFolder(r.Context(), req.TargetFolderID)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	res, err := h.svc.BatchMove(r.Context(), fileIDs, folderIDs, target)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"moved_files": res.Files, "moved_folders": res.Folders})
}

// resolveTargetFolder devuelve la carpeta destino de un lote: el UUID indicado, o la
// raíz del tenant si viene vacío («Mi unidad»). El error ya es un *httpx.Error.
func (h *Handlers) resolveTargetFolder(ctx context.Context, raw string) (uuid.UUID, error) {
	if raw = strings.TrimSpace(raw); raw == "" {
		root, err := h.svc.Root(ctx)
		if err != nil {
			return uuid.Nil, toAPIError(err)
		}
		return root.ID, nil
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		return uuid.Nil, httpx.ErrBadRequest.WithMessage("Invalid target_folder_id.")
	}
	return id, nil
}

// BatchTrash envía N ficheros/carpetas a la papelera en UNA operación (soft delete;
// respeta el ciclo de papelera existente). Mutación por POST (CSRF).
func (h *Handlers) BatchTrash(w http.ResponseWriter, r *http.Request) {
	var req batchRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	fileIDs, folderIDs, err := batchTargets(req.FileIDs, req.FolderIDs)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	res, err := h.svc.BatchTrash(r.Context(), fileIDs, folderIDs)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"trashed_files": res.Files, "trashed_folders": res.Folders})
}

// BatchZip descarga una selección (o una carpeta entera) como ZIP en STREAMING. GET
// (descarga navegable; autenticado por Bearer, sin cuerpo → sin CSRF). ?file_ids= y
// ?folder_ids= son listas separadas por comas; ?name= nombra la descarga. El árbol
// accesible se reúne ANTES de escribir cabeceras (errores → HTTP); luego el ZIP se
// transmite directo al cuerpo (memoria O(1)). Disposición SEGURA (attachment +
// nosniff): un ZIP nunca se sirve inline.
func (h *Handlers) BatchZip(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	fileIDs, err := parseIDs(splitCSV(q.Get("file_ids")))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	folderIDs, err := parseIDs(splitCSV(q.Get("folder_ids")))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	switch n := len(fileIDs) + len(folderIDs); {
	case n == 0:
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Select at least one item."))
		return
	case n > maxBatchItems:
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Too many items in the selection."))
		return
	}
	entries, err := h.svc.ZipEntries(r.Context(), fileIDs, folderIDs)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	w.Header().Set("Content-Type", "application/zip")
	// Lo que suman los ficheros SIN comprimir. El ZIP va en streaming y comprimido, así
	// que su tamaño no se sabe antes de acabar (no hay Content-Length); con esto, quien
	// lo baja y lo va leyendo —la app nativa— enseña el progreso en porcentaje.
	w.Header().Set(ZipBytesHeader, strconv.FormatInt(zipContentBytes(entries), 10))
	w.Header().Set("Content-Disposition", "attachment; filename*=UTF-8''"+escapeFilename(zipDownloadName(q.Get("name"))))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "no-store")
	// Un árbol vacío (carpeta vacía o nada accesible) produce un ZIP vacío válido
	// (200): no distingue «vacío» de «sin acceso» sin más consultas, y un ZIP vacío
	// no filtra nada (deny-by-default).
	w.WriteHeader(http.StatusOK)
	if err := h.svc.StreamZip(r.Context(), w, entries); err != nil {
		// Cabeceras y parte del cuerpo ya enviadas: no se puede cambiar el estado.
		// Registramos; el ZIP del cliente quedará truncado.
		log.From(r.Context()).Error("drive: stream zip", "error", err)
	}
}

// ZipBytesHeader es la cabecera del ZIP con lo que suman sus ficheros sin comprimir.
const ZipBytesHeader = "X-Zip-Content-Bytes"

// zipContentBytes es lo que suman las entradas del ZIP sin comprimir.
func zipContentBytes(entries []service.ZipEntry) int64 {
	var n int64
	for _, e := range entries {
		n += e.Size
	}
	return n
}

// splitCSV parte una lista separada por comas en elementos no vacíos.
func splitCSV(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// zipDownloadName construye un nombre de descarga *.zip sensato a partir del ?name=
// (base, sin extensión): recorta, acota longitud y garantiza sufijo .zip.
// escapeFilename (RFC 5987) codifica el resultado en la cabecera, así que cualquier
// carácter especial viaja porcentaje-codificado (sin inyección de cabecera).
func zipDownloadName(raw string) string {
	base := strings.TrimSuffix(strings.TrimSpace(raw), ".zip")
	if base == "" {
		base = "sytue-drive"
	}
	if len(base) > 120 {
		base = base[:120]
	}
	return base + ".zip"
}

// ---------------------------------------------------------------------------
// Historial de versiones
// ---------------------------------------------------------------------------

// ListFileVersions lista el historial de un fichero, más nueva primero, con
// paginación keyset (?cursor=<opaco>&limit=N). La primera página encabeza con la
// versión ACTUAL (is_current). Un fichero inexistente/ajeno → 404 (RLS, sin fuga).
func (h *Handlers) ListFileVersions(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	cursor := strings.TrimSpace(r.URL.Query().Get("cursor"))
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	page, err := h.svc.FileVersions(r.Context(), id, cursor, limit)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	versions := page.Versions
	if versions == nil {
		versions = []domain.FileVersion{}
	}
	resp := map[string]any{"versions": versions}
	if page.NextCursor != "" {
		resp["next_cursor"] = page.NextCursor
	}
	httpx.JSON(w, r, http.StatusOK, resp)
}

// DownloadFileVersion transmite el blob de una versión archivada. Disposición
// SEGURA (attachment + nosniff, igual que DownloadFile): nunca inline. El nombre
// de descarga usa el nombre ACTUAL del fichero (GetFile gatea por RLS).
func (h *Handlers) DownloadFileVersion(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	version, err := parseVersion(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	f, err := h.svc.GetFile(r.Context(), id)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	v, rc, err := h.svc.OpenVersionContent(r.Context(), id, version)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	defer rc.Close()

	w.Header().Set("Content-Type", v.ContentType)
	w.Header().Set("Content-Length", strconv.FormatInt(v.SizeBytes, 10))
	w.Header().Set("Content-Disposition", "attachment; filename*=UTF-8''"+escapeFilename(f.Name))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if v.Checksum != "" {
		w.Header().Set("ETag", `"`+v.Checksum+`"`)
	}
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, rc)
}

// RestoreFileVersion hace ACTUAL una versión archivada (no destructivo). Escritura
// por POST (CSRF: mutación con cuerpo/ruta). En «Mi unidad» el miembro del tenant
// es editor del fichero (ya puede reemplazarlo), así que concedemos PermWrite; la
// verja viewer/editor (deny-by-default) la impone el servicio.
func (h *Handlers) RestoreFileVersion(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	version, err := parseVersion(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	f, err := h.svc.RestoreVersion(r.Context(), id, version, subFromCtx(r), domain.PermWrite)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, f)
}

// ---------------------------------------------------------------------------
// Papelera
// ---------------------------------------------------------------------------

// trashResponse es la papelera: los elementos borrados (cada uno con su
// deleted_at) más la RETENCIÓN vigente del servicio, en segundos. Con las dos
// cosas el cliente puede decir cuánto le queda a cada elemento antes de que el
// worker de auto-purga lo borre de verdad. 0 = auto-purga desactivada (lo
// borrado se conserva hasta que alguien lo elimine a mano).
type trashResponse struct {
	listResponse
	RetentionSeconds int64 `json:"retention_seconds"`
}

// ListTrash lista los elementos en la papelera del tenant (carpetas y ficheros
// borrados de nivel superior) con la retención vigente.
func (h *Handlers) ListTrash(w http.ResponseWriter, r *http.Request) {
	folders, files, err := h.svc.ListTrash(r.Context())
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	if folders == nil {
		folders = []domain.Folder{}
	}
	if files == nil {
		files = []domain.File{}
	}
	retention := int64(0)
	if h.cfg.TrashRetention > 0 {
		retention = int64(h.cfg.TrashRetention.Seconds())
	}
	httpx.JSON(w, r, http.StatusOK, trashResponse{
		listResponse:     listResponse{Folders: folders, Files: files},
		RetentionSeconds: retention,
	})
}

// RestoreFile restaura un fichero de la papelera a su carpeta.
func (h *Handlers) RestoreFile(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	f, err := h.svc.RestoreFile(r.Context(), id)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, f)
}

// RestoreFolder restaura una carpeta (y su subárbol) de la papelera.
func (h *Handlers) RestoreFolder(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	f, err := h.svc.RestoreFolder(r.Context(), id)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, f)
}

// PurgeFile elimina definitivamente un fichero de la papelera (borra el blob).
func (h *Handlers) PurgeFile(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if err := h.svc.PurgeFile(r.Context(), id); err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.NoContent(w)
}

// PurgeFolder elimina definitivamente una carpeta (y su subárbol) de la papelera.
func (h *Handlers) PurgeFolder(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if err := h.svc.PurgeFolder(r.Context(), id); err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.NoContent(w)
}

// Search busca ficheros por nombre (?q=...&limit=...).
func (h *Handlers) Search(w http.ResponseWriter, r *http.Request) {
	// `httpx.Consulta` Y NO `r.URL.Query()`: esta ruta está montada con
	// `GetYQuery`, y por QUERY los parámetros viajan en el CUERPO —la query
	// string llega vacía—. Leyéndolos de la URL, el buscador de Drive se quedaba sin
	// argumentos y respondía como si no le hubieran pedido nada. Pasó con el
	// buscador global del Hub, que devolvía 400 en cada pulsación.
	params, err := httpx.Consulta(w, r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	q := strings.TrimSpace(params.Get("q"))
	if q == "" {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Search parameter 'q' is required."))
		return
	}
	limit, _ := strconv.Atoi(params.Get("limit"))
	entries, err := h.svc.Search(r.Context(), q, limit)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	// nodeView (con la RUTA resuelta) y no domain.File: una búsqueda cruza todo el árbol,
	// así que sin ruta un resultado no se puede abrir ni situar. Es la misma vista que
	// «Recientes»/«Favoritos», que cruzan el árbol igual.
	//
	// La clave sigue siendo "files" a propósito: el conector del Hub
	// (services/hub/internal/connectors/drive.go) decodifica esta respuesta en una struct
	// PARCIAL, así que añadir campos no le afecta, pero renombrar la envoltura lo rompería.
	files := make([]nodeView, 0, len(entries))
	for _, e := range entries {
		files = append(files, nodeViewOf(e))
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"files": files})
}

// ---------------------------------------------------------------------------
// Recientes y Favoritos
// ---------------------------------------------------------------------------

// nodeView es la vista JSON plana de una entrada de «Recientes»/«Favoritos»: un
// fichero o carpeta con su RUTA absoluta ya resuelta (los listados cruzan todo el
// árbol y no son navegables por carpeta, así que la ruta viaja en la respuesta).
type nodeView struct {
	ID          uuid.UUID `json:"id"`
	Name        string    `json:"name"`
	IsDir       bool      `json:"is_directory"`
	Path        string    `json:"path"`
	SizeBytes   int64     `json:"size_bytes,omitempty"`
	ContentType string    `json:"content_type,omitempty"`
	UpdatedAt   time.Time `json:"updated_at"`
	// Label es el snapshot de etiqueta de sensibilidad, con la MISMA forma que en el
	// listado principal (ausente si el fichero no lleva ninguna). Va aquí para que
	// estas vistas avisen de un bloqueo de compartición ANTES de intentarlo, en vez
	// de descubrirlo con el 403 del servicio (que sigue siendo la verja real).
	Label *domain.Label `json:"label,omitempty"`
}

func nodeViewOf(e service.Entry) nodeView {
	return nodeView{
		ID: e.ID, Name: e.Name, IsDir: e.IsDir, Path: e.Path,
		SizeBytes: e.SizeBytes, ContentType: e.ContentType, UpdatedAt: e.UpdatedAt,
		Label: e.Label,
	}
}

// Recent lista los ficheros del tenant por reciente-primero (updated_at DESC)
// con paginación keyset (?cursor=<opaco>&limit=N). Devuelve `next_cursor` cuando
// hay más resultados.
func (h *Handlers) Recent(w http.ResponseWriter, r *http.Request) {
	cursor := strings.TrimSpace(r.URL.Query().Get("cursor"))
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	page, err := h.svc.Recent(r.Context(), cursor, limit)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	items := make([]nodeView, 0, len(page.Entries))
	for _, e := range page.Entries {
		items = append(items, nodeViewOf(e))
	}
	resp := map[string]any{"items": items}
	if page.NextCursor != "" {
		resp["next_cursor"] = page.NextCursor
	}
	httpx.JSON(w, r, http.StatusOK, resp)
}

// ListFavorites lista los favoritos vivos del usuario autenticado (carpetas y
// ficheros, con su ruta resuelta).
func (h *Handlers) ListFavorites(w http.ResponseWriter, r *http.Request) {
	entries, err := h.svc.Favorites(r.Context(), subFromCtx(r))
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	items := make([]nodeView, 0, len(entries))
	for _, e := range entries {
		items = append(items, nodeViewOf(e))
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

type setFavoriteRequest struct {
	FileID   string `json:"file_id,omitempty"`
	FolderID string `json:"folder_id,omitempty"`
	Favorite bool   `json:"favorite"`
}

// SetFavorite marca/desmarca un fichero o carpeta como favorito del usuario. POST
// (no DELETE) para el toggle: la mutación va por POST con cuerpo JSON.
func (h *Handlers) SetFavorite(w http.ResponseWriter, r *http.Request) {
	var req setFavoriteRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var fileID, folderID *uuid.UUID
	if req.FileID != "" {
		id, err := uuid.Parse(req.FileID)
		if err != nil {
			httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid file_id."))
			return
		}
		fileID = &id
	}
	if req.FolderID != "" {
		id, err := uuid.Parse(req.FolderID)
		if err != nil {
			httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid folder_id."))
			return
		}
		folderID = &id
	}
	if (fileID == nil) == (folderID == nil) {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Give exactly one: file_id or folder_id."))
		return
	}
	if err := h.svc.SetFavorite(r.Context(), subFromCtx(r), fileID, folderID, req.Favorite); err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"favorite": req.Favorite})
}

// ---------------------------------------------------------------------------
// Cuotas
// ---------------------------------------------------------------------------

// GetQuota devuelve la cuota del tenant.
func (h *Handlers) GetQuota(w http.ResponseWriter, r *http.Request) {
	q, err := h.svc.Quota(r.Context())
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.JSON(w, r, http.StatusOK, quotaPayload(q, h.svc.ExternalStorageEnabled(r.Context())))
}

func quotaPayload(q domain.Quota, externalStorage bool) map[string]any {
	baseLimit, baseUsed := q.LimitBytes, q.UsedBytes
	if q.ConnectedStorage {
		baseLimit, baseUsed = q.BaseLimitBytes, q.BaseUsedBytes
		externalStorage = true
	}
	out := map[string]any{
		"tenant":            q.Tenant,
		"limit_bytes":       q.LimitBytes,
		"used_bytes":        q.UsedBytes,
		"available_bytes":   q.Available(),
		"updated_at":        q.UpdatedAt,
		"base_limit_bytes":  baseLimit,
		"base_used_bytes":   baseUsed,
		"external_storage":  externalStorage,
		"external_capacity": nil,
		// Lo que ocupan las OTRAS apps de la suite en esta misma cuota, por app
		// («cast», mig 0026). used_bytes ya lo incluye: es el desglose del medidor,
		// y va siempre —vacío es `{}`, no ausente— para que el cliente no tenga
		// que distinguir «nadie ha declarado» de «este Drive es antiguo».
		"external_usage": externalUsageDe(q),
	}
	if externalStorage {
		out["external_capacity"] = map[string]any{
			"total_bytes":     q.StorageTotalBytes,
			"used_bytes":      q.StorageUsedBytes,
			"available_bytes": q.StorageAvailableBytes,
			"partial":         q.StorageCapacityPartial,
		}
	}
	return out
}

// externalUsageDe devuelve el desglose externo de la cuota, nunca nil.
func externalUsageDe(q domain.Quota) map[string]int64 {
	if q.ExternalUsage == nil {
		return map[string]int64{}
	}
	return q.ExternalUsage
}

type setQuotaRequest struct {
	LimitBytes int64 `json:"limit_bytes"`
}

// SetQuota fija el límite de cuota del tenant (admin/owner).
func (h *Handlers) SetQuota(w http.ResponseWriter, r *http.Request) {
	var req setQuotaRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if req.LimitBytes < 0 {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("limit_bytes cannot be negative."))
		return
	}
	q, err := h.svc.SetQuotaLimit(r.Context(), req.LimitBytes)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.JSON(w, r, http.StatusOK, quotaPayload(q, h.svc.ExternalStorageEnabled(r.Context())))
}

// ---------------------------------------------------------------------------
// Compartición
// ---------------------------------------------------------------------------

type createShareRequest struct {
	FileID     string `json:"file_id,omitempty"`
	FolderID   string `json:"folder_id,omitempty"`
	Kind       string `json:"kind"`
	Subject    string `json:"subject,omitempty"`
	Permission string `json:"permission,omitempty"`
	Password   string `json:"password,omitempty"`
	ExpiresAt  string `json:"expires_at,omitempty"`
}

// CreateShare crea una compartición (enlace, usuario o grupo).
func (h *Handlers) CreateShare(w http.ResponseWriter, r *http.Request) {
	var req createShareRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	in := service.CreateShareInput{
		Kind:       domain.ShareKind(req.Kind),
		Subject:    strings.TrimSpace(req.Subject),
		Permission: domain.Permission(req.Permission),
		Password:   req.Password,
		CreatedBy:  subFromCtx(r),
	}
	if !domain.ValidShareKind(in.Kind) {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("kind must be link, user or group."))
		return
	}
	if req.Permission != "" && !domain.ValidPermission(in.Permission) {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("permission must be read or write."))
		return
	}
	if req.FileID != "" {
		id, err := uuid.Parse(req.FileID)
		if err != nil {
			httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid file_id."))
			return
		}
		in.FileID = &id
	}
	if req.FolderID != "" {
		id, err := uuid.Parse(req.FolderID)
		if err != nil {
			httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid folder_id."))
			return
		}
		in.FolderID = &id
	}
	if (in.FileID == nil) == (in.FolderID == nil) {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Give exactly one: file_id or folder_id."))
		return
	}
	if req.ExpiresAt != "" {
		ts, err := time.Parse(time.RFC3339, req.ExpiresAt)
		if err != nil {
			httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("expires_at must be an RFC3339 date."))
			return
		}
		in.ExpiresAt = &ts
	}

	sh, err := h.svc.CreateShare(r.Context(), in)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	resp := map[string]any{"share": sh}
	// Para enlaces, devolver la URL pública completa (incluye el token una vez).
	if sh.Kind == domain.ShareLink && sh.Token != "" {
		resp["url"] = strings.TrimRight(h.cfg.PublicURL, "/") + "/public/" + sh.Token
	}
	httpx.JSON(w, r, http.StatusCreated, resp)
}

// ListFileShares lista las comparticiones de un fichero.
func (h *Handlers) ListFileShares(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	shares, err := h.svc.ListFileShares(r.Context(), id)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	if shares == nil {
		shares = []domain.Share{}
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"shares": shares})
}

// ListFolderShares lista las comparticiones de una carpeta.
func (h *Handlers) ListFolderShares(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	shares, err := h.svc.ListFolderShares(r.Context(), id)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	if shares == nil {
		shares = []domain.Share{}
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"shares": shares})
}

// RevokeShare revoca una compartición.
func (h *Handlers) RevokeShare(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if err := h.svc.RevokeShare(r.Context(), id); err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.NoContent(w)
}

// ---------------------------------------------------------------------------
// Compartido conmigo (shares kind=user|group dirigidos al usuario)
// ---------------------------------------------------------------------------

// sharedNodeView es nodeView + el permiso del share (viewer/editor) y quién lo
// compartió, para «Compartido conmigo»: el cliente pinta el dueño y oculta la
// escritura a los viewers.
type sharedNodeView struct {
	nodeView
	Permission string `json:"permission"`
	SharedBy   string `json:"shared_by,omitempty"`
	// EditRequested: en lectura, ya ha pedido editarlo (la franja lo dice).
	EditRequested bool `json:"edit_requested,omitempty"`
}

// permissionLabel traduce el permiso del share (read/write) al vocabulario de la
// UI de compartición (view/edit); un permiso desconocido cae a "view" (mínimo).
func permissionLabel(p domain.Permission) string {
	if p == domain.PermWrite {
		return "edit"
	}
	return "view"
}

// SharedWithMe lista los recursos compartidos CON el usuario (por su sub o sus
// grupos del JWT) vía shares kind=user|group vigentes, con su ruta resuelta, el
// permiso y quién lo compartió. Deny-by-default: sin shares, lista vacía.
func (h *Handlers) SharedWithMe(w http.ResponseWriter, r *http.Request) {
	entries, err := h.svc.SharedWithMe(r.Context(), subFromCtx(r), jwtGroups(r))
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	lectura := make([]uuid.UUID, 0, len(entries))
	for _, e := range entries {
		if e.Permission != domain.PermWrite {
			lectura = append(lectura, e.ID)
		}
	}
	// Una consulta para toda la lista; si falla, la franja solo no sabrá que ya lo pidió.
	pedidas, _ := h.svc.PedidasPor(r.Context(), subFromCtx(r), lectura)
	items := make([]sharedNodeView, 0, len(entries))
	for _, e := range entries {
		items = append(items, sharedNodeView{
			nodeView:      nodeViewOf(e.Entry),
			Permission:    permissionLabel(e.Permission),
			SharedBy:      e.SharedBy,
			EditRequested: pedidas[e.ID],
		})
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

// SharedList navega DENTRO de un recurso compartido (verja de acceso por share):
// GET /api/v1/shared/{id}/list?subpath=<sub>. Devuelve el permiso concedido y el
// listado (subcarpetas + primera página de ficheros) con rutas RELATIVAS a la raíz
// del share (el cliente las reenvía como ?subpath= para descender/descargar). Sin
// share vigente → 404 (deny-by-default, sin fugas).
func (h *Handlers) SharedList(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	subpath := strings.TrimSpace(r.URL.Query().Get("subpath"))
	node, perm, err := h.svc.SharedBrowse(r.Context(), subFromCtx(r), jwtGroups(r), id, subpath)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	items := make([]nodeView, 0)
	if node.IsDir {
		for _, sub := range node.Subfolders {
			items = append(items, nodeView{
				ID: sub.ID, Name: sub.Name, IsDir: true,
				Path: appendSubpath(subpath, sub.Name), UpdatedAt: sub.UpdatedAt,
			})
		}
		for _, f := range node.Files {
			items = append(items, nodeView{
				ID: f.ID, Name: f.Name, IsDir: false, Path: appendSubpath(subpath, f.Name),
				SizeBytes: f.SizeBytes, ContentType: f.ContentType, UpdatedAt: f.UpdatedAt,
				Label: f.Label,
			})
		}
	} else {
		// La raíz compartida es un fichero: no hay body que servir aquí (se sirve por
		// SharedContent); devolvemos su metadato como único elemento.
		if node.Body != nil {
			_ = node.Body.Close()
		}
		items = append(items, nodeView{
			ID: node.File.ID, Name: node.File.Name, IsDir: false, Path: subpath,
			SizeBytes: node.File.SizeBytes, ContentType: node.File.ContentType, UpdatedAt: node.File.UpdatedAt,
			Label: node.File.Label,
		})
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{
		"permission": permissionLabel(perm),
		"is_dir":     node.IsDir,
		"more":       node.More,
		"items":      items,
	})
}

// SharedContent transmite el contenido de un fichero DENTRO de un recurso
// compartido (verja de acceso por share): GET /api/v1/shared/{id}/content?subpath=.
// Sin share vigente → 404. El BFF aplica la política de disposition (inline seguro
// solo para tipos seguros); aquí siempre se sirve como descarga con nosniff.
func (h *Handlers) SharedContent(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	subpath := strings.TrimSpace(r.URL.Query().Get("subpath"))
	node, _, err := h.svc.SharedBrowse(r.Context(), subFromCtx(r), jwtGroups(r), id, subpath)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	if node.IsDir {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("That path is a folder, not a file."))
		return
	}
	defer node.Body.Close()
	w.Header().Set("Content-Type", node.File.ContentType)
	w.Header().Set("Content-Length", strconv.FormatInt(node.File.SizeBytes, 10))
	w.Header().Set("Content-Disposition", "attachment; filename*=UTF-8''"+escapeFilename(node.File.Name))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if node.File.Checksum != "" {
		w.Header().Set("ETag", `"`+node.File.Checksum+`"`)
	}
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, node.Body)
}

// SharedUpload sube un fichero DENTRO de una carpeta compartida (editor):
// POST /api/v1/shared/{id}/files?subpath=<carpeta>. Un viewer recibe 403; sin
// share vigente, 404. Mismo contrato de cuerpo/nombre que UploadFile.
func (h *Handlers) SharedUpload(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	subpath := strings.TrimSpace(r.URL.Query().Get("subpath"))
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

	f, err := h.svc.SharedUpload(r.Context(), subFromCtx(r), jwtGroups(r), id, subpath, name, contentType, subFromCtx(r), body)
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

// appendSubpath compone la subruta relativa de un hijo dentro de un recurso
// compartido (raíz "" → nombre; si no, subpath + "/" + nombre).
func appendSubpath(subpath, name string) string {
	if subpath == "" {
		return name
	}
	return subpath + "/" + name
}

// El endpoint público por enlace (fichero y carpeta, con verja de contraseña y
// listado navegable) vive en public.go.

// escapeFilename codifica un nombre de fichero para el parámetro filename* de
// Content-Disposition (RFC 5987: porcentaje-codificación sobre UTF-8).
func escapeFilename(name string) string {
	return url.PathEscape(name)
}
