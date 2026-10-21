// SPDX-License-Identifier: Apache-2.0
package pageshttp

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"mime/multipart"
	"testing"
)

func TestMultipartConservaRutasDeIdiomasYAssets(t *testing.T) {
	var cuerpo bytes.Buffer
	writer := multipart.NewWriter(&cuerpo)
	contenido := []byte("<h1>Hello</h1>")
	hash := sha256.Sum256(contenido)
	rutas := []string{"index.html", "en/index.html", "assets/tema.css"}
	manifiesto := ManifiestoPublicacion{}
	for _, ruta := range rutas {
		manifiesto.Files = append(manifiesto.Files, FicheroManifiesto{Path: ruta, SHA256: hex.EncodeToString(hash[:]), Bytes: int64(len(contenido))})
	}
	manifest, err := writer.CreateFormField("manifest")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.NewEncoder(manifest).Encode(manifiesto); err != nil {
		t.Fatal(err)
	}
	for _, ruta := range rutas {
		fichero, err := writer.CreateFormFile("file", ruta)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := fichero.Write(contenido); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	_, ficheros, total, err := procesarMultipartPublicacion(multipart.NewReader(&cuerpo, writer.Boundary()))
	if err != nil {
		t.Fatal(err)
	}
	if len(ficheros) != len(rutas) || total != int64(len(rutas)*len(contenido)) {
		t.Fatalf("salida incompleta: %d ficheros, %d bytes", len(ficheros), total)
	}
	for i, fichero := range ficheros {
		if fichero.Ruta != rutas[i] {
			t.Errorf("ruta = %q; queremos %q", fichero.Ruta, rutas[i])
		}
	}
}
