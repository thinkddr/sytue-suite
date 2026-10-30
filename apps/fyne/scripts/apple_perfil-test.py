#!/usr/bin/env python3
# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
"""CMS reales con CA efímera sólo en CI: cadena, firma, rol y replay."""
import datetime
import importlib.util
import os
import pathlib
import plistlib
import subprocess
import sys
import tempfile
import unittest
from unittest import mock

AQUI = pathlib.Path(__file__).resolve().parent
spec = importlib.util.spec_from_file_location("perfil_cms", AQUI / "apple_perfil.py")
p = importlib.util.module_from_spec(spec)
spec.loader.exec_module(p)


class Entrada(unittest.TestCase):
    def test_fifo_sin_escritor_falla_sin_bloquear_ni_publicar_contenido(self):
        with tempfile.TemporaryDirectory(prefix="sytue-cms-fifo-test-") as temporal:
            fifo = pathlib.Path(temporal, "sin-escritor.fifo")
            os.mkfifo(fifo, 0o600)
            resultado = subprocess.run([sys.executable, str(AQUI / "apple_perfil.py"),
                                        "decodificar", str(fifo), "ios"],
                                       capture_output=True, timeout=2, check=False)
            self.assertEqual(resultado.returncode, 1)
            self.assertEqual(resultado.stdout, b"")
            self.assertEqual(resultado.stderr, b"::error::perfil CMS o derechos no v\xc3\xa1lidos\n")


class CMS(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.temporal = tempfile.TemporaryDirectory(prefix="sytue-cms-crypto-test-")
        cls.addClassCleanup(cls.temporal.cleanup)
        cls.base = pathlib.Path(cls.temporal.name)
        cls.raiz = cls.base / "root.pem"
        cls.clave = cls.base / "root.key"
        cls.run_crypto("req", "-x509", "-newkey", "ec", "-pkeyopt", "ec_paramgen_curve:P-256", "-nodes",
                       "-subj", "/CN=Sytue ephemeral test CA", "-days", "2", "-keyout", cls.clave, "-out", cls.raiz,
                       "-addext", "basicConstraints=critical,CA:TRUE", "-addext", "keyUsage=critical,keyCertSign,cRLSign")
        cls.der = cls.run_crypto("x509", "-in", cls.raiz, "-outform", "DER")
        cls.signers = {}
        for nombre, cn in (("ios", "Apple iPhone OS Provisioning Profile Signing"),
                           ("mac", "Mac OS X Provisioning Profile Signing"), ("ajeno", "Apple Distribution")):
            llave, peticion, cert, ext = (cls.base / (nombre + sufijo) for sufijo in (".key", ".csr", ".pem", ".ext"))
            ext.write_text("basicConstraints=critical,CA:FALSE\nkeyUsage=critical,digitalSignature\n")
            cls.run_crypto("req", "-new", "-newkey", "ec", "-pkeyopt", "ec_paramgen_curve:P-256", "-nodes",
                           "-keyout", llave, "-out", peticion, "-subj", "/CN=" + cn + "/O=Apple Inc./C=US")
            cls.run_crypto("x509", "-req", "-in", peticion, "-CA", cls.raiz, "-CAkey", cls.clave,
                           "-set_serial", str(len(cls.signers) + 10), "-days", "1", "-extfile", ext, "-out", cert)
            cls.signers[nombre] = cert, llave

    @classmethod
    def run_crypto(cls, *argumentos, contenido=None):
        resultado = subprocess.run(["openssl", *map(str, argumentos)], input=contenido,
                                   capture_output=True, timeout=10, check=False)
        if resultado.returncode:
            raise RuntimeError("fixture OpenSSL no creado")
        return resultado.stdout

    def payload(self, plataforma="ios"):
        return {"Platform": ["iOS" if plataforma == "ios" else "OSX"],
                "ProvisionsAllDevices": plataforma == "developerid", "TeamIdentifier": ["VHC5QW7BZ3"],
                "ExpirationDate": datetime.datetime(2050, 1, 1), "Entitlements": {"test": True}}

    def firmar(self, plataforma="ios", nombre="ios", detached=False, segundo=False):
        cert, clave = self.signers[nombre]
        argumentos = ["cms", "-sign", "-binary", "-outform", "DER", "-signer", cert,
                      "-inkey", clave, "-certfile", self.raiz, "-md", "sha256"]
        if not detached:
            argumentos.append("-nodetach")
        if segundo:
            cert2, clave2 = self.signers["mac"]
            argumentos.extend(["-signer", cert2, "-inkey", clave2])
        return self.run_crypto(*argumentos, contenido=plistlib.dumps(self.payload(plataforma)))

    def test_cms_reales_validos_con_firmante_de_su_rol_en_las_tres_plataformas(self):
        for plataforma, firmante in (("ios", "ios"), ("macappstore", "mac"), ("developerid", "mac")):
            with self.subTest(plataforma=plataforma):
                self.assertEqual(p._decodificar_con_raiz(self.firmar(plataforma, firmante), plataforma, self.der),
                                 self.payload(plataforma))

    def test_firma_corrupta_y_raiz_ajena_son_rechazadas(self):
        cms = self.firmar()
        with self.assertRaises(p.PerfilInvalido):
            p._decodificar_con_raiz(cms[:-1] + bytes([cms[-1] ^ 1]), "ios", self.der)
        with self.assertRaises(p.PerfilInvalido):
            # La CA sintética nunca está autorizada por la raíz Apple fijada.
            p.decodificar(cms, "ios")

    def test_certificado_descendiente_de_la_raiz_pero_con_otro_rol_no_firma_perfiles(self):
        with self.assertRaisesRegex(p.PerfilInvalido, "official_profile_signer"):
            p._decodificar_con_raiz(self.firmar(nombre="ajeno"), "ios", self.der)

    def test_firmante_o_payload_de_otra_plataforma_no_se_reutilizan(self):
        for cms, plataforma in ((self.firmar(), "macappstore"),
                                (self.firmar("macappstore", "mac"), "ios"),
                                (self.firmar("developerid", "mac"), "macappstore"),
                                (self.firmar("macappstore", "mac"), "developerid"),
                                (self.firmar("macappstore", "ios"), "ios")):
            with self.subTest(plataforma=plataforma), self.assertRaises(p.PerfilInvalido):
                p._decodificar_con_raiz(cms, plataforma, self.der)

    def test_dos_signer_infos_detached_y_datos_posteriores_no_se_aceptan(self):
        for cms in (self.firmar(segundo=True), self.firmar(detached=True), self.firmar() + b"junk"):
            with self.subTest(bytes=len(cms)), self.assertRaises(p.PerfilInvalido):
                p._decodificar_con_raiz(cms, "ios", self.der)

    def test_entrada_plist_sin_cms_y_fuera_de_limite_no_tienen_fallback(self):
        for contenido in (plistlib.dumps(self.payload()), b"", b"x" * (p.LIMITE + 1)):
            with self.subTest(bytes=len(contenido)), self.assertRaises(p.PerfilInvalido):
                p.decodificar(contenido, "ios")

    def test_la_raiz_de_produccion_no_admite_override_sin_su_huella(self):
        ruta = self.base / "foreign-root.der"
        ruta.write_bytes(self.der)
        with mock.patch.object(p, "RAIZ_APPLE", ruta), self.assertRaisesRegex(p.PerfilInvalido, "pinned_apple_root"):
            p.decodificar(self.firmar(), "ios")


if __name__ == "__main__":
    unittest.main()
