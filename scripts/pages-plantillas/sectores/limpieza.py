# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
from lib import *
from looks import LOOKS

KEY = "limpieza"

# Paleta propia (azul agua claro); se comprueba AA al construir.
CRISTAL = dict(background="#f5fbfc", surface="#e1f2f5", text="#0b2a33", muted="#3f6068", primary="#0b7285",
               on_primary="#ffffff", border="#c6e3e9", inverse="#0b3a45", on_inverse="#e8f7fa")


def m2(n):
    """Precio por metro cuadrado: 'Desde 4 € por m²'."""
    a, b = eur(n)
    return (f"Desde {a} por m²", f"From {b} per m²")


def visita(t=("Pedir presupuesto", "Ask for a quote")):
    return boton(t, a_pagina("contacto", "contacto"))


def build():
    malos = comprobar_paleta("cristal", CRISTAL)
    assert not malos, malos
    nueva(KEY)
    look = dict(colores=CRISTAL, fuentes=("work-sans", "work-sans"), radio=(10, 16, 26), h1=(50, 40, 32))
    nav = [("inicio", ("Inicio", "Home")), ("servicios", ("Qué limpiamos", "What we clean")),
           ("presupuesto", ("Precios", "Prices")), ("contacto", ("Contacto", "Contact"))]
    horas = [(("Lunes a viernes", "Monday to Friday"), ("8:00 a 18:00", "8:00 am to 6:00 pm")),
             (("Sábado", "Saturday"), ("9:00 a 13:00", "9:00 am to 1:00 pm")),
             (("Domingo", "Sunday"), ("Cerrado", "Closed"))]
    cta_cab = marco([llamar(t=("Llamar", "Call")), whatsapp("secondary", t=("WhatsApp", "WhatsApp"))],
                    dir="row", gap=8, wrap=True)

    inicio = pagina("inicio", ("", ""), ("Empresa de limpieza", "Cleaning company"),
                    ("Limpieza de hogares, oficinas, comunidades, fin de obra y cristales. Pide presupuesto por metros, por teléfono o WhatsApp.",
                     "Cleaning for homes, offices, housing blocks, post-building work and windows. Ask for a quote by square metre, by phone or WhatsApp."), [
        hero(("Limpieza para tu casa, tu oficina o tu comunidad", "Cleaning for your home, your office or your housing block"),
             ("Vamos con nuestro material, siguiendo una lista de tareas que acordamos contigo. Pide presupuesto y te decimos el precio según los metros.",
              "We bring our own equipment and follow a task list we agree with you. Ask for a quote and we tell you the price by square metre."),
             [visita(), whatsapp()],
             eyebrow=("Servicio de limpieza", "Cleaning service"),
             imagen_=("photo", ("Persona limpiando el suelo de un salón luminoso", "Person cleaning the floor of a bright living room"))),
        bloque_valores(("Qué limpiamos", "What we clean"), [
            (("Hogares", "Homes"),
             ("Limpieza de pisos y casas, cada semana, cada quince días o una vez. También limpieza a fondo antes de mudarte.",
              "Cleaning of flats and houses every week, every fortnight or once. Also a deep clean before you move.")),
            (("Oficinas y locales", "Offices and shops"),
             ("Fuera del horario de trabajo o a primera hora, para que lo encuentres listo al llegar.",
              "Outside working hours or first thing, so you find it ready when you arrive.")),
            (("Comunidades de vecinos", "Housing blocks"),
             ("Portal, escaleras, ascensor, garaje y zonas comunes, con un calendario de visitas fijo.",
              "Entrance, stairs, lift, garage and shared areas, on a fixed visiting schedule.")),
            (("Final de obra", "Post-building clean"),
             ("Polvo, restos de pintura y de yeso, suelos y cristales, para que entres a vivir.",
              "Dust, paint and plaster traces, floors and windows, so you can move in.")),
            (("Cristales", "Windows"),
             ("Ventanas, balcones, cristaleras y escaparates, por dentro y por fuera.",
              "Windows, balconies, glazed frontages and shop windows, inside and out.")),
        ], eyebrow=("Servicios", "Services"), cols=3, estilo="relleno"),
        bloque_checks(("Cómo trabajamos", "How we work"), [
            ("Acordamos contigo una lista de tareas por escrito antes de empezar.", "We agree a written task list with you before we start."),
            ("Llevamos nuestros productos y nuestro material, y no te pedimos nada.", "We bring our own products and equipment, and ask you for nothing."),
            ("Mismo equipo cada visita siempre que es posible, para que te conozcan.", "The same team each visit whenever possible, so they get to know you."),
            ("Si algo no queda bien, nos avisas y lo repetimos.", "If something is not right, tell us and we do it again."),
            ("Si nos dejas las llaves, las guardamos con un registro de entrega.", "If you leave us your keys, we keep them with a handover record."),
            ("Factura con IVA después de cada servicio o a fin de mes.", "An invoice with VAT after each service or at month end."),
        ], eyebrow=("Qué puedes esperar", "What to expect"), tono="base", cols=2),
        bloque_pasos(("Cómo pedir presupuesto", "How to ask for a quote"), [
            (("Nos cuentas qué quieres limpiar", "Tell us what you want cleaned"),
             ("Tipo de servicio, metros aproximados y cada cuánto lo necesitas.", "Type of service, approximate square metres and how often you need it.")),
            (("Te damos un precio", "We give you a price"),
             ("Por teléfono o por escrito. Si es una oficina o una obra, pasamos a verlo.", "By phone or in writing. For an office or a building site, we come to see it.")),
            (("Empezamos", "We start"),
             ("Fijamos el día y la hora, y el equipo llega con todo lo necesario.", "We set the day and time, and the team arrives with everything needed.")),
        ], tono="relleno"),
        bloque_opiniones(("Lo que dicen nuestros clientes", "What our customers say"),
                         [("la limpieza", "the cleaning"), ("la puntualidad", "the punctuality"), ("el trato", "the service")], tono="base"),
        bloque_cta(("¿Cuánto cuesta limpiar lo tuyo?", "How much would it cost to clean yours?"),
                   ("Dinos los metros y qué necesitas y te damos un precio.", "Tell us the square metres and what you need and we will give you a price."),
                   [visita(), whatsapp("ghost")]),
    ])

    def servicio(titulo, intro, items, ancla, tono, eyebrow):
        return bloque_checks(titulo, items, intro=intro, eyebrow=eyebrow, tono=tono, cols=2, anchor=ancla)

    serv = pagina("servicios", ("servicios", "services"), ("Qué limpiamos y qué incluye", "What we clean and what is included"),
                  ("Qué incluye la limpieza de hogares, oficinas, comunidades, final de obra y cristales, y cada cuánto se puede contratar.",
                   "What cleaning of homes, offices, housing blocks, post-building work and windows includes, and how often you can book it."), [
        hero(("Qué incluye cada servicio", "What each service includes"),
             ("Esto es lo que hacemos en cada visita. Si quieres añadir o quitar algo, lo acordamos antes de empezar.",
              "This is what we do on every visit. If you want to add or take something off, we agree it before we start."),
             [visita(), whatsapp()], variante="texto"),
        servicio(("Limpieza de hogares", "Home cleaning"),
                 ("Para pisos y casas, con visitas fijas o una sola vez.", "For flats and houses, with regular visits or just once."),
                 [("Cocina: encimera, fregadero, campana por fuera, electrodomésticos por fuera y suelo.", "Kitchen: worktop, sink, outside of the extractor, outside of appliances and floor."),
                  ("Baños: sanitarios, ducha o bañera, azulejos, espejo y suelo.", "Bathrooms: toilet, shower or bath, tiles, mirror and floor."),
                  ("Habitaciones y salón: polvo, superficies, cambio de ropa de cama si lo pides y suelo.", "Bedrooms and living room: dust, surfaces, bed linen change if you ask, and floor."),
                  ("Cristales interiores y marcos de ventanas, una vez al mes.", "Inside windows and window frames, once a month."),
                  ("Planchado y limpieza del interior de nevera u horno, como extra.", "Ironing and cleaning the inside of the fridge or oven, as extras.")],
                 "hogares", "base", ("Hogares", "Homes")),
        servicio(("Oficinas y locales", "Offices and shops"),
                 ("Fuera de horario o a primera hora, con un calendario fijo.", "Outside hours or first thing, on a fixed schedule."),
                 [("Mesas, mostradores y superficies de trabajo, sin tocar tus papeles.", "Desks, counters and work surfaces, without touching your papers."),
                  ("Suelos aspirados y fregados.", "Floors vacuumed and mopped."),
                  ("Baños y office: sanitarios, grifos, espejos y reposición de papel y jabón.", "Toilets and kitchenette: fittings, taps, mirrors and refilling paper and soap."),
                  ("Vaciado de papeleras y retirada de basura.", "Emptying of bins and waste removal."),
                  ("Cristales interiores y puertas de cristal.", "Inside windows and glass doors.")],
                 "oficinas", "relleno", ("Empresas", "Businesses")),
        servicio(("Comunidades de vecinos", "Housing blocks"),
                 ("Con un calendario de visitas que se queda por escrito.", "With a visiting schedule that is put in writing."),
                 [("Portal y escaleras: barrido y fregado.", "Entrance and stairs: sweeping and mopping."),
                  ("Ascensor: suelo, espejo, paredes y botonera.", "Lift: floor, mirror, walls and buttons."),
                  ("Barandillas, buzones y cristales de la entrada.", "Banisters, letterboxes and entrance glass."),
                  ("Garaje y trasteros: barrido y retirada de lo que se acumula.", "Garage and storage rooms: sweeping and clearing what builds up."),
                  ("Cuarto de basuras y patios.", "Bin room and courtyards.")],
                 "comunidades", "base", ("Comunidades", "Housing blocks")),
        servicio(("Limpieza de final de obra", "Post-building clean"),
                 ("Se hace después de que se vayan los operarios y antes de que entres.", "Done after the workers leave and before you move in."),
                 [("Retirada de polvo de obra en todas las superficies, incluidos altillos y rodapiés.", "Removal of building dust from all surfaces, including high shelves and skirting boards."),
                  ("Restos de pintura, silicona, yeso y cemento, con cuidado de no rayar.", "Traces of paint, silicone, plaster and cement, taking care not to scratch."),
                  ("Cocina y baños: sanitarios, azulejos, grifería y muebles por dentro y por fuera.", "Kitchen and bathrooms: fittings, tiles, taps and furniture inside and out."),
                  ("Suelos, puertas y cristales.", "Floors, doors and windows."),
                  ("Retirada de embalajes y pequeños restos.", "Removal of packaging and small leftovers.")],
                 "obra", "relleno", ("Obras y reformas", "Building and refits")),
        servicio(("Limpieza de cristales", "Window cleaning"),
                 ("Por dentro y por fuera, con material propio.", "Inside and out, with our own equipment."),
                 [("Ventanas, balcones y cristaleras de pisos y casas.", "Windows, balconies and glazed frontages of flats and houses."),
                  ("Escaparates y cristales de local.", "Shop windows and glazed shopfronts."),
                  ("Marcos, persianas y alféizares.", "Frames, shutters and sills."),
                  ("Los trabajos en altura los hacemos solo con el equipo adecuado y desde el suelo o desde la azotea.", "Work at height is done only with the right equipment and from the ground or the roof."),
                  ("Una vez, o cada uno, dos o tres meses.", "Once, or every one, two or three months.")],
                 "cristales", "base", ("Cristales", "Windows")),
        bloque_valores(("Cada cuánto", "How often"), [
            (("Una vez", "Just once"), ("Para antes de mudarte, después de una obra o tras unas vacaciones.", "For before you move, after building work or after a holiday.")),
            (("Cada semana", "Every week"), ("La más pedida en hogares y oficinas. Todo se mantiene al día.", "The most common for homes and offices. Everything stays up to date.")),
            (("Cada quince días", "Every fortnight"), ("Para casas que se ensucian menos o presupuestos más ajustados.", "For homes that get less dirty or tighter budgets.")),
            (("Cada mes", "Every month"), ("Una limpieza más a fondo, sobre todo en cristales y comunidades.", "A deeper clean, especially for windows and housing blocks.")),
        ], cols=4, eyebrow=("Frecuencias", "Frequencies"), estilo="borde", tono="relleno", anchor="frecuencias"),
        bloque_cta(("¿Qué necesitas limpiar?", "What do you need cleaned?"), None, [visita(), llamar("ghost")], tono="base"),
    ])

    pres = pagina("presupuesto", ("precios", "prices"), ("Precios y presupuesto", "Prices and quotes"),
                  ("Precios de ejemplo por metros y por visita, un presupuesto de ejemplo, productos y personal, y preguntas frecuentes.",
                   "Example prices by square metre and by visit, a worked quote, products and staff, and frequently asked questions."), [
        hero(("Precios por metros", "Prices by square metre"),
             ("El precio depende de los metros, de cada cuánto limpiamos y del estado de partida. Aquí tienes una guía; el precio exacto te lo damos con tus datos.",
              "The price depends on the square metres, how often we clean and how things start out. Here is a guide; we give you the exact price with your details."),
             [visita(), whatsapp()], variante="texto"),
        bloque_precios(("Cuánto cuesta", "What it costs"), ("Precios de limpieza por metros y por visita", "Cleaning prices by square metre and per visit"),
                       [("Servicio", "Service"), ("Tamaño", "Size"), ("Precio", "Price")],
                       [(("Hogar", "Home"), ("Hasta 60 m²", "Up to 60 m²"), ("Desde 45 € por visita", "From €45 per visit")),
                        (("Hogar", "Home"), ("De 60 a 100 m²", "60 to 100 m²"), ("Desde 65 € por visita", "From €65 per visit")),
                        (("Hogar", "Home"), ("De 100 a 150 m²", "100 to 150 m²"), ("Desde 90 € por visita", "From €90 per visit")),
                        (("Oficina o local", "Office or shop"), ("Hasta 100 m²", "Up to 100 m²"), ("Desde 60 € por visita", "From €60 per visit")),
                        (("Comunidad de vecinos", "Housing block"), ("Según portales y plantas", "Depending on entrances and floors"), ("Presupuesto con visita", "Quote after a visit")),
                        (("Final de obra", "Post-building clean"), ("Por metro cuadrado", "Per square metre"), m2(4)),
                        (("Cristales", "Windows"), ("Por metro cuadrado de cristal", "Per square metre of glass"), m2(3))],
                       nota=("Precios de ejemplo: cámbialos por los tuyos. Incluyen IVA. Los extras (horno, nevera, planchado) se cobran aparte.",
                             "Example prices: replace them with your own. VAT included. Extras (oven, fridge, ironing) are charged separately."),
                       anchor="precios", tono="base"),
        bloque_texto(("Un presupuesto de ejemplo", "A worked example"), [
            ("Un piso de 80 m² con limpieza cada semana entra en el tramo de 60 a 100 m²: 65 € por visita, cuatro visitas al mes, 260 € al mes. Es un ejemplo con precios de ejemplo; el tuyo lo calculamos con tus metros.",
             "A flat of 80 m² cleaned every week falls in the 60 to 100 m² band: €65 per visit, four visits a month, €260 a month. This is an example with example prices; we work yours out with your square metres."),
            ("Si la casa tiene mascotas, está muy sucia al empezar o tiene más baños de lo habitual, te lo decimos antes de empezar.",
             "If the home has pets, is very dirty at the start or has more bathrooms than usual, we tell you before we start.")],
            eyebrow=("Cómo se calcula", "How it is worked out"), tono="relleno", anchor="ejemplo"),
        bloque_valores(("Productos, personal y seguro", "Products, staff and insurance"), [
            (("Productos", "Products"),
             ("Indica aquí qué productos usas y si ofreces opciones ecológicas o sin perfume cuando el cliente las pide.",
              "State here which products you use and whether you offer eco-friendly or fragrance-free options when the customer asks.")),
            (("Personal", "Staff"),
             ("Indica aquí cómo es tu equipo: si está contratado, qué formación tiene y si va uniformado.",
              "State here what your team is like: whether they are employed, what training they have and whether they wear a uniform.")),
            (("Seguro", "Insurance"),
             ("Indica aquí la aseguradora y la póliza de responsabilidad civil que cubre posibles daños.",
              "State here the insurer and the public liability policy that covers possible damage.")),
        ], intro=("Texto de ejemplo: cámbialo por tus datos reales y no pongas nada que no puedas demostrar.", "Example text: replace it with your real details and put nothing you cannot prove."),
            eyebrow=("Con quién entras en casa", "Who comes into your home"), tono="base", anchor="garantias"),
        bloque_faq(("Preguntas frecuentes", "Frequently asked questions"), [
            (("¿Tengo que estar en casa?", "Do I need to be at home?"),
             ("No hace falta. Puedes dejarnos las llaves o abrirnos tú. Acordamos el día y la hora contigo.",
              "Not necessarily. You can leave us the keys or let us in yourself. We agree the day and time with you.")),
            (("¿Ponéis vosotros los productos?", "Do you supply the products?"),
             ("Sí, llevamos los productos y el material. Si prefieres que usemos los tuyos, dínoslo.",
              "Yes, we bring the products and equipment. If you would rather we use yours, tell us.")),
            (("¿Y si algo no queda bien?", "What if something is not right?"),
             ("Avísanos en las 24 horas siguientes y lo repetimos sin coste.",
              "Let us know within 24 hours and we will redo it at no charge.")),
            (("¿Cómo se paga?", "How do I pay?"),
             ("Por transferencia o domiciliación a fin de mes, o en el momento si es un servicio puntual. Siempre con factura.",
              "By bank transfer or direct debit at month end, or on the day for a one-off service. Always with an invoice.")),
            (("¿Qué hago si tengo que cancelar una visita?", "What if I need to cancel a visit?"),
             ("Avísanos con un día de antelación y la cambiamos de fecha sin coste.",
              "Let us know a day ahead and we will move it to another date at no cost.")),
        ], tono="relleno"),
    ])

    cont = pagina("contacto", ("contacto", "contact"), ("Contacto y presupuesto", "Contact and quote"),
                  ("Pide presupuesto de limpieza con el tipo de servicio y los metros. Horario y cómo llegar.",
                   "Ask for a cleaning quote with the type of service and the square metres. Opening hours and how to find us."), [
        hero(("Pide tu presupuesto", "Ask for your quote"),
             ("Dinos el tipo de servicio y los metros aproximados. Te respondemos con un precio en el mismo día laborable.",
              "Tell us the type of service and the approximate square metres. We reply with a price the same working day."),
             [llamar(), whatsapp()], variante="texto"),
        bloque_contacto(("Solicitud de presupuesto", "Quote request"),
                        ("Con estos datos basta para un primer precio. Si hace falta, luego pasamos a verlo.",
                         "These details are enough for a first price. If needed, we then come to see it."),
                        campos_cita(servicios=[("Hogar", "Home"), ("Oficina o local", "Office or shop"), ("Comunidad de vecinos", "Housing block"),
                                               ("Final de obra", "Post-building clean"), ("Cristales", "Windows")],
                                    fecha=False,
                                    extra=[campo("metros", "number", ("Metros cuadrados aproximados", "Approximate square metres"), True),
                                           campo("frecuencia", "select", ("Cada cuánto", "How often"), False,
                                                 opciones=[("Una vez", "Just once"), ("Cada semana", "Every week"),
                                                           ("Cada quince días", "Every fortnight"), ("Cada mes", "Every month")]),
                                           campo("poblacion", "text", ("Población", "Town"), True, maxlen=120)]),
                        ("Pedir presupuesto", "Ask for a quote"), horas,
                        exito=("Gracias. Te llamamos o te escribimos con un precio en cuanto lo tengamos.",
                               "Thank you. We will call or message you with a price as soon as we have it.")),
        bloque_mapa(("Dónde está la oficina", "Where the office is"),
                    ("Vamos a donde nos necesites, no hace falta que vengas. Esta es la oficina.",
                     "We come to wherever you need us, so there is no need to visit. This is the office."), tono="relleno"),
    ])

    return plantilla(KEY, ("Empresa de limpieza", "Cleaning company"),
                     ("Qué incluye cada servicio, frecuencias, precios por metros de ejemplo con un presupuesto de muestra y solicitud de presupuesto con tipo de servicio y metros.",
                      "What each service includes, frequencies, example prices by square metre with a worked quote and a quote request with service type and square metres."),
                     "oficios", ("limpieza empresa de limpieza limpiadora servicio domestico limpieza de hogar limpieza de oficinas limpieza de comunidades fin de obra cristales limpieza de cristales",
                                 "cleaning cleaning company cleaner home cleaning office cleaning housing block cleaning post-building clean end of tenancy window cleaning"),
                     look, cabecera(nav, cta_cab),
                     pie(nav, ("Limpieza de hogares, oficinas, comunidades, final de obra y cristales.",
                               "Cleaning for homes, offices, housing blocks, post-building work and windows."),
                         ("Lunes a viernes de 8:00 a 18:00. Sábado de 9:00 a 13:00.", "Monday to Friday 8 am to 6 pm. Saturday 9 am to 1 pm.")),
                     [inicio, serv, pres, cont])
