# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
from lib import *

KEY = "cerrajero"

# Paleta propia: fondo oscuro cálido y naranja de aviso. No lleva secciones `tono="oscuro"`: el fondo ya lo es.
CERRADURA = dict(
    colores=dict(background="#16171a", surface="#202226", text="#f4f2ed", muted="#b9b6ad", primary="#ff7a45",
                 on_primary="#16171a", border="#34373d", inverse="#0c0d0f", on_inverse="#f4f2ed"),
    fuentes=("manrope", "ibm-plex-sans"), peso=800, h1=(46, 38, 30), radio=(4, 8, 12))


def tel_grande(rotulo):
    """El teléfono como enlace y en grande: lo primero que busca quien se ha quedado fuera."""
    return marco([peq(rotulo, color="$muted"),
                  marco([texto("p", (TEL_VISIBLE, TEL_VISIBLE), preset="h1", color="$primary")], enlace=a_tel())],
                 gap=4, align="start")


def build():
    nueva(KEY)
    look = CERRADURA
    nav = [("inicio", ("Inicio", "Home")), ("servicios", ("Servicios y precios", "Services and prices")),
           ("fuera", ("Si te quedas fuera", "If you are locked out")), ("contacto", ("Contacto", "Contact"))]
    horas = [(("Urgencias", "Emergencies"), ("Todos los días, las 24 horas", "Every day, 24 hours")),
             (("Taller y presupuestos", "Workshop and quotes"),
              ("Lunes a viernes, de 9:00 a 19:00", "Monday to Friday, 9:00 am to 7:00 pm"))]
    cta_cab = marco([llamar(t=("Llamar", "Call")), whatsapp("secondary", t=("WhatsApp", "WhatsApp"))],
                    dir="row", gap=8, wrap=True)
    llegada = ("En Tu ciudad solemos llegar en unos 30 minutos (tiempo de ejemplo: cámbialo por el tuyo).",
               "In Your city we usually arrive within about 30 minutes (example time: replace it with yours).")

    inicio = pagina("inicio", ("", ""), ("Cerrajero 24 horas", "24-hour locksmith"),
                    ("Cerrajero con urgencias las 24 horas: apertura de puertas sin dañar la cerradura, cambio de bombín, puertas blindadas y cajas fuertes.",
                     "24-hour locksmith: door opening without damaging the lock, cylinder changes, security doors and safes."), [
        hero(("Cerrajero. Urgencias las 24 horas", "Locksmith. Emergencies 24 hours a day"),
             ("Abrimos tu puerta sin dañar la cerradura siempre que se puede. Antes de empezar te decimos el precio y no tocamos nada sin tu visto bueno.",
              "We open your door without damaging the lock whenever possible. Before we start we tell you the price and touch nothing without your say-so."),
             [llamar(), whatsapp()], variante="split",
             eyebrow=("Cerrajería", "Locksmith"),
             imagen_=("photo", ("Cerrajero abriendo una puerta de vivienda con herramientas especiales",
                                "Locksmith opening a front door with specialist tools")),
             extra=tel_grande(("Urgencias, todos los días a cualquier hora", "Emergencies, every day at any hour"))),
        bloque_checks(("Cuánto tardamos en llegar", "How long we take to arrive"), [
            llegada,
            ("Al llamar te damos una hora aproximada y, si hay retraso, te avisamos.",
             "When you call we give you an approximate time and, if we are delayed, we let you know."),
            ("Te decimos el precio del desplazamiento antes de salir.", "We tell you the call-out price before we set off."),
        ], cols=1, tono="relleno", kind="features"),
        bloque_valores(("Qué hacemos", "What we do"), [
            (("Apertura de puertas", "Door opening"),
             ("Pisos, casas, trasteros y garajes. Probamos primero técnicas que no dañan la cerradura.",
              "Flats, houses, storerooms and garages. We try methods that do not damage the lock first.")),
            (("Cambio de bombín", "Cylinder changes"),
             ("Si has perdido las llaves, te las han robado o te mudas, cambiamos el bombín y te damos llaves nuevas.",
              "If you have lost your keys, had them stolen or just moved in, we change the cylinder and give you new keys.")),
            (("Puertas blindadas", "Security doors"),
             ("Reparamos y ajustamos cerraduras de puertas blindadas, y las abrimos cuando se han atascado.",
              "We repair and adjust security-door locks, and open them when they have jammed.")),
            (("Cajas fuertes", "Safes"),
             ("Apertura de cajas fuertes cuando se pierde la llave o se olvida la combinación.",
              "Opening safes when the key is lost or the combination is forgotten.")),
            (("Cerraduras de seguridad", "Security locks"),
             ("Instalamos bombines antibumping y cerraduras de seguridad. Te explicamos en qué se diferencian.",
              "We fit anti-bumping cylinders and security locks. We explain how they differ."))
        ], eyebrow=("Servicios", "Services"), cols=3, estilo="borde"),
        bloque_pasos(("Cómo trabajamos", "How we work"), [
            (("Llamas", "You call"),
             ("Nos dices dónde estás y qué tipo de puerta es. Te damos un tiempo de llegada y el desplazamiento.",
              "You tell us where you are and what kind of door it is. We give you an arrival time and the call-out price.")),
            (("Acreditas la vivienda", "You show it is your home"),
             ("Por seguridad, antes de abrir pedimos un documento o un recibo que demuestre que vives o trabajas allí.",
              "For security, before opening we ask for a document or a bill showing that you live or work there.")),
            (("Presupuesto antes de empezar", "A quote before we start"),
             ("Vemos la cerradura y te decimos cuánto cuesta. Sin tu conformidad no empezamos.",
              "We look at the lock and tell you the cost. We do not start without your agreement.")),
            (("Abrimos y dejas la puerta lista", "We open it and leave the door ready"),
             ("Comprobamos que cierra bien, te damos factura y, si hay bombín nuevo, las llaves.",
              "We check it locks properly, give you an invoice and, if there is a new cylinder, the keys.")),
        ]),
        bloque_opiniones(("Lo que dicen nuestros clientes", "What our customers say"),
                         [("la rapidez", "the speed"), ("el precio claro", "the clear price"), ("el trato", "the service")]),
        bloque_cta(("¿Te has quedado fuera?", "Locked out?"),
                   ("Llámanos y te decimos cuánto tardamos. Si te ayuda, mira qué hacer mientras llegamos.",
                    "Call us and we will tell you how long we will take. If it helps, see what to do until we arrive."),
                   [llamar(), whatsapp("ghost")]),
    ])

    serv = pagina("servicios", ("servicios", "services"), ("Servicios y precios", "Services and prices"),
                  ("Precios orientativos de desplazamiento, apertura y cambio de bombín, puertas blindadas, cajas fuertes y preguntas frecuentes.",
                   "Guide prices for call-outs, opening and cylinder changes, security doors, safes and frequently asked questions."), [
        hero(("Servicios y precios", "Services and prices"),
             ("Esto es lo que cuesta venir y los trabajos más habituales. El precio exacto te lo damos antes de empezar.",
              "This is what a call-out costs and our most common jobs. We give you the exact price before we start."),
             [llamar(), whatsapp()], variante="texto"),
        bloque_precios(("Desplazamiento y trabajos", "Call-out and jobs"),
                       ("Precios orientativos de cerrajería", "Guide locksmith prices"),
                       [("Concepto", "Item"), ("Precio", "Price")],
                       [(("Desplazamiento en horario normal", "Call-out in normal hours"), eur(30)),
                        (("Desplazamiento urgente de noche, fin de semana o festivo", "Urgent call-out at night, at weekends or on bank holidays"), desde(60)),
                        (("Apertura de puerta sin daños (puerta cerrada de golpe)", "Opening a door without damage (door pulled shut)"), desde(70)),
                        (("Apertura de puerta con cerradura cerrada con llave", "Opening a door locked with the key"), desde(110)),
                        (("Cambio de bombín (sin contar el bombín)", "Cylinder change (cylinder not included)"), desde(40)),
                        (("Bombín de seguridad", "Security cylinder"), desde(60)),
                        (("Apertura de caja fuerte", "Opening a safe"), desde(120))],
                       nota=("Precios de ejemplo: cámbialos por los tuyos. Incluyen IVA. La cerradura o el bombín nuevos se presupuestan aparte.",
                             "Example prices: replace them with your own. VAT included. A new lock or cylinder is quoted separately."),
                       anchor="precios"),
        bloque_texto(("Puertas blindadas", "Security doors"), [
            ("Cada puerta blindada es distinta, así que aquí no hay precio fijo: vamos a verla y te damos el presupuesto por escrito, sin compromiso.",
             "Every security door is different, so there is no fixed price here: we come to look at it and give you a written quote with no obligation."),
            ("Arreglamos cerraduras que se atascan, puertas que ya no cierran bien y bombines que han dejado de girar.",
             "We fix locks that jam, doors that no longer close properly and cylinders that have stopped turning.")],
            eyebrow=("Blindadas", "Security doors"), tono="relleno", anchor="blindadas",
            imagen_=("photo", ("Puerta blindada con cerradura de varios puntos de cierre", "Security door with a multi-point lock")),
            invertir=True),
        bloque_faq(("Preguntas frecuentes", "Frequently asked questions"), [
            (("¿Podéis abrir sin romper la cerradura?", "Can you open without breaking the lock?"),
             ("En la mayoría de los casos sí, y siempre lo intentamos primero. Si no se puede, te lo explicamos y no forzamos nada sin tu permiso.",
              "In most cases yes, and we always try that first. If it is not possible we explain why and do not force anything without your permission.")),
            (("¿Qué necesito para que abráis la puerta?", "What do I need for you to open the door?"),
             ("Un documento o un recibo que demuestre que es tu vivienda o tu local. Si no lo tienes a mano, llámanos y te decimos qué sirve.",
              "A document or a bill showing it is your home or premises. If you do not have one to hand, call us and we will tell you what works.")),
            (("¿Cobráis más de noche y en festivos?", "Do you charge more at night and on bank holidays?"),
             ("Sí: el desplazamiento de noche, fin de semana y festivo es mayor. Te decimos el importe por teléfono antes de salir.",
              "Yes: the call-out at night, at weekends and on bank holidays is higher. We tell you the amount by phone before we set off.")),
            (("¿Dais factura y garantía?", "Do you give an invoice and a warranty?"),
             ("Sí. Todos los trabajos llevan factura con IVA y la garantía de la cerradura nueva por escrito.",
              "Yes. Every job comes with an invoice including VAT and the warranty on the new lock in writing.")),
        ], tono="base"),
    ])

    fuera = pagina("fuera", ("si-te-quedas-fuera", "if-you-are-locked-out"), ("Si te quedas fuera", "If you are locked out"),
                   ("Qué hacer si te quedas fuera de casa: pasos antes de llamar, qué no hacer y cuánto tarda en llegar el cerrajero.",
                    "What to do if you are locked out: steps before calling, what not to do and how long the locksmith takes to arrive."), [
        hero(("Si te quedas fuera de casa", "If you are locked out of your home"),
             ("Respira: casi siempre tiene arreglo sin romper nada. Estos pasos te ahorran tiempo y dinero.",
              "Breathe: it can almost always be fixed without breaking anything. These steps save you time and money."),
             [llamar(), whatsapp()], variante="centro", tono="relleno",
             extra=tel_grande(("Urgencias, las 24 horas", "Emergencies, 24 hours"))),
        bloque_pasos(("Qué hacer, paso a paso", "What to do, step by step"), [
            (("Prueba la puerta", "Try the door"),
             ("Comprueba si ha quedado solo encajada. Si la hoja no tiene la llave echada, a veces se abre sin más.",
              "Check whether it has simply been pulled shut. If the key was not turned, it sometimes opens easily.")),
            (("Busca otra llave o a quien la tenga", "Look for another key or someone who has one"),
             ("Un vecino, un familiar o el portero pueden tener una copia. Es lo más barato.",
              "A neighbour, a relative or the concierge may have a copy. It is the cheapest way.")),
            (("Prepara un documento", "Get a document ready"),
             ("Cualquier papel con tu nombre y la dirección: DNI, recibo o contrato. Lo pediremos al llegar.",
              "Any paper showing your name and address: ID, a bill or a contract. We will ask for it when we arrive.")),
            (("Llámanos", "Call us"),
             ("Te decimos cuánto tardamos y cuánto cuesta el desplazamiento. Espera en un sitio seguro.",
              "We tell you how long we will take and what the call-out costs. Wait somewhere safe.")),
        ], tono="base"),
        bloque_checks(("Lo que no conviene hacer", "What not to do"), [
            ("No fuerces la cerradura ni la golpees: puedes romperla y la apertura sale más cara.",
             "Do not force or hit the lock: you may break it and the opening costs more."),
            ("No uses tarjetas ni alambres en cerraduras de seguridad: se atascan.",
             "Do not use cards or wires on security locks: they jam."),
            ("No te subas a ventanas ni balcones: un susto con el cerrajero es mucho mejor que una caída.",
             "Do not climb through windows or balconies: a locksmith call is much better than a fall."),
            ("Desconfía de quien no te dice el nombre del negocio o se niega a darte un precio antes de venir.",
             "Be wary of anyone who will not give the business name or refuses to quote a price before coming."),
        ], tono="relleno", cols=1),
        bloque_checks(("Cuánto tardamos en llegar", "How long we take to arrive"), [
            llegada,
            ("De noche y en festivos puede ser algo más. Siempre te lo decimos al llamar.",
             "At night and on bank holidays it can be a little longer. We always tell you when you call."),
        ], cols=1),
        bloque_cta(("Cuando quieras, llámanos", "Call us whenever you like"),
                   ("Estamos al teléfono todos los días del año.", "We are on the phone every day of the year."),
                   [llamar(), whatsapp("ghost")]),
    ])

    cont = pagina("contacto", ("contacto", "contact"), ("Contacto y urgencias", "Contact and emergencies"),
                  ("Llama a cualquier hora para una urgencia, o escríbenos por WhatsApp o con el formulario para pedir presupuesto.",
                   "Call at any hour for an emergency, or message us on WhatsApp or with the form to ask for a quote."), [
        hero(("Llámanos o escríbenos", "Call or message us"),
             ("Para una urgencia, llama: el formulario no lo atendemos al momento. Para presupuestos de blindadas o cajas fuertes, usa el formulario o WhatsApp.",
              "For an emergency, call: we do not answer the form straight away. For quotes on security doors or safes, use the form or WhatsApp."),
             [llamar(), whatsapp()], variante="centro",
             extra=tel_grande(("Urgencias, las 24 horas", "Emergencies, 24 hours"))),
        bloque_contacto(("Pide presupuesto", "Ask for a quote"),
                        ("Cuéntanos qué necesitas y dónde estás. Te llamamos para cerrar el precio.",
                         "Tell us what you need and where you are. We will call you to agree the price."),
                        campos_cita(servicios=[("Me he quedado fuera", "I am locked out"), ("Cambio de bombín", "Cylinder change"),
                                               ("Puerta blindada", "Security door"), ("Caja fuerte", "Safe"),
                                               ("Otro trabajo", "Other job")],
                                    fecha=False,
                                    extra=[campo("poblacion", "text", ("Población o barrio", "Town or area"), True, maxlen=120),
                                           campo("urgente", "select", ("¿Es una urgencia?", "Is it an emergency?"), False,
                                                 opciones=[("No, puede esperar", "No, it can wait"), ("Sí, ahora mismo", "Yes, right now")])]),
                        ("Pedir presupuesto", "Ask for a quote"), horas,
                        exito=("Gracias. Te llamamos lo antes posible. Si es una urgencia, llama al teléfono que ves en esta página.",
                               "Thank you. We will call you as soon as possible. If it is an emergency, call the number on this page.")),
        bloque_mapa(("Dónde está el taller", "Where the workshop is"),
                    ("Vamos a donde estés, no hace falta que vengas. Este es nuestro taller.",
                     "We come to you, so there is no need to visit. This is our workshop."), tono="relleno"),
    ])

    return plantilla(KEY, ("Cerrajero 24 horas", "24-hour locksmith"),
                     ("Urgencias 24 horas con el teléfono grande en la portada, servicios y precios de ejemplo, pasos si te quedas fuera y presupuesto por WhatsApp o formulario.",
                      "24-hour emergencies with a large phone number on the cover, services and example prices, steps if you are locked out and quotes by WhatsApp or form."),
                     "oficios", ("cerrajero cerrajeria urgencias 24 horas apertura de puertas bombin cerradura puerta blindada caja fuerte llaves me he quedado fuera",
                                 "locksmith lock emergency 24 hour door opening cylinder lock security door safe keys locked out"),
                     look, cabecera(nav, cta_cab),
                     pie(nav, ("Cerrajería. Urgencias las 24 horas y presupuesto antes de empezar.",
                               "Locksmith. 24-hour emergencies and a quote before we start."),
                         ("Urgencias 24 horas. Taller de lunes a viernes de 9:00 a 19:00.", "Emergencies 24 hours. Workshop Monday to Friday 9 am to 7 pm.")),
                     [inicio, serv, fuera, cont])
