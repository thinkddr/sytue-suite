# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
from lib import *
from looks import LOOKS  # noqa: F401  (la paleta es propia de este sector)

KEY = "musico-banda"

# Paleta propia: berenjena de teatro con rosa orquídea. Fondo morado profundo (no el casi negro de las otras
# plantillas oscuras) y primario rosa claro; el rosa pasa AA y 3:1 sobre `inverse`.
TELON = dict(
    colores=dict(background="#1a1230", surface="#261a45", text="#f4eefc", muted="#c3b6dd", primary="#f0abfc",
                 on_primary="#1a1230", border="#3d2d66", inverse="#0f0a1f", on_inverse="#f4eefc"),
    fuentes=("playfair-display", "manrope"), peso=700, h1=(56, 44, 34), radio=(4, 10, 16))


def build():
    nueva(KEY)
    look = TELON
    nav = [("inicio", ("Inicio", "Home")), ("repertorio", ("Repertorio y demos", "Repertoire and demos")),
           ("formaciones", ("Formaciones y packs", "Line-ups and packages")), ("contacto", ("Fecha libre", "Check a date"))]
    horas = [(("Lunes a viernes", "Monday to Friday"),
              ("11:00 a 14:00 y 17:00 a 20:00", "11:00 am to 2:00 pm and 5:00 pm to 8:00 pm")),
             (("Sábado y domingo", "Saturday and Sunday"), ("Actuamos; consultas por WhatsApp", "We perform; enquiries by WhatsApp"))]
    nota = ("Precios de ejemplo: cámbialos por los tuyos. Incluyen IVA. El precio final depende de la fecha, el lugar, los kilómetros y las horas.",
            "Example prices: replace them with your own. VAT included. The final price depends on the date, venue, distance and hours.")

    def fecha(var="primary"):
        return boton(("Consultar mi fecha", "Check my date"), a_pagina("contacto", "disponibilidad"), var)

    inicio = pagina("inicio", ("", ""), ("Música en directo", "Live music"),
                    ("Solista, dúo o banda para bodas, fiestas y locales: formaciones, repertorio por estilos, packs y consulta de fecha. Llama o escribe por WhatsApp.",
                     "Soloist, duo or band for weddings, parties and venues: line-ups, repertoire by style, packages and date enquiries. Call or message us on WhatsApp."), [
        hero(("Música en directo para el día que no se repite", "Live music for the day that happens once"),
             ("Cantante, dúo o banda para bodas, fiestas y locales. Tocamos lo que os gusta, con el volumen que pide el sitio.",
              "Singer, duo or band for weddings, parties and venues. We play what you like, at the volume the room calls for."),
             [fecha(), llamar("secondary"), whatsapp("ghost")], eyebrow=("Música en directo", "Live music"),
             imagen_=("photo", ("Escenario con instrumentos, atriles y focos de luz cálida, sin personas",
                                "Stage with instruments, music stands and warm spotlights, with no people"))),
        bloque_valores(("Tres formas de tocar", "Three ways to play"), [
            (("Solista", "Soloist"), ("Una voz y una guitarra o un teclado. Para ceremonias, aperitivos y cenas tranquilas.",
                                      "One voice and a guitar or keyboard. For ceremonies, drinks receptions and quiet dinners.")),
            (("Dúo", "Duo"), ("Dos voces o voz e instrumento. Más cuerpo sin ocupar mucho: buen término medio.",
                              "Two voices or voice and instrument. More body without taking much room: a good middle ground.")),
            (("Banda", "Band"), ("Cuatro o más músicos con batería, bajo y guitarras. Para la pista de baile.",
                                 "Four or more musicians with drums, bass and guitars. For the dance floor.")),
        ], cols=3, estilo="borde", tono="base", anchor="formaciones-resumen",
            intro=("Los detalles de cada formación están en su página.", "The details of each line-up are on their page.")),
        bloque_texto(("Escucha antes de decidir", "Listen before you decide"), [
            ("Te enviamos por correo o WhatsApp una selección de grabaciones del repertorio que te interesa. Aquí no hay reproductor: lo describimos en la página de repertorio.",
             "We send you a selection of recordings of the repertoire you are interested in by email or WhatsApp. There is no player here: we describe it on the repertoire page."),
            ("Si puedes, ven a vernos en directo en un local. Te decimos dónde y cuándo tocamos próximamente.",
             "If you can, come and see us live at a venue. We will tell you where and when we play next."),
        ], imagen_=("landscape", ("Sala pequeña con luces de escenario y una guitarra apoyada en un taburete",
                                  "Small room with stage lights and a guitar resting on a stool")),
            tono="relleno", eyebrow=("Demos", "Demos")),
        bloque_checks(("Dónde tocamos", "Where we play"), [
            ("Bodas: ceremonia, aperitivo, banquete y baile.", "Weddings: ceremony, drinks reception, dinner and dancing."),
            ("Fiestas privadas: cumpleaños, aniversarios y despedidas.", "Private parties: birthdays, anniversaries and stag or hen parties."),
            ("Bares, restaurantes y hoteles, con repertorio a medida del local.", "Bars, restaurants and hotels, with repertoire tailored to the venue."),
            ("Eventos de empresa: cenas, inauguraciones y fiestas de equipo.", "Company events: dinners, openings and team parties."),
            ("Fiestas de pueblo y verbenas.", "Village festivals and open-air dances."),
            ("Bautizos, comuniones y otras celebraciones familiares.", "Christenings, communions and other family celebrations."),
        ], eyebrow=("Ocasiones", "Occasions"), tono="base", cols=2, anchor="ocasiones"),
        bloque_pasos(("De la fecha al escenario", "From date to stage"), [
            (("Consultas la fecha", "You check the date"),
             ("Nos dices día, lugar y tipo de evento. Te contestamos si estamos libres.",
              "Tell us the day, place and type of event. We reply to say whether we are free.")),
            (("Elegimos formación y repertorio", "We choose line-up and repertoire"),
             ("Hablamos del ambiente que buscáis y proponemos la formación y los estilos.",
              "We talk about the atmosphere you want and suggest the line-up and styles.")),
            (("Presupuesto y reserva", "Quote and booking"),
             ("Te enviamos el precio cerrado. Se reserva la fecha con una señal (texto de ejemplo).",
              "We send a fixed price. The date is booked with a deposit (example text).")),
            (("Llega el día", "The day arrives"),
             ("Llegamos antes, montamos, probamos el sonido y tocamos.",
              "We arrive early, set up, sound check and play.")),
        ], tono="relleno"),
        bloque_opiniones(("Lo que dicen quienes nos contrataron", "What our clients say"),
                         [("el repertorio", "the repertoire"), ("la puntualidad", "the punctuality"), ("el ambiente en la pista", "the dance floor atmosphere")],
                         tono="base"),
        bloque_cta(("¿Ya tenéis fecha?", "Do you have a date yet?"),
                   ("Decidnos el día y el sitio y os decimos si estamos libres.",
                    "Tell us the day and the place and we will tell you whether we are free."),
                   [fecha(), whatsapp("ghost")], tono="relleno"),
    ])

    repertorio = pagina("repertorio", ("repertorio", "repertoire"), ("Repertorio y demos", "Repertoire and demos"),
                        ("Repertorio por estilos con lista de ejemplo, cómo se describen las demos, canciones a petición y preguntas frecuentes.",
                         "Repertoire by style with an example list, how the demos are described, requested songs and frequently asked questions."), [
        hero(("Repertorio y demos", "Repertoire and demos"),
             ("Esto es lo que solemos tocar. Si la canción que buscáis no está, preguntadnos: muchas veces la preparamos.",
              "This is what we usually play. If the song you want is not here, ask us: we often prepare it."),
             [fecha(), whatsapp("secondary")], variante="texto"),
        bloque_precios(("Repertorio por estilos", "Repertoire by style"), ("Estilos, cuándo encajan y formación", "Styles, when they fit and line-up"),
                       [("Estilo", "Style"), ("Cuándo encaja", "When it fits"), ("Formación", "Line-up")],
                       [(("Versiones acústicas suaves", "Soft acoustic covers"), ("Ceremonia y aperitivo", "Ceremony and drinks reception"), ("Solista o dúo", "Soloist or duo")),
                        (("Pop y rock español", "Spanish pop and rock"), ("Cena y baile", "Dinner and dancing"), ("Dúo o banda", "Duo or band")),
                        (("Pop internacional de los 80 y 90", "International pop of the 80s and 90s"), ("Baile", "Dancing"), ("Banda", "Band")),
                        (("Baladas", "Ballads"), ("Primer baile y momentos especiales", "First dance and special moments"), ("Cualquiera", "Any")),
                        (("Rock clásico", "Classic rock"), ("Local y pista", "Venue and dance floor"), ("Banda", "Band")),
                        (("Latino: salsa, bachata y rumba", "Latin: salsa, bachata and rumba"), ("Pista abierta", "Open dance floor"), ("Dúo o banda", "Duo or band")),
                        (("Jazz, bossa y swing", "Jazz, bossa nova and swing"), ("Cenas y cócteles", "Dinners and cocktails"), ("Solista o dúo", "Soloist or duo")),
                        (("Fiesta popular y verbena", "Village party and open-air dance"), ("Fiestas de pueblo", "Village festivals"), ("Banda", "Band"))],
                       intro=("Lista de ejemplo: deja solo los estilos que de verdad tocáis.",
                              "Example list: keep only the styles you really play."),
                       anchor="estilos", tono="base"),
        bloque_valores(("Las demos, descritas", "The demos, described"), [
            (("Maqueta de repertorio", "Repertoire sampler"),
             ("Describe aquí qué grabaciones envías y cuánto duran (texto de ejemplo).",
              "Describe here which recordings you send and how long they are (example text).")),
            (("Directo en un local", "Live at a venue"),
             ("Indica aquí dónde puede verse un directo y cuándo (texto de ejemplo).",
              "State here where and when a live show can be seen (example text).")),
            (("Prueba para novios", "Trial for couples"),
             ("Escribe aquí si ofreces una prueba o una videollamada previa (texto de ejemplo).",
              "Write here whether you offer a trial or a prior video call (example text).")),
        ], intro=("Textos de ejemplo: describe solo lo que de verdad puedes enseñar. No hay reproductor en esta plantilla.",
                  "Example texts: describe only what you can really show. There is no player in this template."),
            cols=3, estilo="borde", tono="relleno", anchor="demos"),
        bloque_faq(("Preguntas frecuentes", "Frequently asked questions"), [
            (("¿Tocáis canciones a petición?", "Do you play requests?"),
             ("Sí, hasta cinco canciones que nos pidáis antes del evento. Las ensayamos para que suenen como tienen que sonar.",
              "Yes, up to five songs you ask for before the event. We rehearse them so they sound the way they should.")),
            (("¿Cuánto dura una actuación?", "How long is a performance?"),
             ("Normalmente en pases de 45 minutos con descansos de 15. Podemos adaptarlo al programa de tu evento.",
              "Usually in 45-minute sets with 15-minute breaks. We can adapt it to the schedule of your event.")),
            (("¿Ponéis música entre pases?", "Do you play music between sets?"),
             ("Sí, una lista de fondo para que no haya silencios. Si prefieres otra cosa, lo hablamos.",
              "Yes, a background playlist so there is no silence. If you prefer something else, we can talk about it.")),
            (("¿Quién paga los derechos de autor?", "Who pays the copyright fees?"),
             ("En eventos con público suele corresponder a quien organiza o al local. Pregúntanos y te orientamos.",
              "At events open to the public this usually falls to the organiser or the venue. Ask us and we will point you in the right direction.")),
        ], tono="base"),
        bloque_cta(("Contadnos qué música queréis", "Tell us what music you want"), None, [fecha(), whatsapp("ghost")], tono="relleno"),
    ])

    form = pagina("formaciones", ("formaciones", "line-ups"), ("Formaciones y packs", "Line-ups and packages"),
                  ("Solista, dúo y banda con lo que lleva cada uno, requisitos técnicos del sitio y packs con precios de ejemplo.",
                   "Soloist, duo and band with what each brings, technical requirements for the venue and packages with example prices."), [
        hero(("Formaciones y packs", "Line-ups and packages"),
             ("Elegid la formación que cabe en el sitio y en el presupuesto. Si dudáis, os aconsejamos.",
              "Choose the line-up that fits the venue and the budget. If you are unsure, we will advise you."),
             [fecha(), llamar("secondary")], variante="texto"),
        bloque_valores(("Qué lleva cada formación", "What each line-up brings"), [
            (("Solista", "Soloist"), ("Voz y guitarra o teclado, sonido propio pequeño. Cabe en una esquina del salón.",
                                      "Voice and guitar or keyboard with a small sound system. Fits in a corner of the room.")),
            (("Dúo", "Duo"), ("Dos músicos, sonido propio y repertorio con armonías. Aguanta una cena y un baile corto.",
                              "Two musicians, own sound system and a repertoire with harmonies. Holds up for a dinner and a short dance.")),
            (("Banda", "Band"), ("De cuatro a seis músicos, con equipo de sonido y luces propios. Necesita escenario o espacio amplio.",
                                 "Four to six musicians with their own sound and lights. Needs a stage or plenty of room.")),
        ], cols=3, estilo="relleno", tono="base", anchor="tarjetas"),
        bloque_checks(("Requisitos técnicos", "Technical requirements"), [
            ("Espacio mínimo: solista 2 x 2 m, dúo 3 x 2 m, banda 5 x 4 m (ejemplo).", "Minimum space: soloist 2 x 2 m, duo 3 x 2 m, band 5 x 4 m (example)."),
            ("Una o dos tomas de corriente de 230 V cerca del escenario.", "One or two 230 V power sockets near the stage."),
            ("Acceso para descargar el equipo y una hora antes para montar y probar.", "Access to unload the gear and one hour beforehand to set up and sound check."),
            ("Si es al aire libre, un sitio cubierto del sol, la lluvia y el viento.", "If outdoors, a spot sheltered from sun, rain and wind."),
            ("Un lugar para dejar los estuches y una mesa con agua.", "A place to leave the cases and a table with water."),
            ("Límite de volumen del local o del municipio, si lo hay: avísanos al reservar.", "Any volume limit set by the venue or the town: tell us when you book."),
        ], intro=("Requisitos de ejemplo: ajústalos a tu equipo.", "Example requirements: adjust them to your equipment."),
            eyebrow=("Lo que necesitamos del sitio", "What we need from the venue"), tono="relleno", cols=2, anchor="requisitos"),
        bloque_precios(("Packs", "Packages"), ("Packs musicales con formación, duración y precio", "Music packages with line-up, length and price"),
                       [("Pack", "Package"), ("Formación", "Line-up"), ("Duración", "Length"), ("Precio", "Price")],
                       [(("Ceremonia", "Ceremony"), ("Solista", "Soloist"), ("45 minutos", "45 minutes"), desde(250)),
                        (("Cena y cóctel", "Dinner and cocktails"), ("Solista o dúo", "Soloist or duo"), ("2 horas", "2 hours"), desde(380)),
                        (("Boda: ceremonia y banquete", "Wedding: ceremony and dinner"), ("Dúo", "Duo"), ("3 horas", "3 hours"), desde(650)),
                        (("Fiesta con baile", "Dance party"), ("Banda", "Band"), ("2 pases de 60 minutos", "2 sets of 60 minutes"), desde(1100)),
                        (("Bar o local", "Bar or venue"), ("Solista o dúo", "Soloist or duo"), ("1 hora y 30 minutos", "1 hour 30 minutes"), desde(220))],
                       nota=nota, anchor="packs", tono="base", boton_=fecha("secondary")),
        bloque_cta(("¿Qué formación os encaja?", "Which line-up suits you?"), None, [fecha(), whatsapp("ghost")], tono="relleno"),
    ])

    cont = pagina("contacto", ("fecha-libre", "check-a-date"), ("Fecha libre y contacto", "Check a date and contact"),
                  ("Consulta si tenemos tu fecha libre con el formulario, por teléfono o WhatsApp. Horario y cómo llegar.",
                   "Check whether we are free on your date with the form, by phone or WhatsApp. Opening hours and how to find us."), [
        hero(("¿Estamos libres vuestro día?", "Are we free on your day?"),
             ("Decidnos la fecha, el tipo de evento y dónde será. Os respondemos con disponibilidad y precio.",
              "Tell us the date, the type of event and where it will be. We reply with availability and price."),
             [llamar(), whatsapp()], variante="texto"),
        bloque_contacto(("Consulta tu fecha", "Check your date"),
                        ("Cuantos más datos nos des, más exacto será el presupuesto.", "The more details you give, the more accurate the quote."),
                        [campo("nombre", "text", ("Nombre", "Name"), True, auto="name", maxlen=120),
                         campo("telefono", "tel", ("Teléfono", "Phone"), True, auto="tel", maxlen=30),
                         campo("tipo", "select", ("Tipo de evento", "Type of event"), True,
                               opciones=[("Boda", "Wedding"), ("Fiesta privada", "Private party"),
                                         ("Bar, restaurante u hotel", "Bar, restaurant or hotel"),
                                         ("Evento de empresa", "Company event"), ("Fiesta popular", "Village festival"),
                                         ("Otro", "Other")]),
                         campo("fecha", "date", ("Fecha del evento", "Event date"), True),
                         campo("lugar", "text", ("Lugar o ciudad", "Venue or city"), True, maxlen=160),
                         campo("formacion", "select", ("Formación que os interesa", "Line-up you are interested in"), False,
                               opciones=[("Solista", "Soloist"), ("Dúo", "Duo"), ("Banda", "Band"), ("No lo sé todavía", "I do not know yet")]),
                         campo("invitados", "number", ("Invitados aproximados", "Approximate guests"), False),
                         campo("mensaje", "textarea", ("Estilos o canciones que no pueden faltar (opcional)", "Styles or songs you must have (optional)"), False, maxlen=1000)],
                        ("Consultar disponibilidad", "Check availability"), horas, anchor="disponibilidad",
                        notas=("Fuera de este horario respondemos por WhatsApp. Los fines de semana de temporada alta se llenan pronto: consulta con tiempo.",
                               "Outside these hours we reply on WhatsApp. High-season weekends fill up early: ask well in advance."),
                        exito=("Gracias. Te confirmamos la disponibilidad de tu fecha por teléfono o WhatsApp lo antes posible.",
                               "Thank you. We will confirm availability for your date by phone or WhatsApp as soon as possible.")),
        bloque_mapa(("Dónde ensayamos", "Where we rehearse"),
                    ("Tocamos en tu evento, pero si quieres conocernos y oír un ensayo, escríbenos y quedamos.",
                     "We play at your event, but if you want to meet us and hear a rehearsal, get in touch and we will arrange it."),
                    tono="relleno"),
    ])

    return plantilla(KEY, ("Músico, cantante o banda", "Musician, singer or band"),
                     ("Solista, dúo o banda para bodas, fiestas y locales con formaciones, repertorio por estilos, demos descritas, requisitos técnicos, packs de ejemplo y consulta de fecha.",
                      "Soloist, duo or band for weddings, parties and venues with line-ups, repertoire by style, described demos, technical requirements, example packages and date enquiries."),
                     "creativos", ("musico cantante banda grupo musical duo solista musica en directo orquesta bodas fiestas bares verbena repertorio versiones actuacion concierto",
                                   "musician singer band live band wedding band duo soloist live music cover band function band repertoire gig performance"),
                     look, cabecera(nav, marco([llamar(), whatsapp("ghost")], dir="row", gap=12, wrap=True)),
                     pie(nav, ("Música en directo para bodas, fiestas y locales. Solista, dúo o banda.",
                               "Live music for weddings, parties and venues. Soloist, duo or band."),
                         ("Lunes a viernes 11:00 a 14:00 y 17:00 a 20:00. Fines de semana, actuaciones y WhatsApp.",
                          "Monday to Friday 11 am to 2 pm and 5 pm to 8 pm. Weekends, gigs and WhatsApp.")),
                     [inicio, repertorio, form, cont])
