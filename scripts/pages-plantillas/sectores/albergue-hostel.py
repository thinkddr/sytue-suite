# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
from lib import *

KEY = "albergue-hostel"

# Como COLEGIADO: el 00-0000 lo detecta domain.TextosDemo y el sitio no se publica hasta cambiarlo.
REGISTRO = ("Nº de registro turístico: 00-0000", "Tourism register no.: 00-0000")

# Paleta propia (periwinkle claro con magenta): ninguna otra plantilla tiene este fondo ni este primario.
# No lleva fondo oscuro: primary/inverse no llega a 3, así que aquí no se usa `tono="oscuro"`.
PERIWINKLE = dict(
    colores=dict(background="#e8eef9", surface="#dae3f4", text="#141a33", muted="#4b5473", primary="#a0208a",
                 on_primary="#ffffff", border="#c5d0e8", inverse="#1a2140", on_inverse="#eef2fc"),
    fuentes=("figtree", "dm-sans"), radio=(6, 12, 18))


def build():
    nueva(KEY)
    look = PERIWINKLE
    nav = [("inicio", ("Inicio", "Home")), ("camas", ("Camas y precios", "Beds and prices")),
           ("albergue", ("Vida en el albergue", "Life at the hostel")),
           ("contacto", ("Reservar", "Book"))]
    horas = [(("Recepción", "Reception"), ("De 8:00 a 22:00", "8:00 am to 10:00 pm")),
             (("Entrada (check-in)", "Check-in"), ("A partir de las 14:00", "From 2:00 pm")),
             (("Salida (check-out)", "Check-out"), ("Antes de las 11:00", "Before 11:00 am")),
             (("Teléfono y WhatsApp", "Phone and WhatsApp"), ("Todos los días, de 9:00 a 21:00", "Every day, 9:00 am to 9:00 pm"))]
    cta_cab = marco([llamar(t=("Llamar", "Call")), whatsapp("secondary", t=("WhatsApp", "WhatsApp"))],
                    dir="row", gap=8, wrap=True)
    reservar = lambda v="primary": boton(("Reservar una cama", "Book a bed"), a_pagina("contacto", "contacto"), v)
    nota = ("Precios de ejemplo: cámbialos por los tuyos. IVA incluido; las tasas, si las hay, se indican aparte.",
            "Example prices: replace them with your own. VAT included; any local taxes are shown separately.")

    inicio = pagina("inicio", ("", ""), ("Albergue y hostel", "Hostel and guesthouse"),
                    ("Albergue y hostel con camas en dormitorio compartido y habitaciones privadas, cocina, zonas comunes, grupos y reserva por fechas.",
                     "Hostel with beds in shared dorms and private rooms, kitchen, common areas, groups and booking by dates."), [
        hero(("Una cama limpia y sitio para sentarte con otros viajeros", "A clean bed and somewhere to sit with other travellers"),
             ("Dormitorios compartidos, habitaciones privadas y una cocina que todos usan. Sirve para una noche de camino o para una semana.",
              "Shared dorms, private rooms and a kitchen everybody uses. Fine for a night on the road or for a week."),
             [reservar(), llamar("secondary"), whatsapp("ghost")], eyebrow=("Albergue y hostel", "Hostel"),
             variante="centro", extra=peq(REGISTRO, color="$muted")),
        bloque_valores(("Para quién es el albergue", "Who the hostel is for"), [
            (("Peregrinos y caminantes", "Pilgrims and walkers"),
             ("Cama barata, ducha caliente, sitio para las botas y una cena a una hora razonable. Si tu ruta lo tiene, sellamos la credencial.",
              "A cheap bed, a hot shower, somewhere for your boots and dinner at a sensible hour. If your route has one, we stamp your pilgrim passport.")),
            (("Mochileros y viajeros solos", "Backpackers and solo travellers"),
             ("Dormitorios compartidos, taquilla, enchufe junto a la cama y gente con quien compartir planes.",
              "Shared dorms, a locker, a socket by your bed and people to make plans with.")),
            (("Grupos y familias", "Groups and families"),
             ("Habitaciones privadas, dormitorios enteros para tu grupo y sitio para comer juntos.",
              "Private rooms, whole dorms for your group and space to eat together.")),
        ], eyebrow=("Quién se aloja", "Who stays here"), estilo="borde", anchor="para-quien"),
        bloque_texto(("Cocina y zonas comunes", "Kitchen and common areas"), [
            ("Cocina con fuegos, nevera, microondas y menaje. Cada cual cocina y recoge lo suyo, y hay una balda para lo que te sobra.",
             "A kitchen with hobs, a fridge, a microwave and utensils. Everyone cooks and tidies their own things, and there is a shelf for leftovers."),
            ("Salón con sofás, mesas y juegos, y un patio o terraza para el buen tiempo. Es donde se conoce la gente.",
             "A lounge with sofas, tables and games, and a courtyard or terrace for good weather. It is where people get to know each other."),
        ], imagen_=("photo", ("Cocina compartida con mesas largas y gente preparando la cena", "Shared kitchen with long tables and people preparing dinner")),
            eyebrow=("Compartir", "Sharing"), tono="relleno", anchor="cocina"),
        bloque_pasos(("Cómo llegas", "How you arrive"), [
            (("Reservas", "You book"),
             ("Eliges fechas y cama. Te confirmamos y te decimos cómo entrar.", "You choose dates and a bed. We confirm and tell you how to get in.")),
            (("Llegas", "You arrive"),
             ("Con recepción, de 8:00 a 22:00. O, si llegas más tarde, con llegada autónoma: te enviamos un código el día antes.",
              "At reception, 8:00 am to 10:00 pm. Or, if you arrive later, with self check-in: we send you a code the day before.")),
            (("Te instalas", "You settle in"),
             ("Te damos las sábanas, te enseñamos la cama y la taquilla, y te contamos cómo funciona la casa.",
              "We give you the sheets, show you the bed and the locker, and explain how the place works.")),
            (("Te vas", "You leave"),
             ("Dejas la cama libre antes de las 11:00. Si tu etapa es corta, la mochila puede esperar en consigna.",
              "You free up the bed before 11:00 am. If your day is short, the rucksack can wait in the luggage room.")),
        ], intro=("Cambia esta explicación por cómo se llega en tu albergue: con recepción, autónoma o las dos.",
                  "Replace this explanation with how guests arrive at your hostel: at reception, self check-in or both."),
            tono="base", anchor="llegada"),
        bloque_opiniones(("Lo que cuentan los huéspedes", "What guests say"),
                         [("la cama", "the bed"), ("la cocina", "the kitchen"), ("el ambiente", "the atmosphere")],
                         tono="relleno"),
        bloque_cta(("¿Qué noches necesitas?", "Which nights do you need?"),
                   ("Dinos cuándo llegas, cuándo te vas y cuántos sois, y te decimos qué camas quedan.",
                    "Tell us when you arrive, when you leave and how many of you there are, and we tell you which beds are left."),
                   [reservar(), whatsapp("ghost")], tono="base"),
    ])

    camas = pagina("camas", ("camas-y-precios", "beds-and-prices"), ("Camas y precios", "Beds and prices"),
                   ("Camas en dormitorio compartido y habitaciones privadas con precio por noche, qué incluye cada opción y grupos.",
                    "Beds in shared dorms and private rooms with price per night, what each option includes and groups."), [
        hero(("Camas, habitaciones y precios", "Beds, rooms and prices"),
             ("Dormitorios de varios tamaños y habitaciones privadas. El precio de una cama es por persona y noche.",
              "Dorms of different sizes and private rooms. The price of a bed is per person per night."),
             [reservar(), llamar("secondary")], variante="texto"),
        bloque_precios(("Precios por noche", "Prices per night"),
                       ("Precio por noche de las camas en dormitorio y de las habitaciones privadas",
                        "Price per night of beds in dorms and private rooms"),
                       [("Opción", "Option"), ("Capacidad", "Capacity"), ("Precio", "Price")],
                       [(("Cama en dormitorio mixto de 8", "Bed in a mixed 8-bed dorm"), ("1 persona", "1 person"), eur(18)),
                        (("Cama en dormitorio mixto de 6", "Bed in a mixed 6-bed dorm"), ("1 persona", "1 person"), eur(21)),
                        (("Cama en dormitorio femenino de 4", "Bed in a female 4-bed dorm"), ("1 persona", "1 person"), eur(25)),
                        (("Habitación privada individual", "Private single room"), ("1 persona", "1 person"), eur(45)),
                        (("Habitación privada doble", "Private double room"), ("2 personas", "2 people"), eur(60)),
                        (("Habitación privada familiar", "Private family room"), ("Hasta 4 personas", "Up to 4 people"), eur(85))],
                       intro=("Precio de una noche en temporada media, con sábanas. Habitación con baño en el pasillo o dentro: indica aquí cuál.",
                              "Price for one night in mid season, with sheets. Bathroom in the corridor or in the room: write here which."),
                       nota=nota, anchor="precios", tono="relleno"),
        bloque_checks(("Qué incluye la cama", "What the bed includes"), [
            ("Sábanas y funda de almohada, sin coste extra.", "Sheets and a pillowcase, at no extra cost."),
            ("Enchufe y luz de lectura junto a la cama.", "A socket and reading light next to the bed."),
            ("Taquilla individual: trae tu candado o lo alquilas en recepción.", "An individual locker: bring your own padlock or hire one at reception."),
            ("Ducha caliente y aseos, comunes o dentro de la habitación según la opción.", "Hot shower and toilets, shared or in the room depending on the option."),
            ("Wifi en todo el edificio.", "Wifi throughout the building."),
            ("La toalla se alquila aparte, y el desayuno también si lo ofreces. Indica aquí los precios.", "Towels are hired separately, and breakfast too if you offer it. Write here the prices."),
        ], eyebrow=("Incluido", "Included"), tono="base", anchor="incluye"),
        seccion("features", [titular(("Dormitorios y habitaciones", "Dorms and rooms"),
                                     ("Dinos al reservar si prefieres cama baja, estar junto a la ventana o un dormitorio solo para mujeres.",
                                      "Tell us when you book if you prefer a lower bunk, a spot by the window or a women-only dorm.")),
                             rejilla([
                                 marco([imagen("photo", ("Dormitorio con literas de madera, taquillas y cortinas en cada cama",
                                                         "Dorm with wooden bunk beds, lockers and a curtain on each bed"), ratio=1.3),
                                        h3(("Dormitorio compartido", "Shared dorm")),
                                        p(("Literas con cortina, luz y enchufe en cada cama, y taquilla. Hay dormitorios de 4, 6 y 8 camas.",
                                           "Bunks with a curtain, light and socket at each bed, and a locker. There are dorms of 4, 6 and 8 beds."),
                                          color="$muted")], tag="li", gap=10),
                                 marco([imagen("photo", ("Habitación privada con cama doble, mesilla y ventana",
                                                         "Private room with a double bed, a bedside table and a window"), ratio=1.3),
                                        h3(("Habitación privada", "Private room")),
                                        p(("Individual, doble o familiar, con llave. Para quien quiere más intimidad sin pagar un hotel.",
                                           "Single, double or family, with a key. For anyone who wants more privacy without paying for a hotel."),
                                          color="$muted")], tag="li", gap=10),
                             ], cols=2)], anchor="dormitorios", gap=32, tono="relleno"),
        bloque_cta(("¿Te guardamos una cama?", "Shall we keep a bed for you?"), None,
                   [reservar(), whatsapp("ghost")], tono="base"),
    ])

    albergue = pagina("albergue", ("vida-en-el-albergue", "life-at-the-hostel"),
                      ("Vida en el albergue", "Life at the hostel"),
                      ("Cocina y zonas comunes, normas de convivencia, grupos escolares y deportivos, y preguntas frecuentes.",
                       "Kitchen and common areas, house rules, school and sports groups, and frequently asked questions."), [
        hero(("Cómo se vive en el albergue", "What life at the hostel is like"),
             ("Se comparte cocina, salón y baños. Unas pocas normas sencillas hacen que todos descansen.",
              "Kitchen, lounge and bathrooms are shared. A few simple rules mean everybody gets some rest."),
             [reservar(), llamar("secondary")], variante="texto"),
        bloque_valores(("Zonas comunes", "Common areas"), [
            (("Cocina", "Kitchen"),
             ("Fuegos, nevera, microondas, hervidor y menaje. Abierta de 7:00 a 23:00. Pon tu nombre a lo que guardes en la nevera.",
              "Hobs, fridge, microwave, kettle and utensils. Open from 7:00 am to 11:00 pm. Put your name on what you store in the fridge.")),
            (("Salón y terraza", "Lounge and terrace"),
             ("Sofás, mesas, juegos de mesa y libros para dejar o llevarte. La terraza o el patio, cuando hace bueno.",
              "Sofas, tables, board games and books to leave or take. The terrace or courtyard when the weather is good.")),
            (("Lavadora y secado", "Laundry and drying"),
             ("Lavadora y tendedero. Para quien camina o viaja, es de lo más útil de la casa.",
              "A washing machine and a drying line. For anyone walking or travelling, it is one of the most useful things in the house.")),
            (("Consigna y bicis", "Luggage room and bikes"),
             ("Sitio para dejar las mochilas hasta que se haga la hora y un rincón cubierto para las bicis.",
              "Somewhere to leave rucksacks until it is time and a covered corner for bikes.")),
        ], eyebrow=("Lo que se comparte", "What is shared"), tono="relleno", estilo="borde", cols=2, anchor="zonas-comunes"),
        bloque_checks(("Normas de la casa", "House rules"), [
            ("Silencio de 23:00 a 7:00. En los dormitorios, luz baja y móviles sin sonido.", "Quiet hours from 11:00 pm to 7:00 am. In dorms, low light and phones on silent."),
            ("Se entra con calzado limpio y las botas se dejan en el zapatero.", "Clean shoes indoors, and boots go on the shoe rack."),
            ("No se fuma dentro del edificio. Hay una zona fuera para hacerlo.", "No smoking inside the building. There is an outdoor area for it."),
            ("No se come en los dormitorios. La comida, en la cocina o en el salón.", "No eating in the dorms. Food stays in the kitchen or the lounge."),
            ("Cada persona recoge lo que usa en la cocina y deja la mesa limpia.", "Everyone tidies what they use in the kitchen and leaves the table clean."),
            ("La cama se libera antes de las 11:00, para que podamos prepararla.", "The bed is freed up before 11:00 am, so we can get it ready."),
        ], eyebrow=("Convivencia", "Living together"), tono="base", anchor="normas"),
        bloque_texto(("Grupos escolares y deportivos", "School and sports groups"), [
            ("Recibimos colegios, equipos y clubes. Reservamos dormitorios enteros para el grupo y un espacio para comer juntos.",
             "We welcome schools, teams and clubs. We reserve whole dorms for the group and a space to eat together."),
            ("Pedimos la lista de personas y, si hay menores, el nombre de los adultos responsables. Para el precio, pide presupuesto sin compromiso: depende de las noches y de cuántos seáis.",
             "We ask for the list of people and, if there are minors, the names of the responsible adults. For the price, ask for a quote with no obligation: it depends on the nights and how many of you there are."),
        ], imagen_=("photo", ("Grupo de jóvenes con mochilas ante la puerta de un albergue", "A group of young people with rucksacks at the door of a hostel")),
            eyebrow=("Grupos", "Groups"), tono="relleno", anchor="grupos", invertir=True),
        bloque_faq(("Preguntas frecuentes", "Frequently asked questions"), [
            (("¿Hace falta reservar?", "Do I need to book?"),
             ("Te lo recomendamos, sobre todo en fines de semana, puentes y temporada alta. Si te quedas sin cama, te ayudamos a buscar otra opción cercana.",
              "We recommend it, especially at weekends, bank-holiday weekends and high season. If you are left without a bed, we help you find another option nearby.")),
            (("¿Qué pasa si llego tarde?", "What if I arrive late?"),
             ("Avísanos. Si es después de que cierre recepción, entras con llegada autónoma con el código que te enviamos.",
              "Let us know. If it is after reception closes, you get in with self check-in using the code we send you.")),
            (("¿Puedo cancelar?", "Can I cancel?"),
             ("Sí. Si avisas con tiempo no pagas nada. Te decimos el plazo exacto al confirmar la reserva.",
              "Yes. If you give us notice you pay nothing. We tell you the exact deadline when we confirm the booking.")),
            (("¿Hay edad mínima en los dormitorios?", "Is there a minimum age in the dorms?"),
             ("Los menores van con un adulto responsable. Las familias suelen preferir una habitación privada.",
              "Minors stay with a responsible adult. Families usually prefer a private room.")),
            (("¿Puedo cocinar o hay que comer fuera?", "Can I cook, or do I have to eat out?"),
             ("Puedes cocinar en la cocina común. Si hay cenas o desayuno, te lo explicamos al reservar.",
              "You can cook in the shared kitchen. If we offer dinners or breakfast, we explain when you book.")),
        ], tono="relleno"),
    ])

    cont = pagina("contacto", ("contacto", "contact"), ("Reservar y cómo llegar", "Book and how to find us"),
                  ("Reserva camas o habitaciones con fechas y número de personas. Teléfono, WhatsApp, cómo llegar y horario de recepción.",
                   "Book beds or rooms with dates and number of guests. Phone, WhatsApp, directions and reception hours."), [
        hero(("Reserva tu cama", "Book your bed"),
             ("Dinos qué noches necesitas y cuántos sois. Te contestamos con las camas libres y el precio.",
              "Tell us which nights you need and how many of you there are. We reply with the free beds and the price."),
             [llamar(), whatsapp()], variante="texto", extra=peq(REGISTRO, color="$muted")),
        bloque_contacto(("Fechas y personas", "Dates and guests"),
                        ("Para grupos de más de diez personas, escribe el tipo de grupo en el mensaje y te hacemos un presupuesto.",
                         "For groups of more than ten, write the type of group in the message and we will prepare a quote."),
                        [campo("nombre", "text", ("Nombre", "Name"), True, auto="name", maxlen=120),
                         campo("telefono", "tel", ("Teléfono", "Phone"), True, auto="tel", maxlen=30),
                         campo("correo", "email", ("Correo electrónico (opcional)", "Email (optional)"), False, auto="email", maxlen=200),
                         campo("llegada", "date", ("Fecha de llegada", "Arrival date"), True),
                         campo("salida", "date", ("Fecha de salida", "Departure date"), True),
                         campo("tipo", "select", ("Qué buscas", "What you are looking for"), True,
                               opciones=[("Cama en dormitorio compartido", "Bed in a shared dorm"),
                                         ("Habitación privada", "Private room"),
                                         ("Grupo, colegio o club", "Group, school or club"),
                                         ("No lo sé todavía", "I do not know yet")]),
                         campo("personas", "select", ("Número de personas", "Number of guests"), True,
                               opciones=[("1 persona", "1 guest"), ("2 personas", "2 guests"), ("3 a 5 personas", "3 to 5 guests"),
                                         ("6 a 10 personas", "6 to 10 guests"), ("Más de 10 personas", "More than 10 guests")]),
                         campo("llegada_tipo", "select", ("Cómo llegarás", "How you will arrive"), False,
                               opciones=[("Con recepción abierta", "At reception"), ("Más tarde, con llegada autónoma", "Later, with self check-in")]),
                         campo("mensaje", "textarea", ("Mensaje (opcional)", "Message (optional)"), False, maxlen=1000)],
                        ("Reservar una cama", "Book a bed"), horas,
                        notas=REGISTRO,
                        exito=("Gracias. Te contestamos con las camas libres y el precio en cuanto lo veamos.",
                               "Thank you. We will reply with the free beds and the price as soon as we see it.")),
        bloque_checks(("Cómo llegar", "Getting here"), [
            ("A pie o en bici: indica aquí el camino, la ruta o la etapa que pasa por la puerta.", "On foot or by bike: write here the path, route or stage that passes the door."),
            ("En tren o autobús: indica aquí la estación o parada más cercana y cuánto se tarda andando.", "By train or bus: give the nearest station or stop and how long it takes to walk."),
            ("En coche: indica aquí dónde aparcar y si es de pago.", "By car: write here where to park and whether it is paid."),
            ("Si llegas fuera del horario de recepción, te mandamos el código para entrar el día antes.", "If you arrive outside reception hours, we send you the entry code the day before."),
        ], eyebrow=("Cómo llegar", "Directions"), tono="relleno", anchor="llegar"),
        bloque_mapa(("En el mapa", "On the map"), None, tono="base"),
    ])

    return plantilla(KEY, ("Albergue y hostel", "Hostel and guesthouse"),
                     ("Albergue y hostel con camas en dormitorio compartido y habitaciones privadas, precios por noche, cocina, zonas comunes, grupos y reserva con fechas y personas.",
                      "Hostel with beds in shared dorms and private rooms, nightly prices, kitchen, common areas, groups and booking with dates and guests."),
                     "turismo", ("albergue hostel hostal juvenil peregrinos camino de santiago mochileros camas dormitorio compartido habitaciones privadas cocina compartida grupos escolares equipos deportivos llegada autonoma reservas economico",
                                 "hostel guesthouse youth hostel pilgrims backpackers beds shared dorm private rooms shared kitchen school groups sports teams self check-in bookings budget"),
                     look, cabecera(nav, cta_cab),
                     pie(nav, ("Albergue con dormitorios compartidos y habitaciones privadas. Nº de registro turístico: 00-0000.",
                               "Hostel with shared dorms and private rooms. Tourism register no.: 00-0000."),
                         ("Recepción de 8:00 a 22:00. Entrada desde las 14:00, salida antes de las 11:00.",
                          "Reception 8 am to 10 pm. Check-in from 2 pm, check-out before 11 am.")),
                     [inicio, camas, albergue, cont], vertical="albergue-hostel")
