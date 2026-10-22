/**
 * `esAtajoDeApp` — su contrato: escribir NUNCA dispara un atajo, y con una capa
 * abierta el teclado es de la capa.
 *
 * Lo primero es lo que evita que teclear «este» en el asunto de un correo lo
 * archive. Lo segundo se comprobó de menos durante tres apps: la lista de correos
 * y la de contactos son `role="listbox"` PERMANENTES, así que si el detector de
 * capas las contase, en esas dos apps no funcionaría ni un atajo.
 * Ejecutar: `bun test`.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { beforeEach, expect, test } from 'bun:test';
import { enCampoDeTexto, esAtajoDeApp, haySuperposicionAbierta } from './atajos.js';

// Idempotente: otro fichero de test puede haber registrado ya happy-dom.
if (typeof document === 'undefined') GlobalRegistrator.register();

// El detector de capas pregunta por el DOCUMENTO entero, y toda la batería de
// tests comparte uno: los diálogos que montan Modal.test y Popover.test siguen
// colgando del `<body>` cuando llega este fichero. Se parte de un documento
// limpio en cada caso, o lo que se mide es el desorden de los vecinos.
beforeEach(() => {
  document.body.innerHTML = '';
});

/** Un `keydown` con el destino y los modificadores que se quieran. */
function tecla(target: unknown, extra: Partial<KeyboardEvent> = {}): KeyboardEvent {
  return {
    key: 'e',
    ctrlKey: false,
    metaKey: false,
    altKey: false,
    target,
    ...extra,
  } as KeyboardEvent;
}

test('sobre el documento, la tecla es de la app', () => {
  expect(esAtajoDeApp(tecla({ tagName: 'BODY' }))).toBe(true);
});

test('escribiendo, la tecla es del campo', () => {
  expect(esAtajoDeApp(tecla({ tagName: 'INPUT' }))).toBe(false);
  expect(esAtajoDeApp(tecla({ tagName: 'TEXTAREA' }))).toBe(false);
  expect(esAtajoDeApp(tecla({ tagName: 'SELECT' }))).toBe(false);
  // El cuerpo del compositor de Mail y el lienzo de Docs son contenteditable,
  // no <textarea>: sin esta rama, escribir en ellos ejecutaba atajos.
  expect(esAtajoDeApp(tecla({ tagName: 'DIV', isContentEditable: true }))).toBe(false);
  expect(enCampoDeTexto(null)).toBe(false);
});

test('con modificador la tecla es del navegador o del sistema', () => {
  for (const mod of ['ctrlKey', 'metaKey', 'altKey'] as const) {
    expect(esAtajoDeApp(tecla({ tagName: 'BODY' }, { [mod]: true }))).toBe(false);
  }
});

test('con un diálogo abierto, el teclado es suyo', () => {
  document.body.innerHTML = '<div role="dialog"></div>';
  expect(haySuperposicionAbierta()).toBe(true);
  expect(esAtajoDeApp(tecla({ tagName: 'BODY' }))).toBe(false);
});

test('con un menú abierto, el teclado es suyo', () => {
  document.body.innerHTML = '<div role="menu"></div>';
  expect(esAtajoDeApp(tecla({ tagName: 'BODY' }))).toBe(false);
});

test('una lista seleccionable NO es una capa abierta', () => {
  // `role="listbox"` es lo que pinta <List selectable>, que está SIEMPRE puesta en
  // Mail y en Contactos. Contarla apagaría todos sus atajos, y el fallo se leería
  // como «los atajos no van» sin ninguna pista de por qué.
  document.body.innerHTML = '<ul role="listbox"></ul>';
  expect(haySuperposicionAbierta()).toBe(false);
  expect(esAtajoDeApp(tecla({ tagName: 'BODY' }))).toBe(true);
});
