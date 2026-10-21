# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
from lib import *
from looks import LOOKS

KEY = "blanco"


def build():
    nueva(KEY)
    # La paleta más neutra del catálogo, con otra tipografía que la de tienda-ropa.
    look = dict(LOOKS["tinta"], fuentes=("work-sans", "work-sans"), peso=600)
    nav = [("inicio", ("Inicio", "Home")), ("contacto", ("Contacto", "Contact"))]
    cta_cab = marco([llamar(t=("Llamar", "Call")), whatsapp("ghost", ("WhatsApp", "WhatsApp"))], dir="row", gap=8, align="center")

    inicio = pagina("inicio", ("", ""), ("Inicio", "Home"),
                    ("Página de inicio en blanco con un titular, una frase y un texto guía, lista para escribir lo tuyo.",
                     "Blank home page with a heading, a sentence and a short guide text, ready for you to write your own."), [
        hero(NEGOCIO,
             ("Una frase que diga qué haces y para quién. Escribe aquí la tuya.",
              "One sentence saying what you do and who for. Write yours here."),
             [llamar(), whatsapp()], variante="texto"),
        bloque_texto(("Cómo empezar", "How to start"),
                     [("Texto de ejemplo: cámbialo por el tuyo. Cuenta quién eres, qué ofreces y cómo te pueden encontrar.",
                       "Example text: replace it with yours. Say who you are, what you offer and how people can find you."),
                      ("Desde el editor puedes añadir secciones (servicios, fotos, preguntas frecuentes, tienda) cuando las necesites, y borrar las que no.",
                       "From the editor you can add sections (services, photos, FAQs, a shop) when you need them, and delete the ones you do not.")],
                     anchor="empezar"),
    ])

    cont = pagina("contacto", ("contacto", "contact"), ("Contacto", "Contact"),
                  ("Teléfono, WhatsApp, correo y formulario para que te escriban.",
                   "Phone, WhatsApp, email and a form so people can write to you."), [
        hero(("Contacto", "Contact"),
             ("Llámanos, escríbenos por WhatsApp o usa el formulario.",
              "Call us, message us on WhatsApp or use the form."),
             [llamar(), whatsapp()], variante="texto"),
        bloque_contacto(("Escríbenos", "Write to us"),
                        ("Te respondemos lo antes posible.", "We reply as soon as we can."),
                        campos_consulta(), ("Enviar mensaje", "Send message"), None),
    ])

    return plantilla(KEY, ("En blanco", "Blank"),
                     ("Plantilla mínima sin sector: portada con titular, una frase y un texto guía, y una página de contacto con formulario y botones de llamar y WhatsApp.",
                      "Minimal template with no sector: a home page with a heading, a sentence and a guide text, and a contact page with a form and call and WhatsApp buttons."),
                     "web", ("en blanco blanco vacia desde cero empezar de cero minima sin plantilla",
                             "blank empty from scratch start from nothing minimal no template"),
                     look, cabecera(nav, cta_cab),
                     pie(nav, ("Nombre del negocio: añade aquí una frase corta sobre ti.",
                               "Business name: add a short sentence about you here.")),
                     [inicio, cont],
                     exento={"mapa": "Plantilla sin sector: no sabemos si hay local; el mapa se añade desde el editor si hace falta.",
                             "horario": "Plantilla sin sector: no hay horario que dar por supuesto; se añade desde el editor si hace falta."})
