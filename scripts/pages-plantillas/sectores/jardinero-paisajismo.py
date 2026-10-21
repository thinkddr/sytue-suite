# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
from lib import *
from looks import LOOKS

KEY = "jardinero-paisajismo"

# Paleta propia (verde hoja sobre blanco con tinte de hierba); se comprueba AA al construir.
HOJA = dict(background="#f8fbf1", surface="#e9f1d9", text="#1c2616", muted="#4f5d44", primary="#3b7a12",
            on_primary="#ffffff", border="#d4e2bb", inverse="#1e2d14", on_inverse="#f3f8e8")


def build():
    malos = comprobar_paleta("hoja", HOJA)
    assert not malos, malos
    nueva(KEY)
    look = dict(colores=HOJA, fuentes=("source-serif-4", "work-sans"), radio=(4, 10, 16), h1=(50, 40, 32))
    nav = [("inicio", ("Inicio", "Home")), ("mantenimiento", ("Mantenimiento y cuotas", "Maintenance and fees")),
           ("jardines", ("Diseño y huertos", "Design and vegetable plots")), ("contacto", ("Contacto", "Contact"))]
    horas = [(("Lunes a viernes", "Monday to Friday"), ("8:00 a 17:00", "8:00 am to 5:00 pm")),
             (("Sábado y domingo", "Saturday and Sunday"), ("Cerrado", "Closed"))]
    cta_cab = marco([llamar(t=("Llamar", "Call")), whatsapp("secondary", t=("WhatsApp", "WhatsApp"))],
                    dir="row", gap=8, wrap=True)

    def visita():
        return boton(("Pedir visita de presupuesto", "Book a quote visit"), a_pagina("contacto", "contacto"))


    inicio = pagina("inicio", ("", ""), ("Jardinería y paisajismo", "Gardening and landscaping"),
                    ("Mantenimiento de jardines por cuotas, poda, césped, riego automático, diseño de jardines y huertos urbanos. Pide una visita de presupuesto.",
                     "Garden maintenance by monthly fee, pruning, lawns, automatic irrigation, garden design and urban vegetable plots. Ask for a quote visit."), [
        hero(("Un jardín cuidado todo el año", "A well-kept garden all year round"),
             ("Mantenemos tu jardín por una cuota mensual, podamos, cuidamos el césped, instalamos el riego y diseñamos jardines y huertos. Primero vamos a verlo, luego te damos el precio.",
              "We look after your garden for a monthly fee, prune, care for lawns, fit irrigation and design gardens and vegetable plots. First we come to see it, then we give you the price."),
             [visita(), whatsapp()],
             eyebrow=("Jardinería y paisajismo", "Gardening and landscaping"),
             imagen_=("landscape", ("Jardín con césped, arbustos y un camino de piedra", "Garden with lawn, shrubs and a stone path"))),
        bloque_valores(("Lo que hacemos", "What we do"), [
            (("Mantenimiento por cuotas", "Maintenance by monthly fee"),
             ("Visitas fijas cada semana, cada quince días o cada mes, con una cuota que sabes de antemano.",
              "Fixed visits every week, fortnight or month, for a fee you know in advance.")),
            (("Poda", "Pruning"),
             ("Setos, arbustos, rosales y árboles. Cada especie a su tiempo, y nos llevamos los restos.",
              "Hedges, shrubs, roses and trees. Each species in its season, and we take the waste away.")),
            (("Césped", "Lawns"),
             ("Siega, perfilado, abonado, aireado y resiembra. También césped nuevo, de siembra o en rollo.",
              "Mowing, edging, feeding, aerating and reseeding. Also new lawns, from seed or turf.")),
            (("Riego automático", "Automatic irrigation"),
             ("Instalación, revisión y reparación de aspersores, difusores y goteo, con programador.",
              "Installation, checks and repair of sprinklers, spray heads and drip lines, with a controller.")),
            (("Diseño de jardines", "Garden design"),
             ("Un plano y una lista de plantas pensados para tu espacio, tu sol y lo que quieres cuidar.",
              "A plan and a plant list made for your space, your sun and how much care you want to give it.")),
            (("Huertos urbanos", "Urban vegetable plots"),
             ("Bancales, sustrato, riego por goteo y un calendario de siembra para terraza, patio o jardín.",
              "Raised beds, soil, drip irrigation and a planting calendar for a terrace, patio or garden.")),
        ], eyebrow=("Servicios", "Services"), estilo="linea", tono="relleno"),
        bloque_pasos(("Cómo te damos el presupuesto", "How we give you a quote"), [
            (("Vamos a ver tu jardín", "We come to see your garden"),
             ("Medimos, miramos el sol, el suelo y las plantas, y escuchamos qué quieres.",
              "We measure, look at the sun, the soil and the plants, and listen to what you want.")),
            (("Te mandamos la propuesta", "We send you a proposal"),
             ("Por escrito, con lo que incluye, la frecuencia de visitas y el precio. Sin letra pequeña.",
              "In writing, with what is included, how often we visit and the price. No small print.")),
            (("Empezamos cuando tú digas", "We start when you say"),
             ("Si te encaja, fijamos el día de la primera visita. Si no, no pasa nada.",
              "If it suits you, we set the day of the first visit. If not, no problem.")),
        ], intro=("La visita es sin compromiso.", "The visit carries no obligation."), tono="base", anchor="presupuesto"),
        bloque_opiniones(("Lo que dicen nuestros clientes", "What our customers say"),
                         [("el jardín", "the garden"), ("la puntualidad", "the punctuality"), ("la limpieza al terminar", "the tidying up afterwards")]),
        bloque_cta(("¿Vamos a ver tu jardín?", "Shall we come to see your garden?"),
                   ("Cuéntanos dónde estás y qué necesitas y concertamos la visita.",
                    "Tell us where you are and what you need and we will arrange the visit.")),
    ])

    mant = pagina("mantenimiento", ("mantenimiento", "maintenance"), ("Mantenimiento y cuotas", "Maintenance and fees"),
                  ("Cuotas mensuales de mantenimiento de jardines de ejemplo, plan de trabajo por estaciones, poda, césped y riego automático.",
                   "Example monthly garden maintenance fees, a season-by-season work plan, pruning, lawns and automatic irrigation."), [
        hero(("Mantenimiento de jardines", "Garden maintenance"),
             ("Una cuota fija al mes y un jardín que no se te descontrola. Estas son las cuotas habituales y lo que hacemos en cada estación.",
              "A fixed monthly fee and a garden that never gets out of hand. These are the usual fees and what we do in each season."),
             [visita()], variante="texto"),
        bloque_precios(("Cuotas de mantenimiento", "Maintenance fees"), ("Cuotas mensuales de mantenimiento de jardín", "Monthly garden maintenance fees"),
                       [("Tamaño del jardín", "Garden size"), ("Visitas", "Visits"), ("Cuota al mes", "Fee per month")],
                       [(("Hasta 100 m²", "Up to 100 m²"), ("Cada 15 días", "Every fortnight"), desde(80)),
                        (("De 100 a 300 m²", "100 to 300 m²"), ("Cada 15 días", "Every fortnight"), desde(140)),
                        (("De 100 a 300 m²", "100 to 300 m²"), ("Cada semana", "Every week"), desde(260)),
                        (("De 300 a 600 m²", "300 to 600 m²"), ("Cada semana", "Every week"), desde(380)),
                        (("Más de 600 m² o comunidades", "Over 600 m² or housing blocks"), ("A medida", "Tailored"), ("Presupuesto con visita", "Quote after a visit"))],
                       intro=("Incluyen siega, perfilado de bordes, riego revisado, escarda y recogida de restos en cada visita.",
                              "They include mowing, edging, an irrigation check, weeding and clearing of waste at every visit."),
                       nota=("Precios de ejemplo: cámbialos por los tuyos. Incluyen IVA. La poda de árboles grandes y los materiales se presupuestan aparte.",
                             "Example prices: replace them with your own. VAT included. Pruning of large trees and materials are quoted separately."),
                       anchor="cuotas", tono="relleno"),
        bloque_valores(("Qué hacemos en cada estación", "What we do in each season"), [
            (("Primavera", "Spring"),
             ("Abonado, siembra y resiembra del césped, plantación y puesta en marcha del riego.",
              "Feeding, sowing and reseeding the lawn, planting, and switching the irrigation on.")),
            (("Verano", "Summer"),
             ("Riego ajustado a la ola de calor, siega más alta, escarda y vigilancia de plagas.",
              "Irrigation adjusted to heat waves, higher mowing, weeding and watching for pests.")),
            (("Otoño", "Autumn"),
             ("Aireado y resiembra, recogida de hoja, plantación de arbustos y bulbos y abonado de fondo.",
              "Aerating and reseeding, clearing leaves, planting shrubs and bulbs and a base feed.")),
            (("Invierno", "Winter"),
             ("Poda de árboles de hoja caduca y rosales, protección contra heladas y revisión del riego.",
              "Pruning deciduous trees and roses, frost protection and an irrigation check.")),
        ], intro=("Las fechas cambian con el clima de tu zona: ajustamos cada tarea a cómo viene el año.",
                  "Dates shift with your local climate: we adjust each job to how the year is going."),
            eyebrow=("Plan por estaciones", "Season-by-season plan"), cols=4, estilo="borde", anchor="estaciones"),
        bloque_precios(("Trabajos sueltos", "One-off jobs"), ("Precios de trabajos sueltos de jardinería", "One-off gardening prices"),
                       [("Trabajo", "Job"), ("Precio", "Price")],
                       [(("Hora de jardinero", "Gardener, per hour"), desde(22)),
                        (("Poda de seto (por metro lineal)", "Hedge trimming (per linear metre)"), desde(4)),
                        (("Siega y perfilado de césped (por visita)", "Mowing and edging a lawn (per visit)"), desde(35)),
                        (("Revisión de riego automático", "Irrigation system check"), desde(45)),
                        (("Poda de árbol", "Tree pruning"), ("Presupuesto con visita", "Quote after a visit"))],
                       nota=("Precios de ejemplo: cámbialos por los tuyos. Incluyen IVA.",
                             "Example prices: replace them with your own. VAT included."),
                       anchor="sueltos", tono="base"),
        bloque_texto(("Riego automático", "Automatic irrigation"), [
            ("Instalamos aspersores, difusores y goteo, y los dejamos con un programador ajustado a tus plantas. Un riego programado evita regar de más.",
             "We fit sprinklers, spray heads and drip lines, and leave them on a controller set for your plants. A programmed system avoids over-watering."),
            ("Si ya tienes riego y falla, lo revisamos: boquillas atascadas, fugas, válvulas y programador.",
             "If you already have irrigation and it is failing, we check it: blocked nozzles, leaks, valves and controller.")],
            eyebrow=("Riego", "Irrigation"), tono="relleno", anchor="riego",
            imagen_=("photo", ("Aspersor regando el césped de un jardín", "Sprinkler watering a garden lawn")), invertir=True),
        bloque_faq(("Preguntas frecuentes", "Frequently asked questions"), [
            (("¿Cada cuánto vais?", "How often do you come?"),
             ("Lo acordamos contigo: cada semana, cada quince días o cada mes. En verano y en primavera, el césped y los setos piden más visitas.",
              "We agree it with you: every week, fortnight or month. In spring and summer, lawns and hedges need more visits.")),
            (("¿Tengo que estar en casa?", "Do I need to be at home?"),
             ("No. Si nos dejas acceso al jardín, trabajamos sin que estés y te avisamos al terminar.",
              "No. If you give us access to the garden we work without you there and let you know when we finish.")),
            (("¿Os lleváis los restos de poda?", "Do you take away the pruning waste?"),
             ("Sí, en cada visita dejamos el jardín limpio y nos llevamos lo que sale de podar y escardar.",
              "Yes, at every visit we leave the garden tidy and take away what comes from pruning and weeding.")),
            (("¿Puedo dejar el mantenimiento cuando quiera?", "Can I stop the maintenance whenever I want?"),
             ("Sí. Avísanos con un mes de margen y lo dejamos. No hay permanencia.",
              "Yes. Give us a month's notice and we stop. There is no minimum term.")),
        ], tono="relleno"),
    ])

    jard = pagina("jardines", ("jardines", "gardens"), ("Diseño de jardines y huertos", "Garden design and vegetable plots"),
                  ("Diseño de jardines, huertos urbanos y galería de jardines hechos. Pide una visita de presupuesto.",
                   "Garden design, urban vegetable plots and a gallery of finished gardens. Ask for a quote visit."), [
        hero(("Diseño de jardines y huertos", "Garden design and vegetable plots"),
             ("Un jardín nuevo, uno que renovar o un huerto en la terraza. Te lo dibujamos antes de mover una piedra.",
              "A new garden, one to renovate or a vegetable plot on the terrace. We draw it up before moving a single stone."),
             [visita(), whatsapp()], variante="centro",
             eyebrow=("Paisajismo", "Landscaping")),
        bloque_texto(("Diseño de jardines", "Garden design"), [
            ("Partimos de tu espacio: cuánto sol da, cómo es el suelo, cuánto tiempo quieres dedicarle. Con eso te entregamos un plano y una lista de plantas, y tú eliges.",
             "We start from your space: how much sun it gets, what the soil is like, how much time you want to spend on it. With that we give you a plan and a plant list, and you choose."),
            ("Si quieres, también lo hacemos: preparamos el terreno, plantamos, ponemos el riego y los caminos.",
             "If you wish, we build it too: we prepare the ground, plant, and fit the irrigation and paths.")],
            eyebrow=("Diseño", "Design"), anchor="diseno",
            imagen_=("landscape", ("Plano de un jardín con zonas de césped, plantas y paseo", "Garden plan showing lawn, planting and path areas"))),
        bloque_checks(("Huertos urbanos", "Urban vegetable plots"), [
            ("Bancales de madera o de obra a la medida de tu terraza o patio.", "Raised beds in wood or masonry sized for your terrace or patio."),
            ("Sustrato y abonado adecuados a lo que vas a cultivar.", "Soil and feeding suited to what you will grow."),
            ("Riego por goteo con programador, para que no dependas de acordarte.", "Drip irrigation on a controller, so you do not depend on remembering."),
            ("Un calendario de siembra y cosecha según tu zona.", "A sowing and harvest calendar for your area."),
            ("Te enseñamos a cuidarlo para que empieces con buen pie.", "We show you how to look after it so you start off well."),
        ], intro=("Para quien quiere cultivar sus propias verduras sin complicarse.", "For anyone who wants to grow their own vegetables without the fuss."),
            eyebrow=("Huerto en casa", "Growing at home"), tono="relleno", anchor="huertos"),
        bloque_galeria(("Jardines que hemos hecho", "Gardens we have made"), [
            ("Jardín con césped y borde de arbustos hecho por la empresa", "Garden with a lawn and shrub border made by the company"),
            ("Patio con macetas y riego por goteo instalado por la empresa", "Patio with pots and drip irrigation fitted by the company"),
            ("Huerto urbano con bancales de madera", "Urban vegetable plot with wooden raised beds"),
            ("Seto recién podado y perfilado", "Freshly trimmed and shaped hedge"),
            ("Camino de piedra entre plantas aromáticas", "Stone path between aromatic plants"),
            ("Jardín de comunidad con zona de césped y árboles", "Communal garden with a lawn and trees")],
            intro=("Algunos de nuestros trabajos. Cambia estas fotos por las tuyas.", "Some of our work. Replace these photos with your own."),
            tipo="landscape", tono="base", ratio=1.3),
        bloque_cta(("¿Qué jardín tienes en mente?", "What garden have you got in mind?"),
                   ("Pide una visita y te decimos qué se puede hacer y cuánto costaría.",
                    "Ask for a visit and we will tell you what can be done and what it would cost."),
                   [visita(), whatsapp("ghost")], tono="relleno"),
    ])

    cont = pagina("contacto", ("contacto", "contact"), ("Contacto y visita", "Contact and visit"),
                  ("Pide una visita de presupuesto por teléfono, WhatsApp o formulario. Horario y cómo llegar.",
                   "Ask for a quote visit by phone, WhatsApp or form. Opening hours and how to find us."), [
        hero(("Pide tu visita de presupuesto", "Ask for your quote visit"),
             ("Cuéntanos dónde está el jardín y qué necesitas. Te llamamos para acordar el día de la visita.",
              "Tell us where the garden is and what you need. We will call you to agree the day of the visit."),
             [llamar(), whatsapp()], variante="texto"),
        bloque_contacto(("Escríbenos", "Write to us"),
                        ("Con unas líneas nos basta. Si tienes fotos del jardín, mándalas por WhatsApp.",
                         "A few lines are enough. If you have photos of the garden, send them on WhatsApp."),
                        campos_cita(servicios=[("Mantenimiento por cuotas", "Maintenance by monthly fee"), ("Poda", "Pruning"),
                                               ("Césped", "Lawn"), ("Riego automático", "Automatic irrigation"),
                                               ("Diseño de jardín", "Garden design"), ("Huerto urbano", "Urban vegetable plot")],
                                    extra=[campo("poblacion", "text", ("Población", "Town"), True, maxlen=120),
                                           campo("superficie", "select", ("Tamaño aproximado del jardín", "Approximate garden size"), False,
                                                 opciones=[("Hasta 100 m²", "Up to 100 m²"), ("De 100 a 300 m²", "100 to 300 m²"),
                                                           ("Más de 300 m²", "Over 300 m²"), ("No lo sé", "I do not know")])]),
                        ("Pedir visita", "Request a visit"), horas,
                        exito=("Gracias. Te llamamos para acordar el día de la visita.",
                               "Thank you. We will call you to agree the day of the visit.")),
        bloque_mapa(("Dónde está nuestra oficina", "Where our office is"),
                    ("Vamos a tu jardín, no hace falta que vengas. Esta es la oficina.", "We come to your garden, so there is no need to visit. This is the office."),
                    tono="relleno"),
    ])

    return plantilla(KEY, ("Jardinería y paisajismo", "Gardening and landscaping"),
                     ("Cuotas de mantenimiento de ejemplo, plan por estaciones, diseño de jardines y huertos, galería y visita de presupuesto por WhatsApp o formulario.",
                      "Example maintenance fees, a season-by-season plan, garden and vegetable plot design, a gallery and quote visits by WhatsApp or form."),
                     "oficios", ("jardinero jardineria paisajismo jardines mantenimiento de jardines poda cesped riego automatico huerto urbano huertos diseno de jardines setos",
                                 "gardener gardening landscaping landscape gardener garden maintenance pruning lawn irrigation vegetable plot garden design hedges"),
                     look, cabecera(nav, cta_cab),
                     pie(nav, ("Jardinería y paisajismo. Mantenimiento por cuotas, poda, césped, riego, diseño y huertos.",
                               "Gardening and landscaping. Maintenance by monthly fee, pruning, lawns, irrigation, design and vegetable plots."),
                         ("Lunes a viernes de 8:00 a 17:00.", "Monday to Friday 8 am to 5 pm.")),
                     [inicio, mant, jard, cont])
