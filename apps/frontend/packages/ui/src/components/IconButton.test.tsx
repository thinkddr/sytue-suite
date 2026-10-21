/**
 * Tests de IconButton — lo que se guarda aquí es el contrato ACCESIBLE, que es
 * justamente lo que se perdía en cada copia artesanal del botón-icono: unas llevaban
 * `aria-pressed`, otras solo pintaban el fondo de seleccionado, y unas cuantas no
 * tenían nombre accesible ninguno.
 * Ejecutar: `bun test`.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { expect, test } from 'bun:test';
import { render } from 'preact';
import { IconButton } from './IconButton.js';

// Idempotente: otro fichero de test puede haber registrado ya happy-dom.
if (typeof document === 'undefined') GlobalRegistrator.register();

function montar(ui: preact.JSX.Element): HTMLElement {
  const host = document.createElement('div');
  document.body.appendChild(host);
  render(ui, host);
  return host;
}

test('el icono no habla: el botón siempre tiene nombre accesible', () => {
  const host = montar(
    <IconButton label="Cerrar el panel">
      <svg />
    </IconButton>,
  );
  const btn = host.querySelector('button')!;
  expect(btn.getAttribute('aria-label')).toBe('Cerrar el panel');
  // `type="button"` explícito: dentro de un <form>, el defecto es submit y un botón de
  // cromo enviaría el formulario al pulsarlo.
  expect(btn.getAttribute('type')).toBe('button');
  render(null, host);
});

test('un conmutador anuncia su estado, no solo lo pinta', () => {
  const host = montar(
    <IconButton label="Negrita" pressed>
      <svg />
    </IconButton>,
  );
  const btn = host.querySelector('button')!;
  // Las dos cosas: `aria-pressed` para quien no ve el relleno (WCAG 1.4.1) y el
  // relleno para quien no usa lector de pantalla.
  expect(btn.getAttribute('aria-pressed')).toBe('true');
  expect(btn.className).toContain('bg-selected');
  render(null, host);
});

test('sin `pressed` NO se emite aria-pressed: un botón de acción no es un conmutador', () => {
  const host = montar(
    <IconButton label="Borrar">
      <svg />
    </IconButton>,
  );
  expect(host.querySelector('button')!.hasAttribute('aria-pressed')).toBe(false);
  render(null, host);
});

test('la talla mínima es 24 px, el suelo de WCAG 2.5.8', () => {
  const host = montar(
    <IconButton label="Alejar" size="sm">
      <svg />
    </IconButton>,
  );
  // `h-6` son 21 px con la raíz a 14: lo que da los 24 es el suelo en px. Mirar
  // `h-6` aquí dejaba pasar justo el fallo que el test decía vigilar.
  expect(host.querySelector('button')!.className).toContain('min-h-[24px]');
  expect(host.querySelector('button')!.className).toContain('min-w-[24px]');
  render(null, host);
});

test('el anillo de foco se pide por su clase, no repitiendo la sombra', () => {
  const host = montar(
    <IconButton label="Buscar">
      <svg />
    </IconButton>,
  );
  // Antes cada componente reescribía la sombra completa y podía perder un anillo por su
  // cuenta —pasó 163 veces—. Ahora solo NOMBRA la utilidad; que esa utilidad lleve el
  // contorno de contraste lo guarda el test de styles.css, que es donde ya vive.
  expect(host.querySelector('button')!.className).toContain('sytue-focus-ring');
  render(null, host);
});

test('las clases propias se añaden, no sustituyen a las del sistema', () => {
  const host = montar(
    <IconButton label="Zoom" class="ml-auto">
      <svg />
    </IconButton>,
  );
  const cls = host.querySelector('button')!.className;
  expect(cls).toContain('ml-auto');
  expect(cls).toContain('inline-flex'); // la base sobrevive
  render(null, host);
});

test('el tono `header` usa los velos del CROMO, no los de superficie', () => {
  const host = montar(
    <IconButton label="Cerrar" tone="header">
      <svg />
    </IconButton>,
  );
  const cls = host.querySelector('button')!.className;
  // Un `hover:bg-bg-subtle` sobre la cabecera grafito pinta un velo CLARO y deja el
  // icono a 1,1:1: es el defecto que costó una ronda entera de auditoría.
  expect(cls).toContain('chrome-hover');
  expect(cls).not.toContain('bg-bg-subtle');
  expect(cls).toContain('rounded-control');
  render(null, host);
});

test('el tono `media` trae su fondo: lo de debajo es una foto, no una superficie del tema', () => {
  const host = montar(
    <IconButton label="Foto siguiente" tone="media">
      <svg />
    </IconButton>,
  );
  const cls = host.querySelector('button')!.className;
  // Fondo oscuro con icono blanco: 5,7:1 sobre una foto blanca, que es el peor caso.
  // Un token de superficie no vale: en el tema traslúcido deja ver la foto.
  expect(cls).toContain('bg-black/60');
  expect(cls).toContain('text-white');
  expect(cls).not.toContain('bg-surface');
  expect(cls).toContain('rounded-control');
  // Sin filete, sobre una foto negra la placa negra medía 1:1 contra la foto: del botón
  // solo quedaba el glifo. Con el blanco al 40 %, el borde queda a 3,7:1.
  expect(cls).toContain('border-white/40');
  render(null, host);
});

test('`xl` mide 48 px con cualquier raíz: la talla de las flechas de un visor', () => {
  const host = montar(
    <IconButton label="Foto anterior" tone="media" size="xl">
      <svg />
    </IconButton>,
  );
  const cls = host.querySelector('button')!.className;
  expect(cls).toContain('min-h-[48px]');
  expect(cls).toContain('min-w-[48px]');
  render(null, host);
});

test('con `href` es un enlace de verdad, con el mismo nombre y el mismo aspecto', () => {
  const host = montar(
    <IconButton label="Fotos parecidas" href="/photos/similar?id=1">
      <svg />
    </IconButton>,
  );
  expect(host.querySelector('button')).toBeNull();
  const a = host.querySelector('a')!;
  expect(a.getAttribute('href')).toBe('/photos/similar?id=1');
  expect(a.getAttribute('aria-label')).toBe('Fotos parecidas');
  expect(a.className).toContain('sytue-focus-ring');
  render(null, host);
});

test('un enlace inerte pierde el `href`: `disabled` en un <a> no hace nada', () => {
  const host = montar(
    <IconButton label="Editar" href="/photos/edit?id=1" disabled>
      <svg />
    </IconButton>,
  );
  const a = host.querySelector('a')!;
  expect(a.hasAttribute('href')).toBe(false);
  expect(a.getAttribute('aria-disabled')).toBe('true');
  render(null, host);
});
