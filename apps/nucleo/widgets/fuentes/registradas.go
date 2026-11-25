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
// a las apps. Las de las apps de registros leen el servidor solo con la fuente de la
// cuenta en `legacy` (conModo, registros.go): con ella en Matrix, la app monta la suya
// sobre el repositorio privado (Privados).
var registradas = map[string]Fabrica{
	"bases": func(c Comunes, url func(string) string, s api.Session) widgets.Fuente {
		return conModo("bases", c, url, s, Bases(c, api.NewBases(url("bases"), s, nil)))
	},
	"calendar": func(c Comunes, url func(string) string, s api.Session) widgets.Fuente {
		return conModo("calendar", c, url, s, Calendar(c, api.NewCalendars(url("calendar"), s, nil)))
	},
	"cast": func(c Comunes, url func(string) string, s api.Session) widgets.Fuente {
		cl := api.NewCast(url("cast"), s, nil)
		return conModo("cast", c, url, s, Cast(c, cl, cl))
	},
	"contacts": func(c Comunes, url func(string) string, s api.Session) widgets.Fuente {
		return conModo("contacts", c, url, s, Contacts(c, api.NewContacts(url("contacts"), s, nil)))
	},
	"docs": func(c Comunes, url func(string) string, s api.Session) widgets.Fuente {
		return conModo("docs", c, url, s, Docs(c, api.NewDocs(url("docs"), s, nil), api.DocText))
	},
	"sheets": func(c Comunes, url func(string) string, s api.Session) widgets.Fuente {
		return conModo("sheets", c, url, s, Docs(c, api.NewDocs(url("docs"), s, nil), api.DocSheet))
	},
	"slides": func(c Comunes, url func(string) string, s api.Session) widgets.Fuente {
		return conModo("slides", c, url, s, Docs(c, api.NewDocs(url("docs"), s, nil), api.DocSlides))
	},
	// Drive y CertiPDF guardan en el repositorio privado (la bóveda o Matrix): sin la app
	// abierta no hay de dónde leer, y el servidor está congelado. La app monta la suya
	// (Privada); aquí, la del refresco sin ventana, se conserva lo publicado.
	"drive": func(Comunes, func(string) string, api.Session) widgets.Fuente {
		return sinDatosPrivados()
	},
	"erp": func(c Comunes, url func(string) string, s api.Session) widgets.Fuente {
		return conModo("erp", c, url, s, ERP(c, api.NewERP(url("erp"), s, nil, string(c.idioma()))))
	},
	"mail": func(c Comunes, url func(string) string, s api.Session) widgets.Fuente {
		return Mail(c, api.NewMail(url("mail"), s, nil))
	},
	"notes": func(c Comunes, url func(string) string, s api.Session) widgets.Fuente {
		return conModo("notes", c, url, s, Notes(c, api.NewNotes(url("notes"), s, nil)))
	},
	"one": func(c Comunes, url func(string) string, s api.Session) widgets.Fuente {
		return Hub(c, api.NewHub(url("hub"), s, nil), api.NewCalendars(url("calendar"), s, nil), api.NewMail(url("mail"), s, nil))
	},
	"pdf": func(Comunes, func(string) string, api.Session) widgets.Fuente {
		return sinDatosPrivados()
	},
	"photos": func(c Comunes, url func(string) string, s api.Session) widgets.Fuente {
		return conModo("photos", c, url, s, Photos(c, api.NewPhotos(url("photos"), s, nil)))
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
