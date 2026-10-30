#!/usr/bin/env python3
# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
"""Auditoría y notas ASC contra una API simulada, sin red ni credenciales reales."""
import contextlib
import copy
import importlib.util
import io
import json
import pathlib
import sys
import unittest
from unittest import mock
from urllib.parse import parse_qs, urlsplit

sys.path.insert(0, str(pathlib.Path(__file__).parent))
spec = importlib.util.spec_from_file_location("appstore_publicar", pathlib.Path(__file__).with_name("appstore-publicar.py"))
asc = importlib.util.module_from_spec(spec)
spec.loader.exec_module(asc)
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


class NotasRevision(unittest.TestCase):
    def test_mac_no_contamina_ios_y_conserva_notas_de_la_app(self):
        ficha = {"revision": {"notas": "común", "notas_por_plataforma": {"MAC_OS": "listener local"}}}
        datos = {"notas": "detalle de la app"}
        self.assertEqual(asc.notas_revision(ficha, datos, "IOS"), "común\n\ndetalle de la app")
        self.assertEqual(asc.notas_revision(ficha, datos, "MAC_OS"), "común\n\ndetalle de la app\n\nlistener local")

    def test_4000_incluye_el_anexo_y_los_separadores(self):
        ficha = {"revision": {"notas": "x" * 3995, "notas_por_plataforma": {"MAC_OS": "mac"}}}
        self.assertEqual(len(asc.notas_revision(ficha, {}, "MAC_OS")), 4000)
        ficha["revision"]["notas_por_plataforma"]["MAC_OS"] += "x"
        with self.assertRaisesRegex(RuntimeError, "MAC_OS.*4000"):
            asc.notas_revision(ficha, {}, "MAC_OS")
        self.assertEqual(len(asc.notas_revision(ficha, {}, "IOS")), 3995)

    def test_conserva_formato_anterior_sin_anexos(self):
        self.assertEqual(asc.notas_revision({"revision": {"notas": " común "}}, {"notas": " app "}, "IOS"), "común\n\napp")

    def test_rechaza_plataforma_o_texto_mal_configurados(self):
        for por_plataforma in ({"ANDROID": "otro"}, {"MAC_OS": None}, "mac"):
            with self.subTest(por_plataforma=por_plataforma), self.assertRaises(RuntimeError):
                asc.notas_revision({"revision": {"notas": "común", "notas_por_plataforma": por_plataforma}}, {}, "MAC_OS")


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
        self.escrituras.append((id, copy.deepcopy(attrs)))
        for data in self.datos.values():
            if isinstance(data, dict) and data.get("id") == id:
                data["attributes"]["notes"] = attrs["notes"] if self.confirmar else PRIVADO
                return {"data": copy.deepcopy(data)}
        raise AssertionError("se escribe otro detalle")


class NotasSolamente(unittest.TestCase):
    def ejecutar(self, api, apps=("rooms",), plataformas=("IOS", "MAC_OS"), version="3.14.0"):
        out, err = io.StringIO(), io.StringIO()

        class Entorno(dict):
            def get(self, key, default=None):
                if key.startswith(("APPSTORE_CONTACTO_", "APPSTORE_DEMO_")):
                    raise AssertionError("el modo notas leyó datos privados del entorno")
                return super().get(key, default)

        with contextlib.redirect_stdout(out), contextlib.redirect_stderr(err), mock.patch.object(asc.os, "environ", Entorno()):
            resultado = asc.actualizar_notas(api, list(apps), api.ficha, list(plataformas), version)
        self.assertNotIn(PRIVADO, out.getvalue() + err.getvalue())
        return resultado, out.getvalue(), err.getvalue()

    def test_30_destinos_exactos_sin_tocar_credenciales_builds_o_revisiones(self):
        apps = asc.las_quince()
        api = APINotas(apps)
        resultado, out, _ = self.ejecutar(api, apps)
        self.assertEqual(resultado, 0)
        self.assertEqual(len(api.escrituras), 30)
        resumen = [json.loads(linea) for linea in out.splitlines()]
        self.assertEqual(len(resumen), 30)
        self.assertTrue(all(x["notes_equal"] and len(x["notes_sha256"]) == 64 for x in resumen))
        self.assertTrue(all("nueva-" not in ident for ident, _ in api.escrituras))
        self.assertTrue(all("/build" not in ruta and "reviewSubmissions" not in ruta for ruta in api.rutas))
        self.assertTrue(all("fields[appStoreReviewDetails]=notes" in ruta for ruta in api.rutas if "/appStoreReviewDetail" in ruta))
        for data in api.datos.values():
            if isinstance(data, dict) and str(data.get("id", "")).startswith("rd-"):
                self.assertTrue(data["attributes"]["demoAccountRequired"])
                self.assertTrue(all(data["attributes"][campo] == PRIVADO for campo, _ in asc.DATOS_REVISION))

    def test_plataforma_seleccionada_y_sin_submission_guard(self):
        api = APINotas(plataformas=("IOS",))
        api.datos["/v1/reviewSubmissions"] = [{"attributes": {"state": "UNRESOLVED_ISSUES"}}]
        self.assertEqual(self.ejecutar(api, plataformas=("IOS",))[0], 0)
        self.assertEqual([x[0] for x in api.escrituras], ["rd-rooms-IOS"])

    def test_preflight_global_falla_sin_version_detalle_o_estado_editable(self):
        apps = asc.las_quince()
        for fallo in ("version", "ambigua", "detalle", "estado"):
            with self.subTest(fallo=fallo):
                api = APINotas(apps)
                ultima = apps[-1]
                versiones = api.datos[f"/v1/apps/{ultima}-app/appStoreVersions"]
                if fallo == "version":
                    api.datos[f"/v1/apps/{ultima}-app/appStoreVersions"] = [v for v in versiones if v["attributes"]["platform"] != "MAC_OS"]
                elif fallo == "ambigua":
                    versiones.append(copy.deepcopy(next(v for v in versiones if v["id"] == ultima + "-MAC_OS")))
                elif fallo == "detalle":
                    api.datos[f"/v1/appStoreVersions/{ultima}-MAC_OS/appStoreReviewDetail"] = None
                else:
                    next(v for v in versiones if v["id"] == ultima + "-MAC_OS")["attributes"]["appStoreState"] = "IN_REVIEW"
                self.assertEqual(self.ejecutar(api, apps)[0], 1)
                self.assertEqual(api.escrituras, [])

    def test_notas_ya_iguales_no_se_reescriben(self):
        api = APINotas(plataformas=("MAC_OS",))
        esperado = asc.notas_revision(api.ficha, api.ficha["apps"]["rooms"], "MAC_OS")
        api.datos["/v1/appStoreVersions/rooms-MAC_OS/appStoreReviewDetail"]["attributes"]["notes"] = esperado
        resultado, out, _ = self.ejecutar(api, plataformas=("MAC_OS",))
        self.assertEqual(resultado, 0)
        self.assertEqual(api.escrituras, [])
        self.assertEqual(json.loads(out)["changed"], False)
        self.assertTrue(json.loads(out)["notes_equal"])

    def test_recomprueba_estado_destino_y_sanea_errores_antes_del_patch(self):
        for fallo in ("estado", "version_id", "detalle_id", "error"):
            with self.subTest(fallo=fallo):
                api = APINotas(plataformas=("IOS",))
                original = api.lista
                llamadas = 0

                def lista(ruta):
                    nonlocal llamadas
                    llamadas += 1
                    if llamadas == 2:
                        v = api.datos["/v1/apps/rooms-app/appStoreVersions"][1]
                        if fallo == "estado":
                            v["attributes"]["appStoreState"] = "WAITING_FOR_REVIEW"
                        elif fallo == "version_id":
                            v["id"] = "otro-id"
                            api.datos["/v1/appStoreVersions/otro-id/appStoreReviewDetail"] = {"id": "otro-detalle"}
                        elif fallo == "detalle_id":
                            api.datos["/v1/appStoreVersions/rooms-IOS/appStoreReviewDetail"]["id"] = "otro-detalle"
                        else:
                            raise RuntimeError(PRIVADO)
                    return original(ruta)

                api.lista = lista
                resultado, _, _ = self.ejecutar(api, plataformas=("IOS",))
                self.assertEqual(resultado, 1)
                self.assertEqual(api.escrituras, [])

    def test_readback_no_confirmado_es_rojo_sin_volcar_notas(self):
        api = APINotas(plataformas=("IOS",))
        api.confirmar = False
        resultado, out, _ = self.ejecutar(api, plataformas=("IOS",))
        self.assertEqual(resultado, 1)
        self.assertFalse(json.loads(out)["notes_equal"])

    def test_limite_utf8_del_anexo_mac_no_es_solo_numero_de_caracteres(self):
        ficha = {"revision": {"notas": "ñ" * 2000}}
        self.assertEqual(len(asc.notas_revision(ficha, {}, "MAC_OS").encode("utf-8")), 4000)
        ficha["revision"]["notas"] += "ñ"
        with self.assertRaisesRegex(RuntimeError, "MAC_OS.*4000"):
            asc.notas_revision(ficha, {}, "MAC_OS")

    def test_notas_requiere_version_antes_de_credenciales(self):
        out = io.StringIO()
        with contextlib.redirect_stderr(out), mock.patch.object(asc, "Apple", side_effect=AssertionError("leyó credenciales")):
            self.assertEqual(asc.main(["--notas", "rooms"]), 2)
        self.assertIn("notas requiere --version", out.getvalue())


class Preparacion(unittest.TestCase):
    def setUp(self):
        self.api = API()

    def sondear(self, plataformas=("IOS", "MAC_OS")):
        salida = io.StringIO()
        with contextlib.redirect_stdout(salida):
            asc.preparacion(self.api, "rooms", {"apps": {"rooms": {"id": "app"}}}, plataformas, "3.13.5")
        self.assertNotIn(PRIVADO, salida.getvalue())
        return [json.loads(linea) for linea in salida.getvalue().splitlines()]

    def test_reutiliza_sin_crear_version_y_no_expone_datos(self):
        ios, mac = self.sondear()
        self.assertEqual(ios["sourceVersion"], "3.12.10")
        self.assertFalse(ios["targetVersionExists"])
        self.assertTrue(ios["canonicalScreenshotsComplete"])
        self.assertTrue(mac["canonicalScreenshotsComplete"])
        self.assertTrue(all(ios["reviewDetails"].values()))
        self.assertEqual(ios["privacyLabels"], "CHECK_IN_APP_STORE_CONNECT")
        self.assertTrue(ios["common"]["ageRating"])

    def test_distingue_rechazo_pendiente_y_build_antiguo_sin_datos_privados(self):
        self.api.datos["/v1/reviewSubmissions"] = [{"id": "submission", "attributes": {"state": "UNRESOLVED_ISSUES", "userEmail": PRIVADO}}]
        self.api.datos["/v1/reviewSubmissions/submission/items"] = [{"id": "item", "attributes": {"state": "REJECTED", "privateFeedback": PRIVADO}, "relationships": {"appStoreVersion": {"data": {"id": "old"}}}}]
        self.api.datos["/v1/appStoreVersions/old/build"] = {"id": "build", "attributes": {"version": "12", "processingState": "VALID"}}
        self.api.datos["/v1/builds/build/preReleaseVersion"] = {"attributes": {"version": "3.12.10", "private": PRIVADO}}
        datos = self.sondear(("IOS",))[0]
        revision = datos["reviewSubmissions"][0]
        self.assertEqual(revision["state"], "UNRESOLVED_ISSUES")
        self.assertEqual(revision["items"], [{"state": "REJECTED", "version": "3.12.10", "versionState": "REJECTED", "build": "12", "buildState": "VALID", "buildVersion": "3.12.10"}])
        self.assertEqual(datos["reviewFeedback"], "CHECK_IN_APP_STORE_CONNECT")
        self.assertTrue(all("submittedByActor" not in r and "lastUpdatedByActor" not in r for r in self.api.rutas))
        self.assertIn("/v1/builds/build/preReleaseVersion?fields[preReleaseVersions]=version,platform", self.api.rutas)

    def test_review_sin_build_o_con_item_no_version_no_se_inventa(self):
        self.api.datos["/v1/reviewSubmissions"] = [{"id": "submission", "attributes": {"state": "WAITING_FOR_REVIEW"}}]
        self.api.datos["/v1/reviewSubmissions/submission/items"] = [{"attributes": {"state": "ACCEPTED"}, "relationships": {"appStoreVersion": {"data": {"id": "old"}}}}, {"attributes": {"state": "READY_FOR_REVIEW"}}]
        self.api.datos["/v1/appStoreVersions/old/build"] = None
        datos = self.sondear(("IOS",))[0]
        items = datos["reviewSubmissions"][0]["items"]
        self.assertIsNone(items[0]["build"])
        self.assertIsNone(items[0]["buildVersion"])
        self.assertEqual(items[1], {"state": "READY_FOR_REVIEW"})

    def test_captura_en_proceso_no_es_lista(self):
        self.api.datos["/v1/appScreenshotSets/APP_IPHONE_61/appScreenshots"][0]["attributes"]["assetDeliveryState"]["state"] = "UPLOAD_COMPLETE"
        ios, mac = self.sondear()
        self.assertFalse(ios["canonicalScreenshotsComplete"])
        self.assertTrue(mac["canonicalScreenshotsComplete"])

    def test_sin_info_version_precio_o_disponibilidad(self):
        self.api.datos["/v1/apps/app/appInfos"] = []
        self.api.datos["/v1/apps/app/appStoreVersions"] = []
        self.api.datos["/v1/appPriceSchedules/app/manualPrices"] = None
        self.api.datos["/v1/apps/app/appAvailabilityV2"] = None
        ios = self.sondear(("IOS",))[0]
        self.assertIsNone(ios["sourceVersion"])
        self.assertFalse(ios["common"]["appInfo"])
        self.assertFalse(ios["common"]["availability"])
        self.assertFalse(ios["common"]["manualPrice"])

    def test_no_inventa_contacto_ni_cuenta(self):
        self.api.datos["/v1/appStoreVersions/old/appStoreReviewDetail"] = None
        datos = self.sondear(("IOS",))[0]
        self.assertFalse(any(datos["reviewDetails"].values()))
        self.assertIsNone(datos["demoAccountRequired"])

    def test_id_de_otra_app_se_rechaza(self):
        self.api.datos["/v1/apps/app"]["attributes"]["bundleId"] = "eu.otra.app"
        with self.assertRaisesRegex(RuntimeError, "no de"):
            self.sondear()
        self.assertEqual(len(self.api.rutas), 1)

    def test_sin_localizacion_primaria_o_categoria(self):
        self.api.datos["/v1/appInfos/info/primaryCategory"] = None
        self.api.datos["/v1/appStoreVersions/old/appStoreVersionLocalizations"] = []
        datos = self.sondear(("IOS",))[0]
        self.assertFalse(datos["common"]["primaryCategory"])
        self.assertFalse(any(datos["primaryText"].values()))
        self.assertFalse(datos["canonicalScreenshotsComplete"])


if __name__ == "__main__":
    unittest.main()
