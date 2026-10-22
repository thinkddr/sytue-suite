# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
from lib import *

KEY = "reservas-simple"

# Paleta propia (petróleo sobre papel cálido). Sin fondo $inverse: primary/inverse no llega a 3.
LOOK = dict(
    colores=dict(background="#fffdf9", surface="#f4efe4", text="#1c2530", muted="#566170", primary="#155e75",
                 on_primary="#ffffff", border="#e6dfcf", inverse="#10343f", on_inverse="#eef7f9"),
    fuentes=("noto-serif", "figtree"), radio=(6, 10, 16), ancho=1040)


def cabecera_reserva():
    """Cabecera de una sola página: nombre, llamar y WhatsApp (sin menú, no hay a dónde ir)."""
    logo = marco([texto("span", NEGOCIO, preset="h4")], enlace=a_pagina("inicio"),
                 sz={"width": "hug", "height": "hug"})
    der = marco([llamar("primary", ("Llamar", "Call")), whatsapp("secondary", ("WhatsApp", "WhatsApp"))],
                dir="row", gap=12, align="center", justify="end", wrap=True,
                sz={"width": "fill", "height": "hug", "min_width": 0},
                bp={"mobile": {"layout": {"justify": "start"}}})
    n = seccion("header", [logo, der], pad_v=16, gap=24, dir="row", align="center", justify="space-between")
    n["breakpoints"]["mobile"]["layout"] = {"padding": [12, 20, 12, 20], "gap": 10, "direction": "column",
                                            "align": "start"}
    n["style"] = {"border": {"width": 1, "color": "$border", "style": "solid"}}
    return n


def build():
    nueva(KEY)
    horas = [(("Lunes a viernes", "Monday to Friday"), ("10:00 a 14:00 y 16:00 a 20:00", "10:00 am to 2:00 pm and 4:00 pm to 8:00 pm")),
             (("Sábado", "Saturday"), ("10:00 a 14:00", "10:00 am to 2:00 pm")),
             (("Domingo", "Sunday"), ("Cerrado", "Closed"))]
    nota = ("Servicios, duraciones y precios de ejemplo: cámbialos por los tuyos. Precios con IVA incluido.",
            "Example services, durations and prices: replace them with your own. VAT included.")
    servicios = [(("Visita corta", "Short visit"), ("30 min", "30 min"), eur(25)),
                 (("Visita estándar", "Standard visit"), ("45 min", "45 min"), eur(35)),
                 (("Visita larga", "Long visit"), ("60 min", "60 min"), eur(45)),
                 (("Primera visita", "First visit"), ("60 min", "60 min"), eur(40))]

    def reservar(var="primary"):
        return boton(("Pedir cita", "Request an appointment"), a_ancla("reservar"), var)

    # Portada: texto a la izquierda y, a la derecha, una ficha con lo que se quiere saber antes de pedir cita.
    ficha = tarjeta([
        texto("p", ("Antes de pedir cita", "Before you book"), preset="h4"),
        marco([icono("clock"), p(("Horario en la sección de abajo", "Opening hours further down"))],
              dir="row", gap=12, align="start"),
        marco([icono("map-pin"), p(CALLE)], dir="row", gap=12, align="start"),
        marco([icono("phone"), p((TEL_VISIBLE, TEL_VISIBLE))], dir="row", gap=12, align="start"),
    ], estilo="relleno", tag="div", pad=28, gap=16, sz={"width": "fill", "height": "hug", "min_width": 0})
    texto_hero = marco([
        etiqueta(("Con cita previa", "By appointment")),
        h1(("Te atiendo con cita, sin esperas", "I see you by appointment, no waiting")),
        p(("Trabajo con cita previa para dedicarte el tiempo que necesitas. Elige el servicio, dime qué día te viene bien y te confirmo la hora por teléfono o por WhatsApp.",
           "I work by appointment so I can give you the time you need. Choose a service, tell me which day suits you and I will confirm the time by phone or WhatsApp."),
          color="$muted"),
        marco([reservar(), whatsapp()], dir="row", gap=12, wrap=True, align="center"),
    ], gap=18, sz={"width": "fill", "height": "hug", "min_width": 0})
    portada = seccion("hero", [marco([texto_hero, ficha], dir="row", gap=48, align="center",
                                     bp={"tablet": {"layout": {"gap": 32}},
                                         "mobile": {"layout": {"direction": "column", "gap": 28}}})],
                      pad_v=72, gap=0)

    inicio = pagina("inicio", ("", ""), ("Pedir cita", "Book an appointment"),
                    ("Página de reservas para un negocio con cita previa: servicios con duración y precio, formulario de cita, horario, mapa y botones de llamar y WhatsApp.",
                     "Booking page for an appointment-only business: services with duration and price, booking form, opening hours, map and call and WhatsApp buttons."), [
        portada,
        bloque_precios(("Servicios, duración y precio", "Services, duration and price"),
                       ("Servicios con su duración y su precio", "Services with their duration and price"),
                       [("Servicio", "Service"), ("Duración", "Duration"), ("Precio", "Price")],
                       servicios, nota=nota, tono="base", anchor="servicios",
                       boton_=reservar("secondary")),
        bloque_contacto(("Pide tu cita", "Request your appointment"),
                        ("Rellena el formulario y te confirmo la hora en menos de un día laborable. Si es urgente, llama.",
                         "Fill in the form and I will confirm the time within one working day. If it is urgent, call."),
                        campos_cita(servicios=[(s[0][0], s[0][1]) for s in servicios], fecha=True, mensaje=False,
                                    extra=[campo("hora", "time", ("Hora que prefieres", "Preferred time"))]),
                        ("Pedir cita", "Request appointment"), horas, anchor="reservar", tono="relleno",
                        notas=("Si no puedes venir, avísame con un día de antelación para dar tu hueco a otra persona. Política de cancelación de ejemplo: ajústala a la tuya.",
                               "If you cannot come, let me know a day ahead so I can offer your slot to someone else. Example cancellation policy: adjust it to yours."),
                        exito=("Gracias. He recibido tu petición y te confirmo la hora por teléfono o WhatsApp.",
                               "Thank you. I have received your request and will confirm the time by phone or WhatsApp.")),
        bloque_mapa(("Cómo llegar", "How to get here"),
                    ("Estamos en la dirección que ves arriba. Si te cuesta encontrarnos, llama y te guiamos.",
                     "We are at the address shown above. If you have trouble finding us, call and we will guide you."),
                    tono="base"),
    ])

    return plantilla(KEY, ("Reservas sencillas", "Simple bookings"),
                     ("Una sola página para quien solo quiere que le reserven: servicios con duración y precio, formulario de cita, horario, mapa y botones de llamar y WhatsApp.",
                      "A single page for anyone who just wants to be booked: services with duration and price, booking form, opening hours, map and call and WhatsApp buttons."),
                     "web", ("reservas reserva cita previa citas agenda pedir cita pagina de reservas minima autonomo pequeno negocio",
                             "booking bookings appointment appointments schedule book online simple booking page freelancer small business"),
                     LOOK, cabecera_reserva(),
                     pie([("inicio", ("Inicio", "Home"))],
                         ("Atención con cita previa. Pide la tuya por teléfono, WhatsApp o con el formulario.",
                          "By appointment only. Book by phone, WhatsApp or with the form."),
                         ("Lunes a viernes 10:00 a 14:00 y 16:00 a 20:00. Sábado 10:00 a 14:00.",
                          "Monday to Friday 10 am to 2 pm and 4 pm to 8 pm. Saturday 10 am to 2 pm.")),
                     [inicio])
