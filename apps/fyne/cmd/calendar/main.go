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
// paridad-web: apps/frontend/apps/calendar/src/pages/auth/matrix/callback.astro
package main

import (
	"context"
	"net/url"
	"slices"
	"time"
	_ "time/tzdata" // el selector de zona del evento, también donde el sistema no las trae

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"

	"git.sytue.eu/sytue/one/apps/fyne/cifrado"
	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/fyne/vinculados"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	privateCalendar "git.sytue.eu/sytue/one/apps/nucleo/calendar"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
	mx "git.sytue.eu/sytue/one/apps/nucleo/matrix"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"

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
	// El cromo «papel» de toda la suite (ADR 0080). Aquí y no en `sesion.Abrir`: las
	// pruebas que abren sesión comprueban también el cromo de color.
	tema.ElegirCromo(tema.CromoPapel)

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

	// El widget nativo lo lee su propia Fuente: hoy y lo próximo de TODOS los
	// calendarios, mire la pantalla el mes que mire, y se vuelve a publicar tras cada
	// escritura del cliente de arriba (ClienteConAviso).
	s.PublicadorWidget(s.FuenteWidget(t))
	dev := a.Driver().Device()
	ventanaInicial := calendario.VentanaInicial(dev.IsMobile(), fyne.IsVertical(dev.Orientation()))

	w := a.NewWindow(t.T("page.title", nil))
	w.SetPadded(false)
	w.Resize(fyne.NewSize(1280, 800))

	var current *calendario.App
	enlaces := &enlacesIdentidad{
		despachar: fyne.Do,
		abrir: func(u *url.URL) {
			if current != nil {
				current.AtenderEnlace(u)
			}
		},
	}
	s.AlAbrirURL(enlaces.recibir)

	// mount monta la vista sobre la cuenta que toque: la API del servicio, como la web,
	// o los datos ya migrados a Matrix (ADR 0086, «clientes nativos»).
	mount := func(resources cifrado.Cuenta, facade *api.Calendars) cifrado.ApplicationView {
		p := &calendario.Pantalla{T: t, Cursor: time.Now(), Vista: calendario.VistaMes, Ventana: ventanaInicial}
		c := calendario.NuevaAppConVida(resources.Context(), p, facade)
		salas := api.NewRooms(s.URL("rooms"), resources.HTTP(), nil)
		c.Salas = func(ctx context.Context, uid, titulo string) (string, error) {
			return salas.SalaDeEvento(ctx, uid, titulo, s.URL("rooms"))
		}
		organization := resources.PrivateStorageScope().Tenant
		c.AbrirNotas = func(ruta string) { _ = api.AbrirEn(a, s.Apex(), "notes", organization, ruta) }
		c.Apex = s.Apex()
		c.AbrirEnlace = func(enlace string) {
			u, err := url.Parse(enlace)
			if err != nil {
				return
			}
			if _, esReunion := api.LeerReunion(s.Apex(), u); esReunion {
				_ = api.AbrirURL(a, "rooms", enlace)
				return
			}
			_ = api.AbrirFuera(a, enlace)
		}
		c.ConServiciosVinculados(vinculados.Nuevo(s, w, t, "calendar").Abrir)
		cuenta := api.NewAccount(s.URL("id"), resources.HTTP(), nil)
		vida, parar := context.WithCancel(resources.Context())
		vigente := func() bool { return resources.Check(vida) == nil && current == c }
		pedirDatosCuenta(vida, vigente, func(ctx context.Context) (string, error) {
			yo, err := cuenta.Me(ctx)
			return yo.Email, err
		}, func(ctx context.Context) (bool, error) {
			r, err := s.Sesion.Reclamos(ctx)
			return slices.Contains(r.Roles, "admin") || slices.Contains(r.Roles, "owner"), err
		}, fyne.Do, func(correo string) { c.Correo = correo }, c.PonerGestorRecursos)
		current = c
		c.Arrancar()
		c.Reloj()
		enlaces.activar(vida, vigente)
		return cifrado.ApplicationView{
			Object: c.Objeto(),
			Detach: func() {
				parar()
				enlaces.retirar()
				c.Cerrar()
				if current == c {
					current = nil
				}
			},
			Join: c.Join,
		}
	}

	private, err := cifrado.NuevaAplicacionRegistros(cifrado.AplicacionRegistrosConfig[*privateCalendar.Repository]{
		Matrix: cifrado.PrivateApplicationConfig[*privateCalendar.Repository]{
			Suite: s, Application: a, Window: w, Product: "calendar",
			Translate: func(key string) string { return t.T(key, nil) },
			Build: func(ctx context.Context, resources *cifrado.Resources[*mx.Client]) (cifrado.ApplicationPlan[*privateCalendar.Repository], error) {
				return calendarPlan(ctx, resources, s)
			},
			Mount: func(_ context.Context, resources *cifrado.Resources[*mx.Client], repository *privateCalendar.Repository) (cifrado.ApplicationView, error) {
				facade, err := api.NewCalendars(s.URL("calendar"), resources.HTTP(), nil).WithPrivateData(repository)
				if err != nil {
					return cifrado.ApplicationView{}, err
				}
				return mount(resources, facade), nil //nolint:contextcheck // Presenter retains its mounted resource lifetime; future UI gestures must not inherit the mount request.
			},
		},
		Soltar: cifrado.SoltarEn(s.URL("calendar")),
		Modo: func(ctx context.Context, session api.Session, scope storage.Scope) (string, error) {
			state, err := api.NewCalendars(s.URL("calendar"), session, nil).PrivateStorageState(ctx, scope)
			return state.Mode, err
		},
		API: func(cuenta cifrado.Cuenta) (cifrado.ApplicationView, error) {
			return mount(cuenta, api.NewCalendars(s.URL("calendar"), cuenta.HTTP(), nil)), nil
		},
	})
	if err != nil {
		w.SetContent(ui.NuevoVacio(t.T("matrix.failed", nil), ""))
	} else {
		w.SetContent(ui.NuevaBarraSuite(t, s, iconos.Calendar).Montar(private.Objeto()))
		w.Canvas().SetOnTypedRune(func(r rune) {
			if len(w.Canvas().Overlays().List()) == 0 && current != nil {
				current.Atajo(r)
			}
		})
		w.SetOnClosed(func() {
			enlaces.cerrar()
			private.Close()
		})
		// Con interfaz: lanzada por iOS solo para el widget no se pide nada ni se sondean los
		// recordatorios. El widget lo lee su Fuente.
		s.CuandoHayaEscena(func() {
			private.Start()
		})
	}
	s.Arrancar(context.Background())
	w.ShowAndRun()
}
