# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
"""Variantes que el corpus público no garantiza, hechas con herramientas ajenas.

Toma unos pocos PDF base y escribe en <destino>:
  * con qpdf: linealizado, con flujos de objetos y de referencias, cada cifrado
    (RC4-40, RC4-128, AES-128, AES-256 R5 y R6) con y sin contraseña de usuario;
  * a mano: tablas de referencias dañadas (desplazamiento falso, sin tabla,
    bytes basura delante, sin %%EOF) y una actualización incremental;
  * con pyHanko: firmas PAdES B-B, B-T, B-LT y B-LTA, dos firmas, certificado
    caducado y certificado revocado (con la CRL en el /DSS).
Las contraseñas de usuario van a <herramientas>/claves.json.

Uso: generar.py <destino> <herramientas> <pdf-base>…
"""

import datetime
import io
import json
import os
import re
import subprocess
import sys

from cryptography import x509
from cryptography.hazmat.primitives import hashes, serialization
from cryptography.hazmat.primitives.asymmetric import rsa
from cryptography.x509.oid import ExtendedKeyUsageOID, NameOID

dest, herr, bases = sys.argv[1], sys.argv[2], sys.argv[3:]
os.makedirs(dest, exist_ok=True)
QPDF = os.path.join(herr, "qpdf/usr/bin/qpdf")
ENV = dict(os.environ) | {"LD_LIBRARY_PATH": os.path.join(herr, "qpdf/usr/lib/aarch64-linux-gnu")}
claves_json = os.path.join(herr, "claves.json")
claves = json.load(open(claves_json)) if os.path.exists(claves_json) else {}


def qpdf(*args):
    p = subprocess.run([QPDF, *args], env=ENV, capture_output=True)
    if p.returncode not in (0, 3):
        print("qpdf falló:", args, p.stderr.decode()[:200])


def nombre(base, sufijo):
    return os.path.join(dest, f"gen-{os.path.splitext(os.path.basename(base))[0]}-{sufijo}.pdf")


for base in bases:
    qpdf("--linearize", base, nombre(base, "linealizado"))
    qpdf("--object-streams=generate", base, nombre(base, "flujos-objetos"))
    qpdf("--force-version=2.0", "--object-streams=generate", base, nombre(base, "pdf20"))
    for etiqueta, extra in [("rc4-40", ["40"]), ("rc4-128", ["128", "--use-aes=n"]), ("aes-128", ["128", "--use-aes=y"]),
                            ("aes-256-r5", ["256", "--force-R5"]), ("aes-256-r6", ["256"])]:
        for usuario in ("", "usuario"):
            sal = nombre(base, f"cifrado-{etiqueta}-{'con' if usuario else 'sin'}-clave")
            qpdf("--allow-weak-crypto", "--encrypt", usuario, "dueno", *extra, "--", base, sal)
            if usuario:
                claves[os.path.basename(sal)] = usuario

    datos = open(base, "rb").read()
    # Referencias dañadas: lo que deja una descarga cortada o un programa torpe.
    m = list(re.finditer(rb"startxref\s+(\d+)", datos))
    if m:
        ult = m[-1]
        falso = datos[: ult.start(1)] + str(int(ult.group(1)) + 37).encode() + datos[ult.end(1):]
        open(nombre(base, "danado-startxref"), "wb").write(falso)
    open(nombre(base, "danado-basura-delante"), "wb").write(b"\x00" * 1000 + b"basura\n" + datos)
    open(nombre(base, "danado-sin-eof"), "wb").write(datos.rstrip()[: -len(b"%%EOF")] if datos.rstrip().endswith(b"%%EOF") else datos[:-10])
    x = datos.rfind(b"\nxref")
    if x > 0:
        open(nombre(base, "danado-sin-tabla"), "wb").write(datos[:x] + b"\n%%EOF\n")
    open(nombre(base, "truncado"), "wb").write(datos[: int(len(datos) * 0.8)])

    # Actualización incremental (pypdf): una nota añadida al final, sin reescribir.
    try:
        from pypdf import PdfWriter
        from pypdf.annotations import FreeText

        w = PdfWriter(base, incremental=True)
        w.add_annotation(0, FreeText(text="Añadido después", rect=(50, 50, 250, 80)))
        w.write(nombre(base, "incremental"))
    except Exception as ex:  # noqa: BLE001
        print("incremental falló:", base, ex)

# --- Firmas PAdES ---------------------------------------------------------------

from pyhanko.pdf_utils.incremental_writer import IncrementalPdfFileWriter  # noqa: E402
from pyhanko.sign import signers  # noqa: E402
from pyhanko.sign.fields import SigSeedSubFilter  # noqa: E402
from pyhanko.sign.timestamps.dummy_client import DummyTimeStamper  # noqa: E402
from pyhanko.sign.validation.dss import DocumentSecurityStore  # noqa: E402
from pyhanko_certvalidator import ValidationContext  # noqa: E402
from pyhanko_certvalidator.registry import SimpleCertificateStore  # noqa: E402
from asn1crypto import x509 as a509, crl as acrl, keys as akeys  # noqa: E402

AHORA = datetime.datetime.now(datetime.timezone.utc)
D = datetime.timedelta(days=1)


def clave():
    return rsa.generate_private_key(public_exponent=65537, key_size=2048)


def cert(cn, emisor, k_pub, k_firma, ca=False, desde=AHORA - D, hasta=AHORA + 365 * D, eku=None, crl_url=None):
    sujeto = x509.Name([x509.NameAttribute(NameOID.COUNTRY_NAME, "ES"), x509.NameAttribute(NameOID.ORGANIZATION_NAME, "Sytue Pruebas PAdES"),
                        x509.NameAttribute(NameOID.COMMON_NAME, cn)])
    b = (x509.CertificateBuilder().subject_name(sujeto).issuer_name(emisor or sujeto).public_key(k_pub)
         .serial_number(x509.random_serial_number()).not_valid_before(desde).not_valid_after(hasta)
         .add_extension(x509.BasicConstraints(ca=ca, path_length=None), critical=True)
         .add_extension(x509.KeyUsage(not ca, not ca, False, False, False, ca, ca, False, False), critical=True))
    if eku:
        b = b.add_extension(x509.ExtendedKeyUsage([eku]), critical=True)
    if crl_url:
        b = b.add_extension(x509.CRLDistributionPoints([x509.DistributionPoint([x509.UniformResourceIdentifier(crl_url)], None, None, None)]), critical=False)
    return b.sign(k_firma, hashes.SHA256())


def a_asn1(c):
    return a509.Certificate.load(c.public_bytes(serialization.Encoding.DER))


def clave_asn1(k):
    return akeys.PrivateKeyInfo.load(k.private_bytes(serialization.Encoding.DER, serialization.PrivateFormat.PKCS8, serialization.NoEncryption()))


k_raiz = clave()
raiz = cert("Raiz PAdES de pruebas", None, k_raiz.public_key(), k_raiz, ca=True, desde=AHORA - 3650 * D, hasta=AHORA + 3650 * D)
URL = "http://crl.sytue.invalid/raiz.crl"
k_f = clave()
firmante = cert("Firmante PAdES", raiz.subject, k_f.public_key(), k_raiz, crl_url=URL)
k_cad = clave()
caducado = cert("Firmante caducado", raiz.subject, k_cad.public_key(), k_raiz, desde=AHORA - 800 * D, hasta=AHORA - 30 * D, crl_url=URL)
k_rev = clave()
revocado = cert("Firmante revocado", raiz.subject, k_rev.public_key(), k_raiz, crl_url=URL)
k_tsa = clave()
tsa = cert("TSA de pruebas", raiz.subject, k_tsa.public_key(), k_raiz, eku=ExtendedKeyUsageOID.TIME_STAMPING, crl_url=URL)
lista = (x509.CertificateRevocationListBuilder().issuer_name(raiz.subject).last_update(AHORA - D).next_update(AHORA + 30 * D)
         .add_revoked_certificate(x509.RevokedCertificateBuilder().serial_number(revocado.serial_number).revocation_date(AHORA - 2 * D).build())
         .sign(k_raiz, hashes.SHA256()))
crl = acrl.CertificateList.load(lista.public_bytes(serialization.Encoding.DER))
open(os.path.join(dest, "gen-pades-raiz.pem"), "wb").write(raiz.public_bytes(serialization.Encoding.PEM))

sellador = DummyTimeStamper(tsa_cert=a_asn1(tsa), tsa_key=clave_asn1(k_tsa), certs_to_embed=[a_asn1(raiz)])


def firmador(c, k):
    return signers.SimpleSigner(signing_cert=a_asn1(c), signing_key=clave_asn1(k), cert_registry=SimpleCertificateStore.from_certs([a_asn1(raiz)]))


def firmar(entrada, salida, c, k, campo="Firma1", ts=None, lt=False, lta=False):
    vc = ValidationContext(trust_roots=[a_asn1(raiz)], crls=[crl], allow_fetching=False) if lt else None
    meta = signers.PdfSignatureMetadata(field_name=campo, subfilter=SigSeedSubFilter.PADES, embed_validation_info=lt,
                                        use_pades_lta=lta, validation_context=vc)
    with open(entrada, "rb") as f:
        w = IncrementalPdfFileWriter(f, strict=False)
        out = signers.PdfSigner(meta, signer=firmador(c, k), timestamper=ts).sign_pdf(w)
    open(salida, "wb").write(out.getvalue())


base = bases[0]
pruebas = [
    ("pades-b-b", dict(c=firmante, k=k_f)),
    ("pades-b-t", dict(c=firmante, k=k_f, ts=sellador)),
    ("pades-b-lt", dict(c=firmante, k=k_f, ts=sellador, lt=True)),
    ("pades-b-lta", dict(c=firmante, k=k_f, ts=sellador, lt=True, lta=True)),
    ("pades-caducado", dict(c=caducado, k=k_cad)),
    ("pades-revocado", dict(c=revocado, k=k_rev)),
]
for etiqueta, kw in pruebas:
    try:
        firmar(base, nombre(base, etiqueta), **kw)
    except Exception as ex:  # noqa: BLE001
        print("firma falló:", etiqueta, type(ex).__name__, ex)
# El revocado, con la CRL que lo dice dentro del /DSS (lo que un validador ve sin red).
try:
    buf = io.BytesIO(open(nombre(base, "pades-revocado"), "rb").read())
    DocumentSecurityStore.add_dss(buf, None, certs=[a_asn1(raiz), a_asn1(revocado)], crls=[crl], strict=False)
    open(nombre(base, "pades-revocado-con-crl"), "wb").write(buf.getvalue())
except Exception as ex:  # noqa: BLE001
    print("dss falló:", type(ex).__name__, ex)
# Dos firmas, la segunda sobre la primera.
try:
    firmar(nombre(base, "pades-b-t"), nombre(base, "pades-dos-firmas"), firmante, k_f, campo="Firma2", ts=sellador)
except Exception as ex:  # noqa: BLE001
    print("segunda firma falló:", ex)

json.dump(claves, open(claves_json, "w"), ensure_ascii=False)
print(len(os.listdir(dest)), "ficheros en", dest)
