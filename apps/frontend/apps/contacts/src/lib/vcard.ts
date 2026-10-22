/**
 * Construccion y lectura de cuerpos vCard 4.0 para crear/editar contactos via
 * CardDAV PUT. El SDK @sytue/api-clients/carddav proyecta los campos basicos
 * (emails/telefonos planos); aqui generamos la vCard de salida con campos
 * tipados (etiqueta + principal), direccion postal (ADR) y cumpleanos (BDAY), y
 * leemos esos mismos campos de la vCard cruda para poblar el editor (RFC 6350).
 * Escapado de caracteres especiales.
 */

/** Valor con etiqueta TYPE y marca de principal (EMAIL/TEL). */
export interface TypedValue {
  value: string;
  type: string;
  primary?: boolean | undefined;
}

/** Direccion postal estructurada (ADR, RFC 6350 6.3.1). */
export interface PostalAddress {
  type: string;
  street?: string | undefined;
  city?: string | undefined;
  region?: string | undefined;
  postalCode?: string | undefined;
  country?: string | undefined;
  primary?: boolean | undefined;
}

export interface ContactInput {
  uid: string;
  fullName: string;
  /** Nombre estructurado opcional: apellidos;nombre. */
  lastName?: string | undefined;
  firstName?: string | undefined;
  /** Correos/telefonos planos (fusion/importacion). */
  emails: string[];
  phones: string[];
  /** Correos/telefonos con etiqueta y principal (editor): ganan a los planos. */
  emailsDetailed?: TypedValue[] | undefined;
  phonesDetailed?: TypedValue[] | undefined;
  /**
   * Direcciones postales (ADR). Definido —aunque sea []— = el editor gestiona
   * ADR: se regeneran desde aqui y NO se conserva la ADR de `baseVcard`.
   * Ausente = no se toca la ADR de `baseVcard` (fusion/importacion).
   */
  addresses?: PostalAddress[] | undefined;
  /**
   * Cumpleanos YYYY-MM-DD. Definido —aunque sea ''— = el editor gestiona BDAY.
   * Ausente = se conserva el BDAY de `baseVcard` (p. ej. fechas sin año que el
   * <input type="date"> no puede representar): asi el round-trip no lo pierde.
   */
  birthday?: string | undefined;
  organization?: string | undefined;
  title?: string | undefined;
  note?: string | undefined;
  /**
   * vCard cruda existente (al editar/fusionar). Sus propiedades NO gestionadas
   * (PHOTO/NICKNAME/URL/IMPP/X-*, y ADR/BDAY si el editor no las gestiona) se
   * conservan verbatim para que el round-trip por CardDAV no las pierda.
   */
  baseVcard?: string | undefined;
}

function escapeText(value: string): string {
  return value
    .replace(/\\/g, '\\\\')
    .replace(/;/g, '\\;')
    .replace(/,/g, '\\,')
    .replace(/\n/g, '\\n');
}

// Propiedades que buildVcard SIEMPRE regenera desde el ContactInput: NO se
// copian de la vCard base (se tomarían dos veces / con valor viejo). ADR y BDAY
// NO están aquí: se gestionan de forma condicional (ver buildVcard) para que la
// fusión/importación —que no las edita— las conserve verbatim.
const MANAGED_PROPS = new Set([
  'BEGIN',
  'END',
  'VERSION',
  'UID',
  'FN',
  'N',
  'PRODID',
  'EMAIL',
  'TEL',
  'ORG',
  'TITLE',
  'NOTE',
  'REV',
]);

/** Nombre de propiedad de una línea (sin prefijo de grupo `item1.`), en mayúsculas. */
function propName(line: string): string {
  const match = /^([A-Za-z0-9.-]+)[;:]/.exec(line);
  if (!match) return '';
  return ((match[1] ?? '').split('.').pop() ?? '').toUpperCase();
}

/**
 * Líneas de propiedad de `baseVcard` que buildVcard no gestiona, para no
 * perderlas al reconstruir la tarjeta desde la proyección. Desdobla primero las
 * líneas plegadas (RFC 6350 3.2) para no partir valores largos (p. ej. PHOTO en
 * data URI) y conserva el prefijo de grupo ("item1.ADR" ↔ "item1.X-ABLabel").
 */
export function preservedLines(baseVcard: string): string[] {
  const unfolded = baseVcard.replace(/\r?\n[ \t]/g, '');
  const out: string[] = [];
  for (const line of unfolded.split(/\r?\n/)) {
    const prop = propName(line);
    if (!prop || MANAGED_PROPS.has(prop)) continue;
    out.push(line);
  }
  return out;
}

/** Normaliza una etiqueta TYPE a un token vCard simple; usa `fallback` si falta. */
function vcardType(type: string | undefined, fallback: string): string {
  const t = (type ?? '').toLowerCase().replace(/[^a-z]/g, '');
  return t || fallback;
}

function isEmptyAddress(a: PostalAddress): boolean {
  return !(a.street || a.city || a.region || a.postalCode || a.country);
}

/** Genera la vCard 4.0 para un PUT CardDAV. */
export function buildVcard(input: ContactInput): string {
  const lines = [
    'BEGIN:VCARD',
    'VERSION:4.0',
    `UID:${input.uid}`,
    `FN:${escapeText(input.fullName)}`,
    `N:${escapeText(input.lastName ?? '')};${escapeText(input.firstName ?? '')};;;`,
    'PRODID:-//Sytue One//Sytue Contacts//ES',
  ];

  // EMAIL/TEL: tipados si el editor los aporta; si no, fallback plano (TYPE por
  // defecto, para conservar el comportamiento de fusión/importación).
  const emails: TypedValue[] =
    input.emailsDetailed ?? input.emails.filter(Boolean).map((value) => ({ value, type: 'work' }));
  for (const e of emails) {
    if (!e.value.trim()) continue;
    lines.push(
      `EMAIL;TYPE=${vcardType(e.type, 'home')}${e.primary ? ';PREF=1' : ''}:${escapeText(e.value.trim())}`,
    );
  }
  const phones: TypedValue[] =
    input.phonesDetailed ?? input.phones.filter(Boolean).map((value) => ({ value, type: 'cell' }));
  for (const p of phones) {
    if (!p.value.trim()) continue;
    lines.push(
      `TEL;TYPE=${vcardType(p.type, 'cell')}${p.primary ? ';PREF=1' : ''}:${escapeText(p.value.trim())}`,
    );
  }

  // ADR (RFC 6350 6.3.1): apartado;ext;calle;ciudad;region;CP;pais.
  for (const a of input.addresses ?? []) {
    if (isEmptyAddress(a)) continue;
    const parts = ['', '', a.street, a.city, a.region, a.postalCode, a.country].map((v) =>
      escapeText(v ?? ''),
    );
    lines.push(
      `ADR;TYPE=${vcardType(a.type, 'home')}${a.primary ? ';PREF=1' : ''}:${parts.join(';')}`,
    );
  }

  if (input.birthday) lines.push(`BDAY:${input.birthday}`); // YYYY-MM-DD
  if (input.organization) lines.push(`ORG:${escapeText(input.organization)}`);
  if (input.title) lines.push(`TITLE:${escapeText(input.title)}`);
  if (input.note) lines.push(`NOTE:${escapeText(input.note)}`);

  // Conserva de la base las propiedades no gestionadas. ADR/BDAY solo se
  // conservan si el editor NO las gestiona (campo ausente): así la fusión/
  // importación no pierde direcciones ni cumpleaños que no tocó, y el editor sí
  // los reemplaza sin dejar duplicados.
  if (input.baseVcard) {
    let preserved = preservedLines(input.baseVcard);
    if (input.addresses !== undefined) preserved = preserved.filter((l) => propName(l) !== 'ADR');
    if (input.birthday !== undefined) preserved = preserved.filter((l) => propName(l) !== 'BDAY');
    lines.push(...preserved);
  }

  lines.push('END:VCARD');
  // CRLF segun RFC 6350.
  return lines.join('\r\n') + '\r\n';
}

/** Campos tipados leídos de una vCard cruda, para poblar el editor. */
export interface ContactDetails {
  firstName: string;
  lastName: string;
  emails: TypedValue[];
  phones: TypedValue[];
  addresses: PostalAddress[];
  note: string;
  /** Cumpleaños representable como fecha completa (YYYY-MM-DD), o ''. */
  birthday: string;
  /**
   * true si el editor puede gestionar el BDAY sin pérdida (no hay BDAY, o es una
   * fecha completa). false si hay un BDAY parcial (p. ej. `--MM-DD`, sin año)
   * que el <input type="date"> no representa y debe conservarse verbatim.
   */
  birthdayManaged: boolean;
}

interface ParsedLine {
  prop: string;
  params: string[];
  value: string;
}

function parseLine(line: string): ParsedLine | null {
  const colon = line.indexOf(':');
  if (colon < 0) return null;
  const head = line.slice(0, colon);
  const value = line.slice(colon + 1);
  const segs = head.split(';');
  const prop = ((segs[0] ?? '').split('.').pop() ?? '').toUpperCase();
  if (!prop) return null;
  return { prop, params: segs.slice(1), value };
}

/** Primer TYPE (en minúsculas, ignorando `pref`) de los parámetros, o `fallback`. */
function labelOf(params: string[], fallback: string): string {
  for (const p of params) {
    const [k, v] = p.split('=');
    if ((k ?? '').toUpperCase() === 'TYPE' && v) {
      const t = v
        .split(',')
        .map((x) => x.trim().toLowerCase())
        .find((x) => x && x !== 'pref');
      if (t) return t;
    }
  }
  return fallback;
}

function isPref(params: string[]): boolean {
  for (const p of params) {
    const [k, v] = p.split('=');
    const key = (k ?? '').toUpperCase();
    if (key === 'PREF' && (v ?? '') === '1') return true;
    if (key === 'TYPE' && (v ?? '').split(',').some((x) => x.trim().toLowerCase() === 'pref')) {
      return true;
    }
  }
  return false;
}

/** BDAY -> YYYY-MM-DD si es una fecha completa; '' en otro caso (sin año, etc.). */
function toISODate(v: string): string {
  if (/^\d{4}-\d{2}-\d{2}$/.test(v)) return v;
  if (/^\d{8}$/.test(v)) return `${v.slice(0, 4)}-${v.slice(4, 6)}-${v.slice(6, 8)}`;
  const m = /^(\d{4}-\d{2}-\d{2})T/.exec(v);
  return m?.[1] ?? '';
}

function unescapeText(v: string): string {
  return v.replace(/\\([\\,;nN])/g, (_, c: string) => (c === 'n' || c === 'N' ? '\n' : c));
}

/**
 * Lee de una vCard cruda los campos tipados que el editor edita (nombre
 * estructurado, correos/teléfonos con etiqueta y principal, direcciones,
 * cumpleaños, nota). Desdobla las líneas plegadas y respeta prefijos de grupo.
 */
export function parseVcardDetails(vcard: string): ContactDetails {
  const unfolded = vcard.replace(/\r?\n[ \t]/g, '');
  const emails: TypedValue[] = [];
  const phones: TypedValue[] = [];
  const addresses: PostalAddress[] = [];
  let firstName = '';
  let lastName = '';
  let note = '';
  let birthday = '';
  let birthdayManaged = true;

  for (const line of unfolded.split(/\r?\n/)) {
    const parsed = parseLine(line);
    if (!parsed) continue;
    const { prop, params, value } = parsed;
    if (prop === 'N') {
      const parts = value.split(';');
      lastName = unescapeText(parts[0] ?? '');
      firstName = unescapeText(parts[1] ?? '');
    } else if (prop === 'EMAIL' && value.trim()) {
      emails.push({
        value: unescapeText(value.trim()),
        type: labelOf(params, 'home'),
        primary: isPref(params),
      });
    } else if (prop === 'TEL' && value.trim()) {
      phones.push({
        value: unescapeText(value.trim()),
        type: labelOf(params, 'cell'),
        primary: isPref(params),
      });
    } else if (prop === 'ADR') {
      const c = value.split(';').map(unescapeText);
      const addr: PostalAddress = {
        type: labelOf(params, 'home'),
        street: c[2] ?? '',
        city: c[3] ?? '',
        region: c[4] ?? '',
        postalCode: c[5] ?? '',
        country: c[6] ?? '',
        primary: isPref(params),
      };
      if (!isEmptyAddress(addr)) addresses.push(addr);
    } else if (prop === 'BDAY') {
      const iso = toISODate(value.trim());
      if (iso) birthday = iso;
      else birthdayManaged = false; // BDAY parcial: conservar verbatim (no editable)
    } else if (prop === 'NOTE' && !note) {
      note = unescapeText(value);
    }
  }

  return { firstName, lastName, emails, phones, addresses, note, birthday, birthdayManaged };
}

/** Genera un UID estable para un contacto nuevo. */
export function newUid(): string {
  const rand = globalThis.crypto?.randomUUID?.() ?? Math.random().toString(36).slice(2);
  return `${rand}@sytue.eu`;
}
