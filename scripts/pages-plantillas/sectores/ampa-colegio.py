# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
from lib import *

KEY = "ampa-colegio"


def _c(background, surface, text, muted, primary, on_primary, border, inverse, on_inverse):
    return dict(background=background, surface=surface, text=text, muted=muted, primary=primary,
                on_primary=on_primary, border=border, inverse=inverse, on_inverse=on_inverse)


# Paleta propia: rosa de orquídea y ciruela clara. No usa fondo $inverse.
ORQUIDEA = dict(colores=_c("#fbf1f7", "#f4e0ec", "#2b1824", "#6a4a5e", "#91467a", "#ffffff", "#ebcfe0", "#2e1a28", "#fbeef6"),
                fuentes=("manrope", "noto-sans"), radio=(6, 12, 18), peso=700)

ESTE_AVISO = ("Comunicado de ejemplo: cámbialo por el último que enviasteis a las familias, con su fecha.",
              "Example notice: replace it with the latest one you sent to families, with its date.")


def build():
    nueva(KEY)
    look = ORQUIDEA
    malos = comprobar_paleta("orquidea", look["colores"])
    assert not malos, malos
    nav = [("inicio", ("Inicio", "Home")), ("actividades", ("Actividades", "Activities")),
           ("familias", ("Familias", "Families")), ("socios", ("Hazte socio", "Join us"))]
    horas = [(("Martes y jueves", "Tuesday and Thursday"), ("15:30 a 16:30", "3:30 pm to 4:30 pm")),
             (("Primer viernes de mes", "First Friday of the month"), ("9:00 a 10:00", "9:00 am to 10:00 am")),
             (("Fuera de ese horario", "Outside these times"), ("Por correo o WhatsApp", "By email or WhatsApp"))]

    def asociarme(var="primary"):
        return boton(("Hacerme socio", "Become a member"), a_pagina("socios", "contacto"), var)

    inicio = pagina("inicio", ("", ""), ("AMPA del colegio", "School parents' association"),
                    ("Asociación de madres y padres del colegio: actividades extraescolares, comedor, comunicados, calendario y cómo asociarte.",
                     "The school's parents' association: after-school activities, canteen, notices, calendar and how to join."), [
        hero(("Las familias del colegio, juntas", "The school's families, together"),
             ("Somos la asociación de madres y padres del colegio. Organizamos actividades, hablamos con la dirección y nos ayudamos entre familias. Hay sitio para ti.",
              "We are the school's parents' association. We organise activities, talk to the head teacher and help each other out. There is a place for you."),
             [asociarme(), llamar("secondary"), whatsapp("ghost")], eyebrow=("AMPA", "Parents' association"),
             imagen_=("landscape", ("Aula vacía con mesas y mochilas colgadas", "Empty classroom with desks and bags on hooks"))),
        bloque_valores(("Qué hacemos", "What we do"), [
            (("Actividades extraescolares", "After-school activities"),
             ("Inglés, deporte, arte y música después de clase, a precios pensados para las familias.",
              "English, sport, art and music after class, at prices designed for families.")),
            (("Comedor y acogida", "Canteen and early drop-off"),
             ("Seguimos cómo funcionan y trasladamos al colegio lo que las familias nos cuentan.",
              "We keep an eye on how they run and pass on to the school what families tell us.")),
            (("Fiestas y encuentros", "Parties and gatherings"),
             ("Castañada, Navidad, fin de curso. Cosas para que niños y familias se conozcan.",
              "Chestnut day, Christmas, end of year. Things that help children and families get to know each other.")),
            (("Voz de las familias", "Families' voice"),
             ("Llevamos vuestras propuestas al colegio y os contamos qué se ha acordado.",
              "We take your suggestions to the school and tell you what has been agreed.")),
        ], cols=4, eyebrow=("Lo que hacemos", "What we do"), estilo="relleno", tono="base"),
        bloque_texto(("Quiénes somos", "Who we are"), [
            ("Somos madres, padres y tutores del colegio que dedican un rato a la semana a que el curso vaya mejor para todos. Nadie cobra: lo que se recauda vuelve a las familias en actividades y ayudas.",
             "We are parents and guardians from the school who give a little time each week to making the year better for everyone. Nobody is paid: what we raise goes back to families in activities and support."),
            ("Cualquier familia puede asociarse, y puedes ayudar tanto como quieras: desde ir a una fiesta hasta formar parte de la junta.",
             "Any family can join, and you can help as much as you like: from coming to a party to joining the board."),
        ], imagen_=("photo", ("Mesa con material para preparar la fiesta del colegio", "Table with materials for preparing the school party")),
            eyebrow=("La asociación", "The association"), tono="relleno", invertir=True),
        bloque_valores(("Próximas citas", "Coming up"), [
            (("Reunión de bienvenida", "Welcome meeting"),
             ("Cita de ejemplo: cámbiala por la próxima reunión, con día, hora y lugar.",
              "Example event: replace it with the next meeting, with day, time and place.")),
            (("Fiesta o actividad", "Party or activity"),
             ("Cita de ejemplo: cámbiala por la próxima fiesta o actividad abierta a las familias.",
              "Example event: replace it with the next party or activity open to families.")),
        ], cols=2, tono="base", estilo="borde", anchor="citas",
            intro=("El calendario completo está en la página de Familias.", "The full calendar is on the Families page.")),
        bloque_cta(("¿Te unes?", "Will you join us?"),
                   ("Asociarte es sencillo y la cuota es pequeña. Rellenas un formulario y te contestamos esta semana.",
                    "Joining is simple and the fee is small. You fill in a form and we reply this week."),
                   [asociarme(), whatsapp("ghost")], tono="relleno"),
    ])

    actividades = pagina("actividades", ("actividades", "activities"), ("Actividades y comedor", "Activities and canteen"),
                         ("Actividades extraescolares con días y cuotas, comedor y acogida temprana, y respuestas a las dudas más habituales.",
                          "After-school activities with days and fees, canteen and early drop-off, and answers to common questions."), [
        hero(("Actividades extraescolares", "After-school activities"),
             ("Después de clase, en el propio colegio y con monitores. Las cuotas son más bajas para las familias socias.",
              "After class, at the school itself and with instructors. Fees are lower for member families."),
             [asociarme(), llamar("secondary")], variante="texto"),
        bloque_precios(("Actividades de este curso", "This year's activities"),
                       ("Actividades extraescolares de ejemplo", "Example after-school activities"),
                       [("Actividad", "Activity"), ("Cursos", "Years"), ("Días y hora", "Days and time"),
                        ("Socias/os", "Members"), ("No socias/os", "Non-members")],
                       [(("Inglés", "English"), ("1.º a 6.º de Primaria", "Primary years 1 to 6"),
                         ("Lunes y miércoles, 16:00 a 17:00", "Monday and Wednesday, 4:00 pm to 5:00 pm"), eur(18), eur(22)),
                        (("Ajedrez", "Chess"), ("3.º a 6.º de Primaria", "Primary years 3 to 6"),
                         ("Martes, 16:00 a 17:00", "Tuesday, 4:00 pm to 5:00 pm"), eur(12), eur(15)),
                        (("Judo", "Judo"), ("Infantil y Primaria", "Nursery and Primary"),
                         ("Martes y jueves, 16:00 a 17:00", "Tuesday and Thursday, 4:00 pm to 5:00 pm"), eur(20), eur(25)),
                        (("Robótica", "Robotics"), ("3.º a 6.º de Primaria", "Primary years 3 to 6"),
                         ("Jueves, 16:00 a 17:30", "Thursday, 4:00 pm to 5:30 pm"), eur(22), eur(27)),
                        (("Teatro", "Drama"), ("1.º a 6.º de Primaria", "Primary years 1 to 6"),
                         ("Viernes, 16:00 a 17:00", "Friday, 4:00 pm to 5:00 pm"), eur(14), eur(18)),
                        (("Fútbol sala", "Five-a-side football"), ("1.º a 6.º de Primaria", "Primary years 1 to 6"),
                         ("Lunes y miércoles, 17:00 a 18:00", "Monday and Wednesday, 5:00 pm to 6:00 pm"), eur(16), eur(20))],
                       intro=("Cuota mensual por niño o niña. Cada actividad se abre con un mínimo de plazas y se cierra cuando se llenan.",
                              "Monthly fee per child. Each activity opens with a minimum number of places and closes when it is full."),
                       nota=("Actividades y cuotas de ejemplo: cámbialas por las vuestras.",
                             "Example activities and fees: replace them with your own."),
                       tono="base", anchor="extraescolares"),
        bloque_precios(("Comedor y acogida temprana", "Canteen and early drop-off"),
                       ("Comedor y acogida: horarios y cuotas de ejemplo", "Canteen and early drop-off: example times and fees"),
                       [("Servicio", "Service"), ("Horario", "Time"), ("Cuota", "Fee")],
                       [(("Acogida temprana", "Early drop-off"), ("7:30 a 9:00", "7:30 am to 9:00 am"),
                         ("Desde 35 € al mes o 3 € por día", "From €35 a month or €3 a day")),
                        (("Comedor", "Canteen"), ("14:00 a 16:00", "2:00 pm to 4:00 pm"),
                         ("6,50 € por día", "€6.50 a day"))],
                       intro=("El comedor y la acogida los organiza el colegio con una empresa o con la AMPA. Aquí os explicamos cómo funcionan y a quién preguntar.",
                              "The school organises the canteen and early drop-off with a company or with the association. Here we explain how they work and who to ask."),
                       nota=("Horarios y cuotas de ejemplo: cámbialos por los de tu colegio. Indica aquí quién gestiona cada servicio y cómo se informa de alergias y menús.",
                             "Example times and fees: replace them with those of your school. Say here who runs each service and how allergies and menus are reported."),
                       tono="relleno", anchor="comedor"),
        bloque_faq(("Preguntas frecuentes", "Frequently asked questions"), [
            (("¿Puede apuntarse un niño cuya familia no es socia?", "Can a child whose family is not a member sign up?"),
             ("Sí, con una cuota algo mayor. Si la familia se asocia, la cuota baja desde el mes siguiente.",
              "Yes, with a slightly higher fee. If the family joins, the fee goes down from the following month.")),
            (("¿Cómo me apunto a una actividad?", "How do I sign up for an activity?"),
             ("Escríbenos o llámanos con el nombre de la actividad y el curso. Te confirmamos si hay plaza.",
              "Write or call us with the name of the activity and the year. We confirm whether there is a place.")),
            (("¿Quién recoge a los niños?", "Who picks the children up?"),
             ("Un monitor los acompaña hasta la salida y solo los entrega a las personas autorizadas por la familia.",
              "An instructor takes them to the exit and only hands them over to the people the family has authorised.")),
            (("¿Y si una familia no puede pagar la cuota?", "What if a family cannot afford the fee?"),
             ("Habla con la junta. Tratamos cada caso con discreción y buscamos una solución.",
              "Talk to the board. We deal with each case discreetly and look for a solution.")),
        ], tono="base"),
        bloque_cta(("¿Apuntamos a tus hijos?", "Shall we sign your children up?"), None, tono="relleno"),
    ])

    familias = pagina("familias", ("familias", "families"), ("Comunicados, calendario y junta", "Notices, calendar and board"),
                      ("Últimos comunicados, calendario del curso, junta directiva y cómo protegemos los datos de las familias.",
                       "Latest notices, the school-year calendar, the board and how we protect families' data."), [
        hero(("Para las familias", "For families"),
             ("Aquí encontrarás lo último que hemos comunicado, las fechas del curso y quién forma la junta.",
              "Here you will find our latest notices, the dates for the year and who is on the board."),
             [asociarme(), whatsapp()], variante="texto"),
        bloque_valores(("Últimos comunicados", "Latest notices"), [
            (("Comunicado del mes", "This month's notice"), ESTE_AVISO),
            (("Reunión con la dirección", "Meeting with the head teacher"), ESTE_AVISO),
            (("Campaña en marcha", "Current campaign"), ESTE_AVISO),
        ], estilo="borde", tono="base", anchor="comunicados",
            intro=("Los comunicados también llegan por correo y por el grupo de WhatsApp de la AMPA.",
                   "Notices also reach you by email and through the association's WhatsApp group.")),
        bloque_precios(("Calendario del curso", "School-year calendar"),
                       ("Calendario de ejemplo del curso", "Example school-year calendar"),
                       [("Cuándo", "When"), ("Qué", "What"), ("Dónde", "Where")],
                       [(("Septiembre", "September"), ("Reunión de bienvenida y apertura de inscripciones", "Welcome meeting and sign-ups open"),
                         ("Salón de actos", "Assembly hall")),
                        (("Octubre", "October"), ("Comienzo de las actividades extraescolares", "After-school activities begin"),
                         ("Aulas y patio", "Classrooms and playground")),
                        (("Noviembre", "November"), ("Asamblea general de socios", "General members' meeting"), ("Salón de actos", "Assembly hall")),
                        (("Diciembre", "December"), ("Festival de Navidad", "Christmas show"), ("Patio o gimnasio", "Playground or gym")),
                        (("Mayo", "May"), ("Jornada de puertas abiertas de las actividades", "Open day for the activities"),
                         ("Patio y aulas", "Playground and classrooms")),
                        (("Junio", "June"), ("Fiesta de fin de curso", "End-of-year party"), ("Patio", "Playground"))],
                       nota=("Fechas de ejemplo: cámbialas por las del calendario de tu colegio.",
                             "Example dates: replace them with those of your school's calendar."),
                       tono="relleno", anchor="calendario", kind="text"),
        bloque_equipo(("La junta directiva", "The board"), [
            (("Presidencia", "Chair"),
             ("Representa a la AMPA ante la dirección del colegio y coordina la junta.",
              "Represents the association with the head teacher and coordinates the board.")),
            (("Secretaría", "Secretary"),
             ("Convoca las reuniones, levanta acta y envía los comunicados.",
              "Calls the meetings, takes the minutes and sends the notices.")),
            (("Tesorería", "Treasurer"),
             ("Lleva las cuentas y las presenta a las familias en la asamblea.",
              "Keeps the accounts and presents them to families at the general meeting.")),
        ], intro=("La junta se renueva cada pocos cursos. Si te gustaría formar parte, díselo a cualquiera de nosotros.",
                  "The board is renewed every few years. If you would like to be part of it, tell any of us."),
            tono="base"),
        bloque_texto(("Datos de familias y menores", "Family and children's data"), [
            ("Usamos los datos de las familias solo para gestionar la asociación y no los compartimos; no publicamos fotos identificables de menores sin el permiso escrito de sus familias.",
             "We use families' data only to run the association and we do not share it; we do not publish identifiable photos of children without their families' written permission."),
        ], eyebrow=("Privacidad", "Privacy"), tono="relleno"),
    ])

    socios = pagina("socios", ("hazte-socio", "join"), ("Hazte socio y contacto", "Join us and contact"),
                    ("Cuota anual, cómo asociarte en cuatro pasos, formulario de alta, horario de atención y cómo llegar al colegio.",
                     "Annual fee, how to join in four steps, a sign-up form, opening times and how to get to the school."), [
        hero(("Hazte socio de la AMPA", "Join the parents' association"),
             ("Una cuota al año por familia, sin importar cuántos hijos tengas. Con ella financiamos las actividades, las fiestas y la formación para familias.",
              "One fee a year per family, however many children you have. It pays for activities, parties and workshops for families."),
             [llamar(), whatsapp()], variante="texto"),
        bloque_precios(("Cuota", "Fee"), ("Cuota de socio de ejemplo", "Example membership fee"),
                       [("Concepto", "Item"), ("Importe", "Amount")],
                       [(("Cuota anual por familia", "Annual fee per family"), eur(25)),
                        (("Alta a partir de enero", "Joining from January"), eur(15))],
                       nota=("Cuotas de ejemplo: cámbialas por las vuestras. Se paga una vez por curso, por transferencia o en efectivo en la reunión.",
                             "Example fees: replace them with your own. It is paid once a school year, by bank transfer or in cash at the meeting."),
                       tono="base", anchor="cuota"),
        bloque_pasos(("Cómo asociarte", "How to join"), [
            (("Rellena el formulario", "Fill in the form"),
             ("Te pedimos tu nombre, un teléfono y un correo. No hace falta que nos digas los nombres de tus hijos.",
              "We ask for your name, a phone number and an email. You do not need to tell us your children's names.")),
            (("Paga la cuota", "Pay the fee"),
             ("Te enviamos los datos para la transferencia o te decimos cuándo podemos cobrarla en mano.",
              "We send you the details for the transfer or tell you when we can collect it in cash.")),
            (("Recibes la confirmación", "You get a confirmation"),
             ("Te damos de alta en la lista de comunicados y en el grupo de WhatsApp si quieres.",
              "We add you to the notices list and to the WhatsApp group if you wish.")),
            (("Cada curso, renovación", "Renew each year"),
             ("Te avisamos en septiembre. Si cambias de colegio, puedes darte de baja cuando quieras.",
              "We remind you in September. If you change schools, you can leave whenever you want.")),
        ], tono="relleno", anchor="como"),
        bloque_checks(("Para qué sirve tu cuota", "What your fee pays for"), [
            ("Material y monitores de las actividades.", "Materials and instructors for the activities."),
            ("Fiestas y encuentros del curso.", "School-year parties and gatherings."),
            ("Charlas y talleres para madres y padres.", "Talks and workshops for parents."),
            ("Ayudas para familias que lo necesitan.", "Support for families who need it."),
        ], tono="base", cols=2),
        bloque_contacto(("Hablemos", "Get in touch"),
                        ("Para hacerte socio, apuntarte a una actividad o proponer algo, escríbenos o pasa por el despacho de la AMPA.",
                         "To join, sign up for an activity or suggest something, write to us or drop by the association's room."),
                        [campo("nombre", "text", ("Nombre y apellidos", "Full name"), True, auto="name", maxlen=120),
                         campo("correo", "email", ("Correo electrónico", "Email"), True, auto="email", maxlen=200),
                         campo("telefono", "tel", ("Teléfono (opcional)", "Phone (optional)"), False, auto="tel", maxlen=30),
                         campo("etapa", "select", ("Etapa de tus hijos", "Your children's stage"), False,
                               opciones=[("Infantil", "Nursery"), ("Primaria", "Primary"), ("Secundaria", "Secondary")]),
                         campo("mensaje", "textarea", ("Mensaje (opcional)", "Message (optional)"), False, maxlen=1000)],
                        ("Quiero ser socio", "I want to join"), horas,
                        notas=("Los datos de las familias y de sus hijos se usan solo para gestionar la asociación. Nunca publicamos fotos identificables de menores.",
                               "Families' and children's data is used only to run the association. We never publish identifiable photos of children."),
                        exito=("Gracias. Te escribimos esta semana para confirmar el alta.",
                               "Thank you. We will write to you this week to confirm your membership."),
                        tono="relleno"),
        bloque_mapa(titulo=("Cómo llegar al colegio", "How to get to the school"),
                    texto_=("Indica aquí en qué edificio y planta está el despacho de la AMPA.",
                            "Say here which building and floor the association's room is on."), tono="base"),
    ])

    return plantilla(KEY, ("AMPA o asociación de familias", "Parents' association"),
                     ("Sitio de una AMPA: quiénes somos, extraescolares con cuotas, comedor, comunicados, calendario, junta, alta de socios con formulario y botones de llamar y WhatsApp.",
                      "Website for a school parents' association: who we are, after-school activities with fees, canteen, notices, calendar, board, membership form and call and WhatsApp buttons."),
                     "comunidad", ("ampa asociacion de madres y padres apa asociacion de familias colegio cole instituto extraescolares actividades extraescolares comedor escolar acogida temprana madrugadores junta directiva cuota de socio",
                                   "parents association pta school parents extracurricular after-school clubs school canteen breakfast club board membership"),
                     look, cabecera(nav, marco([llamar(), whatsapp("ghost")], dir="row", gap=12, wrap=True)),
                     pie(nav, ("Asociación de madres y padres del colegio. Actividades, comedor y voz de las familias.",
                               "The school's parents' association. Activities, canteen and a voice for families."),
                         ("Despacho: martes y jueves 15:30 a 16:30.", "Association room: Tuesday and Thursday 3:30 pm to 4:30 pm.")),
                     [inicio, actividades, familias, socios])
