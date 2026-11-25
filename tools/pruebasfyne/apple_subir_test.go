// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package pruebasfyne

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/tools/guiontest"
)

// La respuesta de App Store Connect cuando el número de build ya está subido.
const appleDuplicado = `"code" : "ENTITY_ERROR.ATTRIBUTE.INVALID.DUPLICATE",
"NSLocalizedFailureReason" : "The bundle version must be higher than the previously uploaded version: ‘122’."`

// appleSubir corre `apple-firma.sh subir` sobre un .ipa con un app-store-connect
// falso que contesta `respuesta` y sale con `codigo`.
func appleSubir(t *testing.T, intento, respuesta string, codigo int) guiontest.Resultado {
	t.Helper()
	dir := t.TempDir()
	ipa := filepath.Join(dir, "sytue-hub-ios.ipa")
	if err := os.WriteFile(ipa, []byte("ipa"), 0o600); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(dir, "bin")
	if err := os.Mkdir(bin, 0o755); err != nil { //nolint:gosec // G301: directorio de dobles de la prueba.
		t.Fatal(err)
	}
	guiontest.Escribir(t, filepath.Join(dir, "respuesta"), respuesta)
	guiontest.Doble(t, bin, "app-store-connect", `cat "`+filepath.Join(dir, "respuesta")+`"; exit `+strconv.Itoa(codigo))
	return guiontest.Guion(t, guiontest.Opciones{Env: []string{
		guiontest.ConPATH(bin), "GITHUB_RUN_ATTEMPT=" + intento,
		"APP_STORE_CONNECT_ISSUER_ID=i", "APP_STORE_CONNECT_KEY_IDENTIFIER=k",
		"APP_STORE_CONNECT_PRIVATE_KEY=p", "CERTIFICATE_PRIVATE_KEY=c",
	}}, "apps/fyne/scripts/apple-firma.sh", "subir", ipa)
}

// Un relanzamiento de release-desktop repite el número de build (el de la ejecución)
// con el mismo commit: lo que el intento anterior subió vuelve como duplicado y no
// es un fallo. Pasó en la 45693645649, con las quince apps de iOS ya en TestFlight.
func TestAppleSubirEnUnRelanzamientoElDuplicadoYaEstaSubido(t *testing.T) {
	r := appleSubir(t, "2", appleDuplicado, 1)
	if r.Codigo != 0 || !strings.Contains(r.Todo(), "ya estaba en App Store Connect") {
		t.Fatalf("en el segundo intento, el duplicado cuenta como subido (código %d)\n%s", r.Codigo, r.Todo())
	}
}

// En el primer intento un duplicado sí es un fallo: ese build lo subió otro.
func TestAppleSubirEnElPrimerIntentoElDuplicadoEsRojo(t *testing.T) {
	r := appleSubir(t, "1", appleDuplicado, 1)
	if r.Codigo == 0 || !strings.Contains(r.Todo(), "App Store Connect no aceptó: sytue-hub-ios.ipa") {
		t.Fatalf("en el primer intento, un duplicado es rojo (código %d)\n%s", r.Codigo, r.Todo())
	}
}

// Otro rechazo de Apple es rojo aunque sea un relanzamiento.
func TestAppleSubirOtroRechazoEsRojoTambienAlRelanzar(t *testing.T) {
	r := appleSubir(t, "3", `"code" : "STATE_ERROR.VALIDATION_ERROR"`, 1)
	if r.Codigo == 0 || !strings.Contains(r.Todo(), "App Store Connect no aceptó: sytue-hub-ios.ipa") {
		t.Fatalf("otro rechazo es rojo (código %d)\n%s", r.Codigo, r.Todo())
	}
}

// Lo que sube a la primera sigue diciéndolo, con la salida de la herramienta.
func TestAppleSubirLoQueEntraSeDice(t *testing.T) {
	r := appleSubir(t, "1", "UPLOAD SUCCEEDED with no errors", 0)
	if r.Codigo != 0 || !strings.Contains(r.Todo(), "en App Store Connect (TestFlight, pruebas internas)") ||
		!strings.Contains(r.Todo(), "UPLOAD SUCCEEDED") {
		t.Fatalf("la subida que entra (código %d)\n%s", r.Codigo, r.Todo())
	}
}
