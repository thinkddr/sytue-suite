// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/nucleo/tokens"
)

// TestLaMuestraDeColorEsLaDeBarraTsx mide la muestra contra `pdf/Barra.tsx`:
// `h-5 w-5 rounded-full border`, y la elegida con `border-text scale-110`.
//
// Los 20 y los 22 van ESCRITOS A MANO (5 × 0,25 × 16, y eso por 1,1): sacarlos de
// `LadoMuestraColor` sería medir la constante contra sí misma, y un cambio en ella
// movería el guardián con el widget.
//
// La elegida se consigue con `Elegida = true` y un `Refresh`, SIN redimensionar, que es
// exactamente lo que hace la cinta al pulsar: la medida no cambia, así que Fyne no
// volvería a llamar a `Layout` por su cuenta y una muestra que solo recolocase ahí se
// quedaría pequeña con el borde oscuro.
func TestLaMuestraDeColorEsLaDeBarraTsx(t *testing.T) {
	const (
		lado, ladoElegida float32 = 20, 22
		margen            float32 = 10
	)
	azul := color.NRGBA{R: 0x3b, G: 0x82, B: 0xf6, A: 255}
	blanco := color.NRGBA{R: 255, G: 255, B: 255, A: 255}
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())

	m := NuevaMuestraColor(azul, "Azul", nil)
	if got := m.MinSize(); got != fyne.NewSize(lado, lado) {
		t.Fatalf("la muestra dice medir %v; `h-5 w-5` son %g", got, lado)
	}
	img := colocado(t, m, m.MinSize(), margen)
	guardar(t, "muestra-reposo.png", img)

	x0, x1, y0, y1 := tinta(img, blanco)
	if an, al := x1-x0+1/escala, y1-y0+1/escala; absf(an-lado) > 1/escala || absf(al-lado) > 1/escala {
		t.Errorf("en reposo pinta %g×%g; la web, %g", an, al, lado)
	}
	medio := (margen + lado/2) * escala
	centro := int(medio)
	if p := px(img, centro, centro); !cerca(p, azul) {
		t.Errorf("el centro es %v y no el color pedido %v", p, azul)
	}
	// La segunda fila de dispositivo del borde, por el centro: ahí el círculo es plano
	// y no hay antialias que mezcle.
	// El `rgba(tinta, .12)` del borde va sobre el fondo del PROPIO elemento
	// (`background-clip: border-box`), que aquí es el color de la muestra.
	if p, quiero := px(img, centro, int(margen*escala)+1), tokens.SobreSRGB(tokens.Crudo("border", v), azul); !cerca(p, quiero) {
		t.Errorf("en reposo el borde es %v; la web pone `border-border` sobre la muestra (%v)", p, quiero)
	}

	m.Elegida = true
	m.Refresh()
	if got := m.MinSize(); got != fyne.NewSize(lado, lado) {
		t.Errorf("elegida dice medir %v: `scale-110` es un transform y NO ocupa sitio", got)
	}
	img = pinta(t, lado+2*margen, lado+2*margen, m)
	guardar(t, "muestra-elegida.png", img)
	x0, x1, y0, y1 = tinta(img, blanco)
	if an, al := x1-x0+1/escala, y1-y0+1/escala; absf(an-ladoElegida) > 1/escala || absf(al-ladoElegida) > 1/escala {
		t.Errorf("elegida pinta %g×%g; `scale-110` son %g alrededor del centro", an, al, ladoElegida)
	}
	arriba := margen - (ladoElegida-lado)/2
	if p := px(img, centro, int(arriba*escala)+1); !cerca(p, tema.Color("text", v)) {
		t.Errorf("elegida, el borde es %v; la web pone `border-text` (%v)", p, tema.Color("text", v))
	}
	if p := px(img, centro, centro); !cerca(p, azul) {
		t.Errorf("elegida, el centro es %v y no %v", p, azul)
	}
}

// TestLaMuestraNaceEntera es la regla 7 de `ui.go` para esta pieza: recién montada
// pinta lo mismo que tras un `Refresh`.
func TestLaMuestraNaceEntera(t *testing.T) {
	azul := color.NRGBA{R: 0x3b, G: 0x82, B: 0xf6, A: 255}
	monta := func() *MuestraColor {
		m := NuevaMuestraColor(azul, "Azul", nil)
		m.Elegida = true
		m.Resize(m.MinSize())
		m.Move(fyne.NewPos(3, 3))
		return m
	}
	crudo, refrescado := monta(), monta()
	sin := pixelesPintados(pinta(t, 24, 24, crudo))
	refrescado.Refresh()
	con := pixelesPintados(pinta(t, 24, 24, refrescado))
	if sin != con || con == 0 {
		t.Errorf("la muestra pinta %d píxeles recién montada y %d tras un Refresh", sin, con)
	}
}

func absf(x float32) float32 {
	if x < 0 {
		return -x
	}
	return x
}

// guardar deja la captura en SYTUE_CAPTURA_DIR para MIRARLA; sin la variable, nada.
func guardar(t *testing.T, nombre string, img image.Image) {
	t.Helper()
	dir := os.Getenv("SYTUE_CAPTURA_DIR")
	if dir == "" {
		return
	}
	f, err := os.Create(filepath.Join(dir, nombre))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
}
