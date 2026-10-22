# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
from lib import *

KEY = "almazara-queseria"

# Paleta propia, oscura y de obrador: verde casi negro y dorado de aceite. Pasa AA (ver comprobar_paleta).
OBRADOR = dict(
    colores=dict(background="#161a13", surface="#20261c", text="#f1efe4", muted="#b7bca9", primary="#cdbf6a",
                 on_primary="#161a13", border="#38402f", inverse="#0c0f09", on_inverse="#f1efe4"),
    fuentes=("source-serif-4", "dm-sans"), peso=600, radio=(2, 6, 10),
)
_malos = comprobar_paleta("obrador", OBRADOR["colores"])
assert not _malos, _malos


def ficha(nombre, uso, texto_):
    """Ficha de producto: nombre, para qué sirve y una frase."""
    return tarjeta([h3(nombre), texto("p", uso, preset="label", transform="uppercase", color="$primary"),
                    p(texto_, color="$muted")], estilo="borde")


def build():
    nueva(KEY)
    look = OBRADOR
    nav = [("inicio", ("Inicio", "Home")), ("productos", ("Aceites y quesos", "Oils and cheeses")),
           ("visitas", ("Visitas y catas", "Visits and tastings")), ("contacto", ("Contacto", "Contact"))]
    horas = [(("Lunes a viernes", "Monday to Friday"),
              ("9:00 a 14:00 y 16:00 a 19:00", "9:00 am to 2:00 pm and 4:00 pm to 7:00 pm")),
             (("Sábado", "Saturday"), ("10:00 a 14:00", "10:00 am to 2:00 pm")),
             (("Domingo", "Sunday"), ("Cerrado, salvo visitas con reserva", "Closed, except booked visits"))]
    cta_cab = marco([llamar(t=("Llamar", "Call")), whatsapp("ghost", ("WhatsApp", "WhatsApp"))],
                    dir="row", gap=8, align="center")
    a_prod = a_pagina("productos", "catalogo")
    a_reserva = a_pagina("visitas", "reserva")

    inicio = pagina("inicio", ("", ""), ("Almazara y quesería", "Olive mill and creamery"),
                    ("Almazara y quesería artesana: aceites y quesos con tienda y carrito, cómo se elaboran, visitas y catas con reserva, envío refrigerado y cajas de regalo.",
                     "Artisan olive mill and creamery: oils and cheeses with a shop and cart, how they are made, visits and tastings by booking, chilled shipping and gift boxes."), [
        hero(("Aceite y queso, hechos aquí", "Oil and cheese, made here"),
             ("Elaboramos aceites y quesos en nuestro obrador. Puedes pedirlos desde casa, probarlos en una cata o venir a ver cómo se hacen.",
              "We make oils and cheeses in our workshop. You can order them from home, try them at a tasting or come and see how they are made."),
             [boton(("Ver aceites y quesos", "See oils and cheeses"), a_prod),
              boton(("Reservar una cata", "Book a tasting"), a_reserva, "secondary"), whatsapp("ghost")],
             eyebrow=("Obrador artesano", "Artisan workshop"), variante="centro", tono="oscuro"),
        bloque_productos(("De nuestro obrador", "From our workshop"),
                         ("Los aceites y los quesos de la última tanda. Las cantidades son limitadas y se agotan.",
                          "Oils and cheeses from the latest batch. Quantities are limited and they sell out."),
                         limite=4, cols=4, anchor="novedades",
                         boton_=boton(("Ver todo el catálogo", "See the whole catalogue"), a_prod, "secondary")),
        bloque_texto(("Se hace despacio y se hace aquí", "Made slowly and made here"),
                     [("El aceite sale de aceituna recién cogida y el queso, de leche del día. Todo lo que ves en la tienda se elabora, se madura y se envasa en este obrador.",
                       "The oil comes from freshly picked olives and the cheese from the day's milk. Everything you see in the shop is made, matured and packed in this workshop."),
                      ("Cuando hay tanda nueva, la tienes en el catálogo en cuanto está envasada.",
                       "When there is a new batch, it is in the catalogue as soon as it is packed.")],
                     eyebrow=("El obrador", "The workshop"),
                     imagen_=("photo", ("Maestro quesero cortando la cuajada en el obrador", "Cheesemaker cutting the curd in the workshop")),
                     tono="relleno", anchor="obrador"),
        bloque_valores(("Qué hacemos", "What we make"), [
            (("Aceites", "Oils"), ("Aceite de oliva virgen extra en botellas y latas, para el día a día y para regalar.",
                                   "Extra virgin olive oil in bottles and tins, for everyday use and for gifts.")),
            (("Quesos", "Cheeses"), ("Desde el queso fresco hasta el curado, según el tiempo que les damos en la cámara.",
                                     "From fresh to well-matured cheese, depending on the time we give them in the cellar.")),
            (("Cajas y regalos", "Boxes and gifts"), ("Cajas de aceites, de quesos o mixtas, con tarjeta si quieres.",
                                                      "Boxes of oils, of cheeses or mixed, with a card if you like.")),
        ], estilo="borde", anchor="que-hacemos"),
        bloque_cta(("Ven a probarlo", "Come and taste it"),
                   ("Las catas y las visitas son con reserva. Te enseñamos el obrador y pruebas aceites y quesos con pan.",
                    "Tastings and visits are by booking. We show you the workshop and you taste oils and cheeses with bread."),
                   [boton(("Reservar visita o cata", "Book a visit or tasting"), a_reserva), llamar("ghost")], tono="relleno"),
        bloque_opiniones(("Lo que dicen quienes nos visitan", "What our visitors say"),
                         [("el aceite", "the oil"), ("el queso", "the cheese"), ("la cata", "the tasting")], tono="base"),
    ])

    cab_aceites = [("Tipo", "Type"), ("Cómo es", "What it is like"), ("Para qué va bien", "What it suits")]
    cab_quesos = [("Curación", "Maturation"), ("Cómo es", "What it is like"), ("Cómo se come", "How to eat it")]
    cab_envio = [("Destino", "Destination"), ("Plazo", "Time"), ("Coste", "Cost")]
    productos = pagina("productos", ("aceites-y-quesos", "oils-and-cheeses"), ("Aceites y quesos", "Oils and cheeses"),
                       ("Catálogo con carrito, fichas de aceites por tipo y de quesos por curación, cómo se elaboran, conservación, envío refrigerado y cajas de regalo.",
                        "Catalogue with a cart, oil guide by type and cheese guide by maturation, how they are made, storage, chilled shipping and gift boxes."), [
        hero(("Aceites y quesos", "Oils and cheeses"),
             ("Pide por tipo de aceite o por curación del queso. Si no sabes cuál elegir, mira las fichas de abajo o escríbenos.",
              "Order by type of oil or by cheese maturation. If you are not sure which to choose, read the guides below or message us."),
             [boton(("Ver el catálogo", "See the catalogue"), a_ancla("catalogo")),
              boton(("Envíos y cajas", "Shipping and boxes"), a_ancla("envios"), "secondary"), whatsapp("ghost")],
             variante="texto"),
        bloque_productos(("Catálogo", "Catalogue"), None, limite=24, cols=4, anchor="catalogo"),
        seccion("features", [
            titular(("Aceites por tipo", "Oils by type"),
                    ("Todos son de oliva virgen extra; cambian la aceituna y el momento de la cosecha, y con eso el sabor.",
                     "They are all extra virgin olive oil; the olive and the time of harvest change, and with them the flavour."),
                    eyebrow=("Fichas", "Guide")),
            rejilla([
                ficha(("Virgen extra de cada día", "Everyday extra virgin"), ("Para cocinar y aliñar", "For cooking and dressing"),
                      ("Equilibrado, afrutado, con un amargor y un picor suaves. Sirve para casi todo.",
                       "Balanced and fruity, with a gentle bitterness and peppery kick. It suits almost everything.")),
                ficha(("Virgen extra de cosecha temprana", "Early-harvest extra virgin"), ("Para tomar en crudo", "To use raw"),
                      ("Más verde e intenso, con más amargor y picor. Mejor sobre tostadas, ensaladas y verduras.",
                       "Greener and more intense, with more bitterness and pepper. Best on toast, salads and vegetables.")),
                ficha(("Virgen extra suave", "Mild extra virgin"), ("Para repostería y salsas", "For baking and sauces"),
                      ("De sabor más dulce y redondo. No tapa el resto de ingredientes.",
                       "Sweeter and rounder in flavour. It does not overpower the other ingredients.")),
            ], cols=3),
            peq(("Fichas de ejemplo: cambia los nombres, las variedades y los formatos por los de tus aceites.",
                 "Example guides: replace the names, varieties and sizes with those of your own oils."), color="$muted"),
        ], tono="relleno", anchor="aceites"),
        bloque_precios(("Quesos por curación", "Cheeses by maturation"), ("Quesos según su curación", "Cheeses by maturation"),
                       cab_quesos,
                       [(("Tierno o fresco", "Fresh"), ("Suave, lácteo y húmedo, de pocos días.", "Mild, milky and moist, a few days old."),
                         ("Con miel, membrillo o en ensalada.", "With honey, quince paste or in a salad.")),
                        (("Semicurado", "Semi-matured"), ("Textura firme y sabor suave a mantequilla y frutos secos.", "Firm texture and a mild flavour of butter and nuts."),
                         ("En tabla, en bocadillo o fundido.", "On a board, in a sandwich or melted.")),
                        (("Curado", "Matured"), ("Más seco y sabroso, con más sal y más intensidad.", "Drier and tastier, with more salt and intensity."),
                         ("En lascas, con pan y un vino tinto.", "In slivers, with bread and a red wine.")),
                        (("Viejo", "Extra-matured"), ("Duro, picante y muy aromático.", "Hard, sharp and very aromatic."),
                         ("Rallado o en trozos pequeños, para acabar la comida.", "Grated or in small pieces, to finish a meal."))],
                       intro=("Cuanto más tiempo en la cámara, más seco, más intenso y más salado. La leche y el tiempo de cada queso están en su etiqueta.",
                              "The longer in the cellar, the drier, more intense and saltier. The milk and time of each cheese are on its label."),
                       nota=("Curaciones de ejemplo: cámbialas por las de tus quesos y añade de qué leche está hecho cada uno. El queso contiene leche.",
                             "Example maturations: replace them with those of your cheeses and add which milk each one is made from. Cheese contains milk."),
                       anchor="quesos", tono="base", kind="pricing"),
        bloque_pasos(("Cómo lo elaboramos", "How we make it"), [
            (("Materia prima", "Raw material"),
             ("La aceituna se recoge y se lleva al molino el mismo día. La leche llega del día.",
              "The olives are picked and taken to the mill the same day. The milk arrives that day.")),
            (("Molienda y cuajo", "Milling and curdling"),
             ("La aceituna se muele y se separa el aceite. A la leche se le añade cuajo y se corta la cuajada.",
              "The olives are milled and the oil is separated. Rennet is added to the milk and the curd is cut.")),
            (("Reposo y maduración", "Resting and maturing"),
             ("El aceite reposa antes de envasarse. Los quesos se prensan, se salan y maduran en cámara.",
              "The oil rests before it is bottled. The cheeses are pressed, salted and matured in the cellar.")),
            (("Envasado", "Packing"),
             ("Se envasa por tandas, con la fecha y el lote en la etiqueta.",
              "It is packed in batches, with the date and batch on the label.")),
        ], tono="relleno", anchor="proceso"),
        bloque_valores(("Cómo conservarlo y cómo comerlo", "How to store it and how to eat it"), [
            (("El aceite, a oscuras", "Oil, in the dark"),
             ("Guárdalo lejos de la luz, del calor y de la cocina, y tápalo bien. Tómalo en los meses siguientes a la cosecha.",
              "Keep it away from light, heat and the cooker, and close it well. Use it within the months after harvest.")),
            (("El queso, sin plástico", "Cheese, without plastic"),
             ("Envuélvelo en papel de queso o de horno en el cajón menos frío de la nevera. Evita el film pegado.",
              "Wrap it in cheese paper or baking paper in the least cold drawer of the fridge. Avoid clingfilm pressed on it.")),
            (("Sácalo antes", "Take it out early"),
             ("Saca el queso de la nevera media hora antes: huele y sabe más.",
              "Take cheese out of the fridge half an hour before: it smells and tastes better.")),
            (("Pan y poco más", "Bread and little else"),
             ("Para notar el aceite, pruébalo con pan sin más. Para el queso, un poco de fruta o de miel.",
              "To notice the oil, try it with plain bread. For cheese, a little fruit or honey.")),
        ], cols=4, estilo="linea", anchor="consejos"),
        bloque_precios(("Envío refrigerado", "Chilled shipping"), ("Envíos por destino", "Shipping by destination"),
                       cab_envio,
                       [(("Península, aceite", "Mainland Spain, oil"), ("24 a 48 horas", "24 to 48 hours"), eur(5)),
                        (("Península, quesos refrigerados", "Mainland Spain, chilled cheese"), ("24 horas", "24 hours"), eur(9)),
                        (("Recogida en el obrador", "Collection at the workshop"), ("Cuando quieras en horario", "Whenever you like during opening hours"), ("Gratis", "Free"))],
                       intro=("Los quesos viajan en caja isotérmica con acumulador de frío. Los enviamos de lunes a miércoles para que no pasen el fin de semana en ruta.",
                              "Cheeses travel in an insulated box with a cold pack. We ship from Monday to Wednesday so they do not spend the weekend in transit."),
                       nota=("Plazos, días de envío y precios de ejemplo: cámbialos por los tuyos. Incluyen IVA.",
                             "Example times, shipping days and prices: replace them with your own. VAT included."),
                       anchor="envios", tono="base"),
        bloque_texto(("Cajas y regalos", "Boxes and gifts"),
                     [("Hacemos cajas de aceites, de quesos o mixtas. Dinos para quién es y cuánto quieres gastar y te proponemos el contenido.",
                       "We make boxes of oils, of cheeses or mixed. Tell us who it is for and how much you want to spend and we will suggest what goes in."),
                      ("Puedes añadir una tarjeta escrita a mano y pedir que se envíe directamente a quien lo recibe.",
                       "You can add a handwritten card and ask for it to be sent directly to the person receiving it.")],
                     eyebrow=("Regalos", "Gifts"),
                     imagen_=("product", ("Caja de regalo con una botella de aceite y quesos", "Gift box with a bottle of oil and cheeses")),
                     tono="relleno", invertir=True, anchor="regalos"),
        bloque_cta(("¿Te ayudamos a elegir?", "Shall we help you choose?"),
                   ("Dinos qué sueles cocinar y qué te gusta y te recomendamos un aceite y un queso.",
                    "Tell us what you usually cook and what you like and we will recommend an oil and a cheese."), tono="base"),
    ])

    form_reserva = formulario(
        [campo("nombre", "text", ("Nombre", "Name"), True, auto="name", maxlen=120),
         campo("telefono", "tel", ("Teléfono", "Phone"), True, auto="tel", maxlen=30),
         campo("correo", "email", ("Correo electrónico", "Email"), False, auto="email", maxlen=200),
         campo("tipo", "select", ("¿Qué quieres reservar?", "What would you like to book?"), True,
               opciones=[("Visita al obrador", "Workshop visit"), ("Visita y cata de aceites", "Visit and oil tasting"),
                         ("Visita y cata de quesos", "Visit and cheese tasting"), ("Visita y cata de aceites y quesos", "Visit and oil and cheese tasting"),
                         ("Grupo o empresa", "Group or company")]),
         campo("fecha", "date", ("Día que te viene bien", "Day that suits you"), True),
         campo("personas", "text", ("Número de personas", "Number of people"), True, maxlen=10),
         campo("mensaje", "textarea", ("¿Alguna alergia, intolerancia o necesidad de acceso adaptado?", "Any allergies, intolerances or need for accessible access?"),
               False, maxlen=1000)],
        ("Pedir reserva", "Request a booking"),
        ("Gracias. Hemos recibido tu petición y te confirmamos el día y la hora por teléfono o por correo.",
         "Thank you. We have received your request and will confirm the day and time by phone or email."))
    visitas = pagina("visitas", ("visitas-y-catas", "visits-and-tastings"), ("Visitas y catas", "Visits and tastings"),
                     ("Cómo es una visita al obrador con cata de aceites y quesos, tipos de visita y precios, formulario de reserva y preguntas frecuentes.",
                      "What a workshop visit with oil and cheese tasting is like, types of visit and prices, a booking form and frequently asked questions."), [
        hero(("Visitas y catas con reserva", "Visits and tastings by booking"),
             ("Te enseñamos dónde y cómo hacemos el aceite y el queso, y luego los pruebas con pan. Siempre con reserva, para atenderte con calma.",
              "We show you where and how we make the oil and cheese, and then you taste them with bread. Always by booking, so we can look after you properly."),
             [boton(("Reservar", "Book"), a_ancla("reserva")), llamar("secondary"), whatsapp("ghost")],
             eyebrow=("Visitas y catas", "Visits and tastings"), variante="texto"),
        bloque_pasos(("Cómo es la visita", "What the visit is like"), [
            (("Llegada", "Arrival"), ("Te recibimos en el obrador y te explicamos qué vas a ver.", "We welcome you at the workshop and explain what you will see.")),
            (("Recorrido", "Tour"), ("Vemos el molino, las cubas y la cámara de maduración, según la época del año.", "We see the mill, the tanks and the maturing cellar, depending on the time of year.")),
            (("Cata", "Tasting"), ("Probamos aceites y quesos con pan, uno detrás de otro, y te contamos qué notar.", "We taste oils and cheeses with bread, one after another, and tell you what to look for.")),
            (("Tienda", "Shop"), ("Si algo te ha gustado, te lo llevas o lo pides para que te lo enviemos.", "If you liked something, you take it home or order it and we send it.")),
        ], tono="relleno", anchor="como-es"),
        bloque_precios(("Qué se puede reservar", "What you can book"), ("Visitas y catas", "Visits and tastings"),
                       [("Visita", "Visit"), ("Dura", "Lasts"), ("Incluye", "Includes"), ("Precio por persona", "Price per person")],
                       [(("Visita al obrador", "Workshop visit"), ("Una hora", "One hour"), ("Recorrido y explicación", "Tour and explanation"), eur(6)),
                        (("Visita y cata de aceites", "Visit and oil tasting"), ("Una hora y media", "An hour and a half"), ("Recorrido y cata de tres aceites con pan", "Tour and tasting of three oils with bread"), eur(12)),
                        (("Visita y cata de quesos", "Visit and cheese tasting"), ("Una hora y media", "An hour and a half"), ("Recorrido y cata de cuatro quesos con pan", "Tour and tasting of four cheeses with bread"), eur(14)),
                        (("Visita y cata de aceites y quesos", "Visit and oil and cheese tasting"), ("Dos horas", "Two hours"), ("Recorrido y cata de aceites y quesos con pan", "Tour and tasting of oils and cheeses with bread"), eur(18))],
                       nota=("Duración, contenido y precios de ejemplo: cámbialos por los tuyos. Incluyen IVA.",
                             "Example duration, content and prices: replace them with your own. VAT included."),
                       anchor="precios", tono="base"),
        seccion("contact", [marco([
            marco([titular(("Reserva tu visita", "Book your visit"),
                           ("Dinos qué día te viene bien y cuántos sois. Te confirmamos por teléfono o por correo; la reserva no es firme hasta que te lo confirmemos.",
                            "Tell us which day suits you and how many of you there are. We confirm by phone or email; the booking is not firm until we confirm it."),
                           eyebrow=("Reserva", "Booking")),
                   marco([llamar(), whatsapp()], dir="row", gap=12, wrap=True)],
                  gap=20, sz={"width": "fill", "height": "hug", "min_width": 0}),
            tarjeta([form_reserva], estilo="relleno", pad=28, tag="div", sz={"width": "fill", "height": "hug", "min_width": 0}),
        ], dir="row", gap=48, align="start",
            bp={"tablet": {"layout": {"gap": 32}}, "mobile": {"layout": {"direction": "column", "gap": 28}}})],
            anchor="reserva", tono="relleno", gap=0),
        bloque_faq(("Preguntas frecuentes", "Frequently asked questions"), [
            (("¿Hace falta reservar?", "Do I need to book?"),
             ("Sí. Hacemos las visitas por grupos pequeños y con hora, así que sin reserva no podemos asegurarte plaza.",
              "Yes. We run visits in small groups at set times, so without a booking we cannot guarantee you a place.")),
            (("¿Se puede ir con niños?", "Can I bring children?"),
             ("Sí. Para ellos hay visita sin cata de aceite y pueden probar el queso con pan si no tienen alergia a la leche.",
              "Yes. For them there is a visit without the oil tasting, and they can try the cheese with bread if they are not allergic to milk.")),
            (("Tengo alergia o intolerancia. ¿Qué hago?", "I have an allergy or intolerance. What should I do?"),
             ("Dínoslo al reservar. El queso lleva leche y el obrador puede tener frutos secos u otros alimentos cerca.",
              "Tell us when you book. Cheese contains milk and the workshop may have nuts or other foods nearby.")),
            (("¿Hay acceso para sillas de ruedas?", "Is there wheelchair access?"),
             ("Cuéntanos tu caso al reservar y te explicamos qué partes de la visita son accesibles.",
              "Tell us your situation when you book and we will explain which parts of the visit are accessible.")),
            (("¿Qué pasa fuera de la campaña del aceite?", "What happens outside the oil harvest season?"),
             ("La molienda suele ser en otoño e invierno. El resto del año vemos el molino parado y probamos los aceites igualmente. Los quesos se hacen todo el año.",
              "Milling usually happens in autumn and winter. The rest of the year we see the mill at rest and still taste the oils. Cheese is made all year round.")),
        ], tono="base", anchor="preguntas"),
    ])

    cont = pagina("contacto", ("contacto", "contact"), ("Contacto y horario", "Contact and opening hours"),
                  ("Teléfono, WhatsApp, horario de la tienda del obrador, cómo llegar y formulario para pedidos y consultas.",
                   "Phone, WhatsApp, opening hours of the workshop shop, how to get here and a form for orders and questions."), [
        hero(("Contacto", "Contact"),
             ("Para un pedido, una caja de regalo o una duda sobre un envío, escríbenos por WhatsApp o rellena el formulario. Para una visita, usa la reserva.",
              "For an order, a gift box or a question about a delivery, message us on WhatsApp or fill in the form. For a visit, use the booking."),
             [llamar(), whatsapp(), boton(("Reservar una visita", "Book a visit"), a_reserva, "ghost")], variante="texto"),
        bloque_contacto(("Pásate por la tienda", "Come by the shop"),
                        ("Vendemos en el propio obrador. Si vienes a recoger un pedido, dinos tu nombre y te lo tenemos preparado.",
                         "We sell at the workshop itself. If you are coming to collect an order, give us your name and we will have it ready."),
                        campos_consulta(servicios=[("Un pedido", "An order"), ("Una caja de regalo", "A gift box"),
                                                   ("Un envío", "A delivery"), ("Una visita o cata", "A visit or tasting"),
                                                   ("Otra cosa", "Something else")]),
                        ("Enviar mensaje", "Send message"), horas,
                        notas=("Nº de registro sanitario (RGSEAA): 00-0000.", "Health registration no. (RGSEAA): 00-0000.")),
        bloque_mapa(("Cómo llegar al obrador", "How to get to the workshop"), tono="relleno"),
    ])

    return plantilla(KEY, ("Almazara y quesería", "Olive mill and creamery"),
                     ("Almazara y quesería artesana con tienda y carrito, fichas de aceites y quesos, proceso de elaboración, visitas y catas con reserva, envío refrigerado y cajas de regalo.",
                      "Artisan olive mill and creamery with a shop and cart, oil and cheese guides, how they are made, visits and tastings by booking, chilled shipping and gift boxes."),
                     "local", ("almazara queseria aceite aceite de oliva virgen extra aove queso quesos artesanos obrador cata de aceite cata de quesos visita almazara molino queseria artesana cajas regalo",
                               "olive mill creamery cheese dairy olive oil extra virgin oil artisan cheese tasting oil tasting cheese tasting mill visit gift boxes"),
                     look, cabecera(nav, cta_cab, tienda=True),
                     pie(nav, ("Aceites y quesos artesanos, elaborados en nuestro obrador.", "Artisan oils and cheeses, made in our workshop."),
                         ("Lunes a viernes 9:00 a 14:00 y 16:00 a 19:00. Sábado 10:00 a 14:00.",
                          "Monday to Friday 9 am to 2 pm and 4 pm to 7 pm. Saturday 10 am to 2 pm.")),
                     [inicio, productos, visitas, cont], tienda=True)
