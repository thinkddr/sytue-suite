# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
import lib
from lib import *

KEY = "proximamente"

# Paleta propia: azul de noche y melocotón, para una puerta que aún no se ha abierto.
# primary/inverse alto, pero no se usa fondo $inverse.
LOOK = dict(
    colores=dict(background="#0c1c26", surface="#14293a", text="#f6eee8", muted="#aebfcb", primary="#fbbf9a",
                 on_primary="#0c1c26", border="#27434f", inverse="#07121a", on_inverse="#f6eee8"),
    fuentes=("playfair-display", "manrope"), peso=600, h1=(64, 46, 36), radio=(4, 8, 12), ancho=980)


def dato(ico, etiqueta_, valor):
    return marco([icono(ico, sz=26),
                  marco([peq(etiqueta_, color="$muted"), texto("p", valor, weight=600)], gap=2)],
                 dir="row", gap=12, align="center")


def build():
    nueva(KEY)
    malos = lib.comprobar_paleta("proximamente", LOOK["colores"])
    assert not malos, malos

    anclas = [("que-sera", ("Qué será", "What it will be")), ("avisame", ("Avísame", "Notify me")),
              ("donde", ("Dónde", "Where"))]
    cab = cabecera([("inicio", ("Inicio", "Home"))],
                   boton(("Avísame", "Notify me"), a_pagina("inicio", "avisame"), "primary"))
    cab["children"][1]["children"][0]["items"] = [
        {"id": lib._id(), "label": L(t), "link": a_pagina("inicio", a)} for a, t in anclas]

    avisame = lambda var="primary": boton(("Avísame cuando abramos", "Tell me when you open"),
                                          a_pagina("inicio", "avisame"), var)

    inicio = pagina("inicio", ("", ""), ("Próximamente", "Coming soon"),
                    ("Página de espera de un negocio que aún no ha abierto: qué será, fecha estimada, aviso por correo, redes como texto, dirección con mapa y contacto.",
                     "Holding page for a business that has not opened yet: what it will be, estimated date, email notice, social networks as text, address with map and contact."), [
        hero(("Nombre del negocio abre pronto", "Business name opens soon"),
             ("Escribe aquí en dos líneas qué va a ser este sitio y qué se va a encontrar la gente al entrar.",
              "Write here, in two lines, what this place will be and what people will find when they walk in."),
             [avisame(), boton(("Dónde estaremos", "Where we will be"), a_pagina("inicio", "donde"), "ghost")],
             eyebrow=("Próximamente", "Coming soon"), variante="centro", tono="base",
             extra=dato("calendar", ("Fecha de apertura (estimada)", "Opening date (estimated)"),
                        ("Fecha de apertura", "Opening date"))),
        bloque_valores(("Qué será", "What it will be"), [
            (("Lo que ofreceremos", "What we will offer"),
             ("Escribe aquí lo principal: qué venderás o qué servicio darás.",
              "Write here the main thing: what you will sell or what service you will give.")),
            (("Cómo será el sitio", "What the place will be like"),
             ("Escribe aquí cómo es el local o cómo vas a atender: con cita, sin cita, a domicilio.",
              "Write here what the premises are like or how you will serve people: by appointment, walk-in, at home.")),
            (("Para quién", "Who it is for"),
             ("Escribe aquí a quién va dirigido: el barrio, las familias, quien trabaja cerca.",
              "Write here who it is for: the neighbourhood, families, people who work nearby.")),
        ], cols=3, estilo="borde", tono="relleno", anchor="que-sera", eyebrow=("Lo que viene", "What is coming")),
        seccion("contact", [marco([
            titular(("Te avisamos cuando abramos", "We will let you know when we open"),
                    ("Déjanos tu nombre y tu correo y te escribimos el día de la apertura. Nada más.",
                     "Leave us your name and email and we will write to you on opening day. Nothing else."),
                    eyebrow=("Aviso de apertura", "Opening notice"), align="center"),
            tarjeta([formulario([
                campo("nombre", "text", ("Nombre", "Name"), True, auto="name", maxlen=120),
                campo("correo", "email", ("Correo electrónico", "Email"), True, auto="email", maxlen=200),
            ], ("Avísame", "Notify me"),
                ("Gracias. Te escribiremos cuando abramos.", "Thank you. We will write to you when we open."),
                ("Acepto que usen mi correo solo para avisarme de la apertura.",
                 "I agree that my email is used only to tell me about the opening."))],
                estilo="borde", pad=28, tag="div", sz={"width": "fill", "height": "hug", "max_width": 560}),
        ], gap=28, align="center")], anchor="avisame", tono="base", align="center", gap=0),
        seccion("map", [
            titular(("Dónde estaremos", "Where we will be"),
                    ("Si ya tienes local, esta es su dirección. Escribe aquí la dirección real; si aún no la hay, quita esta sección.",
                     "If you already have premises, this is the address. Write here the real address; if there is none yet, remove this section."),
                    eyebrow=("Dirección", "Address")),
            marco([icono("map-pin"), p(CALLE)], dir="row", gap=12, align="start"),
            mapa(("Mapa con la ubicación del futuro local", "Map showing the location of the future premises")),
        ], tono="relleno", anchor="donde", gap=24),
        seccion("text", [marco([
            titular(("Mientras tanto", "In the meantime"),
                    ("Aún no hay horario de atención. Si quieres saber algo antes, escríbenos o llámanos.",
                     "There are no opening hours yet. If you want to know something beforehand, write or call us."),
                    eyebrow=("Contacto y redes", "Contact and social networks"), align="center"),
            marco([p(("Instagram: escribe aquí tu usuario", "Instagram: write your username here"), align="center"),
                   p(("Facebook: escribe aquí el nombre de tu página", "Facebook: write your page name here"), align="center"),
                   p((CORREO, CORREO), align="center")], gap=6, align="center"),
            marco([llamar("secondary"), whatsapp("ghost")], dir="row", gap=12, wrap=True, align="center", justify="center"),
        ], gap=20, align="center", sz={"width": "fill", "height": "hug", "max_width": 640})],
            tono="base", align="center", gap=0),
    ])

    return plantilla(KEY, ("Próximamente", "Coming soon"),
                     ("Página de espera de una sola pantalla larga para un negocio que aún no ha abierto: qué será, fecha de apertura, aviso por correo, dirección con mapa, redes como texto y botones de llamar.",
                      "Holding page for a business that has not opened yet: what it will be, opening date, email notice, address with map, social networks as text and call buttons."),
                     "web", ("proximamente pronto abrimos apertura inauguracion nuevo negocio pagina de espera aviso de apertura lista de espera una pagina en construccion",
                             "coming soon opening soon grand opening new business holding page launch notice waiting list single page under construction"),
                     LOOK, cab,
                     pie(anclas, ("Abrimos pronto. Déjanos tu correo y te avisamos.",
                                  "We open soon. Leave us your email and we will tell you.")),
                     [inicio],
                     exento={"horario": "Aún no ha abierto: no hay horario de apertura que dar; se añade desde el editor cuando abra."})
