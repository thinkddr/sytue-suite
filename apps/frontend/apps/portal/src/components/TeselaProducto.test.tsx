/**
 * LA TESELA DESACTIVADA DICE POR QUÉ, A LA VISTA.
 *
 * Se vigila:
 * - que el motivo se LEE en la tesela, no en un `title` que solo ve quien pasa el
 *   puntero (en el móvil y con el teclado, nadie);
 * - que trae un enlace de ayuda enfocable, a la ficha del producto en su idioma, y que
 *   ese enlace entra en la navegación con flechas de la rejilla;
 * - que la tesela desactivada no es un enlace al producto ni lleva a ninguna parte;
 * - que la tesela activa sigue siendo el enlace de siempre, sin motivo ni ayuda.
 *
 * Vistos en ROJO con la tesela de antes: un `<a>` sin `href` con la píldora
 * «Desactivado» y el porqué solo en `title`.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterAll, afterEach, expect, test } from 'bun:test';
import { render } from 'preact';
import { act } from 'preact/test-utils';
import { suiteApex } from '@sytue/config';
import { en } from '../i18n/messages/en.js';
import { es } from '../i18n/messages/es.js';
import type { AppConectada, ProductoDeLaGaleria } from './TeselaProducto.js';

const registrado = typeof document === 'undefined';
if (registrado) GlobalRegistrator.register({ url: 'https://one.sytue.test/' });

const { TeselaConectada, TeselaProducto, navegarProducto, inicialDe } =
  await import('./TeselaProducto.js');

afterEach(() => {
  act(() => render(null, document.body));
  document.body.innerHTML = '';
});

afterAll(async () => {
  if (registrado) await GlobalRegistrator.unregister();
});

const erp = (disabled: boolean): ProductoDeLaGaleria => ({
  key: 'erp',
  name: 'Sytue ERP',
  description: 'Contabilidad y facturación',
  href: 'https://erp.sytue.test/acme',
  color: 'var(--sytue-app-erp)',
  disabled,
});

const nada = (): void => {};

function montar(app: ProductoDeLaGaleria, locale: 'en' | 'es' = 'en'): HTMLElement {
  act(() => {
    render(
      <ul>
        <li>
          <TeselaProducto app={app} locale={locale} onKeyDown={nada} />
        </li>
      </ul>,
      document.body,
    );
  });
  return document.body.querySelector('li')!;
}

test('la desactivada enseña el motivo en la propia tesela, no en un title', () => {
  const tesela = montar(erp(true));
  expect(tesela.textContent).toContain(en['launcher.orgOnlyReason']);
  expect(tesela.textContent).toContain(en['launcher.disabled']);
  // El porqué ya no se esconde en el tooltip: nada en la tesela lleva `title`.
  expect(tesela.querySelector('[title]')).toBeNull();
  // Y el grupo lo anuncia como descripción, con el nombre entero del producto.
  const grupo = tesela.querySelector<HTMLElement>('[role="group"]')!;
  expect(grupo.getAttribute('aria-label')).toBe('Sytue ERP');
  const motivo = document.getElementById(grupo.getAttribute('aria-describedby')!);
  expect(motivo?.textContent).toBe(en['launcher.orgOnlyReason']);
});

test('la desactivada lleva a su ayuda con un enlace enfocable, y a ningún producto', () => {
  const tesela = montar(erp(true));
  const enlaces = tesela.querySelectorAll<HTMLAnchorElement>('a');
  expect(enlaces).toHaveLength(1);
  const ayuda = enlaces[0]!;
  expect(ayuda.getAttribute('href')).toBe(`https://www.${suiteApex}/product/erp`);
  expect(ayuda.textContent).toBe(en['launcher.orgOnlyHelp']);
  expect(ayuda.getAttribute('aria-label')).toBe('Learn more about Sytue ERP');
  // Entra en las flechas de la rejilla, que buscan `a[data-hub-product][href]`.
  expect(tesela.closest('ul')!.querySelectorAll('a[data-hub-product][href]')).toHaveLength(1);
  ayuda.focus();
  expect(document.activeElement).toBe(ayuda);
  // El producto en sí no se ofrece.
  expect(tesela.innerHTML).not.toContain('erp.sytue.test');
});

test('la ayuda sale en el idioma de quien mira', () => {
  const tesela = montar(erp(true), 'es');
  expect(tesela.textContent).toContain(es['launcher.orgOnlyReason']);
  expect(tesela.querySelector('a')!.getAttribute('href')).toBe(
    `https://www.${suiteApex}/es/product/erp`,
  );
});

test('Sytue Web lleva a su ficha de /product/web, no a la dirección vieja que solo redirige', () => {
  const tesela = montar({ ...erp(true), key: 'pages', name: 'Sytue Web' });
  expect(tesela.querySelector('a')!.getAttribute('href')).toBe(
    `https://www.${suiteApex}/product/web`,
  );
});

test('la activada sigue siendo el enlace al producto, sin motivo ni ayuda', () => {
  const tesela = montar(erp(false));
  const enlace = tesela.querySelector<HTMLAnchorElement>('a[data-hub-product]')!;
  expect(enlace.getAttribute('href')).toBe('https://erp.sytue.test/acme');
  expect(enlace.getAttribute('aria-disabled')).toBeNull();
  expect(tesela.textContent).not.toContain(en['launcher.orgOnlyReason']);
  expect(tesela.textContent).not.toContain(en['launcher.disabled']);
  expect(tesela.querySelectorAll('a')).toHaveLength(1);
});

// ---- Las apps conectadas, iguales que las propias ----
//
// Vistos en ROJO con la tesela de antes, escrita a mano en `Launcher.tsx`: caja
// `rounded-lg border-border`, sin forma compacta de Marco, sin `data-hub-product` —las
// flechas se la saltaban, y desde la rejilla propia no se llegaba— y, sin `iconUrl`, una
// caja gris vacía.

const holded = (iconUrl = ''): AppConectada => ({
  key: 'i1',
  name: 'holded',
  description: 'Facturación y contabilidad',
  href: 'https://app.holded.com',
  iconUrl,
  color: '#e03',
  status: 'active',
});

/** La galería del lanzador: dos rejillas dentro de `[data-hub-galeria]`. */
function montarGaleria(conectadas: AppConectada[]): HTMLElement {
  act(() => {
    render(
      <div data-hub-galeria>
        <ul>
          <li>
            <TeselaProducto app={erp(false)} locale="en" onKeyDown={navegarProducto} />
          </li>
          <li>
            <TeselaProducto
              app={{ ...erp(false), key: 'mail', name: 'Sytue Mail', href: 'https://mail.test/' }}
              locale="en"
              onKeyDown={navegarProducto}
            />
          </li>
        </ul>
        <ul>
          {conectadas.map((app) => (
            <li key={app.key}>
              <TeselaConectada app={app} locale="en" onKeyDown={navegarProducto} />
            </li>
          ))}
        </ul>
      </div>,
      document.body,
    );
  });
  return document.body.querySelector('[data-hub-galeria]')!;
}

const pulsar = (tecla: string): void => {
  const ev = new KeyboardEvent('keydown', { key: tecla, bubbles: true, cancelable: true });
  act(() => {
    document.activeElement!.dispatchEvent(ev);
  });
};

test('la conectada lleva la misma caja que la propia, con su forma compacta de Marco', () => {
  const galeria = montarGaleria([holded()]);
  const propia = galeria.querySelector<HTMLAnchorElement>('a[data-hub-product="erp"]')!;
  const conectada = galeria.querySelector<HTMLAnchorElement>('a[data-hub-conectada]')!;
  const clases = (el: Element): string[] => el.className.split(/\s+/).sort();
  expect(clases(conectada)).toEqual(clases(propia));
  expect(conectada.className).toContain('rounded-card');
  expect(conectada.className).toContain('marco:flex-row');
  expect(conectada.className).not.toContain('rounded-lg');
  // La descripción, en una línea en Marco, como la de producto.
  expect(conectada.querySelector('.marco\\:line-clamp-1')).not.toBeNull();
  // Sigue abriéndose fuera, con su nombre y su descripción en el rótulo.
  expect(conectada.getAttribute('target')).toBe('_blank');
  expect(conectada.getAttribute('aria-label')).toBe('holded — Facturación y contabilidad');
});

test('el icono de la conectada va en un distintivo con la geometría de ProductIcon', () => {
  const galeria = montarGaleria([holded('data:image/svg+xml;base64,PHN2Zy8+')]);
  const marca = galeria.querySelector<HTMLElement>('a[data-hub-conectada] > span')!;
  expect(marca.style.width).toBe('40px');
  expect(marca.style.height).toBe('40px');
  const distintivo = marca.querySelector<HTMLElement>('[data-hub-distintivo]')!;
  expect(distintivo.style.inset).toBe('4px');
  expect(distintivo.style.borderRadius).toBe('9px');
  expect(distintivo.className).toContain('bg-bg-subtle');
  expect(distintivo.querySelector('img')!.getAttribute('src')).toBe(
    'data:image/svg+xml;base64,PHN2Zy8+',
  );
  // Sin teñir con el color de la app ni con nuestro acento.
  expect(galeria.querySelector('a[data-hub-conectada]')!.outerHTML).not.toContain('#e03');
  expect(distintivo.className).not.toMatch(/primary|accent/);
});

test('sin icono, la conectada enseña su inicial sobre neutro y no una caja vacía', () => {
  const galeria = montarGaleria([holded()]);
  const distintivo = galeria.querySelector<HTMLElement>('[data-hub-distintivo]')!;
  expect(distintivo.querySelector('img')).toBeNull();
  expect(distintivo.textContent).toBe('H');
  expect(distintivo.className).toContain('text-text-muted');
  expect(inicialDe('  écija')).toBe('É');
  expect(inicialDe('')).toBe('?');
});

test('las flechas pasan de la rejilla propia a la de conectadas y vuelven', () => {
  const galeria = montarGaleria([holded(), { ...holded(), key: 'i2', name: 'Factorial' }]);
  const enlaces = Array.from(galeria.querySelectorAll<HTMLAnchorElement>('a[data-hub-product]'));
  expect(enlaces).toHaveLength(4);
  for (const a of enlaces) expect(a.getAttribute('aria-keyshortcuts')).toContain('ArrowRight');
  enlaces[1]!.focus();
  pulsar('ArrowRight');
  expect(document.activeElement).toBe(enlaces[2]!);
  pulsar('ArrowLeft');
  expect(document.activeElement).toBe(enlaces[1]!);
  pulsar('End');
  expect(document.activeElement).toBe(enlaces[3]!);
  pulsar('Home');
  expect(document.activeElement).toBe(enlaces[0]!);
});
