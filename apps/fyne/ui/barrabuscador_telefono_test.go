// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// traductorPt es el catálogo común en portugués: su marca de agua es la más larga de las
// que se midieron en la web («Pesquisar no Sytue One…»).
func traductorPt(t *testing.T) *i18n.Translator {
	t.Helper()
	tr, err := i18n.New("pt", i18n.Common())
	if err != nil {
		t.Fatal(err)
	}
	return tr
}

// TestElBuscadorDelTelefonoOcupaLaFilaAlEscribir es el `data-buscando` de TopBar.tsx: en
// la cara de la suite del teléfono, con el foco en el buscador, la campana (con su punto)
// y la cuenta se apartan y el campo va del ✕ al canalón, con sitio para su marca de agua
// en los idiomas largos; al perder el foco vuelven. VISTO EN ROJO sin el bloque de
// `layoutTelefono` que lo alarga: el campo se quedaba en lo que dejaban la campana y la
// cuenta y «Pesquisar no Sytue One…» no cabía a 320.
func TestElBuscadorDelTelefonoOcupaLaFilaAlEscribir(t *testing.T) {
	defer TactilDePrueba(true)()
	for _, ancho := range []float32{320, 390} {
		b, parar := BarraSuiteDePrueba(traductorPt(t), iconos.Mail)
		w := test.NewWindow(b.Montar(container.NewWithoutLayout()))
		w.SetPadded(false)
		w.Resize(fyne.NewSize(ancho, 700))
		test.Tap(b.cuenta.boton) // la entrada despliega la suite
		if !b.suiteAbierta {
			t.Fatalf("a %v la entrada no despliega la suite", ancho)
		}
		campo := b.buscador.campo
		enReposo := campo.Size().Width

		w.Canvas().Focus(campo)
		conFoco := campo.Size().Width
		x := b.cerrarSuite.Position().X + b.cerrarSuite.Size().Width
		hasta := ancho - canalonBarraUnica
		t.Logf("a %v el campo mide %v en reposo y %v con el foco", ancho, enReposo, conFoco)
		if campo.Position().X <= x || campo.Position().X+conFoco != hasta {
			t.Errorf("a %v, con el foco el campo va de %v a %v: quiero del ✕ (%v) al canalón (%v)",
				ancho, campo.Position().X, campo.Position().X+conFoco, x, hasta)
		}
		for nombre, o := range map[string]fyne.CanvasObject{
			"la campana": b.avisos.boton, "su punto": b.avisos.punto, "la cuenta": b.cuenta.boton,
		} {
			if o.Visible() {
				t.Errorf("a %v, con el foco en el buscador %s sigue a la vista y le quita sitio", ancho, nombre)
			}
		}
		// La marca de agua más larga de las medidas en la web cabe entera.
		marca := Medir(campo.marca, tema.TextoBase, Normal).Width
		if util := conFoco - 2*AltoCampo(CampoMD); marca > util {
			t.Errorf("a %v, «%s» pide %v y el campo deja %v", ancho, campo.marca, marca, util)
		}

		// Al perder el foco, la cara de la suite de siempre.
		w.Canvas().Unfocus()
		if campo.Size().Width != enReposo || !b.avisos.boton.Visible() || !b.cuenta.boton.Visible() {
			t.Errorf("a %v, sin foco el campo mide %v (quiero %v) con campana=%v y cuenta=%v",
				ancho, campo.Size().Width, enReposo, b.avisos.boton.Visible(), b.cuenta.boton.Visible())
		}
		w.Close()
		parar()
	}
}
