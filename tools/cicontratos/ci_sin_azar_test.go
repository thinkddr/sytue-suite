// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package cicontratos

import (
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/tools/guiontest"
)

// TRES CLASES DE ROJO «ALEATORIO» DEL CI QUE NO LO ERAN (diagnóstico del
// 2026-09-23, docs/operations.md#ci-sin-azar). Cada una salía en rojo unas veces
// y en verde otras con el mismo commit, y cada comprobación de aquí cierra la
// clase entera, no el caso que la destapó.
//
//  1. Un job que compila un módulo con forks PRIVADOS sin `forks.sh`: pasa en el
//     runner que ya tiene el módulo en su caché y cae en el que no.
//  2. `| grep -q` o `| grep -m` con pipefail: grep sale al primer acierto, el que
//     escribe recibe SIGPIPE y la tubería da 141 aunque haya coincidencia.
//  3. Una poda de Docker dentro de un workflow: los runners comparten dockerd y
//     la poda se lleva las capas o el lease del build de un vecino. Y desde el
//     28/09 (ADR 0066), el builder clásico: sus capas colgantes no tienen lease y
//     la poda del host podía quitárselas a un build en marcha. Se construye con el
//     BuildKit del CI, y su caché no se poda desde un job.
//  4. Dos entornos bajo una etiqueta, o uno que cambia solo: `runs-on: docker`
//     lo atendía también el pool de git.gay, y una imagen sin digest cambia el
//     día que alguien la vuelve a bajar. En GitHub (desde el 27/09) cada job
//     declara además `container:` con la misma imagen por digest; la etiqueta
//     viva la vigila deploy/production/tests/release_workflow_test.go contra .github/RUNNERS.md.

// azarLineas son las líneas de un fichero del repositorio, como las ve awk: sin
// un último registro vacío por el salto final.
func azarLineas(t *testing.T, ruta string) []string {
	t.Helper()
	contenido := guiontest.Leer(t, ruta)
	if contenido == "" {
		return nil
	}
	return strings.Split(strings.TrimSuffix(contenido, "\n"), "\n")
}

// azarSeguidos devuelve las rutas que `git ls-files -- <patrones>` da en la raíz.
func azarSeguidos(t *testing.T, patrones ...string) []string {
	t.Helper()
	r := guiontest.Ejecutar(t, guiontest.Opciones{}, "git", append([]string{"ls-files", "--"}, patrones...)...)
	if r.Codigo != 0 {
		t.Fatalf("git ls-files: código %d\n%s", r.Codigo, r.Error)
	}
	if r.Texto() == "" {
		return nil
	}
	return strings.Split(r.Texto(), "\n")
}

// azarFlujos son los workflows, con su ruta desde la raíz (`.github/workflows/*.yml`).
func azarFlujos(t *testing.T) []string {
	t.Helper()
	raiz := guiontest.Raiz(t)
	rutas, err := filepath.Glob(filepath.Join(raiz, ".github", "workflows", "*.yml"))
	if err != nil || len(rutas) == 0 {
		t.Fatalf("no hay workflows que comprobar (%v)", err)
	}
	for i, ruta := range rutas {
		rel, err := filepath.Rel(raiz, ruta)
		if err != nil {
			t.Fatal(err)
		}
		rutas[i] = filepath.ToSlash(rel)
	}
	return rutas
}

var (
	azarComentario   = regexp.MustCompile(`^[ \t]*#`)
	azarBarraFinal   = regexp.MustCompile(`\\$`)
	azarTuberiaFinal = regexp.MustCompile(`[^|]\|[ \t]*$`)
	azarBloqueYAML   = regexp.MustCompile(`(:|^[ \t]*-)[ \t]*[|>][-+]?[ \t]*$`)
)

// azarLogicas da las líneas LÓGICAS, sin comentarios y con las continuaciones
// unidas: una barra al final, o una tubería que sigue en la línea de abajo (salvo
// el `run: |` de YAML). Cada una como «fichero:línea: texto».
func azarLogicas(t *testing.T, rutas ...string) []string {
	t.Helper()
	var out []string
	for _, ruta := range rutas {
		buf, ini := "", 0
		for i, l := range azarLineas(t, ruta) {
			if azarComentario.MatchString(l) {
				continue
			}
			if buf == "" {
				ini = i + 1
			}
			if azarBarraFinal.MatchString(l) {
				buf += l[:len(l)-1] + " "
				continue
			}
			if azarTuberiaFinal.MatchString(l) && !azarBloqueYAML.MatchString(l) {
				buf += l + " "
				continue
			}
			out = append(out, ruta+":"+strconv.Itoa(ini)+": "+buf+l)
			buf = ""
		}
	}
	return out
}

// azarCasan son las líneas que casan con el patrón (`grep -E`).
func azarCasan(lineas []string, patron string) []string {
	re := regexp.MustCompile(patron)
	var out []string
	for _, l := range lineas {
		if re.MatchString(l) {
			out = append(out, l)
		}
	}
	return out
}

// azarSin son las líneas que NO casan con el patrón (`grep -vE`).
func azarSin(lineas []string, patron string) []string {
	re := regexp.MustCompile(patron)
	var out []string
	for _, l := range lineas {
		if !re.MatchString(l) {
			out = append(out, l)
		}
	}
	return out
}

// azarMal deja el fallo escrito con las líneas que lo provocan debajo.
func azarMal(t *testing.T, mensaje string, lineas []string) {
	t.Helper()
	t.Errorf("%s\n    %s", mensaje, strings.Join(lineas, "\n    "))
}

// azarJob es un job de un workflow: su fichero, su nombre y su texto sin comentarios.
type azarJob struct{ fichero, nombre, texto string }

var (
	azarJobCabecera = regexp.MustCompile(`^ {2}[A-Za-z0-9_-]+:[ \t]*$`)
	azarAnclaDefine = regexp.MustCompile(`^ *- &[A-Za-z0-9_-]+[ \t]*$`)
	azarAnclaUsa    = regexp.MustCompile(`^ *- \*[A-Za-z0-9_-]+[ \t]*$`)
	azarPaso        = regexp.MustCompile(`^ *- `)
)

// azarJobsDe parte un workflow en sus jobs. La cabecera del workflow (su `env:`
// global) se suma a cada job, que es donde vive un GOPRIVATE común. Un paso
// reutilizado por ancla (`- &nombre` … `- *nombre`, release-desktop) cuenta con
// su texto en el job que lo reutiliza: sin esto, el job `tiendas` parecía no
// llamar a forks.sh, que va dentro de `*apple-clonar`.
func azarJobsDe(t *testing.T, ruta string) []azarJob {
	t.Helper()
	var (
		jobs                  []azarJob
		enJobs                bool
		cab, job, txt, anclaN string
	)
	anclas := map[string]string{}
	// El texto llegaba al bucle por `IFS=$'\t' read`, que se come los tabuladores finales.
	cerrar := func() {
		if job != "" {
			jobs = append(jobs, azarJob{ruta, job, strings.TrimRight(cab+" "+txt, "\t")})
		}
	}
	for _, l := range azarLineas(t, ruta) {
		switch {
		case azarComentario.MatchString(l):
			continue
		case strings.HasPrefix(l, "jobs:"):
			enJobs = true
			continue
		case !enJobs:
			cab += " " + l
			continue
		case azarJobCabecera.MatchString(l):
			cerrar()
			job = strings.TrimSuffix(strings.Fields(l)[0], ":")
			txt, anclaN = "", ""
			continue
		case azarAnclaDefine.MatchString(l):
			anclaN = strings.TrimPrefix(strings.Fields(l)[1], "&")
			continue
		case azarAnclaUsa.MatchString(l):
			txt += " " + anclas[strings.TrimPrefix(strings.Fields(l)[1], "*")]
			anclaN = ""
			continue
		case azarPaso.MatchString(l):
			anclaN = ""
		}
		txt += " " + l
		if anclaN != "" {
			anclas[anclaN] += " " + l
		}
	}
	cerrar()
	return jobs
}

// azarTodosLosJobs son los jobs de todos los workflows.
func azarTodosLosJobs(t *testing.T) []azarJob {
	t.Helper()
	flujos := azarFlujos(t)
	jobs := make([]azarJob, 0, len(flujos))
	for _, wf := range flujos {
		jobs = append(jobs, azarJobsDe(t, wf)...)
	}
	return jobs
}

// azarBloque son las líneas que siguen a la primera que cumple `empieza`, hasta
// la primera que cumple `acaba` (sin incluir ninguna de las dos).
func azarBloque(lineas []string, empieza, acaba func(string) bool) []string {
	var out []string
	dentro := false
	for _, l := range lineas {
		switch {
		case empieza(l):
			dentro = true
		case dentro && acaba(l):
			return out
		case dentro:
			out = append(out, l)
		}
	}
	return out
}

// azarModulosConForks son los módulos cuyo go.mod reemplaza por
// github.com/thinkddr/: dependen de un fork privado.
func azarModulosConForks(t *testing.T) []string {
	t.Helper()
	reemplazo := regexp.MustCompile(`^replace .*=> *github\.com/thinkddr/`)
	comentario := regexp.MustCompile(`^[ \t]*//`)
	var modulos []string
	for _, gomod := range azarSeguidos(t, "*go.mod") {
		conFork := false
		for _, l := range azarLineas(t, gomod) {
			if !comentario.MatchString(l) && reemplazo.MatchString(l) {
				conFork = true
			}
		}
		if !conFork {
			continue
		}
		dir := filepath.ToSlash(filepath.Dir(gomod))
		if dir == "." {
			t.Errorf("el módulo raíz depende de un fork privado: todo job de Go necesitaría forks.sh y este gate no sabe verlo")
			continue
		}
		modulos = append(modulos, dir)
	}
	return modulos
}

// ─── 1. Forks privados: forks.sh y GOPRIVATE en todo job que los compile ─────
//
// El 2706 cayó en «clientes nativos» (govulncheck sobre apps/fyne) en el pool de
// git.gay, sin caché, y salió verde al repetirlo en un runner del frontal, que
// tenía el fork en /var/cache/sytue-ci/go-mod. El job no llevaba ni GOPRIVATE ni
// forks.sh: la caché tapaba la configuración que faltaba.
//
// Los módulos afectados salen de sus go.mod —el que reemplaza por
// github.com/thinkddr/ depende de un fork privado—, no de una lista escrita aquí: el día que apps/nucleo u otro
// módulo pasen a usar uno, entran solos.
// ponytail: una mención de `apps/x/...` cuenta como apps/x aunque sea otro
// módulo anidado; hoy ninguno de los afectados tiene módulos dentro.
func TestCISinAzarForksPrivadosConForksShYGOPRIVATE(t *testing.T) {
	compila := regexp.MustCompile(`accion\.sh( [a-z0-9-]+)* go( |$)|go (build|test|vet|install|list|run|mod|generate) |fyne (package|build|release) |empaquetar\.sh (linux|windows|macos|android|ios)`)
	jobs := azarTodosLosJobs(t)
	for _, modulo := range azarModulosConForks(t) {
		patron := regexp.MustCompile(`(^|[^A-Za-z0-9_./-])` + strings.ReplaceAll(modulo, ".", `\.`) + `($|[^A-Za-z0-9_-])`)
		for _, j := range jobs {
			// Solo si COMPILA: firmar los AAB (release-mobile) lee `empaquetar.sh publicables`
			// y no toca Go, así que ni necesita los forks ni debe depender de su token.
			if !patron.MatchString(j.texto) || !compila.MatchString(j.texto) {
				continue
			}
			if !strings.Contains(j.texto, "forks.sh") {
				t.Errorf("%s, job %s: usa %s (forks privados) y no llama a .github/actions/toolchain/forks.sh", j.fichero, j.nombre, modulo)
			}
			if !strings.Contains(j.texto, "GOPRIVATE") {
				t.Errorf("%s, job %s: usa %s (forks privados) sin GOPRIVATE: Go pediría el fork al proxy público", j.fichero, j.nombre, modulo)
			}
		}
	}
	// Con `||` entre secretos, estos runners mandaban el segundo aunque existiera el
	// primero: 403 en los runs 2642 y 2650, y 200 con la referencia directa en el 2652.
	if o := azarCasan(azarLogicas(t, azarFlujos(t)...),
		`\$\{\{[^}]*(\|\|[^}]*SYTUE_GIT_FORKS_TOKEN|SYTUE_GIT_FORKS_TOKEN[^}]*\|\|)`); len(o) > 0 {
		azarMal(t, "SYTUE_GIT_FORKS_TOKEN con «||»: va con referencia directa, ${{ secrets.SYTUE_GIT_FORKS_TOKEN }}", o)
	}
}

// azarConPipefail son los ficheros que EJECUTA EL CI: los workflows (su shell
// por defecto es `bash -e -o pipefail`, medido en los logs), la plomería de
// .github, scripts/, los guiones de apps/ y todas las carpetas de tests. Los
// guiones que solo corren a mano en un servidor quedan fuera: no dan rojos en el CI.
func azarConPipefail(t *testing.T) []string {
	t.Helper()
	var ficheros []string
	for _, f := range azarSeguidos(t, ".github/*", "scripts/*", "apps/*", "*/tests/*") {
		switch {
		case strings.HasPrefix(f, ".github/") && (strings.HasSuffix(f, ".yml") || strings.HasSuffix(f, ".yaml")):
			ficheros = append(ficheros, f)
		case strings.HasSuffix(f, ".sh"):
			for _, l := range azarLineas(t, f) {
				if !azarComentario.MatchString(l) && strings.Contains(l, "pipefail") {
					ficheros = append(ficheros, f)
					break
				}
			}
		}
	}
	return ficheros
}

// ─── 2. Nada de `| grep -q` ni `| grep -m` con pipefail ───────────────────────
//
// quality v3.6.2 dio por NO declarado un token de theme.css que sí lo estaba
// (issue 861) y salió verde al repetirlo: `printf … | grep -qx` en lint-design.sh.
// grep -q (y -m) deja de leer al primer acierto; si el que escribe aún no había
// terminado, recibe SIGPIPE, sale con 141 y pipefail convierte el acierto en
// fallo. Depende de cómo reparta la CPU el planificador: con el frontal cargado,
// más. `grep … >/dev/null` NO lo arregla: GNU grep detecta /dev/null y también
// para al primer acierto. Lo que sí: una here-string (`grep -q x <<<"$var"`) o
// un `case`, que no tienen tubería.
//
// `| head` también corta, pero en general no se vigila a propósito: casi siempre va
// en una asignación que solo quiere la primera línea, y prohibirlo era ruido. La
// excepción es cuando quien escribe es `printf` o `echo`: bash los saca línea a
// línea, así que con una variable de varias líneas head cierra antes de que acaben.
// `base="$(printf … "$runtime" | head -1 | awk …)"` tumbó `security` el 2026-10-06
// (37419208658): la asignación falló y `set -e` salió sin una sola línea de error.
func TestCISinAzarNingunaTuberiaHaciaGrepQNiHaciaHead(t *testing.T) {
	ficheros := azarConPipefail(t)
	if len(ficheros) == 0 {
		t.Fatal("no hay guiones ni workflows con pipefail que comprobar")
	}
	lineas := azarLogicas(t, ficheros...)
	if o := azarCasan(lineas,
		`(^|[^|])\|[[:space:]]*[ef]?grep[[:space:]]+([^|;&)]*[[:space:]])?(-[A-Za-z]*[qm]|--quiet|--silent|--max-count)`); len(o) > 0 {
		azarMal(t, `tubería hacia grep -q/-m con pipefail (SIGPIPE: el acierto sale como fallo). Usa grep -q … <<<"$var" o un case:`, o)
	}
	if o := azarCasan(lineas,
		`(^|[^[:alnum:]_])(printf|echo)[[:space:]][^|]*\|[[:space:]]*head([[:space:]]|$)`); len(o) > 0 {
		azarMal(t, `printf/echo hacia head con pipefail (SIGPIPE: sale sin mensaje). Usa read … <<<"$var" o awk … <<<"$var":`, o)
	}
}

// azarLineasDelCI son las líneas lógicas de los workflows, la plomería de
// .github y scripts/: lo que corre contra el dockerd compartido.
func azarLineasDelCI(t *testing.T) []string {
	t.Helper()
	ci := azarSeguidos(t, ".github/*.yml", ".github/*.sh", "scripts/*.sh")
	if len(ci) == 0 {
		t.Fatal("no hay workflows ni guiones de CI que comprobar")
	}
	return azarLogicas(t, ci...)
}

// ─── 3. Ninguna poda de Docker desde un workflow ──────────────────────────────
//
// Los runners construyen contra el MISMO dockerd. Cualquier poda de imágenes lanza
// una recolección de containerd que se lleva el lease de un build vecino (ejecución
// 2503: la poda de un nocturno tumbó a los otros dos a los 3 y 7 s), y un
// `image rm` sin --no-prune arrastra etapas intermedias que otro build está usando
// (turno 2 de v3.3.8). Cada job borra SUS etiquetas con --no-prune; lo demás lo poda
// el temporizador del host, que no poda con un job vivo.
func TestCISinAzarNingunaPodaDeDocker(t *testing.T) {
	lineas := azarLineasDelCI(t)
	o := azarCasan(lineas, `docker[[:space:]]+(image|builder|buildx|system|container|volume)[[:space:]]+prune`)
	o = append(o, azarSin(azarCasan(lineas, `docker[[:space:]]+(rmi|image[[:space:]]+rm)([[:space:]]|$)`), `--no-prune`)...)
	if len(o) > 0 {
		azarMal(t, "poda de Docker o image rm sin --no-prune en el CI: el daemon es compartido y rompe el build de un vecino", o)
	}
}

// 3b. EL BUILDER CLÁSICO NO VUELVE, Y LA CACHÉ COMPARTIDA NO SE PODA DESDE UN JOB
// (ADR 0066). Se construye con el BuildKit del CI (`scripts/construir-imagen.sh`):
// lo que un build tiene en uso lo protegen sus leases, y la poda del host
// (deploy/rooms-sfu/podar-ci.sh) ya no puede quitarle nada. Un `docker build` o un
// `DOCKER_BUILDKIT=0` en el CI devolvería el riesgo, y además choca con el proxy de
// los jobs, que ya no abre /build. Y un `buildctl prune` sin su propio `--addr` va
// contra el BuildKit COMPARTIDO (BUILDKIT_HOST): no rompe a un vecino, pero le tira
// la caché a media publicación. Un demonio propio del job (`--addr` a su socket) sí
// puede podarse.
// (El guion de shell del que sale esta prueba se saltaba a sí mismo, porque sus
// patrones casaban consigo mismos. Aquí no hace falta: esto es Go y no entra en
// la lista.)
func TestCISinAzarNiBuilderClasicoNiPodaDelBuildKitCompartido(t *testing.T) {
	lineas := azarLineasDelCI(t)
	o := azarCasan(lineas, `(^|[^A-Za-z0-9_-])docker[[:space:]]+(buildx[[:space:]]+)?build[[:space:]]|DOCKER_BUILDKIT`)
	o = append(o, azarSin(
		azarCasan(lineas, `(^|[^A-Za-z0-9_-])buildctl[[:space:]]([^|;&]*[[:space:]])?prune([[:space:]]|$)`),
		`--addr[[:space:]=]+"?unix://\$BUILDKIT_ROOT/`)...)
	if len(o) > 0 {
		azarMal(t, "builder clásico o poda del BuildKit compartido en el CI: se construye con scripts/construir-imagen.sh y la caché la poda el host", o)
	}
}

// ─── 4. Un solo entorno: `sytue-arm64` e imágenes fijadas por digest ─────────
//
// `docker` la atendían nuestros tres runners (ARM64, node:lts, caché de Go) Y el
// pool compartido de git.gay (AMD64, otra imagen, sin caché), y Forgejo da cada
// job al primero que pregunta: el 16 % de los rojos de la semana del 15/09 salió
// de ahí. `sytue-arm64` solo la declaran los tres del frontal.
//
// Y una imagen por etiqueta cambia el entorno ENTERO un día cualquiera: `node:lts`
// era Node 24 sobre Debian 12 al bajarla el 19/09, y Node 26 pasa a LTS en
// octubre. Así que toda imagen de un job —la del runner, `container:` y
// `services:`— va con `@sha256:`.
func TestCISinAzarUnSoloEntornoEImagenesPorDigest(t *testing.T) {
	lineas := azarLogicas(t, azarFlujos(t)...)
	if o := azarCasan(lineas, `runs-on:[[:space:]]*\[?[[:space:]]*["']?docker["']?([[:space:],]|\]|$)`); len(o) > 0 {
		azarMal(t, "runs-on: docker también lo atiende el pool de git.gay (otra arquitectura, sin caché): va en sytue-arm64", o)
	}
	var imagenes []string
	for _, l := range azarCasan(lineas, `(^|[[:space:]])image:[[:space:]]*[^[:space:]]|docker://`) {
		if !strings.Contains(l, "${{") {
			imagenes = append(imagenes, l)
		}
	}
	const servicio = "deploy/rooms-sfu/runners/sytue-forgejo-runner.service"
	etiqueta := regexp.MustCompile(`--label [A-Za-z0-9_-]+:docker://\S+`)
	for _, l := range azarLineas(t, servicio) {
		for _, e := range etiqueta.FindAllString(l, -1) {
			imagenes = append(imagenes, servicio+": "+e)
		}
	}
	if o := azarSin(imagenes, `@sha256:[0-9a-f]{64}`); len(o) > 0 {
		azarMal(t, "imagen sin digest: una etiqueta que se mueve cambia el entorno de los jobs sin que cambie el repositorio", o)
	}
}

// 5. EL WORKSPACE DE UN RUNNER PROPIO SOBREVIVE ENTRE JOBS (desde el 2026-09-27,
// sytue-arm64 en GitHub). Un paso que clona a mano con `git init .` se encuentra el
// `.git` del job anterior y muere con «remote origin already exists» (codemagic-log,
// 28/09). Cada `git init -q .` va justo después de vaciar el workspace.
func TestCISinAzarTodoClonAManoVaciaAntesElWorkspace(t *testing.T) {
	clona := regexp.MustCompile(`^\s*git init -q \.$`)
	vacia := regexp.MustCompile(`find \. -mindepth 1 -delete$`)
	var o []string
	for _, wf := range azarFlujos(t) {
		lineas := azarLineas(t, wf)
		for i, l := range lineas {
			if clona.MatchString(l) && (i == 0 || !vacia.MatchString(lineas[i-1])) {
				o = append(o, wf+":"+strconv.Itoa(i+1)+":"+l)
			}
		}
	}
	if len(o) > 0 {
		azarMal(t, "git init sobre un workspace sin vaciar: en los runners propios lo deja el job anterior", o)
	}
}

// 6. Y POR LO MISMO, NADA DEL WORKSPACE SE EJECUTA SIN HABERLO CLONADO EN ESE JOB
// (auditoría 2026-10-06, CI-08). `dependencias` no clonaba —Renovate clona él lo que
// analiza— y aun así ejecutaba `$GITHUB_WORKSPACE/.github/actions/informe/accion.sh`
// con un token que escribe en el repositorio: el fichero que hubiera dejado el job
// anterior de ese runner, de la rama o del tag que fuera. No fallaba nunca, que es lo
// malo: siempre había algo allí.
func TestCISinAzarNingunJobEjecutaElWorkspaceSinClonarlo(t *testing.T) {
	for _, j := range azarTodosLosJobs(t) {
		hastaElUso, _, usa := strings.Cut(j.texto, "$GITHUB_WORKSPACE/")
		if !usa {
			continue
		}
		if !strings.Contains(hastaElUso, "actions/checkout@") && !strings.Contains(hastaElUso, "git init -q .") {
			t.Errorf("%s, job %s: ejecuta algo de $GITHUB_WORKSPACE sin clonar antes: es lo que dejó el job anterior del runner", j.fichero, j.nombre)
		}
	}
}

// 7. NINGÚN FLUJO BAJA Y EJECUTA UN PAQUETE AL VUELO (CI-02). `npx --yes renovate@X`
// fijaba la versión de Renovate y nada más: sus ~560 dependencias se resolvían contra
// npm en cada ejecución, sin lockfile y corriendo sus scripts de instalación, en un
// paso con token de escritura. Lo que se ejecuta sale de un lockfile del repositorio
// (o, si es de Go, de `go install pkg@versión`, que verifica la base de sumas).
// `bunx --no-install` y `npx --no-install` valen: solo ejecutan lo que ya instaló el
// lockfile y fallan si no está, sin ir al registro.
func TestCISinAzarNingunFlujoEjecutaPaquetesResueltosAlVuelo(t *testing.T) {
	o := azarSin(
		azarCasan(azarLogicas(t, azarFlujos(t)...),
			`(^|[^A-Za-z0-9_./-])(npx|bunx|bun x|npm exec|pnpm dlx|yarn dlx|pipx run|uvx)([[:space:]]|$)`),
		`(^|[^A-Za-z0-9_./-])(npx|bunx)[[:space:]]+--no-install([[:space:]]|$)`)
	if len(o) > 0 {
		azarMal(t, "un flujo baja y ejecuta un paquete sin lockfile: lo que corre depende de lo publicado ese día", o)
	}
}

// 8. LA CACHÉ DE GO DEL HOST NO ENTRA EN LOS FLUJOS QUE PUBLICAN O FIRMAN (CI-04).
// `/var/cache/sytue-ci/go-build` y `go-mod` son directorios del host compartidos
// entre todos los jobs del runner: un PR o rama que los envenene no puede colarse
// en las imágenes de producción, los instaladores firmados ni los SBOM adjuntos a
// una release. En los flujos de verificación (`ci-*.yml`) siguen montados.
func TestCISinAzarLaCacheDeGoDelHostNoEntraEnLoQuePublicaOFirma(t *testing.T) {
	flujosPublicacion := []string{
		".github/workflows/imagenes-produccion.yml",
		".github/workflows/release-desktop.yml",
		".github/workflows/release-mobile.yml",
		".github/workflows/appstore-publicar.yml",
		".github/workflows/play-publicar.yml",
		".github/workflows/release-marketplace.yml",
		".github/workflows/firma-windows-prueba.yml",
	}
	var o []string
	for _, l := range azarLogicas(t, flujosPublicacion...) {
		if strings.Contains(l, "/var/cache/sytue-ci/go-") {
			o = append(o, l)
		}
	}
	if len(o) > 0 {
		azarMal(t, "un flujo de publicación o firma monta la caché de Go compartida del host (CI-04)", o)
	}
	otroJob := regexp.MustCompile(`^ {2}[a-zA-Z0-9_-]+:`)
	seguridad := azarLineas(t, ".github/workflows/security.yml")
	for _, jobSBOM := range []string{"sbom", "sbom-release"} {
		bloque := azarBloque(seguridad,
			func(l string) bool { return l == "  "+jobSBOM+":" }, otroJob.MatchString)
		if len(azarCasan(bloque, `container:[[:space:]]*\*ci|/var/cache/sytue-ci/go-`)) > 0 {
			t.Errorf(".github/workflows/security.yml, job %s: usa *ci o monta /var/cache/sytue-ci/go- (CI-04)", jobSBOM)
		}
	}
}

// 9. CREDENCIALES Y PERMISOS DE MENOR PRIVILEGIO EN LOS FLUJOS (CI-09).
//   - Ningún flujo deja el PAT en `~/.gitconfig` global (`git config --global ... extraHeader`).
//   - Las claves de firma de Android y Play Store van en el `env:` del paso que las usa, no del job.
//   - `security.yml` declara `contents: read` en la raíz y da escritura solo a los jobs que la usan.
//   - `astro-fyne.yml` pasa `persist-credentials: false` en sus checkouts.
func TestCISinAzarCredencialesYPermisosDeMenorPrivilegio(t *testing.T) {
	if o := azarCasan(azarLogicas(t, azarFlujos(t)...),
		`git[[:space:]]+config[[:space:]]+--global[^#]*extraHeader`); len(o) > 0 {
		azarMal(t, "un flujo escribe extraHeader en el gitconfig global del contenedor o del runner (CI-09)", o)
	}
	for _, par := range [][2]string{
		{".github/workflows/release-mobile.yml", "UPLOAD_KEYSTORE_B64"},
		{".github/workflows/release-mobile.yml", "UPLOAD_KEYSTORE_PASSWORD"},
		{".github/workflows/release-mobile.yml", "UPLOAD_KEY_ALIAS"},
		{".github/workflows/play-publicar.yml", "PLAY_JSON"},
	} {
		wf, sec := par[0], par[1]
		if len(azarCasan(azarLineas(t, wf), `^      `+sec+`:`)) > 0 {
			t.Errorf("%s: %s está en el env del job entero en vez del paso que firma o sube (CI-09)", wf, sec)
		}
	}
	permisosRaiz := azarBloque(azarLineas(t, ".github/workflows/security.yml"),
		func(l string) bool { return strings.HasPrefix(l, "permissions:") },
		func(l string) bool { return l != "" && !strings.HasPrefix(l, " ") })
	if len(azarCasan(permisosRaiz, `(issues|actions):[[:space:]]*write`)) > 0 {
		t.Errorf(".github/workflows/security.yml concede issues/actions: write a todos los jobs desde la raíz (CI-09)")
	}
	astro := azarLineas(t, ".github/workflows/astro-fyne.yml")
	checkouts := len(azarCasan(astro, `uses: actions/checkout@`))
	sinCredenciales := len(azarCasan(astro, `persist-credentials: false`))
	if checkouts == 0 || checkouts != sinCredenciales {
		t.Errorf(".github/workflows/astro-fyne.yml tiene %d checkout(s) y %d con persist-credentials: false (CI-09)", checkouts, sinCredenciales)
	}
}

// 10. ENTRADAS DE WORKFLOW_DISPATCH Y ETIQUETAS VALIDADAS + VIGILANTE CERRADO + GITLEAKS EN TESTS (CI-11, CI-13, CI-14, CI-15).
func TestCISinAzarEntradasValidadasVigilanteCerradoYGitleaksEnTests(t *testing.T) {
	const imagenes = ".github/workflows/imagenes-produccion.yml"
	bloqueDominio := azarBloque(azarLineas(t, imagenes),
		func(l string) bool { return strings.HasPrefix(l, "      dominio:") },
		regexp.MustCompile(`^ {6}[a-z_]+:`).MatchString)
	if !strings.Contains(strings.Join(bloqueDominio, "\n"), "type: choice") {
		t.Errorf("%s: el input dominio debe ser type: choice (CI-11)", imagenes)
	}
	if !strings.Contains(guiontest.Leer(t, imagenes), "refs/tags/$TAG") {
		t.Errorf("%s: no comprueba si la etiqueta $TAG ya existe en otro commit (CI-13)", imagenes)
	}
	if !strings.Contains(guiontest.Leer(t, ".github/workflows/ci-integration.yml"), "paquete inválido") {
		t.Errorf(".github/workflows/ci-integration.yml: no valida los paquetes de inputs.pkgs antes de pasarlos a go test (CI-11)")
	}
	if o := azarSin(
		azarCasan(azarLogicas(t, ".github/workflows/release-marketplace.yml"),
			`\$\{\{[[:space:]]*github\.event\.inputs\.ref[[:space:]]*\}\}`),
		`(TAG|ref):[[:space:]]*\$\{\{`); len(o) > 0 {
		azarMal(t, ".github/workflows/release-marketplace.yml: interpola github.event.inputs.ref directo en run: (CI-11)", o)
	}
	const vigilante = "deploy/helm/bin/vigilante-releases.sh"
	guion := guiontest.Leer(t, vigilante)
	if !strings.Contains(guion, `^v[0-9]+\.[0-9]+\.[0-9]+$`) ||
		len(azarCasan(azarLineas(t, vigilante), `sed -i.*RELEASE_TAG=`)) > 0 ||
		strings.Contains(guion, `[ -n "$verde" ] && [ "$verde" != "success" ]`) {
		t.Errorf("%s: debe validar tag semver, fallar cerrado si estado_ci viene vacío y no usar sed con el tag (CI-14)", vigilante)
	}
	if strings.Contains(guiontest.Leer(t, ".gitleaks.toml"), `_test\.go`) {
		t.Errorf(".gitleaks.toml sigue excluyendo *_test.go del escaneo de secretos (CI-15)")
	}
}
