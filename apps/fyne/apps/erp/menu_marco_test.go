// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package erp

import (
	"image/color"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

func TestElMenuERPComparteMarcoSinPerderSuDensidad(t *testing.T) {
	t.Cleanup(tema.FormaDeLosGuardianes)
	tema.Elegir("claro")
	t.Cleanup(func() { tema.Elegir("") })
	tema.ElegirForma("marco")
	m := &Menu{RotuloCerrar: "Cerrar", RotuloPaleta: "Ir a…", NombrePaleta: "Buscar", RotuloIA: "Sytue AI"}
	m.Fijas = []Destino{{Rotulo: "Inicio", Activa: true}}
	o := m.Objeto(tema.Claro).(*fyne.Container)
	o.Resize(fyne.NewSize(menuMedido, 800))
	entrada, ok := buscarEntrada(o, "Inicio").(*Entrada)
	if !ok {
		t.Fatal("falta la entrada de Inicio")
	}
	for _, variante := range []string{"claro", "oscuro", "traslucido"} {
		tema.Elegir(variante)
		v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
		for _, forma := range []string{"marco", "carril", "marco"} {
			tema.ElegirForma(forma)
			entrada.Activa = true
			o.Refresh()
			fondo, divisor := o.Objects[0].(*canvas.Rectangle), o.Objects[1].(*canvas.Rectangle)
			quieroFondo, quieroDivisor := color.NRGBA{}, color.NRGBA{}
			if forma == "carril" {
				quieroFondo = tema.Color("sidebar-bg", v)
				quieroDivisor = ui.BordeSobre("border-subtle", "sidebar-bg", v)
			}
			if fondo.FillColor != quieroFondo || divisor.FillColor != quieroDivisor {
				t.Fatalf("%s %s: el mismo menú conserva fondo/filete %v/%v; quiero %v/%v", forma, variante,
					fondo.FillColor, divisor.FillColor, quieroFondo, quieroDivisor)
			}
			if entrada.Size() != fyne.NewSize(anchoEntrada, entradaMedida) {
				t.Fatal("el adaptador de Marco ha cambiado la geometría del menú ERP")
			}
			r := test.WidgetRenderer(entrada).(*entradaRenderer)
			if r.contorno.Visible() != (forma == "marco") ||
				r.contorno.Position() != fyne.NewPos(0.5, 0.5) ||
				r.contorno.Size() != fyne.NewSize(anchoEntrada-1, entradaMedida-1) {
				t.Fatalf("%s: la selección del ERP no usa el contorno interior de la suite", forma)
			}
			entrada.FocusGained()
			visible := false
			for _, objeto := range r.aro.Objetos() {
				visible = visible || objeto.Visible()
			}
			if !visible || r.aro.Margen() != 2 || r.contorno.Visible() {
				t.Fatal("la selección del ERP sustituye el aro de foco de 2 px")
			}
			entrada.FocusLost()
			entrada.Activa = false
			entrada.Refresh()
			if r.contorno.Visible() {
				t.Fatal("la selección anterior conserva el contorno después de navegar")
			}
		}
	}
}
