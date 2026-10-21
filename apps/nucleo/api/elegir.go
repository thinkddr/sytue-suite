// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"sync"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/buscar"
)

// # Elegir algo de la suite: el `/api/suite` de la web, sin BFF
//
// La web tiene UN endpoint para los cinco selectores
// (`apps/frontend/apps/notes/src/pages/api/suite.ts`): Drive se NAVEGA por carpetas;
// Docs, Calendar, Rooms y Notes se BUSCAN. Su BFF no hace más que reenviar a cada
// servicio con el token de la persona y normalizar la respuesta. La app nativa ya tiene
// ese token, así que la misma cuenta se hace aquí, en el cliente, y la autorización la
// sigue aplicando cada servicio: Drive solo lista lo que esta persona puede ver, Rooms
// solo sus salas.
//
// Lo que se elige es una REFERENCIA —id, nombre y enlace público—, nunca una copia: si
// mañana se revoca el permiso, quien la guardó deja de poder abrirla.
//
// # El enlace lleva el prefijo de la app
//
// Sale de AppURL (`/drive/files/…`), como hace el Hub. El BFF de la web lo escribía sin
// él (`https://drive.sytue.eu/files/…`) y en producción eso era un 404: medido el
// 2026-09-23 con `curl` en Drive, Docs, Calendar y Rooms, 404 sin prefijo y 302 al login
// con él. Desde el 2026-09-24 la web también usa `appUrl`, así que los dos escriben el
// mismo enlace, Notes incluido (`/notes/?notebook=…`).
//
// Y desde ADR 0056 lleva la ORGANIZACIÓN detrás del prefijo, la de la SESIÓN de quien
// elige (`Elegir.Org`): los resultados se eligen para abrirse después, y la canónica
// aterriza sin el salto del middleware. Sin org —quien compone no la sabe— salen en la
// forma transicional.

// OrigenSuite es de qué producto se elige (`kind` en la web).
type OrigenSuite string

// Los cinco orígenes del selector.
const (
	ElegirDrive    OrigenSuite = "drive"
	ElegirDocs     OrigenSuite = "docs"
	ElegirCalendar OrigenSuite = "calendar"
	ElegirRooms    OrigenSuite = "rooms"
	ElegirNotes    OrigenSuite = "notes"
)

// Navegable dice si el origen se recorre por carpetas (Drive) en vez de buscarse. La
// diferencia la marca el propio origen, como en la web: no es una opción que recordar.
func (o OrigenSuite) Navegable() bool { return o == ElegirDrive }

// Elegible es un candidato ya normalizado: el `Elegible` del BFF.
type Elegible struct {
	ID     string
	Nombre string
	// Carpeta: en Drive las carpetas se navegan, no se eligen. No llevan URL.
	Carpeta bool
	// URL es el enlace público de la referencia. Sin él, la fila no se puede elegir
	// (`disabled={!it.isDirectory && !it.url}`).
	URL string
	// Meta es lo que va a la derecha de la fila: el tipo de documento, el inicio del
	// evento, el tema del canal o el cuaderno de la nota.
	Meta string
	// Bytes es el tamaño de un fichero de Drive; lo escribe quien pinta, con su idioma.
	Bytes int64
}

// PaginaSuite es una página de candidatos y el cursor de la siguiente ("" si no hay más:
// solo Drive pagina).
type PaginaSuite struct {
	Elegibles []Elegible
	Cursor    string
}

// Elegir reúne los clientes de los cinco servicios con la sesión de quien elige. Un
// cliente a nil deja su origen fuera: pedirlo devuelve ErrOrigenNoDisponible.
type Elegir struct {
	Apex string
	// Org es la organización activa de quien elige, leída VIVA de su sesión (el claim
	// `tenant` del token): encabeza los enlaces de los resultados (ADR 0056). Con
	// sesión y sin reclamo —o sin ella— sale nil y los enlaces van sin org. Se pasa
	// como func y no como valor porque cambiar de organización es volver a entrar con
	// la app abierta: el valor de arranque se queda viejo.
	Org      func() string
	Drive    *Drive
	Docs     *Docs
	Calendar *Calendars
	Rooms    *Rooms
	Notes    *Notes
}

// Organizacion es la organización del selector, con el nil del campo como «no la sé».
// La usan también los llamadores que componen enlaces con la misma sesión (MensajeNota
// desde Notepads).
func (e Elegir) Organizacion() string {
	if e.Org == nil {
		return ""
	}
	return e.Org()
}

// ErrOrigenNoDisponible es pedir un origen sin su cliente.
var ErrOrigenNoDisponible = errors.New("api: ese origen no está en el selector")

// Los topes son los del BFF: treinta documentos, veinticinco eventos y notas.
const (
	topeDocs    = 30
	topeBuscado = 25
)

// Listar es `GET /api/suite?kind=…`. En Drive, `carpeta` es el id de la carpeta que se
// mira ("" es «Mi unidad») y `cursor` la página; en los otros cuatro, `q` es lo buscado.
//
// En Drive, un término BUSCA por nombre en toda la unidad. Es lo que pide la herramienta
// de enlaces de Notepads (`listarDeLaSuite('drive', { q })`); el BFF de la web ignora el
// término y devuelve la raíz, así que allí salen siempre los ocho primeros de «Mi
// unidad» escribas lo que escribas. El selector navegable no manda término.
//
// Calendar y Notes no listan nada sin término, como la web: «los últimos eventos» de
// todos los calendarios no es la respuesta a ninguna pregunta.
func (e Elegir) Listar(ctx context.Context, o OrigenSuite, carpeta, q, cursor string) (PaginaSuite, error) {
	q = strings.TrimSpace(q)
	switch o {
	case ElegirDrive:
		if e.Drive != nil {
			return e.drive(ctx, carpeta, q, cursor)
		}
	case ElegirDocs:
		if e.Docs != nil {
			return e.docs(ctx, q)
		}
	case ElegirCalendar:
		if e.Calendar != nil {
			return e.calendar(ctx, q)
		}
	case ElegirRooms:
		if e.Rooms != nil {
			return e.rooms(ctx, q)
		}
	case ElegirNotes:
		if e.Notes != nil {
			return e.notes(ctx, q)
		}
	}
	return PaginaSuite{}, ErrOrigenNoDisponible
}

func (e Elegir) drive(ctx context.Context, carpeta, q, cursor string) (PaginaSuite, error) {
	if q != "" {
		h, err := e.Drive.Search(ctx, q)
		return e.deDrive(h), err
	}
	if carpeta == "" {
		raiz, err := e.Drive.Root(ctx)
		if err != nil {
			return PaginaSuite{}, err
		}
		carpeta = raiz.ID
	}
	h, err := e.Drive.Children(ctx, carpeta, cursor)
	if err != nil {
		return PaginaSuite{}, err
	}
	return e.deDrive(h), nil
}

// deDrive normaliza una página de Drive: carpetas primero, que se navegan, y ficheros.
func (e Elegir) deDrive(h Children) PaginaSuite {
	out := PaginaSuite{Cursor: h.NextCursor, Elegibles: make([]Elegible, 0, len(h.Folders)+len(h.Files))}
	for _, c := range h.Folders {
		out.Elegibles = append(out.Elegibles, Elegible{ID: c.ID, Nombre: c.Name, Carpeta: true})
	}
	for _, f := range h.Files {
		out.Elegibles = append(out.Elegibles, Elegible{
			ID: f.ID, Nombre: f.Name, URL: EnlaceFichero(e.Apex, e.Organizacion(), f.ID), Bytes: f.SizeBytes,
		})
	}
	return out
}

func (e Elegir) docs(ctx context.Context, q string) (PaginaSuite, error) {
	ds, _, err := e.Docs.List(ctx, DocsFilter{Query: q, Limit: topeDocs})
	out := PaginaSuite{Elegibles: make([]Elegible, 0, len(ds))}
	for _, d := range ds {
		out.Elegibles = append(out.Elegibles, Elegible{
			ID: d.ID, Nombre: d.Title, Meta: string(d.Kind),
			URL: EnlaceDocumento(e.Apex, e.Organizacion(), d.ID, d.Kind),
		})
	}
	return out, err
}

func (e Elegir) calendar(ctx context.Context, q string) (PaginaSuite, error) {
	if q == "" {
		return PaginaSuite{}, nil
	}
	evs, err := e.Calendar.SearchEvents(ctx, q, topeBuscado)
	out := PaginaSuite{Elegibles: make([]Elegible, 0, len(evs))}
	for _, ev := range evs {
		// La IDENTIDAD que se guarda es el UID (RFC 5545), no el id interno: es lo que
		// la superficie CalDAV tiene a mano para preguntar por el otro extremo del
		// enlace. El id solo compone la URL.
		out.Elegibles = append(out.Elegibles, Elegible{
			ID: ev.UID, Nombre: ev.Summary, Meta: ev.DTStart.Format(time.RFC3339),
			URL: AppURL(e.Apex, "calendar", e.Organizacion(), "/events/"+url.PathEscape(ev.ID)),
		})
	}
	return out, err
}

// rooms son los canales de TODOS los equipos de la persona, filtrados por nombre. Un
// equipo que falle no se lleva a los demás (el `listChannels` de la web).
func (e Elegir) rooms(ctx context.Context, q string) (PaginaSuite, error) {
	equipos, err := e.Rooms.Teams(ctx)
	if err != nil {
		return PaginaSuite{}, err
	}
	tandas := make([][]RoomsChannel, len(equipos))
	var wg sync.WaitGroup
	for i, t := range equipos {
		wg.Go(func() { tandas[i], _ = e.Rooms.Channels(ctx, t.ID) })
	}
	wg.Wait()
	bajo := buscar.Plegar(q)
	var out PaginaSuite
	for _, tanda := range tandas {
		for _, c := range tanda {
			nombre := c.Name
			if nombre == "" {
				nombre = strings.Join(c.Peers, ", ") // un directo sin nombre: los otros
			}
			if bajo != "" && !strings.Contains(buscar.Plegar(nombre), bajo) {
				continue
			}
			out.Elegibles = append(out.Elegibles, Elegible{
				ID: c.ID, Nombre: nombre, Meta: c.Topic,
				URL: AppURL(e.Apex, "rooms", e.Organizacion(), "/channels/"+url.PathEscape(c.ID)),
			})
		}
	}
	return out, nil
}

func (e Elegir) notes(ctx context.Context, q string) (PaginaSuite, error) {
	if q == "" {
		return PaginaSuite{}, nil
	}
	hits, err := e.Notes.Search(ctx, q, topeBuscado, false)
	out := PaginaSuite{Elegibles: make([]Elegible, 0, len(hits))}
	for _, h := range hits {
		nombre := h.Title
		if nombre == "" {
			nombre = h.Notebook
		}
		// Meta es el cuaderno: dos notas pueden llamarse «Pendientes», y en una lista de
		// resultados el cuaderno es lo único que las distingue.
		out.Elegibles = append(out.Elegibles, Elegible{
			ID: h.ID, Nombre: nombre, Meta: h.Notebook, URL: EnlaceNota(e.Apex, e.Organizacion(), h.NotebookID, h.ID),
		})
	}
	return out, err
}

// EnlaceFichero es el enlace canónico de un fichero de Drive, el que usan la búsqueda
// federada y el Hub (`{drive}/files/{id}`), con el prefijo de la app y —si se sabe— la
// organización detrás de él (ADR 0056). Es el espejo de `driveFileUrl` de la web, que va
// SIN org a propósito cuando el enlace viaja DENTRO de un correo: quien lo pulsa puede
// no ser de la organización.
func EnlaceFichero(apex, org, id string) string {
	return AppURL(apex, "drive", org, "/files/"+url.PathEscape(id))
}

// EnlaceNota es el enlace profundo a una nota: el MISMO que los resultados de búsqueda de
// la web, para que una referencia guardada hoy siga abriendo la nota mañana. La org es
// la de la sesión de quien lo compone, detrás del prefijo (ADR 0056).
func EnlaceNota(apex, org, cuaderno, nota string) string {
	return AppURL(apex, "notes", org, "/?notebook="+url.QueryEscape(cuaderno)+"&note="+url.QueryEscape(nota))
}
