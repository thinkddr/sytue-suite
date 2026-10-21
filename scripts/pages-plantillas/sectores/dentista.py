# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
from lib import *
from looks import LOOKS

KEY = "dentista"

DATOS = ("Tus datos y tu historial clínico se tratan con confidencialidad y según la normativa de protección de datos; solo acceden a ellos las personas que te atienden.",
         "Your details and clinical history are handled in confidence and in line with data protection rules; only the people treating you can see them.")


def _junta(*ts):
    return (" ".join(t[0] for t in ts), " ".join(t[1] for t in ts))


def _equipo(titulo, personas, intro=None, tono="base"):
    """Ficha del equipo con el número de colegiado visible (relleno detectable) en quien lo tiene."""
    tarjetas = []
    for rol, frase, colegiado in personas:
        tarjetas.append(marco([imagen("portrait", ("Foto de la persona del equipo", "Photo of the team member"), ratio=0.9),
                               h3(("Nombre Apellido", "First Last")), peq(rol, color="$muted"), p(frase),
                               peq(COLEGIADO, color="$muted") if colegiado else None], tag="li", gap=10))
    return seccion("team", [titular(titulo, intro), rejilla(tarjetas, cols=3)], tono=tono, anchor="equipo")


def build():
    nueva(KEY)
    look = LOOKS["menta"]
    nav = [("inicio", ("Inicio", "Home")), ("tratamientos", ("Tratamientos y precios", "Treatments and prices")),
           ("clinica", ("La clínica", "The clinic")), ("contacto", ("Contacto", "Contact"))]
    horas = [(("Lunes a jueves", "Monday to Thursday"),
              ("9:30 a 14:00 y 16:00 a 20:00", "9:30 am to 2:00 pm and 4:00 pm to 8:00 pm")),
             (("Viernes", "Friday"), ("9:30 a 15:00", "9:30 am to 3:00 pm")),
             (("Sábado y domingo", "Saturday and Sunday"), ("Cerrado", "Closed")),
             (("Urgencias", "Emergencies"),
              ("Llama y te damos hueco en horario de consulta", "Call and we will fit you in during opening hours"))]
    cita = lambda v="primary": boton(("Pedir cita", "Book an appointment"), a_pagina("contacto", "contacto"), v)
    nota = ("Precios de ejemplo: cámbialos por los tuyos. Revisa también las condiciones de financiación.",
            "Example prices: replace them with your own. Also check the financing terms.")

    inicio = pagina("inicio", ("", ""), ("Clínica dental", "Dental clinic"),
                    ("Clínica dental con cita previa: revisiones, limpiezas, empastes, ortodoncia, implantes y urgencias. Llama o escribe por WhatsApp.",
                     "Dental clinic by appointment: check-ups, cleanings, fillings, orthodontics, implants and emergencies. Call or message us on WhatsApp."), [
        hero(("Cuidamos tu boca, con calma", "We look after your teeth, calmly"),
             ("Revisiones, limpiezas, empastes, ortodoncia e implantes. En la primera visita te miramos la boca, te explicamos lo que vemos y te damos el presupuesto por escrito.",
              "Check-ups, cleanings, fillings, orthodontics and implants. At your first visit we examine your mouth, explain what we see and give you a written quote."),
             [llamar(), whatsapp()],
             eyebrow=("Clínica dental", "Dental clinic"),
             imagen_=("photo", ("Gabinete dental con sillón y lámpara", "Dental surgery with chair and light"))),
        bloque_cta(("¿Dolor, un golpe o un diente roto?", "Toothache, a knock or a broken tooth?"),
                   ("Llámanos y te damos hueco el mismo día siempre que podamos. Fuera de horario, si tienes hinchazón en la cara o te cuesta respirar o tragar, llama al 112.",
                    "Call us and we will fit you in the same day whenever we can. Outside opening hours, if your face is swelling or you have trouble breathing or swallowing, call 112."),
                   [llamar(), whatsapp("ghost")], tono="relleno"),
        bloque_checks(("Lo que hacemos", "What we do"), [
            ("Revisiones y limpiezas", "Check-ups and cleanings"),
            ("Empastes y arreglo de piezas rotas", "Fillings and repairs to broken teeth"),
            ("Endodoncias y extracciones", "Root canals and extractions"),
            ("Ortodoncia para niños y adultos", "Orthodontics for children and adults"),
            ("Implantes y prótesis", "Implants and prostheses"),
            ("Urgencias dentales", "Dental emergencies"),
        ], eyebrow=("Tratamientos", "Treatments"), cols=3, tono="base"),
        bloque_pasos(("Tu primera visita", "Your first visit"), [
            (("Hablamos", "We talk"),
             ("Nos cuentas qué te trae, qué molestias tienes y si tomas medicación o tienes alergias.",
              "You tell us why you came, what bothers you and whether you take medication or have allergies.")),
            (("Te exploramos", "We examine you"),
             ("Revisión completa de dientes y encías. Hacemos radiografías solo si hacen falta.",
              "A full check of teeth and gums. We take X-rays only if they are needed.")),
            (("Te explicamos", "We explain"),
             ("Qué vemos, qué opciones hay y qué pasa si lo dejas. No tienes que decidir nada ese día.",
              "What we see, what the options are and what happens if you leave it. You do not have to decide anything that day.")),
            (("Presupuesto por escrito", "A written quote"),
             ("Si hace falta un tratamiento, te damos el precio y las fases por escrito antes de empezar.",
              "If you need treatment, we give you the price and the stages in writing before we start.")),
        ], tono="relleno"),
        bloque_opiniones(("Lo que dicen nuestros pacientes", "What our patients say"),
                         [("el trato", "the care"), ("las explicaciones", "the explanations"), ("la puntualidad", "the punctuality")],
                         tono="base"),
        bloque_cta(("¿Te damos cita?", "Shall we book you in?"),
                   ("Dinos para qué la quieres y qué día te viene bien, y te confirmamos.",
                    "Tell us what it is for and which day suits you, and we will confirm."),
                   [cita(), whatsapp("ghost")]),
    ])

    trat = pagina("tratamientos", ("tratamientos", "treatments"), ("Tratamientos y precios", "Treatments and prices"),
                  ("Qué trata la clínica dental, precios orientativos de revisión, limpieza, empastes, ortodoncia e implantes, y cómo pagar un tratamiento largo.",
                   "What the dental clinic treats, indicative prices for check-ups, cleanings, fillings, orthodontics and implants, and how to pay for longer treatment."), [
        hero(("Tratamientos y precios", "Treatments and prices"),
             ("El precio final depende de lo que veamos en la revisión. Te lo damos por escrito y no empezamos nada sin tu conformidad.",
              "The final price depends on what we see at the check-up. We give it to you in writing and start nothing without your agreement."),
             [cita()], variante="texto"),
        bloque_valores(("Qué tratamos", "What we treat"), [
            (("Revisión y limpieza", "Check-up and cleaning"),
             ("Mirar dientes y encías, quitar el sarro y enseñarte a cepillarte mejor. Lo normal es una vez al año; tu dentista te dirá si necesitas más.",
              "Checking teeth and gums, removing tartar and showing you how to brush better. Once a year is usual; your dentist will tell you if you need more.")),
            (("Empastes y endodoncias", "Fillings and root canals"),
             ("Arreglamos caries y piezas rotas. Si el nervio está dañado, una endodoncia permite conservar el diente.",
              "We repair cavities and broken teeth. If the nerve is damaged, a root canal lets you keep the tooth.")),
            (("Ortodoncia", "Orthodontics"),
             ("Brackets o alineadores transparentes, para niños y adultos. La duración depende de cada boca y se valora en la revisión.",
              "Braces or clear aligners, for children and adults. How long it takes depends on each mouth and is assessed at the check-up.")),
            (("Implantes y prótesis", "Implants and prostheses"),
             ("Para sustituir dientes que faltan. Antes valoramos con radiografía si es posible y te explicamos las alternativas.",
              "To replace missing teeth. First we assess with an X-ray whether it is possible and explain the alternatives.")),
            (("Encías", "Gums"),
             ("Si sangran o notas los dientes flojos, lo revisamos y te decimos cómo tratarlo y cómo cuidarlo en casa.",
              "If they bleed or your teeth feel loose, we check them and tell you how to treat and care for them at home.")),
            (("Urgencias", "Emergencies"),
             ("Dolor fuerte, golpe, diente roto o salido. Llama y te damos hueco en horario de consulta.",
              "Severe pain, a knock, a broken or knocked-out tooth. Call and we will fit you in during opening hours.")),
        ], cols=3, estilo="relleno", anchor="que-hacemos"),
        bloque_carta(("Precios orientativos", "Indicative prices"), [
            (("Revisión y prevención", "Check-ups and prevention"), ("Precios de revisión y prevención", "Check-up and prevention prices"),
             [("Tratamiento", "Treatment"), ("Precio", "Price")],
             [(("Primera visita y revisión", "First visit and check-up"), eur(30)),
              (("Limpieza dental", "Dental cleaning"), eur(55)),
              (("Radiografía panorámica", "Panoramic X-ray"), eur(35)),
              (("Revisión infantil", "Children's check-up"), eur(25))]),
            (("Tratamientos", "Treatments"), ("Precios de tratamientos", "Treatment prices"),
             [("Tratamiento", "Treatment"), ("Precio", "Price")],
             [(("Empaste", "Filling"), desde(55)), (("Extracción simple", "Simple extraction"), desde(60)),
              (("Endodoncia", "Root canal"), desde(190)),
              (("Férula de descarga", "Night guard"), eur(180))]),
            (("Ortodoncia e implantes", "Orthodontics and implants"), ("Precios de ortodoncia e implantes", "Orthodontics and implant prices"),
             [("Tratamiento", "Treatment"), ("Precio", "Price")],
             [(("Estudio de ortodoncia", "Orthodontic assessment"), eur(60)),
              (("Ortodoncia con brackets, tratamiento completo", "Braces, full treatment"), ("Desde 2.200 €", "From €2,200")),
              (("Implante con corona", "Implant with crown"), ("Desde 1.200 €", "From €1,200"))]),
        ], nota=nota, tono="base", anchor="precios"),
        bloque_texto(("Si el tratamiento es largo", "If the treatment is long"), [
            ("Para tratamientos largos, como una ortodoncia o un implante, pregunta por las opciones de pago aplazado.",
             "For longer treatments, such as orthodontics or an implant, ask about deferred payment options."),
            ("Las condiciones te las damos por escrito antes de empezar, con el importe de cada cuota y el total.",
             "We give you the terms in writing before we start, with the amount of each instalment and the total."),
        ], tono="relleno", anchor="financiacion"),
        bloque_faq(("Preguntas frecuentes", "Frequently asked questions"), [
            (("¿Duele?", "Does it hurt?"),
             ("Usamos anestesia local cuando hace falta y paramos si nos lo pides. Si te da miedo ir al dentista, dínoslo al reservar y vamos a tu ritmo.",
              "We use local anaesthetic when needed and stop if you ask us to. If you are afraid of the dentist, tell us when you book and we go at your pace.")),
            (("¿Cada cuánto debo venir?", "How often should I come?"),
             ("Con una revisión al año suele bastar. Si tienes encías delicadas o llevas ortodoncia, te pediremos venir más a menudo.",
              "A check-up once a year is usually enough. If you have sensitive gums or wear braces, we will ask you to come more often.")),
            (("¿Atendéis a niños?", "Do you see children?"),
             ("Sí. Lo mejor es traerlos a una primera revisión pronto, para que se acostumbren y se detecte cualquier cosa a tiempo.",
              "Yes. It is best to bring them for a first check-up early, so they get used to it and anything is caught in good time.")),
            (("¿Cuánto cuesta un implante?", "How much does an implant cost?"),
             ("Depende de cuántas piezas falten y del hueso que tengas. Primero hacemos la revisión y la radiografía, y después te damos el presupuesto cerrado.",
              "It depends on how many teeth are missing and how much bone you have. First we do the check-up and X-ray, then we give you a fixed quote.")),
            (("¿Qué hago si pierdo un empaste o se me rompe un diente?", "What if I lose a filling or break a tooth?"),
             ("Llámanos cuanto antes. Si es un trozo entero, guárdalo en leche o en un vaso con saliva y tráelo contigo.",
              "Call us as soon as you can. If a whole piece comes off, keep it in milk or in a glass with saliva and bring it with you.")),
        ], tono="base"),
        bloque_cta(("¿Quieres que te miremos la boca?", "Would you like us to look at your teeth?"), None, [cita(), whatsapp("ghost")], tono="relleno"),
    ])

    clinica = pagina("clinica", ("la-clinica", "the-clinic"), ("La clínica", "The clinic"),
                     ("El equipo de la clínica dental con su número de colegiado, las instalaciones y cómo cuidamos tu seguridad y tus datos.",
                      "The dental clinic team with their registration number, the facilities and how we look after your safety and your data."), [
        hero(("La clínica", "The clinic"),
             ("Un equipo pequeño que te conoce por tu nombre y te atiende siempre en el mismo sitio.",
              "A small team that knows you by name and always sees you in the same place."),
             [cita(), llamar("secondary")], variante="texto"),
        _equipo(("Quién te atiende", "Who looks after you"), [
            (("Odontólogo/a", "Dentist"),
             ("Revisión, diagnóstico y tratamiento. Te explica cada paso antes de hacerlo.",
              "Check-up, diagnosis and treatment. Explains each step before doing it."), True),
            (("Odontólogo/a, ortodoncia e implantes", "Dentist, orthodontics and implants"),
             ("Estudia tu caso y te presenta las opciones con sus plazos y su coste.",
              "Studies your case and sets out the options with their timescales and cost."), True),
            (("Higienista dental", "Dental hygienist"),
             ("Limpiezas y consejos para cuidarte la boca en casa.",
              "Cleanings and advice for looking after your teeth at home."), False),
        ]),
        bloque_checks(("Tu seguridad y tus datos", "Your safety and your data"), [
            ("El instrumental se esteriliza después de cada paciente.", "Instruments are sterilised after every patient."),
            ("Usamos material de un solo uso cuando corresponde.", "We use single-use items where appropriate."),
            ("Las radiografías se hacen solo cuando aportan algo al diagnóstico.", "X-rays are taken only when they add something to the diagnosis."),
            (DATOS[0], DATOS[1]),
        ], tono="relleno", cols=2),
        bloque_galeria(("La clínica por dentro", "Inside the clinic"), [
            ("Sala de espera de la clínica dental", "Waiting room of the dental clinic"),
            ("Gabinete con sillón dental", "Surgery with dental chair"),
            ("Zona de esterilización del instrumental", "Instrument sterilisation area"),
            ("Recepción de la clínica", "Clinic reception")], cols=2, ratio=1.4, tono="base"),
        bloque_cta(("Ven a conocernos", "Come and meet us"),
                   ("Pasa por la clínica o llámanos y te enseñamos las instalaciones.",
                    "Drop by the clinic or call us and we will show you around."), None),
    ])

    cont = pagina("contacto", ("contacto", "contact"), ("Contacto y cita", "Contact and appointments"),
                  ("Pide cita por teléfono, WhatsApp o con el formulario. Horario, urgencias y cómo llegar a la clínica dental.",
                   "Book by phone, WhatsApp or with the form. Opening hours, emergencies and how to get to the dental clinic."), [
        hero(("Pide tu cita", "Book your appointment"),
             ("Llámanos, escríbenos por WhatsApp o rellena el formulario. Te confirmamos el mismo día. Si es una urgencia, llama.",
              "Call us, message us on WhatsApp or fill in the form. We confirm the same day. If it is an emergency, call."),
             [llamar(), whatsapp()], variante="texto"),
        bloque_contacto(("Cómo encontrarnos", "How to find us"),
                        ("Estamos en el centro, con parada de autobús cerca.", "We are in the town centre, near a bus stop."),
                        campos_cita(servicios=[("Revisión", "Check-up"), ("Limpieza", "Cleaning"),
                                               ("Dolor o urgencia", "Pain or emergency"), ("Ortodoncia", "Orthodontics"),
                                               ("Implantes", "Implants"), ("Otro motivo", "Something else")],
                                    extra=[campo("paciente", "select", ("¿Has venido antes?", "Have you visited before?"), False,
                                                 opciones=[("Es mi primera visita", "This is my first visit"),
                                                           ("Ya soy paciente", "I am already a patient")])]),
                        ("Pedir cita", "Request appointment"), horas,
                        notas=_junta(COLEGIADO, DATOS),
                        exito=("Gracias. Te confirmamos la cita por teléfono o WhatsApp en cuanto la tengamos.",
                               "Thank you. We will confirm your appointment by phone or WhatsApp as soon as we have it.")),
        bloque_mapa(tono="relleno"),
    ])

    footer = pie(nav, ("Clínica dental con cita previa. Revisiones, limpiezas, ortodoncia e implantes.",
                       "Dental clinic by appointment. Check-ups, cleanings, orthodontics and implants."),
                 ("Lunes a jueves 9:30 a 14:00 y 16:00 a 20:00. Viernes 9:30 a 15:00.",
                  "Monday to Thursday 9:30 am to 2 pm and 4 pm to 8 pm. Friday 9:30 am to 3 pm."))
    footer["children"].insert(1, peq(COLEGIADO, color="$muted"))

    return plantilla(KEY, ("Clínica dental", "Dental clinic"),
                     ("Clínica dental con tratamientos y precios orientativos, primera visita explicada, equipo colegiado, urgencias y cita por WhatsApp.",
                      "Dental clinic with treatments and indicative prices, first visit explained, registered team, emergencies and WhatsApp booking."),
                     "salud", ("dentista clinica dental odontologo odontologia ortodoncia implantes empastes limpieza dental urgencias dentales",
                               "dentist dental clinic orthodontics implants fillings teeth cleaning dental emergency"),
                     look, cabecera(nav, cita()), footer, [inicio, trat, clinica, cont])
