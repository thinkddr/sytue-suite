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
// 2026-10-02: +1 por la sintaxis de captura y los atajos del diario (diario rápido);
// código Fyne y capturas del driver hechos, falta verla en una ventana real.
// 2026-10-02: +2 por las tareas de Notes (vista global; cancelar, posponer y trasladar).
// 2026-10-02: +1 en móvil por los recordatorios de Notes; el escritorio, visto en ventana.
// 2026-10-01: móvil +2 por el Archivo y las Fijadas de Notes; el escritorio ya las tiene.
// 2026-10-02: +3 por Semana/Días, «Este día» y la agenda de Calendar en el diario de
// Notes; el código de Fyne va en la misma PR, falta verlo en una ventana real.
// 2026-10-02: +1 por la captura y lectura sin conexión de Notes; código Fyne y tests
// del driver hechos, falta recorrerla en una ventana real contra el servicio.
// 2026-10-02: móvil +1 por el editor rápido de Notes (escritorio visto en ventana real).
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
// 2026-11-05: +1 en las dos por la papelera en lote de Drive (deshacer una selección y
// vaciarla);
// el código de Fyne y sus tests del driver ya están, falta verla en una ventana real.
// 2026-11-05: +1 en las dos por las acciones rápidas de Contacts en la lista y la ficha
// (deshacer el envío a la papelera, destacar desde la fila, correo y .vcf de la selección, copiar desde la ficha); el código de Fyne y sus tests del driver ya
// están, falta verlas en una ventana real.
// 2026-11-05: +3 en las dos por Notepads: la libreta sobre papel (cromo neutro, líneas con su
// marca, cabecera del día y glifos de una tinta), pegar varias líneas en la captura y
// convertir una línea en tarea o en texto, y `[[` y `@` en el editor; el código de Fyne,
// sus tests del driver y las capturas del pintor de software ya están, falta verlo en una
// ventana real y en un teléfono.
// 2026-11-06: +1 en las dos por las cuentas externas de Mail (Gmail, Outlook, Yahoo,
// iCloud, IMAP y POP3, con sus avisos y errores), que llegan a la web sin entrada en el
// registro; el código de Fyne y sus tests del driver ya están, falta verlas en una
// ventana real.
export const TOPE_DEUDA = { escritorio: 51, movil: 73 } as const;

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
    // 2026-11-05: «Cargar más resultados» en `/buscar` cuando el Hub dice que quedan, la
    // página siguiente por `offset` y sin repetir, en web (`unirPaginas`, `?paginas=`) y en
    // la pantalla Buscar del Hub de Fyne (`UnirBusqueda`, `api.Hub.SearchDesde`). Código y
    // tests del driver hechos; sin ventana real aún.
    // 2026-11-06: aterriza en el primer resultado nuevo también en Fyne (foco y a la vista),
    // avisa si una página falla o llega vacía (`search.loadMoreFailed`) y, sin más en el
    // Hub, el «N de M» es lo enseñado (`totalAMostrar` / `TotalAMostrar`). Sin ventana real.
    // 2026-11-07: BARRA ÚNICA en el teléfono: la topbar acoge la barra de la app (`BarraMovil`
    // de @sytue/ui, portada desde AppShell, AdminShell, Rooms y ERP) y pliega la suite tras
    // el avatar; en Fyne, `ui.BarraSuite` sube la `ui.BarraMovil` del armazón a su fila y
    // despliega marca, buscador y campana con un ✕. Tests en las dos (happy-dom y driver);
    // en Fyne sin teléfono real.
    // 2026-11-07 (pulido): título en la misma x con ☰ o sin él (la marca del producto en su
    // sitio), nombre de MARCA (`nombreCorto` / `NombreCorto`, el del FyneApp.toml; test
    // `TestElNombreDeCadaProductoEsElDeLaWebYElDeSuToml`) sin repetir la cabecera de debajo
    // (`repiteLaBarra` / `CabeceraApp.RepiteLaBarra`), sin franjas claras (banco
    // `test/e2e/barra-unica.mjs` y `TestLaBarraUnicaNoDejaFranjasClaras`), y en Fyne la
    // cara de la suite sin selector de apps (`TestFyneNoTieneSelectorDeApps`).
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
    // 2026-11-05: cada evento de «Hoy» es un enlace que abre ESE evento en Calendar (la
    // vista de día en la fecha de la instancia), en web (`rutaDelEvento`) y Fyne
    // (`RutaDelEvento`, `FilaEvento` pulsable). Tests del driver hechos; esta parte aún
    // sin verla en ventana real.
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
    // 2026-11-05: en la bandeja del Hub el punto azul de un aviso sin leer es un botón
    // «Marcar como leído» que marca ese solo, sin abrirlo: web (`MarcarLeido` de
    // @sytue/shell) y Fyne (`BotonLeido`, `Vista.marcarLeido`). Sin ventana real aún.
    // 2026-11-06: con «Sin leer» el aviso marcado sale de la lista, y el foco pasa a la fila
    // del siguiente (o del anterior si era el último), en web y Fyne (`devolverFoco`).
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
    // 2026-11-05: «Deshacer» en el aviso del triaje (archivar, papelera, spam, mover;
    // fila, lector, teclas y lote), cada mensaje a su carpeta de origen, en web y en
    // Fyne (`origenesDeMovimiento` en las dos, con sus tests). Sin ventana real aún.
    // Diferencia a propósito, que no se ve: deshacer durante una búsqueda, la web
    // vuelve a pedir la búsqueda y la carpeta de debajo en el acto; Fyne relanza la
    // búsqueda y marca la carpeta (`listaCaducada`) para pedirla al salir de ella.
    // Al salir de la búsqueda las dos listas quedan iguales.
    // 2026-11-05: filtros rápidos de la lista (no leídos, destacados, con adjuntos) en
    // web y Fyne (`filtrarConversaciones` / `FiltrarConversaciones`). Sin ventana real.
    // 2026-11-06: el tope de páginas automáticas del filtro vacío no cuenta los
    // repintados con una página en camino, en las dos (`pasoDelFiltroVacio` /
    // `pintarLista`).
    // 2026-11-05: atajos x (marcar), Esc (vaciar selección), l (Etiquetar) y v (Mover
    // a…) en web y Fyne, y en la hoja de «?». Sin ventana real.
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
    // 2026-11-05: «Compartir en Rooms» desde el lector — web por el intent
    // share-to-chat del Hub, Fyne por el esquema de Rooms (`api.RutaCompartir`, como
    // Drive nativo); el mismo texto (`textoParaRooms`). Sin ventana real aún.
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
    // 2026-11-05, en las dos superficies (`lib/importacion.ts`; en Fyne,
    // `apps/erp/importacion.go`, mismos nombres):
    //   · importar CSV con vista previa simulada y «Deshacer» del lote en el aviso.
    //   · libro diario y sumas y saldos por período (`components/periodo.ts`; en Fyne,
    //     `apps/erp/periodo.go`), con el selector junto al título.
    //   · «Deshacer» en el aviso de «Movido a la papelera», de la ficha y del lote
    //     (`lib/papelera.ts`; en Fyne, `apps/erp/papelera.go`).
    //   · el estado vacío propio de la Papelera (`components/vacio.ts`; en Fyne,
    //     `vacioDeLista` en `apps/erp/lista.go`).
    // Código de Fyne y tests del driver hechos; falta verlo en una ventana real.
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
    // La barra de selección lleva el corazón: «Añadir a favoritas» si falta alguna (solo
    // pide las que cambian) o «Quitar de favoritas», lleno, si todas lo son. Web
    // (`favoritasEnLote.ts`) y Fyne (`planFavoritas`), con tests en las dos.
    // Una búsqueda sin aciertos es un estado vacío: «Ninguna foto coincide con…», con
    // «Borrar búsqueda» (devuelve el foco al buscador) y ampliar mientras se pueda. Web
    // (`sinResultados.ts`) y Fyne (`sinResultados`, `vacioDeBusqueda`), con tests.
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
    // En el visor de un álbum, «Usar como portada del álbum» (pulsado en la que ya lo es)
    // junto a «Quitar del álbum», y «Renombrar» en la cabecera del álbum: web
    // (AlbumView.tsx) y Fyne (`ponerPortada`, `renombrarAlbum`), con tests en las dos.
    // La cabecera del visor dice «12 / 240» («12 / 100+» con páginas por pedir) tras el
    // nombre, e Inicio y Fin saltan a la primera y a la última cargada: web
    // (`posicionVisor.ts`) y Fyne (`posicionVisor`, `saltoVisor`), con tests en las dos.
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

  {
    id: 'mail-cuentas-externas',
    nombre:
      'Cuentas externas en Mail (Gmail, Outlook, Yahoo, iCloud, IMAP y POP3) con sus avisos y errores',
    producto: 'mail',
    web: true,
    // 2026-11-06: «Añadir cuenta» con OAuth (Gmail, Outlook, Yahoo), iCloud como preajuste
    // de IMAP con contraseña de app, e IMAP/POP3 a mano; sin filas fantasma ni repetidas
    // («ya conectada» abre la que hay y lleva el foco a ella), errores que dicen qué pasó
    // (cancelado, sin permiso de correo, caducado) y solo las cuentas con buzón en Mail.
    // Web y Fyne (`correo_externo.go`, con sus tests del driver). Sin ventana real aún.
    // En Fyne el inicio de sesión espera al navegador 10 min (no los 45 s de una llamada),
    // con «Cancelar» y el aspa activos mientras; en la web la página entera va al proveedor.
    // Las carpetas de la cuenta salen traducidas, con glifo y en el orden del lateral, con
    // vectores comunes (`apps/fyne/apps/mail/testdata/carpetas-externas.json`).
    // El lateral: la sección «Cuentas de correo externas» va siempre y la primera, encima
    // de «Carpetas», con «Añadir cuenta de correo» al final (web `ExternalAccountsSection`,
    // Fyne `seccionExternas`; el alta ya no está en «Ajustes» de Fyne). Se actualiza al
    // volver del proveedor y al conectar o desconectar, con el diálogo abierto, y si las
    // cuentas no cargan lo dice con «Reintentar».
    // iCloud: enlace a account.apple.com para la contraseña de app, aviso propio si el LOGIN
    // falla (`externalMail.icloudAuthError`), «iCloud Mail» en la lista y la nube en el
    // lateral; las carpetas de iCloud llegan con su rol por nombre o SPECIAL-USE.
    // Fase B (estilo Apple Mail), en curso: el modelo puro del lateral (`arbolDeBuzones`:
    // buzones unificados por rol y una sección por cuenta con estado y no leídos) en web
    // (`components/buzones.ts`) y Fyne (`apps/mail/buzones.go`) con vectores comunes
    // (`apps/fyne/apps/mail/testdata/arbol-de-buzones.json`), y el lateral lee
    // `GET /external-mail/summary` (carpetas y no leídos de cada cuenta en una petición).
    // El lateral ya lo dibuja (web `MailboxesSection`/`ExternalAccountsSection`, Fyne
    // `seccionBuzones`/`seccionExternas`): «Buzones» arriba con Todos los buzones de
    // entrada, Enviados… de todas las cuentas y Sytue, cada uno desplegable en la carpeta
    // de cada cuenta, y cada cuenta como rama plegable con sus carpetas traducidas, glifo
    // y contador, o «Requiere volver a iniciar sesión» / reintentar en su sitio. La rama
    // es `SidebarItem expanded` en @sytue/ui.
    // Una carpeta externa se abre en el panel principal, con la lista y el lector de
    // siempre (web `selectedExternal` + `vistaExterna.ts`, Fyne `Buzon.externa` +
    // `vista_externa.go`): leer, destacar, archivar, tirar, spam y «Mover a…» van al
    // proveedor de su cuenta; responder y «Redactar» con su carpeta salen de ella, y una
    // franja dice si hay que volver a
    // iniciar sesión. «Todos los buzones de entrada» abre la Entrada de todas mezclada por
    // fecha, con vectores comunes (`testdata/mezcla-de-entradas.json`); su triángulo va
    // aparte (`SidebarItem onToggle`). El mini-buzón del diálogo se retiró. Un POP3
    // mezclado ahí se lee sin triaje (`sinAccionesExternas` / `Buzon.sinAcciones`), y
    // desconectar la cuenta a la vista vuelve a la Entrada de Sytue.
    // El «De» del redactor de Sytue lista la principal, los alias y los compartidos y,
    // detrás, cada cuenta externa que envía (`remitentes`, web `components/remitentes.ts`
    // y Fyne `apps/mail/remitentes.go`, vectores en `testdata/remitentes.json`); con una
    // externa elegida el correo sale por `/external-mail/{id}/send` con
    // `expected_address`, sin deshacer ni «Programar», y las imágenes del texto van como
    // adjuntos. Responder y «Redactar» desde una cuenta la preseleccionan; el redactor
    // propio de la cuenta se retiró.
    // Al terminar el alta la hoja se cierra, la Entrada de la cuenta queda elegida con su
    // rama desplegada y un aviso dice qué cuenta se conectó (web `onOpenAccount(…, true)`
    // + toast y `reveal` de `ExternalAccountsSection`; Fyne `cargarCuentas` + `Avisar`).
    // «Todos los enviados» y «Todos los borradores» se abren también mezclados por fecha,
    // con su triángulo aparte (`ROLES_MEZCLADOS` / `rolesMezclados`, vectores en
    // `testdata/buzones-mezclados.json`); en «Todos los borradores» el de Sytue reabre el
    // redactor y el de una cuenta se lee. Archivo, Spam y Papelera solo se despliegan. El
    // refresco de 30 s pide solo las carpetas del buzón a la vista.
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
    // 2026-11-05: «Subir» flotante en el teléfono (`MobileFab`; en Fyne,
    // `BotonFlotante` en la `CapaFlotante` de la pantalla), en «Mi unidad» y con red.
    // Código de Fyne y tests del driver hechos; falta verlo en un teléfono.
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
    // 2026-11-05: «Copiar enlace» en el «⋯», detrás de «Compartir enlace»: el enlace
    // interno (`enlaceInterno` en la web, `enlaceDe` en Fyne), también sin red. Código
    // de Fyne y tests del driver hechos; falta verlo en una ventana real.
    // 2026-11-06: también en el «⋯» de Recientes y Destacados, que en la web es el
    // mismo `ActionsMenu` (en Fyne, `accionesPlano`).
    escritorio: 'planificado',
    movil: 'planificado',
    deuda: DEUDA_AMBAS,
  },
  // La papelera en lote (2026-11-05): «Deshacer» en el aviso de «N elementos enviados
  // a la papelera», con los 10 s del de un solo elemento, que restaura cada uno por su
  // id en tandas de cuatro (`lib/lote.ts`; en Fyne, `apps/drive/lote.go`) y dice
  // cuántos no volvieron. Y «Vaciar papelera» con su confirmación, que purga cada
  // elemento por su id con el mismo `enTandas`/`EnTandas`. Código de Fyne y tests del
  // driver hechos; falta verla en una ventana real. 2026-11-06: el «Deshacer» recarga
  // lo que se ve al pulsarlo —otra carpeta, o la Papelera— y no lo de cuando se tiró.
  {
    id: 'drive-papelera-lote',
    nombre: 'Papelera en lote: deshacer el envío de una selección y vaciarla',
    producto: 'drive',
    web: true,
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

  // --- Contacts ------------------------------------------------------------
  // Las acciones rápidas de la lista y la ficha (2026-11-05). Contacts no tenía
  // entrada propia: la app de Fyne (`apps/fyne/contactos`) se portó entera antes de
  // que este registro anotara funciones sueltas.
  //   · «Deshacer» en el aviso de «movido a la papelera», de uno (editor) o en lote
  //     (`client/deshacer.ts`; en Fyne, `contactos/deshacer.go`, mismos nombres): UNA
  //     petición a `/contacts/batch/restore`, que revive en el servicio lo último que
  //     se tiró de cada UID, y el recuento contra lo que el lote dijo que tiró. Si en
  //     esos diez segundos se recuperó a mano, no revive una copia más vieja (2026-11-06).
  //   · Destacar desde la fila: estrella al pasar el puntero, siempre con el dedo
  //     (`FavoriteToggle row`, botón con el nombre del contacto; en Fyne, `AlDestacar`
  //     de `FilaContacto`, que ignora otro toque mientras va la petición de ese UID).
  //     2026-11-06: en Fyne la estrella es un control propio (`estrellaFila`), con
  //     parada de teclado, Intro/Espacio, nombre «Destacar a …» y visible con el foco,
  //     como el `IconButton` de la web; antes era una zona del toque de la fila.
  //   · «Enviar correo a todos» y «Exportar (.vcf)» sobre la selección y sobre la
  //     etiqueta activa (`client/seleccion.ts`; en Fyne, `contactos/seleccion.go`).
  //     La web exporta las vCard que ya tiene; Fyne, desde el 2026-11-06, en UNA
  //     petición por tanda de doscientos (`/contacts/batch/vcard`, que el servicio
  //     manda en streaming), no una por contacto; si no queda ninguno vivo, avisa en
  //     vez de guardar un `.vcf` vacío.
  //   · Copiar cada correo, teléfono o dirección de la ficha con un clic
  //     (`CopyValue`; en Fyne, `botonCopiar` y `AlCopiar`).
  // Código de Fyne y tests del driver hechos; falta verlas en una ventana real.
  {
    id: 'contacts-acciones-rapidas',
    nombre: 'Acciones rápidas de la lista y la ficha de Contacts',
    producto: 'contacts',
    web: true,
    escritorio: 'planificado',
    movil: 'planificado',
    deuda: DEUDA_AMBAS,
  },

  // --- Calendar ------------------------------------------------------------
  {
    id: 'calendar-crear',
    nombre: 'Crear eventos',
    producto: 'calendar',
    web: true,
    // Código y tests del driver hechos (auditoría del 2026-10-01); falta verla en una
    // ventana real para subir el escritorio a 'si'.
    // 2026-11-05: organizador e invitados con su respuesta (sí/quizá/no/sin responder)
    // en la vista rápida y en las píldoras del formulario, y guardar ya no devuelve a
    // todos a NEEDS-ACTION; web (`GuestList`, `parseGuests`) y Fyne
    // (`seccionInvitados`). Sin ventana real aún. Diferencia aceptada: el organizador
    // sale por su CN en la web (`parseOrganizer`) y por su dirección en Fyne, porque
    // `api.Event.Organizer` no trae el CN (el servicio no lo guarda; ver roadmap).
    // 2026-11-05: la búsqueda de eventos filtra por calendario propio y por periodo
    // (cualquiera, próximos, pasados, este mes, este año) en web (`periodRange`) y Fyne
    // (`rangoPeriodo`, `api.SearchEventsFiltrada`). Sin ventana real aún.
    // 2026-11-05: la píldora de la vista de mes lleva la hora de inicio desde `sm:`
    // (640 de ventana) en web (`EventChip`) y Fyne (`pildora.conHora`). Sin ventana real.
    // 2026-11-05: «?» abre la hoja de atajos en web (`lib/atajos.ts` + `ShortcutsModal`) y
    // Fyne (`calendario/atajos.go` + `ui.MostrarHojaAtajos`, la hoja de Mail subida a `ui`).
    // 2026-11-06: por debajo de sm: la vista de mes es el mes compacto del teléfono (semanas
    // que tocan el mes con su número ISO, puntos por evento, selección = cursor, lista del
    // día, cabecera ☰ · mes · hoy · +, aviso de pendientes en ☰, deslizar sobre los días y
    // asa) en web (`MesCompacto`, `modeloMesCompacto.ts`) y Fyne (`vistamescompacta.go`,
    // `mescompacto.go`), con los mismos vectores V1–V14 a los dos lados. La píldora
    // compacta de 9d1608ec5 se retira; la barra de día completo estrecha se queda porque la
    // usa la franja de la semana de Fyne (vistahoraria.go:330): ahí la web es px-1.5 +
    // truncate y Fyne px-0.5 + clip.
    // 2026-11-07: el selector de vista del cajón es 2×2 (`SelectorVista rejilla` /
    // `GrupoVista.Rejilla`), el título de la barra usa el hueco y baja a text-sm antes de
    // truncar (`AppShell` / `ui.BarraMovil`), `n`/`p` dan el paso de lo que se ve y el
    // primer pintado ya es compacto (`max-sm:hidden` / `VentanaInicial`).
    // 2026-11-08: el asa cambia entre mes, semana (el mes plegado) y agenda, y ES la vista:
    // el selector del cajón y el asa dicen lo mismo, y por debajo de sm: la semana es el
    // mes plegado (la rejilla horaria sigue desde sm:). Se arrastra con la hoja pegada al
    // dedo y al soltar se ajusta por recorrido y velocidad (`asaMes.ts` / `asames.go`, con
    // los mismos vectores A1–D3); toque, ↑/↓ e Inicio/Fin también. Web `slider` con
    // `aria-valuetext`; Fyne con la vista en el nombre. Con esto la franja estrecha de
    // día completo solo vive en el DÍA por debajo de sm:.
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
    // Incluye el filtro del cuaderno: si oculta todas las notas que hay, el vacío dice
    // «ninguna nota coincide» y ofrece «Quitar el filtro» (lienzo y árbol), en las dos
    // superficies (`onQuitarFiltro` en NoteCanvas/NoteTree; `AlQuitarFiltro` en Fyne).
    // La búsqueda dice «Buscando…», cuántos resultados hay, y un fallo como fallo (con
    // «Reintentar»), no como «nada encontrado»; en la web el resultado se abre sin
    // recargar la página (Fyne ya lo abría en la propia ventana).
    escritorio: 'planificado',
    movil: 'planificado',
    deuda: DEUDA_AMBAS,
  },
  {
    id: 'notes-eliminar',
    nombre: 'Eliminar nota',
    producto: 'notes',
    web: true,
    // Eliminar y mover de cuaderno dejan un aviso con «Deshacer» que deshace ESE paso
    // del historial (`deshacerEsta` / `DeshacerEsta`), no el último: web y Fyne.
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
    id: 'notes-editor-rapido',
    nombre:
      'Editor rápido: comandos «/», burbuja de formato, buscar en la nota, listas plegables, «Ir a», resaltado de lo buscado y ficha de enlaces a notas',
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
    // Con la organización de cuadernos: «Mover cuaderno a…» (cambiar de padre o sacar a
    // la raíz) sin arrastrar, con las mismas condiciones en las dos superficies
    // (`puedeMoverCuaderno`). En la web en el «…» de la fila del árbol y de la página;
    // en Fyne en el «…» de la página (`dialogoMoverCuaderno`), porque su fila del árbol
    // lleva dos botones y no menú.
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
  {
    id: 'notes-libreta-papel',
    nombre:
      'Libreta sobre papel: cromo neutro, cada nota una línea con su marca, cabecera del día en voz baja y glifos de una sola tinta',
    producto: 'notes',
    web: true,
    // Cromo «papel» (ADR 0079) en tokens nativos y Fyne con capturas del pintor de
    // software en claro y oscuro, 1280 y 390; falta la ventana real y el teléfono.
    escritorio: 'planificado',
    movil: 'planificado',
    deuda: DEUDA_AMBAS,
  },
  {
    id: 'notes-captura-lineas-tarea',
    nombre:
      'Pegar varias líneas en la captura (una nota por línea) y convertir una línea en tarea o en texto',
    producto: 'notes',
    web: true,
    // Fyne con `Entrada.AlPegar`/`Pegar` y convertir.go, con tests del driver.
    escritorio: 'planificado',
    movil: 'planificado',
    deuda: DEUDA_AMBAS,
  },
  {
    id: 'notes-editor-enlazar-mencionar',
    nombre: '`[[` para enlazar otra nota y `@` para mencionar a alguien mientras se escribe',
    producto: 'notes',
    web: true,
    // Fyne: un menú para «/», `[[` y «@» (comandos.go), reglas de mención en ui/menciones.go.
    escritorio: 'planificado',
    movil: 'planificado',
    deuda: DEUDA_AMBAS,
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
    // 2026-11-05: «Pausar avisos» con duración (30 min, 1 h, 2 h, mañana, lunes) en
    // web (`lib/pausa.ts`, StatusMenu) y Fyne (`pausa.go`, menú de estado), con la
    // pausa en el servicio (mig. 0061) y tests en las dos. Cada duración lleva el
    // título en su nombre accesible (`pause.optionAria`; en Fyne, `ElementoMenu.Rotulo`)
    // y una relectura vieja no pisa la pausa recién puesta (`turnosDePausa`/`pausaTurno`).
    // 2026-11-06: con una escritura en vuelo el menú va inerte en las dos
    // (`pauseSaving` / `pausaGuardando`) y Fyne no lanza una segunda.
    // 2026-11-05: total de no leídos en el título de la pestaña (web,
    // `lib/totalNoLeidos.ts`) y sobre el icono de la app (Fyne, `TotalNoLeidos` +
    // `BarraSuite.PonerInsignia`), con tests en las dos. Desde el 2026-11-06 cuenta
    // también lo que llega al canal abierto en segundo plano (`sumaNoVisto`/`SumaNoVisto`).
    // Por id en las dos (una trama repetida no suma dos veces). Diferencia que queda:
    // «segundo plano» es pestaña oculta en la web y ventana sin foco en Fyne escritorio.
    // 2026-11-05: pestaña «Archivados» de «Explorar canales» (verlos, renombrar y
    // desarchivar) en web (`ArchivedChannels.tsx`) y Fyne (`archivados.go`), con tests en
    // las dos; archivar sigue siendo solo de la web. 2026-11-06: en Fyne la fila que
    // se renombra se reutiliza entre pintados (`enEdicion`), como el `<li>` con key.
    // 2026-11-05: ↑ con la caja vacía edita tu último mensaje del canal, en web
    // (`lib/editarUltimo.ts`) y en Fyne (`UltimoEditable`, `Sala.EditarUltimo`), con
    // tests en las dos; sin ventana real todavía, así que los estados no cambian.
    // 2026-11-06: cerrarla (Intro o Esc) devuelve el foco al compositor desde el que
    // se pidió con ↑, en web (`volverFocoRef`) y Fyne (`edicionConFlecha`).
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
    // «Hacer una copia» (2026-11-05), de los tres tipos: en la web desde el menú «Más
    // acciones» de la tesela y desde Archivo; en Fyne, botón de la tesela y Archivo. El
    // servicio copia el contenido (`copy_of`); el editor guarda antes lo pendiente.
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
    // Zoom con Ctrl/⌘ +, − y 0, y el «100 %» de la barra de estado que vuelve al 100
    // de un clic (2026-11-05): el mismo marco en Docus, Calcus y Diapos, web y Fyne.
    // El chip «Guardado hace…» de la cabecera abre el historial de versiones (el mismo
    // menú que Colaboración) y, en error, reintenta el guardado (2026-11-05).
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
    // Autofiltro por condición (2026-11-05): el formulario «Filtrar por condición…» del
    // «▾» llega a Fyne (la web ya lo tenía), y los operandos tecleados como fecha filtran
    // por fecha en la web, en Fyne y en el export del servicio.
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
