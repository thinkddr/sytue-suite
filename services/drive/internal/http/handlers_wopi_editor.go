// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drivehttp

import (
	"context"
	"net/http"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/drive/internal/store"
	"git.sytue.eu/sytue/one/services/drive/internal/wopi"
)

// CONECTAR UN EDITOR EXTERNO — plano de administración de la organización.
//
// Estaba en una variable de entorno del servicio: conectar el Collabora de una empresa
// obligaba a editar el despliegue y reiniciar Drive, y en una instalación con varias
// organizaciones todas compartían editor lo quisieran o no. Quién edita los documentos de
// una empresa es decisión de esa empresa.
//
// Se pide UNA cosa —la dirección del `discovery.xml`— y se comprueba en el acto: quien lo
// conecta ve si responde y qué tipos de fichero abre antes de guardar, en vez de
// descubrir el fallo cuando alguien intente abrir un documento.

// editorProbeTTL es el tiempo de caché del catálogo de una comprobación puntual. Corto a
// propósito: comprobar es justamente querer el estado de AHORA.
const editorProbeTTL = time.Minute

// wopiEditorStore es la superficie de persistencia del editor. La satisface *store.Store.
type wopiEditorStore interface {
	GetWopiEditor(ctx context.Context) (store.WopiEditor, error)
	// WopiEditorFor resuelve el de UNA organización sin sesión de usuario: el host WOPI
	// llega con el token del editor y el tenant viaja dentro de él, no en un JWT.
	WopiEditorFor(ctx context.Context, tenant string) (store.WopiEditor, error)
	SaveWopiEditor(ctx context.Context, e store.WopiEditor, actor string) (store.WopiEditor, error)
	DeleteWopiEditor(ctx context.Context) error
}

// editorPayload es la vista del editor que viaja a la consola.
func editorPayload(e store.WopiEditor) map[string]any {
	out := map[string]any{
		"discovery_url": e.DiscoveryURL,
		"label":         e.Label,
		"enabled":       e.Enabled,
		"check_ok":      e.CheckOK,
		"check_error":   e.CheckError,
		"extensions":    e.Extensions,
		"updated_at":    e.UpdatedAt,
	}
	if e.CheckedAt != nil {
		out["checked_at"] = *e.CheckedAt
	}
	if e.Extensions == nil {
		out["extensions"] = []string{}
	}
	return out
}

// GetWopiEditor — GET /api/v1/wopi/editor: qué editor tiene conectado la organización.
//
// Responde ADEMÁS `host_enabled`: si este despliegue puede acuñar los tokens WOPI con
// los que un editor abre un fichero. Sin `WOPI_KEY` el host no se monta —server.go, «if
// wopiMinter.Enabled()»—, así que se podía conectar un Collabora, ver «editor conectado»
// y que nadie de la organización viera jamás la acción en Drive, sin un solo error por
// ningún lado. Quien administra tiene que poder distinguir «no lo has configurado» de
// «esta instalación no puede».
func (h *Handlers) GetWopiEditor(w http.ResponseWriter, r *http.Request) {
	if h.editors == nil {
		httpx.Fail(w, r, httpx.ErrNotFound)
		return
	}
	hostEnabled := h.wopi != nil && h.wopi.Enabled()
	e, err := h.editors.GetWopiEditor(r.Context())
	if err != nil {
		// Sin editor conectado no es un error: es el estado inicial de toda organización
		// y la consola tiene que poder pintar su pantalla de bienvenida.
		httpx.JSON(w, r, http.StatusOK, map[string]any{"editor": nil, "host_enabled": hostEnabled})
		return
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"editor": editorPayload(e), "host_enabled": hostEnabled})
}

type saveEditorRequest struct {
	DiscoveryURL string `json:"discovery_url"`
	Label        string `json:"label,omitempty"`
	// Enabled ausente ⇒ true: quien acaba de conectar un editor quiere usarlo.
	Enabled *bool `json:"enabled,omitempty"`
}

// CheckWopiEditor — POST /api/v1/wopi/editor/check: prueba una dirección SIN guardarla.
//
// Es el botón «comprobar» de la consola. Se separa de guardar a propósito: probar una
// dirección no debe cambiar la configuración de nadie, y así se puede probar antes de
// decidir.
func (h *Handlers) CheckWopiEditor(w http.ResponseWriter, r *http.Request) {
	var req saveEditorRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	url := strings.TrimSpace(req.DiscoveryURL)
	if url == "" {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithCode("missing_discovery_url"))
		return
	}
	exts, err := wopi.Probe(r.Context(), url, editorProbeTTL)
	if err != nil {
		// 200 con el motivo, no un error HTTP: la comprobación FUNCIONÓ y su resultado es
		// que el editor no responde. Un 502 aquí haría que la consola dijera «no se pudo
		// comprobar» cuando lo que hay que decir es «ese editor no contesta».
		httpx.JSON(w, r, http.StatusOK, map[string]any{
			"ok":     false,
			"reason": err.Error(),
		})
		return
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"ok": true, "extensions": exts})
}

// SaveWopiEditor — PUT /api/v1/wopi/editor: conecta o reconfigura el editor.
//
// Comprueba antes de guardar y persiste el resultado con la configuración: así la consola
// puede decir «conectado, abre estos tipos» o «guardado, pero ahora mismo no responde»,
// que son dos situaciones distintas y ambas legítimas.
func (h *Handlers) SaveWopiEditor(w http.ResponseWriter, r *http.Request) {
	if h.editors == nil {
		httpx.Fail(w, r, httpx.ErrNotFound)
		return
	}
	var req saveEditorRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	url := strings.TrimSpace(req.DiscoveryURL)
	if url == "" {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithCode("missing_discovery_url"))
		return
	}

	ahora := time.Now().UTC()
	e := store.WopiEditor{
		DiscoveryURL: url,
		Label:        strings.TrimSpace(req.Label),
		Enabled:      req.Enabled == nil || *req.Enabled,
		CheckedAt:    &ahora,
		Extensions:   []string{},
	}
	if exts, err := wopi.Probe(r.Context(), url, editorProbeTTL); err != nil {
		// Se guarda igual: una dirección correcta cuyo editor está reiniciándose no debe
		// perderse por eso. Lo que NO se hace es mentir sobre su estado.
		e.CheckError = err.Error()
	} else {
		e.CheckOK = true
		e.Extensions = exts
	}

	saved, err := h.editors.SaveWopiEditor(r.Context(), e, subFromCtx(r))
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	// El catálogo cacheado de esa dirección se descarta: si no, cambiar la configuración
	// seguiría sirviendo el catálogo viejo hasta que caducara solo.
	h.editorReg.Forget(url)
	httpx.JSON(w, r, http.StatusOK, map[string]any{"editor": editorPayload(saved)})
}

// DeleteWopiEditor — DELETE /api/v1/wopi/editor: desconecta el editor.
func (h *Handlers) DeleteWopiEditor(w http.ResponseWriter, r *http.Request) {
	if h.editors == nil {
		httpx.Fail(w, r, httpx.ErrNotFound)
		return
	}
	// Se lee antes de borrar para poder olvidar su catálogo: después ya no se sabe cuál
	// era su dirección.
	if e, err := h.editors.GetWopiEditor(r.Context()); err == nil {
		h.editorReg.Forget(e.DiscoveryURL)
	}
	if err := h.editors.DeleteWopiEditor(r.Context()); err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.NoContent(w)
}

// editorLookup resuelve la dirección del editor de una organización.
//
// `porDefecto` es el valor de despliegue (`WOPI_DISCOVERY_URL`), que sobrevive como
// respaldo de una instalación con un solo editor para todos: quitarlo obligaría a
// reconfigurar a mano a quien ya lo tuviera puesto. Lo que decida una organización gana
// sobre él, incluido apagarlo.
func editorLookup(st wopiEditorStore, porDefecto string) wopi.EditorSource {
	porDefecto = strings.TrimSpace(porDefecto)
	return func(ctx context.Context, tenant string) (string, bool) {
		if st != nil {
			if e, err := st.WopiEditorFor(ctx, tenant); err == nil {
				return e.DiscoveryURL, e.Enabled && e.DiscoveryURL != ""
			}
		}
		return porDefecto, porDefecto != ""
	}
}
