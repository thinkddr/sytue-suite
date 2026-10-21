# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
from lib import *

KEY = "producto"

# Arena clara y verde bosque apagado. Sin `tono="oscuro"`: primary/inverse no llega a 3.
ARENA_BOSQUE = dict(
    colores=dict(background="#f8f3ea", surface="#ede5d6", text="#221f1a", muted="#5b5448", primary="#2f4d3c",
                 on_primary="#ffffff", border="#dcd1bd", inverse="#1f2e26", on_inverse="#f3efe6"),
    fuentes=("lora", "inter"), peso=600, h1=(50, 40, 32), radio=(6, 12, 18), ancho=1100)

NOTA_DATOS = ("Datos de ejemplo: cámbialos por los de tu producto.", "Example data: replace it with your product's.")


def build():
    nueva(KEY)
    look = ARENA_BOSQUE
    nav = [("inicio", ("Producto", "Product")), ("detalles", ("Detalles y preguntas", "Details and questions")),
           ("envios", ("Envío y devoluciones", "Shipping and returns")), ("contacto", ("Contacto", "Contact"))]
    horas = [(("Atención al cliente, lunes a viernes", "Customer service, Monday to Friday"),
              ("9:00 a 14:00 y 16:00 a 19:00", "9:00 am to 2:00 pm and 4:00 pm to 7:00 pm")),
             (("Sábados, domingos y festivos", "Saturdays, Sundays and bank holidays"), ("Cerrado", "Closed"))]
    comprar = lambda: boton(("Comprar ahora", "Buy now"), a_pagina("inicio", "comprar"))
    cta_cab = marco([comprar(), llamar("ghost", ("Llamar", "Call"))], dir="row", gap=8, align="center")

    inicio = pagina("inicio", ("", ""), ("Producto", "Product"),
                    ("Tienda de un solo producto: compra directa con carrito, qué incluye, envío en 24 a 48 horas y 14 días para devolverlo.",
                     "Single-product shop: buy directly with a cart, what is included, delivery in 24 to 48 hours and 14 days to return it."), [
        hero(("Nombre del producto, sin complicaciones", "Product name, no fuss"),
             ("Un solo producto y una compra clara: sin letra pequeña, con envío en 24 a 48 horas laborables y 14 días para devolverlo si no te convence.",
              "One product and a clear purchase: no small print, delivery in 24 to 48 working hours and 14 days to return it if it does not suit you."),
             [comprar(), boton(("Ver especificaciones", "See specifications"), a_pagina("detalles", "especificaciones"), "secondary")],
             eyebrow=("Tienda de un solo producto", "Single-product shop"),
             imagen_=("product", ("El producto sobre un fondo neutro, visto de frente", "The product on a neutral background, seen from the front"))),
        bloque_productos(("Elígelo y añádelo al carrito", "Pick it and add it to the cart"),
                         ("Elige el color o el tamaño, añádelo al carrito y paga con tarjeta al final.",
                          "Choose the colour or size, add it to the cart and pay by card at the end."),
                         limite=3, cols=3, anchor="comprar", tono="relleno"),
        bloque_valores(("Por qué comprarlo", "Why buy it"), [
            (("Qué problema resuelve", "What problem it solves"),
             ("Una frase con lo que consigue quien lo usa. Por ejemplo: «Cabe en el bolsillo y carga el móvil en dos horas». Texto de ejemplo: cámbialo por el tuyo.",
              "One sentence about what the user gets. For example: “Fits in a pocket and charges a phone in two hours.” Example text: replace it with yours.")),
            (("Cómo se usa", "How it is used"),
             ("Tres pasos como máximo, con palabras de andar por casa. Texto de ejemplo: cámbialo por el tuyo.",
              "Three steps at most, in plain words. Example text: replace it with yours.")),
            (("Llega rápido", "It arrives fast"),
             ("Sale en 24 horas laborables y te mandamos el número de seguimiento por correo.",
              "It ships within 24 working hours and we email you the tracking number.")),
            (("Si no te convence, lo devuelves", "If it does not suit you, send it back"),
             ("Tienes 14 días desde que lo recibes y te devolvemos el dinero.",
              "You have 14 days from receiving it and we refund your money.")),
        ], cols=4, tono="base", estilo="borde", anchor="beneficios"),
        bloque_texto(("Qué es y para quién es", "What it is and who it is for"),
                     [("Descripción de ejemplo: cuenta en tres o cuatro líneas qué es el producto, cómo está hecho y a quién le va a servir. Cámbiala por la tuya.",
                       "Example description: say in three or four lines what the product is, how it is made and who it will be useful for. Replace it with yours."),
                      ("Habla de lo que ve y toca quien lo compra: material, tamaño, peso, cómo se siente al usarlo (texto de ejemplo).",
                       "Talk about what the buyer sees and feels: material, size, weight, how it feels to use (example text).")],
                     eyebrow=("Descripción", "Description"), tono="relleno", invertir=True,
                     imagen_=("product", ("El producto en uso en una casa", "The product being used at home")),
                     anchor="descripcion"),
        bloque_checks(("Qué incluye tu pedido", "What your order includes"), [
            ("Nombre del producto, nuevo y en su caja.", "Product name, new and in its box."),
            ("Instrucciones de uso en español.", "Instructions for use in Spanish."),
            ("Factura con el IVA desglosado.", "Invoice with VAT itemised."),
            ("Garantía legal de producto nuevo.", "Statutory guarantee for new goods."),
            ("Accesorios o recambios de ejemplo: lista aquí lo que incluya el tuyo.", "Example accessories or spares: list here whatever yours includes.")],
            cols=1, eyebrow=("En la caja", "In the box"), anchor="incluye"),
        bloque_opiniones(("Lo que dicen quienes ya lo tienen", "What people who already have it say"),
                         [("el producto", "the product"), ("el envío", "the delivery"), ("la atención", "the support")]),
        bloque_cta(("¿Lo quieres en casa esta semana?", "Want it at home this week?"),
                   ("Añádelo al carrito y lo preparamos en 24 horas laborables.",
                    "Add it to the cart and we prepare it within 24 working hours."),
                   [comprar(), whatsapp("ghost")], tono="relleno"),
    ])

    det = pagina("detalles", ("detalles", "details"), ("Detalles y preguntas", "Details and questions"),
                 ("Especificaciones técnicas de ejemplo en tabla, cuidado y mantenimiento, y preguntas frecuentes antes de comprar.",
                  "Example technical specifications in a table, care and maintenance, and frequently asked questions before buying."), [
        hero(("Detalles y preguntas", "Details and questions"),
             ("Todo lo que conviene saber antes de comprar: medidas, materiales, cómo se cuida y qué pasa si algo falla.",
              "Everything worth knowing before you buy: size, materials, how to look after it and what happens if something goes wrong."),
             [comprar(), whatsapp("secondary")], variante="texto"),
        bloque_precios(("Especificaciones", "Specifications"), ("Especificaciones del producto", "Product specifications"),
                       [("Característica", "Feature"), ("Valor", "Value")],
                       [(("Dimensiones", "Dimensions"), ("00 × 00 × 00 cm", "00 × 00 × 00 cm")),
                        (("Peso", "Weight"), ("000 g", "000 g")),
                        (("Materiales", "Materials"), ("Material principal y acabado", "Main material and finish")),
                        (("Colores o modelos", "Colours or models"), ("Los que haya en la tienda", "Those available in the shop")),
                        (("Contenido de la caja", "Box contents"), ("El producto y sus instrucciones", "The product and its instructions")),
                        (("Cuidado y mantenimiento", "Care and maintenance"), ("Cómo limpiarlo y cada cuánto", "How to clean it and how often")),
                        (("Garantía", "Guarantee"), ("La legal de producto nuevo", "The statutory guarantee for new goods"))],
                       nota=NOTA_DATOS, boton_=comprar(), anchor="especificaciones", tono="base"),
        bloque_faq(("Preguntas antes de comprar", "Questions before buying"), [
            (("¿Cuánto tarda en llegar?", "How long does it take to arrive?"),
             ("Sale en 24 horas laborables y llega en 2 a 4 días laborables en la península. Los plazos exactos están en Envío y devoluciones.",
              "It ships within 24 working hours and arrives in 2 to 4 working days on the mainland. Exact times are on the Shipping and returns page.")),
            (("¿Puedo devolverlo si no me gusta?", "Can I return it if I do not like it?"),
             ("Sí. Tienes 14 días naturales desde que lo recibes. Escríbenos y te decimos cómo enviarlo.",
              "Yes. You have 14 calendar days from receiving it. Message us and we tell you how to send it.")),
            (("¿Qué hago si llega con un defecto?", "What if it arrives with a fault?"),
             ("Escríbenos con una foto. Lo cambiamos o te devolvemos el dinero, y el envío de vuelta lo pagamos nosotros.",
              "Message us with a photo. We replace it or refund you, and we pay the return shipping.")),
            (("¿Puedo pedir factura?", "Can I ask for an invoice?"),
             ("Sí, con los datos fiscales que nos indiques al hacer el pedido o por escrito después.",
              "Yes, with the tax details you give us when ordering or in writing afterwards.")),
            (("¿Cómo puedo pagar?", "How can I pay?"),
             ("Con tarjeta de crédito o débito. El pago lo gestiona la pasarela de pago: nosotros no vemos los datos de tu tarjeta.",
              "By credit or debit card. The payment provider handles the payment: we never see your card details.")),
        ], tono="relleno"),
    ])

    env = pagina("envios", ("envios", "shipping"), ("Envío y devoluciones", "Shipping and returns"),
                 ("Plazos y costes de envío con tabla de ejemplo y cómo devolver el producto en 14 días.",
                  "Example shipping times and costs table and how to return the product within 14 days."), [
        hero(("Envío y devoluciones", "Shipping and returns"),
             ("Cuánto tarda en llegar, cuánto cuesta y cómo devolverlo si no es lo que esperabas.",
              "How long it takes to arrive, what it costs and how to send it back if it is not what you expected."),
             [comprar()], variante="texto"),
        bloque_precios(("Envío", "Shipping"), ("Opciones y costes de envío", "Shipping options and costs"),
                       [("Destino y servicio", "Destination and service"), ("Plazo", "Time"), ("Coste", "Cost")],
                       [(("Península, envío estándar", "Mainland, standard shipping"), ("2 a 4 días laborables", "2 to 4 working days"), eur(4.9)),
                        (("Península, envío exprés", "Mainland, express shipping"), ("24 a 48 horas", "24 to 48 hours"), eur(7.9)),
                        (("Baleares y Canarias", "Balearic and Canary Islands"), ("5 a 10 días laborables", "5 to 10 working days"), eur(9.9))],
                       intro=("Preparamos tu pedido en 24 horas laborables y te enviamos el número de seguimiento por correo.",
                              "We prepare your order within 24 working hours and email you the tracking number."),
                       nota=("Precios de ejemplo: cámbialos por los tuyos. Incluyen IVA.", "Example prices: replace them with your own. VAT included."),
                       anchor="costes", tono="base"),
        bloque_pasos(("Devolver el producto", "Returning the product"), [
            (("Avísanos", "Tell us"), ("Escríbenos dentro de los 14 días naturales desde que lo recibes.", "Message us within 14 calendar days of receiving it.")),
            (("Prepáralo", "Pack it"), ("En su caja, sin usar y con el número de pedido a la vista.", "In its box, unused and with the order number visible.")),
            (("Envíalo", "Send it"), ("El transporte de vuelta lo pagas tú, salvo que sea un defecto nuestro.", "You pay the return shipping, unless the fault is ours.")),
            (("Recibes el reembolso", "You get your refund"), ("En un máximo de 14 días desde que lo recibimos y lo revisamos, por el mismo medio de pago.", "Within 14 days at most of us receiving and checking it, by the same payment method.")),
        ], tono="relleno", anchor="devoluciones"),
    ])

    cont = pagina("contacto", ("contacto", "contact"), ("Contacto", "Contact"),
                  ("Atención al cliente por teléfono, WhatsApp y formulario antes y después de comprar.",
                   "Customer service by phone, WhatsApp and form, before and after buying."), [
        hero(("Atención al cliente", "Customer service"),
             ("¿Una duda antes de comprar o un problema con tu pedido? Escríbenos y te respondemos nosotros.",
              "A question before buying or a problem with your order? Write to us and we will reply ourselves."),
             [llamar(), whatsapp()], variante="texto"),
        bloque_contacto(("Habla con nosotros", "Talk to us"),
                        ("Respondemos en horario de atención, normalmente el mismo día laborable.",
                         "We reply during customer service hours, usually the same working day."),
                        campos_consulta(servicios=[("Antes de comprar", "Before buying"), ("Mi pedido", "My order"),
                                                   ("Una devolución", "A return"), ("Otra cosa", "Something else")]),
                        ("Enviar consulta", "Send enquiry"), horas,
                        direccion=("Dirección para devoluciones: Calle Mayor, 1, 00000 Tu ciudad",
                                   "Returns address: Calle Mayor, 1, 00000 Your city"),
                        exito=("Gracias. Hemos recibido tu consulta y te respondemos en horario de atención.",
                               "Thank you. We have received your enquiry and will reply during customer service hours.")),
    ])

    return plantilla(KEY, ("Tienda de un solo producto", "Single-product shop"),
                     ("Tienda de un producto con carrito: beneficios, descripción, qué incluye, tabla de especificaciones, preguntas frecuentes, envío y devoluciones, y atención al cliente.",
                      "Single-product shop with a cart: benefits, description, what is included, a specifications table, FAQs, shipping and returns, and customer service."),
                     "comercio", ("producto un solo producto tienda de un producto landing de producto venta directa marca propia tienda online",
                                  "single product one product shop product page landing page direct sales own brand online store"),
                     look, cabecera(nav, cta_cab, tienda=True),
                     pie(nav, ("Nombre del producto: compra directa, envío con seguimiento y 14 días para devolver.",
                               "Product name: direct purchase, tracked shipping and 14 days to return."),
                         ("Atención al cliente de lunes a viernes, 9:00 a 14:00 y 16:00 a 19:00.",
                          "Customer service Monday to Friday, 9 am to 2 pm and 4 pm to 7 pm.")),
                     [inicio, det, env, cont], tienda=True,
                     exento={"mapa": "Venta online de un solo producto: no hay local abierto al público al que llegar; la dirección que se muestra es la de devoluciones."})
