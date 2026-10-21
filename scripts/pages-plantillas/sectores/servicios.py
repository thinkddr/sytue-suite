# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
from lib import *
from looks import LOOKS  # noqa: F401  (la paleta es propia de este sector)

KEY = "servicios"

# Verde petróleo sobre un blanco algo frío: serio sin parecer un banco, y neutro para cualquier oficio
# de servicios. Sin `tono="oscuro"`: primary/inverse no llega a 3, así que el énfasis usa `relleno`.
PETROLEO_FRIO = dict(
    colores=dict(background="#f5f9f9", surface="#e5eeee", text="#14282a", muted="#46605f", primary="#0a6666",
                 on_primary="#ffffff", border="#cfdddd", inverse="#0e2b2d", on_inverse="#eef7f7"),
    fuentes=("ibm-plex-sans", "noto-sans"), peso=600, radio=(3, 6, 10), ancho=1140)


def build():
    nueva(KEY)
    look = PETROLEO_FRIO
    nav = [("inicio", ("Inicio", "Home")), ("servicios", ("Servicios y casos", "Services and cases")),
           ("contacto", ("Equipo y contacto", "Team and contact"))]
    horas = [(("Lunes a jueves", "Monday to Thursday"),
              ("8:30 a 14:00 y 15:00 a 17:30", "8:30 am to 2:00 pm and 3:00 pm to 5:30 pm")),
             (("Viernes", "Friday"), ("8:30 a 14:30", "8:30 am to 2:30 pm")),
             (("Sábado y domingo", "Saturday and Sunday"), ("Cerrado", "Closed"))]

    def presupuesto(var="primary"):
        return boton(("Pedir presupuesto sin compromiso", "Ask for a no-obligation quote"),
                     a_pagina("contacto", "contacto"), var)

    cabecera_ = cabecera(nav, marco([presupuesto("primary"), llamar("secondary"), whatsapp("ghost")],
                                    dir="row", gap=12, wrap=True))

    inicio = pagina("inicio", ("", ""), ("Servicios profesionales para empresas", "Professional services for businesses"),
                    ("Servicios técnicos y administrativos para empresas pequeñas y medianas: qué hacemos, para quién, cómo trabajamos y presupuesto sin compromiso. Llama o escribe por WhatsApp.",
                     "Technical and administrative services for small and medium-sized businesses: what we do, who for, how we work and a no-obligation quote. Call or message us on WhatsApp."), [
        hero(("Las tareas técnicas y administrativas de tu empresa, en manos de quien ya las conoce",
              "Your company's technical and administrative tasks, in the hands of people who already know them"),
             ("Trabajamos con empresas que prefieren no contratar a una persona para cada tarea. Te asignamos una persona de contacto, un presupuesto por escrito y un calendario.",
              "We work with companies that would rather not hire someone for every task. We assign you a contact person, a written quote and a schedule."),
             [presupuesto(), llamar("secondary"), whatsapp("ghost")],
             eyebrow=("Servicios para empresas", "Services for businesses"), variante="centro"),
        bloque_valores(("Qué hacemos", "What we do"), [
            (("Gestión administrativa", "Administrative support"),
             ("Facturas, pedidos, archivo y seguimiento de cobros y pagos, con la frecuencia que acordemos.",
              "Invoices, orders, filing and follow-up of payments in and out, as often as we agree.")),
            (("Soporte técnico", "Technical support"),
             ("Equipos, red y programas de tu oficina: mantenimiento, avisos y ayuda cuando algo falla.",
              "Your office's equipment, network and software: maintenance, alerts and help when something breaks.")),
            (("Mantenimiento periódico", "Regular maintenance"),
             ("Revisiones programadas, con un parte por escrito de lo que se ha mirado y lo que queda.",
              "Scheduled checks, with a written report of what was checked and what remains.")),
            (("Trámites y documentación", "Paperwork and documentation"),
             ("Preparamos y presentamos la documentación que te piden clientes, proveedores y administraciones.",
              "We prepare and submit the documentation that clients, suppliers and public bodies ask you for.")),
            (("Proyectos puntuales", "One-off projects"),
             ("Una mudanza de oficina, un cambio de sistema, una auditoría interna: con principio y final.",
              "An office move, a system change, an internal review: with a beginning and an end.")),
            (("Apoyo en picos de trabajo", "Cover for busy periods"),
             ("Una persona de refuerzo cuando hay más trabajo que manos, sin contratos largos.",
              "Extra hands when there is more work than people, without long contracts.")),
        ], intro=("Son servicios de ejemplo: cámbialos por los que ofrece tu empresa.",
                  "These are example services: replace them with the ones your company offers."),
            eyebrow=("Servicios", "Services"), cols=3, estilo="relleno", tono="base"),
        bloque_checks(("Para quién trabajamos", "Who we work for"), [
            ("Pequeñas empresas que no tienen un departamento para esto.", "Small companies that have no department for this."),
            ("Negocios con varias sedes o con obras y equipos fuera de la oficina.", "Businesses with several sites or with teams working away from the office."),
            ("Despachos y profesionales que quieren dedicarse a su trabajo y no a los papeles.", "Firms and professionals who want to focus on their work and not the paperwork."),
            ("Empresas que han crecido y han perdido el orden de antes.", "Companies that have grown and lost the order they once had."),
        ], eyebrow=("Clientes", "Clients"), tono="relleno", cols=2),
        bloque_pasos(("Cómo trabajamos", "How we work"), [
            (("Reunión inicial", "First meeting"),
             ("Una hora, en tu oficina o por videollamada, para entender qué haces y qué te quita tiempo.",
              "One hour, at your office or by video call, to understand what you do and what takes up your time.")),
            (("Presupuesto por escrito", "Written quote"),
             ("Qué incluye, cuánto cuesta, cuándo empezamos y qué no entra. Sin compromiso.",
              "What is included, what it costs, when we start and what is not covered. No commitment.")),
            (("Puesta en marcha", "Getting started"),
             ("Recogemos accesos, documentos y contactos. Te presentamos a quien llevará tu cuenta.",
              "We gather access, documents and contacts. We introduce you to the person who will handle your account.")),
            (("Seguimiento", "Follow-up"),
             ("Un resumen breve cada mes de lo hecho y de lo pendiente, y una llamada si algo cambia.",
              "A short summary each month of what was done and what is pending, and a call if something changes.")),
        ], tono="base"),
        bloque_opiniones(("Lo que dicen las empresas con las que trabajamos", "What the companies we work with say"),
                         [("la puntualidad", "the punctuality"), ("la claridad de los informes", "the clarity of the reports"),
                          ("el trato con el equipo", "dealing with the team")]),
        bloque_cta(("¿Qué te está quitando tiempo?", "What is taking up your time?"),
                   ("Cuéntanoslo y te decimos si podemos ayudarte, cómo y a qué precio. El presupuesto no te compromete a nada.",
                    "Tell us and we will say whether we can help, how and at what price. The quote commits you to nothing."),
                   [presupuesto(), llamar("ghost")], tono="base"),
    ])

    cab_serv = [("Servicio", "Service"), ("Qué incluye", "What is included"), ("Cómo se contrata", "How it is hired")]
    servicios = pagina("servicios", ("servicios", "services"), ("Servicios y casos", "Services and cases"),
                       ("Qué incluye cada servicio, cómo se contrata, tres casos de ejemplo y respuestas a las dudas de las empresas.",
                        "What each service includes, how it is hired, three example cases and answers to businesses' questions."), [
        hero(("Servicios y casos", "Services and cases"),
             ("Qué incluye cada servicio, cómo se contrata y qué hemos resuelto para otras empresas.",
              "What each service includes, how it is hired and what we have solved for other companies."),
             [presupuesto()], variante="texto"),
        bloque_precios(("Qué incluye cada servicio", "What each service includes"),
                       ("Servicios de ejemplo y modalidad de contratación", "Example services and hiring arrangement"),
                       cab_serv,
                       [(("Gestión administrativa", "Administrative support"),
                         ("Facturación, archivo, seguimiento de cobros y pagos", "Invoicing, filing, follow-up of payments in and out"),
                         ("Cuota mensual", "Monthly fee")),
                        (("Soporte técnico", "Technical support"),
                         ("Mantenimiento de equipos y red, avisos y ayuda a distancia", "Equipment and network maintenance, alerts and remote help"),
                         ("Cuota mensual o bolsa de horas", "Monthly fee or block of hours")),
                        (("Mantenimiento periódico", "Regular maintenance"),
                         ("Revisiones programadas con parte por escrito", "Scheduled checks with a written report"),
                         ("Contrato anual", "Annual contract")),
                        (("Trámites y documentación", "Paperwork and documentation"),
                         ("Preparación y presentación de documentos", "Preparation and submission of documents"),
                         ("Por encargo", "Per job")),
                        (("Proyectos puntuales", "One-off projects"),
                         ("Planificación, ejecución y cierre con informe", "Planning, execution and closing report"),
                         ("Proyecto cerrado", "Fixed-price project"))],
                       intro=("Cambia esta tabla por tus servicios. No ponemos precios: cada empresa es distinta y por eso hacemos presupuesto sin compromiso.",
                              "Replace this table with your services. We do not list prices: every company is different, which is why we give a no-obligation quote."),
                       anchor="servicios", kind="text", tono="base"),
        bloque_valores(("Casos de ejemplo", "Example cases"), [
            (("Una empresa con las facturas en tres sitios", "A company with invoices in three places"),
             ("Tenían facturas en el correo, en papel y en una hoja de cálculo. Las reunimos en un solo archivo y fijamos un día al mes para cerrarlas. Ahora saben qué han cobrado y qué falta.",
              "They had invoices in email, on paper and in a spreadsheet. We gathered them in one file and set one day a month to close them. Now they know what they have been paid and what is missing.")),
            (("Una oficina que se paraba con cada avería", "An office that stopped with every fault"),
             ("Cada fallo del equipo paraba el trabajo medio día. Hicimos un inventario, un calendario de revisiones y un teléfono único para avisar. Los fallos se resuelven antes de que frenen a nadie.",
              "Every equipment fault stopped work for half a day. We made an inventory, a maintenance calendar and a single number to report problems. Faults are fixed before they hold anyone up.")),
            (("Una empresa que cambió de sede", "A company that moved premises"),
             ("Coordinamos el traslado de puestos, la línea y los archivos en un fin de semana. El lunes abrieron en la nueva sede con todo en su sitio.",
              "We coordinated moving desks, the phone line and the files over a weekend. On Monday they opened at the new premises with everything in place.")),
        ], intro=("Estos tres casos son de ejemplo: cámbialos por los tuyos, y sin nombrar al cliente si no tienes su permiso.",
                  "These three cases are examples: replace them with your own, and do not name the client without their permission."),
            cols=3, estilo="borde", tono="relleno", anchor="casos"),
        bloque_valores(("Cómo contratarnos", "How to hire us"), [
            (("Proyecto cerrado", "Fixed-price project"),
             ("Un trabajo con principio y final y un precio acordado antes de empezar.",
              "A job with a start and an end and a price agreed before we begin.")),
            (("Bolsa de horas", "Block of hours"),
             ("Compras un número de horas y las usas cuando las necesites, sin cuota fija.",
              "You buy a number of hours and use them when you need them, with no fixed fee.")),
            (("Cuota mensual", "Monthly fee"),
             ("Un servicio continuo con una cuota fija y un resumen cada mes de lo hecho.",
              "An ongoing service with a fixed fee and a monthly summary of what was done.")),
        ], tono="base", estilo="linea", anchor="contratar"),
        bloque_faq(("Preguntas frecuentes", "Frequently asked questions"), [
            (("¿Trabajáis con empresas de otra ciudad?", "Do you work with companies in other cities?"),
             ("Sí, en lo que se puede hacer a distancia. Para lo presencial, lo vemos en el presupuesto, con los viajes a la vista.",
              "Yes, for whatever can be done remotely. For on-site work, we look at it in the quote, with travel shown separately.")),
            (("¿Hay permanencia?", "Is there a minimum term?"),
             ("En los contratos mensuales, no: avisas con un mes y lo dejamos ordenado. En los anuales, depende de lo que firmemos y lo ves en el presupuesto.",
              "On monthly contracts, no: you give a month's notice and we leave everything in order. On annual ones, it depends on what we sign, and you can see it in the quote.")),
            (("¿Qué pasa con nuestros datos?", "What happens to our data?"),
             ("Firmamos un acuerdo de confidencialidad. Si tratamos datos personales por tu cuenta, firmamos también el contrato de encargado de tratamiento que pide la normativa de protección de datos.",
              "We sign a confidentiality agreement. If we process personal data on your behalf, we also sign the data processing agreement that data protection rules require.")),
            (("¿Cuánto tardáis en empezar?", "How long do you take to start?"),
             ("Cuando aceptas el presupuesto, la fecha de inicio ya está en él.",
              "When you accept the quote, the start date is already in it.")),
            (("¿Cómo facturáis?", "How do you invoice?"),
             ("Cada mes o al terminar, según la modalidad. La factura detalla lo hecho.",
              "Monthly or on completion, depending on the arrangement. The invoice details the work done.")),
        ], tono="relleno"),
        bloque_cta(("¿Hablamos de tu caso?", "Shall we talk about your case?"), None,
                   [presupuesto(), whatsapp("ghost")], tono="base"),
    ])

    campos = campos_consulta(
        mensaje_etiqueta=("Cuéntanos qué necesitas", "Tell us what you need"),
        servicios=[("Gestión administrativa", "Administrative support"), ("Soporte técnico", "Technical support"),
                   ("Mantenimiento periódico", "Regular maintenance"), ("Trámites y documentación", "Paperwork and documentation"),
                   ("Proyecto puntual", "One-off project"), ("Otra cosa", "Something else")],
        extra=[campo("empresa", "text", ("Empresa", "Company"), False, auto="organization", maxlen=160),
               campo("tamano", "select", ("Tamaño de la empresa", "Company size"), False,
                     opciones=[("Hasta 10 personas", "Up to 10 people"), ("De 11 a 50 personas", "11 to 50 people"),
                               ("Más de 50 personas", "More than 50 people")])])
    contacto = pagina("contacto", ("contacto", "contact"), ("Equipo y contacto", "Team and contact"),
                      ("Quién lleva tu cuenta, cómo pedir un presupuesto sin compromiso con el formulario, por teléfono o WhatsApp, y cómo llegar a la oficina.",
                       "Who looks after your account, how to ask for a no-obligation quote with the form, by phone or WhatsApp, and how to find the office."), [
        hero(("Habla con quien va a hacer el trabajo", "Talk to the people who will do the work"),
             ("Cuéntanos qué necesitas y te respondemos en un día laborable con las siguientes preguntas o con un presupuesto.",
              "Tell us what you need and we will reply within one working day with follow-up questions or a quote."),
             [llamar(), whatsapp()], variante="texto"),
        bloque_equipo(("Con quién vas a tratar", "Who you will deal with"), [
            (("Dirección y cuentas", "Management and accounts"),
             ("Te recibe, entiende qué necesitas y se encarga de que el presupuesto sea claro.",
              "Meets you, understands what you need and makes sure the quote is clear.")),
            (("Responsable técnico", "Technical lead"),
             ("Dirige el trabajo y es quien contesta las dudas de tu equipo.",
              "Runs the work and is the one who answers your team's questions.")),
            (("Administración", "Administration"),
             ("Factura, resuelve los papeles y te manda el resumen mensual.",
              "Invoices, handles the paperwork and sends you the monthly summary.")),
        ], intro=("Cambia estas fichas por tu equipo: foto, nombre y función de cada persona.",
                  "Replace these cards with your team: photo, name and role of each person."),
            tono="relleno"),
        bloque_contacto(("Pide tu presupuesto", "Ask for your quote"),
                        ("Rellena el formulario o llámanos. El presupuesto es sin compromiso.",
                         "Fill in the form or call us. The quote is without obligation."),
                        campos, ("Pedir presupuesto", "Request a quote"), horas,
                        exito=("Gracias. Hemos recibido tu mensaje y te respondemos en un día laborable.",
                               "Thank you. We have received your message and will reply within one working day.")),
        bloque_mapa(tono="relleno"),
    ])

    return plantilla(KEY, ("Servicios profesionales", "Professional services"),
                     ("Servicios técnicos y administrativos para empresas en tres páginas: qué hacemos, para quién, cómo trabajamos, casos de ejemplo, equipo, presupuesto sin compromiso con formulario, horario y mapa.",
                      "Technical and administrative services for businesses in three pages: what we do, who for, how we work, example cases, team, a no-obligation quote form, opening hours and a map."),
                     "profesionales", ("servicios profesionales servicios para empresas empresa de servicios b2b gestion administrativa soporte tecnico mantenimiento tramites presupuesto sin compromiso pyme autonomo",
                                       "professional services business services b2b administrative support technical support maintenance paperwork quote small business"),
                     look, cabecera_,
                     pie(nav, ("Servicios técnicos y administrativos para empresas, con presupuesto sin compromiso.",
                               "Technical and administrative services for businesses, with a no-obligation quote."),
                         ("Lunes a jueves 8:30 a 14:00 y 15:00 a 17:30. Viernes 8:30 a 14:30.",
                          "Monday to Thursday 8:30 am to 2 pm and 3 pm to 5:30 pm. Friday 8:30 am to 2:30 pm.")),
                     [inicio, servicios, contacto])
