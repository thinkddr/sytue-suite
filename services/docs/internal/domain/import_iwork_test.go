// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"strconv"
	"strings"
	"testing"
	"unicode"
)

func leerIWork(t *testing.T, nombre string) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/iwork/" + nombre)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// zipDe arma un ZIP en memoria con esas entradas (nombre → contenido).
func zipDe(t *testing.T, entradas map[string][]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for n, b := range entradas {
		w, err := zw.Create(n)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(b); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func jpegDe(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for i := range img.Pix {
		img.Pix[i] = 200
	}
	img.Set(0, 0, color.Black)
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestDetectaLosPaquetesDeIWork(t *testing.T) {
	casos := []struct {
		fichero, nombre string
		quiere          srcFormat
	}{
		{"testPages2013.pages", "informe.pages", fmtPages},
		{"testPages.pages", "informe.pages", fmtPages},
		{"testPagesPwdProtected.pages", "informe.pages", fmtPages},
		{"testKeynote2018.key", "charla.key", fmtKeynote},
		// Sin extensión manda lo que lleva dentro: un '09 con `index.xml` y un 2013 sin
		// diapositivas son de iWork, aunque no se sepa de cuál.
		{"testPages2013.pages", "adjunto", fmtIWork},
		{"testPages.pages", "adjunto.zip", fmtIWork},
		// La extensión no engaña a la firma: un .docx renombrado sigue sin ser de iWork.
	}
	for _, c := range casos {
		if got := detectFormat(c.nombre, leerIWork(t, c.fichero)); got != c.quiere {
			t.Errorf("%s como %q: formato %d, quiero %d", c.fichero, c.nombre, got, c.quiere)
		}
	}
}

func TestLoQueNoEsDeIWorkSigueComoAntes(t *testing.T) {
	// Una clave PEM se llama .key: entra como texto, que es lo que ha sido siempre.
	pem := []byte("-----BEGIN PRIVATE KEY-----\nMIIB\n-----END PRIVATE KEY-----\n")
	if got := detectFormat("servidor.key", pem); got != fmtText {
		t.Errorf("una clave PEM .key: formato %d, quiero texto", got)
	}
	// Un ZIP cualquiera con un index.xml no es un '09: le falta el resto del paquete.
	suelto := zipDe(t, map[string][]byte{"index.xml": []byte("<a/>")})
	if got := detectFormat("cosas.zip", suelto); got != fmtUnknown {
		t.Errorf("un ZIP con index.xml suelto: formato %d, quiero desconocido", got)
	}
	// Ni un .docx de verdad aunque se llame .pages.
	docx := zipDe(t, map[string][]byte{"[Content_Types].xml": []byte("<Types/>"), "word/document.xml": []byte("<w:document/>")})
	if got := detectFormat("renombrado.pages", docx); got != fmtDocx {
		t.Errorf("un .docx renombrado a .pages: formato %d, quiero docx", got)
	}
}

func TestSinConvertirSeAbreLaVistaPrevia(t *testing.T) {
	// Un 2013 cuyo índice no se lee (testPages2013.pages ya se convierte).
	ilegible := zipDe(t, map[string][]byte{"Index/Document.iwa": {0}, "preview.jpg": jpegDe(t, 8, 8)})
	res, err := ImportLocalized("informe.pages", ilegible, false, "es-ES")
	if err != nil {
		t.Fatal(err)
	}
	if res.Kind != KindDocument || res.Title != "informe" {
		t.Fatalf("kind %q, título %q", res.Kind, res.Title)
	}
	for _, quiero := range []string{"Vista previa · documento .pages", "El original no se ha modificado", `<img src="data:image/jpeg;base64,`} {
		if !strings.Contains(res.Content, quiero) {
			t.Errorf("falta %q en:\n%.400s", quiero, res.Content)
		}
	}
	// '09: la vista previa está en QuickLook/. Un índice que no se lee abre con ella (lo que
	// se convierte a medias la lleva al final, ver import_iwork09_test.go).
	res, err = Import("viejo.pages", pages09(t, "<no es xml", map[string][]byte{"QuickLook/Thumbnail.jpg": jpegDe(t, 8, 8)}))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Content, "data:image/jpeg") {
		t.Errorf("'09 sin la vista previa de QuickLook:\n%.400s", res.Content)
	}
}

func TestSinVistaPreviaNiContrasenaNuncaFalla(t *testing.T) {
	casos := []struct {
		nombre string
		datos  []byte
		quiero string
	}{
		{"charla.key", zipDe(t, map[string][]byte{"Index/Document.iwa": {0}}), "Conversion not available · .key presentation"},
		{"secreto.pages", leerIWork(t, "testPagesPwdProtected.pages"), "Password protection · .pages document"},
		{"secreto.numbers", zipDe(t, map[string][]byte{
			"Index/Document.iwa": {0}, ".iwpv2": {1}, "preview.jpg": jpegDe(t, 8, 8),
		}), "Password protection · .numbers spreadsheet"},
		// Un .pages que ni siquiera es un ZIP (una descarga cortada): aviso, no error.
		{"roto.pages", []byte("esto no es un paquete"), "Conversion not available · .pages document"},
		{"cortado.pages", leerIWork(t, "testPages2013.pages")[:4096], "Conversion not available · .pages document"},
	}
	for _, c := range casos {
		res, err := Import(c.nombre, c.datos)
		if err != nil {
			t.Errorf("%s: %v", c.nombre, err)
			continue
		}
		if res.Kind != KindDocument || !strings.Contains(res.Content, c.quiero) {
			t.Errorf("%s: kind %q, sin %q en:\n%.300s", c.nombre, res.Kind, c.quiero, res.Content)
		}
		if strings.Contains(res.Content, "<img") {
			t.Errorf("%s: no debería llevar imagen", c.nombre)
		}
	}
}

// Los topes, fijados: un ZIP con más entradas de las que tiene un paquete real no se
// trata como paquete, y una «vista previa» que descomprime de más no se lee.
func TestTopesDelPaqueteIWork(t *testing.T) {
	muchas := map[string][]byte{"Index/Document.iwa": {0}}
	for i := 0; i < maxEntradasIWork; i++ {
		muchas["Data/"+strconv.Itoa(i)] = nil
	}
	if p := abrirPaqueteIWork(zipDe(t, muchas)); p != nil {
		t.Errorf("con %d entradas no debería reconocerse", len(muchas))
	}

	enorme := append(jpegDe(t, 8, 8), make([]byte, maxVistaPrevia)...)
	res, err := Import("bomba.pages", zipDe(t, map[string][]byte{"Index/Document.iwa": {0}, "preview.jpg": enorme}))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(res.Content, "<img") || !strings.Contains(res.Content, "Conversion not available") {
		t.Errorf("una vista previa de más de %d bytes no debe leerse:\n%.300s", maxVistaPrevia, res.Content)
	}

	// La imagen se pinta a lo ancho del folio, sin deformarla.
	res, err = Import("ancha.pages", zipDe(t, map[string][]byte{"Index/Document.iwa": {0}, "preview.jpg": jpegDe(t, 1200, 300)}))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Content, `data-w="600" data-h="150"`) {
		t.Errorf("la vista previa no se escaló a 600×150:\n%.300s", res.Content)
	}
}

func TestLosAvisosEstanEnLosVeintiseisIdiomas(t *testing.T) {
	var todos map[string]map[string]string
	if err := json.Unmarshal(avisosJSON, &todos); err != nil {
		t.Fatal(err)
	}
	const idiomas = "bg cs da de el en es et fi fr ga hr hu is it lt lv mt nb nl pl pt ro sk sl sv"
	if len(todos) != len(strings.Fields(idiomas)) {
		t.Errorf("hay %d idiomas, quiero los %d de la plataforma", len(todos), len(strings.Fields(idiomas)))
	}
	for _, l := range strings.Fields(idiomas) {
		m, ok := todos[l]
		if !ok {
			t.Errorf("falta el idioma %q", l)
			continue
		}
		for k := range todos["en"] {
			if strings.TrimSpace(m[k]) == "" {
				t.Errorf("%s: falta %q", l, k)
			}
		}
		for k, v := range m {
			if _, ok := todos["en"][k]; !ok {
				t.Errorf("%s: %q no está en inglés", l, k)
			}
			for _, r := range v {
				if r > 0xFFFF || unicode.Is(unicode.So, r) {
					t.Errorf("%s: %q lleva un símbolo o emoji (%q)", l, k, r)
				}
			}
		}
	}
	if got := avisoImportar("pt_PT", "titulo.vista_previa"); got != "Pré-visualização" {
		t.Errorf("pt_PT: %q", got)
	}
	if got := avisoImportar("xx", "titulo.vista_previa"); got != "Preview" {
		t.Errorf("idioma sin textos: %q, quiero el inglés", got)
	}
}
