// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package cicontratos

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"git.sytue.eu/sytue/one/tools/guiontest"
)

// seleccionar-openssl3.py elige un OpenSSL 3 YA instalado en el Mac sin
// instalar ni modificar nada. subprocess.run se sustituye en proceso: los
// «binarios» son ficheros de texto que nunca se ejecutan.

const seleccionarOpenSSL3Cargar = `
import subprocess
from unittest import mock
modulo = cargar("seleccionar_openssl3", APPLE / "seleccionar-openssl3.py")
`

// seleccionarOpenSSL3Fixture crea un fichero «openssl» con ese contenido y modo.
func seleccionarOpenSSL3Fixture(t *testing.T, ruta, contenido string, modo os.FileMode) {
	t.Helper()
	guiontest.Escribir(t, ruta, contenido)
	if err := os.Chmod(ruta, modo); err != nil {
		t.Fatal(err)
	}
}

func TestSeleccionarOpenSSL3RechazaLibreSSLYSeleccionaOpenSSL3SinInstalar(t *testing.T) {
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	primero := filepath.Join(base, "sistema", "openssl")
	segundo := filepath.Join(base, "existente", "openssl")
	for _, ruta := range []string{primero, segundo} {
		seleccionarOpenSSL3Fixture(t, ruta, "fixture, no ejecutar", 0o700)
	}
	var r struct {
		Elegido  string     `json:"elegido"`
		Llamadas [][]string `json:"llamadas"`
		Conf     []string   `json:"conf"`
		Devnull  string     `json:"devnull"`
	}
	pyRestoCorrer(t, seleccionarOpenSSL3Cargar+`
primero, segundo = ENTRADA
llamadas, conf = [], []

def responder(argv, **kwargs):
    llamadas.append(list(argv))
    conf.append(kwargs["env"]["OPENSSL_CONF"])
    return subprocess.CompletedProcess(argv, 0, b"LibreSSL 3.3.6\n" if argv[0] == primero else b"OpenSSL 3.6.0 1 Oct 2025\n")

with mock.patch.object(subprocess, "run", side_effect=responder):
    elegido = modulo.seleccionar([primero, segundo])
devolver({"elegido": str(elegido), "llamadas": llamadas, "conf": conf, "devnull": os.devnull})
`, &r, pyRestoConEntrada(t, []string{primero, segundo}))
	if r.Elegido != segundo {
		t.Fatalf("eligió %q, no el OpenSSL 3 %q", r.Elegido, segundo)
	}
	// Cada candidato se pregunta solo por `version` y sin leer la configuración del Mac.
	if !reflect.DeepEqual(r.Llamadas, [][]string{{primero, "version"}, {segundo, "version"}}) {
		t.Fatalf("llamadas %v", r.Llamadas)
	}
	if !reflect.DeepEqual(r.Conf, []string{r.Devnull, r.Devnull}) {
		t.Fatalf("OPENSSL_CONF %v, no %s", r.Conf, r.Devnull)
	}
}

func TestSeleccionarOpenSSL3NoEjecutaArchivoEscribiblePorOtrosNiRutaRelativa(t *testing.T) {
	ruta := filepath.Join(t.TempDir(), "openssl")
	seleccionarOpenSSL3Fixture(t, ruta, "no ejecutar", 0o777)
	var e *pyRestoExcepcion
	pyRestoCorrer(t, seleccionarOpenSSL3Cargar+`
ruta = ENTRADA
with mock.patch.object(subprocess, "run", side_effect=AssertionError("ejecutó un binario no confiable")):
    devolver(fallo(modulo.seleccionar, [ruta, "openssl", ruta + "\n"]))
`, &e, pyRestoConEntrada(t, ruta))
	// AssertionError del doble no es ValueError: si llegara a ejecutar algo, esto falla.
	pyRestoLanza(t, e, "ValueError", "")
}

func TestSeleccionarOpenSSL3NoAceptaOpenSSLAntiguoNiFalloDelEjecutable(t *testing.T) {
	ruta := filepath.Join(t.TempDir(), "openssl")
	seleccionarOpenSSL3Fixture(t, ruta, "fixture", 0o700)
	for nombre, resultado := range map[string]string{
		"antiguo": `subprocess.CompletedProcess([], 0, b"OpenSSL 1.1.1w\n")`,
		"codigo1": `subprocess.CompletedProcess([], 1, b"OpenSSL 3.6.0\n")`,
	} {
		t.Run(nombre, func(t *testing.T) {
			var e *pyRestoExcepcion
			pyRestoCorrer(t, seleccionarOpenSSL3Cargar+`
with mock.patch.object(subprocess, "run", return_value=`+resultado+`):
    devolver(fallo(modulo.seleccionar, [ENTRADA]))
`, &e, pyRestoConEntrada(t, ruta))
			pyRestoLanza(t, e, "ValueError", "")
		})
	}
}
