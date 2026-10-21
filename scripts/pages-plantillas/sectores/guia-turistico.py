# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
from lib import *
from looks import LOOKS

KEY = "guia-turistico"

# Como COLEGIADO: el 00-0000 lo detecta domain.TextosDemo y el sitio no se publica hasta cambiarlo.
CARNE = ("Guía oficial de turismo. Carné de guía: 00-0000", "Official tourist guide. Guide licence no.: 00-0000")


def build():
    nueva(KEY)
    look = dict(LOOKS["terracota"], fuentes=("noto-serif", "noto-sans"))
    nav = [("inicio", ("Inicio", "Home")), ("visitas", ("Visitas", "Tours")),
           ("grupos", ("Grupos y tarifas", "Groups and rates")), ("reservar", ("Reservar", "Book"))]
    horas = [(("Lunes a viernes", "Monday to Friday"), ("9:00 a 19:00", "9:00 am to 7:00 pm")),
             (("Sábado y domingo", "Saturday and Sunday"), ("Visitas con reserva previa", "Tours by prior booking")),
             (("Mensajes", "Messages"), ("Respondo en menos de 24 horas", "I reply within 24 hours"))]
    cta_cab = marco([llamar(t=("Llamar", "Call")), whatsapp("secondary", t=("WhatsApp", "WhatsApp"))],
                    dir="row", gap=8, wrap=True)
    reserva = lambda v="primary": boton(("Reservar una visita", "Book a tour"), a_pagina("reservar", "reserva"), v)
    nota = ("Precios de ejemplo: cámbialos por los tuyos. Son por grupo, no por persona.",
            "Example prices: replace them with your own. They are per group, not per person.")

    def visita(nombre, texto_, datos):
        return tarjeta([h3(nombre), p(texto_, color="$muted"), peq(datos, color="$muted")], estilo="relleno", gap=10)

    inicio = pagina("inicio", ("", ""), ("Guía turístico oficial", "Official tourist guide"),
                    ("Guía turístico oficial independiente: visitas guiadas por Tu ciudad, idiomas, grupos privados y escolares, tarifa de ejemplo por grupo y reserva con fecha.",
                     "Independent official tourist guide: guided tours of Your city, languages, private and school groups, an example rate per group and booking by date."), [
        hero(("Conoce Tu ciudad con quien la cuenta de verdad", "Get to know Your city with someone who really tells its story"),
             ("Soy guía oficial y hago visitas a pie, en pequeño grupo y sin prisas. Te enseño lo que se ve y lo que casi nadie cuenta.",
              "I am an official guide and run walking tours for small groups, unhurried. I show you what you can see and what hardly anyone tells."),
             [reserva(), whatsapp()], eyebrow=("Guía turístico", "Tourist guide"),
             extra=peq(CARNE, color="$muted"),
             imagen_=("portrait", ("La guía con un grupo frente a una plaza histórica", "The guide with a group in front of a historic square"))),
        bloque_texto(("Cómo hago mis visitas", "How I run my tours"), [
            ("Prefiero grupos pequeños para que puedas preguntar y oírme sin esfuerzo. Adapto el paso y las paradas a quien viene: niños, personas mayores o gente con prisa.",
             "I prefer small groups so you can ask questions and hear me without effort. I adapt the pace and the stops to who comes: children, older people or people in a hurry."),
            ("Empiezo siempre en un punto de encuentro claro y a la hora acordada, y te digo cuánto va a durar antes de echar a andar.",
             "I always start at a clear meeting point at the agreed time, and tell you how long it will take before we set off.")],
            tono="relleno", imagen_=("landscape", ("Callejón empedrado del casco antiguo", "Cobbled lane in the old town")),
            eyebrow=("Mi forma de trabajar", "How I work")),
        bloque_valores(("Visitas que hago", "Tours I run"), [
            (("Casco histórico de Tu ciudad", "Old town of Your city"),
             ("Plazas, murallas e iglesias, con la historia de quien las construyó y de quien vivió allí.",
              "Squares, walls and churches, with the story of those who built them and those who lived there.")),
            (("Ruta gastronómica", "Food tour"),
             ("Mercado, tabernas y productos de la zona. Probamos y te cuento de dónde viene cada cosa.",
              "Market, taverns and local produce. We taste and I tell you where each thing comes from.")),
            (("Naturaleza en el entorno", "Nature around town"),
             ("Una ruta a pie por los alrededores, con paradas para mirar el paisaje, los pájaros y los pueblos.",
              "A walking route through the surroundings, with stops for the landscape, the birds and the villages.")),
        ], intro=("Son ejemplos. En la página de visitas tienes duración, punto de encuentro e idiomas.",
                  "These are examples. The tours page has the length, meeting point and languages."),
            estilo="borde", anchor="visitas-destacadas"),
        bloque_checks(("Idiomas en los que hago las visitas", "Languages I give tours in"), [
            ("Español", "Spanish"),
            ("Inglés", "English"),
            ("Otro idioma: cámbialo por el tuyo", "Another language: replace it with yours"),
        ], intro=("Dime el idioma al reservar y, si no lo hago yo, te digo con quién podemos organizarlo.",
                  "Tell me the language when you book and, if I do not speak it, I will tell you who we can arrange it with."),
            eyebrow=("Idiomas", "Languages"), cols=3, tono="relleno"),
        bloque_pasos(("Tu visita, paso a paso", "Your tour, step by step"), [
            (("Eliges", "You choose"), ("La visita, el día y el idioma.", "The tour, the day and the language.")),
            (("Te confirmo", "I confirm"), ("Te escribo con hora, punto de encuentro y precio.", "I write to you with the time, meeting point and price.")),
            (("Nos vemos", "We meet"), ("En el punto de encuentro. Llevo un cartel o un paraguas para que me reconozcas.",
                                       "At the meeting point. I carry a sign or an umbrella so you can spot me.")),
            (("Caminamos", "We walk"), ("Con paradas para sentarte, hacer fotos y preguntar lo que quieras.",
                                       "With stops to sit, take photos and ask whatever you like.")),
        ], tono="base"),
        bloque_opiniones(("Lo que dicen quienes han hecho la visita", "What people who joined a tour say"),
                         [("la visita", "the tour"), ("las explicaciones", "the explanations"), ("el ritmo", "the pace")],
                         tono="relleno"),
        bloque_cta(("¿Qué día te viene bien?", "Which day suits you?"),
                   ("Cuéntame cuántos sois y qué os apetece ver.", "Tell me how many of you there are and what you would like to see."),
                   [reserva(), llamar("secondary")], tono="base"),
    ])

    vis = pagina("visitas", ("visitas", "tours"), ("Visitas guiadas", "Guided tours"),
                 ("Visitas guiadas por Tu ciudad: casco histórico, gastronómica, naturaleza y a medida, con duración, punto de encuentro e idiomas.",
                  "Guided tours of Your city: old town, food, nature and tailor-made, with length, meeting point and languages."), [
        hero(("Visitas guiadas", "Guided tours"),
             ("Elige una visita y reserva tu fecha. Si lo que buscas no aparece, la preparo a tu medida.",
              "Choose a tour and book your date. If what you are after is not here, I will prepare it for you."),
             [reserva(), whatsapp("secondary")], variante="texto", extra=peq(CARNE, color="$muted")),
        seccion("features", [
            titular(("Las visitas", "The tours"),
                    ("Duraciones y puntos de encuentro de ejemplo: cámbialos por los tuyos.",
                     "Example lengths and meeting points: replace them with your own.")),
            rejilla([
                visita(("Casco histórico de Tu ciudad", "Old town of Your city"),
                       ("El paseo clásico: plazas, murallas, iglesias y rincones que se escapan en una visita por libre.",
                        "The classic walk: squares, walls, churches and corners you miss when you go alone."),
                       ("2 horas. Se hace a pie, sin cuestas fuertes.", "2 hours. On foot, with no steep climbs.")),
                visita(("Ruta gastronómica", "Food tour"),
                       ("Paradas en el mercado y en bares y tiendas de siempre para probar productos de la zona.",
                        "Stops at the market and in long-standing bars and shops to taste local produce."),
                       ("3 horas. Las degustaciones se pagan aparte.", "3 hours. Tastings are paid separately.")),
                visita(("Naturaleza en el entorno de Tu ciudad", "Nature around Your city"),
                       ("Una ruta a pie por los alrededores, con explicaciones sobre el paisaje, la fauna y los pueblos de la zona.",
                        "A walking route through the surroundings, with explanations of the landscape, wildlife and villages."),
                       ("3 a 4 horas. Calzado de montaña recomendado.", "3 to 4 hours. Walking boots recommended.")),
                visita(("Visita para familias", "Family tour"),
                       ("Un recorrido corto con juegos y preguntas para que los niños vayan delante y no se aburran.",
                        "A short route with games and questions so the children lead the way and do not get bored."),
                       ("90 minutos. Apta para carritos.", "90 minutes. Suitable for pushchairs.")),
                visita(("Visita nocturna", "Night tour"),
                       ("La ciudad con otra luz y las leyendas que se cuentan de noche.",
                        "The city in a different light and the legends told at night."),
                       ("90 minutos. Salimos al anochecer.", "90 minutes. We set off at dusk.")),
                visita(("Ruta a medida", "Tailor-made route"),
                       ("Me dices qué te interesa —arte, historia, arquitectura, fotografía— y preparo el recorrido.",
                        "Tell me what interests you —art, history, architecture, photography— and I plan the route."),
                       ("Duración a elegir.", "Length of your choice.")),
            ], cols=3)], anchor="catalogo"),
        seccion("features", [
            titular(("Duración, punto de encuentro e idiomas", "Length, meeting point and languages")),
            tabla(("Resumen de visitas", "Summary of tours"),
                  [("Visita", "Tour"), ("Duración", "Length"), ("Punto de encuentro", "Meeting point"), ("Idiomas", "Languages")],
                  [(("Casco histórico", "Old town"), ("2 h", "2 h"), ("Plaza principal de Tu ciudad", "Main square of Your city"), ("Español, inglés", "Spanish, English")),
                   (("Ruta gastronómica", "Food tour"), ("3 h", "3 h"), ("Entrada del mercado", "Market entrance"), ("Español, inglés", "Spanish, English")),
                   (("Naturaleza", "Nature"), ("3 a 4 h", "3 to 4 h"), ("Aparcamiento de la ruta", "Route car park"), ("Español", "Spanish")),
                   (("Familias", "Family tour"), ("90 min", "90 min"), ("Plaza principal de Tu ciudad", "Main square of Your city"), ("Español, inglés", "Spanish, English")),
                   (("Nocturna", "Night tour"), ("90 min", "90 min"), ("Plaza principal de Tu ciudad", "Main square of Your city"), ("Español", "Spanish"))])],
            tono="relleno", anchor="resumen", gap=28),
        bloque_checks(("Puntos de encuentro", "Meeting points"), [
            ("Plaza principal de Tu ciudad: junto a la fuente, bajo el reloj.", "Main square of Your city: next to the fountain, under the clock."),
            ("Entrada del mercado, por la puerta de la calle principal.", "Market entrance, by the door on the main street."),
            ("Aparcamiento de la ruta de naturaleza, a unos minutos en coche.", "Car park for the nature route, a few minutes by car."),
            ("Te mando un enlace al mapa con la confirmación.", "I send you a map link with the confirmation."),
        ], intro=("Pon aquí los puntos reales y qué ver para reconocerlos.",
                  "Put the real points here and what to look for to recognise them."),
            eyebrow=("Dónde nos vemos", "Where we meet"), cols=2, anchor="puntos"),
        bloque_faq(("Preguntas frecuentes", "Frequently asked questions"), [
            (("¿Qué pasa si llueve?", "What if it rains?"),
             ("Con lluvia ligera se hace igual, y con paraguas. Si hay aviso por mal tiempo, te propongo otra fecha sin coste.",
              "With light rain it goes ahead, with umbrellas. If there is a bad weather warning, I offer you another date at no cost.")),
            (("¿Las entradas están incluidas?", "Are tickets included?"),
             ("Normalmente no: las entradas a monumentos y museos se pagan aparte. Te digo cuáles hay y cuánto cuestan al confirmar.",
              "Usually not: tickets to monuments and museums are paid separately. I tell you which there are and how much they cost when I confirm.")),
            (("¿Se puede ir con niños o con movilidad reducida?", "Can I come with children or reduced mobility?"),
             ("Sí. Dímelo al reservar y preparo un recorrido sin escaleras ni cuestas, o más corto.",
              "Yes. Tell me when you book and I will plan a route without stairs or slopes, or a shorter one.")),
            (("¿Cuántas personas hacen falta?", "How many people are needed?"),
             ("Con una persona ya salgo. Si sois pocos, el precio por grupo sigue siendo el mismo.",
              "I will go out with just one person. If you are few, the group rate stays the same.")),
        ], tono="relleno"),
        bloque_cta(("¿Qué visita eliges?", "Which tour will you choose?"), None, [reserva(), llamar("secondary")], tono="base"),
    ])

    grupos = pagina("grupos", ("grupos-y-tarifas", "groups-and-rates"), ("Grupos y tarifas", "Groups and rates"),
                    ("Visitas privadas, grupos de empresa y escolares, con una tarifa de ejemplo por grupo y qué incluye.",
                     "Private tours, company and school groups, with an example rate per group and what it includes."), [
        hero(("Grupos y tarifas", "Groups and rates"),
             ("Cobro por grupo y no por persona: así sabes lo que vas a pagar antes de reunir a todos.",
              "I charge per group, not per person: so you know what you will pay before gathering everyone."),
             [reserva(), whatsapp("secondary")], variante="texto"),
        bloque_valores(("Para quién hago visitas privadas", "Who I run private tours for"), [
            (("Familias y amigos", "Families and friends"),
             ("Un paseo solo para vosotros, a la hora que elijáis y con el ritmo que os venga bien.",
              "A walk just for you, at the time you choose and at the pace that suits you.")),
            (("Escolares", "School groups"),
             ("Visitas pensadas para clase, con juegos y preguntas, y con un adulto por cada grupo de alumnos.",
              "Tours designed for a class, with games and questions, and an adult for each group of pupils.")),
            (("Empresas y asociaciones", "Companies and associations"),
             ("Una actividad de equipo o una visita para clientes y socios, con factura a nombre de la entidad.",
              "A team activity or a visit for clients and members, with an invoice in the organisation's name.")),
        ], eyebrow=("Grupos", "Groups"), estilo="borde", tono="base"),
        bloque_precios(("Tarifas de ejemplo por grupo", "Example rates per group"), ("Tarifas por grupo", "Rates per group"),
                       [("Visita", "Tour"), ("Tamaño del grupo", "Group size"), ("Tarifa por grupo", "Group rate")],
                       [(("Visita privada de 2 horas", "Private 2-hour tour"), ("Hasta 10 personas", "Up to 10 people"), eur(150)),
                        (("Visita privada de 3 horas", "Private 3-hour tour"), ("Hasta 10 personas", "Up to 10 people"), eur(210)),
                        (("Grupo grande de 2 horas", "Large group, 2 hours"), ("De 11 a 25 personas", "11 to 25 people"), eur(230)),
                        (("Grupo escolar de 2 horas", "School group, 2 hours"), ("Hasta 30 alumnos", "Up to 30 pupils"), eur(120)),
                        (("Suplemento por idioma distinto del español", "Supplement for a language other than Spanish"), ("Cualquier tamaño", "Any size"), eur(20))],
                       intro=("Los precios son por visita y por grupo. Si tu grupo es más grande, escríbeme y lo organizamos en varios turnos.",
                              "Prices are per tour and per group. If your group is larger, write to me and we will split it into several shifts."),
                       nota=nota, anchor="tarifas", tono="relleno"),
        bloque_checks(("Qué incluye la tarifa", "What the rate includes"), [
            ("La visita guiada, con el recorrido acordado.", "The guided tour, with the agreed route."),
            ("Tiempo para preguntas y fotos durante el recorrido.", "Time for questions and photos along the way."),
            ("Factura con IVA, si la necesitas.", "An invoice with VAT, if you need one."),
            ("Las entradas, las degustaciones y el transporte no están incluidos.", "Tickets, tastings and transport are not included."),
        ], eyebrow=("Incluido", "Included"), cols=2, tono="base"),
        bloque_texto(("Cómo se paga", "How to pay"), [
            ("Confirmo la visita cuando me dices que sí y cobro al terminar, por transferencia o en efectivo. Para grupos grandes y escolares, te pido una pequeña señal al reservar.",
             "I confirm the tour when you say yes and charge at the end, by bank transfer or in cash. For large groups and school groups, I ask for a small deposit when you book.")],
            eyebrow=("Pago", "Payment"), tono="relleno"),
        bloque_cta(("¿Os preparo una visita?", "Shall I prepare a tour for you?"), None, [reserva(), llamar("secondary")], tono="base"),
    ])

    cont = pagina("reservar", ("reservar", "book"), ("Reservar una visita", "Book a tour"),
                  ("Reserva con fecha, número de personas e idioma, y consulta el horario, el punto de encuentro principal y el mapa.",
                   "Book with a date, number of people and language, and see opening hours, the main meeting point and the map."), [
        hero(("Reserva tu visita", "Book your tour"),
             ("Dime qué visita, qué día, cuántos sois y en qué idioma. Te contesto con la hora, el punto de encuentro y el precio.",
              "Tell me which tour, which day, how many of you there are and in what language. I reply with the time, meeting point and price."),
             [llamar(), whatsapp()], variante="texto", extra=peq(CARNE, color="$muted")),
        bloque_contacto(("Pide tu fecha", "Ask for your date"),
                        ("Es una petición, no un pago. Te confirmo la disponibilidad antes de nada.",
                         "This is a request, not a payment. I confirm availability before anything else."),
                        [campo("nombre", "text", ("Nombre", "Name"), True, auto="name", maxlen=120),
                         campo("correo", "email", ("Correo electrónico", "Email"), True, auto="email", maxlen=200),
                         campo("telefono", "tel", ("Teléfono (opcional)", "Phone (optional)"), False, auto="tel", maxlen=30),
                         campo("visita", "select", ("Visita", "Tour"), True,
                               opciones=[("Casco histórico", "Old town"), ("Ruta gastronómica", "Food tour"),
                                         ("Naturaleza", "Nature"), ("Visita para familias", "Family tour"),
                                         ("Visita nocturna", "Night tour"), ("Ruta a medida", "Tailor-made route")]),
                         campo("fecha", "date", ("Fecha", "Date"), True),
                         campo("personas", "number", ("Número de personas", "Number of people"), True),
                         campo("idioma", "select", ("Idioma", "Language"), True,
                               opciones=[("Español", "Spanish"), ("Inglés", "English"), ("Otro idioma", "Another language")]),
                         campo("mensaje", "textarea", ("Algo que deba saber (niños, movilidad, intereses)",
                                                       "Anything I should know (children, mobility, interests)"), False, maxlen=1000)],
                        ("Pedir esta fecha", "Request this date"), horas, anchor="reserva",
                        exito=("Gracias. Te respondo en menos de 24 horas para confirmarte la fecha.",
                               "Thank you. I will reply within 24 hours to confirm your date.")),
        bloque_mapa(("Punto de encuentro principal", "Main meeting point"),
                    ("Quedamos junto a la fuente de la plaza principal de Tu ciudad. En los demás puntos te mando la ubicación al confirmar.",
                     "We meet by the fountain in the main square of Your city. For other points I send the location when I confirm."),
                    tono="relleno"),
    ])

    return plantilla(KEY, ("Guía turístico", "Tourist guide"),
                     ("Guía turístico oficial independiente con visitas guiadas, idiomas, puntos de encuentro, grupos privados y escolares, tarifa de ejemplo por grupo y reserva por fecha, personas e idioma.",
                      "Independent official tourist guide with guided tours, languages, meeting points, private and school groups, an example rate per group and booking by date, people and language."),
                     "turismo", ("guia turistico guia oficial visitas guiadas tour free tour paseo casco historico ruta gastronomica ruta naturaleza grupos escolares visita privada guia local cicerone",
                                 "tourist guide official guide guided tours walking tour old town food tour nature hike private tour school groups local guide city tour"),
                     look, cabecera(nav, cta_cab),
                     pie(nav, ("Guía oficial de turismo. Carné de guía: 00-0000. Visitas guiadas en español e inglés.",
                               "Official tourist guide. Guide licence no.: 00-0000. Guided tours in Spanish and English."),
                         ("Lunes a viernes 9:00 a 19:00. Fines de semana con reserva.",
                          "Monday to Friday 9 am to 7 pm. Weekends by booking.")),
                     [inicio, vis, grupos, cont])
