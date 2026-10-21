# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
from lib import *

KEY = "albanil-construccion"

# Paleta propia: gris cemento oscuro con amarillo de obra. Fondo oscuro, así que `tono="oscuro"` sí vale.
CEMENTO = dict(
    colores=dict(background="#202224", surface="#2b2e31", text="#f2f1ec", muted="#bdbcb4", primary="#ffd23f",
                 on_primary="#202224", border="#42464a", inverse="#131415", on_inverse="#f2f1ec"),
    fuentes=("work-sans", "figtree"), peso=800, mayus="uppercase", ls=0.02, h1=(48, 38, 30), radio=(0, 3, 6), ancho=1160)


def build():
    nueva(KEY)
    look = CEMENTO
    nav = [("inicio", ("Inicio", "Home")), ("obras", ("Obras y licencias", "Works and permits")),
           ("trabajos", ("Trabajos", "Work")), ("contacto", ("Presupuesto", "Quote"))]
    horas = [(("Lunes a viernes", "Monday to Friday"), ("De 7:30 a 17:30", "7:30 am to 5:30 pm")),
             (("Sábados", "Saturdays"), ("Solo visitas con cita", "Visits by appointment only"))]
    cta_cab = marco([llamar(t=("Llamar", "Call")), whatsapp("secondary", t=("WhatsApp", "WhatsApp"))],
                    dir="row", gap=8, wrap=True)

    inicio = pagina("inicio", ("", ""), ("Albañilería y pequeñas obras", "Bricklaying and small building works"),
                    ("Albañilería y pequeñas obras: tabiques, alicatados, suelos y humedades, con presupuesto sin compromiso tras la visita y la obra explicada paso a paso.",
                     "Bricklaying and small building works: partitions, tiling, floors and damp repairs, with a no-obligation quote after the visit and the job explained step by step."), [
        hero(("Albañilería y pequeñas obras", "Bricklaying and small building works"),
             ("Levantamos tabiques, alicatamos baños y cocinas, arreglamos suelos y humedades. Vamos a ver la obra, te damos el presupuesto por escrito y te decimos cuántos días lleva.",
              "We build partitions, tile bathrooms and kitchens, fix floors and damp. We come to see the job, give you a written quote and tell you how many days it takes."),
             [llamar(), whatsapp()], variante="split",
             eyebrow=("Albañil", "Builder"),
             imagen_=("photo", ("Albañil alicatando la pared de un baño", "Builder tiling a bathroom wall"))),
        bloque_valores(("Qué hacemos", "What we do"), [
            (("Tabiques y falsos techos", "Partitions and false ceilings"),
             ("Dividimos una habitación, cerramos un hueco o bajamos un techo para pasar instalaciones.",
              "We divide a room, close an opening or lower a ceiling to run services.")),
            (("Alicatados y suelos", "Tiling and floors"),
             ("Azulejos en baños y cocinas, y suelo de gres o cerámico. Nivelamos antes de colocar.",
              "Tiles in bathrooms and kitchens, and porcelain or ceramic floors. We level before laying.")),
            (("Humedades", "Damp"),
             ("Buscamos de dónde viene: una fuga, una condensación o el agua que sube del suelo. Reparamos la causa, no solo la mancha.",
              "We find where it comes from: a leak, condensation or rising damp. We repair the cause, not just the stain.")),
            (("Reformas pequeñas", "Small renovations"),
             ("Un baño, una cocina, una habitación. Coordinamos fontanero y electricista si hace falta.",
              "A bathroom, a kitchen, a room. We coordinate the plumber and electrician if needed.")),
            (("Arreglos de obra", "Repairs"),
             ("Grietas, desconchones, escalones rotos, bordillos y arquetas.", "Cracks, flaking plaster, broken steps, kerbs and drain covers.")),
            (("Patios y exteriores", "Patios and exteriors"),
             ("Soleras, muretes y pavimentos de patios, terrazas y accesos.", "Slabs, low walls and paving for patios, terraces and access paths.")),
        ], eyebrow=("Trabajos", "Work"), tono="base", estilo="linea", cols=3),
        bloque_pasos(("Cómo se organiza una obra", "How a job is organised"), [
            (("Visita", "Visit"), ("Vamos a verla, medimos y miramos qué hay detrás de la pared o bajo el suelo.", "We come to see it, measure up and look at what is behind the wall or under the floor.")),
            (("Presupuesto", "Quote"), ("Por escrito, con partidas, materiales, días y lo que incluye.", "In writing, with line items, materials, days and what is included.")),
            (("Calendario", "Schedule"), ("Acordamos cuándo empezamos, quién entra a qué hora y qué día se acaba.", "We agree when we start, who comes in at what time and what day we finish.")),
            (("Obra", "Works"), ("Trabajamos protegiendo suelos y zonas comunes, y te contamos cada día cómo va.", "We work protecting floors and shared areas, and tell you each day how it is going.")),
            (("Entrega", "Handover"), ("Limpiamos, retiramos los escombros y repasamos contigo los remates.", "We clean up, remove the rubble and go over the finishing touches with you.")),
        ], tono="oscuro", eyebrow=("Proceso", "Process")),
        bloque_checks(("Presupuesto sin compromiso", "A quote with no obligation"), [
            ("La visita no cuesta nada ni te obliga a contratarnos.", "The visit costs nothing and does not oblige you to hire us."),
            ("El precio va por escrito, con lo que incluye y lo que no.", "The price is in writing, with what is and is not included."),
            ("Si en la obra aparece algo imprevisto, te lo decimos y lo acordamos antes de seguir.", "If something unexpected turns up on site, we tell you and agree it before going on."),
            ("Puedes pedir otros presupuestos: te damos el tiempo que necesites.", "You can ask for other quotes: you can take all the time you need."),
        ], eyebrow=("Cómo se pide", "How to ask"), cols=2, anchor="presupuesto", tono="relleno"),
        bloque_opiniones(("Lo que dicen nuestros clientes", "What our customers say"),
                         [("la limpieza de la obra", "how clean the job was kept"), ("el cumplimiento de los plazos", "keeping to the schedule"), ("el trato en casa", "the care taken in the home")],
                         tono="base"),
        bloque_cta(("¿Tienes una obra en mente?", "Have a job in mind?"),
                   ("Llámanos o mándanos fotos por WhatsApp y quedamos para verla.", "Call us or send photos on WhatsApp and we will arrange to see it."),
                   [llamar(), whatsapp("ghost")]),
    ])

    obras = pagina("obras", ("obras", "works"), ("Obras y licencias", "Works and permits"),
                   ("Qué es una obra menor y cuándo hace falta licencia, cómo trabajamos, garantía y seguro de ejemplo y preguntas frecuentes.",
                    "What minor works are and when a permit is needed, how we work, an example warranty and insurance, and frequently asked questions."), [
        hero(("Obras, licencias y garantía", "Works, permits and warranty"),
             ("Lo que conviene saber antes de empezar una obra: si hace falta permiso, cómo se organiza y qué pasa si algo falla después.",
              "What it helps to know before starting work: whether you need a permit, how it is organised and what happens if something fails afterwards."),
             [llamar(), whatsapp()], variante="texto"),
        bloque_texto(("Obra menor y licencia", "Minor works and permits"), [
            ("Pintar, cambiar un suelo o alicatar suelen considerarse obra menor. Para otras, como mover tabiques, tocar la fachada o la estructura, el ayuntamiento puede pedir una comunicación o una licencia.",
             "Painting, changing a floor or tiling are usually treated as minor works. For others, such as moving partitions or touching the façade or structure, the council may ask for a notification or a permit."),
            ("Depende de cada municipio y de cada obra, así que lo mejor es preguntar en tu ayuntamiento antes de empezar. Si tu edificio tiene comunidad de vecinos, avísala también. Te orientamos con lo que sabemos, pero la respuesta oficial es la suya.",
             "It depends on each municipality and each job, so the best thing is to ask your council before starting. If your building has a residents' association, tell them too. We guide you with what we know, but the official answer is theirs.")],
            eyebrow=("Permisos", "Permits"), tono="relleno", anchor="licencia"),
        bloque_valores(("Qué incluye cada obra", "What each job includes"), [
            (("Protección", "Protection"), ("Cubrimos suelos, puertas y muebles, y las zonas comunes si las cruzamos.", "We cover floors, doors and furniture, and shared areas if we cross them.")),
            (("Escombros", "Rubble"), ("Los retiramos nosotros y los llevamos a un gestor autorizado. Va indicado en el presupuesto.", "We remove them and take them to an authorised handler. It is stated in the quote.")),
            (("Otros oficios", "Other trades"), ("Si hace falta fontanero, electricista o carpintero, lo coordinamos para que no se pisen.", "If a plumber, electrician or carpenter is needed, we coordinate them so they do not get in each other's way.")),
            (("Limpieza final", "Final clean"), ("Dejamos la vivienda barrida y recogida. La limpieza a fondo se acuerda aparte.", "We leave the home swept and tidy. A deep clean is agreed separately.")),
        ], intro=("Así queda claro qué está dentro del precio y qué no.", "That makes it clear what is in the price and what is not."),
            eyebrow=("Organización", "Organisation"), cols=2, estilo="borde"),
        bloque_texto(("Garantía y seguro", "Warranty and insurance"), [
            ("Garantía de ejemplo: 00 años sobre la obra ejecutada, por escrito en la factura. Cámbiala por la tuya.",
             "Example warranty: 00 years on the work carried out, in writing on the invoice. Change it for your own."),
            ("Seguro de responsabilidad civil de ejemplo, póliza nº 000000, con la compañía X. Cámbialo por el tuyo; si lo pides, te enseñamos el certificado.",
             "Example public liability insurance, policy no. 000000, with company X. Change it for your own; if you ask, we show you the certificate.")],
            eyebrow=("Después de la obra", "After the job"), tono="relleno", anchor="garantia"),
        bloque_faq(("Preguntas frecuentes", "Frequently asked questions"), [
            (("¿Cuánto dura una reforma de baño?", "How long does a bathroom renovation take?"),
             ("Depende de lo que se haga. Tras la visita te damos los días por escrito, con el orden de los trabajos.",
              "It depends on what is done. After the visit we give you the days in writing, with the order of the work.")),
            (("¿Puedo estar en casa durante la obra?", "Can I stay at home during the work?"),
             ("En obras pequeñas, sí. Si te quedas sin baño o sin cocina, lo hablamos antes para organizar los días.",
              "On small jobs, yes. If you will be without a bathroom or kitchen, we talk about it first to plan the days.")),
            (("¿Cómo se paga?", "How is it paid?"),
             ("Lo acordamos en el presupuesto: una parte al empezar y el resto por fases o al terminar. Siempre con factura.",
              "We agree it in the quote: a part at the start and the rest in stages or at the end. Always with an invoice.")),
            (("¿Qué pasa si aparece una humedad al abrir la pared?", "What if damp shows up when the wall is opened?"),
             ("Te enseñamos fotos, te explicamos las opciones y su coste, y no seguimos hasta que lo decides.",
              "We show you photos, explain the options and their cost, and do not go on until you decide.")),
            (("¿Aportáis los materiales?", "Do you supply the materials?"),
             ("Sí, o los pones tú si prefieres elegirlos. El presupuesto lo dice claramente.", "Yes, or you supply them if you prefer to choose. The quote says so clearly.")),
        ], tono="base"),
    ])

    trab = pagina("trabajos", ("trabajos", "work"), ("Trabajos", "Work"),
                  ("Galería de alicatados, tabiques, suelos y reparaciones de humedad.", "Gallery of tiling, partitions, floors and damp repairs."), [
        hero(("Trabajos terminados", "Finished work"),
             ("Algunas obras pequeñas que hemos hecho. El presupuesto lo damos siempre tras ver la obra.",
              "Some small jobs we have done. We always give the quote after seeing the job."),
             [llamar(), whatsapp()], variante="texto"),
        bloque_galeria(("Baños y cocinas", "Bathrooms and kitchens"), [
            ("Baño alicatado con azulejo blanco y suelo gris", "Bathroom tiled in white with a grey floor"),
            ("Cocina con encimera y frente alicatado", "Kitchen with worktop and tiled splashback"),
            ("Plato de ducha de obra con mampara", "Built-in shower tray with screen"),
            ("Suelo de gres en una cocina", "Porcelain floor in a kitchen"),
            ("Aseo pequeño reformado", "Small cloakroom renovated"),
            ("Pared alicatada con cenefa", "Tiled wall with a border"),
        ], tipo="photo", cols=3, ratio=1.2, anchor="banos-galeria",
            intro=("Fotos de ejemplo: sustitúyelas por las de tus trabajos.", "Example photos: replace them with your own work.")),
        bloque_galeria(("Tabiques, suelos y humedades", "Partitions, floors and damp"), [
            ("Tabique nuevo que divide una habitación en dos", "New partition dividing a room in two"),
            ("Falso techo con focos empotrados", "False ceiling with recessed spotlights"),
            ("Pared con humedad antes de repararla", "Damp wall before repair"),
            ("La misma pared reparada y pintada", "The same wall repaired and painted"),
        ], tipo="photo", cols=2, ratio=1.4, anchor="obra-galeria", tono="relleno"),
        bloque_cta(("¿Quieres algo parecido?", "Would you like something similar?"),
                   ("Cuéntanos qué quieres hacer y pasamos a verlo.", "Tell us what you want to do and we will come to see it."),
                   [llamar(), whatsapp("ghost")]),
    ])

    cont = pagina("contacto", ("presupuesto", "quote"), ("Presupuesto sin compromiso", "No-obligation quote"),
                  ("Pide una visita para tu presupuesto: llama, escribe por WhatsApp o usa el formulario. Horario y cómo llegar.",
                   "Ask for a visit for your quote: call, message us on WhatsApp or use the form. Opening hours and how to find us."), [
        hero(("Pide tu presupuesto sin compromiso", "Ask for your no-obligation quote"),
             ("Cuéntanos qué obra necesitas y quedamos para verla. La visita no tiene coste.",
              "Tell us what job you need and we will arrange to see it. The visit is free."),
             [llamar(), whatsapp()], variante="texto"),
        bloque_contacto(("Cuéntanos tu obra", "Tell us about your job"),
                        ("Te llamamos para acordar el día de la visita. Con fotos por WhatsApp podemos hacernos una idea antes.",
                         "We will call you to agree the day of the visit. With photos on WhatsApp we can get an idea beforehand."),
                        campos_cita(servicios=[("Tabiques o falso techo", "Partitions or false ceiling"), ("Alicatado de baño o cocina", "Bathroom or kitchen tiling"),
                                               ("Suelo", "Floor"), ("Humedades", "Damp"), ("Reforma pequeña", "Small renovation"),
                                               ("Patio o exterior", "Patio or exterior"), ("Otro trabajo", "Other job")]),
                        ("Pedir presupuesto", "Ask for a quote"), horas,
                        exito=("Gracias. Te llamamos para quedar el día de la visita.", "Thank you. We will call you to arrange the day of the visit.")),
        bloque_mapa(("Dónde estamos", "Where we are"),
                    ("Vamos a ver la obra donde esté; esta es nuestra dirección de contacto.", "We come to see the job wherever it is; this is our contact address."),
                    tono="relleno"),
    ])

    return plantilla(KEY, ("Albañil y pequeñas obras", "Builder and small works"),
                     ("Cuatro páginas: inicio con servicios y proceso de obra, obras y licencias con garantía y seguro de ejemplo, galería de trabajos y presupuesto con llamada, WhatsApp y formulario.",
                      "Four pages: home with services and the building process, works and permits with an example warranty and insurance, a gallery of work and a quote page with call, WhatsApp and form."),
                     "oficios", ("albanil albanileria obra menor pequenas obras construccion tabiques alicatados azulejos suelos humedades reformas licencia de obra presupuesto sin compromiso",
                                 "builder bricklayer small works construction partitions tiling floors damp renovation building permit quote"),
                     look, cabecera(nav, cta_cab),
                     pie(nav, ("Albañilería y pequeñas obras. Presupuesto sin compromiso tras la visita.",
                               "Bricklaying and small building works. A no-obligation quote after the visit."),
                         ("Lunes a viernes de 7:30 a 17:30.", "Monday to Friday 7:30 am to 5:30 pm.")),
                     [inicio, obras, trab, cont])
