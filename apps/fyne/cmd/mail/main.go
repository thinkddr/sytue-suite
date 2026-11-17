// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Command mail abre Sytue Mail en una ventana de verdad.
//
// Es SU PROPIA aplicación, como en la web es su propio subdominio: un binario
// instalable por producto (`docs/CONVENTIONS.md` §13.6). Ir a otro producto de la
// suite no es una pantalla de aquí, es invocar a su aplicación por el esquema de URL
// que declara su `FyneApp.toml`.
//
// Para VERLA hace falta una máquina con pantalla; el piloto no la tiene. Lo que sí
// corre en cualquier sitio, sin CGO y sin servidor gráfico, es
// `go test ./apps/mail/...`, que pinta lo mismo con el pintor de software y compara
// los píxeles contra lo medido en producción.
// paridad-web: apps/frontend/apps/mail/src/pages/auth/matrix/callback.astro
package main

import (
	"context"
	"net/url"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"

	"git.sytue.eu/sytue/one/apps/fyne/apps/mail"
	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/sesion"
	"git.sytue.eu/sytue/one/apps/fyne/soltar"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

func main() {
	a := app.New()
	a.Settings().SetTheme(tema.Sytue{})

	// La sesión, la apariencia y el idioma son TRANSVERSALES a las trece apps: esto lo
	// lee de donde lo dejó la última que lo tocó. `Abrir` no habla con la red.
	s := sesion.Abrir(a, "")
	// El cromo «papel» de toda la suite (ADR 0080). Aquí y no en `sesion.Abrir`: las
	// pruebas que abren sesión comprueban también el cromo de color.
	tema.ElegirCromo(tema.CromoPapel)

	// El idioma lo decide el SISTEMA, igual que en la web lo decide el middleware:
	// nunca se detecta dentro de una vista. Uno que la suite no sirva cae al inglés.
	l := s.Idioma()
	t, err := mail.Traductor(l)
	if err != nil {
		panic(err) // el catálogo va embebido: si falla, es un error de compilación
	}

	// El widget nativo lo lee su propia Fuente (siempre Recibidos) y se vuelve a
	// publicar tras cada escritura de Mail y con lo que traiga el vivo del buzón.
	s.PublicadorWidget(s.FuenteWidget(t))
	// El icono lleva los no leídos de la Entrada, no los avisos de correo del Hub
	// (que eran uno y salía siempre un «1»). Antes de `s.AlCambiar`: el primer árbol
	// llega en cuanto hay sesión.
	barra := ui.NuevaBarraSuite(t, s, iconos.Mail)
	ctx := context.Background()
	identidad := ""
	// Los enlaces entrantes esperan al montaje del buzón privado del mismo usuario.
	var pendiente *api.Redactar
	var hiloPendiente, buzonPendiente string
	// El título de la ventana es el NOMBRE DEL PRODUCTO y sale de `FyneApp.toml`, no
	// del catálogo: una marca no se traduce y no tiene clave en la web.
	nombre := "Sytue Mail"
	// `Metadata()` devuelve un VALOR (`fyne.AppMetadata`), no un puntero: compararlo
	// con nil no compila. Estaba así y nadie se enteró porque el piloto no puede
	// construir los `cmd/` —le faltan las bibliotecas gráficas— y el CI no dispara en
	// push. Se ve con `GOOS=js GOARCH=wasm go build ./cmd/...`, que sí type-chequea
	// sin CGO (no es un destino que sostengamos: es la forma barata de mirar).
	if m := fyne.CurrentApp().Metadata(); m.Name != "" {
		nombre = m.Name
	}
	w := a.NewWindow(nombre)
	w.SetPadded(false)
	host, err := newMailStorage(s, a, w, t)
	if err != nil {
		w.SetContent(ui.NuevoVacio(t.T("matrix.failed", nil), ""))
	} else {
		w.SetContent(barra.Montar(host.application.Objeto()))
		w.SetOnClosed(host.application.Close)
		s.AlFondo(func() {
			if box := host.Current(identidad); box != nil {
				box.EnSegundoPlano()
			}
		})
		s.AlVolver(func() {
			if box := host.Current(identidad); box != nil {
				box.EnPrimerPlano()
			}
		})
		// El texto o el enlace soltado sobre el redactor entra en el campo donde cae.
		soltar.AlSoltar(w, func(x soltar.Soltado) {
			if box := host.Current(identidad); box != nil {
				box.Soltar(x.ComoTexto(), x.Posicion)
			}
		})
	}
	procesarPendientes := func(box *mail.Buzon) {
		if pendiente != nil {
			box.RedactarDesde(*pendiente)
			pendiente = nil
		}
		if hiloPendiente != "" {
			box.AbrirHilo(hiloPendiente)
			hiloPendiente = ""
		}
		if buzonPendiente != "" {
			box.AtenderBuzon(buzonPendiente)
			buzonPendiente = ""
		}
	}
	if host != nil {
		host.onMount = func(box *mail.Buzon) {
			box.AlCambiarNoLeidos = barra.PonerInsignia
			box.Atajos(w.Canvas())
			if identidad != "" {
				procesarPendientes(box)
			}
		}
	}
	arrancar := func(e sesion.Estado) {
		actual := ""
		if e.Dentro() {
			actual = s.URL("id") + "\x00" + e.Sujeto + "\x00" + s.Sesion.Organizacion()
		}
		if actual == identidad {
			return
		}
		identidad = actual
		if actual != "" && host != nil {
			if box := host.Current(actual); box != nil {
				procesarPendientes(box)
			}
		}
	}
	s.AlAbrirURL(func(u *url.URL) {
		// El aviso de un correo nuevo abre su conversación (`hubnotify.EnlaceHilo`).
		if h, ok := api.LeerHilo(u); ok {
			fyne.Do(func() {
				if host != nil {
					if box := host.Current(identidad); box != nil {
						box.AbrirHilo(h)
						return
					}
				}
				hiloPendiente = h
			})
			return
		}
		// El de una petición de acceso abre el buzón compartido (`hubnotify.EnlaceBuzon`).
		if c, ok := api.LeerBuzon(u); ok {
			fyne.Do(func() {
				if host != nil {
					if box := host.Current(identidad); box != nil {
						box.AtenderBuzon(c)
						return
					}
				}
				buzonPendiente = c
			})
			return
		}
		r, ok := api.LeerRedactar(u)
		switch {
		case ok && host != nil && host.Current(identidad) != nil:
			box := host.Current(identidad)
			fyne.Do(func() { box.RedactarDesde(r) })
		case ok:
			pendiente = &r
		case strings.Trim(u.Path, "/") != "" || strings.Trim(u.Opaque, "/") != "":
			// Con un receptor apuntado, la sesión ya no sabe qué enlaces no son de
			// nadie: una ruta que Mail no atiende es «no encontrada», como antes.
			s.NoEncontrado()
		}
	})
	w.Resize(fyne.NewSize(1280, 744))
	s.AlCambiar(arrancar)
	// Con interfaz: lanzada por iOS solo para el widget, ni buzón ni el navegador para
	// entrar.
	s.CuandoHayaEscena(func() {
		if host != nil {
			host.application.Start()
		}
		if s.Estado().Dentro() {
			arrancar(s.Estado())
		} else {
			go func() { _ = s.Entrar(ctx) }() // BLOQUEA hasta volver del navegador
		}
	})
	s.Arrancar(ctx)
	w.ShowAndRun()
}
