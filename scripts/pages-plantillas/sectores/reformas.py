# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
from lib import *

KEY = "reformas"

# Naranja de obra sobre gris cálido. Sin `tono="oscuro"`: primary/inverse no llega a 3.
OBRA = dict(
    colores=dict(background="#f6f5f0", surface="#eae8df", text="#1d2220", muted="#4f5653", primary="#a83f08",
                 on_primary="#ffffff", border="#d9d6c9", inverse="#202726", on_inverse="#f6f5f0"),
    fuentes=("montserrat", "ibm-plex-sans"), peso=700, radio=(2, 4, 8), ancho=1140)

NOTA_PLAZOS = ("Plazos de ejemplo: ajústalos a los tuyos. El plazo exacto va en el plan de obra de cada trabajo.",
               "Example times: adjust them to yours. The exact time goes in each job's work plan.")


def pasos_lista(titulo, items, intro=None, eyebrow=None, tono="base", anchor=None):
    """Pasos en una sola columna, para procesos largos (cinco pasos no caben bien en una fila)."""
    li = []
    for i, (h, t) in enumerate(items, 1):
        li.append(marco([texto("p", (str(i), str(i)), preset="h2", color="$primary", sz=tam("hug", "hug", min_width=40)),
                         marco([h3(h), p(t, color="$muted")], gap=6, sz=tam("fill", "hug", min_width=0))],
                        tag="li", dir="row", gap=20, pad=24, align="start",
                        style={"border": {"width": 1, "color": "$border", "style": "solid"}, "radius": "$md"}))
    return seccion("features", [titular(titulo, intro, eyebrow), marco(li, gap=14, tag="ol")],
                   tono=tono, anchor=anchor)


def build():
    nueva(KEY)
    look = OBRA
    nav = [("inicio", ("Inicio", "Home")), ("reformas", ("Reformas", "Renovations")),
           ("proceso", ("Cómo trabajamos", "How we work")), ("presupuesto", ("Presupuesto", "Quote"))]
    horas = [(("Oficina, de lunes a viernes", "Office, Monday to Friday"), ("9:00 a 18:00", "9:00 am to 6:00 pm")),
             (("Visitas a domicilio", "Home visits"), ("Con cita, también sábados por la mañana", "By appointment, Saturday mornings too")),
             (("Domingos y festivos", "Sundays and bank holidays"), ("Cerrado", "Closed"))]
    cta_cab = marco([llamar(t=("Llamar", "Call")), whatsapp("secondary", ("WhatsApp", "WhatsApp"))], dir="row", gap=8, wrap=True)
    pedir = lambda v="primary": boton(("Pedir presupuesto", "Ask for a quote"), a_pagina("presupuesto", "formulario"), v)

    inicio = pagina("inicio", ("", ""), ("Reformas integrales", "Full renovations"),
                    ("Reformas de baños, cocinas y viviendas completas con presupuesto cerrado por escrito, plan de obra y garantía. Pide presupuesto sin compromiso.",
                     "Bathroom, kitchen and whole-home renovations with a fixed written quote, a work plan and a guarantee. Ask for a quote with no obligation."), [
        hero(("Reformas de baños, cocinas y viviendas completas", "Bathroom, kitchen and whole-home renovations"),
             ("Visitamos tu casa, te damos un presupuesto cerrado por escrito y una fecha de entrega. Si algo cambia durante la obra, te lo decimos antes de hacerlo.",
              "We visit your home and give you a fixed written quote and a handover date. If anything changes during the works, we tell you before we do it."),
             [pedir(), whatsapp()], eyebrow=("Empresa de reformas", "Renovation company"),
             imagen_=("photo", ("Cocina recién reformada con encimera y armarios nuevos", "Newly renovated kitchen with a new worktop and units"))),
        bloque_checks(("Lo que recibes por escrito antes de empezar", "What you get in writing before we start"), [
            ("Presupuesto cerrado, con cada partida y su precio.", "A fixed quote, with every item and its price."),
            ("Plan de obra con fecha de inicio y de entrega.", "A work plan with a start date and a handover date."),
            ("Lista de materiales y acabados que has elegido.", "A list of the materials and finishes you chose."),
            ("Forma de pago por fases, sin sorpresas al final.", "Payment in stages, with no surprises at the end.")],
            eyebrow=("Sin letra pequeña", "No small print"), tono="relleno", anchor="por-escrito"),
        bloque_valores(("Qué reformamos", "What we renovate"), [
            (("Baños", "Bathrooms"), ("Cambio de bañera por plato de ducha, sanitarios, alicatado, fontanería y electricidad.",
                                       "Swapping a bath for a shower tray, toilets, tiling, plumbing and wiring.")),
            (("Cocinas", "Kitchens"), ("Desmontaje, instalación nueva, muebles, encimera y electrodomésticos, con la obra de albañilería incluida.",
                                        "Stripping out, new installation, units, worktop and appliances, builders' work included.")),
            (("Viviendas completas", "Whole homes"), ("Tabiques, suelos, instalaciones, carpintería y pintura, con un solo plan para todos los gremios.",
                                                      "Partitions, floors, services, joinery and paint, with one plan for every trade.")),
            (("Una o dos estancias", "One or two rooms"), ("Suelo, pintura, puertas o una instalación nueva en la parte de la casa que lo necesite.",
                                                           "Floors, paint, doors or a new installation in the part of the home that needs it.")),
        ], cols=4, eyebrow=("Tipos de reforma", "Types of renovation"), anchor="tipos"),
        bloque_opiniones(("Lo que cuentan quienes ya han reformado con nosotros", "What people who renovated with us say"),
                         [("el baño", "the bathroom"), ("la cocina", "the kitchen"), ("la limpieza de la obra", "keeping the site clean")],
                         tono="relleno"),
        bloque_cta(("¿Qué quieres reformar?", "What would you like to renovate?"),
                   ("Cuéntanos el tipo de reforma, los metros y para cuándo la quieres. Te llamamos para ver el día de la visita.",
                    "Tell us the type of renovation, the size and when you want it. We will call you to arrange the visit."),
                   [pedir(), llamar("ghost")], tono="base"),
    ])

    ref = pagina("reformas", ("reformas", "renovations"), ("Reformas", "Renovations"),
                 ("Baños, cocinas y viviendas completas: qué incluye cada reforma, fotos de trabajos y zona donde trabajamos.",
                  "Bathrooms, kitchens and whole homes: what each renovation includes, photos of our work and the area we cover."), [
        hero(("Qué incluye cada reforma", "What each renovation includes"),
             ("Trabajamos con nuestros propios oficiales y con gremios de confianza. Tú hablas con una sola persona.",
              "We work with our own tradespeople and trusted subcontractors. You talk to one person."),
             [pedir()], variante="texto"),
        bloque_texto(("Reforma de baño", "Bathroom renovation"),
                     [("Quitamos lo viejo, revisamos las tuberías y el desagüe, ponemos el alicatado, los sanitarios y la grifería, y dejamos el baño listo para usar.",
                       "We take out the old fittings, check the pipes and the drain, lay the tiles, fit the sanitaryware and taps, and leave the bathroom ready to use."),
                      ("Si hay que abrir pared para cambiar una tubería, lo ves en el presupuesto antes de empezar.",
                       "If a wall has to be opened to replace a pipe, you see it in the quote before we start.")],
                     eyebrow=("Baños", "Bathrooms"),
                     imagen_=("photo", ("Baño reformado con plato de ducha y mampara", "Renovated bathroom with a shower tray and screen")),
                     anchor="banos"),
        bloque_texto(("Reforma de cocina", "Kitchen renovation"),
                     [("Distribución nueva o la misma, con la instalación eléctrica y de agua al día. Los muebles y la encimera se miden en tu cocina, no en un plano.",
                       "A new layout or the same one, with the electrics and water brought up to date. Units and worktop are measured in your kitchen, not from a plan."),
                      ("Coordinamos el montaje de electrodomésticos para que la cocina no se quede sin servicio más días de los necesarios.",
                       "We schedule the appliance fitting so your kitchen is out of use for as few days as possible.")],
                     eyebrow=("Cocinas", "Kitchens"), invertir=True, tono="relleno",
                     imagen_=("photo", ("Cocina con isla y muebles altos recién instalados", "Kitchen with an island and tall units newly installed")),
                     anchor="cocinas"),
        bloque_texto(("Reforma de vivienda completa", "Whole-home renovation"),
                     [("Cuando se reforma toda la casa, el orden importa: primero tabiques e instalaciones, luego suelos y carpintería, y al final la pintura.",
                       "When a whole home is renovated the order matters: partitions and services first, then floors and joinery, and paint last."),
                      ("Te damos un plan de obra por semanas, con los días en que tienes que decidir azulejos, puertas o iluminación.",
                       "We give you a week-by-week work plan, with the days on which you have to choose tiles, doors or lighting.")],
                     eyebrow=("Viviendas", "Homes"),
                     imagen_=("landscape", ("Salón de una vivienda recién reformada", "Living room of a newly renovated home")),
                     anchor="viviendas"),
        bloque_galeria(("Trabajos terminados", "Finished jobs"), [
            ("Baño reformado con alicatado de gran formato", "Renovated bathroom with large-format tiling"),
            ("Cocina reformada con encimera de piedra", "Renovated kitchen with a stone worktop"),
            ("Pasillo con suelo nuevo y puertas lacadas", "Hallway with a new floor and lacquered doors"),
            ("Dormitorio reformado con armario empotrado", "Renovated bedroom with a built-in wardrobe"),
            ("Ducha a ras de suelo con mampara fija", "Walk-in shower with a fixed screen"),
            ("Salón con suelo de parquet recién instalado", "Living room with a newly laid parquet floor")],
            tipo="photo", tono="relleno"),
        bloque_texto(("Dónde trabajamos", "Where we work"),
                     [("Reformamos en Tu ciudad y en los municipios de alrededor. Si tu vivienda está más lejos, escríbenos y vemos si podemos llevar la obra.",
                       "We renovate in Your city and the surrounding towns. If your home is further away, message us and we will see whether we can take the job on.")],
                     eyebrow=("Zona de trabajo", "Area we cover"), anchor="zonas"),
    ])

    proceso = pagina("proceso", ("proceso", "process"), ("Cómo trabajamos", "How we work"),
                     ("Los cinco pasos de una reforma, plazos orientativos, garantía y seguro, y respuestas a las preguntas más habituales.",
                      "The five steps of a renovation, approximate times, guarantee and insurance, and answers to the most common questions."), [
        hero(("Cómo trabajamos", "How we work"),
             ("Una reforma da menos guerra cuando sabes qué va a pasar cada semana. Estos son los cinco pasos que seguimos siempre.",
              "A renovation causes less trouble when you know what will happen each week. These are the five steps we always follow."),
             [pedir(), llamar("secondary")], variante="texto"),
        pasos_lista(("De la visita a la entrega", "From the visit to the handover"), [
            (("Visita", "Visit"),
             ("Vamos a tu casa, medimos y vemos el estado de lo que hay. Sin compromiso.",
              "We come to your home, measure up and check the state of what is there. No obligation.")),
            (("Presupuesto cerrado por escrito", "Fixed written quote"),
             ("Cada partida con su precio, los materiales y el total. Si no cambias nada, no pagas más.",
              "Every item with its price, the materials and the total. If you change nothing, you pay no more.")),
            (("Plan de obra", "Work plan"),
             ("Fecha de inicio y de entrega, qué gremio entra cada semana y cuándo debes decidir cada acabado.",
              "Start and handover dates, which trade comes in each week and when you must choose each finish.")),
            (("Ejecución", "The works"),
             ("Una persona de la empresa coordina la obra y te cuenta cómo va. La casa queda recogida al acabar cada jornada.",
              "One person from the company runs the job and tells you how it is going. The home is tidied at the end of each day.")),
            (("Entrega", "Handover"),
             ("Repasamos la obra contigo, anotamos los retoques y los cerramos. Te damos la garantía por escrito.",
              "We go over the work with you, note any touch-ups and finish them. We give you the guarantee in writing.")),
        ], eyebrow=("El proceso", "The process"), anchor="pasos"),
        bloque_precios(("Plazos orientativos", "Approximate times"), ("Plazos orientativos por tipo de reforma", "Approximate times by type of renovation"),
                       [("Reforma", "Renovation"), ("Plazo habitual", "Usual time")],
                       [(("Baño completo", "Complete bathroom"), ("2 a 3 semanas", "2 to 3 weeks")),
                        (("Cocina completa", "Complete kitchen"), ("3 a 4 semanas", "3 to 4 weeks")),
                        (("Vivienda completa", "Whole home"), ("2 a 4 meses", "2 to 4 months"))],
                       intro=("Cada obra es distinta. Estos son los plazos con los que solemos trabajar cuando no hay sorpresas en las paredes ni en las tuberías.",
                              "Every job is different. These are the times we usually work to when there are no surprises in the walls or the pipes."),
                       nota=NOTA_PLAZOS, anchor="plazos", tono="relleno", kind="features"),
        bloque_texto(("Garantía y seguro", "Guarantee and insurance"),
                     [("Cada obra lleva su garantía por escrito, en el presupuesto y en la factura. Garantía de mano de obra y acabados: 00 meses (dato de ejemplo: cámbialo por el tuyo).",
                       "Every job carries its guarantee in writing, in the quote and on the invoice. Guarantee on labour and finishes: 00 months (example figure: replace it with yours)."),
                      ("Seguro de responsabilidad civil de la empresa: Nombre de la aseguradora, póliza 00-0000 (datos de ejemplo: cámbialos por los tuyos). Si lo pides, te damos copia antes de empezar.",
                       "The company's public liability insurance: Insurer name, policy 00-0000 (example details: replace them with yours). If you ask, we give you a copy before we start.")],
                     eyebrow=("Garantías", "Guarantees"), anchor="garantia"),
        bloque_faq(("Preguntas frecuentes", "Frequently asked questions"), [
            (("¿Cuánto cuesta una reforma?", "How much does a renovation cost?"),
             ("Depende de los metros, de los materiales y de lo que haya detrás de las paredes. Por eso hacemos una visita y luego te damos el precio cerrado.",
              "It depends on the size, the materials and what is behind the walls. That is why we visit first and then give you a fixed price.")),
            (("¿Tengo que irme de casa durante la obra?", "Do I have to move out during the works?"),
             ("En un baño o una cocina, no: te decimos qué días estarás sin agua o sin cocina. En una vivienda completa depende de la fase.",
              "For a bathroom or kitchen, no: we tell you which days you will be without water or a kitchen. For a whole home it depends on the phase.")),
            (("¿Quién pide la licencia?", "Who applies for the permit?"),
             ("Si la obra necesita licencia o comunicación al ayuntamiento, te decimos cuál y nos encargamos de tramitarla.",
              "If the job needs a permit or a notice to the council, we tell you which one and we deal with it.")),
            (("¿Qué pasa si aparece algo inesperado?", "What if something unexpected turns up?"),
             ("Paramos, te lo enseñamos y te damos el precio de arreglarlo. No se hace nada que no hayas aprobado por escrito.",
              "We stop, show you and give you the price to fix it. Nothing is done that you have not approved in writing.")),
            (("¿Cómo se paga?", "How do I pay?"),
             ("Por fases, como se acuerda en el presupuesto: una parte al empezar y el resto según avanza la obra.",
              "In stages, as agreed in the quote: a part at the start and the rest as the work moves on.")),
        ], tono="relleno"),
    ])

    pres = pagina("presupuesto", ("presupuesto", "quote"), ("Presupuesto sin compromiso", "Quote with no obligation"),
                  ("Pide presupuesto sin compromiso: tipo de reforma, metros y plazo. Horario de oficina y mapa para llegar.",
                   "Ask for a quote with no obligation: type of renovation, size and timing. Office hours and a map to find us."), [
        hero(("Pide tu presupuesto sin compromiso", "Ask for your quote with no obligation"),
             ("Dinos qué quieres reformar y para cuándo. Te llamamos para fijar el día de la visita.",
              "Tell us what you want to renovate and when. We will call you to fix the day of the visit."),
             [llamar(), whatsapp()], variante="texto"),
        bloque_contacto(("Cuéntanos tu reforma", "Tell us about your renovation"),
                        ("Con estos datos preparamos la visita. Si tienes fotos o un plano, mándanoslos por WhatsApp.",
                         "With this we prepare the visit. If you have photos or a plan, send them to us on WhatsApp."),
                        [campo("nombre", "text", ("Nombre", "Name"), True, auto="name", maxlen=120),
                         campo("telefono", "tel", ("Teléfono", "Phone"), True, auto="tel", maxlen=30),
                         campo("correo", "email", ("Correo electrónico (opcional)", "Email (optional)"), False, auto="email", maxlen=200),
                         campo("tipo", "select", ("Tipo de reforma", "Type of renovation"), True,
                               opciones=[("Baño", "Bathroom"), ("Cocina", "Kitchen"), ("Vivienda completa", "Whole home"),
                                         ("Una o dos estancias", "One or two rooms"), ("Otra", "Other")]),
                        campo("metros", "number", ("Metros cuadrados aproximados", "Approximate size in square metres")),
                        campo("plazo", "select", ("¿Para cuándo la quieres?", "When do you want it?"), False,
                              opciones=[("Lo antes posible", "As soon as possible"), ("En 1 o 2 meses", "In 1 or 2 months"),
                                        ("En 3 a 6 meses", "In 3 to 6 months"), ("Sin prisa", "No rush")]),
                        campo("mensaje", "textarea", ("Cuéntanos más (opcional)", "Tell us more (optional)"), False, maxlen=1500)],
                        ("Pedir presupuesto", "Ask for a quote"), horas, anchor="formulario",
                        exito=("Gracias. Te llamamos en un día laborable para acordar la visita.",
                               "Thank you. We will call you on a working day to arrange the visit.")),
        bloque_mapa(("Cómo llegar a la oficina", "How to get to the office"),
                    ("Si prefieres hablarlo en persona, pásate por la oficina en horario de mañana.",
                     "If you prefer to talk in person, drop by the office in the morning."), tono="relleno"),
    ])

    return plantilla(KEY, ("Reformas integrales", "Full renovations"),
                     ("Reformas de baños, cocinas y viviendas con proceso en cinco pasos, tipos de reforma, galería, garantía y seguro, plazos y formulario de presupuesto con mapa.",
                      "Bathroom, kitchen and home renovations with a five-step process, types of work, gallery, guarantee and insurance, timings and a quote form with a map."),
                     "oficios", ("reformas reforma integral obras albanil constructor reformista baño cocina vivienda presupuesto de reforma",
                                 "renovation renovations builder building works bathroom kitchen home refurbishment quote"),
                     look, cabecera(nav, cta_cab),
                     pie(nav, ("Reformas de baños, cocinas y viviendas completas, con presupuesto cerrado por escrito.",
                               "Bathroom, kitchen and whole-home renovations with a fixed written quote."),
                         ("Lunes a viernes 9:00 a 18:00. Visitas con cita.", "Monday to Friday 9 am to 6 pm. Visits by appointment.")),
                     [inicio, ref, proceso, pres])
