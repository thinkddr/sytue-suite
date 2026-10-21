# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
from lib import *
from looks import LOOKS

KEY = "veterinaria"


def con_botones(sec, botones):
    """Añade una fila de botones al final de una sección ya hecha."""
    sec["children"].append(marco(botones, dir="row", gap=12, wrap=True))
    return sec


def build():
    nueva(KEY)
    look = LOOKS["oliva"]
    nav = [("inicio", ("Inicio", "Home")), ("servicios", ("Servicios y precios", "Services and prices")),
           ("visita", ("Primera visita y urgencias", "First visit and emergencies")),
           ("contacto", ("Contacto", "Contact"))]
    horas = [(("Lunes a viernes", "Monday to Friday"),
              ("9:00 a 14:00 y 16:00 a 20:00", "9:00 am to 2:00 pm and 4:00 pm to 8:00 pm")),
             (("Sábado", "Saturday"), ("10:00 a 13:30", "10:00 am to 1:30 pm")),
             (("Domingo", "Sunday"), ("Cerrado", "Closed")),
             (("Urgencias", "Emergencies"),
              ("Llama antes de venir y te decimos qué hacer", "Call before you come and we will tell you what to do"))]
    cta_cab = marco([llamar(t=("Llamar", "Call")), whatsapp("secondary", t=("WhatsApp", "WhatsApp"))],
                    dir="row", gap=8, wrap=True)
    cita = lambda v="primary": boton(("Pedir cita", "Book an appointment"), a_pagina("contacto", "contacto"), v)
    nota = ("Precios de ejemplo: cámbialos por los tuyos. Incluyen IVA.",
            "Example prices: replace them with your own. VAT included.")
    cab_precio = [("Servicio", "Service"), ("Precio", "Price")]

    inicio = pagina("inicio", ("", ""), ("Clínica veterinaria", "Veterinary clinic"),
                    ("Clínica veterinaria para perros, gatos y otros animales: consulta, vacunas, cirugía, dentística y urgencias. Llama o escribe por WhatsApp.",
                     "Veterinary clinic for dogs, cats and other pets: consultations, vaccines, surgery, dental care and emergencies. Call or message us on WhatsApp."), [
        hero(("Cuidamos de tu animal como si fuera nuestro", "We look after your animal as if it were our own"),
             ("Consulta con cita, vacunas, cirugía y urgencias. Te explicamos qué le pasa, qué opciones hay y cuánto cuesta antes de hacer nada.",
              "Consultations by appointment, vaccines, surgery and emergencies. We explain what is wrong, what the options are and what they cost before we do anything."),
             [llamar(), whatsapp(), cita("ghost")],
             eyebrow=("Clínica veterinaria", "Veterinary clinic"),
             extra=peq(COLEGIADO, color="$muted"),
             imagen_=("photo", ("Veterinaria revisando a un perro en la consulta", "Vet examining a dog in the consulting room"))),
        con_botones(bloque_checks(("Si es una urgencia, llama antes de venir", "If it is an emergency, call before you come"), [
            ("Dificultad para respirar o encías pálidas o azuladas.", "Difficulty breathing, or pale or bluish gums."),
            ("Ha comido algo tóxico: chocolate, veneno, medicamentos o plantas.", "It has eaten something toxic: chocolate, poison, medicines or plants."),
            ("Convulsiones, desmayo o no responde.", "Seizures, fainting or no response."),
            ("Atropello, caída, herida profunda o sangrado que no para.", "Road accident, fall, deep wound or bleeding that will not stop."),
            ("Vómitos repetidos, abdomen hinchado o no puede orinar.", "Repeated vomiting, a swollen belly or being unable to urinate."),
            ("Golpe de calor: jadeo intenso, babeo y debilidad.", "Heatstroke: heavy panting, drooling and weakness."),
        ], eyebrow=("Urgencias", "Emergencies"), tono="relleno", cols=2, anchor="urgencias"),
            [llamar(), whatsapp("ghost")]),
        bloque_valores(("Lo que hacemos", "What we do"), [
            (("Consulta general", "General consultations"),
             ("Revisiones, vómitos, cojeras, problemas de piel, de oídos y todo lo que te preocupe.",
              "Check-ups, vomiting, limping, skin and ear problems and anything that worries you.")),
            (("Vacunas y desparasitación", "Vaccines and parasite control"),
             ("Calendario según la edad y la vida de tu animal, con recordatorio por mensaje.",
              "A schedule based on your animal's age and lifestyle, with a reminder by message.")),
            (("Cirugía", "Surgery"),
             ("Esterilización, extracción de masas y cirugía de urgencia, con revisión previa y presupuesto por escrito.",
              "Neutering, lump removal and emergency surgery, with a pre-op check and a written quote.")),
            (("Dentística", "Dental care"),
             ("Revisión de boca, limpieza dental y extracciones cuando hacen falta.",
              "Mouth check, dental cleaning and extractions when needed.")),
            (("Chip e identificación", "Microchip and identification"),
             ("Implantación del microchip y papeles para viajar con tu animal.",
              "Microchip implantation and paperwork to travel with your animal.")),
            (("Peluquería y baño", "Grooming and bathing"),
             ("Baño, corte, cepillado y uñas, si tu animal lo necesita.",
              "Bathing, trimming, brushing and nails, if your animal needs it.")),
        ], eyebrow=("Servicios", "Services"), estilo="borde"),
        bloque_pasos(("Cómo es la primera visita", "What the first visit is like"), [
            (("Nos cuentas", "You tell us"),
             ("Qué le pasa, desde cuándo, qué come y qué medicación toma.",
              "What is wrong, since when, what it eats and what medication it takes.")),
            (("Lo revisamos", "We examine it"),
             ("Peso, temperatura, corazón, boca, piel y lo que haga falta, con calma.",
              "Weight, temperature, heart, mouth, skin and whatever else is needed, taking our time.")),
            (("Te explicamos", "We explain"),
             ("Qué hemos visto y qué opciones hay. Si hacen falta pruebas, te decimos cuánto cuestan antes.",
              "What we found and what the options are. If tests are needed, we tell you the cost beforehand.")),
            (("Cuidados en casa", "Care at home"),
             ("Te lo dejamos por escrito y te decimos cuándo volver a revisarlo.",
              "We put it in writing and tell you when to bring it back for a check.")),
        ], tono="relleno"),
        bloque_equipo(("El equipo", "The team"), [
            (("Veterinaria o veterinario", "Vet"),
             ("Atiende la consulta y la cirugía. Cuenta con el número de colegiado que figura en la web.",
              "Handles consultations and surgery. Their registration number is shown on this site.")),
            (("Auxiliar veterinario", "Veterinary nurse"),
             ("Te recibe, prepara la consulta y ayuda a tu animal a estar tranquilo.",
              "Welcomes you, prepares the consultation and helps your animal stay calm.")),
            (("Peluquería canina y felina", "Dog and cat grooming"),
             ("Baño y corte con paciencia, a su ritmo.", "Bathing and trimming with patience, at their own pace.")),
        ], tono="base"),
        bloque_opiniones(("Lo que dicen nuestros clientes", "What our clients say"),
                         [("el trato a mi perro", "how my dog was treated"), ("las explicaciones", "the explanations"),
                          ("la atención en una urgencia", "the care in an emergency")], tono="relleno"),
        bloque_cta(("¿Pedimos cita para tu animal?", "Shall we book your animal in?"),
                   ("Dinos la especie, la edad y qué le pasa, y te damos hora lo antes posible.",
                    "Tell us the species, the age and what is wrong, and we will give you the earliest slot.")),
    ])

    serv = pagina("servicios", ("servicios", "services"), ("Servicios y precios", "Services and prices"),
                  ("Precios de consulta, vacunas, desparasitación, identificación y dentística. Cirugía con presupuesto previo.",
                   "Prices for consultations, vaccines, parasite control, identification and dental care. Surgery with a prior quote."), [
        hero(("Servicios y precios", "Services and prices"),
             ("Estos son nuestros precios habituales. Si hacen falta pruebas o una intervención, te damos el presupuesto por escrito antes de empezar.",
              "These are our usual prices. If tests or surgery are needed, we give you the quote in writing before we start."),
             [cita()], variante="texto"),
        bloque_precios(("Consulta y vacunas", "Consultations and vaccines"), ("Precios de consulta y vacunas", "Consultation and vaccine prices"),
                       cab_precio,
                       [(("Consulta general", "General consultation"), eur(30)),
                        (("Revisión de seguimiento", "Follow-up check"), eur(18)),
                        (("Vacuna polivalente del perro", "Dog multi-purpose vaccine"), eur(38)),
                        (("Vacuna de la rabia", "Rabies vaccine"), eur(28)),
                        (("Vacuna trivalente del gato", "Cat core vaccine"), eur(34)),
                        (("Vacuna de la leucemia felina", "Feline leukaemia vaccine"), eur(40))],
                       anchor="consulta", tono="base"),
        bloque_precios(("Desparasitación e identificación", "Parasite control and identification"),
                       ("Precios de desparasitación e identificación", "Parasite control and identification prices"),
                       cab_precio,
                       [(("Desparasitación interna (pastilla)", "Internal parasite treatment (tablet)"), desde(8)),
                        (("Pipeta o collar antiparasitario", "Spot-on or anti-parasite collar"), desde(12)),
                        (("Microchip con registro", "Microchip with registration"), eur(45)),
                        (("Pasaporte europeo", "EU pet passport"), eur(15)),
                        (("Limpieza dental con anestesia", "Dental cleaning under anaesthesia"), desde(95)),
                        (("Baño y corte de uñas", "Bath and nail trim"), desde(20))],
                       nota=nota, anchor="prevencion", tono="relleno"),
        bloque_texto(("Cirugía: presupuesto antes de operar", "Surgery: a quote before we operate"), [
            ("El precio de una cirugía depende de la especie, el peso, la edad y la intervención. Por eso no ponemos una tabla: lo revisamos y te lo damos por escrito.",
             "The price of surgery depends on the species, weight, age and procedure. That is why we do not list a table: we examine your animal and give you the price in writing."),
            ("Antes de operar hacemos una revisión y, si hace falta, un análisis de sangre. Te decimos cuándo dejarlo en ayunas y a qué hora recogerlo.",
             "Before operating we do a check-up and, if needed, a blood test. We tell you when to fast it and what time to collect it.")],
            eyebrow=("Cirugía", "Surgery"), anchor="cirugia", tono="base"),
        bloque_faq(("Preguntas frecuentes", "Frequently asked questions"), [
            (("¿Cada cuánto hay que vacunar?", "How often should I vaccinate?"),
             ("Depende de la edad, de la especie y de si sale de viaje o convive con otros animales. Los cachorros llevan un calendario con varios refuerzos; los adultos, refuerzos periódicos. Te lo marcamos en la consulta.",
              "It depends on age, species and whether it travels or lives with other animals. Puppies and kittens follow a schedule with several boosters; adults have periodic boosters. We set it out at the consultation.")),
            (("¿Es obligatorio el chip?", "Is a microchip compulsory?"),
             ("En los perros sí. En gatos y otras especies depende de tu comunidad autónoma. Te decimos qué te toca y lo registramos.",
              "For dogs, yes. For cats and other species it depends on your region. We tell you what applies and register it.")),
            (("¿Puedo pagar a plazos una intervención?", "Can I pay for an operation in instalments?"),
             ("Cuéntanoslo al pedir el presupuesto y lo hablamos. Antes de operar siempre sabes el precio.",
              "Tell us when you ask for the quote and we will talk it through. Before any operation you always know the price.")),
            (("¿Atendéis a otros animales además de perros y gatos?", "Do you see animals other than dogs and cats?"),
             ("Atendemos conejos, hurones y pequeños roedores. Para aves y reptiles, llámanos antes y te decimos si podemos verlos.",
              "We see rabbits, ferrets and small rodents. For birds and reptiles, call us first and we will say whether we can see them.")),
        ], tono="relleno"),
        bloque_cta(("¿Quieres pedir cita?", "Would you like to book a visit?"), None, [cita(), llamar("secondary")], tono="base"),
    ])

    visita = pagina("visita", ("primera-visita", "first-visit"), ("Primera visita y urgencias", "First visit and emergencies"),
                    ("Qué traer a la primera visita, qué hacer en una urgencia y cómo funciona el chip y la identificación de tu animal.",
                     "What to bring to the first visit, what to do in an emergency and how microchipping and identification work."), [
        hero(("Antes de venir", "Before you come"),
             ("Unas pautas sencillas para que la visita vaya bien, y qué hacer si tu animal se pone mal.",
              "A few simple tips to make the visit go well, and what to do if your animal falls ill."),
             [llamar(), cita("secondary")], variante="texto"),
        bloque_checks(("Qué traer a la primera visita", "What to bring to the first visit"), [
            ("Su cartilla de vacunación o pasaporte, si lo tiene.", "Its vaccination card or passport, if it has one."),
            ("Informes y análisis anteriores, o una foto de ellos.", "Previous reports and test results, or a photo of them."),
            ("Los medicamentos que toma o su nombre.", "The medicines it takes, or their names."),
            ("Una muestra de heces o de orina, si la consulta es por eso.", "A stool or urine sample, if that is why you are coming."),
            ("Transportín para gatos y animales pequeños, y correa para perros.", "A carrier for cats and small animals, and a lead for dogs."),
            ("Apuntado lo que has notado: desde cuándo y cómo come, bebe y descansa.", "Notes on what you have noticed: since when, and how it eats, drinks and rests."),
        ], eyebrow=("Primera visita", "First visit"), tono="base", cols=2, anchor="primera-visita"),
        bloque_pasos(("Qué hacer en una urgencia", "What to do in an emergency"), [
            (("Llama antes de venir", "Call before you come"),
             ("Cuéntanos qué ha pasado. Te decimos si venir ya, qué hacer por el camino y cuándo llegamos a atenderlo.",
              "Tell us what happened. We say whether to come now, what to do on the way and when we can see it.")),
            (("Mantén la calma", "Stay calm"),
             ("No le des comida, agua ni medicinas de personas salvo que te lo indiquemos.",
              "Do not give food, water or human medicines unless we tell you to.")),
            (("Trasládalo con cuidado", "Move it carefully"),
             ("Si está herido, usa una manta o una tabla para moverlo. Con un tóxico, trae el envase.",
              "If it is injured, use a blanket or board to move it. With a toxin, bring the packaging.")),
            (("Fuera de horario", "Out of hours"),
             ("Llama al servicio de urgencias 24 horas más cercano: 600 000 000. Escribe aquí su nombre y teléfono reales.",
              "Call the nearest 24-hour emergency service: 600 000 000. Write its real name and number here.")),
        ], intro=("Ante la duda, llama. Es mejor una llamada de más que llegar tarde.",
                  "If in doubt, call. A call too many is better than arriving too late."), tono="relleno", anchor="urgencias"),
        bloque_texto(("Chip e identificación", "Microchip and identification"), [
            ("El microchip es del tamaño de un grano de arroz y se pone con una inyección en la consulta, sin anestesia. Dura toda la vida del animal.",
             "The microchip is the size of a grain of rice and is placed with an injection in the consulting room, without anaesthetic. It lasts the animal's whole life."),
            ("Tras ponerlo lo inscribimos en el registro que corresponde a tu comunidad. Si cambias de teléfono o de domicilio, avísanos para actualizar los datos.",
             "After implanting it we register it with the register for your region. If you change phone number or address, let us know so we can update the details.")],
            eyebrow=("Identificación", "Identification"), anchor="chip", tono="base"),
        bloque_cta(("¿Te damos cita?", "Shall we book you in?"), None, [cita(), whatsapp("ghost")], tono="relleno"),
    ])

    cont = pagina("contacto", ("contacto", "contact"), ("Contacto y cita", "Contact and appointments"),
                  ("Pide cita por teléfono, WhatsApp o con el formulario. Si es una urgencia, llama. Horario y cómo llegar a la clínica.",
                   "Book by phone, WhatsApp or with the form. If it is an emergency, call. Opening hours and how to get to the clinic."), [
        hero(("Pide cita para tu animal", "Book an appointment for your animal"),
             ("Llámanos, escríbenos por WhatsApp o rellena el formulario. Si es una urgencia, llama: el formulario no se atiende al momento.",
              "Call us, message us on WhatsApp or fill in the form. If it is an emergency, call: the form is not answered straight away."),
             [llamar(), whatsapp()], variante="texto", extra=peq(COLEGIADO, color="$muted")),
        bloque_contacto(("Cómo encontrarnos", "How to find us"),
                        ("Estamos en la calle principal, con aparcamiento cerca y entrada sin escalones.",
                         "We are on the main street, with parking nearby and a step-free entrance."),
                        campos_cita(servicios=[("Consulta general", "General consultation"), ("Vacunas", "Vaccines"),
                                               ("Cirugía", "Surgery"), ("Dentística", "Dental care"),
                                               ("Chip e identificación", "Microchip and identification"),
                                               ("Peluquería", "Grooming"), ("Otro", "Other")],
                                    extra=[campo("especie", "select", ("Especie", "Species"), False,
                                                 opciones=[("Perro", "Dog"), ("Gato", "Cat"), ("Otro", "Other")]),
                                           campo("mascota", "text", ("Nombre de tu animal", "Your animal's name"), False, maxlen=80)]),
                        ("Pedir cita", "Request appointment"), horas,
                        notas=("Si es una urgencia, no uses el formulario: llama.",
                               "If it is an emergency, do not use the form: call."),
                        exito=("Gracias. Te confirmamos la cita por teléfono o WhatsApp en cuanto la tengamos.",
                               "Thank you. We will confirm your appointment by phone or WhatsApp as soon as we have it.")),
        bloque_mapa(tono="relleno"),
    ])

    return plantilla(KEY, ("Veterinaria", "Veterinary clinic"),
                     ("Clínica veterinaria con consulta, vacunas, cirugía, dentística, urgencias, primera visita, precios orientativos y botones de llamar y WhatsApp.",
                      "Veterinary clinic with consultations, vaccines, surgery, dental care, emergencies, first-visit guide, example prices and call and WhatsApp buttons."),
                     "salud", ("veterinaria veterinario clinica veterinaria mascotas perros gatos vacunas desparasitacion cirugia dentista urgencias chip peluqueria canina animales",
                               "vet veterinary clinic veterinarian pets dogs cats vaccines parasite control surgery dental emergency microchip grooming animals"),
                     look, cabecera(nav, cta_cab),
                     pie(nav, ("Clínica veterinaria. Consulta, vacunas, cirugía y urgencias. Nº de colegiado: 00-0000.",
                               "Veterinary clinic. Consultations, vaccines, surgery and emergencies. Registration no.: 00-0000."),
                         ("Lunes a viernes 9:00 a 14:00 y 16:00 a 20:00. Sábado 10:00 a 13:30.",
                          "Monday to Friday 9 am to 2 pm and 4 pm to 8 pm. Saturday 10 am to 1:30 pm.")),
                     [inicio, serv, visita, cont])
