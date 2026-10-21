# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
from lib import *
from looks import LOOKS  # noqa: F401  (paleta propia más abajo)

KEY = "osteopata-quiropractico"

# Piedra cálida con azul pizarra suave: calma de consulta, sin verde clínico ni teal.
PIEDRA = dict(background="#f7f4ef", surface="#ebe6dd", text="#1f2328", muted="#585d66", primary="#4a5a8c",
              on_primary="#ffffff", border="#d9d3c7", inverse="#232a3d", on_inverse="#f1f2f7")

NOTA = ("Precios de ejemplo: cámbialos por los tuyos. Incluyen IVA si procede.",
        "Example prices: replace them with your own. VAT included where it applies.")
AVISO = ("La osteopatía y la quiropráctica no sustituyen a tu médico ni a un diagnóstico médico. Si algo no es de nuestra competencia, te lo decimos y te derivamos.",
         "Osteopathy and chiropractic do not replace your doctor or a medical diagnosis. If something is outside our remit, we tell you and refer you on.")


def _equipo():
    """Fichas con la titulación como marcador: se quita si no procede en tu caso."""
    def ficha(rol, frase):
        return marco([imagen("portrait", ("Foto de la persona del equipo", "Photo of the team member"), ratio=0.9),
                      h3(("Nombre Apellido", "First Last")), peq(rol, color="$muted"), p(frase),
                      peq(COLEGIADO, color="$muted")], tag="li", gap=10)
    return seccion("team", [
        titular(("Quién te atiende", "Who treats you"),
                ("Sabes quién te va a tratar antes de venir y puedes preguntarle lo que quieras.",
                 "You know who will treat you before you come and can ask them anything.")),
        rejilla([ficha(("Osteópata", "Osteopath"),
                       ("Valoración global y tratamiento manual, adaptado a cómo estés cada día.",
                        "Whole-body assessment and hands-on treatment, adapted to how you are each day.")),
                 ficha(("Quiropráctico", "Chiropractor"),
                       ("Valoración de columna y articulaciones, con ajustes solo si tú quieres y es adecuado.",
                        "Spine and joint assessment, with adjustments only if you want them and they are suitable."))],
                cols=2),
    ], tono="base", anchor="equipo")


def build():
    nueva(KEY)
    look = dict(colores=PIEDRA, fuentes=("source-serif-4", "manrope"), peso=600, radio=(4, 8, 14), ancho=1120)
    nav = [("inicio", ("Inicio", "Home")), ("consulta", ("La consulta", "The practice")),
           ("tarifas", ("Tarifas", "Prices")), ("contacto", ("Contacto", "Contact"))]
    horas = [(("Lunes a viernes", "Monday to Friday"),
              ("9:00 a 14:00 y 16:00 a 20:00", "9:00 am to 2:00 pm and 4:00 pm to 8:00 pm")),
             (("Sábado", "Saturday"), ("Cerrado", "Closed")),
             (("Domingo", "Sunday"), ("Cerrado", "Closed"))]

    def reservar(v="primary"):
        return boton(("Pedir cita", "Book a session"), a_pagina("contacto", "contacto"), v)

    inicio = pagina("inicio", ("", ""), ("Osteopatía y quiropráctica", "Osteopathy and chiropractic"),
                    ("Consulta de osteopatía y quiropráctica con cita previa: motivos de consulta, primera sesión explicada y tarifas claras. Llama o escribe por WhatsApp.",
                     "Osteopathy and chiropractic practice by appointment: common reasons to visit, first session explained and clear prices. Call or message us on WhatsApp."), [
        hero(("Una consulta donde se te escucha antes de tocarte", "A practice where we listen before we touch"),
             ("Osteopatía y quiropráctica con cita previa. Primero hablamos de lo que te pasa y después decidimos juntos qué hacer, sin prisa y sin compromiso de volver.",
              "Osteopathy and chiropractic by appointment. First we talk about what is going on, then we decide together what to do, unhurried and with no obligation to return."),
             [reservar(), whatsapp()],
             eyebrow=("Osteopatía y quiropráctica", "Osteopathy and chiropractic"),
             imagen_=("landscape", ("Sala de consulta con una camilla y luz natural", "Treatment room with a couch and natural light"))),
        bloque_valores(("Con qué suele venir la gente", "What people usually come with"), [
            (("Espalda y cuello", "Back and neck"),
             ("Dolor lumbar, rigidez al levantarte, cuello cargado después de horas de ordenador.",
              "Lower back pain, stiffness when you get up, a tense neck after hours at a computer.")),
            (("Cabeza y mandíbula", "Head and jaw"),
             ("Dolores de cabeza que vienen de la tensión del cuello y molestias al masticar o apretar los dientes.",
              "Headaches linked to neck tension, and discomfort when chewing or clenching your teeth.")),
            (("Hombros, caderas y rodillas", "Shoulders, hips and knees"),
             ("Molestias al moverte o al hacer deporte, y esos dolores que aparecen sin un golpe.",
              "Aches when you move or exercise, and those pains that appear without a knock.")),
            (("Embarazo y posparto", "Pregnancy and postpartum"),
             ("Dolor de espalda, de pelvis o ciática durante o después del embarazo, con técnicas suaves.",
              "Back, pelvic or sciatic pain during or after pregnancy, with gentle techniques.")),
            (("Bebés y niños", "Babies and children"),
             ("Consulta con los padres presentes y técnicas muy suaves. Antes preguntamos por su pediatra.",
              "Sessions with parents present and very gentle techniques. We ask about their paediatrician first.")),
            (("Tensión y estrés", "Tension and stress"),
             ("Cuando el cuerpo se carga por el trabajo, el sueño o la postura y no sabes por dónde empezar.",
              "When your body gets loaded up by work, sleep or posture and you do not know where to start.")),
        ], eyebrow=("Motivos de consulta", "Reasons to visit"), cols=3, tono="relleno", estilo="borde"),
        bloque_pasos(("Cómo es la primera sesión", "What the first session is like"), [
            (("Te escuchamos", "We listen"),
             ("Qué te pasa, desde cuándo, qué has probado y qué te gustaría volver a hacer.",
              "What is wrong, since when, what you have tried and what you would like to do again.")),
            (("Te exploramos", "We examine you"),
             ("Miramos cómo te mueves y dónde está la molestia. Si lo necesitamos, te pedimos informes o pruebas anteriores.",
              "We look at how you move and where the problem is. If needed, we ask for earlier reports or scans.")),
            (("Tratamos", "We treat"),
             ("Con las manos, de forma suave y siempre con tu permiso. Si algo molesta, avísanos y paramos.",
              "With our hands, gently and always with your permission. If anything is uncomfortable, tell us and we stop.")),
            (("Te explicamos", "We explain"),
             ("Qué hemos visto, cuántas sesiones calculamos y qué hacer en casa. Tú decides si seguimos.",
              "What we found, how many sessions we estimate and what to do at home. You decide whether we carry on.")),
        ], intro=("Dura entre 45 y 60 minutos. Ven con ropa cómoda y trae informes, radiografías o resonancias si los tienes.",
                  "It lasts 45 to 60 minutes. Wear comfortable clothes and bring any reports or scans you have."),
            tono="base"),
        bloque_texto(("Lo que no hacemos", "What we do not do"), [
            AVISO,
            ("No prometemos curar ni un número de sesiones: lo que te damos es una estimación que revisamos en cada visita.",
             "We do not promise a cure or a set number of sessions: we give you an estimate that we review at every visit."),
        ], tono="relleno", anchor="limites"),
        _equipo(),
        bloque_opiniones(("Lo que dicen nuestros pacientes", "What our patients say"),
                         [("el trato", "the care"), ("las explicaciones", "the explanations"), ("la puntualidad", "the punctuality")],
                         tono="relleno"),
        bloque_cta(("¿Te hacemos un hueco?", "Shall we find you a slot?"),
                   ("Cuéntanos qué te pasa y te proponemos día y hora para la primera sesión.",
                    "Tell us what is wrong and we will suggest a day and time for the first session."),
                   [reservar(), whatsapp("ghost")], tono="base"),
    ])

    consulta = pagina("consulta", ("consulta", "practice"), ("La consulta", "The practice"),
                      ("Cómo trabajan la osteopatía y la quiropráctica en la consulta, cuándo ver antes a un médico y respuestas a las dudas más comunes.",
                       "How osteopathy and chiropractic work in the practice, when to see a doctor first and answers to the most common questions."), [
        hero(("La consulta", "The practice"),
             ("Dos formas de trabajar con las manos. Según lo que te pase, usamos una, la otra o una combinación, y siempre te explicamos por qué.",
              "Two ways of working with the hands. Depending on what is wrong, we use one, the other or a mix, and always explain why."),
             [reservar()], variante="texto"),
        bloque_valores(("Qué hacemos con las manos", "What we do with our hands"), [
            (("Osteopatía", "Osteopathy"),
             ("Técnicas suaves sobre músculos, articulaciones y tejidos para que el cuerpo se mueva con menos tensión. Miramos la zona que duele y lo que la rodea.",
              "Gentle techniques on muscles, joints and tissue so your body moves with less tension. We look at the sore area and what surrounds it.")),
            (("Quiropráctica", "Chiropractic"),
             ("Valoración de la columna y de las articulaciones, con ajustes breves y precisos cuando son adecuados para ti. Nunca sin tu consentimiento.",
              "Assessment of the spine and joints, with short, precise adjustments when they suit you. Never without your consent.")),
            (("Consejos y ejercicio", "Advice and exercise"),
             ("Pautas cortas de postura, movimiento y descanso para hacer en casa entre sesiones.",
              "Short routines on posture, movement and rest to do at home between sessions.")),
        ], cols=3, estilo="relleno", tono="base", anchor="tecnicas"),
        bloque_checks(("Cuándo conviene ver antes a un médico", "When to see a doctor first"), [
            ("Dolor fuerte tras una caída o un golpe", "Severe pain after a fall or a knock"),
            ("Hormigueo o pérdida de fuerza que va a más", "Tingling or loss of strength that is getting worse"),
            ("Dolor que no cede ni descansando por la noche", "Pain that does not ease even when resting at night"),
            ("Dolor con fiebre o con pérdida de peso sin causa", "Pain with a fever or unexplained weight loss"),
            ("Dolor en el pecho o falta de aire", "Chest pain or shortness of breath"),
        ], intro=("En una urgencia llama al 112 o acude a urgencias. " + AVISO[0],
                  "In an emergency call 112 or go to A&E. " + AVISO[1]),
            tono="relleno", cols=2, kind="features"),
        bloque_faq(("Preguntas frecuentes", "Frequently asked questions"), [
            (("¿Cuántas sesiones necesitaré?", "How many sessions will I need?"),
             ("Depende de cada persona. Tras la primera sesión te damos una estimación, y a veces una o dos bastan. No podemos prometer un número ni un plazo.",
              "It depends on the person. After the first session we give you an estimate, and sometimes one or two are enough. We cannot promise a number or a timescale.")),
            (("¿Duele el tratamiento?", "Does the treatment hurt?"),
             ("No tiene que doler. Alguna técnica puede resultar molesta; dínoslo y la cambiamos. Es normal notar algo de cansancio o agujetas uno o dos días después.",
              "It should not hurt. Some techniques can feel uncomfortable; tell us and we change them. Feeling tired or a little sore for a day or two afterwards is normal.")),
            (("¿Qué es ese «crujido» de las articulaciones?", "What is that “crack” in the joints?"),
             ("Es un gas que se libera dentro de la articulación al ajustarla. No es un hueso que se rompe, y no todos los ajustes lo hacen sonar.",
              "It is gas released inside the joint when it is adjusted. It is not a bone breaking, and not every adjustment makes the sound.")),
            (("¿Puedo ir si estoy embarazada o con un bebé?", "Can I come if I am pregnant or with a baby?"),
             ("Sí, con técnicas suaves y adaptadas. Para un bebé, preguntamos antes por su pediatra y estáis los dos presentes.",
              "Yes, with gentle, adapted techniques. For a baby, we ask about their paediatrician first and you stay with them throughout.")),
            (("¿Necesito un informe médico?", "Do I need a medical report?"),
             ("No para venir. Si tienes radiografías, resonancias o informes, tráelos: nos ayudan a decidir.",
              "Not to come. If you have X-rays, MRIs or reports, bring them: they help us decide.")),
        ], tono="base"),
        bloque_cta(("¿Quieres una primera sesión?", "Would you like a first session?"), None,
                   [reservar(), whatsapp("ghost")], tono="relleno"),
    ])

    tarifas = pagina("tarifas", ("tarifas", "prices"), ("Tarifas", "Prices"),
                     ("Precios orientativos de la primera sesión, las sesiones de seguimiento, embarazo, niños y bonos de osteopatía y quiropráctica.",
                      "Indicative prices for the first session, follow-ups, pregnancy, children and osteopathy and chiropractic vouchers."), [
        hero(("Tarifas", "Prices"),
             ("Sabes lo que cuesta cada sesión antes de reservar. No hay que contratar bonos ni un número de sesiones.",
              "You know what each session costs before you book. There is no need to buy vouchers or a set number of sessions."),
             [reservar()], variante="texto"),
        bloque_carta(("Sesiones y bonos", "Sessions and vouchers"), [
            (("Sesiones", "Sessions"), ("Precios de las sesiones", "Session prices"),
             [("Sesión", "Session"), ("Duración", "Length"), ("Precio", "Price")],
             [(("Primera sesión: valoración y tratamiento", "First session: assessment and treatment"), ("60 min", "60 min"), eur(60)),
              (("Sesión de seguimiento", "Follow-up session"), ("45 min", "45 min"), eur(50)),
              (("Embarazo y posparto", "Pregnancy and postpartum"), ("45 min", "45 min"), eur(50)),
              (("Niños y bebés", "Children and babies"), ("30 min", "30 min"), eur(40))]),
            (("Bonos", "Vouchers"), ("Precios de los bonos", "Voucher prices"),
             [("Bono", "Voucher"), ("Sesiones", "Sessions"), ("Precio", "Price")],
             [(("Bono de 3 sesiones", "3-session voucher"), ("3", "3"), eur(135)),
              (("Bono de 5 sesiones", "5-session voucher"), ("5", "5"), eur(220))]),
        ], nota=NOTA, tono="base", anchor="precios"),
        bloque_texto(("Cómo se paga", "How to pay"), [
            ("Pagas al terminar cada sesión, con tarjeta o en efectivo. Los bonos se pagan al comprarlos y llevan fecha de validez.",
             "You pay at the end of each session, by card or in cash. Vouchers are paid for when bought and have an expiry date."),
            ("Te damos factura de cada sesión. Si tu seguro de salud cubre este tipo de terapias, consúltalo con tu aseguradora.",
             "We give you an invoice for each session. If your health insurance covers this kind of therapy, check with your insurer."),
        ], tono="relleno", anchor="pago"),
        bloque_faq(("Preguntas sobre la cita", "Questions about appointments"), [
            (("¿Y si no puedo ir?", "What if I cannot make it?"),
             ("Avísanos con 24 horas y la movemos sin coste. Con menos aviso, la sesión puede cobrarse.",
              "Let us know 24 hours ahead and we move it at no cost. With less notice, the session may be charged.")),
            (("¿Cada cuánto tengo que volver?", "How often should I come back?"),
             ("Lo decidimos al final de cada sesión según cómo vayas. Muchas veces se espacian a medida que mejoras.",
              "We decide at the end of each session depending on how you are doing. Visits are often spaced further apart as you improve.")),
        ], tono="base"),
        bloque_cta(("¿Reservamos tu primera sesión?", "Shall we book your first session?"), None,
                   [reservar(), whatsapp("ghost")], tono="relleno"),
    ])

    cont = pagina("contacto", ("contacto", "contact"), ("Contacto y cita", "Contact and appointments"),
                  ("Pide cita de osteopatía o quiropráctica por teléfono, WhatsApp o con el formulario. Horario y cómo llegar a la consulta.",
                   "Book an osteopathy or chiropractic appointment by phone, WhatsApp or with the form. Opening hours and how to get to the practice."), [
        hero(("Pide tu cita", "Book your appointment"),
             ("Llámanos, escríbenos por WhatsApp o rellena el formulario. Te confirmamos el mismo día.",
              "Call us, message us on WhatsApp or fill in the form. We confirm the same day."),
             [llamar(), whatsapp()], variante="texto"),
        bloque_contacto(("Cómo encontrarnos", "How to find us"),
                        ("Si necesitas acceso sin escalones o vienes con un bebé, avísanos al reservar.",
                         "If you need step-free access or are coming with a baby, tell us when you book."),
                        campos_cita(servicios=[("Primera sesión", "First session"), ("Sesión de seguimiento", "Follow-up session"),
                                               ("Embarazo o posparto", "Pregnancy or postpartum"), ("Niño o bebé", "Child or baby")],
                                    extra=[campo("motivo", "text", ("Qué te pasa (opcional)", "What is wrong (optional)"),
                                                 False, maxlen=200)]),
                        ("Pedir cita", "Request appointment"), horas,
                        notas=("Tus datos de salud se tratan con confidencialidad y según la normativa de protección de datos.",
                               "Your health data is handled in confidence and in line with data protection rules."),
                        exito=("Gracias. Te confirmamos la cita por teléfono o WhatsApp en cuanto la tengamos.",
                               "Thank you. We will confirm your appointment by phone or WhatsApp as soon as we have it.")),
        bloque_mapa(tono="relleno"),
    ])

    footer = pie(nav, ("Consulta de osteopatía y quiropráctica con cita previa.",
                       "Osteopathy and chiropractic practice by appointment."),
                 ("Lunes a viernes 9:00 a 14:00 y 16:00 a 20:00.", "Monday to Friday 9 am to 2 pm and 4 pm to 8 pm."))

    return plantilla(KEY, ("Osteopatía y quiropráctica", "Osteopathy and chiropractic"),
                     ("Consulta de osteopatía y quiropráctica con motivos de consulta, primera sesión explicada, tarifas orientativas, equipo y cita por WhatsApp.",
                      "Osteopathy and chiropractic practice with reasons to visit, first session explained, indicative prices, team and WhatsApp booking."),
                     "salud", ("osteopata osteopatia quiropractico quiropractica quiropractor consulta de osteopatia espalda cuello lumbalgia cervicales cefaleas embarazo bebes ajustes",
                               "osteopath osteopathy chiropractor chiropractic practice back pain neck headaches pregnancy babies adjustments"),
                     look, cabecera(nav, llamar(t=("Llamar", "Call"))), footer, [inicio, consulta, tarifas, cont])
