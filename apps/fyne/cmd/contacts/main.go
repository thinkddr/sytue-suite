// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Command contacts es Sytue Contacts de escritorio y móvil.
//
// Su propio binario, como Calendar: cada producto de la suite se instala por separado
// también aquí (`docs/CONVENTIONS.md` §13.6). Comparten el paquete `apps/fyne/agenda`
// y el catálogo de `apps/fyne/ui`, no el ejecutable.
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

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/fyne/vinculados"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/ficheros"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"

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

	t, err := i18n.New(s.Idioma(), i18n.Common(), catalogos.Contacts())
	if err != nil {
		panic(err)
	}

	// Sin conexión, solo lectura: la caché «red primero» y su monitor son de la sesión.
	// «Exportar tus datos» y el CSV son descargas, no listados, y no se guardan.
	red, cliente := s.Conexion("contacts", "/api/v1/export")
	c := contactos.NuevaApp(t, api.NewContacts(s.URL("contacts"), s.Sesion, cliente))
	c.Widget = s.CapturarWidget
	c.Conexion(red)
	// La libreta «Organización» sale de la agenda de Sytue ID. Sin la caché: es una
	// sincronización, no un listado que haya que ver sin red.
	c.Directorio, c.Yo = api.NewDirectorio(s.URL("id"), s.Sesion, nil), s.Sesion.Subject
	// «Rellenar» del formulario: Sytue AI, sin la caché (el asistente es una
	// conversación, no un dato; la web tampoco guarda `/api/ai`).
	c.IA = api.NewIA(s.URL("ai"), s.Sesion, nil)
	// «Trae tus contactos», en el pie del menú (si la organización tiene conectado
	// Google o Microsoft 365).
	c.ConImportar(api.NewIntegraciones(s.URL("integrations"), s.Sesion, nil))
	c.Apex = s.Apex()
	c.Org = s.Sesion.Organizacion
	// «Importar» (vCard y CSV) y «Subir una foto…» abren el selector del sistema.
	switch runtime.GOOS {
	case "js":
	default:
		c.ElegirFichero = elegirFichero
	}
	// El nombre del producto (FyneApp.toml), como las demás: «Contacts» a secas no decía
	// de qué suite era la ventana.
	w := a.NewWindow(a.Metadata().Name)
	w.SetPadded(false)
	c.ConServiciosVinculados(vinculados.Nuevo(s, w, t, "contacts").Abrir)
	w.SetContent(ui.NuevaBarraSuite(t, s, iconos.Contacts).Montar(c.Objeto()))
	w.Resize(fyne.NewSize(1280, 800))
	// La «n» crea un contacto, como en la web. Solo llega sin nada enfocado.
	w.Canvas().SetOnTypedRune(c.Atajo)
	// `sytue-contacts:/contacts/{id}` —un resultado del buscador de la suite, un intent
	// del Hub— abre la ficha de ese contacto en su libreta.
	s.RutaPropia(func(u *url.URL) bool { return contactos.IDDeEnlace(u) != "" })
	s.AlAbrirURL(func(u *url.URL) {
		if id := contactos.IDDeEnlace(u); id != "" {
			go c.AbrirContacto(id)
		}
	})

	// La libreta se carga al entrar, y otra vez si se vuelve a entrar desde otra de
	// las trece apps. Sin sesión se abre Sytue ID, que es lo que hace la web: su
	// página redirige al login antes de pintar nada.
	identidad := func(e sesion.Estado) string {
		if !e.Dentro() {
			return ""
		}
		return s.URL("id") + "\x00" + e.Sujeto + "\x00" + s.Sesion.Organizacion()
	}
	actual := identidad(s.Estado())
	s.AlCambiar(func(e sesion.Estado) {
		siguiente := identidad(e)
		if siguiente == actual {
			return
		}
		actual = siguiente
		c.Vaciar()
		if siguiente != "" {
			go c.Cargar()
		}
	})
	if actual != "" {
		go c.Cargar()
	} else {
		go func() { _ = s.Entrar(context.Background()) }()
	}
	w.SetOnClosed(c.Cerrar)
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
