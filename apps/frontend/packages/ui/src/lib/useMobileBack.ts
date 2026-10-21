/**
 * «Atrás» del navegador recoge primero la interfaz móvil abierta. Una sola
 * entrada temporal sirve a todas las capas: cerrar una interior no deja pasos
 * vacíos en el historial. La URL y el estado del router se conservan.
 */
import { useLayoutEffect } from 'preact/hooks';
import { useUltimo } from './useUltimo.js';

const CLAVE = '__sytueMobileBack';
const DEDO = '(pointer: coarse)';

interface Marca {
  sesion: string;
  paso: number;
  previa: unknown;
  url: string;
}

function marcaDe(estado: unknown): Marca | undefined {
  if (!estado || typeof estado !== 'object') return;
  const marca = (estado as Record<string, unknown>)[CLAVE];
  if (!marca || typeof marca !== 'object') return;
  const m = marca as Partial<Marca>;
  if (typeof m.sesion === 'string' && typeof m.paso === 'number' && typeof m.url === 'string') {
    return m as Marca;
  }
}

/** Separado del hook para comprobar los recorridos de historial sin un navegador. */
export function crearHistorialMovil(
  ventana: Window,
): (cerrar: () => void, nivel: number) => () => void {
  const sesion = `sytue-${Date.now()}-${Math.random()}`;
  const capas: { cerrar: () => void; nivel: number }[] = [];
  let paso = 0;
  let actual: Marca | undefined;
  let retirando: Marca | undefined;
  let recuperando = false;

  const esActual = (): boolean => {
    const m = marcaDe(ventana.history.state);
    return m?.sesion === sesion && m.paso === actual?.paso && ventana.location.href === actual?.url;
  };
  const armar = (): void => {
    if (!capas.length || retirando || recuperando || (actual && esActual())) return;
    const previa: unknown = ventana.history.state;
    actual = { sesion, paso: ++paso, previa, url: ventana.location.href };
    ventana.history.pushState(
      { ...(previa && typeof previa === 'object' ? previa : {}), [CLAVE]: actual },
      '',
    );
  };
  const retirar = (): void => {
    if (!actual) return;
    if (esActual()) {
      retirando = actual;
      actual = undefined;
      ventana.history.back();
    } else {
      // El router ya ha navegado (p. ej. elegir un canal de Rooms). Su entrada
      // manda: no retroceder aquí ni sobrescribir su state/URL.
      actual = undefined;
    }
  };

  ventana.addEventListener(
    'popstate',
    (evento: PopStateEvent) => {
      const entrante = marcaDe(evento.state);
      if (recuperando) {
        recuperando = false;
        evento.stopImmediatePropagation();
        armar();
        return;
      }
      if (retirando) {
        const propia = ventana.location.href === retirando.url;
        const retirada = retirando;
        retirando = undefined;
        if (entrante?.sesion === sesion && entrante.paso === retirada.paso) {
          // El router hizo pushState entre solicitar back y recibir popstate.
          // Este back ha consumido SU navegación: devolverla antes de continuar.
          recuperando = true;
          evento.stopImmediatePropagation();
          ventana.history.forward();
          return;
        }
        if (propia) evento.stopImmediatePropagation();
        armar();
        return;
      }
      if (entrante?.sesion === sesion) {
        // Avanzar o volver a una antigua entrada temporal no reabre nada.
        // El router recibe su state original, también si navegó con la capa abierta.
        ventana.history.replaceState(entrante.previa, '');
        return;
      }
      if (!actual || ventana.location.href !== actual.url) return;
      actual = undefined;
      evento.stopImmediatePropagation();
      const superior = capas.reduce<(typeof capas)[number] | undefined>(
        (mejor, capa) => (!mejor || capa.nivel >= mejor.nivel ? capa : mejor),
        undefined,
      );
      superior?.cerrar();
      // Preact confirma el cierre antes de rearmar. Si onClose abre una
      // confirmación o decide conservar el diálogo, sigue habiendo una salida.
      queueMicrotask(armar);
    },
    true,
  );

  return (cerrar, nivel) => {
    const capa = { cerrar, nivel };
    capas.push(capa);
    armar();
    return () => {
      const indice = capas.indexOf(capa);
      if (indice < 0) return;
      capas.splice(indice, 1);
      if (!capas.length) retirar();
    };
  };
}

const historiales = new WeakMap<Window, ReturnType<typeof crearHistorialMovil>>();

/**
 * Solo con el dedo; los cajones que desaparecen en escritorio se limitan al ancho.
 * Los niveles expresan el apilamiento: detalle 0, cajón 1 y capas temporales 2.
 * Entre capas manda la última abierta: un menú puede abrir un diálogo y viceversa.
 */
export function useMobileBack(
  abierto: boolean,
  cerrar: () => void,
  {
    estrecho = false,
    anchoMaximo,
    nivel = 2,
  }: { estrecho?: boolean; anchoMaximo?: string; nivel?: number } = {},
): void {
  const ultimo = useUltimo(cerrar);
  useLayoutEffect(() => {
    if (!abierto || typeof window === 'undefined' || typeof window.matchMedia !== 'function')
      return;
    // El PDF flota hasta lg (64rem); el resto de cajones, hasta md (48rem).
    const ancho = anchoMaximo ?? (estrecho ? '48rem' : undefined);
    const medio = window.matchMedia(ancho ? `${DEDO} and (width < ${ancho})` : DEDO);
    let soltar: (() => void) | undefined;
    const sincronizar = (): void => {
      if (medio.matches && !soltar) {
        let registrar = historiales.get(window);
        if (!registrar) {
          registrar = crearHistorialMovil(window);
          historiales.set(window, registrar);
        }
        soltar = registrar(() => ultimo.current(), nivel);
      } else if (!medio.matches && soltar) {
        soltar();
        soltar = undefined;
      }
    };
    sincronizar();
    medio.addEventListener?.('change', sincronizar);
    return () => {
      medio.removeEventListener?.('change', sincronizar);
      soltar?.();
    };
  }, [abierto, estrecho, anchoMaximo, nivel, ultimo]);
}
