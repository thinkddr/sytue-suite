# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
from lib import *
from looks import LOOKS  # noqa: F401  (la paleta es propia de este sector)

KEY = "alojamiento"

# Como COLEGIADO: el 00-0000 lo detecta domain.TextosDemo y el sitio no se publica hasta cambiarlo.
REGISTRO = ("Nº de registro turístico: 00-0000", "Tourist register no.: 00-0000")

# Paleta propia (verde pino sobre papel): `casa-rural` usa madera y piedra, `apartamentos-turisticos`
# arena y azul. Sin fondo oscuro (primary/inverse < 3).
PINO = dict(
    colores=dict(background="#fafaf7", surface="#ecebe4", text="#17191f", muted="#4f525c", primary="#1f5a52",
                 on_primary="#ffffff", border="#dcdbd1", inverse="#14201f", on_inverse="#f0f4f2"),
    fuentes=("dm-sans", "noto-sans"), radio=(2, 4, 8), ancho=1160)

NOTA = ("Precios de ejemplo: cámbialos por los tuyos. IVA incluido; la tasa turística, si existe, se paga aparte.",
        "Example prices: replace them with your own. VAT included; any tourist tax is paid separately.")
CONDICIONES = ("Condiciones de ejemplo: cámbialas por las tuyas.", "Example terms: replace them with your own.")

HUESPEDES = [("1 huésped", "1 guest"), ("2 huéspedes", "2 guests"), ("3 huéspedes", "3 guests"),
             ("4 huéspedes", "4 guests"), ("Más de 4", "More than 4")]


def portada(titulo, entradilla, botones, eyebrow, campos, envio, exito):
    """Portada propia: título a la izquierda y, a la derecha, el formulario de reserva directa."""
    texto_ = marco([etiqueta(eyebrow), h1(titulo), p(entradilla, color="$muted"),
                    marco(botones, dir="row", gap=12, wrap=True, align="center"),
                    peq(REGISTRO, color="$muted")],
                   gap=18, sz={"width": "fill", "height": "hug", "min_width": 0})
    ficha = tarjeta([h2(("Reserva directa", "Book direct")),
                     peq(("Dinos las fechas y te respondemos con habitaciones libres y precio.",
                          "Tell us the dates and we reply with free rooms and a price."), color="$muted"),
                     formulario(campos, envio, exito)],
                    estilo="relleno", tag="div", gap=14, pad=28, sz={"width": "fill", "height": "hug", "min_width": 0})
    fila = marco([texto_, ficha], dir="row", gap=56, align="center",
                 bp={"tablet": {"layout": {"gap": 32}}, "mobile": {"layout": {"direction": "column", "gap": 28}}})
    return seccion("hero", [fila], pad_v=80, gap=0)


def habitacion(nombre, capacidad, texto_, alt):
    return marco([imagen("photo", alt, ratio=1.3), h3(nombre), peq(capacidad, color="$muted"), p(texto_)],
                 tag="li", gap=10)


def build():
    nueva(KEY)
    look = PINO
    nav = [("inicio", ("Inicio", "Home")), ("habitaciones", ("Habitaciones y tarifas", "Rooms and rates")),
           ("servicios", ("Servicios y zona", "Services and area")), ("contacto", ("Reservas", "Bookings"))]
    horas = [(("Recepción", "Reception"), ("Las 24 horas, todos los días", "24 hours, every day")),
             (("Entrada (check-in)", "Check-in"), ("A partir de las 14:00", "From 2:00 pm")),
             (("Salida (check-out)", "Check-out"), ("Hasta las 11:00", "Until 11:00 am")),
             (("Desayuno", "Breakfast"), ("De 7:30 a 10:30", "7:30 am to 10:30 am"))]
    cta_cab = marco([llamar(t=("Llamar", "Call")), whatsapp("secondary", t=("WhatsApp", "WhatsApp"))],
                    dir="row", gap=8, wrap=True)
    reservar = lambda v="primary": boton(("Reservar", "Book"), a_pagina("contacto", "reservar"), v)

    def campos_rapidos():
        return [campo("llegada", "date", ("Llegada", "Arrival"), True),
                campo("salida", "date", ("Salida", "Departure"), True),
                campo("huespedes", "select", ("Huéspedes", "Guests"), True, opciones=HUESPEDES),
                campo("nombre", "text", ("Nombre", "Name"), True, auto="name", maxlen=120),
                campo("correo", "email", ("Correo electrónico", "Email"), True, auto="email", maxlen=200)]

    inicio = pagina("inicio", ("", ""), ("Hotel pequeño", "Small hotel"),
                    ("Hotel o hostal urbano con habitaciones por tipo, tarifas de ejemplo, servicios, qué ver cerca y reserva directa por fechas y huéspedes.",
                     "Urban small hotel or guesthouse with rooms by type, example rates, services, what to see nearby and direct booking by dates and guests."), [
        portada(("Una habitación tranquila en el centro", "A quiet room in the city centre"),
                ("Habitaciones sencillas y limpias, recepción siempre abierta y todo lo que quieres ver a pie. Reserva directamente con nosotros.",
                 "Simple, clean rooms, a reception that is always open and everything you want to see on foot. Book directly with us."),
                [llamar(), whatsapp("secondary")],
                ("Hotel y hostal urbano", "Urban hotel and guesthouse"),
                campos_rapidos(), ("Consultar disponibilidad", "Check availability"),
                ("Gracias. Te respondemos con las habitaciones libres y el precio en cuanto lo veamos.",
                 "Thank you. We will reply with the free rooms and the price as soon as we see it.")),
        bloque_valores(("Lo que encontrarás", "What you will find"), [
            (("Habitaciones por tipo", "Rooms by type"),
             ("Individuales, dobles y familiares, cada una con baño propio. Te decimos cuántas personas caben antes de que reserves.",
              "Singles, doubles and family rooms, each with its own bathroom. We tell you how many people fit before you book.")),
            (("Recepción abierta", "Open reception"),
             ("Alguien te recibe a cualquier hora, guarda tu equipaje y te dice por dónde ir.",
              "Someone welcomes you at any hour, keeps your luggage and tells you where to go.")),
            (("Desayuno", "Breakfast"),
             ("Café, pan, fruta y algo salado, de 7:30 a 10:30. Si sales antes, pídenos un desayuno para llevar.",
              "Coffee, bread, fruit and something savoury, from 7:30 to 10:30. If you leave earlier, ask for a breakfast to go.")),
        ], estilo="linea", eyebrow=("El alojamiento", "The accommodation")),
        seccion("features", [
            titular(("Habitaciones", "Rooms"),
                    ("Mira todos los tipos, con capacidad y tarifa, en la página de habitaciones.",
                     "See every type, with capacity and rate, on the rooms page.")),
            rejilla([
                habitacion(("Individual", "Single"), ("1 persona", "1 guest"),
                           ("Cama de 90 y escritorio, para viajes de trabajo o de una noche.",
                            "A 90 cm bed and a desk, for work trips or a night's stay."),
                           ("Habitación individual con escritorio y ventana", "Single room with a desk and a window")),
                habitacion(("Doble", "Double"), ("Hasta 2 personas", "Up to 2 guests"),
                           ("Cama de matrimonio o dos camas, con baño y espacio para las maletas.",
                            "Double bed or two single beds, with a bathroom and room for suitcases."),
                           ("Habitación doble con cama de matrimonio y mesillas", "Double room with a double bed and bedside tables")),
                habitacion(("Familiar", "Family"), ("Hasta 4 personas", "Up to 4 guests"),
                           ("Dos ambientes, con cama de matrimonio y dos camas más.",
                            "Two spaces, with a double bed and two more beds."),
                           ("Habitación familiar con una cama de matrimonio y dos camas individuales", "Family room with a double bed and two single beds")),
            ], cols=3),
            boton(("Ver habitaciones y tarifas", "See rooms and rates"), a_pagina("habitaciones"), "secondary"),
        ], tono="relleno", gap=32, anchor="resumen-habitaciones"),
        bloque_opiniones(("Lo que cuentan los huéspedes", "What guests say"),
                         [("la habitación", "the room"), ("la ubicación", "the location"), ("la recepción", "the reception")],
                         tono="base"),
        bloque_cta(("¿Qué fechas tienes pensadas?", "Which dates do you have in mind?"),
                   ("Dinos cuándo llegas, cuándo te vas y cuántos sois y te respondemos.",
                    "Tell us when you arrive, when you leave and how many of you there are and we will reply."),
                   [reservar(), llamar("secondary"), whatsapp("ghost")], tono="relleno"),
    ])

    habs = pagina("habitaciones", ("habitaciones", "rooms"), ("Habitaciones y tarifas", "Rooms and rates"),
                  ("Tabla de habitaciones con capacidad y tarifa por noche de ejemplo, equipamiento común, fotos y desayuno.",
                   "Table of rooms with capacity and example nightly rate, shared equipment, photos and breakfast."), [
        hero(("Habitaciones y tarifas", "Rooms and rates"),
             ("Cuatro tipos de habitación. El precio es por noche y por habitación, con desayuno si lo marcas en la reserva.",
              "Four room types. The price is per night and per room, with breakfast if you add it to the booking."),
             [reservar(), llamar("secondary"), whatsapp("ghost")], variante="texto", tono="relleno"),
        bloque_precios(("Tipos y tarifas", "Types and rates"), ("Habitaciones con capacidad y tarifa por noche", "Rooms with capacity and nightly rate"),
                       [("Habitación", "Room"), ("Capacidad", "Capacity"), ("Camas", "Beds"), ("Tarifa por noche", "Rate per night")],
                       [(("Individual", "Single"), ("1 persona", "1 guest"), ("1 cama de 90", "1 single bed"), desde(55)),
                        (("Doble", "Double"), ("2 personas", "2 guests"), ("1 cama de 150", "1 double bed"), desde(75)),
                        (("Doble con dos camas", "Twin"), ("2 personas", "2 guests"), ("2 camas de 90", "2 single beds"), desde(75)),
                        (("Familiar", "Family"), ("Hasta 4 personas", "Up to 4 guests"), ("1 cama de 150 y 2 de 90", "1 double and 2 single beds"), desde(120))],
                       intro=("La tarifa cambia con la temporada y los fines de semana. Los precios de abajo son de ejemplo: cámbialos por los tuyos.",
                              "The rate changes with the season and at weekends. The prices below are examples: replace them with your own."),
                       nota=NOTA, anchor="tarifas", tono="base"),
        bloque_checks(("En todas las habitaciones", "In every room"), [
            ("Baño privado con ducha y toallas.", "Private bathroom with shower and towels."),
            ("Aire acondicionado y calefacción.", "Air conditioning and heating."),
            ("Wifi gratuito.", "Free wifi."),
            ("Televisión y caja fuerte.", "Television and safe."),
            ("Escritorio o mesa de trabajo.", "Desk or work table."),
            ("Ropa de cama y limpieza diaria.", "Bed linen and daily cleaning."),
        ], intro=("Lista de ejemplo: cámbiala por lo que tienen de verdad tus habitaciones.",
                  "Example list: replace it with what your rooms really have."),
            eyebrow=("Incluido", "Included"), tono="relleno", anchor="equipamiento"),
        bloque_galeria(("Las habitaciones", "The rooms"), [
            ("Habitación doble con cama de matrimonio", "Double room with a double bed"),
            ("Habitación individual con escritorio", "Single room with a desk"),
            ("Habitación familiar con tres camas", "Family room with three beds"),
            ("Baño con ducha y toallas", "Bathroom with a shower and towels")], cols=2, ratio=1.4, tono="base"),
        bloque_cta(("¿Te guardamos una habitación?", "Shall we keep a room for you?"), None,
                   [reservar(), whatsapp("ghost")], tono="relleno"),
    ])

    serv = pagina("servicios", ("servicios-y-zona", "services-and-area"), ("Servicios y zona", "Services and area"),
                  ("Servicios del alojamiento, ubicación, qué ver cerca y cómo llegar en tren, coche o avión.",
                   "Services at the accommodation, location, what to see nearby and how to get here by train, car or plane."), [
        hero(("Servicios y zona", "Services and area"),
             ("Lo que tienes en el alojamiento y lo que tienes alrededor, para que planees tu estancia sin llamar.",
              "What you have at the accommodation and what is around it, so you can plan your stay without calling."),
             [reservar(), llamar("secondary")], variante="texto"),
        bloque_valores(("Servicios", "Services"), [
            (("Desayuno", "Breakfast"),
             ("De 7:30 a 10:30, en el comedor. Avisa de alergias al reservar.", "From 7:30 to 10:30, in the dining room. Tell us about allergies when you book.")),
            (("Wifi", "Wifi"),
             ("Gratuito en todo el edificio. La clave te la damos en recepción.", "Free throughout the building. We give you the password at reception.")),
            (("Recepción 24 horas", "24-hour reception"),
             ("Llegues cuando llegues, hay alguien. Servicio de ejemplo: cámbialo si tu recepción cierra por la noche.",
              "Whenever you arrive, someone is there. Example service: change it if your reception closes at night.")),
            (("Equipaje", "Luggage"),
             ("Guardamos tu equipaje el día de la salida y el de la llegada.", "We keep your luggage on the day you leave and the day you arrive.")),
            (("Lavandería", "Laundry"),
             ("Servicio por encargo, o lavandería de autoservicio cerca.", "Service on request, or a self-service launderette nearby.")),
            (("Información", "Information"),
             ("Mapas, horarios de transporte y consejos de dónde comer, siempre a mano.", "Maps, transport timetables and tips on where to eat, always to hand.")),
        ], intro=("Servicios de ejemplo: cámbialos por los que ofreces.", "Example services: replace them with what you offer."),
            cols=3, estilo="relleno", tono="relleno", anchor="servicios-lista"),
        bloque_checks(("Qué ver cerca", "What to see nearby"), [
            ("Casco antiguo y plaza principal, a unos minutos a pie.", "Old town and main square, a few minutes on foot."),
            ("Museos y monumentos más visitados de la ciudad.", "The city's most visited museums and monuments."),
            ("Mercado o calle de tapas para comer sin reserva.", "Market or tapas street where you can eat without booking."),
            ("Parque o paseo junto al río para pasear.", "Park or riverside walk for a stroll."),
            ("Estación de tren y paradas de autobús cercanas.", "Nearby train station and bus stops."),
            ("Tiendas y farmacia a pie de calle.", "Shops and a pharmacy on the street."),
        ], intro=("Texto genérico de ejemplo: cámbialo por lo que de verdad hay cerca de tu alojamiento y por cuánto se tarda a pie.",
                  "Example generic text: replace it with what is really near your accommodation and how long it takes on foot."),
            eyebrow=("Ubicación", "Location"), tono="base", anchor="cerca"),
        bloque_valores(("Cómo llegar", "Getting here"), [
            (("En tren o autobús", "By train or bus"),
             ("Indica aquí la estación o parada más cercana y cuánto se tarda a pie.", "Write here the nearest station or stop and how long it takes on foot.")),
            (("En coche", "By car"),
             ("Indica aquí la calle de acceso y el aparcamiento más cercano, con su precio por día.", "Write here the access street and the nearest car park, with its price per day.")),
            (("Desde el aeropuerto", "From the airport"),
             ("Indica aquí cómo llegar en transporte público o taxi y cuánto cuesta aproximadamente.", "Write here how to get here by public transport or taxi and roughly what it costs.")),
        ], intro=("Texto de ejemplo: cámbialo por tus indicaciones.", "Example text: replace it with your directions."),
            cols=3, estilo="borde", tono="relleno", anchor="llegar"),
        bloque_cta(("¿Alguna duda sobre la zona?", "Any questions about the area?"),
                   ("Escríbenos y te recomendamos dónde comer, qué ver y cómo moverte.",
                    "Write to us and we will recommend where to eat, what to see and how to get around."),
                   [whatsapp(), llamar("secondary")], tono="base"),
    ])

    cont = pagina("contacto", ("reservas", "bookings"), ("Reservas y condiciones", "Bookings and terms"),
                  ("Reserva directa con fechas y huéspedes, condiciones de llegada, salida y cancelación, preguntas frecuentes y cómo llegar.",
                   "Direct booking with dates and guests, check-in, check-out and cancellation terms, frequently asked questions and directions."), [
        hero(("Reserva directa", "Direct booking"),
             ("Dinos las fechas y cuántos sois. Te respondemos el mismo día con habitaciones libres y precio.",
              "Tell us the dates and how many of you there are. We reply the same day with free rooms and a price."),
             [llamar(), whatsapp()], variante="texto", extra=peq(REGISTRO, color="$muted")),
        bloque_contacto(("Tu reserva", "Your booking"),
                        ("Si las fechas son flexibles, dínoslo en el mensaje y miramos alternativas.",
                         "If your dates are flexible, say so in the message and we will look at alternatives."),
                        [campo("nombre", "text", ("Nombre", "Name"), True, auto="name", maxlen=120),
                         campo("telefono", "tel", ("Teléfono", "Phone"), True, auto="tel", maxlen=30),
                         campo("correo", "email", ("Correo electrónico", "Email"), False, auto="email", maxlen=200),
                         campo("llegada", "date", ("Fecha de llegada", "Arrival date"), True),
                         campo("salida", "date", ("Fecha de salida", "Departure date"), True),
                         campo("huespedes", "select", ("Huéspedes", "Guests"), True, opciones=HUESPEDES),
                         campo("habitacion", "select", ("Tipo de habitación", "Room type"), False,
                               opciones=[("Individual", "Single"), ("Doble", "Double"), ("Doble con dos camas", "Twin"),
                                         ("Familiar", "Family"), ("Me da igual", "No preference")]),
                         campo("mensaje", "textarea", ("Mensaje (opcional)", "Message (optional)"), False, maxlen=1000)],
                        ("Consultar disponibilidad", "Check availability"), horas, anchor="reservar",
                        notas=REGISTRO,
                        exito=("Gracias. Te respondemos con las habitaciones libres y el precio en cuanto lo veamos.",
                               "Thank you. We will reply with the free rooms and the price as soon as we see it.")),
        bloque_valores(("Llegada, salida y cancelación", "Check-in, check-out and cancellation"), [
            (("Llegada y salida", "Arrival and departure"),
             ("Entrada desde las 14:00 y salida hasta las 11:00. Si llegas antes o te vas después, dínoslo y guardamos tu equipaje.",
              "Check-in from 2:00 pm and check-out until 11:00 am. If you arrive earlier or leave later, tell us and we keep your luggage.")),
            (("Cancelación", "Cancellation"),
             ("Cancelación gratuita hasta 48 horas antes de la llegada. Después, se cobra la primera noche.",
              "Free cancellation up to 48 hours before arrival. After that, the first night is charged.")),
            (("Pago", "Payment"),
             ("Pagas al llegar o con un enlace de pago si lo prefieres. No hace falta tarjeta para consultar disponibilidad.",
              "You pay on arrival or with a payment link if you prefer. No card is needed to check availability.")),
        ], intro=CONDICIONES, cols=3, estilo="relleno", tono="relleno", anchor="condiciones"),
        bloque_faq(("Preguntas frecuentes", "Frequently asked questions"), [
            (("¿Aceptáis mascotas?", "Do you accept pets?"),
             ("Dínoslo al reservar y te contestamos si es posible y si tiene un suplemento.",
              "Tell us when you book and we will answer whether it is possible and whether there is a surcharge.")),
            (("¿Tenéis aparcamiento?", "Do you have parking?"),
             ("Te indicamos el aparcamiento más cercano y cuánto cuesta por día.",
              "We tell you the nearest car park and what it costs per day.")),
            (("¿Puedo pedir factura?", "Can I ask for an invoice?"),
             ("Sí. Dinos los datos fiscales al reservar o antes de la salida.",
              "Yes. Give us the tax details when you book or before you leave.")),
            (("¿Qué pasa si llego de madrugada?", "What if I arrive in the early hours?"),
             ("Avísanos con la hora aproximada y te esperamos en recepción.",
              "Let us know your approximate time and we will be waiting at reception.")),
        ], tono="base"),
        bloque_mapa(tono="relleno"),
    ])

    return plantilla(KEY, ("Hotel pequeño", "Small hotel"),
                     ("Hotel pequeño, hostal o pensión urbana con habitaciones por tipo y tarifa de ejemplo, servicios, qué ver cerca, política de cancelación y reserva directa por fechas y huéspedes.",
                      "Small urban hotel, hostel or guesthouse with rooms by type and example rate, services, what to see nearby, cancellation policy and direct booking by dates and guests."),
                     "turismo", ("hotel hotel pequeno hostal pension hospedaje alojamiento habitaciones habitacion reservas reserva directa recepcion desayuno wifi check-in registro turistico tarifas cancelacion",
                                 "hotel small hotel hostel guesthouse pension b&b lodging accommodation rooms room booking direct booking reception breakfast wifi check-in tourist register rates cancellation"),
                     look, cabecera(nav, cta_cab),
                     pie(nav, ("Hotel pequeño con recepción y desayuno. Nº de registro turístico: 00-0000.",
                               "Small hotel with reception and breakfast. Tourist register no.: 00-0000."),
                         ("Recepción las 24 horas. Entrada desde las 14:00, salida hasta las 11:00.",
                          "Reception 24 hours. Check-in from 2 pm, check-out until 11 am.")),
                     [inicio, habs, serv, cont], vertical="alojamiento")
