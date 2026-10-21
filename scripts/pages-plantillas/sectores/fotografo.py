# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
from lib import *
from looks import LOOKS  # noqa: F401  (la paleta es propia de este sector)

KEY = "fotografo"

# Paleta propia: papel de copia fotográfica y tinta casi negra; la foto pone el color. Sin `tono="oscuro"`:
# primary/inverse no llega a 3, así que los bloques de énfasis usan `relleno`.
PAPEL = dict(
    colores=dict(background="#fcfbf9", surface="#f2f0eb", text="#181816", muted="#5b5a55", primary="#1c1c1a",
                 on_primary="#ffffff", border="#dedbd3", inverse="#1a1a18", on_inverse="#f4f2ee"),
    fuentes=("source-serif-4", "inter"), peso=500, h1=(54, 42, 32), radio=(0, 0, 2), ancho=1240)


def galeria_mixta(titulo, fotos, intro=None, eyebrow=None, tono="base", anchor=None, tipo="photo"):
    """Rejilla de tres columnas con formatos distintos (vertical y apaisado), como un contacto de copias."""
    li = [marco([imagen(tipo, alt, ratio=r, radio=0)], tag="li", gap=0) for alt, r in fotos]
    return seccion("gallery", [titular(titulo, intro, eyebrow), rejilla(li, cols=3, gap=14, movil=2, align="center")],
                   tono=tono, anchor=anchor)


def build():
    nueva(KEY)
    look = PAPEL
    nav = [("inicio", ("Inicio", "Home")), ("portafolio", ("Portafolio", "Portfolio")),
           ("sesiones", ("Sesiones y precios", "Sessions and prices")), ("contacto", ("Contacto", "Contact"))]
    horas = [(("Lunes a viernes", "Monday to Friday"),
              ("10:00 a 14:00 y 16:00 a 19:00", "10:00 am to 2:00 pm and 4:00 pm to 7:00 pm")),
             (("Sábado", "Saturday"), ("Solo sesiones con cita", "Sessions by appointment only")),
             (("Domingo", "Sunday"), ("Cerrado", "Closed"))]
    nota = ("Precios de ejemplo: cámbialos por los tuyos. Las fotos y las horas incluidas también son de ejemplo. Incluyen IVA.",
            "Example prices: replace them with your own. The photos and hours included are examples too. VAT included.")

    def reservar(variante="primary"):
        return boton(("Reservar fecha", "Book a date"), a_pagina("contacto", "contacto"), variante)

    inicio = pagina("inicio", ("", ""), ("Fotógrafo", "Photographer"),
                    ("Fotografía de retrato, familia, producto, eventos y bodas: portafolio, packs con precio y reserva de fecha. Llama o escribe por WhatsApp.",
                     "Portrait, family, product, event and wedding photography: portfolio, priced packs and date booking. Call or message us on WhatsApp."), [
        seccion("hero", [
            marco([etiqueta(("Fotografía", "Photography")),
                   h1(("Fotos que querrás tener en la pared", "Photos you will want on your wall")),
                   p(("Retratos, familias, producto, eventos y bodas. Te explicamos cómo es la sesión, cuánto cuesta y cuándo recibes las fotos.",
                      "Portraits, families, products, events and weddings. We explain how the session goes, what it costs and when you get your photos."),
                     color="$muted"),
                   marco([reservar(), whatsapp()], dir="row", gap=12, wrap=True, align="center")],
                  gap=18, sz={"width": "fill", "height": "hug", "max_width": 680}),
            imagen("photo", ("Retrato a la luz de una ventana, con fondo claro y sin poses forzadas",
                             "Portrait by window light against a pale background, with no forced poses"),
                   ratio=1.9, prioridad=True, radio=0),
        ], pad_v=64, gap=40),
        bloque_valores(("Lo que fotografiamos", "What we photograph"), [
            (("Retratos", "Portraits"), ("Personales, profesionales o de pareja. En estudio o en la calle.",
                                         "Personal, professional or couples. In the studio or outdoors.")),
            (("Familia", "Family"), ("Sesiones tranquilas con niños, abuelos y mascotas.",
                                     "Relaxed sessions with children, grandparents and pets.")),
            (("Producto", "Product"), ("Fotos limpias para tu tienda o tu catálogo.",
                                       "Clean photos for your shop or catalogue.")),
            (("Eventos y bodas", "Events and weddings"), ("Cobertura del día, sin que tengas que posar.",
                                                           "Coverage of the day, without you having to pose.")),
        ], cols=4, estilo="linea", tono="base"),
        galeria_mixta(("Un vistazo al portafolio", "A look at the portfolio"), [
            (("Pareja riendo en un paseo al atardecer", "Couple laughing on an evening walk"), 0.8),
            (("Mesa de una celebración con luz natural", "Celebration table in natural light"), 1.5),
            (("Retrato de una mujer con la mirada al frente", "Portrait of a woman looking straight ahead"), 0.8),
            (("Niños corriendo en un parque", "Children running in a park"), 1.5),
            (("Detalle de unas manos con alianzas", "Close-up of hands with wedding rings"), 0.8),
            (("Familia sentada en las escaleras de casa", "Family sitting on the front steps"), 1.5)],
            tono="relleno"),
        bloque_pasos(("Cómo es una sesión", "What a session is like"), [
            (("Hablamos", "We talk"), ("Nos cuentas qué quieres y para qué usarás las fotos.",
                                       "You tell us what you want and what the photos are for.")),
            (("Elegimos fecha y sitio", "We pick a date and place"),
             ("Con luz natural o en estudio, a la hora que mejor quede.", "In natural light or in the studio, at the best time of day.")),
            (("Hacemos la sesión", "We do the session"), ("Sin prisa y sin poses forzadas. Te vamos guiando.",
                                                          "No rush and no forced poses. We guide you as we go.")),
            (("Recibes tus fotos", "You get your photos"), ("Eliges tus favoritas y las recibimos editadas en una galería privada.",
                                                           "You pick your favourites and we send them edited in a private gallery.")),
        ], tono="base"),
        bloque_opiniones(("Lo que dicen quienes han venido", "What past clients say"),
                         [("la sesión", "the session"), ("las fotos", "the photos"), ("el trato", "the service")]),
        bloque_cta(("¿Tienes una fecha en mente?", "Do you have a date in mind?"),
                   ("Dinos el tipo de sesión y el día. Te decimos si está libre y cuánto cuesta.",
                    "Tell us the kind of session and the day. We will tell you if it is free and what it costs."),
                   [reservar(), llamar("ghost")], tono="base"),
    ])

    porta = pagina("portafolio", ("portafolio", "portfolio"), ("Portafolio", "Portfolio"),
                   ("Fotos de retratos, familia, producto, eventos y bodas, agrupadas por tipo de sesión.",
                    "Photos of portraits, families, products, events and weddings, grouped by type of session."), [
        hero(("Portafolio", "Portfolio"),
             ("Un poco de cada tipo de sesión. Si buscas algo concreto y no lo ves aquí, pregúntanos.",
              "A little of each kind of session. If you are after something specific and do not see it here, just ask."),
             [reservar()], variante="texto"),
        galeria_mixta(("Retratos y familia", "Portraits and family"), [
            (("Retrato en blanco y negro de un hombre con barba", "Black and white portrait of a bearded man"), 0.8),
            (("Madre e hija abrazadas en un campo", "Mother and daughter hugging in a field"), 1.5),
            (("Retrato de pareja junto a una pared de piedra", "Couple portrait beside a stone wall"), 0.8),
            (("Abuelo con su nieta leyendo un libro", "Grandfather reading a book with his granddaughter"), 1.5),
            (("Retrato profesional con chaqueta y fondo gris", "Professional portrait in a jacket against a grey backdrop"), 0.8),
            (("Familia con un perro en el salón", "Family with a dog in the living room"), 1.5)],
            anchor="retratos"),
        galeria_mixta(("Producto", "Product"), [
            (("Botella de aceite sobre fondo blanco", "Oil bottle on a white background"), 1.0),
            (("Cerámica artesana sobre una mesa de madera", "Handmade ceramics on a wooden table"), 1.0),
            (("Joya sobre tela oscura con luz lateral", "Piece of jewellery on dark cloth with side lighting"), 1.0)],
            tipo="product", tono="relleno", anchor="producto"),
        galeria_mixta(("Eventos y bodas", "Events and weddings"), [
            (("Novios saliendo de la ceremonia entre invitados", "Couple leaving the ceremony among guests"), 1.5),
            (("Brindis durante una cena de empresa", "Toast during a company dinner"), 0.8),
            (("Ramo de novia sobre un banco", "Bridal bouquet on a bench"), 0.8),
            (("Baile de los novios en el salón", "The couple's first dance in the hall"), 1.5),
            (("Charla en un congreso con público", "Talk at a conference with an audience"), 1.5),
            (("Detalle de la mesa de un banquete", "Detail of a banquet table"), 0.8)],
            anchor="eventos"),
        bloque_cta(("¿Te gusta lo que ves?", "Like what you see?"), None, [reservar(), whatsapp("ghost")], tono="relleno"),
    ])

    sesiones = pagina("sesiones", ("sesiones", "sessions"), ("Sesiones y precios", "Sessions and prices"),
                      ("Packs de retrato, familia, producto, eventos y bodas con precio, plazo de entrega, derechos de imagen y preguntas frecuentes.",
                       "Portrait, family, product, event and wedding packs with prices, delivery time, image rights and frequently asked questions."), [
        hero(("Sesiones y precios", "Sessions and prices"),
             ("Elige el pack que se parezca a lo que necesitas. Si no encaja del todo, lo ajustamos.",
              "Choose the pack closest to what you need. If it does not fit exactly, we adjust it."),
             [reservar()], variante="texto"),
        bloque_precios(("Packs", "Packs"), ("Packs de fotografía", "Photography packs"),
                       [("Pack", "Pack"), ("Qué incluye", "What is included"), ("Precio", "Price")],
                       [(("Mini sesión de retrato", "Mini portrait session"),
                         ("30 minutos y 10 fotos editadas", "30 minutes and 10 edited photos"), desde(70)),
                        (("Sesión de retrato o familia", "Portrait or family session"),
                         ("1 hora y 25 fotos editadas", "1 hour and 25 edited photos"), desde(140)),
                        (("Sesión de producto", "Product session"),
                         ("Hasta 10 productos con fondo blanco o ambientado", "Up to 10 products on a white or styled background"), desde(120)),
                        (("Evento", "Event"),
                         ("2 horas de cobertura y galería online", "2 hours of coverage and an online gallery"), desde(220)),
                        (("Boda", "Wedding"),
                         ("Ceremonia y celebración; lo vemos contigo", "Ceremony and celebration; we work it out with you"),
                         ("Según el día", "Depends on the day"))],
                       nota=nota, boton_=reservar(), anchor="precios", tono="base"),
        bloque_texto(("Entrega de las fotos", "How you get your photos"), [
            ("Las fotos se entregan editadas en una galería privada, con descarga en alta resolución para imprimir y en tamaño pequeño para redes.",
             "Photos are delivered edited in a private gallery, with a high-resolution download for printing and a small size for social media."),
            ("Plazo de entrega de ejemplo: 10 días laborables para retratos, familia y producto, y 4 semanas para bodas. Cámbialo por tu plazo real.",
             "Example delivery time: 10 working days for portraits, family and product, and 4 weeks for weddings. Change it to your real lead time."),
        ], eyebrow=("Plazos", "Timing"), tono="relleno"),
        bloque_texto(("Derechos de imagen, en llano", "Image rights, in plain words"), [
            ("Las fotos de tu sesión son para ti: puedes imprimirlas, regalarlas y subirlas a tus redes. No puedes venderlas ni usarlas en publicidad sin pactarlo antes.",
             "The photos from your session are for you: you can print them, give them away and post them on your social media. You cannot sell them or use them in advertising without agreeing it first."),
            ("Si queremos enseñar tus fotos en nuestra web o en redes, te lo preguntamos antes y no publicamos nada sin tu permiso. Con menores, firma su madre, su padre o su tutor.",
             "If we want to show your photos on our website or social media, we ask first and publish nothing without your permission. For children, a parent or guardian signs."),
            ("Este texto es de ejemplo: cámbialo por lo que acuerdes tú con tus clientes.",
             "This text is an example: change it to whatever you agree with your clients."),
        ], eyebrow=("Cuidado con las fotos", "Looking after the photos"), tono="base"),
        bloque_faq(("Preguntas frecuentes", "Frequently asked questions"), [
            (("¿Qué pasa si llueve?", "What if it rains?"),
             ("Si la sesión es en exterior, movemos la fecha sin coste o la pasamos al estudio.",
              "If the session is outdoors, we move the date at no cost or switch to the studio.")),
            (("¿Cómo reservo la fecha?", "How do I book the date?"),
             ("Nos dices el tipo de sesión y el día. Cuando lo confirmamos, se paga una señal para reservarlo y el resto, el día de la sesión.",
              "You tell us the type of session and the day. Once we confirm it, a deposit secures it and the rest is paid on the day.")),
            (("¿Puedo elegir las fotos?", "Can I choose the photos?"),
             ("Sí. Te enseñamos una selección y eliges tus favoritas, que editamos con más detalle.",
              "Yes. We show you a selection and you pick your favourites, which we edit in more detail.")),
            (("¿Hacéis fotos fuera del estudio?", "Do you shoot away from the studio?"),
             ("Sí, en tu casa, en un evento o en el lugar que elijas. Dinos dónde y lo valoramos.",
              "Yes, at your home, at an event or wherever you choose. Tell us where and we will assess it.")),
        ], tono="relleno"),
        bloque_cta(("¿Reservamos tu fecha?", "Shall we book your date?"), None, [reservar(), whatsapp("ghost")], tono="base"),
    ])

    cont = pagina("contacto", ("contacto", "contact"), ("Contacto y reserva", "Contact and booking"),
                  ("Reserva tu sesión con el formulario, por teléfono o por WhatsApp. Horario y cómo llegar al estudio.",
                   "Book your session with the form, by phone or on WhatsApp. Opening hours and how to find the studio."), [
        hero(("Reserva tu fecha", "Book your date"),
             ("Cuéntanos el tipo de sesión y el día que tienes en mente. Te respondemos para confirmarlo.",
              "Tell us the type of session and the day you have in mind. We reply to confirm it."),
             [llamar(), whatsapp()], variante="texto"),
        bloque_contacto(("Dónde estamos", "Where we are"),
                        ("Tenemos estudio y también vamos adonde nos necesites.",
                         "We have a studio and we also come to wherever you need us."),
                        [campo("nombre", "text", ("Nombre", "Name"), True, auto="name", maxlen=120),
                         campo("telefono", "tel", ("Teléfono", "Phone"), True, auto="tel", maxlen=30),
                         campo("correo", "email", ("Correo electrónico", "Email"), False, auto="email", maxlen=200),
                         campo("sesion", "select", ("Tipo de sesión", "Type of session"), True,
                               opciones=[("Retrato", "Portrait"), ("Familia", "Family"), ("Producto", "Product"),
                                         ("Evento", "Event"), ("Boda", "Wedding"), ("Otro", "Other")]),
                         campo("fecha", "date", ("Fecha que tienes en mente", "Date you have in mind")),
                         campo("lugar", "text", ("Lugar (estudio, exterior u otro)", "Place (studio, outdoors or other)"),
                               False, maxlen=200),
                         campo("mensaje", "textarea", ("Cuéntanos qué necesitas (opcional)", "Tell us what you need (optional)"),
                               False, maxlen=1000)],
                        ("Reservar fecha", "Request date"), horas,
                        exito=("Gracias. Te confirmamos si la fecha está libre por teléfono, WhatsApp o correo.",
                               "Thank you. We will confirm whether the date is free by phone, WhatsApp or email.")),
        bloque_mapa(tono="relleno"),
    ])

    return plantilla(KEY, ("Fotógrafo", "Photographer"),
                     ("Portafolio por tipo de sesión, packs con precio, entrega, derechos de imagen, preguntas frecuentes y reserva de fecha con tipo de sesión.",
                      "Portfolio by type of session, priced packs, delivery, image rights, FAQ and date booking with session type."),
                     "creativos", ("fotografo fotografa fotografia fotos retratos retrato familia producto eventos bodas boda sesion estudio fotografico book reportaje",
                                   "photographer photography photos portrait family product events weddings session studio shoot"),
                     look, cabecera(nav, reservar()),
                     pie(nav, ("Fotografía de retrato, familia, producto, eventos y bodas.",
                               "Portrait, family, product, event and wedding photography."),
                         ("Lunes a viernes 10:00 a 14:00 y 16:00 a 19:00. Sábado con cita.",
                          "Monday to Friday 10 am to 2 pm and 4 pm to 7 pm. Saturday by appointment.")),
                     [inicio, porta, sesiones, cont])
