# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
from lib import *

KEY = "catering"

# Violeta apagado sobre blanco con un punto de lila: sobrio y de mantel largo, nada que ver con la
# carta de un bar. Pasa AA en todos los pares.
MANTEL = dict(
    colores=dict(background="#f9f7fb", surface="#ece6f3", text="#1f1830", muted="#5a5170", primary="#5f3f86",
                 on_primary="#ffffff", border="#ddd3e8", inverse="#2a1d3e", on_inverse="#f3eefa"),
    fuentes=("source-serif-4", "work-sans"), peso=700, h1=(50, 40, 32), radio=(2, 6, 10), ancho=1120)

NOTA = ("Precios de ejemplo: cámbialos por los tuyos. Son por persona e incluyen IVA.",
        "Example prices: replace them with your own. They are per person and include VAT.")


def sec_form(titulo, intro, campos, envio, exito, anchor, tono="base"):
    return seccion("contact", [titular(titulo, intro),
                               tarjeta([formulario(campos, envio, exito)], estilo="relleno" if tono == "base" else "borde",
                                       pad=28, tag="div", sz=tam(max_width=720))],
                   tono=tono, anchor=anchor, gap=28)


def build():
    nueva(KEY)
    look = MANTEL
    nav = [("inicio", ("Inicio", "Home")), ("menus", ("Menús", "Menus")),
           ("presupuesto", ("Presupuesto", "Quote")), ("contacto", ("Contacto", "Contact"))]
    horas = [(("Lunes a viernes", "Monday to Friday"), ("9:00 a 18:00", "9:00 am to 6:00 pm")),
             (("Sábado", "Saturday"), ("Con cita previa", "By appointment")),
             (("Domingo", "Sunday"), ("Solo eventos", "Events only"))]
    cta_cab = marco([llamar(), whatsapp("ghost")], dir="row", gap=8, wrap=True, align="center")
    pedir = lambda t=("Pedir presupuesto", "Ask for a quote"): boton(t, a_pagina("presupuesto", "pedir-presupuesto"))

    datos = tarjeta([
        texto("p", ("Antes de empezar", "Before we start"), preset="label", transform="uppercase", color="$primary"),
        p(("Mínimo de 20 comensales y, para un menú servido, 30. Pide con 15 días de antelación; con menos, pregúntanos y vemos si llegamos.",
           "A minimum of 20 guests and, for a served menu, 30. Ask 15 days ahead; with less notice, ask and we will see if we can make it.")),
        peq(("Condiciones de ejemplo: cámbialas por las tuyas.", "Example terms: change them to your own."), color="$muted"),
    ], estilo="borde", tag="div", gap=8, pad=20)

    inicio = pagina("inicio", ("", ""), ("Catering para eventos y empresas", "Catering for events and companies"),
                    ("Catering para eventos y empresas: tipos de evento, menús de cóctel, buffet y servido, personal y montaje, y botón de pedir presupuesto.",
                     "Catering for events and companies: types of event, cocktail, buffet and served menus, staff and set-up, and a button to ask for a quote."), [
        hero(("Catering para tu evento, de la cocina a la mesa", "Catering for your event, from our kitchen to your table"),
             ("Cocinamos, llevamos, montamos y servimos. Tú nos dices la fecha, cuántos sois y dónde, y te mandamos un presupuesto sin compromiso.",
              "We cook, deliver, set up and serve. You tell us the date, how many of you there are and where, and we send you a quote with no commitment."),
             [pedir(), llamar("secondary"), whatsapp("ghost")],
             eyebrow=("Eventos y empresas", "Events and companies"), variante="texto", tono="relleno", extra=datos),
        bloque_valores(("Para qué eventos trabajamos", "The events we work on"), [
            (("Eventos de empresa", "Company events"), ("Comidas de equipo, jornadas, presentaciones y cafés de bienvenida en tu oficina o en un local.", "Team lunches, conferences, launches and welcome coffees at your office or a venue.")),
            (("Celebraciones familiares", "Family celebrations"), ("Cumpleaños, comuniones, bautizos y aniversarios, en casa o en un salón.", "Birthdays, communions, christenings and anniversaries, at home or in a hall.")),
            (("Bodas", "Weddings"), ("Aperitivo, cena o cóctel. Hablamos con quien lleve la organización del día.", "Drinks reception, dinner or cocktail. We work with whoever is organising the day.")),
            (("Inauguraciones y presentaciones", "Openings and launches"), ("Un cóctel de pie, bien pensado para que se pueda charlar.", "A standing cocktail, planned so that people can chat.")),
            (("Colegios, asociaciones y clubes", "Schools, associations and clubs"), ("Fiestas de fin de curso, comidas solidarias y encuentros.", "End-of-year parties, charity lunches and get-togethers.")),
            (("Ferias y jornadas", "Fairs and open days"), ("Servicio de comida y bebida durante todo el día.", "Food and drink service all day long.")),
        ], cols=3, estilo="relleno", anchor="eventos", intro=("Si tu evento no está aquí, cuéntanoslo y vemos qué se puede hacer.", "If your event is not here, tell us and we will see what we can do.")),
        bloque_valores(("Tres formas de servir", "Three ways to serve"), [
            (("Cóctel", "Cocktail"), ("Canapés y bocados que se comen de pie. Ideal para recibir y charlar.", "Canapés and bites eaten standing. Ideal for welcoming guests and chatting.")),
            (("Buffet", "Buffet"), ("Todo montado en mesas y cada persona se sirve. Cómodo si hay muchos invitados.", "Everything laid out on tables and everyone helps themselves. Easy with large groups.")),
            (("Servido", "Served"), ("Platos a la mesa, con personal de sala. Para cenas y comidas más formales.", "Plated dishes at the table, with waiting staff. For more formal dinners and lunches.")),
        ], cols=3, estilo="borde", anchor="servicios",
            intro=("Cada uno tiene su menú, su precio por persona y su personal. Míralos con detalle en la página de menús.", "Each has its own menu, price per person and staff. See them in detail on the menus page.")),
        bloque_checks(("Qué incluye el servicio", "What the service includes"), [
            ("Cocina y transporte de la comida hasta el lugar del evento.", "Cooking and transport of the food to the venue."),
            ("Personal de cocina y de sala durante todo el servicio.", "Kitchen and service staff throughout."),
            ("Montaje de mesas, mantelería, vajilla y cristalería, según el tipo de servicio.", "Setting up tables, linen, crockery and glassware, depending on the type of service."),
            ("Recogida y limpieza de lo que traemos al terminar.", "Collecting and cleaning what we bring when we finish."),
        ], cols=2, tono="relleno", anchor="incluye", intro=("Lo que va en cada presupuesto se detalla por escrito: nada se da por supuesto.", "What goes in each quote is spelled out in writing: nothing is taken for granted.")),
        bloque_opiniones(("Lo que dicen quienes nos han contratado", "What people who have hired us say"),
                         [("el menú", "the menu"), ("el servicio en sala", "the service"), ("la puntualidad", "the timing")], tono="base"),
        bloque_cta(("Cuéntanos qué quieres celebrar", "Tell us what you want to celebrate"),
                   ("Fecha, número de personas y lugar. Con eso ya podemos empezar a hacerte un presupuesto.",
                    "Date, number of guests and place. With that we can start working on a quote."),
                   [pedir(), llamar("secondary")], tono="relleno"),
    ])

    menus = pagina("menus", ("menus", "menus"), ("Menús y precios por persona", "Menus and prices per person"),
                   ("Menús de cóctel, buffet y servido con lo que incluye cada uno, precios de ejemplo por persona, alérgenos y preguntas frecuentes.",
                    "Cocktail, buffet and served menus with what each includes, example prices per person, allergens and frequently asked questions."), [
        hero(("Menús para eventos", "Menus for events"),
             ("Tres formatos y varios menús dentro de cada uno. Son una base: los ajustamos a tu fecha, a tu presupuesto y a tus invitados.",
              "Three formats and several menus within each. They are a starting point: we adapt them to your date, your budget and your guests."),
             [pedir(), whatsapp("secondary")], variante="texto"),
        bloque_carta(("Qué lleva cada menú", "What each menu includes"), [
            (("Cóctel (de pie)", "Cocktail (standing)"), ("Menú de cóctel", "Cocktail menu"), [("Pase", "Course"), ("Ejemplos", "Examples")],
             [(("Canapés fríos", "Cold canapés"), ("Salmón y queso crema, jamón y tomate, queso y membrillo", "Salmon and cream cheese, ham and tomato, cheese and quince")),
              (("Bocados calientes", "Hot bites"), ("Croquetas, mini quiches, brochetas de pollo", "Croquettes, mini quiches, chicken skewers")),
              (("Cucharitas", "Spoon bites"), ("Ensaladilla, hummus con verduras, tartar de atún", "Russian salad, hummus with vegetables, tuna tartare")),
              (("Dulce", "Sweet"), ("Mini tartas, brownies y fruta cortada", "Mini cakes, brownies and cut fruit"))]),
            (("Buffet", "Buffet"), ("Menú de buffet", "Buffet menu"), [("Zona", "Station"), ("Ejemplos", "Examples")],
             [(("Para empezar", "To start"), ("Ensaladas, embutidos y quesos, tortilla", "Salads, cured meats and cheeses, omelette")),
              (("Plato principal", "Main dish"), ("Dos platos calientes a elegir: carne, pescado o arroz", "Two hot dishes to choose from: meat, fish or rice")),
              (("Guarnición", "Sides"), ("Patatas, verduras asadas y pan", "Potatoes, roast vegetables and bread")),
              (("Postres", "Desserts"), ("Tartas, fruta y café", "Cakes, fruit and coffee"))]),
            (("Servido (a la mesa)", "Served (at the table)"), ("Menú servido", "Served menu"), [("Plato", "Course"), ("Ejemplos", "Examples")],
             [(("Entrante", "Starter"), ("Crema de temporada o ensalada templada", "Seasonal cream soup or warm salad")),
              (("Principal", "Main"), ("A elegir entre carne, pescado o una opción vegetariana", "To choose from meat, fish or a vegetarian option")),
              (("Postre", "Dessert"), ("Tarta o fruta de temporada", "Cake or seasonal fruit")),
              (("Bebidas", "Drinks"), ("Agua, vino, refrescos y café", "Water, wine, soft drinks and coffee"))]),
        ], nota=("Los platos son ejemplos: cámbialos por los que cocines. Cada menú se cierra contigo en el presupuesto, según la época del año y los productos de mercado.",
                 "The dishes are examples: change them to the ones you cook. Each menu is agreed with you in the quote, depending on the time of year and market produce."),
            tono="base", anchor="menus"),
        bloque_precios(("Precios por persona", "Prices per person"), ("Precios de ejemplo por persona y por tipo de menú", "Example prices per person by type of menu"),
                       [("Menú", "Menu"), ("Mínimo de comensales", "Minimum guests"), ("Precio por persona", "Price per person")],
                       [(("Cóctel de pie", "Standing cocktail"), ("20", "20"), desde(18)),
                        (("Buffet", "Buffet"), ("20", "20"), desde(28)),
                        (("Menú servido", "Served menu"), ("30", "30"), desde(42)),
                        (("Café de bienvenida o pausa", "Welcome coffee or break"), ("15", "15"), desde(6)),
                        (("Barra de bebidas", "Drinks bar"), ("30", "30"), ("Según consumo", "Depending on consumption"))],
                       intro=("El precio incluye la comida, el personal y el montaje. Alquilar mobiliario, flores o música se presupuesta aparte si lo necesitas.",
                              "The price includes food, staff and set-up. Hiring furniture, flowers or music is quoted separately if you need it."),
                       nota=NOTA, tono="relleno", anchor="precios"),
        seccion("text", [
            titular(("Alérgenos y dietas", "Allergens and diets"),
                    ("Pregúntanos a la hora de pedir el presupuesto. Para cada menú te damos la lista de alérgenos de cada plato. Podemos preparar opciones vegetarianas, veganas, sin gluten o sin lactosa, pero cocinamos con harina, frutos secos y leche en la misma cocina, así que puede haber trazas.",
                     "Ask when you request the quote. For each menu we give you the allergen list for every dish. We can prepare vegetarian, vegan, gluten-free or lactose-free options, but we cook with flour, nuts and milk in the same kitchen, so traces are possible.")),
            tabla(("Opciones para dietas y alergias", "Options for diets and allergies"),
                  [("Necesidad", "Need"), ("Qué hacemos", "What we do")],
                  [(("Vegetariana o vegana", "Vegetarian or vegan"), ("Plato alternativo en cada menú", "An alternative dish in every menu")),
                   (("Sin gluten", "Gluten-free"), ("Platos adaptados, sin pan y sin rebozados; te avisamos de las trazas", "Adapted dishes, without bread or batter; we tell you about traces")),
                   (("Sin lactosa", "Lactose-free"), ("Platos y postres sin leche, según el menú", "Dishes and desserts without milk, depending on the menu")),
                   (("Alergias graves", "Serious allergies"), ("Lo hablamos por teléfono antes de cerrar el menú", "We talk it over by phone before settling the menu"))]),
            peq(("Alérgenos de ejemplo: cambia la tabla por la que corresponda a tu cocina y revísala cuando cambies una receta.",
                 "Example allergens: replace the table with the one that applies to your kitchen and review it when you change a recipe."), color="$muted"),
        ], tono="base", anchor="alergenos", gap=24),
        bloque_faq(("Preguntas frecuentes", "Frequently asked questions"), [
            (("¿Podéis adaptar un menú a mi presupuesto?", "Can you adapt a menu to my budget?"),
             ("Sí. Dinos cuánto quieres gastar por persona y buscamos qué platos entran y cuáles se quedan fuera.",
              "Yes. Tell us how much you want to spend per person and we work out which dishes fit and which do not.")),
            (("¿Qué pasa si vienen menos invitados de los previstos?", "What if fewer guests come than planned?"),
             ("Fijamos un número cerrado unos días antes y a partir de ahí se factura. Si luego faltan algunos, el menú ya está hecho.",
              "We agree a final number a few days beforehand and bill from there. If a few people then do not come, the food has already been made.")),
            (("¿Cómo se paga?", "How do I pay?"),
             ("Una señal para reservar la fecha y el resto antes del evento. Condiciones de ejemplo: cámbialas por las tuyas.",
              "A deposit to reserve the date and the rest before the event. Example terms: change them to your own.")),
            (("¿Qué pasa si hay que cancelar?", "What if I have to cancel?"),
             ("Lo ponemos por escrito en el presupuesto: hasta cuándo se puede cancelar y qué se devuelve.",
              "We put it in writing in the quote: until when you can cancel and what is refunded.")),
        ], tono="relleno"),
        bloque_cta(("¿Qué menú encaja con tu evento?", "Which menu fits your event?"), None, [pedir(), llamar("secondary")], tono="base"),
    ])

    presupuesto = pagina("presupuesto", ("presupuesto", "quote"), ("Presupuesto sin compromiso", "Quote with no commitment"),
                         ("Cómo pedir un presupuesto en pasos, mínimo de comensales y antelación, personal y montaje, y formulario con fecha, comensales y lugar.",
                          "How to ask for a quote in steps, minimum guests and lead time, staff and set-up, and a form with date, guests and venue."), [
        hero(("Pide tu presupuesto", "Ask for your quote"),
             ("Sin compromiso y por escrito. Te contestamos con el menú propuesto, el precio por persona y lo que incluye.",
              "No commitment and in writing. We reply with the proposed menu, the price per person and what is included."),
             [boton(("Ir al formulario", "Go to the form"), a_ancla("pedir-presupuesto")), llamar("secondary"), whatsapp("ghost")], variante="texto"),
        bloque_pasos(("Cómo se pide un presupuesto", "How to ask for a quote"), [
            (("Nos cuentas lo básico", "You tell us the basics"), ("Fecha, número de comensales, lugar y tipo de evento.", "Date, number of guests, venue and type of event.")),
            (("Hablamos del menú", "We talk about the menu"), ("Te llamamos o escribimos para ver gustos, alergias y presupuesto.", "We call or message you to go over tastes, allergies and budget.")),
            (("Te mandamos el presupuesto", "We send you the quote"), ("Por escrito, con el menú, el personal, el montaje y el precio.", "In writing, with the menu, staff, set-up and price.")),
            (("Reservamos la fecha", "We book the date"), ("Con una señal quedan reservados el día y el equipo.", "With a deposit the day and the team are reserved.")),
        ], tono="relleno", anchor="pasos"),
        bloque_precios(("Mínimo de comensales y antelación", "Minimum guests and lead time"),
                       ("Mínimo de comensales y antelación por tipo de servicio", "Minimum guests and lead time by type of service"),
                       [("Servicio", "Service"), ("Comensales mínimos", "Minimum guests"), ("Antelación recomendada", "Recommended lead time")],
                       [(("Café o pausa", "Coffee or break"), ("15", "15"), ("3 días", "3 days")),
                        (("Cóctel de pie", "Standing cocktail"), ("20", "20"), ("10 días", "10 days")),
                        (("Buffet", "Buffet"), ("20", "20"), ("15 días", "15 days")),
                        (("Menú servido", "Served menu"), ("30", "30"), ("20 días", "20 days")),
                        (("Bodas y eventos de más de 100 personas", "Weddings and events of over 100 guests"), ("100", "100"), ("Lo antes posible", "As early as possible"))],
                       intro=("Mínimos y plazos de ejemplo: cámbialos por los tuyos. Si te falta tiempo, llámanos: a veces se puede.",
                              "Example minimums and lead times: change them to your own. If you are short of time, call us: sometimes it can be done."),
                       nota=("Condiciones de ejemplo: cámbialas por las tuyas.", "Example terms: change them to your own."),
                       tono="base", anchor="minimos"),
        bloque_valores(("Personal y montaje", "Staff and set-up"), [
            (("Equipo de cocina", "Kitchen team"), ("Cocinamos en nuestro obrador y terminamos los platos en el lugar si hace falta.", "We cook in our own kitchen and finish the dishes on site if needed.")),
            (("Personal de sala", "Service staff"), ("Un camarero por cada 25 comensales en un servido o un buffet, y más en un cóctel con barra. Cifra de ejemplo: cámbiala por la tuya.", "One waiter for every 25 guests at a served meal or a buffet, and more at a cocktail with a bar. Example figure: change it to your own.")),
            (("Montaje y recogida", "Set-up and clear-up"), ("Llegamos con tiempo para montar y nos llevamos todo lo nuestro al terminar. Te pedimos acceso, un enchufe y agua cerca.", "We arrive in good time to set up and take away everything that is ours when we finish. We ask for access, a socket and water nearby.")),
        ], cols=3, tono="relleno", estilo="borde", anchor="personal"),
        sec_form(("Cuéntanos tu evento", "Tell us about your event"),
                 ("Con estos datos podemos empezar. Si algo no lo sabes todavía, déjalo en blanco y lo hablamos.",
                  "With this we can get started. If you do not know something yet, leave it blank and we will talk it over."),
                 [campo("nombre", "text", ("Nombre y empresa (si la hay)", "Name and company (if any)"), True, auto="name", maxlen=160),
                  campo("telefono", "tel", ("Teléfono", "Phone"), True, auto="tel", maxlen=30),
                  campo("correo", "email", ("Correo electrónico", "Email"), True, auto="email", maxlen=200),
                  campo("evento", "select", ("Tipo de evento", "Type of event"), False,
                        opciones=[("Evento de empresa", "Company event"), ("Celebración familiar", "Family celebration"), ("Boda", "Wedding"),
                                  ("Inauguración o presentación", "Opening or launch"), ("Otro", "Other")]),
                  campo("servicio", "select", ("Tipo de servicio", "Type of service"), False,
                        opciones=[("Cóctel de pie", "Standing cocktail"), ("Buffet", "Buffet"), ("Menú servido", "Served menu"), ("No lo sé todavía", "Not sure yet")]),
                  campo("fecha", "date", ("Fecha del evento", "Event date"), True),
                  campo("comensales", "text", ("Número de comensales", "Number of guests"), True, maxlen=10),
                  campo("lugar", "text", ("Lugar del evento", "Event venue"), True, maxlen=200),
                  campo("mensaje", "textarea", ("Alergias, dietas o cualquier otra cosa que debamos saber (opcional)", "Allergies, diets or anything else we should know (optional)"), False, maxlen=1000)],
                 ("Pedir presupuesto", "Request a quote"),
                 ("Gracias. Hemos recibido tu petición y te contestamos con una propuesta de menú y de precio lo antes posible.",
                  "Thank you. We have received your request and will reply with a menu and price proposal as soon as possible."),
                 "pedir-presupuesto", tono="base"),
    ])

    cont = pagina("contacto", ("contacto", "contact"), ("Contacto y cómo llegar", "Contact and how to get here"),
                  ("Teléfono, WhatsApp, formulario, horario de atención, número de registro sanitario y mapa del obrador.",
                   "Phone, WhatsApp, form, opening hours, health registration number and a map of the kitchen."), [
        hero(("Contacto", "Contact"),
             ("Llámanos, escríbenos por WhatsApp o ven al obrador con cita previa para hablar del menú y probar algunos platos.",
              "Call us, message us on WhatsApp or visit our kitchen by appointment to talk about the menu and taste a few dishes."),
             [llamar(), whatsapp()], variante="texto"),
        bloque_contacto(("Habla con nosotros", "Talk to us"),
                        ("Para pedir presupuesto, usa la página de presupuesto. Para cualquier otra cosa, aquí.",
                         "To ask for a quote, use the quote page. For anything else, here."),
                        campos_consulta(servicios=[("Pedir información", "Ask for information"), ("Visitar el obrador", "Visit the kitchen"),
                                                   ("Un evento ya contratado", "An event already booked"), ("Otra cosa", "Something else")]),
                        ("Enviar mensaje", "Send message"), horas,
                        notas=("Horario de ejemplo de la oficina: cámbialo por el tuyo. Nuestro número de registro sanitario es el 00-0000 (marcador: cámbialo por el tuyo).",
                               "Example office hours: change them to your own. Our health registration number is 00-0000 (placeholder: change it to yours).")),
        bloque_mapa(("Cómo llegar al obrador", "How to get to our kitchen"),
                    ("El mapa señala el obrador. Para visitarnos, avísanos antes: por el trabajo de cocina no siempre hay alguien en la puerta.",
                     "The map shows our kitchen. To visit, tell us first: because of the kitchen work there is not always someone at the door."),
                    tono="relleno"),
    ])

    return plantilla(KEY, ("Empresa de catering", "Catering company"),
                     ("Tipos de evento, menús de cóctel, buffet y servido con precios por persona, personal y montaje, alérgenos, cómo pedir presupuesto en pasos y formulario con fecha, comensales y lugar, con botones de llamar y WhatsApp.",
                      "Types of event, cocktail, buffet and served menus with prices per person, staff and set-up, allergens, how to ask for a quote in steps and a form with date, guests and venue, with call and WhatsApp buttons."),
                     "comida", ("catering empresa de catering eventos comida para eventos banquetes buffet coctel menu servido bodas comuniones comidas de empresa presupuesto comensales",
                                "catering caterer company event food banquet buffet cocktail served menu weddings communions corporate lunch quote guests"),
                     look, cabecera(nav, cta_cab),
                     pie(nav, ("Empresa de catering. Menús para eventos y empresas, con personal y montaje. Nº de registro sanitario: 00-0000.",
                               "Catering company. Menus for events and companies, with staff and set-up. Health registration no.: 00-0000."),
                         ("Oficina de lunes a viernes, de 9:00 a 18:00.", "Office Monday to Friday, 9 am to 6 pm.")),
                     [inicio, menus, presupuesto, cont])
