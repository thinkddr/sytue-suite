#!/usr/bin/env python3
# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
"""Signing probe regressions without keys, keychain access, or real signing."""

import importlib.util
import pathlib
import ssl
import subprocess
import tempfile
import unittest


AQUI = pathlib.Path(__file__).resolve().parent
SPEC = importlib.util.spec_from_file_location("sonda", AQUI / "apple_firma_sonda.py")
s = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(s)
NOMBRE, LLAVERO = "Developer ID Application: Sytue", "/tmp/probe.keychain-db"
SELECTOR, OTRO_SELECTOR = "A" * 40, "B" * 40
DER, OTRO_DER = b"expected public certificate", b"another public certificate"


def exportado(der=DER, selector=SELECTOR):
    return (f"SHA-256 hash: {s.huella(der)}\nSHA-1 hash: {selector}\n"
            + ssl.DER_cert_to_PEM_cert(der))


class Herramientas:
    def __init__(self):
        self.comandos = []
        self.certificados = exportado()
        self.globales = False
        self.hojas = {}
        self.verificar = self.extraer = self.firmar = self.exportar = self.identidades = 0
        self.sin_hoja = False
        self.firmas = 0
        self.firmas_fallidas = set()

    def __call__(self, argv):
        self.comandos.append(argv)
        status, output = 0, ""
        if argv[:2] == ["codesign", "-f"]:
            self.firmas += 1
            status = self.firmar or int(self.firmas in self.firmas_fallidas)
        elif argv[:2] == ["codesign", "--verify"]:
            status = self.verificar
        elif argv[:2] == ["codesign", "-d"]:
            status = self.extraer
            prefijo = argv[2].split("=", 1)[1]
            indice = int(pathlib.Path(argv[-1]).name.split("-")[-1])
            if not self.sin_hoja:
                pathlib.Path(prefijo + "0").write_bytes(self.hojas.get(indice, DER))
            # An expected certificate at index 1 must not replace a missing leaf.
            pathlib.Path(prefijo + "1").write_bytes(DER)
        elif argv[:2] == ["security", "find-identity"]:
            status = self.identidades
            output = f' 1) {SELECTOR} "{NOMBRE}"\n'
            if self.globales and argv[-1] != LLAVERO:
                output += f' 2) {OTRO_SELECTOR} "{NOMBRE}"\n'
        elif argv[:2] == ["security", "find-certificate"]:
            if argv[-1] == LLAVERO:
                output, status = self.certificados, self.exportar
            elif argv[-1] == NOMBRE:
                output = exportado(OTRO_DER, OTRO_SELECTOR)
            else:
                self.assert_raices(argv)
        elif argv[0] == "openssl" or argv[:2] in (
                ["security", "list-keychains"], ["security", "show-keychain-info"],
                ["security", "verify-cert"]):
            pass
        else:
            raise AssertionError("unexpected command: " + repr(argv))
        return subprocess.CompletedProcess(argv, status, output, "")

    @staticmethod
    def assert_raices(argv):
        assert argv[-1] == "/System/Library/Keychains/SystemRootCertificates.keychain"


class Sonda(unittest.TestCase):
    def test_exported_certificates_are_indexed_by_sha256(self):
        with tempfile.TemporaryDirectory() as carpeta:
            archivos, metadatos = s.certificados_publicos(
                exportado(), pathlib.Path(carpeta), "cert", Herramientas())
            self.assertEqual(set(archivos), {s.huella(DER)})
            self.assertEqual(ssl.PEM_cert_to_DER_cert(archivos[s.huella(DER)].read_text()), DER)
        self.assertEqual(metadatos[0]["sha1"], SELECTOR)
        self.assertEqual(s.certificados_por_selector(metadatos), {SELECTOR: s.huella(DER)})

    def test_missing_or_mismatched_export_fingerprints_fail_closed(self):
        for texto in [ssl.DER_cert_to_PEM_cert(DER),
                      exportado().replace(s.huella(DER), "0" * 64)]:
            with self.subTest(texto=texto), tempfile.TemporaryDirectory() as carpeta:
                with self.assertRaises(ValueError):
                    s.certificados_publicos(texto, pathlib.Path(carpeta), "cert", Herramientas())

    def test_same_legacy_selector_for_different_certificates_is_rejected(self):
        # Model a SHA-1 collision without using a weak hash or real credentials.
        herramientas = Herramientas()
        herramientas.certificados += exportado(OTRO_DER, SELECTOR)
        with self.assertRaisesRegex(ValueError, "ambiguo"):
            s.sondear(LLAVERO, NOMBRE, herramientas)

    def test_repeated_export_of_same_certificate_is_not_ambiguous(self):
        herramientas = Herramientas()
        herramientas.certificados += exportado()
        self.assertTrue(s.sondear(LLAVERO, NOMBRE, herramientas)["production_ok"])

    def test_valid_leaf_preserves_native_codesign_selectors_and_cold_probe(self):
        herramientas = Herramientas()
        resultado = s.sondear(LLAVERO, NOMBRE, herramientas)
        self.assertTrue(resultado["production_ok"])
        self.assertEqual(resultado["expected_signatures"], 5)
        self.assertTrue(all(f["ok"] for f in resultado["signatures"]))
        firmas = [c for c in herramientas.comandos if c[:2] == ["codesign", "-f"]]
        self.assertEqual(herramientas.comandos[0], firmas[0])
        self.assertEqual([c[3] for c in firmas], [NOMBRE, NOMBRE, NOMBRE, SELECTOR, SELECTOR])
        self.assertTrue(all(f["chain"][0]["sha256"] == s.huella(DER)
                            for f in resultado["signatures"]))
        self.assertEqual(len(resultado["policies"]), 4)
        self.assertTrue(all(p["sha256"] == s.huella(DER) for p in resultado["policies"]))
        self.assertEqual({p["policy"] for p in resultado["policies"]}, {"basic", "codeSign"})
        politicas = [i for i, c in enumerate(herramientas.comandos) if c[:2] == ["security", "verify-cert"]]
        firmas = [i for i, c in enumerate(herramientas.comandos) if c[:2] == ["codesign", "-f"]]
        self.assertLess(max(firmas), min(politicas))
        self.assertTrue(all("-L" in herramientas.comandos[i] for i in politicas))
        extracciones = [c for c in herramientas.comandos if c[:2] == ["codesign", "-d"]]
        self.assertTrue(all(len(c) == 4 and c[2].startswith("--extract-certificates=") for c in extracciones))

    def test_another_leaf_is_rejected_even_when_codesign_verifies(self):
        herramientas = Herramientas()
        herramientas.hojas = {i: OTRO_DER for i in range(5)}
        resultado = s.sondear(LLAVERO, NOMBRE, herramientas)
        self.assertFalse(resultado["production_ok"])
        self.assertTrue(all(not f["ok"] for f in resultado["signatures"]))

    def test_no_leaf_is_not_replaced_by_expected_intermediate(self):
        herramientas = Herramientas()
        herramientas.sin_hoja = True
        resultado = s.sondear(LLAVERO, NOMBRE, herramientas)
        self.assertFalse(resultado["production_ok"])
        self.assertTrue(all(not f["ok"] for f in resultado["signatures"]))

    def test_codesign_and_export_failures_do_not_pass(self):
        for opcion in ["verificar", "extraer", "firmar", "exportar", "identidades"]:
            with self.subTest(opcion=opcion):
                herramientas = Herramientas()
                setattr(herramientas, opcion, 1)
                resultado = s.sondear(LLAVERO, NOMBRE, herramientas)
                self.assertFalse(resultado["production_ok"])

    def test_cold_failure_is_not_hidden_by_later_valid_signatures(self):
        herramientas = Herramientas()
        herramientas.firmas_fallidas = {1}
        resultado = s.sondear(LLAVERO, NOMBRE, herramientas)
        self.assertFalse(resultado["production_ok"])
        self.assertEqual([f["ok"] for f in resultado["signatures"]],
                         [False, True, True, True, True])

    def test_name_failure_is_not_hidden_by_valid_hash_selectors(self):
        herramientas = Herramientas()
        herramientas.firmas_fallidas = {1, 2, 3}
        resultado = s.sondear(LLAVERO, NOMBRE, herramientas)
        self.assertFalse(resultado["production_ok"])
        self.assertEqual([f["ok"] for f in resultado["signatures"]],
                         [False, False, False, True, True])

    def test_global_name_search_uses_exported_certificate_but_keychain_stays_local(self):
        herramientas = Herramientas()
        herramientas.globales = True
        herramientas.hojas = {i: OTRO_DER for i in range(5)}
        resultado = s.sondear(LLAVERO, NOMBRE, herramientas)
        self.assertTrue(resultado["production_ok"])
        self.assertEqual(resultado["identities_outside_ephemeral"], [OTRO_SELECTOR])
        self.assertEqual([f["ok"] for f in resultado["signatures"]],
                         [True, True, False, False, False])
        self.assertIn(["security", "find-certificate", "-a", "-Z", "-p", "-c", NOMBRE],
                      herramientas.comandos)

    def test_global_selector_collision_is_rejected_across_keychains(self):
        class Colision(Herramientas):
            def __call__(self, argv):
                r = super().__call__(argv)
                if argv[:2] == ["security", "find-certificate"] and argv[-1] == NOMBRE:
                    r.stdout += exportado(OTRO_DER, SELECTOR)
                return r
        herramientas = Colision()
        herramientas.globales = True
        with self.assertRaisesRegex(ValueError, "ambiguo"):
            s.sondear(LLAVERO, NOMBRE, herramientas)

    def test_unexported_global_identity_is_not_trusted_by_selector(self):
        class SinCertificado(Herramientas):
            def __call__(self, argv):
                r = super().__call__(argv)
                if argv[:2] == ["security", "find-certificate"] and argv[-1] == NOMBRE:
                    r.stdout = ""
                return r
        herramientas = SinCertificado()
        herramientas.globales = True
        self.assertFalse(s.sondear(LLAVERO, NOMBRE, herramientas)["production_ok"])


if __name__ == "__main__":
    unittest.main()
