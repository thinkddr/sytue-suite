# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
from lib import *
from looks import LOOKS  # noqa: F401  (paleta propia más abajo)

KEY = "centro-depilacion-laser"

# Blanco rosado con malva vino: limpio y cuidado, sin el rosa chicle de las peluquerías ni el lila de las uñas.
PERLA = dict(background="#fdf6f4", surface="#f7e8e5", text="#2b1b20", muted="#6b5258", primary="#8c3b52",
             on_primary="#ffffff", border="#ecd5d2", inverse="#3a1c28", on_inverse="#fdf0f2")

NOTA = ("Precios de ejemplo: cámbialos por los tuyos. Incluyen IVA. Las sesiones necesarias varían de una persona a otra.",
        "Example prices: replace them with your own. VAT included. The number of sessions needed varies from person to person.")
AVISO = ("Este tratamiento es estético y no sustituye a una consulta médica. Si tienes dudas sobre tu piel, un lunar o tu salud, consulta antes con tu médico o dermatólogo.",
         "This is a cosmetic treatment and does not replace a medical consultation. If you have doubts about your skin, a mole or your health, see your doctor or dermatologist first.")


def build():
    nueva(KEY)
    look = dict(colores=PERLA, fuentes=("figtree", "manrope"), peso=600, radio=(10, 16, 24), ancho=1120)
    nav = [("inicio", ("Inicio", "Home")), ("tratamientos", ("Zonas y sesiones", "Areas and sessions")),
           ("tarifas", ("Tarifas y bonos", "Prices and vouchers")), ("contacto", ("Contacto", "Contact"))]
    horas = [(("Lunes a viernes", "Monday to Friday"), ("10:00 a 20:00", "10:00 am to 8:00 pm")),
             (("Sábado", "Saturday"), ("10:00 a 14:00", "10:00 am to 2:00 pm")),
             (("Domingo", "Sunday"), ("Cerrado", "Closed"))]
    cab_zona = [("Zona", "Area"), ("Sesiones orientativas", "Indicative sessions"), ("Entre sesiones", "Between sessions")]

    def pedir(v="primary"):
        return boton(("Pedir cita", "Book an appointment"), a_pagina("contacto", "contacto"), v)

    inicio = pagina("inicio", ("", ""), ("Depilación láser y estética avanzada", "Laser hair removal and advanced aesthetics"),
                    ("Centro de depilación láser y luz pulsada con cita previa: zonas, sesiones orientativas, primera cita con test de piel y bonos. Llama o escribe por WhatsApp.",
                     "Laser and pulsed-light hair removal centre by appointment: areas, indicative sessions, first visit with a skin test and vouchers. Call or message us on WhatsApp."), [
        hero(("Depilación láser, con una valoración antes de empezar", "Laser hair removal, with an assessment before we start"),
             ("Depilación con láser y luz pulsada y estética avanzada. Antes de la primera sesión revisamos tu piel y tu vello, y te decimos qué esperar y cuántas sesiones calculamos.",
              "Laser and pulsed-light hair removal and advanced aesthetics. Before the first session we check your skin and hair, and tell you what to expect and how many sessions we estimate."),
             [pedir(), whatsapp()],
             eyebrow=("Depilación láser y luz pulsada", "Laser and pulsed-light hair removal"), variante="centro", tono="relleno"),
        bloque_valores(("Qué hacemos", "What we do"), [
            (("Depilación láser", "Laser hair removal"),
             ("Para zonas pequeñas y grandes, con un equipo elegido según tu tipo de piel y de vello.",
              "For small and large areas, with equipment chosen for your skin and hair type.")),
            (("Luz pulsada", "Pulsed light"),
             ("Otra tecnología para depilar, que a veces se propone cuando el láser no es lo más adecuado.",
              "Another hair removal technology, sometimes suggested when laser is not the best fit.")),
            (("Estética avanzada", "Advanced aesthetics"),
             ("Tratamientos faciales y corporales con aparatos. Te explicamos cada uno antes de reservarlo.",
              "Facial and body treatments with devices. We explain each one before you book it.")),
        ], cols=3, eyebrow=("Tratamientos", "Treatments"), tono="base", estilo="relleno"),
        bloque_pasos(("Tu primera cita", "Your first appointment"), [
            (("Hablamos", "We talk"),
             ("Qué zonas quieres tratar, qué has usado hasta ahora y si tomas medicación o tienes alguna condición de la piel.",
              "Which areas you want treated, what you have used so far and whether you take medication or have a skin condition.")),
            (("Miramos tu piel", "We check your skin"),
             ("Valoramos el tono de piel, el color y el grosor del vello, y si hay bronceado reciente, lunares o tatuajes en la zona.",
              "We assess your skin tone, hair colour and thickness, and whether there is recent tan, moles or tattoos in the area.")),
            (("Test de piel", "Skin test"),
             ("Hacemos una prueba en una zona pequeña y esperamos unos minutos para ver cómo responde tu piel.",
              "We try a small patch and wait a few minutes to see how your skin responds.")),
            (("Plan y primera sesión", "Plan and first session"),
             ("Si todo está bien, te proponemos un plan por zonas. Podemos empezar el mismo día si lo prefieres.",
              "If all is well, we suggest a plan by area. We can start the same day if you prefer.")),
        ], intro=("La valoración y el test duran unos 30 minutos y son gratuitos. Ven con la piel limpia y sin crema.",
                  "The assessment and test take about 30 minutes and are free. Come with clean skin and no cream."),
            tono="relleno"),
        bloque_checks(("Antes de venir", "Before you come"), [
            ("Rasura la zona el día anterior, sin cera ni pinzas durante las 4 semanas previas", "Shave the area the day before, and no waxing or plucking in the 4 weeks before"),
            ("Evita el sol y las cabinas de bronceado unas semanas antes", "Avoid the sun and sunbeds for a few weeks beforehand"),
            ("Dinos si tomas medicación o usas cremas con ácidos", "Tell us if you take medication or use creams with acids"),
            ("Si estás embarazada, avísanos antes de reservar", "If you are pregnant, tell us before you book"),
        ], eyebrow=("Preparación", "Preparation"), cols=2, tono="base"),
        bloque_texto(("No es una consulta médica", "It is not a medical consultation"), [
            AVISO,
            ("El número de sesiones es una estimación. El vello se reduce poco a poco y el resultado varía de una persona a otra: no podemos prometer una eliminación definitiva.",
             "The number of sessions is an estimate. Hair thins out gradually and results vary from person to person: we cannot promise permanent removal."),
        ], tono="relleno", anchor="aviso"),
        bloque_opiniones(("Lo que dicen nuestras clientas", "What our clients say"),
                         [("el trato", "the service"), ("las explicaciones", "the explanations"), ("la limpieza", "the cleanliness")],
                         tono="base"),
        bloque_cta(("¿Empezamos con una valoración?", "Shall we start with an assessment?"),
                   ("Dinos qué zonas te interesan y el día que te viene bien, y te damos cita.",
                    "Tell us which areas interest you and the day that suits you, and we will book you in."),
                   [pedir(), whatsapp("ghost")], tono="relleno"),
    ])

    trat = pagina("tratamientos", ("zonas-y-sesiones", "areas-and-sessions"), ("Zonas y sesiones", "Areas and sessions"),
                  ("Zonas que se tratan con depilación láser, número de sesiones orientativo, cuidados antes y después y a quién no conviene hacerla.",
                   "Areas treated with laser hair removal, indicative number of sessions, care before and after, and who it is not suitable for."), [
        hero(("Zonas y sesiones", "Areas and sessions"),
             ("Cada zona necesita un número distinto de sesiones, y cada persona responde a su ritmo. Estas cifras son orientativas.",
              "Each area needs a different number of sessions, and each person responds at their own pace. These figures are indicative."),
             [pedir()], variante="texto"),
        bloque_precios(("Zonas y número de sesiones", "Areas and number of sessions"),
                       ("Sesiones orientativas por zona", "Indicative sessions by area"), cab_zona,
                       [(("Labio superior y mentón", "Upper lip and chin"), ("6 a 8", "6 to 8"), ("4 a 6 semanas", "4 to 6 weeks")),
                        (("Axilas", "Underarms"), ("6 a 8", "6 to 8"), ("4 a 6 semanas", "4 to 6 weeks")),
                        (("Ingles", "Bikini line"), ("6 a 8", "6 to 8"), ("4 a 6 semanas", "4 to 6 weeks")),
                        (("Piernas completas", "Full legs"), ("6 a 10", "6 to 10"), ("6 a 8 semanas", "6 to 8 weeks")),
                        (("Espalda", "Back"), ("6 a 10", "6 to 10"), ("6 a 8 semanas", "6 to 8 weeks")),
                        (("Cuerpo completo", "Full body"), ("6 a 10", "6 to 10"), ("6 a 8 semanas", "6 to 8 weeks"))],
                       intro=("Se hacen sesiones espaciadas porque solo se trata el vello que está en su fase de crecimiento.",
                              "Sessions are spaced out because only hair in its growth phase is treated."),
                       nota=("Estas cifras son una estimación y el centro puede cambiarlas según cada caso. Nada de esto es una promesa de resultado definitivo.",
                             "These figures are an estimate and may change case by case. None of this is a promise of permanent results."),
                       tono="base", anchor="zonas", kind="features"),
        bloque_valores(("Cuidados antes y después", "Care before and after"), [
            (("Antes", "Before"),
             ("Rasura la zona el día anterior. Sin cera, pinzas ni cremas depilatorias durante las semanas previas. Piel limpia, sin desodorante ni crema.",
              "Shave the area the day before. No waxing, plucking or hair-removal cream in the preceding weeks. Clean skin, no deodorant or lotion.")),
            (("Después", "After"),
             ("Es normal notar la piel algo roja o sensible unas horas. Hidrátala y evita calor intenso, sauna y piscina uno o dos días.",
              "It is normal for skin to look a little red or feel sensitive for a few hours. Moisturise it and avoid intense heat, saunas and pools for a day or two.")),
            (("Sol", "Sun"),
             ("Protección solar alta en las zonas tratadas durante todo el tratamiento y evita broncearte justo antes y después.",
              "High-factor sun protection on treated areas throughout the treatment, and avoid tanning just before and after.")),
        ], cols=3, tono="relleno", estilo="borde", anchor="cuidados"),
        bloque_checks(("Cuándo te pedimos esperar o consultar antes", "When we ask you to wait or check first"), [
            ("Embarazo o lactancia", "Pregnancy or breastfeeding"),
            ("Bronceado reciente o piel muy irritada en la zona", "A recent tan or very irritated skin in the area"),
            ("Medicación que hace la piel más sensible al sol", "Medication that makes skin more sensitive to the sun"),
            ("Lunares nuevos, que cambian o que te preocupan", "Moles that are new, changing or worrying you"),
            ("Vello muy claro o canoso, que responde poco al láser", "Very light or grey hair, which responds poorly to laser"),
        ], intro=AVISO, tono="base", cols=2, kind="features"),
        bloque_texto(("Quién te atiende", "Who treats you"), [
            ("La valoración inicial y el seguimiento de los casos con dudas los hace un profesional sanitario del centro.",
             "The initial assessment and follow-up of cases with doubts are carried out by a healthcare professional at the centre."),
            (COLEGIADO[0], COLEGIADO[1]),
        ], tono="relleno", anchor="profesional"),
        bloque_faq(("Preguntas frecuentes", "Frequently asked questions"), [
            (("¿Duele?", "Does it hurt?"),
             ("Se nota un calor y un pequeño pellizco, más en zonas sensibles. El equipo enfría la piel mientras trabaja.",
              "You feel warmth and a small pinch, more in sensitive areas. The device cools the skin while it works.")),
            (("¿Cuánto dura una sesión?", "How long does a session take?"),
             ("Labio o axilas, entre 10 y 15 minutos. Piernas completas, entre 40 y 60. Te damos el tiempo exacto al reservar.",
              "Upper lip or underarms, 10 to 15 minutes. Full legs, 40 to 60. We give you the exact time when you book.")),
            (("¿Puedo depilarme con láser si tengo la piel bronceada?", "Can I have laser if my skin is tanned?"),
             ("Mejor esperar. La piel bronceada absorbe más energía y aumenta el riesgo de quemadura; te proponemos fecha cuando se haya aclarado.",
              "Better to wait. Tanned skin absorbs more energy and raises the risk of burns; we suggest a date once it has faded.")),
        ], tono="base"),
        bloque_cta(("¿Quieres tu valoración?", "Would you like your assessment?"), None, [pedir(), whatsapp("ghost")], tono="relleno"),
    ])

    tarifas = pagina("tarifas", ("tarifas", "prices"), ("Tarifas y bonos", "Prices and vouchers"),
                     ("Tarifas orientativas de depilación láser por zona, bonos de sesiones y condiciones de pago.",
                      "Indicative laser hair removal prices by area, session vouchers and payment terms."), [
        hero(("Tarifas y bonos", "Prices and vouchers"),
             ("Precio por sesión y por zona. Si necesitas varias sesiones, un bono sale más barato que pagarlas una a una.",
              "Price per session and per area. If you need several sessions, a voucher works out cheaper than paying one by one."),
             [pedir()], variante="texto"),
        bloque_precios(("Precio por zona", "Price by area"), ("Precio por sesión y zona", "Price per session and area"),
                       [("Zona", "Area"), ("Por sesión", "Per session")],
                       [(("Labio superior", "Upper lip"), eur(25)), (("Axilas", "Underarms"), eur(35)),
                        (("Ingles", "Bikini line"), eur(45)), (("Medias piernas", "Half legs"), eur(60)),
                        (("Piernas completas", "Full legs"), eur(90)), (("Espalda", "Back"), desde(80)),
                        (("Cuerpo completo", "Full body"), desde(220))],
                       nota=NOTA, tono="base", anchor="precios"),
        bloque_precios(("Bonos", "Vouchers"), ("Precios de bonos", "Voucher prices"),
                       [("Bono", "Voucher"), ("Precio", "Price")],
                       [(("6 sesiones de axilas", "6 underarm sessions"), eur(180)),
                        (("6 sesiones de ingles", "6 bikini line sessions"), eur(230)),
                        (("6 sesiones de piernas completas", "6 full leg sessions"), eur(460)),
                        (("Bono personalizado de varias zonas", "Custom voucher for several areas"), ("Consúltanos", "Ask us"))],
                       intro=("El bono es personal, se paga al empezar y lleva fecha de validez. Si tras la valoración no es adecuado para ti, no lo vendemos.",
                              "A voucher is personal, paid at the start and has an expiry date. If it turns out not to be suitable for you after the assessment, we do not sell it."),
                       nota=NOTA, tono="relleno", anchor="bonos"),
        bloque_faq(("Preguntas sobre las tarifas", "Questions about prices"), [
            (("¿La valoración se cobra?", "Is the assessment charged?"),
             ("No: la valoración y el test de piel son gratuitos y sin compromiso.",
              "No: the assessment and skin test are free and without obligation.")),
            (("¿Qué pasa si tengo que cancelar?", "What if I have to cancel?"),
             ("Avísanos con 24 horas y movemos la sesión sin coste.", "Let us know 24 hours ahead and we move the session at no cost.")),
            (("¿Cómo se paga?", "How do I pay?"),
             ("Con tarjeta o en efectivo al terminar la sesión. Los bonos, al comprarlos. Te damos factura.",
              "By card or in cash at the end of the session. Vouchers, when you buy them. We give you an invoice.")),
        ], tono="base"),
        bloque_cta(("¿Reservamos tu valoración?", "Shall we book your assessment?"), None, [pedir(), whatsapp("ghost")], tono="relleno"),
    ])

    cont = pagina("contacto", ("contacto", "contact"), ("Contacto y cita", "Contact and appointments"),
                  ("Pide cita de valoración por teléfono, WhatsApp o con el formulario. Horario y cómo llegar al centro.",
                   "Book an assessment by phone, WhatsApp or with the form. Opening hours and how to get to the centre."), [
        hero(("Pide tu cita", "Book your appointment"),
             ("Llámanos, escríbenos por WhatsApp o rellena el formulario. Te confirmamos la cita el mismo día.",
              "Call us, message us on WhatsApp or fill in the form. We confirm your appointment the same day."),
             [llamar(), whatsapp()], variante="texto"),
        bloque_contacto(("Cómo encontrarnos", "How to find us"),
                        ("Estamos en una calle céntrica, con parada de autobús cerca.", "We are on a central street, near a bus stop."),
                        campos_cita(servicios=[("Valoración y test de piel", "Assessment and skin test"),
                                               ("Depilación láser", "Laser hair removal"), ("Luz pulsada", "Pulsed light"),
                                               ("Estética avanzada", "Advanced aesthetics"), ("Bono", "Voucher")],
                                    extra=[campo("zonas", "text", ("Zonas que te interesan (opcional)", "Areas you are interested in (optional)"),
                                                 False, maxlen=200)]),
                        ("Pedir cita", "Request appointment"), horas,
                        notas=AVISO,
                        exito=("Gracias. Te confirmamos la cita por teléfono o WhatsApp en cuanto la tengamos.",
                               "Thank you. We will confirm your appointment by phone or WhatsApp as soon as we have it.")),
        bloque_mapa(tono="relleno"),
    ])

    return plantilla(KEY, ("Depilación láser y estética avanzada", "Laser hair removal and advanced aesthetics"),
                     ("Centro de depilación láser y luz pulsada con zonas y sesiones orientativas, tarifas por zona, bonos, primera cita con test de piel y cita por WhatsApp.",
                      "Laser and pulsed-light hair removal centre with indicative areas and sessions, prices by area, vouchers, a first visit with a skin test and WhatsApp booking."),
                     "belleza", ("depilacion laser centro de depilacion luz pulsada ipl estetica avanzada zonas sesiones bonos test de piel",
                                 "laser hair removal hair removal clinic pulsed light ipl advanced aesthetics areas sessions vouchers skin test"),
                     look, cabecera(nav, pedir()),
                     pie(nav, ("Centro de depilación láser y estética avanzada con cita previa.",
                               "Laser hair removal and advanced aesthetics centre by appointment."),
                         ("Lunes a viernes 10:00 a 20:00. Sábado 10:00 a 14:00.", "Monday to Friday 10 am to 8 pm. Saturday 10 am to 2 pm.")),
                     [inicio, trat, tarifas, cont])
