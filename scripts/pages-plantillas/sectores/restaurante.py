# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
from lib import *
from looks import LOOKS  # noqa: F401  (la paleta es propia de este sector)

KEY = "restaurante"

# Paleta propia (pergamino y madera tostada): `vino` es de escuela-musica y `terracota`/`burdeos` de otros.
# El fondo oscuro es casi negro a propósito: primary/inverse tiene que llegar a 3.
MESA = dict(
    colores=dict(background="#fcf8f1", surface="#f3e9d8", text="#2a1d12", muted="#63503c", primary="#8c5a12",
                 on_primary="#ffffff", border="#e5d6bb", inverse="#170e07", on_inverse="#f8efdf"),
    fuentes=("playfair-display", "figtree"), radio=(2, 6, 10), ancho=1120)

NOTA = ("Precios de ejemplo: cámbialos por los tuyos. IVA incluido.",
        "Example prices: replace them with your own. VAT included.")
CAB_PLATO = [("Plato", "Dish"), ("Descripción", "Description"), ("Precio", "Price")]


def plato(nombre, desc, precio):
    return (nombre, desc, precio)


def portada(titulo, entradilla, botones, eyebrow, alt):
    """Portada propia: título centrado y debajo una foto panorámica a todo el ancho."""
    texto_ = marco([etiqueta(eyebrow, align="center"), h1(titulo, align="center"),
                    p(entradilla, align="center", color="$muted"),
                    marco(botones, dir="row", gap=12, wrap=True, align="center", justify="center")],
                   gap=18, align="center", sz={"width": "hug", "height": "hug", "max_width": 720})
    return seccion("hero", [texto_, imagen("landscape", alt, ratio=2.3, prioridad=True)], pad_v=72, gap=44, align="center")


def plato_destacado(nombre, desc, precio, alt):
    return marco([imagen("photo", alt, ratio=1.25), h3(nombre), p(desc, color="$muted"), peq(precio)], tag="li", gap=10)


def build():
    nueva(KEY)
    look = MESA
    nav = [("inicio", ("Inicio", "Home")), ("carta", ("La carta", "The menu")),
           ("eventos", ("Eventos", "Events")), ("reservas", ("Reservar mesa", "Book a table"))]
    horas = [(("Lunes", "Monday"), ("Cerrado", "Closed")),
             (("Martes a jueves", "Tuesday to Thursday"),
              ("13:00 a 16:00 y 20:30 a 23:00", "1:00 pm to 4:00 pm and 8:30 pm to 11:00 pm")),
             (("Viernes y sábado", "Friday and Saturday"),
              ("13:00 a 16:00 y 20:30 a 23:30", "1:00 pm to 4:00 pm and 8:30 pm to 11:30 pm")),
             (("Domingo", "Sunday"), ("13:00 a 16:30 (noche cerrado)", "1:00 pm to 4:30 pm (closed in the evening)"))]
    cta_cab = marco([llamar(t=("Llamar", "Call")), whatsapp("secondary", t=("WhatsApp", "WhatsApp"))],
                    dir="row", gap=8, wrap=True)
    reservar = lambda v="primary": boton(("Reservar mesa", "Book a table"), a_pagina("reservas", "reservar"), v)
    ver_carta = lambda v="secondary": boton(("Ver la carta", "See the menu"), a_pagina("carta"), v)

    inicio = pagina("inicio", ("", ""), ("Restaurante de cocina de mercado", "Market-cooking restaurant"),
                    ("Restaurante de carta con cocina de mercado: platos de temporada, bodega, eventos y reserva de mesa por formulario, teléfono o WhatsApp.",
                     "À la carte restaurant with market cooking: seasonal dishes, wine list, events and table booking by form, phone or WhatsApp."), [
        portada(("Cocina de mercado, hecha sin prisa", "Market cooking, done without hurry"),
                ("Platos de temporada, una carta corta y una bodega para acompañarla. Reserva mesa para comer o para cenar.",
                 "Seasonal dishes, a short menu and a wine list to go with it. Book a table for lunch or dinner."),
                [reservar(), llamar("secondary"), whatsapp("ghost")],
                ("Restaurante de carta", "À la carte restaurant"),
                ("Sala del restaurante con mesas vestidas y luz cálida", "Restaurant dining room with set tables and warm light")),
        bloque_valores(("Así cocinamos", "How we cook"), [
            (("Lo que hay en el mercado", "What is in the market"),
             ("Compramos lo que está en su mejor momento y la carta cambia con la temporada. Si un plato se acaba, se acaba.",
              "We buy what is at its best and the menu changes with the season. When a dish runs out, it runs out.")),
            (("Fuego y tiempo", "Fire and time"),
             ("Guisos de cocción larga, brasa para el pescado y la carne, y salsas hechas en casa cada mañana.",
              "Slow-cooked stews, charcoal for fish and meat, and sauces made in-house every morning.")),
            (("Una carta corta", "A short menu"),
             ("Pocos platos y bien hechos. Si no sabes qué pedir, te aconsejamos según tu hambre y tu presupuesto.",
              "Few dishes, done well. If you do not know what to order, we advise you by appetite and budget.")),
        ], estilo="linea", eyebrow=("La cocina", "The kitchen")),
        seccion("features", [
            titular(("De la carta", "From the menu"),
                    ("Platos y precios de ejemplo: cámbialos por los tuyos y sustituye las fotos por las de tus platos.",
                     "Example dishes and prices: replace them with your own and swap the photos for your dishes.")),
            rejilla([
                plato_destacado(("Arroz de temporada", "Seasonal rice"),
                                ("Caldo de la casa, producto del día y punto meloso. Mínimo dos personas.",
                                 "House stock, the day's produce and a creamy finish. Minimum two people."),
                                ("Desde 19 € por persona", "From €19 per person"),
                                ("Arroz meloso servido en cazuela", "Creamy rice served in a casserole dish")),
                plato_destacado(("Pescado del día a la brasa", "Charcoal-grilled fish of the day"),
                                ("Según lo que llegue a la lonja. Lo servimos entero o en lomos.",
                                 "Depending on what reaches the fish market. Served whole or filleted."),
                                ("Precio según pieza", "Price by piece"),
                                ("Pescado a la brasa con guarnición de verduras", "Charcoal-grilled fish with vegetables")),
                plato_destacado(("Guiso de la casa", "House stew"),
                                ("El plato de cuchara de la semana, con su pan y su aceite.",
                                 "The week's spoon dish, with bread and oil."),
                                (eur(16)[0], eur(16)[1]),
                                ("Plato de guiso humeante con pan", "Steaming stew with bread")),
            ], cols=3),
            boton(("Ver la carta completa", "See the full menu"), a_pagina("carta"), "secondary"),
        ], tono="relleno", gap=32, anchor="destacados"),
        seccion("text", [marco([
            titular(("Una bodega para acompañar", "A wine list to go with the food"),
                    ("Vinos de la zona y de fuera, por copa y por botella. Dinos qué vas a comer y qué te gusta beber y te proponemos dos o tres.",
                     "Wines from the area and further afield, by the glass and by the bottle. Tell us what you are eating and what you like and we suggest two or three."),
                    tono="oscuro"),
            boton(("Ver la bodega", "See the wine list"), a_pagina("carta", "bodega"), "primary")],
            gap=20, sz={"width": "fill", "height": "hug", "max_width": 680})], tono="oscuro", gap=0, anchor="bodega-inicio"),
        bloque_opiniones(("Lo que dicen quienes han comido aquí", "What our diners say"),
                         [("la cocina", "the food"), ("el servicio", "the service"), ("el ambiente", "the atmosphere")],
                         tono="base"),
        bloque_cta(("¿Os guardamos mesa?", "Shall we keep you a table?"),
                   ("Dinos el día, la hora y cuántos sois. Si es una celebración, cuéntanoslo y lo preparamos.",
                    "Tell us the day, the time and how many of you there are. If it is a celebration, tell us and we will prepare it."),
                   [reservar(), llamar("secondary"), whatsapp("ghost")], tono="relleno"),
    ])

    carta = pagina("carta", ("la-carta", "the-menu"), ("La carta", "The menu"),
                   ("Carta por secciones con descripción y precio de ejemplo, bodega por copa y botella, información de alérgenos y preguntas frecuentes.",
                    "Menu by section with description and example prices, wine list by the glass and bottle, allergen information and frequently asked questions."), [
        hero(("La carta", "The menu"),
             ("Cocina de mercado: la carta cambia con la temporada. Si buscas algo que no ves aquí, pregunta; a veces lo tenemos fuera de carta.",
              "Market cooking: the menu changes with the season. If you are after something you do not see here, ask; sometimes we have it off the menu."),
             [reservar(), llamar("secondary"), whatsapp("ghost")], variante="texto"),
        bloque_carta(("Platos", "Dishes"), [
            (("Para empezar", "To start"), ("Precios de los entrantes", "Starter prices"), CAB_PLATO, [
                plato(("Croquetas caseras (6)", "Home-made croquettes (6)"),
                      ("De cocido o de jamón, según el día.", "Stew or ham, depending on the day."), eur(9)),
                plato(("Ensalada de temporada", "Seasonal salad"),
                      ("Verduras del mercado, aceite de oliva y vinagre de la casa.", "Market vegetables, olive oil and house vinegar."), eur(11)),
                plato(("Tabla de embutidos y queso", "Cured meat and cheese board"),
                      ("Para compartir, con pan.", "To share, with bread."), eur(16))]),
            (("Del mar", "From the sea"), ("Precios de los platos de pescado", "Fish dish prices"), CAB_PLATO, [
                plato(("Bacalao confitado", "Confit cod"),
                      ("Con pisto de verduras.", "With a vegetable stew."), eur(19)),
                plato(("Pescado del día a la brasa", "Charcoal-grilled fish of the day"),
                      ("Pregunta qué ha llegado hoy.", "Ask what came in today."), ("Según pieza", "By piece")),
                plato(("Arroz de temporada (mín. 2 personas)", "Seasonal rice (min. 2 people)"),
                      ("Caldo de la casa y punto meloso.", "House stock and a creamy finish."),
                      ("19 € por persona", "€19 per person"))]),
            (("De la tierra", "From the land"), ("Precios de los platos de carne", "Meat dish prices"), CAB_PLATO, [
                plato(("Guiso de la casa", "House stew"),
                      ("El plato de cuchara de la semana.", "The week's spoon dish."), eur(16)),
                plato(("Carrillera melosa", "Slow-cooked beef cheek"),
                      ("Con puré de patata y su salsa.", "With mashed potato and its sauce."), eur(20)),
                plato(("Chuletón a la brasa (para 2)", "Charcoal-grilled T-bone (for 2)"),
                      ("Con pimientos y patatas.", "With peppers and potatoes."), eur(48))]),
            (("Postres", "Desserts"), ("Precios de los postres", "Dessert prices"), CAB_PLATO, [
                plato(("Torrija caramelizada", "Caramelised French toast"),
                      ("Con helado de vainilla.", "With vanilla ice cream."), eur(7)),
                plato(("Flan de la casa", "House crème caramel"),
                      ("Con nata.", "With cream."), eur(6)),
                plato(("Queso con membrillo", "Cheese with quince paste"),
                      ("Curado, con membrillo casero.", "Cured, with home-made quince paste."), eur(8))]),
        ], nota=NOTA, tono="base", anchor="platos"),
        bloque_precios(("Bodega", "Wine list"), ("Precios de los vinos", "Wine prices"),
                       [("Vino", "Wine"), ("Tipo", "Type"), ("Copa", "Glass"), ("Botella", "Bottle")],
                       [(("Tinto joven de la zona", "Young local red"), ("Tinto", "Red"), eur(3.5), eur(16)),
                        (("Tinto crianza", "Aged red"), ("Tinto", "Red"), eur(4.5), eur(22)),
                        (("Blanco de la zona", "Local white"), ("Blanco", "White"), eur(3.5), eur(17)),
                        (("Rosado", "Rosé"), ("Rosado", "Rosé"), eur(3.5), eur(16)),
                        (("Espumoso", "Sparkling wine"), ("Espumoso", "Sparkling"), eur(5), eur(24))],
                       intro=("Escribe aquí tus vinos y sus precios: estos son de ejemplo. Si tienes bodega más larga, añade una fila por vino.",
                              "Write your wines and prices here: these are examples. If your list is longer, add one row per wine."),
                       nota=NOTA, anchor="bodega", tono="relleno"),
        bloque_texto(("Alérgenos e intolerancias", "Allergens and intolerances"), [
            ("Si tienes alguna alergia o intolerancia, dínoslo antes de pedir. Te enseñamos por escrito los alérgenos de cada plato.",
             "If you have an allergy or an intolerance, tell us before you order. We show you in writing the allergens in each dish."),
            ("La normativa obliga a informar de 14 grupos de alérgenos, como gluten, lácteos, huevo, frutos secos, pescado o marisco. Si dudas de un plato, pregunta.",
             "The rules require us to declare 14 groups of allergens, such as gluten, dairy, egg, nuts, fish or shellfish. If you are unsure about a dish, ask."),
        ], eyebrow=("Información", "Information"), anchor="alergenos"),
        bloque_faq(("Preguntas frecuentes", "Frequently asked questions"), [
            (("¿Tenéis opciones vegetarianas?", "Do you have vegetarian options?"),
             ("Siempre hay platos sin carne ni pescado. Dinos qué comes y qué no y te decimos cuáles sirven.",
              "There are always dishes without meat or fish. Tell us what you eat and what you do not and we will say which ones suit you.")),
            (("¿Hay menú para niños?", "Is there a children's menu?"),
             ("Preparamos medias raciones y platos sencillos. Pídelos al sentarte.",
              "We prepare half portions and simple dishes. Ask for them when you sit down.")),
            (("¿Puedo pedir para llevar?", "Can I order takeaway?"),
             ("Sí, de la mayor parte de la carta. Llama con un rato de antelación y te lo preparamos.",
              "Yes, most of the menu. Call a little ahead and we will prepare it.")),
        ], tono="relleno"),
        bloque_cta(("¿Reservamos?", "Shall we book?"), None, [reservar(), llamar("secondary"), whatsapp("ghost")], tono="base"),
    ])

    eventos = pagina("eventos", ("eventos", "events"), ("Eventos y celebraciones", "Events and celebrations"),
                     ("Celebraciones, comidas de empresa y cenas de grupo: menús cerrados, cómo organizarlo en cuatro pasos y precios de ejemplo.",
                      "Celebrations, business lunches and group dinners: set menus, how to organise it in four steps and example prices."), [
        hero(("Eventos y celebraciones", "Events and celebrations"),
             ("Comuniones, cumpleaños, comidas de empresa o una cena entre amigos. Preparamos un menú cerrado y te lo damos por escrito.",
              "Communions, birthdays, business lunches or dinner with friends. We prepare a set menu and give it to you in writing."),
             [boton(("Pedir presupuesto", "Ask for a quote"), a_pagina("reservas", "reservar")), llamar("secondary"), whatsapp("ghost")],
             variante="texto", tono="relleno"),
        bloque_valores(("Qué celebramos aquí", "What we celebrate here"), [
            (("Familia", "Family"),
             ("Comuniones, bautizos y cumpleaños. Menú infantil aparte y tarta si la traes.",
              "Communions, christenings and birthdays. Separate children's menu and cake if you bring one.")),
            (("Empresa", "Business"),
             ("Comidas de equipo, cenas de Navidad y reuniones con comida. Factura a nombre de la empresa.",
              "Team lunches, Christmas dinners and working meals. Invoice in the company's name.")),
            (("Amigos", "Friends"),
             ("Cenas de grupo, despedidas y aniversarios, con mesa larga y un menú para compartir.",
              "Group dinners, farewells and anniversaries, with a long table and a menu to share.")),
        ], estilo="borde", eyebrow=("Tipos de evento", "Types of event")),
        bloque_checks(("Sala y condiciones", "Room and terms"), [
            ("Salón privado para hasta 30 personas (de ejemplo: pon la capacidad real).", "Private room for up to 30 people (example: write the real capacity)."),
            ("Mesa larga o varias mesas redondas, como prefieras.", "One long table or several round ones, as you prefer."),
            ("Menú cerrado acordado con antelación, con alérgenos incluidos.", "Set menu agreed in advance, with allergens included."),
            ("Una señal confirma la fecha y se descuenta de la cuenta.", "A deposit confirms the date and is deducted from the bill."),
        ], tono="relleno", cols=2, anchor="sala"),
        bloque_pasos(("Cómo se organiza", "How it is organised"), [
            (("Cuéntanos", "Tell us"), ("Fecha, hora y cuántos sois, y si hay niños o alergias.",
                                         "Date, time, how many of you there are, and whether there are children or allergies.")),
            (("Te proponemos", "We propose"), ("Un menú y un precio por persona, por escrito.",
                                                 "A menu and a price per person, in writing.")),
            (("Confirmas", "You confirm"), ("Con una señal reservamos el día y el salón.", "With a deposit we reserve the day and the room.")),
            (("Una semana antes", "A week before"), ("Cerramos el número de comensales y los últimos detalles.",
                                                     "We settle the number of guests and the final details.")),
        ], tono="base"),
        bloque_precios(("Menús de grupo", "Group menus"), ("Menús de grupo por persona", "Group menus per person"),
                       [("Menú", "Menu"), ("Incluye", "Includes"), ("Precio por persona", "Price per person")],
                       [(("Menú cerrado", "Set menu"), ("Entrantes para compartir, un plato, postre y bebida", "Shared starters, a main, dessert and drinks"), desde(38)),
                        (("Menú degustación", "Tasting menu"), ("Seis pases y maridaje de vinos", "Six courses and wine pairing"), desde(65)),
                        (("Menú infantil", "Children's menu"), ("Un plato, postre y bebida", "A main, dessert and a drink"), eur(18)),
                        (("Alquiler del salón", "Room hire"), ("Solo para grupos pequeños fuera de comida", "Only for small groups outside meal times"), desde(150))],
                       nota=NOTA, anchor="menus", tono="relleno"),
        bloque_cta(("¿Qué vais a celebrar?", "What are you celebrating?"),
                   ("Cuéntanoslo y te respondemos con un menú y un precio.",
                    "Tell us and we will reply with a menu and a price."),
                   [boton(("Pedir presupuesto", "Ask for a quote"), a_pagina("reservas", "reservar")), whatsapp("ghost")], tono="base"),
    ])

    reservas = pagina("reservas", ("reservar-mesa", "book-a-table"), ("Reservar mesa y cómo llegar", "Book a table and how to find us"),
                      ("Reserva con fecha, hora y número de comensales. Horario de comidas y cenas, teléfono, WhatsApp y mapa.",
                       "Book with a date, a time and the number of diners. Lunch and dinner hours, phone, WhatsApp and a map."), [
        hero(("Reserva tu mesa", "Book your table"),
             ("Dinos el día, la hora y cuántos sois. Te confirmamos por teléfono o WhatsApp el mismo día.",
              "Tell us the day, the time and how many of you there are. We confirm by phone or WhatsApp the same day."),
             [llamar(), whatsapp()], variante="texto"),
        bloque_contacto(("Tu reserva", "Your booking"),
                        ("Para grupos de más de 8, llámanos o mira la página de eventos.",
                         "For groups of more than 8, call us or see the events page."),
                        [campo("nombre", "text", ("Nombre", "Name"), True, auto="name", maxlen=120),
                         campo("telefono", "tel", ("Teléfono", "Phone"), True, auto="tel", maxlen=30),
                         campo("fecha", "date", ("Día", "Day"), True),
                         campo("hora", "select", ("Hora", "Time"), True,
                               opciones=[("Comida, 13:30", "Lunch, 1:30 pm"), ("Comida, 14:00", "Lunch, 2:00 pm"),
                                         ("Comida, 14:30", "Lunch, 2:30 pm"), ("Cena, 21:00", "Dinner, 9:00 pm"),
                                         ("Cena, 21:30", "Dinner, 9:30 pm"), ("Cena, 22:00", "Dinner, 10:00 pm")]),
                         campo("comensales", "select", ("Comensales", "Diners"), True,
                               opciones=[(str(n), str(n)) for n in range(1, 9)] + [("Más de 8", "More than 8")]),
                         campo("mensaje", "textarea", ("Alergias, trona o celebración (opcional)", "Allergies, high chair or celebration (optional)"), False, maxlen=1000)],
                        ("Reservar mesa", "Request a table"), horas, anchor="reservar",
                        exito=("Gracias. Te confirmamos la reserva por teléfono o WhatsApp en cuanto la veamos.",
                               "Thank you. We will confirm your booking by phone or WhatsApp as soon as we see it.")),
        bloque_checks(("Antes de venir", "Before you come"), [
            ("Guardamos la mesa 15 minutos. Si vas a llegar tarde, avísanos.", "We keep your table for 15 minutes. If you will be late, let us know."),
            ("Si cambian los planes, avísanos con un día y se la damos a otra persona.", "If your plans change, tell us a day ahead and we will offer it to someone else."),
            ("Para alergias o intolerancias, dínoslo al reservar y lo tenemos preparado.", "For allergies or intolerances, tell us when you book and we will have it ready."),
            ("Trona y sitio para el carrito, si nos avisas.", "High chair and room for a pushchair, if you tell us."),
        ], eyebrow=("Reservas", "Bookings"), tono="relleno", anchor="antes-de-venir"),
        bloque_mapa(tono="base"),
    ])

    return plantilla(KEY, ("Restaurante", "Restaurant"),
                     ("Restaurante de carta con cocina de mercado: carta por secciones con precios de ejemplo, bodega, alérgenos, eventos, reserva de mesa con fecha, hora y comensales, horario y mapa.",
                      "À la carte restaurant with market cooking: menu by section with example prices, wine list, allergens, events, table booking with date, time and diners, opening hours and a map."),
                     "comida", ("restaurante restaurantes carta a la carta cocina de mercado cocina tradicional comer cenar reservar mesa reservas bodega vinos celebraciones comuniones comida de empresa eventos alergenos",
                                "restaurant restaurants menu a la carte market cuisine traditional cooking lunch dinner book a table reservations wine list celebrations events allergens"),
                     look, cabecera(nav, cta_cab),
                     pie(nav, ("Restaurante de carta con cocina de mercado, bodega y salón para celebraciones.",
                               "À la carte restaurant with market cooking, a wine list and a room for celebrations."),
                         ("Martes a sábado: comidas de 13:00 a 16:00 y cenas desde las 20:30. Domingo solo comidas. Lunes cerrado.",
                          "Tuesday to Saturday: lunch 1 pm to 4 pm and dinner from 8:30 pm. Sunday lunch only. Closed Monday.")),
                     [inicio, carta, eventos, reservas])
