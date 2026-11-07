/*
 * LA PAUSA RECIÉN CONFIRMADA NO LA PISA UNA LECTURA VIEJA, Y CON UNA ESCRITURA EN
 * VUELO EL MENÚ VA INERTE.
 *
 * Se monta el gancho que usa ChatApp —no la utilidad suelta (`pausa.test.ts`)— para
 * que quitar `vigente()` de la relectura o los `escribir()` de la pausa lo ponga en
 * rojo. Gemelo en Fyne: TestUnaLecturaViejaNoPisaLaPausa y
 * TestUnaPausaEnVueloNoDejaPedirOtra (apps/fyne/apps/rooms/pausa_test.go).
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterAll, afterEach, expect, test } from 'bun:test';
import { render } from 'preact';
import { useLayoutEffect } from 'preact/hooks';
import { act } from 'preact/test-utils';
import type { NotifyPrefs } from '../client/chatApi.js';
import { usePreferenciasAviso, type ApiPreferenciasAviso } from './usePreferenciasAviso.js';

// Solo aquí, y se retira al acabar: el resto de la suite usa el fetch de Bun.
const registrado = typeof document === 'undefined';
if (registrado) GlobalRegistrator.register();
afterAll(async () => {
  if (registrado) await GlobalRegistrator.unregister();
});

/** Una promesa que se resuelve a mano, para decidir el orden de las respuestas. */
function aMano<T>() {
  let resolver!: (v: T) => void;
  const promesa = new Promise<T>((r) => {
    resolver = r;
  });
  return { promesa, resolver };
}

/** Responde y deja correr las cadenas `then`/`finally` antes de mirar. */
async function responder<T>(p: { resolver: (v: T) => void }, v: T): Promise<void> {
  await act(async () => {
    p.resolver(v);
    await new Promise((r) => setTimeout(r, 0));
  });
}

const SIN_PAUSA: NotifyPrefs = { keywords: [], dndStart: 0, dndEnd: 0 };

let host: HTMLElement | null = null;
afterEach(() => {
  if (host) {
    render(null, host);
    host.remove();
  }
  host = null;
});

type Gancho = ReturnType<typeof usePreferenciasAviso>;

async function montar(api: ApiPreferenciasAviso): Promise<() => Gancho> {
  const sonda: { gancho?: Gancho } = {};
  function Sonda() {
    const gancho = usePreferenciasAviso(() => undefined, api);
    useLayoutEffect(() => {
      sonda.gancho = gancho;
    });
    return null;
  }
  host = document.createElement('div');
  document.body.appendChild(host);
  await act(async () => {
    render(<Sonda />, host!);
  });
  return () => sonda.gancho!;
}

test('una lectura lanzada antes de pausar no pisa la pausa confirmada', async () => {
  const lecturas: ReturnType<typeof aMano<{ prefs: NotifyPrefs }>>[] = [];
  const escrituras: ReturnType<typeof aMano<{ prefs: NotifyPrefs }>>[] = [];
  const gancho = await montar({
    getNotifyPrefs: () => {
      const l = aMano<{ prefs: NotifyPrefs }>();
      lecturas.push(l);
      return l.promesa;
    },
    setNotifyPause: () => {
      const e = aMano<{ prefs: NotifyPrefs }>();
      escrituras.push(e);
      return e.promesa;
    },
  });
  await responder(lecturas[0]!, { prefs: SIN_PAUSA }); // la del montaje

  await act(async () => gancho().recargar()); // abrir el menú: relee…
  await act(async () => gancho().pausar('1h')); // …y enseguida pausa
  expect(gancho().pausaGuardando).toBe(true);

  const hasta = new Date(Date.now() + 3_600_000).toISOString();
  await responder(escrituras[0]!, { prefs: { ...SIN_PAUSA, pausedUntil: hasta } });
  expect(gancho().notifyPrefs.pausedUntil).toBe(hasta);
  expect(gancho().pausaGuardando).toBe(false);

  // La lectura la atendió el servicio antes que la pausa: trae «sin pausa».
  await responder(lecturas[1]!, { prefs: SIN_PAUSA });
  expect(gancho().notifyPrefs.pausedUntil).toBe(hasta);

  // Lanzada con la escritura en vuelo también es vieja: la confirmación cuenta otro turno.
  const luego = new Date(Date.now() + 7_200_000).toISOString();
  await act(async () => gancho().pausar('2h'));
  await act(async () => gancho().recargar());
  await responder(escrituras[1]!, { prefs: { ...SIN_PAUSA, pausedUntil: luego } });
  await responder(lecturas[2]!, { prefs: { ...SIN_PAUSA, pausedUntil: hasta } });
  expect(gancho().notifyPrefs.pausedUntil).toBe(luego);

  // Una lectura lanzada después sí vale: si se quitó en otro dispositivo, llega.
  await act(async () => gancho().recargar());
  await responder(lecturas[3]!, { prefs: SIN_PAUSA });
  expect(gancho().notifyPrefs.pausedUntil).toBeUndefined();
});
