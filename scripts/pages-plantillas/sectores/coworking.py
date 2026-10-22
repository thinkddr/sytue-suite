# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
from lib import *

KEY = "coworking"


def _c(background, surface, text, muted, primary, on_primary, border, inverse, on_inverse):
    return dict(background=background, surface=surface, text=text, muted=muted, primary=primary,
                on_primary=on_primary, border=border, inverse=inverse, on_inverse=on_inverse)


# Paleta propia: papel verdoso, tinta casi negra y acento lima en los botones. Sin fondo $inverse
# (el primario ya es oscuro y no contrastaría con él).
LIMA = dict(colores=_c("#fafaf7", "#eff1e8", "#151712", "#54584a", "#1d2410", "#d7f26a", "#dcdfd0", "#151712", "#f2f5e6"),
            fuentes=("work-sans", "inter"), radio=(6, 10, 14), ancho=1200, peso=700, h1=(54, 42, 32))


def con_titulo_de_tabla(nodo, nuevo):
    """Cambia el título de la tabla de horario que lleva bloque_contacto (por defecto «Horario de apertura»)."""
    if nodo.get("type") == "table":
        nodo["caption"] = L(nuevo)
    for h in nodo.get("children", []):
        con_titulo_de_tabla(h, nuevo)
    return nodo


def build():
    nueva(KEY)
    look = LIMA
    malos = comprobar_paleta("lima", look["colores"])
    assert not malos, malos
    nav = [("inicio", ("Inicio", "Home")), ("puestos", ("Puestos y tarifas", "Desks and prices")),
           ("espacio", ("El espacio", "The space")), ("visita", ("Visita", "Visit"))]
    acceso = [(("Lunes a viernes", "Monday to Friday"), ("8:00 a 21:00", "8:00 am to 9:00 pm")),
              (("Sábado", "Saturday"), ("9:00 a 14:00", "9:00 am to 2:00 pm")),
              (("Domingo y festivos", "Sunday and holidays"), ("Cerrado", "Closed"))]
    nota = ("Precios de ejemplo: cámbialos por los tuyos.", "Example prices: replace them with your own.")
    puestos_opc = [("Pase de día", "Day pass"), ("Puesto flexible", "Hot desk"), ("Puesto fijo", "Dedicated desk"),
                   ("Despacho privado", "Private office"), ("Todavía no lo sé", "I do not know yet")]

    def visita(var="primary"):
        return boton(("Reservar una visita", "Book a visit"), a_pagina("visita", "contacto"), var)

    inicio = pagina("inicio", ("", ""), ("Coworking", "Coworking"),
                    ("Espacio de coworking con puestos flexibles, fijos y despachos privados, servicios incluidos y visita con fecha.",
                     "Coworking space with hot desks, dedicated desks and private offices, included services and visits by date."), [
        hero(("Un sitio para trabajar, sin montar una oficina", "A place to work, without setting up an office"),
             ("Mesa, wifi, sala de reuniones y café desde un pase de día. Cuando necesites algo más fijo, hay puesto propio o despacho con llave.",
              "A desk, wifi, a meeting room and coffee from a day pass. When you need something more permanent, there are dedicated desks and private offices with a key."),
             [visita(), llamar("secondary"), whatsapp("ghost")],
             eyebrow=("Espacio de coworking", "Coworking space"),
             imagen_=("photo", ("Zona de trabajo compartida con mesas largas y luz natural", "Shared work area with long tables and natural light")),
             tono="relleno"),
        bloque_valores(("Tres formas de trabajar aquí", "Three ways to work here"), [
            (("Puesto flexible", "Hot desk"),
             ("Te sientas donde haya sitio libre. Para quien trabaja con portátil y no necesita dejar nada.",
              "You sit wherever there is a free spot. For people who work on a laptop and do not need to leave anything.")),
            (("Puesto fijo", "Dedicated desk"),
             ("Tu mesa es tuya cada día, con cajonera y taquilla. Dejas tus cosas y las encuentras como las dejaste.",
              "Your desk is yours every day, with a drawer unit and locker. You leave your things and find them as you left them.")),
            (("Despacho privado", "Private office"),
             ("Una puerta propia para tu equipo, con las zonas comunes y la sala de reuniones a mano.",
              "A door of your own for your team, with the shared areas and meeting room close by.")),
        ], eyebrow=("Puestos", "Desks"), estilo="borde", tono="base", anchor="formas"),
        bloque_checks(("Ya incluido en la cuota", "Already included in the fee"), [
            ("Wifi de fibra para todo el espacio", "Fibre wifi throughout the space"),
            ("Café, té y agua", "Coffee, tea and water"),
            ("Salas de reuniones, con horas incluidas según el puesto", "Meeting rooms, with hours included depending on the desk"),
            ("Taquillas con llave", "Lockers with a key"),
            ("Cocina, nevera y microondas", "Kitchen, fridge and microwave"),
            ("Impresora y escáner", "Printer and scanner"),
        ], intro=("Cámbialo por lo que de verdad incluye tu espacio: aquí solo van cosas que ofrezcas.",
                  "Replace this with what your space actually includes: only list things you offer."),
            tono="relleno", cols=2, anchor="incluido"),
        bloque_pasos(("De la visita al primer día", "From the visit to your first day"), [
            (("Ven a verlo", "Come and see it"),
             ("Reserva una visita con fecha. Te enseñamos las zonas y te dejamos probar una mesa.",
              "Book a visit with a date. We show you the areas and let you try a desk.")),
            (("Elige tu puesto", "Choose your desk"),
             ("Pase de día, flexible, fijo o despacho. Te decimos cuál encaja con tu forma de trabajar.",
              "Day pass, hot desk, dedicated desk or office. We tell you which one suits the way you work.")),
            (("Empieza", "Get started"),
             ("Firmas el alta, te damos tu acceso y entras el día que acordemos.",
              "You sign up, we give you your access and you start on the day we agree.")),
        ], tono="base"),
        bloque_cta(("¿Pasamos a verlo juntos?", "Shall we take a look together?"),
                   ("Elige un día y te enseñamos el espacio sin compromiso.",
                    "Pick a day and we will show you the space with no obligation."),
                   [visita(), llamar("ghost")], tono="relleno"),
    ])

    puestos = pagina("puestos", ("puestos", "desks"), ("Puestos y tarifas", "Desks and prices"),
                     ("Tarifas de ejemplo de pase de día, puesto flexible, fijo y despacho privado, qué incluye cada uno y preguntas frecuentes.",
                      "Example prices for day pass, hot desk, dedicated desk and private office, what each includes, and common questions."), [
        hero(("Puestos y tarifas", "Desks and prices"),
             ("Elige por cuántos días vas a venir y si necesitas dejar tus cosas. Todo incluye wifi, café y zonas comunes.",
              "Choose by how many days you will come and whether you need to leave your things. Everything includes wifi, coffee and shared areas."),
             [visita()], variante="texto"),
        bloque_precios(("Tarifas", "Prices"), ("Tarifas de ejemplo", "Example prices"),
                       [("Puesto", "Desk"), ("Para quién", "Who it suits"), ("Tarifa", "Price")],
                       [(("Pase de día", "Day pass"), ("Probar o venir de vez en cuando", "Trying it out or dropping in now and then"), (f"{eur(15)[0]} al día", f"{eur(15)[1]} a day")),
                        (("Puesto flexible", "Hot desk"), ("Venir a diario sin mesa propia", "Coming daily without your own desk"), (f"{eur(120)[0]} al mes", f"{eur(120)[1]} a month")),
                        (("Puesto fijo", "Dedicated desk"), ("Una mesa tuya con taquilla", "Your own desk with a locker"), (f"{eur(190)[0]} al mes", f"{eur(190)[1]} a month")),
                        (("Despacho privado, 2 puestos", "Private office, 2 desks"), ("Dos personas con puerta propia", "Two people with a door of their own"), (f"{eur(480)[0]} al mes", f"{eur(480)[1]} a month")),
                        (("Despacho privado, 4 puestos", "Private office, 4 desks"), ("Un equipo pequeño", "A small team"), (f"{eur(850)[0]} al mes", f"{eur(850)[1]} a month")),
                        (("Sala de reuniones", "Meeting room"), ("Para quien no es socio o se pasa de horas", "For non-members or when you use up your hours"), (f"{eur(12)[0]} por hora", f"{eur(12)[1]} an hour"))],
                       nota=("Precios de ejemplo: cámbialos por los tuyos. Indica aquí si llevan IVA y si hay fianza o permanencia.",
                             "Example prices: replace them with your own. Say here whether VAT is included and whether there is a deposit or minimum term."),
                       tono="base"),
        bloque_valores(("Qué cambia de un puesto a otro", "What differs between desks"), [
            (("Flexible", "Hot desk"), ("Sitio libre cada día. Taquilla compartida si la pides. Salas por horas.",
                                         "A free spot each day. Shared locker on request. Rooms by the hour.")),
            (("Fijo", "Dedicated"), ("Mesa y taquilla tuyas. Acceso en el horario del espacio. Horas de sala incluidas.",
                                      "Your own desk and locker. Access during the space's opening hours. Meeting room hours included.")),
            (("Despacho", "Office"), ("Puerta con llave. Rótulo con el nombre de tu empresa. Sala de reuniones con descuento.",
                                       "Door with a key. Sign with your company name. Meeting room at a discount.")),
        ], intro=("Cada punto es de ejemplo: cámbialo por lo que de verdad tenga cada puesto.",
                  "Each point is an example: change it to what each desk really has."),
            estilo="relleno", tono="relleno", anchor="diferencias"),
        bloque_faq(("Preguntas frecuentes", "Frequently asked questions"), [
            (("¿Tengo que ser empresa o autónomo?", "Do I have to be a company or self-employed?"),
             ("No. Vienen autónomos, equipos pequeños, gente que teletrabaja para una empresa y estudiantes. Lo que importa es que trabajes y dejes trabajar.",
              "No. Members include freelancers, small teams, remote employees and students. What matters is that you work and let others work.")),
            (("¿Puedo usar la dirección del espacio para mi empresa?", "Can I use the space's address for my company?"),
             ("Pregúntalo en la visita: depende del contrato y del tipo de puesto. Indica aquí si ofreces domicilio social o buzón.",
              "Ask during the visit: it depends on the contract and the type of desk. Say here whether you offer a registered address or mailbox.")),
            (("¿Puedo traer a un cliente?", "Can I bring a client?"),
             ("Sí, a la sala de reuniones o a la zona de visitas. Avísanos antes para tenerla libre.",
              "Yes, to the meeting room or the visitors' area. Let us know beforehand so it is free.")),
            (("¿Puedo cambiar de puesto más adelante?", "Can I change desk later on?"),
             ("Sí, cuando haya plaza libre. Muchos empiezan con un pase de día y acaban en un puesto fijo.",
              "Yes, when a place is free. Many start with a day pass and end up with a dedicated desk.")),
            (("¿Hay sitio para hacer llamadas?", "Is there somewhere to take calls?"),
             ("En la zona de trabajo pedimos hablar en voz baja. Para llamadas largas hay cabinas o salas.",
              "In the work area we ask people to keep their voices low. For longer calls there are booths or rooms.")),
        ], tono="base"),
        bloque_cta(("¿Qué puesto te encaja?", "Which desk suits you?"), None, [visita(), whatsapp("ghost")],
                   tono="relleno"),
    ])

    espacio = pagina("espacio", ("el-espacio", "the-space"), ("El espacio", "The space"),
                     ("Servicios del coworking, comunidad y eventos, horario de acceso y normas básicas de convivencia.",
                      "Coworking services, community and events, access hours and basic house rules."), [
        hero(("El espacio", "The space"),
             ("Un sitio pensado para concentrarse sin aislarse. Mesas amplias, luz natural y gente con la que cruzar dos palabras en el café.",
              "A place designed to focus without being cut off. Big tables, natural light and people to exchange a few words with over coffee."),
             [visita(), llamar("secondary")], variante="texto"),
        bloque_valores(("Servicios", "Services"), [
            (("Wifi", "Wifi"), ("Fibra para todo el espacio, con red aparte para visitas.",
                                "Fibre for the whole space, with a separate network for visitors.")),
            (("Salas de reuniones", "Meeting rooms"), ("Se reservan por horas. Pantalla, pizarra y mesa para varias personas.",
                                                       "Booked by the hour. Screen, whiteboard and a table for several people.")),
            (("Café y cocina", "Coffee and kitchen"), ("Café, té y agua, nevera y microondas para comer sin salir.",
                                                       "Coffee, tea and water, plus a fridge and microwave to eat without going out.")),
            (("Taquillas", "Lockers"), ("Con llave, para dejar el portátil o la mochila.",
                                        "With a key, to leave your laptop or bag.")),
            (("Impresión", "Printing"), ("Impresora y escáner para lo puntual. Las copias grandes se cobran aparte.",
                                         "Printer and scanner for occasional use. Large print runs are charged separately.")),
            (("Zona de descanso", "Break area"), ("Un rincón con sofás para parar un rato sin molestar a los demás.",
                                                  "A corner with sofas to stop for a while without disturbing others.")),
        ], intro=("Servicios de ejemplo: cámbialos por los que tenga tu espacio y borra los que no.",
                  "Example services: replace them with those your space has and delete the rest."),
            eyebrow=("Servicios incluidos", "Included services"), estilo="relleno", tono="relleno", anchor="servicios"),
        bloque_texto(("Comunidad y eventos", "Community and events"), [
            ("Trabajar con otras personas se nota. Se comparten contactos, dudas y a veces proyectos. Nadie está obligado a nada: quien quiere charlar, charla, y quien quiere callar, calla.",
             "Working alongside other people makes a difference. Contacts, questions and sometimes projects are shared. Nobody is obliged to do anything: those who want to chat, chat, and those who want quiet, have it."),
            ("Escribe aquí qué encuentros organizáis de verdad: desayunos, charlas, afterwork. Si no hay ninguno, borra este párrafo.",
             "Write here which events you really run: breakfasts, talks, after-work drinks. If there are none, delete this paragraph."),
        ], imagen_=("landscape", ("Socios charlando junto a la máquina de café", "Members chatting by the coffee machine")),
            eyebrow=("Comunidad", "Community"), tono="base", invertir=True, anchor="comunidad"),
        bloque_precios(("Cuándo se puede entrar", "When you can come in"), ("Horario de acceso de ejemplo", "Example access hours"),
                       [("Día", "Day"), ("Horas", "Hours")], acceso,
                       intro=("Es el horario de atención. Si tu espacio da acceso fuera de él, indica aquí cómo.",
                              "These are staffed hours. If your space gives access outside them, say here how."),
                       nota=("Horario de ejemplo: cámbialo por el tuyo.", "Example hours: replace them with your own."),
                       tono="relleno", anchor="acceso", kind="text"),
        bloque_checks(("Cómo convivimos", "How we get along"), [
            ("Hablar en voz baja en la zona de trabajo.", "Keeping voices low in the work area."),
            ("Llamadas y vídeo en las cabinas o salas.", "Calls and video in the booths or rooms."),
            ("Dejar la mesa y la cocina como las encontraste.", "Leaving the desk and kitchen as you found them."),
            ("Avisar de las visitas con antelación.", "Letting us know about visitors in advance."),
        ], tono="base", cols=2),
        bloque_cta(("Se entiende mejor en persona", "It is easier to understand in person"), None,
                   [visita(), whatsapp("ghost")], tono="relleno"),
    ])

    vis = pagina("visita", ("visita", "visit"), ("Visita y cómo llegar", "Visit and getting here"),
                 ("Reserva una visita con fecha, horario de acceso, mapa y cómo llegar en transporte público, bici o coche.",
                  "Book a visit by date, see access hours, a map and how to get here by public transport, bike or car."), [
        hero(("Reserva tu visita", "Book your visit"),
             ("Dinos qué día te viene bien y qué tipo de puesto buscas. Te confirmamos la hora y te enseñamos todo, sin compromiso.",
              "Tell us which day suits you and what kind of desk you are after. We confirm the time and show you around, with no obligation."),
             [llamar(), whatsapp()], variante="texto"),
        con_titulo_de_tabla(
            bloque_contacto(("Cómo encontrarnos", "How to find us"),
                            ("La visita dura unos 30 minutos. Puedes traer a tu equipo.",
                             "A visit takes about 30 minutes. You can bring your team."),
                            campos_cita(servicios=puestos_opc,
                                        extra=[campo("personas", "select", ("Cuántas personas sois", "How many of you"), False,
                                                     opciones=[("Solo yo", "Just me"), ("De 2 a 4", "2 to 4"), ("5 o más", "5 or more")])]),
                            ("Pedir visita", "Request a visit"), acceso,
                            exito=("Gracias. Te confirmamos la visita por teléfono o WhatsApp en cuanto la tengamos.",
                                   "Thank you. We will confirm your visit by phone or WhatsApp as soon as we have it.")),
            ("Horario de acceso al espacio", "Space access hours")),
        bloque_mapa(texto_=("Indica aquí en qué planta estamos y cómo se abre el portal.",
                            "Say here which floor we are on and how to open the main door."), tono="relleno"),
        bloque_valores(("Cómo llegar", "Getting here"), [
            (("Transporte público", "Public transport"), ("Indica aquí las líneas de metro, tren o autobús y la parada más cercana.",
                                                          "Say here the metro, train or bus lines and the nearest stop.")),
            (("En bici", "By bike"), ("Indica aquí si hay aparcabicis o sitio para guardarla dentro.",
                                      "Say here whether there are bike racks or space to keep it inside.")),
            (("En coche", "By car"), ("Indica aquí dónde aparcar cerca y si hay aparcamiento con tarifa para socios.",
                                      "Say here where to park nearby and whether there is parking at a members' rate.")),
        ], eyebrow=("Transporte", "Transport"), estilo="linea", tono="base", anchor="transporte"),
    ])

    return plantilla(KEY, ("Coworking", "Coworking space"),
                     ("Espacio de coworking con puestos flexibles, fijos y despachos, tarifas, servicios incluidos, visita con fecha, horario de acceso y cómo llegar.",
                      "Coworking space with hot desks, dedicated desks and offices, prices, included services, visits by date, access hours and directions."),
                     "comunidad", ("coworking cowork espacio de trabajo compartido oficina compartida puesto flexible puesto fijo despacho privado sala de reuniones alquiler de mesa autonomos freelance",
                                   "coworking cowork shared office shared workspace hot desk dedicated desk private office meeting room desk rental freelancers"),
                     look, cabecera(nav, marco([llamar(), whatsapp("ghost")], dir="row", gap=12, wrap=True)),
                     pie(nav, ("Coworking. Puestos flexibles, fijos y despachos con wifi, café y salas de reuniones.",
                               "Coworking space. Hot desks, dedicated desks and offices with wifi, coffee and meeting rooms."),
                         ("Lunes a viernes 8:00 a 21:00. Sábado 9:00 a 14:00.",
                          "Monday to Friday 8 am to 9 pm. Saturday 9 am to 2 pm.")),
                     [inicio, puestos, espacio, vis])
