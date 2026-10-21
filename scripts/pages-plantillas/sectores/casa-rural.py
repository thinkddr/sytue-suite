# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
from lib import *
from looks import LOOKS

KEY = "casa-rural"

# Como COLEGIADO: el 00-0000 lo detecta domain.TextosDemo y el sitio no se publica hasta cambiarlo.
REGISTRO = ("Nº de registro de turismo: 00-0000", "Tourism register no.: 00-0000")

# Paleta propia (madera y piedra): `alojamiento` ya usa verde con lora y manrope. No lleva fondo
# oscuro: primary/inverse no llega a 3, así que aquí no se usa `tono="oscuro"`.
PIEDRA = dict(
    colores=dict(background="#fbf9f4", surface="#efe8da", text="#262018", muted="#5e5240", primary="#6b4423",
                 on_primary="#ffffff", border="#ddd3bf", inverse="#2a2019", on_inverse="#f7f1e6"),
    fuentes=("lora", "dm-sans"), radio=(4, 8, 14))


def habitacion(nombre, capacidad, texto_, alt):
    return marco([imagen("photo", alt, ratio=1.3), h3(nombre), peq(capacidad, color="$muted"), p(texto_)],
                 tag="li", gap=10)


def build():
    nueva(KEY)
    look = PIEDRA
    nav = [("inicio", ("Inicio", "Home")), ("casa", ("La casa", "The house")),
           ("tarifas", ("Tarifas y normas", "Rates and house rules")),
           ("contacto", ("Disponibilidad", "Availability"))]
    horas = [(("Atención por teléfono y WhatsApp", "Phone and WhatsApp"),
              ("Todos los días, de 9:00 a 21:00", "Every day, 9:00 am to 9:00 pm")),
             (("Entrada (check-in)", "Check-in"), ("A partir de las 16:00", "From 4:00 pm")),
             (("Salida (check-out)", "Check-out"), ("Antes de las 12:00", "Before 12:00 noon"))]
    cta_cab = marco([llamar(t=("Llamar", "Call")), whatsapp("secondary", t=("WhatsApp", "WhatsApp"))],
                    dir="row", gap=8, wrap=True)
    consultar = lambda v="primary": boton(("Consultar disponibilidad", "Check availability"),
                                         a_pagina("contacto", "contacto"), v)
    nota = ("Precios de ejemplo: cámbialos por los tuyos. IVA incluido.",
            "Example prices: replace them with your own. VAT included.")

    inicio = pagina("inicio", ("", ""), ("Casa rural", "Country house"),
                    ("Casa rural con habitaciones, tarifas por temporada, normas de la casa, entorno y consulta de disponibilidad por fechas.",
                     "Country house with rooms, seasonal rates, house rules, the surroundings and an availability form by dates."), [
        hero(("Una casa en el campo para parar unos días", "A house in the countryside to stop for a few days"),
             ("Habitaciones con desayuno, cocina de casa y rutas a pie desde la puerta. Mira las fechas libres y escríbenos.",
              "Rooms with breakfast, home cooking and walks from the front door. Check the free dates and write to us."),
             [consultar(), llamar("secondary"), whatsapp("ghost")],
             eyebrow=("Casa rural", "Country house"), extra=peq(REGISTRO, color="$muted"),
             imagen_=("landscape", ("Fachada de piedra de una casa rural con un porche y un huerto delante",
                                    "Stone facade of a country house with a porch and a vegetable garden in front"))),
        bloque_texto(("La casa", "The house"), [
            ("Una casa de piedra y madera con pocas habitaciones, para que todo el mundo duerma bien y haya sitio para sentarse al fresco o junto a la chimenea.",
             "A stone and timber house with only a few rooms, so everyone sleeps well and there is room to sit out in the cool or by the fire."),
            ("Nos tienes a un teléfono para lo que haga falta: una ruta, un sitio donde comer o una manta más.",
             "You have us on the phone for anything you need: a route, somewhere to eat or an extra blanket."),
        ], imagen_=("photo", ("Salón con chimenea, sofás y vigas de madera", "Living room with a fireplace, sofas and timber beams")),
            tono="relleno", anchor="la-casa"),
        bloque_valores(("Qué encontrarás", "What you will find"), [
            (("Habitaciones", "Rooms"),
             ("Dobles, triples y familiares, cada una con baño. Las ves con capacidad y detalles en la página de la casa.",
              "Doubles, triples and family rooms, each with a bathroom. See capacity and details on the house page.")),
            (("Desayuno", "Breakfast"),
             ("Pan, fruta, embutidos y dulces de la zona, de 8:30 a 10:30. Si necesitas algo especial, avísanos al reservar.",
              "Bread, fruit, cured meats and sweets from the area, from 8:30 to 10:30. If you need something special, tell us when you book.")),
            (("Zonas comunes", "Shared areas"),
             ("Salón con chimenea, porche, jardín y sitio para dejar las bicis o las botas de monte.",
              "A living room with a fireplace, a porch, a garden and room to leave bikes or hiking boots.")),
        ], eyebrow=("Todo lo que necesitas", "Everything you need"), estilo="borde"),
        bloque_checks(("Qué hacer por aquí", "Things to do nearby"), [
            ("Rutas a pie y en bici que salen de la propia casa.", "Walking and cycling routes that start at the house."),
            ("Pueblos de alrededor con mercado y fiestas.", "Surrounding villages with markets and festivals."),
            ("Sitios para comer cocina de la zona.", "Places to eat local food."),
            ("Visitas a bodegas, queserías o talleres de artesanía.", "Visits to wineries, dairies or craft workshops."),
            ("Un río, un monte o un mirador a un paseo.", "A river, a hill or a viewpoint within a walk."),
            ("Días de lluvia: museos, termas y plan de mesa.", "Rainy days: museums, thermal baths and board games."),
        ], intro=("Cambia esta lista por lo que de verdad hay cerca de tu casa y cuánto se tarda en llegar.",
                  "Replace this list with what is really near your house and how long it takes to get there."),
            eyebrow=("Entorno", "Surroundings"), tono="base", anchor="entorno"),
        bloque_opiniones(("Lo que cuentan los huéspedes", "What guests say"),
                         [("la casa", "the house"), ("el desayuno", "the breakfast"), ("la tranquilidad", "the peace and quiet")],
                         tono="relleno"),
        bloque_cta(("¿Qué fechas tienes pensadas?", "Which dates do you have in mind?"),
                   ("Dinos cuándo llegas, cuándo te vas y cuántos sois, y te contestamos si hay sitio.",
                    "Tell us when you arrive, when you leave and how many of you there are, and we will reply whether there is room."),
                   [consultar(), whatsapp("ghost")], tono="base"),
    ])

    casa = pagina("casa", ("la-casa", "the-house"), ("La casa y las habitaciones", "The house and the rooms"),
                  ("Habitaciones con su capacidad, qué incluye la estancia, zonas comunes y una galería de fotos de la casa.",
                   "Rooms with their capacity, what your stay includes, shared areas and a photo gallery of the house."), [
        hero(("La casa y las habitaciones", "The house and the rooms"),
             ("Cada habitación tiene baño propio. Te decimos cuántas personas caben y qué incluye antes de que reserves.",
              "Every room has its own bathroom. We tell you how many people fit and what is included before you book."),
             [consultar(), llamar("secondary")], variante="texto"),
        seccion("features", [titular(("Habitaciones", "Rooms"),
                                     ("Pregúntanos por camas supletorias o cunas. Para grupos, consulta si se alquila la casa entera.",
                                      "Ask about extra beds or cots. For groups, ask whether the whole house can be hired.")),
                             rejilla([
                                 habitacion(("Habitación doble", "Double room"),
                                            ("Hasta 2 personas", "Up to 2 people"),
                                            ("Cama de matrimonio o dos camas, baño privado y ventana al jardín.",
                                             "Double bed or two single beds, private bathroom and a window onto the garden."),
                                            ("Habitación con cama de matrimonio, colcha de lino y mesilla de madera",
                                             "Room with a double bed, a linen cover and a wooden bedside table")),
                                 habitacion(("Habitación triple", "Triple room"),
                                            ("Hasta 3 personas", "Up to 3 people"),
                                            ("Una cama de matrimonio y una individual, con espacio para el equipaje.",
                                             "A double bed and a single bed, with room for luggage."),
                                            ("Habitación con tres camas y suelo de madera",
                                             "Room with three beds and a wooden floor")),
                                 habitacion(("Habitación familiar", "Family room"),
                                            ("Hasta 4 personas", "Up to 4 people"),
                                            ("Dos ambientes comunicados, con cama de matrimonio y literas o dos camas.",
                                             "Two connected spaces, with a double bed and bunk beds or two single beds."),
                                            ("Habitación familiar con literas de madera y una cama de matrimonio",
                                             "Family room with wooden bunk beds and a double bed")),
                             ], cols=3)], anchor="habitaciones", gap=32),
        bloque_checks(("Qué incluye la estancia", "What your stay includes"), [
            ("Desayuno cada mañana con productos de la zona.", "Breakfast every morning with local produce."),
            ("Ropa de cama y toallas.", "Bed linen and towels."),
            ("Calefacción en invierno y ventilador o aire en verano.", "Heating in winter and a fan or air conditioning in summer."),
            ("Wifi en toda la casa.", "Wifi throughout the house."),
            ("Leña para la chimenea del salón.", "Firewood for the living-room fireplace."),
            ("Aparcamiento dentro de la finca.", "Parking on the grounds."),
        ], eyebrow=("Incluido", "Included"), tono="relleno", anchor="incluye"),
        bloque_galeria(("La casa por dentro y por fuera", "The house inside and out"), [
            ("Salón con chimenea y sofás", "Living room with a fireplace and sofas"),
            ("Comedor con mesa de madera para el desayuno", "Dining room with a wooden table for breakfast"),
            ("Porche con sillas y vistas al campo", "Porch with chairs and a view of the countryside"),
            ("Jardín con huerto y árboles", "Garden with a vegetable patch and trees"),
            ("Camino que sale de la casa hacia el monte", "Path leading from the house into the hills"),
            ("Baño con ducha y toallas dobladas", "Bathroom with a shower and folded towels"),
        ], eyebrow=("Fotos", "Photos"), cols=3, ratio=1.2, tono="base"),
        bloque_cta(("¿Te guardamos una habitación?", "Shall we keep a room for you?"), None,
                   [consultar(), whatsapp("ghost")], tono="relleno"),
    ])

    tarifas = pagina("tarifas", ("tarifas-y-normas", "rates-and-house-rules"), ("Tarifas y normas", "Rates and house rules"),
                     ("Tarifas por temporada, suplementos, normas de la casa (llegada, salida y mascotas) y preguntas frecuentes.",
                      "Seasonal rates, extras, house rules (arrival, departure and pets) and frequently asked questions."), [
        hero(("Tarifas y normas de la casa", "Rates and house rules"),
             ("El precio cambia con la temporada. Todo va con desayuno e IVA incluidos; las tasas, si las hay, se indican al reservar.",
              "The price changes with the season. Everything includes breakfast and VAT; any local tax is shown when you book."),
             [consultar(), llamar("secondary")], variante="texto"),
        bloque_precios(("Tarifas por temporada", "Seasonal rates"),
                       ("Tarifas por temporada de la habitación doble", "Seasonal rates for the double room"),
                       [("Temporada", "Season"), ("Noche", "Per night"), ("Fin de semana (2 noches)", "Weekend (2 nights)"),
                        ("Semana (7 noches)", "Week (7 nights)")],
                       [(("Temporada baja", "Low season"), eur(70), eur(130), eur(420)),
                        (("Temporada media", "Mid season"), eur(85), eur(160), eur(520)),
                        (("Temporada alta y agosto", "High season and August"), eur(110), eur(210), eur(700)),
                        (("Semana Santa, puentes y Navidad", "Easter, bank-holiday weekends and Christmas"), eur(120), eur(240), eur(780))],
                       intro=("Precio por noche de la habitación doble para dos personas, con desayuno. Escribe qué meses son cada temporada.",
                              "Price per night of the double room for two people, with breakfast. Write which months make up each season."),
                       nota=nota, anchor="tarifas", tono="base"),
        bloque_precios(("Suplementos", "Extras"), ("Suplementos", "Extras"),
                       [("Concepto", "Item"), ("Precio", "Price")],
                       [(("Persona extra en habitación triple o familiar (por noche)", "Extra person in a triple or family room (per night)"), eur(25)),
                        (("Cuna o cama supletoria (por noche)", "Cot or extra bed (per night)"), eur(10)),
                        (("Mascota pequeña (por estancia)", "Small pet (per stay)"), eur(15)),
                        (("Cena casera, por persona (con reserva)", "Home-cooked dinner, per person (by reservation)"), eur(22))],
                       nota=nota, anchor="suplementos", tono="relleno"),
        bloque_checks(("Normas de la casa", "House rules"), [
            ("Entrada a partir de las 16:00. Si llegas más tarde, avísanos y te esperamos.", "Check-in from 4:00 pm. If you arrive later, tell us and we will wait for you."),
            ("Salida antes de las 12:00, para preparar la habitación.", "Check-out before 12:00 noon, so we can prepare the room."),
            ("Mascotas pequeñas, solo con aviso previo y sin subir a camas ni sofás.", "Small pets only with prior notice, and not on beds or sofas."),
            ("Silencio de 23:00 a 8:00, para que todos descansen.", "Quiet hours from 11:00 pm to 8:00 am so everyone can rest."),
            ("No se fuma dentro de la casa; en el porche, sí.", "No smoking inside the house; on the porch it is fine."),
            ("Para confirmar la reserva pedimos una señal, que te descontamos de la estancia.", "To confirm a booking we ask for a deposit, which we deduct from your stay."),
        ], eyebrow=("Normas", "Rules"), tono="base", anchor="normas"),
        bloque_faq(("Preguntas frecuentes", "Frequently asked questions"), [
            (("¿Cuántas noches hay que reservar como mínimo?", "What is the minimum stay?"),
             ("En temporada alta y en puentes, dos noches. El resto del año, una.",
              "Two nights in high season and on bank-holiday weekends. One night the rest of the year.")),
            (("¿Puedo cancelar?", "Can I cancel?"),
             ("Sí. Si avisas con tiempo, te devolvemos la señal. Te decimos el plazo exacto al confirmar la reserva.",
              "Yes. If you give us notice, we return your deposit. We tell you the exact deadline when we confirm the booking.")),
            (("¿Se alquila la casa entera?", "Can the whole house be hired?"),
             ("Sí, para familias y grupos. Escríbenos con las fechas y cuántos sois y te damos el precio.",
              "Yes, for families and groups. Write to us with the dates and how many of you there are and we will give you a price.")),
            (("¿Hay comida cerca o se puede cenar en la casa?", "Is there food nearby, or can we have dinner at the house?"),
             ("Puedes cenar en la casa si lo pides con un día de antelación, o ir a algún restaurante de la zona. Te recomendamos.",
              "You can have dinner at the house if you ask a day ahead, or go to a restaurant in the area. We will recommend some.")),
            (("¿Qué hago si tengo alergias?", "What if I have allergies?"),
             ("Dínoslo al reservar y preparamos el desayuno y la cena teniéndolas en cuenta.",
              "Tell us when you book and we prepare breakfast and dinner with them in mind.")),
        ], tono="relleno"),
    ])

    cont = pagina("contacto", ("contacto", "contact"), ("Disponibilidad y cómo llegar", "Availability and how to find us"),
                  ("Consulta fechas libres con llegada, salida y número de personas. Teléfono, WhatsApp, cómo llegar y horario de atención.",
                   "Check free dates with arrival, departure and number of guests. Phone, WhatsApp, directions and contact hours."), [
        hero(("Consulta la disponibilidad", "Check availability"),
             ("Dinos cuándo llegas, cuándo te vas y cuántos sois. Te contestamos el mismo día con las habitaciones libres y el precio.",
              "Tell us when you arrive, when you leave and how many of you there are. We reply the same day with the free rooms and the price."),
             [llamar(), whatsapp()], variante="texto", extra=peq(REGISTRO, color="$muted")),
        bloque_contacto(("Fechas y personas", "Dates and guests"),
                        ("Si las fechas son flexibles, dínoslo en el mensaje y miramos alternativas.",
                         "If your dates are flexible, say so in the message and we will look at alternatives."),
                        [campo("nombre", "text", ("Nombre", "Name"), True, auto="name", maxlen=120),
                         campo("telefono", "tel", ("Teléfono", "Phone"), True, auto="tel", maxlen=30),
                         campo("correo", "email", ("Correo electrónico (opcional)", "Email (optional)"), False, auto="email", maxlen=200),
                         campo("llegada", "date", ("Fecha de llegada", "Arrival date"), True),
                         campo("salida", "date", ("Fecha de salida", "Departure date"), True),
                         campo("personas", "select", ("Número de personas", "Number of guests"), True,
                               opciones=[("1 persona", "1 guest"), ("2 personas", "2 guests"), ("3 personas", "3 guests"),
                                         ("4 personas", "4 guests"), ("5 a 8 personas", "5 to 8 guests"),
                                         ("Más de 8 personas", "More than 8 guests")]),
                         campo("mascota", "select", ("¿Viene con mascota?", "Bringing a pet?"), False,
                               opciones=[("No", "No"), ("Sí, pequeña", "Yes, small")]),
                         campo("mensaje", "textarea", ("Mensaje (opcional)", "Message (optional)"), False, maxlen=1000)],
                        ("Consultar disponibilidad", "Check availability"), horas,
                        notas=REGISTRO,
                        exito=("Gracias. Te contestamos con las habitaciones libres y el precio en cuanto lo veamos.",
                               "Thank you. We will reply with the free rooms and the price as soon as we see it.")),
        bloque_checks(("Cómo llegar", "Getting here"), [
            ("En coche: indica aquí la carretera, la salida y el último tramo hasta la casa.", "By car: write here the road, the exit and the last stretch to the house."),
            ("Aparcamiento gratuito dentro de la finca.", "Free parking on the grounds."),
            ("En transporte público: indica la estación o parada más cercana y cómo ir desde allí.", "By public transport: give the nearest station or stop and how to get from there."),
            ("Si llegas de noche, llámanos desde la carretera y te esperamos con la luz encendida.", "If you arrive after dark, call us from the road and we will be waiting with the light on."),
        ], eyebrow=("Cómo llegar", "Directions"), tono="relleno", anchor="llegar"),
        bloque_mapa(("En el mapa", "On the map"), None, tono="base"),
    ])

    return plantilla(KEY, ("Casa rural", "Country house"),
                     ("Casa rural y hotel pequeño con habitaciones y capacidad, tarifas por temporada, normas de la casa, entorno y consulta de disponibilidad por fechas.",
                      "Country house and small hotel with rooms and capacity, seasonal rates, house rules, surroundings and an availability form by dates."),
                     "turismo", ("casa rural hotel rural hotel pequeno hotel con encanto alojamiento rural turismo rural habitaciones agroturismo posada hospedaje escapada fin de semana desayuno mascotas disponibilidad",
                                 "country house rural house rural stay small hotel boutique hotel bed and breakfast guesthouse inn farm stay agritourism rooms weekend break availability pets"),
                     look, cabecera(nav, cta_cab),
                     pie(nav, ("Casa rural con habitaciones y desayuno. Nº de registro de turismo: 00-0000.",
                               "Country house with rooms and breakfast. Tourism register no.: 00-0000."),
                         ("Atención todos los días de 9:00 a 21:00. Entrada desde las 16:00, salida antes de las 12:00.",
                          "Open for contact every day 9 am to 9 pm. Check-in from 4 pm, check-out before 12 noon.")),
                     [inicio, casa, tarifas, cont], vertical="casa-rural")
