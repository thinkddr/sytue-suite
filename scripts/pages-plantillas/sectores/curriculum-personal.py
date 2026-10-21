# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
import lib
from lib import *

KEY = "curriculum-personal"

# Paleta propia: papel verdoso y ciruela profunda, como una hoja de currículum impresa.
# Sin fondo $inverse (no se usa), así que no hace falta primary/inverse >= 3.
LOOK = dict(
    colores=dict(background="#f2f6e8", surface="#e6edd3", text="#1f1630", muted="#54476a", primary="#6a1b9a",
                 on_primary="#ffffff", border="#d3dcb9", inverse="#2a1238", on_inverse="#f6ecfb"),
    fuentes=("lora", "inter"), peso=600, h1=(54, 42, 34), radio=(4, 8, 14), ancho=1040)

NOMBRE = ("Nombre Apellido", "First Last")
EJEMPLO = ("Datos de ejemplo: cámbialos por los tuyos.", "Example details: replace them with your own.")


def S(x):
    return (x, x)


def hito(fechas, cargo, lugar, texto_):
    """Una fila de la línea de tiempo: fechas a la izquierda, cargo, lugar y una frase a la derecha."""
    cuerpo = marco([h3(cargo), peq(lugar, color="$muted"), p(texto_)], gap=6,
                   sz={"width": "fill", "height": "hug", "min_width": 0})
    fecha = marco([texto("p", fechas, preset="label", color="$primary")], sz={"width": 170, "height": "hug"},
                  bp={"mobile": {"size": {"width": "fill", "height": "hug"}}})
    return tarjeta([marco([fecha, cuerpo], dir="row", gap=24,
                          bp={"mobile": {"layout": {"direction": "column", "gap": 8}}})],
                   estilo="linea", pad=[20, 0, 20, 0])


def linea_de_tiempo(hitos):
    return marco(hitos, gap=0, tag="ul", sz={"width": "fill", "height": "hug", "max_width": 820})


def grupo(titulo, items):
    return tarjeta([h3(titulo), marco([marco([icono("check", sz=20), p(t)], tag="li", dir="row", gap=10)
                                       for t in items], gap=8, tag="ul")], estilo="borde", gap=14)


def proyecto(nombre, texto_, alt):
    return tarjeta([imagen("photo", alt, ratio=1.4), h3(nombre), p(texto_, color="$muted"),
                    peq(("Año · Mi papel en el proyecto", "Year · My role in the project"), color="$muted")],
                   estilo="borde", gap=12)


def build():
    nueva(KEY)
    malos = lib.comprobar_paleta("curriculum", LOOK["colores"])
    assert not malos, malos

    anclas = [("sobre-mi", ("Sobre mí", "About me")), ("experiencia", ("Experiencia", "Experience")),
              ("formacion", ("Formación", "Education")), ("habilidades", ("Habilidades", "Skills")),
              ("proyectos", ("Proyectos", "Projects")), ("contacto", ("Contacto", "Contact"))]

    cab = cabecera([("inicio", ("Inicio", "Home"))],
                   boton(("Escríbeme", "Write to me"), a_pagina("inicio", "contacto"), "primary", "mail"),
                   marca=NOMBRE)
    # Una sola página: el menú salta a las secciones.
    cab["children"][1]["children"][0]["items"] = [
        {"id": lib._id(), "label": L(t), "link": a_pagina("inicio", a)} for a, t in anclas]

    escribeme = lambda var="primary": boton(("Escríbeme", "Write to me"), a_pagina("inicio", "contacto"), var, "mail")

    inicio = pagina("inicio", ("", ""), ("Currículum y web personal", "Résumé and personal website"),
                    ("Web personal de un profesional: presentación, qué hago, experiencia y formación en línea de tiempo, habilidades, proyectos y contacto por correo y formulario.",
                     "Personal website for a professional: introduction, what I do, experience and education timeline, skills, projects and contact by email and form."), [
        hero(("Hola, soy Nombre Apellido", "Hi, I am First Last"),
             ("Soy [tu profesión]. Escribe aquí en dos frases qué haces, para quién trabajas y qué te gusta de tu oficio.",
              "I am a [your profession]. Write here, in two sentences, what you do, who you work for and what you like about your craft."),
             [escribeme(), boton(("Ver mis proyectos", "See my projects"), a_pagina("inicio", "proyectos"), "secondary")],
             eyebrow=("Profesión o especialidad", "Profession or speciality"), variante="split",
             imagen_=("portrait", ("Retrato de la persona que presenta su currículum", "Portrait of the person presenting their résumé"))),
        bloque_texto(("Sobre mí", "About me"),
                     [("Escribe aquí tu historia en primera persona: de dónde vienes, cómo llegaste a tu profesión y qué tipo de trabajo te gusta hacer.",
                       "Write your story here in the first person: where you come from, how you got into your profession and the kind of work you like to do."),
                      ("Escribe aquí un párrafo corto sobre cómo trabajas: cómo te organizas, cómo hablas con quien te contrata y qué esperas de un buen proyecto.",
                       "Write here a short paragraph about how you work: how you organise yourself, how you talk to whoever hires you and what you expect from a good project.")],
                     eyebrow=("Quién soy", "Who I am"), tono="relleno", anchor="sobre-mi"),
        bloque_valores(("Qué hago", "What I do"), [
            (("Primer servicio o tarea", "First service or task"),
             ("Escribe aquí en una frase qué incluye y para quién sirve.",
              "Write here, in one sentence, what it includes and who it is for.")),
            (("Segundo servicio o tarea", "Second service or task"),
             ("Escribe aquí en una frase qué incluye y para quién sirve.",
              "Write here, in one sentence, what it includes and who it is for.")),
            (("Tercer servicio o tarea", "Third service or task"),
             ("Escribe aquí en una frase qué incluye y para quién sirve.",
              "Write here, in one sentence, what it includes and who it is for.")),
        ], intro=("Lo que puedes pedirme, dicho sin rodeos.", "What you can ask me for, plainly put."),
            eyebrow=("Trabajo", "Work"), cols=3, estilo="borde"),
        seccion("features", [
            titular(("Experiencia", "Experience"), EJEMPLO, eyebrow=("Dónde he trabajado", "Where I have worked")),
            linea_de_tiempo([
                hito(("20XX – hoy", "20XX – present"), ("Cargo actual", "Current role"),
                     ("Empresa o cliente · Ciudad", "Company or client · City"),
                     ("Escribe aquí en una o dos frases qué haces y qué has conseguido.",
                      "Write here, in one or two sentences, what you do and what you have achieved.")),
                hito(("20XX – 20XX", "20XX – 20XX"), ("Cargo anterior", "Previous role"),
                     ("Empresa o cliente · Ciudad", "Company or client · City"),
                     ("Escribe aquí en una o dos frases qué hacías y qué aprendiste.",
                      "Write here, in one or two sentences, what you did and what you learned.")),
                hito(("20XX – 20XX", "20XX – 20XX"), ("Primer puesto", "First job"),
                     ("Empresa o cliente · Ciudad", "Company or client · City"),
                     ("Escribe aquí en una frase cómo empezó todo.",
                      "Write here, in one sentence, how it all started.")),
            ])], tono="base", anchor="experiencia", gap=28),
        seccion("features", [
            titular(("Formación", "Education"), EJEMPLO, eyebrow=("Lo que he estudiado", "What I have studied")),
            linea_de_tiempo([
                hito(("20XX – 20XX", "20XX – 20XX"), ("Título o grado", "Degree"),
                     ("Centro de estudios · Ciudad", "School or university · City"),
                     ("Escribe aquí la especialidad y, si quieres, el trabajo final o lo que más te sirvió.",
                      "Write here the speciality and, if you like, your final project or what helped you most.")),
                hito(("20XX", "20XX"), ("Curso o certificado", "Course or certificate"),
                     ("Centro o plataforma · Ciudad", "Provider or platform · City"),
                     ("Escribe aquí qué aprendiste y cuántas horas duró.",
                      "Write here what you learned and how many hours it took.")),
            ])], tono="relleno", anchor="formacion", gap=28),
        seccion("features", [
            titular(("Habilidades", "Skills"),
                    ("Habilidades de ejemplo: sustitúyelas por las tuyas y deja solo las que de verdad domines.",
                     "Example skills: replace them with your own and keep only the ones you really master."),
                    eyebrow=("Lo que se me da bien", "What I am good at")),
            rejilla([
                grupo(("Lo que hago a diario", "What I do every day"),
                      [("Habilidad principal 1", "Main skill 1"), ("Habilidad principal 2", "Main skill 2"),
                       ("Habilidad principal 3", "Main skill 3")]),
                grupo(("Herramientas", "Tools"),
                      [("Herramienta o programa 1", "Tool or program 1"), ("Herramienta o programa 2", "Tool or program 2"),
                       ("Herramienta o programa 3", "Tool or program 3")]),
                grupo(("Idiomas", "Languages"),
                      [("Idioma y nivel", "Language and level"), ("Otro idioma y nivel", "Another language and level")]),
            ], cols=3, gap=20)], tono="base", anchor="habilidades", gap=28),
        seccion("gallery", [
            titular(("Proyectos", "Projects"),
                    ("Proyectos de ejemplo: sustitúyelos por trabajos tuyos que puedas enseñar.",
                     "Example projects: replace them with your own work that you can show."),
                    eyebrow=("Lo que he hecho", "What I have done")),
            rejilla([
                proyecto(("Nombre del proyecto uno", "Project one name"),
                         ("Escribe aquí qué problema resolvía y qué hiciste tú.",
                          "Write here which problem it solved and what you did."),
                         ("Imagen del primer proyecto", "Image of the first project")),
                proyecto(("Nombre del proyecto dos", "Project two name"),
                         ("Escribe aquí qué problema resolvía y qué hiciste tú.",
                          "Write here which problem it solved and what you did."),
                         ("Imagen del segundo proyecto", "Image of the second project")),
                proyecto(("Nombre del proyecto tres", "Project three name"),
                         ("Escribe aquí qué problema resolvía y qué hiciste tú.",
                          "Write here which problem it solved and what you did."),
                         ("Imagen del tercer proyecto", "Image of the third project")),
            ], cols=3, gap=20)], tono="relleno", anchor="proyectos", gap=28),
        seccion("contact", [marco([
            marco([
                titular(("Hablemos", "Let us talk"),
                        ("Cuéntame qué necesitas y te respondo por correo en cuanto pueda.",
                         "Tell me what you need and I will reply by email as soon as I can."),
                        eyebrow=("Contacto", "Contact")),
                marco([icono("mail"), p((CORREO, CORREO))], dir="row", gap=12, align="start"),
                boton(("Escribir un correo", "Send an email"), a_correo(), "secondary", "mail"),
                marco([icono("linkedin"), p(("LinkedIn: escribe aquí tu usuario", "LinkedIn: write your username here"))],
                      dir="row", gap=12, align="start"),
                marco([icono("globe"), p(("Otra red o web: escribe aquí su dirección", "Another network or site: write its address here"))],
                      dir="row", gap=12, align="start"),
            ], gap=16, sz={"width": "fill", "height": "hug", "min_width": 0}),
            tarjeta([formulario(campos_consulta(("¿Qué necesitas?", "What do you need?")),
                                ("Enviar mensaje", "Send message"),
                                ("Gracias. He recibido tu mensaje y te respondo por correo.",
                                 "Thank you. I have received your message and will reply by email."))],
                    estilo="borde", pad=28, tag="div", sz={"width": "fill", "height": "hug", "min_width": 0}),
        ], dir="row", gap=48, align="start",
            bp={"tablet": {"layout": {"gap": 32}}, "mobile": {"layout": {"direction": "column", "gap": 28}}})],
            anchor="contacto", gap=0),
    ])

    # Pie de web personal: nombre, correo y avisos legales (sin calle ni teléfono).
    legal = marco([boton(("Aviso legal", "Legal notice"), a_legal("notice"), "ghost"),
                   boton(("Privacidad", "Privacy"), a_legal("privacy"), "ghost"),
                   boton(("Cookies", "Cookies"), a_legal("cookies"), "ghost")], dir="row", gap=8, wrap=True)
    pie_ = seccion("footer", [
        marco([marco([texto("p", NOMBRE, preset="h4"), peq((CORREO, CORREO), color="$muted")], gap=6), legal],
              dir="row", gap=16, wrap=True, align="center", justify="space-between"),
        peq((f"© {NOMBRE[0]}. Todos los derechos reservados.", f"© {NOMBRE[1]}. All rights reserved."), color="$muted"),
    ], tono="relleno", pad_v=40, gap=16)

    return plantilla(KEY, ("Currículum personal", "Personal résumé"),
                     ("Web personal de un profesional en una sola página: presentación, qué hago, experiencia y formación en línea de tiempo, habilidades, proyectos y contacto por correo y formulario.",
                      "Single-page personal website for a professional: introduction, what I do, experience and education timeline, skills, projects and contact by email and form."),
                     "web", ("curriculum personal cv web personal curriculum vitae perfil profesional autonomo freelance presentacion experiencia formacion habilidades proyectos contacto una pagina",
                             "resume cv personal website personal site professional profile freelancer introduction experience education skills projects contact single page"),
                     LOOK, cab, pie_, [inicio],
                     exento={"telefono": "Web personal: se contacta por correo y formulario; el teléfono y WhatsApp son opcionales y se añaden desde el editor.",
                             "mapa": "Profesional sin local abierto al público: no hay dirección que mostrar en un mapa.",
                             "horario": "Sin horario de apertura: no atiende en un local; responde por correo."})
