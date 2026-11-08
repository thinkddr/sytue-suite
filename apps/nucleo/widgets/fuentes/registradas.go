// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package fuentes

import (
	"maps"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/widgets"
)

// Fabrica construye la Fuente de un producto con sus clientes `api`: los Comunes de la
// app, la URL de cada servicio (api.ServiceURL del apex de la suite) y la sesión. Los
// clientes van SIN aviso de escritura: la Fuente solo lee.
type Fabrica func(c Comunes, url func(servicio string) string, ses api.Session) widgets.Fuente

// registradas es, por producto —el esquema de la app sin «sytue-», como en
// FyneApp.toml—, cómo se construye su Fuente. Es EL sitio: lo usan los `cmd/<app>`
// (sesion.Suite.FuenteWidget) y el refresco sin ventana de Android, que no corre
// ningún `main` (apps/fyne/sesion, fondo_sinventana.go). TestHayUnaFuentePorApp la ata
// a las apps.
var registradas = map[string]Fabrica{
	"bases": func(c Comunes, url func(string) string, s api.Session) widgets.Fuente {
		return Bases(c, api.NewBases(url("bases"), s, nil))
	},
	"calendar": func(c Comunes, url func(string) string, s api.Session) widgets.Fuente {
		return Calendar(c, api.NewCalendars(url("calendar"), s, nil))
	},
	"cast": func(c Comunes, url func(string) string, s api.Session) widgets.Fuente {
		return Cast(c, api.NewCast(url("cast"), s, nil))
	},
	"contacts": func(c Comunes, url func(string) string, s api.Session) widgets.Fuente {
		return Contacts(c, api.NewContacts(url("contacts"), s, nil))
	},
	"docs": func(c Comunes, url func(string) string, s api.Session) widgets.Fuente {
		return Docs(c, api.NewDocs(url("docs"), s, nil), api.DocText)
	},
	"sheets": func(c Comunes, url func(string) string, s api.Session) widgets.Fuente {
		return Docs(c, api.NewDocs(url("docs"), s, nil), api.DocSheet)
	},
	"slides": func(c Comunes, url func(string) string, s api.Session) widgets.Fuente {
		return Docs(c, api.NewDocs(url("docs"), s, nil), api.DocSlides)
	},
	"drive": func(c Comunes, url func(string) string, s api.Session) widgets.Fuente {
		return Drive(c, api.NewDrive(url("drive"), s, nil))
	},
	"erp": func(c Comunes, url func(string) string, s api.Session) widgets.Fuente {
		return ERP(c, api.NewERP(url("erp"), s, nil, string(c.idioma())))
	},
	"mail": func(c Comunes, url func(string) string, s api.Session) widgets.Fuente {
		return Mail(c, api.NewMail(url("mail"), s, nil))
	},
	"notes": func(c Comunes, url func(string) string, s api.Session) widgets.Fuente {
		return Notes(c, api.NewNotes(url("notes"), s, nil))
	},
	"one": func(c Comunes, url func(string) string, s api.Session) widgets.Fuente {
		return Hub(c, api.NewHub(url("hub"), s, nil), api.NewCalendars(url("calendar"), s, nil), api.NewMail(url("mail"), s, nil))
	},
	"pdf": func(c Comunes, url func(string) string, s api.Session) widgets.Fuente {
		return CertiPDF(c, api.NewPDF(url("docs"), url("drive"), s, nil))
	},
	"photos": func(c Comunes, url func(string) string, s api.Session) widgets.Fuente {
		return Photos(c, api.NewPhotos(url("photos"), s, nil))
	},
	"rooms": func(c Comunes, url func(string) string, s api.Session) widgets.Fuente {
		return Rooms(c, api.NewRooms(url("rooms"), s, nil))
	},
}

// Registradas son las Fábricas de las quince apps con widget, por producto. Es una
// copia: quien la recibe no cambia la de nadie.
func Registradas() map[string]Fabrica { return maps.Clone(registradas) }

// Registrada es la Fábrica de `producto`, si tiene widget.
func Registrada(producto string) (Fabrica, bool) {
	f, ok := registradas[producto]
	return f, ok
}
