// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package pruebasfyne

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/tools/guiontest"
)

// empaquetarApps15 son las quince apps que se publican, en orden.
var empaquetarApps15 = []string{"bases", "calcus", "calendar", "cast", "certipdf", "contacts", "diapos", "docus", "drive", "erp", "hub", "mail", "notes", "photos", "rooms"}

// --- 9. Android para Google Play (Play App Signing).
// Un AAB por app, con el versionCode del tag, firmado SOLO con la clave de subida y
// sin que vuelvan los secretos del keystore antiguo, que no existen en git.gay: el job
// los pedía y por eso no firmó nunca. Visto en rojo quitando una app de la carpeta,
// rompiendo la fórmula y devolviendo ANDROID_KEYSTORE_BASE64 a release-mobile.yml.
func TestEmpaquetarAndroidUnAABPorApp(t *testing.T) {
	quiero := make([]string, 0, len(empaquetarApps15))
	for _, a := range empaquetarApps15 {
		quiero = append(quiero, "sytue-"+a+"-android.aab")
	}
	if got := empaquetarJunto(empaquetar(t, "publicables", "android")); got != strings.Join(quiero, "\n") {
		t.Errorf("empaquetar.sh publicables android no da un AAB por cada una de las quince apps: %s", empaquetarUnaLinea(got))
	}
	aab := t.TempDir()
	for _, f := range quiero {
		guiontest.Escribir(t, empaquetarUnir(aab, f), "")
	}
	if r := empaquetar(t, "publicables", "android", aab); r.Codigo != 0 {
		t.Error("publicables android no da por completos los quince AAB")
	}
	if err := os.Remove(empaquetarUnir(aab, "sytue-rooms-android.aab")); err != nil {
		t.Fatal(err)
	}
	r := empaquetar(t, "publicables", "android", aab)
	switch {
	case r.Codigo == 0:
		t.Error("publicables android da por completa una release sin el AAB de Rooms")
	case !strings.Contains(r.Todo(), "sytue-rooms-android.aab"):
		t.Errorf("publicables android falla sin decir qué AAB falta: %s", empaquetarJunto(r))
	}
}

// El nombre bajo el icono en iOS y Android: los de más de 12 caracteres llevan un ESPACIO
// EN (U+2002) para que iOS no lo apriete hasta borrarlo; los que caben, el espacio normal.
func TestEmpaquetarNombreBajoElIcono(t *testing.T) {
	const en = " "
	for _, c := range [][2]string{
		{"calendar", "Sytue" + en + "Calendar"},
		{"contacts", "Sytue" + en + "Contacts"},
		{"certipdf", "Sytue" + en + "CertiPDF"},
		{"notes", "Sytue" + en + "Notepads"},
		{"hub", "Sytue Hub"},
		{"calcus", "Sytue Calcus"},
		{"mail", "Sytue Mail"},
	} {
		if got := empaquetarJunto(empaquetar(t, "nombre-movil", c[0])); got != c[1] {
			t.Errorf("el nombre bajo el icono de %s es «%s» y tiene que ser «%s»", c[0], got, c[1])
		}
	}
	if !strings.Contains(empaquetarJunto(empaquetar(t, "plist-simulador", "calendar")), "<key>CFBundleDisplayName</key><string>Sytue"+en+"Calendar</string>") {
		t.Error("el Info.plist del Simulador no lleva el nombre bajo el icono con el espacio en")
	}
}

// El versionCode: el del tag, creciente y sin colisiones.
func TestEmpaquetarVersionCode(t *testing.T) {
	if got := empaquetarJunto(empaquetar(t, "version-code", "v3.6.23")); got != "3006023" {
		t.Errorf("el versionCode de v3.6.23 no es 3006023: %s", got)
	}
	anterior := 0
	for _, tag := range []string{"v3.6.23", "v3.6.999", "v3.7.0", "v3.10.1", "v4.0.0", "v2099.999.999"} {
		r := empaquetar(t, "version-code", tag)
		c, err := strconv.Atoi(r.Texto())
		if r.Codigo != 0 || err != nil {
			t.Errorf("sin versionCode para %s", tag)
			continue
		}
		if c <= anterior {
			t.Errorf("el versionCode de %s (%d) no crece sobre el anterior (%d)", tag, c, anterior)
		}
		if c > 2100000000 {
			t.Errorf("el versionCode de %s (%d) pasa del tope de Play", tag, c)
		}
		anterior = c
	}
	for _, tag := range []string{"v3.1000.0", "v3.6.1000", "v0.1.0", "v2100.0.0", "main", "v3.6"} {
		if r := empaquetar(t, "version-code", tag); r.Codigo == 0 {
			t.Errorf("version-code acepta «%s», que colisionaría o no es un tag", tag)
		}
	}
}

// Lo que firma: la clave de SUBIDA, con la contraseña fuera de argv, y nada del keystore viejo.
func TestEmpaquetarAndroidFirmaConLaClaveDeSubida(t *testing.T) {
	d := empaquetarDirs(t)
	flujos, _ := filepath.Glob(empaquetarUnir(d.raiz, ".github/workflows/*.yml"))
	for _, f := range flujos {
		if l := empaquetarLineas(guiontest.Leer(t, f), `ANDROID_KEYSTORE_|ANDROID_KEY_ALIAS`); len(l) > 0 {
			t.Errorf("se vuelven a pedir los secretos ANDROID_KEYSTORE_* (no existen; la clave es ANDROID_UPLOAD_*): %s: %s", f, l[0])
		}
	}
	movil := guiontest.Leer(t, empaquetarUnir(d.raiz, ".github/workflows/release-mobile.yml"))
	for _, s := range []string{
		"ANDROID_UPLOAD_KEYSTORE_BASE64", "ANDROID_UPLOAD_KEYSTORE_PASSWORD", "ANDROID_UPLOAD_KEY_ALIAS",
		"ANDROID_MIN_SDK", "android:minSdkVersion",
		"publicables android-sin-firmar sin-firmar", "publicables android firmados", "storepass:env", "RUNNER_TEMP", "trap 'rm -f",
	} {
		if !strings.Contains(movil, s) {
			t.Errorf("release-mobile.yml no contiene «%s»", s)
		}
	}
	if l := empaquetarLineas(movil, `-storepass +[^:]|-storepass "|--ks-pass|pass:\$`); len(l) > 0 {
		t.Errorf("release-mobile.yml pasa la contraseña del keystore por argv: %s", strings.Join(l, "\n"))
	}
	if l := empaquetarLineas(movil, `\.apk`); len(l) > 0 {
		t.Errorf("release-mobile.yml vuelve a publicar APK sueltos (Play App Signing: no se podrían actualizar desde Play): %s", strings.Join(l, "\n"))
	}
	if !empaquetarHay(movil, `empaquetar.sh android$`) {
		t.Error("release-mobile.yml ya no construye los AAB de Android")
	}
	if !strings.Contains(movil, "no se suben a Google Play") {
		t.Error("release-mobile.yml ya no se salta la subida a Play con un aviso cuando falta el secreto")
	}
	// El mínimo de la protección de Play, el target y las páginas de 16 KiB, sobre un
	// XML binario y ELF de verdad. No basta cambiar la constante: hay que reescribirlo
	// en el manifiesto que se convierte a AAB y verificar el resultado.
	guion := guiontest.Leer(t, d.guion)
	if !empaquetarHayLinea(guion, "ANDROID_MIN_SDK=24") {
		t.Error("el mínimo de Android no es API 24, requerido por la protección automática de Play")
	}
	if !strings.Contains(guion, `min-sdk "$t/apk/AndroidManifest.xml" "$ANDROID_MIN_SDK"`) {
		t.Error("apk_a_aab no aplica ANDROID_MIN_SDK al manifiesto compilado")
	}
	if !strings.Contains(guion, "android:minSdkVersion=") {
		t.Error("verificar_aab no comprueba el mínimo del manifiesto real")
	}
}

// --- XML binario de Android de prueba ----------------------------------------------
// Los mismos bytes que escribían los fragmentos de Python del original (struct.pack).

func empaquetarLE(vals ...any) []byte {
	var b bytes.Buffer
	for _, v := range vals {
		_ = binary.Write(&b, binary.LittleEndian, v)
	}
	return b.Bytes()
}

const empaquetarNinguno = uint32(0xFFFFFFFF)

// empaquetarAXML arma un manifiesto: pool de cadenas UTF-8, mapa de recursos y cuerpo.
func empaquetarAXML(cadenas []string, recursos [2]uint32, cuerpo func(elem func(nombre uint32, atrs ...[]byte) []byte) []byte) []byte {
	var datos bytes.Buffer
	offs := make([]uint32, 0, len(cadenas))
	o := uint32(0)
	for _, c := range cadenas {
		datos.Write([]byte{byte(len(c)), byte(len(c))})
		datos.WriteString(c)
		datos.WriteByte(0)
		offs = append(offs, o)
		o += uint32(len(c)) + 3
	}
	for datos.Len()%4 != 0 {
		datos.WriteByte(0)
	}
	n := uint32(len(cadenas))
	inicio := 28 + 4*n
	pool := empaquetarLE(uint16(1), uint16(28), inicio+uint32(datos.Len()), n, uint32(0), uint32(0x100), inicio, uint32(0))
	pool = append(pool, empaquetarLE(offs)...)
	pool = append(pool, datos.Bytes()...)
	mapa := empaquetarLE(uint16(0x180), uint16(8), uint32(16), recursos[0], recursos[1])
	elem := func(nombre uint32, atrs ...[]byte) []byte {
		ext := empaquetarLE(empaquetarNinguno, nombre, uint16(20), uint16(20), uint16(len(atrs)), uint16(0), uint16(0), uint16(0))
		for _, a := range atrs {
			ext = append(ext, a...)
		}
		return append(empaquetarLE(uint16(0x102), uint16(16), uint32(16+len(ext)), uint32(1), empaquetarNinguno), ext...)
	}
	todo := append(append(pool, mapa...), cuerpo(elem)...)
	return append(empaquetarLE(uint16(3), uint16(8), uint32(8+len(todo))), todo...)
}

// empaquetarAtr es un atributo: espacio, nombre, valor en cadena, tamaño, tipo y dato.
func empaquetarAtr(ns, nombre, cadena uint32, tipo uint8, dato uint32) []byte {
	return empaquetarLE(ns, nombre, cadena, uint16(8), uint8(0), tipo, dato)
}

var empaquetarFin = empaquetarLE(uint16(0x103), uint16(16), uint32(24), uint32(1), empaquetarNinguno, empaquetarNinguno, uint32(2))

// Un <uses-sdk> con minSdkVersion 21 y targetSdkVersion 29.
func empaquetarManifiestoSDK() []byte {
	return empaquetarAXML([]string{"minSdkVersion", "targetSdkVersion", "uses-sdk", "manifest"}, [2]uint32{0x0101020C, 0x01010270},
		func(elem func(uint32, ...[]byte) []byte) []byte {
			return append(elem(3), elem(2, empaquetarAtr(empaquetarNinguno, 0, empaquetarNinguno, 0x10, 21), empaquetarAtr(empaquetarNinguno, 1, empaquetarNinguno, 0x10, 29))...)
		})
}

// El tipo del servicio en primer plano: un <service> con name y exported, más una
// cadena de VALOR detrás del mapa (como en los manifiestos de verdad).
func empaquetarManifiestoServicio() []byte {
	cad := []string{"name", "exported", "service", "eu.sytue.nativo.Avisos", "http://schemas.android.com/apk/res/android", "manifest", "eu.sytue.nativo.PantallaServicio"}
	return empaquetarAXML(cad, [2]uint32{0x01010003, 0x01010010}, func(elem func(uint32, ...[]byte) []byte) []byte {
		nombre := empaquetarAtr(4, 0, 3, 0x03, 3)
		exportado := empaquetarAtr(4, 1, empaquetarNinguno, 0x12, 0)
		otro := empaquetarAtr(4, 0, 6, 0x03, 6)
		var b []byte
		b = append(b, elem(5)...)
		b = append(b, elem(2, nombre, exportado)...)
		b = append(b, empaquetarFin...)
		b = append(b, elem(2, otro, exportado)...)
		return append(b, empaquetarFin...)
	})
}

// La GoNativeActivity con configChanges y una segunda <activity> (SesionDesbloqueo).
func empaquetarManifiestoActividad() []byte {
	cad := []string{"name", "configChanges", "activity", "org.golang.app.GoNativeActivity", "http://schemas.android.com/apk/res/android", "manifest", "eu.sytue.nativo.SesionDesbloqueo"}
	return empaquetarAXML(cad, [2]uint32{0x01010003, 0x0101001F}, func(elem func(uint32, ...[]byte) []byte) []byte {
		nombre := empaquetarAtr(4, 0, 3, 0x03, 3)
		sesion := empaquetarAtr(4, 0, 6, 0x03, 6)
		cambios := empaquetarAtr(4, 1, empaquetarNinguno, 0x11, 0x4a0)
		var b []byte
		b = append(b, elem(5)...)
		b = append(b, elem(2, nombre, cambios)...)
		b = append(b, empaquetarFin...)
		b = append(b, elem(2, sesion)...)
		return append(b, empaquetarFin...)
	})
}

// android-binario.py sobre XML binario y ELF de verdad. El original fallaba sin python3.
func TestEmpaquetarAndroidBinario(t *testing.T) {
	d := empaquetarDirs(t)
	bin := empaquetarUnir(d.aqui, "android-binario.py")
	py := func(args ...string) guiontest.Resultado {
		t.Helper()
		return guiontest.Ejecutar(t, guiontest.Opciones{}, "python3", append([]string{bin}, args...)...)
	}
	leer := func(que, ruta string, extra ...string) string {
		t.Helper()
		return empaquetarJunto(py(append([]string{que, ruta}, extra...)...))
	}
	tmp := t.TempDir()

	t.Run("mínimo y target", func(t *testing.T) {
		m := empaquetarUnir(tmp, "manifiesto.bin")
		empaquetarEscribirBin(t, m, empaquetarManifiestoSDK())
		if leer("leer-min-sdk", m) != "21" {
			t.Error("android-binario.py no lee el mínimo 21 del manifiesto original")
		}
		if py("target-sdk", m, "36").Codigo != 0 || leer("leer-target-sdk", m) != "36" {
			t.Errorf("android-binario.py no deja el targetSdkVersion en 36: %s", leer("leer-target-sdk", m))
		}
		if leer("leer-min-sdk", m) != "21" {
			t.Error("cambiar targetSdkVersion ha cambiado también minSdkVersion")
		}
		tam := len(empaquetarLeer(m))
		if py("min-sdk", m, "24").Codigo != 0 || leer("leer-min-sdk", m) != "24" {
			t.Error("android-binario.py no deja minSdkVersion en 24 para la protección de Play")
		}
		if leer("leer-target-sdk", m) != "36" {
			t.Error("cambiar minSdkVersion ha perdido targetSdkVersion 36")
		}
		if len(empaquetarLeer(m)) != tam {
			t.Error("cambiar el mínimo ha movido el tamaño del XML binario")
		}
		antes := empaquetarLeer(m)
		if py("min-sdk", m, "24").Codigo != 0 || empaquetarLeer(m) != antes {
			t.Error("aplicar mínimo 24 por segunda vez cambia el manifiesto")
		}
		// Un manifiesto sin el servicio de avisos no se da por bueno.
		if py("servicio-en-primer-plano", m, "eu.sytue.nativo.Avisos", "0x40000000").Codigo == 0 {
			t.Error("android-binario.py da por bueno un manifiesto sin el servicio de avisos")
		}
	})

	// El atributo nuevo tiene que quedar leíble, ordenado por identificador y sin tocar lo demás.
	t.Run("servicio en primer plano", func(t *testing.T) {
		s := empaquetarUnir(tmp, "servicio.bin")
		empaquetarEscribirBin(t, s, empaquetarManifiestoServicio())
		if py("servicio-en-primer-plano", s, "eu.sytue.nativo.Avisos", "0x40000000").Codigo != 0 || leer("leer-servicio", s, "eu.sytue.nativo.Avisos") != "0x40000000" {
			t.Errorf("android-binario.py no deja el servicio de avisos con foregroundServiceType specialUse: %s", leer("leer-servicio", s, "eu.sytue.nativo.Avisos"))
		}
		if py("servicio-en-primer-plano", s, "eu.sytue.nativo.Avisos", "0x40000000").Codigo == 0 {
			t.Error("android-binario.py mete foregroundServiceType dos veces")
		}
		// Un segundo servicio en el mismo manifiesto (el de compartir pantalla de Rooms) reutiliza
		// la cadena y no pisa al primero.
		if py("servicio-en-primer-plano", s, "eu.sytue.nativo.PantallaServicio", "0x20").Codigo != 0 ||
			leer("leer-servicio", s, "eu.sytue.nativo.PantallaServicio") != "0x20" ||
			leer("leer-servicio", s, "eu.sytue.nativo.Avisos") != "0x40000000" {
			t.Error("android-binario.py no da su tipo a un segundo servicio sin tocar el primero")
		}
		// Y un manifiesto con servicios pero sin actividad no admite la imagen dentro de imagen.
		if py("imagen-en-imagen", s).Codigo == 0 {
			t.Error("android-binario.py da por bueno un manifiesto sin actividad para la imagen dentro de imagen")
		}
	})

	// La imagen dentro de imagen: supportsPictureInPicture en la GoNativeActivity, leíble, una
	// sola vez, y sin confundirse con una segunda <activity> (SesionDesbloqueo).
	t.Run("imagen dentro de imagen", func(t *testing.T) {
		a := empaquetarUnir(tmp, "actividad.bin")
		empaquetarEscribirBin(t, a, empaquetarManifiestoActividad())
		if leer("leer-imagen-en-imagen", a) != "" {
			t.Error("android-binario.py lee supportsPictureInPicture en una actividad que no lo lleva")
		}
		if py("imagen-en-imagen", a).Codigo != 0 || leer("leer-imagen-en-imagen", a) != "true" {
			t.Errorf("android-binario.py no deja la actividad con supportsPictureInPicture: %s", leer("leer-imagen-en-imagen", a))
		}
		if py("imagen-en-imagen", a).Codigo == 0 {
			t.Error("android-binario.py mete supportsPictureInPicture dos veces")
		}
	})

	t.Run("no es un manifiesto", func(t *testing.T) {
		n := empaquetarUnir(tmp, "no-manifiesto.bin")
		empaquetarEscribirBin(t, n, []byte("no es xml"))
		if py("target-sdk", n, "36").Codigo == 0 {
			t.Error("android-binario.py da por bueno un fichero que no es un manifiesto")
		}
	})

	// Go alinea sus PT_LOAD a 64 KiB en arm64 y a 4 KiB en amd64: uno pasa y otro no.
	t.Run("páginas de 16 KiB", func(t *testing.T) {
		_, amd64, arm64 := empaquetarBinarios(t)
		if py("alineacion", arm64, "16384").Codigo != 0 {
			t.Error("android-binario.py rechaza un ELF alineado a 64 KiB")
		}
		if py("alineacion", amd64, "16384").Codigo == 0 {
			t.Error("android-binario.py da por buena una .so alineada a 4 KiB: Play la rechazaría")
		}
	})
}

func empaquetarEscribirBin(t *testing.T, ruta string, datos []byte) {
	t.Helper()
	if err := os.WriteFile(ruta, datos, 0o600); err != nil {
		t.Fatal(err)
	}
}

// Los permisos pedidos son los de <uses-permission>, no los que el proveedor de Drive
// exige a quien lo llama.
func TestEmpaquetarPermisosPedidos(t *testing.T) {
	d := empaquetarDirs(t)
	funcion := empaquetarExtraerFunciones(guiontest.Leer(t, d.guion), false, "permisos_pedidos")
	volcado := `N: android=http://schemas.android.com/apk/res/android (line=2)
  E: manifest (line=2)
    E: uses-permission (line=7)
      A: http://schemas.android.com/apk/res/android:name(0x01010003)="android.permission.INTERNET" (Raw: "android.permission.INTERNET")
    E: uses-permission (line=8)
      A: http://schemas.android.com/apk/res/android:name(0x01010003)="android.permission.USE_BIOMETRIC" (Raw: "android.permission.USE_BIOMETRIC")
    E: application (line=10)
      E: provider (line=20)
        A: http://schemas.android.com/apk/res/android:permission(0x01010006)="android.permission.MANAGE_DOCUMENTS" (Raw: "android.permission.MANAGE_DOCUMENTS")`
	r := guiontest.Bash(t, guiontest.Opciones{Entrada: volcado + "\n"}, `eval "$1"; permisos_pedidos`, funcion)
	if r.Texto() != "android.permission.INTERNET android.permission.USE_BIOMETRIC " {
		t.Errorf("permisos_pedidos cuenta como pedido lo que el proveedor exige, o pierde un uses-permission: %s", r.Texto())
	}
}

// Lo que la CLI de Fyne sabe y no sabe codificar en el manifiesto.
func TestEmpaquetarManifiestoQueLaCLIEmpaqueta(t *testing.T) {
	guion := guiontest.Leer(t, empaquetarDirs(t).guion)
	// La CLI de Fyne codifica el manifiesto con una tabla de atributos anterior a la API 23:
	// `android:autoVerify` (y cualquier otro que no esté en ella) tumba el AAB de TODAS las
	// apps al llegar a la que lo declara (Rooms, v3.6.25). Se añade en el binario, no aquí.
	comentario := regexp.MustCompile(`^\s*#`)
	for i, l := range strings.Split(guion, "\n") {
		if (strings.Contains(l, "android:autoVerify") || strings.Contains(l, "android:foregroundServiceType")) && !comentario.MatchString(l) {
			t.Errorf("empaquetar.sh escribe android:autoVerify o android:foregroundServiceType en el manifiesto: la CLI de Fyne no los sabe codificar (%d: %s)", i+1, l)
		}
	}
	// La CLI de Fyne solo empaqueta UNA actividad, la GoNativeActivity: cualquier otra en el
	// manifiesto tumba el AAB con «can only build an .apk for GoNativeActivity» (Rooms,
	// v3.7.10, con la del permiso de compartir pantalla; todas las apps en v3.13.12 con
	// SesionDesbloqueo inyectada por android-sesion.py). Lo que añade actividades o
	// componentes adicionales solo puede ir en $ESCENARIO/android-$app.xml (el manifiesto de
	// aapt2 link), nunca en $dir/AndroidManifest.xml (el que lee `fyne package`).
	for _, m := range regexp.MustCompile(`<activity android:name="[^"]*"`).FindAllString(guion, -1) {
		if !strings.Contains(m, `"org.golang.app.GoNativeActivity"`) {
			t.Errorf("empaquetar.sh declara una actividad que la CLI de Fyne no empaqueta: %s", m)
		}
	}
	if !strings.Contains(guion, `manifiesto_android "" "" >"$dir/AndroidManifest.xml"`) {
		t.Error("empaquetar.sh pasa componentes adicionales a $dir/AndroidManifest.xml: fyne package rechaza SesionDesbloqueo")
	}
	if !strings.Contains(guion, `manifiesto_android "$sesion_queries" "$sesion_components" >"$ESCENARIO/android-$app.xml"`) {
		t.Error("empaquetar.sh no incluye sesion_queries y sesion_components en $ESCENARIO/android-$app.xml para el AAB final")
	}
}

// empaquetarRotacionParaTodas: el manifiesto común de TODAS las apps atiende el giro y
// los cambios de tamaño.
func empaquetarRotacionParaTodas(texto string) bool {
	cambios := strings.Join(empaquetarSed(texto, `^[[:space:]]*android:configChanges="([^"]*)"[[:space:]]*$`), "\n")
	if cambios == "" || strings.Contains(cambios, "$") {
		return false
	}
	for _, c := range []string{"orientation", "keyboardHidden", "uiMode", "screenSize", "smallestScreenSize", "screenLayout"} {
		if !strings.Contains("|"+cambios+"|", "|"+c+"|") {
			return false
		}
	}
	return true
}

// empaquetarSustituirPrimera es `sed 's/viejo/nuevo/'`: la primera vez en cada línea.
func empaquetarSustituirPrimera(texto, viejo, nuevo string) string {
	l := strings.Split(texto, "\n")
	for i := range l {
		l[i] = strings.Replace(l[i], viejo, nuevo, 1)
	}
	return strings.Join(l, "\n")
}

// No basta con orientation: desde API 13 también cambian screenSize/screenLayout.
// Antes se reservaban a las dos apps con PiP y las otras podían recrear la actividad.
func TestEmpaquetarRotacionAndroidParaTodas(t *testing.T) {
	guion := guiontest.Leer(t, empaquetarDirs(t).guion)
	if !empaquetarRotacionParaTodas(guion) {
		t.Error("el manifiesto común de Android no conserva TODAS las apps al girar o redimensionar, incluidas las que no tienen PiP")
	}
	// Casos negativos: el guardián rechaza tanto perder los tamaños como volver a
	// condicionar los flags a una capacidad de la app.
	if empaquetarRotacionParaTodas(empaquetarSustituirPrimera(guion, "|screenSize|smallestScreenSize|screenLayout", "")) {
		t.Error("el guardián acepta Android sin los cambios de tamaño del giro")
	}
	if empaquetarRotacionParaTodas(empaquetarSustituirPrimera(guion, `android:configChanges="`, `android:configChanges="$pip|`)) {
		t.Error("el guardián acepta que el giro dependa de PiP en vez de proteger todas las apps")
	}
}
