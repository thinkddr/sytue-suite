# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
from lib import *

KEY = "jugueteria"

# Crema, amarillo suave y rojo juguete sobre tinta índigo. No lleva fondo oscuro: primary/inverse no llega a 3.
CAJA = dict(
    colores=dict(background="#fffdf8", surface="#fff0c2", text="#1f1b3a", muted="#55507a", primary="#c4262e",
                 on_primary="#ffffff", border="#f0dfa8", inverse="#241f5e", on_inverse="#fff8e1"),
    fuentes=("montserrat", "dm-sans"), peso=800, h1=(50, 40, 32), radio=(12, 20, 30))

NOTA_PRECIOS = ("Precios de ejemplo: cámbialos por los tuyos. Incluyen IVA.", "Example prices: replace them with your own. VAT included.")

EDADES = [
    (("0 a 12 meses", "0 to 12 months"), ("Sonajeros, mordedores, libros de tela, alfombras de actividades.", "Rattles, teethers, cloth books, activity mats."),
     ("Nada de piezas pequeñas ni cuerdas largas.", "No small parts or long cords.")),
    (("1 a 3 años", "1 to 3 years"), ("Apilables, encajables, correpasillos, instrumentos sencillos, cocinitas.", "Stacking toys, shape sorters, ride-ons, simple instruments, play kitchens."),
     ("Los de menos de 3 años, sin piezas que quepan en la boca.", "For under-3s, nothing with parts that fit in the mouth.")),
    (("3 a 5 años", "3 to 5 years"), ("Construcciones grandes, disfraces, muñecos, puzles de pocas piezas, pintura.", "Large building sets, costumes, dolls, puzzles with few pieces, painting."),
     ("Mira la edad de la caja antes de regalar.", "Check the age on the box before giving.")),
    (("6 a 8 años", "6 to 8 years"), ("Juegos de mesa, construcciones con piezas pequeñas, manualidades, bicicletas, ciencia.", "Board games, building sets with small pieces, crafts, bikes, science."),
     ("Casco y protecciones con las ruedas.", "Helmet and pads with wheels.")),
    (("9 a 12 años", "9 to 12 years"), ("Juegos de estrategia, maquetas, puzles grandes, kits de ciencia, patinetes.", "Strategy games, model kits, large puzzles, science kits, scooters."),
     ("Los kits con pegamentos o calor, con un adulto cerca.", "Kits with glue or heat, with an adult nearby.")),
    (("Más de 12 años y adultos", "Over 12 and adults"), ("Puzles de mil piezas, juegos de cartas, juegos cooperativos, coleccionismo.", "Thousand-piece puzzles, card games, cooperative games, collecting."),
     ("Para jugar en familia, mira el número de jugadores.", "For family play, check the number of players.")),
]


def build():
    nueva(KEY)
    look = CAJA
    nav = [("inicio", ("Inicio", "Home")), ("tienda", ("Tienda por edades", "Shop by age")),
           ("regalos", ("Regalos y cambios", "Gifts and returns")), ("contacto", ("Contacto", "Contact"))]
    horas = [(("Lunes a viernes", "Monday to Friday"), ("10:00 a 14:00 y 17:00 a 20:30", "10:00 am to 2:00 pm and 5:00 pm to 8:30 pm")),
             (("Sábado", "Saturday"), ("10:00 a 14:00 y 17:00 a 20:30", "10:00 am to 2:00 pm and 5:00 pm to 8:30 pm")),
             (("Domingo", "Sunday"), ("Cerrado", "Closed"))]
    cta_cab = marco([llamar(t=("Llamar", "Call")), whatsapp("ghost", ("WhatsApp", "WhatsApp"))],
                    dir="row", gap=8, align="center")

    def ver():
        return boton(("Ver la tienda", "Browse the shop"), a_pagina("tienda", "catalogo"))

    def edades_botones():
        return marco([boton(t, a_pagina("tienda", "guia-edades"), "secondary", sz=None)
                      for t in (("0 a 2 años", "0 to 2 years"), ("3 a 5", "3 to 5"), ("6 a 8", "6 to 8"), ("9 o más", "9 and over"))],
                     dir="row", gap=8, wrap=True)

    inicio = pagina("inicio", ("", ""), ("Juguetería", "Toy shop"),
                    ("Juguetería con tienda online: juguetes por edades, regalos y listas de cumpleaños, envoltorio, cambios y tienda física.",
                     "Toy shop with an online store: toys by age, gifts and birthday lists, gift wrapping, returns and a physical shop."), [
        hero(("Juguetes para cada edad, a un paso de casa", "Toys for every age, a short walk from home"),
             ("Elige por edad, pide online y recoge en la tienda o recíbelo en casa. Y si no sabes qué regalar, te aconsejamos.",
              "Choose by age, order online and collect in the shop or have it sent to your door. And if you do not know what to give, we will advise you."),
             [ver(), llamar("secondary"), whatsapp("ghost")],
             eyebrow=("Juguetería", "Toy shop"), variante="centro", tono="relleno",
             extra=edades_botones()),
        bloque_productos(("Novedades", "New in"),
                         ("Lo último que ha llegado a la tienda. En la página Tienda está todo el catálogo.",
                          "The latest arrivals. The full catalogue is on the Shop page."),
                         limite=4, cols=4, anchor="novedades",
                         boton_=boton(("Ver todo el catálogo", "See the whole catalogue"), a_pagina("tienda", "catalogo"), "secondary")),
        bloque_valores(("Para regalar sin fallar", "Gifts that hit the mark"), [
            (("Te aconsejamos", "We advise you"),
             ("Dinos edad, gustos y cuánto quieres gastar. Te proponemos tres opciones y eliges.",
              "Tell us the age, interests and how much you want to spend. We suggest three options and you choose.")),
            (("Lista de cumpleaños", "Birthday list"),
             ("Los invitados miran la lista y nadie repite regalo.", "Guests look at the list and nobody doubles up on a present.")),
            (("Lo envolvemos", "We wrap it"),
             ("Papel, lazo y tarjeta, listo para llevar a la fiesta.", "Paper, ribbon and a card, ready to take to the party.")),
        ], tono="relleno", estilo="borde", anchor="regalos-resumen"),
        bloque_texto(("Ven a jugar con ellos antes de comprar", "Come and try them before you buy"),
                     [("En la tienda puedes ver el tamaño real, tocar los materiales y preguntarnos cómo funciona cada juego.",
                       "In the shop you can see the real size, feel the materials and ask us how each game works."),
                      ("Abrimos de lunes a sábado, mañana y tarde. Horario y mapa, en Contacto.",
                       "We are open Monday to Saturday, mornings and afternoons. Hours and map are on the Contact page.")],
                     eyebrow=("Tienda física", "In-store"),
                     imagen_=("photo", ("Estantes de juguetes de colores en la juguetería", "Shelves of colourful toys in the toy shop")),
                     anchor="tienda-fisica"),
        bloque_opiniones(("Lo que dicen las familias", "What families say"),
                         [("un regalo", "a gift"), ("la atención", "the service"), ("el envío", "the delivery")], tono="relleno"),
        bloque_cta(("¿No sabes qué regalar?", "Do not know what to give?"),
                   ("Escríbenos la edad y los gustos y te decimos qué tenemos.", "Message us the age and interests and we will tell you what we have."),
                   [whatsapp("primary"), llamar("ghost")], tono="base"),
    ])

    tienda = pagina("tienda", ("tienda", "shop"), ("Tienda por edades", "Shop by age"),
                    ("Catálogo de juguetes con carrito y una guía de edades: qué suele gustar y a qué prestar atención en cada tramo.",
                     "Toy catalogue with a cart and an age guide: what children usually enjoy and what to watch for at each stage."), [
        hero(("Tienda por edades", "Shop by age"),
             ("Añade al carrito y paga online. Recoge en la tienda sin coste o recíbelo en casa.",
              "Add to the cart and pay online. Collect in the shop at no cost or have it delivered to your door."),
             [boton(("Guía de edades", "Age guide"), a_ancla("guia-edades"), "secondary"), whatsapp("ghost")], variante="texto"),
        bloque_productos(("Todo el catálogo", "Full catalogue"), None, limite=24, cols=4, anchor="catalogo"),
        bloque_precios(("Guía de edades", "Age guide"), ("Guía de juguetes por edades", "Toy guide by age"),
                       [("Edad", "Age"), ("Suele gustar", "Usually enjoyed"), ("Ojo con", "Watch for")],
                       EDADES,
                       intro=("Cada niño va a su ritmo: es una orientación, no una regla. Si dudas, pregúntanos.",
                              "Every child goes at their own pace: this is a guide, not a rule. If in doubt, ask us."),
                       nota=("La edad que pone la caja es una indicación de seguridad, no solo de gusto.",
                             "The age on the box is a safety guide, not just a matter of taste."),
                       anchor="guia-edades", tono="relleno", kind="features"),
        bloque_checks(("Seguridad, en una frase", "Safety in a nutshell"), [
            ("Busca el marcado CE en el juguete o en la caja: indica que el fabricante declara que cumple la normativa europea.",
             "Look for the CE mark on the toy or box: it means the maker declares it meets European rules."),
            ("Si la caja avisa de que no es para menores de 3 años, hazle caso: lleva piezas pequeñas que se pueden tragar.",
             "If the box says it is not for under-3s, believe it: it has small parts that can be swallowed."),
            ("Las pilas de botón y los imanes pequeños son un peligro si se tragan: guárdalos fuera de su alcance.",
             "Button batteries and small magnets are dangerous if swallowed: keep them out of reach."),
            ("Pregúntanos si tienes dudas: te ayudamos a leer la caja.", "Ask us if in doubt: we help you read the box.")],
            tono="base", anchor="seguridad"),
        bloque_cta(("¿Algo que no ves en el catálogo?", "Something you cannot see in the catalogue?"),
                   ("A veces nos queda en la tienda lo que aún no hemos subido. Pregúntanos por WhatsApp.",
                    "Sometimes we still have in the shop what we have not uploaded yet. Ask us on WhatsApp."),
                   [whatsapp("primary"), llamar("ghost")], tono="relleno"),
    ])

    regalos = pagina("regalos", ("regalos", "gifts"), ("Regalos y cambios", "Gifts and returns"),
                     ("Listas de cumpleaños, envoltorio de regalo, envíos y plazos, y cómo cambiar o devolver un juguete.",
                      "Birthday lists, gift wrapping, shipping and times, and how to exchange or return a toy."), [
        hero(("Regalos y cambios", "Gifts and returns"),
             ("Cómo funciona la lista de cumpleaños, cuánto cuesta envolver y qué hacer si el regalo no gusta o no encaja.",
              "How the birthday list works, what wrapping costs and what to do if the gift is not liked or does not fit."),
             [boton(("Cambios y devoluciones", "Returns"), a_ancla("cambios"), "secondary"), whatsapp("ghost")], variante="texto"),
        bloque_pasos(("La lista de cumpleaños", "The birthday list"), [
            (("Elegís", "You choose"),
             ("Venís a la tienda con el niño o nos mandáis una lista por WhatsApp con lo que le hace ilusión.",
              "You come to the shop with your child or send us a list on WhatsApp of what they would love.")),
            (("La guardamos", "We keep it"),
             ("Apuntamos nombre y fecha de la fiesta y dejamos la lista en la tienda.", "We note the name and party date and keep the list in the shop.")),
            (("Los invitados llaman", "Guests call"),
             ("Quien quiera regalar nos llama o nos escribe y le decimos qué queda libre.", "Anyone who wants to give a present calls or messages us and we tell them what is still free.")),
            (("Nadie repite", "No doubles"),
             ("Vamos tachando lo que se compra. Lo que no se compra, se queda para otra ocasión.", "We tick off what is bought. What is not bought stays for another occasion.")),
        ], tono="base", anchor="listas"),
        bloque_precios(("Envoltorio y envío", "Wrapping and shipping"), ("Opciones de envoltorio y entrega", "Wrapping and delivery options"),
                       [("Opción", "Option"), ("Plazo", "Time"), ("Coste", "Cost")],
                       [(("Envoltorio de regalo en tienda", "Gift wrapping in the shop"), ("Mientras esperas", "While you wait"), ("Gratis", "Free")),
                        (("Envoltorio con tarjeta en pedidos online", "Wrapping with a card on online orders"), ("En la preparación del pedido", "When the order is prepared"), eur(2)),
                        (("Recogida en tienda", "In-store pickup"), ("En 24 horas", "Within 24 hours"), ("Gratis", "Free")),
                        (("Envío a domicilio (península)", "Home delivery (mainland)"), ("2 a 3 días laborables", "2 to 3 working days"), eur(4.9))],
                       intro=("Si es para una fiesta, pide con tiempo: lo preparamos y te avisamos cuando puedes pasar a por él.",
                              "If it is for a party, order early: we prepare it and tell you when you can come for it."),
                       nota=NOTA_PRECIOS, anchor="envoltorio", tono="relleno"),
        bloque_pasos(("Cambios y devoluciones", "Exchanges and returns"), [
            (("Avísanos", "Tell us"),
             ("Escríbenos o llámanos dentro de los 14 días desde que lo recibes o lo recoges.", "Message or call us within 14 days of receiving or collecting it.")),
            (("Prepáralo", "Get it ready"),
             ("Con su caja, sus instrucciones y todas las piezas. Si lo estrenas un rato para probarlo, no pasa nada.",
              "With its box, instructions and all the pieces. If you try it out for a while, that is fine.")),
            (("Tráelo o envíalo", "Bring or send it"),
             ("En tienda no cuesta nada. Por correo, el envío de vuelta corre de tu cuenta, salvo que llegue defectuoso.",
              "In the shop it costs nothing. By post, the return shipping is on you, unless it arrives faulty.")),
            (("Cambio o reembolso", "Exchange or refund"),
             ("Cambiamos por otro o te devolvemos el dinero por el mismo medio de pago.", "We swap it for another or refund you by the original payment method.")),
        ], intro=("Si un juguete llega roto o no funciona, la garantía legal cubre tres años para productos nuevos: escríbenos con una foto.",
                  "If a toy arrives broken or does not work, the legal guarantee covers three years for new goods: message us with a photo."),
            tono="base", anchor="cambios"),
        bloque_faq(("Preguntas frecuentes", "Frequently asked questions"), [
            (("¿Puedo cambiar un regalo sin ticket?", "Can I exchange a gift without a receipt?"),
             ("Sí: pide el ticket regalo al comprarlo y quien lo recibe puede cambiarlo sin que se vea el precio.",
              "Yes: ask for a gift receipt when you buy it and the recipient can exchange it without seeing the price.")),
            (("¿Cuánto tardáis en preparar un pedido?", "How long do you take to prepare an order?"),
             ("Normalmente 24 horas laborables. Si algo no está en tienda, te lo decimos antes de cobrar.",
              "Usually 24 working hours. If something is not in the shop, we tell you before charging you.")),
            (("¿Tenéis juguetes de segunda mano?", "Do you have second-hand toys?"),
             ("Pregúntanos en la tienda: te decimos qué hay y en qué estado.", "Ask us in the shop: we tell you what we have and its condition.")),
            (("¿Los pedidos online se pueden recoger y pagar en tienda?", "Can online orders be collected and paid for in the shop?"),
             ("Sí. Haces el pedido, lo preparamos y pagas al recogerlo, si lo prefieres.",
              "Yes. You place the order, we prepare it and you pay on collection, if you prefer.")),
        ], tono="relleno"),
        bloque_cta(("¿Te preparamos el regalo?", "Shall we get the gift ready?"), None, [whatsapp("primary"), llamar("ghost")], tono="base"),
    ])

    cont = pagina("contacto", ("contacto", "contact"), ("Contacto", "Contact"),
                  ("Horario de la juguetería, cómo llegar, teléfono, WhatsApp y formulario para pedidos, listas y regalos.",
                   "Toy shop hours, how to get here, phone, WhatsApp and a form for orders, lists and gifts."), [
        hero(("Contacto", "Contact"),
             ("Llámanos, escríbenos por WhatsApp o pásate por la tienda. Para un regalo, dinos la edad y los gustos.",
              "Call us, message us on WhatsApp or come by the shop. For a gift, tell us the age and interests."),
             [llamar(), whatsapp()], variante="texto"),
        bloque_contacto(("Estamos en la tienda", "We are in the shop"),
                        ("Si ya has hecho un pedido online, dinos el número que te llegó por correo.",
                         "If you have already placed an online order, tell us the number you received by email."),
                        campos_consulta(servicios=[("Un pedido", "An order"), ("Un regalo", "A gift"), ("Lista de cumpleaños", "Birthday list"),
                                                   ("Cambio o devolución", "Exchange or return"), ("Otra cosa", "Something else")]),
                        ("Enviar consulta", "Send enquiry"), horas),
        bloque_mapa(("Cómo llegar a la tienda", "How to get to the shop"), tono="relleno"),
    ])

    return plantilla(KEY, ("Juguetería", "Toy shop"),
                     ("Juguetería con catálogo y carrito, guía de edades, listas de cumpleaños, envoltorio de regalo, cambios y botones de llamar y WhatsApp.",
                      "Toy shop with a catalogue and cart, age guide, birthday lists, gift wrapping, returns and call and WhatsApp buttons."),
                     "comercio", ("jugueteria juguetes tienda de juguetes tienda online regalos cumpleanos lista de cumpleanos juegos de mesa puzles muñecos ninos comercio",
                                  "toy shop toyshop toys toy store online shop gifts birthday list board games puzzles dolls kids children"),
                     look, cabecera(nav, cta_cab, tienda=True),
                     pie(nav, ("Juguetería de barrio, en la tienda y online.", "Neighbourhood toy shop, in the shop and online."),
                         ("Lunes a sábado 10:00 a 14:00 y 17:00 a 20:30.", "Monday to Saturday 10 am to 2 pm and 5 pm to 8:30 pm.")),
                     [inicio, tienda, regalos, cont], tienda=True)
