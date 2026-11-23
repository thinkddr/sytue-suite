// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package cicontratos

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"git.sytue.eu/sytue/one/tools/guiontest"
)

// UN ABORTO DE RENOVATE NO ES «TODO AL DÍA» (auditoría 2026-10-06, CI-12 y CI-02).
//
// Prueba scripts/renovate-resultado.sh contra registros como los que escribe
// Renovate, y que dependencias.yml lo usa: sin esa llamada el guion existe y no
// decide nada. El registro del aborto es el de la ejecución 37293277959.

const renovateResultadoGuion = "scripts/renovate-resultado.sh"

// Contrato de dryRun=lookup en Renovate 44.190.2: no hay result al completar,
// pero un aborto sí lo añade. No se puede reutilizar sólo su código de salida.
const (
	renovateResultadoInicio     = `{"repository":"thinkddr/sytue-one","logContext":"actual","msg":"Repository started"}`
	renovateResultadoExtraccion = `{"repository":"thinkddr/sytue-one","logContext":"actual","msg":"Dependency extraction complete"}`
	renovateResultadoConsultas  = `{"repository":"thinkddr/sytue-one","logContext":"actual","level":20,"msg":"Package releases lookups complete"}`
	renovateResultadoCierre     = `{"repository":"thinkddr/sytue-one","logContext":"actual","msg":"Repository finished"}`
)

// renovateResultadoCaso: el guion, con ese registro y en ese modo, tiene que
// salir con ese código. sinRegistro = el fichero no existe.
type renovateResultadoCaso struct {
	modo        string
	esperado    int
	que         string
	lineas      []string
	sinRegistro bool
}

func renovateResultadoCasos() []renovateResultadoCaso {
	inicio, extraccion := renovateResultadoInicio, renovateResultadoExtraccion
	consultas, cierre := renovateResultadoConsultas, renovateResultadoCierre
	return []renovateResultadoCaso{
		{modo: "full", esperado: 0, que: "un repositorio procesado entero es verde", lineas: []string{
			`{"level":30,"msg":"Dependency extraction complete"}`,
			`{"level":30,"cloned":true,"durationMs":278182,"result":"done","status":"onboarded","msg":"Repository finished"}`,
		}},
		{modo: "full", esperado: 1, que: "un aborto por un host externo es rojo aunque Renovate salga con 0", lineas: []string{
			`{"level":40,"hostType":"npm","packageName":"oxlint-tsgolint","msg":"Host error"}`,
			`{"level":30,"msg":"External host error causing abort - skipping"}`,
			`{"level":30,"cloned":true,"durationMs":278182,"result":"external-host-error","status":"onboarded","msg":"Repository finished"}`,
		}},
		{modo: "full", esperado: 1, que: "un registro que no llega a «Repository finished» es rojo", lineas: []string{
			`{"level":30,"msg":"Dependency extraction complete"}`,
		}},
		{modo: "full", esperado: 1, que: "repository-changed sigue siendo un aborto, no un resultado completo", lineas: []string{
			`{"level":30,"result":"repository-changed","msg":"Repository finished"}`,
		}},
		{modo: "full", esperado: 1, que: "sin registro no hay veredicto, y eso es rojo", sinRegistro: true},
		{modo: "full", esperado: 1, que: "un registro vacío es rojo", lineas: []string{""}},
		{modo: "full", esperado: 0, que: "las líneas que no son JSON no estorban", lineas: []string{
			`npm warn deprecated glob@6.0.4`,
			`{"level":30,"result":"done","msg":"Repository finished"}`,
		}},
		{
			modo: "lookup", esperado: 0, que: "lookup completo tiene extracción, consultas y cierre sin result",
			lineas: []string{inicio, extraccion, consultas, cierre},
		},
		{
			modo: "full", esperado: 1, que: "el modo normal sigue rechazando el cierre sin result",
			lineas: []string{inicio, extraccion, consultas, cierre},
		},
		{modo: "lookup", esperado: 1, que: "lookup rechaza un aborto incluso después de consultar versiones", lineas: []string{
			inicio, extraccion, consultas,
			`{"repository":"thinkddr/sytue-one","logContext":"actual","result":"external-host-error","msg":"Repository finished"}`,
		}},
		{
			modo: "lookup", esperado: 1, que: "lookup exige extracción antes de las consultas",
			lineas: []string{inicio, consultas, extraccion, cierre},
		},
		{
			modo: "lookup", esperado: 1, que: "lookup no acepta sólo extracción y cierre",
			lineas: []string{inicio, extraccion, cierre},
		},
		{
			modo: "lookup", esperado: 1, que: "lookup no acepta consultas sin cierre",
			lineas: []string{inicio, extraccion, consultas},
		},
		{modo: "lookup", esperado: 1, que: "lookup no mezcla el marcador de otro repositorio", lineas: []string{
			inicio, extraccion,
			`{"repository":"otro/repo","logContext":"actual","msg":"Package releases lookups complete"}`, cierre,
		}},
		{modo: "lookup", esperado: 1, que: "lookup no mezcla el marcador de otro contexto", lineas: []string{
			inicio, extraccion,
			`{"repository":"thinkddr/sytue-one","logContext":"anterior","msg":"Package releases lookups complete"}`, cierre,
		}},
		{
			modo: "lookup", esperado: 1, que: "un intento nuevo incompleto invalida el lookup anterior",
			lineas: []string{inicio, extraccion, consultas, cierre, inicio},
		},
		{modo: "lookup", esperado: 1, que: "lookup no acepta un cierre sin inicio ni contexto", lineas: []string{cierre}},
		{modo: "lookup", esperado: 1, que: "lookup no acepta result explícito aunque sea done", lineas: []string{
			inicio, extraccion, consultas,
			`{"repository":"thinkddr/sytue-one","logContext":"actual","result":"done","msg":"Repository finished"}`,
		}},
	}
}

func TestRenovateResultadoDecideConElRegistro(t *testing.T) {
	if _, err := os.Stat(filepath.Join(guiontest.Raiz(t), renovateResultadoGuion)); err != nil {
		t.Fatal("no existe scripts/renovate-resultado.sh")
	}
	casos := renovateResultadoCasos()
	if len(casos) != 18 {
		t.Fatalf("hay %d casos y el guion original tenía 18", len(casos))
	}
	for _, c := range casos {
		t.Run(c.modo+": "+c.que, func(t *testing.T) {
			registro := filepath.Join(t.TempDir(), "registro")
			if !c.sinRegistro {
				// Como `printf '%s\n' "$@"`: cada línea con su salto.
				guiontest.Escribir(t, registro, strings.Join(c.lineas, "\n")+"\n")
			}
			if r := guiontest.Guion(t, guiontest.Opciones{}, renovateResultadoGuion, registro, c.modo); r.Codigo != c.esperado {
				t.Fatalf("%s: salió con %d y se esperaba %d\n%s", c.que, r.Codigo, c.esperado, r.Todo())
			}
		})
	}
}

const renovateResultadoPropuestas = `{"config":{"npm":[{"deps":[{"depName":"astro","currentValue":">=7.4.1 <8","datasource":"npm","updates":[{"newValue":">=7.4.2 <8","updateType":"patch","sourceUrl":"https://synthetic.invalid/private"}]}]}]},"token":"synthetic-secret"}
{"upgrade":{"depName":"qs","currentVersion":"6.16.1","newValue":"6.17.0","updateType":"patch","datasource":"npm","password":"synthetic-secret"}}
{"deps":[{"depName":"synthetic-image","currentValue":"1.2.3","currentDigest":"sha256:0000000000000000000000000000000000000000000000000000000000000000","updates":[{"newValue":"1.2.3","newDigest":"sha256:1111111111111111111111111111111111111111111111111111111111111111","updateType":"digest"}]}]}
{"deps":[{"depName":"unsafe?token=synthetic-secret","updates":[{"newValue":"1.2.3"}]},{"depName":"unsafe\nrow","newValue":"2.0.0"},{"depName":"https://user:synthetic-secret@synthetic.invalid/repo","newValue":"1.0.0"}]}
`

// El informe usa el nombre del padre para updates[] y sólo exporta propuestas.
func TestRenovateResultadoElInformeConservaUpdatesYOmiteLoAjeno(t *testing.T) {
	registro := filepath.Join(t.TempDir(), "propuestas-log")
	guiontest.Escribir(t, registro, renovateResultadoPropuestas)
	r := guiontest.Guion(t, guiontest.Opciones{}, renovateResultadoGuion, "--propuestas", registro)
	if r.Codigo != 0 {
		t.Fatalf("--propuestas salió con %d: %s", r.Codigo, r.Error)
	}
	var filas []map[string]any
	if err := json.Unmarshal([]byte(r.Salida), &filas); err != nil {
		t.Fatalf("el informe no es un array JSON de objetos: %v\n%s", err, r.Salida)
	}
	const pierde = "el informe pierde las propuestas anidadas o publica campos ajenos"
	if len(filas) != 3 {
		t.Fatalf("%s: %d propuestas y se esperaban 3\n%s", pierde, len(filas), r.Salida)
	}
	hay := func(f func(map[string]any) bool) bool { return slices.ContainsFunc(filas, f) }
	if !hay(func(f map[string]any) bool {
		return f["depName"] == "astro" && f["currentValue"] == ">=7.4.1 <8" && f["newValue"] == ">=7.4.2 <8"
	}) {
		t.Errorf("%s: falta astro con el nombre del padre\n%s", pierde, r.Salida)
	}
	if !hay(func(f map[string]any) bool {
		return f["depName"] == "qs" && f["currentValue"] == "6.16.1" && f["newValue"] == "6.17.0"
	}) {
		t.Errorf("%s: falta qs\n%s", pierde, r.Salida)
	}
	if !hay(func(f map[string]any) bool {
		return f["depName"] == "synthetic-image" && f["currentDigest"] != f["newDigest"] && f["newDigest"] != nil
	}) {
		t.Errorf("%s: falta el cambio de digest de synthetic-image\n%s", pierde, r.Salida)
	}
	quiero := []string{"currentDigest", "currentValue", "datasource", "depName", "newDigest", "newValue", "newVersion", "updateType"}
	for _, f := range filas {
		if !reflect.DeepEqual(claves(f), quiero) {
			t.Errorf("%s: una propuesta lleva los campos %v", pierde, claves(f))
		}
	}
	if strings.Contains(r.Salida, "synthetic-secret") {
		t.Errorf("%s: sale una credencial\n%s", pierde, r.Salida)
	}
}

// renovateResultadoFlujoSinComentarios es dependencias.yml sin las líneas que
// son solo comentario (lo que dejaba `grep -v '^[[:space:]]*#'`).
func renovateResultadoFlujoSinComentarios(t *testing.T) string {
	t.Helper()
	comentario := regexp.MustCompile(`^\s*#`)
	var lineas []string
	for _, linea := range strings.Split(guiontest.Leer(t, ".github/workflows/dependencias.yml"), "\n") {
		if !comentario.MatchString(linea) {
			lineas = append(lineas, linea)
		}
	}
	return strings.Join(lineas, "\n")
}

// renovateResultadoJob es el job `actualizar` de dependencias.yml, y el flujo entero.
func renovateResultadoJob(t *testing.T) (job map[string]any, f flujo) {
	t.Helper()
	f, ok := leerFlujos(t)["dependencias.yml"]
	if !ok {
		t.Fatal("no existe .github/workflows/dependencias.yml")
	}
	return mapa(mapa(f["jobs"])["actualizar"]), f
}

// renovateResultadoCampo junta ese campo de los pasos que se llaman así.
func renovateResultadoCampo(job map[string]any, campo string, nombres ...string) []any {
	var out []any
	for _, p := range pasos(job) {
		if nombre, _ := p["name"].(string); slices.Contains(nombres, nombre) {
			out = append(out, p[campo])
		}
	}
	return out
}

// renovateResultadoTodos: son exactamente n valores, todos texto, y cada uno
// contiene todas esas cadenas y ninguna de las prohibidas.
func renovateResultadoTodos(valores []any, n int, contienen, prohibidas []string) bool {
	if len(valores) != n {
		return false
	}
	for _, v := range valores {
		s, ok := v.(string)
		if !ok {
			return false
		}
		for _, c := range contienen {
			if !strings.Contains(s, c) {
				return false
			}
		}
		for _, p := range prohibidas {
			if strings.Contains(s, p) {
				return false
			}
		}
	}
	return true
}

// El flujo lo llama con el registro de Renovate, dentro del mismo `if` que lo ejecuta.
func TestRenovateResultadoElFlujoLoLlamaConElRegistro(t *testing.T) {
	texto := renovateResultadoFlujoSinComentarios(t)
	t.Run("el workflow conserva el diagnóstico sanitizado cuando Renovate aborta", func(t *testing.T) {
		if !regexp.MustCompile(`scripts/renovate-resultado.sh" --diagnostico "\$LOG_FILE"`).MatchString(texto) {
			t.Fatal("el workflow deja mudo el aborto de Renovate")
		}
	})
	t.Run("dependencias.yml decide con el resultado del registro", func(t *testing.T) {
		if !regexp.MustCompile(`scripts/renovate-resultado.sh" "\$LOG_FILE"`).MatchString(texto) {
			t.Fatal("dependencias.yml no llama a scripts/renovate-resultado.sh con el registro de Renovate")
		}
	})
	// Renovate, con su árbol fijado: del lockfile, sin scripts y sin resolver nada al vuelo.
	t.Run("Renovate se instala de tools/renovate/bun.lock, sin scripts de instalación", func(t *testing.T) {
		info, err := os.Stat(filepath.Join(guiontest.Raiz(t), "tools", "renovate", "bun.lock"))
		if !strings.Contains(texto, "bun install --frozen-lockfile --ignore-scripts") || err != nil || info.Size() == 0 {
			t.Fatal("Renovate no se instala de un lockfile con --frozen-lockfile --ignore-scripts")
		}
	})
}

// El modo se comparte con el informe, y lookup registra su marcador debug.
func TestRenovateResultadoLookupComparteModoYNoPublicaIncidencias(t *testing.T) {
	job, _ := renovateResultadoJob(t)
	const mal = "lookup pierde su contrato de cierre o puede publicar incidencias"
	if modo, _ := mapa(job["env"])["RENOVATE_DRY_RUN"].(string); !strings.Contains(modo, "inputs.dry == 'lookup'") {
		t.Errorf("%s: RENOVATE_DRY_RUN = %q", mal, modo)
	}
	debug := false
	for _, env := range renovateResultadoCampo(job, "env", "Ejecutar Renovate") {
		debug = debug || mapa(env)["LOG_FILE_LEVEL"] == "debug"
	}
	if !debug {
		t.Errorf("%s: «Ejecutar Renovate» no registra en debug", mal)
	}
	runs := renovateResultadoCampo(job, "run", "Ejecutar Renovate", "Qué hay más nuevo que lo que llevamos")
	if !renovateResultadoTodos(runs, 2, []string{`"${RENOVATE_DRY_RUN:-full}"`}, nil) {
		t.Errorf("%s: la ejecución y el informe no comparten el modo", mal)
	}
	ifs := renovateResultadoCampo(job, "if", "Publicar las actualizaciones pendientes como incidencia", "Informe si Renovate falla")
	if !renovateResultadoTodos(ifs, 2, []string{"inputs.dry != 'true'", "inputs.dry != 'lookup'"}, nil) {
		t.Errorf("%s: las incidencias no excluyen los dos modos de prueba: %v", mal, ifs)
	}
}

func TestRenovateResultadoSoloSeConservaElInformeSanitizado(t *testing.T) {
	job, _ := renovateResultadoJob(t)
	const mal = "el workflow pierde las propuestas o exporta el registro debug completo"
	env := mapa(job["env"])
	informe, _ := env["RENOVATE_REPORT"].(string)
	if !strings.Contains(informe, "github.workspace") || !strings.HasSuffix(informe, "/.artifacts/renovate/propuestas.json") {
		t.Errorf("%s: RENOVATE_REPORT = %q", mal, informe)
	}
	cobertura, _ := env["DEPENDENCIAS_COBERTURA"].(string)
	if !strings.Contains(cobertura, "catálogo raíz de Bun requiere auditoría manual") || !strings.Contains(cobertura, "cero propuestas") {
		t.Errorf("%s: DEPENDENCIAS_COBERTURA = %q", mal, cobertura)
	}
	extrae := false
	for _, run := range renovateResultadoCampo(job, "run", "Qué hay más nuevo que lo que llevamos") {
		extrae = extrae || renovateResultadoTodos([]any{run}, 1, []string{
			`scripts/renovate-resultado.sh" --propuestas`, "$RENOVATE_REPORT",
			"coverageNote: $cobertura", `echo "$DEPENDENCIAS_COBERTURA"`,
		}, nil)
	}
	if !extrae {
		t.Errorf("%s: el informe no extrae las propuestas sanitizadas con su nota de cobertura", mal)
	}
	conservados := renovateResultadoCampo(job, "with", "Conservar propuestas sanitizadas")
	rutas := make([]any, 0, len(conservados))
	for _, con := range conservados {
		rutas = append(rutas, mapa(con)["path"])
	}
	if !reflect.DeepEqual(rutas, []any{"${{ env.RENOVATE_REPORT }}"}) {
		t.Errorf("%s: el artefacto conserva %v", mal, rutas)
	}
	runs := renovateResultadoCampo(job, "run", "Conservar propuestas en el log", "Informe si Renovate falla")
	if !renovateResultadoTodos(runs, 2, []string{"$RENOVATE_REPORT"}, []string{"/tmp/renovate.json"}) {
		t.Errorf("%s: el log o la incidencia no usan solo el informe sanitizado", mal)
	}
}

// getBranchStatus lee los checks y minimumReleaseAge publica un commit status.
// El run 41526463059 confirmó 403 en POST /statuses/{sha} con permiso de lectura.
// Se mira el mapa efectivo del job: uno propio reemplazaría al del workflow.
// Sin lectura de checks Renovate también puede ignorar los resultados de Actions.
func TestRenovateResultadoPermisosEfectivosDeRenovate(t *testing.T) {
	job, f := renovateResultadoJob(t)
	permisos := job["permissions"]
	if permisos == nil || permisos == false {
		permisos = f["permissions"]
	}
	p := mapa(permisos)
	if p["statuses"] != "write" || p["checks"] != "read" || p["packages"] != "read" {
		t.Fatalf("Renovate necesita statuses: write y checks/packages: read en los permisos efectivos: %v", permisos)
	}
}

func TestRenovateResultadoRenovateAutenticaGHCRConElTokenDelJob(t *testing.T) {
	job, _ := renovateResultadoJob(t)
	for _, env := range renovateResultadoCampo(job, "env", "Ejecutar Renovate") {
		reglas, _ := mapa(env)["RENOVATE_HOST_RULES"].(string)
		if strings.Contains(reglas, `"matchHost":"ghcr.io"`) && strings.Contains(reglas, `"hostType":"docker"`) &&
			strings.Contains(reglas, "${{ github.token }}") {
			return
		}
	}
	t.Fatal("Renovate necesita hostRules de Docker para GHCR con el token del job")
}

// Los charts declaran el registro que publica sus imágenes. En producción,
// global.imageRegistry lo sustituye por el registro privado de Scaleway. El
// manager helm-values de Renovate lee repository sin combinar ese campo registry.
func TestRenovateResultadoLosChartsDeclaranGHCRParaLasImagenesInternas(t *testing.T) {
	rutas, err := filepath.Glob(filepath.Join(guiontest.Raiz(t), "deploy", "helm", "charts", "*", "values.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(rutas) == 0 {
		t.Fatal("no hay ningún values.yaml en deploy/helm/charts")
	}
	for _, ruta := range rutas {
		dec := yaml.NewDecoder(strings.NewReader(guiontest.Leer(t, ruta)))
		for {
			var doc any
			if err := dec.Decode(&doc); errors.Is(err, io.EOF) {
				break
			} else if err != nil {
				t.Fatalf("%s: %v", ruta, err)
			}
			imagen := mapa(mapa(doc)["image"])
			if repo, _ := imagen["repository"].(string); strings.HasPrefix(repo, "sytue-") && imagen["registry"] != "ghcr.io/thinkddr" {
				t.Errorf("%s: la imagen %s queda sin registro GHCR explícito (registry = %v)", ruta, repo, imagen["registry"])
			}
		}
	}
}

// renovateResultadoRegla es lo que se lee de cada packageRule de renovate.json.
type renovateResultadoRegla struct {
	Enabled           *bool
	MatchDatasources  []string
	MatchManagers     []string
	MatchFileNames    []string
	MatchPackageNames []string
	RangeStrategy     string
}

type renovateResultadoConfig struct {
	RangeStrategy  string
	PackageRules   []renovateResultadoRegla
	CustomManagers []struct {
		DatasourceTemplate  string
		ManagerFilePatterns []string
		MatchStrings        []string
	}
}

// renovateResultadoCoincide: un patrón `/…/` es una expresión regular; el resto,
// el nombre exacto.
func renovateResultadoCoincide(t *testing.T, patron, nombre string) bool {
	t.Helper()
	if len(patron) < 2 || !strings.HasPrefix(patron, "/") || !strings.HasSuffix(patron, "/") {
		return patron == nombre
	}
	re, err := regexp.Compile(patron[1 : len(patron)-1])
	if err != nil {
		t.Fatalf("renovate.json: el patrón %s no se puede evaluar aquí: %v", patron, err)
	}
	return re.MatchString(nombre)
}

// excluida dice si alguna regla desactivada deja ese paquete fuera del lookup
// de ese datasource, para ese manager y ese fichero.
func (c renovateResultadoConfig) excluida(t *testing.T, datasource, nombre, manager, fichero string) bool {
	t.Helper()
	return slices.ContainsFunc(c.PackageRules, func(r renovateResultadoRegla) bool {
		return r.Enabled != nil && !*r.Enabled && slices.Contains(r.MatchDatasources, datasource) &&
			(r.MatchManagers == nil || slices.Contains(r.MatchManagers, manager)) &&
			(r.MatchFileNames == nil || slices.Contains(r.MatchFileNames, fichero)) &&
			slices.ContainsFunc(r.MatchPackageNames, func(p string) bool { return renovateResultadoCoincide(t, p, nombre) })
	})
}

// renovateResultadoCatalogo lee upstream-images.txt: alias → upstream.
// Una reconstruida (deploy/upstream-reconstruidas) se sirve desde
// ghcr.io/thinkddr, pero su upstream sigue siendo el de `reconstruida-de=`.
func renovateResultadoCatalogo(t *testing.T) map[string]string {
	t.Helper()
	sinEtiqueta := regexp.MustCompile(`:[^:/]+$`)
	catalogo := map[string]string{}
	for _, linea := range strings.Split(guiontest.Leer(t, "deploy/production/upstream-images.txt"), "\n") {
		campos := strings.Fields(linea)
		if len(campos) == 0 || strings.HasPrefix(campos[0], "#") {
			continue
		}
		alias, upstream := campos[0], ""
		if len(campos) > 2 {
			upstream = campos[2]
		}
		if len(campos) > 4 {
			for _, anotacion := range campos[4:] {
				if origen, ok := strings.CutPrefix(anotacion, "reconstruida-de="); ok {
					upstream = sinEtiqueta.ReplaceAllString(origen, "")
					break
				}
			}
		}
		catalogo[alias] = upstream
	}
	return catalogo
}

// La exclusión no puede tapar imágenes publicadas ni paquetes de otros datasources.
func TestRenovateResultadoSoloLasImagenesLocalesQuedanFueraDelLookupDocker(t *testing.T) {
	var config renovateResultadoConfig
	if err := json.Unmarshal([]byte(guiontest.Leer(t, "renovate.json")), &config); err != nil {
		t.Fatalf("renovate.json: %v", err)
	}
	// Por defecto, como en el original: manager dockerfile y sin fichero.
	excluida := func(datasource, nombre string) bool {
		return config.excluida(t, datasource, nombre, "dockerfile", "")
	}
	for _, nombre := range []string{"sytue-rooms", "sytue-syerp", "sytue-syphotos", "sytue-cast-transcoder"} {
		if !excluida("docker", nombre) {
			t.Errorf("falta excluir imagen local %s", nombre)
		}
		if excluida("npm", nombre) {
			t.Errorf("la regla local no debe afectar npm: %s", nombre)
		}
	}
	for _, nombre := range []string{
		"node", "postgres", "docker.io/library/node",
		"ghcr.io/thinkddr/sytue-rooms", "sytue-prod-platform/traefik",
		"registry.example.invalid/sytue-rooms", "upstream-sytue-rooms",
	} {
		if excluida("docker", nombre) {
			t.Errorf("la auditoría debe conservar %s", nombre)
		}
	}

	catalogo := renovateResultadoCatalogo(t)
	alimenta := false
	for _, m := range config.CustomManagers {
		alimenta = alimenta || (m.DatasourceTemplate == "docker" &&
			slices.Contains(m.ManagerFilePatterns, `/^deploy/production/upstream-images\.txt$/`) &&
			slices.ContainsFunc(m.MatchStrings, func(p string) bool { return strings.Contains(p, "(?<depName>") }))
	}
	if !alimenta {
		t.Error("el catálogo cualificado debe seguir alimentando el datasource Docker")
	}
	for _, espejo := range [][2]string{
		{"cert-manager-webhook-scaleway", "docker.io/scaleway/cert-manager-webhook-scaleway"},
		{"livekit-egress", "docker.io/livekit/egress"},
		{"prometheus-adapter", "registry.k8s.io/prometheus-adapter/prometheus-adapter"},
	} {
		alias, upstream := espejo[0], espejo[1]
		fichero := "deploy/helm/charts/" + alias + "/values.yaml"
		if catalogo[alias] != upstream {
			t.Errorf("falta el upstream cualificado de %s en el catálogo auditado (hay %q)", alias, catalogo[alias])
		}
		if !config.excluida(t, "docker", alias, "helm-values", fichero) {
			t.Errorf("sigue el lookup del alias Helm %s", alias)
		}
		if excluida("docker", alias) {
			t.Errorf("no se debe excluir globalmente %s", alias)
		}
		if config.excluida(t, "docker", alias, "helm-values", "otro/values.yaml") {
			t.Errorf("exclusión Helm demasiado amplia: %s", alias)
		}
		if config.excluida(t, "docker", upstream, "regex", "deploy/production/upstream-images.txt") {
			t.Errorf("upstream oculto: %s", upstream)
		}
	}
	// La última regla de npm que nombra a astro es la que decide.
	astro := ""
	for _, r := range config.PackageRules {
		if slices.Contains(r.MatchDatasources, "npm") && slices.Contains(r.MatchPackageNames, "astro") {
			astro = r.RangeStrategy
		}
	}
	if config.RangeStrategy != "pin" {
		t.Errorf("la estrategia general sigue intacta: rangeStrategy = %q", config.RangeStrategy)
	}
	if astro != "bump" {
		t.Errorf("Astro conserva el intervalo exigido por check-toolchain: rangeStrategy = %q", astro)
	}
}
