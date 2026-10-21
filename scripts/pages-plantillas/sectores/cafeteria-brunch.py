# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
from lib import *

KEY = "cafeteria-brunch"

# Paleta propia: azul cielo empolvado con marrón de café tostado. Pasa AA;
# primary/inverse no llega a 3, así que no se usa `tono="oscuro"`.
CIELO = dict(
    colores=dict(background="#cfe3f5", surface="#bfd8ee", text="#1c1710", muted="#473a2f", primary="#6a2f19",
                 on_primary="#ffffff", border="#a8c5de", inverse="#2a1a12", on_inverse="#e8f2fb"),
    fuentes=("lora", "dm-sans"), peso=600, radio=(8, 14, 20), ancho=1160)

NOTA_PRECIOS = ("Precios de ejemplo: cámbialos por los tuyos. Incluyen IVA.",
                "Example prices: replace them with your own. VAT included.")
CAB = [("Producto", "Item"), ("Precio", "Price")]


def plato(nombre, detalle, precio):
    return ((f"{nombre[0]}. {detalle[0]}", f"{nombre[1]}. {detalle[1]}"), precio)


def sec_form(titulo, intro, campos, envio, exito, anchor, tono="base"):
    return seccion("contact", [titular(titulo, intro),
                               tarjeta([formulario(campos, envio, exito)], estilo="relleno" if tono == "base" else "borde",
                                       pad=28, tag="div", sz=tam(max_width=640))],
                   tono=tono, anchor=anchor, gap=28)


def build():
    nueva(KEY)
    look = CIELO
    nav = [("inicio", ("Inicio", "Home")), ("carta", ("Café y brunch", "Coffee and brunch")),
           ("local", ("El local y las reservas", "The café and bookings")), ("contacto", ("Contacto", "Contact"))]
    horas = [(("Lunes a viernes", "Monday to Friday"), ("8:00 a 20:00", "8:00 am to 8:00 pm")),
             (("Sábado y domingo", "Saturday and Sunday"), ("9:00 a 18:00", "9:00 am to 6:00 pm"))]
    cta_cab = marco([llamar(), whatsapp("ghost")], dir="row", gap=8, wrap=True, align="center")
    a_carta = a_pagina("carta")
    a_reserva = a_pagina("local", "reserva")

    inicio = pagina("inicio", ("", ""), ("Cafetería de especialidad y brunch", "Specialty café and brunch"),
                    ("Cafetería de especialidad y brunch: café por métodos, desayunos y brunch de carta, repostería, leches vegetales, terraza y reservas de grupo.",
                     "Specialty café and brunch: coffee by brew method, breakfast and brunch menu, baked goods, plant milks, terrace and group bookings."), [
        hero(("Buen café, y algo rico para acompañarlo", "Good coffee, and something tasty to go with it"),
             ("Café de especialidad preparado por métodos, desayunos y brunch de carta, repostería y leches vegetales. Para quedarte un rato o para llevar.",
              "Specialty coffee prepared by different methods, breakfast and brunch from the menu, baked goods and plant milks. To stay a while or to take away."),
             [boton(("Ver la carta", "See the menu"), a_carta), boton(("Reservar mesa", "Book a table"), a_reserva, "secondary"),
              whatsapp("ghost")],
             eyebrow=("Café y brunch", "Coffee and brunch"),
             imagen_=("photo", ("Taza de café con leche sobre la barra de la cafetería", "Cup of milky coffee on the café counter"))),
        bloque_valores(("Qué tomar a cada hora", "What to have at each time of day"), [
            (("Mañana", "Morning"),
             ("Café, tostadas, bollería y zumo natural para empezar el día sin prisa.",
              "Coffee, toast, pastries and fresh juice to start the day without rushing.")),
            (("Brunch", "Brunch"),
             ("Platos de media mañana y mediodía, con huevos, pan, verdura y fruta. Con reserva si venís en grupo.",
              "Late-morning and midday plates with eggs, bread, vegetables and fruit. Book if you come as a group.")),
            (("Tarde", "Afternoon"),
             ("Café, té y repostería casera, con mesas tranquilas para leer o trabajar.",
              "Coffee, tea and home-baked cakes, with quiet tables for reading or working.")),
        ], estilo="relleno", eyebrow=("El día en la cafetería", "The day at the café"), anchor="franjas"),
        bloque_texto(("El café, explicado sin misterio", "Coffee, explained without the mystery"), [
            ("Tostamos poco y preparamos cada taza según el método: espresso para algo intenso, filtrado para algo suave y limpio. Si no sabes cuál elegir, dinos qué sueles tomar y te proponemos uno.",
             "We roast lightly and prepare each cup according to the method: espresso for something intense, filter for something soft and clean. If you do not know which to choose, tell us what you usually drink and we suggest one."),
            ("Cada semana hay uno o dos granos distintos. Los tienes anotados en la carta, con sus notas de sabor.",
             "Each week there are one or two different beans. They are listed on the menu, with their flavour notes.")],
            eyebrow=("Café de especialidad", "Specialty coffee"), tono="base", invertir=True, anchor="cafe",
            imagen_=("product", ("Granos de café y un filtrado preparándose", "Coffee beans and a filter brew being made"))),
        bloque_galeria(("Del desayuno al brunch", "From breakfast to brunch"), [
            ("Tostada con aguacate y huevo en un plato", "Avocado and egg toast on a plate"),
            ("Café filtrado servido en una jarra", "Filter coffee served in a carafe"),
            ("Porción de tarta casera con una taza", "Slice of home-baked cake with a cup"),
            ("Bol de yogur con fruta y granola", "Yoghurt bowl with fruit and granola"),
            ("Flat white con dibujo en la espuma", "Flat white with latte art"),
            ("Mesa de brunch para compartir", "Brunch table to share")], tipo="product", tono="relleno"),
        bloque_checks(("Leche vegetal sin recargos raros", "Plant milks, no surprise charges"), [
            ("Avena, soja, almendra y coco para cualquier café.", "Oat, soy, almond and coconut for any coffee."),
            ("Leche sin lactosa, si la prefieres.", "Lactose-free milk, if you prefer it."),
            ("El suplemento está en la carta, junto a los precios.", "The extra charge is on the menu, next to the prices."),
            ("Si tienes alergia, pregúntanos antes de pedir: las leches se preparan en la misma barra.",
             "If you have an allergy, ask us before you order: the milks are all prepared at the same counter."),
        ], tono="base", cols=2, anchor="leches"),
        bloque_opiniones(("Lo que dicen quienes vienen", "What our customers say"),
                         [("el café", "the coffee"), ("el brunch", "the brunch"), ("el ambiente", "the atmosphere")],
                         tono="relleno"),
        bloque_cta(("¿Te guardamos mesa?", "Shall we keep you a table?"),
                   ("Para dos personas no hace falta. Para un grupo o un brunch en fin de semana, escríbenos con el día, la hora y cuántos sois.",
                    "You do not need to book for two. For a group or a weekend brunch, message us with the day, the time and how many of you there are."),
                   [boton(("Reservar mesa", "Book a table"), a_reserva), llamar("secondary"), whatsapp("ghost")], tono="base"),
    ])

    carta = pagina("carta", ("cafe-y-brunch", "coffee-and-brunch"), ("Café y brunch", "Coffee and brunch"),
                   ("Carta con precios de ejemplo: café por métodos, granos de la semana, desayunos y brunch, repostería, leches vegetales y alérgenos.",
                    "Menu with example prices: coffee by method, beans of the week, breakfast and brunch, baked goods, plant milks and allergens."), [
        hero(("Café y brunch", "Coffee and brunch"),
             ("Todo lo que servimos, con su precio. Si tienes una alergia o una dieta concreta, pregunta en la barra: te ayudamos a elegir.",
              "Everything we serve, with its price. If you have an allergy or a particular diet, ask at the counter: we help you choose."),
             [boton(("Reservar mesa", "Book a table"), a_reserva), llamar("secondary"), whatsapp("ghost")], variante="texto"),
        bloque_carta(("Para beber", "To drink"), [
            (("Café", "Coffee"), ("Precios del café", "Coffee prices"), CAB, [
                plato(("Espresso", "Espresso"), ("Corto e intenso, a presión.", "Short and intense, made under pressure."), eur(1.6)),
                plato(("Cortado", "Cortado"), ("Espresso con un poco de leche.", "Espresso with a little milk."), eur(1.9)),
                plato(("Flat white", "Flat white"), ("Doble espresso con leche cremosa.", "Double espresso with creamy milk."), eur(3.2)),
                plato(("Capuchino", "Cappuccino"), ("Espresso, leche y espuma.", "Espresso, milk and foam."), eur(2.8)),
                plato(("Filtrado V60", "V60 filter"), ("Suave y limpio, preparado a mano.", "Soft and clean, brewed by hand."), eur(3.5)),
                plato(("Cold brew", "Cold brew"), ("Café en frío durante horas, servido con hielo.", "Coffee steeped cold for hours, served over ice."), eur(3.8))]),
            (("Otras bebidas", "Other drinks"), ("Precios de otras bebidas", "Other drink prices"), CAB, [
                (("Té o infusión", "Tea or herbal tea"), eur(2.6)), (("Chocolate caliente", "Hot chocolate"), eur(3)),
                (("Zumo natural de naranja", "Fresh orange juice"), eur(3.5)), (("Agua", "Water"), eur(1.6))]),
        ], nota=NOTA_PRECIOS, tono="base", anchor="beber"),
        bloque_precios(("Granos de la semana", "Beans of the week"), ("Granos de la semana", "Beans of the week"),
                       [("Grano", "Bean"), ("Notas de sabor", "Flavour notes"), ("Mejor en", "Best as")],
                       [(("Grano 1 (natural)", "Bean 1 (natural)"), ("Frutos rojos y floral", "Red fruit and floral"), ("Filtrado", "Filter")),
                        (("Grano 2 (lavado)", "Bean 2 (washed)"), ("Cacao y avellana", "Cocoa and hazelnut"), ("Espresso y flat white", "Espresso and flat white")),
                        (("Descafeinado", "Decaf"), ("Chocolate con leche", "Milk chocolate"), ("Espresso y cortado", "Espresso and cortado"))],
                       nota=("Granos y notas de ejemplo: cámbialos por los tuyos cada semana.",
                             "Example beans and notes: replace them with your own each week."),
                       tono="relleno", anchor="granos"),
        bloque_carta(("Para comer", "To eat"), [
            (("Desayunos", "Breakfast"), ("Precios de los desayunos", "Breakfast prices"), CAB, [
                plato(("Tostada con tomate y aceite", "Toast with tomato and olive oil"), ("Pan de masa madre.", "Sourdough bread."), eur(3.2)),
                plato(("Tostada de aguacate y huevo", "Avocado and egg toast"), ("Con semillas y limón.", "With seeds and lemon."), eur(7.5)),
                plato(("Bol de yogur", "Yoghurt bowl"), ("Con fruta de temporada y granola.", "With seasonal fruit and granola."), eur(5.8)),
                plato(("Croissant", "Croissant"), ("Solo, o con jamón y queso.", "Plain, or with ham and cheese."), eur(2.2))]),
            (("Brunch", "Brunch"), ("Precios del brunch", "Brunch prices"), CAB, [
                plato(("Huevos al plato", "Baked eggs"), ("Con tomate, pimientos y pan.", "With tomato, peppers and bread."), eur(9.5)),
                plato(("Tortitas", "Pancakes"), ("Con fruta y sirope.", "With fruit and syrup."), eur(8.9)),
                plato(("Sándwich de pollo y rúcula", "Chicken and rocket sandwich"), ("Con ensalada pequeña.", "With a small salad."), eur(9.2)),
                plato(("Plato vegetariano", "Vegetarian plate"), ("Verduras asadas, hummus y huevo.", "Roasted vegetables, hummus and egg."), eur(9.8))]),
            (("Repostería", "Baked goods"), ("Precios de la repostería", "Baked goods prices"), CAB, [
                (("Porción de tarta del día", "Slice of cake of the day"), eur(4.5)), (("Brownie", "Brownie"), eur(3.6)),
                (("Galleta de mantequilla", "Butter cookie"), eur(2.2)), (("Bizcocho de temporada", "Seasonal sponge cake"), eur(3.8))]),
        ], nota=NOTA_PRECIOS, tono="base", anchor="comer"),
        bloque_precios(("Leches vegetales y alérgenos", "Plant milks and allergens"), ("Leches vegetales: suplemento y alérgeno", "Plant milks: extra charge and allergen"),
                       [("Leche", "Milk"), ("Suplemento", "Extra charge"), ("Contiene", "Contains")],
                       [(("Avena", "Oat"), eur(0.4), ("Gluten", "Gluten")), (("Soja", "Soy"), eur(0.4), ("Soja", "Soy")),
                        (("Almendra", "Almond"), eur(0.5), ("Frutos de cáscara", "Tree nuts")), (("Coco", "Coconut"), eur(0.5), ("Coco", "Coconut")),
                        (("Sin lactosa", "Lactose-free"), eur(0.3), ("Lácteos", "Dairy"))],
                       nota=("Suplementos de ejemplo: cámbialos por los tuyos. En la cocina hay gluten, huevo, lácteos y frutos secos: pregúntanos antes de pedir si tienes alergia o intolerancia.",
                             "Example charges: replace them with your own. The kitchen handles gluten, egg, dairy and nuts: ask us before you order if you have an allergy or intolerance."),
                       tono="relleno", anchor="alergenos"),
        bloque_cta(("¿Vienes a probarlo?", "Coming to try it?"), None, tono="base"),
    ])

    local = pagina("local", ("el-local", "the-cafe"), ("El local y las reservas", "The café and bookings"),
                   ("Terraza, wifi y ordenadores, horario por franjas, reservas de grupo con formulario y preguntas frecuentes.",
                    "Terrace, wifi and laptops, opening hours by time of day, group bookings with a form and frequently asked questions."), [
        hero(("El local, la terraza y las reservas", "The café, the terrace and bookings"),
             ("Sitio para un café rápido, para trabajar un rato o para un brunch con amigos. Para grupos, reserva mesa.",
              "A place for a quick coffee, a spell of work or a brunch with friends. For groups, book a table."),
             [boton(("Reservar mesa", "Book a table"), a_pagina("local", "reserva")), llamar("secondary"), whatsapp("ghost")],
             variante="texto", tono="relleno"),
        bloque_texto(("La terraza", "The terrace"), [
            ("Mesas fuera para cuando acompaña el tiempo. Se ocupan por orden de llegada; solo reservamos terraza para grupos.",
             "Tables outside for when the weather allows. They are taken in order of arrival; we only book the terrace for groups."),
            ("Se admiten perros bien educados en la terraza. Pregúntanos si necesitas un cuenco de agua.",
             "Well-behaved dogs are welcome on the terrace. Ask us if you need a bowl of water.")],
            eyebrow=("Al aire libre", "Outdoors"), tono="base", anchor="terraza",
            imagen_=("landscape", ("Terraza de la cafetería con mesas pequeñas y sillas", "Café terrace with small tables and chairs"))),
        bloque_checks(("Wifi y ordenadores", "Wifi and laptops"), [
            ("Wifi para clientes: pide la clave en la barra.", "Wifi for customers: ask for the password at the counter."),
            ("Hay enchufes en algunas mesas de la sala.", "There are sockets at some tables in the main room."),
            ("Puedes trabajar con tu ordenador. En las horas del brunch pedimos que las mesas grandes se dejen para grupos.",
             "You are welcome to work on your laptop. During brunch hours we ask that the big tables are left free for groups."),
            ("Un café y ya estás en tu mesa: no hay consumo mínimo por horas.", "Order a coffee and the table is yours: there is no minimum spend per hour."),
        ], tono="relleno", cols=2, anchor="trabajar"),
        bloque_precios(("Horario por franjas", "Hours by time of day"), ("Franjas del día y qué servimos", "Times of day and what we serve"),
                       [("Franja", "Time of day"), ("Horario", "Hours"), ("Qué servimos", "What we serve")],
                       [(("Desayunos", "Breakfast"), ("8:00 a 11:30", "8:00 am to 11:30 am"), ("Café, tostadas y bollería", "Coffee, toast and pastries")),
                        (("Brunch", "Brunch"), ("10:00 a 15:00", "10:00 am to 3:00 pm"), ("Platos de brunch y café", "Brunch plates and coffee")),
                        (("Tarde", "Afternoon"), ("15:00 a cierre", "3:00 pm to closing"), ("Café, té y repostería", "Coffee, tea and baked goods"))],
                       nota=("Franjas de ejemplo: ajústalas a tu horario real.", "Example time slots: adjust them to your real hours."),
                       tono="base", anchor="franjas-horario"),
        sec_form(("Reserva para un grupo", "Group booking"),
                 ("Para seis personas o más. Dinos el día, la hora y cuántos sois, y te confirmamos si hay sitio.",
                  "For six people or more. Tell us the day, the time and how many of you there are, and we confirm whether we have space."),
                 [campo("nombre", "text", ("Nombre", "Name"), True, auto="name", maxlen=120),
                  campo("telefono", "tel", ("Teléfono", "Phone"), True, auto="tel", maxlen=30),
                  campo("fecha", "date", ("Día", "Day"), True),
                  campo("franja", "select", ("Franja", "Time of day"), True,
                        opciones=[("Desayuno", "Breakfast"), ("Brunch", "Brunch"), ("Tarde", "Afternoon")]),
                  campo("personas", "select", ("Número de personas", "Number of people"), True,
                        opciones=[("6 a 8", "6 to 8"), ("9 a 12", "9 to 12"), ("Más de 12", "More than 12")]),
                  campo("zona", "select", ("Dónde os apetece", "Where you would like to sit"), False,
                        opciones=[("Sala", "Inside"), ("Terraza", "Terrace"), ("Me da igual", "No preference")]),
                  campo("mensaje", "textarea", ("Alergias, cumpleaños u otros detalles", "Allergies, birthdays or other details"), False, maxlen=1000)],
                 ("Pedir reserva", "Request booking"),
                 ("Gracias. Te confirmamos la reserva por teléfono o WhatsApp lo antes posible.",
                  "Thank you. We confirm your booking by phone or WhatsApp as soon as possible."),
                 "reserva", tono="relleno"),
        bloque_faq(("Preguntas frecuentes", "Frequently asked questions"), [
            (("¿Hace falta reservar?", "Do I need to book?"),
             ("Para uno o dos, no: te sentamos según llegues. Para seis o más, sí, sobre todo en el brunch del fin de semana.",
              "Not for one or two: we seat you as you arrive. For six or more, yes, especially for weekend brunch.")),
            (("¿Tenéis opciones sin gluten o veganas?", "Do you have gluten-free or vegan options?"),
             ("Tenemos leches vegetales y algún plato sin carne ni huevo. En la cocina hay gluten, así que no podemos asegurar que no haya trazas.",
              "We have plant milks and some dishes without meat or egg. There is gluten in the kitchen, so we cannot promise there are no traces.")),
            (("¿Puedo llevarme el café?", "Can I take my coffee away?"),
             ("Sí, en vaso para llevar. Si traes tu propio vaso, también te lo llenamos.",
              "Yes, in a takeaway cup. If you bring your own cup, we fill that too.")),
            (("¿Se puede pagar con tarjeta?", "Can I pay by card?"),
             ("Sí, con tarjeta o en efectivo, también para importes pequeños.", "Yes, by card or cash, including small amounts.")),
        ], tono="base"),
    ])

    cont = pagina("contacto", ("contacto", "contact"), ("Contacto y horario", "Contact and opening hours"),
                  ("Llama o escribe por WhatsApp, consulta el horario y mira cómo llegar a la cafetería.",
                   "Call or message on WhatsApp, check opening hours and see how to get to the café."), [
        hero(("Pasa a tomar un café", "Come in for a coffee"),
             ("Para preguntar por el café, por el brunch o por una reserva de grupo.",
              "To ask about the coffee, the brunch or a group booking."),
             [llamar(), whatsapp()], variante="texto"),
        bloque_contacto(("Cómo encontrarnos", "How to find us"),
                        ("Estamos a pie de calle y con terraza. Se puede venir con carrito de bebé.",
                         "We are at street level with a terrace. Pushchairs are welcome."),
                        campos_consulta(servicios=[("Reserva de grupo", "Group booking"), ("Alergias e intolerancias", "Allergies and intolerances"),
                                                   ("Otra consulta", "Other question")]),
                        ("Enviar mensaje", "Send message"), horas,
                        exito=("Gracias. Te respondemos lo antes posible.", "Thank you. We will reply as soon as possible.")),
        bloque_mapa(tono="relleno"),
    ])

    return plantilla(KEY, ("Cafetería de especialidad y brunch", "Specialty café and brunch"),
                     ("Café por métodos, granos de la semana, desayunos y brunch con precios, leches vegetales, terraza, reservas de grupo y horario por franjas.",
                      "Coffee by method, beans of the week, breakfast and brunch with prices, plant milks, terrace, group bookings and hours by time of day."),
                     "comida", ("cafeteria cafe cafeteria de especialidad brunch desayunos cafe de especialidad tostadas reposteria leche de avena terraza wifi trabajar coworking cafe",
                                "specialty coffee shop cafe brunch breakfast flat white filter coffee oat milk terrace wifi laptop friendly"),
                     look, cabecera(nav, cta_cab),
                     pie(nav, ("Café de especialidad, desayunos y brunch, con terraza y reservas para grupos.",
                               "Specialty coffee, breakfast and brunch, with a terrace and group bookings."),
                         ("Lunes a viernes 8:00 a 20:00. Sábado y domingo 9:00 a 18:00.",
                          "Monday to Friday 8 am to 8 pm. Saturday and Sunday 9 am to 6 pm.")),
                     [inicio, carta, local, cont])
