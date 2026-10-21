# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
from lib import *

KEY = "tienda-mascotas"

# Verde petróleo sobre blanco roto frío. No lleva fondo oscuro: primary/inverse no llega a 3.
PETROLEO = dict(
    colores=dict(background="#f5faf9", surface="#e3f0ee", text="#12282a", muted="#46605f", primary="#0f6b6f",
                 on_primary="#ffffff", border="#c9e0dd", inverse="#0d3336", on_inverse="#eaf7f6"),
    fuentes=("figtree", "work-sans"), radio=(10, 16, 26), ancho=1120)

NOTA_PRECIOS = ("Precios de ejemplo: cámbialos por los tuyos. Incluyen IVA.", "Example prices: replace them with your own. VAT included.")


def build():
    nueva(KEY)
    look = PETROLEO
    nav = [("inicio", ("Inicio", "Home")), ("tienda", ("Tienda", "Shop")),
           ("peluqueria", ("Peluquería canina", "Dog grooming")), ("contacto", ("Contacto", "Contact"))]
    horas = [(("Lunes a viernes", "Monday to Friday"), ("9:30 a 14:00 y 17:00 a 20:00", "9:30 am to 2:00 pm and 5:00 pm to 8:00 pm")),
             (("Sábado", "Saturday"), ("10:00 a 14:00", "10:00 am to 2:00 pm")),
             (("Domingo", "Sunday"), ("Cerrado", "Closed"))]
    cta_cab = marco([llamar(t=("Llamar", "Call")), whatsapp("ghost", ("WhatsApp", "WhatsApp"))],
                    dir="row", gap=8, align="center")

    def cita():
        return boton(("Pedir cita de peluquería", "Book a grooming slot"), a_pagina("contacto", "contacto"))

    def ver():
        return boton(("Ver la tienda", "Browse the shop"), a_pagina("tienda", "catalogo"))

    inicio = pagina("inicio", ("", ""), ("Tienda de mascotas", "Pet shop"),
                    ("Tienda de mascotas con tienda online y peluquería canina: alimentación, accesorios, higiene, cita y consejos de adopción.",
                     "Pet shop with an online store and dog grooming: food, accessories, hygiene, appointments and adoption advice."), [
        hero(("Todo para tu mascota, y la peluquería para tu perro", "Everything for your pet, and grooming for your dog"),
             ("Pienso, accesorios e higiene en la tienda y online, con recogida gratis. Y si tu perro necesita un baño o un corte, pide cita.",
              "Food, accessories and hygiene in the shop and online, with free pickup. And if your dog needs a bath or a trim, book a slot."),
             [ver(), cita(), whatsapp("ghost")],
             eyebrow=("Tienda de mascotas y peluquería canina", "Pet shop and dog grooming"),
             imagen_=("photo", ("Perro esperando en el mostrador de la tienda de mascotas", "Dog waiting at the pet shop counter"))),
        bloque_valores(("Qué encontrarás", "What you will find"), [
            (("Alimentación", "Food"), ("Pienso, latas y snacks para perros y gatos, y comida para pájaros, roedores y peces.",
                                       "Dry and wet food and treats for dogs and cats, plus food for birds, small mammals and fish.")),
            (("Accesorios", "Accessories"), ("Collares, correas, arneses, camas, transportines, juguetes y rascadores.",
                                            "Collars, leads, harnesses, beds, carriers, toys and scratching posts.")),
            (("Higiene", "Hygiene"), ("Champús, cepillos, areneros, arena y productos de cuidado diario.",
                                     "Shampoos, brushes, litter trays, litter and everyday care products.")),
        ], tono="relleno", estilo="relleno", anchor="que-encontraras"),
        bloque_productos(("Novedades", "New in"),
                         ("Lo último que ha llegado a la tienda. En la página Tienda está todo el catálogo.",
                          "The latest arrivals. The full catalogue is on the Shop page."),
                         limite=4, cols=4, anchor="novedades",
                         boton_=boton(("Ver todo el catálogo", "See the whole catalogue"), a_pagina("tienda", "catalogo"), "secondary")),
        bloque_texto(("Peluquería canina con cita", "Dog grooming by appointment"),
                     [("Baño, secado, corte, uñas y oídos, con el tiempo que necesita tu perro y sin jaulas llenas de perros esperando.",
                       "Bath, drying, trim, nails and ears, taking the time your dog needs and without crowded cages."),
                      ("Las tarifas van por tamaño. Mira cuánto cuesta el tuyo y reserva.",
                       "Rates go by size. See what yours costs and book.")],
                     eyebrow=("Peluquería", "Grooming"),
                     imagen_=("photo", ("Perro siendo cepillado en la mesa de peluquería", "Dog being brushed on the grooming table")),
                     tono="relleno", invertir=True, anchor="peluqueria-resumen"),
        bloque_checks(("Antes de adoptar", "Before you adopt"), [
            ("Una mascota vive muchos años: un perro, entre diez y quince. Piensa si puedes con el tiempo y el gasto.",
             "A pet lives many years: a dog, between ten and fifteen. Think about whether you can manage the time and cost."),
            ("Adopta en una protectora cuando puedas: hay muchos animales esperando casa.",
             "Adopt from a shelter when you can: many animals are waiting for a home."),
            ("Pasa por el veterinario en la primera semana: revisión, vacunas y desparasitación.",
             "Visit the vet in the first week: check-up, vaccinations and worming."),
            ("El microchip es obligatorio en perros y lo regula cada comunidad: pregunta a tu veterinario.",
             "Microchipping is compulsory for dogs and is regulated by each region: ask your vet."),
            ("Prepara la casa antes: cama, comedero, collar con chapa y un sitio tranquilo.",
             "Get the house ready beforehand: bed, bowl, collar with a tag and a quiet corner.")],
            intro=("Algunas pautas llanas para empezar bien. No sustituyen a la opinión de tu veterinario.",
                   "A few plain tips to get off to a good start. They do not replace your vet's opinion."),
            eyebrow=("Adopción responsable", "Responsible adoption"), tono="base", anchor="adopcion"),
        bloque_opiniones(("Lo que dicen nuestros clientes", "What our customers say"),
                         [("la tienda", "the shop"), ("la peluquería", "the grooming"), ("el trato con los animales", "how animals are treated")],
                         tono="relleno"),
        bloque_cta(("¿Buscas algo para tu mascota?", "Looking for something for your pet?"),
                   ("Dinos qué animal tienes y qué necesitas y te decimos qué tenemos.", "Tell us what animal you have and what you need and we will tell you what we have."),
                   [whatsapp("primary"), llamar("ghost")], tono="base"),
    ])

    tienda = pagina("tienda", ("tienda", "shop"), ("Tienda", "Shop"),
                    ("Catálogo de alimentación, accesorios e higiene para mascotas con carrito, recogida gratis y pienso a domicilio.",
                     "Catalogue of pet food, accessories and hygiene with a cart, free pickup and home delivery of food."), [
        hero(("Tienda", "Shop"),
             ("Añade al carrito y paga online. Recoge en la tienda sin coste o recíbelo en casa. Si no ves tu marca, pregúntanos.",
              "Add to the cart and pay online. Collect in the shop at no cost or have it delivered to your door. If you cannot see your brand, ask us."),
             [boton(("Pienso a domicilio", "Food delivery"), a_ancla("domicilio"), "secondary"), whatsapp("ghost")], variante="texto"),
        bloque_productos(("Todo el catálogo", "Full catalogue"), None, limite=24, cols=4, anchor="catalogo"),
        bloque_valores(("Qué hay en cada sección", "What is in each section"), [
            (("Alimentación", "Food"), ("Pienso seco, comida húmeda, snacks y suplementos para perros y gatos; semillas, heno y comida para otros animales.",
                                       "Dry food, wet food, treats and supplements for dogs and cats; seeds, hay and food for other animals.")),
            (("Accesorios", "Accessories"), ("Collares con chapa, correas, arneses, camas, mantas, transportines, comederos y juguetes.",
                                            "Tags and collars, leads, harnesses, beds, blankets, carriers, bowls and toys.")),
            (("Higiene y cuidado", "Hygiene and care"), ("Champús, cepillos, cortaúñas, toallitas, areneros y arena para gatos.",
                                                         "Shampoos, brushes, nail clippers, wipes, litter trays and cat litter.")),
        ], tono="relleno", estilo="borde", anchor="secciones"),
        bloque_texto(("Pienso a domicilio", "Food delivery to your door"),
                     [("Si das siempre el mismo pienso, te lo llevamos a casa. Dinos la marca, el tamaño del saco y cada cuánto lo necesitas, y lo preparamos.",
                       "If you always feed the same food, we bring it to your door. Tell us the brand, the bag size and how often you need it, and we prepare it."),
                      ("Repartimos en Tu ciudad y alrededores los martes y viernes (días de ejemplo: cámbialos por los tuyos). Los sacos pesados los subimos hasta la puerta.",
                       "We deliver in Your city and surrounding area on Tuesdays and Fridays (example days: replace them with your own). We carry heavy bags up to your door.")],
                     eyebrow=("Reparto", "Delivery"),
                     imagen_=("photo", ("Saco de pienso y cuenco junto a la puerta de una casa", "Bag of pet food and a bowl by the door of a house")),
                     tono="base", anchor="domicilio"),
        bloque_checks(("Cómo se recibe", "How you get it"), [
            ("Recogida en tienda, gratis, normalmente en 24 horas.", "In-store pickup, free, usually within 24 hours."),
            ("Envío a domicilio con número de seguimiento.", "Home delivery with a tracking number."),
            ("Si tu marca no está en el catálogo, pregúntanos: podemos pedirla.", "If your brand is not in the catalogue, ask us: we can order it."),
            ("Cambios y devoluciones en 14 días, con el envase cerrado.", "Exchanges and returns within 14 days, with the pack sealed.")],
            tono="relleno", anchor="entrega"),
        bloque_cta(("¿No sabes qué pienso darle?", "Not sure what food to give?"),
                   ("Cuéntanos qué animal es, qué edad tiene y si tiene alguna sensibilidad. Si es una cuestión de salud, mejor consulta a tu veterinario.",
                    "Tell us what animal it is, its age and whether it has any sensitivities. If it is a health matter, it is best to ask your vet."),
                   [whatsapp("primary"), llamar("ghost")], tono="base"),
    ])

    pelu = pagina("peluqueria", ("peluqueria", "grooming"), ("Peluquería canina", "Dog grooming"),
                  ("Tarifas de peluquería canina por tamaño, extras, cómo es una sesión y preguntas frecuentes. Pide cita por teléfono, WhatsApp o formulario.",
                   "Dog grooming rates by size, extras, what a session is like and frequently asked questions. Book by phone, WhatsApp or form."), [
        hero(("Peluquería canina", "Dog grooming"),
             ("Con cita previa y de uno en uno. El precio depende del tamaño y del pelo de tu perro; aquí tienes las tarifas.",
              "By appointment and one at a time. The price depends on your dog's size and coat; here are the rates."),
             [cita(), llamar("secondary"), whatsapp("ghost")], variante="texto"),
        bloque_precios(("Tarifas por tamaño", "Rates by size"), ("Tarifas de peluquería canina por tamaño", "Dog grooming rates by size"),
                       [("Tamaño", "Size"), ("Baño y cepillado", "Bath and brushing"), ("Baño y corte", "Bath and trim")],
                       [(("Pequeño (hasta 8 kg)", "Small (up to 8 kg)"), eur(18), eur(30)),
                        (("Mediano (de 8 a 20 kg)", "Medium (8 to 20 kg)"), eur(24), eur(38)),
                        (("Grande (de 20 a 35 kg)", "Large (20 to 35 kg)"), eur(32), eur(48)),
                        (("Gigante (más de 35 kg)", "Giant (over 35 kg)"), desde(40), desde(60))],
                       intro=("Si el pelo viene muy enredado o con mucho pelo muerto, puede haber un suplemento. Te lo decimos antes de empezar.",
                              "If the coat is badly matted or has a lot of dead hair, there may be a surcharge. We tell you before we start."),
                       nota=NOTA_PRECIOS, anchor="tarifas", tono="base"),
        bloque_precios(("Extras", "Extras"), ("Extras de peluquería canina", "Dog grooming extras"),
                       [("Extra", "Extra"), ("Precio", "Price")],
                       [(("Corte de uñas", "Nail clipping"), eur(5)), (("Limpieza de oídos", "Ear cleaning"), eur(4)),
                        (("Cepillado de dientes", "Teeth brushing"), eur(4)), (("Deslanado", "De-shedding"), desde(10)),
                        (("Tratamiento antiparasitario externo", "External parasite treatment"), desde(6))],
                       nota=NOTA_PRECIOS, anchor="extras", tono="relleno"),
        bloque_pasos(("Cómo es una sesión", "What a session is like"), [
            (("Llegáis y lo vemos", "You arrive and we take a look"),
             ("Miramos el pelo y la piel, y hablamos de cómo lo quieres.", "We check the coat and skin, and talk about how you want it.")),
            (("Baño y secado", "Bath and drying"),
             ("Champú adecuado a su pelo, aclarado a fondo y secado sin prisas.", "A shampoo suited to its coat, a thorough rinse and unhurried drying.")),
            (("Corte y detalles", "Trim and finishing"),
             ("Corte, uñas y oídos si los has pedido, con descansos si se pone nervioso.", "Trim, nails and ears if you asked for them, with breaks if it gets nervous.")),
            (("Te avisamos", "We call you"),
             ("Cuando está listo, te llamamos para que vengas a por él. Suele tardar entre una hora y media y tres horas.",
              "When it is ready, we call so you can come for it. It usually takes between an hour and a half and three hours.")),
        ], tono="base", anchor="sesion"),
        bloque_faq(("Preguntas frecuentes", "Frequently asked questions"), [
            (("¿Qué tiene que llevar mi perro?", "What does my dog need?"),
             ("Venir desparasitado y con las vacunas al día, por la salud de todos los que pasan por la mesa. Si tiene algún problema de piel o de salud, avísanos al pedir cita.",
              "To arrive wormed and with up-to-date vaccinations, for the health of every dog that uses the table. If it has a skin or health problem, tell us when you book.")),
            (("¿Cada cuánto debería venir?", "How often should it come?"),
             ("Depende del pelo: el corto suele bastar cada uno o dos meses; el largo, cada mes o seis semanas. Te aconsejamos en la primera visita.",
              "It depends on the coat: short coats usually need it every one or two months; long coats every month or six weeks. We advise you at the first visit.")),
            (("¿Y si mi perro se pone nervioso?", "What if my dog gets nervous?"),
             ("Lo tratamos con calma y paramos cuando hace falta. Si no es posible hacerlo con seguridad, te lo decimos y lo dejamos para otro día.",
              "We treat it calmly and stop when needed. If it cannot be done safely, we tell you and leave it for another day.")),
            (("¿Y a los gatos?", "What about cats?"),
             ("Consúltanos antes de pedir cita: depende del gato y del servicio.", "Ask us before booking: it depends on the cat and the service.")),
        ], tono="relleno"),
        bloque_cta(("¿Qué día le viene bien a tu perro?", "Which day suits your dog?"), None, [cita(), whatsapp("ghost")], tono="base"),
    ])

    cont = pagina("contacto", ("contacto", "contact"), ("Contacto y cita", "Contact and appointments"),
                  ("Pide cita de peluquería canina o consulta por un pedido. Horario, teléfono, WhatsApp y cómo llegar a la tienda.",
                   "Book a dog grooming slot or ask about an order. Hours, phone, WhatsApp and how to get to the shop."), [
        hero(("Pide cita o consúltanos", "Book a slot or ask us"),
             ("Para la peluquería, dinos el tamaño de tu perro y el día que te viene bien. Para un pedido, el número que te llegó por correo.",
              "For grooming, tell us your dog's size and the day that suits you. For an order, the number you received by email."),
             [llamar(), whatsapp()], variante="texto"),
        bloque_contacto(("Estamos en la tienda", "We are in the shop"),
                        ("Te confirmamos la cita por teléfono o WhatsApp.", "We confirm your slot by phone or WhatsApp."),
                        campos_cita(servicios=[("Baño y cepillado", "Bath and brushing"), ("Baño y corte", "Bath and trim"),
                                               ("Un pedido de la tienda", "A shop order"), ("Otra consulta", "Other enquiry")],
                                    extra=[campo("tamano", "select", ("Tamaño del perro", "Dog's size"), False,
                                                 opciones=[("Pequeño (hasta 8 kg)", "Small (up to 8 kg)"), ("Mediano (8 a 20 kg)", "Medium (8 to 20 kg)"),
                                                           ("Grande (20 a 35 kg)", "Large (20 to 35 kg)"), ("Gigante (más de 35 kg)", "Giant (over 35 kg)")])]),
                        ("Pedir cita", "Request a slot"), horas,
                        exito=("Gracias. Te confirmamos la cita por teléfono o WhatsApp en cuanto la tengamos.",
                               "Thank you. We will confirm your slot by phone or WhatsApp as soon as we have it.")),
        bloque_mapa(("Cómo llegar a la tienda", "How to get to the shop"), tono="relleno"),
    ])

    return plantilla(KEY, ("Tienda de mascotas", "Pet shop"),
                     ("Tienda de mascotas con catálogo y carrito, peluquería canina con tarifas por tamaño y cita, pienso a domicilio, consejos de adopción y botones de llamar y WhatsApp.",
                      "Pet shop with a catalogue and cart, dog grooming with rates by size and booking, food delivery, adoption advice and call and WhatsApp buttons."),
                     "comercio", ("tienda de mascotas mascotas animales perros gatos pienso accesorios peluqueria canina pelu canina bano perros adopcion tienda online comercio",
                                  "pet shop pet store pets animals dogs cats pet food accessories dog grooming groomer dog wash adoption online shop"),
                     look, cabecera(nav, cta_cab, tienda=True),
                     pie(nav, ("Tienda de mascotas y peluquería canina, en la tienda y online.",
                               "Pet shop and dog grooming, in the shop and online."),
                         ("Lunes a viernes 9:30 a 14:00 y 17:00 a 20:00. Sábado 10:00 a 14:00.",
                          "Monday to Friday 9:30 am to 2 pm and 5 pm to 8 pm. Saturday 10 am to 2 pm.")),
                     [inicio, tienda, pelu, cont], tienda=True)
