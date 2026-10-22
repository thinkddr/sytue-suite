# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
from lib import *
from looks import LOOKS

KEY = "apartamentos-turisticos"

# Como COLEGIADO: el 00-0000 lo detecta domain.TextosDemo y el sitio no se publica hasta cambiarlo.
REGISTRO = ("Nº de registro turístico: 00-0000", "Tourist rental registration no.: 00-0000")


def build():
    nueva(KEY)
    look = dict(LOOKS["arena"], fuentes=("dm-sans", "figtree"))
    nav = [("inicio", ("Inicio", "Home")), ("apartamentos", ("Apartamentos", "Apartments")),
           ("tarifas", ("Tarifas y normas", "Rates and rules")), ("reservar", ("Reservar", "Book"))]
    horas = [(("Todos los días", "Every day"), ("9:00 a 20:00 por teléfono y WhatsApp", "9:00 am to 8:00 pm by phone and WhatsApp")),
             (("Entrada (check-in)", "Check-in"), ("Desde las 16:00", "From 4:00 pm")),
             (("Salida (check-out)", "Check-out"), ("Hasta las 11:00", "By 11:00 am"))]
    cta_cab = marco([llamar(t=("Llamar", "Call")), whatsapp("secondary", t=("WhatsApp", "WhatsApp"))],
                    dir="row", gap=8, wrap=True)
    consulta = lambda v="primary": boton(("Consultar disponibilidad", "Check availability"),
                                         a_pagina("reservar", "disponibilidad"), v)
    nota = ("Precios de ejemplo: cámbialos por los tuyos. Son por noche y por apartamento.",
            "Example prices: replace them with your own. They are per night and per apartment.")

    def dato(t):
        return fila_li([icono("check", sz=18), peq(t)], dir="row", gap=10, align="start")

    def ficha(nombre, resumen, datos, alt):
        return tarjeta([imagen("photo", alt, ratio=1.3), h3(nombre), p(resumen, color="$muted"),
                        lista_vertical([dato(d) for d in datos], gap=8)], estilo="borde", gap=14, pad=16)

    estudio = ficha(("Estudio", "Studio"),
                    ("Para una pareja o para una persona que viaja sola.", "For a couple or someone travelling alone."),
                    [("Hasta 2 personas", "Up to 2 guests"), ("1 cama de matrimonio", "1 double bed"),
                     ("1 baño con ducha", "1 bathroom with shower"), ("Cocina en el mismo espacio", "Kitchen in the same room")],
                    ("Estudio con cama de matrimonio y cocina", "Studio with a double bed and kitchen"))
    uno = ficha(("Apartamento de un dormitorio", "One-bedroom apartment"),
                ("Para dos o tres personas, o una familia pequeña, con salón aparte.",
                 "For two or three guests, or a small family, with a separate living room."),
                [("Hasta 4 personas", "Up to 4 guests"), ("1 cama de matrimonio y sofá cama", "1 double bed and a sofa bed"),
                 ("1 baño con ducha", "1 bathroom with shower"), ("Cocina y salón", "Kitchen and living room")],
                ("Salón de un apartamento de un dormitorio", "Living room of a one-bedroom apartment"))
    dos = ficha(("Apartamento de dos dormitorios", "Two-bedroom apartment"),
                ("Para grupos de amigos o familias, con dos habitaciones y espacio para comer todos juntos.",
                 "For groups of friends or families, with two bedrooms and room to eat together."),
                [("Hasta 6 personas", "Up to 6 guests"), ("1 cama de matrimonio, 2 camas individuales y sofá cama",
                                                         "1 double bed, 2 single beds and a sofa bed"),
                 ("2 baños", "2 bathrooms"), ("Cocina y comedor", "Kitchen and dining area")],
                ("Dormitorio de un apartamento de dos dormitorios", "Bedroom of a two-bedroom apartment"))

    inicio = pagina("inicio", ("", ""), ("Apartamentos turísticos", "Holiday apartments"),
                    ("Apartamentos turísticos con cocina propia: fichas con capacidad y equipamiento, tarifas por temporada, normas y consulta de disponibilidad.",
                     "Holiday apartments with their own kitchen: details on capacity and equipment, seasonal rates, house rules and an availability enquiry."), [
        hero(("Tu casa durante las vacaciones, con llave propia", "Your home for the holidays, with your own key"),
             ("Apartamentos con cocina, ropa de cama y toallas para estancias desde una noche. Mira cuál se ajusta a tu grupo y consulta las fechas que te interesan.",
              "Apartments with a kitchen, bed linen and towels for stays from one night. See which one fits your group and ask about the dates you want."),
             [consulta(), whatsapp()],
             eyebrow=("Apartamentos turísticos", "Holiday apartments"),
             extra=peq(REGISTRO, color="$muted"),
             imagen_=("landscape", ("Salón luminoso de un apartamento turístico", "Bright living room of a holiday apartment"))),
        bloque_checks(("Lo que encuentras en todas las estancias", "What you find in every stay"), [
            ("Cocina equipada para cocinar de verdad.", "A fully equipped kitchen you can really cook in."),
            ("Ropa de cama y toallas ya puestas a tu llegada.", "Bed linen and towels ready when you arrive."),
            ("Wifi, aire acondicionado y calefacción.", "Wifi, air conditioning and heating."),
            ("Limpieza final incluida en la tarifa.", "Final cleaning included in the rate."),
            ("Un teléfono y un WhatsApp al que escribir si algo falla.", "A phone number and WhatsApp to contact if anything goes wrong."),
            ("Normas claras desde antes de reservar.", "Clear rules before you book."),
        ], eyebrow=("Incluido", "Included"), tono="relleno", cols=2),
        bloque_valores(("Elige tu apartamento", "Choose your apartment"), [
            (("Estudio", "Studio"), ("Hasta 2 personas. Un espacio único con cama de matrimonio y cocina.",
                                     "Up to 2 guests. A single room with a double bed and a kitchen.")),
            (("Un dormitorio", "One bedroom"), ("Hasta 4 personas. Dormitorio aparte y sofá cama en el salón.",
                                                 "Up to 4 guests. A separate bedroom and a sofa bed in the living room.")),
            (("Dos dormitorios", "Two bedrooms"), ("Hasta 6 personas. Dos habitaciones y dos baños para grupos y familias.",
                                                    "Up to 6 guests. Two bedrooms and two bathrooms for groups and families.")),
        ], intro=("Capacidades de ejemplo: ajústalas a tus apartamentos en la página de fichas.",
                  "Example capacities: adjust them to your apartments on the details page."),
            estilo="relleno", anchor="tipos"),
        bloque_galeria(("Un vistazo a los apartamentos", "A look at the apartments"), [
            ("Dormitorio con ropa de cama blanca", "Bedroom with white bed linen"),
            ("Cocina con encimera y menaje", "Kitchen with worktop and utensils"),
            ("Terraza con mesa y sillas", "Terrace with a table and chairs"),
        ], tipo="photo", cols=3, ratio=1.2, tono="base"),
        bloque_pasos(("Reservar es sencillo", "Booking is simple"), [
            (("Pides fechas", "You ask for dates"),
             ("Nos dices cuándo llegas, cuándo te vas y cuántas personas sois.",
              "Tell us when you arrive, when you leave and how many of you there are.")),
            (("Te confirmamos", "We confirm"),
             ("Te contestamos con la disponibilidad y el precio total, sin letra pequeña.",
              "We reply with availability and the total price, with no small print.")),
            (("Reservas", "You book"),
             ("Te indicamos cómo abonar la reserva y te mandamos la confirmación.",
              "We tell you how to pay the deposit and send you the confirmation.")),
            (("Llegas", "You arrive"),
             ("Unos días antes te enviamos cómo entrar y dónde aparcar.",
              "A few days before, we send you how to get in and where to park.")),
        ], tono="relleno"),
        bloque_opiniones(("Lo que cuentan quienes se han alojado", "What past guests say"),
                         [("la limpieza", "the cleanliness"), ("la ubicación", "the location"), ("el trato", "the service")],
                         tono="base"),
        bloque_cta(("¿Qué fechas tienes en mente?", "Which dates do you have in mind?"),
                   ("Consúltanos y te respondemos con disponibilidad y precio.",
                    "Ask us and we will reply with availability and price."),
                   [consulta(), llamar("secondary")]),
    ])

    apart = pagina("apartamentos", ("apartamentos", "apartments"), ("Apartamentos", "Apartments"),
                   ("Ficha de cada apartamento con capacidad, camas y equipamiento. Tabla comparativa de lo que incluye cada uno.",
                    "Details of each apartment with capacity, beds and equipment. A comparison table of what each one includes."), [
        hero(("Nuestros apartamentos", "Our apartments"),
             ("Tres tipos de apartamento según cuántos seáis. Todos tienen cocina y están preparados para que llegues y te instales.",
              "Three types of apartment depending on how many of you there are. All have a kitchen and are ready for you to arrive and settle in."),
             [consulta(), whatsapp("secondary")], variante="texto", extra=peq(REGISTRO, color="$muted")),
        seccion("features", [
            titular(("Fichas", "Details"),
                    ("Capacidades y camas de ejemplo: cámbialas por las de tus apartamentos. Si tienes varios iguales, deja una sola ficha.",
                     "Example capacities and beds: replace them with those of your apartments. If you have several identical ones, keep a single card.")),
            rejilla([estudio, uno, dos], cols=3)], anchor="fichas"),
        seccion("features", [
            titular(("Equipamiento de cada apartamento", "What each apartment has"),
                    ("Para que no te sorprenda nada al llegar.", "So nothing comes as a surprise when you arrive.")),
            tabla(("Equipamiento por tipo de apartamento", "Equipment by apartment type"),
                  [("Equipamiento", "Equipment"), ("Estudio", "Studio"), ("Un dormitorio", "One bedroom"),
                   ("Dos dormitorios", "Two bedrooms")],
                  [(("Cocina equipada", "Equipped kitchen"), ("Sí", "Yes"), ("Sí", "Yes"), ("Sí", "Yes")),
                   (("Wifi", "Wifi"), ("Sí", "Yes"), ("Sí", "Yes"), ("Sí", "Yes")),
                   (("Aire acondicionado y calefacción", "Air conditioning and heating"), ("Sí", "Yes"), ("Sí", "Yes"), ("Sí", "Yes")),
                   (("Ropa de cama y toallas", "Bed linen and towels"), ("Sí", "Yes"), ("Sí", "Yes"), ("Sí", "Yes")),
                   (("Lavadora", "Washing machine"), ("No", "No"), ("Sí", "Yes"), ("Sí", "Yes")),
                   (("Televisión", "Television"), ("Sí", "Yes"), ("Sí", "Yes"), ("Sí", "Yes")),
                   (("Terraza o balcón", "Terrace or balcony"), ("No", "No"), ("Sí", "Yes"), ("Sí", "Yes")),
                   (("Cuna y trona", "Cot and high chair"), ("A petición", "On request"), ("A petición", "On request"),
                    ("A petición", "On request")),
                   (("Plaza de aparcamiento", "Parking space"), ("Consultar", "Ask"), ("Consultar", "Ask"), ("Consultar", "Ask"))]),
            peq(("Equipamiento de ejemplo: cambia cada fila por lo que tienes de verdad.",
                 "Example equipment: change each row to what you really have."), color="$muted")],
            tono="relleno", anchor="equipamiento", gap=28),
        bloque_texto(("Si viajas con niños o con una persona con movilidad reducida", "If you travel with children or someone with reduced mobility"), [
            ("Dínoslo al pedir las fechas y te recomendamos el apartamento que mejor se adapta: planta baja o con ascensor, cuna, trona o protector de escaleras.",
             "Tell us when you ask for dates and we will recommend the apartment that suits best: ground floor or with a lift, cot, high chair or stair gate."),
            ("No prometemos lo que no tenemos: si un apartamento no te sirve, te lo decimos.",
             "We do not promise what we do not have: if an apartment will not work for you, we say so.")],
            eyebrow=("Accesibilidad y familias", "Accessibility and families"),
            imagen_=("photo", ("Cuna y juguetes en un dormitorio de apartamento", "Cot and toys in an apartment bedroom"))),
        bloque_cta(("¿Cuál se ajusta a tu grupo?", "Which one suits your group?"), None,
                   [consulta(), llamar("secondary")]),
    ])

    tarifas = pagina("tarifas", ("tarifas-y-normas", "rates-and-rules"), ("Tarifas y normas", "Rates and rules"),
                     ("Tarifas por temporada, qué incluyen, check-in autónomo o con recepción, normas de la casa y preguntas frecuentes.",
                      "Seasonal rates, what they include, self check-in or reception check-in, house rules and frequently asked questions."), [
        hero(("Tarifas y normas", "Rates and rules"),
             ("El precio por noche cambia con la temporada. Te damos el total antes de que reserves, con todo incluido.",
              "The nightly price changes with the season. You get the total before you book, with everything included."),
             [consulta(), whatsapp("secondary")], variante="texto"),
        bloque_precios(("Tarifas por temporada", "Seasonal rates"), ("Tarifas por noche y por apartamento", "Nightly rates per apartment"),
                       [("Temporada", "Season"), ("Estudio", "Studio"), ("Un dormitorio", "One bedroom"),
                        ("Dos dormitorios", "Two bedrooms"), ("Estancia mínima", "Minimum stay")],
                       [(("Temporada baja", "Low season"), eur(55), eur(70), eur(95), ("2 noches", "2 nights")),
                        (("Temporada media", "Mid season"), eur(70), eur(90), eur(120), ("3 noches", "3 nights")),
                        (("Temporada alta", "High season"), eur(95), eur(120), eur(160), ("5 noches", "5 nights")),
                        (("Semana Santa y Navidad", "Easter and Christmas"), eur(110), eur(140), eur(185), ("4 noches", "4 nights"))],
                       intro=("Cada tarifa es por noche e incluye ropa de cama, toallas, wifi y limpieza final.",
                              "Each rate is per night and includes bed linen, towels, wifi and final cleaning."),
                       nota=nota, anchor="tarifas-temporada", tono="base"),
        bloque_checks(("Qué se paga aparte", "What is charged separately"), [
            ("Una fianza que te devolvemos al salir si todo está en orden.", "A deposit we return when you leave if everything is in order."),
            ("La tasa turística, si tu municipio la cobra.", "The tourist tax, if your municipality charges one."),
            ("Cuna, trona o cama supletoria: dínoslo al reservar.", "Cot, high chair or extra bed: tell us when you book."),
            ("Aparcamiento, si el apartamento no lo incluye.", "Parking, if the apartment does not include it."),
        ], eyebrow=("Aparte de la tarifa", "On top of the rate"), tono="relleno", cols=2, anchor="aparte"),
        bloque_valores(("Check-in", "Check-in"), [
            (("Autónomo", "Self check-in"),
             ("Te enviamos un código o la ubicación de la caja de llaves unos días antes. Entras a partir de las 16:00 sin esperar a nadie.",
              "We send you a code or the location of the key box a few days before. You let yourself in from 4:00 pm with no one to wait for.")),
            (("Con recepción", "With a reception"),
             ("Te recibimos en persona, te enseñamos el apartamento y te damos la llave. Avísanos de tu hora de llegada.",
              "We meet you in person, show you the apartment and hand over the key. Let us know your arrival time.")),
        ], intro=("Según el apartamento, la llegada es de una forma o de otra. La salida es hasta las 11:00 y dejas la llave como te indiquemos.",
                  "Depending on the apartment, arrival works one way or the other. Check-out is by 11:00 am and you leave the key as we tell you."),
            cols=2, estilo="borde", anchor="check-in"),
        bloque_texto(("Registro de huéspedes", "Guest registration"), [
            ("Por ley tenemos que registrar a todas las personas mayores de edad que se alojan. Antes de la llegada te pedimos el DNI o el pasaporte de cada una.",
             "By law we have to register every adult who stays. Before arrival we ask for the ID card or passport of each one.")],
            eyebrow=("Antes de llegar", "Before you arrive"), tono="relleno"),
        bloque_checks(("Normas de la casa", "House rules"), [
            ("Silencio de 22:00 a 8:00 por respeto a los vecinos.", "Quiet hours from 10:00 pm to 8:00 am out of respect for the neighbours."),
            ("No se puede fumar dentro del apartamento.", "No smoking inside the apartment."),
            ("No se permiten fiestas ni más personas que las de la reserva.", "No parties and no more guests than those on the booking."),
            ("Mascotas: consúltanos antes de reservar.", "Pets: ask us before you book."),
            ("Separa la basura y el reciclaje en los contenedores de la calle.", "Sort your rubbish and recycling in the street bins."),
            ("Si algo se rompe, avísanos. Preferimos saberlo a que te preocupes.", "If anything breaks, tell us. We would rather know than have you worry."),
        ], tono="base", cols=2, anchor="normas"),
        bloque_faq(("Preguntas frecuentes", "Frequently asked questions"), [
            (("¿Cómo reservo?", "How do I book?"),
             ("Pídenos las fechas con el formulario o por WhatsApp. Te confirmamos disponibilidad y precio, y la reserva queda hecha cuando abonas el importe que te indiquemos.",
              "Ask for your dates with the form or on WhatsApp. We confirm availability and price, and the booking is made once you pay the amount we tell you.")),
            (("¿Puedo cancelar?", "Can I cancel?"),
             ("Te decimos las condiciones de cancelación antes de que pagues. En general, cuanto antes avises, más te devolvemos.",
              "We tell you the cancellation terms before you pay. In general, the earlier you let us know, the more we refund.")),
            (("¿Y si llego tarde?", "What if I arrive late?"),
             ("Escríbenos o llámanos con tu hora aproximada. Con check-in autónomo puedes entrar a cualquier hora.",
              "Message or call us with your approximate time. With self check-in you can come in at any hour.")),
            (("¿Me dais factura?", "Can I have an invoice?"),
             ("Sí. Pídela al reservar con tus datos fiscales y te la mandamos por correo.",
              "Yes. Ask for it when you book with your tax details and we email it to you.")),
            (("¿Puedo quedarme más noches?", "Can I stay more nights?"),
             ("Si el apartamento está libre, sí. Dínoslo cuanto antes y te lo confirmamos.",
              "If the apartment is free, yes. Tell us as soon as you can and we will confirm.")),
        ], tono="relleno"),
        bloque_cta(("¿Miramos tus fechas?", "Shall we look at your dates?"), None, [consulta(), llamar("secondary")], tono="base"),
    ])

    reservar = pagina("reservar", ("reservar", "book"), ("Reservar y cómo llegar", "Book and how to get here"),
                      ("Consulta la disponibilidad con fechas y número de personas, y mira cómo llegar en coche, tren, autobús o desde el aeropuerto.",
                       "Check availability with dates and number of guests, and see how to get here by car, train, bus or from the airport."), [
        hero(("Consulta la disponibilidad", "Check availability"),
             ("Dinos las fechas y cuántas personas sois. Te respondemos el mismo día con el apartamento libre y el precio total.",
              "Tell us the dates and how many of you there are. We reply the same day with the free apartment and the total price."),
             [llamar(), whatsapp()], variante="texto", extra=peq(REGISTRO, color="$muted")),
        bloque_contacto(("Pide tus fechas", "Ask for your dates"),
                        ("Sin compromiso: es una consulta, no una reserva.",
                         "No commitment: this is an enquiry, not a booking."),
                        [campo("nombre", "text", ("Nombre", "Name"), True, auto="name", maxlen=120),
                         campo("correo", "email", ("Correo electrónico", "Email"), True, auto="email", maxlen=200),
                         campo("telefono", "tel", ("Teléfono (opcional)", "Phone (optional)"), False, auto="tel", maxlen=30),
                         campo("entrada", "date", ("Día de llegada", "Arrival date"), True),
                         campo("salida", "date", ("Día de salida", "Departure date"), True),
                         campo("personas", "number", ("Número de personas", "Number of guests"), True),
                         campo("apartamento", "select", ("Apartamento", "Apartment"), False,
                               opciones=[("Estudio", "Studio"), ("Un dormitorio", "One bedroom"),
                                         ("Dos dormitorios", "Two bedrooms"), ("No lo sé todavía", "I do not know yet")]),
                         campo("mensaje", "textarea", ("Algo que debamos saber (niños, mascota, hora de llegada)",
                                                       "Anything we should know (children, pet, arrival time)"), False, maxlen=1000)],
                        ("Consultar disponibilidad", "Check availability"), horas, anchor="disponibilidad",
                        exito=("Gracias. Te respondemos con la disponibilidad y el precio en cuanto lo miremos.",
                               "Thank you. We will reply with availability and price as soon as we have checked.")),
        bloque_mapa(("Cómo llegar", "How to get here"),
                    ("Los apartamentos están en el centro de Tu ciudad. Te mandamos la ubicación exacta con la confirmación.",
                     "The apartments are in the centre of Your city. We send you the exact location with the confirmation."),
                    tono="relleno"),
        bloque_valores(("Transporte", "Getting around"), [
            (("En coche", "By car"), ("Te indicamos dónde aparcar: en la calle, en un garaje cercano o en el propio edificio.",
                                      "We tell you where to park: on the street, in a nearby car park or in the building itself.")),
            (("En tren o autobús", "By train or bus"), ("Desde la estación de Tu ciudad se llega a pie, en taxi o en autobús urbano. Te damos la línea y la parada.",
                                                       "From the station in Your city you can walk, take a taxi or an urban bus. We give you the line and the stop.")),
            (("Desde el aeropuerto", "From the airport"), ("Hay taxi y transporte público. Si lo prefieres, te ayudamos a pedir un traslado.",
                                                           "There are taxis and public transport. If you prefer, we help you arrange a transfer.")),
        ], eyebrow=("Antes de salir", "Before you set off"), tono="base", estilo="linea"),
    ])

    return plantilla(KEY, ("Apartamentos turísticos", "Holiday apartments"),
                     ("Apartamentos turísticos con fichas y tabla de equipamiento, tarifas por temporada, check-in, normas, cómo llegar y consulta de disponibilidad por formulario o WhatsApp.",
                      "Holiday apartments with details and an equipment table, seasonal rates, check-in, house rules, directions and an availability enquiry by form or WhatsApp."),
                     "turismo", ("apartamentos turisticos apartamento piso turistico alquiler vacacional alquiler turistico vivienda de uso turistico estudio alojamiento vacaciones reservar disponibilidad check-in tarifas temporada",
                                 "holiday apartments tourist apartment holiday rental short term rental self catering studio accommodation vacation book availability check-in seasonal rates"),
                     look, cabecera(nav, cta_cab),
                     pie(nav, ("Apartamentos turísticos con cocina propia. Nº de registro turístico: 00-0000.",
                               "Holiday apartments with their own kitchen. Tourist rental registration no.: 00-0000."),
                         ("Todos los días 9:00 a 20:00 por teléfono y WhatsApp. Entrada desde las 16:00.",
                          "Every day 9 am to 8 pm by phone and WhatsApp. Check-in from 4 pm.")),
                     [inicio, apart, tarifas, reservar])
