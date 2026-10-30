// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Command notes es Sytue Notepads de escritorio y móvil.
//
// Es SU PROPIA aplicación, no una pantalla de un binario que las contenga a todas:
// `docs/CONVENTIONS.md` §13.6 excepción 2. Lo que en la web es ir a otro subdominio,
// aquí es invocar a la otra app por su esquema de URL, que está declarado en
// `FyneApp.toml` y del que la cadena de empaquetado hace las cinco copias que cada
// sistema necesita.
//
// Para VERLA hace falta una máquina con pantalla. Lo que corre en cualquier sitio, sin
// CGO y sin servidor gráfico, es `go test ./apps/notes/...`, que pinta lo mismo con el
// pintor de software y compara los píxeles.
package main

import (
	"context"
	"log/slog"
	"net/url"
	"os"
	"slices"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"

	"git.sytue.eu/sytue/one/apps/fyne/apps/notes"
	"git.sytue.eu/sytue/one/apps/fyne/catalogos"
	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/sesion"
	"git.sytue.eu/sytue/one/apps/fyne/soltar"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/fyne/vinculados"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/auth"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
	"git.sytue.eu/sytue/one/apps/nucleo/sistema/escritorio"
)

// Medidas de la ventana al abrir: el ancho canónico de la suite y un alto que deja ver
// el árbol de cuadernos entero sin desplazar.
const (
	anchoInicial = 1280
	altoInicial  = 800
)

// administraCuadernos liga los roles a la identidad adoptada por esta app. Una
// respuesta de otra organización o de un inicio anterior nunca habilita ajustes.
func administraCuadernos(r auth.Reclamos, generacion string, actual sesion.Estado, org string) bool {
	return actual.Dentro() && generacion != "" && generacion == actual.GeneracionSesion &&
		r.Sub == actual.Sujeto && org != "" && r.Tenant == org && !r.EsCuentaPersonal() &&
		(slices.Contains(r.Roles, "owner") || slices.Contains(r.Roles, "admin"))
}

func main() {
	a := app.New()
	a.Settings().SetTheme(tema.Sytue{})

	// La sesión, la apariencia y el idioma son TRANSVERSALES a las trece apps: esto lo
	// lee de donde lo dejó la última que lo tocó. `Abrir` no habla con la red.
	s := sesion.Abrir(a, "")

	l := s.Idioma()
	tr, err := i18n.New(l, i18n.Common(), catalogos.Notes())
	if err != nil {
		slog.Error("no se pudo cargar el catálogo de idiomas", "error", err)
		os.Exit(1)
	}

	// El nombre del producto es una MARCA y no se traduce: es el mismo en los 26
	// idiomas, igual que en la web.
	w := a.NewWindow("Sytue Notepads")
	w.SetPadded(false)
	// El servicio con la sesión TRANSVERSAL: la misma con la que han entrado las otras
	// doce apps, así que si ya hay sesión no se vuelve a pedir.
	// La caché «red primero» de la suite: sin red se leen los cuadernos y las notas que
	// ya se abrieron. Y el almacén, para la cola de capturas y los borradores.
	// Las exportaciones no se guardan: la de toda la cuenta, por prefijo; la de un
	// cuaderno, con `Cache-Control: no-store` desde el SDK (ExportNotebook).
	red, cliente := s.Conexion("notes", "/api/v1/export")
	p := notes.NuevaPantalla(tr, l, api.NewNotes(s.URL("notes"), s.Sesion, cliente), red, w)
	p.Widget = s.CapturarWidget
	if almacen, err := sesion.Almacen(); err == nil {
		p.Almacen = almacen
	} else {
		slog.Warn("notes: sin almacén, sin red no se captura", "error", err)
	}
	// La organización activa encabeza los enlaces de página (ADR 0056), menos el del
	// cuaderno PÚBLICO (`/p/<token>`): es para quien no tiene sesión, y va sin org como
	// el `/s/<token>` de la web (ShareModal la compone aparte por lo mismo).
	org := s.Sesion.Organizacion
	p.AppURL = func(ruta string) string {
		if strings.HasPrefix(ruta, "/p/") {
			return api.AppURL(s.Apex(), "notes", "", ruta)
		}
		return api.AppURL(s.Apex(), "notes", org(), ruta)
	}
	p.Docs = api.NewDocs(s.URL("docs"), s.Sesion, nil)
	// Compartir un cuaderno o una nota con alguien, y decir quién me compartió una.
	p.Directorio, p.Yo = api.NewDirectorio(s.URL("id"), s.Sesion, nil), s.Sesion.Subject
	// La cola y los borradores van por persona Y organización (claveAlmacen).
	p.Org = org
	p.GeneracionSesion = func() string { return s.Estado().GeneracionSesion }
	// Como en Rooms, los claims se leen fuera del hilo de la interfaz. Su caché solo
	// se consulta y publica en ese hilo, con la generación y organización vigentes.
	var rolesCuadernos auth.Reclamos
	var generacionRoles string
	p.AdminOrg = func() bool {
		return administraCuadernos(rolesCuadernos, generacionRoles, s.Estado(), org())
	}
	// El asistente del editor, con Sytue AI en directo (el `/api/ai` de la web).
	p.Asistir = api.NewIA(s.URL("ai"), s.Sesion, nil).Asistir
	// «Compartir en Rooms» y los ficheros de Drive de la herramienta de enlaces: los
	// servicios de esas apps, con la misma sesión, como el `/api/suite` de la web.
	p.Suite = &api.Elegir{
		Apex:     s.Apex(),
		Org:      s.Sesion.Organizacion,
		Drive:    api.NewDrive(s.URL("drive"), s.Sesion, nil),
		Rooms:    api.NewRooms(s.URL("rooms"), s.Sesion, nil),
		Docs:     p.Docs,
		Calendar: api.NewCalendars(s.URL("calendar"), s.Sesion, nil),
		Notes:    api.NewNotes(s.URL("notes"), s.Sesion, nil),
	}
	p.AbrirCalendar = func(ruta string) { _ = api.AbrirEn(a, s.Apex(), "calendar", org(), ruta) }
	// Abrir un cuaderno cifrado con Touch ID, Face ID o Windows Hello, si el sistema los
	// tiene (decisión del dueño, 2026-09-24): la clave derivada, en el almacén seguro.
	if metodo, _ := escritorio.Biometria(); metodo != "" {
		p.Recordar = &notes.Recordador{
			Metodo: metodo, Motivo: tr.T("shell.lock.reasonNotebook", nil),
			Verificar: escritorio.Verificar,
			Leer:      auth.LeerSecreto, Guardar: auth.GuardarSecreto, Borrar: auth.BorrarSecreto,
		}
	}
	barra := ui.NuevaBarraSuite(tr, s, iconos.Notes)
	p.ConServiciosVinculados(vinculados.Nuevo(s, w, tr, "notes").Abrir)
	barra.CederAtajoSi(p.EnfocarCaptura)
	w.SetContent(barra.Montar(p.Objeto()))
	// Soltar texto o un enlace sobre el cuaderno crea la nota, como en la web.
	soltar.AlSoltar(w, func(x soltar.Soltado) { p.Soltar(x.Texto, x.URLs, x.Posicion) })
	w.Resize(fyne.NewSize(anchoInicial, altoInicial))

	// Los enlaces entrantes: `?notebook=&note=&h=` y `/notebooks/<id>` abren esa nota, y
	// `?link=<tipo>:<id>&label=` crea la nota vinculada, como `pages/index.astro`.
	s.AlAbrirURL(func(u *url.URL) {
		fyne.Do(func() {
			if !p.AtenderEnlace(u) && (strings.Trim(u.Path, "/") != "" || strings.Trim(u.Opaque, "/") != "") {
				s.NoEncontrado() // con un receptor apuntado, la sesión ya no sabe qué no es de nadie
			}
		})
	})
	// Se carga en cuanto HAY sesión, y no antes: sin ella cada petición sería un 401 y
	// la pantalla se llenaría de «tu sesión ha caducado» a quien todavía no ha entrado.
	// Si entra —aquí o en otra app de la suite—, se carga entonces.
	generacion, sujeto, organizacion, dentro := "", "", "", false
	arrancar := func(e sesion.Estado) {
		orgActual := org()
		cambio := e.GeneracionSesion != generacion || e.Sujeto != sujeto || orgActual != organizacion
		if cambio || !e.Dentro() || !dentro {
			rolesCuadernos, generacionRoles = auth.Reclamos{}, ""
			p.ReiniciarSesion()
		}
		if e.Dentro() && (!dentro || cambio) {
			p.Arrancar()
			go func(adoptada sesion.Estado, orgAdoptada string) {
				ctx, cancelar := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancelar()
				r, err := s.Sesion.Reclamos(ctx)
				if err != nil {
					return
				}
				fyne.Do(func() {
					actual := s.Estado()
					if actual.GeneracionSesion == adoptada.GeneracionSesion &&
						actual.Sujeto == adoptada.Sujeto && org() == orgAdoptada {
						rolesCuadernos, generacionRoles = r, adoptada.GeneracionSesion
					}
				})
			}(e, orgActual)
		}
		generacion, sujeto, organizacion, dentro = e.GeneracionSesion, e.Sujeto, orgActual, e.Dentro()
	}
	s.AlCambiar(arrancar)
	arrancar(s.Estado())
	s.Arrancar(context.Background())
	w.ShowAndRun()
}
