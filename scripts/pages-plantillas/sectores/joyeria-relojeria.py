# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
from lib import *

KEY = "joyeria-relojeria"

# Marfil y verde botella con dorado antiguo. Pasa AA y primary/inverse >= 3, así que admite fondo oscuro.
ESMERALDA = dict(
    colores=dict(background="#faf8f2", surface="#f0ebdf", text="#1a1f1c", muted="#5a5a4e", primary="#8a6414",
                 on_primary="#ffffff", border="#ded6c2", inverse="#10231d", on_inverse="#f6f1e4"),
    fuentes=("playfair-display", "work-sans"), radio=(2, 4, 8), ancho=1120)

NOTA_PLAZOS = ("Plazos y precios de ejemplo: cámbialos por los tuyos. El precio definitivo te lo damos al ver la pieza.",
               "Example times and prices: replace them with your own. We give you the final price once we have seen the piece.")


def coleccion(nombre, texto_, alt):
    return tarjeta([imagen("product", alt, ratio=1.0), h3(nombre), p(texto_, color="$muted")], estilo="borde", pad=16)


def colecciones(titulo, intro=None, eyebrow=None, tono="base", anchor="colecciones"):
    cs = [
        (("Anillos", "Rings"), ("Solitarios, sellos y anillos de diario.", "Solitaires, signet rings and everyday rings."),
         ("Anillos de oro y plata sobre un paño", "Gold and silver rings on a cloth")),
        (("Alianzas", "Wedding bands"), ("Para boda y aniversario, lisas o con detalle.", "For weddings and anniversaries, plain or detailed."),
         ("Par de alianzas de boda", "Pair of wedding bands")),
        (("Pendientes", "Earrings"), ("De botón, de aro y largos, para uso diario o de fiesta.", "Studs, hoops and drops, for every day or for parties."),
         ("Pendientes de aro y de botón", "Hoop and stud earrings")),
        (("Collares y colgantes", "Necklaces and pendants"), ("Cadenas, medallas y colgantes con o sin piedra.", "Chains, medals and pendants with or without a stone."),
         ("Colgante con cadena fina", "Pendant on a fine chain")),
        (("Pulseras", "Bracelets"), ("Rígidas, de cadena y de cuero, para ellas y para ellos.", "Bangles, chain and leather, for women and for men."),
         ("Pulsera de cadena sobre el mostrador", "Chain bracelet on the counter")),
        (("Relojes", "Watches"), ("De pulsera, de pila y automáticos, con correa o brazalete.", "Wristwatches, quartz and automatic, with strap or bracelet."),
         ("Reloj de pulsera sobre una vitrina", "Wristwatch on a display case")),
    ]
    return seccion("gallery", [titular(titulo, intro, eyebrow),
                               rejilla([coleccion(n, t, a) for n, t, a in cs], cols=3, gap=20, movil=1)],
                   tono=tono, anchor=anchor)


def build():
    nueva(KEY)
    look = ESMERALDA
    nav = [("inicio", ("Inicio", "Home")), ("colecciones", ("Colecciones", "Collections")),
           ("taller", ("Taller y tasación", "Workshop and valuation")), ("contacto", ("Cita en tienda", "Visit us"))]
    horas = [(("Lunes a viernes", "Monday to Friday"), ("10:00 a 13:30 y 17:00 a 20:00", "10:00 am to 1:30 pm and 5:00 pm to 8:00 pm")),
             (("Sábado", "Saturday"), ("10:00 a 14:00", "10:00 am to 2:00 pm")),
             (("Domingo", "Sunday"), ("Cerrado", "Closed"))]
    cta_cab = marco([llamar(t=("Llamar", "Call")), whatsapp("ghost", ("WhatsApp", "WhatsApp"))],
                    dir="row", gap=8, align="center")
    def cita():
        return boton(("Pedir cita en tienda", "Book a visit"), a_pagina("contacto", "contacto"))

    inicio = pagina("inicio", ("", ""), ("Joyería y relojería", "Jewellery and watches"),
                    ("Joyería y relojería con taller propio: colecciones, alianzas de boda, reparaciones, compra de oro y cita en tienda.",
                     "Jewellery and watch shop with its own workshop: collections, wedding bands, repairs, gold buying and in-store appointments."), [
        hero(("Joyas y relojes, y el taller que los cuida", "Jewellery and watches, and the workshop that looks after them"),
             ("Ven a verlas, a probártelas o a traernos la que ya tienes: la arreglamos, la limpiamos o te decimos qué vale.",
              "Come to see them, try them on or bring us the one you already own: we repair it, clean it or tell you what it is worth."),
             [cita(), llamar("secondary"), whatsapp("ghost")],
             eyebrow=("Joyería, relojería y taller", "Jewellery, watches and workshop"), variante="centro", tono="relleno"),
        colecciones(("Lo que encontrarás en la tienda", "What you will find in the shop"),
                    ("Las piezas se ven mejor en la mano. Estas son las familias; pregúntanos por modelos y medidas.",
                     "Pieces look best in the hand. These are the families; ask us about models and sizes."),
                    eyebrow=("Colecciones", "Collections")),
        bloque_texto(("Un taller dentro de la tienda", "A workshop inside the shop"),
                     [("Arreglamos en el propio taller: cierres, cadenas, tallas de anillo, piedras sueltas, pilas y correas. Lo que se puede hacer en el momento, se hace en el momento.",
                       "We repair in our own workshop: clasps, chains, ring sizes, loose stones, batteries and straps. What can be done on the spot is done on the spot."),
                      ("Cuando una pieza se queda, te damos un resguardo con su descripción y una foto, y un plazo para recogerla.",
                       "When a piece stays with us, we give you a receipt with its description and a photo, and a date to collect it.")],
                     eyebrow=("Taller", "Workshop"), tono="oscuro", invertir=True,
                     imagen_=("photo", ("Banco de trabajo del taller con lupa y herramientas de joyería", "Workshop bench with a loupe and jewellery tools")),
                     anchor="taller-propio"),
        bloque_valores(("Tres cosas que puedes hacer aquí", "Three things you can do here"), [
            (("Elegir una alianza", "Choose a wedding band"),
             ("Te enseñamos metales, anchos y perfiles, y te medimos el dedo sin compromiso.",
              "We show you metals, widths and profiles, and measure your finger with no obligation.")),
            (("Arreglar lo que tienes", "Repair what you own"),
             ("Una cadena rota o un reloj parado casi siempre tienen arreglo. Te decimos el precio antes de empezar.",
              "A broken chain or a stopped watch can almost always be fixed. We tell you the price before we start.")),
            (("Saber qué vale", "Find out what it is worth"),
             ("Traes la pieza, la miramos delante de ti y te contamos qué es y qué opciones tienes.",
              "You bring the piece, we look at it in front of you and tell you what it is and what your options are.")),
        ], tono="base", estilo="linea", anchor="que-hacer"),
        bloque_opiniones(("Lo que dicen nuestros clientes", "What our customers say"),
                         [("una alianza", "a wedding band"), ("una reparación", "a repair"), ("el trato en la tienda", "the service in the shop")]),
        bloque_cta(("¿Pasas por la tienda?", "Will you drop by the shop?"),
                   ("Dinos qué buscas y qué día te viene bien y te esperamos con las piezas preparadas.",
                    "Tell us what you are looking for and which day suits you and we will have the pieces ready."),
                   [cita(), whatsapp("ghost")], tono="relleno"),
    ])

    colec = pagina("colecciones", ("colecciones", "collections"), ("Colecciones", "Collections"),
                   ("Colecciones de anillos, alianzas, pendientes, colgantes, pulseras y relojes, y cómo elegir una alianza de boda en cuatro pasos.",
                    "Collections of rings, wedding bands, earrings, pendants, bracelets and watches, and how to choose a wedding band in four steps."), [
        hero(("Colecciones", "Collections"),
             ("No publicamos precios porque cada pieza depende del metal, el peso y el trabajo. Pide cita o escríbenos y te los damos con la pieza delante.",
              "We do not publish prices because each piece depends on the metal, the weight and the work. Book a visit or message us and we give them to you with the piece in front of you."),
             [cita(), whatsapp("ghost")], variante="texto"),
        colecciones(("Qué tenemos", "What we have"), None, anchor="familias"),
        bloque_pasos(("Cómo elegir tus alianzas de boda", "How to choose your wedding bands"), [
            (("Mide el dedo", "Measure your finger"),
             ("Mejor por la tarde y con el dedo templado: por la mañana o con frío está más fino. En la tienda lo medimos con anillos de prueba.",
              "Best in the afternoon with a warm finger: in the morning or in the cold it is thinner. In the shop we measure with sizing rings.")),
            (("Elige metal y color", "Pick metal and colour"),
             ("Oro amarillo, blanco o rosa. Si tenéis alergia a algún metal, decídnoslo antes de elegir.",
              "Yellow, white or rose gold. If either of you has a metal allergy, tell us before choosing.")),
            (("Decide ancho y perfil", "Decide width and profile"),
             ("Un ancho de 3 a 4 mm es el más habitual. El perfil, plano o curvo, cambia cómo se siente al llevarla todo el día.",
              "A width of 3 to 4 mm is the most common. The profile, flat or curved, changes how it feels to wear all day.")),
            (("Grabado y plazo", "Engraving and timing"),
             ("Fecha, nombres o una frase corta. Si se hace a medida, cuenta con unas 3 semanas (plazo de ejemplo: cámbialo por el tuyo).",
              "Date, names or a short phrase. If made to measure, allow about 3 weeks (example time: replace it with your own).")),
        ], intro=("Con tiempo, mejor: pensadlo con dos o tres meses de margen sobre la boda.",
                  "The earlier the better: allow two or three months before the wedding."),
            eyebrow=("Alianzas", "Wedding bands"), tono="relleno", anchor="alianzas"),
        bloque_checks(("Al comprar una pieza nueva", "When you buy a new piece"), [
            ("Factura y garantía legal.", "Invoice and legal guarantee."),
            ("Estuche de regalo, con tarjeta si quieres.", "Gift box, with a card if you like."),
            ("Ajuste de talla de anillo y de largo de cadena.", "Ring resizing and chain shortening."),
            ("Te explicamos cómo cuidarla y guardarla en casa.", "We explain how to look after it and store it at home.")],
            tono="base", anchor="al-comprar"),
        bloque_cta(("¿Quieres ver alguna en persona?", "Would you like to see one in person?"), None, [cita(), llamar("ghost")],
                   tono="relleno"),
    ])

    taller = pagina("taller", ("taller", "workshop"), ("Taller y tasación", "Workshop and valuation"),
                    ("Reparación de joyas y relojes con plazos, compra de oro y tasación de piezas, con presupuesto antes de empezar.",
                     "Jewellery and watch repairs with turnaround times, gold buying and valuation, with a quote before we start."), [
        hero(("Taller y tasación", "Workshop and valuation"),
             ("Cuánto tarda cada arreglo, cómo funciona la compra de oro y qué hacemos cuando traes una pieza para saber qué vale.",
              "How long each repair takes, how gold buying works and what we do when you bring a piece to find out what it is worth."),
             [boton(("Ver arreglos y plazos", "See repairs and times"), a_ancla("reparaciones"), "secondary"), whatsapp("ghost")],
             variante="texto"),
        bloque_precios(("Arreglos y plazos", "Repairs and times"), ("Arreglos de joyas y relojes, con plazo orientativo", "Jewellery and watch repairs, with approximate times"),
                       [("Arreglo", "Repair"), ("Plazo", "Time"), ("Precio", "Price")],
                       [(("Cambio de pila", "Battery change"), ("En el momento", "While you wait"), desde(8)),
                        (("Cambio de correa", "Strap change"), ("En el momento", "While you wait"), desde(10)),
                        (("Limpieza y pulido de joyas", "Cleaning and polishing"), ("24 a 48 horas", "24 to 48 hours"), desde(10)),
                        (("Grabado", "Engraving"), ("2 a 3 días", "2 to 3 days"), desde(12)),
                        (("Ajuste de talla de anillo", "Ring resizing"), ("3 a 5 días", "3 to 5 days"), ("Presupuesto sin compromiso", "Free quote")),
                        (("Arreglo de cierre o cadena rota", "Clasp or broken chain repair"), ("2 a 5 días", "2 to 5 days"), ("Presupuesto sin compromiso", "Free quote")),
                        (("Piedra suelta o engaste", "Loose stone or setting"), ("Hasta 1 semana", "Up to 1 week"), ("Presupuesto sin compromiso", "Free quote")),
                        (("Revisión de reloj mecánico", "Mechanical watch service"), ("2 a 4 semanas", "2 to 4 weeks"), ("Presupuesto sin compromiso", "Free quote"))],
                       intro=("Antes de tocar nada te decimos qué tiene la pieza, qué hay que hacer y cuánto cuesta. Si no te convence, te la devolvemos tal cual.",
                              "Before touching anything we tell you what is wrong with the piece, what needs doing and what it costs. If you are not convinced, we give it back as it was."),
                       nota=NOTA_PLAZOS, anchor="reparaciones", tono="base"),
        bloque_pasos(("Cómo dejas una pieza", "How to leave a piece"), [
            (("La miramos contigo", "We look at it with you"),
             ("La revisamos en el mostrador y te explicamos qué ha pasado.", "We check it at the counter and explain what has happened.")),
            (("Resguardo con foto", "Receipt with a photo"),
             ("Anotamos cómo es, la fotografiamos y te damos un resguardo con la fecha de recogida.",
              "We note what it looks like, photograph it and give you a receipt with the collection date.")),
            (("Presupuesto", "Quote"),
             ("Te llamamos con el precio antes de empezar. Sin tu visto bueno, no se hace nada.",
              "We call you with the price before we start. Nothing is done without your say-so.")),
            (("Recogida", "Collection"),
             ("Te avisamos cuando está lista. Se paga al recogerla y se entrega con factura.",
              "We let you know when it is ready. You pay on collection and receive an invoice.")),
        ], tono="relleno", anchor="como-dejarla"),
        bloque_texto(("Compra de oro", "Gold buying"),
                     [("Si tienes joyas rotas, solas o que ya no usas, podemos valorarlas. Pesamos la pieza y comprobamos el quilataje delante de ti.",
                       "If you have broken, odd or unused jewellery, we can value it. We weigh the piece and check the carat in front of you."),
                      ("El precio depende del peso, de la pureza y del precio del oro ese día. Te damos la cifra por escrito y no hay ningún compromiso de venta: puedes pensártelo y volver.",
                       "The price depends on weight, purity and the price of gold that day. We give you the figure in writing and there is no obligation to sell: you can think it over and come back."),
                      ("La normativa nos obliga a registrar estas operaciones, así que tendrás que traer tu DNI.",
                       "The regulations require us to record these transactions, so you will need to bring your ID.")],
                     eyebrow=("Oro y plata", "Gold and silver"),
                     imagen_=("photo", ("Báscula de precisión con una cadena de oro para pesarla", "Precision scale weighing a gold chain")),
                     tono="base", anchor="oro"),
        bloque_valores(("Tasación de una pieza", "Valuing a piece"), [
            (("Qué miramos", "What we look at"),
             ("Metal, peso, piedras, marcas del fabricante y estado.", "Metal, weight, stones, maker's marks and condition.")),
            (("Qué te contamos", "What we tell you"),
             ("De qué está hecha, cuánto vale aproximadamente y si merece la pena arreglarla, venderla o guardarla.",
              "What it is made of, roughly what it is worth and whether it is worth repairing, selling or keeping.")),
            (("Para un seguro o una herencia", "For insurance or an inheritance"),
             ("Si necesitas un documento de valoración, consúltanos antes: depende de lo que te pida quien lo reciba.",
              "If you need a valuation document, ask us first: it depends on what whoever receives it asks for.")),
        ], tono="relleno", estilo="borde", anchor="tasacion"),
        bloque_faq(("Preguntas frecuentes", "Frequently asked questions"), [
            (("¿Puedo traer una joya de la familia aunque esté muy estropeada?", "Can I bring a family piece even if it is badly damaged?"),
             ("Sí. Casi todo se puede reparar; si no merece la pena, te lo decimos con franqueza antes de cobrarte nada.",
              "Yes. Almost anything can be repaired; if it is not worth it, we tell you frankly before charging you anything.")),
            (("¿Qué pasa si no recojo la pieza en la fecha?", "What if I do not collect the piece on the date?"),
             ("Te llamamos y la guardamos. Dinos si necesitas más tiempo.", "We call you and keep it safe. Tell us if you need more time.")),
            (("¿Arregláis relojes de cualquier marca?", "Do you repair watches of any brand?"),
             ("Pilas, correas y cristales, casi todas. Si es una revisión del mecanismo, miramos primero qué reloj es y te decimos si podemos o a quién llevarlo.",
              "Batteries, straps and glasses, almost all. For a movement service, we first look at which watch it is and tell you whether we can do it or who to take it to.")),
            (("¿Se puede grabar en una joya que no es de la tienda?", "Can you engrave a piece that is not from your shop?"),
             ("Depende de la pieza y del metal. Tráela y la miramos.", "It depends on the piece and the metal. Bring it and we will take a look.")),
        ], tono="base"),
        bloque_cta(("¿Traes algo para arreglar o valorar?", "Bringing something to repair or value?"), None, [cita(), whatsapp("ghost")]),
    ])

    cont = pagina("contacto", ("contacto", "contact"), ("Cita en tienda", "Visit us"),
                  ("Pide cita en la tienda para ver alianzas, dejar un arreglo o valorar una pieza. Horario, teléfono, WhatsApp y mapa.",
                   "Book a visit to the shop to see wedding bands, leave a repair or have a piece valued. Hours, phone, WhatsApp and map."), [
        hero(("Cita en tienda", "Visit us"),
             ("No hace falta cita para pasar, pero con ella te atendemos sin prisa y con las piezas preparadas. Para alianzas, mejor reservar.",
              "You do not need an appointment to drop in, but with one we serve you unhurried and with the pieces ready. For wedding bands, booking is best."),
             [llamar(), whatsapp()], variante="texto"),
        bloque_contacto(("Dónde estamos", "Where we are"),
                        ("Cuéntanos qué te trae y te proponemos día y hora.", "Tell us what brings you and we will suggest a day and time."),
                        campos_cita(servicios=[("Ver alianzas", "See wedding bands"), ("Ver joyas o relojes", "See jewellery or watches"),
                                               ("Dejar un arreglo", "Leave a repair"), ("Tasación o compra de oro", "Valuation or gold buying")]),
                        ("Pedir cita", "Request a visit"), horas,
                        exito=("Gracias. Te confirmamos la cita por teléfono o WhatsApp en cuanto la tengamos.",
                               "Thank you. We will confirm your appointment by phone or WhatsApp as soon as we have it.")),
        bloque_mapa(tono="relleno"),
    ])

    return plantilla(KEY, ("Joyería y relojería", "Jewellery and watches"),
                     ("Joyería y relojería con colecciones sin precios, alianzas de boda, taller de reparación con plazos, compra de oro, tasación y cita en tienda.",
                      "Jewellery and watch shop with price-free collections, wedding bands, a repair workshop with times, gold buying, valuation and in-store appointments."),
                     "comercio", ("joyeria relojeria joyero relojero joyas relojes alianzas anillos oro plata taller reparacion pila correa grabado tasacion compra de oro boda",
                                  "jewellery jeweller jewelry watch watches watchmaker wedding bands rings gold silver repair battery strap engraving valuation sell gold"),
                     look, cabecera(nav, cta_cab),
                     pie(nav, ("Joyería y relojería con taller propio. Alianzas, reparaciones y compra de oro.",
                               "Jewellery and watch shop with its own workshop. Wedding bands, repairs and gold buying."),
                         ("Lunes a viernes 10:00 a 13:30 y 17:00 a 20:00. Sábado 10:00 a 14:00.",
                          "Monday to Friday 10 am to 1:30 pm and 5 pm to 8 pm. Saturday 10 am to 2 pm.")),
                     [inicio, colec, taller, cont])
