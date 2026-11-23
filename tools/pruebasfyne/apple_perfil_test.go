// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package pruebasfyne

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/tools/guiontest"
)

// Traducción de apps/fyne/scripts/apple_perfil-test.py: CMS reales con una CA
// efímera (solo en la prueba): cadena, firma, rol y replay. La CA, los
// firmantes y los CMS los crea esta prueba con openssl; apple_perfil.py es el
// real y decodifica esos bytes.

const applePerfilCarga = `
import importlib.util, plistlib, tempfile
from unittest import mock
spec = importlib.util.spec_from_file_location("perfil_cms", pathlib.Path("apple_perfil.py").resolve())
p = importlib.util.module_from_spec(spec)
spec.loader.exec_module(p)
DER = pathlib.Path(os.environ["CMS_DER"]).read_bytes() if os.environ.get("CMS_DER") else b""
def cms(nombre):
    return pathlib.Path(os.environ["CMS_DIR"], nombre).read_bytes()
`

func TestApplePerfilUnFIFOSinEscritorFallaSinBloquearNiPublicarContenido(t *testing.T) {
	fifo := filepath.Join(t.TempDir(), "sin-escritor.fifo")
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Fatal(err)
	}
	r := guiontest.Ejecutar(t, guiontest.Opciones{Dir: appleScripts(t), Plazo: 2 * time.Second},
		"python3", "apple_perfil.py", "decodificar", fifo, "ios")
	if r.Codigo != 1 || r.Salida != "" || r.Error != "::error::perfil CMS o derechos no válidos: profile_input_invalid\n" {
		t.Errorf("código %d, stdout %q, stderr %q", r.Codigo, r.Salida, r.Error)
	}
}

// appleCMSFirmantes es la CA efímera de la prueba y sus tres firmantes.
type appleCMSFirmantes struct {
	base, raiz, clave, der string
}

func appleOpenSSL(t *testing.T, dir, entrada string, args ...string) {
	t.Helper()
	r := guiontest.Ejecutar(t, guiontest.Opciones{Dir: dir, Entrada: entrada}, "openssl", args...)
	if r.Codigo != 0 {
		t.Fatalf("fixture OpenSSL no creado: openssl %v\n%s", args, r.Error)
	}
}

func appleNuevaCA(t *testing.T) *appleCMSFirmantes {
	t.Helper()
	base := t.TempDir()
	c := &appleCMSFirmantes{base: base, raiz: filepath.Join(base, "root.pem"), clave: filepath.Join(base, "root.key"), der: filepath.Join(base, "root.der")}
	appleOpenSSL(t, base, "", "req", "-x509", "-newkey", "ec", "-pkeyopt", "ec_paramgen_curve:P-256", "-nodes",
		"-subj", "/CN=Sytue ephemeral test CA", "-days", "2", "-keyout", c.clave, "-out", c.raiz,
		"-addext", "basicConstraints=critical,CA:TRUE", "-addext", "keyUsage=critical,keyCertSign,cRLSign")
	appleOpenSSL(t, base, "", "x509", "-in", c.raiz, "-outform", "DER", "-out", c.der)
	for i, f := range [][2]string{
		{"ios", "Apple iPhone OS Provisioning Profile Signing"},
		{"mac", "Mac OS X Provisioning Profile Signing"},
		{"ajeno", "Apple Distribution"},
	} {
		nombre, cn := f[0], f[1]
		ext := filepath.Join(base, nombre+".ext")
		guiontest.Escribir(t, ext, "basicConstraints=critical,CA:FALSE\nkeyUsage=critical,digitalSignature\n")
		appleOpenSSL(t, base, "", "req", "-new", "-newkey", "ec", "-pkeyopt", "ec_paramgen_curve:P-256", "-nodes",
			"-keyout", filepath.Join(base, nombre+".key"), "-out", filepath.Join(base, nombre+".csr"), "-subj", "/CN="+cn+"/O=Apple Inc./C=US")
		appleOpenSSL(t, base, "", "x509", "-req", "-in", filepath.Join(base, nombre+".csr"), "-CA", c.raiz, "-CAkey", c.clave,
			"-set_serial", strconv.Itoa(i+10), "-days", "1", "-extfile", ext, "-out", filepath.Join(base, nombre+".pem"))
	}
	return c
}

// applePayloadXML es el payload del perfil sintético, como plist XML.
func applePayloadXML(plataforma string) string {
	sistema, todos := "OSX", "false"
	if plataforma == "ios" {
		sistema = "iOS"
	}
	if plataforma == "developerid" {
		todos = "true"
	}
	return `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>Platform</key><array><string>` + sistema + `</string></array>
<key>ProvisionsAllDevices</key><` + todos + `/>
<key>TeamIdentifier</key><array><string>VHC5QW7BZ3</string></array>
<key>ExpirationDate</key><date>2050-01-01T00:00:00Z</date>
<key>Entitlements</key><dict><key>test</key><true/></dict>
</dict></plist>
`
}

// applePayloadJSON es lo que tiene que devolver la decodificación de ese payload.
func applePayloadJSON(plataforma string) string {
	sistema, todos := "OSX", "false"
	if plataforma == "ios" {
		sistema = "iOS"
	}
	if plataforma == "developerid" {
		todos = "true"
	}
	return `{"Platform": ["` + sistema + `"], "ProvisionsAllDevices": ` + todos +
		`, "TeamIdentifier": ["VHC5QW7BZ3"], "ExpirationDate": "2050-01-01 00:00:00", "Entitlements": {"test": true}}`
}

// firmar crea el CMS DER y devuelve su contenido.
func (c *appleCMSFirmantes) firmar(t *testing.T, plataforma, firmante string, separado, segundo bool) []byte {
	t.Helper()
	salida := filepath.Join(t.TempDir(), "perfil.cms")
	args := []string{
		"cms", "-sign", "-binary", "-outform", "DER", "-signer", filepath.Join(c.base, firmante+".pem"),
		"-inkey", filepath.Join(c.base, firmante+".key"), "-certfile", c.raiz, "-md", "sha256", "-out", salida,
	}
	if !separado {
		args = append(args, "-nodetach")
	}
	if segundo {
		args = append(args, "-signer", filepath.Join(c.base, "mac.pem"), "-inkey", filepath.Join(c.base, "mac.key"))
	}
	appleOpenSSL(t, c.base, applePayloadXML(plataforma), args...)
	raw, err := os.ReadFile(salida) //nolint:gosec // G304: fichero de la propia prueba.
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

// decodificar escribe los CMS en un directorio y evalúa `expr` (con `cms(n)`,
// `DER` y el módulo `p`) para cada uno; devuelve lo que vio `atrapar`.
func (c *appleCMSFirmantes) decodificar(t *testing.T, codigo string, cms map[string][]byte) map[string]any {
	t.Helper()
	dir := t.TempDir()
	for n, b := range cms {
		if err := os.WriteFile(filepath.Join(dir, n), b, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return applePy(t, applePerfilCarga+codigo, "CMS_DIR="+dir, "CMS_DER="+c.der, "CMS_BASE="+c.base)
}

func TestApplePerfilCMSRealesValidosConFirmanteDeSuRolEnLasTresPlataformas(t *testing.T) {
	c := appleNuevaCA(t)
	cms := map[string][]byte{
		"ios":         c.firmar(t, "ios", "ios", false, false),
		"macappstore": c.firmar(t, "macappstore", "mac", false, false), "developerid": c.firmar(t, "developerid", "mac", false, false),
	}
	R := c.decodificar(t, `
for plataforma in ("ios", "macappstore", "developerid"):
    R[plataforma] = atrapar(p._decodificar_con_raiz, cms(plataforma), plataforma, DER)
`, cms)
	for _, plataforma := range []string{"ios", "macappstore", "developerid"} {
		t.Run(plataforma, func(t *testing.T) {
			appleIgual(t, plataforma, appleOK(t, plataforma, R[plataforma]), applePayloadJSON(plataforma))
		})
	}
}

func TestApplePerfilFirmaCorruptaYRaizAjenaSonRechazadas(t *testing.T) {
	c := appleNuevaCA(t)
	bueno := c.firmar(t, "ios", "ios", false, false)
	corrupto := append([]byte{}, bueno...)
	corrupto[len(corrupto)-1] ^= 1
	R := c.decodificar(t, `
R["corrupto"] = atrapar(p._decodificar_con_raiz, cms("corrupto"), "ios", DER)
# La CA sintética nunca está autorizada por la raíz Apple fijada.
R["raiz_ajena"] = atrapar(p.decodificar, cms("bueno"), "ios")
`, map[string][]byte{"bueno": bueno, "corrupto": corrupto})
	appleExc(t, "firma corrupta", R["corrupto"], "PerfilInvalido", "")
	appleExc(t, "raíz ajena", R["raiz_ajena"], "PerfilInvalido", "")
}

func TestApplePerfilUnCertificadoDeLaRaizConOtroRolNoFirmaPerfiles(t *testing.T) {
	c := appleNuevaCA(t)
	R := c.decodificar(t, `R["r"] = atrapar(p._decodificar_con_raiz, cms("ajeno"), "ios", DER)`,
		map[string][]byte{"ajeno": c.firmar(t, "ios", "ajeno", false, false)})
	appleExc(t, "firmante con otro rol", R["r"], "PerfilInvalido", "official_profile_signer")
}

func TestApplePerfilFirmanteOPayloadDeOtraPlataformaNoSeReutilizan(t *testing.T) {
	c := appleNuevaCA(t)
	casos := []struct{ payload, firmante, plataforma string }{
		{"ios", "ios", "macappstore"},
		{"macappstore", "mac", "ios"},
		{"developerid", "mac", "macappstore"},
		{"macappstore", "mac", "developerid"},
		{"macappstore", "ios", "ios"},
	}
	cms := map[string][]byte{}
	for i, k := range casos {
		cms[strconv.Itoa(i)] = c.firmar(t, k.payload, k.firmante, false, false)
	}
	plataformas, _ := json.Marshal([]string{"macappstore", "ios", "macappstore", "developerid", "ios"})
	R := c.decodificar(t, `
R["r"] = [atrapar(p._decodificar_con_raiz, cms(str(i)), plataforma, DER) for i, plataforma in enumerate(`+string(plataformas)+`)]
`, cms)
	for i, k := range casos {
		appleExc(t, k.payload+" firmado por "+k.firmante+" leído como "+k.plataforma, appleCampo(R, "r", i), "PerfilInvalido", "")
	}
}

func TestApplePerfilDosSignerInfosSeparadoYDatosPosterioresNoSeAceptan(t *testing.T) {
	c := appleNuevaCA(t)
	cms := map[string][]byte{
		"dos": c.firmar(t, "ios", "ios", false, true), "separado": c.firmar(t, "ios", "ios", true, false),
		"basura": append(c.firmar(t, "ios", "ios", false, false), []byte("junk")...),
	}
	R := c.decodificar(t, `
for n in ("dos", "separado", "basura"):
    R[n] = atrapar(p._decodificar_con_raiz, cms(n), "ios", DER)
`, cms)
	for _, n := range []string{"dos", "separado", "basura"} {
		appleExc(t, n, R[n], "PerfilInvalido", "")
	}
}

func TestApplePerfilEntradaPlistSinCMSYFueraDeLimiteNoTienenRespaldo(t *testing.T) {
	c := appleNuevaCA(t)
	R := c.decodificar(t, `
R["plist"] = atrapar(p.decodificar, cms("plist"), "ios")
R["vacio"] = atrapar(p.decodificar, b"", "ios")
R["grande"] = atrapar(p.decodificar, b"x" * (p.LIMITE + 1), "ios")
`, map[string][]byte{"plist": []byte(applePayloadXML("ios"))})
	for _, n := range []string{"plist", "vacio", "grande"} {
		appleExc(t, n, R[n], "PerfilInvalido", "")
	}
}

func TestApplePerfilLaRaizDeProduccionNoAdmiteOverrideSinSuHuella(t *testing.T) {
	c := appleNuevaCA(t)
	R := c.decodificar(t, `
ruta = pathlib.Path(os.environ["CMS_BASE"], "foreign-root.der")
ruta.write_bytes(DER)
with mock.patch.object(p, "RAIZ_APPLE", ruta):
    R["r"] = atrapar(p.decodificar, cms("ios"), "ios")
`, map[string][]byte{"ios": c.firmar(t, "ios", "ios", false, false)})
	appleExc(t, "raíz sustituida", R["r"], "PerfilInvalido", "pinned_apple_root")
}

// El python3 del Mac propio no trae tomllib (3.11+): el lector de respaldo
// tiene que dar exactamente lo mismo en cada FyneApp.toml real.
func TestApplePerfilElRespaldoLeeIgualQueTomllibCadaFyneApp(t *testing.T) {
	catalogo, err := filepath.Glob(filepath.Join(guiontest.Raiz(t), "apps", "fyne", "cmd", "*", "FyneApp.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(catalogo) <= 10 {
		t.Fatalf("catálogo de FyneApp.toml demasiado corto: %d", len(catalogo))
	}
	rutas, _ := json.Marshal(catalogo)
	R := applePy(t, applePerfilCarga+`
import tomllib as real
R["apps"] = {}
with mock.patch.object(p, "tomllib", None):
    for ruta in json.loads(os.environ["CATALOGO"]):
        texto = pathlib.Path(ruta).read_text()
        R["apps"][ruta] = [p.leer_fyneapp(texto), real.loads(texto)]
    R["rechazo"] = atrapar(p.leer_fyneapp, "[Details]\n  Tags = [\"a\"]\n")
`, "CATALOGO="+string(rutas))
	for _, ruta := range catalogo {
		app := filepath.Base(filepath.Dir(ruta))
		t.Run(app, func(t *testing.T) {
			par, _ := appleCampo(R, "apps", ruta).([]any)
			if len(par) != 2 {
				t.Fatalf("sin lectura de %s", ruta)
			}
			appleIgual(t, app, par[0], appleTexto(par[1]))
		})
	}
	appleExc(t, "el respaldo rechaza lo que no sabe leer", R["rechazo"], "ValueError", "")
}

// SYTUE_DESPERTAR_APPLE: `aps-environment` en los derechos del host solo se
// firma si el perfil lo autoriza. Visto en rojo sin la guarda: se aceptaba un
// perfil sin Push.
func TestApplePerfilApsEnvironmentExigeElPerfilConPush(t *testing.T) {
	R := applePy(t, applePerfilCarga+`
def perfil(**derechos):
    return {"TeamIdentifier": ["VHC5QW7BZ3"], "Platform": ["iOS"], "ProvisionsAllDevices": False,
            "ExpirationDate": datetime.datetime(2050, 1, 1),
            "Entitlements": {"application-identifier": "VHC5QW7BZ3.eu.sytue.calendar",
                             "com.apple.security.application-groups": ["group.eu.sytue.suite"],
                             "keychain-access-groups": ["VHC5QW7BZ3.*"], **derechos}}
def validar(perfil, pedidos):
    with tempfile.TemporaryDirectory() as tmp:
        ruta = pathlib.Path(tmp, "derechos.plist")
        ruta.write_bytes(plistlib.dumps({"com.apple.security.application-groups": ["group.eu.sytue.suite"],
                                         "keychain-access-groups": ["VHC5QW7BZ3.eu.sytue.calendar",
                                                                    "VHC5QW7BZ3.eu.sytue.suite"], **pedidos}))
        return p.validar_derechos(perfil, "eu.sytue.calendar", "ios", "group.eu.sytue.suite", "",
                                  "VHC5QW7BZ3.eu.sytue.suite", str(ruta))
R["sin_push"] = atrapar(validar, perfil(), {})
R["con_push"] = atrapar(validar, perfil(**{"aps-environment": "production"}), {"aps-environment": "production"})
R["rechazos"] = [atrapar(validar, perf, {"aps-environment": pedido}) for perf, pedido in (
    (perfil(), "production"),
    (perfil(**{"aps-environment": "development"}), "production"),
    (perfil(**{"aps-environment": "development"}), "development"))]
`)
	appleOK(t, "sin aps-environment ni Push", R["sin_push"])
	appleOK(t, "Push de producción autorizado", R["con_push"])
	for i, n := range []string{"perfil sin Push, pedido production", "perfil development, pedido production", "perfil development, pedido development"} {
		appleExc(t, n, appleCampo(R, "rechazos", i), "PerfilInvalido", "avisos")
	}
}
