# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
from lib import *

KEY = "hamburgueseria"

# Paleta propia: pan de hamburguesa (fondo naranja tostado claro) con rojo kétchup. Pasa AA;
# primary/inverse no llega a 3, así que no se usa `tono="oscuro"`.
KETCHUP = dict(
    colores=dict(background="#f8dbc4", surface="#f1c9a9", text="#2a100b", muted="#64352a", primary="#9d1c12",
                 on_primary="#ffffff", border="#e0b48f", inverse="#2a100b", on_inverse="#fbe9d9"),
    fuentes=("montserrat", "dm-sans"), peso=800, h1=(50, 40, 32), radio=(4, 8, 12))

NOTA_PRECIOS = ("Precios de ejemplo: cámbialos por los tuyos. Incluyen IVA.",
                "Example prices: replace them with your own. VAT included.")
CAB = [("Producto", "Item"), ("Precio", "Price")]


def plato(nombre, detalle, precio):
    """Fila de carta: nombre con su descripción corta en la misma celda."""
    return ((f"{nombre[0]}. {detalle[0]}", f"{nombre[1]}. {detalle[1]}"), precio)


def sec_form(titulo, intro, campos, envio, exito, anchor, tono="base"):
    return seccion("contact", [titular(titulo, intro),
                               tarjeta([formulario(campos, envio, exito)], estilo="relleno" if tono == "base" else "borde",
                                       pad=28, tag="div", sz=tam(max_width=640))],
                   tono=tono, anchor=anchor, gap=28)


SI, NO = ("Sí", "Yes"), ("No", "No")


def build():
    nueva(KEY)
    look = KETCHUP
    nav = [("inicio", ("Inicio", "Home")), ("carta", ("Carta", "Menu")),
           ("pedidos", ("Para llevar y a domicilio", "Takeaway and delivery")), ("contacto", ("Contacto", "Contact"))]
    horas = [(("Lunes", "Monday"), ("Cerrado", "Closed")),
             (("Martes a jueves", "Tuesday to Thursday"), ("13:00 a 16:00 y de 20:00 a 23:30", "1:00 pm to 4:00 pm and 8:00 pm to 11:30 pm")),
             (("Viernes y sábado", "Friday and Saturday"), ("13:00 a 16:30 y de 20:00 a 00:30", "1:00 pm to 4:30 pm and 8:00 pm to 12:30 am")),
             (("Domingo", "Sunday"), ("13:00 a 16:30", "1:00 pm to 4:30 pm"))]
    cta_cab = marco([llamar(), whatsapp("ghost")], dir="row", gap=8, wrap=True, align="center")
    a_carta = a_pagina("carta")
    a_pedir = a_pagina("pedidos", "pedido")

    inicio = pagina("inicio", ("", ""), ("Hamburguesería y bocadillos", "Burgers and sandwiches"),
                    ("Hamburguesería: carta de hamburguesas y bocadillos, opciones vegetarianas, pedidos para llevar y a domicilio, horario y botones de llamar y WhatsApp.",
                     "Burger place: menu of burgers and sandwiches, vegetarian options, takeaway and delivery orders, opening hours and call and WhatsApp buttons."), [
        hero(("Hamburguesas hechas al momento", "Burgers made to order"),
             ("Pan, carne a la plancha y salsas de la casa. Para comer en el local, para llevar o a domicilio, con las alergias claras desde el principio.",
              "Bun, griddled patty and house sauces. To eat in, take away or have delivered, with allergens clear from the start."),
             [boton(("Ver la carta", "See the menu"), a_carta), boton(("Pedir ahora", "Order now"), a_pedir, "secondary"),
              whatsapp("ghost")],
             eyebrow=("Hamburguesas y bocadillos", "Burgers and sandwiches"),
             imagen_=("product", ("Hamburguesa con queso y patatas fritas en una bandeja", "Cheeseburger with fries on a tray"))),
        bloque_valores(("Así hacemos una hamburguesa", "How we make a burger"), [
            (("Se hace al pedirla", "Made when you order it"),
             ("La carne va a la plancha cuando llega tu comanda, no antes. Por eso tarda unos minutos y llega caliente.",
              "The patty goes on the griddle when your order arrives, not before. That is why it takes a few minutes and arrives hot.")),
            (("Pan y salsas de la casa", "House bread and sauces"),
             ("El pan se tuesta en la plancha y las salsas se preparan en la cocina. Si quieres una sin salsa, se quita.",
              "The bun is toasted on the griddle and the sauces are prepared in the kitchen. If you want one without sauce, we leave it out.")),
            (("Tú eliges el punto", "You choose how it is cooked"),
             ("Te preguntamos cómo la quieres y te lo explicamos con calma. Abajo tienes lo que significa cada punto.",
              "We ask how you want it and explain it calmly. Below you have what each doneness means.")),
        ], estilo="relleno", eyebrow=("La cocina", "The kitchen")),
        bloque_valores(("El punto de la carne, en llano", "Doneness explained plainly"), [
            (("Poco hecha", "Rare"),
             ("Rosada y jugosa por dentro. No la recomendamos en carne picada: te la ofrecemos solo si lo pides y lo hablamos.",
              "Pink and juicy inside. We do not recommend it for minced meat: we only offer it if you ask and we talk it through.")),
            (("Al punto", "Medium"),
             ("Ligeramente rosada en el centro, jugosa. Es como la servimos si no nos dices nada.",
              "Slightly pink in the centre, juicy. This is how we serve it if you do not tell us otherwise.")),
            (("Muy hecha", "Well done"),
             ("Sin rosa por dentro y más firme. Es la mejor opción para niños, embarazadas y personas con defensas bajas.",
              "No pink inside and firmer. It is the best choice for children, pregnant women and people with a weak immune system.")),
        ], estilo="borde", anchor="punto"),
        bloque_checks(("Si no comes carne", "If you do not eat meat"), [
            ("Hamburguesa vegetal de legumbres, con su pan y sus salsas.", "Pulse-based veggie burger, with its bun and sauces."),
            ("Bocadillo vegetal con verduras a la plancha.", "Veggie sandwich with griddled vegetables."),
            ("Ensalada y entrantes sin carne: patatas, aros de cebolla y más.", "Salad and meat-free starters: fries, onion rings and more."),
            ("Dinos si eres vegetariano o vegano y te decimos qué se puede quitar o cambiar.",
             "Tell us if you are vegetarian or vegan and we tell you what can be removed or swapped."),
        ], eyebrow=("Opciones vegetarianas", "Vegetarian options"), tono="relleno", cols=2, anchor="vegetariano"),
        bloque_precios(("Menús", "Set meals"), ("Menús de la hamburguesería", "Set meals"),
                       [("Menú", "Set meal"), ("Precio", "Price")],
                       [(("Menú hamburguesa: hamburguesa, patatas y bebida", "Burger set: burger, fries and drink"), eur(12.5)),
                        (("Menú bocadillo: bocadillo, patatas y bebida", "Sandwich set: sandwich, fries and drink"), eur(9.9)),
                        (("Menú infantil: mini hamburguesa, patatas y bebida", "Kids' set: mini burger, fries and drink"), eur(7.5))],
                       nota=NOTA_PRECIOS, boton_=boton(("Ver toda la carta", "See the full menu"), a_carta, "secondary"),
                       tono="base", anchor="menus"),
        seccion("text", [titular(("Horario", "Opening hours"),
                                 ("Servimos en el local, para llevar y a domicilio durante todo el horario.",
                                  "We serve in the restaurant, for takeaway and for delivery throughout opening hours.")),
                         horario(horas)], tono="relleno", anchor="horario", gap=28),
        bloque_opiniones(("Lo que dicen quienes han venido", "What our customers say"),
                         [("las hamburguesas", "the burgers"), ("el servicio", "the service"), ("el pedido a domicilio", "the delivery")],
                         tono="base"),
        bloque_cta(("¿Pedimos?", "Shall we get your order started?"),
                   ("Llámanos o escríbenos por WhatsApp con lo que quieres y cuándo lo quieres.",
                    "Call or message us on WhatsApp with what you want and when you want it."), tono="relleno"),
    ])

    carta = pagina("carta", ("carta", "menu"), ("Carta", "Menu"),
                   ("Carta por secciones con precio de ejemplo: hamburguesas, bocadillos, entrantes, postres y bebidas, y tabla de alérgenos.",
                    "Menu by section with example prices: burgers, sandwiches, starters, desserts and drinks, and an allergen table."), [
        hero(("La carta", "The menu"),
             ("Hamburguesas y bocadillos con su precio. Si tienes alergia o intolerancia, mira la tabla de alérgenos de más abajo o pregúntanos.",
              "Burgers and sandwiches with their price. If you have an allergy or intolerance, see the allergen table below or ask us."),
             [boton(("Pedir ahora", "Order now"), a_pedir), llamar("secondary"), whatsapp("ghost")], variante="texto"),
        bloque_carta(("Hamburguesas y bocadillos", "Burgers and sandwiches"), [
            (("Hamburguesas", "Burgers"), ("Precios de las hamburguesas", "Burger prices"), CAB, [
                plato(("Clásica", "Classic"), ("Carne, queso, lechuga, tomate y salsa de la casa.", "Patty, cheese, lettuce, tomato and house sauce."), eur(9.5)),
                plato(("Doble", "Double"), ("Dos carnes, doble de queso y cebolla.", "Two patties, double cheese and onion."), eur(11.9)),
                plato(("Pollo crujiente", "Crispy chicken"), ("Pollo empanado, lechuga y mayonesa.", "Breaded chicken, lettuce and mayonnaise."), eur(9.9)),
                plato(("Picante", "Spicy"), ("Carne, queso, jalapeños y salsa picante.", "Patty, cheese, jalapeños and hot sauce."), eur(10.5)),
                plato(("Vegetal", "Veggie"), ("Hamburguesa de legumbres, verduras y salsa de yogur.", "Pulse burger, vegetables and yoghurt sauce."), eur(9.5)),
                plato(("Mini (para niños)", "Mini (for kids)"), ("Carne pequeña con queso, sin salsa.", "Small patty with cheese, no sauce."), eur(5.5))]),
            (("Bocadillos", "Sandwiches"), ("Precios de los bocadillos", "Sandwich prices"), CAB, [
                plato(("Lomo con queso", "Pork loin with cheese"), ("A la plancha, en pan crujiente.", "Griddled, in crusty bread."), eur(6.5)),
                plato(("Pollo", "Chicken"), ("Pechuga a la plancha, lechuga y tomate.", "Griddled chicken breast, lettuce and tomato."), eur(6.5)),
                plato(("Calamares", "Squid"), ("Rebozados, con mayonesa de limón.", "Battered, with lemon mayonnaise."), eur(7)),
                plato(("Vegetal", "Veggie"), ("Verduras a la plancha, queso y salsa.", "Griddled vegetables, cheese and sauce."), eur(6))]),
            (("Entrantes", "Starters"), ("Precios de los entrantes", "Starter prices"), CAB, [
                (("Patatas fritas", "Fries"), eur(3.5)), (("Aros de cebolla", "Onion rings"), eur(4.5)),
                (("Nuggets de pollo (8)", "Chicken nuggets (8)"), eur(5)), (("Ensalada de la casa", "House salad"), eur(6))]),
            (("Postres", "Desserts"), ("Precios de los postres", "Dessert prices"), CAB, [
                (("Brownie de chocolate", "Chocolate brownie"), eur(4.5)), (("Tarta del día", "Cake of the day"), eur(4.5)),
                (("Helado", "Ice cream"), eur(3.5))]),
            (("Bebidas", "Drinks"), ("Precios de las bebidas", "Drink prices"), CAB, [
                (("Agua", "Water"), eur(1.6)), (("Refresco", "Soft drink"), eur(2.2)), (("Cerveza", "Beer"), eur(2.5)),
                (("Batido", "Milkshake"), eur(3.8))]),
        ], nota=NOTA_PRECIOS, tono="base", anchor="platos"),
        bloque_precios(("Extras", "Extras"), ("Precios de los extras", "Extras prices"), CAB,
                       [(("Más queso", "Extra cheese"), eur(0.8)), (("Beicon", "Bacon"), eur(1.2)),
                        (("Huevo a la plancha", "Fried egg"), eur(1)), (("Cambiar patatas por ensalada", "Swap fries for salad"), eur(1))],
                       nota=NOTA_PRECIOS, tono="relleno", anchor="extras"),
        seccion("pricing", [
            titular(("Alérgenos de la carta", "Menu allergens"),
                    ("Mira qué lleva cada hamburguesa antes de pedir. En la cocina se manejan gluten, huevo, leche, sésamo y mostaza, así que cualquier plato puede llevar trazas.",
                     "See what each burger contains before you order. The kitchen handles gluten, egg, milk, sesame and mustard, so any dish may contain traces.")),
            tabla(("Alérgenos por hamburguesa", "Allergens by burger"),
                  [("Hamburguesa", "Burger"), ("Gluten", "Gluten"), ("Lácteos", "Dairy"), ("Huevo", "Egg"), ("Sésamo", "Sesame"), ("Mostaza", "Mustard")],
                  [[("Clásica", "Classic"), SI, SI, SI, SI, SI],
                   [("Doble", "Double"), SI, SI, SI, SI, SI],
                   [("Pollo crujiente", "Crispy chicken"), SI, NO, SI, SI, NO],
                   [("Picante", "Spicy"), SI, SI, NO, SI, NO],
                   [("Vegetal", "Veggie"), SI, SI, NO, SI, NO]]),
            peq(("Tabla de ejemplo: cámbiala por la de tus recetas. Si tienes una alergia o intolerancia, pregúntanos antes de pedir: te lo decimos por escrito.",
                 "Example table: replace it with the one for your recipes. If you have an allergy or intolerance, ask us before you order: we tell you in writing."),
                color="$muted"),
        ], tono="base", anchor="alergenos", gap=28),
        bloque_cta(("¿Lo pedimos?", "Shall we get your order started?"), None, tono="relleno"),
    ])

    pedidos = pagina("pedidos", ("pedidos", "orders"), ("Para llevar y a domicilio", "Takeaway and delivery"),
                     ("Cómo pedir para llevar o a domicilio: pasos, zonas y gastos de envío, formulario de pedido, horario de reparto y preguntas frecuentes.",
                      "How to order takeaway or delivery: steps, delivery areas and charges, an order form, delivery hours and frequently asked questions."), [
        hero(("Para llevar y a domicilio", "Takeaway and delivery"),
             ("Pide por teléfono, por WhatsApp o con el formulario. Te decimos cuánto tardamos y, si es a domicilio, lo que cuesta el envío.",
              "Order by phone, on WhatsApp or with the form. We tell you how long it takes and, for delivery, what it costs."),
             [llamar(), whatsapp(), boton(("Hacer el pedido", "Place an order"), a_pagina("pedidos", "pedido"), "ghost")],
             variante="texto", tono="relleno"),
        bloque_pasos(("Cómo pedir", "How to order"), [
            (("Elige de la carta", "Choose from the menu"), ("Mira la carta y apunta lo que quieres, con el punto de la carne y las salsas.",
                                                              "Look at the menu and note what you want, with how you want the meat cooked and the sauces.")),
            (("Dinos cómo lo quieres", "Tell us how you want it"), ("Llama, escribe por WhatsApp o usa el formulario. Indica si es para recoger o a domicilio.",
                                                                    "Call, message on WhatsApp or use the form. Say whether it is for collection or delivery.")),
            (("Te confirmamos", "We confirm"), ("Te damos el precio total y el tiempo de espera antes de empezar.", "We give you the total price and the waiting time before we start.")),
            (("Recíbelo o recógelo", "Receive or collect it"), ("En tu puerta o en el local. El pago, en efectivo o con tarjeta.", "At your door or in the restaurant. Pay by cash or card.")),
        ]),
        bloque_precios(("Zonas y gastos de envío", "Delivery areas and charges"), ("Zonas de reparto y gastos de envío", "Delivery areas and charges"),
                       [("Zona", "Area"), ("Pedido mínimo", "Minimum order"), ("Envío", "Delivery")],
                       [(("Barrio del local", "Neighbourhood of the restaurant"), eur(12), ("Gratis", "Free")),
                        (("Barrios cercanos", "Nearby neighbourhoods"), eur(15), eur(2)),
                        (("Resto de la ciudad", "Rest of the city"), eur(20), eur(3.5))],
                       nota=("Zonas, mínimos y precios de ejemplo: cámbialos por los tuyos. Si tu zona no aparece, pregúntanos.",
                             "Example areas, minimums and prices: replace them with your own. If your area is not listed, ask us."),
                       tono="base", anchor="envio"),
        sec_form(("Haz tu pedido", "Place your order"),
                 ("Escribe lo que quieres y cuándo lo quieres. Te llamamos o te escribimos para confirmarlo.",
                  "Write what you want and when you want it. We call or message you to confirm."),
                 [campo("nombre", "text", ("Nombre", "Name"), True, auto="name", maxlen=120),
                  campo("telefono", "tel", ("Teléfono", "Phone"), True, auto="tel", maxlen=30),
                  campo("tipo", "select", ("¿Para llevar o a domicilio?", "Takeaway or delivery?"), True,
                        opciones=[("Para recoger en el local", "Collection from the restaurant"), ("A domicilio", "Home delivery")]),
                  campo("direccion", "text", ("Dirección (solo a domicilio)", "Address (delivery only)"), False, auto="street-address", maxlen=200),
                  campo("hora", "text", ("Hora a la que lo quieres", "Time you want it"), False, maxlen=40),
                  campo("pedido", "textarea", ("Tu pedido, con el punto de la carne y las salsas", "Your order, with how you want the meat cooked and the sauces"), True, maxlen=1000),
                  campo("alergias", "text", ("Alergias o intolerancias", "Allergies or intolerances"), False, maxlen=200)],
                 ("Enviar pedido", "Send order"),
                 ("Gracias. Te confirmamos el pedido, el precio y el tiempo de espera por teléfono o WhatsApp.",
                  "Thank you. We confirm your order, the price and the waiting time by phone or WhatsApp."),
                 "pedido", tono="relleno"),
        bloque_faq(("Preguntas frecuentes", "Frequently asked questions"), [
            (("¿Cuánto tardan en llegar?", "How long does it take to arrive?"),
             ("Para llevar, unos 15 o 20 minutos. A domicilio, entre 30 y 45, según la hora y la zona. Te damos el tiempo exacto al confirmar.",
              "For takeaway, about 15 to 20 minutes. For delivery, between 30 and 45, depending on the time and the area. We give you the exact time when we confirm.")),
            (("¿Llegan calientes?", "Do they arrive hot?"),
             ("Van en caja cerrada y las patatas, aparte del resto, para que no se reblandezcan. Aun así, cuanto antes las comas, mejor.",
              "They come in a closed box with the fries separate from the rest so they do not go soft. Even so, the sooner you eat them, the better.")),
            (("¿Qué pasa si hay un error en mi pedido?", "What if there is a mistake in my order?"),
             ("Llámanos y lo arreglamos: lo repetimos o te devolvemos lo que falte.",
              "Call us and we sort it out: we remake it or refund what is missing.")),
            (("¿Cómo se paga?", "How do I pay?"),
             ("En efectivo o con tarjeta al recibirlo o al recogerlo. Te lo confirmamos al hacer el pedido.",
              "By cash or card when you receive or collect it. We confirm it when you order.")),
        ], tono="base"),
    ])

    cont = pagina("contacto", ("contacto", "contact"), ("Contacto y horario", "Contact and opening hours"),
                  ("Llama o escribe por WhatsApp para pedir o preguntar, y mira el horario, el formulario y cómo llegar al local.",
                   "Call or message on WhatsApp to order or ask a question, and see opening hours, the form and how to get to the restaurant."), [
        hero(("Ven al local o llámanos", "Come in or call us"),
             ("Para pedir, para preguntar por alérgenos o para reservar una mesa de grupo.",
              "To order, to ask about allergens or to book a table for a group."),
             [llamar(), whatsapp()], variante="texto"),
        bloque_contacto(("Cómo encontrarnos", "How to find us"),
                        ("Estamos a pie de calle. Si vais más de seis, avisadnos y juntamos mesas.",
                         "We are at street level. If there are more than six of you, let us know and we put tables together."),
                        campos_consulta(servicios=[("Pedido", "Order"), ("Mesa para un grupo", "Table for a group"),
                                                   ("Alergias e intolerancias", "Allergies and intolerances"), ("Otra consulta", "Other question")]),
                        ("Enviar mensaje", "Send message"), horas,
                        exito=("Gracias. Te respondemos lo antes posible.", "Thank you. We will reply as soon as possible.")),
        bloque_mapa(tono="relleno"),
    ])

    return plantilla(KEY, ("Hamburguesería y bocadillos", "Burgers and sandwiches"),
                     ("Carta de hamburguesas y bocadillos con precios, punto de la carne, opciones vegetarianas, alérgenos en tabla y pedidos para llevar y a domicilio.",
                      "Menu of burgers and sandwiches with prices, meat doneness, vegetarian options, an allergen table and takeaway and delivery orders."),
                     "comida", ("hamburgueseria hamburguesas hamburguesa burger bocadillos bocateria bocata fast food comida rapida para llevar a domicilio menu",
                                "burger burgers burger place hamburger sandwiches sandwich bar takeaway delivery fast food"),
                     look, cabecera(nav, cta_cab),
                     pie(nav, ("Hamburguesas y bocadillos hechos al momento, para comer aquí, para llevar y a domicilio.",
                               "Burgers and sandwiches made to order, to eat in, take away or have delivered."),
                         ("Martes a jueves 13:00 a 16:00 y 20:00 a 23:30. Viernes y sábado hasta las 00:30.",
                          "Tuesday to Thursday 1 pm to 4 pm and 8 pm to 11:30 pm. Friday and Saturday until 12:30 am.")),
                     [inicio, carta, pedidos, cont])
