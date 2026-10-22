# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
import lib
from lib import *

KEY = "comunidad-vecinos"

# Paleta propia: azul grisáceo tranquilo, de papel de oficina. Pasa AA (se comprueba al construir).
VECINDAD = dict(
    colores=dict(background="#f6f9fa", surface="#e6eef1", text="#12232b", muted="#46606b", primary="#1c6577",
                 on_primary="#ffffff", border="#d0dde2", inverse="#12303a", on_inverse="#e9f4f7"),
    fuentes=("manrope", "noto-sans"), radio=(6, 10, 14))


def S(x):
    return (x, x)


EJ = ("000 000 000 (de ejemplo)", "000 000 000 (example)")


def build():
    nueva(KEY)
    malos = lib.comprobar_paleta("vecindad", VECINDAD["colores"])
    assert not malos, malos
    look = VECINDAD
    nav = [("inicio", ("Inicio", "Home")), ("normas", ("Normas y zonas comunes", "Rules and shared areas")),
           ("juntas", ("Juntas y documentos", "Meetings and documents")),
           ("incidencias", ("Incidencias y contacto", "Issues and contact"))]
    horas = [(("Lunes a jueves", "Monday to Thursday"), ("9:00 a 14:00 y 16:00 a 18:30", "9:00 am to 2:00 pm and 4:00 pm to 6:30 pm")),
             (("Viernes", "Friday"), ("9:00 a 14:00", "9:00 am to 2:00 pm")),
             (("Sábado y domingo", "Saturday and Sunday"), ("Cerrado. Urgencias: 112", "Closed. Emergencies: 112"))]
    avisar = ("Comunicar una incidencia", "Report an issue")

    def incidencia(var="primary"):
        return boton(avisar, a_pagina("incidencias", "contacto"), var)

    cta_cab = marco([llamar("secondary", ("Llamar al administrador", "Call the administrator")),
                     whatsapp("ghost", ("WhatsApp", "WhatsApp"))], dir="row", gap=12, wrap=True)

    def aviso(titulo, texto_):
        return tarjeta([etiqueta(("Fecha del aviso", "Notice date")), h3(titulo), p(texto_, color="$muted")],
                       estilo="borde", gap=8)

    inicio = pagina("inicio", ("", ""), ("Comunidad de vecinos", "Residents' community"),
                    ("Web de la comunidad de vecinos: avisos, contactos útiles del edificio, qué hacer en una urgencia y botón para comunicar una incidencia.",
                     "The residents' community website: notices, useful building contacts, what to do in an emergency and a button to report an issue."), [
        hero(("La web de nuestra comunidad", "Our community's website"),
             ("Aquí están los avisos, las normas, las fechas de las juntas y a quién llamar. Si algo se estropea, cuéntalo en el formulario de incidencias.",
              "Here you will find the notices, the rules, the meeting dates and who to call. If something breaks, tell us in the issues form."),
             [incidencia(), llamar("secondary", ("Llamar al administrador", "Call the administrator")),
              whatsapp("ghost", ("Escribir al administrador", "Message the administrator"))],
             eyebrow=CALLE, variante="centro", tono="relleno"),
        seccion("features", [
            titular(("Avisos vigentes", "Current notices"),
                    ("Avisos de ejemplo: cámbialos por los de tu comunidad y borra los que ya no valgan.",
                     "Example notices: replace them with your community's and delete the ones that no longer apply.")),
            lista_vertical([
                aviso(("Aviso de ejemplo: revisión del ascensor", "Example notice: lift inspection"),
                      ("Escribe aquí qué día es, cuánto dura y qué tienen que hacer los vecinos.",
                       "Write here which day it is, how long it lasts and what residents need to do.")),
                aviso(("Aviso de ejemplo: limpieza del garaje", "Example notice: garage cleaning"),
                      ("Escribe aquí qué plazas hay que dejar libres y a qué hora.",
                       "Write here which spaces must be left free and at what time.")),
                aviso(("Aviso de ejemplo: convocatoria de junta", "Example notice: meeting call"),
                      ("Escribe aquí la fecha y dónde se puede consultar el orden del día.",
                       "Write here the date and where the agenda can be read.")),
            ], gap=14),
        ], tono="base", anchor="avisos", gap=28),
        seccion("features", [
            titular(("Contactos útiles del edificio", "Useful building contacts"),
                    ("Para cada cosa, una persona. Así no hace falta llamar a todos.",
                     "One person for each thing, so nobody has to call everyone."), eyebrow=("Teléfonos", "Phone numbers")),
            tabla(("Contactos útiles del edificio (de ejemplo)", "Useful building contacts (example)"),
                  [("Quién", "Who"), ("Para qué", "What for"), ("Teléfono", "Phone")],
                  [(("Administrador de fincas", "Property administrator"),
                    ("Cuentas, recibos, seguros, juntas y obras", "Accounts, bills, insurance, meetings and works"),
                    S("600 000 000")),
                   (("Portero o conserje", "Doorman or caretaker"),
                    ("Paquetes, llaves y avisos del día a día", "Parcels, keys and day-to-day matters"), EJ),
                   (("Mantenimiento del ascensor", "Lift maintenance"),
                    ("Ascensor parado o averiado", "Lift stopped or out of order"), EJ),
                   (("Urgencias", "Emergencies"),
                    ("Incendio, gas o riesgo para las personas", "Fire, gas or danger to people"), S("112"))]),
            marco([llamar("primary", ("Llamar al administrador", "Call the administrator")),
                   whatsapp("secondary", ("Escribir al administrador", "Message the administrator"))],
                  dir="row", gap=12, wrap=True),
        ], tono="relleno", anchor="contactos", gap=28),
        bloque_valores(("Qué hacer en una urgencia", "What to do in an emergency"), [
            (("Fuga de agua", "Water leak"),
             ("Cierra la llave de paso de tu vivienda y avisa al administrador. Si el agua llega a otros pisos, avisa a esos vecinos.",
              "Turn off the stopcock in your home and tell the administrator. If water reaches other flats, warn those neighbours.")),
            (("Ascensor parado con gente dentro", "Lift stopped with people inside"),
             ("Llama al teléfono de mantenimiento que figura en la cabina. Si hay riesgo, al 112. No intentes abrir las puertas.",
              "Call the maintenance number shown in the lift car. If there is danger, call 112. Do not try to open the doors.")),
            (("Olor a gas o fuego", "Smell of gas or fire"),
             ("Sal del edificio por la escalera y llama al 112 desde fuera. No uses el ascensor ni enciendas luces.",
              "Leave the building by the stairs and call 112 from outside. Do not use the lift or switch lights on.")),
        ], eyebrow=("Urgencias", "Emergencies"), tono="base", estilo="borde", anchor="urgencias"),
        bloque_cta(("¿Algo no funciona en el edificio?", "Is something not working in the building?"),
                   ("Dinos qué es y dónde está. El administrador lo recibe y te contesta.",
                    "Tell us what it is and where it is. The administrator receives it and replies."),
                   [incidencia(), llamar("ghost", ("Llamar al administrador", "Call the administrator"))], tono="relleno"),
    ])

    normas = pagina("normas", ("normas", "rules"), ("Normas y zonas comunes", "Rules and shared areas"),
                    ("Normas de convivencia del edificio, zonas comunes con su horario y cómo reservar la pista de pádel o la sala.",
                     "House rules, shared areas with their hours and how to book the padel court or the community room."), [
        hero(("Normas y zonas comunes", "Rules and shared areas"),
             ("Unas normas sencillas para vivir bien en un edificio con muchos vecinos. Las zonas comunes se reservan antes de usarlas.",
              "A few simple rules for living well in a building with many neighbours. Shared areas are booked before use."),
             [incidencia(), whatsapp("secondary", ("Escribir al administrador", "Message the administrator"))],
             variante="texto"),
        bloque_checks(("Convivencia", "Living together"), [
            ("Respeta el descanso: evita ruidos fuertes por la noche y a la hora de la siesta.",
             "Respect quiet hours: avoid loud noise at night and during the afternoon rest."),
            ("Baja la basura en bolsas cerradas y llévala al contenedor, no la dejes en el portal.",
             "Take rubbish down in closed bags and put it in the bin, not in the entrance hall."),
            ("Escaleras, portal y pasillos son salida de emergencia: no dejes bicis, carritos ni cajas.",
             "Stairs, entrance hall and corridors are emergency exits: do not leave bikes, prams or boxes."),
            ("Cierra siempre la puerta del portal y no dejes entrar a quien no conozcas.",
             "Always close the main door and do not let in people you do not know."),
            ("Las mascotas, atadas en las zonas comunes, y recoge siempre lo que dejen.",
             "Pets on a lead in shared areas, and always clean up after them."),
            ("Las bicis y patinetes, en el cuarto de bicis y no en el ascensor.",
             "Bikes and scooters go in the bike room, not in the lift."),
        ], intro=("Normas de ejemplo: cámbialas por las de vuestros estatutos y de vuestro reglamento interno.",
                  "Example rules: replace them with those of your statutes and your house regulations."),
            eyebrow=("Normas de convivencia", "House rules"), tono="base", cols=2, anchor="convivencia"),
        bloque_valores(("Zonas comunes y reservas", "Shared areas and bookings"), [
            (("Pista de pádel", "Padel court"),
             ("Se reserva por franjas de una hora. Pide la hora al administrador o al portero con un día de antelación.",
              "Booked in one-hour slots. Ask the administrator or doorman for a time one day ahead.")),
            (("Sala comunitaria", "Community room"),
             ("Para reuniones y celebraciones de los vecinos. Se reserva con el día y la hora, y se deja limpia y recogida.",
              "For residents' meetings and celebrations. Booked by day and time, and left clean and tidy.")),
            (("Cuarto de bicis y trasteros", "Bike room and storage rooms"),
             ("De uso libre para quien tenga plaza o trastero. Si se te ha perdido la llave, pídela al administrador.",
              "Free to use for whoever has a space or storage room. If you have lost your key, ask the administrator.")),
        ], intro=("Reglas de ejemplo: cámbialas por las de tu comunidad (franjas, antelación, fianza si la hay).",
                  "Example rules: replace them with your community's (slots, notice, deposit if there is one)."),
            eyebrow=("Reservas", "Bookings"), tono="relleno", estilo="borde", anchor="reservas"),
        seccion("pricing", [
            titular(("Horario de las zonas comunes", "Opening hours of shared areas")),
            tabla(("Horario de las zonas comunes (de ejemplo)", "Opening hours of shared areas (example)"),
                  [("Zona", "Area"), ("Horario", "Hours")],
                  [(("Pista de pádel", "Padel court"), ("10:00 a 22:00, con reserva", "10:00 am to 10:00 pm, by booking")),
                   (("Sala comunitaria", "Community room"), ("10:00 a 23:00, con reserva", "10:00 am to 11:00 pm, by booking")),
                   (("Cuarto de bicis", "Bike room"), ("Siempre abierto para quien tiene llave", "Always open for key holders"))]),
            peq(("Horarios de ejemplo: cámbialos por los de tu comunidad.", "Example hours: replace them with your community's."),
                color="$muted"),
        ], tono="base", anchor="horario-zonas", gap=28),
        bloque_pasos(("Cómo reservar", "How to book"), [
            (("Pide la zona", "Ask for the area"),
             ("Escribe al administrador o al portero con la zona, el día y la hora.",
              "Write to the administrator or doorman with the area, the day and the time.")),
            (("Espera la confirmación", "Wait for confirmation"),
             ("Te contestamos si está libre. Si no, te proponemos otra hora.",
              "We reply to say whether it is free. If not, we suggest another time.")),
            (("Déjala como la encontraste", "Leave it as you found it"),
             ("Al terminar, recoge, apaga las luces y avisa si algo se ha roto.",
              "When you finish, tidy up, switch off the lights and tell us if anything is broken.")),
        ], tono="relleno", anchor="como-reservar"),
        bloque_cta(("¿Quieres reservar una zona?", "Want to book an area?"), None,
                   [whatsapp("primary", ("Reservar por WhatsApp", "Book on WhatsApp")), llamar("ghost")], tono="base"),
    ])

    juntas = pagina("juntas", ("juntas", "meetings"), ("Juntas y documentos", "Meetings and documents"),
                    ("Calendario de juntas de la comunidad, cómo funciona una junta y qué documentos se pueden pedir al administrador.",
                     "The community's meeting calendar, how a meeting works and which documents you can ask the administrator for."), [
        hero(("Juntas y documentos", "Meetings and documents"),
             ("Cuándo son las juntas, cómo participar si no puedes ir y qué papeles puedes pedir. Los documentos no se publican aquí.",
              "When the meetings are, how to take part if you cannot attend and which papers you can ask for. Documents are not published here."),
             [incidencia("secondary"), whatsapp()], variante="texto"),
        seccion("pricing", [
            titular(("Calendario de juntas", "Meeting calendar"),
                    ("El administrador envía la convocatoria por escrito, con el orden del día.",
                     "The administrator sends the call in writing, with the agenda."), eyebrow=("Calendario", "Calendar")),
            tabla(("Calendario de juntas (de ejemplo)", "Meeting calendar (example)"),
                  [("Junta", "Meeting"), ("Fecha", "Date"), ("Hora", "Time"), ("Lugar", "Place"), ("Se trata", "Agenda")],
                  [(("Junta ordinaria", "Annual meeting"), ("Fecha de la junta", "Meeting date"), ("Hora de la junta", "Meeting time"),
                    ("Lugar de la junta", "Meeting place"),
                    ("Cuentas del año, presupuesto y obras previstas", "Year's accounts, budget and planned works")),
                   (("Junta extraordinaria", "Extraordinary meeting"), ("Si se convoca", "If called"), ("Hora de la junta", "Meeting time"),
                    ("Lugar de la junta", "Meeting place"),
                    ("Un asunto que no puede esperar a la ordinaria", "A matter that cannot wait for the annual meeting"))]),
            peq(("Fechas de ejemplo: cámbialas por las reales de tu comunidad.", "Example dates: replace them with your community's real ones."),
                color="$muted"),
        ], tono="base", anchor="calendario", gap=28),
        bloque_pasos(("Cómo funciona una junta", "How a meeting works"), [
            (("Recibes la convocatoria", "You receive the call"),
             ("Con la fecha, el lugar y los temas. Mírala con calma y apunta lo que quieras decir.",
              "With the date, place and topics. Read it calmly and note what you want to say.")),
            (("Vas o delegas", "Attend or delegate"),
             ("Si no puedes ir, puedes delegar tu voto por escrito en otra persona. Pregunta al administrador qué documento hace falta.",
              "If you cannot attend, you can delegate your vote in writing to someone else. Ask the administrator which document is needed.")),
            (("Se vota y se redacta el acta", "Votes and minutes"),
             ("Cada tema se debate y se vota. Lo acordado queda por escrito en el acta.",
              "Each topic is discussed and voted on. What is agreed goes into the minutes.")),
        ], tono="relleno", anchor="como-funciona"),
        seccion("features", [
            titular(("Documentos que puedes pedir", "Documents you can ask for"),
                    ("Se piden al administrador por correo o en su horario de atención. Contienen datos de vecinos y no se cuelgan en la web.",
                     "Ask the administrator by email or during their opening hours. They contain residents' data and are not posted on the website."),
                    eyebrow=("Documentos", "Documents")),
            rejilla([
                tarjeta([h3(("Actas de las juntas", "Meeting minutes")),
                         p(("Lo que se acordó y cómo se votó, de cada junta.", "What was agreed and how it was voted, for each meeting."), color="$muted")], estilo="linea"),
                tarjeta([h3(("Estatutos y normas internas", "Statutes and house regulations")),
                         p(("Las reglas propias del edificio: usos, cuotas y zonas comunes.", "The building's own rules: uses, fees and shared areas."), color="$muted")], estilo="linea"),
                tarjeta([h3(("Presupuesto y cuentas", "Budget and accounts")),
                         p(("Lo que se prevé gastar y lo que se ha gastado en el año.", "What is expected to be spent and what has been spent in the year."), color="$muted")], estilo="linea"),
                tarjeta([h3(("Seguro del edificio", "Building insurance")),
                         p(("La póliza, lo que cubre y a quién avisar en un siniestro.", "The policy, what it covers and who to notify in a claim."), color="$muted")], estilo="linea"),
            ], cols=2, gap=20),
        ], tono="base", anchor="documentos", gap=28),
        bloque_faq(("Preguntas frecuentes", "Frequently asked questions"), [
            (("¿Dónde pido el acta de la última junta?", "Where do I ask for the last meeting's minutes?"),
             ("Al administrador, por correo o en su horario de atención. Te la envía a ti, no se cuelga en la web.",
              "From the administrator, by email or during opening hours. They send it to you; it is not posted on the website.")),
            (("¿Puedo proponer un tema para la junta?", "Can I propose a topic for the meeting?"),
             ("Sí. Díselo al administrador antes de que envíe la convocatoria, para que entre en el orden del día.",
              "Yes. Tell the administrator before the call goes out, so it can be added to the agenda.")),
            (("¿Y si no estoy de acuerdo con un acuerdo?", "What if I disagree with a decision?"),
             ("Habla primero con el administrador. Para los plazos y los pasos a seguir, consulta los estatutos o pide asesoramiento profesional.",
              "Speak to the administrator first. For deadlines and next steps, check the statutes or seek professional advice.")),
        ], tono="relleno"),
    ])

    campos = [
        campo("tipo", "select", ("Tipo de incidencia", "Type of issue"), True,
              opciones=[("Ascensor", "Lift"), ("Luz o iluminación", "Lighting"), ("Fuga o humedad", "Leak or damp"),
                        ("Limpieza", "Cleaning"), ("Ruido o convivencia", "Noise or neighbours"),
                        ("Garaje o trasteros", "Garage or storage"), ("Puertas, cerraduras o portero automático", "Doors, locks or entryphone"),
                        ("Otra", "Other")]),
        campo("ubicacion", "select", ("Dónde está", "Where it is"), True,
              opciones=[("Portal o entrada", "Entrance hall"), ("Escalera o pasillo", "Stairs or corridor"),
                        ("Ascensor", "Lift"), ("Garaje", "Garage"), ("Azotea o cubierta", "Roof"),
                        ("Patio o jardín", "Courtyard or garden"), ("Pista de pádel", "Padel court"),
                        ("Sala comunitaria", "Community room"), ("Otra zona común", "Other shared area")]),
        campo("detalle", "text", ("Planta, portal o escalera (opcional)", "Floor, entrance or staircase (optional)"), False, maxlen=120),
        campo("descripcion", "textarea", ("Cuéntanos qué pasa", "Tell us what is happening"), True, maxlen=1500),
        campo("nombre", "text", ("Tu nombre (opcional)", "Your name (optional)"), False, auto="name", maxlen=120),
        campo("contacto", "text", ("Teléfono o correo para avisarte (opcional)", "Phone or email to reach you (optional)"),
              False, maxlen=200),
    ]
    cont = pagina("incidencias", ("incidencias", "issues"), ("Incidencias y contacto", "Issues and contact"),
                  ("Formulario para comunicar una incidencia en el edificio, teléfono y WhatsApp del administrador y su horario de atención.",
                   "Form to report an issue in the building, the administrator's phone and WhatsApp and their opening hours."), [
        hero(("Comunicar una incidencia", "Report an issue"),
             ("Una luz fundida, una fuga, un ruido que se repite. Dinos qué es y dónde está, y el administrador lo gestiona.",
              "A blown bulb, a leak, a noise that keeps happening. Tell us what it is and where it is, and the administrator deals with it."),
             [llamar("primary", ("Llamar al administrador", "Call the administrator")),
              whatsapp("secondary", ("Escribir al administrador", "Message the administrator"))], variante="texto"),
        bloque_contacto(("Formulario de incidencias", "Issues form"),
                        ("Si hay peligro para las personas, no uses el formulario: llama al 112.",
                         "If people are in danger, do not use the form: call 112."),
                        campos, ("Enviar la incidencia", "Send the issue"), None,
                        notas=("Esta web no publica datos personales de los vecinos. Lo que escribas en el formulario solo se usa para gestionar la incidencia.",
                               "This website does not publish residents' personal data. What you write in the form is only used to deal with the issue."),
                        exito=("Gracias. Hemos recibido la incidencia y el administrador la revisará lo antes posible.",
                               "Thank you. We have received the issue and the administrator will look at it as soon as possible.")),
        seccion("features", [
            titular(("Cuándo atiende el administrador", "When the administrator is available")),
            horario(horas, caption=("Horario de atención (de ejemplo)", "Opening hours (example)")),
            peq(("Horario de ejemplo: cámbialo por el de tu administrador.", "Example hours: replace them with your administrator's."),
                color="$muted"),
        ], tono="relleno", anchor="horario", gap=28),
    ])

    return plantilla(KEY, ("Comunidad de vecinos", "Residents' community"),
                     ("Web de comunidad de vecinos con avisos, contactos útiles, normas, reservas de zonas comunes, calendario de juntas, documentos y formulario de incidencias.",
                      "Residents' community website with notices, useful contacts, rules, shared-area bookings, a meeting calendar, documents and an issues form."),
                     "comunidad", ("comunidad de vecinos comunidad de propietarios edificio urbanizacion portal administrador de fincas portero conserje junta de vecinos actas estatutos incidencias zonas comunes padel normas de convivencia",
                                   "residents community homeowners association block of flats building notices property administrator caretaker meetings minutes shared areas issues"),
                     look, cabecera(nav, cta_cab),
                     pie(nav, ("Comunidad de vecinos. Avisos, normas, juntas e incidencias. No publicamos datos personales de los vecinos.",
                               "Residents' community. Notices, rules, meetings and issues. We do not publish residents' personal data."),
                         ("Administrador: lunes a jueves 9:00 a 14:00 (horario de ejemplo).",
                          "Administrator: Monday to Thursday 9 am to 2 pm (example hours).")),
                     [inicio, normas, juntas, cont],
                     exento={"mapa": "Un edificio no necesita mapa: los vecinos ya saben dónde viven y la dirección figura en el pie y en contacto."})
