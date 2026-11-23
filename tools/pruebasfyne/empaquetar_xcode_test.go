// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package pruebasfyne

import (
	"maps"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/tools/guiontest"
)

const (
	empaquetarUso    = "ITSAppUsesNonExemptEncryption"
	empaquetarCodigo = "ITSEncryptionExportComplianceCode"
)

// La declaración autorizada vive en los Info.plist canónicos. La ruta manual la
// copia al artefacto, antes de firmarlo, sin cambiar sus otros metadatos o derechos.
func TestEmpaquetarElCifradoCanonicoLlegaAlPlistDeTienda(t *testing.T) {
	d := empaquetarDirs(t)
	texto := guiontest.Leer(t, d.guion)
	llamada := `copiar_cifrado_apple "$FYNE/xcode/Info-$plataforma.plist" "$plist"`
	i := strings.Index(texto, llamada)
	ipa := strings.Index(texto, `[ -z "$ipa_abierto" ] || firmar_ipa`)
	tienda := strings.Index(texto, `firmar_tienda "$destino" "$dir" "$appid"`)
	if i < 0 || ipa < 0 || tienda < 0 || i >= ipa || i >= tienda {
		t.Error("el plist de tienda no conserva la declaración canónica de cifrado: la copia no va antes de firmar")
	}
	if !strings.Contains(texto, "if [ \"$plataforma\" = ios ] || [ -n \"$tienda\" ]; then\n        "+llamada) {
		t.Error("el plist de tienda no conserva la declaración canónica de cifrado: la copia no es para iOS y la tienda")
	}
	tmp := t.TempDir()
	cifrado := func(fuente, destino string) guiontest.Resultado {
		t.Helper()
		return guiontest.Ejecutar(t, guiontest.Opciones{}, "bash", d.guion, "cifrado-apple", fuente, destino)
	}
	for _, p := range []string{"ios", "macos"} {
		fuente := empaquetarUnir(d.fyne, "xcode", "Info-"+p+".plist")
		destino := empaquetarUnir(tmp, "artefacto-"+p+".plist")
		canonico, err := empaquetarPlist(t, guiontest.Leer(t, fuente))
		if err != nil {
			t.Fatal(err)
		}
		for _, binario := range []bool{false, true} {
			previo := map[string]any{
				"CFBundleIdentifier": "eu.sytue.rooms", "CFBundleVersion": "42",
				"CFBundleURLTypes": []any{map[string]any{"CFBundleURLSchemes": []any{"sytue-rooms"}}},
				empaquetarUso:      true, empaquetarCodigo: "obsoleto-no-enviar",
			}
			empaquetarEscribirPlist(t, destino, previo, binario)
			if r := cifrado(fuente, destino); r.Codigo != 0 {
				t.Errorf("el plist de tienda no conserva la declaración canónica de cifrado: %s", r.Error)
				continue
			}
			esperado := maps.Clone(previo)
			delete(esperado, empaquetarUso)
			delete(esperado, empaquetarCodigo)
			for _, k := range []string{empaquetarUso, empaquetarCodigo} {
				if v, ok := canonico[k]; ok {
					esperado[k] = v
				}
			}
			datos := guiontest.Leer(t, destino)
			if got, err := empaquetarPlist(t, datos); err != nil || !empaquetarIgualJSONMapa(got, esperado) {
				t.Errorf("el plist de tienda no conserva la declaración canónica de cifrado (%s, binario=%v): %v", p, binario, got)
			}
			if strings.HasPrefix(datos, "bplist00") != binario {
				t.Errorf("el plist de tienda cambia de formato (%s, binario=%v)", p, binario)
			}
		}
		// Un código autorizado futuro también se copia: no se vuelve a decidir el cifrado aquí.
		fuente = empaquetarUnir(tmp, "canonico-"+p+".plist")
		empaquetarEscribirPlist(t, fuente, map[string]any{empaquetarUso: true, empaquetarCodigo: "codigo-de-prueba-no-enviar"}, false)
		if r := cifrado(fuente, destino); r.Codigo != 0 {
			t.Errorf("el plist de tienda no copia un código autorizado: %s", r.Error)
		}
		if info, err := empaquetarPlist(t, guiontest.Leer(t, destino)); err != nil || info[empaquetarUso] != true || info[empaquetarCodigo] != "codigo-de-prueba-no-enviar" {
			t.Errorf("el plist de tienda no copia un código autorizado: %v", info)
		}
		for _, roto := range []map[string]any{{}, {empaquetarUso: true}, {empaquetarUso: "false"}, {empaquetarUso: false, empaquetarCodigo: "obsoleto"}, {empaquetarUso: true, empaquetarCodigo: "[]"}} {
			empaquetarEscribirPlist(t, fuente, roto, false)
			original := guiontest.Leer(t, destino)
			if r := cifrado(fuente, destino); r.Codigo == 0 || guiontest.Leer(t, destino) != original {
				t.Errorf("el plist de tienda acepta una declaración rota o toca el artefacto: %v", roto)
			}
		}
	}
}

// empaquetarIgualJSONMapa compara dos diccionarios de plist (sin orden de claves).
func empaquetarIgualJSONMapa(a, b map[string]any) bool { return empaquetarMismoJSON(a, b) }

// --- 14. Xcode Cloud: el proyecto versionado es el que sale del árbol.
// Xcode Cloud solo construye un proyecto de Xcode que esté en el repositorio, así que
// apps/fyne/xcode/Sytue.xcodeproj se genera (generar.py) y se versiona. Aquí: que sea el
// de las apps de cmd/ con sus derechos de tienda (una app nueva o un derecho cambiado sin
// regenerar es rojo), que ci_post_clone.sh sea ejecutable (si no, Xcode Cloud lo corre con
// zsh) y que los Info.plist omitan el par de exportación mientras no haya código aprobado. Visto en
// rojo editando calcus/tienda.entitlements, borrando la carpeta de una app, añadiendo una
// carpeta sobrante y quitando el bit de ejecución.
func TestEmpaquetarProyectoDeXcodeVersionado(t *testing.T) {
	d := empaquetarDirs(t)
	if r := guiontest.Ejecutar(t, guiontest.Opciones{}, "python3", empaquetarUnir(d.fyne, "xcode/generar.py"), "--comprobar"); r.Codigo != 0 {
		t.Errorf("Sytue.xcodeproj no es el que genera generar.py: %s", empaquetarJunto(r))
	}
	r := guiontest.Ejecutar(t, guiontest.Opciones{}, "git", "-C", d.raiz, "ls-files", "-s", "apps/fyne/xcode/ci_scripts/ci_post_clone.sh")
	if !strings.HasPrefix(r.Salida, "100755") {
		t.Error("apps/fyne/xcode/ci_scripts/ci_post_clone.sh no está versionado como ejecutable: Xcode Cloud lo correría con zsh")
	}
}

// empaquetarGit ejecuta git en un repositorio de prueba y para si falla.
func empaquetarGit(t *testing.T, args ...string) string {
	t.Helper()
	r := guiontest.Ejecutar(t, guiontest.Opciones{}, "git", args...)
	if r.Codigo != 0 {
		t.Fatalf("git %v: %s", args, r.Todo())
	}
	return r.Texto()
}

// Una rama de Xcode Cloud llega shallow y sin tags. La release alcanzable debe ser
// 3.12.2: hay también una RC en HEAD y una versión mayor en OTRA rama. No basta con
// escoger el tag más reciente de origin, ni con escribir 0.0.0 cuando describe falla.
func TestEmpaquetarVersionDeXcodeCloud(t *testing.T) {
	d := empaquetarDirs(t)
	fix := empaquetarUnir(t.TempDir(), "xcode-version")
	fuente := empaquetarUnir(fix, "fuente")
	empaquetarGit(t, "init", "-q", "--bare", empaquetarUnir(fix, "remoto.git"))
	empaquetarGit(t, "init", "-q", "-b", "main", fuente)
	g := func(args ...string) string {
		t.Helper()
		return empaquetarGit(t, append([]string{"-C", fuente}, args...)...)
	}
	g("config", "user.name", "Prueba Xcode Cloud")
	g("config", "user.email", "prueba@sytue.invalid")
	archivo := empaquetarUnir(fuente, "archivo")
	guiontest.Escribir(t, archivo, "release\n")
	g("add", "archivo")
	g("commit", "-qm", "release")
	g("tag", "-a", "v3.12.2", "-m", "release")
	for paso := 1; paso <= 5; paso++ {
		guiontest.Escribir(t, archivo, empaquetarLeer(archivo)+string(rune('0'+paso))+"\n")
		g("commit", "-qam", "rama")
	}
	g("tag", "v3.13.0-rc.1")
	sha := g("rev-parse", "HEAD")
	g("checkout", "-qb", "ajena", "v3.12.2")
	guiontest.Escribir(t, archivo, empaquetarLeer(archivo)+"ajena\n")
	g("commit", "-qam", "ajena")
	g("tag", "v99.0.0")
	g("remote", "add", "origin", empaquetarUnir(fix, "remoto.git"))
	g("push", "-q", "origin", "main", "ajena", "--tags")
	rama := empaquetarUnir(fix, "rama")
	empaquetarGit(t, "clone", "-q", "--depth=1", "--no-tags", "--branch", "main", "file://"+empaquetarUnir(fix, "remoto.git"), rama)
	if err := os.MkdirAll(empaquetarUnir(rama, "apps/fyne/xcode"), 0o750); err != nil {
		t.Fatal(err)
	}
	plist := empaquetarUnir(rama, "apps/fyne/xcode/version.xcconfig")
	probar := func(tag string, version ...string) guiontest.Resultado {
		t.Helper()
		v := ""
		if len(version) > 0 {
			v = version[0]
		}
		return guiontest.Ejecutar(t, guiontest.Opciones{Env: []string{
			"CI_PRIMARY_REPOSITORY_PATH=" + rama, "CI_XCODE_SCHEME=calendar", "CI_BUILD_NUMBER=17",
			"CI_PRODUCT_PLATFORM=iOS", "CI_TAG=" + tag, "SYTUE_APP_VERSION=" + v,
		}},
			"bash", empaquetarUnir(d.fyne, "xcode/ci_scripts/ci_post_clone.sh"), "--solo-version")
	}
	if r := probar(""); r.Codigo != 0 {
		t.Errorf("Xcode Cloud no resuelve la release de una rama shallow: %s", r.Todo())
	}
	if !empaquetarHayLinea(empaquetarLeer(plist), "MARKETING_VERSION = 3.12.2") || !empaquetarHayLinea(empaquetarLeer(plist), "CURRENT_PROJECT_VERSION = 17") {
		t.Error("la rama shallow no conserva la versión estable alcanzable y su número de build")
	}
	if empaquetarGit(t, "-C", rama, "rev-parse", "HEAD") != sha {
		t.Error("resolver la versión de Xcode Cloud cambió el commit que se compila")
	}
	empaquetarGit(t, "-C", rama, "tag", "-d", "v3.12.2")
	empaquetarGit(t, "-C", rama, "remote", "set-url", "origin", "file://"+empaquetarUnir(fix, "no-existe.git"))
	if r := probar("v3.12.3"); r.Codigo != 0 || !empaquetarHayLinea(empaquetarLeer(plist), "MARKETING_VERSION = 3.12.3") {
		t.Error("un build de tag necesita fetch o no usa CI_TAG")
	}
	// Los flujos de Xcode Cloud arrancan con tienda-vX.Y.Z: la versión es la misma.
	if r := probar("tienda-v3.12.4"); r.Codigo != 0 || !empaquetarHayLinea(empaquetarLeer(plist), "MARKETING_VERSION = 3.12.4") {
		t.Errorf("un build de tienda-vX.Y.Z no saca su versión del tag: %s", r.Todo())
	}
	_ = os.Remove(plist)
	if r := probar(""); r.Codigo == 0 {
		t.Error("Xcode Cloud acepta una rama sin versión ni acceso a los tags")
	}
	if empaquetarExiste(plist) {
		t.Error("Xcode Cloud escribió una versión antes de resolverla")
	}
	if r := probar("", "3.12.2"); r.Codigo != 0 || !empaquetarHayLinea(empaquetarLeer(plist), "MARKETING_VERSION = 3.12.2") {
		t.Error("Xcode Cloud no admite la versión explícita cuando Apple no permite fetch")
	}
	if probar("v3.13.0-rc.1").Codigo == 0 || probar("tienda-v3.13.0-rc.1").Codigo == 0 || probar("", "0.0.0").Codigo == 0 {
		t.Error("Xcode Cloud admite una RC o 0.0.0 como versión de App Store Connect")
	}
}

// Regresión real de Apple (90592): TRUE sin código archiva, pero no se puede subir.
// Se prueba EL post-clone contra plists temporales; ningún código de prueba llega a Apple.
func TestEmpaquetarExportacionEnElPostClone(t *testing.T) {
	d := empaquetarDirs(t)
	xcode := empaquetarUnir(d.fyne, "xcode")
	guion := empaquetarUnir(xcode, "ci_scripts/ci_post_clone.sh")
	// La lista de países excluidos la da appstore-publicar.py, ejecutado de verdad.
	r := guiontest.Ejecutar(t, guiontest.Opciones{}, "python3", "-c", `import json, runpy, sys
sys.path.insert(0, sys.argv[1])
print(json.dumps(sorted(runpy.run_path(sys.argv[2])["EXCLUIDOS"])))`, empaquetarUnir(d.fyne, "scripts"), empaquetarUnir(d.fyne, "scripts/appstore-publicar.py"))
	if r.Codigo != 0 || !strings.Contains(r.Salida, `"FRA"`) {
		t.Fatalf("el post-clone no protege el par de exportación de Apple: revisar la exención documental antes de habilitar Francia (%s)", r.Todo())
	}
	fuentes := []string{empaquetarUnir(xcode, "Info-ios.plist"), empaquetarUnir(xcode, "Info-macos.plist")}
	for _, f := range fuentes {
		info, err := empaquetarPlist(t, guiontest.Leer(t, f))
		if _, hay := info[empaquetarCodigo]; err != nil || info[empaquetarUso] != false || hay {
			t.Fatalf("el post-clone no protege el par de exportación de Apple: %s: cifrado estándar sin Francia debe declarar FALSE sin código", f)
		}
	}
	carpeta := t.TempDir()
	destino := empaquetarUnir(carpeta, "apps/fyne/xcode")
	if err := os.MkdirAll(destino, 0o750); err != nil {
		t.Fatal(err)
	}
	codigo := ""
	restaurar := func() {
		t.Helper()
		for _, f := range fuentes {
			guiontest.Escribir(t, empaquetarUnir(destino, filepath.Base(f)), guiontest.Leer(t, f))
		}
	}
	ejecutar := func() guiontest.Resultado {
		t.Helper()
		return guiontest.Ejecutar(t, guiontest.Opciones{Env: []string{
			"CI_PRIMARY_REPOSITORY_PATH=" + carpeta, "CI_XCODE_SCHEME=calendar",
			"SYTUE_APP_ENCRYPTION_EXPORT_CODE=" + codigo,
		}}, "bash", guion, "--solo-cifrado")
	}
	restaurar()
	r = ejecutar()
	for _, f := range fuentes {
		if r.Codigo != 0 || guiontest.Leer(t, empaquetarUnir(destino, filepath.Base(f))) != guiontest.Leer(t, f) {
			t.Errorf("sin código el post-clone no conserva la exención documental: %s", r.Error)
		}
	}
	// Verlo en rojo en LAS DOS plataformas: true sin código, tipos o pares inválidos.
	for _, f := range fuentes {
		ruta := empaquetarUnir(destino, filepath.Base(f))
		for _, cambios := range []map[string]any{
			{empaquetarUso: true},
			{empaquetarUso: true, empaquetarCodigo: ""},
			{empaquetarUso: true, empaquetarCodigo: "[]"},
			{empaquetarCodigo: ""},
			{empaquetarCodigo: "solo-prueba-no-enviar"},
			{empaquetarUso: "false"},
			{empaquetarUso: 0},
		} {
			restaurar()
			info, _ := empaquetarPlist(t, guiontest.Leer(t, ruta))
			maps.Copy(info, cambios)
			empaquetarEscribirPlist(t, ruta, info, false)
			if r := ejecutar(); r.Codigo == 0 || !strings.Contains(r.Error, "par TRUE + código no vacío") {
				t.Errorf("el post-clone no rechaza el par de exportación roto en %s: %v", filepath.Base(f), cambios)
			}
		}
		restaurar()
		info, _ := empaquetarPlist(t, guiontest.Leer(t, ruta))
		delete(info, empaquetarUso)
		empaquetarEscribirPlist(t, ruta, info, false)
		if r := ejecutar(); r.Codigo == 0 {
			t.Errorf("el post-clone acepta metadatos de exportación ausentes en %s", filepath.Base(f))
		}
	}
	restaurar()
	codigo = " "
	if r := ejecutar(); r.Codigo == 0 || !strings.Contains(r.Error, "SYTUE_APP_ENCRYPTION_EXPORT_CODE no contiene un código") {
		t.Error("el post-clone acepta un código vacío con espacios")
	}
	// Solo formato y propagación en una carpeta temporal; no acredita aprobación.
	restaurar()
	codigo = "solo-prueba-no-enviar"
	if r := ejecutar(); r.Codigo != 0 {
		t.Fatalf("el post-clone no admite la ruta con código: %s", r.Error)
	}
	for _, f := range fuentes {
		original, _ := empaquetarPlist(t, guiontest.Leer(t, f))
		generado, _ := empaquetarPlist(t, guiontest.Leer(t, empaquetarUnir(destino, filepath.Base(f))))
		esperado := maps.Clone(original)
		esperado[empaquetarUso] = true
		esperado[empaquetarCodigo] = codigo
		if !empaquetarIgualJSONMapa(generado, esperado) {
			t.Errorf("el post-clone altera datos ajenos a la declaración en %s", filepath.Base(f))
		}
	}
}
