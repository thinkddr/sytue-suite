# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
from lib import *

KEY = "tapiceria-restauracion"

# Paleta propia: terciopelo oscuro y rosa de arcilla clara. Fondo oscuro, así que `tono="oscuro"` sí vale.
TERCIOPELO = dict(
    colores=dict(background="#1b1517", surface="#271e21", text="#f6ede9", muted="#c9b8b3", primary="#e6a98f",
                 on_primary="#1b1517", border="#41333a", inverse="#0f0b0c", on_inverse="#f6ede9"),
    fuentes=("noto-serif", "dm-sans"), peso=600, radio=(4, 10, 18), ancho=1100)


def build():
    nueva(KEY)
    look = TERCIOPELO
    nav = [("inicio", ("Inicio", "Home")), ("tapiceria", ("Tapicería y madera", "Upholstery and wood")),
           ("galeria", ("Galería", "Gallery")), ("contacto", ("Presupuesto", "Quote"))]
    horas = [(("Lunes a viernes", "Monday to Friday"), ("De 9:30 a 13:30 y de 16:00 a 19:30", "9:30 am to 1:30 pm and 4:00 pm to 7:30 pm")),
             (("Sábados", "Saturdays"), ("Solo con cita", "By appointment only")),
             (("Recogidas y entregas", "Collections and deliveries"), ("Se acuerdan el día y la hora contigo", "Agreed with you for a day and time"))]
    cta_cab = marco([llamar(t=("Llamar", "Call")), whatsapp("secondary", t=("WhatsApp", "WhatsApp"))],
                    dir="row", gap=8, wrap=True)

    inicio = pagina("inicio", ("", ""), ("Tapicería y restauración de muebles", "Upholstery and furniture restoration"),
                    ("Tapicería y restauración de muebles: sofás, sillas y butacas, madera restaurada, ayuda para elegir tela, recogida y entrega y presupuesto con fotos por WhatsApp.",
                     "Upholstery and furniture restoration: sofas, chairs and armchairs, restored wood, help choosing fabric, collection and delivery and a quote from photos on WhatsApp."), [
        hero(("Tapicería y restauración de muebles", "Upholstery and furniture restoration"),
             ("Tapizamos sofás, sillas y butacas y restauramos muebles de madera. Mándanos fotos por WhatsApp y te decimos cuánto costaría y cuánto tardaríamos.",
              "We upholster sofas, chairs and armchairs and restore wooden furniture. Send us photos on WhatsApp and we will tell you what it would cost and how long it would take."),
             [whatsapp("primary", t=("Mandar fotos por WhatsApp", "Send photos on WhatsApp")), llamar("secondary")], variante="split",
             eyebrow=("Tapicero", "Upholsterer"),
             imagen_=("photo", ("Butaca desmontada en el taller de tapicería con tela nueva al lado", "Armchair stripped down in the upholstery workshop with new fabric beside it"))),
        bloque_valores(("Qué hacemos", "What we do"), [
            (("Sofás", "Sofas"), ("Cambiamos la tela, rellenamos y reparamos la estructura. Un sofá cómodo no siempre hay que tirarlo.", "We change the fabric, refill and repair the frame. A comfortable sofa does not always need throwing out.")),
            (("Sillas y butacas", "Chairs and armchairs"), ("Asientos, respaldos y orejeras. Desde una silla de comedor hasta una butaca de familia.", "Seats, backs and wing chairs. From a dining chair to a family armchair.")),
            (("Restauración de madera", "Wood restoration"), ("Decapamos, reparamos lo roto, tratamos la carcoma si la hay y damos un acabado nuevo a patas, mesas y cómodas.", "We strip, mend what is broken, treat woodworm if there is any and give legs, tables and chests a new finish.")),
            (("Cabeceros y bancos", "Headboards and benches"), ("Tapizados a medida, con la forma y la tela que elijas.", "Upholstered to measure, in the shape and fabric you choose.")),
            (("Muebles de bar y local", "Bar and shop furniture"), ("Taburetes, banquetas y asientos de uso diario, con telas resistentes.", "Stools, banquettes and everyday seating, in hard-wearing fabrics.")),
            (("Arreglos pequeños", "Small repairs"), ("Muelles sueltos, esquinas rotas, cremalleras y fundas.", "Loose springs, torn corners, zips and covers.")),
        ], eyebrow=("Trabajos", "Work"), tono="base", estilo="relleno"),
        bloque_texto(("Elegir la tela, en llano", "Choosing the fabric, in plain words"), [
            ("Si el sofá se usa cada día, busca una tela de trama apretada y con buen roce: aguanta mejor el uso y los lavados. Si hay mascotas, mejor evitar las telas con lazos sueltos, que se enganchan.",
             "If the sofa is used every day, look for a tightly woven fabric with a good rub rating: it stands up better to use and cleaning. If you have pets, avoid fabrics with loose loops, which snag."),
            ("Te enseñamos muestras, las pones al lado de tu sala y decides. Te decimos qué se limpia con facilidad y qué no.",
             "We show you samples, you hold them against your room and decide. We tell you what cleans easily and what does not.")],
            eyebrow=("Telas", "Fabrics"), tono="relleno",
            imagen_=("photo", ("Muestrario de telas de tapicería extendido sobre una mesa", "Upholstery fabric samples spread out on a table"))),
        bloque_pasos(("Cómo lo hacemos", "How we do it"), [
            (("Fotos", "Photos"), ("Nos mandas fotos del mueble por WhatsApp, con su medida aproximada.", "You send photos of the piece on WhatsApp, with its approximate size.")),
            (("Presupuesto", "Quote"), ("Te decimos el precio por escrito y el plazo. Si hace falta verlo, pasamos a tu casa.", "We give you the price in writing and the lead time. If it needs seeing, we come to your home.")),
            (("Recogida", "Collection"), ("Lo recogemos en tu casa el día que quedemos.", "We collect it from your home on the day we agree.")),
            (("Taller", "Workshop"), ("Desmontamos, reparamos, rellenamos y tapizamos.", "We strip, repair, refill and upholster.")),
            (("Entrega", "Delivery"), ("Lo llevamos de vuelta y lo dejamos en su sitio.", "We bring it back and put it in its place.")),
        ], tono="oscuro"),
        bloque_opiniones(("Lo que dicen nuestros clientes", "What our customers say"),
                         [("el acabado del sofá", "the finish on the sofa"), ("el cuidado con el mueble", "the care taken with the furniture"), ("el plazo de entrega", "the delivery time")],
                         tono="base"),
        bloque_cta(("¿Tienes un mueble que merece otra vida?", "Have a piece of furniture that deserves another life?"),
                   ("Mándanos una foto por WhatsApp y te contestamos con una idea de precio.", "Send us a photo on WhatsApp and we will reply with an idea of the price."),
                   [whatsapp("primary"), llamar("ghost")]),
    ])

    tap = pagina("tapiceria", ("tapiceria", "upholstery"), ("Tapicería y madera", "Upholstery and wood"),
                 ("Tipos de tela explicados en llano, plazos orientativos de ejemplo, recogida y entrega, restauración de madera y preguntas frecuentes.",
                  "Types of fabric explained in plain words, example lead times, collection and delivery, wood restoration and frequently asked questions."), [
        hero(("Tapicería y restauración, paso a paso", "Upholstery and restoration, step by step"),
             ("Qué tela elegir, cuánto se tarda más o menos y cómo recogemos y entregamos tu mueble.",
              "What fabric to choose, roughly how long it takes and how we collect and deliver your piece."),
             [whatsapp("primary", t=("Mandar fotos por WhatsApp", "Send photos on WhatsApp")), llamar("secondary")], variante="texto"),
        bloque_valores(("Telas, en llano", "Fabrics, in plain words"), [
            (("Tela de tapicería", "Upholstery fabric"), ("La más usada. Hay muchas texturas y colores, y la hay fácil de limpiar. Es la opción más equilibrada de precio.", "The most common. There are many textures and colours, including easy-clean ones. It is the best-balanced choice on price.")),
            (("Piel", "Leather"), ("Dura mucho y con el uso se vuelve más suave. Cuesta más y pide un poco de cuidado.", "It lasts a long time and gets softer with use. It costs more and needs a little care.")),
            (("Polipiel", "Faux leather"), ("Parece piel, se limpia con un paño y cuesta menos. Con los años puede cuartearse.", "It looks like leather, wipes clean and costs less. Over the years it can crack.")),
            (("Terciopelo", "Velvet"), ("Suave y con brillo. Bonito en butacas y cabeceros; en sofás de uso diario conviene uno de buena calidad.", "Soft with a sheen. Lovely on armchairs and headboards; on everyday sofas a good-quality one is advisable.")),
            (("Lino y algodón", "Linen and cotton"), ("Fresco y natural. Se arruga y se mancha con más facilidad. Mejor con funda lavable.", "Cool and natural. It creases and stains more easily. Better with a washable cover.")),
            (("Microfibra", "Microfibre"), ("Resistente a manchas y a los arañazos de mascotas. Es una buena opción si hay niños o animales.", "Resistant to stains and pet scratches. A good option if you have children or animals.")),
        ], intro=("Si dudas entre dos, te prestamos las muestras para que las compares en casa.", "If you are torn between two, we lend you the samples so you can compare them at home."),
            eyebrow=("Telas", "Fabrics"), cols=3, estilo="borde"),
        bloque_precios(("Plazos orientativos", "Guide lead times"), ("Plazos orientativos por pieza", "Guide lead times by piece"),
                       [("Pieza", "Piece"), ("Plazo aproximado", "Approximate lead time")],
                       [(("Silla de comedor", "Dining chair"), ("1 a 2 semanas", "1 to 2 weeks")),
                        (("Butaca", "Armchair"), ("2 a 3 semanas", "2 to 3 weeks")),
                        (("Sofá de dos plazas", "Two-seater sofa"), ("3 a 4 semanas", "3 to 4 weeks")),
                        (("Sofá de tres plazas o rinconera", "Three-seater or corner sofa"), ("4 a 6 semanas", "4 to 6 weeks")),
                        (("Restauración de una mesa o cómoda", "Restoring a table or chest of drawers"), ("3 a 5 semanas", "3 to 5 weeks"))],
                       nota=("Plazos de ejemplo: cámbialos por los tuyos. El plazo real te lo damos por escrito en el presupuesto.",
                             "Example lead times: change them for your own. We give you the real lead time in writing in the quote."),
                       anchor="plazos", kind="pricing", tono="relleno"),
        bloque_checks(("Recogida y entrega", "Collection and delivery"), [
            ("Recogemos en tu casa el día y la hora que acordemos, y lo cubrimos para el transporte.", "We collect from your home at the day and time we agree, and cover it for transport."),
            ("Si es un sofá grande, miramos antes el portal y el ascensor para saber cómo bajarlo.", "For a big sofa, we check the entrance and lift beforehand to see how to take it down."),
            ("También puedes traerlo tú al taller dentro del horario.", "You can also bring it to the workshop yourself during opening hours."),
            ("Al entregarlo lo revisamos contigo y lo dejamos en su sitio.", "On delivery we go over it with you and put it in its place."),
        ], eyebrow=("Transporte", "Transport"), cols=2, anchor="recogida"),
        bloque_texto(("Restauración de madera", "Wood restoration"), [
            ("Quitamos la pintura o el barniz viejo, reparamos lo roto y tratamos la carcoma si la hay. Después lijamos y damos el acabado que prefieras: barniz, cera, aceite o lacado.",
             "We remove old paint or varnish, mend what is broken and treat woodworm if there is any. Then we sand and apply the finish you prefer: varnish, wax, oil or lacquer."),
            ("Conservamos lo que se pueda y lo que le da carácter al mueble. No siempre hay que dejarlo como nuevo.",
             "We keep what can be kept and what gives the piece its character. It does not always have to look brand new.")],
            eyebrow=("Madera", "Wood"), tono="relleno",
            imagen_=("photo", ("Cómoda de madera a medio restaurar con una parte decapada", "Wooden chest of drawers half restored with part of it stripped")), invertir=True),
        bloque_faq(("Preguntas frecuentes", "Frequently asked questions"), [
            (("¿Merece la pena tapizar un sofá viejo?", "Is it worth reupholstering an old sofa?"),
             ("Si la estructura está sana, casi siempre sí. Te decimos si compensa antes de empezar, y si no, te lo decimos igual.",
              "If the frame is sound, nearly always. We tell you whether it is worthwhile before starting, and if it is not we say so.")),
            (("¿Cómo sé cuánta tela hace falta?", "How do I know how much fabric is needed?"),
             ("No lo calcules tú: lo medimos nosotros. Si la tela la pones tú, te decimos los metros.",
              "Do not work it out yourself: we measure it. If you supply the fabric, we tell you how many metres.")),
            (("¿Puedo traer mi propia tela?", "Can I bring my own fabric?"),
             ("Sí. La revisamos antes: si no es apta para tapicería, te lo decimos.", "Yes. We check it first: if it is not suitable for upholstery, we tell you.")),
            (("¿Cómo se paga?", "How is it paid?"),
             ("Se acuerda en el presupuesto: normalmente una parte al recoger y el resto al entregar.", "It is agreed in the quote: usually a part on collection and the rest on delivery.")),
            (("¿Qué hago con un mueble con carcoma?", "What do I do with a piece with woodworm?"),
             ("Lo ves por los agujeritos y el polvillo. Tiene tratamiento: te lo explicamos al verlo.", "You can tell by tiny holes and fine dust. It can be treated: we explain when we see it.")),
        ], tono="base"),
    ])

    gal = pagina("galeria", ("galeria", "gallery"), ("Galería de trabajos", "Gallery of work"),
                 ("Fotos de sofás, sillas, butacas y muebles de madera restaurados.", "Photos of sofas, chairs, armchairs and restored wooden furniture."), [
        hero(("Galería de trabajos", "Gallery of work"),
             ("Algunos muebles que han pasado por el taller.", "Some pieces of furniture that have been through the workshop."),
             [whatsapp("primary", t=("Mandar fotos por WhatsApp", "Send photos on WhatsApp")), llamar("secondary")], variante="texto"),
        bloque_galeria(("Tapicería", "Upholstery"), [
            ("Sofá de tres plazas tapizado en tela gris", "Three-seater sofa upholstered in grey fabric"),
            ("Butaca orejera con terciopelo verde", "Wing chair in green velvet"),
            ("Sillas de comedor con asientos nuevos", "Dining chairs with new seats"),
            ("Cabecero tapizado para una cama doble", "Upholstered headboard for a double bed"),
            ("Banqueta de entrada tapizada en piel", "Hallway bench upholstered in leather"),
            ("Taburetes de barra con polipiel", "Bar stools in faux leather"),
        ], tipo="photo", cols=3, ratio=1.0, anchor="tapiceria-galeria", tono="base",
            intro=("Fotos de ejemplo: sustitúyelas por las de tus trabajos.", "Example photos: replace them with your own work.")),
        bloque_galeria(("Madera restaurada", "Restored wood"), [
            ("Mesa de comedor con el tablero lijado y barnizado", "Dining table with a sanded and varnished top"),
            ("Cómoda antigua antes de restaurar", "Antique chest of drawers before restoration"),
            ("La misma cómoda ya restaurada", "The same chest of drawers restored"),
            ("Silla de madera con las patas reparadas", "Wooden chair with repaired legs"),
        ], tipo="photo", cols=2, ratio=1.4, anchor="madera-galeria", tono="relleno"),
        bloque_cta(("¿Qué mueble tienes en casa?", "What piece do you have at home?"),
                   ("Mándanos una foto por WhatsApp y te contestamos con una idea de precio.", "Send us a photo on WhatsApp and we will reply with an idea of the price."),
                   [whatsapp("primary"), llamar("ghost")]),
    ])

    cont = pagina("contacto", ("presupuesto", "quote"), ("Presupuesto con fotos", "Quote from photos"),
                  ("Pide presupuesto mandando fotos por WhatsApp o usa el formulario. Horario del taller, recogidas y cómo llegar.",
                   "Ask for a quote by sending photos on WhatsApp or use the form. Workshop hours, collections and how to find us."), [
        hero(("Presupuesto con fotos por WhatsApp", "A quote from photos on WhatsApp"),
             ("Manda una foto del mueble entero y otra del detalle que quieres arreglar, con una medida aproximada. Te contestamos con un precio orientativo y un plazo.",
              "Send a photo of the whole piece and another of the detail you want fixed, with an approximate size. We reply with a guide price and a lead time."),
             [whatsapp("primary", t=("Mandar fotos por WhatsApp", "Send photos on WhatsApp")), llamar("secondary")], variante="texto"),
        bloque_contacto(("Cuéntanos qué mueble es", "Tell us about the piece"),
                        ("Si prefieres el formulario, te respondemos por teléfono. El presupuesto final lo damos al ver el mueble.",
                         "If you prefer the form, we reply by phone. We give the final quote once we see the piece."),
                        campos_cita(servicios=[("Sofá", "Sofa"), ("Sillas o butaca", "Chairs or armchair"), ("Cabecero o banco", "Headboard or bench"),
                                               ("Restauración de madera", "Wood restoration"), ("Otro mueble", "Other piece")],
                                    fecha=False,
                                    extra=[campo("recogida", "select", ("¿Necesitas recogida?", "Do you need collection?"), False,
                                                 opciones=[("Sí, que lo recojáis", "Yes, please collect it"), ("No, lo llevo yo", "No, I will bring it")])]),
                        ("Pedir presupuesto", "Ask for a quote"), horas,
                        exito=("Gracias. Te llamamos para hablar del mueble y del presupuesto.", "Thank you. We will call you to talk about the piece and the quote.")),
        bloque_mapa(("Cómo llegar al taller", "How to get to the workshop"),
                    ("Para dejar o recoger un mueble, dentro del horario o con cita.", "To drop off or collect a piece, during opening hours or by appointment."),
                    tono="relleno"),
    ])

    return plantilla(KEY, ("Tapicería y restauración", "Upholstery and restoration"),
                     ("Cuatro páginas: inicio con ayuda para elegir tela, tapicería y madera con plazos de ejemplo y recogida y entrega, galería de trabajos y presupuesto con fotos por WhatsApp, llamada y formulario.",
                      "Four pages: home with help choosing fabric, upholstery and wood with example lead times and collection and delivery, a gallery of work and a quote from photos on WhatsApp, call and form."),
                     "oficios", ("tapiceria tapicero tapizar sofa sofas sillas butacas restauracion restaurar muebles madera decapado barnizado carcoma telas cabecero recogida entrega presupuesto whatsapp",
                                 "upholstery upholsterer reupholster sofa chairs armchair furniture restoration wood stripping varnish woodworm fabric headboard collection delivery quote whatsapp"),
                     look, cabecera(nav, cta_cab),
                     pie(nav, ("Tapicería y restauración de muebles. Presupuesto con fotos por WhatsApp; recogida y entrega a domicilio.",
                               "Upholstery and furniture restoration. Quotes from photos on WhatsApp; collection and delivery to your home."),
                         ("Lunes a viernes de 9:30 a 13:30 y de 16:00 a 19:30.", "Monday to Friday 9:30 am to 1:30 pm and 4 pm to 7:30 pm.")),
                     [inicio, tap, gal, cont])
