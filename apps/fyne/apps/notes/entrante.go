// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes

import (
	"net/url"
	"strings"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// Paridad web: los ENLACES ENTRANTES de `pages/index.astro` y `pages/notebooks/[id].astro`.
//
//   - `?notebook=<id>&note=<id>&h=<slug>` (y `/notebooks/<id>?note=…`, la dirección
//     canónica que componen la búsqueda del Hub y los enlaces compartidos en Rooms):
//     abre ese cuaderno, selecciona esa nota y baja hasta ese título.
//   - `?action=create`: la acción «Nueva nota» del widget nativo (api.LeerAccion).
//     Pregunta en el cuaderno abierto y, con el sí, crea una nota de texto, como la de
//     la barra: un enlace del esquema lo abre cualquiera y no escribe nada solo.
//   - `?link=<tipo>:<id>&label=<rótulo>`: el otro extremo de la referencia
//     bidireccional —«escribir una nota para este evento» de Calendar—. Crea UNA nota
//     vinculada en el cuaderno abierto, que es el que elige quien escribe, y la crea
//     DESPUÉS DE PREGUNTAR: la web la crea sin más, pero un enlace del esquema lo abre
//     cualquier página o cualquier app sin que nadie haya pulsado nada en la suite.
//
// Llegan por el esquema de la app (`sesion.AlAbrirURL`), también antes de que se hayan
// cargado los cuadernos: lo que no se puede hacer todavía se queda pendiente y se hace
// al cargar, como las props `notebookInicial`/`vinculoInicial` de la isla.

// Entrante es lo que trae un enlace a la app.
type Entrante struct {
	Cuaderno, Nota, Ancla string
	Vinculo, Rotulo       string
	// Compartida es una nota que alguien me ha compartido (`?shared=`, el aviso
	// `notes.shared`): se abre en «Compartido conmigo».
	Compartida string
	// Crear es la acción «Nueva nota» del widget (`?action=create`).
	Crear bool
}

// TipoPorDestino es `TIPO_POR_DESTINO` de NotesApp.tsx: el tipo de la nota que nace de
// un vínculo, por el tipo de lo vinculado. Lo que no está es una referencia.
var TipoPorDestino = map[string]string{
	"calendar_event": "event",
	"room_channel":   "room",
	"room_message":   "room",
	"docs_document":  "doc",
	"drive_file":     "drive",
	"note":           "crossref",
	"notebook":       "crossref",
}

// LeerEntrante saca de un enlace lo que la app atiende; `false` si no trae nada suyo.
func LeerEntrante(u *url.URL) (Entrante, bool) {
	if u == nil {
		return Entrante{}, false
	}
	q := u.Query()
	e := Entrante{
		Cuaderno: q.Get("notebook"), Nota: q.Get("note"), Ancla: q.Get("h"),
		Vinculo: q.Get("link"), Rotulo: q.Get("label"), Compartida: q.Get("shared"),
		Crear: api.LeerAccion("notes", u) == "create",
	}
	if id, ok := strings.CutPrefix(u.Path, "/notebooks/"); ok && id != "" && !strings.Contains(id, "/") {
		e.Cuaderno = id
	}
	return e, e.Cuaderno != "" || e.Nota != "" || e.Vinculo != "" || e.Compartida != "" || e.Crear
}

// AtenderEnlace atiende un enlace del esquema de Notepads y dice si era suyo. Va en el
// hilo de la interfaz.
func (p *Pantalla) AtenderEnlace(u *url.URL) bool {
	e, ok := LeerEntrante(u)
	if !ok {
		return false
	}
	if e.Compartida != "" {
		p.irA(VistaCompartidas)
		p.comp.abierta = e.Compartida
		return true
	}
	p.entrante = &e
	// El aviso de una petición de edición trae el cuaderno: si es mío, su franja me
	// enseña quién la pide.
	p.cuadernoDelAviso = e.Cuaderno
	p.aplicarEntrante()
	return true
}

// aplicarEntrante hace lo que el enlace pide y se pueda hacer ya. Se llama al llegar el
// enlace, al cargar los cuadernos y al cargar las notas del abierto.
//
//nolint:gocyclo // Coordina deliberadamente todas las fases asíncronas de un deep link.
func (p *Pantalla) aplicarEntrante() {
	e := p.entrante
	if e == nil || len(p.cuadernos) == 0 {
		return // sin cuadernos todavía: al cargarlos
	}
	if e.Cuaderno != "" || e.Nota != "" {
		destino := p.activo
		for _, c := range p.cuadernos {
			if c.ID == e.Cuaderno {
				destino = c
			}
		}
		if visibles := enArbol(p.cuadernos); destino.ID == "" && len(visibles) > 0 {
			destino = visibles[0] // `lista.find(…) ?? cuadernosEnArbol(lista)[0]`
		}
		p.ancla = e.Ancla
		otro := destino.ID != p.activo.ID
		if otro {
			p.abrirCuaderno(destino)
		}
		p.vista = VistaCuaderno
		if e.Nota != "" {
			p.abrirNota(e.Nota)
		}
		e.Cuaderno, e.Nota = "", ""
		if otro && e.Vinculo != "" {
			return // la nota vinculada, cuando lleguen las de este cuaderno
		}
	}
	// «Nueva nota» del widget, cuando hay cuaderno abierto con sus notas: se pregunta una
	// vez (el enlace se consume) y, con el sí, nuevaNota, que pide la contraseña de un
	// cuaderno cerrado como la de la barra.
	if e.Crear {
		if p.activo.ID == "" || p.lienzo.Fase == Cargando {
			return // al cargar el cuaderno o sus notas
		}
		e.Crear = false
		p.vista = VistaCuaderno
		p.preguntarNueva()
	}
	// El vínculo, cuando hay cuaderno abierto, sus notas cargadas y se puede escribir en
	// él (la web espera a `!bloqueado`). Una vez: el enlace se consume al usarlo.
	if e.Vinculo == "" || p.activo.ID == "" || p.bloqueado || p.lienzo.Fase == Cargando {
		if e.Vinculo == "" {
			p.entrante = nil
		}
		return
	}
	tipo, id, ok := strings.Cut(e.Vinculo, ":")
	p.entrante = nil
	if !ok || tipo == "" || id == "" {
		return
	}
	clase := TipoPorDestino[tipo]
	if clase == "" {
		clase = "crossref"
	}
	// La nota NO se crea aquí: se pregunta. El enlace lo puede haber escrito cualquiera
	// —una página, un correo, otra app del teléfono—, y creándola sin más bastaba pulsarlo
	// para dejar una nota con su título en el cuaderno abierto, a la vista de quien lo
	// comparta (NAT-05).
	p.vinculoPendiente = &api.NoteInput{
		Kind: clase, Title: e.Rotulo,
		Links: []api.NoteLink{{TargetKind: tipo, TargetID: id, Label: e.Rotulo}},
	}
	t := p.tr.T
	p.preguntar(t("note.new", nil), e.Rotulo, t("capture.add", nil), false, p.aceptarVinculo)
}

// preguntarNueva es la «Nueva nota» del widget: se pregunta en el cuaderno abierto, con
// su título. Crearla sin más dejaría una nota en blanco, a la vista de quien comparta el
// cuaderno, con solo pulsar un enlace que cualquiera puede abrir (NAT-05).
func (p *Pantalla) preguntarNueva() {
	t := p.tr.T
	p.nuevaPendiente = true
	p.preguntar(t("note.new", nil), p.activo.Titulo, t("capture.add", nil), false, p.aceptarNueva)
}

// aceptarNueva es el «sí» de preguntarNueva: la nota nueva de la barra, UNA vez.
func (p *Pantalla) aceptarNueva() {
	if !p.nuevaPendiente {
		return
	}
	p.nuevaPendiente = false
	p.nuevaNota()
}

// aceptarVinculo crea la nota vinculada que pidió un enlace entrante, UNA vez: es el «sí»
// de su pregunta. Sin pregunta pendiente no hace nada.
func (p *Pantalla) aceptarVinculo() {
	in := p.vinculoPendiente
	p.vinculoPendiente = nil
	if in != nil {
		p.lanzar(func() error { return p.crear(*in) })
	}
}
