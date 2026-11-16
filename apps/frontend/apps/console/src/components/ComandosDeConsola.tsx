/**
 * ComandosDeConsola — lo que Sytue Admin ofrece en ⌘K: SECCIONES (las que puede ver
 * quien administra), ACCIONES y PERSONAS del directorio.
 *
 * Sytue Admin tenía su propia paleta y le quitaba ⌘K a la de la barra en fase de
 * captura. Ahora la paleta es la de la suite (`PaletaDeLaSuite` de @sytue/shell), que
 * busca además en todas las apps, y aquí solo se registra la capa de la consola: qué se
 * ofrece está en `lib/paleta.ts`. No pinta nada.
 */
import { useEffect } from 'preact/hooks';
import { useCapaDePaleta, usePaletaDeLaSuite, type ComandoDeApp } from '@sytue/shell';
import type { T } from '../i18n/index.js';
import { comandosDeConsola, type AccionDePaleta, type SeccionDeMenu } from '../lib/paleta.js';
import { useUserSearch } from '../lib/useUserSearch.js';

interface ComandosDeConsolaProps {
  t: T;
  nav: readonly SeccionDeMenu[];
  allowed?: readonly string[] | undefined;
  /** Ruta interna → URL navegable (con prefijo y org). */
  ruta: (p: string) => string;
}

// Claves literales, no plantillas: así el control de claves usadas las ve.
const TEXTOS_DE_ACCION: Record<AccionDePaleta, (t: T) => { label: string; keywords: string }> = {
  invitar: (t) => ({
    label: t('palette.action.invitar'),
    keywords: t('palette.action.invitar.keywords'),
  }),
  'crear-grupo': (t) => ({
    label: t('palette.action.crear-grupo'),
    keywords: t('palette.action.crear-grupo.keywords'),
  }),
  'conectar-app': (t) => ({
    label: t('palette.action.conectar-app'),
    keywords: t('palette.action.conectar-app.keywords'),
  }),
  'conectar-sso': (t) => ({
    label: t('palette.action.conectar-sso'),
    keywords: t('palette.action.conectar-sso.keywords'),
  }),
};

const SIN_EXCLUIDOS = new Set<string>();
const SIN_CARGADOS: never[] = [];

export default function ComandosDeConsola({ t, nav, allowed, ruta }: ComandosDeConsolaProps): null {
  const { consulta } = usePaletaDeLaSuite();
  const { setQuery, candidates } = useUserSearch(SIN_CARGADOS, SIN_EXCLUIDOS);
  // Las personas las busca el servidor con lo que se escribe en la paleta.
  useEffect(() => setQuery(consulta), [consulta, setQuery]);

  const comandos = comandosDeConsola(
    nav,
    allowed,
    {
      grupoSecciones: t('palette.group.sections'),
      grupoAcciones: t('palette.group.actions'),
      grupoPersonas: t('palette.group.people'),
      accion: (a) => TEXTOS_DE_ACCION[a](t),
    },
    consulta.trim() ? candidates : [],
  );
  useCapaDePaleta('console', {
    comandos: comandos.map((c): ComandoDeApp => ({
      id: c.id,
      rotulo: c.label,
      grupo: c.group,
      icono: c.icon,
      ...(c.hint ? { pista: c.hint } : {}),
      ...(c.keywords ? { palabras: c.keywords } : {}),
      // Las personas ya llegan buscadas por el servidor (nombre o correo).
      ...(c.id.startsWith('persona:') ? { yaFiltrado: true } : {}),
      ejecutar: () => {
        window.location.href = ruta(c.href);
      },
    })),
  });
  return null;
}
