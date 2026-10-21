// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// El bloque «Dibujo» de Docus (<figure data-dibujo>, ver apps/frontend/packages/office/src/editor/dibujo.ts)
// lleva su escena en JSON, un <svg> con el render y una imagen PNG de respaldo oculta. Los
// exportadores solo aceptan imágenes rasterizadas embebidas: el respaldo viaja como imagen y
// NI el SVG NI el JSON se vuelcan como texto suelto.
const figuraDibujo = `<p>Antes</p><figure data-dibujo data-escena="{&quot;v&quot;:1,&quot;secreto&quot;:&quot;ESCENA-JSON&quot;}" contenteditable="false" tabindex="0">` +
	`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 10 10" role="img" aria-label="Flujo"><title>Flujo de alta</title><desc>Tres pasos hasta la alta</desc>` +
	`<g><text>ROTULO-DEL-SVG</text></g></svg>` +
	`<img data-dibujo-respaldo hidden alt="Tres pasos hasta la alta" width="300" height="200" data-w="300" data-h="200" src="%s"></figure><p>Después</p>`

func htmlDibujo(respaldo string) string {
	if respaldo == "" {
		return strings.Replace(strings.Replace(figuraDibujo, `<img data-dibujo-respaldo hidden alt="Tres pasos hasta la alta" width="300" height="200" data-w="300" data-h="200" src="%s">`, "", 1), "%s", "", 1)
	}
	return strings.Replace(figuraDibujo, "%s", respaldo, 1)
}

func TestDibujoViajaComoImagenDeRespaldoAOfficeYNoComoTextoSuelto(t *testing.T) {
	for _, caso := range []struct {
		format        BlobFormat
		ext, media, c string
	}{
		{FormatOOXML, "docx", "word/media/image3.png", "word/document.xml"},
		{FormatODF, "odt", "Pictures/inline1.png", "content.xml"},
	} {
		t.Run(caso.ext, func(t *testing.T) {
			data, _, _, err := Export(KindDocument, "", stateWithContent(t, "doc:html", htmlDibujo(onePxPNG)), caso.format)
			if err != nil {
				t.Fatal(err)
			}
			files, err := readZip(data)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(files[caso.media], pngBytes(t)) {
				t.Fatalf("la imagen de respaldo no viajó (partes: %v)", claves(files))
			}
			cuerpo := string(files[caso.c])
			mustContain(t, caso.ext, cuerpo, "Tres pasos hasta la alta") // texto alternativo
			mustContain(t, caso.ext, cuerpo, "Antes")
			mustContain(t, caso.ext, cuerpo, "Después")
			for _, nunca := range []string{"ROTULO-DEL-SVG", "ESCENA-JSON", "Flujo de alta", "[dibujo"} {
				if strings.Contains(cuerpo, nunca) {
					t.Fatalf("%s no debía salir en el documento: %q", caso.ext, nunca)
				}
			}
		})
	}
}

func TestDibujoSinRespaldoDejaUnMarcadorConElTextoAlternativo(t *testing.T) {
	for _, format := range []BlobFormat{FormatOOXML, FormatODF} {
		data, _, _, err := Export(KindDocument, "", stateWithContent(t, "doc:html", htmlDibujo("")), format)
		if err != nil {
			t.Fatal(err)
		}
		files, err := readZip(data)
		if err != nil {
			t.Fatal(err)
		}
		for name := range files {
			if strings.HasPrefix(name, "word/media/") || strings.HasPrefix(name, "Pictures/") {
				t.Fatalf("no había imagen que embeber y salió %q", name)
			}
		}
		parte := "word/document.xml"
		if format == FormatODF {
			parte = "content.xml"
		}
		cuerpo := string(files[parte])
		mustContain(t, string(format), cuerpo, "[dibujo: Tres pasos hasta la alta]")
		if strings.Contains(cuerpo, "ROTULO-DEL-SVG") || strings.Contains(cuerpo, "ESCENA-JSON") {
			t.Fatalf("%s: el SVG o el JSON salieron como texto", format)
		}
	}
}

func TestDibujoEnPDFQuedaComoMarcadorConElTextoAlternativo(t *testing.T) {
	// El PDF de Docus es de texto: sin imágenes, el dibujo deja su marcador y NO el texto del SVG.
	data, _, _, err := Export(KindDocument, "", stateWithContent(t, "doc:html", htmlDibujo(onePxPNG)), FormatPDF)
	if err != nil {
		t.Fatal(err)
	}
	cuerpo := string(data)
	mustContain(t, "pdf", cuerpo, "dibujo: Tres pasos hasta la alta")
	if strings.Contains(cuerpo, "ROTULO-DEL-SVG") || strings.Contains(cuerpo, "ESCENA-JSON") {
		t.Fatal("el SVG o el JSON salieron como texto")
	}
}

func TestDibujoNoArrastraUnaFiguraAjena(t *testing.T) {
	// Una <figure> normal sigue recorriéndose como siempre (su texto sale).
	data, _, _, err := Export(KindDocument, "", stateWithContent(t, "doc:html", `<figure><p>Pie de figura</p></figure>`), FormatOOXML)
	if err != nil {
		t.Fatal(err)
	}
	files, err := readZip(data)
	if err != nil {
		t.Fatal(err)
	}
	mustContain(t, "figura", string(files["word/document.xml"]), "Pie de figura")
}

func claves(m map[string][]byte) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// --- SVG vectorial además del PNG (DOCX svgBlip, ODT con dos imágenes), HTML y Markdown ---

const figuraVectorial = `<p>Antes</p><figure data-dibujo data-dibujo-id="dib-1" data-escena="{&quot;v&quot;:1,&quot;ancho&quot;:300,&quot;alto&quot;:200,&quot;capas&quot;:[]}" contenteditable="false" tabindex="0">` +
	`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 300 200" width="100%" style="display:block;height:auto" role="img" aria-label="Flujo" onload="alert(1)">` +
	`<title>Flujo</title><desc>Tres pasos</desc><script>alert(2)</script>` +
	`<g data-capa="c1" role="group" aria-label="Capa 1"><rect x="10" y="10" width="80" height="40" rx="6" fill="currentColor" fill-opacity="0.08" stroke="currentColor" onclick="x()" style="fill:url(#x)"/>` +
	`<image href="https://evil.example/x.png" width="5" height="5"/><a href="javascript:alert(3)"><text>DENTRO-DEL-ENLACE</text></a><text font-size="14" fill="currentColor"><tspan x="50" y="30">A &amp; B</tspan></text>` +
	`<path d="M 0 0 L 10 10" fill="url(#g)" stroke="#112233"/><foreignObject><div>HTML</div></foreignObject></g></svg>` +
	`<img data-dibujo-respaldo hidden alt="Tres pasos" width="300" height="200" data-w="300" data-h="200" src="` + onePxPNG + `"></figure><p>Después</p>`

func exportaVectorial(t *testing.T, format BlobFormat) map[string][]byte {
	t.Helper()
	data, _, _, err := Export(KindDocument, "", stateWithContent(t, "doc:html", figuraVectorial), format)
	if err != nil {
		t.Fatal(err)
	}
	files, err := readZip(data)
	if err != nil {
		t.Fatal(err)
	}
	return files
}

func svgDe(t *testing.T, files map[string][]byte) string {
	t.Helper()
	for name, b := range files {
		if strings.HasSuffix(name, ".svg") {
			return string(b)
		}
	}
	t.Fatalf("no hay SVG en el paquete: %v", claves(files))
	return ""
}

func TestDibujoLlevaSuSVGVectorialSaneadoADocxYOdt(t *testing.T) {
	for _, format := range []BlobFormat{FormatOOXML, FormatODF} {
		files := exportaVectorial(t, format)
		svg := svgDe(t, files)
		for _, quiero := range []string{
			`viewBox="0 0 300 200"`, `width="300" height="200"`,
			`<rect x="10" y="10" width="80" height="40" rx="6" fill="#000000" fill-opacity="0.08" stroke="#000000"></rect>`,
			`A &amp; B`, `<metadata id="sytue-escena">{&quot;v&quot;:1,`, `stroke="#112233"`, `<desc>Tres pasos</desc>`,
		} {
			mustContain(t, string(format)+" svg", svg, quiero)
		}
		for _, nunca := range []string{"script", "onload", "onclick", "alert", "javascript", "evil.example", "<image", "foreignObject", "HTML", "url(", "style=", "currentColor", "<a ", "DENTRO-DEL-ENLACE"} {
			if strings.Contains(svg, nunca) {
				t.Fatalf("%s: el SVG exportado lleva %q:\n%s", format, nunca, svg)
			}
		}
		// Lo que cuelga de un elemento no admitido se va con él.
		mustContain(t, string(format)+" svg", svg, `<tspan x="50" y="30">`)
	}
}

func TestDocxDeclaraElSvgBlipConElPNGDeRespaldo(t *testing.T) {
	files := exportaVectorial(t, FormatOOXML)
	doc := string(files["word/document.xml"])
	mustContain(t, "docx", doc, `<asvg:svgBlip xmlns:asvg="http://schemas.microsoft.com/office/drawing/2016/SVG/main" r:embed="rId`)
	mustContain(t, "docx", doc, `uri="{96DAC541-7B7A-43D3-8B79-37D633B846F1}"`)
	mustContain(t, "tipos", string(files["[Content_Types].xml"]), `<Default Extension="svg" ContentType="image/svg+xml"/>`)
	rels := string(files["word/_rels/document.xml.rels"])
	if strings.Count(rels, "relationships/image") != 2 {
		t.Fatalf("quería dos relaciones de imagen (PNG y SVG):\n%s", rels)
	}
	png := 0
	for name := range files {
		if strings.HasPrefix(name, "word/media/") && strings.HasSuffix(name, ".png") {
			png++
		}
	}
	if png != 1 {
		t.Fatalf("PNG de respaldo: %d", png)
	}
}

func TestOdtPoneElSvgAntesDelPNGEnElMismoMarco(t *testing.T) {
	files := exportaVectorial(t, FormatODF)
	c := string(files["content.xml"])
	i, j := strings.Index(c, `.svg" xlink:type`), strings.Index(c, `.png" xlink:type`)
	if i < 0 || j < 0 || i > j || strings.Count(c, "<draw:frame") != 1 {
		t.Fatalf("marco con SVG y luego PNG:\n%s", c)
	}
	mustContain(t, "manifest", string(files["META-INF/manifest.xml"]), `.svg" manifest:media-type="image/svg+xml"`)
}

// Ida y vuelta: lo exportado por Sytue vuelve a ser un dibujo EDITABLE (escena intacta), con su
// SVG saneado y el PNG de respaldo, y no una imagen suelta ni dos.
func TestDibujoVuelveComoDibujoEditableAlImportar(t *testing.T) {
	for _, caso := range []struct {
		format BlobFormat
		ext    string
	}{{FormatOOXML, "docx"}, {FormatODF, "odt"}} {
		data, _, _, err := Export(KindDocument, "", stateWithContent(t, "doc:html", figuraVectorial), caso.format)
		if err != nil {
			t.Fatal(err)
		}
		res, err := Import("dibujo."+caso.ext, data)
		if err != nil {
			t.Fatal(err)
		}
		h := res.Content
		for _, quiero := range []string{
			`<figure data-dibujo data-escena="{&quot;v&quot;:1,&quot;ancho&quot;:300,&quot;alto&quot;:200,&quot;capas&quot;:[]}" contenteditable="false"`,
			`<img data-dibujo-respaldo hidden alt="Tres pasos"`, `src="` + onePxPNG + `"`, `viewBox="0 0 300 200" width="100%"`, "Antes", "Después",
		} {
			mustContain(t, caso.ext+" importado", h, quiero)
		}
		if strings.Count(h, "<img") != 1 || strings.Count(h, "<figure") != 1 {
			t.Fatalf("%s: quería una figura con una imagen de respaldo:\n%s", caso.ext, h)
		}
		for _, nunca := range []string{"script", "onload", "javascript", "metadata"} {
			if strings.Contains(h, nunca) {
				t.Fatalf("%s: el HTML importado lleva %q", caso.ext, nunca)
			}
		}
		// Punto fijo: volver a exportar lo importado da el mismo SVG.
		data2, _, _, err := Export(KindDocument, "", stateWithContent(t, "doc:html", h), caso.format)
		if err != nil {
			t.Fatal(err)
		}
		f1, _ := readZip(data)
		f2, _ := readZip(data2)
		if svgDe(t, f1) != svgDe(t, f2) {
			t.Fatalf("%s: el SVG cambia en la segunda vuelta:\n%s\n---\n%s", caso.ext, svgDe(t, f1), svgDe(t, f2))
		}
	}
}

// Un SVG de terceros (sin la escena de Sytue) NO se convierte en dibujo ni entra como <img>.
func TestUnSvgAjenoNoSeVuelveDibujo(t *testing.T) {
	if got := dibujoDesdeSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1 1"><script>x</script></svg>`), onePxPNG, 10, 10, ""); got != "" {
		t.Fatalf("un SVG ajeno salió como dibujo: %s", got)
	}
	hostil := `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 9 9"><metadata id="sytue-escena">{"v":1}</metadata><script>alert(1)</script><rect width="9" height="9" onclick="x()"/></svg>`
	got := dibujoDesdeSVG([]byte(hostil), onePxPNG, 9, 9, `a"><script>`)
	mustContain(t, "figura", got, `data-escena="{&quot;v&quot;:1}"`)
	if strings.Contains(got, "<script") || strings.Contains(got, "onclick") || strings.Contains(got, "alert") {
		t.Fatalf("el SVG importado no se saneó: %s", got)
	}
	if dibujoDesdeSVG([]byte(`<svg viewBox="0 0 9 9"><metadata id="sytue-escena">no es json</metadata></svg>`), "", 0, 0, "") != "" {
		t.Fatal("una escena que no es JSON no debe dar dibujo")
	}
}

func TestHTMLConservaElSVGDelDibujoYMarkdownDejaElTextoAlternativo(t *testing.T) {
	data, _, _, err := Export(KindDocument, "", stateWithContent(t, "doc:html", htmlDibujo(onePxPNG)), FormatHTML)
	if err != nil {
		t.Fatal(err)
	}
	mustContain(t, "html", string(data), `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 10 10"`)
	mustContain(t, "html", string(data), `<figure data-dibujo`)
	md, _, _, err := Export(KindDocument, "", stateWithContent(t, "doc:html", htmlDibujo(onePxPNG)), FormatMarkdown)
	if err != nil {
		t.Fatal(err)
	}
	mustContain(t, "markdown", string(md), "[dibujo: Tres pasos hasta la alta]")
	for _, nunca := range []string{"ROTULO-DEL-SVG", "ESCENA-JSON", "<svg"} {
		if strings.Contains(string(md), nunca) {
			t.Fatalf("markdown lleva %q:\n%s", nunca, md)
		}
	}
}

// Escribe un DOCX y un ODT con un dibujo real en SYTUE_DIBUJO_SALIDA para abrirlos con
// LibreOffice en el contenedor de los agentes (`soffice --convert-to pdf`). Sin la variable no
// hace nada.
func TestDibujoEscribeParaLibreOffice(t *testing.T) {
	dir := os.Getenv("SYTUE_DIBUJO_SALIDA")
	if dir == "" {
		t.Skip("solo con SYTUE_DIBUJO_SALIDA")
	}
	h := strings.Replace(figuraVectorial, `<a href="javascript:alert(3)"><text>DENTRO-DEL-ENLACE</text></a>`, "", 1)
	for format, ext := range map[BlobFormat]string{FormatOOXML: "docx", FormatODF: "odt"} {
		data, _, _, err := Export(KindDocument, "Dibujo", stateWithContent(t, "doc:html", h), format)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "dibujo."+ext), data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
}
