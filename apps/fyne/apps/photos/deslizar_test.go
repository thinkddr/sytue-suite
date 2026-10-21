// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package photos

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
)

// TestDeslizarEsLaReglaDeLaWeb repite los casos de `gestos.test.ts`: si se cambia un
// umbral allí, este se pone en rojo hasta cambiarlo aquí.
func TestDeslizarEsLaReglaDeLaWeb(t *testing.T) {
	casos := []struct {
		nombre    string
		dx, dy, w float32
		ms        float64
		quiero    int
	}{
		{"a la izquierda, la siguiente", -120, 4, 390, 300, 1},
		{"a la derecha, la anterior", 120, -4, 390, 300, -1},
		{"un roce lento", -40, 0, 390, 400, 0},
		{"estrecha: 55 no llegan al mínimo", -55, 0, 300, 500, 0},
		{"estrecha: 60, sí", -60, 0, 300, 500, 1},
		{"un temblor rápido", -20, 0, 390, 10, 0},
		{"en una tableta, 120 despacio no bastan", -120, 0, 820, 500, 0},
		{"en un teléfono, sí", -120, 0, 390, 500, 1},
		{"un golpe rápido pasa aunque recorra poco", -40, 0, 820, 60, 1},
		{"más vertical que horizontal", -100, 80, 390, 200, 0},
		{"horizontal por 1,4", -150, 100, 390, 200, 1},
	}
	for _, c := range casos {
		if got := decidirDeslizamiento(c.dx, c.dy, c.w, c.ms); got != c.quiero {
			t.Errorf("%s: decidir(%v, %v, %v, %v) = %d y la web da %d", c.nombre, c.dx, c.dy, c.w, c.ms, got, c.quiero)
		}
	}
}

// conTelefono hace de teléfono mientras dura el test.
func conTelefono(t *testing.T) {
	t.Helper()
	antes := conDedo
	conDedo = func() bool { return true }
	t.Cleanup(func() { conDedo = antes })
}

// TestDeslizarPasaDeFotoConElDedo: con el dedo, arrastrar de lado pasa de foto y la foto
// vuelve a su sitio; con el ratón, el mismo arrastre no hace nada.
func TestDeslizarPasaDeFotoConElDedo(t *testing.T) {
	test.NewTempApp(t)
	var pasos []int
	l := &LienzoVisor{}
	l.ExtendBaseWidget(l)
	d := NuevoDeslizaFoto(l, func() { pasos = append(pasos, -1) }, func() { pasos = append(pasos, 1) }, nil)
	d.Resize(fyne.NewSize(390, 600))
	arrastrar := func(dx float32) {
		for range 4 {
			d.Dragged(&fyne.DragEvent{Dragged: fyne.Delta{DX: dx / 4}})
		}
		d.DragEnd()
	}
	arrastrar(-120)
	if len(pasos) != 0 {
		t.Fatalf("con el ratón el arrastre pasó de foto: %v", pasos)
	}
	conTelefono(t)
	d.Dragged(&fyne.DragEvent{Dragged: fyne.Delta{DX: -60}})
	if x := l.Position().X; x != -60 {
		t.Errorf("la foto no sigue al dedo: está en %v y el dedo en −60", x)
	}
	d.DragEnd()
	// 60 px de golpe —aquí no pasa el tiempo— es un deslizamiento rápido: la siguiente.
	if len(pasos) != 1 || pasos[0] != 1 {
		t.Fatalf("un golpe a la izquierda tenía que abrir la siguiente y van %v", pasos)
	}
	arrastrar(120)
	if len(pasos) != 2 || pasos[1] != -1 {
		t.Errorf("deslizar a la derecha tenía que abrir la anterior y van %v", pasos)
	}
	if x := l.Position().X; x != 0 {
		t.Errorf("al soltar, la foto se queda desplazada %v", x)
	}
	// Hacia un lado sin foto, la foto se mueve a un cuarto del dedo y no pasa nada.
	sin := NuevoDeslizaFoto(l, nil, nil, nil)
	sin.Resize(fyne.NewSize(390, 600))
	sin.Dragged(&fyne.DragEvent{Dragged: fyne.Delta{DX: -100}})
	if x := l.Position().X; x != -25 {
		t.Errorf("hacia un lado sin foto la foto va a %v y tocaba −25 (un cuarto)", x)
	}
	sin.DragEnd()
}

// TestLasFlechasDelVisor: 48 de placa con el filete del tono `media`; con ratón el
// objetivo es una franja pegada al canto y con el dedo, solo la placa. Eran de 32, sin
// borde y a 12 del canto.
func TestLasFlechasDelVisor(t *testing.T) {
	test.NewTempApp(t)
	f := NuevaFlechaVisor("Foto anterior", true, func() {})
	zona := fyne.NewSize(1440, 851)
	if m := f.MinSize(); m.Width != 48 || m.Height != 48 {
		t.Fatalf("la placa mide %v y en la web 48×48", m)
	}
	pos, tam := f.cajaEn(zona, 1440)
	if pos.X != 0 || tam.Width != 80 || tam.Height != 288 {
		t.Errorf("con ratón el objetivo es %v en %v; la web, 80×288 pegado al canto", tam, pos)
	}
	der := NuevaFlechaVisor("Foto siguiente", false, func() {})
	if pos, tam := der.cajaEn(zona, 1440); pos.X+tam.Width != zona.Width {
		t.Errorf("la de la derecha no llega al canto: acaba en %v", pos.X+tam.Width)
	}
	conTelefono(t)
	pos, tam = f.cajaEn(zona, 390)
	if pos.X != 8 || tam.Width != 48 {
		t.Errorf("con el dedo en un teléfono la flecha es %v en %v; la web, la placa a 8 del canto", tam, pos)
	}
	if fileteSobreFoto.A == 0 {
		t.Error("el tono `media` va sin filete: sobre una foto negra el botón no se distingue")
	}
}

// TestElTamanoRealSeAbreDondeSePulsa: el punto pulsado se queda bajo el puntero, y la
// «z» deja el centro en el centro. Abría siempre por la esquina de arriba a la izquierda.
func TestElTamanoRealSeAbreDondeSePulsa(t *testing.T) {
	s := &servicioFalso{fotos: fotosDelServicio(3), porPagina: 100}
	a, _ := appDePrueba(t, s)
	a.Arrancar()
	a.abrirEnGaleria(a.g.fotos[0].ID)
	f, _ := a.vis.foto()
	// 0,4 y no más: el recorrido se queda en lo que la foto sobresale del hueco, y con la
	// ventana de pruebas (1280) a lo ancho no pasa de 4032 − 1280.
	a.tamanyoRealEn(0.4, 0.5, fyne.NewPos(100, 120))
	quiero := fyne.NewPos(0.4*float32(f.Width)-100, 0.5*float32(f.Height)-120)
	if !a.vis.tamanyoReal || a.vis.desplazamiento != quiero {
		t.Errorf("tras pulsar en (0,4, 0,5) el recorrido va por %v y tocaba %v", a.vis.desplazamiento, quiero)
	}
	a.alternarTamanyoReal() // vuelve a encajar
	a.alternarTamanyoReal() // la «z»: el centro en el centro
	h := a.huecoVisor()
	centro := fyne.NewPos(0.5*float32(f.Width)-h.Width/2, 0.5*float32(f.Height)-h.Height/2)
	if a.vis.desplazamiento != centro {
		t.Errorf("con la «z» el recorrido va por %v y tocaba %v (el centro)", a.vis.desplazamiento, centro)
	}
}

// TestConElDedoAmpliaElDobleToque: un toque suelto no hace nada; dos seguidos, sí.
func TestConElDedoAmpliaElDobleToque(t *testing.T) {
	test.NewTempApp(t)
	conTelefono(t)
	veces := 0
	l := &LienzoVisor{}
	l.ExtendBaseWidget(l)
	d := NuevoDeslizaFoto(l, nil, nil, func(float32, float32, fyne.Position) { veces++ })
	toque := &fyne.PointEvent{Position: fyne.NewPos(50, 50)}
	d.Tapped(toque)
	if veces != 0 {
		t.Fatal("con el dedo, un toque suelto amplió la foto")
	}
	d.Tapped(toque)
	if veces != 1 {
		t.Errorf("el doble toque no amplió la foto (%d)", veces)
	}
}
