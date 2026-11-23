// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package pruebasfyne

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/tools/guiontest"
)

const empaquetarEquipo = "VHC5QW7BZ3"

// empaquetarSinVars ejecuta empaquetar.sh sin esas variables en el entorno (`env -u`).
func empaquetarSinVars(t *testing.T, quitar, env []string, args ...string) guiontest.Resultado {
	t.Helper()
	var base []string
	for _, kv := range os.Environ() {
		clave, _, _ := strings.Cut(kv, "=")
		if !slices.Contains(quitar, clave) {
			base = append(base, kv)
		}
	}
	base = append(base, empaquetarGoprivate)
	return guiontest.Ejecutar(t, guiontest.Opciones{Limpio: true, Env: append(base, env...)}, empaquetarDirs(t).guion, args...)
}

// --- 13. La Mac App Store: sandbox, derechos mínimos y sin actualizador propio.
func TestEmpaquetarPermisosDelPayloadDeApple(t *testing.T) {
	app := empaquetarUnir(t.TempDir(), "Permisos.app")
	perfil := empaquetarUnir(app, "Contents/embedded.provisionprofile")
	guiontest.Escribir(t, perfil, "perfil público de distribución\n")
	chmod := func(ruta string, modo os.FileMode) {
		t.Helper()
		if err := os.Chmod(ruta, modo); err != nil {
			t.Fatal(err)
		}
	}
	chmod(app, 0o755)
	chmod(empaquetarUnir(app, "Contents"), 0o755)
	chmod(perfil, 0o600)
	if r := empaquetar(t, "verificar-permisos-apple", app); r.Codigo == 0 {
		t.Error("Apple acepta un perfil 0600 en un payload que pertenecerá a root")
	}
	chmod(perfil, 0o644)
	if r := empaquetar(t, "verificar-permisos-apple", app); r.Codigo != 0 {
		t.Errorf("Apple rechaza un perfil 0644 públicamente legible: %s", r.Todo())
	}
	chmod(empaquetarUnir(app, "Contents"), 0o700)
	if r := empaquetar(t, "verificar-permisos-apple", app); r.Codigo == 0 {
		t.Error("Apple acepta un directorio privado aunque el perfil sea legible")
	}
	chmod(empaquetarUnir(app, "Contents"), 0o755)
	if r := empaquetar(t, "verificar-permisos-apple", app); r.Codigo != 0 {
		t.Error("Apple rechaza el payload con directorios 0755 y perfil 0644")
	}
}

// La copia de seguridad de Photos con la app cerrada (TareaFondo, `fototeca`): sin el
// servicio con BIND_JOB_SERVICE, JobScheduler no lo encola; sin RECEIVE_BOOT_COMPLETED,
// el trabajo persistente lanza una excepción al programarse; sin ACCESS_NETWORK_STATE,
// «solo con Wi-Fi» no sabe qué red hay. Solo en las apps que leen la galería.
func TestEmpaquetarTareaDeFondoDeLaFototeca(t *testing.T) {
	guion := guiontest.Leer(t, empaquetarDirs(t).guion)
	if !empaquetarHay(guion, `tarea_fondo" != si \] \|\| printf .*android:name="eu.sytue.nativo.TareaFondo" android:permission="android.permission.BIND_JOB_SERVICE"`) {
		t.Error("el manifiesto no declara TareaFondo (con BIND_JOB_SERVICE) en las apps con fototeca")
	}
	for _, permiso := range []string{"READ_MEDIA_VIDEO", "ACCESS_NETWORK_STATE", "RECEIVE_BOOT_COMPLETED"} {
		if !empaquetarHay(guion, `^    permisos_extra\+=\(.*android.permission.`+permiso) {
			t.Errorf("las apps con fototeca no piden %s", permiso)
		}
	}
}

// empaquetarDerechosEsperados son las claves decididas para la tienda, ordenadas y en una
// línea. Una app nueva tiene que decidirse AQUÍ, no heredar derechos.
func empaquetarDerechosEsperados(app string) string {
	comunes := []string{
		"com.apple.application-identifier", "com.apple.developer.team-identifier", "com.apple.security.app-sandbox",
		"com.apple.security.application-groups", "com.apple.security.files.downloads.read-write", "com.apple.security.network.client",
		"com.apple.security.network.server", "keychain-access-groups",
	}
	ficheros := []string{"com.apple.security.files.user-selected.read-write"}
	llamadas := []string{"com.apple.security.device.audio-input", "com.apple.security.device.camera"}
	fototeca := []string{"com.apple.security.personal-information.photos-library"}
	var extra []string
	switch app {
	case "bases", "calendar", "hub":
	case "cast", "rooms":
		extra = append(slices.Clone(ficheros), llamadas...)
	case "calcus", "certipdf", "contacts", "diapos", "docus", "drive", "erp", "mail", "notes":
		extra = ficheros
	case "photos":
		extra = append(slices.Clone(ficheros), fototeca...)
	default:
		extra = []string{"app-sin-decidir"}
	}
	todas := append(slices.Clone(comunes), extra...)
	slices.Sort(todas)
	return strings.Join(todas, " ") + " "
}

// Visto en rojo, uno a uno: quitando `app-sandbox` de derechos_tienda, dándole a todas el
// derecho de ficheros, añadiendo `com.apple.security.print`, cambiando el grupo en
// base_tienda_darwin.go, firmando el .pkg con SYTUE_MAS_IDENTIDAD, quitando `--tags tienda`,
// convirtiendo el `elif` del Developer ID en `if`, quitando el `break` de los bundles,
// devolviendo actualizar_darwin.go a `darwin && !ios` y quitando DeTienda de Plataforma.
func TestEmpaquetarDerechosDeTienda(t *testing.T) {
	d := empaquetarDirs(t)
	for _, app := range empaquetarApps(t) {
		if app == "demo" {
			continue
		}
		r := empaquetar(t, "derechos-tienda", app, empaquetarEquipo)
		plist := empaquetarJunto(r)
		if r.Codigo != 0 {
			t.Errorf("derechos-tienda %s falla: %s", app, plist)
			continue
		}
		if tiene := empaquetarClaves(plist); tiene != empaquetarDerechosEsperados(app) {
			t.Errorf("%s iría a la Mac App Store con «%s» y lo decidido es «%s»", app, tiene, empaquetarDerechosEsperados(app))
		}
		if !strings.Contains(plist, "<key>com.apple.security.app-sandbox</key><true/>") {
			t.Errorf("%s iría a la Mac App Store SIN App Sandbox", app)
		}
		id := empaquetarID(t, empaquetarUnir(d.fyne, "cmd", app, "FyneApp.toml"))
		if !strings.Contains(plist, "<string>"+empaquetarEquipo+"."+id+"</string>") {
			t.Errorf("%s no se firma con su App ID (%s.%s)", app, empaquetarEquipo, id)
		}
		if _, err := empaquetarPlist(t, plist+"\n"); err != nil {
			t.Errorf("los derechos de tienda de %s no son un plist válido", app)
		}
	}
	if r := empaquetar(t, "derechos-tienda", "calcus", "equipo-raro"); r.Codigo == 0 {
		t.Error("derechos-tienda acepta un Team ID que no lo es")
	}
	// El grupo de la suite es el MISMO en la firma y en el código que busca la carpeta compartida.
	grupo := strings.Join(empaquetarSed(guiontest.Leer(t, empaquetarUnir(d.raiz, "apps/nucleo/ajustes/grupo_apple.go")), `^const GrupoSuite = "(.*)"$`), "\n")
	plist := empaquetar(t, "derechos-tienda", "hub", empaquetarEquipo).Texto()
	if grupo == "" || empaquetarCuenta(plist, regexp.QuoteMeta("<string>"+grupo+"</string>")) != 2 {
		t.Errorf("el grupo de la suite del código («%s») no es el de application-groups y keychain-access-groups de la firma", grupo)
	}
}

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

func empaquetarLista(v ...string) []any {
	r := make([]any, 0, len(v))
	for _, s := range v {
		r = append(r, s)
	}
	return r
}

// empaquetarIgual compara un valor de plist con la lista esperada, en orden.
func empaquetarIgual(a any, b []any) bool { return empaquetarMismoJSON(a, b) }

// Los hosts Apple comparten la sesión con derechos mínimos, y no piden el App Group con
// los widgets apagados. Las dos posiciones del interruptor SYTUE_WIDGETS_APPLE: apagado,
// ni rastro del App Group (la firma de antes de los widgets); encendido, el contenedor
// común en todos los hosts.
func TestEmpaquetarDerechosDeSesion(t *testing.T) {
	t.Parallel()
	d := empaquetarDirs(t)
	codigo := guiontest.Leer(t, empaquetarUnir(d.raiz, "apps/nucleo/ajustes/grupo_apple.go"))
	llavero := empaquetarSed(codigo, `^const GrupoSuite = "([^"]+)"$`)
	contenedor := empaquetarSed(codigo, `^const GrupoContenedorSuite = "([^"]+)"$`)
	if len(llavero) == 0 || len(contenedor) == 0 {
		t.Fatal("los hosts Apple no comparten la sesión con derechos mínimos, o piden el App Group con los widgets apagados: sin GrupoSuite o GrupoContenedorSuite")
	}
	for _, activo := range []string{"0", "1"} {
		for _, app := range empaquetarApps(t) {
			if app == "demo" {
				continue
			}
			t.Run(app+"/widgets="+activo, func(t *testing.T) {
				t.Parallel()
				empaquetarComprobarSesion(t, empaquetarUnir(d.fyne, "cmd", app), activo, llavero[0], contenedor[0])
			})
		}
	}
	t.Run("interruptor y extensión", func(t *testing.T) {
		t.Parallel()
		if r := empaquetarCon(t, []string{"SYTUE_WIDGETS_APPLE=1"}, "widgets-apple"); r.Salida != "si\n" {
			t.Errorf("widgets-apple con SYTUE_WIDGETS_APPLE=1 da %q", r.Salida)
		}
		if r := empaquetarCon(t, []string{"SYTUE_WIDGETS_APPLE=0"}, "widgets-apple"); r.Salida != "no\n" {
			t.Errorf("widgets-apple con SYTUE_WIDGETS_APPLE=0 da %q", r.Salida)
		}
		if r := empaquetarCon(t, []string{"SYTUE_WIDGETS_APPLE=quizá"}, "widgets-apple"); r.Codigo == 0 {
			t.Error("widgets-apple acepta SYTUE_WIDGETS_APPLE=quizá")
		}
		for _, modo := range []string{"ios", "macos"} {
			p, ok := empaquetarPlistDe(t, nil, "derechos-archivos", "drive", empaquetarEquipo, modo)
			grupos, _ := empaquetarEn(p, "keychain-access-groups").([]any)
			if !ok || slices.Contains(grupos, any(llavero[0])) {
				t.Errorf("la extensión de archivos (%s) lleva el llavero de la suite: %v", modo, p)
			}
		}
	})
}

func empaquetarComprobarSesion(t *testing.T, dir, activo, llavero, contenedor string) {
	datos := empaquetarTOML(t, empaquetarUnir(dir, "FyneApp.toml"))
	appid, _ := empaquetarEn(datos, "Details", "ID").(string)
	custom, _ := empaquetarEn(datos, "Development").(map[string]any)
	archivos := custom["archivos"] == "si"
	env := []string{"SYTUE_WIDGETS_APPLE=" + activo}
	grupos := empaquetarLista(contenedor)
	claves := empaquetarLista(empaquetarEquipo+"."+appid, llavero)
	if archivos {
		grupos = append(grupos, "group."+appid)
		claves = append(claves, empaquetarEquipo+"."+appid+".shared")
	}
	for _, modo := range []string{"ios", "developerid"} {
		p, ok := empaquetarPlistDe(t, env, "derechos-sesion", dir, empaquetarEquipo, modo)
		if !ok {
			t.Errorf("derechos-sesion %s %s falla", appid, modo)
			continue
		}
		if !empaquetarIgual(p["com.apple.security.application-groups"], grupos) {
			t.Errorf("%s %s: application-groups %v, se esperaba %v", appid, modo, p["com.apple.security.application-groups"], grupos)
		}
		if !empaquetarIgual(p["keychain-access-groups"], claves) {
			t.Errorf("%s %s: keychain-access-groups %v, se esperaba %v", appid, modo, p["keychain-access-groups"], claves)
		}
		_, camara := p["com.apple.security.device.camera"]
		if camara != (modo == "developerid" && custom["llamadas"] == "si") {
			t.Errorf("%s %s: la cámara no sigue a `llamadas` en Developer ID", appid, modo)
		}
		_, fotos := p["com.apple.security.personal-information.photos-library"]
		if fotos != (modo == "developerid" && custom["fototeca"] == "si") {
			t.Errorf("%s %s: la fototeca no sigue a `fototeca` en Developer ID", appid, modo)
		}
		// El despertar por aviso silencioso: `aps-environment` solo en iOS, con su
		// interruptor y el de los widgets, y en las apps con widget. Visto en rojo
		// escribiéndolo también en developerid.
		for _, despertar := range []string{"0", "1"} {
			dp, ok := empaquetarPlistDe(t, append(slices.Clone(env), "SYTUE_DESPERTAR_APPLE="+despertar), "derechos-sesion", dir, empaquetarEquipo, modo)
			acciones, _ := custom["widget_acciones"].(string)
			quiere := despertar == "1" && activo == "1" && modo == "ios" && acciones != ""
			aps, tiene := dp["aps-environment"]
			if !ok || tiene != quiere || (quiere && aps != "production") {
				t.Errorf("%s %s widgets=%s despertar=%s: aps-environment %v", appid, modo, activo, despertar, aps)
			}
		}
	}
	tienda, ok := empaquetarPlistDe(t, env, "derechos-tienda", dir, empaquetarEquipo)
	gruposTienda := empaquetarLista(contenedor, llavero)
	if archivos {
		gruposTienda = append(gruposTienda, "group."+appid)
	}
	if !ok || !empaquetarIgual(tienda["com.apple.security.application-groups"], gruposTienda) {
		t.Errorf("%s widgets=%s: application-groups de tienda %v", appid, activo, tienda["com.apple.security.application-groups"])
	}
	// SecItemAdd usa el PRIMER grupo: lo privado de la app no puede ir por defecto a la suite.
	if !empaquetarIgual(tienda["keychain-access-groups"], claves) {
		t.Errorf("%s widgets=%s: keychain-access-groups de tienda %v", appid, activo, tienda["keychain-access-groups"])
	}
}

// Lo versionado manda: sin variable de entorno, el interruptor es el de Sytue.xcconfig, y
// un release no puede encenderlo sin que se vea en el diff del tag.
func TestEmpaquetarInterruptorDeWidgets(t *testing.T) {
	d := empaquetarDirs(t)
	interruptor := strings.Join(empaquetarSed(guiontest.Leer(t, empaquetarUnir(d.fyne, "xcode/Sytue.xcconfig")), `^SYTUE_WIDGETS_APPLE = ([A-Z]*)$`), "\n")
	switch interruptor + ":" + empaquetarSinVars(t, []string{"SYTUE_WIDGETS_APPLE"}, nil, "widgets-apple").Texto() {
	case "YES:si", "NO:no":
	default:
		t.Errorf("empaquetar.sh widgets-apple no dice lo que fija Sytue.xcconfig («%s»)", interruptor)
	}
	wf := guiontest.Leer(t, empaquetarUnir(d.raiz, ".github/workflows/release-desktop.yml"))
	if !strings.Contains(wf, "SYTUE_WIDGETS_APPLE") || empaquetarHay(wf, `^[[:space:]]+SYTUE_WIDGETS_APPLE:`) {
		t.Error("release-desktop.yml define SYTUE_WIDGETS_APPLE en el entorno o ya no lo consulta: el interruptor es el del tag")
	}
}

func empaquetarOrdenes(l ...string) string { return strings.Join(l, "\n") }

// EL REFRESCO DEL WIDGET EN iOS (BGAppRefreshTask): con su interruptor y el de los
// widgets, `fetch` y `<App ID>.refresco` en cada app con widget; con cualquiera de los dos
// apagado, o sin widget, nada. Y plist_apple lo aplica (y lo borra antes, para que
// apagarlo lo quite de un plist incremental). El despertar va apagado aquí: tiene su
// bloque abajo, y encenderlo en Sytue.xcconfig no debe romper este.
func TestEmpaquetarRefrescoDelWidgetEnIOS(t *testing.T) {
	d := empaquetarDirs(t)
	refresco := func(env, app string) string {
		t.Helper()
		return empaquetarCon(t, strings.Fields(env), "refresco-ios", app).Texto()
	}
	for _, a := range []string{"calendar", "mail", "hub"} {
		id := empaquetarID(t, empaquetarUnir(d.fyne, "cmd", a, "FyneApp.toml"))
		quiero := empaquetarOrdenes("Add :UIBackgroundModes array", "Add :UIBackgroundModes:0 string fetch",
			"Add :BGTaskSchedulerPermittedIdentifiers array", "Add :BGTaskSchedulerPermittedIdentifiers:0 string "+id+".refresco")
		if refresco("SYTUE_DESPERTAR_APPLE=0 SYTUE_WIDGETS_APPLE=1 SYTUE_REFRESCO_APPLE=1", a) != quiero {
			t.Errorf("con el refresco y los widgets de Apple, %s no pide el refresco en segundo plano (%s.refresco)", a, id)
		}
		if refresco("SYTUE_DESPERTAR_APPLE=0 SYTUE_WIDGETS_APPLE=0 SYTUE_REFRESCO_APPLE=1", a) != "" {
			t.Errorf("con los widgets de Apple apagados, %s pide el refresco en segundo plano", a)
		}
		if refresco("SYTUE_DESPERTAR_APPLE=0 SYTUE_WIDGETS_APPLE=1 SYTUE_REFRESCO_APPLE=0", a) != "" {
			t.Errorf("con el refresco apagado, %s pide el refresco en segundo plano", a)
		}
	}
	if refresco("SYTUE_DESPERTAR_APPLE=0 SYTUE_WIDGETS_APPLE=1 SYTUE_REFRESCO_APPLE=1", "demo") != "" {
		t.Error("una app sin widget pide el refresco en segundo plano")
	}
	if r := empaquetarCon(t, []string{"SYTUE_DESPERTAR_APPLE=0", "SYTUE_WIDGETS_APPLE=1", "SYTUE_REFRESCO_APPLE=quizá"}, "refresco-ios", "calendar"); r.Codigo == 0 {
		t.Error("SYTUE_REFRESCO_APPLE acepta un valor que no es YES/NO")
	}
	// Lo versionado manda también aquí, y el interruptor no se enciende antes que el fork: con
	// el tag sin el parche 0016 nadie registra la tarea, y las quince apps pedirían a App
	// Review un modo de fondo sin usar. Visto en rojo con SYTUE_REFRESCO_APPLE = YES.
	xc := guiontest.Leer(t, empaquetarUnir(d.fyne, "xcode/Sytue.xcconfig"))
	valor := strings.Join(empaquetarSed(xc, `^SYTUE_REFRESCO_APPLE = ([A-Z]*)$`), "\n")
	if valor != "YES" && valor != "NO" {
		t.Errorf("Sytue.xcconfig no declara SYTUE_REFRESCO_APPLE (YES/NO) una vez: «%s»", valor)
	}
	if empaquetarHay(guiontest.Leer(t, empaquetarUnir(d.fyne, "go.mod")), `^replace fyne\.io/fyne/v2 => github\.com/thinkddr/fyne/v2 v2\.8\.2-sytue\.[0-3]$`) {
		if valor != "NO" {
			t.Error("SYTUE_REFRESCO_APPLE encendido con un fork sin el parche 0016: nadie registraría la tarea")
		}
		if empaquetarSinVars(t, []string{"SYTUE_REFRESCO_APPLE"}, []string{"SYTUE_DESPERTAR_APPLE=0", "SYTUE_WIDGETS_APPLE=1"}, "refresco-ios", "calendar").Texto() != "" {
			t.Error("con el fork sin el parche 0016, el Info.plist de iOS pide el refresco en segundo plano")
		}
	}
}

// EL DESPERTAR POR AVISO SILENCIOSO (APNs): con su interruptor y el de los widgets,
// `remote-notification` detrás de `fetch` si también hay refresco, o solo; nada sin widget.
func TestEmpaquetarDespertarPorAvisoSilencioso(t *testing.T) {
	d := empaquetarDirs(t)
	refresco := func(env, app string) string {
		t.Helper()
		return empaquetarCon(t, strings.Fields(env), "refresco-ios", app).Texto()
	}
	for _, a := range []string{"calendar", "mail", "hub"} {
		id := empaquetarID(t, empaquetarUnir(d.fyne, "cmd", a, "FyneApp.toml"))
		if refresco("SYTUE_WIDGETS_APPLE=1 SYTUE_REFRESCO_APPLE=0 SYTUE_DESPERTAR_APPLE=1", a) != empaquetarOrdenes("Add :UIBackgroundModes array", "Add :UIBackgroundModes:0 string remote-notification") {
			t.Errorf("con el despertar y los widgets de Apple, %s no pide remote-notification", a)
		}
		quiero := empaquetarOrdenes("Add :UIBackgroundModes array", "Add :UIBackgroundModes:0 string fetch", "Add :UIBackgroundModes:1 string remote-notification",
			"Add :BGTaskSchedulerPermittedIdentifiers array", "Add :BGTaskSchedulerPermittedIdentifiers:0 string "+id+".refresco")
		if refresco("SYTUE_WIDGETS_APPLE=1 SYTUE_REFRESCO_APPLE=1 SYTUE_DESPERTAR_APPLE=1", a) != quiero {
			t.Errorf("con el refresco y el despertar, %s no pide los dos modos de fondo", a)
		}
		if refresco("SYTUE_WIDGETS_APPLE=0 SYTUE_REFRESCO_APPLE=0 SYTUE_DESPERTAR_APPLE=1", a) != "" {
			t.Errorf("con los widgets de Apple apagados, %s pide el despertar", a)
		}
	}
	if refresco("SYTUE_WIDGETS_APPLE=1 SYTUE_DESPERTAR_APPLE=1", "demo") != "" {
		t.Error("una app sin widget pide el despertar")
	}
	if r := empaquetarCon(t, []string{"SYTUE_WIDGETS_APPLE=1", "SYTUE_DESPERTAR_APPLE=quizá"}, "refresco-ios", "calendar"); r.Codigo == 0 {
		t.Error("SYTUE_DESPERTAR_APPLE acepta un valor que no es YES/NO")
	}
	// Y no se enciende antes que el fork: con el tag sin el parche 0017 nadie pide el token ni
	// atiende el aviso, y App Review vería un modo de fondo sin usar. Visto en rojo con
	// SYTUE_DESPERTAR_APPLE = YES.
	valor := strings.Join(empaquetarSed(guiontest.Leer(t, empaquetarUnir(d.fyne, "xcode/Sytue.xcconfig")), `^SYTUE_DESPERTAR_APPLE = ([A-Z]*)$`), "\n")
	if valor != "YES" && valor != "NO" {
		t.Errorf("Sytue.xcconfig no declara SYTUE_DESPERTAR_APPLE (YES/NO) una vez: «%s»", valor)
	}
	if empaquetarHay(guiontest.Leer(t, empaquetarUnir(d.fyne, "go.mod")), `^replace fyne\.io/fyne/v2 => github\.com/thinkddr/fyne/v2 v2\.8\.2-sytue\.[0-4]$`) {
		if valor != "NO" {
			t.Error("SYTUE_DESPERTAR_APPLE encendido con un fork sin el parche 0017: nadie atendería el aviso")
		}
		if strings.Contains(empaquetarSinVars(t, []string{"SYTUE_DESPERTAR_APPLE"}, []string{"SYTUE_WIDGETS_APPLE=1"}, "refresco-ios", "calendar").Texto(), "remote-notification") {
			t.Error("con el fork sin el parche 0017, el Info.plist de iOS pide remote-notification")
		}
	}
}

// plist_apple aplica (y borra antes) el refresco, y UIBackgroundModes solo lo escribe
// plist_refresco: plist_apple borra el array entero, y un modo que pusiera otro sitio (la
// plantilla de Xcode, otra función) se perdería. Tres apariciones: las dos órdenes de
// plist_refresco y el Delete. Visto en rojo añadiendo un
// `Add :UIBackgroundModes:1 string remote-notification` en plist_apple.
func TestEmpaquetarUIBackgroundModesSoloEnPlistRefresco(t *testing.T) {
	d := empaquetarDirs(t)
	guion := guiontest.Leer(t, d.guion)
	if !strings.Contains(guion, `refresco="$(plist_refresco "$dir")"`) || !strings.Contains(guion, `PlistBuddy -c "Delete :BGTaskSchedulerPermittedIdentifiers"`) {
		t.Error("plist_apple no aplica (ni borra antes) el refresco del widget")
	}
	// awk '/^plist_refresco\(\)/,/^}/'
	var funcion strings.Builder
	dentro := false
	for l := range strings.SplitSeq(guion, "\n") {
		if !dentro && strings.HasPrefix(l, "plist_refresco()") {
			dentro = true
			funcion.WriteString(l + "\n")
			continue
		}
		if dentro {
			funcion.WriteString(l + "\n")
			if strings.HasPrefix(l, "}") {
				dentro = false
			}
		}
	}
	fuera := false
	for _, dir := range []string{empaquetarUnir(d.fyne, "xcode"), empaquetarUnir(d.fyne, "cmd")} {
		_ = filepath.WalkDir(dir, func(ruta string, e fs.DirEntry, err error) error {
			if err == nil && e.Type().IsRegular() && strings.Contains(empaquetarLeer(ruta), "UIBackgroundModes") {
				fuera = true
			}
			return nil
		})
	}
	if strings.Count(guion, ":UIBackgroundModes") != 3 || strings.Count(funcion.String(), ":UIBackgroundModes") != 2 || fuera {
		t.Error("UIBackgroundModes se escribe fuera de plist_refresco: el Delete de plist_apple lo borraría")
	}
	if r := empaquetar(t, "derechos-sesion", "hub", "ABCDE12345", "ios"); r.Codigo == 0 {
		t.Error("la sesión acepta otro equipo Apple")
	}
	if r := empaquetar(t, "derechos-sesion", "hub", empaquetarEquipo, "desconocido"); r.Codigo == 0 {
		t.Error("la sesión acepta una plataforma desconocida")
	}
}

// LA EXTENSIÓN FILE PROVIDER (Drive en el Finder y en Archivos): solo la app que declara
// `archivos` lleva el segundo grupo de llavero, y ese grupo es el MISMO en la firma de la
// app, en la de la extensión y en el código que guarda la credencial. Visto en rojo dándole
// el grupo a todas, cambiando la constante de archivos.go y dándole a la extensión del Mac
// network.server.
func TestEmpaquetarExtensionFileProvider(t *testing.T) {
	d := empaquetarDirs(t)
	grupo := strings.Join(empaquetarSed(guiontest.Leer(t, empaquetarUnir(d.raiz, "apps/nucleo/sistema/archivos.go")), `^[[:space:]]*GrupoLlaveroArchivos = "(.*)"$`), "\n")
	for _, app := range empaquetarApps(t) {
		if app == "demo" {
			continue
		}
		plist := empaquetarJunto(empaquetar(t, "derechos-tienda", app, empaquetarEquipo))
		if empaquetarHay(empaquetarLeer(empaquetarUnir(d.fyne, "cmd", app, "FyneApp.toml")), `^[[:space:]]*archivos[[:space:]]*=[[:space:]]*"si"`) {
			if !strings.Contains(plist, "<string>"+grupo+"</string>") {
				t.Errorf("%s lleva la extensión de archivos y su firma no comparte el llavero «%s»", app, grupo)
			}
		} else if strings.Contains(plist, ".shared<") {
			t.Errorf("%s no lleva la extensión de archivos y su firma comparte un llavero con ella", app)
		}
	}
	if grupo == "" {
		t.Error("apps/nucleo/sistema/archivos.go ya no declara GrupoLlaveroArchivos")
	}
	quiero := map[string]string{
		"macos":   "com.apple.application-identifier com.apple.developer.team-identifier com.apple.security.app-sandbox com.apple.security.application-groups com.apple.security.network.client keychain-access-groups ",
		"ios":     "application-identifier com.apple.developer.default-data-protection com.apple.developer.team-identifier com.apple.security.application-groups keychain-access-groups ",
		"app-ios": "keychain-access-groups ",
	}
	for _, p := range []string{"macos", "ios", "app-ios"} {
		r := empaquetar(t, "derechos-archivos", "drive", empaquetarEquipo, p)
		plist := empaquetarJunto(r)
		if r.Codigo != 0 {
			t.Errorf("derechos-archivos drive %s falla: %s", p, plist)
			continue
		}
		if !strings.Contains(plist, "<string>"+grupo+"</string>") {
			t.Errorf("los derechos de archivos (%s) no llevan el llavero «%s»", p, grupo)
		}
		if tiene := empaquetarClaves(plist); tiene != quiero[p] {
			t.Errorf("la extensión de archivos (%s) se firmaría con «%s» y lo decidido es «%s»", p, tiene, quiero[p])
		}
	}
	// En iOS el PRIMER grupo de la app es su App ID: es donde guarda la sesión sin grupo.
	if !strings.Contains(empaquetar(t, "derechos-archivos", "drive", empaquetarEquipo, "app-ios").Texto(), "<array><string>"+empaquetarEquipo+".eu.sytue.drive</string>") {
		t.Error("en iOS la sesión de Drive cambiaría de grupo de llavero")
	}
	if r := empaquetar(t, "derechos-archivos", "hub", empaquetarEquipo, "ios"); r.Codigo == 0 {
		t.Error("derechos-archivos firma una extensión para una app que no la declara")
	}
}

// Un .pkg por app (no por bundle), y el .dmg de fuera de la tienda sigue siendo el suyo; y lo
// que solo se puede leer en el guion sin un Mac: quién firma qué, y con qué etiqueta.
func TestEmpaquetarMacAppStoreFirmaYPublica(t *testing.T) {
	quiero := make([]string, 0, len(empaquetarApps15))
	for _, a := range empaquetarApps15 {
		quiero = append(quiero, "sytue-"+a+"-macos-tienda.pkg")
	}
	if empaquetarJunto(empaquetar(t, "publicables", "macappstore")) != strings.Join(quiero, "\n") {
		t.Error("publicables macappstore no da un .pkg por cada una de las quince apps")
	}
	if got := empaquetarJunto(empaquetar(t, "publicables", "macos")); got != "sytue-drive-macos-universal.dmg\nsytue-one-macos-universal.dmg" {
		t.Errorf("la variante de tienda ha cambiado lo que se publica como .dmg: %s", empaquetarUnaLinea(got))
	}
	guion := guiontest.Leer(t, empaquetarDirs(t).guion)
	tiene := func(s ...string) bool {
		for _, x := range s {
			if !strings.Contains(guion, x) {
				return false
			}
		}
		return true
	}
	if !tiene(`productbuild --component "$ESCENARIO/tienda/$app/$nombre.app" /Applications`, `--sign "$SYTUE_MAS_INSTALADOR" "$destino"`) {
		t.Error("el .pkg de la Mac App Store no se firma con el certificado de instalador (SYTUE_MAS_INSTALADOR)")
	}
	if !tiene(`grep -qF "$SYTUE_MAS_INSTALADOR" <<<"$firma_pkg"`) {
		t.Error("no se comprueba quién firma el .pkg")
	}
	if !tiene(`codesign -f -s "$SYTUE_MAS_IDENTIDAD" --options runtime --timestamp --entitlements "$ent" "$app"`) {
		t.Error("el .app de la Mac App Store no se firma con el certificado de distribución y sus derechos")
	}
	if !tiene(`install -m 0644 "$perfil" "$app/Contents/embedded.provisionprofile"`) {
		t.Error("el .app de la Mac App Store no lleva el perfil embebido públicamente legible")
	}
	if !tiene(`compilar_mac "$exe" ${etiquetas[@]+"${etiquetas[@]}"}`, `[ -z "$tienda" ] || etiquetas=(--tags "tienda,goolm")`,
		`GOARCH=arm64 compilar_fyne "$salida.arm64" "$@"`, `GOARCH=amd64 compilar_fyne "$salida.x86_64" "$@"`, `compilar_mac "$salida" --tags tienda,goolm`) {
		t.Error("las dos mitades del universal de tienda (macappstore y la fase de Xcode Cloud) no se compilan con la etiqueta «tienda»")
	}
	// El .dmg no se ve afectado: Developer ID solo fuera de la tienda, y la tienda sin bundles.
	if !tiene(`elif [ "$plataforma" = macos ] && [ -n "${SYTUE_MACOS_IDENTIDAD:-}" ]; then`) {
		t.Error("la firma de Developer ID y la de la tienda ya no se excluyen: una app llevaría las dos")
	}
	if !tiene(`[ -z "$tienda" ] || break`) {
		t.Error("la variante de tienda armaría además los .dmg de los bundles")
	}
}

// SIN ACTUALIZADOR PROPIO en la variante de tienda (la Mac App Store lo prohíbe): el
// montaje del DMG ni se compila, y no hay plataforma que buscar en el catálogo.
func TestEmpaquetarTiendaSinActualizadorPropio(t *testing.T) {
	d := empaquetarDirs(t)
	golist := func(tags ...string) string {
		t.Helper()
		args := append([]string{"list"}, tags...)
		args = append(args, "-f", "{{.GoFiles}} {{.CgoFiles}}", "./sistema/escritorio", "./auth")
		return guiontest.Ejecutar(t, guiontest.Opciones{
			Dir: empaquetarUnir(d.raiz, "apps/nucleo"),
			Env: []string{"GOFLAGS=-mod=mod", "GOOS=darwin", "GOARCH=arm64", "CGO_ENABLED=1"},
		}, "go", args...).Todo()
	}
	tienda := golist("-tags", "tienda")
	if strings.Contains(tienda, "actualizar_darwin.go") {
		t.Error("la variante de tienda compila el actualizador de macOS (actualizar_darwin.go)")
	}
	if !regexp.MustCompile(`(?s)actualizar_otros\.go.*tienda\.go.*store_apple\.go`).MatchString(tienda) {
		t.Errorf("la variante de tienda no compila lo que debe (sin actualizador, DeTienda, llavero de Apple): %s", tienda)
	}
	if strings.Contains(tienda, " store.go") || strings.Contains(tienda, "[store.go") {
		t.Error("la variante de tienda guardaría la sesión con /usr/bin/security (store.go), que el sandbox no deja compartir")
	}
	normal := golist()
	if !regexp.MustCompile(`(?s)actualizar_darwin\.go.*tienda_no\.go.*store\.go`).MatchString(normal) {
		t.Errorf("el macOS de fuera de la tienda ha perdido su actualizador o su llavero: %s", normal)
	}
	if !strings.Contains(empaquetarDespues(guiontest.Leer(t, empaquetarUnir(d.raiz, "apps/nucleo/sistema/escritorio/actualizar.go")), "^func Plataforma\\(\\) string \\{", 3), "if DeTienda {") {
		t.Error("escritorio.Plataforma ya no es vacía en la variante de tienda: el Hub buscaría actualizaciones propias")
	}
	if !strings.Contains(empaquetarDespues(guiontest.Leer(t, empaquetarUnir(d.fyne, "cmd/hub/main.go")), `if !escritorio\.DeTienda \{`, 3), "ElementoActualizar") {
		t.Error("el Hub de la Mac App Store enseñaría «Buscar actualizaciones» en su menú")
	}
}

// empaquetarDespues es `grep -A n patrón`: cada línea que casa y las n siguientes.
func empaquetarDespues(texto, patron string, n int) string {
	re := regexp.MustCompile(patron)
	l := strings.Split(texto, "\n")
	var b strings.Builder
	for i := range l {
		if re.MatchString(l[i]) {
			for j := i; j <= i+n && j < len(l); j++ {
				b.WriteString(l[j] + "\n")
			}
		}
	}
	return b.String()
}
