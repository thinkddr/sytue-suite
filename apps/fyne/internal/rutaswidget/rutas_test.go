// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package rutaswidget_test ata la ruta de cada fila del widget nativo (formato 3,
// widgets.Item.Route) al receptor de enlaces de SU app: lo que escribe la Fuente
// (apps/nucleo/widgets/fuentes) con el esquema de la app delante es lo que esa app ya
// atiende por `sesion.AlAbrirURL` o `sesion.AlAbrirFichero`, y abre ESE elemento. Si una
// app cambia su enlace, o una Fuente su ruta, esto falla.
package rutaswidget_test

import (
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/fyne/apps/bases"
	"git.sytue.eu/sytue/one/apps/fyne/apps/cast"
	"git.sytue.eu/sytue/one/apps/fyne/apps/drive"
	"git.sytue.eu/sytue/one/apps/fyne/apps/erp"
	"git.sytue.eu/sytue/one/apps/fyne/apps/hub"
	"git.sytue.eu/sytue/one/apps/fyne/apps/notes"
	"git.sytue.eu/sytue/one/apps/fyne/calcus"
	"git.sytue.eu/sytue/one/apps/fyne/calendario"
	"git.sytue.eu/sytue/one/apps/fyne/contactos"
	"git.sytue.eu/sytue/one/apps/fyne/diapos"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/widgets"
	"git.sytue.eu/sytue/one/apps/nucleo/widgets/fuentes"
)

// abrir es lo que hace el lector nativo con la ruta de una fila: el esquema firmado de la
// app, `:` y la ruta. Lo mismo que SytueWidgetActions.route (Swift) y Widgets.java.
func abrir(t *testing.T, producto, ruta string) *url.URL {
	t.Helper()
	if ruta == "" || !widgets.ValidRoute(ruta) {
		t.Fatalf("%s: ruta %q", producto, ruta)
	}
	u, err := url.Parse(api.Esquema(producto) + ":" + ruta)
	if err != nil || u.Scheme != api.Esquema(producto) || u.Host != "" || u.Opaque != "" {
		t.Fatalf("%s: %q no es un enlace de su app: %+v, %v", producto, ruta, u, err)
	}
	return u
}

func TestCadaFilaDelWidgetAbreSuElemento(t *testing.T) {
	const uuid = "0b5c6f3e-1d2a-4c8e-9f00-112233445566"
	madrid, err := time.LoadLocation("Europe/Madrid")
	if err != nil {
		t.Skip("sin tzdata")
	}

	t.Run("calendar", func(t *testing.T) {
		for _, diaCompleto := range []bool{false, true} {
			u := abrir(t, "calendar", fuentes.RutaDia(time.Date(2026, 11, 30, 18, 30, 0, 0, madrid), diaCompleto))
			if d, ok := calendario.FechaDelEnlace(u.Query(), madrid); !ok || d.Day() != 30 || d.Month() != 11 {
				t.Fatalf("%s lleva a %v", u, d)
			}
		}
	})
	t.Run("notes", func(t *testing.T) {
		u := abrir(t, "notes", fuentes.RutaNota("cuaderno-1", "nota 2"))
		if e, ok := notes.LeerEntrante(u); !ok || e.Cuaderno != "cuaderno-1" || e.Nota != "nota 2" || e.Vinculo != "" {
			t.Fatalf("%s abre %+v", u, e)
		}
	})
	t.Run("contacts", func(t *testing.T) {
		if u := abrir(t, "contacts", fuentes.RutaContacto(uuid)); contactos.IDDeEnlace(u) != uuid {
			t.Fatalf("%s no abre la ficha", u)
		}
	})
	t.Run("drive", func(t *testing.T) {
		nombre := "Q3 & Q4 · ñ+1.pdf"
		carpeta, fichero, ok := drive.DestinoDeEnlace(abrir(t, "drive", fuentes.RutaDrive(nombre, false)))
		if !ok || carpeta != "/" || fichero != "/"+nombre {
			t.Fatalf("un fichero de la raíz abre %q, %q, %v", carpeta, fichero, ok)
		}
		carpeta, fichero, ok = drive.DestinoDeEnlace(abrir(t, "drive", fuentes.RutaDrive("Proyectos 2026", true)))
		if !ok || carpeta != "/Proyectos 2026" || fichero != "" {
			t.Fatalf("una carpeta de la raíz abre %q, %q, %v", carpeta, fichero, ok)
		}
	})
	t.Run("docs, sheets y slides", func(t *testing.T) {
		if id := api.IDDeEdicion("docs", abrir(t, "docs", fuentes.RutaDocumento("d1"))); id != "d1" {
			t.Fatalf("Docus abre %q", id)
		}
		if id := calcus.IDDeEnlace(abrir(t, "sheets", fuentes.RutaDocumento("h1"))); id != "h1" {
			t.Fatalf("Calcus abre %q", id)
		}
		if id := diapos.IDDeEnlace(abrir(t, "slides", fuentes.RutaDocumento("p1"))); id != "p1" {
			t.Fatalf("Diapos abre %q", id)
		}
	})
	t.Run("pdf", func(t *testing.T) {
		ap, ok := api.LeerApertura("pdf", abrir(t, "pdf", fuentes.RutaPDF("/PDF/Contrato firmado.pdf")))
		if !ok || ap.Ref.Ruta != "/PDF/Contrato firmado.pdf" {
			t.Fatalf("CertiPDF abre %+v", ap)
		}
	})
	t.Run("bases", func(t *testing.T) {
		if base, ok := bases.DestinoDeEnlace(abrir(t, "bases", fuentes.RutaBase(uuid))); !ok || base != uuid {
			t.Fatalf("Bases abre %q", base)
		}
	})
	t.Run("erp", func(t *testing.T) {
		if v, ok := api.LeerVista(abrir(t, "erp", fuentes.RutaTareasERP())); !ok || v != erp.VistaTareas {
			t.Fatalf("el ERP abre la vista %q", v)
		}
	})
	t.Run("photos", func(t *testing.T) {
		if id, ok := api.LeerAlbum(abrir(t, "photos", fuentes.RutaAlbum("al1"))); !ok || id != "al1" {
			t.Fatalf("Photos abre el álbum %q", id)
		}
		if ap, ok := api.LeerApertura("photos", abrir(t, "photos", fuentes.RutaFoto("df1"))); !ok || ap.Ref.ID != "df1" {
			t.Fatalf("Photos abre la foto %+v", ap)
		}
	})
	t.Run("cast", func(t *testing.T) {
		u := abrir(t, "cast", fuentes.RutaVideo("v1"))
		if !cast.EsEnlaceDeCast(u) || cast.VideoDeEnlace(u) != "v1" {
			t.Fatalf("Cast abre %q", cast.VideoDeEnlace(u))
		}
	})
	t.Run("rooms", func(t *testing.T) {
		if canal, _, ok := api.LeerCanal(abrir(t, "rooms", fuentes.RutaCanal("!c1:sytue.eu"))); !ok || canal != "!c1:sytue.eu" {
			t.Fatalf("Rooms abre el canal %q", canal)
		}
	})
	t.Run("one", func(t *testing.T) {
		if d, ok := hub.DestinoDeRuta(abrir(t, "one", fuentes.RutaActividad()).Path); !ok || d != hub.AActividad {
			t.Fatalf("One abre %v", d)
		}
	})
	// La agenda de hoy de One: su día en Calendar (hub.DiaDeAgenda), la misma ruta que la
	// fila de «Hoy» y que Calendar lee.
	t.Run("one, agenda", func(t *testing.T) {
		for _, diaCompleto := range []bool{false, true} {
			u := abrir(t, "one", fuentes.RutaAgenda(time.Date(2026, 11, 30, 18, 30, 0, 0, madrid), diaCompleto))
			ruta, ok := hub.DiaDeAgenda(u)
			if !ok {
				t.Fatalf("One no atiende %s", u)
			}
			cal, err := url.Parse(api.Esquema("calendar") + ":" + ruta)
			if err != nil {
				t.Fatal(err)
			}
			if d, ok := calendario.FechaDelEnlace(cal.Query(), madrid); !ok || d.Day() != 30 || d.Month() != 11 ||
				(cal.Query().Get("all_day") == "1") != diaCompleto {
				t.Fatalf("%s lleva a Calendar %s: %v", u, ruta, d)
			}
		}
		for _, malo := range []string{"/agenda", "/agenda?date=ayer", "/agenda?date=2026-11-30&x=1", "/agenda?date=2026-11-30&all_day=0"} {
			if u, _ := url.Parse(api.Esquema("one") + ":" + malo); u != nil {
				if _, ok := hub.DiaDeAgenda(u); ok {
					t.Errorf("One atiende %s", malo)
				}
			}
		}
	})
	t.Run("mail", func(t *testing.T) {
		if hilo, ok := api.LeerHilo(abrir(t, "mail", fuentes.RutaHilo("<t1@sytue.eu>"))); !ok || hilo != "<t1@sytue.eu>" {
			t.Fatalf("Mail abre el hilo %q", hilo)
		}
	})
}

// TestCadaAccionDelWidgetTieneQuienLaAtienda: cada acción rápida que declara un
// FyneApp.toml (`widget_acciones`, la que ponen los widgets de Apple y de Android como
// `sytue-<producto>:/?action=<acción>`) la lee el receptor de esa app y abre un
// formulario, una pregunta o el selector: Calendar su formulario, Contacts el suyo
// (contactos.App.PedirNuevo), Notes la pregunta de «Nueva nota» (notes.Entrante.Crear),
// Photos el selector (photos.App.AnyadirFotos) y la ofimática la pregunta de «Documento
// nuevo» (editor.Portada.PreguntarCrear). Una acción nueva sin receptor falla aquí.
func TestCadaAccionDelWidgetTieneQuienLaAtienda(t *testing.T) {
	atiende := map[string]func(*url.URL) bool{
		"calendar/create": func(u *url.URL) bool {
			_, ok := calendario.LeerIntencion(u.Query(), time.Now(), time.UTC)
			return ok
		},
		"contacts/create": func(u *url.URL) bool { return api.LeerAccion("contacts", u) == "create" },
		"notes/create": func(u *url.URL) bool {
			e, ok := notes.LeerEntrante(u)
			return ok && e.Crear
		},
		"photos/upload": func(u *url.URL) bool { return api.LeerAccion("photos", u) == "upload" },
		// Docus, Calcus y Diapos preguntan antes de crear (editor.Portada.PreguntarCrear,
		// desde el AlAbrirURL de su cmd).
		"docs/create":   func(u *url.URL) bool { return api.LeerAccion("docs", u) == "create" },
		"sheets/create": func(u *url.URL) bool { return api.LeerAccion("sheets", u) == "create" },
		"slides/create": func(u *url.URL) bool { return api.LeerAccion("slides", u) == "create" },
		// Mail la atiende su propia rama (cmd/mail, fuera de este cambio).
		"mail/compose": func(u *url.URL) bool { return api.LeerAccion("mail", u) == "compose" },
	}
	tomls, err := filepath.Glob(filepath.Join("..", "..", "cmd", "*", "FyneApp.toml"))
	if err != nil || len(tomls) < 15 {
		t.Fatalf("sin los FyneApp.toml: %v", err)
	}
	esquema := regexp.MustCompile(`(?m)^\s*esquema\s*=\s*"sytue-([a-z0-9-]+)"`)
	acciones := regexp.MustCompile(`(?m)^\s*widget_acciones\s*=\s*"([^"]*)"`)
	vistas := 0
	for _, ruta := range tomls {
		datos, err := os.ReadFile(ruta)
		if err != nil {
			t.Fatal(err)
		}
		e, a := esquema.FindSubmatch(datos), acciones.FindSubmatch(datos)
		if e == nil || a == nil {
			continue
		}
		producto := string(e[1])
		for _, accion := range strings.Split(string(a[1]), ",") {
			if accion == "open" || accion == "search" {
				continue
			}
			lee, ok := atiende[producto+"/"+accion]
			u, _ := url.Parse(api.Esquema(producto) + ":/?action=" + accion)
			if !ok || !lee(u) {
				t.Errorf("%s declara la acción %q y su app no la atiende", ruta, accion)
			}
			vistas++
		}
	}
	if vistas < 8 {
		t.Fatalf("solo %d acciones declaradas; son ocho (mail, calendar, contacts, notes, photos, docs, sheets, slides)", vistas)
	}
}
