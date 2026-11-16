/**
 * La paleta de la suite (⌘K), sin Preact: el registro de capas, qué casa con qué y la
 * orden de abrirla desde cualquier sitio.
 *
 * Había tres paletas con el mismo atajo —Sytue Cloud, el ERP y Sytue Admin— y el
 * buscador de la barra escuchando también ⌘K: convivían porque cada una le ganaba a la
 * siguiente con `preventDefault`, y el resultado era que ⌘K hacía una cosa distinta en
 * cada app (buscar en la suite, saltar a un modelo, ir a una sección). Ahora ⌘K abre
 * SIEMPRE la misma paleta, la de la shell, que busca en la suite y ofrece encima lo que
 * cada app registra en ella como una CAPA: sus pantallas, sus acciones, sus registros.
 */
import { filterCommands, type IconName } from '@sytue/ui';

/** Lo que una app ofrece en la paleta: los textos llegan YA traducidos. */
export interface ComandoDeApp {
  id: string;
  /** Lo que se lee, y lo primero en que se busca. */
  rotulo: string;
  /** Encabezado del grupo; también entra en la búsqueda («acciones» encuentra el grupo). */
  grupo: string;
  icono?: IconName;
  /** Contexto a la derecha (el área, el slug). */
  pista?: string;
  /** Términos que se buscan pero no se ven (alias, sinónimos, la pista). */
  palabras?: string;
  /**
   * Ya viene buscado por el servidor con la consulta actual (personas del directorio):
   * no pasa otra vez por el filtro de palabras, que lo descartaría por el grupo.
   */
  yaFiltrado?: boolean;
  /** Lo que pasa al elegirlo: navegar o hacer algo de verdad, nunca un no-op. */
  ejecutar: () => void | Promise<void>;
}

/** Una capa: lo que registra una app (o una parte de ella) mientras está montada. */
export interface CapaDePaleta {
  comandos: readonly ComandoDeApp[];
  /** Cuántas opciones de esta capa caben como mucho (el ERP tiene cientos de modelos). */
  limite?: number;
  /**
   * La app aún está buscando lo de esta capa en su servidor (los registros del ERP):
   * mientras, la paleta no afirma que no haya nada.
   */
  buscando?: boolean;
}

/**
 * Lo que casa con la consulta, capa a capa en el orden en que se registraron y dentro
 * de cada una en el suyo (que ya llega agrupado). Pliega acentos y mayúsculas: el
 * filtro es `filterCommands` de @sytue/ui, el mismo que usaban las tres paletas.
 */
export function comandosQueCasan(capas: readonly CapaDePaleta[], consulta: string): ComandoDeApp[] {
  const salida: ComandoDeApp[] = [];
  for (const capa of capas) {
    const buscables = capa.comandos
      .filter((c) => !c.yaFiltrado)
      .map((c) => ({
        label: c.rotulo,
        group: c.grupo,
        keywords: [c.palabras, c.pista].filter(Boolean).join(' '),
        c,
      }));
    const casan = [
      ...filterCommands(buscables, consulta).map((b) => b.c),
      ...capa.comandos.filter((c) => c.yaFiltrado),
    ];
    salida.push(...(capa.limite !== undefined ? casan.slice(0, capa.limite) : casan));
  }
  return salida;
}

/**
 * El registro de capas de UNA shell. Vive fuera del estado de Preact a propósito: una
 * app registra en cada render (sus comandos dependen de lo que tenga cargado) y, si eso
 * fuera estado del proveedor, cada registro volvería a pintar la app que registra.
 */
export interface RegistroDePaleta {
  poner: (id: string, capa: CapaDePaleta) => void;
  quitar: (id: string) => void;
  capas: () => CapaDePaleta[];
  /** Avisa cuando cambia alguna capa; devuelve cómo dejar de escuchar. */
  suscribir: (alCambiar: () => void) => () => void;
}

export function crearRegistroDePaleta(): RegistroDePaleta {
  const capas = new Map<string, CapaDePaleta>();
  const oyentes = new Set<() => void>();
  const avisar = () => oyentes.forEach((f) => f());
  return {
    poner(id, capa) {
      if (capas.get(id) === capa) return;
      capas.set(id, capa);
      avisar();
    },
    quitar(id) {
      if (capas.delete(id)) avisar();
    },
    capas: () => [...capas.values()],
    suscribir(alCambiar) {
      oyentes.add(alCambiar);
      return () => oyentes.delete(alCambiar);
    },
  };
}

/** El evento con el que cualquier botón abre la paleta, esté o no dentro de la shell. */
export const EVENTO_ABRIR_PALETA = 'sytue:abrir-paleta';

/**
 * Abre la paleta de la suite. Es un evento y no un hook porque los botones que la
 * abren suelen vivir en el componente que MONTA la shell (la barra lateral del ERP, la
 * de Cloud), por encima de su contexto.
 */
export function abrirPaleta(): void {
  if (typeof document === 'undefined') return;
  document.dispatchEvent(new CustomEvent(EVENTO_ABRIR_PALETA));
}

/** ⌘K en Mac, Ctrl+K en el resto: sin Alt ni Mayúsculas, que son otros atajos. */
export function esAtajoDePaleta(
  e: Pick<KeyboardEvent, 'key' | 'metaKey' | 'ctrlKey' | 'altKey' | 'shiftKey'>,
): boolean {
  return e.key.toLowerCase() === 'k' && (e.metaKey || e.ctrlKey) && !e.altKey && !e.shiftKey;
}
