#!/usr/bin/env python3
# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
import copy
import contextlib
import importlib.util
import io
import json
import pathlib
import types
import unittest
import urllib.parse
from unittest import mock


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
        self.lecturas = []

    def lista(self, ruta):
        self.lecturas.append(ruta)
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
            capacidad = {"id": id or attrs["capabilityType"], "attributes": copy.deepcopy(attrs)}
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

    def test_consulta_incompleta_no_da_verde_al_operador(self):
        for registrado in (False, True):
            with self.subTest(registrado=registrado):
                asc = Apple(registrado)
                gen = types.SimpleNamespace(apps=lambda: [{**APP, "widget_acciones": ["abrir"]}],
                                            grupo_widgets=lambda: "group.eu.sytue.suite")
                def importar(nombre, ruta):
                    return gen if str(ruta).endswith("generar.py") else types.SimpleNamespace(Apple=lambda: asc)
                with mock.patch.object(widgets, "importar", side_effect=importar), \
                        mock.patch("sys.argv", ["apple-widgets.py", "rooms"]), contextlib.redirect_stdout(io.StringIO()) as salida:
                    self.assertEqual(widgets.main(), 1)
                dato = json.loads(salida.getvalue())
                self.assertFalse(dato["registrationReady"])
                self.assertFalse(dato["groupAssociationVerified"])
                self.assertEqual(asc.escrituras, [])

    def test_consulta_completa_no_confunde_capacidades_con_asociacion_del_grupo(self):
        asc = Apple()
        widgets.registrar(asc, APP, True, "group.eu.sytue.suite")
        asc.escrituras.clear()
        gen = types.SimpleNamespace(apps=lambda: [{**APP, "widget_acciones": ["abrir"]}],
                                    grupo_widgets=lambda: "group.eu.sytue.suite")
        def importar(nombre, ruta):
            return gen if str(ruta).endswith("generar.py") else types.SimpleNamespace(Apple=lambda: asc)
        with mock.patch.object(widgets, "importar", side_effect=importar), \
                mock.patch("sys.argv", ["apple-widgets.py"]), contextlib.redirect_stdout(io.StringIO()) as salida:
            self.assertEqual(widgets.main(), 0)
        dato = json.loads(salida.getvalue())
        self.assertTrue(dato["registrationReady"])
        self.assertFalse(dato["groupAssociationVerified"])
        self.assertEqual(asc.escrituras, [])

    def test_registra_solo_widget_con_prefijo_y_plataforma_del_host(self):
        asc = Apple()
        widgets.registrar(asc, APP, True, "group.eu.sytue.suite")
        self.assertEqual(asc.escrituras[0], ("bundleIds", {"identifier": "eu.sytue.rooms.widgets", "name": "Sytue Rooms Widgets", "platform": "UNIVERSAL", "seedId": "TEAM"}, None, None))
        self.assertEqual([x[1]["capabilityType"] for x in asc.escrituras[1:]], ["APP_GROUPS", "DATA_PROTECTION"])
        self.assertEqual(asc.escrituras[-1][1]["settings"], widgets.PROTECCION)

    def test_get_con_unica_opcion_seleccionada_no_se_reescribe_ni_exige_enabled_omitido(self):
        asc = Apple()
        widgets.registrar(asc, APP, True, "group.eu.sytue.suite")
        opcion = asc.capacidades[-1]["attributes"]["settings"][0]["options"][0]
        self.assertIs(opcion.pop("enabled"), True)
        asc.escrituras.clear()
        for registrar in (False, True, True):
            with self.subTest(registrar=registrar):
                resultado = widgets.registrar(asc, APP, registrar, "group.eu.sytue.suite")
                self.assertTrue(resultado["registrationReady"])
                self.assertEqual(resultado["capabilities"], ["APP_GROUPS", "DATA_PROTECTION"])
                self.assertFalse(resultado["groupAssociationVerified"])
                self.assertEqual(asc.escrituras, [])
        self.assertIs(widgets.PROTECCION[0]["options"][0]["enabled"], True, "no cambiar el contrato POST/PATCH")

    def test_proteccion_rechaza_proyecciones_ambiguas_y_booleanos_no_verdaderos(self):
        completa = {"key": "COMPLETE_PROTECTION"}
        otra = {"key": "PROTECTED_UNTIL_FIRST_USER_AUTH"}
        opciones = [None, [], {}, "COMPLETE_PROTECTION", [None], [otra], [completa, otra], [completa, completa],
                    [{**completa, "enabledByDefault": True}], [{**completa, "name": "Complete Protection"}],
                    [{**completa, "enabled": True}, otra],
                    [{**completa, "enabled": True}, {**completa, "enabled": False}]]
        opciones += [[{**completa, "enabled": valor}] for valor in (False, None, 0, 1, "true", "false", [], {})]
        for options in opciones:
            with self.subTest(options=options):
                capacidad = {"attributes": {"settings": [{"key": "DATA_PROTECTION_PERMISSION_LEVEL", "options": options}]}}
                self.assertFalse(widgets.completa(capacidad))
        for settings in (None, False, {}, "DATA_PROTECTION_PERMISSION_LEVEL", [None],
                         [{"key": "DATA_PROTECTION_PERMISSION_LEVEL", "options": [completa]}] * 2):
            with self.subTest(settings=settings):
                self.assertFalse(widgets.completa({"attributes": {"settings": settings}}))

    def test_proteccion_booleana_explicita_conserva_alternativas_desactivadas(self):
        capacidad = {"attributes": {"settings": [{"key": "DATA_PROTECTION_PERMISSION_LEVEL", "options": [
            {"key": "COMPLETE_PROTECTION", "enabled": True},
            {"key": "PROTECTED_UNTIL_FIRST_USER_AUTH", "enabled": False}]}]}}
        self.assertTrue(widgets.completa(capacidad))

    def test_registro_relee_capacidades_si_post_omite_los_settings(self):
        asc = Apple()
        escribir = asc.escribir
        def respuesta_minima(tipo, attrs, rels=None, id=None):
            respuesta = escribir(tipo, attrs, rels, id)
            if tipo == "bundleIdCapabilities":
                return {"data": {"id": respuesta["data"]["id"],
                                 "attributes": {"capabilityType": attrs["capabilityType"]}}}
            return respuesta
        with mock.patch.object(asc, "escribir", side_effect=respuesta_minima):
            resultado = widgets.registrar(asc, APP, True, "group.eu.sytue.suite")
        self.assertTrue(resultado["registrationReady"])
        self.assertEqual(resultado["capabilities"], ["APP_GROUPS", "DATA_PROTECTION"])
        self.assertFalse(resultado["groupAssociationVerified"])

    def test_get_sin_proteccion_no_se_da_por_bueno_aunque_post_la_devuelva(self):
        asc = Apple()
        lista = asc.lista
        def lectura_sin_proteccion(ruta):
            if urllib.parse.urlsplit(ruta).path.endswith("/bundleIdCapabilities"):
                return []
            return lista(ruta)
        with mock.patch.object(asc, "lista", side_effect=lectura_sin_proteccion):
            resultado = widgets.registrar(asc, APP, True, "group.eu.sytue.suite")
        self.assertFalse(resultado["registrationReady"])
        self.assertEqual(resultado["capabilities"], [])

    def test_diagnostico_dp_no_expone_ids_nombres_descripciones_ni_valores_extra(self):
        asc = Apple()
        widgets.registrar(asc, APP, True, "group.eu.sytue.suite")
        capacidad = asc.capacidades[-1]
        capacidad["id"] = "ID_PRIVADO_NO_PUBLICAR"
        capacidad["attributes"]["name"] = "NOMBRE_PRIVADO_NO_PUBLICAR"
        setting = capacidad["attributes"]["settings"][0]
        setting["description"] = "DESCRIPCION_PRIVADA_NO_PUBLICAR"
        setting["options"][0]["name"] = "PERSONA_PRIVADA_NO_PUBLICAR"
        setting["options"][0]["id"] = "OTRO_ID_PRIVADO_NO_PUBLICAR"
        resultado = widgets.registrar(asc, APP, False, "group.eu.sytue.suite")
        dato = resultado["dataProtectionDiagnostic"]
        self.assertTrue(dato["present"])
        self.assertEqual(dato["observedCapabilityTypes"], ["APP_GROUPS", "DATA_PROTECTION"])
        opcion = dato["settings"][0]["options"][0]
        self.assertEqual((opcion["key"], opcion["enabled"], opcion["enabledType"]), ("COMPLETE_PROTECTION", True, "bool"))
        self.assertNotIn("NO_PUBLICAR", json.dumps(resultado))

    def test_diagnostico_dp_acota_listas_cadenas_y_muestra_tipos_sin_coercion(self):
        settings = [{"key": "x" * 200, "options": [{"key": "opcion@example.test", "enabled": "true"},
                    {"key": "COMPLETE_PROTECTION", "enabled": {"NO_PUBLICAR": "SECRETO"}}] * 20}] * 20
        capacidades = {"DATA_PROTECTION": {"attributes": {"settings": settings}}}
        dato = widgets.diagnostico_proteccion(capacidades)
        self.assertEqual(len(dato["settings"]), 8)
        self.assertEqual(len(dato["settings"][0]["options"]), 8)
        self.assertEqual(dato["settings"][0]["key"], "<str>")
        opciones = dato["settings"][0]["options"]
        self.assertEqual(opciones[0]["key"], "<str>")
        self.assertEqual((opciones[0]["enabled"], opciones[0]["enabledType"]), ("true", "str"))
        self.assertEqual((opciones[1]["enabled"], opciones[1]["enabledType"]), ("<dict>", "dict"))
        self.assertNotIn("NO_PUBLICAR", json.dumps(dato))
        self.assertFalse(widgets.completa(capacidades["DATA_PROTECTION"]), "diagnosticar no relaja Complete Protection")

    def test_no_muta_host_perfiles_certificados_o_appgroups(self):
        asc = Apple()
        widgets.registrar(asc, APP, True, "group.eu.sytue.suite")
        self.assertTrue(all(t in ("bundleIds", "bundleIdCapabilities") for t, *_ in asc.escrituras))
        self.assertTrue(all(not r or r["bundleId"]["id"] == "widget" for _, _, r, _ in asc.escrituras))

    def test_respuesta_parcial_usa_get_persistido_y_no_reescribe_al_repetir(self):
        asc = Apple()
        escribir_original = asc.escribir
        def escribir(*args, **kwargs):
            respuesta = copy.deepcopy(escribir_original(*args, **kwargs))
            if args[0] == "bundleIdCapabilities":
                respuesta["data"]["attributes"].pop("settings", None)
            return respuesta
        with mock.patch.object(asc, "escribir", side_effect=escribir):
            resultado = widgets.registrar(asc, APP, True, "group.eu.sytue.suite")
            self.assertTrue(resultado["registrationReady"])
            proteccion = resultado["dataProtectionDiagnostic"]["settings"][0]
            self.assertEqual(proteccion["key"], "DATA_PROTECTION_PERMISSION_LEVEL")
            self.assertTrue(proteccion["options"][0]["enabled"])
            lecturas = [r for r in asc.lecturas if "bundleIdCapabilities" in r]
            self.assertEqual(len(lecturas), 2)
            for ruta in lecturas:
                consulta = urllib.parse.parse_qs(urllib.parse.urlsplit(ruta).query)
                self.assertEqual(consulta["fields[bundleIdCapabilities]"], ["capabilityType,settings"])
            asc.escrituras.clear()
            repetido = widgets.registrar(asc, APP, True, "group.eu.sytue.suite")
            self.assertTrue(repetido["registrationReady"])
            self.assertEqual(asc.escrituras, [])

    def test_respuesta_2xx_no_suple_proteccion_ausente_o_desactivada_en_get(self):
        for settings in (None, [{"key": "DATA_PROTECTION_PERMISSION_LEVEL", "options": [
                {"key": "COMPLETE_PROTECTION", "enabled": False}]}]):
            with self.subTest(settings=settings):
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
                    resultado = widgets.registrar(asc, APP, True, "group.eu.sytue.suite")
                self.assertFalse(resultado["registrationReady"])
                self.assertEqual(resultado["capabilities"], ["APP_GROUPS"])
                diagnostico = resultado["dataProtectionDiagnostic"]
                self.assertEqual(diagnostico["settingsType"], "NoneType" if settings is None else "list")
                if settings is not None:
                    self.assertFalse(diagnostico["settings"][0]["options"][0]["enabled"])

    def test_patch_parcial_relee_proteccion_persistida(self):
        asc = Apple()
        widgets.registrar(asc, APP, True, "group.eu.sytue.suite")
        asc.capacidades[-1]["attributes"]["settings"] = None
        asc.escrituras.clear()
        escribir_original = asc.escribir
        def escribir(*args, **kwargs):
            respuesta = copy.deepcopy(escribir_original(*args, **kwargs))
            respuesta["data"]["attributes"].pop("settings", None)
            return respuesta
        with mock.patch.object(asc, "escribir", side_effect=escribir):
            resultado = widgets.registrar(asc, APP, True, "group.eu.sytue.suite")
        self.assertTrue(resultado["registrationReady"])
        self.assertEqual(len(asc.escrituras), 1)
        self.assertEqual(asc.escrituras[0][3], "DATA_PROTECTION")

    def test_get_posterior_fallido_no_acredita_la_escritura(self):
        asc = Apple()
        lista_original = asc.lista
        def lista(ruta):
            if "bundleIdCapabilities" in ruta and asc.escrituras:
                raise RuntimeError("HTTP 503 GET bundleIdCapabilities")
            return lista_original(ruta)
        with mock.patch.object(asc, "lista", side_effect=lista), self.assertRaisesRegex(RuntimeError, "HTTP 503"):
            widgets.registrar(asc, APP, True, "group.eu.sytue.suite")

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
