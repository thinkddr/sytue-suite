# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
import lib
from lib import *

KEY = "arquitecto-interiorismo"

# Paleta propia: piedra y grafito, casi sin color. Pasa AA (se comprueba al construir).
ESTUDIO = dict(
    colores=dict(background="#fafaf7", surface="#efede7", text="#1b1b19", muted="#5b5a54", primary="#46544c",
                 on_primary="#ffffff", border="#dcd9d0", inverse="#222421", on_inverse="#f3f2ee"),
    fuentes=("work-sans", "work-sans"), peso=500, h1=(58, 44, 34), radio=(0, 0, 2), ancho=1240)


def ficha(alt, titulo, tipo, sup, texto_):
    """Un proyecto del portfolio: foto, nombre, tipo y superficie de ejemplo."""
    return marco([
        imagen("photo", alt, ratio=1.4),
        marco([h3(titulo),
               peq((f"{tipo[0]} · Superficie: {sup} m² (de ejemplo)", f"{tipo[1]} · Floor area: {sup} m² (example)"),
                   color="$muted"),
               p(texto_, color="$muted")], gap=6)], tag="li", gap=14)


def fases(items):
    """Proceso en fases: lista numerada a una columna, con el número grande a la izquierda."""
    li = []
    for i, (h, t) in enumerate(items, 1):
        li.append(marco([
            texto("p", (f"0{i}", f"0{i}"), preset="h3", color="$primary", sz={"width": 56, "height": "hug"}),
            marco([h3(h), p(t, color="$muted")], gap=6, sz={"width": "fill", "height": "hug", "min_width": 0})],
            tag="li", dir="row", gap=20, pad=[20, 0, 20, 0],
            style={"border": {"width": 1, "color": "$border", "style": "solid"}, "radius": 0}))
    return marco(li, tag="ol", gap=0, sz={"width": "fill", "height": "hug", "max_width": 820})


def build():
    nueva(KEY)
    malos = lib.comprobar_paleta("estudio", ESTUDIO["colores"])
    assert not malos, malos
    look = ESTUDIO
    nav = [("inicio", ("Inicio", "Home")), ("proyectos", ("Proyectos", "Projects")),
           ("estudio", ("Estudio y honorarios", "Studio and fees")), ("contacto", ("Contacto", "Contact"))]
    horas = [(("Lunes a jueves, con cita", "Monday to Thursday, by appointment"),
              ("9:00 a 14:00 y 16:00 a 19:00", "9:00 am to 2:00 pm and 4:00 pm to 7:00 pm")),
             (("Viernes, con cita", "Friday, by appointment"), ("9:00 a 14:00", "9:00 am to 2:00 pm")),
             (("Sábado y domingo", "Saturday and Sunday"), ("Cerrado", "Closed"))]
    proyecto = ("Cuéntanos tu proyecto", "Tell us about your project")

    def contar(var="primary"):
        return boton(proyecto, a_pagina("contacto", "contacto"), var)

    cta_cab = marco([llamar("secondary", ("Llamar", "Call")), whatsapp("ghost", ("WhatsApp", "WhatsApp"))],
                    dir="row", gap=12, wrap=True)

    destacados = [
        ficha(("Salón y comedor con luz natural tras la reforma", "Living and dining room with natural light after the refurbishment"),
              ("Reforma integral de vivienda", "Full home refurbishment"), ("Reforma", "Refurbishment"), 90,
              ("Escribe aquí en dos líneas qué pidió el cliente y qué se hizo.",
               "Write here, in two lines, what the client asked for and what was done.")),
        ficha(("Cocina abierta al salón con isla de madera", "Open-plan kitchen with a timber island"),
              ("Cocina abierta al salón", "Kitchen opened to the living room"), ("Interiorismo", "Interior design"), 24,
              ("Escribe aquí qué cambió en la distribución y en los materiales.",
               "Write here what changed in the layout and in the materials.")),
        ficha(("Local comercial reformado con mostrador y estanterías", "Refurbished shop with counter and shelving"),
              ("Local comercial", "Shop unit"), ("Local", "Commercial"), 60,
              ("Escribe aquí para quién era, qué había antes y cómo quedó.",
               "Write here who it was for, what was there before and how it turned out.")),
    ]

    inicio = pagina("inicio", ("", ""), ("Estudio de arquitectura e interiorismo", "Architecture and interior design studio"),
                    ("Estudio de arquitectura e interiorismo: proyectos de reforma, dirección de obra, licencias, portfolio, honorarios explicados y formulario para contar tu proyecto.",
                     "Architecture and interior design studio: refurbishment projects, site supervision, permits, portfolio, fees explained and a form to tell us about your project."), [
        hero(("Espacios claros, obras sin sorpresas", "Clear spaces, builds without surprises"),
             ("Estudio pequeño de arquitectura e interiorismo. Proyectamos tu reforma, la dibujamos al detalle y vigilamos la obra para que se haga como se pensó.",
              "A small architecture and interior design studio. We design your refurbishment, draw it in detail and supervise the work so it is built as planned."),
             [contar(), llamar("secondary")],
             eyebrow=("Arquitectura e interiorismo", "Architecture and interior design"),
             imagen_=("landscape", ("Salón luminoso de una vivienda reformada por el estudio", "Bright living room in a home refurbished by the studio"))),
        bloque_valores(("Qué hacemos", "What we do"), [
            (("Proyecto de reforma", "Refurbishment design"),
             ("Medimos, dibujamos y te enseñamos cómo quedará: planos, materiales y presupuesto de obra.",
              "We measure, draw and show you how it will look: plans, materials and a build budget.")),
            (("Interiorismo", "Interior design"),
             ("Distribución, iluminación, color y mobiliario, para que el espacio sirva a quien lo vive.",
              "Layout, lighting, colour and furniture, so the space works for the people who live in it.")),
            (("Dirección de obra", "Site supervision"),
             ("Visitamos la obra, resolvemos dudas con el constructor y comprobamos que se hace lo dibujado.",
              "We visit the site, settle questions with the builder and check that what was drawn is what is built.")),
            (("Licencias y trámites", "Permits and paperwork"),
             ("Preparamos la documentación que pida tu ayuntamiento para la licencia o la declaración responsable.",
              "We prepare the documents your council asks for the building licence or the declaration of works.")),
        ], cols=2, eyebrow=("Servicios", "Services"), estilo="linea", tono="base", anchor="servicios"),
        seccion("gallery", [
            titular(("Proyectos recientes", "Recent projects"),
                    ("Tres de los últimos. Proyectos de ejemplo: cámbialos por los tuyos.",
                     "Three of the latest. Example projects: replace them with your own."), align="start"),
            rejilla(destacados, cols=3, gap=28),
            marco([boton(("Ver todos los proyectos", "See all projects"), a_pagina("proyectos"), "secondary")]),
        ], tono="relleno", anchor="proyectos"),
        bloque_texto(("Primero escuchamos, después dibujamos", "We listen first, then we draw"), [
            ("En la primera reunión no traemos soluciones hechas. Te preguntamos cómo vives, qué te molesta de tu casa y qué no quieres perder.",
             "At the first meeting we do not bring ready-made solutions. We ask how you live, what bothers you about your home and what you do not want to lose."),
            ("Con eso hacemos un anteproyecto sencillo para decidir juntos antes de gastar en planos de detalle.",
             "From that we make a simple preliminary design so we can decide together before spending on detailed drawings."),
        ], imagen_=("photo", ("Mesa de trabajo con planos y muestras de materiales", "Work table with plans and material samples")),
            tono="base", invertir=True),
        bloque_opiniones(("Lo que dicen quienes han reformado con nosotros", "What people who refurbished with us say"),
                         [("el proyecto", "the design"), ("la obra", "the build"), ("el trato", "the service")],
                         tono="relleno"),
        bloque_cta(("¿Tienes un espacio que quieres cambiar?", "Is there a space you want to change?"),
                   ("Cuéntanos qué tienes y qué quieres. Te respondemos con los siguientes pasos, sin compromiso.",
                    "Tell us what you have and what you want. We reply with the next steps, no commitment."),
                   [contar(), whatsapp("ghost")], tono="base"),
    ])

    proyectos = pagina("proyectos", ("proyectos", "projects"), ("Proyectos", "Projects"),
                       ("Portfolio del estudio: reformas de vivienda, cocinas, locales y oficinas, con tipo de proyecto y superficie.",
                        "The studio's portfolio: home refurbishments, kitchens, shops and offices, with project type and floor area."), [
        hero(("Proyectos", "Projects"),
             ("Cada ficha dice qué tipo de encargo fue y cuánto espacio había. Si algo se parece a lo tuyo, cuéntanoslo.",
              "Each card says what kind of commission it was and how much space there was. If something looks like yours, tell us."),
             [contar()], variante="texto"),
        seccion("gallery", [
            titular(("Obras y reformas", "Built work and refurbishments"),
                    ("Proyectos de ejemplo: cámbialos por los tuyos y borra los que no uses.",
                     "Example projects: replace them with your own and delete the ones you do not use.")),
            rejilla([
                ficha(("Vivienda reformada con suelo de madera y paredes blancas", "Refurbished home with timber floor and white walls"),
                      ("Reforma integral de vivienda", "Full home refurbishment"), ("Reforma", "Refurbishment"), 90,
                      ("Escribe aquí qué pidió el cliente y qué se hizo.", "Write here what the client asked for and what was done.")),
                ficha(("Cocina abierta al salón con isla de madera", "Open-plan kitchen with a timber island"),
                      ("Cocina abierta al salón", "Kitchen opened to the living room"), ("Interiorismo", "Interior design"), 24,
                      ("Escribe aquí qué cambió en la distribución.", "Write here what changed in the layout.")),
                ficha(("Baño con ducha de obra y azulejo claro", "Bathroom with a walk-in shower and pale tiles"),
                      ("Baño con ducha a ras de suelo", "Bathroom with a level-access shower"), ("Reforma", "Refurbishment"), 6,
                      ("Escribe aquí qué problema había y cómo se resolvió.", "Write here what the problem was and how it was solved.")),
                ficha(("Local comercial reformado con mostrador y estanterías", "Refurbished shop with counter and shelving"),
                      ("Local comercial", "Shop unit"), ("Local", "Commercial"), 60,
                      ("Escribe aquí para quién era y cómo quedó.", "Write here who it was for and how it turned out.")),
                ficha(("Ático rehabilitado con terraza y luz natural", "Rehabilitated attic flat with a terrace and natural light"),
                      ("Rehabilitación de ático", "Attic rehabilitation"), ("Rehabilitación", "Rehabilitation"), 75,
                      ("Escribe aquí qué se conservó y qué se cambió.", "Write here what was kept and what was changed.")),
                ficha(("Oficina pequeña con mesas de trabajo y luz cálida", "Small office with work desks and warm light"),
                      ("Oficina para un equipo pequeño", "Office for a small team"), ("Interiorismo", "Interior design"), 45,
                      ("Escribe aquí cómo se organizó el espacio.", "Write here how the space was organised.")),
            ], cols=2, gap=36, tablet=2),
        ], tono="base", anchor="portfolio"),
        bloque_cta(("¿Quieres ver algo parecido en tu casa?", "Want something like this in your home?"),
                   ("Cuéntanos qué tienes y qué buscas. Te decimos por dónde empezar.",
                    "Tell us what you have and what you are after. We will say where to start."),
                   [contar(), llamar("ghost")], tono="relleno"),
    ])

    estudio = pagina("estudio", ("estudio", "studio"), ("Estudio y honorarios", "Studio and fees"),
                     ("El equipo colegiado, las cinco fases de un proyecto, cómo se calculan los honorarios y preguntas frecuentes.",
                      "The registered team, the five phases of a project, how fees are worked out and frequently asked questions."), [
        hero(("El estudio y cómo trabajamos", "The studio and how we work"),
             ("Somos un equipo pequeño: la persona que te atiende en la primera reunión es la que dibuja y la que visita la obra.",
              "We are a small team: the person who meets you at the first meeting is the one who draws and visits the site."),
             [contar(), llamar("secondary")], variante="texto"),
        bloque_equipo(("Equipo", "Team"), [
            (("Arquitecto o arquitecta", "Architect"),
             (f"{COLEGIADO[0]}. Dirige los proyectos y firma la documentación.",
              f"{COLEGIADO[1]}. Leads the projects and signs the documents.")),
            (("Interiorista", "Interior designer"),
             ("Presenta aquí en una frase cómo trabaja con los clientes.", "Introduce here, in one sentence, how they work with clients.")),
        ], cols=2, intro=("Los datos de colegiación van siempre a la vista: es lo que da tranquilidad a quien contrata.",
                          "Registration details are always on show: it gives peace of mind to whoever hires."),
            tono="base"),
        seccion("features", [
            titular(("Cómo es un proyecto, fase a fase", "What a project looks like, phase by phase"),
                    ("Al final de cada fase decides si seguimos. No empezamos la siguiente sin tu visto bueno.",
                     "At the end of each phase you decide whether we go on. We do not start the next one without your approval."),
                    eyebrow=("Proceso", "Process")),
            fases([
                (("Primera reunión", "First meeting"),
                 ("Vemos el espacio, escuchamos qué necesitas y te decimos si es viable y por dónde empezaría.",
                  "We see the space, listen to what you need and tell you whether it is feasible and where we would start.")),
                (("Anteproyecto", "Preliminary design"),
                 ("Dos o tres croquis y una primera idea de coste. Aquí es fácil cambiar de opinión.",
                  "Two or three sketches and a first idea of cost. This is when it is easy to change your mind.")),
                (("Proyecto", "Detailed design"),
                 ("Planos, mediciones, materiales y presupuesto cerrado para que los constructores ofrezcan lo mismo.",
                  "Plans, measurements, materials and a fixed budget so builders quote for the same thing.")),
                (("Obra", "Build"),
                 ("Visitamos la obra de forma periódica, decidimos contigo los imprevistos y revisamos las certificaciones.",
                  "We visit the site regularly, decide with you on the unexpected and check the stage payments.")),
                (("Entrega", "Handover"),
                 ("Revisamos pieza a pieza, apuntamos lo pendiente y te damos los documentos finales de la obra.",
                  "We go through everything piece by piece, list what is left and hand over the final documents.")),
            ]),
        ], tono="relleno", anchor="proceso"),
        seccion("pricing", [
            titular(("Honorarios, explicados en llano", "Fees, explained plainly"),
                    ("No hay una tarifa única. Antes de empezar recibes un presupuesto por escrito.",
                     "There is no single rate. Before we start you get a written quote."),
                    eyebrow=("Honorarios", "Fees")),
            rejilla([
                tarjeta([h3(("De qué depende", "What it depends on")),
                         p(("De la superficie, de lo que haya que cambiar y del trabajo de dibujo y de gestión que lleve. No es lo mismo cambiar una cocina que rehabilitar un piso entero.",
                            "On the floor area, on what has to change and on the drawing and admin work involved. Changing a kitchen is not the same as rehabilitating a whole flat."),
                           color="$muted")], estilo="linea"),
                tarjeta([h3(("Una cantidad cerrada por fase", "A fixed amount per phase")),
                         p(("Te damos el importe de cada fase por escrito. Pagas al cerrarla y, si no seguimos, solo has pagado lo hecho.",
                            "We give you the amount for each phase in writing. You pay when it closes and, if we stop, you have only paid for what was done."),
                           color="$muted")], estilo="linea"),
                tarjeta([h3(("Dirección de obra", "Site supervision")),
                         p(("Se presupuesta aparte: sobre el coste de la obra o por visitas. Te explicamos cuál conviene a tu caso antes de firmar.",
                            "Quoted separately: on the cost of the build or per visit. We explain which suits your case before you sign."),
                           color="$muted")], estilo="linea"),
                tarjeta([h3(("Lo que no incluyen", "What they do not include")),
                         p(("Las tasas del ayuntamiento, el visado del colegio cuando es obligatorio, los honorarios de otros técnicos si hacen falta y la propia obra, que pagas al constructor.",
                            "Council fees, the professional body's endorsement fee where it is compulsory, other specialists' fees if needed and the build itself, which you pay to the builder."),
                           color="$muted")], estilo="linea"),
            ], cols=2, gap=24),
            boton(("Pedir presupuesto sin compromiso", "Ask for a quote with no commitment"),
                  a_pagina("contacto", "contacto")),
        ], tono="base", anchor="honorarios", gap=28),
        bloque_faq(("Preguntas frecuentes", "Frequently asked questions"), [
            (("¿Necesito un arquitecto para una reforma?", "Do I need an architect for a refurbishment?"),
             ("Depende de lo que toques. Si cambias estructura, fachada o distribución con obra, normalmente sí. Para pintar o cambiar el suelo, no. Lo vemos en la primera reunión y lo confirmamos con tu ayuntamiento.",
              "It depends on what you change. If you alter structure, façade or layout with building work, usually yes. For painting or replacing a floor, no. We check it at the first meeting and confirm it with your council.")),
            (("¿Cuánto tarda un proyecto?", "How long does a project take?"),
             ("Un anteproyecto, de dos a cuatro semanas. El proyecto, de cuatro a ocho. La licencia depende del ayuntamiento y no la controlamos.",
              "A preliminary design takes two to four weeks. The detailed design, four to eight. The licence depends on the council and is out of our hands.")),
            (("¿Puedo usar mi propio constructor?", "Can I use my own builder?"),
             ("Sí. Y si no tienes, te proponemos varios para que compares presupuestos sobre el mismo proyecto.",
              "Yes. And if you do not have one, we suggest several so you can compare quotes on the same design.")),
            (("¿Qué llevo a la primera reunión?", "What should I bring to the first meeting?"),
             ("Planos o fotos del espacio, si los tienes, y una lista de lo que quieres cambiar. Si no hay planos, medimos nosotros.",
              "Plans or photos of the space, if you have them, and a list of what you want to change. If there are no plans, we measure.")),
            (("¿La primera reunión tiene coste?", "Does the first meeting cost anything?"),
             ("Escribe aquí si la primera reunión es gratuita o tiene un coste, y cuánto dura.",
              "Write here whether the first meeting is free or has a fee, and how long it lasts.")),
        ], tono="relleno"),
        bloque_cta(("¿Hablamos de tu proyecto?", "Shall we talk about your project?"), None,
                   [contar(), whatsapp("ghost")], tono="base"),
    ])

    cont = pagina("contacto", ("contacto", "contact"), ("Contacto y proyecto", "Contact and project"),
                  ("Cuéntanos tu proyecto con el formulario, por teléfono o por WhatsApp. Horario de reuniones y cómo llegar al estudio.",
                   "Tell us about your project with the form, by phone or on WhatsApp. Meeting hours and how to find the studio."), [
        hero(("Cuéntanos tu proyecto", "Tell us about your project"),
             ("Dinos qué espacio es, dónde está y qué quieres cambiar. Te contestamos con una propuesta de primera reunión.",
              "Tell us which space it is, where it is and what you want to change. We reply with a proposal for a first meeting."),
             [llamar(), whatsapp()], variante="texto"),
        bloque_contacto(("Cómo encontrarnos", "How to find us"),
                        ("Las reuniones son con cita. Si prefieres que veamos tu vivienda o local, vamos nosotros.",
                         "Meetings are by appointment. If you prefer, we come and see your home or shop."),
                        campos_consulta(
                            mensaje_etiqueta=("Cuéntanos tu proyecto", "Tell us about your project"),
                            servicios=[("Proyecto de reforma", "Refurbishment design"), ("Interiorismo", "Interior design"),
                                       ("Dirección de obra", "Site supervision"), ("Licencias y trámites", "Permits and paperwork"),
                                       ("No lo sé todavía", "I am not sure yet")],
                            extra=[campo("inmueble", "select", ("Qué espacio es", "Type of space"), False,
                                         opciones=[("Vivienda", "Home"), ("Local o comercio", "Shop or commercial unit"),
                                                   ("Oficina", "Office"), ("Otro", "Other")]),
                                   campo("municipio", "text", ("Municipio", "Town or city"), False, maxlen=120),
                                   campo("superficie", "text", ("Superficie aproximada (m²)", "Approximate floor area (m²)"),
                                         False, maxlen=20),
                                   campo("planos", "select", ("¿Tienes planos?", "Do you have plans?"), False,
                                         opciones=[("Sí", "Yes"), ("No", "No"), ("No lo sé", "Not sure")])]),
                        ("Enviar mi proyecto", "Send my project"), horas,
                        exito=("Gracias. Hemos recibido tu proyecto y te contestamos con una propuesta de primera reunión.",
                               "Thank you. We have received your project and will reply with a proposal for a first meeting.")),
        bloque_mapa(tono="relleno"),
    ])

    return plantilla(KEY, ("Arquitecto e interiorismo", "Architect and interior design"),
                     ("Estudio de arquitectura e interiorismo con servicios, portfolio de proyectos, proceso en fases, honorarios explicados, equipo colegiado y formulario para contar tu proyecto.",
                      "Architecture and interior design studio with services, project portfolio, phased process, fees explained, registered team and a form to tell us about your project."),
                     "creativos", ("arquitecto arquitecta estudio de arquitectura interiorismo interiorista decoracion reforma reformas proyecto de reforma direccion de obra licencia de obra anteproyecto",
                                   "architect architecture studio interior design interior designer refurbishment renovation site supervision planning permission building licence"),
                     look, cabecera(nav, cta_cab),
                     pie(nav, (f"Estudio de arquitectura e interiorismo. {COLEGIADO[0]}.",
                               f"Architecture and interior design studio. {COLEGIADO[1]}."),
                         ("Lunes a jueves 9:00 a 14:00 y 16:00 a 19:00. Viernes 9:00 a 14:00. Con cita.",
                          "Monday to Thursday 9 am to 2 pm and 4 pm to 7 pm. Friday 9 am to 2 pm. By appointment.")),
                     [inicio, proyectos, estudio, cont])
