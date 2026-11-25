/*
 * LA TESELA DE UN DOCUMENTO EN EL TELÉFONO, Y SU TÍTULO A 1280.
 *
 * Medido el 2026-11-24 (roadmap, «Rendimiento y estética de la web»): con el dedo cada
 * tarjeta enseñaba cinco iconos —estrella, renombrar, «…», compartir y eliminar—, el
 * escritorio metido en el teléfono; y a 1280 los títulos se cortaban a los quince
 * caracteres bajo portadas iguales (sin miniatura todas son el mismo glifo).
 *
 * Ahora, con el dedo (`pointer-coarse:`), queda solo «…» y la estrella si el documento ya
 * está destacado; el menú trae todo lo que hacían los otros. Y el título va en dos líneas,
 * reservadas cuando la tesela comparte fila para que la fecha caiga a la misma altura.
 * happy-dom no evalúa `pointer-coarse:`: se comprueba la clase que lo hace.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterEach, expect, test } from 'bun:test';
import { render } from 'preact';
import { act } from 'preact/test-utils';
import type { DocumentMeta } from '@sytue/api-clients';
import { docsT } from '../i18n';
import { EXPORT_FORMATS } from '../editor/types';
import { DocTile, type DocTileProps } from './DocList.js';

if (typeof document === 'undefined') GlobalRegistrator.register();

const t = docsT('es');
const doc: DocumentMeta = {
  id: 'd1',
  title: 'Informe trimestral de ventas del tercer trimestre',
  kind: 'document',
  ownerId: 'ada',
  createdAt: '2026-11-01T10:00:00Z',
  updatedAt: '2026-11-02T10:00:00Z',
  version: 1,
  starred: false,
};

let host: HTMLElement | null = null;
afterEach(() => {
  if (host) {
    act(() => render(null, host!));
    host.remove();
    host = null;
  }
});

function pintar(props: Partial<DocTileProps> = {}): {
  host: HTMLElement;
  llamadas: string[];
} {
  const llamadas: string[] = [];
  const anota = (n: string) => () => llamadas.push(n);
  host = document.createElement('div');
  document.body.appendChild(host);
  act(() =>
    render(
      <DocTile
        doc={doc}
        locale="es"
        t={t}
        onRename={anota('renombrar')}
        onDownload={(_, f) => llamadas.push(`descargar:${f}`)}
        onCopy={anota('copiar')}
        onShare={anota('compartir')}
        onDelete={anota('eliminar')}
        onStar={anota('destacar')}
        offline={false}
        {...props}
      />,
      host!,
    ),
  );
  return { host: host!, llamadas };
}

const boton = (h: HTMLElement, etiqueta: string) =>
  h.querySelector<HTMLButtonElement>(`button[aria-label="${etiqueta}"]`)!;

test('con el dedo, de los cinco iconos de la tesela solo queda «…»', () => {
  const { host } = pintar();
  const titulo = doc.title;
  for (const clave of ['tile.rename.label', 'tile.share.label', 'tile.delete.label'] as const) {
    expect(boton(host, t(clave, { title: titulo })).className).toContain('pointer-coarse:hidden');
  }
  // La estrella vacía tampoco: con la opacidad a cero seguía ahí para un toque a ciegas.
  expect(boton(host, t('tile.star.label', { title: titulo })).className).toContain(
    'pointer-coarse:hidden',
  );
  const mas = boton(host, t('tile.more.label', { title: titulo }));
  expect(mas.className).not.toContain('pointer-coarse:hidden');
  // Y a la vista siempre: con el dedo no hay `hover` que lo descubra.
  expect(mas.closest('.pointer-coarse\\:opacity-100')).not.toBeNull();
});

test('la estrella de un documento destacado se queda: dice algo de él', () => {
  const { host } = pintar({ doc: { ...doc, starred: true } });
  const estrella = boton(host, t('tile.unstar.label', { title: doc.title }));
  expect(estrella.className).not.toContain('pointer-coarse:hidden');
});

test('el «…» trae todo lo que hacían los otros botones, y cada entrada funciona', async () => {
  const { host, llamadas } = pintar();
  act(() => boton(host, t('tile.more.label', { title: doc.title })).click());
  const entradas = () => [...document.querySelectorAll<HTMLElement>('[role="menuitem"]')];
  expect(entradas().map((e) => e.textContent?.trim())).toEqual([
    t('tile.rename'),
    t('tile.star'),
    t('tile.share'),
    t('copy.action'),
    ...EXPORT_FORMATS.map((f) => t(`export.format.${f}`)),
    t('tile.delete'),
  ]);
  const pulsar = (texto: string) => {
    act(() => boton(host, t('tile.more.label', { title: doc.title })).click());
    act(() =>
      entradas()
        .find((e) => e.textContent?.trim() === texto)!
        .click(),
    );
  };
  act(() => entradas()[0]!.click());
  // Se cierra al elegir.
  expect(document.querySelector('[role="menu"]')).toBeNull();
  pulsar(t('tile.star'));
  pulsar(t('tile.share'));
  pulsar(t('tile.delete'));
  expect(llamadas).toEqual(['renombrar', 'destacar', 'compartir', 'eliminar']);
});

test('el título va en dos líneas; en una fila de varias, reservadas', () => {
  const titulo = (h: HTMLElement) => h.querySelector<HTMLElement>(`[title="${doc.title}"]`)!;
  const sola = pintar().host;
  expect(titulo(sola).className).toContain('line-clamp-2');
  expect(titulo(sola).className).not.toContain('truncate');
  expect(titulo(sola).className).not.toContain('min-h-[2lh]');
  act(() => render(null, sola));
  sola.remove();
  host = null;
  expect(titulo(pintar({ enFila: true }).host).className).toContain('min-h-[2lh]');
});
