// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package pruebasfyne

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/tools/guiontest"
)

// Regresiones de los metadatos que widgetkit-apple.py observa antes de firmar
// los widgets y el proveedor de archivos. Las herramientas de Apple (lipo, nm,
// otool, vtool, codesign, swift-stdlib-tool) se sustituyen en proceso, como en
// el unittest original: `capturar` responde lo que diría la herramienta y
// `ejecutar` solo anota la orden.

const widgetkitDobles = `
import contextlib, io, plistlib, types
from unittest.mock import patch
widgetkit = cargar("widgetkit", SCRIPTS / "widgetkit-apple.py")


class Producto:
    """Productos de Xcode ya compilados: la extensión de widgets y el puente."""

    def __init__(self):
        self.productos = TMP / "productos"
        self.productos.mkdir()
        self.apps = [{"clave": "rooms", "id": "eu.sytue.rooms"}]
        self.plataforma = "ios"
        self.minimo = "15.0"
        self.arquitecturas = "arm64"
        self.tipo = "EXECUTE"
        self.simbolo = "_sytue_widgets_reload"
        self.instalados = "_sytue_widgets_instalados"
        self.bundle("Sytue-rooms-Widgets.appex", "eu.sytue.rooms.widgets", True)
        self.bundle("SytueWidgetBridge.framework", "eu.sytue.widgetbridge", False)

    def bundle(self, nombre, identificador, extension):
        contenido = self.productos / nombre
        if self.plataforma == "macos" and extension:
            contenido /= "Contents"
        contenido.mkdir(parents=True)
        info = {"CFBundleIdentifier": identificador, "CFBundleExecutable": "main"}
        if extension:
            info["NSExtension"] = {"NSExtensionPointIdentifier": "com.apple.widgetkit-extension"}
        (contenido / "Info.plist").write_bytes(plistlib.dumps(info))

    def comando(self, argumentos):
        extension = ".appex" in str(argumentos[-1])
        if argumentos[1] == "lipo":
            return self.arquitecturas
        if argumentos[1] == "nm":
            return "0000000100 T " + self.simbolo + "\n0000000200 T " + self.instalados + "\n"
        if argumentos[1] == "otool":
            tipo = self.tipo if extension else "DYLIB"
            return ("Mach header\nmagic cputype cpusubtype caps filetype ncmds sizeofcmds flags\n"
                    "MH_MAGIC_64 ARM64 ALL 0x00 " + tipo + " 25 4000 0x01\n")
        if argumentos[1] == "vtool":
            return "\n".join("Load command 10\n cmd LC_BUILD_VERSION\n cmdsize 32\n platform IOS\n minos " + self.minimo +
                             "\n sdk 27.1\n ntools 1\n tool LD\n version 1250.0\n" for _ in self.arquitecturas.split())
        raise AssertionError(argumentos)

    def verificar(self, *extra, plataforma=None, **kw):
        with patch.object(widgetkit, "capturar", self.comando), contextlib.redirect_stdout(io.StringIO()):
            return fallo(widgetkit.verificar, self.productos, plataforma or self.plataforma, self.apps, "SytueWidgetBridge", *extra, **kw)


class Insercion:
    """Un .ipa (o .app de macOS) ya empaquetado en el que se insertan los productos."""

    def __init__(self):
        self.raiz = TMP
        self.productos = self.raiz / "productos"
        self.productos.mkdir()
        for nombre in ("Sytue-rooms-Widgets.appex", "SytueWidgetBridge.framework"):
            (self.productos / nombre).mkdir()
            (self.productos / nombre / "main").write_bytes(b"fixture-ejecutable")
        self.host = self.raiz / "ipa/Payload/Sytue Rooms.app"
        self.host.mkdir(parents=True)
        (self.host / "Info.plist").write_bytes(plistlib.dumps({
            "CFBundleIdentifier": "eu.sytue.rooms", "CFBundleExecutable": "main",
            "CFBundleShortVersionString": "3.14.0", "CFBundleVersion": "37"}))
        (self.host / "main").write_bytes(b"fixture-host")
        self.perfil = self.raiz / "perfil.mobileprovision"
        self.perfil.write_bytes(b"fixture-perfil-sin-datos-reales")
        self.app = {"clave": "rooms", "id": "eu.sytue.rooms"}
        self.generador = types.SimpleNamespace(EQUIPO="VHC5QW7BZ3", PUENTE_WIDGETS="SytueWidgetBridge", grupo_widgets=lambda: "group.eu.sytue.suite")
        self.comandos = []
        self.lecturas = []
        self.carga = "@rpath/SytueWidgetBridge.framework/SytueWidgetBridge"
        self.rpath = "@executable_path/Frameworks"

    def capturar(self, argumentos):
        self.lecturas.append(argumentos)
        if "perfil-ruta" in argumentos:
            return str(self.perfil) + "\n"
        if "-L" in argumentos:
            return self.carga + " (compatibility version 1.0.0, current version 1.0.0)\n"
        if "-l" in argumentos:
            return "Load command 10\n cmd LC_RPATH\n path " + self.rpath + " (offset 12)\n"
        raise AssertionError(argumentos)

    def insertar(self, ejecutar=None, destino="ios", *extra, identificador=None):
        parches = [patch.object(widgetkit, "capturar", self.capturar),
                   patch.object(widgetkit, "ejecutar", ejecutar or self.comandos.append)]
        if identificador is not None:
            parches.append(patch.object(widgetkit, "identificador_firma", return_value=identificador))
        with contextlib.ExitStack() as pila:
            for p in parches:
                pila.enter_context(p)
            pila.enter_context(contextlib.redirect_stdout(io.StringIO()))
            return fallo(widgetkit.insertar, self.productos, self.host, destino, "Distribución de pruebas", self.app, self.generador, *extra)

    def a_macos(self):
        (self.host / "Contents/MacOS").mkdir(parents=True)
        (self.host / "Info.plist").rename(self.host / "Contents/Info.plist")
        (self.host / "main").rename(self.host / "Contents/MacOS/main")
        (self.productos / "Sytue-rooms-Widgets.appex/Contents").mkdir()
        self.rpath = "@executable_path/../Frameworks"

    def observado(self, **extra):
        return {"comandos": self.comandos, "lecturas": self.lecturas, "raiz": str(self.raiz), "host": str(self.host), **extra}


def modo(ruta):
    return ruta.stat().st_mode & 0o777
`

// --- Productos ----------------------------------------------------------------

// widgetkitVerificar prepara el producto con `ajuste` y verifica; devuelve la
// excepción de verificar(), si la hubo.
func widgetkitVerificar(t *testing.T, ajuste, llamada string) *pyRestoExcepcion {
	t.Helper()
	if llamada == "" {
		llamada = "p.verificar()"
	}
	var e *pyRestoExcepcion
	pyRestoCorrer(t, widgetkitDobles+"\np = Producto()\n"+ajuste+"\ndevolver("+llamada+")\n", &e)
	return e
}

func widgetkitSinError(t *testing.T, e *pyRestoExcepcion) {
	t.Helper()
	if e != nil {
		t.Fatalf("falló: %v %s", e.Tipos, e.Mensaje)
	}
}

func TestWidgetkitAppleProductoIOSRealModerno(t *testing.T) {
	widgetkitSinError(t, widgetkitVerificar(t, "", ""))
}

func TestWidgetkitAppleElMinimoDelEnlaceAntiguoNoSeOcultaConPlistModerno(t *testing.T) {
	pyRestoLanza(t, widgetkitVerificar(t, `p.minimo = "7.0"`, ""), "ValueError", "mínimo real")
}

func TestWidgetkitAppleObjetoOArchiveNoEsExtensionEjecutable(t *testing.T) {
	pyRestoLanza(t, widgetkitVerificar(t, `p.tipo = "OBJECT"`, ""), "ValueError", "tipo Mach-O")
}

func TestWidgetkitAppleNoAceptaArquitecturaDeSimulador(t *testing.T) {
	pyRestoLanza(t, widgetkitVerificar(t, `p.arquitecturas = "x86_64"`, ""), "ValueError", "arquitecturas")
}

func TestWidgetkitApplePuenteSinExportacionNoActualizariaWidgets(t *testing.T) {
	pyRestoLanza(t, widgetkitVerificar(t, `p.simbolo = "_otro_simbolo"`, ""), "ValueError", "puente público")
}

func TestWidgetkitApplePuenteSinContarLosWidgetsNoDespertariaLaApp(t *testing.T) {
	// Visto en rojo sin la comprobación: el puente sin el contador pasaba.
	pyRestoLanza(t, widgetkitVerificar(t, `p.instalados = "_otro_simbolo"`, ""), "ValueError", "cuenta los widgets")
}

func TestWidgetkitAppleProductoMacOSUniversalConMinimo13(t *testing.T) {
	widgetkitSinError(t, widgetkitVerificar(t, `
p.plataforma, p.minimo, p.arquitecturas = "macos", "13.0", "arm64 x86_64"
p.bundle("Sytue-rooms-Widgets.appex", "eu.sytue.rooms.widgets", True)
`, ""))
}

func TestWidgetkitAppleExtensionConVersionAnteriorSeRechazaAntesDeFirmar(t *testing.T) {
	pyRestoLanza(t, widgetkitVerificar(t, "", `p.verificar("3.14.0", "37")`), "ValueError", "versión distinta")
}

func TestWidgetkitAppleExtensionDeOtroBuildSeRechaza(t *testing.T) {
	pyRestoLanza(t, widgetkitVerificar(t, `
for ruta in p.productos.glob("*/Info.plist"):
    info = plistlib.loads(ruta.read_bytes())
    info.update(CFBundleShortVersionString="3.14.0", CFBundleVersion="36")
    ruta.write_bytes(plistlib.dumps(info))
`, `p.verificar("3.14.0", "37")`), "ValueError", "build distinto")
}

func TestWidgetkitAppleFileProviderIOSRequiereMinimo16YSuTipoReal(t *testing.T) {
	var r []*pyRestoExcepcion
	pyRestoCorrer(t, widgetkitDobles+`
p = Producto()
p.apps = [{"clave": "drive", "id": "eu.sytue.drive", "archivos": True}]
p.bundle("Archivos.appex", "eu.sytue.drive.archivos", True)
ruta = p.productos / "Archivos.appex/Info.plist"
datos = plistlib.loads(ruta.read_bytes())
datos["NSExtension"]["NSExtensionPointIdentifier"] = "com.apple.fileprovider-nonui"
ruta.write_bytes(plistlib.dumps(datos))
resultados = []
p.minimo = "16.0"
resultados.append(p.verificar(plataforma="ios", incluir_widgets=False))
p.minimo = "15.0"
resultados.append(p.verificar(plataforma="ios", incluir_widgets=False))
p.minimo = "16.0"
datos["NSExtension"]["NSExtensionPointIdentifier"] = "com.apple.widgetkit-extension"
ruta.write_bytes(plistlib.dumps(datos))
resultados.append(p.verificar(plataforma="ios", incluir_widgets=False))
devolver(resultados)
`, &r)
	widgetkitSinError(t, r[0])
	pyRestoLanza(t, r[1], "ValueError", "mínimo real")
	pyRestoLanza(t, r[2], "ValueError", "tipo de extensión")
}

// --- Inserción en el host -----------------------------------------------------

type widgetkitInsercion struct {
	Comandos [][]any           `json:"comandos"`
	Lecturas [][]any           `json:"lecturas"`
	Raiz     string            `json:"raiz"`
	Host     string            `json:"host"`
	Error    *pyRestoExcepcion `json:"error"`
	Datos    map[string]any    `json:"datos"`
}

func widgetkitTextos(c []any) []string {
	s := make([]string, len(c))
	for i, v := range c {
		s[i], _ = v.(string)
	}
	return s
}

// firmas son las órdenes `codesign … -s …` (las que firman, no las que verifican).
func (w widgetkitInsercion) firmas() [][]string {
	var out [][]string
	for _, c := range w.Comandos {
		s := widgetkitTextos(c)
		if len(s) > 0 && s[0] == "codesign" && slices.Contains(s, "-s") {
			out = append(out, s)
		}
	}
	return out
}

func (w widgetkitInsercion) con(elemento string) [][]string {
	var out [][]string
	for _, c := range w.Comandos {
		if s := widgetkitTextos(c); slices.Contains(s, elemento) {
			out = append(out, s)
		}
	}
	return out
}

func (w widgetkitInsercion) leyo(elemento string) bool {
	for _, c := range w.Lecturas {
		if slices.Contains(widgetkitTextos(c), elemento) {
			return true
		}
	}
	return false
}

func widgetkitUltimos(cs [][]string) []string {
	out := make([]string, len(cs))
	for i, c := range cs {
		out[i] = c[len(c)-1]
	}
	return out
}

func widgetkitTras(c []string, opcion string) string {
	i := slices.Index(c, opcion)
	if i < 0 || i+1 >= len(c) {
		return ""
	}
	return c[i+1]
}

func widgetkitInsertar(t *testing.T, fragmento string) widgetkitInsercion {
	t.Helper()
	var r widgetkitInsercion
	pyRestoCorrer(t, widgetkitDobles+"\ni = Insercion()\n"+fragmento, &r)
	return r
}

func TestWidgetkitAppleFirmaAnidadaPerfil0644YSwiftSupportDentroDelIPA(t *testing.T) {
	r := widgetkitInsertar(t, `
creado_antes = []

def registrar(comando):
    if "swift-stdlib-tool" in comando:
        destino = comando[comando.index("--unsigned-destination") + 1]
        plataforma = comando[comando.index("--platform") + 1]
        creado_antes.append(destino.is_dir() and not destino.is_symlink())
        (destino / plataforma).mkdir(parents=True, exist_ok=True)
    i.comandos.append(comando)

error = i.insertar(registrar)
perfil = i.host / "PlugIns/Sytue-rooms-Widgets.appex/embedded.mobileprovision"
devolver(i.observado(error=error, datos={"creado_antes": creado_antes, "swift_support": (i.raiz / "ipa/SwiftSupport").exists(),
    "perfil": perfil.read_text(), "original": i.perfil.read_text(), "modo": modo(perfil)}))
`)
	widgetkitSinError(t, r.Error)
	if !reflect.DeepEqual(r.Datos["creado_antes"], []any{true}) {
		t.Fatalf("el destino de swift-stdlib-tool no existía como directorio real antes: %v", r.Datos["creado_antes"])
	}
	if r.Datos["swift_support"] != false {
		t.Fatal("quedó un SwiftSupport vacío en el ipa")
	}
	if r.Datos["perfil"] != r.Datos["original"] || r.Datos["modo"] != float64(0o644) {
		t.Fatalf("perfil anidado %v con modo %o", r.Datos["perfil"], int(r.Datos["modo"].(float64)))
	}
	if !slices.Contains(widgetkitTextos(r.Comandos[0]), "verificar-perfil-grupo") {
		t.Fatalf("la primera orden no es la guarda del perfil: %v", r.Comandos[0])
	}
	swift := r.con("swift-stdlib-tool")
	if len(swift) == 0 {
		t.Fatal("no se llamó a swift-stdlib-tool")
	}
	if widgetkitTras(swift[0], "--unsigned-destination") != r.Raiz+"/ipa/SwiftSupport" || slices.Contains(swift[0], "--strip-bitcode") {
		t.Fatalf("swift-stdlib-tool %v", swift[0])
	}
	firmas := r.firmas()
	if len(firmas) != 2 || !strings.HasSuffix(firmas[0][len(firmas[0])-1], "SytueWidgetBridge.framework") ||
		!strings.HasSuffix(firmas[1][len(firmas[1])-1], "Sytue-rooms-Widgets.appex") {
		t.Fatalf("firmas %v: primero el puente y luego la extensión", firmas)
	}
	if slices.Contains(widgetkitUltimos(firmas), r.Host) {
		t.Fatal("firmó el host: lo firma después quien empaqueta")
	}
}

func TestWidgetkitAppleDriveIncorporaFileProviderConSuPerfilYSinLlaveroDeLaSuite(t *testing.T) {
	r := widgetkitInsertar(t, `
i.app["archivos"] = True
(i.productos / "Archivos.appex").mkdir()
(i.productos / "Archivos.appex/main").write_bytes(b"fixture-proveedor")
error = i.insertar()
proveedor = i.host / "PlugIns/Archivos.appex"
devolver(i.observado(error=error, datos={"perfil": (proveedor / "embedded.mobileprovision").read_text(), "original": i.perfil.read_text()}))
`)
	widgetkitSinError(t, r.Error)
	if r.Datos["perfil"] != r.Datos["original"] {
		t.Fatal("el proveedor no lleva su perfil")
	}
	guardas := r.con("verificar-perfil-grupo")
	if len(guardas) != 2 {
		t.Fatalf("%d guardas de perfil, no 2", len(guardas))
	}
	if !reflect.DeepEqual(guardas[1][4:8], []string{"eu.sytue.rooms.archivos", "ios", "group.eu.sytue.rooms", "NSFileProtectionComplete"}) ||
		guardas[1][8] != "VHC5QW7BZ3.eu.sytue.rooms.shared" {
		t.Fatalf("guarda del proveedor %v", guardas[1])
	}
	firmas := widgetkitUltimos(r.firmas())
	if firmas[len(firmas)-1] != r.Host+"/PlugIns/Archivos.appex" || slices.Contains(firmas, r.Host) {
		t.Fatalf("firmas %v", firmas)
	}
}

func TestWidgetkitAppleFileProviderSinWidgetNoNecesitaPuenteNiSuPerfil(t *testing.T) {
	r := widgetkitInsertar(t, `
i.app["archivos"] = True
(i.productos / "Archivos.appex").mkdir()
(i.productos / "Archivos.appex/main").write_bytes(b"fixture-proveedor")
i.carga = "sin-puente"
error = i.insertar(None, "ios", False)
devolver(i.observado(error=error, datos={"plugins": [str(p) for p in (i.host / "PlugIns").iterdir()],
    "widgets": any(".widgets" in str(c) for c in i.comandos)}))
`)
	widgetkitSinError(t, r.Error)
	if !reflect.DeepEqual(r.Datos["plugins"], []any{r.Host + "/PlugIns/Archivos.appex"}) {
		t.Fatalf("PlugIns %v", r.Datos["plugins"])
	}
	if r.leyo("otool") {
		t.Fatal("miró con otool si el host carga el puente sin hacer falta")
	}
	if r.Datos["widgets"] != false {
		t.Fatal("alguna orden toca los widgets")
	}
	if !reflect.DeepEqual(widgetkitUltimos(r.firmas()), []string{r.Host + "/PlugIns/Archivos.appex"}) {
		t.Fatalf("firmas %v", r.firmas())
	}
}

func TestWidgetkitAppleHostFueraDePayloadNoPuedeEscribirSwiftSupport(t *testing.T) {
	r := widgetkitInsertar(t, `
original = i.host
i.host = i.raiz / "Sytue Rooms.app"
original.rename(i.host)
devolver(i.observado(error=i.insertar()))
`)
	pyRestoLanza(t, r.Error, "ValueError", "Payload real")
	if len(r.Comandos) != 0 {
		t.Fatalf("ejecutó %v", r.Comandos)
	}
}

func TestWidgetkitApplePuenteNoCargadoNoProduceWidgetInutil(t *testing.T) {
	r := widgetkitInsertar(t, `
i.carga = "/System/Library/Frameworks/Foundation.framework/Foundation"
devolver(i.observado(error=i.insertar()))
`)
	pyRestoLanza(t, r.Error, "ValueError", "no carga el puente")
	if len(r.Comandos) != 0 {
		t.Fatalf("ejecutó %v", r.Comandos)
	}
}

func TestWidgetkitAppleNombreORutaParcialDelPuenteNoAutorizaInsercion(t *testing.T) {
	for _, carga := range []string{
		"@rpath/SytueWidgetBridge.framework/SytueWidgetBridgeOtro",
		"@rpath/SytueWidgetBridge.framework/../SytueWidgetBridge",
		"@rpath/SytueWidgetBridge.framework/Versions/B/SytueWidgetBridge",
		"/Library/Frameworks/SytueWidgetBridge.framework/SytueWidgetBridge",
	} {
		t.Run(carga, func(t *testing.T) {
			r := widgetkitInsertar(t, "\ni.carga = "+`"`+carga+`"`+"\ndevolver(i.observado(error=i.insertar()))\n")
			pyRestoLanza(t, r.Error, "ValueError", "no carga el puente")
			if len(r.Comandos) != 0 {
				t.Fatalf("ejecutó %v", r.Comandos)
			}
		})
	}
}

func TestWidgetkitAppleHostIOSQueAbreElPuentePorDlopenNoNecesitaEnlazarlo(t *testing.T) {
	// El host de `fyne package --target ios` no enlaza ningún framework: lo abre
	// notify_apple.go con dlopen. release-desktop 42390120840 caía aquí con photos.
	r := widgetkitInsertar(t, `
i.carga = "/System/Library/Frameworks/Foundation.framework/Foundation"
i.rpath = "otra"
(i.host / "main").write_bytes(b"\x00cabecera\x00@executable_path/Frameworks/SytueWidgetBridge.framework/SytueWidgetBridge\x00resto")
error = i.insertar()
devolver(i.observado(error=error, datos={"puente": (i.host / "Frameworks/SytueWidgetBridge.framework").is_dir()}))
`)
	widgetkitSinError(t, r.Error)
	if r.leyo("otool") {
		t.Fatal("miró con otool un host que abre el puente por dlopen")
	}
	if r.Datos["puente"] != true {
		t.Fatal("el puente no quedó en Frameworks del host")
	}
}

func TestWidgetkitAppleUnaRutaDlopenParecidaNoVale(t *testing.T) {
	for nombre, ruta := range map[string]string{
		"otro-nombre": `b"@executable_path/Frameworks/SytueWidgetBridge.framework/SytueWidgetBridgeOtro\x00"`,
		"subiendo":    `b"@executable_path/../Frameworks/SytueWidgetBridge.framework/SytueWidgetBridge\x00"`,
		"sin-nulo":    `b"@executable_path/Frameworks/SytueWidgetBridge.framework/SytueWidgetBridge"`,
	} {
		t.Run(nombre, func(t *testing.T) {
			r := widgetkitInsertar(t, `
i.carga = "/System/Library/Frameworks/Foundation.framework/Foundation"
(i.host / "main").write_bytes(b"x" + `+ruta+`)
devolver(i.observado(error=i.insertar()))
`)
			pyRestoLanza(t, r.Error, "ValueError", "no carga el puente")
			if len(r.Comandos) != 0 {
				t.Fatalf("ejecutó %v", r.Comandos)
			}
		})
	}
}

func TestWidgetkitAppleLaRutaDlopenEsLaDeNotifyApple(t *testing.T) {
	fuente := guiontest.Leer(t, "apps/nucleo/widgets/notify_apple.go")
	if !strings.Contains(fuente, `dlopen("@executable_path/Frameworks/SytueWidgetBridge.framework/SytueWidgetBridge"`) {
		t.Fatal("notify_apple.go ya no abre el puente en la ruta que widgetkit-apple.py busca en el host")
	}
}

func TestWidgetkitApplePuenteVersionadoEnElBundleEsValido(t *testing.T) {
	r := widgetkitInsertar(t, `
i.carga = "@rpath/SytueWidgetBridge.framework/Versions/A/SytueWidgetBridge"
devolver(i.observado(error=i.insertar()))
`)
	widgetkitSinError(t, r.Error)
}

func TestWidgetkitAppleRpathParcialNoEsRutaValida(t *testing.T) {
	r := widgetkitInsertar(t, `
i.rpath += "Otro"
devolver(i.observado(error=i.insertar()))
`)
	pyRestoLanza(t, r.Error, "ValueError", "no puede resolver")
	if len(r.Comandos) != 0 {
		t.Fatalf("ejecutó %v", r.Comandos)
	}
}

func TestWidgetkitAppleNoSustituyePlugInsFueraDelHost(t *testing.T) {
	r := widgetkitInsertar(t, `
fuera = i.raiz / "fuera"
fuera.mkdir()
(i.host / "PlugIns").symlink_to(fuera, target_is_directory=True)
error = i.insertar()
devolver(i.observado(error=error, datos={"fuera": [p.name for p in fuera.iterdir()]}))
`)
	pyRestoLanza(t, r.Error, "ValueError", "dentro del host")
	if len(r.Datos["fuera"].([]any)) != 0 {
		t.Fatalf("escribió fuera del host: %v", r.Datos["fuera"])
	}
}

func TestWidgetkitAppleRecursosAnidadosCopiadosConUmask077QuedanLegiblesAntesDeFirmar(t *testing.T) {
	r := widgetkitInsertar(t, `
recursos = i.productos / "Sytue-rooms-Widgets.appex/Contents/Resources"
recursos.mkdir(parents=True)
icono = recursos / "Icon.png"
icono.write_bytes(b"png")
os.chmod(icono, 0o600)
os.chmod(recursos, 0o700)
os.chmod(i.productos / "Sytue-rooms-Widgets.appex/main", 0o700)
puente_info = i.productos / "SytueWidgetBridge.framework/Resources"
puente_info.mkdir(parents=True)
(puente_info / "Info.plist").write_bytes(b"plist")
os.chmod(puente_info / "Info.plist", 0o600)
os.chmod(puente_info, 0o700)
modos_al_firmar = {}
destino_era_dir = []

def registrar(comando):
    i.comandos.append(comando)
    if "swift-stdlib-tool" in comando:
        destino = comando[comando.index("--unsigned-destination") + 1]
        plataforma = comando[comando.index("--platform") + 1]
        destino_era_dir.append(destino.is_dir())
        soporte = destino / plataforma
        soporte.mkdir(parents=True, exist_ok=True)
        (i.host / "Frameworks/libswiftCore.dylib").write_bytes(b"dylib")
        (soporte / "libswiftCore.dylib").write_bytes(b"dylib")
    if comando[0] == "codesign" and "-s" in comando:
        objetivo = pathlib.Path(comando[-1])
        modos_al_firmar[objetivo.name] = {
            str(r.relative_to(objetivo)): r.stat().st_mode & 0o777
            for c, _, fs in os.walk(objetivo)
            for r in [pathlib.Path(c)] + [pathlib.Path(c) / f for f in fs]
        }

umask_previa = os.umask(0o077)
try:
    error = i.insertar(registrar, identificador="com.apple.dt.runtime.swiftCore")
finally:
    os.umask(umask_previa)
devolver(i.observado(error=error, datos={"modos": modos_al_firmar, "destino_era_dir": destino_era_dir,
    "soporte": modo(i.raiz / "ipa/SwiftSupport"), "dylib": modo(i.raiz / "ipa/SwiftSupport/iphoneos/libswiftCore.dylib")}))
`)
	widgetkitSinError(t, r.Error)
	if !reflect.DeepEqual(r.Datos["destino_era_dir"], []any{true}) {
		t.Fatalf("el destino de swift-stdlib-tool no era un directorio: %v", r.Datos["destino_era_dir"])
	}
	modos := r.Datos["modos"].(map[string]any)
	for objetivo, esperados := range map[string]map[string]int{
		"Sytue-rooms-Widgets.appex":   {"Contents/Resources/Icon.png": 0o644, "Contents/Resources": 0o755, "main": 0o755},
		"SytueWidgetBridge.framework": {"Resources/Info.plist": 0o644, "Resources": 0o755},
	} {
		vistos, ok := modos[objetivo].(map[string]any)
		if !ok {
			t.Fatalf("no se firmó %s: %v", objetivo, modos)
		}
		for ruta, m := range esperados {
			if vistos[ruta] != float64(m) {
				t.Fatalf("%s/%s al firmar: %v, no %o", objetivo, ruta, vistos[ruta], m)
			}
		}
	}
	if r.Datos["soporte"] != float64(0o755) || r.Datos["dylib"] != float64(0o644) {
		t.Fatalf("SwiftSupport %v y su dylib %v; se esperaba 0755 y 0644", r.Datos["soporte"], r.Datos["dylib"])
	}
}

func TestWidgetkitAppleMacOSFirmaAntesDelHostConElPerfilDelDestinoCorrecto(t *testing.T) {
	for _, destino := range []string{"macappstore", "developerid"} {
		t.Run(destino, func(t *testing.T) {
			r := widgetkitInsertar(t, `
i.a_macos()
error = i.insertar(None, "`+destino+`")
perfil = i.host / "Contents/PlugIns/Sytue-rooms-Widgets.appex/Contents/embedded.provisionprofile"
devolver(i.observado(error=error, datos={"modo": modo(perfil)}))
`)
			widgetkitSinError(t, r.Error)
			if r.Datos["modo"] != float64(0o644) {
				t.Fatalf("perfil con modo %v, no 0644", r.Datos["modo"])
			}
			if len(r.Lecturas) < 3 {
				t.Fatalf("lecturas %v", r.Lecturas)
			}
			if l := widgetkitTextos(r.Lecturas[2]); l[len(l)-1] != destino {
				t.Fatalf("el perfil se buscó para %v, no para %s", l, destino)
			}
			swift := r.con("swift-stdlib-tool")
			if len(swift) == 0 || slices.Contains(swift[0], "--unsigned-destination") || !slices.Contains(swift[0], "--options=runtime") {
				t.Fatalf("swift-stdlib-tool %v", swift)
			}
			for _, firma := range r.firmas() {
				if !slices.Contains(firma, "--timestamp") || firma[len(firma)-1] == r.Host {
					t.Fatalf("firma %v: sin --timestamp o sobre el host", firma)
				}
			}
		})
	}
}

func TestWidgetkitAppleSwiftStdlibEliminaResiduosOriginalYFirmaDylibsEnMacOS(t *testing.T) {
	r := widgetkitInsertar(t, `
i.a_macos()
frameworks = i.host / "Contents/Frameworks"

def registrar(comando):
    i.comandos.append(comando)
    if "swift-stdlib-tool" in comando:
        dylib = frameworks / "libswiftCore.dylib"
        dylib.write_bytes(b"dylib")
        os.chmod(dylib, 0o600)
        (frameworks / "libswiftCore.dylib.original").write_bytes(b"expired-serial-33")

error = i.insertar(registrar, "macappstore")
devolver(i.observado(error=error, datos={"original": (frameworks / "libswiftCore.dylib.original").exists(),
    "modo": modo(frameworks / "libswiftCore.dylib"), "dylib": str(frameworks / "libswiftCore.dylib")}))
`)
	widgetkitSinError(t, r.Error)
	if r.Datos["original"] != false {
		t.Fatal("quedó libswiftCore.dylib.original")
	}
	if r.Datos["modo"] != float64(0o644) {
		t.Fatalf("la dylib quedó con %v, no 0644", r.Datos["modo"])
	}
	dylib := r.Datos["dylib"].(string)
	if !slices.Contains(widgetkitUltimos(r.firmas()), dylib) {
		t.Fatalf("no firmó %s", dylib)
	}
	var verificadas []string
	for _, c := range r.Comandos {
		if s := widgetkitTextos(c); len(s) >= 3 && reflect.DeepEqual(s[:3], []string{"codesign", "--verify", "--strict"}) {
			verificadas = append(verificadas, s[len(s)-1])
		}
	}
	if !slices.Contains(verificadas, dylib) {
		t.Fatalf("no verificó %s: %v", dylib, verificadas)
	}
}

func TestWidgetkitAppleSwiftStdlibEliminaResiduosOriginalEnIOSYRechazaBasuraEnFrameworks(t *testing.T) {
	r := widgetkitInsertar(t, `
frameworks = i.host / "Frameworks"
soporte = i.raiz / "ipa/SwiftSupport/iphoneos"
era_dir = []

def registrar(comando):
    i.comandos.append(comando)
    if "swift-stdlib-tool" in comando:
        destino = comando[comando.index("--unsigned-destination") + 1]
        plataforma = comando[comando.index("--platform") + 1]
        era_dir.append(destino.is_dir())
        soporte_real = destino / plataforma
        soporte_real.mkdir(parents=True, exist_ok=True)
        (frameworks / "libswiftCore.dylib").write_bytes(b"dylib")
        (frameworks / "libswiftCore.dylib.original").write_bytes(b"backup")
        (soporte_real / "libswiftCore.dylib").write_bytes(b"unsigned")
        (soporte_real / "libswiftCore.dylib.original").write_bytes(b"unsigned-backup")

primero = i.insertar(registrar, identificador="com.apple.dt.runtime.swiftCore")
datos = {"primero": primero, "era_dir": era_dir,
         "fw_original": (frameworks / "libswiftCore.dylib.original").exists(),
         "soporte_original": (soporte / "libswiftCore.dylib.original").exists(),
         "dylib_fw": str(frameworks / "libswiftCore.dylib"), "dylib_soporte": str(soporte / "libswiftCore.dylib"),
         "firmados": [str(c[-1]) for c in i.comandos if c[0] == "codesign" and "-s" in c]}

def registrar_desajuste(comando):
    i.comandos.append(comando)
    if "swift-stdlib-tool" in comando:
        destino = comando[comando.index("--unsigned-destination") + 1]
        plataforma = comando[comando.index("--platform") + 1]
        (destino / plataforma).mkdir(parents=True, exist_ok=True)
        (frameworks / "libswiftCore.dylib").write_bytes(b"dylib")

(soporte / "libswiftCore.dylib").unlink()
with patch.object(widgetkit, "capturar", i.capturar), patch.object(widgetkit, "ejecutar", registrar_desajuste):
    datos["desajuste"] = fallo(widgetkit.insertar, i.productos, i.host, "ios", "Distribución de pruebas", i.app, i.generador)

def registrar_basura(comando):
    i.comandos.append(comando)
    if "swift-stdlib-tool" in comando:
        (frameworks / "libswiftCore.dylib.bak").write_bytes(b"basura")

with patch.object(widgetkit, "capturar", i.capturar), patch.object(widgetkit, "ejecutar", registrar_basura):
    datos["basura"] = fallo(widgetkit.insertar, i.productos, i.host, "ios", "Distribución de pruebas", i.app, i.generador)
devolver(i.observado(datos=datos))
`)
	d := r.Datos
	if d["primero"] != nil {
		t.Fatalf("la primera inserción falló: %v", d["primero"])
	}
	if !reflect.DeepEqual(d["era_dir"], []any{true}) {
		t.Fatalf("el destino de swift-stdlib-tool no era un directorio: %v", d["era_dir"])
	}
	if d["fw_original"] != false || d["soporte_original"] != false {
		t.Fatalf("quedaron residuos .original: Frameworks %v, SwiftSupport %v", d["fw_original"], d["soporte_original"])
	}
	firmadosJSON := d["firmados"].([]any)
	firmados := make([]string, 0, len(firmadosJSON))
	for _, f := range firmadosJSON {
		firmados = append(firmados, f.(string))
	}
	if !slices.Contains(firmados, d["dylib_fw"].(string)) || slices.Contains(firmados, d["dylib_soporte"].(string)) {
		t.Fatalf("firmados %v: la dylib de Frameworks sí, la de SwiftSupport no", firmados)
	}
	var desajuste, basura *pyRestoExcepcion
	pyRestoExcepcionDe(t, d["desajuste"], &desajuste)
	pyRestoExcepcionDe(t, d["basura"], &basura)
	pyRestoLanza(t, desajuste, "ValueError", "SwiftSupport no coincide con Frameworks")
	pyRestoLanza(t, basura, "ValueError", "contenido no admitido en Frameworks")
}

func TestWidgetkitAppleSwiftSupportVacioEnIOSSeEliminaParaEvitar90430(t *testing.T) {
	r := widgetkitInsertar(t, `
era_dir = []

def registrar_vacio(comando):
    i.comandos.append(comando)
    if "swift-stdlib-tool" in comando:
        destino = comando[comando.index("--unsigned-destination") + 1]
        plataforma = comando[comando.index("--platform") + 1]
        era_dir.append(destino.is_dir())
        soporte_real = destino / plataforma
        soporte_real.mkdir(parents=True, exist_ok=True)
        (soporte_real / "libswiftCore.dylib.original").write_bytes(b"backup")

datos = {"vacio": i.insertar(registrar_vacio), "era_dir": era_dir, "existe": (i.raiz / "ipa/SwiftSupport").exists()}

def registrar_basura_soporte(comando):
    i.comandos.append(comando)
    if "swift-stdlib-tool" in comando:
        destino = comando[comando.index("--unsigned-destination") + 1]
        plataforma = comando[comando.index("--platform") + 1]
        soporte_real = destino / plataforma
        soporte_real.mkdir(parents=True, exist_ok=True)
        (soporte_real / "basura.txt").write_bytes(b"basura")

with patch.object(widgetkit, "capturar", i.capturar), patch.object(widgetkit, "ejecutar", registrar_basura_soporte):
    datos["basura"] = fallo(widgetkit.insertar, i.productos, i.host, "ios", "Distribución de pruebas", i.app, i.generador)
devolver(i.observado(datos=datos))
`)
	d := r.Datos
	if d["vacio"] != nil || !reflect.DeepEqual(d["era_dir"], []any{true}) {
		t.Fatalf("inserción %v, destino directorio %v", d["vacio"], d["era_dir"])
	}
	if d["existe"] != false {
		t.Fatal("quedó un SwiftSupport vacío: App Store lo rechaza (ITMS-90430)")
	}
	var basura *pyRestoExcepcion
	pyRestoExcepcionDe(t, d["basura"], &basura)
	pyRestoLanza(t, basura, "ValueError", "contenido no admitido en SwiftSupport")
}

func TestWidgetkitAppleIdentificadorFirmaExigeSigningIDRealEnSwiftSupport(t *testing.T) {
	var r struct {
		Ok       string            `json:"ok"`
		SinID    *pyRestoExcepcion `json:"sin_id"`
		SinFirma *pyRestoExcepcion `json:"sin_firma"`
	}
	pyRestoCorrer(t, widgetkitDobles+`
dylib = TMP / "libswift_Concurrency.dylib"
dylib.write_bytes(b"macho")
resultado_ok = types.SimpleNamespace(returncode=0, stdout="Executable=/x\nIdentifier=com.apple.dt.runtime.swiftConcurrency\n")
with patch.object(widgetkit.subprocess, "run", return_value=resultado_ok):
    ok = widgetkit.identificador_firma(dylib)
sin_id = types.SimpleNamespace(returncode=0, stdout="Executable=/x\nIdentifier=not set\n")
with patch.object(widgetkit.subprocess, "run", return_value=sin_id):
    error_id = fallo(widgetkit.identificador_firma, dylib)
sin_firma = types.SimpleNamespace(returncode=1, stdout="code object is not signed at all\n")
with patch.object(widgetkit.subprocess, "run", return_value=sin_firma):
    error_firma = fallo(widgetkit.identificador_firma, dylib)
devolver({"ok": ok, "sin_id": error_id, "sin_firma": error_firma})
`, &r)
	if r.Ok != "com.apple.dt.runtime.swiftConcurrency" {
		t.Fatalf("identificador %q", r.Ok)
	}
	pyRestoLanza(t, r.SinID, "ValueError", "sin signing ID en SwiftSupport")
	pyRestoLanza(t, r.SinFirma, "ValueError", "sin firma en SwiftSupport")
}
