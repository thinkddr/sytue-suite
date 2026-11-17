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

// El viaje de la CCO por el BFF se prueba en lib/borrador-cco.test.ts; aquí, que
// el compositor la mande al guardar y que reabrir un borrador la vuelva a poner.
test('el borrador guarda la CCO y reabrirlo la enseña', async () => {
  const desde = fuente.indexOf('async function doSave(');
  const guardar = fuente.slice(desde, fuente.indexOf('setDraftId(res.id)', desde));
  expect(guardar).toContain('bcc: parseAddresses(bcc)');
  expect(fuente).toContain("setBcc(initial?.bcc ?? '')");
  expect(fuente).toContain('setShowCcBcc(Boolean(initial?.cc || initial?.bcc))');

  const app = await Bun.file(join(import.meta.dir, 'MailApp.tsx')).text();
  const abrir = app.slice(
    app.indexOf('function openDraft('),
    app.indexOf('.catch(', app.indexOf('function openDraft(')),
  );
  expect(abrir).toContain('bcc: d.bcc');
});
