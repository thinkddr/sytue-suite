/**
 * Tests de VersionList — la fila del historial que comparten Drive y CertiPDF: la
 * actual se marca (también para el lector de pantalla), la fecha sale en el formato
 * del idioma y las acciones y lo desplegado son de quien la monta. Ejecutar: `bun test`.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { expect, test } from 'bun:test';
import { render } from 'preact';
import { I18nProvider } from '@sytue/i18n/preact';

if (typeof document === 'undefined') GlobalRegistrator.register();

const { VersionList, formatVersionDate } = await import('./VersionList.js');

const VERSIONES = [
  { version: 2, sizeBytes: 2048, isCurrent: true, createdAt: '2026-11-10T09:30:00Z' },
  { version: 1, sizeBytes: 1024, isCurrent: false, author: 'ana' },
];

function montar(locale: 'de' | 'es', propio?: 'de' | 'es'): HTMLElement {
  const host = document.createElement('div');
  render(
    <I18nProvider locale={locale}>
      <VersionList
        versions={VERSIONES}
        {...(propio ? { locale: propio } : {})}
        versionLabel={(v) => `V${v.version}`}
        currentLabel="Actual"
        actions={(v) => (v.isCurrent ? null : <button type="button">Restaurar {v.version}</button>)}
        below={(v) => (v.version === 1 ? <p data-debajo>debajo</p> : null)}
      />
    </I18nProvider>,
    host,
  );
  return host;
}

test('la actual va marcada y la anterior lleva sus acciones y lo de debajo', () => {
  const host = montar('es');
  const filas = [...host.querySelectorAll('li')];
  expect(filas).toHaveLength(2);
  expect(filas[0]!.getAttribute('aria-current')).toBe('true');
  expect(filas[0]!.textContent).toContain('Actual');
  expect(filas[0]!.querySelector('button')).toBeNull();
  expect(filas[1]!.hasAttribute('aria-current')).toBe(false);
  expect(filas[1]!.querySelector('button')?.textContent).toBe('Restaurar 1');
  expect(filas[1]!.querySelector('[data-debajo]')).not.toBeNull();
  // Sin fecha, un guion; con autor, el autor delante del tamaño.
  expect(filas[1]!.textContent).toContain('—');
  expect(filas[1]!.textContent).toContain('ana · ');
});

test('la fecha sale en el idioma del contexto, o en el que se le pase', () => {
  const fecha = formatVersionDate('de', VERSIONES[0]!.createdAt);
  expect(fecha).not.toBe('');
  expect(montar('de').querySelector('li')!.textContent).toContain(fecha);
  // Una isla que no cuelga de la Shell pasa su idioma: manda sobre el contexto.
  expect(montar('es', 'de').querySelector('li')!.textContent).toContain(fecha);
  expect(formatVersionDate('es', 'no es una fecha')).toBe('');
  expect(formatVersionDate('es')).toBe('');
});
