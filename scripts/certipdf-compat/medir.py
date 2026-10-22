# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
"""Mide CertiPDF contra un corpus de PDF ajenos y deja una fila por fichero.

La mitad de CertiPDF la pone `pdfcompat_medida_test.go` (abrir, pintar, texto,
formulario, anotar, censurar, editar, firmar, verificar). Esta mitad pone la VARA
DE MEDIR, siempre con herramientas ajenas usadas como CAJA NEGRA —se ejecutan sus
binarios, nunca se lee su código (docs/security.md §4)—:

  * Poppler (pdftoppm, pdftotext, pdfinfo, pdffonts, pdfimages): referencia de
    pintado y de texto, y clasificación del fichero.
  * qpdf --check: si lo que escribe CertiPDF es un PDF válido.
  * pypdf (BSD): segundo lector de lo que sale; lee los campos rellenados.
  * pyHanko (MIT): validador independiente de las firmas que pone CertiPDF y de
    las que ya traía el documento.

Uso (en el frontal, con el venv de herramientas):
  medir.py --corpus DIR --salida DIR --binario domain.test --herramientas DIR [--hilos 3] [--solo patrón]
"""

import argparse
import concurrent.futures
import json
import logging
import os
import re
import resource
import shutil
import subprocess
import sys
import time
import unicodedata
from collections import Counter

import numpy as np
from PIL import Image

Image.MAX_IMAGE_PIXELS = None
ESCALA = 1.5
DPI = int(72 * ESCALA)
PAGINAS = 3

# Contraseñas conocidas del corpus (las publica el propio corpus).
CLAVES = {}

H = {}  # rutas de herramientas, se rellena en main


def run(args, timeout=90, entrada=None):
    try:
        p = subprocess.run(args, capture_output=True, timeout=timeout, env=H["env"], input=entrada)
        return p.returncode, p.stdout.decode("utf-8", "replace"), p.stderr.decode("utf-8", "replace")
    except subprocess.TimeoutExpired:
        return -9, "", "timeout"


# --- Clasificación ------------------------------------------------------------

PRODUCTORES = [
    ("word", r"microsoft.{0,20}word|word.{0,10}microsoft"),
    ("office-otro", r"microsoft.{0,20}(excel|powerpoint|office|publisher)|microsoft: print to pdf"),
    ("libreoffice", r"libreoffice|openoffice|staroffice"),
    ("acrobat", r"acrobat|distiller|adobe pdf library|pdfmaker|adobe indesign|adobe illustrator|adobe photoshop|framemaker"),
    ("chrome", r"skia|chrom|google docs"),
    ("quartz", r"quartz|mac os x|macos"),
    ("ghostscript", r"ghostscript|gpl gs|afpl"),
    ("escaner", r"scan|canon|xerox|ricoh|konica|kyocera|sharp|epson|hp digital|fujitsu|kodak|abbyy|paperport|omnipage|brother|lexmark|toshiba|captiva|kofax|readiris|naps2|bizhub|imagerunner|docuware"),
    ("generador", r"itext|pdfsharp|tcpdf|fpdf|reportlab|wkhtmltopdf|prince|dompdf|jasper|crystal|birt|aspose|pdflib|qt |pdfkit|mpdf|fop|weasyprint|cairo|xpdf|foxit|nitro|pdf-xchange|pdfcreator|primopdf|cutepdf|bullzip|dopdf|pdf24|novapdf|winnovative|evo|syncfusion|devexpress|telerik|stimulsoft|sap|oracle|ibm|hummingbird"),
    ("latex", r"tex|dvipdf|xetex|luatex"),
    ("bibliotecas", r"pdfbox|qpdf|pypdf|pyhanko|pdfium|mupdf|pdf\.js|hexapdf|pdfjs|pikepdf|pdfmake|pdf-lib|jspdf"),
]


def pdfinfo(ruta, clave):
    args = [H["pdfinfo"], "-isodates"]
    if clave:
        args += ["-upw", clave]
    rc, out, err = run(args + [ruta], timeout=60)
    d = {}
    for l in out.splitlines():
        if ":" in l:
            k, v = l.split(":", 1)
            d[k.strip()] = v.strip()
    return rc, d, err


def tabla_poppler(texto):
    """Lee las tablas de pdffonts/pdfimages usando la fila de guiones."""
    lineas = texto.splitlines()
    for i, l in enumerate(lineas):
        if l.startswith("---"):
            cols = [(m.start(), m.end()) for m in re.finditer(r"-+", l)]
            cab = lineas[i - 1]
            nombres = [cab[a:b + 1].strip() for a, b in cols]
            filas = []
            for f in lineas[i + 1:]:
                if not f.strip():
                    continue
                # La última columna se lleva lo que sobre.
                celdas = [f[a:(cols[j + 1][0] if j + 1 < len(cols) else len(f))].strip() for j, (a, _) in enumerate(cols)]
                filas.append(dict(zip(nombres, celdas)))
            return filas
    return []


def clasificar(ruta, datos, clave):
    c = {"bytes": len(datos)}
    m = re.match(rb"%PDF-(\d\.\d)", datos[:1024].lstrip()) or re.search(rb"%PDF-(\d\.\d)", datos[:4096])
    c["version"] = m.group(1).decode() if m else "?"
    rc, info, err = pdfinfo(ruta, clave)
    c["poppler_abre"] = rc == 0
    c["poppler_error"] = err.strip().splitlines()[-1][:200] if rc != 0 and err.strip() else ""
    prod = (info.get("Producer", "") + " | " + info.get("Creator", "")).strip(" |")
    c["productor_texto"] = prod[:160]
    c["productor"] = "desconocido" if not prod else "otro"
    for nombre, rx in PRODUCTORES:
        if re.search(rx, prod, re.I):
            c["productor"] = nombre
            break
    c["paginas_poppler"] = int(info.get("Pages", "0") or 0)
    c["formulario"] = info.get("Form", "none")
    c["etiquetado"] = info.get("Tagged", "no") == "yes"
    c["linealizado"] = info.get("Optimized", "no") == "yes" or b"/Linearized" in datos[:2048]
    c["javascript"] = info.get("JavaScript", "no") == "yes"
    c["xref_flujo"] = bool(re.search(rb"/Type\s*/XRef", datos))
    n_startxref = datos.count(b"startxref")
    c["incremental"] = n_startxref > (2 if c["linealizado"] else 1)
    # Cifrado.
    rcq, outq, _ = run([H["qpdf"], "--show-encryption"] + ([f"--password={clave}"] if clave else []) + [ruta], timeout=60)
    c["cifrado"] = "no"
    if "R = " in outq:
        r = re.search(r"R = (\d)", outq).group(1)
        metodo = re.search(r"stream encryption method: (\S+)", outq)
        c["cifrado"] = f"R{r}-{metodo.group(1) if metodo else '?'}"
        rcp, _, _ = run([H["qpdf"], "--requires-password", ruta], timeout=60)
        c["clave_usuario"] = rcp == 0
    # Daños: lo que qpdf dice del fichero de ENTRADA.
    rcc, outc, errc = run([H["qpdf"], "--check"] + ([f"--password={clave}"] if clave else []) + [ruta], timeout=90)
    c["qpdf_entrada"] = {0: "ok", 3: "avisos", 2: "errores"}.get(rcc, f"rc{rcc}")
    c["danado"] = bool(re.search(r"damaged|reconstruct|xref not found|invalid xref|startxref", outc + errc, re.I))
    # Fuentes.
    rcf, outf, _ = run([H["pdffonts"], "-l", "10"] + (["-upw", clave] if clave else []) + [ruta], timeout=60)
    fuentes = tabla_poppler(outf)
    tipos = set()
    for f in fuentes:
        t = f.get("type", "")
        base = "type3" if "Type 3" in t else "cid" if t.startswith("CID") else "truetype" if "TrueType" in t else "type1" if "Type 1" in t else t.lower() or "?"
        emb = "incrustada" if f.get("emb") == "yes" else "no-incrustada"
        tipos.add(f"{base}-{emb}")
        if f.get("encoding", "").endswith("-V"):
            c["vertical"] = True
    c["fuentes"] = sorted(tipos)
    # Imágenes.
    rci, outi, _ = run([H["pdfimages"], "-list", "-l", "10"] + (["-upw", clave] if clave else []) + [ruta], timeout=60)
    imgs = tabla_poppler(outi)
    encs = set()
    for im in imgs:
        e = im.get("enc", "")
        e = {"image": "flate/crudo", "jpeg": "dct", "jp2": "jpx"}.get(e, e)
        encs.add(e)
        if im.get("type") in ("smask", "mask", "stencil"):
            encs.add("mascara")
    c["imagenes"] = sorted(encs)
    c["escaneado"] = bool(imgs) and not fuentes
    # Normas declaradas en el XMP.
    pa = re.search(rb"pdfaid:part\s*(?:=\s*[\"']|>)\s*(\d)", datos)
    if pa:
        conf = re.search(rb"pdfaid:conformance\s*(?:=\s*[\"']|>)\s*([A-Za-z])", datos)
        c["pdfa"] = pa.group(1).decode() + (conf.group(1).decode().lower() if conf else "")
    if re.search(rb"pdfuaid:part", datos):
        c["pdfua"] = True
    # Firmas.
    n_firmas = len(re.findall(rb"/ByteRange\s*\[", datos))
    if n_firmas:
        sub = sorted({s.decode("latin-1") for s in re.findall(rb"/SubFilter\s*/([A-Za-z0-9.#_-]+)", datos)})
        c["firmas"] = n_firmas
        c["subfiltros"] = sub
        nivel = "B-B" if any("CAdES" in s for s in sub) else "pkcs7"
        if b"\x06\x0b\x2a\x86\x48\x86\xf7\x0d\x01\x09\x10\x02\x0e" in datos or re.search(rb"060b2a864886f70d010910020e", datos, re.I):
            nivel = "B-T" if nivel == "B-B" else "pkcs7+sello"
        if re.search(rb"/DSS\b", datos):
            nivel = "B-LT" if nivel.startswith("B-") else nivel + "+DSS"
            if b"/DocTimeStamp" in datos:
                nivel = "B-LTA" if nivel == "B-LT" else nivel
        c["nivel_firma"] = nivel
    return c


def estructura(ruta, clave=""):
    """Lo que un lector AJENO (pypdf) ve en un fichero: para saber qué se pierde."""
    from pypdf import PdfReader

    e = {}
    try:
        r = PdfReader(ruta, strict=False)
        if r.is_encrypted:
            r.decrypt(clave or "")
        root = r.trailer["/Root"].get_object()
        e["paginas"] = len(r.pages)

        def contar(o):
            return sum(contar(x) if isinstance(x, list) else 1 for x in o)

        try:
            e["marcadores"] = contar(r.outline)
        except Exception:  # noqa: BLE001
            e["marcadores"] = -1
        for k, nombre in [("/StructTreeRoot", "estructura"), ("/Lang", "idioma"), ("/PageLabels", "etiquetas_pagina"),
                          ("/OCProperties", "capas"), ("/OutputIntents", "output_intents"), ("/Metadata", "xmp"),
                          ("/ViewerPreferences", "preferencias"), ("/OpenAction", "accion_apertura"), ("/DSS", "dss")]:
            e[nombre] = k in root
        nombres = root.get("/Names")
        nombres = nombres.get_object() if nombres is not None else {}
        e["destinos"] = "/Dests" in nombres or "/Dests" in root
        try:
            e["adjuntos"] = len(r.attachments)
        except Exception:  # noqa: BLE001
            e["adjuntos"] = -1
        af = root.get("/AcroForm")
        af = af.get_object() if af is not None else {}
        e["xfa"] = "/XFA" in af
        try:
            campos = r.get_fields() or {}
        except Exception:  # noqa: BLE001
            campos = {}
        e["campos"] = len(campos)
        e["firmas"] = sum(1 for v in campos.values() if v.get("/FT") == "/Sig" and v.get("/V") is not None)
        try:
            annots = r.pages[0].get("/Annots")
            e["anotaciones_p1"] = len(annots.get_object()) if annots is not None else 0
        except Exception:  # noqa: BLE001
            e["anotaciones_p1"] = -1
    except Exception as ex:  # noqa: BLE001
        e["error"] = f"{type(ex).__name__}: {str(ex)[:160]}"
    return e


def pypdf_estricto(ruta):
    from pypdf import PdfReader

    try:
        r = PdfReader(ruta, strict=True)
        _ = [p.mediabox for p in r.pages]
        return "ok"
    except Exception as ex:  # noqa: BLE001
        return f"{type(ex).__name__}: {str(ex)[:120]}"


def pypdf_texto(ruta, clave="", paginas=20):
    """El texto según pypdf, un tercer lector independiente (BSD, como caja negra):
    solo hace de desempate cuando Poppler y CertiPDF no coinciden."""
    from pypdf import PdfReader

    try:
        r = PdfReader(ruta, strict=False)
        if r.is_encrypted:
            r.decrypt(clave)
        return [(p.extract_text() or "") for p in r.pages[:paginas]]
    except Exception:  # noqa: BLE001
        return None


def recall(ref, otro):
    total = sum(ref.values())
    return round(sum((ref & otro).values()) / total, 4) if total else None


# --- Comparaciones --------------------------------------------------------------


def plegar(s):
    s = unicodedata.normalize("NFKD", s)
    return "".join(ch for ch in s if not unicodedata.combining(ch)).casefold()


def palabras(s):
    return re.findall(r"\w+", plegar(s))


def cargar(ruta):
    with Image.open(ruta) as im:
        return np.asarray(im.convert("RGB"), dtype=np.int16)


def comparar_imagenes(a, b):
    """a = nuestra, b = referencia. Diferencia que se VE, no la del suavizado."""
    h, w = min(a.shape[0], b.shape[0]), min(a.shape[1], b.shape[1])
    d = {"tam_nuestro": [a.shape[1], a.shape[0]], "tam_ref": [b.shape[1], b.shape[0]]}
    if h == 0 or w == 0:
        return d | {"clase": "sin-imagen"}
    a, b = a[:h, :w], b[:h, :w]
    dif = np.abs(a - b).max(axis=2)
    d["pix64"] = round(float((dif > 64).mean() * 100), 3)
    # Bloques de 8x8: el suavizado y medio píxel de desplazamiento de una letra
    # desaparecen en la media; una letra que falta o un color equivocado, no.
    hb, wb = h // 8, w // 8
    if hb and wb:
        ma = a[: hb * 8, : wb * 8].reshape(hb, 8, wb, 8, 3).mean(axis=(1, 3))
        mb = b[: hb * 8, : wb * 8].reshape(hb, 8, wb, 8, 3).mean(axis=(1, 3))
        d["bloques"] = round(float((np.abs(ma - mb).max(axis=2) > 40).mean() * 100), 3)
    else:
        d["bloques"] = d["pix64"]
    tinta_a = float((a.mean(axis=2) < 128).mean() * 100)
    tinta_b = float((b.mean(axis=2) < 128).mean() * 100)
    d["tinta_nuestra"], d["tinta_ref"] = round(tinta_a, 3), round(tinta_b, 3)
    nb_a = float((a.min(axis=2) < 245).mean() * 100)  # cualquier cosa que no sea papel
    nb_b = float((b.min(axis=2) < 245).mean() * 100)
    if abs(a.shape[0] - b.shape[0]) > 3 or abs(a.shape[1] - b.shape[1]) > 3:
        d["clase"] = "tamano"
    elif nb_b > 0.5 and nb_a < nb_b * 0.05:
        d["clase"] = "en-blanco"
    elif d["bloques"] < 0.5:
        d["clase"] = "igual"
    elif d["bloques"] < 3:
        d["clase"] = "leve"
    elif d["bloques"] < 15:
        d["clase"] = "visible"
    else:
        d["clase"] = "grave"
    return d


def refs_pagina(dirsal, prefijo):
    out = {}
    for f in os.listdir(dirsal):
        m = re.fullmatch(re.escape(prefijo) + r"-0*(\d+)\.png", f)
        if m:
            out[int(m.group(1))] = os.path.join(dirsal, f)
    return out


def poppler_pinta(ruta, dirsal, prefijo, clave="", primera=1, ultima=PAGINAS, dpi=DPI):
    args = [H["pdftoppm"], "-r", str(dpi), "-f", str(primera), "-l", str(ultima), "-png"]
    if clave:
        args += ["-upw", clave]
    rc, _, err = run(args + [ruta, os.path.join(dirsal, prefijo)], timeout=120)
    errores = len(re.findall(r"Syntax Error|Internal Error|Error:", err))
    return rc, errores, err.strip().splitlines()[:3]


def poppler_texto(ruta, clave=""):
    args = [H["pdftotext"], "-enc", "UTF-8", "-f", "1", "-l", "20"]
    if clave:
        args += ["-upw", clave]
    rc, out, _ = run(args + [ruta, "-"], timeout=90)
    return out.split("\f") if rc == 0 else None


def qpdf_check(ruta, clave=""):
    rc, out, err = run([H["qpdf"], "--check"] + ([f"--password={clave}"] if clave else []) + [ruta], timeout=120)
    clase = {0: "ok", 3: "avisos", 2: "errores"}.get(rc, f"rc{rc}")
    lineas = [l for l in (out + err).splitlines() if re.search(r"WARNING|error|damaged", l, re.I)]
    return {"clase": clase, "detalle": lineas[:3]}


def validar_firmas_pyhanko(ruta, raiz=None):
    from pyhanko.keys import load_cert_from_pemder
    from pyhanko.pdf_utils.reader import PdfFileReader
    from pyhanko.sign.validation import validate_pdf_signature, validate_pdf_timestamp
    from pyhanko_certvalidator import ValidationContext

    out = []
    try:
        with open(ruta, "rb") as f:
            r = PdfFileReader(f, strict=False)
            for sig in r.embedded_signatures:
                vc = ValidationContext(trust_roots=[load_cert_from_pemder(raiz)] if raiz else [], allow_fetching=False)
                fila = {"campo": sig.field_name}
                try:
                    if sig.sig_object.get("/Type") == "/DocTimeStamp":
                        st = validate_pdf_timestamp(sig, validation_context=vc)
                    else:
                        st = validate_pdf_signature(sig, vc)
                    fila |= {"intacta": bool(st.intact), "valida": bool(st.valid), "confianza": bool(st.trusted),
                             "cobertura": str(getattr(st, "coverage", "")).split(".")[-1],
                             "modificacion": str(getattr(st, "modification_level", "")).split(".")[-1]}
                except Exception as ex:  # noqa: BLE001
                    fila["error"] = f"{type(ex).__name__}: {str(ex)[:160]}"
                out.append(fila)
    except Exception as ex:  # noqa: BLE001
        return [{"error": f"{type(ex).__name__}: {str(ex)[:160]}"}]
    return out


# --- Un fichero ---------------------------------------------------------------


def franja_ampliada(ruta, clave, hh):
    """Las filas de la imagen que no se comparan al censurar: la franja (del 40 al
    60 % de la altura) y además la altura entera de cada palabra de la entrada que
    la cruza. Censurar bien es quitar el glifo ENTERO que toca la zona —medio
    glifo se lee—, así que su tinta fuera de la franja tiene que desaparecer y no
    es una pérdida. Sin esto, una página de una línea que cruza la franja (los
    300×54 de pdf.js) contaba como «en blanco»."""
    ini, fin = int(hh * 0.4) - 3, int(hh * 0.6) + 3
    args = [H["pdftotext"], "-f", "1", "-l", "1", "-bbox"] + (["-upw", clave] if clave else []) + [ruta, "-"]
    _, xml, _ = run(args, timeout=60)
    alto = re.search(r'<page width="[\d.]+" height="([\d.]+)"', xml or "")
    if not alto or float(alto.group(1)) <= 0:
        return max(ini, 0), fin
    k = hh / float(alto.group(1))
    for y0, y1 in re.findall(r'yMin="([\d.]+)" xMax="[\d.]+" yMax="([\d.]+)"', xml):
        a, z = float(y0) * k, float(y1) * k
        if a < fin and z > ini:
            ini, fin = min(ini, int(a) - 2), max(fin, int(z) + 2)
    return max(ini, 0), min(fin, hh)


def medir_fichero(tarea):
    ruta, origen, rel = tarea
    ident = re.sub(r"[^A-Za-z0-9._-]", "_", f"{origen}__{rel}")[-150:]
    dirsal = os.path.join(H["salida"], "ficheros", ident)
    final = os.path.join(dirsal, "medida.json")
    if os.path.exists(final) and not H["rehacer"]:
        with open(final) as f:
            return json.load(f)
    shutil.rmtree(dirsal, ignore_errors=True)
    os.makedirs(dirsal)
    with open(ruta, "rb") as f:
        datos = f.read()
    clave = CLAVES.get(os.path.basename(ruta), "")
    fila = {"id": ident, "origen": origen, "fichero": rel, "ruta": ruta, "dir": dirsal, "clave": bool(clave)}
    t0 = time.time()
    fila["clase"] = clasificar(ruta, datos, clave)

    # Referencia de Poppler (entrada).
    rc, errs, _ = poppler_pinta(ruta, dirsal, "ref", clave)
    refs = refs_pagina(dirsal, "ref")
    ref_texto = poppler_texto(ruta, clave)

    # CertiPDF.
    env = dict(H["env"]) | {
        "SYTUE_COMPAT_PDF": ruta, "SYTUE_COMPAT_OUT": dirsal, "SYTUE_COMPAT_CLAVE": clave,
        "SYTUE_COMPAT_CERT": H["cert"], "SYTUE_COMPAT_KEY": H["key"], "SYTUE_COMPAT_PAGINAS": str(PAGINAS),
        "SYTUE_COMPAT_ESCALA": str(ESCALA), "GOMAXPROCS": "2", "SYTUE_COMPAT_MAX_MIB": "2048",
    }
    try:
        p = subprocess.run([H["binario"], "-test.run", "^TestMedirCompatibilidad$", "-test.count=1"],
                           capture_output=True, timeout=H["limite"], env=env, cwd=dirsal, preexec_fn=_sin_tope)
        rc_go, err_go = p.returncode, p.stdout.decode("utf-8", "replace")[-2000:] + p.stderr.decode("utf-8", "replace")[-2000:]
    except subprocess.TimeoutExpired:
        rc_go, err_go = -9, "timeout"
    res = {}
    try:
        with open(os.path.join(dirsal, "resultado.json")) as f:
            res = json.load(f)
    except Exception:  # noqa: BLE001
        pass
    fila["go_rc"] = rc_go
    if rc_go != 0:
        m = re.search(r"(panic: .{0,300})", err_go)
        fila["go_fallo"] = ("timeout en " + res.get("en_curso", "?")) if rc_go == -9 else (m.group(1) if m else err_go[-400:])
        fila["go_en_curso"] = res.get("en_curso", "")
    ops = res.get("ops", {})
    fila["ops"] = {k: {kk: vv for kk, vv in v.items() if kk != "datos"} | ({"datos": v["datos"]} if v.get("datos") else {})
                   for k, v in ops.items()}

    # Pintado: nuestra página contra la de Poppler.
    pint = {}
    for n, rref in refs.items():
        nuestro = os.path.join(dirsal, f"nuestro-{n}.png")
        if not os.path.exists(nuestro):
            continue
        try:
            pint[n] = comparar_imagenes(cargar(nuestro), cargar(rref))
        except Exception as ex:  # noqa: BLE001
            pint[n] = {"clase": "error", "error": str(ex)[:120]}
    fila["pintado"] = pint
    fila["ref_paginas_pintadas"] = len(refs)

    # Texto y búsqueda.
    try:
        with open(os.path.join(dirsal, "texto.json")) as f:
            nuestro_texto = json.load(f) or []
    except Exception:  # noqa: BLE001
        nuestro_texto = []
    if ref_texto is not None and nuestro_texto:
        ref_p, nue_p, inter, busq, busq_ok = 0, 0, 0, 0, 0
        for i, pag in enumerate(nuestro_texto):
            if i >= len(ref_texto):
                break
            rp = palabras(ref_texto[i])
            if len(rp) < 5:
                continue
            npal = Counter(palabras(pag.get("plano") or ""))
            rc_ = Counter(rp)
            ref_p += sum(rc_.values())
            nue_p += sum(npal.values())
            inter += sum((rc_ & npal).values())
            frags = [plegar(x) for x in pag.get("fragmentos") or []]
            candidatas = [w for w in dict.fromkeys(rp) if len(w) >= 4 and not w.isdigit()][:25]
            for w in candidatas:
                busq += 1
                busq_ok += any(w in f for f in frags)
        if ref_p:
            fila["texto"] = {"recall": round(inter / ref_p, 4), "precision": round(inter / nue_p, 4) if nue_p else 0.0,
                             "palabras_ref": ref_p, "buscar": round(busq_ok / busq, 4) if busq else None}
            # Desempate: si CertiPDF no llega al 95 % de Poppler, ¿qué dice pypdf?
            # Cuánto de lo que lee pypdf tenemos nosotros y cuánto tiene Poppler.
            if fila["texto"]["recall"] < 0.95:
                pt = pypdf_texto(ruta, clave)
                if pt:
                    n = min(len(pt), len(nuestro_texto), len(ref_texto))
                    pp = Counter(palabras("\n".join(pt[:n])))
                    fila["texto"]["pypdf"] = {
                        "nuestro": recall(pp, Counter(palabras("\n".join((x.get("plano") or "") for x in nuestro_texto[:n])))),
                        "poppler": recall(pp, Counter(palabras("\n".join(ref_texto[:n])))),
                        "palabras": sum(pp.values()),
                    }
    rtl = ref_texto and re.search(r"[֐-ۿݐ-ݿיִ-﷿ﹰ-﻿]", "".join(ref_texto[:5]))
    if rtl:
        fila["clase"]["rtl"] = True

    # Lo que sale: cada PDF que escribe CertiPDF.
    salidas = res.get("salidas", {})
    est_in = estructura(ruta, clave)
    fila["estructura_entrada"] = est_in
    fila["salidas"] = {}
    for op, rsal in salidas.items():
        s = {"ruta": rsal, "qpdf": qpdf_check(rsal)}
        s["pypdf"] = pypdf_estricto(rsal)
        rcp, errp, _ = poppler_pinta(rsal, dirsal, f"sal-{op}", "", 1, 1)
        s["poppler"] = "ok" if rcp == 0 and errp == 0 else f"rc{rcp} errores {errp}"
        est = estructura(rsal)
        perdido = []
        for k, v in est_in.items():
            if k in ("error", "paginas", "anotaciones_p1"):
                continue
            ov = est.get(k)
            if (v is True and ov is not True) or (isinstance(v, int) and not isinstance(v, bool) and v > 0 and isinstance(ov, int) and ov < v):
                perdido.append(k)
        if est_in.get("paginas") and est.get("paginas") != est_in.get("paginas"):
            perdido.append("paginas")
        s["pierde"] = perdido
        sal_refs = refs_pagina(dirsal, f"sal-{op}")
        if op in ("firmado",) and 1 in sal_refs and 1 in refs:
            # Una firma invisible no puede cambiar ni un píxel de la página.
            s["pintado_vs_entrada"] = comparar_imagenes(cargar(sal_refs[1]), cargar(refs[1]))
        if op == "rotado" and 1 in sal_refs and 1 in refs:
            s["pintado_vs_entrada"] = comparar_imagenes(np.rot90(cargar(sal_refs[1]), 1), cargar(refs[1]))
        if op == "censurado" and 1 in sal_refs and 1 in refs:
            a, b = cargar(sal_refs[1]), cargar(refs[1])
            hh = min(a.shape[0], b.shape[0])
            fuera = np.ones(hh, bool)
            ini, fin = franja_ampliada(ruta, clave, hh)
            fuera[ini:fin] = False  # la franja (eje Y de imagen al revés, simétrica)
            s["pintado_fuera_de_franja"] = comparar_imagenes(a[:hh][fuera], b[:hh][fuera])
            # ¿Queda texto legible en la franja?
            rct, xml, _ = run([H["pdftotext"], "-f", "1", "-l", "1", "-bbox", rsal, "-"], timeout=60)
            alto = re.search(r'<page width="([\d.]+)" height="([\d.]+)"', xml)
            if alto:
                hp = float(alto.group(2))
                fuga = [w for y0, y1, w in re.findall(r'yMin="([\d.]+)" xMax="[\d.]+" yMax="([\d.]+)">([^<]*)<', xml)
                        if hp * 0.42 < (float(y0) + float(y1)) / 2 < hp * 0.58 and len(w.strip()) > 1]
                s["fuga_franja"] = fuga[:10]
        if op == "formulario":
            s["campos_pypdf"] = comparar_campos(rsal, os.path.join(dirsal, "valores.json"))
        if op == "firmado":
            with open(rsal, "rb") as f:
                firmado = f.read()
            s["prefijo_intacto"] = firmado[: len(datos)] == datos
            s["pyhanko"] = validar_firmas_pyhanko(rsal, H["raiz"])
            rcs, outs, _ = run([H["pdfsig"], rsal], timeout=60)
            s["pdfsig"] = re.findall(r"Signature Validation: (.+)", outs)
            # Firmar ANEXA: los bytes de la entrada siguen tal cual. Si un lector se
            # queja, se mira si ya se quejaba de la entrada (lo heredado no es de
            # CertiPDF, igual que con los errores de Poppler en poppler.py).
            if s["prefijo_intacto"]:
                ent = {}
                if s["pypdf"] != "ok":
                    ent["pypdf"] = pypdf_estricto(ruta)
                if s["qpdf"]["clase"] not in ("ok", "avisos"):
                    ent["qpdf"] = qpdf_check(ruta, clave)["clase"]
                if not any(x.get("campo", "").startswith("Sytue") for x in s["pyhanko"]):
                    ent["pyhanko"] = validar_firmas_pyhanko(ruta)
                if ent:
                    s["entrada"] = ent
        if op == "editado":
            txt = poppler_texto(rsal)
            nuevo = (ops.get("editar", {}).get("datos") or {}).get("nuevo", "")
            s["texto_nuevo_visible"] = bool(txt and nuevo and plegar(nuevo.strip()) in plegar(txt[0]).replace("\n", " "))
        if op in ("anotado", "formulario") and fila["clase"].get("firmas"):
            # Un documento firmado se anota o se rellena ANEXANDO: lo firmado sigue
            # byte a byte y sus firmas siguen validando.
            with open(rsal, "rb") as f:
                s["prefijo_intacto"] = f.read()[: len(datos)] == datos
            s["pyhanko"] = validar_firmas_pyhanko(rsal)
        if op == "anotado":
            s["anotaciones_pypdf"] = est.get("anotaciones_p1")
            s["anotaciones_entrada"] = est_in.get("anotaciones_p1")
        fila["salidas"][op] = s
    if fila["clase"].get("firmas"):
        fila["firmas_entrada_pyhanko"] = validar_firmas_pyhanko(ruta)
    fila["segundos"] = round(time.time() - t0, 2)
    # Las PNG pesan: se quedan solo las de las páginas con diferencia visible.
    for n, d in pint.items():
        if d.get("clase") in ("igual", "leve"):
            for pref in ("nuestro", "ref"):
                p = os.path.join(dirsal, f"{pref}-{n}.png")
                if pref == "ref":
                    p = refs.get(n, p)
                if os.path.exists(p):
                    os.remove(p)
    for f in os.listdir(dirsal):
        if f.startswith("sal-") and f.endswith(".png"):
            os.remove(os.path.join(dirsal, f))
    # json ida y vuelta: pyHanko y pypdf devuelven str/int con referencias al
    # lector abierto dentro, que no se pueden mandar de un proceso a otro.
    fila = json.loads(json.dumps(fila, ensure_ascii=False))
    with open(final, "w") as f:
        json.dump(fila, f, ensure_ascii=False)
    return fila


def comparar_campos(ruta, valores_json):
    from pypdf import PdfReader

    try:
        with open(valores_json) as f:
            valores = json.load(f)
        campos = PdfReader(ruta, strict=False).get_fields() or {}
    except Exception as ex:  # noqa: BLE001
        return {"error": str(ex)[:160]}
    bien, mal = 0, []
    for nombre, v in valores.items():
        c = campos.get(nombre)
        got = None if c is None else c.get("/V")
        got = str(got) if got is not None else None
        ok = got is not None and (got == v or got.lstrip("/") == v.lstrip("/") or (v == "sí" and got not in ("/Off", "Off", "")))
        bien += ok
        if not ok and len(mal) < 3:
            mal.append([nombre, v, got])
    return {"bien": bien, "total": len(valores), "mal": mal}


def pki(d):
    """Raíz y firmante RSA-2048 de usar y tirar para firmar en la medida."""
    import datetime

    from cryptography import x509
    from cryptography.hazmat.primitives import hashes, serialization
    from cryptography.hazmat.primitives.asymmetric import rsa
    from cryptography.x509.oid import NameOID

    if os.path.exists(os.path.join(d, "cadena.pem")):
        return
    os.makedirs(d, exist_ok=True)
    ahora = datetime.datetime.now(datetime.timezone.utc)

    def cert(cn, emisor, clave_pub, clave_firma, ca, dias):
        sujeto = x509.Name([x509.NameAttribute(NameOID.ORGANIZATION_NAME, "Sytue Pruebas"), x509.NameAttribute(NameOID.COMMON_NAME, cn)])
        uso = x509.KeyUsage(not ca, not ca, False, False, False, ca, ca, False, False)
        return (x509.CertificateBuilder().subject_name(sujeto).issuer_name(emisor or sujeto).public_key(clave_pub)
                .serial_number(x509.random_serial_number()).not_valid_before(ahora - datetime.timedelta(days=1))
                .not_valid_after(ahora + datetime.timedelta(days=dias))
                .add_extension(x509.BasicConstraints(ca=ca, path_length=None), critical=True)
                .add_extension(uso, critical=True).sign(clave_firma, hashes.SHA256()))

    kr = rsa.generate_private_key(public_exponent=65537, key_size=2048)
    raiz = cert("Raiz de pruebas CertiPDF", None, kr.public_key(), kr, True, 3650)
    kf = rsa.generate_private_key(public_exponent=65537, key_size=2048)
    firm = cert("Firmante de pruebas", raiz.subject, kf.public_key(), kr, False, 730)
    pem = lambda c: c.public_bytes(serialization.Encoding.PEM)  # noqa: E731
    with open(os.path.join(d, "raiz.pem"), "wb") as f:
        f.write(pem(raiz))
    with open(os.path.join(d, "cadena.pem"), "wb") as f:
        f.write(pem(firm) + pem(raiz))
    with open(os.path.join(d, "clave.pem"), "wb") as f:
        f.write(kf.private_bytes(serialization.Encoding.PEM, serialization.PrivateFormat.PKCS8, serialization.NoEncryption()))


def sabor_veraPDF(pdfa):
    """«2u» → 2u; «1» → 1b; «4» → 4; vacío o raro → None."""
    if not pdfa or not re.fullmatch(r"[1-4][abue]?", pdfa):
        return None
    return pdfa if len(pdfa) == 2 or pdfa[0] == "4" else pdfa + "b"


def marca_pdfa(ruta):
    """La parte y el nivel PDF/A que declara un fichero, o "".

    Se lee el XMP VIGENTE, el del catálogo de la última revisión: en un guardado
    anexado (un firmado) el XMP anterior sigue en los bytes del fichero, y
    buscarlo a pelo encontraba la marca que ya se había quitado."""
    try:
        import pypdf
        meta = pypdf.PdfReader(ruta).trailer["/Root"].get("/Metadata")
        datos = meta.get_object().get_data() if meta is not None else b""
    except Exception:  # noqa: BLE001 — un fichero que pypdf no lee: a pelo
        try:
            datos = open(ruta, "rb").read()
        except OSError:
            return ""
    pa = re.search(rb"pdfaid:part\s*(?:=\s*[\"']|>)\s*(\d)", datos)
    if not pa:
        return ""
    conf = re.search(rb"pdfaid:conformance\s*(?:=\s*[\"']|>)\s*([A-Za-z])", datos)
    return pa.group(1).decode() + (conf.group(1).decode().lower() if conf else "")


def verapdf(filas, herr):
    """PDF/A: lo que declara la entrada y lo que queda de ello en cada salida.

    veraPDF va al final y por lotes: arrancar la JVM por fichero costaba más que
    validar. Las salidas se validan contra la parte que declaraba la ENTRADA,
    no contra la que declaren ellas: si una operación borra el XMP, la salida
    deja de decir que es PDF/A, y eso es justo lo que hay que contar.
    """
    vp = next((p for p in (os.path.join(herr, "verapdf/inst/verapdf"), os.path.join(herr, "verapdf/verapdf"),
                           "/srv/agentes/verapdf/inst/verapdf") if os.path.exists(p)), None)
    if not vp:
        print("sin veraPDF: no se mide PDF/A", flush=True)
        return
    por_sabor = {}
    for f in filas:
        sabor = sabor_veraPDF(f["clase"].get("pdfa"))
        if not sabor:
            continue
        ruta = f["ruta"]
        por_sabor.setdefault(sabor, []).append((f, None, ruta))
        for op, s in f["salidas"].items():
            # La salida que conserva la marca se valida contra lo que DECLARA
            # (censurar baja un nivel «a» a «b»); la que la quitó, contra la
            # entrada, para contar lo que se perdió.
            marca = marca_pdfa(s["ruta"])
            s["pdfa_marca"] = marca
            por_sabor.setdefault(sabor_veraPDF(marca) or sabor, []).append((f, op, s["ruta"]))
    for sabor, lista in por_sabor.items():
        for i in range(0, len(lista), 100):
            lote = lista[i:i + 100]
            rc, out, _ = run([vp, "--flavour", sabor, "--format", "text"] + [r for _, _, r in lote], timeout=1800)
            veredicto = {}
            for l in out.splitlines():
                m = re.match(r"(PASS|FAIL)\s+(.+?)\s+\S+\s*$", l)
                if m:
                    veredicto[m.group(2)] = m.group(1)
            for f, op, r in lote:
                v = veredicto.get(r, "error")
                if op is None:
                    f["pdfa_entrada"] = v
                else:
                    f["salidas"][op]["pdfa"] = v


MAX_MEMORIA_PY = 3 << 30


def _con_tope():
    """Tope de memoria (blando) del medidor: Python, Poppler y qpdf lo heredan."""
    resource.setrlimit(resource.RLIMIT_AS, (MAX_MEMORIA_PY, resource.RLIM_INFINITY))


def _sin_tope():
    """El binario de Go reserva espacio de direcciones de sobra y tiene su propio
    vigilante (SYTUE_COMPAT_MAX_MIB): se le devuelve el límite."""
    resource.setrlimit(resource.RLIMIT_AS, (resource.RLIM_INFINITY, resource.RLIM_INFINITY))


def medir_aislado(tarea, comunes):
    ruta, origen, rel = tarea
    ident = re.sub(r"[^A-Za-z0-9._-]", "_", f"{origen}__{rel}")[-150:]
    dirsal = os.path.join(H["salida"], "ficheros", ident)
    final = os.path.join(dirsal, "medida.json")
    if os.path.exists(final) and not H["rehacer"]:
        with open(final) as f:
            return json.load(f)
    args = [sys.executable, "-I", os.path.abspath(__file__), *comunes, "--uno", ruta, origen, rel]
    if H["rehacer"]:
        args.append("--rehacer")
    try:
        p = subprocess.run(args, capture_output=True, timeout=H["limite"] * 3, preexec_fn=_con_tope)
        motivo = f"rc {p.returncode}: {p.stderr.decode('utf-8', 'replace')[-300:]}"
    except subprocess.TimeoutExpired:
        motivo = "tiempo agotado"
    if os.path.exists(final):
        with open(final) as f:
            return json.load(f)
    # El medidor murió (memoria o tiempo): se guarda lo que CertiPDF sí dejó.
    res = {}
    try:
        with open(os.path.join(dirsal, "resultado.json")) as f:
            res = json.load(f)
    except Exception:  # noqa: BLE001
        pass
    fila = {"id": ident, "origen": origen, "fichero": rel, "ruta": ruta, "dir": dirsal, "medidor_fallo": motivo,
            "clase": {"productor": "?", "version": "?", "fuentes": [], "imagenes": [], "formulario": "none"},
            "go_rc": 0 if res.get("terminado") else -1, "ops": res.get("ops", {}), "pintado": {}, "salidas": {}}
    with open(final, "w") as f:
        json.dump(fila, f, ensure_ascii=False)
    return fila


def tareas(corpus, solo):
    for origen in sorted(os.listdir(corpus)):
        base = os.path.join(corpus, origen)
        for raiz, _, ficheros in os.walk(base):
            if "/.git" in raiz:
                continue
            for f in sorted(ficheros):
                if not f.lower().endswith(".pdf"):
                    continue
                ruta = os.path.join(raiz, f)
                rel = os.path.relpath(ruta, base)
                if solo and not re.search(solo, f"{origen}/{rel}"):
                    continue
                yield ruta, origen, rel


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--corpus", required=True)
    ap.add_argument("--salida", required=True)
    ap.add_argument("--binario", required=True)
    ap.add_argument("--herramientas", required=True)
    ap.add_argument("--hilos", type=int, default=3)
    ap.add_argument("--solo", default="")
    ap.add_argument("--limite", type=int, default=240)
    ap.add_argument("--rehacer", action="store_true")
    ap.add_argument("--sin-verapdf", action="store_true", help="para medir por tandas: veraPDF en la última")
    ap.add_argument("--uno", nargs=3, metavar=("RUTA", "ORIGEN", "REL"), help=argparse.SUPPRESS)
    a = ap.parse_args()
    a.corpus, a.salida, a.binario = map(os.path.abspath, (a.corpus, a.salida, a.binario))
    herr = os.path.abspath(a.herramientas)
    lib = os.path.join(herr, "qpdf/usr/lib/aarch64-linux-gnu")
    H.update({
        "pdftoppm": "pdftoppm", "pdftotext": "pdftotext", "pdfinfo": "pdfinfo", "pdffonts": "pdffonts",
        "pdfimages": "pdfimages", "pdfsig": "pdfsig", "qpdf": os.path.join(herr, "qpdf/usr/bin/qpdf"),
        "env": dict(os.environ) | {"LD_LIBRARY_PATH": lib, "LC_ALL": "C.UTF-8"},
        "salida": a.salida, "binario": a.binario, "limite": a.limite, "rehacer": a.rehacer,
        "cert": os.path.join(herr, "pki/cadena.pem"), "key": os.path.join(herr, "pki/clave.pem"),
        "raiz": os.path.join(herr, "pki/raiz.pem"),
    })
    pki(os.path.join(herr, "pki"))
    claves = os.path.join(herr, "claves.json")
    if os.path.exists(claves):
        with open(claves) as f:
            CLAVES.update(json.load(f))
    if a.uno:
        logging.disable(logging.CRITICAL)  # pyHanko cuenta cada SubFilter que no entiende
        medir_fichero(tuple(a.uno))
        return
    os.makedirs(os.path.join(a.salida, "ficheros"), exist_ok=True)
    lista = list(tareas(a.corpus, a.solo))
    print(f"{len(lista)} ficheros", flush=True)
    t0 = time.time()
    filas = []
    # Un PROCESO por fichero, con tope de memoria y de tiempo. Con un Pool, un
    # fichero de qpdf hizo que pypdf pidiera 14 GB: el núcleo mató al trabajador
    # —y la memoria era la del contenedor de TODOS los agentes— y el Pool se
    # quedó esperando para siempre a un proceso que ya no existía.
    comunes = [a2 for a2 in sys.argv[1:] if a2 not in ("--rehacer",)]
    with concurrent.futures.ThreadPoolExecutor(a.hilos) as ex:
        futuros = [ex.submit(medir_aislado, t, comunes) for t in lista]
        for i, fut in enumerate(concurrent.futures.as_completed(futuros), 1):
            filas.append(fut.result())
            if i % 50 == 0:
                print(f"{i}/{len(lista)} {time.time() - t0:.0f}s", flush=True)
    if not a.sin_verapdf:
        verapdf(filas, herr)
    with open(os.path.join(a.salida, "resultados.jsonl"), "w") as salida:
        for fila in sorted(filas, key=lambda f: f["id"]):
            salida.write(json.dumps(fila, ensure_ascii=False) + "\n")
    print(f"hecho en {time.time() - t0:.0f}s", flush=True)


if __name__ == "__main__":
    main()
