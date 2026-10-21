# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
from lib import *

KEY = "prevencion-riesgos"

# Como COLEGIADO: el 00-0000 lo detecta domain.TextosDemo y el sitio no se publica hasta cambiarlo.
ACREDITACION = ("Nº de acreditación: 00-0000", "Accreditation no.: 00-0000")


def _c(background, surface, text, muted, primary, on_primary, border, inverse, on_inverse):
    return dict(background=background, surface=surface, text=text, muted=muted, primary=primary,
                on_primary=on_primary, border=border, inverse=inverse, on_inverse=on_inverse)


# Paleta propia: verde pizarra sobre blanco lila. Sin fondo $inverse.
SENAL = dict(colores=_c("#f4f2f8", "#e7e4ef", "#14221e", "#475a54", "#336655", "#ffffff", "#d8d4e4", "#12302a", "#ecf5f2"),
             fuentes=("work-sans", "inter"), radio=(3, 8, 12), ancho=1120, peso=700)


def build():
    nueva(KEY)
    look = SENAL
    malos = comprobar_paleta("senal", look["colores"])
    assert not malos, malos
    nav = [("inicio", ("Inicio", "Home")), ("servicios", ("Servicios y presupuesto", "Services and quote")),
           ("obligaciones", ("Obligaciones y cómo trabajamos", "Obligations and how we work")),
           ("contacto", ("Contacto", "Contact"))]
    horas = [(("Lunes a jueves", "Monday to Thursday"),
              ("8:30 a 14:00 y 15:30 a 18:30", "8:30 am to 2:00 pm and 3:30 pm to 6:30 pm")),
             (("Viernes", "Friday"), ("8:30 a 14:30", "8:30 am to 2:30 pm")),
             (("Sábado y domingo", "Saturday and Sunday"), ("Cerrado", "Closed"))]
    presu = lambda v="primary": boton(("Pedir presupuesto", "Ask for a quote"), a_pagina("contacto", "contacto"), v)
    nota_precios = ("Precios de ejemplo: cámbialos por los tuyos. IVA no incluido.",
                    "Example prices: replace them with your own. VAT not included.")

    inicio = pagina("inicio", ("", ""), ("Prevención de riesgos laborales", "Occupational risk prevention"),
                    ("Servicio de prevención de riesgos laborales para pymes: evaluación de riesgos, planes, formación, vigilancia de la salud y coordinación de actividades.",
                     "Occupational risk prevention service for small businesses: risk assessment, plans, training, health surveillance and coordination of activities."), [
        hero(("La prevención de tu empresa, resuelta y al día", "Your company's prevention, sorted and up to date"),
             ("Si tienes trabajadores, tienes que organizar la prevención. Nos encargamos del papeleo, de las visitas y de la formación para que tú te dediques a tu negocio.",
              "If you have employees, you have to organise prevention. We take care of the paperwork, the visits and the training so you can focus on your business."),
             [presu(), llamar("secondary"), whatsapp("ghost")],
             eyebrow=("Prevención para pymes", "Prevention for small businesses"),
             variante="texto", tono="relleno", extra=p(ACREDITACION, preset="small", color="$muted")),
        bloque_valores(("Qué hacemos", "What we do"), [
            (("Evaluación de riesgos", "Risk assessment"),
             ("Visitamos tu centro de trabajo, vemos cada puesto y anotamos qué puede causar un daño y cómo evitarlo.",
              "We visit your workplace, look at each job and note what could cause harm and how to avoid it.")),
            (("Plan de prevención", "Prevention plan"),
             ("El documento que recoge cómo organizas la prevención y qué medidas tomas, con responsables y fechas.",
              "The document that sets out how you organise prevention and which measures you take, with owners and dates.")),
            (("Formación", "Training"),
             ("Cursos cortos para cada puesto, en tu empresa o en línea, con justificante para cada trabajador.",
              "Short courses for each job, at your company or online, with a certificate for each employee.")),
            (("Vigilancia de la salud", "Health surveillance"),
             ("Reconocimientos médicos adaptados al puesto, hechos por personal sanitario y con datos confidenciales.",
              "Medical check-ups adapted to the job, carried out by healthcare staff with confidential data.")),
            (("Coordinación de actividades", "Coordination of activities"),
             ("Si en tu centro trabajan otras empresas, o trabajas tú en el de otra, intercambiamos la información de riesgos.",
              "If other companies work at your site, or you work at someone else's, we exchange risk information.")),
            (("Emergencias y accidentes", "Emergencies and accidents"),
             ("Te ayudamos a preparar qué hacer ante un incendio o un accidente y a revisar qué ocurrió.",
              "We help you prepare what to do in a fire or an accident and to review what happened.")),
        ], eyebrow=("Servicios", "Services"), estilo="borde", tono="base", anchor="que-hacemos"),
        bloque_pasos(("Cómo empezamos contigo", "How we start with you"), [
            (("Llamada", "A call"),
             ("Nos cuentas a qué te dedicas, cuántas personas trabajan y en cuántos centros.",
              "You tell us what you do, how many people work for you and at how many sites.")),
            (("Presupuesto", "Quote"),
             ("Te lo damos por escrito, con lo que incluye y cuánto cuesta por trabajador.",
              "We give it to you in writing, with what it includes and the cost per employee.")),
            (("Visita al centro", "Site visit"),
             ("Una persona técnica va a tu empresa, ve los puestos y habla con quien haga falta.",
              "A technician comes to your company, looks at the jobs and talks to whoever is needed.")),
            (("Documentación y calendario", "Documents and schedule"),
             ("Recibes los documentos y un calendario con lo que toca cada año: formación, revisiones, simulacros.",
              "You receive the documents and a schedule of what is due each year: training, check-ups, drills.")),
        ], tono="relleno", anchor="empezar"),
        bloque_texto(("Para quién trabajamos", "Who we work with"), [
            ("Pymes y autónomos con trabajadores: talleres, comercios, bares y restaurantes, oficinas, obras pequeñas, almacenes y empresas de servicios.",
             "Small businesses and self-employed people with employees: workshops, shops, bars and restaurants, offices, small building sites, warehouses and service companies."),
            ("Nos adaptamos al tamaño de tu empresa: sin documentos que no necesitas y sin dejar fuera los que sí.",
             "We adapt to your company's size: no documents you do not need and none of those you do left out."),
        ], eyebrow=("A quién ayudamos", "Who we help"), tono="base",
            imagen_=("photo", ("Persona con casco y chaleco revisando un puesto de trabajo", "Person in a helmet and vest checking a workstation")),
            invertir=True),
        bloque_opiniones(("Lo que dicen las empresas con las que trabajamos", "What the companies we work with say"),
                         [("las visitas", "the visits"), ("la formación", "the training"), ("la documentación", "the paperwork")],
                         tono="relleno"),
        bloque_cta(("Cuéntanos a qué se dedica tu empresa", "Tell us what your company does"),
                   ("Con eso te damos un presupuesto sin compromiso por trabajador.",
                    "With that we give you a no-obligation quote per employee."),
                   [presu(), llamar("ghost")], tono="base"),
    ])

    servicios = pagina("servicios", ("servicios", "services"), ("Servicios y presupuesto", "Services and quote"),
                       ("Evaluación de riesgos, planes, formación, vigilancia de la salud y coordinación de actividades, con un presupuesto por trabajador de ejemplo.",
                        "Risk assessment, plans, training, health surveillance and coordination of activities, with an example quote per employee."), [
        hero(("Servicios y presupuesto", "Services and quote"),
             ("Qué incluye cada servicio, cómo se organiza y cuánto cuesta por trabajador. El precio final depende de tu actividad y de tus centros.",
              "What each service includes, how it is organised and what it costs per employee. The final price depends on your activity and your sites."),
             [presu()], variante="texto"),
        bloque_carta(("Qué incluye cada servicio", "What each service includes"), [
            (("Evaluación y planificación", "Assessment and planning"), ("Servicios de evaluación y planificación", "Assessment and planning services"),
             [("Servicio", "Service"), ("Qué recibes", "What you receive")],
             [(("Evaluación de riesgos", "Risk assessment"), ("Informe por puesto con los riesgos y las medidas para reducirlos", "A report per job with the risks and the measures to reduce them")),
              (("Planificación de la actividad preventiva", "Planning of preventive activity"), ("Lista de medidas con responsable, plazo y prioridad", "A list of measures with owner, deadline and priority")),
              (("Plan de prevención y plan de emergencia", "Prevention plan and emergency plan"), ("Documentos adaptados a tu centro, listos para consultar", "Documents adapted to your site, ready to consult"))]),
            (("Personas", "People"), ("Servicios para las personas", "Services for people"),
             [("Servicio", "Service"), ("Qué recibes", "What you receive")],
             [(("Formación e información de puesto", "Job training and information"), ("Curso y justificante firmado por cada trabajador", "A course and signed certificate for each employee")),
              (("Vigilancia de la salud", "Health surveillance"), ("Reconocimiento voluntario por personal sanitario; a la empresa solo llegan las conclusiones de aptitud para el puesto", "A voluntary check-up by healthcare staff; the company receives only the conclusions on fitness for the job")),
              (("Coordinación de actividades empresariales", "Coordination of business activities"), ("Intercambio de información de riesgos con otras empresas del centro", "Exchange of risk information with other companies at the site"))]),
        ], intro=("La lista es de ejemplo: cámbiala por lo que ofrece tu servicio de prevención.",
                  "The list is an example: change it to what your prevention service offers."),
            nota=("La vigilancia de la salud es, por regla general, voluntaria para el trabajador, con las excepciones que fija la normativa. Los datos médicos son confidenciales.",
                  "Health surveillance is, as a general rule, voluntary for the employee, with the exceptions set by regulations. Medical data is confidential."),
            anchor="servicios", tono="base"),
        bloque_precios(("Presupuesto por trabajador", "Quote per employee"),
                       ("Presupuesto de ejemplo por trabajador", "Example quote per employee"),
                       [("Servicio", "Service"), ("Precio", "Price")],
                       [(("Evaluación y plan de prevención, por centro", "Assessment and prevention plan, per site"), desde(150)),
                        (("Seguimiento anual de la actividad preventiva, por trabajador", "Annual follow-up of preventive activity, per employee"), desde(60)),
                        (("Formación de puesto, por trabajador", "Job training, per employee"), desde(25)),
                        (("Reconocimiento médico, por trabajador", "Medical check-up, per employee"), desde(45)),
                        (("Coordinación de actividades, por empresa y año", "Coordination of activities, per company per year"), desde(120))],
                       intro=("Así calculamos un presupuesto: unas partidas fijas por centro o empresa y otras por trabajador. Te lo detallamos por escrito.",
                              "This is how we work out a quote: some fixed items per site or company and others per employee. We detail it for you in writing."),
                       eyebrow=("Precios", "Prices"), nota=nota_precios, boton_=presu(), tono="relleno"),
        bloque_faq(("Preguntas frecuentes", "Frequently asked questions"), [
            (("¿Cuánto cuesta para mi empresa?", "How much does it cost for my company?"),
             ("Depende de tu actividad, de cuántas personas trabajan y de cuántos centros tienes. Con esos datos te damos un presupuesto cerrado por escrito.",
              "It depends on your activity, how many people work for you and how many sites you have. With those details we give you a fixed quote in writing.")),
            (("¿Tengo que hacer los reconocimientos médicos a todos?", "Do I have to give check-ups to everyone?"),
             ("Por regla general es voluntario para el trabajador, salvo los casos que prevé la normativa. Te explicamos cuál es tu caso al contratar.",
              "As a general rule it is voluntary for the employee, except in the cases covered by regulations. We explain which is your case when you sign up.")),
            (("¿Lo que me dais sirve si viene una inspección?", "Is what you give me useful if there is an inspection?"),
             ("Te entregamos los documentos de prevención de tu empresa ordenados y al día, para que sepas dónde está cada cosa. No podemos prever lo que decida un inspector.",
              "We hand over your company's prevention documents in order and up to date, so you know where everything is. We cannot predict what an inspector will decide.")),
            (("¿Qué hago si un trabajador tiene un accidente?", "What do I do if an employee has an accident?"),
             ("Atiende a la persona, avisa a emergencias si hace falta y llámanos. Te decimos qué documentación preparar; el aviso oficial tiene plazos que te explicamos al contratar.",
              "Look after the person, call emergency services if needed and call us. We tell you what paperwork to prepare; the official notification has deadlines which we explain when you sign up.")),
        ], tono="base"),
        bloque_cta(("¿Te preparamos un presupuesto?", "Shall we prepare a quote for you?"), None,
                   [presu(), whatsapp("ghost")], tono="relleno"),
    ])

    oblig = pagina("obligaciones", ("obligaciones", "obligations"),
                   ("Obligaciones y cómo trabajamos", "Obligations and how we work"),
                   ("Qué suele tener que hacer una empresa con trabajadores en prevención, explicado en llano, y cómo trabajamos con tu empresa.",
                    "What a company with employees usually has to do about prevention, explained in plain words, and how we work with your company."), [
        hero(("Qué tienes que hacer y cómo te ayudamos", "What you have to do and how we help"),
             ("Una explicación en llano de lo que suele pedirse a una pyme, y cómo lo organizamos juntos. Es información general, no asesoramiento jurídico.",
              "A plain explanation of what small businesses are usually asked to do, and how we organise it together. It is general information, not legal advice."),
             [presu()], variante="texto"),
        bloque_valores(("Lo que suele pedirse a una empresa con trabajadores", "What a company with employees is usually asked to do"), [
            (("Organizar la prevención", "Organise prevention"),
             ("Decidir cómo se hace: con una persona de la empresa, con un servicio de prevención ajeno o de otra forma que prevea la normativa.",
              "Decide how it is done: with someone in the company, with an external prevention service or in another way the regulations allow.")),
            (("Evaluar los riesgos", "Assess the risks"),
             ("Revisar cada puesto, anotar los riesgos y planificar medidas para reducirlos, y actualizarlo cuando algo cambia.",
              "Review each job, note the risks, plan measures to reduce them, and update it when something changes.")),
            (("Informar y formar", "Inform and train"),
             ("Contar a cada trabajador los riesgos de su puesto y darle formación que pueda aplicar.",
              "Tell each employee the risks of their job and give them training they can apply.")),
            (("Cuidar la salud", "Look after health"),
             ("Ofrecer la vigilancia de la salud adecuada a cada puesto y tratar los datos médicos como confidenciales.",
              "Offer health surveillance suited to each job and treat medical data as confidential.")),
            (("Preparar las emergencias", "Prepare for emergencies"),
             ("Saber qué hacer y a quién llamar si hay un incendio, un accidente o hay que evacuar.",
              "Know what to do and who to call if there is a fire, an accident or an evacuation.")),
            (("Coordinarse con otras empresas", "Coordinate with other companies"),
             ("Si compartes centro con otras empresas, intercambiar información de riesgos antes de empezar a trabajar.",
              "If you share a site with other companies, exchange risk information before work begins.")),
        ], intro=("Lo que corresponde a tu empresa depende de tu actividad y de tu tamaño. Para un asunto legal concreto, consulta con un profesional del derecho.",
                  "What applies to your company depends on your activity and size. For a specific legal matter, consult a legal professional."),
            eyebrow=("En llano", "In plain words"), estilo="relleno", tono="base", anchor="obligaciones"),
        bloque_pasos(("Cómo trabajamos con tu empresa", "How we work with your company"), [
            (("Primera reunión", "First meeting"),
             ("Entendemos tu actividad y qué documentos tienes ya.", "We understand your activity and which documents you already have.")),
            (("Visita y evaluación", "Visit and assessment"),
             ("Revisamos puestos y equipos, y hablamos con las personas que los usan.", "We review jobs and equipment, and talk to the people who use them.")),
            (("Plan y formación", "Plan and training"),
             ("Te entregamos el plan y formamos a tu equipo según su puesto.", "We hand over the plan and train your team according to their job.")),
            (("Seguimiento", "Follow-up"),
             ("Revisamos cada año, y cuando hay cambios: nueva maquinaria, nuevo centro, un trabajador más.",
              "We review each year, and when things change: new machinery, a new site, one more employee.")),
        ], tono="relleno", anchor="como-trabajamos"),
        bloque_checks(("Qué necesitamos de ti", "What we need from you"), [
            ("Qué hace tu empresa y cuántas personas trabajan en ella", "What your company does and how many people work in it"),
            ("Dirección de cada centro de trabajo", "The address of each workplace"),
            ("Una persona de contacto para las visitas", "A contact person for the visits"),
            ("Documentos de prevención que ya tengas, si los hay", "Any prevention documents you already have"),
            ("Aviso de cualquier cambio: nuevo local, nueva máquina, nuevas contrataciones", "Notice of any change: new premises, new machine, new hires"),
        ], eyebrow=("Colaboración", "Working together"),
            intro=("Cuanta menos burocracia, mejor: pedimos solo lo que usamos.", "The less bureaucracy the better: we ask only for what we use."),
            tono="base", cols=1),
        bloque_cta(("Hablamos de tu empresa", "Let's talk about your company"), None,
                   [presu(), llamar("ghost")], tono="relleno"),
    ])

    cont = pagina("contacto", ("contacto", "contact"), ("Contacto y presupuesto", "Contact and quote"),
                  ("Pide un presupuesto sin compromiso con el formulario, por teléfono o por WhatsApp. Horario y cómo llegar.",
                   "Ask for a no-obligation quote using the form, by phone or on WhatsApp. Opening hours and directions."), [
        hero(("Pide tu presupuesto de prevención", "Ask for your prevention quote"),
             ("Dinos a qué se dedica tu empresa y cuántos trabajadores tiene. Te respondemos con un presupuesto por escrito y sin compromiso.",
              "Tell us what your company does and how many employees it has. We reply with a written quote, with no obligation."),
             [llamar(), whatsapp()], variante="texto"),
        bloque_contacto(("Cuéntanos sobre tu empresa", "Tell us about your company"),
                        ("Con la actividad, el número de trabajadores y los centros podemos darte un primer presupuesto.",
                         "With the activity, number of employees and sites we can give you a first quote."),
                        campos_consulta(("¿Qué necesitas?", "What do you need?"),
                                        servicios=[("Servicio de prevención completo", "Complete prevention service"),
                                                   ("Evaluación de riesgos", "Risk assessment"),
                                                   ("Formación para mi equipo", "Training for my team"),
                                                   ("Reconocimientos médicos", "Medical check-ups"),
                                                   ("Coordinación de actividades", "Coordination of activities"),
                                                   ("No lo tengo claro", "I am not sure")],
                                        extra=[campo("actividad", "text", ("Actividad de la empresa", "Company activity"), False, maxlen=120),
                                               campo("trabajadores", "select", ("Número de trabajadores", "Number of employees"), False,
                                                     opciones=[("De 1 a 5", "1 to 5"), ("De 6 a 25", "6 to 25"),
                                                               ("De 26 a 50", "26 to 50"), ("Más de 50", "More than 50")])]),
                        ("Pedir presupuesto", "Ask for a quote"), horas,
                        notas=("Lo que nos cuentes es confidencial y solo lo usamos para responderte.",
                               "What you tell us is confidential and we use it only to reply to you."),
                        exito=("Gracias. Te enviamos el presupuesto por correo o te llamamos para completar los datos.",
                               "Thank you. We will send the quote by email or call you to complete the details.")),
        bloque_mapa(texto_=("Recibimos con cita previa. Las visitas a tu centro de trabajo las hacemos nosotros.",
                            "We receive visitors by appointment. We carry out the visits to your workplace ourselves."), tono="relleno"),
    ])

    return plantilla(KEY, ("Prevención de riesgos laborales", "Occupational risk prevention"),
                     ("Servicio de prevención de riesgos laborales con servicios, obligaciones en llano, presupuesto por trabajador de ejemplo y formulario de presupuesto, llamada y WhatsApp.",
                      "Occupational risk prevention service with services, obligations in plain words, an example quote per employee and a quote form, call and WhatsApp."),
                     "profesionales", ("prevencion de riesgos laborales prevencion riesgos servicio de prevencion evaluacion de riesgos plan de prevencion formacion prl vigilancia de la salud reconocimientos medicos coordinacion de actividades seguridad laboral salud laboral pymes",
                                       "occupational risk prevention health and safety risk assessment prevention plan training workplace safety health surveillance medical check-ups coordination of activities occupational health small business"),
                     look, cabecera(nav, marco([llamar(), whatsapp("ghost")], dir="row", gap=12, wrap=True)),
                     pie(nav, ("Servicio de prevención de riesgos laborales para pymes: evaluación, planes, formación y vigilancia de la salud.",
                               "Occupational risk prevention service for small businesses: assessment, plans, training and health surveillance."),
                         ("Lunes a jueves 8:30 a 14:00 y 15:30 a 18:30; viernes 8:30 a 14:30.",
                          "Monday to Thursday 8:30 am to 2 pm and 3:30 pm to 6:30 pm; Friday 8:30 am to 2:30 pm.")),
                     [inicio, servicios, oblig, cont])
