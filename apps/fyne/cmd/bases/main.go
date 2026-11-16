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
// paridad-web: apps/frontend/apps/bases/src/pages/auth/matrix/callback.astro
package main

import (
	"context"
	"io"
	"net/url"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"

	"git.sytue.eu/sytue/one/apps/fyne/apps/bases"
	"git.sytue.eu/sytue/one/apps/fyne/cifrado"
	"git.sytue.eu/sytue/one/apps/fyne/editor"
	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/sesion"
	"git.sytue.eu/sytue/one/apps/fyne/soltar"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	privateBases "git.sytue.eu/sytue/one/apps/nucleo/bases"
	"git.sytue.eu/sytue/one/apps/nucleo/ficheros"
	mx "git.sytue.eu/sytue/one/apps/nucleo/matrix"
)

func main() {
	a := app.New()
	a.Settings().SetTheme(tema.Sytue{})

	// La sesión, la apariencia y el idioma son TRANSVERSALES a las apps de la suite: esto
	// lo lee de donde lo dejó la última que lo tocó. `Abrir` no habla con la red.
	s := sesion.Abrir(a, "")
	// El cromo «papel» de toda la suite (ADR 0080). Aquí y no en `sesion.Abrir`: las
	// pruebas que abren sesión comprueban también el cromo de color.
	tema.ElegirCromo(tema.CromoPapel)
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

	// El widget nativo lo lee su propia Fuente, sin depender de la pantalla, y se
	// vuelve a publicar tras cada escritura del cliente de arriba (ClienteConAviso).
	s.PublicadorWidget(s.FuenteWidget(t))

	var current *bases.Pantalla
	var pendingBase, pendingTabla, pendingRegistro string

	private, err := cifrado.NewPrivateApplication(cifrado.PrivateApplicationConfig[*privateBases.Repository]{
		Suite:       s,
		Application: a,
		Window:      w,
		Product:     "bases",
		Translate:   func(key string) string { return t.T(key, nil) },
		Build: func(ctx context.Context, resources *cifrado.Resources[*mx.Client]) (cifrado.ApplicationPlan[*privateBases.Repository], error) {
			return basesPlan(ctx, resources, s)
		},
		//nolint:contextcheck // The presenter retains its own mounted lifetime, not this short mount request.
		Mount: func(_ context.Context, resources *cifrado.Resources[*mx.Client], repository *privateBases.Repository) (cifrado.ApplicationView, error) {
			facade, err := api.NewBases(s.URL("bases"), resources.HTTP(), nil).WithPrivateData(repository)
			if err != nil {
				return cifrado.ApplicationView{}, err
			}
			p := bases.NuevaPantalla(t, facade, w)
			organization := resources.PrivateStorageScope().Tenant
			p.Org = func() string { return organization }
			// Quien mira, para contar «Tú» en la actividad de la base.
			subject := resources.PrivateStorageScope().Subject
			p.Yo = func() string { return subject }
			p.URLPublica = func(ruta string) string { return api.AppURL(s.Apex(), "bases", "", ruta) }
			p.Guardar = func(nombre string, r io.Reader) (string, error) {
				carpeta, err := ficheros.Carpeta()
				if err != nil {
					return "", err
				}
				return ficheros.Guardar(carpeta, nombre, r)
			}
			p.Elegir = editor.ElegirDelSistema
			current = p
			p.Arrancar()
			if pendingBase != "" {
				id, tabla, registro := pendingBase, pendingTabla, pendingRegistro
				pendingBase, pendingTabla, pendingRegistro = "", "", ""
				p.AbrirEnlace(id, tabla, registro)
			}
			return cifrado.ApplicationView{
				Object: p.Objeto(),
				Detach: func() {
					if current == p {
						current = nil
					}
				},
			}, nil
		},
	})

	// Adjuntos e importar un CSV: el selector del sistema (en el móvil, el que pone la
	// sesión) o soltar los ficheros sobre la ventana. El texto soltado (un bloque copiado
	// de una hoja de cálculo) se pega en la celda sobre la que cae.
	w.SetOnDropped(func(pos fyne.Position, uris []fyne.URI) {
		if current != nil {
			current.Soltado(pos, uris)
		}
	})
	soltar.AlArrastrar(w, func(activa bool) {
		if current != nil {
			current.Arrastrando(activa)
		}
	})
	soltar.AlSoltar(w, func(x soltar.Soltado) {
		if current != nil {
			current.SoltarTexto(x.Texto, x.Posicion)
		}
	})

	// Un enlace a una base llega como `sytue-bases:/bases/base/<id>` (o su dirección
	// web): se abre aquí. Lo que la app no sabe enseñar es «no encontrado».
	s.AlAbrirURL(func(u *url.URL) {
		base, ok := bases.DestinoDeEnlace(u)
		switch {
		case !ok:
			s.NoEncontrado()
		case base != "":
			tabla, registro := bases.RegistroDeEnlace(u)
			fyne.Do(func() {
				if current != nil {
					current.AbrirEnlace(base, tabla, registro)
				} else {
					pendingBase, pendingTabla, pendingRegistro = base, tabla, registro
				}
			})
		}
	})

	if err != nil {
		w.SetContent(ui.NuevoVacio(t.T("matrix.failed", nil), ""))
	} else {
		w.SetContent(ui.NuevaBarraSuite(t, s, iconos.Bases).Montar(private.Objeto()))
		w.SetOnClosed(private.Close)
		// Con interfaz: lanzada por iOS solo para el widget no se pide la lista.
		s.CuandoHayaEscena(func() {
			private.Start()
		})
	}
	s.Arrancar(context.Background())
	w.ShowAndRun()
}
