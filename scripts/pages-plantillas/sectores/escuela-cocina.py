# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
from lib import *
from looks import LOOKS

KEY = "escuela-cocina"

# Rosa tenue y rojo ladrillo oscuro: ninguna otra plantilla los usa (comprobado con grep de primary=).
ALBAHACA = dict(background="#ffe0e6", surface="#fcc9d3", text="#2a1410", muted="#5a3630", primary="#7c2d12",
                on_primary="#ffffff", border="#efaebb", inverse="#3a1410", on_inverse="#fff0f2")


def S(x):
    return (x, x)


def build():
    nueva(KEY)
    look = dict(LOOKS["terracota"], colores=ALBAHACA, fuentes=("playfair-display", "noto-sans"), radio=(4, 6, 10))
    nav = [("inicio", ("Inicio", "Home")), ("cursos", ("Cursos y talleres", "Courses and workshops")),
           ("regalos", ("Regalos y empresas", "Gifts and companies")), ("contacto", ("Contacto", "Contact"))]
    horas = [(("Martes a viernes", "Tuesday to Friday"), ("De 10:00 a 14:00 (oficina)", "10:00 am to 2:00 pm (office)")),
             (("Talleres de tarde", "Evening workshops"), ("De 18:30 a 21:30", "6:30 pm to 9:30 pm")),
             (("Sábado", "Saturday"), ("Talleres de 10:30 a 14:00", "Workshops 10:30 am to 2:00 pm")),
             (("Domingo y lunes", "Sunday and Monday"), ("Cerrado", "Closed"))]
    cta_cab = marco([llamar(t=("Llamar", "Call")), whatsapp("secondary", t=("WhatsApp", "WhatsApp"))],
                    dir="row", gap=8, wrap=True)
    prueba = lambda v="primary": boton(("Reservar plaza en un taller", "Book a place on a workshop"),
                                       a_pagina("contacto", "contacto"), v)

    inicio = pagina("inicio", ("", ""), ("Escuela de cocina", "Cooking school"),
                    ("Escuela de cocina con talleres de una tarde y cursos de varias semanas, grupos reducidos, regalo de talleres y eventos de empresa.",
                     "Cooking school with one-evening workshops and multi-week courses, small groups, workshop gift vouchers and company events."), [
        hero(("Aprende a cocinar con las manos en la masa", "Learn to cook hands-on"),
             ("Talleres de una tarde y cursos de varias semanas, en grupos pequeños. Cocinas, comes lo que has hecho y te llevas la receta.",
              "One-evening workshops and multi-week courses in small groups. You cook, eat what you have made and take the recipe home."),
             [prueba(), llamar("secondary"), whatsapp("ghost")],
             eyebrow=("Escuela de cocina y talleres", "Cooking school and workshops"),
             variante="centro", tono="relleno"),
        bloque_valores(("Qué se cocina aquí", "What is cooked here"), [
            (("Cocina de diario", "Everyday cooking"),
             ("Platos para el día a día: sofritos, guisos, legumbres y cómo organizar la semana para cocinar menos y mejor.",
              "Dishes for every day: sautés, stews, pulses and how to plan the week to cook less and better.")),
            (("Pan y repostería", "Bread and baking"),
             ("Masas, fermentos, bizcochos y tartas. Aprendes por qué sube una masa y qué hacer cuando no sube.",
              "Doughs, ferments, sponges and cakes. You learn why a dough rises and what to do when it does not.")),
            (("Cocina del mundo", "World cooking"),
             ("Un país por taller: especias, técnicas y platos que luego puedes repetir en casa con ingredientes del barrio.",
              "One country per workshop: spices, techniques and dishes you can repeat at home with local ingredients.")),
            (("Cocina vegetal", "Plant-based cooking"),
             ("Verduras, cereales y legumbres como protagonistas, con platos que sacian de verdad.",
              "Vegetables, grains and pulses as the stars, with dishes that really satisfy.")),
        ], eyebrow=("Temas", "Topics"), cols=2, estilo="linea", tono="base", anchor="temas"),
        bloque_texto(("Grupos reducidos, cada uno en su fogón", "Small groups, each at their own hob"), [
            ("Cada taller se limita a un grupo pequeño, para que el profesor pueda ver cómo cortas, cómo remueves y cuándo se pasa la cebolla. Si cocinas con otro, os ponemos en pareja; si prefieres ir solo, también.",
             "Each workshop is limited to a small group so the teacher can see how you chop, how you stir and when the onion is done. If you cook with someone else we pair you up; if you prefer to go alone, that is fine too."),
            ("Al final nos sentamos a la mesa y comemos lo que hemos cocinado, con una copa de vino o un refresco.",
             "At the end we sit at the table and eat what we have cooked, with a glass of wine or a soft drink."),
        ], imagen_=("photo", ("Mesa de trabajo con tablas de cortar, verduras troceadas y cuencos de madera, sin personas",
                              "Worktable with chopping boards, chopped vegetables and wooden bowls, with no people")),
            tono="base", invertir=True),
        bloque_checks(("Qué incluye cada taller", "What each workshop includes"), [
            ("Ingredientes, utensilios y delantal.", "Ingredients, utensils and an apron."),
            ("La receta por escrito, para repetirla en casa.", "The recipe in writing, to make again at home."),
            ("Comer lo que has cocinado, sentado a la mesa.", "Eating what you have cooked, sitting at the table."),
            ("Un táper para llevarte lo que sobre.", "A container to take away what is left."),
        ], eyebrow=("Todo incluido", "All included"), tono="relleno", cols=2),
        bloque_pasos(("Cómo es una tarde de taller", "What a workshop evening is like"), [
            (("Llegas y te lavas las manos", "You arrive and wash your hands"), ("Te damos el delantal y repasamos el menú de la noche.", "We give you an apron and go through the evening's menu.")),
            (("Preparamos juntos", "We prepare together"), ("El profesor enseña cada técnica y tú la haces en tu puesto.", "The teacher shows each technique and you do it at your station.")),
            (("Cocinamos", "We cook"), ("Fuego, horno y emplatado. Si algo se pega, lo arreglamos entre todos.", "Hob, oven and plating. If something sticks, we fix it together.")),
            (("Comemos", "We eat"), ("A la mesa, con lo que habéis hecho. Te llevas la receta.", "At the table, with what you have made. You take the recipe home.")),
        ], tono="base"),
        bloque_opiniones(("Lo que cuentan quienes han venido", "What people who came say"),
                         [("un taller de una tarde", "a one-evening workshop"), ("un curso de pan", "a bread course"),
                          ("una actividad de empresa", "a company activity")], tono="relleno"),
        bloque_cta(("¿Reservamos tu plaza?", "Shall we book your place?"),
                   ("Dinos qué taller te apetece y cuántos vais. Te contestamos con las fechas que quedan.",
                    "Tell us which workshop you fancy and how many of you are coming. We will reply with the dates left."),
                   [prueba(), llamar("ghost")], tono="base"),
    ])

    cursos = pagina("cursos", ("cursos", "courses"), ("Cursos y talleres", "Courses and workshops"),
                    ("Talleres de una tarde y cursos de varias semanas con nivel, duración y precio de ejemplo, calendario, alérgenos y preguntas frecuentes.",
                     "One-evening workshops and multi-week courses with example level, length and price, calendar, allergens and frequently asked questions."), [
        hero(("Cursos y talleres", "Courses and workshops"),
             ("Elige entre un taller suelto de una tarde y un curso de varias semanas. Aquí ves el nivel, lo que dura y lo que cuesta.",
              "Choose between a single evening workshop and a course of several weeks. Here you see the level, the length and the price."),
             [prueba()], variante="texto"),
        bloque_carta(("Talleres y cursos", "Workshops and courses"), [
            (("Talleres de una tarde", "One-evening workshops"), ("Talleres de ejemplo", "Example workshops"),
             [("Taller", "Workshop"), ("Nivel", "Level"), ("Duración", "Length"), ("Precio", "Price")],
             [(("Cocina italiana: pasta fresca y salsas", "Italian cooking: fresh pasta and sauces"), ("Todos los niveles", "All levels"), ("3 horas", "3 hours"), eur(55)),
              (("Cocina tailandesa y pasta de curry", "Thai cooking and curry paste"), ("Todos los niveles", "All levels"), ("3 horas", "3 hours"), eur(60)),
              (("Sushi y arroces", "Sushi and rice dishes"), ("Iniciación", "Beginner"), ("3 horas", "3 hours"), eur(65)),
              (("Tapas y cocina de fiesta", "Tapas and party cooking"), ("Todos los niveles", "All levels"), ("3 horas", "3 hours"), eur(50)),
              (("Cocina para cada día con legumbres", "Everyday cooking with pulses"), ("Todos los niveles", "All levels"), ("2,5 horas", "2.5 hours"), eur(45))]),
            (("Cursos de varias semanas", "Multi-week courses"), ("Cursos de ejemplo", "Example courses"),
             [("Curso", "Course"), ("Nivel", "Level"), ("Duración", "Length"), ("Precio", "Price")],
             [(("Cocina básica", "Basic cooking"), ("Principiante", "Beginner"), ("6 sesiones de 3 horas", "6 sessions of 3 hours"), eur(280)),
              (("Pan en casa", "Home bread"), ("Principiante", "Beginner"), ("4 sesiones de 3 horas", "4 sessions of 3 hours"), eur(190)),
              (("Repostería y tartas", "Baking and cakes"), ("Intermedio", "Intermediate"), ("5 sesiones de 3 horas", "5 sessions of 3 hours"), eur(250)),
              (("Cocina de temporada", "Seasonal cooking"), ("Intermedio", "Intermediate"), ("8 sesiones de 3 horas", "8 sessions of 3 hours"), eur(420))]),
        ], intro=("Los niveles se leen así: principiante, si nunca has cocinado; intermedio, si ya te defiendes con lo básico; todos los niveles, si puedes venir sin más.",
                  "Levels read like this: beginner, if you have never cooked; intermediate, if you can already manage the basics; all levels, if you can simply come."),
            nota=("Precios de ejemplo: cámbialos por los tuyos. Indica aquí qué incluye el precio y si hay descuento por traer a un amigo.",
                  "Example prices: replace them with your own. Say here what the price includes and whether there is a discount for bringing a friend."),
            anchor="cursos", tono="base"),
        bloque_precios(("Calendario", "Calendar"), ("Calendario de ejemplo", "Example calendar"),
                       [("Día", "Day"), ("Taller o curso", "Workshop or course"), ("Horario", "Time")],
                       [(("Primer jueves del mes", "First Thursday of the month"), ("Cocina del mundo", "World cooking"), ("De 18:30 a 21:30", "6:30 pm to 9:30 pm")),
                        (("Segundo sábado del mes", "Second Saturday of the month"), ("Pan y repostería", "Bread and baking"), ("De 10:30 a 13:30", "10:30 am to 1:30 pm")),
                        (("Tercer miércoles del mes", "Third Wednesday of the month"), ("Cocina vegetal", "Plant-based cooking"), ("De 18:30 a 21:30", "6:30 pm to 9:30 pm")),
                        (("Cada martes del curso", "Every Tuesday of the course"), ("Cocina básica", "Basic cooking"), ("De 19:00 a 22:00", "7:00 pm to 10:00 pm"))],
                       nota=("Calendario de ejemplo: cámbialo por tus fechas reales.", "Example calendar: replace it with your real dates."),
                       anchor="calendario", kind="text", tono="relleno"),
        bloque_texto(("Alérgenos e intolerancias", "Allergens and intolerances"), [
            ("En la cocina se manejan a la vez gluten, huevo, lácteos, frutos secos, pescado, marisco, soja, sésamo y otros alérgenos. No podemos asegurar que un plato esté libre de trazas.",
             "Gluten, egg, dairy, nuts, fish, shellfish, soya, sesame and other allergens are handled in the kitchen at the same time. We cannot guarantee that a dish is free of traces."),
            ("Cada taller trae el listado de alérgenos de sus recetas. Si tienes alergia o intolerancia, dínoslo al reservar y vemos si el taller te sirve o cómo adaptarlo.",
             "Every workshop comes with the allergen list for its recipes. If you have an allergy or intolerance, tell us when you book and we will see whether the workshop suits you or how to adapt it."),
        ], eyebrow=("Importante", "Important"), tono="base", anchor="alergenos"),
        bloque_faq(("Preguntas frecuentes", "Frequently asked questions"), [
            (("¿Hace falta saber cocinar?", "Do I need to know how to cook?"),
             ("No. Los talleres de todos los niveles empiezan por lo básico y el profesor va a tu ritmo.",
              "No. All-level workshops begin with the basics and the teacher goes at your pace.")),
            (("¿Qué pasa si no puedo ir?", "What if I cannot come?"),
             ("Si avisas con unos días de margen, cambiamos tu plaza a otra fecha. Si no, se pierde. Cambia esta frase por tu política de cancelación.",
              "If you tell us a few days ahead, we move your place to another date. Otherwise it is lost. Change this sentence for your cancellation policy.")),
            (("¿Puedo ir con un amigo?", "Can I come with a friend?"),
             ("Sí. Reserva dos plazas y os ponemos en el mismo puesto de cocina.",
              "Yes. Book two places and we will put you at the same cooking station.")),
            (("¿Qué debo llevar?", "What should I bring?"),
             ("Solo ganas y zapato cerrado. El delantal, los utensilios y los ingredientes los ponemos nosotros.",
              "Just enthusiasm and closed shoes. We provide the apron, utensils and ingredients.")),
        ], tono="relleno"),
        bloque_cta(("Reserva tu taller", "Book your workshop"), None, [prueba(), whatsapp("ghost")], tono="base"),
    ])

    regalos = pagina("regalos", ("regalos-y-empresas", "gifts-and-companies"), ("Regalos y empresas", "Gifts and companies"),
                     ("Bonos regalo de talleres y cursos, actividades de empresa, cumpleaños y despedidas en la cocina de la escuela, con presupuesto sin compromiso.",
                      "Workshop and course gift vouchers, company activities, birthdays and send-offs in the school kitchen, with no-obligation quotes."), [
        hero(("Regala un taller o cocinad juntos", "Give a workshop or cook together"),
             ("Un bono para quien ya lo tiene todo, o una tarde de cocina para vuestro equipo o vuestra celebración.",
              "A voucher for someone who has everything, or an evening in the kitchen for your team or your celebration."),
             [boton(("Pedir un bono regalo", "Ask for a gift voucher"), a_pagina("contacto", "contacto")),
              llamar("secondary")], variante="texto"),
        bloque_precios(("Bonos regalo", "Gift vouchers"), ("Bonos regalo de ejemplo", "Example gift vouchers"),
                       [("Bono", "Voucher"), ("Para quién", "Who it is for"), ("Precio", "Price")],
                       [(("Un taller de una tarde", "One evening workshop"), ("Una persona", "One person"), eur(55)),
                        (("Taller para dos", "Workshop for two"), ("Dos personas", "Two people"), eur(100)),
                        (("Curso de pan en casa", "Home bread course"), ("Una persona", "One person"), eur(190)),
                        (("Importe libre", "Free amount"), ("Quien lo reciba elige taller", "The recipient chooses a workshop"), ("Desde 30 €", "From €30"))],
                       intro=("Lo imprimimos en una tarjeta o te lo mandamos por correo. Quien lo recibe elige el día entre los talleres con plaza.",
                              "We print it on a card or email it to you. The recipient picks the day from the workshops with places left."),
                       nota=("Precios de ejemplo: cámbialos por los tuyos. Indica aquí cuánto tiempo vale el bono.",
                             "Example prices: replace them with your own. Say here how long the voucher is valid."),
                       anchor="bonos", tono="base"),
        bloque_valores(("Cocina para equipos y celebraciones", "Cooking for teams and celebrations"), [
            (("Eventos de empresa", "Company events"),
             ("Una tarde en que el equipo cocina junto y come lo que ha hecho. El menú lo elegimos con vosotros.",
              "An evening where the team cooks together and eats what they have made. We choose the menu with you.")),
            (("Cumpleaños y despedidas", "Birthdays and send-offs"),
             ("Para grupos de amigos o familia, con un menú a vuestra medida y la cocina para vosotros.",
              "For groups of friends or family, with a menu made to measure and the kitchen to yourselves.")),
            (("Cocina con niños", "Cooking with children"),
             ("Talleres de pizzas, galletas o bocadillos creativos para cumpleaños o vacaciones, siempre con un adulto de cada familia.",
              "Pizza, biscuit or creative sandwich workshops for birthdays or holidays, always with an adult from each family.")),
        ], eyebrow=("Grupos", "Groups"), cols=3, estilo="relleno", tono="relleno", anchor="grupos"),
        bloque_pasos(("Cómo organizamos un evento", "How we organise an event"), [
            (("Nos cuentas", "You tell us"), ("Cuántos sois, qué día os viene bien y si hay alergias o personas que no comen algo.", "How many of you there are, which day suits you and whether there are allergies or foods someone avoids.")),
            (("Te enviamos un presupuesto", "We send you a quote"), ("Sin compromiso, con el menú propuesto y lo que incluye.", "With no obligation, with the proposed menu and what it includes.")),
            (("Cocináis", "You cook"), ("Con un profesor para el grupo, delantales para todos y mesa preparada para comer.", "With a teacher for the group, aprons for everyone and a table set for eating.")),
        ], tono="base"),
        bloque_cta(("Pide un presupuesto sin compromiso", "Ask for a no-obligation quote"),
                   ("Dinos cuántos sois y qué celebráis. Te respondemos con un menú y un precio por persona.",
                    "Tell us how many of you there are and what you are celebrating. We will reply with a menu and a price per person."),
                   [boton(("Pedir presupuesto", "Request a quote"), a_pagina("contacto", "contacto")), whatsapp("ghost")], tono="relleno"),
    ])

    cont = pagina("contacto", ("contacto", "contact"), ("Contacto y reservas", "Contact and bookings"),
                  ("Reserva plaza en un taller, un bono regalo o un evento con el formulario, por teléfono o WhatsApp. Horario y cómo llegar a la escuela de cocina.",
                   "Book a place on a workshop, a gift voucher or an event with the form, by phone or WhatsApp. Opening hours and how to find the cooking school."), [
        hero(("Reserva tu plaza", "Book your place"),
             ("Dinos qué taller te interesa, cuántos sois y si hay alguna alergia. Te contestamos con las fechas libres.",
              "Tell us which workshop interests you, how many of you there are and whether anyone has an allergy. We will reply with the free dates."),
             [llamar(), whatsapp()], variante="texto"),
        bloque_contacto(("Cómo encontrarnos", "How to find us"),
                        ("La escuela está en una planta baja. Si vienes en coche, aparca en la calle y llámanos si no encuentras el portal.",
                         "The school is on a ground floor. If you come by car, park on the street and call us if you cannot find the door."),
                        campos_cita(servicios=[("Taller de una tarde", "One-evening workshop"), ("Curso de varias semanas", "Multi-week course"),
                                               ("Bono regalo", "Gift voucher"), ("Evento de empresa", "Company event"),
                                               ("Cumpleaños o celebración", "Birthday or celebration")],
                                    extra=[campo("personas", "text", ("Cuántas personas", "How many people"), False, maxlen=20),
                                           campo("alergias", "text", ("Alergias o intolerancias (opcional)", "Allergies or intolerances (optional)"), False, maxlen=200)]),
                        ("Pedir reserva", "Request booking"), horas,
                        notas=("Horario de ejemplo: cámbialo por el tuyo.", "Example hours: replace them with your own."),
                        exito=("Gracias. Te escribimos con las fechas libres y cómo confirmar la plaza.",
                               "Thank you. We will write to you with the free dates and how to confirm your place.")),
        bloque_mapa(tono="relleno"),
    ])

    return plantilla(KEY, ("Escuela de cocina y talleres", "Cooking school and workshops"),
                     ("Escuela de cocina con talleres y cursos de ejemplo, calendario, bonos regalo, eventos de empresa, aviso de alérgenos y reserva de plaza.",
                      "Cooking school with example workshops and courses, calendar, gift vouchers, company events, allergen notice and place booking."),
                     "educacion", ("escuela de cocina taller de cocina talleres gastronomicos curso de cocina clases de cocina cocina pan reposteria regalo experiencia team building evento de empresa bono regalo",
                                   "cooking school cooking class cooking workshop culinary course baking class gift voucher team building company event"),
                     look, cabecera(nav, cta_cab),
                     pie(nav, ("Escuela de cocina. Talleres de una tarde, cursos, bonos regalo y eventos de empresa.",
                               "Cooking school. One-evening workshops, courses, gift vouchers and company events."),
                         ("Talleres de martes a sábado, por la tarde y el sábado por la mañana.",
                          "Workshops Tuesday to Saturday, in the evening and on Saturday morning.")),
                     [inicio, cursos, regalos, cont])
