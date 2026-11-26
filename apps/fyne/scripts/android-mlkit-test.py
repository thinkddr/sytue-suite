#!/usr/bin/env python3
# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
"""Lo de android-mlkit.py que no necesita red: el orden de versiones, el fichero fijado y la
revisión de lo que cada AAR pide al manifiesto. Lo corre `empaquetar.sh android` con ML Kit."""
import importlib.util
from pathlib import Path
import tempfile
import unittest

SPEC = importlib.util.spec_from_file_location("mlkit", Path(__file__).with_name("android-mlkit.py"))
MLKIT = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(MLKIT)

AAR = '''<manifest xmlns:android="http://schemas.android.com/apk/res/android"
    xmlns:tools="http://schemas.android.com/tools" package="com.ejemplo">
    <uses-sdk android:minSdkVersion="21" />%s
</manifest>'''


class VersionTests(unittest.TestCase):
    def test_gradle_order(self):
        for menor, mayor in (("1.0.0-beta5", "1.0.0"), ("1.0.0-beta4", "1.0.0-beta5"),
                             ("1.0.0-alpha2", "1.0.0-beta1"), ("2.2.20", "2.4.0"), ("1.9.0", "1.10.0"),
                             ("1.0", "9999.0-empty-to-avoid-conflict-with-guava"), ("32.0.0", "32.0.0-jre")):
            self.assertEqual(MLKIT.mayor(menor, mayor), mayor, (menor, mayor))
            self.assertEqual(MLKIT.mayor(mayor, menor), mayor, (mayor, menor))
        self.assertEqual(MLKIT._trozos("1.0"), MLKIT._trozos("1.0.0"))


class FijadoTests(unittest.TestCase):
    def test_the_pinned_tree(self):
        filas = MLKIT.leer_fijado()
        vistos = set()
        for g, a, v, tipo, repo, suma, licencia in filas:
            self.assertNotIn((g, a), vistos, "%s:%s dos veces" % (g, a))
            vistos.add((g, a))
            self.assertNotEqual(licencia.strip(), "-", "%s:%s:%s sin licencia revisada" % (g, a, v))
            if tipo == "pom":
                self.assertEqual(suma, "-")
        self.assertIn(MLKIT.RAIZ[:2], vistos)
        # Una sola stdlib de Kotlin: con las jdk7/jdk8 viejas al lado, d8 ve clases repetidas.
        self.assertNotIn(("org.jetbrains.kotlin", "kotlin-stdlib-jdk7"), vistos)

    def test_a_bad_sum_is_refused(self):
        with tempfile.TemporaryDirectory() as temp:
            ruta = Path(temp) / "dependencias.tsv"
            ruta.write_text("com.google.mlkit:genai-prompt:1.0.0-beta5\taar\tgoogle\tnosuma\tML Kit\n")
            with self.assertRaises(SystemExit):
                MLKIT.leer_fijado(ruta)


class ManifiestoTests(unittest.TestCase):
    def test_reviewed_components_pass(self):
        _, declaraciones = MLKIT._declaraciones((AAR % '''
            <uses-permission android:name="com.google.android.apps.aicore.service.BIND_SERVICE" />
            <queries><package android:name="com.google.android.aicore" /></queries>
            <application><provider android:name="com.google.mlkit.common.internal.MlKitInitProvider"
                android:authorities="${applicationId}.mlkitinitprovider" tools:node="merge" /></application>''').encode(),
            "prueba")
        self.assertEqual({(c, n) for c, n, _ in declaraciones}, {
            ("uses-permission", "com.google.android.apps.aicore.service.BIND_SERVICE"),
            ("queries/package", "com.google.android.aicore"),
            ("provider", "com.google.mlkit.common.internal.MlKitInitProvider")})

    def test_an_unreviewed_component_stops_packaging(self):
        for nuevo in ('<application><service android:name="com.google.Nuevo" /></application>',
                      '<uses-permission android:name="android.permission.READ_CONTACTS" />',
                      '<application android:usesCleartextTraffic="true" />',
                      '<uses-feature android:name="android.hardware.camera" />'):
            with self.assertRaises(SystemExit, msg=nuevo):
                MLKIT._declaraciones((AAR % nuevo).encode(), "prueba")

    def test_the_metrics_channel_is_never_kept(self):
        canal = {(c, n) for c, n in MLKIT.DESCARTADOS if "datatransport" in n}
        self.assertEqual(len(canal), 3)
        self.assertFalse(canal & MLKIT.COMPONENTES)


if __name__ == "__main__":
    unittest.main()
