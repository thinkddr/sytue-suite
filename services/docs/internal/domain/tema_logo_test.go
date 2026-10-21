// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"encoding/json"
	"strings"
	"testing"
)

func logoDePrueba(hash string, ancho, alto int) *LogoMarca {
	return &LogoMarca{PNG: []byte("png-" + hash), Hash: hash, Ancho: ancho, Alto: alto}
}

// El logo va con la marca a la cabecera de un documento: una vez, sin tocar el texto, se
// cambia cuando cambia su versión y se va con cualquier otro tema.
func TestLogoDeMarcaEnDocumento(t *testing.T) {
	marca := marcaDePrueba().Tema().ConLogo(logoDePrueba("aaaa", 200, 50))
	if marca.LogoHash != "aaaa" {
		t.Fatalf("la versión del logo no viaja en el tema: %+v", marca)
	}
	out, err := AplicarTema(KindDocument, docConTema, marca)
	if err != nil {
		t.Fatal(err)
	}
	// 200×50 a 48 px de alto = 192×48; va delante de todo.
	const cabecera = `<p data-logo-marca="aaaa"><img src="data:image/png;base64,cG5nLWFhYWE=" alt="Acme" width="192" height="48"/></p><h1 `
	if !strings.HasPrefix(out, cabecera) {
		t.Fatalf("cabecera con logo: %.200s", out)
	}
	if strings.TrimSuffix(textoPlano(t, out), "|") != strings.TrimSuffix(textoPlano(t, docConTema), "|") {
		t.Error("el logo cambió el texto")
	}
	// Dos y tres aplicaciones: ni se duplica ni cambia un byte. (El párrafo del logo es un
	// <p>: si el tema lo estilara como cuerpo, la segunda vuelta lo cambiaría.)
	paso := out
	for i := 2; i <= 3; i++ {
		otra, err := AplicarTema(KindDocument, paso, marca)
		if err != nil || otra != out || strings.Count(otra, attrLogoMarca) != 1 || strings.Count(otra, "data:image/png") != 1 {
			t.Fatalf("aplicación %d: duplica o cambia el logo (%v):\n%.300s", i, err, otra)
		}
		paso = otra
	}
	// Otra versión del logo: se cambia, no se acumula.
	v2, _ := AplicarTema(KindDocument, out, marca.ConLogo(logoDePrueba("bbbb", 600, 60)))
	if strings.Count(v2, attrLogoMarca) != 1 || !strings.Contains(v2, `data-logo-marca="bbbb"`) || !strings.Contains(v2, `width="240" height="24"`) {
		t.Errorf("cambio de logo (apaisado: limitado por el ancho): %.200s", v2)
	}
	if strings.Contains(v2, "cG5nLWFhYWE=") || strings.Count(v2, "data:image/png") != 1 || !strings.HasPrefix(v2, `<p data-logo-marca="bbbb">`) {
		t.Errorf("el logo anterior debe SUSTITUIRSE, en el mismo sitio: %.200s", v2)
	}
	// Otra versión de la MARCA (otro acento) con el mismo logo: el logo ni se mueve ni se repite.
	otraMarca := marcaDePrueba()
	otraMarca.Version, otraMarca.Accent = 9, "#0F766E"
	v3, err := AplicarTema(KindDocument, v2, otraMarca.Tema().ConLogo(logoDePrueba("bbbb", 600, 60)))
	if err != nil || strings.Count(v3, attrLogoMarca) != 1 || !strings.HasPrefix(v3, `<p data-logo-marca="bbbb"><img `) || !strings.Contains(v3, "#0F766E") || strings.Contains(v3, "#B45309") {
		t.Errorf("cambio de versión de marca con el mismo logo: %v %.300s", err, v3)
	}
	if otra, _ := AplicarTema(KindDocument, v3, otraMarca.Tema().ConLogo(logoDePrueba("bbbb", 600, 60))); otra != v3 {
		t.Error("tras cambiar de versión de marca, repetir cambia el documento")
	}
	// La marca sin logo, otro tema o «papel» lo quitan; lo demás del documento sigue.
	for nombre, tema := range map[string]TemaComun{
		"sin logo": marca.ConLogo(nil),
		"carbon":   func() TemaComun { c, _ := TemaPorID("carbon", nil); return c }(),
		"papel":    func() TemaComun { c, _ := TemaPorID(IDTemaPapel, nil); return c }(),
	} {
		sin, err := AplicarTema(KindDocument, v2, tema)
		if err != nil || strings.Contains(sin, attrLogoMarca) || strings.Contains(sin, "data:image") || !strings.Contains(sin, "Informe") {
			t.Errorf("%s: el logo debe irse y el contenido quedarse: %v %.160s", nombre, err, sin)
		}
	}
	// Un tema del catálogo nunca pone logo aunque se le cuelgue uno.
	carbon, _ := TemaPorID("carbon", nil)
	if c, _ := AplicarTema(KindDocument, docConTema, carbon.ConLogo(logoDePrueba("cccc", 10, 10))); strings.Contains(c, attrLogoMarca) {
		t.Error("un tema que no es la marca puso un logo")
	}
}

func TestLogoDeMarcaEnPresentacionYHoja(t *testing.T) {
	const pres = `{"slides":[{"id":"s1","boxes":[{"id":"t","x":1,"y":2,"w":3,"h":4,"html":"<h1>Hola</h1>"}]},{"id":"s2","boxes":[]}]}`
	marca := marcaDePrueba().Tema().ConLogo(logoDePrueba("aaaa", 160, 90))
	out, err := AplicarTema(KindPresentation, pres, marca)
	if err != nil {
		t.Fatal(err)
	}
	var m struct {
		Slides []struct {
			Boxes []map[string]any `json:"boxes"`
		} `json:"slides"`
	}
	if err := json.Unmarshal([]byte(out), &m); err != nil {
		t.Fatal(err)
	}
	if len(m.Slides[0].Boxes) != 2 || len(m.Slides[1].Boxes) != 0 {
		t.Fatalf("el logo va SOLO en la portada: %s", out)
	}
	caja := m.Slides[0].Boxes[1]
	// 16:9 en un lienzo 16:9: 10 % de alto = 10 % de ancho.
	if caja["id"] != "logo-marca" || caja["kind"] != "image" || caja["w"] != 10.0 || caja["h"] != 10.0 || caja["src"] != "data:image/png;base64,cG5nLWFhYWE=" {
		t.Errorf("caja del logo: %v", caja)
	}
	if m.Slides[0].Boxes[0]["html"] != "<h1>Hola</h1>" {
		t.Error("el logo tocó el contenido de la portada")
	}
	paso := out
	for i := 2; i <= 3; i++ {
		otra, err := AplicarTema(KindPresentation, paso, marca)
		if err != nil || otra != out || strings.Count(otra, `"logo-marca"`) != 1 {
			t.Fatalf("aplicación %d: duplica o cambia el logo de la portada (%v): %s", i, err, otra)
		}
		paso = otra
	}
	// Otro logo (y otra versión de la marca): se sustituye, sigue habiendo uno y en la portada.
	otraMarca := marcaDePrueba()
	otraMarca.Version, otraMarca.Bg = 9, "#ECFDF5"
	v2, err := AplicarTema(KindPresentation, out, otraMarca.Tema().ConLogo(logoDePrueba("bbbb", 90, 160)))
	if err != nil || strings.Count(v2, `"logo-marca"`) != 1 || strings.Contains(v2, "cG5nLWFhYWE=") || !strings.Contains(v2, "cG5nLWJiYmI=") || !strings.Contains(v2, "#ECFDF5") {
		t.Errorf("cambio de logo y de versión de marca en la portada: %v %s", err, v2)
	}
	if otra, _ := AplicarTema(KindPresentation, v2, otraMarca.Tema().ConLogo(logoDePrueba("bbbb", 90, 160))); otra != v2 {
		t.Error("tras cambiar de logo, repetir cambia la presentación")
	}
	pizarra, _ := TemaPorID("pizarra", nil)
	if sin, _ := AplicarTema(KindPresentation, out, pizarra); strings.Contains(sin, "logo-marca") || !strings.Contains(sin, "Hola") {
		t.Errorf("otro tema debe quitar el logo: %s", sin)
	}
	// La hoja no tiene imágenes en su modelo: la marca le pone sus colores y nada más.
	hoja, err := AplicarTema(KindSpreadsheet, hojaConTema, marca)
	if err != nil || strings.Contains(hoja, "data:image") || strings.Contains(hoja, "logo") {
		t.Errorf("la hoja no lleva logo: %v %s", err, hoja)
	}
}

// El diff de «Actualizar a la marca vigente» nombra el logo cuando es lo que cambia, y no
// descuadra el resto (el párrafo del logo desplaza los nodos del documento).
func TestPreviaConLogo(t *testing.T) {
	sin := marcaDePrueba().Tema()
	con := sin.ConLogo(logoDePrueba("aaaa", 100, 50))
	for _, c := range []struct {
		kind      Kind
		contenido string
	}{{KindDocument, docConTema}, {KindPresentation, `{"slides":[{"id":"s1","boxes":[]}]}`}} {
		base, _ := AplicarTema(c.kind, c.contenido, sin)
		cambios, err := PrevisualizarTema(c.kind, base, con)
		if err != nil || len(cambios) != 1 || cambios[0].Scope != "logo" || cambios[0].From != "" || cambios[0].To != "logo" {
			t.Errorf("%s: poner el logo: %v %+v", c.kind, err, cambios)
		}
		conLogo, _ := AplicarTema(c.kind, base, con)
		if otra, _ := PrevisualizarTema(c.kind, conLogo, con); len(otra) != 0 {
			t.Errorf("%s: con el logo vigente no hay nada que cambiar: %+v", c.kind, otra)
		}
		quitar, _ := PrevisualizarTema(c.kind, conLogo, sin)
		if len(quitar) != 1 || quitar[0].Scope != "logo" || quitar[0].From != "logo" || quitar[0].To != "" {
			t.Errorf("%s: quitar el logo: %+v", c.kind, quitar)
		}
	}
}
