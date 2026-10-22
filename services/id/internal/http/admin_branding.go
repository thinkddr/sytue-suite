// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"slices"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/logofit"
	"git.sytue.eu/sytue/one/services/id/internal/domain"
)

// El logotipo de la organización: subirlo, previsualizarlo, quitarlo y servirlo.
//
// La subida va reservada a owner/admin —el logo lo ve TODA la plantilla en la barra
// de las once superficies— y la lectura es pública a propósito, por lo mismo: es el
// primer píxel que se pinta en cada carga, así que no puede depender de una sesión
// ni de una cabecera de autorización.
//
// Lo que se guarda NUNCA es el fichero que subió el administrador: es lo que sale de
// logofit, decodificado y vuelto a codificar por nosotros. Esa regla —el byte que se
// sirve no es el byte que se subió— es la que hace que el endpoint público sea
// seguro sin tener que confiar en el fichero de nadie.

// Techo del cuerpo de la subida. Un logo de marca no llega ni de lejos, y lo que
// importa no es el megabyte sino que exista un tope: quien decodifica la imagen es
// el servicio que emite todos los tokens de la plataforma.
const maxCuerpoLogo = 1 << 20 // 1 MiB

// handleAdminPutLogo — PUT /api/v1/admin/org/logo — sube (o solo prueba) un logo.
//
// El fichero viaja como CUERPO CRUDO, no como multipart: es un único fichero sin
// más campos, y `fetch(url, {method:'PUT', body: file})` ahorra el parseo de
// multipart en el servidor y el FormData en el cliente.
//
// Con `?save=0` se analiza y se devuelve el resultado SIN escribir nada. Eso es lo
// que permite que la consola enseñe el reencuadre antes de confirmarlo: el
// administrador ve lo que va a quedar en su barra y decide, en vez de descubrir
// después que el sistema le recortó algo.
func (s *Server) handleAdminPutLogo(w http.ResponseWriter, r *http.Request) {
	org, err := s.activeOrg(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}

	crudo, err := io.ReadAll(io.LimitReader(r.Body, maxCuerpoLogo+1))
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("The file could not be read."))
		return
	}
	if len(crudo) == 0 {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("The file is empty."))
		return
	}
	if len(crudo) > maxCuerpoLogo {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("The file is larger than 1 MB."))
		return
	}

	png, rep, err := logofit.Fit(crudo)
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage(mensajeLogo(err)))
		return
	}

	suma := sha256.Sum256(png)
	hash := hex.EncodeToString(suma[:8])

	// El resultado va SIEMPRE en la respuesta, se guarde o no: quien previsualiza y
	// quien confirma quieren ver lo mismo.
	res := map[string]any{
		"png":         base64.StdEncoding.EncodeToString(png),
		"hash":        hash,
		"width":       rep.Ancho,
		"height":      rep.Alto,
		"trimmed":     rep.Recortado,
		"trim":        map[string]int{"top": rep.Arriba, "bottom": rep.Abajo, "left": rep.Izquierda, "right": rep.Derecha},
		"trim_capped": rep.TopeRecorte,
		"no_margin":   rep.BordeSinMargen,
		"opaque_box":  rep.FondoOpaco,
		"saved":       false,
	}

	if r.URL.Query().Get("save") != "1" {
		httpx.JSON(w, r, http.StatusOK, res)
		return
	}

	ctx := r.Context()
	if err := s.store.GuardarLogo(ctx, nil, org.ID, png, hash); err != nil {
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	}
	c, _ := claimsFrom(ctx)
	_ = s.store.Audit(ctx, nil, auditEntry(c.Subject, org.ID, "org.branding.updated", hash, r))
	res["saved"] = true
	httpx.JSON(w, r, http.StatusOK, res)
}

// handleAdminDeleteLogo — DELETE /api/v1/admin/org/logo — vuelve a la marca de la
// casa. Borrar lo que no hay devuelve 204 igualmente: el resultado es el que se
// pedía.
func (s *Server) handleAdminDeleteLogo(w http.ResponseWriter, r *http.Request) {
	org, err := s.activeOrg(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	ctx := r.Context()
	if err := s.store.BorrarLogo(ctx, nil, org.ID); err != nil {
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	}
	c, _ := claimsFrom(ctx)
	_ = s.store.Audit(ctx, nil, auditEntry(c.Subject, org.ID, "org.branding.deleted", "", r))
	w.WriteHeader(http.StatusNoContent)
}

// ejeDeApariencia es un valor por defecto de la organización que la web pinta con
// un atributo en la raíz: el color (ADR 0071) y la forma. Los dos se fijan, se
// validan y se auditan igual; lo único que cambia es el nombre y la lista.
type ejeDeApariencia struct {
	// nombre es la clave del JSON, la columna de `organizations` y el sufijo de la
	// entrada de auditoría.
	nombre string
	// valores es lo que el dominio acepta.
	valores []string
}

var (
	ejeColor = ejeDeApariencia{nombre: "color", valores: domain.OrgColors}
	ejeForma = ejeDeApariencia{nombre: "forma", valores: domain.OrgFormas}
)

// handleAdminPutApariencia — PUT /api/v1/admin/org/{color,forma} — lo que ve quien
// no ha elegido lo suyo.
//
// No cambia la elección de nadie: la de cada persona vive en su navegador y manda
// sobre esta. Llega a la plantilla con el siguiente token de cada uno —los claims
// `org_color` y `org_forma`—, no al instante: la apariencia no justifica invalidar
// sesiones.
func (s *Server) handleAdminPutApariencia(eje ejeDeApariencia) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		org, err := s.activeOrg(r)
		if err != nil {
			httpx.Fail(w, r, err)
			return
		}
		var req map[string]string
		if err := httpx.DecodeJSON(w, r, &req); err != nil {
			httpx.Fail(w, r, err)
			return
		}
		valor := req[eje.nombre]
		if !slices.Contains(eje.valores, valor) {
			httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Unknown "+eje.nombre+"."))
			return
		}
		ctx := r.Context()
		if err := s.store.GuardarApariencia(ctx, nil, org.ID, eje.nombre, valor); err != nil {
			httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
			return
		}
		c, _ := claimsFrom(ctx)
		_ = s.store.Audit(ctx, nil, auditEntry(c.Subject, org.ID, "org.branding."+eje.nombre, valor, r))
		httpx.JSON(w, r, http.StatusOK, map[string]string{eje.nombre: valor})
	}
}

// handleLogoPublico — GET /branding/{slug}/logo.png — el logo, sin sesión.
//
// Se sirve como INMUTABLE durante un año, y aun así cambia en cuanto el
// administrador sube otro: la URL que compone el cliente lleva `?v=<hash>` y el hash
// viaja en el token, así que al cambiar el logo cambia la URL y el navegador pide
// una nueva. El parámetro se IGNORA aquí a propósito —siempre se sirve el logo
// vigente—: es un cache-buster para el navegador, no un selector de versión.
func (s *Server) handleLogoPublico(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	png, hash, err := s.store.LogoPorSlug(r.Context(), nil, slug)
	if err != nil {
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	}

	h := w.Header()
	// Que el navegador no adivine el tipo, que no lo trate como documento y que no
	// ejecute nada aunque algún día se cuele algo que no sea un PNG.
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Content-Security-Policy", "sandbox; default-src 'none'")
	h.Set("X-Robots-Tag", "noindex")

	if png == nil {
		// Sin logo no es un error, es el caso normal: la app pinta la marca de la
		// casa. Se cachea cinco minutos para que estrenar logo se vea enseguida.
		h.Set("Cache-Control", "public, max-age=300")
		http.NotFound(w, r)
		return
	}

	// El logo lo pide un <img> de otro subdominio (mail., drive., …), así que la
	// política por defecto del servicio —same-origin— lo bloquearía.
	h.Set("Cross-Origin-Resource-Policy", "cross-origin")
	h.Set("Content-Type", "image/png")
	h.Set("Cache-Control", "public, max-age=31536000, immutable")
	h.Set("ETag", `"`+hash+`"`)
	if match := r.Header.Get("If-None-Match"); match == `"`+hash+`"` {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	_, _ = w.Write(png)
}

// mensajeLogo traduce el fallo del reencuadre a algo que un administrador pueda
// arreglar sin saber de imágenes. Nada de «err de decodificación»: qué pasa y qué
// hacer.
//
// EN INGLÉS, que es el idioma por defecto de la plataforma (CONVENTIONS §13.1).
// No es cosmética interna: la tarjeta de la consola enseña este texto TAL CUAL al
// administrador, así que en castellano un administrador alemán recibía castellano.
func mensajeLogo(err error) string {
	switch {
	case errors.Is(err, logofit.ErrTipo):
		return "The file must be a PNG, JPEG or GIF. If your logo is an SVG, export it to PNG with a transparent background."
	case errors.Is(err, logofit.ErrEnorme):
		return "The image has too many pixels. 500 px tall is more than enough."
	case errors.Is(err, logofit.ErrIlegible):
		return "The file cannot be read as an image. Export it again."
	case errors.Is(err, logofit.ErrVacio):
		return "The image is a single flat colour: there is no logo inside it."
	case errors.Is(err, logofit.ErrAlargado):
		return "The image is too elongated for the top bar. Crop the spare space at the sides."
	default:
		return "The image could not be processed."
	}
}
