# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
from lib import *
from looks import LOOKS

KEY = "guarderia"

# Como COLEGIADO: el 00-0000 lo detecta domain.TextosDemo y el sitio no se publica hasta cambiarlo.
AUTORIZACION = ("Escuela infantil autorizada. Nº de centro: 00-0000",
                "Authorised nursery school. Centre no.: 00-0000")


def dia(titulo, intro, cap, filas, anchor):
    """Horario del día como tabla: lo que se hace y a qué hora."""
    return seccion("text", [titular(titulo, intro),
                            tabla(cap, [("Hora", "Time"), ("Qué hacemos", "What we do")], filas)],
                   anchor=anchor, gap=28)


def build():
    nueva(KEY)
    look = dict(LOOKS["durazno"], fuentes=("figtree", "manrope"), radio=(12, 18, 26))
    nav = [("inicio", ("Inicio", "Home")), ("escuela", ("El día a día", "A day at the school")),
           ("matricula", ("Matrícula", "Enrolment")), ("contacto", ("Contacto", "Contact"))]
    horas = [(("Lunes a viernes", "Monday to Friday"), ("7:30 a 17:30", "7:30 am to 5:30 pm")),
             (("Visitas y atención a familias", "Visits and family enquiries"),
              ("De 10:00 a 13:00, y por la tarde con cita", "10:00 am to 1:00 pm, and in the afternoon by appointment")),
             (("Sábado y domingo", "Saturday and Sunday"), ("Cerrado", "Closed"))]
    cta_cab = marco([llamar(t=("Llamar", "Call")), whatsapp("secondary", t=("WhatsApp", "WhatsApp"))],
                    dir="row", gap=8, wrap=True)
    visita = lambda v="primary": boton(("Pedir una visita", "Ask for a visit"), a_pagina("contacto", "visita"), v)

    inicio = pagina("inicio", ("", ""), ("Escuela infantil", "Nursery school"),
                    ("Escuela infantil de 0 a 3 años: aulas por edades, comedor con menús, periodo de adaptación y visita al centro. Llama o escribe por WhatsApp.",
                     "Nursery school for ages 0 to 3: classrooms by age, meals, a settling-in period and a visit to the school. Call or message us on WhatsApp."), [
        hero(("Una escuela infantil cerca de casa, para los primeros años", "A nursery school close to home, for the early years"),
             ("Cuidamos a niños y niñas de 0 a 3 años con un horario pensado para familias que trabajan. Ven a conocer las aulas antes de decidir.",
              "We look after children from 0 to 3 years old, with opening hours that suit working families. Come and see the classrooms before you decide."),
             [llamar(), whatsapp(), visita("ghost")],
             eyebrow=("Escuela infantil de 0 a 3 años", "Nursery school, ages 0 to 3"),
             extra=peq(AUTORIZACION, color="$muted"),
             imagen_=("photo", ("Aula luminosa con alfombra, estantes bajos y juguetes de madera",
                                "Bright classroom with a rug, low shelves and wooden toys"))),
        bloque_valores(("Cómo cuidamos", "How we look after them"), [
            (("Jugar es aprender", "Play is learning"),
             ("A estas edades se aprende tocando, moviéndose y jugando. Preparamos rincones y materiales para que cada niño o niña elija.",
              "At this age children learn by touching, moving and playing. We set up corners and materials so each child can choose.")),
            (("Rutinas tranquilas", "Calm routines"),
             ("Comer, dormir y asearse a las mismas horas ayuda a los peques a sentirse seguros. Respetamos el ritmo de cada uno.",
              "Eating, sleeping and washing at the same times helps little ones feel safe. We respect each child's pace.")),
            (("Una persona de referencia", "A key person"),
             ("Cada niño o niña tiene una educadora o un educador de referencia en el aula, que conoce sus costumbres.",
              "Each child has a key educator in the classroom who knows their habits.")),
            (("Familias al día", "Families kept informed"),
             ("Al recoger te contamos cómo ha ido el día: qué ha comido, cuánto ha dormido y a qué ha jugado.",
              "At pick-up we tell you how the day went: what they ate, how long they slept and what they played.")),
        ], eyebrow=("Cómo trabajamos", "How we work"), cols=2, estilo="relleno"),
        bloque_texto(("Nuestro proyecto educativo, en llano", "Our educational project, in plain words"), [
            ("Pensamos que lo importante entre los 0 y los 3 años es que el niño o la niña esté a gusto, se sienta querido y tenga espacio para explorar.",
             "We believe that between 0 and 3 what matters is that children feel comfortable and loved and have room to explore."),
            ("Por eso hay mucho juego libre, tiempo en el patio, cuentos y canciones, y poca ficha. El proyecto completo lo tienes en papel y te lo explicamos en la visita.",
             "That is why there is plenty of free play, time in the yard, stories and songs, and very little paperwork for the children. You can have the full project on paper and we go through it at the visit."),
        ], imagen_=("landscape", ("Patio exterior con arenero y bancos bajos", "Outdoor yard with a sandpit and low benches")),
            tono="relleno", invertir=True),
        bloque_valores(("Aulas por edades", "Classrooms by age"), [
            (("Bebés, de 0 a 1 año", "Babies, 0 to 1"),
             ("Aula tranquila con zona de cunas, suelo blando y biberones y papillas a la hora de cada uno.",
              "A quiet room with a cot area and a soft floor, with bottles and purées at each baby's own time.")),
            (("Caminantes, de 1 a 2 años", "Walkers, 1 to 2"),
             ("Empiezan a andar y a comer solos. Más movimiento, juegos de encajar y primeras canciones.",
              "They start walking and feeding themselves. More movement, stacking games and first songs.")),
            (("Mayores, de 2 a 3 años", "Older ones, 2 to 3"),
             ("Hablan cada vez más. Cuentos, pintura con las manos, juego simbólico y dejar el pañal a su tiempo.",
              "They talk more and more. Stories, finger painting, pretend play and leaving nappies behind in their own time.")),
        ], eyebrow=("Edades y aulas", "Ages and classrooms"), estilo="borde"),
        bloque_opiniones(("Lo que dicen las familias", "What families say"),
                         [("la adaptación", "the settling-in"), ("el trato de las educadoras", "how the educators treat the children"),
                          ("la comunicación", "the communication")]),
        bloque_cta(("Ven a conocer la escuela", "Come and see the school"),
                   ("Te enseñamos las aulas, el patio y la cocina, y resolvemos tus dudas sin compromiso.",
                    "We show you the classrooms, the yard and the kitchen, and answer your questions with no obligation."),
                   [visita(), llamar("secondary")], tono="base"),
    ])

    escuela = pagina("escuela", ("el-dia-a-dia", "a-day-at-the-school"), ("El día a día", "A day at the school"),
                     ("Cómo es un día en la escuela infantil: horario, comedor y menús con alérgenos y periodo de adaptación.",
                      "What a day at the nursery school is like: timetable, meals and menus with allergens, and the settling-in period."), [
        hero(("Un día en la escuela", "A day at the school"),
             ("Así pasan el día los peques: llegada, juego, comida, siesta y vuelta a casa. Los horarios se adaptan a la edad de cada grupo.",
              "This is how the little ones spend the day: arrival, play, lunch, nap and home time. Times adapt to each age group."),
             [visita(), llamar("secondary")], variante="texto"),
        dia(("Horario de un día", "A typical day"),
            ("Es un ejemplo. Los bebés comen y duermen cuando lo necesitan.",
             "This is an example. Babies eat and sleep when they need to."),
            ("Horario de un día en la escuela", "Timetable of a day at the school"),
            [(("7:30 a 9:30", "7:30 to 9:30"), ("Llegada y juego libre. Los que llegan temprano desayunan.", "Arrival and free play. Early arrivals have breakfast.")),
             (("9:30 a 10:00", "9:30 to 10:00"), ("Asamblea con canciones y fruta.", "Circle time with songs and fruit.")),
             (("10:00 a 11:15", "10:00 to 11:15"), ("Actividades en el aula: juego con materiales, pintura, música.", "Classroom activities: play with materials, painting, music.")),
             (("11:15 a 11:45", "11:15 to 11:45"), ("Patio o paseo, según el tiempo.", "Yard or a walk, depending on the weather.")),
             (("11:45 a 12:45", "11:45 to 12:45"), ("Aseo y comida.", "Washing and lunch.")),
             (("12:45 a 15:00", "12:45 to 3:00 pm"), ("Siesta, con un adulto siempre en la sala.", "Nap, with an adult always in the room.")),
             (("15:00 a 16:00", "3:00 to 4:00 pm"), ("Aseo y merienda.", "Washing and an afternoon snack.")),
             (("16:00 a 17:30", "4:00 to 5:30 pm"), ("Juego tranquilo y recogida.", "Quiet play and pick-up."))],
            "dia"),
        bloque_checks(("Comedor y menús", "Meals and menus"), [
            ("Cocinamos en la escuela, con menú de temporada que cambia cada mes.", "We cook at the school, with a seasonal menu that changes every month."),
            ("Cada semana colgamos el menú con los alérgenos de cada plato.", "Every week we put up the menu with the allergens in each dish."),
            ("Pedimos a las familias que nos digan cualquier alergia o intolerancia al matricular.", "We ask families to tell us about any allergy or intolerance when enrolling."),
            ("Hay menú adaptado para alergias, intolerancias y dietas por motivos de salud o de creencias.", "There is an adapted menu for allergies, intolerances and diets for health or belief reasons."),
            ("Los bebés toman la leche o el biberón que traéis, y las papillas a su ritmo.", "Babies have the milk or bottle you bring, and purées at their own pace."),
            ("Les damos tiempo para comer solos, aunque se manchen.", "We give them time to feed themselves, even if it gets messy."),
        ], eyebrow=("Comedor", "Meals"), tono="relleno", anchor="comedor"),
        bloque_pasos(("El periodo de adaptación", "The settling-in period"), [
            (("Primera visita", "First visit"),
             ("Venís los dos con el peque, vemos el aula y conocéis a su educadora.", "You both come with your child, see the classroom and meet their educator.")),
            (("Ratos cortos", "Short stays"),
             ("Los primeros días se queda un rato, con vosotros o sin vosotros, según cómo se sienta.", "In the first days they stay for a short while, with or without you, depending on how they feel.")),
            (("Más horas", "Longer days"),
             ("Alargamos poco a poco hasta que se queda a comer y a dormir.", "We extend little by little until they stay for lunch and the nap.")),
            (("Jornada completa", "Full days"),
             ("Cada niño o niña tarda lo que necesita: unos días o un par de semanas.", "Every child takes what they need: a few days or a couple of weeks.")),
        ], intro=("No hay prisa. Acordamos el calendario con cada familia.", "There is no rush. We agree the schedule with each family."),
            anchor="adaptacion"),
        bloque_faq(("Preguntas frecuentes", "Frequently asked questions"), [
            (("¿Qué hago si mi hijo o hija llora al dejarle?", "What if my child cries when I leave?"),
             ("Es normal. Nos lo entregas con calma, te despides sin alargarlo y te avisamos si necesita algo. Al recoger te contamos cómo ha estado.",
              "That is normal. You hand them over calmly, say goodbye without dragging it out and we tell you if they need anything. At pick-up we tell you how they were.")),
            (("¿Qué tengo que traer cada día?", "What do I need to bring each day?"),
             ("Una muda, pañales y toallitas, el chupete si usa, y una mochila marcada con su nombre. Te damos la lista completa al matricular.",
              "A change of clothes, nappies and wipes, a dummy if they use one, and a bag with their name on it. We give you the full list at enrolment.")),
            (("¿Y si está malito?", "What if they are unwell?"),
             ("Si tiene fiebre o una enfermedad que se contagia, no puede venir. Si se pone malo aquí, te llamamos para que vengas a por él o ella.",
              "If they have a fever or an infectious illness they cannot come in. If they fall ill here, we call you to come and collect them.")),
            (("¿Dais medicación?", "Do you give medicine?"),
             ("Solo con receta o informe del pediatra y con tu autorización por escrito.",
              "Only with a prescription or a paediatrician's note and your written permission.")),
        ], tono="relleno"),
    ])

    matricula = pagina("matricula", ("matricula", "enrolment"), ("Matrícula", "Enrolment"),
                       ("Horario de apertura y de matrícula, documentos para matricularse y respuestas sobre plazas y cuotas.",
                        "Opening and enrolment hours, documents you need to enrol, and answers about places and fees."), [
        hero(("Matrícula", "Enrolment"),
             ("Te explicamos cómo reservar una plaza, qué documentos hacen falta y cuándo puedes empezar.",
              "We explain how to reserve a place, which documents you need and when your child can start."),
             [visita(), whatsapp()], variante="texto", extra=peq(AUTORIZACION, color="$muted")),
        bloque_pasos(("Cómo conseguir una plaza", "How to get a place"), [
            (("Visita", "Visit"), ("Vienes a ver la escuela y a conocernos. Sin compromiso.", "You come to see the school and meet us. No obligation.")),
            (("Solicitud", "Application"), ("Rellenas la solicitud con los datos de tu hijo o hija y del horario que necesitas.", "You fill in the application with your child's details and the hours you need.")),
            (("Reserva de plaza", "Reserving the place"), ("Confirmamos si hay plaza en su aula y te decimos la cuota.", "We confirm whether there is a place in their classroom and tell you the fee.")),
            (("Adaptación", "Settling in"), ("Acordamos el calendario de los primeros días.", "We agree the schedule for the first days.")),
        ], tono="base", anchor="pasos"),
        bloque_checks(("Documentos para matricular", "Documents to enrol"), [
            ("Libro de familia o documento de identidad del niño o la niña.", "Family book or ID document of the child."),
            ("DNI o NIE de la madre, el padre o quien tenga la tutela.", "ID or NIE of the mother, father or legal guardian."),
            ("Tarjeta sanitaria y cartilla de vacunación al día.", "Health card and up-to-date vaccination record."),
            ("Informe médico si hay alergias, intolerancias o necesidades especiales.", "Medical report if there are allergies, intolerances or special needs."),
            ("Datos de las personas autorizadas a recoger al niño o la niña.", "Details of the people authorised to collect the child."),
            ("Datos bancarios para el recibo mensual.", "Bank details for the monthly direct debit."),
        ], intro=("Los iremos pidiendo contigo. Si te falta alguno, no pasa nada: empezamos con lo que tengas.",
                  "We go through them with you. If you are missing one, it does not matter: we start with what you have."),
            eyebrow=("Papeles", "Paperwork"), tono="relleno", anchor="documentos"),
        seccion("text", [titular(("Horario de apertura y de matrícula", "Opening and enrolment hours"),
                                 ("La escuela abre a las 7:30 y cierra a las 17:30. Para visitas y matrícula, mejor por la mañana.",
                                  "The school opens at 7:30 am and closes at 5:30 pm. For visits and enrolment, mornings are best.")),
                         tabla(("Horario de apertura y de matrícula", "Opening and enrolment hours"),
                               [("Qué", "What"), ("Cuándo", "When")],
                               [(("Escuela abierta", "School open"), ("Lunes a viernes, de 7:30 a 17:30", "Monday to Friday, 7:30 am to 5:30 pm")),
                                (("Visitas y matrícula", "Visits and enrolment"), ("Lunes a viernes, de 10:00 a 13:00", "Monday to Friday, 10:00 am to 1:00 pm")),
                                (("Por la tarde", "In the afternoon"), ("Con cita previa", "By appointment"))]),
                         peq(("Las fechas de matrícula y el calendario del curso los publicamos cada año; pregúntanos por ellos.",
                              "We publish enrolment dates and the school calendar every year; ask us about them."), color="$muted")],
               anchor="horario-matricula", gap=24),
        bloque_faq(("Plazas, cuotas y datos", "Places, fees and data"), [
            (("¿Cuánto cuesta?", "How much does it cost?"),
             ("Depende del horario y de si incluye comedor. Te damos la cuota por escrito en la visita. Pregunta también en tu ayuntamiento o comunidad por ayudas y bonificaciones.",
              "It depends on the hours and whether meals are included. We give you the fee in writing at the visit. Also ask your town hall or region about grants and discounts.")),
            (("¿Desde qué edad admitís?", "From what age do you take children?"),
             ("Te lo decimos en la visita, porque depende de la edad del bebé y de las plazas del aula.",
              "We tell you at the visit, because it depends on the baby's age and the places in the classroom.")),
            (("¿Puedo elegir horas sueltas?", "Can I choose individual hours?"),
             ("Tenemos jornada completa, media jornada y horarios ampliados. Cuéntanos qué necesitas y vemos qué encaja.",
              "We offer full days, half days and extended hours. Tell us what you need and we will see what fits.")),
            (("¿Qué hacéis con los datos y las fotos de mi hijo o hija?", "What do you do with my child's data and photos?"),
             ("Los datos solo los usamos para la matrícula y el cuidado, y las fotos solo se publican con tu autorización por escrito.",
              "We use the data only for enrolment and care, and photos are published only with your written permission.")),
        ], tono="base"),
        bloque_cta(("¿Vienes a verla?", "Will you come and see it?"), None, [visita(), llamar("secondary")], tono="relleno"),
    ])

    cont = pagina("contacto", ("contacto", "contact"), ("Contacto y visita", "Contact and visit"),
                  ("Pide una visita a la escuela con el formulario, por teléfono o por WhatsApp. Horario y cómo llegar.",
                   "Ask for a visit to the school with the form, by phone or on WhatsApp. Opening hours and how to find us."), [
        hero(("Pide una visita", "Ask for a visit"),
             ("Llámanos, escríbenos por WhatsApp o rellena el formulario. Te proponemos un día y una hora para que vengas con tu peque.",
              "Call us, message us on WhatsApp or fill in the form. We suggest a day and time for you to come with your little one."),
             [llamar(), whatsapp()], variante="texto", extra=peq(AUTORIZACION, color="$muted")),
        bloque_contacto(("Ven a conocernos", "Come and meet us"),
                        ("Con unos datos te llamamos para fijar la visita. La visita dura unos veinte minutos.",
                         "With a few details we will call you to set the visit. It takes about twenty minutes."),
                        campos_cita(extra=[
                            campo("edad", "select", ("Edad del niño o la niña", "Child's age"), False,
                                  opciones=[("De 0 a 1 año", "0 to 1 year"), ("De 1 a 2 años", "1 to 2 years"),
                                            ("De 2 a 3 años", "2 to 3 years"), ("Aún no ha nacido", "Not born yet")]),
                            campo("correo", "email", ("Correo electrónico (opcional)", "Email (optional)"), False,
                                  auto="email", maxlen=200)]),
                        ("Pedir visita", "Ask for a visit"), horas, anchor="visita",
                        notas=("Los datos de tu hijo o hija solo se usan para atender tu solicitud. No pedimos nada más en este formulario.",
                               "Your child's details are used only to answer your request. We ask for nothing more in this form."),
                        exito=("Gracias. Te llamamos para acordar el día y la hora de la visita.",
                               "Thank you. We will call you to agree the day and time of the visit.")),
        bloque_mapa(("Cómo llegar", "How to find us"),
                    ("Tienes parada de autobús cerca y sitio para aparcar un momento junto a la puerta.",
                     "There is a bus stop nearby and room to park briefly by the door."), tono="relleno"),
    ])

    return plantilla(KEY, ("Escuela infantil", "Nursery school"),
                     ("Escuela infantil de 0 a 3 años con proyecto educativo, horario del día, aulas por edades, menús con alérgenos, adaptación, matrícula y formulario de visita.",
                      "Nursery school for ages 0 to 3 with educational project, daily timetable, classrooms by age, menus with allergens, settling-in, enrolment and a visit form."),
                     "educacion", ("guarderia escuela infantil infantil bebes bebe peques ludoteca educacion infantil cole comedor adaptacion matricula visita",
                                   "nursery nursery school daycare day care preschool creche childcare babies toddlers early years enrolment settling in"),
                     look, cabecera(nav, cta_cab),
                     pie(nav, ("Escuela infantil de 0 a 3 años. Nº de centro: 00-0000.",
                               "Nursery school for ages 0 to 3. Centre no.: 00-0000."),
                         ("Lunes a viernes 7:30 a 17:30.", "Monday to Friday 7:30 am to 5:30 pm.")),
                     [inicio, escuela, matricula, cont], vertical="guarderia")
