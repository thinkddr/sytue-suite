# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
from lib import *
from looks import LOOKS

KEY = "club-deportivo"

# Paleta propia: azul marino de equipación con rojo de club; pasa AA. Sin fondo $inverse (primary/inverse < 3).
CLUB = dict(background="#f6f8fb", surface="#e6ecf4", text="#0c1b33", muted="#46556b", primary="#c01030",
            on_primary="#ffffff", border="#d3dbe7", inverse="#0c1b33", on_inverse="#f4f7fb")


def build():
    nueva(KEY)
    look = dict(colores=CLUB, fuentes=("montserrat", "noto-sans"), peso=800, mayus="uppercase", ls=0.02,
                h1=(46, 36, 28), radio=(2, 6, 10))
    malos = comprobar_paleta("club-deportivo", CLUB)
    assert not malos, malos
    nav = [("inicio", ("Inicio", "Home")), ("equipos", ("Categorías y cuotas", "Teams and fees")),
           ("club", ("Instalaciones y partidos", "Facilities and matches")),
           ("contacto", ("Inscripción y contacto", "Registration and contact"))]
    horas = [(("Lunes a jueves", "Monday to Thursday"), ("17:00 a 21:00", "5:00 pm to 9:00 pm")),
             (("Viernes", "Friday"), ("17:00 a 20:00", "5:00 pm to 8:00 pm")),
             (("Sábado", "Saturday"), ("9:00 a 14:00, día de partidos", "9:00 am to 2:00 pm, match day")),
             (("Domingo", "Sunday"), ("Cerrado", "Closed"))]
    cab = [("Categoría", "Category"), ("Edad", "Age")]
    nota_p = ("Precios de ejemplo: cámbialos por los tuyos.", "Example prices: replace them with your own.")

    def inscribe(var="primary"):
        return boton(("Apuntarme al club", "Join the club"), a_pagina("contacto", "contacto"), var)

    inicio = pagina("inicio", ("", ""), ("Club deportivo", "Sports club"),
                    ("Club deportivo local con categorías por edades, horarios de entrenamiento, cuotas, instalaciones e inscripción.",
                     "Local sports club with age categories, training times, fees, facilities and registration."), [
        hero(("Un club para jugar y para pertenecer", "A club to play in and belong to"),
             ("Entrenamos de lunes a viernes y jugamos los fines de semana. Hay equipo para cada edad, desde los primeros pasos hasta el adulto.",
              "We train from Monday to Friday and play at weekends. There is a team for every age, from first steps to adults."),
             [inscribe(), llamar("secondary"), whatsapp("ghost")],
             eyebrow=("Club deportivo", "Sports club"),
             imagen_=("photo", ("Jugadores del club entrenando en la pista", "Club players training on the court")), tono="relleno"),
        bloque_valores(("Qué encontrarás en el club", "What you will find at the club"), [
            (("Deporte para todos", "Sport for everyone"),
             ("Aquí juega quien quiere jugar. Cada grupo se organiza por edad y nivel, no por quién gana más.",
              "Here, whoever wants to play plays. Each group is organised by age and level, not by who wins most.")),
            (("Entrenadores cercanos", "Approachable coaches"),
             ("Personas que enseñan el deporte, cuidan del grupo y hablan con las familias cuando hace falta.",
              "People who teach the sport, look after the group and talk to families when needed.")),
            (("Un club de barrio", "A neighbourhood club"),
             ("Cuotas claras, un campo cercano y un ambiente en el que los chavales hacen amigos.",
              "Clear fees, a nearby pitch and an atmosphere in which kids make friends.")),
        ], tono="base", estilo="borde", eyebrow=("Por qué apuntarse", "Why join")),
        bloque_pasos(("Cómo apuntarte", "How to join"), [
            (("Pides una prueba", "Ask for a trial"), ("Vienes a un entrenamiento de la categoría que te corresponde. Es gratis y sin compromiso.", "You come to a training session in the category that suits you. It is free and with no commitment.")),
            (("Te inscribes", "You register"), ("Rellenas la ficha y, si eres menor, la autorización de tu padre, madre o tutor.", "You fill in the form and, if you are a minor, the permission of your parent or guardian.")),
            (("Pagas la cuota", "You pay the fee"), ("Por meses o por temporada, como te venga mejor.", "By the month or by the season, whichever suits you best.")),
            (("A jugar", "Time to play"), ("Te damos tu horario y tu equipación, y a entrenar.", "We give you your timetable and kit, and you start training.")),
        ], tono="relleno", anchor="como-apuntarte"),
        bloque_galeria(("En el campo", "On the pitch"), [
            ("Entrenamiento de los más pequeños del club", "Training session for the club's youngest players"),
            ("Partido de la categoría juvenil", "Match of the youth category"),
            ("Equipo del club celebrando tras un partido", "Club team celebrating after a match"),
            ("Entrenador explicando un ejercicio al grupo", "Coach explaining an exercise to the group")], cols=2, tono="base", ratio=1.4),
        bloque_opiniones(("Lo que dicen jugadores y familias", "What players and families say"),
                         [("los entrenadores", "the coaches"), ("el ambiente", "the atmosphere"), ("la organización", "the organisation")]),
        bloque_cta(("¿Vienes a probar?", "Coming to try it out?"),
                   ("Dinos la edad y la categoría y te decimos cuándo es el próximo entrenamiento.", "Tell us the age and category and we will tell you when the next training session is."),
                   [inscribe(), llamar("ghost")], tono="base"),
    ])

    equipos = pagina("equipos", ("categorias", "teams"), ("Categorías y cuotas", "Teams and fees"),
                     ("Categorías por edades, horarios de entrenamiento y cuotas de ejemplo del club deportivo.",
                      "Age categories, training times and example fees of the sports club."), [
        hero(("Categorías y cuotas", "Teams and fees"),
             ("Cada jugador va al grupo que le toca por edad. Estas son las categorías, cuándo se entrena y cuánto cuesta.",
              "Each player goes to the group that matches their age. These are the categories, when we train and what it costs."),
             [inscribe()], variante="texto"),
        bloque_precios(("Categorías por edades", "Categories by age"), ("Categorías de ejemplo", "Example categories"),
                       [("Categoría", "Category"), ("Edad", "Age"), ("Qué hacen", "What they do")],
                       [(("Escuela", "Academy"), ("4 a 7 años", "4 to 7"), ("Juegos y primeros gestos del deporte", "Games and the first moves of the sport")),
                        (("Benjamín", "Under 9"), ("8 y 9 años", "8 and 9"), ("Fundamentos y primeros partidos", "Basics and first matches")),
                        (("Alevín", "Under 11"), ("10 y 11 años", "10 and 11"), ("Técnica y juego en equipo", "Technique and team play")),
                        (("Infantil", "Under 13"), ("12 y 13 años", "12 and 13"), ("Táctica básica y competición", "Basic tactics and competition")),
                        (("Cadete", "Under 15"), ("14 y 15 años", "14 and 15"), ("Competición y trabajo físico", "Competition and fitness work")),
                        (("Juvenil", "Under 18"), ("16 a 18 años", "16 to 18"), ("Competición y paso al equipo sénior", "Competition and the step up to the senior team")),
                        (("Sénior", "Senior"), ("Desde 19 años", "19 and over"), ("Liga y entrenamientos de adultos", "League and adult training"))],
                       intro=("Tabla de ejemplo: cambia las categorías y las edades por las de tu deporte y tu federación.",
                              "Example table: replace the categories and ages with those of your sport and federation."),
                       anchor="categorias", tono="base"),
        bloque_precios(("Horarios de entrenamiento", "Training times"), ("Horarios de entrenamiento de ejemplo", "Example training times"),
                       [("Categoría", "Category"), ("Días", "Days"), ("Hora", "Time")],
                       [(("Escuela", "Academy"), ("Martes y jueves", "Tuesday and Thursday"), ("17:30 a 18:30", "5:30 pm to 6:30 pm")),
                        (("Benjamín y alevín", "Under 9 and under 11"), ("Lunes y miércoles", "Monday and Wednesday"), ("18:00 a 19:30", "6:00 pm to 7:30 pm")),
                        (("Infantil y cadete", "Under 13 and under 15"), ("Martes, jueves y viernes", "Tuesday, Thursday and Friday"), ("19:00 a 20:30", "7:00 pm to 8:30 pm")),
                        (("Juvenil y sénior", "Under 18 and senior"), ("Lunes, miércoles y viernes", "Monday, Wednesday and Friday"), ("20:30 a 22:00", "8:30 pm to 10:00 pm"))],
                       intro=("Los horarios de ejemplo cambian con la temporada: cámbialos por los vuestros.",
                              "The example times change with the season: replace them with your own."),
                       anchor="horarios", tono="relleno"),
        bloque_precios(("Cuotas", "Fees"), ("Cuotas de ejemplo", "Example fees"),
                       [("Concepto", "Item"), ("Importe", "Amount")],
                       [(("Cuota mensual, escuela", "Monthly fee, academy"), eur(25)),
                        (("Cuota mensual, categorías de base", "Monthly fee, youth categories"), eur(30)),
                        (("Cuota mensual, sénior", "Monthly fee, senior"), eur(35)),
                        (("Inscripción (una vez por temporada)", "Registration (once per season)"), eur(40)),
                        (("Equipación", "Kit"), desde(45))],
                       intro=("Indica aquí si hay descuento para hermanos o pago de toda la temporada de una vez.",
                              "Say here whether there is a discount for siblings or for paying the whole season at once."),
                       nota=("Precios de ejemplo: cámbialos por los tuyos. Indica qué incluye la cuota (seguro, ficha federativa, equipación).",
                             "Example prices: replace them with your own. Say what the fee includes (insurance, federation licence, kit)."),
                       anchor="cuotas", tono="base"),
        bloque_faq(("Preguntas frecuentes", "Frequently asked questions"), [
            (("¿Puedo probar antes de apuntarme?", "Can I try before I join?"),
             ("Sí. Ven a un entrenamiento de tu categoría, sin coste, y decide después.", "Yes. Come to a training session in your category at no cost and decide afterwards.")),
            (("¿Y si mi hijo no tiene experiencia?", "What if my child has no experience?"),
             ("No hace falta. Los grupos pequeños empiezan desde cero y los entrenadores van a su ritmo.", "It is not needed. The younger groups start from scratch and the coaches go at their pace.")),
            (("¿Qué hay que llevar al primer día?", "What do I need to bring on the first day?"),
             ("Ropa y calzado deportivos y una botella de agua. La equipación del club se entrega al inscribirte.", "Sports clothes and shoes and a bottle of water. The club kit is handed over when you register.")),
        ], tono="relleno"),
        bloque_cta(("¿Qué categoría te toca?", "Which category is yours?"), None, [inscribe(), whatsapp("ghost")], tono="base"),
    ])

    club = pagina("club", ("instalaciones", "facilities"), ("Instalaciones y partidos", "Facilities and matches"),
                  ("Instalaciones del club, cómo llegar y calendario de partidos de ejemplo.",
                   "Club facilities, how to get there and an example match calendar."), [
        hero(("Instalaciones y partidos", "Facilities and matches"),
             ("Dónde entrenamos, cómo llegar y cuándo se juegan los partidos.", "Where we train, how to get there and when the matches are played."),
             [boton(("Cómo llegar", "How to get there"), a_pagina("contacto", "como-llegar")), llamar("secondary")], variante="texto"),
        bloque_checks(("Nuestras instalaciones", "Our facilities"), [
            ("Campo o pista principal con iluminación para los entrenamientos de tarde.", "Main pitch or court with lighting for evening training."),
            ("Vestuarios con duchas, separados para cada equipo.", "Changing rooms with showers, separate for each team."),
            ("Zona de calentamiento y almacén de material.", "Warm-up area and equipment store."),
            ("Un rincón para las familias, con bancos y sombra.", "A spot for families, with benches and shade."),
        ], intro=("Instalaciones de ejemplo: cámbialas por las que tenga vuestro club, y pon solo lo que de verdad usáis.",
                  "Example facilities: replace them with what your club has, and list only what you actually use."),
            tono="base", cols=2, imagen_=None, anchor="instalaciones"),
        bloque_galeria(("El sitio", "The place"), [
            ("Campo principal del club visto desde la grada", "The club's main pitch seen from the stand"),
            ("Vestuarios del club", "The club's changing rooms"),
            ("Entrada a las instalaciones del club", "Entrance to the club facilities")], tipo="landscape", cols=3, tono="relleno", ratio=1.4),
        bloque_precios(("Calendario de partidos", "Match calendar"), ("Calendario de partidos de ejemplo", "Example match calendar"),
                       [("Día y hora", "Day and time"), ("Categoría", "Category"), ("Encuentro", "Match"), ("Lugar", "Venue")],
                       [(("Sábado, 10:00", "Saturday, 10:00 am"), ("Infantil", "Under 13"), ("Nombre del club contra Equipo rival", "Club name against Opposing team"), ("En casa", "Home")),
                        (("Sábado, 12:00", "Saturday, 12:00 pm"), ("Alevín", "Under 11"), ("Equipo rival contra Nombre del club", "Opposing team against Club name"), ("Fuera", "Away")),
                        (("Domingo, 11:00", "Sunday, 11:00 am"), ("Sénior", "Senior"), ("Nombre del club contra Equipo rival", "Club name against Opposing team"), ("En casa", "Home"))],
                       intro=("Calendario de ejemplo en texto: cámbialo cada jornada por los partidos reales.",
                              "Example calendar as text: replace it every round with the real matches."),
                       nota=("Calendario de ejemplo. Los horarios pueden cambiar: confírmalos con el club.",
                             "Example calendar. Times may change: confirm them with the club."),
                       anchor="partidos", tono="base"),
        bloque_cta(("¿Te vemos el sábado?", "See you on Saturday?"), None, [inscribe(), llamar("ghost")], tono="relleno"),
    ])

    cont = pagina("contacto", ("contacto", "contact"), ("Inscripción y contacto", "Registration and contact"),
                  ("Formulario de inscripción con edad y categoría, teléfono, WhatsApp, horario de secretaría y cómo llegar al club.",
                   "Registration form with age and category, phone, WhatsApp, office hours and how to get to the club."), [
        hero(("Inscríbete en el club", "Register with the club"),
             ("Rellena la ficha, llámanos o escríbenos por WhatsApp. Te decimos a qué grupo vas y cuándo empieza.",
              "Fill in the form, call us or message us on WhatsApp. We tell you which group you join and when it starts."),
             [llamar(), whatsapp()], variante="texto"),
        bloque_contacto(("Ficha de inscripción", "Registration form"),
                        ("Si el jugador o la jugadora es menor de edad, rellena la ficha su padre, madre o tutor, y le pediremos su autorización por escrito.",
                         "If the player is a minor, a parent or guardian fills in the form, and we will ask for their written permission."),
                        [campo("jugador", "text", ("Nombre del jugador o jugadora", "Player's name"), True, maxlen=120),
                         campo("edad", "text", ("Edad", "Age"), True, maxlen=3),
                         campo("categoria", "select", ("Categoría", "Category"), True,
                               opciones=[("Escuela (4 a 7 años)", "Academy (4 to 7)"), ("Benjamín y alevín (8 a 11 años)", "Under 9 and under 11 (8 to 11)"),
                                         ("Infantil y cadete (12 a 15 años)", "Under 13 and under 15 (12 to 15)"),
                                         ("Juvenil (16 a 18 años)", "Under 18 (16 to 18)"), ("Sénior (desde 19 años)", "Senior (19 and over)"),
                                         ("No lo sé", "I do not know")]),
                         campo("contacto", "text", ("Persona de contacto (padre, madre o tutor si es menor)", "Contact person (parent or guardian if a minor)"), True, auto="name", maxlen=120),
                         campo("telefono", "tel", ("Teléfono", "Phone"), True, auto="tel", maxlen=30),
                         campo("correo", "email", ("Correo electrónico", "Email"), False, auto="email", maxlen=200),
                         campo("mensaje", "textarea", ("Experiencia o dudas (opcional)", "Experience or questions (optional)"), False, maxlen=1000)],
                        ("Enviar inscripción", "Send registration"), horas,
                        exito=("Gracias. Hemos recibido la inscripción y te llamamos para confirmar el grupo y el primer día.",
                               "Thank you. We have received the registration and will call to confirm the group and the first day.")),
        bloque_mapa(texto_=("Estamos junto a las instalaciones, con aparcamiento cerca. Texto de ejemplo: cámbialo por tus indicaciones.",
                            "We are next to the facilities, with parking close by. Example text: replace it with your directions."), tono="relleno"),
    ])

    return plantilla(KEY, ("Club deportivo", "Sports club"),
                     ("Club deportivo local con categorías por edades, horarios de entrenamiento, cuotas, instalaciones, calendario de partidos de ejemplo e inscripción.",
                      "Local sports club with age categories, training times, fees, facilities, an example match calendar and registration."),
                     "comunidad", ("club deportivo club de futbol club de baloncesto club de atletismo equipo escuela deportiva categorias inscripcion entrenamientos cantera futbol baloncesto",
                                   "sports club football club basketball club athletics club team youth academy training registration"),
                     look, cabecera(nav, inscribe()),
                     pie(nav, ("Club deportivo local. Categorías por edades, entrenamientos y partidos.",
                               "Local sports club. Age categories, training and matches."),
                         ("Lunes a jueves 17:00 a 21:00. Viernes 17:00 a 20:00. Sábado 9:00 a 14:00.",
                          "Monday to Thursday 5 pm to 9 pm. Friday 5 pm to 8 pm. Saturday 9 am to 2 pm.")),
                     [inicio, equipos, club, cont])
