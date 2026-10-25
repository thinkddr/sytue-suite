// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package hubhttp

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/log"
	"git.sytue.eu/sytue/one/platform/pkg/logsafe"
	"git.sytue.eu/sytue/one/services/hub/internal/instaladores"
)

// errSinInstaladores es el 503 de un despliegue sin bucket, con su código propio: la
// página de descargas de la app nativa dice «sin configurar» solo con este. Un bucket que
// no contesta es el `unavailable` de siempre, que allí es «inténtalo más tarde».
var errSinInstaladores = httpx.ErrUnavailable.WithCode("installers_unconfigured").
	WithMessage("Installers are not configured here.")

// catalogoInstaladores es lo que los manejadores usan de instaladores.Catalogo.
type catalogoInstaladores interface {
	Listar(ctx context.Context, plataforma string) ([]instaladores.Instalador, error)
	Abrir(ctx context.Context, nombre string) (io.ReadCloser, instaladores.Instalador, error)
	Sumas(ctx context.Context) ([]byte, string, error)
}

// ListarInstaladores responde qué instaladores hay publicados para una plataforma, con
// su versión y su suma: es lo que pregunta el actualizador de las apps de escritorio.
// Sin plataforma, los de todas: es la página de descargas de la app nativa del Hub.
//
//	GET /api/v1/instaladores?plataforma=macos  →  {"instaladores":[{app, version, sha256, url…}]}
//
// Sin bucket configurado, 503: las apps lo toman por «no se sabe» y siguen como están.
func (h *Handlers) ListarInstaladores(w http.ResponseWriter, r *http.Request) {
	if h.instaladores == nil {
		httpx.Fail(w, r, errSinInstaladores)
		return
	}
	p := r.URL.Query().Get("plataforma")
	if p != "" && !instaladores.EsPlataforma(p) {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("The platform must be macos, windows or linux."))
		return
	}
	lista, err := h.instaladores.Listar(r.Context(), p)
	if err != nil {
		httpx.Fail(w, r, httpx.ErrUnavailable.Wrap(err))
		return
	}
	if lista == nil {
		lista = []instaladores.Instalador{}
	}
	w.Header().Set("Cache-Control", "private, no-store") // lleva URL firmadas
	httpx.JSON(w, r, http.StatusOK, map[string]any{"instaladores": lista})
}

// BajarInstalador sirve UN instalador por el propio Hub, para cuando el almacén solo se
// alcanza desde dentro (el SeaweedFS del piloto) y la URL firmada no la abriría nadie.
// Va en flujo y sin plazo de escritura, como la exportación: son cientos de megas.
//
//	GET /api/v1/instaladores/{fichero}
func (h *Handlers) BajarInstalador(w http.ResponseWriter, r *http.Request) {
	if h.instaladores == nil {
		httpx.Fail(w, r, errSinInstaladores)
		return
	}
	cuerpo, inst, err := h.instaladores.Abrir(r.Context(), chi.URLParam(r, "fichero"))
	if errors.Is(err, instaladores.ErrNoEsta) {
		httpx.Fail(w, r, httpx.ErrNotFound)
		return
	}
	if err != nil {
		httpx.Fail(w, r, httpx.ErrUnavailable.Wrap(err))
		return
	}
	defer func() { _ = cuerpo.Close() }()
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", strconv.FormatInt(inst.Bytes, 10))
	w.Header().Set("Content-Disposition", `attachment; filename="`+inst.Fichero+`"`)
	w.Header().Set("Cache-Control", "private, no-store")
	_ = http.NewResponseController(w).SetWriteDeadline(time.Time{})
	if _, err := io.Copy(w, cuerpo); err != nil {
		// Las cabeceras ya salieron: la app lo nota al comprobar la suma.
		log.From(r.Context()).Info("instaladores: la descarga no terminó", "fichero", logsafe.Text(inst.Fichero), "error", err)
	}
}

// SumasInstaladores da el SHA256SUMS de los instaladores publicados y su firma, para
// que el actualizador de las apps compruebe con su clave embebida lo que va a instalar
// (auditoría 2026-10-06, CI-05). Sin SHA256SUMS, 404: la app sigue con la suma del
// catálogo mientras no traiga clave, y con clave no instala.
//
//	GET /api/v1/instaladores/sumas  →  {"sumas": "<texto>", "firma": "<base64 o vacío>"}
func (h *Handlers) SumasInstaladores(w http.ResponseWriter, r *http.Request) {
	if h.instaladores == nil {
		httpx.Fail(w, r, errSinInstaladores)
		return
	}
	sumas, firma, err := h.instaladores.Sumas(r.Context())
	if errors.Is(err, instaladores.ErrNoEsta) {
		httpx.Fail(w, r, httpx.ErrNotFound)
		return
	}
	if err != nil {
		httpx.Fail(w, r, httpx.ErrUnavailable.Wrap(err))
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	httpx.JSON(w, r, http.StatusOK, map[string]string{"sumas": string(sumas), "firma": firma})
}
