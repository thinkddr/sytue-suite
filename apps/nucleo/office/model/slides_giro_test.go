// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package model

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
)

// El giro de las cajas de Diapos (`SlideBox.rot`) sale en .pptx y en .odp y vuelve al
// importar en el mismo sitio: girada sobre su centro, con la x/y de la caja sin girar.

const modeloGirado = `{"slides":[{"id":"s1","boxes":[` +
	`{"id":"a","x":10,"y":20,"w":30,"h":15,"html":"<p>Girada</p>","rot":30},` +
	`{"id":"b","x":50,"y":40,"w":20,"h":20,"kind":"shape","shape":"rect","fill":"#FF0000","html":"","rot":270},` +
	`{"id":"c","x":5,"y":70,"w":20,"h":10,"html":"<p>Recta</p>"}]}]}`

func cajasReimportadas(t *testing.T, formato BlobFormat, ext string) []slideBox {
	t.Helper()
	data, _, sale, err := Export(KindPresentation, "Giro", stateWithContent(t, "doc:slides", modeloGirado), formato)
	if err != nil || sale != ext {
		t.Fatalf("export %s: %v %s", ext, err, sale)
	}
	r, err := Import("giro."+ext, data)
	if err != nil {
		t.Fatalf("import %s: %v", ext, err)
	}
	var m slidesModel
	if err := json.Unmarshal([]byte(r.Content), &m); err != nil || len(m.Slides) != 1 {
		t.Fatalf("modelo %s: %v", ext, err)
	}
	return m.Slides[0].Boxes
}

func comprobarGiros(t *testing.T, ext string, cajas []slideBox) {
	t.Helper()
	if len(cajas) != 3 {
		t.Fatalf("%s: %d cajas, quiero 3", ext, len(cajas))
	}
	quiero := []struct{ x, y, w, h, rot float64 }{{10, 20, 30, 15, 30}, {50, 40, 20, 20, 270}, {5, 70, 20, 10, 0}}
	cerca := func(a, b float64) bool { return math.Abs(a-b) < 0.05 }
	for i, q := range quiero {
		c := cajas[i]
		if !cerca(c.X, q.x) || !cerca(c.Y, q.y) || !cerca(c.W, q.w) || !cerca(c.H, q.h) || !cerca(c.Rot, q.rot) {
			t.Errorf("%s caja %d = x%.3f y%.3f w%.3f h%.3f rot%.3f, quiero %+v", ext, i, c.X, c.Y, c.W, c.H, c.Rot, q)
		}
	}
}

func TestElGiroDeLasCajasVaYVuelveEnPptx(t *testing.T) {
	data, _, _, err := Export(KindPresentation, "Giro", stateWithContent(t, "doc:slides", modeloGirado), FormatOOXML)
	if err != nil {
		t.Fatal(err)
	}
	s := zipParts(t, data)["ppt/slides/slide1.xml"]
	// 30° = 1 800 000 sesenta-milésimas; 270° = 16 200 000. La recta no lleva el atributo.
	mustContain(t, "slide1.xml", s, `<a:xfrm rot="1800000">`)
	mustContain(t, "slide1.xml", s, `<a:xfrm rot="16200000">`)
	if n := strings.Count(s, ` rot="`); n != 2 {
		t.Errorf("atributos rot = %d, quiero 2", n)
	}
	comprobarGiros(t, "pptx", cajasReimportadas(t, FormatOOXML, "pptx"))
}

func TestElGiroDeLasCajasVaYVuelveEnOdp(t *testing.T) {
	c := zipParts(t, exportarOdp(t, modeloGirado))["content.xml"]
	if n := strings.Count(c, `draw:transform="rotate (`); n != 2 {
		t.Errorf("draw:transform = %d, quiero 2", n)
	}
	comprobarGiros(t, "odp", cajasReimportadas(t, FormatODF, "odp"))
}

// LibreOffice escribe el giro antihorario en radianes y el sitio de la esquina superior
// izquierda ya girada. Un cuadrado de 2 cm en (4, 4) girado 90° a la derecha: su esquina
// superior izquierda acaba arriba a la derecha, en (6, 4), y θ = 3π/2.
func TestLeerTransformODPDeLibreOffice(t *testing.T) {
	var b slideBox
	leerTransformODP(&b, "rotate (4.71238898038469) translate (6cm 4cm)", 2, 2, 20, 10)
	if math.Abs(b.Rot-90) > 1e-6 || math.Abs(b.X-20) > 1e-3 || math.Abs(b.Y-40) > 1e-3 {
		t.Errorf("caja = x%.3f y%.3f rot%.3f, quiero x20 y40 rot90", b.X, b.Y, b.Rot)
	}
	// Sin transform no toca nada.
	z := slideBox{X: 1, Y: 2}
	leerTransformODP(&z, "", 2, 2, 20, 10)
	if z.X != 1 || z.Y != 2 || z.Rot != 0 {
		t.Errorf("sin transform cambió la caja: %+v", z)
	}
}

func TestGiroDML(t *testing.T) {
	for _, c := range []struct {
		in   string
		want float64
	}{{"5400000", 90}, {"-5400000", 270}, {"21600000", 0}, {"", 0}, {"x", 0}} {
		if got := giroDML(c.in); got != c.want {
			t.Errorf("giroDML(%q) = %v, quiero %v", c.in, got, c.want)
		}
	}
	if atributoGiroDML(0) != "" || atributoGiroDML(45) != ` rot="2700000"` {
		t.Error("atributoGiroDML")
	}
}
