// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package cicontratos

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/tools/guiontest"
)

// CADA HERRAMIENTA SE BAJA UNA VEZ POR RUNNER (2026-11-08). `quality` murió sin
// ejecutar nada porque Bun llegaba de GitHub a menos de 100 KB/s. `bajar_verificado`
// guarda ahora cada descarga en la caché del host, con su sha256 por nombre, y la
// saca de ahí la vez siguiente. Aquí se fija:
//  1. caché vacía: se descarga una vez y queda guardada;
//  2. caché llena: no se descarga;
//  3. una entrada corrupta no se usa: se tira y se vuelve a bajar;
//  4. sin el montaje (el Mac): se descarga y no se guarda nada;
//  5. todo contenedor de workflow que monta la caché de Go monta también esta.

// cacheDescargasGuion carga sha256.sh y `bajar_verificado` tal cual está en
// accion.sh, con una descarga de mentira que copia $DIR/origen y cuenta cuántas
// veces se la llamó. $1 = directorio de trabajo, $2 = caché (o una ruta que no
// existe), $3 = sha256 esperado.
const cacheDescargasGuion = `set -uo pipefail
DIR="$1"
ACCION=.github/actions/toolchain/accion.sh
. .github/actions/toolchain/sha256.sh
funcion="$(sed -n '/^DESCARGAS=/p; /^bajar_verificado() {/,/^}$/p' "$ACCION")"
grep -q 'bajar_verificado() {' <<<"$funcion" || { echo "no se encontró bajar_verificado en $ACCION" >&2; exit 97; }
descargar_reanudable() { echo x >>"$DIR/llamadas"; cp "$DIR/origen" "$1"; }
: >"$DIR/llamadas"
rm -f "$DIR/destino"
SYTUE_CACHE_DESCARGAS="$2"
eval "$funcion"
bajar_verificado https://ejemplo.invalid/bun.zip "$3" "$DIR/destino" 2>&1
`

// cacheDescargasIntento es lo que dejó una llamada a bajar_verificado.
type cacheDescargasIntento struct {
	codigo   int
	salida   string
	llamadas int
	destino  string // contenido del destino; vacío si no existe
}

func cacheDescargasProbar(t *testing.T, dir, cache, sha string) cacheDescargasIntento {
	t.Helper()
	r := guiontest.Bash(t, guiontest.Opciones{}, cacheDescargasGuion, dir, cache, sha)
	if r.Codigo == 97 {
		t.Fatalf("not ok - %s", r.Todo())
	}
	return cacheDescargasIntento{
		codigo:   r.Codigo,
		salida:   r.Todo(),
		llamadas: strings.Count(guiontest.Leer(t, filepath.Join(dir, "llamadas")), "\n"),
		destino:  cacheDescargasLeerSiHay(t, filepath.Join(dir, "destino")),
	}
}

func cacheDescargasLeerSiHay(t *testing.T, ruta string) string {
	t.Helper()
	raw, err := os.ReadFile(ruta) //nolint:gosec // G304: ruta dentro del directorio temporal de la prueba.
	if os.IsNotExist(err) {
		return ""
	}
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func cacheDescargasExiste(ruta string) bool {
	_, err := os.Lstat(ruta)
	return err == nil
}

func cacheDescargasListar(t *testing.T, dir string) []string {
	t.Helper()
	entradas, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	out := make([]string, 0, len(entradas))
	for _, e := range entradas {
		out = append(out, e.Name())
	}
	return out
}

// Los casos comparten la caché y van en orden, como en el guion original: el
// segundo encuentra lo que guardó el primero y el tercero lo corrompe.
func TestCacheDescargasSeGuardanYSeReutilizanPorSha256(t *testing.T) {
	const contenido = "bun de mentira"
	dir := t.TempDir()
	origen := filepath.Join(dir, "origen")
	guiontest.Escribir(t, origen, contenido)
	suma := sha256.Sum256([]byte(contenido))
	sha := hex.EncodeToString(suma[:])
	cache := filepath.Join(dir, "cache")
	if err := os.MkdirAll(cache, 0o750); err != nil {
		t.Fatal(err)
	}
	guardada := filepath.Join(cache, sha)

	t.Run("caché vacía: se descarga una vez y queda guardada por su sha", func(t *testing.T) {
		r := cacheDescargasProbar(t, dir, cache, sha)
		if r.llamadas != 1 || r.destino != contenido || cacheDescargasLeerSiHay(t, guardada) == "" {
			t.Fatalf("caché vacía: %d descargas, destino %q, guardada: %v", r.llamadas, r.destino, cacheDescargasListar(t, cache))
		}
	})

	t.Run("caché llena: no se descarga y se dice", func(t *testing.T) {
		r := cacheDescargasProbar(t, dir, cache, sha)
		if r.llamadas != 0 || r.destino != contenido || !strings.Contains(r.salida, "de la caché del runner") {
			t.Fatalf("caché llena: %d descargas, destino %q, salida %q", r.llamadas, r.destino, r.salida)
		}
	})

	t.Run("una entrada corrupta no se usa: se vuelve a bajar y se repone", func(t *testing.T) {
		guiontest.Escribir(t, guardada, "corrupto")
		r := cacheDescargasProbar(t, dir, cache, sha)
		if r.llamadas != 1 || r.destino != contenido || cacheDescargasLeerSiHay(t, guardada) != contenido {
			t.Fatalf("entrada corrupta: %d descargas, destino «%s», guardada «%s»",
				r.llamadas, r.destino, cacheDescargasLeerSiHay(t, guardada))
		}
	})

	t.Run("sin el montaje (el Mac): se descarga y no se crea nada", func(t *testing.T) {
		noExiste := filepath.Join(dir, "no-existe")
		r := cacheDescargasProbar(t, dir, noExiste, sha)
		if r.llamadas != 1 || r.destino != contenido || cacheDescargasExiste(noExiste) {
			t.Fatalf("sin montaje: %d descargas, destino %q", r.llamadas, r.destino)
		}
	})

	// Un sha que no casa sigue siendo rojo, con o sin caché.
	t.Run("un sha que no casa falla y no se guarda", func(t *testing.T) {
		guiontest.Escribir(t, origen, "otro contenido")
		vacia := cache + ".vacia"
		// Primero sin el directorio (el original lanzaba esta llamada y no miraba
		// cómo salía) y después con la caché vacía ya creada, que es la que decide.
		cacheDescargasProbar(t, dir, vacia, sha)
		if err := os.MkdirAll(vacia, 0o750); err != nil {
			t.Fatal(err)
		}
		r := cacheDescargasProbar(t, dir, vacia, sha)
		if r.codigo == 0 || cacheDescargasExiste(filepath.Join(vacia, sha)) {
			t.Fatalf("un sha que no casa no falló o se guardó (código %d, caché: %v)", r.codigo, cacheDescargasListar(t, vacia))
		}
	})
}

// Todo contenedor que monta la caché de Go monta también la de descargas: si no,
// ese flujo vuelve a bajarlo todo en cada job sin que nada lo diga.
func TestCacheDescargasLosContenedoresConCacheDeGoMontanLaDeDescargas(t *testing.T) {
	if sin := cacheDescargasSinMontaje(t, filepath.Join(guiontest.Raiz(t), ".github", "workflows")); len(sin) > 0 {
		t.Fatalf("contenedores sin la caché de descargas: %s", strings.Join(sin, " "))
	}
}

// cacheDescargasSinMontaje cuenta, por workflow, las líneas con cada montaje
// (lo que hacía `grep -c`) y devuelve los ficheros donde no coinciden.
func cacheDescargasSinMontaje(t *testing.T, dir string) []string {
	t.Helper()
	rutas, err := filepath.Glob(filepath.Join(dir, "*.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(rutas) == 0 {
		t.Fatalf("no hay workflows en %s", dir)
	}
	var sin []string
	for _, ruta := range rutas {
		var conGo, conDescargas int
		for _, linea := range strings.Split(guiontest.Leer(t, ruta), "\n") {
			if strings.Contains(linea, "/var/cache/sytue-ci/go-mod:") {
				conGo++
			}
			if strings.Contains(linea, "/var/cache/sytue-ci/descargas:/github/home/.cache/sytue-descargas") {
				conDescargas++
			}
		}
		if conGo != conDescargas {
			sin = append(sin, fmt.Sprintf(".github/workflows/%s (%d con caché de Go, %d con descargas)",
				filepath.Base(ruta), conGo, conDescargas))
		}
	}
	return sin
}

// El guardián de los montajes tiene que saltar cuando a un workflow le falta el
// de descargas: una copia con el montaje quitado de un solo sitio.
func TestCacheDescargasElGuardianDeMontajesFallaCuandoDebe(t *testing.T) {
	dir := t.TempDir()
	const montaje = "/var/cache/sytue-ci/descargas:/github/home/.cache/sytue-descargas"
	rutas, err := filepath.Glob(filepath.Join(guiontest.Raiz(t), ".github", "workflows", "*.yml"))
	if err != nil {
		t.Fatal(err)
	}
	mutado := false
	for _, ruta := range rutas {
		texto := guiontest.Leer(t, ruta)
		if !mutado && strings.Contains(texto, montaje) {
			texto = strings.Replace(texto, montaje, "/var/cache/sytue-ci/otra:/otra", 1)
			mutado = true
		}
		guiontest.Escribir(t, filepath.Join(dir, filepath.Base(ruta)), texto)
	}
	if !mutado {
		t.Fatal("ningún workflow monta la caché de descargas: la mutación no prueba nada")
	}
	if sin := cacheDescargasSinMontaje(t, dir); len(sin) != 1 {
		t.Fatalf("el guardián no vio el montaje que falta: %q", sin)
	}
}
