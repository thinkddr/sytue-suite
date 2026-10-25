// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drivehttp

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/log"
	"git.sytue.eu/sytue/one/services/drive/internal/compat"
	"git.sytue.eu/sytue/one/services/drive/internal/config"
	"git.sytue.eu/sytue/one/services/drive/internal/domain"
)

// Capa de compatibilidad de ficheros: Sytue Drive respondiendo un subconjunto
// PRAGMÁTICO de la API de Google Drive v3 (/compat/google) y de Microsoft Graph
// files (/compat/graph), para que una integración escrita contra Google/Microsoft
// —con base URL configurable— funcione sin cambios. La traducción de modelos vive
// en internal/compat (pura y testeable); aquí solo hay handlers finos.
//
// AISLAMIENTO (crítico): estos handlers usan EXACTAMENTE el mismo driveService y
// el mismo contexto de petición que la API nativa. Nunca tocan el store en crudo,
// así que el tenant (RLS), las unidades de equipo y los permisos se aplican igual.
// La autenticación es el Bearer de Sytue ID (mismo authn): no se emula ningún
// flujo OAuth de Google/Microsoft.
//
// Lo que NO está implementado responde 501 con mensaje claro (nunca un 200 vacío):
// permissions/changes/comments/revisions de Google, subida resumable, y el resto
// de Graph (delta, search, sharing, la sintaxis /root:/ruta:/…).

// maxCompatPage acota la página de ambos dialectos al máximo que sirve el store
// (500); pedir más devuelve 500 elementos + token de continuación, que es lo que
// el protocolo espera. defaultCompatPage es el tamaño por defecto de Google.
const (
	maxCompatPage     = 500
	defaultCompatPage = 100
)

// compatHandlers sirve ambos dialectos sobre el mismo servicio que la API nativa.
type compatHandlers struct {
	svc driveService
	cfg config.Config
}

// Scopes de Google Drive que abren /compat/google a una app de terceros registrada con
// permisos de Google (cuenta de Google emulada, ver authn.RequireGoogleAPIs).
var (
	googleScopesLectura   = []string{googleScope + "drive.readonly"}
	googleScopesEscritura = []string{googleScope + "drive"}
)

const googleScope = "https://www.googleapis.com/auth/"

// mountCompat monta las dos capas. /compat/google lleva su propia verja
// (googleAuth), que admite además los tokens de apps de terceros con scopes de
// Google; /compat/graph sigue solo con la de la suite.
func mountCompat(r chi.Router, h *compatHandlers, googleAuth, auth func(http.Handler) http.Handler) {
	r.Route("/compat/google", func(g chi.Router) {
		g.Use(googleAuth)
		g.Get("/drive/v3/about", h.googleAbout)
		g.Get("/drive/v3/files", h.googleList)
		g.Get("/drive/v3/files/{id}", h.googleGet)
		g.Delete("/drive/v3/files/{id}", h.googleDelete)
		g.Post("/drive/v3/files", h.googleCreateMetadata)
		g.Patch("/drive/v3/files/{id}", h.googleUpdateMetadata)
		g.Post("/drive/v3/files/{id}/copy", h.googleCopy)
		g.Post("/upload/drive/v3/files", h.googleUpload)
		g.NotFound(h.googleNotImplemented)
		g.MethodNotAllowed(h.googleNotImplemented)
	})

	r.Route("/compat/graph", func(g chi.Router) {
		g.Use(auth)
		g.Get("/v1.0/me/drive", h.graphDrive)
		g.Get("/v1.0/me/drive/root", h.graphItem)
		g.Get("/v1.0/me/drive/root/children", h.graphChildren)
		g.Get("/v1.0/me/drive/items/{id}", h.graphItem)
		g.Get("/v1.0/me/drive/items/{id}/children", h.graphChildren)
		g.Get("/v1.0/me/drive/items/{id}/content", h.graphContent)
		g.Put("/v1.0/me/drive/items/{id}/content", h.graphPutContent)
		g.NotFound(h.graphNotImplemented)
		g.MethodNotAllowed(h.graphNotImplemented)
	})
}

// ---------------------------------------------------------------------------
// Respuestas de error en cada dialecto
// ---------------------------------------------------------------------------

// failGoogle responde el sobre de error de Google con el estado que la API nativa
// habría devuelto para ese mismo error de dominio.
func failGoogle(w http.ResponseWriter, r *http.Request, err error) {
	e := compatError(r, err)
	httpx.JSON(w, r, e.Status, compat.NewGoogleError(e.Status, e.Message))
}

// failGraph responde el sobre de error de Microsoft Graph.
func failGraph(w http.ResponseWriter, r *http.Request, err error) {
	e := compatError(r, err)
	httpx.JSON(w, r, e.Status, compat.NewGraphError(e.Status, e.Message))
}

// compatError normaliza el error a *httpx.Error (los de dominio, con el MISMO
// mapeo de códigos que la API nativa) y registra los 5xx sin filtrar la causa al
// cliente.
func compatError(r *http.Request, err error) *httpx.Error {
	var e *httpx.Error
	if !errors.As(err, &e) {
		e = httpx.AsError(toAPIError(err))
	}
	if e.Status >= http.StatusInternalServerError {
		log.From(r.Context()).Error("compat: error de servidor", "code", e.Code, "error", e.Error())
	}
	return e
}

// errNotImplemented describe un endpoint del dialecto que no se emula.
func errNotImplemented(dialect string) error {
	return httpx.NewError(http.StatusNotImplemented, "not_implemented",
		"Sytue Drive emula un subconjunto de "+dialect+"; este endpoint no está implementado.")
}

func (h *compatHandlers) googleNotImplemented(w http.ResponseWriter, r *http.Request) {
	failGoogle(w, r, errNotImplemented("la API de Google Drive v3"))
}

func (h *compatHandlers) graphNotImplemented(w http.ResponseWriter, r *http.Request) {
	failGraph(w, r, errNotImplemented("Microsoft Graph (files)"))
}

// ---------------------------------------------------------------------------
// Resolución de elementos (compartida por ambos dialectos)
// ---------------------------------------------------------------------------

// compatNode es un elemento resuelto por id: fichero o carpeta.
type compatNode struct {
	file   domain.File
	folder domain.Folder
	isDir  bool
}

// resolveNode resuelve un id de cualquiera de los dialectos ("root" = raíz del
// tenant) a través del SERVICIO: un id que el usuario no puede ver devuelve
// ErrNotFound igual que en la API nativa (sin distinguir «ajeno» de «inexistente»).
// Ficheros y carpetas tienen espacios de id distintos, así que se prueba primero
// fichero y luego carpeta.
func (h *compatHandlers) resolveNode(ctx context.Context, id string) (compatNode, error) {
	if id == "" || id == compat.RootAlias {
		root, err := h.svc.Root(ctx)
		if err != nil {
			return compatNode{}, err
		}
		return compatNode{folder: root, isDir: true}, nil
	}
	uid, err := uuid.Parse(id)
	if err != nil {
		return compatNode{}, domain.ErrNotFound
	}
	f, err := h.svc.GetFile(ctx, uid)
	if err == nil {
		return compatNode{file: f}, nil
	}
	if !errors.Is(err, domain.ErrNotFound) {
		return compatNode{}, err
	}
	d, err := h.svc.GetFolder(ctx, uid)
	if err != nil {
		return compatNode{}, err
	}
	return compatNode{folder: d, isDir: true}, nil
}

// resolveFolderID traduce un id de carpeta ("root" incluido) a UUID sin cargarla.
func (h *compatHandlers) resolveFolderID(ctx context.Context, id string) (uuid.UUID, error) {
	if id == "" || id == compat.RootAlias {
		root, err := h.svc.Root(ctx)
		if err != nil {
			return uuid.Nil, err
		}
		return root.ID, nil
	}
	uid, err := uuid.Parse(id)
	if err != nil {
		return uuid.Nil, domain.ErrNotFound
	}
	return uid, nil
}

// listFolder devuelve una página de hijos de una carpeta: las subcarpetas solo en
// la PRIMERA página (igual que la API nativa: el store no las pagina) y una página
// keyset de ficheros con su token de continuación.
func (h *compatHandlers) listFolder(ctx context.Context, folderID uuid.UUID, token string, size int, wantFolders, wantFiles bool) ([]domain.Folder, []domain.File, string, error) {
	var folders []domain.Folder
	if wantFolders && token == "" {
		var err error
		folders, err = h.svc.ListSubfolders(ctx, folderID)
		if err != nil {
			return nil, nil, "", err
		}
	}
	if !wantFiles {
		return folders, nil, "", nil
	}
	page, err := h.svc.ListFilesPage(ctx, folderID, token, size)
	if err != nil {
		return nil, nil, "", err
	}
	return folders, page.Files, page.NextCursor, nil
}

// compatPageSize acota el tamaño de página pedido por el cliente.
func compatPageSize(raw string) int {
	n, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || n <= 0 {
		return defaultCompatPage
	}
	if n > maxCompatPage {
		return maxCompatPage
	}
	return n
}

// ---------------------------------------------------------------------------
// Google Drive v3 — /compat/google
// ---------------------------------------------------------------------------

// googleAbout responde about.get: identidad del usuario autenticado y cuota REAL
// del tenant (si el servicio no la conoce, storageQuota se omite). El parámetro
// `fields` se IGNORA a propósito: siempre se devuelve el recurso completo, que es
// un superconjunto de lo que el cliente pide.
func (h *compatHandlers) googleAbout(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	c, _ := authn.FromContext(r.Context())
	var quota *domain.Quota
	if q, err := h.svc.Quota(r.Context()); err == nil {
		quota = &q
	}
	httpx.JSON(w, r, http.StatusOK, compat.NewGoogleAbout(c.Subject, c.Name, c.Email, quota))
}

// googleList responde files.list. Soporta `pageSize`/`pageToken`/`nextPageToken` y
// el subconjunto de `q` documentado en compat.Query; cualquier otro filtro es 400
// con el término exacto (nunca resultados a medias en silencio).
//
// Sin `'<id>' in parents` el listado es transversal (todos los ficheros del ámbito
// del usuario, recientes primero) y por eso NO incluye carpetas: pedirlas sin
// carpeta padre es 400 explícito en vez de devolver una lista vacía engañosa.
func (h *compatHandlers) googleList(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	q, err := compat.ParseQuery(r.URL.Query().Get("q"))
	if err != nil {
		failGoogle(w, r, httpx.ErrBadRequest.WithMessage(err.Error()))
		return
	}
	if q.TrashedSet && q.Trashed {
		failGoogle(w, r, httpx.ErrBadRequest.WithMessage("The emulation does not list the trash; use `trashed = false` or the native API /api/v1/trash."))
		return
	}
	size := compatPageSize(r.URL.Query().Get("pageSize"))
	token := strings.TrimSpace(r.URL.Query().Get("pageToken"))

	var out []compat.GoogleFile
	var next string

	if q.ParentID == "" {
		if q.FoldersOnly() {
			failGoogle(w, r, httpx.ErrBadRequest.WithMessage("To list folders, give the parent folder: q=\"'<id>' in parents and mimeType = 'application/vnd.google-apps.folder'\"."))
			return
		}
		page, err := h.svc.Recent(ctx, token, size)
		if err != nil {
			failGoogle(w, r, err)
			return
		}
		for _, e := range page.Entries {
			if g := compat.GoogleNode(e.ID, e.Name, e.IsDir, e.SizeBytes, e.ContentType, e.UpdatedAt); q.Matches(g) {
				out = append(out, g)
			}
		}
		next = page.NextCursor
	} else {
		parent, err := h.resolveFolderID(ctx, q.ParentID)
		if err != nil {
			failGoogle(w, r, err)
			return
		}
		folders, files, nextCursor, err := h.listFolder(ctx, parent, token, size, !q.FilesOnly(), !q.FoldersOnly())
		if err != nil {
			failGoogle(w, r, err)
			return
		}
		for _, d := range folders {
			if g := compat.GoogleFromFolder(d); q.Matches(g) {
				out = append(out, g)
			}
		}
		for _, f := range files {
			if g := compat.GoogleFromFile(f); q.Matches(g) {
				out = append(out, g)
			}
		}
		next = nextCursor
	}

	// Una página puede quedar vacía tras filtrar y aun así traer nextPageToken: es
	// el contrato de Google (el cliente sigue paginando hasta que no venga token).
	httpx.JSON(w, r, http.StatusOK, compat.NewGoogleFileList(out, next))
}

// googleGet responde files.get: metadatos o, con ?alt=media, el contenido.
func (h *compatHandlers) googleGet(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if r.URL.Query().Get("alt") == "media" {
		h.serveNodeContent(w, r, id, failGoogle)
		return
	}
	node, err := h.resolveNode(r.Context(), id)
	if err != nil {
		failGoogle(w, r, err)
		return
	}
	if node.isDir {
		httpx.JSON(w, r, http.StatusOK, compat.GoogleFromFolder(node.folder))
		return
	}
	httpx.JSON(w, r, http.StatusOK, compat.GoogleFromFile(node.file))
}

// googleDelete responde files.delete enviando el elemento a la PAPELERA (el
// borrado del servicio: reversible y auditado), no purgando bytes.
func (h *compatHandlers) googleDelete(w http.ResponseWriter, r *http.Request) {
	node, err := h.resolveNode(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		failGoogle(w, r, err)
		return
	}
	if node.isDir {
		err = h.svc.DeleteFolder(r.Context(), node.folder.ID)
	} else {
		err = h.svc.DeleteFile(r.Context(), node.file.ID)
	}
	if err != nil {
		failGoogle(w, r, err)
		return
	}
	httpx.NoContent(w)
}

// googleFileMetadata es el cuerpo de files.create, update y copy que la emulación
// escribe. Un campo más (description, starred, appProperties…) es 400 por su nombre:
// guardarlo a medias haría creer al cliente que quedó guardado.
type googleFileMetadata struct {
	Name     *string  `json:"name"`
	MimeType string   `json:"mimeType"`
	Parents  []string `json:"parents"`
}

func decodeGoogleMetadata(w http.ResponseWriter, r *http.Request) (googleFileMetadata, error) {
	var m googleFileMetadata
	if r.ContentLength == 0 {
		return m, nil
	}
	if err := httpx.DecodeJSON(w, r, &m); err != nil {
		if _, field, ok := strings.Cut(err.Error(), "unknown field "); ok {
			return m, httpx.ErrBadRequest.WithMessage("The body carries a field the emulation does not write: " + field + ".")
		}
		return m, err
	}
	return m, nil
}

// googleCreateMetadata responde files.create sin contenido: con el mimeType de
// carpeta crea una carpeta (lo habitual) y con cualquier otro, un fichero vacío.
func (h *compatHandlers) googleCreateMetadata(w http.ResponseWriter, r *http.Request) {
	m, err := decodeGoogleMetadata(w, r)
	if err != nil {
		failGoogle(w, r, err)
		return
	}
	if m.Name == nil || strings.TrimSpace(*m.Name) == "" {
		failGoogle(w, r, httpx.ErrBadRequest.WithMessage("The metadata must include `name`."))
		return
	}
	padre := ""
	if len(m.Parents) > 0 {
		padre = m.Parents[0]
	}
	if m.MimeType != compat.FolderMIME {
		h.googleStore(w, r, padre, *m.Name, m.MimeType, strings.NewReader(""))
		return
	}
	parentID, err := h.resolveFolderID(r.Context(), padre)
	if err != nil {
		failGoogle(w, r, err)
		return
	}
	d, err := h.svc.CreateFolder(r.Context(), parentID, *m.Name, subFromCtx(r), false)
	if err != nil {
		failGoogle(w, r, err)
		return
	}
	httpx.JSON(w, r, http.StatusOK, compat.GoogleFromFolder(d))
}

// googleUpdateMetadata responde files.update sin contenido: `name` renombra y
// `addParents` mueve. Un elemento de Sytue tiene UN padre, así que mover es cambiar de
// padre: `removeParents`, si viene, tiene que ser el actual.
func (h *compatHandlers) googleUpdateMetadata(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	m, err := decodeGoogleMetadata(w, r)
	if err != nil {
		failGoogle(w, r, err)
		return
	}
	if len(m.Parents) > 0 || m.MimeType != "" {
		failGoogle(w, r, httpx.ErrBadRequest.WithMessage("files.update cannot change parents or mimeType in the body; move with addParents."))
		return
	}
	node, err := h.resolveNode(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		failGoogle(w, r, err)
		return
	}
	ctx := r.Context()
	if destino := strings.TrimSpace(q.Get("addParents")); destino != "" {
		if strings.Contains(destino, ",") {
			failGoogle(w, r, httpx.ErrBadRequest.WithMessage("An item in Sytue Drive has a single parent: addParents takes one folder."))
			return
		}
		actual := node.file.FolderID
		if node.isDir && node.folder.ParentID != nil {
			actual = *node.folder.ParentID
		}
		if quitar := strings.TrimSpace(q.Get("removeParents")); quitar != "" && quitar != actual.String() {
			failGoogle(w, r, httpx.ErrBadRequest.WithMessage("removeParents must be the current parent."))
			return
		}
		folderID, err := h.resolveFolderID(ctx, destino)
		if err != nil {
			failGoogle(w, r, err)
			return
		}
		if node.isDir {
			node.folder, err = h.svc.MoveFolder(ctx, node.folder.ID, folderID)
		} else {
			node.file, err = h.svc.MoveFile(ctx, node.file.ID, folderID)
		}
		if err != nil {
			failGoogle(w, r, err)
			return
		}
	}
	if m.Name != nil {
		if node.isDir {
			node.folder, err = h.svc.RenameFolder(ctx, node.folder.ID, *m.Name)
		} else {
			node.file, err = h.svc.RenameFile(ctx, node.file.ID, *m.Name)
		}
		if err != nil {
			failGoogle(w, r, err)
			return
		}
	}
	if node.isDir {
		httpx.JSON(w, r, http.StatusOK, compat.GoogleFromFolder(node.folder))
		return
	}
	httpx.JSON(w, r, http.StatusOK, compat.GoogleFromFile(node.file))
}

// googleCopy responde files.copy: la copia va a `parents[0]` o junto al original, y
// con `name` se renombra. Como en Google, las carpetas no se copian por aquí.
func (h *compatHandlers) googleCopy(w http.ResponseWriter, r *http.Request) {
	m, err := decodeGoogleMetadata(w, r)
	if err != nil {
		failGoogle(w, r, err)
		return
	}
	node, err := h.resolveNode(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		failGoogle(w, r, err)
		return
	}
	if node.isDir {
		failGoogle(w, r, httpx.ErrBadRequest.WithMessage("Folders cannot be copied through files.copy."))
		return
	}
	destino := node.file.FolderID
	if len(m.Parents) > 0 {
		if destino, err = h.resolveFolderID(r.Context(), m.Parents[0]); err != nil {
			failGoogle(w, r, err)
			return
		}
	}
	f, err := h.svc.CopyFile(r.Context(), node.file.ID, destino, subFromCtx(r), domain.PermWrite)
	if err != nil {
		failGoogle(w, r, err)
		return
	}
	if m.Name != nil && strings.TrimSpace(*m.Name) != "" && *m.Name != f.Name {
		if f, err = h.svc.RenameFile(r.Context(), f.ID, *m.Name); err != nil {
			failGoogle(w, r, err)
			return
		}
	}
	httpx.JSON(w, r, http.StatusOK, compat.GoogleFromFile(f))
}

// googleUpload responde a POST /upload/drive/v3/files con uploadType=media
// (cuerpo crudo) o multipart (metadatos JSON + contenido, multipart/related).
//
// uploadType=media no lleva metadatos en el protocolo de Google, así que el
// nombre se toma de ?name= o de la cabecera X-File-Name y la carpeta de ?parents=
// (extensión documentada; sin nombre es 400 explícito). resumable no está
// soportado: 501 claro.
func (h *compatHandlers) googleUpload(w http.ResponseWriter, r *http.Request) {
	switch strings.ToLower(strings.TrimSpace(r.URL.Query().Get("uploadType"))) {
	case "", "media":
		name := fileNameFromRequest(r)
		if name == "" {
			failGoogle(w, r, httpx.ErrBadRequest.WithMessage("The file name is missing (?name= or X-File-Name header) for uploadType=media."))
			return
		}
		body := http.MaxBytesReader(w, r.Body, h.cfg.MaxUploadBytes+1)
		defer body.Close()
		h.googleStore(w, r, r.URL.Query().Get("parents"), name, r.Header.Get("Content-Type"), body)
	case "multipart":
		h.googleMultipartUpload(w, r)
	case "resumable":
		failGoogle(w, r, httpx.NewError(http.StatusNotImplemented, "not_implemented",
			"La subida resumable no está implementada; usa uploadType=media o multipart."))
	default:
		failGoogle(w, r, httpx.ErrBadRequest.WithMessage("Unsupported uploadType (use media or multipart)."))
	}
}

// googleMultipartUpload interpreta el cuerpo multipart/related de Google: primera
// parte = metadatos JSON (name, parents, mimeType), segunda = contenido.
func (h *compatHandlers) googleMultipartUpload(w http.ResponseWriter, r *http.Request) {
	mt, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || !strings.HasPrefix(mt, "multipart/") || params["boundary"] == "" {
		failGoogle(w, r, httpx.ErrBadRequest.WithMessage("uploadType=multipart requires Content-Type multipart/related with a boundary."))
		return
	}
	body := http.MaxBytesReader(w, r.Body, h.cfg.MaxUploadBytes+1)
	defer body.Close()
	mr := multipart.NewReader(body, params["boundary"])

	metaPart, err := mr.NextPart()
	if err != nil {
		failGoogle(w, r, httpx.ErrBadRequest.WithMessage("The metadata part of the multipart body is missing."))
		return
	}
	var meta struct {
		Name     string   `json:"name"`
		MimeType string   `json:"mimeType"`
		Parents  []string `json:"parents"`
	}
	if err := json.NewDecoder(io.LimitReader(metaPart, 1<<20)).Decode(&meta); err != nil {
		failGoogle(w, r, httpx.ErrBadRequest.WithMessage("The metadata in the multipart body is not valid JSON."))
		return
	}
	if strings.TrimSpace(meta.Name) == "" {
		failGoogle(w, r, httpx.ErrBadRequest.WithMessage("The metadata must include `name`."))
		return
	}
	contentPart, err := mr.NextPart()
	if err != nil {
		failGoogle(w, r, httpx.ErrBadRequest.WithMessage("The content part of the multipart body is missing."))
		return
	}
	contentType := meta.MimeType
	if ct := contentPart.Header.Get("Content-Type"); ct != "" {
		contentType = ct
	}
	var parent string
	if len(meta.Parents) > 0 {
		parent = meta.Parents[0]
	}
	h.googleStore(w, r, parent, meta.Name, contentType, contentPart)
}

// googleStore resuelve la carpeta destino y delega la subida en el servicio
// (misma cuota, mismo versionado por nombre y mismo límite de tamaño que la API
// nativa), respondiendo el recurso creado en el dialecto de Google (200, como
// files.create).
func (h *compatHandlers) googleStore(w http.ResponseWriter, r *http.Request, parentID, name, contentType string, body io.Reader) {
	// `parents` puede venir como lista separada por comas: se usa la primera.
	if i := strings.IndexByte(parentID, ','); i >= 0 {
		parentID = parentID[:i]
	}
	folderID, err := h.resolveFolderID(r.Context(), strings.TrimSpace(parentID))
	if err != nil {
		failGoogle(w, r, err)
		return
	}
	f, err := h.svc.UploadFile(r.Context(), folderID, name, contentType, subFromCtx(r), body)
	if err != nil {
		failGoogle(w, r, uploadError(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, compat.GoogleFromFile(f))
}

// uploadError traduce el corte por tamaño máximo a 413 (el resto lo mapea
// toAPIError, igual que en la subida nativa).
func uploadError(err error) error {
	var maxErr *http.MaxBytesError
	if errors.As(err, &maxErr) {
		return httpx.NewError(http.StatusRequestEntityTooLarge, "file_too_large", "The file is larger than the maximum allowed size.")
	}
	return err
}

// serveNodeContent transmite el contenido de un fichero (descarga en streaming,
// sin exponer el blob de S3). Una carpeta no tiene contenido: 400 claro.
func (h *compatHandlers) serveNodeContent(w http.ResponseWriter, r *http.Request, id string,
	fail func(http.ResponseWriter, *http.Request, error),
) {
	node, err := h.resolveNode(r.Context(), id)
	if err != nil {
		fail(w, r, err)
		return
	}
	if node.isDir {
		fail(w, r, httpx.ErrBadRequest.WithMessage("A folder has no downloadable content."))
		return
	}
	f, rc, err := h.svc.OpenFileContent(r.Context(), node.file.ID)
	if err != nil {
		fail(w, r, err)
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

// ---------------------------------------------------------------------------
// Microsoft Graph (files) — /compat/graph
// ---------------------------------------------------------------------------

// graphDrive responde GET /v1.0/me/drive: el drive del usuario con su raíz y su
// cuota real (omitida si el servicio no la conoce).
func (h *compatHandlers) graphDrive(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	root, err := h.svc.Root(r.Context())
	if err != nil {
		failGraph(w, r, err)
		return
	}
	c, _ := authn.FromContext(r.Context())
	var quota *domain.Quota
	if q, err := h.svc.Quota(r.Context()); err == nil {
		quota = &q
	}
	httpx.JSON(w, r, http.StatusOK, compat.NewGraphDrive(root, c.Subject, c.Name, quota))
}

// graphItem responde GET /v1.0/me/drive/items/{id} y /v1.0/me/drive/root.
func (h *compatHandlers) graphItem(w http.ResponseWriter, r *http.Request) {
	node, err := h.resolveNode(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		failGraph(w, r, err)
		return
	}
	if node.isDir {
		httpx.JSON(w, r, http.StatusOK, compat.GraphFromFolder(node.folder, nil))
		return
	}
	httpx.JSON(w, r, http.StatusOK, compat.GraphFromFile(node.file))
}

// graphChildren responde los hijos de una carpeta (/root/children o
// /items/{id}/children) con paginación OData: $top y @odata.nextLink ($skiptoken).
func (h *compatHandlers) graphChildren(w http.ResponseWriter, r *http.Request) {
	folderID, err := h.resolveFolderID(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		failGraph(w, r, err)
		return
	}
	size := compatPageSize(r.URL.Query().Get("$top"))
	token := strings.TrimSpace(r.URL.Query().Get("$skiptoken"))
	folders, files, next, err := h.listFolder(r.Context(), folderID, token, size, true, true)
	if err != nil {
		failGraph(w, r, err)
		return
	}
	items := make([]compat.DriveItem, 0, len(folders)+len(files))
	for _, d := range folders {
		items = append(items, compat.GraphFromFolder(d, nil))
	}
	for _, f := range files {
		items = append(items, compat.GraphFromFile(f))
	}
	link := compat.NextLink(h.cfg.PublicURL, r.URL.Path, size, next)
	httpx.JSON(w, r, http.StatusOK, compat.NewDriveItemCollection(items, link))
}

// graphContent responde GET /v1.0/me/drive/items/{id}/content en streaming.
func (h *compatHandlers) graphContent(w http.ResponseWriter, r *http.Request) {
	h.serveNodeContent(w, r, chi.URLParam(r, "id"), failGraph)
}

// graphPutContent responde PUT /v1.0/me/drive/items/{id}/content: sustituye el
// contenido del fichero (el servicio archiva la versión anterior, como la subida
// nativa con el mismo nombre). Crear ficheros nuevos con la sintaxis
// /root:/ruta:/content no está implementado (501 por la ruta no registrada).
func (h *compatHandlers) graphPutContent(w http.ResponseWriter, r *http.Request) {
	node, err := h.resolveNode(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		failGraph(w, r, err)
		return
	}
	if node.isDir {
		failGraph(w, r, httpx.ErrBadRequest.WithMessage("Content cannot be written over a folder."))
		return
	}
	contentType := r.Header.Get("Content-Type")
	if contentType == "" {
		contentType = node.file.ContentType
	}
	body := http.MaxBytesReader(w, r.Body, h.cfg.MaxUploadBytes+1)
	defer body.Close()

	f, err := h.svc.UploadFile(r.Context(), node.file.FolderID, node.file.Name, contentType, subFromCtx(r), body)
	if err != nil {
		failGraph(w, r, uploadError(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, compat.GraphFromFile(f))
}
