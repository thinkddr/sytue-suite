# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
from lib import *

KEY = "parroquia-comunidad"


def _c(background, surface, text, muted, primary, on_primary, border, inverse, on_inverse):
    return dict(background=background, surface=surface, text=text, muted=muted, primary=primary,
                on_primary=on_primary, border=border, inverse=inverse, on_inverse=on_inverse)


# Paleta propia: pergamino y bronce. No se usa fondo $inverse (el bronce no llega a 3 sobre oscuro).
BRONCE = dict(colores=_c("#fbf8f2", "#f0ebdf", "#2a2519", "#5e5848", "#7a5416", "#ffffff", "#e0d8c4", "#2b2618", "#f7f1e2"),
              fuentes=("source-serif-4", "noto-sans"), radio=(4, 8, 12), ancho=1080, h1=(48, 38, 30), peso=600)


def con_titulo_de_tabla(nodo, nuevo):
    """Cambia el título de la tabla de horario que lleva bloque_contacto (por defecto «Horario de apertura»)."""
    if nodo.get("type") == "table":
        nodo["caption"] = L(nuevo)
    for h in nodo.get("children", []):
        con_titulo_de_tabla(h, nuevo)
    return nodo


def build():
    nueva(KEY)
    look = BRONCE
    malos = comprobar_paleta("bronce", look["colores"])
    assert not malos, malos
    nav = [("inicio", ("Inicio", "Home")), ("celebraciones", ("Celebraciones", "Services")),
           ("comunidad", ("Comunidad", "Community")), ("contacto", ("Contacto", "Contact"))]
    secretaria = [(("Lunes a viernes", "Monday to Friday"), ("10:00 a 13:00 y 17:00 a 19:00", "10:00 am to 1:00 pm and 5:00 pm to 7:00 pm")),
                  (("Sábado", "Saturday"), ("10:00 a 12:00", "10:00 am to 12:00 pm")),
                  (("Domingo", "Sunday"), ("Cerrado (tras las celebraciones)", "Closed (after the services)"))]
    nota_horas = ("Horarios de ejemplo: cámbialos por los de tu comunidad.",
                  "Example times: replace them with your community's own.")

    def solicitar(var="primary"):
        return boton(("Hablar con la secretaría", "Talk to the parish office"), a_pagina("contacto", "contacto"), var)

    def celebraciones_tabla(titulo, intro=None):
        return seccion("text", [
            titular(titulo, intro),
            tabla(("Horario de celebraciones de ejemplo", "Example service times"),
                  [("Cuándo", "When"), ("Celebración", "Service"), ("Hora", "Time")],
                  [(("Lunes a viernes", "Monday to Friday"), ("Misa de tarde", "Evening Mass"), ("19:30", "7:30 pm")),
                   (("Sábado", "Saturday"), ("Misa de vísperas", "Vigil Mass"), ("19:30", "7:30 pm")),
                   (("Domingos y festivos", "Sundays and holidays"), ("Misa de la mañana", "Morning Mass"), ("10:00", "10:00 am")),
                   (("Domingos y festivos", "Sundays and holidays"), ("Misa de mediodía", "Midday Mass"), ("12:00", "12:00 pm")),
                   (("Domingos y festivos", "Sundays and holidays"), ("Misa de la tarde", "Evening Mass"), ("19:00", "7:00 pm")),
                   (("Antes de cada misa", "Before each Mass"), ("Confesiones", "Confession"), ("30 minutos antes", "30 minutes before")),
                   (("Primer viernes de mes", "First Friday of the month"), ("Adoración", "Adoration"), ("18:30", "6:30 pm"))]),
            peq(nota_horas, color="$muted"),
        ], anchor="horario", gap=24)

    inicio = pagina("inicio", ("", ""), ("Parroquia y comunidad", "Parish and community"),
                    ("Comunidad de fieles abierta a todos: horarios de celebraciones, sacramentos, grupos, secretaría y cómo ayudar.",
                     "A community open to everyone: service times, sacraments, groups, parish office and how to help."), [
        hero(("Una casa abierta a todos", "A home open to everyone"),
             ("Aquí encontrarás los horarios de las celebraciones, cómo pedir un sacramento y cómo acercarte a los grupos de la comunidad. Si es tu primera vez, eres bienvenido.",
              "Here you will find service times, how to ask for a sacrament and how to join the community's groups. If it is your first time, you are welcome."),
             [boton(("Ver horarios", "See times"), a_ancla("horario")), solicitar("secondary"), llamar("ghost")],
             eyebrow=("Comunidad parroquial", "Parish community"), variante="centro"),
        celebraciones_tabla(("Horarios de celebraciones", "Service times"),
                            ("Los horarios cambian en fiestas y en verano. Te avisamos en esta página y en el tablón de la entrada.",
                             "Times change on feast days and in summer. We post changes on this page and on the notice board at the entrance.")),
        bloque_texto(("Si vienes por primera vez", "If you are coming for the first time"), [
            ("No hace falta saber nada ni pedir permiso: entra, siéntate donde quieras y, si lo deseas, quédate al final a saludar. La gente de la comunidad estará encantada de acompañarte.",
             "You do not need to know anything or ask permission: come in, sit wherever you like and, if you wish, stay at the end to say hello. People in the community will be glad to help."),
            ("Si buscas un rato de silencio, el templo está abierto fuera de las celebraciones en el horario de la secretaría.",
             "If you are looking for a quiet moment, the church is open outside services during the parish office hours."),
        ], imagen_=("landscape", ("Interior de la iglesia con luz de la tarde", "Inside the church in the afternoon light")),
            eyebrow=("Bienvenida", "Welcome"), tono="relleno", invertir=True),
        bloque_valores(("Lo que se vive en la comunidad", "Life in the community"), [
            (("Celebrar", "Celebrate"),
             ("La misa de cada semana, las fiestas del año y los momentos importantes de cada familia.",
              "Weekly Mass, the feasts of the year and the important moments in each family.")),
            (("Formarse", "Learn"),
             ("Catequesis para niños, jóvenes y adultos, y charlas para quien quiera preguntar.",
              "Catechesis for children, young people and adults, and talks for anyone who wants to ask questions.")),
            (("Ayudar", "Help"),
             ("Voluntariado, colectas y acompañamiento a las personas del barrio que lo necesitan.",
              "Volunteering, collections and support for neighbours who need it.")),
        ], eyebrow=("La comunidad", "The community"), estilo="linea", tono="base"),
        bloque_valores(("Avisos de esta semana", "This week's notices"), [
            (("Reunión o encuentro", "Meeting or gathering"),
             ("Aviso de ejemplo: cámbialo por la próxima reunión, con día, hora y lugar.",
              "Example notice: replace it with the next meeting, with day, time and place.")),
            (("Colecta o campaña", "Collection or campaign"),
             ("Aviso de ejemplo: cámbialo por la campaña en marcha y cómo colaborar.",
              "Example notice: replace it with the current campaign and how to take part.")),
        ], cols=2, estilo="borde", tono="relleno", kind="features", anchor="avisos"),
        bloque_cta(("¿Necesitas hablar con alguien?", "Do you need to talk to someone?"),
                   ("La secretaría parroquial te atiende en persona, por teléfono o por mensaje.",
                    "The parish office will help you in person, by phone or by message."),
                   [solicitar(), llamar("ghost")], tono="base"),
    ])

    celebr = pagina("celebraciones", ("celebraciones", "services"), ("Celebraciones y sacramentos", "Services and sacraments"),
                    ("Horario de misas y confesiones, los sacramentos y cómo solicitarlos, y respuestas a las dudas más habituales.",
                     "Mass and confession times, the sacraments and how to ask for them, and answers to common questions."), [
        hero(("Celebraciones y sacramentos", "Services and sacraments"),
             ("Cuándo nos reunimos y cómo pedir un bautizo, una comunión, una boda o cualquier otro sacramento.",
              "When we meet and how to ask for a baptism, a first communion, a wedding or any other sacrament."),
             [solicitar()], variante="texto"),
        celebraciones_tabla(("Horario semanal", "Weekly times")),
        bloque_valores(("Los sacramentos", "The sacraments"), [
            (("Bautismo", "Baptism"), ("Para niños y para adultos. Antes hay una charla con la familia o con la persona que se bautiza.",
                                       "For children and adults. Beforehand there is a talk with the family or the person being baptised.")),
            (("Primera comunión", "First communion"), ("Tras un tiempo de catequesis con otros niños de la comunidad.",
                                                       "After a period of catechesis with other children in the community.")),
            (("Confirmación", "Confirmation"), ("Para jóvenes y adultos que quieren completar su iniciación, con un camino de preparación.",
                                                "For young people and adults who want to complete their initiation, with a period of preparation.")),
            (("Matrimonio", "Marriage"), ("Conviene avisar con meses de antelación: hay charlas de preparación y papeles que reunir.",
                                          "It is best to let us know months ahead: there are preparation talks and papers to gather.")),
            (("Reconciliación", "Reconciliation"), ("La confesión, antes de cada misa o cuando lo pidas con tiempo.",
                                                    "Confession, before each Mass or whenever you ask in advance.")),
            (("Unción de enfermos", "Anointing of the sick"), ("Para quien está enfermo o muy mayor. Llama a la secretaría o a la persona de guardia.",
                                                               "For those who are ill or very old. Call the parish office or the person on call.")),
        ], intro=("Pedir un sacramento es hablar con alguien, no rellenar un trámite. Empieza por la secretaría.",
                  "Asking for a sacrament means talking to someone, not filling in a procedure. Start with the parish office."),
            eyebrow=("Sacramentos", "Sacraments"), estilo="relleno", tono="relleno", anchor="sacramentos"),
        bloque_pasos(("Cómo solicitar un sacramento", "How to ask for a sacrament"), [
            (("Habla con la secretaría", "Talk to the parish office"),
             ("Por teléfono, en persona o con el formulario. Dinos qué sacramento y para quién es.",
              "By phone, in person or with the form. Tell us which sacrament and who it is for.")),
            (("Te explicamos el camino", "We explain the path"),
             ("Cada sacramento tiene su preparación: charlas, catequesis o encuentros. Te decimos cuánto dura y cuándo empieza.",
              "Each sacrament has its preparation: talks, catechesis or meetings. We tell you how long it lasts and when it starts.")),
            (("Reúnes los papeles", "You gather the papers"),
             ("La secretaría te indica qué documentos hacen falta. Varían según el sacramento y la persona.",
              "The parish office tells you which documents are needed. They vary by sacrament and by person.")),
            (("Fijamos la fecha", "We set the date"),
             ("Con la preparación hecha, acordamos el día y la hora de la celebración.",
              "Once the preparation is done, we agree the day and time of the celebration.")),
        ], tono="base", anchor="solicitar"),
        bloque_faq(("Preguntas frecuentes", "Frequently asked questions"), [
            (("¿Hace falta vivir en la parroquia?", "Do I need to live in the parish?"),
             ("No para ir a las celebraciones. Para pedir un sacramento, la secretaría te dirá si hace falta algún trámite según tu caso.",
              "Not to attend services. To ask for a sacrament, the parish office will tell you whether any step is needed in your case.")),
            (("¿Puedo pedir una misa por una intención?", "Can I ask for a Mass for an intention?"),
             ("Sí. Pásate por la secretaría o escríbenos, y te decimos qué días hay hueco. Indica aquí si se acepta un donativo orientativo.",
              "Yes. Call at the parish office or write to us, and we will tell you which days are free. Say here whether a suggested donation is accepted.")),
            (("¿Se puede entrar con silla de ruedas o carrito?", "Can I come in with a wheelchair or pushchair?"),
             ("Indica aquí si el templo tiene rampa o entrada accesible y por dónde se entra.",
              "Say here whether the church has a ramp or accessible entrance and where to go in.")),
            (("¿Puedo visitar el templo fuera de las celebraciones?", "Can I visit the church outside services?"),
             ("Está abierto en el horario de la secretaría. Si vienes en grupo, avísanos con unos días para recibiros.",
              "It is open during parish office hours. If you come as a group, let us know a few days ahead so we can welcome you.")),
        ], tono="relleno"),
        bloque_cta(("¿Quieres pedir un sacramento?", "Would you like to ask for a sacrament?"), None,
                   [solicitar(), whatsapp("ghost")], tono="base"),
    ])

    comunidad = pagina("comunidad", ("comunidad", "community"), ("Grupos y cómo ayudar", "Groups and how to help"),
                       ("Grupos y actividades de la comunidad, y las maneras de colaborar: voluntariado, donativos y acción social.",
                        "The community's groups and activities, and ways to help: volunteering, donations and social action."), [
        hero(("La comunidad", "The community"),
             ("Grupos para cada edad y maneras de echar una mano, grandes o pequeñas. Cada uno aporta lo que puede.",
              "Groups for every age and ways to lend a hand, big or small. Everyone gives what they can."),
             [solicitar()], variante="texto"),
        bloque_valores(("Grupos y actividades", "Groups and activities"), [
            (("Catequesis infantil", "Children's catechesis"), ("Niños por edades, una tarde a la semana, con juegos y canciones.",
                                                               "Children by age, one afternoon a week, with games and songs.")),
            (("Jóvenes", "Young people"), ("Encuentros para charlar, hacer proyectos y salir juntos.",
                                          "Meetings to talk, run projects and go out together.")),
            (("Coro", "Choir"), ("Para cantar en las celebraciones. No hace falta saber música.",
                                 "To sing at services. You do not need to read music.")),
            (("Mayores", "Older people"), ("Un rato de compañía, café y conversación cada semana.",
                                          "Some company, coffee and conversation each week.")),
            (("Lectura y oración", "Reading and prayer"), ("Un grupo pequeño que se reúne a leer y a rezar en común.",
                                                          "A small group that meets to read and pray together.")),
            (("Matrimonios y familias", "Couples and families"), ("Encuentros para compartir lo que supone criar y convivir.",
                                                                 "Meetings to share what raising a family and living together involves.")),
        ], intro=("Grupos de ejemplo: cámbialos por los de tu comunidad y pon día y hora de cada uno.",
                  "Example groups: replace them with those of your community and add the day and time of each."),
            eyebrow=("Grupos", "Groups"), estilo="borde", tono="base", anchor="grupos"),
        bloque_texto(("Cómo ayudar", "How to help"), [
            ("Hay muchas formas de colaborar, y no todas son dinero. Puedes dar un rato de tu tiempo, un saber que tengas o una ayuda puntual en una campaña.",
             "There are many ways to help, and not all of them involve money. You can give some of your time, a skill you have or occasional help with a campaign."),
            ("Escribe aquí cómo se puede colaborar con la acción social de la comunidad, por ejemplo con el grupo de Cáritas parroquial si existe.",
             "Write here how people can help with the community's social action, for example with the parish Cáritas group if there is one."),
        ], imagen_=("photo", ("Voluntarios preparando cajas con alimentos", "Volunteers packing boxes of food")),
            eyebrow=("Colaborar", "Get involved"), tono="relleno", anchor="ayudar"),
        bloque_checks(("Maneras de colaborar", "Ways to help"), [
            ("Voluntariado: acompañar, repartir o preparar.", "Volunteering: accompanying, handing out or preparing."),
            ("Donativos para el mantenimiento y la acción social.", "Donations for upkeep and social action."),
            ("Colectas de alimentos, ropa o material escolar.", "Collections of food, clothes or school supplies."),
            ("Ayuda en el templo: limpieza, flores, lecturas.", "Help in the church: cleaning, flowers, readings."),
            ("Dar a conocer a alguien que necesita ayuda.", "Letting us know about someone who needs help."),
            ("Rezar por la comunidad.", "Praying for the community."),
        ], intro=("Indica aquí el número de cuenta o el modo de hacer un donativo. Nunca pidas datos de tarjeta por el formulario.",
                  "Say here the bank account or way to make a donation. Never ask for card details through the form."),
            tono="base", cols=2, anchor="colaborar"),
        bloque_cta(("¿Te apuntas a algo?", "Would you like to join something?"),
                   ("Dinos qué grupo o qué ayuda te interesa y te ponemos en contacto con quien lo lleva.",
                    "Tell us which group or kind of help interests you and we will put you in touch with the person who runs it."),
                   [solicitar(), whatsapp("ghost")], tono="relleno"),
    ])

    cont = pagina("contacto", ("contacto", "contact"), ("Contacto y secretaría", "Contact and parish office"),
                  ("Horario de la secretaría parroquial, teléfono, WhatsApp, formulario y cómo llegar a la iglesia.",
                   "Parish office hours, phone, WhatsApp, a form and how to get to the church."), [
        hero(("Secretaría parroquial", "Parish office"),
             ("Para pedir un sacramento, un certificado o una misa por una intención, o simplemente para preguntar. Te atendemos con calma.",
              "To ask for a sacrament, a certificate or a Mass for an intention, or simply to ask a question. We will help you without hurry."),
             [llamar(), whatsapp()], variante="texto"),
        con_titulo_de_tabla(
            bloque_contacto(("Cómo encontrarnos", "How to find us"),
                            ("Fuera del horario de la secretaría, deja un mensaje y te devolvemos la llamada.",
                             "Outside parish office hours, leave a message and we will call you back."),
                            campos_consulta(mensaje_etiqueta=("Cuéntanos qué necesitas", "Tell us what you need"),
                                            servicios=[("Pedir un sacramento", "Ask for a sacrament"),
                                                       ("Pedir un certificado", "Ask for a certificate"),
                                                       ("Misa por una intención", "Mass for an intention"),
                                                       ("Unirme a un grupo o colaborar", "Join a group or help"),
                                                       ("Otra consulta", "Something else")]),
                            ("Enviar mensaje", "Send message"), secretaria,
                            notas=("No hace falta que cuentes nada de tus creencias. Usamos tus datos solo para responderte.",
                                   "You do not need to tell us anything about your beliefs. We use your details only to reply to you."),
                            exito=("Gracias. La secretaría te responderá en cuanto pueda.",
                                   "Thank you. The parish office will reply as soon as it can.")),
            ("Horario de la secretaría parroquial", "Parish office hours")),
        bloque_mapa(texto_=("Indica aquí dónde está la entrada y dónde se puede aparcar o dejar la bici.",
                            "Say here where the entrance is and where to park or leave a bike."), tono="relleno"),
    ])

    return plantilla(KEY, ("Parroquia o comunidad", "Parish or faith community"),
                     ("Sitio de una parroquia o comunidad religiosa: horarios de celebraciones, sacramentos, grupos, cómo ayudar, secretaría con formulario y botones de llamar y WhatsApp.",
                      "Website for a parish or faith community: service times, sacraments, groups, how to help, parish office with a form and call and WhatsApp buttons."),
                     "comunidad", ("parroquia iglesia comunidad religiosa comunidad de fieles misa misas horario de misas sacramentos bautizo comunion confirmacion boda catequesis capilla secretaria parroquial caritas voluntariado",
                                   "parish church faith community mass service times sacraments baptism communion confirmation wedding catechesis chapel parish office volunteering"),
                     look, cabecera(nav, marco([llamar(), whatsapp("ghost")], dir="row", gap=12, wrap=True)),
                     pie(nav, ("Una comunidad abierta a todos. Celebraciones, sacramentos, grupos y ayuda a quien lo necesita.",
                               "A community open to everyone. Services, sacraments, groups and help for those in need."),
                         ("Secretaría: lunes a viernes 10:00 a 13:00 y 17:00 a 19:00. Sábado 10:00 a 12:00.",
                          "Parish office: Monday to Friday 10 am to 1 pm and 5 pm to 7 pm. Saturday 10 am to 12 pm.")),
                     [inicio, celebr, comunidad, cont])
