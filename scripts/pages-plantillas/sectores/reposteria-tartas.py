# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
from lib import *

KEY = "reposteria-tartas"

# Paleta propia: lavanda empolvada con frambuesa oscura. Pasa AA;
# primary/inverse no llega a 3, así que no se usa `tono="oscuro"`.
FRAMBUESA = dict(
    colores=dict(background="#d9d2f5", surface="#cbc2ef", text="#1d1030", muted="#463b66", primary="#7c134f",
                 on_primary="#ffffff", border="#b9afe2", inverse="#2b0f22", on_inverse="#f1ecfc"),
    fuentes=("playfair-display", "manrope"), peso=600, h1=(52, 40, 32), radio=(8, 14, 22))

NOTA_PRECIOS = ("Precios de ejemplo: cámbialos por los tuyos. Incluyen IVA.",
                "Example prices: replace them with your own. VAT included.")


def sec_form(titulo, intro, campos, envio, exito, anchor, tono="base"):
    return seccion("contact", [titular(titulo, intro),
                               tarjeta([formulario(campos, envio, exito)], estilo="relleno" if tono == "base" else "borde",
                                       pad=28, tag="div", sz=tam(max_width=640))],
                   tono=tono, anchor=anchor, gap=28)


def build():
    nueva(KEY)
    look = FRAMBUESA
    nav = [("inicio", ("Inicio", "Home")), ("tartas", ("Tartas y sabores", "Cakes and flavours")),
           ("encargos", ("Cómo encargar", "How to order")), ("contacto", ("Contacto", "Contact"))]
    horas = [(("Lunes", "Monday"), ("Cerrado", "Closed")),
             (("Martes a viernes", "Tuesday to Friday"), ("10:00 a 14:00 y de 16:30 a 19:30", "10:00 am to 2:00 pm and 4:30 pm to 7:30 pm")),
             (("Sábado", "Saturday"), ("9:00 a 14:00", "9:00 am to 2:00 pm")),
             (("Domingo", "Sunday"), ("Solo recogidas de encargos acordados", "Only pick-ups of agreed orders"))]
    cta_cab = marco([llamar(), whatsapp("ghost")], dir="row", gap=8, wrap=True, align="center")
    a_encargar = a_pagina("encargos", "encargar")

    inicio = pagina("inicio", ("", ""), ("Repostería y tartas personalizadas", "Custom cakes and patisserie"),
                    ("Obrador de repostería: tartas personalizadas por encargo, sabores y rellenos, mesas dulces para eventos y recogida o entrega.",
                     "Patisserie workshop: custom cakes to order, flavours and fillings, dessert tables for events and pick-up or delivery."), [
        hero(("Tartas hechas para tu celebración", "Cakes made for your celebration"),
             ("Cuéntanos la ocasión, las raciones y el sabor, y la preparamos en el obrador. Para cumpleaños, bautizos, bodas y mesas dulces.",
              "Tell us the occasion, the servings and the flavour, and we make it in our workshop. For birthdays, christenings, weddings and dessert tables."),
             [boton(("Encargar una tarta", "Order a cake"), a_encargar), boton(("Ver tartas y sabores", "See cakes and flavours"), a_pagina("tartas"), "secondary"),
              whatsapp("ghost")],
             eyebrow=("Obrador de repostería", "Patisserie workshop"),
             imagen_=("product", ("Tarta decorada con flores de crema sobre un pie", "Cake decorated with cream flowers on a stand"))),
        bloque_pasos(("Encargar es sencillo", "Ordering is simple"), [
            (("Cuéntanos la idea", "Tell us your idea"), ("Ocasión, número de personas, sabor y, si tienes, una foto de referencia.",
                                                          "Occasion, number of people, flavour and, if you have one, a reference photo.")),
            (("Te damos precio y plazo", "We give you a price and lead time"), ("Respondemos con el presupuesto y la fecha en la que podemos tenerla.",
                                                                                "We reply with a quote and the date we can have it ready.")),
            (("Reservamos tu fecha", "We book your date"), ("Con tu conformidad, apuntamos el día y la hora de recogida o entrega.",
                                                             "Once you agree, we note the day and the time of pick-up or delivery.")),
            (("La recoges o te la llevamos", "You collect it or we deliver it"), ("Te explicamos cómo conservarla y transportarla.",
                                                                                   "We explain how to keep and transport it.")),
        ], tono="relleno"),
        bloque_valores(("Qué hacemos", "What we make"), [
            (("Tartas personalizadas", "Custom cakes"),
             ("Con la decoración, el rotulado y el tamaño que necesites.", "With the decoration, lettering and size you need.")),
            (("Tartas clásicas", "Classic cakes"),
             ("Queso, chocolate, zanahoria o limón, listas con unos días de antelación.", "Cheesecake, chocolate, carrot or lemon, ready with a few days' notice.")),
            (("Mesas dulces", "Dessert tables"),
             ("Cupcakes, galletas y mini tartas para eventos, montadas en el lugar si hace falta.", "Cupcakes, cookies and mini cakes for events, set up on site if needed.")),
        ], estilo="borde", cols=3, eyebrow=("Del obrador", "From the workshop"), anchor="que-hacemos"),
        bloque_galeria(("Por dentro, lo que hay", "What is inside"), [
            ("Tarta de chocolate cortada con relleno de ganache", "Chocolate cake cut open showing a ganache filling"),
            ("Tarta de zanahoria con crema de queso entre las capas", "Carrot cake with cream cheese between the layers"),
            ("Tarta de limón con crema de merengue", "Lemon cake with meringue cream"),
            ("Tarta de vainilla con mermelada de frutos rojos", "Vanilla cake with red berry jam"),
            ("Cupcakes decorados sobre una bandeja", "Decorated cupcakes on a tray"),
            ("Galletas decoradas para un bautizo", "Decorated cookies for a christening")], tipo="product", tono="base"),
        bloque_texto(("Si hay alergias, cuéntanoslo antes", "If there are allergies, tell us first"), [
            ("En el obrador se trabaja con harina de trigo, huevo, leche y frutos secos, así que cualquier producto puede llevar trazas. Si alguien tiene una alergia o una intolerancia, díselo a quien encarga y te decimos qué se puede hacer y qué no.",
             "The workshop handles wheat flour, egg, milk and nuts, so any product may contain traces. If someone has an allergy or an intolerance, tell us when you order and we say what can and cannot be done.")],
            tono="relleno", anchor="alergias-inicio"),
        bloque_opiniones(("Lo que dicen quienes han encargado", "What our customers say"),
                         [("la tarta", "the cake"), ("el sabor", "the flavour"), ("la entrega", "the delivery")], tono="base"),
        bloque_cta(("¿Qué celebras y para cuándo?", "What are you celebrating and when?"),
                   ("Dinos la fecha y el número de raciones y te respondemos con precio y plazo.",
                    "Tell us the date and the number of servings and we reply with a price and lead time."),
                   [boton(("Encargar una tarta", "Order a cake"), a_encargar), llamar("secondary"), whatsapp("ghost")], tono="relleno"),
    ])

    tartas = pagina("tartas", ("tartas", "cakes"), ("Tartas y sabores", "Cakes and flavours"),
                    ("Tipos de tarta, tamaños y raciones con precio de ejemplo, sabores y rellenos con alérgenos, y opciones sin gluten con aviso.",
                     "Types of cake, sizes and servings with example prices, flavours and fillings with allergens, and gluten-free options with a notice."), [
        hero(("Tartas y sabores", "Cakes and flavours"),
             ("Elige el tamaño según las raciones, el bizcocho y el relleno. La decoración se habla aparte, según lo que quieras.",
              "Choose the size by servings, then the sponge and the filling. Decoration is discussed separately, depending on what you want."),
             [boton(("Encargar una tarta", "Order a cake"), a_encargar), llamar("secondary"), whatsapp("ghost")], variante="texto"),
        bloque_valores(("Tipos de tarta", "Types of cake"), [
            (("Tarta de capas", "Layer cake"), ("Bizcochos con crema o ganache entre capas. La más pedida para cumpleaños.", "Sponges with cream or ganache between layers. The most requested for birthdays.")),
            (("Tarta de queso", "Cheesecake"), ("Horneada o fría, con base de galleta y cobertura de fruta o chocolate.", "Baked or chilled, with a biscuit base and a fruit or chocolate topping.")),
            (("Tarta decorada", "Decorated cake"), ("Con rotulado, figuras o flores de crema sobre bizcocho y relleno a tu elección.", "With lettering, figures or cream flowers on a sponge and filling of your choice.")),
            (("Tartas de varios pisos", "Tiered cakes"), ("Para bodas y celebraciones grandes. Se presupuestan según el diseño.", "For weddings and big celebrations. Priced according to the design.")),
        ], cols=4, estilo="relleno", anchor="tipos"),
        bloque_precios(("Tamaños y precios", "Sizes and prices"), ("Tamaños, raciones y precios de las tartas", "Cake sizes, servings and prices"),
                       [("Tarta", "Cake"), ("Diámetro", "Diameter"), ("Raciones", "Servings"), ("Precio", "Price")],
                       [(("Pequeña", "Small"), ("16 cm", "16 cm"), ("6 a 8", "6 to 8"), desde(28)),
                        (("Mediana", "Medium"), ("20 cm", "20 cm"), ("10 a 12", "10 to 12"), desde(38)),
                        (("Grande", "Large"), ("24 cm", "24 cm"), ("16 a 20", "16 to 20"), desde(52)),
                        (("De varios pisos", "Tiered"), ("Según diseño", "Depends on design"), ("30 o más", "30 or more"),
                         ("Presupuesto sin compromiso", "Quote with no commitment"))],
                       nota=("Precios de ejemplo: cámbialos por los tuyos. La decoración personalizada, las figuras y los rellenos especiales se suman al precio base.",
                             "Example prices: replace them with your own. Custom decoration, figures and special fillings are added to the base price."),
                       tono="base", anchor="tamanos"),
        bloque_precios(("Extras de decoración", "Decoration extras"), ("Precios de los extras de decoración", "Decoration extras prices"),
                       [("Extra", "Extra"), ("Precio", "Price")],
                       [(("Rotulado con un mensaje", "Lettering with a message"), eur(6)),
                        (("Flores de crema o fruta fresca", "Cream flowers or fresh fruit"), desde(10)),
                        (("Figura modelada (cada una)", "Modelled figure (each)"), desde(15)),
                        (("Foto o dibujo comestible", "Edible photo or drawing"), eur(9))],
                       nota=NOTA_PRECIOS, tono="relleno", anchor="extras"),
        bloque_precios(("Sabores y rellenos", "Flavours and fillings"), ("Sabores, rellenos y alérgenos principales", "Flavours, fillings and main allergens"),
                       [("Bizcocho", "Sponge"), ("Relleno que le va bien", "Filling that suits it"), ("Alérgenos principales", "Main allergens")],
                       [(("Vainilla", "Vanilla"), ("Mermelada de frutos rojos o crema de mantequilla", "Red berry jam or buttercream"), ("Gluten, huevo, lácteos", "Gluten, egg, dairy")),
                        (("Chocolate", "Chocolate"), ("Ganache de chocolate o crema de avellanas", "Chocolate ganache or hazelnut cream"), ("Gluten, huevo, lácteos, frutos de cáscara", "Gluten, egg, dairy, tree nuts")),
                        (("Zanahoria", "Carrot"), ("Crema de queso", "Cream cheese frosting"), ("Gluten, huevo, lácteos, frutos de cáscara", "Gluten, egg, dairy, tree nuts")),
                        (("Limón", "Lemon"), ("Crema de limón o merengue", "Lemon cream or meringue"), ("Gluten, huevo, lácteos", "Gluten, egg, dairy")),
                        (("Queso (horneada)", "Cheesecake (baked)"), ("Fruta roja o dulce de leche", "Red fruit or caramel"), ("Gluten, huevo, lácteos", "Gluten, egg, dairy"))],
                       nota=("Sabores de ejemplo: cámbialos por los tuyos y revisa los alérgenos de cada receta.",
                             "Example flavours: replace them with your own and check the allergens of each recipe."),
                       tono="base", anchor="sabores"),
        bloque_galeria(("Rellenos", "Fillings"), [
            ("Corte de una tarta con ganache de chocolate", "Slice of a cake with chocolate ganache"),
            ("Corte de una tarta con crema de queso y zanahoria", "Slice of a cake with cream cheese and carrot"),
            ("Corte de una tarta con mermelada de frutos rojos", "Slice of a cake with red berry jam"),
            ("Corte de una tarta con crema de limón", "Slice of a cake with lemon cream")], tipo="product", cols=4, tono="relleno", ratio=1.2),
        bloque_texto(("Alérgenos y opciones sin gluten", "Allergens and gluten-free options"), [
            ("Podemos hacer una tarta con harinas sin gluten. Pero en el obrador se trabaja también con harina de trigo, y no podemos asegurar que no queden trazas.",
             "We can make a cake with gluten-free flours. But the workshop also handles wheat flour, and we cannot promise there will be no traces."),
            ("Por eso, si la tarta es para una persona celíaca o con una alergia grave, díselo a quien encarga antes de pedirla: te decimos con franqueza si podemos hacerla o no.",
             "So, if the cake is for someone with coeliac disease or a serious allergy, tell us before you order: we say plainly whether we can make it or not.")],
            tono="base", anchor="alergenos"),
        bloque_cta(("¿Cuál se parece a lo que buscas?", "Which one is closest to what you want?"), None, tono="relleno"),
    ])

    enc = pagina("encargos", ("encargos", "orders"), ("Cómo encargar", "How to order"),
                 ("Pasos para encargar una tarta personalizada, plazos, mesas dulces, recogida o entrega, formulario de encargo y preguntas frecuentes.",
                  "Steps to order a custom cake, lead times, dessert tables, pick-up or delivery, an order form and frequently asked questions."), [
        hero(("Cómo encargar una tarta", "How to order a cake"),
             ("Cuanto antes nos avises, más margen hay para el diseño y la fecha. Por teléfono, por WhatsApp o con el formulario.",
              "The sooner you tell us, the more room there is for the design and the date. By phone, on WhatsApp or with the form."),
             [llamar(), whatsapp(), boton(("Hacer el encargo", "Place an order"), a_pagina("encargos", "encargar"), "ghost")],
             variante="texto", tono="relleno"),
        bloque_pasos(("Pasos para una tarta personalizada", "Steps for a custom cake"), [
            (("Idea y referencias", "Idea and references"), ("Mándanos fotos de lo que te gusta, los colores y, si quieres, un mensaje para escribir.",
                                                              "Send us photos of what you like, the colours and, if you want, a message to write on it.")),
            (("Presupuesto", "Quote"), ("Te decimos el precio, el plazo y qué incluye, antes de que decidas nada.", "We tell you the price, the lead time and what is included, before you decide anything.")),
            (("Reserva de la fecha", "Booking the date"), ("Para apartar el día pedimos una señal, que se descuenta del total.", "To hold the day we ask for a deposit, which is deducted from the total.")),
            (("Entrega o recogida", "Pick-up or delivery"), ("Te la entregamos lista para servir, con indicaciones para conservarla.", "We hand it over ready to serve, with instructions on how to keep it.")),
        ], tono="base", anchor="pasos"),
        bloque_precios(("Con cuánta antelación encargar", "How far ahead to order"), ("Plazos para encargar", "Lead times for orders"),
                       [("Encargo", "Order"), ("Encárgalo con", "Order at least")],
                       [(("Tarta clásica (sin decoración especial)", "Classic cake (no special decoration)"), ("3 días", "3 days")),
                        (("Tarta personalizada con rotulado o figuras", "Custom cake with lettering or figures"), ("1 semana", "1 week")),
                        (("Tarta de varios pisos o de boda", "Tiered or wedding cake"), ("3 semanas", "3 weeks")),
                        (("Mesa dulce para un evento", "Dessert table for an event"), ("2 semanas", "2 weeks"))],
                       nota=("Plazos de ejemplo: ajústalos a lo que puede preparar tu obrador. En fechas de mucha demanda, mejor con más margen.",
                             "Example lead times: adjust them to what your workshop can prepare. On busy dates, allow more time."),
                       tono="relleno", anchor="plazos"),
        bloque_precios(("Mesas dulces para eventos", "Dessert tables for events"), ("Mesas dulces: personas, qué incluyen y precio", "Dessert tables: guests, what is included and price"),
                       [("Mesa", "Table"), ("Para", "For"), ("Incluye", "Includes"), ("Precio", "Price")],
                       [(("Pequeña", "Small"), ("Hasta 20 personas", "Up to 20 guests"), ("Cupcakes, galletas y una tarta", "Cupcakes, cookies and one cake"), desde(160)),
                        (("Mediana", "Medium"), ("Hasta 40 personas", "Up to 40 guests"), ("Cupcakes, galletas, mini tartas y una tarta", "Cupcakes, cookies, mini cakes and one cake"), desde(300)),
                        (("A medida", "Made to measure"), ("Más de 40 personas", "More than 40 guests"), ("Según lo que necesites", "Depends on what you need"),
                         ("Presupuesto sin compromiso", "Quote with no commitment"))],
                       nota=("Precios y contenido de ejemplo: cámbialos por los tuyos. El montaje en el lugar del evento se presupuesta aparte.",
                             "Example prices and contents: replace them with your own. Setting up at the event venue is quoted separately."),
                       tono="base", anchor="mesas-dulces"),
        bloque_valores(("Recogida o entrega", "Pick-up or delivery"), [
            (("Recogida en el obrador", "Pick-up at the workshop"),
             ("El día y a la hora acordados. Si llegas antes o después, avísanos y lo organizamos.",
              "On the agreed day and time. If you will be early or late, let us know and we sort it out.")),
            (("Entrega a domicilio o en el evento", "Delivery to your home or the event"),
             ("Según la zona y el tipo de tarta. Te decimos el coste al confirmar el encargo.",
              "Depending on the area and the type of cake. We tell you the cost when we confirm the order.")),
            (("Cómo transportarla", "How to transport it"),
             ("En el suelo del coche o en el maletero, sobre una superficie plana, nunca en el asiento. Y en la nevera hasta el momento de servirla.",
              "On the car floor or in the boot, on a flat surface, never on the seat. And in the fridge until serving.")),
        ], estilo="borde", cols=3, tono="relleno", anchor="entrega"),
        sec_form(("Haz tu encargo", "Place your order"),
                 ("Cuéntanos la ocasión, la fecha y las raciones. Te respondemos con el presupuesto y el plazo.",
                  "Tell us the occasion, the date and the servings. We reply with a quote and the lead time."),
                 [campo("nombre", "text", ("Nombre", "Name"), True, auto="name", maxlen=120),
                  campo("telefono", "tel", ("Teléfono", "Phone"), True, auto="tel", maxlen=30),
                  campo("fecha", "date", ("Día de recogida o entrega", "Pick-up or delivery day"), True),
                  campo("producto", "select", ("¿Qué necesitas?", "What do you need?"), True,
                        opciones=[("Tarta clásica", "Classic cake"), ("Tarta personalizada", "Custom cake"),
                                  ("Tarta de varios pisos o de boda", "Tiered or wedding cake"), ("Mesa dulce", "Dessert table")]),
                  campo("raciones", "select", ("Raciones", "Servings"), True,
                        opciones=[("6 a 8", "6 to 8"), ("10 a 12", "10 to 12"), ("16 a 20", "16 to 20"), ("Más de 20", "More than 20")]),
                  campo("entrega", "select", ("Recogida o entrega", "Pick-up or delivery"), False,
                        opciones=[("Recogeré en el obrador", "I will collect at the workshop"), ("Entrega a domicilio o en el evento", "Delivery to my home or the event")]),
                  campo("alergias", "text", ("Alergias o intolerancias de quien la va a comer", "Allergies or intolerances of the people eating it"), False, maxlen=200),
                  campo("mensaje", "textarea", ("Sabor, colores, decoración y mensaje", "Flavour, colours, decoration and message"), False, maxlen=1000)],
                 ("Enviar encargo", "Send order"),
                 ("Gracias. Te respondemos con el presupuesto y la confirmación de la fecha lo antes posible.",
                  "Thank you. We reply with a quote and confirmation of the date as soon as possible."),
                 "encargar", tono="base"),
        bloque_faq(("Preguntas frecuentes", "Frequently asked questions"), [
            (("¿Puedo llevar una foto de referencia?", "Can I bring a reference photo?"),
             ("Sí, mándala por WhatsApp. La usamos de guía: no hacemos copias exactas de otros trabajos, pero sí algo muy parecido en estilo y colores.",
              "Yes, send it on WhatsApp. We use it as a guide: we do not make exact copies of other people's work, but we can do something very close in style and colours.")),
            (("¿Qué pasa si cambio la fecha o las raciones?", "What if I change the date or the servings?"),
             ("Avísanos cuanto antes. Si todavía no hemos empezado, no hay problema; si sí, te decimos qué se puede cambiar.",
              "Tell us as soon as you can. If we have not started yet, there is no problem; if we have, we say what can still be changed.")),
            (("¿Cuánto dura la tarta?", "How long does the cake keep?"),
             ("En la nevera aguanta bien un par de días. Sácala una hora antes de servirla para que se note el sabor.",
              "In the fridge it keeps well for a couple of days. Take it out an hour before serving so the flavour comes through.")),
            (("¿Cómo se paga?", "How do I pay?"),
             ("Una señal al reservar la fecha y el resto al recoger o recibir la tarta. Te lo ponemos por escrito en el presupuesto.",
              "A deposit when you book the date and the rest when you collect or receive the cake. We put it in writing in the quote.")),
        ], tono="relleno"),
    ])

    cont = pagina("contacto", ("contacto", "contact"), ("Contacto y horario", "Contact and opening hours"),
                  ("Llama o escribe por WhatsApp para encargar, consulta el horario de recogidas y mira cómo llegar al obrador.",
                   "Call or message on WhatsApp to order, check pick-up hours and see how to get to the workshop."), [
        hero(("Habla con el obrador", "Talk to the workshop"),
             ("Para encargar, preguntar por un sabor o por alérgenos, o pedir presupuesto para un evento.",
              "To order, ask about a flavour or allergens, or get a quote for an event."),
             [llamar(), whatsapp()], variante="texto"),
        bloque_contacto(("Cómo encontrarnos", "How to find us"),
                        ("El obrador atiende con horario de recogidas. Si vienes a probar o a ver una tarta, avísanos antes.",
                         "The workshop is open during pick-up hours. If you are coming to try or see a cake, let us know beforehand."),
                        campos_consulta(servicios=[("Encargo de tarta", "Cake order"), ("Mesa dulce", "Dessert table"),
                                                   ("Alergias e intolerancias", "Allergies and intolerances"), ("Otra consulta", "Other question")]),
                        ("Enviar mensaje", "Send message"), horas,
                        exito=("Gracias. Te respondemos lo antes posible.", "Thank you. We will reply as soon as possible.")),
        bloque_mapa(tono="relleno"),
    ])

    return plantilla(KEY, ("Repostería y tartas personalizadas", "Custom cakes and patisserie"),
                     ("Tartas por encargo con tamaños, raciones y precios, sabores y rellenos con alérgenos, mesas dulces, recogida o entrega y formulario de encargo.",
                      "Cakes to order with sizes, servings and prices, flavours and fillings with allergens, dessert tables, pick-up or delivery and an order form."),
                     "comida", ("reposteria tartas tarta personalizada tartas por encargo tartas de cumpleaños pasteleria obrador cupcakes galletas decoradas mesa dulce bodas tarta de boda sin gluten",
                                "patisserie custom cakes birthday cakes cakes to order wedding cake cupcakes decorated cookies dessert table bakery gluten free"),
                     look, cabecera(nav, cta_cab),
                     pie(nav, ("Tartas personalizadas, mesas dulces y repostería por encargo.",
                               "Custom cakes, dessert tables and made-to-order patisserie."),
                         ("Martes a viernes 10:00 a 14:00 y 16:30 a 19:30. Sábado 9:00 a 14:00.",
                          "Tuesday to Friday 10 am to 2 pm and 4:30 pm to 7:30 pm. Saturday 9 am to 2 pm.")),
                     [inicio, tartas, enc, cont])
