// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package pruebasfyne

import (
	"encoding/json"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// Auditoría y notas de App Store Connect (appstore-publicar.py) contra una API
// simulada, sin red ni credenciales reales. El doble de la API vive en proceso
// (Python), como en el unittest original; lo observado vuelve en JSON y las
// comprobaciones son de Go.

// appstorePreparacionDobles son los dobles de la API: `API` responde la
// auditoría solo con GET, `APINotas` deja escribir únicamente las notas de
// revisión tras el preflight global y `Escribible` simula la disponibilidad por
// territorios. PRIVADO es un dato que nunca debe salir por stdout ni stderr.
const appstorePreparacionDobles = `
import contextlib, copy, io
from unittest import mock
from urllib.parse import parse_qs, urlsplit
sys.path.insert(0, str(SCRIPTS))
asc = cargar("appstore_publicar", SCRIPTS / "appstore-publicar.py")
PRIVADO = "dato-del-revisor-no-imprimir"


class API:
    def __init__(self):
        self.rutas = []
        self.datos = {
            "/v1/apps/app": {"attributes": {"bundleId": asc.bundle_de("rooms"), "name": "Sytue Rooms", "primaryLocale": "es-ES", "contentRightsDeclaration": "DOES_NOT_USE_THIRD_PARTY_CONTENT"}},
            "/v1/reviewSubmissions": [],
            "/v1/apps/app/appInfos": [{"id": "info", "attributes": {"appStoreState": "PREPARE_FOR_SUBMISSION"}}],
            "/v1/appInfos/info/appInfoLocalizations": [{"id": "loc", "attributes": {"locale": "es-ES", "name": PRIVADO, "privacyPolicyUrl": PRIVADO}}],
            "/v1/appInfos/info/primaryCategory": {"id": "PRODUCTIVITY"},
            "/v1/appInfos/info/ageRatingDeclaration": {"id": "age"},
            "/v1/appPriceSchedules/app/manualPrices": [{"id": "price"}],
            "/v1/apps/app/appAvailabilityV2": {"id": "available"},
            "/v1/apps/app/appStoreVersions": [{"id": "old", "attributes": {"versionString": "3.12.10", "appStoreState": "REJECTED", "copyright": PRIVADO}}],
            "/v1/appStoreVersions/old/appStoreVersionLocalizations": [{"id": "version-loc", "attributes": {"locale": "es-ES", "description": PRIVADO, "keywords": PRIVADO, "supportUrl": PRIVADO}}],
            "/v1/appStoreVersionLocalizations/version-loc/appScreenshotSets": [{"id": tipo, "attributes": {"screenshotDisplayType": tipo}} for tipo, _, _ in asc.CAPTURAS.values()],
            "/v1/appStoreVersions/old/appStoreReviewDetail": {"attributes": {**{campo: PRIVADO for campo, _ in asc.DATOS_REVISION}, "demoAccountRequired": True, "notes": PRIVADO}},
        }
        for tipo, _, _ in asc.CAPTURAS.values():
            self.datos[f"/v1/appScreenshotSets/{tipo}/appScreenshots"] = [{"attributes": {"assetDeliveryState": {"state": "COMPLETE"}}}]

    def pedir(self, metodo, ruta, cuerpo=None, tolerar=()):
        assert metodo == "GET" and cuerpo is None, "la auditoría intentó escribir"
        self.rutas.append(ruta)
        base = ruta.split("?")[0]
        if base.startswith("/v1/appStoreVersions/") and base.endswith("/build") and "include=" in ruta:
            raise RuntimeError("HTTP 400: The parameter 'include' can not be used with this request")
        assert base in self.datos, base
        datos = copy.deepcopy(self.datos[base])
        # Apple omite el enlace de versión si sólo se pide el campo; include es
        # necesario para identificar qué versión originó un rechazo pendiente.
        if base.startswith("/v1/reviewSubmissions/") and base.endswith("/items") and "include=appStoreVersion" not in ruta:
            for item in datos:
                item.pop("relationships", None)
        return {"data": datos} if datos is not None else None

    def lista(self, ruta):
        return self.pedir("GET", ruta)["data"]


class APINotas(API):
    def __init__(self, apps=("rooms",), plataformas=("IOS", "MAC_OS")):
        super().__init__()
        self.escrituras, self.confirmar = [], True
        self.total = len(apps) * len(plataformas)
        self.ficha = {"revision": {"notas": "común", "notas_por_plataforma": {"MAC_OS": "listener local"}}, "apps": {}}
        for app in apps:
            ident = app + "-app"
            self.ficha["apps"][app] = {"id": ident, "notas": "detalle de " + app}
            self.datos[f"/v1/apps/{ident}"] = {"attributes": {"bundleId": asc.bundle_de(app)}}
            versiones = []
            for plataforma in plataformas:
                vid = app + "-" + plataforma
                versiones.append({"id": "nueva-" + vid, "attributes": {"versionString": "3.14.1", "platform": plataforma, "appStoreState": "PREPARE_FOR_SUBMISSION"}})
                versiones.append({"id": vid, "attributes": {"versionString": "3.14.0", "platform": plataforma, "appStoreState": "PREPARE_FOR_SUBMISSION"}})
                self.datos[f"/v1/appStoreVersions/{vid}/appStoreReviewDetail"] = {"id": "rd-" + vid, "attributes": {**{campo: PRIVADO for campo, _ in asc.DATOS_REVISION}, "demoAccountRequired": True, "notes": PRIVADO}}
            self.datos[f"/v1/apps/{ident}/appStoreVersions"] = versiones

    def lista(self, ruta):
        datos = super().lista(ruta)
        if urlsplit(ruta).path.endswith("/appStoreVersions"):
            plataforma = parse_qs(urlsplit(ruta).query)["filter[platform]"][0]
            return [v for v in datos if v["attributes"]["platform"] == plataforma]
        return datos

    def escribir(self, tipo, attrs=None, rels=None, id=None, ruta=None):
        assert tipo == "appStoreReviewDetails" and set(attrs) == {"notes"} and id and rels is None and ruta is None
        revisadas = {r.split("?")[0] for r in self.rutas if "/appStoreReviewDetail" in r}
        assert len(revisadas) == self.total, "se escribió antes del preflight global"
        self.escrituras.append([id, copy.deepcopy(attrs)])
        for data in self.datos.values():
            if isinstance(data, dict) and data.get("id") == id:
                data["attributes"]["notes"] = attrs["notes"] if self.confirmar else PRIVADO
                return {"data": copy.deepcopy(data)}
        raise AssertionError("se escribe otro detalle")


def ejecutar(api, apps=("rooms",), plataformas=("IOS", "MAC_OS"), version="3.14.0"):
    """El modo notas: devuelve el código, stdout, stderr y si se filtró PRIVADO."""
    out, err = io.StringIO(), io.StringIO()

    class Entorno(dict):
        def get(self, key, default=None):
            if key.startswith(("APPSTORE_CONTACTO_", "APPSTORE_DEMO_")):
                raise AssertionError("el modo notas leyó datos privados del entorno")
            return super().get(key, default)

    with contextlib.redirect_stdout(out), contextlib.redirect_stderr(err), mock.patch.object(asc.os, "environ", Entorno()):
        resultado = asc.actualizar_notas(api, list(apps), api.ficha, list(plataformas), version)
    return {"resultado": resultado, "out": out.getvalue(), "err": err.getvalue(),
            "privado": PRIVADO in out.getvalue() + err.getvalue(), "escrituras": api.escrituras, "rutas": api.rutas}


def sondear(api, plataformas=("IOS", "MAC_OS")):
    """La auditoría de preparación: devuelve cada línea JSON y si se filtró PRIVADO."""
    salida = io.StringIO()
    error = None
    with contextlib.redirect_stdout(salida):
        try:
            asc.preparacion(api, "rooms", {"apps": {"rooms": {"id": "app"}}}, plataformas, "3.13.5")
        except Exception as e:
            error = excepcion(e)
    lineas = [json.loads(l) for l in salida.getvalue().splitlines()] if error is None else []
    return {"lineas": lineas, "privado": PRIVADO in salida.getvalue(), "rutas": api.rutas, "error": error}


TERRITORIOS = ["ESP", "DEU", "USA", "JPN", "BRA", "FRA", "CHN", "RUS", "BLR"] + [f"T{n:02d}" for n in range(100)]


class Escribible:
    def __init__(self, filas, automatico=False, rechaza=()):
        self.filas = filas
        self.automatico = automatico
        self.rechaza = set(rechaza)
        self.parches = []

    def pedir(self, metodo, ruta, cuerpo=None, tolerar=()):
        base = ruta.split("?")[0]
        if base == "/v1/apps/app":
            return {"data": {"attributes": {"bundleId": asc.bundle_de("rooms")}}}
        if base == "/v1/apps/app/appAvailabilityV2":
            return {"data": {"id": "disp", "attributes": {"availableInNewTerritories": self.automatico}}}
        raise AssertionError(base)

    def lista(self, ruta):
        if ruta.startswith("/v1/territories"):
            return [{"id": t} for t in TERRITORIOS]
        assert ruta.startswith("/v2/appAvailabilities/disp/territoryAvailabilities"), ruta
        return copy.deepcopy(self.filas)

    def escribir(self, tipo, attrs=None, rels=None, id=None, ruta=None):
        assert tipo == "territoryAvailabilities" and id and set(attrs) == {"available"}
        territorio = next(f for f in self.filas if f["id"] == id)["relationships"]["territory"]["data"]["id"]
        if territorio in self.rechaza:
            raise RuntimeError("HTTP 409 PATCH /v1/territoryAvailabilities: rechazado")
        self.parches.append([territorio, attrs["available"]])
        next(f for f in self.filas if f["id"] == id)["attributes"]["available"] = attrs["available"]
        return {}


def filas_ue():
    ue = {"ESP", "DEU"}
    return [{"id": "fila-" + t, "attributes": {"available": t in ue},
             "relationships": {"territory": {"data": {"type": "territories", "id": t}}}} for t in TERRITORIOS]


def paises(api):
    """ajustar_paises con stdout silenciado; devuelve la excepción si la hubo."""
    with contextlib.redirect_stdout(io.StringIO()):
        return fallo(asc.ajustar_paises, api, "rooms", {"apps": {"rooms": {"id": "app"}}}, ["IOS"], "")
`

// appstorePreparacion corre un fragmento con los dobles cargados.
func appstorePreparacion(t *testing.T, fragmento string, v any) {
	t.Helper()
	pyRestoCorrer(t, appstorePreparacionDobles+fragmento, v)
}

// --- Notas de revisión -------------------------------------------------------

func TestAppstorePreparacionNotasMacNoContaminaIOSYConservaLasDeLaApp(t *testing.T) {
	var r []string
	appstorePreparacion(t, `
ficha = {"revision": {"notas": "común", "notas_por_plataforma": {"MAC_OS": "listener local"}}}
datos = {"notas": "detalle de la app"}
devolver([asc.notas_revision(ficha, datos, "IOS"), asc.notas_revision(ficha, datos, "MAC_OS")])
`, &r)
	if r[0] != "común\n\ndetalle de la app" || r[1] != "común\n\ndetalle de la app\n\nlistener local" {
		t.Fatalf("notas IOS %q y MAC_OS %q", r[0], r[1])
	}
}

func TestAppstorePreparacionNotas4000IncluyeElAnexoYLosSeparadores(t *testing.T) {
	var r struct {
		Mac   int               `json:"mac"`
		Error *pyRestoExcepcion `json:"error"`
		IOS   int               `json:"ios"`
	}
	appstorePreparacion(t, `
ficha = {"revision": {"notas": "x" * 3995, "notas_por_plataforma": {"MAC_OS": "mac"}}}
mac = len(asc.notas_revision(ficha, {}, "MAC_OS"))
ficha["revision"]["notas_por_plataforma"]["MAC_OS"] += "x"
devolver({"mac": mac, "error": fallo(asc.notas_revision, ficha, {}, "MAC_OS"), "ios": len(asc.notas_revision(ficha, {}, "IOS"))})
`, &r)
	if r.Mac != 4000 {
		t.Fatalf("las notas de MAC_OS miden %d, no 4000", r.Mac)
	}
	pyRestoLanza(t, r.Error, "RuntimeError", "MAC_OS.*4000")
	if r.IOS != 3995 {
		t.Fatalf("las notas de IOS miden %d, no 3995", r.IOS)
	}
}

func TestAppstorePreparacionNotasConservanElFormatoAnteriorSinAnexos(t *testing.T) {
	var r string
	appstorePreparacion(t, `
devolver(asc.notas_revision({"revision": {"notas": " común "}}, {"notas": " app "}, "IOS"))
`, &r)
	if r != "común\n\napp" {
		t.Fatalf("notas %q", r)
	}
}

func TestAppstorePreparacionNotasRechazanPlataformaOTextoMalConfigurados(t *testing.T) {
	for nombre, porPlataforma := range map[string]string{
		"ANDROID": `{"ANDROID": "otro"}`, "None": `{"MAC_OS": None}`, "cadena": `"mac"`,
	} {
		t.Run(nombre, func(t *testing.T) {
			var r *pyRestoExcepcion
			appstorePreparacion(t, `
devolver(fallo(asc.notas_revision, {"revision": {"notas": "común", "notas_por_plataforma": `+porPlataforma+`}}, {}, "MAC_OS"))
`, &r)
			pyRestoLanza(t, r, "RuntimeError", "")
		})
	}
}

// --- Solo notas ----------------------------------------------------------------

type appstorePreparacionNotas struct {
	Resultado  int              `json:"resultado"`
	Out        string           `json:"out"`
	Err        string           `json:"err"`
	Privado    bool             `json:"privado"`
	Escrituras [][]any          `json:"escrituras"`
	Rutas      []string         `json:"rutas"`
	Detalles   []map[string]any `json:"detalles"`
	Campos     []string         `json:"campos"`
}

func (n appstorePreparacionNotas) comprobarPrivado(t *testing.T) {
	t.Helper()
	if n.Privado {
		t.Fatalf("el modo notas imprimió un dato privado\n%s%s", n.Out, n.Err)
	}
}

func TestAppstorePreparacionNotas30DestinosExactosSinTocarCredencialesBuildsORevisiones(t *testing.T) {
	var r appstorePreparacionNotas
	appstorePreparacion(t, `
apps = asc.las_quince()
api = APINotas(apps)
r = ejecutar(api, apps)
r["detalles"] = [d["attributes"] for d in api.datos.values() if isinstance(d, dict) and str(d.get("id", "")).startswith("rd-")]
r["campos"] = [campo for campo, _ in asc.DATOS_REVISION]
devolver(r)
`, &r)
	r.comprobarPrivado(t)
	if r.Resultado != 0 {
		t.Fatalf("código %d\n%s", r.Resultado, r.Err)
	}
	if len(r.Escrituras) != 30 {
		t.Fatalf("%d escrituras, no 30", len(r.Escrituras))
	}
	lineas := strings.Split(strings.TrimRight(r.Out, "\n"), "\n")
	if len(lineas) != 30 {
		t.Fatalf("%d líneas de resumen, no 30", len(lineas))
	}
	for _, l := range lineas {
		var x struct {
			NotesEqual  bool   `json:"notes_equal"`
			NotesSHA256 string `json:"notes_sha256"`
		}
		if err := json.Unmarshal([]byte(l), &x); err != nil {
			t.Fatal(err)
		}
		if !x.NotesEqual || len(x.NotesSHA256) != 64 {
			t.Fatalf("resumen sin confirmar: %s", l)
		}
	}
	for _, e := range r.Escrituras {
		if strings.Contains(e[0].(string), "nueva-") {
			t.Fatalf("escribió en la versión nueva: %v", e)
		}
	}
	for _, ruta := range r.Rutas {
		if strings.Contains(ruta, "/build") || strings.Contains(ruta, "reviewSubmissions") {
			t.Fatalf("pidió %s", ruta)
		}
		if strings.Contains(ruta, "/appStoreReviewDetail") && !strings.Contains(ruta, "fields[appStoreReviewDetails]=notes") {
			t.Fatalf("leyó el detalle de revisión entero: %s", ruta)
		}
	}
	if len(r.Detalles) == 0 {
		t.Fatal("no hay detalles de revisión que comprobar")
	}
	for _, d := range r.Detalles {
		if d["demoAccountRequired"] != true {
			t.Fatalf("cambió demoAccountRequired: %v", d)
		}
		for _, c := range r.Campos {
			if d[c] != "dato-del-revisor-no-imprimir" {
				t.Fatalf("cambió el campo %s: %v", c, d)
			}
		}
	}
}

func TestAppstorePreparacionNotasPlataformaSeleccionadaYSinGuardaDeSubmission(t *testing.T) {
	var r appstorePreparacionNotas
	appstorePreparacion(t, `
api = APINotas(plataformas=("IOS",))
api.datos["/v1/reviewSubmissions"] = [{"attributes": {"state": "UNRESOLVED_ISSUES"}}]
devolver(ejecutar(api, plataformas=("IOS",)))
`, &r)
	r.comprobarPrivado(t)
	if r.Resultado != 0 {
		t.Fatalf("código %d\n%s", r.Resultado, r.Err)
	}
	if len(r.Escrituras) != 1 || r.Escrituras[0][0] != "rd-rooms-IOS" {
		t.Fatalf("escrituras %v, no [rd-rooms-IOS]", r.Escrituras)
	}
}

func TestAppstorePreparacionNotasPreflightGlobalFallaSinVersionDetalleOEstadoEditable(t *testing.T) {
	for _, fallo := range []string{"version", "ambigua", "detalle", "estado"} {
		t.Run(fallo, func(t *testing.T) {
			var r appstorePreparacionNotas
			appstorePreparacion(t, `
fallo_ = `+"\""+fallo+"\""+`
apps = asc.las_quince()
api = APINotas(apps)
ultima = apps[-1]
versiones = api.datos[f"/v1/apps/{ultima}-app/appStoreVersions"]
if fallo_ == "version":
    api.datos[f"/v1/apps/{ultima}-app/appStoreVersions"] = [v for v in versiones if v["attributes"]["platform"] != "MAC_OS"]
elif fallo_ == "ambigua":
    versiones.append(copy.deepcopy(next(v for v in versiones if v["id"] == ultima + "-MAC_OS")))
elif fallo_ == "detalle":
    api.datos[f"/v1/appStoreVersions/{ultima}-MAC_OS/appStoreReviewDetail"] = None
else:
    next(v for v in versiones if v["id"] == ultima + "-MAC_OS")["attributes"]["appStoreState"] = "IN_REVIEW"
devolver(ejecutar(api, apps))
`, &r)
			r.comprobarPrivado(t)
			if r.Resultado != 1 || len(r.Escrituras) != 0 {
				t.Fatalf("código %d y escrituras %v; se esperaba 1 y ninguna", r.Resultado, r.Escrituras)
			}
		})
	}
}

func TestAppstorePreparacionNotasYaIgualesNoSeReescriben(t *testing.T) {
	var r appstorePreparacionNotas
	appstorePreparacion(t, `
api = APINotas(plataformas=("MAC_OS",))
esperado = asc.notas_revision(api.ficha, api.ficha["apps"]["rooms"], "MAC_OS")
api.datos["/v1/appStoreVersions/rooms-MAC_OS/appStoreReviewDetail"]["attributes"]["notes"] = esperado
devolver(ejecutar(api, plataformas=("MAC_OS",)))
`, &r)
	r.comprobarPrivado(t)
	var x struct {
		Changed    *bool `json:"changed"`
		NotesEqual bool  `json:"notes_equal"`
	}
	if err := json.Unmarshal([]byte(r.Out), &x); err != nil {
		t.Fatalf("resumen ilegible: %v\n%s", err, r.Out)
	}
	if r.Resultado != 0 || len(r.Escrituras) != 0 || x.Changed == nil || *x.Changed || !x.NotesEqual {
		t.Fatalf("código %d, escrituras %v, resumen %s", r.Resultado, r.Escrituras, r.Out)
	}
}

func TestAppstorePreparacionNotasRecompruebaEstadoDestinoYSaneaErroresAntesDelPatch(t *testing.T) {
	for _, fallo := range []string{"estado", "version_id", "detalle_id", "error"} {
		t.Run(fallo, func(t *testing.T) {
			var r appstorePreparacionNotas
			appstorePreparacion(t, `
fallo_ = `+"\""+fallo+"\""+`
api = APINotas(plataformas=("IOS",))
original = api.lista
llamadas = 0

def lista(ruta):
    global llamadas
    llamadas += 1
    if llamadas == 2:
        v = api.datos["/v1/apps/rooms-app/appStoreVersions"][1]
        if fallo_ == "estado":
            v["attributes"]["appStoreState"] = "WAITING_FOR_REVIEW"
        elif fallo_ == "version_id":
            v["id"] = "otro-id"
            api.datos["/v1/appStoreVersions/otro-id/appStoreReviewDetail"] = {"id": "otro-detalle"}
        elif fallo_ == "detalle_id":
            api.datos["/v1/appStoreVersions/rooms-IOS/appStoreReviewDetail"]["id"] = "otro-detalle"
        else:
            raise RuntimeError(PRIVADO)
    return original(ruta)

api.lista = lista
devolver(ejecutar(api, plataformas=("IOS",)))
`, &r)
			r.comprobarPrivado(t)
			if r.Resultado != 1 || len(r.Escrituras) != 0 {
				t.Fatalf("código %d y escrituras %v; se esperaba 1 y ninguna", r.Resultado, r.Escrituras)
			}
		})
	}
}

func TestAppstorePreparacionNotasReadbackNoConfirmadoEsRojoSinVolcarNotas(t *testing.T) {
	var r appstorePreparacionNotas
	appstorePreparacion(t, `
api = APINotas(plataformas=("IOS",))
api.confirmar = False
devolver(ejecutar(api, plataformas=("IOS",)))
`, &r)
	r.comprobarPrivado(t)
	var x struct {
		NotesEqual *bool `json:"notes_equal"`
	}
	if err := json.Unmarshal([]byte(r.Out), &x); err != nil {
		t.Fatalf("resumen ilegible: %v\n%s", err, r.Out)
	}
	if r.Resultado != 1 || x.NotesEqual == nil || *x.NotesEqual {
		t.Fatalf("código %d y resumen %s; se esperaba 1 y notes_equal=false", r.Resultado, r.Out)
	}
}

func TestAppstorePreparacionNotasLimiteUTF8DelAnexoMacNoEsSoloNumeroDeCaracteres(t *testing.T) {
	var r struct {
		Bytes int               `json:"bytes"`
		Error *pyRestoExcepcion `json:"error"`
	}
	appstorePreparacion(t, `
ficha = {"revision": {"notas": "ñ" * 2000}}
n = len(asc.notas_revision(ficha, {}, "MAC_OS").encode("utf-8"))
ficha["revision"]["notas"] += "ñ"
devolver({"bytes": n, "error": fallo(asc.notas_revision, ficha, {}, "MAC_OS")})
`, &r)
	if r.Bytes != 4000 {
		t.Fatalf("%d bytes, no 4000", r.Bytes)
	}
	pyRestoLanza(t, r.Error, "RuntimeError", "MAC_OS.*4000")
}

func TestAppstorePreparacionNotasRequiereVersionAntesDeCredenciales(t *testing.T) {
	var r struct {
		Codigo int    `json:"codigo"`
		Err    string `json:"err"`
	}
	appstorePreparacion(t, `
out = io.StringIO()
with contextlib.redirect_stderr(out), mock.patch.object(asc, "Apple", side_effect=AssertionError("leyó credenciales")):
    codigo = asc.main(["--notas", "rooms"])
devolver({"codigo": codigo, "err": out.getvalue()})
`, &r)
	if r.Codigo != 2 || !strings.Contains(r.Err, "notas requiere --version") {
		t.Fatalf("código %d, stderr %q", r.Codigo, r.Err)
	}
}

// --- Auditoría de preparación --------------------------------------------------

type appstorePreparacionSondeo struct {
	Lineas  []map[string]any  `json:"lineas"`
	Privado bool              `json:"privado"`
	Rutas   []string          `json:"rutas"`
	Error   *pyRestoExcepcion `json:"error"`
}

// appstorePreparacionSondear prepara la API con `ajuste` y devuelve las líneas
// de la auditoría, comprobando que no se filtró nada privado.
func appstorePreparacionSondear(t *testing.T, ajuste, plataformas string) appstorePreparacionSondeo {
	t.Helper()
	var r appstorePreparacionSondeo
	appstorePreparacion(t, "\napi = API()\n"+ajuste+"\ndevolver(sondear(api, "+plataformas+"))\n", &r)
	if r.Privado {
		t.Fatal("la auditoría imprimió un dato privado")
	}
	return r
}

func appstorePreparacionMapa(t *testing.T, v any, clave string) map[string]any {
	t.Helper()
	m, ok := v.(map[string]any)[clave].(map[string]any)
	if !ok {
		t.Fatalf("falta %s en %v", clave, v)
	}
	return m
}

func appstorePreparacionTodos(m map[string]any, valor bool) bool {
	for _, v := range m {
		if (v != nil && v != false && v != "" && v != 0.0) != valor {
			return false
		}
	}
	return true
}

func TestAppstorePreparacionReutilizaSinCrearVersionYNoExponeDatos(t *testing.T) {
	r := appstorePreparacionSondear(t, "", `("IOS", "MAC_OS")`)
	if r.Error != nil || len(r.Lineas) != 2 {
		t.Fatalf("auditoría: %v, %d líneas", r.Error, len(r.Lineas))
	}
	ios, mac := r.Lineas[0], r.Lineas[1]
	if ios["sourceVersion"] != "3.12.10" || ios["targetVersionExists"] != false ||
		ios["canonicalScreenshotsComplete"] != true || mac["canonicalScreenshotsComplete"] != true {
		t.Fatalf("auditoría inesperada: %v / %v", ios, mac)
	}
	if !appstorePreparacionTodos(appstorePreparacionMapa(t, ios, "reviewDetails"), true) {
		t.Fatalf("faltan datos de revisión: %v", ios["reviewDetails"])
	}
	if ios["privacyLabels"] != "CHECK_IN_APP_STORE_CONNECT" || appstorePreparacionMapa(t, ios, "common")["ageRating"] != true {
		t.Fatalf("privacyLabels o ageRating inesperados: %v", ios)
	}
}

func TestAppstorePreparacionDistingueRechazoPendienteYBuildAntiguoSinDatosPrivados(t *testing.T) {
	r := appstorePreparacionSondear(t, `
api.datos["/v1/reviewSubmissions"] = [{"id": "submission", "attributes": {"state": "UNRESOLVED_ISSUES", "userEmail": PRIVADO}}]
api.datos["/v1/reviewSubmissions/submission/items"] = [{"id": "item", "attributes": {"state": "REJECTED", "privateFeedback": PRIVADO}, "relationships": {"appStoreVersion": {"data": {"id": "old"}}}}]
api.datos["/v1/appStoreVersions/old/build"] = {"id": "build", "attributes": {"version": "12", "processingState": "VALID"}}
api.datos["/v1/builds/build/preReleaseVersion"] = {"attributes": {"version": "3.12.10", "private": PRIVADO}}
`, `("IOS",)`)
	if r.Error != nil {
		t.Fatal(r.Error.Mensaje)
	}
	datos := r.Lineas[0]
	revision := datos["reviewSubmissions"].([]any)[0].(map[string]any)
	if revision["state"] != "UNRESOLVED_ISSUES" {
		t.Fatalf("estado %v", revision["state"])
	}
	esperado := []any{map[string]any{"state": "REJECTED", "version": "3.12.10", "versionState": "REJECTED", "build": "12", "buildState": "VALID", "buildVersion": "3.12.10"}}
	if !reflect.DeepEqual(revision["items"], esperado) {
		t.Fatalf("items %v, no %v", revision["items"], esperado)
	}
	if datos["reviewFeedback"] != "CHECK_IN_APP_STORE_CONNECT" {
		t.Fatalf("reviewFeedback %v", datos["reviewFeedback"])
	}
	for _, ruta := range r.Rutas {
		if strings.Contains(ruta, "submittedByActor") || strings.Contains(ruta, "lastUpdatedByActor") {
			t.Fatalf("pidió quién lo envió: %s", ruta)
		}
	}
	if !slices.Contains(r.Rutas, "/v1/builds/build/preReleaseVersion?fields[preReleaseVersions]=version,platform") {
		t.Fatalf("no pidió la versión del build con sus campos: %v", r.Rutas)
	}
}

func TestAppstorePreparacionReviewSinBuildOConItemNoVersionNoSeInventa(t *testing.T) {
	r := appstorePreparacionSondear(t, `
api.datos["/v1/reviewSubmissions"] = [{"id": "submission", "attributes": {"state": "WAITING_FOR_REVIEW"}}]
api.datos["/v1/reviewSubmissions/submission/items"] = [{"attributes": {"state": "ACCEPTED"}, "relationships": {"appStoreVersion": {"data": {"id": "old"}}}}, {"attributes": {"state": "READY_FOR_REVIEW"}}]
api.datos["/v1/appStoreVersions/old/build"] = None
`, `("IOS",)`)
	if r.Error != nil {
		t.Fatal(r.Error.Mensaje)
	}
	items := r.Lineas[0]["reviewSubmissions"].([]any)[0].(map[string]any)["items"].([]any)
	primero := items[0].(map[string]any)
	if v, ok := primero["build"]; !ok || v != nil {
		t.Fatalf("build inventado: %v", primero)
	}
	if v, ok := primero["buildVersion"]; !ok || v != nil {
		t.Fatalf("buildVersion inventado: %v", primero)
	}
	if !reflect.DeepEqual(items[1], map[string]any{"state": "READY_FOR_REVIEW"}) {
		t.Fatalf("segundo item %v", items[1])
	}
}

func TestAppstorePreparacionCapturaEnProcesoNoEsLista(t *testing.T) {
	r := appstorePreparacionSondear(t, `
api.datos["/v1/appScreenshotSets/APP_IPHONE_61/appScreenshots"][0]["attributes"]["assetDeliveryState"]["state"] = "UPLOAD_COMPLETE"
`, `("IOS", "MAC_OS")`)
	if r.Error != nil {
		t.Fatal(r.Error.Mensaje)
	}
	if r.Lineas[0]["canonicalScreenshotsComplete"] != false || r.Lineas[1]["canonicalScreenshotsComplete"] != true {
		t.Fatalf("capturas: IOS %v, MAC_OS %v", r.Lineas[0]["canonicalScreenshotsComplete"], r.Lineas[1]["canonicalScreenshotsComplete"])
	}
}

func TestAppstorePreparacionSinInfoVersionPrecioODisponibilidad(t *testing.T) {
	r := appstorePreparacionSondear(t, `
api.datos["/v1/apps/app/appInfos"] = []
api.datos["/v1/apps/app/appStoreVersions"] = []
api.datos["/v1/appPriceSchedules/app/manualPrices"] = None
api.datos["/v1/apps/app/appAvailabilityV2"] = None
`, `("IOS",)`)
	if r.Error != nil {
		t.Fatal(r.Error.Mensaje)
	}
	ios := r.Lineas[0]
	if v, ok := ios["sourceVersion"]; !ok || v != nil {
		t.Fatalf("sourceVersion %v", v)
	}
	comun := appstorePreparacionMapa(t, ios, "common")
	if comun["appInfo"] != false || comun["availability"] != false || comun["manualPrice"] != false {
		t.Fatalf("common %v", comun)
	}
}

func TestAppstorePreparacionNoInventaContactoNiCuenta(t *testing.T) {
	r := appstorePreparacionSondear(t, `
api.datos["/v1/appStoreVersions/old/appStoreReviewDetail"] = None
`, `("IOS",)`)
	if r.Error != nil {
		t.Fatal(r.Error.Mensaje)
	}
	datos := r.Lineas[0]
	if !appstorePreparacionTodos(appstorePreparacionMapa(t, datos, "reviewDetails"), false) {
		t.Fatalf("inventó datos de revisión: %v", datos["reviewDetails"])
	}
	if v, ok := datos["demoAccountRequired"]; !ok || v != nil {
		t.Fatalf("demoAccountRequired %v", v)
	}
}

func TestAppstorePreparacionIDDeOtraAppSeRechaza(t *testing.T) {
	r := appstorePreparacionSondear(t, `
api.datos["/v1/apps/app"]["attributes"]["bundleId"] = "eu.otra.app"
`, `("IOS", "MAC_OS")`)
	pyRestoLanza(t, r.Error, "RuntimeError", "no de")
	if len(r.Rutas) != 1 {
		t.Fatalf("siguió pidiendo tras ver otra app: %v", r.Rutas)
	}
}

func TestAppstorePreparacionSinLocalizacionPrimariaOCategoria(t *testing.T) {
	r := appstorePreparacionSondear(t, `
api.datos["/v1/appInfos/info/primaryCategory"] = None
api.datos["/v1/appStoreVersions/old/appStoreVersionLocalizations"] = []
`, `("IOS",)`)
	if r.Error != nil {
		t.Fatal(r.Error.Mensaje)
	}
	datos := r.Lineas[0]
	if appstorePreparacionMapa(t, datos, "common")["primaryCategory"] != false {
		t.Fatalf("primaryCategory %v", datos["common"])
	}
	if !appstorePreparacionTodos(appstorePreparacionMapa(t, datos, "primaryText"), false) {
		t.Fatalf("primaryText %v", datos["primaryText"])
	}
	if datos["canonicalScreenshotsComplete"] != false {
		t.Fatalf("canonicalScreenshotsComplete %v", datos["canonicalScreenshotsComplete"])
	}
}

// --- Países: todos los territorios menos paises.json, sin activar los nuevos automáticos

type appstorePreparacionPaises struct {
	Error     *pyRestoExcepcion `json:"error"`
	Parches   [][]any           `json:"parches"`
	Segunda   [][]any           `json:"segunda"`
	Filas     []map[string]any  `json:"filas"`
	Excluidos []string          `json:"excluidos"`
}

func TestAppstorePreparacionPaisesAmpliaATodosSalvoExcluidosYConservaFranciaFuera(t *testing.T) {
	var r appstorePreparacionPaises
	appstorePreparacion(t, `
api = Escribible(filas_ue())
error = paises(api)
parches = list(api.parches)
api.parches.clear()
segundo = paises(api)
devolver({"error": error or segundo, "parches": parches, "segunda": api.parches, "filas": api.filas, "excluidos": sorted(asc.EXCLUIDOS)})
`, &r)
	if r.Error != nil {
		t.Fatal(r.Error.Mensaje)
	}
	cambiados := map[string]bool{}
	for _, p := range r.Parches {
		cambiados[p[0].(string)] = p[1].(bool)
	}
	for _, t2 := range []string{"FRA", "CHN", "RUS", "BLR", "ESP", "DEU"} {
		if _, ok := cambiados[t2]; ok {
			t.Fatalf("tocó %s", t2)
		}
	}
	if !cambiados["USA"] || !cambiados["JPN"] || !cambiados["BRA"] {
		t.Fatalf("no abrió USA, JPN y BRA: %v", cambiados)
	}
	for _, f := range r.Filas {
		territorio := f["relationships"].(map[string]any)["territory"].(map[string]any)["data"].(map[string]any)["id"].(string)
		if f["attributes"].(map[string]any)["available"] != !slices.Contains(r.Excluidos, territorio) {
			t.Fatalf("%s: available=%v con excluidos %v", territorio, f["attributes"], r.Excluidos)
		}
	}
	if len(r.Segunda) != 0 {
		t.Fatalf("repetir no reescribe nada, y reescribió %v", r.Segunda)
	}
}

func TestAppstorePreparacionPaisesRetiraUnExcluidoQueEstuvieraDisponible(t *testing.T) {
	var r appstorePreparacionPaises
	appstorePreparacion(t, `
filas = filas_ue()
next(f for f in filas if f["id"] == "fila-RUS")["attributes"]["available"] = True
api = Escribible(filas)
devolver({"error": paises(api), "parches": api.parches})
`, &r)
	if r.Error != nil {
		t.Fatal(r.Error.Mensaje)
	}
	if !slices.ContainsFunc(r.Parches, func(p []any) bool { return reflect.DeepEqual(p, []any{"RUS", false}) }) {
		t.Fatalf("no retiró RUS: %v", r.Parches)
	}
}

func TestAppstorePreparacionPaisesNoTocaNadaConTerritoriosNuevosAutomaticos(t *testing.T) {
	var r appstorePreparacionPaises
	appstorePreparacion(t, `
api = Escribible(filas_ue(), automatico=True)
devolver({"error": paises(api), "parches": api.parches})
`, &r)
	pyRestoLanza(t, r.Error, "RuntimeError", "territorios nuevos")
	if len(r.Parches) != 0 {
		t.Fatalf("escribió %v", r.Parches)
	}
}

func TestAppstorePreparacionPaisesUnRechazoDeAppleEsRojoYLoNombra(t *testing.T) {
	var r appstorePreparacionPaises
	appstorePreparacion(t, `
api = Escribible(filas_ue(), rechaza={"JPN"})
devolver({"error": paises(api)})
`, &r)
	pyRestoLanza(t, r.Error, "RuntimeError", "JPN")
}

func TestAppstorePreparacionPaisesPoliticaExcluyeFranciaChinaYSanciones(t *testing.T) {
	var excluidos []string
	appstorePreparacion(t, "\ndevolver(sorted(asc.EXCLUIDOS))\n", &excluidos)
	for _, p := range []string{"FRA", "CHN", "RUS", "BLR"} {
		if !slices.Contains(excluidos, p) {
			t.Fatalf("EXCLUIDOS no contiene %s: %v", p, excluidos)
		}
	}
}

func TestAppstorePreparacionPaisesPideConfirmacionAntesDeCredenciales(t *testing.T) {
	var r struct {
		Codigo int    `json:"codigo"`
		Err    string `json:"err"`
	}
	appstorePreparacion(t, `
with mock.patch.dict("os.environ", {}, clear=True), contextlib.redirect_stderr(io.StringIO()) as err:
    codigo = asc.main(["--paises", "rooms"])
devolver({"codigo": codigo, "err": err.getvalue()})
`, &r)
	if r.Codigo != 2 || !regexp.MustCompile(`GLOBAL_SALVO_EXCLUIDOS`).MatchString(r.Err) {
		t.Fatalf("código %d, stderr %q", r.Codigo, r.Err)
	}
}
