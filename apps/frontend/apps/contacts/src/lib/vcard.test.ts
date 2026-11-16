/** Chequeo de buildVcard: usa la proyección nueva pero conserva los campos no proyectados de la base. */
import { expect, test } from 'bun:test';
import {
  buildVcard,
  newUid,
  preservedLines,
  parseVcardDetails,
  structuredNameFor,
} from './vcard.js';

const BASE =
  [
    'BEGIN:VCARD',
    'VERSION:4.0',
    'UID:old-uid',
    'FN:Ada Vieja',
    'N:Vieja;Ada;;;',
    'EMAIL;TYPE=home:old@x.eu',
    'TEL;TYPE=home:+34 000',
    'ORG:OrgVieja',
    'NICKNAME:Adita',
    'BDAY:1815-12-10',
    'item1.ADR;TYPE=home:;;Calle 1;Madrid;;28001;ES',
    'item1.X-ABLabel:Casa',
    'X-CUSTOM:algo',
    'REV:20200101T000000Z',
    'END:VCARD',
  ].join('\r\n') + '\r\n';

test('newUid genera un identificador aleatorio UUID', () => {
  expect(newUid()).toMatch(/^[0-9a-f-]{36}@sytue\.eu$/i);
});

test('preservedLines: conserva no proyectadas (N/ORG/BDAY/NICKNAME/ADR/X-*), descarta las gestionadas', () => {
  expect(preservedLines(BASE)).toEqual([
    'N:Vieja;Ada;;;',
    'ORG:OrgVieja',
    'NICKNAME:Adita',
    'BDAY:1815-12-10',
    'item1.ADR;TYPE=home:;;Calle 1;Madrid;;28001;ES',
    'item1.X-ABLabel:Casa',
    'X-CUSTOM:algo',
  ]);
});

test('buildVcard con base: proyección nueva + extras conservados, sin datos viejos gestionados', () => {
  const out = buildVcard({
    uid: 'new-uid',
    fullName: 'Ada Lovelace',
    emails: ['ada@sytue.eu'],
    phones: [],
    baseVcard: BASE,
  });
  // Proyección nueva (gestionada):
  expect(out).toContain('UID:new-uid');
  expect(out).toContain('FN:Ada Lovelace');
  expect(out).toContain('EMAIL;TYPE=work:ada@sytue.eu');
  // Extras conservados de la base:
  expect(out).toContain('BDAY:1815-12-10');
  expect(out).toContain('NICKNAME:Adita');
  expect(out).toContain('item1.ADR;TYPE=home:;;Calle 1;Madrid;;28001;ES');
  expect(out).toContain('X-CUSTOM:algo');
  // Los valores viejos de campos gestionados NO se cuelan:
  expect(out).not.toContain('old-uid');
  expect(out).not.toContain('old@x.eu');
  expect(out).not.toContain('Ada Vieja');
  expect(out).not.toContain('REV:20200101');
});

test('buildVcard sin base: solo la proyección (crear/importar)', () => {
  const out = buildVcard({ uid: 'u', fullName: 'X', emails: [], phones: [] });
  expect(out).not.toContain('BDAY');
  expect(out.startsWith('BEGIN:VCARD')).toBe(true);
  expect(out.endsWith('END:VCARD\r\n')).toBe(true);
});

test('preservedLines: desdobla una PHOTO plegada (data URI) en una sola línea', () => {
  const folded =
    'BEGIN:VCARD\r\nVERSION:4.0\r\nFN:X\r\nPHOTO:data:image/png;base64,AAAA\r\n BBBB\r\n CCCC\r\nEND:VCARD\r\n';
  expect(preservedLines(folded)).toEqual(['PHOTO:data:image/png;base64,AAAABBBBCCCC']);
});

test('buildVcard: emite EMAIL/TEL tipados con principal (PREF), ADR y BDAY', () => {
  const out = buildVcard({
    uid: 'u1',
    fullName: 'Grace Hopper',
    firstName: 'Grace',
    lastName: 'Hopper',
    emails: [],
    phones: [],
    emailsDetailed: [
      { value: 'grace@work.eu', type: 'work' },
      { value: 'grace@home.eu', type: 'home', primary: true },
    ],
    phonesDetailed: [{ value: '+34600000001', type: 'cell', primary: true }],
    addresses: [
      {
        type: 'work',
        street: '1 Trafalgar Sq',
        city: 'London',
        postalCode: 'WC2N',
        country: 'UK',
        primary: true,
      },
    ],
    birthday: '1906-12-09',
  });
  expect(out).toContain('EMAIL;TYPE=work:grace@work.eu');
  expect(out).toContain('EMAIL;TYPE=home;PREF=1:grace@home.eu');
  expect(out).toContain('TEL;TYPE=cell;PREF=1:+34600000001');
  expect(out).toContain('ADR;TYPE=work;PREF=1:;;1 Trafalgar Sq;London;;WC2N;UK');
  expect(out).toContain('BDAY:1906-12-09');
});

test('buildVcard: al gestionar ADR/BDAY reemplaza los de la base sin duplicar', () => {
  const base =
    [
      'BEGIN:VCARD',
      'VERSION:4.0',
      'FN:Vieja',
      'ADR;TYPE=home:;;Vieja;Nowhere;;00000;XX',
      'BDAY:1999-01-01',
      'X-CUSTOM:algo',
      'END:VCARD',
    ].join('\r\n') + '\r\n';
  const out = buildVcard({
    uid: 'u1',
    fullName: 'Grace',
    emails: [],
    phones: [],
    addresses: [{ type: 'work', city: 'Madrid' }],
    birthday: '2000-02-02',
    baseVcard: base,
  });
  // Una sola ADR y un solo BDAY (los de la base, gestionados, no se cuelan).
  expect(out.match(/^ADR/gm)?.length ?? 0).toBe(1);
  expect(out.match(/^BDAY/gm)?.length ?? 0).toBe(1);
  expect(out).toContain('ADR;TYPE=work:;;;Madrid;;;');
  expect(out).toContain('BDAY:2000-02-02');
  expect(out).not.toContain('Nowhere');
  expect(out).not.toContain('1999-01-01');
  // Lo no gestionado de la base se conserva.
  expect(out).toContain('X-CUSTOM:algo');
});

test('buildVcard: sin gestionar ADR/BDAY (import/fusión) los conserva de la base', () => {
  const base =
    'BEGIN:VCARD\r\nVERSION:4.0\r\nFN:X\r\nADR;TYPE=home:;;Calle 1;Madrid;;28001;ES\r\nBDAY:--12-10\r\nEND:VCARD\r\n';
  // Sin `addresses`/`birthday` en el input: no se tocan.
  const out = buildVcard({ uid: 'u', fullName: 'X', emails: [], phones: [], baseVcard: base });
  expect(out).toContain('ADR;TYPE=home:;;Calle 1;Madrid;;28001;ES');
  expect(out).toContain('BDAY:--12-10');
});

test('parseVcardDetails: lee tipos, principal, ADR, BDAY y nota; round-trip con buildVcard', () => {
  const card = buildVcard({
    uid: 'u1',
    fullName: 'Grace Hopper',
    firstName: 'Grace',
    lastName: 'Hopper',
    emails: [],
    phones: [],
    emailsDetailed: [
      { value: 'grace@work.eu', type: 'work' },
      { value: 'grace@home.eu', type: 'home', primary: true },
    ],
    phonesDetailed: [{ value: '+34600000001', type: 'fax' }],
    addresses: [
      {
        type: 'work',
        street: 'Calle, con coma',
        city: 'Madrid',
        postalCode: '28001',
        country: 'ES',
      },
    ],
    birthday: '1906-12-09',
    note: 'Con; punto y coma',
  });
  const d = parseVcardDetails(card);
  expect(d.firstName).toBe('Grace');
  expect(d.lastName).toBe('Hopper');
  expect(d.emails).toEqual([
    { value: 'grace@work.eu', type: 'work', primary: false },
    { value: 'grace@home.eu', type: 'home', primary: true },
  ]);
  expect(d.phones[0]).toEqual({ value: '+34600000001', type: 'fax', primary: false });
  expect(d.addresses).toEqual([
    {
      type: 'work',
      street: 'Calle, con coma',
      city: 'Madrid',
      region: '',
      postalCode: '28001',
      country: 'ES',
      primary: false,
    },
  ]);
  expect(d.birthday).toBe('1906-12-09');
  expect(d.birthdayManaged).toBe(true);
  expect(d.note).toBe('Con; punto y coma');
});

test('parseVcardDetails: BDAY sin año no es gestionable (se conservará verbatim)', () => {
  const card = 'BEGIN:VCARD\r\nVERSION:4.0\r\nFN:X\r\nBDAY:--12-10\r\nEND:VCARD\r\n';
  const d = parseVcardDetails(card);
  expect(d.birthday).toBe('');
  expect(d.birthdayManaged).toBe(false);
});

test('parseVcardDetails round-trips escaped separators and backslashes in structured name and address', () => {
  const original = {
    uid: 'private-contact',
    fullName: 'Ada; Doe',
    firstName: 'Ada; Marie',
    lastName: 'Doe\\; Smith',
    emails: [],
    phones: [],
    addresses: [
      {
        type: 'home',
        street: 'Block; 3\\ West',
        city: 'City\\; District',
        region: 'Region, North',
        postalCode: '1;2',
        country: 'E\\U',
        primary: true,
      },
    ],
  };
  const card = buildVcard(original);
  const parsed = parseVcardDetails(card);
  expect(parsed.firstName).toBe(original.firstName);
  expect(parsed.lastName).toBe(original.lastName);
  expect(parsed.addresses).toEqual(original.addresses);
  const reencoded = buildVcard({
    ...original,
    firstName: parsed.firstName,
    lastName: parsed.lastName,
    addresses: parsed.addresses,
  });
  expect(parseVcardDetails(reencoded)).toEqual(parsed);
});

test('parseVcardDetails treats a delimiter following an escaped backslash as a separator', () => {
  // The two literal slashes encode one trailing slash; the following semicolon
  // starts the next component, unlike a single slash before a semicolon.
  const card = String.raw`BEGIN:VCARD
VERSION:4.0
FN:Ada
N:Doe\\;Ada;;;
ADR;TYPE=home:;;Street\\;City;Region;123;ES
END:VCARD`;
  const details = parseVcardDetails(card);
  expect(details.lastName).toBe('Doe\\');
  expect(details.firstName).toBe('Ada');
  expect(details.addresses[0]?.street).toBe('Street\\');
  expect(details.addresses[0]?.city).toBe('City');
});

test('quoted GEO and LABEL parameters do not move address components', () => {
  const address =
    'item1.ADR;GEO="geo:12.3457,78.910";LABEL="HQ: floor; 2";TYPE=work:;;123 Main Street;Madrid;;28001;España';
  const card = `BEGIN:VCARD\r\nVERSION:4.0\r\nFN:X\r\n${address}\r\nEND:VCARD\r\n`;
  const details = parseVcardDetails(card);
  expect(details.addresses).toEqual([
    {
      type: 'work',
      street: '123 Main Street',
      city: 'Madrid',
      region: '',
      postalCode: '28001',
      country: 'España',
      primary: false,
    },
  ]);

  // A caller that does not manage ADR retains the original parameters.
  const preserved = buildVcard({
    uid: 'u',
    fullName: 'X',
    emails: [],
    phones: [],
    baseVcard: card,
  });
  expect(preserved).toContain(address);
  expect(parseVcardDetails(preserved).addresses).toEqual(details.addresses);

  // Rebuilding a managed address preserves its projected fields; it does not
  // promise to preserve GEO, LABEL or other parameters of the original ADR.
  const rebuilt = buildVcard({
    uid: 'u',
    fullName: 'X',
    emails: [],
    phones: [],
    addresses: details.addresses,
    baseVcard: card,
  });
  expect(parseVcardDetails(rebuilt).addresses).toEqual(details.addresses);
});

test('quoted parameters and RFC 6868 escapes do not change content-line boundaries', () => {
  const card = String.raw`BEGIN:VCARD
VERSION:4.0
FN:Ada
N;X-LABEL="Desk^':^nwest;^^":Doe\;Smith;Ada;;;
EMAIL;X-LABEL="a:b;
 c,d";TYPE="work";PREF="1":ada@example.invalid
EMAIL;TYPE="home","pref":ada-home@example.invalid
TEL;TYPE="pref";TYPE="cell":+34123456789
ADR;X-PATH="C:\folder\";TYPE="home":;;Calle\; Mayor;Madrid;;28001;ES
END:VCARD`;
  const details = parseVcardDetails(card);
  expect(details.lastName).toBe('Doe;Smith');
  expect(details.firstName).toBe('Ada');
  expect(details.emails).toEqual([
    { value: 'ada@example.invalid', type: 'work', primary: true },
    { value: 'ada-home@example.invalid', type: 'home', primary: true },
  ]);
  expect(details.phones).toEqual([{ value: '+34123456789', type: 'cell', primary: true }]);
  expect(details.addresses[0]).toMatchObject({
    type: 'home',
    street: 'Calle; Mayor',
    city: 'Madrid',
  });
});

test('quoted commas stay inside one parameter value and caret escapes decode after parsing', () => {
  // Quoted TYPE/PREF are tolerated for interoperability; the vCard 4 TYPE
  // grammar itself specifies tokens. Unknown caret escapes remain unchanged.
  const card =
    `BEGIN:VCARD\r\nVERSION:4.0\r\nFN:X\r\n` +
    `EMAIL;TYPE="custom,pref":one@example.invalid\r\n` +
    `EMAIL;TYPE="x^'^^^n^q":two@example.invalid\r\nEND:VCARD\r\n`;
  expect(parseVcardDetails(card).emails).toEqual([
    { value: 'one@example.invalid', type: 'custom,pref', primary: false },
    { value: 'two@example.invalid', type: 'x"^\n^q', primary: false },
  ]);
});

// CONT-01: un contacto de Google o del iPhone con apellidos, cargo y ORG con
// departamento, editado en la web cambiando solo el teléfono.
const IMPORTADA =
  [
    'BEGIN:VCARD',
    'VERSION:3.0',
    'UID:ana',
    'FN:Ana García',
    'N:García;Ana;;;',
    'TITLE:Directora',
    'ORG:Acme;Ventas',
    'TEL;TYPE=cell:+34 600 000 000',
    'END:VCARD',
  ].join('\r\n') + '\r\n';

/** Lo que manda el editor al guardar (ContactModal.handleSave) con esa base. */
function desdeEditor(fullName: string, organization: string, phone: string): string {
  return buildVcard({
    uid: 'ana',
    fullName,
    emails: [],
    phones: [phone],
    emailsDetailed: [],
    phonesDetailed: [{ value: phone, type: 'cell', primary: true }],
    addresses: [],
    birthday: '',
    // El editor recibe el ORG ya aplanado por el SDK («Acme Ventas»).
    organization,
    ...structuredNameFor(fullName, IMPORTADA),
    baseVcard: IMPORTADA,
  });
}

function lineas(vcard: string, prop: string): string[] {
  return vcard.split('\r\n').filter((l) => l.split(/[;:]/)[0] === prop);
}

test('buildVcard: editar solo el teléfono deja intactos N, TITLE y ORG (con sus componentes)', () => {
  const out = desdeEditor('Ana García', 'Acme Ventas', '+34 611 111 111');
  expect(lineas(out, 'N')).toEqual(['N:García;Ana;;;']);
  expect(lineas(out, 'TITLE')).toEqual(['TITLE:Directora']);
  expect(lineas(out, 'ORG')).toEqual(['ORG:Acme;Ventas']);
  expect(out).toContain('TEL;TYPE=cell;PREF=1:+34 611 111 111');
  expect(out).not.toContain('+34 600 000 000');
});

test('buildVcard: cambiar el nombre completo reescribe FN y reaprovecha los apellidos', () => {
  const out = desdeEditor('Ana María García', 'Acme Ventas', '+34 600 000 000');
  expect(lineas(out, 'FN')).toEqual(['FN:Ana María García']);
  expect(lineas(out, 'N')).toEqual(['N:García;Ana María;;;']);
  expect(lineas(out, 'TITLE')).toEqual(['TITLE:Directora']);
});

test('buildVcard: cambiar o vaciar la organización sí reescribe ORG, sin duplicados', () => {
  expect(lineas(desdeEditor('Ana García', 'Globex', '+34 6'), 'ORG')).toEqual(['ORG:Globex']);
  expect(lineas(desdeEditor('Ana García', '', '+34 6'), 'ORG')).toEqual([]);
});

test('buildVcard: N, TITLE y ORG gestionados explícitamente reemplazan los de la base', () => {
  const out = buildVcard({
    uid: 'ana',
    fullName: 'Ana García',
    firstName: 'Ana',
    lastName: 'García López',
    title: '',
    organization: 'Acme',
    emails: [],
    phones: [],
    baseVcard: IMPORTADA,
  });
  expect(lineas(out, 'N')).toEqual(['N:García López;Ana;;;']);
  expect(lineas(out, 'TITLE')).toEqual([]);
  expect(lineas(out, 'ORG')).toEqual(['ORG:Acme']);
});

test('buildVcard: la fusión/importación (sin N/TITLE/ORG en el input) los conserva de la base', () => {
  const out = buildVcard({
    uid: 'ana',
    fullName: 'Ana García',
    emails: [],
    phones: [],
    baseVcard: IMPORTADA,
  });
  expect(lineas(out, 'N')).toEqual(['N:García;Ana;;;']);
  expect(lineas(out, 'TITLE')).toEqual(['TITLE:Directora']);
  expect(lineas(out, 'ORG')).toEqual(['ORG:Acme;Ventas']);
});

test('buildVcard: sin base ni nombre estructurado sigue emitiendo un N vacío (vCard 3 lo exige)', () => {
  const out = buildVcard({ uid: 'u', fullName: 'X', emails: [], phones: [] });
  expect(lineas(out, 'N')).toEqual(['N:;;;;']);
});

test('structuredNameFor: solo toca N si cambia el nombre completo', () => {
  expect(structuredNameFor('Ana García', IMPORTADA)).toBeNull();
  expect(structuredNameFor('  Ana García ', IMPORTADA)).toBeNull();
  expect(structuredNameFor('Ana nueva', undefined)).toBeNull();
  expect(structuredNameFor('Ana García López', IMPORTADA)).toEqual({
    firstName: 'Ana',
    lastName: 'García López',
  });
  expect(structuredNameFor('Beatriz Ruiz', IMPORTADA)).toEqual({ firstName: '', lastName: '' });
});

test('parseVcardDetails: lee el cargo (TITLE) y los apodos (NICKNAME) desescapados', () => {
  const d = parseVcardDetails(
    [
      'BEGIN:VCARD',
      'VERSION:4.0',
      'FN:Ana García',
      'item1.TITLE:Directora de I+D\\, Europa',
      'NICKNAME:Anita,La Jefa',
      'END:VCARD',
    ].join('\r\n'),
  );
  expect(d.title).toBe('Directora de I+D, Europa');
  expect(d.nickname).toBe('Anita, La Jefa');
  expect(parseVcardDetails(IMPORTADA).nickname).toBe('');
});

test('buildVcard: el editor crea y cambia cargo y apodos, y el round-trip no los pierde', () => {
  const out = buildVcard({
    uid: 'ana',
    fullName: 'Ana García',
    emails: [],
    phones: [],
    title: 'Directora; Ventas',
    nickname: 'Anita, La Jefa',
    baseVcard: IMPORTADA,
  });
  expect(lineas(out, 'TITLE')).toEqual(['TITLE:Directora\\; Ventas']);
  // Varios apodos = lista con comas sin escapar (RFC 6350 6.2.3), uno por elemento.
  expect(lineas(out, 'NICKNAME')).toEqual(['NICKNAME:Anita,La Jefa']);
  const d = parseVcardDetails(out);
  expect(d.title).toBe('Directora; Ventas');
  expect(d.nickname).toBe('Anita, La Jefa');
});

test('buildVcard: sin cargo ni apodo en el input se conservan los de la base; vacíos los borran', () => {
  const base = IMPORTADA.replace('END:VCARD', 'NICKNAME;LANGUAGE=es:Anita\r\nEND:VCARD');
  const sinTocar = buildVcard({
    uid: 'ana',
    fullName: 'Ana García',
    emails: [],
    phones: [],
    baseVcard: base,
  });
  expect(lineas(sinTocar, 'TITLE')).toEqual(['TITLE:Directora']);
  expect(lineas(sinTocar, 'NICKNAME')).toEqual(['NICKNAME;LANGUAGE=es:Anita']);
  const borrados = buildVcard({
    uid: 'ana',
    fullName: 'Ana García',
    emails: [],
    phones: [],
    title: '',
    nickname: ' , ',
    baseVcard: base,
  });
  expect(lineas(borrados, 'TITLE')).toEqual([]);
  expect(lineas(borrados, 'NICKNAME')).toEqual([]);
});
