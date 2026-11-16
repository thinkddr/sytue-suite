/**
 * La paleta de la suite (⌘K / Ctrl+K): buscar en Sytue One y, encima, lo que la app
 * en la que se está ha registrado como capa (pantallas, acciones, registros).
 *
 * docs/design.md §3.3 dice que ⌘K «abre command palette, nivel 4»; hasta ahora
 * enfocaba el buscador de la barra salvo en tres apps que se lo quedaban con su propia
 * paleta. La capa visual (campo, lista, teclado, foco, la hoja del teléfono) sigue
 * siendo `PaletaDeComandos` de @sytue/ui; aquí está el QUÉ y el CUÁNDO.
 *
 * - El proveedor (`ProveedorDePaleta`) lo monta `SytueShell`: escucha ⌘K y
 *   `abrirPaleta()`, y guarda si está abierta y qué se ha escrito.
 * - Cada app registra su capa con `useCapaDePaleta(id, capa)` desde dentro de la shell;
 *   al desmontarse, la capa se va con ella.
 * - Con dos letras o más pregunta también al Hub (la búsqueda federada) y pone sus
 *   resultados debajo, agrupados por producto como en el buscador de la barra.
 */
import { createContext, type ComponentChildren, type JSX } from 'preact';
import { useCallback, useContext, useEffect, useMemo, useRef, useState } from 'preact/hooks';
import { appForKey, Kbd, PaletaDeComandos, useAtajo, type OpcionDePaleta } from '@sytue/ui';
import { useI18n } from '@sytue/i18n/preact';
import type { HubClient, SearchResult } from '@sytue/api-clients';
import { detalleDe } from './GlobalSearch.js';
import {
  comandosQueCasan,
  crearRegistroDePaleta,
  esAtajoDePaleta,
  EVENTO_ABRIR_PALETA,
  type CapaDePaleta,
  type ComandoDeApp,
  type RegistroDePaleta,
} from './paleta.js';

/** Si está abierta y qué se ha escrito: lo leen las apps que buscan en su servidor. */
export interface EstadoDePaleta {
  abierta: boolean;
  consulta: string;
  abrir: () => void;
  cerrar: () => void;
}

const RegistroContext = createContext<RegistroDePaleta | null>(null);
const EstadoContext = createContext<EstadoDePaleta | null>(null);

const CERRADA: EstadoDePaleta = { abierta: false, consulta: '', abrir: () => {}, cerrar: () => {} };

/** El estado de la paleta de la suite; fuera de la shell, cerrada y sin efecto. */
export function usePaletaDeLaSuite(): EstadoDePaleta {
  return useContext(EstadoContext) ?? CERRADA;
}

/**
 * Registra lo que la app ofrece en ⌘K mientras el componente está montado.
 *
 * Se puede llamar con una capa nueva en cada render: el registro solo avisa a la
 * paleta, no a la app, así que no hay bucle. Fuera de la shell no hace nada (las
 * pruebas de una vista suelta no la montan).
 */
export function useCapaDePaleta(id: string, capa: CapaDePaleta): void {
  const registro = useContext(RegistroContext);
  useEffect(() => {
    registro?.poner(id, capa);
  }, [registro, id, capa]);
  useEffect(() => () => registro?.quitar(id), [registro, id]);
}

/** Una opción de la lista: o un comando de la app o un resultado del Hub. */
type Opcion = OpcionDePaleta & ({ comando: ComandoDeApp } | { resultado: SearchResult });

export function ProveedorDePaleta({
  hub,
  children,
}: {
  hub: HubClient;
  children?: ComponentChildren;
}): JSX.Element {
  const registro = useMemo(() => crearRegistroDePaleta(), []);
  const [abierta, setAbierta] = useState(false);
  const [consulta, setConsulta] = useState('');
  // El oyente de ⌘K se pone una vez; lee de aquí si ya está abierta para cerrarla.
  const abiertaRef = useRef(abierta);
  useEffect(() => {
    abiertaRef.current = abierta;
  }, [abierta]);

  const abrir = useCallback(() => {
    // Cada apertura empieza en blanco, como Spotlight: lo de la vez anterior estorba.
    setConsulta('');
    setAbierta(true);
  }, []);
  const cerrar = useCallback(() => setAbierta(false), []);

  useEffect(() => {
    const alPulsar = (e: KeyboardEvent): void => {
      if (!esAtajoDePaleta(e) || e.isComposing) return;
      // Una app que se lo ha quedado antes (Notepads lleva ⌘K a su captura, los
      // editores a «insertar enlace») manda.
      if (e.defaultPrevented) return;
      if (abiertaRef.current) {
        e.preventDefault();
        setAbierta(false);
        return;
      }
      // Dentro de otro diálogo, ⌘K es de ese diálogo: abrir otra capa encima lo taparía.
      if (document.querySelector('dialog[open], [role="dialog"], [role="alertdialog"]')) return;
      e.preventDefault();
      abrir();
    };
    document.addEventListener('keydown', alPulsar);
    document.addEventListener(EVENTO_ABRIR_PALETA, abrir);
    return () => {
      document.removeEventListener('keydown', alPulsar);
      document.removeEventListener(EVENTO_ABRIR_PALETA, abrir);
    };
  }, [abrir]);

  const estado = useMemo(
    () => ({ abierta, consulta, abrir, cerrar }),
    [abierta, consulta, abrir, cerrar],
  );

  return (
    <RegistroContext.Provider value={registro}>
      <EstadoContext.Provider value={estado}>
        {children}
        <PaletaDeLaSuite
          registro={registro}
          hub={hub}
          abierta={abierta}
          consulta={consulta}
          onConsulta={setConsulta}
          cerrar={cerrar}
        />
      </EstadoContext.Provider>
    </RegistroContext.Provider>
  );
}

/** Lo que tarda en preguntar al Hub tras la última tecla (el mismo que la barra). */
export const ESPERA_BUSQUEDA_MS = 250;

function PaletaDeLaSuite({
  registro,
  hub,
  abierta,
  consulta,
  onConsulta,
  cerrar,
}: {
  registro: RegistroDePaleta;
  hub: HubClient;
  abierta: boolean;
  consulta: string;
  onConsulta: (q: string) => void;
  cerrar: () => void;
}): JSX.Element | null {
  const { locale, t } = useI18n();
  const atajo = useAtajo('Ctrl K');
  // Las capas viven fuera del estado (ver `RegistroDePaleta`): aquí solo se cuenta que
  // han cambiado, para volver a pintar la lista.
  const [, setVersion] = useState(0);
  useEffect(() => registro.suscribir(() => setVersion((v) => v + 1)), [registro]);

  const [resultados, setResultados] = useState<SearchResult[]>([]);
  const [buscando, setBuscando] = useState(false);
  const [fallo, setFallo] = useState(false);

  const q = consulta.trim();
  useEffect(() => {
    // Mientras se teclea se dejan los resultados de antes hasta que lleguen los nuevos:
    // vaciar la lista en cada letra la hace parpadear.
    if (!abierta || q.length < 2) {
      setResultados([]);
      setFallo(false);
      setBuscando(false);
      return;
    }
    setBuscando(true);
    // Una respuesta que llega tarde, de una consulta ya cambiada, no pisa la nueva.
    let vigente = true;
    const espera = setTimeout(() => {
      hub.search(q).then(
        (r) => {
          if (!vigente) return;
          setResultados(r.results);
          setFallo(false);
          setBuscando(false);
        },
        () => {
          if (!vigente) return;
          setResultados([]);
          setFallo(true);
          setBuscando(false);
        },
      );
    }, ESPERA_BUSQUEDA_MS);
    return () => {
      vigente = false;
      clearTimeout(espera);
    };
  }, [abierta, q, hub]);

  if (!abierta) return null;

  const deApp: Opcion[] = comandosQueCasan(registro.capas(), consulta).map((c) => ({
    id: `app:${c.id}`,
    rotulo: c.rotulo,
    grupo: c.grupo,
    ...(c.icono ? { icono: c.icono } : {}),
    ...(c.pista ? { pista: c.pista } : {}),
    comando: c,
  }));
  // Agrupados por producto en el orden en que llegan (el Hub ya pone delante el mejor).
  const orden: string[] = [];
  for (const r of resultados) if (!orden.includes(r.source)) orden.push(r.source);
  const deSuite: Opcion[] = orden.flatMap((fuente) => {
    const nombre = appForKey(fuente)?.name.replace('Sytue ', '') ?? fuente;
    return resultados
      .filter((r) => r.source === fuente)
      .map((r) => {
        const detalle = detalleDe(locale, r);
        return {
          id: `hub:${r.source}:${r.id}`,
          rotulo: r.title,
          grupo: nombre,
          ...(detalle ? { pista: detalle } : {}),
          resultado: r,
        };
      });
  });

  const vacio = (): string => {
    if (!q) return t('suitePalette.typeToSearch');
    if (buscando || registro.capas().some((c) => c.buscando)) return t('search.busy');
    if (fallo) return t('search.failedShort');
    return t('search.emptyTitle', { query: q });
  };

  return (
    <PaletaDeComandos<Opcion>
      open
      onClose={cerrar}
      etiqueta={t('suitePalette.dialog')}
      rotuloCampo={t('suitePalette.input')}
      placeholder={t('suitePalette.placeholder')}
      rotuloResultados={t('search.results')}
      sinResultados={vacio}
      consulta={consulta}
      onConsulta={onConsulta}
      opciones={[...deApp, ...deSuite]}
      onElegir={(o) => {
        // La paleta se va antes: si lo elegido pregunta algo (dejar una ficha a
        // medias), que la pregunta no quede debajo de otra capa.
        cerrar();
        if ('comando' in o) void o.comando.ejecutar();
        else window.location.assign(o.resultado.url);
      }}
      pie={
        <>
          <span>
            <Kbd>↑↓</Kbd> {t('suitePalette.hint.move')}
          </span>
          <span>
            <Kbd>↵</Kbd> {t('suitePalette.hint.open')}
          </span>
          <span>
            <Kbd>esc</Kbd> {t('suitePalette.hint.close')}
          </span>
          {atajo ? <span class="ml-auto">{atajo}</span> : null}
        </>
      }
    />
  );
}
