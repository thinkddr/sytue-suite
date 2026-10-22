/**
 * El disquete de «Guardar borrador» es solo visual: su rótulo traducido sigue
 * siendo el nombre accesible y el tooltip. Se mira la fuente porque comprobar
 * este contrato requiere abrir el modal completo (BFF, editor y almacenamiento).
 */
import { join } from 'node:path';
import { expect, test } from 'bun:test';

const fuente = await Bun.file(join(import.meta.dir, 'ComposeModal.tsx')).text();

test('guardar borrador usa el disquete sin perder su nombre accesible', () => {
  const desde = fuente.indexOf("aria-label={t('compose.saveDraft')}");
  expect(desde).toBeGreaterThan(0);
  const boton = fuente.slice(fuente.lastIndexOf('<Button', desde), fuente.indexOf('/>', desde) + 2);
  expect(boton).toContain('iconOnly');
  expect(boton).toContain("title={t('compose.saveDraft')}");
  expect(boton).toContain('<Icon name="save" size={16} />');
});
