# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
from lib import *

KEY = "perfumeria-cosmetica"

# Paleta propia: lila muy claro y palo de rosa oscuro (madera de perfumería). No lleva fondo oscuro.
PALO_ROSA = dict(
    colores=dict(background="#f7edfa", surface="#eedcf3", text="#2b1830", muted="#6a4f6e", primary="#8a402e",
                 on_primary="#ffffff", border="#e0cae8", inverse="#2d1a24", on_inverse="#fbeef6"),
    fuentes=("playfair-display", "manrope"), peso=600, radio=(2, 6, 10), ancho=1120)

NOTA_FORMATOS = ("Capacidades habituales: cada producto indica la suya en su ficha.",
                 "Usual capacities: each product states its own on its page.")


def build():
    nueva(KEY)
    malos = comprobar_paleta("palo-rosa", PALO_ROSA["colores"])
    assert not malos, malos
    look = PALO_ROSA
    nav = [("inicio", ("Inicio", "Home")), ("tienda", ("Tienda y formatos", "Shop and sizes")),
           ("asesoramiento", ("Asesoramiento y regalos", "Advice and gifts")), ("contacto", ("Contacto", "Contact"))]
    horas = [(("Lunes a viernes", "Monday to Friday"),
              ("10:00 a 14:00 y 17:00 a 20:30", "10:00 am to 2:00 pm and 5:00 pm to 8:30 pm")),
             (("Sábado", "Saturday"), ("10:00 a 14:00 y 17:00 a 20:30", "10:00 am to 2:00 pm and 5:00 pm to 8:30 pm")),
             (("Domingo", "Sunday"), ("Cerrado", "Closed"))]
    cta_cab = marco([llamar(t=("Llamar", "Call")), whatsapp("ghost", ("WhatsApp", "WhatsApp"))],
                    dir="row", gap=8, wrap=True)
    a_catalogo = a_pagina("tienda", "catalogo")

    def tipo(h, t, puntos):
        return marco([tarjeta([h3(h), p(t, color="$muted"),
                               marco([p(x) for x in puntos], gap=6, tag="div")],
                              estilo="borde", tag="div", enlace=a_catalogo, gap=14, pad=28)], tag="li", gap=0)

    inicio = pagina("inicio", ("", ""), ("Perfumería y cosmética", "Perfumery and cosmetics"),
                    ("Perfumería y cosmética con tienda online: fragancias, maquillaje y cuidado de la piel, asesoramiento en tienda, muestras, regalos y botones de llamar y WhatsApp.",
                     "Perfumery and cosmetics with an online store: fragrances, make-up and skincare, in-shop advice, samples, gifts and call and WhatsApp buttons."), [
        hero(("Huele, prueba y elige con calma", "Smell, try and choose at your own pace"),
             ("Fragancias, maquillaje y cuidado de la piel en la tienda y en la web. Si no sabes qué elegir, te aconsejamos sin prisa y te dejamos probar.",
              "Fragrances, make-up and skincare in the shop and online. If you do not know what to choose, we advise you without rushing and let you try."),
             [boton(("Ver la tienda", "Browse the shop"), a_catalogo), llamar("secondary"), whatsapp("ghost")],
             eyebrow=("Perfumería y cosmética", "Perfumery and cosmetics"), tono="relleno",
             imagen_=("product", ("Frascos de perfume sobre una bandeja de madera", "Perfume bottles on a wooden tray"))),
        seccion("features", [
            titular(("Elige por tipo", "Choose by type"),
                    ("Tres secciones en la tienda y en la web. Entra en la que busques.",
                     "Three sections in the shop and online. Go into the one you are after."),
                    eyebrow=("Catálogo", "Catalogue")),
            rejilla([tipo(("Fragancias", "Fragrances"),
                          ("Para llevar a diario o para una ocasión.", "To wear every day or for an occasion."),
                          [("Colonias y aguas frescas", "Colognes and fresh waters"), ("Perfumes de día y de noche", "Day and evening perfumes"),
                           ("Velas y ambientadores", "Candles and room scents")]),
                     tipo(("Maquillaje", "Make-up"),
                          ("Para la cara, los ojos y los labios.", "For the face, eyes and lips."),
                          [("Bases y correctores", "Foundations and concealers"), ("Ojos y cejas", "Eyes and brows"),
                           ("Labios y colorete", "Lips and blusher")]),
                     tipo(("Cuidado de la piel", "Skincare"),
                          ("Para limpiar, hidratar y proteger.", "To cleanse, moisturise and protect."),
                          [("Limpiadores y desmaquillantes", "Cleansers and make-up removers"), ("Hidratantes y sérums", "Moisturisers and serums"),
                           ("Protección solar y cuerpo", "Sun protection and body care")])],
                    cols=3, gap=20)], tono="base", anchor="tipos"),
        bloque_productos(("Novedades", "New in"),
                         ("Lo último que ha llegado. En Tienda y formatos está todo el catálogo.",
                          "The latest arrivals. The full catalogue is on the Shop and sizes page."),
                         limite=4, cols=4, anchor="novedades", tono="relleno",
                         boton_=boton(("Ver todo el catálogo", "See the whole catalogue"), a_catalogo, "secondary")),
        bloque_texto(("Te aconsejamos en la tienda", "We advise you in the shop"),
                     [("Cuéntanos qué sueles llevar, qué te gusta y para qué lo quieres. Probamos en papel y en la piel, y esperas un rato antes de decidir: un perfume cambia con las horas.",
                       "Tell us what you usually wear, what you like and what it is for. We try it on paper and on skin, and you wait a while before deciding: a perfume changes over the hours."),
                      ("Con la cosmética, lo mismo: miramos tu tipo de piel y te explicamos para qué sirve cada paso, sin llenarte el neceser de cosas que no necesitas.",
                       "With skincare, the same: we look at your skin type and explain what each step is for, without filling your wash bag with things you do not need.")],
                     eyebrow=("Asesoramiento", "Advice"),
                     imagen_=("photo", ("Mostrador de la perfumería con probadores y tiras de papel", "Perfumery counter with testers and paper strips")),
                     anchor="asesoramiento-inicio", invertir=True),
        bloque_checks(("Antes de comprar", "Before you buy"), [
            ("Muestras de algunos productos para probar en casa.", "Samples of some products to try at home."),
            ("Envoltorio para regalo y tarjeta con tu mensaje.", "Gift wrapping and a card with your message."),
            ("Recogida en tienda o envío a domicilio.", "Pick up in the shop or home delivery."),
            ("Cambios en 14 días con el producto sin abrir.", "Exchanges within 14 days if the product is unopened.")],
            eyebrow=("Tienda online", "Online store"), tono="relleno", anchor="comprar"),
        bloque_opiniones(("Lo que dicen nuestros clientes", "What our customers say"),
                         [("el asesoramiento", "the advice"), ("un regalo", "a gift"), ("el trato", "the service")], tono="base"),
        bloque_cta(("¿Buscas algo en concreto?", "Looking for something in particular?"),
                   ("Dinos qué te gusta o para quién es y te proponemos un par de opciones por WhatsApp.",
                    "Tell us what you like or who it is for and we will suggest a couple of options on WhatsApp."),
                   [whatsapp("primary"), llamar("ghost")], tono="relleno"),
    ])

    tienda = pagina("tienda", ("tienda", "shop"), ("Tienda y formatos", "Shop and sizes"),
                    ("Catálogo de perfumería y cosmética con carrito, familias olfativas, tabla de formatos, envío y devoluciones por higiene.",
                     "Perfumery and cosmetics catalogue with a cart, scent families, a table of sizes, delivery and returns for hygiene reasons."), [
        hero(("Tienda y formatos", "Shop and sizes"),
             ("Añade al carrito y paga online. Si dudas entre un frasco grande o uno pequeño, mira la tabla de formatos más abajo.",
              "Add to the cart and pay online. If you are unsure between a large bottle and a small one, see the table of sizes further down."),
             [boton(("Ver formatos", "See sizes"), a_ancla("formatos"), "secondary"), whatsapp("ghost")], variante="texto"),
        bloque_productos(("Todo el catálogo", "Full catalogue"), None, limite=24, cols=4, anchor="catalogo"),
        bloque_valores(("Familias olfativas", "Scent families"), [
            (("Cítricas y frescas", "Citrus and fresh"),
             ("Limón, bergamota, hierbas y notas de agua. Ligeras, para el día y el calor.",
              "Lemon, bergamot, herbs and watery notes. Light, for daytime and warm weather.")),
            (("Florales", "Floral"),
             ("Rosa, jazmín, azahar o flor blanca. Van de lo suave a lo intenso.",
              "Rose, jasmine, orange blossom or white flower. They range from soft to intense.")),
            (("Amaderadas", "Woody"),
             ("Cedro, sándalo o vetiver. Secas y cálidas, con mucha presencia.",
              "Cedar, sandalwood or vetiver. Dry and warm, with a lot of presence.")),
            (("Orientales y especiadas", "Oriental and spicy"),
             ("Ámbar, vainilla, pimienta o canela. Dulces o envolventes, para la noche y el frío.",
              "Amber, vanilla, pepper or cinnamon. Sweet or enveloping, for evenings and cold weather.")),
        ], cols=4, eyebrow=("Cómo elegir", "How to choose"), estilo="borde", tono="relleno", anchor="familias"),
        seccion("pricing", [
            titular(("Tabla de formatos", "Table of sizes"),
                    ("Un mismo producto suele venir en varios tamaños. Te ayudamos a elegir el que usarás de verdad.",
                     "The same product often comes in several sizes. We help you pick the one you will really use."),
                    eyebrow=("Capacidades", "Capacities")),
            tabla(("Formatos de perfumería y cosmética, con su capacidad", "Perfumery and cosmetics sizes, with capacity"),
                  [("Formato", "Size"), ("Capacidad habitual", "Usual capacity"), ("Para qué sirve", "What it is for")],
                  [(("Miniatura", "Miniature"), ("5 a 10 ml", "5 to 10 ml"), ("Probar una fragancia o llevarla en el bolso.", "Trying a fragrance or carrying it in your bag.")),
                   (("Formato de viaje", "Travel size"), ("30 ml", "30 ml"), ("Cabe en la bolsa de líquidos de cabina, donde el límite es 100 ml por envase.", "Fits in the cabin liquids bag, where the limit is 100 ml per container.")),
                   (("Frasco mediano", "Medium bottle"), ("50 ml", "50 ml"), ("El de diario: dura meses con un uso normal.", "The everyday one: lasts months with normal use.")),
                   (("Frasco grande", "Large bottle"), ("100 ml", "100 ml"), ("Para una fragancia que ya sabes que te gusta.", "For a fragrance you already know you like.")),
                   (("Crema o sérum facial", "Face cream or serum"), ("30 a 50 ml", "30 to 50 ml"), ("Uso diario durante uno o dos meses.", "Daily use for one or two months.")),
                   (("Crema de manos", "Hand cream"), ("30 a 75 ml", "30 to 75 ml"), ("Para el bolso o el cajón del trabajo.", "For your bag or your desk drawer.")),
                   (("Barra de labios", "Lipstick"), ("3 a 4 g", "3 to 4 g"), ("Se vende por peso, no por mililitros.", "Sold by weight, not by millilitres."))]),
            peq(NOTA_FORMATOS, color="$muted"),
        ], anchor="formatos", tono="base", gap=28),
        bloque_pasos(("Cambios y devoluciones", "Exchanges and returns"), [
            (("Avísanos", "Tell us"),
             ("Escríbenos o llámanos en los 14 días siguientes a recibir o recoger el pedido.",
              "Message or call us within 14 days of receiving or collecting your order.")),
            (("Sin abrir", "Unopened"),
             ("Por higiene, solo admitimos perfumes y cosmética precintados, con su celofán y su caja, y maquillaje sin usar.",
              "For hygiene reasons we only take back perfumes and skincare that are sealed, with their cellophane and box, and unused make-up.")),
            (("Tráelo o envíalo", "Bring it or send it"),
             ("En la tienda no cuesta nada. Por correo, el envío de vuelta lo pagas tú, salvo que el producto llegue defectuoso o sea otro.",
              "In the shop it costs nothing. By post, you pay the return postage, unless the product arrives faulty or is the wrong one.")),
            (("Otro producto o reembolso", "Another product or a refund"),
             ("Te cambiamos por otro producto o te devolvemos el dinero por el mismo medio de pago.",
              "We swap it for another product or refund you by the same payment method.")),
        ], anchor="devoluciones", tono="relleno"),
        bloque_checks(("Recogida y envío", "Pickup and delivery"), [
            ("Recogida en tienda, sin coste: te avisamos cuando el pedido está listo.", "Pickup in the shop at no cost: we tell you when your order is ready."),
            ("Envío a domicilio con número de seguimiento. El coste lo ves antes de pagar.", "Home delivery with a tracking number. You see the cost before you pay."),
            ("Perfumes y aerosoles son inflamables: algunos transportistas limitan su envío. Te lo decimos antes de pagar.",
             "Perfumes and aerosols are flammable: some carriers limit their shipping. We tell you before you pay.")],
            tono="base", cols=1, anchor="envios"),
        bloque_faq(("Preguntas frecuentes", "Frequently asked questions"), [
            (("¿Puedo probar un perfume antes de comprarlo online?", "Can I try a perfume before buying it online?"),
             ("Pasa por la tienda a probarlo y, si quieres, luego lo pides desde casa. También puedes pedir una miniatura o una muestra antes del frasco.",
              "Come to the shop to try it and, if you like, order it from home afterwards. You can also ask for a miniature or a sample before the full bottle.")),
            (("Tengo la piel sensible o alergias, ¿qué hago?", "I have sensitive skin or allergies, what should I do?"),
             ("Mira la lista de ingredientes en la caja o en la ficha y, si tienes dudas, enséñanosla: te ayudamos a leerla. Probar en el antebrazo antes de usarlo en la cara es buena idea. Ante una reacción, deja de usarlo.",
              "Check the ingredients list on the box or on the product page and, if in doubt, show it to us: we will help you read it. Trying a small patch on your forearm before using it on your face is a good idea. If you react, stop using it.")),
            (("¿Puedo llevar un perfume en el avión?", "Can I take a perfume on a plane?"),
             ("En la cabina, cada envase puede ser de hasta 100 ml y va en una bolsa transparente. Los frascos más grandes, en la maleta facturada.",
              "In the cabin, each container can hold up to 100 ml and goes in a clear bag. Larger bottles go in your checked luggage.")),
        ], tono="relleno"),
    ])

    aseso = pagina("asesoramiento", ("asesoramiento", "advice"), ("Asesoramiento y regalos", "Advice and gifts"),
                   ("Cómo te asesoramos en la tienda, muestras para probar en casa, regalos con envoltorio y tarjeta, y preguntas frecuentes.",
                    "How we advise you in the shop, samples to try at home, gifts with wrapping and a card, and frequently asked questions."), [
        hero(("Asesoramiento y regalos", "Advice and gifts"),
             ("Un perfume o una crema no se eligen igual en el escaparate que en la piel. Ven con calma, y si es un regalo, dinos para quién y te ayudamos.",
              "You do not choose a perfume or a cream the same way in a shop window as on your skin. Come without rushing, and if it is a gift, tell us who it is for and we will help."),
             [boton(("Hablar con una persona", "Talk to someone"), a_pagina("contacto", "contacto")), whatsapp("secondary")], variante="texto"),
        bloque_pasos(("Cómo te asesoramos", "How we advise you"), [
            (("Cuéntanos qué buscas", "Tell us what you are after"),
             ("Qué llevas ahora, qué te gusta y para qué ocasión. Si es cosmética, cómo notas la piel.",
              "What you wear now, what you like and for which occasion. For skincare, how your skin feels.")),
            (("Probamos", "We try"),
             ("En tira de papel y en la piel, de dos o tres a la vez como mucho. Si el olfato se cansa, descansa con granos de café o con el aire de la calle.",
              "On paper strips and on your skin, two or three at a time at most. If your nose tires, rest it with coffee beans or some fresh air.")),
            (("Esperas y decides", "You wait and decide"),
             ("Una fragancia cambia en una o dos horas. Sal a dar una vuelta y vuelve si te sigue gustando.",
              "A fragrance changes over an hour or two. Go for a walk and come back if you still like it.")),
            (("Te llevas una muestra", "You take a sample"),
             ("Si el producto lo permite, te damos una muestra para probar unos días en casa.",
              "If the product allows it, we give you a sample to try for a few days at home.")),
        ], anchor="como-asesoramos", tono="relleno"),
        bloque_valores(("Muestras y regalos", "Samples and gifts"), [
            (("Muestras", "Samples"),
             ("De algunas fragancias y cremas, mientras haya. Pídelas en la tienda o al hacer un pedido.",
              "Of some fragrances and creams, while stocks last. Ask for them in the shop or when you place an order.")),
            (("Envoltorio para regalo", "Gift wrapping"),
             ("Lo envolvemos en la tienda. En pedidos online, avísanos por WhatsApp al hacer el pedido.",
              "We wrap it in the shop. For online orders, tell us on WhatsApp when you place the order.")),
            (("Tarjeta con tu mensaje", "A card with your message"),
             ("Escribimos tu mensaje a mano en la tarjeta, y el regalo va sin precio.",
              "We write your message by hand on the card, and the gift goes without a price.")),
            (("Para quien no sabe qué elegir", "For when you do not know what to choose"),
             ("Dinos para quién es, qué le gusta y cuánto quieres gastar, y te proponemos opciones. Pregúntanos también por la tarjeta de regalo.",
              "Tell us who it is for, what they like and how much you want to spend, and we will suggest options. Ask us about a gift card too.")),
        ], cols=2, eyebrow=("Detalles", "Details"), estilo="borde", tono="base", anchor="regalos"),
        bloque_faq(("Preguntas frecuentes", "Frequently asked questions"), [
            (("¿Me puedo probar el maquillaje?", "Can I try the make-up?"),
             ("Sí, con probadores y, por higiene, con aplicadores desechables que te damos nosotros. Los probadores se limpian entre clientes.",
              "Yes, with testers and, for hygiene, with disposable applicators that we give you. Testers are cleaned between customers.")),
            (("¿Cuánto dura un perfume?", "How long does a perfume last?"),
             ("Un frasco de 50 ml con uso diario suele durar varios meses. Guárdalo lejos del sol y del baño: el calor y la luz lo estropean.",
              "A 50 ml bottle used daily usually lasts several months. Keep it away from sun and from the bathroom: heat and light spoil it.")),
            (("¿Puedo cambiar un regalo?", "Can I exchange a gift?"),
             ("Sí, con el ticket y el producto sin abrir, dentro de los 14 días. Si es un regalo, pide un ticket sin precio.",
              "Yes, with the receipt and the product unopened, within 14 days. If it is a gift, ask for a receipt without the price.")),
        ], tono="relleno"),
        bloque_cta(("¿Buscas un regalo?", "Looking for a gift?"), None, [whatsapp("primary"), llamar("ghost")], tono="base"),
    ])

    cont = pagina("contacto", ("contacto", "contact"), ("Contacto", "Contact"),
                  ("Horario de la perfumería, cómo llegar, teléfono, WhatsApp y formulario para pedidos, asesoramiento y regalos.",
                   "Shop hours, how to get here, phone, WhatsApp and a form for orders, advice and gifts."), [
        hero(("Contacto", "Contact"),
             ("Llámanos, escríbenos por WhatsApp o pasa por la tienda. Si es para un regalo, cuéntanos para quién es y cuánto quieres gastar.",
              "Call us, message us on WhatsApp or come by the shop. If it is a gift, tell us who it is for and how much you would like to spend."),
             [llamar(), whatsapp()], variante="texto"),
        bloque_contacto(("Estamos en la tienda", "We are in the shop"),
                        ("Si ya has hecho un pedido, dinos el número que te llegó por correo.",
                         "If you have already placed an order, tell us the number that came by email."),
                        campos_consulta(servicios=[("Un pedido", "An order"), ("Asesoramiento", "Advice"), ("Un regalo", "A gift"),
                                                   ("Cambio o devolución", "Exchange or return"), ("Otra cosa", "Something else")]),
                        ("Enviar consulta", "Send enquiry"), horas),
        bloque_mapa(("Cómo llegar a la tienda", "How to get to the shop"), tono="relleno"),
    ])

    return plantilla(KEY, ("Perfumería y cosmética", "Perfumery and cosmetics"),
                     ("Perfumería y cosmética con tienda online: fragancias, maquillaje y cuidado de la piel, tabla de formatos, asesoramiento, muestras, regalos y devoluciones por higiene.",
                      "Perfumery and cosmetics with an online store: fragrances, make-up and skincare, a table of sizes, advice, samples, gifts and returns for hygiene reasons."),
                     "comercio", ("perfumeria cosmetica perfumes fragancias colonias maquillaje cuidado de la piel cremas tienda de perfumes tienda de cosmetica perfumeria de barrio regalos muestras tienda online",
                                  "perfumery cosmetics perfume fragrance cologne make-up skincare creams perfume shop beauty shop gifts samples online shop"),
                     look, cabecera(nav, cta_cab, tienda=True),
                     pie(nav, ("Perfumería y cosmética, en la tienda y online.", "Perfumery and cosmetics, in the shop and online."),
                         ("Lunes a sábado 10:00 a 14:00 y 17:00 a 20:30.", "Monday to Saturday 10 am to 2 pm and 5 pm to 8:30 pm.")),
                     [inicio, tienda, aseso, cont], tienda=True)
