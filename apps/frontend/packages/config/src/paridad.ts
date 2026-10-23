// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos

/**
 * REGISTRO DE PARIDAD web → nativas — NORMATIVO (CONVENTIONS.md §13.5).
 *
 * Toda función nueva de las web apps se anota AQUÍ en el mismo PR que la
 * introduce, con su estado en las apps nativas de escritorio y móvil. El test
 * `paridad.test.ts` valida las invariantes y rompe el push si una función de
 * web queda sin decisión en las nativas. Sin registro no hay paridad: se
 * promete de oído.
 *
 * LAS NATIVAS SON LA SUITE EN FYNE v2.8.1 (decidido el 2026-09-21; el port a
 * Gio queda cancelado y la web sigue en Astro+Preact+Go). Este registro dice lo
 * que las nativas HACEN, no lo que se espera que hagan.
 *
 * TODO lo que figuraba como hecho lo estaba en GIO, que se abandona, así que el
 * 2026-09-21 bajó a 'planificado'. Es la segunda vez que este registro cambia de
 * toolkit debajo; de ahí el tope de deuda que vigila el test.
 *
 * EL LISTÓN DE 'si' (decisión del dueño, 2026-10-01): **haberla visto funcionar en
 * una VENTANA REAL de la app de Fyne** —GL de verdad: Xvfb o sway en Linux con
 * `apps/fyne/scripts/ventana-real.sh`, o la ventana del Mac—, con el recorrido que
 * la ejerce y su captura. Que esté implementada y pase los tests del driver de
 * Fyne con el pintor de software NO basta: eso es 'planificado' con el código hecho,
 * y se dice en el comentario de la entrada. En el móvil, lo mismo en un teléfono o
 * en su emulador. (Los marcadores `paridad-web:` de pantallas y piezas son otra
 * puerta, la del §13.7, y siguen la regla del código.)
 *
 * Estados:
 *   'si'          — vista funcionar en una ventana real de esa superficie (arriba).
 *   'planificado' — aceptada como deuda CON `deuda` apuntando a la entrada de
 *                   roadmap que la cierra. Único estado distinto de 'si' que
 *                   deja pasar el test.
 *   'no'          — PROHIBIDO para algo que existe en web: el test falla.
 */

/** Superficies nativas del producto. */
export type EstadoParidad = 'si' | 'planificado';

/**
 * Claves de producto válidas. Lista CERRADA espejo del catálogo de
 * `packages/ui/src/lib/apps.ts` — vive aquí y no como import porque ui depende
 * de config y no al revés. `apps.test.ts` (en ui) ya ata el catálogo real;
 * si se añade un producto ahí, se añade aquí.
 */
export const PRODUCTOS_PARIDAD: ReadonlySet<string> = new Set([
  'one',
  'mail',
  'drive',
  'calendar',
  'contacts',
  'docs',
  'notes',
  'photos',
  'rooms',
  'cloud',
  'console',
  'sheets',
  'slides',
  'pdf',
  'erp',
  'cast',
  'pages',
  'bases',
  'suite',
]);

export interface FuncionParidad {
  /** Identificador estable (kebab-case). No se reusa ni se renombra. */
  id: string;
  /** Nombre legible de la función. */
  nombre: string;
  /** Clave de producto del catálogo (`apps.ts`) o 'suite' si es transversal. */
  producto: string;
  /** ¿Existe en las web apps? El registro existe para gobernar esto. */
  web: boolean;
  /** Estado en las apps de escritorio nativas (las de Fyne, `apps/fyne/cmd`). */
  escritorio: EstadoParidad;
  /**
   * Estado en las apps móviles nativas (las mismas de Fyne, empaquetadas con
   * `fyne package`; la suite Capacitor se rige por la web). 'si' es para lo que se ha
   * VISTO funcionar en un teléfono o en su emulador; lo que el escritorio tiene y en el
   * teléfono nadie ha abierto sigue en 'planificado', aunque sea el mismo código.
   */
  movil: EstadoParidad;
  /**
   * Obligatoria con 'planificado': dónde está anotada la deuda (roadmap.md §,
   * incidencia). El test valida que no quede vacía.
   */
  deuda?: string;
}

/**
 * TOPE DE DEUDA — NORMATIVO (CONVENTIONS.md §13.5).
 *
 * Cuántas funciones de web pueden estar SIN portar a las nativas. El test falla
 * si se supera, así que añadir una función de web sin su versión nativa obliga
 * a subir este número A MANO, en el mismo PR, y eso se ve en la revisión.
 *
 * Por qué hace falta: hasta hoy el gate solo exigía DECLARAR el estado, y
 * 'planificado' con una deuda apuntando al roadmap valía indefinidamente. Se
 * podía prometer paridad para siempre sin entregarla nunca, que es exactamente
 * lo que pasó: 35 de 35 funciones acabaron en 'planificado' en móvil.
 *
 * El número BAJA cuando se porta algo. No se baja solo: el test también avisa
 * si sobra margen, para que cerrar deuda quede registrado en vez de dejar hueco
 * para la siguiente.
 *
 * 2026-09-21: 31 y 31 — las 31 funciones de web, todas sin portar. Es el punto
 * de partida del port a Fyne, con todo por hacer. Debe bajar, nunca subir.
 * (Las otras 4 entradas del registro solo existen en nativas y no cuentan.)
 *
 * 2026-09-27: 43 y 43 — Sytue Cast llega con sus doce funciones de web y la app
 * de Fyne aún por hacer (CAST-ENCARGO §10 y §11). Baja cuando fyne-cast las porte.
 *
 * 2026-09-28: 44 y 44 — «Subir a Cast» desde Drive llega a la web; la parte de
 * Fyne la lleva el mismo port de Cast y baja con él.
 *
 * 2026-09-28: 31 y 44 — la app de Fyne de Sytue Cast porta sus doce funciones y
 * «Subir a Cast» del Drive nativo. Móvil sigue igual.
 *
 * 2026-09-28: 31 y 45 — elegir las calidades con el espacio estimado
 * (`cast-calidades`, decisión del dueño) llega a la web y al escritorio a la vez;
 * el móvil de Cast entero está por hacer y esta función va con él.
 *
 * 2026-09-28: 31 y 41 — Sytue Cast en el teléfono: subir desde la galería, Explorar, el
 * canal y compartir, vistos funcionar en el emulador de Android (API 36) contra el
 * servicio falso de las pruebas. Ver un vídeo funciona allí también (tocar, pantalla
 * completa al tumbarlo, pausa en segundo plano) pero su fila nombra la imagen dentro de
 * imagen, que en el teléfono no existe todavía. Lo demás de Cast espera a verse contra el
 * servicio desplegado (roadmap § Sytue Cast en el teléfono).
 *
 * 2026-09-28: 31 y 40 — «Ver un vídeo» en el teléfono con la imagen dentro de imagen de
 * Android (supportsPictureInPicture y enterPictureInPictureMode), vista en el emulador.
 *
 * 2026-09-29: 45 y 54 — Sytue Web llega a la web con catorce funciones
 * (sitios, crear, editor, idiomas, publicar, vista previa, versiones, dominio
 * propio, tienda, pedidos, formularios, analitica, exportar, ia); las nativas
 * van planificadas (decisión 9).
 *
 * 2026-10-02: −14 y −14 (de 48 y 59 a 34 y 45 ese día) — Sytue Web pasa a solo web (dueño): sus catorce funciones salen
 * del registro, que solo cuenta lo que las nativas deben llegar a tener.
 */
// 2026-10-01: +2 por el diario de Notes; código Fyne hecho, falta recorrido real.
// 2026-10-02: +1 por la sintaxis de captura y los atajos del diario (al estilo Twos);
// código Fyne y capturas del driver hechos, falta verla en una ventana real.
// 2026-10-02: +2 por las tareas de Notes (vista global; cancelar, posponer y trasladar).
// 2026-10-02: +1 en móvil por los recordatorios de Notes; el escritorio, visto en ventana.
// 2026-10-01: móvil +2 por el Archivo y las Fijadas de Notes; el escritorio ya las tiene.
// 2026-10-02: +3 por Semana/Días, «Este día» y la agenda de Calendar en el diario de
// Notes; el código de Fyne va en la misma PR, falta verlo en una ventana real.
// 2026-10-02: +1 por la captura y lectura sin conexión de Notes; código Fyne y tests
// del driver hechos, falta recorrerla en una ventana real contra el servicio.
// 2026-10-02: móvil +1 por el editor al estilo Twos de Notes (escritorio visto en ventana real).
// 2026-10-02: −14 en las dos por Sytue Web (solo web); ver más arriba.
// 2026-10-04: +1 en las dos por las filas justificadas y la jerarquía de mandos de
// Sytue Photos; el código de Fyne y sus tests del driver ya están, falta la ventana real.
// 2026-10-04: escritorio −1 y móvil +3 por Office en Fyne visto en ventana real (Xvfb, GL
// de Mesa, puntero de verdad): el modo presentación con su moderador sale de la deuda de
// escritorio, y «Presentar con Cast», la galería de plantillas y el grupo Tema con la
// marca entran ya vistos en escritorio y pendientes en móvil.
// 2026-10-04: móvil +1 por el Administrador de nombres de Calcus (escritorio visto en ventana real).
// 2026-10-05: móvil +1 por los bloques reutilizables de Office (escritorio visto en ventana real).
// 2026-10-05: móvil +1 por la forma Carril (menú lateral de color y barra clara con esquina
// de marca); el escritorio, visto en ventana real (Xvfb, GL de Mesa) en Mail, Drive y Hub,
// en claro, oscuro y traslúcido.
// 2026-10-05: +2 en las dos por los otros dos ejes del rediseño, que la web ya tiene: el
// color de la suite (petróleo, cobalto y el de cada app, con su selector) —el código de
// Fyne y sus tests del driver ya están, falta la ventana real— y la forma Marco, que en
// nativo aún no existe (las nativas pintan Carril).
// 2026-10-05: escritorio −1 por el color de la suite, visto en ventana real (Xvfb, GL de
// Mesa) en Mail: azul cobalto y «el de cada app», en claro, oscuro y traslúcido, con su
// selector en el menú de cuenta y la barra estrecha de color a 360.
// 2026-10-05: escritorio −1 por la forma Marco, que entra de golpe y por defecto en las
// catorce apps de Fyne, vistas en ventana real a 1280 y a 360 y en los tres modos.
// 2026-10-05: +1 en las dos por el punto de venta del ERP. NO es deuda nueva: está en la
// 2026-10-16: +1 en las dos por la navegación del visor de Sytue Photos (flechas de 48 con
// filete, franja de objetivo con ratón, deslizar con el dedo y precarga de las vecinas): el
// código de Fyne y sus tests del driver ya están, falta verlo en una ventana real.
// The external-storage wizard and Rooms OAuth connections are web first (+2):
export const TOPE_DEUDA = { escritorio: 45, movil: 67 } as const;

/**
 * DÓNDE SE CIERRA LA DEUDA. Una sola sección y no tres cadenas: las tres que
 * había —«Gio — móvil nativo en Gio: primera build verificada», «Gio — lo que
 * hacía la shell de Fyne y el escritorio en Gio aún no» y su concatenación—
 * apuntaban a un puerto CANCELADO y, comprobado el 2026-09-21, a secciones que
 * NUNCA existieron con ese título en roadmap.md. Pasaron porque el gate solo
 * miraba que `deuda` no estuviera vacía. Ahora `paridad.test.ts` exige que lo
 * que va tras `§` sea un encabezado real del roadmap.
 *
 * Sin el nombre del toolkit dentro, a propósito: ha cambiado dos veces en un
 * mes y estos gates sobrevivieron precisamente por no nombrarlo.
 *
 * Si una entrada necesita precisar, la prosa va DELANTE y la referencia al
 * final: `Lo que sea — ${ROADMAP_NATIVAS}`.
 */
const ROADMAP_NATIVAS =
  'docs/roadmap.md § Apps nativas sin webview — y paridad de superficies (decisión 2026-09-17)';
const DEUDA_AMBAS = `escritorio y móvil — ${ROADMAP_NATIVAS}`;
const DEUDA_MOVIL = `móvil — ${ROADMAP_NATIVAS}`;

export const FUNCIONES_PARIDAD: FuncionParidad[] = [
  // --- Transversal (suite) -------------------------------------------------
  {
    id: 'shell-topbar',
    nombre: 'TopBar global (buscador, app switcher, campanita, cuenta)',
    producto: 'suite',
    web: true,
    // Desde el 2026-09-21 la TopBar es SOLO de la web (encargo v4 §3): en
    // nativo hay un cromo propio —el de la suite en Gio, retirada el 2026-09-23— con
    // campana, cuenta, «Salir» y el lanzador a un botón, y el conmutador de
    // productos son atajos de teclado (⌘/Ctrl+0…9) y, en el móvil, el del
    // propio sistema. Eso NO es deuda: es el diseño, y el gate lo vigila con
    // la regla `topbar-solo-web`. Lo que sí falta en las tres superficies de
    // Gio es el BUSCADOR federado, y por eso la fila sigue en 'planificado'.
    escritorio: 'planificado',
    movil: 'planificado',
    deuda: DEUDA_AMBAS,
  },
  {
    id: 'suite-forma-carril',
    nombre:
      'Forma Carril: menú lateral de color con señal ámbar, barra superior clara con la esquina de marca y tarjetas blancas',
    producto: 'suite',
    web: true,
    // En Fyne: `apps/nucleo/tokens` lee appearance.css encima de colors.css, así que el
    // carril sale de la MISMA hoja que en la web; `ui.Lateral` y las piezas propias de
    // cada app piden los tokens `sidebar-*` por su nombre (no hay cascada), `ui.BarraSuite`
    // pinta la barra clara con la esquina de marca y `ui.Tarjeta` es la superficie blanca.
    // Guardianes del driver en `apps/fyne/ui` (píxeles y contraste en los tres modos).
    // Escritorio: vista en ventana real el 2026-10-05 (`ventana-real.sh`, Xvfb con el GL
    // de Mesa) en Mail, Drive y Hub, a 1280 y a 360, en claro, oscuro y traslúcido. En el
    // teléfono el menú es el cajón, que pinta el mismo `ui.Lateral`, y la barra estrecha
    // es carril entera (<768 px), con sus tests del driver; falta verlo en un teléfono.
    escritorio: 'si',
    movil: 'planificado',
    deuda: DEUDA_MOVIL,
  },
  {
    id: 'suite-color',
    nombre:
      'Color de la suite: petróleo, azul cobalto o el de cada app, con su selector en el menú de cuenta',
    producto: 'suite',
    web: true,
    // ADR 0071. En Fyne: `apps/nucleo/tokens` lee los bloques `[data-color]` (y el del
    // producto, `[data-app]`) de las MISMAS hojas, con su cascada; el color viaja por los
    // ajustes compartidos (`ajustes.json`, campo `color`) y el menú de cuenta lo elige
    // junto a la apariencia, con las claves de la web. El color por defecto de la
    // organización llega en el id_token (`org_color`) y lo guarda la sesión: persona →
    // organización → casa. Tests del driver
    // en `apps/nucleo/tokens`, `apps/fyne/sesion` y `apps/fyne/ui`. Escritorio: visto en
    // ventana real el 2026-10-05 (`ventana-real.sh --color`, Xvfb con el GL de Mesa) en
    // Mail, en cobalto y en «el de cada app», en los tres modos y con el selector abierto.
    // Falta verlo en un teléfono.
    escritorio: 'si',
    movil: 'planificado',
    deuda: DEUDA_MOVIL,
  },
  {
    id: 'suite-forma-marco',
    nombre:
      'Forma Marco (por defecto): barra y menú en un marco de color continuo, contenido en una hoja redondeada que flota, botones y navegación en píldora y tarjetas de color pleno',
    producto: 'suite',
    web: true,
    // En Fyne es también la forma por defecto (dueño, 2026-10-05: de golpe). Las medidas
    // salen de una copia embebida de theme.css y lo que pinta, de appearance.css
    // (`apps/nucleo/tokens`, comparadas con las de la web por test); la preferencia va en
    // `ajustes.json` (`forma`), se elige en el menú de cuenta y, sin elegir, vale la de la
    // organización (`org_forma` del id_token). La hoja se pinta una vez, en
    // `apps/fyne/ui/marco.go`: Fyne no recorta a esquinas redondeadas, así que son el
    // marco detrás, el papel, cuatro máscaras de esquina y la sombra en teselas. A sangre
    // los editores y todo por debajo de 768; el ERP, con hoja y con su carril de iconos
    // en el marco, como en la web. CONVENTIONS §13.6. Las tarjetas de recurso de color
    // pleno son las carpetas de la cuadrícula de Drive. El Hub es en Marco otra portada,
    // «Bienvenida y tablero», como en la web: el saludo por la hora, los accesos rápidos en
    // píldoras con un «Más», las apps en teselas compactas y «Hoy» y «Reciente» al lado
    // desde 1024 (`apps/fyne/apps/hub/lanzador.go` y `pildoras.go`).
    // Escritorio: visto en ventana real el 2026-10-05 (`ventana-real.sh`, Xvfb con el GL
    // de Mesa) en las catorce apps, a 1280 y a 360 y en los tres modos; la portada del Hub,
    // con el menú de «Más» abierto. Falta verlo en un teléfono.
    escritorio: 'si',
    movil: 'planificado',
    deuda: DEUDA_MOVIL,
  },
  {
    id: 'notificaciones-so',
    nombre: 'Notificaciones del sistema operativo',
    producto: 'suite',
    web: false,
    // Lo hacía el shell de Wails (`beeep`), retirado el 2026-09-23; en Fyne es
    // `escritorio.Avisar` de apps/nucleo/sistema: avisa cuando LLEGA una notificación
    // del Hub, y pulsarlo lleva a su destino en Linux, macOS, Windows y Android. Con
    // la app cerrada: el Hub en la bandeja en escritorio y, en Android, su servicio en
    // primer plano (`escritorio.MantenerAvisos`, tipo specialUse). Los
    // avisos por acción local de la shell de Fyne («Archivo guardado») no se portan:
    // eso lo dice la interfaz. En 'planificado' hasta verlo en cada sistema.
    escritorio: 'planificado',
    movil: 'planificado',
    deuda: DEUDA_AMBAS,
  },
  {
    id: 'abrir-en-aplicacion',
    nombre: 'Deep links «abrir en la aplicación» (sytue-<producto>://)',
    producto: 'suite',
    web: false,
    // En Fyne: `apps/fyne/scripts/empaquetar.sh` registra el esquema de cada app
    // por sistema, el enlace llega a la instancia que ya está abierta
    // (`apps/fyne/sesion/instancia.go`) y el `open?url=<https>` de la web
    // (`openInApp.ts`) lo traduce `api.DesdeLaWeb` a la página que se miraba:
    // Docus, Calcus y Diapos abren su documento y Drive su carpeta. Sigue en
    // 'planificado' hasta verlo en una ventana de verdad de cada sistema.
    escritorio: 'planificado',
    movil: 'planificado',
    deuda: DEUDA_AMBAS,
  },
  {
    id: 'modo-offline-cola',
    nombre: 'Modo sin conexión con cola de sincronización',
    producto: 'suite',
    web: true,
    // Gio sabe si el SERVIDOR contesta (`offline.Monitor`) y lo enseña; cola de
    // mutaciones no tiene. La de Fyne era un simulacro sobre datos sembrados.
    escritorio: 'planificado',
    movil: 'planificado',
    deuda: DEUDA_AMBAS,
  },
  {
    id: 'campanita-hub',
    nombre: 'Avisos del Hub como notificaciones (sesión OIDC PKCE + sondeo)',
    producto: 'suite',
    web: true,
    // Nativo: login con el navegador (PKCE, redirect a loopback), campana en la
    // barra con sondeo del Hub cada minuto (lo era en Gio, retirado) y aviso por
    // el centro de notificaciones del SO. La diferencia honesta con la web: ahí
    // el aviso es instantáneo (SSE); en nativo llega con el reloj del sondeo.
    escritorio: 'planificado',
    movil: 'planificado',
    deuda: DEUDA_AMBAS,
  },

  // --- Mail ----------------------------------------------------------------
  {
    id: 'mail-bandeja-carpetas',
    nombre: 'Bandeja con carpetas',
    producto: 'mail',
    web: true,
    // Código y tests del driver hechos (auditoría del 2026-10-01); falta verla en una
    // ventana real para subir el escritorio a 'si'.
    escritorio: 'planificado',
    movil: 'planificado',
    deuda: DEUDA_AMBAS,
  },
  {
    id: 'mail-mensajes-backend',
    nombre: 'Lista, lectura y envío reales vía JMAP',
    producto: 'mail',
    web: true,
    // Código y tests del driver hechos (auditoría del 2026-10-01); falta verla en una
    // ventana real para subir el escritorio a 'si'.
    escritorio: 'planificado',
    movil: 'planificado',
    deuda: DEUDA_AMBAS,
  },
  {
    id: 'estados-vacios',
    nombre: 'Estados vacíos ilustrados (Tinta y papel)',
    producto: 'suite',
    web: true,
    escritorio: 'planificado',
    movil: 'planificado',
    deuda: DEUDA_AMBAS,
  },
  {
    id: 'erp-dashboard-resumen',
    nombre: 'Dashboard con las cifras reales del backend (resumen)',
    producto: 'erp',
    web: true,
    // El ERP de Gio es genérico sobre el catálogo del servicio —lista y ficha de
    // cualquier modelo—; tablero no tiene.
    escritorio: 'planificado',
    movil: 'planificado',
    deuda: DEUDA_AMBAS,
  },
  {
    id: 'erp-facturas-lista',
    nombre: 'Lista de facturas de cliente (CRUD genérico)',
    producto: 'erp',
    web: true,
    // La lista y la ficha, de este modelo y de los demás; de momento solo lectura.
    escritorio: 'planificado',
    movil: 'planificado',
    deuda: DEUDA_AMBAS,
  },
  {
    id: 'erp-punto-de-venta',
    nombre: 'Punto de venta (TPV): catálogo, cesta y cobro en efectivo o con tarjeta',
    producto: 'erp',
    web: true,
    // `PuntoVenta.tsx`, en la web desde el 2026-10-04. En Fyne no existe: el ERP nativo
    // no tiene esa vista, y por eso a su carril de iconos le falta el botón (`pos.title`,
    // entre Tareas y Banco). Estuvo sin apuntar porque el trinquete de
    // `paridad-pantallas.test.ts` deriva de las páginas `.astro` y de `packages/ui`, y
    // esta es una pantalla entera DENTRO de una app de una sola página.
    escritorio: 'planificado',
    movil: 'planificado',
    deuda: DEUDA_AMBAS,
  },
  {
    id: 'photos-subida',
    nombre: 'Subida de fotos (Drive + indexar)',
    producto: 'photos',
    web: true,
    // Fotos en Gio enseña, marca favoritas, descarga y manda a la papelera.
    escritorio: 'planificado',
    movil: 'planificado',
    deuda: DEUDA_AMBAS,
  },
  {
    id: 'photos-galeria-justificada',
    nombre: 'Galería en filas justificadas y jerarquía de mandos',
    producto: 'photos',
    web: true,
    // Cada foto con su proporción (`.filas`), la barra de selección como barra de
    // contexto, el visor en un solo tamaño de mando y el menú por foto de la papelera.
    // El código de Fyne está hecho (`photos.Filas`, la misma cuenta que flexbox, atada
    // a las medidas de Chromium) y pasa los tests del driver; FALTA verla en una
    // ventana real, así que se queda en 'planificado'. Dos diferencias conocidas con
    // la web: «Seleccionar el mes» conserva su forma anterior (en la web ya es el
    // `Button` de la casa) y las pestañas no suben a 44 px con el dedo.
    escritorio: 'planificado',
    movil: 'planificado',
    deuda: DEUDA_AMBAS,
  },
  {
    id: 'photos-visor-navegacion',
    nombre: 'Visor: anterior y siguiente a la vista, deslizar entre fotos y ampliar donde se toca',
    producto: 'photos',
    web: true,
    // Las flechas de 48 con el filete del tono `media`, la franja de objetivo pegada al
    // canto con ratón, deslizar con el dedo (la misma regla en `decidirDeslizamiento`) y
    // la anterior y la siguiente pedidas antes de verlas; el tamaño real abierto en el
    // punto pulsado y el doble toque. El pellizco no tiene pareja en Fyne: su driver móvil
    // no dice de qué dedo es cada arrastre. El código de Fyne está hecho y pasa los tests
    // del driver; falta verlo en una ventana real.
    escritorio: 'planificado',
    movil: 'planificado',
    deuda: DEUDA_AMBAS,
  },
  {
    id: 'photos-describir-ia',
    nombre: 'Descripción de fotos con IA (alt text)',
    producto: 'photos',
    web: true,
    // Sytue AI no está en Gio: se fue con `gui/ia.go`.
    escritorio: 'planificado',
    movil: 'planificado',
    deuda: DEUDA_AMBAS,
  },
  {
    id: 'mail-redaccion-offline',
    nombre: 'Redacción en cola sin conexión',
    producto: 'mail',
    web: true,
    // Cola no hay, tampoco en la web: lo que la web tiene es la copia del borrador en el
    // dispositivo. En escritorio, cerrar el redactor sin red pierde lo escrito.
    escritorio: 'planificado',
    movil: 'planificado',
    deuda: DEUDA_AMBAS,
  },
  {
    id: 'mail-cuerpo-html',
    nombre: 'Cuerpo HTML del correo con su maqueta y contenido remoto bloqueado',
    producto: 'mail',
    web: true,
    // La web lo pinta en un iframe aislado; Fyne no tiene motor de HTML y enseña el texto
    // con el aviso `reader.plainTextOnly`. Hace falta un pintor propio de HTML de correo.
    escritorio: 'planificado',
    movil: 'planificado',
    deuda: DEUDA_AMBAS,
  },
  {
    id: 'mail-buscador',
    nombre: 'Buscador de la lista (asunto/remitente)',
    producto: 'mail',
    web: true,
    // Código y tests del driver hechos (auditoría del 2026-10-01); falta verla en una
    // ventana real para subir el escritorio a 'si'.
    escritorio: 'planificado',
    movil: 'planificado',
    deuda: DEUDA_AMBAS,
  },

  // --- Drive ---------------------------------------------------------------
  {
    id: 'drive-almacenamiento-externo',
    nombre: 'Asistente de almacenamiento externo con consentimiento OAuth2 y acceso desde Hub',
    producto: 'drive',
    web: true,
    escritorio: 'planificado',
    movil: 'planificado',
    deuda: DEUDA_AMBAS,
  },
  {
    id: 'drive-subir-descargar',
    nombre: 'Subir y guardar en disco',
    producto: 'drive',
    web: true,
    escritorio: 'planificado',
    movil: 'planificado',
    deuda: DEUDA_AMBAS,
  },
  {
    id: 'drive-compartir',
    nombre: 'Compartir fichero',
    producto: 'drive',
    web: true,
    // Lo hacía la suite en Gio (retirada el 2026-09-23): el panel de enlaces,
    // crearlos y revocarlos, contra los mismos `/shares` del servicio que la web.
    escritorio: 'planificado',
    movil: 'planificado',
    deuda: DEUDA_AMBAS,
  },
  {
    id: 'drive-sistema-archivos',
    nombre: 'Drive en el sistema de archivos del SO',
    producto: 'drive',
    web: false,
    // El montaje FUSE de solo lectura se retiró con el escritorio de Wails
    // (2026-09-23) sin que nadie lo lanzara nunca desde la interfaz. En Fyne,
    // «Conectar Drive en este equipo» de Hub › Dispositivos: en Windows, archivos a
    // petición con caché sin conexión (Cloud Files, `apps/nucleo/nube`, verificado en
    // una VM, de ida y vuelta); en macOS y Linux, el montaje WebDAV del explorador
    // (`escritorio.MontarWebDAV`), sin caché: File Provider pide una cuenta de Apple
    // que firme (docs/roadmap.md, «La caché sin conexión que daba FUSE»). En
    // 'planificado' hasta verlo desde el Hub en cada sistema.
    escritorio: 'planificado',
    movil: 'planificado',
    deuda: `escritorio, y en móvil File Provider (iOS); SAF (Android) hecho sin ver en un teléfono — ${ROADMAP_NATIVAS}`,
  },

  // --- Calendar ------------------------------------------------------------
  {
    id: 'calendar-crear',
    nombre: 'Crear eventos',
    producto: 'calendar',
    web: true,
    // Código y tests del driver hechos (auditoría del 2026-10-01); falta verla en una
    // ventana real para subir el escritorio a 'si'.
    escritorio: 'planificado',
    movil: 'planificado',
    deuda: DEUDA_AMBAS,
  },
  {
    id: 'calendar-colores',
    nombre: 'Color por evento en la lista',
    producto: 'calendar',
    web: true,
    // Código y tests del driver hechos (auditoría del 2026-10-01); falta verla en una
    // ventana real para subir el escritorio a 'si'.
    escritorio: 'planificado',
    movil: 'planificado',
    deuda: DEUDA_AMBAS,
  },
  {
    id: 'calendar-so',
    nombre: 'Exportar al calendario del SO',
    producto: 'calendar',
    web: false,
    // El .ics hacia el calendario del sistema era de la shell de Fyne.
    escritorio: 'planificado',
    movil: 'planificado',
    deuda: `escritorio, y en móvil el calendario del sistema (EventKit) — ${ROADMAP_NATIVAS}`,
  },

  // --- Notepads ------------------------------------------------------------
  {
    id: 'notes-buscador',
    nombre: 'Buscador de notas',
    producto: 'notes',
    web: true,
    escritorio: 'planificado',
    movil: 'planificado',
    deuda: DEUDA_AMBAS,
  },
  {
    id: 'notes-eliminar',
    nombre: 'Eliminar nota',
    producto: 'notes',
    web: true,
    // Código y tests del driver hechos (auditoría del 2026-10-01); falta verla en una
    // ventana real para subir el escritorio a 'si'.
    escritorio: 'planificado',
    movil: 'planificado',
    deuda: DEUDA_AMBAS,
  },

  {
    id: 'notes-diario-captura',
    nombre: 'Diario, navegación semanal y captura rápida de notas y tareas',
    producto: 'notes',
    web: true,
    // Código compartido por dominio, SDK y API; pendiente de recorrido GL/teléfono.
    escritorio: 'planificado',
    movil: 'planificado',
    deuda: DEUDA_AMBAS,
  },
  {
    id: 'notes-editor-twos',
    nombre:
      'Editor al estilo Twos: comandos «/», burbuja de formato, buscar en la nota, listas plegables, «Ir a», resaltado de lo buscado y ficha de enlaces a notas',
    producto: 'notes',
    web: true,
    // Visto en una ventana real (Xvfb con GLFW y Mesa en el frontal, 2026-10-02).
    escritorio: 'si',
    movil: 'planificado',
    deuda: DEUDA_MOVIL,
  },
  {
    id: 'notes-diario-enfoques-plantillas',
    nombre: 'Pendientes, completadas, fijadas, progreso y plantillas de notas',
    producto: 'notes',
    web: true,
    escritorio: 'planificado',
    movil: 'planificado',
    deuda: DEUDA_AMBAS,
  },
  {
    id: 'notes-diario-atajos',
    nombre: 'Sintaxis de captura (- tarea, --- separador, dd, tt) y atajos ⌘D, ⌘⇧←/→ y ⌘K',
    producto: 'notes',
    web: true,
    // Código Fyne y capturas del driver hechos; falta verla en una ventana real.
    escritorio: 'planificado',
    movil: 'planificado',
    deuda: DEUDA_AMBAS,
  },
  {
    id: 'notes-tareas-vista',
    nombre: 'Vista «Tareas»: pendientes, completadas y canceladas de todos los cuadernos',
    producto: 'notes',
    web: true,
    // Código Fyne hecho (tareas.go); pendiente de recorrido en ventana real y teléfono.
    escritorio: 'planificado',
    movil: 'planificado',
    deuda: DEUDA_AMBAS,
  },
  {
    id: 'notes-tareas-cancelar-posponer',
    nombre: 'Cancelar, posponer y trasladar tareas con su subárbol; ocultar completadas',
    producto: 'notes',
    web: true,
    escritorio: 'planificado',
    movil: 'planificado',
    deuda: DEUDA_AMBAS,
  },
  {
    id: 'notes-recordatorios',
    nombre: 'Recordatorio por nota con repetición (diaria, laborables, semanal, mensual, flexible)',
    producto: 'notes',
    web: true,
    // Visto en una ventana real de escritorio (Xvfb) poniendo, cambiando y quitando uno.
    escritorio: 'si',
    movil: 'planificado',
    deuda: DEUDA_MOVIL,
  },
  {
    id: 'notes-archivo',
    nombre: 'Archivar y desarchivar cuadernos, vista Archivo y búsqueda con archivados',
    producto: 'notes',
    web: true,
    // Visto en una ventana real (Xvfb en el frontal) el 2026-10-01.
    escritorio: 'si',
    movil: 'planificado',
    deuda: DEUDA_MOVIL,
  },
  {
    id: 'notes-fijadas',
    nombre: 'Fijadas: las notas fijadas de todos los cuadernos en una vista',
    producto: 'notes',
    web: true,
    escritorio: 'si',
    movil: 'planificado',
    deuda: DEUDA_MOVIL,
  },
  {
    id: 'notes-diario-semana-dias',
    nombre: 'Vistas Semana y Días, puntos de la semana y soltar una nota sobre un día',
    producto: 'notes',
    web: true,
    // Código y pruebas del driver en apps/fyne/apps/notes/semana.go; falta la ventana real.
    escritorio: 'planificado',
    movil: 'planificado',
    deuda: DEUDA_AMBAS,
  },
  {
    id: 'notes-diario-este-dia',
    nombre: '«Este día»: el mismo día de años anteriores',
    producto: 'notes',
    web: true,
    escritorio: 'planificado',
    movil: 'planificado',
    deuda: DEUDA_AMBAS,
  },
  {
    id: 'notes-diario-agenda',
    nombre: 'Agenda del día de Sytue Calendar y crear un evento desde una nota',
    producto: 'notes',
    web: true,
    escritorio: 'planificado',
    movil: 'planificado',
    deuda: DEUDA_AMBAS,
  },
  {
    id: 'notes-sinconexion',
    nombre: 'Captura sin conexión con cola idempotente y lectura de lo ya abierto',
    producto: 'notes',
    web: true,
    // Cola, borradores persistidos y reintento en Fyne con tests del driver; falta
    // verla en una ventana real sin red y al volver.
    escritorio: 'planificado',
    movil: 'planificado',
    deuda: DEUDA_AMBAS,
  },
  {
    id: 'notes-cifrado-datos',
    nombre: 'Cifrado de contenido, adjuntos y datos auxiliares con migración CAS al desbloquear',
    producto: 'notes',
    web: true,
    // SDK y driver Fyne compartidos; la evidencia del PR separa esta QA de la prueba física.
    escritorio: 'si',
    movil: 'si',
  },
  // --- Rooms ---------------------------------------------------------------
  {
    id: 'rooms-conexiones-oauth',
    nombre: 'Conectar cuentas, compartir mensajes de canal y enlaces de Google Meet',
    producto: 'rooms',
    web: true,
    escritorio: 'planificado',
    movil: 'planificado',
    deuda: DEUDA_AMBAS,
  },
  {
    id: 'rooms-canales-enviar',
    nombre: 'Canales y envío de mensajes',
    producto: 'rooms',
    web: true,
    escritorio: 'planificado',
    movil: 'planificado',
    deuda: DEUDA_AMBAS,
  },
  {
    id: 'rooms-tiempo-real',
    nombre: 'Mensajes en tiempo real (WebSocket)',
    producto: 'rooms',
    web: true,
    // 2026-10-23: web y Fyne comparten la regla Hub rooms/rooms.message para los
    // avisos de directos. Web usa el SDK mediante BFF ligado a sub+tenant; permisos
    // del navegador y filtro local legado siguen separados y nunca se importan
    // automáticamente. Pruebas remotas de contrato/identidad/carreras, sin dar por
    // verificado un SO o dispositivo: ambos estados de esta fila se conservan.
    escritorio: 'planificado',
    movil: 'planificado',
    deuda: DEUDA_AMBAS,
  },

  // --- Sytue WorkHub -------------------------------------------------------
  // Docus, Calcus, Diapos y CertiPDF llegaron a Gio en la v4 y faltaban aquí:
  // el registro no gobierna lo que no nombra. Estado medido el 2026-09-21
  // leyendo las apps de Gio (retiradas el 2026-09-23), no de oído.
  {
    id: 'docus-documentos',
    nombre: 'Documentos: lista, crear, renombrar, borrar y destacar',
    producto: 'docs',
    web: true,
    escritorio: 'planificado',
    movil: 'planificado',
    deuda: DEUDA_AMBAS,
  },
  {
    id: 'docus-editar',
    nombre: 'Editar el documento con autoguardado e historial',
    producto: 'docs',
    web: true,
    // Edita por bloques y guarda con su contador (`SaveContent` con dot y
    // lamport), enseña el historial de versiones y restaura una.
    escritorio: 'planificado',
    movil: 'planificado',
    deuda: DEUDA_AMBAS,
  },
  {
    id: 'docus-comentarios',
    nombre: 'Comentarios por rango con hilos, menciones y resolución',
    producto: 'docs',
    web: true,
    // Gio los LEE en su panel; no crea, no responde y no resuelve.
    escritorio: 'planificado',
    movil: 'planificado',
    deuda: DEUDA_AMBAS,
  },
  {
    id: 'docus-coedicion',
    nombre: 'Coedición en tiempo real con presencia y cursores',
    producto: 'docs',
    web: true,
    // El guardado lleva su contador, pero no hay canal en vivo ni cursores de
    // nadie más: dos personas a la vez todavía no se ven.
    escritorio: 'planificado',
    movil: 'planificado',
    deuda: DEUDA_AMBAS,
  },
  {
    id: 'calcus-abrir-libro',
    nombre: 'Abrir un libro y calcular sus fórmulas',
    producto: 'sheets',
    web: true,
    // El motor es `internal/formula`, el MISMO que la web, comparado contra
    // sus trazas. Lista, libro, hojas y valores calculados.
    escritorio: 'planificado',
    movil: 'planificado',
    deuda: DEUDA_AMBAS,
  },
  {
    id: 'calcus-editar',
    nombre: 'Editar celdas, formatos y estructura del libro',
    producto: 'sheets',
    web: true,
    // SOLO LECTURA a propósito, y dicho en el propio paquete: guardar espera a
    // que el modelo vaya y vuelva sin pérdida y haya un test que lo demuestre
    // con un libro real. Un documento de la gente no se corrompe nunca.
    escritorio: 'planificado',
    movil: 'planificado',
    deuda: DEUDA_AMBAS,
  },
  {
    id: 'calcus-nombres-definidos',
    nombre: 'Administrador de nombres: rangos, valores y fórmulas con nombre, por libro o por hoja',
    producto: 'sheets',
    web: true,
    // Visto en una ventana real (Xvfb, 2026-10-04): panel con la lista, edición y borrado;
    // las fórmulas resuelven el nombre y dan #NAME? al quitarlo.
    escritorio: 'si',
    movil: 'planificado',
    deuda: DEUDA_MOVIL,
  },
  {
    id: 'diapos-editar',
    nombre: 'Presentaciones: lista, crear, editar diapositivas y reordenar',
    producto: 'slides',
    web: true,
    escritorio: 'planificado',
    movil: 'planificado',
    deuda: DEUDA_AMBAS,
  },
  {
    id: 'diapos-presentar',
    nombre: 'Modo presentación a pantalla completa',
    producto: 'slides',
    web: true,
    // Visto en una ventana real (Xvfb con GLFW y Mesa en el frontal, 2026-10-04):
    // presentación a pantalla completa, vista del moderador con notas y avance con flechas.
    escritorio: 'si',
    movil: 'planificado',
    deuda: DEUDA_MOVIL,
  },
  {
    id: 'diapos-presentar-cast',
    nombre: 'Presentar con Cast: programar el directo y avisar de la diapositiva en curso',
    producto: 'slides',
    web: true,
    // Visto en una ventana real (Xvfb, 2026-10-04): diálogo, directo programado y un PUT
    // de posición por diapositiva contra un Cast de pega; las notas no salen.
    escritorio: 'si',
    movil: 'planificado',
    deuda: DEUDA_MOVIL,
  },
  {
    id: 'docs-plantillas-galeria',
    nombre:
      'Galería de plantillas en «Crear nuevo»: las de la organización con versión y el tema del documento nuevo',
    producto: 'docs',
    web: true,
    // Visto en una ventana real (Xvfb, 2026-10-04) en las portadas de Diapos y Calcus. La
    // miniatura es el glifo del tipo, no el esquema dibujado de la web.
    escritorio: 'si',
    movil: 'planificado',
    deuda: DEUDA_MOVIL,
  },
  {
    id: 'docs-bloques-reutilizables',
    nombre: 'Bloques reutilizables de Sytue y de la organización, insertables desde la cinta',
    producto: 'docs',
    web: true,
    // Visto en una ventana real (Xvfb, 2026-10-05) en Docus, Diapos y Calcus: el
    // desplegable, el bloque insertado (HTML en el cursor, diapositiva tras la activa,
    // celdas en la celda activa con la fórmula desplazada) y el documento guardado.
    escritorio: 'si',
    movil: 'planificado',
    deuda: DEUDA_MOVIL,
  },
  {
    id: 'docs-tema-marca',
    nombre:
      'Grupo Tema: aplicar un tema común o la marca, «Actualizar a la marca actual» con previa y editar la marca',
    producto: 'docs',
    web: true,
    // Visto en una ventana real (Xvfb, 2026-10-04) en Diapos (Diseño) y Calcus (Inicio):
    // selector, diálogo de la marca y previa del servidor.
    escritorio: 'si',
    movil: 'planificado',
    deuda: DEUDA_MOVIL,
  },
  {
    id: 'certipdf-abrir',
    nombre: 'Abrir un PDF: texto, firmas declaradas, campos y adjuntos',
    producto: 'pdf',
    web: true,
    // Lista desde Drive, sube, renombra, borra, descarga y organiza páginas;
    // del documento enseña su estructura y el texto de cada página.
    escritorio: 'planificado',
    movil: 'planificado',
    deuda: DEUDA_AMBAS,
  },
  {
    id: 'certipdf-pagina-dibujada',
    nombre: 'Ver la página dibujada del PDF',
    producto: 'pdf',
    web: true,
    // Solo en el navegador: `rasterizarSVG` es nil fuera de js, así que en el
    // escritorio y en el móvil la sección «Página» no se ofrece —en vez de
    // ofrecerla vacía—. Se cierra el día que el servicio entregue píxeles.
    escritorio: 'planificado',
    movil: 'planificado',
    deuda: DEUDA_AMBAS,
  },
  {
    id: 'certipdf-firmar',
    nombre: 'Firmar y anotar el PDF',
    producto: 'pdf',
    web: true,
    // Piden pinchar sobre la página dibujada: Gio abre la suite actual para
    // eso y lo dice, en vez de fingir un lienzo que no tiene.
    escritorio: 'planificado',
    movil: 'planificado',
    deuda: DEUDA_AMBAS,
  },
  // --- Sytue Cast (2026-09-27) ---------------------------------------------
  // La web entera llegó en la misma ola que el servicio; la app de Fyne va detrás
  // (apps/fyne/apps/cast) y el integrador sube cada entrada a 'si' al verla funcionar.
  {
    id: 'cast-subida',
    nombre: 'Subir vídeos con cola y progreso, y medidor de cuota',
    producto: 'cast',
    web: true,
    escritorio: 'si',
    movil: 'si',
  },
  {
    id: 'cast-visibilidad-personas',
    nombre: 'Visibilidad del vídeo, con personas concretas por @usuario',
    producto: 'cast',
    web: true,
    escritorio: 'si',
    movil: 'planificado',
    deuda: DEUDA_MOVIL,
  },
  {
    id: 'cast-ver-vod',
    nombre:
      'Ver un vídeo bajo demanda (menú de calidad con «Automática», imagen dentro de imagen, teclado, pantalla completa)',
    producto: 'cast',
    web: true,
    // La web con los niveles del HLS (hls.js) y el escritorio con su escalera de WebM,
    // las mismas calidades; la imagen dentro de imagen, del navegador en la web y una
    // ventana propia siempre encima en el escritorio.
    escritorio: 'si',
    // En el teléfono: tocar, pantalla completa al tumbarlo, pausa al irse al fondo, y la
    // imagen dentro de imagen del sistema en Android (entrar con el botón o sola al salir,
    // reproducir y pausar desde la ventana, cerrarla pausa).
    movil: 'si',
  },
  {
    id: 'cast-ver-directo',
    nombre: 'Ver un directo con el contador de espectadores',
    producto: 'cast',
    web: true,
    escritorio: 'si',
    movil: 'planificado',
    deuda: DEUDA_MOVIL,
  },
  {
    id: 'cast-emitir-obs',
    nombre: 'Emitir desde OBS con la clave de emisión (mostrar, copiar, regenerar)',
    producto: 'cast',
    web: true,
    escritorio: 'si',
    movil: 'planificado',
    deuda: DEUDA_MOVIL,
  },
  {
    id: 'cast-emitir-navegador',
    nombre: 'Emitir desde la app con cámara, pantalla y micrófono',
    producto: 'cast',
    web: true,
    escritorio: 'si',
    movil: 'planificado',
    deuda: DEUDA_MOVIL,
  },
  {
    id: 'cast-chat',
    nombre: 'Chat del directo, con moderación',
    producto: 'cast',
    web: true,
    escritorio: 'si',
    movil: 'planificado',
    deuda: DEUDA_MOVIL,
  },
  {
    id: 'cast-comentarios',
    nombre: 'Comentarios bajo el vídeo, con respuestas y moderación',
    producto: 'cast',
    web: true,
    escritorio: 'si',
    movil: 'planificado',
    deuda: DEUDA_MOVIL,
  },
  {
    id: 'cast-incrustar',
    nombre: 'Compartir el enlace y el código de inserción',
    producto: 'cast',
    web: true,
    escritorio: 'si',
    movil: 'si',
  },
  {
    id: 'cast-explorar',
    nombre: 'Portada pública: directos y vídeos públicos',
    producto: 'cast',
    web: true,
    escritorio: 'si',
    movil: 'si',
  },
  {
    id: 'cast-canal',
    nombre: 'Canal público de una organización',
    producto: 'cast',
    web: true,
    escritorio: 'si',
    movil: 'si',
  },
  {
    id: 'cast-gestionar',
    nombre: 'Gestionar un vídeo: datos, póster, interruptores y borrar',
    producto: 'cast',
    web: true,
    escritorio: 'si',
    movil: 'planificado',
    deuda: DEUDA_MOVIL,
  },
  // Decisión del dueño (2026-09-28): quien sube elige las calidades, viendo antes lo
  // que ocupa cada una; se aplican a la escalera HLS de la web y a la WebM del
  // escritorio, y la estimación es UNA, la del servicio (GET /videos:estimate).
  {
    id: 'cast-calidades',
    nombre:
      'Elegir las calidades al subir, desde Drive y en Gestionar, con el espacio estimado y la recomendada',
    producto: 'cast',
    web: true,
    escritorio: 'si',
    movil: 'planificado',
    deuda: DEUDA_MOVIL,
  },
  // «Subir a Cast» desde Drive (decisión del dueño, 2026-09-28): una COPIA del vídeo
  // en Cast desde el menú de un fichero de Drive. La parte nativa (el Drive de Fyne
  // y el cliente de Cast en apps/nucleo) la hace quien porta Cast a Fyne; sube a
  // 'si' cuando se fusione.
  {
    id: 'drive-subir-a-cast',
    nombre: 'Subir a Sytue Cast un vídeo de Drive (copia, con título y visibilidad)',
    producto: 'drive',
    web: true,
    escritorio: 'si',
    movil: 'planificado',
    deuda: DEUDA_MOVIL,
  },
  // Sytue Web (14 funciones: sitios, crear, editor, idiomas, publicar, vista previa,
  // versiones, dominio propio, tienda, pedidos, formularios, analítica, exportar e IA)
  // NO figura aquí: es solo web por ahora (dueño, 2026-10-02) y lo que las
  // nativas no llevan por decisión no es deuda. Sus pantallas están en SOLO_WEB de paridad-pantallas.test.ts.
];
