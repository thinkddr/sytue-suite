# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
from lib import *
from looks import LOOKS

KEY = "spa-masajes"


def build():
    nueva(KEY)
    look = LOOKS["salvia"]
    nav = [("inicio", ("Inicio", "Home")), ("masajes", ("Masajes", "Massages")),
           ("spa", ("El spa", "The spa")), ("contacto", ("Contacto", "Contact"))]
    horas = [(("Lunes a viernes", "Monday to Friday"),
              ("10:00 a 21:00", "10:00 am to 9:00 pm")),
             (("Sábado", "Saturday"), ("10:00 a 20:00", "10:00 am to 8:00 pm")),
             (("Domingo", "Sunday"), ("Cerrado", "Closed"))]
    def reservar():
        return boton(("Reservar", "Book"), a_pagina("contacto", "contacto"))
    nota = ("Precios de ejemplo: cámbialos por los tuyos. Incluyen IVA.",
            "Example prices: replace them with your own. VAT included.")

    inicio = pagina("inicio", ("", ""), ("Spa y masajes", "Spa and massage"),
                    ("Spa y masajes con cita previa: masajes de 30 a 90 minutos, circuito de aguas y bonos regalo. Llama o escribe por WhatsApp.",
                     "Spa and massage by appointment: 30 to 90 minute massages, a water circuit and gift vouchers. Call or message us on WhatsApp."), [
        hero(("Un rato para ti", "Time for yourself"),
             ("Masajes y circuito de aguas con cita previa, en un ambiente tranquilo. Elige el que te apetezca y nosotros nos encargamos del resto.",
              "Massages and a water circuit by appointment, in a quiet setting. Pick what you fancy and we take care of the rest."),
             [reservar(), whatsapp()],
             eyebrow=("Spa y masajes", "Spa and massage"), variante="centro"),
        bloque_texto(("Despacio, sin prisas", "Slowly, no rush"), [
            ("Aquí vienes a parar. Reservamos una cabina o una hora de circuito para ti y no mezclamos grupos grandes.",
             "You come here to slow down. We set a treatment room or a circuit slot aside for you and we do not mix large groups."),
            ("Antes de empezar te preguntamos cómo vienes, qué zonas prefieres que trabajemos y si hay algo que debamos evitar.",
             "Before we start we ask how you are feeling, which areas you would like us to work on and whether there is anything to avoid."),
        ], imagen_=("landscape", ("Cabina de masaje con velas y toallas", "Massage room with candles and towels")),
            tono="relleno"),
        bloque_valores(("Masajes", "Massages"), [
            (("Relajante", "Relaxing"),
             ("Movimientos suaves y continuos para soltar el cuerpo.", "Gentle, continuous strokes to let the body unwind.")),
            (("Descontracturante", "Deep tissue"),
             ("Presión firme en cuello, hombros y espalda, donde más cargas.",
              "Firm pressure on neck, shoulders and back, where you carry the most tension.")),
            (("En pareja", "Couples"),
             ("Dos camillas en la misma cabina, con el mismo masaje o con distinto.",
              "Two tables in the same room, with the same massage or different ones.")),
        ], eyebrow=("Lo que te ofrecemos", "What we offer"), estilo="borde", tono="base"),
        bloque_pasos(("Tu visita, paso a paso", "Your visit, step by step"), [
            (("Llegas", "You arrive"),
             ("Ven diez minutos antes. Te damos toalla y taquilla y te enseñamos el spa.",
              "Come ten minutes early. We give you a towel and a locker and show you around.")),
            (("Te contamos", "We talk"),
             ("Nos dices cómo vienes y si hay molestias, alergias a aceites o embarazo.",
              "Tell us how you are and about any discomfort, oil allergies or pregnancy.")),
            (("Te cuidamos", "We look after you"),
             ("El masaje o el circuito, a tu ritmo y con la temperatura que te guste.",
              "The massage or the circuit, at your pace and at the temperature you like.")),
            (("Te quedas un rato", "You stay a while"),
             ("Al terminar puedes descansar en la sala de relax antes de irte.",
              "When you finish you can rest in the relaxation room before leaving.")),
        ], tono="relleno"),
        bloque_opiniones(("Lo que dicen nuestros clientes", "What our customers say"),
                         [("el masaje", "the massage"), ("el circuito", "the circuit"), ("el trato", "the service")],
                         tono="base"),
        bloque_cta(("Regala un rato de calma", "Give someone a moment of calm"),
                   ("Los bonos regalo valen para cualquier masaje o para el circuito. Te los preparamos el mismo día.",
                    "Gift vouchers work for any massage or for the circuit. We prepare them the same day."),
                   [boton(("Ver bonos regalo", "See gift vouchers"), a_pagina("spa", "bonos")), whatsapp("ghost")]),
    ])

    masajes = pagina("masajes", ("masajes", "massages"), ("Masajes y tarifas", "Massages and prices"),
                     ("Tipos de masaje con duración y precio: relajante, descontracturante, en pareja y más. Reserva tu cita.",
                      "Massage types with duration and price: relaxing, deep tissue, couples and more. Book your appointment."), [
        hero(("Masajes y tarifas", "Massages and prices"),
             ("Elige el masaje y la duración. Si no sabes cuál te conviene, llámanos y te orientamos.",
              "Choose the massage and its length. If you are not sure which suits you, call us and we will guide you."),
             [reservar()], variante="texto"),
        bloque_precios(("Masaje individual", "Single massage"), ("Precios de masaje individual", "Single massage prices"),
                       [("Masaje", "Massage"), ("Duración", "Length"), ("Precio", "Price")],
                       [(("Relajante", "Relaxing"), ("30 min", "30 min"), eur(35)),
                        (("Relajante", "Relaxing"), ("60 min", "60 min"), eur(55)),
                        (("Descontracturante", "Deep tissue"), ("45 min", "45 min"), eur(50)),
                        (("Descontracturante", "Deep tissue"), ("60 min", "60 min"), eur(62)),
                        (("Espalda, cuello y hombros", "Back, neck and shoulders"), ("30 min", "30 min"), eur(38)),
                        (("Piernas cansadas", "Tired legs"), ("30 min", "30 min"), eur(36)),
                        (("Masaje con piedras calientes", "Hot stone massage"), ("75 min", "75 min"), eur(75))],
                       nota=nota, anchor="individual", tono="base"),
        bloque_precios(("Masaje en pareja", "Couples massage"), ("Precios de masaje en pareja", "Couples massage prices"),
                       [("Masaje", "Massage"), ("Duración", "Length"), ("Precio por pareja", "Price per couple")],
                       [(("Relajante en pareja", "Couples relaxing"), ("60 min", "60 min"), eur(105)),
                        (("Relajante en pareja", "Couples relaxing"), ("90 min", "90 min"), eur(150))],
                       nota=nota, anchor="pareja", tono="relleno"),
        bloque_checks(("Conviene saber", "Good to know"), [
            ("Un masaje relaja, pero no sustituye a un tratamiento médico.", "A massage relaxes, but it does not replace medical treatment."),
            ("Si estás embarazada, tienes una lesión o tomas medicación, dínoslo al reservar.", "If you are pregnant, injured or on medication, tell us when you book."),
            ("Te dejamos elegir la presión y la música.", "You choose the pressure and the music."),
            ("La toalla va incluida.", "A towel is included."),
        ], tono="base", cols=2),
        bloque_faq(("Preguntas frecuentes", "Frequently asked questions"), [
            (("¿Qué masaje me conviene?", "Which massage suits me?"),
             ("Si buscas soltarte y descansar, el relajante. Si cargas cuello y espalda, el descontracturante. Si dudas, te lo preguntamos al llegar.",
              "If you want to unwind and rest, the relaxing one. If you carry tension in neck and back, deep tissue. If you are unsure, we ask when you arrive.")),
            (("¿Tengo que desnudarme?", "Do I have to undress?"),
             ("Solo hasta donde te sientas cómodo. Te tapamos con una toalla y solo descubrimos la zona que se trabaja.",
              "Only as far as you feel comfortable. We cover you with a towel and uncover only the area being worked on.")),
            (("¿Puedo cancelar?", "Can I cancel?"),
             ("Sí, sin coste hasta 24 horas antes. Con menos aviso, nos quedamos con la mitad del precio.",
              "Yes, at no cost up to 24 hours before. With less notice, we keep half the price.")),
        ], tono="relleno"),
        bloque_cta(("¿Reservamos tu masaje?", "Shall we book your massage?"), None, tono="base"),
    ])

    spa = pagina("spa", ("el-spa", "the-spa"), ("El spa", "The spa"),
                 ("Circuito de aguas, bonos regalo y normas de la casa. Lo que necesitas saber antes de venir.",
                  "Water circuit, gift vouchers and house rules. What you need to know before you come."), [
        hero(("El spa", "The spa"),
             ("Un circuito de aguas para hacer a tu ritmo, y una sala de relax para quedarte después.",
              "A water circuit to do at your own pace, and a relaxation room to stay in afterwards."),
             [reservar(), llamar("secondary")], variante="texto"),
        bloque_pasos(("El circuito de aguas", "The water circuit"), [
            (("Ducha", "Shower"), ("Empiezas con una ducha templada.", "You start with a warm shower.")),
            (("Piscina y jacuzzi", "Pool and jacuzzi"), ("Agua templada y chorros para soltar el cuerpo.", "Warm water and jets to loosen up.")),
            (("Sauna y vapor", "Sauna and steam room"), ("Calor seco y calor húmedo, el tiempo que te apetezca.", "Dry heat and steam, for as long as you like.")),
            (("Relax", "Rest"), ("Tumbonas, infusiones y silencio.", "Loungers, herbal teas and silence.")),
        ], intro=("Dura unos 90 minutos. Vas de una zona a otra sin orden obligado.",
                  "It lasts about 90 minutes. You move between areas in any order."), tono="base"),
        bloque_precios(("Circuito y combinados", "Circuit and combos"), ("Precios del circuito", "Circuit prices"),
                       [("Opción", "Option"), ("Precio", "Price")],
                       [(("Circuito de aguas (90 min)", "Water circuit (90 min)"), eur(28)),
                        (("Circuito y masaje relajante de 30 min", "Circuit and 30 min relaxing massage"), eur(58)),
                        (("Circuito y masaje relajante de 60 min", "Circuit and 60 min relaxing massage"), eur(78))],
                       nota=nota, anchor="circuito", tono="relleno"),
        bloque_valores(("Bonos regalo", "Gift vouchers"), [
            (("Para un masaje", "For a massage"),
             ("Eliges el masaje y la duración, y quien lo recibe elige el día.",
              "You pick the massage and its length, and the recipient picks the day.")),
            (("Para el circuito", "For the circuit"),
             ("Una entrada al circuito de aguas, sola o para dos personas.",
              "One entry to the water circuit, alone or for two.")),
            (("Por un importe", "For an amount"),
             ("Una cantidad libre que se gasta en lo que quiera.", "A free amount to spend on whatever they like.")),
        ], intro=("Te lo preparamos impreso o por correo el mismo día. Tiene fecha de caducidad y se indica en el bono.",
                  "We prepare it printed or by email the same day. It has an expiry date, shown on the voucher."),
            anchor="bonos", tono="base", estilo="relleno"),
        bloque_checks(("Normas de la casa", "House rules"), [
            ("Llega diez minutos antes de tu hora.", "Arrive ten minutes before your slot."),
            ("Trae bañador y chanclas; la toalla va incluida.", "Bring swimwear and flip-flops; a towel is included."),
            ("Pon el móvil en silencio y deja que el resto descanse.", "Silence your phone and let others rest."),
            ("Avísanos de embarazo, lesiones o alergias antes de empezar.", "Tell us about pregnancy, injuries or allergies before we start."),
            ("Las personas menores vienen con un adulto.", "Minors come with an adult."),
            ("Cancelación gratuita hasta 24 horas antes.", "Free cancellation up to 24 hours before."),
        ], intro=("Pocas y sencillas, para que todos descansen a gusto.",
                  "Few and simple, so everyone can rest easy."), tono="relleno", cols=2, anchor="normas"),
        bloque_cta(("¿Vienes un rato?", "Will you come for a while?"), None, [reservar(), whatsapp("ghost")], tono="base"),
    ])

    cont = pagina("contacto", ("contacto", "contact"), ("Contacto y reservas", "Contact and bookings"),
                  ("Reserva masaje o circuito por teléfono, WhatsApp o con el formulario. Horario y cómo llegar al spa.",
                   "Book a massage or the circuit by phone, WhatsApp or with the form. Opening hours and how to find the spa."), [
        hero(("Reserva tu momento", "Book your moment"),
             ("Llámanos, escríbenos por WhatsApp o rellena el formulario. Te confirmamos el mismo día.",
              "Call us, message us on WhatsApp or fill in the form. We confirm the same day."),
             [llamar(), whatsapp()], variante="texto"),
        bloque_contacto(("Cómo encontrarnos", "How to find us"),
                        ("Tienes aparcamiento cerca y parada de autobús a un paso.",
                         "There is parking nearby and a bus stop just around the corner."),
                        campos_cita(servicios=[("Masaje relajante", "Relaxing massage"),
                                               ("Masaje descontracturante", "Deep tissue massage"),
                                               ("Masaje en pareja", "Couples massage"),
                                               ("Circuito de aguas", "Water circuit"),
                                               ("Bono regalo", "Gift voucher")],
                                    extra=[campo("avisos", "text", ("Embarazo, lesiones o alergias (opcional)",
                                                                     "Pregnancy, injuries or allergies (optional)"),
                                                 False, maxlen=200)]),
                        ("Pedir cita", "Request appointment"), horas,
                        exito=("Gracias. Te confirmamos la reserva por teléfono o WhatsApp en cuanto la tengamos.",
                               "Thank you. We will confirm your booking by phone or WhatsApp as soon as we have it.")),
        bloque_mapa(tono="relleno"),
    ])

    return plantilla(KEY, ("Spa y masajes", "Spa and massage"),
                     ("Spa y masajes con tarifas por duración, circuito de aguas, bonos regalo, normas de la casa y reserva por WhatsApp.",
                      "Spa and massage with prices by length, water circuit, gift vouchers, house rules and WhatsApp booking."),
                     "belleza", ("spa masajes masaje masajista circuito aguas relax relajante descontracturante pareja bono regalo balneario",
                                 "spa massage masseur masseuse circuit relaxation deep tissue couples gift voucher wellness"),
                     look, cabecera(nav, boton(("Reservar", "Book"), a_pagina("contacto", "contacto"))),
                     pie(nav, ("Spa y masajes con cita previa. Masajes, circuito de aguas y bonos regalo.",
                               "Spa and massage by appointment. Massages, water circuit and gift vouchers."),
                         ("Lunes a viernes 10:00 a 21:00. Sábado 10:00 a 20:00.",
                          "Monday to Friday 10 am to 9 pm. Saturday 10 am to 8 pm.")),
                     [inicio, masajes, spa, cont])
