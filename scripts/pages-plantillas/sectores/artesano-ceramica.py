# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
from lib import *
from looks import LOOKS  # noqa: F401

KEY = "artesano-ceramica"

# Paleta propia: gres claro y azul pizarra de esmalte. Pasa AA. Sin `tono="oscuro"`: primary/inverse no llega a 3.
GRES = dict(
    colores=dict(background="#f7f6f3", surface="#eae8e3", text="#1c1e22", muted="#555a63", primary="#3b4a6b",
                 on_primary="#ffffff", border="#d6d3cc", inverse="#1f2638", on_inverse="#f1f3f8"),
    fuentes=("noto-serif", "work-sans"), peso=600, radio=(3, 8, 16), ancho=1120)


def build():
    nueva(KEY)
    look = GRES
    nav = [("inicio", ("Inicio", "Home")), ("tienda", ("Tienda", "Shop")),
           ("taller", ("Taller y cursos", "Workshop and courses")), ("contacto", ("Contacto", "Contact"))]
    horas = [(("Martes a viernes", "Tuesday to Friday"), ("10:00 a 14:00", "10:00 am to 2:00 pm")),
             (("Sábado", "Saturday"), ("10:00 a 14:00 (solo con cita)", "10:00 am to 2:00 pm (by appointment only)")),
             (("Lunes y domingo", "Monday and Sunday"), ("Cerrado", "Closed"))]
    nota = ("Precios de ejemplo: cámbialos por los tuyos.", "Example prices: replace them with your own.")
    a_tienda = a_pagina("tienda", "catalogo")

    def visita(var="primary", t=("Reservar visita al taller", "Book a studio visit")):
        return boton(t, a_pagina("contacto", "cita"), var)

    inicio = pagina("inicio", ("", ""), ("Taller de cerámica", "Ceramics studio"),
                    ("Taller de cerámica con tienda online: piezas hechas a mano, encargos, cursos y visitas al taller con cita.",
                     "Ceramics studio with an online shop: handmade pieces, commissions, courses and studio visits by appointment."), [
        hero(("Cerámica hecha a mano, una pieza cada vez", "Handmade ceramics, one piece at a time"),
             ("Tazas, platos y jarrones que se modelan, se esmaltan y se cuecen en nuestro taller. Cada pieza sale un poco distinta y eso es lo bueno.",
              "Mugs, plates and vases that are shaped, glazed and fired in our studio. Every piece comes out a little different, and that is the point."),
             [boton(("Ver la tienda", "Browse the shop"), a_tienda), llamar("secondary"), whatsapp("ghost")],
             eyebrow=("Taller de cerámica", "Ceramics studio"),
             imagen_=("product", ("Cuenco y taza de cerámica esmaltada hechos a mano", "Handmade glazed ceramic bowl and mug"))),
        bloque_productos(("Piezas en la tienda", "Pieces in the shop"),
                         ("Lo que hay ahora mismo, listo para enviar. Cuando una pieza se acaba, no siempre se repite.",
                          "What is available right now, ready to ship. When a piece sells out, it is not always made again."),
                         limite=4, cols=4, tono="base", anchor="novedades",
                         boton_=boton(("Ver toda la tienda", "See the whole shop"), a_tienda, "secondary")),
        bloque_valores(("Tres formas de tener una pieza", "Three ways to get a piece"), [
            (("De la tienda", "From the shop"),
             ("Piezas ya hechas, listas para enviar con embalaje protector.",
              "Ready-made pieces, ready to ship with protective packaging.")),
            (("Por encargo", "Made to order"),
             ("Un juego de vajilla, una pieza con tu color o un regalo con nombre. Te decimos plazo y precio antes de empezar.",
              "A dinner set, a piece in your colour or a named gift. We tell you the lead time and price before we start.")),
            (("En un curso", "On a course"),
             ("Aprende a modelar y llévate a casa lo que hayas hecho.", "Learn to shape clay and take home what you make.")),
        ], tono="relleno", estilo="borde", cols=3, anchor="formas"),
        bloque_texto(("Hecho con las manos y con tiempo", "Made by hand and given time"), [
            ("Una pieza pasa por varias semanas de trabajo: se modela, se deja secar, se cuece, se esmalta y se cuece otra vez. No se puede acelerar sin que se rompa.",
             "A piece goes through several weeks of work: it is shaped, left to dry, fired, glazed and fired again. It cannot be rushed without breaking."),
            ("Por eso hay pocas piezas de cada modelo y por eso duran.",
             "That is why there are few pieces of each model and why they last."),
        ], imagen_=("photo", ("Manos modelando una pieza en el torno de alfarero", "Hands shaping a piece on the potter's wheel")),
            tono="base", eyebrow=("El taller", "The studio"), invertir=True),
        bloque_opiniones(("Lo que dicen quienes las usan", "What people who use them say"),
                         [("una taza", "a mug"), ("el envío", "the delivery"), ("un curso", "a course")], tono="relleno"),
        bloque_cta(("¿Quieres ver el taller?", "Would you like to see the studio?"),
                   ("Pásate con cita: ves cómo trabajamos y eliges las piezas en persona.",
                    "Drop by with an appointment: see how we work and choose pieces in person."),
                   [visita(), whatsapp("ghost")], tono="base"),
    ])

    tienda = pagina("tienda", ("tienda", "shop"), ("Tienda", "Shop"),
                    ("Catálogo de cerámica hecha a mano con carrito, envío protegido y consejos para cuidar las piezas.",
                     "Handmade ceramics catalogue with a cart, protected shipping and advice on looking after your pieces."), [
        hero(("Tienda de cerámica", "Ceramics shop"),
             ("Piezas únicas o de serie corta, hechas en el taller. Pídelas desde casa y las enviamos bien protegidas.",
              "One-off or short-run pieces made in the studio. Order from home and we ship them well protected."),
             [boton(("Ver el catálogo", "See the catalogue"), a_ancla("catalogo")), whatsapp("secondary")], variante="texto"),
        bloque_productos(("Catálogo", "Catalogue"),
                         ("El color y el brillo pueden variar un poco de una pieza a otra: es esmalte, no pintura.",
                          "Colour and shine can vary a little between pieces: it is glaze, not paint."),
                         limite=24, cols=4, anchor="catalogo"),
        bloque_valores(("Envío con protección", "Protected shipping"), [
            (("Embalaje que aguanta", "Packaging that holds"),
             ("Cada pieza va envuelta una a una y con relleno que amortigua los golpes.",
              "Each piece is wrapped individually, with padding that cushions knocks.")),
            (("Plazo de envío", "Shipping time"),
             ("Escribe aquí cuándo sale el pedido (texto de ejemplo: preparamos tu pedido en 2 a 4 días laborables).",
              "Write here when orders go out (example text: we prepare your order within 2 to 4 working days).")),
            (("Si llega roto", "If it arrives broken"),
             ("Hazle una foto al paquete y a la pieza y escríbenos el mismo día. Te la reponemos o te devolvemos el dinero.",
              "Take a photo of the parcel and the piece and write to us the same day. We replace it or refund you.")),
        ], intro=("Los gastos de envío se calculan en el carrito. Indica aquí tus condiciones de envío y devolución (texto de ejemplo).",
                  "Shipping costs are worked out in the cart. State your shipping and returns terms here (example text)."),
            tono="relleno", estilo="borde", anchor="envio"),
        bloque_valores(("Cómo cuidar tus piezas", "How to look after your pieces"), [
            (("Lavado", "Washing"),
             ("A mano con agua tibia y un paño suave. Si el esmalte lo permite, también en lavavajillas con programa suave. Indica aquí qué admite cada pieza (texto de ejemplo).",
              "By hand with warm water and a soft cloth. If the glaze allows it, also in a dishwasher on a gentle cycle. State here what each piece can take (example text).")),
            (("Calor y frío", "Heat and cold"),
             ("Evita los cambios bruscos de temperatura: una taza fría no va directa al fuego ni al horno.",
              "Avoid sudden changes of temperature: a cold mug should not go straight onto a flame or into an oven.")),
            (("Microondas y horno", "Microwave and oven"),
             ("Solo si lo pone en la ficha de la pieza. Si no lo pone, mejor no.",
              "Only if the piece description says so. If it does not, better not.")),
            (("Alimentos", "Food"),
             ("Escribe aquí si el esmalte es apto para uso alimentario (texto de ejemplo: confírmalo con tu proveedor de esmaltes).",
              "Write here whether the glaze is food safe (example text: check with your glaze supplier).")),
        ], cols=2, tono="base", estilo="relleno", anchor="cuidado"),
        bloque_faq(("Preguntas frecuentes", "Frequently asked questions"), [
            (("¿Las piezas son todas iguales?", "Are all the pieces the same?"),
             ("No. Se hacen a mano y cada una tiene su forma, su esmalte y sus pequeñas marcas. La foto es de la pieza que recibes.",
              "No. They are made by hand and each has its own shape, glaze and small marks. The photo is of the piece you receive.")),
            (("¿Puedo devolver una pieza?", "Can I return a piece?"),
             ("Las compras online tienen 14 días para desistir; las piezas hechas por encargo, no. Revisa tus condiciones (texto de ejemplo).",
              "Online purchases have 14 days to withdraw; made-to-order pieces do not. Check your terms (example text).")),
            (("¿Puedo recoger el pedido en el taller?", "Can I collect my order at the studio?"),
             ("Sí, con cita y sin gastos de envío. Elige esa opción al pagar o escríbenos.",
              "Yes, by appointment and with no shipping cost. Choose that option at checkout or write to us.")),
        ], tono="relleno"),
        bloque_cta(("¿Buscas algo que no está en la tienda?", "Looking for something that is not in the shop?"),
                   ("Cuéntanos qué necesitas y vemos si podemos hacértelo.", "Tell us what you need and we will see whether we can make it."),
                   [boton(("Pedir un encargo", "Ask for a commission"), a_pagina("taller", "encargos")), whatsapp("ghost")], tono="base"),
    ])

    taller = pagina("taller", ("taller", "workshop"), ("Taller, encargos y cursos", "Studio, commissions and courses"),
                    ("Encargos de cerámica a medida, proceso paso a paso, cursos con fechas de ejemplo y visitas al taller con cita.",
                     "Made-to-order ceramics, the process step by step, courses with example dates and studio visits by appointment."), [
        hero(("El taller", "The studio"),
             ("Aquí se modela, se esmalta y se cuece. Puedes encargarnos una pieza, aprender con nosotros o venir a verlo.",
              "This is where we shape, glaze and fire. You can order a piece, learn with us or come and see it."),
             [boton(("Pedir un encargo", "Ask for a commission"), a_ancla("encargos")), visita("secondary")], variante="texto"),
        bloque_pasos(("Cómo se hace una pieza", "How a piece is made"), [
            (("Modelado", "Shaping"),
             ("A mano o en el torno, con arcilla de gres. Se le da forma y se alisa.",
              "By hand or on the wheel, with stoneware clay. It is shaped and smoothed.")),
            (("Secado", "Drying"),
             ("Lento y a la sombra, durante días. Si se seca deprisa, se agrieta.",
              "Slow and out of the sun, for days. If it dries too fast, it cracks.")),
            (("Esmaltado", "Glazing"),
             ("Tras una primera cocción, se baña o se pinta con esmalte. El color real aparece solo después del horno.",
              "After a first firing, it is dipped or painted with glaze. The real colour only shows after the kiln.")),
            (("Cocción", "Firing"),
             ("Una segunda cocción a alta temperatura funde el esmalte y deja la pieza dura y lista para usar.",
              "A second firing at high temperature melts the glaze and leaves the piece hard and ready to use.")),
        ], intro=("Cada paso tiene su espera, y por eso un encargo tarda semanas y no días.",
                  "Every step has its wait, which is why a commission takes weeks, not days."),
            eyebrow=("Proceso", "Process"), tono="base", anchor="proceso"),
        bloque_valores(("Piezas por encargo", "Made-to-order pieces"), [
            (("Vajilla y juegos", "Dinner sets"),
             ("Platos, cuencos y tazas a juego, en el color y el número que necesites.",
              "Matching plates, bowls and mugs, in the colour and number you need.")),
            (("Regalos con nombre", "Named gifts"),
             ("Una taza o un plato con un nombre, una fecha o un dibujo, para bodas, nacimientos y aniversarios.",
              "A mug or plate with a name, a date or a drawing, for weddings, births and anniversaries.")),
            (("Para restaurantes y tiendas", "For restaurants and shops"),
             ("Series de piezas iguales para tu local. Presupuesto sin compromiso.",
              "Runs of identical pieces for your venue. Quote with no obligation.")),
        ], intro=("Presupuesto sin compromiso. Plazo orientativo de ejemplo: de 4 a 8 semanas; cámbialo por el tuyo.",
                  "Quote with no obligation. Example lead time: 4 to 8 weeks; replace it with your own."),
            eyebrow=("Encargos", "Commissions"), tono="relleno", estilo="borde", anchor="encargos"),
        bloque_precios(("Cursos y talleres", "Courses and workshops"), ("Cursos y talleres con fechas y precio", "Courses and workshops with dates and price"),
                       [("Curso", "Course"), ("Fecha", "Date"), ("Duración", "Length"), ("Precio", "Price")],
                       [(("Iniciación al modelado a mano", "Introduction to hand building"), ("Sábado 00/00, de ejemplo", "Saturday 00/00, example"), ("3 horas", "3 hours"), eur(45)),
                        (("Primer contacto con el torno", "First time on the wheel"), ("Domingo 00/00, de ejemplo", "Sunday 00/00, example"), ("3 horas", "3 hours"), eur(55)),
                        (("Esmaltado de tus piezas", "Glazing your pieces"), ("Sábado 00/00, de ejemplo", "Saturday 00/00, example"), ("2 horas", "2 hours"), eur(35)),
                        (("Curso de cuatro sesiones", "Four-session course"), ("Martes 00/00, de ejemplo", "Tuesday 00/00, example"), ("4 × 2 horas", "4 × 2 hours"), eur(150))],
                       intro=("Grupos pequeños y el material incluido. Las piezas se cuecen en el taller y las recoges unas semanas después.",
                              "Small groups and materials included. Pieces are fired in the studio and you collect them a few weeks later."),
                       nota=("Fechas y precios de ejemplo: cámbialos por los tuyos.", "Example dates and prices: replace them with your own."),
                       boton_=boton(("Apuntarme a un curso", "Sign up for a course"), a_pagina("contacto", "cita")),
                       anchor="cursos", tono="base"),
        bloque_texto(("Visita al taller", "Studio visit"), [
            ("Si quieres ver cómo trabajamos y elegir piezas en persona, ven con cita. Así estamos libres para enseñártelo sin prisas.",
             "If you want to see how we work and choose pieces in person, come by appointment. That way we are free to show you without rushing."),
            ("Texto de ejemplo: las visitas duran media hora; ajústalo a lo que ofrezcas.",
             "Example text: visits last half an hour; adjust this to what you offer."),
        ], imagen_=("landscape", ("Estantes del taller con piezas de cerámica secándose", "Studio shelves with ceramic pieces drying")),
            tono="relleno", eyebrow=("Con cita", "By appointment"), anchor="visitas"),
        bloque_cta(("¿Te apuntas o vienes a verlo?", "Will you sign up or come and see it?"), None,
                   [visita(), whatsapp("ghost")], tono="base"),
    ])

    cont = pagina("contacto", ("contacto", "contact"), ("Contacto y cita", "Contact and appointments"),
                  ("Reserva tu visita al taller, un curso o un encargo con el formulario, por teléfono o WhatsApp. Horario y cómo llegar.",
                   "Book a studio visit, a course or a commission with the form, by phone or WhatsApp. Opening hours and how to find us."), [
        hero(("Contacto y cita", "Contact and appointments"),
             ("Para visitar el taller, apuntarte a un curso, hacer un encargo o preguntar por un pedido.",
              "To visit the studio, sign up for a course, order a commission or ask about an order."),
             [llamar(), whatsapp()], variante="texto"),
        bloque_contacto(("Cómo encontrarnos", "How to find us"),
                        ("El taller está abierto con cita para no interrumpir el trabajo. Escríbenos y quedamos.",
                         "The studio is open by appointment so we do not interrupt the work. Write to us and we will arrange it."),
                        campos_cita(servicios=[("Visita al taller", "Studio visit"), ("Curso o taller", "Course or workshop"),
                                               ("Encargo a medida", "Commission"), ("Pregunta sobre un pedido", "Question about an order")]),
                        ("Pedir cita", "Request appointment"), horas, anchor="cita",
                        exito=("Gracias. Te confirmamos la cita por teléfono o WhatsApp en cuanto la tengamos.",
                               "Thank you. We will confirm your appointment by phone or WhatsApp as soon as we have it.")),
        bloque_mapa(("Cómo llegar al taller", "How to get to the studio"), tono="relleno"),
    ])

    return plantilla(KEY, ("Taller de cerámica y artesanía", "Ceramics and craft studio"),
                     ("Taller de cerámica con tienda online, piezas por encargo, proceso en pasos, cursos con fechas de ejemplo, visitas con cita y botón de WhatsApp.",
                      "Ceramics studio with an online shop, made-to-order pieces, the process in steps, courses with example dates, visits by appointment and a WhatsApp button."),
                     "creativos", ("ceramica ceramista alfareria alfarero artesano artesania taller de ceramica torno gres platos tazas vajilla cursos de ceramica tienda online encargos",
                                   "ceramics pottery potter craft handmade studio wheel stoneware mugs plates tableware pottery class online shop commissions"),
                     look, cabecera(nav, visita("primary", ("Pedir cita", "Book a visit")), tienda=True),
                     pie(nav, ("Taller de cerámica hecha a mano. Tienda online, encargos, cursos y visitas con cita.",
                               "Handmade ceramics studio. Online shop, commissions, courses and visits by appointment."),
                         ("Martes a viernes 10:00 a 14:00. Sábado con cita.", "Tuesday to Friday 10 am to 2 pm. Saturday by appointment.")),
                     [inicio, tienda, taller, cont], tienda=True)
