# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
from lib import *
from looks import LOOKS  # noqa: F401  (la paleta es propia de este sector)

KEY = "portfolio"

# Papel de imprenta y tinta casi negra, con un solo acento bermellón (el rojo de las pruebas de
# imprenta). Tipografía grande y sin cajas de más: el protagonista es el trabajo.
IMPRENTA = dict(
    colores=dict(background="#f4f2ee", surface="#e9e6df", text="#141414", muted="#57544d", primary="#b5351a",
                 on_primary="#ffffff", border="#d6d2c8", inverse="#141414", on_inverse="#f4f2ee"),
    fuentes=("lora", "inter"), peso=500, h1=(64, 46, 34), radio=(0, 2, 4), ancho=1200)

NOTA = ("Precios de ejemplo: cámbialos por los tuyos. IVA no incluido. Cada proyecto lleva un presupuesto cerrado por escrito. Los plazos y las rondas de cambios de las preguntas de abajo también son de ejemplo.",
        "Example prices: replace them with your own. VAT not included. Every project comes with a fixed quote in writing. The timescales and rounds of changes in the questions below are examples too.")


def trabajo(titulo, tipo, texto_, alt, enlace=None):
    t = tarjeta([imagen("landscape", alt, ratio=1.35, radio=0), peq(tipo, color="$primary"), h3(titulo),
                 p(texto_, color="$muted")], estilo="linea", gap=10, pad=[16, 0, 0, 0],
                tag="div" if enlace else "li", enlace=enlace)
    return marco([t], tag="li", gap=0) if enlace else t


def build():
    nueva(KEY)
    look = IMPRENTA
    nav = [("inicio", ("Inicio", "Home")), ("trabajos", ("Trabajos", "Work")),
           ("servicios", ("Servicios y tarifas", "Services and rates")), ("sobre-mi", ("Sobre mí", "About me")),
           ("contacto", ("Contacto", "Contact"))]

    def proyecto(var="primary"):
        return boton(("Cuéntame tu proyecto", "Tell me about your project"), a_pagina("contacto", "contacto"), var)

    cabecera_ = cabecera(nav, marco([llamar("secondary"), whatsapp("ghost")], dir="row", gap=12, wrap=True))

    portada = seccion("hero", [marco([
        etiqueta(("Diseño freelance", "Freelance design")),
        h1(("Diseño marcas, webs e ilustraciones.", "I design brands, websites and illustrations.")),
        p(("Trabajo por proyectos, a distancia y con pocos clientes a la vez. Me cuentas qué necesitas, te mando una propuesta cerrada y te enseño el trabajo por fases.",
           "I work on projects, remotely and with few clients at a time. You tell me what you need, I send you a fixed proposal and show you the work in stages."),
          color="$muted"),
        marco([proyecto(), llamar("secondary"), whatsapp("ghost")], dir="row", gap=12, wrap=True, align="center")],
        gap=22, sz={"width": "fill", "height": "hug", "max_width": 880})], pad_v=96, gap=0)

    cuatro = [("Identidad visual", "Visual identity"), ("Diseño web", "Web design"), ("Ilustración", "Illustration"),
              ("Material impreso", "Printed material")]
    trabajos_inicio = [
        trabajo(("Identidad para una cafetería", "Identity for a coffee shop"), ("Identidad visual", "Visual identity"),
                ("Logotipo, colores y rótulos para un local de barrio.", "Logo, colours and signage for a neighbourhood shop."),
                ("Rótulo y vasos con la identidad de una cafetería", "Sign and cups carrying a coffee shop's identity"),
                a_pagina("trabajos")),
        trabajo(("Web de un estudio de arquitectura", "Website for an architecture studio"), ("Diseño web", "Web design"),
                ("Una web de cinco páginas que se lee bien en el móvil.", "A five-page website that reads well on a phone."),
                ("Web de un estudio en un portátil y en un móvil", "A studio website on a laptop and on a phone"),
                a_pagina("trabajos")),
        trabajo(("Serie de láminas para una librería", "Series of prints for a bookshop"), ("Ilustración", "Illustration"),
                ("Seis ilustraciones para bolsas y marcapáginas.", "Six illustrations for bags and bookmarks."),
                ("Láminas ilustradas sobre una mesa", "Illustrated prints on a table"), a_pagina("trabajos")),
    ]
    inicio = pagina("inicio", ("", ""), ("Portfolio de diseño", "Design portfolio"),
                    ("Portfolio de diseño freelance: trabajos recientes, servicios, tarifas de ejemplo y formulario para contar tu proyecto. Llama o escribe por WhatsApp.",
                     "Freelance design portfolio: recent work, services, example rates and a form to tell me about your project. Call or message me on WhatsApp."), [
        portada,
        seccion("gallery", [
            titular(("Trabajos recientes", "Recent work"),
                    ("Tres proyectos de ejemplo: cámbialos por los tuyos con una foto buena de cada uno.",
                     "Three example projects: replace them with your own, with a good photo of each."),
                    eyebrow=("Selección", "Selection")),
            rejilla(trabajos_inicio, cols=3, gap=28),
            boton(("Ver todos los trabajos", "See all the work"), a_pagina("trabajos"), "secondary")],
            anchor="trabajos-recientes", gap=36),
        bloque_valores(("En qué puedo ayudarte", "How I can help"), [
            (("Identidad visual", "Visual identity"),
             ("Logotipo, colores y tipografías para que tu negocio se reconozca a la primera.",
              "Logo, colours and typefaces so your business is recognised at first sight.")),
            (("Diseño web", "Web design"),
             ("Webs claras, rápidas y pensadas para el móvil, que puedas actualizar tú.",
              "Clear, fast websites designed for phones, which you can update yourself.")),
            (("Ilustración", "Illustration"),
             ("Piezas sueltas o series, para la web, un producto o una publicación.",
              "Single pieces or series, for a website, a product or a publication.")),
            (("Material impreso", "Printed material"),
             ("Carteles, folletos, catálogos y cartas, listos para llevar a imprenta.",
              "Posters, leaflets, catalogues and menus, ready to send to the printer.")),
        ], cols=4, estilo="linea", eyebrow=("Servicios", "Services"), tono="relleno"),
        bloque_opiniones(("Lo que dicen mis clientes", "What my clients say"),
                         [("el proceso de trabajo", "the way of working"), ("el resultado final", "the final result"),
                          ("la comunicación", "the communication")], tono="base"),
        bloque_cta(("Tengo hueco para un proyecto nuevo el mes que viene", "I have room for a new project next month"),
                   ("Es un ejemplo de disponibilidad: cámbialo cuando cambie tu agenda. Si tu proyecto tiene fecha, escríbeme cuanto antes y te digo si llego.",
                    "This is an example of availability: change it when your schedule changes. If your project has a deadline, write to me as soon as you can and I will tell you whether I can make it."),
                   [proyecto(), llamar("ghost")], tono="relleno"),
    ])

    trabajos = pagina("trabajos", ("trabajos", "work"), ("Trabajos", "Work"),
                      ("Selección de proyectos de identidad visual, web, ilustración y material impreso, con lo que se pedía y lo que se entregó.",
                       "A selection of visual identity, web, illustration and print projects, with what was asked for and what was delivered."), [
        hero(("Trabajos", "Work"),
             ("Cada proyecto, con lo que se pedía y lo que entregué. Si te interesa algo parecido, cuéntamelo.",
              "Each project, with what was asked for and what I delivered. If you are interested in something similar, tell me."),
             [proyecto()], variante="texto"),
        seccion("gallery", [
            titular(("Proyectos", "Projects"),
                    ("Estos seis proyectos son de ejemplo: cámbialos por tus trabajos, con una foto buena y dos frases sobre qué se pedía y qué entregaste.",
                     "These six projects are examples: replace them with your own work, with a good photo and two sentences on what was asked for and what you delivered.")),
            rejilla([
                trabajo(("Identidad para una cafetería", "Identity for a coffee shop"), ("Identidad visual", "Visual identity"),
                        ("Pedían algo cálido y fácil de pintar en la fachada. Entregué logotipo, paleta y guía de uso.",
                         "They wanted something warm and easy to paint on the front. I delivered a logo, palette and usage guide."),
                        ("Rótulo y vasos con la identidad de una cafetería", "Sign and cups carrying a coffee shop's identity")),
                trabajo(("Web de un estudio de arquitectura", "Website for an architecture studio"), ("Diseño web", "Web design"),
                        ("Querían enseñar obras sin saturar. Entregué cinco páginas con las fotos a gran tamaño.",
                         "They wanted to show buildings without clutter. I delivered five pages with large photos."),
                        ("Web de un estudio en un portátil y en un móvil", "A studio website on a laptop and on a phone")),
                trabajo(("Serie de láminas para una librería", "Series of prints for a bookshop"), ("Ilustración", "Illustration"),
                        ("Necesitaban una imagen propia para bolsas y marcapáginas. Entregué seis láminas y sus versiones pequeñas.",
                         "They needed their own image for bags and bookmarks. I delivered six prints and small versions of them."),
                        ("Láminas ilustradas sobre una mesa", "Illustrated prints on a table")),
                trabajo(("Carta para un restaurante", "Menu for a restaurant"), ("Material impreso", "Printed material"),
                        ("Una carta que se leyera con poca luz. Entregué el diseño listo para imprenta.",
                         "A menu that could be read in low light. I delivered the design ready for the printer."),
                        ("Carta de un restaurante abierta sobre una mesa", "A restaurant menu open on a table")),
                trabajo(("Catálogo de una ceramista", "Catalogue for a ceramicist"), ("Material impreso", "Printed material"),
                        ("Ordenar cuarenta piezas sin que parecieran un inventario. Entregué un catálogo de dieciséis páginas.",
                         "Arranging forty pieces without it looking like an inventory. I delivered a sixteen-page catalogue."),
                        ("Páginas de un catálogo de cerámica", "Pages from a ceramics catalogue")),
                trabajo(("Tienda online de una marca de ropa", "Online shop for a clothing brand"), ("Diseño web", "Web design"),
                        ("Que comprar desde el móvil fuera tan fácil como en la tienda. Entregué el diseño de cuatro pantallas clave.",
                         "Making buying from a phone as easy as in the shop. I delivered the design of four key screens."),
                        ("Pantallas de una tienda online en un móvil", "Screens of an online shop on a phone")),
            ], cols=3, gap=28)], anchor="proyectos", gap=36),
        bloque_cta(("¿Tienes algo parecido en mente?", "Do you have something similar in mind?"),
                   ("Cuéntame qué necesitas y te digo si puedo ayudarte, cuánto tardaría y cuánto costaría.",
                    "Tell me what you need and I will say whether I can help, how long it would take and what it would cost."),
                   [proyecto(), whatsapp("ghost")], tono="relleno"),
    ])

    servicios = pagina("servicios", ("servicios", "services"), ("Servicios y tarifas", "Services and rates"),
                       ("Qué hago, cómo trabajo en cuatro pasos, tarifas por proyecto de ejemplo y respuestas a las dudas más habituales.",
                        "What I do, how I work in four steps, example per-project rates and answers to the most common questions."), [
        hero(("Servicios y tarifas", "Services and rates"),
             ("Trabajo por proyecto, con un presupuesto cerrado antes de empezar. Estas son las tarifas habituales.",
              "I work per project, with a fixed quote before I start. These are my usual rates."),
             [proyecto(), llamar("secondary")], variante="texto"),
        bloque_valores(("Qué hago", "What I do"), [
            (("Identidad visual", "Visual identity"),
             ("Logotipo, colores, tipografías y una guía corta para usarlos sin equivocarte.",
              "Logo, colours, typefaces and a short guide so you can use them without mistakes.")),
            (("Diseño web", "Web design"),
             ("Páginas sencillas, que se ven bien en el móvil y en el ordenador, con tus textos y tus fotos.",
              "Simple pages that look good on a phone and on a computer, with your own texts and photos.")),
            (("Ilustración", "Illustration"),
             ("Una pieza o una serie, con boceto previo para que veas el camino antes de pintar.",
              "One piece or a series, with a sketch first so you can see the direction before I paint.")),
            (("Material impreso y editorial", "Print and editorial"),
             ("Carteles, folletos, catálogos y cartas, con los archivos listos para la imprenta.",
              "Posters, leaflets, catalogues and menus, with files ready for the printer.")),
        ], cols=2, estilo="borde", tono="base", anchor="que-hago"),
        bloque_pasos(("Cómo trabajo", "How I work"), [
            (("Conversación", "Conversation"),
             ("Me cuentas qué necesitas y para quién. Si no puedo ayudarte bien, te lo digo.",
              "You tell me what you need and who it is for. If I cannot help you well, I say so.")),
            (("Propuesta cerrada", "Fixed proposal"),
             ("Recibes por escrito qué incluye, cuánto cuesta y cuánto tardo. Sin sorpresas.",
              "You get in writing what is included, what it costs and how long it takes. No surprises.")),
            (("Trabajo por fases", "Work in stages"),
             ("Te enseño boceto, propuesta y versión final. En cada fase me dices qué cambiarías.",
              "I show you a sketch, a proposal and the final version. At each stage you tell me what you would change.")),
            (("Entrega", "Handover"),
             ("Archivos listos para usar y un documento corto que explica cómo usarlos.",
              "Files ready to use and a short document explaining how to use them.")),
        ], tono="relleno", anchor="proceso"),
        bloque_precios(("Tarifas por proyecto", "Rates per project"), ("Tarifas de ejemplo por proyecto", "Example rates per project"),
                       [("Proyecto", "Project"), ("Qué incluye", "What is included"), ("Precio", "Price")],
                       [(("Logotipo", "Logo"), ("Dos propuestas y una ronda de cambios", "Two proposals and one round of changes"), desde(350)),
                        (("Identidad visual", "Visual identity"), ("Logotipo, colores, tipografías y guía de uso", "Logo, colours, typefaces and usage guide"), desde(900)),
                        (("Web de una página", "One-page website"), ("Diseño para móvil y ordenador", "Design for phone and computer"), desde(800)),
                        (("Web de hasta cinco páginas", "Website of up to five pages"), ("Diseño para móvil y ordenador", "Design for phone and computer"),
                         ("Desde 1.800 €", "From €1,800")),
                        (("Ilustración", "Illustration"), ("Una pieza, con boceto previo", "One piece, with a preliminary sketch"), desde(120)),
                        (("Encargos pequeños", "Small jobs"), ("Retoques y ajustes sueltos", "Touch-ups and small adjustments"),
                         ("40 € la hora", "€40 per hour"))],
                       intro=("No cobro por horas salvo en lo pequeño: el precio del proyecto se acuerda antes y no cambia.",
                              "I do not charge by the hour except for small jobs: the project price is agreed beforehand and does not change."),
                       nota=NOTA, anchor="tarifas", tono="base"),
        bloque_faq(("Preguntas frecuentes", "Frequently asked questions"), [
            (("¿Cuántas rondas de cambios incluye?", "How many rounds of changes are included?"),
             ("Una por fase. Si hace falta más, te digo cuánto tiempo supone antes de empezar.",
              "One per stage. If more is needed, I tell you how much time it takes before I start.")),
            (("¿Cuánto tarda un proyecto?", "How long does a project take?"),
             ("Un logotipo, entre dos y tres semanas. Una web pequeña, entre cuatro y seis. Depende mucho de lo rápido que me llegan los textos y las fotos.",
              "A logo takes two to three weeks. A small website takes four to six. It depends a lot on how quickly I receive the texts and photos.")),
            (("¿Cómo se paga?", "How do I pay?"),
             ("La mitad al empezar y la otra mitad al entregar, por transferencia. Te mando la factura.",
              "Half at the start and half on delivery, by bank transfer. I send you the invoice.")),
            (("¿De quién son los archivos?", "Who owns the files?"),
             ("Tuyos, cuando pagas el total. Las condiciones de uso quedan por escrito en el presupuesto.",
              "Yours, once you have paid in full. The terms of use are set out in writing in the quote.")),
            (("¿Trabajas con clientes de otra ciudad?", "Do you work with clients in other cities?"),
             ("Sí. Trabajo a distancia, por correo y videollamada.", "Yes. I work remotely, by email and video call.")),
        ], tono="relleno"),
        bloque_cta(("¿Te preparo un presupuesto?", "Shall I prepare a quote?"), None, [proyecto(), whatsapp("ghost")], tono="base"),
    ])

    sobre = pagina("sobre-mi", ("sobre-mi", "about"), ("Sobre mí", "About me"),
                   ("Quién soy, cómo me gusta trabajar, qué proyectos me encajan y cuáles prefiero no aceptar.",
                    "Who I am, how I like to work, which projects suit me and which I prefer not to take on."), [
        hero(("Sobre mí", "About me"),
             ("Hola, soy Nombre Apellido. Diseño de forma independiente, a distancia y con un proyecto cada vez.",
              "Hello, I am First Last. I design independently, remotely and one project at a time."),
             [proyecto(), whatsapp("ghost")],
             imagen_=("portrait", ("Retrato de la persona que diseña, sentada junto a su mesa de trabajo",
                                   "Portrait of the designer, seated beside their desk"))),
        bloque_texto(("Cómo me gusta trabajar", "How I like to work"), [
            ("Prefiero pocos proyectos y hacerlos bien. Contesto yo, no un equipo: cuando me escribes, hablas con la persona que diseña.",
             "I prefer few projects, done well. I reply myself, not a team: when you write to me, you talk to the person who designs."),
            ("Empiezo en papel. Si la idea no se entiende en un boceto a lápiz, tampoco se entenderá en pantalla.",
             "I start on paper. If an idea cannot be understood in a pencil sketch, it will not be understood on screen either."),
            ("Te explico mis decisiones con palabras sencillas, para que puedas defenderlas ante tu socio, tu familia o tus clientes.",
             "I explain my decisions in plain words, so you can stand by them with your partner, your family or your clients."),
        ], eyebrow=("En primera persona", "In the first person"),
            imagen_=("photo", ("Mesa de trabajo con bocetos a lápiz, una libreta y una pantalla",
                               "Desk with pencil sketches, a notebook and a screen")), tono="base", invertir=True),
        bloque_checks(("Los proyectos que mejor me encajan", "The projects that suit me best"), [
            ("Negocios que empiezan o que quieren ordenar su imagen.", "Businesses that are starting out or want to tidy up their image."),
            ("Una sola persona de tu parte que decide y responde.", "One person on your side who decides and replies."),
            ("Plazos razonables y comentarios claros, como «me gusta esto, cambiaría aquello».", "Reasonable deadlines and clear feedback, like \"I like this, I would change that\"."),
            ("Encargos donde el diseño resuelve algo concreto: vender, explicar, orientar.", "Jobs where design solves something concrete: selling, explaining, guiding."),
        ], tono="relleno", cols=2),
        bloque_valores(("A veces digo que no", "Sometimes I say no"), [
            (("Pruebas sin pagar", "Unpaid trials"),
             ("No participo en concursos ni hago pruebas gratis. Enseño mi trabajo anterior y hablamos.",
              "I do not enter contests or do free trials. I show my previous work and we talk.")),
            (("Plazos imposibles", "Impossible deadlines"),
             ("Si no llego con calidad, te lo digo desde el principio y te recomiendo otra opción.",
              "If I cannot deliver good quality in time, I say so from the start and suggest another option.")),
            (("Copiar a otros", "Copying others"),
             ("Me puedes traer referencias, pero no hago una copia de la marca de otra persona.",
              "You can bring references, but I will not make a copy of someone else's brand.")),
        ], tono="base", estilo="linea"),
        bloque_cta(("¿Trabajamos juntos?", "Shall we work together?"), None, [proyecto(), llamar("ghost")], tono="relleno"),
    ])

    campos = [campo("nombre", "text", ("Nombre", "Name"), True, auto="name", maxlen=120),
              campo("correo", "email", ("Correo electrónico", "Email"), True, auto="email", maxlen=200),
              campo("telefono", "tel", ("Teléfono (opcional)", "Phone (optional)"), False, auto="tel", maxlen=30),
              campo("tipo", "select", ("Tipo de proyecto", "Type of project"), True,
                    opciones=[("Identidad visual", "Visual identity"), ("Diseño web", "Web design"),
                              ("Ilustración", "Illustration"), ("Material impreso", "Printed material"),
                              ("Otra cosa", "Something else")]),
              campo("plazo", "select", ("¿Para cuándo lo necesitas?", "When do you need it?"), False,
                    opciones=[("Lo antes posible", "As soon as possible"), ("En uno o dos meses", "In one or two months"),
                              ("Sin prisa", "No rush")]),
              campo("referencias", "text", ("Enlace con referencias (opcional)", "Link with references (optional)"), False, maxlen=300),
              campo("mensaje", "textarea", ("Cuéntame qué necesitas", "Tell me what you need"), True, maxlen=2000)]
    datos = marco([
        titular(("Escríbeme o llámame", "Write to me or call me"),
                ("Cuéntame qué quieres hacer y para quién. Con eso te contesto con una idea de plazo y de precio.",
                 "Tell me what you want to do and who it is for. With that I will reply with an idea of timing and price.")),
        marco([icono("mail"), p((CORREO, CORREO))], dir="row", gap=12, align="start"),
        marco([icono("phone"), p((TEL_VISIBLE, TEL_VISIBLE))], dir="row", gap=12, align="start"),
        marco([icono("map-pin"), p(("Trabajo a distancia desde Tu ciudad.", "I work remotely from Your city."))],
              dir="row", gap=12, align="start"),
        marco([llamar(), whatsapp()], dir="row", gap=12, wrap=True),
        tarjeta([etiqueta(("Disponibilidad", "Availability")),
                 p(("Ejemplo: tengo hueco para un proyecto nuevo el mes que viene. Respondo en uno o dos días laborables. Cambia este texto cuando cambie tu agenda.",
                    "Example: I have room for a new project next month. I reply within one or two working days. Change this text when your schedule changes."))],
                estilo="relleno", pad=20, tag="div", gap=8),
    ], gap=18, sz={"width": "fill", "height": "hug", "min_width": 0})
    form = tarjeta([formulario(campos, ("Enviar el proyecto", "Send the project"),
                               ("Gracias. He recibido tu mensaje y te respondo con una idea de plazo y de precio.",
                                "Thank you. I have received your message and will reply with an idea of timing and price."))],
                   estilo="relleno", pad=28, tag="div", sz={"width": "fill", "height": "hug", "min_width": 0})
    contacto = pagina("contacto", ("contacto", "contact"), ("Contacto", "Contact"),
                      ("Cuéntame tu proyecto con el formulario, por correo, por teléfono o por WhatsApp. Trabajo a distancia.",
                       "Tell me about your project with the form, by email, by phone or on WhatsApp. I work remotely."), [
        hero(("Cuéntame tu proyecto", "Tell me about your project"),
             ("Escríbeme, llámame o rellena el formulario. No hace falta tenerlo todo claro: lo vamos aclarando juntos.",
              "Write to me, call me or fill in the form. You do not need to have it all worked out: we will sort it out together."),
             [llamar(), whatsapp()], variante="texto"),
        seccion("contact", [marco([datos, form], dir="row", gap=48, align="start",
                                  bp={"tablet": {"layout": {"gap": 32}}, "mobile": {"layout": {"direction": "column", "gap": 28}}})],
                anchor="contacto", gap=0),
    ])

    return plantilla(KEY, ("Estudio creativo y portfolio", "Creative studio and portfolio"),
                     ("Portfolio de diseñador freelance con trabajos en rejilla, servicios, proceso en cuatro pasos, tarifas por proyecto de ejemplo, página sobre mí y formulario para contar el proyecto.",
                      "Freelance designer portfolio with a grid of work, services, a four-step process, example per-project rates, an about page and a form to describe the project."),
                     "creativos", ("portfolio portafolio disenador disenadora freelance diseno grafico diseno web ilustrador ilustracion identidad visual logotipo marca estudio creativo autonomo",
                                   "portfolio designer freelance graphic design web design illustrator illustration brand identity logo creative studio"),
                     look, cabecera_,
                     pie(nav, ("Diseño de identidad, web e ilustración, por proyectos y a distancia.",
                               "Identity, web and illustration design, per project and remotely.")),
                     [inicio, trabajos, servicios, sobre, contacto],
                     exento={"mapa": "profesional que trabaja a distancia: no atiende al público en un local",
                             "horario": "sin horario de apertura: atiende por correo y mensaje, y la disponibilidad va en texto"})
