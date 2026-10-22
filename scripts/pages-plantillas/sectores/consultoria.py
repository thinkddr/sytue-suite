# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
from lib import *

KEY = "consultoria"


def _c(background, surface, text, muted, primary, on_primary, border, inverse, on_inverse):
    return dict(background=background, surface=surface, text=text, muted=muted, primary=primary,
                on_primary=on_primary, border=border, inverse=inverse, on_inverse=on_inverse)


# Paleta propia: verde petróleo sobre blanco roto. Sin fondo $inverse (primary/inverse no llega a 3).
PETROLEO = dict(colores=_c("#f7f9f9", "#e8eff0", "#14262b", "#4a5d62", "#0e5a63", "#ffffff", "#d2dedf", "#0c2a30", "#e9f4f5"),
                fuentes=("source-serif-4", "ibm-plex-sans"), radio=(2, 6, 10), ancho=1140, peso=600)


def build():
    nueva(KEY)
    look = PETROLEO
    malos = comprobar_paleta("petroleo", look["colores"])
    assert not malos, malos
    nav = [("inicio", ("Inicio", "Home")), ("servicios", ("Servicios", "Services")),
           ("metodo", ("Cómo trabajamos", "How we work")), ("contacto", ("Contacto", "Contact"))]
    horas = [(("Lunes a viernes", "Monday to Friday"), ("9:00 a 14:00 y 16:00 a 19:00", "9:00 am to 2:00 pm and 4:00 pm to 7:00 pm")),
             (("Sábado y domingo", "Saturday and Sunday"), ("Cerrado", "Closed"))]

    def reunion(var="primary"):
        return boton(("Pedir primera reunión", "Request a first meeting"), a_pagina("contacto", "contacto"), var)

    inicio = pagina("inicio", ("", ""), ("Consultoría para pymes", "Consultancy for small businesses"),
                    ("Consultoría de empresa para pymes: organización, ventas, digitalización básica y ayudas, con una primera reunión sin compromiso.",
                     "Business consultancy for small companies: organisation, sales, basic digitalisation and grants, with a first meeting and no obligation."), [
        hero(("Ayudamos a tu empresa a ordenarse, vender y dar el siguiente paso", "We help your company get organised, sell and take the next step"),
             ("Trabajamos con pymes y autónomos con equipo. Primero escuchamos cómo funciona tu negocio, después te proponemos un plan que puedas llevar a cabo.",
              "We work with small companies and self-employed people with staff. First we listen to how your business runs, then we propose a plan you can actually carry out."),
             [reunion(), llamar("secondary"), whatsapp("ghost")],
             eyebrow=("Consultoría de empresa", "Business consultancy"), tono="relleno", variante="texto"),
        bloque_valores(("En qué te ayudamos", "How we can help"), [
            (("Organización", "Organisation"),
             ("Quién hace qué, cómo se decide y dónde se apunta cada cosa. Procesos simples que el equipo siga sin que tengas que estar encima.",
              "Who does what, how decisions are made and where each thing is recorded. Simple processes the team follows without you having to chase them.")),
            (("Ventas", "Sales"),
             ("Qué ofreces, a quién y a qué precio. Cómo llegan los clientes, cómo se les hace seguimiento y por qué se pierden.",
              "What you offer, to whom and at what price. How customers arrive, how they are followed up and why some are lost.")),
            (("Digitalización básica", "Basic digitalisation"),
             ("Facturar, llevar clientes y compartir información con las herramientas justas. Sin cambiar todo de golpe.",
              "Invoicing, keeping track of customers and sharing information with just the right tools. Without changing everything at once.")),
            (("Ayudas y subvenciones", "Grants and subsidies"),
             ("Te ayudamos a ver a qué convocatorias puedes optar y a preparar la solicitud. La decisión la toma la administración: no prometemos que te la concedan.",
              "We help you see which calls you may apply to and prepare the application. The decision is the administration's: we do not promise you will be awarded one.")),
        ], cols=2, eyebrow=("Servicios", "Services"), estilo="borde", tono="base", anchor="ayudamos"),
        bloque_pasos(("Así trabajamos", "How we work"), [
            (("Diagnóstico", "Diagnosis"),
             ("Hablamos contigo y con tu equipo, miramos números y forma de trabajar, y te contamos lo que vemos.",
              "We talk to you and your team, look at the figures and the way you work, and tell you what we see.")),
            (("Plan", "Plan"),
             ("Acordamos pocas prioridades, con responsables y fechas, y un presupuesto cerrado por escrito.",
              "We agree on a few priorities, with owners and dates, and a fixed quote in writing.")),
            (("Acompañamiento", "Support"),
             ("Revisamos cómo va, resolvemos dudas y ajustamos el plan. Tú decides el ritmo.",
              "We review how it is going, answer questions and adjust the plan. You decide the pace.")),
        ], intro=("Sin informes de cien páginas. Lo que te damos es para usarlo.",
                  "No hundred-page reports. What we give you is meant to be used."),
            tono="relleno", anchor="metodo"),
        bloque_checks(("Para quién trabajamos", "Who we work with"), [
            ("Pymes de entre 2 y 50 personas", "Companies of 2 to 50 people"),
            ("Autónomos que ya tienen equipo o van a tenerlo", "Self-employed people who already have staff or are about to"),
            ("Negocios familiares que cambian de generación", "Family businesses changing generation"),
            ("Empresas que van a crecer, o que se han quedado atascadas", "Companies that are about to grow, or have got stuck"),
        ], eyebrow=("A quién ayudamos", "Who we help"),
            intro=("No damos asesoramiento fiscal ni jurídico. Si lo necesitas, lo coordinamos con tu gestoría o tu abogado.",
                   "We do not give tax or legal advice. If you need it, we coordinate with your accountant or lawyer."),
            tono="base", cols=2),
        bloque_opiniones(("Lo que dicen las empresas con las que trabajamos", "What the companies we work with say"),
                         [("el diagnóstico", "the diagnosis"), ("el plan", "the plan"), ("el acompañamiento", "the support")],
                         tono="relleno"),
        bloque_cta(("Cuéntanos cómo va tu negocio", "Tell us how your business is going"),
                   ("La primera reunión es para conocernos y ver si podemos ayudarte. No te comprometes a nada.",
                    "The first meeting is to get to know each other and see whether we can help. You are not committing to anything."),
                   [reunion(), llamar("ghost")], tono="base"),
    ])

    servicios = pagina("servicios", ("servicios", "services"), ("Servicios y honorarios", "Services and fees"),
                       ("Qué hacemos en organización, ventas, digitalización y ayudas, para quién es cada servicio y cómo cobramos.",
                        "What we do in organisation, sales, digitalisation and grants, who each service is for, and how we charge."), [
        hero(("Servicios y honorarios", "Services and fees"),
             ("Qué hacemos, para quién tiene sentido y cómo se paga. Antes de empezar sabrás cuánto cuesta.",
              "What we do, who it makes sense for and how it is paid. Before we start you will know what it costs."),
             [reunion()], variante="texto"),
        bloque_carta(("Qué hacemos", "What we do"), [
            (("Organización", "Organisation"), ("Servicios de organización", "Organisation services"),
             [("Servicio", "Service"), ("Para quién", "Who it is for")],
             [(("Reparto de funciones y responsables", "Roles and responsibilities"), ("Equipos en los que todo pasa por una persona", "Teams where everything goes through one person")),
              (("Procesos y rutinas de trabajo", "Work processes and routines"), ("Empresas que repiten errores o trabajo", "Companies that repeat mistakes or work")),
              (("Cuadro de mando sencillo", "A simple dashboard"), ("Quien quiere ver cada mes cómo va el negocio", "Anyone who wants to see each month how the business is doing"))]),
            (("Ventas", "Sales"), ("Servicios de ventas", "Sales services"),
             [("Servicio", "Service"), ("Para quién", "Who it is for")],
             [(("Oferta, precios y mensaje", "Offer, prices and message"), ("Quien no sabe cómo explicar lo que vende", "Anyone who struggles to explain what they sell")),
              (("Seguimiento de clientes y presupuestos", "Customer and quote follow-up"), ("Negocios que pierden oportunidades por falta de tiempo", "Businesses losing opportunities for lack of time")),
              (("Plan comercial del año", "The year's sales plan"), ("Quien quiere crecer con orden", "Anyone who wants to grow in an orderly way"))]),
            (("Digitalización básica", "Basic digitalisation"), ("Servicios de digitalización", "Digitalisation services"),
             [("Servicio", "Service"), ("Para quién", "Who it is for")],
             [(("Elegir y poner en marcha herramientas de gestión", "Choosing and setting up management tools"), ("Quien trabaja con hojas de cálculo y papeles", "Anyone working with spreadsheets and paper")),
              (("Facturación y control de cobros", "Invoicing and payment control"), ("Negocios que cobran tarde o sin llevar cuenta", "Businesses that get paid late or without keeping track")),
              (("Formación al equipo en lo que se implanta", "Training the team on what is introduced"), ("Equipos que no quieren aprender diez programas", "Teams that do not want to learn ten programs"))]),
            (("Ayudas y subvenciones", "Grants and subsidies"), ("Servicios de ayudas", "Grants services"),
             [("Servicio", "Service"), ("Para quién", "Who it is for")],
             [(("Buscar convocatorias que encajen contigo", "Finding calls that fit you"), ("Quien no tiene tiempo de seguirlas", "Anyone without time to follow them")),
              (("Preparar la memoria y la documentación", "Preparing the project description and documents"), ("Quien ya sabe a cuál se presenta", "Anyone who already knows which one they are applying to")),
              (("Seguimiento y justificación", "Follow-up and justification"), ("Quien ha recibido una ayuda y debe justificarla", "Anyone who has received a grant and must justify it"))]),
        ], intro=("La lista es de ejemplo: cámbiala por los servicios que de verdad ofreces y borra los que no.",
                  "The list is an example: change it to the services you really offer and delete the rest."),
            nota=("Las ayudas dependen de cada convocatoria y de que la administración las conceda. No podemos garantizarlas.",
                  "Grants depend on each call and on the administration awarding them. We cannot guarantee them."),
            anchor="servicios", tono="base"),
        bloque_valores(("Cómo cobramos", "How we charge"), [
            (("Por proyecto", "Per project"),
             ("Un trabajo con principio y fin, como un diagnóstico y su plan. Presupuesto cerrado por escrito antes de empezar.",
              "A job with a start and an end, such as a diagnosis and its plan. A fixed quote in writing before we start.")),
            (("Acompañamiento mensual", "Monthly support"),
             ("Una cuota mensual acordada por un número de reuniones y de horas de trabajo. Se puede parar cuando quieras, avisando.",
              "A monthly fee agreed for a number of meetings and working hours. You can stop whenever you like, with notice.")),
            (("Horas puntuales", "One-off hours"),
             ("Para una duda concreta o una segunda opinión. Se factura la sesión y nada más.",
              "For a specific question or a second opinion. We invoice the session and nothing else.")),
        ], intro=("Honorarios a convenir según el proyecto. Te los damos por escrito tras la primera reunión.",
                  "Fees are agreed per project. We give them to you in writing after the first meeting."),
            eyebrow=("Honorarios", "Fees"), estilo="relleno", tono="relleno", anchor="honorarios"),
        bloque_faq(("Preguntas frecuentes", "Frequently asked questions"), [
            (("¿Cuánto cuesta?", "How much does it cost?"),
             ("Depende de lo que haya que hacer. Tras la primera reunión te damos un presupuesto por escrito y no empezamos sin tu visto bueno.",
              "It depends on what needs to be done. After the first meeting we give you a written quote and do not start without your approval.")),
            (("¿Cuánto tiempo lleva?", "How long does it take?"),
             ("Un diagnóstico suele llevar unas semanas. Un acompañamiento dura lo que haga falta, y tú decides cuándo acabarlo.",
              "A diagnosis usually takes a few weeks. Support lasts as long as needed, and you decide when to end it.")),
            (("¿Qué pasa con la información de mi empresa?", "What happens to my company's information?"),
             ("Es confidencial. Si lo prefieres, firmamos un acuerdo de confidencialidad antes de empezar.",
              "It is confidential. If you prefer, we sign a confidentiality agreement before we start.")),
            (("¿Hacéis el trabajo o me decís cómo hacerlo?", "Do you do the work or tell me how to do it?"),
             ("Las dos cosas, según lo que acordemos. Preferimos que el equipo aprenda a hacerlo, para no depender de nosotros.",
              "Both, depending on what we agree. We prefer the team learns how to do it, so you do not depend on us.")),
        ], tono="base"),
        bloque_cta(("¿Hablamos de tu caso?", "Shall we talk about your case?"), None,
                   [reunion(), whatsapp("ghost")], tono="relleno"),
    ])

    metodo = pagina("metodo", ("como-trabajamos", "how-we-work"), ("Cómo trabajamos", "How we work"),
                    ("Las tres fases de un trabajo (diagnóstico, plan y acompañamiento), casos tipo de ejemplo y quién te atiende.",
                     "The three phases of a project (diagnosis, plan and support), example cases and who looks after you."), [
        hero(("Cómo trabajamos", "How we work"),
             ("Trabajamos en tres tiempos y con pocas cosas a la vez. Así se avanza y no se queda en un informe.",
              "We work in three stages and with few things at once. That way things move forward instead of staying in a report."),
             [reunion()], variante="texto"),
        bloque_pasos(("Las tres fases", "The three phases"), [
            (("1. Diagnóstico", "1. Diagnosis"),
             ("Entrevistas, revisión de números y de cómo se trabaja de verdad. Termina con una reunión en la que te decimos lo que hemos visto, también lo incómodo.",
              "Interviews, a look at the figures and at how work really gets done. It ends with a meeting where we tell you what we saw, including the uncomfortable parts.")),
            (("2. Plan", "2. Plan"),
             ("Dos o tres prioridades, con quién las lleva, para cuándo y cómo sabremos si funcionan. Presupuesto cerrado.",
              "Two or three priorities, with who leads each one, by when and how we will know whether they work. Fixed quote.")),
            (("3. Acompañamiento", "3. Support"),
             ("Reuniones periódicas, tareas con fecha y ajustes sobre la marcha. Si algo no funciona, lo cambiamos.",
              "Regular meetings, tasks with dates and adjustments along the way. If something does not work, we change it.")),
        ], tono="base", anchor="fases"),
        bloque_valores(("Casos tipo", "Typical cases"), [
            (("Caso de ejemplo: un taller sin tarifas claras", "Example case: a workshop with unclear prices"),
             ("Trabaja mucho y no sabe qué trabajos le dejan margen. Se revisan costes y precios y se fija una forma sencilla de presupuestar. Cámbialo por un caso tuyo.",
              "It works a lot and does not know which jobs leave a margin. Costs and prices are reviewed and a simple way of quoting is set. Replace it with a case of your own.")),
            (("Caso de ejemplo: un comercio que vende por WhatsApp", "Example case: a shop that sells over WhatsApp"),
             ("Los pedidos se pierden entre mensajes. Se ordena cómo se reciben, se apuntan y se cobran, con una herramienta que el equipo ya sabe usar. Cámbialo por un caso tuyo.",
              "Orders get lost among messages. How they are received, recorded and paid is organised, with a tool the team already knows how to use. Replace it with a case of your own.")),
            (("Caso de ejemplo: un negocio familiar que cambia de relevo", "Example case: a family business changing hands"),
             ("Todo depende de una persona. Se reparten funciones, se escribe cómo se hacen las cosas y se acuerda quién decide qué. Cámbialo por un caso tuyo.",
              "Everything depends on one person. Roles are shared out, how things are done is written down and it is agreed who decides what. Replace it with a case of your own.")),
        ], intro=("Son casos de ejemplo, sin cifras: cámbialos por los tuyos o bórralos. No uses nombres de clientes sin su permiso.",
                  "These are example cases, without figures: replace them with your own or delete them. Do not use client names without their permission."),
            eyebrow=("Casos", "Cases"), estilo="relleno", tono="relleno", anchor="casos"),
        bloque_equipo(("Quién te atiende", "Who looks after you"), [
            (("Consultor o consultora", "Consultant"), ("Presenta aquí en una frase en qué trabaja y con qué empresas.",
                                                        "Introduce here, in one sentence, what they work on and with what kind of companies.")),
            (("Consultor o consultora", "Consultant"), ("Presenta aquí en una frase en qué trabaja y con qué empresas.",
                                                        "Introduce here, in one sentence, what they work on and with what kind of companies.")),
        ], intro=("Siempre hablas con las mismas personas, de principio a fin.",
                  "You always talk to the same people, from start to finish."),
            cols=2, tono="base"),
        bloque_cta(("Empezamos por una conversación", "We start with a conversation"), None,
                   [reunion(), llamar("ghost")], tono="relleno"),
    ])

    cont = pagina("contacto", ("contacto", "contact"), ("Contacto y primera reunión", "Contact and first meeting"),
                  ("Pide tu primera reunión sin compromiso con el formulario, por teléfono o por WhatsApp. Horario y cómo llegar.",
                   "Request your first meeting, with no obligation, using the form, by phone or on WhatsApp. Opening hours and directions."), [
        hero(("Primera reunión sin compromiso", "First meeting with no obligation"),
             ("Una conversación de unos 45 minutos, en persona o por videollamada. Nos cuentas cómo va tu negocio y te decimos si podemos ayudarte y cómo.",
              "A conversation of about 45 minutes, in person or by video call. You tell us how your business is going and we tell you whether and how we can help."),
             [llamar(), whatsapp()], variante="texto"),
        bloque_contacto(("Dinos cómo podemos ayudarte", "Tell us how we can help"),
                        ("Si ya sabes qué día te viene bien, indícalo y te confirmamos la hora. No hace falta enviar documentos.",
                         "If you already know which day suits you, let us know and we will confirm a time. You do not need to send any documents."),
                        campos_cita(servicios=[("Organización", "Organisation"), ("Ventas", "Sales"),
                                               ("Digitalización", "Digitalisation"), ("Ayudas y subvenciones", "Grants and subsidies"),
                                               ("No lo tengo claro", "I am not sure")],
                                    extra=[campo("correo", "email", ("Correo electrónico (opcional)", "Email (optional)"), False, auto="email", maxlen=200),
                                           campo("tamano", "select", ("Tamaño de tu empresa", "Size of your company"), False,
                                                 opciones=[("Solo yo", "Just me"), ("De 2 a 9 personas", "2 to 9 people"),
                                                           ("De 10 a 49 personas", "10 to 49 people"), ("50 o más", "50 or more")])]),
                        ("Pedir reunión", "Request meeting"), horas,
                        notas=("Lo que nos cuentes es confidencial y solo lo usamos para responderte.",
                               "What you tell us is confidential and we use it only to reply to you."),
                        exito=("Gracias. Te confirmamos la reunión por teléfono o por correo en cuanto la tengamos.",
                               "Thank you. We will confirm the meeting by phone or email as soon as we have it.")),
        bloque_mapa(texto_=("Recibimos con cita previa. También vamos a tu negocio si lo prefieres.",
                            "We receive visitors by appointment. We can also come to your business if you prefer."), tono="relleno"),
    ])

    return plantilla(KEY, ("Consultoría de empresas", "Business consultancy"),
                     ("Consultoría para pymes con servicios, cómo trabajamos, casos tipo, honorarios a convenir y primera reunión sin compromiso con formulario, llamada y WhatsApp.",
                      "Consultancy for small companies with services, how we work, example cases, fees by agreement and a no-obligation first meeting with form, call and WhatsApp."),
                     "profesionales", ("consultoria consultor consultora consultoria de empresas asesoramiento empresarial pymes organizacion ventas digitalizacion subvenciones ayudas plan de negocio gestion de empresas",
                                       "consultancy consultant business consulting small business sme organisation sales digitalisation grants subsidies business plan management"),
                     look, cabecera(nav, marco([llamar(), whatsapp("ghost")], dir="row", gap=12, wrap=True)),
                     pie(nav, ("Consultoría para pymes. Organización, ventas, digitalización básica y ayudas.",
                               "Consultancy for small companies. Organisation, sales, basic digitalisation and grants."),
                         ("Lunes a viernes 9:00 a 14:00 y 16:00 a 19:00.",
                          "Monday to Friday 9 am to 2 pm and 4 pm to 7 pm.")),
                     [inicio, servicios, metodo, cont])
