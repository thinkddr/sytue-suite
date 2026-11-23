// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package cicontratos

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"testing"

	"git.sytue.eu/sytue/one/tools/guiontest"
)

// Las pruebas de los módulos Python de .github corren en Go: cada una
// lanza python3 con un fragmento que carga el módulo REAL y lo llama (con los
// dobles en proceso que hagan falta, como hacía el unittest original) y deja en
// JSON lo observado; las comprobaciones son de Go.

// pyRestoPreambulo es lo que tienen todos los fragmentos: cargar un módulo por
// ruta, devolver lo observado y capturar una excepción sin perder su tipo. TMP
// es un directorio temporal de la prueba (t.TempDir), vacío y solo suyo.
const pyRestoPreambulo = `
import importlib.util, json, os, pathlib, sys
RAIZ = pathlib.Path(os.environ["PYRESTO_RAIZ"])
APPLE = RAIZ / ".github/actions/apple"
TMP = pathlib.Path(os.environ["PYRESTO_TMP"])
ENTRADA = json.loads(pathlib.Path(os.environ["PYRESTO_ENTRADA"]).read_text()) if os.environ.get("PYRESTO_ENTRADA") else None

def cargar(nombre, ruta):
    spec = importlib.util.spec_from_file_location(nombre, ruta)
    modulo = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(modulo)
    return modulo

def devolver(valor):
    with open(os.environ["PYRESTO_SALIDA"], "w", encoding="utf-8") as f:
        json.dump(valor, f, default=str, ensure_ascii=False)

def excepcion(e):
    return {"tipos": [c.__name__ for c in type(e).__mro__], "mensaje": str(e)}

def fallo(funcion, *args, **kwargs):
    try:
        funcion(*args, **kwargs)
    except Exception as e:
        return excepcion(e)
    return None
`

// pyRestoExcepcion es lo que devuelve `fallo` o `excepcion` en el fragmento.
type pyRestoExcepcion struct {
	Tipos   []string `json:"tipos"`
	Mensaje string   `json:"mensaje"`
}

// pyRestoCorrer ejecuta el fragmento con python3 desde la raíz del repositorio y
// decodifica en v lo que pasó a `devolver`. Un fragmento que no llega a
// devolver nada (excepción no prevista, python3 roto) es un fallo de la prueba.
func pyRestoCorrer(t *testing.T, fragmento string, v any, env ...string) guiontest.Resultado {
	t.Helper()
	return pyRestoCorrerCon(t, "python3", fragmento, v, env...)
}

// pyRestoCorrerCon es pyRestoCorrer con otro intérprete (el de un venv, por ejemplo).
func pyRestoCorrerCon(t *testing.T, python, fragmento string, v any, env ...string) guiontest.Resultado {
	t.Helper()
	dir := t.TempDir()
	salida := filepath.Join(dir, "salida.json")
	r := guiontest.Ejecutar(t, guiontest.Opciones{
		Env: append([]string{
			"PYRESTO_RAIZ=" + guiontest.Raiz(t), "PYRESTO_SALIDA=" + salida,
			"PYRESTO_TMP=" + t.TempDir(), "PYTHONDONTWRITEBYTECODE=1",
		}, env...),
	}, python, "-c", pyRestoPreambulo+fragmento)
	raw, err := os.ReadFile(salida) //nolint:gosec // G304: fichero del directorio temporal de la prueba.
	if err != nil {
		t.Fatalf("el fragmento de Python no devolvió nada (código %d)\n%s", r.Codigo, r.Todo())
	}
	if v != nil {
		if err := json.Unmarshal(raw, v); err != nil {
			t.Fatalf("salida del fragmento ilegible: %v\n%s", err, raw)
		}
	}
	return r
}

// pyRestoConEntrada escribe la entrada del fragmento (ENTRADA en Python) y
// devuelve la variable de entorno que la señala.
func pyRestoConEntrada(t *testing.T, entrada any) string {
	t.Helper()
	raw, err := json.Marshal(entrada)
	if err != nil {
		t.Fatal(err)
	}
	ruta := filepath.Join(t.TempDir(), "entrada.json")
	guiontest.Escribir(t, ruta, string(raw))
	return "PYRESTO_ENTRADA=" + ruta
}

// pyRestoLanza comprueba que hubo excepción de ese tipo (o de una subclase) y,
// si patron no está vacío, que su mensaje lo cumple (assertRaisesRegex).
func pyRestoLanza(t *testing.T, e *pyRestoExcepcion, tipo, patron string) {
	t.Helper()
	if e == nil {
		t.Fatalf("se esperaba %s y no hubo excepción", tipo)
	}
	if !slices.Contains(e.Tipos, tipo) {
		t.Fatalf("se esperaba %s y saltó %v: %s", tipo, e.Tipos, e.Mensaje)
	}
	if patron != "" && !regexp.MustCompile(patron).MatchString(e.Mensaje) {
		t.Fatalf("el mensaje de %s no cumple %q: %s", tipo, patron, e.Mensaje)
	}
}
