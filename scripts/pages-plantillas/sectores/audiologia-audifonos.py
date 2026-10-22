# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
from lib import *
from looks import LOOKS

KEY = "audiologia-audifonos"

# Paleta propia: azul ultramar profundo sobre blanco azulado (ninguna otra plantilla lo usa).
ULTRAMAR = dict(background="#f8f9fe", surface="#e7eaf8", text="#12143a", muted="#474b76", primary="#2b1fa8",
                on_primary="#ffffff", border="#d4d8ee", inverse="#14125a", on_inverse="#eef0ff")

DATOS = ("Tus datos y los resultados de tus pruebas se tratan con confidencialidad y según la normativa de protección de datos; solo acceden a ellos las personas que te atienden.",
         "Your details and test results are handled in confidence and in line with data protection rules; only the people treating you can see them.")

NOTA = ("Precios de ejemplo: cámbialos por los tuyos. Revisa también la prueba gratuita y las condiciones de financiación.",
        "Example prices: replace them with your own. Also check the free test and the financing terms.")


def _junta(*ts):
    return (" ".join(t[0] for t in ts), " ".join(t[1] for t in ts))


def _equipo():
    fichas = [(("Audioprotesista", "Hearing aid audiologist"),
               ("Hace la prueba, te explica el resultado y te propone qué audífono puede encajarte.",
                "Carries out the test, explains the result and suggests which hearing aid might suit you.")),
              (("Audiólogo/a", "Audiologist"),
               ("Hace los ajustes y las revisiones, y resuelve las dudas del día a día.",
                "Does the adjustments and check-ups, and answers everyday questions."))]
    li = [marco([imagen("portrait", ("Foto de la persona del equipo", "Photo of the team member"), ratio=1.0),
                 h3(("Nombre Apellido", "First Last")), peq(rol, color="$muted"), p(frase),
                 peq(COLEGIADO, color="$muted")], tag="li", gap=10) for rol, frase in fichas]
    return seccion("team", [titular(("Quién te atiende", "Who looks after you")), rejilla(li, cols=2)],
                   tono="relleno", anchor="equipo")


def build():
    nueva(KEY)
    look = dict(LOOKS["cobalto"], colores=ULTRAMAR, fuentes=("ibm-plex-sans", "dm-sans"), radio=(2, 6, 10), ancho=1160)
    nav = [("inicio", ("Inicio", "Home")), ("prueba", ("Prueba auditiva", "Hearing test")),
           ("audifonos", ("Audífonos y ajustes", "Hearing aids and fitting")), ("contacto", ("Contacto", "Contact"))]
    horas = [(("Lunes a viernes", "Monday to Friday"),
              ("9:30 a 13:30 y 16:00 a 19:30", "9:30 am to 1:30 pm and 4:00 pm to 7:30 pm")),
             (("Sábado", "Saturday"), ("Con cita previa", "By appointment")),
             (("Domingo", "Sunday"), ("Cerrado", "Closed"))]
    cita = lambda v="primary": boton(("Pedir cita", "Book an appointment"), a_pagina("contacto", "contacto"), v)

    inicio = pagina("inicio", ("", ""), ("Centro auditivo", "Hearing centre"),
                    ("Centro auditivo con cita previa: prueba auditiva, audífonos explicados en llano, revisiones y ajustes, protectores a medida y financiación. Llama o escribe por WhatsApp.",
                     "Hearing centre by appointment: hearing test, hearing aids explained plainly, check-ups and adjustments, custom ear protection and financing. Call or message us on WhatsApp."), [
        hero(("Oír bien también es cuestión de revisarlo", "Hearing well starts with having it checked"),
             ("Una prueba auditiva sencilla y sin dolor te dice cómo oyes hoy. Si hace falta un audífono, te lo explicamos con calma y vas probando antes de decidir.",
              "A simple, painless hearing test tells you how you hear today. If a hearing aid is needed, we explain it calmly and you try it before you decide."),
             [llamar(), whatsapp()], eyebrow=("Centro auditivo", "Hearing centre"),
             imagen_=("portrait", ("Audioprotesista explicando un resultado a un paciente con un audiograma", "Audiologist explaining a result to a patient with an audiogram"))),
        bloque_cta(("Prueba auditiva gratuita (texto de ejemplo)", "Free hearing test (example text)"),
                   ("De ejemplo: «Pide tu prueba auditiva gratuita, sin compromiso». Quítalo si no la ofreces o cambia las condiciones.",
                    "Example: «Book your free hearing test, no obligation». Remove it if you do not offer it, or change the terms."),
                   [cita(), whatsapp("ghost")], tono="relleno"),
        bloque_checks(("Señales de que conviene revisar el oído", "Signs it is worth having your hearing checked"), [
            ("Subes el volumen de la tele más que el resto.", "You turn the TV up louder than everyone else."),
            ("Pides que te repitan, sobre todo en sitios con ruido.", "You ask people to repeat themselves, especially in noisy places."),
            ("Entiendes las voces pero no las palabras.", "You hear voices but not the words."),
            ("Evitas reuniones o restaurantes porque te cansa seguir la conversación.", "You avoid meetings or restaurants because following the conversation tires you."),
            ("Notas un pitido o un zumbido en los oídos.", "You notice a whistling or buzzing in your ears."),
            ("Te dicen en casa que no oyes el timbre o el teléfono.", "People at home say you do not hear the doorbell or the phone."),
        ], intro=("Si te reconoces en alguna, pide una prueba. Es corta y no hace falta preparación.",
                  "If you recognise yourself in any of these, book a test. It is short and needs no preparation."),
            cols=2, tono="base"),
        bloque_pasos(("Cómo es la prueba auditiva", "What the hearing test is like"), [
            (("Hablamos", "We talk"),
             ("Cómo oyes, cuándo te cuesta y si has tenido ruidos, infecciones o mareos.", "How you hear, when it is hard and whether you have had noise, infections or dizziness.")),
            (("Miramos el oído", "We look in your ear"),
             ("Una mirada al conducto para descartar un tapón de cera.", "A look into the canal to rule out earwax.")),
            (("Te hacemos la prueba", "We do the test"),
             ("Con unos auriculares, pulsas un botón cada vez que oyes un pitido. No duele.", "With headphones, you press a button each time you hear a beep. It does not hurt.")),
            (("Te explicamos el resultado", "We explain the result"),
             ("Con un gráfico sencillo te decimos qué sonidos oyes peor y qué opciones hay, incluida la de no hacer nada.",
              "With a simple chart we tell you which sounds you hear less well and what the options are, including doing nothing.")),
        ], intro=("Dura unos 30 a 45 minutos en total.", "It takes about 30 to 45 minutes in total."),
            tono="relleno", anchor="la-prueba"),
        bloque_texto(("Si algo va rápido, no esperes", "If something changes quickly, do not wait"), [
            ("Si pierdes audición de golpe en un oído, tienes un dolor fuerte, secreción o mareo intenso, acude a urgencias o llama a tu médico antes de pedir cita con nosotros.",
             "If you lose hearing suddenly in one ear, or have severe pain, discharge or strong dizziness, go to A&E or call your doctor before booking with us."),
            ("Nosotros no diagnosticamos enfermedades del oído: si vemos algo que lo requiere, te derivamos a tu médico o al otorrino.",
             "We do not diagnose ear conditions: if we see something that needs it, we refer you to your doctor or an ENT specialist."),
        ], tono="base"),
        _equipo(),
        bloque_opiniones(("Lo que dicen nuestros pacientes", "What our patients say"),
                         [("la prueba", "the test"), ("las explicaciones", "the explanations"), ("las revisiones", "the check-ups")],
                         tono="base"),
        bloque_cta(("¿Te damos cita?", "Shall we book you in?"),
                   ("Dinos qué día te viene bien y te confirmamos. Si vienes con alguien de confianza, mejor.",
                    "Tell us which day suits you and we will confirm. If you bring someone you trust, even better."),
                   [cita(), whatsapp("ghost")], tono="relleno"),
    ])

    prueba = pagina("prueba", ("prueba-auditiva", "hearing-test"), ("Prueba auditiva", "Hearing test"),
                    ("Cómo es una prueba auditiva, cuánto dura, cómo leer el resultado, qué hacer después y preguntas frecuentes.",
                     "What a hearing test is like, how long it takes, how to read the result, what to do next and frequently asked questions."), [
        hero(("Prueba auditiva", "Hearing test"),
             ("Qué pasa desde que entras hasta que sales. Sin pruebas dolorosas y sin obligación de comprar nada.",
              "What happens from the moment you arrive until you leave. No painful tests and no obligation to buy anything."),
             [cita(), llamar("secondary")], variante="texto"),
        bloque_texto(("Qué incluye", "What it includes"), [
            ("Una conversación sobre cómo oyes, la revisión del conducto del oído y la prueba con auriculares. Cada oído por separado, con sonidos de distinto tono.",
             "A conversation about how you hear, a look inside the ear canal and the test with headphones. Each ear separately, with sounds of different pitch."),
            ("Si hace falta, también comprobamos cuánto entiendes de las palabras con y sin ruido de fondo.",
             "If needed, we also check how much you understand of words with and without background noise."),
        ], tono="relleno", anchor="que-incluye", imagen_=("photo", ("Cabina de pruebas auditivas con auriculares y mando", "Hearing test booth with headphones and response button"))),
        bloque_valores(("Cómo leer el resultado", "How to read the result"), [
            (("El audiograma", "The audiogram"),
             ("Es un gráfico con los sonidos graves a la izquierda y los agudos a la derecha. Cuanto más abajo está la línea, más fuerte tiene que sonar para que lo oigas.",
              "A chart with low sounds on the left and high sounds on the right. The lower the line, the louder a sound has to be for you to hear it.")),
            (("Qué te decimos", "What we tell you"),
             ("Qué sonidos oyes peor y cómo puede notarse en el día a día: voces, tele, conversaciones en grupo.",
              "Which sounds you hear less well and how it can show up in daily life: voices, TV, group conversations.")),
            (("Qué puedes hacer", "What you can do"),
             ("Revisar de nuevo más adelante, probar un audífono o seguir como estás. La decisión es tuya y no hay prisa.",
              "Have another check later, try a hearing aid or carry on as you are. The decision is yours and there is no rush.")),
        ], cols=3, estilo="borde", tono="base"),
        bloque_checks(("Antes de venir", "Before you come"), [
            ("Si usas audífonos ahora, tráelos.", "If you wear hearing aids now, bring them."),
            ("Si puedes, ven con un familiar o amigo.", "If you can, come with a relative or friend."),
            ("Un informe del otorrino, si lo tienes.", "A report from your ENT specialist, if you have one."),
            ("No hace falta ir en ayunas ni hacer preparación.", "No fasting or preparation is needed."),
        ], cols=2, tono="relleno"),
        bloque_faq(("Preguntas frecuentes", "Frequently asked questions"), [
            (("¿Duele?", "Does it hurt?"),
             ("No. Solo hay que escuchar y pulsar un botón.", "No. You just listen and press a button.")),
            (("¿Es de verdad gratuita?", "Is it really free?"),
             ("De ejemplo: «Sí, la prueba no tiene coste y no te obliga a nada». Confirma esta frase con tu centro antes de publicar.",
              "Example: «Yes, the test is free and does not oblige you to anything». Confirm this sentence with your centre before publishing.")),
            (("¿Tengo que decidir ese mismo día?", "Do I have to decide on the same day?"),
             ("No. Te explicamos el resultado y puedes llevártelo a casa para pensarlo.",
              "No. We explain the result and you can take it home to think about it.")),
            (("¿Sirve la prueba si tengo cera?", "Does the test work if I have earwax?"),
             ("Si hay un tapón, no se puede medir bien. Te lo decimos y te indicamos cómo y dónde quitarlo.",
              "If there is a plug of wax it cannot be measured properly. We tell you and explain how and where to have it removed.")),
        ], tono="base"),
        bloque_cta(("¿Quieres saber cómo oyes?", "Would you like to know how you hear?"), None,
                   [cita(), whatsapp("ghost")], tono="relleno"),
    ])

    aud = pagina("audifonos", ("audifonos-y-ajustes", "hearing-aids-and-fitting"), ("Audífonos y ajustes", "Hearing aids and fitting"),
                 ("Tipos de audífono explicados en llano, qué esperar al adaptarte, revisiones y ajustes, protectores a medida, precios orientativos y financiación.",
                  "Types of hearing aid explained plainly, what to expect as you adjust, check-ups and fine-tuning, custom ear protection, indicative prices and financing."), [
        hero(("Audífonos y ajustes", "Hearing aids and fitting"),
             ("Qué tipos hay, cuál puede ir contigo y cómo es el primer mes. Probar antes de decidir y revisarlo después, parte del servicio.",
              "What types there are, which might suit you and what the first month is like. Trying before you decide and check-ups afterwards are part of the service."),
             [cita(), llamar("secondary")], variante="centro", eyebrow=("Audífonos", "Hearing aids")),
        bloque_precios(("Tipos de audífono, en llano", "Types of hearing aid, in plain words"),
                       ("Tipos de audífono: dónde van y cuándo suelen encajar", "Types of hearing aid: where they go and when they usually suit"),
                       [("Tipo", "Type"), ("Dónde va", "Where it sits"), ("Suele encajar si...", "Often suits if..."), ("Ten en cuenta", "Bear in mind")],
                       [(("Detrás de la oreja", "Behind the ear"),
                         ("Un cuerpo detrás de la oreja y un tubito fino al oído", "A body behind the ear and a thin tube into the ear"),
                         ("Quieres algo fácil de manejar, o la pérdida es más grande", "You want something easy to handle, or the loss is larger"),
                         ("Se ve algo más, aunque hay modelos pequeños", "Slightly more visible, although there are small models")),
                        (("Con auricular en el conducto", "Receiver in the canal"),
                         ("Cuerpo pequeño detrás de la oreja y el altavoz dentro del oído", "A small body behind the ear and the speaker inside the ear"),
                         ("Quieres algo discreto y con buen sonido", "You want something discreet with good sound"),
                         ("Hay que cambiar de vez en cuando la cúpula o el filtro", "The dome or filter needs changing from time to time")),
                        (("A medida, dentro del oído", "Custom, in the ear"),
                         ("Se hace con la forma de tu oído y va dentro", "Made to the shape of your ear and sits inside"),
                         ("Prefieres que casi no se vea", "You prefer that it is barely visible"),
                         ("Más pequeño, con menos botones y baterías de poca duración", "Smaller, with fewer controls and shorter battery life")),
                        (("Recargable", "Rechargeable"),
                         ("Detrás de la oreja o en el conducto, con batería", "Behind the ear or in the canal, with a battery"),
                         ("No quieres cambiar pilas", "You do not want to change batteries"),
                         ("Hay que ponerlo a cargar cada noche", "It needs to be charged every night"))],
                       intro=("No hay un tipo mejor que otro. Depende de tu pérdida, de tu oído, de tus manos y de cómo vives. Te lo recomendamos tras la prueba.",
                              "No type is better than the others. It depends on your loss, your ear, your hands and how you live. We recommend one after the test."),
                       tono="relleno", anchor="tipos"),
        bloque_pasos(("Qué esperar al adaptarte", "What to expect as you adjust"), [
            (("Los primeros días", "The first days"),
             ("Los sonidos del día a día, como pasos o el frigorífico, pueden resultar fuertes. Es normal: el oído se reacostumbra.",
              "Everyday sounds such as footsteps or the fridge can seem loud. That is normal: your ear gets used to it again.")),
            (("Poco a poco", "Gradually"),
             ("Empieza por unas horas al día en casa y ve ampliando. Después, en sitios con más gente.",
              "Start with a few hours a day at home and build up. Then try places with more people.")),
            (("Primer ajuste", "First adjustment"),
             ("Volvemos a verte para retocar el sonido con lo que has notado. Anota lo que te molesta.",
              "We see you again to fine-tune the sound with what you have noticed. Note down what bothers you.")),
            (("Revisiones", "Check-ups"),
             ("Limpiamos, comprobamos y ajustamos de vez en cuando. Si algo cambia, vuelve antes.",
              "We clean, check and adjust from time to time. If anything changes, come back sooner.")),
        ], intro=("Oír con audífonos es un proceso: suele llevar semanas acostumbrarse del todo.",
                  "Hearing with hearing aids is a process: it often takes weeks to get fully used to them."),
            tono="base", anchor="adaptacion"),
        bloque_valores(("Protectores a medida", "Custom ear protection"), [
            (("Para el ruido en el trabajo", "For noise at work"),
             ("Taller, obra, fábrica o máquinas. Se hacen con el molde de tu oído para que sellen bien y no molesten.",
              "Workshop, building site, factory or machinery. Made from a mould of your ear so they seal well and are comfortable.")),
            (("Para músicos y conciertos", "For musicians and concerts"),
             ("Bajan el volumen sin apagar el sonido, para oír la música con claridad.", "They reduce volume without muffling sound, so you hear the music clearly.")),
            (("Para el agua y para dormir", "For water and for sleeping"),
             ("Moldes para nadar o para descansar sin ruido. Te asesoramos sobre el más adecuado.", "Moulds for swimming or for resting without noise. We advise on the most suitable.")),
        ], cols=3, estilo="relleno", tono="relleno", anchor="protectores"),
        bloque_carta(("Precios orientativos", "Indicative prices"), [
            (("Pruebas, audífonos y revisiones", "Tests, hearing aids and check-ups"), ("Precios de pruebas, audífonos y revisiones", "Prices for tests, hearing aids and check-ups"),
             [("Servicio", "Service"), ("Precio", "Price")],
             [(("Prueba auditiva", "Hearing test"), ("Gratuita (ejemplo)", "Free (example)")),
              (("Audífono, por unidad", "Hearing aid, each"), desde(690)),
              (("Revisión y limpieza de audífonos", "Hearing aid check and clean"), eur(25)),
              (("Protectores a medida (par)", "Custom ear protection (pair)"), desde(45)),
              (("Moldes para natación (par)", "Swim moulds (pair)"), desde(35))]),
        ], nota=NOTA, tono="base", anchor="precios"),
        bloque_texto(("Si el audífono es una inversión", "If a hearing aid is a big outlay"), [
            ("Pregunta por las opciones de pago aplazado y por ayudas públicas o de tu seguro. Las condiciones te las damos por escrito antes de empezar, con el importe de cada cuota y el total.",
             "Ask about deferred payment options and about public or insurance support. We give you the terms in writing before we start, with the amount of each instalment and the total."),
            ("No vendemos online: el audífono se prueba y se ajusta en persona.",
             "We do not sell online: a hearing aid is tried and adjusted in person."),
        ], tono="relleno", anchor="financiacion"),
        bloque_cta(("¿Quieres probar un audífono?", "Would you like to try a hearing aid?"),
                   ("Empieza por la prueba auditiva. Después hablamos de opciones.", "Start with the hearing test. Then we talk about options."),
                   [cita(), whatsapp("ghost")], tono="base"),
    ])

    cont = pagina("contacto", ("contacto", "contact"), ("Contacto y cita", "Contact and appointments"),
                  ("Pide cita en el centro auditivo por teléfono, WhatsApp o formulario. Horario y cómo llegar.",
                   "Book at the hearing centre by phone, WhatsApp or form. Opening hours and how to get here."), [
        hero(("Pide tu cita", "Book your appointment"),
             ("Llámanos, escríbenos por WhatsApp o rellena el formulario. Te confirmamos el mismo día.",
              "Call us, message us on WhatsApp or fill in the form. We confirm the same day."),
             [llamar(), whatsapp()], variante="texto"),
        bloque_contacto(("Cómo encontrarnos", "How to find us"),
                        ("Local a pie de calle, sin escalones.", "Street-level premises, no steps."),
                        campos_cita(servicios=[("Prueba auditiva", "Hearing test"), ("Probar un audífono", "Try a hearing aid"),
                                               ("Revisión o ajuste", "Check-up or adjustment"), ("Protectores a medida", "Custom ear protection"),
                                               ("Otro motivo", "Something else")],
                                    extra=[campo("audifono", "select", ("¿Usas audífonos ahora?", "Do you wear hearing aids now?"), False,
                                                 opciones=[("No", "No"), ("Sí", "Yes")])]),
                        ("Pedir cita", "Request appointment"), horas,
                        notas=_junta(COLEGIADO, DATOS),
                        exito=("Gracias. Te confirmamos la cita por teléfono o WhatsApp en cuanto la tengamos.",
                               "Thank you. We will confirm your appointment by phone or WhatsApp as soon as we have it.")),
        bloque_mapa(tono="relleno"),
    ])

    footer = pie(nav, ("Centro auditivo con cita previa. Pruebas, audífonos y protectores a medida.",
                       "Hearing centre by appointment. Tests, hearing aids and custom ear protection."),
                 ("Lunes a viernes 9:30 a 13:30 y 16:00 a 19:30. Sábado con cita.",
                  "Monday to Friday 9:30 am to 1:30 pm and 4 pm to 7:30 pm. Saturday by appointment."))
    footer["children"].insert(1, peq(COLEGIADO, color="$muted"))

    return plantilla(KEY, ("Centro auditivo", "Hearing centre"),
                     ("Centro auditivo con prueba auditiva explicada, tipos de audífono en llano, adaptación, revisiones, protectores a medida, equipo colegiado y cita por WhatsApp.",
                      "Hearing centre with the hearing test explained, types of hearing aid in plain words, adjusting, check-ups, custom ear protection, registered team and WhatsApp booking."),
                     "salud", ("audiologia audifonos audifono centro auditivo audioprotesista prueba auditiva sordera hipoacusia protectores auditivos a medida oido oir",
                               "audiology hearing aids hearing aid hearing centre audiologist hearing test hearing loss earplugs ear protection custom"),
                     look, cabecera(nav, marco([llamar(), whatsapp()], dir="row", gap=12, wrap=True)), footer,
                     [inicio, prueba, aud, cont])
