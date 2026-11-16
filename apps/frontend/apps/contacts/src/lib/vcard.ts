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
  /**
   * Nombre estructurado (N: apellidos;nombre). Si llega alguno de los dos, se
   * regenera N desde aqui; si faltan ambos, se conserva el N de `baseVcard`
   * (asi editar el telefono de un contacto de Google o del iPhone no le borra
   * los apellidos ni el orden por apellido en los clientes CardDAV).
   */
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
  /**
   * Organizacion. Ausente = se conserva el ORG de `baseVcard`. Definida —aunque
   * sea ''— = la gestiona quien llama; pero si coincide con lo que el SDK
   * proyecta del ORG de la base (que aplana `Acme;Ventas` a `Acme Ventas`), se
   * conserva la linea original con sus componentes.
   */
  organization?: string | undefined;
  /** Cargo (TITLE). Ausente = se conserva el de `baseVcard`; '' lo borra. */
  title?: string | undefined;
  /**
   * Apodo (NICKNAME), como lo escribe la persona: si trae comas, son varios
   * apodos (RFC 6350 6.2.3, lista separada por comas). Ausente = se conserva
   * el de `baseVcard`; '' lo borra.
   */
  nickname?: string | undefined;
  note?: string | undefined;
  /**
   * Sitios web (URL http/https). Definido —aunque sea []— = el editor los gestiona:
   * se regeneran desde aquí y de la base solo se conservan los URL que no son un
   * sitio web (ftp:, sin esquema…), que la ficha no enseña. Ausente = no se tocan.
   * Lo inválido se descarta (`normalizeWebsite`): la ficha los abre con un clic.
   */
  urls?: TypedValue[] | undefined;
  /**
   * Personas relacionadas (RELATED de texto: `value` es el nombre, `type` el tipo de
   * relación de RFC 6350 o ''). Mismo trato que `urls`: de la base se conservan las
   * referencias a otra tarjeta (`urn:uuid:`), que no son un nombre.
   */
  related?: TypedValue[] | undefined;
  /** Aniversario YYYY-MM-DD; mismo trato que `birthday` (ausente = se conserva). */
  anniversary?: string | undefined;
  /**
   * vCard cruda existente (al editar/fusionar). Sus propiedades NO gestionadas
   * (PHOTO/URL/IMPP/X-*, y ADR/BDAY/TITLE/NICKNAME si el editor no las gestiona) se
   * conservan verbatim para que el round-trip por CardDAV no las pierda.
   */
  baseVcard?: string | undefined;
}

/** Apodos de lo que escribe la persona: separados por comas, sin vacíos. */
function splitNicknames(value: string): string[] {
  return value
    .split(',')
    .map((n) => n.trim())
    .filter(Boolean);
}

function escapeText(value: string): string {
  return value
    .replace(/\\/g, '\\\\')
    .replace(/;/g, '\\;')
    .replace(/,/g, '\\,')
    .replace(/\n/g, '\\n');
}

// Propiedades que buildVcard SIEMPRE regenera desde el ContactInput: NO se
// copian de la vCard base (se tomarían dos veces / con valor viejo). ADR, BDAY,
// N, ORG, TITLE y NICKNAME NO están aquí: se gestionan de forma condicional (ver
// buildVcard) para que quien no las edita —el editor no tiene campo de
// apellidos y no manda el cargo ni el apodo si no cambiaron; la
// fusión/importación no toca ninguna— las conserve verbatim.
const MANAGED_PROPS = new Set([
  'BEGIN',
  'END',
  'VERSION',
  'UID',
  'FN',
  'PRODID',
  'EMAIL',
  'TEL',
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

/** Valor (crudo) de cada línea `prop` de una vCard ya desdoblada en líneas. */
function rawValues(lines: string[], prop: string): string[] {
  return lines.filter((l) => propName(l) === prop).map((l) => l.slice(l.indexOf(':') + 1));
}

/**
 * true si `organization` es lo que la lista proyecta del ORG crudo de la base.
 * El SDK CardDAV sustituye los `;` por espacios sin desescapar; se acepta
 * también la grafía desescapada para no reescribir un ORG que nadie tocó.
 */
function sameOrganization(raw: string, organization: string): boolean {
  const target = organization.trim();
  const sdk = raw.replace(/;/g, ' ').trim();
  const unescaped = raw
    .split(/(?<!\\);/)
    .map(unescapeText)
    .join(' ')
    .trim();
  return target === sdk || target === unescaped;
}

/**
 * Sitio web escrito por una persona -> dirección completa, o null si no vale. Sin
 * esquema se le pone «https://»; solo http y https, sin usuario (en
 * «https://banco.example@otro.example» se lee un sitio y se abre otro). Es
 * `NormalizeURL` del servicio: lo que uno acepta, el otro también.
 */
export function normalizeWebsite(raw: string): string | null {
  const v = raw.trim();
  if (!v || v.length > 2048 || /\s/.test(v)) return null;
  const full = v.includes('://') || /^[A-Za-z][A-Za-z0-9+.-]*:[^0-9]/.test(v) ? v : `https://${v}`;
  try {
    const u = new URL(full);
    if ((u.protocol !== 'http:' && u.protocol !== 'https:') || !u.hostname) return null;
    if (u.username || u.password) return null;
  } catch {
    return null;
  }
  return full;
}

/** true si el URL de la vCard es un sitio web que la ficha enseña (y el editor edita). */
function isWebsite(value: string): boolean {
  return /^https?:\/\//i.test(value) && normalizeWebsite(value) === value;
}

/** Tipos de relación de RFC 6350 §6.6.6 (registro de la IANA). */
const RELATED_TYPES = new Set([
  'contact',
  'acquaintance',
  'friend',
  'met',
  'co-worker',
  'colleague',
  'co-resident',
  'neighbor',
  'child',
  'parent',
  'sibling',
  'spouse',
  'kin',
  'muse',
  'crush',
  'date',
  'sweetheart',
  'me',
  'agent',
  'emergency',
]);

/** Primer TYPE de relación conocido de los parámetros de un RELATED, o ''. */
function relatedTypeOf(params: ParsedParameter[]): string {
  for (const p of params) {
    if (p.name !== 'TYPE') continue;
    const t = p.values
      .map((x) => x.replace(/"/g, '').trim().toLowerCase())
      .find((x) => RELATED_TYPES.has(x));
    if (t) return t;
  }
  return '';
}

/** true si un RELATED es de texto (un nombre) y no una referencia a otra tarjeta. */
function isRelatedText(params: ParsedParameter[], value: string): boolean {
  const text = params.some(
    (p) => p.name === 'VALUE' && p.values.some((v) => /^text$/i.test(v.trim())),
  );
  return value.trim() !== '' && (text || !/^[A-Za-z][A-Za-z0-9+.-]*:\S*$/.test(value.trim()));
}

/** Genera la vCard 4.0 para un PUT CardDAV. */
export function buildVcard(input: ContactInput): string {
  const base = input.baseVcard ? preservedLines(input.baseVcard) : [];
  const nManaged = input.firstName !== undefined || input.lastName !== undefined;
  const baseN = base.filter((l) => propName(l) === 'N');
  const lines = [
    'BEGIN:VCARD',
    'VERSION:4.0',
    `UID:${input.uid}`,
    `FN:${escapeText(input.fullName)}`,
    // N es obligatorio en vCard 3: sin N en la base ni en el input, uno vacío.
    ...(nManaged || baseN.length === 0
      ? [`N:${escapeText(input.lastName ?? '')};${escapeText(input.firstName ?? '')};;;`]
      : baseN),
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
  if (input.anniversary) lines.push(`ANNIVERSARY:${input.anniversary}`); // YYYY-MM-DD
  for (const u of input.urls ?? []) {
    const url = normalizeWebsite(u.value);
    if (!url) continue;
    const type = ['home', 'work'].includes(u.type) ? `;TYPE=${u.type}` : '';
    lines.push(`URL${type}:${url}`);
  }
  // VALUE=text: sin él, un lector de vCard 4 toma el nombre por una URI.
  for (const r of input.related ?? []) {
    const name = r.value.trim().replace(/\r?\n/g, ' ');
    if (!name) continue;
    const type = RELATED_TYPES.has(r.type) ? `;TYPE=${r.type}` : '';
    lines.push(`RELATED;VALUE=text${type}:${escapeText(name)}`);
  }
  // ORG intacto (misma proyección que la de la base) = no gestionado: se
  // conserva la línea original con sus componentes (`Acme;Ventas`).
  const baseOrg = rawValues(base, 'ORG');
  const org = input.organization;
  const orgManaged = org !== undefined && !baseOrg.some((raw) => sameOrganization(raw, org));
  if (orgManaged && input.organization) lines.push(`ORG:${escapeText(input.organization)}`);
  if (input.title) lines.push(`TITLE:${escapeText(input.title)}`);
  const nicknames = splitNicknames(input.nickname ?? '');
  // Cada apodo se escapa por separado y se unen con comas SIN escapar: así
  // «Pepe, Pepito» es una lista de dos, como la escriben Apple y Google.
  if (nicknames.length) lines.push(`NICKNAME:${nicknames.map(escapeText).join(',')}`);
  if (input.note) lines.push(`NOTE:${escapeText(input.note)}`);

  // Conserva de la base las propiedades no gestionadas. ADR/BDAY/N/ORG/TITLE/NICKNAME
  // solo se conservan si quien llama NO las gestiona (campo ausente): así la
  // fusión/importación y el editor no pierden lo que no tocaron, y lo que sí
  // gestionan se reemplaza sin dejar duplicados.
  const managed = new Set(['N']);
  if (input.addresses !== undefined) managed.add('ADR');
  if (input.birthday !== undefined) managed.add('BDAY');
  if (input.anniversary !== undefined) managed.add('ANNIVERSARY');
  // URL y RELATED gestionados: fuera de la base solo lo que la ficha enseña.
  const hidden = (l: string): boolean => {
    const parsed = parseLine(l);
    if (!parsed) return true;
    if (parsed.prop === 'URL' && input.urls !== undefined) {
      return !isWebsite(unescapeText(parsed.value.trim()));
    }
    if (parsed.prop === 'RELATED' && input.related !== undefined) {
      return !isRelatedText(parsed.params, parsed.value);
    }
    return true;
  };
  if (orgManaged) managed.add('ORG');
  if (input.title !== undefined) managed.add('TITLE');
  if (input.nickname !== undefined) managed.add('NICKNAME');
  lines.push(...base.filter((l) => !managed.has(propName(l)) && hidden(l)));

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
  /** Cargo (primer TITLE), o ''. */
  title: string;
  /**
   * Apodos (primer NICKNAME) unidos por «, », que es también como el editor
   * los devuelve a buildVcard: el round-trip no los funde en uno.
   */
  nickname: string;
  /** Cumpleaños representable como fecha completa (YYYY-MM-DD), o ''. */
  birthday: string;
  /**
   * true si el editor puede gestionar el BDAY sin pérdida (no hay BDAY, o es una
   * fecha completa). false si hay un BDAY parcial (p. ej. `--MM-DD`, sin año)
   * que el <input type="date"> no representa y debe conservarse verbatim.
   */
  birthdayManaged: boolean;
  /** Sitios web http(s), con su TYPE (home/work u other). */
  urls: TypedValue[];
  /** Personas relacionadas de texto: `value` el nombre, `type` la relación o ''. */
  related: TypedValue[];
  /** Aniversario como fecha completa (YYYY-MM-DD), o ''. */
  anniversary: string;
  /** Como `birthdayManaged`, para un ANNIVERSARY parcial que hay que conservar. */
  anniversaryManaged: boolean;
}

interface ParsedLine {
  prop: string;
  params: ParsedParameter[];
  value: string;
}

interface ParsedParameter {
  name: string;
  values: string[];
}

// RFC 6868 escapes apply to parameter values, not to property text. Decode
// after finding the delimiters so ^' cannot become a syntactic quote.
function unescapeParameter(value: string): string {
  return value.replace(/\^(\^|n|')/g, (_, c: string) => {
    if (c === 'n') return '\n';
    return c === "'" ? '"' : '^';
  });
}

function parameterValues(raw: string): string[] {
  const values: string[] = [];
  let start = 0;
  let quoted = false;
  for (let i = 0; i <= raw.length; i++) {
    const char = raw[i];
    if (char === '"') quoted = !quoted;
    if (i === raw.length || (char === ',' && !quoted)) {
      let value = raw.slice(start, i).trim();
      if (value.startsWith('"') && value.endsWith('"')) value = value.slice(1, -1);
      values.push(unescapeParameter(value));
      start = i + 1;
    }
  }
  return values;
}

function parseLine(line: string): ParsedLine | null {
  let quoted = false;
  let start = 0;
  let colon = -1;
  const segs: string[] = [];
  for (let i = 0; i < line.length; i++) {
    const char = line[i];
    if (char === '"') quoted = !quoted;
    else if (!quoted && char === ';') {
      segs.push(line.slice(start, i));
      start = i + 1;
    } else if (!quoted && char === ':') {
      segs.push(line.slice(start, i));
      colon = i;
      break;
    }
  }
  if (colon < 0) return null;
  const value = line.slice(colon + 1);
  const prop = ((segs[0] ?? '').split('.').pop() ?? '').toUpperCase();
  if (!prop) return null;
  const params: ParsedParameter[] = [];
  for (const segment of segs.slice(1)) {
    const equals = segment.indexOf('=');
    if (equals < 0) continue;
    params.push({
      name: segment.slice(0, equals).toUpperCase(),
      values: parameterValues(segment.slice(equals + 1)),
    });
  }
  return { prop, params, value };
}

/** Primer TYPE (en minúsculas, ignorando `pref`) de los parámetros, o `fallback`. */
function labelOf(params: ParsedParameter[], fallback: string): string {
  for (const p of params) {
    if (p.name === 'TYPE') {
      const t = p.values.map((x) => x.trim().toLowerCase()).find((x) => x && x !== 'pref');
      if (t) return t;
    }
  }
  return fallback;
}

function isPref(params: ParsedParameter[]): boolean {
  for (const p of params) {
    if (p.name === 'PREF' && p.values.some((x) => x === '1')) return true;
    if (p.name === 'TYPE' && p.values.some((x) => x.trim().toLowerCase() === 'pref')) {
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

// A separator is structural only when not escaped by an odd number of
// backslashes. Keep escapes intact until each component has been isolated.
function structuredComponents(v: string): string[] {
  const components: string[] = [];
  let start = 0;
  for (let i = 0; i < v.length; i++) {
    if (v[i] === '\\' && i + 1 < v.length) {
      i++;
    } else if (v[i] === ';') {
      components.push(unescapeText(v.slice(start, i)));
      start = i + 1;
    }
  }
  components.push(unescapeText(v.slice(start)));
  return components;
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
  let title = '';
  let nickname = '';
  let birthday = '';
  let birthdayManaged = true;
  const urls: TypedValue[] = [];
  const related: TypedValue[] = [];
  let anniversary = '';
  let anniversaryManaged = true;

  for (const line of unfolded.split(/\r?\n/)) {
    const parsed = parseLine(line);
    if (!parsed) continue;
    const { prop, params, value } = parsed;
    if (prop === 'N') {
      const parts = structuredComponents(value);
      lastName = parts[0] ?? '';
      firstName = parts[1] ?? '';
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
      const c = structuredComponents(value);
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
    } else if (prop === 'ANNIVERSARY' && !anniversary && anniversaryManaged) {
      const iso = toISODate(value.trim());
      if (iso) anniversary = iso;
      else anniversaryManaged = false;
    } else if (prop === 'URL') {
      const url = unescapeText(value.trim());
      if (isWebsite(url)) {
        const type = labelOf(params, 'other');
        urls.push({ value: url, type: ['home', 'work'].includes(type) ? type : 'other' });
      }
    } else if (prop === 'RELATED' && isRelatedText(params, value)) {
      related.push({ value: unescapeText(value.trim()), type: relatedTypeOf(params) });
    } else if (prop === 'NOTE' && !note) {
      note = unescapeText(value);
    } else if (prop === 'TITLE' && !title) {
      title = unescapeText(value).trim();
    } else if (prop === 'NICKNAME' && !nickname) {
      // Lista separada por comas SIN escapar (una `\,` es parte del apodo).
      nickname = value
        .split(/(?<!\\),/)
        .map((n) => unescapeText(n).trim())
        .filter(Boolean)
        .join(', ');
    }
  }

  return {
    firstName,
    lastName,
    emails,
    phones,
    addresses,
    note,
    title,
    nickname,
    birthday,
    birthdayManaged,
    urls,
    related,
    anniversary,
    anniversaryManaged,
  };
}

/**
 * Línea bajo el nombre en la ficha y en la vista previa del formulario:
 * «Cargo · Empresa», o el que haya de los dos ('' si ninguno). Como Apple y
 * Google, el cargo va delante porque es lo que dice quién es esa persona.
 */
export function roleLine(title: string | undefined, organization: string | undefined): string {
  return [title, organization]
    .map((v) => (v ?? '').trim())
    .filter(Boolean)
    .join(' · ');
}

/**
 * Nombre estructurado (N) que debe acompañar a un FN editado en el formulario,
 * que solo tiene el campo «nombre completo». `null` = no tocar N (se conserva el
 * de la base): el nombre no cambió, o no hay base (crear, como siempre).
 *
 * Si cambió, se reaprovecha lo que siga siendo cierto del N anterior para no
 * dejar uno obsoleto ni inventar uno: si el nombre nuevo acaba en los apellidos
 * de antes, esos siguen siendo los apellidos («Ana García» → «Ana María
 * García»); si empieza por el nombre de antes, lo demás son los apellidos
 * («Ana García» → «Ana García López»). Si no casa nada, N vacío (como antes):
 * los clientes CardDAV caen entonces al FN.
 */
export function structuredNameFor(
  fullName: string,
  baseVcard: string | undefined,
): { firstName: string; lastName: string } | null {
  if (!baseVcard) return null;
  const unfolded = baseVcard.replace(/\r?\n[ \t]/g, '');
  let oldFull = '';
  let hasN = false;
  for (const line of unfolded.split(/\r?\n/)) {
    const prop = propName(line);
    if (prop === 'FN' && !oldFull) oldFull = unescapeText(line.slice(line.indexOf(':') + 1)).trim();
    if (prop === 'N') hasN = true;
  }
  const name = fullName.trim();
  if (name === oldFull) return null;
  if (!hasN) return { firstName: '', lastName: '' };
  const { firstName: oldFirst, lastName: oldLast } = parseVcardDetails(baseVcard);
  const first = oldFirst.trim();
  const last = oldLast.trim();
  if (last && name.endsWith(` ${last}`)) {
    return { firstName: name.slice(0, -last.length).trim(), lastName: last };
  }
  if (first && name.startsWith(`${first} `)) {
    return { firstName: first, lastName: name.slice(first.length).trim() };
  }
  return { firstName: '', lastName: '' };
}

/** Genera un UID estable para un contacto nuevo. */
export function newUid(): string {
  const rand = globalThis.crypto.randomUUID();
  return `${rand}@sytue.eu`;
}
