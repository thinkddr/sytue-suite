# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
from lib import *
from looks import LOOKS

KEY = "escuela-arte"

# Marrón tierra oscuro y rosa pálido: ninguna otra plantilla los usa (comprobado con grep de primary=).
TIERRA = dict(background="#3a2418", surface="#4a3022", text="#f6efe6", muted="#d1bfae", primary="#f7b2c8",
              on_primary="#3a2418", border="#654632", inverse="#241408", on_inverse="#f6efe6")


def S(x):
    return (x, x)


def build():
    nueva(KEY)
    look = dict(LOOKS["noche"], colores=TIERRA, fuentes=("noto-serif", "inter"), peso=600, radio=(2, 4, 8))
    nav = [("inicio", ("Inicio", "Home")), ("talleres", ("Talleres y cuotas", "Workshops and fees")),
           ("galeria", ("Exposición", "Exhibition")), ("contacto", ("Contacto", "Contact"))]
    horas = [(("Lunes a jueves", "Monday to Thursday"), ("De 10:00 a 13:00 y de 16:00 a 20:30", "10:00 am to 1:00 pm and 4:00 pm to 8:30 pm")),
             (("Viernes", "Friday"), ("De 10:00 a 13:00 y de 16:00 a 19:00", "10:00 am to 1:00 pm and 4:00 pm to 7:00 pm")),
             (("Sábado", "Saturday"), ("Talleres puntuales de 10:30 a 13:30", "One-off workshops 10:30 am to 1:30 pm")),
             (("Domingo", "Sunday"), ("Cerrado", "Closed"))]
    cta_cab = marco([llamar(t=("Llamar", "Call")), whatsapp("secondary", t=("WhatsApp", "WhatsApp"))],
                    dir="row", gap=8, wrap=True)
    prueba = lambda v="primary": boton(("Reservar clase de prueba", "Book a trial class"),
                                       a_pagina("contacto", "contacto"), v)

    inicio = pagina("inicio", ("", ""), ("Escuela de arte", "Art school"),
                    ("Escuela de arte con dibujo, pintura, cerámica y manualidades por edades y niveles, talleres puntuales, exposición de trabajos y clase de prueba.",
                     "Art school with drawing, painting, ceramics and crafts by age and level, one-off workshops, an exhibition of students' work and a trial class."), [
        hero(("Un taller para crear, sin prisa y sin exámenes", "A studio to create in, with no rush and no exams"),
             ("Dibujo, pintura, cerámica y manualidades para niños, jóvenes y adultos. Vienes con ganas y nosotros ponemos el espacio, la guía y los materiales básicos.",
              "Drawing, painting, ceramics and crafts for children, teenagers and adults. You bring the enthusiasm and we provide the space, the guidance and the basic materials."),
             [prueba(), llamar("secondary"), whatsapp("ghost")],
             eyebrow=("Escuela de arte y talleres creativos", "Art school and creative workshops"),
             imagen_=("portrait", ("Mesa de taller con pinceles en un bote, pastillas de acuarela y un cuaderno de dibujo abierto",
                                   "Studio table with brushes in a jar, watercolour pans and an open sketchbook"))),
        bloque_valores(("Qué se hace en el taller", "What happens in the studio"), [
            (("Dibujo", "Drawing"),
             ("Línea, sombra y proporción, del lápiz al carboncillo. Dibujo del natural y dibujo de imaginación.",
              "Line, shade and proportion, from pencil to charcoal. Drawing from life and from imagination.")),
            (("Pintura", "Painting"),
             ("Acuarela, acrílico y óleo. Aprendes a mezclar el color y a mirar antes de pintar.",
              "Watercolour, acrylic and oil. You learn to mix colour and to look before you paint.")),
            (("Cerámica", "Ceramics"),
             ("Modelado a mano, torno y esmaltes. Las piezas se cuecen en el horno de la escuela.",
              "Hand-building, wheel and glazes. Pieces are fired in the school's kiln.")),
            (("Manualidades", "Crafts"),
             ("Papel, tela, collage y reciclaje creativo. Para quien quiere hacer cosas con las manos y sin presión.",
              "Paper, fabric, collage and creative recycling. For anyone who wants to make things by hand, with no pressure.")),
        ], eyebrow=("Disciplinas", "Disciplines"), cols=4, estilo="borde", tono="base", anchor="disciplinas"),
        bloque_texto(("Por edades y por niveles", "By age and by level"), [
            ("Los grupos se forman por edad y por nivel, para que nadie vaya demasiado deprisa ni demasiado despacio. Los más pequeños exploran materiales; los mayores trabajan técnica y proyectos propios.",
             "Groups are formed by age and level, so nobody goes too fast or too slow. The youngest explore materials; the older ones work on technique and projects of their own."),
            ("En los grupos de adultos hay quien nunca ha dibujado y quien lleva años pintando. Lo importante es que cada uno trabaje en lo suyo, con el profesor cerca.",
             "In adult groups there are people who have never drawn and people who have painted for years. What matters is that each person works on their own thing, with the teacher nearby."),
        ], imagen_=("landscape", ("Aula luminosa con caballetes, mesas de trabajo y estanterías con piezas de cerámica",
                                  "Bright classroom with easels, worktables and shelves of ceramic pieces")),
            tono="relleno", anchor="edades-niveles"),
        bloque_pasos(("Tu primera clase", "Your first class"), [
            (("Vienes a probar", "You come to try"), ("Una clase de prueba en el grupo que más te encaje. No hace falta traer nada.", "A trial class in the group that suits you best. You do not need to bring anything.")),
            (("Hablamos de lo que quieres", "We talk about what you want"), ("Si buscas aprender una técnica, hacer un regalo o pasar un rato. Te proponemos un grupo y un día fijo.", "Whether you want to learn a technique, make a gift or just spend some time. We suggest a group and a fixed day.")),
            (("Empiezas a crear", "You start creating"), ("Cada día tu pieza avanza. Lo que haces es tuyo y te lo llevas a casa.", "Your piece moves forward each session. What you make is yours to take home.")),
        ], tono="base"),
        bloque_galeria(("Trabajos de nuestros alumnos", "Our students' work"), [
            ("Acuarela de un paisaje con árboles, colgada en la pared del taller", "Watercolour landscape with trees, hung on the studio wall"),
            ("Cuencos de cerámica esmaltados en una estantería de madera", "Glazed ceramic bowls on a wooden shelf"),
            ("Dibujo a carboncillo de un bodegón con frutas", "Charcoal drawing of a still life with fruit"),
        ], intro=("Imágenes de ejemplo: cámbialas por fotos de trabajos reales, con permiso de su autor.",
                  "Example images: replace them with photos of real work, with the author's permission."),
            eyebrow=("Galería", "Gallery"), tono="relleno", anchor="galeria-inicio"),
        bloque_opiniones(("Lo que dicen alumnos y familias", "What students and families say"),
                         [("las clases", "the classes"), ("el taller de cerámica", "the ceramics studio"), ("la exposición", "the exhibition")],
                         tono="base"),
        bloque_cta(("Ven a probar una clase", "Come and try a class"),
                   ("La primera clase es de prueba: miras, pruebas y decides.", "The first class is a trial: you look, try and decide."),
                   [prueba(), llamar("ghost")], tono="relleno"),
    ])

    talleres = pagina("talleres", ("talleres", "workshops"), ("Talleres y cuotas", "Workshops and fees"),
                      ("Grupos por edades y niveles, cuotas mensuales y talleres puntuales de ejemplo, qué materiales van incluidos y preguntas frecuentes.",
                       "Groups by age and level, example monthly fees and one-off workshops, which materials are included and frequently asked questions."), [
        hero(("Talleres y cuotas", "Workshops and fees"),
             ("Elige disciplina, grupo y día. Pagas una cuota fija al mes y los materiales básicos ya están incluidos.",
              "Choose a discipline, group and day. You pay one fixed fee a month and the basic materials are included."),
             [prueba()], variante="texto"),
        bloque_valores(("Grupos por edades y niveles", "Groups by age and level"), [
            (("Pequeños, de 4 a 7 años", "Little ones, ages 4 to 7"),
             ("Exploración de materiales: pintura de dedos, barro, collage. El proceso importa más que el resultado.",
              "Exploring materials: finger paint, clay, collage. The process matters more than the result.")),
            (("De 8 a 12 años", "Ages 8 to 12"),
             ("Primeras técnicas de dibujo, color y modelado. Proyectos cortos que se terminan en pocas semanas.",
              "First drawing, colour and modelling techniques. Short projects finished in a few weeks.")),
            (("Jóvenes, de 13 a 17 años", "Teenagers, ages 13 to 17"),
             ("Dibujo y pintura con más técnica, cuaderno de bocetos y portfolio para quien lo necesite.",
              "Drawing and painting with more technique, a sketchbook and a portfolio for those who need one.")),
            (("Adultos", "Adults"),
             ("Principiantes y avanzados en el mismo taller, cada uno con su proyecto. Sin edad ni nivel mínimos.",
              "Beginners and advanced students in the same studio, each with their own project. No minimum age or level.")),
        ], cols=2, estilo="relleno", tono="relleno", anchor="grupos"),
        bloque_precios(("Cuotas mensuales", "Monthly fees"), ("Cuotas mensuales de ejemplo", "Example monthly fees"),
                       [("Grupo", "Group"), ("Sesión", "Session"), ("Cuota al mes", "Fee per month")],
                       [(("Pequeños (4 a 7 años)", "Little ones (ages 4 to 7)"), ("1 hora a la semana", "1 hour a week"), eur(40)),
                        (("Niños (8 a 12 años)", "Children (ages 8 to 12)"), ("1,5 horas a la semana", "1.5 hours a week"), eur(52)),
                        (("Jóvenes (13 a 17 años)", "Teenagers (ages 13 to 17)"), ("2 horas a la semana", "2 hours a week"), eur(62)),
                        (("Adultos, dibujo y pintura", "Adults, drawing and painting"), ("2 horas a la semana", "2 hours a week"), eur(68)),
                        (("Adultos, cerámica", "Adults, ceramics"), ("2,5 horas a la semana", "2.5 hours a week"), eur(85)),
                        (("Matrícula (una vez al año)", "Registration (once a year)"), S("—"), eur(25))],
                       nota=("Precios de ejemplo: cámbialos por los tuyos. Indica aquí si hay descuento por hermanos o por apuntarse a dos talleres.",
                             "Example prices: replace them with your own. Say here whether there is a discount for siblings or for joining two workshops."),
                       anchor="cuotas", tono="base"),
        bloque_precios(("Materiales", "Materials"), ("Qué materiales van incluidos", "Which materials are included"),
                       [("Taller", "Workshop"), ("Incluido en la cuota", "Included in the fee"), ("Lo compras tú", "You buy yourself")],
                       [(("Dibujo", "Drawing"), ("Papel, lápices, carboncillo y gomas", "Paper, pencils, charcoal and erasers"), ("Cuadernos y lápices propios, si quieres", "Your own sketchbooks and pencils, if you want"))
                        , (("Pintura", "Painting"), ("Pinturas básicas, pinceles y papel", "Basic paints, brushes and paper"), ("Lienzos y soportes especiales", "Canvases and special supports")),
                        (("Cerámica", "Ceramics"), ("Arcilla, esmaltes y cocción", "Clay, glazes and firing"), ("Herramientas propias, si las quieres", "Your own tools, if you want them")),
                        (("Manualidades", "Crafts"), ("Material de uso común", "Shared everyday materials"), ("Telas o piezas especiales para tu proyecto", "Fabrics or special items for your project"))],
                       intro=("Para empezar no necesitas comprar nada. Si quieres material propio, te decimos qué comprar y dónde.",
                              "To start you do not need to buy anything. If you want your own materials, we tell you what to buy and where."),
                       nota=("Datos de ejemplo: cámbialos por lo que de verdad incluyes.", "Example information: replace it with what you really include."),
                       anchor="materiales", kind="text", tono="relleno"),
        bloque_precios(("Talleres puntuales", "One-off workshops"), ("Talleres puntuales de ejemplo", "Example one-off workshops"),
                       [("Taller", "Workshop"), ("Duración", "Length"), ("Precio", "Price")],
                       [(("Acuarela para principiantes", "Watercolour for beginners"), ("3 horas", "3 hours"), eur(35)),
                        (("Primera pieza en torno de cerámica", "First piece on the ceramics wheel"), ("3 horas", "3 hours"), eur(45)),
                        (("Cuaderno de viaje", "Travel sketchbook"), ("3 horas", "3 hours"), eur(32)),
                        (("Manualidades con niños, para las vacaciones", "Children's crafts for the holidays"), ("2 horas", "2 hours"), eur(22))],
                       intro=("Para probar una técnica sin apuntarte a un curso entero, o para regalar. Se publican con unas semanas de antelación.",
                              "To try a technique without enrolling on a whole course, or as a gift. They are announced a few weeks ahead."),
                       nota=("Precios de ejemplo: cámbialos por los tuyos.", "Example prices: replace them with your own."),
                       anchor="puntuales", kind="text", tono="base"),
        bloque_faq(("Preguntas frecuentes", "Frequently asked questions"), [
            (("¿Hace falta saber dibujar?", "Do I need to know how to draw?"),
             ("No. Se empieza por mirar y por practicar trazos sencillos. Dibujar se aprende.",
              "No. You begin by looking and practising simple strokes. Drawing can be learnt.")),
            (("¿Me llevo lo que hago?", "Do I take home what I make?"),
             ("Sí. La cerámica se lleva cuando está cocida, que puede tardar unas semanas.",
              "Yes. Ceramics go home once fired, which can take a few weeks.")),
            (("¿Puedo cambiar de día?", "Can I change day?"),
             ("Si hay plaza en otro grupo, sí. Avísanos con tiempo.", "If there is a place in another group, yes. Let us know in advance.")),
            (("¿Mi hijo necesita ropa especial?", "Does my child need special clothes?"),
             ("Ropa que se pueda manchar. Ponemos bata a los pequeños.", "Clothes that can get stained. We give the little ones a smock.")),
        ], tono="relleno"),
        bloque_cta(("¿Qué grupo te va mejor?", "Which group suits you best?"), None, [prueba(), whatsapp("ghost")], tono="base"),
    ])

    galeria = pagina("galeria", ("exposicion", "exhibition"), ("Exposición de trabajos", "Exhibition of work"),
                     ("Exposiciones de los trabajos del alumnado durante el curso y galería de imágenes de relleno para sustituir por fotos reales.",
                      "Exhibitions of students' work during the year and a gallery of placeholder images to replace with real photos."), [
        hero(("Exposición de trabajos", "Exhibition of work"),
             ("Colgamos lo que se hace en el taller. Puedes pasar a verlo, traer a tu familia o apuntarte para el próximo curso.",
              "We hang what is made in the studio. You can drop by to see it, bring your family or sign up for next term."),
             [prueba(), llamar("secondary")], variante="texto"),
        bloque_texto(("Cómo exponemos", "How we exhibit"), [
            ("Al final de cada trimestre montamos una pequeña exposición en la sala de la escuela con los trabajos de todos los grupos. Texto de ejemplo: cuenta aquí tus fechas.",
             "At the end of each term we put on a small exhibition in the school's main room with work from every group. Example text: say your dates here."),
            ("Cada trabajo lleva el nombre de quien lo hizo, solo si quiere. Si prefieres que tu obra no se exponga, no pasa nada: lo respetamos.",
             "Each piece carries its maker's name, only if they want it. If you prefer your work not to be shown, that is fine: we respect it."),
            ("Hay inauguración con familias y amigos, y quien quiera puede vender sus piezas. Es una tarde informal, abierta a todo el barrio.",
             "There is an opening for families and friends, and anyone who wishes can sell their pieces. It is an informal evening, open to the whole neighbourhood."),
        ], tono="base", anchor="como-exponemos"),
        bloque_galeria(("Galería", "Gallery"), [
            ("Acuarela de un paisaje con árboles, colgada en la pared del taller", "Watercolour landscape with trees, hung on the studio wall"),
            ("Cuencos de cerámica esmaltados en una estantería de madera", "Glazed ceramic bowls on a wooden shelf"),
            ("Dibujo a carboncillo de un bodegón con frutas", "Charcoal drawing of a still life with fruit"),
            ("Collage de papeles de colores sobre una mesa de manualidades", "Collage of coloured papers on a crafts table"),
            ("Lienzo al óleo con tonos cálidos sobre un caballete", "Oil canvas in warm tones on an easel"),
            ("Jarrón de barro sin esmaltar secándose en una tabla", "Unglazed clay vase drying on a board"),
        ], intro=("Imágenes de ejemplo: cámbialas por fotos de trabajos reales, con permiso de su autor y sin que salgan menores identificables.",
                  "Example images: replace them with photos of real work, with the author's permission and with no identifiable minors."),
            cols=3, tono="relleno", ratio=1.2, anchor="galeria"),
        bloque_checks(("Quieres exponer o vender", "If you want to show or sell"), [
            ("Pregunta por el próximo montaje de la exposición.", "Ask about the next exhibition set-up."),
            ("Te ayudamos a enmarcar y a poner etiqueta a tu obra.", "We help you frame and label your work."),
            ("Una pieza vendida es de quien la hizo: nosotros no cobramos comisión.", "A piece sold belongs to its maker: we take no commission."),
        ], intro=("Texto de ejemplo: ajusta estas condiciones a las tuyas.", "Example text: adjust these terms to your own."),
            cols=1, tono="base"),
        bloque_cta(("Ven a verlo y pasa por el taller", "Come and see it, and drop by the studio"), None,
                   [prueba(), whatsapp("ghost")], tono="relleno"),
    ])

    cont = pagina("contacto", ("contacto", "contact"), ("Contacto y clase de prueba", "Contact and trial class"),
                  ("Reserva una clase de prueba o pide información de talleres con el formulario, por teléfono o WhatsApp. Horario y cómo llegar a la escuela de arte.",
                   "Book a trial class or ask about workshops with the form, by phone or WhatsApp. Opening hours and how to find the art school."), [
        hero(("Reserva tu clase de prueba", "Book your trial class"),
             ("Dinos qué te gustaría hacer y para quién es. Te llamamos para fijar el día.",
              "Tell us what you would like to do and who it is for. We will call you to set the day."),
             [llamar(), whatsapp()], variante="texto"),
        bloque_contacto(("Cómo encontrarnos", "How to find us"),
                        ("En la primera clase te enseñamos el taller y te presentamos al profesor. Ven con ropa que se pueda manchar.",
                         "In the first class we show you the studio and introduce you to the teacher. Come in clothes that can get stained."),
                        campos_cita(servicios=[("Dibujo", "Drawing"), ("Pintura", "Painting"), ("Cerámica", "Ceramics"),
                                               ("Manualidades", "Crafts"), ("Taller puntual", "One-off workshop")],
                                    extra=[campo("edad", "select", ("Para quién es", "Who it is for"), False,
                                                 opciones=[("Para mí, soy adulto", "For me, I am an adult"),
                                                           ("Para un niño de 4 a 7 años", "For a child aged 4 to 7"),
                                                           ("Para un niño de 8 a 12 años", "For a child aged 8 to 12"),
                                                           ("Para un joven de 13 a 17 años", "For a teenager aged 13 to 17")])]),
                        ("Pedir clase de prueba", "Request trial class"), horas,
                        notas=("Horario de ejemplo: cámbialo por el tuyo.", "Example hours: replace them with your own."),
                        exito=("Gracias. Te confirmamos el día de la clase de prueba por teléfono o WhatsApp.",
                               "Thank you. We will confirm the day of your trial class by phone or WhatsApp.")),
        bloque_mapa(tono="relleno"),
    ])

    return plantilla(KEY, ("Escuela de arte y talleres creativos", "Art school and creative workshops"),
                     ("Escuela de arte con dibujo, pintura, cerámica y manualidades por edades, cuotas y talleres puntuales de ejemplo, exposición de trabajos y clase de prueba.",
                      "Art school with drawing, painting, ceramics and crafts by age, example fees and one-off workshops, an exhibition of work and a trial class."),
                     "educacion", ("escuela de arte taller de arte talleres creativos clases de dibujo clases de pintura ceramica manualidades academia de dibujo acuarela taller de ceramica exposicion alumnos clase de prueba",
                                   "art school art studio creative workshops drawing classes painting lessons ceramics crafts watercolour pottery class kids art"),
                     look, cabecera(nav, cta_cab),
                     pie(nav, ("Escuela de arte. Dibujo, pintura, cerámica y manualidades, para todas las edades.",
                               "Art school. Drawing, painting, ceramics and crafts, for all ages."),
                         ("Lunes a viernes de 10:00 a 13:00 y de 16:00 a 20:30. Sábado talleres puntuales.",
                          "Monday to Friday 10:00 am to 1:00 pm and 4:00 pm to 8:30 pm. Saturday one-off workshops.")),
                     [inicio, talleres, galeria, cont])
