// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

// TestLaFilaCopiaMideLoQueDiceMedir es el guardián de `boton_test.go` para el
// `CopyField`: el alto sale de la web (`py-2` + borde + el botón `sm`, que es lo más
// alto de una fila de una línea), `MinSize` es esa misma cuenta y lo PINTADO también.
func TestLaFilaCopiaMideLoQueDiceMedir(t *testing.T) {
	f := NuevaFilaCopia("Servidor IMAP", "imap.acme.eu", nil)
	const ancho = 600
	boton := MedidaBoton(textoComun("ui.copy.copy"), TallaSM, 0).Height
	quiero := 2*(tema.Borde+MargenCopiaY) + max(boton, tema.TextoXS.Linea)
	if got := f.AltoPara(ancho); got != quiero {
		t.Errorf("AltoPara(%v) = %v; `border py-2` con el botón sm son %v", ancho, got, quiero)
	}
	f.Resize(fyne.NewSize(ancho, quiero))
	if got := test.WidgetRenderer(f).MinSize().Height; got != quiero {
		t.Errorf("MinSize() da %v de alto y AltoPara %v: hay dos cuentas donde tiene que haber una", got, quiero)
	}
	img := pinta(t, ancho+20, quiero+20, f)
	x0, x1, y0, y1 := caja(img, tema.Color("border", tema.Claro))
	mismaCaja(t, "la fila de copiar", fyne.NewSize(ancho, quiero), x0, x1, y0, y1)

	// El rótulo tiene el ancho de `sm:w-52` (o `sm:w-24` con labelWidth="sm") y el
	// valor arranca detrás, con su `gap-2`.
	if r, _, bajada := f.geometria(ancho); r != RotuloCopiaSM || bajada != 0 {
		t.Errorf("ancha, el rótulo mide %v (quiero %v) y el valor baja %v", r, RotuloCopiaSM, bajada)
	}
	f.RotuloSM = RotuloCopiaCorto
	if r, _, _ := f.geometria(ancho); r != RotuloCopiaCorto {
		t.Errorf("con RotuloCopiaCorto el rótulo mide %v", r)
	}
}

// TestLaFilaCopiaEstrechaBajaElValor: por debajo de `sm:` el rótulo es `w-full` y el
// valor va en la línea de debajo, así que la fila crece en una línea de rótulo y un
// hueco.
func TestLaFilaCopiaEstrechaBajaElValor(t *testing.T) {
	f := NuevaFilaCopia("Servidor", "imap.acme.eu", nil)
	ancha := f.AltoPara(600)
	f.Ventana = 360 // la ventana decide el `sm:`; el ancho de la fila, no
	estrecha := f.AltoPara(600)
	if quiero := ancha + tema.TextoXS.Linea + HuecoCopia; estrecha != quiero {
		t.Errorf("estrecha mide %v; quiero %v (una línea de rótulo y el gap-2 más)", estrecha, quiero)
	}
}

// TestLaFilaCopiaDiceCopiadoSinCambiarDeAncho: pulsar copia el valor, el botón dice
// «Copiado» un momento y vuelve; el ancho del botón es el mismo en los dos estados. Sin
// valor, una raya y el botón inerte.
func TestLaFilaCopiaDiceCopiadoSinCambiarDeAncho(t *testing.T) {
	var copiado string
	f := NuevaFilaCopia("Valor", "v=spf1 -all", func(s string) { copiado = s })
	f.Aviso = 20 * time.Millisecond
	f.Resize(fyne.NewSize(600, f.AltoPara(600)))
	test.WidgetRenderer(f).Layout(f.Size())
	antes := f.Boton().Size()

	test.Tap(f.Boton())
	if copiado != "v=spf1 -all" || !f.Copiado() {
		t.Fatalf("«Copiar» manda %q y el botón copiado=%v", copiado, f.Copiado())
	}
	if f.Boton().Texto != textoComun("ui.copy.copied") {
		t.Errorf("tras copiar el botón dice %q", f.Boton().Texto)
	}
	if got := f.Boton().Size(); got != antes {
		t.Errorf("el botón pasa de %v a %v al copiar: la fila salta", antes, got)
	}

	limite := time.Now().Add(2 * time.Second)
	for f.Copiado() && time.Now().Before(limite) {
		time.Sleep(5 * time.Millisecond)
	}
	fyne.DoAndWait(func() {})
	if f.Copiado() || f.Boton().Texto != textoComun("ui.copy.copy") {
		t.Errorf("pasado el aviso el botón sigue diciendo %q", f.Boton().Texto)
	}

	vacia := NuevaFilaCopia("Valor", "", func(string) { t.Error("una fila sin valor no copia") })
	test.WidgetRenderer(vacia)
	if !vacia.Boton().Inerte || vacia.valorVisible() != "—" {
		t.Errorf("sin valor: inerte=%v y se escribe %q", vacia.Boton().Inerte, vacia.valorVisible())
	}
	test.Tap(vacia.Boton())
}
