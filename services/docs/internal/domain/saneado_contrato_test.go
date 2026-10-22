// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// EL SANEADOR NO CAMBIA UN BYTE DE LO LEGÍTIMO (XSS-01). Lo legítimo es lo que guarda el
// editor y lo que produce la importación: si el saneador tocara algo de eso, el registro y
// la coedición por carácter divergirían en cada guardado, y un fichero importado se
// guardaría distinto de como se convirtió.
//
// Las plantillas y los bloques los mira plantillas.TestLasPlantillasPasanElSaneadoSinCambiarUnByte.

// difiere dice dónde empiezan a diferir dos cadenas, con un poco de contexto, para que el
// fallo diga qué se tocó sin volcar un documento entero.
func difiere(antes, despues string) string {
	i := 0
	for i < len(antes) && i < len(despues) && antes[i] == despues[i] {
		i++
	}
	tramo := func(s string) string {
		desde, hasta := max(0, i-60), min(len(s), i+120)
		return s[desde:hasta]
	}
	return "en el byte " + strconv.Itoa(i) + "\n   era: …" + tramo(antes) + "…\n queda: …" + tramo(despues) + "…"
}

// Un documento de cada tipo tal como lo guarda el editor. El fichero lo escribe y lo vigila
// apps/frontend/apps/docs/test/saneado-lienzos.test.ts, a partir de las muestras del propio
// editor (cada bloque que hornea, el documento, la hoja y la presentación de las superficies
// de desarrollo): si el editor cambia lo que escribe, esa prueba pide regenerarlo.
func TestUnDocumentoDeCadaTipoDelEditorPasaElSaneadoSinCambiarUnByte(t *testing.T) {
	datos, err := os.ReadFile(filepath.Join("testdata", "saneado", "editor.json"))
	if err != nil {
		t.Fatal(err)
	}
	var porClave map[string]string
	if err := json.Unmarshal(datos, &porClave); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []Kind{KindDocument, KindSpreadsheet, KindPresentation} {
		clave := ContentKey(kind)
		contenido := porClave[clave]
		if len(contenido) < 1000 {
			t.Fatalf("%s: el fichero no trae un documento de verdad (%d bytes)", clave, len(contenido))
		}
		if limpio := SanearContenido(clave, contenido); limpio != contenido {
			t.Errorf("%s del editor cambia al sanearlo, %s", clave, difiere(contenido, limpio))
		}
	}
}

// Lo que produce la importación de los ficheros del corpus de fidelidad y de Calcus. Si esto
// falla, la importación emite algo fuera del contrato (saneado_html.go): manda el contrato,
// y lo que hay que cambiar es el importador.
func TestLoQueProduceLaImportacionPasaElSaneadoSinCambiarUnByte(t *testing.T) {
	type fichero struct {
		nombre string
		datos  []byte
	}
	extra, _ := filepath.Glob(filepath.Join("testdata", "calcus", "*"))
	avanzada, _ := filepath.Glob(filepath.Join("testdata", "fidelidad", "libreoffice", "diapos-avanzada.*"))
	ficheros := make([]fichero, 0, len(corpus)+len(extra)+len(avanzada))
	for _, m := range corpus {
		ficheros = append(ficheros, fichero{filepath.Base(m.ruta) + "." + m.ext, leerMuestra(t, m)})
	}
	for _, ruta := range append(extra, avanzada...) {
		b, err := os.ReadFile(ruta)
		if err != nil {
			t.Fatal(err)
		}
		ficheros = append(ficheros, fichero{filepath.Base(ruta), b})
	}
	if len(ficheros) < 15 {
		t.Fatalf("solo %d ficheros: el recorrido no encuentra el corpus", len(ficheros))
	}
	for _, f := range ficheros {
		res, err := ImportWithComments(f.nombre, f.datos)
		if err != nil {
			t.Errorf("%s: %v", f.nombre, err)
			continue
		}
		clave := ContentKey(res.Kind)
		if limpio := SanearContenido(clave, res.Content); limpio != res.Content {
			t.Errorf("%s importado cambia al sanearlo (%s), %s", f.nombre, clave, difiere(res.Content, limpio))
		}
	}
}

// El corpus de .docx reales del frontal (el de TestCorpusDocx), si se apunta: lo que el
// importador de Word saca de ficheros que no escribimos nosotros.
//
//	SYTUE_CORPUS_DOCX=/ruta/al/corpus go test ./services/docs/internal/domain -run TestElCorpusDocx
func TestElCorpusDocxPasaElSaneadoSinCambiarUnByte(t *testing.T) {
	dir := os.Getenv("SYTUE_CORPUS_DOCX")
	if dir == "" {
		t.Skip("sin SYTUE_CORPUS_DOCX")
	}
	var rutas []string
	_ = filepath.WalkDir(dir, func(p string, e fs.DirEntry, err error) error {
		if err == nil && !e.IsDir() && strings.HasSuffix(strings.ToLower(p), ".docx") {
			rutas = append(rutas, p)
		}
		return nil
	})
	if len(rutas) == 0 {
		t.Fatalf("no hay .docx en %s", dir)
	}
	cambian := 0
	for _, ruta := range rutas {
		datos, err := os.ReadFile(ruta)
		if err != nil {
			t.Fatal(err)
		}
		res, err := ImportWithComments(filepath.Base(ruta), datos)
		if err != nil {
			continue // lo que el importador no abre lo mide TestCorpusDocx
		}
		if limpio := SanearContenido("doc:html", res.Content); limpio != res.Content {
			cambian++
			t.Errorf("%s cambia al sanearlo, %s", filepath.Base(ruta), difiere(res.Content, limpio))
		}
	}
	t.Logf("%d .docx, %d cambian al sanearlos", len(rutas), cambian)
}
