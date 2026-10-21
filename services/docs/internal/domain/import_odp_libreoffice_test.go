// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"io"
	"math/rand/v2"
	"os"
	"strings"
	"testing"
)

// Los .odp de testdata/odf son de LibreOffice de verdad (su corpus de pruebas de
// Impress, 7.1 y 7.3): lo que llega a Drive desde el escritorio de la gente, y no lo
// que escribe nuestro exportador.

type modeloImportado struct {
	Slides []struct {
		Boxes []struct {
			X, Y, W, H float64
			Kind, Src  string
		} `json:"boxes"`
	} `json:"slides"`
	Size *outSize `json:"size"`
}

func importarODP(t *testing.T, nombre string, datos []byte) (*ImportResult, modeloImportado) {
	t.Helper()
	res, err := Import(nombre, datos)
	if err != nil {
		t.Fatalf("%s no se importa: %v", nombre, err)
	}
	var m modeloImportado
	if err := json.Unmarshal([]byte(res.Content), &m); err != nil {
		t.Fatal(err)
	}
	return res, m
}

// TestLasCajasDeUnODPDeLibreOfficeQuedanEnSuDiapositiva: las cajas van en % de la
// página de la diapositiva que el fichero declara (28 × 15,75 cm en Impress), y no de
// un lienzo fijo; y esa página es la de la diapositiva, no la de las notas, que en
// `styles.xml` va PRIMERO (21 × 29,7).
//
// VISTO EN ROJO (2026-09-24, en el frontal): con el lienzo fijo de 25,4 × 14,288 las
// veinte cajas de impress-7-1.odp se salían (hasta el 113 % de alto); leyendo el primer
// page-layout, en vez del de su página maestra, salían a lo alto de un A4.
func TestLasCajasDeUnODPDeLibreOfficeQuedanEnSuDiapositiva(t *testing.T) {
	for _, f := range []string{"impress-7-1.odp", "impress-7-3.odp"} {
		datos, err := os.ReadFile("testdata/odf/" + f)
		if err != nil {
			t.Fatal(err)
		}
		_, m := importarODP(t, f, datos)
		if m.Size != nil {
			t.Errorf("%s es 16:9 y sale con el tamaño %+v", f, *m.Size)
		}
		cajas := 0
		for i, s := range m.Slides {
			for _, c := range s.Boxes {
				cajas++
				if c.X < -0.5 || c.Y < -0.5 || c.X+c.W > 100.5 || c.Y+c.H > 100.5 {
					t.Errorf("%s, diapositiva %d: la caja %+v se sale", f, i+1, c)
				}
			}
		}
		if cajas == 0 {
			t.Errorf("%s no trae ninguna caja", f)
		}
	}
	if s := tamanoDiapositiva(28, 21); s == nil || s.Preset != "4:3" {
		t.Errorf("28 × 21 es 4:3 y sale %+v", s)
	}
	if s := tamanoDiapositiva(20, 10); s == nil || s.Preset != "custom" || s.W != 20 || s.H != 10 || s.Unit != "cm" {
		t.Errorf("20 × 10 es propio, en cm, y sale %+v", s)
	}
}

// conFoto es el .odp con una foto de ~2 MB en su primera diapositiva, metida como la
// mete Impress: el fichero en Pictures/, su entrada en el manifiesto y un draw:frame con
// su draw:image. El ruido no se comprime, como una foto de verdad.
func conFoto(t *testing.T, odp []byte) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 1800, 1200))
	r := rand.New(rand.NewPCG(1, 2))
	for i := range img.Pix {
		img.Pix[i] = uint8(r.IntN(256))
		if i%4 == 3 {
			img.Pix[i] = 255
		}
	}
	img.Set(0, 0, color.RGBA{255, 0, 0, 255})
	var foto bytes.Buffer
	if err := jpeg.Encode(&foto, img, &jpeg.Options{Quality: 90}); err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(bytes.NewReader(odp), int64(len(odp)))
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	zw := zip.NewWriter(&out)
	marco := `<draw:frame svg:width="12cm" svg:height="8cm" svg:x="14cm" svg:y="6cm">` +
		`<draw:image xlink:href="Pictures/foto.jpg" xlink:type="simple" xlink:show="embed" xlink:actuate="onLoad"/></draw:frame>`
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(rc)
		_ = rc.Close()
		switch f.Name {
		case "content.xml":
			b = []byte(strings.Replace(string(b), "</draw:page>", marco+"</draw:page>", 1))
		case "META-INF/manifest.xml":
			b = []byte(strings.Replace(string(b), "</manifest:manifest>",
				`<manifest:file-entry manifest:full-path="Pictures/foto.jpg" manifest:media-type="image/jpeg"/></manifest:manifest>`, 1))
		}
		w, err := zw.CreateHeader(&zip.FileHeader{Name: f.Name, Method: f.Method})
		if err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write(b)
	}
	w, _ := zw.CreateHeader(&zip.FileHeader{Name: "Pictures/foto.jpg", Method: zip.Store})
	_, _ = w.Write(foto.Bytes())
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

// TestUnODPConFotosCabeEnUnaOperacion es EL guardián de «un .odp no se abre desde
// Drive»: lo convertido se siembra con una sola operación y el cuerpo de una operación no
// pasa de 1 MiB, así que con una foto de verdad dentro la conversión salía bien y sembrarla
// daba 413. Lo convertido cabe, la foto sigue ahí —reducida, en JPEG— y en su sitio.
//
// VISTO EN ROJO (2026-09-24, en el frontal): sin `caberEnElDocumento`, lo convertido
// medía 2 588 242 caracteres.
func TestUnODPConFotosCabeEnUnaOperacion(t *testing.T) {
	odp, err := os.ReadFile("testdata/odf/impress-7-3.odp")
	if err != nil {
		t.Fatal(err)
	}
	gordo := conFoto(t, odp)
	res, m := importarODP(t, "con-foto.odp", gordo)
	if n := tamanoSembrado(res.Content); n > MaxContenidoSembrado {
		t.Fatalf("lo convertido mide %d y una operación admite %d", n, MaxContenidoSembrado)
	}
	var foto bool
	for _, c := range m.Slides[0].Boxes {
		if c.Kind == "image" {
			foto = strings.HasPrefix(c.Src, "data:image/jpeg;base64,")
			if c.X < 49 || c.X > 51 || c.Y+c.H > 100.5 {
				t.Errorf("la foto va en x=%v (quiero 50 %%: 14 de 28 cm) y acaba en %v %%", c.X, c.Y+c.H)
			}
		}
	}
	if !foto {
		t.Error("la foto se ha perdido por el camino")
	}
	// Y lo que ya cabía no se toca: ni un byte distinto.
	chico, _ := importarODP(t, "sin-foto.odp", odp)
	if again, err := caberEnElDocumento(chico.Content); err != nil || again != chico.Content {
		t.Error("un contenido que cabe se ha reescrito")
	}
}

// TestUnDocumentoConFotosTambienCabe: el presupuesto es de la pieza común, no del
// formato. Un .odt o un .docx con una foto (HTML de Docus, `<img src="data:…">`) pasaba
// igual del MiB al sembrarse, y sale igual de encogido.
func TestUnDocumentoConFotosTambienCabe(t *testing.T) {
	odp, err := os.ReadFile("testdata/odf/impress-7-3.odp")
	if err != nil {
		t.Fatal(err)
	}
	// La foto de conFoto, sin encoger, dentro de un documento.
	foto := ""
	grande := conFoto(t, odp)
	zr, _ := zip.NewReader(bytes.NewReader(grande), int64(len(grande)))
	for _, f := range zr.File {
		if f.Name == "Pictures/foto.jpg" {
			rc, _ := f.Open()
			b, _ := io.ReadAll(rc)
			_ = rc.Close()
			foto = "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(b)
		}
	}
	html := `<h1>Informe</h1><p>Antes <img src="` + foto + `" alt="foto"> después</p>`
	cabe, err := caberEnElDocumento(html)
	if err != nil || tamanoSembrado(cabe) > MaxContenidoSembrado {
		t.Fatalf("un documento con una foto de %d caracteres no cabe: %v", len(foto), err)
	}
	if !strings.HasPrefix(cabe, `<h1>Informe</h1><p>Antes <img src="data:image/jpeg;base64,`) || !strings.HasSuffix(cabe, `" alt="foto"> después</p>`) {
		t.Error("al encoger la foto se ha tocado el resto del documento")
	}
}

// TestLoQueNoCabeNiAlMinimoSeDice: sin imágenes que encoger, lo que no cabe es un error
// que la capa HTTP convierte en 413, y no un documento que luego no se puede sembrar.
func TestLoQueNoCabeNiAlMinimoSeDice(t *testing.T) {
	enorme := "<p>" + strings.Repeat("palabra ", MaxContenidoSembrado/7) + "</p>"
	if _, err := caberEnElDocumento(enorme); !errors.Is(err, ErrDemasiadoGrande) {
		t.Errorf("un texto que no cabe da %v", err)
	}
}
