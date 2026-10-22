# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
"""Segunda mitad de `generar.sh`: firma y manipula PDF con pyHanko y con OpenSSL.

No se ejecuta suelto: `generar.sh` prepara la PKI de prueba, el PDF base de
LibreOffice y el venv con pyHanko, y llama aquí. Lee el porqué de la batería en la
cabecera de `generar.sh`.

Cada fixture se construye con una función de este fichero y se escribe con el
nombre que espera `manifiesto.json`. Los que NO son «un documento tal como lo dejó
la herramienta» (manipulados a mano, cambios posteriores) dicen en su comentario
qué se les hizo y por qué es un caso que un verificador tiene que distinguir.
"""

import argparse
import asyncio
import datetime
import hashlib
import io
import os
import re
import subprocess
import tempfile

from asn1crypto import tsp
from pyhanko.keys import load_cert_from_pemder, load_certs_from_pemder, load_private_key_from_pemder
from pyhanko.pdf_utils import generic
from pyhanko.pdf_utils.incremental_writer import IncrementalPdfFileWriter
from pyhanko.pdf_utils.writer import PageObject, PdfFileWriter
from pyhanko.sign import fields, signers, timestamps
from pyhanko.sign.fields import MDPPerm, SigSeedSubFilter
from pyhanko.sign.signers.pdf_signer import PdfTBSDocument

UTC = datetime.timezone.utc


class Generador:
    def __init__(self, pki):
        self.pki = pki

    def p(self, nombre):
        return os.path.join(self.pki, nombre)

    # --- Sellos de tiempo ------------------------------------------------------

    def sello_openssl(self, tsa="tsa"):
        """Una TSA RFC 3161 de verdad: `openssl ts -reply` con la hora del reloj."""
        return _TsaOpenssl(self.p(f"{tsa}.tsa.cnf"))

    def sello_de_otra_firma(self):
        """Un sello AUTÉNTICO de la TSA, pero del resumen de otra cosa: el que cuela un
        atacante que quiere la fecha de un sello sin sellar esta firma."""
        return _TsaEquivocada(self.p("tsa.tsa.cnf"))

    def sello_con_fecha(self, fecha, tsa="tsa", cadena="cadena-sello"):
        """El MISMO par clave/certificado de la TSA, pero firmando una fecha del pasado.

        `openssl ts` no deja fijar la hora, y para el caso «el certificado caducó
        DESPUÉS de sellar» hace falta un sello de cuando aún estaba vigente.
        """
        return timestamps.DummyTimeStamper(
            tsa_cert=load_cert_from_pemder(self.p(f"{tsa}.crt")),
            tsa_key=load_private_key_from_pemder(self.p(f"{tsa}.key"), None),
            certs_to_embed=list(load_certs_from_pemder([self.p(f"{cadena}.pem")])),
            fixed_dt=fecha,
        )

    # --- Firmar ----------------------------------------------------------------

    def firmante(self, quien, cadena, pss=False):
        return signers.SimpleSigner.load(
            self.p(f"{quien}.key"), self.p(f"{quien}.crt"),
            ca_chain_files=(self.p(f"{cadena}.pem"),), prefer_pss=pss,
        )

    def firmar(self, pdf, quien, campo, *, cadena="cadena-a", sello=None, md="sha256",
               subfilter=SigSeedSubFilter.PADES, certificar=None, nombre=None,
               contra_la_certificacion=False, pss=False):
        """Firma `pdf` por actualización incremental, como lo hace un lector de PDF.

        `contra_la_certificacion` esquiva la negativa de pyHanko a firmar un documento
        certificado con «ningún cambio» (/P 1): es justo el fichero que un tercero
        malicioso, o una herramienta que no respeta DocMDP, sí puede producir.
        """
        meta = signers.PdfSignatureMetadata(
            field_name=campo, md_algorithm=md, subfilter=subfilter,
            name=nombre or quien.capitalize(), reason="Conforme", location="Valencia",
            certify=certificar is not None,
            docmdp_permissions=certificar or MDPPerm.FILL_FORMS,
        )
        firmador = signers.PdfSigner(
            meta, signer=self.firmante(quien, cadena, pss), timestamper=sello,
            new_field_spec=fields.SigFieldSpec(sig_field_name=campo),
        )
        if contra_la_certificacion:
            firmador._enforce_certification_constraints = lambda *_: None
        salida = firmador.sign_pdf(IncrementalPdfFileWriter(io.BytesIO(pdf)), in_place=False)
        return salida.getvalue()

    def firmar_a_mano_con_openssl(self, pdf, quien, campo, cadena="cadena-a"):
        """Firma con `openssl cms -sign`: ninguna biblioteca de PDF produce el CMS.

        pyHanko solo deja el hueco /Contents y el /ByteRange; el CMS lo hace OpenSSL
        sobre los dos tramos del fichero, con atributos firmados CAdES.
        """
        meta = signers.PdfSignatureMetadata(
            field_name=campo, md_algorithm="sha256", subfilter=SigSeedSubFilter.PADES,
            name=quien.capitalize(), reason="Conforme", location="Valencia",
        )
        firmador = signers.PdfSigner(
            meta, signer=self.firmante(quien, cadena),
            new_field_spec=fields.SigFieldSpec(sig_field_name=campo),
        )
        prep, _, salida = asyncio.run(firmador.async_digest_doc_for_signing(
            IncrementalPdfFileWriter(io.BytesIO(pdf)), bytes_reserved=8192))
        fichero = salida.getvalue()
        firmado = fichero[:prep.reserved_region_start] + fichero[prep.reserved_region_end:]
        with tempfile.TemporaryDirectory() as d:
            ent, sal = os.path.join(d, "firmado.bin"), os.path.join(d, "firma.der")
            with open(ent, "wb") as f:
                f.write(firmado)
            subprocess.run([
                "openssl", "cms", "-sign", "-binary", "-cades", "-md", "sha256",
                "-in", ent, "-signer", self.p(f"{quien}.crt"), "-inkey", self.p(f"{quien}.key"),
                "-certfile", self.p(f"{cadena}.pem"), "-outform", "DER", "-out", sal,
            ], check=True)
            with open(sal, "rb") as f:
                cms_der = f.read()
        PdfTBSDocument.finish_signing(salida, prep, cms_der)
        return salida.getvalue()

    def sello_de_documento(self, pdf, sello, md="sha256"):
        """Sello de tiempo de documento (/SubFilter /ETSI.RFC3161), el de PAdES B-LTA."""
        w = IncrementalPdfFileWriter(io.BytesIO(pdf))
        return signers.PdfTimeStamper(sello).timestamp_pdf(w, md, in_place=False).getvalue()

    # --- Cambios posteriores a la firma ---------------------------------------

    def cambiar_pagina(self, pdf, pagina):
        """Actualización incremental SIN firmar que pinta un rectángulo sobre la página."""
        w = IncrementalPdfFileWriter(io.BytesIO(pdf))
        flujo = w.add_object(generic.StreamObject(
            stream_data=b"q 1 0 0 rg 40 600 400 120 re f Q"))
        w.add_stream_to_page(pagina, flujo)
        return _escribir(w)

    def cambiar_titulo(self, pdf):
        """Actualización incremental sin firmar que solo cambia el /Title del /Info."""
        w = IncrementalPdfFileWriter(io.BytesIO(pdf))
        info_ref = w.trailer.raw_get("/Info")
        info = info_ref.get_object()
        info["/Title"] = generic.pdf_string("Contrato de prueba (editado)")
        w.mark_update(info_ref)
        return _escribir(w)

    def anotar_pagina(self, pdf, pagina):
        """Actualización incremental sin firmar que añade un comentario (anotación)."""
        w = IncrementalPdfFileWriter(io.BytesIO(pdf))
        anot = w.add_object(generic.DictionaryObject({
            generic.pdf_name("/Type"): generic.pdf_name("/Annot"),
            generic.pdf_name("/Subtype"): generic.pdf_name("/Text"),
            generic.pdf_name("/Rect"): generic.ArrayObject(
                [generic.NumberObject(n) for n in (50, 700, 70, 720)]),
            generic.pdf_name("/Contents"): generic.pdf_string("Revisar la cláusula segunda"),
            generic.pdf_name("/F"): generic.NumberObject(4),
        }))
        pagina_ref, _ = w.find_page_for_modification(pagina)
        w.register_annotation(pagina_ref, anot)
        return _escribir(w)

    # --- Un PDF base con otra forma ------------------------------------------

    def base_con_flujos(self):
        """PDF base de pyHanko con flujo de referencias cruzadas y flujo de objetos.

        La forma que escriben Word, Acrobat y casi todo lo moderno (PDF 1.5+), y que el
        PDF de LibreOffice 7.4 no tiene: comprueba que verificar no depende de la
        tabla xref clásica.
        """
        w = PdfFileWriter(stream_xrefs=True)
        objs = w.prepare_object_stream()
        fuente = w.add_object(generic.DictionaryObject({
            generic.pdf_name("/Type"): generic.pdf_name("/Font"),
            generic.pdf_name("/Subtype"): generic.pdf_name("/Type1"),
            generic.pdf_name("/BaseFont"): generic.pdf_name("/Helvetica"),
        }), obj_stream=objs)
        for i, texto in enumerate(("Primera pagina: formulario de alta.",
                                   "Segunda pagina: clausulas.")):
            contenido = w.add_object(generic.StreamObject(stream_data=(
                f"BT /F1 14 Tf 60 720 Td ({texto}) Tj ET".encode("latin-1"))))
            pagina = PageObject(
                contents=contenido, media_box=(0, 0, 595, 842),
                resources=generic.DictionaryObject({
                    generic.pdf_name("/Font"): generic.DictionaryObject({
                        generic.pdf_name("/F1"): fuente}),
                }),
            )
            w.insert_page(pagina)
        w.set_info(generic.DictionaryObject({
            generic.pdf_name("/Title"): generic.pdf_string("Formulario de prueba"),
            generic.pdf_name("/Producer"): generic.pdf_string("pyHanko"),
        }))
        salida = io.BytesIO()
        w.write(salida)
        return salida.getvalue()


def _escribir(w):
    salida = io.BytesIO()
    w.write(salida)
    return salida.getvalue()


class _TsaOpenssl(timestamps.TimeStamper):
    """Un cliente de sello de tiempo cuya «autoridad» es `openssl ts -reply`."""

    def __init__(self, cnf):
        super().__init__(include_nonce=True)
        self.cnf = cnf

    async def async_request_tsa_response(self, req):
        with tempfile.TemporaryDirectory() as d:
            q, r = os.path.join(d, "q.tsq"), os.path.join(d, "r.tsr")
            with open(q, "wb") as f:
                f.write(req.dump())
            subprocess.run(["openssl", "ts", "-reply", "-config", self.cnf,
                            "-queryfile", q, "-out", r], check=True)
            with open(r, "rb") as f:
                return tsp.TimeStampResp.load(f.read())


class _TsaEquivocada(_TsaOpenssl):
    """La misma TSA, que sella el resumen de «otra firma» en vez del que se le pide."""

    async def async_timestamp(self, message_digest, md_algorithm):
        return await super().async_timestamp(hashlib.sha256(b"otra firma").digest(), "sha256")


# --- Manipulaciones de bytes (lo que haría un atacante o un editor torpe) -------


def tocar_contenido_firmado(pdf):
    """Cambia UN byte dentro del tramo firmado (el /Producer del /Info), sin romper el PDF."""
    marca = b"LibreOffice"
    i = pdf.find(marca)
    assert 0 < i < pdf.find(b"/ByteRange"), "no hay /Producer legible antes de la firma"
    return pdf[:i + len(marca) - 1] + b"f" + pdf[i + len(marca):]


def desplazar_byterange(pdf):
    """Adelanta en 1 el inicio del hueco del /ByteRange, conservando el ancho del campo."""
    m = re.search(rb"/ByteRange\s*\[\s*0\s+(\d+)\s+(\d+)\s+(\d+)\s*\]", pdf)
    assert m, "sin /ByteRange"
    h0 = int(m.group(1)) - 1
    original = m.group(0)
    nuevo = original.replace(m.group(1), str(h0).encode().rjust(len(m.group(1))), 1)
    assert len(nuevo) == len(original) and nuevo != original
    return pdf.replace(original, nuevo, 1)


def cambiar_subfiltro(pdf):
    """Cambia el /SubFilter a adbe.pkcs7.sha1 (SHA-1, que nadie emite ya) rellenando con
    espacios, que tras un nombre son sintaxis PDF válida. El verificador debe decir que
    no sabe comprobarlo, no intentar adivinar."""
    viejo, nuevo = b"/adbe.pkcs7.detached", b"/adbe.pkcs7.sha1"
    assert pdf.count(viejo) == 1
    return pdf.replace(viejo, nuevo.ljust(len(viejo)), 1)


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--pki", required=True)
    ap.add_argument("--base", required=True)
    ap.add_argument("--salida", required=True)
    ap.add_argument("--trabajo", required=True)
    a = ap.parse_args()

    g = Generador(a.pki)
    with open(a.base, "rb") as f:
        base = f.read()
    flujos = g.base_con_flujos()
    epoca = datetime.datetime(2026, 2, 1, 12, 0, tzinfo=UTC)  # dentro de la vigencia de «carla»

    f = {}  # nombre de fichero -> bytes

    f["sin-firmas.pdf"] = base
    f["sin-firmas-flujos.pdf"] = flujos

    # (a) firmados válidos, por cada herramienta y cada algoritmo
    f["firma-valida-rsa.pdf"] = g.firmar(base, "ana", "Firma1")
    f["firma-valida-ecdsa-sello.pdf"] = g.firmar(
        base, "bruno", "Firma1", cadena="cadena-a-sin-raiz", sello=g.sello_openssl())
    f["firma-valida-sha384.pdf"] = g.firmar(base, "ana", "Firma1", md="sha384")
    f["firma-pkcs7-detached.pdf"] = g.firmar(
        base, "ana", "Firma1", subfilter=SigSeedSubFilter.ADOBE_PKCS7_DETACHED)
    f["firma-valida-rsa-pss.pdf"] = g.firmar(base, "ana", "Firma1", pss=True)
    f["firma-valida-flujos.pdf"] = g.firmar(flujos, "bruno", "Firma1")
    f["firma-openssl-cms.pdf"] = g.firmar_a_mano_con_openssl(base, "ana", "Firma1")

    # (b) manipulados DENTRO de lo firmado
    f["manipulado-contenido.pdf"] = tocar_contenido_firmado(f["firma-valida-rsa.pdf"])
    f["manipulado-subfiltro.pdf"] = cambiar_subfiltro(f["firma-pkcs7-detached.pdf"])
    f["manipulado-byterange.pdf"] = desplazar_byterange(f["firma-valida-rsa.pdf"])

    # (c) contenido añadido DESPUÉS de firmar, por actualización incremental
    firmado = f["firma-valida-rsa.pdf"]
    f["cambio-posterior-pagina.pdf"] = g.cambiar_pagina(firmado, 1)
    f["cambio-posterior-info.pdf"] = g.cambiar_titulo(firmado)
    f["cambio-posterior-anotacion.pdf"] = g.anotar_pagina(firmado, 0)

    # (d) vigencia del certificado
    f["caducado-sin-sello.pdf"] = g.firmar(base, "carla", "Firma1")
    f["caducado-con-sello-de-su-epoca.pdf"] = g.firmar(
        base, "carla", "Firma1", sello=g.sello_con_fecha(epoca))
    f["aun-no-vigente.pdf"] = g.firmar(base, "dario", "Firma1")

    # (e) sin sello / con sello (el sin sello es firma-valida-rsa; el con sello,
    #     firma-valida-ecdsa-sello) y un sello de una autoridad que no es de confianza
    f["sello-de-otra-firma.pdf"] = g.firmar(base, "ana", "Firma1", sello=g.sello_de_otra_firma())
    f["sello-no-confiable.pdf"] = g.firmar(
        base, "ana", "Firma1", sello=g.sello_openssl("tsa-otra"))

    # (f) firma de certificación (DocMDP) con cada nivel y lo que viene después
    f["certificacion-p1.pdf"] = g.firmar(base, "ana", "Certificacion", certificar=MDPPerm.NO_CHANGES)
    f["certificacion-p1-con-segunda-firma.pdf"] = g.firmar(
        f["certificacion-p1.pdf"], "bruno", "Firma2", contra_la_certificacion=True)
    f["certificacion-p2-con-segunda-firma.pdf"] = g.firmar(
        g.firmar(base, "ana", "Certificacion", certificar=MDPPerm.FILL_FORMS),
        "bruno", "Firma2")
    f["certificacion-p2-con-cambio-de-pagina.pdf"] = g.cambiar_pagina(
        g.firmar(base, "ana", "Certificacion", certificar=MDPPerm.FILL_FORMS), 0)
    f["certificacion-p3-con-comentario.pdf"] = g.anotar_pagina(
        g.firmar(base, "ana", "Certificacion", certificar=MDPPerm.ANNOTATE), 0)

    f["certificacion-p3-con-cambio-de-pagina.pdf"] = g.cambiar_pagina(
        g.firmar(base, "ana", "Certificacion", certificar=MDPPerm.ANNOTATE), 0)

    # (g) varias firmas encadenadas
    f["firma-y-contrafirma.pdf"] = g.firmar(
        f["firma-valida-rsa.pdf"], "bruno", "Firma2", sello=g.sello_openssl())
    f["tres-firmas-y-sello-de-documento.pdf"] = g.sello_de_documento(
        f["firma-y-contrafirma.pdf"], g.sello_openssl())
    f["firma-tras-cambio-de-pagina.pdf"] = g.firmar(
        g.cambiar_pagina(f["firma-valida-rsa.pdf"], 1), "elena", "Firma2")

    # (h) cadena cuya raíz no está en las anclas
    f["raiz-desconocida.pdf"] = g.firmar(base, "intruso", "Firma1", cadena="cadena-b")

    for nombre, datos in f.items():
        with open(os.path.join(a.salida, nombre), "wb") as out:
            out.write(datos)


if __name__ == "__main__":
    main()
