// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Command contacts es Sytue Contacts de escritorio y móvil.
//
// Su propio binario, como Calendar: cada producto de la suite se instala por separado
// también aquí (`docs/CONVENTIONS.md` §13.6). Comparten los estados de pantalla
// y el catálogo de `apps/fyne/ui`, no el ejecutable.
// paridad-web: apps/frontend/apps/contacts/src/pages/auth/matrix/callback.astro
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"

	"git.sytue.eu/sytue/one/apps/fyne/cifrado"
	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/fyne/vinculados"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	privateContacts "git.sytue.eu/sytue/one/apps/nucleo/contacts"
	"git.sytue.eu/sytue/one/apps/nucleo/ficheros"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
	mx "git.sytue.eu/sytue/one/apps/nucleo/matrix"

	"git.sytue.eu/sytue/one/apps/fyne/catalogos"
	"git.sytue.eu/sytue/one/apps/fyne/contactos"
	"git.sytue.eu/sytue/one/apps/fyne/sesion"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

func main() {
	a := app.NewWithID("eu.sytue.contacts")
	a.Settings().SetTheme(tema.Sytue{})

	// La sesión, la apariencia y el idioma son TRANSVERSALES a las trece apps: esto lo
	// lee de donde lo dejó la última que lo tocó. `Abrir` no habla con la red.
	s := sesion.Abrir(a, "")
	// El cromo «papel» de toda la suite (ADR 0080). Aquí y no en `sesion.Abrir`: las
	// pruebas que abren sesión comprueban también el cromo de color.
	tema.ElegirCromo(tema.CromoPapel)

	t, err := i18n.New(s.Idioma(), i18n.Common(), catalogos.Contacts())
	if err != nil {
		panic(err)
	}

	// El widget nativo lo lee su propia Fuente, SIN la caché de trabajar sin conexión:
	// lo que enseña tiene que ser lo que hay en el servicio.
	s.PublicadorWidget(s.FuenteWidget(t))
	// El nombre del producto (FyneApp.toml), como las demás: «Contacts» a secas no decía
	// de qué suite era la ventana.
	w := a.NewWindow(a.Metadata().Name)
	w.SetPadded(false)
	w.Resize(fyne.NewSize(1280, 800))
	var current *contactos.App
	var pendingID string
	// pendingNuevo: la acción «Nuevo contacto» del widget llegó sin vista montada.
	var pendingNuevo bool
	// mount monta la vista sobre la cuenta que toque: la API del servicio, como la web,
	// o los datos ya migrados a Matrix (ADR 0086, «clientes nativos»).
	mount := func(resources cifrado.Cuenta, facade *api.Contacts) cifrado.ApplicationView {
		c := contactos.NuevaApp(t, facade)
		subject := resources.PrivateStorageScope().Subject
		c.Directorio, c.Yo = api.NewDirectorio(s.URL("id"), resources.HTTP(), nil), func() string { return subject }
		c.Apex = s.Apex()
		c.Almacen = s.PorPersona() // el orden de la libreta, por persona
		organization := resources.PrivateStorageScope().Tenant
		c.Org = func() string { return organization }
		if runtime.GOOS != "js" {
			c.ElegirFichero = elegirFichero
		}
		c.ConServiciosVinculados(vinculados.Nuevo(s, w, t, "contacts").Abrir)
		// Private contacts are never sent to the server-side AI/import endpoint.
		// The verified repository is the only personal data client in this view.
		current = c
		c.IniciarCarga()
		if pendingID != "" {
			id := pendingID
			pendingID = ""
			c.SolicitarContacto(id)
		}
		if pendingNuevo {
			pendingNuevo = false
			c.PedirNuevo()
		}
		return cifrado.ApplicationView{Object: c.Objeto(), Detach: func() {
			c.Cerrar()
			if current == c {
				current = nil
			}
		}, Join: c.Esperar}
	}
	private, err := cifrado.NuevaAplicacionRegistros(cifrado.AplicacionRegistrosConfig[*privateContacts.Repository]{
		Matrix: cifrado.PrivateApplicationConfig[*privateContacts.Repository]{
			Suite: s, Application: a, Window: w, Product: "contacts", Translate: func(key string) string { return t.T(key, nil) },
			Build: func(ctx context.Context, resources *cifrado.Resources[*mx.Client]) (cifrado.ApplicationPlan[*privateContacts.Repository], error) {
				return contactsPlan(ctx, resources, s)
			},
			Mount: func(_ context.Context, resources *cifrado.Resources[*mx.Client], repository *privateContacts.Repository) (cifrado.ApplicationView, error) {
				facade, err := api.NewContacts(s.URL("contacts"), resources.HTTP(), nil).WithPrivateData(repository)
				if err != nil {
					return cifrado.ApplicationView{}, err
				}
				// El widget lee este repositorio mientras esté montado: el servidor de esta
				// cuenta está congelado desde que pasó a Matrix (ADR 0078). Vive lo que la cuenta
				// montada, no esta petición de montaje.
				//nolint:contextcheck // la vida es la de la cuenta (resources.Context).
				s.WidgetDesdeRegistrosPrivados(resources.Context(), resources.HTTP(), facade)
				return mount(resources, facade), nil //nolint:contextcheck // This view retains its own presenter/resource lifetime; future gestures must not inherit the mount request.
			},
		},
		Soltar: cifrado.SoltarEn(s.URL("contacts")),
		Modo:   contactsMode(s),
		API: func(cuenta cifrado.Cuenta) (cifrado.ApplicationView, error) {
			return mount(cuenta, api.NewContacts(s.URL("contacts"), cuenta.HTTP(), s.ClienteConAviso(nil))), nil
		},
	})
	if err != nil {
		w.SetContent(ui.NuevoVacioError(t.T("matrix.failed", nil), ""))
	} else {
		w.SetContent(ui.NuevaBarraSuite(t, s, iconos.Contacts).Montar(private.Objeto()))
		// Las letras de la web (n, j, k, /, e, s, x, ?). Llegan sin nada enfocado o desde
		// la fila de la lista, que las deja subir.
		w.Canvas().SetOnTypedRune(func(r rune) {
			if current != nil {
				current.Atajo(r)
			}
		})
		s.RutaPropia(func(u *url.URL) bool { return contactos.IDDeEnlace(u) != "" })
		s.AlAbrirURL(func(u *url.URL) {
			// «Nuevo contacto» del widget: el formulario de crear, que se guarda a mano.
			if api.LeerAccion("contacts", u) == "create" {
				fyne.Do(func() {
					if current != nil {
						current.PedirNuevo()
					} else {
						pendingNuevo = true
					}
				})
				return
			}
			if id := contactos.IDDeEnlace(u); id != "" {
				fyne.Do(func() {
					if current != nil {
						current.SolicitarContacto(id)
					} else {
						pendingID = id
					}
				})
			}
		})
		w.SetOnClosed(private.Close)
		// Con interfaz: lanzada por iOS solo para el widget, ni se carga ni se abre el
		// navegador para entrar.
		s.CuandoHayaEscena(func() {
			private.Start()
		})
	}
	s.Arrancar(context.Background())
	w.ShowAndRun()
}

// elegirFichero es App.ElegirFichero con el selector del sistema: se queda con el primer
// fichero elegido de esas extensiones —el `accept` del `<input type="file">` de la web—
// y le pasa su contenido a `listo` en el hilo de la interfaz. Cancelar no hace nada.
func elegirFichero(extensiones []string, listo func(datos []byte)) {
	go func() {
		rutas, err := ficheros.Elegir()
		if err != nil {
			if !errors.Is(err, ficheros.ErrNoDisponible) {
				slog.Warn("contacts: el selector de ficheros ha fallado", "error", err)
			}
			return
		}
		for _, r := range rutas {
			if !slices.Contains(extensiones, strings.ToLower(filepath.Ext(r))) {
				continue
			}
			datos, err := os.ReadFile(r)
			if err != nil {
				slog.Warn("contacts: no se puede leer el fichero elegido", "error", err)
				return
			}
			fyne.Do(func() { listo(datos) })
			return
		}
	}()
}
