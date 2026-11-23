// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package pruebasfyne

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/tools/guiontest"
)

// Las pruebas de los módulos Python de la firma de Apple (apple-perfiles.py,
// apple_llavero.py, apple_perfil.py…) cargan el módulo REAL dentro de un
// python3 y le ponen los mismos dobles en proceso que tenían sus pruebas de
// unittest (mock.patch de la frontera con Apple, `security` o `codesign`): esos
// dobles no pueden vivir fuera del proceso que importa el módulo. El fragmento
// solo ejecuta y anota en `R` lo que observa; las comprobaciones están en Go.

// applePyPreludio va delante de cada fragmento: `R` es lo observado, y
// `atrapar` convierte el resultado o la excepción en datos que Go comprueba.
const applePyPreludio = `
import json, os, sys, pathlib, datetime, base64
R = {}
def atrapar(fn, *a, **k):
    try:
        return {"ok": fn(*a, **k)}
    except BaseException as e:
        return {"exc": type(e).__name__, "msg": str(e), "code": getattr(e, "code", None)}
def _a_json(o):
    if isinstance(o, (bytes, bytearray)):
        return bytes(o).decode("utf-8", "backslashreplace")
    if isinstance(o, (pathlib.PurePath, datetime.datetime)):
        return str(o)
    if isinstance(o, (set, frozenset)):
        return sorted(o)
    return repr(o)
def _volcar():
    with open(os.environ["APPLE_PY_SALIDA"], "w") as f:
        json.dump(R, f, default=_a_json)
`

// appleScripts es apps/fyne/scripts dentro de la raíz.
func appleScripts(t *testing.T) string {
	t.Helper()
	return filepath.Join(guiontest.Raiz(t), "apps", "fyne", "scripts")
}

// applePy ejecuta el fragmento con python3 en apps/fyne/scripts (como el
// original, para que `from apple_clave import …` encuentre el módulo) y
// devuelve `R`. Un fallo de Python fuera de `atrapar` es un fallo de la prueba.
func applePy(t *testing.T, codigo string, env ...string) map[string]any {
	t.Helper()
	salida := filepath.Join(t.TempDir(), "observado.json")
	fuente := applePyPreludio + "\ntry:\n" + appleSangrar(codigo) + "\nfinally:\n    _volcar()\n"
	r := guiontest.Ejecutar(t, guiontest.Opciones{Dir: appleScripts(t), Env: append([]string{"APPLE_PY_SALIDA=" + salida}, env...)},
		"python3", "-c", fuente)
	if r.Codigo != 0 {
		t.Fatalf("el fragmento de Python falló (código %d):\n%s", r.Codigo, r.Todo())
	}
	var R map[string]any
	if err := json.Unmarshal([]byte(guiontest.Leer(t, salida)), &R); err != nil {
		t.Fatalf("R no es JSON: %v", err)
	}
	return R
}

func appleSangrar(codigo string) string {
	lineas := strings.Split(codigo, "\n")
	for i, l := range lineas {
		lineas[i] = "    " + l
	}
	return strings.Join(lineas, "\n") + "\n    pass"
}

// appleIgual compara un valor observado con el JSON esperado (normalizado: el
// orden de las claves y la diferencia entre tupla y lista no cuentan).
func appleIgual(t *testing.T, que string, observado any, esperado string) {
	t.Helper()
	var quiero any
	if err := json.Unmarshal([]byte(esperado), &quiero); err != nil {
		t.Fatalf("%s: JSON esperado mal escrito: %v", que, err)
	}
	if !reflect.DeepEqual(appleNormalizar(observado), appleNormalizar(quiero)) {
		t.Errorf("%s:\n  observado %s\n  esperado  %s", que, appleTexto(observado), appleTexto(quiero))
	}
}

func appleNormalizar(v any) any {
	raw, err := json.Marshal(v)
	if err != nil {
		return v
	}
	var n any
	_ = json.Unmarshal(raw, &n)
	return n
}

func appleTexto(v any) string {
	raw, _ := json.Marshal(v)
	return string(raw)
}

// appleOK exige que `atrapar` no viera excepción y devuelve el valor.
func appleOK(t *testing.T, que string, r any) any {
	t.Helper()
	m, _ := r.(map[string]any)
	if exc, hay := m["exc"]; hay {
		t.Fatalf("%s: se esperaba éxito y lanzó %v: %v", que, exc, m["msg"])
	}
	return m["ok"]
}

// appleExc exige la excepción `tipo` cuyo mensaje case con `patron` (vacío:
// cualquiera), como assertRaises/assertRaisesRegex.
func appleExc(t *testing.T, que string, r any, tipo, patron string) {
	t.Helper()
	m, _ := r.(map[string]any)
	exc, hay := m["exc"]
	if !hay {
		t.Errorf("%s: se esperaba %s y devolvió %s", que, tipo, appleTexto(m["ok"]))
		return
	}
	if exc != tipo {
		t.Errorf("%s: se esperaba %s y lanzó %v: %v", que, tipo, exc, m["msg"])
		return
	}
	if patron != "" {
		msg, _ := m["msg"].(string)
		if !regexp.MustCompile(patron).MatchString(msg) {
			t.Errorf("%s: el mensaje %q no casa con %q", que, msg, patron)
		}
	}
}

// appleVerdad interpreta un valor observado como lo haría bool() de Python
// sobre JSON (assertTrue/assertFalse).
func appleVerdad(v any) bool {
	switch x := v.(type) {
	case nil:
		return false
	case bool:
		return x
	case float64:
		return x != 0
	case string:
		return x != ""
	case []any:
		return len(x) > 0
	case map[string]any:
		return len(x) > 0
	default:
		return true
	}
}

// appleCierto y appleFalso son assertTrue y assertFalse.
func appleCierto(t *testing.T, que string, v any) {
	t.Helper()
	if !appleVerdad(v) {
		t.Errorf("%s: se esperaba verdadero y es %s", que, appleTexto(v))
	}
}

func appleFalso(t *testing.T, que string, v any) {
	t.Helper()
	if appleVerdad(v) {
		t.Errorf("%s: se esperaba falso y es %s", que, appleTexto(v))
	}
}

// appleCampo navega por mapas y listas: appleCampo(R, "a", 0, "b").
func appleCampo(v any, ruta ...any) any {
	for _, paso := range ruta {
		switch p := paso.(type) {
		case string:
			m, _ := v.(map[string]any)
			v = m[p]
		case int:
			l, _ := v.([]any)
			if p < 0 {
				p += len(l)
			}
			if p < 0 || p >= len(l) {
				return nil
			}
			v = l[p]
		}
	}
	return v
}
