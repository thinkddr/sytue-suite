# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
from lib import *
from looks import LOOKS

KEY = "fontanero"


def tel_grande(rotulo):
    """El teléfono como enlace y en grande: lo primero que busca quien tiene una fuga."""
    return marco([peq(rotulo, color="$muted", align="center"),
                  marco([texto("p", (TEL_VISIBLE, TEL_VISIBLE), preset="h1", color="$primary", align="center")],
                        enlace=a_tel())],
                 gap=4, align="center")


def build():
    nueva(KEY)
    look = LOOKS["celeste"]
    nav = [("inicio", ("Inicio", "Home")), ("servicios", ("Servicios y zonas", "Services and areas")),
           ("contacto", ("Contacto", "Contact"))]
    horas = [(("Urgencias", "Emergencies"), ("Todos los días, las 24 horas", "Every day, 24 hours")),
             (("Oficina y presupuestos", "Office and quotes"),
              ("Lunes a viernes, de 9:00 a 18:00", "Monday to Friday, 9:00 am to 6:00 pm"))]
    cta_cab = marco([llamar(t=("Llamar", "Call")), whatsapp("secondary", t=("WhatsApp", "WhatsApp"))],
                    dir="row", gap=8, wrap=True)

    inicio = pagina("inicio", ("", ""), ("Fontanero", "Plumber"),
                    ("Fontanero y calefacción con urgencias las 24 horas: fugas, desatascos, calentadores y reformas de baño. Llama o escribe por WhatsApp.",
                     "Plumber and heating engineer with 24-hour emergency calls: leaks, blocked drains, water heaters and bathroom refits. Call or message us on WhatsApp."), [
        hero(("Fontanero con urgencias las 24 horas", "Plumber with 24-hour emergency calls"),
             ("Fugas, atascos, calentadores y reformas de baño. Llama y te decimos cuánto tardamos en llegar y cuánto cuesta el desplazamiento.",
              "Leaks, blocked drains, water heaters and bathroom refits. Call and we will tell you how long we will take to arrive and what the call-out costs."),
             [llamar(), whatsapp()], variante="centro",
             eyebrow=("Fontanería y calefacción", "Plumbing and heating"),
             extra=tel_grande(("Urgencias, todos los días a cualquier hora", "Emergencies, every day at any hour"))),
        bloque_valores(("Qué arreglamos", "What we fix"), [
            (("Urgencias", "Emergencies"),
             ("Tuberías rotas, fugas fuertes e inundaciones. Si no consigues cortar el agua, llámanos a cualquier hora.",
              "Burst pipes, bad leaks and flooding. If you cannot turn the water off, call us at any hour.")),
            (("Desatascos", "Blocked drains"),
             ("Fregaderos, lavabos, duchas, inodoros y bajantes. Con máquina cuando hace falta.",
              "Sinks, basins, showers, toilets and drainpipes. With a drain machine when needed.")),
            (("Fugas y humedades", "Leaks and damp"),
             ("Buscamos de dónde sale el agua y lo arreglamos. Si hay que abrir pared, te lo decimos antes.",
              "We find where the water is coming from and fix it. If a wall has to be opened, we tell you first.")),
            (("Calentadores y calefacción", "Water heaters and heating"),
             ("Reparación y cambio de calentadores y termos, purgado de radiadores y revisión de la caldera.",
              "Repair and replacement of water heaters and cylinders, radiator bleeding and boiler checks.")),
            (("Grifos y cisternas", "Taps and cisterns"),
             ("Grifos que gotean, cisternas que no paran, llaves de paso que no cierran.",
              "Dripping taps, cisterns that will not stop running, stopcocks that will not close.")),
            (("Reformas de baño", "Bathroom refits"),
             ("Bañera por plato de ducha, sanitarios, grifería y baño completo, con presupuesto cerrado.",
              "Bath to shower tray, toilets, taps and full bathrooms, with a fixed quote.")),
        ], eyebrow=("Servicios", "Services"), estilo="relleno"),
        bloque_pasos(("Cómo trabajamos", "How we work"), [
            (("Llamas", "You call"),
             ("Nos cuentas qué pasa. Te damos un tiempo aproximado de llegada y el precio del desplazamiento.",
              "You tell us what is happening. We give you an approximate arrival time and the call-out price.")),
            (("Vamos a tu casa", "We come to you"),
             ("Vemos la avería y te explicamos qué ha pasado y qué hay que hacer.",
              "We look at the fault and explain what happened and what needs doing.")),
            (("Te damos el precio antes", "You get the price first"),
             ("Mano de obra y material, por separado. Sin tu conformidad no empezamos.",
              "Labour and materials, listed separately. We do not start without your agreement.")),
            (("Arreglamos y dejamos limpio", "We fix it and tidy up"),
             ("Probamos que no hay fugas, recogemos y te damos la factura.",
              "We test that there are no leaks, clear up and give you the invoice.")),
        ], tono="base"),
        bloque_opiniones(("Lo que dicen nuestros clientes", "What our customers say"),
                         [("la rapidez", "the speed"), ("el precio claro", "the clear price"), ("la limpieza", "the tidiness")]),
        bloque_cta(("¿Tienes una fuga ahora mismo?", "Have you got a leak right now?"),
                   ("Cierra la llave de paso general y llámanos.", "Turn off the main stopcock and call us."),
                   [llamar(), whatsapp("ghost")]),
    ])

    serv = pagina("servicios", ("servicios", "services"), ("Servicios, precios y zonas", "Services, prices and areas"),
                  ("Precios orientativos de desplazamiento y mano de obra, qué incluye el presupuesto, reformas de baño y zonas donde trabajamos.",
                   "Guide prices for call-outs and labour, what the quote includes, bathroom refits and the areas we cover."), [
        hero(("Servicios, precios y zonas", "Services, prices and areas"),
             ("Aquí tienes lo que cuesta venir y los trabajos más habituales. El precio exacto lo cerramos contigo antes de empezar.",
              "Here is what a call-out costs and our most common jobs. We agree the exact price with you before we start."),
             [llamar(), whatsapp()], variante="texto"),
        bloque_precios(("Desplazamiento y mano de obra", "Call-out and labour"),
                       ("Precios orientativos de fontanería", "Guide plumbing prices"),
                       [("Concepto", "Item"), ("Precio", "Price")],
                       [(("Desplazamiento en horario normal", "Call-out in normal hours"), eur(25)),
                        (("Desplazamiento urgente de noche, fin de semana o festivo", "Urgent call-out at night, at weekends or on bank holidays"), desde(60)),
                        (("Hora de mano de obra", "Hour of labour"), desde(40)),
                        (("Desatasco de fregadero o lavabo", "Unblocking a sink or basin"), desde(70)),
                        (("Cambio de grifo (sin contar el grifo)", "Tap replacement (tap not included)"), desde(55)),
                        (("Arreglo de cisterna", "Cistern repair"), desde(50)),
                        (("Búsqueda de fuga", "Leak detection"), desde(80)),
                        (("Cambio de termo eléctrico (sin contar el termo)", "Cylinder replacement (cylinder not included)"), desde(90))],
                       nota=("Precios de ejemplo: cámbialos por los tuyos. Incluyen IVA. El material se presupuesta aparte.",
                             "Example prices: replace them with your own. VAT included. Materials are quoted separately."),
                       anchor="precios"),
        bloque_checks(("Presupuesto sin compromiso", "A quote with no obligation"), [
            ("Vemos el trabajo y te damos el precio antes de empezar.", "We look at the job and give you the price before we start."),
            ("Mano de obra y material, cada cosa por separado.", "Labour and materials, each listed separately."),
            ("Si al abrir aparece algo que no se veía, te llamamos antes de seguir.",
             "If something unexpected turns up when we open up, we call you before going on."),
            ("Factura con IVA y garantía del arreglo por escrito.", "An invoice with VAT and a written warranty on the repair."),
        ], eyebrow=("Cómo cobramos", "How we charge"), tono="relleno", anchor="presupuesto"),
        bloque_texto(("Reforma de baño con precio cerrado", "A bathroom refit at a fixed price"), [
            ("Vamos a verlo, medimos y te damos un presupuesto por escrito con lo que incluye y lo que no. Cambiamos bañera por plato de ducha, sanitarios, grifería y tuberías.",
             "We come to see it, measure up and give you a written quote with what is and is not included. We replace baths with shower trays, toilets, taps and pipework."),
            ("Te decimos cuántos días lleva y cuándo no tendrás agua, para que puedas organizarte.",
             "We tell you how many days it takes and when you will be without water, so you can plan ahead.")],
            eyebrow=("Reformas", "Refits"), anchor="reformas",
            imagen_=("photo", ("Baño reformado con plato de ducha y grifería nueva", "Refitted bathroom with a shower tray and new taps")),
            invertir=True),
        bloque_checks(("Zonas donde trabajamos", "Areas we cover"), [
            ("Centro de Tu ciudad", "Your city centre"),
            ("Barrios del norte", "Northern districts"),
            ("Barrios del sur y urbanizaciones", "Southern districts and housing estates"),
            ("Pueblos de alrededor (consúltanos)", "Surrounding villages (ask us)"),
        ], intro=("Si tu zona no aparece, llámanos y te decimos si llegamos.",
                  "If your area is not listed, call us and we will tell you if we can get there."),
            eyebrow=("Zonas de servicio", "Service areas"), anchor="zonas"),
        bloque_faq(("Preguntas frecuentes", "Frequently asked questions"), [
            (("¿Cuánto tardáis en llegar?", "How long do you take to arrive?"),
             ("Depende de dónde estés y de la hora. Al llamar te damos un tiempo aproximado y, si hay retraso, te avisamos.",
              "It depends on where you are and the time. When you call we give you an approximate time and, if we are delayed, we let you know.")),
            (("¿Cobráis más de noche y en festivos?", "Do you charge more at night and on bank holidays?"),
             ("Sí: el desplazamiento de las urgencias nocturnas, de fin de semana y de festivos es mayor. Te decimos el importe por teléfono antes de salir.",
              "Yes: the call-out for night, weekend and bank holiday emergencies is higher. We give you the amount by phone before we set off.")),
            (("¿Qué hago mientras llegáis?", "What should I do until you arrive?"),
             ("Cierra la llave de paso general y, si el agua llega a enchufes o al cuadro, baja la luz desde el cuadro si puedes hacerlo sin riesgo. Recoge el agua con cubos y toallas.",
              "Turn off the main stopcock and, if water is reaching sockets or the fuse box, switch the power off at the fuse box if you can do so safely. Mop up the water with buckets and towels.")),
            (("¿Dais factura y garantía?", "Do you give an invoice and a warranty?"),
             ("Sí. Todos los trabajos llevan factura con IVA, y la garantía del arreglo va por escrito.",
              "Yes. Every job comes with an invoice including VAT, and the warranty on the repair is in writing.")),
        ], tono="relleno"),
    ])

    cont = pagina("contacto", ("contacto", "contact"), ("Contacto y urgencias", "Contact and emergencies"),
                  ("Llama a cualquier hora para una urgencia, o escríbenos por WhatsApp o con el formulario para pedir presupuesto.",
                   "Call at any hour for an emergency, or message us on WhatsApp or with the form to ask for a quote."), [
        hero(("Llámanos o escríbenos", "Call or message us"),
             ("Para una urgencia, llama: el formulario no lo atendemos al momento. Para un presupuesto, usa el formulario o WhatsApp.",
              "For an emergency, call: we do not answer the form straight away. For a quote, use the form or WhatsApp."),
             [llamar(), whatsapp()], variante="centro",
             extra=tel_grande(("Urgencias, las 24 horas", "Emergencies, 24 hours"))),
        bloque_contacto(("Pide presupuesto", "Ask for a quote"),
                        ("Cuéntanos qué pasa y dónde estás. Te llamamos para cerrar el precio.",
                         "Tell us what is happening and where you are. We will call you to agree the price."),
                        campos_cita(servicios=[("Urgencia", "Emergency"), ("Desatasco", "Blocked drain"), ("Fuga o humedad", "Leak or damp"),
                                               ("Calentador o calefacción", "Water heater or heating"),
                                               ("Reforma de baño", "Bathroom refit"), ("Otra avería", "Other fault")],
                                    fecha=False,
                                    extra=[campo("poblacion", "text", ("Población o barrio", "Town or area"), True, maxlen=120),
                                           campo("urgente", "select", ("¿Es una urgencia?", "Is it an emergency?"), False,
                                                 opciones=[("No, puede esperar", "No, it can wait"), ("Sí, ahora mismo", "Yes, right now")])]),
                        ("Pedir presupuesto", "Ask for a quote"), horas,
                        exito=("Gracias. Te llamamos lo antes posible. Si es una urgencia, llama al teléfono que ves en esta página.",
                               "Thank you. We will call you as soon as possible. If it is an emergency, call the number on this page.")),
        bloque_mapa(("Dónde está la oficina", "Where the office is"),
                    ("Vamos a tu casa, no hace falta que vengas. Esta es nuestra oficina.",
                     "We come to you, so there is no need to visit. This is our office."), tono="relleno"),
    ])

    return plantilla(KEY, ("Fontanero y calefacción", "Plumber and heating"),
                     ("Urgencias 24 horas con el teléfono bien visible, servicios y zonas, precios de desplazamiento orientativos y presupuesto por WhatsApp o formulario.",
                      "24-hour emergencies with the phone number front and centre, services and areas, guide call-out prices and quotes by WhatsApp or form."),
                     "oficios", ("fontanero fontaneria plomero urgencias 24 horas desatascos fugas calentador calefaccion caldera reforma de bano grifo tuberias",
                                 "plumber plumbing emergency 24 hour drain unblocking blocked drain leak water heater boiler heating bathroom refit tap pipes"),
                     look, cabecera(nav, cta_cab),
                     pie(nav, ("Fontanería y calefacción. Urgencias las 24 horas y presupuesto sin compromiso.",
                               "Plumbing and heating. 24-hour emergencies and no-obligation quotes."),
                         ("Urgencias 24 horas. Oficina de lunes a viernes de 9:00 a 18:00.", "Emergencies 24 hours. Office Monday to Friday 9 am to 6 pm.")),
                     [inicio, serv, cont])
