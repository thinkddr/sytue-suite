// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

// TestElEsqueletoPulsaComoLaHoja compara los fotogramas de `sytue-skeleton-pulse` con
// los de `packages/ui/src/styles.css`: opaco en el 0 % y en el 100 %, y 0,55 en el 50 %.
//
// VISTO EN ROJO (2026-09-22, en el piloto): devolviendo 1 siempre desde `opacidadPulso`
// —que es lo que hacían las cinco copias locales antes de este widget— el caso del 50 %
// falla con «pinta 1 y la hoja pide 0,55». Deshecho.
func TestElEsqueletoPulsaComoLaHoja(t *testing.T) {
	for _, c := range []struct {
		p, quiere float32
	}{{0, 1}, {0.5, OpacidadPulso}, {1, 1}} {
		if o := opacidadPulso(c.p); o != c.quiere {
			t.Errorf("en el %g %% del ciclo el pulso pinta %g y la hoja pide %g", c.p*100, o, c.quiere)
		}
	}
	// Y entre fotograma y fotograma se mueve de verdad, con la curva de la casa: a
	// mitad del primer tramo tiene que estar ENTRE los dos, no en ninguno de los dos.
	if o := opacidadPulso(0.25); o >= 1 || o <= OpacidadPulso {
		t.Errorf("a un cuarto del ciclo la opacidad es %g y tiene que caer entre %g y 1", o, OpacidadPulso)
	}

	// El alfa llega al píxel: el mismo bloque a mitad de ciclo pinta más claro sobre
	// blanco que recién montado.
	e := NuevoEsqueleto(40, 20)
	e.Resize(fyne.NewSize(40, 20))
	opaco := px(pinta(t, 40, 20, e), 40, 20)
	e.fase = 0.5
	e.Refresh()
	medio := px(pinta(t, 40, 20, e), 40, 20)
	if medio.R <= opaco.R {
		t.Errorf("a mitad de pulso el bloque pinta %v y sin pulso %v: el alfa no llega al píxel", medio, opaco)
	}
}

// TestElEsqueletoSeQuedaQuietoSinMovimiento: con `prefers-reduced-motion` la hoja apaga
// la animación y el bloque se queda opaco, no desaparece.
func TestElEsqueletoSeQuedaQuietoSinMovimiento(t *testing.T) {
	SinMovimiento = true
	defer func() { SinMovimiento = false }()

	e := NuevoEsqueleto(40, 20)
	e.Resize(fyne.NewSize(40, 20))
	_ = pinta(t, 40, 20, e) // monta el renderer, que es quien arrancaría el pulso
	if e.anim != nil {
		t.Error("con SinMovimiento el esqueleto ha arrancado una animación")
	}
	if o := opacidadPulso(e.fase); o != 1 {
		t.Errorf("quieto, el bloque tiene que quedarse opaco y está a %g", o)
	}
}

// TestLaMedidaDelEsqueletoEsLaDeLaHoja: `height = '1rem'` por defecto, y el ancho es el
// que se pida (cero = el `width: 100%`, que lo decide el contenedor).
func TestLaMedidaDelEsqueletoEsLaDeLaHoja(t *testing.T) {
	if m := MedidaEsqueleto(NuevoEsqueleto(0, 0)); m.Height != AltoEsqueleto || m.Width != 0 {
		t.Errorf("un esqueleto sin medidas ocupa %v y la hoja pide 0×%g", m, float32(AltoEsqueleto))
	}
	e := NuevoEsqueleto(80, 12)
	if m, d := MedidaEsqueleto(e), e.MinSize(); m != d {
		t.Errorf("MedidaEsqueleto dice %v y MinSize %v: la medida se declara UNA vez (regla 2)", m, d)
	}
}

// TestElParrafoRecortaConElipsis es el `line-clamp-N` de la web: como mucho N líneas y
// la última acaba en puntos suspensivos.
//
// VISTO EN ROJO (2026-09-22, en el piloto): quitando el recorte de `LineasDe` —o sea,
// dejando el `Parrafo` que tenía Photos— el caso de dos líneas devuelve cuatro y el
// test lo dice con el número. Deshecho.
func TestElParrafoRecortaConElipsis(t *testing.T) {
	const texto = "Sytue AI escribe esta descripción y se guarda como texto alternativo de la foto " +
		"para que un lector de pantalla pueda contarla."
	const ancho float32 = 160

	suelto := NuevoParrafo(texto, tema.TextoSM, Normal, "text", tema.Claro)
	entero := LineasDe(suelto, ancho)
	if len(entero) < 3 {
		t.Fatalf("el párrafo de prueba solo se reparte en %d líneas: no sirve para medir el recorte", len(entero))
	}

	topado := NuevoParrafo(texto, tema.TextoSM, Normal, "text", tema.Claro).Recortado(2)
	lineas := LineasDe(topado, ancho)
	if len(lineas) != 2 {
		t.Fatalf("con `line-clamp-2` salen %d líneas: %q", len(lineas), lineas)
	}
	if ultima := lineas[1]; !strings.HasSuffix(ultima, "…") {
		t.Errorf("la última línea recortada es %q y tiene que acabar en elipsis", ultima)
	}
	// Y lo recortado CABE: una elipsis que se sale por el borde es el defecto que se
	// venía a arreglar.
	if w := Medir(lineas[1], tema.TextoSM, Normal).Width; w > ancho {
		t.Errorf("la línea recortada mide %g y solo caben %g", w, ancho)
	}
	// El alto es el de las líneas que quedan, no el del texto entero.
	if a, quiere := AltoParrafo(topado, ancho), 2*tema.TextoSM.Linea; a != quiere {
		t.Errorf("un párrafo topado a dos líneas ocupa %g y tiene que ocupar %g", a, quiere)
	}
	// Y sin tope no recorta nada.
	if n := len(LineasDe(suelto, ancho)); n != len(entero) {
		t.Errorf("sin `line-clamp` el párrafo se ha recortado igual: %d líneas", n)
	}
	// El nombre accesible es el párrafo ENTERO: `line-clamp` es presentación.
	if topado.AccessibilityLabel() != texto {
		t.Error("el recorte se ha llevado por delante el nombre accesible")
	}
}

// TestElParrafoTachadoLlevaUnaRayaPorLinea: el `line-through` de un título que parte en
// varias líneas tacha cada una, a su ancho y a la altura de la raya de la Figtree.
func TestElParrafoTachadoLlevaUnaRayaPorLinea(t *testing.T) {
	const texto = "Llamar al del seguro para cambiar la póliza de la casa antes del viernes"
	const ancho float32 = 160
	p := NuevoParrafo(texto, tema.TextoBase, Medio, "text-muted", tema.Claro).Tachada()
	r := p.CreateRenderer()
	r.Layout(fyne.NewSize(ancho, AltoParrafo(p, ancho)))
	lineas := LineasDe(p, ancho)
	var rayas []*canvas.Rectangle
	for _, o := range r.Objects() {
		if raya, ok := o.(*canvas.Rectangle); ok {
			rayas = append(rayas, raya)
		}
	}
	if len(lineas) < 2 || len(rayas) != len(lineas) {
		t.Fatalf("%d líneas y %d rayas", len(lineas), len(rayas))
	}
	e := tema.TextoBase
	for i, raya := range rayas {
		base := float32(i)*e.Linea + LineaBase(e, Medio)
		if y := raya.Position().Y; y < base-0.4*e.Tam || y > base-0.2*e.Tam {
			t.Errorf("la raya %d va a %.1f y la base de su línea a %.1f", i, y, base)
		}
		if w := Medir(lineas[i], e, Medio).Width; raya.Size().Width != w {
			t.Errorf("la raya %d mide %.1f y su línea %.1f", i, raya.Size().Width, w)
		}
	}
}
