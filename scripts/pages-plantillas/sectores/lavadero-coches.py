# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
from lib import *

KEY = "lavadero-coches"

# Paleta propia de este sector (no existe en looks.py): agua y espuma.
ESPUMA = dict(colores=dict(background="#f6fcfd", surface="#e3f4f7", text="#0a2530", muted="#3f5f6b",
                           primary="#00697f", on_primary="#ffffff", border="#c6e3ea", inverse="#06303c",
                           on_inverse="#e6f7fa"),
              fuentes=("manrope", "work-sans"), peso=800, radio=(8, 14, 22), ancho=1120)

NOTA = ("Precios de ejemplo: cámbialos por los tuyos. Incluyen IVA.", "Example prices: replace them with your own. VAT included.")


def build():
    nueva(KEY)
    malos = comprobar_paleta("espuma", ESPUMA["colores"])
    assert not malos, malos
    look = ESPUMA
    nav = [("inicio", ("Inicio", "Home")), ("tarifas", ("Tarifas y bonos", "Prices and passes")),
           ("contacto", ("Cita y contacto", "Booking and contact"))]
    horas = [(("Lunes a viernes", "Monday to Friday"), ("9:00 a 20:00", "9:00 am to 8:00 pm")),
             (("Sábado", "Saturday"), ("9:00 a 15:00", "9:00 am to 3:00 pm")),
             (("Domingo", "Sunday"), ("Cerrado", "Closed"))]
    cta_cab = marco([llamar(t=("Llamar", "Call")), whatsapp("secondary", t=("WhatsApp", "WhatsApp"))],
                    dir="row", gap=8, wrap=True)
    pedir = lambda v="primary": boton(("Pedir cita", "Book a slot"), a_pagina("contacto", "contacto"), v)
    ver_tarifas = lambda v="secondary": boton(("Ver tarifas", "See prices"), a_pagina("tarifas", "a-mano"), v)

    inicio = pagina("inicio", ("", ""), ("Lavadero de coches", "Car wash"),
                    ("Lavadero de coches a mano y en túnel: tarifas por tipo de vehículo, extras, bonos mensuales, horario y cita por teléfono, WhatsApp o formulario.",
                     "Hand and tunnel car wash: prices by vehicle type, extras, monthly passes, opening hours and booking by phone, WhatsApp or form."), [
        hero(("Tu coche limpio, a mano o en cinco minutos", "Your car clean, by hand or in five minutes"),
             ("Lavado a mano con cita para cuidar la pintura, y túnel sin cita para cuando tienes prisa. Los precios, claros y por tipo de vehículo.",
              "Hand wash by appointment to look after the paintwork, and a no-appointment tunnel for when you are in a hurry. Clear prices by vehicle type."),
             [pedir(), llamar("secondary"), whatsapp("ghost")],
             eyebrow=("Lavadero de coches", "Car wash"), variante="centro", tono="relleno"),
        bloque_valores(("Dos formas de lavar", "Two ways to wash"), [
            (("A mano, con cita", "By hand, by appointment"),
             ("Agua, manopla y secado con paño de microfibra, llantas y cristales incluidos. Es el más suave con la pintura y llega a los rincones. Dura entre 30 y 40 minutos para un exterior.",
              "Water, wash mitt and a microfibre drying cloth, wheels and windows included. It is the gentlest on the paintwork and reaches the corners. An exterior takes 30 to 40 minutes.")),
            (("En el túnel, sin cita", "In the tunnel, no appointment"),
             ("Entras, esperas unos cinco o diez minutos y sales con el coche limpio y seco. Es solo exterior. Si llevas baca, antena exterior o el coche está modificado, pregúntanos antes.",
              "You drive in, wait five to ten minutes and drive out clean and dry. Exterior only. If you have a roof rack or an external aerial, or the car is modified, ask us first.")),
        ], eyebrow=("Cómo lavamos", "How we wash"), cols=2, estilo="relleno", anchor="formas"),
        bloque_pasos(("Cómo es la visita", "What a visit looks like"), [
            (("Llegas y nos dices qué quieres", "You arrive and tell us what you want"),
             ("Exterior, interior o completo. Si hay una mancha concreta, enséñanosla.",
              "Exterior, interior or full. If there is a particular stain, show us.")),
            (("Dejas tus cosas a mano", "Take your things with you"),
             ("Llévate lo de valor y retira lo que haya en el salpicadero y en la bandeja trasera.",
              "Take anything valuable and clear whatever is on the dashboard and the parcel shelf.")),
            (("Esperas o vuelves", "Wait or come back"),
             ("En la sala de espera o dando una vuelta. Te avisamos cuando está listo.",
              "In the waiting area or out for a walk. We call you when it is ready.")),
            (("Lo revisas y pagas", "You check it and pay"),
             ("Mira el coche con nosotros. Si algo se nos ha pasado, lo repasamos en el momento.",
              "Look the car over with us. If we missed anything, we go over it there and then.")),
        ], tono="base"),
        bloque_texto(("Con cita o sin cita", "With or without an appointment"),
                     [("El túnel y el lavado exterior a mano de un turismo se hacen sin cita, por orden de llegada. Si vienes en hora punta, como el mediodía o el sábado por la mañana, puede haber cola.",
                       "The tunnel and a hand exterior wash for a car are done without an appointment, in order of arrival. At busy times, such as lunchtime or Saturday morning, there may be a queue."),
                      ("Los lavados completos, la tapicería, el pulido y el encerado llevan cita, porque necesitan tiempo y un hueco libre. Pídela por teléfono, por WhatsApp o con el formulario.",
                       "Full washes, upholstery cleaning, polishing and waxing need an appointment, because they take time and a free bay. Book by phone, WhatsApp or the form.")],
                     eyebrow=("Cita", "Booking"),
                     imagen_=("photo", ("Coche recién lavado saliendo del túnel de lavado", "Freshly washed car leaving the wash tunnel")),
                     tono="relleno", anchor="cita", invertir=True),
        bloque_checks(("Bonos mensuales para el que lava a menudo", "Monthly passes for regular washers"), [
            ("Lavados de túnel o a mano a un precio más bajo que suelto.", "Tunnel or hand washes at a lower price than paying one by one."),
            ("Sin cita para los de túnel: llegas y entras.", "No appointment for tunnel washes: just turn up and drive in."),
            ("Se pagan al empezar el mes y se usan dentro del mes.", "Paid at the start of the month and used within the month."),
            ("Tienes las condiciones y los precios en Tarifas y bonos.", "Conditions and prices are on the Prices and passes page.")],
            eyebrow=("Bonos", "Passes"), cols=2),
        bloque_opiniones(("Lo que dicen nuestros clientes", "What our customers say"),
                         [("el lavado a mano", "the hand wash"), ("el interior", "the interior"), ("el trato", "the service")]),
        bloque_cta(("¿Cuándo te va bien?", "When suits you?"),
                   ("Llámanos o escríbenos y te decimos qué hueco hay hoy.", "Call or message us and we will tell you what slots are free today."),
                   [pedir(), llamar("secondary")], tono="base"),
    ])

    tarifas = pagina("tarifas", ("tarifas", "prices"), ("Tarifas y bonos", "Prices and passes"),
                     ("Precios de ejemplo del lavado a mano por tipo de vehículo, del túnel, de los extras con su duración, y bonos mensuales.",
                      "Example prices for hand washing by vehicle type, the tunnel, extras with their duration, and monthly passes."), [
        hero(("Tarifas y bonos", "Prices and passes"),
             ("El precio depende del tamaño del coche y de lo sucio que esté. Si está muy sucio o hay pelos de mascota, te lo decimos antes de empezar.",
              "The price depends on the size of the car and how dirty it is. If it is very dirty or has pet hair, we tell you before we start."),
             [pedir(), whatsapp("ghost")], variante="texto"),
        bloque_precios(("Lavado a mano", "Hand wash"), ("Precios del lavado a mano por vehículo y nivel", "Hand wash prices by vehicle and level"),
                       [("Vehículo", "Vehicle"), ("Exterior", "Exterior"), ("Exterior e interior", "Exterior and interior"),
                        ("Completo con cera", "Full with wax")],
                       [(("Utilitario", "Small car"), eur(14), eur(26), eur(45)),
                        (("Berlina o familiar", "Saloon or estate"), eur(17), eur(30), eur(52)),
                        (("SUV o monovolumen", "SUV or people carrier"), eur(20), eur(35), eur(60)),
                        (("Furgoneta", "Van"), eur(24), eur(40), eur(70))],
                       intro=("Exterior: lavado, llantas, cristales y secado. Interior: aspirado, salpicadero, plásticos y cristales por dentro. Completo: todo lo anterior y una capa de cera rápida. Duran unos 35 minutos, 1 hora y 1 hora y media.",
                              "Exterior: wash, wheels, windows and drying. Interior: vacuum, dashboard, plastics and inside windows. Full: all of that plus a quick coat of wax. They take about 35 minutes, 1 hour and 1.5 hours."),
                       nota=NOTA, anchor="a-mano"),
        bloque_precios(("Túnel de lavado", "Tunnel wash"), ("Programas del túnel", "Tunnel programmes"),
                       [("Programa", "Programme"), ("Qué hace", "What it does"), ("Dura", "Takes"), ("Precio", "Price")],
                       [(("Básico", "Basic"), ("Lavado y secado", "Wash and dry"), ("5 minutos", "5 minutes"), eur(6)),
                        (("Completo", "Full"), ("Prelavado, llantas, cera y secado", "Pre-wash, wheels, wax and drying"), ("8 minutos", "8 minutes"), eur(9)),
                        (("Brillo", "Shine"), ("Todo lo anterior y tratamiento de brillo", "All of the above and a shine treatment"), ("10 minutos", "10 minutes"), eur(12))],
                       intro=("Sin cita y solo exterior. El precio es para turismos; para SUV y furgonetas, pregúntanos.",
                              "No appointment and exterior only. The price is for cars; for SUVs and vans, ask us."),
                       nota=NOTA, anchor="tunel", tono="relleno"),
        bloque_precios(("Extras", "Extras"), ("Extras con su duración y si necesitan cita", "Extras with their duration and whether they need an appointment"),
                       [("Servicio", "Service"), ("Dura", "Takes"), ("Cita", "Appointment"), ("Precio", "Price")],
                       [(("Limpieza de tapicería (asientos y moqueta)", "Upholstery cleaning (seats and carpets)"), ("1 a 2 horas", "1 to 2 hours"), ("Con cita", "Booked"), desde(60)),
                        (("Tratamiento de asientos de cuero", "Leather seat treatment"), ("45 minutos", "45 minutes"), ("Con cita", "Booked"), desde(30)),
                        (("Eliminación de pelos de mascota", "Pet hair removal"), ("30 minutos", "30 minutes"), ("Sin cita", "No appointment"), desde(20)),
                        (("Tratamiento de olores con ozono", "Ozone odour treatment"), ("30 minutos", "30 minutes"), ("Con cita", "Booked"), desde(20)),
                        (("Pulido de faros", "Headlight polishing"), ("45 minutos", "45 minutes"), ("Con cita", "Booked"), desde(35)),
                        (("Encerado a mano", "Hand waxing"), ("1 hora", "1 hour"), ("Con cita", "Booked"), desde(40)),
                        (("Pulido de carrocería", "Paintwork polish"), ("3 a 5 horas", "3 to 5 hours"), ("Con cita", "Booked"), desde(120)),
                        (("Limpieza del vano motor", "Engine bay cleaning"), ("30 minutos", "30 minutes"), ("Sin cita", "No appointment"), desde(25))],
                       intro=("El pulido quita los micro-arañazos de la pintura, pero no los rayones profundos ni los golpes. Te decimos qué se puede mejorar al ver el coche.",
                              "Polishing removes fine swirl marks from the paint, but not deep scratches or dents. We tell you what can be improved once we see the car."),
                       nota=NOTA, anchor="extras"),
        bloque_precios(("Bonos mensuales", "Monthly passes"), ("Bonos mensuales de ejemplo", "Example monthly passes"),
                       [("Bono", "Pass"), ("Qué incluye", "What it includes"), ("Precio al mes", "Price per month")],
                       [(("Túnel básico", "Basic tunnel"), ("5 lavados del programa básico", "5 washes of the basic programme"), eur(22)),
                        (("Túnel completo", "Full tunnel"), ("4 lavados del programa completo", "4 washes of the full programme"), eur(28)),
                        (("A mano", "By hand"), ("2 lavados exteriores a mano de un turismo", "2 hand exterior washes for a car"), eur(26)),
                        (("Completo", "Complete"), ("1 lavado completo con cera y 2 de túnel básico", "1 full wash with wax and 2 basic tunnel washes"), eur(55))],
                       intro=("Se paga al empezar el mes y los lavados se usan dentro de ese mes. Son personales y van con una matrícula.",
                              "Paid at the start of the month, and the washes are used within that month. They are personal and tied to one registration plate."),
                       nota=("Precios y condiciones de ejemplo: cámbialos por los tuyos.", "Example prices and conditions: replace them with your own."),
                       anchor="bonos", tono="relleno"),
        bloque_faq(("Preguntas frecuentes", "Frequently asked questions"), [
            (("¿El túnel raya la pintura?", "Does the tunnel scratch the paint?"),
             ("Un túnel bien mantenido no debería marcar un coche en buen estado. Si tu pintura está muy dañada o quieres el máximo cuidado, el lavado a mano es la mejor opción.",
              "A well-maintained tunnel should not mark a car in good condition. If your paint is badly worn or you want the most care, the hand wash is the better choice.")),
            (("¿Cuánto tarda sin cita?", "How long does it take without an appointment?"),
             ("Entre cinco y diez minutos en el túnel y unos 35 minutos en un exterior a mano, más lo que haya de cola en hora punta.",
              "Five to ten minutes in the tunnel and about 35 minutes for a hand exterior wash, plus whatever queue there is at busy times.")),
            (("¿Qué pasa si llueve justo después?", "What if it rains right after?"),
             ("Un lavado no se repite por la lluvia, pero el encerado y el tratamiento de brillo ayudan a que el agua resbale y la suciedad se pegue menos.",
              "A wash is not repeated because of rain, but waxing and the shine treatment help water run off and dirt stick less.")),
            (("¿Puedo dejar el coche y recogerlo luego?", "Can I leave the car and collect it later?"),
             ("Sí, en los servicios con cita. Dinos a qué hora vuelves y lo dejamos listo.",
              "Yes, for booked services. Tell us when you will be back and we will have it ready.")),
            (("¿Cómo se paga?", "How do I pay?"),
             ("Al recoger el coche, en efectivo o con tarjeta. Los bonos se pagan al empezar el mes.",
              "When you collect the car, in cash or by card. Passes are paid at the start of the month.")),
        ], tono="base"),
        bloque_cta(("¿Qué lavado te hace falta?", "Which wash do you need?"), None, [pedir(), llamar("secondary")], tono="relleno"),
    ])

    cont = pagina("contacto", ("contacto", "contact"), ("Cita y contacto", "Booking and contact"),
                  ("Pide cita para el lavado a mano, la tapicería o el pulido por teléfono, WhatsApp o formulario, y mira el horario y cómo llegar.",
                   "Book a hand wash, upholstery cleaning or polishing by phone, WhatsApp or form, and see the opening hours and how to get here."), [
        hero(("Pide tu cita", "Book your slot"),
             ("Para el túnel no hace falta cita: pásate. Para lo demás, llámanos, escríbenos por WhatsApp o rellena el formulario y te confirmamos el día y la hora.",
              "The tunnel needs no appointment: just come by. For everything else, call us, message us on WhatsApp or fill in the form and we will confirm the day and time."),
             [llamar(), whatsapp()], variante="texto"),
        bloque_contacto(("Dónde estamos", "Where we are"),
                        ("Si tu coche es muy grande o lleva remolque, dínoslo al pedir cita.",
                         "If your vehicle is very large or has a trailer, tell us when you book."),
                        campos_cita(servicios=[("Lavado a mano", "Hand wash"), ("Lavado completo con cera", "Full wash with wax"),
                                               ("Tapicería", "Upholstery"), ("Pulido", "Polishing"), ("Encerado", "Waxing"),
                                               ("Bono mensual", "Monthly pass"), ("Otra cosa", "Something else")],
                                    extra=[campo("vehiculo", "select", ("Tipo de vehículo", "Vehicle type"), False,
                                                 opciones=[("Utilitario", "Small car"), ("Berlina o familiar", "Saloon or estate"),
                                                           ("SUV o monovolumen", "SUV or people carrier"), ("Furgoneta", "Van")]),
                                           campo("hora", "select", ("Franja que te viene bien", "Time of day that suits you"), False,
                                                 opciones=[("Mañana", "Morning"), ("Mediodía", "Midday"), ("Tarde", "Afternoon")])]),
                        ("Pedir cita", "Request a slot"), horas,
                        notas=("El último lavado a mano de cada día empieza una hora antes de cerrar.",
                               "The last hand wash of each day starts an hour before closing."),
                        exito=("Gracias. Te confirmamos la cita por teléfono o WhatsApp en cuanto la tengamos.",
                               "Thank you. We will confirm your appointment by phone or WhatsApp as soon as we have it.")),
        bloque_mapa(("Cómo llegar al lavadero", "How to get to the car wash"), tono="relleno"),
    ])

    return plantilla(KEY, ("Lavadero de coches", "Car wash"),
                     ("Lavadero de coches con tarifas por vehículo, túnel sin cita, extras con duración, bonos mensuales, horario y formulario de cita.",
                      "Car wash with prices by vehicle, a no-appointment tunnel, extras with durations, monthly passes, opening hours and a booking form."),
                     "motor", ("lavadero de coches lavadero autolavado tunel de lavado lavado a mano limpieza de coche limpieza de tapiceria pulido encerado lavacoches bono de lavados",
                               "car wash hand car wash tunnel wash car valeting car cleaning upholstery cleaning polishing waxing detailing wash pass"),
                     look, cabecera(nav, cta_cab),
                     pie(nav, ("Lavadero de coches a mano y en túnel. Con cita para lo largo, sin cita para lo rápido.",
                               "Hand and tunnel car wash. By appointment for the long jobs, no appointment for the quick ones."),
                         ("Lunes a viernes 9:00 a 20:00. Sábado 9:00 a 15:00.", "Monday to Friday 9 am to 8 pm. Saturday 9 am to 3 pm.")),
                     [inicio, tarifas, cont])
