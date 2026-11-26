// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package main

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/png"
	"path/filepath"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/local/archivos"
	localpdf "git.sytue.eu/sytue/one/apps/nucleo/pdf"
	"git.sytue.eu/sytue/one/apps/nucleo/pdfsimple"
)

func sinTesseract() (string, bool) { return "", false }

func TestPalabrasDeTSV(t *testing.T) {
	tsv := "level\tpage_num\tblock_num\tpar_num\tline_num\tword_num\tleft\ttop\twidth\theight\tconf\ttext\n" +
		"1\t1\t0\t0\t0\t0\t0\t0\t800\t600\t-1\t\n" +
		"5\t1\t1\t1\t1\t1\t36\t92\t120\t30\t96.5\tContrato\n" +
		"5\t1\t1\t1\t1\t2\t170\t92\t60\t30\t91\tde\n" +
		"5\t1\t1\t1\t1\t3\t240\t92\t0\t30\t-1\t \n"
	got := palabrasDeTSV([]byte(tsv))
	if len(got) != 2 || got[0] != (api.PDFPalabra{Texto: "Contrato", X: 36, Y: 92, Ancho: 120, Alto: 30}) || got[1].Texto != "de" {
		t.Fatalf("%+v", got)
	}
}

func TestSinOCRNiIANoSeOfreceReconocer(t *testing.T) {
	r := &reconocedor{asistir: func() func(context.Context, api.PeticionIA) (string, error) { return nil }, buscar: sinTesseract}
	if _, err := r.Idiomas(t.Context()); !errors.Is(err, api.ErrOCRNoDisponible) {
		t.Fatal(err)
	}
	if _, err := r.Reconocer(t.Context(), nil, "", ""); !errors.Is(err, api.ErrOCRNoDisponible) {
		t.Fatal(err)
	}
}

func pngDe(t *testing.T, w, h int) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := png.Encode(&b, image.NewGray(image.Rect(0, 0, w, h))); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestElModeloTranscribeYLasLineasVanEnSuFranja(t *testing.T) {
	var pedida api.PeticionIA
	ia := func(_ context.Context, q api.PeticionIA) (string, error) {
		pedida = q
		return "Primera línea\n\n  Segunda  \n", nil
	}
	r := &reconocedor{asistir: func() func(context.Context, api.PeticionIA) (string, error) { return ia }, buscar: sinTesseract}
	if idiomas, err := r.Idiomas(t.Context()); err != nil || len(idiomas) != 0 {
		t.Fatal("con el modelo se ofrece, sin idiomas que elegir", idiomas, err)
	}
	got, err := r.Reconocer(t.Context(), pngDe(t, 1000, 2000), "image/png", "spa")
	if err != nil || len(got) != 2 || pedida.ImagenTipo != "image/png" || len(pedida.Imagen) == 0 || pedida.Accion != "transcribir" {
		t.Fatal(got, err)
	}
	if got[0].Texto != "Primera línea" || got[1].Texto != "Segunda" || got[0].Y >= got[1].Y || got[0].X != 50 || got[0].Ancho != 900 {
		t.Fatalf("franjas: %+v", got)
	}
	if !strings.Contains(r.descripcion("es"), "se le envía la imagen") {
		t.Fatal("la nota dice que la imagen sale hacia el modelo")
	}
}

// TestTesseractReconoceUnEscaneoYQuedaLaCapa es de punta a punta: un escaneo de verdad
// (una página con texto dibujada a PNG y vuelta a PDF), reconocido por el tesseract del
// equipo a través del motor local, con la capa de texto escrita encima.
func TestTesseractReconoceUnEscaneoYQuedaLaCapa(t *testing.T) {
	if _, ok := buscarTesseract(); !ok {
		t.Skip("tesseract no está instalado")
	}
	disco, err := archivos.Abrir(filepath.Join(t.TempDir(), "Drive"), archivos.Opciones{Instalacion: "prueba"})
	if err != nil {
		t.Fatal(err)
	}
	pdf, err := api.NewPDF("https://sin-servidor.invalid", "https://sin-servidor.invalid", nil, nil).WithPrivateDrive(disco)
	if err != nil {
		t.Fatal(err)
	}
	r := &reconocedor{asistir: func() func(context.Context, api.PeticionIA) (string, error) { return nil }, buscar: buscarTesseract}
	pdf.ConLocal(api.PDFLocal{Reconocedor: r})

	w := pdfsimple.Nuevo()
	w.Texto("CONTRATO DE ARRENDAMIENTO", "F1", 24)
	paginas, err := pdf.RenderPNG(t.Context(), api.PDFFichero{Nombre: "t.pdf", Datos: w.Bytes()}, "1", 3, "")
	if err != nil || paginas[1] == nil {
		t.Fatal("no se dibujó la página", err)
	}
	escaneo, err := pdf.Motor(t.Context(), "/pdf/tools", map[string]string{"op": "imagenes-a-pdf"}, nil, api.PDFFichero{Nombre: "hoja.png", Datos: paginas[1]})
	if err != nil {
		t.Fatal(err)
	}
	doc := api.PDFFichero{Nombre: "escaneo.pdf", Datos: escaneo.Datos}
	estado, err := pdf.Motor(t.Context(), "/pdf/ocr", map[string]string{"op": "estado"}, nil, doc)
	if err != nil || !strings.Contains(string(estado.Datos), `"disponible":true`) || !strings.Contains(string(estado.Datos), `"spa"`) {
		t.Fatal("estado", string(estado.Datos), err)
	}
	res, err := pdf.Motor(t.Context(), "/pdf/ocr", map[string]string{"op": "reconocer", "idiomas": "spa"}, nil, doc)
	if err != nil || res.Cabecera.Get("X-Sytue-Ocr-Paginas") != "1" {
		t.Fatal("reconocer", err)
	}
	leido, err := localpdf.LeerPDF(res.Datos)
	if err != nil {
		t.Fatal(err)
	}
	texto, err := leido.TextoPlano(0)
	if err != nil || !strings.Contains(strings.ToUpper(texto), "ARRENDAMIENTO") {
		t.Fatalf("la capa no tiene el texto reconocido: %q %v", texto, err)
	}
}
