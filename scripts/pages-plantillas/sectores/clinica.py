# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
from lib import *
from looks import LOOKS  # noqa: F401  (la paleta es propia de este sector)

KEY = "clinica"

# Paleta propia (azul acero sobre gris azulado). `dentista` usa menta, `fisioterapeuta` pizarra y
# `optica` cobalto; tampoco comparte fuentes con ninguna. Sin fondo oscuro (primary/inverse < 3).
ACERO = dict(
    colores=dict(background="#f7f9fb", surface="#e7eef5", text="#10202e", muted="#465868", primary="#2d5f8e",
                 on_primary="#ffffff", border="#d2dde8", inverse="#12273a", on_inverse="#eef4fa"),
    fuentes=("work-sans", "manrope"), peso=600, radio=(4, 8, 12), ancho=1120)

DATOS = ("Tus datos y tu historia clínica se tratan con confidencialidad y según la normativa de protección de datos, y solo los ven quienes te atienden; al pedir cita aceptas ese tratamiento.",
         "Your details and medical record are handled in confidence and under data protection rules, and only the people treating you can see them; by booking you accept that processing.")
URGENCIAS = ("Esta clínica no es un servicio de urgencias. Si tienes dolor fuerte en el pecho, dificultad para respirar, pérdida de conciencia, una hemorragia que no para o sospecha de ictus, llama al 112.",
             "This clinic is not an emergency service. If you have severe chest pain, trouble breathing, loss of consciousness, bleeding that will not stop or suspect a stroke, call 112.")
NOTA = ("Precios de ejemplo: cámbialos por los tuyos. Si tienes seguro o mutua, la tarifa puede ser distinta.",
        "Example prices: replace them with your own. If you have insurance, the rate may be different.")


def _junta(*ts):
    return (" ".join(t[0] for t in ts), " ".join(t[1] for t in ts))


def _equipo(titulo, personas, intro=None, tono="base"):
    """Ficha del equipo con el número de colegiado visible (relleno detectable) en cada profesional."""
    tarjetas = []
    for rol, frase in personas:
        tarjetas.append(marco([imagen("portrait", ("Foto de la persona del equipo", "Photo of the team member"), ratio=0.9),
                               h3(("Nombre Apellido", "First Last")), peq(rol, color="$muted"), p(frase),
                               peq(COLEGIADO, color="$muted")], tag="li", gap=10))
    return seccion("team", [titular(titulo, intro), rejilla(tarjetas, cols=3)], tono=tono, anchor="equipo")


def portada(titulo, entradilla, botones, eyebrow, horas):
    """Portada propia: a la derecha del título, una tarjeta con el horario y qué hacer en una urgencia."""
    texto_ = marco([etiqueta(eyebrow), h1(titulo), p(entradilla, color="$muted"),
                    marco(botones, dir="row", gap=12, wrap=True, align="center")],
                   gap=18, sz={"width": "fill", "height": "hug", "min_width": 0})
    ficha = tarjeta([h2(("Cuándo estamos", "When we are open")),
                     horario(horas, caption=("Horario de consulta", "Consultation hours")),
                     peq(("Urgencia grave: llama al 112.", "Serious emergency: call 112."), color="$muted")],
                    estilo="relleno", tag="div", gap=14, pad=28, sz={"width": "fill", "height": "hug", "min_width": 0})
    fila = marco([texto_, ficha], dir="row", gap=56, align="center",
                 bp={"tablet": {"layout": {"gap": 32}}, "mobile": {"layout": {"direction": "column", "gap": 28}}})
    return seccion("hero", [fila], pad_v=80, gap=0)


def build():
    nueva(KEY)
    look = ACERO
    nav = [("inicio", ("Inicio", "Home")), ("especialidades", ("Especialidades", "Specialities")),
           ("equipo", ("El equipo", "The team")), ("contacto", ("Cita previa", "Appointments"))]
    horas = [(("Lunes a viernes", "Monday to Friday"),
              ("8:30 a 14:00 y 16:00 a 20:00", "8:30 am to 2:00 pm and 4:00 pm to 8:00 pm")),
             (("Sábado", "Saturday"), ("9:00 a 13:00", "9:00 am to 1:00 pm")),
             (("Domingo y festivos", "Sunday and public holidays"), ("Cerrado", "Closed")),
             (("Analíticas (extracción)", "Blood tests (sample taking)"),
              ("Lunes a viernes, de 8:30 a 10:30", "Monday to Friday, 8:30 am to 10:30 am"))]
    cta_cab = marco([llamar(t=("Llamar", "Call")), whatsapp("secondary", t=("WhatsApp", "WhatsApp"))],
                    dir="row", gap=8, wrap=True)
    cita = lambda v="primary": boton(("Pedir cita", "Book an appointment"), a_pagina("contacto", "cita"), v)

    inicio = pagina("inicio", ("", ""), ("Clínica médica", "Medical clinic"),
                    ("Clínica médica con medicina general, pediatría, ginecología y analíticas: especialidades, primera visita, urgencias, horario y cita previa.",
                     "Medical clinic with general medicine, paediatrics, gynaecology and blood tests: specialities, first visit, emergencies, opening hours and booking."), [
        portada(("Tu médico de cabecera, cerca de casa", "Your family doctor, close to home"),
                ("Medicina general, pediatría, ginecología y analíticas en un mismo centro, con cita previa y el tiempo de consulta que hace falta.",
                 "General medicine, paediatrics, gynaecology and blood tests in one centre, by appointment and with the consultation time that is needed."),
                [cita(), llamar("secondary"), whatsapp("ghost")],
                ("Clínica médica", "Medical clinic"), horas),
        bloque_valores(("Especialidades", "Specialities"), [
            (("Medicina general", "General medicine"),
             ("Consulta de adultos, seguimiento de enfermedades crónicas, recetas y derivación a especialistas.",
              "Adult consultations, follow-up of long-term conditions, prescriptions and referral to specialists.")),
            (("Pediatría", "Paediatrics"),
             ("Revisiones del niño sano, vacunas y consultas por fiebre, tos o cualquier duda de los más pequeños.",
              "Healthy-child check-ups, vaccines and consultations for fever, coughs or any worry about little ones.")),
            (("Ginecología", "Gynaecology"),
             ("Revisiones, citologías, anticoncepción y seguimiento del embarazo.",
              "Check-ups, smear tests, contraception and pregnancy follow-up.")),
            (("Enfermería", "Nursing"),
             ("Curas, inyectables, toma de tensión y vacunas.", "Dressings, injections, blood pressure checks and vaccines.")),
            (("Analíticas", "Blood tests"),
             ("Extracción en el centro, con o sin volante. Te decimos cuándo estarán los resultados.",
              "Samples taken at the centre, with or without a referral. We tell you when results will be ready.")),
            (("Certificados médicos", "Medical certificates"),
             ("Para el carné de conducir, el deporte o el trabajo. Trae tu documentación.",
              "For a driving licence, sport or work. Bring your documents.")),
        ], cols=3, estilo="borde", anchor="especialidades-resumen"),
        bloque_pasos(("Tu primera visita", "Your first visit"), [
            (("Pides cita", "You book"),
             ("Por teléfono, por WhatsApp o con el formulario. Te confirmamos el día y la hora.",
              "By phone, WhatsApp or with the form. We confirm the day and the time.")),
            (("Traes tus papeles", "You bring your papers"),
             ("DNI, tu tarjeta sanitaria o de la aseguradora, informes anteriores y la lista de medicación que tomas.",
              "ID, your health or insurance card, previous reports and a list of the medication you take.")),
            (("Te atiende tu médico", "Your doctor sees you"),
             ("Habla contigo, te explora y te cuenta lo que ve y qué opciones hay.",
              "Talks with you, examines you and tells you what they see and what the options are.")),
            (("Sales con un plan", "You leave with a plan"),
             ("Receta, informe o pruebas si hacen falta, y cuándo volver.",
              "A prescription, report or tests if needed, and when to come back.")),
        ], tono="relleno"),
        bloque_cta(("¿Es una urgencia?", "Is it an emergency?"), URGENCIAS,
                   [llamar(t=("Llamar a la clínica", "Call the clinic")), whatsapp("ghost")], tono="base"),
        bloque_opiniones(("Lo que dicen nuestros pacientes", "What our patients say"),
                         [("el trato", "the care"), ("las explicaciones", "the explanations"), ("la puntualidad", "the punctuality")],
                         tono="relleno"),
        bloque_cta(("¿Te damos cita?", "Shall we book you in?"),
                   ("Dinos para qué la quieres y qué día te viene bien, y te confirmamos.",
                    "Tell us what it is for and which day suits you, and we will confirm."),
                   [cita(), whatsapp("ghost")], tono="base"),
    ])

    esp = pagina("especialidades", ("especialidades", "specialities"), ("Especialidades y tarifas", "Specialities and rates"),
                 ("Qué atiende cada especialidad de la clínica, tarifas particulares de ejemplo, con qué seguros y mutuas se trabaja y preguntas frecuentes.",
                  "What each speciality at the clinic covers, example private rates, which insurers and mutuals we work with and frequently asked questions."), [
        hero(("Especialidades y tarifas", "Specialities and rates"),
             ("Qué atendemos en cada consulta, cuánto cuesta si vienes como particular y con qué seguros trabajamos.",
              "What we see in each consultation, what it costs as a private patient and which insurers we work with."),
             [cita(), llamar("secondary"), whatsapp("ghost")], variante="texto", tono="relleno"),
        bloque_valores(("Qué atiende cada consulta", "What each consultation covers"), [
            (("Medicina general", "General medicine"),
             ("Catarros y gripes, dolores, tensión alta, diabetes, colesterol y controles de salud. Si hace falta un especialista, te lo indicamos.",
              "Colds and flu, pain, high blood pressure, diabetes, cholesterol and health checks. If you need a specialist, we point you to one.")),
            (("Pediatría", "Paediatrics"),
             ("Control del crecimiento, calendario de vacunas, fiebre, alergias y problemas de piel. Desde recién nacidos hasta la adolescencia.",
              "Growth monitoring, vaccine schedule, fever, allergies and skin problems. From newborns to teenagers.")),
            (("Ginecología", "Gynaecology"),
             ("Revisión anual, citología, métodos anticonceptivos, menopausia y control del embarazo con ecografía.",
              "Annual check-up, smear test, contraception, menopause and pregnancy monitoring with ultrasound.")),
            (("Enfermería y analíticas", "Nursing and blood tests"),
             ("Extracciones, curas, vacunas y tomas de tensión.",
              "Blood samples, dressings, vaccines and blood pressure checks.")),
        ], cols=2, estilo="relleno", tono="base", anchor="que-atiende"),
        bloque_precios(("Tarifas particulares", "Private rates"), ("Tarifas de consulta para pacientes particulares", "Consultation rates for private patients"),
                       [("Servicio", "Service"), ("Tarifa", "Rate")],
                       [(("Consulta de medicina general", "General medicine consultation"), eur(45)),
                        (("Consulta de pediatría", "Paediatrics consultation"), eur(55)),
                        (("Consulta de ginecología", "Gynaecology consultation"), eur(70)),
                        (("Citología", "Smear test"), eur(60)),
                        (("Extracción de sangre", "Blood sample"), eur(10)),
                        (("Analítica básica", "Basic blood test"), desde(30)),
                        (("Certificado médico", "Medical certificate"), desde(25))],
                       nota=NOTA, anchor="tarifas", tono="relleno"),
        bloque_texto(("Seguros y mutuas", "Insurers and mutuals"), [
            ("Trabajamos con aseguradoras y mutuas. Lista de ejemplo: escribe aquí con cuáles, o di que solo atiendes como particular.",
             "We work with insurers and mutuals. Example list: write here which ones, or say you only see private patients."),
            ("Si tienes seguro, llama antes de venir para saber si hace falta autorización y qué cubre. Te lo comprobamos.",
             "If you have insurance, call before you come to find out whether authorisation is needed and what it covers. We check it for you."),
        ], anchor="seguros"),
        bloque_faq(("Preguntas frecuentes", "Frequently asked questions"), [
            (("¿Cuánto dura una consulta?", "How long does a consultation last?"),
             ("Una consulta suele durar entre 15 y 30 minutos. Si tienes varios asuntos, dilo al pedir cita y reservamos más tiempo.",
              "A consultation usually lasts between 15 and 30 minutes. If you have several matters, say so when booking and we set aside more time.")),
            (("¿Puedo traer a mi hijo si tiene fiebre?", "Can I bring my child if they have a fever?"),
             ("Llama por la mañana y te damos el primer hueco posible. Si el niño está muy decaído o tiene dificultad para respirar, no esperes: llama al 112.",
              "Call in the morning and we will give you the first slot we can. If the child seems very unwell or has trouble breathing, do not wait: call 112.")),
            (("¿Necesito volante para una analítica?", "Do I need a referral for a blood test?"),
             ("Si tu médico te la ha pedido, trae el volante. Si quieres hacerte un control por tu cuenta, lo pedimos en consulta.",
              "If your doctor requested it, bring the referral. If you want a check-up of your own accord, we request it at the consultation.")),
            (("¿Me dais un informe de lo que hemos hablado?", "Can I have a report of what we discussed?"),
             ("Sí. Pídelo al terminar la consulta y te lo damos por escrito.",
              "Yes. Ask for it at the end of the consultation and we give it to you in writing.")),
        ], tono="base"),
        bloque_cta(("¿Pedimos cita?", "Shall we book?"), None, [cita(), whatsapp("ghost")], tono="relleno"),
    ])

    equipo = pagina("equipo", ("el-equipo", "the-team"), ("El equipo y el centro", "The team and the centre"),
                    ("Los profesionales de la clínica con su número de colegiado, cómo se cuida la información de cada paciente y el centro por dentro.",
                     "The clinic's professionals with their registration number, how each patient's information is looked after and the centre inside."), [
        hero(("El equipo y el centro", "The team and the centre"),
             ("Un equipo pequeño y estable, para que te atienda siempre la misma persona.",
              "A small, stable team, so that the same person always sees you."),
             [cita(), llamar("secondary")], variante="texto"),
        _equipo(("Quién te atiende", "Who sees you"), [
            (("Medicina general", "General medicine"),
             ("Atiende a adultos, coordina tu seguimiento y te deriva si hace falta.",
              "Sees adults, coordinates your follow-up and refers you when needed.")),
            (("Pediatría", "Paediatrics"),
             ("Controla el crecimiento y las vacunas y atiende las consultas de los más pequeños.",
              "Monitors growth and vaccines and sees the little ones' consultations.")),
            (("Ginecología", "Gynaecology"),
             ("Revisiones, anticoncepción y seguimiento del embarazo.",
              "Check-ups, contraception and pregnancy follow-up.")),
        ], intro=("Texto de ejemplo: escribe aquí el nombre y el número de colegiado de cada profesional.",
                  "Example text: write here the name and registration number of each professional."),
            tono="relleno"),
        bloque_checks(("Tu información y tu seguridad", "Your information and your safety"), [
            (DATOS[0], DATOS[1]),
            ("Puedes pedirnos una copia de tu historia o de un informe cuando la necesites.", "You can ask us for a copy of your record or a report whenever you need one."),
            ("El material de curas e inyectables es de un solo uso.", "Dressing and injection supplies are single-use."),
            ("Si algo no te ha quedado claro, pregúntanos: para eso es la consulta.", "If anything is unclear, ask us: that is what the consultation is for."),
        ], tono="base", cols=2),
        bloque_galeria(("El centro por dentro", "Inside the centre"), [
            ("Sala de espera de la clínica", "Clinic waiting room"),
            ("Consulta médica con camilla y escritorio", "Doctor's office with an examination couch and desk"),
            ("Sala de extracciones", "Sample-taking room"),
            ("Recepción de la clínica", "Clinic reception")], cols=2, ratio=1.4, tono="relleno"),
        bloque_cta(("Ven a conocernos", "Come and meet us"),
                   ("Pasa por el centro o llámanos y te enseñamos las instalaciones.",
                    "Drop by the centre or call us and we will show you round."), None, tono="base"),
    ])

    cont = pagina("contacto", ("cita-previa", "appointments"), ("Cita previa y cómo llegar", "Appointments and how to find us"),
                  ("Pide cita con el formulario, por teléfono o por WhatsApp. Horario, qué hacer en una urgencia y mapa para llegar a la clínica.",
                   "Book with the form, by phone or on WhatsApp. Opening hours, what to do in an emergency and a map to the clinic."), [
        hero(("Pide tu cita", "Book your appointment"),
             ("Llámanos, escríbenos por WhatsApp o rellena el formulario. Te confirmamos el mismo día.",
              "Call us, message us on WhatsApp or fill in the form. We confirm the same day."),
             [llamar(), whatsapp()], variante="texto"),
        bloque_contacto(("Datos para tu cita", "Details for your appointment"),
                        ("Este formulario es para pedir cita, no para consultas médicas ni urgencias.",
                         "This form is for booking, not for medical questions or emergencies."),
                        campos_cita(servicios=[("Medicina general", "General medicine"), ("Pediatría", "Paediatrics"),
                                               ("Ginecología", "Gynaecology"), ("Analítica", "Blood test"),
                                               ("Certificado médico", "Medical certificate"), ("Otro motivo", "Something else")],
                                    extra=[campo("cobertura", "select", ("¿Cómo vienes?", "How are you coming?"), False,
                                                 opciones=[("Como particular", "As a private patient"),
                                                           ("Con seguro o mutua", "With insurance or a mutual")])]),
                        ("Pedir cita", "Request appointment"), horas, anchor="cita",
                        notas=_junta(COLEGIADO, DATOS),
                        exito=("Gracias. Te confirmamos la cita por teléfono o WhatsApp en cuanto la tengamos.",
                               "Thank you. We will confirm your appointment by phone or WhatsApp as soon as we have it.")),
        bloque_texto(("Urgencias y qué hacer", "Emergencies and what to do"), [
            URGENCIAS,
            ("Si no es grave pero no puedes esperar a tu cita, llámanos y te damos el primer hueco que tengamos.",
             "If it is not serious but you cannot wait for your appointment, call us and we will give you the first slot we have."),
        ], tono="relleno", anchor="urgencias"),
        bloque_mapa(tono="base"),
    ])

    footer = pie(nav, ("Clínica médica con cita previa. Medicina general, pediatría, ginecología y analíticas.",
                       "Medical clinic by appointment. General medicine, paediatrics, gynaecology and blood tests."),
                 ("Lunes a viernes 8:30 a 14:00 y 16:00 a 20:00. Sábado 9:00 a 13:00.",
                  "Monday to Friday 8:30 am to 2 pm and 4 pm to 8 pm. Saturday 9 am to 1 pm."))
    footer["children"].insert(1, peq(COLEGIADO, color="$muted"))

    return plantilla(KEY, ("Clínica médica", "Medical clinic"),
                     ("Clínica médica con especialidades en tarjetas, primera visita en pasos, tarifas de ejemplo, seguros y mutuas, equipo colegiado, urgencias, horario, mapa y cita previa.",
                      "Medical clinic with specialities as cards, first visit in steps, example rates, insurers and mutuals, registered team, emergencies, opening hours, a map and booking."),
                     "salud", ("clinica clinica medica policlinica centro medico medicina general medico de cabecera pediatria ginecologia analiticas enfermeria cita previa mutuas seguros urgencias certificado medico",
                               "clinic medical clinic polyclinic medical centre general practitioner family doctor paediatrics gynaecology blood tests nursing appointment insurance emergencies medical certificate"),
                     look, cabecera(nav, cta_cab), footer, [inicio, esp, equipo, cont])
