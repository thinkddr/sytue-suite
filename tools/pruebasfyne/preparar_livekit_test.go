// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package pruebasfyne

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/tools/guiontest"
)

// preparar-livekit.sh materializa el SDK de LiveKit fijado con el parche VP8
// revisado. Solo corre en CI remoto o en desarrollo efímero, como la propia
// compilación nativa: baja la fuente de proxy.golang.org.

// prepararLivekitCopia monta en un directorio temporal la parte del repositorio
// que lee el guion, para que la fuente generada (.deps) y las mutaciones no
// toquen el repositorio de verdad.
func prepararLivekitCopia(t *testing.T) string {
	t.Helper()
	raiz := guiontest.Raiz(t)
	tmp := t.TempDir()
	copiar := func(origen, destino string) {
		t.Helper()
		raw, err := os.ReadFile(filepath.Join(raiz, origen)) //nolint:gosec // G304: ficheros del repositorio.
		if err != nil {
			t.Fatal(err)
		}
		guiontest.Escribir(t, filepath.Join(tmp, destino), string(raw))
	}
	copiar("apps/fyne/scripts/preparar-livekit.sh", "apps/fyne/scripts/preparar-livekit.sh")
	parches, err := filepath.Glob(filepath.Join(raiz, "apps", "fyne", "patches", "*.patch"))
	if err != nil || len(parches) == 0 {
		t.Fatalf("no hay parches de apps/fyne/patches: %v", err)
	}
	for _, p := range parches {
		copiar(filepath.Join("apps", "fyne", "patches", filepath.Base(p)), filepath.Join("apps", "fyne", "patches", filepath.Base(p)))
	}
	copiar("deploy/sytue-git/versions.env", "deploy/sytue-git/versions.env")
	for _, b := range []string{"bajar-herramienta.sh", "descarga-reanudable.sh"} {
		copiar("deploy/sytue-git/bin/"+b, "deploy/sytue-git/bin/"+b)
	}
	copiar(".github/actions/toolchain/sha256.sh", ".github/actions/toolchain/sha256.sh")
	copiar("apps/fyne/go.mod", "apps/fyne/go.mod")
	return tmp
}

func TestPrepararLivekitParcheVerificadoYSusGuardas(t *testing.T) {
	tmp := prepararLivekitCopia(t)
	raiz := guiontest.Raiz(t)
	var salida strings.Builder
	preparar := func() int {
		t.Helper()
		r := guiontest.Guion(t, guiontest.Opciones{}, filepath.Join(tmp, "apps", "fyne", "scripts", "preparar-livekit.sh"))
		salida.WriteString(r.Todo())
		return r.Codigo
	}

	// Preparar dos veces: la segunda reutiliza la fuente ya verificada.
	for i := range 2 {
		if c := preparar(); c != 0 {
			t.Fatalf("preparar-livekit.sh falló en la pasada %d (código %d)\n%s", i+1, c, salida.String())
		}
	}
	e2ee := filepath.Join(tmp, ".deps", "livekit-server-sdk-go", "e2ee.go")
	if !strings.Contains(guiontest.Leer(t, e2ee), "CodecVP8") {
		t.Fatalf("e2ee.go no lleva el parche VP8 (falta CodecVP8)\n%s", salida.String())
	}

	// Editar la fuente tiene que fallar en vez de bendecir en silencio una caché cambiada.
	f, err := os.OpenFile(e2ee, os.O_APPEND|os.O_WRONLY, 0) //nolint:gosec // G304: fichero del directorio temporal.
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString("\n// modified fixture source\n"); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	if preparar() == 0 {
		t.Fatalf("aceptó la caché del SDK editada\n%s", salida.String())
	}
	if err := os.RemoveAll(filepath.Join(tmp, ".deps")); err != nil {
		t.Fatal(err)
	}

	// Otra versión del módulo tiene que fallar antes de descargar o parchear.
	gomod := guiontest.Leer(t, "apps/fyne/go.mod")
	otra := regexp.MustCompile(`server-sdk-go/v2 v[0-9.]*`).ReplaceAllString(gomod, "server-sdk-go/v2 v0.0.0")
	guiontest.Escribir(t, filepath.Join(tmp, "apps", "fyne", "go.mod"), otra)
	if preparar() == 0 {
		t.Fatalf("aceptó otra versión del módulo\n%s", salida.String())
	}
	guiontest.Escribir(t, filepath.Join(tmp, "apps", "fyne", "go.mod"), gomod)

	// Texto upstream retirado corrupto: la aplicación sin fuzz tiene que rechazarlo.
	parches, err := filepath.Glob(filepath.Join(tmp, "apps", "fyne", "patches", "*.patch"))
	if err != nil || len(parches) == 0 {
		t.Fatalf("sin parches en la copia: %v", err)
	}
	parche := parches[0]
	original := guiontest.Leer(t, parche)
	guiontest.Escribir(t, parche, strings.ReplaceAll(original, "unencrypted_audio_bytes", "missing_fixture_header"))
	if preparar() == 0 {
		t.Fatalf("aceptó un parche con contenido upstream equivocado\n%s", salida.String())
	}

	// Las comprobaciones de origen siguen activas aunque el parche sea correcto.
	guiontest.Escribir(t, parche, original)
	versiones := guiontest.Leer(t, filepath.Join(raiz, "deploy", "sytue-git", "versions.env"))
	ceros := regexp.MustCompile(`(?m)^LIVEKIT_GO_SOURCE_SHA256=.*$`).
		ReplaceAllString(versiones, "LIVEKIT_GO_SOURCE_SHA256="+strings.Repeat("0", 64))
	guiontest.Escribir(t, filepath.Join(tmp, "deploy", "sytue-git", "versions.env"), ceros)
	if preparar() == 0 {
		t.Fatalf("aceptó un SHA upstream incorrecto\n%s", salida.String())
	}
}
