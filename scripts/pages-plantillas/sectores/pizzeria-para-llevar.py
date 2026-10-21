# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
from lib import *

KEY = "pizzeria-para-llevar"

# Marrón de horno de leña con tomate vivo: fondo oscuro cálido, pensado para pedir desde el móvil de noche.
HORNO = dict(
    colores=dict(background="#1c1410", surface="#2a1e18", text="#f6ede4", muted="#c9b8a8", primary="#f2542d",
                 on_primary="#1c1410", border="#47362b", inverse="#0f0a08", on_inverse="#f6ede4"),
    fuentes=("figtree", "inter"), peso=800, h1=(52, 40, 32), radio=(8, 12, 18), ancho=1120)

NOTA = ("Precios de ejemplo: cámbialos por los tuyos. Incluyen IVA.", "Example prices: replace them with your own. VAT included.")
PIZZA = [("Pizza", "Pizza"), ("Individual 26 cm", "Single 26 cm"), ("Mediana 32 cm", "Medium 32 cm"), ("Familiar 40 cm", "Family 40 cm")]
PLATO = [("Producto", "Item"), ("Precio", "Price")]


def sec_form(titulo, intro, campos, envio, exito, anchor, tono="base"):
    return seccion("contact", [titular(titulo, intro),
                               tarjeta([formulario(campos, envio, exito)], estilo="relleno" if tono == "base" else "borde",
                                       pad=28, tag="div", sz=tam(max_width=680))],
                   tono=tono, anchor=anchor, gap=28)


def build():
    nueva(KEY)
    look = HORNO
    nav = [("inicio", ("Inicio", "Home")), ("carta", ("Carta", "Menu")),
           ("pedir", ("Cómo pedir", "How to order")), ("contacto", ("Contacto", "Contact"))]
    horas = [(("Lunes", "Monday"), ("Cerrado", "Closed")),
             (("Martes a viernes", "Tuesday to Friday"), ("19:00 a 23:30", "7:00 pm to 11:30 pm")),
             (("Sábado y domingo", "Saturday and Sunday"), ("13:00 a 16:00 y de 19:00 a 23:30", "1:00 pm to 4:00 pm and 7:00 pm to 11:30 pm"))]
    cta_cab = marco([llamar(), whatsapp("ghost")], dir="row", gap=8, wrap=True, align="center")
    pedir_wa = lambda t=("Pedir por WhatsApp", "Order on WhatsApp"): whatsapp("primary", t)

    ofertas = bloque_valores(("Menús y ofertas", "Set menus and deals"), [
        (("Menú individual · 9,50 €", "Single menu · €9.50"),
         ("Pizza individual a elegir y una bebida.", "A single pizza of your choice and a drink.")),
        (("Menú pareja · 21 €", "Menu for two · €21"),
         ("Dos pizzas individuales, un entrante para compartir y dos bebidas.", "Two single pizzas, a starter to share and two drinks.")),
        (("Menú familiar · 24 €", "Family menu · €24"),
         ("Una pizza familiar, un entrante y bebidas para cuatro.", "One family pizza, a starter and drinks for four.")),
    ], intro=("Ofertas de ejemplo: cámbialas por las tuyas. Todas valen para recoger y para llevar a casa.",
              "Example deals: change them to your own. All of them work for collection and for delivery."),
        tono="relleno", estilo="borde", anchor="menus", cols=3)
    ofertas["children"].append(boton(("Ver la carta completa", "See the full menu"), a_pagina("carta"), "secondary"))

    inicio = pagina("inicio", ("", ""), ("Pizzería para llevar", "Takeaway pizzeria"),
                    ("Pizzería para llevar y a domicilio: menús y ofertas, cómo pedir, zona de reparto y botones de llamar y WhatsApp.",
                     "Takeaway and delivery pizzeria: set menus and deals, how to order, delivery area and call and WhatsApp buttons."), [
        hero(("Pizza recién hecha, en tu casa o en tu mano", "Fresh pizza, at your door or in your hand"),
             ("Masa del día, horno caliente y reparto por tu barrio. Pide por teléfono o por WhatsApp y dinos si prefieres pasar a recogerla.",
              "Dough made today, a hot oven and delivery around your neighbourhood. Order by phone or WhatsApp and tell us if you would rather collect it."),
             [llamar(), whatsapp("secondary"), boton(("Ver la carta", "See the menu"), a_pagina("carta"), "ghost")],
             eyebrow=("Para llevar y a domicilio", "Takeaway and delivery"), imagen_=("photo", ("Pizza recién sacada del horno sobre una caja de cartón abierta", "A pizza fresh from the oven on an open cardboard box"))),
        bloque_valores(("Para qué momento", "For every occasion"), [
            (("Cena entre semana", "Midweek dinner"), ("Una individual y a casa: en media hora, si estás en la zona.", "A single pizza sent home: in half an hour if you are in the area.")),
            (("Fin de semana en familia", "Weekend with the family"), ("Familiares y menús para cuatro, con entrante y bebidas.", "Family pizzas and menus for four, with a starter and drinks.")),
            (("Pedido para la oficina o el cole", "Order for the office or school"), ("Varias pizzas a la vez: avísanos con tiempo y las tenemos a la hora.", "Several pizzas at once: tell us ahead of time and they will be ready on the dot.")),
        ], cols=3, estilo="relleno", anchor="momentos"),
        ofertas,
        bloque_pasos(("Pedir es fácil", "Ordering is easy"), [
            (("Elige", "Choose"), ("Mira la carta y decide tamaño y masa.", "Look at the menu and pick a size and a base.")),
            (("Llama o escribe", "Call or message"), ("Por teléfono o WhatsApp. Dinos tu dirección o que pasas a recogerla.", "By phone or WhatsApp. Give us your address or say you will collect it.")),
            (("Te damos la hora", "We give you a time"), ("Te decimos cuánto tarda según el momento.", "We tell you how long it takes at that moment.")),
            (("Recibe y paga", "Receive and pay"), ("Pagas al recibirla o al recogerla.", "You pay when it arrives or when you collect it.")),
        ], tono="base", anchor="como-pedir"),
        bloque_opiniones(("Lo que dicen de nuestras pizzas", "What people say about our pizzas"),
                         [("la masa", "the base"), ("el reparto", "the delivery"), ("el trato", "the service")], tono="relleno"),
        bloque_cta(("¿Cenamos pizza?", "Pizza for dinner?"),
                   ("Llámanos o escríbenos y te decimos cuánto tarda hoy.", "Call or message us and we will tell you how long it takes today."),
                   [llamar(), whatsapp("ghost")], tono="base"),
    ])

    carta = pagina("carta", ("carta", "menu"), ("Carta y alérgenos", "Menu and allergens"),
                   ("Carta de pizzas por tamaños, entrantes, postres y bebidas con precios, alérgenos y preguntas frecuentes.",
                    "Pizzas by size, starters, desserts and drinks with prices, allergens and frequently asked questions."), [
        hero(("La carta", "The menu"),
             ("Pizzas en tres tamaños, entrantes para compartir, postres y bebidas. Si tienes una alergia, dínoslo antes de pedir.",
              "Pizzas in three sizes, starters to share, desserts and drinks. If you have an allergy, tell us before you order."),
             [pedir_wa(), llamar("secondary")], variante="texto", tono="relleno"),
        bloque_carta(("Pizzas", "Pizzas"), [
            (("Pizzas clásicas", "Classic pizzas"), ("Pizzas clásicas por tamaños", "Classic pizzas by size"), PIZZA,
             [(("Margarita: tomate, mozzarella y albahaca", "Margherita: tomato, mozzarella and basil"), eur(7.5), eur(10.5), eur(14)),
              (("Jamón y queso", "Ham and cheese"), eur(8), eur(11), eur(15)),
              (("Pepperoni", "Pepperoni"), eur(8.5), eur(12), eur(16)),
              (("Cuatro quesos", "Four cheeses"), eur(9), eur(12.5), eur(16.5)),
              (("Carbonara: nata, bacón y champiñón", "Carbonara: cream, bacon and mushroom"), eur(9), eur(12.5), eur(16.5))]),
            (("Pizzas de la casa", "House pizzas"), ("Pizzas de la casa por tamaños", "House pizzas by size"), PIZZA,
             [(("Barbacoa: carne, cebolla y salsa barbacoa", "Barbecue: beef, onion and barbecue sauce"), eur(9.5), eur(13), eur(17.5)),
              (("Vegetal: pimiento, cebolla, champiñón y maíz", "Veggie: pepper, onion, mushroom and sweetcorn"), eur(8.5), eur(12), eur(16)),
              (("Atún y cebolla", "Tuna and onion"), eur(8.5), eur(12), eur(16)),
              (("Mitad y mitad", "Half and half"), ("Precio de la más cara", "Price of the dearer half"), ("Precio de la más cara", "Price of the dearer half"), ("Precio de la más cara", "Price of the dearer half"))]),
            (("Extras", "Extras"), ("Extras de las pizzas", "Pizza extras"), PLATO,
             [(("Ingrediente extra", "Extra topping"), eur(1.5)), (("Borde relleno de queso", "Cheese-filled crust"), eur(2)),
              (("Masa fina o masa gruesa", "Thin or thick base"), ("Sin coste", "No charge")),
              (("Masa sin gluten (consúltanos)", "Gluten-free base (ask us)"), eur(3))]),
        ], nota=NOTA, tono="base", anchor="pizzas",
            intro=("Los precios de ejemplo van por tamaños. Si llevas una pizza grande a una cena de seis, la familiar sale a mejor precio por ración.",
                   "The example prices are by size. If you are feeding six, the family size works out cheaper per slice.")),
        bloque_carta(("Entrantes, postres y bebidas", "Starters, desserts and drinks"), [
            (("Para compartir", "To share"), ("Entrantes", "Starters"), PLATO,
             [(("Pan de ajo con queso", "Garlic bread with cheese"), eur(4.5)), (("Patatas con salsa", "Chips with sauce"), eur(4)),
              (("Alitas de pollo (8)", "Chicken wings (8)"), eur(6.5)), (("Ensalada mixta", "Mixed salad"), eur(5.5))]),
            (("Postres y bebidas", "Desserts and drinks"), ("Postres y bebidas", "Desserts and drinks"), PLATO,
             [(("Tiramisú", "Tiramisu"), eur(4)), (("Brownie con helado", "Brownie with ice cream"), eur(4.5)),
              (("Refresco o agua", "Soft drink or water"), eur(2)), (("Refresco de 1,5 litros", "1.5 litre soft drink"), eur(3.5)),
              (("Cerveza", "Beer"), eur(2.5))]),
        ], nota=NOTA, tono="relleno", anchor="entrantes"),
        seccion("text", [
            titular(("Alérgenos", "Allergens"),
                    ("Pregúntanos antes de pedir y te decimos qué lleva cada cosa. En una cocina con harina, queso y frutos secos puede haber trazas, también en la masa sin gluten.",
                     "Ask us before you order and we will tell you what each item contains. In a kitchen with flour, cheese and nuts there can be traces, including in the gluten-free base.")),
            tabla(("Alérgenos por producto", "Allergens by product"),
                  [("Producto", "Product"), ("Puede llevar", "May contain")],
                  [(("Masa de pizza", "Pizza base"), ("Gluten", "Gluten")),
                   (("Mozzarella y quesos", "Mozzarella and cheeses"), ("Leche", "Milk")),
                   (("Salsa carbonara", "Carbonara sauce"), ("Leche, huevo y gluten", "Milk, egg and gluten")),
                   (("Salsa barbacoa", "Barbecue sauce"), ("Mostaza, soja y gluten", "Mustard, soy and gluten")),
                   (("Atún", "Tuna"), ("Pescado", "Fish")),
                   (("Tiramisú y brownie", "Tiramisu and brownie"), ("Gluten, huevo, leche y frutos de cáscara", "Gluten, egg, milk and tree nuts"))]),
            peq(("Alérgenos de ejemplo: cambia la tabla por los de tus recetas y revísala cuando cambies un ingrediente o un proveedor.",
                 "Example allergens: replace the table with those of your recipes and review it whenever you change an ingredient or a supplier."), color="$muted"),
        ], tono="base", anchor="alergenos", gap=24),
        bloque_faq(("Preguntas frecuentes", "Frequently asked questions"), [
            (("¿Puedo pedir media pizza de un sabor y media de otro?", "Can I order half one flavour and half another?"),
             ("Sí, en cualquier tamaño. Pagas el precio de la mitad más cara.", "Yes, in any size. You pay the price of the dearer half.")),
            (("¿Tenéis algo sin gluten?", "Do you have anything gluten-free?"),
             ("Hacemos la masa sin gluten bajo pedido. Se cocina en el mismo horno, así que puede haber trazas: si eres celíaca o celíaco, dínoslo al pedir.",
              "We make a gluten-free base to order. It is baked in the same oven, so traces are possible: if you are coeliac, tell us when you order.")),
            (("¿Hay opciones vegetarianas?", "Are there vegetarian options?"),
             ("La Margarita, la Cuatro quesos y la Vegetal no llevan carne ni pescado. Pregúntanos por las salsas.",
              "The Margherita, Four cheeses and Veggie pizzas have no meat or fish. Ask us about the sauces.")),
            (("¿Cómo se paga?", "How do I pay?"),
             ("Con efectivo o con tarjeta, al recibir o al recoger. Formas de pago de ejemplo: cámbialas por las que aceptes.",
              "By cash or card, on delivery or collection. Example payment methods: change them to the ones you accept.")),
        ], tono="relleno"),
        bloque_cta(("¿Ya sabes qué te apetece?", "Know what you fancy?"), None, [pedir_wa(), llamar("secondary")], tono="base"),
    ])

    pedir = pagina("pedir", ("como-pedir", "how-to-order"), ("Cómo pedir y reparto", "How to order and delivery"),
                   ("Cómo pedir en cuatro pasos, zona de reparto, mínimo de pedido, gastos de envío y recogida en el local.",
                    "How to order in four steps, delivery area, minimum order, delivery charges and collection at the shop."), [
        hero(("Cómo pedir y cuánto tarda", "How to order and how long it takes"),
             ("Por teléfono o por WhatsApp. Te llevamos la pizza a casa si estás en nuestra zona, o la tienes lista para recoger.",
              "By phone or on WhatsApp. We bring the pizza to your door if you are in our area, or it is ready for you to collect."),
             [llamar(), whatsapp("secondary")], variante="texto"),
        bloque_pasos(("Pide en cuatro pasos", "Order in four steps"), [
            (("Elige la pizza", "Pick your pizza"), ("Tamaño, masa y extras. Si tienes alguna alergia, avísanos ahora.", "Size, base and extras. If you have an allergy, tell us now.")),
            (("Dinos cómo la quieres", "Tell us how you want it"), ("A domicilio con tu dirección y piso, o para recoger en el local.", "Delivered with your address and floor, or to collect at the shop.")),
            (("Confirma la hora", "Confirm the time"), ("Te decimos la hora aproximada. Si hay mucha cola, te lo contamos con claridad.", "We give you an approximate time. If it is busy, we say so plainly.")),
            (("Paga al recibirla", "Pay when you get it"), ("En efectivo o con tarjeta, al recibir la pizza o al recogerla.", "In cash or by card, when you receive the pizza or collect it.")),
        ], tono="relleno", anchor="pasos"),
        bloque_precios(("Zona de reparto y mínimo de pedido", "Delivery area and minimum order"),
                       ("Zonas de reparto, mínimo y gastos de envío", "Delivery zones, minimum order and delivery charges"),
                       [("Zona", "Zone"), ("Pedido mínimo", "Minimum order"), ("Gastos de envío", "Delivery charge"), ("Tiempo aproximado", "Approximate time")],
                       [(("Centro de Tu ciudad", "Central Your city"), eur(10), ("Gratis", "Free"), ("30 a 40 minutos", "30 to 40 minutes")),
                        (("Barrios cercanos", "Nearby neighbourhoods"), eur(15), eur(2), ("40 a 50 minutos", "40 to 50 minutes")),
                        (("Afueras de Tu ciudad", "Outskirts of Your city"), eur(20), eur(3.5), ("50 a 60 minutos", "50 to 60 minutes")),
                        (("Recoger en el local", "Collect at the shop"), ("Sin mínimo", "No minimum"), ("Gratis", "Free"), ("15 a 20 minutos", "15 to 20 minutes"))],
                       intro=("Las zonas, los mínimos y los tiempos son de ejemplo: cámbialos por los de tu reparto. Si una calle queda fuera, pregúntanos.",
                              "Zones, minimums and times are examples: change them to your own delivery details. If your street is outside, ask us."),
                       nota=("Precios de ejemplo: cámbialos por los tuyos. Los viernes, los sábados y los días de lluvia puede tardar más.",
                             "Example prices: replace them with your own. On Fridays, Saturdays and rainy days it may take longer."),
                       tono="base", anchor="reparto"),
        bloque_checks(("Antes de pedir, ten en cuenta", "Before you order, bear in mind"), [
            ("Un pedido grande para una hora concreta, mejor con antelación: llámanos y lo anotamos.", "For a big order at a set time, it is best to book ahead: call us and we will note it down."),
            ("Dinos el piso, la puerta y un teléfono por si el repartidor no encuentra el portal.", "Give us the floor, the door and a phone number in case the rider cannot find the entrance."),
            ("Si pides para recoger, la pizza se mantiene caliente unos minutos en su caja: pasa cuando te avisemos.", "If you collect, the pizza stays warm in its box for a few minutes: come when we tell you it is ready."),
            ("Si algo no llega como esperabas, escríbenos el mismo día y lo vemos.", "If something does not arrive as you expected, message us the same day and we will sort it out."),
        ], cols=2, tono="relleno"),
        sec_form(("Haz tu pedido por escrito", "Place your order in writing"),
                 ("Ponlo aquí y te llamamos para confirmar. Para pedidos con prisa, mejor llama o escribe por WhatsApp.",
                  "Fill it in and we will call you to confirm. For urgent orders, it is better to call or message on WhatsApp."),
                 [campo("nombre", "text", ("Nombre", "Name"), True, auto="name", maxlen=120),
                  campo("telefono", "tel", ("Teléfono", "Phone"), True, auto="tel", maxlen=30),
                  campo("entrega", "select", ("¿Cómo lo quieres?", "How would you like it?"), True,
                        opciones=[("A domicilio", "Delivered"), ("Recoger en el local", "Collect at the shop")]),
                  campo("direccion", "text", ("Dirección (si es a domicilio)", "Address (if delivered)"), False, auto="street-address", maxlen=200),
                  campo("hora", "text", ("Hora a la que lo quieres", "Time you would like it"), False, maxlen=40),
                  campo("pedido", "textarea", ("Tu pedido y las alergias que tengamos que conocer", "Your order and any allergies we should know about"), True, maxlen=1000)],
                 ("Enviar pedido", "Send order"),
                 ("Gracias. Hemos recibido tu pedido y te llamamos enseguida para confirmar la hora.",
                  "Thank you. We have received your order and will call you shortly to confirm the time."),
                 "pedido-escrito", tono="base"),
    ])

    cont = pagina("contacto", ("contacto", "contact"), ("Contacto y cómo llegar", "Contact and how to get here"),
                  ("Teléfono, WhatsApp, formulario, horario de reparto y recogida y mapa del local.",
                   "Phone, WhatsApp, form, delivery and collection hours and a map of the shop."), [
        hero(("Contacto", "Contact"),
             ("Para pedir, llámanos o escríbenos por WhatsApp. Si tienes una pregunta, puedes dejarla aquí abajo.",
              "To order, call us or message us on WhatsApp. If you have a question, you can leave it below."),
             [llamar(), whatsapp()], variante="texto"),
        bloque_contacto(("Habla con nosotros", "Talk to us"),
                        ("El teléfono y WhatsApp están abiertos mientras hacemos pizzas. Fuera de horario, deja tu mensaje y te contestamos al abrir.",
                         "The phone and WhatsApp are open while we are making pizzas. Outside those hours, leave a message and we will reply when we open."),
                        campos_cita(servicios=[("Pedir para recoger", "Order for collection"), ("Pedir a domicilio", "Order for delivery"),
                                               ("Una pregunta", "A question"), ("Otra cosa", "Something else")], fecha=False),
                        ("Enviar mensaje", "Send message"), horas,
                        notas=("Horario de ejemplo, de reparto y de recogida: cámbialo por el tuyo.", "Example hours, for delivery and collection: change them to your own.")),
        bloque_mapa(("Cómo llegar para recoger tu pedido", "How to get here to collect your order"),
                    ("El mapa señala el local. Para recoger, avísanos al llegar y te lo damos por la ventanilla.",
                     "The map shows the shop. To collect, tell us when you arrive and we will hand it over at the counter."),
                    tono="relleno"),
    ])

    return plantilla(KEY, ("Pizzería para llevar", "Takeaway pizzeria"),
                     ("Pizzas por tamaños con precios, menús y ofertas, cómo pedir en pasos, zona de reparto con mínimo, alérgenos y formulario, con botones de llamar y WhatsApp.",
                      "Pizzas by size with prices, set menus and deals, how to order in steps, delivery area with minimum order, allergens and a form, with call and WhatsApp buttons."),
                     "comida", ("pizzeria pizza pizzas para llevar a domicilio reparto comida rapida takeaway pedir pizza horno italiana menu familiar",
                                "pizzeria pizza takeaway delivery fast food order pizza oven italian family menu collection"),
                     look, cabecera(nav, cta_cab),
                     pie(nav, ("Pizzería para llevar. Pizzas recién hechas, a domicilio y para recoger.",
                               "Takeaway pizzeria. Fresh pizzas, delivered or for collection."),
                         ("Martes a viernes 19:00 a 23:30. Fines de semana también a mediodía.", "Tuesday to Friday 7 pm to 11:30 pm. Weekends at lunchtime too.")),
                     [inicio, carta, pedir, cont])
