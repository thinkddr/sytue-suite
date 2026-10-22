#!/usr/bin/env python3
# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
"""Fail-closed validation of the public final-signer build catalog."""
import copy
import importlib.util
import json
from pathlib import Path
import tempfile
import unittest

spec = importlib.util.spec_from_file_location("android_sesion", Path(__file__).with_name("android-sesion.py"))
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)


class CatalogTests(unittest.TestCase):
    def setUp(self):
        self.original = json.loads(module.CATALOG.read_text())

    def parse(self, value):
        with tempfile.TemporaryDirectory() as temporary:
            source = Path(temporary) / "catalog.json"
            source.write_text(json.dumps(value))
            return module.catalog(source)

    def test_reviewed_final_pins_are_package_specific_and_runtime_is_not_enabled(self):
        enabled, apps = self.parse(self.original)
        self.assertFalse(enabled, "Binder/Keystore device verification has not been recorded")
        self.assertEqual(len(apps), 15)
        self.assertEqual(len({pin for pins in apps.values() for pin in pins}), 15)
        self.assertFalse(any("975e779a17f48687ae96be7726a49c96c01180460e6873b3b3e8f303289c5f82" in pins for pins in apps.values()), "AAB upload signer is not proof of an APK signer")
        generated = module.java(enabled, apps)
        self.assertIn("HABILITADO = false", generated)
        self.assertIn("Collections.unmodifiableMap", generated)

    def test_missing_duplicate_or_foreign_packages_never_expand_trust(self):
        candidates = []
        missing = copy.deepcopy(self.original)
        missing["apps"].pop()
        candidates.append(missing)
        duplicate = copy.deepcopy(self.original)
        duplicate["apps"].append(duplicate["apps"][0])
        candidates.append(duplicate)
        foreign = copy.deepcopy(self.original)
        foreign["apps"][0]["package_name"] = "evil.app"
        candidates.append(foreign)
        for invalid in candidates:
            with self.subTest(invalid=invalid["apps"][0]["package_name"]):
                with self.assertRaises(ValueError):
                    self.parse(invalid)

    def test_invalid_fingerprints_and_activation_values_are_rejected(self):
        for value in [[], [""], ["certificate"], ["A" * 64], ["0" * 64, "0" * 64]]:
            invalid = copy.deepcopy(self.original)
            invalid["apps"][0]["certificate_sha256"] = value
            with self.assertRaises(ValueError):
                self.parse(invalid)
        for value in ["true", 1, None]:
            invalid = copy.deepcopy(self.original)
            invalid["runtime_enabled"] = value
            with self.assertRaises(ValueError):
                self.parse(invalid)

    def test_manifest_declares_only_the_fifteen_known_packages(self):
        _, apps = self.parse(self.original)
        query = module.manifest(apps, "queries", "eu.sytue.mail")
        self.assertEqual(query.count("<package "), 15)
        self.assertNotIn("QUERY_ALL_PACKAGES", query)
        provider = module.manifest(apps, "components", "eu.sytue.mail")
        self.assertIn('android:authorities="eu.sytue.mail.sytue.sesion"', provider)
        self.assertIn('android:exported="false"', provider)
        self.assertIn('android:grantUriPermissions="false"', provider)
        with self.assertRaises(ValueError):
            module.manifest(apps, "components", "eu.sytue.fyne.demo")


if __name__ == "__main__":
    unittest.main()
