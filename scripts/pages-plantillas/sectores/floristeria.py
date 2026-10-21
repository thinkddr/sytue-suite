# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
from lib import *
from looks import LOOKS

KEY = "floristeria"


def build():
    nueva(KEY)
    look = LOOKS["coral"]
    nav = [("inicio", ("Inicio", "Home")), ("ramos", ("Ramos y plantas", "Bouquets and plants")),
           ("eventos", ("Eventos y bodas", "Events and weddings")), ("contacto", ("Contacto", "Contact"))]
    horas = [(("Lunes a viernes", "Monday to Friday"), ("9:00 a 14:00 y 17:00 a 20:00", "9:00 am to 2:00 pm and 5:00 pm to 8:00 pm")),
             (("Sábado", "Saturday"), ("9:00 a 14:00", "9:00 am to 2:00 pm")),
             (("Domingo", "Sunday"), ("Cerrado", "Closed"))]
    cta_cab = marco([llamar(t=("Llamar", "Call")), whatsapp("ghost", ("WhatsApp", "WhatsApp"))],
                    dir="row", gap=8, align="center")
    a_ramos = a_pagina("ramos", "catalogo")
    a_presu = a_pagina("eventos", "presupuesto")

    inicio = pagina("inicio", ("", ""), ("Floristería", "Florist"),
                    ("Floristería con pedidos online: ramos y plantas con carrito, entrega a domicilio en el día, eventos y bodas.",
                     "Florist with online orders: bouquets and plants with a cart, same-day home delivery, events and weddings."), [
        hero(("Flores frescas, en tu casa hoy", "Fresh flowers at your door today"),
             ("Elige un ramo o una planta, dinos a dónde lo llevamos y qué quieres que ponga la tarjeta. Nosotros lo preparamos y lo entregamos.",
              "Choose a bouquet or a plant, tell us where to take it and what the card should say. We make it up and deliver it."),
             [boton(("Ver ramos y plantas", "See bouquets and plants"), a_ramos), llamar("secondary"), whatsapp("ghost")],
             eyebrow=("Floristería con entrega a domicilio", "Florist with home delivery"),
             imagen_=("photo", ("Ramo de flores frescas en el mostrador de la floristería", "Bouquet of fresh flowers on the florist's counter"))),
        bloque_productos(("Ramos y plantas", "Bouquets and plants"),
                         ("Cada ramo se hace al día con las flores que han llegado. La foto es orientativa: las flores pueden cambiar de color o de variedad según la semana.",
                          "Each bouquet is made on the day with the flowers that have arrived. The photo is a guide: flowers may change in colour or variety depending on the week."),
                         limite=4, cols=4, anchor="ramos",
                         boton_=boton(("Ver todo el catálogo", "See the whole catalogue"), a_ramos, "secondary")),
        bloque_texto(("Entrega a domicilio en el día", "Same-day home delivery"),
                     [("Entregamos en el día en Tu ciudad y alrededores si haces el pedido antes de las 13:00. Fuera de esa zona, te decimos el plazo antes de cobrar.",
                       "We deliver the same day in Your city and surrounding area if you order before 1:00 pm. Outside that area, we tell you the time before charging you."),
                      ("Si no hay nadie en casa, llamamos al teléfono que nos des o lo dejamos con un vecino, como prefieras.",
                       "If nobody is home, we call the number you give us or leave it with a neighbour, whichever you prefer.")],
                     eyebrow=("Reparto", "Delivery"), imagen_=("photo", ("Repartidor entregando un ramo en la puerta de una casa", "Courier handing over a bouquet at a front door")),
                     tono="relleno", anchor="entrega"),
        bloque_valores(("Flores de temporada", "Seasonal flowers"), [
            (("Primavera", "Spring"), ("Tulipanes, peonías, lilas y margaritas.", "Tulips, peonies, lilacs and daisies.")),
            (("Verano", "Summer"), ("Girasoles, gladiolos, hortensias y zinnias.", "Sunflowers, gladioli, hydrangeas and zinnias.")),
            (("Otoño", "Autumn"), ("Crisantemos, dalias, bayas y hojas secas.", "Chrysanthemums, dahlias, berries and dried leaves.")),
            (("Invierno", "Winter"), ("Anémonas, ranúnculos, narcisos y camelias.", "Anemones, ranunculus, daffodils and camellias.")),
        ], intro=("La flor de temporada dura más y cuesta menos. Rosas, gerberas y claveles tenemos casi todo el año. Pregúntanos qué ha llegado esta semana.",
                  "Seasonal flowers last longer and cost less. We have roses, gerberas and carnations almost all year round. Ask us what has arrived this week."),
            cols=4, estilo="linea", anchor="temporada"),
        bloque_cta(("¿Una boda, un bautizo o una despedida?", "A wedding, a christening or a farewell?"),
                   ("Cuéntanos la fecha y el sitio y te preparamos un presupuesto sin compromiso.",
                    "Tell us the date and the place and we will prepare a quote with no obligation."),
                   [boton(("Pedir presupuesto", "Ask for a quote"), a_presu), whatsapp("ghost")], tono="relleno"),
        bloque_opiniones(("Lo que dicen nuestros clientes", "What our customers say"),
                         [("un ramo", "a bouquet"), ("la entrega", "the delivery"), ("las flores de una boda", "wedding flowers")], tono="base"),
    ])

    ramos = pagina("ramos", ("ramos", "bouquets"), ("Ramos y plantas", "Bouquets and plants"),
                   ("Catálogo de ramos y plantas con carrito, zonas y plazos de entrega, y cuidados para que duren más.",
                    "Catalogue of bouquets and plants with a cart, delivery areas and times, and care tips to make them last."), [
        hero(("Ramos y plantas", "Bouquets and plants"),
             ("Añade lo que quieras al carrito, escribe la dirección y la dedicatoria y pagas online. Si prefieres hablarlo, llámanos.",
              "Add what you like to the cart, write the address and the message, and pay online. If you would rather talk it over, call us."),
             [boton(("Cómo cuidarlos", "How to care for them"), a_ancla("cuidados"), "secondary"), llamar("ghost")], variante="texto"),
        bloque_productos(("Catálogo", "Catalogue"), None, limite=24, cols=4, anchor="catalogo"),
        bloque_precios(("Zonas y plazos de entrega", "Delivery areas and times"), ("Entrega a domicilio por zona", "Home delivery by area"),
                       [("Zona", "Area"), ("Plazo", "Time"), ("Coste", "Cost")],
                       [(("Tu ciudad", "Your city"), ("En el día, si pides antes de las 13:00", "Same day, if you order before 1:00 pm"), eur(4)),
                        (("Barrios y pueblos cercanos", "Nearby districts and villages"), ("En el día o al siguiente", "Same day or the next"), eur(7)),
                        (("Recogida en la floristería", "Pick up at the shop"), ("Cuando quieras en horario", "Whenever you like during opening hours"), ("Gratis", "Free"))],
                       intro=("Para un día señalado, como San Valentín o el Día de la Madre, pide con unos días de margen.",
                              "For a special day, such as Valentine's Day or Mother's Day, order a few days ahead."),
                       nota=("Zonas y precios de ejemplo: cámbialos por los tuyos. Incluyen IVA.",
                             "Example areas and prices: replace them with your own. VAT included."),
                       anchor="entregas", tono="relleno"),
        bloque_faq(("Cuidados: que duren más", "Care: make them last"), [
            (("¿Cada cuánto cambio el agua del ramo?", "How often should I change the bouquet water?"),
             ("Cada dos días. Corta un par de centímetros de los tallos en diagonal y límpialos bien, porque el agua sucia las marchita antes.",
              "Every two days. Cut a couple of centimetres off the stems at an angle and clean the vase well, because dirty water wilts them sooner.")),
            (("¿Dónde pongo las flores?", "Where should I put the flowers?"),
             ("En un sitio fresco, lejos del sol directo, de la calefacción y de la fruta, que desprende gas y las envejece.",
              "Somewhere cool, away from direct sun, heating and fruit, which gives off gas and ages them.")),
            (("¿Cuánto riego una planta de interior?", "How much should I water an indoor plant?"),
             ("Mejor poco y a menudo que de golpe. Toca la tierra: si está seca a dos dedos de profundidad, riega; si está húmeda, espera.",
              "Little and often is better than one big soak. Touch the soil: if it is dry two fingers deep, water; if it is damp, wait.")),
            (("¿Y si las hojas se ponen amarillas?", "What if the leaves turn yellow?"),
             ("Casi siempre es exceso de agua. Deja secar la tierra unos días y comprueba que la maceta tiene agujero de drenaje.",
              "It is almost always too much water. Let the soil dry for a few days and check that the pot has a drainage hole.")),
            (("¿Las flores llegan abiertas?", "Do the flowers arrive open?"),
             ("Llegan en capullo o medio abiertas, para que las disfrutes más días. En uno o dos días terminan de abrir.",
              "They arrive in bud or half open so you enjoy them for longer. They finish opening in a day or two.")),
        ], tono="base", anchor="cuidados"),
        bloque_cta(("¿Quieres algo que no está en el catálogo?", "Want something that is not in the catalogue?"),
                   ("Dinos tu presupuesto y para quién es y te lo preparamos a medida.",
                    "Tell us your budget and who it is for and we will make it to order."), tono="relleno"),
    ])

    form_presu = formulario(
        [campo("nombre", "text", ("Nombre", "Name"), True, auto="name", maxlen=120),
         campo("telefono", "tel", ("Teléfono", "Phone"), True, auto="tel", maxlen=30),
         campo("correo", "email", ("Correo electrónico", "Email"), False, auto="email", maxlen=200),
         campo("evento", "select", ("Tipo de evento", "Type of event"), True,
               opciones=[("Boda", "Wedding"), ("Bautizo o comunión", "Christening or communion"),
                         ("Despedida o funeral", "Farewell or funeral"), ("Fiesta o evento de empresa", "Party or company event"),
                         ("Otro", "Other")]),
         campo("fecha", "date", ("Fecha del evento", "Event date"), True),
         campo("lugar", "text", ("Lugar", "Venue"), False, maxlen=200),
         campo("invitados", "text", ("Número aproximado de invitados", "Approximate number of guests"), False, maxlen=20),
         campo("mensaje", "textarea", ("Cuéntanos qué imaginas (colores, estilo, presupuesto)", "Tell us what you have in mind (colours, style, budget)"),
               False, maxlen=2000)],
        ("Pedir presupuesto", "Ask for a quote"),
        ("Gracias. Revisamos tu fecha y te respondemos con un presupuesto en uno o dos días laborables.",
         "Thank you. We will check your date and reply with a quote within one or two working days."))
    eventos = pagina("eventos", ("eventos", "events"), ("Eventos y bodas", "Events and weddings"),
                     ("Flores para bodas, bautizos y despedidas: cómo trabajamos y formulario para pedir presupuesto sin compromiso.",
                      "Flowers for weddings, christenings and farewells: how we work and a form to ask for a quote with no obligation."), [
        hero(("Flores para bodas y eventos", "Flowers for weddings and events"),
             ("Ramo de novia, ceremonia, mesas y coronas. Te hacemos un presupuesto sin compromiso y, si quieres, una prueba antes del día.",
              "Bridal bouquet, ceremony, tables and wreaths. We give you a quote with no obligation and, if you like, a trial before the day."),
             [boton(("Pedir presupuesto", "Ask for a quote"), a_ancla("presupuesto")), whatsapp()],
             eyebrow=("Eventos y bodas", "Events and weddings"), variante="centro", tono="relleno"),
        bloque_valores(("Qué preparamos", "What we make"), [
            (("Ramo y ramillete", "Bridal bouquet and buttonholes"), ("Ramo de novia y flores para el cortejo, a juego con el vestido.", "Bridal bouquet and flowers for the wedding party, to match the dress.")),
            (("Ceremonia", "Ceremony"), ("Decoración del altar o del lugar de la ceremonia, arcos y bancos.", "Decoration for the altar or ceremony space, arches and pews.")),
            (("Mesas", "Tables"), ("Centros de mesa, minimesas y detalles para invitados.", "Table centrepieces, small arrangements and guest favours.")),
            (("Despedidas", "Farewells"), ("Coronas y ramos, con entrega en el tanatorio o en la iglesia a la hora acordada.", "Wreaths and bouquets, delivered to the funeral home or church at the agreed time.")),
        ], cols=4, estilo="borde", anchor="servicios"),
        bloque_pasos(("Cómo lo preparamos", "How we plan it"), [
            (("Nos cuentas", "You tell us"), ("Fecha, lugar, número de invitados, colores y lo que quieres gastar.", "Date, venue, number of guests, colours and what you want to spend.")),
            (("Te hacemos un presupuesto", "We send a quote"), ("Por escrito y desglosado, sin compromiso. Puedes quitar o cambiar partidas.", "In writing and itemised, with no obligation. You can remove or change items.")),
            (("Prueba y reserva", "Trial and booking"), ("Hacemos una prueba del ramo o de un centro. Para reservar la fecha pedimos una señal.", "We make a trial of the bouquet or a centrepiece. To hold the date we ask for a deposit.")),
            (("Montaje y entrega", "Set-up and delivery"), ("Llevamos y colocamos todo a la hora acordada, y recogemos lo que haga falta después.", "We bring and place everything at the agreed time, and collect whatever is needed afterwards.")),
        ], tono="relleno", anchor="proceso"),
        seccion("contact", [marco([
            marco([titular(("Presupuesto sin compromiso", "Quote with no obligation"),
                           ("Cuanto antes nos avises, mejor: en temporada de bodas las fechas de mayo a septiembre se llenan pronto. Rellena el formulario o llámanos.",
                            "The sooner you tell us, the better: in wedding season the dates from May to September fill up fast. Fill in the form or call us.")),
                   marco([llamar(), whatsapp()], dir="row", gap=12, wrap=True)],
                  gap=20, sz={"width": "fill", "height": "hug", "min_width": 0}),
            tarjeta([form_presu], estilo="relleno", pad=28, tag="div", sz={"width": "fill", "height": "hug", "min_width": 0}),
        ], dir="row", gap=48, align="start",
            bp={"tablet": {"layout": {"gap": 32}}, "mobile": {"layout": {"direction": "column", "gap": 28}}})],
            anchor="presupuesto", tono="base", gap=0),
    ])

    cont = pagina("contacto", ("contacto", "contact"), ("Contacto y horario", "Contact and opening hours"),
                  ("Horario de la floristería, cómo llegar, teléfono, WhatsApp y formulario para pedidos y consultas.",
                   "Florist's opening hours, how to get here, phone, WhatsApp and a form for orders and questions."), [
        hero(("Contacto", "Contact"),
             ("Para un pedido para hoy, lo más rápido es llamar o escribir por WhatsApp. Para un evento, usa el formulario de presupuesto.",
              "For an order for today, the quickest way is to call or message on WhatsApp. For an event, use the quote form."),
             [llamar(), whatsapp()], variante="texto"),
        bloque_contacto(("Pasa por la floristería", "Come by the shop"),
                        ("Si vienes a recoger un pedido, dinos tu nombre y te lo tenemos preparado.",
                         "If you are coming to collect an order, give us your name and we will have it ready."),
                        campos_consulta(servicios=[("Un pedido", "An order"), ("Una entrega", "A delivery"),
                                                   ("Un evento o boda", "An event or wedding"), ("Otra cosa", "Something else")]),
                        ("Enviar mensaje", "Send message"), horas),
        bloque_mapa(("Cómo llegar a la floristería", "How to get to the shop"), tono="relleno"),
    ])

    return plantilla(KEY, ("Floristería", "Florist"),
                     ("Floristería con ramos y plantas con carrito, entrega a domicilio en el día, presupuesto para eventos y bodas, cuidados y botones de llamar y WhatsApp.",
                      "Florist with bouquets and plants with a cart, same-day home delivery, quotes for events and weddings, care tips and call and WhatsApp buttons."),
                     "comercio", ("floristeria florista flores ramos plantas ramo de novia bodas eventos entrega a domicilio flores a domicilio",
                                  "florist flower shop flowers bouquets plants wedding flowers events flower delivery"),
                     look, cabecera(nav, cta_cab, tienda=True),
                     pie(nav, ("Floristería con pedidos online y entrega a domicilio.", "Florist with online orders and home delivery."),
                         ("Lunes a viernes 9:00 a 14:00 y 17:00 a 20:00. Sábado 9:00 a 14:00.",
                          "Monday to Friday 9 am to 2 pm and 5 pm to 8 pm. Saturday 9 am to 2 pm.")),
                     [inicio, ramos, eventos, cont], tienda=True)
