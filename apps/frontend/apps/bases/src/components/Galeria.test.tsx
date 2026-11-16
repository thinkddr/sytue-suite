/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * La galería: una tarjeta por registro, con la MINIATURA del primer adjunto de imagen
 * de portada (o el hueco si no hay), y la tarjeta entera abre la ficha.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterAll, afterEach, beforeAll, expect, test } from 'bun:test';
import { render } from 'preact';
import type { VNode } from 'preact';
import type { BasesAttachment, BasesField, BasesRecord } from '@sytue/api-clients/bases';
import { useT as traductor } from '../i18n/index.js';
import { Galeria } from './Galeria.js';

let registrado = false;
beforeAll(() => {
  if (typeof document === 'undefined') {
    GlobalRegistrator.register();
    registrado = true;
  }
});
afterAll(async () => {
  if (registrado) await GlobalRegistrator.unregister();
});

let host: HTMLElement;
afterEach(() => {
  render(null, host);
  host.remove();
});
function montar(v: VNode): void {
  host = document.createElement('div');
  document.body.appendChild(host);
  render(v, host);
}
const t = traductor('es');
const nombre: BasesField = { id: 'x', table_id: 't', name: 'Nombre', kind: 'text', position: 0 };
const ciudad: BasesField = { id: 'c', table_id: 't', name: 'Ciudad', kind: 'text', position: 1 };
const fotos: BasesField = {
  id: 'f',
  table_id: 't',
  name: 'Fotos',
  kind: 'attachment',
  position: 2,
};
const adjunto = (id: string, mime: string, has_thumb = true): BasesAttachment => ({
  id,
  record_id: '',
  field_id: 'f',
  name: `${id}.bin`,
  mime,
  size: 10,
  has_thumb,
  created_by: '',
  created_at: '',
});
const reg = (id: string, data: BasesRecord['data'], f: BasesAttachment[] = []): BasesRecord => ({
  id,
  table_id: 't',
  data,
  links: {},
  attachments: f.length ? { f: f.map((a) => ({ ...a, record_id: id })) } : {},
  rev: 1,
  created_by: '',
  updated_by: '',
  created_at: '',
  updated_at: '',
});

test('cada tarjeta lleva de portada la miniatura de su primera imagen, o el hueco', () => {
  const abiertos: string[] = [];
  montar(
    <Galeria
      t={t}
      campos={[nombre, ciudad, fotos]}
      registros={[
        reg('r1', { x: 'Casa azul', c: 'Lugo' }, [
          adjunto('pdf', 'application/pdf'),
          adjunto('a1', 'image/jpeg'),
        ]),
        reg('r2', { x: 'Sin foto' }),
      ]}
      puedeEditar
      onAbrir={(r) => abiertos.push(r.id)}
    />,
  );
  const tarjetas = document.querySelectorAll('[data-bases-card]');
  expect(tarjetas.length).toBe(2);
  const img = tarjetas[0]!.querySelector('img')!;
  expect(img.getAttribute('src')).toEndWith('/api/records/r1/attachments/a1/thumb');
  expect(img.getAttribute('alt')).toBe('');
  expect(tarjetas[0]!.textContent).toContain('Ciudad: Lugo');
  expect(tarjetas[1]!.querySelector('img')).toBeNull();
  expect(tarjetas[1]!.querySelector('[data-sin-portada]')).not.toBeNull();

  const abrir = document.querySelector<HTMLButtonElement>(
    `[aria-label="${t('board.open', { name: 'Sin foto' })}"]`,
  )!;
  abrir.click();
  expect(abiertos).toEqual(['r2']);
});

test('sin registros la galería lo dice, y en solo lectura avisa', () => {
  montar(
    <Galeria
      t={t}
      campos={[nombre]}
      registros={[]}
      puedeEditar={false}
      onAbrir={() => undefined}
    />,
  );
  expect(document.body.textContent).toContain(t('grid.empty'));
  expect(document.body.textContent).toContain(t('grid.readOnly'));
});

test('si la miniatura no llega cae al hueco, y una portada nueva vuelve a intentarlo', () => {
  const galeria = (f: BasesAttachment[]) => (
    <Galeria
      t={t}
      campos={[nombre, fotos]}
      registros={[reg('r1', { x: 'Casa' }, f)]}
      puedeEditar
      onAbrir={() => undefined}
    />
  );
  montar(galeria([adjunto('a1', 'image/jpeg')]));
  host.querySelector('img')!.dispatchEvent(new Event('error'));
  render(galeria([adjunto('a1', 'image/jpeg')]), host);
  expect(host.querySelector('img')).toBeNull();
  expect(host.querySelector('[data-sin-portada]')).not.toBeNull();
  render(galeria([adjunto('a2', 'image/png')]), host);
  expect(host.querySelector('img')?.getAttribute('src')).toEndWith('/attachments/a2/thumb');
});
