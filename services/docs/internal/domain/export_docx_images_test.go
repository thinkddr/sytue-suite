// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"bytes"
	"encoding/base64"
	"strings"
	"testing"
)

func TestOfficeImagenEmbebidaConservaBytesYTamano(t *testing.T) {
	src := "data:image/png;base64," + base64.StdEncoding.EncodeToString(pngBytes(t))
	h := `<p>Antes<img src="` + src + `" data-w="144" data-h="72" alt="Riego &amp; agua">Después</p>`
	for _, caso := range []struct {
		format                     BlobFormat
		ext, media, content, types string
	}{
		{FormatOOXML, "docx", "word/media/image3.png", "word/document.xml", "[Content_Types].xml"},
		{FormatODF, "odt", "Pictures/inline1.png", "content.xml", "META-INF/manifest.xml"},
	} {
		t.Run(caso.ext, func(t *testing.T) {
			data, _, _, err := Export(KindDocument, "", stateWithContent(t, "doc:html", h), caso.format)
			if err != nil {
				t.Fatal(err)
			}
			files, err := readZip(data)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(files[caso.media], pngBytes(t)) {
				t.Fatal("la imagen no viajó sin modificar sus bytes")
			}
			if !strings.Contains(string(files[caso.content]), `Riego &amp; agua`) {
				t.Fatal("se perdió el texto alternativo")
			}
			if !strings.Contains(string(files[caso.types]), `image/png`) {
				t.Fatal("falta el tipo de la imagen en el paquete")
			}
			res, err := Import("imagen."+caso.ext, data)
			if err != nil {
				t.Fatal(err)
			}
			for _, want := range []string{src, `data-w="144"`, `data-h="72"`, "Antes", "Después"} {
				mustContain(t, "imagen ida y vuelta", res.Content, want)
			}
		})
	}
}

func TestDOCXNoEmbebeURLsExternasNiImagenesInvalidas(t *testing.T) {
	for _, src := range []string{"https://example.invalid/secreto.png", "file:///etc/passwd", "data:image/svg+xml;base64,PHN2Zz4=", "data:image/png;base64,AAAA", strings.Replace(onePxPNG, "image/png", "image/jpeg", 1)} {
		data, _, _, err := Export(KindDocument, "", stateWithContent(t, "doc:html", `<p><img src="`+src+`">Texto</p>`), FormatOOXML)
		if err != nil {
			t.Fatal(err)
		}
		files, err := readZip(data)
		if err != nil {
			t.Fatal(err)
		}
		for name := range files {
			if strings.HasPrefix(name, "word/media/") {
				t.Fatalf("se embebió %q", src)
			}
		}
	}
}
