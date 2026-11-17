/**
 * Tests de groupByThread — el recuento que enseña la fila de la lista.
 *
 * Lo que puede salir mal aquí no da error: la fila decía «2» y el hilo se abría con
 * cinco, porque contaba solo lo plegado de ESTA carpeta mientras el lector pide el
 * hilo entero. Ahora el número viene del servidor, y lo que hay que fijar es que el
 * plegado de la carpeta sea el SUELO —nunca menos de lo que ya se ve— y que sin ese
 * dato la fila siga diciendo algo cierto. Ejecutar: `bun test`.
 */
import { expect, test } from 'bun:test';
import { readdirSync, readFileSync } from 'node:fs';
import {
  ATRIBUCIONES_DE_CITA,
  OPCIONES_BUSQUEDA_VACIAS,
  componerConsulta,
  conversacionAbierta,
  desmontarConsulta,
  groupByThread,
  separarCita,
  type OpcionesBusqueda,
} from './utils.js';
import type { EmailSummary } from '@sytue/api-clients';

function correo(id: string, threadId: string, threadCount?: number): EmailSummary {
  return {
    id,
    threadId,
    subject: 'Asunto',
    from: [{ email: 'ana@ejemplo.eu' }],
    to: [],
    cc: [],
    preview: '',
    receivedAt: '2026-08-15T10:00:00Z',
    keywords: { $seen: true },
    hasAttachment: false,
    ...(threadCount ? { threadCount } : {}),
  };
}

test('el total es el del hilo entero, aunque en la carpeta solo se vea uno', () => {
  const [conv] = groupByThread([correo('m1', 't1', 5)]);
  expect(conv?.total).toBe(5);
  expect(conv?.messages).toHaveLength(1); // la carpeta sigue teniendo uno
});

test('sin el dato del servidor cae a lo que se ve, no a cero', () => {
  const [conv] = groupByThread([correo('m1', 't1'), correo('m2', 't1')]);
  expect(conv?.total).toBe(2);
});

test('lo plegado en la carpeta es el SUELO: un total menor no encoge la fila', () => {
  // Puede pasar: el recuento se calcula por hilo y la página trae mensajes que
  // acaban de moverse. Enseñar menos de lo que ya se ve sería mentir hacia abajo.
  const [conv] = groupByThread([correo('m1', 't1', 1), correo('m2', 't1', 1)]);
  expect(conv?.total).toBe(2);
});

test('los mensajes sin hilo no se funden en uno solo', () => {
  const convs = groupByThread([correo('m1', ''), correo('m2', '')]);
  expect(convs).toHaveLength(2);
  expect(convs.every((c) => c.total === 1)).toBe(true);
});

// --- Motivo de un envío rechazado -------------------------------------------
//
// El compositor decía siempre «no se ha podido enviar»: el porqué existía en los
// tres saltos (servicio → BFF → cliente) y se tiraba en el primero. Lo que hay que
// fijar es que un rechazo DEL CLIENTE se explique y que un fallo del servidor —o
// un corte de red— NO invente una explicación que no tiene.
import { motivoDeEnvio } from './ComposeModal.js';
import { MailHttpError } from '../client/mailApi.js';

test('un 403 dice que el buzón no puede enviar desde esa dirección', () => {
  expect(motivoDeEnvio(new MailHttpError(403, 'forbidden'))).toBe('compose.sendForbidden');
});

test('un 400 manda a revisar destinatarios y fecha', () => {
  expect(motivoDeEnvio(new MailHttpError(400, 'bad_request'))).toBe('compose.sendInvalid');
});

test('un 502 no se le explica a quien escribe: no es culpa suya ni tiene arreglo ahí', () => {
  expect(motivoDeEnvio(new MailHttpError(502))).toBeNull();
});

test('un corte de red no se disfraza de rechazo', () => {
  expect(motivoDeEnvio(new TypeError('Failed to fetch'))).toBeNull();
});

// Abrir un correo desde el buscador global: el hilo no tiene por qué estar en la
// carpeta que se ve, y aun así el lector tiene que abrirse con él.
test('un hilo abierto por enlace se arma con el hilo completo si no está en la lista', () => {
  const lista = groupByThread([correo('m1', 't1')]);
  const hilo = {
    id: 't9',
    messages: [
      { ...correo('v1', 't9'), receivedAt: '2026-08-01T09:00:00Z', mailboxId: 'archivo' },
      { ...correo('v2', 't9'), receivedAt: '2026-08-02T09:00:00Z', mailboxId: 'enviados' },
    ],
  };
  expect(conversacionAbierta(lista, 't1', null)?.threadId).toBe('t1');
  // Todavía sin el hilo del servicio: no se inventa nada.
  expect(conversacionAbierta(lista, 't9', null)).toBeNull();
  const conv = conversacionAbierta(lista, 't9', hilo);
  expect(conv?.threadId).toBe('t9');
  expect(conv?.latest.id).toBe('v2'); // lo más reciente arriba, como en la lista
  expect(conversacionAbierta(lista, null, hilo)).toBeNull();
});

/*
 * separarCita — el pliegue de la cita en los mensajes del hilo. Los casos los comparte
 * con `cita.go` (apps/fyne/apps/mail/testdata/cita.json, que recorre
 * `TestLaCitaSeSeparaComoEnLaWeb`).
 *
 * VISTO EN ROJO (2026-11-09): sin la coma opcional, la cabecera de Gmail en inglés
 * no se reconocía; sin unir dos líneas, la atribución partida se quedaba en lo nuevo;
 * sin el «no queda nada nuevo», un mensaje que era solo cita salía vacío. Deshecho.
 */
const vectoresCita = JSON.parse(
  readFileSync(
    new URL('../../../../../fyne/apps/mail/testdata/cita.json', import.meta.url),
    'utf8',
  ),
) as { casos: { nombre: string; texto: string; cuerpo: string; cita: string }[] };

test('hay casos de cita que recorrer', () => {
  expect(vectoresCita.casos.length).toBeGreaterThanOrEqual(10);
});

for (const c of vectoresCita.casos) {
  test(`separarCita: ${c.nombre}`, () => {
    expect(separarCita(c.texto)).toEqual({ cuerpo: c.cuerpo, cita: c.cita });
  });
}

test('las atribuciones de cita son las de reader.quoteHeader de los 26 catálogos', async () => {
  const dir = new URL('../i18n/messages/', import.meta.url);
  const ficheros = readdirSync(dir).filter((f) => f.endsWith('.ts'));
  expect(ficheros.length).toBe(26);
  const plantillas = new Set<string>();
  for (const f of ficheros) {
    const mod = (await import(new URL(f, dir).href)) as Record<string, Record<string, string>>;
    const catalogo = mod[f.replace(/\.ts$/, '')]!;
    plantillas.add(catalogo['reader.quoteHeader']!);
  }
  expect(new Set(ATRIBUCIONES_DE_CITA)).toEqual(plantillas);
  expect(ATRIBUCIONES_DE_CITA.length).toBe(plantillas.size);
});

test('separarCita reconoce la cabecera que escribe la propia respuesta en cada idioma', () => {
  for (const p of ATRIBUCIONES_DE_CITA) {
    const cabecera = p.replace('{date}', '5 ene 2026, 10:00').replace('{name}', 'Ana Ruiz');
    const { cuerpo } = separarCita(`Vale.\n\n${cabecera}\n> Hola`);
    expect(cuerpo).toBe('Vale.');
  }
});

/*
 * VISTO EN ROJO (2026-11-09): sin el tope de largo, una línea de 8 000 caracteres sin
 * saltos justo encima de la cita tardaba más de 7 s en `separarCita` (la expresión de
 * la atribución probaba todos los repartos de sus huecos), y colgaba el lector. Deshecho.
 */
test('una línea larga encima de la cita no cuelga separarCita', () => {
  const largo = 'a, b '.repeat(4000);
  const t0 = performance.now();
  const { cuerpo, cita } = separarCita(`Hola\n${largo}\n${largo}\n> cita`);
  expect(performance.now() - t0).toBeLessThan(500);
  expect(cita).toBe('> cita');
  expect(cuerpo).toBe(`Hola\n${largo}\n${largo}`.trimEnd());
});

/*
 * VISTO EN ROJO (2026-11-09): con el tope de 256 caracteres pero los espacios de la
 * plantilla como `\s*`, una línea «a␣…␣b» de 252 caracteres encima de la cita aún tardaba
 * 1,3 s (el `\s*` y los huecos `.+` se disputaban cada espacio). Deshecho.
 */
test('una racha de espacios dentro del tope tampoco cuelga separarCita', () => {
  const linea = `a${' '.repeat(250)}b`;
  const t0 = performance.now();
  for (let i = 0; i < 5; i++) expect(separarCita(`Hola\n${linea}\n> cita`).cita).toBe('> cita');
  expect(performance.now() - t0).toBeLessThan(250);
});

/*
 * componerConsulta / desmontarConsulta — el panel «Opciones de búsqueda». Los casos los
 * comparte con `consulta.go` (apps/fyne/apps/mail/testdata/consulta-busqueda.json, que
 * recorren `TestLaConsultaSeComponeComoEnLaWeb` y el parser del servicio).
 *
 * VISTO EN ROJO (2026-11-13): escribiendo «Hasta» tal cual, el 28 de febrero se quedaba
 * fuera (before excluye su día); sin quitar las comillas de dentro, `subject:"el "plan" B"`
 * partía la consulta en tres. Deshecho.
 */
type CasoConsulta = { nombre: string; opciones: Partial<OpcionesBusqueda>; consulta: string };
const vectoresConsulta = JSON.parse(
  readFileSync(
    new URL('../../../../../fyne/apps/mail/testdata/consulta-busqueda.json', import.meta.url),
    'utf8',
  ),
) as { componer: CasoConsulta[]; desmontar: CasoConsulta[] };

test('hay casos de consulta que recorrer', () => {
  expect(vectoresConsulta.componer.length).toBeGreaterThanOrEqual(10);
  expect(vectoresConsulta.desmontar.length).toBeGreaterThanOrEqual(10);
});

for (const c of vectoresConsulta.componer) {
  test(`componerConsulta: ${c.nombre}`, () => {
    expect(componerConsulta({ ...OPCIONES_BUSQUEDA_VACIAS, ...c.opciones })).toBe(c.consulta);
  });
}

for (const c of vectoresConsulta.desmontar) {
  test(`desmontarConsulta: ${c.nombre}`, () => {
    const o = desmontarConsulta(c.consulta);
    expect(o).toEqual({ ...OPCIONES_BUSQUEDA_VACIAS, ...c.opciones });
    // Aplicar el panel sin tocar nada no cambia lo que se busca.
    expect(desmontarConsulta(componerConsulta(o))).toEqual(o);
  });
}
