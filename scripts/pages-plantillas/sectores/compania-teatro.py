# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
from lib import *

KEY = "compania-teatro"


def _c(background, surface, text, muted, primary, on_primary, border, inverse, on_inverse):
    return dict(background=background, surface=surface, text=text, muted=muted, primary=primary,
                on_primary=on_primary, border=border, inverse=inverse, on_inverse=on_inverse)


# Paleta propia: violeta de telón sobre rosa de bambalinas. No usa fondo $inverse.
TELON = dict(colores=_c("#faf1fb", "#f1e0f4", "#241028", "#5f4a66", "#4b176d", "#ffffff", "#e6cfeb", "#1f0f2a", "#f8eefa"),
             fuentes=("playfair-display", "ibm-plex-sans"), radio=(2, 4, 8), h1=(54, 42, 34), peso=700)


def build():
    nueva(KEY)
    look = TELON
    malos = comprobar_paleta("telon", look["colores"])
    assert not malos, malos
    nav = [("inicio", ("Inicio", "Home")), ("cartel", ("En cartel", "Now showing")),
           ("taller", ("Taller de teatro", "Drama workshop")), ("contacto", ("Contratación y contacto", "Hire us and contact"))]
    horas = [(("Taquilla, días de función", "Box office, show days"), ("Una hora antes de cada función", "One hour before each show")),
             (("Teléfono y WhatsApp", "Phone and WhatsApp"), ("Lunes a viernes, 10:00 a 14:00 y 17:00 a 20:00",
                                                              "Monday to Friday, 10:00 am to 2:00 pm and 5:00 pm to 8:00 pm")),
             (("Ensayos", "Rehearsals"), ("Martes y jueves por la tarde, a puerta cerrada", "Tuesday and Thursday evenings, closed to visitors"))]
    nota_tarifas = ("Funciones y precios de ejemplo: cámbialos por los vuestros.",
                    "Example shows and prices: replace them with your own.")

    def entradas(var="primary"):
        return boton(("Reservar entradas", "Book tickets"), a_wa(), var, "whatsapp")

    def espectaculo(genero, titulo, sinopsis, duracion, alt):
        return tarjeta([imagen("photo", alt, ratio=1.3), etiqueta(genero), h3(titulo), p(sinopsis, color="$muted"),
                        peq(duracion), entradas("secondary")], estilo="borde", gap=12, pad=20)

    inicio = pagina("inicio", ("", ""), ("Compañía de teatro", "Theatre company"),
                    ("Compañía de teatro: espectáculos en cartel, próximas funciones, taller, funciones para colegios y eventos, y entradas por teléfono o WhatsApp.",
                     "Theatre company: shows now playing, upcoming performances, a workshop, shows for schools and events, and tickets by phone or WhatsApp."), [
        hero(("Teatro cerca de ti", "Theatre close to home"),
             ("Montamos y representamos obras para todos los públicos, damos clases de teatro y llevamos nuestros espectáculos a colegios y fiestas. Mira qué hay en cartel y reserva por teléfono o WhatsApp.",
              "We stage and perform plays for all audiences, run drama classes and take our shows to schools and festivals. See what is on and book by phone or WhatsApp."),
             [boton(("Ver la cartelera", "See what is on"), a_pagina("cartel")), entradas("secondary"), llamar("ghost")],
             eyebrow=("Compañía de teatro", "Theatre company"),
             imagen_=("photo", ("Escenario con telón abierto y focos encendidos", "Stage with the curtain open and the spotlights on"))),
        bloque_valores(("Lo que hacemos", "What we do"), [
            (("Funciones", "Performances"),
             ("Obras de teatro para adultos, jóvenes y familias, en nuestro local y de gira.",
              "Plays for adults, young people and families, at our venue and on tour.")),
            (("Taller de teatro", "Drama workshop"),
             ("Clases por edades, para quien quiere probar o para quien ya tiene experiencia.",
              "Classes by age group, for people who want to try it or who already have experience.")),
            (("Colegios", "Schools"),
             ("Funciones y actividades en horario escolar, adaptadas a cada edad.",
              "Shows and activities during school hours, adapted to each age group.")),
            (("Eventos", "Events"),
             ("Fiestas, ferias y celebraciones, en la calle o en sala.",
              "Festivals, fairs and celebrations, outdoors or indoors.")),
        ], cols=4, estilo="relleno", tono="base", eyebrow=("Qué hacemos", "What we do")),
        bloque_texto(("Quiénes somos", "Who we are"), [
            ("Somos un grupo de personas a las que nos une el teatro. Ensayamos, escribimos, hacemos el vestuario y el decorado entre todos, y cada estreno es un trabajo de equipo.",
             "We are a group of people brought together by theatre. We rehearse, write, and make the costumes and sets together, and each opening night is teamwork."),
            ("Escribe aquí cuántas personas sois, desde cuándo trabajáis juntas y si sois una compañía profesional o un grupo amateur.",
             "Say here how many of you there are, since when you have worked together and whether you are a professional company or an amateur group."),
        ], imagen_=("photo", ("Actores ensayando una escena en la sala", "Actors rehearsing a scene in the hall")),
            eyebrow=("La compañía", "The company"), tono="relleno", invertir=True),
        bloque_opiniones(("Lo que dice el público", "What audiences say"),
                         [("la función", "the show"), ("el taller", "the workshop"), ("la función en el colegio", "the school show")],
                         tono="base"),
        bloque_cta(("¿Reservamos tu butaca?", "Shall we reserve your seat?"),
                   ("Llámanos o escríbenos por WhatsApp con el espectáculo, el día y cuántas entradas necesitas. Las pagas en taquilla.",
                    "Call us or message us on WhatsApp with the show, the day and how many tickets you need. You pay at the box office."),
                   [llamar(), entradas("ghost")], tono="relleno"),
    ])

    cartel = pagina("cartel", ("cartel", "now-showing"), ("En cartel y entradas", "Now showing and tickets"),
                    ("Espectáculos en cartel, tabla con las próximas funciones y cómo reservar entradas por teléfono o WhatsApp.",
                     "Shows now playing, a table of upcoming performances and how to book tickets by phone or WhatsApp."), [
        hero(("En cartel", "Now showing"),
             ("Nuestros espectáculos de la temporada, con día, hora y precio. Para reservar, llama o escribe por WhatsApp.",
              "Our shows this season, with day, time and price. To book, call or message us on WhatsApp."),
             [entradas(), llamar("secondary")], variante="texto"),
        seccion("features", [
            titular(("Espectáculos de la temporada", "This season's shows"),
                    ("Espectáculos de ejemplo: cámbialos por los vuestros, con su cartel y su sinopsis.",
                     "Example shows: replace them with your own, with their poster and synopsis.")),
            rejilla([
                espectaculo(("Comedia", "Comedy"), ("Obra de ejemplo: comedia", "Example play: comedy"),
                            ("Sinopsis de ejemplo: cámbiala por la de vuestra obra, en dos o tres líneas.",
                             "Example synopsis: replace it with your play's own, in two or three lines."),
                            ("Dura unos 90 minutos. Recomendada para mayores de 12 años.",
                             "Runs for about 90 minutes. Recommended for ages 12 and over."),
                            ("Cartel de una comedia", "Poster for a comedy")),
                espectaculo(("Drama", "Drama"), ("Obra de ejemplo: drama", "Example play: drama"),
                            ("Sinopsis de ejemplo: cámbiala por la de vuestra obra, en dos o tres líneas.",
                             "Example synopsis: replace it with your play's own, in two or three lines."),
                            ("Dura unos 100 minutos, con descanso. Mayores de 14 años.",
                             "Runs for about 100 minutes, with an interval. Ages 14 and over."),
                            ("Cartel de un drama", "Poster for a drama")),
                espectaculo(("Familiar", "Family"), ("Obra de ejemplo: espectáculo familiar", "Example play: family show"),
                            ("Sinopsis de ejemplo: cámbiala por la de vuestra obra, en dos o tres líneas.",
                             "Example synopsis: replace it with your play's own, in two or three lines."),
                            ("Dura unos 55 minutos. Para todos los públicos, desde 4 años.",
                             "Runs for about 55 minutes. For all audiences, from age 4."),
                            ("Cartel de un espectáculo familiar", "Poster for a family show")),
            ], cols=3, tablet=2),
        ], anchor="espectaculos", tono="base"),
        bloque_precios(("Próximas funciones", "Upcoming performances"),
                       ("Próximas funciones de ejemplo", "Example upcoming performances"),
                       [("Día", "Day"), ("Hora", "Time"), ("Espectáculo", "Show"), ("Lugar", "Venue"), ("Entrada", "Ticket")],
                       [(("Viernes", "Friday"), ("20:30", "8:30 pm"), ("Comedia", "Comedy"), ("Sala principal", "Main hall"), eur(12)),
                        (("Sábado", "Saturday"), ("20:30", "8:30 pm"), ("Comedia", "Comedy"), ("Sala principal", "Main hall"), eur(12)),
                        (("Domingo", "Sunday"), ("18:00", "6:00 pm"), ("Espectáculo familiar", "Family show"), ("Sala principal", "Main hall"), eur(8)),
                        (("Viernes de la semana siguiente", "Friday the following week"), ("20:30", "8:30 pm"), ("Drama", "Drama"),
                         ("Sala principal", "Main hall"), eur(12)),
                        (("Sábado de la semana siguiente", "Saturday the following week"), ("20:30", "8:30 pm"), ("Drama", "Drama"),
                         ("Sala principal", "Main hall"), eur(12))],
                       intro=("Las entradas infantiles y las de grupo tienen un precio reducido. Pregúntanos al reservar.",
                              "Children's and group tickets are cheaper. Ask us when you book."),
                       nota=nota_tarifas, tono="relleno", anchor="funciones"),
        bloque_pasos(("Cómo reservar entradas", "How to book tickets"), [
            (("Llama o escribe", "Call or message"),
             ("Por teléfono o por WhatsApp, en horario de atención.",
              "By phone or WhatsApp, during office hours.")),
            (("Dinos función y número", "Tell us the show and number"),
             ("El espectáculo, el día y cuántas entradas necesitas, con un nombre de contacto.",
              "The show, the day and how many tickets you need, with a contact name.")),
            (("Te confirmamos el sitio", "We confirm your seats"),
             ("Te decimos si hay plaza y te guardamos las butacas a tu nombre.",
              "We tell you whether there is room and hold the seats in your name.")),
            (("Pagas en taquilla", "Pay at the box office"),
             ("Una hora antes de la función, en efectivo o con tarjeta. No pedimos pago por adelantado.",
              "One hour before the show, in cash or by card. We do not ask for payment in advance.")),
        ], tono="base", anchor="entradas"),
        bloque_faq(("Preguntas frecuentes", "Frequently asked questions"), [
            (("¿Puedo cambiar o anular la reserva?", "Can I change or cancel my booking?"),
             ("Sí. Avísanos cuanto antes y liberamos las butacas para otras personas.",
              "Yes. Tell us as soon as you can and we release the seats for other people.")),
            (("¿Hay acceso para sillas de ruedas?", "Is there wheelchair access?"),
             ("Indica aquí si la sala es accesible y cuántos sitios hay reservados para sillas de ruedas.",
              "Say here whether the venue is accessible and how many places are set aside for wheelchairs.")),
            (("¿Se puede entrar con la función empezada?", "Can I come in after the show has started?"),
             ("Para no molestar al resto, las puertas se cierran cuando empieza. Te recomendamos llegar diez minutos antes.",
              "So as not to disturb others, the doors close when the show begins. We recommend arriving ten minutes early.")),
            (("¿A partir de qué edad pueden venir los niños?", "From what age can children come?"),
             ("Cada espectáculo indica la edad recomendada. Los espectáculos familiares son para toda la familia.",
              "Each show lists its recommended age. Family shows are for everyone.")),
        ], tono="relleno"),
        bloque_cta(("¿Qué función te apetece?", "Which show would you like to see?"), None, [entradas(), llamar("ghost")], tono="base"),
    ])

    taller = pagina("taller", ("taller", "workshop"), ("Taller de teatro", "Drama workshop"),
                    ("Grupos de teatro por edades con días y cuotas, cómo apuntarte y cómo unirte a la compañía.",
                     "Drama groups by age with days and fees, how to sign up and how to join the company."), [
        hero(("Taller de teatro", "Drama workshop"),
             ("Aprender teatro es perder la vergüenza, ganar voz y pasarlo bien en grupo. Hay grupos por edades y no hace falta experiencia.",
              "Learning theatre means losing shyness, finding your voice and having fun in a group. There are groups for every age and no experience is needed."),
             [boton(("Escribirnos", "Write to us"), a_pagina("contacto", "contacto")), whatsapp("secondary")], variante="texto"),
        bloque_precios(("Grupos y cuotas", "Groups and fees"), ("Grupos del taller de ejemplo", "Example workshop groups"),
                       [("Grupo", "Group"), ("Edades", "Ages"), ("Día y hora", "Day and time"), ("Cuota mensual", "Monthly fee")],
                       [(("Peques", "Little ones"), ("De 6 a 9 años", "Ages 6 to 9"),
                         ("Sábados, 11:00 a 12:30", "Saturdays, 11:00 am to 12:30 pm"), eur(25)),
                        (("Jóvenes", "Young people"), ("De 10 a 14 años", "Ages 10 to 14"),
                         ("Miércoles, 18:00 a 19:30", "Wednesdays, 6:00 pm to 7:30 pm"), eur(28)),
                        (("Adolescentes", "Teenagers"), ("De 15 a 18 años", "Ages 15 to 18"),
                         ("Viernes, 18:00 a 20:00", "Fridays, 6:00 pm to 8:00 pm"), eur(30)),
                        (("Adultos", "Adults"), ("Desde los 19 años", "From age 19"),
                         ("Lunes, 20:00 a 22:00", "Mondays, 8:00 pm to 10:00 pm"), eur(32))],
                       intro=("Cada grupo trabaja juego, voz, cuerpo e improvisación y acaba el curso con una muestra para las familias.",
                              "Each group works on play, voice, body and improvisation and ends the year with a showcase for families."),
                       nota=("Grupos y cuotas de ejemplo: cámbialos por los vuestros. Puedes venir a una clase de prueba; pregúntanos las fechas.",
                             "Example groups and fees: replace them with your own. You can come to a trial class; ask us for the dates."),
                       tono="base", anchor="grupos"),
        bloque_pasos(("Cómo apuntarte", "How to sign up"), [
            (("Elige tu grupo", "Choose your group"), ("Según tu edad y el día que te viene mejor.", "By your age and the day that suits you best.")),
            (("Clase de prueba", "Trial class"), ("Vienes, juegas y ves si te gusta, sin compromiso.", "You come, play and see if you like it, with no commitment.")),
            (("Matrícula", "Enrolment"), ("Rellenas una ficha con tus datos, o los de tu hijo o hija.", "You fill in a form with your details, or your child's.")),
            (("Empiezas el curso", "You start the course"), ("Pagas la cuota mensual y ya eres parte del grupo.", "You pay the monthly fee and you are part of the group.")),
        ], tono="relleno", anchor="apuntarse"),
        bloque_checks(("Cómo unirte a la compañía", "How to join the company"), [
            ("Actuar: si ya tienes experiencia, escríbenos y te proponemos una prueba.", "Acting: if you already have experience, write to us and we will suggest an audition."),
            ("Luces y sonido: si manejas una mesa, te necesitamos.", "Lights and sound: if you can run a desk, we need you."),
            ("Vestuario y decorado: coser, pintar o construir.", "Costumes and sets: sewing, painting or building."),
            ("Producción y taquilla: organizar, vender y recibir al público.", "Production and box office: organising, selling and welcoming the audience."),
        ], intro=("No hace falta ser actor o actriz: hay sitio para cualquiera que quiera ayudar.",
                  "You do not need to be an actor: there is room for anyone who wants to help."),
            tono="base", cols=2, anchor="unirte"),
        bloque_cta(("¿Te animas a probar?", "Will you give it a go?"),
                   ("Escríbenos y te contamos qué grupo te encaja y cuándo hay clase de prueba.",
                    "Write to us and we will tell you which group suits you and when the next trial class is."),
                   [boton(("Escribirnos", "Write to us"), a_pagina("contacto", "contacto")), whatsapp("ghost")], tono="relleno"),
    ])

    cont = pagina("contacto", ("contacto", "contact"), ("Contratación y contacto", "Hire us and contact"),
                  ("Funciones para colegios y eventos, presupuesto sin compromiso, formulario, teléfono, WhatsApp, horario y cómo llegar al teatro.",
                   "Shows for schools and events, a quote with no obligation, a form, phone, WhatsApp, hours and how to get to the theatre."), [
        hero(("Contratación y contacto", "Hire us and contact"),
             ("Llevamos nuestros espectáculos a colegios, fiestas y eventos. Cuéntanos qué necesitas y te preparamos un presupuesto sin compromiso.",
              "We take our shows to schools, festivals and events. Tell us what you need and we will prepare a quote with no obligation."),
             [llamar(), whatsapp()], variante="texto"),
        bloque_valores(("Funciones a medida", "Shows made to measure"), [
            (("Colegios e institutos", "Schools"),
             ("Funciones en horario escolar para cada edad, con un rato de preguntas y respuestas con los actores si lo quieres.",
              "Shows during school hours for each age group, with a question-and-answer session with the actors if you wish.")),
            (("Ayuntamientos y fiestas", "Councils and festivals"),
             ("Espectáculos de calle o de sala para ferias, fiestas patronales y ciclos culturales.",
              "Street or indoor shows for fairs, local festivals and cultural programmes.")),
            (("Empresas y particulares", "Companies and private clients"),
             ("Teatro para celebraciones, aniversarios y encuentros, en vuestro local o en el nuestro.",
              "Theatre for celebrations, anniversaries and gatherings, at your venue or ours.")),
        ], intro=("El presupuesto depende de la obra, el lugar, el número de actores y el montaje. Te lo damos por escrito.",
                  "The quote depends on the play, the venue, the number of actors and the set-up. We give it to you in writing."),
            estilo="borde", tono="base", anchor="contratar"),
        bloque_contacto(("Escríbenos", "Write to us"),
                        ("Para contratar, apuntarte al taller o reservar entradas, llámanos o usa el formulario.",
                         "To hire us, sign up for the workshop or book tickets, call us or use the form."),
                        campos_consulta(mensaje_etiqueta=("Cuéntanos qué necesitas", "Tell us what you need"),
                                        servicios=[("Contratar una función", "Hire a show"), ("Taller de teatro", "Drama workshop"),
                                                   ("Reservar entradas", "Book tickets"), ("Unirme a la compañía", "Join the company"),
                                                   ("Otra consulta", "Something else")]),
                        ("Enviar mensaje", "Send message"), horas,
                        notas=("Para reservar entradas es más rápido llamar o escribir por WhatsApp.",
                               "To book tickets it is quicker to call or message us on WhatsApp."),
                        exito=("Gracias. Te respondemos en cuanto podamos, por correo o por teléfono.",
                               "Thank you. We will reply as soon as we can, by email or by phone."), tono="relleno"),
        bloque_mapa(titulo=("Cómo llegar al teatro", "How to get to the theatre"),
                    texto_=("Indica aquí dónde está la entrada de la sala y dónde se puede aparcar.",
                            "Say here where the entrance to the hall is and where to park."), tono="base"),
    ])

    return plantilla(KEY, ("Compañía de teatro", "Theatre company"),
                     ("Sitio de una compañía de teatro: espectáculos en cartel, próximas funciones, taller con grupos y cuotas, contratación para colegios y eventos, y entradas por teléfono o WhatsApp.",
                      "Website for a theatre company: shows now playing, upcoming performances, a workshop with groups and fees, hiring for schools and events, and tickets by phone or WhatsApp."),
                     "comunidad", ("compania de teatro grupo de teatro teatro amateur teatro aficionado cartelera funciones obras espectaculos taller de teatro clases de teatro contratacion colegios actuaciones entradas grupo cultural",
                                   "theatre company drama group amateur theatre community theatre shows plays performances drama workshop drama classes school shows hire tickets cultural group"),
                     look, cabecera(nav, marco([llamar(), whatsapp("ghost")], dir="row", gap=12, wrap=True)),
                     pie(nav, ("Teatro, taller y funciones a medida. Reserva tus entradas por teléfono o WhatsApp.",
                               "Theatre, workshop and made-to-measure shows. Book your tickets by phone or WhatsApp."),
                         ("Taquilla una hora antes de cada función. Teléfono de lunes a viernes.",
                          "Box office one hour before each show. Phone Monday to Friday.")),
                     [inicio, cartel, taller, cont])
