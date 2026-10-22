# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
from lib import *

KEY = "cerveceria-artesanal"

# Botella de cerveza: verde oscuro de cristal y ámbar de lúpulo. Oscura (primary/inverse > 3, se puede usar $inverse).
BOTELLA = dict(
    colores=dict(background="#0f1f17", surface="#18301f", text="#f3f0e2", muted="#b7c4b2", primary="#f2b640",
                 on_primary="#0f1f17", border="#2b4a36", inverse="#08140d", on_inverse="#f3f0e2"),
    fuentes=("work-sans", "ibm-plex-sans"), peso=800, mayus="uppercase", ls=0.03, h1=(50, 40, 32), radio=(2, 4, 8),
    ancho=1140)
_malos = comprobar_paleta("botella", BOTELLA["colores"])
assert not _malos, _malos

AVISO_18 = ("La venta y el consumo de alcohol son solo para mayores de 18 años.",
            "Alcohol is sold and consumed only by people aged 18 or over.")
NOTA_PRECIOS = ("Precios de ejemplo: cámbialos por los tuyos. Incluyen IVA.",
                "Example prices: replace them with your own. VAT included.")


def sec_form(titulo, intro, campos, envio, exito, anchor, tono="base"):
    return seccion("contact", [titular(titulo, intro),
                               tarjeta([formulario(campos, envio, exito)], estilo="relleno" if tono == "base" else "borde",
                                       pad=28, tag="div", sz=tam(max_width=640))],
                   tono=tono, anchor=anchor, gap=28)


def ficha(estilo, nota, grad, cuerpo):
    return tarjeta([h3(estilo), p(nota, color="$muted"),
                    peq(grad, color="$muted"), peq(cuerpo, color="$muted")], estilo="borde", gap=10)


def build():
    nueva(KEY)
    look = BOTELLA
    nav = [("inicio", ("Inicio", "Home")), ("cervezas", ("Cervezas y tienda", "Beers and shop")),
           ("taproom", ("Taproom y catas", "Taproom and tastings")), ("contacto", ("Contacto", "Contact"))]
    horas = [(("Lunes", "Monday"), ("Cerrado", "Closed")),
             (("Martes a jueves", "Tuesday to Thursday"), ("17:00 a 23:00", "5:00 pm to 11:00 pm")),
             (("Viernes", "Friday"), ("17:00 a 00:30", "5:00 pm to 12:30 am")),
             (("Sábado", "Saturday"), ("12:00 a 00:30", "12:00 pm to 12:30 am")),
             (("Domingo", "Sunday"), ("12:00 a 20:00", "12:00 pm to 8:00 pm"))]
    cta_cab = marco([llamar(t=("Llamar", "Call")), whatsapp("ghost", ("WhatsApp", "WhatsApp"))],
                    dir="row", gap=8, align="center")
    a_cervezas = a_pagina("cervezas", "catalogo")
    a_reserva = a_pagina("taproom", "reservar")

    inicio = pagina("inicio", ("", ""), ("Cervecería artesanal", "Craft brewery"),
                    ("Cervecería artesanal con taproom y tienda con carrito: cervezas, cómo se elaboran, catas y visitas con reserva, envíos y botones de llamar y WhatsApp.",
                     "Craft brewery with a taproom and a shop with a cart: beers, how they are made, tastings and visits by booking, delivery and call and WhatsApp buttons."), [
        hero(("Cerveza hecha aquí, para beber aquí o en casa", "Beer made here, to drink here or at home"),
             ("Elaboramos cerveza en pequeñas tandas. Pásate por el taproom, reserva una cata o pide una caja y te la llevamos a casa.",
              "We brew beer in small batches. Drop by the taproom, book a tasting or order a box and we will send it to your door."),
             [boton(("Ver las cervezas", "See the beers"), a_cervezas), boton(("Reservar una cata", "Book a tasting"), a_reserva, "secondary"),
              whatsapp("ghost")],
             eyebrow=("Cervecería artesanal", "Craft brewery"), variante="centro", tono="relleno"),
        bloque_productos(("Para empezar", "To start with"),
                         ("Una selección de la tienda. Todo el catálogo está en Cervezas y tienda. " + AVISO_18[0],
                          "A selection from the shop. The whole catalogue is on Beers and shop. " + AVISO_18[1]),
                         limite=4, cols=4, anchor="seleccion",
                         boton_=boton(("Ver todo el catálogo", "See the whole catalogue"), a_cervezas, "secondary")),
        bloque_pasos(("Cómo se hace la cerveza", "How the beer is made"), [
            (("Molienda y maceración", "Milling and mashing"),
             ("Se muele la malta y se mezcla con agua caliente. Los azúcares de la malta pasan al agua y salen las sustancias que dan cuerpo y color.",
              "The malt is milled and mixed with hot water. The malt sugars pass into the water, along with what gives body and colour.")),
            (("Cocción y lúpulo", "Boiling and hops"),
             ("El mosto se hierve y se añade el lúpulo: el que va al principio da amargor y el del final, aroma.",
              "The wort is boiled and hops are added: those at the start give bitterness and those at the end give aroma.")),
            (("Fermentación", "Fermentation"),
             ("Se enfría el mosto, se añade la levadura y esta transforma el azúcar en alcohol y gas. Dura de días a semanas según el estilo.",
              "The wort is cooled and yeast is added, turning the sugar into alcohol and gas. It lasts days to weeks depending on the style.")),
            (("Maduración y envasado", "Conditioning and packaging"),
             ("La cerveza reposa para asentarse y aclararse. Después se envasa en botella, lata o barril.",
              "The beer rests to settle and clear. Then it is packaged in bottle, can or keg.")),
        ], tono="relleno", anchor="proceso",
            intro=("Es el mismo camino para todas; lo que cambia son las maltas, el lúpulo y la levadura.",
                   "The path is the same for all of them; what changes are the malts, the hops and the yeast.")),
        bloque_texto(("El taproom", "The taproom"),
                     [("Es donde se bebe la cerveza más fresca: de grifo, a pie de depósito, con algo de picar y sin prisa. Hay mesas dentro y puedes venir en grupo.",
                       "It is where you drink the freshest beer: from the tap, next to the tanks, with something to nibble and no rush. There are tables inside and you can come in a group."),
                      ("Si quieres conocer cómo trabajamos, reserva una visita con cata y te enseñamos la sala de cocción y los depósitos.",
                       "If you want to see how we work, book a visit with a tasting and we will show you the brewhouse and the tanks.")],
                     eyebrow=("Taproom y visitas", "Taproom and visits"), tono="base", anchor="taproom-resumen",
                     imagen_=("photo", ("Grifos de cerveza en la barra del taproom", "Beer taps on the taproom bar"))),
        bloque_opiniones(("Lo que dicen los clientes", "What customers say"),
                         [("la cerveza de grifo", "the draught beer"), ("la cata", "the tasting"), ("el taproom", "the taproom")]),
        bloque_cta(("¿Vienes a probarla?", "Coming to try it?"),
                   ("Reserva una cata o escríbenos y te decimos qué hay en grifo esta semana.",
                    "Book a tasting or message us and we will tell you what is on tap this week."),
                   [boton(("Reservar una cata", "Book a tasting"), a_reserva), llamar("secondary"), whatsapp("ghost")]),
    ])

    cervezas = pagina("cervezas", ("cervezas", "beers"), ("Cervezas y tienda", "Beers and shop"),
                      ("Fichas por estilo, catálogo con carrito, envíos con plazos y preguntas frecuentes sobre guardar la cerveza y el gluten.",
                       "Style notes, catalogue with a cart, delivery with times and frequently asked questions about storing beer and gluten."), [
        hero(("Cervezas y tienda", "Beers and shop"),
             ("Cada cerveza con su estilo y su graduación, para que sepas qué pides. " + AVISO_18[0],
              "Each beer with its style and its strength, so you know what you are ordering. " + AVISO_18[1]),
             [boton(("Ir al catálogo", "Go to the catalogue"), a_ancla("catalogo")), boton(("Envíos", "Delivery"), a_ancla("envios"), "secondary"),
              llamar("ghost")], variante="texto"),
        seccion("features", [
            titular(("Los estilos que elaboramos", "The styles we brew"),
                    ("Los nombres y las graduaciones de abajo son de ejemplo: cámbialos por tus cervezas.",
                     "The names and strengths below are examples: replace them with your own beers.")),
            rejilla([
                ficha(("Rubia", "Lager"), ("Ligera, limpia y fresca. La que entra sola un día de calor.", "Light, clean and fresh. The one that goes down easily on a hot day."),
                      ("Graduación de ejemplo: 4,8 %", "Example strength: 4.8%"), ("Cuerpo ligero, amargor bajo", "Light body, low bitterness")),
                ficha(("Pale ale", "Pale ale"), ("Afrutada, con lúpulo que huele a cítrico y a flores.", "Fruity, with hops that smell of citrus and flowers."),
                      ("Graduación de ejemplo: 5,2 %", "Example strength: 5.2%"), ("Cuerpo medio, amargor medio", "Medium body, medium bitterness")),
                ficha(("IPA", "IPA"), ("Mucho lúpulo: aroma intenso y amargor marcado.", "Plenty of hops: intense aroma and pronounced bitterness."),
                      ("Graduación de ejemplo: 6,5 %", "Example strength: 6.5%"), ("Cuerpo medio, amargor alto", "Medium body, high bitterness")),
                ficha(("De trigo", "Wheat beer"), ("Turbia y suave, con notas de plátano y clavo.", "Cloudy and smooth, with notes of banana and clove."),
                      ("Graduación de ejemplo: 5 %", "Example strength: 5%"), ("Cuerpo medio, amargor bajo", "Medium body, low bitterness")),
                ficha(("Tostada", "Amber ale"), ("Con malta tostada: caramelo, pan y frutos secos.", "With toasted malt: caramel, bread and nuts."),
                      ("Graduación de ejemplo: 5,5 %", "Example strength: 5.5%"), ("Cuerpo medio, amargor bajo", "Medium body, low bitterness")),
                ficha(("Stout", "Stout"), ("Oscura y cremosa, con café y cacao de la malta muy tostada.", "Dark and creamy, with coffee and cocoa from the heavily roasted malt."),
                      ("Graduación de ejemplo: 6 %", "Example strength: 6%"), ("Cuerpo alto, amargor medio", "Full body, medium bitterness")),
            ], cols=3)], tono="relleno", anchor="estilos"),
        bloque_productos(("Catálogo", "Catalogue"), AVISO_18, limite=24, cols=4, anchor="catalogo"),
        bloque_precios(("Envíos", "Delivery"), ("Plazos y precios de envío", "Delivery times and prices"),
                       [("Pedido", "Order"), ("Plazo", "Time"), ("Envío", "Shipping")],
                       [(("Recogida en el taproom", "Collection at the taproom"), ("El mismo día, en horario de apertura", "The same day, during opening hours"), ("Gratis", "Free")),
                        (("Hasta 6 botellas o latas", "Up to 6 bottles or cans"), ("De 24 a 72 horas laborables", "24 to 72 working hours"), eur(6)),
                        (("Hasta 12", "Up to 12"), ("De 24 a 72 horas laborables", "24 to 72 working hours"), eur(8)),
                        (("Caja de 24 o más", "Box of 24 or more"), ("De 24 a 72 horas laborables", "24 to 72 working hours"), eur(10))],
                       intro=("Embalamos las botellas con separadores para que lleguen enteras. El paquete se entrega a una persona mayor de 18 años.",
                              "We pack the bottles with dividers so they arrive in one piece. The parcel is handed to a person aged 18 or over."),
                       nota=("Plazos y precios de ejemplo: cámbialos por los tuyos. Incluyen IVA.",
                             "Example times and prices: replace them with your own. VAT included."),
                       anchor="envios", tono="relleno"),
        bloque_faq(("Preguntas frecuentes", "Frequently asked questions"), [
            (("¿Cómo guardo la cerveza?", "How should I store the beer?"),
             ("En un sitio fresco y oscuro, de pie y sin sol. Las más lupuladas están mejor cuanto más frescas, así que no las guardes meses.",
              "Somewhere cool and dark, standing up and out of the sun. The hoppiest ones are best when fresh, so do not keep them for months.")),
            (("¿Lleva gluten?", "Does it contain gluten?"),
             ("La cerveza se hace con cebada o trigo, así que lleva gluten salvo que la ficha diga lo contrario. Si eres celíaco, pregúntanos antes de comprar.",
              "Beer is made with barley or wheat, so it contains gluten unless the label says otherwise. If you have coeliac disease, ask us before you buy.")),
            (("¿Puedo devolver un pedido?", "Can I return an order?"),
             ("Si llega una botella rota, escríbenos con una foto y la reponemos. Las botellas abiertas no se devuelven.",
              "If a bottle arrives broken, write to us with a photo and we will replace it. Opened bottles cannot be returned.")),
            (("¿Hacéis pedidos para eventos?", "Do you supply events?"),
             ("Sí, con tiempo. Cuéntanos la fecha y cuántos sois y te decimos qué podemos preparar.",
              "Yes, with notice. Tell us the date and how many of you there are and we will tell you what we can prepare.")),
        ], tono="base"),
    ])

    taproom = pagina("taproom", ("taproom", "taproom"), ("Taproom y catas", "Taproom and tastings"),
                     ("Horario del taproom, carta corta, catas y visitas a la fábrica con reserva y formulario.",
                      "Taproom hours, a short menu, tastings and brewery visits by booking, and a booking form."), [
        hero(("Taproom, catas y visitas", "Taproom, tastings and visits"),
             ("Cerveza de grifo, algo de picar y, si quieres, una visita a la sala de cocción con cata. Para las catas, reserva.",
              "Draught beer, something to nibble and, if you like, a visit to the brewhouse with a tasting. For tastings, book ahead."),
             [boton(("Reservar una cata", "Book a tasting"), a_ancla("reservar")), llamar("secondary"), whatsapp("ghost")],
             eyebrow=("Taproom", "Taproom"),
             imagen_=("photo", ("Mesas del taproom junto a los depósitos de fermentación", "Taproom tables next to the fermentation tanks"))),
        seccion("pricing", [titular(("Horario del taproom", "Taproom hours"),
                                    ("La tienda abre en el mismo horario. Los festivos pueden cambiarlo: lo avisamos en la barra.",
                                     "The shop opens at the same times. Holidays may change them: we put a notice on the bar.")),
                            horario(horas, ("Horario del taproom", "Taproom opening hours"))],
                tono="relleno", anchor="horario", gap=28),
        bloque_carta(("Carta corta", "Short menu"), [
            (("En grifo", "On tap"), ("Precios de las cervezas de grifo", "Draught beer prices"),
             [("Medida", "Measure"), ("Precio", "Price")],
             [(("Caña (25 cl)", "Small glass (25 cl)"), eur(2.5)), (("Pinta (50 cl)", "Pint (50 cl)"), eur(4.5)),
              (("Cata de cuatro vasos pequeños", "Tasting flight of four small glasses"), eur(8))]),
            (("Para picar", "To nibble"), ("Precios de la comida", "Food prices"),
             [("Plato", "Dish"), ("Precio", "Price")],
             [(("Frutos secos", "Nuts"), eur(3)), (("Tabla de quesos", "Cheese board"), eur(9)),
              (("Bocadillo del día", "Sandwich of the day"), eur(6.5))]),
            (("Sin alcohol", "Alcohol-free"), ("Precios de las bebidas sin alcohol", "Soft drink prices"),
             [("Bebida", "Drink"), ("Precio", "Price")],
             [(("Agua", "Water"), eur(1.5)), (("Refresco", "Soft drink"), eur(2.5))]),
        ], nota=("Precios de ejemplo: cámbialos por los tuyos. Incluyen IVA. Si tienes alguna alergia, díselo a quien te sirva: tenemos por escrito qué lleva cada plato.",
                 "Example prices: replace them with your own. VAT included. If you have any allergy, tell whoever serves you: we have in writing what each dish contains."),
            tono="base", anchor="carta"),
        bloque_pasos(("Cómo es una cata", "What a tasting is like"), [
            (("Recibimos", "Welcome"), ("Te esperamos en la puerta y te enseñamos la sala de cocción.", "We meet you at the door and show you the brewhouse.")),
            (("Recorremos", "Walk round"), ("Pasamos por los depósitos y te contamos cada paso de la elaboración.", "We go past the tanks and explain each step of the brewing.")),
            (("Probamos", "Taste"), ("Cuatro cervezas con algo de picar, de la más ligera a la más intensa.", "Four beers with something to nibble, from the lightest to the strongest.")),
            (("Compras si quieres", "Buy if you like"), ("En la tienda, con las cervezas que más te hayan gustado.", "In the shop, with the beers you liked most.")),
        ], tono="relleno", anchor="visitas",
            intro=("Las visitas son para mayores de 18 años y con reserva. Dura aproximadamente una hora y media.",
                   "Visits are for people aged 18 or over and by booking. It lasts about an hour and a half.")),
        bloque_precios(("Visitas y catas", "Visits and tastings"), ("Tipos de visita y precios", "Types of visit and prices"),
                       [("Visita", "Visit"), ("Qué incluye", "What it includes"), ("Precio por persona", "Price per person")],
                       [(("Cata en el taproom", "Tasting in the taproom"), ("Cuatro cervezas y algo de picar", "Four beers and something to nibble"), eur(12)),
                        (("Visita con cata", "Visit with tasting"), ("Recorrido, cuatro cervezas y algo de picar", "Tour, four beers and something to nibble"), eur(18)),
                        (("Grupo (a partir de 8 personas)", "Group (from 8 people)"), ("Visita con cata, con fecha a convenir", "Visit with tasting, on a date to be agreed"), ("Consulta el precio", "Ask for the price"))],
                       nota=("Precios de ejemplo: cámbialos por los tuyos. Incluyen IVA.", "Example prices: replace them with your own. VAT included."),
                       anchor="precios-visitas", tono="base"),
        sec_form(("Reserva tu cata", "Book your tasting"),
                 ("Dinos el día y cuántos vais. " + AVISO_18[0] + " Te confirmamos por teléfono o WhatsApp.",
                  "Tell us the day and how many of you there are. " + AVISO_18[1] + " We will confirm by phone or WhatsApp."),
                 [campo("nombre", "text", ("Nombre", "Name"), True, auto="name", maxlen=120),
                  campo("telefono", "tel", ("Teléfono", "Phone"), True, auto="tel", maxlen=30),
                  campo("fecha", "date", ("Día que te viene bien", "Day that suits you"), True),
                  campo("tipo", "select", ("Qué quieres reservar", "What you want to book"), False,
                        opciones=[("Cata en el taproom", "Tasting in the taproom"), ("Visita con cata", "Visit with tasting"),
                                  ("Grupo", "Group")]),
                  campo("personas", "select", ("Cuántas personas", "How many people"), False,
                        opciones=[("1 o 2", "1 or 2"), ("De 3 a 5", "3 to 5"), ("De 6 a 10", "6 to 10"), ("Más de 10", "More than 10")]),
                  campo("mensaje", "textarea", ("Mensaje (opcional)", "Message (optional)"), False, maxlen=1000)],
                 ("Reservar", "Book"),
                 ("Gracias. Te confirmamos la reserva por teléfono o WhatsApp en cuanto la tengamos.",
                  "Thank you. We will confirm your booking by phone or WhatsApp as soon as we have it."),
                 "reservar", tono="relleno"),
    ])

    cont = pagina("contacto", ("contacto", "contact"), ("Contacto y cómo llegar", "Contact and directions"),
                  ("Teléfono, WhatsApp, formulario, horario del taproom y mapa.",
                   "Phone, WhatsApp, form, taproom hours and map."), [
        hero(("Contacto", "Contact"),
             ("Llámanos, escríbenos por WhatsApp o pásate por el taproom. Para reservar una cata, mira Taproom y catas.",
              "Call us, message us on WhatsApp or drop by the taproom. To book a tasting, see Taproom and tastings."),
             [llamar(), whatsapp()], variante="texto"),
        bloque_contacto(("Dónde estamos", "Where we are"),
                        ("Para pedidos, eventos o preguntas, lo más ágil es WhatsApp.",
                         "For orders, events or questions, WhatsApp is quickest."),
                        campos_cita(servicios=[("Un pedido", "An order"), ("Un evento", "An event"), ("Una pregunta", "A question"),
                                               ("Otra cosa", "Something else")], fecha=False),
                        ("Enviar mensaje", "Send message"), horas,
                        notas=("Aparcamiento y transporte público cerca. Texto de ejemplo: cámbialo por los datos reales.",
                               "Parking and public transport nearby. Example text: replace it with the real details.")),
        bloque_mapa(tono="relleno"),
    ])

    return plantilla(KEY, ("Cervecería artesanal", "Craft brewery"),
                     ("Cervezas con ficha, proceso de elaboración, tienda con carrito y envíos, taproom con carta corta y catas con reserva, horario y mapa, con botones de llamar y WhatsApp.",
                      "Beers with notes, how they are brewed, shop with a cart and delivery, taproom with a short menu and booked tastings, hours and a map, with call and WhatsApp buttons."),
                     "comida", ("cerveceria artesanal cerveza artesana cervecera taproom birra craft ipa lupulo catas visitas fabrica de cerveza microcerveceria tienda de cerveza",
                                "craft brewery craft beer brewery taproom beer shop ipa hops tasting brewery tour microbrewery"),
                     look, cabecera(nav, cta_cab, tienda=True),
                     pie(nav, ("Cervecería artesanal con taproom y tienda. " + AVISO_18[0],
                               "Craft brewery with a taproom and shop. " + AVISO_18[1]),
                         ("Martes a jueves 17:00 a 23:00. Viernes y sábado hasta las 00:30.",
                          "Tuesday to Thursday 5 pm to 11 pm. Friday and Saturday until 12:30 am.")),
                     [inicio, cervezas, taproom, cont], tienda=True)
