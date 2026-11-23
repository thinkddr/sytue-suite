// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package pruebasfyne

import (
	"reflect"
	"strings"
	"testing"
)

// `appstore-publicar.py --caducar` contra una API simulada: solo caduca la
// build pedida, nunca la última.

// appstoreCaducarAPI es el doble en proceso de la API de App Store Connect: solo
// lee la app y la lista de builds de la versión 3.19.3 por plataforma, y anota
// lo que se escribiría. Las builds las pone cada prueba (ENTRADA).
const appstoreCaducarAPI = `
import contextlib, io
sys.path.insert(0, str(SCRIPTS))
asc = cargar("appstore_publicar", SCRIPTS / "appstore-publicar.py")
FICHA = {"apps": {"rooms": {"id": "app"}}}

class API:
    def __init__(self, builds):
        self.builds = builds
        self.escrito = []

    def pedir(self, metodo, ruta, cuerpo=None, tolerar=()):
        assert metodo == "GET" and ruta.startswith("/v1/apps/app?"), ruta
        return {"data": {"attributes": {"bundleId": asc.bundle_de("rooms"), "name": "Sytue Rooms", "primaryLocale": "es-ES"}}}

    def lista(self, ruta):
        assert "filter[preReleaseVersion.version]=3.19.3" in ruta, ruta
        plataforma = ruta.split("filter[preReleaseVersion.platform]=")[1].split("&")[0]
        return self.builds.get(plataforma, [])

    def escribir(self, tipo, attrs=None, rels=None, id=None, ruta=None):
        self.escrito.append([tipo, id, attrs])
        return {}

api = API(ENTRADA["builds"])
salida = io.StringIO()
error = None
with contextlib.redirect_stdout(salida):
    try:
        asc.caducar(api, "rooms", FICHA, ENTRADA["plataformas"], "3.19.3", "95")
    except Exception as e:
        error = excepcion(e)
devolver({"escrito": api.escrito, "salida": salida.getvalue(), "error": error})
`

type appstoreCaducarResultado struct {
	Escrito [][]any           `json:"escrito"`
	Salida  string            `json:"salida"`
	Error   *pyRestoExcepcion `json:"error"`
}

// appstoreCaducarBuild es una build como la devuelve la API.
func appstoreCaducarBuild(id, numero, estado string, caducada bool) map[string]any {
	if estado == "" {
		estado = "VALID"
	}
	return map[string]any{"id": id, "attributes": map[string]any{"version": numero, "processingState": estado, "expired": caducada}}
}

func appstoreCaducarCorrer(t *testing.T, builds map[string][]map[string]any, plataformas ...string) appstoreCaducarResultado {
	t.Helper()
	if len(plataformas) == 0 {
		plataformas = []string{"IOS", "MAC_OS"}
	}
	var r appstoreCaducarResultado
	pyRestoCorrer(t, appstoreCaducarAPI, &r, pyRestoConEntrada(t, map[string]any{"builds": builds, "plataformas": plataformas}))
	return r
}

func TestAppstoreCaducarSoloLaBuildPedida(t *testing.T) {
	builds := map[string][]map[string]any{}
	for _, p := range []string{"IOS", "MAC_OS"} {
		builds[p] = []map[string]any{appstoreCaducarBuild(p+"-97", "97", "", false), appstoreCaducarBuild(p+"-95", "95", "", false)}
	}
	r := appstoreCaducarCorrer(t, builds)
	if r.Error != nil {
		t.Fatalf("caducar falló: %s", r.Error.Mensaje)
	}
	esperado := [][]any{{"builds", "IOS-95", map[string]any{"expired": true}}, {"builds", "MAC_OS-95", map[string]any{"expired": true}}}
	if !reflect.DeepEqual(r.Escrito, esperado) {
		t.Fatalf("escrituras %v, no %v", r.Escrito, esperado)
	}
	if !strings.Contains(r.Salida, "queda 97") {
		t.Fatalf("la salida no dice «queda 97»:\n%s", r.Salida)
	}
}

func TestAppstoreCaducarNoTocaLaUltimaSiNoHayRelevo(t *testing.T) {
	// Solo la 95, o una 97 que aún procesa o ya caducó: nada se escribe y es un error.
	for nombre, otras := range map[string][]map[string]any{
		"sola":     nil,
		"procesa":  {appstoreCaducarBuild("x", "97", "PROCESSING", false)},
		"caducada": {appstoreCaducarBuild("x", "97", "", true)},
	} {
		t.Run(nombre, func(t *testing.T) {
			r := appstoreCaducarCorrer(t, map[string][]map[string]any{"IOS": append(otras, appstoreCaducarBuild("IOS-95", "95", "", false))}, "IOS")
			pyRestoLanza(t, r.Error, "RuntimeError", "")
			if len(r.Escrito) != 0 {
				t.Fatalf("escribió %v", r.Escrito)
			}
		})
	}
}

func TestAppstoreCaducarUnaBuildMasViejaNoEsRelevo(t *testing.T) {
	r := appstoreCaducarCorrer(t, map[string][]map[string]any{"IOS": {
		appstoreCaducarBuild("IOS-95", "95", "", false), appstoreCaducarBuild("IOS-90", "90", "", false),
	}}, "IOS")
	pyRestoLanza(t, r.Error, "RuntimeError", "")
	if len(r.Escrito) != 0 {
		t.Fatalf("escribió %v", r.Escrito)
	}
}

func TestAppstoreCaducarYaCaducadaOAusenteNoEscribe(t *testing.T) {
	r := appstoreCaducarCorrer(t, map[string][]map[string]any{
		"IOS":    {appstoreCaducarBuild("IOS-97", "97", "", false), appstoreCaducarBuild("IOS-95", "95", "", true)},
		"MAC_OS": {appstoreCaducarBuild("M-97", "97", "", false)},
	})
	if r.Error != nil {
		t.Fatalf("caducar falló: %s", r.Error.Mensaje)
	}
	if len(r.Escrito) != 0 {
		t.Fatalf("escribió %v", r.Escrito)
	}
	for _, s := range []string{"ya estaba caducada", "sin build 95"} {
		if !strings.Contains(r.Salida, s) {
			t.Fatalf("la salida no dice %q:\n%s", s, r.Salida)
		}
	}
}

// La frase se exige ANTES de leer credenciales: sin ella, ni se construye el cliente.
const appstoreCaducarMain = `
import contextlib, io
from unittest import mock
sys.path.insert(0, str(SCRIPTS))
asc = cargar("appstore_publicar", SCRIPTS / "appstore-publicar.py")
entorno = {k: v for k, v in os.environ.items() if k != "APPSTORE_CONFIRMAR_CADUCAR"}
if ENTRADA["frase"] is not None:
    entorno["APPSTORE_CONFIRMAR_CADUCAR"] = ENTRADA["frase"]
with mock.patch.dict(os.environ, entorno, clear=True), mock.patch.object(asc, "Apple") as cliente:
    with contextlib.redirect_stderr(io.StringIO()):
        codigo = asc.main(ENTRADA["argv"])
devolver({"codigo": codigo, "cliente": cliente.called})
`

func appstoreCaducarConfirmar(t *testing.T, argv []string, frase any) {
	t.Helper()
	var r struct {
		Codigo  int  `json:"codigo"`
		Cliente bool `json:"cliente"`
	}
	pyRestoCorrer(t, appstoreCaducarMain, &r, pyRestoConEntrada(t, map[string]any{"argv": argv, "frase": frase}))
	if r.Codigo != 2 || r.Cliente {
		t.Fatalf("%v con frase %v: código %d y cliente construido=%v; se esperaba (2, false)", argv, frase, r.Codigo, r.Cliente)
	}
}

func TestAppstoreCaducarSinFraseOConOtraNoEntra(t *testing.T) {
	for _, frase := range []any{nil, "CADUCAR_BUILD_97", "caducar_build_95"} {
		appstoreCaducarConfirmar(t, []string{"--caducar", "--version", "3.19.3", "--build", "95", "rooms"}, frase)
	}
}

func TestAppstoreCaducarSinVersionOBuildNoEntra(t *testing.T) {
	appstoreCaducarConfirmar(t, []string{"--caducar", "--build", "95", "rooms"}, "CADUCAR_BUILD_95")
	appstoreCaducarConfirmar(t, []string{"--caducar", "--version", "3.19.3", "rooms"}, "CADUCAR_BUILD_")
	appstoreCaducarConfirmar(t, []string{"--caducar", "--version", "3.19.3", "--build", "9x", "rooms"}, "CADUCAR_BUILD_9x")
}
