# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
from lib import *
from looks import LOOKS

KEY = "taller-mecanico"

NOTA_PRECIOS = ("Precios de ejemplo: cámbialos por los tuyos. Incluyen IVA y mano de obra.",
                "Example prices: replace them with your own. VAT and labour included.")
CAB = [("Concepto", "Item"), ("Precio", "Price")]


def build():
    nueva(KEY)
    look = LOOKS["grafito"]
    nav = [("inicio", ("Inicio", "Home")), ("servicios", ("Servicios y precios", "Services and prices")),
           ("contacto", ("Cita previa", "Book a slot"))]
    horas = [(("Lunes a viernes", "Monday to Friday"),
              ("8:30 a 14:00 y 16:00 a 19:30", "8:30 am to 2:00 pm and 4:00 pm to 7:30 pm")),
             (("Sábado y domingo", "Saturday and Sunday"), ("Cerrado", "Closed"))]
    cta_cab = marco([llamar(t=("Llamar", "Call")), whatsapp("secondary", t=("WhatsApp", "WhatsApp"))],
                    dir="row", gap=8, wrap=True)
    pedir = lambda v="primary": boton(("Pedir cita", "Book a slot"), a_pagina("contacto", "contacto"), v)

    inicio = pagina("inicio", ("", ""), ("Taller mecánico", "Garage"),
                    ("Taller mecánico con cita previa: mantenimiento, frenos, neumáticos, ITV, diagnosis y climatización, con presupuesto por escrito.",
                     "Garage by appointment: servicing, brakes, tyres, ITV, diagnostics and air conditioning, with a written quote."), [
        hero(("Tu coche, en un taller que te explica el trabajo", "Your car, in a garage that explains the job"),
             ("Mantenimiento, frenos, neumáticos, diagnosis y climatización. Antes de tocar nada, te damos el presupuesto por escrito.",
              "Servicing, brakes, tyres, diagnostics and air conditioning. Before we touch anything, you get the quote in writing."),
             [pedir(), llamar("secondary"), whatsapp("ghost")],
             eyebrow=("Taller mecánico", "Garage"),
             imagen_=("landscape", ("Interior del taller con un coche elevado en el puente", "Inside the garage with a car raised on the lift"))),
        bloque_valores(("Qué hacemos", "What we do"), [
            (("Mantenimiento", "Servicing"),
             ("Aceite, filtros, correa, líquidos y revisión por kilómetros o por años, según el libro de mantenimiento de tu coche.",
              "Oil, filters, belt, fluids and servicing by mileage or by age, following your car's service book.")),
            (("Frenos", "Brakes"),
             ("Pastillas, discos y líquido. Te enseñamos la pieza gastada antes de cambiarla.",
              "Pads, discs and fluid. We show you the worn part before we replace it.")),
            (("Neumáticos", "Tyres"),
             ("Montaje, equilibrado y alineado. Te decimos cuándo toca cambiarlos y cuándo todavía aguantan.",
              "Fitting, balancing and alignment. We tell you when they need replacing and when they still have life left.")),
            (("ITV", "ITV test"),
             ("Revisión previa para llegar a la ITV sin sorpresas y arreglo de lo que te hayan marcado.",
              "A check before your ITV test so there are no surprises, and repairs for anything it flagged.")),
            (("Diagnosis", "Diagnostics"),
             ("Conectamos la máquina, leemos las averías y te explicamos qué significa cada luz del cuadro.",
              "We plug in the diagnostic tool, read the fault codes and explain what each warning light means.")),
            (("Climatización", "Air conditioning"),
             ("Recarga del aire acondicionado, búsqueda de fugas y cambio del filtro del habitáculo.",
              "Air conditioning re-gas, leak checks and cabin filter replacement.")),
        ], eyebrow=("Servicios", "Services"), estilo="borde"),
        bloque_pasos(("Cómo trabajamos", "How we work"), [
            (("Nos cuentas qué le pasa", "You tell us what is wrong"),
             ("Por teléfono, por WhatsApp o en el taller. Ruidos, luces, cuándo ocurre: cuanto más detalle, mejor.",
              "By phone, on WhatsApp or at the garage. Noises, warning lights, when it happens: the more detail, the better.")),
            (("Revisamos el coche", "We check the car"),
             ("Lo miramos con calma y, si hace falta, lo conectamos a la máquina de diagnosis.",
              "We look it over properly and, if needed, connect it to the diagnostic tool.")),
            (("Presupuesto por escrito", "A written quote"),
             ("Piezas, mano de obra e IVA, cada cosa por separado. No empezamos sin tu visto bueno.",
              "Parts, labour and VAT, each listed separately. We do not start without your go-ahead.")),
            (("Reparamos y te avisamos", "We repair it and call you"),
             ("Te llamamos cuando está listo. Te damos la factura y, si quieres, las piezas cambiadas.",
              "We call you when it is ready. You get the invoice and, if you want, the parts we replaced.")),
        ], tono="relleno"),
        bloque_checks(("Lo que te llevas", "What you get"), [
            ("Presupuesto previo y por escrito, sin compromiso.", "A written quote beforehand, with no obligation."),
            ("Garantía en piezas y mano de obra. El plazo va por escrito en la factura.",
             "A warranty on parts and labour. The period is written on the invoice."),
            ("Factura con el detalle de cada pieza y de las horas de trabajo.",
             "An invoice itemising each part and the hours of work."),
            ("Las piezas cambiadas, si nos las pides.", "The replaced parts, if you ask for them."),
            ("Coche de sustitución con reserva previa, según disponibilidad. Pídelo al reservar.",
             "A courtesy car with advance booking, subject to availability. Ask for it when you book."),
            ("Cita previa, para tener hueco y no hacerte esperar.", "Appointments, so there is a slot ready and you do not wait."),
        ], eyebrow=("Nuestro compromiso", "Our promise")),
        bloque_opiniones(("Lo que dicen nuestros clientes", "What our customers say"),
                         [("el presupuesto", "the quote"), ("el plazo de entrega", "the turnaround"), ("el trato", "the service")]),
        bloque_cta(("¿Qué le pasa a tu coche?", "What is wrong with your car?"),
                   ("Cuéntanoslo y te decimos qué hueco tenemos para verlo.",
                    "Tell us and we will find you a slot to look at it.")),
    ])

    serv = pagina("servicios", ("servicios", "services"), ("Servicios y precios", "Services and prices"),
                  ("Precios orientativos de revisiones, frenos, neumáticos, climatización y preparación de la ITV, y respuestas a las dudas más habituales.",
                   "Guide prices for servicing, brakes, tyres, air conditioning and ITV preparation, plus answers to common questions."), [
        hero(("Servicios y precios", "Services and prices"),
             ("Estos son nuestros precios orientativos. El precio final depende del modelo y de las piezas, y te lo damos por escrito antes de empezar.",
              "These are guide prices. The final price depends on the model and the parts, and you get it in writing before we start."),
             [pedir()], variante="texto"),
        bloque_precios(("Mantenimiento y revisiones", "Servicing and checks"), ("Precios de mantenimiento", "Servicing prices"), CAB,
                       [(("Revisión básica (aceite, filtro y 20 puntos de control)", "Basic service (oil, filter and 20-point check)"), desde(89)),
                        (("Revisión completa (con filtros de aire, polen y combustible)", "Full service (with air, pollen and fuel filters)"), desde(160)),
                        (("Cambio de correa de distribución", "Timing belt replacement"), desde(380)),
                        (("Cambio de batería", "Battery replacement"), desde(30)),
                        (("Revisión previa a la ITV", "Pre-ITV check"), desde(35))],
                       nota=NOTA_PRECIOS, anchor="mantenimiento"),
        bloque_precios(("Frenos, neumáticos y climatización", "Brakes, tyres and air conditioning"),
                       ("Precios de frenos, neumáticos y climatización", "Brake, tyre and air conditioning prices"), CAB,
                       [(("Pastillas de freno, un eje", "Brake pads, one axle"), desde(85)),
                        (("Discos y pastillas, un eje", "Discs and pads, one axle"), desde(220)),
                        (("Montaje y equilibrado de un neumático", "Fitting and balancing one tyre"), desde(15)),
                        (("Alineado de dirección", "Wheel alignment"), desde(45)),
                        (("Diagnosis con máquina", "Diagnostic scan"), desde(40)),
                        (("Recarga del aire acondicionado", "Air conditioning re-gas"), desde(70))],
                       nota=NOTA_PRECIOS, anchor="frenos", tono="relleno"),
        bloque_texto(("Antes de pasar la ITV", "Before the ITV test"), [
            ("Revisamos luces, frenos, neumáticos, escape, amortiguación y holguras de la dirección, que es donde suele haber fallos.",
             "We check lights, brakes, tyres, exhaust, suspension and steering play, which is where faults usually turn up."),
            ("Si hay algo que arreglar, te damos el presupuesto aparte y te decimos qué es urgente y qué puede esperar.",
             "If something needs fixing, we quote it separately and tell you what is urgent and what can wait.")],
            eyebrow=("ITV", "ITV test"), anchor="itv"),
        bloque_faq(("Preguntas frecuentes", "Frequently asked questions"), [
            (("¿Me cobráis por el presupuesto?", "Do you charge for a quote?"),
             ("Mirar el coche y presupuestarte una avería sencilla no tiene coste. Si hace falta diagnosis con máquina, te lo decimos antes y su precio se descuenta si nos haces la reparación.",
              "Looking at the car and quoting a simple fault is free. If it needs a diagnostic scan, we tell you first and its price is deducted if you have the repair done with us.")),
            (("¿Cuánto tarda una revisión?", "How long does a service take?"),
             ("Una revisión básica, entre una y dos horas. Si hay que pedir una pieza, te damos el plazo al hacer el presupuesto.",
              "A basic service takes one to two hours. If a part has to be ordered, we give you the timescale in the quote.")),
            (("¿Qué garantía tienen las reparaciones?", "What warranty do repairs have?"),
             ("Las piezas y la mano de obra llevan garantía, y su plazo va por escrito en la factura. Si algo falla, tráenos el coche y lo revisamos.",
              "Parts and labour are under warranty, and the period is written on the invoice. If something fails, bring the car back and we will look at it.")),
            (("¿Puedo traer mis propias piezas?", "Can I bring my own parts?"),
             ("Mejor que no: no podemos dar garantía de una pieza que no hemos comprado nosotros. Si tienes un caso especial, consúltanoslo.",
              "We would rather you did not: we cannot give a warranty on a part we did not supply. If you have a special case, ask us.")),
            (("¿Tenéis coche de sustitución?", "Do you have a courtesy car?"),
             ("Con reserva previa y según disponibilidad. Pídelo al reservar la cita.",
              "With advance booking and subject to availability. Ask for it when you book.")),
        ], tono="relleno"),
        bloque_cta(("¿Te buscamos hueco?", "Shall we find you a slot?"), None, [pedir(), llamar("secondary")], tono="base"),
    ])

    cont = pagina("contacto", ("contacto", "contact"), ("Cita previa", "Book a slot"),
                  ("Pide cita por teléfono, WhatsApp o con el formulario, y mira cómo llegar al taller.",
                   "Book a slot by phone, WhatsApp or with the form, and see how to get to the garage."), [
        hero(("Pide tu cita", "Book your slot"),
             ("Llámanos, escríbenos por WhatsApp o rellena el formulario. Te confirmamos el día y la hora.",
              "Call us, message us on WhatsApp or fill in the form. We confirm the day and time."),
             [llamar(), whatsapp()], variante="texto"),
        bloque_contacto(("Cómo encontrarnos", "How to find us"),
                        ("Si tu coche no arranca, dínoslo y vemos cómo traerlo.",
                         "If your car will not start, tell us and we will work out how to get it here."),
                        campos_cita(servicios=[("Mantenimiento", "Servicing"), ("Frenos", "Brakes"), ("Neumáticos", "Tyres"),
                                               ("ITV", "ITV test"), ("Diagnosis", "Diagnostics"),
                                               ("Climatización", "Air conditioning"), ("Otra avería", "Other fault")],
                                    extra=[campo("matricula", "text", ("Matrícula (opcional)", "Registration number (optional)"), False, maxlen=12),
                                           campo("sustitucion", "select", ("¿Necesitas coche de sustitución?", "Do you need a courtesy car?"), False,
                                                 opciones=[("No", "No"), ("Sí, si hay disponible", "Yes, if one is available")])]),
                        ("Pedir cita", "Request a slot"), horas,
                        notas=("Los datos de tu coche solo los usamos para preparar la cita.",
                               "We only use your car details to prepare the appointment."),
                        exito=("Gracias. Te confirmamos la cita por teléfono o WhatsApp en cuanto la tengamos.",
                               "Thank you. We will confirm your appointment by phone or WhatsApp as soon as we have it.")),
        bloque_mapa(tono="relleno"),
    ])

    return plantilla(KEY, ("Taller mecánico", "Garage"),
                     ("Mantenimiento, frenos, neumáticos, ITV y diagnosis con presupuesto por escrito, precios orientativos y cita previa por teléfono o WhatsApp.",
                      "Servicing, brakes, tyres, ITV and diagnostics with a written quote, guide prices and booking by phone or WhatsApp."),
                     "motor", ("taller mecanico taller de coches mecanico reparacion de coches frenos aceite neumaticos itv diagnosis aire acondicionado revision cambio de aceite",
                               "garage car garage mechanic car repair brakes oil change tyres mot itv diagnostics air conditioning servicing"),
                     look, cabecera(nav, cta_cab),
                     pie(nav, ("Taller mecánico con cita previa. Presupuesto por escrito y garantía en cada reparación.",
                               "Garage by appointment. Written quote and a warranty on every repair."),
                         ("Lunes a viernes 8:30 a 14:00 y 16:00 a 19:30.", "Monday to Friday 8:30 am to 2 pm and 4 pm to 7:30 pm.")),
                     [inicio, serv, cont])
