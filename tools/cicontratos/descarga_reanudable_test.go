// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package cicontratos

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/tools/guiontest"
)

// UNA DESCARGA DE HERRAMIENTA CORTADA POR TIEMPO SE REANUDA, EN LOS DOS GUIONES.
//
// El 2026-10-05 `scripts de producción` salió en rojo sin ejecutar un test: la
// descarga de Go iba a ~130 KB/s, `--max-time 300` la cortó con 39 MB de 67 y no
// quedaba reintento. Se arregló en `accion.sh`, y `bajar-herramienta.sh` —que baja
// trivy, osv-scanner, gitleaks, buildkit y mesa3d— seguía con el mismo hueco y
// números más viejos. Ahora los dos pasan por `descargar_reanudable`
// (deploy/sytue-git/bin/descarga-reanudable.sh) y esto lo ejercita por los dos.
//
// Con un `curl` de mentira en el PATH, no contra la red. Lo que hace el curl de
// verdad está medido aparte, contra go.dev estrangulado a 130 KB/s con el curl del
// CI (7.88.1): sin reanudar, corte a los 300 s con 40 MB; reanudando, completo a
// los 502 s y con su sha256.

// EL `curl` DE MENTIRA. Una descarga normal le cuesta dos llamadas: la primera
// deja «hola » y sale con 28, como un corte de `--max-time`; la segunda, solo si
// llega con `-C` y encuentra lo anterior, añade «mundo» y sale bien. Sin `-C`
// empieza de cero y se vuelve a cortar, que es lo que hacía el guion antes.
//
//	…/espejo/…   deja un resto en el destino y falla con 22 (no está en el espejo)
//	…/no-hay…    falla con 22 sin escribir nada
//	…/lenta…     se corta siempre: cada llamada añade un byte y sale con 28
const descargaReanudableCurl = `destino="" url="" reanuda=0
while [ $# -gt 0 ]; do
  case "$1" in
    -o) destino="$2"; shift ;;
    -C) reanuda=1; shift ;;
    http*) url="$1" ;;
  esac
  shift
done
echo "$url" >>"$REGISTRO"
case "$url" in
  */espejo/*) printf 'resto-del-espejo' >"$destino"; exit 22 ;;
  */no-hay*) exit 22 ;;
  */lenta*) printf 'x' >>"$destino"; exit 28 ;;
esac
if [ "$reanuda" = 1 ] && [ -s "$destino" ]; then
  printf 'mundo' >>"$destino"
  exit 0
fi
printf 'hola ' >"$destino"
exit 28`

const descargaReanudableOrigen = "https://origen.invalid/herramienta.tar.gz"

// descargaReanudableBanco es el directorio de una prueba: el curl de mentira
// delante en el PATH y el registro de sus llamadas.
type descargaReanudableBanco struct {
	dir, registro, sha string
	env                []string
}

func descargaReanudableMontar(t *testing.T) descargaReanudableBanco {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "bin")
	guiontest.Doble(t, bin, "curl", descargaReanudableCurl)
	suma := sha256.Sum256([]byte("hola mundo"))
	registro := filepath.Join(dir, "llamadas")
	return descargaReanudableBanco{
		dir: dir, registro: registro, sha: hex.EncodeToString(suma[:]),
		env: []string{"REGISTRO=" + registro, guiontest.ConPATH(bin)},
	}
}

func (b descargaReanudableBanco) llamadas(t *testing.T) int {
	t.Helper()
	return strings.Count(guiontest.Leer(t, b.registro), "\n")
}

// leer devuelve lo que quedó en un fichero del banco, o nada si no existe.
func (b descargaReanudableBanco) leer(nombre string) string {
	raw, err := os.ReadFile(filepath.Join(b.dir, nombre))
	if err != nil {
		return ""
	}
	return string(raw)
}

// descargaReanudableBajada es cómo salió bajar-herramienta.sh: su código, lo que
// dijo, cuántas veces llamó a curl y lo que quedó en el destino.
type descargaReanudableBajada struct {
	codigo, llamadas int
	salida, bajado   string
}

// bajar lanza bajar-herramienta.sh contra una URL de origen y, si se da, la raíz
// de un espejo (con su token de mentira).
func (b descargaReanudableBanco) bajar(t *testing.T, origen, espejo string) descargaReanudableBajada {
	t.Helper()
	guiontest.Escribir(t, b.registro, "")
	destino := filepath.Join(b.dir, "destino")
	if err := os.RemoveAll(destino); err != nil {
		t.Fatal(err)
	}
	token := ""
	if espejo != "" {
		token = "de-mentira"
	}
	env := append([]string{"SYTUE_TOOLS_MIRROR=" + espejo, "SYTUE_GIT_PACKAGES_TOKEN=" + token}, b.env...)
	r := guiontest.Guion(t, guiontest.Opciones{Env: env}, "deploy/sytue-git/bin/bajar-herramienta.sh",
		"herramienta", "1.0", "herramienta.tar.gz", b.sha, origen, destino)
	return descargaReanudableBajada{codigo: r.Codigo, llamadas: b.llamadas(t), salida: r.Todo(), bajado: b.leer("destino")}
}

// 1. EL CASO DEL ROJO: un corte por tiempo y la descarga sigue donde iba.
func TestDescargaReanudableBajarHerramientaReanudaUnCortePorTiempo(t *testing.T) {
	b := descargaReanudableMontar(t)
	r := b.bajar(t, descargaReanudableOrigen, "")
	if r.codigo != 0 || r.bajado != "hola mundo" || r.llamadas != 2 {
		t.Fatalf("bajar-herramienta.sh no reanuda un corte por tiempo: salida %d, %d llamada(s) a curl, bajado «%s»\n%s",
			r.codigo, r.llamadas, r.bajado, r.salida)
	}
}

// 2. Lo que dejó el espejo al fallar no se toma por media descarga del origen.
func TestDescargaReanudableTrasFallarElEspejoSeBajaDelOrigenDesdeCero(t *testing.T) {
	b := descargaReanudableMontar(t)
	r := b.bajar(t, descargaReanudableOrigen, "https://forja.invalid/espejo")
	if r.codigo != 0 || r.bajado != "hola mundo" || !strings.Contains(r.salida, "no está en el espejo") {
		t.Fatalf("tras fallar el espejo: salida %d, bajado «%s» (se esperaba «hola mundo» y el aviso del espejo)\n%s",
			r.codigo, r.bajado, r.salida)
	}
}

// 3. Solo se reanuda un corte por tiempo: un 404 falla a la primera.
func TestDescargaReanudableUnFalloQueNoEsDeTiempoNoSeReanuda(t *testing.T) {
	b := descargaReanudableMontar(t)
	r := b.bajar(t, "https://origen.invalid/no-hay.tar.gz", "")
	if r.codigo == 0 || r.llamadas != 1 {
		t.Fatalf("un 22 del origen: salida %d y %d llamada(s) a curl (se esperaba fallo con 1)", r.codigo, r.llamadas)
	}
}

// 4. Y no para siempre: tres tandas y se rinde, en rojo.
func TestDescargaReanudableUnaDescargaQueSeCortaSiempreSeDejaALasTresTandas(t *testing.T) {
	b := descargaReanudableMontar(t)
	r := b.bajar(t, "https://origen.invalid/lenta.tar.gz", "")
	if r.codigo == 0 || r.llamadas != 3 {
		t.Fatalf("una descarga que se corta siempre: salida %d y %d llamada(s) a curl (se esperaba fallo con 3)", r.codigo, r.llamadas)
	}
}

// 5. `accion.sh` baja por el mismo sitio. Sus dos funciones, tal cual están, con
// la descarga compartida cargada como la carga él.
func TestDescargaReanudableBajarVerificadoDeAccionReanudaIgual(t *testing.T) {
	b := descargaReanudableMontar(t)
	guiontest.Escribir(t, b.registro, "")
	// comprobar_sha256 vive en sha256.sh desde fb159b367; accion.sh lo carga igual.
	r := guiontest.Bash(t, guiontest.Opciones{Env: b.env}, `set -euo pipefail
. deploy/sytue-git/bin/descarga-reanudable.sh
. .github/actions/toolchain/sha256.sh
eval "$(sed -n '/^bajar_verificado() {/,/^}/p' .github/actions/toolchain/accion.sh)"
bajar_verificado "$1" "$2" "$3"`, descargaReanudableOrigen, b.sha, filepath.Join(b.dir, "de-accion"))
	if r.Codigo != 0 || b.leer("de-accion") != "hola mundo" || b.llamadas(t) != 2 {
		t.Fatalf("bajar_verificado de accion.sh no reanuda un corte por tiempo: salida %d, %d llamada(s), bajado «%s»\n%s",
			r.Codigo, b.llamadas(t), b.leer("de-accion"), r.Todo())
	}
}

// 6. Y la carga de verdad: sin argumentos, accion.sh llega hasta su modo de empleo.
// Si la ruta de la descarga compartida estuviera mal, moriría antes, en el `.`.
func TestDescargaReanudableAccionCargaLaDescargaCompartidaAlArrancar(t *testing.T) {
	b := descargaReanudableMontar(t)
	env := append([]string{"GITHUB_WORKSPACE=" + guiontest.Raiz(t)}, b.env...)
	r := guiontest.Guion(t, guiontest.Opciones{Env: env}, ".github/actions/toolchain/accion.sh")
	if !strings.Contains(r.Todo(), "uso: accion.sh") {
		t.Fatalf("accion.sh no llega a su modo de empleo: ¿encuentra deploy/sytue-git/bin/descarga-reanudable.sh? Dijo: %s", r.Todo())
	}
}
