# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
from lib import *

KEY = "pescaderia-marisqueria"

# Hielo y salmón: fondo azul muy pálido de caja de hielo y un rojo salmón. Sin fondos oscuros (primary/inverse < 3).
HIELO = dict(
    colores=dict(background="#f1f6f8", surface="#e1ebf0", text="#10222d", muted="#465a66", primary="#b3402a",
                 on_primary="#ffffff", border="#cbdae2", inverse="#12303f", on_inverse="#eaf4f8"),
    fuentes=("figtree", "ibm-plex-sans"), peso=700, radio=(6, 10, 14), ancho=1120)
_malos = comprobar_paleta("hielo", HIELO["colores"])
assert not _malos, _malos

NOTA_PRECIOS = ("Precios de ejemplo: cámbialos por los tuyos. Incluyen IVA. El precio final depende del peso y del día.",
                "Example prices: replace them with your own. VAT included. The final price depends on the weight and the day.")
KILO = [("Producto", "Product"), ("Precio por kilo", "Price per kilo")]


def sec_form(titulo, intro, campos, envio, exito, anchor, tono="base"):
    return seccion("contact", [titular(titulo, intro),
                               tarjeta([formulario(campos, envio, exito)], estilo="relleno" if tono == "base" else "borde",
                                       pad=28, tag="div", sz=tam(max_width=640))],
                   tono=tono, anchor=anchor, gap=28)


def campos_encargo():
    return [campo("nombre", "text", ("Nombre", "Name"), True, auto="name", maxlen=120),
            campo("telefono", "tel", ("Teléfono", "Phone"), True, auto="tel", maxlen=30),
            campo("fecha", "date", ("Fecha de recogida", "Pick-up date"), True),
            campo("hora", "select", ("Hora de recogida", "Pick-up time"), False,
                  opciones=[("Por la mañana", "In the morning"), ("Por la tarde", "In the afternoon")]),
            campo("ocasion", "select", ("Para qué es", "What it is for"), False,
                  opciones=[("Comida del domingo", "Sunday lunch"), ("Arroz o paella", "Rice dish or paella"),
                            ("Celebración o fiesta", "Celebration or party"), ("Otra", "Something else")]),
            campo("comensales", "select", ("Para cuántas personas", "For how many people"), False,
                  opciones=[("Hasta 4", "Up to 4"), ("De 5 a 8", "5 to 8"), ("De 9 a 15", "9 to 15"), ("Más de 15", "More than 15")]),
            campo("pedido", "textarea", ("Qué necesitas y cómo lo quieres (entero, limpio, en filetes...)",
                                         "What you need and how you want it (whole, cleaned, filleted...)"), True, maxlen=1500)]


def build():
    nueva(KEY)
    look = HIELO
    nav = [("inicio", ("Inicio", "Home")), ("pescado", ("Pescado y marisco", "Fish and shellfish")),
           ("encargos", ("Encargos", "Orders")), ("contacto", ("Contacto", "Contact"))]
    horas = [(("Lunes", "Monday"), ("Cerrado", "Closed")),
             (("Martes a viernes", "Tuesday to Friday"), ("8:00 a 14:00 y de 17:00 a 20:00", "8:00 am to 2:00 pm and 5:00 pm to 8:00 pm")),
             (("Sábado", "Saturday"), ("8:00 a 14:30", "8:00 am to 2:30 pm")),
             (("Domingo", "Sunday"), ("Cerrado", "Closed"))]
    cta_cab = marco([llamar(), whatsapp("ghost")], dir="row", gap=8, wrap=True, align="center")
    a_encargo = a_pagina("encargos", "hacer-encargo")

    inicio = pagina("inicio", ("", ""), ("Pescadería y marisquería", "Fishmonger's and shellfish shop"),
                    ("Pescadería y marisquería de barrio: pescado y marisco fresco, limpieza y fileteado sin coste, encargos con fecha de recogida y botones de llamar y WhatsApp.",
                     "Neighbourhood fishmonger's and shellfish shop: fresh fish and shellfish, free cleaning and filleting, orders with a collection date and call and WhatsApp buttons."), [
        hero(("Pescado y marisco del día, limpio y listo para cocinar", "Fish and shellfish of the day, cleaned and ready to cook"),
             ("Pasa por el mostrador, dinos qué vas a cocinar y cómo lo quieres, y te lo limpiamos y fileteamos sin coste. Para celebraciones, encarga con fecha de recogida.",
              "Drop by the counter, tell us what you are cooking and how you want it, and we clean and fillet it free of charge. For celebrations, order with a collection date."),
             [boton(("Hacer un encargo", "Place an order"), a_encargo), llamar("secondary"), whatsapp("ghost")],
             eyebrow=("Pescadería y marisquería", "Fishmonger's and shellfish shop"),
             imagen_=("photo", ("Mostrador de la pescadería con pescado fresco sobre hielo", "Fishmonger's counter with fresh fish on ice"))),
        bloque_valores(("Qué hay en el mostrador", "What is on the counter"), [
            (("Pescado blanco", "White fish"), ("Merluza, lubina, dorada, rape, lenguado y otros según el día.", "Hake, sea bass, sea bream, monkfish, sole and others depending on the day.")),
            (("Pescado azul", "Oily fish"), ("Sardina, boquerón, bonito y caballa en su momento.", "Sardines, anchovies, tuna and mackerel when they are in season.")),
            (("Marisco", "Shellfish"), ("Gamba, langostino, cigala, cangrejo y centollo, crudo o cocido.", "Prawns, king prawns, langoustines, crab and spider crab, raw or cooked.")),
            (("Moluscos", "Molluscs"), ("Mejillón, almeja, berberecho, calamar, sepia y pulpo.", "Mussels, clams, cockles, squid, cuttlefish and octopus.")),
        ], cols=4, tono="relleno", estilo="borde", anchor="familias",
            intro=("Lo que llega cambia cada día. Si buscas algo que no ves, pregúntalo y lo pedimos.",
                   "What arrives changes every day. If you are after something you cannot see, ask and we will order it in.")),
        bloque_checks(("Limpieza y fileteado, sin coste", "Cleaning and filleting, free of charge"), [
            ("Escamado y limpieza: te lo damos listo para cocinar.", "Scaling and gutting: we hand it over ready to cook."),
            ("Filetes y lomos sin espinas gordas, o el pescado entero si lo prefieres.", "Fillets and loins without the large bones, or the whole fish if you prefer."),
            ("Cabeza, espinas y recortes aparte, para hacer caldo.", "Head, bones and trimmings packed separately, for making stock."),
            ("Troceado para guiso, parrilla o horno según lo que vayas a hacer.", "Cut up for stewing, grilling or the oven, depending on what you are making."),
            ("Marisco abierto o cortado para plancha, arroz o sopa.", "Shellfish opened or cut for the griddle, rice dishes or soup."),
            ("Bolsa con hielo si vas a tardar en llegar a casa.", "A bag with ice if it will take you a while to get home."),
        ], cols=2, tono="base", anchor="servicio", eyebrow=("Servicio", "Service"),
            intro=("Es parte de lo que compras: no se cobra aparte.", "It comes with what you buy: there is no extra charge.")),
        bloque_texto(("De dónde viene lo que vendemos", "Where what we sell comes from"), [
            ("Aquí cuentas, con tus palabras, de dónde llega tu pescado y tu marisco: la lonja, el puerto, el barco o el proveedor, y a qué hora llega. Es un texto de ejemplo: cámbialo por el tuyo y no pongas nada que no sea verdad.",
             "Here you say, in your own words, where your fish and shellfish come from: the fish market, the harbour, the boat or the supplier, and what time it arrives. This is an example text: replace it with your own and do not write anything that is not true."),
            ("Todo el pescado fresco lleva su etiqueta con el nombre, la zona de captura o cría y el método de producción. Pídenos verla.",
             "All fresh fish carries its label with the name, the catch or farming area and the production method. Ask to see it.")],
            eyebrow=("Procedencia", "Origin"), tono="relleno", anchor="procedencia", invertir=True,
            imagen_=("landscape", ("Barcas amarradas en un puerto pesquero", "Boats moored in a fishing harbour"))),
        bloque_opiniones(("Lo que dicen los vecinos", "What the neighbours say"),
                         [("la frescura del pescado", "the freshness of the fish"), ("la limpieza y el fileteado", "the cleaning and filleting"),
                          ("los encargos de marisco", "the shellfish orders")], tono="base"),
        bloque_cta(("¿Qué vas a cocinar esta semana?", "What are you cooking this week?"),
                   ("Escríbenos y te decimos qué ha llegado y qué te conviene según el plato.",
                    "Message us and we will tell you what has arrived and what suits your dish."),
                   [boton(("Hacer un encargo", "Place an order"), a_encargo), llamar("secondary"), whatsapp("ghost")]),
    ])

    pescado = pagina("pescado", ("pescado", "fish"), ("Pescado y marisco", "Fish and shellfish"),
                     ("Calendario de temporada, precios orientativos por kilo, consejos de conservación y cocinado, alérgenos y preguntas frecuentes.",
                      "Seasonal calendar, guide prices per kilo, storage and cooking tips, allergens and frequently asked questions."), [
        hero(("Pescado y marisco de temporada", "Seasonal fish and shellfish"),
             ("Qué suele estar mejor en cada época, a cuánto va el kilo y cómo guardarlo y cocinarlo para que llegue bien a la mesa.",
              "What is usually at its best at each time of year, how much a kilo costs and how to store and cook it so it reaches the table in good shape."),
             [boton(("Encargar", "Order"), a_encargo), whatsapp("secondary")], variante="texto"),
        bloque_precios(("Calendario de temporada", "Seasonal calendar"), ("Cuándo está mejor cada pescado y marisco", "When each fish and shellfish is at its best"),
                       [("Producto", "Product"), ("Cuándo está mejor", "When it is best")],
                       [(("Merluza, lubina y dorada", "Hake, sea bass and sea bream"), ("Casi todo el año", "Most of the year")),
                        (("Mejillón y almeja", "Mussels and clams"), ("Casi todo el año", "Most of the year")),
                        (("Boquerón", "Anchovies"), ("De primavera a verano", "Spring to summer")),
                        (("Sardina", "Sardines"), ("De primavera a principios de otoño", "Spring to early autumn")),
                        (("Bonito", "Tuna"), ("De verano a principios de otoño", "Summer to early autumn")),
                        (("Pulpo", "Octopus"), ("Invierno y primavera", "Winter and spring")),
                        (("Besugo", "Red sea bream"), ("Otoño e invierno, sobre todo en Navidad", "Autumn and winter, especially at Christmas")),
                        (("Ostras", "Oysters"), ("Otoño e invierno", "Autumn and winter")),
                        (("Centollo", "Spider crab"), ("Invierno y primavera", "Winter and spring"))],
                       intro=("Es una guía general: la temporada cambia según la zona, el año y la mar.",
                              "This is a general guide: the season changes with the area, the year and the sea."),
                       nota=("Calendario de ejemplo: cámbialo por lo que de verdad llega a tu mostrador.",
                             "Example calendar: replace it with what really arrives at your counter."),
                       anchor="temporada", tono="relleno"),
        bloque_carta(("Precios orientativos por kilo", "Guide prices per kilo"), [
            (("Pescado blanco", "White fish"), ("Precios de pescado blanco", "White fish prices"), KILO,
             [(("Merluza", "Hake"), eur(16)), (("Lubina", "Sea bass"), eur(14)), (("Dorada", "Sea bream"), eur(12)),
              (("Rape", "Monkfish"), eur(22))]),
            (("Pescado azul", "Oily fish"), ("Precios de pescado azul", "Oily fish prices"), KILO,
             [(("Sardina", "Sardines"), eur(7)), (("Boquerón", "Anchovies"), eur(9)), (("Bonito", "Tuna"), eur(15))]),
            (("Marisco", "Shellfish"), ("Precios de marisco", "Shellfish prices"), KILO,
             [(("Gamba blanca", "White prawns"), desde(28)), (("Langostino", "King prawns"), eur(22)), (("Cigala", "Langoustines"), desde(40))]),
            (("Moluscos", "Molluscs"), ("Precios de moluscos", "Mollusc prices"), KILO,
             [(("Mejillón", "Mussels"), eur(3.5)), (("Almeja", "Clams"), eur(24)), (("Calamar", "Squid"), eur(13)),
              (("Pulpo", "Octopus"), eur(15))]),
        ], nota=NOTA_PRECIOS, tono="base", anchor="precios"),
        bloque_valores(("Conservación y cocinado", "Storage and cooking"), [
            (("De la tienda a la nevera", "From the shop to the fridge"),
             ("Llévalo directo a casa y guárdalo en la parte más fría de la nevera. Consúmelo el mismo día o al siguiente.",
              "Take it straight home and keep it in the coldest part of the fridge. Eat it the same day or the next.")),
            (("Marisco vivo, con cuidado", "Live shellfish, with care"),
             ("Mejillones y almejas, en un paño húmedo y sin agua dulce. Tira los que estén abiertos y no se cierren al tocarlos.",
              "Mussels and clams go in a damp cloth, never in fresh water. Throw away any that are open and do not close when touched.")),
            (("Si lo vas a comer crudo", "If you will eat it raw"),
             ("Para ceviche, marinados o sushi, el pescado tiene que congelarse antes por el anisakis. Dínoslo al comprar y te orientamos.",
              "For ceviche, marinades or sushi, the fish must be frozen first because of anisakis. Tell us when you buy and we will advise you.")),
            (("Congelar bien", "Freezing well"),
             ("Envuélvelo sin aire, anota la fecha y descongélalo en la nevera, no sobre la encimera.",
              "Wrap it with no air, write the date and thaw it in the fridge, not on the worktop.")),
            (("Cocinado justo", "Just cooked"),
             ("El pescado se pasa en un momento. Mejor poco que mucho: sigue haciéndose con el calor de la sartén.",
              "Fish overcooks in a moment. Better a little under than over: it keeps cooking in the heat of the pan.")),
            (("Cabezas y espinas, para caldo", "Heads and bones, for stock"),
             ("Pídelas: te las damos aparte y con ellas se hace un buen fumet para arroces y sopas.",
              "Ask for them: we pack them separately and they make a good fish stock for rice dishes and soups.")),
        ], cols=3, tono="relleno", estilo="borde", anchor="conservacion",
            intro=("Consejos generales. Si dudas con una pieza, pregúntanos cómo prepararla.",
                   "General tips. If you are unsure about a fish, ask us how to prepare it.")),
        bloque_texto(("Alérgenos", "Allergens"), [
            ("El pescado, los crustáceos (gamba, langostino, cangrejo) y los moluscos (mejillón, almeja, calamar) son alérgenos de declaración obligatoria. Algunos productos pueden llevar además sulfitos, gluten o huevo, como los rebozados o los elaborados.",
             "Fish, crustaceans (prawns, king prawns, crab) and molluscs (mussels, clams, squid) are allergens that must be declared. Some products may also contain sulphites, gluten or egg, such as battered items and prepared dishes."),
            ("Si alguien en casa es alérgico, díselo a quien te atienda antes de comprar: te decimos qué lleva cada producto.",
             "If anyone at home has an allergy, tell whoever serves you before buying: we will tell you what each product contains.")],
            anchor="alergenos", tono="base"),
        bloque_faq(("Preguntas frecuentes", "Frequently asked questions"), [
            (("¿Cómo sé que el pescado está fresco?", "How can I tell the fish is fresh?"),
             ("Ojo brillante y abultado, agallas rojas, piel con brillo y olor a mar, no a fuerte. Si quieres, te lo enseñamos en el mostrador.",
              "A bright, bulging eye, red gills, shiny skin and a smell of the sea, not a strong one. If you like, we will show you at the counter.")),
            (("¿Cuánto compro por persona?", "How much should I buy per person?"),
             ("Entre 150 y 200 gramos de pescado limpio, o unos 300 si lleva espina y cabeza. De marisco para picar, unos 150 gramos.",
              "Between 150 and 200 grams of cleaned fish, or about 300 with bones and head. For shellfish as a starter, about 150 grams.")),
            (("¿Me lo limpiáis y me lo fileteáis?", "Will you clean and fillet it for me?"),
             ("Sí, sin coste. Dinos cómo lo vas a cocinar y lo dejamos como te haga falta.",
              "Yes, free of charge. Tell us how you will cook it and we will leave it the way you need.")),
            (("¿Tenéis pescado congelado?", "Do you have frozen fish?"),
             ("Cuando lo tengamos, estará indicado en el mostrador. Si lo necesitas congelado para comerlo crudo, pídelo con unos días.",
              "When we have it, it will be marked at the counter. If you need it frozen to eat raw, ask a few days ahead.")),
        ], tono="relleno"),
        bloque_cta(("¿Qué necesitas esta semana?", "What do you need this week?"), None, tono="base"),
    ])

    encargos = pagina("encargos", ("encargos", "orders"), ("Encargos y celebraciones", "Orders and celebrations"),
                      ("Encargos de pescado y marisco para celebraciones: ideas, cómo funciona, formulario con fecha de recogida y qué traer.",
                       "Fish and shellfish orders for celebrations: ideas, how it works, a form with a collection date and what to bring."), [
        hero(("Encargos y celebraciones", "Orders and celebrations"),
             ("Cuéntanos qué necesitas, cuántos sois y qué día lo recoges. Lo dejamos limpio y preparado, y lo llevas sin esperar cola.",
              "Tell us what you need, how many of you there are and what day you will collect. We leave it cleaned and ready, and you take it without queuing."),
             [boton(("Rellenar el encargo", "Fill in the order"), a_ancla("hacer-encargo")), whatsapp("secondary")], variante="texto"),
        bloque_valores(("Ideas para encargar", "Ideas to order"), [
            (("Bandeja de marisco", "Shellfish platter"),
             ("Gamba, langostino, cigala o cangrejo, cocido o crudo, para la mesa de las fiestas.", "Prawns, king prawns, langoustines or crab, cooked or raw, for the holiday table.")),
            (("Pescado entero al horno", "Whole fish in the oven"),
             ("Besugo, lubina o dorada de ración grande, limpio y listo para el horno.", "A large-portion red sea bream, sea bass or sea bream, cleaned and ready for the oven.")),
            (("Para el arroz o la paella", "For a rice dish or paella"),
             ("Marisco, sepia o calamar en la cantidad justa y el fumet con cabezas y espinas.", "Shellfish, cuttlefish or squid in the right amount and the stock made from heads and bones.")),
            (("Comida de empresa o de familia", "Company or family meal"),
             ("Cantidades para muchos comensales: avísanos con tiempo y reservamos las piezas.", "Amounts for many guests: let us know in good time and we set the pieces aside.")),
        ], cols=2, tono="relleno", estilo="borde", anchor="ideas"),
        bloque_pasos(("Cómo funciona", "How it works"), [
            (("Pides", "You ask"), ("Con el formulario, por WhatsApp o por teléfono.", "With the form, by WhatsApp or by phone.")),
            (("Confirmamos", "We confirm"), ("Te decimos si hay lo que pides y el precio aproximado, que depende del peso.", "We tell you whether we have what you ask for and the approximate price, which depends on the weight.")),
            (("Preparamos", "We prepare"), ("El día de la recogida lo limpiamos, lo cortamos y lo envasamos con hielo.", "On the collection day we clean it, cut it and pack it with ice.")),
            (("Recoges y pagas", "You collect and pay"), ("En la tienda, en efectivo o con tarjeta.", "At the shop, in cash or by card.")),
        ], tono="base"),
        sec_form(("Haz tu encargo", "Place your order"),
                 ("Escribe lo que necesitas y el día que vendrás. Para Navidad y fiestas, encarga con una semana de margen.",
                  "Write what you need and the day you will come. For Christmas and holidays, order a week ahead."),
                 campos_encargo(), ("Enviar encargo", "Send order"),
                 ("Gracias. Hemos recibido tu encargo y te lo confirmamos lo antes posible.",
                  "Thank you. We have received your order and will confirm it as soon as possible."),
                 "hacer-encargo", tono="relleno"),
        bloque_checks(("Antes de recoger", "Before you collect"), [
            ("Dinos tu nombre: el encargo está a tu nombre.", "Tell us your name: the order is in your name."),
            ("Si cambia el número de comensales, avísanos un día antes para ajustar el peso.", "If the number of guests changes, tell us a day ahead so we can adjust the weight."),
            ("Para un trayecto largo, pide una bolsa isotérmica o trae la tuya.", "For a long journey, ask for an insulated bag or bring your own."),
            ("Si alguien en casa tiene alergias, díselo a quien te atienda al encargar.", "If anyone at home has allergies, tell whoever serves you when you order."),
        ], cols=2, tono="base"),
    ])

    cont = pagina("contacto", ("contacto", "contact"), ("Contacto y cómo llegar", "Contact and directions"),
                  ("Teléfono, WhatsApp, formulario, horario del mostrador y mapa.",
                   "Phone, WhatsApp, form, counter hours and map."), [
        hero(("Contacto", "Contact"),
             ("Llámanos, escríbenos por WhatsApp o pasa por el mostrador. Para encargos, mira la página de encargos.",
              "Call us, message us on WhatsApp or drop by the counter. For orders, see the orders page."),
             [llamar(), whatsapp()], variante="texto"),
        bloque_contacto(("Dónde estamos", "Where we are"),
                        ("Pasa por el mostrador o escríbenos. Para consultas rápidas, lo más ágil es WhatsApp.",
                         "Drop by the counter or write to us. For quick questions, WhatsApp is quickest."),
                        campos_cita(servicios=[("Hacer un encargo", "Place an order"), ("Una pregunta", "A question"), ("Otra cosa", "Something else")],
                                    fecha=False),
                        ("Enviar mensaje", "Send message"), horas,
                        notas=("Los festivos pueden cambiar el horario: lo avisamos en el mostrador.",
                               "Holidays may change the hours: we put a notice on the counter.")),
        bloque_mapa(tono="relleno"),
    ])

    return plantilla(KEY, ("Pescadería y marisquería", "Fishmonger's and shellfish shop"),
                     ("Pescado y marisco de temporada, precios orientativos, limpieza y fileteado sin coste, encargos con fecha de recogida, alérgenos, horario y mapa, con botones de llamar y WhatsApp.",
                      "Seasonal fish and shellfish, guide prices, free cleaning and filleting, orders with a collection date, allergens, hours and a map, with call and WhatsApp buttons."),
                     "comercio", ("pescaderia marisqueria pescadero pescado marisco mariscos gambas mejillones lonja tienda de barrio encargos fresco",
                                  "fishmonger fishmongers fish shop seafood shellfish prawns mussels fresh fish orders"),
                     look, cabecera(nav, cta_cab),
                     pie(nav, ("Pescadería y marisquería. Limpieza y fileteado sin coste y encargos para celebraciones.",
                               "Fishmonger's and shellfish shop. Free cleaning and filleting and orders for celebrations."),
                         ("Martes a viernes 8:00 a 14:00 y 17:00 a 20:00. Sábado 8:00 a 14:30.",
                          "Tuesday to Friday 8 am to 2 pm and 5 pm to 8 pm. Saturday 8 am to 2:30 pm.")),
                     [inicio, pescado, encargos, cont])
