# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
from lib import *

KEY = "coach-orientador"


def _c(background, surface, text, muted, primary, on_primary, border, inverse, on_inverse):
    return dict(background=background, surface=surface, text=text, muted=muted, primary=primary,
                on_primary=on_primary, border=border, inverse=inverse, on_inverse=on_inverse)


# Paleta propia: lila grisáceo con ámbar oscuro. Sin fondo $inverse (primary/inverse no llega a 3).
LILA_AMBAR = dict(colores=_c("#f2eef9", "#e4dcf2", "#241a38", "#544a6e", "#8f4a00", "#ffffff", "#d4c9e8", "#2e1f4a", "#f4effb"),
                  fuentes=("playfair-display", "manrope"), radio=(8, 14, 22), ancho=1120, peso=600)


def build():
    nueva(KEY)
    look = LILA_AMBAR
    malos = comprobar_paleta("lila-ambar", look["colores"])
    assert not malos, malos
    nav = [("inicio", ("Inicio", "Home")), ("sesiones", ("Cómo son las sesiones", "What sessions are like")),
           ("tarifas", ("Tarifas y bonos", "Rates and bundles")), ("contacto", ("Contacto", "Contact"))]
    horas = [(("Lunes a jueves", "Monday to Thursday"),
              ("10:00 a 14:00 y 16:00 a 20:00", "10:00 am to 2:00 pm and 4:00 pm to 8:00 pm")),
             (("Viernes", "Friday"), ("10:00 a 14:00", "10:00 am to 2:00 pm")),
             (("Sábado y domingo", "Saturday and Sunday"), ("Cerrado", "Closed"))]
    primera = lambda v="primary": boton(("Pedir una primera conversación", "Book a first conversation"),
                                        a_pagina("contacto", "contacto"), v)
    nota = ("Precios de ejemplo: cámbialos por los tuyos. Di si llevan IVA o no.",
            "Example prices: replace them with your own. Say whether they include VAT or not.")

    inicio = pagina("inicio", ("", ""), ("Coaching y orientación", "Coaching and guidance"),
                    ("Coaching y orientación profesional y personal: para quién es, cómo son las sesiones, presencial y online, y una primera conversación sin compromiso.",
                     "Professional and personal coaching and guidance: who it is for, what sessions are like, in person and online, and a no-obligation first conversation."), [
        hero(("Un rato para pensar con calma lo que quieres cambiar", "Time to think calmly about what you want to change"),
             ("Acompaño a personas que tienen una decisión por delante o sienten que algo en su trabajo o en su vida ya no encaja. Te escucho, te hago preguntas y te ayudo a dar pasos pequeños. Las decisiones son siempre tuyas.",
              "I support people who have a decision ahead or feel that something in their work or life no longer fits. I listen, ask questions and help you take small steps. The decisions are always yours."),
             [primera(), llamar("secondary"), whatsapp("ghost")],
             eyebrow=("Coaching y orientación", "Coaching and guidance"),
             imagen_=("portrait", ("Dos sillas frente a frente junto a una ventana con luz natural", "Two chairs facing each other by a window with natural light"))),
        bloque_checks(("¿Para quién es?", "Who is it for?"), [
            ("Quien se plantea cambiar de trabajo o de sector y no sabe por dónde empezar.", "People thinking of changing job or field who do not know where to start."),
            ("Quien termina los estudios y duda entre varios caminos.", "People finishing their studies who are unsure between several paths."),
            ("Quien vuelve a trabajar tras un parón y quiere preparar el regreso.", "People returning to work after a break who want to prepare for it."),
            ("Quien tiene una decisión importante y quiere pensarla en voz alta con alguien.", "People with an important decision who want to think it through aloud with someone."),
            ("Quien siente que va en piloto automático y quiere recuperar el rumbo.", "People who feel they are on autopilot and want to get their bearings back."),
            ("Quien necesita organizar su tiempo y sus prioridades.", "People who need to organise their time and priorities."),
        ], eyebrow=("Para quién", "For whom"), tono="base", cols=2, anchor="para-quien"),
        bloque_valores(("Cómo trabajo", "How I work"), [
            (("Escucho", "I listen"),
             ("Primero entiendo qué te pasa y qué quieres que sea distinto. Sin prisa y sin juzgar.",
              "First I understand what is happening and what you want to be different. No rush and no judgement.")),
            (("Pregunto", "I ask"),
             ("Te hago preguntas para que veas tu situación desde otro lado. No te doy la respuesta ni decido por ti.",
              "I ask questions so you can see your situation from another angle. I do not give you the answer or decide for you.")),
            (("Acordamos pasos", "We agree on steps"),
             ("Al final de cada sesión eliges una o dos cosas pequeñas para probar. En la siguiente vemos cómo fue.",
              "At the end of each session you choose one or two small things to try. At the next one we see how it went.")),
        ], eyebrow=("Método", "Method"), cols=3, estilo="relleno", tono="relleno", anchor="como-trabajo"),
        bloque_texto(("Lo que el coaching no es", "What coaching is not"), [
            ("El coaching no es terapia ni sustituye a un tratamiento psicológico o médico. No diagnostico ni trato problemas de salud mental.",
             "Coaching is not therapy and does not replace psychological or medical treatment. I do not diagnose or treat mental health problems."),
            ("Si estás pasando por un momento de mucho sufrimiento, ansiedad o tristeza, lo mejor es que lo consultes con un psicólogo, un psiquiatra o tu médico de cabecera. Si durante nuestras sesiones veo que es lo que necesitas, te lo diré con claridad y te orientaré hacia quien pueda ayudarte.",
             "If you are going through a time of great distress, anxiety or sadness, the best thing is to see a psychologist, a psychiatrist or your family doctor. If during our sessions I see that this is what you need, I will tell you clearly and point you to someone who can help."),
            ("Si es una urgencia, llama al 112.", "If it is an emergency, call 112."),
        ], eyebrow=("Con claridad", "To be clear"), tono="base", anchor="que-no-es"),
        bloque_opiniones(("Lo que cuentan las personas que han pasado por las sesiones", "What people who have been through the sessions say"),
                         [("las sesiones", "the sessions"), ("el trato", "the way I treat people"), ("los pasos acordados", "the agreed steps")],
                         tono="relleno"),
        bloque_cta(("Empezamos por una conversación", "We start with a conversation"),
                   ("Veinte minutos para contarme qué te trae y ver si puedo ayudarte. Si no soy la persona adecuada, te lo digo. No te comprometes a nada.",
                    "Twenty minutes to tell me what brings you here and see whether I can help. If I am not the right person, I will tell you. You are not committing to anything."),
                   [primera(), whatsapp("ghost")], tono="base"),
    ])

    sesiones = pagina("sesiones", ("sesiones", "sessions"), ("Cómo son las sesiones", "What sessions are like"),
                      ("El proceso paso a paso, sesiones presenciales y online, qué puedes esperar de mí y qué no, y respuestas a las dudas más comunes.",
                       "The process step by step, in-person and online sessions, what you can expect from me and what not, and answers to the most common questions."), [
        hero(("Cómo son las sesiones", "What sessions are like"),
             ("Cada proceso es distinto, pero casi todos siguen el mismo recorrido. Esto es lo que puedes esperar desde la primera conversación hasta la sesión de cierre.",
              "Every process is different, but almost all follow the same path. This is what you can expect from the first conversation to the last session."),
             [primera(), llamar("secondary")], variante="texto", tono="relleno"),
        bloque_pasos(("El proceso", "The process"), [
            (("Primera conversación", "First conversation"),
             ("Unos 20 minutos, sin compromiso. Me cuentas qué te trae y yo te explico cómo trabajo.",
              "About 20 minutes, with no obligation. You tell me what brings you here and I explain how I work.")),
            (("Sesión de arranque", "Kick-off session"),
             ("Ponemos en palabras qué quieres cambiar y qué sería para ti un buen punto de llegada.",
              "We put into words what you want to change and what a good outcome would be for you.")),
            (("Sesiones de trabajo", "Working sessions"),
             ("De una hora, cada una o dos semanas. Exploramos, decides y eliges qué probar hasta la siguiente.",
              "One hour each, every one or two weeks. We explore, you decide and you choose what to try until the next one.")),
            (("Cierre", "Closing"),
             ("Miramos lo que has avanzado y decidimos juntos cuándo terminar. No hay un número fijo de sesiones.",
              "We look at what you have achieved and decide together when to finish. There is no fixed number of sessions.")),
        ], tono="base", anchor="proceso"),
        bloque_valores(("Presencial u online", "In person or online"), [
            (("Presencial", "In person"),
             ("En mi consulta, con cita. Un sitio tranquilo, sin interrupciones.",
              "At my practice, by appointment. A quiet place without interruptions.")),
            (("Online", "Online"),
             ("Por videollamada. Solo necesitas conexión y un sitio donde puedas hablar sin que te oigan.",
              "By video call. All you need is a connection and somewhere you can talk without being overheard.")),
            (("Mixto", "Mixed"),
             ("Puedes empezar en persona y seguir online, o al revés, según te venga mejor.",
              "You can start in person and continue online, or the other way round, whichever suits you.")),
        ], eyebrow=("Formato", "Format"), cols=3, estilo="borde", tono="relleno", anchor="formato"),
        bloque_checks(("Qué puedes esperar de mí", "What you can expect from me"), [
            ("Que te escuche con atención y no te juzgue.", "That I listen carefully and do not judge you."),
            ("Que te haga preguntas que a veces incomodan, con respeto.", "That I ask questions that are sometimes uncomfortable, with respect."),
            ("Que lo que me cuentes quede entre nosotros.", "That what you tell me stays between us."),
            ("Que te diga con claridad cuándo algo no es coaching y te oriente.", "That I tell you clearly when something is not coaching and point you elsewhere."),
        ], intro=("Y lo que no puedes esperar: que decida por ti, que te dé una respuesta hecha o que te prometa un resultado.",
                  "And what you cannot expect: that I decide for you, give you a ready-made answer or promise you a result."),
            eyebrow=("Compromiso", "Commitment"), tono="base", cols=2, anchor="esperar"),
        bloque_faq(("Preguntas frecuentes", "Frequently asked questions"), [
            (("¿Cuántas sesiones necesito?", "How many sessions do I need?"),
             ("Depende de lo que traigas. Lo hablamos al empezar y lo revisamos por el camino. Tú decides cuándo parar.",
              "It depends on what you bring. We discuss it at the start and review it along the way. You decide when to stop.")),
            (("¿Qué diferencia hay con ir a un psicólogo?", "How is it different from seeing a psychologist?"),
             ("El coaching mira hacia delante: qué quieres y cómo avanzar. La psicología trata el malestar y la salud mental, y es otra profesión. Si necesitas lo segundo, te lo diré.",
              "Coaching looks ahead: what you want and how to move forward. Psychology treats distress and mental health, and is a different profession. If you need the latter, I will tell you.")),
            (("¿Es confidencial?", "Is it confidential?"),
             ("Sí. Lo que me cuentas no sale de las sesiones, salvo que tú me lo pidas o la ley me obligue.",
              "Yes. What you tell me stays within the sessions, unless you ask me otherwise or the law requires it.")),
            (("¿Y si tengo que cancelar una sesión?", "What if I have to cancel a session?"),
             ("Avísame con 24 horas y la cambiamos de día sin coste. Si no avisas, se cobra la sesión.",
              "Let me know 24 hours ahead and we move it to another day at no cost. If you do not, the session is charged.")),
        ], tono="relleno"),
        bloque_cta(("¿Hablamos?", "Shall we talk?"), None, [primera(), whatsapp("ghost")], tono="base"),
    ])

    tarifas = pagina("tarifas", ("tarifas", "rates"), ("Tarifas y bonos", "Rates and bundles"),
                     ("Precio de la sesión individual, presencial u online, bonos de sesiones y cómo se paga y se cancela.",
                      "Price of the individual session, in person or online, session bundles, and how payment and cancellation work."), [
        hero(("Tarifas y bonos", "Rates and bundles"),
             ("Sabes lo que cuesta antes de empezar. La primera conversación no tiene coste.",
              "You know what it costs before you start. The first conversation is free of charge."),
             [primera(), llamar("secondary")], variante="texto", tono="relleno"),
        bloque_carta(("Lo que cuesta", "What it costs"), [
            (("Sesiones sueltas", "Single sessions"), ("Sesiones sueltas de ejemplo", "Example single sessions"),
             [("Sesión", "Session"), ("Duración", "Length"), ("Precio", "Price")],
             [(("Primera conversación", "First conversation"), ("20 minutos", "20 minutes"), ("Sin coste", "Free")),
              (("Sesión individual, presencial u online", "Individual session, in person or online"),
               ("60 minutos", "60 minutes"), eur(60)),
              (("Sesión de orientación profesional: currículum y entrevistas", "Career guidance session: CV and interviews"),
               ("60 minutos", "60 minutes"), eur(60))]),
            (("Bonos", "Bundles"), ("Bonos de sesiones de ejemplo", "Example session bundles"),
             [("Bono", "Bundle"), ("Qué incluye", "What it includes"), ("Precio", "Price")],
             [(("Bono de 4 sesiones", "Bundle of 4 sessions"),
               ("Cuatro sesiones de 60 minutos, para usar en el plazo que acordemos.",
                "Four 60-minute sessions, to use within the period we agree."), eur(220)),
              (("Bono de 8 sesiones", "Bundle of 8 sessions"),
               ("Ocho sesiones de 60 minutos, para usar en el plazo que acordemos.",
                "Eight 60-minute sessions, to use within the period we agree."), eur(420))]),
        ], intro=("Los bonos salen algo más baratos que las sesiones sueltas. No hay obligación de comprar un bono para empezar.",
                  "Bundles work out a little cheaper than single sessions. There is no obligation to buy a bundle to start."),
            nota=nota, anchor="precios", tono="base"),
        bloque_checks(("Cómo se paga y se cancela", "How payment and cancellation work"), [
            ("Pagas por transferencia o con tarjeta antes de cada sesión, o al comprar el bono.", "You pay by transfer or card before each session, or when buying the bundle."),
            ("Te doy factura de todo lo que pagas.", "I give you an invoice for everything you pay."),
            ("Puedes cambiar o cancelar una sesión avisando con 24 horas.", "You can change or cancel a session with 24 hours' notice."),
            ("Si decides parar a mitad de un bono, hablamos y vemos cómo hacerlo.", "If you decide to stop halfway through a bundle, we talk and work out how."),
        ], eyebrow=("Pagos", "Payments"), tono="relleno", cols=2, anchor="pagos"),
        bloque_cta(("¿Empezamos con una conversación?", "Shall we start with a conversation?"),
                   ("La primera conversación es para ver si encajamos. Después decides tú.",
                    "The first conversation is to see whether we are a good fit. After that, you decide."),
                   [primera(), whatsapp("ghost")], tono="base"),
    ])

    cont = pagina("contacto", ("contacto", "contact"), ("Contacto y primera conversación", "Contact and first conversation"),
                  ("Pide tu primera conversación sin compromiso con el formulario, por teléfono o por WhatsApp. Horario, cómo llegar y qué hacer en caso de urgencia.",
                   "Book your first conversation with no obligation using the form, by phone or on WhatsApp. Opening hours, directions and what to do in an emergency."), [
        hero(("Primera conversación sin compromiso", "First conversation with no obligation"),
             ("Veinte minutos, por teléfono, por videollamada o en persona. Me cuentas qué te trae y yo te digo si puedo ayudarte y cómo.",
              "Twenty minutes, by phone, by video call or in person. You tell me what brings you here and I tell you whether and how I can help."),
             [llamar(), whatsapp()], variante="texto", tono="relleno"),
        bloque_contacto(("Cuéntame qué te trae", "Tell me what brings you here"),
                        ("Con una o dos frases me basta. Dime también qué días y qué horas te vienen bien.",
                         "One or two sentences are enough. Also tell me which days and times suit you."),
                        campos_cita(servicios=[("Orientación profesional", "Career guidance"),
                                               ("Un cambio o una decisión personal", "A personal change or decision"),
                                               ("No lo tengo claro", "I am not sure")],
                                    extra=[campo("formato", "select", ("Formato", "Format"), False,
                                                 opciones=[("Presencial", "In person"), ("Online", "Online"), ("Me da igual", "No preference")]),
                                           campo("correo", "email", ("Correo electrónico (opcional)", "Email (optional)"), False, auto="email", maxlen=200)]),
                        ("Pedir conversación", "Request conversation"), horas,
                        notas=("Esto no es un servicio de urgencias. Si necesitas ayuda ahora, llama al 112 o a tu médico. Lo que me cuentes es confidencial y lo uso solo para responderte.",
                               "This is not an emergency service. If you need help now, call 112 or your doctor. What you tell me is confidential and I use it only to reply to you."),
                        exito=("Gracias. Te respondo en un día laborable para fijar la conversación.",
                               "Thank you. I will reply within one working day to set up the conversation.")),
        bloque_mapa(texto_=("Atiendo en mi consulta con cita previa. Las sesiones online se hacen por videollamada.",
                            "I see people at my practice by appointment. Online sessions are by video call."), tono="relleno"),
    ])

    return plantilla(KEY, ("Coach y orientador", "Coach and guidance counsellor"),
                     ("Coaching y orientación profesional y personal con para quién es, proceso en pasos, presencial y online, tarifas y bonos de ejemplo, qué no es y primera conversación con formulario, llamada y WhatsApp.",
                      "Professional and personal coaching and guidance with who it is for, a step-by-step process, in-person and online sessions, example rates and bundles, what it is not, and a first conversation with form, call and WhatsApp."),
                     "profesionales", ("coach coaching coach personal orientador orientacion profesional orientacion laboral orientacion vocacional cambio de trabajo desarrollo personal sesiones online bonos acompanamiento cambio de carrera",
                                       "coach coaching life coach career coach career guidance career change personal development online sessions bundles counselling support"),
                     look, cabecera(nav, marco([llamar(t=("Llamar", "Call")),
                                                whatsapp("secondary", t=("WhatsApp", "WhatsApp"))],
                                               dir="row", gap=8, wrap=True)),
                     pie(nav, ("Coaching y orientación profesional y personal. Presencial y online.",
                               "Professional and personal coaching and guidance. In person and online."),
                         ("Lunes a jueves 10:00 a 14:00 y 16:00 a 20:00. Viernes 10:00 a 14:00.",
                          "Monday to Thursday 10 am to 2 pm and 4 pm to 8 pm. Friday 10 am to 2 pm.")),
                     [inicio, sesiones, tarifas, cont])
