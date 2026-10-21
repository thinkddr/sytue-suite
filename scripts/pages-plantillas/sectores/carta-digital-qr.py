# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
import lib
from lib import *

KEY = "carta-digital-qr"

# Paleta propia: crema de carta y rojo granate. Sin fondo $inverse (no se usa).
LOOK = dict(
    colores=dict(background="#fdf6e3", surface="#f5e8c8", text="#2b1710", muted="#68503c", primary="#7a1d1d",
                 on_primary="#ffffff", border="#e6d5ad", inverse="#2b1710", on_inverse="#fbf1dc"),
    fuentes=("playfair-display", "figtree"), peso=700, h1=(44, 38, 32), radio=(4, 8, 12), ancho=720)

NOTA = ("Precios de ejemplo: cámbialos por los tuyos. IVA incluido.",
        "Example prices: replace them with your own. VAT included.")


def plato(nombre, desc, precio, alergenos):
    """Un plato: nombre y precio en una línea, descripción corta y alérgenos debajo."""
    return marco([
        marco([h3(nombre), texto("p", precio, weight=700, color="$primary")],
              dir="row", gap=16, justify="space-between", align="start"),
        p(desc, color="$muted"),
        peq((f"Alérgenos: {alergenos[0]}", f"Allergens: {alergenos[1]}"), color="$muted"),
    ], tag="li", gap=4, pad=[14, 0, 14, 0], style={"border": {"width": 1, "color": "$border", "style": "solid"}, "radius": 0})


def grupo(clave, titulo, platos, tono="base"):
    return seccion("pricing", [h2(titulo), marco(platos, gap=0, tag="ul")], anchor=clave, tono=tono, pad_v=36, gap=12)


def build():
    nueva(KEY)
    malos = lib.comprobar_paleta("carta", LOOK["colores"])
    assert not malos, malos

    anclas = [("entrantes", ("Entrantes", "Starters")), ("principales", ("Principales", "Mains")),
              ("postres", ("Postres", "Desserts")), ("bebidas", ("Bebidas", "Drinks")),
              ("alergenos", ("Alérgenos", "Allergens"))]
    cab = cabecera([("inicio", ("Inicio", "Home"))], llamar("primary", ("Llamar", "Call")))
    # Navegación por la carta, siempre visible (sin menú desplegable) y en la cabecera.
    menu_ = cab["children"][1]["children"][0]
    menu_["items"] = [{"id": lib._id(), "label": L(t), "link": a_pagina("inicio", a)} for a, t in anclas]
    menu_["collapse_below"] = "never"

    horas = [(("Lunes a jueves", "Monday to Thursday"), ("12:30 a 16:00 y 20:00 a 23:30", "12:30 pm to 4:00 pm and 8:00 pm to 11:30 pm")),
             (("Viernes y sábado", "Friday and Saturday"), ("12:30 a 16:30 y 20:00 a 00:30", "12:30 pm to 4:30 pm and 8:00 pm to 12:30 am")),
             (("Domingo", "Sunday"), ("12:30 a 17:00", "12:30 pm to 5:00 pm"))]

    portada = hero(("Nuestra carta", "Our menu"),
                   ("Mira la carta con calma y díselo a quien te atienda. Si tienes alguna alergia, avísanos antes de pedir.",
                    "Take your time with the menu and tell whoever is serving you. If you have any allergy, let us know before ordering."),
                   [llamar("primary", ("Llamar para reservar", "Call to book")), whatsapp("secondary")],
                   eyebrow=("Carta digital", "Digital menu"), variante="texto",
                   extra=peq(NOTA, color="$muted"))
    portada["layout"]["padding"] = [36, 32, 20, 32]
    portada["breakpoints"]["mobile"]["layout"]["padding"] = [28, 20, 16, 20]

    inicio = pagina("inicio", ("", ""), ("Carta digital", "Digital menu"),
                    ("Carta digital de un restaurante o bar para ver desde el móvil: entrantes, principales, postres, bebidas, tabla de alérgenos, horario, reserva y botón de llamar.",
                     "Digital menu for a restaurant or bar to view on a phone: starters, mains, desserts, drinks, allergen table, opening hours, booking and call button."), [
        portada,
        grupo("entrantes", ("Entrantes", "Starters"), [
            plato(("Ensalada de la casa", "House salad"),
                  ("Lechuga, tomate, cebolla y atún.", "Lettuce, tomato, onion and tuna."), eur(8.5),
                  ("pescado", "fish")),
            plato(("Croquetas caseras", "Homemade croquettes"),
                  ("Seis croquetas de jamón.", "Six ham croquettes."), eur(9),
                  ("gluten, leche, huevos", "gluten, milk, eggs")),
            plato(("Patatas bravas", "Spicy potatoes"),
                  ("Con salsa brava y alioli.", "With spicy sauce and aioli."), eur(6.5),
                  ("huevos, mostaza", "eggs, mustard")),
            plato(("Tortilla de patatas", "Spanish omelette"),
                  ("Poco cuajada, con cebolla.", "Soft-centred, with onion."), eur(7.5),
                  ("huevos", "eggs")),
        ]),
        grupo("principales", ("Principales", "Mains"), [
            plato(("Arroz del día", "Rice of the day"),
                  ("Por persona, mínimo dos raciones.", "Per person, minimum two servings."), eur(14),
                  ("pescado, crustáceos, sulfitos", "fish, crustaceans, sulphites")),
            plato(("Merluza a la plancha", "Grilled hake"),
                  ("Con verduras salteadas.", "With sautéed vegetables."), eur(16.5),
                  ("pescado", "fish")),
            plato(("Hamburguesa de ternera", "Beef burger"),
                  ("Con queso y patatas fritas.", "With cheese and chips."), eur(12.5),
                  ("gluten, leche, sésamo, mostaza", "gluten, milk, sesame, mustard")),
            plato(("Pasta vegetariana", "Vegetarian pasta"),
                  ("Con verduras de temporada.", "With seasonal vegetables."), eur(11),
                  ("gluten, apio", "gluten, celery")),
        ], tono="relleno"),
        grupo("postres", ("Postres", "Desserts"), [
            plato(("Tarta de queso", "Cheesecake"), ("Hecha en casa.", "Homemade."), eur(5.5),
                  ("gluten, leche, huevos", "gluten, milk, eggs")),
            plato(("Flan casero", "Homemade crème caramel"), ("Con nata.", "With cream."), eur(4.5),
                  ("leche, huevos", "milk, eggs")),
            plato(("Fruta de temporada", "Seasonal fruit"), ("La que haya hoy.", "Whatever is in season today."), eur(4),
                  ("ninguno", "none")),
        ]),
        seccion("pricing", [
            h2(("Bebidas", "Drinks")),
            tabla(("Bebidas y sus precios (de ejemplo)", "Drinks and their prices (example)"),
                  [("Bebida", "Drink"), ("Precio", "Price")],
                  [(("Agua", "Water"), eur(2)), (("Refresco", "Soft drink"), eur(2.5)),
                   (("Caña de cerveza", "Small beer"), eur(2.2)), (("Copa de vino de la casa", "Glass of house wine"), eur(3)),
                   (("Café", "Coffee"), eur(1.6))]),
            peq(("Los vinos y las cervezas contienen sulfitos y gluten. Pregunta por las opciones sin alcohol.",
                 "Wine and beer contain sulphites and gluten. Ask about the alcohol-free options."), color="$muted"),
        ], anchor="bebidas", tono="relleno", pad_v=36, gap=12),
        seccion("features", [
            h2(("Alérgenos", "Allergens")),
            p(("Estos son los 14 alérgenos que hay que declarar y los platos de esta carta donde pueden estar.",
               "These are the 14 allergens that must be declared and the dishes on this menu where they may be found."),
              color="$muted"),
            tabla(("Alérgenos y dónde puede haberlos (de ejemplo)", "Allergens and where they may be found (example)"),
                  [("Alérgeno", "Allergen"), ("Platos y bebidas", "Dishes and drinks")],
                  [(("Gluten", "Gluten"), ("Croquetas, hamburguesa, pasta, tarta de queso, cerveza", "Croquettes, burger, pasta, cheesecake, beer")),
                   (("Crustáceos", "Crustaceans"), ("Arroz del día", "Rice of the day")),
                   (("Huevos", "Eggs"), ("Croquetas, bravas, tortilla, tarta de queso, flan", "Croquettes, bravas, omelette, cheesecake, crème caramel")),
                   (("Pescado", "Fish"), ("Ensalada de la casa, arroz del día, merluza", "House salad, rice of the day, hake")),
                   (("Cacahuetes", "Peanuts"), ("Ninguno", "None")),
                   (("Soja", "Soy"), ("Ninguno", "None")),
                   (("Leche", "Milk"), ("Croquetas, hamburguesa, tarta de queso, flan", "Croquettes, burger, cheesecake, crème caramel")),
                   (("Frutos de cáscara", "Tree nuts"), ("Ninguno", "None")),
                   (("Apio", "Celery"), ("Pasta vegetariana", "Vegetarian pasta")),
                   (("Mostaza", "Mustard"), ("Patatas bravas, hamburguesa", "Bravas, burger")),
                   (("Sésamo", "Sesame"), ("Pan de la hamburguesa", "Burger bun")),
                   (("Sulfitos", "Sulphites"), ("Arroz del día, vino", "Rice of the day, wine")),
                   (("Altramuces", "Lupin"), ("Ninguno", "None")),
                   (("Moluscos", "Molluscs"), ("Ninguno", "None"))]),
            peq(("Tabla de ejemplo: ajústala a tus platos y a tus recetas. Cuéntanos cualquier alergia o intolerancia antes de pedir; en la cocina se manejan todos estos alimentos.",
                 "Example table: adjust it to your dishes and recipes. Tell us about any allergy or intolerance before ordering; all of these foods are handled in the kitchen."),
                color="$muted"),
        ], anchor="alergenos", tono="base", pad_v=36, gap=12),
        seccion("contact", [
            titular(("Reserva y contacto", "Booking and contact"),
                    ("Para reservar mesa, llama, escribe por WhatsApp o deja tus datos y te confirmamos.",
                     "To book a table, call, message on WhatsApp or leave your details and we will confirm.")),
            marco([icono("map-pin"), p(CALLE)], dir="row", gap=12, align="start"),
            marco([icono("phone"), p((TEL_VISIBLE, TEL_VISIBLE))], dir="row", gap=12, align="start"),
            marco([icono("mail"), p((CORREO, CORREO))], dir="row", gap=12, align="start"),
            marco([llamar(), whatsapp()], dir="row", gap=12, wrap=True),
            horario(horas),
            tarjeta([formulario(campos_cita(fecha=True, mensaje=False, extra=[
                campo("personas", "select", ("Personas", "People"), True,
                      opciones=[("1", "1"), ("2", "2"), ("3", "3"), ("4", "4"), ("5", "5"), ("6", "6"),
                                ("7 o más", "7 or more")]),
                campo("hora", "time", ("Hora que prefieres", "Preferred time"))]),
                ("Pedir mesa", "Request a table"),
                ("Gracias. Te confirmamos la mesa por teléfono o WhatsApp.",
                 "Thank you. We will confirm your table by phone or WhatsApp."))],
                estilo="relleno", pad=24, tag="div", sz={"width": "fill", "height": "hug", "min_width": 0}),
        ], anchor="contacto", tono="relleno", pad_v=36, gap=18),
        bloque_mapa(("Cómo llegar", "How to get here"),
                    ("Estamos en la dirección de arriba.", "We are at the address above."), tono="base"),
    ])

    return plantilla(KEY, ("Carta digital con QR", "Digital menu with QR code"),
                     ("Carta de restaurante o bar pensada para el móvil, para abrir al escanear un QR: entrantes, principales, postres y bebidas con precios, tabla de alérgenos, horario, reserva y botón de llamar.",
                      "Restaurant or bar menu made for phones, opened by scanning a QR code: starters, mains, desserts and drinks with prices, allergen table, opening hours, booking and call button."),
                     "web", ("carta digital qr carta qr menu digital escanear movil restaurante bar cafeteria platos precios alergenos carta online codigo qr",
                             "digital menu qr code qr menu scan phone restaurant bar cafe dishes prices allergens online menu"),
                     LOOK, cab,
                     pie(anclas, ("Carta digital. Los precios llevan IVA y los alérgenos están en su tabla.",
                                  "Digital menu. Prices include VAT and allergens are in their table."),
                         ("Lunes a jueves 12:30 a 16:00 y 20:00 a 23:30. Viernes y sábado hasta las 00:30. Domingo 12:30 a 17:00.",
                          "Monday to Thursday 12:30 pm to 4 pm and 8 pm to 11:30 pm. Friday and Saturday until 12:30 am. Sunday 12:30 pm to 5 pm.")),
                     [inicio])
