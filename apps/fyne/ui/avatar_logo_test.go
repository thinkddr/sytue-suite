// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"image"
	"image/color"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
)

// TestElAvatarDeLogoVaEnteroSobreLosaBlanca es el `logo` de Avatar.tsx: el icono
// entero con un 14 % de margen sobre blanco, en una losa de radio 22 %, sin círculo
// de color debajo; sin Foto, las iniciales de siempre.
func TestElAvatarDeLogoVaEnteroSobreLosaBlanca(t *testing.T) {
	rojo := color.NRGBA{R: 220, A: 255}
	img := image.NewNRGBA(image.Rect(0, 0, 32, 32))
	for i := 0; i < len(img.Pix); i += 4 {
		copy(img.Pix[i:], []byte{rojo.R, rojo.G, rojo.B, rojo.A})
	}
	a := NuevoAvatar("Vodafone").Con(AvatarLG)
	a.Logo = true
	a.Resize(MedidaAvatar(AvatarLG))
	r := test.WidgetRenderer(a).(*avatarRenderer)
	if r.losa.Visible() || !r.iniciales.Visible() || !r.circulo.Visible() {
		t.Fatal("sin foto, un avatar de logo son las iniciales en su círculo")
	}
	a.Foto = img
	a.Refresh()
	if !r.losa.Visible() || r.circulo.Visible() || r.iniciales.Visible() {
		t.Fatalf("con logo: losa %v, círculo %v", r.losa.Visible(), r.circulo.Visible())
	}
	if r.foto.Image != img {
		t.Error("el logo no se recorta: va entero")
	}
	m := redondear(margenLogo * 48)
	if r.foto.Position() != fyne.NewPos(m, m) || r.foto.Size() != fyne.NewSize(48-2*m, 48-2*m) {
		t.Errorf("foto en %v de %v; quiero el margen de %v", r.foto.Position(), r.foto.Size(), m)
	}
	if r.losa.CornerRadius != radioLogo*48 {
		t.Errorf("radio %v", r.losa.CornerRadius)
	}
	lienzo := pinta(t, 52, 52, a)
	if got := px(lienzo, int(2*escala), int(24*escala)); !cerca(got, color.NRGBA{R: 255, G: 255, B: 255, A: 255}) {
		t.Errorf("el margen pinta %v y es la losa blanca", got)
	}
	if got := px(lienzo, int(24*escala), int(24*escala)); !cerca(got, rojo) {
		t.Errorf("el centro pinta %v y es el logo", got)
	}
	// Quitarle el modo vuelve a la foto en círculo, sin rehacer el widget.
	a.Logo = false
	a.Refresh()
	if r.losa.Visible() || r.foto.Image == img {
		t.Error("sin Logo la foto vuelve a recortarse en círculo")
	}
}
