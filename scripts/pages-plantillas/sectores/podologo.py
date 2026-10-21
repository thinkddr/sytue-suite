# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
from lib import *
from looks import LOOKS

KEY = "podologo"

# Paleta propia: verde hoja sobre lino verdoso (ninguna otra plantilla usa este fondo con este primario).
HOJA = dict(background="#fbfcf6", surface="#e9f1dd", text="#14230f", muted="#485a41", primary="#0b6b14",
            on_primary="#ffffff", border="#d3e0c2", inverse="#12301a", on_inverse="#eef7e6")

DATOS = ("Tus datos y tu historial clínico se tratan con confidencialidad y según la normativa de protección de datos; solo acceden a ellos las personas que te atienden.",
         "Your details and clinical history are handled in confidence and in line with data protection rules; only the people treating you can see them.")

NOTA = ("Precios de ejemplo: cámbialos por los tuyos. La duración es orientativa y depende de cada pie.",
        "Example prices: replace them with your own. Timings are approximate and depend on each foot.")


def _junta(*ts):
    return (" ".join(t[0] for t in ts), " ".join(t[1] for t in ts))


def _equipo():
    """Quién atiende, con el número de colegiado visible en cada ficha."""
    fichas = [(("Podólogo/a", "Podiatrist"),
               ("Te explora, te dice qué ve y qué opciones hay antes de tocar nada.",
                "Examines you, tells you what they see and what the options are before touching anything.")),
              (("Podólogo/a, pie diabético y plantillas", "Podiatrist, diabetic foot and insoles"),
               ("Hace el estudio de la pisada y las revisiones periódicas del pie con diabetes.",
                "Carries out the gait study and the regular checks for diabetic feet."))]
    li = [marco([imagen("portrait", ("Foto de la persona del equipo", "Photo of the team member"), ratio=1.0),
                 h3(("Nombre Apellido", "First Last")), peq(rol, color="$muted"), p(frase),
                 peq(COLEGIADO, color="$muted")], tag="li", gap=10) for rol, frase in fichas]
    return seccion("team", [titular(("Quién te atiende", "Who looks after you")), rejilla(li, cols=2)],
                   tono="base", anchor="equipo")


def build():
    nueva(KEY)
    look = dict(LOOKS["menta"], colores=HOJA, fuentes=("lora", "work-sans"), radio=(4, 8, 12))
    nav = [("inicio", ("Inicio", "Home")), ("tratamientos", ("Tratamientos", "Treatments")),
           ("plantillas", ("Plantillas y pisada", "Insoles and gait")), ("contacto", ("Contacto", "Contact"))]
    horas = [(("Lunes a viernes", "Monday to Friday"),
              ("10:00 a 14:00 y 16:00 a 20:00", "10:00 am to 2:00 pm and 4:00 pm to 8:00 pm")),
             (("Sábado", "Saturday"), ("10:00 a 13:00", "10:00 am to 1:00 pm")),
             (("Domingo", "Sunday"), ("Cerrado", "Closed"))]
    cita = lambda v="primary": boton(("Pedir cita", "Book an appointment"), a_pagina("contacto", "contacto"), v)
    wa2 = lambda: whatsapp("ghost")

    inicio = pagina("inicio", ("", ""), ("Clínica de podología", "Podiatry clinic"),
                    ("Clínica de podología con cita previa: quiropodia, uñas encarnadas, pie diabético, estudio de la pisada y plantillas a medida. Llama o escribe por WhatsApp.",
                     "Podiatry clinic by appointment: routine foot care, ingrown toenails, diabetic foot, gait study and custom insoles. Call or message us on WhatsApp."), [
        hero(("Tus pies, revisados con calma", "Your feet, looked after calmly"),
             ("Cuidamos durezas, uñas, rozaduras y dolores al andar. En la primera visita te miramos los pies, te explicamos qué vemos y qué se puede hacer, y decides tú.",
              "We look after hard skin, nails, rubbing and pain when walking. At your first visit we examine your feet, explain what we see and what can be done, and you decide."),
             [llamar(), whatsapp()], eyebrow=("Clínica de podología", "Podiatry clinic"),
             imagen_=("photo", ("Camilla de podología con lámpara y material ordenado", "Podiatry couch with lamp and tidy equipment"))),
        bloque_valores(("¿Te suena alguna de estas?", "Does any of these sound familiar?"), [
            (("Me duele al andar o al estar de pie", "It hurts when I walk or stand"),
             ("Miramos cómo pisas y de dónde viene la molestia. A veces basta con un consejo de calzado; otras, unas plantillas.",
              "We look at how you step and where the pain comes from. Sometimes shoe advice is enough; sometimes insoles help.")),
            (("Una uña me molesta o se me clava", "A toenail hurts or digs in"),
             ("Cuanto antes la veamos, más sencillo es. No te la arregles tú en casa si hay dolor, rojez o pus.",
              "The sooner we see it, the simpler it is. Do not treat it yourself at home if there is pain, redness or pus.")),
            (("Tengo diabetes", "I have diabetes"),
             ("Una revisión periódica del pie ayuda a detectar a tiempo heridas, durezas o cambios de sensibilidad.",
              "A regular foot check helps spot sores, hard skin or changes in feeling in good time.")),
            (("Corro o hago deporte", "I run or play sport"),
             ("Ampollas, uñas golpeadas, durezas y dolor en la planta. Vemos tu pisada y tu calzado.",
              "Blisters, bruised nails, hard skin and pain under the foot. We look at your gait and your shoes.")),
            (("Tengo callos, durezas o pies agrietados", "I have corns, hard skin or cracked feet"),
             ("Los quitamos sin dolor y te explicamos por qué salen para que no vuelvan tan deprisa.",
              "We remove them painlessly and explain why they appear so they do not come back as quickly.")),
            (("Quiero que revisen los pies de mis hijos", "I want my children's feet checked"),
             ("Una revisión tranquila, con juego, para ver cómo apoyan y si hay algo que vigilar.",
              "A relaxed, playful check to see how they stand and whether there is anything to keep an eye on.")),
        ], cols=3, estilo="linea", anchor="para-quien"),
        bloque_pasos(("Tu primera visita, paso a paso", "Your first visit, step by step"), [
            (("Hablamos", "We talk"),
             ("Qué te molesta, desde cuándo, qué calzado usas y si tienes diabetes, alergias o tomas medicación.",
              "What bothers you, since when, what shoes you wear and whether you have diabetes, allergies or take medication.")),
            (("Te miramos los pies", "We examine your feet"),
             ("Piel, uñas, apoyo y movimiento, de pie y andando. No duele.",
              "Skin, nails, foot position and movement, standing and walking. It does not hurt.")),
            (("Te lo explicamos", "We explain it"),
             ("Qué hemos visto, qué opciones hay y cuánto cuesta cada una. No hace falta decidir ese día.",
              "What we saw, what the options are and what each one costs. You do not have to decide that day.")),
            (("Si procede, empezamos", "If needed, we begin"),
             ("Muchas veces el tratamiento se hace en la misma visita. Si no, te damos otra cita.",
              "Often the treatment is done at the same visit. If not, we book you another one.")),
        ], eyebrow=("Primera visita", "First visit"), tono="relleno"),
        bloque_checks(("Qué llevar a la primera visita", "What to bring to your first visit"), [
            ("El calzado que más usas, y el de deporte si haces.", "The shoes you wear most, and your sports shoes if you play sport."),
            ("Tus plantillas, si ya llevas.", "Your insoles, if you already wear them."),
            ("Informes o análisis recientes, si tienes diabetes u otra enfermedad.", "Recent reports or test results, if you have diabetes or another condition."),
            ("Una lista de la medicación que tomas.", "A list of the medication you take."),
        ], cols=2, tono="base"),
        _equipo(),
        bloque_opiniones(("Lo que dicen nuestros pacientes", "What our patients say"),
                         [("el trato", "the care"), ("las explicaciones", "the explanations"), ("la puntualidad", "the punctuality")],
                         tono="relleno"),
        bloque_cta(("¿Te damos cita?", "Shall we book you in?"),
                   ("Dinos qué te molesta y qué día te viene bien, y te confirmamos.",
                    "Tell us what bothers you and which day suits you, and we will confirm."),
                   [cita(), wa2()], tono="base"),
    ])

    trat = pagina("tratamientos", ("tratamientos", "treatments"), ("Tratamientos y precios", "Treatments and prices"),
                  ("Qué se hace en podología, cuánto suele durar cada tratamiento y precios orientativos: quiropodia, uñas encarnadas, pie diabético y pie del deportista.",
                   "What is done in podiatry, how long each treatment usually takes and indicative prices: routine foot care, ingrown toenails, diabetic foot and the sportsperson's foot."), [
        hero(("Tratamientos y precios", "Treatments and prices"),
             ("Qué te hacemos, cuánto suele durar y cuánto cuesta. El precio final lo confirmamos en la visita, antes de empezar.",
              "What we do, how long it usually takes and what it costs. We confirm the final price at the visit, before we start."),
             [cita(), llamar("secondary")], variante="texto"),
        bloque_precios(("Qué te hacemos, cuánto dura y cuánto cuesta", "What we do, how long it takes and what it costs"),
                       ("Tratamientos de podología con duración y precio", "Podiatry treatments with timing and price"),
                       [("Tratamiento", "Treatment"), ("En qué consiste", "What it involves"), ("Duración", "Duration"), ("Precio", "Price")],
                       [(("Primera visita y exploración", "First visit and examination"),
                         ("Exploración de pies y andar, y plan por escrito", "Foot and gait examination, plus a written plan"),
                         ("Unos 40 min", "About 40 min"), eur(35)),
                        (("Quiropodia", "Routine foot care"),
                         ("Corte y limado de uñas, retirada de callos y durezas", "Nail trimming and filing, removal of corns and hard skin"),
                         ("Unos 30 min", "About 30 min"), eur(30)),
                        (("Uña encarnada", "Ingrown toenail"),
                         ("Retirar la parte clavada y curar la zona", "Removing the part that digs in and dressing the area"),
                         ("Unos 30 min", "About 30 min"), desde(40)),
                        (("Revisión del pie diabético", "Diabetic foot check"),
                         ("Piel, uñas, sensibilidad y pulsos del pie", "Skin, nails, sensation and pulses of the foot"),
                         ("Unos 30 min", "About 30 min"), eur(35)),
                        (("Tratamiento de hongos en uñas", "Fungal nail treatment"),
                         ("Limpieza de la uña y tratamiento tópico", "Cleaning the nail and topical treatment"),
                         ("Unos 30 min", "About 30 min"), desde(35)),
                        (("Verrugas plantares", "Plantar warts"),
                         ("Tratamiento de cada lesión, por sesiones", "Treatment of each lesion, in sessions"),
                         ("Unos 20 min", "About 20 min"), desde(40))],
                       nota=NOTA, tono="relleno", anchor="precios"),
        bloque_texto(("Uña encarnada: qué pasa", "Ingrown toenail: what happens"), [
            ("Retiramos la parte de la uña que se clava, limpiamos y vendamos. Suele notarse alivio al terminar. Si tiene pus o llevas tiempo con ella, te lo decimos antes y lo valoramos con calma.",
             "We remove the part of the nail that digs in, clean and dress it. Most people feel relief afterwards. If there is pus or you have had it a while, we tell you first and assess it calmly."),
            ("Si la uña vuelve a clavarse a menudo, puede plantearse una pequeña intervención con anestesia local. Te explicamos cómo es y qué cuidados hay después; decides tú.",
             "If the nail keeps digging in, a minor procedure under local anaesthetic may be suggested. We explain what it involves and the aftercare; the choice is yours."),
        ], eyebrow=("Uñas", "Nails"), tono="base", anchor="una-encarnada"),
        bloque_texto(("Pie diabético: revisiones periódicas", "Diabetic foot: regular checks"), [
            ("Con diabetes, una herida pequeña puede pasar desapercibida. En cada revisión miramos la piel, las uñas, si sientes bien y cómo está la circulación del pie, y te enseñamos a mirártelos tú en casa.",
             "With diabetes, a small wound can go unnoticed. At each check we look at the skin, the nails, whether you can feel properly and how the foot's circulation is, and show you how to check your own feet at home."),
            ("Esto no sustituye a tu médico ni a tu enfermera. Si ves una herida, una llaga o un cambio de color, no esperes a la revisión: llama y te vemos cuanto antes.",
             "This does not replace your doctor or nurse. If you see a wound, a sore or a change of colour, do not wait for the check: call and we will see you as soon as possible."),
        ], eyebrow=("Diabetes", "Diabetes"), tono="relleno", anchor="pie-diabetico"),
        bloque_checks(("Pie del deportista: lo que solemos ver", "The sportsperson's foot: what we often see"), [
            ("Ampollas y rozaduras por el calzado", "Blisters and rubbing from shoes"),
            ("Uñas golpeadas o que se oscurecen", "Bruised or darkening nails"),
            ("Durezas en zonas de apoyo", "Hard skin where you take weight"),
            ("Dolor en la planta o el talón al entrenar", "Pain under the foot or heel when training"),
            ("Calzado que no es el adecuado para tu pisada", "Shoes that do not suit your gait"),
            ("Hongos en uñas y piel de los pies", "Fungus on nails and skin of the feet"),
        ], intro=("Revisamos el pie, la pisada y el calzado, y te decimos si conviene cambiar de zapatilla o llevar plantillas.",
                  "We check the foot, your gait and your shoes, and tell you whether a different trainer or insoles would help."),
            cols=2, tono="base", anchor="deportista"),
        bloque_faq(("Preguntas frecuentes", "Frequently asked questions"), [
            (("¿Duele?", "Does it hurt?"),
             ("Lo normal es que no. Si una zona está sensible, vamos despacio y paramos cuando nos lo pidas.",
              "Usually not. If an area is sensitive we go slowly and stop whenever you ask us to.")),
            (("¿Cada cuánto tengo que venir?", "How often should I come?"),
             ("Depende de cada pie. Hay quien viene una vez al año y quien necesita revisión cada pocas semanas; te lo decimos en la visita.",
              "It depends on the foot. Some people come once a year and others need a check every few weeks; we tell you at the visit.")),
            (("¿Necesito derivación del médico?", "Do I need a referral from my doctor?"),
             ("No para pedir cita. Si tienes un informe o una analítica reciente, tráelo: nos ayuda.",
              "Not to book. If you have a recent report or test result, bring it: it helps us.")),
            (("¿Atendéis a niños y a personas mayores?", "Do you see children and older people?"),
             ("Sí, a todas las edades. Si cuesta venir a la clínica, pregúntanos por las opciones.",
              "Yes, all ages. If getting to the clinic is hard, ask us about the options.")),
            (("¿Qué hago si me sangra o se infecta una herida del pie?", "What if a foot wound bleeds or gets infected?"),
             ("Lávala con agua y jabón, tápala con una gasa limpia y llámanos o ve a tu centro de salud. Con diabetes, no lo dejes para otro día.",
              "Wash it with soap and water, cover it with a clean dressing and call us or go to your health centre. If you have diabetes, do not leave it for another day.")),
        ], tono="relleno"),
        bloque_cta(("¿Quieres que te veamos los pies?", "Would you like us to look at your feet?"), None,
                   [cita(), whatsapp("ghost")], tono="base"),
    ])

    plant = pagina("plantillas", ("plantillas-y-pisada", "insoles-and-gait"), ("Plantillas y estudio de la pisada", "Insoles and gait study"),
                   ("Cómo es el estudio de la pisada, cómo se hacen las plantillas personalizadas, cuánto cuestan y cómo adaptarse a ellas.",
                    "What a gait study is like, how custom insoles are made, what they cost and how to get used to them."), [
        hero(("Estudio de la pisada y plantillas a medida", "Gait study and custom insoles"),
             ("Primero miramos cómo pisas. Solo si te hacen falta, fabricamos unas plantillas para tus pies, no unas de caja.",
              "First we look at how you step. Only if you need them do we make insoles for your feet, not off-the-shelf ones."),
             [cita(), whatsapp()], variante="centro", eyebrow=("Pisada", "Gait")),
        bloque_pasos(("Cómo lo hacemos", "How we do it"), [
            (("Estudio de la pisada", "Gait study"),
             ("Te vemos de pie y andando, y si hace falta medimos dónde se reparte el peso con una plataforma.",
              "We watch you standing and walking and, if needed, measure how your weight is spread using a platform.")),
            (("Molde o escáner del pie", "Foot mould or scan"),
             ("Tomamos la forma de tu pie. No duele ni mancha.",
              "We take the shape of your foot. It does not hurt and it is not messy.")),
            (("Fabricación", "Making them"),
             ("Se hacen a partir de tu estudio, con el material que mejor encaje con tu calzado y tu actividad.",
              "They are made from your study, in the material that best fits your shoes and your activity.")),
            (("Prueba y ajuste", "Fitting and adjustment"),
             ("Te las ponemos, comprobamos que van cómodas en tu calzado y las retocamos si hace falta.",
              "We fit them, check they are comfortable in your shoes and adjust them if needed.")),
        ], intro=("Te decimos el plazo de entrega al encargarlas.", "We tell you when they will be ready when you order them."),
            tono="relleno", anchor="como-se-hacen"),
        bloque_valores(("A quién le suelen ayudar", "Who they usually help"), [
            (("Dolor en la planta o el talón", "Pain under the foot or heel"),
             ("Para repartir mejor el peso al andar o al correr.", "To spread the weight better when walking or running.")),
            (("Niños que apoyan raro", "Children who stand oddly"),
             ("Solo cuando hay motivo. Muchas veces basta con vigilar cómo crecen.", "Only when there is a reason. Often it is enough to keep an eye on how they grow.")),
            (("Quien está muchas horas de pie", "People on their feet all day"),
             ("Comercio, hostelería, sanidad: el pie se carga mucho.", "Retail, hospitality, healthcare: the foot takes a lot of load.")),
            (("Deportistas", "Sportspeople"),
             ("Ajustadas al calzado y al tipo de entrenamiento.", "Fitted to the shoe and the type of training.")),
        ], cols=2, estilo="relleno", tono="base"),
        bloque_carta(("Precios orientativos", "Indicative prices"), [
            (("Estudio y plantillas", "Study and insoles"), ("Precios del estudio de la pisada y las plantillas", "Gait study and insole prices"),
             [("Servicio", "Service"), ("Precio", "Price")],
             [(("Estudio de la pisada", "Gait study"), eur(45)),
              (("Plantillas a medida (par)", "Custom insoles (pair)"), desde(120)),
              (("Revisión y ajuste de plantillas", "Insole check and adjustment"), eur(20)),
              (("Plantillas infantiles (par)", "Children's insoles (pair)"), desde(90))]),
        ], nota=("Precios de ejemplo: cámbialos por los tuyos.",
                 "Example prices: replace them with your own."),
            tono="relleno", anchor="precios-plantillas"),
        bloque_texto(("Los primeros días con plantillas", "The first days with insoles"), [
            ("Pueden notarse raras al principio. Úsalas unas horas al día y ve subiendo poco a poco, como te indiquemos.",
             "They can feel strange at first. Wear them a few hours a day and build up gradually, as we advise."),
            ("Si molestan o aparece una rozadura, no aguantes: vuelve y las ajustamos. Con el tiempo se desgastan, así que conviene revisarlas.",
             "If they hurt or rub, do not put up with it: come back and we will adjust them. They wear over time, so it is worth having them checked."),
        ], tono="base"),
        bloque_cta(("¿Quieres que te estudiemos la pisada?", "Would you like your gait studied?"),
                   ("Pide cita y trae el calzado que más usas.", "Book a visit and bring the shoes you wear most."),
                   [cita(), whatsapp("ghost")], tono="relleno"),
    ])

    cont = pagina("contacto", ("contacto", "contact"), ("Contacto y cita", "Contact and appointments"),
                  ("Pide cita en la clínica de podología por teléfono, WhatsApp o formulario. Horario y cómo llegar.",
                   "Book at the podiatry clinic by phone, WhatsApp or form. Opening hours and how to get here."), [
        hero(("Pide tu cita", "Book your appointment"),
             ("Llámanos, escríbenos por WhatsApp o rellena el formulario. Te confirmamos el mismo día. Si hay herida, dolor fuerte o fiebre, llama.",
              "Call us, message us on WhatsApp or fill in the form. We confirm the same day. If there is a wound, severe pain or fever, call."),
             [llamar(), whatsapp()], variante="texto"),
        bloque_contacto(("Cómo encontrarnos", "How to find us"),
                        ("Planta baja, sin escalones.", "Ground floor, no steps."),
                        campos_cita(servicios=[("Primera visita", "First visit"), ("Quiropodia", "Routine foot care"),
                                               ("Uña encarnada", "Ingrown toenail"), ("Pie diabético", "Diabetic foot"),
                                               ("Estudio de la pisada", "Gait study"), ("Otro motivo", "Something else")],
                                    extra=[campo("paciente", "select", ("¿Has venido antes?", "Have you visited before?"), False,
                                                 opciones=[("Es mi primera visita", "This is my first visit"),
                                                           ("Ya soy paciente", "I am already a patient")])]),
                        ("Pedir cita", "Request appointment"), horas,
                        notas=_junta(COLEGIADO, DATOS),
                        exito=("Gracias. Te confirmamos la cita por teléfono o WhatsApp en cuanto la tengamos.",
                               "Thank you. We will confirm your appointment by phone or WhatsApp as soon as we have it.")),
        bloque_mapa(tono="relleno"),
    ])

    footer = pie(nav, ("Clínica de podología con cita previa. Quiropodia, pie diabético y plantillas a medida.",
                       "Podiatry clinic by appointment. Foot care, diabetic foot and custom insoles."),
                 ("Lunes a viernes 10:00 a 14:00 y 16:00 a 20:00. Sábado 10:00 a 13:00.",
                  "Monday to Friday 10 am to 2 pm and 4 pm to 8 pm. Saturday 10 am to 1 pm."))
    footer["children"].insert(1, peq(COLEGIADO, color="$muted"))

    return plantilla(KEY, ("Clínica de podología", "Podiatry clinic"),
                     ("Clínica de podología con tratamientos, duración y precios orientativos, estudio de la pisada, plantillas a medida, equipo colegiado y cita por WhatsApp.",
                      "Podiatry clinic with treatments, timings and indicative prices, gait study, custom insoles, registered team and WhatsApp booking."),
                     "salud", ("podologo podologia clinica de podologia quiropodia una encarnada unero pie diabetico plantillas a medida estudio de la pisada pisada callos durezas pies",
                               "podiatrist podiatry chiropody foot clinic ingrown toenail diabetic foot insoles orthotics gait analysis corns feet"),
                     look, cabecera(nav, marco([llamar(), whatsapp()], dir="row", gap=12, wrap=True)), footer,
                     [inicio, trat, plant, cont])
