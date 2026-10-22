/*
 * LO ESCRITO DEPRISA NO SE PIERDE.
 *
 * El texto de una nota solo salía de la tarjeta al PERDER EL FOCO, y hasta ese
 * momento vivía nada más que en el estado del componente. Pulsar en el lienzo
 * vacío no cuenta como perder el foco —no deja `relatedTarget`—, así que la nota
 * parecía cerrada sin estarlo: recargar o cambiar de app se llevaba lo escrito.
 *
 * Se comprueba lo que de verdad importa: que escribir y esperar guarda sin
 * tocar nada más, y que si la tarjeta desaparece con algo a medias, se vuelca
 * antes de irse. Y que los guardados de una misma edición van con la MISMA
 * marca de sesión, que es lo que mantiene el deshacer en una sola entrada.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterEach, expect, test } from 'bun:test';
import { render } from 'preact';
import type { Note } from '@sytue/api-clients';
import { NoteCard, esperaDeAutoguardado } from './NoteCard.js';

if (typeof document === 'undefined') GlobalRegistrator.register();

const t = ((k: string) => k) as never;
const nota = { id: 'n1', kind: 'text', title: 'una', content: '<p>viejo</p>' } as Note;

type Guardado = { patch: { content?: string }; opts?: { fusion?: string } };

// Cada test se lleva su tarjeta al terminar. Dos tarjetas vivas a la vez se
// pisan el foco —la que se monta llama a `focus()` y la otra recibe un `blur`,
// que guarda—, y eso hacía fallar a la primera de ellas según el orden.
let montadas: HTMLElement[] = [];
afterEach(() => {
  for (const h of montadas) {
    render(null, h);
    h.remove();
  }
  montadas = [];
});

function montar(): { host: HTMLElement; guardados: Guardado[] } {
  const guardados: Guardado[] = [];
  const host = document.createElement('div');
  montadas.push(host);
  document.body.appendChild(host);
  render(
    <NoteCard
      t={t}
      note={nota}
      seleccionada={false}
      onSelect={() => {}}
      onChange={(patch, opts) => guardados.push({ patch, ...(opts ? { opts } : {}) })}
      onDelete={() => {}}
      onDuplicate={() => {}}
      onTogglePin={() => {}}
      onShare={() => {}}
      onTags={() => {}}
      onBacklinks={() => {}}
    />,
    host,
  );
  return { host, guardados };
}

const esperar = (ms: number) => new Promise((r) => setTimeout(r, ms));
/** Preact aplica los cambios de estado en microtarea: sin esto el DOM va un paso atrás. */
const repintar = () => esperar(0);

/** Entra en edición y escribe, como haría quien apunta algo deprisa. */
async function escribir(host: HTMLElement, texto: string): Promise<void> {
  // El cuerpo de la nota es el `role="button"` que abre el editor al pulsarlo.
  // Solo la primera vez: con el editor ya abierto ese selector cae en otro
  // botón, y pulsarlo saca el foco de la nota —lo que en la app guarda—.
  if (!host.querySelector('[contenteditable]')) {
    (host.querySelector('[role="button"]') as HTMLElement | null)?.click();
    await repintar();
  }
  const campo = host.querySelector('[contenteditable]') as HTMLElement | null;
  if (!campo) throw new Error('no se abrió el editor');
  campo.innerHTML = texto;
  campo.dispatchEvent(new Event('input', { bubbles: true }));
  await repintar();
}

test('escribir y parar guarda solo, sin salir de la nota', async () => {
  const { host, guardados } = montar();
  await escribir(host, '<p>nuevo</p>');

  await esperar(1500);
  // Se guarda una vez y con el texto escrito, sin que nadie salga de la nota.
  expect(guardados.map((g) => g.patch.content)).toEqual(['<p>nuevo</p>']);
});
// Que además NO cierre la nota se comprueba a mano: aquí no se puede afirmar,
// porque los efectos pendientes de otros ficheros de test llaman a `focus()`
// sobre el mismo `document` y el editor pierde el foco a mitad de la espera
// —que en la app guarda y cierra, como debe—.

test('si la tarjeta se va con algo a medias, se vuelca antes', async () => {
  const { host, guardados } = montar();
  await escribir(host, '<p>a medias</p>');
  // Desaparece antes de que venza la pausa: cambia el filtro, se cierra el cuaderno.
  render(null, host);
  expect(guardados.map((g) => g.patch.content)).toEqual(['<p>a medias</p>']);
});

test('los guardados de una misma edición llevan la misma sesión', async () => {
  const { host, guardados } = montar();
  await escribir(host, '<p>uno</p>');
  await esperar(1500);
  await escribir(host, '<p>uno dos</p>');
  await esperar(1500);
  expect(guardados).toHaveLength(2);
  expect(guardados[0]?.opts?.fusion).toBeTruthy();
  expect(guardados[0]?.opts?.fusion).toBe(guardados[1]?.opts?.fusion as string);
});

/*
 * EL TECHO DE ESPERA, aparte y sin temporizadores.
 *
 * La pausa sola no basta: escribiendo a 60 ppm caen ~200 ms entre teclas, asi
 * que un mecanografo rapido puede no dejar NUNCA el hueco de 400 ms y pasarse
 * media nota sin que salga un byte. Es el caso que el debounce no cubre.
 *
 * Se prueba la cuenta y no el reloj: hacerlo tecleando de verdad significaria
 * esperar dos segundos y medio en el arnes compartido, donde ya se vio que
 * otros ficheros roban el foco a mitad de espera.
 */
test('la espera cede al techo cuando se escribe sin parar', () => {
  const t0 = 1_000_000;
  // Recien ensuciado: manda la pausa normal.
  expect(esperaDeAutoguardado(t0, t0)).toBe(400);
  expect(esperaDeAutoguardado(t0, t0 + 1_000)).toBe(400);
  // Cerca del techo: se acorta para no pasarse de los dos segundos.
  expect(esperaDeAutoguardado(t0, t0 + 1_800)).toBe(200);
  // En el techo o pasado: ya.
  expect(esperaDeAutoguardado(t0, t0 + 2_000)).toBe(0);
  expect(esperaDeAutoguardado(t0, t0 + 5_000)).toBe(0);
});
