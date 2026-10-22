# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
from lib import *
from looks import LOOKS

KEY = "escuela-musica"


def S(x):
    return (x, x)


def build():
    nueva(KEY)
    look = LOOKS["vino"]
    nav = [("inicio", ("Inicio", "Home")), ("clases", ("Clases y cuotas", "Classes and fees")),
           ("escuela", ("La escuela", "The school")), ("contacto", ("Contacto", "Contact"))]
    horas = [(("Lunes a viernes", "Monday to Friday"),
              ("16:00 a 21:30", "4:00 pm to 9:30 pm")),
             (("Sábado", "Saturday"), ("10:00 a 14:00", "10:00 am to 2:00 pm")),
             (("Domingo", "Sunday"), ("Cerrado", "Closed"))]
    nota = ("Precios de ejemplo: cámbialos por los tuyos.", "Example prices: replace them with your own.")

    def prueba(var="primary"):
        return boton(("Reservar clase de prueba", "Book a trial lesson"), a_pagina("contacto", "contacto"), var)

    inicio = pagina("inicio", ("", ""), ("Escuela de música", "Music school"),
                    ("Escuela de música con piano, guitarra, violín, batería y canto para todas las edades. Clase de prueba, audiciones y cuotas claras.",
                     "Music school with piano, guitar, violin, drums and singing for all ages. Trial lesson, recitals and clear fees."), [
        hero(("Aprende música tocando", "Learn music by playing"),
             ("Clases individuales y en grupo de piano, guitarra, violín, batería y canto. Para niños que empiezan y para adultos que siempre quisieron.",
              "One-to-one and group lessons in piano, guitar, violin, drums and singing. For children starting out and adults who always wanted to."),
             [prueba(), llamar("secondary"), whatsapp("ghost")],
             eyebrow=("Escuela de música", "Music school"),
             imagen_=("photo", ("Alumna tocando el piano con su profesor", "Student playing the piano with her teacher")),
             tono="oscuro"),
        bloque_valores(("Qué puedes aprender", "What you can learn"), [
            (("Piano", "Piano"),
             ("Desde las primeras escalas hasta tus piezas favoritas, con partitura o de oído.",
              "From first scales to your favourite pieces, from sheet music or by ear.")),
            (("Guitarra", "Guitar"),
             ("Acústica, clásica o eléctrica. Acordes, ritmos y canciones desde el primer mes.",
              "Acoustic, classical or electric. Chords, rhythms and songs from the first month.")),
            (("Violín", "Violin"),
             ("Postura, arco y afinación con paciencia, que es lo que más cuesta al principio.",
              "Posture, bow and tuning, with patience: the hardest part at the start.")),
            (("Batería", "Drums"),
             ("Coordinación y ritmo en una sala preparada, sin molestar a nadie.",
              "Coordination and rhythm in a prepared room that does not disturb anyone.")),
            (("Canto", "Singing"),
             ("Respiración, voz y afinación, para cantar solo o en grupo.",
              "Breathing, voice and pitch, for singing alone or in a group.")),
            (("Lenguaje musical", "Music theory"),
             ("Leer y escribir música: notas, ritmo y compases, en grupo y por edades.",
              "Reading and writing music: notes, rhythm and time signatures, in groups and by age.")),
        ], eyebrow=("Instrumentos", "Instruments"), estilo="borde", tono="base", anchor="instrumentos"),
        bloque_texto(("Individual o en grupo", "One-to-one or group"), [
            ("En la clase individual, el profesor trabaja contigo y con tu ritmo: es lo más rápido para avanzar.",
             "In a one-to-one lesson the teacher works with you at your pace: it is the quickest way to progress."),
            ("En grupo se aprende con otros, se toca junto y sale más barato. Un buen punto de partida para niños y para el lenguaje musical.",
             "In a group you learn with others, play together and it costs less. A good starting point for children and for music theory."),
        ], imagen_=("landscape", ("Aula de música con guitarras y un piano", "Music room with guitars and a piano")),
            tono="relleno", invertir=True),
        bloque_pasos(("Tu primera clase", "Your first lesson"), [
            (("Pruebas", "You try"),
             ("Una clase de prueba con el instrumento que te gusta. No necesitas llevar el tuyo.",
              "A trial lesson with the instrument you like. You do not need to bring your own.")),
            (("Te decimos por dónde empezar", "We tell you where to start"),
             ("Con lo que has visto, te proponemos clase individual o en grupo, y un horario fijo cada semana.",
              "From what we have seen, we suggest a one-to-one or group lesson, and a fixed time each week.")),
            (("Aprendes y tocas", "You learn and play"),
             ("Cada semana, una pieza o una canción nueva que puedes practicar en casa.",
              "Each week, a new piece or song you can practise at home.")),
        ], tono="base"),
        bloque_opiniones(("Lo que dicen alumnos y familias", "What students and families say"),
                         [("las clases", "the lessons"), ("el profesorado", "the teachers"), ("las audiciones", "the recitals")],
                         tono="relleno"),
        bloque_cta(("¿Qué instrumento te llama?", "Which instrument calls you?"),
                   ("Reserva una clase de prueba y toca antes de decidir.", "Book a trial lesson and play before you decide."),
                   [prueba(), llamar("ghost")], tono="base"),
    ])

    clases = pagina("clases", ("clases", "classes"), ("Clases y cuotas", "Classes and fees"),
                    ("Edades, clases individuales y en grupo, cuotas mensuales de ejemplo y consejos para alquilar o comprar un instrumento.",
                     "Ages, one-to-one and group lessons, example monthly fees and advice on renting or buying an instrument."), [
        hero(("Clases y cuotas", "Classes and fees"),
             ("Para todas las edades. Elige el instrumento, el formato y la hora, y paga una cuota fija al mes.",
              "For all ages. Choose the instrument, the format and the time, and pay one fixed monthly fee."),
             [prueba()], variante="texto"),
        bloque_valores(("Por edades", "By age"), [
            (("Iniciación (4 a 7 años)", "Introduction (ages 4 to 7)"),
             ("Música y movimiento en grupo: ritmo, canciones y primeros instrumentos de aula. Sin presión.",
              "Music and movement in a group: rhythm, songs and first classroom instruments. No pressure.")),
            (("Niños y jóvenes (8 a 17 años)", "Children and teenagers (ages 8 to 17)"),
             ("Instrumento y lenguaje musical juntos. Con audiciones para tocar ante la familia.",
              "Instrument and music theory together. With recitals to play for the family.")),
            (("Adultos", "Adults"),
             ("Sin límite de edad ni de nivel. Una hora a la semana, a tu ritmo y sin exámenes.",
              "No age or level limit. One hour a week, at your pace and with no exams.")),
        ], estilo="relleno", tono="relleno", anchor="edades"),
        bloque_precios(("Cuotas de ejemplo", "Example fees"), ("Cuotas mensuales de ejemplo", "Example monthly fees"),
                       [("Clase", "Lesson"), ("Duración", "Length"), ("Cuota al mes", "Fee per month")],
                       [(("Instrumento, individual", "Instrument, one-to-one"), ("30 min a la semana", "30 min a week"), eur(60)),
                        (("Instrumento, individual", "Instrument, one-to-one"), ("45 min a la semana", "45 min a week"), eur(85)),
                        (("Instrumento, individual", "Instrument, one-to-one"), ("60 min a la semana", "60 min a week"), eur(105)),
                        (("Instrumento, en grupo", "Instrument, group"), ("60 min a la semana", "60 min a week"), eur(45)),
                        (("Canto, individual", "Singing, one-to-one"), ("45 min a la semana", "45 min a week"), eur(85)),
                        (("Lenguaje musical, en grupo", "Music theory, group"), ("60 min a la semana", "60 min a week"), eur(30)),
                        (("Iniciación musical (4 a 7 años)", "Introduction to music (ages 4 to 7)"), ("45 min a la semana", "45 min a week"), eur(32)),
                        (("Matrícula (una vez al año)", "Registration (once a year)"), S("—"), eur(25))],
                       nota=("Precios de ejemplo: cámbialos por los tuyos. Indica aquí si hay descuento para hermanos o por dos instrumentos.",
                             "Example prices: replace them with your own. Say here whether there is a discount for siblings or for two instruments."),
                       anchor="cuotas", tono="base"),
        bloque_texto(("¿Alquilar o comprar un instrumento?", "Rent or buy an instrument?"), [
            ("Si empiezas, no compres todavía. Alquila o pide prestado uno durante los primeros meses y mira si te engancha.",
             "If you are starting out, do not buy yet. Rent or borrow one for the first months and see if it hooks you."),
            ("Cuando decidas comprar, pregunta antes a tu profesor: te dirá qué tamaño, qué tipo y qué gama necesitas. Un instrumento muy barato y mal ajustado desanima.",
             "When you decide to buy, ask your teacher first: they will tell you the size, type and range you need. A very cheap, badly set-up instrument is discouraging."),
            ("El piano se puede sustituir al principio por un teclado con teclas pesadas, y la batería por un pad de estudio, si en casa no hay espacio.",
             "At first a piano can be replaced by a weighted-key keyboard, and a drum kit by a practice pad, if there is no space at home."),
        ], eyebrow=("Consejo", "Advice"), tono="relleno", anchor="instrumento"),
        bloque_faq(("Preguntas frecuentes", "Frequently asked questions"), [
            (("¿Hace falta saber música?", "Do I need to know music?"),
             ("No. Empezamos desde cero y a tu ritmo, a cualquier edad.",
              "No. We start from scratch and at your pace, at any age.")),
            (("¿Cuánto hay que practicar?", "How much should I practise?"),
             ("Con diez o quince minutos al día se nota mucho. Es mejor un poco cada día que mucho un solo día.",
              "Ten or fifteen minutes a day makes a big difference. A little each day is better than a lot on a single day.")),
            (("¿Y si no puedo ir un día?", "What if I cannot come one day?"),
             ("Si avisas con 24 horas, recuperas la clase otro día de la semana. Si no, se pierde.",
              "If you tell us 24 hours ahead, you can make it up another day that week. Otherwise it is lost.")),
            (("¿Puedo cambiar de instrumento?", "Can I change instrument?"),
             ("Sí, durante el primer trimestre. Mucha gente descubre que le gusta otro.",
              "Yes, during the first term. Many people find they prefer a different one.")),
        ], tono="base"),
        bloque_cta(("¿Empezamos con una clase de prueba?", "Shall we start with a trial lesson?"), None,
                   [prueba(), whatsapp("ghost")], tono="relleno"),
    ])

    escuela = pagina("escuela", ("la-escuela", "the-school"), ("La escuela", "The school"),
                     ("Profesorado, audiciones durante el curso y cómo es el día a día de la escuela de música.",
                      "Teachers, recitals during the year and what day-to-day life at the music school is like."), [
        hero(("La escuela", "The school"),
             ("Un sitio pequeño, con profesores que conocen a cada alumno por su nombre y por cómo toca.",
              "A small place, with teachers who know each student by name and by how they play."),
             [prueba(), llamar("secondary")], variante="texto"),
        bloque_equipo(("Profesorado", "Teachers"), [
            (("Profesor de piano", "Piano teacher"), ("Presenta aquí en una frase cómo da clase.", "Introduce here, in one sentence, how they teach.")),
            (("Profesora de guitarra", "Guitar teacher"), ("Presenta aquí en una frase cómo da clase.", "Introduce here, in one sentence, how they teach.")),
            (("Profesora de canto", "Singing teacher"), ("Presenta aquí en una frase cómo da clase.", "Introduce here, in one sentence, how they teach.")),
        ], intro=("Cada profesor trabaja con su instrumento y con tus gustos: clásica, pop, rock, jazz, lo que toque.",
                  "Each teacher works with their instrument and your taste: classical, pop, rock, jazz, whatever suits."),
            tono="base"),
        bloque_pasos(("Audiciones", "Recitals"), [
            (("Preparamos la pieza", "We prepare the piece"),
             ("Elegimos una pieza a tu medida y la ensayas con tu profesor varias semanas.",
              "We choose a piece to suit you and you rehearse it with your teacher for several weeks.")),
            (("Tocas ante tu gente", "You play for your people"),
             ("Una audición es una tarde informal en la escuela, con familia y amigos.",
              "A recital is an informal afternoon at the school, with family and friends.")),
            (("Aprendes de ello", "You learn from it"),
             ("Después hablamos de cómo ha ido. Nadie suspende: es una forma de perder el miedo a tocar delante de otros.",
              "Afterwards we talk about how it went. Nobody fails: it is a way of losing the fear of playing in front of others.")),
        ], intro=("Dos o tres veces durante el curso. Son voluntarias y se anuncian con tiempo.",
                  "Two or three times during the year. They are voluntary and announced in advance."),
            tono="relleno", anchor="audiciones"),
        bloque_checks(("Cómo es la escuela", "What the school is like"), [
            ("Aulas insonorizadas, con piano y amplificador.", "Soundproofed rooms with a piano and an amplifier."),
            ("Un horario fijo cada semana, siempre con el mismo profesor.", "A fixed time each week, always with the same teacher."),
            ("Sala de estudio para practicar antes o después de la clase.", "A study room to practise before or after your lesson."),
            ("Grupos por edad y por nivel.", "Groups by age and level."),
        ], intro=("Cámbialo por lo que tenga tu escuela. Aquí solo van cosas que de verdad ofrezcas.",
                  "Replace this with what your school has. Only put things you actually offer."),
            tono="base", cols=2),
        bloque_cta(("Ven a conocernos", "Come and meet us"), None, [prueba(), whatsapp("ghost")], tono="relleno"),
    ])

    cont = pagina("contacto", ("contacto", "contact"), ("Contacto y clase de prueba", "Contact and trial lesson"),
                  ("Reserva tu clase de prueba con el formulario, por teléfono o WhatsApp. Horario y cómo llegar a la escuela de música.",
                   "Book your trial lesson with the form, by phone or WhatsApp. Opening hours and how to find the music school."), [
        hero(("Reserva tu clase de prueba", "Book your trial lesson"),
             ("Dinos qué instrumento te interesa y para quién es. Te llamamos para fijar la hora.",
              "Tell us which instrument interests you and who it is for. We will call you to set a time."),
             [llamar(), whatsapp()], variante="texto"),
        bloque_contacto(("Cómo encontrarnos", "How to find us"),
                        ("Si es tu primera visita, ven con tiempo y te enseñamos las aulas.",
                         "If it is your first visit, come early and we will show you the rooms."),
                        campos_cita(servicios=[("Piano", "Piano"), ("Guitarra", "Guitar"), ("Violín", "Violin"),
                                               ("Batería", "Drums"), ("Canto", "Singing"),
                                               ("Lenguaje musical", "Music theory"), ("Iniciación musical", "Introduction to music")],
                                    extra=[campo("edad", "select", ("Para quién es", "Who it is for"), False,
                                                 opciones=[("Para mí, soy adulto", "For me, I am an adult"),
                                                           ("Para un niño de 4 a 7 años", "For a child aged 4 to 7"),
                                                           ("Para un niño o joven de 8 a 17 años", "For a child or teenager aged 8 to 17")])]),
                        ("Pedir clase de prueba", "Request trial lesson"), horas,
                        exito=("Gracias. Te confirmamos la clase por teléfono o WhatsApp en cuanto la tengamos.",
                               "Thank you. We will confirm your lesson by phone or WhatsApp as soon as we have it.")),
        bloque_mapa(tono="relleno"),
    ])

    return plantilla(KEY, ("Escuela de música", "Music school"),
                     ("Escuela de música con instrumentos, clases individuales y en grupo, cuotas, audiciones, profesorado de relleno y clase de prueba.",
                      "Music school with instruments, one-to-one and group lessons, fees, recitals, placeholder teachers and a trial lesson."),
                     "educacion", ("escuela de musica academia de musica conservatorio clases de piano guitarra violin bateria canto lenguaje musical solfeo iniciacion musical audiciones",
                                   "music school music academy piano lessons guitar violin drums singing music theory recital music lessons"),
                     look, cabecera(nav, marco([llamar(), whatsapp("ghost")], dir="row", gap=12, wrap=True)),
                     pie(nav, ("Escuela de música. Piano, guitarra, violín, batería y canto, para todas las edades.",
                               "Music school. Piano, guitar, violin, drums and singing, for all ages."),
                         ("Lunes a viernes 16:00 a 21:30. Sábado 10:00 a 14:00.",
                          "Monday to Friday 4 pm to 9:30 pm. Saturday 10 am to 2 pm.")),
                     [inicio, clases, escuela, cont])
