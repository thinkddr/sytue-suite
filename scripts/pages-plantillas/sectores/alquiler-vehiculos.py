# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
from lib import *

KEY = "alquiler-vehiculos"

# Gris azulado frío con naranja de furgoneta. Se comprueba AA al construir.
RUTA = dict(background="#f3f5f8", surface="#e9edf1", text="#14202b", muted="#4b5967", primary="#b23a09",
            on_primary="#ffffff", border="#d3dae1", inverse="#1b2733", on_inverse="#eef2f6")

NOTA_TARIFAS = ("Precios de ejemplo: cámbialos por los tuyos. IVA incluido. Kilómetros, fianza y seguro, según las condiciones.",
                "Example prices: replace them with your own. VAT included. Mileage, deposit and insurance as per the terms.")


def build():
    malos = comprobar_paleta("ruta", RUTA)
    assert not malos, malos
    nueva(KEY)
    look = dict(colores=RUTA, fuentes=("figtree", "ibm-plex-sans"), peso=800, radio=(4, 8, 12), ancho=1160)
    nav = [("inicio", ("Inicio", "Home")), ("flota", ("Flota y tarifas", "Fleet and rates")),
           ("condiciones", ("Condiciones", "Terms")), ("reservar", ("Reservar", "Book"))]
    horas = [(("Lunes a viernes", "Monday to Friday"), ("8:30 a 13:30 y 16:00 a 20:00", "8:30 am to 1:30 pm and 4:00 pm to 8:00 pm")),
             (("Sábado", "Saturday"), ("9:00 a 14:00", "9:00 am to 2:00 pm")),
             (("Domingo", "Sunday"), ("Cerrado", "Closed"))]
    cta_cab = marco([llamar(t=("Llamar", "Call")), whatsapp("secondary", t=("WhatsApp", "WhatsApp"))],
                    dir="row", gap=8, wrap=True)
    reservar = lambda v="primary": boton(("Reservar fechas", "Book dates"), a_pagina("reservar", "reserva"), v)

    inicio = pagina("inicio", ("", ""), ("Alquiler de vehículos", "Vehicle hire"),
                    ("Alquiler de coches, furgonetas y vehículos para mudanzas y eventos con tarifa por día, requisitos, fianza y seguro explicados y reserva por fechas.",
                     "Hire of cars, vans and vehicles for removals and events with a daily rate, requirements, deposit and insurance explained and booking by dates."), [
        hero(("El vehículo que necesitas, los días que lo necesitas", "The vehicle you need, for the days you need it"),
             ("Coches para moverte, furgonetas para una mudanza y vehículos para un evento. Eliges fechas, te confirmamos disponibilidad y recoges las llaves.",
              "Cars to get around, vans for a move and vehicles for an event. You pick dates, we confirm availability and you collect the keys."),
             [reservar(), llamar("secondary"), whatsapp("ghost")], variante="texto", tono="relleno",
             eyebrow=("Alquiler de vehículos", "Vehicle hire")),
        bloque_valores(("Qué puedes alquilar", "What you can hire"), [
            (("Coches", "Cars"),
             ("Para el día a día, un viaje o cuando el tuyo está en el taller. De cinco plazas o más, según la flota.",
              "For everyday use, a trip or while yours is at the garage. Five seats or more, depending on the fleet.")),
            (("Furgonetas de carga", "Cargo vans"),
             ("Para llevar muebles, mercancía o herramientas, por días o por horas si lo necesitas.",
              "To carry furniture, goods or tools, by the day or by the hour if you need it.")),
            (("Para mudanzas", "For moving"),
             ("Furgones más grandes, con rampa o con remolque, para mudarte tú sin contratar una mudanza.",
              "Larger vans, with a ramp or a trailer, so you can move yourself without hiring removers.")),
            (("Para eventos", "For events"),
             ("Furgonetas de pasajeros y de carga para ferias, bodas o conciertos, con la salida y la vuelta a la hora que te encaje.",
              "Passenger and cargo vans for fairs, weddings or concerts, with pick-up and return at the time that suits you.")),
        ], eyebrow=("Flota", "Fleet"), cols=4, estilo="borde", anchor="tipos"),
        bloque_pasos(("Cómo se reserva", "How to book"), [
            (("Eliges fechas y vehículo", "You pick dates and vehicle"),
             ("Con el formulario, por teléfono o por WhatsApp. Dinos qué vas a llevar y cuántos sois.",
              "With the form, by phone or on WhatsApp. Tell us what you will carry and how many of you there are.")),
            (("Te confirmamos", "We confirm"),
             ("Te decimos si hay vehículo libre, el precio total y lo que debes traer.",
              "We tell you whether a vehicle is free, the total price and what you need to bring.")),
            (("Recoges las llaves", "You collect the keys"),
             ("Con tu carné y tu documento. Revisamos el vehículo juntos y firmamos el contrato.",
              "With your licence and ID. We check the vehicle together and sign the contract.")),
            (("Lo devuelves", "You return it"),
             ("A la hora acordada y en las condiciones que lo recogiste. Lo repasamos juntos y cerramos el contrato.",
              "At the agreed time and in the condition you collected it. We go over it together and close the contract.")),
        ], tono="relleno", anchor="como-reservar"),
        bloque_checks(("Lo que te dejamos claro antes de firmar", "What we make clear before you sign"), [
            ("La tarifa total de tu alquiler, por escrito y con los días contados.", "The total price of your hire, in writing and with the days counted."),
            ("Los kilómetros incluidos y lo que cuesta pasarse.", "The mileage included and what going over costs."),
            ("La fianza que se retiene y cuándo se devuelve.", "The deposit held and when it is returned."),
            ("Qué cubre el seguro y qué parte pagarías tú en caso de daño.", "What the insurance covers and what part you would pay in case of damage."),
            ("Hora y lugar de recogida y devolución.", "Time and place of collection and return."),
        ], eyebrow=("Sin sorpresas", "No surprises"), tono="base", anchor="claro"),
        bloque_opiniones(("Lo que dicen nuestros clientes", "What our customers say"),
                         [("la reserva", "the booking"), ("el estado del vehículo", "the vehicle condition"), ("la devolución", "the return")]),
        bloque_cta(("¿Qué fechas tienes en mente?", "Which dates do you have in mind?"),
                   ("Dinos cuándo y qué vas a llevar, y te decimos qué vehículo te encaja.",
                    "Tell us when and what you will carry, and we will tell you which vehicle suits you."),
                   [reservar(), llamar("secondary")], tono="relleno"),
    ])

    flota = pagina("flota", ("flota-y-tarifas", "fleet-and-rates"), ("Flota y tarifas", "Fleet and rates"),
                   ("Tabla de vehículos por tipo con tarifa por día y una nota, y consejos para elegir el que te conviene.",
                    "Table of vehicles by type with a daily rate and a note, and tips for choosing the right one."), [
        hero(("Flota y tarifas", "Fleet and rates"),
             ("Estas son nuestras tarifas por día. El precio total depende de cuántos días lo lleves y de la temporada, y te lo damos por escrito al reservar.",
              "These are our daily rates. The total price depends on how many days you keep it and the season, and you get it in writing when you book."),
             [reservar(), llamar("secondary")], variante="texto"),
        bloque_precios(("Vehículos y tarifa por día", "Vehicles and daily rate"), ("Flota por tipo con tarifa por día", "Fleet by type with daily rate"),
                       [("Vehículo", "Vehicle"), ("Ideal para", "Best for"), ("Tarifa por día", "Daily rate"), ("Nota", "Note")],
                       [(("Coche urbano", "City car"), ("Ciudad y trayectos cortos", "Town and short trips"), desde(30),
                         ("Pregunta por disponibilidad", "Ask about availability")),
                        (("Coche familiar", "Family car"), ("Viajes con equipaje", "Trips with luggage"), desde(45),
                         ("Mejor reservar con antelación en vacaciones", "Best booked ahead in holiday periods")),
                        (("Monovolumen de 7 plazas", "7-seat people carrier"), ("Familias y grupos", "Families and groups"), desde(70),
                         ("Consulta el equipaje que cabe", "Ask how much luggage fits")),
                        (("Furgoneta de carga pequeña", "Small cargo van"), ("Compras grandes y pequeños traslados", "Large purchases and small moves"), desde(45),
                         ("Se puede alquilar por horas, según disponibilidad", "Can be hired by the hour, subject to availability")),
                        (("Furgón grande de mudanza", "Large removal van"), ("Mudar un piso", "Moving a flat"), desde(80),
                         ("Pídelo con mantas y correas si las necesitas", "Ask for blankets and straps if you need them")),
                        (("Furgoneta de 9 plazas", "9-seat van"), ("Eventos y grupos", "Events and groups"), desde(90),
                         ("Comprueba el permiso que exige", "Check the licence it requires")),
                        (("Remolque", "Trailer"), ("Moto, jardín o carga ligera", "Motorbike, garden or light loads"), desde(20),
                         ("Tu coche debe poder arrastrarlo", "Your car must be able to tow it"))],
                       nota=NOTA_TARIFAS, anchor="tarifas"),
        bloque_valores(("Cómo elegir", "How to choose"), [
            (("Para una mudanza", "For a move"),
             ("Cuenta cuántas cajas y muebles grandes hay. Es mejor un vehículo algo más grande que dos viajes.",
              "Count how many boxes and large pieces of furniture there are. A slightly bigger vehicle beats two trips.")),
            (("Para un viaje", "For a trip"),
             ("Cuenta personas y maletas, y los kilómetros que harás: miramos si te salen bien las condiciones.",
              "Count people and suitcases, and the kilometres you will drive: we check the terms work out for you.")),
            (("Para un evento", "For an event"),
             ("Dinos cuántas personas, qué carga y a qué hora entras: te recomendamos el vehículo y los tiempos.",
              "Tell us how many people, what load and what time you arrive: we recommend the vehicle and the timings.")),
        ], eyebrow=("Consejos", "Tips"), cols=3, estilo="relleno", tono="relleno", anchor="elegir"),
        bloque_cta(("¿Dudas entre dos vehículos?", "Torn between two vehicles?"), None, [reservar(), whatsapp("secondary")], tono="base"),
    ])

    cond = pagina("condiciones", ("condiciones", "terms"), ("Requisitos y condiciones", "Requirements and terms"),
                  ("Requisitos del conductor, fianza, seguro, kilometraje, recogida y devolución del vehículo y preguntas frecuentes.",
                   "Driver requirements, deposit, insurance, mileage, vehicle collection and return, and frequently asked questions."), [
        hero(("Requisitos y condiciones", "Requirements and terms"),
             ("Lo que tienes que traer, cuánto se retiene de fianza, qué cubre el seguro y cómo se recoge y se devuelve. El contrato manda; aquí va el resumen.",
              "What to bring, how much deposit is held, what the insurance covers and how collection and return work. The contract prevails; this is the summary."),
             [reservar(), llamar("secondary")], variante="texto"),
        bloque_checks(("Requisitos del conductor", "Driver requirements"), [
            ("Carné de conducir en vigor y válido para el vehículo que alquilas.", "A valid driving licence for the vehicle you are hiring."),
            ("DNI, NIE o pasaporte, el original.", "ID card, NIE or passport, the original."),
            ("Una tarjeta a nombre del conductor principal para la fianza.", "A card in the main driver's name for the deposit."),
            ("Un teléfono de contacto y un correo, para el contrato y para avisarte.", "A contact phone and an email, for the contract and to reach you."),
            ("Texto de ejemplo: escribe aquí la edad mínima y la antigüedad del carné que exiges, y si hay recargo para conductores jóvenes.",
             "Sample text: write here the minimum age and years of licence you require, and whether young drivers pay a surcharge."),
        ], intro=("Si vais a conducir varias personas, todas tienen que figurar en el contrato y traer su carné.",
                  "If several people will drive, all of them must be named in the contract and bring their licence."),
            eyebrow=("Conductor", "Driver"), anchor="requisitos"),
        bloque_texto(("Fianza", "Deposit"), [
            ("Texto de ejemplo: indica el importe de la fianza de cada tipo de vehículo, cómo se retiene (tarjeta o efectivo) y en cuántos días se devuelve.",
             "Sample text: state the deposit for each vehicle type, how it is held (card or cash) and how many days it takes to be returned."),
            ("La fianza no es el precio del alquiler: se devuelve al entregar el vehículo en las condiciones del contrato, descontando, si los hay, daños o gastos pendientes.",
             "The deposit is not the hire price: it is returned when you hand the vehicle back in the condition set by the contract, less any damage or outstanding costs.")],
            eyebrow=("Fianza", "Deposit"), tono="relleno", anchor="fianza"),
        bloque_texto(("Seguro", "Insurance"), [
            ("Todo vehículo que alquilamos tiene el seguro obligatorio. Lo que cubre, la franquicia (la parte que pagas tú) y lo que queda fuera lo pone el contrato, y te lo explicamos antes de firmar.",
             "Every vehicle we hire out has the compulsory insurance. What it covers, the excess (the part you pay) and what is excluded is in the contract, and we explain it before you sign."),
            ("Texto de ejemplo: describe aquí los seguros opcionales que ofreces y qué cubren, o borra este párrafo si no ofreces ninguno.",
             "Sample text: describe here the optional insurance you offer and what it covers, or delete this paragraph if you offer none.")],
            eyebrow=("Seguro", "Insurance"), anchor="seguro"),
        bloque_texto(("Kilometraje", "Mileage"), [
            ("Texto de ejemplo: cuenta cuántos kilómetros incluye cada alquiler (por día o totales), cuánto cuesta cada kilómetro de más o si es ilimitado.",
             "Sample text: explain how many kilometres each hire includes (per day or in total), what each extra kilometre costs or whether it is unlimited."),
            ("Si vas a hacer un viaje largo, dínoslo al reservar y calculamos cuál es la mejor opción.",
             "If you are planning a long trip, tell us when you book and we work out the best option.")],
            eyebrow=("Kilómetros", "Kilometres"), tono="relleno", anchor="kilometraje"),
        bloque_pasos(("Recogida y devolución", "Collection and return"), [
            (("Al recogerlo", "On collection"),
             ("Comprobamos tu carné y tu documento, firmamos el contrato y te explicamos cómo funciona el vehículo.",
              "We check your licence and ID, sign the contract and explain how the vehicle works.")),
            (("Revisamos juntos", "We check it together"),
             ("Damos una vuelta al vehículo, anotamos cualquier golpe o raya que ya tenga y hacemos fotos.",
              "We walk around the vehicle, note any knock or scratch it already has and take photos.")),
            (("Al devolverlo", "On return"),
             ("En la hora y el lugar acordados. Lo repasamos de nuevo juntos y lo comparamos con las fotos de salida.",
              "At the agreed time and place. We go over it again together and compare it with the departure photos.")),
            (("Si te retrasas", "If you are late"),
             ("Avísanos cuanto antes. Si el vehículo está libre, ampliamos el alquiler; si no, hablamos de la mejor solución.",
              "Tell us as soon as you can. If the vehicle is free, we extend the hire; if not, we find the best solution together.")),
        ], tono="base", anchor="recogida"),
        bloque_faq(("Preguntas frecuentes", "Frequently asked questions"), [
            (("¿Puedo salir de España con el vehículo?", "Can I take the vehicle out of Spain?"),
             ("Pregúntanoslo antes de reservar: depende del seguro y del contrato.", "Ask us before booking: it depends on the insurance and the contract.")),
            (("¿Puede conducir otra persona?", "Can someone else drive?"),
             ("Sí, si figura en el contrato y trae su carné. Consulta si tiene coste.", "Yes, if named in the contract and bringing their licence. Ask whether there is a charge.")),
            (("¿Qué hago si hay una avería o un accidente?", "What do I do if there is a breakdown or an accident?"),
             ("Llámanos al teléfono que figura en el contrato. Si hay heridos, llama primero al 112.",
              "Call us on the number in the contract. If anyone is hurt, call 112 first.")),
            (("¿Puedo cancelar la reserva?", "Can I cancel the booking?"),
             ("Texto de ejemplo: explica aquí tu política de cancelación y si se devuelve algún importe.",
              "Sample text: explain here your cancellation policy and whether anything is refunded.")),
        ], tono="relleno"),
    ])

    campos_reserva = [
        campo("nombre", "text", ("Nombre", "Name"), True, auto="name", maxlen=120),
        campo("telefono", "tel", ("Teléfono", "Phone"), True, auto="tel", maxlen=30),
        campo("correo", "email", ("Correo electrónico", "Email"), False, auto="email", maxlen=200),
        campo("vehiculo", "select", ("Vehículo", "Vehicle"), True,
              opciones=[("Coche urbano", "City car"), ("Coche familiar", "Family car"), ("Monovolumen de 7 plazas", "7-seat people carrier"),
                        ("Furgoneta de carga pequeña", "Small cargo van"), ("Furgón grande de mudanza", "Large removal van"),
                        ("Furgoneta de 9 plazas", "9-seat van"), ("Remolque", "Trailer")]),
        campo("recogida", "date", ("Día de recogida", "Collection date"), True),
        campo("hora", "time", ("Hora de recogida", "Collection time")),
        campo("devolucion", "date", ("Día de devolución", "Return date"), True),
        campo("uso", "select", ("¿Para qué lo necesitas?", "What do you need it for?"), False,
              opciones=[("Viaje o día a día", "Trip or everyday use"), ("Mudanza", "Move"), ("Evento", "Event"), ("Otro", "Other")]),
        campo("mensaje", "textarea", ("Qué vas a llevar o cuántos sois (opcional)", "What you will carry or how many of you (optional)"), False, maxlen=1000),
    ]
    reserva = pagina("reservar", ("reservar", "book"), ("Reservar", "Book"),
                     ("Reserva un vehículo con las fechas de recogida y devolución por formulario, teléfono o WhatsApp. Horario y cómo llegar.",
                      "Book a vehicle with collection and return dates by form, phone or WhatsApp. Opening hours and how to find us."), [
        hero(("Reserva tus fechas", "Book your dates"),
             ("Dinos qué vehículo y qué días. Te confirmamos disponibilidad y el precio total antes de que pagues nada.",
              "Tell us which vehicle and which days. We confirm availability and the total price before you pay anything."),
             [llamar(), whatsapp()], variante="texto"),
        bloque_contacto(("Fechas y vehículo", "Dates and vehicle"),
                        ("La reserva queda confirmada cuando te contestamos. Si cambian tus fechas, avísanos.",
                         "The booking is confirmed once we reply. If your dates change, let us know."),
                        campos_reserva, ("Pedir reserva", "Request booking"), horas, anchor="reserva",
                        notas=("Usamos estos datos solo para preparar tu reserva.", "We only use these details to prepare your booking."),
                        exito=("Gracias. Te confirmamos disponibilidad y precio por teléfono o correo en cuanto lo comprobemos.",
                               "Thank you. We will confirm availability and price by phone or email as soon as we have checked.")),
        bloque_mapa(("Dónde recoger y devolver", "Where to collect and return"),
                    ("Aquí recoges y devuelves el vehículo. Si necesitas otro punto, pregúntanos.",
                     "This is where you collect and return the vehicle. If you need another spot, ask us."), tono="relleno"),
    ])

    return plantilla(KEY, ("Alquiler de vehículos", "Vehicle hire"),
                     ("Alquiler de coches, furgonetas y vehículos para mudanzas y eventos con flota por tipo y tarifa por día, requisitos, fianza, seguro, kilometraje y reserva por fechas.",
                      "Hire of cars, vans and vehicles for removals and events with a fleet by type and daily rate, requirements, deposit, insurance, mileage and booking by dates."),
                     "motor", ("alquiler de coches alquiler de furgonetas alquiler de vehiculos rent a car alquilar coche alquilar furgoneta furgoneta de mudanza alquiler para mudanzas alquiler por dias fianza reserva",
                               "car hire van hire vehicle hire rent a car rental removal van hire hire for events daily rental deposit booking"),
                     look, cabecera(nav, cta_cab),
                     pie(nav, ("Alquiler de coches, furgonetas y vehículos para mudanzas y eventos, con tarifa por día y contrato claro.",
                               "Hire of cars, vans and vehicles for removals and events, with a daily rate and a clear contract."),
                         ("Lunes a viernes 8:30 a 13:30 y 16:00 a 20:00. Sábado 9:00 a 14:00.",
                          "Monday to Friday 8:30 am to 1:30 pm and 4 pm to 8 pm. Saturday 9 am to 2 pm.")),
                     [inicio, flota, cond, reserva])
