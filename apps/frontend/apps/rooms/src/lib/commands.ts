/**
 * Comandos de barra («slash commands») integrados de Sytue Rooms (Ola 14).
 *
 * Registro + parseo PUROS y testeables. Al escribir en el composer un mensaje que
 * empieza por «/», se ofrece un autocompletado (nombre + uso + descripción) y, al
 * ejecutar, se despacha la acción correspondiente EN VEZ de enviar el texto. La
 * DISPATCH (efectos: presencia, DM, tema, búsqueda…) vive en ChatApp, que tiene las
 * dependencias reales; aquí solo el registro y el parseo, sin efectos.
 *
 * El registro es DATA extensible: la Ola 15 (integraciones) añadirá comandos de
 * SERVIDOR (custom, POST a una URL) reusando esta misma estructura —marcando la
 * entrada como remota y resolviendo su acción contra el backend— sin tocar el parser.
 * Aquí NO se implementan URLs externas.
 */

/**
 * Una entrada del registro de comandos. Solo lleva lo ESTRUCTURAL (nombre canónico y
 * alias): la descripción y el uso son texto de interfaz y viven en el catálogo, bajo
 * `command.<nombre>.description` y `command.<nombre>.usage` (el nombre es la clave).
 */
export interface SlashCommand {
  /** Nombre canónico, sin «/» (p. ej. 'shrug'). */
  name: CommandName;
  /** Nombres alternativos que resuelven al mismo comando (p. ej. 'dm' → 'msg'). */
  aliases?: string[];
}

/**
 * Nombres de los comandos INTEGRADOS. Es una unión y no `string` a propósito: la
 * descripción y el uso viven en el catálogo bajo `command.<nombre>.description` /
 * `.usage`, así que con la unión el compilador comprueba que las 24 claves existen
 * —y que no sobra ninguna— en vez de dejarlas resolverse en ejecución.
 * Cuando entren comandos de SERVIDOR (Ola 15) llevarán su propio tipo: su texto
 * viene del backend, no del catálogo.
 */
export type CommandName =
  | 'shrug'
  | 'me'
  | 'away'
  | 'online'
  | 'dnd'
  | 'status'
  | 'leave'
  | 'topic'
  | 'purpose'
  | 'invite'
  | 'remove'
  | 'msg'
  | 'search'
  | 'help';

/**
 * Registro de comandos integrados (cliente). El orden es el del autocompletado y
 * «/help». Extensible a comandos de servidor en la Ola 15 sin cambiar el parser.
 */
export const COMMANDS: SlashCommand[] = [
  { name: 'shrug' },
  { name: 'me' },
  { name: 'away' },
  { name: 'online' },
  { name: 'dnd' },
  { name: 'status' },
  { name: 'leave' },
  { name: 'topic', aliases: ['header'] },
  { name: 'purpose' },
  { name: 'invite' },
  { name: 'remove', aliases: ['kick'] },
  { name: 'msg', aliases: ['dm'] },
  { name: 'search' },
  { name: 'help' },
];

/** Comando reconocido con su argumento (todo lo que sigue al nombre, sin recortar). */
export interface ParsedCommand {
  kind: 'command';
  /** Nombre CANÓNICO del comando (los alias se resuelven a él). */
  name: CommandName;
  /** Argumento crudo (lo que sigue al nombre; recórtalo según el comando). */
  arg: string;
  command: SlashCommand;
}

/** Intento de comando que no coincide con ninguno del registro. */
export interface UnknownCommand {
  kind: 'unknown';
  /** Nombre tal cual se escribió (para el aviso «comando no reconocido»). */
  name: string;
}

/**
 * Comando de SERVIDOR: lo registró la organización y lo atiende un servicio de un
 * tercero. Su nombre y su texto de ayuda vienen del backend, no del catálogo: los
 * escribió quien lo registró.
 */
export interface RemoteCommand {
  name: string;
  description?: string;
  usageHint?: string;
}

/** Un comando de servidor reconocido, con su argumento crudo. */
export interface ParsedRemoteCommand {
  kind: 'remote';
  name: string;
  arg: string;
  command: RemoteCommand;
}

/** Resultado del parseo: integrado, de servidor, desconocido, o null (texto normal). */
export type CommandParse = ParsedCommand | ParsedRemoteCommand | UnknownCommand | null;

// Un intento de comando: «/» + una LETRA + palabra, al principio absoluto. Empezar
// por letra deja escapar rutas/urls como «//x» o «/ x» (se envían como texto normal).
const COMMAND_RE = /^\/([a-zA-Z][\w-]*)\s?([\s\S]*)$/;

/** Localiza un comando por nombre o alias (sin distinción de mayúsculas). */
export function findCommand(name: string): SlashCommand | undefined {
  const n = name.toLowerCase();
  return COMMANDS.find((c) => c.name === n || c.aliases?.includes(n));
}

/**
 * Parsea el texto del composer como comando. Devuelve:
 * - null: no es un intento de comando (envíalo como texto normal).
 * - {kind:'unknown', name}: empieza por «/palabra» pero no coincide (avisa, no envíes).
 * - {kind:'command', …}: integrado (despáchalo aquí).
 * - {kind:'remote', …}: de servidor (mándalo al backend, que llama al tercero).
 *
 * Los INTEGRADOS ganan: si la organización registrara un «/help», el suyo quedaría
 * inalcanzable. Por eso el backend rechaza registrar un nombre que ya sea integrado —lo
 * contrario dejaría un comando muerto sin pista de por qué.
 */
export function parseCommand(input: string, remote: RemoteCommand[] = []): CommandParse {
  const m = COMMAND_RE.exec(input);
  if (!m) return null;
  const raw = m[1]!;
  const arg = m[2] ?? '';
  const command = findCommand(raw);
  if (command) return { kind: 'command', name: command.name, arg, command };
  const r = remote.find((c) => c.name === raw.toLowerCase());
  if (r) return { kind: 'remote', name: r.name, arg, command: r };
  return { kind: 'unknown', name: raw };
}

/**
 * ¿El valor es una PALABRA de comando aún en curso (para el autocompletado)? Es
 * decir, «/» seguido solo de [a-z0-9-] sin espacio todavía. Devuelve la query (que
 * puede ser '' para «/»), o null si no procede el autocompletado de comandos.
 */
export function commandAutocompleteQuery(value: string): string | null {
  const m = /^\/([a-z0-9-]*)$/i.exec(value);
  return m ? m[1]!.toLowerCase() : null;
}

/** Comandos cuyo nombre o alias empieza por la query (para el autocompletado). */
export function matchCommands(query: string): SlashCommand[] {
  const q = query.toLowerCase();
  if (!q) return COMMANDS;
  return COMMANDS.filter((c) => c.name.startsWith(q) || c.aliases?.some((a) => a.startsWith(q)));
}

/**
 * Comandos de SERVIDOR que casan con la query, para el mismo autocompletado.
 *
 * Van detrás de los integrados y sin alias: los nombres los pone quien registra el
 * comando, y no hay catálogo que traduzca su ayuda —viene tal cual del backend.
 */
export function matchRemoteCommands(query: string, remote: RemoteCommand[]): RemoteCommand[] {
  const q = query.toLowerCase();
  return q ? remote.filter((c) => c.name.startsWith(q)) : remote;
}

// ---------------------------------------------------------------------------
// Transformaciones de cuerpo (puras)
// ---------------------------------------------------------------------------

/** El shrug clásico. */
const SHRUG = '¯\\_(ツ)_/¯';

/** Cuerpo del comando /shrug: «texto ¯\\_(ツ)_/¯» (o solo el shrug si no hay texto). */
export function shrugBody(text: string): string {
  const t = text.trim();
  return t ? `${t} ${SHRUG}` : SHRUG;
}

/**
 * Prefijo de un mensaje de emote (/me). Convención de cuerpo (no un tipo aparte):
 * se guarda «/me <acción>» y MessageBody lo pinta como «* Autor acción» en cursiva.
 * Degrada de forma limpia en cualquier render que no lo entienda (muestra «/me …»).
 */
export const EMOTE_PREFIX = '/me ';

/** Cuerpo de un emote a partir de la acción. */
export function emoteBody(action: string): string {
  return EMOTE_PREFIX + action.trim();
}

const EMOTE_RE = /^\/me\s+([\s\S]+)$/;

/** Si el cuerpo es un emote (/me acción), devuelve la acción; si no, null. Puro. */
export function parseEmote(body: string): string | null {
  const m = EMOTE_RE.exec(body);
  return m ? m[1]! : null;
}
