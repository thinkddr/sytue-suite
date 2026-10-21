# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
from lib import *
from looks import LOOKS  # noqa: F401  (la paleta es propia de este sector)

KEY = "alquiler-eventos-mobiliario"

# Paleta propia: melocotón de lino con azul marino de carpa. Fondo rosado-anaranjado pálido, distinto de los
# cremas y blancos del catálogo; el marino pasa AA sobre fondo y sobre `surface`. Sin `tono="oscuro"`:
# primary/inverse no llega a 3, así que los bloques de énfasis usan `relleno`.
LINO = dict(
    colores=dict(background="#fdeadd", surface="#f8d6c0", text="#1d2340", muted="#4d4654", primary="#233b82",
                 on_primary="#ffffff", border="#ecc9b2", inverse="#1b2a5c", on_inverse="#fdeadd"),
    fuentes=("work-sans", "ibm-plex-sans"), peso=700, h1=(50, 40, 32), radio=(2, 6, 10), ancho=1160)


def build():
    nueva(KEY)
    look = LINO
    nav = [("inicio", ("Inicio", "Home")), ("catalogo", ("Catálogo y tarifas", "Catalogue and rates")),
           ("entrega", ("Entrega y condiciones", "Delivery and terms")), ("contacto", ("Pedir presupuesto", "Request a quote"))]
    horas = [(("Lunes a viernes", "Monday to Friday"),
              ("9:00 a 14:00 y 16:00 a 19:00", "9:00 am to 2:00 pm and 4:00 pm to 7:00 pm")),
             (("Sábado", "Saturday"), ("9:00 a 13:00; entregas y recogidas", "9:00 am to 1:00 pm; deliveries and pick-ups")),
             (("Domingo", "Sunday"), ("Solo entregas ya acordadas", "Agreed deliveries only"))]
    nota = ("Precios de ejemplo: cámbialos por los tuyos. Por día de alquiler, sin IVA. La entrega y la recogida se presupuestan aparte.",
            "Example prices: replace them with your own. Per rental day, excluding VAT. Delivery and collection are quoted separately.")

    def presupuesto(var="primary"):
        return boton(("Pedir presupuesto", "Request a quote"), a_pagina("contacto", "presupuesto"), var)

    inicio = pagina("inicio", ("", ""), ("Alquiler para eventos", "Event rentals"),
                    ("Alquiler de mobiliario, carpas, vajilla y hinchables para eventos: catálogo, montaje y recogida, zona de entrega y presupuesto con fecha.",
                     "Rental of furniture, marquees, tableware and inflatables for events: catalogue, set-up and collection, delivery area and a dated quote."), [
        hero(("Todo lo que necesita tu evento, en tu puerta y recogido al terminar", "Everything your event needs, at your door and collected afterwards"),
             ("Sillas, mesas, carpas, vajilla e hinchables en alquiler para bodas, comuniones, fiestas y eventos de empresa. Pide el presupuesto con tu fecha y el número de invitados.",
              "Chairs, tables, marquees, tableware and inflatables for hire for weddings, communions, parties and company events. Request a quote with your date and number of guests."),
             [presupuesto(), llamar("secondary"), whatsapp("ghost")], eyebrow=("Alquiler de material para eventos", "Event equipment hire"),
             imagen_=("photo", ("Mesas redondas con manteles blancos y sillas montadas en un jardín, sin invitados",
                                "Round tables with white cloths and chairs set up in a garden, with no guests"))),
        bloque_valores(("Qué alquilamos", "What we hire out"), [
            (("Mobiliario", "Furniture"), ("Sillas, mesas redondas y rectangulares, mesas altas de cóctel y bancos.",
                                           "Chairs, round and rectangular tables, high cocktail tables and benches.")),
            (("Carpas y toldos", "Marquees and awnings"), ("Carpas de distintos tamaños con suelo, laterales e iluminación.",
                                                          "Marquees of different sizes with flooring, side panels and lighting.")),
            (("Vajilla y cristalería", "Tableware and glassware"), ("Platos, cubiertos, copas y manteles, por docenas y listos para usar.",
                                                                    "Plates, cutlery, glasses and tablecloths, by the dozen and ready to use.")),
            (("Hinchables y juegos", "Inflatables and games"), ("Castillos, toboganes y juegos de feria para eventos con niños.",
                                                                "Bouncy castles, slides and fairground games for events with children.")),
        ], cols=4, estilo="relleno", tono="base", anchor="alquilamos",
            intro=("El catálogo con medidas y tarifas está en su página.", "The catalogue with sizes and rates is on its own page.")),
        bloque_pasos(("Del pedido a la recogida", "From order to collection"), [
            (("Nos cuentas el evento", "Tell us about the event"),
             ("Fecha, lugar y número de invitados. Con eso calculamos lo que hace falta.",
              "Date, place and number of guests. With that we work out what you need.")),
            (("Presupuesto", "Quote"),
             ("Te enviamos la lista de material con el precio total, entrega incluida.",
              "We send you the equipment list with the total price, delivery included.")),
            (("Reserva", "Booking"),
             ("Se aparta el material para tu fecha con una señal (texto de ejemplo).",
              "The equipment is set aside for your date with a deposit (example text).")),
            (("Entrega y recogida", "Delivery and collection"),
             ("Lo llevamos, lo montamos si lo has pedido y volvemos a recogerlo al terminar.",
              "We bring it, set it up if you asked us to, and come back to collect it afterwards.")),
        ], tono="relleno"),
        bloque_texto(("Montamos y recogemos para que tú no cargues nada", "We set up and collect so you do not carry a thing"), [
            ("El montaje de mesas, sillas y carpas va incluido o se presupuesta aparte, según lo que pidas. Siempre lo sabes antes de reservar.",
             "Setting up tables, chairs and marquees is either included or quoted separately, depending on what you order. You always know before you book."),
            ("Recogemos el material cuando acaba el evento o a la mañana siguiente, como acordemos.",
             "We collect the equipment when the event ends or the next morning, as we agree."),
        ], imagen_=("landscape", ("Furgoneta cargada con sillas apiladas y carpa plegada junto a un salón de celebraciones",
                                  "Van loaded with stacked chairs and a folded marquee beside a function room")),
            tono="base", eyebrow=("Montaje y recogida", "Set-up and collection")),
        bloque_opiniones(("Lo que dicen quienes nos alquilaron", "What our customers say"),
                         [("la puntualidad de la entrega", "the punctual delivery"), ("el estado del material", "the condition of the equipment"), ("el montaje", "the set-up")],
                         tono="relleno"),
        bloque_cta(("¿Qué fecha tienes y cuántos sois?", "What is your date and how many of you are there?"),
                   ("Dinos el día y el número de invitados y te preparamos un presupuesto con el material que hace falta.",
                    "Tell us the day and the number of guests and we will put together a quote with the equipment you need."),
                   [presupuesto(), whatsapp("ghost")], tono="base"),
    ])

    def tarjetas_cat():
        return bloque_valores(("Categorías", "Categories"), [
            (("Sillas", "Chairs"), ("Plegables, de resina y de madera, en blanco y en natural.", "Folding, resin and wooden, in white and natural.")),
            (("Mesas", "Tables"), ("Redondas para 8 o 10 personas, rectangulares y altas de cóctel.", "Round for 8 or 10 people, rectangular and high cocktail tables.")),
            (("Carpas", "Marquees"), ("De 3 x 3 m a 6 x 12 m, con laterales y suelo si lo necesitas.", "From 3 x 3 m to 6 x 12 m, with side panels and flooring if you need them.")),
            (("Vajilla", "Tableware"), ("Platos, cubiertos, copas y vasos por unidades o por lotes.", "Plates, cutlery, glasses and tumblers by the piece or in sets.")),
            (("Hinchables", "Inflatables"), ("Castillos y toboganes de distintos tamaños, con monitor opcional.", "Castles and slides of different sizes, with an optional supervisor.")),
            (("Decoración", "Decoration"), ("Manteles, fundas, guirnaldas de luz y arcos para ceremonias.", "Tablecloths, chair covers, string lights and ceremony arches.")),
        ], cols=3, estilo="borde", tono="base", kind="features", anchor="categorias",
            intro=("Categorías de ejemplo: deja solo las que de verdad alquilas.", "Example categories: keep only the ones you really hire out."))

    catalogo = pagina("catalogo", ("catalogo", "catalogue"), ("Catálogo y tarifas", "Catalogue and rates"),
                      ("Categorías de material en alquiler y tablas de tarifas de ejemplo de mobiliario, carpas, vajilla y hinchables.",
                       "Rental equipment categories and example rate tables for furniture, marquees, tableware and inflatables."), [
        hero(("Catálogo y tarifas", "Catalogue and rates"),
             ("Precios por día de alquiler para que te hagas una idea. El presupuesto final lleva tu fecha, la cantidad y la entrega.",
              "Prices per rental day so you can get an idea. The final quote includes your date, the quantity and delivery."),
             [presupuesto()], variante="texto"),
        tarjetas_cat(),
        bloque_carta(("Tarifas de ejemplo", "Example rates"), [
            (("Mobiliario", "Furniture"), ("Tarifas de mobiliario por unidad", "Furniture rates per unit"),
             [("Artículo", "Item"), ("Medida o capacidad", "Size or capacity"), ("Precio", "Price")],
             [(("Silla plegable blanca", "White folding chair"), ("1 persona", "1 person"), eur(0.8)),
              (("Silla de madera", "Wooden chair"), ("1 persona", "1 person"), eur(1.5)),
              (("Mesa redonda", "Round table"), ("8 a 10 personas, 150 cm", "8 to 10 people, 150 cm"), eur(8)),
              (("Mesa rectangular", "Rectangular table"), ("6 a 8 personas, 180 cm", "6 to 8 people, 180 cm"), eur(6)),
              (("Mesa alta de cóctel", "High cocktail table"), ("70 cm de diámetro", "70 cm diameter"), eur(9))]),
            (("Carpas", "Marquees"), ("Tarifas de carpas con montaje", "Marquee rates with set-up"),
             [("Tamaño", "Size"), ("Para unas", "Seats about"), ("Precio", "Price")],
             [(("3 x 3 m", "3 x 3 m"), ("Hasta 10 personas", "Up to 10 people"), desde(70)),
              (("4 x 8 m", "4 x 8 m"), ("Hasta 30 personas", "Up to 30 people"), desde(220)),
              (("6 x 12 m", "6 x 12 m"), ("Hasta 70 personas", "Up to 70 people"), desde(480))]),
            (("Vajilla y cristalería", "Tableware and glassware"), ("Tarifas de vajilla por docena", "Tableware rates per dozen"),
             [("Artículo", "Item"), ("Precio por docena", "Price per dozen")],
             [(("Platos llanos", "Dinner plates"), eur(4)), (("Cubiertos (juego de tres)", "Cutlery (set of three)"), eur(5)),
              (("Copas de agua o vino", "Water or wine glasses"), eur(4)), (("Manteles (unidad)", "Tablecloths (each)"), eur(6))]),
            (("Hinchables", "Inflatables"), ("Tarifas de hinchables por día", "Inflatable rates per day"),
             [("Artículo", "Item"), ("Medida", "Size"), ("Precio", "Price")],
             [(("Castillo pequeño", "Small castle"), ("4 x 4 m", "4 x 4 m"), desde(90)),
              (("Castillo con tobogán", "Castle with slide"), ("6 x 5 m", "6 x 5 m"), desde(150)),
              (("Juego de feria", "Fairground game"), ("Según juego", "Depends on game"), desde(60))]),
        ], nota=nota, anchor="tarifas", tono="relleno"),
        bloque_cta(("¿Te falta algo en la lista?", "Is something missing from the list?"),
                   ("Si necesitas un artículo que no aparece, pregúntanos: quizá lo tenemos o sabemos dónde conseguirlo.",
                    "If you need an item that is not listed, ask us: we may have it or know where to find it."),
                   [presupuesto(), whatsapp("ghost")], tono="base"),
    ])

    entrega = pagina("entrega", ("entrega", "delivery"), ("Entrega, montaje y condiciones", "Delivery, set-up and terms"),
                     ("Cómo es el montaje y la recogida, zona de entrega con costes de ejemplo, fianza, condiciones y preguntas frecuentes.",
                      "How set-up and collection work, delivery area with example costs, deposit, terms and frequently asked questions."), [
        hero(("Entrega, montaje y condiciones", "Delivery, set-up and terms"),
             ("Lo que pasa antes y después de tu evento, en claro: cuándo llegamos, qué fianza se pide y qué hacer si algo se rompe.",
              "What happens before and after your event, plainly: when we arrive, what deposit is asked and what to do if something breaks."),
             [presupuesto(), llamar("secondary")], variante="texto"),
        bloque_pasos(("Montaje y recogida", "Set-up and collection"), [
            (("Entrega", "Delivery"), ("Llegamos en la franja que acordamos. Alguien tiene que recibir el material y firmar el albarán.",
                                       "We arrive in the slot we agree. Someone has to receive the equipment and sign the delivery note.")),
            (("Montaje", "Set-up"), ("Colocamos mesas, sillas y carpas donde nos indiques. Necesitamos acceso para la furgoneta.",
                                     "We place tables, chairs and marquees where you tell us. We need vehicle access.")),
            (("Durante el evento", "During the event"), ("Tienes un teléfono para cualquier problema con el material.",
                                                         "You have a phone number for any problem with the equipment.")),
            (("Recogida", "Collection"), ("Recogemos al terminar o a primera hora del día siguiente. El material, apilado y recogido.",
                                          "We collect at the end or first thing the next day. Equipment stacked and gathered.")),
        ], tono="base", anchor="montaje"),
        bloque_precios(("Zona de entrega", "Delivery area"), ("Zonas de entrega con coste y pedido mínimo", "Delivery areas with cost and minimum order"),
                       [("Zona", "Area"), ("Entrega y recogida", "Delivery and collection"), ("Pedido mínimo", "Minimum order")],
                       [(("Tu ciudad", "Your city"), eur(35), eur(100)),
                        (("Hasta 15 km", "Up to 15 km"), eur(55), eur(150)),
                        (("De 15 a 40 km", "15 to 40 km"), eur(90), eur(250)),
                        (("Más de 40 km", "Over 40 km"), ("Según distancia", "Depends on distance"), ("Consultar", "Ask us"))],
                       intro=("Zonas y costes de ejemplo: cámbialos por los tuyos.", "Example areas and costs: replace them with your own."),
                       nota=nota, anchor="zona", tono="relleno"),
        bloque_checks(("Fianza y condiciones", "Deposit and terms"), [
            ("Fianza: escribe aquí el importe y cuándo se devuelve (texto de ejemplo).", "Deposit: write here the amount and when it is returned (example text)."),
            ("Reserva con una señal; el resto se paga antes de la entrega (texto de ejemplo).", "Booking with a deposit; the rest is paid before delivery (example text)."),
            ("Cancelación: indica aquí el plazo gratuito y lo que ocurre después (texto de ejemplo).", "Cancellation: state here the free period and what happens afterwards (example text)."),
            ("El material se devuelve limpio de restos gruesos y apilado como se entregó.", "Equipment is returned free of heavy residue and stacked as delivered."),
            ("Lo roto o perdido se cobra al precio de reposición que figura en el albarán.", "Anything broken or lost is charged at the replacement price shown on the delivery note."),
            ("En exterior, avísanos del tipo de suelo para anclar bien las carpas.", "For outdoor events, tell us the type of ground so we anchor marquees properly."),
        ], intro=("Condiciones de ejemplo: deja solo las que de verdad aplicas.", "Example terms: keep only the ones you really apply."),
            eyebrow=("Antes de firmar", "Before you sign"), tono="base", cols=2, anchor="condiciones"),
        bloque_faq(("Preguntas frecuentes", "Frequently asked questions"), [
            (("¿Con cuánta antelación tengo que reservar?", "How far ahead do I need to book?"),
             ("Cuanto antes, mejor, sobre todo para sábados de primavera y verano. Para pedidos pequeños a veces llegamos con pocos días.",
              "The sooner the better, especially for spring and summer Saturdays. For small orders we can sometimes manage with a few days' notice.")),
            (("¿Puedo recoger yo el material?", "Can I collect the equipment myself?"),
             ("Sí, si tu vehículo es adecuado. Te decimos cuándo pasar y cómo cargarlo; el precio baja porque no hay entrega.",
              "Yes, if your vehicle is suitable. We tell you when to come and how to load it; the price drops because there is no delivery.")),
            (("¿Qué pasa si llueve el día del evento?", "What if it rains on the day of the event?"),
             ("Si tienes carpa, tu fiesta sigue. Si todavía no la has pedido y el pronóstico cambia, llámanos: a veces podemos añadirla.",
              "If you have a marquee, your party carries on. If you have not ordered one and the forecast changes, call us: we can sometimes add it.")),
            (("¿Cómo sé cuántas sillas y mesas necesito?", "How do I know how many chairs and tables I need?"),
             ("Cuéntanos el número de invitados y si habrá banquete sentado o de pie. Te proponemos la cantidad y el reparto de mesas.",
              "Tell us the number of guests and whether the meal will be seated or standing. We suggest the quantity and table layout.")),
            (("¿Los hinchables llevan monitor?", "Do the inflatables come with a supervisor?"),
             ("Se alquilan con o sin monitor. Si no lo llevan, un adulto de tu evento tiene que vigilarlos todo el tiempo (texto de ejemplo).",
              "They are hired with or without a supervisor. Without one, an adult from your event must watch them at all times (example text).")),
        ], tono="relleno"),
        bloque_cta(("¿Calculamos tu pedido?", "Shall we work out your order?"), None, [presupuesto(), whatsapp("ghost")], tono="base"),
    ])

    cont = pagina("contacto", ("presupuesto", "quote"), ("Presupuesto y contacto", "Quote and contact"),
                  ("Pide presupuesto con tu fecha, el número de invitados y lo que necesitas, o llama o escribe por WhatsApp. Horario y cómo llegar.",
                   "Request a quote with your date, number of guests and what you need, or call or message us on WhatsApp. Opening hours and how to find us."), [
        hero(("Pide tu presupuesto", "Request your quote"),
             ("Dinos la fecha, cuántos invitados habrá y qué necesitas. Te contestamos con el material y el precio total.",
              "Tell us the date, how many guests there will be and what you need. We reply with the equipment and the total price."),
             [llamar(), whatsapp()], variante="texto"),
        bloque_contacto(("Presupuesto con fecha", "Quote with a date"),
                        ("Cuantos más datos nos des, más exacto será el presupuesto.", "The more details you give, the more accurate the quote."),
                        [campo("nombre", "text", ("Nombre", "Name"), True, auto="name", maxlen=120),
                         campo("telefono", "tel", ("Teléfono", "Phone"), True, auto="tel", maxlen=30),
                         campo("correo", "email", ("Correo electrónico (opcional)", "Email (optional)"), False, auto="email", maxlen=200),
                         campo("fecha", "date", ("Fecha del evento", "Event date"), True),
                         campo("invitados", "number", ("Número de invitados", "Number of guests"), True),
                         campo("evento", "select", ("Tipo de evento", "Type of event"), False,
                               opciones=[("Boda", "Wedding"), ("Comunión o bautizo", "Communion or christening"),
                                         ("Fiesta privada", "Private party"), ("Evento de empresa", "Company event"),
                                         ("Otro", "Other")]),
                         campo("material", "select", ("Qué necesitas", "What you need"), False,
                               opciones=[("Sillas y mesas", "Chairs and tables"), ("Carpa", "Marquee"),
                                         ("Vajilla y cristalería", "Tableware and glassware"), ("Hinchables", "Inflatables"),
                                         ("Varias cosas", "Several things")]),
                         campo("lugar", "text", ("Dónde se entrega", "Where to deliver"), True, maxlen=200),
                         campo("mensaje", "textarea", ("Cuéntanos algo más (opcional)", "Tell us more (optional)"), False, maxlen=1000)],
                        ("Pedir presupuesto", "Request quote"), horas, anchor="presupuesto",
                        notas=("Fuera de este horario respondemos por WhatsApp. En fines de semana de temporada alta, pide con tiempo.",
                               "Outside these hours we reply on WhatsApp. For high-season weekends, order well in advance."),
                        exito=("Gracias. Te enviamos el presupuesto con el material y el precio total lo antes posible.",
                               "Thank you. We will send you the quote with the equipment and the total price as soon as possible.")),
        bloque_mapa(("Dónde está el almacén", "Where the warehouse is"),
                    ("Entregamos en tu evento, pero puedes pasar a ver el material con cita previa.",
                     "We deliver to your event, but you can come and see the equipment by appointment."),
                    tono="relleno"),
    ])

    return plantilla(KEY, ("Alquiler de mobiliario para eventos", "Event furniture rental"),
                     ("Alquiler de mobiliario, carpas, vajilla y hinchables con catálogo, tarifas de ejemplo, montaje y recogida, zona de entrega, condiciones y presupuesto con fecha.",
                      "Hire of furniture, marquees, tableware and inflatables with a catalogue, example rates, set-up and collection, delivery area, terms and a dated quote."),
                     "creativos", ("alquiler mobiliario eventos alquiler sillas mesas carpas toldos vajilla cristaleria hinchables castillo hinchable menaje boda comunion fiesta montaje recogida material eventos",
                                   "event hire furniture rental chair hire table hire marquee tent hire tableware glassware bouncy castle inflatable hire wedding party equipment"),
                     look, cabecera(nav, marco([llamar(), whatsapp("ghost")], dir="row", gap=12, wrap=True)),
                     pie(nav, ("Alquiler de mobiliario, carpas, vajilla y hinchables para eventos, con entrega, montaje y recogida.",
                               "Hire of furniture, marquees, tableware and inflatables for events, with delivery, set-up and collection."),
                         ("Lunes a viernes 9:00 a 14:00 y 16:00 a 19:00. Sábado 9:00 a 13:00.",
                          "Monday to Friday 9 am to 2 pm and 4 pm to 7 pm. Saturday 9 am to 1 pm.")),
                     [inicio, catalogo, entrega, cont])
