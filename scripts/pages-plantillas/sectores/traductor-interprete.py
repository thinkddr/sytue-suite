# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
from lib import *

KEY = "traductor-interprete"

# Número de traductor-intérprete jurado: solo lo trae quien lo es; el marcador 00-0000 impide publicar sin cambiarlo.
JURADO = ("Nº de traductor-intérprete jurado: 00-0000", "Sworn translator-interpreter no.: 00-0000")


def _c(background, surface, text, muted, primary, on_primary, border, inverse, on_inverse):
    return dict(background=background, surface=surface, text=text, muted=muted, primary=primary,
                on_primary=on_primary, border=border, inverse=inverse, on_inverse=on_inverse)


# Paleta propia: tinta nocturna con primario pervinca. Fondo oscuro; $inverse (aún más oscuro) pasa primary/inverse >= 3.
TINTA_PERVINCA = dict(colores=_c("#0f1220", "#181c30", "#eceefa", "#a9b0cf", "#a5b4fc", "#0f1220", "#2b3150", "#080a14", "#eceefa"),
                      fuentes=("source-serif-4", "dm-sans"), radio=(2, 6, 10), ancho=1100, peso=600)


def build():
    nueva(KEY)
    look = TINTA_PERVINCA
    malos = comprobar_paleta("tinta-pervinca", look["colores"])
    assert not malos, malos
    nav = [("inicio", ("Inicio", "Home")), ("idiomas", ("Idiomas y documentos", "Languages and documents")),
           ("tarifas", ("Tarifas y presupuesto", "Rates and quotes")), ("contacto", ("Contacto", "Contact"))]
    horas = [(("Lunes a viernes", "Monday to Friday"),
              ("9:00 a 14:00 y 16:00 a 19:00", "9:00 am to 2:00 pm and 4:00 pm to 7:00 pm")),
             (("Sábado y domingo", "Saturday and Sunday"), ("Respondo el lunes", "I reply on Monday"))]
    presupuesto = lambda v="primary": boton(("Pedir presupuesto", "Ask for a quote"),
                                            a_pagina("contacto", "contacto"), v)
    nota = ("Precios de ejemplo: cámbialos por los tuyos. IVA no incluido. Los plazos son orientativos y dependen del idioma y de lo legible que esté el original.",
            "Example prices: replace them with your own. VAT not included. Turnaround times are approximate and depend on the language and on how legible the original is.")
    mas = lambda n, uni: (f"Desde {eur(n)[0]} {uni[0]}", f"From {eur(n)[1]} {uni[1]}")

    inicio = pagina("inicio", ("", ""), ("Traductor e intérprete", "Translator and interpreter"),
                    ("Traductor e intérprete profesional independiente: traducción jurada y no jurada, interpretación, pares de idiomas, tarifas y cómo pedir presupuesto.",
                     "Independent professional translator and interpreter: sworn and non-sworn translation, interpreting, language pairs, rates and how to ask for a quote."), [
        hero(("Traduzco tus documentos y interpreto en tus reuniones", "I translate your documents and interpret at your meetings"),
             ("Trabajo por cuenta propia. Tú hablas conmigo desde el primer correo hasta la entrega, y el texto lo traduzco yo, no una máquina ni una subcontrata.",
              "I work independently. You deal with me from the first email to delivery, and I translate the text myself, not a machine or a subcontractor."),
             [presupuesto(), llamar("secondary"), whatsapp("ghost")],
             eyebrow=("Traducción e interpretación", "Translation and interpreting"),
             imagen_=("photo", ("Escritorio con un diccionario, un portátil y un documento con sello", "Desk with a dictionary, a laptop and a stamped document"))),
        bloque_valores(("Mis idiomas", "My languages"), [
            (("Español e inglés", "Spanish and English"),
             ("Par de ejemplo: cámbialo por tus idiomas y di en qué sentido traduces, a tu lengua materna o desde ella.",
              "Example pair: change it to your languages and say in which direction you translate, into or out of your mother tongue.")),
            (("Español y francés", "Spanish and French"),
             ("Par de ejemplo: cámbialo por tus idiomas o bórralo si no lo ofreces.",
              "Example pair: change it to your languages or delete it if you do not offer it.")),
            (("Español y alemán", "Spanish and German"),
             ("Par de ejemplo: cámbialo por tus idiomas o bórralo si no lo ofreces.",
              "Example pair: change it to your languages or delete it if you do not offer it.")),
        ], intro=("Solo traduzco a los idiomas que domino a nivel profesional. Si me pides otro, te recomiendo a alguien de confianza.",
                  "I only translate into languages I command at a professional level. If you ask for another, I recommend someone I trust."),
            eyebrow=("Pares de idiomas", "Language pairs"), cols=3, estilo="linea", tono="base", anchor="mis-idiomas"),
        bloque_valores(("Qué hago", "What I do"), [
            (("Traducción jurada", "Sworn translation"),
             ("Documentos oficiales con mi firma y sello, para presentar ante organismos, juzgados y universidades.",
              "Official documents with my signature and stamp, to submit to authorities, courts and universities.")),
            (("Traducción no jurada", "Non-sworn translation"),
             ("Contratos, manuales, webs, cartas y textos de empresa. Los reviso dos veces antes de entregarlos.",
              "Contracts, manuals, websites, letters and company texts. I proofread them twice before delivery.")),
            (("Interpretación", "Interpreting"),
             ("Reuniones, visitas, gestiones y citas, en persona o por videollamada. Consecutiva y de enlace.",
              "Meetings, visits, errands and appointments, in person or by video call. Consecutive and liaison interpreting.")),
            (("Revisión de textos", "Proofreading"),
             ("Si ya tienes una traducción, la reviso contra el original y te marco lo que corregiría.",
              "If you already have a translation, I check it against the original and mark what I would correct.")),
        ], cols=2, eyebrow=("Servicios", "Services"), estilo="relleno", tono="relleno", anchor="que-hago"),
        bloque_texto(("Traducción jurada, en pocas palabras", "Sworn translation in a nutshell"), [
            ("Una traducción jurada lleva la firma y el sello de un traductor-intérprete jurado y certifica que el texto es fiel al original. Es la que suelen pedir los organismos oficiales.",
             "A sworn translation bears the signature and stamp of a sworn translator-interpreter and certifies that the text is faithful to the original. It is what official bodies usually ask for."),
            ("Antes de encargarla, pregunta al organismo en el que la vas a presentar si la quiere en papel o en formato electrónico. Yo te digo cómo la entrego.",
             "Before ordering one, ask the body where you will submit it whether it wants paper or electronic format. I will tell you how I deliver it."),
            (JURADO[0], JURADO[1]),
        ], eyebrow=("Jurada", "Sworn"), tono="base", anchor="jurada"),
        bloque_cta(("Envíame el documento y te digo precio y plazo", "Send me the document and I will tell you the price and turnaround"),
                   ("Basta con enviarlo por correo. No hace falta que lo traduzcas ni que lo expliques: dime el idioma y para qué lo necesitas.",
                    "Just send it by email. You do not need to translate or explain it: tell me the language and what you need it for."),
                   [presupuesto(), whatsapp("ghost")], tono="relleno"),
    ])

    idiomas = pagina("idiomas", ("idiomas-y-documentos", "languages-and-documents"), ("Idiomas y documentos", "Languages and documents"),
                     ("Qué idiomas traduzco, qué tipos de documento acepto, cuándo hace falta traducción jurada y cómo interpreto en reuniones.",
                      "Which languages I translate, which types of document I accept, when a sworn translation is needed and how I interpret at meetings."), [
        hero(("Idiomas y tipos de documento", "Languages and types of document"),
             ("Dime el idioma, el documento y dónde lo vas a presentar. Con eso te digo si hace falta jurada y cuánto tardo.",
              "Tell me the language, the document and where you will submit it. With that I can tell you whether it needs to be sworn and how long I take."),
             [presupuesto(), llamar("secondary")], variante="texto"),
        bloque_checks(("Documentos personales y oficiales", "Personal and official documents"), [
            ("Certificados de nacimiento, matrimonio y defunción.", "Birth, marriage and death certificates."),
            ("Certificados de antecedentes penales y de empadronamiento.", "Criminal record and residence certificates."),
            ("Títulos, expedientes y certificaciones académicas.", "Degrees, transcripts and academic certificates."),
            ("Sentencias, poderes y documentos notariales.", "Judgments, powers of attorney and notarial documents."),
            ("Documentos con apostilla: la apostilla también se traduce.", "Documents with an apostille: the apostille is translated too."),
            ("Informes médicos y documentos para trámites de extranjería.", "Medical reports and documents for immigration procedures."),
        ], intro=("Casi todos estos documentos se piden en traducción jurada. Confírmalo con el organismo que los va a recibir.",
                  "Almost all of these documents are requested as sworn translations. Confirm it with the body that will receive them."),
            eyebrow=("Jurada", "Sworn"), tono="base", cols=2, anchor="personales"),
        bloque_checks(("Documentos de empresa y de trabajo", "Business and work documents"), [
            ("Contratos, estatutos y actas.", "Contracts, articles of association and minutes."),
            ("Facturas, cuentas y documentos de importación o exportación.", "Invoices, accounts and import or export documents."),
            ("Manuales, fichas técnicas y catálogos.", "Manuals, data sheets and catalogues."),
            ("Páginas web, folletos y presentaciones.", "Websites, brochures and presentations."),
            ("Currículums, cartas de presentación y correspondencia.", "CVs, cover letters and correspondence."),
            ("Artículos y textos de divulgación.", "Articles and general-interest texts."),
        ], intro=("Estos suelen ir sin traducción jurada, salvo que alguien te pida lo contrario.",
                  "These usually go without sworn translation, unless someone asks you otherwise."),
            eyebrow=("Empresa", "Business"), tono="relleno", cols=2, anchor="empresa"),
        bloque_valores(("Interpretación", "Interpreting"), [
            (("Consecutiva", "Consecutive"),
             ("Hablas tú, hablo yo. Para reuniones, entrevistas y visitas con pocas personas.",
              "You speak, then I speak. For meetings, interviews and visits with few people.")),
            (("De enlace o acompañamiento", "Liaison or accompanying"),
             ("Te acompaño a una gestión, una visita médica o una reunión de trabajo y facilito la conversación.",
              "I go with you to an errand, a medical visit or a business meeting and help the conversation along.")),
            (("Por videollamada", "By video call"),
             ("Interpreto a distancia cuando no puedo ir. Necesito saber la plataforma y la hora.",
              "I interpret remotely when I cannot be there. I need to know the platform and the time.")),
        ], intro=("Dime el lugar, la duración y de qué se va a hablar, y te preparo un presupuesto.",
                  "Tell me the place, the length and what will be discussed, and I will prepare a quote."),
            eyebrow=("Reuniones", "Meetings"), estilo="borde", tono="base", anchor="interpretacion"),
        bloque_faq(("Preguntas frecuentes", "Frequently asked questions"), [
            (("¿Cómo sé si necesito traducción jurada?", "How do I know whether I need a sworn translation?"),
             ("Pregúntalo al organismo o a la empresa que recibirá el documento. Si no lo tienes claro, mándamelo y te lo explico.",
              "Ask the body or company that will receive the document. If you are not sure, send it to me and I will explain.")),
            (("¿Necesitas el original?", "Do you need the original?"),
             ("Para traducir me basta una copia legible, en PDF o en foto. Si el organismo lo exige, te lo cuento antes.",
              "A legible copy, as a PDF or photo, is enough for me to translate. If the body requires more, I tell you beforehand.")),
            (("¿Puedes traducir un documento que no se lee bien?", "Can you translate a document that is hard to read?"),
             ("Lo intento, y marco entre corchetes lo que no se lee. Si es ilegible, te aviso antes de empezar.",
              "I try, and I mark what is unreadable in square brackets. If it cannot be read, I let you know before I start.")),
        ], tono="relleno"),
        bloque_cta(("¿Qué documento tienes?", "What document do you have?"), None,
                   [presupuesto(), whatsapp("ghost")], tono="base"),
    ])

    tarifas = pagina("tarifas", ("tarifas", "rates"), ("Tarifas y presupuesto", "Rates and quotes"),
                     ("Tarifas orientativas por palabra, por página y por hora, plazos, cómo pedir presupuesto enviando el documento por correo y confidencialidad.",
                      "Indicative rates per word, per page and per hour, turnaround times, how to ask for a quote by emailing the document, and confidentiality."), [
        hero(("Tarifas, plazos y presupuesto", "Rates, turnaround and quotes"),
             ("Cobro por palabra o por página según el documento. Antes de empezar te doy el precio y el plazo por escrito.",
              "I charge per word or per page depending on the document. Before I start, I give you the price and turnaround in writing."),
             [presupuesto(), llamar("secondary")], variante="texto"),
        bloque_precios(("Tarifas orientativas", "Indicative rates"),
                       ("Tarifas y plazos orientativos por tipo de trabajo", "Indicative rates and turnaround by type of work"),
                       [("Trabajo", "Job"), ("Tarifa", "Rate"), ("Plazo orientativo", "Approximate turnaround")],
                       [(("Traducción general", "General translation"), mas(0.09, ("por palabra", "per word")),
                         ("Unas 2.000 palabras por día laborable", "About 2,000 words per working day")),
                        (("Traducción técnica o jurídica", "Technical or legal translation"), mas(0.12, ("por palabra", "per word")),
                         ("Unas 1.500 palabras por día laborable", "About 1,500 words per working day")),
                        (("Traducción jurada de un certificado", "Sworn translation of a certificate"), mas(35, ("por documento", "per document")),
                         ("De 2 a 3 días laborables", "2 to 3 working days")),
                        (("Traducción jurada de otros documentos", "Sworn translation of other documents"), mas(30, ("por página", "per page")),
                         ("Según la extensión", "Depending on length")),
                        (("Revisión de una traducción", "Proofreading a translation"), mas(0.04, ("por palabra", "per word")),
                         ("Unas 5.000 palabras por día laborable", "About 5,000 words per working day")),
                        (("Interpretación", "Interpreting"), mas(60, ("por hora, mínimo dos horas", "per hour, two-hour minimum")),
                         ("Con 48 horas de aviso", "With 48 hours' notice"))],
                       intro=("Son cifras de partida. El precio final depende del idioma, de la dificultad del texto y de lo que cuente como palabra en tu documento.",
                              "These are starting figures. The final price depends on the language, the difficulty of the text and what counts as a word in your document."),
                       nota=nota, boton_=presupuesto("secondary"), tono="base"),
        bloque_valores(("Urgencias y envíos", "Rush jobs and delivery"), [
            (("Recargo por urgencia", "Rush surcharge"),
             ("Si lo necesitas antes del plazo normal y puedo hacerlo, se aplica un recargo que te digo antes de empezar.",
              "If you need it before the normal turnaround and I can do it, a surcharge applies that I tell you before I start.")),
            (("Entrega en PDF", "Delivery as PDF"),
             ("Recibes el documento por correo. Las traducciones no juradas se entregan también en Word si lo quieres.",
              "You receive the document by email. Non-sworn translations are also delivered in Word if you wish.")),
            (("Entrega en papel", "Delivery on paper"),
             ("Las juradas en papel se envían por mensajería o se recogen. El envío se cobra aparte.",
              "Sworn translations on paper are sent by courier or collected. Postage is charged separately.")),
        ], eyebrow=("Plazos", "Deadlines"), estilo="relleno", tono="relleno", anchor="entrega"),
        bloque_pasos(("Cómo pedir presupuesto", "How to ask for a quote"), [
            (("Envíalo por correo", "Email it to me"),
             ("Manda el documento por correo, en PDF o en foto legible. En el mensaje di el idioma y para qué lo necesitas.",
              "Email the document, as a PDF or a legible photo. In the message, say the language and what you need it for.")),
            (("Te contesto", "I reply"),
             ("Miro el texto y te digo el precio, el plazo y si hace falta jurada. Normalmente el mismo día laborable.",
              "I look at the text and tell you the price, the turnaround and whether it needs to be sworn. Usually the same working day.")),
            (("Aceptas y empiezo", "You accept and I start"),
             ("Con tu conformidad por escrito empiezo a traducir. Si tengo una duda, te la consulto.",
              "Once you agree in writing I start translating. If I have a doubt, I ask you.")),
            (("Entrega y factura", "Delivery and invoice"),
             ("Recibes la traducción y la factura. Si ves algo que corregir, me lo dices y lo reviso.",
              "You receive the translation and the invoice. If you see something to correct, tell me and I will review it.")),
        ], tono="base", anchor="pedir"),
        bloque_texto(("Confidencialidad", "Confidentiality"), [
            ("Lo que me envías es confidencial. Lo veo yo y no lo comparto con nadie. Si tu caso lo requiere, firmo un acuerdo de confidencialidad antes de que me mandes nada.",
             "What you send me is confidential. I see it and share it with nobody. If your case requires it, I sign a confidentiality agreement before you send me anything."),
            ("Trato tus datos solo para hacer el trabajo y para facturarlo. Lo explico en la política de privacidad.",
             "I use your data only to do the work and to invoice it. I explain this in the privacy policy."),
        ], eyebrow=("Tus documentos", "Your documents"), tono="relleno", anchor="confidencialidad"),
        bloque_cta(("Envíame el documento por correo", "Email me the document"),
                   ("Te respondo con precio y plazo. No te comprometes a nada hasta que aceptes el presupuesto.",
                    "I reply with price and turnaround. You are not committing to anything until you accept the quote."),
                   [boton(("Enviar por correo", "Send by email"), a_correo(), "primary", "mail"), presupuesto("ghost")], tono="base"),
    ])

    cont = pagina("contacto", ("contacto", "contact"), ("Contacto y presupuesto", "Contact and quote"),
                  ("Pide presupuesto con el formulario, por teléfono o por WhatsApp, y envía el documento por correo. Horario de atención y datos de contacto.",
                   "Ask for a quote with the form, by phone or on WhatsApp, and email the document. Opening hours and contact details."), [
        hero(("Pide presupuesto", "Ask for a quote"),
             ("Cuéntame qué necesitas con el formulario, por teléfono o por WhatsApp. El documento envíalo por correo: aquí no se adjuntan archivos.",
              "Tell me what you need with the form, by phone or on WhatsApp. Email me the document: files cannot be attached here."),
             [llamar(), whatsapp()], variante="texto"),
        bloque_contacto(("Cuéntame qué necesitas", "Tell me what you need"),
                        ("Dime de qué idioma a qué idioma, qué tipo de documento es y cuándo lo necesitas. Después envíalo por correo a la dirección de abajo.",
                         "Tell me from which language to which, what kind of document it is and when you need it. Then email it to the address below."),
                        campos_consulta(("Cuéntame el encargo", "Tell me about the job"),
                                        servicios=[("Traducción jurada", "Sworn translation"), ("Traducción no jurada", "Non-sworn translation"),
                                                   ("Interpretación", "Interpreting"), ("Revisión de una traducción", "Proofreading"),
                                                   ("Otro", "Other")],
                                        extra=[campo("idiomas", "text", ("Idioma de origen y de destino", "Source and target language"), False, maxlen=120),
                                               campo("plazo", "text", ("Cuándo lo necesitas", "When you need it"), False, maxlen=120)]),
                        ("Pedir presupuesto", "Request a quote"), horas,
                        notas=("Trabajo a distancia: la dirección es para envíos de originales y de traducciones juradas en papel. Uso lo que me cuentes solo para responderte.",
                               "I work remotely: the address is for sending originals and sworn translations on paper. I use what you tell me only to reply to you."),
                        exito=("Gracias. Te respondo con el presupuesto en un día laborable. Recuerda enviar el documento por correo.",
                               "Thank you. I will reply with the quote within one working day. Remember to email me the document.")),
    ])

    return plantilla(KEY, ("Traductor e intérprete", "Translator and interpreter"),
                     ("Traductor e intérprete independiente con pares de idiomas, tipos de documento, traducción jurada, tarifas y plazos de ejemplo, cómo pedir presupuesto por correo y confidencialidad.",
                      "Independent translator and interpreter with language pairs, document types, sworn translation, example rates and turnaround, how to ask for a quote by email and confidentiality."),
                     "profesionales", ("traductor interprete traductora traduccion traducciones traduccion jurada traductor jurado traductor interprete jurado interpretacion idiomas traducir documentos apostilla certificados freelance",
                                       "translator interpreter translation sworn translation sworn translator interpreting languages translate documents apostille certificates freelance"),
                     look, cabecera(nav, marco([llamar(t=("Llamar", "Call")),
                                                whatsapp("secondary", t=("WhatsApp", "WhatsApp"))],
                                               dir="row", gap=8, wrap=True)),
                     pie(nav, ("Traducción e interpretación. Traducción jurada y no jurada, interpretación y revisión.",
                               "Translation and interpreting. Sworn and non-sworn translation, interpreting and proofreading."),
                         ("Lunes a viernes 9:00 a 14:00 y 16:00 a 19:00.",
                          "Monday to Friday 9 am to 2 pm and 4 pm to 7 pm.")),
                     [inicio, idiomas, tarifas, cont],
                     exento={"mapa": "Traductor que trabaja a distancia: no atiende al público en un local, la dirección es solo postal para envíos de originales y de traducciones juradas en papel."})
