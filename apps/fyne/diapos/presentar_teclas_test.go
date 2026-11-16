// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package diapos

import (
	"image/color"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/test"
)

// cincoDiapos es un mazo de cinco sin animaciones: cada avance es una diapositiva.
func cincoDiapos(t *testing.T) *Presentacion {
	t.Helper()
	m, err := Leer([]byte(`{"slides":[{"id":"a","boxes":[]},{"id":"b","boxes":[]},
		{"id":"c","boxes":[]},{"id":"d","boxes":[]},{"id":"e","boxes":[]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	p := NuevaPresentacion(m, 0, traductor(t))
	w := test.NewTempWindow(t, p)
	w.Resize(fyne.NewSize(960, 540))
	return p
}

// pulsa es lo que entrega Fyne con una tecla que escribe: primero el nombre y después
// el carácter. Así se prueba también que no se atiende dos veces.
func pulsa(p *Presentacion, k fyne.KeyName, r rune) {
	p.Tecla(k)
	p.Runa(r)
}

// TestNumeroEIntroVaAEsaDiapositiva es «número + Intro» de PowerPoint: las cifras no
// mueven nada hasta Intro, y otra tecla entre medias olvida el número.
func TestNumeroEIntroVaAEsaDiapositiva(t *testing.T) {
	p := cincoDiapos(t)
	pulsa(p, fyne.Key4, '4')
	if p.Diapo != 0 {
		t.Fatalf("una cifra sola llevó a la %d", p.Diapo)
	}
	p.Tecla(fyne.KeyReturn)
	if p.Diapo != 3 {
		t.Fatalf("«4 + Intro» lleva a la %d y tocaba la 3 (0-based)", p.Diapo)
	}
	pulsa(p, fyne.Key9, '9')
	p.Tecla(fyne.KeyEnter)
	if p.Diapo != 4 {
		t.Errorf("«9 + Intro» con cinco diapositivas lleva a la %d y tocaba la última", p.Diapo)
	}
	p.Ir(0, 0)
	pulsa(p, fyne.Key3, '3')
	p.Tecla(fyne.KeyRight) // olvida el 3 y avanza
	p.Tecla(fyne.KeyReturn)
	if p.Diapo != 2 {
		t.Errorf("«3 → Intro» deja la %d: son dos avances, la 2", p.Diapo)
	}
	p.Tecla(fyne.KeyBackspace)
	if p.Diapo != 1 {
		t.Errorf("Retroceso deja la %d y tocaba la 1", p.Diapo)
	}
}

// TestLaPantallaNegraTapaYCualquierTeclaVuelve: B y «.» apagan en negro, W y «,» en
// blanco, el lector de pantalla lo oye, y cualquier tecla o un clic la devuelven SIN
// hacer nada más —ni avanzar ni salir—.
func TestLaPantallaNegraTapaYCualquierTeclaVuelve(t *testing.T) {
	p := cincoDiapos(t)
	tr := traductor(t)
	salido := false
	p.AlSalir = func() { salido = true }

	pulsa(p, fyne.KeyB, 'b')
	if p.Pantalla != Negra {
		t.Fatalf("B deja la pantalla en %d", p.Pantalla)
	}
	if got, quiero := p.AccessibilityLabel(), tr("slides.present.blackScreen", nil); got != quiero {
		t.Errorf("la región viva dice %q y tocaba %q", got, quiero)
	}
	if !contieneObjeto(p, p.r.apagon) {
		t.Error("la pantalla negra no tapa el visor")
	}
	guarda(t, "presentacion-negra.png", fyne.CurrentApp().Driver().CanvasForObject(p).Capture())
	p.Tecla(desktop.KeyShiftLeft)
	if p.Pantalla != Negra {
		t.Error("Mayúsculas sola devolvió la pantalla")
	}
	pulsa(p, fyne.KeyB, 'b') // la B que la devuelve no la vuelve a apagar
	if p.Pantalla != Encendida || p.Diapo != 0 {
		t.Fatalf("B otra vez deja pantalla=%d en la %d; tocaba encendida en la 0", p.Pantalla, p.Diapo)
	}
	if contieneObjeto(p, p.r.apagon) {
		t.Error("encendida, la capa negra sigue puesta")
	}

	pulsa(p, fyne.KeyPeriod, '.')
	p.Tecla(fyne.KeyEscape)
	if p.Pantalla != Encendida || salido {
		t.Errorf("Escape con la pantalla negra: pantalla=%d, salido=%v; tocaba volver sin salir", p.Pantalla, salido)
	}

	pulsa(p, fyne.KeyW, 'w')
	if p.Pantalla != Blanca || p.r.apagon.FillColor != color.White {
		t.Fatalf("W deja la pantalla en %d con %v", p.Pantalla, p.r.apagon.FillColor)
	}
	p.Tecla(fyne.KeyRight)
	if p.Pantalla != Encendida || p.Diapo != 0 {
		t.Errorf("→ con la pantalla blanca deja pantalla=%d en la %d; tocaba volver sin avanzar", p.Pantalla, p.Diapo)
	}
	pulsa(p, fyne.KeyComma, ',')
	p.Tapped(nil)
	if p.Pantalla != Encendida || p.Diapo != 0 {
		t.Errorf("un clic con la pantalla blanca deja pantalla=%d en la %d", p.Pantalla, p.Diapo)
	}
}

// TestInterrogacionAbreLaListaDeTeclas: «?» y H abren la hoja de atajos de la suite con
// las teclas en el idioma de quien presenta; N y P conservan su sentido.
func TestInterrogacionAbreLaListaDeTeclas(t *testing.T) {
	p := cincoDiapos(t)
	p.Runa('?')
	if !p.ayudaAbierta() {
		t.Fatal("«?» no abrió la lista de teclas")
	}
	guarda(t, "presentacion-teclas.png", fyne.CurrentApp().Driver().CanvasForObject(p).Capture())
	// Desde la ventana del moderador la tecla llega al visor: cualquiera la cierra.
	pulsa(p, fyne.KeyX, 'x')
	if p.ayudaAbierta() {
		t.Error("una tecla desde el moderador no cerró la lista")
	}
	pulsa(p, fyne.KeyH, 'h')
	if !p.ayudaAbierta() {
		t.Error("H no abrió la lista de teclas")
	}
	p.ayuda.Cerrar()
	pulsa(p, fyne.KeyN, 'n')
	if !p.Notas {
		t.Error("N ya no alterna las notas")
	}

	grupos := GruposDeTeclasPresentando(traductor(t))
	var filas []string
	for _, g := range grupos {
		for _, a := range g.Atajos {
			filas = append(filas, strings.Join(a.Teclas, " ")+" = "+a.Descripcion)
		}
	}
	todo := strings.Join(filas, "\n")
	if strings.Contains(todo, "slides.") {
		t.Errorf("hay claves sin traducir en la lista:\n%s", todo)
	}
	if !strings.Contains(todo, "Número Intro = Ir a esa diapositiva") {
		t.Errorf("falta «número + Intro» en español:\n%s", todo)
	}
}

func contieneObjeto(p *Presentacion, o fyne.CanvasObject) bool {
	for _, x := range p.r.Objects() {
		if x == o {
			return true
		}
	}
	return false
}

// TestVolverDelModeradorConLaPantallaApagada: en el teléfono el moderador va en la misma
// ventana, y tocar su diapositiva actual vuelve a la presentación aunque se apagara antes
// con la B. Antes `AlVolver` pasaba por `Runa('p')`, que con la pantalla apagada solo la
// encendía: el toque no volvía y había que salir con «Terminar».
func TestVolverDelModeradorConLaPantallaApagada(t *testing.T) {
	p := cincoDiapos(t)
	w := test.NewTempWindow(t, p)
	w.Resize(fyne.NewSize(AnchoModerador, AltoModerador))
	p.AlCambiarModerador = ModeradorEnLaMisma(w, p, "es")
	p.Runa('p')
	mod, ok := w.Canvas().Content().(*Moderador)
	if !ok {
		t.Fatalf("la P deja en la ventana %T", w.Canvas().Content())
	}
	p.Runa('b')
	if p.Pantalla != Negra {
		t.Fatalf("B deja la pantalla en %d", p.Pantalla)
	}
	mod.AlVolver()
	if w.Canvas().Content() != p || p.Moderador {
		t.Errorf("«Volver» con la pantalla negra deja %T (moderador=%v)", w.Canvas().Content(), p.Moderador)
	}
}
