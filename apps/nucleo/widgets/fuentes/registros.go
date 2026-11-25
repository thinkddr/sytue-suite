// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package fuentes

import (
	"context"
	"errors"
	"sync"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/auth"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/widgets"
)

// # Las apps de registros: el servidor o el repositorio privado
//
// Calendar, Contacts, Notes, Photos, Bases, ERP, Cast y la ofimática leen y escriben por la
// API de su servicio mientras la fuente del servidor de esa cuenta siga `legacy` (ADR 0086,
// «clientes nativos»). Desde que un dispositivo copia sus datos a Matrix, esa fuente queda
// congelada y los datos viven en el repositorio privado: un widget que leyera el servidor
// enseñaría lo de antes de pasar y nunca lo nuevo, como el de Drive «que no cambia». Así
// que, como Drive (privada.go):
//
//   - con la app abierta sobre su repositorio, la Fuente lee ESE repositorio (Privados, que
//     monta sesion.Suite.WidgetDesdeRegistrosPrivados en una Privada);
//   - sin él —la app por la API, al fondo en iOS o el refresco sin ventana de Android—, la
//     registrada pregunta antes al servicio el modo de la fuente de la cuenta (una
//     petición, recordada VigenciaModo) y solo lee el servidor si sigue `legacy`; si no,
//     falla con ErrSinDatosPrivados y el Publicador conserva lo publicado desde el
//     repositorio, con su antigüedad: nunca lo congelado.
//
// Recordar el modo no enseña nada viejo: justo al congelarse, lo del servidor es lo último
// que había; y al soltar la fuente (volver a `legacy`) se conserva lo publicado hasta
// volver a preguntar.

// fuenteLegacy es el modo de la fuente que sigue siendo la de la web (cifrado.FuenteLegacy,
// que no se importa aquí: arrastraría Fyne).
const fuenteLegacy = "legacy"

// VigenciaModo es cuánto se recuerda, por cuenta, el modo de la fuente del servidor.
const VigenciaModo = 30 * time.Minute

// modoFuente lee el modo de la fuente del servidor de un producto de registros (`legacy`,
// `building`, `frozen` o `private`): el mismo `Modo` con el que su `cmd` elige entre la API
// y Matrix (cifrado.AplicacionRegistrosConfig).
type modoFuente func(ctx context.Context, url func(string) string, s api.Session, scope storage.Scope) (string, error)

// modos son las apps de registros con widget, por producto.
var modos = map[string]modoFuente{
	"bases": func(ctx context.Context, url func(string) string, s api.Session, scope storage.Scope) (string, error) {
		e, err := api.NewBases(url("bases"), s, nil).PrivateStorageState(ctx, scope)
		return e.Mode, err
	},
	"calendar": func(ctx context.Context, url func(string) string, s api.Session, scope storage.Scope) (string, error) {
		e, err := api.NewCalendars(url("calendar"), s, nil).PrivateStorageState(ctx, scope)
		return e.Mode, err
	},
	"cast": func(ctx context.Context, url func(string) string, s api.Session, scope storage.Scope) (string, error) {
		e, err := api.NewCast(url("cast"), s, nil).PrivateStorageState(ctx, scope)
		return e.Mode, err
	},
	// Contacts, por su modo y no por el estado de la migración: una libreta compartida no
	// impide ir por la API (cmd/contacts, contactsMode).
	"contacts": func(ctx context.Context, url func(string) string, s api.Session, scope storage.Scope) (string, error) {
		return api.NewContacts(url("contacts"), s, nil).PrivateStorageMode(ctx, scope)
	},
	"docs": func(ctx context.Context, url func(string) string, s api.Session, scope storage.Scope) (string, error) {
		e, err := api.NewDocs(url("docs"), s, nil).PrivateStorageState(ctx, scope)
		return e.Mode, err
	},
	"erp": func(ctx context.Context, url func(string) string, s api.Session, scope storage.Scope) (string, error) {
		e, err := api.NewERP(url("erp"), s, nil, "").PrivateStorageState(ctx, scope)
		return e.Mode, err
	},
	"notes": func(ctx context.Context, url func(string) string, s api.Session, scope storage.Scope) (string, error) {
		e, err := api.NewNotes(url("notes"), s, nil).PrivateStorageState(ctx, scope)
		return e.Mode, err
	},
	"photos": func(ctx context.Context, url func(string) string, s api.Session, scope storage.Scope) (string, error) {
		e, err := api.NewPhotos(url("photos"), s, nil).PrivateStorageState(ctx, scope)
		return e.Mode, err
	},
	"sheets": func(ctx context.Context, url func(string) string, s api.Session, scope storage.Scope) (string, error) {
		e, err := api.NewDocs(url("docs"), s, nil).SheetsSource().PrivateStorageState(ctx, scope)
		return e.Mode, err
	},
	"slides": func(ctx context.Context, url func(string) string, s api.Session, scope storage.Scope) (string, error) {
		e, err := api.NewSlides(url("slides"), s, nil).PrivateStorageState(ctx, scope)
		return e.Mode, err
	},
}

// DeRegistros dice si `producto` es una app de registros: su widget lee el repositorio
// privado cuando la app lo tiene abierto (Privados) y, sin él, el servidor solo si la
// fuente de la cuenta sigue `legacy`.
func DeRegistros(producto string) bool {
	_, ok := modos[producto]
	return ok
}

// identificada es la sesión que dice de quién es (auth.Session): el ámbito con el que se
// pregunta el modo de la fuente.
type identificada interface {
	SnapshotIdentity() auth.IdentitySnapshot
}

func identidad(ses api.Session) (auth.IdentitySnapshot, bool) {
	if s, ok := ses.(*auth.Session); ok && s == nil {
		return auth.IdentitySnapshot{}, false
	}
	i, ok := ses.(identificada)
	if !ok {
		return auth.IdentitySnapshot{}, false
	}
	yo := i.SnapshotIdentity()
	return yo, yo.Subject != ""
}

// conModo es la Fuente registrada de una app de registros: `servidor`, solo si la fuente
// del servidor de la cuenta de `ses` sigue `legacy` (arriba). Sin saber de quién es la
// sesión no se lee nada: ErrSinDatosPrivados, que conserva lo publicado.
func conModo(producto string, c Comunes, url func(string) string, ses api.Session, servidor widgets.Fuente) widgets.Fuente {
	modo := modos[producto]
	var (
		mu     sync.Mutex
		quien  auth.IdentitySnapshot
		legacy bool
		hasta  time.Time
	)
	return func(ctx context.Context) (widgets.Detail, error) {
		yo, ok := identidad(ses)
		if !ok {
			return widgets.Detail{}, ErrSinDatosPrivados
		}
		ahora := c.ahora()
		mu.Lock()
		sabido, esLegacy := quien == yo && ahora.Before(hasta), legacy
		mu.Unlock()
		if !sabido {
			m, err := modo(ctx, url, ses, storage.Scope{Tenant: yo.Organization, Subject: yo.Subject, Product: producto})
			if err != nil {
				return widgets.Detail{}, err
			}
			esLegacy = m == fuenteLegacy
			mu.Lock()
			quien, legacy, hasta = yo, esLegacy, ahora.Add(VigenciaModo)
			mu.Unlock()
		}
		if !esLegacy {
			return widgets.Detail{}, ErrSinDatosPrivados
		}
		return servidor(ctx)
	}
}

// Privados es la Fuente de `producto` sobre la fachada que su app acaba de montar sobre el
// repositorio privado (`datos`, el *api.Calendars, *api.Notes… de WithPrivateData) y, para
// lo que no es dato privado —la cuota de Cast—, el servicio con la sesión de esa cuenta
// (`cuenta`). false si `producto` no es de registros o `datos` no es su fachada.
func Privados(producto string, c Comunes, datos any, url func(string) string, cuenta api.Session) (widgets.Fuente, bool) {
	var f widgets.Fuente
	switch d := datos.(type) {
	case *api.Bases:
		f = siEs(producto == "bases", func() widgets.Fuente { return Bases(c, d) })
	case *api.Calendars:
		f = siEs(producto == "calendar", func() widgets.Fuente { return Calendar(c, d) })
	case *api.Cast:
		f = siEs(producto == "cast", func() widgets.Fuente { return Cast(c, d, api.NewCast(url("cast"), cuenta, nil)) })
	case *api.Contacts:
		f = siEs(producto == "contacts", func() widgets.Fuente { return Contacts(c, d) })
	case *api.ERP:
		f = siEs(producto == "erp", func() widgets.Fuente { return ERP(c, d) })
	case *api.Notes:
		f = siEs(producto == "notes", func() widgets.Fuente { return Notes(c, d) })
	case *api.Photos:
		f = siEs(producto == "photos", func() widgets.Fuente { return Photos(c, d) })
	case *api.Docs:
		switch producto {
		case "docs":
			f = Docs(c, d, api.DocText)
		case "sheets":
			f = Docs(c, d, api.DocSheet)
		}
	case *api.Slides:
		// Diapos monta un *api.Slides; su lista está en la fachada de Docs que comparte su
		// repositorio.
		f = siEs(producto == "slides", func() widgets.Fuente { return Docs(c, d.Docs(), api.DocSlides) })
	}
	return f, f != nil && DeRegistros(producto)
}

func siEs(ok bool, f func() widgets.Fuente) widgets.Fuente {
	if !ok {
		return nil
	}
	return f()
}

// noEnPrivado: una lectura que el repositorio privado no tiene (los álbumes de otra
// persona, lo publicado en la organización, las tareas del ERP). La fachada privada
// nunca vuelve al servidor (api.*Data): ese dato no existe para esa cuenta.
func noEnPrivado(err error) bool {
	for _, e := range []error{
		api.ErrPrivateBasesOperation, api.ErrPrivateCalendarOperation, api.ErrPrivateCastOperation,
		api.ErrPrivateContactsOperation, api.ErrPrivateDataOnly, api.ErrPrivateERPOperation,
		api.ErrPrivateNotesOperation, api.ErrPrivatePhotosOperation,
	} {
		if errors.Is(err, e) {
			return true
		}
	}
	return false
}
