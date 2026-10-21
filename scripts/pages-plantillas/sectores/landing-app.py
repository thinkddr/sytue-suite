# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
from lib import *

KEY = "landing-app"

# Paleta propia (índigo sobre blanco). Sin fondo $inverse: primary/inverse no llega a 3.
LOOK = dict(
    colores=dict(background="#ffffff", surface="#eff0fe", text="#14142b", muted="#52526e", primary="#4338ca",
                 on_primary="#ffffff", border="#dcdef5", inverse="#1b1a4a", on_inverse="#eff0fe"),
    fuentes=("figtree", "ibm-plex-sans"), radio=(8, 14, 20), ancho=1120)


def cabecera_ancla(enlaces, cta):
    """Cabecera de una sola página: nombre, anclas a las secciones y el botón principal."""
    logo = marco([texto("span", NEGOCIO, preset="h4")], enlace=a_ancla("inicio"),
                 sz={"width": "hug", "height": "hug"})
    der = marco([boton(t, a_ancla(a), "ghost") for a, t in enlaces] + [cta], dir="row", gap=12, align="center",
                justify="end", wrap=True, sz={"width": "fill", "height": "hug", "min_width": 0},
                bp={"mobile": {"layout": {"justify": "start"}}})
    n = seccion("header", [logo, der], pad_v=16, gap=24, dir="row", align="center", justify="space-between")
    n["breakpoints"]["mobile"]["layout"] = {"padding": [12, 20, 12, 20], "gap": 10, "direction": "column",
                                            "align": "start"}
    n["style"] = {"border": {"width": 1, "color": "$border", "style": "solid"}}
    return n


def pie_online():
    """Pie de un negocio online: nombre, correo y enlaces legales (sin dirección ni teléfono)."""
    izq = marco([texto("p", NEGOCIO, preset="h4"),
                 peq(("Aplicación para repartir gastos entre quienes comparten piso.",
                      "App for splitting costs between people who share a flat."), color="$muted"),
                 marco([icono("mail"), p((CORREO, CORREO))], dir="row", gap=12, align="center")], gap=8)
    legal = marco([boton(("Aviso legal", "Legal notice"), a_legal("notice"), "ghost"),
                   boton(("Privacidad", "Privacy"), a_legal("privacy"), "ghost"),
                   boton(("Cookies", "Cookies"), a_legal("cookies"), "ghost")], dir="row", gap=8, wrap=True)
    abajo = marco([peq((f"© {NEGOCIO[0]}. Todos los derechos reservados.", f"© {NEGOCIO[1]}. All rights reserved."),
                       color="$muted"), legal],
                  dir="row", gap=16, wrap=True, align="center", justify="space-between")
    return seccion("footer", [izq, abajo], tono="relleno", pad_v=48, gap=28)


def build():
    nueva(KEY)
    avisame = boton(("Avísame cuando esté lista", "Tell me when it is ready"), a_ancla("contacto"), "primary")
    nota_precios = ("Planes y precios de ejemplo: cámbialos por los tuyos.",
                    "Example plans and prices: replace them with your own.")

    cab = cabecera_ancla([("como-funciona", ("Cómo funciona", "How it works")),
                          ("precios", ("Precios", "Pricing")),
                          ("preguntas", ("Preguntas", "Questions"))],
                         boton(("Avísame", "Notify me"), a_ancla("contacto"), "primary"))

    secciones = [
        hero(("Reparte los gastos del piso sin hacer cuentas", "Split the flat's costs without doing the maths"),
             ("Apuntas lo que pagas, la app calcula quién debe qué a quién y al final del mes cada uno sabe cuánto tiene que pagar.",
              "You note down what you pay, the app works out who owes whom, and at the end of the month everyone knows what to pay."),
             [avisame, boton(("Ver cómo funciona", "See how it works"), a_ancla("como-funciona"), "secondary")],
             eyebrow=NEGOCIO,
             imagen_=("portrait", ("Captura de la pantalla principal de la app con los gastos del mes",
                                   "Screenshot of the app's main screen showing the month's costs"))),
        bloque_valores(("Tres cosas que hace por ti", "Three things it does for you"), [
            (("Apuntar un gasto en diez segundos", "Log a cost in ten seconds"),
             ("Importe, quién ha pagado y entre quiénes se reparte. Sin hojas de cálculo ni mensajes de grupo.",
              "Amount, who paid and who shares it. No spreadsheets or group chats.")),
            (("Saber quién debe a quién", "Know who owes whom"),
             ("Te dice el mínimo de pagos necesarios para dejarlo todo a cero, en vez de diez transferencias cruzadas.",
              "It shows the fewest payments needed to settle up, instead of ten crossed transfers.")),
            (("Cerrar el mes sin discutir", "Close the month without arguing"),
             ("Todos ven el mismo historial, con fecha y quién apuntó cada gasto. Si algo no cuadra, se ve dónde.",
              "Everyone sees the same history, with the date and who logged each cost. If something is off, you can see where.")),
        ], intro=("Beneficios de ejemplo: cámbialos por los de tu app.", "Example benefits: replace them with those of your app."),
            eyebrow=("Qué hace", "What it does"), estilo="relleno", tono="base", anchor="beneficios"),
        bloque_pasos(("Cómo funciona", "How it works"), [
            (("Crea el grupo", "Create the group"),
             ("Pones el nombre del piso e invitas a tus compañeros con un enlace.",
              "Name the flat and invite your housemates with a link.")),
            (("Apunta los gastos", "Log the costs"),
             ("Cada uno añade lo que paga: alquiler, luz, internet, la compra.",
              "Everyone adds what they pay: rent, electricity, internet, shopping.")),
            (("Liquida", "Settle up"),
             ("La app te dice cuánto transferir y a quién. Marcas el pago hecho y queda a cero.",
              "The app tells you how much to transfer and to whom. Mark the payment as done and it goes back to zero.")),
        ], intro=("Pasos de ejemplo: cámbialos por los de tu app.", "Example steps: replace them with those of your app."),
            eyebrow=("En tres pasos", "In three steps"), tono="relleno", anchor="como-funciona"),
        bloque_galeria(("Así se ve", "What it looks like"),
                       [("Captura de la lista de gastos del mes", "Screenshot of the month's cost list"),
                        ("Captura de la pantalla de cuentas entre compañeros", "Screenshot of the balances between housemates"),
                        ("Captura de la pantalla para liquidar un pago", "Screenshot of the settle-up screen")],
                       tipo="portrait", cols=3, ratio=0.56, tono="base", anchor="capturas"),
        bloque_precios(("Planes", "Plans"), ("Planes de la app", "App plans"),
                       [("Plan", "Plan"), ("Precio", "Price"), ("Incluye", "Includes")],
                       [(("Gratis", "Free"), eur(0),
                         ("Un grupo de hasta 4 personas y el historial de 3 meses.",
                          "One group of up to 4 people and 3 months of history.")),
                        (("Piso", "Flat"), (f"{eur(2.99)[0]} al mes", f"{eur(2.99)[1]} per month"),
                         ("Grupos sin límite de personas e historial completo.",
                          "Groups of any size and the full history.")),
                        (("Anual", "Yearly"), (f"{eur(24)[0]} al año", f"{eur(24)[1]} per year"),
                         ("Lo mismo que el plan Piso, pagado una vez al año.",
                          "The same as the Flat plan, paid once a year."))],
                       nota=nota_precios, tono="relleno", anchor="precios"),
        bloque_faq(("Preguntas frecuentes", "Frequently asked questions"), [
            (("¿Cuándo estará disponible?", "When will it be available?"),
             ("Déjanos tu correo en el formulario de abajo y te escribimos el día que se abra, sin enviarte nada más.",
              "Leave your email in the form below and we will write to you the day it opens, and nothing else.")),
            (("¿Tengo que pagar para probarla?", "Do I have to pay to try it?"),
             ("No. El plan gratuito sirve para un piso pequeño y no pide tarjeta.",
              "No. The free plan works for a small flat and does not ask for a card.")),
            (("¿Mis compañeros tienen que instalar algo?", "Do my housemates have to install anything?"),
             ("Necesitan la app para apuntar gastos, pero pueden ver las cuentas desde el enlace que les envías.",
              "They need the app to log costs, but they can see the balances from the link you send them.")),
            (("¿Puedo cancelar cuando quiera?", "Can I cancel whenever I want?"),
             ("Sí. Cancelas desde la propia app y sigues usando el plan hasta que termina el periodo pagado.",
              "Yes. You cancel from the app itself and keep the plan until the paid period ends.")),
        ], tono="base", anchor="preguntas", intro=("Respuestas de ejemplo: cámbialas por las tuyas.",
                                                   "Example answers: replace them with your own.")),
        bloque_texto(("Tus datos", "Your data"), [
            ("Guardamos solo lo que hace falta para que la app funcione: tu correo y los gastos que apuntas. No vendemos tus datos. Puedes pedir que te borremos cuando quieras.",
             "We keep only what the app needs to work: your email and the costs you log. We do not sell your data. You can ask us to delete it whenever you want."),
            ("Texto de ejemplo: ajústalo a lo que hace de verdad tu app.",
             "Example text: adjust it to what your app really does."),
        ], tono="relleno", anchor="privacidad"),
        seccion("contact", [marco([
            titular(("Te avisamos cuando esté lista", "We will tell you when it is ready"),
                    ("Déjanos tu correo y te escribimos una sola vez, el día que se abra.",
                     "Leave your email and we will write to you once, the day it opens."), align="center"),
            tarjeta([formulario([campo("nombre", "text", ("Nombre", "Name"), True, auto="name", maxlen=120),
                                 campo("correo", "email", ("Correo electrónico", "Email"), True, auto="email",
                                       maxlen=200),
                                 campo("dispositivo", "select", ("¿Dónde la usarás?", "Where will you use it?"), False,
                                       opciones=[("En el móvil Android", "On an Android phone"),
                                                 ("En el iPhone", "On an iPhone"),
                                                 ("En el ordenador", "On a computer"),
                                                 ("Aún no lo sé", "I do not know yet")])],
                                envio=("Avísame", "Notify me"),
                                exito=("Gracias. Te escribiremos en cuanto esté lista.",
                                       "Thank you. We will write to you as soon as it is ready."))],
                    estilo="borde", pad=28, tag="div", sz={"width": "fill", "height": "hug", "min_width": 0}),
        ], gap=28, align="center", sz={"width": "fill", "height": "hug", "max_width": 560})],
            anchor="contacto", tono="base", align="center", gap=0),
    ]
    inicio = pagina("inicio", ("", ""), ("Inicio", "Home"),
                    ("Página de aterrizaje de una app: qué hace, tres beneficios, cómo funciona, capturas, planes, preguntas y aviso de lanzamiento.",
                     "Landing page for an app: what it does, three benefits, how it works, screenshots, plans, questions and a launch notice."),
                    secciones)
    # El ancla "inicio" del logo apunta a la primera sección.
    inicio["sections"][0]["anchor"] = "inicio"

    return plantilla(KEY, ("Aterrizaje de una app", "App landing page"),
                     ("Una sola página para una app o producto digital: qué hace, beneficios, pasos, capturas, planes, preguntas y formulario de aviso.",
                      "A single page for an app or digital product: what it does, benefits, steps, screenshots, plans, questions and a notify-me form."),
                     "web", ("landing aterrizaje app aplicacion producto digital saas software lanzamiento pagina de producto movil",
                             "landing page app application product digital saas software launch product page mobile"),
                     LOOK, cab, pie_online(), [inicio],
                     exento={"mapa": "Producto digital sin local: no hay dirección que mostrar en un mapa.",
                             "horario": "Producto digital sin atención presencial: no tiene horario de apertura.",
                             "telefono": "Negocio online: el contacto es el formulario de aviso y el correo; teléfono y WhatsApp son opcionales."})
