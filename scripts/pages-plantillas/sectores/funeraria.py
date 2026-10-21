# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
from lib import *

KEY = "funeraria"


def _c(background, surface, text, muted, primary, on_primary, border, inverse, on_inverse):
    return dict(background=background, surface=surface, text=text, muted=muted, primary=primary,
                on_primary=on_primary, border=border, inverse=inverse, on_inverse=on_inverse)


# Paleta propia: verde niebla y azul petróleo apagado, serena. No usa fondo $inverse.
NIEBLA = dict(colores=_c("#eef7f6", "#dfeeec", "#1a2a2c", "#4b5e60", "#386f75", "#ffffff", "#c9dedb", "#1b2f31", "#eaf4f3"),
              fuentes=("lora", "noto-sans"), radio=(4, 8, 12), ancho=1080, h1=(46, 36, 30), peso=600)

REGISTRO = ("Autorización y registro: nº 00-0000", "Licence and registration: no. 00-0000")


def telefono_grande():
    """Teléfono de atención 24 horas, grande y pulsable."""
    return marco([
        etiqueta(("Atención las 24 horas, todos los días", "Open 24 hours, every day")),
        marco([icono("phone", sz=32), texto("p", (TEL_VISIBLE, TEL_VISIBLE), preset="h2")], dir="row", gap=14,
              align="center"),
    ], gap=6, enlace=a_tel(), pad=[16, 20, 16, 20],
        style={"border": {"width": 1, "color": "$border", "style": "solid"}, "radius": "$md",
               "background": "$surface"})


def build():
    nueva(KEY)
    look = NIEBLA
    malos = comprobar_paleta("niebla", look["colores"])
    assert not malos, malos
    nav = [("inicio", ("Inicio", "Home")), ("tramites", ("Trámites y ayuda", "Help and paperwork")),
           ("despedidas", ("Despedidas y previsión", "Farewells and planning")),
           ("contacto", ("Contacto", "Contact"))]
    horas = [(("Teléfono de atención", "Phone line"), ("Las 24 horas, todos los días", "24 hours, every day")),
             (("Oficina, lunes a viernes", "Office, Monday to Friday"),
              ("9:30 a 13:30 y 16:30 a 19:00", "9:30 am to 1:30 pm and 4:30 pm to 7:00 pm")),
             (("Oficina, sábado", "Office, Saturday"), ("10:00 a 13:00", "10:00 am to 1:00 pm")),
             (("Tanatorio", "Funeral home"), ("Abierto cuando hay un servicio", "Open whenever there is a service"))]

    def llamar_ahora(var="primary"):
        return llamar(var, ("Llamar al teléfono de atención", "Call the care line"))

    inicio = pagina("inicio", ("", ""), ("Servicios funerarios", "Funeral services"),
                    ("Servicios funerarios con atención las 24 horas: qué hacer, trámites, despedidas, previsión y tanatorio.",
                     "Funeral services with round-the-clock care: what to do, paperwork, farewells, planning and funeral home."), [
        hero(("Estamos a tu lado cuando más falta hace", "We are by your side when you need it most"),
             ("Si ha fallecido alguien de tu familia, llámanos a cualquier hora. Una persona te atiende, te explica los pasos y se ocupa de los trámites mientras tú acompañas a los tuyos.",
              "If someone in your family has died, call us at any hour. One person will answer, explain the steps and take care of the paperwork while you stay with your loved ones."),
             [llamar_ahora(), whatsapp()], eyebrow=("Servicios funerarios", "Funeral services"),
             imagen_=("landscape", ("Jardín tranquilo junto al tanatorio al atardecer", "A quiet garden next to the funeral home at dusk")),
             extra=telefono_grande()),
        bloque_pasos(("Qué hacer cuando ocurre un fallecimiento", "What to do when someone dies"), [
            (("Llámanos", "Call us"),
             ("A cualquier hora. Te preguntamos dónde ha ocurrido y te decimos qué hacer en ese momento.",
              "At any hour. We ask where it happened and tell you what to do right then.")),
            (("Certificado médico", "Medical certificate"),
             ("Un médico confirma el fallecimiento y firma el certificado. En un hospital lo gestiona el centro; en casa, te indicamos a quién avisar.",
              "A doctor confirms the death and signs the certificate. In a hospital the centre arranges it; at home, we tell you who to call.")),
            (("Nos ocupamos del traslado", "We arrange the transfer"),
             ("Acudimos al lugar, trasladamos a tu familiar con respeto y lo atendemos en el tanatorio.",
              "We go to the place, move your relative with care and look after them at the funeral home.")),
            (("Decidimos la despedida contigo", "We plan the farewell with you"),
             ("Hablamos con calma de la ceremonia, la fecha y las personas a quienes avisar. Sin prisas.",
              "We talk calmly about the ceremony, the date and who should be told. There is no rush.")),
        ], tono="base", anchor="pasos"),
        bloque_valores(("Cómo te acompañamos", "How we support you"), [
            (("Una persona de referencia", "One person to turn to"),
             ("Siempre hablas con la misma persona, que conoce tu caso y contesta a tus preguntas.",
              "You always talk to the same person, who knows your case and answers your questions.")),
            (("Los trámites, resueltos", "Paperwork taken care of"),
             ("Registro, licencias, cementerio, seguros: te lo explicamos y lo gestionamos nosotros cuando nos lo pides.",
              "Registry, licences, cemetery, insurance: we explain it and handle it for you when you ask us to.")),
            (("La despedida que queréis", "The farewell you want"),
             ("Religiosa, civil o íntima. Tú decides cómo y nosotros lo preparamos con cuidado.",
              "Religious, civil or private. You decide how, and we prepare it with care.")),
        ], tono="relleno", estilo="borde", eyebrow=("Nuestro trabajo", "Our work")),
        bloque_texto(("El tanatorio y sus salas", "The funeral home and its rooms"), [
            ("Las salas son privadas, con espacio para la familia, una zona de descanso y un lugar donde tomar algo. Puedes quedarte el tiempo que necesites.",
             "The rooms are private, with space for the family, a place to rest and somewhere to have a drink. You can stay as long as you need."),
            ("Hay acceso para sillas de ruedas y aparcamiento cerca. Cuéntanos qué necesitas y lo dejamos preparado.",
             "There is wheelchair access and parking nearby. Tell us what you need and we will have it ready."),
        ], imagen_=("photo", ("Sala de despedida con luz suave y sillas dispuestas", "Farewell room with soft light and chairs set out")),
            eyebrow=("Tanatorio", "Funeral home"), tono="base", invertir=True, anchor="tanatorio"),
        bloque_cta(("Si necesitas hablar, aquí estamos", "If you need to talk, we are here"),
                   ("Puedes llamarnos a cualquier hora, escribirnos por WhatsApp o pasar por la oficina.",
                    "You can call us at any hour, message us on WhatsApp or drop by the office."),
                   [llamar_ahora(), whatsapp("ghost")], tono="relleno"),
    ])

    tramites = pagina("tramites", ("tramites", "paperwork"), ("Trámites y ayuda", "Help and paperwork"),
                      ("Qué documentos suelen hacer falta, qué trámites gestionamos y respuestas a las dudas habituales tras un fallecimiento.",
                       "Which documents are usually needed, which procedures we handle and answers to common questions after a death."), [
        hero(("Trámites y ayuda", "Help and paperwork"),
             ("Tras un fallecimiento hay gestiones que no esperas. Aquí te contamos cuáles son y cuáles hacemos nosotros por ti.",
              "After a death there are procedures you do not expect. Here we explain what they are and which ones we do for you."),
             [llamar_ahora(), whatsapp()], variante="texto"),
        bloque_valores(("Según dónde ocurra", "Depending on where it happens"), [
            (("En un hospital o residencia", "In a hospital or care home"),
             ("El centro emite el certificado médico y nos avisa o te pide que nos llames. Nosotros nos encargamos del traslado.",
              "The centre issues the medical certificate and calls us, or asks you to call us. We take care of the transfer.")),
            (("En casa", "At home"),
             ("Hay que avisar a un médico para certificar el fallecimiento. Llámanos y te decimos a quién avisar y cómo.",
              "A doctor must be called to certify the death. Call us and we will tell you who to contact and how.")),
            (("Fuera de tu ciudad", "Away from your home town"),
             ("Nos coordinamos con la funeraria del lugar y organizamos el traslado hasta donde queráis la despedida.",
              "We coordinate with the local funeral director and arrange the transfer to wherever you want the farewell.")),
        ], estilo="linea", tono="base", eyebrow=("Primeras horas", "First hours")),
        bloque_checks(("Trámites que gestionamos", "Procedures we handle"), [
            ("Inscripción de la defunción en el Registro Civil.", "Registration of the death with the Civil Registry."),
            ("Licencia de inhumación o de cremación.", "Burial or cremation licence."),
            ("Traslado dentro y fuera de la provincia.", "Transfer within and outside the province."),
            ("Reserva de sala en el tanatorio y de la ceremonia.", "Booking of the funeral home room and the ceremony."),
            ("Concesión o reserva de nicho, sepultura o columbario.", "Niche, grave or columbarium allocation or reservation."),
            ("Esquela en prensa y aviso a familiares y amigos.", "Newspaper notice and a message to relatives and friends."),
            ("Solicitud de certificados de últimas voluntades y de seguros.", "Request for the last-wills and insurance certificates."),
            ("Orientación sobre pensión de viudedad y otras gestiones.", "Guidance on widow's pension and other procedures."),
        ], intro=("Los documentos que hacen falta cambian según cada caso. Te los indicamos uno a uno y los revisamos contigo.",
                  "The documents needed vary from case to case. We list them one by one and go through them with you."),
            tono="relleno", cols=2, anchor="gestiones"),
        bloque_checks(("Qué conviene tener a mano", "What it helps to have to hand"), [
            ("DNI o documento de identidad de la persona fallecida.", "ID document of the person who has died."),
            ("Tarjeta sanitaria y libro de familia, si los tienes.", "Health card and family record book, if you have them."),
            ("Datos de la póliza de seguro de decesos, si existe.", "Details of the funeral insurance policy, if there is one."),
            ("Las voluntades que dejó dichas, por escrito o de palabra.", "Any wishes they expressed, in writing or in person."),
        ], intro=("Si no encuentras algo, no pasa nada: te ayudamos a pedir copias.",
                  "If you cannot find something, that is fine: we will help you request copies."),
            tono="base", cols=2),
        bloque_faq(("Preguntas frecuentes", "Frequently asked questions"), [
            (("¿Cuánto tiempo tengo para decidir?", "How long do I have to decide?"),
             ("Hay un plazo mínimo y un máximo antes de la ceremonia, y te lo explicamos según tu caso. Dentro de él, tienes tiempo para decidir con calma.",
              "There is a minimum and a maximum period before the ceremony, and we explain it for your case. Within it, you have time to decide calmly.")),
            (("¿Puedo velar a mi familiar en casa?", "Can I hold the vigil at home?"),
             ("En muchos casos sí. Hablamos contigo y te decimos qué hace falta.",
              "In many cases, yes. We talk to you and tell you what is needed.")),
            (("¿Quién avisa a los demás?", "Who tells everyone else?"),
             ("Si lo prefieres, nos encargamos de avisar a familiares y amigos con el texto que elijas.",
              "If you prefer, we let relatives and friends know with the wording you choose.")),
            (("¿Cómo se paga?", "How do I pay?"),
             ("Te entregamos el presupuesto por escrito antes de empezar. Si hay seguro de decesos, nos coordinamos con la aseguradora.",
              "We give you the quote in writing before we start. If there is funeral insurance, we coordinate with the insurer.")),
        ], tono="relleno"),
        bloque_cta(("¿Quieres que lo veamos juntos?", "Would you like us to go through it together?"), None, tono="base"),
    ])

    despedidas = pagina("despedidas", ("despedidas", "farewells"), ("Despedidas y previsión", "Farewells and planning"),
                        ("Tipos de despedida, tanatorio, esquelas y planes de previsión para dejarlo todo organizado con tiempo.",
                         "Types of farewell, funeral home, notices and planning schemes to have everything arranged in advance."), [
        hero(("Despedidas y previsión", "Farewells and planning"),
             ("Cada familia despide a los suyos a su manera. Te explicamos las opciones y te ayudamos a elegir la que mejor encaja.",
              "Every family says goodbye in its own way. We explain the options and help you choose the one that fits best."),
             [llamar_ahora("primary"), boton(("Ver planes de previsión", "See planning schemes"), a_ancla("prevision"), "secondary")],
             variante="texto"),
        bloque_valores(("Tipos de despedida", "Types of farewell"), [
            (("Inhumación", "Burial"),
             ("En nicho o sepultura, con una despedida en el tanatorio o junto al cementerio.",
              "In a niche or grave, with a farewell at the funeral home or at the cemetery.")),
            (("Cremación", "Cremation"),
             ("Con ceremonia previa o sin ella. Las cenizas pueden conservarse, depositarse en un columbario o esparcirse donde lo permita la ley.",
              "With a ceremony beforehand or without one. The ashes can be kept, placed in a columbarium or scattered where the law allows.")),
            (("Ceremonia religiosa", "Religious ceremony"),
             ("En el tanatorio, en una iglesia o en el lugar de culto que elijas, de cualquier confesión.",
              "At the funeral home, in a church or in the place of worship you choose, of any faith.")),
            (("Ceremonia civil", "Civil ceremony"),
             ("Con palabras de la familia, música, lecturas y un momento de recuerdo. Te ayudamos a prepararla.",
              "With words from the family, music, readings and a moment of remembrance. We help you prepare it.")),
            (("Despedida íntima", "Private farewell"),
             ("Solo la familia más cercana, en una sala privada y sin horarios.",
              "Only the closest family, in a private room with no set times.")),
            (("Traslados", "Transfers"),
             ("A otra localidad, otra provincia o fuera del país. Nos ocupamos de los permisos y del viaje.",
              "To another town, another province or abroad. We handle the permits and the journey.")),
        ], tono="base", estilo="relleno", anchor="tipos",
            intro=("No hay una forma correcta de despedirse. Te damos opciones y tú decides.",
                   "There is no right way to say goodbye. We give you options and you decide.")),
        bloque_valores(("Esquelas y avisos", "Notices and announcements"), [
            (("Esquela en prensa", "Newspaper notice"),
             ("Redactamos el texto con la familia, con el nombre, la fecha y los datos de la ceremonia, y lo enviamos al periódico que elijas.",
              "We write the text with the family, with the name, date and ceremony details, and send it to the newspaper you choose.")),
            (("Esquela en esta web", "Notice on this website"),
             ("Si la familia lo desea, publicamos un aviso breve para que amigos y vecinos sepan cuándo y dónde es la despedida.",
              "If the family wishes, we publish a short notice so friends and neighbours know when and where the farewell is.")),
            (("Mensajes y recordatorio", "Messages and keepsake"),
             ("Recogemos los mensajes de la familia y preparamos recordatorios impresos para quien asista.",
              "We gather the family's messages and prepare printed keepsakes for those who attend.")),
        ], tono="relleno", estilo="linea", cols=3,
            intro=("Solo publicamos una esquela con el permiso de la familia.", "We only publish a notice with the family's permission.")),
        bloque_texto(("Tanatorio y salas", "Funeral home and rooms"), [
            ("Salas privadas de distintos tamaños, con zona de descanso, aseos y cafetería. Hay una sala más pequeña para despedidas íntimas y otra más amplia para ceremonias con más asistentes.",
             "Private rooms of different sizes, with a rest area, toilets and a café. There is a smaller room for private farewells and a larger one for ceremonies with more guests."),
            ("Indica aquí cuántas salas hay, su aforo y si hay sala de ceremonias.",
             "Say here how many rooms there are, their capacity and whether there is a ceremony room."),
        ], eyebrow=("Instalaciones", "Facilities"), tono="base", anchor="salas"),
        bloque_precios(("Planes de previsión", "Planning schemes"), ("Planes de previsión de ejemplo", "Example planning schemes"),
                       [("Plan", "Plan"), ("Qué incluye", "What it covers"), ("Pago único", "One-off payment"),
                        ("Cuota mensual", "Monthly fee")],
                       [(("Sencillo", "Simple"), ("Traslado, ataúd, sala y trámites", "Transfer, coffin, room and paperwork"),
                         eur(2200), eur(16)),
                        (("Completo", "Complete"), ("Lo anterior, ceremonia y esquela en prensa", "The above, plus ceremony and newspaper notice"),
                         eur(3500), eur(24)),
                        (("Con ceremonia y sala amplia", "With ceremony and large room"),
                         ("Lo anterior, sala amplia y recordatorios", "The above, plus large room and keepsakes"),
                         eur(4800), eur(33))],
                       intro=("Prever es dejar elegido lo que quieres y que tu familia no tenga que decidir con prisa. Es opcional y puedes cambiarlo.",
                              "Planning ahead means choosing what you want so your family does not have to decide in a hurry. It is optional and you can change it."),
                       nota=("Precios de ejemplo: cámbialos por los tuyos. Cada plan es un contrato con condiciones por escrito, que te entregamos antes de firmar.",
                             "Example prices: replace them with your own. Each plan is a contract with written terms, which we give you before you sign."),
                       boton_=boton(("Pedir información", "Ask for information"), a_pagina("contacto", "contacto"), "secondary"),
                       tono="relleno", anchor="prevision"),
        bloque_cta(("¿Quieres que te expliquemos las opciones?", "Would you like us to explain the options?"),
                   ("Sin compromiso y a tu ritmo. Puedes venir a la oficina o hablar con nosotros por teléfono.",
                    "No obligation and at your pace. You can come to the office or talk to us by phone."),
                   [llamar_ahora(), whatsapp("ghost")], tono="base"),
    ])

    cont = pagina("contacto", ("contacto", "contact"), ("Contacto", "Contact"),
                  ("Teléfono de atención las 24 horas, WhatsApp, formulario, horario de la oficina y cómo llegar al tanatorio.",
                   "24-hour phone line, WhatsApp, a form, office hours and how to get to the funeral home."), [
        hero(("Contacto", "Contact"),
             ("Para avisarnos de un fallecimiento, llama al teléfono de atención: contesta una persona, a cualquier hora. Para lo demás, escríbenos y te respondemos en cuanto podamos.",
              "To tell us about a death, call the care line: a person answers at any hour. For anything else, write to us and we will reply as soon as we can."),
             [llamar_ahora(), whatsapp()], variante="texto", extra=telefono_grande()),
        bloque_contacto(("Cómo encontrarnos", "How to find us"),
                        ("La oficina y el tanatorio están en el mismo edificio, con aparcamiento cerca.",
                         "The office and the funeral home are in the same building, with parking nearby."),
                        campos_consulta(mensaje_etiqueta=("Cuéntanos qué necesitas", "Tell us what you need"),
                                        servicios=[("Información sobre trámites", "Information about paperwork"),
                                                   ("Información sobre una despedida", "Information about a farewell"),
                                                   ("Planes de previsión", "Planning schemes"),
                                                   ("Otra consulta", "Something else")]),
                        ("Enviar mensaje", "Send message"), horas,
                        notas=("Este formulario no es para avisar de un fallecimiento: para eso, llama al teléfono de atención. Usamos tus datos solo para responderte.",
                               "This form is not for reporting a death: for that, call the care line. We use your details only to reply to you."),
                        exito=("Gracias. Te respondemos en cuanto podamos, por correo o por teléfono.",
                               "Thank you. We will reply as soon as we can, by email or by phone.")),
        bloque_mapa(texto_=("Indica aquí dónde está la entrada del tanatorio y dónde se puede aparcar.",
                            "Say here where the funeral home entrance is and where to park."), tono="relleno"),
    ])

    return plantilla(KEY, ("Funeraria", "Funeral home"),
                     ("Sitio de una funeraria: qué hacer ante un fallecimiento, trámites, tipos de despedida, previsión, tanatorio, teléfono 24 horas, WhatsApp, horario y formulario.",
                      "Website for a funeral director: what to do after a death, paperwork, types of farewell, planning ahead, funeral home, 24-hour phone, WhatsApp, hours and a form."),
                     "comunidad", ("funeraria servicios funerarios tanatorio pompas funebres incineracion cremacion entierro inhumacion esquela velatorio prevision seguro de decesos defuncion fallecimiento duelo",
                                   "funeral home funeral director undertaker mortuary cremation burial obituary wake pre-planning funeral plan bereavement"),
                     look, cabecera(nav, marco([llamar(), whatsapp("ghost")], dir="row", gap=12, wrap=True)),
                     pie(nav, ("Acompañamos a las familias en los días más difíciles, con calma y con cuidado. " + REGISTRO[0] + ".",
                               "We support families through the hardest days, calmly and with care. " + REGISTRO[1] + "."),
                         ("Teléfono 24 horas. Oficina: lunes a viernes 9:30 a 13:30 y 16:30 a 19:00.",
                          "24-hour phone line. Office: Monday to Friday 9:30 am to 1:30 pm and 4:30 pm to 7 pm.")),
                     [inicio, tramites, despedidas, cont])
