# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
from lib import *
from looks import LOOKS

KEY = "reparacion-moviles-informatica"

# Paleta propia: verde noche con primario menta (oscura; fondos $inverse permitidos, primary/inverse = 10).
NOCHE_MENTA = dict(background="#0f1512", surface="#18211c", text="#eaf3ee", muted="#a3b5ab", primary="#4fd1a0",
                   on_primary="#08130e", border="#2b3a32", inverse="#070b09", on_inverse="#eaf3ee")

CAB = [("Reparación", "Repair"), ("Plazo orientativo", "Guide time"), ("Precio", "Price")]


def build():
    nueva(KEY)
    look = dict(colores=NOCHE_MENTA, fuentes=("ibm-plex-sans", "inter"), peso=600, h1=(48, 38, 30),
                radio=(6, 10, 14), ancho=1160)
    nav = [("inicio", ("Inicio", "Home")), ("reparaciones", ("Reparaciones y precios", "Repairs and prices")),
           ("contacto", ("Presupuesto", "Quote"))]
    horas = [(("Lunes a viernes", "Monday to Friday"),
              ("10:00 a 14:00 y 17:00 a 20:30", "10:00 am to 2:00 pm and 5:00 pm to 8:30 pm")),
             (("Sábado", "Saturday"), ("10:00 a 14:00", "10:00 am to 2:00 pm")),
             (("Domingo", "Sunday"), ("Cerrado", "Closed"))]
    cta_cab = marco([llamar(t=("Llamar", "Call")), whatsapp("secondary", t=("WhatsApp", "WhatsApp"))],
                    dir="row", gap=8, wrap=True)
    pedir = lambda v="primary": boton(("Pedir presupuesto", "Get a quote"), a_pagina("contacto", "contacto"), v)

    inicio = pagina("inicio", ("", ""), ("Reparación de móviles y ordenadores", "Phone and computer repair"),
                    ("Reparación de móviles, tablets y ordenadores: pantallas, baterías, conectores, virus y recuperación de datos, con presupuesto sin compromiso.",
                     "Repairs for phones, tablets and computers: screens, batteries, charging ports, viruses and data recovery, with a no-obligation quote."), [
        hero(("Tu móvil u ordenador, de nuevo en marcha", "Your phone or computer, back up and running"),
             ("Pantallas, baterías, conectores, virus y lentitud. Lo miramos, te damos el presupuesto sin compromiso y no tocamos nada sin tu visto bueno.",
              "Screens, batteries, charging ports, viruses and slowness. We look at it, give you a no-obligation quote and touch nothing without your go-ahead."),
             [pedir(), llamar("secondary"), whatsapp("ghost")],
             eyebrow=("Reparación de móviles, tablets y ordenadores", "Phone, tablet and computer repair"),
             variante="centro"),
        bloque_valores(("Qué arreglamos", "What we fix"), [
            (("Móviles", "Phones"),
             ("Pantalla, batería, conector de carga, cámara, altavoz y micrófono. Si se ha mojado, tráelo cuanto antes.",
              "Screen, battery, charging port, camera, speaker and microphone. If it got wet, bring it in as soon as you can.")),
            (("Tablets", "Tablets"),
             ("Pantalla rota, batería hinchada o agotada, que no carga o que va muy lenta.",
              "Cracked screen, swollen or worn-out battery, not charging or running very slowly.")),
            (("Ordenadores", "Computers"),
             ("Portátiles y de sobremesa: virus, lentitud, disco, memoria, pantalla, limpieza y sistema operativo.",
              "Laptops and desktops: viruses, slowness, disk, memory, screen, cleaning and operating system.")),
        ], eyebrow=("Equipos", "Devices"), estilo="borde"),
        bloque_checks(("Lo que nos traen cada semana", "What people bring us every week"), [
            ("La pantalla rota o que no responde al tacto.", "A cracked screen, or one that does not respond to touch."),
            ("La batería que se acaba en media tarde o que se calienta.", "A battery that is flat by mid-afternoon or gets hot."),
            ("El cargador que hay que mover para que cargue: casi siempre es el conector.",
             "A charger you have to wiggle to make it work: nearly always the port."),
            ("El ordenador que tarda minutos en arrancar o saca ventanas que no has pedido.",
             "A computer that takes minutes to start up or shows pop-ups you did not ask for."),
            ("Fotos y archivos que no se abren desde el disco o la tarjeta.", "Photos and files that will not open from a disk or card."),
            ("Un equipo que no enciende y no sabes por qué.", "A device that will not switch on and you do not know why."),
        ], cols=2, tono="relleno", anchor="averias"),
        bloque_pasos(("Cómo trabajamos", "How we work"), [
            (("Lo traes o nos escribes", "You bring it in or message us"),
             ("Pásate por la tienda en horario o cuéntanos por WhatsApp qué le pasa y qué marca y modelo es.",
              "Drop in during opening hours or tell us on WhatsApp what is wrong and what make and model it is.")),
            (("Lo revisamos y te presupuestamos", "We check it and quote you"),
             ("Te decimos qué tiene, cuánto cuesta y cuánto tarda. Mirarlo no te compromete a nada.",
              "We tell you what is wrong, how much it costs and how long it takes. Looking at it commits you to nothing.")),
            (("Copia de seguridad y reparación", "Backup and repair"),
             ("Si hay datos en riesgo, antes de reparar hacemos copia, siempre con tu permiso.",
              "If data is at risk, we make a backup before repairing, always with your permission.")),
            (("Te avisamos y lo pruebas", "We call you and you try it"),
             ("Lo probamos contigo al recogerlo. Te damos la factura y el plazo de garantía por escrito.",
              "We test it with you when you collect it. You get the invoice and the warranty period in writing.")),
        ], tono="base"),
        bloque_texto(("Tus datos, antes que nada", "Your data comes first"), [
            ("Un móvil o un ordenador guarda tu vida: fotos, mensajes, contraseñas. No miramos nada de eso. Si necesitamos tu código para probar el equipo, lo desbloqueas tú delante de nosotros.",
             "A phone or a computer holds your life: photos, messages, passwords. We do not look at any of it. If we need your code to test the device, you unlock it yourself in front of us."),
            ("Si puedes, haz tú una copia antes de traerlo. Si no puedes, la hacemos nosotros con tu permiso, siempre que el equipo lo permita.",
             "If you can, make a backup before bringing it in. If you cannot, we make it with your permission, as long as the device allows it.")],
            eyebrow=("Privacidad y copias", "Privacy and backups"), tono="relleno", anchor="datos"),
        bloque_opiniones(("Lo que dicen nuestros clientes", "What our customers say"),
                         [("la reparación", "the repair"), ("el plazo", "the turnaround"), ("el trato", "the service")]),
        bloque_cta(("¿Qué le pasa a tu equipo?", "What is wrong with your device?"),
                   ("Cuéntanoslo y te damos un presupuesto sin compromiso.",
                    "Tell us and we will give you a no-obligation quote.")),
    ])

    rep = pagina("reparaciones", ("reparaciones", "repairs"), ("Reparaciones y precios", "Repairs and prices"),
                 ("Precios y plazos de ejemplo para móviles, tablets y ordenadores, recuperación de datos, qué hacer antes de traer el equipo y preguntas frecuentes.",
                  "Example prices and turnaround times for phones, tablets and computers, data recovery, what to do before bringing your device in and common questions."), [
        hero(("Reparaciones, plazos y precios", "Repairs, turnaround times and prices"),
             ("Estos son los precios y los plazos habituales. El importe final lo confirmamos tras revisar el equipo y antes de empezar.",
              "These are the usual prices and turnaround times. We confirm the final amount after checking the device and before we start."),
             [pedir()], variante="texto"),
        bloque_precios(("Móviles y tablets", "Phones and tablets"), ("Precios y plazos en móviles y tablets", "Phone and tablet prices and turnaround times"), CAB,
                       [(("Cambio de pantalla", "Screen replacement"), ("1 a 2 horas", "1 to 2 hours"), desde(69)),
                        (("Cambio de batería", "Battery replacement"), ("30 a 60 minutos", "30 to 60 minutes"), desde(39)),
                        (("Conector de carga", "Charging port"), ("1 a 2 horas", "1 to 2 hours"), desde(45)),
                        (("Altavoz o micrófono", "Speaker or microphone"), ("1 a 2 horas", "1 to 2 hours"), desde(35)),
                        (("Cámara trasera o delantera", "Rear or front camera"), ("1 a 2 horas", "1 to 2 hours"), desde(49)),
                        (("Equipo mojado: revisión y limpieza", "Water damage: check and clean"), ("1 a 3 días", "1 to 3 days"), desde(30))],
                       nota=("Precios de ejemplo: cámbialos por los tuyos. IVA y pieza incluidos. Los plazos son orientativos y cuentan desde que tenemos la pieza.",
                             "Example prices: replace them with your own. VAT and part included. Times are a guide and start once we have the part."),
                       anchor="moviles", tono="base"),
        bloque_precios(("Ordenadores", "Computers"), ("Precios y plazos en ordenadores", "Computer prices and turnaround times"), CAB,
                       [(("Limpieza de virus y puesta a punto", "Virus removal and tune-up"), ("1 a 2 días", "1 to 2 days"), desde(39)),
                        (("Formateo e instalación del sistema, con copia de tus datos", "Reinstall and system setup, with a backup of your data"),
                         ("1 a 2 días", "1 to 2 days"), desde(55)),
                        (("Cambio de disco por uno SSD (mano de obra)", "Swap to an SSD (labour)"), ("1 día", "1 day"), desde(35)),
                        (("Ampliación de memoria (mano de obra)", "Memory upgrade (labour)"), ("30 a 60 minutos", "30 to 60 minutes"), desde(20)),
                        (("Limpieza interior y pasta térmica", "Internal clean and thermal paste"), ("Mismo día", "Same day"), desde(35)),
                        (("Cambio de pantalla de portátil (mano de obra)", "Laptop screen replacement (labour)"),
                         ("2 a 5 días según la pieza", "2 to 5 days depending on the part"), desde(45))],
                       nota=("Precios de ejemplo: cámbialos por los tuyos. IVA incluido. En ordenadores, las piezas se cobran aparte y te las presupuestamos antes.",
                             "Example prices: replace them with your own. VAT included. On computers, parts are charged separately and quoted beforehand."),
                       anchor="ordenadores", tono="relleno"),
        bloque_texto(("Recuperación de datos", "Data recovery"), [
            ("Si tu disco, tu tarjeta o tu móvil no abre los archivos, miramos si se pueden salvar. A veces sí, a veces no, y depende de lo dañado que esté.",
             "If your disk, card or phone will not open your files, we check whether they can be saved. Sometimes they can and sometimes not, depending on how badly damaged it is."),
            ("No podemos asegurar el resultado antes de revisarlo. Te decimos qué posibilidades hay y cuánto costaría, y no seguimos adelante sin tu permiso. Y deja de usar el equipo cuanto antes: cada uso puede pisar lo que queremos recuperar.",
             "We cannot promise a result before checking it. We tell you what the chances are and what it would cost, and we do not go ahead without your permission. And stop using the device straight away: every use can overwrite what we want to recover.")],
            eyebrow=("Datos", "Data"), anchor="recuperacion", tono="base",
            imagen_=("product", ("Disco duro abierto sobre una mesa de trabajo", "Open hard drive on a workbench"))),
        bloque_checks(("Antes de traer tu equipo", "Before you bring your device in"), [
            ("Haz una copia de seguridad si puedes: fotos, contactos y documentos.",
             "Make a backup if you can: photos, contacts and documents."),
            ("Trae el cargador y, en ordenadores, el adaptador.", "Bring the charger and, for computers, the power adapter."),
            ("Ten a mano el código de desbloqueo y las cuentas de acceso, por si el equipo las pide.",
             "Have your unlock code and account logins to hand in case the device asks for them."),
            ("Apunta qué pasa y cuándo empezó. Cuanto más detalle, antes lo encontramos.",
             "Make a note of what happens and when it started. The more detail, the sooner we find it."),
        ], cols=2, tono="relleno", anchor="antes"),
        bloque_faq(("Preguntas frecuentes", "Frequently asked questions"), [
            (("¿Me cobráis por el presupuesto?", "Do you charge for a quote?"),
             ("Mirar el equipo y darte un presupuesto no te compromete a nada. Si hace falta desmontar o llevar tiempo, te lo decimos antes de cobrar.",
              "Looking at the device and giving you a quote commits you to nothing. If it needs taking apart or takes time, we tell you before charging.")),
            (("¿Perderé mis datos al reparar?", "Will I lose my data when it is repaired?"),
             ("Un cambio de pantalla o de batería no los toca. En un formateo sí se borra todo, por eso hacemos copia antes con tu permiso.",
              "A screen or battery replacement does not touch them. A reinstall does wipe everything, which is why we make a backup first with your permission.")),
            (("¿Y si no tiene arreglo?", "What if it cannot be repaired?"),
             ("Te lo decimos con franqueza. Pagas solo lo que hayamos hecho para revisarlo, y siempre lo habremos acordado antes.",
              "We tell you frankly. You only pay for what we did to check it, and we will have agreed that beforehand.")),
            (("¿Tienen garantía las reparaciones?", "Are repairs under warranty?"),
             ("Garantía de ejemplo: 3 meses en la pieza y en la mano de obra. Cámbiala por la tuya. El plazo va por escrito en la factura.",
              "Example warranty: 3 months on the part and labour. Replace it with your own. The period is written on the invoice.")),
            (("¿Reparáis cualquier marca?", "Do you repair any make?"),
             ("Casi todas. Dinos la marca y el modelo y te decimos si conseguimos la pieza y en cuánto tiempo.",
              "Almost all of them. Tell us the make and model and we will say whether we can get the part and how long it takes.")),
        ], tono="base"),
        bloque_cta(("¿Te lo miramos?", "Shall we take a look?"), None, [pedir(), llamar("secondary")], tono="oscuro"),
    ])

    cont = pagina("contacto", ("contacto", "contact"), ("Presupuesto", "Quote"),
                  ("Pide presupuesto sin compromiso por teléfono, WhatsApp o formulario, y mira cómo llegar a la tienda.",
                   "Ask for a no-obligation quote by phone, WhatsApp or the form, and see how to get to the shop."), [
        hero(("Pide tu presupuesto", "Ask for your quote"),
             ("Cuéntanos qué equipo es y qué le pasa. También puedes pasarte por la tienda en horario, sin cita.",
              "Tell us which device it is and what is wrong. You can also drop into the shop during opening hours, no appointment needed."),
             [llamar(), whatsapp()], variante="texto"),
        bloque_contacto(("Cómo encontrarnos", "How to find us"),
                        ("Si no puedes venir, mándanos fotos por WhatsApp y te damos una idea del coste.",
                         "If you cannot come in, send us photos on WhatsApp and we will give you an idea of the cost."),
                        campos_cita(servicios=[("Móvil", "Phone"), ("Tablet", "Tablet"), ("Portátil", "Laptop"),
                                               ("Ordenador de sobremesa", "Desktop computer"), ("Otro equipo", "Other device")],
                                    fecha=False,
                                    extra=[campo("modelo", "text", ("Marca y modelo", "Make and model"), False, maxlen=80),
                                           campo("problema", "select", ("¿Qué le pasa?", "What is wrong?"), False,
                                                 opciones=[("Pantalla rota", "Cracked screen"), ("Batería", "Battery"),
                                                           ("No carga", "Not charging"), ("Lento o con virus", "Slow or infected"),
                                                           ("No enciende", "Will not switch on"), ("Recuperar datos", "Recover data"),
                                                           ("Otra cosa", "Something else")]),
                                           campo("copia", "select", ("¿Tienes copia de seguridad?", "Do you have a backup?"), False,
                                                 opciones=[("Sí", "Yes"), ("No", "No"), ("No lo sé", "I do not know")])]),
                        ("Pedir presupuesto", "Request a quote"), horas,
                        notas=("Los datos del equipo solo los usamos para prepararte el presupuesto.",
                               "We only use the device details to prepare your quote."),
                        exito=("Gracias. Te respondemos con el presupuesto por teléfono o WhatsApp lo antes posible.",
                               "Thank you. We will reply with your quote by phone or WhatsApp as soon as we can.")),
        bloque_mapa(tono="relleno"),
    ])

    return plantilla(KEY, ("Reparación de móviles y ordenadores", "Phone and computer repair"),
                     ("Reparación de pantallas, baterías, conectores, virus y datos con presupuesto sin compromiso, precios y plazos de ejemplo y copia de seguridad antes de reparar.",
                      "Repairs for screens, batteries, ports, viruses and data with a no-obligation quote, example prices and turnaround times and a backup before repairing."),
                     "comercio", ("reparacion de moviles reparar movil arreglar movil pantalla rota cambio de bateria tablet ordenador portatil informatico arreglar ordenador virus lento recuperar datos formatear servicio tecnico informatica tienda de informatica",
                                  "phone repair mobile repair screen repair battery replacement tablet laptop computer repair virus removal slow computer data recovery it repair shop"),
                     look, cabecera(nav, cta_cab),
                     pie(nav, ("Reparación de móviles, tablets y ordenadores con presupuesto sin compromiso.",
                               "Phone, tablet and computer repair with a no-obligation quote."),
                         ("Lunes a viernes 10:00 a 14:00 y 17:00 a 20:30. Sábado 10:00 a 14:00.",
                          "Monday to Friday 10 am to 2 pm and 5 pm to 8:30 pm. Saturday 10 am to 2 pm.")),
                     [inicio, rep, cont])
