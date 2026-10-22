# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
from lib import *

KEY = "compraventa-coches"

# Sala de exposición en oscuro: grafito azulado con verde esmeralda. Se comprueba AA al construir.
SALA = dict(background="#101418", surface="#192027", text="#eef1f4", muted="#a9b4bf", primary="#34d399",
            on_primary="#0b1511", border="#2b353e", inverse="#070a0d", on_inverse="#eef1f4")

TEXTO_FIN = ("Texto de ejemplo: escribe aquí las condiciones reales de tu financiación (entidad, plazos, entrada) o borra la sección si no financias.",
             "Sample text: write the real terms of your financing (lender, terms, deposit) or delete the section if you do not offer it.")


def coche_demo(i):
    return tarjeta([
        imagen("photo", (f"Foto de un coche de ocasión de la exposición, imagen {i}", f"Photo of a used car in the showroom, image {i}"), ratio=1.4),
        h3(("Marca y modelo", "Make and model")),
        p(("Año, kilómetros y combustible", "Year, mileage and fuel"), color="$muted"),
        peq(("Vehículo de ejemplo: sustitúyelo por uno de tu stock. Precio: consultar.",
             "Sample vehicle: replace it with one from your stock. Price: ask us."), color="$muted"),
        boton(("Me interesa", "I am interested"), a_pagina("contacto", "contacto"), "secondary"),
    ], estilo="borde", pad=16, gap=10)


def build():
    malos = comprobar_paleta("sala", SALA)
    assert not malos, malos
    nueva(KEY)
    look = dict(colores=SALA, fuentes=("manrope", "work-sans"), radio=(6, 10, 16), peso=800, h1=(50, 40, 32))
    nav = [("inicio", ("Inicio", "Home")), ("comprar", ("Comprar y garantía", "Buying and warranty")),
           ("vender", ("Vende tu coche", "Sell your car")), ("contacto", ("Contacto", "Contact"))]
    horas = [(("Lunes a viernes", "Monday to Friday"),
              ("10:00 a 14:00 y 16:30 a 20:00", "10:00 am to 2:00 pm and 4:30 pm to 8:00 pm")),
             (("Sábado", "Saturday"), ("10:00 a 14:00", "10:00 am to 2:00 pm")),
             (("Domingo", "Sunday"), ("Cerrado", "Closed"))]
    cta_cab = marco([llamar(t=("Llamar", "Call")), whatsapp("secondary", t=("WhatsApp", "WhatsApp"))],
                    dir="row", gap=8, wrap=True)
    ver = lambda v="primary": boton(("Ver un coche", "See a car"), a_pagina("contacto", "contacto"), v)
    tasar = lambda v="secondary": boton(("Tasar mi coche", "Value my car"), a_pagina("vender", "tasacion"), v)

    inicio = pagina("inicio", ("", ""), ("Compraventa de coches", "Used car dealer"),
                    ("Coches de ocasión revisados antes de venderlos, garantía legal explicada, tasación de tu coche y gestión del cambio de titularidad.",
                     "Used cars checked before they are sold, the legal warranty explained, a valuation of your car and help with the change of ownership."), [
        hero(("Coches de ocasión revisados, con los papeles en regla", "Used cars checked, with the paperwork in order"),
             ("Antes de ponerlo a la venta lo revisamos. Te enseñamos lo que hemos mirado, te explicamos la garantía y nos ocupamos del cambio de titularidad.",
              "Before a car goes on sale we check it. We show you what we looked at, explain the warranty and handle the change of ownership."),
             [ver(), tasar(), llamar("ghost")], variante="centro",
             eyebrow=("Compraventa de coches", "Used car dealer")),
        seccion("features", [
            titular(("Algunos de nuestros coches", "A few of our cars"),
                    ("Estas fichas son un modelo. Sustitúyelas por tus coches con su foto, año y kilómetros.",
                     "These cards are a template. Replace them with your cars, each with a photo, year and mileage."),
                    eyebrow=("Vehículos destacados", "Featured vehicles")),
            rejilla([coche_demo(i) for i in (1, 2, 3)], cols=3),
        ], anchor="destacados", tono="relleno"),
        bloque_pasos(("Comprar un coche aquí, paso a paso", "Buying a car here, step by step"), [
            (("Eliges y lo ves", "You choose and see it"),
             ("Vienes a verlo, lo pruebas y te contamos su historia: quién lo tenía, qué mantenimiento lleva.",
              "You come to see it, test drive it and we tell you its history: who owned it and what servicing it has had.")),
            (("Revisas la documentación", "You check the paperwork"),
             ("Te enseñamos el informe del vehículo, el libro de mantenimiento y el resultado de nuestra revisión.",
              "We show you the vehicle report, the service book and the result of our check.")),
            (("Firmas el contrato", "You sign the contract"),
             ("Por escrito, con el precio final, la garantía y la fecha de entrega. Lees antes de firmar.",
              "In writing, with the final price, the warranty and the delivery date. You read before you sign.")),
            (("Te lo llevas a tu nombre", "You drive away, in your name"),
             ("Hacemos el cambio de titularidad en Tráfico y te entregamos las llaves, la documentación y la factura.",
              "We do the change of ownership with the traffic authority and hand you the keys, the documents and the invoice.")),
        ], tono="base", anchor="como-comprar"),
        bloque_checks(("Lo que no se queda sin decir", "What never goes unsaid"), [
            ("Los kilómetros, comprobados con el historial de mantenimiento y el informe de Tráfico.",
             "The mileage, checked against the service history and the traffic authority report."),
            ("Las averías o golpes que conozcamos, por escrito antes de que decidas.",
             "Any faults or knocks we know of, in writing before you decide."),
            ("El precio final con todo incluido, sin sumas al firmar.", "The final price with everything included, with nothing added at signing."),
            ("La garantía legal explicada en el contrato, con su plazo y lo que cubre.",
             "The legal warranty set out in the contract, with its period and what it covers."),
        ], eyebrow=("Claridad", "Clarity"), tono="relleno", cols=2),
        bloque_opiniones(("Lo que dicen quienes nos han comprado", "What our buyers say"),
                         [("la compra", "the purchase"), ("la información del coche", "the car information"), ("la entrega", "the handover")],
                         tono="base"),
        bloque_cta(("¿Buscas un coche concreto?", "Looking for a particular car?"),
                   ("Dinos qué necesitas y para qué lo vas a usar, y te avisamos cuando entre uno que encaje.",
                    "Tell us what you need and what you will use it for, and we will let you know when a suitable one comes in."),
                   [ver(), llamar("secondary")], tono="relleno"),
    ])

    comprar = pagina("comprar", ("comprar-y-garantia", "buying-and-warranty"), ("Comprar y garantía", "Buying and warranty"),
                     ("Cómo comprar un coche de ocasión, la revisión previa, la garantía legal en llano, la financiación y el cambio de titularidad.",
                      "How to buy a used car, the pre-sale check, the legal warranty in plain words, financing and the change of ownership."), [
        hero(("Comprar un coche de ocasión", "Buying a used car"),
             ("Lo que revisamos, qué garantía tienes por ley, cómo se paga y quién hace los papeles. Si algo no queda claro, pregúntanos antes de firmar.",
              "What we check, what warranty you have by law, how you pay and who does the paperwork. If anything is unclear, ask us before you sign."),
             [ver(), llamar("secondary")], variante="texto"),
        bloque_texto(("La revisión previa", "The pre-sale check"), [
            ("Cada coche pasa por el taller antes de salir a la venta. Miramos neumáticos, frenos, luces, líquidos, escape, amortiguación, batería y las luces del cuadro de mandos.",
             "Every car goes through the workshop before it is put on sale. We look at tyres, brakes, lights, fluids, exhaust, suspension, battery and the dashboard warning lights."),
            ("Conectamos la máquina de diagnosis, hacemos una prueba de carretera y te damos el resultado por escrito. Lo que arreglamos y lo que no, queda anotado.",
             "We connect the diagnostic tool, do a road test and give you the result in writing. What we fixed and what we did not is noted down.")],
            eyebrow=("Revisión", "Inspection"), anchor="revision", imagen_=("photo", ("Mecánico revisando los frenos de un coche de ocasión", "Mechanic checking the brakes of a used car"))),
        bloque_texto(("La garantía legal, en llano", "The legal warranty, in plain words"), [
            ("Cuando compras un coche a un profesional, la ley te da una garantía. Su plazo y lo que cubre los fija la ley, no nosotros, y los ponemos en el contrato con la fecha de entrega.",
             "When you buy a car from a trader, the law gives you a warranty. Its period and what it covers are set by law, not by us, and we state them in the contract with the delivery date."),
            ("Cubre las faltas que ya existían cuando te entregamos el coche, no el desgaste normal del uso. Si aparece una, avísanos cuanto antes: se arregla, se sustituye o, si no es posible, la ley prevé rebajar el precio o devolverlo.",
             "It covers faults that already existed when we handed over the car, not normal wear and tear. If one appears, tell us as soon as you can: it is repaired, replaced or, if that is not possible, the law provides for a price reduction or a refund."),
            ("Esto es una explicación general, no asesoramiento legal. Lo que vale es lo que pone tu contrato y la ley vigente.",
             "This is a general explanation, not legal advice. What counts is what your contract says and the law in force.")],
            eyebrow=("Garantía", "Warranty"), tono="relleno", anchor="garantia"),
        bloque_texto(("Financiación", "Financing"), [TEXTO_FIN,
            ("Si financias, lees la oferta completa antes de firmar y te llevas una copia. Pedir información no te compromete a nada.",
             "If you finance, you read the full offer before you sign and keep a copy. Asking for information commits you to nothing.")],
            eyebrow=("Cómo pagarlo", "How to pay"), anchor="financiacion"),
        bloque_checks(("Cambio de titularidad", "Change of ownership"), [
            ("El coche pasa a tu nombre ante Tráfico. Lo gestionamos nosotros o te decimos cómo hacerlo.",
             "The car is put in your name with the traffic authority. We handle it or tell you how to do it."),
            ("Te detallamos por escrito las tasas e impuestos que correspondan y quién los paga.",
             "We list in writing the fees and taxes that apply and who pays them."),
            ("Tu DNI o NIE en vigor y, si es una empresa, su documentación.", "Your valid ID card or NIE and, for a company, its documents."),
            ("El permiso de circulación, la ficha técnica y la ITV, que te entregamos nosotros.",
             "The registration document, the technical data sheet and the ITV certificate, which we hand to you."),
            ("El contrato de compraventa firmado por las dos partes.", "The sales contract signed by both parties."),
        ], eyebrow=("Papeles", "Paperwork"), tono="relleno", anchor="titularidad"),
        bloque_faq(("Preguntas frecuentes", "Frequently asked questions"), [
            (("¿Puedo probar el coche?", "Can I test drive the car?"),
             ("Sí. Pídenos cita, trae tu carné y lo probamos juntos en una ruta corta.", "Yes. Book a slot, bring your licence and we test it together on a short route.")),
            (("¿Puedo llevarlo a mi mecánico antes de comprar?", "Can I take it to my own mechanic before buying?"),
             ("Claro. Lo acordamos y te dejamos que lo vea quien tú quieras.", "Of course. We arrange it and let whoever you choose look it over.")),
            (("¿Aceptáis mi coche como parte del pago?", "Do you take my car in part exchange?"),
             ("Lo tasamos y, si nos encaja, descontamos su valor del precio. Te lo ponemos por escrito.",
              "We value it and, if it suits us, deduct its value from the price. You get it in writing.")),
            (("¿Qué papeles me dan al comprar?", "What documents do I get when I buy?"),
             ("El contrato, la factura, el permiso de circulación, la ficha técnica, la ITV y el libro de mantenimiento, si lo hay.",
              "The contract, the invoice, the registration document, the technical data sheet, the ITV certificate and the service book, if there is one.")),
        ], tono="base"),
    ])

    vender = pagina("vender", ("vende-tu-coche", "sell-your-car"), ("Vende tu coche", "Sell your car"),
                    ("Tasación de tu coche: cómo se hace, qué necesitamos saber y formulario con marca, modelo, año y kilómetros.",
                     "A valuation of your car: how it works, what we need to know and a form with make, model, year and mileage."), [
        hero(("Cuánto vale tu coche", "What your car is worth"),
             ("Cuéntanos marca, modelo, año y kilómetros y te damos una tasación. Es sin compromiso: si no te cuadra, no pasa nada.",
              "Tell us the make, model, year and mileage and we give you a valuation. No obligation: if it does not suit you, that is fine."),
             [tasar("primary"), llamar("secondary")], variante="texto"),
        bloque_pasos(("Cómo tasamos", "How we value"), [
            (("Nos cuentas los datos", "You give us the details"),
             ("Con el formulario o por teléfono. Cuanto más exacto, más afinada será la tasación.", "With the form or by phone. The more accurate you are, the closer the valuation.")),
            (("Lo vemos en persona", "We see it in person"),
             ("Revisamos el estado, la carrocería, los neumáticos y la documentación.", "We check the condition, the bodywork, the tyres and the paperwork.")),
            (("Te damos el precio", "We give you a price"),
             ("Por escrito, con el motivo. Lo piensas el tiempo que necesites.", "In writing, with the reasons. You think it over as long as you need.")),
            (("Si aceptas, nos ocupamos", "If you accept, we deal with it"),
             ("Firmamos el contrato, hacemos el cambio de titularidad y te pagamos como acordemos.", "We sign the contract, do the change of ownership and pay you as agreed.")),
        ], tono="relleno"),
        seccion("contact", [
            titular(("Pide tu tasación", "Request your valuation"),
                    ("Te llamamos para confirmar el día en que podemos verlo.", "We call you to confirm a day to see it."), eyebrow=("Formulario", "Form")),
            tarjeta([formulario([
                campo("nombre", "text", ("Nombre", "Name"), True, auto="name", maxlen=120),
                campo("telefono", "tel", ("Teléfono", "Phone"), True, auto="tel", maxlen=30),
                campo("marca", "text", ("Marca", "Make"), True, maxlen=60),
                campo("modelo", "text", ("Modelo", "Model"), True, maxlen=80),
                campo("anio", "number", ("Año de matriculación", "Year of registration"), True),
                campo("kilometros", "number", ("Kilómetros", "Mileage"), True),
                campo("combustible", "select", ("Combustible", "Fuel"), False,
                      opciones=[("Gasolina", "Petrol"), ("Diésel", "Diesel"), ("Híbrido", "Hybrid"), ("Eléctrico", "Electric"), ("Otro", "Other")]),
                campo("mensaje", "textarea", ("Algo más que debamos saber (opcional)", "Anything else we should know (optional)"), False, maxlen=1000),
            ], ("Pedir tasación", "Request valuation"),
                exito=("Gracias. Te llamamos para fijar el día en que vemos tu coche.", "Thank you. We will call you to set a day to see your car."))],
                estilo="relleno", pad=28, tag="div", sz={"width": "fill", "height": "hug", "max_width": 720}),
            peq(("Usamos estos datos solo para preparar la tasación.", "We use these details only to prepare the valuation."), color="$muted"),
        ], anchor="tasacion", gap=24),
        bloque_faq(("Preguntas sobre vender", "Questions about selling"), [
            (("¿Qué documentos necesito?", "What documents do I need?"),
             ("Tu DNI o NIE, el permiso de circulación, la ficha técnica y la ITV en vigor. Si tiene cargas o está financiado, dínoslo antes.",
              "Your ID card or NIE, the registration document, the technical data sheet and a valid ITV. If there are charges on it or it is financed, tell us beforehand.")),
            (("¿Y si todavía lo estoy pagando?", "What if I am still paying for it?"),
             ("Se puede, pero hay que cancelar la deuda al vender. Te explicamos cómo se hace.", "It can be done, but the debt has to be cleared on sale. We explain how.")),
            (("¿Cuándo me pagáis?", "When do you pay me?"),
             ("Lo acordamos por escrito antes de firmar, junto con la forma de pago.", "We agree it in writing before signing, along with the way you are paid.")),
        ], tono="base"),
    ])

    cont = pagina("contacto", ("contacto", "contact"), ("Contacto", "Contact"),
                  ("Pide ver un coche, una tasación o información de financiación por teléfono, WhatsApp o formulario. Horario y cómo llegar.",
                   "Ask to see a car, a valuation or financing information by phone, WhatsApp or form. Opening hours and how to find us."), [
        hero(("Ven a ver tu próximo coche", "Come and see your next car"),
             ("Llámanos, escríbenos por WhatsApp o rellena el formulario. Te decimos qué hueco tenemos para enseñártelo con calma.",
              "Call us, message us on WhatsApp or fill in the form. We tell you which slot we have to show it to you properly."),
             [llamar(), whatsapp()], variante="texto"),
        bloque_contacto(("Cuéntanos qué buscas", "Tell us what you are after"),
                        ("Con unos datos te llamamos para concretar el día y la hora.", "With a few details we call you to set the day and time."),
                        campos_cita(servicios=[("Ver un coche", "See a car"), ("Tasar el mío", "Value mine"),
                                               ("Información de financiación", "Financing information"),
                                               ("Cambio de titularidad", "Change of ownership"), ("Otra consulta", "Other question")],
                                    extra=[campo("coche", "text", ("Coche que te interesa (opcional)", "Car you are interested in (optional)"), False, maxlen=120)]),
                        ("Enviar", "Send"), horas,
                        notas=("Los datos que nos des solo los usamos para contestarte.", "We only use the details you give us to reply."),
                        exito=("Gracias. Te llamamos para concretar la visita.", "Thank you. We will call you to arrange the visit.")),
        bloque_mapa(("Cómo llegar a la exposición", "How to get to the showroom"),
                    ("Tenemos aparcamiento para que dejes tu coche mientras ves los nuestros.",
                     "We have parking so you can leave your car while you look at ours."), tono="relleno"),
    ])

    return plantilla(KEY, ("Compraventa de coches", "Used car dealer"),
                     ("Compraventa de coches de ocasión con fichas de vehículos, pasos para comprar, garantía legal en llano, financiación, tasación con formulario y cambio de titularidad.",
                      "Used car dealer with vehicle cards, buying steps, the legal warranty in plain words, financing, a valuation form and change of ownership."),
                     "motor", ("compraventa de coches coches de ocasion coches de segunda mano concesionario venta de coches vender mi coche tasacion garantia financiacion cambio de titularidad km0 seminuevos",
                               "used car dealer second hand cars used cars car sales sell my car valuation warranty financing change of ownership dealership"),
                     look, cabecera(nav, cta_cab),
                     pie(nav, ("Coches de ocasión revisados, con garantía legal y ayuda con el cambio de titularidad.",
                               "Used cars checked, with the legal warranty and help with the change of ownership."),
                         ("Lunes a viernes 10:00 a 14:00 y 16:30 a 20:00. Sábado 10:00 a 14:00.",
                          "Monday to Friday 10 am to 2 pm and 4:30 to 8 pm. Saturday 10 am to 2 pm.")),
                     [inicio, comprar, vender, cont])
