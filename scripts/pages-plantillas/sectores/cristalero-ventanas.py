# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
from lib import *

KEY = "cristalero-ventanas"

# Paleta propia: plata de aluminio y violeta profundo. Sin `tono="oscuro"`: primary/inverse no llega a 3.
ALUMINIO = dict(
    colores=dict(background="#f4f6f8", surface="#e3e7ee", text="#15181f", muted="#4a5260", primary="#5b21b6",
                 on_primary="#ffffff", border="#d2d8e2", inverse="#1d1538", on_inverse="#f1eefb"),
    fuentes=("dm-sans", "ibm-plex-sans"), peso=700, radio=(4, 8, 12), ancho=1140)


def build():
    nueva(KEY)
    look = ALUMINIO
    nav = [("inicio", ("Inicio", "Home")), ("ventanas", ("Ventanas y cristales", "Windows and glass")),
           ("trabajos", ("Trabajos", "Work")), ("contacto", ("Presupuesto", "Quote"))]
    horas = [(("Lunes a viernes", "Monday to Friday"), ("De 9:00 a 18:30", "9:00 am to 6:30 pm")),
             (("Sábados", "Saturdays"), ("De 9:00 a 13:00", "9:00 am to 1:00 pm")),
             (("Cristal roto", "Broken glass"), ("Llama y te decimos cuándo podemos ir", "Call and we will tell you when we can come"))]
    cta_cab = marco([llamar(t=("Llamar", "Call")), whatsapp("secondary", t=("WhatsApp", "WhatsApp"))],
                    dir="row", gap=8, wrap=True)

    inicio = pagina("inicio", ("", ""), ("Cristalería y ventanas", "Glazier and windows"),
                    ("Cristalería y ventanas de aluminio y PVC: cristales rotos con aviso de urgencias, doble acristalamiento explicado en llano, mamparas, espejos y presupuesto tras tomar medidas.",
                     "Glazier and aluminium and PVC windows: broken glass with an emergency call-out, double glazing explained in plain words, shower screens, mirrors and a quote after we measure up."), [
        hero(("Cristalería, ventanas y mamparas", "Glazing, windows and shower screens"),
             ("Cambiamos cristales rotos, ponemos ventanas de aluminio y PVC y hacemos mamparas y espejos a medida. Vamos a tu casa, medimos y te damos el precio por escrito.",
              "We replace broken glass, fit aluminium and PVC windows and make shower screens and mirrors to measure. We come to your home, measure up and give you the price in writing."),
             [llamar(), whatsapp()], variante="centro",
             eyebrow=("Cristalero", "Glazier")),
        seccion("features", [marco([
            etiqueta(("Aviso de urgencias", "Emergency call-out")),
            h2(("¿Se ha roto un cristal?", "Has a pane broken?")),
            p(("Llama y cuéntanos qué ha pasado y de qué medida es. Si hay riesgo de cortes o de que entre agua o frío, vamos lo antes posible a retirar el vidrio roto y a cerrar el hueco. El cristal nuevo se encarga a medida.",
               "Call and tell us what happened and roughly what size it is. If there is a risk of cuts or of rain or cold getting in, we come as soon as we can to clear the broken glass and close up the opening. The new pane is ordered to measure.")),
            marco([llamar(t=("Llamar por una urgencia", "Call about an emergency")), whatsapp("ghost", t=("Mandar una foto", "Send a photo"))],
                  dir="row", gap=12, wrap=True),
        ], gap=16, sz={"width": "fill", "height": "hug", "max_width": 760})], tono="relleno", anchor="urgencias", pad_v=56, gap=0),
        bloque_valores(("Qué hacemos", "What we do"), [
            (("Cristales y reposiciones", "Glass and replacements"),
             ("Cristal sencillo, doble, laminado o templado, para ventanas, puertas y mamparas. Medimos el hueco y lo cortamos a medida.",
              "Single, double, laminated or toughened glass for windows, doors and screens. We measure the opening and cut to size.")),
            (("Ventanas de aluminio y PVC", "Aluminium and PVC windows"),
             ("Cambiamos ventanas viejas por otras nuevas, con o sin persiana, y sellamos bien el contorno.",
              "We replace old windows with new ones, with or without a roller shutter, and seal the surround properly.")),
            (("Mamparas de baño", "Shower screens"),
             ("Fijas, correderas o abatibles, a la medida de tu plato o bañera, con cristal templado.",
              "Fixed, sliding or hinged, made to the size of your tray or bath, in toughened glass.")),
            (("Espejos", "Mirrors"),
             ("Espejos de baño, de vestidor o de pared completa, con el canto pulido o con marco.",
              "Bathroom, dressing-room or full-wall mirrors, with polished edges or a frame.")),
            (("Cerramientos", "Enclosures"),
             ("Cerramos terrazas y porches con perfilería de aluminio y cristal.", "We enclose terraces and porches with aluminium frames and glass.")),
            (("Arreglos pequeños", "Small repairs"),
             ("Cambio de gomas, herrajes, manetas y ruedas de correderas, y ajuste de ventanas que cierran mal.",
              "Replacing seals, hardware, handles and sliding-door wheels, and adjusting windows that do not close properly.")),
        ], eyebrow=("Trabajos", "Work"), tono="relleno", estilo="borde"),
        bloque_texto(("Doble acristalamiento, en llano", "Double glazing, in plain words"), [
            ("Son dos cristales con una cámara de aire o gas en medio. Esa cámara frena el paso del frío y del calor y también del ruido de la calle.",
             "It is two panes with a gap of air or gas in between. The gap slows the passage of cold and heat, and street noise too."),
            ("Se nota sobre todo en invierno: la pared de cristal deja de estar fría y hay menos condensación. Te explicamos qué grosor y qué cámara conviene en cada ventana.",
             "You notice it most in winter: the glass stops feeling cold and there is less condensation. We explain what thickness and gap suit each window.")],
            eyebrow=("Cristales", "Glass"), imagen_=("photo", ("Ventana de aluminio con doble acristalamiento en un salón", "Aluminium window with double glazing in a living room"))),
        bloque_pasos(("Cómo trabajamos", "How we work"), [
            (("Medimos", "We measure"), ("Vamos a verlo y medimos el hueco por dentro y por fuera.", "We come to see it and measure the opening from inside and outside.")),
            (("Presupuesto", "Quote"), ("Te lo damos por escrito, con el tipo de cristal o perfil y el plazo.", "We give it to you in writing, with the type of glass or frame and the lead time.")),
            (("Fabricamos", "We make it"), ("Encargamos el cristal o la ventana a medida y te avisamos cuando llega.", "We order the glass or window to measure and tell you when it arrives.")),
            (("Colocamos", "We fit it"), ("Lo instalamos, sellamos, comprobamos que cierra bien y recogemos los restos.", "We fit it, seal it, check it closes properly and take away the waste.")),
        ], tono="relleno"),
        bloque_opiniones(("Lo que dicen nuestros clientes", "What our customers say"),
                         [("la rapidez ante un cristal roto", "how quickly a broken pane was dealt with"), ("el ajuste de la ventana nueva", "the fit of the new window"), ("la limpieza al terminar", "the tidiness at the end")],
                         tono="base"),
        bloque_cta(("¿Necesitas un cristal o una ventana?", "Do you need glass or a window?"),
                   ("Llámanos o mándanos una foto con la medida aproximada.", "Call us or send a photo with the approximate size."),
                   [llamar(), whatsapp("ghost")]),
    ])

    ven = pagina("ventanas", ("ventanas", "windows"), ("Ventanas y cristales", "Windows and glass"),
                 ("Aluminio o PVC, tipos de cristal explicados en llano, precios orientativos, cómo se toman las medidas y preguntas frecuentes.",
                  "Aluminium or PVC, types of glass explained in plain words, guide prices, how measurements are taken and frequently asked questions."), [
        hero(("Ventanas y cristales", "Windows and glass"),
             ("Qué ventana te conviene, qué cristal lleva y cuánto cuesta más o menos. Sin tecnicismos.",
              "Which window suits you, which glass it takes and roughly what it costs. No technical jargon."),
             [llamar(), whatsapp()], variante="texto"),
        bloque_precios(("Precios orientativos", "Guide prices"), ("Precios orientativos según el trabajo", "Guide prices by job"),
                       [("Trabajo", "Job"), ("Precio", "Price")],
                       [(("Cristal sencillo de ventana (por pieza)", "Single pane for a window (each)"), desde(45)),
                        (("Doble acristalamiento (por metro cuadrado)", "Double glazing (per square metre)"), desde(85)),
                        (("Ventana de aluminio de 1 hoja", "Single-sash aluminium window"), desde(220)),
                        (("Ventana de PVC de 2 hojas", "Two-sash PVC window"), desde(380)),
                        (("Mampara de baño con cristal templado", "Shower screen in toughened glass"), desde(260)),
                        (("Espejo a medida (por metro cuadrado)", "Mirror to measure (per square metre)"), desde(70))],
                       nota=("Precios de ejemplo: cámbialos por los tuyos. Con la medida exacta te damos el precio final por escrito.",
                             "Example prices: change them for your own. With the exact measurement we give you the final price in writing."),
                       boton_=whatsapp("secondary", t=("Pedir presupuesto por WhatsApp", "Ask for a quote on WhatsApp")), tono="relleno"),
        bloque_valores(("Tipos de cristal, en llano", "Types of glass, in plain words"), [
            (("Sencillo", "Single"), ("Un solo cristal. Es el más barato, pero deja pasar el frío y el ruido.", "One pane. It is the cheapest, but lets cold and noise through.")),
            (("Doble con cámara", "Double with gap"), ("Dos cristales con aire o gas en medio. Aísla mejor del frío, el calor y el ruido.", "Two panes with air or gas between. It insulates better against cold, heat and noise.")),
            (("Laminado", "Laminated"), ("Dos cristales unidos por una lámina. Si se rompe, los trozos se quedan pegados. Es el que se usa en puertas y planta baja.", "Two panes bonded by a film. If it breaks the pieces stay stuck. It is used in doors and on the ground floor.")),
            (("Templado", "Toughened"), ("Es más resistente a golpes. Si se rompe, salta en trocitos pequeños que no cortan. Va en mamparas y puertas de vidrio.", "It is tougher against knocks. If it breaks it shatters into small pieces that do not cut. It goes in shower screens and glass doors.")),
        ], intro=("Si no sabes cuál necesitas, mándanos una foto y te lo decimos.", "If you are not sure which you need, send us a photo and we will tell you."),
            eyebrow=("Cristales", "Glass"), cols=2, estilo="relleno", tono="base"),
        seccion("features", [titular(("Aluminio o PVC", "Aluminium or PVC"), ("Las dos opciones valen. Esto es lo que suele pesar al elegir.", "Both are good options. This is what usually decides it."), eyebrow=("Ventanas", "Windows")),
                             tabla(("Aluminio y PVC comparados", "Aluminium and PVC compared"),
                                   [("Aspecto", "Aspect"), ("Aluminio", "Aluminium"), ("PVC", "PVC")],
                                   [(("Aislamiento", "Insulation"), ("Bueno si lleva rotura de puente térmico", "Good if it has a thermal break"), ("Muy bueno de serie", "Very good as standard")),
                                    (("Perfil", "Frame"), ("Más fino, más cristal y más luz", "Slimmer, more glass and more light"), ("Más grueso", "Chunkier")),
                                    (("Mantenimiento", "Upkeep"), ("Casi ninguno", "Almost none"), ("Limpiar con un paño", "Wipe with a cloth")),
                                    (("Precio", "Price"), ("Medio", "Medium"), ("Medio, algo menor en medidas estándar", "Medium, slightly lower in standard sizes")),
                                    (("Colores", "Colours"), ("Muchos, lacado", "Many, lacquered"), ("Blanco y acabados que imitan madera", "White and wood-effect finishes"))])],
                 tono="relleno", gap=28, anchor="aluminio-pvc"),
        bloque_checks(("Cómo se toman las medidas", "How we take measurements"), [
            ("Medimos el hueco en alto, en ancho y en las diagonales, porque las paredes rara vez están a escuadra.", "We measure the opening height, width and diagonals, because walls are rarely square."),
            ("Miramos de qué está hecho el marco actual y si hay persiana o caja.", "We check what the current frame is made of and whether there is a roller shutter or box."),
            ("Anotamos el lado por el que abre y el tipo de cierre que quieres.", "We note which way it opens and the type of catch you want."),
            ("Si solo es un cristal, basta con que nos mandes una foto con un metro al lado.", "If it is only a pane, a photo with a tape measure beside it is enough."),
        ], eyebrow=("Medidas", "Measurements"), cols=2, anchor="medidas"),
        bloque_faq(("Preguntas frecuentes", "Frequently asked questions"), [
            (("¿Cuánto tarda una ventana nueva?", "How long does a new window take?"),
             ("Se fabrica a medida, así que depende del tipo. Al aceptar el presupuesto te damos el plazo por escrito.",
              "It is made to measure, so it depends on the type. When you accept the quote we give you the lead time in writing.")),
            (("¿Hay que hacer obra?", "Does it mean building work?"),
             ("Casi nunca. Se quita la ventana vieja y se coloca la nueva en el mismo hueco, con algún remate de yeso o silicona.",
              "Hardly ever. The old window comes out and the new one goes in the same opening, with some plaster or silicone finishing.")),
            (("¿Puedo cambiar solo el cristal?", "Can I change just the glass?"),
             ("Sí, si el marco está sano. Cambiar solo el cristal por uno doble a veces no cabe en un marco antiguo: lo comprobamos al medir.",
              "Yes, if the frame is sound. A double pane sometimes does not fit an old frame: we check when we measure.")),
            (("¿Qué hago con el cristal roto mientras llegáis?", "What do I do with the broken glass until you arrive?"),
             ("No lo toques con las manos. Aparta a niños y mascotas, pon cinta en el cristal roto si está agrietado y llámanos.",
              "Do not touch it with your hands. Keep children and pets away, put tape across the cracked pane and call us.")),
            (("¿Dais garantía?", "Do you give a warranty?"),
             ("Sí. Queda por escrito en la factura, con lo que cubre y por cuánto tiempo.", "Yes. It is written on the invoice, with what it covers and for how long.")),
        ], tono="relleno"),
    ])

    trab = pagina("trabajos", ("trabajos", "work"), ("Trabajos", "Work"),
                  ("Galería de ventanas, mamparas, espejos y cristales colocados.", "Gallery of windows, shower screens, mirrors and glass fitted."), [
        hero(("Nuestros trabajos", "Our work"),
             ("Algunas ventanas, mamparas y espejos que hemos colocado.", "Some windows, shower screens and mirrors we have fitted."),
             [llamar(), whatsapp()], variante="texto"),
        bloque_galeria(("Ventanas y cerramientos", "Windows and enclosures"), [
            ("Ventanas de PVC blancas en una fachada", "White PVC windows on a façade"),
            ("Ventana de aluminio con persiana en un dormitorio", "Aluminium window with a roller shutter in a bedroom"),
            ("Cerramiento de terraza con cristal y aluminio", "Terrace enclosure in glass and aluminium"),
            ("Puerta corredera de cristal a un jardín", "Sliding glass door to a garden"),
        ], tipo="photo", cols=2, ratio=1.4, anchor="ventanas-galeria",
            intro=("Fotos de ejemplo: sustitúyelas por las de tus trabajos.", "Example photos: replace them with your own work.")),
        bloque_galeria(("Mamparas y espejos", "Shower screens and mirrors"), [
            ("Mampara de ducha corredera de cristal templado", "Sliding toughened-glass shower screen"),
            ("Mampara fija junto a un plato de ducha", "Fixed screen beside a shower tray"),
            ("Espejo de baño con el canto pulido", "Bathroom mirror with a polished edge"),
            ("Espejo de pared completa en un vestidor", "Full-wall mirror in a dressing room"),
            ("Mampara de bañera abatible", "Hinged bath screen"),
            ("Cristal en una barandilla de escalera", "Glass in a staircase balustrade"),
        ], tipo="photo", cols=3, ratio=1.0, anchor="mamparas-galeria", tono="relleno"),
        bloque_cta(("¿Quieres algo así?", "Would you like something like this?"),
                   ("Mándanos una foto del hueco y te decimos cómo lo haríamos.", "Send us a photo of the opening and we will tell you how we would do it."),
                   [llamar(), whatsapp("ghost")]),
    ])

    cont = pagina("contacto", ("presupuesto", "quote"), ("Presupuesto y urgencias", "Quote and emergencies"),
                  ("Pide presupuesto sin compromiso o avisa de un cristal roto: llama, escribe por WhatsApp o usa el formulario. Horario y cómo llegar.",
                   "Ask for a no-obligation quote or report broken glass: call, message us on WhatsApp or use the form. Opening hours and how to find us."), [
        hero(("Presupuesto sin compromiso", "No-obligation quote"),
             ("Cuéntanos qué necesitas y vamos a tomar medidas. La visita para el presupuesto no te obliga a nada. Si es un cristal roto, llama directamente.",
              "Tell us what you need and we will come to measure up. The visit for the quote does not oblige you to anything. If it is broken glass, call us directly."),
             [llamar(t=("Llamar por una urgencia", "Call about an emergency")), whatsapp()], variante="texto"),
        bloque_contacto(("Cuéntanos qué necesitas", "Tell us what you need"),
                        ("Te llamamos para acordar el día de la visita. Si tienes una foto del hueco o del cristal roto, mándala por WhatsApp.",
                         "We will call you to agree the day of the visit. If you have a photo of the opening or the broken pane, send it on WhatsApp."),
                        campos_cita(servicios=[("Cristal roto (urgente)", "Broken glass (urgent)"), ("Ventana de aluminio", "Aluminium window"),
                                               ("Ventana de PVC", "PVC window"), ("Doble acristalamiento", "Double glazing"),
                                               ("Mampara de baño", "Shower screen"), ("Espejo", "Mirror"), ("Otro trabajo", "Other job")],
                                    extra=[campo("medidas", "text", ("Medidas aproximadas (ancho x alto)", "Approximate size (width x height)"), False, maxlen=60)]),
                        ("Pedir presupuesto", "Ask for a quote"), horas,
                        exito=("Gracias. Te llamamos para acordar la visita y tomar medidas.", "Thank you. We will call you to arrange the visit and take measurements.")),
        bloque_mapa(("Dónde estamos", "Where we are"),
                    ("Vamos a tomar medidas donde esté el hueco; esta es nuestra dirección de contacto.", "We come to measure wherever the opening is; this is our contact address."),
                    tono="relleno"),
    ])

    return plantilla(KEY, ("Cristalería y ventanas", "Glazier and windows"),
                     ("Cuatro páginas: inicio con aviso de urgencias y doble acristalamiento explicado, ventanas y cristales con precios orientativos y medidas, galería de trabajos y presupuesto con llamada, WhatsApp y formulario.",
                      "Four pages: home with an emergency call-out and double glazing explained, windows and glass with guide prices and measuring, a gallery of work and a quote page with call, WhatsApp and form."),
                     "oficios", ("cristalero cristaleria vidriero cristales rotos urgencias ventanas aluminio pvc doble acristalamiento climalit mamparas bano espejos persianas cerramientos presupuesto",
                                 "glazier glass glazing broken window emergency windows aluminium pvc double glazing shower screen mirror enclosure quote"),
                     look, cabecera(nav, cta_cab),
                     pie(nav, ("Cristalería y ventanas de aluminio y PVC. Presupuesto sin compromiso y aviso de urgencias.",
                               "Glazing and aluminium and PVC windows. No-obligation quotes and an emergency call-out."),
                         ("Lunes a viernes de 9:00 a 18:30; sábados de 9:00 a 13:00.", "Monday to Friday 9 am to 6:30 pm; Saturdays 9 am to 1 pm.")),
                     [inicio, ven, trab, cont])
