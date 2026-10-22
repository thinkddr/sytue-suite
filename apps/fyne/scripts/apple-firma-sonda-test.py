#!/usr/bin/env python3
# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
import hashlib
import importlib.util
import pathlib
import unittest
from unittest import mock


AQUI = pathlib.Path(__file__).resolve().parent
spec = importlib.util.spec_from_file_location("sonda", AQUI / "apple_firma_sonda.py")
sonda = importlib.util.module_from_spec(spec)
spec.loader.exec_module(sonda)


class Huella(unittest.TestCase):
    def test_usa_sha1_como_identificador_no_criptografico_de_codesign(self):
        certificado, der = "certificado", b"certificado DER"
        esperado = hashlib.sha1(der, usedforsecurity=False).hexdigest().upper()
        with mock.patch.object(sonda.ssl, "PEM_cert_to_DER_cert", return_value=der) as convertir, \
                mock.patch.object(sonda.hashlib, "sha1", wraps=hashlib.sha1) as sha1:
            self.assertEqual(sonda.huella(certificado), esperado)
        convertir.assert_called_once_with(certificado)
        sha1.assert_called_once_with(der, usedforsecurity=False)


if __name__ == "__main__":
    unittest.main()
