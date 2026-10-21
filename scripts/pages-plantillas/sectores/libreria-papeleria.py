# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
from lib import *

KEY = "libreria-papeleria"

# Papel crema y verde de tapete. No lleva fondo oscuro: primary/inverse no llega a 3.
TAPETE = dict(
    colores=dict(background="#fbf8f1", surface="#ece8d8", text="#1b2420", muted="#56594c", primary="#245c3c",
                 on_primary="#ffffff", border="#dcd8c3", inverse="#1b3326", on_inverse="#f3f1e3"),
    fuentes=("source-serif-4", "ibm-plex-sans"), radio=(2, 6, 10), ancho=1140)

NOTA_PLAZOS = ("Plazos y precios de ejemplo: cámbialos por los tuyos.",
               "Example times and prices: replace them with your own.")


def build():
    nueva(KEY)
    look = TAPETE
    nav = [("inicio", ("Inicio", "Home")), ("tienda", ("Tienda", "Shop")),
           ("encargos", ("Encargos y material escolar", "Orders and school supplies")),
           ("contacto", ("Contacto", "Contact"))]
    horas = [(("Lunes a viernes", "Monday to Friday"), ("9:30 a 14:00 y 16:30 a 20:00", "9:30 am to 2:00 pm and 4:30 pm to 8:00 pm")),
             (("Sábado", "Saturday"), ("10:00 a 14:00", "10:00 am to 2:00 pm")),
             (("Domingo", "Sunday"), ("Cerrado", "Closed"))]
    cta_cab = marco([llamar(t=("Llamar", "Call")), whatsapp("ghost", ("WhatsApp", "WhatsApp"))],
                    dir="row", gap=8, align="center")

    def a_tienda():
        return boton(("Ver la tienda", "Browse the shop"), a_pagina("tienda", "catalogo"))

    def pedir():
        return boton(("Encargar un libro", "Order a book"), a_pagina("encargos", "libro"), "secondary")

    secciones_tienda = [
        (("Libros", "Books"),
         ("Novela, ensayo, infantil y juvenil, cómic y libros de cocina o de viajes. Si no lo tenemos, te lo pedimos.",
          "Fiction, non-fiction, children's and young adult, comics, and cookery or travel books. If we do not have it, we order it for you."),
         "libros"),
        (("Papelería", "Stationery"),
         ("Cuadernos, agendas, bolígrafos, rotuladores, archivadores y todo lo de la mesa de trabajo.",
          "Notebooks, planners, pens, markers, binders and everything for the work desk."),
         "papeleria"),
        (("Material escolar", "School supplies"),
         ("Mochilas, estuches, lápices, colores, pinturas y material de manualidades para el cole.",
          "Backpacks, pencil cases, pencils, crayons, paints and craft supplies for school."),
         "escolar"),
    ]

    inicio = pagina("inicio", ("", ""), ("Librería y papelería", "Bookshop and stationer's"),
                    ("Librería y papelería con tienda online: libros, papelería y material escolar, encargos, recogida en tienda y lista del colegio.",
                     "Bookshop and stationer's with an online shop: books, stationery and school supplies, special orders, in-store pickup and school lists."), [
        hero(("Libros, papel y material del cole, cerca de casa", "Books, paper and school supplies, close to home"),
             ("Mira el catálogo desde casa, pídelo y recógelo en la tienda. Y si buscas un libro que no vemos aquí, te lo traemos.",
              "Browse the catalogue from home, order it and collect it in the shop. And if you want a book you cannot see here, we will get it for you."),
             [a_tienda(), pedir(), whatsapp("ghost")],
             eyebrow=("Librería y papelería", "Bookshop and stationer's"),
             imagen_=("photo", ("Estantería de libros y mostrador de la librería", "Bookshelves and counter of the bookshop"))),
        seccion("features", [
            titular(("Tres secciones, una tienda", "Three sections, one shop"), None, ("La tienda", "The shop")),
            rejilla([tarjeta([h3(n), p(t, color="$muted"),
                              boton(("Ver qué hay", "See what is in it"), a_pagina("tienda", "secciones"), "ghost")], estilo="linea")
                     for n, t, a in secciones_tienda], cols=3)], tono="base", anchor="tres-secciones"),
        bloque_productos(("Novedades", "New in"),
                         ("Lo último que ha entrado, de libros y de papelería. En la página Tienda está todo el catálogo.",
                          "The latest arrivals, books and stationery. The full catalogue is on the Shop page."),
                         limite=4, cols=4, tono="relleno", anchor="novedades",
                         boton_=boton(("Ver todo el catálogo", "See the whole catalogue"), a_pagina("tienda", "catalogo"), "secondary")),
        bloque_pasos(("Un libro que no tenemos", "A book we do not have"), [
            (("Dinos cuál", "Tell us which"),
             ("Título y autor, o el ISBN si lo tienes. Por la web, por WhatsApp o en el mostrador.",
              "Title and author, or the ISBN if you have it. On the web, on WhatsApp or at the counter.")),
            (("Lo pedimos", "We order it"),
             ("Lo encargamos al distribuidor y te decimos cuándo llegará.", "We order it from the distributor and tell you when it will arrive.")),
            (("Te avisamos", "We let you know"),
             ("Cuando llega, te escribimos o te llamamos y lo guardamos para ti.", "When it arrives, we message or call you and keep it for you.")),
            (("Lo recoges", "You collect it"),
             ("En la tienda, sin gastos de envío. El precio es el mismo en toda librería: el libro tiene precio fijo.",
              "In the shop, with no shipping cost. The price is the same in every bookshop: books have a fixed price.")),
        ], intro=("Sin compromiso de compra hasta que lo tienes en la mano.", "No obligation to buy until you have it in your hands."),
            eyebrow=("Encargos", "Special orders"), tono="base", anchor="encargo-resumen"),
        bloque_texto(("La lista del colegio, resuelta", "The school list, sorted"),
                     [("Trae la lista que te dio el centro, o una foto por WhatsApp. Preparamos el pedido entero, con cada cosa en la marca y la medida que pide el profesor.",
                       "Bring the list the school gave you, or a photo on WhatsApp. We prepare the whole order, with each item in the brand and size the teacher asks for."),
                      ("Lo recoges ya preparado y pagas una sola vez.", "You collect it ready and pay once.")],
                     eyebrow=("Vuelta al cole", "Back to school"),
                     imagen_=("photo", ("Mochila, estuche y cuadernos sobre una mesa", "Backpack, pencil case and notebooks on a table")),
                     tono="relleno", invertir=True, anchor="lista-escolar"),
        bloque_opiniones(("Lo que dicen nuestros clientes", "What our customers say"),
                         [("un encargo", "a special order"), ("la atención", "the service"), ("el material escolar", "the school supplies")],
                         tono="base"),
        bloque_cta(("¿Buscas algo y no lo ves?", "Looking for something you cannot see?"),
                   ("Escríbenos y te decimos si lo tenemos o cuándo podemos tenerlo.",
                    "Message us and we will tell you whether we have it or when we can get it."), tono="relleno"),
    ])

    tienda = pagina("tienda", ("tienda", "shop"), ("Tienda", "Shop"),
                    ("Catálogo de libros, papelería y material escolar con carrito, recogida gratis en tienda y envío a domicilio.",
                     "Catalogue of books, stationery and school supplies with a cart, free in-store pickup and home delivery."), [
        hero(("Tienda", "Shop"),
             ("Añade al carrito y paga online. Recoge en la tienda sin coste o recíbelo en casa.",
              "Add to the cart and pay online. Collect in the shop at no cost or have it delivered to your door."),
             [boton(("Ver el catálogo", "See the catalogue"), a_ancla("catalogo"), "secondary"), whatsapp("ghost")],
             variante="texto"),
        bloque_productos(("Todo el catálogo", "Full catalogue"), None, limite=24, cols=4, anchor="catalogo"),
        bloque_valores(("Qué hay en cada sección", "What is in each section"),
                       [(n, t) for n, t, _ in secciones_tienda], tono="relleno", estilo="borde", anchor="secciones"),
        bloque_checks(("Cómo se recibe", "How you get it"), [
            ("Recogida en tienda, gratis, normalmente en 24 horas.", "In-store pickup, free, usually within 24 hours."),
            ("Envío a domicilio con número de seguimiento.", "Home delivery with a tracking number."),
            ("Si el libro no está en el catálogo, encárgalo: te lo pedimos.", "If the book is not in the catalogue, order it: we will get it for you."),
            ("Cambios y devoluciones dentro de 14 días, con el ticket o el pedido.", "Exchanges and returns within 14 days, with the receipt or order.")],
            eyebrow=("Antes de pagar", "Before you pay"), tono="base", anchor="entrega"),
        seccion("text", [marco([h2(("Y en la papelería", "And in the stationer's")),
                                p(("Si necesitas una cantidad grande de algo (cuadernos para una clase, material de oficina o regalos para una empresa), escríbenos y te hacemos precio.",
                                   "If you need a large quantity of something (notebooks for a class, office supplies or gifts for a company), message us and we will quote you."))],
                               gap=16, sz={"width": "fill", "height": "hug", "max_width": 760})],
                tono="relleno", anchor="pedidos-grandes", gap=0),
        bloque_cta(("¿No encuentras un libro?", "Cannot find a book?"),
                   ("Dinos título y autor y te lo pedimos. Mira cómo funciona en Encargos.", "Give us the title and author and we will order it. See how it works in Orders."),
                   [pedir(), whatsapp("ghost")], tono="base"),
    ])

    enc = pagina("encargos", ("encargos", "orders"), ("Encargos y material escolar", "Orders and school supplies"),
                 ("Cómo encargar un libro, recoger tu pedido en tienda, preparar la lista de material del colegio y las actividades de la librería.",
                  "How to order a book, collect your order in the shop, prepare the school supply list and the bookshop's activities."), [
        hero(("Encargos y material escolar", "Orders and school supplies"),
             ("Cómo pedirnos un libro, cuándo puedes recogerlo y cómo preparamos la lista del cole.",
              "How to ask us for a book, when you can collect it and how we prepare the school list."),
             [boton(("Lista del colegio", "School list"), a_ancla("lista"), "secondary"), whatsapp("ghost")], variante="texto"),
        bloque_pasos(("Encargar un libro", "Order a book"), [
            (("Dinos cuál", "Tell us which"), ("Título, autor y, si lo sabes, editorial o ISBN.", "Title, author and, if you know it, publisher or ISBN.")),
            (("Te damos fecha", "We give you a date"),
             ("Lo pedimos al distribuidor y te decimos cuándo estará.", "We order it from the distributor and tell you when it will be here.")),
            (("Aviso", "Notification"), ("Te escribimos en cuanto llega y te lo guardamos unos días.", "We message you as soon as it arrives and keep it for you for a few days.")),
            (("Recogida y pago", "Collection and payment"), ("En la tienda. Pagas al recogerlo, o antes si prefieres.", "In the shop. You pay on collection, or beforehand if you prefer.")),
        ], tono="base", anchor="libro"),
        bloque_precios(("Plazos y recogida", "Times and collection"), ("Plazos orientativos de entrega", "Approximate delivery times"),
                       [("Qué", "What"), ("Cuándo", "When"), ("Coste", "Cost")],
                       [(("Libro de catálogo con existencias", "Catalogue book in stock"), ("En 24 horas", "Within 24 hours"), ("Sin coste", "No cost")),
                        (("Libro que hay que pedir al distribuidor", "Book to order from the distributor"), ("2 a 4 días laborables", "2 to 4 working days"), ("Sin coste", "No cost")),
                        (("Libro descatalogado o de editorial pequeña", "Out-of-print or small-press book"), ("Te lo confirmamos al buscarlo", "We confirm once we have looked"), ("Sin coste", "No cost")),
                        (("Envío a domicilio", "Home delivery"), ("2 a 3 días laborables", "2 to 3 working days"), eur(4.5))],
                       nota=NOTA_PLAZOS, anchor="plazos", tono="relleno"),
        bloque_pasos(("La lista del colegio", "The school list"), [
            (("Tráenos la lista", "Bring us the list"),
             ("En papel o en foto por WhatsApp, con el curso y el nombre del colegio.", "On paper or as a photo on WhatsApp, with the year and the name of the school.")),
            (("La preparamos", "We prepare it"),
             ("Reunimos cada cosa en la marca y la medida que pide el profesor. Lo que falta, lo pedimos.",
              "We gather each item in the brand and size the teacher asks for. What is missing, we order.")),
            (("Lo recoges", "You collect it"), ("Te avisamos cuando está todo. Pagas una vez y te llevas una sola bolsa.", "We tell you when everything is ready. You pay once and take away a single bag.")),
        ], intro=("Mejor con tiempo: en septiembre todos vamos a la vez.", "Earlier is better: in September everyone comes at once."),
            eyebrow=("Material escolar", "School supplies"), tono="base", anchor="lista"),
        bloque_valores(("Actividades en la librería", "Activities at the bookshop"), [
            (("Club de lectura", "Book club"),
             ("Un libro al mes y una tarde para comentarlo (actividad de ejemplo: cámbiala por la tuya).",
              "One book a month and an afternoon to talk about it (example activity: replace it with your own).")),
            (("Cuentacuentos", "Storytelling"),
             ("Una sesión para peques con cuentos en voz alta (actividad de ejemplo: cámbiala por la tuya).",
              "A session for little ones with stories read aloud (example activity: replace it with your own).")),
            (("Presentaciones y firmas", "Launches and signings"),
             ("Autores y autoras que pasan a presentar su libro (actividad de ejemplo: cámbiala por la tuya).",
              "Authors who drop by to present their book (example activity: replace it with your own).")),
        ], intro=("Los días y las plazas los publicamos en la tienda y por WhatsApp.", "We post days and places in the shop and on WhatsApp."),
            tono="relleno", estilo="borde", anchor="actividades"),
        bloque_faq(("Preguntas frecuentes", "Frequently asked questions"), [
            (("¿Tengo que pagar el libro al encargarlo?", "Do I have to pay for the book when I order it?"),
             ("No. Se paga al recogerlo. Si es un libro caro o raro, te podemos pedir una señal; te lo decimos antes.",
              "No. You pay on collection. If it is an expensive or rare book we may ask for a deposit; we tell you beforehand.")),
            (("¿Cuánto tiempo me lo guardáis?", "How long do you keep it for me?"),
             ("Unos días desde el aviso. Si necesitas más, dínoslo.", "A few days from the notification. If you need more, tell us.")),
            (("¿Puedo cambiar un libro?", "Can I exchange a book?"),
             ("Sí, con el ticket y en buen estado, dentro de los 14 días. Los libros que encargaste, solo si llegan defectuosos.",
              "Yes, with the receipt and in good condition, within 14 days. Books you ordered, only if they arrive faulty.")),
            (("¿Y si el colegio cambia la lista?", "What if the school changes the list?"),
             ("Avísanos y ajustamos el pedido antes de que lo recojas.", "Let us know and we adjust the order before you collect it.")),
        ], tono="base"),
        bloque_cta(("¿Preparamos tu pedido?", "Shall we prepare your order?"), None, [whatsapp("primary"), llamar("ghost")], tono="relleno"),
    ])

    cont = pagina("contacto", ("contacto", "contact"), ("Contacto", "Contact"),
                  ("Horario de la librería, cómo llegar, teléfono, WhatsApp y formulario para encargos y consultas.",
                   "Bookshop hours, how to get here, phone, WhatsApp and a form for orders and enquiries."), [
        hero(("Contacto", "Contact"),
             ("Llámanos, escríbenos por WhatsApp o pásate por la tienda. Para un encargo, basta con el título y el autor.",
              "Call us, message us on WhatsApp or come by the shop. For a special order, the title and author are enough."),
             [llamar(), whatsapp()], variante="texto"),
        bloque_contacto(("Estamos en la tienda", "We are in the shop"),
                        ("Si ya has hecho un pedido online, dinos el número que te llegó por correo.",
                         "If you have already placed an online order, tell us the number you received by email."),
                        campos_consulta(servicios=[("Encargar un libro", "Order a book"), ("Un pedido de la tienda", "A shop order"),
                                                   ("Lista del colegio", "School list"), ("Otra cosa", "Something else")]),
                        ("Enviar consulta", "Send enquiry"), horas),
        bloque_mapa(("Cómo llegar a la tienda", "How to get to the shop"), tono="relleno"),
    ])

    return plantilla(KEY, ("Librería y papelería", "Bookshop and stationer's"),
                     ("Librería y papelería con catálogo y carrito, encargos de libros, recogida en tienda, lista del colegio, actividades y botones de llamar y WhatsApp.",
                      "Bookshop and stationer's with a catalogue and cart, book orders, in-store pickup, school list, activities and call and WhatsApp buttons."),
                     "comercio", ("libreria papeleria libros libreria online tienda online material escolar cuadernos lista del colegio vuelta al cole encargos club de lectura comercio",
                                  "bookshop book shop bookstore stationery stationer online shop school supplies school list back to school book orders book club"),
                     look, cabecera(nav, cta_cab, tienda=True),
                     pie(nav, ("Librería y papelería de barrio, en la tienda y online.",
                               "Neighbourhood bookshop and stationer's, in the shop and online."),
                         ("Lunes a viernes 9:30 a 14:00 y 16:30 a 20:00. Sábado 10:00 a 14:00.",
                          "Monday to Friday 9:30 am to 2 pm and 4:30 pm to 8 pm. Saturday 10 am to 2 pm.")),
                     [inicio, tienda, enc, cont], tienda=True)
