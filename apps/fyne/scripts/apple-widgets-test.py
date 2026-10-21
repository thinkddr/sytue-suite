#!/usr/bin/env python3
# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
import copy
import importlib.util
import pathlib
import unittest


spec = importlib.util.spec_from_file_location("widgets_apple", pathlib.Path(__file__).with_name("apple-widgets.py"))
widgets = importlib.util.module_from_spec(spec)
spec.loader.exec_module(widgets)
APP = {"clave": "rooms", "id": "eu.sytue.rooms", "nombre": "Sytue Rooms"}


class Apple:
    def __init__(self, registrado=False):
        self.host = {"id": "host", "attributes": {"identifier": APP["id"], "platform": "UNIVERSAL", "seedId": "TEAM"}}
        self.extension = {"id": "widget", "attributes": {"identifier": APP["id"] + ".widgets", "platform": "UNIVERSAL"}} if registrado else None
        self.capacidades = []
        self.escrituras = []

    def lista(self, ruta):
        if "bundleIdCapabilities" in ruta:
            return copy.deepcopy(self.capacidades)
        if "eu.sytue.rooms.widgets" in ruta:
            return [self.extension] if self.extension else []
        return [self.host] if self.host else []

    def escribir(self, tipo, attrs, rels=None, id=None):
        self.escrituras.append((tipo, copy.deepcopy(attrs), rels, id))
        if tipo == "bundleIds":
            self.extension = {"id": "widget", "attributes": attrs}
            return {"data": self.extension}
        if tipo == "bundleIdCapabilities":
            assert (rels and rels["bundleId"]["id"] == "widget") or id
            capacidad = {"id": id or attrs["capabilityType"], "attributes": attrs}
            self.capacidades = [c for c in self.capacidades if c["id"] != capacidad["id"]] + [capacidad]
            return {"data": capacidad}
        raise AssertionError("unexpected mutation " + tipo)


class RegistroTests(unittest.TestCase):
    def test_consulta_no_crea_identificadores_ni_capacidades(self):
        asc = Apple()
        resultado = widgets.registrar(asc, APP, False, "group.eu.sytue.suite")
        self.assertFalse(resultado["registered"])
        self.assertEqual(asc.escrituras, [])
        self.assertFalse(resultado["groupAssociationVerified"])

    def test_registra_solo_widget_con_prefijo_y_plataforma_del_host(self):
        asc = Apple()
        widgets.registrar(asc, APP, True, "group.eu.sytue.suite")
        self.assertEqual(asc.escrituras[0], ("bundleIds", {"identifier": "eu.sytue.rooms.widgets", "name": "Sytue Rooms Widgets", "platform": "UNIVERSAL", "seedId": "TEAM"}, None, None))
        self.assertEqual([x[1]["capabilityType"] for x in asc.escrituras[1:]], ["APP_GROUPS", "DATA_PROTECTION"])
        self.assertEqual(asc.escrituras[-1][1]["settings"], widgets.PROTECCION)

    def test_no_muta_host_perfiles_certificados_o_appgroups(self):
        asc = Apple()
        widgets.registrar(asc, APP, True, "group.eu.sytue.suite")
        self.assertTrue(all(t in ("bundleIds", "bundleIdCapabilities") for t, *_ in asc.escrituras))
        self.assertTrue(all(not r or r["bundleId"]["id"] == "widget" for _, _, r, _ in asc.escrituras))

    def test_repetir_no_invalida_capacidades_ya_correctas(self):
        asc = Apple()
        widgets.registrar(asc, APP, True, "group.eu.sytue.suite")
        asc.escrituras.clear()
        # Apple adds descriptive metadata and disabled options to GET responses.
        asc.capacidades[-1]["attributes"]["settings"][0]["name"] = "Data Protection"
        asc.capacidades[-1]["attributes"]["settings"][0]["options"].append({"key": "PROTECTED_UNTIL_FIRST_USER_AUTH", "enabled": False})
        widgets.registrar(asc, APP, True, "group.eu.sytue.suite")
        self.assertEqual(asc.escrituras, [])

    def test_falta_host_no_se_crea_ni_toca_su_cuenta(self):
        asc = Apple()
        asc.host = None
        with self.assertRaisesRegex(RuntimeError, "falta el App ID del host"):
            widgets.registrar(asc, APP, True, "group.eu.sytue.suite")
        self.assertEqual(asc.escrituras, [])

    def test_plataforma_incompatible_no_se_modifica(self):
        asc = Apple(True)
        asc.extension["attributes"]["platform"] = "MAC_OS"
        with self.assertRaisesRegex(RuntimeError, "plataforma distinta"):
            widgets.registrar(asc, APP, True, "group.eu.sytue.suite")
        self.assertEqual(asc.escrituras, [])

    def test_mac_solo_habilita_appgroups(self):
        asc = Apple()
        asc.host["attributes"]["platform"] = "MAC_OS"
        resultado = widgets.registrar(asc, APP, True, "group.eu.sytue.suite")
        self.assertEqual(resultado["capabilities"], ["APP_GROUPS"])
        self.assertTrue(resultado["profileRequired"])


if __name__ == "__main__":
    unittest.main()
