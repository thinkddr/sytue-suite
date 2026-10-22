# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
from lib import *
from looks import LOOKS

KEY = "panaderia-pasteleria"

NOTA_PRECIOS = ("Precios de ejemplo: cámbialos por los tuyos. Incluyen IVA.",
                "Example prices: replace them with your own. VAT included.")
PROD = [("Producto", "Item"), ("Precio", "Price")]
ALERGENOS = [
    ("En una panadería hay harina, huevo, leche y frutos secos por todas partes, así que cualquier producto puede llevar trazas de ellos.",
     "In a bakery there is flour, egg, milk and nuts everywhere, so any product may contain traces of them."),
    ("Si tienes alergia o intolerancia, pregúntanos qué lleva cada producto antes de comprarlo. Lo tenemos por escrito en la tienda.",
     "If you have an allergy or an intolerance, ask us what each product contains before you buy it. We have it in writing in the shop.")]


def sec_form(titulo, intro, campos, envio, exito, anchor, tono="base"):
    return seccion("contact", [titular(titulo, intro),
                               tarjeta([formulario(campos, envio, exito)], estilo="relleno" if tono == "base" else "borde",
                                       pad=28, tag="div", sz=tam(max_width=640))],
                   tono=tono, anchor=anchor, gap=28)


def build():
    nueva(KEY)
    look = LOOKS["durazno"]
    nav = [("inicio", ("Inicio", "Home")), ("productos", ("Pan y dulces", "Bread and sweets")),
           ("encargos", ("Tartas por encargo", "Cakes to order")), ("contacto", ("Contacto", "Contact"))]
    horas = [(("Lunes a viernes", "Monday to Friday"), ("7:00 a 14:00 y de 17:00 a 20:00", "7:00 am to 2:00 pm and 5:00 pm to 8:00 pm")),
             (("Sábado", "Saturday"), ("7:00 a 14:00", "7:00 am to 2:00 pm")),
             (("Domingo", "Sunday"), ("8:00 a 13:00", "8:00 am to 1:00 pm"))]
    cta_cab = marco([llamar(), whatsapp("ghost")], dir="row", gap=8, wrap=True, align="center")
    a_encargo = a_pagina("encargos", "encargar")

    inicio = pagina("inicio", ("", ""), ("Panadería y pastelería", "Bakery and patisserie"),
                    ("Panadería y pastelería: pan del día, bollería y tartas por encargo, con horario de tienda y botones de llamar y WhatsApp.",
                     "Bakery and patisserie: daily bread, pastries and cakes to order, with shop hours and call and WhatsApp buttons."), [
        hero(("Pan del día y tartas por encargo", "Daily bread and cakes to order"),
             ("Pan, bollería y dulces en la tienda, y tartas para tus celebraciones. Ven a por el pan o encarga con tiempo.",
              "Bread, pastries and sweets in the shop, and cakes for your celebrations. Come for your bread or order ahead."),
             [boton(("Encargar una tarta", "Order a cake"), a_encargo), llamar("secondary"), whatsapp("ghost")],
             eyebrow=("Panadería y pastelería", "Bakery and patisserie"), variante="centro", tono="relleno"),
        bloque_valores(("Qué encontrarás en la tienda", "What you will find in the shop"), [
            (("Pan del día", "Daily bread"), ("Barras, hogazas y panes especiales. Lo bueno se acaba, así que si lo quieres a una hora, resérvalo.",
                                              "Loaves of every size and special breads. The good ones sell out, so if you want some at a set time, reserve it.")),
            (("Bollería", "Pastries"), ("Croissants, napolitanas, palmeras y ensaimadas.", "Croissants, chocolate pastries, palmiers and ensaimadas.")),
            (("Pastelería", "Patisserie"), ("Pasteles individuales y porciones de tarta para llevar.", "Individual cakes and slices of cake to take away.")),
            (("Tartas por encargo", "Cakes to order"), ("Para cumpleaños, comuniones y bodas, con sabor y raciones a tu medida.",
                                                        "For birthdays, communions and weddings, with the flavour and number of servings you need.")),
        ], cols=4, estilo="relleno"),
        bloque_galeria(("Del obrador a la tienda", "From the bakery to the shop"), [
            ("Barras de pan recién hechas en la cesta", "Freshly baked loaves in the basket"),
            ("Bandeja de croissants", "Tray of croissants"),
            ("Tarta decorada para un cumpleaños", "Decorated birthday cake"),
            ("Hogazas de pan en el mostrador", "Round loaves on the counter"),
            ("Pasteles individuales en la vitrina", "Individual cakes in the display case"),
            ("Magdalenas en una bandeja", "Muffins on a tray")], tipo="product", tono="base"),
        bloque_texto(("Para una celebración, encarga con tiempo", "For a celebration, order ahead"), [
            ("Una tarta sencilla se prepara en dos días; una con decoración o de varios pisos, necesita más. Dinos la fecha y las raciones y te damos precio y plazo.",
             "A simple cake takes two days; one with decoration or several tiers needs longer. Tell us the date and the number of servings and we give you the price and lead time.")],
            tono="relleno", invertir=True, anchor="celebraciones",
            imagen_=("photo", ("Tarta de celebración con velas sobre una mesa", "Celebration cake with candles on a table"))),
        seccion("text", [titular(("Horario de la tienda", "Shop hours"),
                                 ("El pan del día se acaba: ven pronto o resérvalo.", "The daily bread sells out: come early or reserve it.")),
                         horario(horas)], tono="base", anchor="horario", gap=28),
        bloque_opiniones(("Lo que dicen nuestros clientes", "What our customers say"),
                         [("el pan", "the bread"), ("las tartas", "the cakes"), ("el trato", "the service")]),
        bloque_cta(("¿Quieres que te reservemos el pan o una tarta?", "Would you like us to reserve your bread or a cake?"),
                   ("Llámanos o escríbenos y te lo dejamos preparado.", "Call or message us and we will have it ready for you.")),
    ])

    prod = pagina("productos", ("pan-y-dulces", "bread-and-sweets"), ("Pan y dulces", "Bread and sweets"),
                  ("Precios de pan, bollería y pastelería de la tienda, con información de alérgenos.",
                   "Prices for bread, pastries and patisserie in the shop, with allergen information."), [
        hero(("Pan y dulces de la tienda", "Bread and sweets in the shop"),
             ("Lo que hacemos cada día, con su precio. Si buscas algo que no ves, pregúntanos.",
              "What we make each day, with its price. If you are looking for something you cannot see, ask us."),
             [llamar("secondary"), whatsapp("ghost")], variante="texto"),
        bloque_carta(("Pan, bollería y pastelería", "Bread, pastries and patisserie"), [
            (("Pan", "Bread"), ("Precios del pan", "Bread prices"), PROD,
             [(("Barra de pan", "Loaf of bread"), eur(0.8)), (("Hogaza de pan", "Round loaf"), eur(3.5)),
              (("Pan de masa madre", "Sourdough bread"), eur(3.8)), (("Pan integral", "Wholemeal bread"), eur(2.2))]),
            (("Bollería", "Pastries"), ("Precios de la bollería", "Pastry prices"), PROD,
             [(("Croissant", "Croissant"), eur(1.4)), (("Napolitana de chocolate", "Chocolate pastry"), eur(1.7)),
              (("Palmera", "Palmier"), eur(1.5)), (("Ensaimada individual", "Individual ensaimada"), eur(1.9)),
              (("Magdalena", "Muffin"), eur(1.2))]),
            (("Pastelería", "Patisserie"), ("Precios de la pastelería", "Patisserie prices"), PROD,
             [(("Pastel individual", "Individual cake"), desde(2.8)), (("Porción de tarta", "Slice of cake"), desde(3.5)),
              (("Caja de pastas (250 g)", "Box of biscuits (250 g)"), eur(7))]),
        ], nota=NOTA_PRECIOS, tono="base", anchor="precios"),
        bloque_texto(("Alérgenos", "Allergens"), ALERGENOS, tono="relleno", anchor="alergenos"),
        bloque_cta(("¿Te lo reservamos?", "Shall we reserve it for you?"), None, tono="base"),
    ])

    enc = pagina("encargos", ("encargos", "orders"), ("Tartas por encargo", "Cakes to order"),
                 ("Cómo encargar una tarta: pasos, tamaños y precios, plazos, formulario con fecha y raciones y preguntas frecuentes.",
                  "How to order a cake: steps, sizes and prices, lead times, a form with date and servings, and frequently asked questions."), [
        hero(("Tartas por encargo", "Cakes to order"),
             ("Dinos para cuándo la necesitas y cuántas raciones. Te contestamos con precio y plazo.",
              "Tell us when you need it and how many servings. We reply with a price and lead time."),
             [boton(("Hacer el encargo", "Place an order"), a_pagina("encargos", "encargar")), llamar("secondary"), whatsapp("ghost")],
             variante="texto", tono="relleno"),
        bloque_pasos(("Cómo encargar", "How to order"), [
            (("Cuéntanos la idea", "Tell us your idea"), ("Sabor, tamaño y ocasión. Si tienes una foto de referencia, mándanosla por WhatsApp.",
                                                          "Flavour, size and occasion. If you have a reference photo, send it on WhatsApp.")),
            (("Fecha y raciones", "Date and servings"), ("Dinos el día de recogida y cuántas personas comerán.", "Tell us the pick-up day and how many people will eat it.")),
            (("Te confirmamos", "We confirm"), ("Te damos el precio y el plazo antes de empezar.", "We give you the price and lead time before we start.")),
            (("Recógela", "Pick it up"), ("En la tienda, el día y a la hora acordados.", "In the shop, on the agreed day and time.")),
        ]),
        bloque_precios(("Tamaños y precios", "Sizes and prices"), ("Tamaños y precios de las tartas", "Cake sizes and prices"),
                       [("Tarta", "Cake"), ("Raciones", "Servings"), ("Precio", "Price")],
                       [(("Pequeña", "Small"), ("6 a 8", "6 to 8"), desde(24)), (("Mediana", "Medium"), ("10 a 12", "10 to 12"), desde(34)),
                        (("Grande", "Large"), ("15 a 20", "15 to 20"), desde(48)),
                        (("Para un evento", "For an event"), ("Más de 25", "More than 25"), ("Presupuesto sin compromiso", "Quote with no commitment"))],
                       nota=("Precios de ejemplo: cámbialos por los tuyos. La decoración y los rellenos especiales pueden cambiar el precio.",
                             "Example prices: replace them with your own. Decoration and special fillings can change the price."),
                       tono="relleno", anchor="tamanos"),
        bloque_precios(("Plazos", "Lead times"), ("Plazos para encargar", "Lead times for orders"),
                       [("Tipo de tarta", "Type of cake"), ("Encárgala con", "Order at least")],
                       [(("Tarta sencilla", "Simple cake"), ("2 días", "2 days")),
                        (("Con decoración personalizada", "With custom decoration"), ("5 días", "5 days")),
                        (("De varios pisos o de boda", "Tiered or wedding cake"), ("2 semanas", "2 weeks"))],
                       nota=("Plazos de ejemplo: ajústalos a lo que puede preparar tu obrador.", "Example lead times: adjust them to what your bakery can prepare."),
                       tono="base", anchor="plazos"),
        sec_form(("Haz tu encargo", "Place your order"),
                 ("Rellena el formulario y te respondemos con el precio y la confirmación.", "Fill in the form and we reply with the price and confirmation."),
                 [campo("nombre", "text", ("Nombre", "Name"), True, auto="name", maxlen=120),
                  campo("telefono", "tel", ("Teléfono", "Phone"), True, auto="tel", maxlen=30),
                  campo("fecha", "date", ("Día de recogida", "Pick-up day"), True),
                  campo("raciones", "select", ("Número de raciones", "Number of servings"), True,
                        opciones=[("6 a 8", "6 to 8"), ("10 a 12", "10 to 12"), ("15 a 20", "15 to 20"), ("Más de 20", "More than 20")]),
                  campo("tipo", "select", ("Tipo de tarta", "Type of cake"), False,
                        opciones=[("Cumpleaños", "Birthday"), ("Comunión o bautizo", "Communion or christening"),
                                  ("Boda", "Wedding"), ("Otra ocasión", "Other occasion")]),
                  campo("alergias", "text", ("Alergias o intolerancias", "Allergies or intolerances"), False, maxlen=200),
                  campo("mensaje", "textarea", ("Sabor, decoración y otros detalles", "Flavour, decoration and other details"), False, maxlen=1000)],
                 ("Enviar encargo", "Send order"),
                 ("Gracias. Te respondemos con el precio y la confirmación de la fecha lo antes posible.",
                  "Thank you. We will reply with the price and confirmation of the date as soon as possible."),
                 "encargar", tono="relleno"),
        bloque_faq(("Preguntas frecuentes", "Frequently asked questions"), [
            (("¿Con cuánta antelación debo encargar?", "How far ahead should I order?"),
             ("Una tarta sencilla, dos días antes. Con decoración, cinco. De boda o de varios pisos, dos semanas.",
              "A simple cake, two days ahead. With decoration, five. A wedding or tiered cake, two weeks.")),
            (("¿Puedo cambiar la fecha o las raciones?", "Can I change the date or the number of servings?"),
             ("Sí, avísanos cuanto antes. Si ya hemos empezado, te decimos qué se puede cambiar.",
              "Yes, tell us as soon as you can. If we have already started, we will tell you what can still be changed.")),
            (("¿Y si alguien es alérgico?", "What if someone has an allergy?"),
             ("Escríbelo en el formulario. Te decimos qué lleva cada tarta, aunque en el obrador hay harina, huevo, leche y frutos secos.",
              "Write it in the form. We tell you what each cake contains, although there is flour, egg, milk and nuts in the bakery.")),
            (("¿Cómo se paga?", "How do I pay?"),
             ("En la tienda, al recogerla. Si hace falta algo más, te lo decimos al confirmar el encargo.",
              "In the shop when you collect it. If anything else is needed, we tell you when we confirm the order.")),
        ], tono="base"),
    ])

    cont = pagina("contacto", ("contacto", "contact"), ("Contacto y horario", "Contact and opening hours"),
                  ("Llama o escribe por WhatsApp para reservar pan o una tarta, y mira el horario y cómo llegar a la tienda.",
                   "Call or message on WhatsApp to reserve bread or a cake, and see opening hours and how to get to the shop."), [
        hero(("Ven a la tienda o llámanos", "Visit the shop or call us"),
             ("Reserva el pan para una hora, encarga una tarta o pregúntanos lo que quieras.",
              "Reserve your bread for a set time, order a cake or ask us anything."),
             [llamar(), whatsapp()], variante="texto"),
        bloque_contacto(("Cómo encontrarnos", "How to find us"),
                        ("La tienda está a pie de calle. Para tartas, usa también el formulario de encargos.",
                         "The shop is at street level. For cakes, you can also use the order form."),
                        campos_consulta(servicios=[("Reservar pan", "Reserve bread"), ("Tarta por encargo", "Cake to order"), ("Otra consulta", "Other question")]),
                        ("Enviar mensaje", "Send message"), horas,
                        exito=("Gracias. Te respondemos lo antes posible.", "Thank you. We will reply as soon as possible.")),
        bloque_mapa(tono="relleno"),
    ])

    return plantilla(KEY, ("Panadería y pastelería", "Bakery and patisserie"),
                     ("Pan del día, bollería y pastelería con precios, tartas por encargo con formulario y plazos, y horario de tienda.",
                      "Daily bread, pastries and patisserie with prices, cakes to order with a form and lead times, and shop hours."),
                     "comida", ("panaderia pasteleria pan horno panadero pastelero tartas bolleria tartas por encargo reposteria dulces obrador tahona",
                                "bakery patisserie bread baker pastry cakes birthday cake custom cakes sweets"),
                     look, cabecera(nav, cta_cab),
                     pie(nav, ("Panadería y pastelería. Pan del día, bollería y tartas por encargo.",
                               "Bakery and patisserie. Daily bread, pastries and cakes to order."),
                         ("Lunes a viernes 7:00 a 14:00 y 17:00 a 20:00. Sábado 7:00 a 14:00.",
                          "Monday to Friday 7 am to 2 pm and 5 pm to 8 pm. Saturday 7 am to 2 pm.")),
                     [inicio, prod, enc, cont])
