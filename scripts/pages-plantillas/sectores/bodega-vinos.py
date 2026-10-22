# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
from lib import *

KEY = "bodega-vinos"

# Paleta propia: crema y ciruela de vino tinto, clara y sobria. Pasa AA (ver comprobar_paleta).
BODEGA = dict(
    colores=dict(background="#faf6f0", surface="#efe6da", text="#26151d", muted="#665259", primary="#6b1f3f",
                 on_primary="#ffffff", border="#e2d5c6", inverse="#2b1220", on_inverse="#f8eef2"),
    fuentes=("playfair-display", "work-sans"), radio=(2, 4, 6),
)
_malos = comprobar_paleta("bodega", BODEGA["colores"])
assert not _malos, _malos

AVISO_18 = ("La venta de alcohol es solo para mayores de 18 años.", "Alcohol is sold only to people aged 18 or over.")


def build():
    nueva(KEY)
    look = BODEGA
    nav = [("inicio", ("Inicio", "Home")), ("vinos", ("Vinos y envíos", "Wines and delivery")),
           ("visitas", ("Cata y visita", "Tasting and visit")), ("contacto", ("Contacto", "Contact"))]
    horas = [(("Lunes a viernes", "Monday to Friday"),
              ("10:00 a 14:00 y 17:00 a 20:30", "10:00 am to 2:00 pm and 5:00 pm to 8:30 pm")),
             (("Sábado", "Saturday"), ("10:00 a 14:00 y 17:00 a 20:30", "10:00 am to 2:00 pm and 5:00 pm to 8:30 pm")),
             (("Domingo", "Sunday"), ("Cerrado, salvo catas con reserva", "Closed, except booked tastings"))]
    cta_cab = marco([llamar(t=("Llamar", "Call")), whatsapp("ghost", ("WhatsApp", "WhatsApp"))],
                    dir="row", gap=8, align="center")
    a_vinos = a_pagina("vinos", "catalogo")
    a_reserva = a_pagina("visitas", "reserva")
    a_tipos = a_pagina("vinos", "tipos")

    inicio = pagina("inicio", ("", ""), ("Bodega y vinos", "Winery and wines"),
                    ("Bodega y tienda de vinos con carrito: catálogo, guía para elegir por tipo, cata y visita con reserva, envíos y botones de llamar y WhatsApp.",
                     "Winery and wine shop with a cart: catalogue, guide to choosing by type, tasting and visit by booking, delivery and call and WhatsApp buttons."), [
        hero(("Vinos para beber, regalar y compartir", "Wines to drink, give and share"),
             ("Elige por tipo, por plato o por precio. Si dudas, cuéntanos qué vas a comer y te recomendamos una botella.",
              "Choose by type, by dish or by price. If you are unsure, tell us what you will be eating and we will recommend a bottle."),
             [boton(("Ver los vinos", "See the wines"), a_vinos), boton(("Reservar una cata", "Book a tasting"), a_reserva, "secondary"),
              whatsapp("ghost")],
             eyebrow=("Bodega y tienda de vinos", "Winery and wine shop"),
             imagen_=("product", ("Botellas de vino tinto y blanco sobre una barrica", "Bottles of red and white wine on a barrel")),
             tono="relleno"),
        bloque_productos(("Para empezar", "To start with"),
                         ("Una selección de la tienda. En la página Vinos y envíos está todo el catálogo. " + AVISO_18[0],
                          "A selection from the shop. The whole catalogue is on the Wines and delivery page. " + AVISO_18[1]),
                         limite=4, cols=4, anchor="seleccion",
                         boton_=boton(("Ver todo el catálogo", "See the whole catalogue"), a_vinos, "secondary")),
        bloque_valores(("Elige por tipo", "Choose by type"), [
            (("Tinto", "Red"), ("Del más fresco y afrutado al más profundo y con barrica.", "From the freshest and fruitiest to the deepest, oak-aged ones.")),
            (("Blanco", "White"), ("Fresco, de aperitivo, o con más cuerpo para la mesa.", "Fresh, for an aperitif, or fuller-bodied for the table.")),
            (("Rosado", "Rosé"), ("Ligero y frutal, bueno para el verano y los arroces.", "Light and fruity, good for summer and rice dishes.")),
            (("Espumoso", "Sparkling"), ("Para brindar, pero también para acompañar la comida.", "For toasting, but also to go with food.")),
        ], cols=4, estilo="borde", tono="relleno", anchor="tipos-resumen",
            intro=("Una guía rápida. En Vinos y envíos tienes cada tipo explicado con calma.",
                   "A quick guide. On Wines and delivery each type is explained at more length.")),
        bloque_texto(("Catar es probar con calma", "Tasting is trying with care"),
                     [("Si quieres conocer lo que vendemos, ven a una cata: probamos varios vinos con algo de picar y te contamos qué notar en cada uno.",
                       "If you want to get to know what we sell, come to a tasting: we try several wines with a little something to eat and tell you what to notice in each."),
                      ("Las catas son con reserva y en grupos pequeños. Hay opciones para dos personas y para grupos.",
                       "Tastings are by booking and in small groups. There are options for two people and for groups.")],
                     eyebrow=("Cata y visita", "Tasting and visit"),
                     imagen_=("photo", ("Copas de vino en una mesa de cata", "Wine glasses on a tasting table")),
                     invertir=True, anchor="cata"),
        bloque_cta(("¿Buscas un regalo?", "Looking for a gift?"),
                   ("Dinos para quién es, qué le gusta y cuánto quieres gastar. Te proponemos una botella o un lote, y lo envolvemos si quieres.",
                    "Tell us who it is for, what they like and how much you want to spend. We suggest a bottle or a set, and wrap it if you like."),
                   [boton(("Escribir por WhatsApp", "Message on WhatsApp"), a_wa(), "primary"), llamar("ghost")], tono="relleno"),
        bloque_opiniones(("Lo que dicen nuestros clientes", "What our customers say"),
                         [("un vino", "a wine"), ("la cata", "the tasting"), ("el envío", "the delivery")], tono="base"),
    ])

    vinos = pagina("vinos", ("vinos", "wines"), ("Vinos y envíos", "Wines and delivery"),
                   ("Catálogo con carrito, guía llana por tipo de vino, maridaje, club de vinos a rellenar, zonas y plazos de envío y preguntas frecuentes.",
                    "Catalogue with a cart, plain guide by type of wine, food pairing, a wine club to fill in, delivery areas and times, and frequently asked questions."), [
        hero(("Vinos y envíos", "Wines and delivery"),
             ("Todo el catálogo con carrito. Si no sabes cuál elegir, baja a la guía por tipo o escríbenos qué vas a comer.",
              "The whole catalogue with a cart. If you are not sure which to choose, scroll to the guide by type or message us what you will be eating."),
             [boton(("Ver el catálogo", "See the catalogue"), a_ancla("catalogo")), boton(("Guía por tipo", "Guide by type"), a_ancla("tipos"), "secondary"),
              llamar("ghost")], variante="texto"),
        bloque_productos(("Catálogo", "Catalogue"), AVISO_18, limite=24, cols=4, anchor="catalogo"),
        bloque_valores(("Qué vino es cada uno", "What each wine is"), [
            (("Tinto", "Red"),
             ("Joven: fresco y afrutado, sin paso largo por barrica. Crianza: ha pasado por barrica y botella, más redondo. Reserva: más tiempo de espera, más complejo y suave.",
              "Young: fresh and fruity, without long ageing in oak. Crianza: aged in oak and bottle, rounder. Reserva: aged for longer, more complex and smooth.")),
            (("Blanco", "White"),
             ("Los jóvenes son ligeros y cítricos, para tomar fríos. Los de más cuerpo, a veces con barrica, van mejor con la comida.",
              "Young ones are light and citrusy, to drink cold. Fuller-bodied ones, sometimes oak-aged, go better with food.")),
            (("Rosado", "Rosé"),
             ("Se hace con uva tinta y poco contacto con la piel. Es fresco y frutal, y se bebe bien frío.",
              "Made from red grapes with little skin contact. It is fresh and fruity, and best served cold.")),
            (("Espumoso", "Sparkling"),
             ("Tiene burbuja. Brut es seco, el más habitual para comer; semiseco y dulce, para postres.",
              "It has bubbles. Brut is dry, the most common one with food; medium-dry and sweet, for desserts.")),
        ], cols=2, estilo="borde", tono="relleno", anchor="tipos", intro=(
            "Cuanto más tiempo de envejecimiento, más suave y complejo, y por lo general más caro. Cualquiera sirve si te gusta.",
            "The longer it is aged, the smoother and more complex, and generally the more expensive. Any of them works if you like it.")),
        bloque_precios(("Qué vino con qué plato", "Which wine with which dish"), ("Maridaje: guía básica", "Food pairing: basic guide"),
                       [("Plato", "Dish"), ("Vino que suele ir bien", "Wine that usually works")],
                       [(("Aperitivo, jamón, queso fresco", "Aperitif, cured ham, fresh cheese"), ("Blanco joven o espumoso brut", "Young white or brut sparkling")),
                        (("Pescado, marisco y arroces", "Fish, seafood and rice dishes"), ("Blanco o rosado", "White or rosé")),
                        (("Carnes rojas, guisos y asados", "Red meat, stews and roasts"), ("Tinto crianza o reserva", "Crianza or reserva red")),
                        (("Quesos curados", "Matured cheeses"), ("Tinto con cuerpo", "Full-bodied red")),
                        (("Postres y pastelería", "Desserts and pastries"), ("Espumoso semiseco o vino dulce", "Medium-dry sparkling or sweet wine"))],
                       intro=("Son pistas, no normas: el mejor vino con la comida es el que a ti te gusta.",
                              "These are clues, not rules: the best wine with a meal is the one you like."),
                       anchor="maridaje", tono="base"),
        bloque_texto(("Club de vinos", "Wine club"),
                     [("Si te gusta descubrir vinos, el club te manda una selección cada cierto tiempo. Tú eliges cada cuánto la quieres y cuántas botellas, y puedes darte de baja cuando quieras.",
                       "If you like discovering wines, the club sends you a selection from time to time. You choose how often you want it and how many bottles, and you can cancel whenever you like."),
                      ("Texto de ejemplo: indica la frecuencia, el número de botellas y el precio de tu club, o borra esta sección si no tienes.",
                       "Example text: give the frequency, number of bottles and price of your club, or delete this section if you do not have one.")],
                     eyebrow=("Club", "Club"), tono="relleno", anchor="club"),
        bloque_precios(("Envíos", "Delivery"), ("Zonas, plazos y costes de envío", "Delivery areas, times and costs"),
                       [("Destino", "Destination"), ("Plazo", "Time"), ("Coste", "Cost")],
                       [(("Península, hasta 6 botellas", "Mainland Spain, up to 6 bottles"), ("24 a 48 horas", "24 to 48 hours"), eur(8)),
                        (("Península, más de 6 botellas", "Mainland Spain, more than 6 bottles"), ("24 a 48 horas", "24 to 48 hours"), ("Gratis", "Free")),
                        (("Recogida en la tienda", "Collection at the shop"), ("Cuando quieras en horario", "Whenever you like during opening hours"), ("Gratis", "Free"))],
                       intro=("Las botellas viajan en cajas con separadores, para que no se rompan. Quien recibe el pedido tiene que ser mayor de 18 años.",
                              "Bottles travel in boxes with dividers so they do not break. The person receiving the order must be 18 or over."),
                       nota=("Zonas, plazos y precios de ejemplo: cámbialos por los tuyos. Incluyen IVA.",
                             "Example areas, times and prices: replace them with your own. VAT included."),
                       anchor="envios", tono="base"),
        bloque_faq(("Preguntas frecuentes", "Frequently asked questions"), [
            (("¿Cómo guardo el vino en casa?", "How do I store wine at home?"),
             ("Tumbado si lleva corcho, en un sitio fresco, oscuro y sin cambios de temperatura. Lejos de la cocina y de la calefacción.",
              "Lying down if it has a cork, somewhere cool, dark and with a steady temperature. Away from the kitchen and heating.")),
            (("¿A qué temperatura se sirve?", "What temperature should I serve it at?"),
             ("Blancos, rosados y espumosos, bien fríos. Tintos jóvenes, algo frescos. Tintos de crianza, a temperatura de la sala, sin calentarlos.",
              "Whites, rosés and sparkling, well chilled. Young reds, slightly cool. Aged reds at room temperature, without warming them.")),
            (("¿Cuánto tiempo aguanta una botella abierta?", "How long does an open bottle last?"),
             ("Tápala y guárdala en la nevera: blancos y rosados, dos o tres días; tintos, un par de días. Los espumosos pierden la burbuja antes.",
              "Close it and keep it in the fridge: whites and rosés, two or three days; reds, a couple of days. Sparkling wines lose their fizz sooner.")),
            (("¿Qué hago si llega una botella rota?", "What if a bottle arrives broken?"),
             ("Haz una foto de la caja y de la botella y escríbenos el mismo día. Te la reponemos o te devolvemos el importe.",
              "Take a photo of the box and the bottle and write to us the same day. We replace it or refund you.")),
            (("¿Dais factura?", "Do you give invoices?"),
             ("Sí. Pídela al hacer el pedido y escribe los datos fiscales que quieras que figuren.",
              "Yes. Ask for it when you order and write the tax details you want shown.")),
        ], tono="relleno", anchor="preguntas"),
    ])

    form_cata = formulario(
        [campo("nombre", "text", ("Nombre", "Name"), True, auto="name", maxlen=120),
         campo("telefono", "tel", ("Teléfono", "Phone"), True, auto="tel", maxlen=30),
         campo("correo", "email", ("Correo electrónico", "Email"), False, auto="email", maxlen=200),
         campo("tipo", "select", ("¿Qué quieres reservar?", "What would you like to book?"), True,
               opciones=[("Cata de vinos", "Wine tasting"), ("Visita a la bodega y cata", "Winery visit and tasting"),
                         ("Cata para un grupo o una empresa", "Tasting for a group or company"), ("Otra cosa", "Something else")]),
         campo("fecha", "date", ("Día que te viene bien", "Day that suits you"), True),
         campo("personas", "text", ("Número de personas (todas mayores de 18)", "Number of people (all aged 18 or over)"), True, maxlen=10),
         campo("mensaje", "textarea", ("¿Alguna alergia, intolerancia o algo que debamos saber?", "Any allergies, intolerances or anything we should know?"),
               False, maxlen=1000)],
        ("Pedir reserva", "Request a booking"),
        ("Gracias. Hemos recibido tu petición y te confirmamos el día y la hora por teléfono o por correo.",
         "Thank you. We have received your request and will confirm the day and time by phone or email."))
    visitas = pagina("visitas", ("cata-y-visita", "tasting-and-visit"), ("Cata y visita", "Tasting and visit"),
                     ("Cómo son las catas y la visita a la bodega, tipos y precios, formulario de reserva y preguntas frecuentes sobre la cata.",
                      "What the tastings and winery visit are like, types and prices, a booking form and frequently asked questions about the tasting."), [
        hero(("Cata y visita con reserva", "Tasting and visit by booking"),
             ("Probamos varios vinos contigo y te explicamos qué notar en cada uno. Todo con reserva y para mayores de 18 años.",
              "We try several wines with you and explain what to notice in each. Everything by booking and for people aged 18 or over."),
             [boton(("Reservar", "Book"), a_ancla("reserva")), llamar("secondary"), whatsapp("ghost")],
             eyebrow=("Cata y visita", "Tasting and visit"), variante="centro", tono="relleno"),
        bloque_pasos(("Cómo es una cata", "What a tasting is like"), [
            (("Mirar", "Look"), ("Se mira el color en la copa, contra algo blanco.", "You look at the colour in the glass, against something white.")),
            (("Oler", "Smell"), ("Se huele quieto y luego moviendo la copa. Es lo que más cuenta.", "You smell it still and then swirling the glass. It is what counts most.")),
            (("Probar", "Taste"), ("Un sorbo pequeño, que pasee por la boca. Se nota el dulzor, la acidez y el cuerpo.", "A small sip that travels around the mouth. You notice sweetness, acidity and body.")),
            (("Comentar", "Talk"), ("No hay respuestas buenas o malas: lo que te gusta, te gusta.", "There are no right or wrong answers: what you like, you like.")),
        ], tono="base", anchor="como-es"),
        bloque_precios(("Qué se puede reservar", "What you can book"), ("Catas y visitas", "Tastings and visits"),
                       [("Opción", "Option"), ("Dura", "Lasts"), ("Incluye", "Includes"), ("Precio por persona", "Price per person")],
                       [(("Cata de vinos", "Wine tasting"), ("Una hora", "One hour"), ("Cuatro vinos con algo de picar", "Four wines with a little to eat"), eur(15)),
                        (("Visita y cata", "Visit and tasting"), ("Hora y media", "An hour and a half"), ("Recorrido y cuatro vinos con algo de picar", "Tour and four wines with a little to eat"), eur(20)),
                        (("Grupo o empresa", "Group or company"), ("A convenir", "To be agreed"), ("Cata a medida", "Tasting tailored to the group"), ("Consultar", "Ask us"))],
                       nota=("Duración, contenido y precios de ejemplo: cámbialos por los tuyos. Incluyen IVA.",
                             "Example duration, content and prices: replace them with your own. VAT included."),
                       anchor="precios", tono="relleno"),
        seccion("contact", [marco([
            marco([titular(("Reserva tu cata", "Book your tasting"),
                           ("Dinos qué día te viene bien y cuántos sois. La reserva no es firme hasta que te la confirmemos.",
                            "Tell us which day suits you and how many of you there are. The booking is not firm until we confirm it."),
                           eyebrow=("Reserva", "Booking")),
                   marco([llamar(), whatsapp()], dir="row", gap=12, wrap=True)],
                  gap=20, sz={"width": "fill", "height": "hug", "min_width": 0}),
            tarjeta([form_cata], estilo="relleno", pad=28, tag="div", sz={"width": "fill", "height": "hug", "min_width": 0}),
        ], dir="row", gap=48, align="start",
            bp={"tablet": {"layout": {"gap": 32}}, "mobile": {"layout": {"direction": "column", "gap": 28}}})],
            anchor="reserva", tono="base", gap=0),
        bloque_faq(("Preguntas frecuentes", "Frequently asked questions"), [
            (("¿Hay que tragar el vino?", "Do I have to swallow the wine?"),
             ("No. En una cata se puede escupir en la escupidera. Si vas a conducir, es lo mejor.",
              "No. At a tasting you can spit it into the spittoon. If you are driving, it is the best choice.")),
            (("¿Pueden venir menores?", "Can under-18s come?"),
             ("Para probar vino, no: solo mayores de 18 años. Los menores pueden acompañar en la visita si lo hablamos antes.",
              "Not to taste wine: only people aged 18 or over. Under-18s can join the visit if we agree it beforehand.")),
            (("Tengo una alergia. ¿Qué hago?", "I have an allergy. What should I do?"),
             ("Dínoslo al reservar. El vino puede llevar sulfitos y, a veces, clara de huevo o proteínas de la leche en la clarificación.",
              "Tell us when you book. Wine may contain sulphites and sometimes egg white or milk proteins from fining.")),
            (("¿Puedo comprar después?", "Can I buy afterwards?"),
             ("Sí. Si algo te ha gustado, te lo llevas o lo pides y te lo enviamos.",
              "Yes. If you liked something, you take it home or order it and we send it.")),
        ], tono="relleno", anchor="preguntas"),
    ])

    cont = pagina("contacto", ("contacto", "contact"), ("Contacto y horario", "Contact and opening hours"),
                  ("Teléfono, WhatsApp, horario de la tienda, cómo llegar y formulario para pedidos, regalos y consultas.",
                   "Phone, WhatsApp, shop opening hours, how to get here and a form for orders, gifts and questions."), [
        hero(("Contacto", "Contact"),
             ("Para un pedido, un regalo o una recomendación, lo más rápido es un WhatsApp. Para una cata, usa la reserva.",
              "For an order, a gift or a recommendation, the quickest way is a WhatsApp. For a tasting, use the booking."),
             [llamar(), whatsapp(), boton(("Reservar una cata", "Book a tasting"), a_reserva, "ghost")], variante="texto"),
        bloque_contacto(("Pasa por la tienda", "Come by the shop"),
                        ("Si vienes a recoger un pedido, dinos tu nombre y te lo tenemos preparado. Pedimos la edad al recoger alcohol.",
                         "If you are coming to collect an order, give us your name and we will have it ready. We check age when alcohol is collected."),
                        campos_consulta(servicios=[("Un pedido", "An order"), ("Un regalo", "A gift"), ("Un envío", "A delivery"),
                                                   ("Una cata o visita", "A tasting or visit"), ("Otra cosa", "Something else")]),
                        ("Enviar mensaje", "Send message"), horas, notas=AVISO_18),
        bloque_mapa(("Cómo llegar a la tienda", "How to get to the shop"), tono="relleno"),
    ])

    return plantilla(KEY, ("Bodega y vinos", "Winery and wines"),
                     ("Bodega y tienda de vinos con carrito, guía por tipo de vino, maridaje, cata y visita con reserva, club, envíos y botones de llamar y WhatsApp.",
                      "Winery and wine shop with a cart, guide by wine type, food pairing, tasting and visit by booking, club, delivery and call and WhatsApp buttons."),
                     "comercio", ("bodega bodegas vinoteca vinos vino tienda de vinos tinto blanco rosado espumoso cava cata de vinos catas visita bodega enoteca maridaje club de vinos",
                                  "winery wine shop wine merchant wines red white rose sparkling cava wine tasting winery visit pairing wine club"),
                     look, cabecera(nav, cta_cab, tienda=True),
                     pie(nav, ("Bodega y tienda de vinos. Venta de alcohol solo a mayores de 18 años.",
                               "Winery and wine shop. Alcohol is sold only to people aged 18 or over."),
                         ("Lunes a sábado 10:00 a 14:00 y 17:00 a 20:30.", "Monday to Saturday 10 am to 2 pm and 5 pm to 8:30 pm.")),
                     [inicio, vinos, visitas, cont], tienda=True)
