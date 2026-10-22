# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
from lib import *
from looks import LOOKS

KEY = "fisioterapeuta"

NOTA = ("Precios de ejemplo: cámbialos por los tuyos.", "Example prices: replace them with your own.")
URGENCIA = ("Si tienes dolor en el pecho, pérdida de fuerza repentina, o dolor con fiebre, no esperes a una sesión: acude a urgencias o llama al 112.",
            "If you have chest pain, sudden loss of strength, or pain with a fever, do not wait for a session: go to A&E or call 112.")


def _equipo(titulo, personas, intro=None, tono="base"):
    """Ficha del equipo con el número de colegiado visible (relleno detectable)."""
    tarjetas = [marco([imagen("portrait", ("Foto de la persona del equipo", "Photo of the team member"), ratio=0.9),
                       h3(("Nombre Apellido", "First Last")), peq(rol, color="$muted"), p(frase),
                       peq(COLEGIADO, color="$muted")], tag="li", gap=10) for rol, frase in personas]
    return seccion("team", [titular(titulo, intro), rejilla(tarjetas, cols=2)], tono=tono, anchor="equipo")


def build():
    nueva(KEY)
    look = LOOKS["pizarra"]
    nav = [("inicio", ("Inicio", "Home")), ("tratamientos", ("Tratamientos", "Treatments")),
           ("tarifas", ("Tarifas y bonos", "Prices and vouchers")), ("contacto", ("Contacto", "Contact"))]
    horas = [(("Lunes a viernes", "Monday to Friday"),
              ("9:00 a 14:00 y 16:00 a 20:00", "9:00 am to 2:00 pm and 4:00 pm to 8:00 pm")),
             (("Sábado", "Saturday"), ("9:00 a 13:00, con cita", "9:00 am to 1:00 pm, by appointment")),
             (("Domingo", "Sunday"), ("Cerrado", "Closed")),
             (("A domicilio", "Home visits"), ("Con cita, de lunes a viernes", "By appointment, Monday to Friday"))]
    reservar = lambda v="primary": boton(("Reservar sesión", "Book a session"), a_pagina("contacto", "contacto"), v)

    inicio = pagina("inicio", ("", ""), ("Fisioterapia", "Physiotherapy"),
                    ("Clínica de fisioterapia con cita previa: lesiones, dolor de espalda, fisioterapia deportiva, suelo pélvico y a domicilio. Llama o escribe por WhatsApp.",
                     "Physiotherapy clinic by appointment: injuries, back pain, sports physiotherapy, pelvic floor and home visits. Call or message us on WhatsApp."), [
        hero(("Fisioterapia con cita, para que te muevas mejor", "Physiotherapy by appointment, to help you move better"),
             ("Lesiones, dolor de espalda, fisioterapia deportiva y suelo pélvico. Empezamos por escucharte y valorarte, y después te explicamos el plan sesión a sesión.",
              "Injuries, back pain, sports physiotherapy and pelvic floor. We start by listening to you and assessing you, then explain the plan session by session."),
             [reservar(), whatsapp()],
             eyebrow=("Clínica de fisioterapia", "Physiotherapy clinic"), variante="centro"),
        bloque_checks(("Quién viene a vernos", "Who comes to see us"), [
            ("Personas con una lesión reciente o que arrastran una vieja", "People with a recent injury or an old one that lingers"),
            ("Dolor de espalda, cuello u hombros por el trabajo o el estrés", "Back, neck or shoulder pain from work or stress"),
            ("Deportistas que quieren recuperarse o prevenir", "Sportspeople who want to recover or prevent injury"),
            ("Embarazo, posparto y suelo pélvico", "Pregnancy, postpartum and pelvic floor"),
            ("Personas que no pueden desplazarse y prefieren el tratamiento en casa", "People who cannot travel and prefer treatment at home"),
        ], eyebrow=("Para quién", "Who it is for"), cols=2, tono="relleno"),
        bloque_pasos(("Cómo es la primera sesión", "What the first session is like"), [
            (("Te escuchamos", "We listen"),
             ("Qué te duele, desde cuándo, qué te limita y qué quieres volver a hacer.",
              "What hurts, since when, what limits you and what you want to get back to doing.")),
            (("Te valoramos", "We assess you"),
             ("Miramos cómo te mueves, dónde duele y qué lo provoca.",
              "We look at how you move, where it hurts and what triggers it.")),
            (("Empezamos", "We begin"),
             ("Si procede, la misma sesión incluye un primer tratamiento: manos, ejercicio o ambos.",
              "If appropriate, the same session includes a first treatment: hands-on work, exercise or both.")),
            (("Te damos el plan", "We give you the plan"),
             ("Cuántas sesiones calculamos, cada cuánto y qué hacer en casa. Lo revisamos en cada visita.",
              "How many sessions we estimate, how often and what to do at home. We review it at each visit.")),
        ], intro=("Dura unos 60 minutos. Ven con ropa cómoda y trae los informes o pruebas que tengas.",
                  "It lasts about 60 minutes. Wear comfortable clothes and bring any reports or scans you have."),
            tono="base"),
        bloque_texto(("También vamos a tu casa", "We also come to your home"), [
            ("Si tienes movilidad reducida, estás recuperándote de una operación o con un bebé pequeño, te tratamos en casa con material portátil.",
             "If you have reduced mobility, are recovering from an operation or have a small baby, we treat you at home with portable equipment."),
            ("Se concierta por teléfono y se avisa del desplazamiento antes de confirmar la cita.",
             "It is arranged by phone and any travel charge is stated before the appointment is confirmed."),
        ], imagen_=("landscape", ("Fisioterapeuta tratando a una persona en su salón", "Physiotherapist treating a person in their living room")),
            tono="relleno", anchor="domicilio"),
        _equipo(("Quién te trata", "Who treats you"), [
            (("Fisioterapeuta, lesiones y deporte", "Physiotherapist, injuries and sport"),
             ("Valoración, tratamiento manual y ejercicio terapéutico.",
              "Assessment, hands-on treatment and therapeutic exercise.")),
            (("Fisioterapeuta, suelo pélvico", "Physiotherapist, pelvic floor"),
             ("Embarazo, posparto y suelo pélvico, en un espacio privado.",
              "Pregnancy, postpartum and pelvic floor, in a private room.")),
        ], tono="base"),
        bloque_opiniones(("Lo que dicen nuestros pacientes", "What our patients say"),
                         [("el trato", "the care"), ("las explicaciones", "the explanations"), ("la puntualidad", "the punctuality")],
                         tono="relleno"),
        bloque_cta(("¿Te hacemos un hueco?", "Shall we find you a slot?"),
                   ("Cuéntanos qué te pasa y te proponemos día y hora para la primera sesión.",
                    "Tell us what is wrong and we will suggest a day and time for the first session."),
                   [reservar(), whatsapp("ghost")], tono="base"),
    ])

    trat = pagina("tratamientos", ("tratamientos", "treatments"), ("Tratamientos", "Treatments"),
                  ("Qué trata la clínica de fisioterapia: lesiones, dolor de espalda, fisioterapia deportiva, suelo pélvico y tratamiento a domicilio.",
                   "What the physiotherapy clinic treats: injuries, back pain, sports physiotherapy, pelvic floor and treatment at home."), [
        hero(("Tratamientos", "Treatments"),
             ("Cada persona llega distinta. Por eso no hay un protocolo único: te valoramos y adaptamos el tratamiento.",
              "Everyone arrives different. That is why there is no single protocol: we assess you and tailor the treatment."),
             [reservar()], variante="texto"),
        bloque_valores(("En qué podemos ayudarte", "How we can help"), [
            (("Lesiones", "Injuries"),
             ("Esguinces, roturas musculares, tendinitis y recuperación tras una operación o una inmovilización.",
              "Sprains, muscle tears, tendinitis and recovery after an operation or immobilisation.")),
            (("Dolor de espalda y cuello", "Back and neck pain"),
             ("Lumbalgia, cervicalgia, ciática y contracturas. Trabajamos el dolor y también lo que lo mantiene.",
              "Lower back pain, neck pain, sciatica and muscle knots. We work on the pain and on what sustains it.")),
            (("Fisioterapia deportiva", "Sports physiotherapy"),
             ("Recuperación de lesiones, readaptación al entrenamiento y prevención.",
              "Injury recovery, return to training and prevention.")),
            (("Suelo pélvico", "Pelvic floor"),
             ("Embarazo, posparto, pérdidas de orina y dolor pélvico. Con valoración individual y en privado.",
              "Pregnancy, postpartum, urine leakage and pelvic pain. With individual assessment, in private.")),
            (("A domicilio", "At home"),
             ("El mismo tratamiento, en tu casa, para quien no puede desplazarse.",
              "The same treatment, at your home, for those who cannot travel.")),
            (("Ejercicio terapéutico", "Therapeutic exercise"),
             ("Una pauta corta para hacer en casa entre sesiones, que se ajusta según cómo vayas.",
              "A short routine to do at home between sessions, adjusted as you progress.")),
        ], cols=3, estilo="relleno", anchor="areas"),
        bloque_checks(("Cuándo conviene ver antes a un médico", "When to see a doctor first"), [
            ("Dolor fuerte tras un golpe o una caída", "Severe pain after a knock or a fall"),
            ("Hormigueo o pérdida de fuerza que va a más", "Tingling or loss of strength that is getting worse"),
            ("Dolor que no cede ni descansando por la noche", "Pain that does not ease even when resting at night"),
            ("Dolor acompañado de fiebre o de pérdida de peso sin causa", "Pain with a fever or unexplained weight loss"),
        ], intro=(URGENCIA[0] + " La fisioterapia no sustituye al diagnóstico médico: si algo no es de nuestra competencia, te lo decimos y te derivamos.",
                  URGENCIA[1] + " Physiotherapy does not replace a medical diagnosis: if something is outside our remit, we tell you and refer you on."),
            tono="base", cols=2, kind="features"),
        bloque_faq(("Preguntas frecuentes", "Frequently asked questions"), [
            (("¿Cuántas sesiones necesitaré?", "How many sessions will I need?"),
             ("Depende de cada caso. Tras la valoración te damos una estimación y la revisamos en cada sesión. No podemos prometerte un número ni un plazo.",
              "It depends on each case. After the assessment we give you an estimate and review it at every session. We cannot promise you a number or a timescale.")),
            (("¿Necesito un informe médico?", "Do I need a medical report?"),
             ("No para venir. Si lo tienes, o tienes pruebas como una resonancia o una radiografía, tráelo: nos ayuda.",
              "Not to come. If you have one, or scans such as an MRI or X-ray, bring them: they help us.")),
            (("¿Duele el tratamiento?", "Does the treatment hurt?"),
             ("Puede haber alguna molestia en ciertas técnicas, pero no tiene que ser doloroso. Dinos cuándo es demasiado y lo adaptamos.",
              "Some techniques can be uncomfortable, but they should not be painful. Tell us when it is too much and we adapt.")),
            (("¿Qué ropa llevo?", "What should I wear?"),
             ("Ropa cómoda y que permita ver y tocar la zona. Te dejamos toalla si necesitas cubrirte.",
              "Comfortable clothes that let us see and reach the area. We provide a towel if you want to be covered.")),
        ], tono="relleno"),
        bloque_cta(("¿Quieres una primera valoración?", "Would you like us to assess you?"), None, [reservar(), whatsapp("ghost")], tono="base"),
    ])

    tarifas = pagina("tarifas", ("tarifas", "prices"), ("Tarifas y bonos", "Prices and vouchers"),
                     ("Precios orientativos de la sesión de fisioterapia, valoración inicial, suelo pélvico, domicilio y bonos de sesiones.",
                      "Indicative prices for physiotherapy sessions, initial assessment, pelvic floor, home visits and session vouchers."), [
        hero(("Tarifas y bonos", "Prices and vouchers"),
             ("Sabes lo que cuesta cada sesión antes de reservar. Si necesitas varias, los bonos salen más baratos.",
              "You know what each session costs before you book. If you need several, vouchers work out cheaper."),
             [reservar()], variante="texto"),
        bloque_carta(("Sesiones", "Sessions"), [
            (("Sesión individual", "Single session"), ("Precios de sesión", "Session prices"),
             [("Servicio", "Service"), ("Duración", "Length"), ("Precio", "Price")],
             [(("Primera valoración y tratamiento", "First assessment and treatment"), ("60 min", "60 min"), eur(55)),
              (("Sesión de seguimiento", "Follow-up session"), ("45 min", "45 min"), eur(45)),
              (("Sesión de fisioterapia deportiva", "Sports physiotherapy session"), ("45 min", "45 min"), eur(50)),
              (("Valoración de suelo pélvico", "Pelvic floor assessment"), ("60 min", "60 min"), eur(60)),
              (("Sesión a domicilio", "Home visit"), ("60 min", "60 min"), desde(65))]),
            (("Bonos de sesiones", "Session vouchers"), ("Precios de bonos", "Voucher prices"),
             [("Bono", "Voucher"), ("Sesiones", "Sessions"), ("Precio", "Price")],
             [(("Bono 5 sesiones", "5-session voucher"), ("5", "5"), eur(210)),
              (("Bono 10 sesiones", "10-session voucher"), ("10", "10"), eur(400))]),
        ], nota=NOTA, tono="base", anchor="precios"),
        bloque_texto(("Bonos, factura y seguros", "Vouchers, invoices and insurance"), [
            ("Un bono se paga entero al empezar y se gasta sesión a sesión. Cada bono indica su fecha de validez.",
             "A voucher is paid in full at the start and used up session by session. Each voucher states its validity date."),
            ("Te damos factura de cada sesión o bono. Si tu seguro de salud cubre fisioterapia, consúltalo con tu aseguradora.",
             "We give you an invoice for each session or voucher. If your health insurance covers physiotherapy, check with your insurer."),
        ], tono="relleno", anchor="bonos"),
        bloque_faq(("Preguntas sobre las tarifas", "Questions about prices"), [
            (("¿Cómo se paga?", "How do I pay?"),
             ("Al terminar cada sesión, con tarjeta o en efectivo. Los bonos se pagan al comprarlos.",
              "At the end of each session, by card or in cash. Vouchers are paid for when you buy them.")),
            (("¿Y si no puedo ir a una sesión?", "What if I cannot make a session?"),
             ("Avísanos con 24 horas y la movemos sin coste. Con menos aviso, la sesión puede contar como hecha.",
              "Let us know 24 hours ahead and we move it at no cost. With less notice, the session may count as used.")),
        ], tono="base"),
        bloque_cta(("¿Reservamos tu primera sesión?", "Shall we book your first session?"), None, [reservar(), whatsapp("ghost")], tono="relleno"),
    ])

    cont = pagina("contacto", ("contacto", "contact"), ("Contacto y cita", "Contact and appointments"),
                  ("Pide cita de fisioterapia por teléfono, WhatsApp o con el formulario. Horario, visitas a domicilio y cómo llegar.",
                   "Book a physiotherapy appointment by phone, WhatsApp or with the form. Opening hours, home visits and how to get here."), [
        hero(("Pide tu cita", "Book your appointment"),
             ("Llámanos, escríbenos por WhatsApp o rellena el formulario. Te confirmamos el mismo día.",
              "Call us, message us on WhatsApp or fill in the form. We confirm the same day."),
             [llamar(), whatsapp()], variante="texto"),
        bloque_contacto(("Cómo encontrarnos", "How to find us"),
                        ("Si necesitas acceso sin escalones o ayuda para entrar, avísanos al reservar.", "If you need step-free access or help getting in, tell us when you book."),
                        campos_cita(servicios=[("Lesión o dolor", "Injury or pain"), ("Espalda o cuello", "Back or neck"),
                                               ("Fisioterapia deportiva", "Sports physiotherapy"),
                                               ("Suelo pélvico", "Pelvic floor"), ("A domicilio", "Home visit")],
                                    extra=[campo("zona", "text", ("Zona o motivo (opcional)", "Area or reason (optional)"),
                                                 False, maxlen=200)]),
                        ("Pedir cita", "Request appointment"), horas,
                        notas=(COLEGIADO[0] + ". Tus datos de salud se tratan con confidencialidad y según la normativa de protección de datos.",
                               COLEGIADO[1] + ". Your health data is handled in confidence and in line with data protection rules."),
                        exito=("Gracias. Te confirmamos la cita por teléfono o WhatsApp en cuanto la tengamos.",
                               "Thank you. We will confirm your appointment by phone or WhatsApp as soon as we have it.")),
        bloque_mapa(tono="relleno"),
    ])

    footer = pie(nav, ("Clínica de fisioterapia con cita previa. Lesiones, espalda, deporte, suelo pélvico y domicilio.",
                       "Physiotherapy clinic by appointment. Injuries, back, sport, pelvic floor and home visits."),
                 ("Lunes a viernes 9:00 a 14:00 y 16:00 a 20:00.", "Monday to Friday 9 am to 2 pm and 4 pm to 8 pm."))
    footer["children"].insert(1, peq(COLEGIADO, color="$muted"))

    return plantilla(KEY, ("Clínica de fisioterapia", "Physiotherapy clinic"),
                     ("Fisioterapia con tipos de tratamiento, primera sesión explicada, tarifas y bonos orientativos, equipo colegiado, visitas a domicilio y cita por WhatsApp.",
                      "Physiotherapy with treatment types, first session explained, indicative prices and vouchers, registered team, home visits and WhatsApp booking."),
                     "salud", ("fisioterapeuta fisioterapia fisio clinica de fisioterapia lesiones dolor de espalda lumbalgia deportiva suelo pelvico domicilio bonos sesiones",
                               "physiotherapist physiotherapy physio clinic injuries back pain sports pelvic floor home visits vouchers"),
                     look, cabecera(nav, llamar(t=("Llamar", "Call"))), footer, [inicio, trat, tarifas, cont])
