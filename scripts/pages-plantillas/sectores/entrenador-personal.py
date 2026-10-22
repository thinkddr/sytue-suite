# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
from lib import *
from looks import LOOKS

KEY = "entrenador-personal"


def build():
    nueva(KEY)
    look = LOOKS["noche"]
    nav = [("inicio", ("Inicio", "Home")), ("entrenamientos", ("Entrenamientos", "Training")),
           ("planes", ("Planes y bonos", "Plans and passes")), ("contacto", ("Contacto", "Contact"))]
    horas = [(("Lunes a viernes", "Monday to Friday"),
              ("7:00 a 14:00 y 16:00 a 21:00, con cita", "7:00 am to 2:00 pm and 4:00 pm to 9:00 pm, by appointment")),
             (("Sábado", "Saturday"), ("9:00 a 13:00, con cita", "9:00 am to 1:00 pm, by appointment")),
             (("Domingo", "Sunday"), ("Cerrado", "Closed"))]
    nota = ("Precios de ejemplo: cámbialos por los tuyos. Incluyen IVA.",
            "Example prices: replace them with your own. VAT included.")

    def valoracion():
        return boton(("Pedir mi primera valoración", "Book your first assessment"), a_pagina("contacto", "contacto"))

    inicio = pagina("inicio", ("", ""), ("Entrenador personal", "Personal trainer"),
                    ("Entrenador personal: valoración inicial, sesiones presenciales, online o en grupo pequeño, planes y bonos. Pide tu valoración por WhatsApp.",
                     "Personal trainer: initial assessment, in-person, online or small-group sessions, plans and passes. Book your assessment on WhatsApp."), [
        hero(("Entrenamiento personal, a tu medida", "Personal training, made for you"),
             ("Empiezo escuchándote: qué quieres, cuánto tiempo tienes y cómo te encuentras. Con eso diseño tus sesiones y te acompaño en cada una.",
              "I start by listening: what you want, how much time you have and how you feel. With that I design your sessions and go through each one with you."),
             [valoracion(), whatsapp("secondary")],
             eyebrow=("Entrenador personal", "Personal trainer"),
             imagen_=("portrait", ("Entrenador personal en la sala de entrenamiento", "Personal trainer in the training space"))),
        bloque_pasos(("Cómo empezamos", "How we start"), [
            (("Valoración", "Assessment"),
             ("Nos vemos una hora. Hablamos de tus objetivos, de tu salud y probamos unos ejercicios sencillos.",
              "We meet for an hour. We talk about your goals and health and try a few simple exercises.")),
            (("Tu plan", "Your plan"),
             ("Te preparo un plan por escrito, con las sesiones de la semana y lo que puedes hacer por tu cuenta.",
              "I write you a plan with the week's sessions and what you can do on your own.")),
            (("Sesiones", "Sessions"),
             ("Entrenamos juntos, yo marco el ritmo y corrijo la técnica.",
              "We train together; I set the pace and correct your technique.")),
            (("Seguimiento", "Follow-up"),
             ("Cada pocas semanas repasamos cómo vas y ajustamos el plan.",
              "Every few weeks we review how you are doing and adjust the plan.")),
        ], tono="relleno"),
        bloque_valores(("Dónde y cómo entrenamos", "Where and how we train"), [
            (("Presencial", "In person"),
             ("Una sesión conmigo, solo para ti, en el lugar que acordemos.", "A session with me, just for you, at the place we agree.")),
            (("Online", "Online"),
             ("Entrenamos por videollamada o con un plan que haces cuando puedes y me cuentas cómo ha ido.",
              "We train over video call, or you follow a plan when you can and tell me how it went.")),
            (("Grupo pequeño", "Small group"),
             ("De dos a cuatro personas. Cada una con su carga, y más barato por persona.",
              "Two to four people, each with their own load, and cheaper per person.")),
        ], tono="base", estilo="borde", eyebrow=("Modalidades", "Formats")),
        bloque_texto(("Quién te entrena", "Who trains you"), [
            ("Trabajo con personas de todas las edades y niveles: quien nunca ha pisado un gimnasio y quien quiere mejorar lo que ya hace.",
             "I work with people of all ages and levels: those who have never set foot in a gym and those who want to improve what they already do."),
            ("No te voy a dar una rutina de internet. Cada plan parte de tu situación, y si algo no te funciona, lo cambiamos.",
             "I will not hand you a routine from the internet. Each plan starts from your situation, and if something does not work for you, we change it."),
        ], imagen_=("photo", ("Entrenador explicando un ejercicio con una mancuerna", "Trainer explaining an exercise with a dumbbell")),
            tono="relleno", invertir=True),
        bloque_checks(("Para quién es", "Who it is for"), [
            ("Quien empieza y no quiere hacerlo solo.", "Anyone starting out who does not want to do it alone."),
            ("Quien vuelve a entrenar tras un parón o una baja.", "Anyone returning to training after a break or time off."),
            ("Quien ya entrena y se ha estancado.", "Anyone who already trains and has hit a plateau."),
            ("Quien tiene poco tiempo y necesita aprovecharlo.", "Anyone short of time who needs to make the most of it."),
            ("Quien prepara una carrera, una prueba o una temporada.", "Anyone preparing for a race, a test or a season."),
        ], intro=("No prometo resultados concretos: cada cuerpo responde a su ritmo. Si tienes una lesión o una enfermedad, antes habla con tu médico.",
                  "I do not promise specific results: every body responds at its own pace. If you have an injury or an illness, talk to your doctor first."),
            tono="base", cols=2),
        bloque_opiniones(("Lo que dicen quienes entrenan conmigo", "What people who train with me say"),
                         [("las sesiones", "the sessions"), ("el seguimiento", "the follow-up"), ("el trato", "the coaching")]),
        bloque_cta(("Empecemos con una valoración", "Let us start with an assessment"),
                   ("Cuéntame qué buscas y quedamos. La primera valoración no te compromete a nada más.",
                    "Tell me what you are after and we will meet. The first assessment does not commit you to anything else."),
                   [valoracion(), llamar("secondary")], tono="oscuro"),
    ])

    entren = pagina("entrenamientos", ("entrenamientos", "training"), ("Entrenamientos", "Training"),
                    ("Primera valoración, cómo es una sesión, entrenamiento presencial, online o en grupo pequeño, y seguimiento.",
                     "First assessment, what a session is like, in-person, online or small-group training, and follow-up."), [
        hero(("Entrenamientos", "Training"),
             ("Así trabajo desde el primer día: una valoración, un plan y sesiones en las que sabes qué haces y por qué.",
              "This is how I work from day one: an assessment, a plan and sessions in which you know what you are doing and why."),
             [valoracion()], variante="texto"),
        bloque_checks(("Tu primera valoración", "Your first assessment"), [
            ("Una charla sobre tus objetivos y tu día a día.", "A chat about your goals and your daily routine."),
            ("Preguntas sobre lesiones, operaciones y medicación.", "Questions about injuries, operations and medication."),
            ("Pruebas sencillas de movilidad, fuerza y resistencia.", "Simple tests of mobility, strength and endurance."),
            ("Un primer ejercicio para que notes cómo entreno.", "A first exercise so you get a feel for how I train."),
            ("Un plan inicial por escrito, con el siguiente paso.", "A written initial plan, with the next step."),
        ], intro=("Dura una hora. Es tu momento para preguntar todo lo que quieras.",
                  "It lasts an hour. It is your time to ask anything you like."),
            tono="base", cols=2, anchor="valoracion"),
        bloque_pasos(("Cómo es una sesión", "What a session is like"), [
            (("Calentamiento", "Warm-up"), ("Diez minutos para soltar articulaciones y activar el cuerpo.", "Ten minutes to loosen the joints and get the body going.")),
            (("Parte principal", "Main part"), ("Ejercicios de fuerza, resistencia o movilidad, según el plan.", "Strength, endurance or mobility exercises, according to the plan.")),
            (("Vuelta a la calma", "Cool-down"), ("Estiramientos suaves y respiración.", "Gentle stretching and breathing.")),
            (("Cierre", "Wrap-up"), ("Apunto qué has hecho y cómo te has sentido para la siguiente.", "I note what you did and how you felt, ready for the next one.")),
        ], intro=("Una sesión dura entre 45 y 60 minutos.", "A session lasts between 45 and 60 minutes."), tono="relleno"),
        bloque_valores(("Presencial, online o en grupo", "In person, online or in a group"), [
            (("Presencial", "In person"),
             ("Corrijo cada movimiento sobre la marcha. Ideal si empiezas o si tienes poco tiempo.",
              "I correct every movement as you go. Ideal if you are starting out or short of time.")),
            (("Online", "Online"),
             ("Con videollamada o con un plan semanal. Solo necesitas un hueco en casa y algo de material básico.",
              "By video call or with a weekly plan. All you need is a space at home and some basic equipment.")),
            (("Grupo pequeño", "Small group"),
             ("De dos a cuatro personas que entrenan juntas, con carga y ejercicios ajustados a cada una.",
              "Two to four people training together, with loads and exercises adjusted to each person.")),
        ], tono="base", estilo="relleno", anchor="modalidades"),
        bloque_valores(("Seguimiento", "Follow-up"), [
            (("Cada semana", "Every week"), ("Me cuentas cómo has entrenado y cómo has dormido y comido, si quieres.", "You tell me how you trained and, if you like, how you slept and ate.")),
            (("Cada mes", "Every month"), ("Repetimos las pruebas de la valoración y comparamos.", "We repeat the assessment tests and compare.")),
            (("Siempre", "Always"), ("Me escribes por WhatsApp si tienes una duda entre sesiones.", "You message me on WhatsApp if you have a question between sessions.")),
        ], intro=("Para que veas cómo avanzas con tus propios datos, no con los míos.",
                  "So you see your progress from your own numbers, not mine."),
            tono="relleno", estilo="borde"),
        bloque_faq(("Preguntas frecuentes", "Frequently asked questions"), [
            (("¿Me garantizas resultados?", "Do you guarantee results?"),
             ("No. Te acompaño, ajusto el plan y te doy herramientas, pero cada cuerpo responde a su ritmo. Lo que sí garantizo es que no entrenarás sin saber qué haces.",
              "No. I coach you, adjust the plan and give you tools, but every body responds at its own pace. What I do promise is that you will never train without knowing what you are doing.")),
            (("¿Y si tengo una lesión o una enfermedad?", "What if I have an injury or an illness?"),
             ("Pídele antes el visto bueno a tu médico o a tu fisioterapeuta. Yo adapto los ejercicios, pero no hago diagnósticos ni sustituyo un tratamiento.",
              "Get the go-ahead from your doctor or physiotherapist first. I adapt the exercises, but I do not diagnose or replace treatment.")),
            (("¿Necesito material en casa?", "Do I need equipment at home?"),
             ("Para empezar, no. Unas gomas y un par de mancuernas ayudan, y si hace falta te digo cuáles.",
              "Not to start with. A few resistance bands and a pair of dumbbells help, and if needed I tell you which.")),
            (("¿Y si no puedo ir a una sesión?", "What if I cannot make a session?"),
             ("Avísame con 24 horas y la cambiamos de día. Con menos aviso, cuenta como hecha.",
              "Let me know 24 hours ahead and we move it to another day. With less notice, it counts as taken.")),
        ], tono="base"),
        bloque_cta(("¿Hablamos?", "Shall we talk?"), None, [valoracion(), whatsapp("ghost")], tono="relleno"),
    ])

    planes = pagina("planes", ("planes", "plans"), ("Planes y bonos", "Plans and passes"),
                    ("Precios de la valoración, sesiones sueltas, bonos, entrenamiento online y grupo pequeño. Pide tu primera valoración.",
                     "Prices for the assessment, single sessions, passes, online training and small groups. Book your first assessment."), [
        hero(("Planes y bonos", "Plans and passes"),
             ("Pagas por sesión o con un bono, sin cuotas de alta ni permanencias. Si algo lleva más trabajo, te lo digo antes.",
              "You pay per session or with a pass, with no joining fees or minimum terms. If something takes more work, I tell you beforehand."),
             [valoracion()], variante="texto"),
        bloque_precios(("Tarifas", "Rates"), ("Tarifas de entrenamiento personal", "Personal training rates"),
                       [("Servicio", "Service"), ("Detalle", "Details"), ("Precio", "Price")],
                       [(("Primera valoración", "First assessment"), ("Una hora, con plan inicial", "One hour, with an initial plan"), eur(30)),
                        (("Sesión presencial suelta", "Single in-person session"), ("45 a 60 minutos", "45 to 60 minutes"), eur(45)),
                        (("Bono de 5 sesiones presenciales", "5 in-person session pass"), ("Caduca a los dos meses", "Expires after two months"), eur(210)),
                        (("Bono de 10 sesiones presenciales", "10 in-person session pass"), ("Caduca a los cuatro meses", "Expires after four months"), eur(400)),
                        (("Entrenamiento online", "Online training"), ("Plan semanal y seguimiento, al mes", "Weekly plan and follow-up, per month"), eur(60)),
                        (("Grupo pequeño", "Small group"), ("De dos a cuatro personas, por persona y sesión", "Two to four people, per person per session"), eur(20))],
                       nota=nota, anchor="tarifas", tono="base"),
        bloque_checks(("Qué incluye todo plan", "What every plan includes"), [
            ("Un plan escrito y adaptado a ti.", "A written plan tailored to you."),
            ("Corrección de la técnica en cada sesión.", "Technique correction in every session."),
            ("Mensajes por WhatsApp entre sesiones.", "WhatsApp messages between sessions."),
            ("Revisión del plan cada pocas semanas.", "A review of the plan every few weeks."),
        ], tono="relleno", cols=2, anchor="incluye"),
        bloque_pasos(("Cómo empezar", "How to start"), [
            (("Me escribes", "You message me"), ("Por WhatsApp, por teléfono o con el formulario.", "On WhatsApp, by phone or with the form.")),
            (("Quedamos", "We meet"), ("Fijamos día y hora para tu valoración.", "We set a day and time for your assessment.")),
            (("Eliges", "You choose"), ("Con el plan en la mano, decides tarifa y modalidad.", "With the plan in hand, you decide on a rate and format.")),
        ], tono="base"),
        bloque_cta(("¿Qué plan te encaja?", "Which plan suits you?"),
                   ("Si dudas, empieza con la valoración y lo decidimos juntos.",
                    "If you are unsure, start with the assessment and we decide together."),
                   [valoracion(), llamar("secondary")], tono="oscuro"),
    ])

    cont = pagina("contacto", ("contacto", "contact"), ("Contacto y valoración", "Contact and assessment"),
                  ("Pide tu primera valoración por teléfono, WhatsApp o con el formulario. Horario y cómo llegar.",
                   "Book your first assessment by phone, WhatsApp or with the form. Opening hours and how to find me."), [
        hero(("Pide tu primera valoración", "Book your first assessment"),
             ("Llámame, escríbeme por WhatsApp o rellena el formulario. Te contesto el mismo día.",
              "Call me, message me on WhatsApp or fill in the form. I reply the same day."),
             [llamar(), whatsapp()], variante="texto"),
        bloque_contacto(("Dónde entreno", "Where I train"),
                        ("Entreno en la dirección que ves aquí o donde acordemos para las sesiones al aire libre o a domicilio.",
                         "I train at the address shown here, or wherever we agree for outdoor or home sessions."),
                        campos_cita(servicios=[("Primera valoración", "First assessment"), ("Sesiones presenciales", "In-person sessions"),
                                               ("Entrenamiento online", "Online training"), ("Grupo pequeño", "Small group")],
                                    extra=[campo("objetivo", "text", ("¿Qué te gustaría conseguir?", "What would you like to achieve?"),
                                                 False, maxlen=200),
                                           campo("avisos", "text", ("Lesión o enfermedad que deba saber (opcional)",
                                                                    "Injury or illness I should know about (optional)"),
                                                 False, maxlen=200)]),
                        ("Pedir valoración", "Request assessment"), horas,
                        exito=("Gracias. Te confirmo la valoración por teléfono o WhatsApp en cuanto tenga hueco.",
                               "Thank you. I will confirm your assessment by phone or WhatsApp as soon as I have a slot.")),
        bloque_mapa(tono="relleno"),
    ])

    return plantilla(KEY, ("Entrenador personal", "Personal trainer"),
                     ("Entrenador personal independiente con primera valoración, sesiones presenciales, online o en grupo pequeño, planes y bonos, y botones de llamar y WhatsApp.",
                      "Independent personal trainer with a first assessment, in-person, online or small-group sessions, plans and passes, and call and WhatsApp buttons."),
                     "deporte", ("entrenador personal entrenador personal trainer entrenamiento personalizado sesiones bonos valoracion online grupo pequeno fuerza fitness coach",
                                 "personal trainer personal training coach fitness strength online training small group sessions passes assessment"),
                     look, cabecera(nav, llamar()),
                     pie(nav, ("Entrenamiento personal presencial, online o en grupo pequeño, con valoración inicial y seguimiento.",
                               "Personal training in person, online or in a small group, with an initial assessment and follow-up."),
                         ("Lunes a viernes 7:00 a 14:00 y 16:00 a 21:00, con cita. Sábado 9:00 a 13:00.",
                          "Monday to Friday 7 am to 2 pm and 4 pm to 9 pm, by appointment. Saturday 9 am to 1 pm.")),
                     [inicio, entren, planes, cont])
