// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rooms

import (
	"image/color"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

func TestLasMencionesTraslucidasOcultanLosMensajes(t *testing.T) {
	tema.Elegir("traslucido")
	t.Cleanup(func() { tema.Elegir("") })
	lista := NuevaListaSugerencias(traductor(t))
	lista.Items = []Sugerencia{{Tipo: SugierePersona, Nombre: "Ana"}, {Tipo: SugierePersona, Nombre: "Luis"}}
	lista.Resize(fyne.NewSize(320, 100))
	debajo := canvas.NewRectangle(color.NRGBA{R: 255, A: 255})
	debajo.Resize(lista.Size())
	img := pinta(t, 320, 100, debajo, lista)
	if got, want := px(img, int(300*escala), int(90*escala)), tema.Color("menu-surface", tema.Translucido); !cerca(got, want) {
		t.Errorf("los mensajes se mezclan con las menciones: %v; el papel opaco es %v", got, want)
	}
	if got, want := px(img, int(300*escala), int(12*escala)), tema.Color("menu-selected", tema.Translucido); !cerca(got, want) {
		t.Errorf("la mención elegida pinta %v; el resaltado del menú es %v", got, want)
	}
}

func TestLaListaLargaDeSugerenciasEmpiezaEnLaPrimeraOpcion(t *testing.T) {
	lista := NuevaListaSugerencias(traductor(t))
	for range 10 {
		lista.Items = append(lista.Items, Sugerencia{Tipo: SugierePersona, Nombre: "Ana"})
	}
	// El contenedor consulta primero MinSize y solo después entrega el viewport.
	_ = lista.MinSize()
	lista.Resize(fyne.NewSize(320, MedidaListaSugerencias(len(lista.Items))))
	_ = pinta(t, lista.Size().Width, lista.Size().Height, lista)
	if got := lista.scroll.Offset.Y; got != 0 {
		t.Errorf("la lista oculta la primera opción al abrirse: desplazamiento %v", got)
	}
	lista.Mover(len(lista.Items) - 1)
	y := float32(lista.Elegida) * AltoFilaSugerencia()
	if arriba, abajo := lista.scroll.Offset.Y, lista.scroll.Offset.Y+lista.scroll.Size().Height; y < arriba || y+AltoFilaSugerencia() > abajo {
		t.Errorf("la opción elegida [%v, %v] queda fuera del viewport [%v, %v]", y, y+AltoFilaSugerencia(), arriba, abajo)
	}
}

func TestLaBarraDeMensajeTraslucidaOcultaElHistorial(t *testing.T) {
	tema.Elegir("traslucido")
	t.Cleanup(func() { tema.Elegir("") })
	barra := NuevaBarraMensaje(traductor(t))
	barra.AlHilo = func() {}
	tam := barra.MinSize()
	barra.Resize(tam)
	for _, fondo := range []color.NRGBA{{R: 255, A: 255}, {B: 255, A: 255}} {
		debajo := canvas.NewRectangle(fondo)
		debajo.Resize(tam)
		barra.Refresh()
		img := pinta(t, tam.Width, tam.Height, debajo, barra)
		// El centro del relleno superior queda fuera del glifo y de las esquinas.
		if got, want := px(img, int(tam.Width/2*escala), int(2*escala)), tema.Color("menu-surface", tema.Translucido); !cerca(got, want) {
			t.Errorf("el historial %v se mezcla con las acciones: %v; toca %v", fondo, got, want)
		}
	}
}
