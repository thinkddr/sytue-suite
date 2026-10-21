# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
from lib import *
from looks import LOOKS

KEY = "menu-del-dia"

NOTA_PRECIOS = ("Precios de ejemplo: cámbialos por los tuyos. Incluyen IVA.",
                "Example prices: replace them with your own. VAT included.")


def menu_dia(titulo, intro, anchor="menu-de-hoy", tono="base", eyebrow=None):
    """El menú del día: primeros, segundos y postres, con el precio y lo que incluye."""
    def col(nombre, platos):
        return tarjeta([h3(nombre), marco([marco([p(x)], tag="li", gap=0) for x in platos], tag="ul", gap=8)],
                       estilo="borde", gap=10, tag="div")

    cols = rejilla([
        col(("Primeros", "Starters"), [
            ("Lentejas con verduras", "Lentil stew with vegetables"),
            ("Ensalada mixta", "Mixed salad"),
            ("Macarrones con tomate", "Macaroni with tomato sauce")]),
        col(("Segundos", "Main courses"), [
            ("Merluza a la plancha con patatas", "Grilled hake with potatoes"),
            ("Pollo guisado con arroz", "Stewed chicken with rice"),
            ("Tortilla de patata con ensalada", "Spanish omelette with salad")]),
        col(("Postres", "Desserts"), [
            ("Flan casero", "Home-made crème caramel"),
            ("Fruta de temporada", "Seasonal fruit"),
            ("Yogur o café", "Yoghurt or coffee")]),
    ], cols=3, lista=False)
    precio = tarjeta([
        texto("h3", ("Menú completo: 13,50 €", "Full menu: €13.50")),
        p(("Incluye primero, segundo, postre o café, pan y bebida.",
           "Includes a starter, a main course, dessert or coffee, bread and a drink."), color="$muted"),
        peq(("Los platos de arriba son un ejemplo: escribe aquí los de cada día.",
             "The dishes above are an example: write here the ones for each day."), color="$muted"),
        peq(NOTA_PRECIOS, color="$muted"),
    ], estilo="relleno", tag="div", gap=8)
    return seccion("pricing", [titular(titulo, intro, eyebrow), cols, precio], tono=tono, anchor=anchor, gap=28)


def sec_form(titulo, intro, campos, envio, exito, anchor, tono="base"):
    return seccion("contact", [titular(titulo, intro),
                               tarjeta([formulario(campos, envio, exito)], estilo="relleno" if tono == "base" else "borde",
                                       pad=28, tag="div", sz=tam(max_width=640))],
                   tono=tono, anchor=anchor, gap=28)


def build():
    nueva(KEY)
    look = LOOKS["ladrillo"]
    nav = [("inicio", ("Inicio", "Home")), ("menu", ("Menú y carta", "Menu")),
           ("grupos", ("Grupos y encargos", "Groups and orders")), ("contacto", ("Contacto", "Contact"))]
    horas = [(("Lunes a viernes", "Monday to Friday"), ("8:00 a 17:00 (comidas de 13:00 a 16:00)", "8:00 am to 5:00 pm (lunch 1:00 to 4:00 pm)")),
             (("Sábado", "Saturday"), ("13:00 a 17:00", "1:00 pm to 5:00 pm")),
             (("Domingo", "Sunday"), ("Cerrado", "Closed"))]
    cta_cab = marco([llamar(), whatsapp("ghost")], dir="row", gap=8, wrap=True, align="center")
    a_menu = a_pagina("menu", "menu-de-hoy")
    a_reserva = a_pagina("contacto", "contacto")

    inicio = pagina("inicio", ("", ""), ("Restaurante de menú del día", "Set lunch restaurant"),
                    ("Menú del día y cocina casera: precio, platos y qué incluye. Reserva por teléfono o WhatsApp.",
                     "Set lunch menu and home cooking: price, dishes and what is included. Book by phone or WhatsApp."), [
        hero(("El menú del día, como en casa", "The set lunch, just like at home"),
             ("Primero, segundo, postre, pan y bebida a un precio cerrado. Los platos cambian cada día: mira el de hoy o reserva mesa.",
              "A starter, a main course, dessert, bread and a drink at one fixed price. The dishes change every day: see today's menu or book a table."),
             [boton(("Ver el menú de hoy", "See today's menu"), a_menu), llamar("secondary"), whatsapp("ghost")],
             eyebrow=("Cocina casera", "Home cooking"), variante="texto", tono="relleno"),
        menu_dia(("Menú de hoy", "Today's menu"),
                 ("Se sirve a la hora de comer. Si tienes alguna alergia o intolerancia, díselo a quien te atienda.",
                  "Served at lunchtime. If you have an allergy or an intolerance, tell whoever is serving you.")),
        bloque_valores(("Comer aquí", "Eating here"), [
            (("Platos de cada día", "Dishes of the day"),
             ("Guisos y platos de cuchara cuando hace frío, ensaladas y plancha cuando hace calor.",
              "Stews and spoon dishes when it is cold, salads and grilled food when it is warm.")),
            (("Si tienes poco tiempo", "If you are short of time"),
             ("Dilo al sentarte y te servimos para que llegues a tu hora.",
              "Tell us when you sit down and we will serve you so you are back on time.")),
            (("Alergias e intolerancias", "Allergies and intolerances"),
             ("Cuéntanos lo que no puedes comer y te decimos qué platos del día te sirven.",
              "Tell us what you cannot eat and we will tell you which dishes of the day suit you.")),
        ], tono="relleno", estilo="borde"),
        bloque_cta(("¿Venís en grupo o necesitas comida para llevar?", "Coming as a group, or need food to take away?"),
                   ("Reservamos mesa para grupos y preparamos comidas por encargo. Mira cómo pedirlo.",
                    "We book tables for groups and prepare meals to order. See how to ask."),
                   [boton(("Grupos y encargos", "Groups and orders"), a_pagina("grupos")), whatsapp("ghost")], tono="base"),
        bloque_opiniones(("Lo que dicen quienes comen aquí", "What our diners say"),
                         [("el menú", "the set menu"), ("el trato", "the service"), ("los postres", "the desserts")]),
        bloque_cta(("¿Te guardamos mesa?", "Shall we keep you a table?"),
                   ("Llámanos o escríbenos con la hora y cuántos sois.", "Call or message us with the time and how many of you there are.")),
    ])

    menu_p = pagina("menu", ("menu", "menu"), ("Menú del día y carta", "Set menu and à la carte"),
                    ("Menú del día con precio y qué incluye, carta corta, alérgenos y preguntas frecuentes.",
                     "Set menu with price and what is included, short à la carte menu, allergens and frequently asked questions."), [
        hero(("Menú del día y carta", "Set menu and à la carte"),
             ("El menú de cada día, la carta corta para quien quiera otra cosa y la información de alérgenos.",
              "Each day's menu, a short à la carte list for anyone who wants something else, and allergen information."),
             [llamar("secondary"), whatsapp("ghost")], variante="texto"),
        menu_dia(("Menú de hoy", "Today's menu"),
                 ("Cambia cada día. Puedes cambiar un plato por otro de la carta si lo pides al hacer la comanda.",
                  "It changes every day. You can swap a dish for one from the à la carte list if you ask when you order.")),
        bloque_carta(("Carta corta", "Short à la carte menu"), [
            (("Para compartir", "To share"), ("Precios para compartir", "Sharing prices"),
             [("Plato", "Dish"), ("Precio", "Price")],
             [(("Ensaladilla rusa", "Russian salad"), eur(7)), (("Croquetas caseras (8)", "Home-made croquettes (8)"), eur(9)),
              (("Calamares a la romana", "Fried squid rings"), eur(11))]),
            (("Platos", "Main dishes"), ("Precios de platos", "Main dish prices"),
             [("Plato", "Dish"), ("Precio", "Price")],
             [(("Filete de ternera con patatas", "Veal steak with potatoes"), eur(13)),
              (("Hamburguesa casera con patatas", "Home-made burger with chips"), eur(11)),
              (("Ensalada completa", "Full salad"), eur(9))]),
        ], nota=NOTA_PRECIOS, tono="relleno", anchor="carta"),
        bloque_texto(("Alérgenos", "Allergens"), [
            ("Si tienes alergia o intolerancia, avísanos antes de pedir. Te enseñamos por escrito qué alérgenos lleva cada plato.",
             "If you have an allergy or an intolerance, tell us before you order. We show you in writing which allergens each dish contains."),
            ("Es la norma para todos los restaurantes: son 14 grupos de alérgenos y se informa de cada uno.",
             "It is the rule for every restaurant: there are 14 groups of allergens and each one has to be declared.")],
            anchor="alergenos"),
        bloque_faq(("Preguntas frecuentes", "Frequently asked questions"), [
            (("¿Qué incluye el menú del día?", "What does the set menu include?"),
             ("Primero, segundo, postre o café, pan y bebida. Si quieres otra cosa, se paga aparte.",
              "A starter, a main course, dessert or coffee, bread and a drink. Anything else is charged separately.")),
            (("¿Hay opciones vegetarianas?", "Are there vegetarian options?"),
             ("Dinos qué no comes y te decimos qué platos del día te sirven. Si no hay ninguno, te proponemos algo de la carta.",
              "Tell us what you do not eat and we will say which dishes of the day suit you. If none does, we suggest something from the list.")),
            (("¿Se puede pedir para llevar?", "Can I order to take away?"),
             ("Sí, el menú y la carta. Llama antes y te lo dejamos listo a la hora que digas.",
              "Yes, both the set menu and the à la carte list. Call ahead and we will have it ready at the time you say.")),
            (("¿Hay que reservar?", "Do I need to book?"),
             ("Para menos de seis personas no hace falta. Para más, reserva con un día de antelación.",
              "Not for fewer than six people. For more, book a day ahead.")),
        ], tono="relleno"),
        bloque_cta(("¿Reservamos tu mesa?", "Shall we book your table?"), None, tono="base"),
    ])

    grupos = pagina("grupos", ("grupos", "groups"), ("Grupos y comidas por encargo", "Groups and meals to order"),
                    ("Reserva de grupos, menú de grupo y comidas por encargo, con precios y formulario.",
                     "Group bookings, group menu and meals to order, with prices and a form."), [
        hero(("Grupos y comidas por encargo", "Groups and meals to order"),
             ("Cumpleaños, comidas de empresa, de familia o de amigos. Te decimos precio y menú antes de que reserves.",
              "Birthdays, work lunches, family or friends. We give you the price and the menu before you book."),
             [llamar("secondary"), whatsapp("ghost")], variante="texto"),
        bloque_pasos(("Cómo reservar un grupo", "How to book a group"), [
            (("Dinos cuántos sois", "Tell us how many"), ("Fecha, hora y número de personas, por teléfono, WhatsApp o el formulario.",
                                                          "Date, time and number of people, by phone, WhatsApp or the form.")),
            (("Elige el menú", "Choose the menu"), ("Te proponemos un menú cerrado y cambiamos lo que haga falta.",
                                                    "We suggest a fixed menu and change whatever you need.")),
            (("Cuéntanos las alergias", "Tell us about allergies"), ("Avísanos de intolerancias o dietas de cualquier persona del grupo.",
                                                                    "Let us know about intolerances or diets of anyone in the group.")),
            (("Confirmamos", "We confirm"), ("Te escribimos con el menú, el precio y la hora. Si cambia el número, avísanos con 48 horas.",
                                             "We write back with the menu, price and time. If the number changes, let us know 48 hours ahead.")),
        ], tono="relleno"),
        bloque_precios(("Menús de grupo y por encargo", "Group and take-away menus"), ("Precios de grupos y encargos", "Group and order prices"),
                       [("Servicio", "Service"), ("Precio por persona", "Price per person")],
                       [(("Menú de grupo (desde 8 personas)", "Group menu (from 8 people)"), desde(18)),
                        (("Menú infantil", "Children's menu"), eur(9)),
                        (("Comida por encargo para llevar", "Take-away meal to order"), desde(9))],
                       nota=NOTA_PRECIOS, tono="base", anchor="grupos-precios"),
        bloque_valores(("Comidas por encargo", "Meals to order"), [
            (("Para llevar a casa", "To take home"),
             ("Guisos y platos de cuchara en tartera, para comer en casa o en el trabajo.",
              "Stews and spoon dishes in a container, to eat at home or at work.")),
            (("Para una celebración", "For a celebration"),
             ("Raciones para compartir, platos principales y postres para tu reunión.",
              "Sharing plates, main courses and desserts for your get-together.")),
        ], cols=2, tono="relleno", estilo="borde"),
        sec_form(("Pide presupuesto de grupo o encargo", "Ask for a group or order quote"),
                 ("Cuéntanos qué necesitas y te respondemos con menú y precio, sin compromiso.",
                  "Tell us what you need and we will reply with a menu and price, with no commitment."),
                 campos_cita(servicios=[("Reserva de grupo", "Group booking"), ("Comida por encargo", "Meal to order"), ("Otra consulta", "Other question")],
                             extra=[campo("personas", "text", ("Número de personas", "Number of people"), True, maxlen=10)]),
                 ("Pedir presupuesto", "Request a quote"),
                 ("Gracias. Te respondemos con el menú y el precio lo antes posible.", "Thank you. We will reply with the menu and price as soon as possible."),
                 "pedir-presupuesto"),
    ])

    cont = pagina("contacto", ("contacto", "contact"), ("Contacto y reservas", "Contact and bookings"),
                  ("Reserva mesa por teléfono, WhatsApp o con el formulario, y mira el horario y cómo llegar.",
                   "Book a table by phone, WhatsApp or with the form, and see opening hours and how to get here."), [
        hero(("Reserva tu mesa", "Book your table"),
             ("Llámanos, escríbenos por WhatsApp o rellena el formulario. Te confirmamos la reserva el mismo día.",
              "Call us, message us on WhatsApp or fill in the form. We confirm your booking the same day."),
             [llamar(), whatsapp()], variante="texto"),
        bloque_contacto(("Dónde estamos", "Where we are"),
                        ("Comemos desde las 13:00. Si vienes en coche, pregúntanos dónde aparcar.",
                         "Lunch starts at 1:00 pm. If you come by car, ask us where to park."),
                        campos_cita(servicios=[("Mesa para comer", "Table for lunch"), ("Grupo", "Group"), ("Comida por encargo", "Meal to order")],
                                    extra=[campo("personas", "text", ("Número de personas", "Number of people"), False, maxlen=10)]),
                        ("Reservar mesa", "Book a table"), horas,
                        notas=("Si tienes alguna alergia o intolerancia, escríbela en el mensaje.",
                               "If you have an allergy or an intolerance, write it in the message."),
                        exito=("Gracias. Te confirmamos la reserva por teléfono o WhatsApp en cuanto la tengamos.",
                               "Thank you. We will confirm your booking by phone or WhatsApp as soon as we have it.")),
        bloque_mapa(tono="relleno"),
    ])

    return plantilla(KEY, ("Menú del día y cocina casera", "Set lunch and home cooking"),
                     ("Menú del día con precio y qué incluye, carta corta, alérgenos, grupos y encargos, con botones de llamar y WhatsApp.",
                      "Set lunch menu with price and what is included, short menu, allergens, groups and orders, with call and WhatsApp buttons."),
                     "comida", ("menu del dia restaurante comida casera cocina casera casa de comidas comedor menu diario bar de menus meson tasca reservas grupos",
                                "set lunch lunch menu restaurant home cooking daily menu canteen group bookings takeaway"),
                     look, cabecera(nav, cta_cab),
                     pie(nav, ("Menú del día y cocina casera. Reservas de grupos y comidas por encargo.",
                               "Set lunch and home cooking. Group bookings and meals to order."),
                         ("Lunes a viernes 8:00 a 17:00. Sábado 13:00 a 17:00.", "Monday to Friday 8 am to 5 pm. Saturday 1 pm to 5 pm.")),
                     [inicio, menu_p, grupos, cont])
