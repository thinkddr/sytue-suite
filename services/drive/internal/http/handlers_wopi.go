// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drivehttp

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/keycrypto"
	"git.sytue.eu/sytue/one/platform/pkg/log"
	"git.sytue.eu/sytue/one/platform/pkg/logsafe"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/drive/internal/config"
	"git.sytue.eu/sytue/one/services/drive/internal/domain"
	"git.sytue.eu/sytue/one/services/drive/internal/store"
	"git.sytue.eu/sytue/one/services/drive/internal/wopi"
)

// Host WOPI: Sytue Drive expuesto a un editor externo (Office Online, Collabora Online,
// OnlyOffice) para que abra y guarde ficheros que viven aquí. Mismo papel que hacen
// Nextcloud, ownCloud o Seafile.
//
// # ESTE ES EL ÚNICO SITIO DEL SERVICIO QUE NO USA EL BEARER DE SYTUE ID
//
// WOPI es a tres bandas: el navegador abre el editor y es el EDITOR, desde su servidor,
// quien llama de vuelta con un `access_token` en la QUERY STRING. Ese token acaba en los
// logs del editor y lo maneja software de un tercero, así que no puede ser el de la
// sesión. Se acuña uno propio, acotado a un fichero, un usuario, un permiso y unas horas
// (ver internal/wopi). El tenant viaja DENTRO del token y se fija aquí: tomarlo de la
// petición dejaría que el editor pidiera el mismo id de fichero en otra organización.
//
// # LO QUE NO IMPLEMENTA
//
// PutRelativeFile («guardar como»), RenameFile, ExecuteCellStorageRequest y la vista de
// versiones del editor responden 501. Un 200 vacío haría que el editor creyera que guardó.

// wopiCipher construye el sellador de tokens WOPI desde la configuración.
//
// Devuelve nil si no hay clave o si no mide 32 bytes, y entonces el host WOPI NO se monta.
// Fallar hacia «apagado» y no hacia «sin cifrar» es deliberado: un token WOPI viaja en la
// query string por software de terceros, así que o va sellado o no se emite.
func wopiCipher(cfg config.Config) *keycrypto.Cipher {
	raw := strings.TrimSpace(cfg.WopiKey)
	if raw == "" {
		return nil
	}
	key, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return nil
	}
	c, err := keycrypto.New(key)
	if err != nil {
		return nil
	}
	return c
}

// wopiLockTTL es cuánto dura un bloqueo sin refrescar. WOPI recomienda 30 min: los
// editores refrescan cada pocos minutos, y si uno muere, el fichero se libera solo.
const wopiLockTTL = 30 * time.Minute

// wopiHandlers sirve el subconjunto de WOPI que hace falta para ver y editar.
type wopiHandlers struct {
	svc    driveService
	locks  wopiLockStore
	minter *wopi.Minter
	// maxUpload acota PutFile igual que la subida nativa.
	maxUpload int64
}

// wopiLockStore es la superficie de bloqueo que consume el host. La satisface *store.Store.
type wopiLockStore interface {
	AcquireWopiLock(ctx context.Context, fileID uuid.UUID, lockID, holder string, ttl time.Duration) (store.WopiLock, error)
	GetWopiLock(ctx context.Context, fileID uuid.UUID) (store.WopiLock, error)
	ReleaseWopiLock(ctx context.Context, fileID uuid.UUID, lockID string) error
}

// mountWopi registra las rutas. NO lleva el middleware de auth del resto del servicio:
// aquí autoriza el token WOPI, no el Bearer.
func mountWopi(r chi.Router, h *wopiHandlers) {
	r.Route("/wopi/files/{id}", func(g chi.Router) {
		g.Get("/", h.checkFileInfo)
		g.Get("/contents", h.getFile)
		g.Post("/contents", h.putFile)
		// Las operaciones de bloqueo comparten verbo y ruta; las distingue la cabecera
		// X-WOPI-Override, que es como la especificación las multiplexa.
		g.Post("/", h.override)
	})
}

// wopiFail responde un error del protocolo. WOPI NO lleva cuerpo de error: el editor solo
// mira el código y las cabeceras, así que enviar JSON aquí solo serviría para filtrar
// detalles internos a un tercero.
func wopiFail(w http.ResponseWriter, r *http.Request, status int, motivo string) {
	if motivo != "" {
		log.From(r.Context()).Debug("wopi rechazado", "status", status, "motivo", logsafe.Text(motivo))
	}
	w.WriteHeader(status)
}

// authorize valida el token de la query y prepara el contexto con SU tenant.
//
// Devuelve el fichero ya resuelto porque todas las operaciones lo necesitan y porque
// resolverlo aquí garantiza que se hace DESPUÉS de validar el token, nunca antes.
func (h *wopiHandlers) authorize(w http.ResponseWriter, r *http.Request) (*http.Request, wopi.Claims, domain.File, bool) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		wopiFail(w, r, http.StatusNotFound, "id de fichero inválido")
		return nil, wopi.Claims{}, domain.File{}, false
	}
	// La especificación lo pasa en `access_token`; algunos editores lo mandan además
	// como Bearer. Se acepta el de la query, que es el que exige el estándar.
	claims, err := h.minter.Verify(r.URL.Query().Get("access_token"), id)
	if err != nil {
		wopiFail(w, r, http.StatusUnauthorized, "token")
		return nil, wopi.Claims{}, domain.File{}, false
	}
	// El tenant y el sujeto salen del TOKEN: impiden pedir el id de otra organización
	// o de otra persona dentro de la misma organización.
	ctx := store.WithOwner(tenancy.Into(r.Context(), tenancy.Tenant{Slug: claims.Tenant}), claims.Subject)
	r = r.WithContext(ctx)

	f, err := h.svc.GetFile(ctx, id)
	if err != nil {
		wopiFail(w, r, http.StatusNotFound, "fichero")
		return nil, wopi.Claims{}, domain.File{}, false
	}
	return r, claims, f, true
}

// checkFileInfo responde GET /wopi/files/{id}: los metadatos con los que el editor decide
// qué puede hacer. Es la primera llamada de todo el flujo.
func (h *wopiHandlers) checkFileInfo(w http.ResponseWriter, r *http.Request) {
	r, claims, f, ok := h.authorize(w, r)
	if !ok {
		return
	}
	// `Version` DEBE cambiar cuando cambia el contenido: el editor lo usa para detectar
	// que alguien más guardó. La versión del fichero sirve exactamente para eso.
	httpx.JSON(w, r, http.StatusOK, map[string]any{
		"BaseFileName": f.Name,
		"Size":         f.SizeBytes,
		"Version":      strconv.Itoa(f.Version),
		"OwnerId":      f.OwnerSub,
		"UserId":       claims.Subject,
		"UserFriendlyName": func() string {
			if claims.Name != "" {
				return claims.Name
			}
			return claims.Subject
		}(),
		// Capacidades. Se declaran a partir del permiso REAL que llevaba el token: si
		// aquí se dijera que sí y luego PutFile lo negara, el editor dejaría escribir al
		// usuario y perdería su trabajo al guardar.
		"UserCanWrite":               claims.Write,
		"SupportsUpdate":             claims.Write,
		"SupportsLocks":              true,
		"SupportsGetLock":            true,
		"SupportsExtendedLockLength": true,
		"ReadOnly":                   !claims.Write,
		// No implementadas: se declaran en falso para que el editor NO ofrezca el botón.
		"SupportsRename":  false,
		"UserCanRename":   false,
		"SupportsFolders": false,
		"DisablePrint":    false,
	})
}

// getFile responde GET /wopi/files/{id}/contents: los bytes.
// El contexto SÍ se hereda: `authorize` devuelve la petición con
// `tenancy.Into(r.Context(), …)` dentro. contextcheck no sigue esa
// reasignación a través del valor de retorno de un ayudante y lo lee como
// un contexto nuevo. Silenciarlo aquí es describir el falso positivo, no
// tapar un contexto huérfano.
//
//nolint:contextcheck // el contexto se hereda vía authorize; ver arriba
func (h *wopiHandlers) getFile(w http.ResponseWriter, r *http.Request) {
	r, _, f, ok := h.authorize(w, r)
	if !ok {
		return
	}
	_, rc, err := h.svc.OpenFileContent(r.Context(), f.ID)
	if err != nil {
		wopiFail(w, r, http.StatusInternalServerError, "abrir contenido")
		return
	}
	defer func() { _ = rc.Close() }()

	w.Header().Set("X-WOPI-ItemVersion", strconv.Itoa(f.Version))
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", strconv.FormatInt(f.SizeBytes, 10))
	w.WriteHeader(http.StatusOK)
	if _, err := io.Copy(w, rc); err != nil {
		log.From(r.Context()).Debug("wopi: contenido truncado", "error", err)
	}
}

// putFile responde POST /wopi/files/{id}/contents: guarda.
//
// Comprueba el bloqueo ANTES de escribir. WOPI no fusiona: un PutFile reemplaza el
// contenido entero, así que guardar sin respetar el lock borraría el trabajo de quien lo
// tiene abierto sin que nadie se entere.
// El contexto SÍ se hereda: `authorize` devuelve la petición con
// `tenancy.Into(r.Context(), …)` dentro. contextcheck no sigue esa
// reasignación a través del valor de retorno de un ayudante y lo lee como
// un contexto nuevo. Silenciarlo aquí es describir el falso positivo, no
// tapar un contexto huérfano.
//
//nolint:contextcheck // el contexto se hereda vía authorize; ver arriba
func (h *wopiHandlers) putFile(w http.ResponseWriter, r *http.Request) {
	r, claims, f, ok := h.authorize(w, r)
	if !ok {
		return
	}
	if !claims.Write {
		wopiFail(w, r, http.StatusNotFound, "sin permiso de escritura")
		return
	}

	enviado := r.Header.Get("X-WOPI-Lock")
	actual, err := h.locks.GetWopiLock(r.Context(), f.ID)
	switch {
	case err == nil && actual.LockID != enviado:
		// Bloqueado por otro. El editor necesita el lock actual para explicarlo.
		w.Header().Set("X-WOPI-Lock", actual.LockID)
		wopiFail(w, r, http.StatusConflict, "lock ajeno")
		return
	case err != nil && !errors.Is(err, domain.ErrNotFound):
		wopiFail(w, r, http.StatusInternalServerError, "leer lock")
		return
	case err != nil && enviado != "" && f.SizeBytes > 0:
		// Manda un lock que ya no existe sobre un fichero con contenido: el suyo
		// caducó y otro pudo escribir mientras. Se rechaza con lock vacío, que es lo
		// que la especificación pide para «no está bloqueado».
		w.Header().Set("X-WOPI-Lock", "")
		wopiFail(w, r, http.StatusConflict, "lock caducado")
		return
	}

	contentType := f.ContentType
	if ct := r.Header.Get("Content-Type"); ct != "" && ct != "application/octet-stream" {
		contentType = ct
	}
	body := http.MaxBytesReader(w, r.Body, h.maxUpload+1)
	defer func() { _ = body.Close() }()

	// El servicio archiva la versión anterior, igual que una subida nativa con el mismo
	// nombre: el historial de versiones de Drive sigue funcionando con lo que guarda el
	// editor externo.
	nueva, err := h.svc.UploadFile(r.Context(), f.FolderID, f.Name, contentType, claims.Subject, body)
	if err != nil {
		wopiFail(w, r, http.StatusInternalServerError, fmt.Sprintf("guardar: %v", err))
		return
	}
	w.Header().Set("X-WOPI-ItemVersion", strconv.Itoa(nueva.Version))
	w.WriteHeader(http.StatusOK)
}

// override despacha las operaciones que comparten POST /wopi/files/{id} y se distinguen
// por la cabecera X-WOPI-Override, que es como la especificación las multiplexa.
// El contexto SÍ se hereda: `authorize` devuelve la petición con
// `tenancy.Into(r.Context(), …)` dentro. contextcheck no sigue esa
// reasignación a través del valor de retorno de un ayudante y lo lee como
// un contexto nuevo. Silenciarlo aquí es describir el falso positivo, no
// tapar un contexto huérfano.
//
//nolint:contextcheck // el contexto se hereda vía authorize; ver arriba
func (h *wopiHandlers) override(w http.ResponseWriter, r *http.Request) {
	r, claims, f, ok := h.authorize(w, r)
	if !ok {
		return
	}
	op := r.Header.Get("X-WOPI-Override")

	// Todas las de bloqueo modifican estado: sin permiso de escritura no se conceden.
	if !claims.Write && op != "GET_LOCK" {
		wopiFail(w, r, http.StatusNotFound, "sin permiso de escritura")
		return
	}

	switch op {
	case "GET_LOCK":
		l, err := h.locks.GetWopiLock(r.Context(), f.ID)
		if err != nil && !errors.Is(err, domain.ErrNotFound) {
			wopiFail(w, r, http.StatusInternalServerError, "leer lock")
			return
		}
		// Sin bloqueo se devuelve cadena VACÍA con 200, no un 404: así lo pide WOPI.
		w.Header().Set("X-WOPI-Lock", l.LockID)
		w.WriteHeader(http.StatusOK)

	case "LOCK", "REFRESH_LOCK":
		h.lock(w, r, f, claims, r.Header.Get("X-WOPI-Lock"))

	case "UNLOCK_AND_RELOCK":
		// Suelta el viejo (X-WOPI-OldLock) y toma el nuevo en una sola operación.
		if err := h.locks.ReleaseWopiLock(r.Context(), f.ID, r.Header.Get("X-WOPI-OldLock")); err != nil {
			h.lockConflict(w, r, f, err)
			return
		}
		h.lock(w, r, f, claims, r.Header.Get("X-WOPI-Lock"))

	case "UNLOCK":
		if err := h.locks.ReleaseWopiLock(r.Context(), f.ID, r.Header.Get("X-WOPI-Lock")); err != nil {
			h.lockConflict(w, r, f, err)
			return
		}
		w.Header().Set("X-WOPI-ItemVersion", strconv.Itoa(f.Version))
		w.WriteHeader(http.StatusOK)

	default:
		// PUT_RELATIVE, RENAME_FILE, DELETE… No implementadas. 501 y no 200: un 200
		// vacío haría creer al editor que la operación surtió efecto.
		wopiFail(w, r, http.StatusNotImplemented, "override no soportado: "+op)
	}
}

// lock concede o refresca, y traduce el conflicto al 409 con cabecera que espera el editor.
func (h *wopiHandlers) lock(w http.ResponseWriter, r *http.Request, f domain.File, claims wopi.Claims, lockID string) {
	if lockID == "" {
		wopiFail(w, r, http.StatusBadRequest, "X-WOPI-Lock vacío")
		return
	}
	if _, err := h.locks.AcquireWopiLock(r.Context(), f.ID, lockID, claims.Subject, wopiLockTTL); err != nil {
		h.lockConflict(w, r, f, err)
		return
	}
	w.Header().Set("X-WOPI-ItemVersion", strconv.Itoa(f.Version))
	w.WriteHeader(http.StatusOK)
}

// lockConflict responde 409 con el bloqueo ACTUAL, que es lo que el editor necesita para
// decidir si reintentar o avisar a la persona.
func (h *wopiHandlers) lockConflict(w http.ResponseWriter, r *http.Request, f domain.File, cause error) {
	if !errors.Is(cause, store.ErrLockConflict) {
		wopiFail(w, r, http.StatusInternalServerError, "bloqueo")
		return
	}
	actual, err := h.locks.GetWopiLock(r.Context(), f.ID)
	if err == nil {
		w.Header().Set("X-WOPI-Lock", actual.LockID)
	}
	wopiFail(w, r, http.StatusConflict, "lock ajeno")
}

// ---------------------------------------------------------------------------
// Puerta NATIVA: acuñar el token para abrir un fichero en un editor externo
// ---------------------------------------------------------------------------

// MintWopiToken responde POST /api/v1/files/{id}/wopi-token (API nativa, Bearer normal):
// devuelve el `access_token` y el `WOPISrc` que necesita un editor WOPI para abrir ESTE
// fichero.
//
// `editor_url` sale del catálogo del editor configurado (su `discovery.xml`), no de una
// plantilla escrita a mano: Collabora, Office for the web y OnlyOffice difieren en la
// acción por tipo de fichero, y adivinarla daría una URL que carga en blanco. Sin editor
// configurado se omite ese campo y se devuelven igual el token y el `wopi_src`, que es lo
// único que solo Sytue puede dar.
//
// El permiso de escritura se declara según lo que pida el llamante, igual que hace el
// resto de esta API: la verja REAL la aplica el servicio al guardar (RLS + permisos de la
// carpeta), no esta capa.
func (h *Handlers) MintWopiToken(w http.ResponseWriter, r *http.Request) {
	if h.wopi == nil || !h.wopi.Enabled() {
		httpx.Fail(w, r, httpx.ErrNotFound.WithMessage("The WOPI host is not configured."))
		return
	}
	id, err := parseID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	// Comprueba que existe Y que este usuario lo ve: acuñar un token para un fichero
	// que no puede abrir daría al editor una llave que Sytue rechazaría después.
	f, err := h.svc.GetFile(r.Context(), id)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	t, terr := tenancy.From(r.Context())
	if terr != nil {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	// `mode=view` acuña un token de solo lectura. Es lo que permite compartir la vista
	// del documento sin dar permiso de escritura al editor.
	write := r.URL.Query().Get("mode") != "view"

	token, err := h.wopi.Mint(wopi.Claims{
		Tenant:  t.Slug,
		Subject: subFromCtx(r),
		FileID:  f.ID,
		Write:   write,
	})
	if err != nil {
		httpx.Fail(w, r, httpx.ErrInternal)
		return
	}
	src := h.cfg.PublicURL + "/wopi/files/" + f.ID.String()
	out := map[string]any{
		"access_token":     token,
		"access_token_ttl": time.Now().Add(wopi.DefaultTTL).UnixMilli(),
		"wopi_src":         src,
		"file_name":        f.Name,
		"can_write":        write,
	}
	// Solo si el editor declara saber abrir ESTA extensión. Devolver una URL para un
	// tipo que no maneja llevaría a una pantalla en blanco en vez de a un error claro.
	if u, ok := h.editorReg.For(r.Context(), t.Slug).URLFor(r.Context(), f.Name, src, write); ok {
		out["editor_url"] = u
	}
	httpx.JSON(w, r, http.StatusOK, out)
}

// WopiCapabilities responde GET /api/v1/wopi/capabilities: si hay editor externo y qué
// extensiones sabe abrir.
//
// Existe para que la UI no tenga que acuñar un token por fichero solo para averiguar si
// enseña el botón: un token WOPI es una llave con permiso de escritura, y emitir una por
// cada icono de una carpeta sería regalarlas.
func (h *Handlers) WopiCapabilities(w http.ResponseWriter, r *http.Request) {
	exts := []string{}
	if h.wopi != nil && h.wopi.Enabled() {
		if t, err := tenancy.From(r.Context()); err == nil {
			exts = h.editorReg.For(r.Context(), t.Slug).Extensions(r.Context())
		}
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{
		"enabled":    len(exts) > 0,
		"extensions": exts,
	})
}
