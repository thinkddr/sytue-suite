# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
from lib import *
from looks import LOOKS  # noqa: F401  (la paleta es propia de este sector)

KEY = "ayuda-domicilio-mayores"

# Oliva dorado sobre crema de mantequilla. Sin fondo oscuro (nada usa $inverse).
MIEL = dict(
    colores=dict(background="#fdfbf0", surface="#f6f2d9", text="#27240d", muted="#5a5634", primary="#74630a",
                 on_primary="#ffffff", border="#e6e1b8", inverse="#2b280c", on_inverse="#f8f4d8"),
    fuentes=("manrope", "source-serif-4"), peso=700, cuerpo=(18, 17, 17), radio=(8, 14, 20), ancho=1120)

# Como COLEGIADO: el 00-0000 lo detecta domain.TextosDemo y el sitio no se publica hasta cambiarlo.
ACREDITACION = ("Servicio acreditado. Nº de acreditación: 00-0000", "Accredited service. Accreditation no.: 00-0000")
DATOS = ("Lo que nos cuentas sobre la salud de la persona y sobre su casa se trata con confidencialidad y según la normativa de protección de datos, y solo lo ve quien la atiende. No publicamos fotos de las personas que cuidamos.",
         "What you tell us about the person's health and their home is handled in confidence and under data protection rules, and only the people looking after them see it. We do not publish photos of the people we care for.")
NOTA = ("Precios de ejemplo: cámbialos por los tuyos. El presupuesto final depende de las horas, de la ayuda que necesite la persona y de si hay festivos.",
        "Example prices: replace them with your own. The final quote depends on the hours, how much help the person needs and whether holidays are included.")


def build():
    nueva(KEY)
    look = MIEL
    nav = [("inicio", ("Inicio", "Home")), ("servicios", ("Cuidados y cuidadores", "Care and carers")),
           ("tarifas", ("Horarios y tarifas", "Hours and rates")), ("contacto", ("Contacto", "Contact"))]
    horas = [(("Atención telefónica y visitas para conocernos", "Phone line and first meetings"),
              ("Lunes a viernes, de 9:00 a 14:00 y de 16:00 a 19:00", "Monday to Friday, 9:00 am to 2:00 pm and 4:00 pm to 7:00 pm")),
             (("Servicio de cuidado en casa", "Care at home"), ("Todos los días, también festivos y noches, con cita", "Every day, including holidays and nights, by arrangement")),
             (("Aviso por una incidencia de un cuidado en curso", "Contact about a care visit in progress"), ("Las 24 horas, por teléfono", "24 hours, by phone"))]
    cta_cab = marco([llamar(t=("Llamar", "Call")), whatsapp("secondary", t=("WhatsApp", "WhatsApp"))],
                    dir="row", gap=8, wrap=True)
    pedir = lambda v="primary": boton(("Pedir información", "Ask for information"), a_pagina("contacto", "contacto-form"), v)

    inicio = pagina("inicio", ("", ""), ("Ayuda a domicilio para mayores", "Home care for older people"),
                    ("Servicio de ayuda a domicilio para mayores y personas dependientes: aseo, compañía, movilidad, comidas y gestiones, por horas, noches o con cuidadora interna.",
                     "Home care service for older and dependent people: washing, company, mobility, meals and errands, by the hour, overnight or with a live-in carer."), [
        hero(("Que tu familiar siga en su casa, bien atendido", "Let your relative stay at home, well looked after"),
             ("Personas cuidadoras para ayudar a mayores y a personas dependientes en casa, por horas, por la noche o a jornada completa. Llámanos y lo organizamos contigo.",
              "Carers to help older and dependent people at home, by the hour, overnight or full time. Call us and we will arrange it with you."),
             [llamar(), whatsapp(), pedir("ghost")],
             eyebrow=("Ayuda a domicilio", "Home care"),
             extra=peq(ACREDITACION, color="$muted"),
             imagen_=("photo", ("Manos de una persona cuidadora sosteniendo las de una persona mayor sobre una mesa con una taza de té",
                                "A carer's hands holding an older person's hands on a table with a cup of tea"))),
        bloque_valores(("Con qué ayudamos en casa", "How we help at home"), [
            (("Aseo y vestirse", "Washing and dressing"),
             ("Ayuda con la ducha, el aseo, vestirse y arreglarse, cuidando la intimidad y sin prisa.",
              "Help with showering, washing, dressing and grooming, respecting privacy and without rushing.")),
            (("Compañía", "Company"),
             ("Charlar, pasear, leer, jugar a las cartas o simplemente estar. La soledad también se cuida.",
              "Chatting, walking, reading, playing cards or simply being there. Loneliness needs looking after too.")),
            (("Movilidad", "Mobility"),
             ("Levantarse de la cama o del sillón, andar por casa y salir con seguridad, y avisar si ve riesgo de caída.",
              "Getting out of bed or the armchair, moving around the house and going out safely, and flagging any risk of falls.")),
            (("Comidas", "Meals"),
             ("Hacer la compra, cocinar según la dieta que le han indicado, ayudar a comer y dejar la cocina recogida.",
              "Shopping, cooking according to the diet they have been given, helping with eating and leaving the kitchen tidy.")),
            (("Medicación", "Medication"),
             ("Recordar la hora de cada toma según la pauta de su médico. No cambiamos dosis ni medicamentos.",
              "Reminding them of each dose as set by their doctor. We do not change doses or medicines.")),
            (("Gestiones y recados", "Errands"),
             ("Ir a la farmacia, acompañar al médico o al banco, recoger recetas y ayudar con la limpieza ligera del día a día.",
              "Going to the pharmacy, going with them to the doctor or bank, collecting prescriptions and helping with light everyday housework.")),
        ], eyebrow=("Tareas", "Tasks"), cols=3, estilo="linea", anchor="tareas"),
        bloque_pasos(("Cómo empezar", "How to get started"), [
            (("Nos llamas", "You call us"), ("Nos cuentas quién necesita ayuda, qué le cuesta hacer y qué horas hacen falta.", "You tell us who needs help, what they find hard and which hours are needed.")),
            (("Vamos a conocerla", "We meet them"), ("Una persona nuestra visita la casa, habla con quien va a recibir el cuidado y con la familia, y apunta las rutinas.", "One of our staff visits the home, talks to the person who will be cared for and the family, and notes their routines.")),
            (("Te proponemos a alguien", "We suggest a carer"), ("Te presentamos a la persona cuidadora que mejor encaja. Si no hay buena relación, la cambiamos.", "We introduce the carer who fits best. If it does not go well, we change them.")),
            (("Empieza el cuidado", "Care begins"), ("Acordamos el horario y te llamamos al principio para ver cómo va. Puedes ajustar horas cuando haga falta.", "We agree the schedule and call you at the start to see how it goes. You can adjust the hours whenever needed.")),
        ], tono="relleno", anchor="empezar"),
        bloque_checks(("Con quien te quedas tranquilo", "So you can rest easy"), [
            ("Una persona de referencia en la oficina, que conoce el caso y coge el teléfono.", "A named contact at the office who knows the case and picks up the phone."),
            ("Si la persona cuidadora se pone enferma o no puede venir, buscamos una suplencia.", "If the carer falls ill or cannot come, we find a replacement."),
            ("Un cuaderno en casa donde se anota cada visita, para que la familia sepa cómo ha ido.", "A notebook at home where every visit is recorded, so the family knows how it went."),
            ("Te explicamos de antemano cuánto cuesta y qué incluye, por escrito.", "We explain beforehand what it costs and what it includes, in writing."),
        ], tono="base", cols=2),
        bloque_opiniones(("Lo que dicen las familias", "What families say"),
                         [("el trato de la cuidadora", "the carer's manner"), ("la puntualidad", "the punctuality"), ("la comunicación", "the communication")]),
        bloque_cta(("Cuéntanos qué necesitáis", "Tell us what you need"),
                   ("Sin compromiso: te escuchamos y te decimos cómo podríamos ayudar y cuánto costaría.",
                    "No obligation: we listen and tell you how we could help and what it would cost."),
                   [llamar(), pedir("secondary")], tono="base"),
    ])

    servicios = pagina("servicios", ("cuidados-y-cuidadores", "care-and-carers"), ("Cuidados y cuidadores", "Care and carers"),
                       ("Para quién es el servicio, cómo se eligen y se acompañan las personas cuidadoras y cómo cuidamos la privacidad de cada familia.",
                        "Who the service is for, how carers are chosen and supported, and how we protect each family's privacy."), [
        hero(("Cuidados y cuidadores", "Care and carers"),
             ("Quién puede necesitar el servicio y quién viene a casa. La confianza empieza por conocer a la persona que va a cuidar.",
              "Who might need the service and who comes to the home. Trust starts with knowing the person who will provide the care."),
             [pedir(), llamar("secondary")], variante="centro", tono="relleno", eyebrow=("Quién cuida", "Who cares")),
        bloque_valores(("Para quién es", "Who it is for"), [
            (("Mayores que viven solos", "Older people living alone"),
             ("Un rato al día para el aseo, la comida y la compañía, y que alguien se dé cuenta si algo no va bien.",
              "A little time each day for washing, meals and company, and someone to notice if something is wrong.")),
            (("Personas con dependencia", "People who are dependent"),
             ("Ayuda continuada con las tareas del día a día, adaptada a lo que cada persona puede hacer.",
              "Continuing help with everyday tasks, adapted to what each person can do.")),
            (("Tras una operación o un alta", "After surgery or a hospital stay"),
             ("Ayuda temporal en casa mientras se recupera, hasta que vuelva a manejarse solo o sola.",
              "Temporary help at home while recovering, until they can manage on their own again.")),
            (("Familias que necesitan un relevo", "Families who need a break"),
             ("Unas horas libres para quien cuida a diario, para trabajar, descansar o hacer gestiones.",
              "A few hours free for the person who cares every day, to work, rest or run errands.")),
        ], cols=2, estilo="borde", tono="base", anchor="para-quien"),
        bloque_texto(("Las personas que cuidan", "The people who care"), [
            ("Cada persona cuidadora trabaja con pocas familias a la vez para conocerlas bien. Se presenta antes del primer día, y si la relación no funciona la cambiamos sin que tengas que justificarlo.",
             "Each carer works with only a few families at a time so they get to know them well. They are introduced before the first day, and if it does not work out we change them without you needing to explain."),
            ("Les damos pautas claras para cada casa: horarios, dietas, rutinas y a quién llamar. Nadie está solo en un caso difícil.",
             "We give them clear instructions for each home: schedules, diets, routines and who to call. Nobody is left alone with a difficult case."),
        ], imagen_=("portrait", ("Persona cuidadora ayudando a ordenar una mesa de pastillero con una persona mayor, sin que se vean las caras",
                                 "A carer helping to organise a pill box on a table with an older person, with faces out of shot")),
            tono="relleno", anchor="cuidadores"),
        bloque_checks(("Cómo las seleccionamos", "How we select them"), [
            ("Entrevista personal para conocer cómo es y por qué quiere dedicarse a esto.", "A personal interview to learn what they are like and why they want to do this work."),
            ("Comprobamos las referencias de trabajos anteriores.", "We check references from previous jobs."),
            ("Pedimos la documentación en regla para trabajar en España.", "We ask for the documents they need to work in Spain."),
            ("Pedimos la formación y las titulaciones que declare cada persona y las comprobamos antes de anotarlas.", "We ask for the training and qualifications each person declares and check them before recording them."),
            ("Una visita de prueba con alguien de nuestro equipo antes de empezar sola en una casa.", "A trial visit with one of our team before they start alone in a home."),
            ("Hablamos con la familia a las primeras semanas para ver cómo va.", "We speak to the family in the first weeks to see how it is going."),
        ], intro=("Tú decides si la persona que proponemos te convence. Nadie empieza sin tu visto bueno.",
                  "You decide whether the person we suggest suits you. Nobody starts without your approval."),
            eyebrow=("Selección", "Selection"), tono="base", anchor="seleccion"),
        bloque_equipo(("Quién está detrás", "Who is behind it"), [
            (("Coordinación de casos", "Case coordination"),
             ("Visita las casas, prepara el plan de cuidados y es tu contacto en la oficina.",
              "Visits the homes, prepares the care plan and is your contact at the office.")),
            (("Persona cuidadora", "Carer"),
             ("Atiende a la persona en casa y anota cada visita en el cuaderno.",
              "Looks after the person at home and records each visit in the notebook.")),
            (("Atención a familias", "Family support"),
             ("Contesta al teléfono, organiza suplencias y cambios de horario.",
              "Answers the phone and organises replacements and schedule changes.")),
        ], tono="relleno", anchor="equipo"),
        bloque_texto(("Privacidad y respeto", "Privacy and respect"), [
            (DATOS[0], DATOS[1]),
            ("La persona cuidada decide en lo que pueda: qué ropa ponerse, qué comer y a qué hora levantarse. Cuidamos con ella, no por ella.",
             "The person being cared for decides what they can: what to wear, what to eat and when to get up. We care with them, not for them."),
        ], tono="base", anchor="privacidad"),
        bloque_cta(("¿Hablamos?", "Shall we talk?"), None, [pedir(), whatsapp("ghost")], tono="relleno"),
    ])

    tarifas = pagina("tarifas", ("horarios-y-tarifas", "hours-and-rates"), ("Horarios y tarifas", "Hours and rates"),
                     ("Turnos por horas, noches y cuidadora interna con sus tarifas, ayudas a la dependencia y preguntas frecuentes.",
                      "Hourly, overnight and live-in shifts with their rates, dependency support and frequently asked questions."), [
        hero(("Horarios y tarifas", "Hours and rates"),
             ("Elige cuántas horas necesitas y cuándo. Siempre te damos el precio por escrito antes de empezar.",
              "Choose how many hours you need and when. We always give you the price in writing before we start."),
             [pedir(), llamar("secondary")], variante="texto", extra=peq(ACREDITACION, color="$muted")),
        bloque_valores(("Tres formas de organizarlo", "Three ways to arrange it"), [
            (("Por horas", "By the hour"),
             ("Unas horas al día o a la semana, en la franja que te venga mejor. Desde dos horas por visita.",
              "A few hours a day or a week, at the time that suits you best. From two hours per visit.")),
            (("Por la noche", "Overnight"),
             ("Alguien que pasa la noche en casa y se levanta si la persona necesita ayuda o se pone nerviosa.",
              "Someone who spends the night at home and gets up if the person needs help or becomes anxious.")),
            (("Cuidadora interna", "Live-in carer"),
             ("Una persona que vive en la casa, con sus descansos y su día libre, para una atención continuada.",
              "A person who lives in the home, with their breaks and day off, for continuous care.")),
        ], cols=3, estilo="relleno", tono="relleno", anchor="modalidades"),
        bloque_precios(("Tarifas", "Rates"), ("Tarifas por tipo de servicio", "Rates by type of service"),
                       [("Servicio", "Service"), ("Cuándo", "When"), ("Tarifa", "Rate")],
                       [(("Por horas, laborables", "By the hour, weekdays"), ("De lunes a viernes, de 8:00 a 20:00", "Monday to Friday, 8:00 am to 8:00 pm"), ("16 € por hora", "€16 per hour")),
                        (("Por horas, sábados, domingos y festivos", "By the hour, Saturdays, Sundays and holidays"), ("Todo el día", "All day"), ("19 € por hora", "€19 per hour")),
                        (("Noche con presencia", "Overnight with presence"), ("De 22:00 a 8:00", "10:00 pm to 8:00 am"), ("90 € por noche", "€90 per night")),
                        (("Cuidadora interna", "Live-in carer"), ("Lunes a viernes, con descansos acordados", "Monday to Friday, with agreed breaks"), desde(1600)),
                        (("Acompañamiento a una cita médica", "Accompanying someone to a medical appointment"), ("Según la duración", "Depending on how long it lasts"), desde(32))],
                       nota=NOTA, boton_=pedir(), tono="base", anchor="tarifas"),
        seccion("text", [titular(("Turnos y horarios", "Shifts and hours"),
                                 ("Estos son los turnos más pedidos. Si necesitas otro, lo organizamos contigo.",
                                  "These are the most requested shifts. If you need another, we will arrange it with you.")),
                         tabla(("Turnos y horarios del servicio", "Service shifts and hours"), [("Turno", "Shift"), ("Franja", "Time"), ("Suele servir para", "Usually used for")],
                               [(("Mañana", "Morning"), ("De 8:00 a 12:00", "8:00 am to 12:00 pm"), ("Levantarse, aseo, desayuno y medicación", "Getting up, washing, breakfast and medication")),
                                (("Mediodía", "Midday"), ("De 12:00 a 16:00", "12:00 pm to 4:00 pm"), ("Compra, comida y recogida de la cocina", "Shopping, lunch and tidying the kitchen")),
                                (("Tarde", "Afternoon"), ("De 16:00 a 20:00", "4:00 pm to 8:00 pm"), ("Paseo, compañía, merienda y cena", "A walk, company, a snack and dinner")),
                                (("Noche", "Night"), ("De 22:00 a 8:00", "10:00 pm to 8:00 am"), ("Estar presente y ayudar si hace falta", "Being present and helping if needed")),
                                (("Interna", "Live-in"), ("Todo el día, con descansos", "All day, with breaks"), ("Atención continuada en la casa", "Continuous care in the home"))])],
                anchor="turnos", tono="relleno", gap=28),
        bloque_texto(("Ayudas a la dependencia", "Dependency support"), [
            ("Si la persona tiene reconocida una situación de dependencia, puede haber ayudas públicas para pagar parte del servicio. Las condiciones y las cuantías cambian según la comunidad y el año, así que consulta las vigentes en los servicios sociales de tu ayuntamiento o de tu comunidad.",
             "If the person has been officially recognised as dependent, there may be public help towards part of the cost. The conditions and amounts vary by region and by year, so check the current ones with your town hall's or your region's social services."),
            ("Si nos lo pides, te contamos qué suelen pedir y te damos los presupuestos y los justificantes que necesites. No podemos asegurarte que te la concedan.",
             "If you ask, we tell you what they usually require and give you the quotes and receipts you need. We cannot promise it will be granted."),
        ], tono="base", anchor="ayudas"),
        bloque_faq(("Preguntas frecuentes", "Frequently asked questions"), [
            (("¿Y si la persona cuidadora no puede venir?", "What if the carer cannot come?"),
             ("Te avisamos y enviamos a otra persona que ya conozca el caso o, si no es posible, que se haya leído las pautas de la casa. No te quedas sin atención.",
              "We tell you and send someone else who already knows the case or, if that is not possible, who has read the instructions for the home. You are not left without care.")),
            (("¿Puedo cambiar las horas?", "Can I change the hours?"),
             ("Sí. Avísanos con un par de días y ajustamos. Para una necesidad urgente, llámanos y vemos qué podemos hacer.",
              "Yes. Tell us a couple of days ahead and we adjust. For something urgent, call us and we will see what we can do.")),
            (("¿Qué pasa si no hay buena relación?", "What if they do not get along?"),
             ("Lo cambiamos. No hace falta que lo justifiques: lo importante es que la persona esté tranquila.",
              "We change it. You do not need to justify it: what matters is that the person is at ease.")),
            (("¿Cómo se organiza la contratación?", "How is the hiring arranged?"),
             ("Te lo explicamos antes de empezar, por escrito, con lo que incluye cada servicio. Si tienes dudas sobre el contrato o sobre las cuotas, pregunta en tu asesoría.",
              "We explain it before we begin, in writing, with what each service includes. If you have doubts about the contract or contributions, ask your adviser.")),
            (("¿Dais servicio si la persona tiene demencia?", "Do you provide care if the person has dementia?"),
             ("Cuéntanoslo al llamar. Hablamos del caso y vemos si podemos atenderlo bien; si no, te lo decimos con claridad.",
              "Tell us when you call. We talk about the case and see whether we can care for them well; if not, we tell you plainly.")),
        ], tono="relleno"),
    ])

    cont = pagina("contacto", ("contacto", "contact"), ("Contacto", "Contact"),
                  ("Pide información con el formulario, por teléfono o por WhatsApp. Horario de atención y cómo llegar a la oficina.",
                   "Ask for information with the form, by phone or on WhatsApp. Opening hours and how to get to the office."), [
        hero(("Cuéntanos qué necesitáis", "Tell us what you need"),
             ("Llámanos, escríbenos por WhatsApp o rellena el formulario. Te llamamos y vemos cómo podemos ayudar.",
              "Call us, message us on WhatsApp or fill in the form. We will call you and see how we can help."),
             [llamar(), whatsapp()], variante="texto", extra=peq(ACREDITACION, color="$muted")),
        bloque_contacto(("Datos para empezar", "Details to get started"),
                        ("Con estos datos te llamamos para concretar horas y precio. No hace falta que nos cuentes más por aquí.",
                         "With these details we will call you to settle the hours and price. You do not need to tell us more here."),
                        campos_cita(servicios=[("Por horas", "By the hour"), ("Por la noche", "Overnight"),
                                               ("Cuidadora interna", "Live-in carer"), ("Aún no lo sé", "Not sure yet")],
                                    fecha=False, mensaje=False,
                                    extra=[campo("para", "select", ("¿Para quién es?", "Who is it for?"), False,
                                                 opciones=[("Para mí", "For me"), ("Para mi madre o padre", "For my mother or father"),
                                                           ("Para otro familiar", "For another relative"), ("Otro caso", "Something else")]),
                                           campo("mensaje", "textarea", ("Qué ayuda necesita y en qué horas", "What help is needed and at what times"), False, maxlen=1000)]),
                        ("Pedir información", "Ask for information"), horas, anchor="contacto-form",
                        notas=DATOS,
                        exito=("Gracias. Te llamamos para hablar de lo que necesitáis.",
                               "Thank you. We will call you to talk about what you need.")),
        bloque_mapa(("Cómo llegar a la oficina", "How to get to the office"),
                    ("Si prefieres hablar en persona, ven a la oficina. Mejor con cita para que haya alguien esperándote.",
                     "If you would rather talk in person, come to the office. It is best to book so someone is expecting you."), tono="relleno"),
    ])

    return plantilla(KEY, ("Ayuda a domicilio para mayores", "Home care for older people"),
                     ("Ayuda a domicilio para mayores con tareas, selección de cuidadores, turnos por horas, noche o interna con tarifas de ejemplo, ayudas a la dependencia, nº de acreditación y formulario.",
                      "Home care for older people with tasks, how carers are selected, hourly, overnight or live-in shifts with their rates, dependency support, accreditation number and a form."),
                     "salud", ("ayuda a domicilio cuidado de mayores cuidadora cuidador interna dependencia ancianos acompanamiento aseo noche por horas asistencia domiciliaria personas mayores cuidadores sad",
                               "home care home help carer caregiver live-in carer elderly care dependent adults overnight care companionship personal care"),
                     look, cabecera(nav, cta_cab),
                     pie(nav, ("Ayuda a domicilio para mayores y personas dependientes. Nº de acreditación: 00-0000.",
                               "Home care for older and dependent people. Accreditation no.: 00-0000."),
                         ("Lunes a viernes 9:00 a 14:00 y 16:00 a 19:00. Aviso de incidencias las 24 horas.",
                          "Monday to Friday 9 am to 2 pm and 4 pm to 7 pm. Incident line 24 hours.")),
                     [inicio, servicios, tarifas, cont], vertical="ayuda-domicilio-mayores")
