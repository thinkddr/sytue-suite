# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
from lib import *
from looks import LOOKS  # noqa: F401  (paleta propia más abajo)

KEY = "estudio-tatuajes"

# Negro tinta con acento hueso: oscura y sobria, sin neón. Sin fondo $inverse con primary bajo (primary/inverse > 3).
TINTA_HUESO = dict(background="#100f0f", surface="#1b1a19", text="#efe9df", muted="#b4aca0", primary="#e3d6bd",
                   on_primary="#100f0f", border="#33302d", inverse="#050505", on_inverse="#efe9df")

NOTA = ("Precios de ejemplo: cámbialos por los tuyos. El precio final se cierra tras el boceto.",
        "Example prices: replace them with your own. The final price is agreed after the sketch.")
REGISTRO = ("Nº de registro sanitario: 00-0000", "Health registration no.: 00-0000")
EDAD = ("Solo atendemos a mayores de 18 años y pedimos documento de identidad.",
        "We only work with people aged 18 or over and ask for photo ID.")


def build():
    nueva(KEY)
    look = dict(colores=TINTA_HUESO, fuentes=("work-sans", "ibm-plex-sans"), peso=700, mayus="uppercase", ls=0.05,
                h1=(50, 40, 32), radio=(0, 2, 4), ancho=1160)
    nav = [("inicio", ("Inicio", "Home")), ("diseno", ("Estilos y proceso", "Styles and process")),
           ("higiene", ("Higiene y cuidados", "Hygiene and aftercare")), ("contacto", ("Reserva", "Booking"))]
    horas = [(("Lunes", "Monday"), ("Cerrado", "Closed")),
             (("Martes a sábado", "Tuesday to Saturday"), ("11:00 a 20:00, con cita", "11:00 am to 8:00 pm, by appointment")),
             (("Domingo", "Sunday"), ("Cerrado", "Closed"))]

    def reservar(v="primary"):
        return boton(("Reservar fecha", "Book a date"), a_pagina("contacto", "contacto"), v)

    inicio = pagina("inicio", ("", ""), ("Estudio de tatuajes y piercing", "Tattoo and piercing studio"),
                    ("Estudio de tatuajes y piercing con cita previa: estilos, galería de trabajos, material estéril de un solo uso y reserva de fecha.",
                     "Tattoo and piercing studio by appointment: styles, gallery of work, sterile single-use equipment and date booking."), [
        hero(("Tatuajes pensados contigo, hechos con calma", "Tattoos designed with you, made without rushing"),
             ("Primero hablamos de tu idea, después dibujamos el boceto y solo entonces fijamos la cita. Todo el material que toca tu piel es estéril y de un solo uso.",
              "First we talk through your idea, then we draw the sketch, and only then do we set the date. Everything that touches your skin is sterile and single-use."),
             [reservar(), whatsapp()],
             eyebrow=("Tatuajes y piercing", "Tattoos and piercing"),
             imagen_=("portrait", ("Persona tatuándose el antebrazo en el estudio", "Person being tattooed on the forearm at the studio"))),
        bloque_valores(("Estilos con los que trabajamos", "Styles we work in"), [
            (("Línea fina", "Fine line"), ("Trazo delgado y detalle pequeño: flores, letras, símbolos.", "Thin lines and small detail: flowers, lettering, symbols.")),
            (("Negro y gris", "Black and grey"), ("Sombreado y realismo, de retratos a escenas.", "Shading and realism, from portraits to scenes.")),
            (("Tradicional", "Traditional"), ("Contornos gruesos y color plano. Anclas, rosas, panteras.", "Bold outlines and flat colour. Anchors, roses, panthers.")),
            (("Color y neotradicional", "Colour and neo-traditional"), ("Más paleta, más volumen y más libertad que el tradicional.", "More palette, more volume and more freedom than the traditional style.")),
            (("Lettering", "Lettering"), ("Frases, nombres y fechas con una caligrafía hecha a medida.", "Phrases, names and dates in custom-drawn lettering.")),
            (("Blackwork y geométrico", "Blackwork and geometric"), ("Formas puras, mandalas y grandes manchas de negro.", "Clean shapes, mandalas and large areas of solid black.")),
            (("Tapados y retoques", "Cover-ups and touch-ups"), ("Si tienes un tatuaje que ya no te gusta, lo vemos y te decimos qué se puede hacer.", "If you have a tattoo you no longer like, we look at it and tell you what can be done.")),
            (("Piercing", "Piercing"), ("Oreja, nariz, ombligo y más, con joyería de titanio. Sin pistola.", "Ear, nose, navel and more, with titanium jewellery. No piercing gun.")),
        ], cols=4, eyebrow=("Estilos", "Styles"), tono="relleno", estilo="borde", anchor="estilos"),
        bloque_galeria(("Trabajos recientes", "Recent work"), [
            ("Tatuaje de línea fina en el antebrazo realizado en el estudio", "Fine-line tattoo on the forearm done at the studio"),
            ("Tatuaje en negro y gris sobre el hombro realizado en el estudio", "Black and grey tattoo on the shoulder done at the studio"),
            ("Tatuaje tradicional en color en la pantorrilla realizado en el estudio", "Traditional colour tattoo on the calf done at the studio"),
            ("Lettering en la clavícula realizado en el estudio", "Lettering on the collarbone done at the studio"),
            ("Diseño geométrico en el brazo realizado en el estudio", "Geometric design on the arm done at the studio"),
            ("Piercing de oreja con joyería de titanio realizado en el estudio", "Ear piercing with titanium jewellery done at the studio")],
            cols=3, tono="base", ratio=1.0),
        bloque_pasos(("De la idea a la cita", "From idea to appointment"), [
            (("Cuéntanos tu idea", "Tell us your idea"),
             ("Por mensaje o en el estudio: qué quieres, dónde y de qué tamaño. Trae referencias si las tienes.",
              "By message or at the studio: what you want, where and how big. Bring references if you have them.")),
            (("Boceto", "Sketch"),
             ("Dibujamos tu diseño y lo ajustamos contigo hasta que te convence. No empezamos hasta entonces.",
              "We draw your design and adjust it with you until you are happy. We do not start until then.")),
            (("Cita y depósito", "Appointment and deposit"),
             ("Fijamos día y hora. Se paga un depósito que se descuenta del precio final.",
              "We set the day and time. A deposit is paid and deducted from the final price.")),
            (("Tatuaje", "Tattoo"),
             ("Calcamos el diseño sobre la piel, lo revisas y empezamos. Puedes parar cuando lo necesites.",
              "We transfer the design to your skin, you check it, and we begin. You can stop whenever you need to.")),
        ], tono="relleno"),
        bloque_checks(("Limpio de verdad", "Properly clean"), [
            ("Agujas y cartuchos estériles de un solo uso", "Sterile single-use needles and cartridges"),
            ("Guantes nuevos y superficies protegidas para cada persona", "New gloves and protected surfaces for every client"),
            ("Material reutilizable esterilizado en autoclave", "Reusable tools sterilised in an autoclave"),
            ("Tintas en envases individuales y cerrados", "Inks in sealed individual caps"),
        ], intro=("Lo abrimos delante de ti. Si algo no te cuadra, pregúntanos.", "We open it in front of you. If anything seems off, ask us."),
            eyebrow=("Higiene", "Hygiene"), cols=2, tono="base"),
        bloque_opiniones(("Lo que dicen nuestros clientes", "What our clients say"),
                         [("el boceto", "the sketch"), ("el trato", "the service"), ("la higiene", "the hygiene")], tono="relleno"),
        bloque_cta(("¿Tienes una idea?", "Have an idea in mind?"),
                   ("Cuéntanosla y te decimos cómo lo haríamos, cuánto puede costar y cuándo tenemos hueco.",
                    "Tell us and we will explain how we would do it, roughly what it could cost and when we have a slot."),
                   [reservar(), whatsapp("ghost")], tono="base"),
    ])

    diseno = pagina("diseno", ("estilos-y-proceso", "styles-and-process"), ("Estilos y proceso", "Styles and process"),
                    ("Cómo se diseña un tatuaje en el estudio: consulta, boceto, depósito y cita, con precios orientativos de tatuajes y piercing.",
                     "How a tattoo is designed at the studio: consultation, sketch, deposit and appointment, with indicative tattoo and piercing prices."), [
        hero(("Estilos y proceso", "Styles and process"),
             ("Un tatuaje es para siempre, así que no hay prisa. Así llegamos del «quiero algo así» al diseño final.",
              "A tattoo is permanent, so there is no rush. This is how we get from “I want something like this” to the final design."),
             [reservar()], variante="texto"),
        bloque_texto(("La consulta", "The consultation"), [
            ("Antes de dibujar hablamos de qué significa para ti, dónde lo quieres y cómo envejece cada zona del cuerpo. Algunos diseños funcionan mejor grandes o con más contraste, y te lo decimos con franqueza.",
             "Before drawing we talk about what it means to you, where you want it and how each part of the body ages. Some designs work better larger or with more contrast, and we tell you plainly."),
            ("La consulta es gratuita y sin compromiso. Puedes hacerla por WhatsApp con fotos, o en el estudio.",
             "The consultation is free and without obligation. You can do it on WhatsApp with photos, or at the studio."),
        ], imagen_=("landscape", ("Mesa del estudio con bocetos y referencias", "Studio desk with sketches and references")),
            tono="base", anchor="consulta"),
        bloque_texto(("Boceto y depósito", "Sketch and deposit"), [
            ("Dibujamos el diseño a partir de tu idea y te lo enseñamos antes de la cita. Los cambios pequeños se hacen sin coste; si lo rehacemos entero, lo hablamos.",
             "We draw the design from your idea and show it to you before the appointment. Small changes are free; if we redo it entirely, we talk it over."),
            ("Para reservar fecha pedimos un depósito. Se descuenta del precio final y no se devuelve si cancelas con menos de 48 horas.",
             "To book a date we ask for a deposit. It is deducted from the final price and not refunded if you cancel with less than 48 hours' notice."),
        ], tono="relleno", anchor="deposito", invertir=True,
            imagen_=("photo", ("Boceto de un tatuaje sobre papel", "Tattoo sketch on paper"))),
        bloque_precios(("Precios orientativos", "Indicative prices"), ("Precios de tatuajes y piercing", "Tattoo and piercing prices"),
                       [("Trabajo", "Work"), ("Precio", "Price")],
                       [(("Tatuaje pequeño (hasta 5 cm)", "Small tattoo (up to 5 cm)"), desde(60)),
                        (("Tatuaje mediano", "Medium tattoo"), desde(120)),
                        (("Sesión de media jornada (unas 4 horas)", "Half-day session (about 4 hours)"), eur(280)),
                        (("Sesión de jornada completa (unas 7 horas)", "Full-day session (about 7 hours)"), eur(450)),
                        (("Piercing (sin joya)", "Piercing (jewellery not included)"), desde(20)),
                        (("Joya de titanio", "Titanium jewellery"), desde(8)),
                        (("Depósito para reservar fecha", "Deposit to book a date"), eur(30))],
                       intro=("Cada pieza es distinta: el precio final depende del tamaño, el detalle y las horas.",
                              "Every piece is different: the final price depends on size, detail and hours."),
                       nota=NOTA, tono="base", anchor="precios"),
        bloque_cta(("¿Hablamos de tu diseño?", "Shall we talk about your design?"), None,
                   [reservar(), whatsapp("ghost")], tono="relleno"),
    ])

    higiene = pagina("higiene", ("higiene-y-cuidados", "hygiene-and-aftercare"), ("Higiene y cuidados", "Hygiene and aftercare"),
                     ("Higiene y material estéril de un solo uso en el estudio, y cuidados del tatuaje paso a paso. Solo mayores de 18 años.",
                      "Hygiene and sterile single-use equipment at the studio, and tattoo aftercare step by step. Adults aged 18 and over only."), [
        hero(("Higiene y cuidados", "Hygiene and aftercare"),
             ("Un tatuaje es una herida controlada: el estudio la hace en un entorno limpio y tú la cuidas durante unas semanas.",
              "A tattoo is a controlled wound: the studio makes it in a clean setting and you look after it for a few weeks."),
             [reservar()], variante="texto"),
        bloque_valores(("Cómo trabajamos para que sea seguro", "How we work to keep it safe"), [
            (("Un solo uso", "Single use"),
             ("Agujas, cartuchos, guantes, protectores y recipientes de tinta son nuevos para cada persona y se desechan después.",
              "Needles, cartridges, gloves, barriers and ink caps are new for each person and thrown away afterwards.")),
            (("Esterilización", "Sterilisation"),
             ("Lo que se reutiliza se limpia y se esteriliza en autoclave en bolsas selladas, que abrimos delante de ti.",
              "Anything reusable is cleaned and sterilised in an autoclave in sealed pouches, which we open in front of you.")),
            (("Estudio limpio", "Clean studio"),
             ("Superficies cubiertas y desinfectadas entre cliente y cliente, y zona de trabajo separada de la de espera.",
              "Surfaces covered and disinfected between clients, and a work area separate from the waiting area.")),
        ], cols=3, tono="base", estilo="relleno", anchor="higiene"),
        bloque_texto(("Mayores de 18 y registro sanitario", "Over 18s and health registration"), [
            EDAD,
            ("Si estás embarazada, tomas anticoagulantes, tienes una enfermedad de la piel o una alergia importante, cuéntanoslo antes de reservar: puede que te pidamos hablar antes con tu médico.",
             "If you are pregnant, take blood thinners, have a skin condition or a significant allergy, tell us before you book: we may ask you to speak to your doctor first."),
            REGISTRO,
        ], tono="relleno", anchor="registro"),
        bloque_pasos(("Cuidados después del tatuaje", "Tattoo aftercare"), [
            (("Primeras horas", "First hours"),
             ("Deja el protector que te pongamos entre 2 y 24 horas, según el tipo, y retíralo con las manos limpias.",
              "Keep the dressing we apply on for 2 to 24 hours, depending on its type, and remove it with clean hands.")),
            (("Lava", "Wash"),
             ("Con agua tibia y jabón neutro, sin frotar. Seca a toquecitos con papel limpio.",
              "With lukewarm water and mild soap, without rubbing. Pat dry with clean paper.")),
            (("Hidrata", "Moisturise"),
             ("Una capa muy fina de la crema que te indiquemos, dos o tres veces al día durante las primeras semanas.",
              "A very thin layer of the cream we recommend, two or three times a day for the first few weeks.")),
            (("Evita", "Avoid"),
             ("Sol, piscina, mar, sauna y rascarte las costras durante al menos dos semanas.",
              "Sun, pools, the sea, saunas and picking at scabs for at least two weeks.")),
            (("Vigila", "Watch"),
             ("Si hay mucho enrojecimiento, pus, fiebre o dolor que va a más, contacta con tu médico y avísanos.",
              "If there is heavy redness, pus, a fever or growing pain, contact your doctor and let us know.")),
        ], intro=("Te damos estas pautas por escrito al terminar. Cada piel cicatriza distinto: ante la duda, escríbenos.",
                  "We give you these instructions in writing when we finish. Every skin heals differently: if in doubt, write to us."),
            tono="base", anchor="cuidados"),
        bloque_faq(("Preguntas frecuentes", "Frequently asked questions"), [
            (("¿Duele?", "Does it hurt?"),
             ("Sí, algo. Depende de la zona y de cada persona: costillas, pies y codos suelen molestar más. Hacemos pausas cuando las necesitas.",
              "Yes, a little. It depends on the area and the person: ribs, feet and elbows tend to be worse. We take breaks whenever you need them.")),
            (("¿Cuánto tarda en curar?", "How long does it take to heal?"),
             ("La piel cierra en dos o tres semanas, pero por dentro tarda más. Para valorar el resultado final, espera un par de meses.",
              "The skin closes in two or three weeks but takes longer to settle underneath. To judge the final result, wait a couple of months.")),
            (("¿Puedo tatuarme sin cita?", "Can I get tattooed without an appointment?"),
             ("No. Trabajamos con cita para tener el diseño listo y el tiempo reservado.",
              "No. We work by appointment so the design is ready and the time is set aside.")),
        ], tono="relleno"),
        bloque_cta(("¿Reservamos fecha?", "Shall we book a date?"), None, [reservar(), whatsapp("ghost")], tono="base"),
    ])

    cont = pagina("contacto", ("contacto", "contact"), ("Reserva y contacto", "Booking and contact"),
                  ("Reserva fecha para tu tatuaje o piercing por teléfono, WhatsApp o con el formulario. Horario y cómo llegar al estudio.",
                   "Book a date for your tattoo or piercing by phone, WhatsApp or with the form. Opening hours and how to get to the studio."), [
        hero(("Reserva tu fecha", "Book your date"),
             ("Cuéntanos tu idea y te respondemos con el siguiente paso. Si tienes referencias, envíalas por WhatsApp.",
              "Tell us your idea and we will reply with the next step. If you have references, send them on WhatsApp."),
             [llamar(), whatsapp()], variante="texto"),
        bloque_contacto(("Cómo encontrarnos", "How to find us"),
                        ("Solo con cita. Cuando la fijemos, te decimos cómo llegar y dónde dejar la bici o el coche.",
                         "By appointment only. When we set it, we tell you how to get here and where to leave your bike or car."),
                        campos_cita(servicios=[("Tatuaje", "Tattoo"), ("Tapado o retoque", "Cover-up or touch-up"),
                                               ("Piercing", "Piercing")],
                                    extra=[campo("zona", "text", ("Zona del cuerpo", "Part of the body"), False, maxlen=100),
                                           campo("tamano", "select", ("Tamaño aproximado", "Approximate size"), False,
                                                 opciones=[("Pequeño (hasta 5 cm)", "Small (up to 5 cm)"),
                                                           ("Mediano", "Medium"), ("Grande", "Large"),
                                                           ("No lo sé", "I do not know")])]),
                        ("Pedir fecha", "Request a date"), horas,
                        notas=(EDAD[0] + " " + REGISTRO[0] + ".", EDAD[1] + " " + REGISTRO[1] + "."),
                        exito=("Gracias. Te respondemos por teléfono o WhatsApp para seguir con el boceto y la fecha.",
                               "Thank you. We will reply by phone or WhatsApp to go on with the sketch and the date.")),
        bloque_mapa(tono="relleno"),
    ])

    return plantilla(KEY, ("Estudio de tatuajes y piercing", "Tattoo and piercing studio"),
                     ("Estudio de tatuajes y piercing oscuro y sobrio con estilos, galería, proceso de diseño, precios orientativos, higiene, cuidados y reserva de fecha.",
                      "Dark, restrained tattoo and piercing studio with styles, gallery, design process, indicative prices, hygiene, aftercare and date booking."),
                     "belleza", ("estudio de tatuajes tatuajes tatuador tatuadora tattoo tattoo studio piercing perforaciones tapados retoques lettering linea fina",
                                 "tattoo studio tattoo artist tattoos piercing body piercing cover-up fine line lettering"),
                     look, cabecera(nav, reservar()),
                     pie(nav, ("Estudio de tatuajes y piercing con cita previa. Material estéril de un solo uso.",
                               "Tattoo and piercing studio by appointment. Sterile single-use equipment."),
                         ("Martes a sábado 11:00 a 20:00, con cita.", "Tuesday to Saturday 11 am to 8 pm, by appointment.")),
                     [inicio, diseno, higiene, cont])
