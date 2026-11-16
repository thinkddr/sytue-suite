/*
 * LA PORTADA ENSEÑA EL DOCUMENTO, NO SOLO SU TIPO.
 *
 * Con miniatura (la secuencia viene en la lista), la tesela pinta la imagen que el
 * servicio dibujó al guardar, con esa secuencia en la URL para que se cachee mientras no
 * cambie; sin ella, o si no carga, el glifo del tipo, como siempre.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { expect, test } from 'bun:test';
import { render } from 'preact';
import { act } from 'preact/test-utils';
import type { DocumentMeta } from '@sytue/api-clients';
import { PortadaDeTesela } from './DocList.js';

if (typeof document === 'undefined') GlobalRegistrator.register();

const base: DocumentMeta = {
  id: '3f2b6d1c-8a4e-4c1b-9d2e-5a6b7c8d9e0f',
  title: 'Acta',
  kind: 'document',
  ownerId: 'ada',
  createdAt: '2026-11-01T10:00:00Z',
  updatedAt: '2026-11-02T10:00:00Z',
  version: 9,
  starred: false,
};

function pintar(doc: DocumentMeta): HTMLElement {
  const host = document.createElement('div');
  document.body.appendChild(host);
  act(() => render(<PortadaDeTesela doc={doc} />, host));
  return host;
}

test('con miniatura, la tesela pinta su imagen con la versión en la URL', () => {
  const host = pintar({ ...base, thumbnailSeq: 7 });
  const img = host.querySelector('img');
  expect(img).not.toBeNull();
  expect(img!.getAttribute('src')).toBe(`/api/thumbnail?id=${base.id}&v=7`);
  // Decorativa: el título y el tipo van escritos debajo; y perezosa, con cien teselas.
  expect(img!.getAttribute('alt')).toBe('');
  expect(img!.getAttribute('loading')).toBe('lazy');
  expect(host.querySelector('[aria-hidden="true"]')).not.toBeNull();
  expect(host.querySelector('svg')).toBeNull();
});

test('un documento y una hoja se recortan por arriba; una diapositiva, entera', () => {
  expect(pintar({ ...base, thumbnailSeq: 1 }).querySelector('img')!.className).toContain(
    'object-top',
  );
  expect(
    pintar({ ...base, kind: 'presentation', thumbnailSeq: 1 }).querySelector('img')!.className,
  ).toContain('object-contain');
});

test('sin miniatura, el glifo del tipo', () => {
  const host = pintar(base);
  expect(host.querySelector('img')).toBeNull();
  expect(host.querySelector('svg')).not.toBeNull();
});

test('si la imagen no carga (sin conexión, acceso revocado), vuelve el glifo', () => {
  const host = pintar({ ...base, thumbnailSeq: 7 });
  act(() => {
    host.querySelector('img')!.dispatchEvent(new Event('error'));
  });
  expect(host.querySelector('img')).toBeNull();
  expect(host.querySelector('svg')).not.toBeNull();
});
