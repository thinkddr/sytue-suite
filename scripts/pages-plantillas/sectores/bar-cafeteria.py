# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
from lib import *
from looks import LOOKS

KEY = "bar-cafeteria"

NOTA_PRECIOS = ("Precios de ejemplo: cámbialos por los tuyos. Incluyen IVA.",
                "Example prices: replace them with your own. VAT included.")
PROD = [("Producto", "Item"), ("Precio", "Price")]


def build():
    nueva(KEY)
    look = LOOKS["terracota"]
    nav = [("inicio", ("Inicio", "Home")), ("carta", ("Carta y precios", "Menu and prices")),
           ("local", ("El local y la terraza", "The bar and terrace")), ("contacto", ("Contacto", "Contact"))]
    horas = [(("Lunes a viernes", "Monday to Friday"), ("7:00 a 23:00", "7:00 am to 11:00 pm")),
             (("Sábado", "Saturday"), ("8:00 a 00:00", "8:00 am to midnight")),
             (("Domingo", "Sunday"), ("9:00 a 17:00", "9:00 am to 5:00 pm"))]
    cta_cab = marco([llamar(), whatsapp("ghost")], dir="row", gap=8, wrap=True, align="center")
    a_carta = a_pagina("carta")

    inicio = pagina("inicio", ("", ""), ("Bar y cafetería", "Bar and café"),
                    ("Bar y cafetería: desayunos, tapas y raciones, terraza y horario largo. Reserva mesa por teléfono o WhatsApp.",
                     "Bar and café: breakfasts, tapas and sharing plates, terrace and long opening hours. Book a table by phone or WhatsApp."), [
        hero(("Tu bar de siempre, de la mañana a la noche", "Your local bar, from morning to night"),
             ("Desayunos, bocadillos, tapas y raciones. Con terraza y abierto muchas horas, para el café de las siete o la caña de después.",
              "Breakfasts, sandwiches, tapas and sharing plates. With a terrace and long opening hours, for the seven o'clock coffee or the beer after work."),
             [boton(("Ver la carta", "See the menu"), a_carta), llamar("secondary"), whatsapp("ghost")],
             eyebrow=("Bar y cafetería", "Bar and café"),
             imagen_=("landscape", ("Terraza del bar con mesas y sillas al aire libre", "Bar terrace with outdoor tables and chairs"))),
        bloque_valores(("Qué tomar a cada hora", "What to have at each hour"), [
            (("Desayunos", "Breakfasts"), ("Café, tostadas, bollería y zumo de naranja.", "Coffee, toast, pastries and orange juice.")),
            (("Bocadillos y platos", "Sandwiches and dishes"), ("Para el almuerzo o la comida: bocadillos calientes y fríos.", "For a mid-morning or lunch: hot and cold sandwiches.")),
            (("Tapas y raciones", "Tapas and sharing plates"), ("Para compartir en la barra o en la terraza.", "To share at the bar or on the terrace.")),
            (("Meriendas y cañas", "Afternoon snacks and drinks"), ("Café con leche con algo dulce, o una caña fresca.", "A milky coffee with something sweet, or a cold beer.")),
        ], cols=4, estilo="linea"),
        bloque_texto(("La terraza", "The terrace"), [
            ("Mesas fuera para cuando hace bueno. Para grupos grandes, reserva mesa y te la dejamos preparada.",
             "Tables outside for when the weather is good. For large groups, book a table and we will have it ready.")],
            eyebrow=("Terraza", "Terrace"), tono="relleno", invertir=True,
            imagen_=("photo", ("Mesas de la terraza con bebidas y tapas", "Terrace tables with drinks and tapas"))),
        bloque_precios(("Precios de siempre", "The usual prices"), ("Algunos precios de la carta", "A few menu prices"), PROD,
                       [(("Café con leche", "Coffee with milk"), eur(1.6)), (("Tostada con tomate y aceite", "Toast with tomato and olive oil"), eur(2.2)),
                        (("Caña", "Small draught beer"), eur(1.8)), (("Ración de tortilla de patata", "Portion of Spanish omelette"), eur(9))],
                       nota=NOTA_PRECIOS, boton_=boton(("Ver toda la carta", "See the full menu"), a_carta, "secondary"),
                       tono="base", anchor="precios"),
        bloque_opiniones(("Lo que dicen de nosotros", "What people say about us"),
                         [("el café", "the coffee"), ("las tapas", "the tapas"), ("la terraza", "the terrace")]),
        bloque_cta(("¿Te guardamos una mesa?", "Shall we keep you a table?"),
                   ("Llámanos o escríbenos con el día, la hora y cuántos sois.", "Call or message us with the day, time and how many of you there are.")),
    ])

    carta = pagina("carta", ("carta", "menu"), ("Carta y precios", "Menu and prices"),
                   ("Precios de desayunos, bocadillos, tapas y raciones y bebidas, con información de alérgenos.",
                    "Prices for breakfasts, sandwiches, tapas and sharing plates and drinks, with allergen information."), [
        hero(("Carta y precios", "Menu and prices"),
             ("Todo lo que servimos, con su precio. Si no ves algo que buscas, pregúntalo en la barra.",
              "Everything we serve, with its price. If you cannot see what you are after, ask at the bar."),
             [llamar("secondary"), whatsapp("ghost")], variante="texto"),
        bloque_carta(("Desayunos y meriendas", "Breakfasts and afternoon snacks"), [
            (("Café y desayunos", "Coffee and breakfasts"), ("Precios de café y desayunos", "Coffee and breakfast prices"), PROD,
             [(("Café solo", "Black coffee"), eur(1.4)), (("Café con leche", "Coffee with milk"), eur(1.6)),
              (("Tostada con tomate y aceite", "Toast with tomato and olive oil"), eur(2.2)),
              (("Tostada con jamón", "Toast with ham"), eur(3.2)), (("Zumo de naranja natural", "Fresh orange juice"), eur(2.8)),
              (("Desayuno completo: café, tostada y zumo", "Full breakfast: coffee, toast and juice"), eur(4.5))]),
        ], tono="base", anchor="desayunos"),
        bloque_carta(("Bocadillos, tapas y raciones", "Sandwiches, tapas and sharing plates"), [
            (("Bocadillos", "Sandwiches"), ("Precios de bocadillos", "Sandwich prices"), PROD,
             [(("Bocadillo de tortilla", "Omelette sandwich"), eur(4.5)), (("Bocadillo de jamón", "Ham sandwich"), eur(4.8)),
              (("Bocadillo de calamares", "Fried squid sandwich"), eur(6))]),
            (("Tapas y raciones", "Tapas and sharing plates"), ("Precios de tapas y raciones", "Tapas and sharing plate prices"), PROD,
             [(("Patatas bravas", "Spicy fried potatoes"), eur(6.5)), (("Croquetas caseras (8)", "Home-made croquettes (8)"), eur(9)),
              (("Ensaladilla rusa", "Russian salad"), eur(7)), (("Calamares a la romana", "Fried squid rings"), eur(11)),
              (("Tortilla de patata", "Spanish omelette"), eur(9))]),
        ], tono="relleno", anchor="tapas"),
        bloque_carta(("Bebidas", "Drinks"), [
            (("Cañas, vinos y refrescos", "Beers, wines and soft drinks"), ("Precios de bebidas", "Drink prices"), PROD,
             [(("Caña", "Small draught beer"), eur(1.8)), (("Refresco", "Soft drink"), eur(2.2)),
              (("Copa de vino de la casa", "Glass of house wine"), eur(2.5)), (("Agua", "Water"), eur(1.5))]),
        ], nota=NOTA_PRECIOS, tono="base", anchor="bebidas"),
        bloque_texto(("Alérgenos", "Allergens"), [
            ("Si tienes alergia o intolerancia, avísanos antes de pedir. Te enseñamos por escrito qué alérgenos lleva cada plato.",
             "If you have an allergy or an intolerance, tell us before you order. We show you in writing which allergens each dish contains.")],
            tono="relleno", anchor="alergenos"),
        bloque_cta(("¿Vienes a probarlo?", "Coming to try it?"), None, tono="base"),
    ])

    local = pagina("local", ("el-local", "the-bar"), ("El local y la terraza", "The bar and terrace"),
                   ("La terraza, la barra, las mesas para grupos, los partidos y eventos, y preguntas frecuentes.",
                    "The terrace, the bar, tables for groups, matches and events, and frequently asked questions."), [
        hero(("El local y la terraza", "The bar and terrace"),
             ("Barra para el café de paso, mesas para sentarse y terraza para cuando apetece estar fuera.",
              "A bar for a quick coffee, tables to sit at and a terrace for when you want to be outside."),
             [llamar("secondary"), whatsapp("ghost")], variante="texto"),
        bloque_valores(("Dónde sentarte", "Where to sit"), [
            (("La barra", "The bar"), ("Para el café de paso o la caña de pie.", "For a quick coffee or a beer standing up.")),
            (("Las mesas", "The tables"), ("Para comer o desayunar con calma.", "To eat or have breakfast at your own pace.")),
            (("La terraza", "The terrace"), ("Mesas fuera, con sol por la mañana o fresco por la tarde.", "Outdoor tables, in the sun in the morning or in the cool of the evening.")),
        ], tono="base", estilo="relleno"),
        bloque_texto(("Partidos y eventos", "Matches and events"), [
            ("Cuando retransmitimos un partido o preparamos un evento, lo avisamos en el cartel de la puerta y por WhatsApp.",
             "When we show a match or are putting on an event, we let you know on the sign at the door and on WhatsApp."),
            ("Si vais en grupo, reserva mesa con antelación.", "If you are going as a group, book a table in advance.")],
            tono="relleno",
            imagen_=("photo", ("Grupo de amigos en la barra del bar", "Group of friends at the bar counter"))),
        bloque_galeria(("El bar por dentro y por fuera", "The bar inside and out"), [
            ("Barra del bar con vasos y cafetera", "Bar counter with glasses and coffee machine"),
            ("Mesas de la terraza al sol", "Terrace tables in the sun"),
            ("Tapas servidas en la barra", "Tapas served at the bar"),
            ("Desayuno con café y tostada", "Breakfast with coffee and toast"),
            ("Interior del local con mesas", "Inside the bar with tables"),
            ("Terraza por la tarde", "Terrace in the afternoon")], tono="base"),
        bloque_faq(("Preguntas frecuentes", "Frequently asked questions"), [
            (("¿Se puede reservar mesa?", "Can I book a table?"),
             ("Sí. Llama o escribe por WhatsApp con el día, la hora y cuántos sois. Para dos o tres personas, normalmente no hace falta.",
              "Yes. Call or message us on WhatsApp with the day, time and how many of you there are. For two or three people it is usually not needed.")),
            (("¿Hasta qué hora hay desayunos?", "Until what time is breakfast served?"),
             ("Hasta las 12:00. Después, bocadillos, tapas y raciones.", "Until noon. After that, sandwiches, tapas and sharing plates.")),
            (("¿Hay opciones para alérgicos?", "Are there options for people with allergies?"),
             ("Díselo a quien te atienda antes de pedir y te decimos qué lleva cada cosa.",
              "Tell whoever is serving you before you order and we will tell you what each item contains.")),
        ], tono="relleno"),
    ])

    cont = pagina("contacto", ("contacto", "contact"), ("Contacto y reservas", "Contact and bookings"),
                  ("Reserva mesa por teléfono, WhatsApp o con el formulario, y mira el horario y cómo llegar.",
                   "Book a table by phone, WhatsApp or with the form, and see opening hours and how to get here."), [
        hero(("Reserva tu mesa", "Book your table"),
             ("Llámanos, escríbenos por WhatsApp o rellena el formulario. Te confirmamos la reserva en el día.",
              "Call us, message us on WhatsApp or fill in the form. We confirm your booking within the day."),
             [llamar(), whatsapp()], variante="texto"),
        bloque_contacto(("Cómo encontrarnos", "How to find us"),
                        ("Pasa por la barra o reserva mesa en la terraza.", "Come to the bar or book a table on the terrace."),
                        campos_cita(servicios=[("Mesa en la terraza", "Terrace table"), ("Mesa dentro", "Table inside"),
                                               ("Grupo o evento", "Group or event")],
                                    extra=[campo("personas", "text", ("Número de personas", "Number of people"), True, maxlen=10)]),
                        ("Reservar mesa", "Book a table"), horas,
                        exito=("Gracias. Te confirmamos la reserva por teléfono o WhatsApp en cuanto la tengamos.",
                               "Thank you. We will confirm your booking by phone or WhatsApp as soon as we have it.")),
        bloque_mapa(tono="relleno"),
    ])

    return plantilla(KEY, ("Bar y cafetería", "Bar and café"),
                     ("Desayunos, tapas y raciones, tabla de precios, terraza y horario largo, con botones de llamar y WhatsApp.",
                      "Breakfasts, tapas and sharing plates, price list, terrace and long opening hours, with call and WhatsApp buttons."),
                     "comida", ("bar cafeteria cafe cafeteria desayunos tapas raciones bocadillos cervezas terraza tasca taberna pub reservas",
                                "bar cafe coffee shop breakfast tapas terrace pub sandwiches drinks"),
                     look, cabecera(nav, cta_cab),
                     pie(nav, ("Bar y cafetería con terraza. Desayunos, tapas y raciones.",
                               "Bar and café with a terrace. Breakfasts, tapas and sharing plates."),
                         ("Lunes a viernes 7:00 a 23:00. Sábado 8:00 a 00:00. Domingo 9:00 a 17:00.",
                          "Monday to Friday 7 am to 11 pm. Saturday 8 am to midnight. Sunday 9 am to 5 pm.")),
                     [inicio, carta, local, cont])
