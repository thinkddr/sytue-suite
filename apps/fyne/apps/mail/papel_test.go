// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package mail

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// sobrePapel pone el cromo «papel» en Marco y lo deshace al acabar.
func sobrePapel(t *testing.T) {
	t.Helper()
	forma := "carril"
	if tema.EsMarco() {
		forma = "marco"
	}
	tema.ElegirForma("marco")
	tema.ElegirCromo(tema.CromoPapel)
	t.Cleanup(func() {
		tema.ElegirCromo("")
		tema.ElegirForma(forma)
	})
}

// TestElLectorPintaSuFondoEnLaHoja: sobre papel la hoja es blanca y el lienzo gris; el
// lector, vacío o con el hilo abierto, va en la hoja (`main` sin fondo en AppShell).
//
// VISTO EN ROJO (2026-11-06): el fondo era `bg`, el gris del lienzo, dentro de la hoja
// blanca: el hilo y «Selecciona una conversación» salían sobre una mancha gris.
func TestElLectorPintaSuFondoEnLaHoja(t *testing.T) {
	sobrePapel(t)
	v := tema.Claro
	if tema.Color("sheet-bg", v) == tema.Color("bg", v) {
		t.Fatal("sobre papel la hoja y el lienzo son del mismo color: la prueba no distingue nada")
	}
	for _, abierto := range []bool{false, true} {
		x := NuevoLector(traductor(t), "es")
		x.Ahora = reloj()
		if abierto {
			x.Estado, x.Conversacion = Listo, ejemplo()[0]
		}
		w := test.NewWindow(x)
		w.SetPadded(false)
		w.Resize(fyne.NewSize(900, 600))
		w.Canvas().Capture()
		r := test.WidgetRenderer(x).(*lectorRenderer)
		if got, want := r.fondo.FillColor, tema.Color("sheet-bg", v); got != want {
			t.Errorf("abierto=%v: el fondo del lector es %v y la hoja %v", abierto, got, want)
		}
		w.Close()
	}
}

// glifoFantasma comprueba que el glifo de un botón fantasma va en la tinta de su rótulo
// (`text-primary-text`, el `currentColor` del `ghost` de Button.tsx).
func glifoFantasma(t *testing.T, donde string, b *ui.Boton) {
	t.Helper()
	if b.Glifo == nil {
		t.Fatalf("%s: «%s» no lleva glifo", donde, b.Rotulo)
	}
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	if got, want := b.Glifo.Tinta(), tema.Color("primary-text", v); got != want {
		t.Errorf("%s: el glifo de «%s» es %v y su rótulo %v", donde, b.Rotulo, got, want)
	}
}

// TestLosGlifosDeLosFantasmasVanEnLaTintaDelRotulo: en la web el glifo de un `ghost` es
// `currentColor`, el del rótulo. En la cabecera del lector, el pie de cada mensaje, la
// tira de triaje de la fila, la barra de selección y el pie del redactor.
//
// VISTO EN ROJO (2026-11-06): los glifos de Mail iban en `text` (casi negro) y el rótulo,
// el galón y los «···» en `primary-text`: «Etiquetar» salía con la etiqueta negra y el
// rótulo y el galón verdes, y la barra mezclaba glifos negros con los «···» verdes. Las
// flechas de «Responder a todos» y «Reenviar» salían negras sobre el relleno de marca.
// Y «Programar envío» abierto no cambiaba de fondo.
func TestLosGlifosDeLosFantasmasVanEnLaTintaDelRotulo(t *testing.T) {
	tr := traductor(t)
	// Cabecera del lector y pie del mensaje: `botonCabecera` sin token es el del rótulo.
	for _, c := range []struct{ clave, d string }{
		{"action.label", EtiquetaD},
		{"action.moveTo", CarpetaD},
		{"action.archive", ArchivarD},
		{"reader.reply", ResponderD},
		{"reader.unsubscribe", BajaD},
	} {
		glifoFantasma(t, "cabecera", botonCabecera(tr.T(c.clave, nil), c.d, ui.Fantasma, "", false, false, iconos.Ninguno, nil))
	}
	// Y el secundario (relleno de marca, rótulo `on-primary`): el glifo, igual que el rótulo.
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	todos := botonCabecera(tr.T("reader.replyAll", nil), ResponderTodosD, ui.Secundario, "", false, false, iconos.Ninguno, nil)
	if got, want := todos.Glifo.Tinta(), tema.Color("on-primary", v); got != want {
		t.Errorf("cabecera: el glifo de «%s» es %v sobre el relleno de marca y su rótulo %v", todos.Rotulo, got, want)
	}
	// La estrella de la cabecera, sin destacar, sale de `grupoTriaje` con su token.
	x := NuevoLector(tr, "es")
	x.Estado, x.Conversacion = Listo, ejemplo()[0]
	nada := func(Conversacion) {}
	x.Acciones = &Acciones{Destacar: nada, Archivar: nada, Papelera: nada}
	for _, o := range x.grupoTriaje(false).Objects {
		if bt, ok := o.(*ui.Boton); ok && bt.Glifo != nil {
			glifoFantasma(t, "triaje del lector", bt)
		}
	}
	glifoFantasma(t, "barra de selección", botonLote(tr.T("action.archive", nil), ArchivarD, nil))

	b, _ := bandejaConTriaje(t, nil)
	f := primeraFila(t, b)
	r := test.WidgetRenderer(f).(*filaRenderer)
	for _, bt := range r.triaje {
		if bt.token == "warning-text" {
			continue // la estrella de un destacado va en su ámbar, también en la web
		}
		glifoFantasma(t, "fila", &bt.Boton)
	}

	s := servicioConTodas()
	bz, w, _ := buzonConVentana(t, s, fyne.NewSize(1280, 744))
	bz.abrirRedactor(Borrador{Asunto: "Hola"})
	glifoFantasma(t, "redactor", bz.redaccion.descartar)
	// Formato y programar son `BotonIcono`, que tiñe el glifo con su `Trazo`.
	vistos := 0
	var visitar func(fyne.CanvasObject)
	visitar = func(o fyne.CanvasObject) {
		if bi, ok := o.(*ui.BotonIcono); ok && (bi.Rotulo == tr.T("compose.schedule", nil) || bi.Rotulo == tr.T("compose.formatting", nil)) {
			vistos++
			if bi.Trazo != "primary-text" {
				t.Errorf("redactor: «%s» lleva el glifo en %q y sus vecinos en primary-text", bi.Rotulo, bi.Trazo)
			}
			// Programar abierto es `bg-selected text-text` (ComposeModal.tsx).
			if bi.Rotulo == tr.T("compose.schedule", nil) && bi.FondoPulsado != "selected" {
				t.Errorf("redactor: «%s» abierto no se distingue (FondoPulsado %q)", bi.Rotulo, bi.FondoPulsado)
			}
		}
		for _, h := range hijos(o) {
			visitar(h)
		}
	}
	visitar(w.Canvas().Overlays().Top())
	if vistos != 2 {
		t.Fatalf("en el pie del redactor hay %d de los dos conmutadores (formato y programar)", vistos)
	}
}
