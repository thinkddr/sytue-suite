# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
from lib import *
from looks import LOOKS

KEY = "logopeda"

# Paleta propia: violeta vivo sobre crema cálida (pensada para una consulta con niños).
VIOLETA = dict(background="#fffaf3", surface="#f6ecde", text="#241a33", muted="#5b4d6e", primary="#7a3fe0",
               on_primary="#ffffff", border="#eadcc8", inverse="#2a1a4f", on_inverse="#f6f0ff")

DATOS = ("Tus datos y los de tu hijo o hija, y lo que se trabaja en las sesiones, se tratan con confidencialidad y según la normativa de protección de datos.",
         "Your details, your child's, and what is worked on in the sessions are handled in confidence and in line with data protection rules.")

NOTA = ("Precios de ejemplo: cámbialos por los tuyos. Pregunta también por bonos de sesiones.",
        "Example prices: replace them with your own. Ask also about session packs.")


def _junta(*ts):
    return (" ".join(t[0] for t in ts), " ".join(t[1] for t in ts))


def _equipo():
    fichas = [(("Logopeda, niños y familias", "Speech and language therapist, children and families"),
               ("Hace la valoración, explica los resultados a la familia y coordina con el colegio.",
                "Carries out the assessment, explains the results to the family and liaises with the school.")),
              (("Logopeda, voz y adultos", "Speech and language therapist, voice and adults"),
               ("Trabaja la voz y el habla en adultos, y se coordina con tu médico cuando hace falta.",
                "Works on voice and speech in adults, and liaises with your doctor when needed."))]
    li = [marco([imagen("portrait", ("Foto de la persona del equipo", "Photo of the team member"), ratio=1.0),
                 h3(("Nombre Apellido", "First Last")), peq(rol, color="$muted"), p(frase),
                 peq(COLEGIADO, color="$muted")], tag="li", gap=10) for rol, frase in fichas]
    return seccion("team", [titular(("Quién te atiende", "Who looks after you")), rejilla(li, cols=2)],
                   tono="relleno", anchor="equipo")


def build():
    nueva(KEY)
    look = dict(LOOKS["lavanda"], colores=VIOLETA, fuentes=("manrope", "dm-sans"), radio=(10, 16, 24))
    nav = [("inicio", ("Inicio", "Home")), ("trabajamos", ("Qué trabajamos", "What we work on")),
           ("sesiones", ("Cómo trabajamos", "How we work")), ("contacto", ("Contacto", "Contact"))]
    horas = [(("Lunes a jueves", "Monday to Thursday"),
              ("9:30 a 13:30 y 16:00 a 20:00", "9:30 am to 1:30 pm and 4:00 pm to 8:00 pm")),
             (("Viernes", "Friday"), ("9:30 a 14:00", "9:30 am to 2:00 pm")),
             (("Sábado y domingo", "Saturday and Sunday"), ("Cerrado", "Closed"))]
    cita = lambda v="primary": boton(("Pedir cita", "Book an appointment"), a_pagina("contacto", "contacto"), v)

    inicio = pagina("inicio", ("", ""), ("Logopedia", "Speech and language therapy"),
                    ("Consulta de logopedia para niños y adultos: lenguaje, pronunciación, fluidez, voz y apoyo en lectura y escritura. Valoración inicial, sesiones y trabajo con familias y colegios.",
                     "Speech and language therapy for children and adults: language, pronunciation, fluency, voice and support with reading and writing. Initial assessment, sessions and work with families and schools."), [
        hero(("Ayudamos a hablar, a entender y a leer con más soltura", "We help people talk, understand and read with more ease"),
             ("Consulta de logopedia para niños y adultos. Empezamos con una valoración en la que te explicamos qué ocurre y cómo vamos a trabajar, sin tecnicismos.",
              "Speech and language therapy for children and adults. We start with an assessment where we explain what is going on and how we will work, without jargon."),
             [llamar(), whatsapp()], variante="centro", eyebrow=("Logopedia", "Speech and language therapy")),
        seccion("features", [
            titular(("¿Para quién es?", "Who is it for?")),
            rejilla([
                tarjeta([h3(("Niños y niñas", "Children")),
                         p(("Cuando cuesta entenderles, dicen pocas palabras, se atascan al hablar o les cuesta la lectura. Trabajamos con juegos y con la familia.",
                            "When they are hard to understand, use few words, get stuck when speaking or find reading hard. We work through play and with the family."),
                           color="$muted"),
                         boton(("Ver qué trabajamos", "See what we work on"), a_pagina("trabajamos", "ninos"), "ghost")],
                        estilo="relleno", pad=28),
                tarjeta([h3(("Adultos", "Adults")),
                         p(("Cuando la voz se cansa o se queda ronca, o cuesta hablar tras una enfermedad o una lesión. Coordinamos con tu médico si hace falta.",
                            "When the voice tires or turns hoarse, or speaking is hard after an illness or injury. We liaise with your doctor if needed."),
                           color="$muted"),
                         boton(("Ver qué trabajamos", "See what we work on"), a_pagina("trabajamos", "adultos"), "ghost")],
                        estilo="relleno", pad=28),
            ], cols=2),
        ], tono="base", anchor="para-quien"),
        bloque_checks(("Cuándo conviene consultar", "When it is worth getting in touch"), [
            ("Cuesta entender a tu hijo o hija más de lo que te parece normal para su edad.", "Your child is harder to understand than seems usual for their age."),
            ("Se atasca, repite sonidos o evita hablar.", "They get stuck, repeat sounds or avoid speaking."),
            ("En el colegio os han comentado dificultades con el lenguaje o la lectura.", "The school has mentioned difficulties with language or reading."),
            ("A ti se te cansa o se te rompe la voz con frecuencia.", "Your voice tires or breaks often."),
            ("Notas que hablas peor desde una operación o un problema de salud.", "You speak less clearly since an operation or a health problem."),
            ("Tienes dudas. No hace falta tener un diagnóstico para preguntar.", "You are unsure. You do not need a diagnosis to ask."),
        ], intro=("Si dudas, pregunta: una conversación corta ya orienta.", "If in doubt, ask: a short conversation already points the way."),
            cols=2, tono="relleno", anchor="cuando-consultar"),
        bloque_pasos(("Cómo empezamos", "How we begin"), [
            (("Llamas o escribes", "You call or message"),
             ("Nos cuentas qué te preocupa y buscamos hueco para la valoración.", "You tell us what worries you and we find a slot for the assessment.")),
            (("Valoración inicial", "Initial assessment"),
             ("Hablamos con la familia y evaluamos con pruebas y juegos. En niños, sin prisa.", "We talk with the family and assess with tests and games. With children, without rushing.")),
            (("Te lo explicamos", "We explain it"),
             ("Qué hemos visto, si hace falta terapia y cada cuánto, en palabras claras.", "What we saw, whether therapy is needed and how often, in plain words.")),
            (("Sesiones", "Sessions"),
             ("Si procede, empezamos. Te damos pautas cortas para practicar en casa.", "If it is appropriate, we begin. We give you short exercises to practise at home.")),
        ], eyebrow=("Primera consulta", "First appointment"), tono="base"),
        bloque_opiniones(("Lo que cuentan las familias", "What families say"),
                         [("la valoración", "the assessment"), ("el trato con los niños", "how they treat the children"), ("las pautas para casa", "the home exercises")],
                         tono="relleno"),
        bloque_cta(("¿Hablamos?", "Shall we talk?"),
                   ("Cuéntanos qué te preocupa y te orientamos, sin compromiso.",
                    "Tell us what worries you and we will point you in the right direction, no strings attached."),
                   [cita(), whatsapp("ghost")], tono="base"),
    ])

    trab = pagina("trabajamos", ("que-trabajamos", "what-we-work-on"), ("Qué trabajamos", "What we work on"),
                  ("Dislalias, retraso del lenguaje, tartamudez, voz, apoyo en dislexia y logopedia para adultos explicados en llano, y preguntas frecuentes.",
                   "Speech sound difficulties, language delay, stammering, voice, dyslexia support and adult speech therapy explained plainly, with frequently asked questions."), [
        hero(("Qué trabajamos", "What we work on"),
             ("Lo que hacemos en cada caso, dicho sin tecnicismos. Cada persona avanza a su ritmo y la terapia se adapta a ella.",
              "What we do in each case, without jargon. Everyone progresses at their own pace and therapy adapts to them."),
             [cita(), llamar("secondary")], variante="texto"),
        seccion("features", [
            titular(("Niños y niñas", "Children")),
            rejilla([
                tarjeta([h3(("Dislalias: no pronunciar bien algunos sonidos", "Speech sound difficulties: not saying some sounds clearly")),
                         p(("Cambiar o saltarse sonidos, como decir «tapato» por «zapato». Practicamos cómo colocar la lengua y los labios, con ejercicios y juegos cortos.",
                            "Swapping or skipping sounds, such as saying «wabbit» for «rabbit». We practise tongue and lip placement with short exercises and games."),
                           color="$muted")], estilo="borde"),
                tarjeta([h3(("Retraso del lenguaje", "Language delay")),
                         p(("Cuando un niño dice menos palabras o frases más cortas de lo esperado, o entiende peor lo que se le dice. Estimulamos el lenguaje y damos pautas a la familia.",
                            "When a child uses fewer words or shorter sentences than expected, or understands less of what is said. We stimulate language and give the family guidance."),
                           color="$muted")], estilo="borde"),
                tarjeta([h3(("Tartamudez", "Stammering")),
                         p(("Bloqueos y repeticiones al hablar. Trabajamos para hablar con más fluidez y menos tensión, y con la familia, para que el ambiente ayude. No se trata de no equivocarse nunca.",
                            "Blocks and repetitions when speaking. We work towards more fluent, less tense speech, and with the family so the environment helps. The aim is not never to slip."),
                           color="$muted")], estilo="borde"),
                tarjeta([h3(("Dislexia: apoyo en lectura y escritura", "Dyslexia: support with reading and writing")),
                         p(("Entrenamos la lectura y la escritura como apoyo a lo que se trabaja en el colegio. El diagnóstico lo hace el equipo de orientación o un profesional que lo evalúe.",
                            "We train reading and writing as support for what is done at school. The diagnosis is made by the school guidance team or a professional who assesses it."),
                           color="$muted")], estilo="borde"),
            ], cols=2),
        ], tono="base", anchor="ninos"),
        seccion("features", [
            titular(("Adultos", "Adults")),
            rejilla([
                tarjeta([h3(("Voz", "Voice")),
                         p(("Afonía, ronquera o cansancio al hablar, sobre todo si usas la voz para trabajar. Vemos cómo la usas y te enseñamos a cuidarla. Si la ronquera dura semanas, antes consulta con tu médico.",
                            "Loss of voice, hoarseness or tiredness when speaking, especially if you use your voice for work. We look at how you use it and teach you to look after it. If hoarseness lasts for weeks, see your doctor first."),
                           color="$muted")], estilo="relleno"),
                tarjeta([h3(("Habla y lenguaje tras una lesión o enfermedad", "Speech and language after injury or illness")),
                         p(("Tras un ictus, una operación u otra enfermedad. Trabajamos con tu médico y con tu familia, con ejercicios adaptados a lo que necesitas en el día a día.",
                            "After a stroke, an operation or another illness. We work with your doctor and your family, with exercises adapted to what you need day to day."),
                           color="$muted")], estilo="relleno"),
            ], cols=2),
        ], tono="relleno", anchor="adultos"),
        bloque_faq(("Preguntas frecuentes", "Frequently asked questions"), [
            (("¿Cuánto dura la terapia?", "How long does therapy last?"),
             ("Depende de cada caso y se va viendo con el tiempo. Tras la valoración te damos una idea y la revisamos contigo cada cierto tiempo.",
              "It depends on the case and becomes clearer over time. After the assessment we give you an idea and review it with you from time to time.")),
            (("¿Es como una clase para mi hijo?", "Is it like a lesson for my child?"),
             ("No. Con los más pequeños las sesiones son de juego dirigido; con los mayores, ejercicios cortos con objetivos que entiendan.",
              "No. With the youngest the sessions are guided play; with older ones, short exercises with goals they can understand.")),
            (("¿Puedo estar en la sesión?", "Can I stay in the session?"),
             ("Sí, sobre todo al principio. Es una forma de aprender qué hacer en casa. Si es mejor que no, te lo decimos y lo hablamos.",
              "Yes, especially at the start. It is a way to learn what to do at home. If it is better that you do not, we will tell you and talk it through.")),
            (("¿Tengo que traer un informe del colegio o del médico?", "Do I have to bring a report from school or my doctor?"),
             ("No para pedir cita. Si tienes alguno, tráelo: nos ayuda a entender mejor el caso.",
              "Not to book. If you have one, bring it: it helps us understand the case better.")),
        ], tono="base"),
        bloque_cta(("¿Tienes dudas sobre tu caso?", "Not sure about your case?"), None, [cita(), whatsapp("ghost")], tono="relleno"),
    ])

    ses = pagina("sesiones", ("como-trabajamos", "how-we-work"), ("Cómo trabajamos", "How we work"),
                 ("Cómo es la valoración inicial y las sesiones de logopedia, trabajo con familias y colegios, informes, tarifas orientativas y equipo colegiado.",
                  "What the initial assessment and speech therapy sessions are like, work with families and schools, reports, indicative prices and the registered team."), [
        hero(("Cómo trabajamos", "How we work"),
             ("Una valoración al principio, sesiones con objetivos claros y contacto con la familia y el colegio.",
              "An assessment at the start, sessions with clear goals and contact with the family and school."),
             [cita(), llamar("secondary")], variante="texto"),
        bloque_texto(("La valoración inicial", "The initial assessment"), [
            ("Es una primera sesión más larga. Hablamos con la familia o contigo sobre qué ha pasado y desde cuándo, y evaluamos con pruebas, juegos o lectura según la edad.",
             "This is a longer first session. We talk with the family or with you about what has happened and since when, and assess with tests, games or reading depending on age."),
            ("Al terminar te explicamos qué hemos visto y si recomendamos terapia. Si no hace falta, también te lo decimos. Después te damos un informe por escrito.",
             "At the end we explain what we saw and whether we recommend therapy. If it is not needed, we tell you that too. Afterwards we give you a written report."),
        ], eyebrow=("Primera sesión", "First session"), tono="base", anchor="valoracion",
            imagen_=("photo", ("Mesa de consulta con juegos, tarjetas de imágenes y cuaderno", "Consulting table with games, picture cards and a notebook"))),
        bloque_valores(("Cómo son las sesiones", "What sessions are like"), [
            (("Individuales y breves", "One to one and short"),
             ("Suelen durar unos 45 minutos y se hacen con una frecuencia que se acuerda contigo.",
              "They usually last about 45 minutes and are held as often as we agree with you.")),
            (("Con objetivos claros", "With clear goals"),
             ("En cada sesión sabes qué se ha trabajado y qué toca practicar hasta la siguiente.",
              "In each session you know what was worked on and what to practise until the next one.")),
            (("Tareas cortas en casa", "Short tasks at home"),
             ("Unos minutos al día valen más que un rato largo. Te las damos por escrito y te enseñamos a hacerlas.",
              "A few minutes a day beat one long stretch. We give them in writing and show you how to do them.")),
        ], cols=3, estilo="borde", tono="relleno"),
        bloque_checks(("Trabajo con familias y colegios", "Working with families and schools"), [
            ("Pautas claras para la familia, adaptadas a vuestro día a día.", "Clear guidance for the family, adapted to your daily life."),
            ("Contacto con el tutor o la orientadora, con vuestro permiso.", "Contact with the class teacher or school counsellor, with your permission."),
            ("Ideas sencillas para el aula, como dónde sentarse o cómo dar instrucciones.", "Simple ideas for the classroom, such as where to sit or how to give instructions."),
            ("Reuniones de revisión para ver cómo va todo.", "Review meetings to see how things are going."),
        ], cols=2, tono="base", anchor="familias-colegios"),
        bloque_texto(("Informes", "Reports"), [
            ("Tras la valoración te damos un informe con lo que hemos visto y lo que proponemos. Si lo necesitas para el colegio, el médico u otro trámite, lo preparamos con el formato que te pidan.",
             "After the assessment we give you a report with what we saw and what we propose. If you need one for the school, your doctor or another procedure, we prepare it in the format you are asked for."),
        ], tono="relleno", anchor="informes"),
        bloque_carta(("Tarifas orientativas", "Indicative prices"), [
            (("Valoración y sesiones", "Assessment and sessions"), ("Precios de valoración, sesiones e informes", "Assessment, session and report prices"),
             [("Servicio", "Service"), ("Precio", "Price")],
             [(("Valoración inicial con informe", "Initial assessment with report"), eur(70)),
              (("Sesión individual (45 min)", "Individual session (45 min)"), eur(40)),
              (("Bono de 4 sesiones", "Pack of 4 sessions"), eur(150)),
              (("Informe para colegio o médico", "Report for school or doctor"), eur(30)),
              (("Reunión con el colegio", "Meeting with the school"), eur(45))]),
        ], nota=NOTA, tono="base", anchor="tarifas"),
        _equipo(),
        bloque_faq(("Dudas sobre las sesiones", "Questions about sessions"), [
            (("¿Qué pasa si mi hijo no quiere venir?", "What if my child does not want to come?"),
             ("Lo hablamos. Muchas veces ayuda cambiar el tipo de juego o el momento del día. Nadie tiene que sufrir en la consulta.",
              "We talk about it. Often changing the type of game or the time of day helps. Nobody has to suffer in the consulting room.")),
            (("¿Y si no puedo ir un día?", "What if I cannot make it one day?"),
             ("Avísanos con tiempo y buscamos otro hueco.", "Let us know in good time and we will find another slot.")),
        ], tono="relleno"),
    ])

    cont = pagina("contacto", ("contacto", "contact"), ("Contacto y cita", "Contact and appointments"),
                  ("Pide cita en la consulta de logopedia por teléfono, WhatsApp o formulario. Horario y cómo llegar.",
                   "Book at the speech therapy practice by phone, WhatsApp or form. Opening hours and how to get here."), [
        hero(("Pide tu cita", "Book your appointment"),
             ("Llámanos, escríbenos por WhatsApp o rellena el formulario. Te respondemos en uno o dos días laborables.",
              "Call us, message us on WhatsApp or fill in the form. We reply within one or two working days."),
             [llamar(), whatsapp()], variante="texto"),
        bloque_contacto(("Cómo encontrarnos", "How to find us"),
                        ("Consulta tranquila, con zona de espera para los más pequeños.", "A quiet practice with a waiting area for the little ones."),
                        campos_cita(servicios=[("Valoración inicial", "Initial assessment"), ("Sesión de seguimiento", "Follow-up session"),
                                               ("Informe", "Report"), ("Otra consulta", "Another query")],
                                    extra=[campo("para", "select", ("¿Para quién es?", "Who is it for?"), False,
                                                 opciones=[("Para un niño o niña", "For a child"), ("Para un adulto", "For an adult")]),
                                           campo("motivo", "text", ("¿Qué te preocupa? (en pocas palabras)", "What is worrying you? (in a few words)"), False, maxlen=200)]),
                        ("Pedir cita", "Request appointment"), horas,
                        notas=_junta(COLEGIADO, DATOS),
                        exito=("Gracias. Te respondemos en uno o dos días laborables para fijar la cita.",
                               "Thank you. We will reply within one or two working days to arrange the appointment.")),
        bloque_mapa(tono="relleno"),
    ])

    footer = pie(nav, ("Consulta de logopedia para niños y adultos, con cita previa.",
                       "Speech and language therapy for children and adults, by appointment."),
                 ("Lunes a jueves 9:30 a 13:30 y 16:00 a 20:00. Viernes 9:30 a 14:00.",
                  "Monday to Thursday 9:30 am to 1:30 pm and 4 pm to 8 pm. Friday 9:30 am to 2 pm."))
    footer["children"].insert(1, peq(COLEGIADO, color="$muted"))

    return plantilla(KEY, ("Consulta de logopedia", "Speech therapy practice"),
                     ("Consulta de logopedia para niños y adultos con lo que se trabaja explicado en llano, valoración y sesiones, trabajo con familias y colegios, tarifas orientativas y cita por WhatsApp.",
                      "Speech therapy practice for children and adults with what is worked on explained plainly, assessment and sessions, work with families and schools, indicative prices and WhatsApp booking."),
                     "salud", ("logopeda logopedia terapia del habla lenguaje tartamudez dislalia dislexia voz retraso del lenguaje ninos adultos consulta de logopedia pronunciacion",
                               "speech therapist speech and language therapy stammering stutter dyslexia voice language delay children adults pronunciation"),
                     look, cabecera(nav, marco([llamar(), whatsapp()], dir="row", gap=12, wrap=True)), footer,
                     [inicio, trab, ses, cont])
