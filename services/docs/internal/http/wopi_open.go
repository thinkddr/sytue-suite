// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/docs/internal/domain"
	"git.sytue.eu/sytue/one/services/docs/internal/plantillas"
	"git.sytue.eu/sytue/one/services/docs/internal/wopi"
)

// Apertura de un fichero por WOPI: lo que convierte «abrir un .docx que vive en
// Drive» en una pantalla del editor sin crear ningún documento.
//
// SIN sesión de Sytue ID, y es correcto: en este flujo la credencial ES el token
// de acceso WOPI, que acuñó el host tras autorizar a esa persona sobre ESE
// fichero. Exigir además una sesión rompería el protocolo —un host cualquiera no
// tiene por qué compartir sesión con el editor— y no añadiría nada: quien no
// tenga un token válido no pasa del primer CheckFileInfo.

// wopiOpenReq es lo que manda la página del editor tras recibir el formulario
// del host.
type wopiOpenReq struct {
	Src   string `json:"src"`
	Token string `json:"token"`
	// Locale es el idioma de la pantalla: el de los avisos que el importador escribe
	// dentro de lo convertido.
	Locale string `json:"locale,omitempty"`
}

// wopiOpenRes es el documento listo para pintar.
type wopiOpenRes struct {
	Kind  domain.Kind `json:"kind"`
	Title string      `json:"title"`
	// Content es el modelo convertido: HTML, o el JSON de hoja/diapositivas.
	// Vacío cuando el fichero no se convierte y se sirve tal cual (PDF).
	Content string `json:"content,omitempty"`
	// Raw indica que el contenido NO se convierte y hay que pedirlo por
	// `/api/v1/wopi/content` (hoy, el PDF: se pinta tal cual, no se importa).
	Raw bool `json:"raw,omitempty"`
	// CanWrite lo dice el HOST. El editor lo obedece, no lo decide.
	CanWrite bool   `json:"can_write"`
	Version  string `json:"version,omitempty"`
}

// hostPermitido comprueba que el `WOPISrc` apunta a un host que esta instalación
// acepta servir.
//
// ES LA DECISIÓN DE SEGURIDAD DE ESTE FICHERO, y no la trae el protocolo. Sin
// lista, cualquiera podría mandar a Docs un `WOPISrc` a un servidor suyo y
// usarlo de convertidor gratuito —o de proxy con la cara de Sytue— para
// cualquier URL pública que se le ocurriera. La barrera SSRF impide alcanzar lo
// interno, pero no impide eso.
//
// Por defecto (lista vacía) solo se acepta el propio Sytue Drive. Interoperar
// con un host ajeno es una decisión explícita de quien despliega, no algo que
// pase por venir en una URL.
func hostPermitido(src string, permitidos []string) bool {
	u, err := url.Parse(src)
	if err != nil || u.Host == "" {
		return false
	}
	host := strings.ToLower(u.Hostname())
	for _, p := range permitidos {
		p = strings.ToLower(strings.TrimSpace(p))
		if p == "" {
			continue
		}
		if host == p {
			return true
		}
		// Un patrón `*.dominio` cubre el multi-tenant por subdominio sin abrir la
		// mano a todo internet.
		if strings.HasPrefix(p, "*.") && strings.HasSuffix(host, p[1:]) {
			return true
		}
	}
	return false
}

// wopiOpen trae el fichero del host y devuelve el modelo listo para el lienzo.
func (h *Handler) wopiOpen(w http.ResponseWriter, r *http.Request) {
	var req wopiOpenReq
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		return
	}
	if req.Src == "" || req.Token == "" {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Missing WOPISrc or access token."))
		return
	}
	if !hostPermitido(req.Src, h.wopiHosts) {
		// Mismo mensaje para «host no permitido» que para un token malo: decir
		// cuál de las dos cosas falló ayuda a quien está probando a colarse.
		httpx.Fail(w, r, httpx.ErrForbidden.WithMessage("This file host is not allowed."))
		return
	}

	c := wopi.New(20 * time.Second)
	info, err := c.CheckFileInfo(r.Context(), req.Src, req.Token)
	if err != nil {
		httpx.Fail(w, r, httpx.ErrForbidden.WithMessage("The file could not be opened."))
		return
	}
	ver, _ := wopi.Soporta(info.Extension())
	if !ver {
		httpx.Fail(w, r, httpx.ErrUnprocessable.WithMessage("This file type cannot be opened here."))
		return
	}

	res := wopiOpenRes{Title: info.BaseFileName, CanWrite: info.UserCanWrite, Version: info.Version}
	// El PDF no se convierte: se pinta tal cual. Importarlo a un modelo de
	// documento sería reescribirlo, y un PDF que vuelve distinto de como entró no
	// es una previsualización, es una alteración.
	if info.Extension() == "pdf" {
		res.Kind = domain.Kind("pdf")
		res.Raw = true
		res.CanWrite = false
		httpx.JSON(w, r, http.StatusOK, res)
		return
	}

	data, err := c.GetFile(r.Context(), req.Src, req.Token)
	if err != nil {
		httpx.Fail(w, r, httpx.ErrUnavailable.WithMessage("The file could not be downloaded."))
		return
	}
	imp, err := domain.ImportLocalized(info.BaseFileName, data, false, plantillas.Idioma(req.Locale))
	if err != nil {
		if errors.Is(err, domain.ErrInvalidFormat) {
			httpx.Fail(w, r, httpx.ErrUnprocessable.WithMessage("The file could not be converted."))
			return
		}
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	}
	res.Kind, res.Content = imp.Kind, imp.Content
	if imp.Title != "" {
		res.Title = imp.Title
	}
	httpx.JSON(w, r, http.StatusOK, res)
}

// wopiContent devuelve el fichero TAL CUAL, para lo que no se convierte.
//
// Hoy es el PDF: lo pinta el motor del propio navegador dentro de la superficie
// de Sytue PDF. Pasa por aquí y no por una URL directa al host porque el token
// WOPI no debe salir al navegador —viaja en la query y acabaría en el historial
// y en los registros de cualquier proxy—, y porque así el contenido llega del
// mismo origen y no hay que abrirle CORS a nadie.
func (h *Handler) wopiContent(w http.ResponseWriter, r *http.Request) {
	var req wopiOpenReq
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		return
	}
	if req.Src == "" || req.Token == "" || !hostPermitido(req.Src, h.wopiHosts) {
		httpx.Fail(w, r, httpx.ErrForbidden)
		return
	}
	c := wopi.New(30 * time.Second)
	info, err := c.CheckFileInfo(r.Context(), req.Src, req.Token)
	if err != nil {
		httpx.Fail(w, r, httpx.ErrForbidden.WithMessage("The file could not be opened."))
		return
	}
	if ver, _ := wopi.Soporta(info.Extension()); !ver {
		httpx.Fail(w, r, httpx.ErrUnprocessable)
		return
	}
	data, err := c.GetFile(r.Context(), req.Src, req.Token)
	if err != nil {
		httpx.Fail(w, r, httpx.ErrUnavailable)
		return
	}
	// `inline` y no `attachment`: esto se pinta, no se descarga. Y `nosniff` con
	// un tipo declarado: sin él, un fichero con bytes de HTML dentro podría
	// interpretarse como página en NUESTRO origen.
	w.Header().Set("Content-Type", tipoDe(info.Extension()))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Disposition", "inline")
	w.Header().Set("Cache-Control", "private, no-store")
	_, _ = w.Write(data)
}

// tipoDe da el content-type de lo que se sirve tal cual.
func tipoDe(ext string) string {
	if ext == "pdf" {
		return "application/pdf"
	}
	return "application/octet-stream"
}
