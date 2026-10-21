# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
from lib import *
from looks import LOOKS  # noqa: F401  (la paleta es propia de este sector)

KEY = "residencia-mayores"

# Caoba suave sobre un fondo melocotón pálido. Sin fondo oscuro (nada usa $inverse).
CAOBA = dict(
    colores=dict(background="#fbf1ec", surface="#f3e1d9", text="#2b1a17", muted="#6a514c", primary="#5f2e2a",
                 on_primary="#ffffff", border="#e6d1c8", inverse="#3a1f1b", on_inverse="#fbefe9"),
    fuentes=("lora", "noto-sans"), peso=600, cuerpo=(18, 17, 17), radio=(6, 10, 16), ancho=1120)

# Como COLEGIADO: el 00-0000 lo detecta domain.TextosDemo y el sitio no se publica hasta cambiarlo.
REGISTRO = ("Centro autorizado. Nº de registro: 00-0000", "Authorised centre. Registration no.: 00-0000")
DATOS = ("Los datos de cada residente y de su familia, y su historial de salud, se tratan con confidencialidad y según la normativa de protección de datos. Las fotos solo se publican con la autorización de la persona o de su representante.",
         "Each resident's and family's details and health history are handled in confidence and under data protection rules. Photos are published only with the permission of the person or their representative.")
NOTA = ("Precios de ejemplo: cámbialos por los tuyos. Dependen de la habitación, del grado de ayuda que necesite la persona y de los servicios extra.",
        "Example prices: replace them with your own. They depend on the room, how much help the person needs and any extra services.")


def build():
    nueva(KEY)
    look = CAOBA
    nav = [("inicio", ("Inicio", "Home")), ("vida", ("Vida en la residencia", "Life at the home")),
           ("ingreso", ("Plazas e ingreso", "Places and admission")), ("contacto", ("Visita", "Visit"))]
    horas = [(("Visitas de familiares", "Family visits"), ("Todos los días, de 11:00 a 13:00 y de 16:00 a 20:00", "Every day, 11:00 am to 1:00 pm and 4:00 pm to 8:00 pm")),
             (("Atención a familias y visitas al centro", "Family enquiries and tours"), ("Lunes a viernes, de 10:00 a 14:00; por la tarde con cita", "Monday to Friday, 10:00 am to 2:00 pm; afternoons by appointment")),
             (("Centro de día", "Day centre"), ("Lunes a viernes, de 8:30 a 18:00", "Monday to Friday, 8:30 am to 6:00 pm")),
             (("Teléfono para urgencias de residentes", "Phone for residents' emergencies"), ("Las 24 horas", "24 hours"))]
    cta_cab = marco([llamar(t=("Llamar", "Call")), whatsapp("secondary", t=("WhatsApp", "WhatsApp"))],
                    dir="row", gap=8, wrap=True)
    visita = lambda v="primary": boton(("Pedir una visita", "Ask for a visit"), a_pagina("contacto", "visita"), v)

    inicio = pagina("inicio", ("", ""), ("Residencia y centro de día para mayores", "Care home and day centre for older people"),
                    ("Residencia y centro de día para personas mayores: cuidados, comidas, actividades, atención sanitaria, visitas de familiares y visita al centro.",
                     "Care home and day centre for older people: personal care, meals, activities, health care, family visits and a tour of the centre."), [
        hero(("Cuidar con calma a quien tú quieres, cerca de casa", "Caring calmly for someone you love, close to home"),
             ("Residencia y centro de día para personas mayores. Atención a cualquier hora, comida casera y actividades cada día. Ven a conocerla cuando quieras, sin compromiso.",
              "A care home and day centre for older people. Care at any hour, home cooking and activities every day. Come and see it whenever you like, with no obligation."),
             [visita(), llamar("secondary"), whatsapp("ghost")],
             eyebrow=("Residencia y centro de día", "Care home and day centre"),
             extra=peq(REGISTRO, color="$muted"),
             imagen_=("photo", ("Sala de estar luminosa con sillones, mesas bajas y una ventana al jardín",
                                "Bright lounge with armchairs, low tables and a window onto the garden"))),
        bloque_valores(("Qué hacemos cada día", "What we do each day"), [
            (("Atención y cuidados", "Care and support"),
             ("Ayudamos a levantarse, asearse, vestirse y moverse con seguridad, y solo en lo que cada persona necesita. Lo que puede hacer sola, lo sigue haciendo.",
              "We help with getting up, washing, dressing and moving safely, and only with what each person needs. What they can do alone, they keep doing.")),
            (("Comidas", "Meals"),
             ("Menú casero y de temporada, con dietas para diabetes, sin sal o textura blanda. Los alérgenos de cada plato están a la vista.",
              "Home-cooked seasonal menu, with diets for diabetes, low salt or soft textures. The allergens in each dish are on display.")),
            (("Actividades", "Activities"),
             ("Gimnasia suave, juegos de memoria, música, manualidades, paseos y tertulias. Cada uno participa en lo que le apetece.",
              "Gentle exercise, memory games, music, crafts, walks and conversation groups. Everyone joins in what they feel like.")),
            (("Atención sanitaria", "Health care"),
             ("Control de la medicación según la pauta de su médico, curas, toma de constantes y acompañamiento a las citas médicas.",
              "Medication management following the person's doctor's instructions, dressings, vital-signs checks and company at medical appointments.")),
        ], eyebrow=("Servicios", "Services"), cols=2, estilo="relleno", anchor="servicios"),
        bloque_texto(("Una casa pensada para estar a gusto", "A home designed to be comfortable"), [
            ("Habitaciones con luz natural, salas de estar para charlar, comedor y un jardín para pasear sentados o de pie. Se puede traer una butaca, fotos y objetos de casa para sentir la habitación como propia.",
             "Rooms with natural light, lounges for chatting, a dining room and a garden to stroll or sit in. You can bring an armchair, photos and things from home so the room feels like theirs."),
            ("Hay barandillas en los pasillos, suelos antideslizantes y timbre al alcance de la cama. Si quieres verlo con tus ojos, pide una visita.",
             "There are handrails in the corridors, non-slip floors and a call button within reach of the bed. If you want to see it for yourself, ask for a visit."),
        ], imagen_=("landscape", ("Jardín de la residencia con bancos a la sombra y caminos llanos", "The home's garden with benches in the shade and level paths")),
            tono="relleno", invertir=True),
        bloque_checks(("Las familias, siempre informadas", "Families kept informed"), [
            ("Una persona de referencia a quien llamar para cualquier duda.", "A named contact you can call with any question."),
            ("Te avisamos enseguida si hay un cambio de salud, una caída o una visita al médico.", "We tell you straight away about any change in health, a fall or a doctor's visit."),
            ("Reuniones periódicas para repasar cómo está, cómo come, cómo duerme y qué necesita.", "Regular meetings to review how they are, how they eat and sleep and what they need."),
            ("Puedes visitar en el horario de visitas sin avisar, y comer con tu familiar si lo pides antes.", "You can visit during visiting hours without notice, and have lunch with your relative if you ask beforehand."),
        ], eyebrow=("Familias", "Families"), cols=2, anchor="familias"),
        bloque_opiniones(("Lo que dicen las familias", "What families say"),
                         [("el trato del personal", "how the staff treat people"), ("la información que recibimos", "the information we receive"),
                          ("las actividades", "the activities")]),
        bloque_cta(("Ven a conocer la residencia", "Come and see the home"),
                   ("Te enseñamos las habitaciones, el comedor y el jardín, y resolvemos tus dudas sin compromiso.",
                    "We show you the rooms, the dining room and the garden, and answer your questions with no obligation."),
                   [visita(), llamar("secondary")], tono="base"),
    ])

    vida = pagina("vida", ("vida-en-la-residencia", "life-at-the-home"), ("Vida en la residencia", "Life at the home"),
                  ("Cómo es un día en la residencia: horario, comidas y menús con alérgenos, actividades y atención sanitaria.",
                   "What a day at the care home is like: timetable, meals and menus with allergens, activities and health care."), [
        hero(("Un día en la residencia", "A day at the care home"),
             ("Los horarios son una guía: cada persona se levanta, come y descansa a su ritmo.",
              "The times are a guide: each person gets up, eats and rests at their own pace."),
             [visita(), llamar("secondary")], variante="texto"),
        seccion("text", [titular(("Un día normal", "A normal day"),
                                 ("Así se organiza el día. Los fines de semana hay menos actividades y más visitas.",
                                  "This is how the day is organised. At weekends there are fewer activities and more visits.")),
                         tabla(("Horario de un día en la residencia", "Timetable of a day at the care home"),
                               [("Hora", "Time"), ("Qué se hace", "What happens")],
                               [(("8:00 a 9:30", "8:00 to 9:30"), ("Levantarse, aseo y desayuno, con ayuda cuando hace falta.", "Getting up, washing and breakfast, with help where needed.")),
                                (("9:30 a 10:30", "9:30 to 10:30"), ("Medicación y control de tensión según la pauta de cada persona.", "Medication and blood pressure checks according to each person's plan.")),
                                (("10:30 a 12:30", "10:30 to 12:30"), ("Gimnasia suave, taller de memoria o paseo por el jardín.", "Gentle exercise, memory workshop or a walk in the garden.")),
                                (("13:00 a 14:30", "1:00 to 2:30 pm"), ("Comida.", "Lunch.")),
                                (("14:30 a 16:30", "2:30 to 4:30 pm"), ("Descanso, lectura o televisión en la sala.", "Rest, reading or television in the lounge.")),
                                (("16:30 a 18:00", "4:30 to 6:00 pm"), ("Merienda, música, juegos de mesa o manualidades. Hora de visitas.", "Afternoon snack, music, board games or crafts. Visiting time.")),
                                (("19:30 a 20:30", "7:30 to 8:30 pm"), ("Cena.", "Dinner.")),
                                (("20:30 a 22:00", "8:30 to 10:00 pm"), ("Tertulia o televisión y preparación para dormir. Por la noche hay personal despierto.", "Chat or television and getting ready for bed. Staff are awake overnight."))]),
                         ], anchor="dia", gap=28),
        bloque_checks(("Comidas y dietas", "Meals and diets"), [
            ("Cocina propia, con menú de temporada que cambia cada semana.", "Our own kitchen, with a seasonal menu that changes every week."),
            ("El menú de la semana está a la vista, con los alérgenos de cada plato.", "The week's menu is on display, with the allergens in each dish."),
            ("Dietas para diabetes, hipertensión, sin sal, sin gluten o sin lactosa, siempre con indicación médica.", "Diets for diabetes, high blood pressure, low salt, gluten-free or lactose-free, always on medical advice."),
            ("Textura blanda o triturada para quien tiene dificultad para masticar o tragar.", "Soft or puréed texture for anyone who has difficulty chewing or swallowing."),
            ("Se ayuda a comer a quien lo necesita, sin prisa y sin quitar el plato antes de tiempo.", "People who need it are helped to eat, without rushing and without taking the plate away early."),
            ("Hay agua y algo de beber a la vista a todas horas.", "Water and drinks are within reach at all hours."),
        ], eyebrow=("Cocina", "Kitchen"), tono="relleno", anchor="comidas"),
        bloque_valores(("Actividades", "Activities"), [
            (("Para moverse", "Keeping active"),
             ("Gimnasia suave, estiramientos, paseos y baile sentados. Se adaptan a quien usa bastón, andador o silla.",
              "Gentle exercise, stretching, walks and seated dancing. They are adapted for anyone using a stick, walker or wheelchair.")),
            (("Para la memoria", "For the memory"),
             ("Juegos de palabras, cartas, lectura en voz alta, fotos antiguas y talleres de memoria.",
              "Word games, cards, reading aloud, old photographs and memory workshops.")),
            (("Para estar acompañado", "For company"),
             ("Música, manualidades, huerto, cine en la sala y celebraciones de cumpleaños y fiestas.",
              "Music, crafts, a vegetable patch, films in the lounge and birthday and holiday celebrations.")),
        ], eyebrow=("Cada día", "Every day"), cols=3, estilo="borde", anchor="actividades"),
        bloque_checks(("Atención sanitaria", "Health care"), [
            ("Control de la medicación según la pauta de su médico, con registro de cada toma.", "Medication managed according to the person's doctor's plan, with a record of each dose."),
            ("Curas, toma de tensión, glucemia y peso con la frecuencia que se haya indicado.", "Dressings, blood pressure, blood sugar and weight checks as often as prescribed."),
            ("Acompañamiento a las citas del centro de salud y del hospital, y aviso a la familia.", "Company at health centre and hospital appointments, with the family told."),
            ("Ayuda para la movilidad y prevención de caídas.", "Help with mobility and fall prevention."),
            ("Ante una urgencia se llama al 112 y se avisa a la familia de inmediato.", "In an emergency we call 112 and tell the family straight away."),
            ("Pregunta en la visita qué atención sanitaria hay en cada turno y cómo se coordina con su médico.", "At the visit, ask what health care is available on each shift and how it is coordinated with their doctor."),
        ], tono="base", anchor="salud"),
        bloque_galeria(("Por dentro", "Inside"), [
            ("Comedor con mesas de cuatro y sillas con reposabrazos", "Dining room with tables for four and chairs with armrests"),
            ("Sala de actividades con mesas largas y estantes con juegos", "Activity room with long tables and shelves of games"),
            ("Pasillo luminoso con barandillas a ambos lados", "Bright corridor with handrails on both sides"),
            ("Habitación individual con cama articulada y butaca", "Single room with an adjustable bed and an armchair")],
            cols=2, ratio=1.4, tono="relleno"),
        bloque_cta(("¿Quieres verlo en persona?", "Would you like to see it in person?"), None, [visita(), whatsapp("ghost")], tono="base"),
    ])

    ingreso = pagina("ingreso", ("plazas-e-ingreso", "places-and-admission"), ("Plazas e ingreso", "Places and admission"),
                     ("Tipos de plaza y precios, cómo es el ingreso paso a paso, visitas de familiares, comunicación con las familias y preguntas frecuentes.",
                      "Types of place and prices, how admission works step by step, family visits, communication with families and frequently asked questions."), [
        hero(("Plazas e ingreso", "Places and admission"),
             ("Qué tipos de plaza hay, cuánto cuestan, cómo se entra y cuándo se puede visitar. Si algo no queda claro, llámanos.",
              "What kinds of place there are, what they cost, how people move in and when you can visit. If anything is unclear, call us."),
             [visita(), llamar("secondary")], variante="texto", extra=peq(REGISTRO, color="$muted")),
        bloque_pasos(("Cómo es el ingreso", "How admission works"), [
            (("Visita al centro", "Visit the centre"), ("Vienes a ver la residencia y a conocer al equipo, con quien vaya a vivir aquí si le apetece. Sin compromiso.", "You come to see the home and meet the team, with the person who may live here if they would like to. No obligation.")),
            (("Hablamos de la persona", "We talk about the person"), ("Nos cuentas cómo es, qué necesita, qué le gusta y qué medicación toma. Con esto vemos si podemos atenderla.", "You tell us what they are like, what they need, what they enjoy and what medication they take. With that we see whether we can look after them.")),
            (("Papeles y reserva", "Paperwork and reservation"), ("Preparamos el contrato y los documentos, y reservamos la plaza cuando haya una libre.", "We prepare the contract and the documents, and reserve the place when one is free.")),
            (("Primeros días", "First days"), ("Recibimos a la persona con calma, le presentamos al equipo y a sus vecinos de planta, y hablamos con la familia a los pocos días.", "We welcome the person calmly, introduce them to the team and their neighbours, and talk to the family after a few days.")),
        ], intro=("No hay que decidir en un día. Muchas familias vienen más de una vez.", "There is no need to decide in a day. Many families come more than once."),
            tono="relleno", anchor="pasos"),
        bloque_precios(("Tipos de plaza", "Types of place"), ("Tipos de plaza y precio mensual", "Types of place and monthly price"),
                       [("Plaza", "Place"), ("Qué incluye", "What it includes"), ("Al mes", "Per month")],
                       [(("Residencia, habitación individual", "Care home, single room"), ("Alojamiento, comidas, limpieza, lavandería y cuidados diarios", "Accommodation, meals, cleaning, laundry and daily care"), desde(2100)),
                        (("Residencia, habitación compartida", "Care home, shared room"), ("Lo mismo, en habitación para dos personas", "The same, in a room for two people"), desde(1700)),
                        (("Estancia temporal", "Short stay"), ("Para convalecencias o para el respiro de la familia. Se paga por días", "For recovery or to give the family a break. Paid by the day"), desde(75)),
                        (("Centro de día, jornada completa", "Day centre, full day"), ("Comida, merienda, actividades y transporte opcional", "Lunch, afternoon snack, activities and optional transport"), desde(850)),
                        (("Centro de día, media jornada", "Day centre, half day"), ("Mañana o tarde, con comida o merienda según el turno", "Morning or afternoon, with lunch or a snack depending on the shift"), desde(550))],
                       nota=NOTA, boton_=visita(), tono="base", anchor="plazas"),
        bloque_texto(("Ayudas y financiación", "Help with the cost"), [
            ("Hay ayudas públicas para pagar una plaza si la persona tiene reconocida una situación de dependencia, y las condiciones cambian según la comunidad y el año. Consulta las vigentes en los servicios sociales de tu ayuntamiento o de tu comunidad.",
             "There is public help towards the cost of a place if the person has been officially recognised as dependent, and the conditions vary by region and by year. Check the current ones with your town hall's or your region's social services."),
            ("Si nos cuentas tu caso, te explicamos qué papeles suelen pedir. No podemos decirte si te la concederán.",
             "If you tell us your situation, we explain which papers they usually ask for. We cannot tell you whether it will be granted."),
        ], tono="relleno", anchor="ayudas"),
        seccion("text", [titular(("Horario de visitas", "Visiting hours"),
                                 ("Puedes visitar sin avisar dentro de este horario. Para comer con tu familiar, avisa el día anterior.",
                                  "You can visit without notice during these hours. To have lunch with your relative, let us know the day before.")),
                         tabla(("Horario de visitas y de atención a familias", "Visiting and family enquiry hours"), [("Qué", "What"), ("Cuándo", "When")],
                               [(("Visitas de familiares", "Family visits"), ("Todos los días, de 11:00 a 13:00 y de 16:00 a 20:00", "Every day, 11:00 am to 1:00 pm and 4:00 pm to 8:00 pm")),
                                (("Visitas al centro para conocerlo", "Tours of the centre"), ("Lunes a viernes, de 10:00 a 14:00", "Monday to Friday, 10:00 am to 2:00 pm")),
                                (("Por la tarde", "In the afternoon"), ("Con cita previa", "By appointment"))])],
                anchor="visitas-familiares", gap=24),
        bloque_checks(("Comunicación con las familias", "Communication with families"), [
            ("Una persona de referencia en la residencia, con teléfono directo.", "A named contact at the home, with a direct phone line."),
            ("Llamada inmediata ante una caída, un ingreso en el hospital o un cambio de salud.", "An immediate call after a fall, a hospital admission or a change in health."),
            ("Reunión con la familia a los pocos días del ingreso y después cada cierto tiempo.", "A meeting with the family a few days after admission and then at regular intervals."),
            ("Llamadas y videollamadas con la persona a la hora que acordemos.", "Phone and video calls with the person at a time we agree."),
            (DATOS[0], DATOS[1]),
        ], eyebrow=("Familias", "Families"), cols=2, tono="base", anchor="comunicacion"),
        bloque_faq(("Preguntas frecuentes", "Frequently asked questions"), [
            (("¿Puede traer sus muebles y sus cosas?", "Can they bring their own furniture and belongings?"),
             ("Sí: una butaca, fotos, un cuadro o una manta de casa. Para los muebles grandes, mejor lo hablamos antes por la seguridad de los pasillos.",
              "Yes: an armchair, photos, a picture or a blanket from home. For large furniture, let us talk first because of corridor safety.")),
            (("¿Qué pasa si sus necesidades cambian?", "What happens if their needs change?"),
             ("Lo revisamos con vosotros. Si necesita más ayuda, se ajusta el cuidado y se informa del coste antes. Si necesita una atención que aquí no podemos dar, os orientamos hacia otro recurso.",
              "We review it with you. If they need more help, care is adjusted and the cost is explained beforehand. If they need care we cannot provide here, we point you to another service.")),
            (("¿Puede salir con su familia?", "Can they go out with their family?"),
             ("Sí. Avisa en recepción al salir y al volver, y nos llevamos la medicación necesaria. Las salidas son una parte importante de la vida de cualquiera.",
              "Yes. Tell reception when you leave and return, and we send along any medication needed. Outings are an important part of anyone's life.")),
            (("¿Quién decide sobre su vida aquí?", "Who makes decisions about their life here?"),
             ("La propia persona, siempre que pueda. Si no puede, su representante legal, con quien hablamos de lo importante.",
              "The person themselves, whenever they can. If they cannot, their legal representative, whom we talk to about anything important.")),
            (("¿Se hacen fotos de los residentes?", "Are residents photographed?"),
             ("Solo con autorización por escrito de la persona o de su representante, y puedes retirarla cuando quieras.",
              "Only with written permission from the person or their representative, and you can withdraw it whenever you like.")),
        ], tono="relleno"),
    ])

    cont = pagina("contacto", ("visita", "visit"), ("Pide una visita", "Ask for a visit"),
                  ("Pide una visita a la residencia con el formulario, por teléfono o por WhatsApp. Horario de visitas y atención a familias, y cómo llegar.",
                   "Ask for a visit to the care home with the form, by phone or on WhatsApp. Visiting and family enquiry hours, and how to find us."), [
        hero(("Pide una visita", "Ask for a visit"),
             ("Llámanos, escríbenos por WhatsApp o rellena el formulario. Te proponemos un día y una hora, y te enseñamos la residencia con calma.",
              "Call us, message us on WhatsApp or fill in the form. We suggest a day and time and show you around without rushing."),
             [llamar(), whatsapp()], variante="texto", extra=peq(REGISTRO, color="$muted")),
        bloque_contacto(("Ven a conocernos", "Come and meet us"),
                        ("Con unos datos te llamamos para fijar la visita. Dura media hora y puedes venir con quien quieras.",
                         "With a few details we will call you to set the visit. It lasts half an hour and you can bring anyone you like."),
                        campos_cita(extra=[
                            campo("plaza", "select", ("¿Qué te interesa?", "What are you interested in?"), False,
                                  opciones=[("Plaza de residencia", "A care home place"), ("Estancia temporal", "A short stay"),
                                            ("Centro de día", "Day centre"), ("Aún no lo sé", "Not sure yet")]),
                            campo("correo", "email", ("Correo electrónico (opcional)", "Email (optional)"), False, auto="email", maxlen=200)]),
                        ("Pedir visita", "Ask for a visit"), horas, anchor="visita",
                        notas=("Los datos de este formulario solo se usan para atender tu solicitud. No hace falta que nos cuentes la salud de la persona hasta que hablemos.",
                               "The details in this form are used only to answer your request. You do not need to tell us about the person's health until we speak."),
                        exito=("Gracias. Te llamamos para acordar el día y la hora de la visita.",
                               "Thank you. We will call you to agree the day and time of the visit.")),
        bloque_mapa(("Cómo llegar", "How to find us"),
                    ("Hay parada de autobús cerca y plazas para aparcar junto a la entrada. La puerta no tiene escalones.",
                     "There is a bus stop nearby and parking spaces by the entrance. The door has no steps."), tono="relleno"),
    ])

    return plantilla(KEY, ("Residencia y centro de día para mayores", "Care home and day centre for older people"),
                     ("Residencia y centro de día con servicios, un día en la residencia, tipos de plaza, ingreso en pasos, visitas, comunicación con familias, nº de registro, horario y formulario de visita.",
                      "Care home and day centre with services, a day at the home, types of place, admission in steps, visits, communication with families, registration number, opening hours and a visit form."),
                     "salud", ("residencia mayores residencia de ancianos ancianos geriatrico centro de dia tercera edad personas mayores abuelos dependencia plazas estancia temporal respiro familiar cuidados visita ingreso",
                               "care home nursing home residential home older people elderly senior day centre respite short stay long-term care dementia visit admission"),
                     look, cabecera(nav, cta_cab),
                     pie(nav, ("Residencia y centro de día para personas mayores. Nº de registro: 00-0000.",
                               "Care home and day centre for older people. Registration no.: 00-0000."),
                         ("Visitas todos los días, de 11:00 a 13:00 y de 16:00 a 20:00.", "Visits every day, 11 am to 1 pm and 4 pm to 8 pm.")),
                     [inicio, vida, ingreso, cont], vertical="residencia-mayores")
