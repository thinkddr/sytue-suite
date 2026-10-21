// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package contactos

import (
	"os"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/nucleo/api"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// TestCapturasDeLasPantallas deja en SYTUE_CAPTURAS las pantallas de esta tanda para
// mirarlas al lado de las de producción (`test/e2e/medidas-web.mjs contacts`). Sin la
// variable no hace nada: no es un guardián, es la cámara.
func TestCapturasDeLasPantallas(t *testing.T) {
	if os.Getenv("SYTUE_CAPTURAS") == "" {
		t.Skip("sin SYTUE_CAPTURAS")
	}
	muestra := pantallaDeMuestra(t)
	hoy := time.Now()
	for i, dias := range map[int]int{0: 0, 2: 5, 4: 40} {
		f := time.Date(1990, hoy.Month(), hoy.Day()+dias, 0, 0, 0, 0, time.UTC)
		muestra.Contactos[i].Birthday = &f
	}
	foto, err := ui.AvatarPredefinido("Héctor Lamas", tema.Color("avatar-3", tema.Claro))
	if err != nil {
		t.Fatal(err)
	}
	muestra.Contactos[1].HasPhoto = true
	s := &servicioFalso{contactos: muestra.Contactos, fotos: map[string][]byte{muestra.Contactos[1].UID: foto}, ia: "{}"}
	a := appDePrueba(t, s)
	a.Cargar()
	s.correrCola()

	// El teléfono, a 390 como la captura de producción (sin la topbar de la web): la
	// lista, el cajón abierto y la ficha.
	var w fyne.Window
	for _, x := range fyne.CurrentApp().Driver().AllWindows() {
		if x.Canvas() == a.lienzo() {
			w = x
		}
	}
	w.Resize(fyne.NewSize(390, 788))
	guardar(t, a.lienzo().Capture(), "contactos-telefono")
	a.Pantalla.barraMovil(armazonDe(t, a)).AlMenu()
	a.repintar()
	guardar(t, a.lienzo().Capture(), "contactos-telefono-cajon")
	a.Pantalla.CajonAbierto = false
	a.Pantalla.AlElegirContacto(a.Pantalla.Contactos[0])
	guardar(t, a.lienzo().Capture(), "contactos-telefono-ficha")
	a.Pantalla.AlVolver()

	w.Resize(fyne.NewSize(ventana, altoUtil))
	a.Pantalla.AlMarcar(a.Pantalla.Contactos[0].ID)
	a.Pantalla.AlMarcar(a.Pantalla.Contactos[1].ID)
	guardar(t, a.lienzo().Capture(), "contactos-lote")
	a.Pantalla.AlSoltarMarcados()

	a.dialogoEtiqueta(nil)
	guardar(t, a.lienzo().Capture(), "contactos-nueva-etiqueta")
	a.lienzo().Overlays().Top().(interface{ Descartar() }).Descartar()

	a.abrirFormulario(nil)
	guardar(t, a.lienzo().Capture(), "contactos-nuevo")
	a.lienzo().Overlays().Top().(interface{ Descartar() }).Descartar()

	c := a.Pantalla.Contactos[0]
	f := a.abrirFusion(c, a.candidatosDe(c))
	guardar(t, a.lienzo().Capture(), "contactos-combinar-1")
	otro := a.Pantalla.Contactos[1]
	f.perdedor = &otro
	f.pintar()
	guardar(t, a.lienzo().Capture(), "contactos-combinar-2")
	f.d.Cerrar()

	a.verPapelera()
	guardar(t, a.lienzo().Capture(), "contactos-papelera")
	a.lienzo().Overlays().Top().(interface{ Descartar() }).Descartar()

	a.verCumples()
	guardar(t, a.lienzo().Capture(), "contactos-cumpleanos")
	a.lienzo().Overlays().Top().(interface{ Descartar() }).Descartar()

	// La ficha a 1280, con su AvatarPicker y el menú de la cámara abierto.
	c = a.Pantalla.Contactos[1]
	c.Organization, c.Phones = "Analytical Engines", []api.ContactPhone{{Phone: "+34 600 11 22 33", Type: "cell"}}
	a.todos[1] = c
	a.elegir(c)
	guardar(t, a.lienzo().Capture(), "contactos-ficha")
	for _, o := range fichaObjetos(a) {
		if s, es := o.(*ui.SelectorAvatar); es {
			s.Tapped(nil)
		}
	}
	guardar(t, a.lienzo().Capture(), "contactos-foto-menu")
}

// fichaObjetos recorre el árbol montado buscando lo que haya.
func fichaObjetos(a *App) []fyne.CanvasObject {
	var fuera []fyne.CanvasObject
	var andar func(o fyne.CanvasObject)
	andar = func(o fyne.CanvasObject) {
		fuera = append(fuera, o)
		switch x := o.(type) {
		case *fyne.Container:
			for _, h := range x.Objects {
				andar(h)
			}
		case *container.Scroll:
			andar(x.Content)
		case fyne.Widget:
			if _, es := x.(*ui.SelectorAvatar); es {
				return
			}
			for _, h := range test.WidgetRenderer(x).Objects() {
				andar(h)
			}
		}
	}
	andar(a.raiz)
	return fuera
}
