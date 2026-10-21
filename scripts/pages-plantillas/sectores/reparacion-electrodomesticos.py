# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
from lib import *
from looks import LOOKS

KEY = "reparacion-electrodomesticos"

# Paleta propia (violeta sobre blanco frío). Sin fondos $inverse: primary/inverse queda por debajo de 3.
VIOLETA = dict(background="#f8f6fc", surface="#ece7f6", text="#1d1530", muted="#54486f", primary="#5e35b1",
               on_primary="#ffffff", border="#dcd3ee", inverse="#231536", on_inverse="#f4effb")

NOTA_TARIFAS = ("Precios de ejemplo: cámbialos por los tuyos. Incluyen IVA. Las piezas se cobran aparte, al precio que figura en la factura.",
                "Example prices: replace them with your own. VAT included. Parts are charged separately, at the price shown on the invoice.")


def build():
    nueva(KEY)
    look = dict(colores=VIOLETA, fuentes=("manrope", "noto-sans"), radio=(6, 10, 16))
    nav = [("inicio", ("Inicio", "Home")), ("reparaciones", ("Qué reparamos y tarifas", "What we repair and rates")),
           ("contacto", ("Pedir visita", "Book a visit"))]
    horas = [(("Lunes a viernes", "Monday to Friday"),
              ("9:00 a 14:00 y 16:00 a 19:00", "9:00 am to 2:00 pm and 4:00 pm to 7:00 pm")),
             (("Sábado", "Saturday"), ("9:00 a 13:00, solo con cita", "9:00 am to 1:00 pm, by appointment only")),
             (("Domingo", "Sunday"), ("Cerrado", "Closed"))]
    cta_cab = marco([llamar(t=("Llamar", "Call")), whatsapp("secondary", t=("WhatsApp", "WhatsApp"))],
                    dir="row", gap=8, wrap=True)
    pedir = lambda v="primary": boton(("Pedir visita", "Book a visit"), a_pagina("contacto", "contacto"), v)
    aparatos = [("Lavadora", "Washing machine"), ("Frigorífico o congelador", "Fridge or freezer"), ("Horno", "Oven"),
                ("Lavavajillas", "Dishwasher"), ("Secadora", "Tumble dryer"), ("Otro aparato", "Other appliance")]

    inicio = pagina("inicio", ("", ""), ("Servicio técnico de electrodomésticos", "Appliance repair service"),
                    ("Reparación de lavadoras, frigoríficos, hornos, lavavajillas y secadoras en tu casa, con diagnóstico y presupuesto antes de reparar.",
                     "Repairs for washing machines, fridges, ovens, dishwashers and tumble dryers at your home, with a diagnosis and a quote before any repair."), [
        hero(("Si se estropea en casa, lo arreglamos en casa", "If it breaks at home, we fix it at home"),
             ("Vamos a tu domicilio, vemos qué le pasa al aparato y te decimos si compensa repararlo antes de tocar nada.",
              "We come to your home, find out what is wrong with the appliance and tell you whether a repair is worth it before touching anything."),
             [pedir(), llamar("secondary"), whatsapp("ghost")],
             eyebrow=("Servicio técnico a domicilio", "Home repair service"),
             imagen_=("photo", ("Técnico revisando una lavadora en una cocina", "Technician checking a washing machine in a kitchen"))),
        bloque_valores(("Qué aparatos reparamos", "Which appliances we repair"), [
            (("Lavadoras", "Washing machines"),
             ("No desagua, no centrifuga, pierde agua, hace ruido o se queda con la puerta cerrada.",
              "It does not drain, does not spin, leaks, makes noise or keeps the door locked.")),
            (("Frigoríficos y congeladores", "Fridges and freezers"),
             ("No enfría, enfría de más, hace hielo donde no debe o no para de sonar.",
              "It does not cool, cools too much, ices up where it should not or never stops running.")),
            (("Hornos", "Ovens"),
             ("No calienta, calienta mal, el ventilador no gira o el mando no responde.",
              "It does not heat, heats unevenly, the fan does not turn or the control does not respond.")),
            (("Lavavajillas", "Dishwashers"),
             ("No desagua, deja la vajilla sucia, no entra agua o no termina el programa.",
              "It does not drain, leaves dishes dirty, does not fill or does not finish the cycle.")),
            (("Secadoras", "Tumble dryers"),
             ("No seca, tarda demasiado, el tambor no gira o salta el programa a mitad.",
              "It does not dry, takes too long, the drum does not turn or the cycle stops halfway.")),
            (("Otros aparatos", "Other appliances"),
             ("Placas, campanas o microondas: pregúntanos y te decimos si podemos ir a verlo.",
              "Hobs, cooker hoods or microwaves: ask us and we will tell you if we can come and look.")),
        ], eyebrow=("Reparaciones", "Repairs"), estilo="relleno", tono="base"),
        bloque_pasos(("Cómo es una visita", "What a visit is like"), [
            (("Nos cuentas la avería", "You tell us the fault"),
             ("Por teléfono o WhatsApp. Cuando puedas, mándanos una foto de la etiqueta con la marca y el modelo.",
              "By phone or WhatsApp. When you can, send us a photo of the label with the make and model.")),
            (("Fijamos día y franja", "We agree a day and time slot"),
             ("Te damos una franja horaria para que no pases el día esperando.",
              "We give you a time slot so you do not spend the whole day waiting.")),
            (("Diagnóstico en tu casa", "Diagnosis at your home"),
             ("Revisamos el aparato y te explicamos la causa en lenguaje llano, con el coste de la reparación.",
              "We check the appliance and explain the cause in plain language, with the cost of the repair.")),
            (("Reparamos o lo dejas", "We repair it, or you decide not to"),
             ("Si te encaja, reparamos en la misma visita siempre que llevemos la pieza. Si no, la pedimos y volvemos.",
              "If you agree, we repair it on the same visit whenever we have the part with us. If not, we order it and come back.")),
        ], tono="base"),
        bloque_texto(("¿Compensa repararlo?", "Is it worth repairing?"), [
            ("A veces no, y te lo decimos. Como regla orientativa, suele compensar cuando la reparación cuesta bastante menos que un aparato nuevo parecido y el tuyo no es muy antiguo.",
             "Sometimes it is not, and we will tell you. As a rule of thumb, a repair is usually worth it when it costs well under a similar new appliance and yours is not very old."),
            ("Te damos el precio de la reparación y tú decides. Si prefieres no repararlo, solo pagas el desplazamiento y el diagnóstico.",
             "We give you the price of the repair and you decide. If you prefer not to go ahead, you only pay the call-out and diagnosis.")],
            eyebrow=("Reparar o cambiar", "Repair or replace"), tono="relleno",
            imagen_=("photo", ("Mano señalando el panel de un horno abierto", "Hand pointing at the panel of an open oven"))),
        bloque_opiniones(("Lo que dicen nuestros clientes", "What our customers say"),
                         [("la reparación", "the repair"), ("el aviso de llegada", "the arrival notice"), ("el precio", "the price")],
                         tono="base"),
        bloque_cta(("¿Qué aparato te da guerra?", "Which appliance is giving you trouble?"),
                   ("Dinos cuál es y qué hace, y buscamos hueco para ir a verlo.",
                    "Tell us which one it is and what it does, and we will find a slot to come and see it.")),
    ])

    rep = pagina("reparaciones", ("reparaciones", "repairs"), ("Qué reparamos y tarifas", "What we repair and rates"),
                 ("Averías habituales por aparato, cuándo compensa reparar, tarifas de ejemplo, garantía de la reparación y preguntas frecuentes.",
                  "Common faults by appliance, when a repair is worth it, example rates, repair warranty and frequently asked questions."), [
        hero(("Qué reparamos y cuánto cuesta", "What we repair and what it costs"),
             ("Las averías más habituales, cuándo compensa arreglarlas y las tarifas, con el desplazamiento y la mano de obra por separado.",
              "The most common faults, when they are worth fixing and our rates, with call-out and labour shown separately."),
             [pedir()], variante="texto"),
        bloque_checks(("Compensa reparar si...", "A repair is usually worth it if..."), [
            ("El aparato tiene pocos años y su reparación cuesta mucho menos que uno nuevo.",
             "The appliance is fairly new and the repair costs much less than a new one."),
            ("La avería es una pieza concreta: una bomba, una resistencia, un termostato, una goma.",
             "The fault is one specific part: a pump, a heating element, a thermostat, a seal."),
            ("Es un aparato bien construido, hay repuestos y, aparte de esta avería, sigue funcionando bien.",
             "It is a well-built appliance, spare parts are available and, apart from this fault, it still works well."),
            ("Es un modelo de calidad que, por lo demás, funciona bien.",
             "It is a good-quality model that otherwise works well."),
        ], eyebrow=("Reparar o cambiar", "Repair or replace"), cols=2, anchor="compensa", tono="base"),
        bloque_checks(("Mejor pensar en cambiarlo si...", "Better to think about replacing it if..."), [
            ("El aparato es muy antiguo y ya se han roto varias piezas.", "The appliance is very old and several parts have already failed."),
            ("La pieza que falla es la más cara y ya no se fabrica.", "The failed part is the most expensive one and is no longer made."),
            ("La reparación se acerca al precio de uno nuevo.", "The repair comes close to the price of a new one."),
            ("Se ha dañado el motor o el circuito principal y el aparato ya daba problemas.",
             "The motor or main circuit has failed and the appliance was already giving trouble."),
        ], cols=2, tono="relleno",
            intro=("Te lo decimos con franqueza, aunque eso signifique no reparar. Tampoco te cobramos más por decírtelo.",
                   "We tell you frankly, even if that means no repair. And we do not charge extra for saying so.")),
        bloque_precios(("Tarifas", "Rates"), ("Tarifas de visita y reparación", "Call-out and repair rates"),
                       [("Concepto", "Item"), ("Qué incluye", "What it includes"), ("Precio", "Price")],
                       [(("Desplazamiento", "Call-out"),
                         ("Ir a tu casa dentro de la zona de servicio", "Coming to your home within the service area"), eur(25)),
                        (("Diagnóstico", "Diagnosis"),
                         ("Revisar el aparato y darte la causa y el presupuesto. Si reparas con nosotros, se descuenta.",
                          "Checking the appliance and giving you the cause and a quote. Deducted if you have the repair done with us."), eur(30)),
                        (("Mano de obra, primera media hora", "Labour, first half hour"),
                         ("Reparación sencilla en la misma visita", "A simple repair on the same visit"), eur(35)),
                        (("Cada media hora más", "Each additional half hour"),
                         ("Reparaciones que llevan más tiempo", "Repairs that take longer"), eur(20)),
                        (("Segunda visita por pieza pedida", "Second visit for an ordered part"),
                         ("Volver a montar la pieza cuando llega", "Coming back to fit the part when it arrives"), eur(25)),
                        (("Visita en sábado", "Saturday visit"),
                         ("Solo con cita previa", "By appointment only"), desde(15))],
                       nota=NOTA_TARIFAS, anchor="tarifas", tono="base",
                       intro=("El precio final depende de la avería y de la pieza. Antes de empezar te damos el total y no seguimos sin tu visto bueno.",
                              "The final price depends on the fault and the part. Before we start we give you the total and do not carry on without your go-ahead.")),
        bloque_checks(("Garantía de la reparación", "Repair warranty"), [
            ("Garantía de ejemplo: 3 meses en la mano de obra y en las piezas que cambiemos. Cámbiala por la tuya.",
             "Example warranty: 3 months on labour and on the parts we replace. Replace it with your own."),
            ("El plazo y lo que cubre van por escrito en la factura.", "The period and what it covers are written on the invoice."),
            ("Si la misma avería se repite en ese plazo, vuelves a llamarnos y lo revisamos.",
             "If the same fault comes back within that period, call us again and we will look at it."),
            ("Una avería distinta, o un golpe o mal uso, no entra en la garantía de la reparación.",
             "A different fault, or damage from a knock or misuse, is not covered by the repair warranty."),
        ], cols=2, tono="relleno", anchor="garantia",
            intro=("Esta garantía se suma a la que la ley da al aparato por su compra, no la sustituye.",
                   "This warranty comes on top of the one the law gives the appliance as bought; it does not replace it.")),
        bloque_faq(("Preguntas frecuentes", "Frequently asked questions"), [
            (("¿Cuánto tardáis en venir?", "How soon can you come?"),
             ("Depende de la agenda de la semana. Al llamar te damos el primer hueco libre y una franja horaria.",
              "It depends on the week's schedule. When you call we give you the first free slot and a time slot.")),
            (("¿Lleváis las piezas en la furgoneta?", "Do you carry parts in the van?"),
             ("Las más comunes, sí. Si hace falta una que no llevamos, la pedimos y volvemos cuando llega.",
              "The common ones, yes. If we need one we do not carry, we order it and come back when it arrives.")),
            (("¿Y si decido no repararlo?", "What if I decide not to repair it?"),
             ("Pagas solo el desplazamiento y el diagnóstico. No hay ninguna obligación de seguir adelante.",
              "You only pay the call-out and diagnosis. There is no obligation to go ahead.")),
            (("¿Qué datos os hacen falta?", "What details do you need?"),
             ("La marca, el modelo y lo que hace el aparato. El modelo suele venir en una etiqueta dentro de la puerta o detrás.",
              "The make, the model and what the appliance does. The model is usually on a label inside the door or on the back.")),
            (("¿Dais factura?", "Do you give an invoice?"),
             ("Sí, siempre, con el detalle de piezas y mano de obra.", "Yes, always, itemising parts and labour.")),
        ], tono="base"),
        bloque_cta(("¿Lo vemos esta semana?", "Shall we look at it this week?"), None, [pedir(), llamar("secondary")], tono="relleno"),
    ])

    cont = pagina("contacto", ("contacto", "contact"), ("Pedir visita", "Book a visit"),
                  ("Pide visita a domicilio por teléfono, WhatsApp o formulario, con la marca y el modelo del aparato, y mira dónde estamos.",
                   "Book a home visit by phone, WhatsApp or the form, with the make and model of the appliance, and see where we are."), [
        hero(("Pide tu visita", "Book your visit"),
             ("Cuéntanos qué aparato es, qué hace y qué marca y modelo tiene. Te llamamos para darte día y franja.",
              "Tell us which appliance it is, what it does and its make and model. We call you with a day and time slot."),
             [llamar(), whatsapp()], variante="texto"),
        bloque_contacto(("Cómo localizarnos", "How to reach us"),
                        ("Atendemos en la zona y alrededores. Si no estás seguro de que lleguemos, pregúntanos.",
                         "We cover the local area and surroundings. If you are not sure we can reach you, ask us."),
                        campos_cita(servicios=aparatos, mensaje=True,
                                    extra=[campo("marca", "text", ("Marca del aparato", "Make of the appliance"), False, maxlen=60),
                                           campo("modelo", "text", ("Modelo (suele estar en la etiqueta)", "Model (usually on the label)"), False, maxlen=60),
                                           campo("edad", "select", ("¿Cuántos años tiene?", "How old is it?"), False,
                                                 opciones=[("Menos de 5 años", "Under 5 years"), ("Entre 5 y 10 años", "5 to 10 years"),
                                                           ("Más de 10 años", "Over 10 years"), ("No lo sé", "I do not know")])]),
                        ("Pedir visita", "Request a visit"), horas,
                        notas=("Usamos estos datos solo para preparar la visita y localizar la pieza.",
                               "We only use these details to prepare the visit and find the part."),
                        exito=("Gracias. Te llamamos para darte el día y la franja de la visita.",
                               "Thank you. We will call you to give you the day and time slot of the visit.")),
        bloque_mapa(("Desde dónde salimos", "Where we set out from"),
                    ("Esta es nuestra base. Vamos a domicilio en la zona de servicio.",
                     "This is our base. We make home visits within the service area."), tono="relleno"),
    ])

    return plantilla(KEY, ("Servicio técnico de electrodomésticos", "Appliance repair service"),
                     ("Reparación a domicilio de lavadoras, frigoríficos, hornos, lavavajillas y secadoras, con tarifas de ejemplo, garantía y cita con marca y modelo.",
                      "Home repairs for washing machines, fridges, ovens, dishwashers and tumble dryers, with example rates, warranty and booking with make and model."),
                     "oficios", ("servicio tecnico electrodomesticos reparacion lavadoras frigorificos hornos lavavajillas secadoras tecnico a domicilio sat averia reparar electrodomestico lavadora frigo",
                                 "appliance repair washing machine fridge oven dishwasher dryer technician home visit engineer repair service"),
                     look, cabecera(nav, cta_cab),
                     pie(nav, ("Servicio técnico a domicilio con diagnóstico y presupuesto antes de reparar.",
                               "Home repair service with a diagnosis and a quote before any repair."),
                         ("Lunes a viernes 9:00 a 14:00 y 16:00 a 19:00. Sábado con cita.", "Monday to Friday 9 am to 2 pm and 4 pm to 7 pm. Saturday by appointment.")),
                     [inicio, rep, cont])
