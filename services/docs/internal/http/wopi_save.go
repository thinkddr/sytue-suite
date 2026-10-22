// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"encoding/json"
	"errors"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/docs/internal/domain"
	"git.sytue.eu/sytue/one/services/docs/internal/wopi"
)

// Guardar en el host lo que se editó aquí: la mitad que le faltaba al cliente
// WOPI. Hasta ahora el catálogo anunciaba solo `view` porque sin esto abrir un
// fichero para «editar» habría sido mandar a alguien a escribir en algo que al
// cerrar no conserva nada.
//
// EL CICLO ES LOCK → PUT → UNLOCK, y el orden importa: sin bloqueo, dos personas
// con el mismo fichero abierto se pisan y el último en escribir se lleva el
// trabajo del otro sin que ninguno se entere. El bloqueo es lo que convierte
// «guardar» en una operación segura y no en una carrera.
//
// SE GUARDA EN EL FORMATO CON EL QUE VINO. Un .docx que se abre y se guarda sigue
// siendo un .docx: cambiarle el formato al fichero de alguien porque el editor
// prefiere otro es alterar su archivo, no editarlo. De ahí que el formato se
// deduzca de la extensión del nombre que dio el host y no de una preferencia
// nuestra.

// wopiSaveReq es lo que manda la pantalla del editor al guardar.
type wopiSaveReq struct {
	Src   string `json:"src"`
	Token string `json:"token"`
	// Kind y Content son el modelo del editor, igual que los devolvió
	// `/api/v1/wopi/open`.
	Kind    string `json:"kind"`
	Content string `json:"content"`
	// Lock es el identificador del bloqueo que ya se tiene. Vacío la primera vez:
	// entonces se toma aquí y se devuelve para las siguientes.
	Lock string `json:"lock"`
}

// formatoDe deduce el formato de salida de la extensión del fichero.
//
// Devuelve cadena vacía si no se sabe escribir ese formato, y entonces NO se
// guarda: dejar caer el contenido en un formato distinto sería peor que fallar,
// porque el fichero seguiría en su sitio con el nombre de siempre y el contenido
// de otra cosa.
func formatoDe(nombre string) domain.BlobFormat {
	switch strings.ToLower(strings.TrimPrefix(filepath.Ext(nombre), ".")) {
	case "docx", "xlsx", "pptx":
		return domain.FormatOOXML
	case "odt", "ods", "odp":
		return domain.FormatODF
	case "html", "htm":
		return domain.FormatHTML
	case "md", "markdown":
		return domain.FormatMarkdown
	default:
		return ""
	}
}

// wopiSave guarda el contenido editado en el host, con su bloqueo.
func (h *Handler) wopiSave(w http.ResponseWriter, r *http.Request) {
	var req wopiSaveReq
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		return
	}
	if req.Src == "" || req.Token == "" {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Missing WOPISrc or access token."))
		return
	}
	if !hostPermitido(req.Src, h.wopiHosts) {
		httpx.Fail(w, r, httpx.ErrForbidden.WithMessage("This file host is not allowed."))
		return
	}
	kind := domain.Kind(req.Kind)
	if !kind.Valid() {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Unknown document kind."))
		return
	}

	c := wopi.New(30 * time.Second)
	info, err := c.CheckFileInfo(r.Context(), req.Src, req.Token)
	if err != nil {
		httpx.Fail(w, r, httpx.ErrForbidden.WithMessage("The file could not be opened."))
		return
	}
	// El permiso lo dice el HOST, y se vuelve a preguntar al guardar y no solo al
	// abrir: entre las dos cosas puede haber pasado media hora y habérsele
	// retirado el acceso a esa persona.
	if !info.UserCanWrite {
		httpx.Fail(w, r, httpx.ErrForbidden.WithMessage("You cannot edit this file."))
		return
	}
	formato := formatoDe(info.BaseFileName)
	if formato == "" {
		httpx.Fail(w, r, httpx.ErrUnprocessable.WithMessage("This file format cannot be saved."))
		return
	}

	// El modelo del editor a bytes del formato original. Se monta un estado con el
	// contenido en su clave, que es la forma en la que el exportador espera
	// recibirlo — la misma que usa «Descargar como».
	estado := domain.NewState()
	valor, err := json.Marshal(req.Content)
	if err != nil {
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	}
	if _, err := estado.Apply(domain.Op{
		Kind:    domain.OpSet,
		Key:     claveDe(kind),
		Value:   valor,
		Dot:     domain.Dot{Actor: "wopi", Counter: 1},
		Lamport: 1,
	}); err != nil {
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	}
	datos, _, _, err := domain.Export(kind, info.BaseFileName, estado, formato)
	if err != nil {
		httpx.Fail(w, r, httpx.ErrUnprocessable.WithMessage("The file could not be written."))
		return
	}

	// Bloqueo. Si no se tenía, se toma ahora; el identificador vuelve al cliente
	// para que los guardados siguientes lo reutilicen en vez de pelearse consigo
	// mismo por el mismo fichero.
	lock := req.Lock
	tomadoAqui := false
	if lock == "" {
		lock = "sytue-docs-" + info.Version
		if err := c.Lock(r.Context(), req.Src, req.Token, lock); err != nil {
			responderBloqueado(w, r, err)
			return
		}
		tomadoAqui = true
	}

	if err := c.PutFile(r.Context(), req.Src, req.Token, datos, lock); err != nil {
		// Si el bloqueo se tomó en esta misma llamada y el guardado falló, se suelta:
		// dejarlo puesto haría el fichero inaccesible para los demás hasta que
		// caducara, por un guardado que no llegó a ocurrir.
		if tomadoAqui {
			_ = c.Unlock(r.Context(), req.Src, req.Token, lock)
		}
		responderBloqueado(w, r, err)
		return
	}

	httpx.JSON(w, r, http.StatusOK, map[string]any{"ok": true, "lock": lock})
}

// wopiUnlock suelta el bloqueo al cerrar el editor.
//
// Va aparte y no dentro del guardado porque el bloqueo debe SOBREVIVIR entre
// guardados: soltarlo tras cada uno abriría una ventana en la que otro puede
// entrar a mitad de una edición.
func (h *Handler) wopiUnlock(w http.ResponseWriter, r *http.Request) {
	var req wopiSaveReq
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		return
	}
	if req.Src == "" || req.Token == "" || req.Lock == "" {
		httpx.Fail(w, r, httpx.ErrBadRequest)
		return
	}
	if !hostPermitido(req.Src, h.wopiHosts) {
		httpx.Fail(w, r, httpx.ErrForbidden)
		return
	}
	if err := wopi.New(15*time.Second).Unlock(r.Context(), req.Src, req.Token, req.Lock); err != nil {
		responderBloqueado(w, r, err)
		return
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"ok": true})
}

// responderBloqueado traduce un ErrLocked a un 409 que la interfaz pueda explicar.
//
// Un «no se pudo guardar» genérico ante un fichero bloqueado deja a la persona sin
// saber qué hacer; un 409 le dice que alguien lo tiene abierto, que es información
// con la que sí puede actuar.
func responderBloqueado(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, wopi.ErrLocked) {
		httpx.Fail(w, r, httpx.ErrConflict.WithMessage("Somebody else is editing this file right now."))
		return
	}
	httpx.Fail(w, r, httpx.ErrUnavailable.WithMessage("The file could not be saved."))
}

// claveDe es la clave del registro donde el editor guarda el modelo de cada tipo
// (domain.ContentKey, el espejo de `CONTENT_KEY` del frontend).
func claveDe(k domain.Kind) string { return domain.ContentKey(k) }
