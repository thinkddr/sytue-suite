// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package pruebasfyne

import (
	"regexp"
	"sort"
	"strings"
	"testing"
)

// Traducción de apps/fyne/scripts/apple-perfiles-test.py. Contrato de
// provisión: sondeo de solo lectura y perfiles de la identidad real.
// apple-perfiles.py es el real; la API de App Store Connect (`API`), el
// llavero (`capturar`) y el CMS (`validar_perfil`) son dobles en proceso,
// como en el original.

const applePerfilesCarga = `
import contextlib, hashlib, importlib.util, io, plistlib, shlex, stat, tempfile, types, urllib.parse
from unittest import mock
p_spec = importlib.util.spec_from_file_location("perfiles", pathlib.Path("apple-perfiles.py").resolve())
p = importlib.util.module_from_spec(p_spec)
p_spec.loader.exec_module(p)
AQUI = pathlib.Path(".").resolve()
GRUPO, LLAVERO = "group.eu.sytue.suite", "VHC5QW7BZ3.eu.sytue.suite"
CADUCA = "2050-01-01T00:00:00Z"
REQUISITO = {"app": "rooms", "id": "eu.sytue.rooms", "widget": False, "group": GRUPO,
             "keychain": LLAVERO, "protection": "",
             "entitlements": {"com.apple.security.application-groups": [GRUPO],
                              "keychain-access-groups": ["VHC5QW7BZ3.eu.sytue.rooms", LLAVERO]}}

def exportado(der, selector="a" * 40):
    return ("SHA-256 hash: " + hashlib.sha256(der).hexdigest().upper() +
            "\nSHA-1 hash: " + selector.upper() + "\n-----BEGIN CERTIFICATE-----\n" +
            base64.b64encode(der).decode() + "\n-----END CERTIFICATE-----\n")

def certificado_api(nombre, der, **attrs):
    return {"id": nombre, "attributes": {"certificateType": "DISTRIBUTION", "expirationDate": CADUCA,
            "certificateContent": base64.b64encode(der).decode(), **attrs}}

def perfil(identificador="perfil", certificado="cert-importado", autorizado=True):
    return {"type": "profiles", "id": identificador, "certificate": certificado,
            "attributes": {"profileType": "IOS_APP_STORE", "profileState": "ACTIVE",
                           "createdDate": "2026-10-20T10:00:00Z", "expirationDate": CADUCA,
                           "authorized": autorizado, "profileContent": base64.b64encode(b"perfil publico").decode()}}

class API:
    def __init__(self):
        self.perfiles, self.escritos, self.gets = [], [], []
        self.identificador = "eu.sytue.rooms"
        self.capacidades = [{"attributes": {"capabilityType": "APP_GROUPS"}}]
        self.certs = []

    def lista(self, ruta):
        self.gets.append(ruta)
        if ruta.startswith("/v1/bundleIds?"):
            return [{"id": "bundle", "attributes": {"identifier": self.identificador, "platform": "UNIVERSAL"}}]
        if urllib.parse.urlsplit(ruta).path.endswith("/bundleIdCapabilities"):
            return self.capacidades
        if ruta.startswith("/v1/certificates?"):
            return self.certs
        if ruta == "/v1/bundleIds/bundle/profiles?limit=200":
            return self.perfiles
        if ruta.startswith("/v1/profiles/") and ruta.endswith("/certificates?limit=200"):
            identificador = ruta.split("/")[3]
            return [{"id": next(x["certificate"] for x in self.perfiles if x["id"] == identificador)}]
        raise AssertionError("lectura no esperada: " + ruta)

    def escribir(self, tipo, attrs, rels):
        self.escritos.append((tipo, attrs, rels))
        assert tipo == "profiles", "no se pueden crear certificados, grupos ni App IDs"
        nuevo = perfil("creado", rels["certificates"][0]["id"])
        self.perfiles.append(nuevo)
        return {"data": nuevo}

def validar_falso(item, requisito, plataforma, motivos=None):
    assert requisito == REQUISITO
    assert plataforma == "ios"
    if not item["attributes"]["authorized"] and motivos is not None:
        motivos.append("el perfil no autoriza el AppGroup")
    return b"perfil publico" if item["attributes"]["authorized"] else None

def recibos(plataforma):
    catalogo = {"hub": {"clave": "hub", "id": "eu.sytue.one", "widget_acciones": "open"}}
    catalogo.update({nombre: {"clave": nombre, "id": "eu.sytue." + nombre, "widget_acciones": "open", "archivos": nombre == "drive"}
                     for nombre in ("bases", "calcus", "calendar", "cast", "certipdf", "contacts", "docs",
                                    "drive", "erp", "mail", "notes", "photos", "rooms", "sheets")})
    datos = [{"app": app["clave"], "app_id": app["id"] + sufijo, "platform": plataforma,
              "profile_type": p.TIPOS[plataforma], "profile_ready": True, "certificate_verified": True,
              "created": bool(sufijo), "path": "no-publicar-perfil-cms", "otro": "no-publicar"}
             for app in catalogo.values() for sufijo, _, _ in p.destinos_app(app)]
    return catalogo, datos

def escribir_recibo(tmp, datos):
    ruta = pathlib.Path(tmp, "recibo.jsonl")
    ruta.write_text("".join(json.dumps(dato) + "\n" for dato in datos))
    return ruta

def ficheros(tmp):
    return sorted(x.name for x in pathlib.Path(tmp).iterdir())

def modo(ruta):
    return stat.S_IMODE(pathlib.Path(ruta).stat().st_mode)
`

func applePerfiles(t *testing.T, codigo string) map[string]any {
	t.Helper()
	return applePy(t, applePerfilesCarga+codigo)
}

func TestApplePerfilesElReciboCotejaTodosLosDestinosYSoloEmiteCamposSegurosSinAPI(t *testing.T) {
	R := applePerfiles(t, `
R["plataformas"] = list(p.TIPOS)
for plataforma in p.TIPOS:
    catalogo, datos = recibos(plataforma)
    gen = types.SimpleNamespace(apps=lambda: catalogo.values())
    with tempfile.TemporaryDirectory() as tmp:
        ruta = escribir_recibo(tmp, datos)
        with mock.patch.dict(p.REGISTRO, {"importar": mock.Mock(return_value=gen)}) as registro, \
                mock.patch.object(p, "requisitos", side_effect=AssertionError("consultó derechos")), \
                mock.patch.object(p, "identidad_importada", side_effect=AssertionError("consultó llavero")), \
                mock.patch("sys.argv", ["apple-perfiles.py", "--plataforma", plataforma, "--comprobar-recibo", str(ruta)]), \
                contextlib.redirect_stdout(io.StringIO()) as salida:
            codigo = p.main()
            importado = registro["importar"].call_count
    R[plataforma] = {"codigo": codigo, "importado": importado, "salida": salida.getvalue(),
                     "lineas": [json.loads(l) for l in salida.getvalue().splitlines()]}
`)
	plataformas, _ := R["plataformas"].([]any)
	if len(plataformas) != 3 {
		t.Fatalf("se esperaban tres plataformas: %s", appleTexto(plataformas))
	}
	for _, pl := range plataformas {
		plataforma, _ := pl.(string)
		t.Run(plataforma, func(t *testing.T) {
			r := R[plataforma]
			appleFalso(t, "main()", appleCampo(r, "codigo"))
			appleIgual(t, "importar una vez", appleCampo(r, "importado"), `1`)
			lineas, _ := appleCampo(r, "lineas").([]any)
			if len(lineas) != 32 {
				t.Fatalf("se esperaban 32 líneas, hay %d", len(lineas))
			}
			appleIgual(t, "resumen", lineas[31], `{"platform": "`+plataforma+`", "profile_count": 31, "ready_count": 31, "created_count": 16}`)
			m, _ := lineas[0].(map[string]any)
			claves := make([]string, 0, len(m))
			for k := range m {
				claves = append(claves, k)
			}
			sort.Strings(claves)
			appleIgual(t, "campos de la primera línea", claves,
				`["app", "app_id", "certificate_verified", "created", "platform", "profile_ready", "profile_type"]`)
			if s, _ := appleCampo(r, "salida").(string); strings.Contains(s, "no-publicar") {
				t.Error("el recibo publica campos privados")
			}
		})
	}
}

func TestApplePerfilesElReciboRechazaAusenciasDuplicadosIdentidadTipoCertificadoYBooleanosAmbiguos(t *testing.T) {
	R := applePerfiles(t, `
catalogo, originales = recibos("ios")
casos = [("incompleto", originales[:-1]), ("duplicado", originales[:-1] + [originales[0]])]
for campo, valores in {"profile_ready": (False, None, 1, "true"),
                       "certificate_verified": (False, None, 1, "true"),
                       "created": (None, 0, "false"), "app": ("drive",),
                       "app_id": ("ajeno",), "platform": ("developerid",),
                       "profile_type": ("MAC_APP_DIRECT",)}.items():
    for valor in valores:
        casos.append((campo + repr(valor), [{**originales[0], campo: valor}, *originales[1:]]))
casos.extend((("demasiados", originales + [originales[0]]), ("objeto", [None, *originales[1:]])))
R["casos"] = []
for nombre, datos in casos:
    with tempfile.TemporaryDirectory() as tmp:
        ruta = escribir_recibo(tmp, datos)
        with contextlib.redirect_stdout(io.StringIO()) as salida:
            r = atrapar(p.comprobar_recibo, ruta, catalogo, "ios")
    R["casos"].append({"nombre": nombre, "r": r, "salida": salida.getvalue()})
`)
	casos, _ := R["casos"].([]any)
	if len(casos) != 19 {
		t.Errorf("se esperaban 19 casos, hay %d", len(casos))
	}
	for _, c := range casos {
		nombre, _ := appleCampo(c, "nombre").(string)
		t.Run(nombre, func(t *testing.T) {
			appleExc(t, nombre, appleCampo(c, "r"), "ValueError", "")
			appleIgual(t, "salida", appleCampo(c, "salida"), `""`)
		})
	}
}

func TestApplePerfilesFileProviderEsObligatorioSoloEnAppsQueLoDeclaran(t *testing.T) {
	R := applePerfiles(t, `
catalogo, datos = recibos("ios")
R["archivos"] = [d["app_id"] for d in datos if d["app_id"].endswith(".archivos")]
with tempfile.TemporaryDirectory() as tmp:
    ruta = escribir_recibo(tmp, [d for d in datos if not d["app_id"].endswith(".archivos")])
    R["r"] = atrapar(p.comprobar_recibo, ruta, catalogo, "ios")
`)
	appleIgual(t, "destinos .archivos", R["archivos"], `["eu.sytue.drive.archivos"]`)
	appleExc(t, "recibo sin FileProvider", R["r"], "ValueError", "incompleto")
}

func TestApplePerfilesElModoReciboRechazaEntradasMezcladasAntesDeImportar(t *testing.T) {
	R := applePerfiles(t, `
R["casos"] = []
for extra in (["--preparar", "--salida", "tmp"], ["--salida", "tmp"], ["--seco"],
              ["--certificado-sha1", "a" * 40], ["--destinos", "todos"], ["rooms"]):
    importar = mock.Mock()
    with mock.patch.dict(p.REGISTRO, {"importar": importar}), \
            mock.patch("sys.argv", ["apple-perfiles.py", "--plataforma", "ios", "--comprobar-recibo", "recibo", *extra]), \
            contextlib.redirect_stderr(io.StringIO()):
        r = atrapar(p.main)
    R["casos"].append({"extra": " ".join(extra), "r": r, "importado": importar.called})
`)
	casos, _ := R["casos"].([]any)
	for _, c := range casos {
		extra, _ := appleCampo(c, "extra").(string)
		t.Run(extra, func(t *testing.T) {
			appleExc(t, extra, appleCampo(c, "r"), "SystemExit", "")
			appleIgual(t, "código de salida", appleCampo(c, "r", "code"), `2`)
			appleIgual(t, "no importa", appleCampo(c, "importado"), `false`)
		})
	}
}

func TestApplePerfilesElSondeoPorDefectoSoloHaceGETYNoInspeccionaLlaveros(t *testing.T) {
	R := applePerfiles(t, `
asc = API()
asc.perfiles = [perfil()]
gen = types.SimpleNamespace(apps=lambda: [{"clave": "rooms"}])
def importar(nombre, ruta):
    return gen if str(ruta).endswith("generar.py") else types.SimpleNamespace(Apple=lambda: asc)
with mock.patch.dict(p.REGISTRO, {"importar": importar}), mock.patch.object(p, "requisitos", return_value=REQUISITO), \
        mock.patch.object(p, "validar_perfil", side_effect=validar_falso), \
        mock.patch.object(p, "identidad_importada", side_effect=AssertionError("sondeo leyó el llavero")), \
        mock.patch("sys.argv", ["apple-perfiles.py", "--plataforma", "ios", "rooms"]), contextlib.redirect_stdout(io.StringIO()) as salida:
    R["codigo"] = p.main()
R["dato"] = json.loads(salida.getvalue())
R["escritos"] = asc.escritos
`)
	appleFalso(t, "main()", R["codigo"])
	appleCierto(t, "profile_ready", appleCampo(R, "dato", "profile_ready"))
	appleIgual(t, "registered_platform", appleCampo(R, "dato", "registered_platform"), `"UNIVERSAL"`)
	appleFalso(t, "certificate_verified", appleCampo(R, "dato", "certificate_verified"))
	appleIgual(t, "escritos", R["escritos"], `[]`)
}

func TestApplePerfilesPrepararUsaElDERImportadoParaElegirElCertificadoDeLaAPI(t *testing.T) {
	R := applePerfiles(t, `
asc, der = API(), b"certificado publico importado"
asc.perfiles, asc.certs = [perfil()], [certificado_api("cert-importado", der)]
gen = types.SimpleNamespace(EQUIPO="VHC5QW7BZ3", apps=lambda: [{"clave": "rooms"}])
def importar(nombre, ruta):
    return gen if str(ruta).endswith("generar.py") else types.SimpleNamespace(Apple=lambda: asc)
with tempfile.TemporaryDirectory() as tmp, mock.patch.dict(p.REGISTRO, {"importar": importar}), \
        mock.patch.object(p, "requisitos", return_value=REQUISITO), \
        mock.patch.object(p, "validar_perfil", side_effect=validar_falso), \
        mock.patch.object(p, "identidad_importada", return_value=der) as identidad, \
        mock.patch("sys.argv", ["apple-perfiles.py", "--plataforma", "ios", "--preparar", "--salida", tmp,
                                "--certificado-sha1", "a" * 40, "rooms"]), contextlib.redirect_stdout(io.StringIO()) as salida:
    R["codigo"] = p.main()
    R["perfil"] = pathlib.Path(tmp, "eu.sytue.rooms.mobileprovision").read_bytes()
R["identidad"] = [list(c.args) for c in identidad.call_args_list]
R["dato"] = json.loads(salida.getvalue())
R["escritos"] = asc.escritos
`)
	appleFalso(t, "main()", R["codigo"])
	appleIgual(t, "perfil guardado", R["perfil"], `"perfil publico"`)
	appleIgual(t, "identidad_importada una vez", R["identidad"], `[["ios", "VHC5QW7BZ3", "`+strings.Repeat("a", 40)+`"]]`)
	appleCierto(t, "certificate_verified", appleCampo(R, "dato", "certificate_verified"))
	appleIgual(t, "escritos", R["escritos"], `[]`)
}

func TestApplePerfilesElClienteSoloExistentesRechazaTodaEscrituraAntesDeAutenticar(t *testing.T) {
	R := applePerfiles(t, `
pedir = mock.Mock(return_value={"data": []})
class Base:
    def pedir(self, *args, **kwargs):
        return pedir(*args, **kwargs)
    def escribir(self):
        return self.pedir("POST", "/v1/profiles", {"data": {}})
asc = p.cliente_apple(types.SimpleNamespace(Apple=Base), True)
R["metodos"] = [atrapar(asc.pedir, m, "/v1/profiles", c) for m, c in (("POST", {}), ("PATCH", {}), ("DELETE", None), ("GET", {}))]
R["escribir"] = atrapar(asc.escribir)
R["antes"] = pedir.call_count
R["get"] = asc.pedir("GET", "/v1/profiles")
R["llamadas"] = [[list(c.args), c.kwargs] for c in pedir.call_args_list]
`)
	for i, m := range []string{"POST", "PATCH", "DELETE", "GET con cuerpo"} {
		appleExc(t, m, appleCampo(R, "metodos", i), "RuntimeError", "sólo admite consultas GET")
	}
	appleExc(t, "escribir()", R["escribir"], "RuntimeError", "")
	appleIgual(t, "nada llegó a la API", R["antes"], `0`)
	appleIgual(t, "el GET pasa", R["get"], `{"data": []}`)
	appleIgual(t, "una única llamada GET", R["llamadas"], `[[["GET", "/v1/profiles"], {"tolerar": []}]]`)
}

func TestApplePerfilesSoloExistentesNoCreaSiFaltaCertificadoVigenciaTipoODerechos(t *testing.T) {
	R := applePerfiles(t, `
invalido, caducado, tipo = perfil(), perfil(), perfil()
invalido["attributes"]["profileState"] = "INVALID"
caducado["attributes"]["expirationDate"] = "2020-01-01T00:00:00Z"
tipo["attributes"]["profileType"] = "IOS_APP_DEVELOPMENT"
R["casos"] = []
for perfiles in ([], [perfil(certificado="otra-clave")], [invalido], [caducado], [tipo], [perfil(autorizado=False)]):
    asc = API()
    asc.perfiles = perfiles
    with tempfile.TemporaryDirectory() as tmp, mock.patch.object(p, "validar_perfil", side_effect=validar_falso):
        r = p.preparar(asc, REQUISITO, "ios", "cert-importado", pathlib.Path(tmp), True)
        R["casos"].append({"r": r, "escritos": asc.escritos, "ficheros": ficheros(tmp)})
`)
	for i, nombre := range []string{"sin perfiles", "otro certificado", "INVALID", "caducado", "IOS_APP_DEVELOPMENT", "no autorizado"} {
		t.Run(nombre, func(t *testing.T) {
			c := appleCampo(R, "casos", i)
			appleFalso(t, "profile_ready", appleCampo(c, "r", "profile_ready"))
			appleFalso(t, "created", appleCampo(c, "r", "created"))
			appleIgual(t, "reason", appleCampo(c, "r", "reason"), `"existing_profile_missing_or_invalid"`)
			appleIgual(t, "escritos", appleCampo(c, "escritos"), `[]`)
			appleIgual(t, "ficheros", appleCampo(c, "ficheros"), `[]`)
		})
	}
}

func TestApplePerfilesSoloExistentesReutilizaYGuardaSoloDespuesDeValidarElPerfil(t *testing.T) {
	R := applePerfiles(t, `
asc = API()
asc.perfiles = [perfil()]
with tempfile.TemporaryDirectory() as tmp, mock.patch.object(p, "validar_perfil", side_effect=validar_falso) as cms:
    r = p.preparar(asc, REQUISITO, "ios", "cert-importado", pathlib.Path(tmp), True)
    R["r"] = r
    R["contenido"] = pathlib.Path(r["path"]).read_bytes()
    R["modo"] = modo(r["path"])
    R["cms"] = cms.call_count
R["escritos"] = asc.escritos
`)
	appleCierto(t, "profile_ready", appleCampo(R, "r", "profile_ready"))
	appleCierto(t, "certificate_verified", appleCampo(R, "r", "certificate_verified"))
	appleFalso(t, "created", appleCampo(R, "r", "created"))
	appleIgual(t, "contenido", R["contenido"], `"perfil publico"`)
	appleIgual(t, "modo 0600", R["modo"], `384`)
	appleIgual(t, "validar_perfil una vez", R["cms"], `1`)
	appleIgual(t, "escritos", R["escritos"], `[]`)
}

func TestApplePerfilesElReciboSoloExistentesExige31PerfilesNoCreadosPorPlataforma(t *testing.T) {
	R := applePerfiles(t, `
R["plataformas"] = list(p.TIPOS)
for plataforma in p.TIPOS:
    catalogo, datos = recibos(plataforma)
    datos = [{**dato, "created": False} for dato in datos]
    with tempfile.TemporaryDirectory() as tmp:
        ruta = escribir_recibo(tmp, datos)
        with contextlib.redirect_stdout(io.StringIO()) as salida:
            bien = p.comprobar_recibo(ruta, catalogo, plataforma, True)
        resumen = json.loads(salida.getvalue().splitlines()[-1])
        datos[-1]["created"] = True
        ruta = escribir_recibo(tmp, datos)
        with contextlib.redirect_stdout(io.StringIO()) as salida2:
            mal = atrapar(p.comprobar_recibo, ruta, catalogo, plataforma, True)
    R[plataforma] = {"bien": bien, "resumen": resumen, "mal": mal, "salida": salida2.getvalue()}
`)
	plataformas, _ := R["plataformas"].([]any)
	for _, pl := range plataformas {
		plataforma, _ := pl.(string)
		t.Run(plataforma, func(t *testing.T) {
			r := R[plataforma]
			appleFalso(t, "comprobar_recibo", appleCampo(r, "bien"))
			appleIgual(t, "resumen", appleCampo(r, "resumen"), `{"platform": "`+plataforma+`", "profile_count": 31, "ready_count": 31, "created_count": 0}`)
			appleExc(t, "un perfil creado", appleCampo(r, "mal"), "ValueError", "perfil creado")
			appleIgual(t, "salida del rechazo", appleCampo(r, "salida"), `""`)
		})
	}
}

// 2026-11-05: el perfil creado antes de asociar el App Group tenía la misma
// huella y el mismo día que su sustituto, y Apple contestó 409 a los 31.
func TestApplePerfilesElSustitutoDeUnPerfilDeHoyNoRepiteSuNombre(t *testing.T) {
	R := applePerfiles(t, `
asc = API()
with tempfile.TemporaryDirectory() as tmp, mock.patch.object(p, "validar_perfil", side_effect=validar_falso):
    p.preparar(asc, REQUISITO, "ios", "cert-importado", pathlib.Path(tmp))
R["nombre"] = asc.escritos[0][1]["name"]
`)
	// Fecha y hora hasta el segundo (UTC), no solo el día.
	if n, _ := R["nombre"].(string); !regexp.MustCompile(` \d{14}$`).MatchString(n) {
		t.Errorf("el nombre del perfil no acaba en fecha y hora al segundo: %q", n)
	}
}

func TestApplePerfilesUnPerfilDeOtroCertificadoNoSeReutilizaYLaSiguienteEjecucionNoDuplica(t *testing.T) {
	R := applePerfiles(t, `
asc = API()
asc.perfiles = [perfil("ajeno", "otra-clave")]
with tempfile.TemporaryDirectory() as tmp, mock.patch.object(p, "validar_perfil", side_effect=validar_falso):
    primera = p.preparar(asc, REQUISITO, "ios", "cert-importado", pathlib.Path(tmp))
    segunda = p.preparar(asc, REQUISITO, "ios", "cert-importado", pathlib.Path(tmp))
    R["primera"], R["segunda"] = primera, segunda
    R["esperada"] = str(pathlib.Path(tmp, "eu.sytue.rooms.mobileprovision"))
    R["contenido"] = pathlib.Path(primera["path"]).read_bytes()
    R["modo"] = modo(primera["path"])
R["escritos"] = asc.escritos
`)
	appleCierto(t, "la primera crea", appleCampo(R, "primera", "created"))
	appleFalso(t, "la segunda no", appleCampo(R, "segunda", "created"))
	if e, _ := R["escritos"].([]any); len(e) != 1 {
		t.Errorf("se esperaba una sola creación: %s", appleTexto(e))
	}
	appleIgual(t, "certificado del perfil creado", appleCampo(R, "escritos", 0, 2, "certificates"), `[{"type": "certificates", "id": "cert-importado"}]`)
	appleIgual(t, "ruta", appleCampo(R, "primera", "path"), appleTexto(R["esperada"]))
	appleIgual(t, "contenido", R["contenido"], `"perfil publico"`)
	appleIgual(t, "modo 0600", R["modo"], `384`)
}

func TestApplePerfilesCreatedDateOpcionalNoImpideValidarYReutilizarPerfiles(t *testing.T) {
	R := applePerfiles(t, `
R["casos"] = []
for fechas in ((None, None), (None, CADUCA), ("ausente", CADUCA), (42, CADUCA), ({"fecha": CADUCA}, None), (False, CADUCA)):
    asc = API()
    asc.perfiles = [perfil("primero", "otro-certificado"), perfil("cms-invalido", autorizado=False), perfil("vigente")]
    for item, fecha in zip(asc.perfiles, (*fechas, fechas[-1]), strict=True):
        if fecha == "ausente":
            item["attributes"].pop("createdDate")
        else:
            item["attributes"]["createdDate"] = fecha
    with tempfile.TemporaryDirectory() as tmp, mock.patch.object(p, "validar_perfil", side_effect=validar_falso) as cms:
        r = p.preparar(asc, REQUISITO, "ios", "cert-importado", pathlib.Path(tmp))
        R["casos"].append({"fechas": repr(fechas), "r": r, "escritos": asc.escritos,
                           "contenido": pathlib.Path(r["path"]).read_bytes() if r.get("path") else None,
                           "validados": [c.args[0]["id"] for c in cms.call_args_list]})
`)
	casos, _ := R["casos"].([]any)
	if len(casos) != 6 {
		t.Fatalf("se esperaban 6 casos: %d", len(casos))
	}
	for _, c := range casos {
		nombre, _ := appleCampo(c, "fechas").(string)
		t.Run(nombre, func(t *testing.T) {
			appleCierto(t, "profile_ready", appleCampo(c, "r", "profile_ready"))
			appleFalso(t, "created", appleCampo(c, "r", "created"))
			appleIgual(t, "escritos", appleCampo(c, "escritos"), `[]`)
			appleIgual(t, "contenido", appleCampo(c, "contenido"), `"perfil publico"`)
			appleIgual(t, "perfiles validados", appleCampo(c, "validados"), `["cms-invalido", "vigente"]`)
		})
	}
}

func TestApplePerfilesLosPerfilesPidenLosSettingsYRechazanCapacidadesDuplicadas(t *testing.T) {
	R := applePerfiles(t, `
asc = API()
p.preparar(asc, REQUISITO, "ios")
ruta = next(r for r in asc.gets if "bundleIdCapabilities" in r)
R["consulta"] = urllib.parse.parse_qs(urllib.parse.urlsplit(ruta).query)
asc.capacidades *= 2
R["duplicada"] = atrapar(p.preparar, asc, REQUISITO, "ios")
R["escritos"] = asc.escritos
`)
	appleIgual(t, "consulta de capacidades", R["consulta"], `{"fields[bundleIdCapabilities]": ["capabilityType,settings"]}`)
	appleExc(t, "capacidad duplicada", R["duplicada"], "RuntimeError", "capacidad repetida")
	appleIgual(t, "escritos", R["escritos"], `[]`)
}

func TestApplePerfilesUnaCapacidadQueFaltaNoCreaPerfiles(t *testing.T) {
	R := applePerfiles(t, `
asc = API()
asc.capacidades = []
with tempfile.TemporaryDirectory() as tmp:
    R["r"] = p.preparar(asc, REQUISITO, "ios", "cert-importado", pathlib.Path(tmp))
R["escritos"] = asc.escritos
`)
	appleFalso(t, "profile_ready", appleCampo(R, "r", "profile_ready"))
	appleIgual(t, "reason", appleCampo(R, "r", "reason"), `"app_group_capability_missing"`)
	appleIgual(t, "escritos", R["escritos"], `[]`)
}

// SYTUE_DESPERTAR_APPLE: `aps-environment` en los derechos pide Push en el App
// ID. Visto en rojo sin la comprobación: se creaba un perfil sin los avisos.
func TestApplePerfilesElDespertarExigeLaCapacidadPushSinCrearPerfiles(t *testing.T) {
	R := applePerfiles(t, `
asc = API()
requisito = {**REQUISITO, "entitlements": {**REQUISITO["entitlements"], "aps-environment": "production"}}
with tempfile.TemporaryDirectory() as tmp:
    R["sin_push"] = p.preparar(asc, requisito, "ios", "cert-importado", pathlib.Path(tmp))
R["escritos"] = list(asc.escritos)
asc.capacidades.append({"attributes": {"capabilityType": "PUSH_NOTIFICATIONS"}})
with mock.patch.object(p, "validar_perfil", return_value=b"perfil publico"), tempfile.TemporaryDirectory() as tmp:
    R["con_push"] = p.preparar(asc, requisito, "ios", "cert-importado", pathlib.Path(tmp))
`)
	appleIgual(t, "reason", appleCampo(R, "sin_push", "reason"), `"push_capability_missing"`)
	appleIgual(t, "escritos", R["escritos"], `[]`)
	appleCierto(t, "con Push, profile_ready", appleCampo(R, "con_push", "profile_ready"))
}

func TestApplePerfilesUnCMSQueNoAutorizaElGrupoNoSeGuarda(t *testing.T) {
	R := applePerfiles(t, `
asc = API()
def crear(*args, **kwargs):
    nuevo = perfil("creado", autorizado=False)
    asc.perfiles.append(nuevo)
    return {"data": nuevo}
with tempfile.TemporaryDirectory() as tmp, mock.patch.object(asc, "escribir", side_effect=crear), \
        mock.patch.object(p, "validar_perfil", side_effect=validar_falso):
    R["r"] = p.preparar(asc, REQUISITO, "ios", "cert-importado", pathlib.Path(tmp))
    R["ficheros"] = ficheros(tmp)
`)
	appleFalso(t, "profile_ready", appleCampo(R, "r", "profile_ready"))
	appleIgual(t, "ficheros", R["ficheros"], `[]`)
	// El recibo dice QUÉ no autoriza: sin eso, 31 «no autoriza» no dicen qué
	// cambiar en el portal de Apple (2026-11-05).
	appleIgual(t, "detail", appleCampo(R, "r", "detail"), `"el perfil no autoriza el AppGroup"`)
}

func TestApplePerfilesUnPerfilNuevoExigeTipoActivoVigenteYCertificadoImportado(t *testing.T) {
	R := applePerfiles(t, `
R["casos"] = []
for campo, valor in (("profileType", "IOS_APP_DEVELOPMENT"), ("profileState", "INVALID"),
                     ("expirationDate", "2020-01-01T00:00:00Z"), ("certificate", "otra-clave")):
    asc = API()
    crear_original = asc.escribir
    def crear(*args, **kwargs):
        respuesta = crear_original(*args, **kwargs)
        nuevo = respuesta["data"]
        (nuevo if campo == "certificate" else nuevo["attributes"])[campo] = valor
        return respuesta
    with tempfile.TemporaryDirectory() as tmp, mock.patch.object(asc, "escribir", side_effect=crear), \
            mock.patch.object(p, "validar_perfil", side_effect=validar_falso) as cms:
        r = p.preparar(asc, REQUISITO, "ios", "cert-importado", pathlib.Path(tmp))
        R["casos"].append({"r": r, "ficheros": ficheros(tmp), "cms": cms.called})
`)
	for i, campo := range []string{"profileType", "profileState", "expirationDate", "certificate"} {
		t.Run(campo, func(t *testing.T) {
			c := appleCampo(R, "casos", i)
			appleCierto(t, "created", appleCampo(c, "r", "created"))
			appleFalso(t, "profile_ready", appleCampo(c, "r", "profile_ready"))
			appleIgual(t, "reason", appleCampo(c, "r", "reason"), `"created_profile_invalid_or_wrong_certificate"`)
			appleIgual(t, "ficheros", appleCampo(c, "ficheros"), `[]`)
			appleIgual(t, "no valida el CMS", appleCampo(c, "cms"), `false`)
		})
	}
}

func TestApplePerfilesLosWidgetsExigenProteccionCompletaSinCrearPerfilesIncompletos(t *testing.T) {
	R := applePerfiles(t, `
asc = API()
asc.identificador = "eu.sytue.rooms.widgets"
requisito = {**REQUISITO, "id": asc.identificador, "widget": True, "keychain": "", "protection": "NSFileProtectionComplete"}
with tempfile.TemporaryDirectory() as tmp:
    R["sin"] = p.preparar(asc, requisito, "ios", "cert-importado", pathlib.Path(tmp))
    R["escritos"] = list(asc.escritos)
asc.capacidades.append({"attributes": {"capabilityType": "DATA_PROTECTION", "settings": p.REGISTRO["PROTECCION"]}})
with mock.patch.object(p, "validar_perfil", return_value=b"perfil publico"), tempfile.TemporaryDirectory() as tmp:
    R["con"] = p.preparar(asc, requisito, "ios", "cert-importado", pathlib.Path(tmp))
`)
	appleIgual(t, "reason", appleCampo(R, "sin", "reason"), `"data_protection_complete_missing"`)
	appleIgual(t, "escritos", R["escritos"], `[]`)
	appleCierto(t, "profile_ready", appleCampo(R, "con", "profile_ready"))
	if ruta, _ := appleCampo(R, "con", "path").(string); !strings.HasSuffix(ruta, "eu.sytue.rooms.widgets.mobileprovision") {
		t.Errorf("ruta del perfil del widget: %q", ruta)
	}
}

func TestApplePerfilesLaProyeccionSeleccionadaNoSupleElCMSDeProteccionCompleta(t *testing.T) {
	R := applePerfiles(t, `
asc = API()
asc.identificador = "eu.sytue.rooms.widgets"
asc.capacidades.append({"attributes": {"capabilityType": "DATA_PROTECTION", "settings": [
    {"key": "DATA_PROTECTION_PERMISSION_LEVEL", "options": [{"key": "COMPLETE_PROTECTION"}]}]}})
asc.perfiles = [perfil()]
requisito = {**REQUISITO, "id": asc.identificador, "widget": True, "keychain": "", "protection": "NSFileProtectionComplete"}
with tempfile.TemporaryDirectory() as tmp, mock.patch.object(p, "validar_perfil", return_value=None) as cms:
    R["rechazo"] = p.preparar(asc, requisito, "ios", "cert-importado", pathlib.Path(tmp))
    R["cms1"] = cms.called
    R["ficheros"] = ficheros(tmp)
asc.escritos.clear()
with tempfile.TemporaryDirectory() as tmp, mock.patch.object(p, "validar_perfil", return_value=b"perfil publico") as cms:
    R["bien"] = p.preparar(asc, requisito, "ios", "cert-importado", pathlib.Path(tmp))
    R["cms2"] = cms.called
R["escritos"] = asc.escritos
`)
	appleCierto(t, "valida el CMS", R["cms1"])
	appleFalso(t, "profile_ready sin CMS válido", appleCampo(R, "rechazo", "profile_ready"))
	appleIgual(t, "ficheros", R["ficheros"], `[]`)
	appleCierto(t, "valida el CMS (2)", R["cms2"])
	appleCierto(t, "profile_ready con CMS válido", appleCampo(R, "bien", "profile_ready"))
	appleIgual(t, "reutilizar el perfil que sí autoriza los derechos", R["escritos"], `[]`)
}

func TestApplePerfilesLaIdentidadSoloValeEnElLlaveroEfimeroYConElEquipoCanonico(t *testing.T) {
	R := applePerfiles(t, `
buena, otra = "a" * 40, "b" * 40
der = b"certificado publico importado"
respuestas = ["/tmp/ephemeral.keychain-db\n",
              ' 1) ' + buena + ' "Apple Distribution: Sytue (VHC5QW7BZ3)"\n 2) ' + otra + ' "Apple Distribution: Otro (ABCDE12345)"',
              exportado(b"otro equipo", otra) + exportado(der, buena)]
with mock.patch.object(p, "capturar", side_effect=respuestas) as capturar:
    R["sin_selector"] = p.identidad_importada("ios", "VHC5QW7BZ3") == der
    R["llamada1"] = capturar.call_args_list[1].args[0][-1]
    R["llamada2"] = capturar.call_args_list[2].args[0]
with mock.patch.object(p, "capturar", side_effect=respuestas):
    R["con_dos_puntos"] = p.identidad_importada("ios", "VHC5QW7BZ3", ":".join(["AA"] * 20)) == der
with mock.patch.object(p, "capturar", side_effect=respuestas):
    R["otro_equipo"] = atrapar(p.identidad_importada, "ios", "VHC5QW7BZ3", otra)
dos = respuestas[1] + '\n 3) ' + "c" * 40 + ' "Apple Distribution: Otro certificado (VHC5QW7BZ3)"'
with mock.patch.object(p, "capturar", side_effect=[respuestas[0], dos]):
    R["dos"] = atrapar(p.identidad_importada, "ios", "VHC5QW7BZ3")
`)
	appleIgual(t, "identidad del equipo canónico", R["sin_selector"], `true`)
	appleIgual(t, "find-identity sobre el llavero efímero", R["llamada1"], `"/tmp/ephemeral.keychain-db"`)
	appleIgual(t, "exportación", R["llamada2"], `["security", "find-certificate", "-a", "-Z", "-p", "/tmp/ephemeral.keychain-db"]`)
	appleIgual(t, "selector con dos puntos", R["con_dos_puntos"], `true`)
	appleExc(t, "selector de otro equipo", R["otro_equipo"], "ValueError", "")
	appleExc(t, "dos identidades del equipo", R["dos"], "ValueError", "")
}

func TestApplePerfilesUnSelectorAmbiguoNoAutenticaCertificadosConSHA1Coincidente(t *testing.T) {
	R := applePerfiles(t, `
selector = "a" * 40
identidad = ' 1) ' + selector + ' "Apple Distribution: Sytue (VHC5QW7BZ3)"'
# Simula dos DER con el mismo selector SHA-1, cada uno con su SHA-256 real.
salida = exportado(b"certificado original", selector) + exportado(b"certificado colisionado", selector)
with mock.patch.object(p, "capturar", side_effect=["/tmp/ephemeral.keychain-db", identidad, salida]):
    R["r"] = atrapar(p.identidad_importada, "ios", "VHC5QW7BZ3", selector)
`)
	appleExc(t, "selector ambiguo", R["r"], "ValueError", "certificado único")
}

func TestApplePerfilesUnaExportacionInvalidaOSinCertificadoNoSeAcepta(t *testing.T) {
	R := applePerfiles(t, `
identidad = ' 1) ' + "a" * 40 + ' "Apple Distribution: Sytue (VHC5QW7BZ3)"'
certificado = exportado(b"certificado original")
alterado = certificado.replace(hashlib.sha256(b"certificado original").hexdigest().upper(), "0" * 64)
R["casos"] = []
for salida in ("", exportado(b"otro", "b" * 40), alterado, certificado.replace("SHA-1 hash:", "hash:"),
               "error de exportación\n" + certificado, certificado.replace("b3JpZ2luYWw=", "@@@")):
    with mock.patch.object(p, "capturar", side_effect=["/tmp/ephemeral.keychain-db", identidad, salida]):
        R["casos"].append(atrapar(p.identidad_importada, "ios", "VHC5QW7BZ3"))
`)
	for i, n := range []string{"vacía", "otro selector", "SHA-256 alterado", "sin SHA-1", "texto delante", "base64 roto"} {
		appleExc(t, n, appleCampo(R, "casos", i), "ValueError", "")
	}
}

func TestApplePerfilesElSelectorSigueSiendoSHA1NoSHA256(t *testing.T) {
	R := applePerfiles(t, `
identidad = ' 1) ' + "a" * 40 + ' "Apple Distribution: Sytue (VHC5QW7BZ3)"'
R["casos"] = []
for selector in ("a" * 39, "a" * 41, "a" * 64, "g" * 40):
    with mock.patch.object(p, "capturar", side_effect=["/tmp/ephemeral.keychain-db", identidad]):
        R["casos"].append(atrapar(p.identidad_importada, "ios", "VHC5QW7BZ3", selector))
`)
	for i, n := range []string{"39 hex", "41 hex", "64 hex", "no hex"} {
		appleExc(t, n, appleCampo(R, "casos", i), "ValueError", "")
	}
}

func TestApplePerfilesElCertificadoDeLaAPISeEligePorDERNoPorNombreNiOrden(t *testing.T) {
	R := applePerfiles(t, `
asc = API()
der = b"certificado publico importado"
asc.certs = [certificado_api("otra-clave", b"otra clave"), certificado_api("correcto", der)]
with mock.patch.object(p.hashlib, "sha1", side_effect=AssertionError("SHA-1 no autentica certificados remotos")):
    R["elegido"] = atrapar(p.certificado_de_identidad, asc, "ios", der)
asc.certs[1]["attributes"]["expirationDate"] = "2020-01-01T00:00:00Z"
R["caducado"] = atrapar(p.certificado_de_identidad, asc, "ios", der)
R["escritos"] = asc.escritos
`)
	appleIgual(t, "certificado elegido", appleOK(t, "elegir", R["elegido"]), `"correcto"`)
	appleExc(t, "certificado caducado", R["caducado"], "ValueError", "")
	appleIgual(t, "escritos", R["escritos"], `[]`)
}

func TestApplePerfilesUnCertificadoRemotoDistintoNoSeAceptaAunqueSHA1Colisionara(t *testing.T) {
	R := applePerfiles(t, `
asc = API()
asc.certs = [certificado_api("colisionado", b"certificado colisionado")]
with mock.patch.object(p.hashlib, "sha1", return_value=types.SimpleNamespace(hexdigest=lambda: "a" * 40)):
    R["r"] = atrapar(p.certificado_de_identidad, asc, "ios", b"certificado original")
R["escritos"] = asc.escritos
`)
	appleExc(t, "colisión SHA-1", R["r"], "ValueError", "vigente único")
	appleIgual(t, "escritos", R["escritos"], `[]`)
}

func TestApplePerfilesCertificadosDeLaAPIInactivosIncorrectosInvalidosODuplicadosNoSeAceptan(t *testing.T) {
	R := applePerfiles(t, `
der = b"certificado original"
R["certificados"] = []
for certificados in ([certificado_api("inactivo", der, activated=False)],
                     [certificado_api("otro-tipo", der, certificateType="DEVELOPER_ID_APPLICATION")],
                     [certificado_api("invalido", der, certificateContent="@@@")],
                     [certificado_api("duplicado-1", der), certificado_api("duplicado-2", der)]):
    asc = API()
    asc.certs = certificados
    R["certificados"].append({"r": atrapar(p.certificado_de_identidad, asc, "ios", der), "escritos": asc.escritos})
R["identidades"] = []
for identidad in (b"", "a" * 40, None):
    asc = API()
    R["identidades"].append({"r": atrapar(p.certificado_de_identidad, asc, "ios", identidad), "gets": asc.gets})
`)
	for i, n := range []string{"inactivo", "otro tipo", "contenido inválido", "duplicado"} {
		t.Run(n, func(t *testing.T) {
			appleExc(t, n, appleCampo(R, "certificados", i, "r"), "ValueError", "")
			appleIgual(t, "escritos", appleCampo(R, "certificados", i, "escritos"), `[]`)
		})
	}
	for i, n := range []string{"DER vacío", "selector en vez de DER", "None"} {
		t.Run(n, func(t *testing.T) {
			appleExc(t, n, appleCampo(R, "identidades", i, "r"), "ValueError", "")
			appleIgual(t, "sin consultar la API", appleCampo(R, "identidades", i, "gets"), `[]`)
		})
	}
}

func TestApplePerfilesElValidadorCompartidoCompruebaTodosLosDerechosReales(t *testing.T) {
	// apple_perfil_test_fixtures.py sustituye solo el límite CMS (los fixtures
	// heredados son plist sin firma); el validador de derechos es el real.
	R := applePerfiles(t, `
contenido = {"Platform": ["iOS"], "TeamIdentifier": ["VHC5QW7BZ3"],
             "ExpirationDate": datetime.datetime(2050, 1, 1),
             "Entitlements": {"application-identifier": "VHC5QW7BZ3.eu.sytue.rooms",
                              "com.apple.security.application-groups": [GRUPO],
                              "keychain-access-groups": ["VHC5QW7BZ3.*"]}}
item = perfil()
item["attributes"]["profileContent"] = base64.b64encode(plistlib.dumps(contenido)).decode()
with tempfile.TemporaryDirectory() as tmp:
    python = pathlib.Path(tmp, "python3")
    python.write_text('#!/bin/sh\nexec ' + shlex.quote(sys.executable) + ' ' +
                      shlex.quote(str(AQUI / "apple_perfil_test_fixtures.py")) + ' "$@"\n')
    python.chmod(0o755)
    with mock.patch.dict(os.environ, {"PATH": tmp + os.pathsep + os.environ["PATH"]}):
        R["completo"] = p.validar_perfil(item, REQUISITO, "ios") is not None
        incompleto = {**REQUISITO, "entitlements": {**REQUISITO["entitlements"],
                      "com.apple.security.application-groups": [GRUPO, "group.eu.sytue.drive"]}}
        R["grupo_extra"] = p.validar_perfil(item, incompleto, "ios") is None
        contenido["Entitlements"]["keychain-access-groups"] = ["VHC5QW7BZ3.eu.sytue.rooms", LLAVERO]
        item["attributes"]["profileContent"] = base64.b64encode(plistlib.dumps(contenido)).decode()
        claves = {**REQUISITO, "entitlements": {**REQUISITO["entitlements"],
                  "keychain-access-groups": REQUISITO["entitlements"]["keychain-access-groups"] + ["VHC5QW7BZ3.eu.sytue.drive.shared"]}}
        R["llavero_extra"] = p.validar_perfil(item, claves, "ios") is None
        contenido["Entitlements"]["application-identifier"] = "VHC5QW7BZ3.eu.sytue.rooms.widgets"
        contenido["Entitlements"]["com.apple.developer.default-data-protection"] = "NSFileProtectionComplete"
        contenido["Entitlements"]["keychain-access-groups"] = ["VHC5QW7BZ3.*"]
        item["attributes"]["profileContent"] = base64.b64encode(plistlib.dumps(contenido)).decode()
        widget = {**REQUISITO, "id": "eu.sytue.rooms.widgets", "keychain": "", "protection": "NSFileProtectionComplete"}
        R["widget"] = p.validar_perfil(item, widget, "ios") is None
`)
	appleIgual(t, "el perfil con todos los derechos vale", R["completo"], `true`)
	appleIgual(t, "un AppGroup pedido que no autoriza", R["grupo_extra"], `true`)
	appleIgual(t, "un grupo de llavero pedido que no autoriza", R["llavero_extra"], `true`)
	appleIgual(t, "un widget sin llavero frente a un perfil de host", R["widget"], `true`)
}

func TestApplePerfilesUnaIdentidadQueFindIdentityVOmiteSoloValeSiVerifyCertLocalPasa(t *testing.T) {
	R := applePerfiles(t, `
selector, otra = "a" * 40, "b" * 40
der = b"developer id g2 publico"
candidata = ' 1) ' + selector + ' "Developer ID Application: Sytue (VHC5QW7BZ3)" (CSSMERR_TP_NOT_TRUSTED)\n     0 valid identities found'
pem_exportado = exportado(der, selector)
R["errores"] = []
with tempfile.TemporaryDirectory() as job_dir:
    llavero = str(pathlib.Path(job_dir, "firma.keychain-db"))
    vistos_cert = []
    def capturar_valido(argv):
        cmd = [str(a) for a in argv]
        if cmd[-1] == "llavero-efimero":
            return llavero + "\n"
        if cmd == ["security", "find-identity", "-v", "-p", "codesigning", llavero]:
            return "     0 valid identities found\n"
        if cmd == ["security", "find-identity", "-p", "codesigning", llavero]:
            return candidata
        if cmd == ["security", "find-certificate", "-a", "-Z", "-p", llavero]:
            return pem_exportado
        if cmd[:5] == ["security", "verify-cert", "-L", "-p", "codeSign"]:
            ruta_c = pathlib.Path(cmd[cmd.index("-c") + 1])
            R["errores"] += [] if ruta_c.parent == pathlib.Path(job_dir) else ["certificado fuera del job: " + str(ruta_c)]
            R["errores"] += [] if cmd[cmd.index("-k") + 1] == llavero else ["verify-cert fuera del llavero efímero"]
            R["errores"] += [] if "-----BEGIN CERTIFICATE-----" in ruta_c.read_text() else ["verify-cert sin PEM"]
            vistos_cert.append(ruta_c)
            return ""
        raise AssertionError("orden inesperada: " + repr(cmd))
    with mock.patch.dict(os.environ, {"SYTUE_APPLE_JOB_DIR": job_dir}), \
            mock.patch.object(p, "capturar", side_effect=capturar_valido):
        R["valida"] = p.identidad_importada("developerid", "VHC5QW7BZ3") == der
    R["vistos"] = len(vistos_cert)
    R["temporal_borrado"] = not vistos_cert[0].exists()
    def capturar_rechazado(argv):
        cmd = [str(a) for a in argv]
        if cmd[:2] == ["security", "verify-cert"]:
            vistos_cert.append(pathlib.Path(cmd[cmd.index("-c") + 1]))
            raise p.subprocess.CalledProcessError(1, cmd)
        return capturar_valido(argv)
    with mock.patch.dict(os.environ, {"SYTUE_APPLE_JOB_DIR": job_dir}), \
            mock.patch.object(p, "capturar", side_effect=capturar_rechazado):
        R["rechazada"] = atrapar(p.identidad_importada, "developerid", "VHC5QW7BZ3")
    R["temporal_borrado2"] = not vistos_cert[-1].exists()
    ambiguas = candidata + '\n 2) ' + otra + ' "Developer ID Application: Otro (VHC5QW7BZ3)" (CSSMERR_TP_NOT_TRUSTED)'
    with mock.patch.dict(os.environ, {"SYTUE_APPLE_JOB_DIR": job_dir}), \
            mock.patch.object(p, "capturar", side_effect=[llavero + "\n", "     0 valid identities found\n", ambiguas]):
        R["ambiguas"] = atrapar(p.identidad_importada, "developerid", "VHC5QW7BZ3")
`)
	appleIgual(t, "comprobaciones de verify-cert", R["errores"], `[]`)
	appleIgual(t, "verify-cert la acepta", R["valida"], `true`)
	appleIgual(t, "un único verify-cert", R["vistos"], `1`)
	appleIgual(t, "el PEM temporal se borra", R["temporal_borrado"], `true`)
	appleExc(t, "verify-cert la rechaza", R["rechazada"], "ValueError", "identidad válida única")
	appleIgual(t, "el PEM temporal se borra también al rechazar", R["temporal_borrado2"], `true`)
	appleExc(t, "dos candidatas", R["ambiguas"], "ValueError", "identidad válida única")
}
