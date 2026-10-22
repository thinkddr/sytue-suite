# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
from lib import *
from looks import LOOKS  # noqa: F401  (la paleta es propia de este sector)

KEY = "academia"

# Bolígrafo azul sobre papel de cuaderno, con el amarillo pálido de un marcador de fondo. Sin
# `tono="oscuro"`: primary/inverse no llega a 3, así que los bloques de énfasis usan `relleno`.
CUADERNO = dict(
    colores=dict(background="#fffdf7", surface="#fbf1c7", text="#17203a", muted="#4d5470", primary="#1c3f94",
                 on_primary="#ffffff", border="#e6dcae", inverse="#17203a", on_inverse="#fdf8e6"),
    fuentes=("figtree", "source-serif-4"), peso=700, radio=(4, 8, 12), ancho=1120)


def build():
    nueva(KEY)
    look = CUADERNO
    nav = [("inicio", ("Inicio", "Home")), ("clases", ("Clases y cuotas", "Classes and fees")),
           ("metodo", ("Método y familias", "Method and families")),
           ("contacto", ("Clase de prueba", "Trial class"))]
    horas = [(("Lunes a jueves", "Monday to Thursday"), ("16:00 a 21:00", "4:00 pm to 9:00 pm")),
             (("Viernes", "Friday"), ("16:00 a 20:00", "4:00 pm to 8:00 pm")),
             (("Sábado y domingo", "Saturday and Sunday"), ("Cerrado", "Closed")),
             (("Atención a familias", "Families' office hours"),
              ("Mañanas de lunes a viernes, de 10:00 a 13:00", "Weekday mornings, 10:00 am to 1:00 pm"))]

    def S(x):
        return (x, x)

    def prueba(var="primary"):
        return boton(("Pedir clase de prueba", "Book a trial class"), a_pagina("contacto", "contacto"), var)

    cabecera_ = cabecera(nav, marco([prueba(), llamar("secondary"), whatsapp("ghost")], dir="row", gap=12, wrap=True))

    def etapas():
        items = [("Primaria, de 1.º a 6.º", "Primary, years 1 to 6"), ("ESO, de 1.º a 4.º", "Secondary (ESO), years 1 to 4"),
                 ("Bachillerato, 1.º y 2.º", "Sixth form (Bachillerato), years 1 and 2"),
                 ("Preparación de selectividad (EBAU/PAU)", "University entrance exam preparation (EBAU/PAU)")]
        lista = lista_vertical([fila_li([icono("check", sz=22), p(t)], dir="row", gap=12, align="start") for t in items], gap=14)
        return tarjeta([etiqueta(("Cursos que atendemos", "Years we cover")), lista], estilo="relleno", pad=28, tag="div",
                       gap=18, sz={"width": "fill", "height": "hug", "min_width": 0})

    portada = seccion("hero", [marco([
        marco([etiqueta(("Apoyo escolar y refuerzo", "School support and revision")),
               h1(("Entender la asignatura, no solo aprobar el examen", "Understand the subject, not just pass the exam")),
               p(("Clases de refuerzo en grupos pequeños para primaria, ESO y bachillerato. Empezamos con una clase de prueba para ver dónde se atasca cada alumno y por dónde conviene empezar.",
                  "Revision classes in small groups for primary, secondary and sixth form. We start with a trial class to see where each student gets stuck and where it makes sense to begin."),
                 color="$muted"),
               marco([prueba(), llamar("secondary"), whatsapp("ghost")], dir="row", gap=12, wrap=True, align="center")],
              gap=18, sz={"width": "fill", "height": "hug", "min_width": 0}),
        etapas()], dir="row", gap=56, align="center",
        bp={"tablet": {"layout": {"gap": 32}}, "mobile": {"layout": {"direction": "column", "gap": 28}}})],
        pad_v=80, gap=0)

    inicio = pagina("inicio", ("", ""), ("Academia de apoyo escolar", "School support academy"),
                    ("Academia de apoyo escolar y refuerzo para primaria, ESO y bachillerato: grupos pequeños, horarios, cuotas y clase de prueba. Llama o escribe por WhatsApp.",
                     "School support and revision academy for primary, secondary and sixth form: small groups, timetables, fees and a trial class. Call or message us on WhatsApp."), [
        portada,
        bloque_checks(("Asignaturas que repasamos", "Subjects we revise"), [
            ("Matemáticas", "Maths"), ("Lengua y literatura", "Spanish language and literature"), ("Inglés", "English"),
            ("Física y química", "Physics and chemistry"), ("Biología y geología", "Biology and geology"),
            ("Historia y geografía", "History and geography"), ("Técnicas de estudio", "Study skills"),
            ("Ayuda con los deberes", "Help with homework"), ("Repaso antes de cada examen", "Revision before each exam"),
        ], intro=("Son asignaturas de ejemplo: cámbialas por las que das en tu academia.",
                  "These are example subjects: replace them with the ones you teach at your academy."),
            tono="relleno", cols=3),
        bloque_texto(("Un grupo pequeño, con un profesor que los conoce", "A small group, with a teacher who knows them"), [
            ("En un grupo pequeño todos preguntan. Cada alumno hace sus ejercicios en clase, el profesor los corrige en el momento y se ve enseguida qué no se ha entendido.",
             "In a small group everyone asks questions. Each student does their own exercises in class, the teacher marks them on the spot and it is clear straight away what has not been understood."),
            ("Los grupos son del mismo curso, para seguir el temario del colegio. Te decimos cuántos alumnos hay antes de que te apuntes.",
             "Groups are made up of the same school year, so we follow the school syllabus. We tell you how many students there are before you sign up."),
        ], eyebrow=("Cómo son las clases", "What the classes are like"),
            imagen_=("photo", ("Una profesora explica en la pizarra a un grupo pequeño de alumnos sentados a una mesa",
                               "A teacher explains at the whiteboard to a small group of students seated at a table"))),
        bloque_valores(("Qué se llevan las familias", "What families get"), [
            (("Un informe cada trimestre", "A report every term"),
             ("Corto y claro: a qué clases ha venido, qué ha trabajado y dónde le cuesta.",
              "Short and clear: which classes they attended, what they worked on and where they struggle.")),
            (("Una persona que responde", "Someone who answers"),
             ("Si tienes una duda, llamas o escribes y te contestamos en el día. No hace falta pedir cita.",
              "If you have a question, you call or write and we reply the same day. No appointment needed.")),
            (("Cero promesas de notas", "No promises about marks"),
             ("Nadie puede prometer un aprobado. Trabajamos con constancia y os contamos cómo va.",
              "Nobody can promise a pass. We work steadily and tell you how it is going.")),
        ], tono="base", estilo="borde"),
        bloque_opiniones(("Lo que dicen las familias", "What families say"),
                         [("el trato con los alumnos", "how the students are treated"),
                          ("los informes", "the reports"), ("el ambiente de la clase", "the atmosphere in class")]),
        bloque_cta(("¿Probamos una clase?", "Shall we try a class?"),
                   ("Cuéntanos el curso y las asignaturas y te damos hora para la clase de prueba. Sin compromiso.",
                    "Tell us the year and the subjects and we will give you a time for the trial class. No commitment."),
                   [prueba(), llamar("ghost")], tono="base"),
    ])

    nota_h = ("Horarios de ejemplo: cámbialos por los tuyos. Si no hay grupo a la hora que os viene bien, apuntad al alumno en la lista y os avisamos al abrir uno.",
              "Example timetables: replace them with your own. If there is no group at a time that suits you, add the student to the waiting list and we will tell you when one opens.")
    clases = pagina("clases", ("clases", "classes"), ("Clases y cuotas", "Classes and fees"),
                    ("Horarios de los grupos por curso y asignatura, cuotas mensuales de ejemplo y respuestas a las dudas más habituales.",
                     "Group timetables by year and subject, example monthly fees and answers to the most common questions."), [
        hero(("Clases, horarios y cuotas", "Classes, timetables and fees"),
             ("Mira qué grupos hay para su curso, cuánto cuesta cada mes y cómo funciona. Si no encuentras el horario, llámanos.",
              "See which groups there are for their year, what it costs each month and how it works. If you cannot find a timetable, call us."),
             [prueba()], variante="texto"),
        bloque_precios(("Horarios de los grupos", "Group timetables"), ("Horarios de ejemplo de los grupos", "Example group timetables"),
                       [("Curso", "Year"), ("Asignaturas", "Subjects"), ("Días", "Days"), ("Hora", "Time")],
                       [(("5.º y 6.º de primaria", "Years 5 and 6 of primary"), ("Matemáticas y lengua", "Maths and Spanish"),
                         ("Lunes y miércoles", "Monday and Wednesday"), S("16:30 a 17:30")),
                        (("1.º y 2.º de ESO", "Years 1 and 2 of ESO"), ("Matemáticas", "Maths"),
                         ("Martes y jueves", "Tuesday and Thursday"), S("17:00 a 18:00")),
                        (("1.º y 2.º de ESO", "Years 1 and 2 of ESO"), ("Lengua e inglés", "Spanish and English"),
                         ("Lunes y miércoles", "Monday and Wednesday"), S("17:30 a 18:30")),
                        (("3.º y 4.º de ESO", "Years 3 and 4 of ESO"), ("Matemáticas, física y química", "Maths, physics and chemistry"),
                         ("Martes y jueves", "Tuesday and Thursday"), S("18:30 a 20:00")),
                        (("1.º de bachillerato", "Sixth form, year 1"), ("Matemáticas y ciencias", "Maths and sciences"),
                         ("Lunes y miércoles", "Monday and Wednesday"), S("18:30 a 20:00")),
                        (("2.º de bachillerato y selectividad", "Sixth form, year 2 and entrance exam"),
                         ("Matemáticas, lengua e historia", "Maths, Spanish and history"),
                         ("Martes y jueves", "Tuesday and Thursday"), S("19:30 a 21:00"))],
                       nota=nota_h, anchor="horarios", kind="text", tono="base"),
        bloque_precios(("Cuotas", "Fees"), ("Cuotas mensuales de ejemplo", "Example monthly fees"),
                       [("Concepto", "Item"), ("Horas a la semana", "Hours a week"), ("Cuota al mes", "Fee per month")],
                       [(("Primaria", "Primary"), ("2 horas", "2 hours"), eur(45)),
                        (("ESO", "ESO"), ("3 horas", "3 hours"), eur(62)),
                        (("Bachillerato", "Sixth form"), ("3 horas", "3 hours"), eur(72)),
                        (("Preparación de selectividad", "Entrance exam preparation"), ("4 horas", "4 hours"), eur(85)),
                        (("Clase particular", "Private class"), ("Por clase de 1 hora", "Per 1-hour class"), eur(22)),
                        (("Matrícula (una vez al año)", "Enrolment fee (once a year)"), S("—"), eur(30)),
                        (("Clase de prueba", "Trial class"), ("1 hora", "1 hour"), ("Sin coste", "No charge"))],
                       nota=("Precios de ejemplo: cámbialos por los tuyos. Indica aquí si el material va incluido y si hay descuento por hermanos.",
                             "Example prices: replace them with your own. Say here whether materials are included and whether there is a sibling discount."),
                       anchor="cuotas", tono="relleno"),
        bloque_faq(("Preguntas frecuentes", "Frequently asked questions"), [
            (("¿Cuántos alumnos hay en cada grupo?", "How many students are in each group?"),
             ("Pocos, del mismo curso, para que todos tengan turno para preguntar. Te decimos el máximo antes de apuntarte.",
              "Few, all from the same year, so everyone gets a turn to ask. We tell you the maximum before you sign up.")),
            (("¿Podéis garantizar el aprobado?", "Can you guarantee a pass?"),
             ("No, y desconfía de quien lo haga. Trabajamos con el temario de su curso y cada trimestre te contamos cómo va.",
              "No, and be wary of anyone who does. We work with the syllabus for their year and each term we tell you how it is going.")),
            (("¿Hacéis los deberes por ellos?", "Do you do their homework for them?"),
             ("No. Los revisamos juntos y explicamos lo que no sale, para que la próxima vez lo hagan solos.",
              "No. We go through it together and explain what does not work, so next time they can do it alone.")),
            (("¿Se puede empezar a mitad de curso?", "Can they start mid-year?"),
             ("Sí, si hay plaza en un grupo de su curso. Si no, lo apuntamos en la lista de espera o proponemos clases particulares para ponerse al día.",
              "Yes, if there is a place in a group for their year. If not, we add them to the waiting list or suggest private classes to catch up.")),
            (("¿Y si falta un día?", "What if they miss a day?"),
             ("Avísanos y le pasamos lo que se ha hecho. Si hay hueco en otro grupo del mismo curso, puede recuperar la clase.",
              "Let us know and we send them what was covered. If there is space in another group for the same year, they can make the class up.")),
        ], tono="base"),
        bloque_cta(("¿Le buscamos grupo?", "Shall we find them a group?"), None, [prueba(), whatsapp("ghost")], tono="relleno"),
    ])

    metodo = pagina("metodo", ("metodo", "method"), ("Método y familias", "Method and families"),
                    ("Cómo trabajamos, quién da las clases, qué lleva el informe trimestral para las familias y qué no prometemos.",
                     "How we work, who teaches, what the termly report for families includes and what we do not promise."), [
        hero(("Cómo trabajamos", "How we work"),
             ("Explicamos poco y practicamos mucho. Y las familias saben en todo momento cómo va el curso.",
              "We explain a little and practise a lot. And families know at every point how the year is going."),
             [prueba(), llamar("secondary")], variante="texto"),
        bloque_pasos(("De la primera clase al informe", "From the first class to the report"), [
            (("Clase de prueba y charla", "Trial class and chat"),
             ("Una clase y diez minutos con la familia para saber qué le cuesta y qué esperáis.",
              "One class and ten minutes with the family to learn what they find hard and what you expect.")),
            (("Plan por asignatura", "A plan for each subject"),
             ("Fijamos qué repasar primero: lo del próximo examen y los huecos que vienen de cursos anteriores.",
              "We decide what to revise first: the next exam and the gaps left over from earlier years.")),
            (("Clase con ejercicios", "Classes with exercises"),
             ("Cada alumno resuelve y se corrige en el momento. El profesor pasa por todas las mesas.",
              "Each student solves and is corrected on the spot. The teacher goes round every desk.")),
            (("Informe cada trimestre", "A report each term"),
             ("Un informe corto para la familia y una reunión si la pedís.",
              "A short report for the family and a meeting if you ask for one.")),
        ], tono="base", anchor="pasos"),
        bloque_checks(("Qué lleva el informe", "What the report includes"), [
            ("Asistencia y puntualidad.", "Attendance and punctuality."),
            ("Temas trabajados en el trimestre.", "Topics covered during the term."),
            ("Dónde se atasca y qué hemos hecho al respecto.", "Where they get stuck and what we have done about it."),
            ("Qué puede hacer la familia en casa, sin añadir deberes.", "What the family can do at home, without adding homework."),
        ], eyebrow=("Informes a las familias", "Reports for families"), tono="relleno", cols=2, anchor="informes"),
        bloque_equipo(("Profesorado", "Teaching staff"), [
            (("Matemáticas y ciencias", "Maths and sciences"),
             ("Explica con ejemplos y no pasa de tema hasta que se entiende.",
              "Explains with examples and does not move on until it is understood.")),
            (("Lengua e idiomas", "Spanish and languages"),
             ("Corrige los textos con el alumno al lado, para que vea el porqué de cada error.",
              "Marks texts with the student alongside, so they see why each mistake happens.")),
            (("Primaria y técnicas de estudio", "Primary and study skills"),
             ("Enseña a organizar la semana, a subrayar y a preparar un examen.",
              "Teaches how to plan the week, how to highlight and how to prepare for an exam.")),
        ], intro=("Cambia estas fichas por tu profesorado: foto, nombre y asignaturas de cada persona.",
                  "Replace these cards with your teaching staff: photo, name and subjects for each person."),
            tono="base"),
        bloque_valores(("Lo que prometemos y lo que no", "What we promise and what we do not"), [
            (("No prometemos notas", "We do not promise marks"),
             ("Una nota depende de muchas cosas. Prometemos buena explicación, constancia y claridad con vosotros.",
              "A mark depends on many things. We promise good explanations, steadiness and clarity with you.")),
            (("Respetamos al colegio", "We respect the school"),
             ("Seguimos el temario y los métodos de su profesor. Nunca los contradecimos delante del alumno.",
              "We follow the syllabus and its teacher's methods. We never contradict them in front of the student.")),
            (("Hablamos claro", "We speak plainly"),
             ("Si pensamos que necesita otra cosa, como un apoyo especializado, te lo decimos.",
              "If we think they need something else, such as specialist support, we tell you.")),
        ], tono="relleno", estilo="borde"),
        bloque_cta(("¿Hablamos de su caso?", "Shall we talk about their situation?"), None, [prueba(), whatsapp("ghost")], tono="base"),
    ])

    campos = campos_cita(servicios=None, fecha=False, extra=[
        campo("curso", "select", ("Curso del alumno", "Student's year"), True,
              opciones=[("Primaria", "Primary"), ("1.º o 2.º de ESO", "ESO year 1 or 2"),
                        ("3.º o 4.º de ESO", "ESO year 3 or 4"), ("1.º de bachillerato", "Sixth form, year 1"),
                        ("2.º de bachillerato", "Sixth form, year 2"), ("Selectividad (EBAU/PAU)", "Entrance exam (EBAU/PAU)")]),
        campo("asignaturas", "select", ("Asignatura principal", "Main subject"), True,
              opciones=[("Matemáticas", "Maths"), ("Lengua", "Spanish"), ("Inglés", "English"),
                        ("Ciencias", "Sciences"), ("Historia y geografía", "History and geography"),
                        ("Varias asignaturas", "Several subjects")]),
        campo("franja", "select", ("Horario preferido", "Preferred time"), False,
              opciones=[("De 16:00 a 18:00", "4:00 pm to 6:00 pm"), ("De 18:00 a 21:00", "6:00 pm to 9:00 pm"),
                        ("Cualquiera", "Any")])])
    cont = pagina("contacto", ("contacto", "contact"), ("Clase de prueba y contacto", "Trial class and contact"),
                  ("Pide una clase de prueba con el formulario, por teléfono o por WhatsApp. Horario y cómo llegar a la academia.",
                   "Book a trial class with the form, by phone or on WhatsApp. Opening hours and how to find the academy."), [
        hero(("Pide una clase de prueba", "Book a trial class"),
             ("Dinos el curso y las asignaturas. Te llamamos para darte hora y contarte cómo es el grupo.",
              "Tell us the year and the subjects. We will call you to give you a time and tell you what the group is like."),
             [llamar(), whatsapp()], variante="texto"),
        bloque_contacto(("Cómo encontrarnos", "How to find us"),
                        ("Estamos cerca del instituto y de la parada de autobús. A la clase de prueba basta con venir con ganas y un cuaderno.",
                         "We are near the secondary school and the bus stop. For the trial class it is enough to come with a notebook."),
                        campos, ("Pedir clase de prueba", "Request trial class"), horas,
                        notas=("Solo pedimos los datos necesarios para contactar con la familia. Los datos del alumno los trata la academia según su política de privacidad.",
                               "We only ask for the details needed to contact the family. The student's details are processed by the academy under its privacy policy."),
                        exito=("Gracias. Te llamamos o te escribimos para darte hora para la clase de prueba.",
                               "Thank you. We will call or message you to give you a time for the trial class.")),
        bloque_mapa(tono="relleno"),
    ])

    return plantilla(KEY, ("Academia de apoyo escolar", "School support academy"),
                     ("Academia de refuerzo para primaria, ESO y bachillerato con horarios de grupos, cuotas de ejemplo, método en pasos, informes a las familias, clase de prueba con formulario, horario y mapa.",
                      "Revision academy for primary, secondary and sixth form with group timetables, example fees, a step-by-step method, reports for families, a trial class form, opening hours and a map."),
                     "educacion", ("academia apoyo escolar refuerzo clases particulares primaria eso bachillerato selectividad ebau pau repaso deberes tecnicas de estudio colegio instituto",
                                   "academy school support tutoring revision classes primary secondary sixth form entrance exam homework study skills"),
                     look, cabecera_,
                     pie(nav, ("Academia de apoyo escolar en grupos pequeños, con clase de prueba.",
                               "School support academy in small groups, with a trial class."),
                         ("Lunes a jueves 16:00 a 21:00. Viernes 16:00 a 20:00.", "Monday to Thursday 4 pm to 9 pm. Friday 4 pm to 8 pm.")),
                     [inicio, clases, metodo, cont])
