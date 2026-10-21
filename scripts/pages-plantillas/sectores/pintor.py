# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
from lib import *

KEY = "pintor"

# Paleta propia, casi monocroma: cal y tinta. El color lo ponen las fotos de los trabajos, no la web.
# Sin `tono="oscuro"`: primary/inverse no llega a 3.
CAL = dict(
    colores=dict(background="#fcfcfa", surface="#f0efea", text="#1a1a19", muted="#57554f", primary="#26262a",
                 on_primary="#ffffff", border="#dcdad2", inverse="#1a1a19", on_inverse="#f7f6f1"),
    fuentes=("manrope", "figtree"), peso=700, radio=(2, 6, 10), ancho=1140)


def build():
    nueva(KEY)
    look = CAL
    nav = [("inicio", ("Inicio", "Home")), ("trabajos", ("Trabajos y proceso", "Work and process")),
           ("galeria", ("Galería", "Gallery")), ("contacto", ("Presupuesto", "Quote"))]
    horas = [(("Lunes a viernes", "Monday to Friday"), ("De 8:00 a 18:00", "8:00 am to 6:00 pm")),
             (("Sábados", "Saturdays"), ("Solo con cita para visitas", "By appointment for visits only"))]
    cta_cab = marco([llamar(t=("Llamar", "Call")), whatsapp("secondary", t=("WhatsApp", "WhatsApp"))],
                    dir="row", gap=8, wrap=True)

    inicio = pagina("inicio", ("", ""), ("Pintor de interiores y fachadas", "Interior and exterior painter"),
                    ("Pintor de interiores y fachadas: presupuesto sin compromiso tras visitar la obra, proceso en pasos, galería de trabajos y zonas de servicio.",
                     "Interior and exterior painter: a no-obligation quote after visiting the job, a step-by-step process, a gallery of work and the areas we cover."), [
        hero(("Pintura de interiores y fachadas", "Interior and exterior painting"),
             ("Pintamos pisos, casas, locales y fachadas. Vamos a verlo, te damos el presupuesto por escrito y protegemos tus muebles antes de abrir un bote.",
              "We paint flats, houses, shops and façades. We come to see it, give you a written quote and cover your furniture before opening a tin."),
             [llamar(), whatsapp()], variante="texto",
             eyebrow=("Pintor", "Painter")),
        bloque_galeria(("Trabajos recientes", "Recent work"), [
            ("Salón pintado en blanco roto con rodapié lacado", "Living room painted off-white with lacquered skirting"),
            ("Fachada de edificio repintada", "Building façade repainted"),
            ("Dormitorio con una pared de color", "Bedroom with a colour feature wall"),
            ("Escalera de comunidad pintada", "Communal staircase painted"),
            ("Puerta y ventanas lacadas", "Lacquered door and windows"),
            ("Local comercial recién pintado", "Newly painted shop"),
        ], tipo="landscape", cols=3, ratio=1.4, anchor="galeria-inicio",
            intro=("Fotos de ejemplo: sustitúyelas por las de tus trabajos.", "Example photos: replace them with your own work.")),
        bloque_valores(("Qué pintamos", "What we paint"), [
            (("Interiores", "Interiors"),
             ("Paredes y techos de pisos y casas, con plástica lavable o pintura antihumedad donde hace falta.",
              "Walls and ceilings in flats and houses, with washable emulsion or anti-damp paint where needed.")),
            (("Fachadas", "Façades"),
             ("Pintura exterior sobre fachada y medianeras. Repasamos grietas y desconchones antes de pintar.",
              "Exterior paint on façades and party walls. We go over cracks and flaking before painting.")),
            (("Escaleras y comunidades", "Staircases and communal areas"),
             ("Portales y escaleras, con horarios pactados con la comunidad para molestar lo menos posible.",
              "Entrances and staircases, with times agreed with the residents so we disturb them as little as possible.")),
            (("Lacado de puertas y rodapiés", "Door and skirting lacquer"),
             ("Puertas, ventanas, barandillas y rodapiés, lijados y lacados a mano o a pistola.",
              "Doors, windows, railings and skirting boards, sanded and lacquered by hand or spray.")),
            (("Locales y oficinas", "Shops and offices"),
             ("Trabajamos fuera de tu horario de apertura para que no pierdas días de venta.",
              "We work outside your opening hours so you do not lose trading days.")),
            (("Quitar gotelé y alisar", "Removing textured finish and smoothing"),
             ("Raspamos el gotelé y alisamos la pared. Es sucio, así que protegemos y limpiamos a fondo.",
              "We scrape off the textured finish and smooth the wall. It is messy, so we protect and clean thoroughly.")),
        ], eyebrow=("Tipos de trabajo", "Types of job"), tono="relleno", estilo="borde"),
        bloque_checks(("Presupuesto sin compromiso", "A quote with no obligation"), [
            ("Vamos a verlo, medimos y miramos el estado de las paredes.", "We come to see it, measure up and check the state of the walls."),
            ("Te damos el precio por escrito, con lo que incluye y lo que no.", "We give you the price in writing, with what is and is not included."),
            ("La visita no cuesta nada ni te obliga a contratarnos.", "The visit costs nothing and does not oblige you to hire us."),
            ("Si quieres comparar con otros presupuestos, te damos el tiempo que necesites.", "If you want to compare other quotes, you can take all the time you need."),
        ], eyebrow=("Cómo se pide", "How to ask"), cols=2, anchor="presupuesto"),
        bloque_checks(("Zonas donde pintamos", "Areas we cover"), [
            ("Centro de Tu ciudad", "Your city centre"),
            ("Barrios y urbanizaciones de alrededor", "Surrounding districts and housing estates"),
            ("Pueblos cercanos (consúltanos)", "Nearby villages (ask us)"),
        ], intro=("Si tu zona no aparece, llámanos y te decimos si llegamos.", "If your area is not listed, call us and we will tell you if we can get there."),
            eyebrow=("Zonas", "Areas"), tono="relleno", cols=3),
        bloque_opiniones(("Lo que dicen nuestros clientes", "What our customers say"),
                         [("la limpieza", "the tidiness"), ("el acabado", "the finish"), ("el cumplimiento de los plazos", "keeping to the schedule")],
                         tono="base"),
        bloque_cta(("¿Quieres pintar tu casa?", "Want to paint your home?"),
                   ("Llámanos o escríbenos y quedamos para verlo.", "Call or message us and we will arrange to see it."),
                   [llamar(), whatsapp("ghost")]),
    ])

    trab = pagina("trabajos", ("trabajos", "work"), ("Trabajos y proceso", "Work and process"),
                  ("Cómo trabajamos paso a paso, de la visita a la limpieza final; de qué depende el precio, la garantía y las preguntas frecuentes.",
                   "How we work step by step, from the visit to the final clean; what the price depends on, the warranty and frequently asked questions."), [
        hero(("Cómo trabajamos", "How we work"),
             ("Cada obra sigue los mismos seis pasos. Así sabes qué pasa cada día y cuándo acabamos.",
              "Every job follows the same six steps. That way you know what happens each day and when we finish."),
             [llamar(), whatsapp()], variante="texto"),
        bloque_pasos(("De la visita a la limpieza", "From the visit to the clean-up"), [
            (("Visita", "Visit"),
             ("Vamos a verlo, medimos y miramos humedades, grietas y pintura vieja.",
              "We come to see it, measure up and look at damp, cracks and old paint.")),
            (("Presupuesto", "Quote"),
             ("Te lo damos por escrito, con la pintura que usaremos, los días que llevará y lo que incluye.",
              "We give it to you in writing, with the paint we will use, how many days it takes and what is included.")),
            (("Protección de muebles", "Covering furniture"),
             ("Movemos y tapamos muebles y suelos con plástico y cinta. Lo que no se pinta, no se mancha.",
              "We move and cover furniture and floors with sheeting and tape. What is not painted is not stained.")),
            (("Preparación", "Preparation"),
             ("Tapamos agujeros, lijamos y damos imprimación. De esto depende que la pintura dure.",
              "We fill holes, sand and prime. How long the paint lasts depends on this.")),
            (("Pintura", "Painting"),
             ("Aplicamos las manos que hagan falta, dejando secar entre una y otra.",
              "We apply as many coats as needed, leaving each to dry before the next.")),
            (("Limpieza", "Clean-up"),
             ("Quitamos protecciones, recogemos y dejamos cada cosa en su sitio. Repasamos contigo los remates.",
              "We remove the covers, tidy up and put everything back. We go over the finishing touches with you.")),
        ]),
        bloque_checks(("De qué depende el precio", "What the price depends on"), [
            ("Los metros cuadrados de pared y techo, y la altura.", "The square metres of wall and ceiling, and the height."),
            ("El estado de las paredes: humedades, grietas, gotelé, pintura que se cae.", "The state of the walls: damp, cracks, textured finish, peeling paint."),
            ("La calidad de la pintura y el número de manos.", "The quality of the paint and the number of coats."),
            ("Si hay que mover muchos muebles o trabajar con andamio o plataforma.", "Whether many pieces of furniture must be moved or scaffolding or a platform is needed."),
        ], intro=("Por eso no damos precios sin ver la obra: el presupuesto lo hacemos tras la visita.",
                  "That is why we do not quote without seeing the job: we give the quote after the visit."),
            eyebrow=("Presupuesto", "Quote"), tono="relleno", cols=2),
        bloque_texto(("Garantía", "Warranty"), [
            ("Damos 00 años de garantía en la pintura aplicada (garantía de ejemplo: cámbiala por la tuya).",
             "We give a 00-year warranty on the paint we apply (example warranty: replace it with yours)."),
            ("Si una mancha de humedad vuelve a salir, te explicamos de dónde viene antes de volver a pintar encima.",
             "If a damp patch comes back, we explain where it comes from before painting over it again.")],
            eyebrow=("Después de pintar", "After painting"), anchor="garantia"),
        bloque_faq(("Preguntas frecuentes", "Frequently asked questions"), [
            (("¿Tengo que sacar los muebles?", "Do I have to take the furniture out?"),
             ("No. Movemos los muebles al centro de cada habitación y los tapamos. Si hay cosas frágiles, recógelas antes.",
              "No. We move furniture to the middle of each room and cover it. If you have fragile items, put them away first.")),
            (("¿Cuánto se tarda en pintar un piso?", "How long does it take to paint a flat?"),
             ("Depende del tamaño y del estado de las paredes. Tras la visita te damos los días que llevará.",
              "It depends on the size and the state of the walls. After the visit we tell you how many days it will take.")),
            (("¿Se puede vivir en casa mientras se pinta?", "Can I live at home while it is painted?"),
             ("Sí, aunque lo mejor es ventilar y dormir en otra habitación el día de la imprimación y las manos finales.",
              "Yes, although it is best to ventilate and sleep in another room on the day of priming and the final coats.")),
            (("¿Qué pintura usáis?", "What paint do you use?"),
             ("La que acordamos contigo en el presupuesto: plástica lavable, antihumedad, esmalte para madera o pintura de fachada.",
              "The one we agree with you in the quote: washable emulsion, anti-damp, wood enamel or façade paint.")),
            (("¿Y si no me gusta el color?", "What if I do not like the colour?"),
             ("Prueba un muestrario antes en la pared. Cambiar de color después se presupuesta aparte.",
              "Try a sample on the wall first. Changing colour afterwards is quoted separately.")),
        ], tono="relleno"),
    ])

    gal = pagina("galeria", ("galeria", "gallery"), ("Galería de trabajos", "Gallery of work"),
                 ("Fotos de trabajos de interiores, fachadas, escaleras y lacados.", "Photos of interiors, façades, staircases and lacquer work."), [
        hero(("Galería de trabajos", "Gallery of work"),
             ("Algunos trabajos terminados. El presupuesto lo hacemos siempre tras visitar la obra.",
              "Some finished jobs. We always give the quote after visiting the job."),
             [llamar(), whatsapp()], variante="texto"),
        bloque_galeria(("Interiores", "Interiors"), [
            ("Salón con paredes blancas y techo liso", "Living room with white walls and a smooth ceiling"),
            ("Cocina pintada en verde suave", "Kitchen painted in soft green"),
            ("Dormitorio infantil con pared de color", "Child's bedroom with a colour wall"),
            ("Pasillo con zócalo y pared clara", "Hallway with dado and a light wall"),
            ("Baño con pintura antihumedad", "Bathroom with anti-damp paint"),
            ("Techo alto pintado con andamio", "High ceiling painted from scaffolding"),
        ], tipo="landscape", cols=3, ratio=1.4, anchor="interiores",
            intro=("Fotos de ejemplo: sustitúyelas por las de tus trabajos.", "Example photos: replace them with your own work.")),
        bloque_galeria(("Fachadas y exteriores", "Façades and exteriors"), [
            ("Fachada de una casa pintada en color crema", "House façade painted cream"),
            ("Fachada de bloque de pisos con andamio", "Block of flats façade with scaffolding"),
            ("Medianera repintada", "Party wall repainted"),
            ("Verja y portón lacados en negro", "Gate and railings lacquered black"),
        ], tipo="landscape", cols=2, ratio=1.4, anchor="exteriores", tono="relleno"),
        bloque_cta(("¿Te gustaría algo así?", "Would you like something like this?"),
                   ("Cuéntanos qué quieres pintar y pasamos a verlo.", "Tell us what you want painted and we will come to see it."),
                   [llamar(), whatsapp("ghost")]),
    ])

    cont = pagina("contacto", ("presupuesto", "quote"), ("Presupuesto sin compromiso", "No-obligation quote"),
                  ("Pide una visita para tu presupuesto: llama, escribe por WhatsApp o usa el formulario. Horario y cómo llegar.",
                   "Ask for a visit for your quote: call, message us on WhatsApp or use the form. Opening hours and how to find us."), [
        hero(("Pide tu presupuesto sin compromiso", "Ask for your no-obligation quote"),
             ("Cuéntanos qué quieres pintar y quedamos para verlo. La visita no tiene coste.",
              "Tell us what you want painted and we will arrange to see it. The visit is free."),
             [llamar(), whatsapp()], variante="texto"),
        bloque_contacto(("Cuéntanos tu obra", "Tell us about your job"),
                        ("Te llamamos para acordar el día de la visita.", "We will call you to agree the day of the visit."),
                        campos_cita(servicios=[("Interior de vivienda", "Home interior"), ("Fachada o exterior", "Façade or exterior"),
                                               ("Escalera o portal", "Staircase or entrance"), ("Local u oficina", "Shop or office"),
                                               ("Lacado de puertas o ventanas", "Door or window lacquer"), ("Otro trabajo", "Other job")],
                                    extra=[campo("metros", "text", ("Metros cuadrados aproximados", "Approximate square metres"), False, maxlen=20)]),
                        ("Pedir presupuesto", "Ask for a quote"), horas,
                        exito=("Gracias. Te llamamos para quedar el día de la visita.", "Thank you. We will call you to arrange the day of the visit.")),
        bloque_mapa(("Dónde estamos", "Where we are"),
                    ("Vamos a verlo donde esté la obra; esta es nuestra dirección de contacto.",
                     "We come to see the job wherever it is; this is our contact address."), tono="relleno"),
    ])

    return plantilla(KEY, ("Pintor de interiores y fachadas", "Interior and exterior painter"),
                     ("Cuatro páginas: inicio con galería, proceso en seis pasos y garantía, galería de trabajos y presupuesto sin compromiso con llamada, WhatsApp y formulario.",
                      "Four pages: home with gallery, a six-step process and warranty, a gallery of work and a no-obligation quote with call, WhatsApp and form."),
                     "oficios", ("pintor pintura pintores pintar piso interiores fachadas lacado gotele alisar escaleras comunidad presupuesto sin compromiso",
                                 "painter decorator painting interior exterior facade lacquer staircase quote house painter"),
                     look, cabecera(nav, cta_cab),
                     pie(nav, ("Pintura de interiores y fachadas. Presupuesto sin compromiso tras la visita.",
                               "Interior and exterior painting. A no-obligation quote after the visit."),
                         ("Lunes a viernes de 8:00 a 18:00.", "Monday to Friday 8 am to 6 pm.")),
                     [inicio, trab, gal, cont])
