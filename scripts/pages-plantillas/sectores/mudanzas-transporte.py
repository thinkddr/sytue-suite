# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
from lib import *
from looks import LOOKS

KEY = "mudanzas-transporte"

# Paleta propia (color cartón y marrón tabaco); no usa fondo $inverse. Se comprueba AA al construir.
CARTON = dict(background="#faf5ec", surface="#efe4d0", text="#231a10", muted="#5e5140", primary="#7c4a03",
              on_primary="#ffffff", border="#decfb3", inverse="#2a1f12", on_inverse="#f8efdf")


def pedir(t=("Contar mi mudanza", "Tell us about my move")):
    return boton(t, a_pagina("contacto", "contacto"))


def build():
    malos = comprobar_paleta("carton", CARTON)
    assert not malos, malos
    nueva(KEY)
    look = dict(colores=CARTON, fuentes=("montserrat", "noto-sans"), peso=800, h1=(48, 38, 30), radio=(3, 6, 10))
    nav = [("inicio", ("Inicio", "Home")), ("servicios", ("Servicios y precios", "Services and prices")),
           ("contacto", ("Cuéntanos tu mudanza", "Tell us about your move"))]
    horas = [(("Lunes a viernes", "Monday to Friday"), ("8:00 a 19:00", "8:00 am to 7:00 pm")),
             (("Sábado", "Saturday"), ("9:00 a 14:00", "9:00 am to 2:00 pm")),
             (("Domingo", "Sunday"), ("Cerrado", "Closed"))]
    cta_cab = marco([llamar(t=("Llamar", "Call")), whatsapp("secondary", t=("WhatsApp", "WhatsApp"))],
                    dir="row", gap=8, wrap=True)

    inicio = pagina("inicio", ("", ""), ("Mudanzas y transporte", "Removals and transport"),
                    ("Mudanzas locales y nacionales, guardamuebles, embalaje, montaje de muebles y transporte de objetos. Cuéntanos tu mudanza y te damos precio.",
                     "Local and long-distance removals, storage, packing, furniture assembly and transport of single items. Tell us about your move and we give you a price."), [
        hero(("Tu mudanza, sin que se te rompa nada", "Your move, with nothing broken"),
             ("Mudanzas en tu ciudad y a toda España, con embalaje, montaje de muebles y guardamuebles. Cuéntanos qué te llevas y te damos precio antes de mover una caja.",
              "Removals in your town and across Spain, with packing, furniture assembly and storage. Tell us what you are taking and we give you a price before a box is moved."),
             [pedir(), llamar("secondary")], variante="centro",
             eyebrow=("Mudanzas y transporte", "Removals and transport")),
        bloque_pasos(("Cómo funciona una mudanza", "How a move works"), [
            (("Nos la cuentas", "You tell us about it"),
             ("Rellenas el formulario o nos llamas: de dónde a dónde, qué fecha y cuánto hay que llevar.",
              "You fill in the form or call us: from where to where, which date and how much there is to move.")),
            (("Te damos precio", "We give you a price"),
             ("Con fotos o con una visita, según el tamaño. Por escrito y con lo que incluye.",
              "With photos or a visit, depending on the size. In writing and with what is included.")),
            (("El día de la mudanza", "Moving day"),
             ("Llegamos a la hora, protegemos muebles y suelos, cargamos y transportamos.",
              "We arrive on time, protect furniture and floors, load and transport.")),
            (("Descargamos y montamos", "We unload and assemble"),
             ("Dejamos cada cosa en su habitación y montamos los muebles que desmontamos.",
              "We put each thing in its room and reassemble the furniture we took apart.")),
        ], tono="relleno"),
        bloque_valores(("Lo que movemos", "What we move"), [
            (("Mudanzas locales", "Local removals"),
             ("De un piso a otro en tu ciudad, normalmente en un solo día y con precio por horas.",
              "From one flat to another in your town, usually in a single day and priced by the hour.")),
            (("Mudanzas nacionales", "Long-distance removals"),
             ("A cualquier punto de España, con precio cerrado según el volumen y la distancia.",
              "To anywhere in Spain, at a fixed price based on volume and distance.")),
            (("Guardamuebles", "Storage"),
             ("Guardamos tus muebles y cajas en un almacén cerrado por días, meses o lo que necesites.",
              "We store your furniture and boxes in a locked warehouse for days, months or as long as you need.")),
            (("Embalaje", "Packing"),
             ("Ponemos cajas, papel, film y protección. Embalamos nosotros o te dejamos el material.",
              "We supply boxes, paper, wrap and protection. We pack for you or leave you the materials.")),
            (("Montaje de muebles", "Furniture assembly"),
             ("Desmontamos y montamos armarios, camas, mesas y estanterías.",
              "We take apart and put together wardrobes, beds, tables and shelves.")),
            (("Transporte de objetos", "Single-item transport"),
             ("Un sofá, una lavadora, una bici o lo que has comprado de segunda mano: sin mudanza completa.",
              "A sofa, a washing machine, a bike or something you bought second-hand: no full move needed.")),
        ], eyebrow=("Servicios", "Services"), estilo="borde", anchor="servicios-resumen"),
        bloque_checks(("Lo que hacemos siempre", "What we always do"), [
            ("Protegemos suelos, puertas y ascensores con mantas y cartón.", "We protect floors, doors and lifts with blankets and cardboard."),
            ("Embalamos los muebles delicados con film y esquineros.", "We wrap delicate furniture with film and corner guards."),
            ("Llevamos carretillas, correas y herramientas para desmontar.", "We bring trolleys, straps and tools for taking furniture apart."),
            ("Te avisamos si algo no cabe por la puerta o por la escalera antes del día.", "We tell you if something will not fit through the door or stairs before the day."),
        ], eyebrow=("El día de la mudanza", "On the day"), tono="base", cols=2),
        bloque_opiniones(("Lo que dicen nuestros clientes", "What our customers say"),
                         [("el cuidado de los muebles", "the care of the furniture"), ("la puntualidad", "the punctuality"), ("el precio acordado", "the agreed price")],
                         tono="relleno"),
        bloque_cta(("¿Cuándo te mudas?", "When are you moving?"),
                   ("Cuéntanos la fecha y de dónde a dónde y te damos precio.", "Tell us the date and where from and to and we will give you a price."),
                   [pedir(), whatsapp("ghost")], tono="base"),
    ])

    serv = pagina("servicios", ("servicios", "services"), ("Servicios y precios", "Services and prices"),
                  ("Mudanzas locales y nacionales, guardamuebles, embalaje, montaje y transporte de objetos, con precios por hora de ejemplo y seguro de la carga.",
                   "Local and long-distance removals, storage, packing, assembly and single-item transport, with example hourly prices and cargo insurance."), [
        hero(("Servicios y precios", "Services and prices"),
             ("Una mudanza local se cobra por horas. Una nacional, con precio cerrado. Aquí tienes cómo trabajamos cada una.",
              "A local move is charged by the hour. A long-distance one has a fixed price. Here is how we work on each."),
             [pedir(), whatsapp()], variante="texto"),
        bloque_texto(("Mudanzas locales", "Local removals"), [
            ("Vienen un camión o una furgoneta y varios mozos, según el tamaño de la vivienda. Pagas por las horas que dura la mudanza, contadas desde que empezamos a cargar hasta que terminamos de descargar.",
             "A lorry or van and several movers come, depending on the size of the home. You pay for the hours the move takes, counted from when we start loading until we finish unloading."),
            ("Antes de empezar te decimos cuántas horas calculamos, para que sepas el orden de magnitud.",
             "Before we start we tell you how many hours we expect, so you know the order of magnitude.")],
            eyebrow=("Dentro de la misma ciudad", "Within the same town"), anchor="locales",
            imagen_=("photo", ("Dos mozos cargando un armario en un camión de mudanzas", "Two movers loading a wardrobe into a removals lorry"))),
        bloque_texto(("Mudanzas nacionales", "Long-distance removals"), [
            ("Te damos un precio cerrado según el volumen de lo que llevas y los kilómetros. Pasamos a verlo o nos mandas fotos de cada habitación.",
             "We give you a fixed price based on the volume of what you are taking and the kilometres. We come to see it or you send us photos of each room."),
            ("Te decimos el día de carga y el día estimado de entrega, y te avisamos si hay algún cambio.",
             "We tell you the loading day and the estimated delivery day, and let you know if anything changes.")],
            eyebrow=("A otra ciudad", "To another city"), anchor="nacionales", tono="relleno", invertir=True,
            imagen_=("landscape", ("Camión de mudanzas en una carretera de montaña", "Removals lorry on a mountain road"))),
        bloque_checks(("Embalaje y montaje de muebles", "Packing and furniture assembly"), [
            ("Cajas, papel, burbujas, film y cinta, a precio de coste o incluidos según el servicio.", "Boxes, paper, bubble wrap, film and tape, at cost or included depending on the service."),
            ("Embalamos nosotros, o te dejamos las cajas y empaquetas tú.", "We pack for you, or leave the boxes for you to pack."),
            ("Cajas de ropa colgada para no arrugar.", "Wardrobe boxes so clothes do not crease."),
            ("Desmontamos camas, armarios, mesas y estanterías, y los montamos en destino.", "We take beds, wardrobes, tables and shelves apart and reassemble them at the destination."),
            ("Etiquetamos cada caja por habitación para que la descarga sea rápida.", "We label each box by room so unloading is quick."),
        ], eyebrow=("Si no quieres embalar tú", "If you would rather not pack"), anchor="embalaje", tono="base"),
        bloque_valores(("Guardamuebles y transporte de objetos", "Storage and single-item transport"), [
            (("Guardamuebles", "Storage"),
             ("Almacén cerrado, en cajas o con los muebles protegidos, por el tiempo que lo necesites. Pagas por metro cuadrado y mes.",
              "A locked warehouse, in boxes or with furniture protected, for as long as you need it. You pay by the square metre per month.")),
            (("Transporte de objetos", "Single-item transport"),
             ("Una pieza o pocas, de una dirección a otra. Te damos precio por teléfono con una foto y las medidas.",
              "One piece or a few, from one address to another. We give you a price by phone from a photo and the measurements.")),
        ], cols=2, estilo="relleno", tono="relleno", anchor="guardamuebles"),
        bloque_precios(("Precios por hora", "Hourly prices"), ("Precios de mudanza por hora", "Removals prices per hour"),
                       [("Equipo", "Crew"), ("Precio", "Price")],
                       [(("Furgoneta y 1 mozo", "Van and 1 mover"), desde(45)),
                        (("Furgoneta y 2 mozos", "Van and 2 movers"), desde(65)),
                        (("Camión y 2 mozos", "Lorry and 2 movers"), desde(85)),
                        (("Camión y 3 mozos", "Lorry and 3 movers"), desde(110)),
                        (("Desmontaje y montaje de muebles (por operario y hora)", "Furniture assembly and disassembly (per worker per hour)"), desde(28)),
                        (("Guardamuebles (por metro cuadrado y mes)", "Storage (per square metre per month)"), desde(14)),
                        (("Mudanza nacional", "Long-distance removal"), ("Presupuesto cerrado", "Fixed quote"))],
                       intro=("Las mudanzas locales se cobran por hora. El precio exacto depende de las plantas, el ascensor y el aparcamiento.",
                              "Local moves are charged by the hour. The exact price depends on floors, lift and parking."),
                       nota=("Precios de ejemplo: cámbialos por los tuyos. Incluyen IVA. El material de embalaje se cobra aparte.",
                             "Example prices: replace them with your own. VAT included. Packing materials are charged separately."),
                       anchor="precios", tono="base"),
        bloque_texto(("Seguro de la carga", "Cargo insurance"), [
            ("Indica aquí el nombre de la aseguradora, qué cubre la póliza de la carga y de los daños a terceros, y hasta qué importe. Pon solo lo que consta en tu póliza.",
             "State here the insurer's name, what the policy covers for the cargo and for damage to third parties, and up to what amount. Put only what is in your policy."),
            ("Antes de contratar te enseñamos la póliza y te decimos cómo se tramita si algo se daña.",
             "Before you book we show you the policy and tell you how a claim is made if something gets damaged.")],
            eyebrow=("Tus cosas, cubiertas", "Your belongings, covered"), anchor="seguro", tono="relleno"),
        bloque_faq(("Preguntas frecuentes", "Frequently asked questions"), [
            (("¿Qué pasa si algo se daña?", "What if something gets damaged?"),
             ("Lo anotamos en el momento y lo tramitamos con el seguro. Por eso es importante que veas cómo se cargan las cosas.",
              "We note it on the spot and claim through the insurance. That is why it matters that you see how things are loaded.")),
            (("¿Tengo que estar el día de la mudanza?", "Do I need to be there on the day?"),
             ("Sí, o alguien de confianza, al menos al empezar y al terminar, para decir qué va a cada habitación y firmar.",
              "Yes, or someone you trust, at least at the start and the end, to say what goes in each room and sign off.")),
            (("¿Cuánto tarda una mudanza?", "How long does a move take?"),
             ("Un piso pequeño en la misma ciudad, de 3 a 5 horas. Uno grande, un día entero. Te damos un cálculo antes de empezar.",
              "A small flat in the same town, 3 to 5 hours. A large one, a whole day. We give you an estimate before we start.")),
            (("¿Qué tengo que preparar yo?", "What do I need to prepare?"),
             ("Vaciar cajones con cosas delicadas, desconectar la nevera la víspera y reservar el aparcamiento para el camión.",
              "Empty drawers with fragile things, defrost and switch off the fridge the day before, and reserve parking for the lorry.")),
            (("¿Cómo se paga?", "How do I pay?"),
             ("Una señal al reservar y el resto al terminar, con factura. Te decimos los importes en el presupuesto.",
              "A deposit when you book and the rest when we finish, with an invoice. We state the amounts in the quote.")),
        ], tono="base"),
    ])

    cont = pagina("contacto", ("contacto", "contact"), ("Cuéntanos tu mudanza", "Tell us about your move"),
                  ("Cuéntanos tu mudanza con origen, destino, fecha, tamaño de la vivienda, plantas y ascensor, y te damos precio.",
                   "Tell us about your move with origin, destination, date, home size, floors and lift, and we give you a price."), [
        hero(("Cuéntanos tu mudanza", "Tell us about your move"),
             ("Con estos datos podemos darte un primer precio. Si la mudanza es grande, luego pasamos a verlo o nos mandas fotos.",
              "With these details we can give you a first price. If the move is large, we then come to see it or you send us photos."),
             [llamar(), whatsapp()], variante="texto"),
        bloque_contacto(("Datos de la mudanza", "Move details"),
                        ("No hace falta que sea exacto. Si no lo sabes, déjalo en blanco y lo vemos por teléfono.",
                         "It does not need to be exact. If you do not know, leave it blank and we will go over it by phone."),
                        [campo("nombre", "text", ("Nombre", "Name"), True, auto="name", maxlen=120),
                         campo("telefono", "tel", ("Teléfono", "Phone"), True, auto="tel", maxlen=30),
                         campo("servicio", "select", ("Qué necesitas", "What you need"), False,
                               opciones=[("Mudanza local", "Local removal"), ("Mudanza nacional", "Long-distance removal"),
                                         ("Guardamuebles", "Storage"), ("Embalaje", "Packing"),
                                         ("Montaje de muebles", "Furniture assembly"), ("Transporte de un objeto", "Single-item transport")]),
                         campo("origen", "text", ("Dirección de origen (población y calle)", "Origin address (town and street)"), True, maxlen=200),
                         campo("destino", "text", ("Dirección de destino (población y calle)", "Destination address (town and street)"), True, maxlen=200),
                         campo("fecha", "date", ("Fecha prevista", "Planned date")),
                         campo("vivienda", "select", ("Tamaño de la vivienda", "Size of the home"), False,
                               opciones=[("Estudio o 1 habitación", "Studio or 1 bedroom"), ("2 habitaciones", "2 bedrooms"),
                                         ("3 habitaciones", "3 bedrooms"), ("4 o más habitaciones", "4 or more bedrooms"),
                                         ("Local u oficina", "Shop or office")]),
                         campo("metros", "number", ("Metros cuadrados aproximados", "Approximate square metres"), False),
                         campo("planta_origen", "text", ("Planta de origen", "Floor at origin"), False, maxlen=40),
                         campo("ascensor_origen", "select", ("¿Hay ascensor en origen?", "Is there a lift at origin?"), False,
                               opciones=[("Sí", "Yes"), ("No", "No")]),
                         campo("planta_destino", "text", ("Planta de destino", "Floor at destination"), False, maxlen=40),
                         campo("ascensor_destino", "select", ("¿Hay ascensor en destino?", "Is there a lift at destination?"), False,
                               opciones=[("Sí", "Yes"), ("No", "No")]),
                         campo("mensaje", "textarea", ("Algo más que debamos saber (piano, piezas grandes...)", "Anything else we should know (piano, large pieces...)"),
                               False, maxlen=1000)],
                        ("Pedir precio", "Ask for a price"), horas,
                        exito=("Gracias. Te llamamos o te escribimos con un precio en cuanto lo tengamos.",
                               "Thank you. We will call or message you with a price as soon as we have it.")),
        bloque_mapa(("Dónde está el almacén", "Where the warehouse is"),
                    ("Vamos a tu casa, no hace falta que vengas. Esta es la dirección del almacén y de la oficina.",
                     "We come to your home, so there is no need to visit. This is the address of the warehouse and the office."),
                    tono="relleno"),
    ])

    return plantilla(KEY, ("Mudanzas y transporte", "Removals and transport"),
                     ("Cómo funciona una mudanza en pasos, servicios con precios por hora de ejemplo, seguro de la carga y un formulario «cuéntanos tu mudanza» con origen, destino, plantas y ascensor.",
                      "How a move works in steps, services with example hourly prices, cargo insurance and a tell-us-about-your-move form with origin, destination, floors and lift."),
                     "oficios", ("mudanzas mudanza transporte porteadores portes guardamuebles embalaje montaje de muebles mozos camion mudanza local mudanza nacional trasteros",
                                 "removals removal company movers man and van moving house storage packing furniture assembly long distance move transport"),
                     look, cabecera(nav, cta_cab),
                     pie(nav, ("Mudanzas locales y nacionales, guardamuebles, embalaje, montaje y transporte de objetos.",
                               "Local and long-distance removals, storage, packing, assembly and single-item transport."),
                         ("Lunes a viernes de 8:00 a 19:00. Sábado de 9:00 a 14:00.", "Monday to Friday 8 am to 7 pm. Saturday 9 am to 2 pm.")),
                     [inicio, serv, cont])
