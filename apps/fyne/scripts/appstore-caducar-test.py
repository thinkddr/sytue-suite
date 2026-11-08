#!/usr/bin/env python3
# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
"""appstore-publicar --caducar contra una API simulada: solo la build pedida, nunca la última."""
import contextlib
import importlib.util
import io
import os
import pathlib
import sys
import unittest
from unittest import mock

sys.path.insert(0, str(pathlib.Path(__file__).parent))
spec = importlib.util.spec_from_file_location("appstore_publicar", pathlib.Path(__file__).with_name("appstore-publicar.py"))
asc = importlib.util.module_from_spec(spec)
spec.loader.exec_module(asc)

FICHA = {"apps": {"rooms": {"id": "app"}}}


def build(id, numero, estado="VALID", caducada=False):
    return {"id": id, "attributes": {"version": numero, "processingState": estado, "expired": caducada}}


class API:
    def __init__(self, builds):
        self.builds = builds  # {plataforma: [build, ...]}
        self.escrito = []

    def pedir(self, metodo, ruta, cuerpo=None, tolerar=()):
        assert metodo == "GET" and ruta.startswith("/v1/apps/app?"), ruta
        return {"data": {"attributes": {"bundleId": asc.bundle_de("rooms"), "name": "Sytue Rooms", "primaryLocale": "es-ES"}}}

    def lista(self, ruta):
        assert "filter[preReleaseVersion.version]=3.19.3" in ruta, ruta
        plataforma = ruta.split("filter[preReleaseVersion.platform]=")[1].split("&")[0]
        return self.builds.get(plataforma, [])

    def escribir(self, tipo, attrs=None, rels=None, id=None, ruta=None):
        self.escrito.append((tipo, id, attrs))
        return {}


def correr(api, plataformas=("IOS", "MAC_OS")):
    salida = io.StringIO()
    with contextlib.redirect_stdout(salida):
        asc.caducar(api, "rooms", FICHA, list(plataformas), "3.19.3", "95")
    return salida.getvalue()


class Caducar(unittest.TestCase):
    def test_caduca_solo_la_build_pedida(self):
        api = API({p: [build(f"{p}-97", "97"), build(f"{p}-95", "95")] for p in ("IOS", "MAC_OS")})
        texto = correr(api)
        self.assertEqual(api.escrito, [("builds", "IOS-95", {"expired": True}), ("builds", "MAC_OS-95", {"expired": True})])
        self.assertIn("queda 97", texto)

    def test_no_toca_la_ultima_si_no_hay_relevo(self):
        # Solo la 95, o una 97 que aún procesa o ya caducó: nada se escribe y es un error.
        for otras in ([], [build("x", "97", estado="PROCESSING")], [build("x", "97", caducada=True)]):
            api = API({"IOS": otras + [build("IOS-95", "95")]})
            with self.assertRaises(RuntimeError):
                correr(api, ("IOS",))
            self.assertEqual(api.escrito, [], otras)

    def test_una_build_mas_vieja_no_es_relevo(self):
        api = API({"IOS": [build("IOS-95", "95"), build("IOS-90", "90")]})
        with self.assertRaises(RuntimeError):
            correr(api, ("IOS",))
        self.assertEqual(api.escrito, [])

    def test_ya_caducada_o_ausente_no_escribe(self):
        api = API({"IOS": [build("IOS-97", "97"), build("IOS-95", "95", caducada=True)], "MAC_OS": [build("M-97", "97")]})
        texto = correr(api)
        self.assertEqual(api.escrito, [])
        self.assertIn("ya estaba caducada", texto)
        self.assertIn("sin build 95", texto)


class Confirmacion(unittest.TestCase):
    """La frase se exige ANTES de leer credenciales: sin ella, ni se construye el cliente."""

    def main(self, argv, frase=None):
        entorno = {k: v for k, v in os.environ.items() if k != "APPSTORE_CONFIRMAR_CADUCAR"}
        if frase is not None:
            entorno["APPSTORE_CONFIRMAR_CADUCAR"] = frase
        with mock.patch.dict(os.environ, entorno, clear=True), mock.patch.object(asc, "Apple") as cliente:
            with contextlib.redirect_stderr(io.StringIO()):
                codigo = asc.main(argv)
        return codigo, cliente.called

    def test_sin_frase_o_con_otra_no_entra(self):
        for frase in (None, "CADUCAR_BUILD_97", "caducar_build_95"):
            self.assertEqual(self.main(["--caducar", "--version", "3.19.3", "--build", "95", "rooms"], frase), (2, False))

    def test_sin_version_o_build_no_entra(self):
        self.assertEqual(self.main(["--caducar", "--build", "95", "rooms"], "CADUCAR_BUILD_95"), (2, False))
        self.assertEqual(self.main(["--caducar", "--version", "3.19.3", "rooms"], "CADUCAR_BUILD_"), (2, False))
        self.assertEqual(self.main(["--caducar", "--version", "3.19.3", "--build", "9x", "rooms"], "CADUCAR_BUILD_9x"), (2, False))


if __name__ == "__main__":
    unittest.main()
