// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package publicar

import (
	"archive/zip"
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
	"time"
)

func TestEscribirZipCorrecto(t *testing.T) {
	archivos := []ArchivoZip{
		{
			Ruta:       "package.json",
			Contenido:  []byte(`{"name":"test"}`),
			Modificado: time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC),
		},
		{
			Ruta:       "src/pages/es/index.astro",
			Contenido:  []byte("--- \n--- \n<h1>Hola</h1>"),
			Modificado: time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC),
		},
	}
	recursos := []RecursoZip{
		{
			Ruta:       "public/_astro/style.css",
			Lector:     strings.NewReader("body { margin: 0; }"),
			Modificado: time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC),
		},
	}

	var buf bytes.Buffer
	if err := EscribirZip(&buf, archivos, recursos); err != nil {
		t.Fatalf("EscribirZip falló: %v", err)
	}

	zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatalf("no se pudo abrir el zip generado: %v", err)
	}

	if len(zr.File) != 3 {
		t.Fatalf("se esperaban 3 ficheros en el zip, obtenidos %d", len(zr.File))
	}

	encontrados := map[string]string{}
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("error abriendo %s: %v", f.Name, err)
		}
		data, _ := io.ReadAll(rc)
		_ = rc.Close()
		encontrados[f.Name] = string(data)
	}

	if encontrados["package.json"] != `{"name":"test"}` {
		t.Errorf("contenido inesperado para package.json: %q", encontrados["package.json"])
	}
	if encontrados["public/_astro/style.css"] != "body { margin: 0; }" {
		t.Errorf("contenido inesperado para style.css: %q", encontrados["public/_astro/style.css"])
	}
}

func TestEscribirZipDefensaZipSlip(t *testing.T) {
	rutasPeligrosas := []string{
		"../escape.txt",
		"/absoluta/ruta.txt",
		"carpeta/../../escape.txt",
		"carpeta/./actual.txt",
		"./directo.txt",
		"..",
		".",
		"ruta con espacios/fichero.txt",
		"ruta/con/caracter;malo.txt",
		"ruta/con/$variable.txt",
		"ruta/con/|tubo.txt",
		"ruta//doble/barra.txt",
		"",
	}

	for _, ruta := range rutasPeligrosas {
		err := ValidarRutaZip(ruta)
		if err == nil {
			t.Errorf("ValidarRutaZip debió fallar para ruta peligrosa %q", ruta)
		}
	}
}

func TestEscribirZipLimiteLongitudRuta(t *testing.T) {
	rutaLarga := strings.Repeat("a", 301)
	err := ValidarRutaZip(rutaLarga)
	if err == nil || !errors.Is(err, ErrRutaInvalida) {
		t.Errorf("debía fallar por longitud > 300: %v", err)
	}

	rutaExacta300 := strings.Repeat("a", 300)
	if err := ValidarRutaZip(rutaExacta300); err != nil {
		t.Errorf("ruta de 300 caracteres válida no debió fallar: %v", err)
	}
}

func TestEscribirZipLimiteFicheros(t *testing.T) {
	archivos := make([]ArchivoZip, 2001)
	for i := 0; i < 2001; i++ {
		archivos[i] = ArchivoZip{
			Ruta:      "f.txt",
			Contenido: []byte("a"),
		}
	}

	var buf bytes.Buffer
	err := EscribirZip(&buf, archivos, nil)
	if err == nil || !errors.Is(err, ErrDemasiadosFicheros) {
		t.Fatalf("debía fallar con ErrDemasiadosFicheros, obtenido: %v", err)
	}
}
