# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
from lib import *

KEY = "placas-solares"

# Como COLEGIADO: el 00-0000 lo detecta domain.TextosDemo y el sitio no se publica hasta cambiarlo.
INSTALADOR = ("Empresa instaladora autorizada. Nº de instalador autorizado: 00-0000",
              "Authorised installation company. Authorised installer no.: 00-0000")

# Azul de panel y amarillo de sol: fondo oscuro azulado y primario amarillo. Propia de este sector.
SOL = dict(
    colores=dict(background="#0b1730", surface="#132447", text="#f4f7fd", muted="#b8c4dc", primary="#ffc43d",
                 on_primary="#0b1730", border="#27396a", inverse="#050d1f", on_inverse="#f4f7fd"),
    fuentes=("manrope", "ibm-plex-sans"), radio=(4, 8, 12), peso=800, h1=(50, 40, 31), ancho=1120)


def build():
    nueva(KEY)
    look = SOL
    nav = [("inicio", ("Inicio", "Home")), ("proceso", ("Proceso y ayudas", "Process and grants")),
           ("estudio", ("Estudio gratuito", "Free study"))]
    horas = [(("Lunes a viernes", "Monday to Friday"), ("9:00 a 18:00", "9:00 am to 6:00 pm")),
             (("Sábado y domingo", "Saturday and Sunday"), ("Cerrado", "Closed")),
             (("Averías de una instalación nuestra", "Faults on one of our installations"),
              ("Llama en horario de oficina", "Call during office hours"))]
    cta_cab = marco([llamar(t=("Llamar", "Call")), whatsapp("secondary", t=("WhatsApp", "WhatsApp"))],
                    dir="row", gap=8, wrap=True)
    estudio = lambda v="primary": boton(("Pedir estudio gratuito", "Ask for a free study"), a_pagina("estudio", "contacto"), v)

    inicio = pagina("inicio", ("", ""), ("Placas solares", "Solar panels"),
                    ("Instalación de placas solares para autoconsumo en viviendas y pequeños negocios: estudio gratuito, trámites, instalación y legalización.",
                     "Solar panel installation for self-consumption in homes and small businesses: free study, paperwork, installation and registration."), [
        hero(("Placas solares para tu casa o tu negocio", "Solar panels for your home or business"),
             ("Estudiamos tu tejado y tu consumo, te presentamos un presupuesto claro y nos ocupamos del proyecto, de los trámites, de la instalación y de la legalización.",
              "We study your roof and your electricity use, give you a clear quote and take care of the design, the paperwork, the installation and the registration."),
             [estudio(), llamar("secondary"), whatsapp("ghost")], variante="centro", tono="oscuro",
             eyebrow=("Autoconsumo fotovoltaico", "Solar self-consumption"),
             extra=peq(INSTALADOR)),
        bloque_valores(("Para quién instalamos", "Who we install for"), [
            (("Viviendas", "Homes"),
             ("Casas con tejado propio y también pisos con acceso a la cubierta, si la comunidad lo permite.",
              "Houses with their own roof and flats with access to the roof, if the community of owners agrees.")),
            (("Pequeños negocios", "Small businesses"),
             ("Tiendas, bares, oficinas y talleres que gastan más luz de día, cuando hay sol.",
              "Shops, bars, offices and workshops that use more electricity during the day, when the sun is out.")),
            (("Naves y cubiertas planas", "Warehouses and flat roofs"),
             ("Estructura sobre cubierta plana, de chapa o de teja. Lo vemos en la visita.",
              "Frames on flat, metal or tiled roofs. We check on the visit.")),
        ], eyebrow=("Qué instalamos", "What we install"), estilo="relleno", tono="base"),
        bloque_pasos(("Cómo es el proceso", "How the process works"), [
            (("Estudio", "Study"),
             ("Vemos tu factura, tu tejado y la sombra que le cae. Es gratuito y no te obliga a nada.",
              "We look at your bill, your roof and the shade it gets. It is free and puts you under no obligation.")),
            (("Proyecto y trámites", "Design and paperwork"),
             ("Preparamos el proyecto y los permisos con el ayuntamiento y la distribuidora.",
              "We prepare the design and the permits with the town hall and the grid operator.")),
            (("Instalación", "Installation"),
             ("Colocamos paneles, estructura, inversor y cableado. Te decimos antes cuántos días lleva.",
              "We fit panels, frames, inverter and cabling. We tell you beforehand how many days it takes.")),
            (("Legalización y puesta en marcha", "Registration and start-up"),
             ("Legalizamos la instalación, te explicamos cómo ver lo que produces y te damos garantía por escrito.",
              "We register the installation, show you how to see what you produce and give you a written warranty.")),
        ], tono="relleno"),
        bloque_checks(("Un presupuesto que se entiende", "A quote you can follow"), [
            ("Cada partida por separado: paneles, inversor, estructura, mano de obra y trámites.", "Each item listed separately: panels, inverter, structure, labour and paperwork."),
            ("Te decimos qué incluye y qué no, y qué hace falta si hay que reforzar o cambiar algo.", "We tell you what is and is not included, and what is needed if anything must be reinforced or replaced."),
            ("No te prometemos cifras mágicas: te enseñamos cómo se estima la producción y qué supuestos usamos.",
             "We do not promise miracle figures: we show you how production is estimated and which assumptions we use."),
            ("Factura con IVA y garantía de equipos e instalación por escrito.", "An invoice with VAT and a written warranty on equipment and installation."),
        ], eyebrow=("Presupuesto", "Quote"), anchor="presupuesto"),
        bloque_opiniones(("Lo que dicen nuestros clientes", "What our customers say"),
                         [("el estudio previo", "the initial study"), ("los trámites", "the paperwork"), ("la instalación", "the installation")]),
        bloque_cta(("¿Quieres saber si tu tejado sirve?", "Want to know if your roof is suitable?"),
                   ("Cuéntanos tu consumo y qué tipo de tejado tienes y te decimos cómo seguir.",
                    "Tell us how much electricity you use and what kind of roof you have and we will tell you what happens next."),
                   [estudio(), whatsapp("ghost")], tono="oscuro"),
    ])

    proceso = pagina("proceso", ("proceso-y-ayudas", "process-and-grants"), ("Proceso y ayudas", "Process and grants"),
                     ("Las seis fases de una instalación fotovoltaica, baterías, mantenimiento, ayudas y subvenciones, y preguntas habituales.",
                      "The six stages of a solar installation, batteries, maintenance, grants and subsidies, and common questions."), [
        hero(("Del estudio a la legalización", "From the study to registration"),
             ("Te contamos cada fase en llano, qué hacemos nosotros y qué trámites dependen de otros.",
              "We explain each stage in plain terms, what we do and which steps depend on others."),
             [estudio(), llamar("secondary")], variante="texto"),
        bloque_pasos(("Las seis fases", "The six stages"), [
            (("Estudio gratuito", "Free study"),
             ("Con tu factura de la luz, fotos del tejado y su orientación vemos qué cabe y cuánto sol recibe.",
              "With your electricity bill, photos of the roof and its orientation, we see what fits and how much sun it gets.")),
            (("Propuesta y presupuesto", "Proposal and quote"),
             ("Te damos el número de paneles, el inversor, el precio por escrito y los supuestos de producción.",
              "We give you the number of panels, the inverter, the price in writing and the production assumptions.")),
            (("Proyecto y trámites", "Design and paperwork"),
             ("Redactamos el proyecto o la memoria técnica y pedimos la licencia o comunicación al ayuntamiento.",
              "We write the design or technical report and file the licence or notification with the town hall.")),
            (("Instalación", "Installation"),
             ("Montamos la estructura, los paneles, el inversor y el cableado, y dejamos el cuadro protegido.",
              "We build the frame and fit the panels, inverter and cabling, and leave the fuse box protected.")),
            (("Legalización", "Registration"),
             ("Presentamos la documentación para legalizar la instalación y, si procede, para la compensación de excedentes.",
              "We submit the paperwork to register the installation and, where it applies, the surplus compensation.")),
            (("Seguimiento", "Follow-up"),
             ("Te enseñamos a ver lo que produces en el móvil y te avisamos de cuándo conviene revisar.",
              "We show you how to see what you produce on your phone and tell you when a check is worthwhile.")),
        ], tono="relleno", anchor="fases"),
        bloque_texto(("Baterías: ¿las necesitas?", "Batteries: do you need them?"), [
            ("Una batería guarda la energía que producen los paneles para usarla por la tarde o por la noche. No siempre compensa: depende de cuándo gastas luz.",
             "A battery stores the energy your panels produce so you can use it in the evening or at night. It is not always worthwhile: it depends on when you use electricity."),
            ("Te presupuestamos la instalación con batería y sin ella, y te explicamos la diferencia para tu caso. Se puede añadir más adelante si lo dejas previsto.",
             "We quote the installation with and without a battery and explain the difference for your case. It can be added later if you plan for it.")],
            eyebrow=("Baterías", "Batteries"), anchor="baterias",
            imagen_=("photo", ("Batería doméstica instalada junto al inversor", "Home battery fitted next to the inverter"))),
        bloque_checks(("Mantenimiento", "Maintenance"), [
            ("Limpieza de paneles cuando haga falta, según polvo, hojas o aves.", "Panel cleaning when needed, depending on dust, leaves or birds."),
            ("Revisión del inversor y de sus avisos.", "A check of the inverter and its alerts."),
            ("Comprobación de la estructura, los anclajes y el cableado.", "A check of the frame, fixings and cabling."),
            ("Lectura de lo que ha producido la instalación y comparación con lo previsto.", "A reading of what the installation has produced compared with what was expected."),
        ], intro=("Una instalación fotovoltaica necesita poco mantenimiento, pero conviene mirarla de vez en cuando. Te ofrecemos una revisión periódica opcional.",
                  "A solar installation needs little maintenance, but it is worth checking now and then. We offer an optional periodic check."),
            eyebrow=("Después de instalar", "After installation"), anchor="mantenimiento", tono="relleno"),
        bloque_texto(("Subvenciones y ayudas", "Grants and subsidies"), [
            ("Pueden existir ayudas, bonificaciones fiscales o deducciones para instalar placas solares. Cambian según el momento, tu comunidad autónoma y tu ayuntamiento.",
             "There may be grants, tax rebates or deductions for installing solar panels. They change with the date, your region and your town hall."),
            ("Consulta las vigentes antes de decidir. Si quieres, te ayudamos con la documentación que nos corresponda, pero no podemos asegurarte que te concedan una ayuda ni su importe.",
             "Check the ones in force before you decide. If you want, we help with the paperwork that is ours to provide, but we cannot assure you that a grant will be awarded or its amount.")],
            eyebrow=("Ayudas", "Grants"), anchor="ayudas"),
        bloque_faq(("Preguntas frecuentes", "Frequently asked questions"), [
            (("¿Cuánto tarda todo el proceso?", "How long does the whole process take?"),
             ("Depende de los permisos del ayuntamiento y de la distribuidora, que no controlamos. Tras el estudio te damos plazos aproximados.",
              "It depends on permits from the town hall and the grid operator, which we do not control. After the study we give you approximate timescales.")),
            (("¿Puedo ponerlas si vivo en un piso?", "Can I fit them if I live in a flat?"),
             ("Si tienes acceso a la cubierta o a una zona común, hace falta el acuerdo de la comunidad. Te decimos qué hay que pedir.",
              "If you have access to the roof or a shared area, the community of owners must agree. We tell you what to ask for.")),
            (("¿Y si no hay sol un día?", "What if there is no sun one day?"),
             ("Sigues conectado a la red y tomas de ella lo que no produzcan los paneles.",
              "You stay connected to the grid and take from it whatever the panels do not produce.")),
            (("¿Qué garantía tiene?", "What warranty does it have?"),
             ("La de los equipos, que da el fabricante, y la de la instalación, que damos nosotros. Las dos van por escrito en el presupuesto.",
              "The one on the equipment, given by the manufacturer, and the one on the installation, given by us. Both are in writing in the quote.")),
            (("¿Me pagan por la energía que no uso?", "Am I paid for the energy I do not use?"),
             ("Si vuelcas excedentes a la red, existe una compensación en la factura. Cómo funciona depende de tu contrato: te lo explicamos en el estudio.",
              "If you feed surplus into the grid, there is a credit on your bill. How it works depends on your contract: we explain it in the study.")),
        ]),
    ])

    est = pagina("estudio", ("estudio-gratuito", "free-study"), ("Estudio gratuito", "Free study"),
                 ("Pide el estudio gratuito de tu tejado y tu consumo: formulario, WhatsApp, teléfono, horario y zonas donde instalamos.",
                  "Ask for the free study of your roof and electricity use: form, WhatsApp, phone, opening hours and the areas where we install."), [
        hero(("Estudio gratuito de tu tejado y tu consumo", "Free study of your roof and electricity use"),
             ("Cuéntanos cuánta luz gastas y qué tejado tienes. Te llamamos para pedirte la factura y concretar una visita si hace falta.",
              "Tell us how much electricity you use and what kind of roof you have. We will call you to ask for your bill and arrange a visit if needed."),
             [llamar(), whatsapp()], variante="texto"),
        bloque_checks(("Qué conviene tener a mano", "What is useful to have to hand"), [
            ("Una factura de la luz reciente, con el consumo y la potencia contratada.", "A recent electricity bill, showing consumption and contracted power."),
            ("Fotos del tejado o de la cubierta desde fuera.", "Photos of the roof from outside."),
            ("Hacia dónde mira el tejado, si lo sabes. Si no, lo vemos nosotros.", "Which way the roof faces, if you know. If not, we will find out."),
            ("Si el inmueble es tuyo o de la comunidad.", "Whether the property is yours or the community's."),
        ], tono="relleno", eyebrow=("Antes de escribirnos", "Before you write"), anchor="a-mano"),
        bloque_contacto(("Pide tu estudio", "Ask for your study"),
                        ("Rellena lo que sepas. Lo demás lo hablamos por teléfono. No te comprometes a nada.",
                         "Fill in what you know. We will cover the rest by phone. There is no obligation."),
                        campos_cita(servicios=[("Vivienda unifamiliar", "Detached or terraced house"), ("Piso con acceso a cubierta", "Flat with roof access"),
                                               ("Pequeño negocio", "Small business"), ("Nave o cubierta industrial", "Warehouse or industrial roof")],
                                    fecha=False,
                                    extra=[campo("tejado", "select", ("Tipo de tejado", "Type of roof"), False,
                                                 opciones=[("Teja inclinada", "Pitched tiles"), ("Azotea o cubierta plana", "Flat roof"),
                                                           ("Chapa o panel sándwich", "Metal or sandwich panel"), ("No lo sé", "I do not know")]),
                                           campo("consumo", "text", ("Consumo anual en kWh o importe medio de la factura", "Yearly use in kWh or average bill"), False, maxlen=60),
                                           campo("poblacion", "text", ("Población", "Town"), True, maxlen=120),
                                           campo("bateria", "select", ("¿Te interesa una batería?", "Are you interested in a battery?"), False,
                                                 opciones=[("Sí", "Yes"), ("No", "No"), ("No lo sé todavía", "Not sure yet")])]),
                        ("Pedir estudio gratuito", "Ask for a free study"), horas,
                        exito=("Gracias. Te llamamos en horario de oficina para pedirte la factura y concretar una visita.",
                               "Thank you. We will call you during office hours to ask for your bill and arrange a visit."),
                        notas=INSTALADOR, tono="relleno"),
        bloque_checks(("Dónde instalamos", "Where we install"), [
            ("Tu ciudad y su área metropolitana", "Your city and its surrounding area"),
            ("Urbanizaciones y zonas de casas de alrededor", "Surrounding housing estates and house areas"),
            ("Polígonos y zonas de negocios", "Industrial estates and business areas"),
            ("Pueblos cercanos (consúltanos)", "Nearby villages (ask us)"),
        ], intro=("Si tu zona no aparece, llámanos y te decimos si llegamos.", "If your area is not listed, call us and we will tell you if we can get there."),
            eyebrow=("Zonas de servicio", "Service areas"), anchor="zonas"),
        bloque_mapa(("Dónde está la oficina", "Where the office is"),
                    ("Vamos a ver tu tejado, no hace falta que vengas. Esta es nuestra oficina.",
                     "We come to see your roof, so there is no need to visit. This is our office."), tono="relleno"),
    ])

    return plantilla(KEY, ("Instalación de placas solares", "Solar panel installation"),
                     ("Autoconsumo fotovoltaico para viviendas y pequeños negocios con proceso en fases, baterías, mantenimiento, ayudas, preguntas frecuentes y formulario de estudio gratuito.",
                      "Solar self-consumption for homes and small businesses with a staged process, batteries, maintenance, grants, FAQs and a free-study form."),
                     "oficios", ("placas solares paneles solares fotovoltaica autoconsumo instalador energia solar tejado inversor baterias subvenciones ayudas estudio gratuito",
                                 "solar panels photovoltaic self-consumption installer solar energy roof inverter batteries grants free study"),
                     look, cabecera(nav, cta_cab),
                     pie(nav, ("Placas solares para autoconsumo en viviendas y pequeños negocios.",
                               "Solar panels for self-consumption in homes and small businesses."),
                         ("Lunes a viernes de 9:00 a 18:00.", "Monday to Friday 9 am to 6 pm.")),
                     [inicio, proceso, est])
