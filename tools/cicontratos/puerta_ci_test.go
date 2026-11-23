// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package cicontratos

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/tools/guiontest"
)

// LA PUERTA DE CI DE `imagenes-produccion`: QUÉ CUENTA Y QUE SE LEE ENTERO.
//
// Dos cosas que se arreglaron el 2026-10-05 y que aquí quedan fijadas.
//
//  1. CUENTA LA ÚLTIMA EJECUCIÓN DE CADA FLUJO (decisión del dueño). Antes era el
//     peor de todo lo que hubiera corrido sobre el commit, y un flujo cancelado lo
//     dejaba en «failure» para siempre: a v3.10.1 y a v3.11.0 les cancelaron los
//     flujos de la release y solo se podían publicar con `ignorar_ci`. Ahora un
//     flujo cancelado y vuelto a lanzar en verde deja de bloquear. Lo que no
//     cambia: si la última ejecución está fallida o sin terminar, bloquea igual;
//     y una cancelada solo si es de la RELEASE (v3.18.3): un despacho manual
//     cancelado no es de la cadena ni un veredicto del código —no cuenta, no
//     sustituye a la anterior y se ve como aviso, con su enlace.
//
//  2. SE LEEN TODAS LAS PÁGINAS. Se pedía `check-runs?per_page=100` y se decidía
//     con esa primera página; GitHub da primero los más recientes, así que en un
//     commit con más de 100 lo más antiguo —un rojo incluido— dejaba de contar.
//
// Sin forja delante: se extraen `leer_paginas` y `leer_estado` del paso «Exigir
// el CI del commit» (sed entre cabecera y cierre, como
// traer-instaladores-completitud-test.sh) y se ejercitan con un `curl` de mentira.
// El último caso no es inventado: son los check runs y las ejecuciones de
// c3c6c0d4a (v3.11.0) tal como quedaron, en puerta-ci-v3.11.0.json.
//
//  3. EL VIGILANTE DEL PILOTO DICE LO MISMO. `vigilante-releases.sh` aplica el mismo
//     criterio antes de desplegar, escrito otra vez en jq porque el piloto no tiene
//     bun ni el contenedor del CI jq. Dos copias de un criterio se separan solas,
//     así que cada caso de «qué cuenta» pasa por las dos y tienen que coincidir:
//     si una release la publica la puerta y el vigilante la deja esperando, o al
//     revés, es que una de las dos ha cambiado sin la otra.
//
//  4. LOS BARRIDOS PROGRAMADOS CONTRA PRODUCCIÓN AVISAN, NO BLOQUEAN (decisión del
//     dueño). `escritorio` y `mobile` por `schedule` miden lo desplegado y su check
//     cae en el commit que sea cabeza de `main`: en rojo frenaban la publicación de
//     un commit que no habían medido. A mano o por release siguen contando.
//
//  6. SOLO CUENTAN LOS FLUJOS DE CI DE LA RELEASE (decisión del dueño, 2026-11-08):
//     ci-desktop, ci-frontend, ci-integration, ci-mobile y security. El `ci` a mano,
//     release-desktop, Codemagic, Xcode Cloud o un estado externo en rojo avisan y no
//     frenan: v3.19.4 se quedó sin imágenes por un token caducado de Codemagic.
//
//  5. LA CADENA NO REPITE LO YA PUBLICADO. En v3.11.1 se publicó a mano y la cadena
//     de la release volvió a construir y subir las 55 imágenes. Ahora, si el
//     despacho viene de la cadena y `desplegable/<perfil>/<tag>` apunta a ese mismo
//     commit, se salta la construcción y se va derecho a `release-desktop`.

const (
	puertaFlujo     = ".github/workflows/imagenes-produccion.yml"
	puertaVigilante = "deploy/helm/bin/vigilante-releases.sh"
	puertaReal      = ".github/tests/puerta-ci-v3.11.0.json"
	// Lo que las funciones leen de su paso.
	puertaSHA  = "0123456789abcdef0123456789abcdef01234567"
	puertaOtro = "1111111111111111111111111111111111111111"
	// A dónde apunta la referencia de una marca anotada: su objeto etiqueta.
	puertaObjetoEtiqueta = "2222222222222222222222222222222222222222"
	puertaRuns           = "https://forja.invalid/sytue/one/actions/runs/"
	// Con qué sale el preámbulo si no encuentra una función en su fichero.
	puertaSinFuncion = 97
)

// puertaPreambulo deja en el shell las funciones tal cual están en el workflow,
// sin su sangría de YAML. Lo único que se les cambia es dónde dejan sus ficheros,
// para no escribir en el /tmp de nadie. Y las dos del vigilante, también tal cual.
const puertaPreambulo = `set -uo pipefail
FUNCIONES="$(sed -n '/^          leer_paginas() {/,/^          }$/p;/^          leer_estado() {$/,/^          }$/p;/^          decidir_publicacion() {$/,/^          }$/p' "$WF" |
  sed -e 's/^          //' -e "s#/tmp/#$DIR/#g")"
grep -F 'leer_estado() {' <<<"$FUNCIONES" >/dev/null || {
  echo "no se encontró leer_estado en $WF" >&2; exit 97; }
grep -F 'decidir_publicacion() {' <<<"$FUNCIONES" >/dev/null || {
  echo "no se encontró decidir_publicacion en $WF (CI-06)" >&2; exit 97; }
eval "$FUNCIONES"

FUNCIONES="$(sed -n '/^leer_lista() {$/,/^}$/p;/^estado_ci() {$/,/^}$/p' "$VIGILANTE")"
grep -F 'estado_ci() {' <<<"$FUNCIONES" >/dev/null || {
  echo "no se encontró estado_ci en $VIGILANTE" >&2; exit 97; }
eval "$FUNCIONES"

FUNCIONES="$(sed -n '/^          ya_publicadas() {$/,/^          }$/p' "$WF" | sed -e 's/^          //' -e "s#/tmp/#$DIR/#g")"
grep -F 'ya_publicadas() {' <<<"$FUNCIONES" >/dev/null || {
  echo "no se encontró ya_publicadas en $WF" >&2; exit 97; }
eval "$FUNCIONES"

API=https://forja.invalid/repos/sytue/one
GITHUB_SHA=` + puertaSHA + `
GITHUB_RUN_ID=999
TOKEN=de-mentira
SYTUE_GIT_TOKEN=$TOKEN
`

// puertaCurl es UN «curl» DE MENTIRA que sirve $FORJA —{check_runs,
// workflow_runs}— de 100 en 100, por la dirección que le pidan.
// AVERIA=<lista>:<página> hace que esa página dé un 502; SORDA=1, que la API no
// haga caso de `page` y devuelva siempre la primera. Una petición sin `page`
// recibe la primera, como en GitHub. Contesta a las dos formas de pedir: con
// `-o` y `-w` (la puerta) y a la salida estándar (el vigilante, que no entra en
// la cuenta de $PETICIONES).
//
// La marca `desplegable/…` (`*/git/ref/tags/*`): MARCA_FORJA vacía es que no
// existe; «averia», un 502; «anotada:…», una etiqueta anotada (lo que es desde
// que lleva los digest); cualquier otra cosa, el commit al que apunta una ligera.
//
// El objeto de una etiqueta anotada (`*/git/tags/*`): «anotada:<commit>» dice a
// qué commit apunta; «anotada:averia», que no se puede leer; «anotada:arbol»,
// que apunta a algo que no es un commit.
const puertaCurl = `salida="" url="" con_codigo=""
while [ $# -gt 0 ]; do
  case "$1" in
    -o) salida="$2"; shift ;;
    -w) con_codigo=1; shift ;;
    --max-time | -H) shift ;;
    http*) url="$1" ;;
  esac
  shift
done
case "$url" in
  */status) printf '{"statuses":%s}' "${ESTADOS:-[]}" >"$salida"; exit 0 ;;
  */git/ref/tags/*)
    case "${MARCA_FORJA:-}" in
      "") echo '{"message":"Not Found"}' >"$salida"; printf 404 ;;
      averia) echo '<html>502 Bad Gateway</html>' >"$salida"; printf 502 ;;
      anotada:*) printf '{"object":{"sha":"%s","type":"tag"}}' "$OBJETO_ETIQUETA" >"$salida"; printf 200 ;;
      *) printf '{"object":{"sha":"%s","type":"commit"}}' "$MARCA_FORJA" >"$salida"; printf 200 ;;
    esac
    exit 0 ;;
  */git/tags/*)
    case "${MARCA_FORJA:-}" in
      anotada:averia) echo '<html>502 Bad Gateway</html>' >"$salida"; printf 502 ;;
      anotada:arbol) printf '{"object":{"sha":"%s","type":"tree"}}' "$OBJETO_ETIQUETA" >"$salida"; printf 200 ;;
      *) printf '{"object":{"sha":"%s","type":"commit"}}' "${MARCA_FORJA#anotada:}" >"$salida"; printf 200 ;;
    esac
    exit 0 ;;
esac
pagina=1 lista=check_runs
case "$url" in *"&page="*) pagina="${url##*&page=}" ;; esac
case "$url" in */actions/runs*) lista=workflow_runs ;; esac
if [ -z "$salida" ]; then
  jq -c --arg l "$lista" --argjson p "$pagina" \
    '{total_count: (.[$l] | length), ($l): .[$l][(($p - 1) * 100):($p * 100)]}' "$FORJA"
  exit
fi
echo "$lista:$pagina" >>"$PETICIONES"
if [ "$lista:$pagina" = "${AVERIA:-}" ]; then
  echo '<html>502 Bad Gateway</html>' >"$salida"
  [ -z "$con_codigo" ] || printf 502
  exit 0
fi
[ "${SORDA:-0}" != 1 ] || pagina=1
jq -c --arg l "$lista" --argjson p "$pagina" \
  '{total_count: (.[$l] | length), ($l): .[$l][(($p - 1) * 100):($p * 100)]}' "$FORJA" >"$salida"
[ -z "$con_codigo" ] || printf 200`

// puertaForja es CÓMO SE MONTA UNA FORJA: las ejecuciones y los check runs que
// hay sobre el commit.
type puertaForja struct{ runs, checks []any }

func puertaConclusion(c string) any {
	if c == "" {
		return nil
	}
	return c
}

// ejecucion añade una ejecución de un flujo con sus jobs («job=conclusión»).
//
// La conclusión vacía es «sin terminar» (la ejecución en cola, el job en marcha).
// El flujo se identifica por su nombre y la suite de la ejecución es su id + 1000,
// que es lo que ata cada check run a la suya.
func (f *puertaForja) ejecucion(id int, flujo, evento, hora, conclusion string, jobs ...string) {
	estado := "completed"
	if conclusion == "" {
		estado = "queued"
	}
	f.runs = append(f.runs, map[string]any{
		"id": id, "workflow_id": flujo, "name": flujo, "path": ".github/workflows/" + flujo + ".yml",
		"event": evento, "status": estado, "conclusion": puertaConclusion(conclusion),
		"run_started_at": "2026-10-05T" + hora + ":00Z", "check_suite_id": id + 1000,
		"html_url": fmt.Sprintf("%s%d", puertaRuns, id),
	})
	for n, j := range jobs {
		corte := strings.LastIndex(j, "=")
		nombre, c := j[:corte], j[corte+1:]
		estado := "completed"
		if c == "" {
			estado = "in_progress"
		}
		f.checks = append(f.checks, map[string]any{
			"name": nombre, "status": estado, "conclusion": puertaConclusion(c),
			"html_url":    fmt.Sprintf("%s%d/job/%d", puertaRuns, id, n+1),
			"check_suite": map[string]any{"id": id + 1000},
		})
	}
}

// suelto añade un check run que no es de ninguna ejecución, como el de una
// aplicación externa.
func (f *puertaForja) suelto(nombre, conclusion, app string) {
	f.checks = append(f.checks, map[string]any{
		"name": nombre, "status": "completed", "conclusion": conclusion,
		"html_url":    "https://externa.invalid/" + nombre,
		"check_suite": map[string]any{"id": 1}, "app": map[string]any{"slug": app},
	})
}

// puertaMuchosJobs es una ejecución con N jobs, todos en verde salvo el de la
// posición ROJO (0 = ninguno).
func puertaMuchosJobs(n, rojo int) *puertaForja {
	conclusion := "success"
	if rojo > 0 {
		conclusion = "failure"
	}
	f := &puertaForja{runs: []any{map[string]any{
		"id": 7, "workflow_id": "ci-frontend", "name": "ci-frontend", "path": ".github/workflows/ci-frontend.yml",
		"event": "workflow_dispatch", "status": "completed", "conclusion": conclusion,
		"run_started_at": "2026-10-05T12:00:00Z", "check_suite_id": 1007,
	}}}
	for i := 1; i <= n; i++ {
		c := "success"
		if i == rojo {
			c = "failure"
		}
		f.checks = append(f.checks, map[string]any{
			"name": fmt.Sprintf("job-%d", i), "status": "completed", "conclusion": c,
			"html_url":    fmt.Sprintf("%s7/job/%d", puertaRuns, i),
			"check_suite": map[string]any{"id": 1007},
		})
	}
	return f
}

// puertaBanco es el banco de pruebas de un caso: su directorio, con la forja,
// el `curl` de mentira y lo que las funciones dejan donde antes era /tmp.
type puertaBanco struct {
	t   *testing.T
	dir string
	// estados es lo que contesta `commits/<sha>/status`; vacío, ninguno.
	estados string
	// marca es a qué apunta `desplegable/…` en la forja (MARCA_FORJA).
	marca string
}

func puertaNueva(t *testing.T) *puertaBanco {
	t.Helper()
	for _, h := range []string{"bun", "jq"} {
		if _, err := exec.LookPath(h); err != nil {
			t.Fatalf("falta %s para probar la puerta", h)
		}
	}
	// No vale t.TempDir(): lleva el nombre de la prueba, con sus espacios y
	// paréntesis, y esa ruta sustituye a /tmp/ DENTRO del texto de las funciones,
	// donde va sin comillas (`> /tmp/estado.json`). Un nombre como los de mktemp.
	dir, err := os.MkdirTemp("", "puerta-ci-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	b := &puertaBanco{t: t, dir: dir}
	guiontest.Doble(t, filepath.Join(b.dir, "bin"), "curl", puertaCurl)
	return b
}

func (b *puertaBanco) ruta(nombre string) string { return filepath.Join(b.dir, nombre) }

// bash ejecuta el cuerpo con las funciones de la puerta y del vigilante cargadas.
func (b *puertaBanco) bash(cuerpo string, env ...string) guiontest.Resultado {
	b.t.Helper()
	raiz := guiontest.Raiz(b.t)
	r := guiontest.Bash(b.t, guiontest.Opciones{Env: append([]string{
		guiontest.ConPATH(filepath.Join(b.dir, "bin")),
		"DIR=" + b.dir,
		"WF=" + filepath.Join(raiz, puertaFlujo),
		"VIGILANTE=" + filepath.Join(raiz, puertaVigilante),
		"FORJA=" + b.ruta("forja.json"),
		"PETICIONES=" + b.ruta("peticiones"),
		"ESTADOS=" + b.estados,
		"MARCA_FORJA=" + b.marca,
		"OBJETO_ETIQUETA=" + puertaObjetoEtiqueta,
		"AVERIA=", "SORDA=0",
	}, env...)}, puertaPreambulo+cuerpo)
	if r.Codigo == puertaSinFuncion && strings.Contains(r.Error, "no se encontró") {
		b.t.Fatal(strings.TrimSpace(r.Error))
	}
	return r
}

// publicar deja la forja montada donde la sirve el `curl` de mentira.
func (b *puertaBanco) publicar(f *puertaForja) {
	b.t.Helper()
	// Listas vacías y no `null`, como las deja `jq -s` sin entrada.
	forja := map[string]any{"check_runs": append([]any{}, f.checks...), "workflow_runs": append([]any{}, f.runs...)}
	raw, err := json.Marshal(forja)
	if err != nil {
		b.t.Fatal(err)
	}
	guiontest.Escribir(b.t, b.ruta("forja.json"), string(raw))
}

// puertaVeredicto es lo que dejó una pasada de la puerta: su veredicto, lo que
// dijo, cómo salió y lo que pidió.
type puertaVeredicto struct {
	estado, salida, peticiones string
	codigo                     int
}

// dice: ¿salió ese texto en lo que dijo la puerta?
func (v puertaVeredicto) dice(texto string) bool { return strings.Contains(v.salida, texto) }

// probar pasa la puerta contra la forja publicada. En subshell y con `set -e`,
// como en su paso: ante un 4xx o una lectura imposible hace `exit`.
func (b *puertaBanco) probar(averia string, sorda bool) puertaVeredicto {
	b.t.Helper()
	env := []string{"AVERIA=" + averia}
	if sorda {
		env = append(env, "SORDA=1")
	}
	r := b.bash(`: >"$PETICIONES"
(set -e; leer_estado; echo "estado=$estado") 2>&1`, env...)
	v := puertaVeredicto{salida: r.Texto(), codigo: r.Codigo}
	var estados []string
	for _, l := range strings.Split(v.salida, "\n") {
		if resto, ok := strings.CutPrefix(l, "estado="); ok {
			estados = append(estados, resto)
		}
	}
	v.estado = strings.Join(estados, "\n")
	v.peticiones = strings.ReplaceAll(guiontest.Leer(b.t, b.ruta("peticiones")), "\n", " ")
	return v
}

// vigilante es lo que dice el vigilante de la misma forja: su veredicto por la
// salida estándar y sus avisos por la de error.
func (b *puertaBanco) vigilante() (veredicto, avisos string) {
	b.t.Helper()
	r := b.bash(`estado_ci "$GITHUB_SHA"`)
	return r.Texto(), r.Error
}

// coincide comprueba que el vigilante dice lo que dice la puerta. Solo de «qué
// cuenta»: ante una página que no se lee cada uno hace lo suyo a propósito —la
// puerta reintenta, el vigilante sigue—.
func (b *puertaBanco) coincide(esperado string) {
	b.t.Helper()
	if v, _ := b.vigilante(); v != esperado {
		if v == "" {
			v = "nada"
		}
		b.t.Errorf("el vigilante dice «%s» y la puerta «%s»", v, esperado)
	}
}

// avisaConEnlace: ¿alguna línea de los avisos del vigilante dice «aviso, no
// frena» y lleva ese enlace?
func puertaAvisaConEnlace(avisos, enlace string) bool {
	for _, l := range strings.Split(avisos, "\n") {
		if strings.Contains(l, "aviso, no frena") && strings.Contains(l, enlace) {
			return true
		}
	}
	return false
}

// avisosEnElResumen cuenta las líneas del fichero de avisos (el que va al
// resumen) que casan con el patrón; 0 si no hay fichero.
func (b *puertaBanco) avisosEnElResumen(patron string) int {
	b.t.Helper()
	raw, err := os.ReadFile(b.ruta("avisos.md"))
	if err != nil {
		return 0
	}
	return len(regexp.MustCompile("(?m)"+patron).FindAllString(string(raw), -1))
}

// puertaCaso es un caso de «qué cuenta»: se publica la forja montada, se pasa la
// puerta y se compara el veredicto; después, el del vigilante. soloPuerta para
// lo que el vigilante no tiene: estados de commit y ejecución propia.
type puertaCaso struct {
	nombre     string
	esperado   string
	soloPuerta bool
	estados    string
	forja      func(f *puertaForja)
	ademas     func(t *testing.T, b *puertaBanco, v puertaVeredicto)
}

func puertaPasar(t *testing.T, casos []puertaCaso) {
	t.Helper()
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			b := puertaNueva(t)
			b.estados = c.estados
			f := &puertaForja{}
			c.forja(f)
			b.publicar(f)
			v := b.probar("", false)
			if v.estado != c.esperado {
				estado := v.estado
				if estado == "" {
					estado = "sin estado"
				}
				t.Errorf("la puerta dice «%s» y se esperaba «%s»\n%s", estado, c.esperado, v.salida)
			}
			if !c.soloPuerta {
				b.coincide(c.esperado)
			}
			if c.ademas != nil {
				c.ademas(t, b, v)
			}
		})
	}
}

// puertaSoloSeguridad es la release mínima en verde sobre la que se monta lo demás.
func puertaSoloSeguridad(f *puertaForja) {
	f.ejecucion(10, "seguridad", "release", "12:26", "success", "gitleaks=success")
}

// ─── QUÉ CUENTA: la última ejecución de cada flujo ───────────────────────────

func TestPuertaCICuentaLaUltimaEjecucionDeCadaFlujo(t *testing.T) {
	puertaPasar(t, []puertaCaso{
		// Lo que pidió el dueño, en sus tres formas.
		{
			nombre: "cancelado y después verde: pasa", esperado: "success",
			forja: func(f *puertaForja) {
				f.ejecucion(10, "seguridad", "release", "12:26", "cancelled", "gitleaks=cancelled", "trivy=cancelled")
				f.ejecucion(11, "seguridad", "workflow_dispatch", "13:00", "success", "gitleaks=success", "trivy=success")
			},
			ademas: func(t *testing.T, _ *puertaBanco, v puertaVeredicto) {
				if !v.dice("no cuenta seguridad, ejecución 10 (cancelled): la sustituye la 11") {
					t.Errorf("no deja escrito qué ejecución ha dejado de contar\n%s", v.salida)
				}
			},
		},
		{
			nombre: "verde y después rojo: bloquea", esperado: "failure",
			forja: func(f *puertaForja) {
				f.ejecucion(10, "seguridad", "workflow_dispatch", "12:00", "success", "gitleaks=success", "trivy=success")
				f.ejecucion(11, "seguridad", "release", "12:26", "failure", "gitleaks=success", "trivy=failure")
			},
			ademas: func(t *testing.T, _ *puertaBanco, v puertaVeredicto) {
				if !v.dice("✗ failure  trivy") {
					t.Errorf("no nombra el job que falla\n%s", v.salida)
				}
			},
		},
		{
			nombre: "cancelado sin repetir: bloquea", esperado: "failure",
			forja: func(f *puertaForja) {
				f.ejecucion(10, "seguridad", "release", "12:26", "cancelled", "gitleaks=cancelled", "trivy=cancelled")
				f.ejecucion(20, "ci-frontend", "release", "12:26", "success", "unitarias=success")
			},
		},
		// UN DESPACHO MANUAL CANCELADO NO ES DE LA CADENA NI UN VEREDICTO DEL CÓDIGO
		// (v3.18.3): tres despachos cancelados a los cuatro minutos, lanzados sobre una
		// rama de sesión que apuntaba al commit de la release, dejaron la publicación
		// bloqueada con la cadena entera en verde. Ya no cuentan —pero se ven, como los
		// barridos: aviso con su enlace, aquí y en el resumen.
		{
			nombre: "despachos manuales cancelados fuera de la cadena: no bloquean (v3.18.3)", esperado: "success",
			forja: func(f *puertaForja) {
				f.ejecucion(10, "seguridad", "release", "12:26", "success", "gitleaks=success")
				f.ejecucion(20, "ci-frontend", "release", "12:26", "success", "quality=success")
				f.ejecucion(30, "ci", "workflow_dispatch", "12:10", "cancelled", "go tests=cancelled", "go lint=cancelled")
				f.ejecucion(31, "seguridad", "workflow_dispatch", "12:10", "cancelled", "gitleaks=cancelled")
				f.ejecucion(32, "ci-desktop", "workflow_dispatch", "12:10", "cancelled", "validar=cancelled")
			},
			ademas: func(t *testing.T, b *puertaBanco, v puertaVeredicto) {
				for _, n := range []string{"30", "31", "32"} {
					if !v.dice("⚠ aviso, no bloquea") || !v.dice(puertaRuns+n) {
						t.Errorf("despacho cancelado: la puerta no avisa con el enlace de la ejecución %s", n)
					}
				}
				if n := b.avisosEnElResumen(`actions/runs/3[012]$`); n != 3 {
					t.Errorf("despachos cancelados: los tres avisos no llegan al fichero del resumen (hay %d)", n)
				}
				if _, dicho := b.vigilante(); !puertaAvisaConEnlace(dicho, "actions/runs/30") {
					t.Errorf("despacho cancelado: el vigilante no avisa con el enlace\n%s", dicho)
				}
			},
		},
		// Ni cuenta ni sustituye: lo que hubiera corrido antes queda como estaba. No tapa
		// un rojo anterior, y no fabrica un verde si solo hay ella.
		{
			nombre: "un despacho cancelado no tapa el rojo anterior del mismo flujo", esperado: "failure",
			forja: func(f *puertaForja) {
				f.ejecucion(10, "ci-frontend", "workflow_dispatch", "12:00", "failure", "quality=failure")
				f.ejecucion(11, "ci-frontend", "workflow_dispatch", "12:30", "cancelled", "quality=cancelled")
			},
		},
		{
			nombre: "un despacho cancelado no sustituye al verde anterior del mismo flujo", esperado: "success",
			forja: func(f *puertaForja) {
				f.ejecucion(10, "seguridad", "workflow_dispatch", "12:00", "success", "gitleaks=success")
				f.ejecucion(11, "seguridad", "workflow_dispatch", "12:30", "cancelled", "gitleaks=cancelled")
			},
		},
		// Y la release cancelada sigue bloqueando aunque su reintento manual también se
		// cancele (v3.10.1, v3.11.0): solo la levanta un despacho que TERMINE.
		{
			nombre: "una release cancelada sigue bloqueando aunque cancelen también su reintento", esperado: "failure",
			forja: func(f *puertaForja) {
				f.ejecucion(10, "seguridad", "release", "12:26", "cancelled", "gitleaks=cancelled")
				f.ejecucion(11, "seguridad", "workflow_dispatch", "13:00", "cancelled", "gitleaks=cancelled")
			},
		},
		{
			nombre: "relanzado y todavía en marcha: espera, no vale el verde anterior", esperado: "pending",
			forja: func(f *puertaForja) {
				f.ejecucion(10, "seguridad", "release", "12:26", "success", "gitleaks=success")
				f.ejecucion(11, "seguridad", "workflow_dispatch", "13:00", "", "gitleaks=")
			},
		},
		// Qué es «el mismo job». El nombre no basta: dos flujos pueden llamar igual a uno.
		{
			nombre: "el mismo nombre de job en dos flujos: el verde de uno no tapa el rojo del otro", esperado: "failure",
			forja: func(f *puertaForja) {
				f.ejecucion(10, "ci-desktop", "workflow_dispatch", "12:00", "failure", "validar=failure")
				f.ejecucion(20, "ci-mobile", "workflow_dispatch", "12:30", "success", "validar=success")
			},
		},
		// Y la ejecución entera sustituye a la anterior, no job a job: una matriz
		// cancelada antes de expandirse deja un check que nadie vuelve a producir. Es el
		// «trivy image (…)» sin expandir que v3.11.0 tiene en su `security` cancelado.
		{
			nombre: "un job que solo existió en la ejecución cancelada no bloquea a la que la repite", esperado: "success",
			forja: func(f *puertaForja) {
				f.ejecucion(10, "seguridad", "release", "12:26", "cancelled", "gitleaks=cancelled", "trivy (matriz sin expandir)=cancelled")
				f.ejecucion(11, "seguridad", "workflow_dispatch", "13:00", "success", "gitleaks=success", "trivy (cloud)=success", "trivy (suite)=success")
			},
		},
		// «La última» es la que empezó más tarde, no la de número más alto: repetir la
		// ejecución 10 después de la 11 la convierte en la última.
		{
			nombre: "una ejecución antigua repetida después cuenta como la última", esperado: "success",
			forja: func(f *puertaForja) {
				f.ejecucion(10, "seguridad", "release", "13:30", "success", "gitleaks=success")
				f.ejecucion(11, "seguridad", "workflow_dispatch", "13:00", "failure", "gitleaks=failure")
			},
		},
		// Lo que NO se puede dejar de ver al quedarse con la última.
		{
			nombre: "la última ejecución en cola y aún sin jobs: espera", esperado: "pending",
			forja: func(f *puertaForja) {
				puertaSoloSeguridad(f)
				f.ejecucion(11, "seguridad", "workflow_dispatch", "13:00", "")
			},
		},
		{
			nombre: "la última ejecución murió al arrancar, sin jobs: bloquea", esperado: "failure",
			forja: func(f *puertaForja) {
				puertaSoloSeguridad(f)
				f.ejecucion(11, "seguridad", "workflow_dispatch", "13:00", "startup_failure")
			},
		},
		{
			nombre: "un check externo en rojo (Codemagic, v3.19.4) no frena las imágenes", esperado: "success",
			forja: func(f *puertaForja) {
				puertaSoloSeguridad(f)
				f.suelto("Apps Fyne (TestFlight y Mac App Store)", "failure", "codemagic")
			},
			ademas: func(t *testing.T, _ *puertaBanco, v puertaVeredicto) {
				if !v.dice("⚠ aviso, no bloquea: el check externo «Apps Fyne (TestFlight y Mac App Store)»") {
					t.Errorf("check externo en rojo: la puerta no lo deja como aviso\n%s", v.salida)
				}
			},
		},
		// Salvo los de Xcode Cloud: archiva para la App Store sobre el mismo commit y un rojo
		// suyo —el cupo de subidas de Apple en v3.12.7— dejaba la release sin imágenes.
		{
			nombre: "un archivo de Xcode Cloud en rojo no frena las imágenes", esperado: "success",
			forja: func(f *puertaForja) {
				puertaSoloSeguridad(f)
				f.suelto("calcus | Default | Archive - iOS", "action_required", "xcode-cloud")
			},
		},
		// Tampoco sus ESTADOS de commit, que pone aparte de los checks (v3.12.28). Un estado en
		// rojo de cualquier otro sí frena. Solo la puerta: el vigilante no lee estados.
		{
			nombre: "un estado de Xcode Cloud en rojo no frena las imágenes", esperado: "success", soloPuerta: true,
			estados: `[{"context":"calcus | Default","state":"failure","creator":{"login":"xcode-cloud[bot]"}}]`,
			forja:   puertaSoloSeguridad,
		},
		{
			nombre: "un estado de Xcode Cloud sin creator tampoco frena (v3.13.0)", esperado: "success", soloPuerta: true,
			estados: `[{"context":"calcus | Default","state":"failure","target_url":"https://appstoreconnect.apple.com/teams/x/apps/1/ci/builds/2"}]`,
			forja:   puertaSoloSeguridad,
		},
		{
			nombre: "un estado externo en rojo de cualquiera tampoco frena", esperado: "success", soloPuerta: true,
			estados: `[{"context":"externa/despliegue","state":"failure","creator":{"login":"otra[bot]"}}]`,
			forja:   puertaSoloSeguridad,
			ademas: func(t *testing.T, _ *puertaBanco, v puertaVeredicto) {
				if !v.dice("⚠ aviso, no bloquea: el estado externo «externa/despliegue»") {
					t.Errorf("estado externo en rojo: la puerta no lo deja como aviso\n%s", v.salida)
				}
			},
		},
		// La propia puerta. Su ejecución anterior —la que se negó a publicar— dejaba un
		// job en rojo sobre el commit y la siguiente se lo encontraba: una negativa se
		// volvía definitiva. La de ahora (999) es la última de su flujo y no se cuenta.
		{
			nombre: "la negativa anterior de la propia puerta no bloquea el siguiente intento", esperado: "success", soloPuerta: true,
			forja: func(f *puertaForja) {
				puertaSoloSeguridad(f)
				f.ejecucion(30, "imagenes", "workflow_dispatch", "12:40", "failure", "Comprobar el CI=failure")
				f.ejecucion(999, "imagenes", "workflow_dispatch", "13:10", "", "Comprobar el CI=")
			},
		},
	})
}

// ─── LOS BARRIDOS PROGRAMADOS CONTRA PRODUCCIÓN: AVISAN, NO BLOQUEAN ─────────

func TestPuertaCILosBarridosProgramadosAvisanNoBloquean(t *testing.T) {
	puertaPasar(t, []puertaCaso{
		{
			nombre: "los dos barridos programados en rojo: no bloquean", esperado: "success",
			forja: func(f *puertaForja) {
				puertaSoloSeguridad(f)
				f.ejecucion(40, "escritorio", "schedule", "08:45", "failure", "barrido de ventana=failure")
				f.ejecucion(41, "mobile", "schedule", "07:00", "failure", "barrido móvil=failure", "barrido de pantallas=failure")
			},
			ademas: func(t *testing.T, b *puertaBanco, v puertaVeredicto) {
				for _, n := range []string{"40", "41"} {
					if !v.dice("⚠ aviso, no bloquea") || !v.dice(puertaRuns+n) {
						t.Errorf("barrido programado en rojo: la puerta no avisa con el enlace de la ejecución %s", n)
					}
				}
				if n := b.avisosEnElResumen(`actions/runs/4[01]$`); n != 2 {
					t.Errorf("barrido programado en rojo: los dos avisos no llegan al fichero del resumen (hay %d)", n)
				}
				if _, dicho := b.vigilante(); !puertaAvisaConEnlace(dicho, "actions/runs/40") {
					t.Errorf("barrido programado en rojo: el vigilante no avisa con el enlace\n%s", dicho)
				}
			},
		},
		// Y lanzados a mano tampoco: no son flujos de CI de la release (2026-11-08).
		{
			nombre: "un barrido lanzado a mano en rojo: no es de la release, no bloquea", esperado: "success",
			forja: func(f *puertaForja) {
				puertaSoloSeguridad(f)
				f.ejecucion(40, "escritorio", "workflow_dispatch", "13:00", "failure", "barrido de ventana=failure")
			},
		},
		// Y solo esos dos flujos: el `security` programado también es `schedule`, pero
		// mide el commit —un CVE publicado después de la release— y sigue bloqueando.
		{
			nombre: "un programado de otro flujo en rojo: bloquea", esperado: "failure",
			forja: func(f *puertaForja) {
				puertaSoloSeguridad(f)
				f.ejecucion(50, "seguridad", "schedule", "23:12", "failure", "gitleaks=success", "osv=failure")
			},
		},
		{
			nombre: "un barrido programado en marcha: no se le espera", esperado: "success",
			forja: func(f *puertaForja) {
				puertaSoloSeguridad(f)
				f.ejecucion(40, "escritorio", "schedule", "08:45", "", "barrido de ventana=")
			},
		},
	})
}

// ─── SOLO CUENTAN LOS FLUJOS DE CI DE LA RELEASE (2026-11-08) ─────────────────
//
// v3.19.4 se quedó sin imágenes con toda su release en verde: frenaban el `ci`
// despachado a mano aún en marcha, release-desktop y el check de Codemagic, en rojo
// por un token caducado. Nada de eso es un veredicto de estas imágenes.
func TestPuertaCISoloCuentanLosFlujosDeCIDeLaRelease(t *testing.T) {
	puertaPasar(t, []puertaCaso{
		{
			nombre: "el ci a mano, release-desktop y Codemagic no frenan la release en verde (v3.19.4)", esperado: "success",
			forja: func(f *puertaForja) {
				f.ejecucion(10, "seguridad", "release", "12:26", "success", "gitleaks=success")
				f.ejecucion(20, "ci-frontend", "release", "12:26", "success", "quality=success")
				f.ejecucion(60, "ci", "workflow_dispatch", "12:00", "", "frontend / accesibilidad-office=")
				f.ejecucion(61, "release-desktop", "workflow_dispatch", "12:30", "failure", "Apps Fyne (TestFlight y Mac App Store, Mac propio)=failure")
				f.suelto("Apps Fyne (TestFlight y Mac App Store)", "failure", "codemagic")
			},
			ademas: func(t *testing.T, _ *puertaBanco, v puertaVeredicto) {
				if !v.dice("⚠ aviso, no bloquea: release-desktop, ejecución 61") {
					t.Errorf("release-desktop en rojo: la puerta no lo deja como aviso\n%s", v.salida)
				}
			},
		},
		// Lo que sí es de la release sigue mandando, se llame su evento como se llame.
		{
			nombre: "un flujo de la release relanzado a mano en rojo sigue bloqueando", esperado: "failure",
			forja: func(f *puertaForja) {
				puertaSoloSeguridad(f)
				f.ejecucion(20, "ci-integration", "workflow_dispatch", "13:00", "failure", "integración=failure")
			},
		},
	})
}

// puertaLineas son las líneas de un fichero del repositorio.
func puertaLineas(t *testing.T, ruta string) []string {
	t.Helper()
	return strings.Split(strings.TrimSuffix(guiontest.Leer(t, ruta), "\n"), "\n")
}

// puertaRango son las líneas de `sed -n '/inicio/,/fin/p'`: desde cada línea que
// casa con inicio hasta la siguiente (nunca ella misma) que casa con fin.
func puertaRango(lineas []string, inicio, fin string) []string {
	reInicio, reFin := regexp.MustCompile(inicio), regexp.MustCompile(fin)
	var out []string
	dentro := false
	for _, l := range lineas {
		switch {
		case !dentro && reInicio.MatchString(l):
			dentro = true
			out = append(out, l)
		case dentro:
			out = append(out, l)
			dentro = !reFin.MatchString(l)
		}
	}
	return out
}

var (
	puertaRutaDeFlujo = regexp.MustCompile(`"\.github/workflows/[a-z-]*\.yml"`)
	// El `.` era un comodín también en el `grep -v seguridad.yml` original.
	puertaNombreAntiguo = regexp.MustCompile(`seguridad.yml`)
)

// puertaFlujosCitados son las rutas de workflow entre comillas que hay en esas
// líneas, ordenadas (`grep -o … | tr -d '"' | sort`).
func puertaFlujosCitados(lineas []string) []string {
	var out []string
	for _, l := range lineas {
		for _, cita := range puertaRutaDeFlujo.FindAllString(l, -1) {
			out = append(out, strings.Trim(cita, `"`))
		}
	}
	sort.Strings(out)
	return out
}

// puertaListaDeRelease es la lista de flujos de la release que hay escrita en
// esas líneas: sin seguridad.yml (el nombre antiguo de security, que las dos
// copias siguen aceptando) y sin repetidos (`grep -v seguridad.yml | sort -u`).
func puertaListaDeRelease(lineas []string) []string {
	var out []string
	for _, f := range puertaFlujosCitados(lineas) {
		if puertaNombreAntiguo.MatchString(f) {
			continue
		}
		if len(out) == 0 || out[len(out)-1] != f {
			out = append(out, f)
		}
	}
	return out
}

// puertaFlujosDelRepo son los workflows que cumplen la condición, con su ruta
// desde la raíz y ordenados.
func puertaFlujosDelRepo(t *testing.T, cumple func(lineas []string) bool) []string {
	t.Helper()
	raiz := guiontest.Raiz(t)
	rutas, err := filepath.Glob(filepath.Join(raiz, ".github", "workflows", "*.yml"))
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, ruta := range rutas {
		rel := ".github/workflows/" + filepath.Base(ruta)
		if cumple(puertaLineas(t, rel)) {
			out = append(out, rel)
		}
	}
	sort.Strings(out)
	return out
}

func puertaAlgunaCasa(lineas []string, patron string) bool {
	re := regexp.MustCompile(patron)
	for _, l := range lineas {
		if re.MatchString(l) {
			return true
		}
	}
	return false
}

// LA LISTA ES LA DE LOS FLUJOS QUE LANZA `on: release`, salvo sincronizar-suite, que
// copia y no valida. Si se añade uno a la release, la puerta y el vigilante tienen
// que enterarse a la vez.
func TestPuertaCILaPuertaYElVigilanteCuentanLosFlujosDeCIDeLaRelease(t *testing.T) {
	empiezaClave := regexp.MustCompile(`^[a-z]`)
	deRelease := puertaFlujosDelRepo(t, func(lineas []string) bool {
		// El bloque `on:` del workflow: lo que sigue a `on:` hasta la siguiente clave de raíz.
		var bloque []string
		dentro := false
		for _, l := range lineas {
			if strings.HasPrefix(l, "on:") {
				dentro = true
				continue
			}
			if dentro && empiezaClave.MatchString(l) {
				break
			}
			if dentro {
				bloque = append(bloque, l)
			}
		}
		return puertaAlgunaCasa(bloque, `^ +release:`)
	})
	var validan []string
	for _, f := range deRelease {
		if !strings.Contains(f, "sincronizar-suite") {
			validan = append(validan, f)
		}
	}
	enPuerta := puertaListaDeRelease(puertaRango(puertaLineas(t, puertaFlujo), `const DE_LA_RELEASE = `, `];`))
	enVigilante := puertaListaDeRelease(puertaRango(puertaLineas(t, puertaVigilante), `def de_la_release:`, `;$`))
	esperado := strings.Join(validan, " ")
	if esperado == "" || strings.Join(enPuerta, " ") != esperado || strings.Join(enVigilante, " ") != esperado {
		t.Fatalf("los flujos de CI de la release son «%s»; la puerta cuenta «%s» y el vigilante «%s»",
			esperado, strings.Join(enPuerta, " "), strings.Join(enVigilante, " "))
	}
}

// LA LISTA ES LA QUE ES. Los flujos que se dejan fuera son los que miden
// producción por `schedule`; si aparece un tercero, o uno deja de serlo, la puerta
// y el vigilante tienen que enterarse a la vez.
func TestPuertaCILaPuertaYElVigilanteDejanFueraLosBarridosDeProduccion(t *testing.T) {
	deVerdad := puertaFlujosDelRepo(t, func(lineas []string) bool {
		return puertaAlgunaCasa(lineas, `SYTUE_BASE_DOMAIN`) && puertaAlgunaCasa(lineas, `^  schedule:`)
	})
	soloCon := func(ruta, marca string) []string {
		var lineas []string
		for _, l := range puertaLineas(t, ruta) {
			if strings.Contains(l, marca) {
				lineas = append(lineas, l)
			}
		}
		return puertaFlujosCitados(lineas)
	}
	enPuerta := soloCon(puertaFlujo, "const BARRIDOS = ")
	enVigilante := soloCon(puertaVigilante, "def barrido:")
	esperado := strings.Join(deVerdad, " ")
	if esperado == "" || strings.Join(enPuerta, " ") != esperado || strings.Join(enVigilante, " ") != esperado {
		t.Fatalf("los flujos con schedule que miden producción son «%s»; la puerta deja fuera «%s» y el vigilante «%s»",
			esperado, strings.Join(enPuerta, " "), strings.Join(enVigilante, " "))
	}
}

// ─── QUE SE LEE ENTERO ───────────────────────────────────────────────────────

func TestPuertaCISeLeeEntero(t *testing.T) {
	sinEstado := func(v puertaVeredicto) string {
		if v.estado == "" {
			return "sin estado"
		}
		return v.estado
	}
	// EL CASO QUE SE ESCAPABA: 150 check runs y el único rojo es el 130, en la segunda
	// página. Leyendo solo la primera, la puerta decía «success».
	t.Run("un rojo en la segunda página (el 130 de 150) pone la puerta en failure y lo nombra", func(t *testing.T) {
		b := puertaNueva(t)
		b.publicar(puertaMuchosJobs(150, 130))
		if v := b.probar("", false); v.estado != "failure" || !v.dice("job-130") {
			t.Errorf("con 150 check runs y el 130 en rojo la puerta dice «%s» (pedido: %s)", sinEstado(v), v.peticiones)
		}
		b.coincide("failure")
	})
	t.Run("150 en verde: se leen las dos páginas y sale success", func(t *testing.T) {
		b := puertaNueva(t)
		b.publicar(puertaMuchosJobs(150, 0))
		if v := b.probar("", false); v.estado != "success" || v.peticiones != "check_runs:1 check_runs:2 workflow_runs:1 " {
			t.Errorf("150 en verde: estado «%s», pedido «%s»", sinEstado(v), v.peticiones)
		}
	})
	// Lo de todos los días no cuesta una petición de más: 57, una sola página de cada.
	t.Run("57 check runs: una petición por lista", func(t *testing.T) {
		b := puertaNueva(t)
		b.publicar(puertaMuchosJobs(57, 0))
		if v := b.probar("", false); v.estado != "success" || v.peticiones != "check_runs:1 workflow_runs:1 " {
			t.Errorf("57 en verde: estado «%s», pedido «%s»", sinEstado(v), v.peticiones)
		}
	})
	// Justo 100: una página llena no dice si hay más, así que se pide la segunda, vacía.
	t.Run("100 justos: se pide la segunda página y, vacía, sale success", func(t *testing.T) {
		b := puertaNueva(t)
		b.publicar(puertaMuchosJobs(100, 0))
		if v := b.probar("", false); v.estado != "success" || v.peticiones != "check_runs:1 check_runs:2 workflow_runs:1 " {
			t.Errorf("100 en verde: estado «%s», pedido «%s»", sinEstado(v), v.peticiones)
		}
	})
	// Una página que no se lee NO es un veredicto, ni de los checks ni de las ejecuciones.
	t.Run("un 502 en la segunda página de checks deja «sin-leer», no el veredicto de la primera", func(t *testing.T) {
		b := puertaNueva(t)
		b.publicar(puertaMuchosJobs(150, 130))
		if v := b.probar("check_runs:2", false); v.estado != "sin-leer" || v.codigo != 0 {
			t.Errorf("con un 502 en la segunda página de checks la puerta dice «%s» (salida %d)", sinEstado(v), v.codigo)
		}
	})
	t.Run("un 502 al leer las ejecuciones deja «sin-leer»", func(t *testing.T) {
		b := puertaNueva(t)
		b.publicar(puertaMuchosJobs(57, 0))
		if v := b.probar("workflow_runs:1", false); v.estado != "sin-leer" || v.codigo != 0 {
			t.Errorf("con un 502 al leer las ejecuciones la puerta dice «%s» (salida %d)", sinEstado(v), v.codigo)
		}
	})
	// Una API que no pagina no deja la puerta dando vueltas ni decidiendo con lo que tiene.
	t.Run("si la API devuelve siempre una página llena, la puerta falla en vez de decidir", func(t *testing.T) {
		b := puertaNueva(t)
		b.publicar(puertaMuchosJobs(150, 0))
		if v := b.probar("", true); v.codigo == 0 || v.estado != "" || !v.dice("no se pudo leer entero") {
			t.Errorf("con una API que no pagina: salida %d, estado «%s», pedido «%s»", v.codigo, sinEstado(v), v.peticiones)
		}
	})
	// Las ejecuciones también pasan de 100: 150 del mismo flujo, canceladas todas menos
	// la última. Si no se leyeran todas, las que faltan no se podrían dar por
	// sustituidas y sus cancelados contarían.
	t.Run("150 ejecuciones del mismo flujo: se leen todas y cuenta solo la última", func(t *testing.T) {
		b := puertaNueva(t)
		f := &puertaForja{}
		// De la 150 a la 1: la forja da primero lo más reciente.
		for i := 150; i >= 1; i-- {
			c := "cancelled"
			if i == 150 {
				c = "success"
			}
			f.runs = append(f.runs, map[string]any{
				"id": i, "workflow_id": "ci-frontend", "name": "ci-frontend", "path": ".github/workflows/ci-frontend.yml",
				"event": "workflow_dispatch", "status": "completed", "conclusion": c,
				"run_started_at": fmt.Sprintf("2026-10-05T%d:%02d:00Z", 10+i/60, i%60), "check_suite_id": i + 1000,
			})
			f.checks = append(f.checks, map[string]any{
				"name": "job", "status": "completed", "conclusion": c,
				"html_url":    fmt.Sprintf("%s%d/job/1", puertaRuns, i),
				"check_suite": map[string]any{"id": i + 1000},
			})
		}
		b.publicar(f)
		if v := b.probar("", false); v.estado != "success" ||
			v.peticiones != "check_runs:1 check_runs:2 workflow_runs:1 workflow_runs:2 " {
			t.Errorf("150 ejecuciones, la última en verde: estado «%s», pedido «%s»", sinEstado(v), v.peticiones)
		}
		b.coincide("success")
	})
}

// ─── LA CADENA NO REPITE LO YA PUBLICADO ─────────────────────────────────────

func TestPuertaCILaCadenaNoRepiteLoYaPublicado(t *testing.T) {
	// La forja de v3.11.1: la ejecución 30 publicó a mano y la 999, la de ahora, es la
	// que despacha la cadena. Se pasa antes la puerta, que es quien deja en su sitio la
	// lista de ejecuciones, como en el job.
	b := puertaNueva(t)
	f := &puertaForja{}
	puertaSoloSeguridad(f)
	f.ejecucion(30, "imagenes", "workflow_dispatch", "12:40", "success", "Comprobar el CI=success")
	f.ejecucion(999, "imagenes", "workflow_dispatch", "13:10", "", "Comprobar el CI=")
	b.publicar(f)
	b.probar("", false)
	resumen := b.ruta("resumen.md")
	// salta pregunta por la marca y dice si se salta la construcción (código 0) o
	// no, lo que dijo y lo que dejó en el resumen del paso.
	salta := func(t *testing.T, marca string) (codigo int, dicho, enResumen string) {
		t.Helper()
		b.t, b.marca = t, marca
		r := b.bash(`: >"$GITHUB_STEP_SUMMARY"
ya_publicadas 2>&1`, "PERFIL=completo", "TAG=v9.9.9", "GITHUB_STEP_SUMMARY="+resumen)
		return r.Codigo, r.Texto(), guiontest.Leer(t, resumen)
	}

	t.Run("la marca apunta a este commit: no se reconstruye, y el resumen dice quién publicó", func(t *testing.T) {
		codigo, dicho, enResumen := salta(t, puertaSHA)
		if codigo != 0 ||
			!strings.Contains(dicho, "imágenes ya publicadas por la ejecución 30 ("+puertaRuns+"30), no se reconstruyen") ||
			!strings.Contains(enResumen, "Imágenes ya publicadas por la ejecución 30") {
			t.Errorf("con la marca en este commit: salto=%d, dijo «%s», resumen «%s»", codigo, dicho, enResumen)
		}
	})
	// Por commit, no por nombre: la marca existe pero es de otro commit.
	t.Run("la marca existe pero apunta a otro commit: se construye, y lo dice", func(t *testing.T) {
		codigo, dicho, enResumen := salta(t, puertaOtro)
		if codigo == 0 || !strings.Contains(dicho, "apunta a "+puertaOtro+", no a "+puertaSHA+": se construye") || enResumen != "" {
			t.Errorf("con la marca en otro commit: salto=%d, dijo «%s»", codigo, dicho)
		}
	})
	t.Run("sin marca: se construye", func(t *testing.T) {
		if codigo, dicho, _ := salta(t, ""); codigo == 0 || !strings.Contains(dicho, "no existe todavía: se construye") {
			t.Errorf("sin marca: salto=%d, dijo «%s»", codigo, dicho)
		}
	})
	// Ante la duda se construye: repetir cuesta runner, saltarse una publicación no.
	t.Run("si la marca no se puede leer: se construye", func(t *testing.T) {
		if codigo, dicho, _ := salta(t, "averia"); codigo == 0 || !strings.Contains(dicho, "ante la duda, se construye") {
			t.Errorf("con un 502 al leer la marca: salto=%d, dijo «%s»", codigo, dicho)
		}
	})
	// LA MARCA ES ANOTADA desde que lleva los digest de lo publicado: la referencia
	// apunta al objeto etiqueta y el commit está un salto más allá. Se sigue.
	t.Run("una marca anotada que apunta a este commit: no se reconstruye", func(t *testing.T) {
		if codigo, dicho, _ := salta(t, "anotada:"+puertaSHA); codigo != 0 || !strings.Contains(dicho, "no se reconstruyen") {
			t.Errorf("con la marca anotada en este commit: salto=%d, dijo «%s»", codigo, dicho)
		}
	})
	t.Run("una marca anotada de otro commit: se construye", func(t *testing.T) {
		if codigo, dicho, _ := salta(t, "anotada:"+puertaOtro); codigo == 0 ||
			!strings.Contains(dicho, "apunta a "+puertaOtro+", no a "+puertaSHA+": se construye") {
			t.Errorf("con la marca anotada en otro commit: salto=%d, dijo «%s»", codigo, dicho)
		}
	})
	t.Run("si la etiqueta anotada no se puede leer: se construye", func(t *testing.T) {
		if codigo, dicho, _ := salta(t, "anotada:averia"); codigo == 0 || !strings.Contains(dicho, "ante la duda, se construye") {
			t.Errorf("con un 502 al leer la etiqueta anotada: salto=%d, dijo «%s»", codigo, dicho)
		}
	})
	t.Run("una marca que no apunta a un commit: se construye", func(t *testing.T) {
		if codigo, dicho, _ := salta(t, "anotada:arbol"); codigo == 0 || !strings.Contains(dicho, "se construye") {
			t.Errorf("con una etiqueta anotada que no apunta a un commit: salto=%d, dijo «%s»", codigo, dicho)
		}
	})
}

// Y LAS CONDICIONES DE LOS JOBS, que no se pueden ejecutar aquí pero sí leer: la
// pregunta solo se hace si el despacho viene de la cadena —a mano se construye
// siempre—; con la respuesta «sí» no corren ni la construcción ni el marcado; y
// el despacho de escritorio corre por los dos caminos.
func TestPuertaCISoloLaCadenaPreguntaYConYaPublicadasNoSeConstruye(t *testing.T) {
	if _, err := exec.LookPath("bun"); err != nil {
		t.Fatal("falta bun para probar la puerta")
	}
	r := guiontest.Ejecutar(t, guiontest.Opciones{Env: []string{"WF=" + filepath.Join(guiontest.Raiz(t), puertaFlujo)}},
		"bun", "-e", `
  const y = Bun.YAML.parse(await Bun.file(Bun.env.WF).text());
  const plano = (v) => String(v).replace(/\s+/g, " ");
  console.log("salida: " + plano(y.jobs.preparar.outputs.ya_publicadas));
  console.log("paso: " + plano(y.jobs.preparar.steps.find((s) => s.id === "ya")?.if));
  console.log("construir: " + plano(y.jobs.construir.if));
  console.log("cerrar: " + plano(y.jobs.cerrar.if));
  console.log("marcar: " + plano(y.jobs.marcar.if) + " needs=" + plano(y.jobs.marcar.needs));
  console.log("escritorio: " + plano(y.jobs["publicar-escritorio"].if));`)
	condiciones := r.Texto()
	// tiene: ¿hay una línea de las condiciones con esos dos textos?
	tiene := func(a, b string) bool {
		for _, l := range strings.Split(condiciones, "\n") {
			if strings.Contains(l, a) && strings.Contains(l, b) {
				return true
			}
		}
		return false
	}
	const (
		si = "needs.preparar.outputs.ya_publicadas == 'si'"
		no = "needs.preparar.outputs.ya_publicadas != 'si'"
	)
	for _, par := range [][2]string{
		{"salida: ", "steps.ya.outputs.ya_publicadas"},
		{"paso: ", "github.event.inputs.pipeline_release == 'true'"},
		{"paso: ", "steps.decidir.outputs.publicar == 'si'"},
		{"construir: ", no},
		{"cerrar: ", no},
		{"escritorio: ", "always()"},
		{"escritorio: ", si},
		{"marcar: ", "needs.cerrar.result == 'success'"},
		{"marcar: ", "needs=preparar,cerrar"},
		{"escritorio: ", "needs.marcar.result == 'success'"},
		{"escritorio: ", "github.event.inputs.pipeline_release == 'true'"},
	} {
		if !tiene(par[0], par[1]) {
			t.Errorf("las condiciones de los jobs de imagenes-produccion no son las del salto: falta «%s» en «%s»\n%s\n%s",
				par[1], par[0], condiciones, r.Error)
		}
	}
}

// ─── v3.11.0 TAL COMO QUEDÓ ──────────────────────────────────────────────────
//
// c3c6c0d4a: una tanda de los seis flujos a las 12:09 (con `quality` y «scripts de
// producción» en rojo) y, a las 12:26, los cinco de la release, cancelados. 57 check
// runs: 19 cancelados y 2 fallidos. La última ejecución de cinco flujos es la
// cancelada, así que la puerta sigue diciendo «failure» —relanzarlos ANTES de la
// release no los sustituye—, pero ya no por los dos rojos de la tanda.
func TestPuertaCIv3110TalComoQuedo(t *testing.T) {
	b := puertaNueva(t)
	guiontest.Escribir(t, b.ruta("forja.json"), guiontest.Leer(t, puertaReal))
	v := b.probar("", false)
	cancelados := 0
	for _, l := range strings.Split(v.salida, "\n") {
		if strings.Contains(l, "✗ cancelled") {
			cancelados++
		}
	}
	if v.estado != "failure" || cancelados != 19 || v.dice("✗ failure") ||
		!v.dice("no cuenta ci-frontend-sytue-git, ejecución 37307593232 (failure): la sustituye la 37309589428") {
		t.Errorf("v3.11.0: estado «%s», %d cancelados contados (se esperaba failure con 19 y ningún «✗ failure»)\n%s",
			v.estado, cancelados, v.salida)
	}
	b.coincide("failure")
}

// ─── CI-06: ignorar_ci NUNCA SALTA security ──────────────────────────────────

func TestPuertaCIIgnorarCINuncaSaltaSecurity(t *testing.T) {
	// decidir publica la forja, pasa la puerta y decide con ignorar_ci marcado:
	// devuelve cómo salió, lo que dijo y si dejó `publicar=si` en la salida del paso.
	decidir := func(t *testing.T, montar func(f *puertaForja)) (codigo int, dicho string, publica bool) {
		t.Helper()
		b := puertaNueva(t)
		f := &puertaForja{}
		montar(f)
		b.publicar(f)
		salidaDelPaso := b.ruta("github_output")
		r := b.bash(`: >"$GITHUB_OUTPUT"
(set -e; leer_estado; decidir_publicacion) 2>&1`, "IGNORAR=true", "GITHUB_OUTPUT="+salidaDelPaso)
		for _, l := range strings.Split(guiontest.Leer(t, salidaDelPaso), "\n") {
			publica = publica || l == "publicar=si"
		}
		return r.Codigo, r.Texto(), publica
	}

	t.Run("CI-06: con security en verde y otro flujo en rojo, ignorar_ci permite publicar", func(t *testing.T) {
		codigo, dicho, publica := decidir(t, func(f *puertaForja) {
			f.ejecucion(10, "seguridad", "release", "12:26", "success", "gitleaks=success", "trivy=success")
			f.ejecucion(20, "ci-frontend", "release", "12:26", "failure", "test=failure")
		})
		if codigo != 0 || !publica {
			t.Errorf("CI-06: con security en verde y otro flujo en rojo, ignorar_ci falló (rc=%d): %s", codigo, dicho)
		}
	})
	t.Run("CI-06: con security en failure, ignorar_ci rechaza publicar", func(t *testing.T) {
		codigo, dicho, publica := decidir(t, func(f *puertaForja) {
			f.ejecucion(10, "seguridad", "release", "12:26", "failure", "gitleaks=success", "trivy=failure")
			f.ejecucion(20, "ci-frontend", "release", "12:26", "success", "test=success")
		})
		if codigo == 0 || publica || !strings.Contains(dicho, "ignorar_ci nunca salta security") {
			t.Errorf("CI-06: con security en failure, ignorar_ci no rechazó publicar (rc=%d): %s", codigo, dicho)
		}
	})
	t.Run("CI-06: con security cancelado, ignorar_ci rechaza publicar", func(t *testing.T) {
		codigo, _, publica := decidir(t, func(f *puertaForja) {
			f.ejecucion(10, "seguridad", "release", "12:26", "cancelled", "gitleaks=cancelled")
		})
		if codigo == 0 || publica {
			t.Errorf("CI-06: con security cancelado, ignorar_ci permitió publicar (rc=%d)", codigo)
		}
	})
	t.Run("CI-06: con security aún en marcha, ignorar_ci rechaza publicar", func(t *testing.T) {
		codigo, _, publica := decidir(t, func(f *puertaForja) {
			f.ejecucion(10, "seguridad", "release", "12:26", "", "gitleaks=")
		})
		if codigo == 0 || publica {
			t.Errorf("CI-06: con security en marcha, ignorar_ci permitió publicar (rc=%d)", codigo)
		}
	})
	t.Run("CI-06: sin ejecución de security sobre el commit, ignorar_ci rechaza publicar", func(t *testing.T) {
		codigo, _, publica := decidir(t, func(f *puertaForja) {
			f.ejecucion(20, "ci-frontend", "release", "12:26", "success", "test=success")
		})
		if codigo == 0 || publica {
			t.Errorf("CI-06: sin ejecución de security, ignorar_ci permitió publicar (rc=%d)", codigo)
		}
	})
}
