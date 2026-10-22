# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
from lib import *

KEY = "taller-motos"

# Asfalto oscuro con rojo señal. Se comprueba AA al construir.
ASFALTO = dict(background="#15151a", surface="#202027", text="#f3f2f0", muted="#b5b3b8", primary="#ff5a68",
               on_primary="#1a0709", border="#34343d", inverse="#0b0b0e", on_inverse="#f3f2f0")

NOTA_PRECIOS = ("Precios de ejemplo: cámbialos por los tuyos. Incluyen IVA y mano de obra; los recambios se presupuestan aparte.",
                "Example prices: replace them with your own. VAT and labour included; parts are quoted separately.")
CAB = [("Trabajo", "Job"), ("Precio", "Price")]


def build():
    malos = comprobar_paleta("asfalto", ASFALTO)
    assert not malos, malos
    nueva(KEY)
    look = dict(colores=ASFALTO, fuentes=("work-sans", "dm-sans"), peso=800, mayus="uppercase", ls=0.02,
                h1=(48, 38, 30), radio=(2, 4, 8))
    nav = [("inicio", ("Inicio", "Home")), ("taller", ("Taller y precios", "Workshop and prices")),
           ("motos", ("Motos y equipamiento", "Bikes and gear")), ("contacto", ("Cita previa", "Book a slot"))]
    horas = [(("Lunes a viernes", "Monday to Friday"),
              ("9:00 a 13:30 y 16:00 a 19:30", "9:00 am to 1:30 pm and 4:00 pm to 7:30 pm")),
             (("Sábado", "Saturday"), ("9:30 a 13:30", "9:30 am to 1:30 pm")),
             (("Domingo", "Sunday"), ("Cerrado", "Closed"))]
    cta_cab = marco([llamar(t=("Llamar", "Call")), whatsapp("secondary", t=("WhatsApp", "WhatsApp"))],
                    dir="row", gap=8, wrap=True)
    cita = lambda v="primary": boton(("Pedir cita", "Book a slot"), a_pagina("contacto", "contacto"), v)

    inicio = pagina("inicio", ("", ""), ("Taller de motos", "Motorbike workshop"),
                    ("Taller y venta de motos y scooters: mantenimiento, ITV, neumáticos, frenos, puesta a punto de primavera, equipamiento del motorista y cita previa.",
                     "Motorbike and scooter workshop and sales: servicing, ITV, tyres, brakes, spring check-up, rider gear and booking."), [
        hero(("Tu moto a punto, y tú sabiendo qué se le hace", "Your bike ready to ride, and you knowing what is done to it"),
             ("Mantenimiento, frenos, neumáticos y preparación de la ITV para motos y scooters. Te explicamos el trabajo y te damos el precio antes de empezar.",
              "Servicing, brakes, tyres and ITV preparation for motorbikes and scooters. We explain the job and give you the price before we start."),
             [cita(), llamar("secondary"), whatsapp("ghost")],
             eyebrow=("Taller de motos y scooters", "Motorbike and scooter workshop"),
             imagen_=("landscape", ("Moto elevada en el banco de trabajo del taller", "Motorbike raised on the workshop bench"))),
        bloque_valores(("Qué hacemos", "What we do"), [
            (("Mantenimiento", "Servicing"),
             ("Aceite, filtros, bujías, cadena, líquidos y revisión según el manual de tu moto o scooter.",
              "Oil, filters, spark plugs, chain, fluids and servicing as set out in your bike's or scooter's manual.")),
            (("ITV", "ITV test"),
             ("Revisión previa para llegar a la ITV sin sorpresas y arreglo de lo que te hayan marcado.",
              "A check before the ITV test so there are no surprises, and repairs for anything it flagged.")),
            (("Neumáticos", "Tyres"),
             ("Montaje y equilibrado. Te decimos cuándo se acaba el dibujo y cuándo todavía aguantan.",
              "Fitting and balancing. We tell you when the tread is done and when they still have life left.")),
            (("Frenos", "Brakes"),
             ("Pastillas, discos y líquido. Te enseñamos la pieza gastada antes de cambiarla.",
              "Pads, discs and fluid. We show you the worn part before we replace it.")),
            (("Transmisión", "Drive"),
             ("Cadena, piñón y corona, o correa y variador en scooters: ajuste, engrase y cambio.",
              "Chain and sprockets, or belt and variator on scooters: adjustment, lubrication and replacement.")),
            (("Electricidad", "Electrics"),
             ("Batería, luces, intermitentes y arranque. Si no arranca, lo miramos con el polímetro.",
              "Battery, lights, indicators and starting. If it will not start, we check it with a multimeter.")),
        ], eyebrow=("Servicios", "Services"), estilo="relleno", cols=3),
        seccion("features", [
            marco([
                marco([etiqueta(("Temporada", "Season")), h2(("Puesta a punto de primavera", "Spring check-up")),
                       p(("Si tu moto ha pasado el invierno parada, antes de la primera salida conviene mirarla: batería, neumáticos, frenos, cadena y líquidos.",
                          "If your bike has sat through the winter, it is worth checking before the first ride: battery, tyres, brakes, chain and fluids."),
                         color="$muted"),
                       marco([boton(("Ver qué incluye", "See what is included"), a_pagina("taller", "primavera"), "secondary"),
                              cita("ghost")], dir="row", gap=12, wrap=True)],
                      gap=14, sz={"width": "fill", "height": "hug", "min_width": 0}),
            ], dir="row", gap=0),
        ], tono="base", anchor="primavera-aviso", pad_v=56),
        bloque_pasos(("Cómo trabajamos", "How we work"), [
            (("Nos cuentas qué le pasa", "You tell us what is wrong"),
             ("Por teléfono, WhatsApp o en el taller: ruidos, cuándo falla y cuántos kilómetros lleva.",
              "By phone, WhatsApp or at the workshop: noises, when it fails and how many kilometres it has.")),
            (("La revisamos", "We check it"),
             ("Con la moto delante y, si hace falta, con la máquina de diagnosis.",
              "With the bike in front of us and, if needed, the diagnostic tool.")),
            (("Presupuesto por escrito", "A written quote"),
             ("Recambios, mano de obra e IVA por separado. No empezamos sin tu visto bueno.",
              "Parts, labour and VAT listed separately. We do not start without your go-ahead.")),
            (("Te avisamos y la recoges", "We call and you collect it"),
             ("Te llamamos cuando está lista. Factura detallada y, si quieres, los recambios cambiados.",
              "We call when it is ready. An itemised invoice and, if you want, the replaced parts.")),
        ], tono="relleno"),
        bloque_opiniones(("Lo que dicen los motoristas", "What riders say"),
                         [("el presupuesto", "the quote"), ("el plazo", "the turnaround"), ("el trato", "the service")], tono="base"),
        bloque_cta(("¿Qué le pasa a tu moto?", "What is wrong with your bike?"),
                   ("Cuéntanoslo y te decimos qué hueco tenemos para verla.",
                    "Tell us and we will find you a slot to look at it."), [cita(), llamar("secondary")], tono="relleno"),
    ])

    taller = pagina("taller", ("taller-y-precios", "workshop-and-prices"), ("Taller y precios", "Workshop and prices"),
                    ("Precios orientativos de mantenimiento, neumáticos, frenos y transmisión, preparación de la ITV, puesta a punto de primavera y moto de sustitución.",
                     "Guide prices for servicing, tyres, brakes and drive, ITV preparation, spring check-up and courtesy bike."), [
        hero(("Taller y precios", "Workshop and prices"),
             ("Estos son nuestros precios orientativos. El final depende de la moto y de los recambios, y te lo damos por escrito antes de empezar.",
              "These are guide prices. The final price depends on the bike and the parts, and you get it in writing before we start."),
             [cita(), llamar("secondary")], variante="texto"),
        bloque_precios(("Mantenimiento y puesta a punto", "Servicing and check-ups"), ("Precios de mantenimiento", "Servicing prices"), CAB,
                       [(("Revisión básica (aceite, filtro y puntos de control)", "Basic service (oil, filter and check points)"), desde(70)),
                        (("Revisión completa según el manual", "Full service as per the manual"), desde(130)),
                        (("Puesta a punto de primavera", "Spring check-up"), desde(60)),
                        (("Cambio de batería", "Battery replacement"), desde(25)),
                        (("Revisión previa a la ITV", "Pre-ITV check"), desde(30))],
                       nota=NOTA_PRECIOS, anchor="mantenimiento"),
        bloque_precios(("Neumáticos, frenos y transmisión", "Tyres, brakes and drive"), ("Precios de neumáticos, frenos y transmisión", "Tyre, brake and drive prices"), CAB,
                       [(("Montaje y equilibrado de una rueda", "Fitting and balancing one wheel"), desde(15)),
                        (("Pastillas de freno, un eje", "Brake pads, one axle"), desde(35)),
                        (("Cambio de líquido de frenos", "Brake fluid change"), desde(30)),
                        (("Kit de arrastre (cadena, piñón y corona)", "Chain kit (chain and sprockets)"), desde(120)),
                        (("Correa y rodillos del variador (scooter)", "Belt and variator rollers (scooter)"), desde(90))],
                       nota=NOTA_PRECIOS, anchor="neumaticos", tono="relleno"),
        bloque_checks(("Qué incluye la puesta a punto de primavera", "What the spring check-up includes"), [
            ("Estado y carga de la batería.", "Battery condition and charge."),
            ("Presión y dibujo de los neumáticos, y su edad.", "Tyre pressure, tread and age."),
            ("Pastillas, discos y nivel del líquido de frenos.", "Pads, discs and brake fluid level."),
            ("Tensión y engrase de la cadena, o estado de la correa.", "Chain tension and lubrication, or belt condition."),
            ("Aceite, refrigerante y filtro de aire.", "Oil, coolant and air filter."),
            ("Luces, intermitentes, claxon y cables de gas y embrague.", "Lights, indicators, horn and throttle and clutch cables."),
        ], intro=("Si algo necesita arreglo, te lo presupuestamos aparte y tú decides.",
                  "If something needs fixing, we quote it separately and you decide."),
            eyebrow=("Primavera", "Spring"), anchor="primavera"),
        bloque_texto(("Antes de pasar la ITV", "Before the ITV test"), [
            ("Cuándo toca pasarla depende de la antigüedad y del tipo de vehículo. Si tienes dudas, mira la ficha técnica o pregúntanos.",
             "When it is due depends on the age and type of vehicle. If in doubt, look at the technical data sheet or ask us."),
            ("Antes revisamos luces, frenos, neumáticos, escape, holguras de la dirección y emisiones, que es donde suele haber fallos.",
             "Beforehand we check lights, brakes, tyres, exhaust, steering play and emissions, which is where faults usually turn up.")],
            eyebrow=("ITV", "ITV test"), anchor="itv", tono="relleno"),
        bloque_texto(("Moto de sustitución", "Courtesy bike"), [
            ("Texto de ejemplo: cuenta aquí si prestas o alquilas una moto o un scooter mientras reparas la tuya, con qué fianza y si hay que reservarla.",
             "Sample text: explain here whether you lend or rent a bike or scooter while repairing theirs, with what deposit and whether it must be booked."),
            ("Si la necesitas, pídela al reservar la cita: depende de la disponibilidad y de que tengas el permiso adecuado.",
             "If you need one, ask when you book: it depends on availability and on you holding the right licence.")],
            eyebrow=("Mientras la arreglamos", "While we repair yours"), anchor="sustitucion"),
        bloque_faq(("Preguntas frecuentes", "Frequently asked questions"), [
            (("¿Me cobráis por el presupuesto?", "Do you charge for a quote?"),
             ("Mirar la moto y presupuestar una avería sencilla no tiene coste. Si hace falta diagnosis con máquina, te lo decimos antes.",
              "Looking at the bike and quoting a simple fault is free. If it needs a diagnostic scan, we tell you first.")),
            (("¿Cuánto tarda una revisión?", "How long does a service take?"),
             ("Una revisión básica, entre una y dos horas. Si hay que pedir un recambio, te damos el plazo en el presupuesto.",
              "A basic service takes one to two hours. If a part has to be ordered, we give you the timescale in the quote.")),
            (("¿Qué garantía tienen las reparaciones?", "What warranty do repairs have?"),
             ("Recambios y mano de obra llevan garantía, y su plazo va por escrito en la factura.",
              "Parts and labour are under warranty, and the period is written on the invoice.")),
            (("¿Reparáis cualquier marca?", "Do you repair every make?"),
             ("Trabajamos con motos y scooters de la mayoría de marcas. Si es un caso especial, llámanos antes de traerla.",
              "We work on motorbikes and scooters of most makes. If it is a special case, call us before bringing it in.")),
        ], tono="relleno"),
    ])

    motos = pagina("motos", ("motos-y-equipamiento", "bikes-and-gear"), ("Motos y equipamiento", "Bikes and gear"),
                   ("Venta de motos y scooters, y qué equipamiento necesita el motorista: casco, guantes, chaqueta, pantalón, botas y protecciones.",
                    "Motorbike and scooter sales, and the gear a rider needs: helmet, gloves, jacket, trousers, boots and protection."), [
        hero(("Motos, scooters y equipamiento", "Bikes, scooters and gear"),
             ("Te ayudamos a elegir moto según lo que haces con ella, y a no salir sin lo que te protege.",
              "We help you choose a bike for what you do with it, and not to ride without what protects you."),
             [cita(), whatsapp("secondary")], variante="texto"),
        bloque_valores(("Venta de motos y scooters", "Bike and scooter sales"), [
            (("Nuevas", "New"),
             ("Te enseñamos las que tenemos y pedimos la que quieras, con plazo de entrega por escrito.",
              "We show you what we have and order the one you want, with the delivery time in writing.")),
            (("De ocasión", "Used"),
             ("Revisadas en el taller antes de venderlas. Te damos el estado, los kilómetros y el historial que conocemos.",
              "Checked in the workshop before sale. We tell you the condition, the mileage and the history we know."),),
            (("Para tu permiso", "For your licence"),
             ("Te decimos qué motos puedes llevar según tu permiso (AM, A1, A2 o A). Pregúntanos antes de elegir.",
              "We tell you which bikes you can ride with your licence (AM, A1, A2 or A). Ask us before choosing.")),
        ], eyebrow=("Motos", "Bikes"), cols=3, estilo="borde", anchor="venta"),
        bloque_valores(("Equipamiento del motorista", "Rider gear"), [
            (("Casco", "Helmet"),
             ("Homologado, de tu talla y bien abrochado. Si ha tenido un golpe fuerte, se cambia.",
              "Approved, your size and fastened properly. If it has taken a hard knock, replace it.")),
            (("Guantes", "Gloves"),
             ("Que cubran bien la muñeca y no se muevan al agarrar. Mejor con protecciones en los nudillos.",
              "That cover the wrist well and do not shift when you grip. Better with knuckle protection.")),
            (("Chaqueta y pantalón", "Jacket and trousers"),
             ("Con protecciones en hombros, codos y rodillas, y que se ajusten sin apretar. De verano o de invierno, según cuándo salgas.",
              "With shoulder, elbow and knee protection, fitting snugly without pinching. Summer or winter, depending on when you ride.")),
            (("Botas", "Boots"),
             ("Que sujeten el tobillo y tengan buena suela para apoyar en parado.",
              "That support the ankle and have a good sole for stopping.")),
            (("Ropa de lluvia y visibilidad", "Rain gear and visibility"),
             ("Un mono o chubasquero ligero y prendas reflectantes para cuando oscurece.",
              "A light suit or waterproof and reflective items for when it gets dark.")),
            (("Antirrobo", "Anti-theft"),
             ("Cadena o candado de disco y anclaje. Te contamos qué te conviene según dónde la dejes.",
              "A chain or disc lock and an anchor. We tell you what suits you depending on where you park.")),
        ], intro=("Pasa por el taller y pruébatelo: la talla y el ajuste importan más que el precio.",
                  "Come by the workshop and try it on: size and fit matter more than price."),
            eyebrow=("Equípate", "Gear up"), estilo="borde", tono="relleno", cols=3, anchor="equipamiento"),
        bloque_cta(("¿Quieres ver una moto o probarte un casco?", "Want to see a bike or try on a helmet?"),
                   ("Dinos cuándo te viene bien y te esperamos en el taller.", "Tell us when suits you and we will be waiting at the workshop."),
                   [cita(), llamar("secondary")], tono="base"),
    ])

    cont = pagina("contacto", ("contacto", "contact"), ("Cita previa", "Book a slot"),
                  ("Pide cita para tu moto por teléfono, WhatsApp o con el formulario, y mira cómo llegar al taller.",
                   "Book a slot for your bike by phone, WhatsApp or with the form, and see how to get to the workshop."), [
        hero(("Pide cita para tu moto", "Book your bike in"),
             ("Llámanos, escríbenos por WhatsApp o rellena el formulario. Te confirmamos el día y la hora.",
              "Call us, message us on WhatsApp or fill in the form. We confirm the day and time."),
             [llamar(), whatsapp()], variante="texto"),
        bloque_contacto(("Cuéntanos qué necesita tu moto", "Tell us what your bike needs"),
                        ("Si no arranca o no se puede mover, dínoslo y vemos cómo traerla.",
                         "If it will not start or cannot be moved, tell us and we will work out how to get it here."),
                        campos_cita(servicios=[("Mantenimiento", "Servicing"), ("Puesta a punto de primavera", "Spring check-up"),
                                               ("ITV", "ITV test"), ("Neumáticos", "Tyres"), ("Frenos", "Brakes"),
                                               ("Ver una moto", "See a bike"), ("Otra avería", "Other fault")],
                                    extra=[campo("vehiculo", "select", ("Tipo de vehículo", "Type of vehicle"), False,
                                                 opciones=[("Moto", "Motorbike"), ("Scooter", "Scooter"), ("Ciclomotor", "Moped"), ("Otro", "Other")]),
                                           campo("modelo", "text", ("Marca y modelo (opcional)", "Make and model (optional)"), False, maxlen=80),
                                           campo("sustitucion", "select", ("¿Necesitas moto de sustitución?", "Do you need a courtesy bike?"), False,
                                                 opciones=[("No", "No"), ("Sí, si hay disponible", "Yes, if one is available")])]),
                        ("Pedir cita", "Request a slot"), horas,
                        notas=("Los datos de tu moto solo los usamos para preparar la cita.", "We only use your bike details to prepare the appointment."),
                        exito=("Gracias. Te confirmamos la cita por teléfono o WhatsApp en cuanto la tengamos.",
                               "Thank you. We will confirm your appointment by phone or WhatsApp as soon as we have it.")),
        bloque_mapa(tono="relleno"),
    ])

    return plantilla(KEY, ("Taller de motos", "Motorbike workshop"),
                     ("Taller y venta de motos y scooters con mantenimiento, ITV, neumáticos, frenos, puesta a punto de primavera, equipamiento del motorista y cita previa.",
                      "Motorbike and scooter workshop and sales with servicing, ITV, tyres, brakes, spring check-up, rider gear and booking."),
                     "motor", ("taller de motos taller de moto mecanico de motos moto scooter scooters ciclomotor motocicleta itv neumaticos frenos puesta a punto primavera cadena equipamiento motorista casco concesionario de motos",
                               "motorbike workshop motorcycle garage bike mechanic scooter moped motorbike repair mot itv tyres brakes spring check up chain rider gear helmet motorbike dealer"),
                     look, cabecera(nav, cta_cab),
                     pie(nav, ("Taller y venta de motos y scooters, con cita previa y presupuesto por escrito.",
                               "Motorbike and scooter workshop and sales, by appointment and with a written quote."),
                         ("Lunes a viernes 9:00 a 13:30 y 16:00 a 19:30. Sábado 9:30 a 13:30.",
                          "Monday to Friday 9 am to 1:30 pm and 4 pm to 7:30 pm. Saturday 9:30 am to 1:30 pm.")),
                     [inicio, taller, motos, cont])
