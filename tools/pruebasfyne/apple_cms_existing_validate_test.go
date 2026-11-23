// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package pruebasfyne

import "testing"

// Traducción de apps/fyne/scripts/apple-cms-existing-validate-test.py: el
// cliente de la validación de CMS existentes es SOLO de lectura (GET de rutas
// propias de la API) y no sigue redirecciones con la credencial puesta.

const appleCMSExistentesCarga = `
import importlib.util, types
from unittest import mock
RUTA = pathlib.Path("apple-cms-existing-validate.py").resolve()
spec = importlib.util.spec_from_file_location("validacion_cms_existentes", RUTA)
modulo = importlib.util.module_from_spec(spec)
spec.loader.exec_module(modulo)
`

func TestAppleCMSExistentesElTransporteNoSigueRedireccionesConLaCredencial(t *testing.T) {
	R := applePy(t, appleCMSExistentesCarga+`
class API:
    pass
with mock.patch.object(modulo.urllib.request, "install_opener") as instalar:
    modulo.cliente_solo_lectura(types.SimpleNamespace(Apple=API))
R["instalado"] = instalar.call_count
opener = instalar.call_args.args[0]
handlers = [h for h in opener.handlers if isinstance(h, modulo.SinRedirecciones)]
R["handlers"] = len(handlers)
R["redirigir"] = atrapar(handlers[0].redirect_request, None, None, 302, "moved", {}, "https://attacker.invalid/")
`)
	appleIgual(t, "install_opener se llama una vez", R["instalado"], `1`)
	appleIgual(t, "un único SinRedirecciones", R["handlers"], `1`)
	appleExc(t, "una redirección no se sigue", R["redirigir"], "ValueError", "redirecciones")
}

func TestAppleCMSExistentesNoEnviaEscriturasNiCredencialesAOtroOrigen(t *testing.T) {
	R := applePy(t, appleCMSExistentesCarga+`
llamadas = []
class API:
    def pedir(self, *args, **kwargs):
        llamadas.append((args, kwargs))
        return {"data": []}
api = modulo.cliente_solo_lectura(types.SimpleNamespace(Apple=API))
R["casos"] = [atrapar(api.pedir, m, r, c) for m, r, c in (
    ("POST", "/v1/profiles", {}),
    ("DELETE", "/v1/profiles/abc", None),
    ("GET", "https://attacker.invalid/v1/bundleIds", None),
    ("GET", "//attacker.invalid/v1/bundleIds", None),
    ("GET", "/v1/certificates", None),
    ("GET", "/v1/bundleIds", {}),
)]
R["antes"] = len(llamadas)
R["get"] = api.pedir("GET", "/v1/bundleIds?limit=200")
R["despues"] = len(llamadas)
`)
	casos := []string{
		"POST /v1/profiles", "DELETE /v1/profiles/abc", "GET https://attacker.invalid/v1/bundleIds",
		"GET //attacker.invalid/v1/bundleIds", "GET /v1/certificates", "GET /v1/bundleIds con cuerpo",
	}
	for i, nombre := range casos {
		t.Run(nombre, func(t *testing.T) {
			appleExc(t, nombre, appleCampo(R, "casos", i), "ValueError", "")
		})
	}
	appleIgual(t, "ninguna petición rechazada llegó a la API", R["antes"], `0`)
	appleIgual(t, "el GET permitido devuelve la respuesta", R["get"], `{"data": []}`)
	appleIgual(t, "y llega una sola vez", R["despues"], `1`)
}
