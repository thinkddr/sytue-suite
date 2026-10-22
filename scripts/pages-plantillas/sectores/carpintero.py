# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
from lib import *

KEY = "carpintero"

# Paleta propia: papel de taller y madera de roble. Sin `tono="oscuro"`: primary/inverse no llega a 3.
ROBLE = dict(
    colores=dict(background="#fbf6ec", surface="#efe3cf", text="#2a1d10", muted="#665240", primary="#8a4a12",
                 on_primary="#ffffff", border="#dccbb0", inverse="#2e1f12", on_inverse="#f8eedd"),
    fuentes=("source-serif-4", "work-sans"), peso=600, radio=(2, 6, 10), ancho=1100)


def build():
    nueva(KEY)
    look = ROBLE
    nav = [("inicio", ("Inicio", "Home")), ("encargos", ("Encargos a medida", "Made-to-measure")),
           ("taller", ("Taller y trabajos", "Workshop and work")), ("contacto", ("Contacto", "Contact"))]
    horas = [(("Taller", "Workshop"), ("Lunes a viernes, de 8:00 a 17:00", "Monday to Friday, 8:00 am to 5:00 pm")),
             (("Visitas al taller", "Workshop visits"), ("Solo con cita previa", "By appointment only"))]
    cta_cab = marco([llamar(t=("Llamar", "Call")), whatsapp("secondary", t=("WhatsApp", "WhatsApp"))],
                    dir="row", gap=8, wrap=True)

    inicio = pagina("inicio", ("", ""), ("Carpintería a medida", "Made-to-measure joinery"),
                    ("Carpintería a medida: muebles de cocina, armarios empotrados, puertas y ventanas, restauración y tarimas, con presupuesto a medida y visitas al taller con cita.",
                     "Made-to-measure joinery: kitchen units, fitted wardrobes, doors and windows, restoration and wood flooring, with bespoke quotes and workshop visits by appointment."), [
        hero(("Carpintería a medida", "Made-to-measure joinery"),
             ("Hacemos en el taller y montamos en tu casa muebles de cocina, armarios, puertas y lo que necesites. Cada pieza se hace para tu espacio.",
              "We build in the workshop and fit in your home kitchen units, wardrobes, doors and whatever you need. Every piece is made for your space."),
             [llamar(), whatsapp()], variante="centro",
             eyebrow=("Carpintero", "Carpenter")),
        bloque_valores(("Qué hacemos", "What we make"), [
            (("Muebles de cocina", "Kitchen units"),
             ("Bajos, altos y columnas medidos al milímetro, con tus electrodomésticos y tu forma de cocinar.",
              "Base units, wall units and tall units measured to the millimetre, around your appliances and the way you cook.")),
            (("Armarios empotrados", "Fitted wardrobes"),
             ("Del suelo al techo, con puertas correderas o abatibles, y por dentro como tú lo necesites.",
              "Floor to ceiling, with sliding or hinged doors, and fitted out inside however you need.")),
            (("Puertas y ventanas", "Doors and windows"),
             ("Puertas de paso, de entrada y ventanas de madera, hechas a la medida del hueco.",
              "Interior doors, front doors and timber windows, made to the size of the opening.")),
            (("Restauración", "Restoration"),
             ("Muebles y puertas viejas: decapamos, reparamos lo roto y damos un acabado nuevo.",
              "Old furniture and doors: we strip, mend what is broken and give a new finish.")),
            (("Tarimas y suelos", "Wood flooring"),
             ("Colocación de tarima flotante y de madera maciza, y rodapiés a juego.",
              "Fitting of floating and solid-wood flooring, with matching skirting.")),
            (("Otros encargos", "Other commissions"),
             ("Estanterías, escritorios, bancos y mesas. Si se hace con madera, cuéntanoslo.",
              "Shelving, desks, benches and tables. If it is made of wood, tell us about it.")),
        ], eyebrow=("Trabajos", "Work"), estilo="relleno", tono="base"),
        bloque_texto(("Por qué a medida", "Why made-to-measure"), [
            ("Las paredes casi nunca están a escuadra y los huecos no miden lo que dice el plano. Un mueble hecho para ese hueco aprovecha cada centímetro, sin huecos ni rellenos.",
             "Walls are rarely square and openings rarely match the plan. A piece made for that opening uses every centimetre, with no gaps or fillers."),
            ("Tú eliges el material, el acabado y la distribución por dentro. Te enseñamos muestras antes de empezar.",
             "You choose the material, the finish and the layout inside. We show you samples before we start.")],
            eyebrow=("A medida", "Made-to-measure"), tono="relleno",
            imagen_=("photo", ("Armario empotrado de madera hecho a medida en un dormitorio", "Made-to-measure timber fitted wardrobe in a bedroom"))),
        bloque_pasos(("Cómo se hace un encargo", "How an order works"), [
            (("Visita y medidas", "Visit and measurements"),
             ("Vamos a tu casa, medimos y hablamos de lo que necesitas.", "We come to your home, measure up and talk about what you need.")),
            (("Presupuesto", "Quote"),
             ("Te lo damos por escrito, con material, acabado, plazo y lo que incluye el montaje.",
              "We give it to you in writing, with material, finish, lead time and what the fitting includes.")),
            (("Taller", "Workshop"),
             ("Fabricamos las piezas en el taller. Si quieres, vienes a verlas antes del montaje.",
              "We make the pieces in the workshop. If you like, you come to see them before fitting.")),
            (("Montaje", "Fitting"),
             ("Montamos en tu casa, ajustamos puertas y cajones y nos llevamos los restos.",
              "We fit it in your home, adjust doors and drawers and take away the offcuts.")),
        ]),
        bloque_opiniones(("Lo que dicen nuestros clientes", "What our customers say"),
                         [("el ajuste al hueco", "the fit to the space"), ("el acabado", "the finish"), ("el trato en el montaje", "the care during fitting")]),
        bloque_cta(("¿Tienes un encargo en mente?", "Have a job in mind?"),
                   ("Cuéntanos qué necesitas y vamos a tomar medidas.", "Tell us what you need and we will come to take measurements."),
                   [llamar(), whatsapp("ghost")]),
    ])

    enc = pagina("encargos", ("encargos", "orders"), ("Encargos a medida", "Made-to-measure orders"),
                 ("Cómo se hace un encargo, qué es la madera maciza, el tablero y la chapa, presupuesto a medida y preguntas frecuentes.",
                  "How an order works, what solid wood, board and veneer are, bespoke quotes and frequently asked questions."), [
        hero(("Encargos a medida", "Made-to-measure orders"),
             ("Aquí te explicamos cómo trabajamos, de qué están hechos los muebles y cómo calculamos el precio.",
              "Here we explain how we work, what furniture is made of and how we work out the price."),
             [llamar(), whatsapp()], variante="texto"),
        bloque_pasos(("Un encargo, paso a paso", "An order, step by step"), [
            (("Nos cuentas la idea", "You tell us your idea"),
             ("Por teléfono, WhatsApp o en el taller. Con una foto o un dibujo a mano nos basta.",
              "By phone, WhatsApp or at the workshop. A photo or a hand sketch is enough.")),
            (("Tomamos medidas", "We take measurements"),
             ("Vamos a verlo y medimos el hueco varias veces. Así no hay sorpresas en el montaje.",
              "We come to see it and measure the space several times. That way there are no surprises at fitting.")),
            (("Dibujo y presupuesto", "Drawing and quote"),
             ("Te enseñamos un dibujo con las medidas y el presupuesto. Cambiamos lo que haga falta.",
              "We show you a drawing with the dimensions and the quote. We change whatever is needed.")),
            (("Fabricación", "Making"),
             ("Cortamos, ensamblamos y acabamos las piezas en el taller. Te decimos el plazo al aceptar.",
              "We cut, assemble and finish the pieces in the workshop. We give you the lead time when you accept.")),
            (("Montaje y entrega", "Fitting and handover"),
             ("Lo montamos, ajustamos y lo revisamos contigo antes de irnos.",
              "We fit it, adjust it and go over it with you before we leave.")),
        ]),
        bloque_valores(("Los materiales, en llano", "Materials in plain words"), [
            (("Madera maciza", "Solid wood"),
             ("Es madera entera, sin mezclar. Se puede lijar y reparar, y dura mucho. Cuesta más y se mueve algo con la humedad.",
              "Whole timber with nothing mixed in. It can be sanded and repaired, and lasts a long time. It costs more and moves a little with humidity.")),
            (("Tablero", "Board"),
             ("Madera aglomerada o contrachapada en planchas. Es estable y más barato. Para armarios y cocinas va muy bien.",
              "Particle board or plywood in sheets. It is stable and cheaper. It works very well for wardrobes and kitchens.")),
            (("Chapa de madera", "Wood veneer"),
             ("Una hoja fina de madera real pegada sobre un tablero. Se ve como madera maciza con el precio y la estabilidad del tablero.",
              "A thin sheet of real wood glued on a board. It looks like solid wood with the price and stability of board.")),
            (("Laminado y lacado", "Laminate and lacquer"),
             ("Laminado es una lámina resistente que imita colores o vetas. Lacado es pintura aplicada a mano o a pistola.",
              "Laminate is a tough sheet that imitates colours or grain. Lacquer is paint applied by hand or spray.")),
        ], intro=("Te enseñamos muestras y te decimos qué conviene en cada sitio. En la cocina o el baño importa mucho la humedad.",
                  "We show you samples and tell you what suits each place. In kitchens and bathrooms humidity matters a great deal."),
            eyebrow=("Materiales", "Materials"), cols=2, tono="relleno", estilo="borde"),
        bloque_checks(("Presupuesto a medida", "A bespoke quote"), [
            ("No damos precios cerrados sin medir: cada encargo es distinto.", "We do not give fixed prices without measuring: every order is different."),
            ("El presupuesto es por escrito y sin compromiso.", "The quote is in writing with no obligation."),
            ("Separamos material, fabricación y montaje.", "We list material, making and fitting separately."),
            ("Si cambias algo a mitad, te avisamos de cuánto cambia el precio antes de seguir.", "If you change something halfway, we tell you how the price changes before we go on."),
        ], eyebrow=("Cómo cobramos", "How we charge"), cols=2, anchor="presupuesto"),
        bloque_faq(("Preguntas frecuentes", "Frequently asked questions"), [
            (("¿Cuánto tarda un encargo?", "How long does an order take?"),
             ("Depende del tamaño y de la carga del taller. Tras tomar medidas te damos un plazo por escrito.",
              "It depends on the size and how busy the workshop is. After measuring we give you a lead time in writing.")),
            (("¿Tengo que pagar por adelantado?", "Do I have to pay in advance?"),
             ("Se acuerda en el presupuesto: normalmente una parte al empezar, otra al montar. Queda por escrito.",
              "It is agreed in the quote: usually a part at the start, the rest at fitting. It is put in writing.")),
            (("¿Podéis trabajar con mis planos?", "Can you work from my plans?"),
             ("Sí. Los revisamos y comprobamos las medidas en tu casa antes de cortar nada.",
              "Yes. We review them and check the measurements at your home before cutting anything.")),
            (("¿Dais garantía?", "Do you give a warranty?"),
             ("Sí, por escrito en la factura. Si un cajón o una puerta se descuelga, vamos a ajustarlo.",
              "Yes, in writing on the invoice. If a drawer or door drops, we come to adjust it.")),
        ], tono="relleno"),
    ])

    taller = pagina("taller", ("taller", "workshop"), ("Taller y trabajos", "Workshop and work"),
                    ("Visita el taller con cita previa, galería de trabajos de cocinas, armarios, puertas y restauración.",
                     "Visit the workshop by appointment and see a gallery of kitchens, wardrobes, doors and restoration."), [
        hero(("El taller y nuestros trabajos", "The workshop and our work"),
             ("Puedes venir a ver cómo trabajamos, las muestras de madera y piezas terminadas. Pide cita y te esperamos.",
              "You can come to see how we work, the timber samples and finished pieces. Book an appointment and we will be expecting you."),
             [llamar(), whatsapp()], variante="split",
             imagen_=("photo", ("Banco de carpintero en el taller con herramientas y una pieza en construcción",
                                "Carpenter's bench in the workshop with tools and a piece being built"))),
        bloque_texto(("Visitas al taller con cita", "Workshop visits by appointment"), [
            ("Para ver muestras de madera, acabados y herrajes, o para enseñarnos lo que quieres, vienes al taller. Lo hacemos con cita para dedicarte el tiempo.",
             "To see timber, finish and hardware samples, or to show us what you want, you come to the workshop. We do it by appointment so we can give you our time."),
            ("Llama o escribe por WhatsApp y quedamos. En el taller hay ruido y serrín: ven con calzado cerrado.",
             "Call or message us on WhatsApp and we will arrange it. The workshop is noisy and dusty: wear closed shoes.")],
            eyebrow=("Taller", "Workshop"), tono="relleno", anchor="visitas"),
        bloque_galeria(("Trabajos terminados", "Finished work"), [
            ("Cocina de madera con isla central", "Timber kitchen with a central island"),
            ("Armario empotrado con puertas correderas", "Fitted wardrobe with sliding doors"),
            ("Puerta de entrada de madera maciza", "Solid wood front door"),
            ("Mueble de baño a medida", "Made-to-measure bathroom unit"),
            ("Mesa restaurada con acabado al aceite", "Restored table with an oiled finish"),
            ("Tarima de roble colocada en un salón", "Oak flooring laid in a living room"),
        ], tipo="photo", cols=3, ratio=1.2, anchor="galeria",
            intro=("Fotos de ejemplo: sustitúyelas por las de tus trabajos.", "Example photos: replace them with your own work.")),
        bloque_cta(("Cuéntanos qué necesitas", "Tell us what you need"),
                   ("Una foto o un dibujo a mano nos basta para hablar de presupuesto.", "A photo or a hand sketch is enough for us to talk about a quote."),
                   [llamar(), whatsapp("ghost")]),
    ])

    cont = pagina("contacto", ("contacto", "contact"), ("Contacto y presupuesto", "Contact and quote"),
                  ("Pide presupuesto o cita en el taller: llama, escribe por WhatsApp o usa el formulario. Horario y cómo llegar.",
                   "Ask for a quote or a workshop appointment: call, message us on WhatsApp or use the form. Opening hours and how to find us."), [
        hero(("Pide tu presupuesto a medida", "Ask for your bespoke quote"),
             ("Cuéntanos qué necesitas. Quedamos para tomar medidas o para que visites el taller.",
              "Tell us what you need. We will arrange to take measurements or for you to visit the workshop."),
             [llamar(), whatsapp()], variante="texto"),
        bloque_contacto(("Cuéntanos tu encargo", "Tell us about your order"),
                        ("Te llamamos para quedar. Si tienes una foto o un dibujo, mándalo por WhatsApp.",
                         "We will call you to arrange it. If you have a photo or sketch, send it on WhatsApp."),
                        campos_cita(servicios=[("Muebles de cocina", "Kitchen units"), ("Armario empotrado", "Fitted wardrobe"),
                                               ("Puertas o ventanas", "Doors or windows"), ("Restauración", "Restoration"),
                                               ("Tarima", "Wood flooring"), ("Visita al taller", "Workshop visit"),
                                               ("Otro encargo", "Other order")]),
                        ("Pedir presupuesto", "Ask for a quote"), horas,
                        exito=("Gracias. Te llamamos para quedar y tomar medidas.", "Thank you. We will call you to arrange measurements.")),
        bloque_mapa(("Cómo llegar al taller", "How to get to the workshop"),
                    ("Recuerda que las visitas son con cita previa.", "Remember that visits are by appointment only."), tono="relleno"),
    ])

    return plantilla(KEY, ("Carpintero a medida", "Made-to-measure carpenter"),
                     ("Cuatro páginas: inicio, encargos a medida con proceso, materiales y presupuesto, taller con visitas con cita y galería, y contacto con llamada, WhatsApp y formulario.",
                      "Four pages: home, made-to-measure orders with process, materials and quote, workshop with visits by appointment and gallery, and contact with call, WhatsApp and form."),
                     "oficios", ("carpintero carpinteria madera a medida muebles de cocina armarios empotrados puertas ventanas restauracion tarima parquet taller ebanista",
                                 "carpenter joiner joinery woodwork bespoke made to measure kitchen wardrobe doors windows restoration wood flooring workshop cabinet maker"),
                     look, cabecera(nav, cta_cab),
                     pie(nav, ("Carpintería a medida. Presupuesto por escrito y visitas al taller con cita.",
                               "Made-to-measure joinery. Written quotes and workshop visits by appointment."),
                         ("Taller de lunes a viernes de 8:00 a 17:00.", "Workshop Monday to Friday 8 am to 5 pm.")),
                     [inicio, enc, taller, cont])
