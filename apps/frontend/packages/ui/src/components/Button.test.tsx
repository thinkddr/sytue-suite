/**
 * Tests de Button — la variante ENLACE, que es la que tiene lógica que se puede
 * equivocar. Un `<a>` no entiende `disabled`: si se le pone el atributo sin más, el
 * navegador lo ignora y el enlace sigue navegando y siendo tabulable. Ejecutar: `bun test`.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { expect, test } from 'bun:test';
import { render } from 'preact';
import type { VNode } from 'preact';
import { Button } from './Button.js';

// Idempotente: otro fichero de test puede haber registrado ya happy-dom.
if (typeof document === 'undefined') GlobalRegistrator.register();

function mount(children: VNode): HTMLElement {
  const host = document.createElement('div');
  document.body.appendChild(host);
  render(children, host);
  return host;
}

test('sin href es un <button> de verdad', () => {
  const host = mount(<Button>Guardar</Button>);
  expect(host.querySelector('button')).not.toBeNull();
  expect(host.querySelector('a')).toBeNull();
});

test('con href es un <a> de verdad, no un botón que navega con JS', () => {
  // Importa que sea un anchor: se abre en otra pestaña, se copia el destino y el lector
  // de pantalla lo anuncia como enlace.
  const host = mount(<Button href="/precios">Ver precios</Button>);
  const a = host.querySelector('a');
  expect(a).not.toBeNull();
  expect(a?.getAttribute('href')).toBe('/precios');
  expect(host.querySelector('button')).toBeNull();
});

test('un enlace deshabilitado PIERDE el href, que es lo único que lo desactiva', () => {
  // `<a disabled>` no existe en HTML: el atributo se ignora y el enlace seguiría
  // navegando y saliendo en el orden de tabulación. Quitar el href es lo que lo saca.
  const host = mount(
    <Button href="/precios" disabled>
      Ver precios
    </Button>,
  );
  const a = host.querySelector('a');
  expect(a).not.toBeNull();
  expect(a?.hasAttribute('href')).toBe(false);
  expect(a?.getAttribute('aria-disabled')).toBe('true');
});

test('cargando también deja el enlace inerte y lo anuncia', () => {
  const host = mount(
    <Button href="/informe" loading>
      Generando
    </Button>,
  );
  const a = host.querySelector('a');
  expect(a?.hasAttribute('href')).toBe(false);
  expect(a?.getAttribute('aria-busy')).toBe('true');
});

test('el enlace conserva el aspecto del botón, incluido el anillo de foco', () => {
  // El motivo de que `href` viva en Button: copiar las clases a mano perdía el
  // anillo, y de hecho ya había pasado en la página 404. Se comprueba por la clase
  // canónica y no por la variable CSS: el anillo se escribía aquí a mano —2px de
  // hueco, 2px de latón y 2px de contorno negro, un marco de 6px— y ahora es la
  // clase compartida, que es justo lo que evita que vuelva a divergir.
  const host = mount(<Button href="/x" variant="secondary" size="lg" />);
  const cls = host.querySelector('a')?.className ?? '';
  expect(cls).toContain('sytue-focus-ring-inset');
  expect(cls).toContain('h-11'); // size="lg"
  expect(cls).toContain('border-border-strong'); // variant="secondary"
});

test('`xs` es 24 px, el suelo de WCAG 2.5.8, y no arrastra el gap de las tallas grandes', () => {
  const host = mount(<Button size="xs">Aceptar</Button>);
  const cls = host.querySelector('button')!.className;
  // El suelo en px: `h-6` solo son 21 con la raíz a 14.
  expect(cls).toContain('min-h-[24px]');
  expect(cls).toContain('text-xs');
  // El gap salió de la clase base a cada talla justamente por esto: `gap-2` y `gap-1` en
  // el mismo elemento no los resuelve el orden de la cadena, sino el de la hoja generada.
  // En un botón de 24 px con icono, 8 px de separación descuadran la caja.
  expect(cls).toContain('gap-1');
  expect(cls).not.toContain('gap-2');
  render(null, host);
});

test('las demás tallas conservan su separación tras sacarla de la base', () => {
  for (const size of ['sm', 'md', 'lg'] as const) {
    const host = mount(<Button size={size}>Guardar</Button>);
    expect(host.querySelector('button')!.className).toContain('gap-2');
    render(null, host);
  }
});

test('el botón solo-icono `xs` es cuadrado de 24 px', () => {
  const host = mount(
    <Button size="xs" iconOnly aria-label="Cerrar">
      <svg />
    </Button>,
  );
  const cls = host.querySelector('button')!.className;
  expect(cls).toContain('min-h-[24px]');
  expect(cls).toContain('min-w-[24px]');
  render(null, host);
});

test('el spinner encoge con la talla, o no cabe en un botón de 24 px', () => {
  const host = mount(
    <Button size="xs" loading>
      Guardando
    </Button>,
  );
  // Spinner pinta un <svg> con width/height explícitos.
  const svg = host.querySelector('svg')!;
  expect(svg.getAttribute('width')).toBe('12');
  render(null, host);
});

test('iconOnly es geometría, no un filtro: el icono pasado como contenido se pinta', () => {
  // Salía vacío en la X de todos los modales, el enviar de Rooms y el menú móvil
  // del sitio público: con su aria-label intacto, así que el teclado funcionaba y
  // no había nada dibujado. Una revisión visual no lo ve si no abre ese modal.
  const host = mount(
    <Button iconOnly aria-label="Cerrar">
      <svg data-glifo />
    </Button>,
  );
  expect(host.querySelector('button svg')).not.toBeNull();
});

test('todas las tallas garantizan 44 px con el dedo, y solo con el dedo', () => {
  // Un botón de 32 px se pulsa bien con un ratón y se falla con la yema. La guía
  // de Apple pide 44, y es la vara con la que se repasaron las doce apps — donde
  // salió que el objetivo pequeño estaba «en casi todas las filas de acciones».
  //
  // Se decide por el PUNTERO y no por el ancho de pantalla: un portátil táctil de
  // quince pulgadas necesita el objetivo grande, y un móvil con ratón conectado no.
  for (const size of ['xs', 'sm', 'md'] as const) {
    const host = mount(<Button size={size}>Aceptar</Button>);
    expect(host.querySelector('button')?.className).toContain(
      'pointer-coarse:min-h-[max(44px,2.75rem)]',
    );
  }
  // `lg` TAMBIÉN LO LLEVA, y esta prueba decía lo contrario.
  //
  // Su premisa era «lg ya mide 44», leyendo `h-11` como 2.75rem = 44px. Pero eso
  // solo es cierto con la raíz en 16px, y LA DE LA SUITE ES 14: ahí `h-11` son
  // 38,5px, que es exactamente el fallo que arregló el commit anterior —«los
  // objetivos táctiles medían 38px para quien reduce el tamaño de letra»—.
  //
  // Por eso el mínimo se escribe `max(44px, 2.75rem)`: los 44 son el suelo
  // absoluto y el rem solo manda cuando la persona ha AGRANDADO la letra. Sin él,
  // `lg` es el único tamaño que se queda corto justo en la configuración por
  // defecto de la casa.
  const grande = mount(<Button size="lg">Aceptar</Button>);
  expect(grande.querySelector('button')?.className).toContain(
    'pointer-coarse:min-h-[max(44px,2.75rem)]',
  );
});

test('el botón de solo icono sube también el ANCHO', () => {
  // Alto y estrecho se sigue fallando: el dedo yerra en los dos ejes.
  const host = mount(
    <Button size="sm" iconOnly aria-label="Borrar">
      <span />
    </Button>,
  );
  const cls = host.querySelector('button')?.className ?? '';
  expect(cls).toContain('pointer-coarse:min-h-[max(44px,2.75rem)]');
  expect(cls).toContain('pointer-coarse:min-w-[max(44px,2.75rem)]');
});

test('los botones RELLENOS llevan la tinta que pone el relleno del glifo en currentColor', async () => {
  // El relleno de un icono es el primario, y sobre un botón primario no se vería
  // (y sobre el de peligro serían dos colores peleando). `styles.css` lo cambia a
  // la propia tinta para quien escribe sobre un color, y lo reconoce por esa
  // tinta: si un día el botón deja de llevarla, el icono vuelve a salir con el
  // acento encima del acento sin que nada más lo diga.
  const css = await Bun.file(new URL('../styles.css', import.meta.url)).text();
  const regla = css.slice(0, css.indexOf('--sytue-glyph-fill: currentColor'));
  const selector = regla.slice(regla.lastIndexOf(':where('));
  // Las dos tintas de «escribo sobre un color»: la del relleno primario y el blanco del
  // de peligro (en oscuro `on-primary` es casi negro y sobre el rojo no contrasta).
  const tintas = ['text-[var(--sytue-on-primary)]', 'text-white'];
  for (const tinta of tintas) {
    expect(selector).toContain('.' + tinta.replace(/[.*+?^${}()|[\]\\]/g, '\\$&'));
  }

  const clases = (variant: 'primary' | 'secondary' | 'ghost' | 'danger'): string[] =>
    (
      mount(<Button variant={variant}>Enviar</Button>).querySelector('button')?.className ?? ''
    ).split(/\s+/);
  const sobreColor = (variant: Parameters<typeof clases>[0]): boolean =>
    clases(variant).some((c) => tintas.includes(c));
  for (const rellena of ['primary', 'secondary', 'danger'] as const) {
    expect(sobreColor(rellena)).toBe(true);
  }
  // El fantasma no tiene relleno: su icono conserva el acento de marca.
  expect(sobreColor('ghost')).toBe(false);
});
