# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
from lib import *
from looks import LOOKS  # noqa: F401  (la paleta es propia de este sector)

KEY = "ortopedia"

# Ciruela baya sobre un rosa grisáceo muy pálido. Sin fondo oscuro (nada usa $inverse).
BAYA = dict(
    colores=dict(background="#f9f5f8", surface="#eee2ea", text="#26141f", muted="#65505c", primary="#7a3566",
                 on_primary="#ffffff", border="#e0d0da", inverse="#2d1426", on_inverse="#f9eef5"),
    fuentes=("ibm-plex-sans", "figtree"), peso=600, cuerpo=(17, 16, 16), radio=(6, 10, 14), ancho=1120)

DATOS = ("Tus datos y los de tu receta o informe médico se tratan con confidencialidad y según la normativa de protección de datos, y solo los ve quien te atiende.",
         "Your details and those of your prescription or medical report are handled in confidence and under data protection rules, and only the people serving you see them.")
NOTA = ("Precios de ejemplo: cámbialos por los tuyos. La fianza se devuelve al entregar el material en buen estado.",
        "Example prices: replace them with your own. The deposit is returned when the equipment comes back in good condition.")


def _tecnico(titulo, personas, intro=None, tono="base"):
    """Ficha del personal con el número de colegiado visible (relleno detectable) en el técnico ortopeda."""
    tarjetas = []
    for rol, frase, colegiado in personas:
        hijos = [imagen("portrait", ("Foto de la persona del equipo", "Photo of the team member"), ratio=0.9),
                 h3(("Nombre Apellido", "First Last")), peq(rol, color="$muted"), p(frase)]
        if colegiado:
            hijos.append(peq(COLEGIADO, color="$muted"))
        tarjetas.append(marco(hijos, tag="li", gap=10))
    return seccion("team", [titular(titulo, intro), rejilla(tarjetas, cols=len(personas))], tono=tono, anchor="equipo")


def build():
    nueva(KEY)
    look = BAYA
    nav = [("inicio", ("Inicio", "Home")), ("productos", ("Productos", "Products")),
           ("servicios", ("Alquiler y receta", "Rental and prescriptions")), ("contacto", ("Contacto", "Contact"))]
    horas = [(("Lunes a viernes", "Monday to Friday"), ("9:30 a 13:30 y 16:30 a 20:00", "9:30 am to 1:30 pm and 4:30 pm to 8:00 pm")),
             (("Sábado", "Saturday"), ("10:00 a 13:30", "10:00 am to 1:30 pm")),
             (("Domingo y festivos", "Sunday and public holidays"), ("Cerrado", "Closed")),
             (("Toma de medidas y visitas a domicilio", "Fittings and home visits"), ("Con cita previa", "By appointment"))]
    cta_cab = marco([llamar(t=("Llamar", "Call")), whatsapp("secondary", t=("WhatsApp", "WhatsApp"))],
                    dir="row", gap=8, wrap=True)
    productos = lambda v="primary": boton(("Ver productos", "See products"), a_pagina("productos"), v)
    cita = lambda v="primary": boton(("Pedir cita", "Book an appointment"), a_pagina("contacto", "cita"), v)

    inicio = pagina("inicio", ("", ""), ("Ortopedia y ayudas técnicas", "Orthopaedic and mobility shop"),
                    ("Tienda de ortopedia con productos para moverte, descansar en casa y recuperarte, alquiler de material, prótesis y ortesis a medida, y servicio a domicilio.",
                     "Orthopaedic shop with products for getting around, resting at home and recovering, equipment rental, made-to-measure prosthetics and orthoses, and a home service."), [
        hero(("Ayudas técnicas para moverte, descansar y recuperarte", "Aids to help you get around, rest and recover"),
             ("Productos de ortopedia, alquiler de camas y andadores, y prótesis y ortesis a medida. Te explicamos cómo se usa cada cosa y te lo ajustamos.",
              "Orthopaedic products, rental of beds and walkers, and made-to-measure prosthetics and orthoses. We explain how each thing is used and adjust it for you."),
             [llamar(), whatsapp(), productos("ghost")],
             eyebrow=("Ortopedia y ayudas técnicas", "Orthopaedics and mobility aids"),
             imagen_=("photo", ("Escaparate de una ortopedia con un andador, bastones y una silla de ruedas plegable",
                                "Orthopaedic shop window with a walker, walking sticks and a folding wheelchair"))),
        bloque_valores(("Qué necesitas", "What you need"), [
            (("Moverte", "Getting around"),
             ("Bastones, muletas, andadores y sillas de ruedas. Te ayudamos a elegir la altura y el modelo que mejor te vaya.",
              "Sticks, crutches, walkers and wheelchairs. We help you choose the height and model that suits you best.")),
            (("Descansar en casa", "Resting at home"),
             ("Camas articuladas, colchones y cojines antiescaras, y barandillas para dormir y cuidar con más comodidad.",
              "Adjustable beds, pressure-relief mattresses and cushions, and bed rails to sleep and care for someone more comfortably.")),
            (("Recuperarte", "Recovering"),
             ("Rodilleras, tobilleras, fajas, collarines, férulas y medias de compresión, para después de una lesión o una operación.",
              "Knee supports, ankle supports, back supports, neck collars, splints and compression stockings, for after an injury or surgery.")),
            (("Baño y vida diaria", "Bathroom and daily living"),
             ("Asientos de ducha, elevadores de WC, barras de apoyo y alargadores para ganar autonomía en casa.",
              "Shower seats, raised toilet seats, grab rails and reachers to help you manage at home.")),
            (("A medida", "Made to measure"),
             ("Plantillas, ortesis y prótesis hechas para ti, tras una toma de medidas y una o varias pruebas.",
              "Insoles, orthoses and prosthetics made for you, after a fitting and one or more trials.")),
            (("Alquiler", "Rental"),
             ("Para unas semanas o unos meses: camas, andadores, sillas, grúas y muletas, con entrega a domicilio.",
              "For a few weeks or months: beds, walkers, wheelchairs, hoists and crutches, with home delivery.")),
        ], eyebrow=("Productos", "Products"), cols=3, estilo="borde", anchor="necesidades"),
        bloque_texto(("Vamos a tu casa", "We come to your home"), [
            ("Si no puedes desplazarte, nos pasamos por casa a tomarte medidas, a entregarte la cama articulada o a montarla. Vemos cómo es el dormitorio y el baño para recomendarte lo que de verdad cabe y te sirve.",
             "If you cannot travel, we come to your home to take your measurements, deliver the adjustable bed or set it up. We look at the bedroom and bathroom so we can recommend what really fits and helps."),
            ("La visita se pide por teléfono o por WhatsApp. Te decimos de antemano qué cuesta el desplazamiento, si lo tiene.",
             "You can book the visit by phone or on WhatsApp. We tell you beforehand what the travel costs, if anything."),
        ], imagen_=("landscape", ("Dormitorio con una cama articulada junto a una mesilla y una butaca", "Bedroom with an adjustable bed beside a bedside table and an armchair")),
            tono="relleno", anchor="domicilio"),
        _tecnico(("Quién te atiende", "Who looks after you"), [
            (("Técnico ortopeda", "Orthopaedic technician"),
             ("Toma las medidas, prepara las ortesis y prótesis a medida y revisa que todo te siente bien.",
              "Takes your measurements, prepares made-to-measure orthoses and prosthetics and checks everything fits you well."), True),
            (("Atención en tienda", "Shop assistant"),
             ("Te enseña los productos, te explica cómo se usan y te ayuda con el alquiler y los pedidos.",
              "Shows you the products, explains how they are used and helps with rental and orders."), False),
        ], intro=("Para los productos a medida, quien te atiende es un profesional titulado. Aquí va su número de colegiado.",
                  "For made-to-measure products, you are seen by a qualified professional. Their registration number is shown here.")),
        bloque_opiniones(("Lo que dicen los clientes", "What customers say"),
                         [("el asesoramiento", "the advice"), ("el servicio a domicilio", "the home service"), ("el ajuste del producto", "the way the product was fitted")],
                         tono="relleno"),
        bloque_cta(("¿No sabes qué necesitas?", "Not sure what you need?"),
                   ("Cuéntanos qué te pasa o qué necesita tu familiar y te orientamos. Mejor si traes el informe o la receta.",
                    "Tell us what is going on or what your relative needs and we will point you in the right direction. It helps if you bring the report or prescription."),
                   [llamar(), cita("secondary")], tono="base"),
    ])

    prod = pagina("productos", ("productos", "products"), ("Productos", "Products"),
                  ("Ayudas técnicas por necesidad: movilidad, descanso en casa, rehabilitación, baño y vida diaria, y prótesis y ortesis a medida.",
                   "Aids by need: mobility, resting at home, rehabilitation, bathroom and daily living, and made-to-measure prosthetics and orthoses."), [
        hero(("Productos por necesidad", "Products by need"),
             ("Todo lo que ves aquí se prueba en la tienda. Si tienes dudas sobre cuál elegir, pregunta: te ayudamos a escoger el más sencillo que te sirva.",
              "Everything you see here can be tried in the shop. If you are unsure which to choose, ask: we help you pick the simplest one that works for you."),
             [llamar(), cita("secondary")], variante="texto", tono="relleno"),
        bloque_valores(("Movilidad", "Mobility"), [
            (("Bastones y muletas", "Sticks and crutches"),
             ("Regulables en altura, con puño ergonómico o antideslizante. Te los ajustamos a tu estatura.",
              "Height-adjustable, with ergonomic or non-slip handles. We adjust them to your height.")),
            (("Andadores y rollators", "Walkers and rollators"),
             ("Con ruedas o sin ellas, plegables, con asiento o con cesta. Te explicamos cuál conviene según cómo andas.",
              "With or without wheels, folding, with a seat or a basket. We explain which suits you depending on how you walk.")),
            (("Sillas de ruedas", "Wheelchairs"),
             ("Para uso diario o para salir de casa, ligeras o más firmes, de empuje o autopropulsadas.",
              "For daily use or going out, lightweight or sturdier, attendant-propelled or self-propelled.")),
        ], cols=3, estilo="relleno", tono="base", anchor="movilidad"),
        bloque_valores(("Descanso y cuidado en casa", "Rest and care at home"), [
            (("Camas articuladas", "Adjustable beds"),
             ("Con elevación de cabecera y piernas, y barandillas. Entrega y montaje en casa.",
              "With head and leg elevation, and bed rails. Delivered and set up at home.")),
            (("Colchones y cojines", "Mattresses and cushions"),
             ("Antiescaras, de espuma o de aire, para quien pasa muchas horas en cama o sentado.",
              "Pressure-relief, foam or air, for those who spend many hours in bed or sitting.")),
            (("Grúas y ayudas para levantarse", "Hoists and rising aids"),
             ("Para pasar de la cama al sillón con menos esfuerzo para quien cuida y para quien es cuidado.",
              "To move from bed to armchair with less effort for both carer and person cared for.")),
        ], cols=3, estilo="borde", tono="relleno", anchor="descanso"),
        seccion("text", [titular(("Rehabilitación y recuperación", "Rehabilitation and recovery"),
                                 ("Para después de una lesión o de una operación. Sigue siempre las indicaciones de tu médico o fisioterapeuta.",
                                  "For after an injury or an operation. Always follow your doctor's or physiotherapist's instructions.")),
                         tabla(("Productos de rehabilitación y para qué se usan", "Rehabilitation products and what they are used for"),
                               [("Producto", "Product"), ("Para qué se usa", "What it is used for"), ("Talla o medida", "Size or fitting")],
                               [(("Rodilleras y tobilleras", "Knee and ankle supports"), ("Sujetar y dar estabilidad a la articulación", "Holding and stabilising the joint"), ("Talla, a veces medida", "Size, sometimes made to measure")),
                                (("Fajas lumbares", "Back supports"), ("Sostener la zona de la espalda mientras se mueve", "Supporting the back area while moving"), ("Medida de cintura", "Waist measurement")),
                                (("Collarines", "Neck collars"), ("Limitar el movimiento del cuello cuando lo indica el médico", "Limiting neck movement when the doctor advises it"), ("Altura de cuello", "Neck height")),
                                (("Férulas y muñequeras", "Splints and wrist supports"), ("Inmovilizar o proteger la mano y la muñeca", "Immobilising or protecting the hand and wrist"), ("Talla y lado", "Size and side")),
                                (("Medias de compresión", "Compression stockings"), ("Para piernas cansadas o con indicación médica", "For tired legs or on medical advice"), ("Medida de pierna", "Leg measurement"))])],
                tono="base", anchor="rehabilitacion", gap=28),
        bloque_checks(("Baño y vida diaria", "Bathroom and daily living"), [
            ("Asientos y taburetes de ducha, regulables y antideslizantes.", "Shower seats and stools, adjustable and non-slip."),
            ("Elevadores de WC y reposabrazos para sentarse y levantarse mejor.", "Raised toilet seats and arm rests to sit down and get up more easily."),
            ("Barras de apoyo y asideros, que se montan con o sin obra.", "Grab rails and handles, fitted with or without building work."),
            ("Alargadores, abotonadores y utensilios adaptados para comer y vestirse.", "Reachers, button hooks and adapted utensils for eating and dressing."),
        ], tono="relleno", cols=2, anchor="vida-diaria"),
        bloque_texto(("Prótesis y ortesis a medida", "Made-to-measure prosthetics and orthoses"), [
            ("Las plantillas, las ortesis y las prótesis a medida se hacen para una sola persona. Primero el técnico ortopeda te toma las medidas, después se fabrica y se prueba, y se ajusta hasta que te siente bien.",
             "Insoles, orthoses and prosthetics made to measure are made for one person only. First the orthopaedic technician takes your measurements, then it is made and tried on, and adjusted until it fits you well."),
            ("El plazo y el precio dependen de cada caso. Te los decimos después de la toma de medidas, antes de empezar a fabricar.",
             "The time and price depend on each case. We tell you after the fitting, before we start making it."),
        ], imagen_=("product", ("Plantillas ortopédicas a medida sobre una mesa de trabajo junto a un metro", "Custom orthopaedic insoles on a workbench next to a tape measure")),
            tono="base", anchor="a-medida", invertir=True),
        bloque_cta(("¿Quieres probarlo?", "Would you like to try it?"), None, [cita(), whatsapp("ghost")], tono="relleno"),
    ])

    serv = pagina("servicios", ("alquiler-y-receta", "rental-and-prescriptions"), ("Alquiler y receta", "Rental and prescriptions"),
                  ("Alquiler de material con tarifas, cómo son la receta y la toma de medidas, prestación ortoprotésica y preguntas frecuentes.",
                   "Equipment rental with rates, how prescriptions and fittings work, orthoprosthetic benefit and frequently asked questions."), [
        hero(("Alquiler, receta y medidas", "Rental, prescriptions and fittings"),
             ("Cuándo alquilar y cuándo comprar, qué papeles hacen falta para lo que va a medida y qué es la prestación ortoprotésica.",
              "When to rent and when to buy, which papers you need for made-to-measure items and what the orthoprosthetic benefit is."),
             [llamar(), cita("secondary")], variante="texto"),
        bloque_precios(("Alquiler de material", "Equipment rental"), ("Tarifas de alquiler por producto", "Rental rates by product"),
                       [("Producto", "Product"), ("Por semana", "Per week"), ("Por mes", "Per month"), ("Fianza", "Deposit")],
                       [(("Cama articulada con barandillas", "Adjustable bed with rails"), eur(18), eur(55), eur(100)),
                        (("Colchón antiescaras", "Pressure-relief mattress"), eur(10), eur(30), eur(60)),
                        (("Andador plegable", "Folding walker"), eur(5), eur(15), eur(30)),
                        (("Silla de ruedas", "Wheelchair"), eur(6), eur(20), eur(50)),
                        (("Muletas, par", "Crutches, pair"), eur(3), eur(8), eur(20)),
                        (("Grúa de traslado", "Transfer hoist"), eur(25), eur(70), eur(150))],
                       intro=("Alquila si lo necesitas para unas semanas o unos meses, por ejemplo tras una operación. Si va a ser para largo, te decimos cuándo sale mejor comprar.",
                              "Rent if you need it for a few weeks or months, for example after an operation. If it will be long term, we tell you when buying works out better."),
                       nota=NOTA, boton_=cita(), tono="relleno", anchor="alquiler"),
        bloque_pasos(("Cómo funciona el alquiler", "How rental works"), [
            (("Eliges el material", "You choose the equipment"), ("Nos dices qué necesitas y desde cuándo. Te lo reservamos.", "You tell us what you need and from when. We reserve it for you.")),
            (("Entrega y montaje", "Delivery and set-up"), ("Te lo llevamos a casa, lo montamos y te enseñamos a usarlo.", "We bring it to your home, set it up and show you how to use it.")),
            (("Lo usas el tiempo que haga falta", "You use it as long as needed"), ("Puedes alargar el alquiler llamándonos. Si algo falla, lo revisamos o te lo cambiamos.", "You can extend the rental by calling us. If something goes wrong, we check it or swap it.")),
            (("Devolución", "Return"), ("Lo recogemos o lo traes. Revisamos que esté bien, lo limpiamos y te devolvemos la fianza.", "We collect it or you bring it. We check it is in good condition, clean it and return your deposit.")),
        ], tono="base", anchor="como-alquilar"),
        bloque_pasos(("Receta y toma de medidas", "Prescriptions and fittings"), [
            (("Vienes con tu informe", "You come with your report"), ("Muchos productos se compran sin receta. Para los que van a medida, trae la prescripción o el informe del médico, si lo tienes.", "Many products can be bought without a prescription. For made-to-measure ones, bring your doctor's prescription or report if you have one.")),
            (("Toma de medidas", "Fitting"), ("El técnico ortopeda te mide y te pregunta cómo usarás el producto: dónde, cuánto y para qué.", "The orthopaedic technician measures you and asks how you will use the product: where, how often and for what.")),
            (("Prueba", "Trial"), ("Probamos el producto y lo ajustamos. Algunos casos necesitan más de una prueba.", "We try the product and adjust it. Some cases need more than one trial.")),
            (("Entrega y revisión", "Delivery and check-up"), ("Te lo entregamos y te explicamos cómo cuidarlo. Vuelves si algo roza o molesta.", "We hand it over and explain how to look after it. Come back if anything rubs or is uncomfortable.")),
        ], tono="relleno", anchor="receta"),
        bloque_texto(("Prestación ortoprotésica", "Orthoprosthetic benefit"), [
            ("Algunos productos pueden estar incluidos en la prestación ortoprotésica del sistema público de salud, con la prescripción de un médico y según el catálogo y las condiciones de cada comunidad. Consulta las vigentes en tu centro de salud o en la web de tu servicio de salud.",
             "Some products may be covered by the public health system's orthoprosthetic benefit, with a doctor's prescription and according to each region's catalogue and conditions. Check the current ones with your health centre or your health service's website."),
            ("Si traes tu prescripción, te explicamos qué trámites suelen hacer falta y qué parte podría quedar a tu cargo. No podemos confirmar de antemano que te lo cubran.",
             "If you bring your prescription, we explain which steps are usually needed and what part you might have to pay. We cannot confirm in advance that it will be covered."),
        ], tono="base", anchor="prestacion"),
        bloque_faq(("Preguntas frecuentes", "Frequently asked questions"), [
            (("¿Necesito receta para comprar un andador o una silla de ruedas?", "Do I need a prescription to buy a walker or wheelchair?"),
             ("Para comprar en la tienda, normalmente no. Si quieres que se financie con la prestación ortoprotésica, sí hará falta la prescripción de un médico.",
              "To buy in the shop, usually not. If you want it covered by the orthoprosthetic benefit, a doctor's prescription will be needed.")),
            (("¿Puedo probar el producto antes de comprarlo?", "Can I try the product before buying?"),
             ("Sí. En la tienda tenemos modelos para probar. Para lo que se hace a medida, haces una prueba antes de la entrega.",
              "Yes. We have models to try in the shop. For made-to-measure items, you have a trial before delivery.")),
            (("¿Cuánto tarda un producto a medida?", "How long does a made-to-measure product take?"),
             ("Depende del producto. Después de la toma de medidas, el técnico te da una fecha por escrito.",
              "It depends on the product. After the fitting, the technician gives you a date in writing.")),
            (("¿Podéis venir a casa?", "Can you come to my home?"),
             ("Sí, con cita previa. Tomamos medidas, entregamos y montamos el material. Te decimos el coste del desplazamiento antes de ir.",
              "Yes, by appointment. We take measurements, deliver and set up the equipment. We tell you the travel cost before we go.")),
            (("¿Cambiáis o arregláis lo que he comprado?", "Do you exchange or repair what I bought?"),
             ("Traes el producto con el justificante de compra y lo revisamos. Si se puede ajustar o reparar, lo hacemos; si no, te explicamos tus opciones.",
              "Bring the product with the proof of purchase and we check it. If it can be adjusted or repaired we do so; if not, we explain your options.")),
        ], tono="base"),
        bloque_cta(("¿Lo hablamos?", "Shall we talk about it?"), None, [cita(), whatsapp("ghost")], tono="relleno"),
    ])

    cont = pagina("contacto", ("contacto", "contact"), ("Contacto y cita", "Contact and appointments"),
                  ("Pide cita en la tienda o a domicilio con el formulario, por teléfono o por WhatsApp. Horario y cómo llegar.",
                   "Book an appointment at the shop or at your home with the form, by phone or on WhatsApp. Opening hours and how to find us."), [
        hero(("Pide cita o pregúntanos", "Book an appointment or ask us"),
             ("Llámanos, escríbenos por WhatsApp o rellena el formulario. Si no sabes qué necesitas, cuéntanos y te orientamos.",
              "Call us, message us on WhatsApp or fill in the form. If you do not know what you need, tell us and we will point you in the right direction."),
             [llamar(), whatsapp()], variante="texto"),
        bloque_contacto(("Datos para tu cita", "Details for your appointment"),
                        ("Este formulario es para pedir cita o presupuesto, no para consultas médicas.",
                         "This form is for booking or asking for a quote, not for medical questions."),
                        campos_cita(servicios=[("Productos de movilidad", "Mobility products"), ("Camas y descanso", "Beds and rest"),
                                               ("Rehabilitación", "Rehabilitation"), ("Prótesis u ortesis a medida", "Made-to-measure prosthetic or orthosis"),
                                               ("Alquiler de material", "Equipment rental"), ("Otra cosa", "Something else")],
                                    extra=[campo("lugar", "select", ("¿Dónde prefieres la cita?", "Where would you prefer the appointment?"), False,
                                                 opciones=[("En la tienda", "At the shop"), ("En mi casa", "At my home")]),
                                           campo("receta", "select", ("¿Tienes receta o informe?", "Do you have a prescription or report?"), False,
                                                 opciones=[("Sí", "Yes"), ("No", "No"), ("No lo sé", "I am not sure")])]),
                        ("Pedir cita", "Request appointment"), horas, anchor="cita",
                        notas=(COLEGIADO[0] + " (técnico ortopeda). " + DATOS[0], COLEGIADO[1] + " (orthopaedic technician). " + DATOS[1]),
                        exito=("Gracias. Te confirmamos la cita por teléfono o por WhatsApp.",
                               "Thank you. We will confirm your appointment by phone or on WhatsApp.")),
        bloque_mapa(("Cómo llegar", "How to find us"),
                    ("La tienda está a pie de calle, sin escalones, con espacio para entrar con andador o silla de ruedas.",
                     "The shop is at street level, without steps, with room to enter with a walker or wheelchair."), tono="relleno"),
    ])

    footer = pie(nav, ("Ortopedia y ayudas técnicas. Alquiler de material y productos a medida.",
                       "Orthopaedics and mobility aids. Equipment rental and made-to-measure products."),
                 ("Lunes a viernes 9:30 a 13:30 y 16:30 a 20:00. Sábado 10:00 a 13:30.",
                  "Monday to Friday 9:30 am to 1:30 pm and 4:30 pm to 8 pm. Saturday 10 am to 1:30 pm."))
    footer["children"].insert(1, peq(COLEGIADO, color="$muted"))

    return plantilla(KEY, ("Ortopedia y ayudas técnicas", "Orthopaedic and mobility shop"),
                     ("Ortopedia con productos por necesidad, alquiler con tarifas de ejemplo, receta y toma de medidas, prestación ortoprotésica, técnico colegiado, servicio a domicilio, horario y mapa.",
                      "Orthopaedic shop with products by need, rental with example rates, prescriptions and fittings, orthoprosthetic benefit, registered technician, a home service, opening hours and a map."),
                     "salud", ("ortopedia ortopedico ayudas tecnicas tienda de ortopedia andador bastones muletas silla de ruedas cama articulada alquiler protesis ortesis plantillas a medida rehabilitacion prestacion ortoprotesica a domicilio",
                               "orthopaedics orthopaedic shop mobility aids walker crutches wheelchair adjustable bed rental prosthetics orthoses custom insoles rehabilitation home delivery"),
                     look, cabecera(nav, cta_cab), footer, [inicio, prod, serv, cont], vertical="ortopedia")
