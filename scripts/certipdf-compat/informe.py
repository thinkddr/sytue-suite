# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
"""Tablas de la medida de compatibilidad: por función y por tipo de fichero.

Uso: informe.py antes.jsonl [después.jsonl] [--capturas DIR] > informe.md

Con dos ficheros pone las dos cifras juntas («antes → después»). Con
--capturas deja en DIR la referencia, lo nuestro y la diferencia de las páginas
peor pintadas (las PNG que medir.py conserva).
"""
import argparse
import json
import os
import statistics
from collections import Counter, defaultdict

BIEN_PINTADO = ("igual", "leve")


def cargar(ruta):
    with open(ruta) as f:
        return [json.loads(l) for l in f]


def op(f, nombre):
    return f["ops"].get(nombre) or {}


def ok_salida(s):
    """Una salida vale si qpdf no ve errores y los otros dos lectores la abren.

    De Poppler cuentan los errores NUEVOS (poppler.py) cuando se han medido: los
    que ya daba la entrada no son de CertiPDF."""
    poppler = s["poppler"] == "ok" if "poppler_nuevos" not in s else not s["poppler_nuevos"]
    return s["qpdf"]["clase"] in ("ok", "avisos") and s["pypdf"] == "ok" and poppler


# --- Tipos de fichero ---------------------------------------------------------


def tipos(f):
    c, e = f["clase"], f.get("estructura_entrada", {})
    t = [f"origen: {f['origen']}", f"productor: {c['productor']}"]
    v = c.get("version", "?")
    t.append(f"versión: {'≤1.3' if v in ('1.0', '1.1', '1.2', '1.3') else v}")
    if c.get("linealizado"):
        t.append("estructura: linealizado")
    if c.get("incremental"):
        t.append("estructura: actualizaciones incrementales")
    if c.get("xref_flujo"):
        t.append("estructura: referencias en flujo")
    if c.get("danado") or c.get("qpdf_entrada") == "errores":
        t.append("estructura: dañado")
    if c.get("cifrado", "no") != "no":
        t.append(f"cifrado: {c['cifrado']}{' con clave de usuario' if c.get('clave_usuario') else ''}")
    for fu in c.get("fuentes", []):
        t.append(f"fuente: {fu}")
    for im in c.get("imagenes", []):
        if im:
            t.append(f"imagen: {im}")
    if c.get("escaneado"):
        t.append("contenido: escaneado")
    if c.get("formulario", "none") != "none":
        t.append(f"formulario: {c['formulario']}")
    if e.get("anotaciones_p1", 0) > 0:
        t.append("contenido: anotaciones")
    if e.get("capas"):
        t.append("contenido: capas")
    if c.get("pdfa"):
        t.append(f"norma: PDF/A-{c['pdfa']}")
    if c.get("pdfua"):
        t.append("norma: PDF/UA")
    if c.get("etiquetado"):
        t.append("norma: etiquetado")
    if c.get("rtl"):
        t.append("texto: derecha a izquierda")
    if c.get("vertical"):
        t.append("texto: vertical")
    if c.get("firmas"):
        t.append(f"firma: {c.get('nivel_firma', '?')}")
    return t


# --- Métricas por fichero ----------------------------------------------------
# Cada una devuelve None si no aplica, o (aciertos, total).


def m_abrir(f):
    return (1 if op(f, "abrir").get("ok") else 0), 1


def m_abrir_otros(f):
    """Abre donde Poppler también abre: lo que de verdad se le reprocha."""
    if not f["clase"].get("poppler_abre"):
        return None
    return m_abrir(f)


def m_panico(f):
    malos = sum(1 for o in f["ops"].values() if o.get("panico")) + (1 if f["go_rc"] not in (0, 1) and "panic" in f.get("go_fallo", "") else 0)
    return (0 if malos else 1), 1


# Imágenes JBIG2 de pdf.js que Poppler 22.12 pinta mal. Todas las bitmap-*
# codifican la misma imagen de 399×400, que Poppler y CertiPDF sacan idéntica de
# bitmap-symbol.pdf; en estas CertiPDF da esa imagen píxel a píxel y Poppler no
# (prueba: herramientas/jbig2-cruce.py en el frontal, ver #corpus-certipdf-jbig2).
# jbig2_file_header.pdf NO está: Poppler la pinta en blanco con «Unknown segment
# type», pero no hay un segundo decodificador que diga que la nuestra es la buena.
_EN_BLANCO = "Poppler la deja en blanco (tinta 0 %); CertiPDF da la imagen común"
PINTADO_FALLA_POPPLER = {
    "pdfjs/test/pdfs/bitmap-halftone-skip-dummy.pdf": _EN_BLANCO,
    "pdfjs/test/pdfs/bitmap-halftone-skip-grid.pdf": _EN_BLANCO,
    "pdfjs/test/pdfs/bitmap-halftone-skip-grid-template1.pdf": _EN_BLANCO,
    "pdfjs/test/pdfs/bitmap-halftone-skip-grid-template2.pdf": _EN_BLANCO,
    "pdfjs/test/pdfs/bitmap-halftone-skip-grid-template3.pdf": _EN_BLANCO,
    "pdfjs/test/pdfs/bitmap-symbol-context-reuse.pdf": _EN_BLANCO,
    "pdfjs/test/pdfs/bitmap-symbol-symhuffB5B3-texthuffB7B9B12.pdf": _EN_BLANCO,
    "pdfjs/test/pdfs/bitmap-symbol-symhuffuncompressed-texthuff.pdf": _EN_BLANCO,
    "pdfjs/test/pdfs/bitmap-refine-tpgron.pdf": "Poppler difiere un 7,91 % de la imagen común; CertiPDF, 0 %",
    "pdfjs/test/pdfs/bitmap-refine-template1-tpgron.pdf": "Poppler difiere un 7,70 % de la imagen común; CertiPDF, 0 %",
}


def m_pintar(f):
    p = f.get("pintado") or {}
    if not p:
        if f.get("ref_paginas_pintadas") and not op(f, "abrir").get("ok"):
            return 0, f["ref_paginas_pintadas"]
        return None
    if f"{f['origen']}/{f['fichero']}" in PINTADO_FALLA_POPPLER:
        return len(p), len(p)
    bien = sum(1 for d in p.values() if d.get("clase") in BIEN_PINTADO)
    # Las páginas que Poppler pintó y nosotros no, cuentan como mal pintadas.
    return bien, max(len(p), f.get("ref_paginas_pintadas", 0))


# Donde los tres lectores fallan, que dos coincidan no prueba nada: se excluye
# a mano, con el motivo, tras revisar la lista que imprime el informe.
SIN_DESEMPATE = {
    "pdfjs/test/pdfs/XiaoBiaoSong.pdf": "chino en GBK con una fuente simple WinAnsi: los tres lectores dan basura",
}


def falla_la_referencia(t, f=None):
    """Acierto donde la referencia falla (criterio del dueño, 2026-10-16): un
    tercer lector independiente, pypdf, lee lo mismo que CertiPDF (≥ 95 % de sus
    palabras) y no lo mismo que Poppler (< 95 %). Dos lectores de tres coinciden y
    Poppler es el que se equivoca: el fichero cuenta a favor. Si pypdf no saca
    texto suficiente (menos de 5 palabras) no hay desempate."""
    if f is not None and f"{f['origen']}/{f['fichero']}" in SIN_DESEMPATE:
        return False
    p = (t or {}).get("pypdf") or {}
    return (p.get("palabras") or 0) >= 5 and (p.get("nuestro") or 0) >= 0.95 and (p.get("poppler") or 0) < 0.95


def m_texto(f):
    t = f.get("texto")
    if not t:
        return None
    return (1 if t["recall"] >= 0.95 or falla_la_referencia(t, f) else 0), 1


def m_buscar(f):
    t = f.get("texto")
    if not t or t.get("buscar") is None:
        return None
    return (1 if t["buscar"] >= 0.95 or falla_la_referencia(t, f) else 0), 1


def m_formulario(f):
    o = op(f, "formulario")
    if not o or not (o.get("datos") or {}).get("rellenados"):
        return None
    s = f["salidas"].get("formulario")
    bien = o.get("ok") and s and ok_salida(s) and s.get("campos_pypdf", {}).get("bien") == s.get("campos_pypdf", {}).get("total")
    return (1 if bien else 0), 1


def m_anotar(f):
    o = op(f, "anotar")
    if not o:
        return None
    s = f["salidas"].get("anotado")
    bien = o.get("ok") and s and ok_salida(s)
    return (1 if bien else 0), 1


def m_censurar(f):
    o = op(f, "censurar")
    if not o:
        return None
    # La franja se mide en la imagen pintada, que en una página girada 90/270 va
    # de lado: ahí la comparación no sabe dónde está la zona y se deja fuera.
    if (op(f, "pintar-1").get("datos") or {}).get("rotacion") in (90, 270):
        return None
    s = f["salidas"].get("censurado")
    bien = o.get("ok") and s and ok_salida(s) and not s.get("fuga_franja") and \
        (s.get("pintado_fuera_de_franja") or {}).get("clase", "igual") in BIEN_PINTADO
    return (1 if bien else 0), 1


def m_editar(f):
    o = op(f, "editar")
    if not o or not ((o.get("datos") or {}).get("fragmentos") or o.get("error") or o.get("panico")):
        return None
    # Sin un trozo de texto legible de cuatro letras o más, la sonda no intenta
    # editar nada: no hay medida, no un fallo (antes contaba como «sin salida»).
    if o.get("ok") and "nuevo" not in (o.get("datos") or {}):
        return None
    s = f["salidas"].get("editado")
    bien = o.get("ok") and s and ok_salida(s)
    return (1 if bien else 0), 1


def m_paginas(f):
    o = op(f, "paginas")
    if not o:
        return None
    s = f["salidas"].get("rotado")
    bien = o.get("ok") and s and ok_salida(s) and (s.get("pintado_vs_entrada") or {}).get("clase", "igual") in BIEN_PINTADO
    return (1 if bien else 0), 1


def heredado(s, lector):
    """El lector ya fallaba con la ENTRADA y la salida es la entrada intacta más
    lo anexado: su queja es de la entrada, no de CertiPDF (medir.py la guarda en
    `entrada` solo para firmado, que siempre anexa)."""
    e = (s.get("entrada") or {}).get(lector)
    if not s.get("prefijo_intacto") or e is None:
        return False
    if lector == "pypdf":
        return e != "ok" and e.split(":")[0] == str(s.get("pypdf", "")).split(":")[0]
    if lector == "qpdf":
        return e not in ("ok", "avisos")
    return bool(e) and all("error" in x for x in e)  # pyhanko no sabe leer la entrada


def m_firmar(f):
    o = op(f, "firmar")
    if not o:
        return None
    s = f["salidas"].get("firmado")
    if not (o.get("ok") and s):
        return 0, 1
    poppler = s["poppler"] == "ok" if "poppler_nuevos" not in s else not s["poppler_nuevos"]
    valida = (s["qpdf"]["clase"] in ("ok", "avisos") or heredado(s, "qpdf")) and \
        (s["pypdf"] == "ok" or heredado(s, "pypdf")) and poppler
    if not valida:
        return 0, 1
    pdfsig = "Signature is Valid." in (s.get("pdfsig") or [])
    ph = [x for x in s.get("pyhanko", []) if x.get("campo", "").startswith("Sytue")]
    if not ph and heredado(s, "pyhanko"):
        # pyHanko no lee la entrada (cabecera ilegal, xref rota…) y por eso no ve
        # la firma: queda la otra referencia, pdfsig.
        return (1 if pdfsig else 0), 1
    bien = ph and ph[-1].get("intacta") and ph[-1].get("valida") and pdfsig
    return (1 if bien else 0), 1


def m_firmar_intacto(f):
    """La primera revisión sigue byte a byte (solo si se firmó anexando)."""
    o = op(f, "firmar")
    s = f["salidas"].get("firmado")
    if not o.get("ok") or not s:
        return None
    return (1 if s.get("prefijo_intacto") else 0), 1


def m_firmar_conserva_firmas(f):
    """Las firmas que ya traía siguen intactas tras firmar otra vez."""
    s = f["salidas"].get("firmado")
    if not s or not f["clase"].get("firmas"):
        return None
    previas = [x for x in s.get("pyhanko", []) if not x.get("campo", "").startswith("Sytue")]
    antes = [x for x in f.get("firmas_entrada_pyhanko", []) if x.get("intacta")]
    if not antes:
        return None
    return (1 if sum(1 for x in previas if x.get("intacta")) >= len(antes) else 0), 1


def prohibido_por_certificacion(f, nombre):
    """Una firma de certificación (DocMDP P1 o P2) prohíbe el cambio: no se
    puede anexar sin romperla, lo dice la norma. Es imposible y CertiPDF lo
    explica y pide confirmación (409 pdf_invalidates_signatures); no cuenta."""
    pide = ((op(f, nombre).get("datos") or {}).get("pide_confirmacion") or "")
    return "certificación" in pide


def m_cambiar_firmado(f):
    """Anotar y rellenar un documento firmado: las firmas que validaban siguen
    validando (pyHanko) y lo firmado sigue byte a byte."""
    antes = [x for x in f.get("firmas_entrada_pyhanko", []) if x.get("intacta") and x.get("valida")]
    if not antes:
        return None
    bien = total = 0
    for op, nombre in (("anotado", "anotar"), ("formulario", "formulario")):
        s = f["salidas"].get(op)
        if not s or prohibido_por_certificacion(f, nombre):
            continue
        total += 1
        despues = {x.get("campo"): x for x in s.get("pyhanko", [])}
        bien += bool(s.get("prefijo_intacto")) and all(
            despues.get(x["campo"], {}).get("intacta") and despues.get(x["campo"], {}).get("valida") for x in antes)
    return (bien, total) if total else None


def m_verificar(f):
    """Coincide con pyHanko en si cada firma está íntegra (las que pyHanko entiende)."""
    ph = [x for x in f.get("firmas_entrada_pyhanko", []) if "intacta" in x]
    nuestras = (op(f, "verificar").get("datos") or {}).get("firmas") or []
    if not ph:
        return None
    if not op(f, "verificar").get("ok"):
        return 0, len(ph)
    por_campo = {n.get("campo"): n for n in nuestras}
    bien = 0
    for x in ph:
        n = por_campo.get(x["campo"])
        bien += bool(n) and bool(n.get("integra")) == bool(x["intacta"] and x["valida"])
    return bien, len(ph)


def m_salidas_validas(f):
    s = f["salidas"]
    if not s:
        return None
    return sum(1 for x in s.values() if ok_salida(x)), len(s)


# Lo que CertiPDF quita A PROPÓSITO y por eso no cuenta como pérdida:
#   - el XMP y el /Info, siempre (pdfwrite.go, claveQueNoViaja);
#   - al censurar, todo lo del catálogo que puede repetir lo tapado: estructura,
#     índice, /Names, formulario, capas y firmas (pdfredact.go, catalogoSinTexto);
#   - el /DSS al reescribir un firmado: sus datos de validación hablan de firmas
#     que el documento nuevo ya no tiene válidas (catalogoQueNoViaja).
QUITA_SIEMPRE = {"xmp"}
QUITA_CENSURAR = {"estructura", "marcadores", "campos", "firmas", "adjuntos", "destinos", "capas", "dss"}


def deliberado(op, p):
    return p in QUITA_SIEMPRE or (op == "censurado" and p in QUITA_CENSURAR) or (op != "firmado" and p == "dss")


def m_conserva(f):
    """Salidas que no pierden nada de lo que tenía la entrada, salvo lo que se
    quita a propósito (QUITA_SIEMPRE, QUITA_CENSURAR)."""
    s = f["salidas"]
    if not s:
        return None
    return sum(1 for op, x in s.items() if not [p for p in x["pierde"] if not deliberado(op, p)]), len(s)


def m_conserva_estricto(f):
    """Lo mismo contando como pérdida todo, menos el XMP y el /Info."""
    s = f["salidas"]
    if not s:
        return None
    return sum(1 for x in s.values() if not [p for p in x["pierde"] if p != "xmp"]), len(s)


def m_pdfa(f):
    if f.get("pdfa_entrada") != "PASS":
        return None
    s = [x for x in f["salidas"].values() if "pdfa" in x]
    return sum(1 for x in s if x["pdfa"] == "PASS"), len(s)


def m_pdfa_honesto(f):
    """Decisión del dueño (2026-10-15): la marca se conserva solo si la salida
    cumple, y si no, se quita y se avisa. Honesta = cumple lo que declara, o ya
    no declara nada (y la interfaz lo avisó)."""
    if f.get("pdfa_entrada") != "PASS":
        return None
    s = [x for x in f["salidas"].values() if "pdfa" in x and "pdfa_marca" in x]
    return sum(1 for x in s if x["pdfa"] == "PASS" or not x["pdfa_marca"]), len(s)


METRICAS = [
    ("Abrir", m_abrir), ("Abrir (donde Poppler abre)", m_abrir_otros), ("Sin pánico", m_panico),
    ("Pintar (páginas)", m_pintar), ("Extraer texto (≥95 %)", m_texto), ("Buscar (≥95 %)", m_buscar),
    ("Rellenar formulario", m_formulario), ("Anotar", m_anotar), ("Censurar", m_censurar), ("Editar texto", m_editar),
    ("Girar páginas", m_paginas), ("Firmar (validada por pyHanko y Poppler)", m_firmar),
    ("Firmar: 1.ª revisión intacta", m_firmar_intacto), ("Firmar: conserva firmas previas", m_firmar_conserva_firmas),
    ("Anotar o rellenar un firmado sin romper la firma", m_cambiar_firmado),
    ("Verificar (coincide con pyHanko)", m_verificar), ("Salidas válidas (qpdf+pypdf+Poppler)", m_salidas_validas),
    ("Salidas sin pérdidas", m_conserva), ("Salidas sin pérdidas, contando lo que censurar quita a propósito", m_conserva_estricto),
    ("Salidas que siguen siendo PDF/A", m_pdfa), ("Salidas PDF/A honestas (cumplen lo que declaran)", m_pdfa_honesto),
]


def agregar(filas, metrica):
    a = t = 0
    for f in filas:
        r = metrica(f)
        if r:
            a += r[0]
            t += r[1]
    return a, t


def pct(a, t):
    return f"{100 * a / t:.1f} % ({a}/{t})" if t else "—"


def celda(antes, despues, metrica):
    a = agregar(antes, metrica)
    if despues is None:
        return pct(*a)
    d = agregar(despues, metrica)
    return f"{pct(*a)} → {pct(*d)}"


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("antes")
    ap.add_argument("despues", nargs="?")
    ap.add_argument("--capturas")
    ap.add_argument("--min-ficheros", type=int, default=5)
    a = ap.parse_args()
    antes = cargar(a.antes)
    despues = cargar(a.despues) if a.despues else None
    if despues is not None:
        # Mismo conjunto en las dos columnas: solo lo que se midió las dos veces.
        ids = {f["id"] for f in antes} & {f["id"] for f in despues}
        antes = [f for f in antes if f["id"] in ids]
        despues = [f for f in despues if f["id"] in ids]

    print(f"# Compatibilidad de CertiPDF — {len(antes)} ficheros\n")
    print("## Por función\n")
    print("| Función | Resultado |\n|---|---|")
    for nombre, m in METRICAS:
        print(f"| {nombre} | {celda(antes, despues, m)} |")

    favor = [f for f in (despues or antes) if (f.get("texto") or {}).get("recall", 1) < 0.95 and falla_la_referencia(f["texto"], f)]
    if favor:
        print("\n## Texto: aciertos donde Poppler falla (pypdf de desempate)\n")
        print("| Fichero | CertiPDF frente a Poppler | pypdf en CertiPDF | pypdf en Poppler |\n|---|---|---|---|")
        for f in sorted(favor, key=lambda f: f["fichero"]):
            t = f["texto"]
            print(f"| {f['origen']}/{f['fichero']} | {t['recall']:.2f} | {t['pypdf']['nuestro']:.2f} | {t['pypdf']['poppler']:.2f} |")

    print("\n## Rendimiento al abrir\n")
    for etiqueta, conj in [("antes", antes), ("después", despues)]:
        if conj is None:
            continue
        ms = sorted(op(f, "abrir").get("ms", 0) for f in conj if op(f, "abrir"))
        rss = sorted(max((o.get("rss_mib", 0) for o in f["ops"].values()), default=0) for f in conj)
        fallos = Counter()
        for f in conj:
            if f.get("medidor_fallo"):
                fallos["el medidor (no CertiPDF) se pasó de memoria o tiempo"] += 1
            if f["go_rc"] == -9:
                fallos["tiempo agotado"] += 1
            elif f["go_rc"] == 3:
                fallos["memoria por encima del tope"] += 1
            elif f["go_rc"] not in (0,):
                fallos[f"proceso muerto (rc {f['go_rc']})"] += 1
        if ms:
            print(f"- {etiqueta}: abrir p50 {statistics.median(ms):.1f} ms, p95 {ms[int(len(ms) * .95)]:.0f} ms, máx {ms[-1]:.0f} ms;"
                  f" pico de memoria p95 {rss[int(len(rss) * .95)]:.0f} MiB, máx {rss[-1]:.0f} MiB; {dict(fallos) or 'sin muertes'}")

    print("\n## Pintado: clase de cada página\n")
    for etiqueta, conj in [("antes", antes), ("después", despues)]:
        if conj is None:
            continue
        c = Counter(d.get("clase") for f in conj for d in (f.get("pintado") or {}).values())
        print(f"- {etiqueta}: " + ", ".join(f"{k} {v}" for k, v in c.most_common()))

    print("\n## Lo que se pierde al guardar (salidas que lo pierden / salidas cuya entrada lo tenía)\n")
    print("| Elemento | " + ("Antes | Después |" if despues else "Salidas |") + "\n|---|---|" + ("---|" if despues else ""))
    elementos = sorted({p for f in antes for s in f["salidas"].values() for p in s["pierde"]})
    for el in elementos:
        def cuenta(conj):
            n = sum(1 for f in conj for s in f["salidas"].values() if el in s["pierde"])
            tot = sum(len(f["salidas"]) for f in conj if (f.get("estructura_entrada") or {}).get(el) not in (None, False, 0))
            return f"{n}/{tot}"
        print(f"| {el} | {cuenta(antes)} |" + (f" {cuenta(despues)} |" if despues else ""))

    print("\n## Por tipo de fichero\n")
    grupos = defaultdict(list)
    for f in antes:
        for t in tipos(f):
            grupos[t].append(f["id"])
    por_id_d = {f["id"]: f for f in despues} if despues else None
    por_id_a = {f["id"]: f for f in antes}
    cols = [("Abrir", m_abrir), ("Pintar", m_pintar), ("Texto", m_texto), ("Buscar", m_buscar),
            ("Formulario", m_formulario), ("Anotar", m_anotar), ("Censurar", m_censurar), ("Firmar", m_firmar),
            ("Verificar", m_verificar), ("Salidas válidas", m_salidas_validas), ("Sin pérdidas", m_conserva)]
    print("| Tipo | Ficheros | " + " | ".join(c for c, _ in cols) + " |")
    print("|---|---|" + "---|" * len(cols))
    for g in sorted(grupos):
        ids = grupos[g]
        if len(ids) < a.min_ficheros and not g.startswith(("firma", "cifrado", "imagen", "texto", "norma", "estructura", "formulario")):
            continue
        fa = [por_id_a[i] for i in ids]
        fd = [por_id_d[i] for i in ids] if por_id_d else None
        celdas = []
        for _, m in cols:
            x = agregar(fa, m)
            s = f"{100 * x[0] / x[1]:.0f} %" if x[1] else "—"
            if fd is not None:
                y = agregar(fd, m)
                s += f" → {100 * y[0] / y[1]:.0f} %" if y[1] else " → —"
            celdas.append(s)
        print(f"| {g} | {len(ids)} | " + " | ".join(celdas) + " |")

    conj = despues if despues is not None else antes
    print("\n## Fallos más frecuentes\n")
    errores = Counter()
    for f in conj:
        for n, o in f["ops"].items():
            msg = o.get("panico") or o.get("error")
            if msg:
                clave = n.split("-")[0] + ": " + (("PÁNICO " + msg.split("|")[0]) if o.get("panico") else msg)[:110]
                errores[clave] += 1
        if f["go_rc"] not in (0,) and f.get("go_fallo"):
            errores["proceso: " + f["go_fallo"][:110]] += 1
    for k, v in errores.most_common(40):
        print(f"- {v} × {k}")

    if a.capturas:
        capturas(conj, a.capturas)


def capturas(filas, dest):
    """Referencia | CertiPDF | diferencia, de las páginas peor pintadas."""
    import numpy as np
    from PIL import Image

    os.makedirs(dest, exist_ok=True)
    peores = sorted(((d.get("bloques", 0), f, n) for f in filas for n, d in (f.get("pintado") or {}).items()
                     if d.get("clase") not in BIEN_PINTADO), key=lambda x: -x[0])
    hechas = 0
    print("\n## Capturas\n")
    for b, f, n in peores:
        base = f.get("dir")
        if base is None:
            continue
        ref = next((os.path.join(base, x) for x in os.listdir(base) if x.startswith("ref-") and x.endswith(".png")
                    and int(x[4:-4]) == int(n)), None)
        nue = os.path.join(base, f"nuestro-{n}.png")
        if not ref or not os.path.exists(nue):
            continue
        a = np.asarray(Image.open(ref).convert("RGB"), dtype=np.int16)
        c = np.asarray(Image.open(nue).convert("RGB"), dtype=np.int16)
        h, w = min(a.shape[0], c.shape[0]), min(a.shape[1], c.shape[1])
        dif = 255 - np.abs(a[:h, :w] - c[:h, :w]).max(axis=2).astype(np.uint8)
        lienzo = np.full((h, w * 3 + 20, 3), 128, np.uint8)
        lienzo[:, :w] = a[:h, :w]
        lienzo[:, w + 10:2 * w + 10] = c[:h, :w]
        lienzo[:, 2 * w + 20:] = np.stack([dif] * 3, axis=2)
        nombre = f"{hechas + 1:02d}-{f['id'][:60]}-p{n}.png"
        Image.fromarray(lienzo).save(os.path.join(dest, nombre))
        print(f"- `{nombre}`: {f['origen']}/{f['fichero']} página {n}, {b:.0f} % de bloques distintos ({f['clase']['productor']})")
        hechas += 1
        if hechas == 12:
            break


if __name__ == "__main__":
    main()
