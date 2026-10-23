// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"fmt"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

func TestAvisoPersistenteNoTapaConfirmacion(t *testing.T) {
	for _, caso := range []struct{ ancho, alto float32 }{{320, 800}, {1280, 800}, {320, 240}} {
		t.Run(fmt.Sprintf("%vx%v", caso.ancho, caso.alto), func(t *testing.T) {
			ancho := caso.ancho
			pila := pilaQueSeVacia(t)
			w := test.NewTempWindow(t, container.NewStack(canvas.NewRectangle(tema.Color("bg", tema.Claro)), pila))
			w.SetPadded(false)
			w.Resize(fyne.NewSize(ancho, caso.alto))
			confirmadas := 0
			d := Confirmacion("Borrar chat", "Cancelar", "Borrar chat", true, NuevoParrafo("¿Borrar el chat con Alice? Sólo se borra para ti. Los demás lo conservan.", tema.TextoBase, Normal, "text", tema.Claro), nil, func() { confirmadas++ })
			d.Mostrar(w.Canvas())
			t.Cleanup(d.Cerrar)
			n := 1
			if ancho > 400 {
				n = 5
			}
			for i := range n {
				pila.Notificar(NuevoAviso(AvisoPeligro, fmt.Sprintf("No se pudo borrar el chat (%d)", i+1), "Revisa la conexión y vuelve a intentarlo."))
			}
			if d.r.desbordando {
				d.r.desborde.ScrollToBottom()
			}
			pos := fyne.CurrentApp().Driver().AbsolutePositionForObject(d.confirmar)
			centro := pos.Add(fyne.NewPos(d.confirmar.Size().Width/2, d.confirmar.Size().Height/2))
			t.Logf("METRIC width=%.0f button=%v size=%v toast=%v size=%v", ancho, pos, d.confirmar.Size(), pila.Avisos()[0].Position(), pila.Avisos()[0].Size())
			for _, aviso := range pila.Avisos() {
				p := fyne.CurrentApp().Driver().AbsolutePositionForObject(aviso)
				si := aviso.Size()
				x0, y0, x1, y1 := p.X, p.Y, p.X+si.Width, p.Y+si.Height
				if d.r.avisos.Size().Height > 0 {
					clip := fyne.CurrentApp().Driver().AbsolutePositionForObject(d.r.desplazaAvisos)
					tam := d.r.desplazaAvisos.Size()
					x0, y0, x1, y1 = max(x0, clip.X), max(y0, clip.Y), min(x1, clip.X+tam.Width), min(y1, clip.Y+tam.Height)
				}
				if x1 > x0 && y1 > y0 && pos.X < x1 && pos.X+d.confirmar.Size().Width > x0 && pos.Y < y1 && pos.Y+d.confirmar.Size().Height > y0 {
					t.Error("el aviso se pinta sobre el botón de confirmar")
				}
			}
			guardarDialogoMovil(t, w.Canvas(), fmt.Sprintf("aviso-confirmacion-%vx%v", ancho, caso.alto))
			test.TapCanvas(w.Canvas(), centro)
			if confirmadas != 1 {
				t.Fatal("el aviso persistente tapa el botón de confirmar")
			}
		})
	}
}

func TestAvisosDelDialogoSeDesplazanYVuelvenAlAnterior(t *testing.T) {
	pila := pilaQueSeVacia(t)
	w := test.NewTempWindow(t, container.NewStack(canvas.NewRectangle(tema.Color("bg", tema.Claro)), pila))
	w.SetPadded(false)
	w.Resize(fyne.NewSize(320, 800))
	uno := NuevoDialogo("Reglas", cuerpoDe(60))
	dos := NuevoDialogo("Confirmar", cuerpoDe(40))
	uno.Mostrar(w.Canvas())
	t.Cleanup(uno.Cerrar)
	for i := range 6 {
		pila.Notificar(NuevoAviso(AvisoPeligro, fmt.Sprintf("Aviso %d", i), "Este aviso persiste hasta que decidas cerrarlo."))
	}
	dos.Mostrar(w.Canvas())
	t.Cleanup(dos.Cerrar)
	if uno.pilaAvisos() != nil || dos.pilaAvisos() != pila {
		t.Fatal("los avisos no siguen al diálogo de arriba")
	}
	r := dos.r
	if r.avisos.Size().Height > 160.1 {
		t.Fatalf("los avisos ocupan %vpx y el máximo es10rem", r.avisos.Size().Height)
	}
	if r.desplazaAvisos.Content.MinSize().Height <= r.desplazaAvisos.Size().Height {
		t.Fatal("los avisos largos no tienen recorrido propio")
	}
	r.desplazaAvisos.ScrollToBottom()
	ultimo := pila.Avisos()[5]
	test.WidgetRenderer(ultimo)
	pos := fyne.CurrentApp().Driver().AbsolutePositionForObject(ultimo.aspa)
	test.TapCanvas(w.Canvas(), pos.Add(fyne.NewPos(ultimo.aspa.Size().Width/2, ultimo.aspa.Size().Height/2)))
	if len(pila.Avisos()) != 5 {
		t.Fatal("el aviso del final no se puede cerrar después de desplazar")
	}
	dos.Cerrar()
	if uno.pilaAvisos() != pila {
		t.Fatal("al cerrar un diálogo anidado los avisos no vuelven al anterior")
	}
	uno.Cerrar()
	if pila.capaEncima() != nil {
		t.Fatal("el aviso deja una capa que bloquea la app al cerrar")
	}
}
