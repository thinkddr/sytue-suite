// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Command calendar es Sytue Calendar de escritorio y móvil.
//
// Es SU PROPIO BINARIO, no una pantalla de una app que las contenga a todas: cada
// producto de la suite se instala por separado también aquí (`docs/CONVENTIONS.md`
// §13.6), y las apps se llaman entre sí por el esquema de URL que declara su
// `FyneApp.toml`.
//
// Para VERLA hace falta una máquina con pantalla. Lo que corre en cualquier sitio, sin
// CGO y sin servidor gráfico, es `go test ./calendario/...`, que pinta lo mismo con el
// pintor de software y comprueba los píxeles contra los números de producción.
package main

import (
	"context"
	"net/url"
	"slices"
	"time"
	_ "time/tzdata" // el selector de zona del evento, también donde el sistema no las trae

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/fyne/vinculados"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"

	"git.sytue.eu/sytue/one/apps/fyne/calendario"
	"git.sytue.eu/sytue/one/apps/fyne/catalogos"
	"git.sytue.eu/sytue/one/apps/fyne/sesion"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

func main() {
	a := app.NewWithID("eu.sytue.calendar")
	a.Settings().SetTheme(tema.Sytue{})

	// La sesión, la apariencia y el idioma son TRANSVERSALES a las trece apps: esto lo
	// lee de donde lo dejó la última que lo tocó. `Abrir` no habla con la red.
	s := sesion.Abrir(a, "")

	// El idioma sale del SISTEMA operativo, no de una cabecera HTTP: aquí no hay
	// middleware que lo negocie. Lo que no esté traducido cae al inglés, que es lo
	// que hace `i18n.New` cargando siempre el catálogo de respaldo.
	t, err := i18n.New(s.Idioma(), i18n.Common(), catalogos.Calendar())
	if err != nil {
		// Un catálogo embebido roto es un fallo de compilación disfrazado de error de
		// ejecución: no hay nada que la persona pueda hacer y tampoco hay pantalla en
		// la que decírselo.
		panic(err)
	}

	p := &calendario.Pantalla{T: t, Cursor: time.Now(), Vista: calendario.VistaMes}
	// La App es quien habla con el servicio; la Pantalla solo pinta. El cliente va con
	// la sesión compartida de las trece apps: sin sesión, las cargas fallan y la
	// pantalla lo dice con su estado de error, y al entrar se vuelve a pedir todo.
	c := calendario.NuevaApp(p, api.NewCalendars(s.URL("calendar"), s.Sesion, nil))
	c.Widget = s.CapturarWidget
	// La videollamada de un evento es una sala de Sytue Rooms ligada a su UID.
	salas := api.NewRooms(s.URL("rooms"), s.Sesion, nil)
	c.Salas = func(ctx context.Context, uid, titulo string) (string, error) {
		return salas.SalaDeEvento(ctx, uid, titulo, s.URL("rooms"))
	}
	// El asistente del formulario propone un evento a partir de una frase.
	c.Asistente = api.NewIA(s.URL("ai"), s.Sesion, nil).Asistir
	// Las notas de Notepads que cuelgan de un evento, en su vista rápida. Se abren en
	// la app de Notepads o, si no está instalada, en su web.
	notas := api.NewNotes(s.URL("notes"), s.Sesion, nil)
	c.Notas = func(ctx context.Context, uid string) ([]api.Note, error) {
		return notas.LinkedNotes(ctx, "calendar_event", uid, 20)
	}
	c.AbrirNotas = func(ruta string) { _ = api.AbrirEn(a, s.Apex(), "notes", s.Sesion.Organizacion(), ruta) }
	c.Apex = s.Apex()
	c.ConImportar(api.NewIntegraciones(s.URL("integrations"), s.Sesion, nil))
	c.AbrirEnlace = func(enlace string) {
		u, err := url.Parse(enlace)
		if err != nil {
			return
		}
		// «Unirse» a una reunión de la suite la abre en la app de Rooms si está (su
		// esquema), y si no en la web; un enlace de otro proveedor, en el navegador.
		if _, esReunion := api.LeerReunion(s.Apex(), u); esReunion {
			_ = api.AbrirURL(a, "rooms", enlace)
			return
		}
		_ = api.AbrirFuera(a, enlace)
	}
	w := a.NewWindow(t.T("page.title", nil))
	w.SetPadded(false)
	c.ConServiciosVinculados(vinculados.Nuevo(s, w, t, "calendar").Abrir)
	w.SetContent(ui.NuevaBarraSuite(t, s, iconos.Calendar).Montar(c.Objeto()))
	// Los atajos de una letra llegan al lienzo solo cuando NADA tiene el foco: un campo
	// o un diálogo abierto se quedan su tecla, que es lo que hace `esAtajoDeApp`.
	w.Canvas().SetOnTypedRune(func(r rune) {
		if len(w.Canvas().Overlays().List()) == 0 {
			c.Atajo(r)
		}
	})
	w.Resize(fyne.NewSize(1280, 800))
	// `sytue-calendar:?action=create&…` (crear desde un correo o desde una ficha de
	// contacto), `?date=…` y `/events/<id>` (un evento del buscador de la suite).
	// Llega también antes de arrancar el bucle (la URL de la línea de órdenes): va al
	// hilo de la interfaz, que es donde se abre el formulario.
	s.AlAbrirURL(func(u *url.URL) { fyne.Do(func() { c.AtenderEnlace(u) }) })
	// Al ENTRAR —aquí o en otra de las trece apps— se vuelve a pedir todo. Solo cuando
	// cambia quién ha entrado: `AlCambiar` avisa también del tema, del idioma y de la
	// presencia, y recargar el calendario por cambiar a oscuro no tiene sentido.
	quien := s.Estado().Sujeto
	// El correo de quien ha entrado dice qué eventos son invitaciones que contestar.
	cuenta := api.NewAccount(s.URL("id"), s.Sesion, nil)
	pedirCorreo := func() {
		go func() {
			ctx, parar := context.WithTimeout(context.Background(), 15*time.Second)
			defer parar()
			if yo, err := cuenta.Me(ctx); err == nil {
				fyne.Do(func() { c.Correo = yo.Email })
			}
			// Los roles del token dicen si sale «Gestionar salas y recursos»
			// (`user.roles` con admin u owner en la web). Quien autoriza es el servicio.
			if r, err := s.Sesion.Reclamos(ctx); err == nil {
				fyne.Do(func() { c.PonerGestorRecursos(slices.Contains(r.Roles, "admin") || slices.Contains(r.Roles, "owner")) })
			}
		}()
	}
	s.AlCambiar(func(e sesion.Estado) {
		if e.Sujeto != quien {
			quien = e.Sujeto
			if e.Dentro() {
				c.Arrancar()
				pedirCorreo()
			}
		}
	})
	c.Arrancar()
	pedirCorreo()
	c.Reloj()
	s.Arrancar(context.Background())
	w.ShowAndRun()
}
