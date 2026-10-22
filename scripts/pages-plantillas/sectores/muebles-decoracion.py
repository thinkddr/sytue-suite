# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
from lib import *

KEY = "muebles-decoracion"

# Paleta propia: gris azulado de pared recién pintada con un rojo teja de cerámica y madera.
PARED = dict(colores=dict(background="#e9eef2", surface="#d9e2ea", text="#1b232b", muted="#44525e",
                          primary="#86392b", on_primary="#ffffff", border="#c2cfd9", inverse="#1f2a33",
                          on_inverse="#eef2f5"),
             fuentes=("noto-serif", "manrope"), peso=600, h1=(46, 38, 30), radio=(2, 6, 10))

NOTA = ("Precios de ejemplo: cámbialos por los tuyos. Incluyen IVA.", "Example prices: replace them with your own. VAT included.")


def build():
    nueva(KEY)
    look = PARED
    malos = comprobar_paleta("pared", look["colores"])
    assert not malos, malos
    nav = [("inicio", ("Inicio", "Home")), ("tienda", ("Tienda", "Shop")),
           ("servicios", ("A medida y montaje", "Made to measure and fitting")),
           ("entrega", ("Entrega y devoluciones", "Delivery and returns")), ("contacto", ("Contacto", "Contact"))]
    horas = [(("Lunes a viernes", "Monday to Friday"),
              ("10:00 a 14:00 y 17:00 a 20:30", "10:00 am to 2:00 pm and 5:00 pm to 8:30 pm")),
             (("Sábado", "Saturday"), ("10:00 a 14:00 y 17:00 a 20:30", "10:00 am to 2:00 pm and 5:00 pm to 8:30 pm")),
             (("Domingo", "Sunday"), ("Cerrado", "Closed"))]
    cta_cab = marco([llamar(t=("Llamar", "Call")), whatsapp("ghost", t=("WhatsApp", "WhatsApp"))],
                    dir="row", gap=8, wrap=True)
    a_tienda = a_pagina("tienda", "catalogo")

    inicio = pagina("inicio", ("", ""), ("Muebles y decoración", "Furniture and home decor"),
                    ("Tienda de muebles y decoración con catálogo y carrito, entrega y montaje a domicilio, proyectos a medida y asesoramiento.",
                     "Furniture and home decor shop with a catalogue and cart, home delivery and fitting, made-to-measure projects and design advice."), [
        hero(("Muebles para una casa en la que apetece estar", "Furniture for a home you want to be in"),
             ("Mira la tienda por estancias, pídelo online o ven a verlo. Te lo llevamos a casa y te lo montamos.",
              "Browse the shop by room, order online or come and see it. We deliver to your home and put it together."),
             [boton(("Ver la tienda", "Browse the shop"), a_tienda), llamar("secondary"), whatsapp("ghost")],
             eyebrow=("Muebles y decoración", "Furniture and decor"), variante="centro"),
        bloque_valores(("Por estancias", "By room"), [
            (("Salón", "Living room"), ("Sofás, mesas de centro, estanterías y muebles de tele.", "Sofas, coffee tables, shelving and TV units.")),
            (("Dormitorio", "Bedroom"), ("Camas, somieres, mesillas, cómodas y armarios.", "Beds, bases, bedside tables, chests of drawers and wardrobes.")),
            (("Comedor y cocina", "Dining room and kitchen"), ("Mesas, sillas, taburetes y aparadores.", "Tables, chairs, stools and sideboards.")),
            (("Despacho", "Home office"), ("Mesas de trabajo, sillas con respaldo ajustable y librerías.", "Desks, chairs with an adjustable back and bookcases.")),
            (("Infantil y juvenil", "Children and teens"), ("Literas, camas nido, escritorios y almacenaje.", "Bunk beds, trundle beds, desks and storage.")),
            (("Decoración", "Decor"), ("Lámparas, espejos, alfombras, cortinas, cuadros y plantas.", "Lamps, mirrors, rugs, curtains, pictures and plants.")),
        ], tono="relleno", estilo="borde", anchor="estancias",
            intro=("Cada pieza trae medidas, materiales y plazo de entrega en su ficha.", "Each piece lists its measurements, materials and delivery time on its page.")),
        bloque_productos(("Novedades", "New in"),
                         ("Lo último que ha llegado. El catálogo completo está en la página Tienda.",
                          "The latest arrivals. The full catalogue is on the Shop page."),
                         limite=4, cols=4, anchor="novedades",
                         boton_=boton(("Ver todo el catálogo", "See the whole catalogue"), a_tienda, "secondary")),
        bloque_texto(("Te lo llevamos y te lo montamos", "We deliver it and fit it"),
                     [("El mueble llega a tu casa, subido hasta la habitación, montado y con el embalaje recogido. Te avisamos el día antes con una franja horaria.",
                       "The furniture arrives at your home, carried up to the room, assembled and with the packaging taken away. We let you know the day before with a time slot."),
                      ("Las zonas y los plazos están en Entrega y devoluciones.",
                       "Delivery areas and lead times are on the Delivery and returns page.")],
                     eyebrow=("Entrega y montaje", "Delivery and fitting"),
                     imagen_=("photo", ("Dos personas montando un armario en un dormitorio", "Two people assembling a wardrobe in a bedroom")),
                     anchor="entrega-montaje"),
        bloque_valores(("Cuando no encuentras lo que buscas", "When you cannot find what you are after"), [
            (("A medida", "Made to measure"),
             ("Armarios, librerías y muebles para ese hueco que no admite uno de catálogo.", "Wardrobes, bookcases and units for that gap no catalogue piece will fit.")),
            (("Asesoramiento", "Design advice"),
             ("Te ayudamos a distribuir la estancia, elegir colores y combinar piezas que ya tienes con otras nuevas.", "We help you plan the room, pick colours and combine pieces you already own with new ones.")),
        ], cols=2, tono="relleno", estilo="relleno", anchor="medida"),
        bloque_opiniones(("Lo que dicen nuestros clientes", "What our customers say"),
                         [("el mueble", "the furniture"), ("la entrega", "the delivery"), ("el montaje", "the fitting")], tono="base"),
        bloque_cta(("¿Tienes una estancia en mente?", "Have a room in mind?"),
                   ("Cuéntanos qué quieres cambiar, mándanos una foto y las medidas, y te proponemos opciones.",
                    "Tell us what you want to change, send a photo and the measurements, and we will suggest options."), tono="relleno"),
    ])

    tienda = pagina("tienda", ("tienda", "shop"), ("Tienda", "Shop"),
                    ("Catálogo de muebles y decoración con carrito. Entrega y montaje a domicilio o recogida en tienda.",
                     "Furniture and decor catalogue with a cart. Home delivery and fitting, or in-store pickup."), [
        hero(("Tienda", "Shop"),
             ("Añade al carrito lo que te guste. En cada ficha verás medidas, materiales y cuánto tarda en llegar.",
              "Add what you like to the cart. Each page shows measurements, materials and how long it takes to arrive."),
             [boton(("Entrega y devoluciones", "Delivery and returns"), a_pagina("entrega", "plazos"), "secondary"), whatsapp("ghost")],
             variante="texto"),
        bloque_productos(("Todo el catálogo", "Full catalogue"), None, limite=24, cols=4, anchor="catalogo"),
        bloque_checks(("Antes de comprar", "Before you buy"), [
            ("Mide el hueco y la puerta por la que tiene que entrar el mueble.", "Measure the space and the door the piece has to get through."),
            ("Cada ficha indica el plazo: en tienda, bajo pedido o a medida.", "Each page gives the lead time: in stock, made to order or made to measure."),
            ("Puedes ver el mueble en la tienda antes de comprarlo. Avísanos y lo tenemos preparado.", "You can see the piece in the shop before buying. Let us know and we will have it ready."),
            ("Devolución en 14 días desde que lo recibes, con las condiciones de la página de devoluciones.", "Returns within 14 days of receiving it, under the conditions on the returns page.")],
            eyebrow=("Compra con calma", "Shop calmly"), tono="relleno", anchor="antes"),
        bloque_cta(("¿No encuentras tu medida o tu color?", "Cannot find your size or colour?"),
                   ("Algunos muebles se fabrican en otros acabados. Pregúntanos y te decimos qué se puede hacer.",
                    "Some pieces are made in other finishes. Ask us and we will tell you what is possible."),
                   [whatsapp("primary"), llamar("ghost")], tono="base"),
    ])

    serv = pagina("servicios", ("servicios", "services"), ("A medida y montaje", "Made to measure and fitting"),
                  ("Muebles a medida, asesoramiento de decoración y servicio de montaje con tarifas.",
                   "Made-to-measure furniture, decor advice and a fitting service with rates."), [
        hero(("Muebles a medida, asesoramiento y montaje", "Made-to-measure furniture, advice and fitting"),
             ("Para lo que no viene en catálogo, y para que lo que compres quede bien puesto.",
              "For what is not in the catalogue, and so what you buy ends up properly in place."),
             [boton(("Pedir una visita", "Request a visit"), a_pagina("contacto", "contacto")), whatsapp("ghost")], variante="texto"),
        bloque_pasos(("Un proyecto a medida, paso a paso", "A made-to-measure project, step by step"), [
            (("Hablamos y medimos", "We talk and measure"),
             ("Vamos a tu casa, vemos el hueco y apuntamos medidas, enchufes y lo que quieras guardar.", "We come to your home, see the space and note measurements, sockets and what you need to store.")),
            (("Te enseñamos el diseño", "We show you the design"),
             ("Un plano con medidas, materiales y acabados, y el presupuesto cerrado.", "A plan with measurements, materials and finishes, and a fixed quote.")),
            (("Se fabrica", "It is made"),
             ("Cuando aceptas, se pide una señal y se fabrica. El plazo va en el presupuesto.", "Once you accept, a deposit is taken and it is made. The lead time is on the quote.")),
            (("Lo instalamos", "We install it"),
             ("Entrega, montaje y ajuste de puertas y cajones. Retiramos el embalaje.", "Delivery, assembly and adjusting doors and drawers. We take away the packaging.")),
        ], intro=("Armarios empotrados, librerías, vestidores y muebles de entrada o de baño.",
                  "Built-in wardrobes, bookcases, dressing rooms and hall or bathroom units."), tono="base", anchor="a-medida"),
        bloque_texto(("Asesoramiento de decoración", "Decor advice"),
                     [("Si no sabes por dónde empezar, venimos a tu casa o hablamos en la tienda. Revisamos la estancia, su luz y lo que ya tienes, y te proponemos una distribución y una paleta de colores con piezas del catálogo.",
                       "If you do not know where to start, we come to your home or talk in the shop. We look at the room, its light and what you already own, and propose a layout and a colour palette using pieces from the catalogue."),
                      ("Te damos un plano sencillo y una lista de lo que hace falta. Tú decides qué comprar y dónde.",
                       "We give you a simple plan and a list of what is needed. You decide what to buy and where.")],
                     eyebrow=("Decoración", "Decor"), tono="relleno", invertir=True,
                     imagen_=("photo", ("Salón con sofá, alfombra y lámpara de pie recién decorado", "Living room with a sofa, rug and floor lamp, freshly decorated")),
                     anchor="asesoramiento"),
        bloque_precios(("Tarifas de montaje y asesoramiento", "Fitting and advice rates"), ("Tarifas de montaje y visitas", "Fitting and visit rates"),
                       [("Servicio", "Service"), ("Precio", "Price")],
                       [(("Montaje de una cama o una mesa", "Assembling a bed or a table"), eur(35)),
                        (("Montaje de un armario (hasta 2 metros)", "Assembling a wardrobe (up to 2 metres)"), desde(60)),
                        (("Montaje de un armario de más de 2 metros o de puertas correderas", "Assembling a wardrobe over 2 metres or with sliding doors"), desde(95)),
                        (("Colgar estantes, espejos y cuadros (por hora)", "Hanging shelves, mirrors and pictures (per hour)"), eur(30)),
                        (("Retirada del mueble viejo", "Removing the old furniture"), desde(30)),
                        (("Visita para medir y asesorar", "Visit to measure and advise"), eur(40))],
                       intro=("El montaje de lo que compras aquí se calcula al hacer el pedido. Si ya tienes el mueble, también lo montamos.",
                              "Fitting of what you buy here is quoted when you order. If you already own the piece, we will assemble it too."),
                       nota=NOTA, anchor="montaje", tono="base"),
        bloque_faq(("Preguntas frecuentes", "Frequently asked questions"), [
            (("¿Cuánto cuesta un mueble a medida?", "How much does a made-to-measure piece cost?"),
             ("Depende del tamaño, el material y los herrajes. Tras la visita te damos un presupuesto cerrado, sin compromiso.",
              "It depends on size, material and fittings. After the visit we give you a fixed quote, with no obligation.")),
            (("¿Puedo ver las muestras de materiales?", "Can I see the material samples?"),
             ("Sí. En la tienda tenemos muestras de maderas, tableros y tiradores, y las llevamos a la visita.",
              "Yes. We have samples of woods, boards and handles in the shop, and bring them to the visit.")),
            (("¿Montáis también muebles de otras tiendas?", "Do you also assemble furniture from other shops?"),
             ("Sí, con un precio por hora o por mueble. Mándanos el modelo y las instrucciones y te decimos cuánto.",
              "Yes, at an hourly or per-piece price. Send us the model and the instructions and we will tell you the cost.")),
        ], tono="relleno"),
    ])

    entrega = pagina("entrega", ("entrega", "delivery"), ("Entrega y devoluciones", "Delivery and returns"),
                     ("Zonas, costes y plazos de entrega a domicilio, recogida en tienda y cómo devolver un mueble.",
                      "Delivery areas, costs and lead times to your door, in-store pickup and how to return a piece."), [
        hero(("Entrega y devoluciones", "Delivery and returns"),
             ("Cuánto tarda en llegar, a qué zonas vamos y qué pasa si el mueble no es lo que esperabas.",
              "How long it takes to arrive, which areas we cover and what happens if the piece is not what you expected."),
             [boton(("Ir a devoluciones", "Go to returns"), a_ancla("devoluciones"), "secondary"), whatsapp("ghost")],
             variante="texto"),
        bloque_precios(("Zonas de entrega", "Delivery areas"), ("Zonas de entrega a domicilio y costes", "Home delivery areas and costs"),
                       [("Zona", "Area"), ("Plazo tras tener el mueble", "Time once we have the piece"), ("Entrega con montaje", "Delivery with fitting")],
                       [(("Tu ciudad y alrededores (hasta 10 km)", "Your city and surroundings (up to 10 km)"),
                         ("2 a 3 días laborables", "2 to 3 working days"), eur(35)),
                        (("Municipios a menos de 40 km", "Towns within 40 km"),
                         ("3 a 5 días laborables", "3 to 5 working days"), eur(55)),
                        (("Resto de la provincia", "Rest of the province"),
                         ("3 a 6 días laborables", "3 to 6 working days"), eur(80)),
                        (("Otras provincias", "Other provinces"),
                         ("Según destino", "Depends on destination"), ("Presupuesto sin compromiso", "No-obligation quote")),
                        (("Recogida en tienda", "In-store pickup"), ("Cuando esté listo", "When it is ready"), ("Gratis", "Free"))],
                       intro=("Subimos el mueble hasta la habitación y retiramos el embalaje. Si no cabe por la escalera o el ascensor, avísanos al pedirlo.",
                              "We carry the piece up to the room and take away the packaging. If it will not fit the stairs or lift, tell us when you order."),
                       nota=("Zonas y precios de ejemplo: cámbialos por los tuyos. Incluyen IVA.", "Example areas and prices: replace them with your own. VAT included."),
                       anchor="zonas", tono="base"),
        bloque_precios(("Plazos", "Lead times"), ("Plazos de entrega según el tipo de pedido", "Lead times by type of order"),
                       [("Tipo de pedido", "Type of order"), ("Plazo habitual", "Usual lead time")],
                       [(("Mueble en tienda", "Piece in stock"), ("3 a 5 días laborables", "3 to 5 working days")),
                        (("Bajo pedido al fabricante", "Ordered from the maker"), ("4 a 8 semanas", "4 to 8 weeks")),
                        (("A medida", "Made to measure"), ("6 a 10 semanas", "6 to 10 weeks")),
                        (("Tapizados con tela a elegir", "Upholstery with a fabric of your choice"), ("6 a 12 semanas", "6 to 12 weeks"))],
                       intro=("El plazo exacto va en la ficha del mueble y en tu confirmación de pedido. Si cambia, te avisamos.",
                              "The exact time is on the piece's page and in your order confirmation. If it changes, we let you know."),
                       nota=NOTA, anchor="plazos", tono="relleno"),
        bloque_pasos(("Devoluciones", "Returns"), [
            (("Avísanos en 14 días", "Tell us within 14 days"),
             ("Escríbenos o llámanos en los 14 días siguientes a recibir el pedido.", "Message or call us within 14 days of receiving your order.")),
            (("Déjalo como lo recibiste", "Leave it as you received it"),
             ("Sin usar, sin montar o desmontado, y con el embalaje original si es posible.", "Unused, unassembled or taken apart, and in the original packaging if possible.")),
            (("Lo recogemos o lo traes", "We collect it or you bring it"),
             ("En tienda no cuesta nada. A domicilio, la recogida tiene un coste que te decimos antes.", "In the shop it costs nothing. From your home, collection has a cost we tell you beforehand.")),
            (("Te devolvemos el dinero", "We refund you"),
             ("Por el mismo medio de pago, en un máximo de 14 días desde que recibimos el mueble.", "By the same payment method, within 14 days at most of getting the piece back.")),
        ], intro=("Los muebles a medida y los hechos con una tela o un acabado elegido por ti no tienen devolución, salvo que lleguen con un defecto. Si llega roto o con un defecto, escríbenos con una foto.",
                  "Pieces made to measure or in a fabric or finish you chose cannot be returned, unless they arrive faulty. If it arrives broken or faulty, message us with a photo."),
            tono="base", anchor="devoluciones"),
        bloque_faq(("Preguntas frecuentes", "Frequently asked questions"), [
            (("¿Tengo que estar en casa?", "Do I need to be at home?"),
             ("Sí, una persona adulta. Te damos una franja de dos horas el día antes y te avisamos cuando estemos cerca.",
              "Yes, an adult. We give you a two-hour slot the day before and let you know when we are close.")),
            (("¿Y si no cabe por la puerta?", "What if it does not fit through the door?"),
             ("Dínoslo al pedirlo y lo miramos: algunos muebles se entregan desmontados y los montamos dentro.",
              "Tell us when you order and we will look into it: some pieces are delivered flat-packed and we assemble them inside.")),
            (("¿Cómo pago?", "How do I pay?"),
             ("Online con tarjeta. En tienda, con tarjeta o en efectivo. Los muebles a medida llevan una señal.",
              "Online by card. In the shop, by card or cash. Made-to-measure pieces take a deposit.")),
        ], tono="relleno"),
    ])

    cont = pagina("contacto", ("contacto", "contact"), ("Contacto", "Contact"),
                  ("Horario de la tienda de muebles, cómo llegar, teléfono, WhatsApp y formulario para pedidos, medidas y visitas.",
                   "Furniture shop hours, how to get here, phone, WhatsApp and a form for orders, measurements and visits."), [
        hero(("Contacto", "Contact"),
             ("Ven a la tienda, llámanos o escríbenos por WhatsApp. Para un proyecto, una foto y las medidas ayudan mucho.",
              "Come to the shop, call us or message us on WhatsApp. For a project, a photo and the measurements help a lot."),
             [llamar(), whatsapp()], variante="texto"),
        bloque_contacto(("Estamos en la tienda", "We are at the shop"),
                        ("Si ya has hecho un pedido, dinos el número que te llegó por correo.",
                         "If you have already placed an order, tell us the number you received by email."),
                        campos_consulta(servicios=[("Un pedido", "An order"), ("Un proyecto a medida", "A made-to-measure project"),
                                                   ("Asesoramiento de decoración", "Decor advice"), ("Montaje", "Fitting"),
                                                   ("Otra cosa", "Something else")]),
                        ("Enviar consulta", "Send enquiry"), horas,
                        notas=("Aparcamiento y carga y descarga delante de la tienda.", "Parking and loading right outside the shop.")),
        bloque_mapa(("Cómo llegar a la tienda", "How to get to the shop"), tono="relleno"),
    ])

    return plantilla(KEY, ("Muebles y decoración", "Furniture and home decor"),
                     ("Tienda de muebles con catálogo y carrito por estancias, entrega y montaje a domicilio, proyectos a medida, asesoramiento y devoluciones.",
                      "Furniture shop with a catalogue and cart by room, home delivery and fitting, made-to-measure projects, design advice and returns."),
                     "comercio", ("muebles decoracion tienda de muebles mueblería muebleria sofas armarios camas decoracion del hogar mueble a medida montaje entrega a domicilio tienda online",
                                  "furniture shop furniture store home decor home decoration sofas wardrobes beds made to measure delivery fitting online shop"),
                     look, cabecera(nav, cta_cab, tienda=True),
                     pie(nav, ("Muebles y decoración para el hogar, con entrega y montaje.", "Furniture and home decor, with delivery and fitting."),
                         ("Lunes a sábado 10:00 a 14:00 y 17:00 a 20:30.", "Monday to Saturday 10 am to 2 pm and 5 pm to 8:30 pm.")),
                     [inicio, tienda, serv, entrega, cont], tienda=True)
