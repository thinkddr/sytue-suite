// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drive

import (
	"slices"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// botonFlotanteDe busca el «+» flotante en lo que pinta la ventana.
func botonFlotanteDe(o fyne.CanvasObject) *ui.BotonFlotante {
	var hallado *ui.BotonFlotante
	recorrer(o, func(x fyne.CanvasObject) {
		if b, ok := x.(*ui.BotonFlotante); ok {
			hallado = b
		}
	})
	return hallado
}

// menuAbierto es el menú que cuelga ahora de la ventana, o nil.
func menuAbierto(w fyne.Window) *ui.Menu {
	top := w.Canvas().Overlays().Top()
	if top == nil {
		return nil
	}
	var m *ui.Menu
	recorrer(top, func(x fyne.CanvasObject) {
		if mm, ok := x.(*ui.Menu); ok {
			m = mm
		}
	})
	return m
}

// TestNuevoFlota: crear en «Mi unidad» es el «+» flotante, abajo a la derecha y en TODOS
// los anchos (`BotonFlotante` de FileBrowser.tsx, producto 2026-11-25), y abre el menú de
// «Nuevo» —«Subir» delante— colgado de él y hacia arriba; «Subir» abre el selector del
// sistema. Sin red, en otra sección, no está; mientras algo ocupa el explorador, apagado.
// La cabecera ya no lleva «Nuevo» ni «Subir» (TestLaCabeceraYaNoCrea).
//
// VISTO EN ROJO con el «Subir» flotante de antes (solo por debajo de 768, sin menú) y sin
// el `Hidden` de `Repintar`, que lo dejaba a la vista sin red.
func TestNuevoFlota(t *testing.T) {
	w := test.NewWindow(nil)
	defer w.Close()
	p := NuevaPantalla(traductor(t), nil, nil, w)
	p.explor.Fase = Exito
	elegido := 0
	p.ElegirFicheros = func(func([]Entrada)) { elegido++ }
	w.SetContent(p.Objeto())
	visible := func(fab *ui.BotonFlotante) bool { return fab.Visible() && p.capaNuevo.Visible() }

	for _, tam := range []fyne.Size{fyne.NewSize(390, 760), fyne.NewSize(1280, 800)} {
		w.Resize(tam)
		_ = w.Canvas().Capture()
		fab := botonFlotanteDe(w.Content())
		if fab == nil || !visible(fab) {
			t.Fatalf("a %v en «Mi unidad» no está el «+» flotante", tam)
		}
		if fab.Nombre != p.tr.T("files.new", nil) {
			t.Errorf("el «+» se llama %q y es «Nuevo»", fab.Nombre)
		}
		pos, lado, raiz := fab.Position(), fab.Size(), p.raiz.Size()
		margen := ui.MargenFlotanteEn(raiz.Width)
		if pos.X+lado.Width != raiz.Width-margen || pos.Y+lado.Height != raiz.Height-margen {
			t.Errorf("a %v el «+» no está en la esquina de abajo a la derecha: %v %v en %v", tam, pos, lado, raiz)
		}
	}

	fab := botonFlotanteDe(w.Content())
	test.Tap(fab)
	m := menuAbierto(w)
	if m == nil {
		t.Fatal("pulsar el «+» no abre el menú de «Nuevo»")
	}
	if !m.Arriba {
		t.Error("el menú del «+» no abre hacia arriba")
	}
	primero, ok := m.Elementos[0].(*ui.ElementoMenu)
	if !ok || primero.Texto != p.tr.T("files.upload", nil) {
		t.Fatalf("el menú del «+» no empieza por «Subir»: %+v", m.Elementos[0])
	}
	fabEn := fyne.CurrentApp().Driver().AbsolutePositionForObject(fab)
	if fin := m.Position().Y + m.Size().Height; fin > fabEn.Y {
		t.Errorf("el menú acaba en y=%v y tapa el «+», que empieza en %v", fin, fabEn.Y)
	}
	primero.AlPulsar()
	if elegido != 1 {
		t.Errorf("«Subir» del menú abrió el selector %d veces", elegido)
	}
	m.Cerrar()

	p.explor.Ocupado = true
	p.Repintar()
	if !fab.Inerte {
		t.Error("con el explorador ocupado el «+» sigue vivo")
	}
	p.explor.Ocupado = false
	p.Repintar()

	p.conexion(false)
	if visible(fab) {
		t.Error("sin red el «+» flotante sigue a la vista")
	}
	p.conexion(true)
	p.irA(Papelera)
	if visible(fab) {
		t.Error("en la papelera el «+» flotante sigue a la vista")
	}
}

// TestSubirCarpetaConElBotonSoloDondeHaySelector: «Nuevo → Subir carpeta» está junto a
// «Carpeta» cuando el escritorio pone el selector de carpetas, abre ese selector con su
// título y le pasa a la subida la carpeta elegida; en el móvil (`ElegirCarpeta` nil) no
// se ofrece, y sin red no abre nada.
func TestSubirCarpetaConElBotonSoloDondeHaySelector(t *testing.T) {
	w := test.NewWindow(nil)
	defer w.Close()
	p := NuevaPantalla(traductor(t), nil, nil, w)
	p.explor.Fase = Exito
	rotulo := p.tr.T("files.upload.folder", nil)
	textos := func() []string {
		var out []string
		for _, o := range p.elementosNuevo() {
			if e, ok := o.(*ui.ElementoMenu); ok {
				out = append(out, e.Texto)
			}
		}
		return out
	}
	if slices.Contains(textos(), rotulo) {
		t.Fatal("sin selector de carpetas (el móvil) «Subir carpeta» se ofrece")
	}

	var titulo string
	abiertos := 0
	p.ElegirCarpeta = func(tit string, _ func([]Entrada)) {
		titulo = tit
		abiertos++
	}
	got := textos()
	if len(got) < 3 || got[2] != rotulo || got[1] != p.tr.T("files.new.folder", nil) {
		t.Fatalf("«Nuevo» = %q; «Subir carpeta» tiene que ir tras «Carpeta»", got)
	}
	e := p.elementosNuevo()[2].(*ui.ElementoMenu)
	e.AlPulsar()
	if abiertos != 1 || titulo != rotulo {
		t.Errorf("el selector se abrió %d veces, con título %q", abiertos, titulo)
	}
	p.conexion(false)
	e.AlPulsar()
	if abiertos != 1 {
		t.Error("sin red «Subir carpeta» abre el selector")
	}
}
