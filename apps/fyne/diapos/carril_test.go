// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package diapos

import (
	"encoding/json"
	"testing"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/catalogos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// traductor es el catálogo de verdad de la app, no un mapa de mentira: lo que se
// comprueba en varios de estos guardianes es que las claves EXISTEN, y con un doble no
// se comprobaría nada.
func traductor(t *testing.T) func(string, i18nVars) string {
	t.Helper()
	tr, err := i18n.New("es", i18n.Common(), catalogos.Docs())
	if err != nil {
		t.Fatalf("no se pudo cargar el catálogo: %v", err)
	}
	return func(c string, v i18nVars) string { return tr.T(c, v) }
}

// TestLaMiniaturaMideLoQueDiceMedir es el guardián del carril: 128 de ancho por la
// proporción del lienzo, y lo declarado igual a lo pedido a Fyne.
//
// El 128 no sale de esta implementación: es THUMB_W de SlidesCanvas.tsx, y va en
// PÍXELES porque lo de dentro se escala a 128 px — medirlo en rem lo ataría al tamaño
// de letra de la raíz, que es como en la web cada miniatura llegó a recortar 16 px de
// su lado derecho con la raíz en 14.
func TestLaMiniaturaMideLoQueDiceMedir(t *testing.T) {
	casos := []struct {
		json string
		alto float32
	}{
		{`{"slides":[{"id":"s","boxes":[]}]}`, 72},                         // 16:9 → el h-18 clásico
		{`{"slides":[{"id":"s","boxes":[]}],"size":{"preset":"4:3"}}`, 96}, // 128 × 720/960
	}
	for _, c := range casos {
		m, err := Leer([]byte(c.json))
		if err != nil {
			t.Fatal(err)
		}
		declarada := MedidaMiniatura(m)
		if declarada.Width != AnchoMiniatura || declarada.Height != c.alto {
			t.Errorf("MedidaMiniatura = %v, quiero %vx%v", declarada, AnchoMiniatura, c.alto)
		}
		mini := NuevaMiniatura(m, 0, nil)
		if got := mini.MinSize(); got != declarada {
			t.Errorf("MinSize() = %v y MedidaMiniatura dice %v: dos cuentas donde va una", got, declarada)
		}
	}
	// Y el carril entero es la miniatura más su aire: THUMB_W + 32.
	if AnchoCarrilDiapos != 160 {
		t.Errorf("el carril mide %v y `THUMB_W + 32` son 160", AnchoCarrilDiapos)
	}
}

// TestLaMiniaturaActivaLlevaElAnilloDeAcento: el `box-shadow: 0 0 0 2px` de la activa,
// medido sobre los píxeles.
func TestLaMiniaturaActivaLlevaElAnilloDeAcento(t *testing.T) {
	m := leeModelo(t)
	acento := tema.Color("primary", tema.Claro)
	for _, activa := range []bool{false, true} {
		mini := NuevaMiniatura(m, 0, nil)
		mini.Activa = activa
		mini.Resize(MedidaMiniatura(m))
		img := pinta(t, MedidaMiniatura(m).Width+8, MedidaMiniatura(m).Height+8, mini)
		guarda(t, map[bool]string{true: "miniatura-activa.png", false: "miniatura.png"}[activa], img)
		_, x1, _, _ := caja(img, acento)
		if hay := x1 >= 0; hay != activa {
			t.Errorf("activa=%v y el anillo de acento %s", activa,
				map[bool]string{true: "está", false: "no está"}[hay])
		}
	}
}

// TestLaMiniaturaSeElige comprueba las tres formas de elegirla: el ratón, Intro y la
// barra espaciadora. La tercera se olvida siempre, y sin ella el carril no se puede
// recorrer con el teclado.
func TestLaMiniaturaSeElige(t *testing.T) {
	m := leeModelo(t)
	for _, c := range []struct {
		nombre string
		hacer  func(*Miniatura)
	}{
		{"ratón", func(x *Miniatura) { x.Tapped(&fyne.PointEvent{}) }},
		{"Intro", func(x *Miniatura) { x.TypedKey(&fyne.KeyEvent{Name: fyne.KeyReturn}) }},
		{"espacio", func(x *Miniatura) { x.TypedRune(' ') }},
	} {
		elegida := -1
		mini := NuevaMiniatura(m, 2, func(i int) { elegida = i })
		c.hacer(mini)
		if elegida != 2 {
			t.Errorf("con %s se eligió %d y tenía que ser la 2", c.nombre, elegida)
		}
	}
}

// TestLasTeclasDePresentacionSonLasDeLaWeb es `presentKeyTarget`, con sus topes: una
// tecla repetida al final del mazo no puede dejar la presentación en blanco.
func TestLasTeclasDePresentacionSonLasDeLaWeb(t *testing.T) {
	const ultima = 4
	casos := []struct {
		tecla  fyne.KeyName
		desde  int
		quiero int
	}{
		{fyne.KeyRight, 0, 1},
		{fyne.KeySpace, 0, 1},
		{fyne.KeyPageDown, 0, 1},
		{fyne.KeyRight, ultima, ultima}, // tope arriba
		{fyne.KeyLeft, 2, 1},
		{fyne.KeyLeft, 0, 0}, // tope abajo
		{fyne.KeyHome, 3, 0},
		{fyne.KeyEnd, 0, ultima},
		{fyne.KeyEscape, 2, Salir},
		{fyne.KeyF1, 2, SinDestino},
	}
	for _, c := range casos {
		if got := TeclaPresentando(c.tecla, c.desde, ultima); got != c.quiero {
			t.Errorf("%v desde %d lleva a %d, quiero %d", c.tecla, c.desde, got, c.quiero)
		}
	}
}

// TestLosEnlacesDeCajaLlevanDondeDicen es `actionTarget`, incluida la unión de tipos
// del modelo: 'first'|'last'|'next'|'prev' o un número 1-based.
func TestLosEnlacesDeCajaLlevanDondeDicen(t *testing.T) {
	const ultima = 4
	casos := []struct {
		accion string
		desde  int
		quiero int
	}{
		{`"first"`, 3, 0},
		{`"last"`, 0, ultima},
		{`"next"`, ultima, ultima},
		{`"prev"`, 0, 0},
		{`3`, 0, 2},          // 1-based en el documento, 0-based aquí
		{`9`, 0, SinDestino}, // fuera de rango
		{`0`, 0, SinDestino}, // el 0 no existe: el modelo empieza en 1
		{`"zzz"`, 0, SinDestino},
	}
	for _, c := range casos {
		if got := DestinoAccion(json.RawMessage(c.accion), c.desde, ultima); got != c.quiero {
			t.Errorf("acción %s desde %d lleva a %d, quiero %d", c.accion, c.desde, got, c.quiero)
		}
	}
	if got := DestinoAccion(nil, 0, ultima); got != SinDestino {
		t.Errorf("una caja SIN enlace da %d y tiene que dar SinDestino", got)
	}
}

// TestElCronometroDelModeradorEsElDeLaWeb es `fmtElapsed`.
func TestElCronometroDelModeradorEsElDeLaWeb(t *testing.T) {
	casos := map[int]string{-5: "0:00", 0: "0:00", 9: "0:09", 65: "1:05", 3600: "1:00:00", 3725: "1:02:05"}
	for s, q := range casos {
		if got := Transcurrido(s); got != q {
			t.Errorf("Transcurrido(%d) = %q, quiero %q", s, got, q)
		}
	}
}
