# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
from lib import *
from looks import LOOKS  # noqa: F401  (la paleta es propia de este sector)

KEY = "bodas-eventos"

# Paleta propia: marfil con malva apagado, a medio camino entre el rosa y el ciruela de otras plantillas.
# Sin `tono="oscuro"`: primary/inverse no llega a 3.
MALVA = dict(
    colores=dict(background="#fdfaf9", surface="#f6eef0", text="#2b2229", muted="#6a5a62", primary="#6e4a66",
                 on_primary="#ffffff", border="#e9dde0", inverse="#35202c", on_inverse="#fbf1f4"),
    fuentes=("playfair-display", "work-sans"), peso=500, h1=(54, 42, 32), radio=(2, 6, 12))


def build():
    nueva(KEY)
    look = MALVA
    nav = [("inicio", ("Inicio", "Home")), ("servicios", ("Servicios", "Services")),
           ("preparativos", ("Preparativos", "Planning")), ("contacto", ("Cuéntanos tu evento", "Tell us about your event"))]
    horas = [(("Lunes a viernes", "Monday to Friday"),
              ("10:00 a 14:00 y 16:00 a 19:00", "10:00 am to 2:00 pm and 4:00 pm to 7:00 pm")),
             (("Sábado", "Saturday"), ("Reuniones con cita", "Meetings by appointment")),
             (("Domingo", "Sunday"), ("Cerrado", "Closed"))]

    def contar(variante="primary"):
        return boton(("Cuéntanos tu evento", "Tell us about your event"), a_pagina("contacto", "contacto"), variante)

    inicio = pagina("inicio", ("", ""), ("Organización de bodas y eventos", "Wedding and event planning"),
                    ("Organización de bodas y eventos: servicios, cómo es el proceso desde la primera reunión, galería y formulario para contarnos tu evento.",
                     "Wedding and event planning: services, how the process works from the first meeting, gallery and a form to tell us about your event."), [
        hero(("Tu boda, como la imaginas y sin agobios", "Your wedding, the way you imagine it and without the stress"),
             ("Organizamos bodas y eventos de principio a fin, o coordinamos solo el día. Empezamos con una reunión sin compromiso para escucharte.",
              "We plan weddings and events from start to finish, or coordinate just the day. We begin with a no-obligation meeting to listen to you."),
             [contar(), whatsapp()], eyebrow=("Bodas y eventos", "Weddings and events"), variante="centro", tono="relleno"),
        bloque_valores(("Qué podemos hacer por ti", "What we can do for you"), [
            (("Organización completa", "Full planning"),
             ("Desde elegir el lugar hasta cerrar la última factura. Tú decides y nosotros lo movemos.",
              "From choosing the venue to settling the last invoice. You decide and we make it happen.")),
            (("Coordinación del día", "Day-of coordination"),
             ("Si ya lo tienes casi todo, nos ocupamos de que el día salga según lo previsto.",
              "If you already have most things sorted, we make sure the day runs as planned.")),
            (("Decoración", "Decoration"),
             ("Mesas, flores, iluminación y detalles, a juego con vuestro estilo.", "Tables, flowers, lighting and details to match your style.")),
            (("Ceremonias", "Ceremonies"),
             ("Ceremonias civiles o simbólicas, con lecturas, música y un guion hecho a vuestra medida.",
              "Civil or symbolic ceremonies, with readings, music and a script made to measure.")),
        ], cols=4, estilo="borde", tono="base", anchor="servicios"),
        bloque_pasos(("Cómo trabajamos", "How we work"), [
            (("Primera reunión", "First meeting"),
             ("Nos contáis la idea, la fecha, cuánta gente y el presupuesto. No hay compromiso.",
              "You tell us the idea, the date, how many guests and the budget. No obligation.")),
            (("Propuesta", "Proposal"), ("Os enviamos una propuesta con lo que incluye y el presupuesto detallado.",
                                         "We send you a proposal with what is included and a detailed quote.")),
            (("Preparación", "Preparation"), ("Elegimos proveedores, vamos cerrando detalles y os mantenemos al día en cada paso.",
                                               "We choose suppliers, close the details one by one and keep you updated at every step.")),
            (("El día", "The day"), ("Estamos desde el montaje hasta el final. Vosotros disfrutáis.",
                                     "We are there from set-up to the end. You enjoy it.")),
        ], tono="relleno"),
        bloque_galeria(("Algunos momentos", "Some moments"), [
            ("Ceremonia al aire libre con sillas blancas y arco de flores", "Outdoor ceremony with white chairs and a flower arch"),
            ("Mesa de banquete con velas y flores en tonos claros", "Banquet table with candles and flowers in soft tones"),
            ("Detalle de un ramo de novia sobre una silla", "Close-up of a bridal bouquet on a chair"),
            ("Salón preparado para una celebración con guirnaldas de luz", "Hall set for a celebration with string lights"),
            ("Pareja junto a la tarta nupcial", "Couple beside the wedding cake"),
            ("Entrada de una finca decorada con flores", "Entrance to an estate decorated with flowers")],
            cols=3, ratio=1.0, tono="base"),
        bloque_opiniones(("Lo que cuentan quienes confiaron en nosotros", "What people who trusted us say"),
                         [("la organización", "the planning"), ("el día de la boda", "the wedding day"), ("el trato", "the service")]),
        bloque_cta(("¿Os contamos cómo lo haríamos?", "Shall we tell you how we would do it?"),
                   ("Rellenad el formulario con fecha, invitados y presupuesto orientativo y os respondemos para quedar.",
                    "Fill in the form with the date, guests and an approximate budget and we will get back to arrange a meeting."),
                   [contar(), llamar("ghost")], tono="base"),
    ])

    serv = pagina("servicios", ("servicios", "services"), ("Servicios", "Services"),
                  ("Organización completa, coordinación del día, decoración y ceremonias de bodas y otros eventos, con presupuesto sin compromiso.",
                   "Full planning, day-of coordination, decoration and ceremonies for weddings and other events, with a no-obligation quote."), [
        hero(("Servicios", "Services"),
             ("Cada evento lleva su presupuesto, porque ninguno es igual. Os lo damos por escrito y sin compromiso.",
              "Every event gets its own quote, because no two are alike. We give it to you in writing and with no obligation."),
             [contar()], variante="texto"),
        bloque_checks(("Organización completa", "Full planning"), [
            ("Búsqueda de lugar, visitas y reserva de fecha.", "Finding the venue, visits and booking the date."),
            ("Selección de proveedores: catering, música, flores, fotografía y vídeo.", "Choosing suppliers: catering, music, flowers, photography and video."),
            ("Control del presupuesto y de los pagos a proveedores.", "Budget control and payments to suppliers."),
            ("Calendario de preparación y reuniones de seguimiento.", "A planning calendar and follow-up meetings."),
            ("Coordinación del día completo.", "Coordination of the whole day."),
        ], eyebrow=("Para quien no quiere ocuparse de nada", "For those who do not want to deal with a thing"),
            tono="base", cols=2, anchor="completa"),
        bloque_checks(("Coordinación del día", "Day-of coordination"), [
            ("Una reunión previa para conocer lo que ya tenéis cerrado.", "A prior meeting to learn what you already have sorted."),
            ("Contacto con vuestros proveedores y horarios del día.", "Contacting your suppliers and setting the day's timings."),
            ("Presencia desde el montaje hasta el final de la celebración.", "Presence from set-up to the end of the celebration."),
            ("Una persona de referencia para cualquier imprevisto.", "One point of contact for anything unexpected."),
        ], eyebrow=("Para quien ya lo tiene casi todo", "For those who have nearly everything"),
            tono="relleno", cols=2, anchor="coordinacion"),
        bloque_valores(("Decoración y ceremonias", "Decoration and ceremonies"), [
            (("Decoración de espacios", "Venue decoration"),
             ("Un diseño a vuestro gusto con flores, mobiliario, iluminación y papelería.",
              "A design to your taste with flowers, furniture, lighting and stationery.")),
            (("Ceremonia civil o simbólica", "Civil or symbolic ceremony"),
             ("Escribimos con vosotros el guion, elegimos lecturas y música, y la dirigimos el día de la boda.",
              "We write the script with you, choose readings and music, and run it on the wedding day.")),
            (("Otros eventos", "Other events"),
             ("Bautizos, comuniones, aniversarios, cumpleaños y celebraciones de empresa.",
              "Christenings, communions, anniversaries, birthdays and company celebrations.")),
        ], intro=("Los trámites de la ceremonia civil o religiosa los hacéis vosotros en el ayuntamiento o en la parroquia. Os recordamos los plazos, que cambian según el sitio.",
                  "The paperwork for a civil or religious ceremony is handled by you at the town hall or parish. We remind you of the deadlines, which vary by place."),
            tono="base", estilo="relleno", anchor="decoracion"),
        bloque_texto(("Presupuesto sin compromiso", "No-obligation quote"), [
            ("No publicamos precios porque dependen de la fecha, el número de invitados y lo que queráis incluir. Os enviamos un presupuesto detallado después de la primera reunión.",
             "We do not publish prices because they depend on the date, the number of guests and what you want to include. We send a detailed quote after the first meeting."),
            ("Lo que se paga como señal y lo que se paga después queda por escrito en la propuesta, antes de que digáis que sí.",
             "What is paid as a deposit and what is paid later is written in the proposal, before you say yes."),
        ], tono="relleno", imagen_=("photo", ("Mesa de reunión con muestras de telas, flores y un cuaderno",
                                              "Meeting table with fabric samples, flowers and a notebook"))),
        bloque_cta(("¿Hablamos de vuestro evento?", "Shall we talk about your event?"), None, [contar(), whatsapp("ghost")], tono="base"),
    ])

    prep = pagina("preparativos", ("preparativos", "planning"), ("Preparativos y preguntas", "Planning and questions"),
                  ("Calendario de preparación de una boda, desde 12 meses antes hasta el día, y respuestas a las dudas más habituales.",
                   "A wedding planning calendar, from 12 months before to the day itself, and answers to the most common questions."), [
        hero(("Preparativos y preguntas", "Planning and questions"),
             ("Un calendario general para que sepáis qué toca y cuándo. Lo ajustamos a vuestra fecha y a vuestro tipo de evento.",
              "A general calendar so you know what comes when. We adapt it to your date and to your kind of event."),
             [contar()], variante="texto"),
        bloque_precios(("Calendario de preparación", "Planning calendar"), ("Qué hacer y cuándo", "What to do and when"),
                       [("Cuándo", "When"), ("Qué toca", "What to do")],
                       [(("12 meses antes", "12 months before"),
                         ("Fijar fecha y presupuesto, hacer la primera lista de invitados y visitar lugares.",
                          "Set the date and budget, make a first guest list and visit venues.")),
                        (("9 meses antes", "9 months before"),
                         ("Reservar el lugar y los proveedores más solicitados: fotografía, vídeo y música.",
                          "Book the venue and the most in-demand suppliers: photography, video and music.")),
                        (("6 meses antes", "6 months before"),
                         ("Elegir catering y menú, vestuario y mandar el aviso de fecha a los invitados.",
                          "Choose the catering and menu, outfits, and send a save-the-date to guests.")),
                        (("4 meses antes", "4 months before"),
                         ("Decoración, flores, transporte y alojamiento para quien viene de fuera.",
                          "Decoration, flowers, transport and accommodation for guests coming from elsewhere.")),
                        (("2 meses antes", "2 months before"),
                         ("Enviar las invitaciones, probar el menú y repasar los trámites de la ceremonia.",
                          "Send the invitations, taste the menu and check the ceremony paperwork.")),
                        (("1 mes antes", "1 month before"),
                         ("Confirmar asistentes, preparar el plan de mesas y cerrar el guion del día.",
                          "Confirm attendance, prepare the seating plan and finalise the day's schedule.")),
                        (("1 semana antes", "1 week before"),
                         ("Confirmar horarios con todos los proveedores y hacer los pagos pendientes.",
                          "Confirm timings with all suppliers and make any outstanding payments.")),
                        (("El día de la boda", "The wedding day"),
                         ("Montaje, ceremonia, celebración y recogida. Vosotros solo disfrutáis.",
                          "Set-up, ceremony, celebration and clear-up. You just enjoy it."))],
                       intro=("Calendario de ejemplo: cada boda es distinta. Cámbialo por el tuyo si trabajas con otros plazos.",
                              "Example calendar: every wedding is different. Change it to yours if you work to other timings."),
                       anchor="calendario", tono="base"),
        bloque_faq(("Preguntas frecuentes", "Frequently asked questions"), [
            (("¿Con cuánta antelación debemos contactar?", "How far ahead should we get in touch?"),
             ("Cuanto antes, mejor: los sitios y proveedores más pedidos se llenan con un año de margen. Pero también organizamos eventos con poco tiempo, si la fecha está libre.",
              "The sooner the better: the most popular venues and suppliers book up a year ahead. We also plan short-notice events, if the date is free.")),
            (("¿Cuánto cuesta organizar una boda?", "How much does it cost to plan a wedding?"),
             ("Depende del número de invitados, el lugar y lo que se incluya. Después de la primera reunión os damos un presupuesto detallado sin compromiso.",
              "It depends on the number of guests, the venue and what is included. After the first meeting we give you a detailed no-obligation quote.")),
            (("¿Podemos elegir nuestros proveedores?", "Can we choose our own suppliers?"),
             ("Sí. Si ya tenéis fotógrafo, catering o música, trabajamos con ellos. Si no, os proponemos opciones.",
              "Yes. If you already have a photographer, caterer or band, we work with them. If not, we suggest options.")),
            (("¿Qué pasa si hay un imprevisto el día de la boda?", "What if something unexpected happens on the day?"),
             ("Para eso estamos. Tenemos un plan B para lo que se puede prever, como la lluvia, y resolvemos lo demás sobre la marcha.",
              "That is what we are for. We have a plan B for what can be foreseen, such as rain, and we handle the rest on the spot.")),
            (("¿Organizáis eventos pequeños?", "Do you plan small events?"),
             ("Sí: bodas íntimas, bautizos, cumpleaños y reuniones de empresa. El tamaño no cambia cómo trabajamos.",
              "Yes: intimate weddings, christenings, birthdays and company get-togethers. Size does not change how we work.")),
        ], tono="relleno"),
        bloque_cta(("¿Os ayudamos con el calendario?", "Shall we help with the calendar?"), None, [contar(), whatsapp("ghost")], tono="base"),
    ])

    cont = pagina("contacto", ("contacto", "contact"), ("Cuéntanos tu evento", "Tell us about your event"),
                  ("Cuéntanos tu boda o evento con fecha, invitados y presupuesto orientativo, o llama o escribe por WhatsApp.",
                   "Tell us about your wedding or event with the date, guests and an approximate budget, or call or message us on WhatsApp."), [
        hero(("Cuéntanos tu evento", "Tell us about your event"),
             ("Con estos datos nos hacemos una idea y os proponemos una reunión sin compromiso.",
              "With these details we get an idea and propose a no-obligation meeting."),
             [llamar(), whatsapp()], variante="texto"),
        bloque_contacto(("Dónde y cuándo vernos", "Where and when to meet"),
                        ("Nos reunimos en nuestra oficina o por videollamada.", "We meet at our office or by video call."),
                        [campo("nombre", "text", ("Nombre", "Name"), True, auto="name", maxlen=120),
                         campo("correo", "email", ("Correo electrónico", "Email"), True, auto="email", maxlen=200),
                         campo("telefono", "tel", ("Teléfono", "Phone"), False, auto="tel", maxlen=30),
                         campo("evento", "select", ("Tipo de evento", "Type of event"), True,
                               opciones=[("Boda", "Wedding"), ("Bautizo o comunión", "Christening or communion"),
                                         ("Cumpleaños o aniversario", "Birthday or anniversary"),
                                         ("Evento de empresa", "Company event"), ("Otro", "Other")]),
                         campo("fecha", "date", ("Fecha prevista", "Planned date")),
                         campo("invitados", "select", ("Número de invitados", "Number of guests"), False,
                               opciones=[("Hasta 30", "Up to 30"), ("De 31 a 70", "31 to 70"),
                                         ("De 71 a 120", "71 to 120"), ("Más de 120", "More than 120")]),
                         campo("presupuesto", "select", ("Presupuesto orientativo", "Approximate budget"), False,
                               opciones=[("Menos de 5.000 €", "Under €5,000"), ("De 5.000 a 10.000 €", "€5,000 to €10,000"),
                                         ("De 10.000 a 20.000 €", "€10,000 to €20,000"), ("Más de 20.000 €", "Over €20,000"),
                                         ("Aún no lo sé", "I do not know yet")]),
                         campo("mensaje", "textarea", ("Contadnos vuestra idea", "Tell us your idea"), False, maxlen=2000)],
                        ("Enviar", "Send"), horas,
                        exito=("Gracias. Os escribimos para proponeros una primera reunión sin compromiso.",
                               "Thank you. We will write to propose a first no-obligation meeting.")),
        bloque_mapa(tono="relleno"),
    ])

    return plantilla(KEY, ("Bodas y eventos", "Weddings and events"),
                     ("Servicios de organización de bodas y eventos, proceso paso a paso, calendario de preparación, galería, preguntas frecuentes y formulario con fecha, invitados y presupuesto.",
                      "Wedding and event planning services, step-by-step process, planning calendar, gallery, FAQ and a form with date, guests and budget."),
                     "creativos", ("bodas eventos organizacion bodas wedding planner organizadora coordinacion decoracion ceremonia boda civil bautizo comunion celebracion",
                                   "wedding planner event planner weddings events planning coordination decoration ceremony celebration christening party"),
                     look, cabecera(nav, contar()),
                     pie(nav, ("Organización de bodas y eventos: organización completa, coordinación del día, decoración y ceremonias.",
                               "Wedding and event planning: full planning, day-of coordination, decoration and ceremonies."),
                         ("Lunes a viernes 10:00 a 14:00 y 16:00 a 19:00. Sábado con cita.",
                          "Monday to Friday 10 am to 2 pm and 4 pm to 7 pm. Saturday by appointment.")),
                     [inicio, serv, prep, cont])
