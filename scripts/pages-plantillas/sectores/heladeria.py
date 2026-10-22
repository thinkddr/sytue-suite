# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
from lib import *

KEY = "heladeria"

# Blanco de menta helada con frambuesa: fresco, claro y alegre, con esquinas muy redondeadas.
SORBETE = dict(
    colores=dict(background="#f3fbf6", surface="#dff3e8", text="#2a1a24", muted="#6a4f5f", primary="#b8245c",
                 on_primary="#ffffff", border="#c4e4d3", inverse="#3a1530", on_inverse="#fdeef5"),
    fuentes=("manrope", "dm-sans"), peso=800, h1=(54, 42, 34), radio=(14, 22, 32), ancho=1120)

NOTA = ("Precios de ejemplo: cámbialos por los tuyos. Incluyen IVA.", "Example prices: replace them with your own. VAT included.")
TAMANO = [("Tamaño", "Size"), ("Qué lleva", "What you get"), ("Precio", "Price")]


def sec_form(titulo, intro, campos, envio, exito, anchor, tono="base"):
    return seccion("contact", [titular(titulo, intro),
                               tarjeta([formulario(campos, envio, exito)], estilo="relleno" if tono == "base" else "borde",
                                       pad=28, tag="div", sz=tam(max_width=680))],
                   tono=tono, anchor=anchor, gap=28)


def build():
    nueva(KEY)
    look = SORBETE
    nav = [("inicio", ("Inicio", "Home")), ("sabores", ("Sabores y precios", "Flavours and prices")),
           ("tartas", ("Tartas y encargos", "Cakes and orders")), ("contacto", ("Contacto", "Contact"))]
    horas = [(("Verano (junio a septiembre)", "Summer (June to September)"), ("Todos los días de 12:00 a 24:00", "Every day, 12:00 pm to midnight")),
             (("Primavera y otoño", "Spring and autumn"), ("De lunes a viernes de 13:00 a 21:00. Fines de semana de 12:00 a 22:00", "Monday to Friday 1:00 pm to 9:00 pm. Weekends 12:00 pm to 10:00 pm")),
             (("Invierno", "Winter"), ("Solo sábados, domingos y festivos de 14:00 a 20:00", "Saturdays, Sundays and public holidays only, 2:00 pm to 8:00 pm"))]
    cta_cab = marco([llamar(), whatsapp("ghost")], dir="row", gap=8, wrap=True, align="center")

    sabores_tarjetas = bloque_valores(("Sabores de esta temporada", "This season's flavours"), [
        (("De crema", "Creamy"), ("Vainilla, chocolate, avellana, stracciatella y dulce de leche.", "Vanilla, chocolate, hazelnut, stracciatella and dulce de leche.")),
        (("De fruta", "Fruity"), ("Limón, fresa, mango y frambuesa, hechos con fruta y sin leche.", "Lemon, strawberry, mango and raspberry, made with fruit and no milk.")),
        (("Del mes", "Of the month"), ("Cada mes uno distinto: higo, melón, calabaza, turrón... según lo que dé la huerta.", "A different one each month: fig, melon, pumpkin, nougat... whatever the market gardens give.")),
    ], intro=("Sabores de ejemplo: cámbialos por los tuyos cada temporada. Si un sabor se acaba, se tacha y listo.",
              "Example flavours: change them to your own each season. If one runs out, cross it off and that is it."),
        tono="relleno", estilo="borde", cols=3, anchor="sabores-temporada")
    sabores_tarjetas["children"].append(boton(("Ver todos los sabores y precios", "See all flavours and prices"), a_pagina("sabores"), "secondary"))

    inicio = pagina("inicio", ("", ""), ("Heladería artesana", "Artisan ice cream shop"),
                    ("Heladería artesana: sabores de temporada, tamaños y precios, tartas heladas por encargo, terraza y horario por temporada.",
                     "Artisan ice cream shop: seasonal flavours, sizes and prices, ice cream cakes to order, a terrace and seasonal opening hours."), [
        hero(("Helado hecho cada día, bola a bola", "Ice cream made every day, scoop by scoop"),
             ("Cucuruchos, tarrinas y tarros para llevar. Sabores de crema y de fruta que cambian con la temporada, y tartas heladas para cumpleaños.",
              "Cones, tubs and jars to take away. Creamy and fruity flavours that change with the season, and ice cream cakes for birthdays."),
             [boton(("Ver sabores y precios", "See flavours and prices"), a_pagina("sabores")), llamar("secondary"), whatsapp("ghost")],
             eyebrow=("Heladería artesana", "Artisan ice cream shop"), variante="centro", tono="relleno"),
        sabores_tarjetas,
        bloque_texto(("Terraza y tarde de helado", "A terrace for ice cream afternoons"),
                     [("Tenemos mesas fuera para tomarlo sin prisa. Se puede venir con niños, con el perro atado y con ganas de decidir con calma.",
                      "We have tables outside to eat without rushing. Children are welcome, so are dogs on a lead, and so is taking your time to choose.")],
                     eyebrow=("Para quedarse", "Stay a while"), tono="base", anchor="terraza",
                     imagen_=("photo", ("Terraza de una heladería con mesas y sombrillas al sol", "Ice cream shop terrace with tables and parasols in the sun"))),
        bloque_checks(("Para quien tiene que mirar la etiqueta", "For anyone who needs to check the label"), [
            ("Los sorbetes se hacen con fruta y agua, sin leche ni huevo.", "Sorbets are made with fruit and water, with no milk or egg."),
            ("Hay cucuruchos sin gluten: pídelo y lo servimos aparte.", "There are gluten-free cones: ask and we serve it separately."),
            ("Compartimos máquinas y cucharas, así que no podemos descartar trazas: si tu alergia es grave, pregúntanos antes.", "We share machines and scoops, so we cannot rule out traces: if your allergy is serious, ask us first."),
            ("Tenemos a mano la lista de alérgenos de cada sabor.", "We keep an allergen list for every flavour at hand."),
        ], cols=2, tono="relleno", anchor="aviso-alergenos"),
        bloque_opiniones(("Lo que dicen de nuestros helados", "What people say about our ice cream"),
                         [("los sabores", "the flavours"), ("las tartas", "the cakes"), ("la terraza", "the terrace")], tono="base"),
        bloque_cta(("¿Una tarta helada para el cumpleaños?", "An ice cream cake for the birthday?"),
                   ("Se encarga con unos días de antelación. Mira cómo hacerlo.", "Order it a few days ahead. See how it works."),
                   [boton(("Encargar una tarta", "Order a cake"), a_pagina("tartas")), whatsapp("secondary")], tono="relleno"),
    ])

    sabores = pagina("sabores", ("sabores", "flavours"), ("Sabores y precios", "Flavours and prices"),
                     ("Sabores por temporada, tamaños de cucurucho, tarrina y tarro con precios, sin lactosa, sin gluten y preguntas frecuentes.",
                      "Flavours by season, cone, tub and jar sizes with prices, lactose-free, gluten-free and frequently asked questions."), [
        hero(("Sabores y precios", "Flavours and prices"),
             ("Lo que hay hoy, lo que cambia con la estación y cuánto cuesta cada tamaño. Si dudas entre dos sabores, te dejamos probar.",
              "What is on today, what changes with the seasons and what each size costs. If you cannot decide between two flavours, we let you taste."),
             [llamar(), whatsapp("secondary")], variante="texto"),
        bloque_carta(("Los sabores", "The flavours"), [
            (("Todo el año", "All year round"), ("Sabores fijos", "Year-round flavours"), [("Sabor", "Flavour"), ("Tipo", "Type")],
             [(("Vainilla, chocolate y nata", "Vanilla, chocolate and cream"), ("Con leche", "With milk")),
              (("Avellana y pistacho", "Hazelnut and pistachio"), ("Con leche y frutos de cáscara", "With milk and tree nuts")),
              (("Limón y mandarina", "Lemon and mandarin"), ("Sorbete, sin leche", "Sorbet, no milk")),
              (("Fresa y frambuesa", "Strawberry and raspberry"), ("Sorbete, sin leche", "Sorbet, no milk"))]),
            (("Según la estación", "By season"), ("Sabores de temporada", "Seasonal flavours"), [("Estación", "Season"), ("Sabores", "Flavours")],
             [(("Primavera", "Spring"), ("Fresa con nata, yogur y flor de azahar", "Strawberries and cream, yoghurt and orange blossom")),
              (("Verano", "Summer"), ("Mango, melón, sandía y horchata", "Mango, melon, watermelon and horchata")),
              (("Otoño", "Autumn"), ("Higo, calabaza, castaña y galleta", "Fig, pumpkin, chestnut and biscuit")),
              (("Invierno", "Winter"), ("Turrón, chocolate con naranja y café", "Nougat, chocolate with orange and coffee"))]),
        ], nota=("Sabores de ejemplo: cámbialos por los tuyos. Cuando un sabor se acabe, quítalo de la lista.",
                 "Example flavours: change them to your own. When one runs out, take it off the list."),
            tono="base", anchor="sabores",
            intro=("Hacemos el helado a diario en pequeñas tandas. Por eso no siempre hay todos los sabores a última hora.",
                   "We make ice cream daily in small batches. That is why not every flavour is always there late in the day.")),
        bloque_precios(("Tamaños y precios", "Sizes and prices"), ("Tamaños y precios del helado", "Ice cream sizes and prices"), TAMANO,
                       [(("Cucurucho pequeño", "Small cone"), ("1 bola", "1 scoop"), eur(2.2)),
                        (("Cucurucho mediano", "Medium cone"), ("2 bolas", "2 scoops"), eur(3.5)),
                        (("Cucurucho grande", "Large cone"), ("3 bolas", "3 scoops"), eur(4.5)),
                        (("Tarrina", "Tub"), ("2 sabores a elegir", "2 flavours of your choice"), eur(4)),
                        (("Tarro de 500 ml para llevar", "500 ml jar to take away"), ("2 o 3 sabores", "2 or 3 flavours"), eur(6.5)),
                        (("Tarro de 1 litro para llevar", "1 litre jar to take away"), ("3 o 4 sabores", "3 or 4 flavours"), eur(11)),
                        (("Copa de la casa", "House sundae"), ("3 bolas, nata y topping", "3 scoops, cream and topping"), eur(5.5))],
                       nota=NOTA, tono="relleno", anchor="precios"),
        bloque_checks(("Sin lactosa y sin gluten", "Lactose-free and gluten-free"), [
            ("Los sorbetes de fruta se hacen sin leche. Los de crema llevan leche.", "Fruit sorbets are made without milk. The creamy ones contain milk."),
            ("Los cucuruchos sin gluten están en un envase aparte. Pídelo al llegar.", "Gluten-free cones are kept in separate packaging. Ask when you arrive."),
            ("Las bolas se sirven con la misma cuchara que se limpia entre sabor y sabor, pero compartimos máquinas, así que puede haber trazas.", "Scoops are served with a scoop that is rinsed between flavours, but we share machines, so traces are possible."),
            ("Si tienes una alergia o una intolerancia, díselo a quien te atienda y te enseña la lista de cada sabor.", "If you have an allergy or intolerance, tell whoever serves you and they will show you the list for each flavour."),
        ], cols=1, tono="base", anchor="sin-lactosa"),
        bloque_faq(("Preguntas frecuentes", "Frequently asked questions"), [
            (("¿Puedo probar antes de elegir?", "Can I taste before I choose?"),
             ("Claro: pide una cucharita de lo que dudes. Mejor que salir con el sabor equivocado.", "Of course: ask for a little spoon of whatever you are unsure about. Better than leaving with the wrong flavour.")),
            (("¿Cuánto aguanta un tarro en casa?", "How long does a jar last at home?"),
             ("En el congelador, bien cerrado, unas dos semanas. Mejor tomarlo pronto: el helado artesano pierde textura con el tiempo.",
              "In the freezer, tightly closed, about two weeks. Best eaten soon: artisan ice cream loses texture over time.")),
            (("¿Puedo llevar helado para una comida?", "Can I take ice cream for a meal?"),
             ("Sí, hacemos tarros y bandejas. Si son muchos, avísanos un día antes.", "Yes, we make jars and trays. If you need a lot, tell us a day ahead.")),
            (("¿Admitís tarjeta?", "Do you take cards?"),
             ("Sí, con tarjeta y en efectivo. Formas de pago de ejemplo: cámbialas por las que aceptes.", "Yes, by card and in cash. Example payment methods: change them to the ones you accept.")),
        ], tono="relleno"),
        bloque_cta(("Ven a probar", "Come and taste"), None, [llamar(), whatsapp("ghost")], tono="base"),
    ])

    tartas = pagina("tartas", ("tartas", "cakes"), ("Tartas heladas y encargos", "Ice cream cakes and orders"),
                    ("Tartas heladas y tarros por encargo con tamaños y precios, cómo encargar en pasos y formulario con fecha de recogida.",
                     "Ice cream cakes and jars to order with sizes and prices, how to order in steps and a form with a collection date."), [
        hero(("Tartas heladas por encargo", "Ice cream cakes to order"),
             ("Para cumpleaños, comuniones o una comida en casa. Eliges sabores y tamaño, la preparamos y pasas a recogerla.",
              "For birthdays, communions or a meal at home. You choose flavours and size, we prepare it and you collect it."),
             [boton(("Encargar una tarta", "Order a cake"), a_ancla("encargar")), whatsapp("secondary")], variante="texto", tono="relleno"),
        bloque_precios(("Tamaños y precios de las tartas", "Cake sizes and prices"), ("Tartas heladas, tamaños y precios", "Ice cream cakes, sizes and prices"),
                       [("Tarta", "Cake"), ("Para", "Serves"), ("Precio", "Price")],
                       [(("Tarta helada pequeña", "Small ice cream cake"), ("6 a 8 raciones", "6 to 8 portions"), desde(24)),
                        (("Tarta helada mediana", "Medium ice cream cake"), ("10 a 12 raciones", "10 to 12 portions"), desde(34)),
                        (("Tarta helada grande", "Large ice cream cake"), ("16 a 20 raciones", "16 to 20 portions"), desde(48)),
                        (("Bandeja de bolas para fiestas", "Tray of scoops for parties"), ("20 raciones", "20 portions"), desde(40)),
                        (("Letrero o mensaje en la tarta", "Message or lettering on the cake"), ("Opcional", "Optional"), eur(3))],
                       intro=("Cada tarta lleva dos o tres capas de sabores a elegir y una cubierta. El precio de ejemplo cambia según el tamaño y los sabores.",
                              "Each cake has two or three layers of flavours of your choice and a topping. The example price varies by size and flavours."),
                       nota=NOTA, tono="base", anchor="tartas-precios"),
        bloque_pasos(("Cómo encargarla", "How to order"), [
            (("Elige tamaño y sabores", "Pick size and flavours"), ("Dos o tres sabores de la carta y para cuántas personas.", "Two or three flavours from the list and how many people it is for.")),
            (("Dinos la fecha", "Tell us the date"), ("Con un mínimo de tres días de antelación, o cinco en fechas de mucha demanda.", "At least three days ahead, or five at busy times of year.")),
            (("Confirmamos y la preparamos", "We confirm and make it"), ("Te escribimos para confirmar sabores y precio.", "We message you to confirm flavours and price.")),
            (("La recoges y la llevas fría", "Collect it and keep it cold"), ("Te la damos en caja isotérmica: llévala directa a casa y al congelador.", "We hand it over in an insulated box: take it straight home and into the freezer.")),
        ], tono="relleno", anchor="como-encargar"),
        bloque_texto(("Antes de encargar", "Before you order"),
                     [("Los plazos son de ejemplo: cámbialos por los tuyos. Si tu tarta es para alguien con alergias, cuéntanoslo al encargar y te decimos qué sabores encajan; no podemos descartar trazas.",
                      "The lead times are examples: change them to your own. If the cake is for someone with allergies, tell us when you order and we will say which flavours fit; we cannot rule out traces.")],
                     tono="base", anchor="antes-de-encargar"),
        sec_form(("Encarga tu tarta", "Order your cake"),
                 ("Cuéntanos la fecha, para cuántas personas y qué sabores te gustan. Te contestamos con el precio y para confirmar.",
                  "Tell us the date, how many people and which flavours you like. We reply with the price and to confirm."),
                 [campo("nombre", "text", ("Nombre", "Name"), True, auto="name", maxlen=120),
                  campo("telefono", "tel", ("Teléfono", "Phone"), True, auto="tel", maxlen=30),
                  campo("fecha", "date", ("Día de recogida", "Collection day"), True),
                  campo("personas", "text", ("Para cuántas personas", "How many people"), True, maxlen=10),
                  campo("sabores", "text", ("Sabores que te gustan", "Flavours you like"), False, maxlen=200),
                  campo("mensaje", "textarea", ("Mensaje en la tarta y alergias que debamos conocer (opcional)", "Message on the cake and any allergies we should know about (optional)"), False, maxlen=1000)],
                 ("Enviar encargo", "Send order"),
                 ("Gracias. Hemos recibido tu encargo y te escribimos para confirmarlo y darte el precio.",
                  "Thank you. We have received your order and will message you to confirm it and give you the price."),
                 "encargar", tono="relleno"),
    ])

    cont = pagina("contacto", ("contacto", "contact"), ("Contacto, horario y cómo llegar", "Contact, hours and how to get here"),
                  ("Teléfono, WhatsApp, formulario, horario por temporada y mapa de la heladería.",
                   "Phone, WhatsApp, form, seasonal opening hours and a map of the ice cream shop."), [
        hero(("Contacto y horario", "Contact and opening hours"),
             ("Estamos abiertos casi todo el año, pero el horario cambia con la estación. Mira el de ahora antes de venir.",
              "We are open most of the year, but the hours change with the seasons. Check the current ones before you come."),
             [llamar(), whatsapp()], variante="texto"),
        bloque_contacto(("Habla con nosotros", "Talk to us"),
                        ("Para encargar una tarta o preguntar por un sabor, escríbenos o llámanos.",
                         "To order a cake or ask about a flavour, message or call us."),
                        campos_cita(servicios=[("Una tarta o un tarro", "A cake or a jar"), ("Una pregunta sobre sabores", "A question about flavours"),
                                               ("Una celebración o un evento", "A celebration or event"), ("Otra cosa", "Something else")], fecha=False),
                        ("Enviar mensaje", "Send message"), horas,
                        notas=("Horario de ejemplo por temporada: cámbialo por el tuyo.", "Example hours by season: change them to your own.")),
        bloque_mapa(("Cómo llegar", "How to get here"),
                    ("El mapa señala la heladería y su terraza. Si vienes con carrito o en silla de ruedas y quieres saber cómo se accede, llámanos.",
                     "The map shows the shop and its terrace. If you come with a pushchair or a wheelchair and want to know about access, call us."),
                    tono="relleno"),
    ])

    return plantilla(KEY, ("Heladería artesana", "Artisan ice cream shop"),
                     ("Sabores de temporada, tamaños con precios, sin lactosa y sin gluten, tartas heladas por encargo con formulario, terraza y horario por temporada, con botones de llamar y WhatsApp.",
                      "Seasonal flavours, sizes with prices, lactose-free and gluten-free, ice cream cakes to order with a form, a terrace and seasonal hours, with call and WhatsApp buttons."),
                     "comida", ("heladeria helados helado artesano artesana gelateria tartas heladas cucurucho tarrina sorbete sin lactosa terraza helados para llevar",
                                "ice cream shop gelato parlour artisan ice cream ice cream cakes cone tub sorbet lactose free terrace takeaway scoops"),
                     look, cabecera(nav, cta_cab),
                     pie(nav, ("Heladería artesana. Helado hecho cada día y tartas heladas por encargo.",
                               "Artisan ice cream shop. Ice cream made every day and ice cream cakes to order."),
                         ("Verano de 12:00 a 24:00. El resto del año cambia: míralo en Contacto.", "Summer 12 pm to midnight. The rest of the year it changes: see Contact.")),
                     [inicio, sabores, tartas, cont])
