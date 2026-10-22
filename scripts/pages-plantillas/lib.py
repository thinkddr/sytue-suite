# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
"""Biblioteca para escribir plantillas de Sytue Web como código y emitir su IR en JSON.

Cada sector es un fichero `sectores/<clave>.py` que llama a estas funciones y devuelve
`plantilla(...)`. `generar.py` los recorre y escribe `services/pages/internal/plantillas/<clave>.json`.
El JSON es lo que manda: el servicio lo embebe y lo valida con su propio validador y con
`catalogo_test.go`. Aquí solo hay una forma cómoda de componerlo, con ids estables (el mismo
fuente da siempre el mismo JSON) y los mismos tokens de diseño en todas las plantillas.

Los textos son pares `(es, en)`. Nada de emojis, de imágenes remotas ni de relleno tipo lorem.
"""

from __future__ import annotations

import hashlib
import re

# ---------------------------------------------------------------------------------------------
# Marcadores de relleno. Todo lo que la persona TIENE que cambiar usa una de estas cadenas, y
# `domain.TextosDemo` impide publicar mientras alguna siga en el sitio.

NEGOCIO = ("Nombre del negocio", "Business name")
TEL_VISIBLE = "600 000 000"
TEL = "+34600000000"
WA = "https://wa.me/34600000000"
CORREO = "info@example.com"
CALLE = ("Calle Mayor, 1, 00000 Tu ciudad", "Calle Mayor, 1, 00000 Your city")
MAPA = "40.4168,-3.7038,15"
COLEGIADO = ("Nº de colegiado: 00-0000", "Registration no.: 00-0000")  # profesiones colegiadas: ver TextosDemo

# ---------------------------------------------------------------------------------------------
# Estado de la plantilla en curso (los ids salen de la clave y de un contador: estables).

_ctx = {"key": "", "n": 0, "paginas": {}}


def _id() -> str:
    _ctx["n"] += 1
    h = hashlib.sha1(f"{_ctx['key']}/{_ctx['n']}".encode()).hexdigest()
    return _b36(int(h, 16))[:10].rjust(10, "0")


def _b36(n: int) -> str:
    cifras = "0123456789abcdefghijklmnopqrstuvwxyz"
    s = ""
    while n:
        n, r = divmod(n, 36)
        s = cifras[r] + s
    return s or "0"


def pagina_id(nombre: str) -> str:
    p = _ctx["paginas"]
    if nombre not in p:
        h = hashlib.sha1(f"{_ctx['key']}/pagina/{nombre}".encode()).hexdigest()
        p[nombre] = _b36(int(h, 16))[:10].rjust(10, "0")
    return p[nombre]


def L(t):
    """(es, en) -> {es, en}. Un dict ya hecho pasa tal cual."""
    if isinstance(t, dict):
        return t
    es, en = t
    return {"es": es, "en": en}


def rico(t):
    t = L(t)
    return {k: [{"text": v}] for k, v in t.items()}


# ---------------------------------------------------------------------------------------------
# Enlaces


def a_pagina(nombre: str, ancla: str | None = None) -> dict:
    e = {"page": pagina_id(nombre)}
    if ancla:
        e["anchor"] = ancla
    return e


def a_ancla(ancla: str) -> dict:
    return {"anchor": ancla}


def a_tel() -> dict:
    return {"url": f"tel:{TEL}"}


def a_wa() -> dict:
    return {"url": WA, "new_tab": True}


def a_correo() -> dict:
    return {"url": f"mailto:{CORREO}"}


def a_legal(cual: str) -> dict:
    return {"legal": cual}


# ---------------------------------------------------------------------------------------------
# Nodos


def _ajusta(n: dict, size=None, style=None, bp=None) -> dict:
    if size:
        n["size"] = size
    if style:
        n["style"] = style
    if bp:
        n["breakpoints"] = bp
    return n


def tam(w="fill", h="hug", **extra) -> dict:
    d = {"width": w, "height": h}
    d.update(extra)
    return d


def texto(tag: str, t, align=None, preset=None, color=None, transform=None, weight=None, size=None,
          italic=None, sz=None, bp=None) -> dict:
    st = {}
    tx = {}
    if preset:
        tx["preset"] = preset
    if align:
        tx["align"] = align
    if transform:
        tx["transform"] = transform
    if weight:
        tx["weight"] = weight
    if size:
        tx["size"] = size
    if italic is not None:
        tx["italic"] = italic
    if tx:
        st["text"] = tx
    if color:
        st["color"] = color
    return _ajusta({"id": _id(), "type": "text", "tag": tag, "content": rico(t)}, sz, st or None, bp)


def h1(t, **k):
    return texto("h1", t, **k)


def h2(t, **k):
    return texto("h2", t, **k)


def h3(t, **k):
    return texto("h3", t, **k)


def p(t, **k):
    return texto("p", t, **k)


def peq(t, **k):
    return texto("p", t, preset="small", **k)


def etiqueta(t, **k):
    """Rótulo corto en mayúsculas sobre un título (en un fondo oscuro, sin color propio)."""
    oscuro = k.pop("oscuro", False)
    return texto("p", t, preset="label", transform="uppercase", color=None if oscuro else k.pop("color", "$primary"), **k)


def boton(t, enlace: dict, variante="primary", icono=None, sz=None, label=None) -> dict:
    n = {"id": _id(), "type": "button", "label": L(t), "variant": variante, "action": enlace}
    if icono:
        n["icon"] = icono
        n["icon_position"] = "start"
    if sz:
        n["size"] = sz
    return n


def llamar(variante="primary", t=("Llamar ahora", "Call now")):
    return boton(t, a_tel(), variante, "phone")


def whatsapp(variante="secondary", t=("Escribir por WhatsApp", "Message on WhatsApp")):
    return boton(t, a_wa(), variante, "whatsapp")


def imagen(tipo: str, alt, ratio=None, fit="cover", prioridad=False, w="fill", h="hug", radio="$md",
           decorativa=False, maxw=None, bp=None) -> dict:
    """Relleno de imagen (photo, portrait, product, landscape, logo) que la persona sustituye."""
    sz = {"width": w, "height": h}
    if ratio:
        sz["aspect_ratio"] = ratio
    if maxw:
        sz["max_width"] = maxw
    n = {"id": _id(), "type": "image", "src": {"placeholder": tipo}, "alt": L(alt), "fit": fit, "size": sz}
    if radio:
        n["style"] = {"radius": radio, "overflow": "hidden"}
    if prioridad:
        n["priority"] = True
    if bp:
        n["breakpoints"] = bp
    return n


def icono(nombre: str, label=None, sz=24) -> dict:
    n = {"id": _id(), "type": "icon", "name": nombre, "size": {"width": sz, "height": sz}}
    if label:
        n["label"] = L(label)
    return n


def marco(hijos, dir="column", gap=16, pad=0, align="start", justify="start", wrap=False, columns=None,
          tag="div", sz=None, style=None, bp=None, enlace=None) -> dict:
    hijos = [h for h in hijos if h is not None]
    pad4 = [pad] * 4 if isinstance(pad, int) else list(pad)
    lay = {"direction": dir, "gap": gap, "padding": pad4, "align": align, "justify": justify, "wrap": wrap}
    if columns:
        lay["columns"] = columns
    n = {"id": _id(), "type": "frame", "tag": tag, "layout": lay, "children": hijos}
    if enlace:
        n["link"] = enlace
    return _ajusta(n, sz, style, bp)


def en_pantalla(**delta) -> dict:
    """Atajo de breakpoints: en_pantalla(tablet={...}, mobile={...})."""
    return {k: v for k, v in delta.items() if v}


def tarjeta(hijos, estilo="borde", gap=12, pad=24, tag="li", enlace=None, align="start", sz=None) -> dict:
    st = {"radius": "$md"}
    if estilo == "borde":
        st["border"] = {"width": 1, "color": "$border", "style": "solid"}
    elif estilo == "relleno":
        st["background"] = "$surface"
    elif estilo == "oscuro":
        st["background"] = "$inverse"
        st["color"] = "$on_inverse"
    elif estilo == "linea":  # sin caja: solo una raya arriba
        st = {"border": {"width": 1, "color": "$border", "style": "solid"}, "radius": 0}
    return marco(hijos, gap=gap, pad=pad, tag=tag, style=st, enlace=enlace, align=align, sz=sz)


def rejilla(items, cols=3, gap=24, tablet=None, movil=1, lista=True, align="start") -> dict:
    """Filas con N columnas que bajan a `tablet` y a `movil`. Con lista=True, ul/li."""
    tablet = tablet or (2 if cols > 2 else cols)
    bp = {"tablet": {"layout": {"columns": tablet}}, "mobile": {"layout": {"columns": movil}}}
    return marco(items, dir="row", gap=gap, wrap=True, columns=cols, tag="ul" if lista else "div", bp=bp,
                 align=align)


def lista_vertical(items, gap=12) -> dict:
    return marco(items, gap=gap, tag="ul")


def fila_li(hijos, **k) -> dict:
    return marco(hijos, tag="li", **k)


def tabla(caption, cabecera, filas) -> dict:
    return {
        "id": _id(), "type": "table", "caption": L(caption),
        "header": [L(c) for c in cabecera],
        "rows": [[L(c) for c in f] for f in filas],
    }


def eur(n) -> tuple:
    """Precio en euros en los dos idiomas: eur(14) -> ('14 €', '€14'); eur(12.5) -> ('12,50 €', '€12.50')."""
    if isinstance(n, str):
        return (n, n)
    if float(n).is_integer():
        return (f"{int(n)} €", f"€{int(n)}")
    es = f"{n:.2f}".replace(".", ",")
    return (f"{es} €", f"€{n:.2f}")


def desde(n) -> tuple:
    a, b = eur(n)
    return (f"Desde {a}", f"From {b}")


def campo(clave: str, tipo: str, etiqueta_, requerido=False, opciones=None, auto=None, maxlen=None) -> dict:
    c = {"id": _id(), "kind": tipo, "label": L(etiqueta_), "required": requerido}
    if opciones:
        c["options"] = [L(o) for o in opciones]
    if auto:
        c["autocomplete"] = auto
    if maxlen:
        c["max_length"] = maxlen
    return c


def formulario(campos, envio=("Enviar", "Send"), exito=None, consentimiento=None, clave="") -> dict:
    exito = exito or ("Gracias. Hemos recibido tu mensaje y te respondemos lo antes posible.",
                      "Thank you. We have received your message and will reply as soon as possible.")
    consentimiento = consentimiento or ("He leído y acepto la política de privacidad.",
                                        "I have read and accept the privacy policy.")
    return {
        "id": _id(), "type": "form", "form": _id(),
        "layout": {"direction": "column", "gap": 14, "padding": [0, 0, 0, 0], "align": "start",
                   "justify": "start", "wrap": False},
        "fields": campos, "submit": L(envio), "success": L(exito), "consent": L(consentimiento),
        "size": {"width": "fill", "height": "hug"},
    }


def mapa(titulo=("Mapa con la ubicación del negocio", "Map showing the business location")) -> dict:
    return {"id": _id(), "type": "embed", "provider": "osm", "ref": MAPA, "title": L(titulo),
            "size": {"width": "fill", "height": "hug", "aspect_ratio": 1.78}}


def productos(limite=8, cols=4) -> dict:
    return {"id": _id(), "type": "product_grid", "source": {"all": True}, "limit": limite, "columns": cols,
            "show_price": True, "show_add_to_cart": True}


def carrito(t=("Carrito", "Cart")) -> dict:
    return {"id": _id(), "type": "cart_button", "label": L(t)}


def menu(items, etiqueta_=("Navegación principal", "Main navigation"), colapso="tablet") -> dict:
    """items: [(nombre_pagina, (es, en))]."""
    return {
        "id": _id(), "type": "menu", "label": L(etiqueta_), "orientation": "horizontal",
        "collapse_below": colapso,
        "items": [{"id": _id(), "label": L(t), "link": a_pagina(n)} for n, t in items],
    }


# ---------------------------------------------------------------------------------------------
# Secciones. `tono`: base (fondo de la página), relleno ($surface) u oscuro ($inverse).


def seccion(kind: str, hijos, anchor=None, tono="base", pad_v=72, gap=40, align="start", justify="start",
            dir="column") -> dict:
    hijos = [h for h in hijos if h is not None]
    st = None
    if tono == "relleno":
        st = {"background": "$surface"}
    elif tono == "oscuro":
        st = {"background": "$inverse", "color": "$on_inverse"}
    lay = {"direction": dir, "gap": gap, "padding": [pad_v, 32, pad_v, 32], "align": align,
           "justify": justify, "wrap": False}
    movil_pad = max(40, pad_v // 2 + 8)
    n = {
        "id": _id(), "type": "section", "kind": kind, "full_bleed": False, "layout": lay, "children": hijos,
        "breakpoints": {"mobile": {"layout": {"padding": [movil_pad, 20, movil_pad, 20], "gap": max(24, gap - 12)}}},
    }
    if anchor:
        n["anchor"] = anchor
    if st:
        n["style"] = st
    return n


def titular(h, intro=None, eyebrow=None, align="start", nivel="h2", maxw=680, tono="base") -> dict:
    """Bloque de título de sección: rótulo, título y entradilla con una medida de línea legible."""
    hijos = []
    if eyebrow:
        hijos.append(etiqueta(eyebrow, align=align, oscuro=tono == "oscuro"))
    hijos.append(texto(nivel, h, align=align))
    if intro:
        hijos.append(p(intro, align=align, color=None if tono == "oscuro" else "$muted"))
    return marco(hijos, gap=12, align="center" if align == "center" else "start",
                 sz={"width": "hug" if align == "center" else "fill", "height": "hug", "max_width": maxw})


def hero(titulo, entradilla, botones, eyebrow=None, imagen_=None, variante="split", tono="base", extra=None,
         alt=None) -> dict:
    """Portada de una página. variante: split (texto + imagen), centro, texto (sin imagen).

    `imagen_` es (tipo, alt). El h1 es el titular de la página.
    """
    texto_ = marco(
        [etiqueta(eyebrow) if eyebrow else None, h1(titulo), p(entradilla, preset="body", color="$muted" if tono != "oscuro" else None),
         extra,
         marco(botones, dir="row", gap=12, wrap=True, align="center")],
        gap=18, align="start" if variante != "centro" else "center",
    )
    if variante == "centro":
        texto_ = marco(
            [etiqueta(eyebrow, align="center") if eyebrow else None, h1(titulo, align="center"),
             p(entradilla, align="center", color="$muted" if tono != "oscuro" else None), extra,
             marco(botones, dir="row", gap=12, wrap=True, align="center", justify="center")],
            gap=18, align="center", sz={"width": "hug", "height": "hug", "max_width": 760},
        )
        return seccion("hero", [texto_], tono=tono, pad_v=88, align="center", gap=0)
    if variante == "texto" or imagen_ is None:
        return seccion("hero", [marco([texto_], sz={"width": "fill", "height": "hug", "max_width": 760})],
                       tono=tono, pad_v=72, gap=0)
    tipo, alt_ = imagen_
    foto = imagen(tipo, alt_, ratio=1.2 if tipo in ("photo", "landscape") else 0.9, prioridad=True)
    fila = marco(
        [marco([texto_], sz={"width": "fill", "height": "hug", "min_width": 0}, align="start"),
         marco([foto], sz={"width": "fill", "height": "hug", "min_width": 0})],
        dir="row", gap=56, align="center",
        bp={"tablet": {"layout": {"gap": 32}}, "mobile": {"layout": {"direction": "column", "gap": 28}}},
    )
    return seccion("hero", [fila], tono=tono, pad_v=80, gap=0)


def bloque_valores(titulo, items, intro=None, eyebrow=None, cols=3, tono="base", estilo="borde", anchor=None,
                   kind="features", align="start") -> dict:
    """Tarjetas con un título corto y una frase (por qué elegirnos, servicios, cómo trabajamos)."""
    tarjetas = [tarjeta([h3(h), p(t, color="$muted" if tono != "oscuro" else None)], estilo=estilo)
                for h, t in items]
    return seccion(kind, [titular(titulo, intro, eyebrow, align=align, tono=tono), rejilla(tarjetas, cols=cols)],
                   tono=tono, anchor=anchor)


def bloque_pasos(titulo, items, intro=None, eyebrow=None, tono="base", anchor=None) -> dict:
    """Pasos numerados (cómo trabajamos, cómo pedir). Lista ordenada."""
    li = []
    for i, (h, t) in enumerate(items, 1):
        li.append(marco([texto("p", (str(i), str(i)), preset="label", color="$primary", weight=700),
                         h3(h), p(t, color="$muted")], tag="li", gap=8,
                        style={"border": {"width": 1, "color": "$border", "style": "solid"}, "radius": "$md"},
                        pad=24))
    cols = min(len(li), 4)
    return seccion("features", [titular(titulo, intro, eyebrow),
                                marco(li, dir="row", gap=20, wrap=True, columns=cols, tag="ol",
                                      bp={"tablet": {"layout": {"columns": 2}}, "mobile": {"layout": {"columns": 1}}})],
                   tono=tono, anchor=anchor)


def bloque_precios(titulo, caption, cabecera, filas, intro=None, eyebrow=None, nota=None, boton_=None,
                   tono="base", anchor="precios", kind="pricing") -> dict:
    hijos = [titular(titulo, intro, eyebrow), tabla(caption, cabecera, filas)]
    if nota:
        hijos.append(peq(nota, color="$muted"))
    if boton_:
        hijos.append(boton_)
    return seccion(kind, hijos, tono=tono, anchor=anchor, gap=28)


def bloque_carta(titulo, grupos, intro=None, eyebrow=None, tono="base", anchor="carta", nota=None) -> dict:
    """grupos: [(nombre_grupo, caption, cabecera, filas)]. Una tabla por grupo."""
    hijos = [titular(titulo, intro, eyebrow)]
    for nombre, cap, cab, filas in grupos:
        hijos.append(marco([h3(nombre), tabla(cap, cab, filas)], gap=14))
    if nota:
        hijos.append(peq(nota, color="$muted"))
    return seccion("pricing", hijos, tono=tono, anchor=anchor, gap=36)


def bloque_galeria(titulo, alts, intro=None, eyebrow=None, tipo="photo", cols=3, tono="base", ratio=1.0,
                   anchor="galeria") -> dict:
    fotos = [marco([imagen(tipo, a, ratio=ratio)], tag="li", gap=0) for a in alts]
    return seccion("gallery", [titular(titulo, intro, eyebrow), rejilla(fotos, cols=cols, gap=16, movil=2)],
                   tono=tono, anchor=anchor)


def bloque_opiniones(titulo, sobre, n=3, intro=None, tono="relleno") -> dict:
    """Opiniones de relleno: el sitio no se publica hasta que se sustituyan por reales (domain.TextosDemo)."""
    tarjetas = []
    for i in range(n):
        tarjetas.append(tarjeta([
            texto("blockquote", (f"Aquí va la opinión de un cliente real sobre {sobre[i % len(sobre)][0]}.",
                                 f"A real customer's opinion about {sobre[i % len(sobre)][1]} goes here.")),
            texto("p", ("Nombre Apellido", "First Last"), preset="small", color="$muted"),
        ], estilo="borde" if tono == "relleno" else "relleno"))
    return seccion("testimonials", [titular(titulo, intro), rejilla(tarjetas, cols=min(n, 3))], tono=tono)


def bloque_equipo(titulo, personas, intro=None, tono="base", cols=3, anchor="equipo") -> dict:
    """personas: [(rol, frase)]. El nombre es de relleno (Nombre Apellido)."""
    tarjetas = [marco([imagen("portrait", ("Foto de la persona del equipo", "Photo of the team member"),
                              ratio=0.9),
                       h3(("Nombre Apellido", "First Last")), peq(rol, color="$muted"), p(frase)],
                      tag="li", gap=10) for rol, frase in personas]
    return seccion("team", [titular(titulo, intro), rejilla(tarjetas, cols=cols)], tono=tono, anchor=anchor)


def bloque_faq(titulo, preguntas, intro=None, tono="base", anchor="preguntas") -> dict:
    li = [marco([h3(q), p(a, color="$muted")], tag="li", gap=8,
                style={"border": {"width": 1, "color": "$border", "style": "solid"}, "radius": "$md"}, pad=20)
          for q, a in preguntas]
    return seccion("faq", [titular(titulo, intro),
                           marco(li, gap=12, tag="ul", sz={"width": "fill", "height": "hug", "max_width": 820})],
                   tono=tono, anchor=anchor)


def bloque_texto(titulo, parrafos, eyebrow=None, tono="base", imagen_=None, anchor=None, invertir=False) -> dict:
    cuerpo = marco([etiqueta(eyebrow) if eyebrow else None, h2(titulo)] + [p(x) for x in parrafos], gap=16)
    if imagen_ is None:
        return seccion("text", [marco([cuerpo], sz={"width": "fill", "height": "hug", "max_width": 760})],
                       tono=tono, anchor=anchor, gap=0)
    tipo, alt = imagen_
    foto = imagen(tipo, alt, ratio=1.2 if tipo in ("photo", "landscape") else 0.9)
    partes = [marco([cuerpo], sz={"width": "fill", "height": "hug", "min_width": 0}),
              marco([foto], sz={"width": "fill", "height": "hug", "min_width": 0})]
    if invertir:
        partes.reverse()
    fila = marco(partes, dir="row", gap=56, align="center",
                 bp={"tablet": {"layout": {"gap": 32}}, "mobile": {"layout": {"direction": "column", "gap": 28}}})
    return seccion("text", [fila], tono=tono, anchor=anchor, gap=0)


def bloque_productos(titulo, intro=None, eyebrow=None, limite=8, cols=4, tono="base", anchor="tienda",
                     boton_=None) -> dict:
    """Rejilla del catálogo de la tienda (vacía hasta que se añaden productos en Tienda)."""
    return seccion("products", [titular(titulo, intro, eyebrow), productos(limite, cols), boton_],
                   tono=tono, anchor=anchor, gap=28)


def bloque_checks(titulo, items, intro=None, eyebrow=None, tono="base", cols=2, anchor=None, kind="features",
                  imagen_=None) -> dict:
    """Lista de puntos con una marca (qué incluye, qué llevamos, a quién ayudamos). items: [(es, en)]."""
    li = [marco([icono("check", sz=22), p(t)], tag="li", dir="row", gap=12, align="start") for t in items]
    lista = marco(li, dir="row", gap=16, wrap=True, columns=cols, tag="ul",
                  bp={"tablet": {"layout": {"columns": min(cols, 2)}}, "mobile": {"layout": {"columns": 1}}})
    return seccion(kind, [titular(titulo, intro, eyebrow), lista], tono=tono, anchor=anchor)


def bloque_cta(titulo, texto_=None, botones=None, tono="relleno", kind="cta") -> dict:
    botones = botones or [llamar(), whatsapp("ghost")]
    return seccion(kind, [marco([h2(titulo, align="center"),
                                 p(texto_, align="center") if texto_ else None,
                                 marco(botones, dir="row", gap=12, wrap=True, align="center", justify="center")],
                                gap=18, align="center", sz={"width": "hug", "height": "hug", "max_width": 720})],
                   tono=tono, pad_v=64, align="center", gap=0)


def horario(filas, caption=("Horario de apertura", "Opening hours")) -> dict:
    """filas: [((es, en) día(s), (es, en) horas)]."""
    return tabla(caption, [("Día", "Day"), ("Horas", "Hours")], filas)


def bloque_contacto(titulo, intro, campos, envio, filas_horario, direccion=CALLE, eyebrow=None,
                    notas=None, anchor="contacto", tono="base", exito=None, mapa_=True) -> dict:
    """Datos de contacto (llamar, WhatsApp, correo, dirección y horario) junto a un formulario."""
    datos = marco([
        titular(titulo, intro, eyebrow),
        marco([icono("map-pin"), p(direccion)], dir="row", gap=12, align="start"),
        marco([icono("phone"), p((TEL_VISIBLE, TEL_VISIBLE))], dir="row", gap=12, align="start"),
        marco([icono("mail"), p((CORREO, CORREO))], dir="row", gap=12, align="start"),
        marco([llamar(), whatsapp()], dir="row", gap=12, wrap=True),
        horario(filas_horario) if filas_horario else None,
        peq(notas, color="$muted") if notas else None,
    ], gap=18, sz={"width": "fill", "height": "hug", "min_width": 0})
    form = tarjeta([formulario(campos, envio, exito)], estilo="relleno" if tono == "base" else "borde", pad=28,
                   tag="div", sz={"width": "fill", "height": "hug", "min_width": 0})
    fila = marco([datos, form], dir="row", gap=48, align="start",
                 bp={"tablet": {"layout": {"gap": 32}}, "mobile": {"layout": {"direction": "column", "gap": 28}}})
    return seccion("contact", [fila], anchor=anchor, tono=tono, gap=0)


def bloque_mapa(titulo=("Cómo llegar", "How to get here"), texto_=None, tono="base", anchor="como-llegar") -> dict:
    return seccion("map", [titular(titulo, texto_), mapa()], tono=tono, anchor=anchor, gap=28)


def campos_cita(servicios=None, fecha=True, mensaje=True, extra=None) -> list:
    c = [campo("nombre", "text", ("Nombre", "Name"), True, auto="name", maxlen=120),
         campo("telefono", "tel", ("Teléfono", "Phone"), True, auto="tel", maxlen=30)]
    if servicios:
        c.append(campo("servicio", "select", ("Servicio", "Service"), False, opciones=servicios))
    if fecha:
        c.append(campo("fecha", "date", ("Día que te viene bien", "Day that suits you")))
    if extra:
        c.extend(extra)
    if mensaje:
        c.append(campo("mensaje", "textarea", ("Mensaje (opcional)", "Message (optional)"), False, maxlen=1000))
    return c


def campos_consulta(mensaje_etiqueta=("¿En qué podemos ayudarte?", "How can we help you?"), servicios=None,
                    extra=None) -> list:
    c = [campo("nombre", "text", ("Nombre", "Name"), True, auto="name", maxlen=120),
         campo("correo", "email", ("Correo electrónico", "Email"), True, auto="email", maxlen=200),
         campo("telefono", "tel", ("Teléfono (opcional)", "Phone (optional)"), False, auto="tel", maxlen=30)]
    if servicios:
        c.append(campo("tema", "select", ("Asunto", "Subject"), False, opciones=servicios))
    if extra:
        c.extend(extra)
    c.append(campo("mensaje", "textarea", mensaje_etiqueta, True, maxlen=2000))
    return c


# ---------------------------------------------------------------------------------------------
# Cabecera y pie


def cabecera(paginas, cta=None, tienda=False, marca=NEGOCIO) -> dict:
    """paginas: [(nombre_pagina, (es, en))]; la primera es la portada. cta: botón de la derecha."""
    logo = marco([texto("span", marca, preset="h4")], enlace=a_pagina(paginas[0][0]), sz={"width": "hug", "height": "hug"})
    cta = cta or llamar()
    der = [menu(paginas)]
    if tienda:
        der.append(carrito())
    # varios botones sueltos (llamar + WhatsApp) en la fila, no en un marco que no pueda partirse
    if cta["type"] == "frame" and cta["tag"] == "div":
        der.extend(cta["children"])
    else:
        der.append(cta)
    der_marco = marco(der, dir="row", gap=12, align="center", justify="end", wrap=True,
                      sz={"width": "fill", "height": "hug", "min_width": 0},
                      bp={"mobile": {"layout": {"justify": "start"}}})
    n = seccion("header", [logo, der_marco],
                tono="base", pad_v=16, gap=24, dir="row", align="center", justify="space-between")
    n["breakpoints"]["mobile"]["layout"] = {"padding": [12, 20, 12, 20], "gap": 10, "direction": "column",
                                            "align": "start"}
    n["style"] = {"border": {"width": 1, "color": "$border", "style": "solid"}}
    return n


def pie(paginas, resumen, horario_corto=None, marca=NEGOCIO) -> dict:
    """Pie: marca y frase, datos de contacto, horario corto y enlaces legales."""
    col_marca = marco([texto("p", marca, preset="h4"), peq(resumen, color="$muted")], gap=8,
                      sz={"width": "fill", "height": "hug", "min_width": 0})
    col_contacto = marco([texto("p", ("Contacto", "Contact"), preset="label", transform="uppercase"),
                          p(CALLE), p((TEL_VISIBLE, TEL_VISIBLE)), p((CORREO, CORREO))], gap=6,
                         sz={"width": "fill", "height": "hug", "min_width": 0})
    cols = [col_marca, col_contacto]
    if horario_corto:
        cols.append(marco([texto("p", ("Horario", "Hours"), preset="label", transform="uppercase"),
                           p(horario_corto)], gap=6, sz={"width": "fill", "height": "hug", "min_width": 0}))
    arriba = marco(cols, dir="row", gap=32, wrap=False,
                   bp={"tablet": {"layout": {"direction": "column", "gap": 24}}})
    legal = marco([boton(("Aviso legal", "Legal notice"), a_legal("notice"), "ghost"),
                   boton(("Privacidad", "Privacy"), a_legal("privacy"), "ghost"),
                   boton(("Cookies", "Cookies"), a_legal("cookies"), "ghost")], dir="row", gap=8, wrap=True)
    abajo = marco([peq((f"© {NEGOCIO[0]}. Todos los derechos reservados.",
                        f"© {NEGOCIO[1]}. All rights reserved."), color="$muted"), legal],
                  dir="row", gap=16, wrap=True, align="center", justify="space-between")
    n = seccion("footer", [arriba, abajo], tono="relleno", pad_v=48, gap=28)
    return n


# ---------------------------------------------------------------------------------------------
# Tema


def _estilos(look: dict) -> dict:
    hd = look.get("h1", (52, 40, 32))
    peso = look.get("peso", 700)
    trans = look.get("mayus")
    ls = look.get("ls", 0)
    base = look.get("cuerpo", (17, 16, 16))

    def e(font, size, lh, w, l=0, tr=None):
        d = {"font": font, "size": list(size), "line_height": lh, "weight": w, "letter_spacing": l}
        if tr:
            d["transform"] = tr
        return d

    return {
        "h1": e("heading", hd, 1.1, peso, ls, trans),
        "h2": e("heading", (round(hd[0] * 0.72), round(hd[1] * 0.75), round(hd[2] * 0.78)), 1.2, peso, ls, trans),
        "h3": e("heading", (22, 21, 20), 1.3, min(peso, 700) if peso > 600 else peso, 0),
        "h4": e("heading", (20, 19, 18), 1.3, 600, 0),
        "h5": e("body", (18, 17, 16), 1.4, 600),
        "h6": e("body", (16, 15, 15), 1.4, 600),
        "body": e("body", base, 1.6, 400),
        "small": e("body", (14, 14, 14), 1.5, 400),
        "label": e("body", (13, 13, 13), 1.4, 600, 0.08),
        "button": e("body", (16, 16, 16), 1.2, 600),
    }


def tema(look: dict) -> dict:
    c = dict(look["colores"])
    return {
        "colors": c,
        "fonts": {"heading": look["fuentes"][0], "body": look["fuentes"][1]},
        "text_styles": _estilos(look),
        "radius": dict(zip(("sm", "md", "lg"), look.get("radio", (4, 8, 14)))),
        "container": look.get("ancho", 1120),
    }


# ---------------------------------------------------------------------------------------------
# Plantilla completa


def pagina(nombre, slug, titulo, descripcion, secciones) -> dict:
    return {
        "id": pagina_id(nombre), "slug": L(slug), "title": L(titulo), "description": L(descripcion),
        "sections": [s for s in secciones if s is not None], "show_header": True, "show_footer": True,
    }


def nueva(key: str) -> None:
    _ctx.update(key=key, n=0, paginas={})


def plantilla(key, nombre, descripcion, categoria, claves, look, header, footer, paginas, tienda=False,
              vertical=None, idiomas=("es", "en"), exento=None) -> dict:
    """`exento`: {'telefono'|'mapa'|'horario': motivo} para sectores a los que no les corresponde.

    No va al JSON: hay que repetirlo en las listas sinTelefono/sinMapa/sinHorario de catalogo_test.go.
    """
    return {"exento": exento or {},
        "key": key, "name": L(nombre), "description": L(descripcion), "category": categoria,
        "keywords": L(claves), "vertical": vertical or key, "store": tienda,
        "ir": {
            "version": 1,
            "languages": {"default": idiomas[0], "others": list(idiomas[1:])},
            "theme": tema(look), "header": header, "footer": footer, "pages": paginas, "components": [],
            "seo": {"site_name": L(NEGOCIO)},
        },
    }


# ---------------------------------------------------------------------------------------------
# Contraste (WCAG) para comprobar las paletas al generar


def _lum(hexa: str) -> float:
    h = hexa.lstrip("#")
    rgb = [int(h[i:i + 2], 16) / 255 for i in (0, 2, 4)]
    lin = [c / 12.92 if c <= 0.04045 else ((c + 0.055) / 1.055) ** 2.4 for c in rgb]
    return 0.2126 * lin[0] + 0.7152 * lin[1] + 0.0722 * lin[2]


def contraste(a: str, b: str) -> float:
    la, lb = _lum(a), _lum(b)
    if la < lb:
        la, lb = lb, la
    return (la + 0.05) / (lb + 0.05)


PARES_AA = [("text", "background"), ("text", "surface"), ("muted", "background"), ("muted", "surface"),
            ("on_primary", "primary"), ("primary", "background"), ("primary", "surface"),
            ("on_inverse", "inverse")]


def comprobar_paleta(nombre: str, c: dict) -> list:
    malos = []
    for a, b in PARES_AA:
        if contraste(c[a], c[b]) < 4.5:
            malos.append(f"{nombre}: {a}/{b} = {contraste(c[a], c[b]):.2f}")
    return malos


_EMOJI = re.compile("[\U0001F300-\U0001FAFF☀-➿⌀-⏿⭐-⭕]")


def revisar(pl: dict) -> list:
    """Comprobaciones rápidas al generar; la autoridad es el validador de Go y catalogo_test.go."""
    import json
    s = json.dumps(pl, ensure_ascii=False)
    malos = []
    if _EMOJI.search(s):
        malos.append("emoji")
    if "!!" in s:
        malos.append("doble exclamación")
    malos += comprobar_paleta(pl["key"], pl["ir"]["theme"]["colors"])
    if re.search(r"lorem|ipsum", s, re.I):
        malos.append("lorem")
    if re.search(r"src\"\s*:\s*\{\"(url|asset|library)", s):
        malos.append("imagen no de relleno")
    return malos


def validar_rapido(pl: dict) -> list:
    """Espejo ligero de catalogo_test.go para dar respuesta inmediata al escribir un sector."""
    malos = []
    ir = pl["ir"]
    ids, paginas = set(), {p["id"] for p in ir["pages"]}

    def nodos(lista):
        for n in lista:
            yield n
            yield from nodos(n.get("children", []))

    def todos():
        raiz = [ir["header"]] + [s for p in ir["pages"] for s in p["sections"]] + [ir["footer"]]
        return list(nodos(raiz))

    for n in todos():
        if n["type"] == "frame":
            lista = n["tag"] in ("ul", "ol")
            for h in n["children"]:
                es_li = h["type"] == "frame" and h["tag"] == "li"
                if lista != es_li:
                    malos.append(f"li/ul mal anidado en un marco {n['tag']} (los hijos de ul/ol son li y los li van en ul/ol)")
                    break
    for n in todos():
        if n["type"] == "frame" and n.get("link") and n["tag"] != "div":
            malos.append(f"un marco con enlace tiene que ser div, no {n['tag']} (mete un div con enlace dentro del li)")
    # anclas por página (con cabecera y pie) para los enlaces página + ancla
    anclas_pag = {}
    for p in ir["pages"]:
        a = set()
        for n in nodos([ir["header"]] + p["sections"] + [ir["footer"]]):
            if n.get("anchor"):
                a.add(n["anchor"])
        anclas_pag[p["id"]] = a
    for n in todos():
        e = n.get("action") if n["type"] == "button" else n.get("link")
        if e and "page" in e and "anchor" in e and e["anchor"] not in anclas_pag.get(e["page"], set()):
            malos.append(f"ancla {e['anchor']!r} inexistente en la página enlazada")
    # un enlace solo a ancla tiene que encontrarla en la MISMA página (la cabecera y el pie, en cualquiera)
    for p in ir["pages"]:
        for n in nodos(p["sections"]):
            e = n.get("action") if n["type"] == "button" else n.get("link")
            if e and "anchor" in e and "page" not in e and e["anchor"] not in anclas_pag[p["id"]]:
                malos.append(f"ancla {e['anchor']!r} inexistente en la página {p['slug']['es'] or 'inicio'} (usa a_pagina(nombre, ancla))")
    anclas = set()
    for n in todos():
        if n["id"] in ids:
            malos.append(f"id repetido {n['id']}")
        ids.add(n["id"])
        if n.get("anchor"):
            anclas.add(n["anchor"])
    tel = wa = mapa_ = hor = form = False
    for n in todos():
        t = n["type"]
        a = n.get("action") or {}
        if t == "button":
            u = a.get("url", "")
            tel |= u.startswith("tel:")
            wa |= u.startswith("https://wa.me/")
            if "page" in a and a["page"] not in paginas:
                malos.append("botón a página inexistente")
            if "anchor" in a and "page" not in a and a["anchor"] not in anclas:
                malos.append(f"ancla inexistente {a['anchor']}")
        if t == "embed":
            mapa_ = True
        if t == "form":
            form = True
        if t == "table" and "horario" in n["caption"]["es"].lower():
            hor = True
        if t == "text" and n["tag"].startswith("h") and "horario" in n["content"]["es"][0]["text"].lower():
            hor = True
        if t == "image" and not n.get("decorative") and not (n["alt"].get("es") and n["alt"].get("en")):
            malos.append("imagen sin alt")
    ex = pl.get("exento") or {}
    for ok, nombre, clave in ((tel, "llamar (tel:)", "telefono"), (wa, "WhatsApp", "telefono"), (mapa_, "mapa", "mapa"),
                              (hor, "horario", "horario"), (form, "formulario", "")):
        if not ok and clave not in ex or (not ok and clave == ""):
            malos.append(f"falta {nombre}")
    for p in ir["pages"]:
        ult = 0
        h1n = 0
        raiz = [ir["header"]] + p["sections"] + [ir["footer"]]
        for n in nodos(raiz):
            if n["type"] == "text" and len(n["tag"]) == 2 and n["tag"][0] == "h":
                nv = int(n["tag"][1])
                h1n += nv == 1
                if (ult == 0 and nv != 1) or (ult and nv > ult + 1):
                    malos.append(f"{p['slug']['es'] or 'inicio'}: salto de encabezado h{ult} a h{nv}")
                ult = nv
        if h1n != 1:
            malos.append(f"{p['slug']['es'] or 'inicio'}: {h1n} h1")
    return malos
