// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

// TestElAvatarPintaLaFotoEnCirculo: con Foto, el avatar enseña la imagen (el cuadrado
// del centro) en círculo y no las iniciales; las esquinas quedan transparentes.
func TestElAvatarPintaLaFotoEnCirculo(t *testing.T) {
	rojo := color.NRGBA{R: 220, A: 255}
	img := image.NewNRGBA(image.Rect(0, 0, 40, 20))
	for y := range 20 {
		for x := range 40 {
			img.SetNRGBA(x, y, rojo)
		}
	}
	rec := RecorteCircular(img)
	if b := rec.Bounds(); b.Dx() != 20 || b.Dy() != 20 {
		t.Fatalf("el recorte mide %v y es el cuadrado del centro", b)
	}
	if _, _, _, a := rec.At(0, 0).RGBA(); a != 0 {
		t.Errorf("la esquina tiene alfa %d y queda fuera del círculo", a)
	}
	a := NuevoAvatar("Ana Ruiz").Con(AvatarXL)
	a.Foto = img
	a.Resize(MedidaAvatar(AvatarXL))
	r := test.WidgetRenderer(a).(*avatarRenderer)
	a.Refresh()
	if r.iniciales.Visible() || !r.foto.Visible() || r.foto.Size() != fyne.NewSize(64, 64) {
		t.Errorf("con foto: iniciales %v, foto %v de %v", r.iniciales.Visible(), r.foto.Visible(), r.foto.Size())
	}
	lienzo := pinta(t, 70, 70, a)
	if got := px(lienzo, int(32*escala), int(32*escala)); !cerca(got, rojo) {
		t.Errorf("el centro pinta %v y es la foto %v", got, rojo)
	}
	a.Foto = nil
	a.Refresh()
	if !r.iniciales.Visible() || r.foto.Visible() {
		t.Error("sin foto vuelven las iniciales")
	}
}

// TestElAvatarPredefinidoEsUnPNGDeSuColor es `presetAvatarDataUri`: 256 × 256, el
// fondo del color y las iniciales en blanco en medio.
func TestElAvatarPredefinidoEsUnPNGDeSuColor(t *testing.T) {
	fondo := tema.Color("avatar-1", tema.Claro)
	b, err := AvatarPredefinido("Ana Ruiz", fondo)
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	if img.Bounds().Dx() != 256 || img.Bounds().Dy() != 256 {
		t.Fatalf("mide %v", img.Bounds())
	}
	if got := color.NRGBAModel.Convert(img.At(4, 4)).(color.NRGBA); got != fondo {
		t.Errorf("el fondo es %v y el color %v", got, fondo)
	}
	blancos := 0
	for y := 96; y < 160; y++ {
		for x := 64; x < 192; x++ {
			if c := color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA); c.R > 240 && c.G > 240 && c.B > 240 {
				blancos++
			}
		}
	}
	if blancos < 500 {
		t.Errorf("en el centro hay %d píxeles blancos: las iniciales no están", blancos)
	}
}

// TestElSelectorDeAvatarSoloAbreDesdeLaCamara: el botón de la cámara (24,5, a 3,5 por
// fuera de la esquina) abre el menú; pulsar la cara del avatar no hace nada, como en la
// web, donde el avatar no es un botón.
func TestElSelectorDeAvatarSoloAbreDesdeLaCamara(t *testing.T) {
	s := NuevoSelectorAvatar(NuevoAvatar("Ana Ruiz").Con(AvatarLG))
	s.RotuloAnadir, s.RotuloColores, s.RotuloSubir = "Añadir foto", "Colores predefinidos", "Subir una foto…"
	w := test.NewWindow(s)
	defer w.Close()
	w.Resize(fyne.NewSize(300, 300))
	if got := s.MinSize(); got != fyne.NewSize(48, 48) {
		t.Errorf("el selector mide %v y es el avatar lg (48)", got)
	}
	s.Tapped(&fyne.PointEvent{Position: fyne.NewPos(20, 20)})
	if w.Canvas().Overlays().Top() != nil {
		t.Fatal("pulsar la cara del avatar abre el menú")
	}
	s.Tapped(&fyne.PointEvent{Position: fyne.NewPos(45, 45)})
	if w.Canvas().Overlays().Top() == nil {
		t.Fatal("la cámara no abre el menú")
	}
}

// TestLaCamaraDelSelectorNoTapaLasIniciales: el botón de la cámara va en la esquina del
// avatar y en la ficha de Contacts (64) tapaba el pie de la segunda inicial (auditoría
// del 2026-11-24). Se busca la tinta BLANCA de las iniciales en el avatar solo y se
// comprueba que el botón, colocado, no la pisa, en las dos tallas que lo llevan.
// VISTO EN ROJO con el `-right-1 -bottom-1` de la web.
func TestLaCamaraDelSelectorNoTapaLasIniciales(t *testing.T) {
	for _, talla := range []TallaAvatar{AvatarLG, AvatarXL} {
		for _, nombre := range []string{"Héctor Lamas", "Ana Ruiz", "Waldo Moreno"} {
			av := NuevoAvatar(nombre).Con(talla)
			m := MedidaAvatar(talla)
			av.Resize(m)
			img := pinta(t, m.Width, m.Height, av)
			blanco := tema.Color("neutral-0", tema.Claro)
			// Solo DENTRO del círculo: fuera está el blanco del lienzo de la prueba.
			centro, radio := m.Width/2*escala, (m.Width/2-2)*escala
			tinta := func(x, y int) bool {
				dx, dy := float32(x)-centro, float32(y)-centro
				return dx*dx+dy*dy <= radio*radio && cercaDe(px(img, x, y), blanco, 40)
			}
			s := NuevoSelectorAvatar(NuevoAvatar(nombre).Con(talla))
			s.Resize(m)
			r := test.WidgetRenderer(s).(*selectorAvatarRenderer)
			r.Layout(m)
			p := r.fondo.Position()
			// La tinta de las iniciales (en píxeles de la captura) no entra en el botón.
			hay := false
			for y := range img.Bounds().Dy() {
				for x := range img.Bounds().Dx() {
					if !tinta(x, y) {
						continue
					}
					hay = true
					if float32(x) >= p.X*escala && float32(y) >= p.Y*escala {
						t.Fatalf("talla %v, «%s»: el botón de la cámara (desde %v) tapa la inicial en (%d, %d)",
							talla, nombre, p, x, y)
					}
				}
			}
			if !hay {
				t.Fatalf("«%s»: no hay iniciales que medir", nombre)
			}
		}
	}
}

// La portada de una tarjeta redondeada: el trozo del centro a la proporción pedida, con las
// esquinas de ARRIBA transparentes y las de abajo enteras (la tarjeta sigue debajo).
func TestRecorteRedondeadoArriba(t *testing.T) {
	rojo := color.NRGBA{R: 220, A: 255}
	img := image.NewNRGBA(image.Rect(0, 0, 200, 100))
	for y := range 100 {
		for x := range 200 {
			img.SetNRGBA(x, y, rojo)
		}
	}
	rec := RecorteRedondeadoArriba(img, 40, 30, 4) // 4:3 → 133×100, radio 4 de 40 → 13 px
	if b := rec.Bounds(); b.Dx() != 133 || b.Dy() != 100 {
		t.Fatalf("el recorte mide %v y es el 4:3 del centro", b)
	}
	alfa := func(x, y int) uint32 { _, _, _, a := rec.At(x, y).RGBA(); return a }
	if alfa(0, 0) != 0 || alfa(132, 0) != 0 || alfa(2, 2) != 0 {
		t.Errorf("las esquinas de arriba asoman: alfa %d, %d, %d", alfa(0, 0), alfa(132, 0), alfa(2, 2))
	}
	if alfa(0, 99) != 0xffff || alfa(132, 99) != 0xffff || alfa(66, 0) != 0xffff || alfa(66, 50) != 0xffff {
		t.Error("lo de fuera de las esquinas de arriba se queda entero")
	}
}
