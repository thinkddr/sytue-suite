// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rico

import "testing"

// TestElRecorteEsElDeLaWeb son los vectores de `clampCrop` en `docs-images.test.ts`: cada
// lado a [0, 0,9] y, por eje, ≥ 10 % a la vista.
func TestElRecorteEsElDeLaWeb(t *testing.T) {
	for _, c := range []struct{ entra, sale [4]float32 }{
		{[4]float32{-1, 2, 0.3, 0.1}, [4]float32{0, 0.81, 0.3, 0.09}},
		{[4]float32{0.6, 0, 0.6, 0}, [4]float32{0.45, 0, 0.45, 0}},
	} {
		if got := ClampRecorte(c.entra); got != c.sale {
			t.Errorf("ClampRecorte(%v) = %v, y la web da %v", c.entra, got, c.sale)
		}
	}
	if got := leerRecorte("0.1,x,,0.2"); got != [4]float32{0.1, 0, 0, 0.2} {
		t.Errorf("parseCrop: %v", got)
	}
}

// TestElEstiloDeLaImagenEsElDeLaWeb son los de `computeImageStyle`: el tamaño, el ajuste
// (flotando, en bloque, anclada) y el recorte que se come con márgenes negativos
// (separación − banda).
func TestElEstiloDeLaImagenEsElDeLaWeb(t *testing.T) {
	for _, c := range []struct {
		im     Imagen
		quiero string
	}{
		{Imagen{Ajuste: "inline", Alinear: "left", Ancho: 200, Alto: 120}, "width:200px;height:120px;max-width:100%;display:inline-block;vertical-align:bottom"},
		{Imagen{Ajuste: "square", Alinear: "left", Ancho: 300, Alto: 200}, "width:300px;height:200px;max-width:none;float:left;margin-right:14px;margin-bottom:14px"},
		{Imagen{Ajuste: "square", Alinear: "right", Ancho: 300, Alto: 200}, "width:300px;height:200px;max-width:none;float:right;margin-bottom:14px;margin-left:14px"},
		{
			Imagen{Ajuste: "square", Alinear: "left", Ancho: 300, Alto: 200, Recorte: [4]float32{0, 0.1, 0.25, 0}},
			"width:300px;height:200px;max-width:none;clip-path:inset(0px 30px 50px 0px);float:left;margin-right:-16px;margin-bottom:-36px",
		},
		{
			Imagen{Src: "data:x", Ajuste: "tight", Alinear: "left", Ancho: 200, Alto: 200},
			"width:200px;height:200px;max-width:none;float:left;shape-outside:inset(0px 0px 0px 0px);shape-margin:14px;margin-right:14px;margin-bottom:14px",
		},
		{Imagen{Ajuste: "behind", Alinear: "left", Ancho: 400, Alto: 300, X: 50, Y: 120}, "width:400px;height:300px;max-width:none;position:absolute;left:50px;top:120px;z-index:-1"},
		{
			Imagen{Ajuste: "topBottom", Alinear: "center", Ancho: 120, Alto: 80},
			"width:120px;height:80px;max-width:none;display:block;clear:both;margin-left:auto;margin-right:auto;margin-top:14px;margin-bottom:14px",
		},
		{
			Imagen{Ajuste: "inline", Alinear: "left", Ancho: 100, Alto: 100, Recorte: [4]float32{0.1, 0, 0, 0.05}},
			"width:100px;height:100px;max-width:100%;clip-path:inset(10px 0px 0px 5px);display:inline-block;vertical-align:bottom;margin-top:-10px;margin-left:-5px",
		},
	} {
		c.im.Recorte = ClampRecorte(c.im.Recorte)
		if got := estiloImagen(&c.im); got != c.quiero {
			t.Errorf("%+v:\n  hay     %s\n  quería  %s", c.im, got, c.quiero)
		}
	}
}

// TestPonerYQuitarLaImagen es `writeModel` sobre la imagen de `insertImage`: los data-* del
// modelo, la marca, el alt, `contenteditable` y el estilo, en el orden del navegador; y
// `removeImage`, que deja el párrafo vacío de la web si el folio se queda sin nada.
func TestPonerYQuitarLaImagen(t *testing.T) {
	d := Parse(`<p><img src="x"></p>`)
	im := d.Bloques()[0].ImagenEn(0)
	if im == nil {
		t.Fatal("no se encuentra la imagen en la runa 0")
	}
	im.Ajuste, im.Ancho, im.Alto, im.Alt, im.Recorte = "square", 300, 200, "Logo", [4]float32{0, 0.125, 0, 0}
	if !d.PonerImagen(im) {
		t.Fatal("la imagen sigue en el documento")
	}
	despues(t, "poner", d, `<p><img src="x" data-wrap="square" data-w="300" data-h="200" data-crop="0,0.125,0,0" data-sytue-image="" alt="Logo" `+
		`contenteditable="false" style="width:300px;height:200px;max-width:none;clip-path:inset(0px 37.5px 0px 0px);float:left;margin-right:-23.5px;margin-bottom:14px"></p>`)
	im = d.Bloques()[0].ImagenEn(0)
	im.Ajuste, im.Alt, im.Recorte = "front", "", [4]float32{}
	d.PonerImagen(im)
	despues(t, "anclarla", d, `<p><img src="x" data-sytue-image="" contenteditable="false" style="width:300px;height:200px;max-width:none;position:absolute;left:0px;top:0px;z-index:3" `+
		`data-wrap="front" data-w="300" data-h="200" data-x="0" data-y="0"></p>`)
	if !d.QuitarImagen(d.Bloques()[0].ImagenEn(0)) {
		t.Fatal("quitar la imagen")
	}
	despues(t, "quitarla", d, `<p></p>`)
	d = Parse(`<p>a<img src="x">b</p>`)
	d.QuitarImagen(d.Bloques()[0].ImagenEn(1))
	despues(t, "entre dos textos", d, `<p>ab</p>`)
	d = Parse(`<img src="x">`)
	d.QuitarImagen(d.Bloques()[0].ImagenEn(0))
	despues(t, "quitar la única", d, `<p><br></p>`)
}
