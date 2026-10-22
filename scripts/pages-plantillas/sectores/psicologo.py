# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
from lib import *
from looks import LOOKS

KEY = "psicologo"

NOTA = ("Precios de ejemplo: cámbialos por los tuyos.", "Example prices: replace them with your own.")
WA_CITA = ("Pedir cita por WhatsApp", "Book by WhatsApp")
SOLO_CITA = ("Por WhatsApp solo gestionamos citas: pedirlas, moverlas o cancelarlas. No atendemos consultas ni temas clínicos por mensaje.",
             "We use WhatsApp only for appointments: booking, moving or cancelling them. We do not handle consultations or clinical matters by message.")
URGENCIA = ("Esta consulta no es un servicio de urgencias. Si tú o alguien cercano está en peligro inmediato, llama al 112. Si estás pensando en hacerte daño, puedes llamar al 024, la línea de atención a la conducta suicida, gratuita y abierta las 24 horas.",
            "This practice is not an emergency service. If you or someone close to you is in immediate danger, call 112. If you are thinking of harming yourself, you can call 024, the suicidal behaviour helpline, free and open 24 hours a day.")


def urgencia(tono="relleno"):
    return bloque_texto(("Si es una urgencia", "If it is an emergency"), [URGENCIA], tono=tono, anchor="urgencias")


def _equipo(titulo, personas, intro=None, tono="base"):
    """Ficha del equipo con el número de colegiado visible (relleno detectable)."""
    tarjetas = [marco([imagen("portrait", ("Foto de la persona del equipo", "Photo of the team member"), ratio=0.9),
                       h3(("Nombre Apellido", "First Last")), peq(rol, color="$muted"), p(frase),
                       peq(COLEGIADO, color="$muted")], tag="li", gap=10) for rol, frase in personas]
    return seccion("team", [titular(titulo, intro), rejilla(tarjetas, cols=2)], tono=tono, anchor="equipo")


def build():
    nueva(KEY)
    look = LOOKS["arena"]
    nav = [("inicio", ("Inicio", "Home")), ("areas", ("Áreas de trabajo", "Areas of work")),
           ("sesiones", ("Sesiones y tarifas", "Sessions and fees")), ("contacto", ("Contacto", "Contact"))]
    horas = [(("Lunes a viernes", "Monday to Friday"),
              ("10:00 a 14:00 y 16:00 a 20:00, con cita", "10:00 am to 2:00 pm and 4:00 pm to 8:00 pm, by appointment")),
             (("Sábado y domingo", "Saturday and Sunday"), ("Cerrado", "Closed")),
             (("Sesiones online", "Online sessions"), ("En las mismas franjas, con cita", "In the same slots, by appointment"))]
    cita = lambda v="primary": boton(("Pedir cita", "Book an appointment"), a_pagina("contacto", "contacto"), v)

    inicio = pagina("inicio", ("", ""), ("Consulta de psicología", "Psychology practice"),
                    ("Consulta de psicología presencial y online: ansiedad, pareja, infancia y duelo. Primera sesión para conocernos. Pide cita por teléfono o WhatsApp.",
                     "Psychology practice in person and online: anxiety, couples, children and grief. A first session to get to know each other. Book by phone or WhatsApp."), [
        hero(("Un espacio tranquilo para hablar de lo que te pesa", "A quiet space to talk about what weighs on you"),
             ("Consulta de psicología, en persona y online. En la primera sesión nos conocemos y hablamos de lo que te trae, sin ningún compromiso de seguir.",
              "Psychology practice, in person and online. In the first session we get to know each other and talk about what brings you, with no commitment to continue."),
             [cita(), whatsapp("secondary", t=WA_CITA)], eyebrow=("Psicología", "Psychology"),
             extra=peq(COLEGIADO, color="$muted"), variante="texto", tono="relleno"),
        bloque_texto(("Sin prisa y sin juicios", "No rush and no judgement"), [
            ("Aquí no hace falta tener claro qué te pasa ni cómo explicarlo. Empezamos por donde tú quieras y al ritmo que necesites.",
             "You do not need to be clear about what is wrong or how to explain it. We start wherever you like and go at your pace."),
            ("Te explicamos en lenguaje llano en qué consiste el trabajo, cuánto puede durar y qué se espera de cada parte.",
             "We explain in plain language what the work involves, how long it may take and what each side is expected to do."),
        ], imagen_=("landscape", ("Consulta de psicología con dos butacas y luz natural", "Psychology consulting room with two armchairs and natural light")),
            tono="base"),
        bloque_valores(("A quién atendemos", "Who we see"), [
            (("Adultos", "Adults"),
             ("Ansiedad, estrés, bajo estado de ánimo, y momentos de cambio o de bloqueo.",
              "Anxiety, stress, low mood, and times of change or feeling stuck.")),
            (("Parejas", "Couples"),
             ("Comunicación, conflictos repetidos, crisis y decisiones difíciles.",
              "Communication, recurring conflict, crises and difficult decisions.")),
            (("Infancia y adolescencia", "Children and teenagers"),
             ("Miedos, conducta, colegio y cambios en casa, con la familia implicada.",
              "Fears, behaviour, school and changes at home, with the family involved.")),
            (("Duelo y pérdidas", "Grief and loss"),
             ("Acompañamiento cuando alguien ha muerto o algo importante se ha acabado.",
              "Support when someone has died or something important has ended.")),
        ], cols=4, estilo="linea", tono="relleno", eyebrow=("Áreas", "Areas")),
        bloque_pasos(("La primera sesión", "The first session"), [
            (("Hablamos", "We talk"),
             ("Cuentas lo que te trae y desde cuándo. Nadie te presiona para decir más de lo que quieras.",
              "You tell us what brings you and since when. Nobody pressures you to say more than you wish.")),
            (("Lo ordenamos", "We make sense of it"),
             ("Ponemos nombre a lo que ocurre y vemos qué puede ayudarte.",
              "We put a name to what is happening and look at what may help.")),
            (("Decides", "You decide"),
             ("Te proponemos cómo seguir. Si prefieres pensártelo o buscar a otra persona, también está bien.",
              "We suggest how to continue. If you prefer to think it over or look for someone else, that is fine too.")),
        ], intro=("Dura unos 50 minutos. Puedes venir a la consulta o hacerla por videollamada.",
                  "It lasts about 50 minutes. You can come to the practice or do it by video call."), tono="base"),
        bloque_texto(("Lo que se habla aquí, se queda aquí", "What is said here stays here"), [
            ("Lo que cuentas está protegido por el secreto profesional. Las pocas excepciones que prevé la ley te las explicamos en la primera sesión.",
             "What you share is protected by professional confidentiality. The few exceptions provided for by law are explained to you in the first session."),
            ("Tus datos y tu historial se guardan con las medidas que exige la normativa de protección de datos.",
             "Your details and records are kept with the safeguards required by data protection rules."),
        ], tono="relleno", anchor="confidencialidad"),
        _equipo(("Quién te atiende", "Who sees you"), [
            (("Psicólogo/a sanitario/a", "Healthcare psychologist"),
             ("Adultos y parejas, en consulta y online.", "Adults and couples, in person and online.")),
            (("Psicólogo/a infanto-juvenil", "Child and adolescent psychologist"),
             ("Niños, adolescentes y sus familias.", "Children, teenagers and their families.")),
        ], tono="base"),
        urgencia("relleno"),
        bloque_cta(("Cuando quieras, empezamos", "Whenever you are ready, we begin"),
                   ("Pide cita y elige si prefieres venir o hacerla online. Te respondemos en uno o dos días laborables.",
                    "Book and choose whether you prefer to come in or do it online. We reply within one or two working days."),
                   [cita(), whatsapp("ghost", t=WA_CITA)], tono="base"),
    ])

    areas = pagina("areas", ("areas", "areas"), ("Áreas de trabajo", "Areas of work"),
                   ("En qué puede ayudar la psicología: ansiedad, estado de ánimo, pareja, infancia y adolescencia, duelo y momentos de cambio.",
                    "How psychology can help: anxiety, mood, couples, children and teenagers, grief and times of change."), [
        hero(("Áreas de trabajo", "Areas of work"),
             ("Si no ves aquí lo que te pasa, escríbenos igualmente: en la primera sesión vemos si podemos ayudarte o a quién acudir.",
              "If you do not see what is happening to you here, get in touch anyway: in the first session we see whether we can help or who you should go to."),
             [cita()], variante="texto", tono="relleno"),
        bloque_valores(("En qué podemos acompañarte", "What we can support you with"), [
            (("Ansiedad y estrés", "Anxiety and stress"),
             ("Preocupación constante, tensión, mal sueño, ataques de pánico o miedo a situaciones concretas.",
              "Constant worry, tension, poor sleep, panic attacks or fear of particular situations.")),
            (("Estado de ánimo", "Mood"),
             ("Tristeza, apatía, cansancio emocional y falta de ganas que se alargan en el tiempo.",
              "Sadness, apathy, emotional exhaustion and lack of motivation that go on for a long time.")),
            (("Pareja", "Couples"),
             ("Discusiones que se repiten, falta de comunicación, infidelidades, separaciones y duelo de pareja.",
              "Recurring arguments, poor communication, infidelity, separation and the grief of a break-up.")),
            (("Infancia y adolescencia", "Children and teenagers"),
             ("Miedos, rabietas, problemas en el colegio, cambios de conducta y dificultades con los amigos.",
              "Fears, tantrums, problems at school, changes in behaviour and trouble with friends.")),
            (("Duelo y pérdidas", "Grief and loss"),
             ("La muerte de alguien querido, una ruptura, perder un trabajo o un cambio grande de vida.",
              "The death of a loved one, a break-up, losing a job or a big change in life.")),
            (("Autoestima y relaciones", "Self-esteem and relationships"),
             ("Inseguridad, dificultad para poner límites o para relacionarte como te gustaría.",
              "Insecurity, difficulty setting limits or relating to others the way you would like.")),
        ], cols=3, estilo="relleno", tono="base", anchor="que-tratamos"),
        bloque_texto(("Cómo trabajamos", "How we work"), [
            ("No hay una única forma de hacer terapia. Te explicamos el método en lenguaje llano y lo ajustamos a ti y a lo que te pasa.",
             "There is no single way of doing therapy. We explain the method in plain language and adapt it to you and to what is happening."),
            ("Revisamos contigo cómo va, cada cierto tiempo, y si no estás cómodo con el rumbo lo cambiamos.",
             "We check in with you from time to time on how it is going, and if you are not comfortable with the direction we change it."),
            ("La psicología no sustituye a un médico. Si tomas medicación, no la cambies ni la dejes sin hablarlo con quien te la recetó.",
             "Psychology does not replace a doctor. If you take medication, do not change or stop it without talking to the person who prescribed it."),
        ], tono="relleno", anchor="metodo"),
        bloque_faq(("Preguntas frecuentes", "Frequently asked questions"), [
            (("¿Cuántas sesiones necesitaré?", "How many sessions will I need?"),
             ("No hay un número fijo. Lo hablamos en la primera sesión y lo revisamos de vez en cuando. Hay personas que mejoran en pocas semanas y otras que prefieren un acompañamiento más largo.",
              "There is no fixed number. We talk about it in the first session and review it from time to time. Some people improve in a few weeks and others prefer longer support.")),
            (("¿Es para mí si no es nada grave?", "Is it for me if it is nothing serious?"),
             ("Para venir basta con que algo te preocupe o te cueste. No hace falta una crisis ni un diagnóstico.",
              "All it takes is that something worries or troubles you. You do not need a crisis or a diagnosis.")),
            (("¿Atendéis a menores?", "Do you see minors?"),
             ("Sí. Necesitamos la autorización de madre, padre o tutores, y parte del trabajo se hace con la familia.",
              "Yes. We need authorisation from a parent or guardian, and part of the work is done with the family.")),
            (("¿Hace falta que me derive un médico?", "Do I need a doctor's referral?"),
             ("No. Puedes pedir cita directamente.", "No. You can book directly.")),
        ], tono="base"),
        bloque_cta(("¿Hablamos?", "Shall we talk?"), None, [cita(), whatsapp("ghost", t=WA_CITA)], tono="relleno"),
    ])

    ses = pagina("sesiones", ("sesiones", "sessions"), ("Sesiones y tarifas", "Sessions and fees"),
                 ("Cómo son las sesiones presenciales y online, tarifas orientativas, pago y cancelaciones, y confidencialidad.",
                  "What in-person and online sessions are like, indicative fees, payment and cancellations, and confidentiality."), [
        hero(("Sesiones y tarifas", "Sessions and fees"),
             ("Sabes lo que cuesta cada sesión antes de venir. Si el precio es un problema, dínoslo y buscamos una solución.",
              "You know what each session costs before you come. If the price is a problem, tell us and we will look for a solution."),
             [cita()], variante="texto", tono="relleno"),
        bloque_valores(("Presencial u online", "In person or online"), [
            (("En la consulta", "At the practice"),
             ("Una sala tranquila y privada. Si necesitas acceso sin escalones, avísanos al reservar.",
              "A quiet, private room. If you need step-free access, tell us when you book.")),
            (("Por videollamada", "By video call"),
             ("Necesitas un sitio donde nadie te oiga y una conexión estable. Te enviamos el enlace por correo antes de la sesión.",
              "You need somewhere nobody can hear you and a stable connection. We send you the link by email before the session.")),
        ], cols=2, estilo="borde", tono="base", anchor="modalidades",
            intro=("El trabajo es el mismo. Elige lo que te resulte más cómodo, y puedes cambiar de una a otra.",
                   "The work is the same. Choose whatever suits you best, and you can switch from one to the other.")),
        bloque_precios(("Tarifas por sesión", "Fees per session"), ("Tarifas de las sesiones", "Session fees"),
                       [("Sesión", "Session"), ("Duración", "Length"), ("Precio", "Price")],
                       [(("Primera sesión", "First session"), ("50 min", "50 min"), eur(60)),
                        (("Sesión individual, presencial", "Individual session, in person"), ("50 min", "50 min"), eur(60)),
                        (("Sesión individual, online", "Individual session, online"), ("50 min", "50 min"), eur(55)),
                        (("Sesión de pareja", "Couples session"), ("75 min", "75 min"), eur(85)),
                        (("Infancia y adolescencia", "Children and teenagers"), ("50 min", "50 min"), eur(60))],
                       nota=NOTA, anchor="tarifas", tono="relleno"),
        bloque_texto(("Pago, cancelaciones y citas", "Payment, cancellations and appointments"), [
            ("Se paga al terminar la sesión, con tarjeta o en efectivo. Te damos factura si la pides.",
             "You pay at the end of the session, by card or in cash. We give you an invoice if you ask."),
            ("Si no puedes venir, avísanos con 24 horas y la movemos sin coste.",
             "If you cannot come, let us know 24 hours ahead and we will move it at no cost."),
            SOLO_CITA,
        ], tono="base", anchor="pago"),
        bloque_faq(("Preguntas sobre las sesiones", "Questions about sessions"), [
            (("¿Es confidencial lo que cuento?", "Is what I say confidential?"),
             ("Sí. Está protegido por el secreto profesional. Las pocas excepciones que prevé la ley te las explicamos en la primera sesión.",
              "Yes. It is protected by professional confidentiality. The few exceptions provided for by law are explained to you in the first session.")),
            (("¿Con qué frecuencia son las sesiones?", "How often are sessions?"),
             ("Al principio suele ser una a la semana o cada quince días. Después se espacian según cómo vayas.",
              "At first it is usually weekly or fortnightly. Later they are spaced out depending on how you are doing.")),
            (("¿Puedo traer a alguien?", "Can I bring someone?"),
             ("Si lo prefieres, sí, en alguna sesión. Lo hablamos antes.", "If you prefer, yes, for some sessions. We talk about it beforehand.")),
        ], tono="relleno"),
        bloque_cta(("¿Pedimos la primera cita?", "Shall we book the first appointment?"), None,
                   [cita(), whatsapp("ghost", t=WA_CITA)], tono="base"),
    ])

    campos = [campo("nombre", "text", ("Nombre", "Name"), True, auto="name", maxlen=120),
              campo("telefono", "tel", ("Teléfono", "Phone"), True, auto="tel", maxlen=30),
              campo("correo", "email", ("Correo electrónico (opcional)", "Email (optional)"), False, auto="email", maxlen=200),
              campo("modalidad", "select", ("Modalidad", "Format"), False,
                    opciones=[("En la consulta", "At the practice"), ("Online", "Online"), ("Me da igual", "Either")]),
              campo("franja", "select", ("Franja que te viene bien", "Time of day that suits you"), False,
                    opciones=[("Mañana", "Morning"), ("Tarde", "Afternoon")]),
              campo("mensaje", "textarea", ("Mensaje (opcional). Por favor, no cuentes aquí datos clínicos.",
                                            "Message (optional). Please do not share clinical details here."), False, maxlen=500)]

    cont = pagina("contacto", ("contacto", "contact"), ("Contacto y cita", "Contact and appointments"),
                  ("Pide cita de psicología por teléfono, WhatsApp o con el formulario. Horario, consulta presencial y online, y qué hacer en una urgencia.",
                   "Book a psychology appointment by phone, WhatsApp or with the form. Opening hours, in-person and online sessions, and what to do in an emergency."), [
        hero(("Pide tu cita", "Book your appointment"),
             ("Llámanos o rellena el formulario y te contestamos en uno o dos días laborables. En la primera sesión nos conocemos.",
              "Call us or fill in the form and we reply within one or two working days. In the first session we get to know each other."),
             [llamar(), whatsapp(t=WA_CITA)], variante="texto", tono="relleno"),
        bloque_contacto(("Cómo encontrarnos", "How to find us"),
                        ("Si tienes dudas para llegar o necesitas acceso sin escalones, avísanos al pedir cita.", "If you are unsure how to get here or need step-free access, tell us when you book."),
                        campos, ("Pedir cita", "Request appointment"), horas,
                        notas=(COLEGIADO[0] + ". " + SOLO_CITA[0], COLEGIADO[1] + ". " + SOLO_CITA[1]),
                        exito=("Gracias. Te llamaremos o te escribiremos para fijar la cita; no hace falta que cuentes nada más por escrito.",
                               "Thank you. We will call or write to set the appointment; you do not need to tell us anything more in writing.")),
        urgencia("base"),
        bloque_mapa(tono="relleno"),
    ])

    footer = pie(nav, ("Consulta de psicología, en persona y online. Con cita previa.",
                       "Psychology practice, in person and online. By appointment."),
                 ("Lunes a viernes 10:00 a 14:00 y 16:00 a 20:00, con cita.", "Monday to Friday 10 am to 2 pm and 4 pm to 8 pm, by appointment."))
    footer["children"].insert(1, peq(COLEGIADO, color="$muted"))

    return plantilla(KEY, ("Consulta de psicología", "Psychology practice"),
                     ("Consulta de psicología con áreas de trabajo, primera sesión explicada, sesiones presenciales y online, tarifas orientativas, confidencialidad, equipo colegiado y cita por teléfono o WhatsApp.",
                      "Psychology practice with areas of work, first session explained, in-person and online sessions, indicative fees, confidentiality, registered team and booking by phone or WhatsApp."),
                     "salud", ("psicologo psicologa psicologia consulta psicologica terapia ansiedad pareja infantil duelo terapia online psicoterapia",
                               "psychologist psychology therapist therapy counselling anxiety couples child grief online therapy"),
                     look, cabecera(nav, llamar(t=("Llamar", "Call"))), footer, [inicio, areas, ses, cont])
