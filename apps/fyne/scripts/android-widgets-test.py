#!/usr/bin/env python3
# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
"""Comprueba recursos Android realmente compilados, sin construir las apps Go."""
import argparse
import importlib.util
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import unittest
import xml.etree.ElementTree as ET
import zipfile

SPEC = importlib.util.spec_from_file_location("widgets", Path(__file__).with_name("android-widgets.py"))
WIDGETS = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(WIDGETS)


class ResourceTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix="sytue-widget-test-")
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.apk = self.root / "apk"
        self.app = WIDGETS.FYNE / "cmd/mail"
        self.manifest = self.root / "AndroidManifest.xml"
        self.manifest.write_text('''<manifest xmlns:android="http://schemas.android.com/apk/res/android"
            package="eu.sytue.mail" android:versionCode="77" android:versionName="9.1.2">
            <uses-permission android:name="android.permission.INTERNET" />
            <application android:label="Sytue Mail" android:debuggable="false">
            <activity android:name="org.golang.app.GoNativeActivity" android:exported="true"
                android:launchMode="singleTask">
                <meta-data android:name="android.app.lib_name" android:value="Sytue_Mail" />
                <intent-filter><action android:name="android.intent.action.VIEW" />
                <data android:scheme="sytue-mail" /></intent-filter></activity>
            <service android:name="eu.sytue.nativo.ExistingService" android:exported="false" />
            <provider android:name="eu.sytue.nativo.ExistingProvider"
                android:authorities="eu.sytue.mail.existing" android:exported="false" />
            </application></manifest>''')
        for name in ("classes.dex", "lib/arm64-v8a/libSytue_Mail.so", "assets/existing.txt"):
            dest = self.apk / name
            dest.parent.mkdir(parents=True, exist_ok=True)
            dest.write_bytes(b"existing artifact")
        self.args = argparse.Namespace(appdir=self.app, apk_root=self.apk, manifest=self.manifest,
            aapt2=Path(os.environ["SYTUE_AAPT2"]), android_jar=Path(os.environ["SYTUE_ANDROID_JAR"]),
            min_sdk=24, target_sdk=36)

    def icon(self, relative):
        dest = self.apk / "res" / relative
        dest.parent.mkdir(parents=True, exist_ok=True)
        shutil.copyfile(self.app / "Icon.png", dest)

    def dump(self, entry):
        path = self.root / "resources.apk"
        with zipfile.ZipFile(path, "w") as archive:
            for source in self.apk.rglob("*"):
                if source.is_file():
                    archive.write(source, source.relative_to(self.apk))
        return subprocess.check_output([str(self.args.aapt2), "dump", "xmltree", str(path),
            "--file", entry], text=True)

    def verify(self):
        WIDGETS.prepare(self.args)
        dump = self.dump("AndroidManifest.xml")
        for value in ("eu.sytue.nativo.Widgets", "android.appwidget.provider", "sytue-mail",
                      "Sytue_Mail", "ExistingService", "ExistingProvider", "eu.sytue.mail.existing",
                      "eu.sytue.nativo.RefrescoWidgets", "android.permission.BIND_JOB_SERVICE"):
            self.assertIn(value, dump)
        self.assertIn("versionCode(0x0101021b)=77", dump)
        self.assertIn("minSdkVersion(0x0101020c)=24", dump)
        self.assertIn("targetSdkVersion(0x01010270)=36", dump)
        self.assertRegex(dump, r"resource\(0x01010025\)=@0x7f[0-9a-f]+")
        self.assertRegex(self.dump("res/xml/sytue_widget_info.xml"),
                         r"initialLayout\(0x01010251\)=@0x7f[0-9a-f]+")
        layout = self.dump("res/layout/sytue_widget.xml")
        self.assertIn("TextView", layout)
        for name in ("classes.dex", "lib/arm64-v8a/libSytue_Mail.so", "assets/existing.txt"):
            self.assertEqual((self.apk / name).read_bytes(), b"existing artifact")
        config = json.loads((self.apk / "assets/sytue-widget.json").read_text())
        self.assertEqual(config["actions"], ["open", "compose", "search"])
        self.assertEqual(config["locales"]["es"]["open"], "Abrir")
        self.assertIsInstance(config["locales"]["en"]["unread"], dict)
        self.assertEqual(len(config["locales"]), 26)
        return dump

    def test_legacy_resources_and_manifest_survive(self):
        self.icon("mipmap-xxxhdpi/icon.png")
        self.verify()

    def test_existing_binary_patches_accept_linked_widget_manifest(self):
        self.icon("mipmap-xxxhdpi-v4/icon.png")
        WIDGETS.prepare(self.args)
        script = Path(__file__).with_name("android-binario.py")
        manifest = str(self.apk / "AndroidManifest.xml")
        for command in (("min-sdk", manifest, "24"), ("target-sdk", manifest, "36"),
                        ("servicio-en-primer-plano", manifest, "eu.sytue.nativo.ExistingService", "0x20"),
                        ("imagen-en-imagen", manifest)):
            subprocess.run([sys.executable, str(script), *command], check=True, stdout=subprocess.DEVNULL)
        dump = self.dump("AndroidManifest.xml")
        self.assertIn("foregroundServiceType(0x01010599)=0x00000020", dump)
        self.assertIn("supportsPictureInPicture(0x010104f7)=true", dump)
        self.assertIn("eu.sytue.nativo.Widgets", dump)

    def test_adaptive_and_monochrome_layers_survive(self):
        for layer in ("background", "foreground", "monochrome"):
            self.icon("mipmap-xxxhdpi/ic_launcher_" + layer + ".png")
        for version in (26, 33):
            folder = self.apk / ("res/mipmap-anydpi-v" + str(version))
            folder.mkdir(parents=True)
            for name in ("ic_launcher", "ic_launcher_round"):
                (folder / (name + ".xml")).write_bytes(b"compiled XML")
        self.verify()
        self.assertIn("monochrome", self.dump("res/mipmap-anydpi-v33/ic_launcher.xml"))
        self.assertNotIn("monochrome", self.dump("res/mipmap-anydpi-v26/ic_launcher.xml"))

    def test_unexpected_resources_are_not_silently_deleted(self):
        self.icon("mipmap-xxxhdpi/icon.png")
        folder = self.apk / "res/raw"
        folder.mkdir()
        (folder / "important.txt").write_text("must survive")
        with self.assertRaisesRegex(ValueError, "recurso Fyne nuevo"):
            WIDGETS.prepare(self.args)
        self.assertEqual((folder / "important.txt").read_text(), "must survive")

    def test_demo_is_packaged_without_widgets(self):
        args = type("Args", (), {"appdir": WIDGETS.FYNE / "cmd" / "demo"})()
        self.assertIsNone(WIDGETS.prepare(args))

    def test_catalog_uses_each_native_app_metadata(self):
        seen = set()
        for path in (WIDGETS.FYNE / "cmd").glob("*/FyneApp.toml"):
            if path.parent.name == "demo":
                continue
            config = WIDGETS.metadata(path.parent)
            self.assertNotIn(config["id"], seen)
            seen.add(config["id"])
            self.assertEqual(config["scheme"], "sytue-" + config["product"])
            self.assertIn("search", config["actions"])
            for labels in config["locales"].values():
                self.assertTrue(labels["search"])
                self.assertTrue(labels["calendars"])
                self.assertTrue(labels["address_books"])
                self.assertEqual(labels["updated_at"].count("{time}"), 1)
        self.assertEqual(len(seen), 15)


class FondoTests(unittest.TestCase):
    """Los componentes del widget con la app cerrada, sin aapt2: corre en cualquier sitio."""

    def manifest(self):
        root = ET.fromstring('''<manifest xmlns:android="http://schemas.android.com/apk/res/android"
            package="eu.sytue.mail"><uses-permission android:name="android.permission.INTERNET" />
            <application android:label="Sytue Mail" /></manifest>''')
        return root, root.find("application")

    def named(self, app, tag, name):
        return app.find(tag + "[@" + WIDGETS.ATTR + "name='" + name + "']")

    def test_without_go_only_the_repainting_job(self):
        root, app = self.manifest()
        WIDGETS.fondo(root, app, "eu.sytue.mail", go=False)
        service = self.named(app, "service", "eu.sytue.nativo.RefrescoWidgets")
        self.assertIsNotNone(service)
        self.assertEqual(service.get(WIDGETS.ATTR + "permission"), "android.permission.BIND_JOB_SERVICE")
        self.assertEqual(service.get(WIDGETS.ATTR + "exported"), "false")
        meta = self.named(service, "meta-data", "eu.sytue.widgets.go")
        self.assertEqual(meta.get(WIDGETS.ATTR + "value"), "false")
        self.assertIsNone(self.named(app, "provider", "eu.sytue.nativo.RelevoWidgets"))
        self.assertIsNone(self.named(root, "uses-permission", "android.permission.ACCESS_NETWORK_STATE"))

    def test_with_go_the_relay_and_the_network_permission(self):
        root, app = self.manifest()
        WIDGETS.fondo(root, app, "eu.sytue.mail", go=True)
        meta = self.named(self.named(app, "service", "eu.sytue.nativo.RefrescoWidgets"), "meta-data", "eu.sytue.widgets.go")
        self.assertEqual(meta.get(WIDGETS.ATTR + "value"), "true")
        relay = self.named(app, "provider", "eu.sytue.nativo.RelevoWidgets")
        self.assertEqual(relay.get(WIDGETS.ATTR + "authorities"), "eu.sytue.mail.sytue.widgets")
        self.assertEqual(relay.get(WIDGETS.ATTR + "exported"), "true")
        permissions = [e.get(WIDGETS.ATTR + "name") for e in root.findall("uses-permission")]
        self.assertEqual(permissions, ["android.permission.INTERNET", "android.permission.ACCESS_NETWORK_STATE"])
        self.assertLess(list(root).index(root.findall("uses-permission")[-1]), list(root).index(app))

    def test_components_are_not_declared_twice(self):
        root, app = self.manifest()
        WIDGETS.fondo(root, app, "eu.sytue.mail", go=True)
        with self.assertRaisesRegex(ValueError, "duplicado"):
            WIDGETS.fondo(root, app, "eu.sytue.mail", go=True)

    def test_go_is_on_by_owner_decision(self):
        # Encendido por decisión del dueño (2026-11-07), aún sin la prueba del runbook en un
        # teléfono. Apagarlo es un commit revisado: este test cambia con él.
        self.assertIs(WIDGETS.FONDO_GO, True)


if __name__ == "__main__":
    unittest.main()
