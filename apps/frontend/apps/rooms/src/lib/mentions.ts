/**
 * Lógica pura de menciones (@usuario) del lado cliente: decidir si un mensaje
 * entrante nombra al usuario de la sesión y preparar el texto de la notificación
 * del navegador. Aislada aquí para poder probarla sin montar la isla (bun test).
 */

/**
 * mentionsUser señala si `msg` te menciona (y NO es propio): por tu sub en `mentions`
 * O por una mención a TODO el canal (@channel/@here/@all, `mentionsAll`). Es la
 * condición que sube el badge de menciones del canal y dispara la notificación del
 * navegador al llegar por WebSocket, así el badge de @menciones sube al instante también
 * para @channel/@here/@all. `mentionsAll` no exige `userId` (te alcanza como miembro).
 */
/** Patrón de mención a TODO el canal, IGUAL que `channelMentionRE` del backend
 *  (services/rooms/internal/domain/domain.go). Si los dos se separan, se confirma
 *  un mensaje que no notifica a nadie —o peor: se notifica al canal entero sin
 *  preguntar—. */
const CHANNEL_MENTION = /(^|[^\w@])@(channel|here|all)($|\W)/;

/** ¿El cuerpo menciona a todo el canal (@channel/@all) o a los presentes (@here)? */
export function mentionsWholeChannel(body: string): boolean {
  return CHANNEL_MENTION.test(body);
}

export function mentionsUser(
  msg: { mine?: boolean; mentions?: string[]; mentionsAll?: boolean },
  userId: string,
): boolean {
  if (msg.mine) return false;
  if (msg.mentionsAll) return true;
  return userId !== '' && (msg.mentions?.includes(userId) ?? false);
}

/**
 * Recorta el cuerpo del mensaje al texto corto de la notificación.
 *
 * Cuenta GRAFEMAS, no code points: `[...texto]` partía por la mitad los emojis
 * compuestos (👨‍👩‍👧 es una secuencia ZWJ de varios code points) y las notificaciones
 * podían acabar con medio emoji. `Intl.Segmenter` corta por donde lo haría el ojo.
 */
export function mentionNotificationBody(body: string, max = 140): string {
  const t = body.trim();
  const r = [...new Intl.Segmenter().segment(t)].map((s) => s.segment);
  return r.length > max ? r.slice(0, max).join('').trimEnd() + '…' : t;
}

/**
 * matchesKeyword indica si `body` contiene alguna de las `keywords` del usuario como
 * PALABRA completa (sin distinguir mayúsculas). Junto a @menciones es la base del aviso
 * de escritorio (Ola 13). Las palabras clave llegan ya en minúsculas del backend.
 *
 * ponytail: tokeniza el cuerpo en palabras Unicode (letras/dígitos/_) y compara por
 * igualdad; cubre el caso común (palabras sueltas: "deploy" no dispara con "redeploy").
 * No matchea frases de varias palabras — añadir un paso de subcadena si se piden.
 */
export function matchesKeyword(body: string, keywords: string[] | undefined): boolean {
  if (!keywords || keywords.length === 0) return false;
  const words = new Set(body.toLowerCase().match(/[\p{L}\p{N}_]+/gu) ?? []);
  return keywords.some((k) => {
    const kw = k.trim().toLowerCase();
    return kw !== '' && words.has(kw);
  });
}

/** Minutos del día [0..1439] de una fecha en HORA LOCAL (por defecto, ahora). */
export function localMinutesNow(d: Date = new Date()): number {
  return d.getHours() * 60 + d.getMinutes();
}

/**
 * inDndWindow indica si `nowMinutes` (minutos del día [0..1439], hora local) cae dentro
 * de la ventana «No molestar» [start, end). start==end = DND desactivado. Si start<end la
 * ventana es del mismo día; si start>end CRUZA MEDIANOCHE (p. ej. 22:00→07:00). Pura.
 */
export function inDndWindow(nowMinutes: number, start: number, end: number): boolean {
  if (start === end) return false; // desactivado
  if (start < end) return nowMinutes >= start && nowMinutes < end; // mismo día
  return nowMinutes >= start || nowMinutes < end; // cruza medianoche
}

/**
 * shouldNotifyNewMessage decide si un mensaje entrante dispara aviso de escritorio.
 *
 * Condiciones comunes: no es propio, es raíz (una respuesta en hilo avisa por su
 * propia ruta), el canal no está silenciado, el ajuste está activo y la ventana NO
 * tiene el foco —si la tienes, ya lo estás viendo—.
 *
 * Lo que decide el resto es de dónde viene, y NO es lo mismo:
 *
 *   - **DM**: avisa salvo que lo hayas bajado. El defecto de un mensaje directo es
 *     avisar, porque va dirigido a ti.
 *   - **Canal**: avisa SOLO si pediste «Todas las notificaciones» (notifyLevel 'all')
 *     de forma explícita. Sin fijar, un canal solo avisa por @mención —que va por otra
 *     ruta—, porque el defecto contrario dejaría a cualquiera con cincuenta canales
 *     recibiendo un aviso por cada mensaje de la organización.
 *
 * Antes esta función exigía `kind === 'direct'` y ahí se acababa: elegir «Todas las
 * notificaciones» en un canal no hacía absolutamente nada. La opción estaba en la
 * interfaz, se guardaba en la base de datos y no llegaba a ninguna parte.
 *
 * Pura y sin efectos, para probar la política aparte del permiso del navegador y de la
 * ventana DND (que comprueba el llamante con inDndWindow).
 */
export function shouldNotifyNewMessage(
  msg: { mine?: boolean; parentId?: string },
  channel: { kind: string; muted?: boolean; notifyLevel?: string } | undefined,
  opts: { windowFocused: boolean; enabled: boolean },
): boolean {
  if (!opts.enabled || opts.windowFocused || msg.mine || msg.parentId) return false;
  if (channel === undefined || channel.muted) return false;
  if (channel.notifyLevel === 'none' || channel.notifyLevel === 'mentions') return false;
  if (channel.kind === 'direct') {
    return channel.notifyLevel === undefined || channel.notifyLevel === 'all';
  }
  // Canal: hace falta el 'all' EXPLÍCITO; `undefined` no vale.
  return channel.notifyLevel === 'all';
}

/** @deprecated Nombre anterior, cuando la política solo cubría los DM. */
export const shouldNotifyDirect = shouldNotifyNewMessage;
