// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package cicontratos

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/tools/guiontest"
)

// El ejecutor de pasos de Codemagic (.github/actions/apple/codemagic-pasos.py)
// hace lo que haría el runner con los pasos de un job de GitHub Actions, y nada
// que no sepa hacer.

const codemagicPasosCargar = `
import io
pasos = cargar("codemagic_pasos", APPLE / "codemagic-pasos.py")
WORKFLOW = RAIZ / ".github/workflows/release-desktop.yml"
MARCAR = "Marcar en la release la subida a TestFlight"
`

const codemagicPasosMarcar = "Marcar en la release la subida a TestFlight"

// codemagicPasosSeco corre el job `tiendas` de release-desktop en seco con ese
// SYTUE_MODO y devuelve el código y los pasos que se ejecutarían, en orden.
func codemagicPasosSeco(t *testing.T, modo string) (int, []string) {
	t.Helper()
	var r struct {
		Codigo int      `json:"codigo"`
		Corren []string `json:"corren"`
	}
	pyRestoCorrer(t, codemagicPasosCargar+`
salida = io.StringIO()
entorno = {"PATH": os.environ["PATH"], "TAG": "v3.18.3", "SYTUE_MODO": ENTRADA, "SYTUE_PLOMERIA": "0" * 40}
codigo = pasos.ejecutar(WORKFLOW, "tiendas", {MARCAR}, seco=True, entorno=entorno, salida=salida)
devolver({"codigo": codigo, "corren": [l for l in salida.getvalue().splitlines()]})
`, &r, pyRestoConEntrada(t, modo))
	var corren []string
	for _, l := range r.Corren {
		if nombre, ok := strings.CutSuffix(l, ": se ejecutaría"); ok {
			corren = append(corren, strings.TrimPrefix(nombre, "── "))
		}
	}
	return r.Codigo, corren
}

func TestCodemagicPasosCadaBuildPublicaSoloSuTiendaYNoEscribeEnGitHub(t *testing.T) {
	comunes := []string{
		"Clonar el tag y la plomería del workflow", "Seleccionar apps de las tiendas",
		"Apple Distribution, instalador y un perfil de tienda por app",
		"Perfiles de tienda de apps y widgets (solo con los widgets de Apple)", "Go y la CLI Fyne fijada",
	}
	codigo, ios := codemagicPasosSeco(t, "ios")
	esperadoIOS := append(append([]string{}, comunes[:4]...), "Copias temporales de perfiles existentes para Xcode", comunes[4],
		"iOS a TestFlight (pruebas internas)", "Retirar exclusivamente el contexto de firma del job")
	if codigo != 0 || !reflect.DeepEqual(ios, esperadoIOS) {
		t.Fatalf("ios: código %d, pasos\n%q\nno\n%q", codigo, ios, esperadoIOS)
	}
	codigo, mas := codemagicPasosSeco(t, "mas")
	esperadoMAS := append(append([]string{}, comunes...), "Mac App Store (sin enviar a revisión)",
		"Retirar exclusivamente el contexto de firma del job")
	if codigo != 0 || !reflect.DeepEqual(mas, esperadoMAS) {
		t.Fatalf("mas: código %d, pasos\n%q\nno\n%q", codigo, mas, esperadoMAS)
	}
	for _, corren := range [][]string{ios, mas} {
		if slices.Contains(corren, codemagicPasosMarcar) {
			t.Fatalf("el build de Codemagic marca la release en GitHub: %q", corren)
		}
	}
}

func TestCodemagicPasosSinModoNoHayBuildDeLasDosTiendasALaVezPorDescuido(t *testing.T) {
	// El lanzador encarga un build por tienda (el máximo de Codemagic es 120 min);
	// sin SYTUE_MODO el job correría las dos, que es justo lo que no cabe.
	_, corren := codemagicPasosSeco(t, "")
	for _, paso := range []string{"iOS a TestFlight (pruebas internas)", "Mac App Store (sin enviar a revisión)"} {
		if !slices.Contains(corren, paso) {
			t.Fatalf("sin modo no correría «%s»: %q", paso, corren)
		}
	}
	if lanzador := guiontest.Leer(t, "deploy/sytue-git/bin/codemagic-lanzar.sh"); !strings.Contains(lanzador, "ios | mas) ;;") {
		t.Fatal("codemagic-lanzar.sh ya no exige SYTUE_MODO ios o mas")
	}
}

// --- Expresiones -------------------------------------------------------------

const codemagicPasosContexto = `
def contexto(**entorno):
    return pasos.Contexto({"SYTUE_MODO": "ios", "TAG": "v1.2.3", "SYTUE_GIT_TOKEN": "lectura", **entorno})
`

func TestCodemagicPasosExpresionesAdmitidas(t *testing.T) {
	casos := map[string]any{
		"inputs.solo_ios": "true", "!inputs.solo_mas": true, "github.event.inputs.ref || github.ref_name": "v1.2.3",
		"inputs.apps_ios || 'todas'": "todas", "steps.apple_go.outcome == 'success'": true,
		"!cancelled() && steps.apple_go.outcome == 'success' && !inputs.solo_ios": false,
		"always()": true, "secrets.GITHUB_TOKEN": "lectura", "job.status": "success",
		"(inputs.solo_ios || inputs.solo_mas) && !inputs.solo_escritorio": true,
	}
	expresiones := make([]string, 0, len(casos))
	for e := range casos {
		expresiones = append(expresiones, e)
	}
	var r map[string]any
	pyRestoCorrer(t, codemagicPasosCargar+codemagicPasosContexto+`
c = contexto()
c.pasos["apple_go"] = "success"
devolver({e: pasos.evaluar(e, c) for e in ENTRADA})
`, &r, pyRestoConEntrada(t, expresiones))
	for expresion, esperado := range casos {
		t.Run(expresion, func(t *testing.T) {
			if r[expresion] != esperado {
				t.Fatalf("evaluar(%q) = %#v, no %#v", expresion, r[expresion], esperado)
			}
		})
	}
}

func TestCodemagicPasosLoQueNoSabeHacerEsUnErrorYNoUnaCadenaVacia(t *testing.T) {
	expresiones := []string{
		"github.token", "inputs.no_existe", "needs.macos.result", "hashFiles('x')",
		"fromJSON(inputs.ref)", "inputs.ref ==", "matrix.plataforma", "vars.X",
	}
	var r map[string]*pyRestoExcepcion
	pyRestoCorrer(t, codemagicPasosCargar+codemagicPasosContexto+`
devolver({e: fallo(pasos.evaluar, e, contexto()) for e in ENTRADA})
`, &r, pyRestoConEntrada(t, expresiones))
	for _, expresion := range expresiones {
		t.Run(expresion, func(t *testing.T) {
			pyRestoLanza(t, r[expresion], "ErrorPasos", "")
		})
	}
}

// --- Ejecución ---------------------------------------------------------------

type codemagicPasosEjecucion struct {
	Codigo int               `json:"codigo"`
	Traza  string            `json:"traza"`
	Salida string            `json:"salida"`
	Error  *pyRestoExcepcion `json:"error"`
}

// codemagicPasosCorrer ejecuta el job `j` del workflow dado, desde su
// directorio temporal y saltando el paso «no corre».
func codemagicPasosCorrer(t *testing.T, cuerpo string, entorno map[string]string) codemagicPasosEjecucion {
	t.Helper()
	tmp := t.TempDir()
	guiontest.Escribir(t, filepath.Join(tmp, "w.yml"), cuerpo)
	var r codemagicPasosEjecucion
	pyRestoCorrer(t, codemagicPasosCargar+`
tmp = pathlib.Path(ENTRADA["tmp"])
salida = io.StringIO()
base = {"PATH": os.environ["PATH"], "TAG": "v1.2.3", "SYTUE_MODO": "ios", **ENTRADA["entorno"]}
anterior = os.getcwd()
os.chdir(tmp)
try:
    try:
        codigo, error = pasos.ejecutar(tmp / "w.yml", "j", {"no corre"}, entorno=base, salida=salida), None
    except Exception as e:
        codigo, error = None, excepcion(e)
finally:
    os.chdir(anterior)
traza = (tmp / "traza").read_text() if (tmp / "traza").exists() else ""
devolver({"codigo": codigo, "traza": traza, "salida": salida.getvalue(), "error": error})
`, &r, pyRestoConEntrada(t, map[string]any{"tmp": tmp, "entorno": entorno}))
	return r
}

func TestCodemagicPasosEntornoCaminoSecretosYEstadoEntrePasos(t *testing.T) {
	r := codemagicPasosCorrer(t, `
env:
  TAG: ${{ github.event.inputs.ref || github.ref_name }}
jobs:
  j:
    env:
      NIVEL: job
    steps:
      - &comun
        name: escribe
        id: uno
        env:
          TOKEN: ${{ secrets.GITHUB_TOKEN }}
        run: |
          mkdir -p "$RUNNER_TEMP/bin"
          printf '#!/bin/sh\necho herramienta\n' > "$RUNNER_TEMP/bin/herramienta"
          chmod +x "$RUNNER_TEMP/bin/herramienta"
          echo "$RUNNER_TEMP/bin" >> "$GITHUB_PATH"
          echo "SIMPLE=uno" >> "$GITHUB_ENV"
          { echo 'MULTI<<FIN'; echo 'a=b'; echo 'c'; echo 'FIN'; } >> "$GITHUB_ENV"
          echo "token=$TOKEN tag=$TAG nivel=$NIVEL" >> traza
      - name: no corre
        run: echo "no debería" >> traza
      - name: lee
        if: ${{ steps.uno.outcome == 'success' && inputs.solo_ios }}
        run: |
          echo "simple=$SIMPLE multi=$(printf '%s' "$MULTI" | tr '\n' '|') $(herramienta)" >> traza
          false
      - name: tras el fallo
        run: echo "no debería" >> traza
      - name: siempre
        if: always()
        env:
          ESTADO: ${{ job.status }}
        run: echo "siempre estado=$ESTADO" >> traza
`, map[string]string{"SYTUE_GIT_TOKEN": "lectura"})
	if r.Error != nil {
		t.Fatalf("ejecutar falló: %s", r.Error.Mensaje)
	}
	if r.Codigo != 1 {
		t.Fatalf("código %d, no 1\n%s", r.Codigo, r.Salida)
	}
	esperado := []string{"token=lectura tag=v1.2.3 nivel=job", "simple=uno multi=a=b|c herramienta", "siempre estado=failure"}
	if traza := strings.Split(strings.TrimRight(r.Traza, "\n"), "\n"); !reflect.DeepEqual(traza, esperado) {
		t.Fatalf("traza\n%q\nno\n%q", traza, esperado)
	}
	for _, s := range []string{"── no corre: se salta", "── tras el fallo: skipped"} {
		if !strings.Contains(r.Salida, s) {
			t.Fatalf("la salida no dice %q\n%s", s, r.Salida)
		}
	}
}

func TestCodemagicPasosUnPasoConUnaAccionDeGitHubNoSeEjecuta(t *testing.T) {
	r := codemagicPasosCorrer(t, `
jobs:
  j:
    steps:
      - name: fuera
        uses: actions/checkout@v4
`, map[string]string{})
	pyRestoLanza(t, r.Error, "ErrorPasos", "")
}

// --- Respaldo ----------------------------------------------------------------

// Si Codemagic no publica una tienda, el job relanza el workflow con motor=mac
// solo para esa tienda (decisión del dueño, 2026-11-05), y no marca TestFlight.

type codemagicPasosRespaldo struct {
	codigo   int
	lanzados []string
	despacho map[string]any
	env      string
	salida   string
}

// codemagicPasosPasoEncargar es el `run` del paso que encarga las tiendas a
// Codemagic en el job tiendas-codemagic, leído con el lector del propio ejecutor.
func codemagicPasosPasoEncargar(t *testing.T) string {
	t.Helper()
	var run string
	pyRestoCorrer(t, codemagicPasosCargar+`
job = pasos.leer_workflow(WORKFLOW)["jobs"]["tiendas-codemagic"]
paso = next(p for p in job["steps"] if p.get("name") == "Encargar iOS y la Mac App Store a Codemagic")
devolver(paso["run"])
`, &run)
	return run
}

func codemagicPasosRespaldar(t *testing.T, codIOS, codMAS, token, soloIOS, soloMAS string) codemagicPasosRespaldo {
	t.Helper()
	run := codemagicPasosPasoEncargar(t)
	raiz := t.TempDir()
	// Lanzador falso: sale con el código que toque a su tienda.
	guiontest.Escribir(t, filepath.Join(raiz, "deploy", "sytue-git", "bin", "codemagic-lanzar.sh"),
		"echo \"$SYTUE_MODO\" >> \"$TRAZA_LANZADOR\"\n"+
			"case \"$SYTUE_MODO\" in ios) exit \"$COD_IOS\" ;; mas) exit \"$COD_MAS\" ;; esac\n")
	binarios := filepath.Join(raiz, "bin")
	guiontest.Doble(t, binarios, "curl", `while [ "$#" -gt 0 ]; do [ "$1" = --data ] && printf "%s" "$2" > "$DESPACHO"; shift; done
echo 204`)
	res := guiontest.Ejecutar(t, guiontest.Opciones{Dir: raiz, Limpio: true, Env: []string{
		guiontest.ConPATH(binarios), "TAG=v1.2.3",
		"SYTUE_SOLO_IOS=" + soloIOS, "SYTUE_SOLO_MAS=" + soloMAS, "SYTUE_CREAR_PERFILES=false",
		"CODEMAGIC_TOKEN=" + token, "TOKEN_DESPACHO=t", "SYTUE_APPS_IOS=", "SYTUE_APPS_MAS=",
		"GITHUB_SHA=" + strings.Repeat("a", 40), "GITHUB_REF_NAME=main", "GITHUB_API_URL=https://api.github.com",
		"GITHUB_REPOSITORY=thinkddr/sytue-one", "GITHUB_SERVER_URL=https://github.com",
		"GITHUB_ENV=" + filepath.Join(raiz, "env"), "TRAZA_LANZADOR=" + filepath.Join(raiz, "lanzados"),
		"DESPACHO=" + filepath.Join(raiz, "despacho"), "COD_IOS=" + codIOS, "COD_MAS=" + codMAS,
	}}, "bash", "-eo", "pipefail", "-c", run)
	leer := func(n string) string {
		raw, err := os.ReadFile(filepath.Join(raiz, n)) //nolint:gosec // G304: fichero del directorio temporal.
		if err != nil {
			return ""
		}
		return string(raw)
	}
	r := codemagicPasosRespaldo{codigo: res.Codigo, lanzados: strings.Fields(leer("lanzados")), env: leer("env"), salida: res.Todo()}
	if d := leer("despacho"); d != "" {
		if err := json.Unmarshal([]byte(d), &r.despacho); err != nil {
			t.Fatalf("despacho ilegible: %v\n%s", err, d)
		}
	}
	return r
}

func TestCodemagicPasosRespaldoTodoBienNoRelanzaYDejaLoQueLeeLaMarca(t *testing.T) {
	r := codemagicPasosRespaldar(t, "0", "0", "x", "false", "false")
	if r.codigo != 0 || !slices.Equal(r.lanzados, []string{"ios", "mas"}) || r.despacho != nil {
		t.Fatalf("código %d, lanzados %v, despacho %v; se esperaba 0, [ios mas] y ninguno\n%s", r.codigo, r.lanzados, r.despacho, r.salida)
	}
	if !strings.Contains(r.env, "SYTUE_APPS_IOS_SELECCIONADAS=") {
		t.Fatalf("GITHUB_ENV no deja SYTUE_APPS_IOS_SELECCIONADAS:\n%s", r.env)
	}
}

func TestCodemagicPasosRespaldoSoloLaTiendaQueFallaVaAlMac(t *testing.T) {
	for _, caso := range []struct {
		nombre, codIOS, codMAS, soloIOS, soloMAS, clave string
	}{
		{"ios", "1", "0", "true", "false", "solo_ios"},
		{"mas", "0", "1", "false", "true", "solo_mas"},
	} {
		t.Run(caso.nombre, func(t *testing.T) {
			r := codemagicPasosRespaldar(t, caso.codIOS, caso.codMAS, "x", "false", "false")
			esperado := map[string]any{"ref": "main", "inputs": map[string]any{
				"ref": "v1.2.3", "motor": "mac",
				"crear_perfiles": "false", caso.clave: "true",
			}}
			if r.codigo != 1 || !reflect.DeepEqual(r.despacho, esperado) || r.env != "" {
				t.Fatalf("código %d, despacho %v, env %q; se esperaba 1, %v y vacío\n%s", r.codigo, r.despacho, r.env, esperado, r.salida)
			}
		})
	}
}

func TestCodemagicPasosRespaldoSinCuotaOSinTokenVanLasDosAlMac(t *testing.T) {
	for _, caso := range []struct {
		nombre, codIOS, token string
		lanzados              []string
	}{
		{"sin-cuota", "3", "x", []string{"ios"}},
		{"sin-token", "0", "", nil},
	} {
		t.Run(caso.nombre, func(t *testing.T) {
			r := codemagicPasosRespaldar(t, caso.codIOS, "0", caso.token, "false", "false")
			if r.codigo != 1 || !slices.Equal(r.lanzados, caso.lanzados) {
				t.Fatalf("código %d, lanzados %v; se esperaba 1 y %v\n%s", r.codigo, r.lanzados, caso.lanzados, r.salida)
			}
			if r.despacho == nil || !reflect.DeepEqual(r.despacho["inputs"], map[string]any{"ref": "v1.2.3", "motor": "mac", "crear_perfiles": "false"}) {
				t.Fatalf("despacho %v", r.despacho)
			}
		})
	}
}
