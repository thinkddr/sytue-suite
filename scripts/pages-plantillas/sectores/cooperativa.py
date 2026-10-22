# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
from lib import *
from looks import LOOKS

KEY = "cooperativa"

# Paleta propia: verde azulado de huerta con superficie de hoja seca; pasa AA (lib.comprobar_paleta).
# Sin fondo $inverse: primary/inverse < 3.
TEAL = dict(background="#f8faf5", surface="#e8efe2", text="#14261f", muted="#4a5e53", primary="#1b6a6f",
            on_primary="#ffffff", border="#d2ddc9", inverse="#15393c", on_inverse="#eef6f3")


def build():
    nueva(KEY)
    look = dict(colores=TEAL, fuentes=("source-serif-4", "work-sans"), radio=(4, 8, 12))
    comprobar = comprobar_paleta("cooperativa", TEAL)
    assert not comprobar, comprobar
    nav = [("inicio", ("Inicio", "Home")), ("productos", ("Productos y servicios", "Products and services")),
           ("socios", ("Hazte socio", "Become a member")), ("contacto", ("Contacto", "Contact"))]
    horas = [(("Lunes a viernes", "Monday to Friday"), ("8:00 a 14:00 y 16:00 a 19:00", "8:00 am to 2:00 pm and 4:00 pm to 7:00 pm")),
             (("Sábado", "Saturday"), ("9:00 a 13:00", "9:00 am to 1:00 pm")),
             (("Domingo", "Sunday"), ("Cerrado", "Closed"))]
    nota = ("Precios de ejemplo: cámbialos por los tuyos.", "Example prices: replace them with your own.")
    registro = ("Inscrita en el Registro de Cooperativas con el nº 00-0000.",
                "Registered in the Register of Cooperatives under no. 00-0000.")

    def alta(var="primary"):
        return boton(("Hazte socio o socia", "Become a member"), a_pagina("socios"), var)

    inicio = pagina("inicio", ("", ""), ("Cooperativa", "Cooperative"),
                    ("Cooperativa de socios: qué es, cómo funciona, qué ofrece y cómo hacerse socio. Llama, escribe por WhatsApp o pide el alta.",
                     "Members' cooperative: what it is, how it works, what it offers and how to join. Call, message us on WhatsApp or ask to join."), [
        hero(("Lo que trabajamos juntos, lo decidimos juntos", "What we work on together, we decide together"),
             ("Somos una cooperativa: las personas que la formamos somos sus dueñas, votamos en la asamblea y repartimos lo que se gana entre quienes lo han ganado.",
              "We are a cooperative: the people who belong to it own it, vote at the assembly and share what is earned among those who earned it."),
             [alta(), llamar("secondary"), whatsapp("ghost")],
             eyebrow=("Cooperativa", "Cooperative"),
             imagen_=("landscape", ("Socios de la cooperativa trabajando juntos en el campo", "Cooperative members working together in the field")),
             tono="relleno"),
        bloque_pasos(("Cómo funciona en llano", "How it works, in plain words"), [
            (("Somos socios", "We are members"),
             ("Cada socio o socia aporta una pequeña cantidad de capital al entrar y trabaja con la cooperativa: lleva su producción o usa sus servicios.",
              "Each member puts in a small amount of capital on joining and works with the cooperative: they bring their produce or use its services.")),
            (("Decidimos en asamblea", "We decide at the assembly"),
             ("Una persona, un voto, aporte lo que aporte. Allí se aprueban las cuentas y las decisiones importantes.",
              "One person, one vote, whatever they put in. The accounts and the important decisions are approved there.")),
            (("Repartimos según lo aportado", "We share by what was contributed"),
             ("Los excedentes vuelven a los socios en proporción a lo que cada uno ha trabajado con la cooperativa, no a su capital.",
              "Surpluses go back to members in proportion to what each has worked with the cooperative, not to their capital.")),
        ], intro=("Texto de ejemplo: ajústalo a lo que dicen vuestros estatutos.",
                  "Example text: adjust it to what your statutes say."),
            eyebrow=("Qué es una cooperativa", "What a cooperative is"), tono="base", anchor="como-funciona"),
        bloque_valores(("Qué hacemos por los socios", "What we do for our members"), [
            (("Vender mejor", "Selling better"),
             ("Reunimos la producción de todos para vender en volumen y a un precio más justo que si cada uno fuera por su cuenta.",
              "We pool everyone's output to sell in volume at a fairer price than each member would get alone.")),
            (("Comprar más barato", "Buying for less"),
             ("Compramos juntos semillas, abonos, piensos y materiales, y el socio los recoge en la cooperativa.",
              "We buy seed, fertiliser, feed and supplies together, and members collect them from the cooperative.")),
            (("Compartir medios", "Sharing equipment"),
             ("Maquinaria y almacén de uso común, para no cargar cada explotación con lo que se usa pocas veces al año.",
              "Shared machinery and storage, so no single farm carries what is used only a few times a year.")),
        ], tono="relleno", estilo="borde", eyebrow=("Para quién trabajamos", "Who we work for")),
        bloque_checks(("Nuestros compromisos", "Our commitments"), [
            ("Pagar al socio en el plazo que acordamos y decirle cómo se ha calculado.", "Pay members within the agreed time and explain how the amount was calculated."),
            ("Enseñar las cuentas en la asamblea y contestar a cualquier pregunta.", "Show the accounts at the assembly and answer any question."),
            ("Admitir socios nuevos con las mismas condiciones que los demás.", "Admit new members on the same terms as everyone else."),
            ("Cuidar la tierra y el entorno del que vivimos.", "Look after the land and the surroundings we live from."),
        ], intro=("Compromisos de ejemplo: cámbialos por los vuestros y deja solo lo que de verdad cumplís.",
                  "Example commitments: replace them with your own and keep only what you really do."),
            tono="base", cols=2, anchor="compromisos"),
        bloque_opiniones(("Lo que dicen los socios", "What our members say"),
                         [("la cooperativa", "the cooperative"), ("el reparto", "the profit sharing"), ("la atención", "the service")]),
        bloque_cta(("¿Quieres formar parte?", "Would you like to join?"),
                   ("Te explicamos los pasos y los requisitos sin compromiso.", "We explain the steps and requirements with no commitment."),
                   [alta(), llamar("ghost")], tono="base"),
    ])

    prod = pagina("productos", ("productos", "products"), ("Productos y servicios", "Products and services"),
                  ("Productos de la cooperativa y servicios para socios: suministros, maquinaria, asesoramiento y venta.",
                   "Cooperative products and member services: supplies, machinery, advice and sales."), [
        hero(("Productos y servicios", "Products and services"),
             ("Lo que producen los socios y lo que la cooperativa pone a su disposición. Particulares y empresas pueden comprar los productos.",
              "What the members produce and what the cooperative makes available to them. Individuals and businesses can buy the products."),
             [boton(("Pedir información", "Ask for information"), a_pagina("contacto", "contacto")), whatsapp()], variante="texto"),
        bloque_valores(("Productos de la cooperativa", "Cooperative products"), [
            (("Fruta y verdura de temporada", "Seasonal fruit and vegetables"),
             ("De las explotaciones de los socios, recogida en su punto y servida sin intermediarios.",
              "From the members' farms, picked ripe and supplied without middlemen.")),
            (("Aceite, vino y conservas", "Oil, wine and preserves"),
             ("Elaborados con la producción de los socios y envasados en la propia cooperativa.",
              "Made from the members' harvests and packed at the cooperative itself.")),
            (("Cereal y forraje", "Grain and fodder"),
             ("Para ganaderos y almacenistas, con pesada y albarán en cada entrega.",
              "For livestock farmers and stockists, with weighing and a delivery note each time.")),
        ], intro=("Productos de ejemplo: cámbialos por los vuestros. Quita lo que no ofrezcáis.",
                  "Example products: replace them with your own. Remove whatever you do not offer."),
            eyebrow=("Para vender", "For sale"), estilo="relleno", tono="base", anchor="productos"),
        bloque_valores(("Servicios para socios", "Member services"), [
            (("Suministros", "Supplies"), ("Semillas, abonos, piensos y material agrícola al precio de la compra conjunta.", "Seed, fertiliser, feed and farm supplies at the joint-purchase price.")),
            (("Maquinaria compartida", "Shared machinery"), ("Reserva por días o por horas, con el operario si lo necesitas.", "Book it by the day or by the hour, with an operator if you need one.")),
            (("Asesoramiento técnico", "Technical advice"), ("Una persona técnica te visita, revisa tu cultivo y te propone qué hacer y cuándo.", "A technician visits you, checks your crop and suggests what to do and when.")),
            (("Comercialización", "Marketing and sales"), ("Recogemos tu producción, la clasificamos y la vendemos junto con la de los demás socios.", "We collect your produce, grade it and sell it together with the other members'.")),
        ], cols=2, eyebrow=("Para trabajar", "For working"), tono="relleno", estilo="borde", anchor="servicios"),
        bloque_texto(("Precios y pedidos", "Prices and orders"), [
            ("Los precios cambian con la campaña y con el producto, por eso no los publicamos aquí. Llámanos o escríbenos y te los decimos al momento.",
             "Prices change with the season and the product, so we do not list them here. Call or message us and we will tell you straight away."),
            ("A los socios se les aplica la tarifa de socio. Si todavía no lo eres, te explicamos cómo serlo.",
             "Members get the member rate. If you are not one yet, we explain how to become one."),
        ], tono="base"),
        bloque_cta(("¿Necesitas algo de la cooperativa?", "Do you need something from the cooperative?"), None,
                   [llamar(), whatsapp("ghost")], tono="relleno"),
    ])

    socios = pagina("socios", ("hazte-socio", "become-a-member"), ("Hazte socio", "Become a member"),
                    ("Pasos y requisitos de ejemplo para entrar en la cooperativa, aportación inicial, transparencia y solicitud de alta.",
                     "Example steps and requirements to join the cooperative, initial contribution, transparency and application."), [
        hero(("Hazte socio o socia", "Become a member"),
             ("Entrar es sencillo: se pide, se aprueba y se firma. Estos son los pasos y lo que necesitas.",
              "Joining is simple: you apply, it is approved and you sign. Here are the steps and what you need."),
             [boton(("Pedir el alta", "Request membership"), a_pagina("contacto", "contacto")), llamar("secondary")],
             variante="texto"),
        bloque_pasos(("Cómo entrar", "How to join"), [
            (("Pides el alta", "You apply"), ("Rellenas la solicitud de esta web o pasas por la oficina. No cuesta nada.", "You fill in the application on this site or drop by the office. It costs nothing.")),
            (("La junta la estudia", "The board reviews it"), ("El consejo rector revisa tu solicitud y te contesta en un plazo corto.", "The governing board reviews your application and replies within a short time.")),
            (("Aportas y firmas", "You contribute and sign"), ("Pagas la aportación inicial, firmas tu alta y recibes tu ficha de socio.", "You pay the initial contribution, sign up and receive your member record.")),
            (("Empiezas a trabajar", "You start working"), ("Desde ese día votas en la asamblea y usas todos los servicios.", "From that day you vote at the assembly and use all the services.")),
        ], intro=("Pasos de ejemplo: ajústalos a vuestros estatutos y a vuestro funcionamiento real.",
                  "Example steps: adjust them to your statutes and how you really work."),
            tono="base", anchor="pasos"),
        bloque_checks(("Requisitos", "Requirements"), [
            ("Ser mayor de edad y tener una explotación o una actividad relacionada con la cooperativa.", "Be of legal age and have a farm or an activity related to the cooperative."),
            ("Pagar la aportación inicial al capital social.", "Pay the initial contribution to the share capital."),
            ("Comprometerse a trabajar con la cooperativa, vendiendo o comprando a través de ella.", "Commit to working with the cooperative, selling or buying through it."),
            ("Aceptar los estatutos y acudir a la asamblea siempre que sea posible.", "Accept the statutes and attend the assembly whenever possible."),
        ], intro=("Requisitos de ejemplo: cámbialos por los que fijan vuestros estatutos.",
                  "Example requirements: replace them with those set in your statutes."),
            tono="relleno", cols=2, anchor="requisitos"),
        bloque_precios(("Aportación inicial", "Initial contribution"), ("Aportaciones de ejemplo", "Example contributions"),
                       [("Concepto", "Item"), ("Importe", "Amount")],
                       [(("Aportación al capital social (se devuelve al causar baja)", "Share capital contribution (refunded on leaving)"), eur(300)),
                        (("Cuota de ingreso (una sola vez)", "Joining fee (one-off)"), eur(60))],
                       intro=("Lo que se paga al entrar. Pregunta en la oficina por las condiciones de baja.",
                              "What you pay on joining. Ask at the office about the terms for leaving."),
                       nota=nota, anchor="aportacion", tono="base"),
        bloque_texto(("Transparencia", "Transparency"), [
            ("Los socios pueden consultar en cualquier momento las cuentas anuales, el informe de gestión y los estatutos de la cooperativa.",
             "Members can check the annual accounts, the management report and the statutes of the cooperative at any time."),
            ("Documentos disponibles bajo petición: escríbenos o pasa por la oficina y te los entregamos.",
             "Documents available on request: write to us or drop by the office and we will hand them over."),
            registro,
        ], eyebrow=("Cuentas claras", "Clear accounts"), tono="relleno", anchor="transparencia"),
        bloque_faq(("Preguntas frecuentes", "Frequently asked questions"), [
            (("¿Qué pasa con mi aportación si me doy de baja?", "What happens to my contribution if I leave?"),
             ("Se te devuelve según lo previsto en los estatutos y en los plazos que ahí se fijan. Pregunta en la oficina por tu caso.",
              "It is refunded as set out in the statutes and within the deadlines they fix. Ask at the office about your case.")),
            (("¿Tengo que vender todo mi producto a la cooperativa?", "Do I have to sell all my produce to the cooperative?"),
             ("Depende de lo que digan los estatutos. Algunas cooperativas piden la producción entera y otras solo una parte.",
              "It depends on what the statutes say. Some cooperatives ask for the whole output and others only a share.")),
            (("¿Puedo ir a la asamblea si soy nuevo?", "Can I go to the assembly if I am new?"),
             ("Sí, desde el día en que firmas tu alta. Tienes voz y un voto, igual que todos.",
              "Yes, from the day you sign up. You have a say and one vote, just like everyone else.")),
        ], tono="base"),
        bloque_cta(("¿Empezamos con tu solicitud?", "Shall we start your application?"), None,
                   [boton(("Pedir el alta", "Request membership"), a_pagina("contacto", "contacto")), whatsapp("ghost")], tono="relleno"),
    ])

    cont = pagina("contacto", ("contacto", "contact"), ("Contacto y solicitud de alta", "Contact and membership application"),
                  ("Solicitud de alta de socio, consultas y pedidos. Teléfono, WhatsApp, horario de la oficina y cómo llegar.",
                   "Membership application, enquiries and orders. Phone, WhatsApp, office hours and how to find us."), [
        hero(("Escríbenos o pasa por la oficina", "Write to us or drop by the office"),
             ("Para pedir el alta, comprar o consultar cualquier cosa. Te respondemos en el horario de la oficina.",
              "To apply to join, buy or ask anything. We reply during office hours."),
             [llamar(), whatsapp()], variante="texto"),
        bloque_contacto(("Solicitud y consultas", "Application and enquiries"),
                        ("Cuéntanos qué necesitas. Si quieres ser socio, indica tu actividad y te llamamos.",
                         "Tell us what you need. If you want to become a member, say what you do and we will call you."),
                        campos_consulta(mensaje_etiqueta=("Cuéntanos qué necesitas", "Tell us what you need"),
                                        servicios=[("Quiero ser socio o socia", "I want to become a member"),
                                                   ("Quiero comprar productos", "I want to buy products"),
                                                   ("Soy socio y tengo una consulta", "I am a member and have a question"),
                                                   ("Otra consulta", "Something else")],
                                        extra=[campo("actividad", "text", ("Tu explotación o actividad", "Your farm or activity"), False, maxlen=200)]),
                        ("Enviar solicitud", "Send application"), horas,
                        notas=registro,
                        exito=("Gracias. Hemos recibido tu solicitud y te llamamos en cuanto la revisemos.",
                               "Thank you. We have received your application and will call you as soon as we have reviewed it.")),
        bloque_mapa(tono="relleno"),
    ])

    return plantilla(KEY, ("Cooperativa", "Cooperative"),
                     ("Cooperativa agrícola o de servicios con cómo funciona, productos y servicios, hazte socio con pasos y requisitos, transparencia y solicitud de alta.",
                      "Agricultural or service cooperative with how it works, products and services, how to join with steps and requirements, transparency and a membership application."),
                     "local", ("cooperativa cooperativa agricola cooperativa de servicios cooperativa de trabajo socios hazte socio asamblea agricultores ganaderos almazara bodega cooperativa de consumo",
                               "cooperative farming cooperative service cooperative members become a member assembly farmers producers co-op"),
                     look, cabecera(nav, alta()),
                     pie(nav, ("Cooperativa de socios. Productos, servicios y alta de nuevos socios. Registro nº 00-0000.",
                               "Members' cooperative. Products, services and new member applications. Registration no. 00-0000."),
                         ("Lunes a viernes 8:00 a 14:00 y 16:00 a 19:00. Sábado 9:00 a 13:00.",
                          "Monday to Friday 8 am to 2 pm and 4 pm to 7 pm. Saturday 9 am to 1 pm.")),
                     [inicio, prod, socios, cont])
