# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
from lib import *
from looks import LOOKS

KEY = "estetica-unas"


def build():
    nueva(KEY)
    look = LOOKS["lavanda"]
    nav = [("inicio", ("Inicio", "Home")), ("tarifas", ("Tarifas y bonos", "Prices and packs")),
           ("contacto", ("Contacto", "Contact"))]
    horas = [(("Lunes", "Monday"), ("Cerrado", "Closed")),
             (("Martes a viernes", "Tuesday to Friday"),
              ("10:00 a 14:00 y 16:00 a 20:00", "10:00 am to 2:00 pm and 4:00 pm to 8:00 pm")),
             (("Sábado", "Saturday"), ("10:00 a 14:00", "10:00 am to 2:00 pm")),
             (("Domingo", "Sunday"), ("Cerrado", "Closed"))]
    def pedir():
        return boton(("Pedir cita", "Book an appointment"), a_pagina("contacto", "contacto"))
    nota = ("Precios de ejemplo: cámbialos por los tuyos. Incluyen IVA.",
            "Example prices: replace them with your own. VAT included.")
    cab = [("Tratamiento", "Treatment"), ("Precio", "Price")]

    inicio = pagina("inicio", ("", ""), ("Estética y uñas", "Beauty and nails"),
                    ("Centro de estética y uñas con cita previa: manicura, pedicura, depilación y faciales. Llama o escribe por WhatsApp.",
                     "Beauty and nail studio by appointment: manicures, pedicures, waxing and facials. Call or message us on WhatsApp."), [
        hero(("Uñas y piel cuidadas, con calma", "Nails and skin looked after, unhurried"),
             ("Manicura, pedicura, depilación y faciales con cita previa. Te decimos cuánto dura cada tratamiento y cuánto cuesta antes de empezar.",
              "Manicures, pedicures, waxing and facials by appointment. We tell you how long each treatment takes and what it costs before we start."),
             [pedir(), whatsapp()],
             eyebrow=("Centro de estética y uñas", "Beauty and nail studio"),
             imagen_=("photo", ("Cabina de estética con mesa de manicura", "Treatment room with a manicure table"))),
        bloque_valores(("Lo que hacemos", "What we do"), [
            (("Manicura", "Manicure"),
             ("Clásica, semipermanente y uñas de gel. Con diseños sencillos si quieres.",
              "Classic, semi-permanent and gel nails. With simple designs if you like.")),
            (("Pedicura", "Pedicure"),
             ("Cuidado de pies y uñas, con esmaltado normal o semipermanente.",
              "Foot and nail care, with regular or semi-permanent polish.")),
            (("Depilación", "Waxing"),
             ("Con cera, por zonas o completa. Con cita para que nadie te meta prisa.",
              "With wax, by area or full. By appointment so nobody rushes you.")),
            (("Faciales", "Facials"),
             ("Limpieza e hidratación según tu tipo de piel. Antes te preguntamos qué usas y qué te sienta mal.",
              "Cleansing and hydration for your skin type. First we ask what you use and what does not agree with you.")),
        ], cols=4, eyebrow=("Servicios", "Services"), estilo="relleno"),
        bloque_pasos(("Tu primera cita", "Your first appointment"), [
            (("Eliges", "You choose"),
             ("Dinos qué tratamiento quieres. Si dudas, te aconsejamos por teléfono.",
              "Tell us which treatment you want. If you are unsure, we advise you by phone.")),
            (("Reservamos", "We book"),
             ("Te damos día y hora, y el tiempo que ocupará.", "We give you a day and time, and how long it will take.")),
            (("Te preguntamos", "We ask"),
             ("Alergias, pieles sensibles o si estás embarazada. Así elegimos los productos adecuados.",
              "Allergies, sensitive skin or pregnancy. That way we choose suitable products.")),
            (("Disfrutas", "You enjoy"),
             ("Te sientas, te cuidamos y pagas al terminar, con tarjeta o en efectivo.",
              "You sit back, we look after you, and you pay at the end by card or cash.")),
        ], tono="base"),
        bloque_checks(("Higiene", "Hygiene"), [
            ("Material limpio y desinfectado para cada persona", "Clean, disinfected tools for every client"),
            ("Limas y accesorios de un solo uso, que se tiran o son para ti", "Single-use files and accessories, thrown away or yours to keep"),
            ("Cera y productos que se usan una sola vez", "Wax and products used only once"),
            ("Cita previa: sin salas llenas y sin esperas", "By appointment: no crowded rooms, no waiting"),
        ], intro=("Lo primero en un centro de estética es que todo esté limpio. Esto es lo que hacemos en cada cita.",
                  "The first thing in a beauty studio is that everything is clean. This is what we do at every appointment."),
            eyebrow=("Cuidado y limpieza", "Care and cleanliness"), tono="relleno", cols=2),
        bloque_galeria(("Trabajos recientes", "Recent work"), [
            ("Manicura semipermanente realizada en el centro", "Semi-permanent manicure done at the studio"),
            ("Uñas de gel realizadas en el centro", "Gel nails done at the studio"),
            ("Pedicura realizada en el centro", "Pedicure done at the studio"),
            ("Diseño de uñas realizado en el centro", "Nail design done at the studio")],
            cols=4, tono="base"),
        bloque_opiniones(("Lo que dicen nuestras clientas", "What our clients say"),
                         [("la manicura", "the manicure"), ("el facial", "the facial"), ("el trato", "the service")]),
        bloque_cta(("¿Te reservamos hueco?", "Shall we book you a slot?"),
                   ("Dinos el tratamiento y el día que te viene bien y te confirmamos la cita.",
                    "Tell us the treatment and the day that suits you and we will confirm your appointment.")),
    ])

    tarifas = pagina("tarifas", ("tarifas", "prices"), ("Tarifas y bonos", "Prices and packs"),
                     ("Precios de manicura, pedicura, depilación y faciales, y bonos de varias sesiones. Reserva tu cita.",
                      "Prices for manicures, pedicures, waxing and facials, plus multi-session packs. Book your appointment."), [
        hero(("Tarifas y bonos", "Prices and packs"),
             ("Estos son nuestros precios habituales. Los bonos salen más baratos si vienes con frecuencia.",
              "These are our usual prices. Packs work out cheaper if you come often."),
             [pedir()], variante="texto"),
        bloque_carta(("Tarifas por tratamiento", "Prices by treatment"), [
            (("Manicura", "Manicure"), ("Precios de manicura", "Manicure prices"), cab,
             [(("Manicura clásica", "Classic manicure"), eur(12)),
              (("Manicura semipermanente", "Semi-permanent manicure"), eur(20)),
              (("Uñas de gel", "Gel nails"), desde(28)),
              (("Retirada de semipermanente", "Semi-permanent removal"), eur(6))]),
            (("Pedicura", "Pedicure"), ("Precios de pedicura", "Pedicure prices"), cab,
             [(("Pedicura clásica", "Classic pedicure"), eur(25)),
              (("Pedicura semipermanente", "Semi-permanent pedicure"), eur(32))]),
            (("Depilación con cera", "Waxing"), ("Precios de depilación", "Waxing prices"), cab,
             [(("Cejas", "Eyebrows"), eur(6)), (("Labio superior", "Upper lip"), eur(4)),
              (("Axilas", "Underarms"), eur(8)), (("Medias piernas", "Half legs"), eur(14)),
              (("Piernas completas", "Full legs"), eur(22)), (("Ingles", "Bikini line"), desde(12))]),
            (("Faciales", "Facials"), ("Precios de faciales", "Facial prices"), cab,
             [(("Limpieza facial", "Facial cleanse"), eur(35)),
              (("Facial hidratante", "Hydrating facial"), eur(42))]),
        ], nota=nota, tono="base"),
        bloque_precios(("Bonos", "Packs"), ("Precios de bonos", "Pack prices"),
                       [("Bono", "Pack"), ("Precio", "Price")],
                       [(("5 manicuras semipermanentes", "5 semi-permanent manicures"), eur(90)),
                        (("3 limpiezas faciales", "3 facial cleanses"), eur(95)),
                        (("5 sesiones de depilación (zona a elegir)", "5 waxing sessions (area of your choice)"), eur(70))],
                       intro=("Pagas varias sesiones por adelantado y te sale más barato. El bono es personal y tiene fecha de caducidad.",
                              "You pay for several sessions up front and it works out cheaper. The pack is personal and has an expiry date."),
                       nota=nota, anchor="bonos", tono="relleno"),
        bloque_faq(("Preguntas frecuentes", "Frequently asked questions"), [
            (("¿Cuánto dura una manicura?", "How long does a manicure take?"),
             ("Una clásica, unos 30 minutos. Una semipermanente, unos 45. Las de gel, entre 1 hora y 1 hora y media.",
              "A classic one takes about 30 minutes. Semi-permanent, about 45. Gel nails take 1 to 1.5 hours.")),
            (("¿Cada cuánto me conviene ir?", "How often should I come?"),
             ("La semipermanente aguanta unas dos o tres semanas. Te avisamos cuando toca retocar.",
              "Semi-permanent polish lasts about two to three weeks. We tell you when it is time for a touch-up.")),
            (("Tengo la piel sensible o estoy embarazada, ¿puedo hacerme tratamientos?",
              "I have sensitive skin or I am pregnant. Can I have treatments?"),
             ("Dínoslo al reservar. Adaptamos los productos y, si hace falta, te pedimos que lo consultes con tu médico.",
              "Tell us when you book. We adapt the products and, if needed, ask you to check with your doctor.")),
            (("¿Y si no puedo ir a mi cita?", "What if I cannot make my appointment?"),
             ("Avísanos con 24 horas y la movemos sin coste.", "Let us know 24 hours ahead and we move it at no cost.")),
        ], tono="base"),
        bloque_cta(("¿Reservamos tu cita?", "Shall we book your appointment?"), None, tono="relleno"),
    ])

    cont = pagina("contacto", ("contacto", "contact"), ("Contacto y cita", "Contact and appointments"),
                  ("Pide cita por teléfono, WhatsApp o con el formulario. Mira el horario y cómo llegar al centro.",
                   "Book by phone, WhatsApp or with the form. See our opening hours and how to find the studio."), [
        hero(("Pide tu cita", "Book your appointment"),
             ("Llámanos, escríbenos por WhatsApp o rellena el formulario. Te confirmamos la cita el mismo día.",
              "Call us, message us on WhatsApp or fill in the form. We confirm your appointment the same day."),
             [llamar(), whatsapp()], variante="texto"),
        bloque_contacto(("Cómo encontrarnos", "How to find us"),
                        ("Estamos en una calle tranquila del centro, con parada de autobús cerca.",
                         "We are on a quiet street in the centre, near a bus stop."),
                        campos_cita(servicios=[("Manicura", "Manicure"), ("Pedicura", "Pedicure"),
                                               ("Depilación", "Waxing"), ("Facial", "Facial"),
                                               ("Bono", "Pack")],
                                    extra=[campo("alergias", "text", ("Alergias o piel sensible (opcional)",
                                                                       "Allergies or sensitive skin (optional)"),
                                                 False, maxlen=200)]),
                        ("Pedir cita", "Request appointment"), horas,
                        exito=("Gracias. Te confirmamos la cita por teléfono o WhatsApp en cuanto la tengamos.",
                               "Thank you. We will confirm your appointment by phone or WhatsApp as soon as we have it.")),
        bloque_mapa(tono="relleno"),
    ])

    return plantilla(KEY, ("Estética y uñas", "Beauty and nails"),
                     ("Centro de estética y uñas con tarifas por tratamiento, bonos, higiene, cita previa y botón de WhatsApp.",
                      "Beauty and nail studio with prices by treatment, packs, hygiene, appointments and a WhatsApp button."),
                     "belleza", ("estetica unas manicura pedicura depilacion cera facial faciales centro de estetica esteticista bonos semipermanente gel",
                                 "beauty salon nails nail studio manicure pedicure waxing facial beautician semi-permanent gel"),
                     look, cabecera(nav, boton(("Pedir cita", "Book an appointment"), a_pagina("contacto", "contacto"))),
                     pie(nav, ("Centro de estética y uñas con cita previa. Manicura, pedicura, depilación y faciales.",
                               "Beauty and nail studio by appointment. Manicures, pedicures, waxing and facials."),
                         ("Martes a viernes 10:00 a 14:00 y 16:00 a 20:00. Sábado 10:00 a 14:00.",
                          "Tuesday to Friday 10 am to 2 pm and 4 pm to 8 pm. Saturday 10 am to 2 pm.")),
                     [inicio, tarifas, cont])
