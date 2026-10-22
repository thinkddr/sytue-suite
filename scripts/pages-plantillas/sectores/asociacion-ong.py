# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
from lib import *
from looks import LOOKS

KEY = "asociacion-ong"

# Paleta propia (fucsia sobre blanco cálido); pasa AA. Sin fondo $inverse: primary/inverse < 3.
FUCSIA = dict(background="#fffcfa", surface="#fbeaf2", text="#2a1522", muted="#6a4c5f", primary="#a8206f",
              on_primary="#ffffff", border="#f0d3e2", inverse="#3a1230", on_inverse="#fff0f8")


def build():
    nueva(KEY)
    look = dict(colores=FUCSIA, fuentes=("work-sans", "noto-sans"), radio=(8, 14, 20))
    malos = comprobar_paleta("asociacion-ong", FUCSIA)
    assert not malos, malos
    nav = [("inicio", ("Inicio", "Home")), ("colabora", ("Cómo colaborar", "How to help")),
           ("asociacion", ("La asociación", "The association")), ("contacto", ("Voluntariado y contacto", "Volunteering and contact"))]
    horas = [(("Lunes a jueves", "Monday to Thursday"), ("17:00 a 20:00", "5:00 pm to 8:00 pm")),
             (("Viernes", "Friday"), ("10:00 a 13:00", "10:00 am to 1:00 pm")),
             (("Sábado y domingo", "Saturday and Sunday"), ("Cerrado, salvo actividades", "Closed, except for activities"))]
    registro = ("Asociación inscrita en el registro con el nº 00-0000.",
                "Association registered under no. 00-0000.")

    def colabora(var="primary"):
        return boton(("Quiero colaborar", "I want to help"), a_pagina("colabora"), var)

    inicio = pagina("inicio", ("", ""), ("Asociación", "Association"),
                    ("Asociación sin ánimo de lucro: misión, proyectos y formas de colaborar como socio, voluntario o con un donativo.",
                     "Non-profit association: mission, projects and ways to help as a member, volunteer or with a donation."), [
        hero(("Un barrio mejor se hace entre todos", "A better neighbourhood is built by everyone"),
             ("Somos una asociación de personas que trabajamos por nuestra comunidad. Hay sitio para ti: como socio, como voluntario o con un donativo.",
              "We are an association of people working for our community. There is room for you: as a member, a volunteer or with a donation."),
             [colabora(), llamar("secondary"), whatsapp("ghost")],
             eyebrow=("Asociación sin ánimo de lucro", "Non-profit association"), variante="centro", tono="relleno"),
        bloque_texto(("Nuestra misión", "Our mission"), [
            ("Escribe aquí en dos o tres frases para qué existe tu asociación y a quién ayuda. Texto de ejemplo: cámbialo por el vuestro.",
             "Write here, in two or three sentences, what your association is for and who it helps. Example text: replace it with your own."),
            ("Trabajamos con personas del barrio, con respeto, sin juzgar y contando con lo que cada uno sabe y puede hacer.",
             "We work with people from the neighbourhood, with respect, without judging and counting on what each person knows and can do."),
        ], eyebrow=("Quiénes somos", "Who we are"), imagen_=("photo", ("Personas de la asociación reunidas en una actividad del barrio", "Association members gathered at a neighbourhood activity")),
            tono="base"),
        bloque_valores(("Nuestros proyectos", "Our projects"), [
            (("Actividades con vecinos", "Activities with neighbours"),
             ("Encuentros, comidas y salidas para conocernos y que nadie esté solo.",
              "Meetings, shared meals and outings so we get to know each other and nobody is alone.")),
            (("Talleres y formación", "Workshops and training"),
             ("Cursos cortos y gratuitos o a precio de coste, impartidos por voluntarios.",
              "Short courses, free or at cost price, given by volunteers.")),
            (("Acompañamiento", "Support"),
             ("Una persona que escucha y orienta a quien lo necesita, con trámites, dudas o simplemente compañía.",
              "Someone who listens and guides whoever needs it, with paperwork, questions or simply company.")),
            (("Sensibilización", "Awareness"),
             ("Charlas y actos abiertos para dar a conocer la causa que defendemos.",
              "Talks and open events to make the cause we stand for better known.")),
        ], intro=("Proyectos de ejemplo: cámbialos por los vuestros, con una frase llana de qué hacéis de verdad.",
                  "Example projects: replace them with your own, with a plain sentence about what you really do."),
            cols=2, eyebrow=("Qué hacemos", "What we do"), estilo="relleno", tono="relleno", anchor="proyectos"),
        bloque_pasos(("Tres formas de sumarte", "Three ways to join in"), [
            (("Hazte socio o socia", "Become a member"),
             ("Una cuota pequeña y regular nos da estabilidad para planificar el año.", "A small, regular fee gives us the stability to plan the year.")),
            (("Hazte voluntario o voluntaria", "Become a volunteer"),
             ("Dos horas al mes también cuentan. Te decimos dónde hace falta y tú eliges.", "Two hours a month count too. We tell you where help is needed and you choose.")),
            (("Haz un donativo", "Make a donation"),
             ("Escríbenos y te decimos cómo, o hazlo por transferencia. Sin importes mínimos.", "Write to us and we will tell you how, or do it by bank transfer. No minimum amount.")),
        ], tono="base"),
        bloque_opiniones(("Lo que dicen socios y voluntarios", "What members and volunteers say"),
                         [("la asociación", "the association"), ("el voluntariado", "volunteering"), ("las actividades", "the activities")]),
        bloque_cta(("¿Cómo quieres ayudar?", "How would you like to help?"),
                   ("Escríbenos o llámanos y te contamos cómo encaja lo que tú puedes dar.", "Write to us or call and we will tell you where what you can give fits in."),
                   [colabora(), llamar("ghost")], tono="base"),
    ])

    cola = pagina("colabora", ("colabora", "help"), ("Cómo colaborar", "How to help"),
                  ("Hazte socio, apúntate como voluntario o haz un donativo por transferencia. Cuotas de ejemplo y datos de la asociación.",
                   "Become a member, sign up as a volunteer or donate by bank transfer. Example fees and association details."), [
        hero(("Cómo colaborar", "How to help"),
             ("Cada persona aporta lo que puede: dinero, tiempo o ganas. Elige tu forma.",
              "Everyone gives what they can: money, time or enthusiasm. Choose your way."),
             [boton(("Quiero ser voluntario o socio", "I want to volunteer or join"), a_pagina("contacto", "contacto")), whatsapp("secondary")],
             variante="texto"),
        bloque_precios(("Hazte socio o socia", "Become a member"), ("Cuotas de socio de ejemplo", "Example membership fees"),
                       [("Tipo de socio", "Type of member"), ("Cuota", "Fee")],
                       [(("Socio o socia, al año", "Member, per year"), eur(30)),
                        (("Socio o socia, al mes", "Member, per month"), eur(3)),
                        (("Persona joven o en desempleo, al año", "Young or unemployed person, per year"), eur(10)),
                        (("Familia, al año", "Family, per year"), eur(50))],
                       intro=("Los socios votan en la asamblea, reciben las novedades y sostienen la asociación con su cuota.",
                              "Members vote at the assembly, receive the news and sustain the association with their fee."),
                       nota=("Cuotas de ejemplo: cámbialas por las tuyas.", "Example fees: replace them with your own."),
                       boton_=boton(("Quiero hacerme socio", "I want to become a member"), a_pagina("contacto", "contacto")),
                       anchor="socios", tono="base"),
        bloque_checks(("Hazte voluntario o voluntaria", "Become a volunteer"), [
            ("Eliges cuántas horas y qué días, desde dos horas al mes.", "You choose how many hours and which days, from two hours a month."),
            ("Te explicamos el trabajo y vas acompañado la primera vez.", "We explain the work and someone goes with you the first time."),
            ("Hace falta ser mayor de edad para algunas tareas. Con menores, pedimos autorización a la familia.", "Some tasks require being of legal age. With minors, we ask the family for permission."),
            ("Puedes dejarlo cuando quieras, sin dar explicaciones.", "You can stop whenever you like, with no explanation needed."),
        ], intro=("Condiciones de ejemplo: cámbialas por las vuestras.", "Example terms: replace them with your own."),
            tono="relleno", cols=2, anchor="voluntariado"),
        bloque_texto(("Haz un donativo", "Make a donation"), [
            ("Puedes colaborar con la cantidad que quieras. Escríbenos y te decimos cómo, o hazlo por transferencia a la cuenta de la asociación.",
             "You can help with any amount you like. Write to us and we will tell you how, or do it by bank transfer to the association's account."),
            ("Titular y cuenta de ejemplo: ES00 0000 0000 0000 0000 0000 (cámbiala por vuestro IBAN, 00-0000). Indica tu nombre en el concepto.",
             "Example holder and account: ES00 0000 0000 0000 0000 0000 (replace it with your IBAN, 00-0000). Put your name in the payment reference."),
            ("No cobramos nada por aquí: esta web no gestiona pagos.", "We do not take any payment here: this site does not handle payments."),
        ], eyebrow=("Sin pasarela de pago", "No payment gateway"), tono="base", anchor="donativo"),
        bloque_faq(("Preguntas frecuentes", "Frequently asked questions"), [
            (("¿Puedo dejar de ser socio cuando quiera?", "Can I stop being a member whenever I like?"),
             ("Sí. Nos avisas y no volvemos a pasarte la cuota. No hay permanencia.", "Yes. Let us know and we will not charge your fee again. There is no minimum term.")),
            (("¿Qué hacen con mis datos?", "What do you do with my data?"),
             ("Guardamos tus datos solo para gestionar tu relación con la asociación y no los cedemos a terceros, salvo que la ley lo exija.",
              "We keep your data only to manage your relationship with the association and do not pass it to third parties unless the law requires it.")),
            (("¿Puedo colaborar si no vivo en el barrio?", "Can I help if I do not live in the neighbourhood?"),
             ("Claro. Un donativo o ser socio se puede hacer desde cualquier sitio. Para el voluntariado, cuéntanos dónde estás y vemos qué encaja.",
              "Of course. A donation or membership can be done from anywhere. For volunteering, tell us where you are and we will see what fits.")),
        ], tono="relleno"),
        bloque_cta(("¿Hablamos?", "Shall we talk?"), None, [boton(("Escríbenos", "Write to us"), a_pagina("contacto", "contacto")), llamar("ghost")], tono="base"),
    ])

    aso = pagina("asociacion", ("la-asociacion", "the-association"), ("La asociación", "The association"),
                 ("Quiénes somos, cómo nos organizamos, junta directiva y transparencia: estatutos, memoria y cuentas bajo petición.",
                  "Who we are, how we are organised, board and transparency: statutes, annual report and accounts on request."), [
        hero(("La asociación", "The association"),
             ("Somos una asociación pequeña y funcionamos con cuentas a la vista y decisiones en asamblea.",
              "We are a small association and work with open accounts and decisions made at the assembly."),
             [colabora(), llamar("secondary")], variante="texto"),
        bloque_texto(("Cómo nos organizamos", "How we are organised"), [
            ("Todas las personas socias forman la asamblea, que se reúne al menos una vez al año para aprobar las cuentas y las actividades.",
             "All members make up the assembly, which meets at least once a year to approve the accounts and the activities."),
            ("Una junta directiva, elegida por la asamblea, se ocupa del día a día. Texto de ejemplo: cámbialo por vuestra forma real de trabajar.",
             "A board, elected by the assembly, takes care of the day-to-day running. Example text: replace it with how you really work."),
        ], tono="base", imagen_=("landscape", ("Asamblea de la asociación con las sillas en círculo", "Association assembly with chairs arranged in a circle"))),
        bloque_equipo(("Junta directiva", "Board"), [
            (("Presidencia", "Chair"), ("Presenta aquí en una frase a quién ocupa el cargo.", "Introduce here, in one sentence, who holds the role.")),
            (("Secretaría", "Secretary"), ("Presenta aquí en una frase a quién ocupa el cargo.", "Introduce here, in one sentence, who holds the role.")),
            (("Tesorería", "Treasurer"), ("Presenta aquí en una frase a quién ocupa el cargo.", "Introduce here, in one sentence, who holds the role.")),
        ], tono="relleno", anchor="junta"),
        bloque_texto(("Transparencia", "Transparency"), [
            ("Los estatutos, la memoria de actividades y las cuentas anuales están disponibles bajo petición. Escríbenos y te los enviamos.",
             "The statutes, the annual report and the annual accounts are available on request. Write to us and we will send them."),
            ("Tus datos de socio solo se usan para gestionar tu pertenencia a la asociación y no se ceden a terceros salvo obligación legal.",
             "Your member data is used only to manage your membership and is not passed to third parties unless legally required."),
            registro,
        ], eyebrow=("Cuentas claras", "Clear accounts"), tono="base", anchor="transparencia"),
        bloque_cta(("¿Quieres saber más?", "Want to know more?"), None, [colabora(), whatsapp("ghost")], tono="relleno"),
    ])

    cont = pagina("contacto", ("contacto", "contact"), ("Voluntariado y contacto", "Volunteering and contact"),
                  ("Formulario para apuntarte como voluntario o socio, teléfono, WhatsApp, horario de la sede y cómo llegar.",
                   "Form to sign up as a volunteer or member, phone, WhatsApp, opening hours of the office and how to find us."), [
        hero(("Apúntate como voluntario o socio", "Sign up as a volunteer or member"),
             ("Rellena el formulario, llámanos o escríbenos por WhatsApp. Te respondemos en pocos días.",
              "Fill in the form, call us or message us on WhatsApp. We will reply within a few days."),
             [llamar(), whatsapp()], variante="texto"),
        bloque_contacto(("Cuéntanos cómo quieres ayudar", "Tell us how you would like to help"),
                        ("Dinos qué te gustaría hacer y cuándo puedes. Nosotros buscamos dónde encaja.",
                         "Tell us what you would like to do and when you are free. We will find where it fits."),
                        campos_consulta(mensaje_etiqueta=("Cuéntanos qué te gustaría hacer", "Tell us what you would like to do"),
                                        servicios=[("Quiero ser voluntario o voluntaria", "I want to volunteer"),
                                                   ("Quiero hacerme socio o socia", "I want to become a member"),
                                                   ("Quiero hacer un donativo", "I want to make a donation"),
                                                   ("Otra consulta", "Something else")],
                                        extra=[campo("disponibilidad", "text", ("Días y horas en que podrías venir", "Days and hours you could come"), False, maxlen=200)]),
                        ("Enviar", "Send"), horas,
                        notas=("Tus datos solo se usan para responderte y, si te haces socio, para gestionar tu alta.",
                               "Your data is used only to reply to you and, if you become a member, to manage your membership."),
                        exito=("Gracias. Hemos recibido tu mensaje y te escribimos pronto para concretar.",
                               "Thank you. We have received your message and will write to you soon to arrange things.")),
        bloque_mapa(tono="relleno"),
    ])

    return plantilla(KEY, ("Asociación u ONG", "Association or NGO"),
                     ("Asociación u ONG pequeña con misión, proyectos, cómo colaborar (socios, voluntariado y donativo), transparencia y formulario de voluntariado.",
                      "Small association or NGO with mission, projects, ways to help (members, volunteering and donations), transparency and a volunteering form."),
                     "comunidad", ("asociacion ong ong pequena fundacion entidad sin animo de lucro voluntariado voluntarios hazte socio donativo asociacion de vecinos colectivo",
                                   "association ngo charity non-profit volunteering volunteers become a member donation community group"),
                     look, cabecera(nav, colabora()),
                     pie(nav, ("Asociación sin ánimo de lucro. Socios, voluntariado y proyectos para la comunidad. Registro nº 00-0000.",
                               "Non-profit association. Members, volunteering and community projects. Registration no. 00-0000."),
                         ("Lunes a jueves 17:00 a 20:00. Viernes 10:00 a 13:00.", "Monday to Thursday 5 pm to 8 pm. Friday 10 am to 1 pm.")),
                     [inicio, cola, aso, cont])
