# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
import lib
from lib import *

KEY = "evento-puntual"

# Paleta propia: papel rosado y fucsia profundo, para un acto que se celebra. Pasa AA (se comprueba al construir).
CELEBRACION = dict(
    colores=dict(background="#fdf8fd", surface="#f6e8f7", text="#2a1230", muted="#66506b", primary="#a21caf",
                 on_primary="#ffffff", border="#ecd5ee", inverse="#2a1230", on_inverse="#fbeefc"),
    fuentes=("playfair-display", "work-sans"), peso=600, h1=(60, 44, 34), radio=(8, 14, 22))


def S(x):
    return (x, x)


def dato(ico, etiqueta_, valor):
    """Un dato de la portada: icono, rótulo pequeño y valor."""
    return marco([icono(ico, sz=26),
                  marco([peq(etiqueta_, color="$muted"), texto("p", valor, weight=600)], gap=2)],
                 dir="row", gap=12, align="center")


def build():
    nueva(KEY)
    malos = lib.comprobar_paleta("celebracion", CELEBRACION["colores"])
    assert not malos, malos
    look = CELEBRACION

    anclas = [("programa", ("Programa", "Programme")), ("lugar", ("Lugar y cómo llegar", "Venue and getting here")),
              ("alojamiento", ("Alojamiento", "Where to stay")), ("preguntas", ("Preguntas", "Questions")),
              ("confirmar", ("Confirmar asistencia", "RSVP"))]

    cab = cabecera([("inicio", ("Inicio", "Home"))],
                   marco([llamar("secondary", ("Llamar", "Call")), whatsapp("ghost", ("WhatsApp", "WhatsApp"))],
                         dir="row", gap=12, wrap=True))
    # Una sola página: el menú salta a las secciones, no a otras páginas.
    menu_ = cab["children"][1]["children"][0]
    menu_["items"] = [{"id": lib._id(), "label": L(t), "link": a_pagina("inicio", a)} for a, t in anclas]

    confirmar = lambda var="primary": boton(("Confirmar asistencia", "Confirm attendance"), a_ancla("confirmar"), var)

    def como(titulo, texto_):
        return tarjeta([h3(titulo), p(texto_, color="$muted")], estilo="borde", gap=8)

    def alojamiento(titulo, texto_):
        return tarjeta([icono("bed", sz=26), h3(titulo), p(texto_, color="$muted")], estilo="relleno", gap=10)

    inicio = pagina("inicio", ("", ""), ("Evento", "Event"),
                    ("Página de un evento puntual: fecha y lugar, programa, cómo llegar, alojamiento cercano, preguntas frecuentes y confirmación de asistencia.",
                     "One-off event page: date and venue, programme, how to get there, nearby places to stay, frequently asked questions and RSVP."), [
        hero(("Nombre del evento", "Event name"),
             ("Escribe aquí en dos líneas de qué trata el evento, quién lo organiza y a quién va dirigido.",
              "Write here, in two lines, what the event is about, who is organising it and who it is for."),
             [confirmar(), llamar("secondary"), whatsapp("ghost")],
             eyebrow=("Fecha del evento", "Event date"), variante="centro", tono="base",
             extra=marco([dato("calendar", ("Fecha", "Date"), ("Fecha del evento", "Event date")),
                          dato("clock", ("Hora", "Time"), ("Hora de inicio", "Start time")),
                          dato("map-pin", ("Lugar", "Venue"), ("Nombre del lugar, Tu ciudad", "Venue name, Your city"))],
                         dir="row", gap=28, wrap=True, align="center", justify="center")),
        seccion("features", [
            titular(("Programa", "Programme"),
                    ("Así será el día, hora por hora. Programa de ejemplo: cámbialo por el tuyo.",
                     "How the day will go, hour by hour. Example programme: replace it with your own."),
                    eyebrow=("El día", "The day")),
            tabla(("Programa y horario del evento (de ejemplo)", "Programme and timetable of the event (example)"),
                  [("Hora", "Time"), ("Qué ocurre", "What happens"), ("Dónde", "Where")],
                  [(S("12:00"), ("Llegada y recepción", "Arrival and welcome"), ("Entrada principal", "Main entrance")),
                   (S("12:30"), ("Acto principal", "Main event"), ("Sala o jardín", "Hall or garden")),
                   (S("14:00"), ("Comida o aperitivo", "Meal or reception"), ("Zona de mesas", "Dining area")),
                   (S("16:30"), ("Música, juegos o visitas", "Music, games or tours"), ("Sala o jardín", "Hall or garden")),
                   (S("19:00"), ("Despedida", "Farewell"), ("Entrada principal", "Main entrance"))]),
        ], tono="relleno", anchor="programa", gap=28),
        seccion("map", [
            titular(("El lugar y cómo llegar", "The venue and how to get there"),
                    ("Nombre del lugar, Calle Mayor, 1, Tu ciudad. El mapa te lleva hasta la puerta.",
                     "Venue name, Calle Mayor, 1, Your city. The map takes you to the door."), eyebrow=("Lugar", "Venue")),
            mapa(("Mapa con la ubicación del evento", "Map showing the location of the event")),
            rejilla([
                como(("En coche", "By car"),
                     ("Escribe aquí dónde se puede aparcar y cuánto se tarda en llegar a pie desde allí.",
                      "Write here where to park and how long it takes to walk from there.")),
                como(("En transporte público", "By public transport"),
                     ("Escribe aquí la parada o estación más cercana y la línea que hay que coger.",
                      "Write here the nearest stop or station and the line to take.")),
                como(("Accesibilidad", "Accessibility"),
                     ("Escribe aquí si el lugar tiene rampa, ascensor y aseos adaptados, y avisa de cualquier escalón.",
                      "Write here whether the venue has a ramp, a lift and accessible toilets, and warn of any steps.")),
            ], cols=3, gap=20),
        ], tono="base", anchor="lugar", gap=28),
        seccion("features", [
            titular(("Dónde alojarse", "Where to stay"),
                    ("Opciones de ejemplo: sustituye estas por alojamientos reales cerca del lugar.",
                     "Example options: replace these with real places to stay near the venue."),
                    eyebrow=("Alojamiento cercano", "Nearby accommodation")),
            rejilla([
                alojamiento(("Hotel o pensión", "Hotel or guesthouse"),
                            ("Escribe aquí el nombre, a qué distancia está y cómo reservar.",
                             "Write here the name, how far it is and how to book.")),
                alojamiento(("Apartamento o casa rural", "Apartment or country house"),
                            ("Escribe aquí el nombre, a qué distancia está y cómo reservar.",
                             "Write here the name, how far it is and how to book.")),
                alojamiento(("Otra opción cerca", "Another option nearby"),
                            ("Escribe aquí el nombre y si hay precio especial para las personas invitadas.",
                             "Write here the name and whether there is a special rate for guests.")),
            ], cols=3, gap=20),
            peq(("Consejo: reserva con tiempo, sobre todo si el evento cae en fin de semana o en temporada alta.",
                 "Tip: book early, especially if the event falls on a weekend or in high season."), color="$muted"),
        ], tono="relleno", anchor="alojamiento", gap=28),
        bloque_faq(("Preguntas frecuentes", "Frequently asked questions"), [
            (("¿Puedo ir con niños?", "Can I bring children?"),
             ("Sí, son bienvenidos. Cuéntanos cuántos vienen en el formulario para preparar su sitio y su comida.",
              "Yes, they are welcome. Tell us how many are coming in the form so we can prepare their seats and food.")),
            (("¿Puedo llevar acompañante?", "Can I bring a guest?"),
             ("Solo si lo pone en tu invitación. Si tienes dudas, pregunta al organizador antes de confirmar.",
              "Only if your invitation says so. If in doubt, ask the organiser before confirming.")),
            (("¿Qué ropa llevo?", "What should I wear?"),
             ("Ropa cómoda y calzado para caminar. Si el evento es al aire libre, algo de abrigo para la tarde.",
              "Comfortable clothes and shoes for walking. If the event is outdoors, something warm for the evening.")),
            (("¿Hay comida sin gluten, vegetariana o sin alérgenos?", "Is there gluten-free, vegetarian or allergen-free food?"),
             ("Sí. Cuéntanos tus alergias e intolerancias en el formulario y se lo pasamos a quien prepara la comida.",
              "Yes. Tell us about your allergies and intolerances in the form and we will pass them on to whoever prepares the food.")),
            (("¿Hasta cuándo puedo confirmar?", "How long can I confirm for?"),
             ("Hasta la fecha límite de confirmación. Si después cambia tu plan, avísanos por teléfono o por WhatsApp.",
              "Until the confirmation deadline. If your plans change afterwards, let us know by phone or on WhatsApp.")),
        ], intro=("Respuestas de ejemplo: cámbialas por las de tu evento.",
                  "Example answers: replace them with those of your event."), tono="base"),
        seccion("contact", [
            titular(("Confirma tu asistencia", "Confirm your attendance"),
                    ("Confirma antes de la fecha límite de confirmación para que podamos contar contigo.",
                     "Confirm before the confirmation deadline so we can count on you."), eyebrow=("Confirmación", "RSVP")),
            tarjeta([formulario([
                campo("nombre", "text", ("Nombre y apellidos", "Full name"), True, auto="name", maxlen=120),
                campo("correo", "email", ("Correo electrónico", "Email"), True, auto="email", maxlen=200),
                campo("telefono", "tel", ("Teléfono (opcional)", "Phone (optional)"), False, auto="tel", maxlen=30),
                campo("asistencia", "select", ("¿Vienes?", "Are you coming?"), True,
                      opciones=[("Sí, asistiré", "Yes, I will be there"), ("No podré asistir", "I will not be able to come")]),
                campo("asistentes", "select", ("Personas en total, contándote a ti", "Total number of people, including you"), True,
                      opciones=[S("1"), S("2"), S("3"), S("4"), S("5"), ("6 o más", "6 or more")]),
                campo("alergias", "textarea", ("Alergias o intolerancias (di quién y cuáles)", "Allergies or intolerances (say who and which)"),
                      False, maxlen=500),
                campo("mensaje", "textarea", ("Mensaje para el organizador (opcional)", "Message for the organiser (optional)"),
                      False, maxlen=1000),
            ], ("Enviar confirmación", "Send confirmation"),
                exito=("Gracias. Hemos recibido tu respuesta y te escribiremos si necesitamos algo más.",
                       "Thank you. We have received your reply and will write to you if we need anything else."),
                consentimiento=("Acepto que se usen estos datos, incluidas las alergias, solo para organizar el evento.",
                                "I agree to this data, including allergies, being used only to organise the event.")),
                peq(("Los datos de este formulario solo sirven para organizar el evento y preparar la comida.",
                     "The data in this form is only used to organise the event and prepare the food."), color="$muted")],
                estilo="borde", pad=28, tag="div", sz={"width": "fill", "height": "hug", "max_width": 680}),
        ], tono="relleno", anchor="confirmar", gap=28),
        bloque_cta(("¿Dudas? Habla con el organizador", "Questions? Talk to the organiser"),
                   ("Nombre Apellido, organizador. Teléfono 600 000 000. Correo info@example.com.",
                    "First Last, organiser. Phone 600 000 000. Email info@example.com."),
                   [llamar(), whatsapp()], tono="base"),
    ])

    return plantilla(KEY, ("Evento puntual", "One-off event"),
                     ("Página de un solo evento (fiesta, jornada, feria, boda o aniversario) con fecha, programa, mapa, alojamiento, preguntas y formulario de asistencia.",
                      "Single-event page (party, open day, fair, wedding or anniversary) with date, programme, map, accommodation, questions and an RSVP form."),
                     "web", ("evento puntual evento fiesta jornada feria boda aniversario cumpleanos celebracion invitacion confirmar asistencia programa del evento una pagina",
                             "event one-off event party open day fair wedding anniversary birthday celebration invitation rsvp programme single page"),
                     look, cab,
                     pie(anclas, ("Página del evento. Fecha, lugar, programa y confirmación de asistencia.",
                                  "Event page. Date, venue, programme and RSVP."),
                         ("El programa del evento está en la sección Programa.", "The event timetable is in the Programme section.")),
                     [inicio])
