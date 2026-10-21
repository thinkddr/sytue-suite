"""Descarga PDF públicos de administraciones de la UE (sin sesión) al directorio dado.

Uso: python3 -I ue.py <destino>
Solo guarda lo que empieza por %PDF. Imprime una línea por fichero.
"""
import json, os, re, sys, urllib.request

dest = sys.argv[1]
os.makedirs(dest, exist_ok=True)
UA = {"User-Agent": "Mozilla/5.0 (X11; Linux aarch64) sytue-certipdf-corpus/1.0", "Accept": "*/*"}


def get(url, accept=None):
    h = dict(UA)
    if accept:
        h["Accept"] = accept
    with urllib.request.urlopen(urllib.request.Request(url, headers=h), timeout=60) as r:
        return r.read()


def guardar(nombre, url):
    ruta = os.path.join(dest, nombre)
    if os.path.exists(ruta):
        return
    try:
        b = get(url)
    except Exception as e:  # noqa: BLE001
        print(f"FALLO {nombre}: {e}")
        return
    if not b.startswith(b"%PDF"):
        print(f"NO-PDF {nombre}: {b[:60]!r}")
        return
    with open(ruta, "wb") as f:
        f.write(b)
    print(f"ok {nombre} {len(b)}")


FIJOS = {
    "aeat-modelo-145.pdf": "https://www.agenciatributaria.es/static_files/Sede/Procedimiento_ayuda/G603/mod145_es_es.pdf",
    "ejustice-small-claims-a.pdf": "https://online-forms.e-justice.europa.eu/download-blank-pdf/sc_a_en",
    "euskadi-gauzatu3.pdf": "https://www.euskadi.eus/contenidos/ayuda_subvencion/1992/es_5007/adjuntos/09gauzatu3.pdf",
    "euskadi-gauzatu2.pdf": "https://euskadi.eus/gobierno-vasco/contenidos/ayuda_subvencion/1611/es_4533/adjuntos/gauzatu2.pdf",
    "de-kindergeld-kg1.pdf": "https://www.arbeitsagentur.de/datei/kg1-antrag-kindergeld_ba034882.pdf",
    "consilium-st-11529-2022.pdf": "https://data.consilium.europa.eu/doc/document/ST-11529-2022-INIT/en/pdf",
    "eurlex-oj-l-2024-1689.pdf": "https://eur-lex.europa.eu/legal-content/ES/TXT/PDF/?uri=OJ:L_202401689",
    "eurlex-32016R0679.pdf": "https://eur-lex.europa.eu/legal-content/ES/TXT/PDF/?uri=CELEX:32016R0679",
    "eurlex-32014R0910.pdf": "https://eur-lex.europa.eu/legal-content/ES/TXT/PDF/?uri=CELEX:32014R0910",
}
for n, u in FIJOS.items():
    guardar(n, u)

# FNMT: los PDF enlazados desde la página de declaraciones de prácticas.
try:
    html = get("https://www.sede.fnmt.gob.es/es/normativa/declaracion-de-practicas-de-certificacion").decode("utf-8", "replace")
    enlaces = sorted(set(re.findall(r'href="([^"]+?\.pdf[^"]*)"', html)))[:4]
    for i, e in enumerate(enlaces):
        if e.startswith("/"):
            e = "https://www.sede.fnmt.gob.es" + e
        guardar(f"fnmt-dpc-{i}.pdf", e)
except Exception as e:  # noqa: BLE001
    print(f"FALLO fnmt: {e}")

# BOE: sumario y unas disposiciones de varias épocas (firmas con certificados ya caducados).
for fecha in ["20090105", "20110103", "20130102", "20150105", "20170103", "20190102", "20210104", "20230103", "20250103", "20261009"]:
    try:
        s = json.loads(get(f"https://www.boe.es/datosabiertos/api/boe/sumario/{fecha}", "application/json"))
    except Exception as e:  # noqa: BLE001
        print(f"FALLO boe {fecha}: {e}")
        continue
    urls = re.findall(r'"url_pdf":\s*\{[^}]*?"texto":\s*"([^"]+)"', json.dumps(s))
    elegidos = urls[:1] + urls[1:3] + urls[len(urls) // 2 : len(urls) // 2 + 1] + urls[-1:]
    for u in dict.fromkeys(elegidos):
        guardar("boe-" + u.rsplit("/", 1)[-1], u)
