# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
from lib import *

KEY = "agencia-marketing"


def _c(background, surface, text, muted, primary, on_primary, border, inverse, on_inverse):
    return dict(background=background, surface=surface, text=text, muted=muted, primary=primary,
                on_primary=on_primary, border=border, inverse=inverse, on_inverse=on_inverse)


# Paleta propia: mantequilla con magenta y tinta de ciruela. Sin fondo $inverse (primary/inverse no llega a 3).
MANTEQUILLA = dict(colores=_c("#fff7cf", "#ffeb99", "#2a0f2e", "#5b3a58", "#9b1a8a", "#ffffff", "#ecd98a", "#2a0f2e", "#fff7cf"),
                   fuentes=("montserrat", "manrope"), radio=(8, 14, 22), peso=800, h1=(50, 40, 32))


def mes(n):
    a, b = eur(n)
    return (f"Desde {a} al mes", f"From {b} a month")


def build():
    nueva(KEY)
    look = MANTEQUILLA
    malos = comprobar_paleta("mantequilla", look["colores"])
    assert not malos, malos
    nav = [("inicio", ("Inicio", "Home")), ("servicios", ("Servicios", "Services")),
           ("planes", ("Cómo trabajamos y planes", "How we work and plans")), ("contacto", ("Contacto", "Contact"))]
    horas = [(("Lunes a viernes", "Monday to Friday"),
              ("9:30 a 14:00 y 16:00 a 19:00", "9:30 am to 2:00 pm and 4:00 pm to 7:00 pm")),
             (("Sábado y domingo", "Saturday and Sunday"), ("Cerrado", "Closed"))]
    reunion = lambda v="primary": boton(("Pedir una primera reunión", "Book a first meeting"),
                                        a_pagina("contacto", "contacto"), v)
    nota = ("Precios de ejemplo: cámbialos por los tuyos. IVA no incluido. El dinero que se gasta en anuncios se paga aparte, directamente a la plataforma.",
            "Example prices: replace them with your own. VAT not included. Money spent on ads is paid separately, directly to the platform.")

    inicio = pagina("inicio", ("", ""), ("Agencia de marketing digital", "Digital marketing agency"),
                    ("Agencia pequeña de marketing digital para negocios locales: web, redes sociales, anuncios y fotografía de producto, con una primera reunión sin compromiso.",
                     "Small digital marketing agency for local businesses: website, social media, ads and product photography, with a no-obligation first meeting."), [
        hero(("Que te encuentren los vecinos que buscan lo que vendes", "Let the neighbours who search for what you sell find you"),
             ("Somos una agencia pequeña. Hacemos webs, llevamos redes sociales, preparamos anuncios y fotografiamos producto para negocios de barrio y de pueblo. Hablas siempre con las mismas personas.",
              "We are a small agency. We build websites, run social media, prepare ads and photograph products for neighbourhood and small-town businesses. You always talk to the same people."),
             [reunion(), llamar("secondary"), whatsapp("ghost")],
             eyebrow=("Marketing digital para negocios locales", "Digital marketing for local businesses"),
             variante="centro"),
        bloque_valores(("Qué hacemos", "What we do"), [
            (("Página web", "Website"),
             ("Una web clara que se ve bien en el móvil: qué haces, dónde estás, cuánto cuesta y cómo contactar contigo.",
              "A clear website that looks good on a phone: what you do, where you are, what it costs and how to reach you.")),
            (("Redes sociales", "Social media"),
             ("Publicaciones pensadas para tu negocio, con tus fotos, y respuesta a los mensajes si lo acordamos.",
              "Posts designed for your business, with your photos, and replies to messages if we agree on it.")),
            (("Anuncios", "Ads"),
             ("Campañas en buscadores y redes para llegar a quien está cerca y busca lo que vendes. Tú decides cuánto gastas.",
              "Search and social campaigns to reach people nearby who are looking for what you sell. You decide how much to spend.")),
            (("Fotografía de producto", "Product photography"),
             ("Fotos de tus productos y de tu local para la web, las redes y la carta o el catálogo.",
              "Photos of your products and your premises for the website, social media and the menu or catalogue.")),
        ], cols=4, eyebrow=("Servicios", "Services"), estilo="relleno", tono="base", anchor="que-hacemos"),
        bloque_checks(("Con quién trabajamos", "Who we work with"), [
            ("Tiendas y comercios de barrio", "Neighbourhood shops and stores"),
            ("Bares, cafeterías y restaurantes", "Bars, cafés and restaurants"),
            ("Peluquerías, clínicas y negocios de cita previa", "Hairdressers, clinics and appointment-based businesses"),
            ("Oficios y reformas", "Trades and renovation work"),
            ("Alojamientos y turismo rural", "Accommodation and rural tourism"),
            ("Asociaciones y pequeños productores", "Associations and small producers"),
        ], intro=("No hace falta que sepas de informática ni de marketing. Nos cuentas cómo trabajas y nosotros lo ponemos en internet.",
                  "You do not need to know about computers or marketing. You tell us how you work and we put it online."),
            eyebrow=("Negocios locales", "Local businesses"), tono="relleno", cols=2),
        bloque_texto(("Informes que se entienden", "Reports you can follow"), [
            ("Cada mes recibes una hoja con lo que hemos hecho, cuánta gente lo ha visto, cuántas personas han llamado, escrito o pedido cita y qué vamos a probar el mes siguiente.",
             "Every month you receive a page with what we have done, how many people saw it, how many called, wrote or booked, and what we will try next month."),
            ("Sin siglas ni gráficos imposibles. Si algo no se entiende, lo explicamos en una llamada de diez minutos.",
             "No acronyms or impossible charts. If anything is unclear, we explain it in a ten-minute call."),
        ], eyebrow=("Informes", "Reports"),
            imagen_=("photo", ("Pantalla de ordenador con un informe mensual sencillo", "Computer screen showing a simple monthly report")),
            tono="base", anchor="informes"),
        bloque_opiniones(("Lo que dicen los negocios con los que trabajamos", "What the businesses we work with say"),
                         [("la web", "the website"), ("las redes", "social media"), ("los informes", "the reports")],
                         tono="relleno"),
        bloque_cta(("Cuéntanos qué vendes y a quién", "Tell us what you sell and to whom"),
                   ("La primera reunión es para conocernos. Te decimos qué haríamos y qué no merece la pena. No te comprometes a nada.",
                    "The first meeting is to get to know each other. We tell you what we would do and what is not worth it. You are not committing to anything."),
                   [reunion(), whatsapp("ghost")], tono="base"),
    ])

    serv = pagina("servicios", ("servicios", "services"), ("Servicios", "Services"),
                  ("Qué incluye cada servicio: página web, redes sociales, anuncios y fotografía de producto, y qué necesitamos de ti para empezar.",
                   "What each service includes: website, social media, ads and product photography, and what we need from you to start."), [
        hero(("Lo que hacemos y lo que incluye", "What we do and what it includes"),
             ("Contrata solo lo que necesitas. Muchos negocios empiezan por la web o por la ficha de Google y añaden el resto más adelante.",
              "Hire only what you need. Many businesses start with the website or the Google listing and add the rest later."),
             [reunion(), whatsapp("ghost")], variante="texto"),
        bloque_texto(("Página web", "Website"), [
            ("Hacemos la web de tu negocio con pocas páginas y bien escritas: inicio, lo que ofreces, quién eres y cómo contactar. Lleva mapa, horario, botón de llamar y formulario.",
             "We build your business website with a few well-written pages: home, what you offer, who you are and how to get in touch. It includes a map, opening hours, a call button and a form."),
            ("Después puedes cambiar tú mismo los textos, los precios y las fotos, o encargárnoslo con un plan mensual.",
             "Afterwards you can change the texts, prices and photos yourself, or ask us to do it with a monthly plan."),
        ], eyebrow=("Web", "Web"),
            imagen_=("landscape", ("Web de un negocio local abierta en el móvil y en el ordenador", "A local business website open on a phone and a computer")),
            tono="base", anchor="web"),
        bloque_checks(("Redes sociales", "Social media"), [
            ("Un calendario de publicaciones que acordamos contigo cada mes.", "A posting calendar we agree with you each month."),
            ("Textos y diseños con tu logotipo y tus colores.", "Texts and designs with your logo and colours."),
            ("Fotos tuyas, de tu local y de tu equipo, no de bancos de imágenes.", "Photos of you, your premises and your team, not stock images."),
            ("Respuesta a comentarios y mensajes, si lo acordamos.", "Replies to comments and messages, if we agree on it."),
            ("Ficha de Google y perfiles al día: horario, teléfono y fotos.", "Google listing and profiles kept up to date: hours, phone and photos."),
        ], eyebrow=("Redes", "Social"), tono="relleno", cols=2, anchor="redes"),
        bloque_texto(("Anuncios", "Ads"), [
            ("Preparamos y vigilamos campañas en buscadores y en redes para enseñar tu negocio a quien vive cerca y busca lo que vendes.",
             "We set up and monitor campaigns on search engines and social networks to show your business to people nearby who are searching for what you sell."),
            ("Tú fijas cuánto quieres gastar al mes y lo pagas directamente a la plataforma. Nosotros cobramos solo por gestionarlo. No prometemos un número de clientes: nadie puede.",
             "You set how much to spend each month and pay it directly to the platform. We charge only for managing it. We do not promise a number of customers: nobody can."),
        ], eyebrow=("Anuncios", "Ads"),
            imagen_=("photo", ("Persona de un comercio revisando una campaña de anuncios en el ordenador", "Shop owner reviewing an ad campaign on a computer")),
            tono="base", invertir=True, anchor="anuncios"),
        bloque_checks(("Fotografía de producto", "Product photography"), [
            ("Una sesión en tu local o en nuestro estudio, de media jornada o de jornada completa.", "A session at your premises or in our studio, half a day or a full day."),
            ("Fotos de producto sobre fondo limpio y fotos del producto en uso.", "Product photos on a clean background and photos of the product in use."),
            ("Entrega en tamaño para web, para redes y para imprimir.", "Delivered in sizes for web, social media and print."),
            ("Los derechos de uso son tuyos: puedes usarlas donde quieras.", "The usage rights are yours: you can use them wherever you like."),
        ], eyebrow=("Fotografía", "Photography"), tono="relleno", cols=2, anchor="fotografia"),
        bloque_checks(("Qué necesitamos de ti", "What we need from you"), [
            ("Acceso a tu web y a tus perfiles actuales, si los tienes.", "Access to your current website and profiles, if you have them."),
            ("Tu logotipo y tus colores, si los tienes. Si no, los vemos juntos.", "Your logo and colours, if you have them. If not, we go through them together."),
            ("Las fotos y los textos que ya tengas. Lo que falte, lo completamos.", "The photos and texts you already have. We fill in what is missing."),
            ("Precios y horarios al día.", "Prices and opening hours up to date."),
            ("Una persona que nos responda en un par de días laborables.", "One person who replies to us within a couple of working days."),
            ("Decirnos qué quieres vender más, y qué no quieres vender.", "Telling us what you want to sell more of, and what you do not want to sell."),
        ], intro=("Lo que más retrasa un proyecto es esperar respuestas. Con esto a mano, avanzamos rápido.",
                  "What delays a project most is waiting for answers. With this to hand, we move quickly."),
            eyebrow=("Para empezar", "To get started"), tono="base", cols=2, anchor="necesitamos"),
        bloque_cta(("¿No sabes por dónde empezar?", "Not sure where to start?"),
                   ("Cuéntanoslo en una primera reunión y te decimos qué haríamos primero.",
                    "Tell us at a first meeting and we will say what we would do first."),
                   [reunion(), llamar("ghost")], tono="relleno"),
    ])

    planes = pagina("planes", ("planes", "plans"), ("Cómo trabajamos y planes", "How we work and plans"),
                    ("Cómo trabajamos paso a paso, planes mensuales y trabajos únicos con precios de partida, y respuestas a las dudas más comunes.",
                     "How we work step by step, monthly plans and one-off jobs with starting prices, and answers to the most common questions."), [
        hero(("Cómo trabajamos y cuánto cuesta", "How we work and what it costs"),
             ("Te damos una propuesta por escrito con lo que incluye y lo que no. Lo que no esté en la propuesta no se cobra.",
              "We give you a written proposal with what is included and what is not. Anything not in the proposal is not charged."),
             [reunion(), llamar("secondary")], variante="texto"),
        bloque_pasos(("Paso a paso", "Step by step"), [
            (("Primera reunión", "First meeting"),
             ("Media hora para que nos cuentes qué vendes, a quién y qué has probado ya.",
              "Half an hour for you to tell us what you sell, to whom and what you have already tried.")),
            (("Propuesta por escrito", "Written proposal"),
             ("Qué haríamos, en qué orden, cuánto cuesta y cuánto tardamos.",
              "What we would do, in what order, what it costs and how long it takes.")),
            (("Ponemos en marcha", "We get started"),
             ("Te pedimos lo que necesitamos, te enseñamos cada avance y no publicamos nada sin tu visto bueno.",
              "We ask for what we need, show you each step and publish nothing without your approval.")),
            (("Informe cada mes", "Monthly report"),
             ("Una hoja clara con lo que ha pasado y lo que haremos después. Ajustamos juntos.",
              "A clear page with what happened and what we will do next. We adjust together.")),
        ], tono="base", anchor="pasos"),
        bloque_carta(("Planes y trabajos", "Plans and jobs"), [
            (("Planes mensuales", "Monthly plans"), ("Planes mensuales de ejemplo", "Example monthly plans"),
             [("Plan", "Plan"), ("Qué incluye", "What it includes"), ("Cuota", "Fee")],
             [(("Web al día", "Website upkeep"),
               ("Cambios de textos, fotos y precios, copias de seguridad y aviso si algo falla. Hasta dos horas de cambios al mes.",
                "Changes to texts, photos and prices, backups and a warning if something fails. Up to two hours of changes a month."),
               mes(49)),
              (("Redes", "Social media"),
               ("Dos redes sociales con publicaciones semanales, fotos y textos, y respuesta a mensajes en horario de oficina.",
                "Two social networks with weekly posts, photos and texts, and replies to messages during office hours."),
               mes(280)),
              (("Redes y anuncios", "Social media and ads"),
               ("Lo anterior más una campaña de anuncios gestionada por nosotros y un informe mensual.",
                "The above plus an ad campaign managed by us and a monthly report."),
               mes(450))]),
            (("Trabajos únicos", "One-off jobs"), ("Trabajos únicos de ejemplo", "Example one-off jobs"),
             [("Trabajo", "Job"), ("Qué incluye", "What it includes"), ("Precio", "Price")],
             [(("Web de hasta cinco páginas", "Website of up to five pages"),
               ("Diseño, textos revisados, mapa, formulario y puesta en marcha.", "Design, edited texts, map, form and launch."),
               desde(950)),
              (("Sesión de fotografía de producto", "Product photography session"),
               ("Media jornada, selección y retoque de las fotos y entrega en varios tamaños.",
                "Half a day, selection and retouching of photos and delivery in several sizes."),
               desde(180)),
              (("Puesta a punto de ficha de Google y perfiles", "Google listing and profiles set-up"),
               ("Datos, horario, fotos y enlaces al día en tus perfiles principales.",
                "Details, hours, photos and links up to date on your main profiles."),
               desde(90))]),
        ], intro=("Son cifras de partida. El precio final depende de cuántas páginas, redes o fotos necesites.",
                  "These are starting figures. The final price depends on how many pages, networks or photos you need."),
            nota=nota, anchor="planes", tono="relleno"),
        bloque_faq(("Preguntas frecuentes", "Frequently asked questions"), [
            (("¿Tengo que firmar por un año?", "Do I have to sign for a year?"),
             ("No. Los planes mensuales se pueden dejar avisando con un mes de antelación. Lo dejamos por escrito en la propuesta.",
              "No. Monthly plans can be ended with a month's notice. We put it in writing in the proposal.")),
            (("¿De quién es la web y las cuentas?", "Who owns the website and the accounts?"),
             ("Tuyas. El dominio, las cuentas de redes y las de anuncios van a tu nombre desde el primer día y tienes acceso a todo.",
              "Yours. The domain and the social and ad accounts are in your name from day one and you have access to everything.")),
            (("¿Cuánto tarda una web?", "How long does a website take?"),
             ("Una web sencilla suele llevar unas semanas. Depende sobre todo de lo rápido que nos lleguen los textos y las fotos.",
              "A simple website usually takes a few weeks. It depends mostly on how quickly the texts and photos reach us.")),
            (("¿Y si no vendo más?", "What if I do not sell more?"),
             ("Nadie puede prometerte cuántos clientes llegarán. Cada mes te enseñamos qué ha pasado y qué vamos a cambiar.",
              "Nobody can promise how many customers will come. Each month we show you what happened and what we will change.")),
        ], tono="base"),
        bloque_cta(("¿Empezamos con una reunión?", "Shall we start with a meeting?"), None,
                   [reunion(), whatsapp("ghost")], tono="relleno"),
    ])

    cont = pagina("contacto", ("contacto", "contact"), ("Contacto y primera reunión", "Contact and first meeting"),
                  ("Pide tu primera reunión sin compromiso con el formulario, por teléfono o por WhatsApp. Horario y cómo llegar.",
                   "Book your first meeting with no obligation using the form, by phone or on WhatsApp. Opening hours and directions."), [
        hero(("Primera reunión sin compromiso", "First meeting with no obligation"),
             ("Media hora, en persona o por videollamada. Nos cuentas qué vendes y a quién, y te decimos qué haríamos. Si no encajamos, también te lo decimos.",
              "Half an hour, in person or by video call. You tell us what you sell and to whom, and we tell you what we would do. If we are not a good fit, we tell you that too."),
             [llamar(), whatsapp()], variante="texto"),
        bloque_contacto(("Cuéntanos tu negocio", "Tell us about your business"),
                        ("Con dos líneas nos basta. Si ya tienes web o redes, pon el enlace y lo miramos antes de la reunión.",
                         "Two lines are enough. If you already have a website or social profiles, add the link and we will look before the meeting."),
                        campos_consulta(("¿Qué vendes y qué te gustaría mejorar?", "What do you sell and what would you like to improve?"),
                                        servicios=[("Página web", "Website"), ("Redes sociales", "Social media"),
                                                   ("Anuncios", "Ads"), ("Fotografía de producto", "Product photography"),
                                                   ("No lo sé todavía", "I do not know yet")],
                                        extra=[campo("enlace", "text", ("Tu web o tus redes (si las tienes)", "Your website or social profiles (if you have any)"),
                                                     False, maxlen=200)]),
                        ("Pedir reunión", "Request meeting"), horas,
                        notas=("Usamos lo que nos cuentes solo para responderte y nunca lo cedemos.",
                               "We use what you tell us only to reply to you and never pass it on."),
                        exito=("Gracias. Te respondemos en un día laborable para fijar la reunión.",
                               "Thank you. We will reply within one working day to arrange the meeting.")),
        bloque_mapa(texto_=("Nos encuentras en la oficina con cita previa. También podemos ir a tu negocio.",
                            "You can find us at the office by appointment. We can also come to your business."), tono="relleno"),
    ])

    return plantilla(KEY, ("Agencia de marketing digital", "Digital marketing agency"),
                     ("Agencia de marketing digital para negocios locales con servicios de web, redes, anuncios y fotografía, cómo trabajamos, planes mensuales de ejemplo y primera reunión con formulario, llamada y WhatsApp.",
                      "Digital marketing agency for local businesses with web, social media, ads and photography services, how we work, example monthly plans and a first meeting with form, call and WhatsApp."),
                     "profesionales", ("agencia marketing agencia de marketing marketing digital diseno web paginas web redes sociales community manager anuncios publicidad google ads fotografia de producto negocios locales publicidad online posicionamiento",
                                       "marketing agency digital marketing web design websites social media community manager ads advertising google ads product photography local business online advertising seo"),
                     look, cabecera(nav, marco([llamar(t=("Llamar", "Call")),
                                                whatsapp("secondary", t=("WhatsApp", "WhatsApp"))],
                                               dir="row", gap=8, wrap=True)),
                     pie(nav, ("Agencia de marketing digital para negocios locales. Web, redes, anuncios y fotografía.",
                               "Digital marketing agency for local businesses. Website, social media, ads and photography."),
                         ("Lunes a viernes 9:30 a 14:00 y 16:00 a 19:00.",
                          "Monday to Friday 9:30 am to 2 pm and 4 pm to 7 pm.")),
                     [inicio, serv, planes, cont])
