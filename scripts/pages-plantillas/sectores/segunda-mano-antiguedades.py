# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
from lib import *

KEY = "segunda-mano-antiguedades"

# Paleta propia: ciruela muy oscura y verdín (la pátina del bronce) como acento. Fondo oscuro de museo pequeño.
PATINA = dict(
    colores=dict(background="#1f1528", surface="#2a1d36", text="#f2ecf4", muted="#c1b4c9", primary="#8fd3c4",
                 on_primary="#1f1528", border="#43325a", inverse="#150d1c", on_inverse="#f2ecf4"),
    fuentes=("lora", "ibm-plex-sans"), peso=600, radio=(2, 4, 8), ancho=1120)


def build():
    nueva(KEY)
    malos = comprobar_paleta("patina", PATINA["colores"])
    assert not malos, malos
    look = PATINA
    nav = [("inicio", ("Inicio", "Home")), ("vender", ("Vender y tasar", "Sell and valuation")),
           ("restauracion", ("Cuidado y restauración", "Care and restoration")), ("contacto", ("Contacto", "Contact"))]
    horas = [(("Lunes", "Monday"), ("Cerrado", "Closed")),
             (("Martes a viernes", "Tuesday to Friday"), ("10:30 a 14:00 y 17:00 a 20:00", "10:30 am to 2:00 pm and 5:00 pm to 8:00 pm")),
             (("Sábado", "Saturday"), ("10:30 a 14:30", "10:30 am to 2:30 pm")),
             (("Domingo", "Sunday"), ("Cerrado", "Closed"))]
    cta_cab = marco([llamar(t=("Llamar", "Call")), whatsapp("ghost", ("WhatsApp", "WhatsApp"))],
                    dir="row", gap=8, wrap=True)
    a_tasar = a_pagina("vender", "tasacion")

    def categoria(h, t):
        return marco([tarjeta([h3(h), p(t, color="$muted")], estilo="linea", tag="div")], tag="li", gap=0)

    def pieza(h, t, alt):
        return tarjeta([imagen("product", alt, ratio=1.0), h3(h), p(t, color="$muted")], estilo="relleno", gap=12, pad=16)

    inicio = pagina("inicio", ("", ""), ("Segunda mano y antigüedades", "Second-hand and antiques"),
                    ("Tienda de segunda mano y antigüedades: muebles, vajilla, relojes y libros con historia, tasación gratuita con fotos por WhatsApp, restauración y botones de llamar.",
                     "Second-hand and antiques shop: furniture, tableware, clocks and books with a past, free valuation from photos on WhatsApp, restoration and call buttons."), [
        hero(("Objetos con historia, para seguir usándolos", "Objects with a past, made to be used again"),
             ("Muebles, vajillas, relojes, lámparas y libros que ya han pasado por otras casas. Pasa a verlos, compramos lo que ya no usas y te decimos cuánto vale con unas fotos.",
              "Furniture, tableware, clocks, lamps and books that have already been in other homes. Come and see them, we buy what you no longer use and tell you what it is worth from a few photos."),
             [boton(("Tasar una pieza gratis", "Get a free valuation"), a_tasar), llamar("secondary"), whatsapp("ghost")],
             eyebrow=("Segunda mano y antigüedades", "Second-hand and antiques"),
             imagen_=("photo", ("Estantes con vajilla, lámparas y libros antiguos en la tienda", "Shelves of tableware, lamps and old books in the shop"))),
        seccion("features", [
            titular(("Qué encontrarás en la tienda", "What you will find in the shop"),
                    ("Cambia casi cada semana. Si buscas algo en concreto, dínoslo: avisamos cuando llegue.",
                     "It changes almost every week. If you are looking for something in particular, tell us and we will let you know when it arrives."),
                    eyebrow=("La tienda", "The shop")),
            rejilla([categoria(("Muebles", "Furniture"), ("Aparadores, mesas, sillas, cómodas y baúles, de época y de diseño de mediados de siglo.", "Sideboards, tables, chairs, chests of drawers and trunks, antique and mid-century.")),
                     categoria(("Vajilla y cristal", "Tableware and glass"), ("Juegos de café, platos, copas, jarras y fuentes sueltas o por servicios.", "Coffee sets, plates, glasses, jugs and serving dishes, single or in sets.")),
                     categoria(("Relojes y lámparas", "Clocks and lamps"), ("De pared, de mesa y de bolsillo; lámparas de latón, de sobremesa y de pie.", "Wall, table and pocket clocks; brass, table and floor lamps.")),
                     categoria(("Libros y papeles", "Books and paper"), ("Ediciones antiguas, mapas, láminas, postales y revistas de otra época.", "Old editions, maps, prints, postcards and magazines from another era.")),
                     categoria(("Cuadros y espejos", "Pictures and mirrors"), ("Óleos, grabados, marcos antiguos y espejos con su pátina.", "Oil paintings, engravings, antique frames and mirrors with their patina.")),
                     categoria(("Curiosidades y coleccionismo", "Curiosities and collectables"), ("Juguetes, herramientas, monedas, cajas y objetos de colección.", "Toys, tools, coins, boxes and collectable objects."))],
                    cols=3, gap=20)], tono="base", anchor="que-hay"),
        seccion("gallery", [
            titular(("Piezas destacadas", "Featured pieces"),
                    ("Una muestra de lo que suele haber. Cada pieza lleva su época, sus medidas y su estado; el precio, pregúntanoslo.",
                     "A taste of what we usually have. Each piece comes with its period, measurements and condition; ask us for the price."),
                    eyebrow=("Selección", "Selection")),
            rejilla([pieza(("Aparador de roble (pieza de ejemplo)", "Oak sideboard (sample piece)"),
                           ("Época, medidas y estado de conservación de la pieza.", "Period, measurements and condition of the piece."),
                           ("Aparador de roble con puertas de madera", "Oak sideboard with wooden doors")),
                     pieza(("Juego de café de porcelana (pieza de ejemplo)", "Porcelain coffee set (sample piece)"),
                           ("Número de piezas, procedencia y si hay desperfectos.", "Number of pieces, origin and whether there is any damage."),
                           ("Juego de café de porcelana con tazas y cafetera", "Porcelain coffee set with cups and pot")),
                     pieza(("Reloj de pared (pieza de ejemplo)", "Wall clock (sample piece)"),
                           ("Si funciona, si trae péndulo y cuándo se revisó por última vez.", "Whether it works, whether it has a pendulum and when it was last serviced."),
                           ("Reloj de pared de madera con péndulo", "Wooden wall clock with a pendulum")),
                     pieza(("Lámpara de latón (pieza de ejemplo)", "Brass lamp (sample piece)"),
                           ("Estado de la instalación eléctrica y de la pantalla.", "Condition of the wiring and the shade."),
                           ("Lámpara de sobremesa de latón con pantalla", "Brass table lamp with a shade"))],
                   cols=4, gap=20, tablet=2, movil=1)], tono="relleno", anchor="destacadas", gap=32),
        bloque_texto(("Compramos lo que ya no usas", "We buy what you no longer use"),
                     [("Si estás vaciando una casa, ordenando un trastero o has heredado cosas que no sabes qué hacer con ellas, escríbenos. Miramos fotos, vamos a verlo si es una casa entera y te decimos qué nos interesa.",
                       "If you are clearing a house, tidying a storeroom or have inherited things you do not know what to do with, write to us. We look at photos, come and see it if it is a whole house and tell you what interests us."),
                      ("La tasación no cuesta nada y no te obliga a vender.",
                       "The valuation costs nothing and does not oblige you to sell.")],
                     eyebrow=("Vender", "Selling"),
                     imagen_=("photo", ("Mesa con objetos antiguos preparados para valorar", "Table with old objects set out for valuation")),
                     anchor="comprar-inicio", invertir=True),
        bloque_opiniones(("Lo que dicen quienes nos visitan", "What our visitors say"),
                         [("una compra", "a purchase"), ("una tasación", "a valuation"), ("el trato", "the service")], tono="base"),
        bloque_cta(("¿Tienes algo que quieras tasar?", "Have something you would like valued?"),
                   ("Mándanos tres o cuatro fotos por WhatsApp y te contestamos.", "Send us three or four photos on WhatsApp and we will reply."),
                   [whatsapp("primary"), llamar("ghost")], tono="relleno"),
    ])

    vender = pagina("vender", ("vender", "sell"), ("Vender y tasar", "Sell and valuation"),
                    ("Qué compramos y cómo funciona: tasación gratuita con fotos por WhatsApp, visita a domicilio, consignación y preguntas frecuentes.",
                     "What we buy and how it works: free valuation from photos on WhatsApp, home visits, consignment and frequently asked questions."), [
        hero(("Vender y tasar", "Selling and valuation"),
             ("Dinos qué tienes y te decimos si nos interesa y cuánto podría valer. Es gratis y sin compromiso: si no te convence, te lo llevas.",
              "Tell us what you have and we will say whether we are interested and roughly what it could be worth. It is free and without obligation: if you are not convinced, you take it back."),
             [whatsapp("primary"), llamar("secondary"), boton(("Cómo funciona", "How it works"), a_ancla("tasacion"), "ghost")],
             variante="texto"),
        bloque_checks(("Qué compramos", "What we buy"), [
            ("Muebles antiguos y de diseño de mediados de siglo, aunque necesiten arreglo.", "Antique and mid-century furniture, even if it needs repair."),
            ("Vajilla, cristalería, porcelana y platería.", "Tableware, glassware, porcelain and silverware."),
            ("Relojes de pared, de mesa y de bolsillo.", "Wall, table and pocket clocks."),
            ("Lámparas, espejos, cuadros, láminas y marcos.", "Lamps, mirrors, pictures, prints and frames."),
            ("Libros antiguos, mapas, postales y papeles.", "Old books, maps, postcards and papers."),
            ("Juguetes, herramientas, monedas y colecciones.", "Toys, tools, coins and collections.")],
            eyebrow=("Piezas que nos interesan", "Pieces we are interested in"), tono="relleno", cols=2, anchor="que-compramos"),
        bloque_pasos(("Cómo funciona la tasación", "How the valuation works"), [
            (("Haz las fotos", "Take the photos"),
             ("Tres o cuatro por pieza: de frente, un detalle, las marcas o firmas de abajo o de atrás y cualquier golpe o desperfecto. Añade las medidas.",
              "Three or four per piece: from the front, a detail, any marks or signatures underneath or at the back, and any knock or damage. Add the measurements.")),
            (("Envíalas por WhatsApp", "Send them on WhatsApp"),
             ("Con una frase: de dónde viene la pieza y desde cuándo la tienes, si lo sabes.",
              "With a line on where the piece comes from and how long you have had it, if you know.")),
            (("Te respondemos", "We reply"),
             ("Te decimos si nos interesa y una horquilla orientativa. El precio cerrado lo damos al verla en persona.",
              "We tell you whether we are interested and give a rough range. We give a firm price when we see it in person.")),
            (("La traes o vamos a verla", "You bring it or we come to see it"),
             ("Si acordáis el precio, se paga en el momento. Para una casa entera o muebles grandes, vamos nosotros.",
              "If you agree on a price, it is paid on the spot. For a whole house or large furniture, we come to you.")),
        ], tono="base", anchor="tasacion"),
        bloque_checks(("Lo que no compramos y lo que te pedimos", "What we do not buy and what we ask of you"), [
            ("No compramos electrodomésticos, ropa actual ni muebles de serie sin valor.", "We do not buy appliances, current clothing or mass-produced furniture with no value."),
            ("No compramos piezas cuya procedencia no puedas explicar.", "We do not buy pieces whose origin you cannot explain."),
            ("Al comprarte, te pedimos el DNI y anotamos la compra en el libro de registro que la normativa exige a la compraventa de objetos usados.",
             "When we buy from you, we ask for your ID and record the purchase in the register that the rules require for dealing in used goods.")],
            tono="relleno", cols=1, anchor="condiciones"),
        seccion("features", [
            titular(("Consignación: la vendemos por ti", "Consignment: we sell it for you"),
                    ("Si prefieres cobrar el precio entero de venta en lugar de una cantidad cerrada, puedes dejarla en la tienda. La exponemos, la cuidamos y, cuando se vende, te pagamos tu parte. Las condiciones son estas.",
                     "If you would rather receive your share of the full sale price than a fixed amount, you can leave it with us. We display it, look after it and, when it sells, we pay you your share. These are the terms."),
                    eyebrow=("Otra forma de vender", "Another way to sell")),
            rejilla([tarjeta([h3(("Comisión", "Commission")), p(("Escribe aquí el porcentaje que se queda la tienda y qué cubre.", "Write here the percentage the shop keeps and what it covers."), color="$muted")], estilo="borde"),
                     tarjeta([h3(("Plazo de exposición", "Display period")), p(("Escribe aquí cuántos meses se expone la pieza y qué pasa si no se vende.", "Write here how many months the piece is displayed and what happens if it does not sell."), color="$muted")], estilo="borde"),
                     tarjeta([h3(("Liquidación", "Payment")), p(("Escribe aquí cuándo y cómo se paga a la persona que consigna una vez vendida la pieza.", "Write here when and how the person consigning is paid once the piece has sold."), color="$muted")], estilo="borde")],
                    cols=3, gap=20, tablet=1)], tono="base", anchor="consignacion", gap=32),
        bloque_faq(("Preguntas frecuentes", "Frequently asked questions"), [
            (("¿Cuánto cuesta la tasación?", "How much does a valuation cost?"),
             ("Nada. Si solo quieres saber cuánto vale algo para el seguro o un reparto, te lo decimos igual, aunque no vendas. Para una tasación escrita, pregúntanos.",
              "Nothing. If you only want to know what something is worth for insurance or an inheritance, we tell you all the same, even if you do not sell. For a written valuation, ask us.")),
            (("¿Y si no me interesa vuestra oferta?", "What if I do not like your offer?"),
             ("Te llevas la pieza y no pasa nada. Puedes probar con otra tienda o dejarla en consignación.",
              "You take the piece back and that is that. You can try another shop or leave it on consignment.")),
            (("¿Tengo que llevar las piezas a la tienda?", "Do I have to bring the pieces to the shop?"),
             ("Para piezas pequeñas, mándanos fotos y luego las traes. Para muebles grandes o una casa entera, vamos a verlo; dinos la zona.",
              "For small pieces, send us photos and bring them afterwards. For large furniture or a whole house, we come to see it; tell us your area.")),
            (("¿Y si está roto o sucio?", "What if it is broken or dirty?"),
             ("Mándala igual. Muchas piezas valen más con una limpieza suave que con una reforma, y te lo decimos antes de tocar nada.",
              "Send it anyway. Many pieces are worth more with a gentle clean than with a makeover, and we tell you before touching anything.")),
        ], tono="relleno"),
    ])

    resta = pagina("restauracion", ("restauracion", "restoration"), ("Cuidado y restauración", "Care and restoration"),
                   ("Cómo cuidar muebles, vajilla, metales, libros y relojes antiguos, qué restauramos y cómo es un encargo con presupuesto sin compromiso.",
                    "How to look after antique furniture, tableware, metals, books and clocks, what we restore and how a job works, with a no-obligation quote."), [
        hero(("Cuidado y restauración", "Care and restoration"),
             ("Una pieza antigua dura siglos si se cuida con cabeza. Aquí tienes lo básico, y si necesita más, la miramos en el taller.",
              "An antique piece lasts centuries if it is looked after sensibly. Here are the basics, and if it needs more, we look at it in the workshop."),
             [boton(("Pedir presupuesto", "Ask for a quote"), a_pagina("contacto", "contacto")), whatsapp("secondary")], variante="texto"),
        bloque_valores(("Cómo cuidar tus piezas", "How to look after your pieces"), [
            (("Madera", "Wood"),
             ("Lejos del sol directo y de radiadores. Una cera de abejas de vez en cuando; nada de limpiadores con silicona.",
              "Away from direct sun and radiators. Beeswax now and then; no silicone cleaners.")),
            (("Latón, cobre y plata", "Brass, copper and silver"),
             ("Un paño seco y suave. Evita los abrasivos: se llevan la pátina y rayan.",
              "A soft, dry cloth. Avoid abrasives: they remove the patina and scratch.")),
            (("Porcelana y cristal", "Porcelain and glass"),
             ("A mano, con agua tibia y sin lavavajillas. Si tiene dorados o grietas finas, ni remojo largo ni agua caliente.",
              "By hand, with lukewarm water and never in a dishwasher. If it has gilding or hairline cracks, no long soaking or hot water.")),
            (("Libros y papel", "Books and paper"),
             ("En vertical, sin apretar y lejos de la humedad. No arregles páginas con cinta adhesiva.",
              "Upright, not packed tight and away from damp. Never mend pages with sticky tape.")),
            (("Tejidos y cuero", "Textiles and leather"),
             ("Sin lavadora. Airea a la sombra y cepilla en seco. El cuero, con un paño apenas húmedo.",
              "No washing machine. Air in the shade and brush dry. Leather, with a barely damp cloth.")),
            (("Relojes", "Clocks"),
             ("No fuerces la cuerda ni muevas las agujas hacia atrás sin saber cómo. Mejor que lo revise alguien antes de ponerlo en marcha.",
              "Do not force the winding or turn the hands backwards without knowing how. Better to have someone check it before starting it.")),
        ], eyebrow=("Consejos", "Tips"), estilo="borde", tono="relleno", anchor="cuidados"),
        bloque_checks(("Qué restauramos", "What we restore"), [
            ("Muebles: pegar juntas, reponer piezas sueltas, limpiar y encerar.", "Furniture: re-gluing joints, replacing loose parts, cleaning and waxing."),
            ("Lámparas: revisión de la instalación eléctrica y cambio de cables.", "Lamps: checking the wiring and replacing cables."),
            ("Vajilla y cristal: pegado de piezas rotas y limpieza.", "Tableware and glass: gluing broken pieces and cleaning."),
            ("Marcos y espejos: arreglo de molduras y limpieza.", "Frames and mirrors: repairing mouldings and cleaning.")],
            eyebrow=("Trabajos", "Work"), tono="base", cols=2, anchor="que-restauramos"),
        bloque_pasos(("Cómo es un encargo", "How a job works"), [
            (("Tráela o mándanos fotos", "Bring it or send us photos"),
             ("Con la pieza delante vemos mejor; con fotos ya podemos orientarte.",
              "With the piece in front of us we see more; photos are enough for a first idea.")),
            (("Presupuesto sin compromiso", "No-obligation quote"),
             ("Te decimos qué haría falta, cuánto costaría y cuánto tardaría. Si no compensa, también te lo decimos.",
              "We tell you what is needed, what it would cost and how long it would take. If it is not worth it, we say so.")),
            (("La restauramos", "We restore it"),
             ("Conservamos lo original que se pueda conservar. No decapamos ni repintamos una pátina buena.",
              "We keep whatever original we can. We do not strip or repaint a good patina.")),
            (("Te avisamos", "We let you know"),
             ("Cuando está lista, la recoges y pagas lo acordado.", "When it is ready, you collect it and pay what we agreed.")),
        ], tono="relleno", anchor="encargo"),
        bloque_faq(("Preguntas frecuentes", "Frequently asked questions"), [
            (("¿Restaurar una pieza hace que valga menos?", "Does restoring a piece make it worth less?"),
             ("Depende. Una limpieza y un pegado discretos suelen mantener el valor. Repintar o decapar una pieza con pátina original puede restarlo, y por eso te lo explicamos antes.",
              "It depends. A discreet clean and re-gluing usually keeps the value. Repainting or stripping a piece with original patina can lower it, which is why we explain first.")),
            (("¿Restauráis piezas que no habéis vendido vosotros?", "Do you restore pieces you did not sell?"),
             ("Sí. Mándanos una foto y te decimos qué se puede hacer.", "Yes. Send us a photo and we will tell you what can be done.")),
            (("¿Cuánto tarda?", "How long does it take?"),
             ("Un pegado sencillo, unos días; un mueble entero, varias semanas. Te damos un plazo al hacer el presupuesto.",
              "A simple re-gluing, a few days; a whole piece of furniture, several weeks. We give you a time when we quote.")),
        ], tono="base"),
        bloque_cta(("¿Tienes una pieza que arreglar?", "Have a piece that needs repairing?"), None,
                   [llamar(), whatsapp("ghost")], tono="relleno"),
    ])

    cont = pagina("contacto", ("contacto", "contact"), ("Contacto", "Contact"),
                  ("Horario de la tienda, cómo llegar, teléfono, WhatsApp y formulario para tasaciones, compras y restauraciones.",
                   "Shop hours, how to get here, phone, WhatsApp and a form for valuations, purchases and restorations."), [
        hero(("Contacto", "Contact"),
             ("Pasa por la tienda, llámanos o escríbenos por WhatsApp. Para una tasación, lo más rápido son las fotos; si buscas algo concreto, dinos qué.",
              "Come by the shop, call us or message us on WhatsApp. For a valuation the quickest way is photos; if you are after something specific, tell us what."),
             [llamar(), whatsapp()], variante="texto"),
        bloque_contacto(("Estamos en la tienda", "We are in the shop"),
                        ("Si vienes con una pieza grande, avísanos antes y te decimos dónde aparcar para descargar.",
                         "If you are bringing a large piece, let us know first and we will tell you where to park to unload."),
                        campos_consulta(servicios=[("Quiero tasar una pieza", "I want a piece valued"), ("Quiero vender o consignar", "I want to sell or consign"),
                                                   ("Busco una pieza", "I am looking for a piece"), ("Restauración", "Restoration"),
                                                   ("Otra cosa", "Something else")]),
                        ("Enviar consulta", "Send enquiry"), horas),
        bloque_mapa(("Cómo llegar a la tienda", "How to get to the shop"), tono="relleno"),
    ])

    return plantilla(KEY, ("Segunda mano y antigüedades", "Second-hand and antiques"),
                     ("Tienda de segunda mano y antigüedades: lo que hay, piezas destacadas, tasación gratuita con fotos por WhatsApp, consignación, cuidado y restauración, y mapa.",
                      "Second-hand and antiques shop: what is in stock, featured pieces, free valuation from photos on WhatsApp, consignment, care and restoration, and a map."),
                     "comercio", ("segunda mano antiguedades anticuario tienda de antiguedades tienda de segunda mano compraventa objetos usados rastro chamarilero almoneda muebles antiguos vintage tasacion consignacion restauracion",
                                  "second hand shop antiques antique dealer vintage shop curiosities collectables valuation consignment restoration furniture thrift"),
                     look, cabecera(nav, cta_cab),
                     pie(nav, ("Tienda de segunda mano y antigüedades. Compramos, vendemos y restauramos.",
                               "Second-hand and antiques shop. We buy, sell and restore."),
                         ("Martes a viernes 10:30 a 14:00 y 17:00 a 20:00. Sábado 10:30 a 14:30.",
                          "Tuesday to Friday 10:30 am to 2 pm and 5 pm to 8 pm. Saturday 10:30 am to 2:30 pm.")),
                     [inicio, vender, resta, cont])
