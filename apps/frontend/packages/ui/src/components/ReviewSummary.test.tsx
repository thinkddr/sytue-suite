/**
 * Tests de ReviewSummary — el «Editar» de cada fila sale del catálogo común y no
 * escrito en español: antes decía «Editar» en los 26 idiomas. Ejecutar: `bun test`.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { expect, test } from 'bun:test';
import { render } from 'preact';
import { I18nProvider } from '@sytue/i18n/preact';

if (typeof document === 'undefined') GlobalRegistrator.register();

const { ReviewSummary } = await import('./ReviewSummary.js');

function botones(locale: 'de' | 'es', editLabel?: string): string[] {
  const host = document.createElement('div');
  render(
    <I18nProvider locale={locale}>
      <ReviewSummary
        items={[
          { label: 'A', value: '1', onEdit: () => {} },
          { label: 'B', value: '2' },
        ]}
        {...(editLabel ? { editLabel } : {})}
      />
    </I18nProvider>,
    host,
  );
  return [...host.querySelectorAll('button')].map((b) => b.textContent ?? '');
}

test('«Editar» habla el idioma de la página', () => {
  expect(botones('de')).toEqual(['Bearbeiten A']);
  expect(botones('es')).toEqual(['Editar A']);
});

test('una etiqueta propia manda sobre la del catálogo', () => {
  expect(botones('de', 'Ändern')).toEqual(['Ändern A']);
});
