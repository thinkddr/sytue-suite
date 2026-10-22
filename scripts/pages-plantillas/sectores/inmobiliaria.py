# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
from lib import *
from looks import LOOKS

KEY = "inmobiliaria"

AVISO_EJEMPLO = ("Inmuebles y precios de ejemplo: cámbialos por los tuyos.", "Example properties and prices: replace them with your own.")


def inmueble(titulo, zona, datos, precio, alt):
    """Tarjeta de inmueble de relleno: foto, rótulo «Ejemplo», precio y datos básicos."""
    return tarjeta([
        imagen("photo", alt, ratio=1.4),
        etiqueta(("Ejemplo", "Example")),
        h3(titulo),
        p(zona, color="$muted"),
        p(datos, preset="small"),
        texto("p", precio, preset="h4"),
        boton(("Me interesa", "I am interested"), a_pagina("contacto", "contacto"), "secondary"),
    ], estilo="borde", pad=16, gap=10)


def build():
    nueva(KEY)
    look = dict(LOOKS["cobalto"], fuentes=("montserrat", "work-sans"))
    nav = [("inicio", ("Inicio", "Home")), ("vender", ("Vender y tasar", "Selling and valuations")),
           ("comprar", ("Comprar y alquilar", "Buying and renting")), ("contacto", ("Contacto", "Contact"))]
    horas = [(("Lunes a viernes", "Monday to Friday"),
              ("10:00 a 14:00 y 17:00 a 20:00", "10:00 am to 2:00 pm and 5:00 pm to 8:00 pm")),
             (("Sábado", "Saturday"), ("10:00 a 13:30", "10:00 am to 1:30 pm")),
             (("Domingo", "Sunday"), ("Cerrado", "Closed")),
             (("Visitas a inmuebles", "Property viewings"), ("A cualquier hora, con cita", "At any time, by appointment"))]
    cta_cab = boton(("Tasación gratuita", "Free valuation"), a_pagina("vender", "tasacion"))
    tasar = lambda v="primary": boton(("Pedir tasación gratuita", "Ask for a free valuation"), a_pagina("vender", "tasacion"), v)

    inicio = pagina("inicio", ("", ""), ("Inmobiliaria", "Estate agency"),
                    ("Agencia inmobiliaria local: vender, comprar y alquilar con acompañamiento, tasación gratuita e inmuebles destacados. Llama o escribe por WhatsApp.",
                     "Local estate agency: selling, buying and renting with support, a free valuation and featured properties. Call or message us on WhatsApp."), [
        hero(("Vender, comprar o alquilar en tu barrio, con alguien que lo conoce", "Selling, buying or renting in your neighbourhood, with someone who knows it"),
             ("Somos una agencia local. Te decimos cuánto vale tu inmueble, te enseñamos lo que encaja con lo que buscas y te acompañamos hasta la firma.",
              "We are a local agency. We tell you what your property is worth, show you what fits what you are looking for and stay with you until you sign."),
             [tasar(), llamar("secondary"), whatsapp("ghost")],
             eyebrow=("Agencia inmobiliaria", "Estate agency"),
             imagen_=("landscape", ("Fachada de un edificio de viviendas en el centro del barrio", "Front of a residential building in the middle of the neighbourhood"))),
        seccion("features", [
            titular(("¿Qué quieres hacer?", "What would you like to do?")),
            rejilla([
                tarjeta([h3(("Vender", "Sell")),
                         p(("Te valoramos el inmueble, lo anunciamos, organizamos las visitas y te acompañamos hasta la firma.",
                            "We value your property, advertise it, arrange the viewings and stay with you until you sign."), color="$muted"),
                         boton(("Cómo vender", "How to sell"), a_pagina("vender"), "secondary")], estilo="relleno"),
                tarjeta([h3(("Comprar", "Buy")),
                         p(("Nos cuentas qué buscas y cuánto puedes gastar. Te enseñamos lo que encaja y te explicamos los gastos.",
                            "You tell us what you are looking for and what you can spend. We show you what fits and explain the costs."), color="$muted"),
                         boton(("Cómo comprar", "How to buy"), a_pagina("comprar", "comprar"), "secondary")], estilo="relleno"),
                tarjeta([h3(("Alquilar", "Rent")),
                         p(("Si eres propietario, buscamos inquilino. Si buscas piso, te decimos qué hay y qué te pediremos.",
                            "If you are an owner, we find a tenant. If you are looking for a flat, we tell you what is available and what we will ask of you."), color="$muted"),
                         boton(("Cómo alquilar", "How to rent"), a_pagina("comprar", "alquilar"), "secondary")], estilo="relleno"),
            ], cols=3),
        ], tono="relleno"),
        seccion("features", [
            titular(("Inmuebles destacados", "Featured properties"),
                    ("Una selección de lo que tenemos ahora. Pregúntanos por lo que aún no está anunciado.",
                     "A selection of what we have right now. Ask us about what is not advertised yet."),
                    eyebrow=("Ahora en cartera", "Currently listed")),
            rejilla([
                inmueble(("Piso de tres habitaciones en el centro", "Three-bedroom flat in the centre"), ("Centro de Tu ciudad", "Your city centre"),
                         ("3 habitaciones, 2 baños, 95 m²", "3 bedrooms, 2 bathrooms, 95 m²"), ("245.000 €", "€245,000"),
                         ("Salón de un piso de ejemplo", "Living room of a sample flat")),
                inmueble(("Casa con jardín", "House with garden"), ("Urbanización de Tu ciudad", "Housing estate in your city"),
                         ("4 habitaciones, 3 baños, 180 m²", "4 bedrooms, 3 bathrooms, 180 m²"), ("389.000 €", "€389,000"),
                         ("Fachada de una casa de ejemplo con jardín", "Front of a sample house with a garden")),
                inmueble(("Piso para alquilar cerca del tranvía", "Flat to rent near the tram"), ("Barrio norte de Tu ciudad", "Northern district of your city"),
                         ("2 habitaciones, 1 baño, 70 m²", "2 bedrooms, 1 bathroom, 70 m²"), ("850 € al mes", "€850 a month"),
                         ("Cocina de un piso de ejemplo", "Kitchen of a sample flat")),
            ], cols=3),
            peq(AVISO_EJEMPLO, color="$muted"),
        ], anchor="destacados", gap=28),
        bloque_checks(("Dónde trabajamos", "Where we work"), [
            ("Centro de Tu ciudad", "Your city centre"),
            ("Barrios del norte", "Northern districts"),
            ("Barrios del sur y urbanizaciones", "Southern districts and housing estates"),
            ("Pueblos de alrededor", "Surrounding villages"),
        ], intro=("Conocemos los precios reales de cada calle porque trabajamos aquí. Si tu zona no aparece, llámanos y te decimos si podemos ayudarte.",
                  "We know the real prices on each street because we work here. If your area is not listed, call us and we will tell you whether we can help."),
            eyebrow=("Zona de actuación", "Area we cover"), tono="relleno", anchor="zonas", cols=2),
        bloque_opiniones(("Lo que dicen quienes han vendido, comprado o alquilado con nosotros", "What people who have sold, bought or rented with us say"),
                         [("la venta", "the sale"), ("la compra", "the purchase"), ("el alquiler", "the rental")], tono="base"),
        bloque_cta(("¿Cuánto vale tu inmueble?", "How much is your property worth?"),
                   ("Cuéntanos dónde está y cómo es. Te damos una valoración gratuita y sin compromiso.",
                    "Tell us where it is and what it is like. We give you a free valuation with no obligation."),
                   [tasar(), llamar("secondary")], tono="relleno"),
    ])

    vender = pagina("vender", ("vender", "sell"), ("Vender y tasar", "Selling and valuations"),
                    ("Tasación gratuita con formulario, los pasos para vender, documentos que te pedimos, honorarios y preguntas frecuentes.",
                     "Free valuation with a form, the steps to sell, the documents we ask for, fees and frequently asked questions."), [
        hero(("Vender tu inmueble con un precio realista", "Selling your property at a realistic price"),
             ("Un precio ajustado a la zona vende antes que uno inflado. Empezamos por una valoración gratuita y sin compromiso.",
              "A price in line with the area sells faster than an inflated one. We start with a free valuation with no obligation."),
             [tasar(), llamar("secondary")], variante="texto"),
        seccion("contact", [
            marco([
                marco([titular(("Pide tu tasación gratuita", "Ask for your free valuation"),
                               ("Rellena estos datos y te llamamos para concretar. Si hace falta, pasamos a ver el inmueble.",
                                "Fill in these details and we will call you to finalise. If needed, we come and see the property."),
                               eyebrow=("Tasación", "Valuation")),
                       peq(("La valoración que te damos es orientativa y gratuita. No sustituye a una tasación oficial de las que piden los bancos.",
                            "The valuation we give you is indicative and free. It does not replace an official valuation of the kind banks ask for."),
                           color="$muted"),
                       marco([llamar("secondary"), whatsapp("ghost")], dir="row", gap=12, wrap=True)],
                      gap=18, sz={"width": "fill", "height": "hug", "min_width": 0}),
                tarjeta([formulario([
                    campo("nombre", "text", ("Nombre", "Name"), True, auto="name", maxlen=120),
                    campo("telefono", "tel", ("Teléfono", "Phone"), True, auto="tel", maxlen=30),
                    campo("correo", "email", ("Correo electrónico (opcional)", "Email (optional)"), False, auto="email", maxlen=200),
                    campo("tipo", "select", ("Tipo de inmueble", "Type of property"), True,
                          opciones=[("Piso", "Flat"), ("Casa o chalet", "House or villa"), ("Local", "Commercial unit"),
                                    ("Garaje o trastero", "Garage or storage room"), ("Terreno", "Land"), ("Otro", "Other")]),
                    campo("zona", "text", ("Zona o dirección", "Area or address"), True, maxlen=200),
                    campo("metros", "text", ("Metros cuadrados aproximados", "Approximate floor area in square metres"), True, maxlen=20),
                    campo("mensaje", "textarea", ("Algo más que debamos saber (reformas, vistas, ascensor...)",
                                                  "Anything else we should know (renovations, views, lift...)"), False, maxlen=1000),
                ], ("Pedir tasación", "Ask for valuation"),
                    exito=("Gracias. Te llamamos para concretar la valoración de tu inmueble.",
                           "Thank you. We will call you to arrange the valuation of your property."))],
                    estilo="relleno", pad=28, tag="div", sz={"width": "fill", "height": "hug", "min_width": 0}),
            ], dir="row", gap=48, align="start",
                bp={"tablet": {"layout": {"gap": 32}}, "mobile": {"layout": {"direction": "column", "gap": 28}}}),
        ], anchor="tasacion", tono="base", gap=0),
        bloque_pasos(("Pasos para vender", "Steps to sell"), [
            (("Valoración", "Valuation"),
             ("Vemos el inmueble o los datos que nos das y te proponemos un precio de salida con los pisos parecidos de la zona.",
              "We see the property or the details you give us and suggest an asking price based on similar properties in the area.")),
            (("Encargo de venta", "Sales agreement"),
             ("Si decides contar con nosotros, firmamos un encargo por escrito con el precio, el plazo y los honorarios.",
              "If you decide to go with us, we sign a written agreement setting out the price, the period and the fees.")),
            (("Fotos y anuncio", "Photos and advert"),
             ("Hacemos las fotos, redactamos el anuncio y lo publicamos. Tú lo revisas antes.",
              "We take the photos, write the advert and publish it. You review it first.")),
            (("Visitas y ofertas", "Viewings and offers"),
             ("Atendemos a los interesados, hacemos las visitas y te contamos cada oferta tal cual llega.",
              "We deal with interested buyers, run the viewings and tell you about each offer exactly as it comes.")),
            (("Reserva y arras", "Reservation and deposit"),
             ("Cuando aceptas una oferta, se firma la reserva y el contrato de arras con lo pactado.",
              "When you accept an offer, the reservation and the deposit contract are signed with what was agreed.")),
            (("Notaría y entrega de llaves", "Notary and handing over the keys"),
             ("Te acompañamos a la firma de la escritura y a la entrega de llaves.",
              "We go with you to the signing of the deeds and the handing over of the keys.")),
        ], tono="relleno", eyebrow=("Cómo es", "How it works"), anchor="pasos"),
        bloque_checks(("Documentos que te vamos a pedir", "Documents we will ask you for"), [
            ("Escritura de propiedad o nota simple del Registro.", "Title deeds or a land registry extract."),
            ("Último recibo del IBI.", "Latest property tax bill."),
            ("Certificado de eficiencia energética, que hace falta para anunciar y vender. Si no lo tienes, te decimos cómo conseguirlo.",
             "Energy performance certificate, which is needed to advertise and sell. If you do not have one, we tell you how to get it."),
            ("Certificado de que la comunidad de vecinos no tiene deudas.", "A certificate that the residents' association has no debts."),
            ("Documento de identidad de quienes figuren como propietarios.", "ID of everyone listed as an owner."),
            ("Otros documentos que pidan en tu comunidad autónoma. Te los decimos en la valoración.",
             "Any other documents required in your region. We tell you at the valuation."),
        ], eyebrow=("Papeles", "Paperwork"), anchor="documentos"),
        bloque_texto(("Honorarios: a convenir", "Fees: to be agreed"), [
            ("Nuestros honorarios se acuerdan contigo y se firman antes de empezar, según el inmueble y los servicios que necesites. No hay sorpresas después: lo que firmas es lo que se aplica.",
             "Our fees are agreed with you and signed before we start, depending on the property and the services you need. No surprises later: what you sign is what applies."),
            ("Pregúntanos sin compromiso y te explicamos cómo se calculan antes de que decidas.",
             "Ask us with no obligation and we will explain how they are worked out before you decide.")],
            eyebrow=("Cuánto cuesta", "What it costs"), tono="relleno", anchor="honorarios"),
        bloque_faq(("Preguntas frecuentes al vender", "Frequently asked questions when selling"), [
            (("¿Cuánto tarda en venderse?", "How long does it take to sell?"),
             ("Depende de la zona, del precio y del estado. Te damos una idea realista en la valoración, sin prometerte un plazo.",
              "It depends on the area, the price and the condition. We give you a realistic idea at the valuation, without promising a timescale.")),
            (("¿Puedo vender si aún tengo hipoteca?", "Can I sell if I still have a mortgage?"),
             ("Sí, es muy habitual. Se cancela con lo que se cobra en la venta. Te explicamos los pasos y los documentos que pedirá tu banco.",
              "Yes, it is very common. It is paid off from the sale proceeds. We explain the steps and the documents your bank will ask for.")),
            (("¿Tengo que dejar la casa vacía?", "Do I have to empty the home?"),
             ("No para anunciarla. Ordenada y despejada se enseña mejor, y te damos ideas para las visitas.",
              "Not to advertise it. Tidy and uncluttered it shows better, and we give you ideas for the viewings.")),
        ], tono="base"),
        bloque_cta(("¿Empezamos por la valoración?", "Shall we start with the valuation?"), None, [tasar(), whatsapp("ghost")], tono="relleno"),
    ])

    comprar = pagina("comprar", ("comprar", "buy"), ("Comprar y alquilar", "Buying and renting"),
                     ("Cómo comprar paso a paso, cómo alquilar si eres propietario o inquilino, y cómo organizar las visitas.",
                      "How to buy step by step, how to rent as an owner or tenant, and how we arrange viewings."), [
        hero(("Comprar o alquilar con calma", "Buying or renting without rushing"),
             ("Nos dices qué necesitas y cuánto puedes pagar. Te enseñamos solo lo que encaja y te explicamos qué gastos hay antes de que decidas.",
              "You tell us what you need and what you can pay. We show you only what fits and explain the costs before you decide."),
             [boton(("Pedir visita", "Book a viewing"), a_pagina("contacto", "contacto")), llamar("secondary")], variante="texto"),
        bloque_pasos(("Pasos para comprar", "Steps to buy"), [
            (("Qué buscas", "What you are looking for"),
             ("Zona, tamaño, presupuesto y lo que no negocias: ascensor, terraza, cerca del colegio.",
              "Area, size, budget and your must-haves: a lift, a terrace, near the school.")),
            (("Visitas", "Viewings"),
             ("Te enseñamos solo lo que encaja y te contamos lo bueno y lo malo de cada inmueble.",
              "We show you only what fits and tell you the good and the bad of each property.")),
            (("Oferta y reserva", "Offer and reservation"),
             ("Presentamos tu oferta a la propiedad y, si la aceptan, se firma la reserva con lo pactado.",
              "We put your offer to the owner and, if accepted, the reservation is signed with what was agreed.")),
            (("Financiación y firma", "Finance and signing"),
             ("Te orientamos con la hipoteca y los gastos de notaría, registro e impuestos. Te acompañamos a la firma.",
              "We guide you with the mortgage and the notary, registry and tax costs. We go with you to the signing.")),
        ], eyebrow=("Comprar", "Buying"), anchor="comprar"),
        seccion("features", [
            titular(("Alquilar", "Renting"), None, eyebrow=("Propietarios e inquilinos", "Owners and tenants")),
            rejilla([
                tarjeta([h3(("Si eres propietario", "If you are an owner")),
                         lista_vertical([
                             fila_li([icono("check", sz=22), p(("Valoramos cuánto puedes pedir de renta en tu zona.", "We assess how much rent you can ask for in your area."))], dir="row", gap=12),
                             fila_li([icono("check", sz=22), p(("Buscamos inquilinos y revisamos su documentación contigo.", "We find tenants and check their paperwork with you."))], dir="row", gap=12),
                             fila_li([icono("check", sz=22), p(("Preparamos el contrato y la entrega del inmueble.", "We prepare the contract and the handover of the property."))], dir="row", gap=12),
                         ])], estilo="relleno", tag="div", pad=28, gap=16),
                tarjeta([h3(("Si buscas piso", "If you are looking for a flat")),
                         lista_vertical([
                             fila_li([icono("check", sz=22), p(("Te decimos qué hay y a qué precio, sin sorpresas.", "We tell you what is available and at what price, no surprises."))], dir="row", gap=12),
                             fila_li([icono("check", sz=22), p(("Te explicamos qué documentación suele pedir la propiedad.", "We explain which documents owners usually ask for."))], dir="row", gap=12),
                             fila_li([icono("check", sz=22), p(("Te explicamos la fianza y los gastos antes de firmar.", "We explain the deposit and the costs before you sign."))], dir="row", gap=12),
                         ])], estilo="relleno", tag="div", pad=28, gap=16),
            ], cols=2, lista=False),
        ], tono="relleno", anchor="alquilar"),
        bloque_faq(("Preguntas frecuentes", "Frequently asked questions"), [
            (("¿Cuánto cuesta comprar, además del precio?", "What does buying cost on top of the price?"),
             ("Hay impuestos, notaría y registro, y a veces gestoría o gastos de hipoteca. Antes de reservar te damos una estimación por escrito.",
              "There are taxes, notary and registry fees, and sometimes administrative or mortgage costs. Before you reserve we give you an estimate in writing.")),
            (("¿Puedo ver un inmueble fuera de horario?", "Can I see a property outside opening hours?"),
             ("Sí, las visitas se hacen con cita y puedes pedir la hora que te venga bien, también tarde o en fin de semana.",
              "Yes, viewings are by appointment and you can ask for any time that suits you, including evenings or weekends.")),
            (("¿Qué hace falta para alquilar?", "What do I need to rent?"),
             ("Cada propiedad pide lo suyo. Lo habitual es identificación y justificantes de ingresos. Te lo decimos al enseñarte el piso.",
              "Each owner asks for different things. Usually it is ID and proof of income. We tell you when we show you the flat.")),
        ], tono="base"),
        bloque_cta(("¿Qué estás buscando?", "What are you looking for?"),
                   ("Cuéntanos qué necesitas y te avisamos cuando entre algo que encaje.",
                    "Tell us what you need and we will let you know when something that fits comes in."),
                   [boton(("Pedir visita", "Book a viewing"), a_pagina("contacto", "contacto")), whatsapp("ghost")], tono="relleno"),
    ])

    cont = pagina("contacto", ("contacto", "contact"), ("Contacto", "Contact"),
                  ("Llama, escribe por WhatsApp o pide una visita con el formulario. Horario y cómo llegar a la agencia.",
                   "Call, message us on WhatsApp or book a viewing with the form. Opening hours and how to get to the agency."), [
        hero(("Hablemos de tu inmueble", "Let us talk about your property"),
             ("Llámanos, escríbenos por WhatsApp o rellena el formulario. Te respondemos en un día laborable.",
              "Call us, message us on WhatsApp or fill in the form. We reply within one working day."),
             [llamar(), whatsapp()], variante="texto"),
        bloque_contacto(("Cuéntanos qué necesitas", "Tell us what you need"),
                        ("Dinos si quieres vender, comprar o alquilar y te llamamos para ponernos de acuerdo.",
                         "Tell us whether you want to sell, buy or rent and we will call you to arrange things."),
                        campos_cita(servicios=[("Quiero vender", "I want to sell"), ("Quiero comprar", "I want to buy"),
                                               ("Quiero alquilar mi inmueble", "I want to rent out my property"),
                                               ("Busco un alquiler", "I am looking for a rental"), ("Otro", "Other")]),
                        ("Enviar", "Send"), horas,
                        exito=("Gracias. Te llamamos para concretar lo que necesitas.", "Thank you. We will call you to arrange what you need.")),
        bloque_mapa(tono="relleno"),
    ])

    return plantilla(KEY, ("Inmobiliaria", "Estate agency"),
                     ("Vender, comprar y alquilar con tasación gratuita (formulario), pasos para vender, inmuebles destacados de ejemplo, zona de actuación y botones de llamar y WhatsApp.",
                      "Selling, buying and renting with a free valuation (form), steps to sell, sample featured properties, area covered and call and WhatsApp buttons."),
                     "profesionales", ("inmobiliaria agencia inmobiliaria inmobiliario tasacion gratuita vender piso comprar casa alquilar pisos alquiler venta propiedades agente inmobiliario",
                                       "estate agency estate agent real estate property valuation sell flat buy house rent letting sales properties"),
                     look, cabecera(nav, cta_cab),
                     pie(nav, ("Agencia inmobiliaria local. Vender, comprar y alquilar, con tasación gratuita.",
                               "Local estate agency. Selling, buying and renting, with a free valuation."),
                         ("Lunes a viernes 10:00 a 14:00 y 17:00 a 20:00. Sábado 10:00 a 13:30.",
                          "Monday to Friday 10 am to 2 pm and 5 pm to 8 pm. Saturday 10 am to 1:30 pm.")),
                     [inicio, vender, comprar, cont])
