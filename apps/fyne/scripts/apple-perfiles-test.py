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
import shlex
import stat
import sys
import tempfile
import types
import unittest
import urllib.parse
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


class Perfiles(unittest.TestCase):
    def validar_falso(self, item, requisito, plataforma):
        self.assertEqual(requisito, REQUISITO)
        self.assertEqual(plataforma, "ios")
        return b"perfil publico" if item["attributes"]["authorized"] else None

    def recibos(self, plataforma):
        catalogo = {"hub": {"clave": "hub", "id": "eu.sytue.one", "widget_acciones": "open"}}
        catalogo.update({nombre: {"clave": nombre, "id": "eu.sytue." + nombre, "widget_acciones": "open", "archivos": nombre == "drive"}
                         for nombre in ("bases", "calcus", "calendar", "cast", "certipdf", "contacts", "docs",
                                        "drive", "erp", "mail", "notes", "photos", "rooms", "sheets")})
        datos = [{"app": app["clave"], "app_id": app["id"] + sufijo, "platform": plataforma,
                  "profile_type": p.TIPOS[plataforma], "profile_ready": True, "certificate_verified": True,
                  "created": bool(sufijo), "path": "no-publicar-perfil-cms", "otro": "no-publicar"}
                 for app in catalogo.values() for sufijo, _, _ in p.destinos_app(app)]
        return catalogo, datos

    def test_recibo_coteja_todos_los_destinos_declarados_y_solo_emite_campos_seguros_sin_api(self):
        for plataforma in p.TIPOS:
            catalogo, datos = self.recibos(plataforma)
            gen = types.SimpleNamespace(apps=lambda: catalogo.values())
            with self.subTest(plataforma=plataforma), tempfile.TemporaryDirectory() as tmp:
                ruta = pathlib.Path(tmp, "recibo.jsonl")
                ruta.write_text("".join(json.dumps(dato) + "\n" for dato in datos))
                with mock.patch.dict(p.REGISTRO, {"importar": mock.Mock(return_value=gen)}) as registro, \
                        mock.patch.object(p, "requisitos", side_effect=AssertionError("consultó derechos")), \
                        mock.patch.object(p, "identidad_importada", side_effect=AssertionError("consultó llavero")), \
                        mock.patch("sys.argv", ["apple-perfiles.py", "--plataforma", plataforma,
                                                "--comprobar-recibo", str(ruta)]), \
                        contextlib.redirect_stdout(io.StringIO()) as salida:
                    self.assertFalse(p.main())
                    registro["importar"].assert_called_once()
                lineas = [json.loads(linea) for linea in salida.getvalue().splitlines()]
                self.assertEqual(len(lineas), 32)
                self.assertEqual(lineas[-1], {"platform": plataforma, "profile_count": 31, "ready_count": 31,
                                              "created_count": 16})
                self.assertEqual(set(lineas[0]), {"app", "app_id", "platform", "profile_type", "profile_ready",
                                                 "certificate_verified", "created"})
                self.assertNotIn("no-publicar", salida.getvalue())

    def test_recibo_rechaza_ausencias_duplicados_identidad_tipo_certificado_y_booleanos_ambiguos(self):
        catalogo, originales = self.recibos("ios")
        casos = [("incompleto", originales[:-1]), ("duplicado", originales[:-1] + [originales[0]])]
        for campo, valores in {"profile_ready": (False, None, 1, "true"),
                               "certificate_verified": (False, None, 1, "true"),
                               "created": (None, 0, "false"), "app": ("drive",),
                               "app_id": ("ajeno",), "platform": ("developerid",),
                               "profile_type": ("MAC_APP_DIRECT",)}.items():
            for valor in valores:
                casos.append((campo + repr(valor), [{**originales[0], campo: valor}, *originales[1:]]))
        casos.extend((("demasiados", originales + [originales[0]]), ("objeto", [None, *originales[1:]])))
        for nombre, datos in casos:
            with self.subTest(nombre=nombre), tempfile.TemporaryDirectory() as tmp:
                ruta = pathlib.Path(tmp, "recibo.jsonl")
                ruta.write_text("".join(json.dumps(dato) + "\n" for dato in datos))
                with self.assertRaises(ValueError), contextlib.redirect_stdout(io.StringIO()) as salida:
                    p.comprobar_recibo(ruta, catalogo, "ios")
                self.assertEqual(salida.getvalue(), "")

    def test_fileprovider_es_obligatorio_solo_en_apps_que_lo_declaran(self):
        catalogo, datos = self.recibos("ios")
        self.assertEqual([d["app_id"] for d in datos if d["app_id"].endswith(".archivos")], ["eu.sytue.drive.archivos"])
        with tempfile.TemporaryDirectory() as tmp:
            ruta = pathlib.Path(tmp, "recibo.jsonl")
            ruta.write_text("".join(json.dumps(d) + "\n" for d in datos if not d["app_id"].endswith(".archivos")))
            with self.assertRaisesRegex(ValueError, "incompleto"):
                p.comprobar_recibo(ruta, catalogo, "ios")

    def test_modo_recibo_rechaza_inputs_mezclados_antes_de_importar(self):
        for extra in (["--preparar", "--salida", "tmp"], ["--salida", "tmp"], ["--seco"],
                      ["--certificado-sha1", "a" * 40], ["--destinos", "todos"], ["rooms"]):
            importar = mock.Mock()
            with self.subTest(extra=extra), mock.patch.dict(p.REGISTRO, {"importar": importar}), \
                    mock.patch("sys.argv", ["apple-perfiles.py", "--plataforma", "ios", "--comprobar-recibo", "recibo", *extra]), \
                    contextlib.redirect_stderr(io.StringIO()), self.assertRaises(SystemExit) as error:
                p.main()
            self.assertEqual(error.exception.code, 2)
            importar.assert_not_called()

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

    def test_preparar_usa_el_der_importado_para_seleccionar_el_certificado_api(self):
        asc, der = API(), b"certificado publico importado"
        asc.perfiles, asc.certs = [perfil()], [certificado_api("cert-importado", der)]
        gen = types.SimpleNamespace(EQUIPO="VHC5QW7BZ3", apps=lambda: [{"clave": "rooms"}])
        def importar(nombre, ruta):
            return gen if str(ruta).endswith("generar.py") else types.SimpleNamespace(Apple=lambda: asc)
        with tempfile.TemporaryDirectory() as tmp, mock.patch.dict(p.REGISTRO, {"importar": importar}), \
                mock.patch.object(p, "requisitos", return_value=REQUISITO), \
                mock.patch.object(p, "validar_perfil", side_effect=self.validar_falso), \
                mock.patch.object(p, "identidad_importada", return_value=der) as identidad, \
                mock.patch("sys.argv", ["apple-perfiles.py", "--plataforma", "ios", "--preparar", "--salida", tmp,
                                        "--certificado-sha1", "a" * 40, "rooms"]), contextlib.redirect_stdout(io.StringIO()) as salida:
            self.assertFalse(p.main())
            self.assertEqual(pathlib.Path(tmp, "eu.sytue.rooms.mobileprovision").read_bytes(), b"perfil publico")
        identidad.assert_called_once_with("ios", "VHC5QW7BZ3", "a" * 40)
        self.assertTrue(json.loads(salida.getvalue())["certificate_verified"])
        self.assertFalse(asc.escritos)

    def test_cliente_solo_existentes_rechaza_todas_las_escrituras_antes_de_autenticar(self):
        pedir = mock.Mock(return_value={"data": []})

        class Base:
            def pedir(self, *args, **kwargs):
                return pedir(*args, **kwargs)

            def escribir(self):
                return self.pedir("POST", "/v1/profiles", {"data": {}})

        asc = p.cliente_apple(types.SimpleNamespace(Apple=Base), True)
        for metodo, cuerpo in (("POST", {}), ("PATCH", {}), ("DELETE", None), ("GET", {})):
            with self.subTest(metodo=metodo, cuerpo=cuerpo), self.assertRaisesRegex(RuntimeError, "sólo admite consultas GET"):
                asc.pedir(metodo, "/v1/profiles", cuerpo)
        with self.assertRaises(RuntimeError):
            asc.escribir()
        pedir.assert_not_called()
        self.assertEqual(asc.pedir("GET", "/v1/profiles"), {"data": []})
        pedir.assert_called_once_with("GET", "/v1/profiles", tolerar=())

    def test_solo_existentes_no_crea_si_falta_certificado_vigencia_tipo_o_derechos(self):
        invalido, caducado, tipo = perfil(), perfil(), perfil()
        invalido["attributes"]["profileState"] = "INVALID"
        caducado["attributes"]["expirationDate"] = "2020-01-01T00:00:00Z"
        tipo["attributes"]["profileType"] = "IOS_APP_DEVELOPMENT"
        for perfiles in ([], [perfil(certificado="otra-clave")], [invalido], [caducado], [tipo], [perfil(autorizado=False)]):
            asc = API()
            asc.perfiles = perfiles
            with self.subTest(perfiles=perfiles), tempfile.TemporaryDirectory() as tmp, \
                    mock.patch.object(p, "validar_perfil", side_effect=self.validar_falso):
                resultado = p.preparar(asc, REQUISITO, "ios", "cert-importado", pathlib.Path(tmp), True)
                self.assertFalse(resultado["profile_ready"])
                self.assertFalse(resultado["created"])
                self.assertEqual(resultado["reason"], "existing_profile_missing_or_invalid")
                self.assertFalse(asc.escritos)
                self.assertFalse(list(pathlib.Path(tmp).iterdir()))

    def test_solo_existentes_reutiliza_y_guarda_solo_despues_de_validar_el_perfil(self):
        asc = API()
        asc.perfiles = [perfil()]
        with tempfile.TemporaryDirectory() as tmp, mock.patch.object(p, "validar_perfil", side_effect=self.validar_falso) as cms:
            resultado = p.preparar(asc, REQUISITO, "ios", "cert-importado", pathlib.Path(tmp), True)
            self.assertTrue(resultado["profile_ready"])
            self.assertTrue(resultado["certificate_verified"])
            self.assertFalse(resultado["created"])
            self.assertEqual(pathlib.Path(resultado["path"]).read_bytes(), b"perfil publico")
            self.assertEqual(stat.S_IMODE(pathlib.Path(resultado["path"]).stat().st_mode), 0o600)
            cms.assert_called_once()
            self.assertFalse(asc.escritos)

    def test_recibo_solo_existentes_exige_31_perfiles_no_creados_por_plataforma(self):
        for plataforma in p.TIPOS:
            catalogo, datos = self.recibos(plataforma)
            datos = [{**dato, "created": False} for dato in datos]
            with self.subTest(plataforma=plataforma), tempfile.TemporaryDirectory() as tmp:
                ruta = pathlib.Path(tmp, "recibo.jsonl")
                ruta.write_text("".join(json.dumps(dato) + "\n" for dato in datos))
                with contextlib.redirect_stdout(io.StringIO()) as salida:
                    self.assertFalse(p.comprobar_recibo(ruta, catalogo, plataforma, True))
                self.assertEqual(json.loads(salida.getvalue().splitlines()[-1]),
                                 {"platform": plataforma, "profile_count": 31, "ready_count": 31, "created_count": 0})
                datos[-1]["created"] = True
                ruta.write_text("".join(json.dumps(dato) + "\n" for dato in datos))
                with self.assertRaisesRegex(ValueError, "perfil creado"), contextlib.redirect_stdout(io.StringIO()) as salida:
                    p.comprobar_recibo(ruta, catalogo, plataforma, True)
                self.assertEqual(salida.getvalue(), "")

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

    def test_created_date_opcional_no_impide_validar_y_reutilizar_perfiles(self):
        for fechas in ((None, None), (None, CADUCA), ("ausente", CADUCA), (42, CADUCA),
                       ({"fecha": CADUCA}, None), (False, CADUCA)):
            with self.subTest(fechas=fechas):
                asc = API()
                asc.perfiles = [perfil("primero", "otro-certificado"), perfil("cms-invalido", autorizado=False),
                                perfil("vigente")]
                for item, fecha in zip(asc.perfiles, (*fechas, fechas[-1]), strict=True):
                    if fecha == "ausente":
                        item["attributes"].pop("createdDate")
                    else:
                        item["attributes"]["createdDate"] = fecha
                with tempfile.TemporaryDirectory() as tmp, \
                        mock.patch.object(p, "validar_perfil", side_effect=self.validar_falso) as cms:
                    resultado = p.preparar(asc, REQUISITO, "ios", "cert-importado", pathlib.Path(tmp))
                    self.assertTrue(resultado["profile_ready"])
                    self.assertFalse(resultado["created"])
                    self.assertFalse(asc.escritos)
                    self.assertEqual(pathlib.Path(resultado["path"]).read_bytes(), b"perfil publico")
                    self.assertEqual([llamada.args[0]["id"] for llamada in cms.call_args_list],
                                     ["cms-invalido", "vigente"])

    def test_perfiles_piden_los_settings_y_rechazan_capacidades_duplicadas(self):
        asc = API()
        p.preparar(asc, REQUISITO, "ios")
        ruta = next(r for r in asc.gets if "bundleIdCapabilities" in r)
        self.assertEqual(urllib.parse.parse_qs(urllib.parse.urlsplit(ruta).query),
                         {"fields[bundleIdCapabilities]": ["capabilityType,settings"]})
        asc.capacidades *= 2
        with self.assertRaisesRegex(RuntimeError, "capacidad repetida"):
            p.preparar(asc, REQUISITO, "ios")
        self.assertFalse(asc.escritos)

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
            nuevo = perfil("creado", autorizado=False)
            asc.perfiles.append(nuevo)
            return {"data": nuevo}
        with tempfile.TemporaryDirectory() as tmp, mock.patch.object(asc, "escribir", side_effect=crear), \
                mock.patch.object(p, "validar_perfil", side_effect=self.validar_falso):
            resultado = p.preparar(asc, REQUISITO, "ios", "cert-importado", pathlib.Path(tmp))
            self.assertFalse(resultado["profile_ready"])
            self.assertFalse(list(pathlib.Path(tmp).iterdir()))

    def test_perfil_nuevo_exige_tipo_activo_vigente_y_certificado_importado(self):
        casos = (("profileType", "IOS_APP_DEVELOPMENT"), ("profileState", "INVALID"),
                 ("expirationDate", "2020-01-01T00:00:00Z"), ("certificate", "otra-clave"))
        for campo, valor in casos:
            with self.subTest(campo=campo):
                asc = API()
                crear_original = asc.escribir
                def crear(*args, **kwargs):
                    respuesta = crear_original(*args, **kwargs)
                    nuevo = respuesta["data"]
                    (nuevo if campo == "certificate" else nuevo["attributes"])[campo] = valor
                    return respuesta
                with tempfile.TemporaryDirectory() as tmp, mock.patch.object(asc, "escribir", side_effect=crear), \
                        mock.patch.object(p, "validar_perfil", side_effect=self.validar_falso) as cms:
                    resultado = p.preparar(asc, REQUISITO, "ios", "cert-importado", pathlib.Path(tmp))
                    self.assertTrue(resultado["created"])
                    self.assertFalse(resultado["profile_ready"])
                    self.assertEqual(resultado["reason"], "created_profile_invalid_or_wrong_certificate")
                    self.assertFalse(list(pathlib.Path(tmp).iterdir()))
                    cms.assert_not_called()

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

    def test_proyeccion_seleccionada_no_suple_el_cms_de_proteccion_completa(self):
        asc = API()
        asc.identificador = "eu.sytue.rooms.widgets"
        asc.capacidades.append({"attributes": {"capabilityType": "DATA_PROTECTION", "settings": [
            {"key": "DATA_PROTECTION_PERMISSION_LEVEL", "options": [{"key": "COMPLETE_PROTECTION"}]}]}})
        asc.perfiles = [perfil()]
        requisito = {**REQUISITO, "id": asc.identificador, "widget": True, "keychain": "",
                     "protection": "NSFileProtectionComplete"}
        with tempfile.TemporaryDirectory() as tmp, mock.patch.object(p, "validar_perfil", return_value=None) as cms:
            resultado = p.preparar(asc, requisito, "ios", "cert-importado", pathlib.Path(tmp))
            self.assertTrue(cms.called)
            self.assertFalse(resultado["profile_ready"])
            self.assertFalse(list(pathlib.Path(tmp).iterdir()))
        asc.escritos.clear()
        with tempfile.TemporaryDirectory() as tmp, mock.patch.object(p, "validar_perfil", return_value=b"perfil publico") as cms:
            resultado = p.preparar(asc, requisito, "ios", "cert-importado", pathlib.Path(tmp))
            self.assertTrue(cms.called)
            self.assertTrue(resultado["profile_ready"])
            self.assertFalse(asc.escritos, "reutilizar el perfil que sí autoriza los derechos")

    def test_identidad_solo_valida_en_llavero_efimero_y_equipo_canonico(self):
        buena, otra = "a" * 40, "b" * 40
        der = b"certificado publico importado"
        respuestas = ["/tmp/ephemeral.keychain-db\n",
                      ' 1) ' + buena + ' "Apple Distribution: Sytue (VHC5QW7BZ3)"\n 2) ' + otra + ' "Apple Distribution: Otro (ABCDE12345)"',
                      exportado(b"otro equipo", otra) + exportado(der, buena)]
        with mock.patch.object(p, "capturar", side_effect=respuestas) as capturar:
            self.assertEqual(p.identidad_importada("ios", "VHC5QW7BZ3"), der)
            self.assertEqual(capturar.call_args_list[1].args[0][-1], "/tmp/ephemeral.keychain-db")
            self.assertEqual(capturar.call_args_list[2].args[0],
                             ["security", "find-certificate", "-a", "-Z", "-p", "/tmp/ephemeral.keychain-db"])
        with mock.patch.object(p, "capturar", side_effect=respuestas):
            self.assertEqual(p.identidad_importada("ios", "VHC5QW7BZ3", ":".join(["AA"] * 20)), der)
        with mock.patch.object(p, "capturar", side_effect=respuestas):
            with self.assertRaises(ValueError):
                p.identidad_importada("ios", "VHC5QW7BZ3", otra)
        dos = respuestas[1] + '\n 3) ' + "c" * 40 + ' "Apple Distribution: Otro certificado (VHC5QW7BZ3)"'
        with mock.patch.object(p, "capturar", side_effect=[respuestas[0], dos]):
            with self.assertRaises(ValueError):
                p.identidad_importada("ios", "VHC5QW7BZ3")

    def test_selector_ambiguo_no_autentica_certificados_con_sha1_coincidente(self):
        selector = "a" * 40
        identidad = ' 1) ' + selector + ' "Apple Distribution: Sytue (VHC5QW7BZ3)"'
        # Simula dos DER con el mismo selector SHA-1, cada uno con su SHA-256 real.
        salida = exportado(b"certificado original", selector) + exportado(b"certificado colisionado", selector)
        with mock.patch.object(p, "capturar", side_effect=["/tmp/ephemeral.keychain-db", identidad, salida]):
            with self.assertRaisesRegex(ValueError, "certificado único"):
                p.identidad_importada("ios", "VHC5QW7BZ3", selector)

    def test_exportacion_invalida_o_sin_certificado_no_se_acepta(self):
        identidad = ' 1) ' + "a" * 40 + ' "Apple Distribution: Sytue (VHC5QW7BZ3)"'
        certificado = exportado(b"certificado original")
        alterado = certificado.replace(hashlib.sha256(b"certificado original").hexdigest().upper(), "0" * 64)
        for salida in ("", exportado(b"otro", "b" * 40), alterado, certificado.replace("SHA-1 hash:", "hash:"),
                       "error de exportación\n" + certificado, certificado.replace("b3JpZ2luYWw=", "@@@")):
            with self.subTest(salida=salida), mock.patch.object(p, "capturar", side_effect=["/tmp/ephemeral.keychain-db", identidad, salida]):
                with self.assertRaises(ValueError):
                    p.identidad_importada("ios", "VHC5QW7BZ3")

    def test_selector_sigue_siendo_sha1_no_sha256(self):
        identidad = ' 1) ' + "a" * 40 + ' "Apple Distribution: Sytue (VHC5QW7BZ3)"'
        for selector in ("a" * 39, "a" * 41, "a" * 64, "g" * 40):
            with self.subTest(selector=selector), mock.patch.object(p, "capturar", side_effect=["/tmp/ephemeral.keychain-db", identidad]):
                with self.assertRaises(ValueError):
                    p.identidad_importada("ios", "VHC5QW7BZ3", selector)

    def test_certificado_api_se_elige_por_der_no_por_nombre_ni_orden(self):
        asc = API()
        der = b"certificado publico importado"
        asc.certs = [certificado_api("otra-clave", b"otra clave"), certificado_api("correcto", der)]
        with mock.patch.object(p.hashlib, "sha1", side_effect=AssertionError("SHA-1 no autentica certificados remotos")):
            self.assertEqual(p.certificado_de_identidad(asc, "ios", der), "correcto")
        asc.certs[1]["attributes"]["expirationDate"] = "2020-01-01T00:00:00Z"
        with self.assertRaises(ValueError):
            p.certificado_de_identidad(asc, "ios", der)
        self.assertFalse(asc.escritos)

    def test_certificado_remoto_distinto_no_se_acepta_aunque_sha1_colisionara(self):
        asc = API()
        asc.certs = [certificado_api("colisionado", b"certificado colisionado")]
        with mock.patch.object(p.hashlib, "sha1", return_value=types.SimpleNamespace(hexdigest=lambda: "a" * 40)):
            with self.assertRaisesRegex(ValueError, "vigente único"):
                p.certificado_de_identidad(asc, "ios", b"certificado original")
        self.assertFalse(asc.escritos)

    def test_certificados_api_inactivos_incorrectos_invalidos_o_duplicados_no_se_aceptan(self):
        der = b"certificado original"
        opciones = ([certificado_api("inactivo", der, activated=False)],
                    [certificado_api("otro-tipo", der, certificateType="DEVELOPER_ID_APPLICATION")],
                    [certificado_api("invalido", der, certificateContent="@@@")],
                    [certificado_api("duplicado-1", der), certificado_api("duplicado-2", der)])
        for certificados in opciones:
            with self.subTest(certificados=certificados):
                asc = API()
                asc.certs = certificados
                with self.assertRaises(ValueError):
                    p.certificado_de_identidad(asc, "ios", der)
                self.assertFalse(asc.escritos)
        for identidad in (b"", "a" * 40, None):
            with self.subTest(identidad=identidad):
                asc = API()
                with self.assertRaises(ValueError):
                    p.certificado_de_identidad(asc, "ios", identidad)
                self.assertFalse(asc.gets)

    def test_validador_compartido_comprueba_todos_los_derechos_reales(self):
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

    def test_identidad_omitida_por_find_identity_v_se_acepta_solo_si_verify_cert_local_pasa(self):
        selector, otra = "a" * 40, "b" * 40
        der = b"developer id g2 publico"
        candidata = ' 1) ' + selector + ' "Developer ID Application: Sytue (VHC5QW7BZ3)" (CSSMERR_TP_NOT_TRUSTED)\n     0 valid identities found'
        pem_exportado = exportado(der, selector)
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
                    self.assertEqual(ruta_c.parent, pathlib.Path(job_dir))
                    self.assertEqual(cmd[cmd.index("-k") + 1], llavero)
                    self.assertIn("-----BEGIN CERTIFICATE-----", ruta_c.read_text())
                    vistos_cert.append(ruta_c)
                    return ""
                raise AssertionError("orden inesperada: " + repr(cmd))

            with mock.patch.dict(os.environ, {"SYTUE_APPLE_JOB_DIR": job_dir}), \
                    mock.patch.object(p, "capturar", side_effect=capturar_valido):
                self.assertEqual(p.identidad_importada("developerid", "VHC5QW7BZ3"), der)
            self.assertEqual(len(vistos_cert), 1)
            self.assertFalse(vistos_cert[0].exists())

            def capturar_rechazado(argv):
                cmd = [str(a) for a in argv]
                if cmd[:2] == ["security", "verify-cert"]:
                    vistos_cert.append(pathlib.Path(cmd[cmd.index("-c") + 1]))
                    raise p.subprocess.CalledProcessError(1, cmd)
                return capturar_valido(argv)

            with mock.patch.dict(os.environ, {"SYTUE_APPLE_JOB_DIR": job_dir}), \
                    mock.patch.object(p, "capturar", side_effect=capturar_rechazado):
                with self.assertRaisesRegex(ValueError, "identidad válida única"):
                    p.identidad_importada("developerid", "VHC5QW7BZ3")
            self.assertFalse(vistos_cert[-1].exists())

            ambiguas = candidata + '\n 2) ' + otra + ' "Developer ID Application: Otro (VHC5QW7BZ3)" (CSSMERR_TP_NOT_TRUSTED)'
            with mock.patch.dict(os.environ, {"SYTUE_APPLE_JOB_DIR": job_dir}), \
                    mock.patch.object(p, "capturar", side_effect=[llavero + "\n", "     0 valid identities found\n", ambiguas]):
                with self.assertRaisesRegex(ValueError, "identidad válida única"):
                    p.identidad_importada("developerid", "VHC5QW7BZ3")


if __name__ == "__main__":
    unittest.main()
