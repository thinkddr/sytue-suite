// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// EL GUARDIÁN DE LA CADENA DE EMPAQUETADO, Y SE VE EN ROJO EN CADA EJECUCIÓN.
//
// Un guardián que nadie ha visto fallar no guarda nada. Este no se limita a
// comprobar el caso bueno: compila a propósito un binario de la arquitectura
// CONTRARIA y EXIGE que `empaquetar.sh verificar` lo rechace. Si algún día la
// comprobación se rompe —un `sed` que deja de casar, un `go version -m` que cambia
// de formato, alguien que la comenta «un momento»— esta prueba se pone roja por el
// caso negativo, no por el positivo.
//
// Por qué importa: los runners propios son ARM64 y el servidor es x86_64. Cuatro
// versiones se publicaron con la arquitectura del runner y ninguna puerta se puso
// en rojo, porque compilar y empaquetar salen bien igual.
//
// Era `apps/fyne/scripts/empaquetar-test.sh`; se reparte en los ficheros
// `empaquetar_*_test.go` de este paquete, uno por sección. Aquí, lo común.

package pruebasfyne

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/tools/guiontest"
)

// empaquetarGoprivate es lo que el CI exporta antes de llamar al guion: apps/fyne
// depende de forks privados y `empaquetar.sh` se niega a seguir sin él (ver
// GOPRIVATE en el propio guion). No es lo que se prueba, es su precondición.
const empaquetarGoprivate = "GOPRIVATE=github.com/thinkddr/*"

// empaquetarUnir es filepath.Join con las partes escritas con `/`, como en el original.
func empaquetarUnir(partes ...string) string {
	for i := range partes {
		partes[i] = filepath.FromSlash(partes[i])
	}
	return filepath.Join(partes...)
}

// empaquetarRutas son las variables del guion original: RAIZ, FYNE, AQUI y GUION.
type empaquetarRutas struct {
	raiz, fyne, aqui, guion string
}

func empaquetarDirs(t *testing.T) empaquetarRutas {
	t.Helper()
	raiz := guiontest.Raiz(t)
	fyne := empaquetarUnir(raiz, "apps", "fyne")
	aqui := empaquetarUnir(fyne, "scripts")
	return empaquetarRutas{raiz: raiz, fyne: fyne, aqui: aqui, guion: empaquetarUnir(aqui, "empaquetar.sh")}
}

// empaquetarCon ejecuta `empaquetar.sh args…` (por su shebang, como "$GUION" en el
// original) con el entorno extra dado.
func empaquetarCon(t *testing.T, env []string, args ...string) guiontest.Resultado {
	t.Helper()
	d := empaquetarDirs(t)
	return guiontest.Ejecutar(t, guiontest.Opciones{Env: append([]string{empaquetarGoprivate}, env...)}, d.guion, args...)
}

// empaquetar ejecuta `empaquetar.sh args…` con el entorno heredado.
func empaquetar(t *testing.T, args ...string) guiontest.Resultado {
	t.Helper()
	return empaquetarCon(t, nil, args...)
}

// empaquetarJunto es `$(… 2>&1)`: stdout y stderr juntos, sin saltos finales.
func empaquetarJunto(r guiontest.Resultado) string {
	return strings.TrimRight(r.Salida+r.Error, "\n")
}

// empaquetarUnaLinea es `| tr '\n' ' '`, para los mensajes.
func empaquetarUnaLinea(s string) string { return strings.ReplaceAll(s, "\n", " ") }

// empaquetarUltima es `tail -1`.
func empaquetarUltima(s string) string {
	l := strings.Split(strings.TrimRight(s, "\n"), "\n")
	return l[len(l)-1]
}

// empaquetarPrimera es `head -1`.
func empaquetarPrimera(s string) string {
	l, _, _ := strings.Cut(s, "\n")
	return l
}

// empaquetarHay es `grep -qE patrón` sobre un texto: alguna línea casa.
func empaquetarHay(texto, patron string) bool {
	re := regexp.MustCompile(patron)
	for l := range strings.SplitSeq(texto, "\n") {
		if re.MatchString(l) {
			return true
		}
	}
	return false
}

// empaquetarLineas es `grep -E patrón`: las líneas que casan.
func empaquetarLineas(texto, patron string) []string {
	re := regexp.MustCompile(patron)
	var r []string
	for l := range strings.SplitSeq(texto, "\n") {
		if re.MatchString(l) {
			r = append(r, l)
		}
	}
	return r
}

// empaquetarCuenta es `grep -c patrón`.
func empaquetarCuenta(texto, patron string) int { return len(empaquetarLineas(texto, patron)) }

// empaquetarHayLinea es `grep -qx`: alguna línea es exactamente esa.
func empaquetarHayLinea(texto, linea string) bool {
	return slices.Contains(strings.Split(texto, "\n"), linea)
}

// empaquetarSed es `sed -n 's/patrón/\1/p'`: el primer grupo de cada línea que casa.
func empaquetarSed(texto, patron string) []string {
	re := regexp.MustCompile(patron)
	var r []string
	for l := range strings.SplitSeq(texto, "\n") {
		if m := re.FindStringSubmatch(l); m != nil {
			r = append(r, m[1])
		}
	}
	return r
}

// empaquetarEjecutable escribe un fichero ejecutable con ese contenido exacto
// (shebang incluido): los dobles del original son de sh, de bash o de python.
func empaquetarEjecutable(t *testing.T, ruta, contenido string) {
	t.Helper()
	guiontest.Escribir(t, ruta, contenido)
	if err := os.Chmod(ruta, 0o755); err != nil {
		t.Fatal(err)
	}
}

// empaquetarLeer lee un fichero; si no existe, devuelve "".
func empaquetarLeer(ruta string) string {
	raw, err := os.ReadFile(ruta)
	if err != nil {
		return ""
	}
	return string(raw)
}

// empaquetarExiste dice si la ruta existe.
func empaquetarExiste(ruta string) bool {
	_, err := os.Stat(ruta)
	return err == nil
}

// empaquetarConPATH antepone dir al PATH.
func empaquetarConPATH(dir string) string { return guiontest.ConPATH(dir) }

// empaquetarBinarios compila, como el original, un programa mínimo para linux/amd64 y
// linux/arm64: lo que se mide es la cabecera del binario, no lo que hace.
func empaquetarBinarios(t *testing.T) (dir, amd64, arm64 string) {
	t.Helper()
	dir = t.TempDir()
	guiontest.Escribir(t, empaquetarUnir(dir, "main.go"), "package main\n\nfunc main() {}\n")
	guiontest.Escribir(t, empaquetarUnir(dir, "go.mod"), "module ejemplo\n\ngo 1.27.4\n")
	amd64 = empaquetarConstruir(t, dir, "linux", "amd64")
	arm64 = empaquetarConstruir(t, dir, "linux", "arm64")
	return dir, amd64, arm64
}

func empaquetarConstruir(t *testing.T, dir, goos, goarch string, extra ...string) string {
	t.Helper()
	salida := empaquetarUnir(dir, "bin-"+goos+"-"+goarch+strings.Join(extra, ""))
	args := append([]string{"build"}, extra...)
	args = append(args, "-o", salida, ".")
	r := guiontest.Ejecutar(t, guiontest.Opciones{Dir: dir, Env: []string{"CGO_ENABLED=0", "GOOS=" + goos, "GOARCH=" + goarch}}, "go", args...)
	if r.Codigo != 0 {
		t.Fatalf("no se pudieron compilar los binarios de prueba (%s/%s): %s", goos, goarch, r.Todo())
	}
	return salida
}

// --- plist y TOML -----------------------------------------------------------------
// Go no trae lector de plist ni de TOML: python3 (plistlib, tomllib) hace de
// conversor a JSON, como jq en otras pruebas. Las comprobaciones son de Go.

const empaquetarPlistAJSON = `import json, plistlib, sys
print(json.dumps(plistlib.loads(sys.stdin.buffer.read()), default=str))`

// empaquetarPlist lee un plist (XML o binario) y lo devuelve como JSON genérico.
func empaquetarPlist(t *testing.T, datos string) (map[string]any, error) {
	t.Helper()
	r := guiontest.Ejecutar(t, guiontest.Opciones{Entrada: datos}, "python3", "-c", empaquetarPlistAJSON)
	if r.Codigo != 0 {
		return nil, &empaquetarError{r.Error}
	}
	var v map[string]any
	if err := json.Unmarshal([]byte(r.Salida), &v); err != nil {
		return nil, err
	}
	return v, nil
}

type empaquetarError struct{ texto string }

func (e *empaquetarError) Error() string { return e.texto }

const empaquetarJSONAPlist = `import json, plistlib, sys
fmt = plistlib.FMT_BINARY if sys.argv[2] == "binario" else plistlib.FMT_XML
open(sys.argv[1], "wb").write(plistlib.dumps(json.loads(sys.stdin.read()), fmt=fmt))`

// empaquetarEscribirPlist escribe v como plist XML o binario.
func empaquetarEscribirPlist(t *testing.T, ruta string, v any, binario bool) {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(ruta), 0o750); err != nil {
		t.Fatal(err)
	}
	formato := "xml"
	if binario {
		formato = "binario"
	}
	r := guiontest.Ejecutar(t, guiontest.Opciones{Entrada: string(raw)}, "python3", "-c", empaquetarJSONAPlist, ruta, formato)
	if r.Codigo != 0 {
		t.Fatalf("no se pudo escribir el plist %s: %s", ruta, r.Error)
	}
}

const empaquetarTOMLAJSON = `import json, sys, tomllib
print(json.dumps(tomllib.load(open(sys.argv[1], "rb")), default=str))`

// empaquetarTOML lee un FyneApp.toml como JSON genérico.
func empaquetarTOML(t *testing.T, ruta string) map[string]any {
	t.Helper()
	r := guiontest.Ejecutar(t, guiontest.Opciones{}, "python3", "-c", empaquetarTOMLAJSON, ruta)
	if r.Codigo != 0 {
		t.Fatalf("no se pudo leer %s: %s", ruta, r.Error)
	}
	var v map[string]any
	if err := json.Unmarshal([]byte(r.Salida), &v); err != nil {
		t.Fatal(err)
	}
	return v
}

// empaquetarJSON decodifica un JSON genérico.
func empaquetarJSON(texto string) any {
	var v any
	if err := json.Unmarshal([]byte(texto), &v); err != nil {
		return nil
	}
	return v
}

// empaquetarEn navega un JSON genérico: claves de objeto o índices de lista.
func empaquetarEn(v any, ruta ...any) any {
	for _, paso := range ruta {
		switch p := paso.(type) {
		case string:
			m, ok := v.(map[string]any)
			if !ok {
				return nil
			}
			v = m[p]
		case int:
			l, ok := v.([]any)
			if !ok || p >= len(l) {
				return nil
			}
			v = l[p]
		}
	}
	return v
}

// empaquetarIgualJSON compara un valor genérico con un literal JSON.
func empaquetarIgualJSON(v any, literal string) bool {
	quiero := empaquetarJSON(literal)
	return quiero != nil && empaquetarMismoJSON(v, quiero)
}

// empaquetarMismoJSON: los dos valores se serializan igual (los mapas, con las claves
// ordenadas: como `==` de jq o de Python, sin importar el orden).
func empaquetarMismoJSON(a, b any) bool {
	x, err := json.Marshal(a)
	y, err2 := json.Marshal(b)
	return err == nil && err2 == nil && bytes.Equal(x, y)
}

// empaquetarApps son los directorios de apps/fyne/cmd, en orden.
func empaquetarApps(t *testing.T) []string {
	t.Helper()
	d := empaquetarDirs(t)
	entradas, err := os.ReadDir(empaquetarUnir(d.fyne, "cmd"))
	if err != nil {
		t.Fatal(err)
	}
	var apps []string
	for _, e := range entradas {
		if e.IsDir() {
			apps = append(apps, e.Name())
		}
	}
	return apps
}

// empaquetarID es el ID del FyneApp.toml como lo lee el original:
// `sed -n 's/^[[:space:]]*ID[[:space:]]*=[[:space:]]*"\(.*\)".*/\1/p' | head -1`.
func empaquetarID(t *testing.T, toml string) string {
	t.Helper()
	ids := empaquetarSed(guiontest.Leer(t, toml), `^[[:space:]]*ID[[:space:]]*=[[:space:]]*"(.*)".*`)
	if len(ids) == 0 {
		return ""
	}
	return ids[0]
}

// empaquetarExtraerFunciones copia de empaquetar.sh las funciones nombradas, como
// `sed -n "/^f() {/,/^}/p"`; con conHeredoc, además salta los `<<'PY'` (el awk del
// original), para que una `}` dentro de un Python no corte la función.
func empaquetarExtraerFunciones(texto string, conHeredoc bool, nombres ...string) string {
	var b strings.Builder
	lineas := strings.Split(texto, "\n")
	for _, fn := range nombres {
		dentro, py := false, false
		for _, l := range lineas {
			if !dentro && strings.HasPrefix(l, fn+"() {") {
				dentro = true
			}
			if !dentro {
				continue
			}
			b.WriteString(l + "\n")
			switch {
			case conHeredoc && !py && strings.Contains(l, "<<'PY'"):
				py = true
			case conHeredoc && py && l == "PY":
				py = false
			case !py && strings.HasPrefix(l, "}") && (conHeredoc && l == "}" || !conHeredoc):
				dentro = false
			}
		}
	}
	return b.String()
}

// empaquetarClaves son las <key> de un plist en XML, ordenadas y en una línea, como
// `grep -o '<key>[^<]*</key>' | sed 's/<[^>]*>//g' | sort | tr '\n' ' '`.
func empaquetarClaves(plist string) string {
	coincidencias := regexp.MustCompile(`<key>([^<]*)</key>`).FindAllStringSubmatch(plist, -1)
	claves := make([]string, 0, len(coincidencias))
	for _, m := range coincidencias {
		claves = append(claves, m[1])
	}
	slices.Sort(claves)
	if len(claves) == 0 {
		return ""
	}
	return strings.Join(claves, " ") + " "
}

// El equipo de Apple de la suite. Aquí y no en empaquetar_apple_tienda_test.go (que no
// se publica en sytue-suite) porque lo usan también las pruebas de Sytue Office, que sí.
const empaquetarEquipo = "VHC5QW7BZ3"

// empaquetarPlistDe ejecuta una orden de empaquetar.sh que imprime un plist y lo lee
// (subprocess.check_output + plistlib.loads del original).
func empaquetarPlistDe(t *testing.T, env []string, args ...string) (map[string]any, bool) {
	t.Helper()
	r := empaquetarCon(t, env, args...)
	if r.Codigo != 0 {
		return nil, false
	}
	p, err := empaquetarPlist(t, r.Salida)
	return p, err == nil
}
