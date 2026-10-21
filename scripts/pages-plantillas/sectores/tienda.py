# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
from lib import *

KEY = "tienda"

# Blanco roto y azul de tienda. Sin `tono="oscuro"`: primary/inverse no llega a 3.
LINO = dict(
    colores=dict(background="#fafaf7", surface="#eeeee8", text="#17191c", muted="#51565d", primary="#0b5a8f",
                 on_primary="#ffffff", border="#dcdcd3", inverse="#12202c", on_inverse="#f0f4f7"),
    fuentes=("figtree", "inter"), peso=700, radio=(6, 10, 16), ancho=1160)

NOTA_PRECIOS = ("Precios de ejemplo: cámbialos por los tuyos. Incluyen IVA.",
                "Example prices: replace them with your own. VAT included.")


def build():
    nueva(KEY)
    look = LINO
    nav = [("inicio", ("Inicio", "Home")), ("tienda", ("Tienda", "Shop")),
           ("envios", ("Envíos y devoluciones", "Shipping and returns")), ("contacto", ("Contacto", "Contact"))]
    horas = [(("Atención al cliente, lunes a viernes", "Customer service, Monday to Friday"),
              ("9:00 a 14:00 y 16:00 a 19:00", "9:00 am to 2:00 pm and 4:00 pm to 7:00 pm")),
             (("Sábados y domingos", "Saturdays and Sundays"), ("Cerrado", "Closed"))]
    cta_cab = marco([llamar(t=("Llamar", "Call")), whatsapp("ghost", ("WhatsApp", "WhatsApp"))], dir="row", gap=8, align="center")
    a_tienda = a_pagina("tienda", "catalogo")

    inicio = pagina("inicio", ("", ""), ("Tienda online", "Online shop"),
                    ("Tienda online con catálogo y carrito, envíos con seguimiento, devoluciones en 14 días y atención por teléfono o WhatsApp.",
                     "Online shop with a catalogue and cart, tracked shipping, returns within 14 days and support by phone or WhatsApp."), [
        hero(("Compra online, recibe en casa", "Shop online, get it at home"),
             ("Elige, añade al carrito y paga con tarjeta. Preparamos tu pedido en 24 horas laborables y te mandamos el seguimiento por correo.",
              "Choose, add to the cart and pay by card. We prepare your order within 24 working hours and email you the tracking."),
             [boton(("Ver la tienda", "Browse the shop"), a_tienda), whatsapp("secondary")],
             eyebrow=("Tienda online", "Online shop"),
             imagen_=("product", ("Varios productos de la tienda sobre una mesa", "Several shop products laid out on a table"))),
        bloque_productos(("Novedades", "New in"),
                         ("Lo último que hemos añadido. En la página Tienda está todo el catálogo.",
                          "The latest additions. The full catalogue is on the Shop page."),
                         limite=4, cols=4, anchor="novedades",
                         boton_=boton(("Ver todo el catálogo", "See the whole catalogue"), a_tienda, "secondary")),
        bloque_pasos(("Comprar es sencillo", "Buying is easy"), [
            (("Elige", "Choose"), ("Mira las fotos y la descripción de cada producto.", "Look at each product's photos and description.")),
            (("Añade al carrito", "Add to the cart"), ("Puedes seguir mirando y revisar el carrito antes de pagar.", "You can keep browsing and review the cart before paying.")),
            (("Paga", "Pay"), ("Con tarjeta, desde el móvil o el ordenador.", "By card, from your phone or computer.")),
            (("Recíbelo", "Receive it"), ("Te avisamos cuando sale el pedido y te damos el número de seguimiento.", "We tell you when the order ships and give you the tracking number.")),
        ], tono="relleno", anchor="como-comprar"),
        bloque_valores(("Compra con garantías", "Shop with confidence"), [
            (("14 días para devolver", "14 days to return"),
             ("Si no es lo que esperabas, lo devuelves y te reembolsamos. Todo el proceso está en Envíos y devoluciones.",
              "If it is not what you expected, send it back and we refund you. The whole process is on the Shipping and returns page.")),
            (("Pago seguro", "Secure payment"),
             ("El pago lo gestiona la pasarela de pago: nosotros no vemos ni guardamos los datos de tu tarjeta.",
              "The payment provider handles the payment: we never see or store your card details.")),
            (("Atención por una persona", "Support from a person"),
             ("Escríbenos o llámanos si tienes una duda antes o después de comprar.",
              "Message or call us if you have a question before or after buying.")),
        ], tono="base", anchor="garantias"),
        bloque_opiniones(("Lo que dicen nuestros clientes", "What our customers say"),
                         [("el producto", "the product"), ("el envío", "the delivery"), ("la atención", "the support")]),
        bloque_cta(("¿No encuentras lo que buscas?", "Cannot find what you are looking for?"),
                   ("Dinos qué necesitas y te decimos si lo tenemos o podemos conseguirlo.",
                    "Tell us what you need and we will say whether we have it or can get it."),
                   [whatsapp("primary"), llamar("ghost")], tono="base"),
    ])

    tienda = pagina("tienda", ("tienda", "shop"), ("Tienda", "Shop"),
                    ("Catálogo completo con carrito. Envío con seguimiento y devolución en 14 días.",
                     "Full catalogue with a cart. Tracked shipping and returns within 14 days."), [
        hero(("Tienda", "Shop"),
             ("Todo el catálogo en un solo sitio. Añade al carrito lo que quieras y paga al final, con tarjeta.",
              "The whole catalogue in one place. Add what you like to the cart and pay at the end, by card."),
             [boton(("Envíos y devoluciones", "Shipping and returns"), a_pagina("envios", "envios"), "secondary"), whatsapp("ghost")],
             variante="texto"),
        bloque_productos(("Todo el catálogo", "Full catalogue"), None, limite=24, cols=4, anchor="catalogo"),
        bloque_checks(("Antes de pagar", "Before you pay"), [
            ("Los precios incluyen IVA. Los gastos de envío se ven antes de confirmar el pedido.", "Prices include VAT. Shipping costs are shown before you confirm the order."),
            ("Preparamos el pedido en 24 horas laborables.", "We prepare your order within 24 working hours."),
            ("Recibes un correo con la confirmación y el número de seguimiento.", "You receive an email with the confirmation and the tracking number."),
            ("Tienes 14 días para devolver lo que no te convenza.", "You have 14 days to return anything that does not suit you.")],
            eyebrow=("Compra con calma", "Shop calmly"), tono="relleno", anchor="condiciones"),
        bloque_cta(("¿Dudas con un producto?", "Unsure about a product?"),
                   ("Escríbenos por WhatsApp con el nombre del producto y te respondemos en horario de atención.",
                    "Message us on WhatsApp with the product name and we reply during customer service hours."),
                   [whatsapp("primary"), llamar("ghost")], tono="base"),
    ])

    envios = pagina("envios", ("envios", "shipping"), ("Envíos y devoluciones", "Shipping and returns"),
                    ("Costes y plazos de envío con tabla de ejemplo, devoluciones en 14 días, métodos de pago y preguntas frecuentes.",
                     "Example shipping costs and times, returns within 14 days, payment methods and frequently asked questions."), [
        hero(("Envíos y devoluciones", "Shipping and returns"),
             ("Cuánto tarda y cuánto cuesta el envío, cómo devolver un pedido y cómo puedes pagar.",
              "How long shipping takes and what it costs, how to return an order and how you can pay."),
             [boton(("Ver cómo devolver", "See how to return"), a_ancla("devoluciones"), "secondary"), whatsapp("ghost")],
             variante="texto"),
        bloque_precios(("Envíos", "Shipping"), ("Opciones y costes de envío", "Shipping options and costs"),
                       [("Destino y servicio", "Destination and service"), ("Plazo", "Time"), ("Coste", "Cost")],
                       [(("Península, envío estándar", "Mainland, standard shipping"), ("2 a 4 días laborables", "2 to 4 working days"), eur(4.9)),
                        (("Península, envío exprés", "Mainland, express shipping"), ("24 a 48 horas", "24 to 48 hours"), eur(7.9)),
                        (("Península, pedidos de más de 50 €", "Mainland, orders over €50"), ("2 a 4 días laborables", "2 to 4 working days"), ("Gratis", "Free")),
                        (("Baleares y Canarias", "Balearic and Canary Islands"), ("5 a 10 días laborables", "5 to 10 working days"), eur(9.9))],
                       intro=("Preparamos el pedido en 24 horas laborables y te mandamos el número de seguimiento por correo.",
                              "We prepare your order within 24 working hours and email you the tracking number."),
                       nota=NOTA_PRECIOS, anchor="envios", tono="base"),
        bloque_pasos(("Cómo devolver un pedido", "How to return an order"), [
            (("Avísanos", "Tell us"),
             ("Escríbenos o llámanos dentro de los 14 días naturales desde que recibes el pedido.",
              "Message or call us within 14 calendar days of receiving your order.")),
            (("Prepara el paquete", "Pack it up"),
             ("El producto, sin usar y con su embalaje, junto con el número de pedido.",
              "The product, unused and in its packaging, together with your order number.")),
            (("Envíalo", "Send it back"),
             ("El transporte de vuelta lo pagas tú, salvo que llegue defectuoso o te hayamos enviado otro producto.",
              "You pay for the return shipping, unless it arrives faulty or we sent you the wrong product.")),
            (("Te reembolsamos", "We refund you"),
             ("Cuando recibimos y revisamos el paquete, te devolvemos el dinero por el mismo medio de pago, en un máximo de 14 días.",
              "Once we receive and check the parcel, we refund you by the original payment method within 14 days at most.")),
        ], tono="relleno", anchor="devoluciones"),
        bloque_texto(("Métodos de pago", "Payment methods"),
                     [("Aceptamos tarjeta de crédito o débito y otros métodos de pago seguros. Texto de ejemplo: cambia esta lista por los métodos que tengas activados.",
                       "We accept credit or debit card and other secure payment methods. Example text: replace this list with the methods you have switched on."),
                      ("El pago lo gestiona la pasarela de pago, y nosotros no vemos ni guardamos los datos de tu tarjeta.",
                       "The payment provider handles the payment, and we never see or store your card details.")],
                     eyebrow=("Pagos", "Payments"), anchor="pagos"),
        bloque_faq(("Preguntas frecuentes", "Frequently asked questions"), [
            (("¿Cuándo sale mi pedido?", "When does my order ship?"),
             ("En 24 horas laborables desde que recibimos el pago. Te avisamos por correo con el seguimiento.",
              "Within 24 working hours of receiving payment. We email you the tracking.")),
            (("¿Puedo cambiar la dirección de entrega?", "Can I change the delivery address?"),
             ("Sí, si el pedido aún no ha salido. Escríbenos cuanto antes con el número de pedido.",
              "Yes, if the order has not shipped yet. Message us as soon as possible with your order number.")),
            (("¿Qué hago si el producto llega roto o defectuoso?", "What if the product arrives broken or faulty?"),
             ("Escríbenos con una foto. Lo cambiamos o te devolvemos el dinero, y el envío de vuelta lo pagamos nosotros.",
              "Message us with a photo. We replace it or refund you, and we pay the return shipping.")),
            (("¿Puedo pedir factura?", "Can I ask for an invoice?"),
             ("Sí. Escríbenos con el número de pedido y los datos fiscales de tu empresa o los tuyos.",
              "Yes. Message us with your order number and your company's or your own tax details.")),
        ], tono="relleno"),
    ])

    cont = pagina("contacto", ("contacto", "contact"), ("Contacto", "Contact"),
                  ("Atención al cliente por teléfono, WhatsApp y formulario: pedidos, envíos, devoluciones y dudas de producto.",
                   "Customer service by phone, WhatsApp and form: orders, shipping, returns and product questions."), [
        hero(("Atención al cliente", "Customer service"),
             ("Si tienes un pedido en camino, dinos el número que te llegó por correo y lo miramos al momento.",
              "If you have an order on its way, give us the number from your email and we will check it right away."),
             [llamar(), whatsapp()], variante="texto"),
        bloque_contacto(("Escríbenos", "Write to us"),
                        ("Respondemos en horario de atención, normalmente en el mismo día laborable.",
                         "We reply during customer service hours, usually the same working day."),
                        campos_consulta(servicios=[("Un pedido", "An order"), ("Un envío", "A delivery"),
                                                   ("Una devolución", "A return"), ("Un producto", "A product"),
                                                   ("Otra cosa", "Something else")],
                                        extra=[campo("pedido", "text", ("Número de pedido (si lo tienes)", "Order number (if you have one)"), False, maxlen=40)]),
                        ("Enviar consulta", "Send enquiry"), horas,
                        direccion=("Dirección para devoluciones: Calle Mayor, 1, 00000 Tu ciudad",
                                   "Returns address: Calle Mayor, 1, 00000 Your city"),
                        notas=("Antes de enviar una devolución, escríbenos: así te damos el número de devolución.",
                               "Before sending a return, message us: that way we give you a return number."),
                        exito=("Gracias. Hemos recibido tu consulta y te respondemos en horario de atención.",
                               "Thank you. We have received your enquiry and will reply during customer service hours.")),
    ])

    return plantilla(KEY, ("Tienda online", "Online shop"),
                     ("Tienda online genérica con carrito: novedades en portada, catálogo, envíos con tabla de costes, devoluciones en 14 días, métodos de pago y atención al cliente.",
                      "General online shop with a cart: new arrivals on the home page, catalogue, shipping cost table, 14-day returns, payment methods and customer service."),
                     "comercio", ("tienda online ecommerce comercio electronico vender por internet catalogo carrito tienda web pedidos",
                                  "online shop e-commerce web shop sell online catalogue cart orders"),
                     look, cabecera(nav, cta_cab, tienda=True),
                     pie(nav, ("Tienda online con envío con seguimiento y devolución en 14 días.",
                               "Online shop with tracked shipping and 14-day returns."),
                         ("Atención al cliente de lunes a viernes, 9:00 a 14:00 y 16:00 a 19:00.",
                          "Customer service Monday to Friday, 9 am to 2 pm and 4 pm to 7 pm.")),
                     [inicio, tienda, envios, cont], tienda=True,
                     exento={"mapa": "Tienda solo online: no hay local abierto al público al que llegar; la dirección que se muestra es la de devoluciones."})
