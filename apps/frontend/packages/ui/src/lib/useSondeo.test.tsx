/**
 * Tests de `useSondeo`. Lo que se comprueba es que NO consulta cuando nadie mira:
 * un `setInterval` suelto seguía pidiendo al servidor con la pestaña de fondo, y
 * eso son 720 consultas/hora solo del panel de logs de Sytue Admin (dos cada diez
 * segundos), 240 del «Visto» de un DM y 240 de las versiones de plataforma.
 *
 * Se monta un componente de verdad —igual que el resto de tests del paquete— en
 * vez de mockear `preact/hooks`: el mock de un módulo es global al proceso de
 * `bun test` y se lleva por delante a los demás ficheros. Ejecutar: `bun test`.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterEach, describe, expect, setSystemTime, test } from 'bun:test';
import { render } from 'preact';
import { act } from 'preact/test-utils';
import { useAhora, useSondeo } from './useSondeo.js';

// Idempotente: otro fichero de test puede haber registrado ya happy-dom.
if (typeof document === 'undefined') GlobalRegistrator.register();

/** Pone `document.hidden` (que es de solo lectura) y avisa como el navegador. */
function verVisibilidad(oculto: boolean): void {
  act(() => {
    Object.defineProperty(document, 'hidden', { value: oculto, configurable: true });
    document.dispatchEvent(new Event('visibilitychange'));
  });
}

/** Monta un componente que no pinta nada y solo sondea. Devuelve cómo desmontarlo. */
function montarSondeo(tarea: () => void, ms: number, activo = true): () => void {
  const host = document.createElement('div');
  document.body.appendChild(host);
  function Sonda() {
    useSondeo(tarea, ms, activo);
    return null;
  }
  act(() => {
    render(<Sonda />, host);
  });
  return () => {
    act(() => {
      render(null, host);
    });
    host.remove();
  };
}

afterEach(() => {
  Object.defineProperty(document, 'hidden', { value: false, configurable: true });
});

describe('useSondeo', () => {
  test('consulta a su cadencia mientras la pestaña se ve', async () => {
    let veces = 0;
    const desmontar = montarSondeo(() => veces++, 10);
    await Bun.sleep(45);
    expect(veces).toBeGreaterThanOrEqual(2);
    desmontar();
  });

  // El motivo de que este hook exista.
  test('deja de consultar con la pestaña de fondo', async () => {
    let veces = 0;
    const desmontar = montarSondeo(() => veces++, 10);
    await Bun.sleep(30);
    verVisibilidad(true);
    const cuandoSeOculto = veces;
    await Bun.sleep(60); // seis cadencias enteras a oscuras
    expect(veces).toBe(cuandoSeOculto);
    desmontar();
  });

  test('al volver a primer plano consulta en el acto, sin esperar al siguiente tick', async () => {
    let veces = 0;
    // Cadencia larguísima: si consulta, es por la vuelta y no por el temporizador.
    const desmontar = montarSondeo(() => veces++, 60_000);
    await Bun.sleep(5);
    expect(veces).toBe(0);
    verVisibilidad(true);
    verVisibilidad(false);
    expect(veces).toBe(1);
    desmontar();
  });

  test('no arranca si nace con la pestaña ya oculta', async () => {
    Object.defineProperty(document, 'hidden', { value: true, configurable: true });
    let veces = 0;
    const desmontar = montarSondeo(() => veces++, 10);
    await Bun.sleep(45);
    expect(veces).toBe(0);
    desmontar();
  });

  test('con activo=false no sondea', async () => {
    let veces = 0;
    const desmontar = montarSondeo(() => veces++, 10, false);
    await Bun.sleep(45);
    expect(veces).toBe(0);
    desmontar();
  });

  test('al desmontar no queda temporizador latiendo', async () => {
    let veces = 0;
    const desmontar = montarSondeo(() => veces++, 10);
    desmontar();
    await Bun.sleep(45);
    expect(veces).toBe(0);
  });
});

test('el reloj avanza, pausa oculto y se actualiza inmediatamente al volver', async () => {
  setSystemTime(new Date('2040-01-01T00:00:00Z'));
  const host = document.createElement('div');
  document.body.appendChild(host);
  function Reloj() {
    const ahora = useAhora(10);
    return <span>{ahora.toISOString().slice(0, 10)}</span>;
  }
  try {
    act(() => render(<Reloj />, host));
    expect(host.textContent).toBe('2040-01-01');
    setSystemTime(new Date('2040-01-02T00:00:00Z'));
    await act(async () => {
      await Bun.sleep(45);
    });
    expect(host.textContent).toBe('2040-01-02');
    verVisibilidad(true);
    setSystemTime(new Date('2040-01-03T00:00:00Z'));
    await act(async () => {
      await Bun.sleep(45);
    });
    expect(host.textContent).toBe('2040-01-02');
    verVisibilidad(false);
    expect(host.textContent).toBe('2040-01-03');
  } finally {
    act(() => render(null, host));
    host.remove();
    setSystemTime();
  }
});
