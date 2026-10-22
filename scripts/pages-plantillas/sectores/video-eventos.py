# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
from lib import *
from looks import LOOKS  # noqa: F401  (la paleta es propia de este sector)

KEY = "video-eventos"

# Paleta propia: sala de montaje (gris casi negro) con el rosa-rojo del piloto de grabación como acento.
# primary/inverse >= 3, así que se puede usar `tono="oscuro"`.
SALA = dict(
    colores=dict(background="#121214", surface="#1c1c20", text="#f4f4f1", muted="#b6b6b0", primary="#ff6b8a",
                 on_primary="#121214", border="#2f2f36", inverse="#08080a", on_inverse="#f4f4f1"),
    fuentes=("dm-sans", "dm-sans"), peso=700, h1=(56, 44, 34), radio=(6, 10, 14))


def fotogramas(titulo, alts, intro=None, eyebrow=None, cols=2, ratio=1.78, tono="base", anchor=None):
    """Fotogramas de relleno (no hay reproductor): imágenes apaisadas con un alt que describe la escena."""
    return bloque_galeria(titulo, alts, intro=intro, eyebrow=eyebrow, tipo="landscape", cols=cols, ratio=ratio,
                          tono=tono, anchor=anchor)


def build():
    nueva(KEY)
    look = SALA
    nav = [("inicio", ("Inicio", "Home")), ("servicios", ("Packs y formatos", "Packs and formats")),
           ("trabajos", ("Trabajos", "Work")), ("contacto", ("Presupuesto", "Quote"))]
    horas = [(("Lunes a viernes", "Monday to Friday"), ("9:30 a 18:30", "9:30 am to 6:30 pm")),
             (("Sábado y domingo", "Saturday and Sunday"),
              ("Rodajes y eventos con cita", "Shoots and events by appointment"))]
    nota = ("Precios de ejemplo: cámbialos por los tuyos. Las horas de rodaje, las rondas de cambios y las entregas incluidas también son de ejemplo: cámbialas por las tuyas. Incluyen IVA.",
            "Example prices: replace them with your own. The shooting hours, revision rounds and deliverables included are examples too: replace them with your own. VAT included.")

    def presupuesto(variante="primary"):
        return boton(("Pedir presupuesto", "Ask for a quote"), a_pagina("contacto", "contacto"), variante)

    inicio = pagina("inicio", ("", ""), ("Productora de vídeo", "Video production"),
                    ("Vídeo para bodas, eventos, empresas y redes sociales: qué hacemos, fases de trabajo, portfolio y presupuesto en 24 horas.",
                     "Video for weddings, events, companies and social media: what we do, how we work, portfolio and a quote within 24 hours."), [
        hero(("Vídeo para tus eventos y tu empresa", "Video for your events and your company"),
             ("Grabamos bodas, eventos, vídeos corporativos y piezas para redes. Te contamos cada fase y cuánto cuesta antes de empezar.",
              "We film weddings, events, corporate videos and pieces for social media. We explain each stage and the cost before we start."),
             [presupuesto(), whatsapp()],
             eyebrow=("Productora de vídeo", "Video production"),
             imagen_=("landscape", ("Cámara sobre trípode grabando una celebración en un salón",
                                    "Camera on a tripod filming a celebration in a hall"))),
        bloque_valores(("Qué hacemos", "What we do"), [
            (("Bodas", "Weddings"), ("La ceremonia y la celebración contadas con calma, en una pieza que se ve más de una vez.",
                                     "The ceremony and the celebration told calmly, in a piece worth watching more than once.")),
            (("Eventos", "Events"), ("Congresos, presentaciones, fiestas y aniversarios, con resumen corto y versión larga.",
                                     "Conferences, launches, parties and anniversaries, with a short recap and a long cut.")),
            (("Vídeo corporativo", "Corporate video"), ("Presentación de tu empresa, producto o equipo, con entrevistas y planos de recurso.",
                                                       "An introduction to your company, product or team, with interviews and b-roll.")),
            (("Redes sociales", "Social media"), ("Piezas cortas y verticales para Instagram, TikTok y YouTube Shorts.",
                                                  "Short vertical pieces for Instagram, TikTok and YouTube Shorts.")),
        ], cols=4, estilo="relleno", tono="base"),
        bloque_pasos(("Cómo trabajamos", "How we work"), [
            (("Guion", "Script"), ("Hablamos contigo, definimos qué quieres contar y preparamos un guion o una lista de planos.",
                                   "We talk with you, define what you want to tell and prepare a script or shot list.")),
            (("Rodaje", "Shoot"), ("Vamos con el equipo justo, sin entorpecer el evento ni a los invitados.",
                                   "We turn up with just the crew we need, without getting in the way of the event or the guests.")),
            (("Edición", "Edit"), ("Montamos, corregimos el color y el sonido y te enseñamos una primera versión.",
                                   "We cut, colour-grade and mix the sound, then show you a first version.")),
            (("Entrega", "Delivery"), ("Atendemos tus cambios y te enviamos los archivos finales en los formatos que necesites.",
                                       "We handle your changes and send the final files in the formats you need.")),
        ], tono="oscuro"),
        fotogramas(("Algunos fotogramas", "A few frames"), [
            ("Fotograma de un vídeo de boda: los novios en la ceremonia", "Frame from a wedding video: the couple at the ceremony"),
            ("Fotograma de un evento de empresa con el público aplaudiendo", "Frame from a company event with the audience applauding"),
            ("Fotograma de una entrevista corporativa en una oficina", "Frame from a corporate interview in an office"),
            ("Fotograma de una pieza para redes en formato apaisado", "Frame from a social media piece in landscape format")],
            tono="base"),
        bloque_opiniones(("Lo que dicen quienes nos han contratado", "What clients who hired us say"),
                         [("el vídeo", "the video"), ("el rodaje", "the shoot"), ("la entrega", "the delivery")]),
        bloque_cta(("Cuéntanos qué quieres grabar", "Tell us what you want to film"),
                   ("Rellena el formulario y te enviamos un presupuesto en 24 horas laborables.",
                    "Fill in the form and we send you a quote within 24 working hours."),
                   [presupuesto(), llamar("ghost")], tono="relleno"),
    ])

    serv = pagina("servicios", ("servicios", "services"), ("Packs y formatos", "Packs and formats"),
                  ("Packs de vídeo para bodas, eventos, empresas y redes, formatos de entrega y qué necesitamos de ti.",
                   "Video packs for weddings, events, companies and social media, delivery formats and what we need from you."), [
        hero(("Packs y formatos de entrega", "Packs and delivery formats"),
             ("Un punto de partida para que te hagas una idea. Cada vídeo es distinto, así que el presupuesto final se ajusta a lo que necesites.",
              "A starting point so you get an idea. Every video is different, so the final quote is adjusted to what you need."),
             [presupuesto()], variante="texto"),
        bloque_precios(("Packs", "Packs"), ("Packs de vídeo", "Video packs"),
                       [("Pack", "Pack"), ("Qué incluye", "What is included"), ("Precio", "Price")],
                       [(("Boda", "Wedding"),
                         ("Ceremonia y celebración, vídeo de 5 a 8 minutos y resumen de 1 minuto",
                          "Ceremony and celebration, a 5 to 8 minute film and a 1 minute recap"), desde(900)),
                        (("Evento", "Event"),
                         ("Hasta 3 horas de rodaje y vídeo resumen de 2 a 3 minutos",
                          "Up to 3 hours of filming and a 2 to 3 minute recap video"), desde(450)),
                        (("Vídeo corporativo", "Corporate video"),
                         ("Guion, jornada de rodaje, entrevistas y un vídeo de 1 a 2 minutos",
                          "Script, one shooting day, interviews and a 1 to 2 minute video"), desde(750)),
                        (("Pack de redes", "Social media pack"),
                         ("Medio día de rodaje y 4 piezas verticales de hasta 30 segundos",
                          "Half a day of filming and 4 vertical pieces up to 30 seconds"), desde(380))],
                       nota=nota, boton_=presupuesto(), anchor="packs", tono="base"),
        bloque_precios(("Formatos de entrega", "Delivery formats"), ("Formatos de entrega", "Delivery formats"),
                       [("Formato", "Format"), ("Para qué sirve", "What it is for")],
                       [(("Horizontal 16:9", "Horizontal 16:9"), ("Web, YouTube, proyector y pantallas.", "Websites, YouTube, projectors and screens.")),
                        (("Vertical 9:16", "Vertical 9:16"), ("Instagram, TikTok, Reels y Shorts.", "Instagram, TikTok, Reels and Shorts.")),
                        (("Cuadrado 1:1", "Square 1:1"), ("Publicaciones en el muro de redes.", "Feed posts on social media.")),
                        (("Con subtítulos", "With subtitles"), ("Para verlo sin sonido o en otro idioma.", "To watch without sound or in another language.")),
                        (("Archivo para pantalla grande", "Large-screen file"), ("Mayor calidad para proyectar en una sala.", "Higher quality for projecting in a hall."))],
                       intro=("Te entregamos los que necesites en un enlace de descarga.",
                              "We deliver the ones you need in a download link."),
                       anchor="formatos", tono="relleno", kind="features"),
        bloque_checks(("Qué necesitamos de ti", "What we need from you"), [
            ("La fecha, el lugar y la hora del evento.", "The date, place and time of the event."),
            ("Qué quieres que se vea y para qué lo vas a usar.", "What you want to see and what you will use it for."),
            ("Música que te guste, o nos encargamos nosotros de buscarla con licencia.", "Music you like, or we find licensed music for you."),
            ("Aviso a los asistentes de que habrá cámaras. En eventos con público, lo hacemos con un cartel.", "Letting attendees know cameras will be there. At public events, we do it with a sign."),
        ], tono="base", cols=2),
        bloque_faq(("Preguntas frecuentes", "Frequently asked questions"), [
            (("¿Cuánto tardáis en entregar?", "How long until delivery?"),
             ("Plazo de ejemplo: 15 días laborables para eventos y vídeo corporativo, y 6 semanas para bodas. Cámbialo por el tuyo.",
              "Example lead time: 15 working days for events and corporate video, and 6 weeks for weddings. Change it to yours.")),
            (("¿Puedo pedir cambios?", "Can I ask for changes?"),
             ("Sí. Tras la primera versión puedes pedir cambios de montaje, orden y música. Rondas de cambios de ejemplo: dos. Cámbialo por lo que ofrezcas.",
              "Yes. After the first version you can ask for edits to the cut, order and music. Example revision rounds: two. Change it to what you offer.")),
            (("¿Y la música?", "What about the music?"),
             ("Usamos música con licencia. Si quieres una canción concreta, avísanos: hay que pedir su licencia y tiene un coste aparte.",
              "We use licensed music. If you want a specific song, tell us: its licence has to be requested and costs extra.")),
            (("¿Cómo se paga?", "How do I pay?"),
             ("Una señal para reservar la fecha y el resto al entregar el vídeo final.",
              "A deposit to book the date and the rest when we deliver the final video.")),
        ], tono="relleno"),
        bloque_cta(("¿Hablamos de tu vídeo?", "Shall we talk about your video?"), None, [presupuesto(), whatsapp("ghost")], tono="base"),
    ])

    trabajos = pagina("trabajos", ("trabajos", "work"), ("Trabajos", "Work"),
                      ("Fotogramas de vídeos de bodas, eventos, empresas y redes sociales.",
                       "Frames from wedding, event, corporate and social media videos."), [
        hero(("Trabajos", "Work"),
             ("Fotogramas de algunos de nuestros vídeos, por tipo de encargo. Si quieres ver una pieza completa, pídenosla.",
              "Frames from some of our videos, by kind of job. If you want to see a full piece, just ask."),
             [presupuesto()], variante="texto"),
        fotogramas(("Bodas", "Weddings"), [
            ("Fotograma de una boda: entrada de los novios al salón", "Frame from a wedding: the couple entering the hall"),
            ("Fotograma de una boda: intercambio de anillos", "Frame from a wedding: the exchange of rings"),
            ("Fotograma de una boda: baile en la celebración", "Frame from a wedding: dancing at the celebration"),
            ("Fotograma de una boda: detalle de la mesa", "Frame from a wedding: table detail")],
            cols=2, anchor="bodas"),
        fotogramas(("Eventos y empresas", "Events and companies"), [
            ("Fotograma de un congreso: ponente en el escenario", "Frame from a conference: speaker on stage"),
            ("Fotograma de una presentación de producto", "Frame from a product launch"),
            ("Fotograma de una entrevista a una persona del equipo", "Frame from an interview with a team member"),
            ("Fotograma de la fábrica con maquinaria en marcha", "Frame from a factory floor with machinery running")],
            cols=2, tono="relleno", anchor="empresas"),
        bloque_galeria(("Redes sociales", "Social media"), [
            ("Fotograma vertical de una pieza para redes: producto en primer plano", "Vertical frame from a social piece: product in the foreground"),
            ("Fotograma vertical de una pieza para redes: persona hablando a cámara", "Vertical frame from a social piece: person talking to camera"),
            ("Fotograma vertical de una pieza para redes: detalle de un plato", "Vertical frame from a social piece: close-up of a dish"),
            ("Fotograma vertical de una pieza para redes: ambiente de un evento", "Vertical frame from a social piece: event atmosphere")],
            tipo="landscape", cols=4, ratio=0.56, anchor="redes"),
        peq_nota(),
        bloque_cta(("¿Qué te gustaría contar?", "What would you like to tell?"), None, [presupuesto(), whatsapp("ghost")], tono="relleno"),
    ])

    cont = pagina("contacto", ("contacto", "contact"), ("Presupuesto y contacto", "Quote and contact"),
                  ("Pide presupuesto de vídeo con el formulario, por teléfono o por WhatsApp y recíbelo en 24 horas laborables.",
                   "Ask for a video quote with the form, by phone or on WhatsApp and get it within 24 working hours."), [
        hero(("Pide tu presupuesto", "Ask for your quote"),
             ("Cuéntanos qué quieres grabar y para cuándo. Te enviamos el presupuesto en 24 horas laborables.",
              "Tell us what you want to film and when for. We send you the quote within 24 working hours."),
             [llamar(), whatsapp()], variante="texto"),
        bloque_contacto(("Cómo localizarnos", "How to reach us"),
                        ("Podemos vernos en nuestra sede o en una videollamada.", "We can meet at our office or on a video call."),
                        [campo("nombre", "text", ("Nombre", "Name"), True, auto="name", maxlen=120),
                         campo("correo", "email", ("Correo electrónico", "Email"), True, auto="email", maxlen=200),
                         campo("telefono", "tel", ("Teléfono (opcional)", "Phone (optional)"), False, auto="tel", maxlen=30),
                         campo("tipo", "select", ("Qué quieres grabar", "What you want to film"), True,
                               opciones=[("Boda", "Wedding"), ("Evento", "Event"), ("Vídeo corporativo", "Corporate video"),
                                         ("Piezas para redes", "Social media pieces"), ("Otro", "Other")]),
                         campo("fecha", "date", ("Fecha del evento o del rodaje", "Event or shoot date")),
                         campo("lugar", "text", ("Ciudad o lugar", "City or venue"), False, maxlen=200),
                         campo("mensaje", "textarea", ("Cuéntanos la idea", "Tell us the idea"), True, maxlen=2000)],
                        ("Pedir presupuesto", "Ask for a quote"), horas,
                        exito=("Gracias. Te enviamos el presupuesto por correo en 24 horas laborables.",
                               "Thank you. We will email you the quote within 24 working hours.")),
        bloque_mapa(tono="relleno"),
    ])

    return plantilla(KEY, ("Productora de vídeo", "Video production"),
                     ("Vídeo para bodas, eventos y empresas: packs con precio, fases de trabajo, formatos de entrega, fotogramas de trabajos y presupuesto en 24 horas.",
                      "Video for weddings, events and companies: priced packs, stages, delivery formats, frames from past work and a quote within 24 hours."),
                     "creativos", ("video videografo videografa productora audiovisual videos bodas eventos corporativo empresa redes sociales reels rodaje edicion presupuesto",
                                   "video production videographer filming wedding video events corporate video social media reels editing quote"),
                     look, cabecera(nav, presupuesto()),
                     pie(nav, ("Productora de vídeo para bodas, eventos, empresas y redes.",
                               "Video production for weddings, events, companies and social media."),
                         ("Lunes a viernes 9:30 a 18:30. Rodajes con cita.", "Monday to Friday 9:30 am to 6:30 pm. Shoots by appointment.")),
                     [inicio, serv, trabajos, cont])


def peq_nota():
    return seccion("text", [peq(("Fotogramas de ejemplo: cámbialos por los de tus trabajos.",
                                 "Example frames: replace them with frames from your own work."), color="$muted")],
                   pad_v=24, gap=0)
