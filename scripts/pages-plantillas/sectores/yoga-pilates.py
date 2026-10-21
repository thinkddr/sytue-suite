# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
from lib import *
from looks import LOOKS

KEY = "yoga-pilates"


def build():
    nueva(KEY)
    look = dict(LOOKS["salvia"], fuentes=("dm-sans", "figtree"), radio=(8, 14, 20))
    nav = [("inicio", ("Inicio", "Home")), ("clases", ("Clases y horario", "Classes and timetable")),
           ("bonos", ("Bonos y reserva", "Passes and booking")), ("contacto", ("Contacto", "Contact"))]
    horas = [(("Lunes a jueves", "Monday to Thursday"),
              ("9:00 a 13:00 y 17:00 a 21:00", "9:00 am to 1:00 pm and 5:00 pm to 9:00 pm")),
             (("Viernes", "Friday"), ("9:00 a 13:00 y 17:00 a 20:00", "9:00 am to 1:00 pm and 5:00 pm to 8:00 pm")),
             (("Sábado", "Saturday"), ("10:00 a 13:00", "10:00 am to 1:00 pm")),
             (("Domingo", "Sunday"), ("Cerrado", "Closed"))]
    nota = ("Precios de ejemplo: cámbialos por los tuyos. Incluyen IVA.",
            "Example prices: replace them with your own. VAT included.")

    def prueba():
        return boton(("Reservar clase de prueba", "Book a trial class"), a_pagina("contacto", "contacto"))

    inicio = pagina("inicio", ("", ""), ("Yoga y pilates", "Yoga and pilates"),
                    ("Estudio de yoga y pilates en grupos reducidos: horario semanal, bonos y clase de prueba. Reserva tu plaza por WhatsApp.",
                     "Yoga and pilates studio with small groups: weekly timetable, passes and a trial class. Book your place on WhatsApp."), [
        hero(("Yoga y pilates en grupos pequeños", "Yoga and pilates in small groups"),
             ("Clases con pocas personas para que el profesor te vea, te corrija y adapte los ejercicios a ti. Empieza con una clase de prueba.",
              "Classes with few people so the teacher can see you, correct you and adapt the exercises to you. Start with a trial class."),
             [prueba(), whatsapp("secondary")],
             eyebrow=("Estudio de yoga y pilates", "Yoga and pilates studio"),
             imagen_=("photo", ("Sala luminosa con esterillas de yoga en el suelo", "Bright studio with yoga mats on the floor"))),
        bloque_valores(("Qué puedes practicar", "What you can practise"), [
            (("Yoga suave", "Gentle yoga"),
             ("Posturas lentas, respiración y estiramientos. Para empezar o para ir con calma.",
              "Slow postures, breathing and stretching. To begin with, or to take it easy.")),
            (("Yoga dinámico", "Dynamic yoga"),
             ("Posturas encadenadas con más ritmo. Se trabaja la fuerza y el equilibrio.",
              "Postures linked together at a quicker pace, working on strength and balance.")),
            (("Pilates de suelo", "Mat pilates"),
             ("Ejercicios sobre la esterilla para el abdomen, la espalda y la postura.",
              "Exercises on the mat for the abs, back and posture.")),
            (("Pilates con material", "Pilates with props"),
             ("Con gomas, aros y pelotas para ajustar la dificultad a tu nivel.",
              "With bands, rings and balls to set the difficulty to your level.")),
        ], cols=4, eyebrow=("Las clases", "The classes"), tono="relleno", estilo="borde"),
        bloque_texto(("Pocas personas por clase", "Few people per class"), [
            ("Los grupos son de ocho personas como máximo. Así se oye la voz del profesor y hay sitio para cada esterilla.",
             "Groups are capped at eight people, so everyone hears the teacher and each mat has room."),
            ("Si tienes una molestia o una limitación, cuéntanoslo al llegar y te proponemos una variante de cada ejercicio.",
             "If you have a niggle or a limitation, tell us when you arrive and we will suggest a variation of each exercise."),
        ], imagen_=("landscape", ("Grupo pequeño practicando pilates en la sala", "Small group doing pilates in the studio")),
            tono="base"),
        bloque_checks(("Para quién son", "Who they are for"), [
            ("Personas que empiezan y no saben por dónde.", "People who are just starting and do not know where to begin."),
            ("Embarazadas, con el visto bueno de su médico o matrona.", "Pregnant women, with the go-ahead of their doctor or midwife."),
            ("Personas mayores que quieren moverse con seguridad.", "Older people who want to move safely."),
            ("Quien pasa muchas horas sentado y llega con la espalda cargada.", "Anyone who sits for long hours and arrives with a stiff back."),
            ("Quien practica otro deporte y busca algo que lo complemente.", "Anyone who plays another sport and wants something to complement it."),
            ("Personas con una lesión previa, siempre que su médico lo apruebe.", "People with a past injury, as long as their doctor approves."),
        ], intro=("Las clases son ejercicio suave y no sustituyen a un tratamiento médico. Si tienes dudas, pregunta antes a tu médico.",
                  "The classes are gentle exercise and do not replace medical treatment. If you are unsure, ask your doctor first."),
            tono="relleno", cols=2, eyebrow=("Para ti", "For you")),
        bloque_opiniones(("Lo que dicen quienes vienen", "What our students say"),
                         [("las clases", "the classes"), ("el grupo reducido", "the small group"), ("el trato", "the teaching")],
                         tono="base"),
        bloque_cta(("Prueba una clase", "Try a class"),
                   ("Elige una del horario y reserva tu plaza. Si te gusta, te explicamos los bonos.",
                    "Pick one from the timetable and book your place. If you like it, we explain the passes."),
                   [prueba(), whatsapp("ghost")]),
    ])

    clases = pagina("clases", ("clases", "classes"), ("Clases y horario", "Classes and timetable"),
                    ("Horario semanal de yoga y pilates con niveles, qué traer a tu primera clase y cómo reservar plaza.",
                     "Weekly yoga and pilates timetable with levels, what to bring to your first class and how to book a place."), [
        hero(("Clases y horario", "Classes and timetable"),
             ("Cada clase dura 60 minutos y tiene un máximo de ocho plazas. Mira el nivel de cada una y elige la tuya.",
              "Each class lasts 60 minutes with a maximum of eight places. Check the level of each one and pick yours."),
             [prueba()], variante="texto"),
        bloque_precios(("Horario semanal", "Weekly timetable"), ("Horario semanal de clases", "Weekly class timetable"),
                       [("Día", "Day"), ("Hora", "Time"), ("Clase", "Class"), ("Nivel", "Level")],
                       [(("Lunes", "Monday"), ("9:30", "9:30 am"), ("Yoga suave", "Gentle yoga"), ("Todos los niveles", "All levels")),
                        (("Lunes", "Monday"), ("19:00", "7:00 pm"), ("Pilates de suelo", "Mat pilates"), ("Iniciación", "Beginners")),
                        (("Martes", "Tuesday"), ("18:00", "6:00 pm"), ("Yoga dinámico", "Dynamic yoga"), ("Intermedio", "Intermediate")),
                        (("Martes", "Tuesday"), ("20:00", "8:00 pm"), ("Yoga suave", "Gentle yoga"), ("Todos los niveles", "All levels")),
                        (("Miércoles", "Wednesday"), ("9:30", "9:30 am"), ("Pilates con material", "Pilates with props"), ("Todos los niveles", "All levels")),
                        (("Miércoles", "Wednesday"), ("19:00", "7:00 pm"), ("Pilates de suelo", "Mat pilates"), ("Intermedio", "Intermediate")),
                        (("Jueves", "Thursday"), ("18:00", "6:00 pm"), ("Yoga dinámico", "Dynamic yoga"), ("Iniciación", "Beginners")),
                        (("Jueves", "Thursday"), ("20:00", "8:00 pm"), ("Pilates con material", "Pilates with props"), ("Intermedio", "Intermediate")),
                        (("Viernes", "Friday"), ("9:30", "9:30 am"), ("Yoga suave", "Gentle yoga"), ("Todos los niveles", "All levels")),
                        (("Sábado", "Saturday"), ("10:30", "10:30 am"), ("Yoga dinámico", "Dynamic yoga"), ("Todos los niveles", "All levels"))],
                       nota=("En vacaciones el horario puede cambiar: consúltanos antes por WhatsApp.", "The timetable may change on holidays: check with us on WhatsApp first."),
                       kind="pricing", anchor="horario", tono="base"),
        bloque_valores(("Niveles", "Levels"), [
            (("Iniciación", "Beginners"),
             ("Para quien empieza o vuelve tras un tiempo. Vamos despacio y explicamos cada postura.",
              "For anyone starting or coming back after a break. We go slowly and explain every posture.")),
            (("Todos los niveles", "All levels"),
             ("Cada ejercicio tiene una versión más fácil y otra más exigente, y tú eliges.",
              "Every exercise has an easier and a harder version, and you choose.")),
            (("Intermedio", "Intermediate"),
             ("Para quien ya practica y conoce las posturas básicas. Más ritmo y menos explicación.",
              "For those who already practise and know the basic postures. More pace and less explaining.")),
        ], tono="relleno", estilo="relleno", anchor="niveles"),
        bloque_checks(("Qué traer a tu primera clase", "What to bring to your first class"), [
            ("Ropa cómoda y elástica, que te deje moverte.", "Comfortable, stretchy clothes you can move in."),
            ("Calcetines antideslizantes si prefieres, o ir descalzo.", "Non-slip socks if you prefer, or go barefoot."),
            ("Una botella de agua.", "A water bottle."),
            ("Una toalla pequeña, por si sudas.", "A small towel, in case you sweat."),
            ("Tu esterilla, o te dejamos una de la casa.", "Your own mat, or we lend you one of ours."),
            ("Ven diez minutos antes: te presentamos la sala y hablamos de tus molestias.",
             "Arrive ten minutes early: we show you the studio and talk about any aches."),
        ], intro=("No hace falta experiencia ni flexibilidad. Come ligero un par de horas antes.",
                  "No experience or flexibility needed. Eat lightly a couple of hours beforehand."),
            tono="base", cols=2, anchor="traer"),
        bloque_cta(("¿Qué clase te apetece?", "Which class do you fancy?"), None, [prueba(), whatsapp("ghost")]),
    ])

    bonos = pagina("bonos", ("bonos", "passes"), ("Bonos y reserva", "Passes and booking"),
                   ("Clase de prueba, clase suelta, bonos de clases y cuota mensual. Cómo reservar plaza y preguntas frecuentes.",
                    "Trial class, single class, class passes and monthly fee. How to book a place and frequently asked questions."), [
        hero(("Bonos y reserva de plaza", "Passes and booking a place"),
             ("Empieza con una clase de prueba. Después elige entre clases sueltas, un bono o una cuota mensual.",
              "Start with a trial class. Then choose single classes, a pass or a monthly fee."),
             [prueba(), whatsapp("secondary")], variante="texto"),
        bloque_precios(("Bonos y cuotas", "Passes and fees"), ("Precios de bonos y cuotas", "Pass and fee prices"),
                       [("Opción", "Option"), ("Detalle", "Details"), ("Precio", "Price")],
                       [(("Clase de prueba", "Trial class"), ("Una clase, para conocernos", "One class, to get to know us"), eur(10)),
                        (("Clase suelta", "Single class"), ("Una clase, sin compromiso", "One class, no commitment"), eur(14)),
                        (("Bono de 5 clases", "5-class pass"), ("Caduca a los dos meses", "Expires after two months"), eur(60)),
                        (("Bono de 10 clases", "10-class pass"), ("Caduca a los cuatro meses", "Expires after four months"), eur(110)),
                        (("Cuota mensual, una clase por semana", "Monthly, one class a week"), ("Cuatro clases al mes", "Four classes a month"), eur(48)),
                        (("Cuota mensual, dos clases por semana", "Monthly, two classes a week"), ("Ocho clases al mes", "Eight classes a month"), eur(72))],
                       nota=nota, anchor="precios", tono="base"),
        bloque_pasos(("Reservar tu plaza", "Booking your place"), [
            (("Elige la clase", "Pick a class"), ("Mira el horario y quédate con una o dos clases.", "Check the timetable and pick one or two classes.")),
            (("Escríbenos", "Message us"), ("Por WhatsApp, por teléfono o con el formulario.", "On WhatsApp, by phone or with the form.")),
            (("Te confirmamos", "We confirm"), ("Te decimos si hay plaza y te la guardamos.", "We tell you if there is a place and hold it for you.")),
            (("Vienes", "You come"), ("Diez minutos antes de la hora, con ropa cómoda.", "Ten minutes before the start, in comfortable clothes.")),
        ], tono="relleno"),
        bloque_faq(("Preguntas frecuentes", "Frequently asked questions"), [
            (("¿Qué pasa si no puedo ir a una clase?", "What if I cannot make a class?"),
             ("Avísanos al menos doce horas antes y no la pierdes: la pasas a otro día. Con menos aviso, cuenta como hecha.",
              "Let us know at least twelve hours ahead and you keep it: we move it to another day. With less notice, it counts as taken.")),
            (("¿Puedo ir embarazada?", "Can I come if I am pregnant?"),
             ("Sí, con el visto bueno de tu médico o matrona. Avísanos al reservar y te recomendamos las clases más adecuadas.",
              "Yes, with the go-ahead of your doctor or midwife. Tell us when you book and we will recommend the most suitable classes.")),
            (("¿Y si tengo una lesión?", "What if I have an injury?"),
             ("Cuéntanosla antes de empezar. Adaptamos los ejercicios, pero no sustituimos a tu médico ni a tu fisioterapeuta.",
              "Tell us before you start. We adapt the exercises, but we do not replace your doctor or physiotherapist.")),
            (("¿Cómo se paga?", "How do I pay?"),
             ("Con tarjeta, Bizum o en efectivo en la primera clase. Te damos factura si la necesitas.",
              "By card, Bizum or cash at your first class. We give you an invoice if you need one.")),
        ], tono="base"),
        bloque_cta(("¿Guardamos tu plaza?", "Shall we hold your place?"), None, [prueba(), llamar("secondary")]),
    ])

    cont = pagina("contacto", ("contacto", "contact"), ("Contacto y reserva", "Contact and booking"),
                  ("Reserva tu clase de prueba por teléfono, WhatsApp o con el formulario. Horario y cómo llegar al estudio.",
                   "Book your trial class by phone, WhatsApp or with the form. Opening hours and how to find the studio."), [
        hero(("Reserva tu plaza", "Book your place"),
             ("Llámanos, escríbenos por WhatsApp o rellena el formulario con la clase que te interesa. Te confirmamos el mismo día.",
              "Call us, message us on WhatsApp or fill in the form with the class you fancy. We confirm the same day."),
             [llamar(), whatsapp()], variante="texto"),
        bloque_contacto(("Cómo encontrarnos", "How to find us"),
                        ("Si necesitas algún apoyo para llegar o moverte por el estudio, dínoslo al reservar.",
                         "If you need any help getting here or moving around the studio, tell us when you book."),
                        campos_cita(servicios=[("Clase de prueba", "Trial class"), ("Yoga", "Yoga"), ("Pilates", "Pilates"),
                                               ("Bonos y cuotas", "Passes and fees")],
                                    extra=[campo("avisos", "text", ("Embarazo, lesión o molestia (opcional)",
                                                                     "Pregnancy, injury or discomfort (optional)"),
                                                 False, maxlen=200)]),
                        ("Reservar plaza", "Request a place"), horas,
                        exito=("Gracias. Te confirmamos la plaza por teléfono o WhatsApp en cuanto la tengamos.",
                               "Thank you. We will confirm your place by phone or WhatsApp as soon as we have it.")),
        bloque_mapa(tono="relleno"),
    ])

    return plantilla(KEY, ("Yoga y pilates", "Yoga and pilates"),
                     ("Estudio de yoga y pilates con grupos reducidos, horario semanal por niveles, bonos de clases, clase de prueba y botones de llamar y WhatsApp.",
                      "Yoga and pilates studio with small groups, a weekly timetable by level, class passes, a trial class and call and WhatsApp buttons."),
                     "deporte", ("yoga pilates estudio de yoga estudio de pilates clases grupos reducidos bonos clase de prueba embarazadas mayores esterilla hatha vinyasa",
                                 "yoga pilates yoga studio pilates studio classes small groups class pass trial class prenatal seniors mat"),
                     look, cabecera(nav, llamar()),
                     pie(nav, ("Estudio de yoga y pilates en grupos reducidos. Clases por niveles, bonos y clase de prueba.",
                               "Yoga and pilates studio with small groups. Classes by level, passes and a trial class."),
                         ("Lunes a jueves 9:00 a 13:00 y 17:00 a 21:00. Viernes hasta las 20:00.",
                          "Monday to Thursday 9 am to 1 pm and 5 pm to 9 pm. Friday until 8 pm.")),
                     [inicio, clases, bonos, cont])
