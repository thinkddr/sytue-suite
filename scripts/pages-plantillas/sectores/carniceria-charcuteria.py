# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
from lib import *

KEY = "carniceria-charcuteria"

# Papel de carnicero: fondo kraft claro, tinta oscura y un rojo sobrio. Pasa AA (lo comprueba lib.comprobar_paleta).
KRAFT = dict(
    colores=dict(background="#f8f2e7", surface="#ece2cf", text="#231a14", muted="#5f5144", primary="#9b2226",
                 on_primary="#ffffff", border="#dccfb8", inverse="#2b1a16", on_inverse="#f8efe3"),
    fuentes=("lora", "ibm-plex-sans"), peso=700, radio=(2, 6, 10), ancho=1140)

NOTA_PRECIOS = ("Precios de ejemplo: cámbialos por los tuyos. Incluyen IVA. El precio final depende del peso.",
                "Example prices: replace them with your own. VAT included. The final price depends on the weight.")
KILO = [("Pieza o producto", "Cut or product"), ("Precio por kilo", "Price per kilo")]


def sec_form(titulo, intro, campos, envio, exito, anchor, tono="base"):
    return seccion("contact", [titular(titulo, intro),
                               tarjeta([formulario(campos, envio, exito)], estilo="relleno" if tono == "base" else "borde",
                                       pad=28, tag="div", sz=tam(max_width=640))],
                   tono=tono, anchor=anchor, gap=28)


def campos_encargo():
    return [campo("nombre", "text", ("Nombre", "Name"), True, auto="name", maxlen=120),
            campo("telefono", "tel", ("Teléfono", "Phone"), True, auto="tel", maxlen=30),
            campo("fecha", "date", ("Fecha de recogida", "Pick-up date"), True),
            campo("hora", "select", ("Hora de recogida", "Pick-up time"), False,
                  opciones=[("Por la mañana", "In the morning"), ("Por la tarde", "In the afternoon")]),
            campo("ocasion", "select", ("Para qué es", "What it is for"), False,
                  opciones=[("Comida del fin de semana", "Weekend meal"), ("Barbacoa", "Barbecue"),
                            ("Celebración o fiesta", "Celebration or party"), ("Otra", "Something else")]),
            campo("pedido", "textarea", ("Qué necesitas (piezas, cantidad y cómo las quieres cortadas)",
                                         "What you need (cuts, amounts and how you want them cut)"), True, maxlen=1500)]


def build():
    nueva(KEY)
    look = KRAFT
    nav = [("inicio", ("Inicio", "Home")), ("productos", ("Carne y embutidos", "Meat and charcuterie")),
           ("encargos", ("Encargos", "Orders")), ("contacto", ("Contacto", "Contact"))]
    horas = [(("Lunes a viernes", "Monday to Friday"), ("8:30 a 14:00 y de 17:00 a 20:00", "8:30 am to 2:00 pm and 5:00 pm to 8:00 pm")),
             (("Sábado", "Saturday"), ("8:30 a 14:30", "8:30 am to 2:30 pm")),
             (("Domingo", "Sunday"), ("Cerrado", "Closed"))]
    cta_cab = marco([llamar(), whatsapp("ghost")], dir="row", gap=8, wrap=True, align="center")
    a_encargo = a_pagina("encargos", "hacer-encargo")

    inicio = pagina("inicio", ("", ""), ("Carnicería y charcutería", "Butcher's and charcuterie"),
                    ("Carnicería y charcutería de barrio: carne por familias, embutidos, encargos para el fin de semana y botones de llamar y WhatsApp.",
                     "Neighbourhood butcher's and charcuterie: meat by type, cured meats, weekend orders and call and WhatsApp buttons."), [
        hero(("Carne y embutidos, cortados como los quieres", "Meat and cured meats, cut the way you want them"),
             ("Ternera, cerdo, pollo, cordero, embutidos y platos elaborados. Pasa por el mostrador o encarga para el fin de semana y recógelo preparado.",
              "Beef, pork, chicken, lamb, cured meats and ready dishes. Drop by the counter or order for the weekend and collect it ready."),
             [boton(("Hacer un encargo", "Place an order"), a_encargo), llamar("secondary"), whatsapp("ghost")],
             eyebrow=("Carnicería y charcutería", "Butcher's and charcuterie"),
             imagen_=("photo", ("Mostrador de la carnicería con piezas de carne y embutidos", "Butcher's counter with cuts of meat and cured meats"))),
        bloque_valores(("Qué encontrarás en el mostrador", "What you will find at the counter"), [
            (("Ternera", "Beef"), ("Filetes, guiso, carne picada, entrecot y piezas para asar.", "Steaks, stewing beef, mince, rib-eye and joints for roasting.")),
            (("Cerdo", "Pork"), ("Lomo, costilla, panceta, secreto y todo lo que pide la barbacoa.", "Loin, ribs, belly, secreto and everything a barbecue needs.")),
            (("Pollo y conejo", "Chicken and rabbit"), ("Enteros o en piezas, troceados o abiertos para la plancha.", "Whole or in pieces, jointed or butterflied for the griddle.")),
            (("Cordero", "Lamb"), ("Chuletillas, pierna, paletilla y cuello para guisar.", "Cutlets, leg, shoulder and neck for stewing.")),
            (("Embutidos y curados", "Cured meats"), ("Chorizo, salchichón, jamón y lo que corresponda a tu zona, cortado al peso.", "Chorizo, salchichón, ham and local specialities, sliced by weight.")),
            (("Elaborados", "Ready dishes"), ("Hamburguesas, albóndigas, croquetas y platos cocinados para llevar.", "Burgers, meatballs, croquettes and cooked dishes to take away.")),
        ], intro=("Lo que hay cada día cambia según la semana. Si buscas algo que no ves, pregúntalo: lo preparamos o lo encargamos.",
                  "What we have changes through the week. If you cannot see what you are after, ask: we prepare it or order it in."),
            tono="relleno", estilo="borde", anchor="familias"),
        bloque_pasos(("Encarga para el fin de semana", "Order for the weekend"), [
            (("Dinos qué necesitas", "Tell us what you need"), ("Por WhatsApp, por teléfono o con el formulario: piezas, cantidades y para cuántos sois.",
                                                                 "By WhatsApp, phone or the form: cuts, amounts and how many of you there are.")),
            (("Elige la fecha de recogida", "Pick a collection date"), ("Para el fin de semana, encarga hasta el jueves. Para fiestas, con una semana.",
                                                                        "For the weekend, order by Thursday. For holidays, a week ahead.")),
            (("Te lo dejamos preparado", "We get it ready"), ("Cortado, picado o abierto como pidas, envasado y con tu nombre.",
                                                               "Cut, minced or butterflied as you ask, packed and with your name on it.")),
            (("Pasas a recogerlo", "You collect it"), ("Pagas al recoger, en efectivo o con tarjeta.", "You pay on collection, in cash or by card.")),
        ], tono="base", anchor="como-encargar"),
        bloque_texto(("De dónde viene la carne", "Where the meat comes from"), [
            ("Aquí cuentas, con tus palabras, de dónde viene lo que vendes: la zona, el proveedor o la explotación, y cuándo llega al mostrador. Es un texto de ejemplo: cámbialo por el tuyo y no pongas nada que no sea verdad.",
             "Here you say, in your own words, where what you sell comes from: the area, the supplier or the farm, and when it reaches the counter. This is an example text: replace it with your own and do not write anything that is not true."),
            ("Cada pieza lleva su etiqueta con el origen. Si quieres saber más de una en concreto, pregúntanos.",
             "Every cut carries its label with the origin. If you want to know more about a particular one, ask us.")],
            eyebrow=("Procedencia", "Origin"), tono="relleno", anchor="procedencia", invertir=True,
            imagen_=("landscape", ("Pasto con ganado en el campo", "Pasture with livestock in the countryside"))),
        bloque_opiniones(("Lo que dicen los vecinos", "What the neighbours say"),
                         [("la calidad de la carne", "the quality of the meat"), ("el trato en el mostrador", "the service at the counter"),
                          ("los encargos", "the orders")], tono="base"),
        bloque_cta(("¿Preparamos tu pedido?", "Shall we get your order ready?"),
                   ("Escríbenos lo que necesitas y la fecha, y te confirmamos.", "Message us what you need and the date, and we will confirm."),
                   [boton(("Hacer un encargo", "Place an order"), a_encargo), llamar("secondary"), whatsapp("ghost")]),
    ])

    productos = pagina("productos", ("productos", "products"), ("Carne y embutidos", "Meat and charcuterie"),
                       ("Precios orientativos por kilo, consejos de cocinado y conservación, procedencia, alérgenos y preguntas frecuentes.",
                        "Guide prices per kilo, cooking and storage tips, origin, allergens and frequently asked questions."), [
        hero(("Carne y embutidos por familias", "Meat and cured meats by type"),
             ("Una guía de lo que vendemos con precios orientativos por kilo, cómo cocinarlo y cómo guardarlo.",
              "A guide to what we sell with guide prices per kilo, how to cook it and how to store it."),
             [boton(("Encargar", "Order"), a_encargo), whatsapp("secondary")], variante="texto"),
        bloque_carta(("Precios orientativos por kilo", "Guide prices per kilo"), [
            (("Ternera", "Beef"), ("Precios de ternera", "Beef prices"), KILO,
             [(("Filetes de ternera", "Beef steaks"), eur(19)), (("Carne picada de ternera", "Minced beef"), eur(11)),
              (("Ternera para guisar", "Stewing beef"), eur(13))]),
            (("Cerdo", "Pork"), ("Precios de cerdo", "Pork prices"), KILO,
             [(("Lomo de cerdo", "Pork loin"), eur(9)), (("Costilla", "Ribs"), eur(7.5)), (("Panceta", "Belly"), eur(8))]),
            (("Pollo y conejo", "Chicken and rabbit"), ("Precios de pollo y conejo", "Chicken and rabbit prices"), KILO,
             [(("Pollo entero", "Whole chicken"), eur(5.5)), (("Pechuga de pollo", "Chicken breast"), eur(9)),
              (("Conejo troceado", "Jointed rabbit"), eur(10))]),
            (("Cordero", "Lamb"), ("Precios de cordero", "Lamb prices"), KILO,
             [(("Chuletillas", "Cutlets"), eur(22)), (("Pierna", "Leg"), eur(16)), (("Cuello para guisar", "Neck for stewing"), eur(12))]),
            (("Embutidos y curados", "Cured meats"), ("Precios de embutidos", "Cured meat prices"), KILO,
             [(("Chorizo", "Chorizo"), eur(14)), (("Salchichón", "Salchichón"), eur(17)), (("Jamón curado", "Cured ham"), eur(28))]),
            (("Elaborados", "Ready dishes"), ("Precios de elaborados", "Ready dish prices"), KILO,
             [(("Hamburguesas de ternera", "Beef burgers"), eur(10)), (("Albóndigas", "Meatballs"), eur(9)),
              (("Croquetas caseras", "Home-made croquettes"), eur(12))]),
        ], nota=NOTA_PRECIOS, tono="relleno", anchor="precios"),
        bloque_valores(("Cocinado y conservación", "Cooking and storage"), [
            (("Sácala antes de cocinarla", "Take it out before cooking"),
             ("Una pieza gruesa, un rato fuera de la nevera antes de ir a la plancha o al horno, se hace más pareja.",
              "A thick cut left out of the fridge for a while before it goes on the griddle or in the oven cooks more evenly.")),
            (("Pollo y cerdo, bien hechos", "Chicken and pork, well cooked"),
             ("Cocínalos hasta que no quede nada rosado por dentro. Lava manos y tabla después de tocar carne cruda.",
              "Cook them until nothing is pink inside. Wash your hands and the board after handling raw meat.")),
            (("La carne picada, pronto", "Mince, soon"),
             ("Se estropea antes que una pieza entera: cocínala ese mismo día o al siguiente, o congélala el día que la compras.",
              "It goes off sooner than a whole cut: cook it the same day or the next, or freeze it the day you buy it.")),
            (("Congelar bien", "Freezing well"),
             ("Envuélvela sin aire, anota la fecha y descongélala en la nevera, no sobre la encimera.",
              "Wrap it with no air, write the date and thaw it in the fridge, not on the worktop.")),
        ], cols=2, tono="base", estilo="relleno", anchor="cocinado",
            intro=("Consejos generales. Si dudas con una pieza, pregúntanos cómo prepararla.",
                   "General tips. If you are unsure about a cut, ask us how to prepare it.")),
        bloque_texto(("Alérgenos", "Allergens"), [
            ("Los embutidos y los elaborados pueden llevar gluten, leche, huevo, soja, mostaza o sulfitos. Una hamburguesa o una albóndiga suelen llevar pan rallado.",
             "Cured meats and ready dishes can contain gluten, milk, egg, soy, mustard or sulphites. A burger or a meatball often contains breadcrumbs."),
            ("Pregúntanos antes de comprar: tenemos por escrito qué lleva cada producto.",
             "Ask us before you buy: we have in writing what each product contains.")],
            anchor="alergenos", tono="relleno"),
        bloque_faq(("Preguntas frecuentes", "Frequently asked questions"), [
            (("¿Cuánta carne compro por persona?", "How much meat should I buy per person?"),
             ("Unos 200 a 250 gramos de carne sin hueso, o 350 a 400 si lleva hueso. Para una barbacoa, cuenta algo más.",
              "About 200 to 250 grams of boneless meat, or 350 to 400 with bone. For a barbecue, allow a little more.")),
            (("¿Me la cortáis o me la picáis?", "Will you cut or mince it for me?"),
             ("Sí. Dinos cómo la quieres: en filetes, troceada, picada, abierta o atada para el horno.",
              "Yes. Tell us how you want it: in steaks, jointed, minced, butterflied or tied for the oven.")),
            (("¿Tenéis algo sin gluten?", "Do you have anything gluten-free?"),
             ("La carne fresca sin adobo no lleva gluten. En embutidos y elaborados depende del producto: pregúntanos y lo miramos juntos.",
              "Fresh meat with no marinade contains no gluten. In cured meats and ready dishes it depends on the product: ask us and we will check together.")),
            (("¿Puedo pedir una pieza que no veo?", "Can I ask for a cut I cannot see?"),
             ("Dínoslo y la preparamos o la pedimos. Para piezas grandes, avisa con unos días.", "Tell us and we will prepare or order it. For large cuts, let us know a few days ahead.")),
        ], tono="base"),
        bloque_cta(("¿Qué necesitas esta semana?", "What do you need this week?"), None, tono="relleno"),
    ])

    encargos = pagina("encargos", ("encargos", "orders"), ("Encargos y celebraciones", "Orders and celebrations"),
                      ("Encargos para el fin de semana y celebraciones: cómo pedirlo, ideas, raciones y formulario con fecha de recogida.",
                       "Weekend orders and celebrations: how to ask, ideas, portions and a form with a collection date."), [
        hero(("Encargos y celebraciones", "Orders and celebrations"),
             ("Cuéntanos qué necesitas y para cuándo. Te lo dejamos preparado y lo recoges sin esperar cola.",
              "Tell us what you need and for when. We get it ready and you collect it without queuing."),
             [boton(("Rellenar el encargo", "Fill in the order"), a_ancla("hacer-encargo")), whatsapp("secondary")], variante="texto"),
        bloque_valores(("Ideas para encargar", "Ideas to order"), [
            (("Barbacoa del fin de semana", "Weekend barbecue"),
             ("Costillas, chuletillas, panceta, hamburguesas y chorizo para parrilla, en la cantidad que necesites.",
              "Ribs, cutlets, belly, burgers and chorizo for the grill, in the amount you need.")),
            (("Asado para celebrar", "Roast for a celebration"),
             ("Cordero, cerdo o pollo para el horno, limpio, atado y con el peso que corresponda a vuestro grupo.",
              "Lamb, pork or chicken for the oven, cleaned, tied and in the weight your group needs.")),
            (("Tabla de embutidos", "Cured meat platter"),
             ("Embutidos cortados al peso para picar, listos para poner en la mesa.", "Cured meats sliced by weight for sharing, ready for the table.")),
            (("Comida de fiestas", "Holiday meal"),
             ("Piezas grandes y guisos para Navidad o fechas señaladas. Encarga con una semana.", "Large cuts and stews for Christmas or special dates. Order a week ahead.")),
        ], cols=2, tono="relleno", estilo="borde", anchor="ideas"),
        bloque_pasos(("Cómo funciona", "How it works"), [
            (("Pides", "You ask"), ("Con el formulario, por WhatsApp o por teléfono.", "With the form, by WhatsApp or by phone.")),
            (("Confirmamos", "We confirm"), ("Te decimos si hay todo y el precio aproximado.", "We tell you if we have everything and the approximate price.")),
            (("Preparamos", "We prepare"), ("Cortamos y envasamos con tu nombre.", "We cut and pack it with your name on it.")),
            (("Recoges y pagas", "You collect and pay"), ("En la tienda, en efectivo o con tarjeta.", "At the shop, in cash or by card.")),
        ], tono="base"),
        sec_form(("Haz tu encargo", "Place your order"),
                 ("Escribe lo que necesitas y el día que vendrás. Te confirmamos por teléfono o WhatsApp.",
                  "Write what you need and the day you will come. We will confirm by phone or WhatsApp."),
                 campos_encargo(), ("Enviar encargo", "Send order"),
                 ("Gracias. Hemos recibido tu encargo y te lo confirmamos lo antes posible.", "Thank you. We have received your order and will confirm it as soon as possible."),
                 "hacer-encargo", tono="relleno"),
        bloque_checks(("Antes de recoger", "Before you collect"), [
            ("Trae el resguardo o dinos tu nombre: el pedido está a tu nombre.", "Bring your receipt or tell us your name: the order is in your name."),
            ("Si cambian los comensales, avísanos con un día para ajustar el peso.", "If the number of guests changes, tell us a day ahead so we can adjust the weight."),
            ("Para llevarlo lejos, pide una bolsa isotérmica o trae la tuya.", "To take it far, ask for an insulated bag or bring your own."),
            ("Si tienes alergias en casa, díselo a quien te atienda al encargar.", "If anyone at home has allergies, tell whoever serves you when you order."),
        ], cols=2, tono="base"),
    ])

    cont = pagina("contacto", ("contacto", "contact"), ("Contacto y cómo llegar", "Contact and directions"),
                  ("Teléfono, WhatsApp, formulario, horario del mostrador y mapa.",
                   "Phone, WhatsApp, form, counter hours and map."), [
        hero(("Contacto", "Contact"),
             ("Llámanos, escríbenos por WhatsApp o pasa por el mostrador. Para encargos, mira la página de encargos.",
              "Call us, message us on WhatsApp or drop by the counter. For orders, see the orders page."),
             [llamar(), whatsapp()], variante="texto"),
        bloque_contacto(("Dónde estamos", "Where we are"),
                        ("Pasa por el mostrador o escríbenos. Para consultas rápidas, lo más ágil es WhatsApp.",
                         "Drop by the counter or write to us. For quick questions, WhatsApp is quickest."),
                        campos_cita(servicios=[("Hacer un encargo", "Place an order"), ("Una pregunta", "A question"), ("Otra cosa", "Something else")],
                                    fecha=False),
                        ("Enviar mensaje", "Send message"), horas,
                        notas=("Los festivos pueden cambiar el horario: lo avisamos en el mostrador.",
                               "Holidays may change the hours: we put a notice on the counter.")),
        bloque_mapa(tono="relleno"),
    ])

    return plantilla(KEY, ("Carnicería y charcutería", "Butcher's and charcuterie"),
                     ("Carne por familias, embutidos, precios orientativos, encargos con fecha de recogida, alérgenos, horario y mapa, con botones de llamar y WhatsApp.",
                      "Meat by type, cured meats, guide prices, orders with a collection date, allergens, hours and a map, with call and WhatsApp buttons."),
                     "comercio", ("carniceria charcuteria carnicero carne embutidos ternera cerdo pollo cordero jamoneria tienda de barrio encargos barbacoa hamburguesas",
                                  "butcher butchers charcuterie meat shop cured meats beef pork chicken lamb deli orders barbecue burgers"),
                     look, cabecera(nav, cta_cab),
                     pie(nav, ("Carnicería y charcutería. Encargos para el fin de semana y celebraciones.",
                               "Butcher's and charcuterie. Orders for the weekend and celebrations."),
                         ("Lunes a viernes 8:30 a 14:00 y 17:00 a 20:00. Sábado 8:30 a 14:30.",
                          "Monday to Friday 8:30 am to 2 pm and 5 pm to 8 pm. Saturday 8:30 am to 2:30 pm.")),
                     [inicio, productos, encargos, cont])
