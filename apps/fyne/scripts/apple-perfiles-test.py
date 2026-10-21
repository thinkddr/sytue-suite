#!/usr/bin/env python3
# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
"""Contrato de provisión: sondeo de sólo lectura y perfiles de la identidad real."""
import base64
import contextlib
import datetime
import hashlib
import importlib.util
import io
import json
import os
import pathlib
import plistlib
import stat
import tempfile
import types
import unittest
from unittest import mock


AQUI = pathlib.Path(__file__).resolve().parent
spec = importlib.util.spec_from_file_location("perfiles", AQUI / "apple-perfiles.py")
p = importlib.util.module_from_spec(spec)
spec.loader.exec_module(p)
GRUPO, LLAVERO = "group.eu.sytue.suite", "VHC5QW7BZ3.eu.sytue.suite"
CADUCA = "2050-01-01T00:00:00Z"
REQUISITO = {"app": "rooms", "id": "eu.sytue.rooms", "widget": False, "group": GRUPO,
             "keychain": LLAVERO, "protection": "",
             "entitlements": {"com.apple.security.application-groups": [GRUPO],
                              "keychain-access-groups": ["VHC5QW7BZ3.eu.sytue.rooms", LLAVERO]}}


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
        if ruta.endswith("/bundleIdCapabilities"):
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


class Perfiles(unittest.TestCase):
    def validar_falso(self, item, requisito, plataforma):
        self.assertEqual(requisito, REQUISITO)
        self.assertEqual(plataforma, "ios")
        return b"perfil publico" if item["attributes"]["authorized"] else None

    def test_sondeo_default_solo_get_no_inspecciona_llaveros(self):
        asc = API()
        asc.perfiles = [perfil()]
        gen = types.SimpleNamespace(apps=lambda: [{"clave": "rooms"}])
        def importar(nombre, ruta):
            return gen if str(ruta).endswith("generar.py") else types.SimpleNamespace(Apple=lambda: asc)
        with mock.patch.dict(p.REGISTRO, {"importar": importar}), mock.patch.object(p, "requisitos", return_value=REQUISITO), \
                mock.patch.object(p, "validar_perfil", side_effect=self.validar_falso), \
                mock.patch.object(p, "identidad_importada", side_effect=AssertionError("sondeo leyó el llavero")), \
                mock.patch("sys.argv", ["apple-perfiles.py", "--plataforma", "ios", "rooms"]), contextlib.redirect_stdout(io.StringIO()) as salida:
            self.assertFalse(p.main())
        dato = json.loads(salida.getvalue())
        self.assertTrue(dato["profile_ready"])
        self.assertEqual(dato["registered_platform"], "UNIVERSAL")
        self.assertFalse(dato["certificate_verified"])
        self.assertFalse(asc.escritos)

    def test_perfil_de_otro_certificado_no_se_reutiliza_y_siguiente_ejecucion_no_duplica(self):
        asc = API()
        asc.perfiles = [perfil("ajeno", "otra-clave")]
        with tempfile.TemporaryDirectory() as tmp, mock.patch.object(p, "validar_perfil", side_effect=self.validar_falso):
            primera = p.preparar(asc, REQUISITO, "ios", "cert-importado", pathlib.Path(tmp))
            segunda = p.preparar(asc, REQUISITO, "ios", "cert-importado", pathlib.Path(tmp))
            self.assertTrue(primera["created"])
            self.assertFalse(segunda["created"])
            self.assertEqual(len(asc.escritos), 1)
            self.assertEqual(asc.escritos[0][2]["certificates"], [{"type": "certificates", "id": "cert-importado"}])
            self.assertEqual(primera["path"], str(pathlib.Path(tmp, "eu.sytue.rooms.mobileprovision")))
            self.assertEqual(pathlib.Path(primera["path"]).read_bytes(), b"perfil publico")
            self.assertEqual(stat.S_IMODE(pathlib.Path(primera["path"]).stat().st_mode), 0o600)

    def test_capacidad_faltante_no_crea_perfiles(self):
        asc = API()
        asc.capacidades = []
        with tempfile.TemporaryDirectory() as tmp:
            resultado = p.preparar(asc, REQUISITO, "ios", "cert-importado", pathlib.Path(tmp))
        self.assertFalse(resultado["profile_ready"])
        self.assertEqual(resultado["reason"], "app_group_capability_missing")
        self.assertFalse(asc.escritos)

    def test_cms_que_no_autoriza_grupo_no_se_guarda(self):
        asc = API()
        def crear(*args, **kwargs):
            return {"data": perfil("creado", autorizado=False)}
        with tempfile.TemporaryDirectory() as tmp, mock.patch.object(asc, "escribir", side_effect=crear), \
                mock.patch.object(p, "validar_perfil", side_effect=self.validar_falso):
            resultado = p.preparar(asc, REQUISITO, "ios", "cert-importado", pathlib.Path(tmp))
            self.assertFalse(resultado["profile_ready"])
            self.assertFalse(list(pathlib.Path(tmp).iterdir()))

    def test_widgets_exigen_proteccion_completa_sin_crear_perfiles_incompletos(self):
        asc = API()
        asc.identificador = "eu.sytue.rooms.widgets"
        requisito = {**REQUISITO, "id": asc.identificador, "widget": True, "keychain": "",
                     "protection": "NSFileProtectionComplete"}
        with tempfile.TemporaryDirectory() as tmp:
            dato = p.preparar(asc, requisito, "ios", "cert-importado", pathlib.Path(tmp))
            self.assertEqual(dato["reason"], "data_protection_complete_missing")
            self.assertFalse(asc.escritos)
        asc.capacidades.append({"attributes": {"capabilityType": "DATA_PROTECTION", "settings": p.REGISTRO["PROTECCION"]}})
        with mock.patch.object(p, "validar_perfil", return_value=b"perfil publico"), tempfile.TemporaryDirectory() as tmp:
            dato = p.preparar(asc, requisito, "ios", "cert-importado", pathlib.Path(tmp))
            self.assertTrue(dato["profile_ready"])
            self.assertTrue(dato["path"].endswith("eu.sytue.rooms.widgets.mobileprovision"))

    def test_identidad_solo_valida_en_llavero_efimero_y_equipo_canonico(self):
        buena, otra = "a" * 40, "b" * 40
        respuestas = ["/tmp/ephemeral.keychain-db\n",
                      ' 1) ' + buena + ' "Apple Distribution: Sytue (VHC5QW7BZ3)"\n 2) ' + otra + ' "Apple Distribution: Otro (ABCDE12345)"']
        with mock.patch.object(p, "capturar", side_effect=respuestas) as capturar:
            self.assertEqual(p.identidad_importada("ios", "VHC5QW7BZ3"), buena)
            self.assertEqual(capturar.call_args_list[1].args[0][-1], "/tmp/ephemeral.keychain-db")
        with mock.patch.object(p, "capturar", side_effect=respuestas):
            with self.assertRaises(ValueError):
                p.identidad_importada("ios", "VHC5QW7BZ3", otra)
        dos = respuestas[1] + '\n 3) ' + "c" * 40 + ' "Apple Distribution: Otro certificado (VHC5QW7BZ3)"'
        with mock.patch.object(p, "capturar", side_effect=[respuestas[0], dos]):
            with self.assertRaises(ValueError):
                p.identidad_importada("ios", "VHC5QW7BZ3")

    def test_certificado_api_se_elige_por_der_no_por_nombre_ni_orden(self):
        asc = API()
        der = b"certificado publico importado"
        def certificado(nombre, contenido, tipo="DISTRIBUTION"):
            return {"id": nombre, "attributes": {"certificateType": tipo, "expirationDate": CADUCA,
                    "certificateContent": base64.b64encode(contenido).decode()}}
        asc.certs = [certificado("otra-clave", b"otra clave"), certificado("correcto", der)]
        huella = hashlib.sha1(der, usedforsecurity=False).hexdigest()
        self.assertEqual(p.certificado_de_identidad(asc, "ios", huella), "correcto")
        asc.certs[1]["attributes"]["expirationDate"] = "2020-01-01T00:00:00Z"
        with self.assertRaises(ValueError):
            p.certificado_de_identidad(asc, "ios", huella)
        self.assertFalse(asc.escritos)

    def test_validador_compartido_comprueba_todos_los_derechos_reales(self):
        contenido = {"Platform": ["iOS"], "TeamIdentifier": ["VHC5QW7BZ3"],
                     "ExpirationDate": datetime.datetime(2050, 1, 1),
                     "Entitlements": {"application-identifier": "VHC5QW7BZ3.eu.sytue.rooms",
                                      "com.apple.security.application-groups": [GRUPO],
                                      "keychain-access-groups": ["VHC5QW7BZ3.*"]}}
        item = perfil()
        item["attributes"]["profileContent"] = base64.b64encode(plistlib.dumps(contenido)).decode()
        with tempfile.TemporaryDirectory() as tmp:
            security = pathlib.Path(tmp, "security")
            security.write_text('#!/usr/bin/env python3\nimport pathlib,sys\nassert sys.argv[1:4]==["cms","-D","-i"]\nsys.stdout.buffer.write(pathlib.Path(sys.argv[4]).read_bytes())\n')
            security.chmod(0o755)
            with mock.patch.dict(os.environ, {"PATH": tmp + os.pathsep + os.environ["PATH"]}):
                self.assertIsNotNone(p.validar_perfil(item, REQUISITO, "ios"))
                incompleto = {**REQUISITO, "entitlements": {**REQUISITO["entitlements"],
                              "com.apple.security.application-groups": [GRUPO, "group.eu.sytue.drive"]}}
                self.assertIsNone(p.validar_perfil(item, incompleto, "ios"))
                contenido["Entitlements"]["keychain-access-groups"] = ["VHC5QW7BZ3.eu.sytue.rooms", LLAVERO]
                item["attributes"]["profileContent"] = base64.b64encode(plistlib.dumps(contenido)).decode()
                claves = {**REQUISITO, "entitlements": {**REQUISITO["entitlements"],
                          "keychain-access-groups": REQUISITO["entitlements"]["keychain-access-groups"] + ["VHC5QW7BZ3.eu.sytue.drive.shared"]}}
                self.assertIsNone(p.validar_perfil(item, claves, "ios"))
                contenido["Entitlements"]["application-identifier"] = "VHC5QW7BZ3.eu.sytue.rooms.widgets"
                contenido["Entitlements"]["com.apple.developer.default-data-protection"] = "NSFileProtectionComplete"
                contenido["Entitlements"]["keychain-access-groups"] = ["VHC5QW7BZ3.*"]
                item["attributes"]["profileContent"] = base64.b64encode(plistlib.dumps(contenido)).decode()
                widget = {**REQUISITO, "id": "eu.sytue.rooms.widgets", "keychain": "", "protection": "NSFileProtectionComplete"}
                self.assertIsNone(p.validar_perfil(item, widget, "ios"))


if __name__ == "__main__":
    unittest.main()
