# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
from lib import *

KEY = "fruteria-tienda-barrio"

# Sandía: fondo verdoso muy claro, rojo de pulpa como color principal. Sin fondos oscuros ($inverse no se usa
# como sección). Pasa AA (lo comprueba lib.comprobar_paleta).
SANDIA = dict(
    colores=dict(background="#f8fbf3", surface="#e5f1d8", text="#17261a", muted="#4b5f4d", primary="#b4163f",
                 on_primary="#ffffff", border="#d2e3c0", inverse="#143a22", on_inverse="#eef8e6"),
    fuentes=("figtree", "work-sans"), radio=(10, 16, 24), ancho=1100)

NOTA_PRECIOS = ("Precios de ejemplo: cámbialos por los tuyos. Incluyen IVA.",
                "Example prices: replace them with your own. VAT included.")


def sec_form(titulo, intro, campos, envio, exito, anchor, tono="base"):
    return seccion("contact", [titular(titulo, intro),
                               tarjeta([formulario(campos, envio, exito)], estilo="relleno" if tono == "base" else "borde",
                                       pad=28, tag="div", sz=tam(max_width=680))],
                   tono=tono, anchor=anchor, gap=28)


def campos_pedido():
    return [campo("nombre", "text", ("Nombre", "Name"), True, auto="name", maxlen=120),
            campo("telefono", "tel", ("Teléfono", "Phone"), True, auto="tel", maxlen=30),
            campo("entrega", "select", ("¿Lo recoges o te lo llevamos?", "Collect it or have it delivered?"), True,
                  opciones=[("Lo recojo en la tienda", "I will collect it in the shop"), ("Quiero reparto a domicilio", "I want home delivery")]),
            campo("fecha", "date", ("Día que lo quieres", "Day you want it"), True),
            campo("direccion", "text", ("Dirección (solo si pides reparto)", "Address (only if you want delivery)"), False,
                  auto="street-address", maxlen=200),
            campo("pago", "select", ("Cómo pagarás", "How you will pay"), False,
                  opciones=[("Efectivo", "Cash"), ("Tarjeta", "Card"), ("Bizum", "Bizum")]),
            campo("lista", "textarea", ("Tu lista (una cosa por línea, con la cantidad)",
                                        "Your list (one item per line, with the amount)"), True, maxlen=1500)]


def build():
    nueva(KEY)
    look = SANDIA
    nav = [("inicio", ("Inicio", "Home")), ("temporada", ("Fruta y verdura", "Fruit and veg")),
           ("pedidos", ("Pedidos y reparto", "Orders and delivery")), ("contacto", ("Contacto", "Contact"))]
    horas = [(("Lunes a viernes", "Monday to Friday"), ("8:00 a 14:00 y de 17:00 a 20:30", "8:00 am to 2:00 pm and 5:00 pm to 8:30 pm")),
             (("Sábado", "Saturday"), ("8:00 a 14:30", "8:00 am to 2:30 pm")),
             (("Domingo", "Sunday"), ("Cerrado", "Closed"))]
    cta_cab = marco([llamar(), whatsapp("ghost")], dir="row", gap=8, wrap=True, align="center")
    a_pedido = a_pagina("pedidos", "hacer-pedido")

    inicio = pagina("inicio", ("", ""), ("Frutería y tienda de barrio", "Greengrocer's and neighbourhood shop"),
                    ("Frutería y tienda de barrio: fruta y verdura de temporada, productos por secciones, cestas, pedidos para recoger o repartir y botones de llamar y WhatsApp.",
                     "Greengrocer's and neighbourhood shop: seasonal fruit and veg, products by section, baskets, orders to collect or deliver and call and WhatsApp buttons."), [
        hero(("Fruta, verdura y lo que falta en la despensa", "Fruit, veg and whatever your pantry is missing"),
             ("Fruta y verdura de temporada, huevos, lácteos y productos de despensa en el barrio. Pasa por la tienda o manda tu lista y la preparamos.",
              "Seasonal fruit and veg, eggs, dairy and pantry goods in the neighbourhood. Drop by the shop or send your list and we will get it ready."),
             [boton(("Hacer un pedido", "Place an order"), a_pedido), llamar("secondary"), whatsapp("ghost")],
             eyebrow=("Frutería y tienda de barrio", "Greengrocer's and neighbourhood shop"),
             variante="centro", tono="relleno"),
        bloque_valores(("Por secciones", "By section"), [
            (("Fruta", "Fruit"), ("De temporada, para comer ya o para madurar en casa. Dinos cuándo la quieres y te elegimos el punto.",
                                  "In season, to eat now or to ripen at home. Tell us when you want it and we will choose the ripeness.")),
            (("Verdura y hortalizas", "Vegetables"), ("Para ensalada, para guiso o para la plancha, a granel y al peso.",
                                                      "For salads, stews or the griddle, loose and by weight.")),
            (("Huevos y lácteos", "Eggs and dairy"), ("Huevos, leche, yogures y quesos de uso diario.", "Eggs, milk, yoghurts and cheeses for everyday use.")),
            (("Despensa", "Pantry"), ("Legumbres, arroz, pasta, aceite, harinas y conservas.", "Pulses, rice, pasta, oil, flour and tinned goods.")),
            (("Bebidas", "Drinks"), ("Agua, zumos, refrescos y lo que pida el barrio.", "Water, juices, soft drinks and whatever the neighbourhood asks for.")),
            (("Hogar y limpieza", "Home and cleaning"), ("Lo básico para no tener que bajar otra vez al súper.", "The basics, so you do not have to go out to the supermarket again.")),
        ], intro=("Lo que hay depende de la semana. Si falta algo que sueles comprar, pídelo: lo traemos.",
                  "What we have depends on the week. If something you usually buy is missing, ask: we will bring it in."),
            tono="base", estilo="relleno", anchor="secciones"),
        bloque_texto(("Qué hay esta semana", "What is in this week"), [
            ("Aquí cuentas qué ha llegado: las fresas de la semana, la primera calabaza, las naranjas de tu zona. Es un texto de ejemplo: cámbialo cada semana por lo que tengas.",
             "Here you say what has arrived: this week's strawberries, the first pumpkin, oranges from your area. This is an example text: change it every week to what you have."),
            ("Para ver qué es de temporada en cada época del año, mira la página de fruta y verdura.",
             "To see what is in season at each time of year, see the fruit and veg page.")],
            eyebrow=("Esta semana", "This week"), tono="relleno", anchor="esta-semana",
            imagen_=("product", ("Cajas de fruta y verdura en el mostrador de la tienda", "Crates of fruit and vegetables at the shop counter"))),
        bloque_pasos(("Pide sin bajar a la tienda", "Order without coming to the shop"), [
            (("Manda tu lista", "Send your list"), ("Por WhatsApp o con el formulario: lo que necesitas y cuánto.", "By WhatsApp or with the form: what you need and how much.")),
            (("Te confirmamos", "We confirm"), ("Te decimos qué hay y el total aproximado.", "We tell you what we have and the approximate total.")),
            (("Recoges o te lo llevamos", "Collect or delivery"), ("Pasas a por él o te lo repartimos si estás en nuestra zona.", "You collect it, or we deliver it if you are in our area.")),
        ], tono="base", anchor="como-pedir"),
        bloque_opiniones(("Lo que dicen los vecinos", "What the neighbours say"),
                         [("la fruta", "the fruit"), ("el reparto", "the delivery"), ("el trato", "the service")], tono="relleno"),
        bloque_cta(("¿Qué te falta en casa?", "What are you missing at home?"),
                   ("Mándanos la lista y te la preparamos.", "Send us the list and we will get it ready."),
                   [boton(("Hacer un pedido", "Place an order"), a_pedido), llamar("secondary"), whatsapp("ghost")], tono="base"),
    ])

    temporada = pagina("temporada", ("temporada", "seasonal"), ("Fruta y verdura de temporada", "Seasonal fruit and veg"),
                       ("Calendario de temporada por estaciones, cestas con precio y consejos para elegir y guardar la fruta y la verdura.",
                        "Seasonal calendar by season, baskets with prices and tips for choosing and storing fruit and veg."), [
        hero(("Fruta y verdura de temporada", "Seasonal fruit and veg"),
             ("Lo de temporada sabe mejor y suele costar menos. Aquí tienes qué comprar en cada época y las cestas que preparamos.",
              "What is in season tastes better and usually costs less. Here is what to buy at each time of year and the baskets we make up."),
             [boton(("Hacer un pedido", "Place an order"), a_pedido), whatsapp("secondary")], variante="texto"),
        seccion("pricing", [
            titular(("Qué comprar en cada estación", "What to buy in each season"),
                    ("Calendario general. La temporada cambia según la zona y el año: cámbialo por lo que tú tengas.",
                     "A general calendar. Seasons change with the area and the year: change it to what you have.")),
            tabla(("Calendario de temporada por estaciones", "Seasonal calendar by season"),
                  [("Estación", "Season"), ("Fruta", "Fruit"), ("Verdura", "Vegetables")],
                  [(("Primavera", "Spring"), ("Fresas, nísperos, cerezas, albaricoques", "Strawberries, loquats, cherries, apricots"),
                    ("Espárragos, guisantes, habas, alcachofas", "Asparagus, peas, broad beans, artichokes")),
                   (("Verano", "Summer"), ("Melocotón, sandía, melón, ciruelas, higos", "Peaches, watermelon, melon, plums, figs"),
                    ("Tomate, pepino, pimiento, calabacín, berenjena", "Tomato, cucumber, pepper, courgette, aubergine")),
                   (("Otoño", "Autumn"), ("Uva, granada, caqui, manzana, membrillo", "Grapes, pomegranate, persimmon, apple, quince"),
                    ("Calabaza, boniato, coliflor, setas, castañas", "Pumpkin, sweet potato, cauliflower, mushrooms, chestnuts")),
                   (("Invierno", "Winter"), ("Naranja, mandarina, kiwi, pera, limón", "Orange, mandarin, kiwi, pear, lemon"),
                    ("Espinacas, acelgas, col, puerro, brócoli", "Spinach, chard, cabbage, leek, broccoli"))]),
        ], tono="base", anchor="calendario", gap=28),
        bloque_precios(("Cestas para recoger o repartir", "Baskets to collect or deliver"), ("Cestas y precios", "Baskets and prices"),
                       [("Cesta", "Basket"), ("Qué lleva", "What is in it"), ("Precio", "Price")],
                       [(("Cesta de fruta", "Fruit basket"), ("Unos 3 kilos de fruta de temporada, variada", "About 3 kilos of mixed seasonal fruit"), eur(9)),
                        (("Cesta de verdura", "Vegetable basket"), ("Unos 4 kilos de verdura de temporada para la semana", "About 4 kilos of seasonal vegetables for the week"), eur(10)),
                        (("Cesta mixta", "Mixed basket"), ("Unos 6 kilos entre fruta y verdura", "About 6 kilos of fruit and veg"), eur(16))],
                       intro=("Llevan lo que esté mejor esa semana, así que no se puede elegir pieza por pieza. Si no comes algo, dínoslo.",
                              "They hold whatever is best that week, so you cannot pick item by item. If you do not eat something, tell us."),
                       nota=NOTA_PRECIOS, boton_=boton(("Pedir una cesta", "Order a basket"), a_pedido), tono="relleno", anchor="cestas"),
        bloque_valores(("Elegir y guardar", "Choosing and storing"), [
            (("Fruta para hoy o para el viernes", "Fruit for today or for Friday"),
             ("Dinos cuándo la comerás y te damos piezas maduras o verdes. Los plátanos y los aguacates maduran en casa.",
              "Tell us when you will eat it and we give you ripe or green pieces. Bananas and avocados ripen at home.")),
            (("En la nevera o fuera", "Fridge or counter"),
             ("Los tomates mejor fuera de la nevera. Las hojas verdes, en la nevera y secas. Las patatas, en un sitio oscuro.",
              "Tomatoes are better out of the fridge. Leafy greens go in the fridge, dry. Potatoes keep best in the dark.")),
            (("Lavar al usarla", "Wash when you use it"),
             ("Lava la fruta y la verdura justo antes de comerla, no al llegar a casa: dura más.", "Wash fruit and veg just before eating, not when you get home: it lasts longer.")),
        ], cols=3, tono="base", estilo="borde", anchor="consejos"),
        bloque_cta(("¿Quieres que te avisemos de lo que llega?", "Want us to tell you what is arriving?"),
                   ("Escríbenos por WhatsApp y te contamos qué ha llegado esta semana.", "Message us on WhatsApp and we will tell you what has arrived this week."),
                   [whatsapp("primary"), llamar("secondary")], tono="relleno"),
    ])

    pedidos = pagina("pedidos", ("pedidos", "orders"), ("Pedidos y reparto", "Orders and delivery"),
                     ("Pedido para recoger o repartir con lista, zona de reparto, formas de pago y preguntas frecuentes.",
                      "Order to collect or deliver with a list, delivery area, ways to pay and frequently asked questions."), [
        hero(("Pedidos y reparto", "Orders and delivery"),
             ("Manda la lista y pasa a recogerla, o te la llevamos a casa si vives en nuestra zona de reparto.",
              "Send your list and collect it, or we bring it to your door if you live in our delivery area."),
             [boton(("Hacer un pedido", "Place an order"), a_ancla("hacer-pedido")), whatsapp("secondary")], variante="texto"),
        sec_form(("Haz tu pedido", "Place your order"),
                 ("Escribe lo que necesitas, una cosa por línea y con la cantidad. Te confirmamos por teléfono o WhatsApp.",
                  "Write what you need, one item per line with the amount. We will confirm by phone or WhatsApp."),
                 campos_pedido(), ("Enviar pedido", "Send order"),
                 ("Gracias. Hemos recibido tu pedido y te confirmamos lo antes posible.", "Thank you. We have received your order and will confirm it as soon as possible."),
                 "hacer-pedido", tono="relleno"),
        seccion("pricing", [
            titular(("Reparto a domicilio", "Home delivery"),
                    ("Repartimos en Tu ciudad y en los barrios que ves abajo. Es un ejemplo: cámbialo por tu zona, tus días y tu pedido mínimo.",
                     "We deliver in Your city and in the areas below. This is an example: change it to your area, your days and your minimum order.")),
            tabla(("Zonas de reparto", "Delivery areas"),
                  [("Zona", "Area"), ("Cuándo repartimos", "When we deliver"), ("Pedido mínimo", "Minimum order")],
                  [(("Tu barrio (de ejemplo)", "Your neighbourhood (example)"), ("Por las mañanas, de lunes a sábado", "Mornings, Monday to Saturday"), eur(15)),
                   (("Barrios cercanos de Tu ciudad (de ejemplo)", "Nearby areas of Your city (example)"), ("Martes y viernes por la tarde", "Tuesday and Friday afternoons"), eur(25))]),
            peq(("Pedidos mínimos y horas de ejemplo: cámbialos por los tuyos. Si no estás en la zona, escríbenos y miramos qué se puede hacer.",
                 "Minimums and times are examples: replace them with your own. If you are outside the area, write to us and we will see what can be done."),
                color="$muted"),
        ], tono="base", anchor="reparto", gap=28),
        bloque_checks(("Cómo se paga", "How to pay"), [
            ("Efectivo, al recoger o al recibir el pedido.", "Cash, when you collect or receive the order."),
            ("Tarjeta, en la tienda.", "Card, in the shop."),
            ("Bizum, al confirmar el pedido: te decimos a qué número enviarlo.", "Bizum, when the order is confirmed: we tell you which number to send it to."),
        ], cols=1, tono="relleno", anchor="pago"),
        bloque_faq(("Preguntas frecuentes", "Frequently asked questions"), [
            (("¿Qué pasa si falta algo de mi lista?", "What if something on my list is missing?"),
             ("Te avisamos antes de prepararlo y te proponemos otra cosa parecida. Tú decides.",
              "We tell you before we prepare it and suggest something similar. You decide.")),
            (("¿Cobráis el reparto?", "Do you charge for delivery?"),
             ("Mira la tabla de reparto: ahí está el pedido mínimo de cada zona. Si hay un coste de envío, te lo decimos al confirmar.",
              "See the delivery table: it shows the minimum order for each area. If there is a delivery charge, we tell you when we confirm.")),
            (("¿Puedo pedir sin que me conozcan?", "Can I order if nobody knows me?"),
             ("Sí, solo necesitamos tu nombre y un teléfono para avisarte.", "Yes, we only need your name and a phone number to reach you.")),
            (("¿Con cuánta antelación hay que pedir?", "How far ahead should I order?"),
             ("Con la tarde anterior es suficiente. Para cestas o pedidos grandes, avísanos con dos días.",
              "The afternoon before is enough. For baskets or large orders, let us know two days ahead.")),
        ], tono="base"),
    ])

    cont = pagina("contacto", ("contacto", "contact"), ("Contacto y cómo llegar", "Contact and directions"),
                  ("Teléfono, WhatsApp, formulario, horario de la tienda y mapa.",
                   "Phone, WhatsApp, form, shop hours and map."), [
        hero(("Contacto", "Contact"),
             ("Pasa por la tienda, llámanos o escríbenos. Para pedidos, mira la página de pedidos y reparto.",
              "Drop by the shop, call us or write to us. For orders, see the orders and delivery page."),
             [llamar(), whatsapp()], variante="texto"),
        bloque_contacto(("Dónde estamos", "Where we are"),
                        ("Escríbenos si quieres saber si hay algo en concreto antes de bajar.",
                         "Write to us if you want to know whether we have something in particular before you come."),
                        campos_cita(servicios=[("Un pedido", "An order"), ("Una pregunta", "A question"), ("Otra cosa", "Something else")], fecha=False),
                        ("Enviar mensaje", "Send message"), horas,
                        notas=("Los festivos pueden cambiar el horario: lo avisamos en la puerta.", "Holidays may change the hours: we put a notice on the door.")),
        bloque_mapa(tono="relleno"),
    ])

    return plantilla(KEY, ("Frutería y tienda de barrio", "Greengrocer's and neighbourhood shop"),
                     ("Fruta y verdura de temporada con calendario, secciones de la tienda, cestas con precio, pedido con lista para recoger o repartir, horario y mapa, con botones de llamar y WhatsApp.",
                      "Seasonal fruit and veg with a calendar, shop sections, priced baskets, an order form with a list to collect or deliver, hours and a map, with call and WhatsApp buttons."),
                     "comercio", ("fruteria tienda de barrio ultramarinos colmado fruta verdura hortalizas frutas y verduras cestas reparto a domicilio pedidos tienda de alimentacion",
                                  "greengrocer fruit and veg corner shop grocery store neighbourhood shop baskets home delivery orders produce"),
                     look, cabecera(nav, cta_cab),
                     pie(nav, ("Frutería y tienda de barrio. Pedidos para recoger o repartir.",
                               "Greengrocer's and neighbourhood shop. Orders to collect or deliver."),
                         ("Lunes a viernes 8:00 a 14:00 y 17:00 a 20:30. Sábado 8:00 a 14:30.",
                          "Monday to Friday 8 am to 2 pm and 5 pm to 8:30 pm. Saturday 8 am to 2:30 pm.")),
                     [inicio, temporada, pedidos, cont])
