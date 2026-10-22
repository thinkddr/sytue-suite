# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
from lib import *
from looks import LOOKS

KEY = "tienda-ropa"


def build():
    nueva(KEY)
    look = LOOKS["tinta"]
    nav = [("inicio", ("Inicio", "Home")), ("tienda", ("Tienda", "Shop")),
           ("envios", ("Envíos, cambios y tallas", "Shipping, returns and sizes")),
           ("contacto", ("Contacto", "Contact"))]
    horas = [(("Lunes a sábado", "Monday to Saturday"),
              ("10:00 a 14:00 y 17:00 a 20:30", "10:00 am to 2:00 pm and 5:00 pm to 8:30 pm")),
             (("Domingo", "Sunday"), ("Cerrado", "Closed"))]
    cta_cab = marco([llamar(t=("Llamar", "Call")), whatsapp("ghost", ("WhatsApp", "WhatsApp"))],
                    dir="row", gap=8, align="center")
    a_tienda = a_pagina("tienda", "catalogo")

    inicio = pagina("inicio", ("", ""), ("Tienda de ropa", "Clothing shop"),
                    ("Tienda de ropa y complementos con tienda online: catálogo con carrito, envíos, recogida en tienda y WhatsApp.",
                     "Clothing and accessories shop with an online store: catalogue with cart, shipping, in-store pickup and WhatsApp."), [
        hero(("Ropa y complementos de barrio, ahora también online", "Neighbourhood clothes and accessories, now online too"),
             ("Mira lo que tenemos en la tienda, pídelo desde casa y recógelo o te lo enviamos. Si dudas con la talla, escríbenos.",
              "Browse what we have in the shop, order from home and pick it up or have it sent. If you are unsure about the size, message us."),
             [boton(("Ver la tienda", "Browse the shop"), a_tienda), llamar("secondary"), whatsapp("ghost")],
             eyebrow=("Tienda de ropa", "Clothing shop"), variante="centro"),
        bloque_productos(("Novedades", "New in"),
                         ("Lo último que ha llegado a la tienda. En la página Tienda está todo el catálogo.",
                          "The latest arrivals. The full catalogue is on the Shop page."),
                         limite=4, cols=4, anchor="novedades",
                         boton_=boton(("Ver todo el catálogo", "See the whole catalogue"), a_tienda, "secondary")),
        bloque_valores(("Comprar sin sustos", "Shop with peace of mind"), [
            (("14 días para cambiar", "14 days to exchange"),
             ("Si no te queda bien o no es lo que esperabas, la cambiamos o te devolvemos el dinero.",
              "If it does not fit or is not what you expected, we exchange it or refund you.")),
            (("Recogida en tienda", "Pick up in store"),
             ("Pide online, pásate a probártelo y llévatelo. Sin gastos de envío.",
              "Order online, come and try it on, and take it home. No shipping costs.")),
            (("Te ayudamos con la talla", "We help with sizing"),
             ("Mándanos tus medidas o una foto por WhatsApp y te decimos qué talla pedir.",
              "Send us your measurements or a photo on WhatsApp and we will tell you which size to order.")),
        ], tono="relleno", estilo="borde", anchor="comprar"),
        bloque_texto(("Pásate por la tienda", "Come by the shop"),
                     [("Probarse la ropa es lo mejor. En la tienda puedes tocar las telas, comparar y llevarte la prenda en el momento.",
                       "Trying clothes on is best. In the shop you can feel the fabrics, compare and take the garment with you."),
                      ("Abrimos de lunes a sábado, por la mañana y por la tarde. Tienes el horario y el mapa en Contacto.",
                       "We are open Monday to Saturday, mornings and afternoons. Hours and map are on the Contact page.")],
                     eyebrow=("Tienda física", "In-store"),
                     imagen_=("photo", ("Escaparate y interior de la tienda de ropa", "Shop window and interior of the clothing shop")),
                     anchor="tienda-fisica"),
        bloque_opiniones(("Lo que dicen nuestros clientes", "What our customers say"),
                         [("la ropa", "the clothes"), ("el trato", "the service"), ("el envío", "the delivery")]),
        bloque_cta(("¿Buscas algo concreto?", "Looking for something specific?"),
                   ("Dinos qué necesitas y te decimos si lo tenemos, en qué talla y de qué color.",
                    "Tell us what you need and we will say whether we have it, in which size and colour."), tono="base"),
    ])

    tienda = pagina("tienda", ("tienda", "shop"), ("Tienda", "Shop"),
                    ("Catálogo de ropa y complementos con carrito. Recogida gratis en tienda o envío a domicilio.",
                     "Catalogue of clothes and accessories with a cart. Free in-store pickup or home delivery."), [
        hero(("Tienda", "Shop"),
             ("Añade al carrito lo que te guste y paga online. Si dudas con la talla, mira la guía o escríbenos.",
              "Add what you like to the cart and pay online. If you are unsure about the size, check the guide or message us."),
             [boton(("Guía de tallas", "Size guide"), a_pagina("envios", "tallas"), "secondary"), whatsapp("ghost")],
             variante="texto"),
        bloque_productos(("Todo el catálogo", "Full catalogue"), None, limite=24, cols=4, anchor="catalogo"),
        bloque_checks(("Antes de pagar", "Before you pay"), [
            ("Cambios y devoluciones en 14 días desde que recibes o recoges el pedido.",
             "Exchanges and returns within 14 days of receiving or collecting your order."),
            ("Recogida en tienda gratis, normalmente en 24 horas.", "Free in-store pickup, usually within 24 hours."),
            ("Envío a domicilio con número de seguimiento.", "Home delivery with a tracking number."),
            ("Si te falla la talla, cambiamos por otra sin coste en tienda.", "If the size is wrong, we swap it for another at no cost in store.")],
            eyebrow=("Compra con calma", "Shop calmly"), tono="relleno", anchor="condiciones"),
        bloque_cta(("¿No ves tu talla o tu color?", "Cannot see your size or colour?"),
                   ("A veces nos queda en la tienda lo que aún no hemos subido. Pregúntanos por WhatsApp.",
                    "Sometimes we still have in the shop what we have not uploaded yet. Ask us on WhatsApp."),
                   [whatsapp("primary"), llamar("ghost")], tono="base"),
    ])

    envios = pagina("envios", ("envios", "shipping"), ("Envíos, cambios y tallas", "Shipping, returns and sizes"),
                    ("Costes y plazos de envío, cómo cambiar o devolver en 14 días y guía de tallas.",
                     "Shipping costs and times, how to exchange or return within 14 days, and a size guide."), [
        hero(("Envíos, cambios y tallas", "Shipping, returns and sizes"),
             ("Lo que necesitas saber antes de comprar: cuánto tarda en llegar, cómo cambiar una prenda y qué talla elegir.",
              "What you need to know before buying: how long delivery takes, how to exchange a garment and which size to pick."),
             [boton(("Ver la guía de tallas", "See the size guide"), a_ancla("tallas"), "secondary"), whatsapp("ghost")],
             variante="texto"),
        bloque_precios(("Envíos y recogida", "Shipping and pickup"), ("Opciones de entrega", "Delivery options"),
                       [("Opción", "Option"), ("Plazo", "Time"), ("Coste", "Cost")],
                       [(("Recogida en tienda", "In-store pickup"), ("En 24 horas", "Within 24 hours"), ("Gratis", "Free")),
                        (("Envío estándar (península)", "Standard shipping (mainland)"), ("2 a 3 días laborables", "2 to 3 working days"), eur(4.9)),
                        (("Envío exprés (península)", "Express shipping (mainland)"), ("24 horas", "24 hours"), eur(7.9)),
                        (("Envío estándar a partir de 60 € de compra", "Standard shipping on orders over €60"),
                         ("2 a 3 días laborables", "2 to 3 working days"), ("Gratis", "Free"))],
                       intro=("Preparamos el pedido en 24 horas laborables y te mandamos el número de seguimiento por correo.",
                              "We prepare your order within 24 working hours and email you the tracking number."),
                       nota=("Precios de ejemplo: cámbialos por los tuyos. Incluyen IVA.", "Example prices: replace them with your own. VAT included."),
                       anchor="envios", tono="base"),
        bloque_pasos(("Cambios y devoluciones", "Exchanges and returns"), [
            (("Avísanos", "Tell us"),
             ("Escríbenos o llámanos dentro de los 14 días desde que recibes o recoges el pedido.",
              "Message or call us within 14 days of receiving or collecting your order.")),
            (("Prepara la prenda", "Get the garment ready"),
             ("Sin usar, con la etiqueta puesta y en su bolsa. La ropa interior y los bañadores, solo con el precinto de higiene.",
              "Unworn, with the tag on and in its bag. Underwear and swimwear only with the hygiene seal intact.")),
            (("Tráela o envíala", "Bring or send it"),
             ("En tienda no cuesta nada. Por correo, el envío de vuelta corre de tu cuenta, salvo que la prenda llegue defectuosa o te hayamos enviado otra.",
              "In store it costs nothing. By post, the return shipping is on you, unless the garment arrives faulty or we sent the wrong one.")),
            (("Cambio o reembolso", "Exchange or refund"),
             ("Te damos otra talla u otra prenda, o te devolvemos el dinero por el mismo medio de pago en un máximo de 14 días.",
              "We give you another size or garment, or refund you by the original payment method within 14 days at most.")),
        ], tono="relleno", anchor="devoluciones"),
        seccion("pricing", [
            titular(("Guía de tallas", "Size guide"),
                    ("Mídete en centímetros, sin apretar, y compara. Si estás entre dos tallas, elige la mayor para prendas holgadas y la menor para ajustadas.",
                     "Measure yourself in centimetres, without pulling tight, and compare. If you are between two sizes, pick the larger for loose garments and the smaller for fitted ones.")),
            tabla(("Medidas del cuerpo por talla, en centímetros", "Body measurements by size, in centimetres"),
                  [("Talla", "Size"), ("Pecho", "Chest"), ("Cintura", "Waist"), ("Cadera", "Hip")],
                  [(("XS", "XS"), ("80 a 84", "80 to 84"), ("62 a 66", "62 to 66"), ("86 a 90", "86 to 90")),
                   (("S", "S"), ("84 a 88", "84 to 88"), ("66 a 70", "66 to 70"), ("90 a 94", "90 to 94")),
                   (("M", "M"), ("88 a 96", "88 to 96"), ("70 a 78", "70 to 78"), ("94 a 102", "94 to 102")),
                   (("L", "L"), ("96 a 104", "96 to 104"), ("78 a 86", "78 to 86"), ("102 a 110", "102 to 110")),
                   (("XL", "XL"), ("104 a 112", "104 to 112"), ("86 a 94", "86 to 94"), ("110 a 118", "110 to 118"))]),
            peq(("Medidas orientativas: cada fabricante corta a su manera. En la ficha de cada prenda te indicamos si viene grande o pequeña.",
                 "Approximate measurements: each maker cuts differently. On each garment's page we say if it runs large or small."),
                color="$muted"),
        ], anchor="tallas", tono="base", gap=28),
        bloque_faq(("Preguntas frecuentes", "Frequently asked questions"), [
            (("¿Puedo cambiar en tienda algo que compré online?", "Can I exchange in store something I bought online?"),
             ("Sí. Trae la prenda con la etiqueta y el justificante del pedido y la cambiamos al momento.",
              "Yes. Bring the garment with its tag and the order confirmation and we swap it on the spot.")),
            (("¿Cómo puedo pagar?", "How can I pay?"),
             ("Online, con tarjeta. Si recoges en tienda, también puedes pagar al recoger.",
              "Online, by card. If you pick up in store, you can also pay on collection.")),
            (("¿Qué pasa si la prenda llega con un defecto?", "What if the garment arrives with a fault?"),
             ("Escríbenos con una foto. La cambiamos o te devolvemos el dinero, y el envío de vuelta lo pagamos nosotros.",
              "Message us with a photo. We exchange it or refund you, and we pay the return shipping.")),
            (("¿Enviáis fuera de la península?", "Do you ship outside the mainland?"),
             ("Consúltanos antes de comprar: el plazo y el coste cambian según el destino.",
              "Ask us before buying: the time and cost depend on the destination.")),
        ], tono="relleno"),
    ])

    cont = pagina("contacto", ("contacto", "contact"), ("Contacto", "Contact"),
                  ("Horario de la tienda, cómo llegar, teléfono, WhatsApp y formulario para consultas sobre pedidos y tallas.",
                   "Shop hours, how to get here, phone, WhatsApp and a form for questions about orders and sizes."), [
        hero(("Contacto", "Contact"),
             ("Llámanos, escríbenos por WhatsApp o pasa por la tienda. Para dudas de talla, una foto o tus medidas nos ayudan mucho.",
              "Call us, message us on WhatsApp or come by the shop. For size questions, a photo or your measurements help a lot."),
             [llamar(), whatsapp()], variante="texto"),
        bloque_contacto(("Estamos en la tienda", "We are in the shop"),
                        ("Si ya has hecho un pedido, dinos el número que te llegó por correo.",
                         "If you have already placed an order, tell us the number that came by email."),
                        campos_consulta(servicios=[("Un pedido", "An order"), ("Talla o medidas", "Size or measurements"),
                                                   ("Cambio o devolución", "Exchange or return"), ("Otra cosa", "Something else")]),
                        ("Enviar consulta", "Send enquiry"), horas),
        bloque_mapa(("Cómo llegar a la tienda", "How to get to the shop"), tono="relleno"),
    ])

    return plantilla(KEY, ("Tienda de ropa", "Clothing shop"),
                     ("Tienda de ropa con catálogo y carrito, envíos y recogida, guía de tallas, cambios en 14 días y botones de llamar y WhatsApp.",
                      "Clothing shop with a catalogue and cart, shipping and pickup, size guide, 14-day exchanges and call and WhatsApp buttons."),
                     "comercio", ("tienda ropa tienda de ropa boutique moda complementos tienda online ropa de barrio comercio tallas",
                                  "clothing shop clothes store boutique fashion accessories online shop local shop sizes"),
                     look, cabecera(nav, cta_cab, tienda=True),
                     pie(nav, ("Tienda de ropa y complementos, en el barrio y online.",
                               "Clothing and accessories shop, in the neighbourhood and online."),
                         ("Lunes a sábado 10:00 a 14:00 y 17:00 a 20:30.", "Monday to Saturday 10 am to 2 pm and 5 pm to 8:30 pm.")),
                     [inicio, tienda, envios, cont], tienda=True)
