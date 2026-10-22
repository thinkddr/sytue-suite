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
