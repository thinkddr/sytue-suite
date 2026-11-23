// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package cicontratos

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// El CI se despacha a mano, por release o por programación. Cerrar una PR no
// necesita otro workflow: no hay ejecuciones de PR que cancelar. La entrada común
// (`ci.yml`) delega en los gates existentes, sin ampliarse permisos ni cancelar a
// sus propios hijos. Se lee el YAML de verdad para no confundir los comentarios
// históricos con eventos, interfaces o concurrencia.

type flujo = map[string]any

const sufijoDespacho = "${{ github.event_name == 'workflow_dispatch' && format('-{0}', github.run_id) || '' }}"

// leerFlujos carga cada workflow como un único documento YAML, por nombre de fichero.
func leerFlujos(t *testing.T) map[string]flujo {
	t.Helper()
	dir := filepath.Join("..", "..", ".github", "workflows")
	var rutas []string
	for _, patron := range []string{"*.yml", "*.yaml"} {
		mas, err := filepath.Glob(filepath.Join(dir, patron))
		if err != nil {
			t.Fatal(err)
		}
		rutas = append(rutas, mas...)
	}
	if len(rutas) == 0 {
		t.Fatal("no hay workflows que comprobar")
	}
	out := map[string]flujo{}
	for _, ruta := range rutas {
		raw, err := os.ReadFile(ruta)
		if err != nil {
			t.Fatal(err)
		}
		out[filepath.Base(ruta)] = decodificar(t, filepath.Base(ruta), raw)
	}
	return out
}

func decodificar(t *testing.T, nombre string, raw []byte) flujo {
	t.Helper()
	dec := yaml.NewDecoder(strings.NewReader(string(raw)))
	var doc flujo
	if err := dec.Decode(&doc); err != nil {
		t.Fatalf("%s: %v", nombre, err)
	}
	var otro any
	if err := dec.Decode(&otro); err == nil {
		t.Fatalf("%s: cada workflow debe contener un único documento YAML", nombre)
	}
	return doc
}

func mapa(v any) map[string]any {
	m, _ := v.(map[string]any)
	return m
}

func claves(v any) []string {
	out := make([]string, 0, len(mapa(v)))
	for k := range mapa(v) {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func eventos(f flujo) []string {
	switch on := f["on"].(type) {
	case map[string]any:
		return claves(on)
	case []any:
		out := []string{}
		for _, e := range on {
			if s, ok := e.(string); ok {
				out = append(out, s)
			}
		}
		return out
	case string:
		return []string{on}
	}
	return nil
}

func grupo(f flujo) (string, bool) {
	g, ok := mapa(f["concurrency"])["group"].(string)
	return g, ok
}

// expresion quita los espacios y el envoltorio `${{ }}`: no cambian el contrato.
func expresion(v any) string {
	s, _ := v.(string)
	s = strings.Join(strings.Fields(s), "")
	return strings.TrimSuffix(strings.TrimPrefix(s, "${{"), "}}")
}

// erroresDeDespacho devuelve todo lo que incumple el contrato, o nada.
func erroresDeDespacho(flujos map[string]flujo) []string {
	var errores []string
	mal := func(formato string, args ...string) {
		for _, a := range args {
			formato = strings.Replace(formato, "%s", a, 1)
		}
		errores = append(errores, formato)
	}
	nombres := make([]string, 0, len(flujos))
	for n := range flujos {
		nombres = append(nombres, n)
	}
	sort.Strings(nombres)
	for _, n := range nombres {
		for _, e := range eventos(flujos[n]) {
			if e == "pull_request" || e == "pull_request_target" {
				mal("%s: el evento %s lanzaría CI automático de PR", n, e)
			}
		}
	}
	for _, ci := range []string{"ci-go", "ci-frontend", "ci-integration", "ci-desktop", "ci-mobile", "security", "crossplane"} {
		f, ok := flujos[ci+".yml"]
		if !ok {
			mal("%s.yml: falta el workflow de CI", ci)
			continue
		}
		if !slices.Contains(eventos(f), "workflow_dispatch") {
			mal("%s.yml: falta workflow_dispatch", ci)
		}
		if g, ok := grupo(f); !ok {
			mal("%s.yml: falta el grupo de concurrencia", ci)
		} else if !strings.Contains(g, sufijoDespacho) {
			mal("%s.yml: cada workflow_dispatch debe conservar su sufijo run_id", ci)
		}
	}
	entrada, ok := flujos["ci.yml"]
	if !ok {
		mal("ci.yml: falta la entrada común de CI")
		return errores
	}
	if !reflect.DeepEqual(eventos(entrada), []string{"workflow_dispatch"}) {
		mal("ci.yml: la entrada común debe ejecutarse solo por despacho manual")
	}
	soloLectura := map[string]any{"contents": "read"}
	if !reflect.DeepEqual(entrada["permissions"], any(soloLectura)) {
		mal("ci.yml: los permisos globales deben limitarse a contents: read")
	}
	jobs := mapa(entrada["jobs"])
	if !reflect.DeepEqual(claves(jobs), []string{"frontend", "go", "integration"}) {
		mal("ci.yml: debe delegar Go, frontend e integración sin añadir otra tanda")
	}
	inputs := mapa(mapa(mapa(entrada["on"])["workflow_dispatch"])["inputs"])
	if !reflect.DeepEqual(claves(inputs), []string{"medir_fidelidad", "verbose"}) {
		mal("ci.yml: la entrada común debe validar toda la tanda, sin filtrar áreas ni paquetes")
	}
	for _, in := range []string{"verbose", "medir_fidelidad"} {
		if mapa(inputs[in])["type"] != "boolean" || mapa(inputs[in])["default"] != false {
			mal("ci.yml: verbose y medir_fidelidad deben conservar sus tipos y valores por defecto")
			break
		}
	}
	padre, hayGrupo := grupo(entrada)
	if !hayGrupo {
		mal("ci.yml: falta un grupo de concurrencia propio")
	} else if !strings.Contains(padre, "github.run_id") {
		mal("ci.yml: dos despachos manuales no deben cancelarse entre sí")
	}
	for _, area := range []string{"go", "frontend", "integration"} {
		job := mapa(jobs[area])
		_, pasos := job["steps"]
		_, runner := job["runs-on"]
		if job["uses"] != "./.github/workflows/ci-"+area+".yml" || pasos || runner {
			mal("ci.yml: %s debe reutilizar su gate local sin duplicar la ejecución", area)
		}
		// La publicación cuenta la última ejecución del workflow. Una tanda parcial
		// verde no puede sustituir otra completa que salió roja.
		_, filtro := job["if"]
		_, depende := job["needs"]
		if filtro || depende {
			mal("ci.yml: %s debe ejecutarse siempre, sin filtros ni dependencias entre áreas", area)
		}
		if _, hereda := job["secrets"]; hereda {
			mal("ci.yml: %s no necesita heredar secretos para ejecutar su gate", area)
		}
		permisos, hayPermisos := job["permissions"]
		if area == "frontend" {
			if !reflect.DeepEqual(permisos, any(map[string]any{"contents": "read", "issues": "write"})) {
				mal("ci.yml: solo frontend necesita issues: write para su informe")
			}
		} else if hayPermisos && !reflect.DeepEqual(permisos, any(soloLectura)) {
			mal("ci.yml: %s debe limitarse a contents: read", area)
		}
		hijo := flujos["ci-"+area+".yml"]
		if !slices.Contains(eventos(hijo), "workflow_call") {
			mal("ci-%s.yml: el gate debe poder llamarse desde la entrada común", area)
		}
		quiero := []string{"release", "workflow_call", "workflow_dispatch"}
		if area == "go" {
			quiero = []string{"workflow_call", "workflow_dispatch"}
		}
		tipos := mapa(mapa(hijo["on"])["release"])["types"]
		if !reflect.DeepEqual(eventos(hijo), quiero) ||
			(area != "go" && !reflect.DeepEqual(tipos, any([]any{"published"}))) {
			mal("ci-%s.yml: reutilizar el gate no debe cambiar sus eventos anteriores", area)
		}
		// En workflow_call, github.workflow sigue siendo el del PADRE. Prefijos
		// distintos impiden que el hijo cancele a quien lo llama.
		if g, _ := grupo(hijo); strings.SplitN(padre, "${{", 2)[0] == strings.SplitN(g, "${{", 2)[0] {
			mal("ci.yml y ci-%s.yml: padre e hijo comparten el grupo de concurrencia", area)
		}
	}
	conIntegracion := mapa(mapa(jobs["integration"])["with"])
	if expresion(mapa(mapa(jobs["go"])["with"])["medir_fidelidad"]) != "inputs.medir_fidelidad" ||
		expresion(conIntegracion["verbose"]) != "inputs.verbose&&'si'||'no'" {
		mal("ci.yml: debe reenviar los inputs y traducir verbose a la interfaz si/no de integración")
	}
	if _, acota := conIntegracion["paquetes"]; acota {
		mal("ci.yml: integración debe usar todos los paquetes por defecto, sin acotar la validación común")
	}
	llamada := func(nombre string) map[string]any {
		return mapa(mapa(mapa(flujos[nombre]["on"])["workflow_call"])["inputs"])
	}
	if in := mapa(llamada("ci-go.yml")["medir_fidelidad"]); in["type"] != "boolean" || in["default"] != false {
		mal("ci-go.yml: workflow_call debe aceptar medir_fidelidad como boolean opcional")
	}
	in := llamada("ci-integration.yml")
	if mapa(in["paquetes"])["type"] != "string" || mapa(in["paquetes"])["default"] != "./..." ||
		mapa(in["verbose"])["type"] != "string" || mapa(in["verbose"])["default"] != "no" {
		mal("ci-integration.yml: workflow_call debe aceptar paquetes y verbose compatibles con el despacho directo")
	}
	return errores
}

func TestElCIManualEsComunCompletoYSinEventosDePR(t *testing.T) {
	for _, e := range erroresDeDespacho(leerFlujos(t)) {
		t.Error(e)
	}
}

// Un guardián que no se ha visto en rojo no protege nada: cada mutación de un
// workflow real tiene que hacer saltar su regla, y solo esa.
func TestElContratoDeDespachoFallaCuandoDebe(t *testing.T) {
	for _, c := range []struct {
		nombre, fichero, antes, despues, quiero string
	}{
		{
			"permisos de escritura", "ci.yml", "\n  contents: read\n\nconcurrency:", "\n  contents: write\n\nconcurrency:",
			"ci.yml: los permisos globales deben limitarse a contents: read",
		},
		{
			"CI automático de PR", "ci.yml", "on:\n  workflow_dispatch:", "on:\n  pull_request:\n  workflow_dispatch:",
			"ci.yml: el evento pull_request lanzaría CI automático de PR",
		},
		{
			"una tanda filtrada", "ci.yml", "  integration:\n    uses:", "  integration:\n    if: false\n    uses:",
			"ci.yml: integration debe ejecutarse siempre, sin filtros ni dependencias entre áreas",
		},
		{
			"despachos que se cancelan", "ci-go.yml", "format('-{0}', github.run_id)", "format('-{0}', github.sha)",
			"ci-go.yml: cada workflow_dispatch debe conservar su sufijo run_id",
		},
	} {
		t.Run(c.nombre, func(t *testing.T) {
			flujos := leerFlujos(t)
			raw, err := os.ReadFile(filepath.Join("..", "..", ".github", "workflows", c.fichero))
			if err != nil {
				t.Fatal(err)
			}
			if strings.Count(string(raw), c.antes) != 1 {
				t.Fatalf("%s ya no contiene una sola vez %q: la mutación no prueba nada", c.fichero, c.antes)
			}
			flujos[c.fichero] = decodificar(t, c.fichero, []byte(strings.Replace(string(raw), c.antes, c.despues, 1)))
			if errores := erroresDeDespacho(flujos); !slices.Contains(errores, c.quiero) {
				t.Fatalf("el contrato aceptó la mutación; errores: %q", errores)
			}
		})
	}
}

// Lo que mide en un navegador durante minutos decide al publicar, no en cada
// iteración: dentro de `ci` (el flujo que se despacha a mano) no corre. En un
// workflow llamado, `github.workflow` es el del padre.
func TestLosJobsPesadosDelFrontendNoCorrenDentroDeCi(t *testing.T) {
	jobs := mapa(leerFlujos(t)["ci-frontend.yml"]["jobs"])
	for _, job := range []string{"lighthouse", "accesibilidad-office"} {
		if got := expresion(mapa(jobs[job])["if"]); got != "github.workflow!='ci'" {
			t.Errorf("ci-frontend.yml: %s debe saltarse dentro de ci (if = %q)", job, got)
		}
	}
	// Una medición que no bloquea tampoco corre en la release.
	if got := expresion(mapa(jobs["bench-calcus"])["if"]); got != "github.workflow!='ci'&&github.event_name!='release'" {
		t.Errorf("ci-frontend.yml: bench-calcus solo corre al despacharlo a mano (if = %q)", got)
	}
	// Los que deciden cada iteración no llevan filtro.
	for _, job := range []string{"quality", "almacenamiento-externo"} {
		if _, filtro := mapa(jobs[job])["if"]; filtro || jobs[job] == nil {
			t.Errorf("ci-frontend.yml: %s debe correr siempre", job)
		}
	}
}

// Cada suite, una vez por commit: el arnés e2e no se repite en dos jobs, y el
// flujo horario del importador solo prueba lo que guarda.
func TestNingunaSuiteSeRepiteNiCuelgaDeUnFlujoAjeno(t *testing.T) {
	flujos := leerFlujos(t)
	veces := 0
	for _, job := range mapa(flujos["ci-frontend.yml"]["jobs"]) {
		pasos, _ := mapa(job)["steps"].([]any)
		for _, paso := range pasos {
			p := mapa(paso)
			if p["working-directory"] == "test/e2e" && p["run"] == "bun run test" {
				veces++
			}
		}
	}
	if veces != 1 {
		t.Errorf("ci-frontend.yml: las regresiones del arnés e2e corren %d veces por commit; quiero 1", veces)
	}
	pasos, _ := mapa(mapa(flujos["reaprovechar-prs-suite.yml"]["jobs"])["verificar"])["steps"].([]any)
	for _, paso := range pasos {
		run, _ := mapa(paso)["run"].(string)
		for _, ajeno := range []string{"test/e2e", "apps/fyne", "tools/astro-fyne", "prettier"} {
			if strings.Contains(run, ajeno) {
				t.Errorf("reaprovechar-prs-suite.yml: el flujo horario prueba %s, que no es suyo", ajeno)
			}
		}
	}
}

func pasos(job any) []map[string]any {
	lista, _ := mapa(job)["steps"].([]any)
	out := make([]map[string]any, 0, len(lista))
	for _, p := range lista {
		out = append(out, mapa(p))
	}
	return out
}

// La release no repite lo que otro flujo suyo ya prueba sobre el mismo commit:
// `ci-desktop` compila y prueba apps/nucleo y apps/fyne con -race, así que
// `ci-mobile` no lo hace otra vez; y el formato no es un veredicto de lo publicado.
func TestLaReleaseNoRepiteLoQueYaPruebaOtroDeSusFlujos(t *testing.T) {
	flujos := leerFlujos(t)
	const fueraDeRelease = "github.event_name!='release'"
	repetidos := 0
	for _, p := range pasos(mapa(flujos["ci-mobile.yml"]["jobs"])["validar-movil"]) {
		run, _ := p["run"].(string)
		if !strings.Contains(run, "go test ./...") && !strings.Contains(run, "go mod tidy -diff") &&
			!strings.Contains(run, "go list -m") {
			continue
		}
		repetidos++
		if expresion(p["if"]) != fueraDeRelease {
			t.Errorf("ci-mobile.yml: %q repite en la release lo que ya hace ci-desktop", p["name"])
		}
	}
	if repetidos != 3 {
		t.Fatalf("ci-mobile.yml: esperaba 3 pasos compartidos con ci-desktop y hay %d: revisa este contrato", repetidos)
	}
	// Y ci-desktop tiene que seguir haciéndolo en la release, o nadie lo haría.
	prueba := false
	for _, p := range pasos(mapa(flujos["ci-desktop.yml"]["jobs"])["validar-escritorio"]) {
		run, _ := p["run"].(string)
		if strings.Contains(run, "for modulo in apps/nucleo apps/fyne") && strings.Contains(run, "go test -race") {
			prueba = !strings.Contains(expresion(p["if"]), "release")
		}
	}
	if !prueba || !slices.Contains(eventos(flujos["ci-desktop.yml"]), "release") {
		t.Error("ci-desktop.yml: debe probar apps/nucleo y apps/fyne con -race en cada release")
	}
	formato := false
	for _, p := range pasos(mapa(flujos["ci-frontend.yml"]["jobs"])["quality"]) {
		if run, _ := p["run"].(string); strings.Contains(run, "format:check") {
			formato = expresion(p["if"]) == fueraDeRelease
		}
	}
	if !formato {
		t.Error("ci-frontend.yml: el formato no se comprueba en la release")
	}
}
