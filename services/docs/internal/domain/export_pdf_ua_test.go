// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/pdfdoc/uacheck"
)

// pngValido es un PNG de verdad (el onePxPNG de los otros tests solo pasa DecodeConfig: el
// decodificador de la stdlib lo rechaza por «too much pixel data»).
func pngValido(t *testing.T) string {
	t.Helper()
	im := image.NewNRGBA(image.Rect(0, 0, 4, 4))
	im.SetNRGBA(1, 1, color.NRGBA{R: 200, A: 255})
	var b bytes.Buffer
	if err := png.Encode(&b, im); err != nil {
		t.Fatal(err)
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(b.Bytes())
}

const htmlAccesible = `<h1>Informe</h1><p>Texto con <a href="https://sytue.eu/a?x=1&amp;y=2">un enlace</a>, <strong>negrita</strong> y ` +
	`<span lang="fr">un tramo en francés</span>. Y en griego: προσβασιμότητα.</p>` +
	`<h4>Salto de nivel</h4><ul><li>Uno<ul><li>Uno-uno</li></ul></li><li>Dos</li></ul><ol><li>Primero</li><li>Segundo</li></ol>` +
	`<table><tr><th colspan="2">Cabecera</th><th rowspan="2">Aparte</th></tr><tr><td>a</td><td>b</td></tr><tr><td>c</td><td>d</td><td>e</td></tr></table>` +
	`<p><img alt="Un pixel rojo" src="%[1]s"> y otra <img src="%[1]s"> sin alt y <img alt="externa" src="https://example.invalid/x.png"></p><hr>` +
	`<blockquote>Una cita</blockquote><p>Fin</p>`

func TestPDFAccesibleDeUnDocumento(t *testing.T) {
	st := stateWithContent(t, "doc:html", strings.ReplaceAll(htmlAccesible, "%[1]s", pngValido(t)))
	data, inf, err := ExportPDFAccesible(KindDocument, "Informe de prueba", "es", st)
	if err != nil {
		t.Fatal(err)
	}
	if dir := os.Getenv("SYTUE_PDF_VOLCAR"); dir != "" { // para pasarlo por veraPDF y pdftoppm
		_ = os.WriteFile(filepath.Join(dir, "docus-accesible.pdf"), data, 0o600)
	}
	cuerpo := string(data)
	for _, quiero := range []string{
		"%PDF-1.7", "/Type /StructTreeRoot", "/MarkInfo << /Marked true >>", "/DisplayDocTitle true",
		"<pdfuaid:part>1</pdfuaid:part>", "/Lang (es)", "/Lang (fr)", "/Lang (el)",
		"/S /H1 ", "/S /H2 ", "/S /L ", "/S /LI ", "/S /Lbl ", "/S /LBody ", "/S /Link ", "/S /BlockQuote ",
		"/S /Table ", "/S /TR ", "/S /TH ", "/S /TD ", "/ColSpan 2", "/RowSpan 2", "/Scope /Column",
		"/S /Figure ", "/Alt (Un pixel rojo)", "/Subtype /Image",
	} {
		if !strings.Contains(cuerpo, quiero) {
			t.Errorf("falta %q", quiero)
		}
	}
	if errs := uacheck.Verificar(data); len(errs) > 0 {
		t.Errorf("verificador PDF/UA: %v", errs)
	}
	if fallos, err := uacheck.VeraPDF(data); err == nil && len(fallos) > 0 {
		t.Errorf("veraPDF PDF/UA-1: %v", fallos)
	}
	if strings.Contains(cuerpo, "/S /H4") {
		t.Error("el salto H1→H4 debe etiquetarse H2")
	}
	if inf.ImagenesSinAlt != 1 || inf.ImagenesPerdidas != 1 {
		t.Errorf("informe = %+v, esperaba 1 imagen sin alt y 1 perdida (externa)", inf)
	}
}

func TestPDFAccesibleDeUnDibujoLlevaSuRespaldoConAlt(t *testing.T) {
	data, _, err := ExportPDFAccesible(KindDocument, "", "es", stateWithContent(t, "doc:html", htmlDibujo(pngValido(t))))
	if err != nil {
		t.Fatal(err)
	}
	cuerpo := string(data)
	mustContain(t, "pdf", cuerpo, "/S /Figure")
	mustContain(t, "pdf", cuerpo, "/Alt (Tres pasos hasta la alta)")
	if strings.Contains(cuerpo, "ROTULO-DEL-SVG") || strings.Contains(cuerpo, "ESCENA-JSON") {
		t.Fatal("el SVG o el JSON salieron como texto")
	}
}

func TestPDFAccesibleDeHojasYPresentaciones(t *testing.T) {
	for _, kind := range []Kind{KindSpreadsheet, KindPresentation} {
		data, _, err := ExportPDFAccesible(kind, "Título", "es", &State{})
		if err != nil {
			t.Fatal(kind, err)
		}
		mustContain(t, string(kind), string(data), "/Type /StructTreeRoot")
	}
	if _, _, err := ExportPDFAccesible(Kind("otro"), "", "es", &State{}); err == nil {
		t.Error("un tipo desconocido debe fallar")
	}
}

// La comprobación de CertiPDF reconoce lo que escribe el exportador accesible: estructura,
// idioma, fuentes incrustadas y la identificación PDF/UA-1; y no se la atribuye al PDF simple.
func TestCertiPDFReconoceElPDFAccesible(t *testing.T) {
	st := stateWithContent(t, "doc:html", `<h1>Título</h1><p>Texto <a href="https://sytue.eu">enlace</a></p>`)
	accesible, _, err := ExportPDFAccesible(KindDocument, "Informe", "es", st)
	if err != nil {
		t.Fatal(err)
	}
	d, err := LeerPDF(accesible)
	if err != nil {
		t.Fatal(err)
	}
	inf := d.ComprobarArchivo()
	if !inf.CumplePDFUA || !inf.DeclaraPDFUA {
		t.Errorf("el PDF accesible debería cumplir y declarar PDF/UA: %+v", inf)
	}
	for _, p := range inf.Problemas {
		if p.Codigo == "fuenteSinIncrustar" || p.Codigo == "sinEstructura" || p.Codigo == "sinIdioma" || p.Codigo == "sinMetadatosXMP" {
			t.Errorf("problema inesperado: %+v", p)
		}
	}
	simple, _, _, err := Export(KindDocument, "Informe", st, FormatPDF)
	if err != nil {
		t.Fatal(err)
	}
	d2, err := LeerPDF(simple)
	if err != nil {
		t.Fatal(err)
	}
	if i2 := d2.ComprobarArchivo(); i2.CumplePDFUA || i2.DeclaraPDFUA {
		t.Errorf("el PDF simple no es accesible: %+v", i2)
	}
}
