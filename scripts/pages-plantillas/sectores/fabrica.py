# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
from lib import *

KEY = "fabrica"

# Gris chapa y verde máquina. Sin `tono="oscuro"`: primary/inverse no llega a 3.
CHAPA = dict(
    colores=dict(background="#f3f4f1", surface="#e5e7e2", text="#14181a", muted="#475053", primary="#1b6a48",
                 on_primary="#ffffff", border="#d3d6cf", inverse="#16241d", on_inverse="#f1f4ef"),
    fuentes=("ibm-plex-sans", "ibm-plex-sans"), peso=600, h1=(46, 36, 30), radio=(0, 2, 4), ancho=1200)

NOTA_TECNICA = ("Datos técnicos de ejemplo: cámbialos por los de tu fábrica.",
                "Example technical data: replace it with your factory's.")
NOTA_PLAZOS = ("Plazos de ejemplo: ajústalos a los tuyos. El plazo exacto se confirma en el presupuesto.",
               "Example lead times: adjust them to yours. The exact time is confirmed in the quote.")


def build():
    nueva(KEY)
    look = CHAPA
    nav = [("inicio", ("Inicio", "Home")), ("capacidades", ("Capacidades", "Capabilities")),
           ("pedidos", ("Pedidos y plazos", "Orders and lead times")),
           ("contacto", ("Contacto", "Contact"))]
    horas = [(("Oficina y teléfono", "Office and phone"), ("Lunes a viernes, 8:00 a 17:00", "Monday to Friday, 8:00 am to 5:00 pm")),
             (("Recepción de mercancía", "Goods receiving"), ("Lunes a viernes, 8:00 a 13:00", "Monday to Friday, 8:00 am to 1:00 pm")),
             (("Fines de semana", "Weekends"), ("Cerrado", "Closed"))]
    cta_cab = marco([llamar(t=("Llamar", "Call")), boton(("Pedir presupuesto", "Request a quote"), a_pagina("contacto", "solicitud"), "secondary")],
                    dir="row", gap=8, wrap=True)
    ficha = lambda v="secondary": boton(("Solicitar ficha técnica", "Request technical data sheet"), a_pagina("contacto", "solicitud"), v)

    inicio = pagina("inicio", ("", ""), ("Fábrica y taller industrial", "Factory and industrial workshop"),
                    ("Fábrica o taller industrial para clientes profesionales: capacidades, materiales, tolerancias, plazos, ficha técnica y solicitud de presupuesto.",
                     "Factory or industrial workshop for business customers: capabilities, materials, tolerances, lead times, technical data sheet and quote requests."), [
        hero(("Fabricación a medida, de la pieza suelta a la serie", "Made-to-order manufacturing, from a single part to a run"),
             ("Trabajamos con tu plano o con una muestra. Te damos precio y plazo por escrito, controlamos las medidas y entregamos con albarán.",
              "We work from your drawing or a sample. We give you a price and lead time in writing, check the dimensions and deliver with a delivery note."),
             [boton(("Pedir presupuesto", "Request a quote"), a_pagina("contacto", "solicitud")), ficha(), llamar("ghost")],
             eyebrow=("Taller y fábrica", "Workshop and factory"),
             imagen_=("landscape", ("Interior de la nave con maquinaria en funcionamiento", "Inside the workshop with machinery running"))),
        bloque_valores(("Lo que necesita un comprador", "What a buyer needs"), [
            (("Precio y plazo por escrito", "Price and lead time in writing"),
             ("Cada presupuesto lleva precio por unidad, plazo de entrega y condiciones de pago.",
              "Every quote states the unit price, the delivery time and the payment terms.")),
            (("Medidas controladas", "Controlled dimensions"),
             ("Medimos la primera pieza de cada serie y, si lo pides, te mandamos el informe.",
              "We measure the first part of every run and, if you ask, send you the report.")),
            (("Un contacto directo", "One direct contact"),
             ("Hablas con quien prepara tu pedido, no con un buzón. Si hay un cambio, te avisamos antes de fabricar.",
              "You talk to the person who prepares your order, not a mailbox. If there is a change, we tell you before we make anything.")),
        ], tono="relleno", anchor="comprador"),
        bloque_checks(("Qué puedes pedirnos", "What you can ask us for"), [
            ("Prototipos y piezas sueltas.", "Prototypes and one-off parts."),
            ("Series cortas y medias, con calendario de entregas.", "Short and medium runs, with a delivery schedule."),
            ("Pedidos recurrentes con la misma referencia.", "Repeat orders on the same part number."),
            ("Repetir una pieza a partir de una muestra, sin plano.", "Reproducing a part from a sample, with no drawing.")],
            eyebrow=("Tipos de pedido", "Order types"), anchor="pedidos-tipos"),
        bloque_cta(("¿Tienes un plano o una pieza de muestra?", "Do you have a drawing or a sample part?"),
                   ("Mándanos la referencia, la cantidad y para cuándo la necesitas. Te respondemos con precio y plazo.",
                    "Send us the part number, the quantity and when you need it. We reply with a price and lead time."),
                   [boton(("Pedir presupuesto", "Request a quote"), a_pagina("contacto", "solicitud")), ficha("ghost")],
                   tono="relleno"),
    ])

    cap = pagina("capacidades", ("capacidades", "capabilities"), ("Capacidades y ficha técnica", "Capabilities and technical sheet"),
                 ("Procesos, materiales, tolerancias y certificaciones de ejemplo, tabla técnica y botón para solicitar la ficha técnica.",
                  "Example processes, materials, tolerances and certifications, a technical table and a button to request the technical data sheet."), [
        hero(("Capacidades y ficha técnica", "Capabilities and technical sheet"),
             ("Esto es lo que podemos fabricar. Si tu pieza no encaja en la tabla, pregúntanos: muchas veces hay una forma de hacerla.",
              "This is what we can make. If your part does not fit the table, ask us: there is often a way to do it."),
             [ficha("primary"), llamar("secondary")], variante="texto"),
        bloque_precios(("Datos técnicos", "Technical data"), ("Datos técnicos de capacidad de fabricación", "Manufacturing capability technical data"),
                       [("Característica", "Feature"), ("Valor", "Value"), ("Observaciones", "Notes")],
                       [(("Materiales habituales", "Usual materials"), ("Acero al carbono, acero inoxidable, aluminio", "Carbon steel, stainless steel, aluminium"),
                         ("Otros materiales, bajo consulta", "Other materials on request")),
                        (("Espesor", "Thickness"), ("1 a 20 mm", "1 to 20 mm"), ("Según material", "Depends on material")),
                        (("Tamaño máximo de pieza", "Maximum part size"), ("2000 × 1000 × 500 mm", "2000 × 1000 × 500 mm"), ("Piezas mayores, consultar", "Larger parts, ask us")),
                        (("Tolerancia dimensional", "Dimensional tolerance"), ("± 0,1 mm", "± 0.1 mm"), ("Más ajustada, según pieza", "Tighter on some parts")),
                        (("Acabado superficial", "Surface finish"), ("Bruto, pulido, pintado o galvanizado", "Raw, polished, painted or galvanised"),
                         ("Acabados externos con plazo añadido", "Outsourced finishes add lead time")),
                        (("Cantidades", "Quantities"), ("Desde 1 unidad", "From 1 unit"), ("El precio baja por tramos", "Price drops in steps")),
                        (("Formatos de plano", "Drawing formats"), ("PDF, DXF y STEP", "PDF, DXF and STEP"), ("También valen croquis acotados", "Dimensioned sketches also work"))],
                       intro=("Una tabla para saber enseguida si podemos hacer tu pieza. La ficha técnica completa, con todos los procesos, te la mandamos por correo.",
                              "A table to see quickly whether we can make your part. We email you the full technical data sheet, with every process."),
                       nota=NOTA_TECNICA, boton_=ficha("primary"), anchor="datos-tecnicos", tono="base"),
        bloque_valores(("Procesos y materiales", "Processes and materials"), [
            (("Corte y mecanizado", "Cutting and machining"),
             ("Corte, taladrado, fresado y torneado de piezas a medida (procesos de ejemplo: deja los que tengas).",
              "Cutting, drilling, milling and turning of made-to-order parts (example processes: keep the ones you have).")),
            (("Soldadura y montaje", "Welding and assembly"),
             ("Conjuntos soldados y atornillados, con pruebas dimensionales antes de entregar.",
              "Welded and bolted assemblies, with dimensional checks before delivery.")),
            (("Acabados", "Finishes"),
             ("Desbarbado, pulido, pintura y tratamientos superficiales, propios o con proveedores de la zona.",
              "Deburring, polishing, painting and surface treatments, in-house or with local suppliers.")),
        ], tono="relleno", estilo="borde", anchor="procesos"),
        bloque_texto(("Certificaciones y normas", "Certifications and standards"),
                     [("Escribe aquí solo las certificaciones y normas que tu empresa tenga en vigor, con su número de certificado (00-0000) y la entidad que lo emite. Si no tienes ninguna, borra esta sección.",
                       "Write here only the certifications and standards your company holds, with the certificate number (00-0000) and the issuing body. If you have none, delete this section."),
                      ("Podemos entregar certificado de material y de dimensiones cuando el cliente lo pide en el pedido.",
                       "We can supply a material and dimensional certificate when the customer asks for it on the order.")],
                     eyebrow=("Calidad", "Quality"), anchor="certificaciones", tono="base"),
        bloque_checks(("Cómo controlamos la calidad", "How we check quality"), [
            ("Medimos la primera pieza de cada serie.", "We measure the first part of every run."),
            ("Control dimensional por muestreo durante la serie.", "Dimensional sampling during the run."),
            ("Material identificado por lote, para saber de dónde viene.", "Material identified by batch, so you know where it comes from."),
            ("Informe de medición si lo pides en el pedido.", "Measurement report if you ask for it on the order.")],
            intro=("Texto de ejemplo: deja solo lo que de verdad hagas.", "Example text: keep only what you really do."),
            tono="relleno", anchor="control"),
    ])

    ped = pagina("pedidos", ("pedidos", "orders"), ("Pedidos y plazos", "Orders and lead times"),
                 ("Cómo hacer un pedido en cuatro pasos, plazos de entrega, condiciones de pago, transporte y preguntas frecuentes de compradores.",
                  "How to place an order in four steps, delivery lead times, payment terms, transport and common buyer questions."), [
        hero(("Pedidos y plazos", "Orders and lead times"),
             ("Un pedido claro ahorra días. Así lo hacemos para que sepas en cada momento en qué punto está.",
              "A clear order saves days. This is how we do it, so you always know where yours stands."),
             [boton(("Pedir presupuesto", "Request a quote"), a_pagina("contacto", "solicitud")), llamar("secondary")], variante="texto"),
        bloque_pasos(("Cómo hacer un pedido", "How to place an order"), [
            (("Nos mandas el plano", "Send us the drawing"),
             ("Plano o croquis acotado, material, cantidad y fecha en que lo necesitas.",
              "A drawing or dimensioned sketch, the material, the quantity and the date you need it.")),
            (("Presupuesto", "Quote"),
             ("Precio por unidad, plazo y condiciones de pago, por escrito y en pocos días laborables.",
              "Unit price, lead time and payment terms, in writing and within a few working days.")),
            (("Confirmas", "You confirm"),
             ("Con tu conformidad entra en la planificación. Si es un pedido nuevo, aprobamos contigo la primera pieza.",
              "With your go-ahead it enters the schedule. For a new order, we approve the first part with you.")),
            (("Fabricación y control", "Manufacture and checks"),
             ("Fabricamos, medimos y embalamos. Te avisamos si hay cualquier desvío en el plazo.",
              "We make, measure and pack. We tell you about any change to the lead time.")),
        ], tono="base", anchor="proceso-pedido"),
        bloque_precios(("Plazos de entrega", "Delivery lead times"), ("Plazos de entrega por tipo de pedido", "Delivery lead times by order type"),
                       [("Tipo de pedido", "Order type"), ("Plazo habitual", "Usual lead time")],
                       [(("Prototipo o pieza suelta", "Prototype or one-off part"), ("3 a 5 días laborables", "3 to 5 working days")),
                        (("Serie corta", "Short run"), ("1 a 2 semanas", "1 to 2 weeks")),
                        (("Serie media", "Medium run"), ("3 a 5 semanas", "3 to 5 weeks")),
                        (("Pedido recurrente", "Repeat order"), ("Según el calendario pactado", "As per the agreed schedule"))],
                       nota=NOTA_PLAZOS, anchor="plazos", tono="relleno", kind="pricing"),
        bloque_valores(("Pago y entrega", "Payment and delivery"), [
            (("Condiciones de pago", "Payment terms"),
             ("Clientes nuevos, pago por adelantado o contra entrega. Clientes habituales, las condiciones que se pacten por escrito.",
              "New customers, payment upfront or on delivery. Regular customers, the terms agreed in writing.")),
            (("Embalaje", "Packaging"),
             ("Cada pedido sale embalado para el transporte y con albarán que indica referencia y cantidad.",
              "Every order leaves packed for transport with a delivery note showing part number and quantity.")),
            (("Transporte", "Transport"),
             ("Entregamos con transporte propio en la zona o preparamos el pedido para que lo recoja tu transportista.",
              "We deliver with our own transport in the area or prepare the order for your carrier to collect.")),
        ], tono="base", estilo="borde", anchor="entrega"),
        bloque_faq(("Preguntas frecuentes de compradores", "Common buyer questions"), [
            (("¿Cuál es el pedido mínimo?", "What is the minimum order?"),
             ("Hacemos desde una pieza. El precio por unidad baja con la cantidad y te damos varios tramos en el presupuesto.",
              "We make from a single part. The unit price drops with quantity and we give you several price breaks in the quote.")),
            (("¿Qué formatos de plano aceptáis?", "Which drawing formats do you accept?"),
             ("PDF, DXF y STEP. También sirve un croquis con cotas o una pieza de muestra que podamos medir.",
              "PDF, DXF and STEP. A sketch with dimensions or a sample part we can measure also works.")),
            (("¿Mis planos son confidenciales?", "Are my drawings confidential?"),
             ("No los compartimos con nadie ni los usamos para otros clientes. Si necesitas un acuerdo de confidencialidad, lo firmamos.",
              "We do not share them with anyone or use them for other customers. If you need a confidentiality agreement, we sign it.")),
            (("¿Puedo visitar la fábrica?", "Can I visit the factory?"),
             ("Sí, con cita previa y en horario de oficina. Llámanos y lo concretamos.",
              "Yes, by appointment and during office hours. Call us and we will arrange it.")),
        ], tono="relleno"),
    ])

    cont = pagina("contacto", ("contacto", "contact"), ("Contacto y presupuesto", "Contact and quotes"),
                  ("Solicitud de presupuesto o ficha técnica con referencia, cantidad y plazo; horario de recepción de mercancía y cómo llegar.",
                   "Request a quote or technical data sheet with part number, quantity and lead time; goods receiving hours and how to get here."), [
        hero(("Solicita presupuesto o ficha técnica", "Request a quote or technical data sheet"),
             ("Rellena la solicitud con la referencia, la cantidad y el plazo. Si tienes el plano, mándalo por correo con el mismo asunto.",
              "Fill in the request with the part number, quantity and lead time. If you have the drawing, email it with the same subject."),
             [llamar(), boton(("Escribir un correo", "Send an email"), a_correo(), "secondary")], variante="texto"),
        bloque_contacto(("Solicitud", "Request"),
                        ("Respondemos en un día laborable. Para las urgencias, llama.",
                         "We reply within one working day. For urgent matters, call."),
                        [campo("empresa", "text", ("Empresa", "Company"), True, auto="organization", maxlen=160),
                         campo("nombre", "text", ("Persona de contacto", "Contact person"), True, auto="name", maxlen=120),
                         campo("correo", "email", ("Correo electrónico", "Email"), True, auto="email", maxlen=200),
                         campo("telefono", "tel", ("Teléfono", "Phone"), False, auto="tel", maxlen=30),
                         campo("tipo", "select", ("¿Qué necesitas?", "What do you need?"), True,
                               opciones=[("Presupuesto", "A quote"), ("Ficha técnica", "Technical data sheet"), ("Otra consulta", "Another enquiry")]),
                         campo("referencia", "text", ("Referencia o descripción de la pieza", "Part number or description"), False, maxlen=200),
                         campo("cantidad", "number", ("Cantidad", "Quantity")),
                         campo("plazo", "select", ("Plazo que necesitas", "Lead time you need"), False,
                               opciones=[("Urgente, menos de una semana", "Urgent, under a week"), ("De 1 a 2 semanas", "1 to 2 weeks"),
                                         ("De 3 a 6 semanas", "3 to 6 weeks"), ("Sin fecha fija", "No fixed date")]),
                         campo("mensaje", "textarea", ("Detalles (material, acabado, tolerancias)", "Details (material, finish, tolerances)"), False, maxlen=2000)],
                        ("Enviar solicitud", "Send request"), horas, anchor="solicitud",
                        notas=("Las visitas de proveedores y de transportistas, con cita previa.",
                               "Supplier and carrier visits are by appointment."),
                        exito=("Gracias. Hemos recibido tu solicitud y te respondemos en un día laborable.",
                               "Thank you. We have received your request and will reply within one working day.")),
        bloque_mapa(("Cómo llegar a la fábrica", "How to get to the factory"),
                    ("Los camiones entran por la puerta de la nave y descargan en el muelle. Avisa antes de venir con una carga grande.",
                     "Lorries enter by the workshop gate and unload at the dock. Let us know before you come with a large load."),
                    tono="relleno"),
    ])

    return plantilla(KEY, ("Fábrica y taller industrial", "Factory and industrial workshop"),
                     ("Web B2B para fábrica o taller: capacidades, tabla técnica, botón Solicitar ficha técnica, pedidos y plazos, y formulario de presupuesto con horario de recepción y mapa.",
                      "B2B site for a factory or workshop: capabilities, technical table, a Request technical data sheet button, orders and lead times, and a quote form with receiving hours and a map."),
                     "profesionales", ("fabrica taller industrial industria nave metalurgica mecanizado piezas a medida b2b ficha tecnica proveedor",
                                       "factory industrial workshop manufacturing machining made to order parts b2b technical data sheet supplier"),
                     look, cabecera(nav, cta_cab),
                     pie(nav, ("Fabricación a medida para empresas: de la pieza suelta a la serie.",
                               "Made-to-order manufacturing for businesses: from a single part to a run."),
                         ("Oficina de lunes a viernes 8:00 a 17:00. Recepción de mercancía 8:00 a 13:00.",
                          "Office Monday to Friday 8 am to 5 pm. Goods receiving 8 am to 1 pm.")),
                     [inicio, cap, ped, cont])
