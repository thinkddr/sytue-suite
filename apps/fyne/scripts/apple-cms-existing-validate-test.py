#!/usr/bin/env python3
# SPDX-License-Identifier: Apache-2.0
import importlib.util
import pathlib
import types
import unittest
from unittest import mock

RUTA = pathlib.Path(__file__).with_name("apple-cms-existing-validate.py")
spec = importlib.util.spec_from_file_location("validacion_cms_existentes", RUTA)
modulo = importlib.util.module_from_spec(spec)
spec.loader.exec_module(modulo)


class SoloGET(unittest.TestCase):
    def test_el_transporte_no_sigue_redirecciones_con_la_credencial(self):
        class API:
            pass
        with mock.patch.object(modulo.urllib.request, "install_opener") as instalar:
            modulo.cliente_solo_lectura(types.SimpleNamespace(Apple=API))
        instalar.assert_called_once()
        opener = instalar.call_args.args[0]
        handlers = [h for h in opener.handlers if isinstance(h, modulo.SinRedirecciones)]
        self.assertEqual(len(handlers), 1)
        with self.assertRaisesRegex(ValueError, "redirecciones"):
            handlers[0].redirect_request(None, None, 302, "moved", {}, "https://attacker.invalid/")

    def test_no_envia_escrituras_ni_credenciales_a_otro_origen(self):
        llamadas = []
        class API:
            def pedir(self, *args, **kwargs):
                llamadas.append((args, kwargs))
                return {"data": []}
        api = modulo.cliente_solo_lectura(types.SimpleNamespace(Apple=API))
        for metodo, ruta, cuerpo in (
            ("POST", "/v1/profiles", {}),
            ("DELETE", "/v1/profiles/abc", None),
            ("GET", "https://attacker.invalid/v1/bundleIds", None),
            ("GET", "//attacker.invalid/v1/bundleIds", None),
            ("GET", "/v1/certificates", None),
            ("GET", "/v1/bundleIds", {}),
        ):
            with self.subTest(metodo=metodo, ruta=ruta), self.assertRaises(ValueError):
                api.pedir(metodo, ruta, cuerpo)
        self.assertEqual(llamadas, [])
        self.assertEqual(api.pedir("GET", "/v1/bundleIds?limit=200"), {"data": []})
        self.assertEqual(len(llamadas), 1)


if __name__ == "__main__":
    unittest.main()
