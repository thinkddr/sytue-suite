// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"net/http"
	"regexp"
	"strings"

	"github.com/go-chi/chi/v5"
)

// VALIDACIÓN DE DOMINIO PARA S/MIME: el fichero que la CA viene a leer.
//
// Para que un correo firmado se valide SOLO en Gmail, Outlook y Apple Mail hace
// falta un certificado de una CA pública, y las reglas del CA/Browser Forum
// obligan a validar TODOS los dominios de las direcciones. Cada organización
// tiene el suyo (`<slug>.sytue.eu`), así que hay una validación por organización
// y, si son cientos, a mano no se hace.
//
// El método que toda CA ofrece es dejar un fichero con un contenido exacto en
// `/.well-known/pki-validation/<nombre>` del dominio, y venir a leerlo. Este
// handler lo sirve desde la tabla donde la tubería de emisión lo dejó.
//
// POR HTTP Y NO POR DNS aunque la zona sea nuestra: el comodín `*.sytue.eu` ya
// enruta cada organización, así que responder es inmediato. Por DNS habría que
// escribir un registro y esperar propagación, que en una emisión automática es
// la diferencia entre acabar en segundos o en horas —y el motivo por el que un
// alta se quedaría a medias sin que nadie sepa por qué—.
//
// SIN SESIÓN, porque quien viene es la CA y no trae ninguna. No publica nada:
// el contenido es un valor aleatorio que la propia CA acaba de generar y que
// solo ella sabe relacionar con una emisión.

// nombreDeReto acota lo que puede ir en la ruta. La tabla lo comprueba también
// (migración 0061): una ruta no se compone nunca con algo que no esté acotado en
// los dos sitios.
var nombreDeReto = regexp.MustCompile(`^[A-Za-z0-9._-]{1,128}$`)

// handleDCVChallenge sirve GET /.well-known/pki-validation/{nombre}.
func (s *Server) handleDCVChallenge(w http.ResponseWriter, r *http.Request) {
	nombre := chi.URLParam(r, "nombre")
	if !nombreDeReto.MatchString(nombre) || strings.Contains(nombre, "..") {
		http.NotFound(w, r)
		return
	}
	// EL DOMINIO ES EL DEL HOST PEDIDO, que es justo lo que la CA está probando:
	// viene a `<dominio>/.well-known/...` precisamente para comprobar que quien
	// contesta ahí controla ESE nombre. Tomarlo de otro sitio —un parámetro, la
	// configuración— rompería la prueba: serviríamos el reto de un dominio desde
	// otro y acreditaríamos un control que no se ha demostrado.
	dominio := hostSinPuerto(r.Host)
	if dominio == "" {
		http.NotFound(w, r)
		return
	}
	contenido, err := s.store.RetoDeDominio(r.Context(), dominio, nombre)
	if err != nil {
		s.log.Error("reto de validación de dominio", "error", err)
		http.Error(w, "", http.StatusInternalServerError)
		return
	}
	if contenido == "" {
		http.NotFound(w, r)
		return
	}
	// `text/plain` y sin nada más: varias CAs comparan el cuerpo BYTE A BYTE, así
	// que ni salto de línea de cortesía ni BOM ni espacios.
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = w.Write([]byte(contenido))
}

// hostSinPuerto normaliza el Host de la petición a un nombre comparable con lo
// guardado: minúsculas, sin puerto y sin el punto final de un FQDN absoluto.
func hostSinPuerto(host string) string {
	h := strings.ToLower(strings.TrimSpace(host))
	if i := strings.LastIndex(h, ":"); i > 0 && !strings.Contains(h[i:], "]") {
		h = h[:i]
	}
	return strings.TrimSuffix(strings.Trim(h, "[]"), ".")
}
