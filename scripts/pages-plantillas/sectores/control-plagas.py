# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
from lib import *

KEY = "control-plagas"

# Como COLEGIADO: el 00-0000 lo detecta domain.TextosDemo y el sitio no se publica hasta cambiarlo.
BIOCIDAS = ("Empresa inscrita en el registro oficial de empresas de biocidas. Nº de inscripción: 00-0000",
            "Company entered in the official register of biocide companies. Registration no.: 00-0000")

# Gris azulado frío y naranja de aviso: propia de este sector.
AVISO = dict(
    colores=dict(background="#f6f7f9", surface="#e8eaee", text="#1a1d23", muted="#505762", primary="#b93a08",
                 on_primary="#ffffff", border="#d5d9e0", inverse="#0d0f13", on_inverse="#f3f4f6"),
    fuentes=("work-sans", "figtree"), radio=(4, 8, 12), peso=700, ancho=1120)


def plaga(nombre, senales, hacemos):
    return tarjeta([h3(nombre),
                    etiqueta(("Cómo saber que la tienes", "How to tell you have them")),
                    p(senales, color="$muted"),
                    etiqueta(("Qué hacemos", "What we do")),
                    p(hacemos)], estilo="borde", gap=10)


def build():
    nueva(KEY)
    look = AVISO
    nav = [("inicio", ("Inicio", "Home")), ("plagas", ("Plagas y tratamientos", "Pests and treatments")),
           ("negocios", ("Para negocios", "For businesses")), ("contacto", ("Contacto", "Contact"))]
    horas = [(("Lunes a viernes", "Monday to Friday"), ("8:00 a 19:00", "8:00 am to 7:00 pm")),
             (("Sábado", "Saturday"), ("9:00 a 13:00", "9:00 am to 1:00 pm")),
             (("Urgencias: avispas, roedores y chinches", "Emergencies: wasps, rodents and bed bugs"),
              ("Llama y te decimos cuándo podemos ir", "Call and we will tell you when we can come"))]
    cta_cab = marco([llamar(t=("Llamar", "Call")), whatsapp("secondary", t=("WhatsApp", "WhatsApp"))],
                    dir="row", gap=8, wrap=True)
    visita = lambda v="primary": boton(("Pedir visita y presupuesto", "Book a visit and quote"), a_pagina("contacto", "contacto"), v)

    inicio = pagina("inicio", ("", ""), ("Control de plagas", "Pest control"),
                    ("Control de plagas urbanas en viviendas y negocios: cucarachas, roedores, termitas, chinches, avispas y palomas, con urgencias y presupuesto tras la visita.",
                     "Urban pest control for homes and businesses: cockroaches, rodents, termites, bed bugs, wasps and pigeons, with emergency calls and a quote after the visit."), [
        hero(("Control de plagas en tu casa y en tu negocio", "Pest control for your home and your business"),
             ("Cucarachas, roedores, termitas, chinches, avispas y palomas. Primero miramos qué hay y de dónde viene, luego te damos el presupuesto y tratamos el problema.",
              "Cockroaches, rodents, termites, bed bugs, wasps and pigeons. We first look at what is there and where it comes from, then give you a quote and deal with it."),
             [llamar(), whatsapp(), visita("ghost")],
             eyebrow=("Plagas urbanas", "Urban pests"),
             extra=peq(BIOCIDAS, color="$muted"),
             imagen_=("photo", ("Técnico revisando un rodapié con una linterna en busca de plagas", "Technician checking a skirting board with a torch for pests"))),
        bloque_valores(("Qué plagas tratamos", "Which pests we treat"), [
            (("Cucarachas", "Cockroaches"), ("En cocinas, bares y bajantes. Tratamiento y seguimiento hasta que dejen de verse.", "In kitchens, bars and drains. Treatment and follow-up until none are seen.")),
            (("Roedores", "Rodents"), ("Ratas y ratones en casas, patios, garajes y locales. Cerramos por dónde entran.", "Rats and mice in homes, yards, garages and shops. We seal where they get in.")),
            (("Termitas", "Termites"), ("Detección en madera, vigas y marcos, y tratamiento de la zona afectada.", "Detection in wood, beams and frames, and treatment of the affected area.")),
            (("Chinches", "Bed bugs"), ("En camas, sofás y alojamientos. Revisamos y tratamos toda la estancia.", "In beds, sofas and holiday lets. We inspect and treat the whole room.")),
            (("Avispas y avispones", "Wasps and hornets"), ("Retirada de nidos en aleros, árboles y persianas, con seguridad.", "Removal of nests under eaves, in trees and in shutters, safely.")),
            (("Palomas", "Pigeons"), ("Control de aves en cornisas, terrazas y naves, y limpieza de la zona.", "Bird control on ledges, terraces and warehouses, and cleaning of the area.")),
        ], eyebrow=("Servicios", "Services"), estilo="relleno"),
        bloque_pasos(("Cómo trabajamos", "How we work"), [
            (("Inspección", "Inspection"),
             ("Vamos a verlo: buscamos la plaga, por dónde entra y qué la atrae. Es el momento de darte el presupuesto.",
              "We come and look: we find the pest, where it gets in and what attracts it. That is when we give you the quote.")),
            (("Tratamiento", "Treatment"),
             ("Elegimos el método según la plaga y el lugar. Te explicamos qué producto usamos y cómo prepararte.",
              "We choose the method to suit the pest and the place. We tell you which product we use and how to prepare.")),
            (("Seguimiento", "Follow-up"),
             ("Volvemos a comprobar que la plaga ha remitido. Si no es así, repetimos el tratamiento según lo acordado.",
              "We come back to check the pest has gone down. If not, we repeat the treatment as agreed.")),
            (("Prevención", "Prevention"),
             ("Te decimos qué cambiar en casa o en el local para que no vuelva: cierres, limpieza, almacenaje.",
              "We tell you what to change at home or on the premises so it does not come back: sealing, cleaning, storage.")),
        ], tono="base"),
        bloque_texto(("Con niños y mascotas en casa", "With children and pets at home"), [
            ("Dinos desde el primer momento si hay niños, embarazadas, mascotas, acuarios o aves. Elegimos el producto y el método pensando en ellos.",
             "Tell us from the start if there are children, pregnant women, pets, aquariums or birds. We choose the product and method with them in mind."),
            ("Antes del tratamiento te decimos qué recoger y dónde no pueden estar. Después, cuánto hay que ventilar y cuándo pueden volver.",
             "Before the treatment we tell you what to put away and where they must not be. Afterwards, how long to ventilate and when they can return.")],
            eyebrow=("Seguridad", "Safety"), anchor="seguridad", tono="relleno",
            imagen_=("photo", ("Salón ventilado tras un tratamiento, con una mascota en el sofá", "Ventilated living room after a treatment, with a pet on the sofa")),
            invertir=True),
        bloque_cta(("¿Has visto un nido de avispas o roedores?", "Have you seen a wasp nest or rodents?"),
                   ("No lo toques ni lo tapes. Llámanos y te decimos cuándo podemos ir.",
                    "Do not touch or block it. Call us and we will tell you when we can come."),
                   [llamar(), whatsapp("ghost")], tono="oscuro"),
        bloque_opiniones(("Lo que dicen nuestros clientes", "What our customers say"),
                         [("la rapidez al venir", "how quickly they came"), ("las explicaciones", "the explanations"), ("el seguimiento", "the follow-up")]),
    ])

    plagas = pagina("plagas", ("plagas-y-tratamientos", "pests-and-treatments"), ("Plagas y tratamientos", "Pests and treatments"),
                    ("Cómo reconocer cada plaga, qué tratamiento aplicamos, productos autorizados, seguridad con niños y mascotas y zonas donde trabajamos.",
                     "How to recognise each pest, what treatment we use, authorised products, safety with children and pets and the areas we cover."), [
        hero(("Plagas y tratamientos", "Pests and treatments"),
             ("Cómo reconocer cada plaga y qué hacemos para quitarla. El presupuesto lo cerramos tras ver el lugar, porque cada caso cambia.",
              "How to recognise each pest and what we do to get rid of it. We settle the quote after seeing the place, because every case is different."),
             [llamar(), whatsapp()], variante="texto"),
        seccion("features", [
            titular(("Las seis plagas que más vemos", "The six pests we see most"),
                    ("Si reconoces las señales, llámanos y lo comprobamos.", "If you recognise the signs, call us and we will check."),
                    eyebrow=("Reconocer y tratar", "Recognise and treat")),
            rejilla([plaga(("Cucarachas", "Cockroaches"),
                           ("Las ves de noche, encuentras excrementos pequeños como granos oscuros o notas olor a cerrado cerca de fregaderos y desagües.",
                            "You see them at night, find tiny dark droppings or notice a musty smell near sinks and drains."),
                           ("Tratamiento con gel o pulverización en los puntos donde se esconden y nueva visita para comprobar.",
                            "Gel or spray treatment where they hide and a second visit to check.")),
                     plaga(("Roedores", "Rodents"),
                           ("Ruidos en techos y paredes, excrementos, cables o envases roídos y marcas de grasa en rodapiés.",
                            "Noises in ceilings and walls, droppings, gnawed cables or packaging and grease marks on skirting boards."),
                           ("Colocamos cebos o trampas protegidos y cerramos huecos de entrada. Revisamos hasta que no haya actividad.",
                            "We place protected baits or traps and seal entry gaps. We check until there is no activity.")),
                     plaga(("Termitas", "Termites"),
                           ("Madera que suena hueca, serrín fino cerca de vigas o marcos, y pequeños túneles de barro en paredes.",
                            "Wood that sounds hollow, fine sawdust near beams or frames, and small mud tunnels on walls."),
                           ("Inspección de la madera y la zona, tratamiento del foco y plan de revisiones.",
                            "Inspection of the wood and surrounding area, treatment of the colony and a plan of checks.")),
                     plaga(("Chinches", "Bed bugs"),
                           ("Picaduras en fila al despertar, manchas pequeñas oscuras en el colchón y puntos en las costuras.",
                            "Bites in a row on waking, small dark stains on the mattress and specks along the seams."),
                           ("Revisión de cama, sofás y rodapiés y tratamiento de toda la estancia. Te decimos qué lavar y cómo.",
                            "Inspection of the bed, sofas and skirting boards and treatment of the whole room. We tell you what to wash and how.")),
                     plaga(("Avispas y avispones", "Wasps and hornets"),
                           ("Un nido en el alero, en un árbol, en una persiana o en un hueco del muro, con mucho movimiento de avispas.",
                            "A nest under the eaves, in a tree, in a shutter or in a gap in the wall, with lots of wasps flying in and out."),
                           ("Retirada del nido con equipo de protección y tratamiento del hueco. No lo toques: te picarán.",
                            "Removal of the nest with protective equipment and treatment of the gap. Do not touch it: they will sting.")),
                     plaga(("Palomas", "Pigeons"),
                           ("Nidos y excrementos en balcones, cornisas o aleros, y ruidos por la noche en el patio de luces.",
                            "Nests and droppings on balconies, ledges or eaves, and noises at night in the light well."),
                           ("Limpieza de la zona y colocación de pinchos, redes u otros sistemas para que no vuelvan a posarse.",
                            "Cleaning of the area and fitting of spikes, nets or other systems to stop them settling again."))],
                    cols=2, tablet=2)], anchor="reconocer", gap=36),
        bloque_checks(("Qué incluye la visita y el presupuesto", "What the visit and quote include"), [
            ("Inspección del lugar y del origen de la plaga.", "Inspection of the place and the origin of the pest."),
            ("Presupuesto por escrito con el tratamiento y las visitas de seguimiento.", "A written quote with the treatment and the follow-up visits."),
            ("Qué producto usamos, cómo prepararte y cuánto hay que ventilar.", "Which product we use, how to prepare and how long to ventilate."),
            ("Un parte del servicio con lo que se ha hecho y las recomendaciones.", "A service report with what was done and our recommendations."),
            ("Factura con IVA y garantía por escrito.", "An invoice with VAT and a written warranty."),
        ], intro=("Sin compromiso: si después de verlo decides no seguir, no pasa nada.", "No obligation: if you decide not to go ahead after we have seen it, that is fine."),
            eyebrow=("Presupuesto con visita", "Quote with visit"), tono="relleno", anchor="presupuesto"),
        bloque_checks(("Productos autorizados", "Authorised products"), [
            ("Usamos solo productos biocidas autorizados.", "We use only authorised biocide products."),
            ("Te decimos el nombre del producto y para qué sirve.", "We tell you the product name and what it is for."),
            ("Te damos la ficha con las indicaciones de seguridad si la pides.", "We give you the sheet with the safety information if you ask for it."),
            (BIOCIDAS[0], BIOCIDAS[1]),
        ], eyebrow=("Cómo lo hacemos con seguridad", "Doing it safely"), anchor="productos"),
        bloque_checks(("Zonas donde trabajamos", "Areas we cover"), [
            ("Centro de Tu ciudad", "Your city centre"),
            ("Barrios y urbanizaciones de alrededor", "Surrounding districts and housing estates"),
            ("Pueblos cercanos (consúltanos)", "Nearby villages (ask us)"),
            ("Locales, naves y comunidades de la zona", "Shops, warehouses and residential blocks in the area"),
        ], intro=("Si tu zona no aparece, llámanos y te decimos si llegamos.", "If your area is not listed, call us and we will tell you if we can get there."),
            eyebrow=("Zonas de servicio", "Service areas"), tono="relleno", anchor="zonas"),
        bloque_faq(("Preguntas frecuentes", "Frequently asked questions"), [
            (("¿Tengo que irme de casa durante el tratamiento?", "Do I have to leave home during the treatment?"),
             ("Depende del método. Antes te decimos si hay que salir, durante cuánto tiempo y cuándo puedes volver.",
              "It depends on the method. Beforehand we tell you whether you have to leave, for how long and when you can return.")),
            (("¿Es peligroso para mis mascotas?", "Is it dangerous for my pets?"),
             ("Usamos productos autorizados y los aplicamos donde no llegan. Avísanos de las mascotas y te decimos cómo hacerlo con seguridad.",
              "We use authorised products and apply them where pets cannot reach. Tell us about your pets and we explain how to do it safely.")),
            (("¿Cuántas visitas hacen falta?", "How many visits are needed?"),
             ("Depende de la plaga y del lugar. Con el presupuesto te decimos cuántas prevemos y qué pasa si hace falta repetir.",
              "It depends on the pest and the place. With the quote we tell you how many we expect and what happens if a repeat is needed.")),
            (("¿Cómo evito que vuelvan?", "How do I stop them coming back?"),
             ("Cerrar huecos, no dejar comida al descubierto, vaciar la basura a diario y arreglar goteos. En la visita te decimos qué cambiar.",
              "Seal gaps, do not leave food out, empty the bin daily and fix drips. On the visit we tell you what to change.")),
        ], tono="relleno"),
    ])

    neg = pagina("negocios", ("para-negocios", "for-businesses"), ("Control de plagas para negocios", "Pest control for businesses"),
                 ("Planes de control de plagas para bares, restaurantes, comunidades y naves, con certificado de cada tratamiento y visitas periódicas.",
                  "Pest control plans for bars, restaurants, residential blocks and warehouses, with a certificate for each treatment and periodic visits."), [
        hero(("Control de plagas para negocios y comunidades", "Pest control for businesses and residential blocks"),
             ("Visitas periódicas, parte de cada servicio y certificado del tratamiento. Presupuesto tras ver tu local.",
              "Periodic visits, a report for each service and a treatment certificate. A quote after we see your premises."),
             [visita(), llamar("secondary"), whatsapp("ghost")], variante="centro"),
        bloque_valores(("A quién atendemos", "Who we work for"), [
            (("Bares y restaurantes", "Bars and restaurants"), ("Cocina, almacén, baños y zonas de basura, sin cerrar el servicio si el tratamiento lo permite.", "Kitchen, storeroom, toilets and bin areas, without closing if the treatment allows.")),
            (("Alojamientos y colegios", "Accommodation and schools"), ("Habitaciones, comedores y zonas comunes con las precauciones que pide cada lugar.", "Rooms, dining halls and shared areas with the precautions each place needs.")),
            (("Comunidades y garajes", "Residential blocks and garages"), ("Patios, azoteas, bajantes y sótanos, con aviso previo a los vecinos.", "Courtyards, roofs, drains and basements, with advance notice to residents.")),
            (("Naves y almacenes", "Warehouses and storage"), ("Control de roedores y aves en zonas de carga, y puntos de seguimiento por la nave.", "Rodent and bird control in loading areas, and monitoring points around the warehouse.")),
        ], eyebrow=("Negocios", "Businesses"), cols=2, estilo="relleno", tono="base", anchor="clientes"),
        bloque_texto(("Certificados para tu negocio", "Certificates for your business"), [
            ("Después de cada tratamiento te damos el certificado del servicio y un parte con los productos que hemos usado y las zonas tratadas.",
             "After each treatment we give you the service certificate and a report listing the products we used and the areas treated."),
            ("Tenlo a mano por si lo pide una inspección. Qué documentos necesita tu actividad lo marca la normativa que te aplique; pregúntanos y te orientamos sin sustituir a tu asesor.",
             "Keep it handy in case an inspection asks for it. Which documents your activity needs depends on the rules that apply to you; ask us and we will point you in the right direction without replacing your adviser.")],
            eyebrow=("Papeles en orden", "Paperwork in order"), anchor="certificados",
            imagen_=("photo", ("Parte de servicio y certificado de tratamiento sobre una mesa", "Service report and treatment certificate on a desk")),
            tono="relleno"),
        bloque_pasos(("Cómo es un plan de mantenimiento", "What a maintenance plan looks like"), [
            (("Visita inicial", "First visit"), ("Recorremos el local, dibujamos un plano con los puntos de control y detectamos los riesgos.", "We walk through the premises, draw a plan with the control points and spot the risks.")),
            (("Presupuesto", "Quote"), ("Con las visitas al año que pide tu actividad. Un precio claro por visita o por el conjunto.", "With the visits a year your activity needs. A clear price per visit or for the whole plan.")),
            (("Visitas periódicas", "Regular visits"), ("Revisamos los puntos, tratamos si hace falta y anotamos todo en el parte.", "We check the points, treat if needed and note everything in the report.")),
            (("Aviso entre visitas", "Calls between visits"), ("Si ves algo raro, nos llamas y vamos sin esperar a la siguiente visita programada.", "If you see something odd, you call and we come without waiting for the next scheduled visit.")),
        ]),
        bloque_cta(("¿Quieres un presupuesto para tu local?", "Want a quote for your premises?"),
                   ("Cuéntanos qué tipo de negocio es y cuándo te viene bien que pasemos.",
                    "Tell us what kind of business it is and when suits you for a visit."),
                   [visita(), whatsapp("ghost")], tono="oscuro"),
    ])

    cont = pagina("contacto", ("contacto", "contact"), ("Contacto y urgencias", "Contact and emergencies"),
                  ("Llama para una urgencia, o escribe por WhatsApp o con el formulario para pedir visita y presupuesto.",
                   "Call for an emergency, or message us on WhatsApp or with the form to book a visit and quote."), [
        hero(("Pide visita y presupuesto", "Book a visit and quote"),
             ("Para una urgencia, llama: el formulario no lo atendemos al momento. Para una revisión o un plan de mantenimiento, usa el formulario o WhatsApp.",
              "For an emergency, call: we do not answer the form straight away. For an inspection or a maintenance plan, use the form or WhatsApp."),
             [llamar(), whatsapp()], variante="texto"),
        bloque_contacto(("Cuéntanos qué has visto", "Tell us what you have seen"),
                        ("Dinos qué plaga crees que es y dónde está. Te llamamos para ver cuándo podemos ir.",
                         "Tell us which pest you think it is and where it is. We will call you to arrange when we can come."),
                        campos_cita(servicios=[("Cucarachas", "Cockroaches"), ("Roedores", "Rodents"), ("Termitas", "Termites"),
                                               ("Chinches", "Bed bugs"), ("Avispas o avispones", "Wasps or hornets"), ("Palomas", "Pigeons"),
                                               ("Otra plaga o no lo sé", "Another pest or not sure")],
                                    fecha=False,
                                    extra=[campo("lugar", "select", ("¿Dónde está?", "Where is it?"), False,
                                                 opciones=[("Vivienda", "Home"), ("Bar o restaurante", "Bar or restaurant"),
                                                           ("Comunidad o garaje", "Residential block or garage"), ("Nave o almacén", "Warehouse or storage"),
                                                           ("Otro local", "Other premises")]),
                                           campo("poblacion", "text", ("Población o barrio", "Town or area"), True, maxlen=120),
                                           campo("ninos", "select", ("¿Hay niños o mascotas?", "Are there children or pets?"), False,
                                                 opciones=[("Niños", "Children"), ("Mascotas", "Pets"), ("Niños y mascotas", "Children and pets"), ("Ninguno", "Neither")]),
                                           campo("urgente", "select", ("¿Es una urgencia?", "Is it an emergency?"), False,
                                                 opciones=[("No, puede esperar", "No, it can wait"), ("Sí, lo antes posible", "Yes, as soon as possible")])]),
                        ("Pedir visita y presupuesto", "Book a visit and quote"), horas,
                        exito=("Gracias. Te llamamos lo antes posible. Si es una urgencia, llama al teléfono que ves en esta página.",
                               "Thank you. We will call you as soon as possible. If it is an emergency, call the number on this page."),
                        notas=BIOCIDAS),
        bloque_mapa(("Dónde está la oficina", "Where the office is"),
                    ("Vamos a tu casa o a tu local, no hace falta que vengas. Esta es nuestra oficina.",
                     "We come to you, so there is no need to visit. This is our office."), tono="relleno"),
    ])

    return plantilla(KEY, ("Control de plagas", "Pest control"),
                     ("Control de plagas urbanas con urgencias, inspección, tratamiento y seguimiento, seguridad con niños y mascotas, página para negocios con certificados y presupuesto con visita.",
                      "Urban pest control with emergencies, inspection, treatment and follow-up, safety with children and pets, a page for businesses with certificates and a quote after a visit."),
                     "oficios", ("control de plagas desratizacion desinsectacion cucarachas ratas ratones roedores termitas chinches avispas nidos palomas fumigacion biocidas plaguicidas",
                                 "pest control exterminator cockroaches rats mice rodents termites bed bugs wasps nest pigeons fumigation biocides"),
                     look, cabecera(nav, cta_cab),
                     pie(nav, ("Control de plagas urbanas en viviendas y negocios, con inspección, tratamiento y seguimiento.",
                               "Urban pest control for homes and businesses, with inspection, treatment and follow-up."),
                         ("Lunes a viernes de 8:00 a 19:00. Urgencias: llama.", "Monday to Friday 8 am to 7 pm. Emergencies: call.")),
                     [inicio, plagas, neg, cont])
