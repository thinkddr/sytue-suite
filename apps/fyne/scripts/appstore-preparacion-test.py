#!/usr/bin/env python3
# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
"""Auditoría ASC sin red ni credenciales: sólo GET, presencia y estados de capturas."""
import contextlib
import copy
import importlib.util
import io
import json
import pathlib
import sys
import unittest

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
