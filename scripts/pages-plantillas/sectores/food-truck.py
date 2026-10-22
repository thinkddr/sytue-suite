# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
from lib import *

KEY = "food-truck"

# Verde azulado oscuro con naranja vivo: pensado para mirar desde el móvil en la calle. Pasa AA y
# primary/inverse >= 3 (la portada usa fondo $inverse).
CALLE_OSCURA = dict(
    colores=dict(background="#0e2a2b", surface="#153a3b", text="#f2f7f5", muted="#a9c4c0", primary="#ff8a4c",
                 on_primary="#0e1f20", border="#2a5254", inverse="#071a1b", on_inverse="#f2f7f5"),
    fuentes=("dm-sans", "work-sans"), peso=800, h1=(50, 40, 32), radio=(6, 12, 18), ancho=1120)

NOTA_PRECIOS = ("Precios de ejemplo: cámbialos por los tuyos. Incluyen IVA.",
                "Example prices: replace them with your own. VAT included.")
PLATO = [("Plato", "Dish"), ("Precio", "Price")]
NOTA_SEMANA = ("Ubicaciones y días de ejemplo: cámbialos cada semana por los tuyos. Si cambias de sitio a última hora, avísalo también en tus redes.",
               "Locations and days are examples: change them every week to your own. If you move at the last minute, say so on your social media too.")
REDES = ("Cada semana avisamos también en nuestras redes: Instagram @tu_usuario (de ejemplo: cámbialo por tu usuario).",
         "Every week we also post on our social media: Instagram @your_username (example: change it to your handle).")


def sec_form(titulo, intro, campos, envio, exito, anchor, tono="base"):
    return seccion("contact", [titular(titulo, intro),
                               tarjeta([formulario(campos, envio, exito)], estilo="relleno" if tono == "base" else "borde",
                                       pad=28, tag="div", sz=tam(max_width=680))],
                   tono=tono, anchor=anchor, gap=28)


def semana(titulo, intro, anchor="esta-semana", tono="base"):
    return seccion("pricing", [
        titular(titulo, intro),
        tabla(("Dónde estamos esta semana", "Where we are this week"),
              [("Día", "Day"), ("Dónde", "Where"), ("Horas", "Hours")],
              [(("Lunes", "Monday"), ("Descanso", "Day off"), ("-", "-")),
               (("Martes", "Tuesday"), ("Plaza Mayor de ejemplo, Tu ciudad", "Example Main Square, Your city"), ("13:00 a 16:00", "1:00 pm to 4:00 pm")),
               (("Miércoles", "Wednesday"), ("Polígono de ejemplo, junto a la rotonda", "Example industrial estate, next to the roundabout"), ("12:30 a 15:30", "12:30 pm to 3:30 pm")),
               (("Jueves", "Thursday"), ("Plaza Mayor de ejemplo, Tu ciudad", "Example Main Square, Your city"), ("13:00 a 16:00", "1:00 pm to 4:00 pm")),
               (("Viernes", "Friday"), ("Parque de ejemplo, Tu ciudad", "Example park, Your city"), ("13:00 a 16:00 y de 20:00 a 23:30", "1:00 pm to 4:00 pm and 8:00 pm to 11:30 pm")),
               (("Sábado", "Saturday"), ("Mercadillo de ejemplo, Tu ciudad", "Example market, Your city"), ("12:00 a 17:00", "12:00 pm to 5:00 pm")),
               (("Domingo", "Sunday"), ("Eventos y catering", "Events and catering"), ("Según el evento", "Depends on the event"))]),
        peq(NOTA_SEMANA, color="$muted"),
        peq(REDES, color="$muted"),
    ], tono=tono, anchor=anchor, gap=24)


def build():
    nueva(KEY)
    look = CALLE_OSCURA
    nav = [("inicio", ("Inicio", "Home")), ("carta", ("Carta", "Menu")),
           ("catering", ("Catering", "Catering")), ("contacto", ("Contacto", "Contact"))]
    horas = [(("Lunes", "Monday"), ("Descanso", "Day off")),
             (("Martes a viernes", "Tuesday to Friday"), ("12:30 a 16:00 (el viernes también de noche)", "12:30 pm to 4:00 pm (on Friday evenings too)")),
             (("Sábado", "Saturday"), ("12:00 a 17:00", "12:00 pm to 5:00 pm")),
             (("Domingo", "Sunday"), ("Solo eventos", "Events only"))]
    cta_cab = marco([llamar(), whatsapp("ghost")], dir="row", gap=8, wrap=True, align="center")
    a_semana = a_ancla("esta-semana")
    pedir_wa = lambda t=("Reservar mi pedido por WhatsApp", "Reserve my order on WhatsApp"): whatsapp("primary", t)

    proxima = tarjeta([
        texto("p", ("Próxima parada", "Next stop"), preset="label", transform="uppercase", color="$primary"),
        texto("h2", ("Plaza Mayor de ejemplo, Tu ciudad", "Example Main Square, Your city")),
        p(("Martes, de 13:00 a 16:00. Es un ejemplo: cámbialo por tu próxima parada.",
           "Tuesday, 1:00 pm to 4:00 pm. This is an example: change it to your next stop.")),
    ], estilo="relleno", tag="div", gap=6, pad=20)

    corta = bloque_valores(("La carta corta", "The short menu"), [
            (("Hamburguesa de la casa · 8,50 €", "House burger · €8.50"), ("Carne de ternera, queso, lechuga, tomate y salsa de la casa, en pan de brioche.", "Beef, cheese, lettuce, tomato and house sauce in a brioche bun.")),
            (("Bocadillo de pollo · 6,50 €", "Chicken sandwich · €6.50"), ("Pollo a la plancha con verduras y salsa suave, en pan crujiente.", "Griddled chicken with vegetables and a mild sauce, in a crusty roll.")),
            (("Tacos de cerdo (3) · 8 €", "Pork tacos (3) · €8"), ("Cerdo desmenuzado, cebolla encurtida y cilantro, en tortilla de maíz.", "Pulled pork, pickled onion and coriander in a corn tortilla.")),
    ], intro=("Precios de ejemplo: cámbialos por los tuyos. La carta cambia poco para que salga rápido.",
              "Example prices: replace them with your own. The menu changes little so it comes out fast."),
        tono="relleno", estilo="borde", anchor="carta-corta")
    corta["children"].append(boton(("Ver la carta completa", "See the full menu"), a_pagina("carta"), "secondary"))

    inicio = pagina("inicio", ("", ""), ("Food truck", "Food truck"),
                    ("Food truck: dónde estamos esta semana, carta corta con precios, pedidos por WhatsApp y catering para eventos.",
                     "Food truck: where we are this week, a short menu with prices, orders by WhatsApp and event catering."), [
        hero(("Comida de la calle, hecha en el momento", "Street food, made to order"),
             ("Un camión, una carta corta y cada día en un sitio. Mira dónde aparcamos esta semana y reserva tu pedido por WhatsApp para no esperar.",
              "One truck, a short menu and a different spot each day. See where we park this week and reserve your order on WhatsApp so you do not have to wait."),
             [boton(("Dónde estamos", "Where we are"), a_semana), whatsapp("secondary"), llamar("ghost")],
             eyebrow=("Food truck", "Food truck"), variante="texto", tono="oscuro", extra=proxima),
        semana(("Dónde estamos esta semana", "Where we are this week"),
               ("Cambiamos de sitio según el día. Esta es nuestra semana; si algo cambia, lo ponemos aquí.",
                "We move around depending on the day. This is our week; if anything changes, we update it here."),
               tono="base"),
        corta,
        bloque_pasos(("Reserva tu pedido y recógelo sin cola", "Reserve your order and collect it with no queue"), [
            (("Escríbenos", "Message us"), ("Por WhatsApp, dinos qué quieres y dónde nos vas a encontrar.", "On WhatsApp, tell us what you want and where you will find us.")),
            (("Te decimos la hora", "We give you a time"), ("Te contestamos con la hora a la que estará listo.", "We reply with the time it will be ready.")),
            (("Pasas a por él", "You collect it"), ("Lo recoges en la ventanilla y pagas ahí.", "You collect it at the hatch and pay there.")),
        ], tono="base", anchor="como-pedir"),
        bloque_opiniones(("Lo que dicen quienes nos han encontrado", "What people who have found us say"),
                         [("la comida", "the food"), ("el tiempo de espera", "the waiting time"), ("el trato", "the service")], tono="relleno"),
        bloque_cta(("¿Montamos algo en tu evento?", "Shall we set up at your event?"),
                   ("Bodas, cumpleaños, empresas, mercadillos y fiestas. Cuéntanos la fecha, cuántos sois y dónde.",
                    "Weddings, birthdays, companies, markets and parties. Tell us the date, how many of you there are and where."),
                   [boton(("Pedir catering", "Ask about catering"), a_pagina("catering")), whatsapp("secondary")], tono="base"),
    ])

    carta = pagina("carta", ("carta", "menu"), ("Carta y alérgenos", "Menu and allergens"),
                   ("Carta corta con precios, alérgenos y preguntas frecuentes del food truck.",
                    "Short menu with prices, allergens and frequently asked questions from the food truck."), [
        hero(("La carta", "The menu"),
             ("Pocos platos, bien hechos y rápidos. Mira qué hay, avísanos de alergias antes de pedir y reserva por WhatsApp.",
              "A few dishes, well made and quick. See what is on, tell us about allergies before you order and reserve on WhatsApp."),
             [pedir_wa(), boton(("Ver dónde estamos", "See where we are"), a_pagina("inicio", "esta-semana"), "secondary")], variante="texto"),
        bloque_carta(("Carta", "Menu"), [
            (("Para comer", "To eat"), ("Platos principales", "Main dishes"), PLATO,
             [(("Hamburguesa de la casa", "House burger"), eur(8.5)), (("Hamburguesa doble", "Double burger"), eur(10.5)),
              (("Bocadillo de pollo", "Chicken sandwich"), eur(6.5)), (("Bocadillo de verduras a la plancha", "Griddled vegetable sandwich"), eur(6)),
              (("Tacos de cerdo (3)", "Pork tacos (3)"), eur(8))]),
            (("Para acompañar y de postre", "Sides and dessert"), ("Acompañamientos y postre", "Sides and dessert"), PLATO,
             [(("Patatas fritas", "Chips"), eur(3)), (("Patatas con salsa", "Chips with sauce"), eur(4)), (("Brownie", "Brownie"), eur(3.5))]),
            (("Bebidas y combo", "Drinks and combo"), ("Bebidas y combo", "Drinks and combo"), PLATO,
             [(("Refresco o agua", "Soft drink or water"), eur(2)), (("Cerveza", "Beer"), eur(2.5)),
              (("Combo: hamburguesa, patatas y bebida", "Combo: burger, chips and a drink"), eur(11))]),
        ], nota=NOTA_PRECIOS, tono="relleno", anchor="carta",
            intro=("La carta de ejemplo: cámbiala por tus platos. Mejor corta: sale más rápido y sin errores.",
                   "This is an example menu: change it to your dishes. Shorter is better: it comes out faster and with fewer mistakes.")),
        seccion("text", [
            titular(("Alérgenos", "Allergens"),
                    ("Dínoslo antes de pedir y te decimos qué puedes tomar. En un camión se comparte plancha y espacio, así que puede haber trazas.",
                     "Tell us before you order and we will say what you can have. A truck shares a griddle and space, so traces are possible.")),
            tabla(("Alérgenos por producto", "Allergens by product"),
                  [("Producto", "Product"), ("Puede llevar", "May contain")],
                  [(("Pan de hamburguesa y de bocadillo", "Burger and sandwich buns"), ("Gluten, huevo, leche y sésamo", "Gluten, egg, milk and sesame")),
                   (("Salsa de la casa", "House sauce"), ("Huevo y mostaza", "Egg and mustard")),
                   (("Tortillas de maíz", "Corn tortillas"), ("Pueden tener trazas de gluten", "May have traces of gluten")),
                   (("Brownie", "Brownie"), ("Gluten, huevo, leche y frutos secos", "Gluten, egg, milk and nuts"))]),
            peq(("Alérgenos de ejemplo: cambia la tabla por los de tus recetas y revísala cada vez que cambies un ingrediente.",
                 "Example allergens: replace the table with those of your recipes and review it every time you change an ingredient."), color="$muted"),
        ], tono="base", anchor="alergenos", gap=24),
        bloque_faq(("Preguntas frecuentes", "Frequently asked questions"), [
            (("¿Cuánto tardáis?", "How long do you take?"),
             ("En hora tranquila, entre cinco y diez minutos. Con cola, más. Si reservas por WhatsApp, lo tienes listo a la hora que te digamos.",
              "At a quiet time, five to ten minutes. With a queue, longer. If you reserve on WhatsApp, it is ready at the time we give you.")),
            (("¿Hay algo vegetariano?", "Is there anything vegetarian?"),
             ("Sí, el bocadillo de verduras a la plancha. Pregúntanos por las salsas.", "Yes, the griddled vegetable sandwich. Ask us about the sauces.")),
            (("¿Cómo puedo pagar?", "How can I pay?"),
             ("Con efectivo o con tarjeta. Es un ejemplo: cambia las formas de pago por las que aceptes.",
              "By cash or card. This is an example: change the payment methods to the ones you accept.")),
            (("¿Y si llueve?", "What if it rains?"),
             ("Servimos igualmente por la ventanilla. Si suspendemos un día, lo avisamos en el WhatsApp y en las redes.",
              "We still serve at the hatch. If we cancel a day, we say so on WhatsApp and on social media.")),
        ], tono="relleno"),
        bloque_cta(("¿Qué te apetece hoy?", "What are you in the mood for today?"), None, [pedir_wa(), llamar("secondary")], tono="base"),
    ])

    catering = pagina("catering", ("catering", "catering"), ("Catering para eventos", "Event catering"),
                      ("Catering con el food truck: cómo se pide, precios de ejemplo, qué necesitamos del lugar y formulario con fecha, personas y lugar.",
                       "Catering with the food truck: how to ask, example prices, what we need to know about the venue and a form with date, guests and place."), [
        hero(("Catering para tu evento", "Catering for your event"),
             ("Aparcamos el camión en tu boda, cumpleaños, fiesta de empresa o mercadillo y servimos desde la ventanilla.",
              "We park the truck at your wedding, birthday, work party or market and serve from the hatch."),
             [boton(("Pedir presupuesto", "Ask for a quote"), a_ancla("pedir-catering")), whatsapp("secondary")], variante="texto"),
        bloque_pasos(("Cómo se organiza", "How it works"), [
            (("Nos cuentas", "You tell us"), ("Fecha, número de invitados y lugar.", "Date, number of guests and place.")),
            (("Vemos el sitio", "We check the venue"), ("Comprobamos que el camión cabe y qué hace falta para servir.", "We check the truck fits and what is needed to serve.")),
            (("Te mandamos presupuesto", "We send you a quote"), ("Con la carta del evento y el precio.", "With the event menu and the price.")),
            (("Reservamos la fecha", "We book the date"), ("Cuando confirmas, la dejamos apuntada.", "When you confirm, we mark it in the diary.")),
        ], tono="relleno", anchor="como-funciona"),
        bloque_precios(("Precios de ejemplo", "Example prices"), ("Precios de catering", "Catering prices"),
                       [("Evento", "Event"), ("Qué incluye", "What is included"), ("Precio", "Price")],
                       [(("Hasta 40 personas", "Up to 40 guests"), ("Dos horas de servicio y carta de evento", "Two hours of service and the event menu"), desde(450)),
                        (("De 40 a 100 personas", "40 to 100 guests"), ("Tres horas de servicio y carta de evento", "Three hours of service and the event menu"), desde(950)),
                        (("Más de 100 personas", "More than 100 guests"), ("A medida, según el evento", "Made to measure, depending on the event"), ("A consultar", "Ask us"))],
                       nota=NOTA_PRECIOS, tono="base", anchor="precios-catering"),
        bloque_checks(("Qué necesitamos saber del lugar", "What we need to know about the venue"), [
            ("Si el camión puede llegar y aparcar: anchura, altura y suelo llano.", "Whether the truck can get in and park: width, height and level ground."),
            ("A qué hora podemos llegar a montar y a qué hora termina el servicio.", "What time we can arrive to set up and what time service ends."),
            ("Si es en la vía pública, hace falta permiso del ayuntamiento: pregunta cuál y te ayudamos.", "If it is on a public road, you need the council's permission: ask which one and we will help."),
            ("Si hay invitados con alergias o dietas, para tenerlo en cuenta en la carta.", "Whether any guests have allergies or diets, so we can plan the menu."),
        ], cols=2, tono="relleno"),
        sec_form(("Pide presupuesto de catering", "Ask for a catering quote"),
                 ("Rellena lo básico y te contestamos con la carta y el precio, sin compromiso.",
                  "Fill in the basics and we will reply with the menu and price, with no commitment."),
                 [campo("nombre", "text", ("Nombre", "Name"), True, auto="name", maxlen=120),
                  campo("telefono", "tel", ("Teléfono", "Phone"), True, auto="tel", maxlen=30),
                  campo("correo", "email", ("Correo electrónico", "Email"), False, auto="email", maxlen=200),
                  campo("evento", "select", ("Tipo de evento", "Type of event"), False,
                        opciones=[("Boda", "Wedding"), ("Cumpleaños o fiesta", "Birthday or party"), ("Empresa", "Company event"),
                                  ("Mercadillo o feria", "Market or fair"), ("Otro", "Other")]),
                  campo("fecha", "date", ("Fecha del evento", "Event date"), True),
                  campo("personas", "text", ("Número de personas", "Number of guests"), True, maxlen=10),
                  campo("lugar", "text", ("Lugar del evento", "Event venue"), True, maxlen=200),
                  campo("mensaje", "textarea", ("Algo más que debamos saber (opcional)", "Anything else we should know (optional)"), False, maxlen=1000)],
                 ("Pedir presupuesto", "Request a quote"),
                 ("Gracias. Hemos recibido tu petición y te respondemos con la carta y el precio lo antes posible.",
                  "Thank you. We have received your request and will reply with the menu and price as soon as possible."),
                 "pedir-catering", tono="base"),
    ])

    cont = pagina("contacto", ("contacto", "contact"), ("Contacto y dónde estamos", "Contact and where we are"),
                  ("Teléfono, WhatsApp, formulario, horario de servicio y mapa con el sitio habitual.",
                   "Phone, WhatsApp, form, service hours and a map with the usual spot."), [
        hero(("Contacto", "Contact"),
             ("Escríbenos por WhatsApp para reservar tu pedido o preguntarnos dónde estamos hoy.",
              "Message us on WhatsApp to reserve your order or to ask where we are today."),
             [llamar(), whatsapp()], variante="texto"),
        bloque_contacto(("Habla con nosotros", "Talk to us"),
                        ("Para pedidos, WhatsApp es lo más rápido. Para catering, usa la página de catering.",
                         "For orders, WhatsApp is quickest. For catering, use the catering page."),
                        campos_cita(servicios=[("Una pregunta", "A question"), ("Catering", "Catering"), ("Otra cosa", "Something else")], fecha=False),
                        ("Enviar mensaje", "Send message"), horas, direccion=("Sitio habitual: Plaza Mayor de ejemplo, Tu ciudad", "Usual spot: Example Main Square, Your city"),
                        notas=("El horario depende del sitio de cada día: míralo en «Dónde estamos esta semana». Horas de ejemplo: cámbialas por las tuyas.",
                               "Hours depend on each day's spot: see «Where we are this week». Example hours: change them to your own.")),
        bloque_mapa(("Nuestro sitio habitual", "Our usual spot"),
                    ("El marcador enseña dónde aparcamos casi siempre. Es una ubicación de ejemplo: cámbiala por la tuya. Los demás días, mira la tabla de la semana.",
                     "The marker shows where we park most of the time. It is an example location: change it to yours. On other days, see the weekly table."),
                    tono="relleno"),
    ])

    return plantilla(KEY, ("Food truck", "Food truck"),
                     ("Dónde estamos esta semana, carta corta con precios, alérgenos, pedidos por WhatsApp y catering para eventos con formulario, con botones de llamar y WhatsApp.",
                      "Where we are this week, a short menu with prices, allergens, orders on WhatsApp and event catering with a form, with call and WhatsApp buttons."),
                     "comida", ("food truck foodtruck camion de comida comida callejera street food hamburguesas bocadillos tacos catering eventos comida rapida ambulante",
                                "food truck street food burgers sandwiches tacos catering events mobile kitchen takeaway van"),
                     look, cabecera(nav, cta_cab),
                     pie(nav, ("Food truck. Comida hecha en el momento, pedidos por WhatsApp y catering para eventos.",
                               "Food truck. Food made to order, WhatsApp orders and event catering."),
                         ("Martes a viernes 12:30 a 16:00. Sábado 12:00 a 17:00.", "Tuesday to Friday 12:30 pm to 4 pm. Saturday 12 pm to 5 pm.")),
                     [inicio, carta, catering, cont])
