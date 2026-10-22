# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
from lib import *
from looks import LOOKS

KEY = "peluqueria"


def build():
    nueva(KEY)
    look = LOOKS["rosa"]
    nav = [("inicio", ("Inicio", "Home")), ("servicios", ("Servicios y precios", "Services and prices")),
           ("contacto", ("Contacto", "Contact"))]
    horas = [(("Lunes", "Monday"), ("Cerrado", "Closed")),
             (("Martes a viernes", "Tuesday to Friday"), ("9:30 a 20:00", "9:30 am to 8:00 pm")),
             (("Sábado", "Saturday"), ("9:00 a 14:00", "9:00 am to 2:00 pm")),
             (("Domingo", "Sunday"), ("Cerrado", "Closed"))]
    cta_cab = boton(("Pedir cita", "Book an appointment"), a_pagina("contacto", "contacto"))

    inicio = pagina("inicio", ("", ""), ("Peluquería", "Hair salon"),
                    ("Peluquería con cita previa: corte, color, tratamientos y peinados. Llama o escribe por WhatsApp.",
                     "Hair salon by appointment: cuts, colour, treatments and styling. Call or message us on WhatsApp."), [
        hero(("Tu pelo, en buenas manos", "Your hair, in good hands"),
             ("Corte, color y peinado con cita previa. Te escuchamos, te aconsejamos y te vas con un resultado que sabes mantener en casa.",
              "Cut, colour and styling by appointment. We listen, we advise, and you leave with a look you can keep up at home."),
             [boton(("Pedir cita", "Book an appointment"), a_pagina("contacto", "contacto")), whatsapp()],
             eyebrow=("Peluquería y estilismo", "Hair and styling"),
             imagen_=("photo", ("Interior de la peluquería", "Inside the hair salon"))),
        bloque_valores(("Por qué vienen a vernos", "Why people come to see us"), [
            (("Con cita, sin esperas", "By appointment, no waiting"),
             ("Reservamos el tiempo que necesita tu servicio. Llegas y te atendemos.",
              "We set aside the time your service needs. You arrive and we look after you.")),
            (("Consejo sincero", "Honest advice"),
             ("Antes de empezar te decimos qué te favorece, cuánto durará y cuánto costará.",
              "Before we start we tell you what suits you, how long it will last and what it will cost.")),
            (("Productos que cuidan", "Products that care"),
             ("Trabajamos con marcas profesionales y te explicamos cuáles usar en casa y cuáles no hacen falta.",
              "We use professional brands and explain which ones to use at home and which you can skip.")),
        ], tono="relleno", estilo="borde"),
        bloque_valores(("Lo que hacemos", "What we do"), [
            (("Corte", "Cuts"), ("Mujer, hombre y niños. Con o sin lavado y secado.", "Women, men and children. With or without wash and blow-dry.")),
            (("Color", "Colour"), ("Tinte, mechas, balayage y retoque de raíz con prueba previa.", "Colour, highlights, balayage and root touch-ups, with a patch test first.")),
            (("Tratamientos", "Treatments"), ("Hidratación, reparación y keratina según lo que necesite tu pelo.", "Hydration, repair and keratin, depending on what your hair needs.")),
            (("Peinados", "Styling"), ("Para eventos, bodas y comuniones, con prueba si lo prefieres.", "For events, weddings and communions, with a trial if you prefer.")),
        ], cols=4, eyebrow=("Servicios", "Services"), estilo="relleno"),
        bloque_galeria(("Trabajos recientes", "Recent work"), [
            ("Corte y peinado realizado en la peluquería", "Cut and styling done at the salon"),
            ("Color y mechas realizados en la peluquería", "Colour and highlights done at the salon"),
            ("Peinado de evento realizado en la peluquería", "Event styling done at the salon"),
            ("Corte de melena realizado en la peluquería", "Bob cut done at the salon"),
            ("Tratamiento de brillo realizado en la peluquería", "Shine treatment done at the salon"),
            ("Recogido realizado en la peluquería", "Updo done at the salon")], tono="base"),
        bloque_opiniones(("Lo que dicen nuestras clientas", "What our clients say"),
                         [("el corte", "the cut"), ("el color", "the colour"), ("el trato", "the service")]),
        bloque_cta(("¿Reservamos tu hueco?", "Shall we book your slot?"),
                   ("Dinos el servicio y el día que te viene bien y te confirmamos la cita.",
                    "Tell us the service and the day that suits you and we will confirm your appointment.")),
    ])

    serv = pagina("servicios", ("servicios", "services"), ("Servicios y precios", "Services and prices"),
                  ("Precios de corte, color, tratamientos y peinados. Consulta las tarifas y reserva tu cita.",
                   "Prices for cuts, colour, treatments and styling. Check our rates and book your appointment."), [
        hero(("Servicios y precios", "Services and prices"),
             ("Estos son nuestros precios habituales. Si tu pelo es muy largo o muy abundante, te lo decimos antes de empezar.",
              "These are our usual prices. If your hair is very long or very thick, we tell you before we start."),
             [boton(("Pedir cita", "Book an appointment"), a_pagina("contacto", "contacto"))], variante="texto"),
        bloque_precios(("Corte", "Cuts"), ("Precios de corte", "Cut prices"),
                       [("Servicio", "Service"), ("Precio", "Price")],
                       [(("Corte mujer", "Women's cut"), eur(22)), (("Corte hombre", "Men's cut"), eur(14)),
                        (("Corte niño (hasta 12 años)", "Children's cut (up to 12)"), eur(10)),
                        (("Lavado y secado", "Wash and blow-dry"), desde(12))],
                       anchor="corte", tono="base"),
        bloque_precios(("Color y tratamientos", "Colour and treatments"), ("Precios de color y tratamientos", "Colour and treatment prices"),
                       [("Servicio", "Service"), ("Precio", "Price")],
                       [(("Tinte de raíz", "Root colour"), eur(35)), (("Tinte completo", "Full colour"), desde(45)),
                        (("Mechas", "Highlights"), desde(60)), (("Balayage", "Balayage"), desde(95)),
                        (("Tratamiento de hidratación", "Hydration treatment"), eur(18)),
                        (("Keratina", "Keratin treatment"), desde(80))],
                       nota=("Precios de ejemplo: cámbialos por los tuyos. Incluyen IVA.",
                             "Example prices: replace them with your own. VAT included."),
                       anchor="color", tono="relleno"),
        bloque_precios(("Peinados y eventos", "Styling and events"), ("Precios de peinados", "Styling prices"),
                       [("Servicio", "Service"), ("Precio", "Price")],
                       [(("Peinado de fiesta", "Party styling"), desde(30)), (("Recogido", "Updo"), desde(40)),
                        (("Novia (con prueba)", "Bride (with a trial)"), desde(120))],
                       anchor="peinados", tono="base"),
        bloque_faq(("Preguntas frecuentes", "Frequently asked questions"), [
            (("¿Cuánto dura un servicio?", "How long does a service take?"),
             ("Un corte, unos 45 minutos. Un color, entre 1 hora y 2 horas y media según el trabajo. Te damos el tiempo exacto al reservar.",
              "A cut takes about 45 minutes. Colour takes between 1 and 2.5 hours depending on the work. We give you the exact time when you book.")),
            (("¿Tengo que hacerme una prueba de alergia?", "Do I need an allergy test?"),
             ("Si es tu primer tinte con nosotros, sí: es gratuita y se hace 48 horas antes. Es por tu seguridad.",
              "If it is your first colour with us, yes: it is free and done 48 hours before. It is for your safety.")),
            (("¿Y si no puedo ir?", "What if I cannot make it?"),
             ("Avísanos con 24 horas y movemos la cita sin coste. Así otra persona puede aprovechar tu hueco.",
              "Let us know 24 hours ahead and we will move your appointment at no cost. That way someone else can use your slot.")),
        ], tono="relleno"),
        bloque_cta(("¿Te reservamos cita?", "Shall we book you in?"), None, tono="base"),
    ])

    cont = pagina("contacto", ("contacto", "contact"), ("Contacto y cita", "Contact and appointments"),
                  ("Reserva tu cita por teléfono, WhatsApp o con el formulario. Mira cómo llegar a la peluquería.",
                   "Book your appointment by phone, WhatsApp or with the form. See how to get to the salon."), [
        hero(("Reserva tu cita", "Book your appointment"),
             ("Llámanos, escríbenos por WhatsApp o rellena el formulario. Te confirmamos el mismo día.",
              "Call us, message us on WhatsApp or fill in the form. We confirm the same day."),
             [llamar(), whatsapp()], variante="texto"),
        bloque_contacto(("Cómo encontrarnos", "How to find us"),
                        ("Estamos en el centro, con parada de autobús cerca.", "We are in the town centre, near a bus stop."),
                        campos_cita(servicios=[("Corte", "Cut"), ("Color", "Colour"), ("Tratamiento", "Treatment"),
                                               ("Peinado", "Styling")]),
                        ("Pedir cita", "Request appointment"), horas,
                        exito=("Gracias. Te confirmamos la cita por teléfono o WhatsApp en cuanto la tengamos.",
                               "Thank you. We will confirm your appointment by phone or WhatsApp as soon as we have it.")),
        bloque_mapa(tono="relleno"),
    ])

    return plantilla(KEY, ("Peluquería", "Hair salon"),
                     ("Cortes, color y peinados con cita previa, precios claros y botón de WhatsApp.",
                      "Cuts, colour and styling by appointment, clear prices and a WhatsApp button."),
                     "belleza", ("peluqueria pelu peluquero peluquera cortes tinte mechas peinados cabello",
                                 "hairdresser hair salon haircut colour highlights styling"),
                     look, cabecera(nav, cta_cab),
                     pie(nav, ("Peluquería con cita previa. Corte, color, tratamientos y peinados.",
                               "Hair salon by appointment. Cuts, colour, treatments and styling."),
                         ("Martes a viernes 9:30 a 20:00. Sábado 9:00 a 14:00.", "Tuesday to Friday 9:30 am to 8 pm. Saturday 9 am to 2 pm.")),
                     [inicio, serv, cont])
