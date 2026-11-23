// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package pruebasfyne

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/tools/guiontest"
)

// --- 12. Google Play: la ficha y el secreto.
// `play-publicar.sh --seco` sobre el árbol real, y en rojo sobre una copia estropeada
// (un título de 31, una sola captura, un CSV sin cabecera): si la validación dejara de
// mirar algo, el caso malo saldría verde. Y el secreto: se sondea contra un curl y un
// openssl de mentira que apuntan su argv, y ni la clave privada ni el token pueden
// aparecer ahí. Visto en rojo quitando el `-H @cabecera` y el `--data @-`.
func TestEmpaquetarPlayFicha(t *testing.T) {
	t.Parallel()
	d := empaquetarDirs(t)
	guion := empaquetarUnir(d.aqui, "play-publicar.sh")
	if info, err := os.Stat(guion); err != nil || info.Mode()&0o111 == 0 {
		t.Error("apps/fyne/scripts/play-publicar.sh no es ejecutable")
	}
	play := func(env []string, args ...string) guiontest.Resultado {
		t.Helper()
		return guiontest.Ejecutar(t, guiontest.Opciones{Env: env}, guion, args...)
	}
	r := play(nil, "--seco")
	salida := empaquetarJunto(r)
	if r.Codigo != 0 {
		l := empaquetarLineas(salida, `error`)
		t.Errorf("play-publicar.sh --seco no pasa en el árbol: %s", strings.Join(l[:min(3, len(l))], "\n"))
	}
	if !strings.Contains(salida, "15 apps válidas") {
		t.Errorf("play-publicar.sh --seco no valida las quince apps: %s", empaquetarUltima(salida))
	}
	if r := play([]string{"PLAY_CONFIRMAR_PRODUCCION=NO"}, "--produccion"); r.Codigo == 0 {
		t.Error("play-publicar.sh deja promover producción sin la confirmación exacta")
	}
	texto := guiontest.Leer(t, guion)
	if !strings.Contains(texto, "tracks/production") || !strings.Contains(texto, "ERROR_IF_IN_REVIEW") {
		t.Error("play-publicar.sh no protege la promoción a producción frente a una revisión existente")
	}
	for _, app := range empaquetarApps(t) {
		if app == "demo" {
			continue
		}
		if info, err := os.Stat(empaquetarUnir(d.fyne, "play", app, "seguridad-datos.csv")); err != nil || info.Size() == 0 {
			t.Errorf("%s no tiene play/%s/seguridad-datos.csv", app, app)
		}
		if capturas, _ := filepath.Glob(empaquetarUnir(d.fyne, "play", app, "graficos", "telefono-*.png")); len(capturas) == 0 {
			t.Errorf("%s no tiene capturas de teléfono", app)
		}
	}
	roto := t.TempDir()
	if r := guiontest.Ejecutar(t, guiontest.Opciones{}, "cp", "-R", empaquetarUnir(d.fyne, "play", "mail"), roto+"/"); r.Codigo != 0 {
		t.Fatal(r.Todo())
	}
	guiontest.Escribir(t, empaquetarUnir(roto, "mail/es-ES/title.txt"), "Sytue Mail con un nombre largo.\n")
	sobran, _ := filepath.Glob(empaquetarUnir(roto, "mail/graficos/telefono-[2-9].png"))
	for _, f := range sobran {
		_ = os.Remove(f)
	}
	guiontest.Escribir(t, empaquetarUnir(roto, "mail/seguridad-datos.csv"), "a,b\n")
	r = play([]string{"PLAY_DIR=" + roto}, "--seco", "mail")
	if r.Codigo == 0 {
		t.Error("play-publicar.sh --seco da por buena una ficha rota")
	}
	for _, que := range []string{"title.txt: 31 caracteres", "1 capturas de teléfono", "cabecera de la exportación"} {
		if !strings.Contains(r.Todo(), que) {
			t.Errorf("play-publicar.sh --seco no ve «%s»", que)
		}
	}
}

// Los workflows escriben la cuenta de servicio en $RUNNER_TEMP y la borran con un trap.
func TestEmpaquetarPlayWorkflows(t *testing.T) {
	d := empaquetarDirs(t)
	for _, wf := range []string{"play-publicar", "release-mobile"} {
		f := guiontest.Leer(t, empaquetarUnir(d.raiz, ".github/workflows", wf+".yml"))
		if !strings.Contains(f, `PLAY_CREDENCIALES="$RUNNER_TEMP/play.json"`) || !strings.Contains(f, `trap 'rm -f "$PLAY_CREDENCIALES"' EXIT`) {
			t.Errorf("%s.yml no escribe la cuenta de servicio en $RUNNER_TEMP con un trap que la borre", wf)
		}
	}
	f := guiontest.Leer(t, empaquetarUnir(d.raiz, ".github/workflows/play-publicar.yml"))
	if !strings.Contains(f, "confirmacion_produccion") || !strings.Contains(f, "PUBLICAR_15_APPS") ||
		!strings.Contains(f, `args=(--produccion --tag "$TAG" --estado "$ESTADO_PRODUCCION")`) {
		t.Error("play-publicar.yml no exige confirmar producción con tag y estado explícitos")
	}
}

// --- La API de Google Play de mentira --------------------------------------------

// empaquetarPlay es el fixture del original: un curl y un openssl que apuntan su argv,
// una cuenta de servicio con una clave de verdad y la API falsa con sus escenarios.
type empaquetarPlay struct {
	t                 *testing.T
	dir, guion, clave string
	aqui, fyne        string
}

func empaquetarPlayFalso(t *testing.T) *empaquetarPlay {
	t.Helper()
	guiontest.Necesita(t, "openssl", "jq")
	d := empaquetarDirs(t)
	f := &empaquetarPlay{t: t, dir: empaquetarUnir(t.TempDir(), "play-falso"), guion: empaquetarUnir(d.aqui, "play-publicar.sh"), aqui: d.aqui, fyne: d.fyne}
	pem := empaquetarUnir(f.dir, "clave.pem")
	if err := os.MkdirAll(empaquetarUnir(f.dir, "bin"), 0o750); err != nil {
		t.Fatal(err)
	}
	if r := guiontest.Ejecutar(t, guiontest.Opciones{}, "openssl", "genpkey", "-algorithm", "RSA", "-pkeyopt", "rsa_keygen_bits:2048", "-out", pem); r.Codigo != 0 {
		t.Fatal(r.Todo())
	}
	clave := guiontest.Leer(t, pem)
	f.clave = strings.Split(clave, "\n")[1]
	cuenta, err := json.Marshal(map[string]string{"client_email": "ci@ejemplo.iam.gserviceaccount.com", "private_key": clave})
	if err != nil {
		t.Fatal(err)
	}
	guiontest.Escribir(t, empaquetarUnir(f.dir, "cuenta.json"), string(cuenta)+"\n")
	openssl, err := exec.LookPath("openssl")
	if err != nil {
		t.Fatal(err)
	}
	empaquetarEjecutable(t, empaquetarUnir(f.dir, "bin/openssl"), "#!/bin/sh\nprintf '%s\\n' \"$*\" >>\""+f.dir+"/argv\"\nexec \""+openssl+"\" \"$@\"\n")
	empaquetarEjecutable(t, empaquetarUnir(f.dir, "bin/curl"), empaquetarCurlDePlay)
	return f
}

func (f *empaquetarPlay) ruta(n string) string { return empaquetarUnir(f.dir, n) }
func (f *empaquetarPlay) leer(n string) string { return empaquetarLeer(f.ruta(n)) }
func (f *empaquetarPlay) peticiones() string   { return f.leer("peticiones") }
func (f *empaquetarPlay) vaciar(n string)      { guiontest.Escribir(f.t, f.ruta(n), "") }

// play ejecuta un play-publicar.sh contra la API falsa y guarda la salida en `fichero`.
func (f *empaquetarPlay) play(guion, fichero string, env []string, args ...string) guiontest.Resultado {
	f.t.Helper()
	env = append([]string{empaquetarConPATH(f.ruta("bin")), "PLAY_CREDENCIALES=" + f.ruta("cuenta.json"), "RUNNER_TEMP=" + f.dir, "PLAY_FALSO_ROOT=" + f.dir}, env...)
	r := guiontest.Ejecutar(f.t, guiontest.Opciones{Env: env}, guion, args...)
	if fichero != "" {
		guiontest.Escribir(f.t, f.ruta(fichero), r.Salida+r.Error)
	}
	return r
}

// interno es `interno_falso`: --interno sobre mail y rooms con un escenario.
func (f *empaquetarPlay) interno(escenario, estado string, extra ...string) guiontest.Resultado {
	f.t.Helper()
	tag, guion := "v3.13.4", f.guion
	if len(extra) > 0 {
		tag = extra[0]
	}
	if len(extra) > 1 {
		guion = extra[1]
	}
	f.vaciar("peticiones")
	for _, sub := range []string{"internos", "pendientes"} {
		viejos, _ := filepath.Glob(empaquetarUnir(f.dir, sub, "*.json"))
		for _, v := range viejos {
			_ = os.Remove(v)
		}
	}
	return f.play(guion, "interno", []string{"PLAY_ESCENARIO=" + escenario}, "--interno", "--tag", tag, "--estado", estado, "mail", "rooms")
}

// reiniciar es `reiniciar_play`.
func (f *empaquetarPlay) reiniciar() {
	_ = os.RemoveAll(f.ruta("tracks"))
	_ = os.RemoveAll(f.ruta("pendientes"))
	f.vaciar("peticiones")
	f.vaciar("autenticaciones")
}

// produccion es `produccion_falsa`: --produccion con la confirmación y un escenario.
func (f *empaquetarPlay) produccion(guion, escenario, estado string, extra ...string) guiontest.Resultado {
	f.t.Helper()
	tag := "v3.12.4"
	if len(extra) > 0 {
		tag = extra[0]
	}
	args := []string{"--produccion", "--tag", tag, "--estado", estado}
	if len(extra) > 1 && extra[1] != "" {
		args = append(args, "--borrador-anterior", extra[1])
	}
	return f.play(guion, "produccion", []string{"PLAY_CONFIRMAR_PRODUCCION=PUBLICAR_15_APPS", "PLAY_ESCENARIO=" + escenario}, args...)
}

// rechazaSinPromover es `rechaza_sin_promover`: falla y no escribe ni confirma nada.
func (f *empaquetarPlay) rechazaSinPromover(guion, escenario string, extra ...string) bool {
	f.t.Helper()
	f.reiniciar()
	if f.produccion(guion, escenario, "draft", extra...).Codigo == 0 {
		return false
	}
	return !f.escribe()
}

// escribe es `grep -Eq '^PUT |^POST .*:commit' peticiones`.
func (f *empaquetarPlay) escribe() bool {
	return empaquetarHay(f.peticiones(), `^PUT |^POST .*:commit`)
}

// modifica es `grep -qE '^(PUT|PATCH)|:commit' peticiones`.
func (f *empaquetarPlay) modifica() bool {
	return empaquetarHay(f.peticiones(), `^(PUT|PATCH)|:commit`)
}

func (f *empaquetarPlay) cuenta(patron string) int { return empaquetarCuenta(f.peticiones(), patron) }

// escribeFuera: alguna escritura toca producción, la ficha, los bundles o los probadores.
func (f *empaquetarPlay) escribeFuera() bool {
	escrituras := strings.Join(empaquetarLineas(f.peticiones(), `^(PUT|PATCH|POST)`), "\n")
	return empaquetarHay(escrituras, `tracks/production|/listings/|/bundles\?uploadType|/testers/.*$`)
}

// antesDelPrimerPUT cuenta las líneas que casan antes del primer PUT, como el awk del original.
func (f *empaquetarPlay) antesDelPrimerPUT(patron string) int {
	re := regexp.MustCompile(patron)
	n := 0
	for l := range strings.SplitSeq(f.peticiones(), "\n") {
		if strings.HasPrefix(l, "PUT ") {
			break
		}
		if re.MatchString(l) {
			n++
		}
	}
	return n
}

// tracks son los tracks de producción confirmados; sin ninguno, la prueba es roja
// (el `for track in tracks/*.json` del original fallaba con el patrón literal).
func (f *empaquetarPlay) tracks() []string {
	f.t.Helper()
	l, _ := filepath.Glob(empaquetarUnir(f.dir, "tracks", "*.json"))
	if len(l) == 0 {
		f.t.Error("no hay ningún track de producción confirmado")
	}
	return l
}

// json lee un fichero JSON de la API falsa; con varios documentos, el último (jq -e).
func (f *empaquetarPlay) json(ruta string) any {
	dec := json.NewDecoder(strings.NewReader(empaquetarLeer(ruta)))
	var ultimo any
	for {
		var v any
		if dec.Decode(&v) != nil {
			return ultimo
		}
		ultimo = v
	}
}

// filas son las líneas `{"app":…}` de la salida de --interno.
func (f *empaquetarPlay) filas() []map[string]any {
	var r []map[string]any
	for l := range strings.SplitSeq(f.leer("interno"), "\n") {
		if strings.HasPrefix(l, `{"app":`) {
			var m map[string]any
			if json.Unmarshal([]byte(l), &m) == nil {
				r = append(r, m)
			}
		}
	}
	return r
}

func empaquetarClavesDe(m map[string]any) []string {
	c := make([]string, 0, len(m))
	for k := range m {
		c = append(c, k)
	}
	slices.Sort(c)
	return c
}

// expone: la clave privada o el token aparecen en alguno de esos ficheros.
func (f *empaquetarPlay) expone(ficheros ...string) bool {
	for _, n := range ficheros {
		texto := f.leer(n)
		if strings.Contains(texto, f.clave) || strings.Contains(texto, "TOKEN-QUE-NO-SE-VE") {
			return true
		}
	}
	return false
}

func TestEmpaquetarPlayAPIFalsa(t *testing.T) {
	t.Parallel()
	f := empaquetarPlayFalso(t)
	r := f.play(f.guion, "salida", nil, "--sondear", "mail")
	if r.Codigo != 0 {
		t.Errorf("play-publicar.sh --sondear contra la API de mentira falla: %s", empaquetarUltima(f.leer("salida")))
	}
	if !strings.Contains(f.leer("salida"), "eu.sytue.mail") {
		t.Error("play-publicar.sh --sondear no sondea eu.sytue.mail")
	}
	// Cada bloque comparte el estado de la API falsa con el siguiente, como en el original.
	t.Run("sondear admite el borrador exacto y la anterior activa; completed selecciona ese borrador; los estados ambiguos siguen bloqueados sin escrituras", func(t *testing.T) {
		f.t = t
		empaquetarPlaySondeoInterno(t, f)
	})
	t.Run("internal consulta sin publicar y completa sólo el tag conservando probadores y targeting", func(t *testing.T) {
		f.t = t
		empaquetarPlayInternoCompleta(t, f)
		empaquetarPlayInternoProduccionPar(t, f)
	})
	t.Run("internal no admite dos ediciones de la misma app y no deja la clave", func(t *testing.T) {
		f.t = t
		if r := guiontest.Ejecutar(t, guiontest.Opciones{}, f.guion, "--interno", "--tag", "v3.13.4", "--estado", "completed", "mail", "mail"); r.Codigo == 0 {
			t.Error("internal acepta dos ediciones simultáneas de la misma app")
		}
		if f.expone("argv", "salida") {
			t.Error("play-publicar.sh deja la clave privada o el token en argv o en el log")
		}
		if quedan, _ := filepath.Glob(empaquetarUnir(f.dir, "play-*")); len(quedan) > 0 {
			t.Errorf("play-publicar.sh no borra su directorio temporal (la clave): %v", quedan)
		}
	})
	t.Run("huellas", func(t *testing.T) {
		f.t = t
		empaquetarPlayHuellas(t, f)
	})
	t.Run("production sustituye sólo el borrador anterior fijado y bloquea revisión/rollout antes de escribir", func(t *testing.T) {
		f.t = t
		empaquetarPlayProduccionPreflight(t, f)
		empaquetarPlayProduccionDraft(t, f)
		empaquetarPlayProduccionPin(t, f)
		empaquetarPlayProduccionReanudar(t, f)
	})
	// Control negativo: al quitar el pin, el mismo guardián debe rechazar la mutación.
	empaquetarPlayMutaciones(t, f)
	f.t = t
	if f.expone("argv", "produccion") {
		t.Error("producción deja la clave privada o el token en argv o en el log")
	}
}

func empaquetarPlaySondeoInterno(t *testing.T, f *empaquetarPlay) {
	if r := f.interno("interno-normal", "sondear"); r.Codigo != 0 {
		t.Errorf("no se puede consultar internal: %s", empaquetarUltima(f.leer("interno")))
	}
	if f.modifica() {
		t.Error("la consulta internal modifica una pista")
	}
	if f.cuenta(`^GET .*testers/internal$`) != 2 {
		t.Error("no se consulta la configuración de probadores")
	}
	if strings.Contains(f.leer("interno"), "testers-privados@example.com") {
		t.Error("el sondeo internal revela los grupos privados")
	}
	if f.cuenta(`^DELETE .*edits/e1$`) != 2 {
		t.Error("el sondeo internal deja ediciones abiertas")
	}
	revela := regexp.MustCompile(`NO-MOSTRAR|testers-privados@example\.com`)
	for _, escenario := range []string{"interno-coexistente", "interno-coexistente-inverso"} {
		if r := f.interno(escenario, "sondear", "v3.14.1"); r.Codigo != 0 {
			t.Errorf("no se consulta el borrador junto a la versión activa: %s", escenario)
		}
		if f.modifica() {
			t.Error("consultar dos releases modifica una pista")
		}
		if f.cuenta(`^GET .*testers/internal$`) != 2 {
			t.Error("la consulta de dos releases pierde la lectura de probadores")
		}
		if f.cuenta(`^DELETE .*edits/e1$`) != 2 {
			t.Error("la consulta de dos releases deja ediciones abiertas")
		}
		if revela.MatchString(f.leer("interno")) {
			t.Error("la consulta de dos releases revela notas, nombres o grupos")
		}
		if !empaquetarPlayDosVersiones(f.filas()) {
			t.Error("el sondeo no conserva las dos versiones y sus estados sin confundir el tag")
		}
		if r := f.interno(escenario, "completed", "v3.14.1"); r.Codigo != 0 {
			t.Error("no se completa el borrador exacto junto a la versión anterior")
		}
		if f.cuenta(`^PUT .*tracks/internal$`) != 2 {
			t.Error("no se completa exactamente el borrador de las dos apps")
		}
		if f.cuenta(`^POST .*:commit`) != 2 {
			t.Error("no se confirman exactamente las dos pistas internas")
		}
		if f.escribeFuera() {
			t.Error("completar el borrador coexistente modifica producción, ficha, bundles o probadores")
		}
	}
	for _, escenario := range []string{"duplicado", "mixto", "ausente", "ajeno-draft", "ajeno-nuevo", "ajeno-mixto", "ajeno-numero", "ajeno-enorme", "desconocido", "fraccion", "tres"} {
		if r := f.interno("interno-coexistente-"+escenario, "sondear", "v3.14.1"); r.Codigo == 0 {
			t.Errorf("el sondeo acepta releases ambiguas: %s", escenario)
		}
		if f.modifica() {
			t.Error("un sondeo ambiguo modifica pistas")
		}
		if f.cuenta(`^DELETE .*edits/e1$`) != 2 {
			t.Error("un sondeo ambiguo deja ediciones abiertas")
		}
		if revela.MatchString(f.leer("interno")) {
			t.Error("el diagnóstico ambiguo revela notas, nombres o grupos")
		}
	}
}

// empaquetarPlayDosVersiones: dos filas con sus cinco campos, el tag esperado y las dos
// releases (el borrador y la activa) con solo versionCodes y status.
func empaquetarPlayDosVersiones(filas []map[string]any) bool {
	if len(filas) != 2 {
		return false
	}
	for _, fila := range filas {
		if !slices.Equal(empaquetarClavesDe(fila), []string{"app", "emailListsVisibleViaAPI", "expectedVersionCode", "googleGroups", "internal"}) ||
			fila["expectedVersionCode"] != "3014001" || fila["googleGroups"] != float64(1) || fila["emailListsVisibleViaAPI"] != false {
			return false
		}
		releases, ok := fila["internal"].([]any)
		if !ok {
			return false
		}
		releases = slices.Clone(releases)
		slices.SortFunc(releases, func(a, b any) int {
			x, _ := json.Marshal(empaquetarEn(a, "versionCodes"))
			y, _ := json.Marshal(empaquetarEn(b, "versionCodes"))
			return strings.Compare(string(x), string(y))
		})
		if !empaquetarIgualJSON(releases, `[{"versionCodes":["3014000"],"status":"completed"},{"versionCodes":["3014001"],"status":"draft"}]`) {
			return false
		}
	}
	return true
}

// empaquetarPlayRechazoFiltrado: la fila de rooms explica el rechazo solo con track,
// versionCodes y status.
func empaquetarPlayRechazoFiltrado(filas []map[string]any) bool {
	for _, fila := range filas {
		if fila["app"] != "rooms" {
			continue
		}
		interno, ok := fila["internal"].(map[string]any)
		if !ok || !slices.Equal(empaquetarClavesDe(fila), []string{"app", "internal"}) || !slices.Equal(empaquetarClavesDe(interno), []string{"releases", "track"}) {
			return false
		}
		if _, ok := interno["track"].(string); !ok {
			return false
		}
		releases, ok := interno["releases"].([]any)
		if !ok || len(releases) == 0 {
			return false
		}
		for _, rel := range releases {
			m, ok := rel.(map[string]any)
			if !ok || !slices.Equal(empaquetarClavesDe(m), []string{"status", "versionCodes"}) {
				return false
			}
		}
		return true
	}
	return false
}

func empaquetarPlayInternoCompleta(t *testing.T, f *empaquetarPlay) {
	revela := regexp.MustCompile(`NO-MOSTRAR|testers-privados@example\.com`)
	for _, escenario := range []string{"interno-track-invalido", "interno-multiples", "interno-parcial"} {
		if r := f.interno(escenario, "sondear"); r.Codigo == 0 {
			t.Errorf("internal acepta %s", escenario)
		}
		if f.modifica() {
			t.Error("un sondeo internal rechazado modifica pistas")
		}
		if f.cuenta(`^DELETE .*edits/e1$`) != 2 {
			t.Error("un sondeo internal rechazado deja ediciones abiertas")
		}
		if revela.MatchString(f.leer("interno")) {
			t.Error("el diagnóstico internal revela datos libres o grupos privados")
		}
		if !empaquetarPlayRechazoFiltrado(f.filas()) {
			t.Error("internal no explica el rechazo con metadatos filtrados")
		}
	}
	mail := f.ruta("internos/eu.sytue.mail.json")
	for _, escenario := range []string{"interno-normal", "interno-anterior"} {
		if r := f.interno(escenario, "completed"); r.Codigo != 0 {
			t.Error("no se completa internal con el AAB ya presente")
		}
		if f.cuenta(`^PUT .*tracks/internal$`) != 2 {
			t.Error("internal no actualiza exactamente las dos apps")
		}
		if f.escribeFuera() {
			t.Error("completar internal modifica producción, ficha, AAB o probadores")
		}
		rel := empaquetarEn(f.json(mail), "releases", 0)
		if !empaquetarIgualJSON(empaquetarEn(rel, "versionCodes"), `["3013004"]`) || empaquetarEn(rel, "status") != "completed" ||
			empaquetarEn(rel, "releaseNotes", 0, "text") != "Notas revisadas" || !empaquetarIgualJSON(empaquetarEn(rel, "countryTargeting", "countries"), `["ES"]`) ||
			empaquetarEn(rel, "inAppUpdatePriority") != float64(2) {
			t.Error("internal pierde el pin, notas, targeting o prioridad existentes")
		}
	}
	if r := f.interno("interno-completado", "completed"); r.Codigo != 0 {
		t.Error("internal completed no es idempotente")
	}
	if empaquetarHay(f.peticiones(), `^PUT|:commit`) {
		t.Error("internal ya completado se vuelve a publicar")
	}
	if r := f.interno("interno-falta", "completed"); r.Codigo == 0 {
		t.Error("se completa internal sin el bundle del tag")
	}
	if empaquetarHay(f.peticiones(), `^PUT|:commit`) {
		t.Error("un preflight incompleto modifica otra app")
	}
	if r := f.interno("interno-normal", "completed", "v3.13.5"); r.Codigo == 0 {
		t.Error("internal ignora el tag esperado")
	}
	if empaquetarHay(f.peticiones(), `^PUT|:commit`) {
		t.Error("un tag diferente modifica una pista")
	}
	if r := f.interno("interno-rechazado", "completed"); r.Codigo == 0 {
		t.Error("se oculta el rechazo draft de Google")
	}
	if !strings.Contains(f.leer("interno"), "Only releases with status draft") {
		t.Error("se pierde el motivo del rechazo internal")
	}
	if r := f.interno("interno-vacio", "completed"); r.Codigo != 0 {
		t.Error("internal vacío no admite el bundle ya subido")
	}
	for _, escenario := range []string{"interno-par-borrador-primero", "interno-par-activo-primero"} {
		if r := f.interno(escenario, "sondear"); r.Codigo != 0 {
			t.Error("el sondeo rechaza el draft del tag junto al completed anterior")
		}
		if f.modifica() {
			t.Error("sondear el par modifica pistas")
		}
		if f.cuenta(`^DELETE .*edits/e1$`) != 2 {
			t.Error("sondear el par deja ediciones abiertas")
		}
		if r := f.interno(escenario, "completed"); r.Codigo != 0 {
			t.Error("no se completa el draft del tag junto al completed anterior")
		}
		if f.cuenta(`^PUT .*tracks/internal$`) != 2 {
			t.Error("el par no publica exactamente las dos apps")
		}
		releases := empaquetarEn(f.json(mail), "releases")
		if l, ok := releases.([]any); !ok || len(l) != 1 ||
			!empaquetarIgualJSON(empaquetarEn(releases, 0, "versionCodes"), `["3013004"]`) || empaquetarEn(releases, 0, "status") != "completed" ||
			empaquetarEn(releases, 0, "releaseNotes", 0, "text") != "Notas del borrador" ||
			!empaquetarIgualJSON(empaquetarEn(releases, 0, "countryTargeting", "countries"), `["ES"]`) || empaquetarEn(releases, 0, "inAppUpdatePriority") != float64(3) {
			t.Error("el par hereda campos del activo por posición o pierde el pin")
		}
	}
	for _, escenario := range []string{"dos-drafts", "dos-completed", "parcial", "otro-tag", "mezclado", "igual", "futuro", "duplicado", "texto", "vacio", "fraccion", "halted", "tres"} {
		if r := f.interno("interno-par-"+escenario, "completed"); r.Codigo == 0 {
			t.Errorf("internal acepta un par ambiguo: %s", escenario)
		}
		if f.modifica() {
			t.Errorf("el par inválido modifica otra app: %s", escenario)
		}
		if f.cuenta(`^DELETE .*edits/e1$`) != 2 {
			t.Errorf("el par inválido deja ediciones: %s", escenario)
		}
	}
}

// Producción mantiene un pin distinto: no admite aún un completed anterior.
func empaquetarPlayInternoProduccionPar(t *testing.T, f *empaquetarPlay) {
	f.vaciar("peticiones")
	r := f.play(f.guion, "produccion-par", []string{"PLAY_ESCENARIO=interno-par-borrador-primero", "PLAY_CONFIRMAR_PRODUCCION=PUBLICAR_15_APPS"},
		"--produccion", "--tag", "v3.13.4", "--estado", "draft")
	if r.Codigo == 0 {
		t.Error("producción acepta el par antes de completar la actualización interna")
	}
	if f.modifica() {
		t.Error("el par interno permite modificar production")
	}
	if f.cuenta(`^DELETE .*edits/e1$`) != 15 {
		t.Error("el rechazo de production deja ediciones abiertas")
	}
	if empaquetarCuenta(f.leer("produccion-par"), `internal debe contener exclusivamente versionCode 3013004`) != 15 {
		t.Error("el rechazo de production no procede de su pin independiente")
	}
}

func empaquetarPlayHuellas(t *testing.T, f *empaquetarPlay) {
	huellas := func(env []string, args ...string) guiontest.Resultado {
		t.Helper()
		r := f.play(f.guion, "", env, args...)
		guiontest.Escribir(t, f.ruta("huellas"), r.Salida)
		guiontest.Escribir(t, f.ruta("error-huellas"), r.Error)
		return r
	}
	f.vaciar("peticiones")
	if r := huellas(nil, "--huellas", "--tag", "v3.13.4", "mail"); r.Codigo != 0 {
		t.Errorf("la consulta de huellas de Play falla: %s", empaquetarUltima(r.Error))
	}
	h := f.json(f.ruta("huellas"))
	if empaquetarEn(h, "package_name") != "eu.sytue.mail" || empaquetarEn(h, "version_code") != float64(3013004) ||
		!empaquetarIgualJSON(empaquetarEn(h, "certificate_sha256"), `["aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"]`) {
		t.Error("Play no conserva las firmas finales rotadas y normalizadas del tag")
	}
	if strings.TrimRight(f.peticiones(), "\n") != "GET https://androidpublisher.googleapis.com/androidpublisher/v3/applications/eu.sytue.mail/generatedApks/3013004" {
		t.Errorf("consultar huellas de Play abre una edición, publica o consulta otra versión: %q", f.peticiones())
	}
	if r := huellas([]string{"PLAY_ESCENARIO=huellas-base64"}, "--huellas", "--tag", "v3.13.4", "mail"); r.Codigo != 0 {
		t.Error("Play no acepta las codificaciones públicas de SHA256 de 32 bytes")
	}
	if !empaquetarIgualJSON(empaquetarEn(f.json(f.ruta("huellas")), "certificate_sha256"), `["ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"]`) {
		t.Error("las codificaciones Base64, URL y hexadecimal no coinciden byte a byte")
	}
	for _, escenario := range []string{"sin-huellas", "huella-rota"} {
		if r := huellas([]string{"PLAY_ESCENARIO=" + escenario}, "--huellas", "--tag", "v3.13.4", "mail"); r.Codigo == 0 {
			t.Errorf("la consulta de huellas acepta %s", escenario)
		}
	}
	f.vaciar("peticiones")
	if r := huellas([]string{"PLAY_ESCENARIO=falta-docus"}, "--huellas", "--tag", "v3.13.4", "docus", "mail"); r.Codigo == 0 {
		t.Error("un AAB ausente se considera catálogo completo")
	}
	if empaquetarEn(f.json(f.ruta("huellas")), "package_name") != "eu.sytue.mail" {
		t.Error("la consulta se para antes de comprobar las otras apps")
	}
	if !strings.Contains(f.leer("error-huellas"), "catálogo incompleto; faltan certificados finales de: docus") {
		t.Error("no se identifica la app sin firma final")
	}
	if f.cuenta(`^GET .*generatedApks/3013004$`) != 2 {
		t.Error("un AAB ausente cambia las consultas de versión o escribe en Play")
	}
	for _, args := range [][]string{
		{"--huellas", "mail"}, // la consulta de huellas acepta una versión implícita
		{"--huellas", "--tag", "v3.13.4", "--aab", "/tmp", "mail"}, // la consulta de huellas permite subir AAB
		{"--huellas", "--tag", "v3.13.4", "../mail"},               // la consulta de huellas acepta una ruta como app
	} {
		if r := guiontest.Ejecutar(t, guiontest.Opciones{}, f.guion, args...); r.Codigo == 0 {
			t.Errorf("la consulta de huellas acepta %v", args)
		}
	}
	if f.expone("argv", "huellas", "error-huellas") {
		t.Error("la consulta de huellas expone credenciales")
	}
}

func empaquetarPlayProduccionPreflight(t *testing.T, f *empaquetarPlay) {
	for _, escenario := range []string{"vacio", "nuevo", "mezclado", "estable", "otro-borrador", "vacio-con-resumen"} {
		if !f.rechazaSinPromover(f.guion, escenario) {
			t.Errorf("producción modifica releases pese a preflight %s en la última app", escenario)
		}
		if f.cuenta(`^GET .*/tracks/production`) < 14 {
			t.Errorf("el caso %s no alcanzó la última app antes de rechazar", escenario)
		}
	}
	if !f.rechazaSinPromover(f.guion, "normal", "v3.12.5") {
		t.Error("producción no fija el versionCode al tag solicitado")
	}
	// Ni el tag ni el estado tienen un valor implícito, y una selección parcial no publica.
	for _, args := range []string{"--estado draft", "--tag v3.12.4", "--tag v3.12.4 --estado raro", "--tag v3.12.4 --estado draft mail"} {
		f.reiniciar()
		if r := f.play(f.guion, "produccion", []string{"PLAY_CONFIRMAR_PRODUCCION=PUBLICAR_15_APPS"}, append([]string{"--produccion"}, strings.Fields(args)...)...); r.Codigo == 0 {
			t.Errorf("producción acepta parámetros incompletos o parciales: %s", args)
		}
		if f.peticiones() != "" {
			t.Error("parámetros inválidos de producción abren una edición")
		}
	}
}

func empaquetarPlayProduccionDraft(t *testing.T, f *empaquetarPlay) {
	f.reiniciar()
	if r := f.produccion(f.guion, "normal", "draft"); r.Codigo != 0 {
		t.Errorf("no se preparan los quince borradores: %s", empaquetarUltima(f.leer("produccion")))
	}
	if f.cuenta(`^PUT `) != 15 {
		t.Error("draft no escribe quince tracks production")
	}
	if f.cuenta(`^POST .*:commit\?`) != 15 {
		t.Error("draft no confirma quince ediciones")
	}
	if f.cuenta(`^POST .*:commit\?changesInReviewBehavior=ERROR_IF_IN_REVIEW&changesNotSentForReview=true$`) != 15 {
		t.Error("draft puede enviar cambios a revisión o cancelar una revisión existente")
	}
	if f.antesDelPrimerPUT(`^GET .*/tracks/production$`) != 15 {
		t.Error("el primer PUT de producción precede a los quince preflights")
	}
	if f.antesDelPrimerPUT(`^GET .*/applications/eu\.sytue\.[a-z]+/tracks/production/releases$`) != 15 {
		t.Error("el primer PUT de producción precede a los quince controles de revisión fuera de la edición")
	}
	for _, track := range f.tracks() {
		v := f.json(track)
		if empaquetarEn(v, "track") != "production" || !empaquetarIgualJSON(empaquetarEn(v, "releases"), `[{"name":"v3.12.4","versionCodes":["3012004"],"status":"draft"}]`) {
			t.Errorf("draft usa otro versionCode/estado en %s", filepath.Base(track))
		}
	}
	if empaquetarCuenta(f.leer("produccion"), `sin envío a revisión ni publicación pública`) != 15 {
		t.Error("draft no explica que preparar un borrador no publica las apps")
	}
	if l := empaquetarLineas(f.peticiones(), `bundles|listings|details|dataSafety`); len(l) > 0 {
		t.Errorf("producción toca AAB, ficha o declaraciones: %s", strings.Join(l, "\n"))
	}

	f.vaciar("peticiones")
	if r := f.produccion(f.guion, "normal", "draft"); r.Codigo != 0 {
		t.Error("repetir el mismo borrador no es idempotente")
	}
	if f.escribe() {
		t.Error("repetir el mismo borrador vuelve a modificar production")
	}
	if f.cuenta(`^DELETE `) != 15 {
		t.Error("el rerun deja ediciones abiertas")
	}

	f.reiniciar()
	if r := f.produccion(f.guion, "objetivo-v5", "draft", "v3.12.5"); r.Codigo != 0 {
		t.Error("el pin no admite un nuevo tag explícito con sus quince AAB")
	}
	for _, track := range f.tracks() {
		if !empaquetarIgualJSON(empaquetarEn(f.json(track), "releases"), `[{"name":"v3.12.5","versionCodes":["3012005"],"status":"draft"}]`) {
			t.Error("el pin ignora el tag solicitado y conserva otro versionCode")
		}
	}

	f.reiniciar()
	if r := f.produccion(f.guion, "mismo", "completed"); r.Codigo != 0 {
		t.Error("completed no completa el mismo borrador")
	}
	if f.cuenta(`^POST .*:commit\?changesInReviewBehavior=ERROR_IF_IN_REVIEW$`) != 15 {
		t.Error("completed no usa el commit explícito para revisión")
	}
	for _, track := range f.tracks() {
		if !empaquetarIgualJSON(empaquetarEn(f.json(track), "releases"), `[{"name":"Conservar","versionCodes":["3012004"],"status":"completed","releaseNotes":[{"language":"es-ES","text":"Notas revisadas"}]}]`) {
			t.Error("completar el borrador pierde las notas o cambia el versionCode")
		}
	}
	f.reiniciar()
	if r := f.produccion(f.guion, "rechazar-commit", "draft"); r.Codigo == 0 {
		t.Error("se oculta un rechazo del commit draft")
	}
	if f.cuenta(`^PUT `) != 1 {
		t.Error("el rechazo draft provoca otro intento/estado o continúa escribiendo")
	}
	pendientes, _ := filepath.Glob(f.ruta("pendientes/*.json"))
	for _, p := range pendientes {
		if empaquetarHay(empaquetarLeer(p), `status.*completed`) {
			t.Error("un rechazo draft cae automáticamente a completed")
		}
	}
}

// payloadAnteriorCorrecto es `payload_anterior_correcto`: el target v3.14.0 con las notas
// y la prioridad del borrador anterior.
func (f *empaquetarPlay) payloadAnteriorCorrecto(estado, ruta string) bool {
	return empaquetarIgualJSON(f.json(ruta), `{"track":"production","releases":[{"name":"v3.14.0","versionCodes":["3014000"],"status":"`+estado+`",
		"releaseNotes":[{"language":"es-ES","text":"Notas revisadas"},{"language":"en-US","text":"Reviewed notes"}],"inAppUpdatePriority":3}]}`)
}

// El borrador anterior sólo puede sustituirse con su tag explícito. Reproduce
// v3.12.24 -> v3.14.0: el éxito HTTP por sí solo no demuestra que el payload haya
// dejado de apuntar al AAB anterior, ni que el rerun no lo omita como idempotente.
func empaquetarPlayProduccionPin(t *testing.T, f *empaquetarPlay) {
	if !f.rechazaSinPromover(f.guion, "pin-normal", "v3.14.0") {
		t.Error("producción sustituye v3.12.24 sin --borrador-anterior")
	}
	if !f.rechazaSinPromover(f.guion, "pin-normal", "v3.14.0", "v3.12.23") {
		t.Error("producción sustituye un borrador distinto del pin explícito")
	}
	for _, escenario := range []string{
		"pin-otro", "pin-mezclado", "pin-futuro", "pin-completed", "pin-inProgress", "pin-halted", "pin-dos",
		"pin-campo-desconocido", "pin-countryTargeting", "pin-userFraction", "pin-notas-invalidas", "pin-prioridad-invalida", "pin-interno-mal",
	} {
		if !f.rechazaSinPromover(f.guion, escenario, "v3.14.0", "v3.12.24") {
			t.Errorf("el pin de borrador anterior permite modificar releases con %s", escenario)
		}
		if f.cuenta(`^POST .*/edits$`) != 15 {
			t.Errorf("%s no comprobó la última app antes de rechazar todas las escrituras", escenario)
		}
	}
	// El estado de revisión no aparece en tracks.get. Incluso un draft editable
	// debe coincidir con exactamente un resumen DRAFT y su artefacto numérico.
	for _, escenario := range []string{
		"pin-ciclo-in-review", "pin-ciclo-approved", "pin-ciclo-desconocido", "pin-ciclo-ausente", "pin-ciclo-vacio",
		"pin-ciclo-doble", "pin-ciclo-track", "pin-ciclo-version", "pin-ciclo-mixto", "pin-ciclo-texto", "pin-ciclo-campo", "pin-ciclo-error",
	} {
		if !f.rechazaSinPromover(f.guion, escenario, "v3.14.0", "v3.12.24") {
			t.Errorf("producción modifica releases con revisión desconocida o bloqueada: %s", escenario)
		}
		if f.cuenta(`^GET .*/applications/eu\.sytue\.[a-z]*/tracks/production/releases$`) != 15 {
			t.Errorf("%s no consultó los quince estados de revisión antes de rechazar", escenario)
		}
	}
	// Las opciones se validan antes de firmar/solicitar OAuth, no después de abrir
	// una edición. Ni otra modalidad ni un pin igual/futuro admite esta excepción.
	for _, args := range []string{
		"--produccion --tag v3.14.0 --estado draft --borrador-anterior v3.14.0",
		"--produccion --tag v3.14.0 --estado draft --borrador-anterior v3.14.1",
		"--produccion --tag v3.14.0 --estado draft --borrador-anterior anterior",
		"--produccion --tag v3.14.0 --estado draft --borrador-anterior",
		"--interno --tag v3.14.0 --estado completed --borrador-anterior v3.12.24",
		"--huellas --tag v3.14.0 --borrador-anterior v3.12.24",
		"--sondear --borrador-anterior v3.12.24", "--seco --borrador-anterior v3.12.24",
	} {
		f.reiniciar()
		if r := f.play(f.guion, "produccion", []string{"PLAY_CONFIRMAR_PRODUCCION=PUBLICAR_15_APPS"}, strings.Fields(args)...); r.Codigo == 0 {
			t.Errorf("se acepta un pin inválido o fuera de producción: %s", args)
		}
		if f.peticiones() != "" || f.leer("autenticaciones") != "" {
			t.Errorf("un pin inválido inicia OAuth o abre una edición: %s", args)
		}
	}
	for _, estado := range []string{"draft", "completed"} {
		empaquetarPlaySustituirPin(t, f, estado)
	}
}

func empaquetarPlaySustituirPin(t *testing.T, f *empaquetarPlay, estado string) {
	f.reiniciar()
	if r := f.produccion(f.guion, "pin-normal", estado, "v3.14.0", "v3.12.24"); r.Codigo != 0 {
		t.Errorf("no se sustituye el borrador fijado v3.12.24 por v3.14.0/%s: %s", estado, empaquetarUltima(f.leer("produccion")))
	}
	if f.cuenta(`^PUT .*tracks/production$`) != 15 {
		t.Error("sustituir el borrador anterior no escribe exactamente las quince apps")
	}
	if f.cuenta(`^POST .*:commit\?`) != 15 {
		t.Error("sustituir el borrador anterior no confirma exactamente las quince ediciones")
	}
	if f.antesDelPrimerPUT(`^GET .*/applications/eu\.sytue\.[a-z]+/tracks/production/releases$`) != 15 {
		t.Error("se sustituye un borrador antes de conocer la revisión de las quince apps")
	}
	for _, track := range f.tracks() {
		if !f.payloadAnteriorCorrecto(estado, track) {
			t.Errorf("la sustitución %s conserva otro AAB/metadata o pierde notas/prioridad en %s", estado, filepath.Base(track))
		}
	}
	if estado == "draft" {
		f.vaciar("peticiones")
		if r := f.produccion(f.guion, "pin-normal", "draft", "v3.14.0", "v3.12.24"); r.Codigo != 0 {
			t.Error("repetir el borrador sustituido deja de ser idempotente")
		}
		if f.escribe() {
			t.Error("repetir el mismo target con pin anterior vuelve a modificar producción")
		}
		if f.cuenta(`^DELETE `) != 15 {
			t.Error("el rerun del borrador sustituido deja ediciones abiertas")
		}
		return
	}
	f.vaciar("peticiones")
	if r := f.produccion(f.guion, "pin-normal", "completed", "v3.14.0", "v3.12.24"); r.Codigo != 0 {
		t.Error("repetir completed del target ya publicado no permite reanudar el lote")
	}
	if f.escribe() {
		t.Error("repetir completed del target vuelve a publicar")
	}
	f.vaciar("peticiones")
	if r := f.produccion(f.guion, "pin-normal", "draft", "v3.14.0", "v3.12.24"); r.Codigo == 0 {
		t.Error("producción completed admite un downgrade a draft")
	}
	if f.escribe() {
		t.Error("se modifica una release production ya completed")
	}
}

// Reanudar no reenvía las apps ya entregadas a Google. Su lifecycle debe ser
// inequívoco y corresponder al target; el resto conserva el pin del borrador.
func empaquetarPlayProduccionReanudar(t *testing.T, f *empaquetarPlay) {
	for _, escenario := range []string{"pin-ya-completed-in-review", "pin-ya-completed-approved", "pin-ya-completed-published"} {
		f.reiniciar()
		if r := f.produccion(f.guion, escenario, "completed", "v3.14.0", "v3.12.24"); r.Codigo != 0 {
			t.Errorf("el rerun completed no reconoce el target ya entregado: %s", escenario)
		}
		if f.escribe() {
			t.Errorf("%s vuelve a enviar una release ya completed", escenario)
		}
		if f.cuenta(`^DELETE `) != 15 {
			t.Errorf("%s deja ediciones de consulta abiertas", escenario)
		}
		if empaquetarCuenta(f.leer("produccion"), `ya enviada; ciclo RELEASE_LIFECYCLE_STATE_.*; sin cambios`) != 15 {
			t.Errorf("%s no explica el estado de las quince apps ya enviadas", escenario)
		}
	}
	for _, escenario := range []string{"pin-ya-completed-draft", "pin-ya-completed-not-approved", "pin-ya-completed-not-sent", "pin-ya-completed-unknown", "pin-ya-completed-version"} {
		f.reiniciar()
		if r := f.produccion(f.guion, escenario, "completed", "v3.14.0", "v3.12.24"); r.Codigo == 0 {
			t.Errorf("el rerun da por entregada una release con lifecycle contradictorio: %s", escenario)
		}
		if f.escribe() {
			t.Errorf("%s modifica una release antes de aclarar su estado", escenario)
		}
	}
	f.reiniciar()
	if r := f.produccion(f.guion, "pin-parcial", "completed", "v3.14.0", "v3.12.24"); r.Codigo != 0 {
		t.Error("no se reanuda un lote con seis apps ya completed y nueve borradores anteriores")
	}
	if f.cuenta(`^PUT .*tracks/production$`) != 9 || f.cuenta(`^POST .*:commit\?`) != 9 {
		t.Error("el rerun parcial no publica exactamente las nueve apps pendientes")
	}
	if f.cuenta(`^DELETE `) != 6 {
		t.Error("el rerun parcial deja abiertas las seis ediciones ya completed")
	}
	if empaquetarCuenta(f.leer("produccion"), `ya enviada; ciclo RELEASE_LIFECYCLE_STATE_PUBLISHED; sin cambios`) != 6 {
		t.Error("el rerun parcial no distingue las seis apps ya entregadas")
	}
	if empaquetarHay(f.peticiones(), `^PUT .*applications/eu\.sytue\.(bases|calcus|calendar|cast|certipdf|contacts)/`) {
		t.Error("el rerun parcial vuelve a publicar una de las seis apps ya entregadas")
	}
	for _, track := range f.tracks() {
		if !f.payloadAnteriorCorrecto("completed", track) {
			t.Errorf("el rerun parcial no fija el target o pierde metadata en %s", filepath.Base(track))
		}
	}
}

// empaquetarSinPinInterno es el `sed` del original: dentro de preparar_produccion, la
// condición del pin de internal pasa a `true`.
func empaquetarSinPinInterno(texto string) string {
	lineas := strings.Split(texto, "\n")
	dentro := false
	pin := regexp.MustCompile(`^ {4}\.track == "internal" and .*`)
	for i, l := range lineas {
		switch {
		case !dentro && strings.HasPrefix(l, "preparar_produccion()"):
			dentro = true
		case dentro && strings.HasPrefix(l, "}"):
			dentro = false
		}
		if dentro && pin.MatchString(l) {
			lineas[i] = "    true"
		}
	}
	return strings.Join(lineas, "\n")
}

// Los controles negativos: el mismo guardián, sobre copias mutadas de play-publicar.sh,
// tiene que verlas en rojo.
func empaquetarPlayMutaciones(t *testing.T, f *empaquetarPlay) {
	mutacion := f.ruta("mutacion")
	scripts := empaquetarUnir(mutacion, "scripts")
	if err := os.MkdirAll(scripts, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(empaquetarUnir(f.aqui, "empaquetar.sh"), empaquetarUnir(scripts, "empaquetar.sh")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(empaquetarUnir(f.fyne, "cmd"), empaquetarUnir(mutacion, "cmd")); err != nil {
		t.Fatal(err)
	}
	fuente := guiontest.Leer(t, f.guion)
	mutar := func(nombre, texto string) string {
		f.t.Helper()
		ruta := empaquetarUnir(scripts, nombre)
		empaquetarEjecutable(f.t, ruta, texto)
		return ruta
	}
	t.Run("el guardián de Play detecta la mutación sin pin del versionCode", func(t *testing.T) {
		f.t = t
		sinPin := empaquetarSinPinInterno(fuente)
		ruta := mutar("play-publicar.sh", sinPin)
		switch {
		case sinPin == fuente:
			t.Error("el control negativo de Play no quitó el pin de internal")
		case f.rechazaSinPromover(ruta, "nuevo"):
			t.Error("el guardián de Play no ve en rojo la mutación sin pin")
		case f.cuenta(`^PUT `) != 15:
			t.Error("la mutación sin pin no llegó a escribir las quince releases")
		}
	})
	reemplazarUnaVez := func(nombre, viejo, nuevo string) string {
		f.t.Helper()
		if strings.Count(fuente, viejo) != 1 {
			f.t.Fatalf("no se pudo generar la mutación %s: «%s» no aparece exactamente una vez", nombre, viejo)
		}
		return mutar(nombre, strings.Replace(fuente, viejo, nuevo, 1))
	}
	t.Run("el guardián internal ve en rojo la mutación que acepta un bundle ausente", func(t *testing.T) {
		f.t = t
		ruta := reemplazarUnaVez("internal-sin-pin.sh", `any(.bundles[]?; (.versionCode | tostring) == $v)`, "true")
		if r := f.interno("interno-falta", "completed", "v3.13.4", ruta); r.Codigo != 0 {
			t.Error("el control negativo internal no quitó la comprobación del bundle")
		} else if f.cuenta(`^PUT .*tracks/internal$`) != 2 {
			t.Error("la mutación internal no llegó a publicar el bundle ausente")
		}
	})
	t.Run("el guardián production detecta la mutación que ignora el pin del borrador anterior", func(t *testing.T) {
		f.t = t
		ruta := reemplazarUnaVez("production-sin-pin-anterior.sh", `.versionCodes == [$anterior]`, "true")
		switch {
		case f.rechazaSinPromover(ruta, "pin-otro", "v3.14.0", "v3.12.24"):
			t.Error("el guardián no ve en rojo la mutación que ignora el borrador anterior autorizado")
		case f.cuenta(`^PUT .*tracks/production$`) != 15:
			t.Error("la mutación sin pin anterior no llegó a escribir el borrador incorrecto")
		}
	})
	t.Run("el guardián production detecta la mutación sin protección del ciclo de revisión", func(t *testing.T) {
		f.t = t
		ruta := reemplazarUnaVez("production-sin-ciclo.sh", `.releaseLifecycleState == "RELEASE_LIFECYCLE_STATE_DRAFT"`, "true")
		switch {
		case f.rechazaSinPromover(ruta, "pin-ciclo-in-review", "v3.14.0", "v3.12.24"):
			t.Error("el guardián no ve en rojo la mutación que acepta un borrador en revisión")
		case f.cuenta(`^PUT .*tracks/production$`) != 15:
			t.Error("la mutación sin protección de revisión no llegó a escribir las quince releases")
		}
	})
	t.Run("el guardián production detecta la mutación que no sustituye el versionCode anterior", func(t *testing.T) {
		f.t = t
		ruta := reemplazarUnaVez("production-sin-cambiar-version.sh",
			`else .name = $t | .versionCodes = [$v] | .status = $s end] end)`, `else .name = $t | .status = $s end] end)`)
		f.reiniciar()
		if r := f.produccion(ruta, "pin-normal", "draft", "v3.14.0", "v3.12.24"); r.Codigo != 0 {
			t.Fatal("el control negativo no quitó la sustitución del versionCode del payload")
		}
		if f.cuenta(`^PUT .*tracks/production$`) != 15 {
			t.Error("la mutación del payload no llegó a escribir las quince releases")
		}
		mail := f.ruta("tracks/eu.sytue.mail.json")
		switch {
		case f.payloadAnteriorCorrecto("draft", mail):
			t.Error("el guardián no ve en rojo un payload que conserva el versionCode anterior")
		case !empaquetarIgualJSON(empaquetarEn(f.json(mail), "releases", 0, "versionCodes"), `["3012024"]`):
			t.Error("la mutación del payload no reprodujo la conservación del versionCode anterior")
		}
	})
}
