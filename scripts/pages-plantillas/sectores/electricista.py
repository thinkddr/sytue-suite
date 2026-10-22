# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
from lib import *
from looks import LOOKS

KEY = "electricista"

# Como COLEGIADO: el 00-0000 lo detecta domain.TextosDemo y el sitio no se publica hasta cambiarlo.
INSTALADOR = ("Instalador autorizado. Nº de instalador: 00-0000", "Authorised installer. Installer no.: 00-0000")


def build():
    nueva(KEY)
    look = LOOKS["marino"]
    nav = [("inicio", ("Inicio", "Home")), ("servicios", ("Servicios y presupuesto", "Services and quotes")),
           ("contacto", ("Contacto", "Contact"))]
    horas = [(("Lunes a viernes", "Monday to Friday"), ("8:00 a 18:00", "8:00 am to 6:00 pm")),
             (("Sábado y domingo", "Saturday and Sunday"), ("Solo urgencias", "Emergencies only")),
             (("Urgencias", "Emergencies"),
              ("Llama y te decimos cuándo podemos llegar", "Call and we will tell you when we can get there"))]
    cta_cab = marco([llamar(t=("Llamar", "Call")), whatsapp("secondary", t=("WhatsApp", "WhatsApp"))],
                    dir="row", gap=8, wrap=True)
    presu = lambda v="primary": boton(("Pedir presupuesto", "Ask for a quote"), a_pagina("contacto", "contacto"), v)

    inicio = pagina("inicio", ("", ""), ("Electricista", "Electrician"),
                    ("Electricista autorizado: boletines, cuadros eléctricos, iluminación, domótica básica y urgencias. Presupuesto sin compromiso.",
                     "Authorised electrician: installation certificates, consumer units, lighting, basic home automation and emergencies. No-obligation quotes."), [
        hero(("Electricista autorizado para tu casa y tu negocio", "Authorised electrician for your home and business"),
             ("Boletines, cuadros eléctricos, iluminación y urgencias. Vemos el trabajo, te damos el presupuesto sin compromiso y te explicamos qué se hace y por qué.",
              "Installation certificates, consumer units, lighting and emergencies. We look at the job, give you a no-obligation quote and explain what is done and why."),
             [llamar(), whatsapp(), presu("ghost")],
             eyebrow=("Instalaciones eléctricas", "Electrical installations"),
             extra=peq(INSTALADOR, color="$muted"),
             imagen_=("photo", ("Electricista trabajando en un cuadro eléctrico", "Electrician working on a consumer unit"))),
        bloque_valores(("Qué hacemos", "What we do"), [
            (("Boletines y certificados", "Installation certificates"),
             ("El certificado de instalación eléctrica para dar de alta la luz, subir la potencia o cuando la normativa lo pida.",
              "The electrical installation certificate to connect your supply, increase power or whenever the regulations require it.")),
            (("Cuadros eléctricos", "Consumer units"),
             ("Cambio de cuadro, diferenciales e interruptores automáticos. Dejamos cada circuito etiquetado.",
              "Replacing the consumer unit, RCDs and circuit breakers. We label every circuit.")),
            (("Iluminación", "Lighting"),
             ("Puntos de luz, focos LED y luz para local, escaparate, garaje o jardín.",
              "Light points, LED spotlights and lighting for a shop, window, garage or garden.")),
            (("Domótica básica", "Basic home automation"),
             ("Luces y enchufes programables, termostatos y persianas con mando, sin complicaciones.",
              "Programmable lights and sockets, thermostats and remote-controlled blinds, without the fuss.")),
            (("Urgencias", "Emergencies"),
             ("Te quedas sin luz, saltan los diferenciales o notas olor a quemado. Llama y vemos cómo atenderte lo antes posible.",
              "You lose power, the RCDs keep tripping or you smell burning. Call and we will see how to get to you as soon as possible.")),
            (("Instalaciones nuevas y reformas", "New installations and refits"),
             ("Instalación completa en vivienda o local nuevo, y puesta al día de instalaciones antiguas.",
              "Full installations in a new home or premises, and updating old installations.")),
        ], eyebrow=("Servicios", "Services"), estilo="linea"),
        bloque_pasos(("Cómo trabajamos", "How we work"), [
            (("Visita", "Visit"),
             ("Vamos a ver la instalación, escuchamos qué necesitas y tomamos medidas.",
              "We come to see the installation, listen to what you need and take measurements.")),
            (("Presupuesto sin compromiso", "No-obligation quote"),
             ("Por escrito, con los materiales y la mano de obra detallados y el plazo.",
              "In writing, with materials and labour itemised and the timescale.")),
            (("Trabajo y pruebas", "Work and testing"),
             ("Hacemos la instalación y la probamos antes de irnos. Lo dejamos recogido y limpio.",
              "We carry out the installation and test it before we leave. We leave everything tidy and clean.")),
            (("Certificado y factura", "Certificate and invoice"),
             ("Cuando corresponde, te entregamos el boletín. Siempre, factura con IVA.",
              "Where it applies, we hand over the certificate. Always an invoice with VAT.")),
        ], tono="relleno"),
        bloque_opiniones(("Lo que dicen nuestros clientes", "What our customers say"),
                         [("el trabajo bien hecho", "the quality of the work"), ("el presupuesto", "the quote"), ("el trato", "the service")],
                         tono="base"),
        bloque_cta(("¿Necesitas un electricista?", "Do you need an electrician?"),
                   ("Cuéntanos qué necesitas y te damos fecha para la visita.",
                    "Tell us what you need and we will give you a date for the visit.")),
    ])

    serv = pagina("servicios", ("servicios", "services"), ("Servicios y presupuesto", "Services and quotes"),
                  ("Qué es el boletín eléctrico, cuándo conviene revisar la instalación, qué incluye el presupuesto sin compromiso y zonas de servicio.",
                   "What an installation certificate is, when to have your installation checked, what the no-obligation quote includes and the areas we cover."), [
        hero(("Servicios y presupuesto", "Services and quotes"),
             ("No damos precios de lista porque cada instalación es distinta. Vamos a verla y te damos el precio cerrado por escrito.",
              "We do not give list prices because every installation is different. We come to see it and give you a fixed price in writing."),
             [presu(), llamar("secondary")], variante="texto"),
        bloque_texto(("El boletín eléctrico, en pocas palabras", "The installation certificate in a nutshell"), [
            ("El boletín (certificado de instalación eléctrica) es el documento que acredita que la instalación cumple el reglamento. Se pide, por ejemplo, para dar de alta la luz o para aumentar la potencia contratada.",
             "The certificate (electrical installation certificate) is the document that shows the installation meets the regulations. It is asked for, for example, when connecting your supply or increasing your contracted power."),
            ("Los requisitos cambian según el caso y la comunidad autónoma. En la visita te decimos qué te toca a ti.",
             "Requirements vary by case and by region. At the visit we tell you what applies to you.")],
            eyebrow=("Boletines", "Certificates"), anchor="boletin"),
        bloque_checks(("Cuándo conviene que revisemos tu instalación", "When to have your installation checked"), [
            ("Saltan los diferenciales o el automático sin motivo claro.", "The RCD or circuit breaker trips for no clear reason."),
            ("Enchufes, clavijas o cables que se calientan.", "Sockets, plugs or cables that get hot."),
            ("Las luces parpadean o bajan de intensidad.", "Lights flicker or dim."),
            ("Vivienda antigua cuya instalación nunca se ha revisado.", "An older home whose installation has never been checked."),
            ("Vas a poner una cocina de inducción, aire acondicionado o un punto de carga para coche eléctrico.",
             "You are about to fit an induction hob, air conditioning or an electric car charging point."),
            ("Reforma de la vivienda o del local.", "Refurbishing your home or premises."),
        ], eyebrow=("Revisiones", "Checks"), tono="relleno", anchor="revision"),
        bloque_checks(("Presupuesto sin compromiso", "A no-obligation quote"), [
            ("Visita para ver la instalación y tomar medidas.", "A visit to see the installation and take measurements."),
            ("Materiales y mano de obra, cada cosa por separado.", "Materials and labour, each listed separately."),
            ("Plazo de ejecución y fecha de inicio.", "Timescale and start date."),
            ("Boletín y legalización, cuando corresponda.", "Certificate and registration, where they apply."),
            ("IVA incluido y factura al terminar.", "VAT included and an invoice when we finish."),
            ("Si surge algo imprevisto, te avisamos antes de seguir.", "If something unexpected comes up, we tell you before going on."),
        ], eyebrow=("Cómo cobramos", "How we charge"), anchor="presupuesto"),
        bloque_checks(("Zonas donde trabajamos", "Areas we cover"), [
            ("Centro de Tu ciudad", "Your city centre"),
            ("Barrios del norte", "Northern districts"),
            ("Barrios del sur y urbanizaciones", "Southern districts and housing estates"),
            ("Pueblos de alrededor (consúltanos)", "Surrounding villages (ask us)"),
        ], intro=("Si tu zona no aparece, llámanos y te decimos si llegamos.",
                  "If your area is not listed, call us and we will tell you if we can get there."),
            eyebrow=("Zonas de servicio", "Service areas"), tono="relleno", anchor="zonas"),
        bloque_faq(("Preguntas frecuentes", "Frequently asked questions"), [
            (("¿Cuánto cuesta?", "How much does it cost?"),
             ("Depende de la instalación, por eso vamos a verla. Te damos el precio cerrado por escrito y la visita para el presupuesto no se cobra.",
              "It depends on the installation, which is why we come to see it. You get a fixed price in writing and the visit for the quote is free.")),
            (("¿Qué hago si salta el diferencial?", "What do I do if the RCD trips?"),
             ("Desconecta los aparatos, sube la palanca y ve enchufando de uno en uno. Si vuelve a saltar, no insistas y llámanos.",
              "Unplug your appliances, reset the switch and plug them back in one at a time. If it trips again, do not keep trying and call us.")),
            (("¿Trabajáis en comunidades y locales?", "Do you work in communal buildings and commercial premises?"),
             ("Sí, en viviendas, comunidades y locales. Cuéntanos qué necesitas y te decimos si podemos hacerlo.",
              "Yes, in homes, communal buildings and commercial premises. Tell us what you need and we will say whether we can do it.")),
            (("¿Dais garantía del trabajo?", "Do you guarantee the work?"),
             ("Sí. La garantía de la instalación y de los materiales va por escrito en la factura.",
              "Yes. The warranty on the installation and the materials is written on the invoice.")),
        ]),
        bloque_cta(("¿Vamos a ver tu instalación?", "Shall we come and see your installation?"), None,
                   [presu(), llamar("secondary")], tono="relleno"),
    ])

    cont = pagina("contacto", ("contacto", "contact"), ("Contacto", "Contact"),
                  ("Pide presupuesto por teléfono, WhatsApp o con el formulario, o llama si es una urgencia. Mira dónde estamos.",
                   "Ask for a quote by phone, WhatsApp or with the form, or call if it is an emergency. See where we are."), [
        hero(("Pídenos presupuesto", "Ask us for a quote"),
             ("Llama, escríbenos por WhatsApp o rellena el formulario. Si es una urgencia, llama.",
              "Call, message us on WhatsApp or fill in the form. If it is an emergency, call."),
             [llamar(), whatsapp()], variante="texto", extra=peq(INSTALADOR, color="$muted")),
        bloque_contacto(("Cuéntanos qué necesitas", "Tell us what you need"),
                        ("Con unos datos te llamamos para quedar en una visita.",
                         "With a few details we will call you to arrange a visit."),
                        campos_cita(servicios=[("Boletín o certificado", "Installation certificate"),
                                               ("Cuadro eléctrico", "Consumer unit"), ("Iluminación", "Lighting"),
                                               ("Domótica básica", "Basic home automation"), ("Urgencia", "Emergency"),
                                               ("Instalación nueva o reforma", "New installation or refit"), ("Otro", "Other")],
                                    fecha=False,
                                    extra=[campo("poblacion", "text", ("Población o barrio", "Town or area"), True, maxlen=120)]),
                        ("Pedir presupuesto", "Ask for a quote"), horas,
                        exito=("Gracias. Te llamamos para quedar en una visita y darte el presupuesto.",
                               "Thank you. We will call you to arrange a visit and give you the quote.")),
        bloque_mapa(tono="relleno"),
    ])

    return plantilla(KEY, ("Electricista", "Electrician"),
                     ("Boletines, cuadros eléctricos, iluminación, domótica básica y urgencias, con instalador autorizado, presupuesto sin compromiso y botones de llamar y WhatsApp.",
                      "Installation certificates, consumer units, lighting, basic home automation and emergencies, with an authorised installer, no-obligation quotes and call and WhatsApp buttons."),
                     "oficios", ("electricista instalador electrico instalador autorizado boletin certificado cuadro electrico iluminacion domotica urgencias enchufes instalacion electrica",
                                 "electrician electrical installer authorised installer installation certificate consumer unit fuse box lighting home automation emergency wiring"),
                     look, cabecera(nav, cta_cab),
                     pie(nav, ("Electricista. Instalador autorizado, nº 00-0000. Presupuesto sin compromiso.",
                               "Electrician. Authorised installer, no. 00-0000. No-obligation quotes."),
                         ("Lunes a viernes 8:00 a 18:00. Urgencias: llama.", "Monday to Friday 8 am to 6 pm. Emergencies: call.")),
                     [inicio, serv, cont])
