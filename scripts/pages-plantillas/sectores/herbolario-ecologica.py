# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
from lib import *

KEY = "herbolario-ecologica"

# Salvia y ciruela: fondo verde grisáceo de hierba seca y un morado de flor de equinácea. Sin fondos oscuros
# (primary/inverse < 3), así que no se usa tono="oscuro" ni tarjetas "oscuro".
SALVIA = dict(
    colores=dict(background="#f1f4ec", surface="#e1e8d8", text="#1f2a22", muted="#506053", primary="#5e3a73",
                 on_primary="#ffffff", border="#ccd8c0", inverse="#2a2036", on_inverse="#f4eff8"),
    fuentes=("source-serif-4", "figtree"), peso=600, radio=(8, 14, 20), ancho=1120)
_malos = comprobar_paleta("salvia-ciruela", SALVIA["colores"])
assert not _malos, _malos

AVISO = ("Las plantas y los suplementos no sustituyen a un médico ni a una dieta variada. Si estás embarazada, das el pecho, tomas medicación o tienes una enfermedad, consúltalo antes con tu médico o tu farmacéutico.",
         "Plants and supplements do not replace a doctor or a varied diet. If you are pregnant, breastfeeding, taking medication or have an illness, check with your doctor or pharmacist first.")


def sec_form(titulo, intro, campos, envio, exito, anchor, tono="base"):
    return seccion("contact", [titular(titulo, intro),
                               tarjeta([formulario(campos, envio, exito)], estilo="relleno" if tono == "base" else "borde",
                                       pad=28, tag="div", sz=tam(max_width=640))],
                   tono=tono, anchor=anchor, gap=28)


def build():
    nueva(KEY)
    look = SALVIA
    nav = [("inicio", ("Inicio", "Home")), ("tienda", ("Secciones y tienda", "Sections and shop")),
           ("consejos", ("Consejos", "Advice")), ("contacto", ("Contacto", "Contact"))]
    horas = [(("Lunes a viernes", "Monday to Friday"), ("9:30 a 14:00 y de 17:00 a 20:00", "9:30 am to 2:00 pm and 5:00 pm to 8:00 pm")),
             (("Sábado", "Saturday"), ("10:00 a 14:00", "10:00 am to 2:00 pm")),
             (("Domingo", "Sunday"), ("Cerrado", "Closed"))]
    cta_cab = marco([llamar(t=("Llamar", "Call")), whatsapp("ghost", ("WhatsApp", "WhatsApp"))],
                    dir="row", gap=8, align="center")
    a_tienda = a_pagina("tienda", "catalogo")
    a_pedir = a_pagina("tienda", "pedir-y-recoger")

    inicio = pagina("inicio", ("", ""), ("Herbolario y tienda ecológica", "Herbalist's and organic shop"),
                    ("Herbolario y tienda ecológica con carrito: infusiones, suplementos, cosmética natural, alimentación ecológica y granel, pedidos con recogida en tienda y botones de llamar y WhatsApp.",
                     "Herbalist's and organic shop with a cart: herbal teas, supplements, natural cosmetics, organic food and loose goods, orders collected in store and call and WhatsApp buttons."), [
        hero(("Plantas, infusiones y alimentación ecológica", "Plants, herbal teas and organic food"),
             ("Una tienda de barrio donde te explican cómo preparar y conservar cada cosa. Compra en la tienda o pide por la web y recoge cuando te venga bien.",
              "A neighbourhood shop where they explain how to prepare and store each item. Buy in store or order on the website and collect when it suits you."),
             [boton(("Ver la tienda", "See the shop"), a_tienda), boton(("Cómo pedir y recoger", "How to order and collect"), a_pedir, "secondary"),
              whatsapp("ghost")],
             eyebrow=("Herbolario y tienda ecológica", "Herbalist's and organic shop"),
             imagen_=("product", ("Frascos de hierbas secas y bolsas de infusión sobre una mesa de madera", "Jars of dried herbs and tea bags on a wooden table"))),
        bloque_productos(("Para empezar", "To start with"),
                         ("Una selección de la tienda. Todas las secciones y el catálogo completo están en Secciones y tienda.",
                          "A selection from the shop. All the sections and the full catalogue are on Sections and shop."),
                         limite=4, cols=4, anchor="seleccion",
                         boton_=boton(("Ver todo el catálogo", "See the whole catalogue"), a_tienda, "secondary")),
        bloque_pasos(("Pide por la web y recoge en la tienda", "Order on the website and collect in store"), [
            (("Elige", "Choose"), ("Añade lo que quieras al carrito. Si dudas, escríbenos y te aconsejamos.", "Add what you want to the cart. If you are unsure, write to us and we will advise you.")),
            (("Preparamos", "We prepare"), ("Reunimos tu pedido con lo que hay en la tienda. Si algo falta, te avisamos antes.", "We put your order together from what is in the shop. If something is missing, we tell you first.")),
            (("Te avisamos", "We let you know"), ("Cuando está listo, te escribimos por WhatsApp o por correo.", "When it is ready, we message you on WhatsApp or by email.")),
            (("Recoges", "You collect"), ("En el horario de la tienda, con tu nombre o el número de pedido.", "During shop hours, with your name or the order number.")),
        ], tono="relleno", anchor="como-pedir"),
        bloque_texto(("Pregunta al herbolarista", "Ask the herbalist"),
                     [("Te explicamos para qué se usa tradicionalmente cada planta, cómo prepararla y cuánto tiempo se conserva. Lo que no hacemos es diagnosticar ni recomendar un tratamiento: eso es cosa de tu médico.",
                       "We explain what each plant is traditionally used for, how to prepare it and how long it keeps. What we do not do is diagnose or recommend a treatment: that is for your doctor."),
                      (AVISO[0], AVISO[1])],
                     eyebrow=("Consejo", "Advice"), tono="base", anchor="consejo-resumen",
                     imagen_=("portrait", ("Herbolarista explicando una planta seca a un cliente en el mostrador", "Herbalist explaining a dried plant to a customer at the counter"))),
        bloque_opiniones(("Lo que dicen los clientes", "What customers say"),
                         [("la atención en la tienda", "the service in the shop"), ("los pedidos con recogida", "the click and collect orders"),
                          ("el granel", "the loose goods")], tono="relleno"),
        bloque_cta(("¿Qué estás buscando?", "What are you looking for?"),
                   ("Escríbenos y te decimos si lo tenemos y cómo preparar lo que lleves.",
                    "Message us and we will tell you whether we have it and how to prepare what you take."),
                   [boton(("Ver la tienda", "See the shop"), a_tienda), llamar("secondary"), whatsapp("ghost")], tono="base"),
    ])

    tienda = pagina("tienda", ("tienda", "shop"), ("Secciones y tienda", "Sections and shop"),
                    ("Las secciones de la tienda, catálogo con carrito, granel, sellos ecológicos y cómo pedir y recoger en tienda.",
                     "The shop sections, catalogue with a cart, loose goods, organic labels and how to order and collect in store."), [
        hero(("Secciones y tienda", "Sections and shop"),
             ("Cinco secciones, un catálogo con carrito y recogida en la tienda. Si no sabes por dónde empezar, escríbenos.",
              "Five sections, a catalogue with a cart and collection in store. If you do not know where to start, write to us."),
             [boton(("Ir al catálogo", "Go to the catalogue"), a_ancla("catalogo")), boton(("Pedir y recoger", "Order and collect"), a_ancla("pedir-y-recoger"), "secondary"),
              llamar("ghost")], variante="texto"),
        bloque_valores(("Las secciones", "The sections"), [
            (("Infusiones", "Herbal teas"),
             ("Hierbas sueltas y mezclas para tomar calientes o frías, con las instrucciones de preparación en cada bolsa.",
              "Loose herbs and blends to drink hot or cold, with preparation instructions on every bag.")),
            (("Suplementos", "Supplements"),
             ("Vitaminas, minerales y otros complementos alimenticios. No sustituyen a una dieta variada: sigue siempre la dosis del envase.",
              "Vitamins, minerals and other food supplements. They do not replace a varied diet: always follow the dose on the pack.")),
            (("Cosmética natural", "Natural cosmetics"),
             ("Jabones, cremas, aceites y desodorantes con ingredientes de origen vegetal. Natural no quiere decir sin alergias: prueba antes en el antebrazo.",
              "Soaps, creams, oils and deodorants made with plant-based ingredients. Natural does not mean allergy-free: test on your forearm first.")),
        ], cols=3, tono="relleno", estilo="borde", anchor="secciones", eyebrow=("Qué encontrarás", "What you will find")),
        bloque_texto(("Alimentación ecológica", "Organic food"),
                     [("Cereales, legumbres, aceites, conservas, bebidas vegetales, chocolates y dulces con certificación ecológica, en la medida en que cada producto la tenga.",
                       "Cereals, pulses, oils, preserves, plant-based drinks, chocolates and sweets with organic certification, as far as each product carries it."),
                      ("Cada envase indica su certificación en la etiqueta. Si tienes una alergia o una intolerancia, léela o pregúntanos antes de comprar.",
                       "Each pack shows its certification on the label. If you have an allergy or an intolerance, read it or ask us before buying.")],
                     eyebrow=("Despensa", "Pantry"), tono="base", anchor="alimentacion",
                     imagen_=("product", ("Bolsas de legumbres y cereales ecológicos en una estantería", "Bags of organic pulses and cereals on a shelf"))),
        bloque_texto(("A granel", "Loose goods"),
                     [("Frutos secos, semillas, cereales, legumbres e infusiones al peso. Compras lo que necesitas y te ahorras envases.",
                       "Nuts, seeds, cereals, pulses and herbal teas by weight. You buy what you need and save on packaging."),
                      ("Puedes traer tu bote o tu bolsa, limpios y secos: los pesamos vacíos y pagas solo por el producto. En el granel pueden convivir productos con gluten, frutos de cáscara y otros alérgenos, y quedar trazas.",
                       "You can bring your own jar or bag, clean and dry: we weigh them empty and you pay only for the product. Loose goods may sit side by side with products containing gluten, tree nuts and other allergens, and traces may remain.")],
                     eyebrow=("Sin envases", "Less packaging"), tono="relleno", anchor="granel", invertir=True,
                     imagen_=("photo", ("Cajones de frutos secos y semillas a granel con una pala", "Bins of nuts and seeds sold loose, with a scoop"))),
        bloque_productos(("Catálogo", "Catalogue"),
                         ("Todo el catálogo con carrito. Los pedidos se recogen en la tienda.",
                          "The whole catalogue with a cart. Orders are collected in store."),
                         limite=24, cols=4, anchor="catalogo"),
        bloque_pasos(("Pedir y recoger en tienda", "Order and collect in store"), [
            (("Haz el pedido", "Place the order"), ("Añade al carrito y confirma. También puedes pedir por WhatsApp o por teléfono.", "Add to the cart and confirm. You can also order by WhatsApp or phone.")),
            (("Espera el aviso", "Wait for our message"), ("Te escribimos cuando esté listo. Si falta algún producto, te lo decimos antes de preparar el resto.", "We write to you when it is ready. If a product is missing, we tell you before preparing the rest.")),
            (("Pasa a recogerlo", "Come and collect it"), ("En el horario de la tienda. Revisamos juntos el pedido en el mostrador.", "During shop hours. We check the order together at the counter.")),
        ], tono="relleno", anchor="pedir-y-recoger",
            intro=("No hacemos envíos a domicilio con el pedido de la web: se recoge en la tienda.",
                   "We do not deliver web orders to your home: they are collected in store.")),
        bloque_valores(("Sellos ecológicos", "Organic labels"), [
            (("Cómo se reconoce un producto ecológico", "How to recognise an organic product"),
             ("El envase lleva el logotipo ecológico europeo y el código del organismo de control. Si falta, no se puede vender como ecológico.",
              "The pack carries the European organic logo and the control body code. If it is missing, it cannot be sold as organic.")),
            (("Nuestra certificación", "Our certification"),
             ("Si tu tienda tiene certificado ecológico, indica aquí el organismo y el número: 00-0000. Si no lo tiene, borra este bloque.",
              "If your shop holds an organic certificate, indicate here the body and the number: 00-0000. If it does not, delete this block.")),
        ], cols=2, tono="base", estilo="relleno", anchor="sellos"),
    ])

    consejos = pagina("consejos", ("consejos", "advice"), ("Consejos del herbolarista", "Advice from the herbalist"),
                      ("Cómo preparar una infusión, cómo conservar las plantas, avisos de prudencia, preguntas frecuentes y quién te atiende.",
                       "How to prepare a herbal tea, how to store plants, words of caution, frequently asked questions and who serves you."), [
        hero(("Consejos del herbolarista", "Advice from the herbalist"),
             ("Cómo preparar, conservar y usar con cabeza lo que compras. Lo importante primero: " + AVISO[0].lower(),
              "How to prepare, store and use sensibly what you buy. The important point first: " + AVISO[1].lower()),
             [llamar(), whatsapp("secondary")], eyebrow=("Consejo", "Advice"), variante="centro", tono="relleno"),
        bloque_pasos(("Cómo preparar una infusión", "How to prepare a herbal tea"), [
            (("Mide", "Measure"), ("Una cucharadita de planta por taza, o lo que indique el envase.", "A teaspoon of plant per cup, or what the pack says.")),
            (("Agua casi hirviendo", "Water just off the boil"), ("Vierte el agua sobre la planta. Para hojas delicadas, deja que baje un poco de temperatura.", "Pour the water over the plant. For delicate leaves, let it cool a little first.")),
            (("Tapa y espera", "Cover and wait"), ("De cinco a diez minutos según la planta. Tapar mantiene el aroma.", "Five to ten minutes depending on the plant. Covering keeps the aroma in.")),
            (("Cuela y bebe", "Strain and drink"), ("Sin prisa y sin azúcar si puedes. La raíz y la corteza suelen pedir hervir unos minutos.", "No rush, and without sugar if you can. Roots and bark usually need a few minutes of boiling.")),
        ], tono="base", anchor="infusion"),
        bloque_valores(("Usar con cabeza", "Use with care"), [
            (("Conservación", "Storage"),
             ("En frasco cerrado, en un sitio seco, fresco y sin luz. Una planta que ha perdido el olor ha perdido casi todo.",
              "In a closed jar, somewhere dry, cool and dark. A plant that has lost its smell has lost most of what it had.")),
            (("Plantas y medicamentos", "Plants and medication"),
             ("Algunas plantas interactúan con medicamentos. Si tomas alguno de forma habitual, pregunta a tu médico o farmacéutico antes de empezar.",
              "Some plants interact with medication. If you take any regularly, ask your doctor or pharmacist before you start.")),
            (("Embarazo, lactancia y niños", "Pregnancy, breastfeeding and children"),
             ("No todo vale en estos casos. Consulta siempre antes con tu médico, y no des plantas ni suplementos a menores sin hacerlo.",
              "Not everything is suitable here. Always check with your doctor first, and do not give plants or supplements to children without doing so.")),
            (("Suplementos", "Supplements"),
             ("Respeta la dosis del envase y no los combines sin consultar. Más no es mejor.",
              "Keep to the dose on the pack and do not combine them without asking. More is not better.")),
            (("Cosmética", "Cosmetics"),
             ("Prueba una gota en el antebrazo y espera un día antes de usarla en la cara. Evita ojos y mucosas.",
              "Try a drop on your forearm and wait a day before using it on your face. Avoid eyes and mucous membranes.")),
            (("Cuándo ir al médico", "When to see a doctor"),
             ("Si un síntoma dura, empeora o te preocupa, pide cita con tu médico. Una infusión no es un diagnóstico.",
              "If a symptom lasts, gets worse or worries you, book an appointment with your doctor. A herbal tea is not a diagnosis.")),
        ], cols=3, tono="relleno", estilo="borde", anchor="prudencia"),
        bloque_equipo(("Quién te atiende", "Who serves you"), [
            (("Herbolarista", "Herbalist"), ("Te explica cómo preparar y conservar cada planta.", "Explains how to prepare and store each plant.")),
            (("Atención y pedidos", "Service and orders"), ("Prepara los pedidos y te avisa cuando están listos.", "Prepares the orders and lets you know when they are ready.")),
        ], cols=2, tono="base"),
        bloque_faq(("Preguntas frecuentes", "Frequently asked questions"), [
            (("¿Me recomendáis algo para un problema de salud?", "Can you recommend something for a health problem?"),
             ("Te contamos para qué se usa tradicionalmente una planta y cómo prepararla. Diagnosticar y tratar es cosa de tu médico.",
              "We tell you what a plant is traditionally used for and how to prepare it. Diagnosing and treating is for your doctor.")),
            (("¿Qué diferencia hay entre natural y ecológico?", "What is the difference between natural and organic?"),
             ("«Ecológico» tiene normas y un organismo que controla. «Natural» lo escribe quien vende, así que mira la etiqueta.",
              "'Organic' has rules and a body that checks it. 'Natural' is written by the seller, so read the label.")),
            (("¿Puedo traer mi propio bote al granel?", "Can I bring my own jar for loose goods?"),
             ("Sí, limpio y seco. Lo pesamos vacío antes de llenarlo y pagas solo el producto.",
              "Yes, clean and dry. We weigh it empty before filling and you pay only for the product.")),
            (("¿Puedo devolver lo que he comprado?", "Can I return what I bought?"),
             ("Los envases cerrados y en buen estado, sí, con el ticket. Los productos abiertos, de cosmética o de granel no se devuelven.",
              "Sealed packs in good condition, yes, with the receipt. Opened, cosmetic or loose products cannot be returned.")),
        ], tono="relleno"),
        bloque_cta(("¿Tienes una duda concreta?", "Have a specific question?"), None, tono="base"),
    ])

    cont = pagina("contacto", ("contacto", "contact"), ("Contacto y cómo llegar", "Contact and directions"),
                  ("Teléfono, WhatsApp, formulario, horario de la tienda y mapa.",
                   "Phone, WhatsApp, form, shop hours and map."), [
        hero(("Contacto", "Contact"),
             ("Llámanos, escríbenos por WhatsApp o pasa por la tienda. Para ver lo que tenemos, mira la página de secciones y tienda.",
              "Call us, message us on WhatsApp or drop by the shop. To see what we have, go to the Sections and shop page."),
             [llamar(), whatsapp()], variante="texto"),
        bloque_contacto(("Dónde estamos", "Where we are"),
                        ("Pasa por la tienda o escríbenos. Para preguntar si tenemos algo, lo más ágil es WhatsApp.",
                         "Drop by the shop or write to us. To ask whether we have something, WhatsApp is quickest."),
                        campos_cita(servicios=[("Un pedido", "An order"), ("Una pregunta sobre un producto", "A question about a product"),
                                               ("Otra cosa", "Something else")], fecha=False),
                        ("Enviar mensaje", "Send message"), horas,
                        notas=("Los festivos pueden cambiar el horario: lo avisamos en la puerta.",
                               "Holidays may change the hours: we put a notice on the door.")),
        bloque_mapa(tono="relleno"),
    ])

    return plantilla(KEY, ("Herbolario y tienda ecológica", "Herbalist's and organic shop"),
                     ("Secciones de infusiones, suplementos, cosmética natural, alimentación ecológica y granel, catálogo con carrito, recogida en tienda, consejos con aviso de prudencia, horario y mapa, con botones de llamar y WhatsApp.",
                      "Sections for herbal teas, supplements, natural cosmetics, organic food and loose goods, catalogue with a cart, collection in store, advice with a word of caution, hours and a map, with call and WhatsApp buttons."),
                     "comercio", ("herbolario tienda ecologica herbolarista infusiones plantas medicinales suplementos cosmetica natural ecologico bio granel dietetica tienda de barrio",
                                  "herbalist herbalists organic shop health food store herbal tea supplements natural cosmetics organic loose goods wholefood"),
                     look, cabecera(nav, cta_cab, tienda=True),
                     pie(nav, ("Herbolario y tienda ecológica. Pedidos con recogida en la tienda.",
                               "Herbalist's and organic shop. Orders collected in store."),
                         ("Lunes a viernes 9:30 a 14:00 y 17:00 a 20:00. Sábado 10:00 a 14:00.",
                          "Monday to Friday 9:30 am to 2 pm and 5 pm to 8 pm. Saturday 10 am to 2 pm.")),
                     [inicio, tienda, consejos, cont], tienda=True)
