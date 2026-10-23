/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * EL GATE QUE IMPIDE QUE LA WEB SE ADELANTE A LAS NATIVAS — NORMATIVO (§13.7).
 *
 * `paridad.ts` gobierna FUNCIONES y se escribe a mano, así que se queda viejo en
 * silencio: es el modo de fallo de todo registro manual. Medido hoy en este
 * repositorio: declara 31 funciones de web mientras la web tiene 80 pantallas
 * (`.astro` bajo `src/pages/`, sin el sitio público) y 46 piezas compartidas
 * (`packages/ui/src/components/*.tsx`, sin la TopBar). Nadie mintió en una
 * entrada; simplemente el registro dejó de mirar.
 *
 * Aquí NO se declara lo que la web tiene: se DERIVA de los ficheros, que es lo
 * que no se puede olvidar de actualizar. Lo único declarado es lo que no se
 * puede leer de un árbol de ficheros:
 *
 *   1. `SOLO_WEB` — lo que por decisión de producto no va a las nativas, con su
 *      motivo. Hoy son dos entradas y ninguna más.
 *   2. `DEUDA_SIN_NATIVO` — la foto de lo que HOY no está portado. Es el
 *      trinquete: no puede crecer sin que alguien añada la línea a mano.
 *
 * Y la contrapartida nativa tampoco se declara aquí: la declara el propio
 * fichero nativo que porta la pantalla, con un marcador en el código:
 *
 *     // paridad-web: apps/frontend/apps/mail/src/pages/index.astro
 *
 * SIN NOMBRE DE TOOLKIT en ninguna parte, a propósito: el marcador vale desde
 * cualquier superficie nativa de `apps/` y el registro habla de `escritorio` y
 * `movil`. El toolkit ha cambiado dos veces en un mes —y ahora mismo hay dos
 * vías vivas a la vez—, así que un gate con el nombre dentro habría que
 * reescribirlo cada vez.
 *
 * Así el gate falla en las CUATRO direcciones que importan, y siempre con el
 * nombre del fichero delante:
 *
 *   - la web estrena una pantalla o una pieza y nadie la porta  → falta en la foto;
 *   - lo nativo porta una y nadie la saca de la foto            → sobra en la foto;
 *   - la web renombra o borra algo que lo nativo decía portar   → marcador colgado;
 *   - un fichero nativo intenta servir a dos productos de la web → apps no independientes.
 *
 * NO EXIGE PARIDAD TOTAL HOY, a propósito: con 126 pendientes y varias sesiones
 * escribiendo las nativas, un gate que la exigiera se pondría rojo entero y
 * alguien lo apagaría. El trinquete solo prohíbe que la deuda CREZCA.
 *
 * SI ESTE TEST TE PARA: no añadas tu fichero a `SOLO_WEB`. Eso es lo último, no
 * lo primero, y pide escribir por qué esa pantalla NUNCA va a ser nativa. Lo
 * normal es una de dos: acabas de estrenar algo en la web y toca añadir su línea
 * a `DEUDA_SIN_NATIVO` (y portarlo, o dejar dicho cuándo), o acabas de portarlo y
 * toca borrar esa misma línea. Cómo se salda la deuda: CONVENTIONS.md §13.7.
 */
import { expect, test } from 'bun:test';

/** Raíz del monorepo: `src/` → config → packages → frontend → apps → raíz. */
const RAIZ = new URL('../../../../../', import.meta.url);

/**
 * De dónde se DERIVA lo que la web tiene. Un fichero = una pantalla o una pieza.
 * No hay lista que mantener: si mañana nace `apps/frontend/apps/x/src/pages/y.astro`,
 * el inventario lo trae solo.
 */
const FUENTES: { que: string; glob: string; es: (ruta: string) => boolean }[] = [
  {
    que: 'pantalla',
    glob: 'apps/frontend/apps/*/src/pages/**',
    // Solo `.astro`: los `.ts` de `pages/` son endpoints del BFF (491 de los 580
    // ficheros), no superficie que se vea. Una API no se «porta»: se llama.
    es: (r) => r.endsWith('.astro'),
  },
  {
    que: 'pieza',
    glob: 'apps/frontend/packages/ui/src/components/**',
    es: (r) => r.endsWith('.tsx') && !r.includes('.test.'),
  },
];

/**
 * Dónde puede vivir un marcador. Se scanea `apps/` entera menos la web, en vez
 * de nombrar los directorios de cada toolkit: el toolkit ya cambió dos veces y
 * el gate no tiene por qué enterarse.
 */
const NO_ES_NATIVO: Record<string, string> = {
  'apps/frontend/': 'es la web: es el ORIGEN de la paridad, no su contrapartida',
};

/** Los lenguajes en los que hoy se escribe una superficie nativa. */
const FUENTE_NATIVA = /\.(go|ts|tsx|swift|kt)$/;

/**
 * LO QUE NUNCA VA A LAS NATIVAS, con su motivo.
 *
 * Añadirse aquí es la ÚLTIMA opción: significa «esto no existirá jamás en
 * escritorio ni en móvil», no «esto todavía no está» — para eso está la deuda.
 * Una clave que acabe en `/` cubre el directorio entero.
 */
const SOLO_WEB: Record<string, string> = {
  'apps/frontend/packages/ui/src/components/TopBar.tsx':
    'EXCEPCIÓN de §13.6: la TopBar existe SOLO en la web; en nativo la sustituye el cromo del sistema (barra de título y menú). Ya registrada como `shell-topbar` en paridad.ts.',
  'apps/frontend/apps/account/src/pages/index.astro':
    'DECISIÓN del dueño (2026-09-22): «Mi cuenta» se accede SOLO por web. El menú de cuenta de la barra transversal de cada app nativa abre el navegador en Sytue ID, en vez de llevar dentro una copia de la pantalla. Se llegó a portar a Fyne y se retiró.',
  'apps/frontend/packages/ui/src/components/AppGrid.tsx':
    'EXCEPCIÓN de §13.6: su único consumidor es el selector de aplicaciones de la TopBar (`shell/AppSwitcher.tsx`), que en nativo no existe: se cambia de app en el sistema.',
  'apps/frontend/packages/ui/src/components/SkipLink.tsx':
    'Es un ancla HTML a un landmark del documento, invisible hasta recibir el foco. Una ventana nativa no tiene anclas ni landmarks; moverse entre paneles con el teclado es otra pieza.',
  'apps/frontend/apps/docus/src/pages/wopi/editor.astro':
    'DECISIÓN del dueño (2026-10-01): es la página del navegador que aloja el editor externo por WOPI —desde el 2026-10-12 la sirve Docus para cualquier fichero—. En escritorio, «Abrir en el editor» de Drive la abre en el navegador, como «Mi cuenta»; no hay versión nativa de un protocolo de navegador.',
  'apps/frontend/apps/calendar/src/pages/dev/servicios-vinculados.astro':
    'Banco de pruebas de DESARROLLO del panel compartido de servicios personales para Playwright: en producción es un 404. Las funciones están en las apps Fyne; la fixture no es una pantalla del producto.',
  'apps/frontend/apps/mail/src/pages/dev/correo-externo.astro':
    'Banco de pruebas de DESARROLLO del asistente real de correo externo para Playwright: en producción es un 404. La función se porta en Fyne; la fixture no es una pantalla del producto.',
  'apps/frontend/apps/drive/src/pages/dev/almacenamiento.astro':
    'Banco de pruebas de DESARROLLO del wizard real de almacenamiento externo para Playwright: en producción es un 404 y nada lo enlaza. No es una pantalla del producto ni necesita ruta nativa.',
  'apps/frontend/apps/rooms/src/pages/dev/conexiones.astro':
    'Banco de pruebas de DESARROLLO del wizard real de consentimiento y canales de Rooms para Playwright: en producción es un 404 y nada lo enlaza. No es una pantalla del producto ni necesita ruta nativa.',
  'apps/frontend/apps/rooms/src/pages/dev/avisos-directos.astro':
    'Banco de pruebas de DESARROLLO de los avisos de mensajes directos de Rooms para Playwright: en producción es un 404. Los avisos también funcionan en Fyne; la fixture no es una pantalla del producto ni necesita ruta nativa.',
  'apps/frontend/apps/rooms/src/pages/dev/idioma-sistema.astro':
    'Banco de pruebas de DESARROLLO de la preferencia de idioma del sistema para Playwright: en producción es un 404. La preferencia también funciona en Fyne; la fixture no es una pantalla del producto ni necesita ruta nativa.',
  'apps/frontend/apps/docus/src/pages/dev/cinta.astro':
    'Banco de pruebas de DESARROLLO de la cinta, para la prueba visual de Playwright (test/e2e/cinta.mjs): en producción es un 404 y nada lo enlaza. No es una pantalla del producto, así que no hay nada que portar.',
  'apps/frontend/apps/docus/src/pages/dev/editor.astro':
    'Banco de pruebas de DESARROLLO del editor entero para la auditoría de accesibilidad de Playwright (test/e2e/accesibilidad-office.mjs): en producción es un 404 y nada lo enlaza. No es una pantalla del producto, así que no hay nada que portar.',
  'apps/frontend/apps/docus/src/pages/dev/inicio.astro':
    'Banco de pruebas de DESARROLLO de los inicios de Office para test/e2e/inicio-office-scroll.mjs: en producción es un 404 y nada lo enlaza. Mide la lista real con datos sintéticos; no es una pantalla del producto ni necesita una ruta nativa.',
  'apps/frontend/apps/docus/src/pages/dev/pdf.astro':
    'Banco de pruebas de DESARROLLO de CertiPDF para la misma auditoría de accesibilidad: en producción es un 404 y nada lo enlaza. No es una pantalla del producto, así que no hay nada que portar.',
  'apps/frontend/apps/docus/src/pages/dev/superposiciones.astro':
    'Banco de pruebas de DESARROLLO de Modal, Popover y compositor de Rooms para test/e2e/superposiciones.mjs: en producción es un 404 y nada lo enlaza. Mide componentes reales con datos sintéticos; no es una pantalla del producto ni necesita una ruta nativa.',
  'apps/frontend/apps/cast/src/pages/embed/[id].astro':
    'El reproductor para un `<iframe>` en la web de OTRO (CAST-ENCARGO §1.7): no hay iframe ajeno en una app nativa. Ver un vídeo en nativo es `watch/[id]`, que sí está en la deuda.',
  'apps/frontend/apps/docus/src/pages/dev/superficies.astro':
    'El banco de pruebas de DevX existe para medir las superficies web de edición durante el desarrollo. No forma parte de una aplicación de escritorio o móvil y no tendrá contraparte nativa.',
  'apps/frontend/apps/console/':
    'DECISIÓN del dueño (2026-10-01): Sytue Admin es solo web, para siempre. La consola de administración de la organización se usa desde el navegador; el catálogo ya la tiene como `nativa: false` y no hay `apps/fyne/cmd` de Admin.',
  'apps/frontend/apps/cloud-dashboard/':
    'DECISIÓN del dueño (2026-10-01): Sytue DevX es solo web, para siempre. El panel del PaaS se usa desde el navegador; el catálogo ya lo tiene como `nativa: false` y no hay `apps/fyne/cmd` de DevX.',
  'apps/frontend/apps/pages/':
    'DECISIÓN del dueño (2026-10-02): Sytue Web es solo web por ahora, como Admin y DevX. El editor de marcos, la publicación alojada y la tienda se usan desde el navegador; el catálogo lo tiene como `nativa: false` y no hay `apps/fyne/cmd` de Pages. A diferencia de ellos no dice «para siempre»: si algún día se porta, esta entrada se quita y sus pantallas vuelven a la deuda.',
  'apps/frontend/apps/bases/src/pages/dev/':
    'Bancos de pruebas de DESARROLLO de Sytue Bases (las pantallas enteras, la rejilla y las vistas) para la auditoría de accesibilidad de Playwright: en producción son un 404 y nada los enlaza. No son pantallas del producto, así que no hay nada que portar.',
  'apps/frontend/apps/bases/src/pages/f/[token].astro':
    'DECISIÓN conservadora (pendiente de confirmar por el dueño): el formulario público lo rellena quien NO tiene cuenta, desde el enlace que le pasan, y entra por el BFF de la web con su reto contra robots; el servicio no publica esa ruta a propósito (ADR 0068). Una app nativa pide sesión, así que quien la tiene no es el destinatario del enlace: se abre en el navegador. La GESTIÓN del enlace sí va en la app nativa, dentro de la base abierta.',
  'apps/frontend/apps/www/':
    'sytue.eu es el sitio público de marketing: SSG, sin sesión y sin backend. No es un producto de la suite —no figura en PRODUCTOS_PARIDAD— y una app de escritorio no lleva dentro su propia web comercial.',
  'apps/frontend/apps/rooms/src/pages/matrix/external/callback.astro':
    'Página de retorno del flujo OAuth de Matrix/BFF en el navegador que completa la autorización y redirige con window.location.replace; en nativo el flujo de autenticación usa el cliente propio.',
};

/** El marcador que un fichero nativo pone para decir qué pantalla de la web porta. */
const MARCADOR = /^\s*(?:\/\/|#)\s*paridad-web:\s*(\S+)/;

/**
 * LA FOTO DE LA DEUDA — el trinquete. Cada línea es algo que la web tiene y las
 * nativas todavía no. BAJA cuando se porta (se borra la línea y se pone el
 * marcador en el fichero nativo); SUBE solo a mano, y entonces se ve en la
 * revisión.
 *
 * 2026-09-21: 126 — el punto de partida del port nativo, con todo por hacer.
 * 2026-09-22: 123 — Sytue Hub porta su portada, su actividad y su búsqueda
 *   (`apps/fyne/apps/hub`). Las otras cinco pantallas del portal siguen aquí.
 * 2026-09-22: 123 — Sytue Calendar (sus dos rutas) y Sytue Contacts, en Fyne.
 * 2026-09-22: +1 — `PaginaNoEncontrada`, el 404 de la web. En nativo no hay rutas,
 *   pero sí enlaces profundos: su contrapartida es el enlace que no resuelve.
 * 2026-09-22: −1 — la portada de documentos (`DocList.tsx`), en `apps/fyne/editor`.
 * 2026-09-22: 112 — Sytue Hub porta su bandeja de avisos (`/notificaciones`).
 * 2026-09-23: 111 — Sytue Hub porta «Tus datos» (`/datos`).
 * 2026-09-23: 110 — Sytue Hub porta «Conecta tus dispositivos» (`/dispositivos`).
 * 2026-09-23: 100 — Photos porta nueve rutas y CertiPDF la pantalla PDF.
 * 2026-09-24: 99 — Sytue Hub porta «Descarga las aplicaciones» (`/descargas`): la lista
 *   se la pide al catálogo de instaladores del Hub, que lleva la credencial del almacén.
 * 2026-09-24: 98 — Sytue Hub porta el alta guiada (`/bienvenida`), contra Sytue ID con
 *   `api.Alta`. El portal entero es ya nativo.
 * 2026-09-24: 97 — Photos porta el álbum compartido (`/p/{token}`), que el binario abre
 *   por su enlace.
 * 2026-09-25: +1 — Docs estrena `/s/{token}`, la vista de solo lectura de un enlace
 *   compartido, que copiaban la web y las nativas y no existía. Portarla es abrirla desde
 *   el binario de Docus/Calcus por su enlace, como el álbum de Photos.
 * 2026-09-28: +8 — Sytue Cast llega a la web con siete pantallas (inicio, explorar,
 *   canal, ver, subir, emitir, gestionar) y `ProgressBar` en @sytue/ui. La app de Fyne
 *   (`apps/fyne/apps/cast`, CAST-ENCARGO §11) las porta y baja estas ocho líneas.
 * 2026-09-28: −8 — la app de Fyne de Sytue Cast porta las siete pantallas y
 *   `ui.BarraProgreso` es el ProgressBar.
 * 2026-09-29: +5 — Sytue Web llega a la web; solo web por ahora (decisión 9).
 * 2026-10-01: −45 — Sytue Admin (23) y Sytue DevX (22) pasan a SOLO_WEB por decisión del
 *   dueño: no tendrán app nativa. No es deuda saldada, es deuda que nunca lo fue.
 * 2026-10-01: −25 — 23 piezas de @sytue/ui que `apps/fyne/ui` ya tenía y nunca se
 *   marcaron, y AppGrid y SkipLink a SOLO_WEB (no existen en una ventana nativa).
 * 2026-10-01: −1 — Sytue Mail, portada desde el 22/09 y sin marcador por olvido. El cuerpo
 *   HTML con maqueta, que Fyne no pinta, queda como deuda propia (`mail-cuerpo-html`).
 * 2026-10-01: −1 — `docs/wopi/editor.astro` a SOLO_WEB: escritorio lo abre en el navegador.
 * 2026-10-02: −5 — Sytue Web pasa a SOLO_WEB (solo web por ahora, dueño): sus cinco pantallas
 *   salen de la deuda. No es deuda saldada, es deuda que se retira.
 * 2026-10-04: +1 — `ResizablePanel`, el panel de lista de `AppShell` que se estira con el
 *   puntero o las flechas y recuerda su ancho. Llegó a @sytue/ui sin su línea y dejó este
 *   gate en rojo; el `Armazon` nativo todavía tiene ese panel de ancho fijo. Se cierra
 *   cuando `apps/fyne/ui` lo deje ajustar, con el marcador en ese fichero.
 * 2026-10-05: +1 — `Glifo`, el icono de trazo común del rediseño. Llegó a @sytue/ui sin su
 *   línea y dejó este gate en rojo en la v3.10.0. Las nativas pintan sus glifos con un
 *   `glifos.go` por app, sin pieza común. Se cierra cuando `apps/fyne/ui` tenga la suya.
 * 2026-10-05: −1 — `Glifo` SÍ tenía pieza común nativa y solo le faltaba el marcador: el
 *   pintor de `apps/fyne/iconos/pintar.go` traza todos los iconos con su mismo grosor y su
 *   misma capa de relleno, y `TestElEstiloEsElDeGlifo` lee los números de ese .tsx. Los
 *   `glifos.go` de cada app son catálogos de DIBUJOS, como los de la web, y pintan a
 *   través de él.
 * 2026-10-05: +1 — Sytue Bases deja de ser solo web (casilla bases-2): «Mis bases» ya es
 *   nativa (`apps/fyne/apps/bases/lista.go`) y la base abierta (`base/[id].astro`) entra en
 *   la deuda hasta que su rejilla se vea en una ventana real. Los bancos de `dev/` y el
 *   formulario público (`f/[token]`) quedan en SOLO_WEB con su motivo.
 * 2026-10-05: −1 — la base abierta de Sytue Bases (`base/[id].astro`), en
 *   `apps/fyne/apps/bases/base.go`: tablas, rejilla editable por tipo de campo, vistas
 *   guardadas y carga por ventanas, vista en una ventana real. Lo que esa pantalla tiene
 *   en la web y la nativa todavía no (tablero, compartir, conexiones, comentarios,
 *   adjuntos, exportar, importar y el tiempo real) queda dicho en el PR que lo porta.
 * 2026-10-06: +1 — Sytue Cast estrena «Denuncias y bloqueos» (`moderation.astro`): a quién
 *   tienes bloqueado y, para quien administra, la cola de denuncias. La web va primero
 *   porque es la que tiene portada pública; la app de Fyne la porta en la PR siguiente
 *   (`apps/fyne/apps/cast`) y baja esta línea.
 * 2026-10-06: −1 — «Denuncias y bloqueos» de Sytue Cast, en `apps/fyne/apps/cast/moderacion.go`:
 *   los bloqueos propios y, para quien administra, la cola con sus acciones y los bloqueos de
 *   la organización. Vista pintada por los guardianes (seis estados, claro y oscuro).
 * 2026-10-12: +6 — NO es deuda nueva: Docs se separa en cuatro apps (decisión del dueño) y
 *   el editor (`edit.astro`) y el enlace compartido (`s/[token].astro`), que ya estaban
 *   aquí, ahora existen también en Docus, Calcus y Diapos. Las dos líneas de `docs/` se
 *   van con la app conjunta, en la PR que la retira. Las portadas, papeleras y la de
 *   CertiPDF no suben: las porta el `cmd/<producto>/main.go` de cada binario nativo.
 * 2026-10-12: −2 — se retira la app conjunta (`apps/docs`, ADR 0076): su editor y su enlace
 *   compartido salen de la foto, y los bancos de desarrollo pasan a Docus.
 * 2026-10-17: −1 — `ResizablePanel`, portado a Fyne en `apps/fyne/ui/armazon.go`: el panel
 *   de lista del `Armazon` nativo ahora es redimensionable mediante arrastre del separador,
 *   atajos de teclado (flechas, Inicio, Fin) y doble clic de restablecimiento, con
 *   persistencia de ancho en preferencias de usuario.
 */
const DEUDA_SIN_NATIVO = `
apps/frontend/apps/calcus/src/pages/edit.astro
apps/frontend/apps/calcus/src/pages/s/[token].astro
apps/frontend/apps/diapos/src/pages/edit.astro
apps/frontend/apps/diapos/src/pages/s/[token].astro
apps/frontend/apps/docus/src/pages/edit.astro
apps/frontend/apps/docus/src/pages/s/[token].astro
apps/frontend/apps/notes/src/pages/p/[token].astro
apps/frontend/apps/rooms/src/pages/pip.astro
apps/frontend/packages/ui/src/components/AvatarPicker.tsx
apps/frontend/packages/ui/src/components/BarChart.tsx
apps/frontend/packages/ui/src/components/DataFlow.tsx
apps/frontend/packages/ui/src/components/FieldHint.tsx
apps/frontend/packages/ui/src/components/IconButton.tsx
apps/frontend/packages/ui/src/components/Illustration.tsx
apps/frontend/packages/ui/src/components/LanguageSelect.tsx
apps/frontend/packages/ui/src/components/List.tsx
apps/frontend/packages/ui/src/components/Meter.tsx
apps/frontend/packages/ui/src/components/MobileFab.tsx
apps/frontend/packages/ui/src/components/Modal.tsx
apps/frontend/packages/ui/src/components/OrgMark.tsx
apps/frontend/packages/ui/src/components/ReviewSummary.tsx
apps/frontend/packages/ui/src/components/SettingsShell.tsx
apps/frontend/packages/ui/src/components/ShortcutsModal.tsx
apps/frontend/packages/ui/src/components/Toolbar.tsx
apps/frontend/packages/ui/src/components/VirtualList.tsx
apps/frontend/packages/ui/src/components/Wizard.tsx
apps/frontend/packages/ui/src/components/dialogosImperativos.tsx
apps/frontend/packages/ui/src/components/useConfirm.tsx
apps/frontend/packages/ui/src/components/usePrompt.tsx
`
  .trim()
  .split('\n');

/**
 * Los ficheros de `apps/` que el repositorio conoce: versionados, y nuevos sin
 * confirmar que git no ignora. Recorrer `apps/**` a pelo bajaba también por
 * `node_modules` —medido: 49 513 ficheros, 2,9 s en frío; con la máquina cargada
 * pasaba de los 5 s del timeout y el test daba un rojo falso—. Git ya sabe qué
 * es código y qué es dependencia o salida de build.
 */
function ficherosDeApps(): string[] {
  const r = Bun.spawnSync(
    ['git', 'ls-files', '--cached', '--others', '--exclude-standard', '-z', '--', 'apps/'],
    { cwd: RAIZ.pathname },
  );
  if (!r.success) throw new Error(`git ls-files no ha podido listar apps/: ${r.stderr.toString()}`);
  return r.stdout.toString().split('\0').filter(Boolean);
}

/**
 * Cada recorrido se hace UNA vez por ejecución y lo comparten los tests: el árbol no
 * cambia a mitad de la suite.
 */
function unaVez<T>(f: () => Promise<T>): () => Promise<T> {
  let hecho: Promise<T> | undefined;
  return () => (hecho ??= f());
}

/** Lo que la web tiene hoy, leído del disco. */
const inventario = unaVez(async (): Promise<string[]> => {
  const todo: string[] = [];
  for (const fuente of FUENTES) {
    for await (const rel of new Bun.Glob(fuente.glob).scan({ cwd: RAIZ.pathname })) {
      if (fuente.es(rel)) todo.push(rel);
    }
  }
  return todo.sort();
});

/** Qué rutas de la web dice portar lo nativo, y desde qué fichero lo dice. */
const portadas = unaVez(async (): Promise<Map<string, string>> => {
  const mapa = new Map<string, string>();
  for (const rel of ficherosDeApps()) {
    if (!FUENTE_NATIVA.test(rel)) continue;
    if (Object.keys(NO_ES_NATIVO).some((k) => rel.startsWith(k))) continue;
    const f = Bun.file(new URL(rel, RAIZ));
    if (!(await f.exists())) continue; // versionado pero borrado en esta copia
    const fuente = await f.text();
    if (!fuente.includes('paridad-web:')) continue;
    for (const linea of fuente.split('\n')) {
      const m = MARCADOR.exec(linea);
      if (m) mapa.set(m[1]!, rel);
    }
  }
  return mapa;
});

/** ¿Cubre `SOLO_WEB` esta ruta, por sí misma o por su directorio? */
function esSoloWeb(ruta: string): boolean {
  return Object.keys(SOLO_WEB).some((k) => (k.endsWith('/') ? ruta.startsWith(k) : k === ruta));
}

/** El producto de la web al que pertenece una ruta, o '' si es una pieza compartida. */
function productoWeb(ruta: string): string {
  return /^apps\/frontend\/apps\/([^/]+)\//.exec(ruta)?.[1] ?? '';
}

/** Lo que la web tiene, lo nativo no porta y nadie ha declarado como solo-web. */
function pendientes(todo: string[], portado: Map<string, string>): string[] {
  return todo.filter((r) => !portado.has(r) && !esSoloWeb(r)).sort();
}

test('la web no se adelanta a las nativas: la deuda no crece sin que se vea — §13.7', async () => {
  const hoy = pendientes(await inventario(), await portadas());
  const foto = new Set(DEUDA_SIN_NATIVO);
  const sinPortar = hoy.filter((r) => !foto.has(r));
  const yaNoPendientes = DEUDA_SIN_NATIVO.filter((r) => !hoy.includes(r));

  const fallos = [
    ...sinPortar.map(
      (r) =>
        `NUEVO EN LA WEB Y NO EN LAS NATIVAS: ${r}\n` +
        `    Pórtalo y pon en el fichero nativo «// paridad-web: ${r}»,\n` +
        `    o añade la línea «${r}» a DEUDA_SIN_NATIVO (paridad-pantallas.test.ts) y di en el PR cuándo se cierra.`,
    ),
    ...yaNoPendientes.map(
      (r) =>
        `YA NO ESTÁ PENDIENTE: ${r}\n` +
        `    Está portado o ha desaparecido de la web: BORRA esa línea de DEUDA_SIN_NATIVO.\n` +
        `    Saldar deuda sin bajar la foto deja hueco libre para la siguiente, que es como se llega a 35 de 35.`,
    ),
  ];
  expect(fallos).toEqual([]);
});

test('ningún marcador nativo apunta a una pantalla que la web ya no tiene — §13.7', async () => {
  const todo = new Set(await inventario());
  const colgados: string[] = [];
  for (const [ruta, fichero] of await portadas()) {
    if (!todo.has(ruta) && !esSoloWeb(ruta)) {
      colgados.push(
        `${fichero}: «// paridad-web: ${ruta}» apunta a un fichero que la web ya no tiene.\n` +
          `    O se renombró —y el marcador hay que reapuntarlo— o se borró, y entonces la pantalla nativa sobra.`,
      );
    }
  }
  expect(colgados).toEqual([]);
});

test('no quedan excepciones de solo-web que ya no hagan falta — §13.7', async () => {
  const todo = await inventario();
  const sobran: string[] = [];
  for (const clave of Object.keys(SOLO_WEB)) {
    const sigue = clave.endsWith('/')
      ? todo.some((r) => r.startsWith(clave))
      : await Bun.file(new URL(clave, RAIZ)).exists();
    if (!sigue) sobran.push(`${clave} ya no existe en la web: quita su excepción de SOLO_WEB`);
  }
  expect(sobran).toEqual([]);
});

test('la auditoría móvil excluye bancos de desarrollo sin ocultar pantallas reales', async () => {
  for (const ruta of [
    'docus/src/pages/dev/superposiciones.astro',
    'docus/src/pages/dev/inicio.astro',
    'calendar/src/pages/dev/servicios-vinculados.astro',
    'drive/src/pages/dev/almacenamiento.astro',
    'rooms/src/pages/dev/conexiones.astro',
    'rooms/src/pages/dev/avisos-directos.astro',
    'rooms/src/pages/dev/idioma-sistema.astro',
    'console/src/pages/dev/conectar-app.astro',
  ]) {
    const banco = await Bun.file(new URL(`apps/frontend/apps/${ruta}`, RAIZ)).text();
    expect(banco).toMatch(
      /if\s*\(!import\.meta\.env\.DEV\)\s*return new Response\(null,\s*\{\s*status:\s*404\s*\}\);/,
    );
  }
  for (const nombre of ['superposiciones', 'inicio']) {
    expect(esSoloWeb(`apps/frontend/apps/docus/src/pages/dev/${nombre}.astro`)).toBe(true);
  }
  expect(esSoloWeb('apps/frontend/apps/docus/src/pages/dev/pantalla-nueva.astro')).toBe(false);
  for (const nombre of ['avisos-directos', 'idioma-sistema']) {
    expect(esSoloWeb(`apps/frontend/apps/rooms/src/pages/dev/${nombre}.astro`)).toBe(true);
    expect(esSoloWeb(`apps/frontend/apps/rooms/src/pages/dev/${nombre}-nuevo.astro`)).toBe(false);
    expect(esSoloWeb(`apps/frontend/apps/rooms/src/pages/dev/${nombre}/otra.astro`)).toBe(false);
  }
  expect(esSoloWeb('apps/frontend/apps/rooms/src/pages/dev/pantalla-nueva.astro')).toBe(false);
  const fuente = await Bun.file(new URL('test/e2e/mobile-audit.mjs', RAIZ)).text();
  // Leer el matcher que usa el barrido, sin ejecutar navegador, sesión ni su inventario.
  const literal = /const NO_VISITABLES = \[\s*\[\s*\/([^\n]+)\/,/.exec(fuente)?.[1];
  expect(literal).toBeDefined();
  const matcher = new RegExp(literal!);
  const comprobar = (re: RegExp) =>
    [
      'docus/dev/cinta',
      'docus/dev/editor',
      'docus/dev/inicio',
      'docus/dev/pdf',
      'docus/dev/superficies',
      'docus/dev/superposiciones',
      'bases/dev/app',
      'bases/dev/vistas',
      'bases/dev/rejilla',
      'calendar/dev/servicios-vinculados',
      'drive/dev/almacenamiento',
      'rooms/dev/conexiones',
      'rooms/dev/avisos-directos',
      'rooms/dev/idioma-sistema',
      'console/dev/conectar-app',
    ].every((ruta) => re.test(ruta)) &&
    [
      'docus/dev/superposiciones-nueva',
      'docus/dev/superposiciones/otra',
      'docus/dev/inicio-nueva',
      'docus/dev/inicio/otra',
      'docus/dev/pantalla-nueva',
      'docus/edit',
      'rooms/index',
      'bases/dev/otra',
      'calendar/dev/servicios-vinculados-nuevo',
      'calendar/dev/servicios-vinculados/otra',
      'drive/dev/almacenamiento-nuevo',
      'drive/dev/almacenamiento/otra',
      'rooms/dev/conexiones-nueva',
      'rooms/dev/conexiones/otra',
      'rooms/dev/avisos-directos-nuevo',
      'rooms/dev/avisos-directos/otra',
      'rooms/dev/idioma-sistema-nuevo',
      'rooms/dev/idioma-sistema/otra',
      'rooms/dev/pantalla-nueva',
      'console/dev/conectar-app-nueva',
      'console/dev/conectar-app/otra',
    ].every((ruta) => !re.test(ruta));
  expect(comprobar(matcher)).toBe(true);
  // Los dos modos de fallo reales: olvidar esta fixture o excluir todo dev/.
  expect(comprobar(new RegExp(matcher.source.replace('superposiciones', 'quitada')))).toBe(false);
  expect(comprobar(new RegExp(matcher.source.replace('inicio', 'quitada')))).toBe(false);
  expect(comprobar(new RegExp(matcher.source.replace('avisos-directos', 'quitada')))).toBe(false);
  expect(comprobar(new RegExp(matcher.source.replace('idioma-sistema', 'quitada')))).toBe(false);
  expect(comprobar(new RegExp(matcher.source.replace('superposiciones', '.*')))).toBe(false);
});

/**
 * CADA APP ES INDEPENDIENTE TAMBIÉN EN NATIVO (corrección del dueño, 2026-09-21):
 * «las apps deben ser independientes aunque se llamen entre sí». Lo que NO se
 * replica de la web es el subdominio y el despliegue; la separación sí, y los
 * productos se invocan entre ellos por deep link. La correspondencia
 * app-de-web → app-nativa es 1:1.
 *
 * Esto se comprueba y no se comenta porque la forma cómoda de portar —un
 * fichero nativo que atiende la lista de Mail y la de Notepads, que «son casi
 * iguales»— es exactamente lo que fusiona dos productos sin que nadie lo decida,
 * y no deja rastro en ningún diff que se lea como tal.
 *
 * Las piezas compartidas (`packages/ui`) no tienen producto y quedan fuera: son
 * compartidas en la web por diseño y lo serán en nativo.
 */
function mezclanProductos(portado: Map<string, string>): string[] {
  const porFichero = new Map<string, Set<string>>();
  for (const [ruta, fichero] of portado) {
    const producto = productoWeb(ruta);
    if (!producto) continue;
    (porFichero.get(fichero) ?? porFichero.set(fichero, new Set()).get(fichero)!).add(producto);
  }
  return [...porFichero]
    .filter(([, productos]) => productos.size > 1)
    .map(
      ([fichero, productos]) =>
        `${fichero} porta pantallas de ${[...productos].sort().join(' y ')}: en nativo cada producto ` +
        `es su propia aplicación y se llaman entre sí por deep link, no compartiendo pantalla.`,
    )
    .sort();
}

test('un fichero nativo no sirve a dos productos de la web — §13.6', async () => {
  expect(mezclanProductos(await portadas())).toEqual([]);
});

test('el guardián de la independencia ve la mezcla y deja pasar lo legítimo', () => {
  const mezcla = new Map([
    ['apps/frontend/apps/mail/src/pages/index.astro', 'apps/nativo/lista.go'],
    ['apps/frontend/apps/notes/src/pages/index.astro', 'apps/nativo/lista.go'],
  ]);
  expect(mezclanProductos(mezcla)).toHaveLength(1);
  // Dos pantallas del MISMO producto en un fichero: normal, y no se toca.
  const mismoProducto = new Map([
    ['apps/frontend/apps/mail/src/pages/index.astro', 'apps/nativo/mail.go'],
    ['apps/frontend/apps/mail/src/pages/etiquetas.astro', 'apps/nativo/mail.go'],
  ]);
  expect(mezclanProductos(mismoProducto)).toEqual([]);
});

/*
 * Y que el trinquete se vea funcionar sin tener que tocar el árbol: una pantalla
 * nueva que nadie porta sale como pendiente, y una portada no sale.
 */
test('el trinquete detecta la pantalla nueva y suelta la portada', () => {
  const inv = [
    'apps/frontend/apps/mail/src/pages/index.astro',
    'apps/frontend/apps/mail/src/pages/nueva.astro',
    'apps/frontend/packages/ui/src/components/TopBar.tsx',
  ];
  const portado = new Map([
    ['apps/frontend/apps/mail/src/pages/index.astro', 'apps/nativo/mail.go'],
  ]);
  expect(pendientes(inv, portado)).toEqual(['apps/frontend/apps/mail/src/pages/nueva.astro']);
});
