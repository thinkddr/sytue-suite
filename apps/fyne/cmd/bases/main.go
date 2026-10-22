// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Command bases abre Sytue Bases —las tablas relacionales colaborativas del ADR 0068—
// en una ventana de verdad.
//
// Es SU PROPIA aplicación, como en la web es su propio subdominio: un binario
// instalable por producto (`docs/CONVENTIONS.md` §13.6). Habla con services/bases en
// directo (`bases.<dominio>/api/v1`, publicado para esto el 2026-10-05) y con el token
// de la sesión transversal, sin pasar por el BFF de la web.
//
// Para VERLA hace falta una máquina con pantalla. Lo que corre en cualquier sitio, sin
// servidor gráfico, es `go test ./apps/bases/...`, que pinta las mismas pantallas con el
// pintor de software.
package main

import (
	"context"
	"io"
	"net/url"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"

	"git.sytue.eu/sytue/one/apps/fyne/apps/bases"
	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/sesion"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/ficheros"
)

func main() {
	a := app.New()
	a.Settings().SetTheme(tema.Sytue{})

	// La sesión, la apariencia y el idioma son TRANSVERSALES a las apps de la suite: esto
	// lo lee de donde lo dejó la última que lo tocó. `Abrir` no habla con la red.
	s := sesion.Abrir(a, "")
	t, err := bases.Traductor(s.Idioma())
	if err != nil {
		panic(err) // el catálogo va embebido: si falla, es un error de compilación
	}

	// El título de la ventana es el NOMBRE DEL PRODUCTO y sale de `FyneApp.toml`, no del
	// catálogo: una marca no se traduce.
	nombre := "Sytue Bases"
	if m := fyne.CurrentApp().Metadata(); m.Name != "" {
		nombre = m.Name
	}
	w := a.NewWindow(nombre)
	w.SetPadded(false)
	w.Resize(fyne.NewSize(1280, 800))

	p := bases.NuevaPantalla(t, api.NewBases(s.URL("bases"), s.Sesion, nil), w)
	// «Toda la organización» al compartir es la de la sesión.
	p.Org = s.Sesion.Organizacion
	// El enlace de un formulario público es de la WEB (lo abre quien no tiene cuenta):
	// host y prefijo, y nunca la organización.
	p.URLPublica = func(ruta string) string { return api.AppURL(s.Apex(), "bases", "", ruta) }
	// Exportar deja el fichero donde el sistema guarda las descargas.
	p.Guardar = func(nombre string, r io.Reader) (string, error) {
		carpeta, err := ficheros.Carpeta()
		if err != nil {
			return "", err
		}
		return ficheros.Guardar(carpeta, nombre, r)
	}

	// Sin nadie dentro, la barra enseña «Iniciar sesión»; al entrar —aquí o en otra app
	// de la suite— se pide la lista.
	dentro := s.Estado().Dentro()
	s.AlCambiar(func(e sesion.Estado) {
		if e.Dentro() && !dentro {
			p.Arrancar()
		}
		dentro = e.Dentro()
	})

	// Un enlace a una base llega como `sytue-bases:/bases/base/<id>` (o su dirección
	// web): se abre aquí. Lo que la app no sabe enseñar es «no encontrado».
	s.AlAbrirURL(func(u *url.URL) {
		base, ok := bases.DestinoDeEnlace(u)
		switch {
		case !ok:
			s.NoEncontrado()
		case base != "":
			fyne.Do(func() { p.Abrir(base) })
		}
	})

	w.SetContent(ui.NuevaBarraSuite(t, s, iconos.Bases).Montar(p.Objeto()))
	if dentro {
		p.Arrancar()
	}
	s.Arrancar(context.Background())
	w.ShowAndRun()
}
