# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
from lib import *
from looks import LOOKS

KEY = "barberia"


def build():
    nueva(KEY)
    look = LOOKS["barberia"]
    nav = [("inicio", ("Inicio", "Home")), ("tarifas", ("Tarifas", "Prices")),
           ("contacto", ("Contacto", "Contact"))]
    horas = [(("Lunes", "Monday"), ("Cerrado", "Closed")),
             (("Martes a viernes", "Tuesday to Friday"),
              ("10:00 a 14:00 y 16:00 a 20:30", "10:00 am to 2:00 pm and 4:00 pm to 8:30 pm")),
             (("Sábado", "Saturday"), ("9:30 a 15:00", "9:30 am to 3:00 pm")),
             (("Domingo", "Sunday"), ("Cerrado", "Closed"))]
    def reservar():
        return whatsapp("primary", ("Reservar por WhatsApp", "Book on WhatsApp"))
    nota_precios = ("Precios de ejemplo: cámbialos por los tuyos. Incluyen IVA.",
                    "Example prices: replace them with your own. VAT included.")
    cab = [("Servicio", "Service"), ("Precio", "Price")]

    inicio = pagina("inicio", ("", ""), ("Barbería", "Barber shop"),
                    ("Barbería de barrio: corte, barba y afeitado a navaja. Precios claros y reserva por WhatsApp.",
                     "Neighbourhood barber shop: haircuts, beard trims and razor shaves. Clear prices and booking on WhatsApp."), [
        hero(("Corte, barba y navaja", "Haircut, beard and razor"),
             ("Barbería de barrio. Sin rodeos: te sientas, dices cómo lo quieres y sales con el pelo y la barba como toca.",
              "Neighbourhood barber shop. No fuss: you sit down, tell us how you want it and leave with your hair and beard just right."),
             [reservar(), llamar("secondary")],
             eyebrow=("Barbería", "Barber shop"),
             imagen_=("photo", ("Sillón de barbero y espejo en la barbería", "Barber chair and mirror in the barber shop"))),
        bloque_checks(("Lo que hacemos", "What we do"), [
            ("Corte a tijera y a máquina", "Scissor and clipper cuts"),
            ("Degradados y cortes al uno", "Fades and buzz cuts"),
            ("Arreglo y perfilado de barba", "Beard trim and shaping"),
            ("Afeitado a navaja con toalla caliente", "Razor shave with a hot towel"),
            ("Corte infantil", "Children's cuts"),
            ("Cejas y contornos", "Eyebrows and edging"),
        ], eyebrow=("Servicios", "Services"), tono="relleno", cols=2),
        bloque_precios(("Lo básico y lo que cuesta", "The basics and what they cost"),
                       ("Precios principales", "Main prices"), cab,
                       [(("Corte", "Haircut"), eur(14)), (("Barba", "Beard"), eur(10)),
                        (("Corte y barba", "Haircut and beard"), eur(22))],
                       nota=nota_precios,
                       boton_=boton(("Ver todas las tarifas", "See all prices"), a_pagina("tarifas"), "secondary"),
                       anchor="precios", tono="base"),
        bloque_equipo(("Quién te corta", "Who cuts your hair"), [
            (("Barbero", "Barber"), ("Cortes clásicos y a tijera.", "Classic and scissor cuts.")),
            (("Barbero", "Barber"), ("Degradados y máquina.", "Fades and clipper work.")),
            (("Barbero", "Barber"), ("Barba y afeitado a navaja.", "Beards and razor shaves.")),
        ], tono="relleno"),
        bloque_pasos(("Reservar es un mensaje", "Booking is one message"), [
            (("Escríbenos", "Message us"),
             ("Dinos qué quieres y qué día te viene bien. Con un WhatsApp basta.",
              "Tell us what you want and which day suits you. A WhatsApp is enough.")),
            (("Te damos hueco", "We find you a slot"),
             ("Te contestamos con la hora libre más cercana a la que pides.",
              "We reply with the free time closest to the one you ask for.")),
            (("Te sientas y listo", "Sit down and relax"),
             ("Llegas a tu hora y te atendemos sin esperas.", "Arrive on time and we look after you, no waiting.")),
        ], tono="base"),
        bloque_opiniones(("Lo que dicen los clientes", "What customers say"),
                         [("el corte", "the cut"), ("la barba", "the beard"), ("el trato", "the service")]),
        bloque_cta(("¿Te guardamos silla?", "Shall we save you a chair?"),
                   ("Escríbenos o llámanos y te decimos cuándo tenemos hueco.",
                    "Message or call us and we will tell you when we have a slot."),
                   [reservar(), llamar("secondary")], tono="oscuro"),
    ])

    tarifas = pagina("tarifas", ("tarifas", "prices"), ("Tarifas", "Prices"),
                     ("Precios de corte, barba, afeitado a navaja y combinados. Consulta las tarifas y reserva por WhatsApp.",
                      "Prices for haircuts, beards, razor shaves and combos. Check our rates and book on WhatsApp."), [
        hero(("Tarifas", "Prices"),
             ("Lo que cuesta cada cosa, sin letra pequeña. Si algo lleva más trabajo de lo normal, te lo decimos antes de empezar.",
              "What each thing costs, no small print. If something takes more work than usual, we tell you before we start."),
             [reservar()], variante="texto"),
        bloque_carta(("Carta de servicios", "Service list"), [
            (("Pelo", "Hair"), ("Precios de pelo", "Hair prices"), cab,
             [(("Corte a tijera o máquina", "Scissor or clipper cut"), eur(14)),
              (("Degradado", "Fade"), eur(16)),
              (("Corte infantil (hasta 12 años)", "Children's cut (up to 12)"), eur(10)),
              (("Cejas y contornos", "Eyebrows and edging"), eur(4))]),
            (("Barba y afeitado", "Beard and shave"), ("Precios de barba y afeitado", "Beard and shave prices"), cab,
             [(("Arreglo de barba", "Beard trim"), eur(10)),
              (("Afeitado a navaja con toalla caliente", "Razor shave with a hot towel"), eur(14)),
              (("Perfilado de barba con navaja", "Beard shaping with a razor"), eur(12))]),
            (("Combinados", "Combos"), ("Precios de combinados", "Combo prices"), cab,
             [(("Corte y barba", "Haircut and beard"), eur(22)),
              (("Corte y afeitado a navaja", "Haircut and razor shave"), eur(26)),
              (("Padre e hijo", "Father and son"), eur(22))]),
        ], nota=nota_precios, tono="base"),
        bloque_faq(("Preguntas frecuentes", "Frequently asked questions"), [
            (("¿Puedo ir sin cita?", "Can I walk in?"),
             ("Sí. Si hay silla libre, te sientas; si no, te decimos cuánto toca esperar. Con cita no esperas.",
              "Yes. If a chair is free you sit down; if not, we tell you how long the wait is. With a booking there is no wait.")),
            (("¿Cuánto dura un corte?", "How long does a haircut take?"),
             ("Un corte, unos 30 minutos. Corte y barba, unos 45. Un afeitado a navaja, unos 30.",
              "A haircut takes about 30 minutes. Haircut and beard, about 45. A razor shave, about 30.")),
            (("¿Qué hago si llego tarde o no puedo ir?", "What if I am late or cannot come?"),
             ("Avísanos por WhatsApp. Si vas a tardar más de diez minutos, movemos tu hora para no retrasar a los demás.",
              "Message us on WhatsApp. If you will be more than ten minutes late, we move your slot so we do not hold up the others.")),
            (("¿Se puede pagar con tarjeta?", "Can I pay by card?"),
             ("Sí, con tarjeta, Bizum o en efectivo. Te damos factura si la necesitas.",
              "Yes, by card, Bizum or cash. We give you an invoice if you need one.")),
        ], tono="relleno"),
        bloque_cta(("¿Qué día te viene bien?", "Which day suits you?"), None, [reservar(), llamar("secondary")],
                   tono="base"),
    ])

    cont = pagina("contacto", ("contacto", "contact"), ("Contacto y reservas", "Contact and bookings"),
                  ("Reserva por WhatsApp, por teléfono o con el formulario. Horario y cómo llegar a la barbería.",
                   "Book on WhatsApp, by phone or with the form. Opening hours and how to find the barber shop."), [
        hero(("Reserva tu hora", "Book your slot"),
             ("Lo más rápido es un WhatsApp. También puedes llamar o dejar tus datos en el formulario.",
              "The quickest way is a WhatsApp. You can also call or leave your details in the form."),
             [reservar(), llamar("secondary")], variante="texto"),
        bloque_contacto(("Dónde estamos", "Where we are"),
                        ("Estamos en el barrio, con aparcamiento cerca.", "We are in the neighbourhood, with parking nearby."),
                        campos_cita(servicios=[("Corte", "Haircut"), ("Barba", "Beard"),
                                               ("Corte y barba", "Haircut and beard"),
                                               ("Afeitado a navaja", "Razor shave")]),
                        ("Pedir hora", "Request a slot"), horas,
                        exito=("Gracias. Te confirmamos la hora por WhatsApp o por teléfono en cuanto la tengamos.",
                               "Thank you. We will confirm your slot by WhatsApp or phone as soon as we have it.")),
        bloque_mapa(tono="relleno"),
    ])

    return plantilla(KEY, ("Barbería", "Barber shop"),
                     ("Barbería de barrio con tarifas claras, equipo, reserva por WhatsApp y botón de llamar.",
                      "Neighbourhood barber shop with clear prices, team, WhatsApp booking and a call button."),
                     "belleza", ("barberia barbero barberos corte barba afeitado navaja degradado fade pelo hombre caballeros",
                                 "barber barbershop barber shop haircut beard razor shave fade men"),
                     look, cabecera(nav, llamar()),
                     pie(nav, ("Barbería de barrio. Corte, barba y afeitado a navaja.",
                               "Neighbourhood barber shop. Haircuts, beards and razor shaves."),
                         ("Martes a viernes 10:00 a 14:00 y 16:00 a 20:30. Sábado 9:30 a 15:00.",
                          "Tuesday to Friday 10 am to 2 pm and 4 pm to 8:30 pm. Saturday 9:30 am to 3 pm.")),
                     [inicio, tarifas, cont])
