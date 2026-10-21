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
package main

import (
	"context"
	"net/url"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"

	"git.sytue.eu/sytue/one/apps/fyne/apps/hub"
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

	// El idioma lo decide el SISTEMA, igual que en la web lo decide el middleware:
	// nunca se detecta dentro de una vista. Uno que la suite no sirva cae al inglés.
	l := s.Idioma()
	t, err := mail.Traductor(l)
	if err != nil {
		panic(err) // el catálogo va embebido: si falla, es un error de compilación
	}

	p := mail.NuevaPantalla(t, l)
	// Los datos: el mismo servicio que la web, con la sesión transversal. Se arranca
	// en cuanto hay alguien dentro; sin sesión, se abre Sytue ID en el navegador del
	// sistema, que es lo que hace el middleware de la web al llegar sin cookie.
	contactos := api.NewContacts(s.URL("contacts"), s.Sesion, nil)
	b := mail.NuevoBuzon(p, api.NewMail(s.URL("mail"), s.Sesion, nil)).
		ConHub(api.NewHub(s.URL("hub"), s.Sesion, nil), a).
		ConDrive(api.NewDrive(s.URL("drive"), s.Sesion, nil)).
		ConContactos(contactos).
		ConLibreta(contactos, s.URL("contacts"))
	b.Abridor = a
	b.Widget = s.CapturarWidget
	ctx := context.Background()
	arrancado := false
	// «Adjuntar a un correo» de Drive, y el correo que deja escrito el ERP, llegan por
	// el esquema con `action=compose` (`api.LeerRedactar`). Si llegan antes de que haya
	// alguien dentro —se abrió la app pulsando el enlace—, esperan a que lo haya.
	var pendiente *api.Redactar
	arrancar := func() {
		arrancado = true
		b.Arrancar(ctx)
		if pendiente != nil {
			//nolint:contextcheck // RedactarDesde usa el contexto de vida del buzón.
			b.RedactarDesde(*pendiente)
			pendiente = nil
		}
	}
	s.AlCambiar(func(e sesion.Estado) {
		if e.Dentro() && !arrancado {
			arrancar()
		}
	})
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
	w.SetContent(ui.NuevaBarraSuite(t, s, iconos.Mail).Montar(p.Objeto()))
	b.Atajos(w.Canvas())
	// El texto o el enlace soltado sobre el redactor entra en el campo donde cae.
	soltar.AlSoltar(w, func(x soltar.Soltado) { b.Soltar(x.ComoTexto(), x.Posicion) })
	// El enlace de un adjunto de Drive lleva el prefijo de la app (`/drive/files/…`):
	// sin él, producción contesta 404 (medido el 2026-09-23). SIN organización a
	// propósito (ADR 0056): el enlace viaja DENTRO de un correo y quien lo pulsa puede
	// no ser de la organización, igual que `driveFileUrl` en la web.
	b.URLCuenta, b.URLDrive = s.URL("id"), api.AppURL(s.Apex(), "drive", "", "")
	// «Volver a iniciar sesión» del selector de Drive (sesión caducada).
	b.Entrar = s.Entrar
	// El asistente del redactor, con Sytue AI en directo (el `/api/ai` de la web).
	b.Asistir = api.NewIA(s.URL("ai"), s.Sesion, nil).Asistir
	// «Trae tus datos», en el pie del lateral (si la organización tiene conectado
	// Google o Microsoft 365).
	gateway := api.NewIntegraciones(s.URL("integrations"), s.Sesion, nil)
	b.ConCorreoExterno(gateway, func(ctx context.Context, provider, instance string, services ...string) (string, error) {
		return s.AutorizarCorreo(ctx, gateway, provider, instance, services...)
	})
	b.ConImportar(gateway, func() {
		_ = api.AbrirFuera(a, hub.PoliticaDePrivacidad(s.Apex(), l))
	})
	s.AlAbrirURL(func(u *url.URL) {
		// El aviso de un correo nuevo abre su conversación (`hubnotify.EnlaceHilo`).
		if h, ok := api.LeerHilo(u); ok {
			fyne.Do(func() { b.AbrirHilo(h) })
			return
		}
		// El de una petición de acceso abre el buzón compartido (`hubnotify.EnlaceBuzon`).
		if c, ok := api.LeerBuzon(u); ok {
			fyne.Do(func() { b.AtenderBuzon(c) })
			return
		}
		r, ok := api.LeerRedactar(u)
		switch {
		case ok && arrancado:
			b.RedactarDesde(r)
		case ok:
			pendiente = &r
		case strings.Trim(u.Path, "/") != "" || strings.Trim(u.Opaque, "/") != "":
			// Con un receptor apuntado, la sesión ya no sabe qué enlaces no son de
			// nadie: una ruta que Mail no atiende es «no encontrada», como antes.
			s.NoEncontrado()
		}
	})
	w.Resize(fyne.NewSize(1280, 744))
	s.Arrancar(ctx)
	if s.Estado().Dentro() {
		arrancar()
	} else {
		go func() { _ = s.Entrar(ctx) }() // BLOQUEA hasta volver del navegador
	}
	w.SetOnClosed(b.Parar)
	w.ShowAndRun()
}
