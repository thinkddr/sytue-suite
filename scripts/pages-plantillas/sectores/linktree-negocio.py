# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
from lib import *

KEY = "linktree-negocio"

# Paleta propia: carbón con lima. primary/inverse = 13, así que admite fondos $inverse.
LOOK = dict(
    colores=dict(background="#121211", surface="#1d1d1b", text="#f5f5f2", muted="#b6b6ae", primary="#a3e635",
                 on_primary="#10130a", border="#35352f", inverse="#0a0a09", on_inverse="#f5f5f2"),
    fuentes=("dm-sans", "dm-sans"), peso=800, h1=(34, 32, 30), radio=(10, 16, 24))

ANCHO = 480


def columna(hijos, gap=16):
    """Columna centrada de ancho máximo 480: la página entera es una tira de móvil."""
    return marco(hijos, gap=gap, align="center", sz={"width": "fill", "height": "hug", "max_width": ANCHO})


def tira(kind, hijos, anchor=None, tono="base", pad_v=32, gap=16):
    return seccion(kind, [columna(hijos, gap)], anchor=anchor, tono=tono, pad_v=pad_v, align="center", gap=0)


def enlace(t, destino, icono_, var="secondary"):
    """Botón grande a todo el ancho de la columna."""
    return boton(t, destino, var, icono_, sz={"width": "fill", "height": "hug"})


def build():
    nueva(KEY)
    horas = [(("Lunes a viernes", "Monday to Friday"), ("9:00 a 14:00 y 16:00 a 20:00", "9:00 am to 2:00 pm and 4:00 pm to 8:00 pm")),
             (("Sábado", "Saturday"), ("9:00 a 14:00", "9:00 am to 2:00 pm")),
             (("Domingo", "Sunday"), ("Cerrado", "Closed"))]

    # Cabecera mínima: solo el nombre, centrado.
    logo = marco([texto("span", NEGOCIO, preset="h4", align="center")], enlace=a_ancla("inicio"),
                 sz={"width": "hug", "height": "hug"})
    cab = seccion("header", [logo], pad_v=16, gap=0, align="center", justify="center")
    cab["breakpoints"]["mobile"]["layout"] = {"padding": [12, 20, 12, 20], "gap": 0, "direction": "column",
                                              "align": "center"}

    portada = tira("hero", [
        imagen("logo", ("Logotipo del negocio", "Business logo"), ratio=1, w="hug", maxw=96, radio="$lg"),
        h1(NEGOCIO, align="center"),
        p(("Escribe aquí tu frase: qué haces y en qué barrio.",
           "Write your line here: what you do and in which neighbourhood."), align="center", color="$muted"),
        marco([
            enlace(("Escribir por WhatsApp", "Message on WhatsApp"), a_wa(), "whatsapp", "primary"),
            enlace(("Llamar ahora", "Call now"), a_tel(), "phone"),
            enlace(("Ver servicios y precios", "See services and prices"), a_ancla("servicios"), "tag"),
            enlace(("Reservar", "Book"), a_ancla("reservar"), "calendar"),
            enlace(("Cómo llegar", "How to get here"), a_ancla("como-llegar"), "map-pin"),
            enlace(("Escribir un correo", "Send an email"), a_correo(), "mail"),
        ], gap=12, sz={"width": "fill", "height": "hug"}),
    ], anchor="inicio", pad_v=40, gap=20)

    servicios = tira("pricing", [
        h2(("Servicios y precios", "Services and prices")),
        tabla(("Servicios y precios", "Services and prices"), [("Servicio", "Service"), ("Precio", "Price")],
              [(("Servicio básico", "Basic service"), eur(20)), (("Servicio completo", "Full service"), eur(35)),
               (("Primera visita", "First visit"), eur(30))]),
        peq(("Precios de ejemplo: cámbialos por los tuyos.", "Example prices: replace them with your own."),
            color="$muted", align="center"),
    ], anchor="servicios", tono="relleno", pad_v=40, gap=16)

    reservar = tira("contact", [
        h2(("Reservar", "Book")),
        p(("Déjanos tus datos y te confirmamos la hora por teléfono o WhatsApp.",
           "Leave your details and we will confirm the time by phone or WhatsApp."), align="center", color="$muted"),
        tarjeta([formulario(campos_cita(fecha=True, mensaje=True),
                            ("Pedir cita", "Request appointment"),
                            ("Gracias. Te confirmamos la hora por teléfono o WhatsApp.",
                             "Thank you. We will confirm the time by phone or WhatsApp."))],
                estilo="borde", pad=24, tag="div", sz={"width": "fill", "height": "hug", "min_width": 0}),
    ], anchor="reservar", pad_v=40, gap=16)

    horario_ = tira("text", [
        h2(("Horario", "Opening hours")),
        horario(horas),
        marco([icono("map-pin"), p(CALLE)], dir="row", gap=12, align="start"),
    ], anchor="horario", pad_v=40, gap=16)

    donde = tira("map", [h2(("Cómo llegar", "How to get here")), mapa()], anchor="como-llegar", tono="relleno",
                 pad_v=40, gap=16)

    inicio = pagina("inicio", ("", ""), ("Enlaces del negocio", "Business links"),
                    ("Página de enlaces de un negocio: WhatsApp, llamar, servicios, reservar, cómo llegar y correo, con horario, mapa y formulario de cita.",
                     "Link page for a business: WhatsApp, call, services, booking, directions and email, with opening hours, map and booking form."),
                    [portada, servicios, reservar, horario_, donde])

    # Pie de una tira: nombre, enlaces legales.
    legal = marco([boton(("Aviso legal", "Legal notice"), a_legal("notice"), "ghost"),
                   boton(("Privacidad", "Privacy"), a_legal("privacy"), "ghost"),
                   boton(("Cookies", "Cookies"), a_legal("cookies"), "ghost")],
                  dir="row", gap=8, wrap=True, justify="center", align="center")
    pie_ = tira("footer", [peq((f"© {NEGOCIO[0]}. Todos los derechos reservados.",
                                 f"© {NEGOCIO[1]}. All rights reserved."), color="$muted", align="center"), legal],
                pad_v=32, gap=12)

    return plantilla(KEY, ("Enlaces de mi negocio", "My business links"),
                     ("Una página en columna para el móvil: botones de WhatsApp, llamar, servicios, reservar, cómo llegar y correo, con horario, mapa y formulario.",
                      "A single mobile-first column page: WhatsApp, call, services, booking, directions and email buttons, with opening hours, map and form."),
                     "web", ("enlaces links linktree linkinbio bio instagram pagina de enlaces tarjeta digital movil whatsapp negocio",
                             "links linktree link in bio instagram link page digital card mobile whatsapp business"),
                     LOOK, cab, pie_, [inicio])
