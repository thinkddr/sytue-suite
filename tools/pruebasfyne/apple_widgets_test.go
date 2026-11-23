// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package pruebasfyne

import (
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/tools/guiontest"
)

// Traducción de apps/fyne/scripts/apple-widgets-test.py: el registro de la
// extensión WidgetKit en App Store Connect (apple-widgets.py, el real) contra
// una API de Apple sintética en proceso (`Apple`), como en el original.

const appleWidgetsCarga = `
import copy, contextlib, importlib.util, io, plistlib, subprocess, types, urllib.parse
from unittest import mock
spec = importlib.util.spec_from_file_location("widgets_apple", pathlib.Path("apple-widgets.py").resolve())
widgets = importlib.util.module_from_spec(spec)
spec.loader.exec_module(widgets)
APP = {"clave": "rooms", "id": "eu.sytue.rooms", "nombre": "Sytue Rooms"}
GRUPO = "group.eu.sytue.suite"

class Apple:
    def __init__(self, registrado=False):
        self.host = {"id": "host", "attributes": {"identifier": APP["id"], "platform": "UNIVERSAL", "seedId": "TEAM"}}
        self.extension = {"id": "widget", "attributes": {"identifier": APP["id"] + ".widgets", "platform": "UNIVERSAL"}} if registrado else None
        self.capacidades = []
        self.capacidades_host = []
        self.escrituras = []
        self.lecturas = []

    def lista(self, ruta):
        self.lecturas.append(ruta)
        if "bundleIdCapabilities" in ruta:
            return copy.deepcopy(self.capacidades_host if ruta.startswith("/v1/bundleIds/host/") else self.capacidades)
        if "eu.sytue.rooms.widgets" in ruta:
            return [self.extension] if self.extension else []
        return [self.host] if self.host else []

    def escribir(self, tipo, attrs, rels=None, id=None):
        self.escrituras.append((tipo, copy.deepcopy(attrs), rels, id))
        if tipo == "bundleIds":
            self.extension = {"id": "widget", "attributes": attrs}
            return {"data": self.extension}
        if tipo == "bundleIdCapabilities":
            assert (rels and rels["bundleId"]["id"] in ("widget", "host")) or id
            capacidad = {"id": id or attrs["capabilityType"], "attributes": copy.deepcopy(attrs)}
            if rels and rels["bundleId"]["id"] == "host":
                self.capacidades_host.append(capacidad)
                return {"data": capacidad}
            self.capacidades = [c for c in self.capacidades if c["id"] != capacidad["id"]] + [capacidad]
            return {"data": capacidad}
        raise AssertionError("unexpected mutation " + tipo)

def con_main(asc, argv, despertar=False, acciones=("abrir",)):
    """main() con el catálogo y la API sintéticos; devuelve (código, JSON impreso)."""
    gen = types.SimpleNamespace(apps=lambda: [{**APP, "widget_acciones": list(acciones)}],
                                grupo_widgets=lambda: GRUPO, despertar_apple=lambda: despertar)
    def importar(nombre, ruta):
        return gen if str(ruta).endswith("generar.py") else types.SimpleNamespace(Apple=lambda: asc)
    with mock.patch.object(widgets, "importar", side_effect=importar), \
            mock.patch("sys.argv", argv), contextlib.redirect_stdout(io.StringIO()) as salida:
        codigo = widgets.main()
    return codigo, json.loads(salida.getvalue())
`

func appleWidgets(t *testing.T, codigo string) map[string]any {
	t.Helper()
	return applePy(t, appleWidgetsCarga+codigo)
}

func TestAppleWidgetsLaConsultaNoCreaIdentificadoresNiCapacidades(t *testing.T) {
	R := appleWidgets(t, `
asc = Apple()
R["r"] = widgets.registrar(asc, APP, False, GRUPO)
R["escrituras"] = asc.escrituras
`)
	appleFalso(t, "registered", appleCampo(R, "r", "registered"))
	appleIgual(t, "escrituras", R["escrituras"], `[]`)
	appleFalso(t, "groupAssociationVerified", appleCampo(R, "r", "groupAssociationVerified"))
}

func TestAppleWidgetsUnaConsultaIncompletaNoDaVerdeAlOperador(t *testing.T) {
	R := appleWidgets(t, `
for registrado in (False, True):
    asc = Apple(registrado)
    codigo, dato = con_main(asc, ["apple-widgets.py", "rooms"])
    R[str(registrado)] = {"codigo": codigo, "dato": dato, "escrituras": asc.escrituras}
`)
	for _, registrado := range []string{"False", "True"} {
		t.Run("registrado="+registrado, func(t *testing.T) {
			r := R[registrado]
			appleIgual(t, "main()", appleCampo(r, "codigo"), `1`)
			appleFalso(t, "registrationReady", appleCampo(r, "dato", "registrationReady"))
			appleFalso(t, "groupAssociationVerified", appleCampo(r, "dato", "groupAssociationVerified"))
			appleIgual(t, "escrituras", appleCampo(r, "escrituras"), `[]`)
		})
	}
}

func TestAppleWidgetsUnaConsultaCompletaNoConfundeCapacidadesConAsociacionDelGrupo(t *testing.T) {
	R := appleWidgets(t, `
asc = Apple()
widgets.registrar(asc, APP, True, GRUPO)
asc.escrituras.clear()
R["codigo"], R["dato"] = con_main(asc, ["apple-widgets.py"])
R["escrituras"] = asc.escrituras
`)
	appleIgual(t, "main()", R["codigo"], `0`)
	appleCierto(t, "registrationReady", appleCampo(R, "dato", "registrationReady"))
	appleFalso(t, "groupAssociationVerified", appleCampo(R, "dato", "groupAssociationVerified"))
	appleIgual(t, "escrituras", R["escrituras"], `[]`)
}

func TestAppleWidgetsRegistraSoloElWidgetConPrefijoYPlataformaDelHost(t *testing.T) {
	R := appleWidgets(t, `
asc = Apple()
widgets.registrar(asc, APP, True, GRUPO)
R["escrituras"] = asc.escrituras
R["PROTECCION"] = widgets.PROTECCION
`)
	appleIgual(t, "primera escritura", appleCampo(R, "escrituras", 0),
		`["bundleIds", {"identifier": "eu.sytue.rooms.widgets", "name": "Sytue Rooms Widgets", "platform": "UNIVERSAL", "seedId": "TEAM"}, null, null]`)
	escrituras, _ := R["escrituras"].([]any)
	tipos := make([]any, 0, len(escrituras))
	for _, e := range escrituras[1:] {
		tipos = append(tipos, appleCampo(e, 1, "capabilityType"))
	}
	appleIgual(t, "capacidades", tipos, `["APP_GROUPS", "DATA_PROTECTION"]`)
	appleIgual(t, "settings de la protección", appleCampo(R, "escrituras", -1, 1, "settings"), appleTexto(R["PROTECCION"]))
}

func TestAppleWidgetsUnGETConUnicaOpcionSeleccionadaNoSeReescribeNiExigeEnabledOmitido(t *testing.T) {
	R := appleWidgets(t, `
asc = Apple()
widgets.registrar(asc, APP, True, GRUPO)
opcion = asc.capacidades[-1]["attributes"]["settings"][0]["options"][0]
R["enabled"] = opcion.pop("enabled")
asc.escrituras.clear()
R["vueltas"] = []
for registrar in (False, True, True):
    resultado = widgets.registrar(asc, APP, registrar, GRUPO)
    R["vueltas"].append({"r": resultado, "escrituras": list(asc.escrituras)})
R["contrato"] = widgets.PROTECCION[0]["options"][0]["enabled"]
`)
	appleIgual(t, "la opción seleccionada llevaba enabled=True", R["enabled"], `true`)
	for i, registrar := range []string{"False", "True", "True"} {
		t.Run("registrar="+registrar, func(t *testing.T) {
			v := appleCampo(R, "vueltas", i)
			appleCierto(t, "registrationReady", appleCampo(v, "r", "registrationReady"))
			appleIgual(t, "capabilities", appleCampo(v, "r", "capabilities"), `["APP_GROUPS", "DATA_PROTECTION"]`)
			appleFalso(t, "groupAssociationVerified", appleCampo(v, "r", "groupAssociationVerified"))
			appleIgual(t, "escrituras", appleCampo(v, "escrituras"), `[]`)
		})
	}
	appleIgual(t, "no cambiar el contrato POST/PATCH", R["contrato"], `true`)
}

func TestAppleWidgetsLaProteccionRechazaProyeccionesAmbiguasYBooleanosNoVerdaderos(t *testing.T) {
	R := appleWidgets(t, `
completa = {"key": "COMPLETE_PROTECTION"}
otra = {"key": "PROTECTED_UNTIL_FIRST_USER_AUTH"}
opciones = [None, [], {}, "COMPLETE_PROTECTION", [None], [otra], [completa, otra], [completa, completa],
            [{**completa, "enabledByDefault": True}], [{**completa, "name": "Complete Protection"}],
            [{**completa, "enabled": True}, otra],
            [{**completa, "enabled": True}, {**completa, "enabled": False}]]
opciones += [[{**completa, "enabled": valor}] for valor in (False, None, 0, 1, "true", "false", [], {})]
R["opciones"] = [[repr(o), widgets.completa({"attributes": {"settings": [{"key": "DATA_PROTECTION_PERMISSION_LEVEL", "options": o}]}})]
                 for o in opciones]
R["settings"] = [[repr(s), widgets.completa({"attributes": {"settings": s}})]
                 for s in (None, False, {}, "DATA_PROTECTION_PERMISSION_LEVEL", [None],
                           [{"key": "DATA_PROTECTION_PERMISSION_LEVEL", "options": [completa]}] * 2)]
`)
	for _, grupo := range []string{"opciones", "settings"} {
		casos, _ := R[grupo].([]any)
		if len(casos) == 0 {
			t.Fatalf("sin casos de %s", grupo)
		}
		for _, c := range casos {
			nombre, _ := appleCampo(c, 0).(string)
			t.Run(grupo+" "+nombre, func(t *testing.T) { appleFalso(t, "completa()", appleCampo(c, 1)) })
		}
	}
	if casos, _ := R["opciones"].([]any); len(casos) != 20 {
		t.Errorf("se esperaban 20 proyecciones de opciones, hay %d", len(casos))
	}
}

func TestAppleWidgetsLaProteccionBooleanaExplicitaConservaAlternativasDesactivadas(t *testing.T) {
	R := appleWidgets(t, `
R["r"] = widgets.completa({"attributes": {"settings": [{"key": "DATA_PROTECTION_PERMISSION_LEVEL", "options": [
    {"key": "COMPLETE_PROTECTION", "enabled": True},
    {"key": "PROTECTED_UNTIL_FIRST_USER_AUTH", "enabled": False}]}]}})
`)
	appleCierto(t, "completa()", R["r"])
}

func TestAppleWidgetsElRegistroRileeCapacidadesSiElPOSTOmiteLosSettings(t *testing.T) {
	R := appleWidgets(t, `
asc = Apple()
escribir = asc.escribir
def respuesta_minima(tipo, attrs, rels=None, id=None):
    respuesta = escribir(tipo, attrs, rels, id)
    if tipo == "bundleIdCapabilities":
        return {"data": {"id": respuesta["data"]["id"], "attributes": {"capabilityType": attrs["capabilityType"]}}}
    return respuesta
with mock.patch.object(asc, "escribir", side_effect=respuesta_minima):
    R["r"] = widgets.registrar(asc, APP, True, GRUPO)
`)
	appleCierto(t, "registrationReady", appleCampo(R, "r", "registrationReady"))
	appleIgual(t, "capabilities", appleCampo(R, "r", "capabilities"), `["APP_GROUPS", "DATA_PROTECTION"]`)
	appleFalso(t, "groupAssociationVerified", appleCampo(R, "r", "groupAssociationVerified"))
}

func TestAppleWidgetsUnGETSinProteccionNoSeDaPorBuenoAunqueElPOSTLaDevuelva(t *testing.T) {
	R := appleWidgets(t, `
asc = Apple()
lista = asc.lista
def lectura_sin_proteccion(ruta):
    if urllib.parse.urlsplit(ruta).path.endswith("/bundleIdCapabilities"):
        return []
    return lista(ruta)
with mock.patch.object(asc, "lista", side_effect=lectura_sin_proteccion):
    R["r"] = widgets.registrar(asc, APP, True, GRUPO)
`)
	appleFalso(t, "registrationReady", appleCampo(R, "r", "registrationReady"))
	appleIgual(t, "capabilities", appleCampo(R, "r", "capabilities"), `[]`)
}

func TestAppleWidgetsElDiagnosticoDPNoExponeIdsNombresDescripcionesNiValoresExtra(t *testing.T) {
	R := appleWidgets(t, `
asc = Apple()
widgets.registrar(asc, APP, True, GRUPO)
capacidad = asc.capacidades[-1]
capacidad["id"] = "ID_PRIVADO_NO_PUBLICAR"
capacidad["attributes"]["name"] = "NOMBRE_PRIVADO_NO_PUBLICAR"
setting = capacidad["attributes"]["settings"][0]
setting["description"] = "DESCRIPCION_PRIVADA_NO_PUBLICAR"
setting["options"][0]["name"] = "PERSONA_PRIVADA_NO_PUBLICAR"
setting["options"][0]["id"] = "OTRO_ID_PRIVADO_NO_PUBLICAR"
resultado = widgets.registrar(asc, APP, False, GRUPO)
R["r"] = resultado
R["volcado"] = json.dumps(resultado)
`)
	d := appleCampo(R, "r", "dataProtectionDiagnostic")
	appleCierto(t, "present", appleCampo(d, "present"))
	appleIgual(t, "observedCapabilityTypes", appleCampo(d, "observedCapabilityTypes"), `["APP_GROUPS", "DATA_PROTECTION"]`)
	opcion := appleCampo(d, "settings", 0, "options", 0)
	appleIgual(t, "opción", []any{appleCampo(opcion, "key"), appleCampo(opcion, "enabled"), appleCampo(opcion, "enabledType")},
		`["COMPLETE_PROTECTION", true, "bool"]`)
	if v, _ := R["volcado"].(string); strings.Contains(v, "NO_PUBLICAR") {
		t.Errorf("el resultado publica datos privados: %s", v)
	}
}

func TestAppleWidgetsElDiagnosticoDPAcotaListasCadenasYMuestraTiposSinCoercion(t *testing.T) {
	R := appleWidgets(t, `
settings = [{"key": "x" * 200, "options": [{"key": "opcion@example.test", "enabled": "true"},
            {"key": "COMPLETE_PROTECTION", "enabled": {"NO_PUBLICAR": "SECRETO"}}] * 20}] * 20
capacidades = {"DATA_PROTECTION": {"attributes": {"settings": settings}}}
dato = widgets.diagnostico_proteccion(capacidades)
R["dato"] = dato
R["volcado"] = json.dumps(dato)
R["completa"] = widgets.completa(capacidades["DATA_PROTECTION"])
`)
	d := R["dato"]
	if s, _ := appleCampo(d, "settings").([]any); len(s) != 8 {
		t.Errorf("settings sin acotar a 8: %d", len(s))
	}
	if o, _ := appleCampo(d, "settings", 0, "options").([]any); len(o) != 8 {
		t.Errorf("options sin acotar a 8: %d", len(o))
	}
	appleIgual(t, "clave del setting", appleCampo(d, "settings", 0, "key"), `"<str>"`)
	appleIgual(t, "clave de la opción", appleCampo(d, "settings", 0, "options", 0, "key"), `"<str>"`)
	appleIgual(t, "opción 0", []any{appleCampo(d, "settings", 0, "options", 0, "enabled"), appleCampo(d, "settings", 0, "options", 0, "enabledType")}, `["true", "str"]`)
	appleIgual(t, "opción 1", []any{appleCampo(d, "settings", 0, "options", 1, "enabled"), appleCampo(d, "settings", 0, "options", 1, "enabledType")}, `["<dict>", "dict"]`)
	if v, _ := R["volcado"].(string); strings.Contains(v, "NO_PUBLICAR") {
		t.Errorf("el diagnóstico publica datos privados: %s", v)
	}
	appleFalso(t, "diagnosticar no relaja Complete Protection", R["completa"])
}

func TestAppleWidgetsNoMutaHostPerfilesCertificadosNiAppGroups(t *testing.T) {
	R := appleWidgets(t, `
asc = Apple()
widgets.registrar(asc, APP, True, GRUPO)
R["escrituras"] = asc.escrituras
`)
	escrituras, _ := R["escrituras"].([]any)
	for _, e := range escrituras {
		if tipo := appleCampo(e, 0); tipo != "bundleIds" && tipo != "bundleIdCapabilities" {
			t.Errorf("escritura de otro tipo: %s", appleTexto(e))
		}
		if rels := appleCampo(e, 2); appleVerdad(rels) && appleCampo(rels, "bundleId", "id") != "widget" {
			t.Errorf("escritura fuera del widget: %s", appleTexto(e))
		}
	}
}

func TestAppleWidgetsUnaRespuestaParcialUsaElGETPersistidoYNoReescribeAlRepetir(t *testing.T) {
	R := appleWidgets(t, `
asc = Apple()
escribir_original = asc.escribir
def escribir(*args, **kwargs):
    respuesta = copy.deepcopy(escribir_original(*args, **kwargs))
    if args[0] == "bundleIdCapabilities":
        respuesta["data"]["attributes"].pop("settings", None)
    return respuesta
with mock.patch.object(asc, "escribir", side_effect=escribir):
    R["r"] = widgets.registrar(asc, APP, True, GRUPO)
    R["lecturas"] = [urllib.parse.parse_qs(urllib.parse.urlsplit(r).query) for r in asc.lecturas if "bundleIdCapabilities" in r]
    asc.escrituras.clear()
    R["repetido"] = widgets.registrar(asc, APP, True, GRUPO)
    R["escrituras"] = asc.escrituras
`)
	appleCierto(t, "registrationReady", appleCampo(R, "r", "registrationReady"))
	p := appleCampo(R, "r", "dataProtectionDiagnostic", "settings", 0)
	appleIgual(t, "clave de la protección", appleCampo(p, "key"), `"DATA_PROTECTION_PERMISSION_LEVEL"`)
	appleCierto(t, "opción activada", appleCampo(p, "options", 0, "enabled"))
	lecturas, _ := R["lecturas"].([]any)
	if len(lecturas) != 2 {
		t.Errorf("se esperaban 2 lecturas de capacidades: %s", appleTexto(lecturas))
	}
	for _, q := range lecturas {
		appleIgual(t, "fields[bundleIdCapabilities]", appleCampo(q, "fields[bundleIdCapabilities]"), `["capabilityType,settings"]`)
	}
	appleCierto(t, "repetido registrationReady", appleCampo(R, "repetido", "registrationReady"))
	appleIgual(t, "escrituras al repetir", R["escrituras"], `[]`)
}

func TestAppleWidgetsUnaRespuesta2xxNoSupleProteccionAusenteODesactivadaEnElGET(t *testing.T) {
	R := appleWidgets(t, `
R["casos"] = []
for settings in (None, [{"key": "DATA_PROTECTION_PERMISSION_LEVEL", "options": [
        {"key": "COMPLETE_PROTECTION", "enabled": False}]}]):
    asc = Apple(True)
    escribir_original = asc.escribir
    def escribir(*args, **kwargs):
        respuesta = copy.deepcopy(escribir_original(*args, **kwargs))
        if args[0] == "bundleIdCapabilities":
            respuesta["data"]["attributes"].pop("settings", None)
            if args[1]["capabilityType"] == "DATA_PROTECTION":
                asc.capacidades[-1]["attributes"]["settings"] = copy.deepcopy(settings)
        return respuesta
    with mock.patch.object(asc, "escribir", side_effect=escribir):
        R["casos"].append(widgets.registrar(asc, APP, True, GRUPO))
`)
	for i, nombre := range []string{"settings=None", "COMPLETE_PROTECTION desactivada"} {
		t.Run(nombre, func(t *testing.T) {
			r := appleCampo(R, "casos", i)
			appleFalso(t, "registrationReady", appleCampo(r, "registrationReady"))
			appleIgual(t, "capabilities", appleCampo(r, "capabilities"), `["APP_GROUPS"]`)
			d := appleCampo(r, "dataProtectionDiagnostic")
			if i == 0 {
				appleIgual(t, "settingsType", appleCampo(d, "settingsType"), `"NoneType"`)
			} else {
				appleIgual(t, "settingsType", appleCampo(d, "settingsType"), `"list"`)
				appleFalso(t, "enabled", appleCampo(d, "settings", 0, "options", 0, "enabled"))
			}
		})
	}
}

func TestAppleWidgetsUnPATCHParcialRileeLaProteccionPersistida(t *testing.T) {
	R := appleWidgets(t, `
asc = Apple()
widgets.registrar(asc, APP, True, GRUPO)
asc.capacidades[-1]["attributes"]["settings"] = None
asc.escrituras.clear()
escribir_original = asc.escribir
def escribir(*args, **kwargs):
    respuesta = copy.deepcopy(escribir_original(*args, **kwargs))
    respuesta["data"]["attributes"].pop("settings", None)
    return respuesta
with mock.patch.object(asc, "escribir", side_effect=escribir):
    R["r"] = widgets.registrar(asc, APP, True, GRUPO)
R["escrituras"] = asc.escrituras
`)
	appleCierto(t, "registrationReady", appleCampo(R, "r", "registrationReady"))
	if e, _ := R["escrituras"].([]any); len(e) != 1 {
		t.Fatalf("se esperaba un único PATCH: %s", appleTexto(e))
	}
	appleIgual(t, "id del PATCH", appleCampo(R, "escrituras", 0, 3), `"DATA_PROTECTION"`)
}

func TestAppleWidgetsUnGETPosteriorFallidoNoAcreditaLaEscritura(t *testing.T) {
	R := appleWidgets(t, `
asc = Apple()
lista_original = asc.lista
def lista(ruta):
    if "bundleIdCapabilities" in ruta and asc.escrituras:
        raise RuntimeError("HTTP 503 GET bundleIdCapabilities")
    return lista_original(ruta)
with mock.patch.object(asc, "lista", side_effect=lista):
    R["r"] = atrapar(widgets.registrar, asc, APP, True, GRUPO)
`)
	appleExc(t, "GET posterior con 503", R["r"], "RuntimeError", "HTTP 503")
}

func TestAppleWidgetsRepetirNoInvalidaCapacidadesYaCorrectas(t *testing.T) {
	R := appleWidgets(t, `
asc = Apple()
widgets.registrar(asc, APP, True, GRUPO)
asc.escrituras.clear()
# Apple añade metadatos descriptivos y opciones desactivadas a las respuestas GET.
asc.capacidades[-1]["attributes"]["settings"][0]["name"] = "Data Protection"
asc.capacidades[-1]["attributes"]["settings"][0]["options"].append({"key": "PROTECTED_UNTIL_FIRST_USER_AUTH", "enabled": False})
widgets.registrar(asc, APP, True, GRUPO)
R["escrituras"] = asc.escrituras
`)
	appleIgual(t, "escrituras", R["escrituras"], `[]`)
}

func TestAppleWidgetsSiFaltaElHostNoSeCreaNiSeTocaSuCuenta(t *testing.T) {
	R := appleWidgets(t, `
asc = Apple()
asc.host = None
R["r"] = atrapar(widgets.registrar, asc, APP, True, GRUPO)
R["escrituras"] = asc.escrituras
`)
	appleExc(t, "sin host", R["r"], "RuntimeError", "falta el App ID del host")
	appleIgual(t, "escrituras", R["escrituras"], `[]`)
}

func TestAppleWidgetsUnaPlataformaIncompatibleNoSeModifica(t *testing.T) {
	R := appleWidgets(t, `
asc = Apple(True)
asc.extension["attributes"]["platform"] = "MAC_OS"
R["r"] = atrapar(widgets.registrar, asc, APP, True, GRUPO)
R["escrituras"] = asc.escrituras
`)
	appleExc(t, "plataforma distinta", R["r"], "RuntimeError", "plataforma distinta")
	appleIgual(t, "escrituras", R["escrituras"], `[]`)
}

func TestAppleWidgetsMacSoloHabilitaAppGroups(t *testing.T) {
	R := appleWidgets(t, `
asc = Apple()
asc.host["attributes"]["platform"] = "MAC_OS"
R["r"] = widgets.registrar(asc, APP, True, GRUPO)
`)
	appleIgual(t, "capabilities", appleCampo(R, "r", "capabilities"), `["APP_GROUPS"]`)
	appleCierto(t, "profileRequired", appleCampo(R, "r", "profileRequired"))
}

// SYTUE_DESPERTAR_APPLE: Push Notifications en el App ID del host, no en la extensión.

func TestAppleWidgetsPushRegistrarActivaPushSoloEnElHostYSinSettings(t *testing.T) {
	R := appleWidgets(t, `
asc = Apple()
R["r"] = widgets.registrar(asc, APP, True, GRUPO, push=True)
R["push"] = [e for e in asc.escrituras if e[0] == "bundleIdCapabilities" and e[1]["capabilityType"] == "PUSH_NOTIFICATIONS"]
R["widget"] = [c["attributes"]["capabilityType"] for c in asc.capacidades]
`)
	appleIgual(t, "escritura de Push", R["push"],
		`[["bundleIdCapabilities", {"capabilityType": "PUSH_NOTIFICATIONS"}, {"bundleId": {"type": "bundleIds", "id": "host"}}, null]]`)
	if appleContiene(R["widget"], "PUSH_NOTIFICATIONS") {
		t.Errorf("Push activado en la extensión: %s", appleTexto(R["widget"]))
	}
	appleIgual(t, "hostCapabilities", appleCampo(R, "r", "hostCapabilities"), `["PUSH_NOTIFICATIONS"]`)
	appleIgual(t, "hostRequiredCapabilities", appleCampo(R, "r", "hostRequiredCapabilities"), `["PUSH_NOTIFICATIONS"]`)
	appleCierto(t, "registrationReady", appleCampo(R, "r", "registrationReady"))
}

func TestAppleWidgetsPushYaActivoNoSeToca(t *testing.T) {
	R := appleWidgets(t, `
asc = Apple()
widgets.registrar(asc, APP, True, GRUPO, push=True)
asc.escrituras.clear()
R["vueltas"] = []
for registrar in (False, True):
    r = widgets.registrar(asc, APP, registrar, GRUPO, push=True)
    R["vueltas"].append({"listo": r["registrationReady"], "escrituras": list(asc.escrituras)})
`)
	for i, registrar := range []string{"False", "True"} {
		t.Run("registrar="+registrar, func(t *testing.T) {
			appleCierto(t, "registrationReady", appleCampo(R, "vueltas", i, "listo"))
			appleIgual(t, "escrituras", appleCampo(R, "vueltas", i, "escrituras"), `[]`)
		})
	}
}

func TestAppleWidgetsPushLaConsultaDiceQueFaltaPushSinEscribir(t *testing.T) {
	R := appleWidgets(t, `
asc = Apple()
widgets.registrar(asc, APP, True, GRUPO)
asc.escrituras.clear()
R["r"] = widgets.registrar(asc, APP, False, GRUPO, push=True)
R["escrituras"] = asc.escrituras
`)
	appleIgual(t, "escrituras", R["escrituras"], `[]`)
	appleIgual(t, "capabilities", appleCampo(R, "r", "capabilities"), `["APP_GROUPS", "DATA_PROTECTION"]`)
	appleIgual(t, "hostCapabilities", appleCampo(R, "r", "hostCapabilities"), `[]`)
	appleIgual(t, "hostRequiredCapabilities", appleCampo(R, "r", "hostRequiredCapabilities"), `["PUSH_NOTIFICATIONS"]`)
	appleFalso(t, "sin Push en el host el despertar no firma", appleCampo(R, "r", "registrationReady"))
}

func TestAppleWidgetsPushQueElGETNoAcreditaNoDaVerde(t *testing.T) {
	R := appleWidgets(t, `
asc = Apple()
escribir = asc.escribir
def sin_persistir(tipo, attrs, rels=None, id=None):
    respuesta = escribir(tipo, attrs, rels, id)
    if rels and rels["bundleId"]["id"] == "host":
        asc.capacidades_host.clear()
    return respuesta
with mock.patch.object(asc, "escribir", side_effect=sin_persistir):
    R["r"] = widgets.registrar(asc, APP, True, GRUPO, push=True)
`)
	appleIgual(t, "hostCapabilities", appleCampo(R, "r", "hostCapabilities"), `[]`)
	appleFalso(t, "registrationReady", appleCampo(R, "r", "registrationReady"))
}

func TestAppleWidgetsPushUnHostSoloMacNoPidePush(t *testing.T) {
	R := appleWidgets(t, `
asc = Apple()
asc.host["attributes"]["platform"] = "MAC_OS"
R["r"] = widgets.registrar(asc, APP, True, GRUPO, push=True)
R["escrituras"] = asc.escrituras
`)
	appleIgual(t, "hostRequiredCapabilities", appleCampo(R, "r", "hostRequiredCapabilities"), `[]`)
	escrituras, _ := R["escrituras"].([]any)
	for _, e := range escrituras {
		if rels := appleCampo(e, 2); appleVerdad(rels) && appleCampo(rels, "bundleId", "id") != "widget" {
			t.Errorf("escritura fuera del widget: %s", appleTexto(e))
		}
	}
}

func TestAppleWidgetsPushMainPasaElInterruptorDelDespertar(t *testing.T) {
	R := appleWidgets(t, `
for despertar in (False, True):
    asc = Apple()
    codigo, dato = con_main(asc, ["apple-widgets.py", "--registrar"], despertar=despertar, acciones=("open",))
    R[str(despertar)] = {"codigo": codigo, "host": dato["hostCapabilities"], "escrito": bool(asc.capacidades_host)}
`)
	for _, despertar := range []string{"False", "True"} {
		t.Run("despertar="+despertar, func(t *testing.T) {
			r := R[despertar]
			appleIgual(t, "main()", appleCampo(r, "codigo"), `0`)
			if despertar == "True" {
				appleIgual(t, "hostCapabilities", appleCampo(r, "host"), `["PUSH_NOTIFICATIONS"]`)
				appleIgual(t, "Push escrito en el host", appleCampo(r, "escrito"), `true`)
			} else {
				appleIgual(t, "hostCapabilities", appleCampo(r, "host"), `[]`)
				appleIgual(t, "Push escrito en el host", appleCampo(r, "escrito"), `false`)
			}
		})
	}
}

// generar.despertar_apple() y el `aps-environment` de empaquetar.sh no divergen.
func TestAppleWidgetsPushElInterruptorDeGenerarEsElDeEmpaquetar(t *testing.T) {
	R := appleWidgets(t, `
gen_spec = importlib.util.spec_from_file_location("generar_push", pathlib.Path("../xcode/generar.py").resolve())
generar = importlib.util.module_from_spec(gen_spec)
gen_spec.loader.exec_module(generar)
for w in ("0", "1"):
    for d in ("0", "1"):
        entorno = dict(os.environ, SYTUE_WIDGETS_APPLE=w, SYTUE_DESPERTAR_APPLE=d)
        with mock.patch.dict(os.environ, entorno):
            derechos = plistlib.loads(subprocess.run(
                ["bash", "empaquetar.sh", "derechos-sesion", str(pathlib.Path("../cmd/calendar").resolve()),
                 generar.EQUIPO, "ios"], env=entorno, check=True, capture_output=True).stdout)
            R[w + d] = [generar.despertar_apple(), "aps-environment" in derechos]
`)
	for _, k := range []string{"00", "01", "10", "11"} {
		t.Run("widgets="+k[:1]+" despertar="+k[1:], func(t *testing.T) {
			par, _ := R[k].([]any)
			if len(par) != 2 || par[0] != par[1] {
				t.Errorf("despertar_apple() y aps-environment divergen: %s", appleTexto(R[k]))
			}
		})
	}
}

func TestAppleWidgetsPushElWorkflowTraeElInterruptor(t *testing.T) {
	flujo := guiontest.Leer(t, ".github/workflows/appstore-publicar.yml")
	inicio := strings.Index(flujo, "git checkout -q FETCH_HEAD")
	fin := strings.Index(flujo, "- name: Hablar con App Store Connect")
	if inicio < 0 || fin < inicio {
		t.Fatalf("no se encuentra el checkout de appstore-publicar.yml (%d, %d)", inicio, fin)
	}
	if !strings.Contains(flujo[inicio:fin], "apps/fyne/xcode/Sytue.xcconfig") {
		t.Error("el checkout del workflow no trae apps/fyne/xcode/Sytue.xcconfig")
	}
}
