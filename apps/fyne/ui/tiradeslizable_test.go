// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"image/color"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
)

// TestLaTiraDeslizableAlcanzaLoQueNoCabe: seis botones de 21 en 100 px se recortan y
// se alcanzan con el gesto horizontal (hasta el final y no más), con el foco —que la
// corre hasta el botón enfocado— y con el arrastre; la rueda vertical se la pasa a la
// lista si hay quien la quiera.
func TestLaTiraDeslizableAlcanzaLoQueNoCabe(t *testing.T) {
	bs := make([]fyne.CanvasObject, 0, 6)
	for range 6 {
		bs = append(bs, NuevoBoton("", nil).Con(Fantasma, TallaXS))
	}
	tira := NuevaTiraDeslizable(21, 1.75, bs...)
	test.WidgetRenderer(tira)
	tira.Resize(fyne.NewSize(100, 21))
	fin := tira.AnchoContenido() - 100
	if fin != 34.75 {
		t.Fatalf("contenido %g", tira.AnchoContenido()+0)
	}

	tira.Scrolled(&fyne.ScrollEvent{Scrolled: fyne.NewDelta(-30, 0)})
	if tira.Desplazamiento() != 30 || bs[0].Position().X != -30 {
		t.Errorf("tras 30 px: desplazada %g, el primero en %g", tira.Desplazamiento(), bs[0].Position().X)
	}
	tira.Scrolled(&fyne.ScrollEvent{Scrolled: fyne.NewDelta(-1000, 0)})
	if tira.Desplazamiento() != fin {
		t.Errorf("se pasa del final: %g, quiero %g", tira.Desplazamiento(), fin)
	}

	bs[0].(*Boton).FocusGained()
	if tira.Desplazamiento() != 0 {
		t.Errorf("el primero enfocado no se ve: desplazada %g", tira.Desplazamiento())
	}
	bs[5].(*Boton).FocusGained()
	if tira.Desplazamiento() != fin {
		t.Errorf("el último enfocado no se ve: desplazada %g", tira.Desplazamiento())
	}
	tira.Dragged(&fyne.DragEvent{Dragged: fyne.NewDelta(20, 0)})
	if tira.Desplazamiento() != fin-20 {
		t.Errorf("el arrastre no la corre: %g", tira.Desplazamiento())
	}

	var vertical float32
	tira.AlDesplazarV = func(e *fyne.ScrollEvent) { vertical += e.Scrolled.DY }
	antes := tira.Desplazamiento()
	tira.Scrolled(&fyne.ScrollEvent{Scrolled: fyne.NewDelta(0, -40)})
	if vertical != -40 || tira.Desplazamiento() != antes {
		t.Errorf("la rueda vertical no llega a la lista: %g, desplazada %g", vertical, tira.Desplazamiento())
	}
}

func TestLaTiraNoRecortaLosBotonesTactiles(t *testing.T) {
	antes := tactilDePrueba
	si := true
	tactilDePrueba = &si
	t.Cleanup(func() { tactilDePrueba = antes })
	for _, propio := range []bool{false, true} {
		acciones := []fyne.CanvasObject{
			NuevoBoton("Hoy", nil).Con(Fantasma, TallaTexto),
			NuevoBoton("Nuevo evento", nil).Con(Secundario, TallaSM),
		}
		tira := NuevaTiraDeslizable(32, 8, acciones...)
		tira.AnchoPropio = propio
		if alto := tira.MinSize().Height; alto < 44 {
			t.Errorf("ancho propio %v: declara %g para un botón táctil de 44", propio, alto)
		}
		r := test.WidgetRenderer(tira)
		tira.Resize(fyne.NewSize(160, tira.MinSize().Height))
		r.Layout(tira.Size())
		for _, a := range acciones {
			if a.Position().Y < 0 || a.Position().Y+a.Size().Height > tira.Size().Height {
				t.Errorf("ancho propio %v: hijo %v/%v fuera de la tira %v", propio, a.Position(), a.Size(), tira.Size())
			}
		}
	}
}

func TestLaTiraRevelaIconosYConservaSusAvisosDeFoco(t *testing.T) {
	uno := NuevoBotonIcono(iconos.Mas, "Añadir", nil)
	dos := NuevoBoton("", nil).Con(Fantasma, TallaSM)
	dos.SoloIcono = true
	tres := NuevoBotonIcono(iconos.Texto, "Formato", nil)
	uno.Lado, tres.Lado = 32, 32
	avisosBoton, avisosIcono := 0, 0
	dos.AlEnfocar = func() { avisosBoton++ }
	tres.AlEnfocar = func() { avisosIcono++ }
	tira := NuevaTiraDeslizable(32, 4, uno, dos, tres)
	tira.AnchoPropio = true
	tira.HuecoEntre = func(anterior, actual fyne.CanvasObject) float32 {
		if anterior == uno && actual == dos {
			return 2
		}
		return 4
	}
	tira.Resize(fyne.NewSize(50, 32))
	test.WidgetRenderer(tira).Layout(tira.Size())
	if ancho := tira.AnchoContenido(); ancho != 102 {
		t.Fatalf("el grupo de formato no conserva sus huecos: %v", ancho)
	}
	tres.FocusGained()
	if avisosIcono != 1 || tira.Desplazamiento() != 52 || tres.Position().X+tres.Size().Width != 50 {
		t.Fatalf("el foco del icono no revela el final o se perdió el aviso: avisos=%d, desplazamiento=%v, icono=%v/%v", avisosIcono, tira.Desplazamiento(), tres.Position(), tres.Size())
	}
	uno.FocusGained()
	if tira.Desplazamiento() != 0 {
		t.Fatal("el primer icono enfocado sigue fuera del borde izquierdo")
	}
	dos.FocusGained()
	if avisosBoton != 1 || dos.Position().X+dos.Size().Width > 50 {
		t.Fatal("conservar el aviso de Boton impide que la tira lo revele")
	}
	tira.Dragged(&fyne.DragEvent{Dragged: fyne.NewDelta(-1000, 0)})
	if tira.Desplazamiento() != 52 {
		t.Fatal("el arrastre no alcanza el final con huecos de grupo")
	}
}

func TestLaTiraDeAnchoPropioConservaElSueloTactilSinEnsancharSeparadores(t *testing.T) {
	for _, tactil := range []bool{false, true} {
		t.Run(map[bool]string{false: "escritorio", true: "tactil"}[tactil], func(t *testing.T) {
			t.Cleanup(TactilDePrueba(tactil))
			uno := NuevoBotonIcono(iconos.Mas, "Añadir", nil)
			dos := NuevoBotonIcono(iconos.Texto, "Formato", nil)
			uno.Lado, dos.Lado = 32, 32
			separador := canvas.NewRectangle(color.NRGBA{})
			separador.SetMinSize(fyne.NewSize(1, 8))
			tira := NuevaTiraDeslizable(32, 4, uno, separador, dos)
			tira.AnchoPropio = true
			lado := float32(32)
			if tactil {
				lado = 44
			}
			if tira.MinSize().Height != lado {
				t.Fatalf("la tira debe declarar el alto de sus mandos: %v, quiero %g", tira.MinSize(), lado)
			}
			tira.Resize(fyne.NewSize(50, lado))
			test.WidgetRenderer(tira).Layout(tira.Size())
			for _, b := range []*BotonIcono{uno, dos} {
				if b.Size() != fyne.NewSize(lado, lado) {
					t.Fatalf("el icono perdió el suelo táctil: %v, quiero %gx%g", b.Size(), lado, lado)
				}
			}
			if separador.Size() != fyne.NewSize(1, 8) {
				t.Fatalf("el suelo táctil ha ensanchado la decoración: %v", separador.Size())
			}
			fin := tira.AnchoContenido() - tira.Size().Width
			dos.FocusGained()
			if tira.Desplazamiento() != fin || dos.Position().X+dos.Size().Width != tira.Size().Width {
				t.Fatal("el foco no deja accesible el último mando con su tamaño real")
			}
			tira.Dragged(&fyne.DragEvent{Dragged: fyne.NewDelta(1e6, 0)})
			if tira.Desplazamiento() != 0 {
				t.Fatal("el gesto no permite volver al primer mando")
			}
		})
	}
}
