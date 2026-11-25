/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * «Traer mis datos» — el asistente de importación, DENTRO del producto.
 *
 * El motor de migración (Google Workspace / Microsoft 365 → Sytue) existía entero y solo
 * tenía puerta en la consola de administración. Quien llega a Sytue y abre Mail con el
 * buzón vacío no va a pedirle a su administrador que le migre el correo: necesita el
 * botón donde está mirando.
 *
 * Un solo componente para los cuatro productos. Lo que cambia por app es QUÉ contenido
 * trae —y eso es un parámetro—, así que cuatro copias de esto solo divergirían.
 *
 * No aparece si la organización no ha conectado ningún origen: en ese caso no hay nada
 * que ofrecer, y un botón que siempre falla es peor que ningún botón.
 *
 * Un botón por CUENTA, no por conexión: la misma cuenta de Google conectada varias veces
 * —o para Drive, Meet y Chat además de para migrar— salía como varios «Traerlo de
 * Google» idénticos. Ahora el servidor unifica (`/migrations/self/origins`), el BFF lo
 * vuelve a comprobar, y aquí el botón lleva la cuenta cuando hay más de una del mismo
 * proveedor. Lo que decide sin pintar vive en `importacion.ts`.
 */
import type { JSX } from 'preact';
import { useEffect, useRef, useState } from 'preact/hooks';
import { Button, DataFlow, Icon, Spinner } from '@sytue/ui';
import type { DataCategory } from '@sytue/ui';
import { formatDate } from '@sytue/i18n';
import { useI18n } from '@sytue/i18n/preact';
import { latestImports } from '@sytue/api-clients';
import type { ImportDomain, ImportJob, ImportOrigin, ImportSource } from '@sytue/api-clients';
import {
  MARCA,
  claveDeFallo,
  contarTraidos,
  cuentaDeOrigen,
  enCurso,
  esDeOrigen,
  proveedoresRepetidos,
  repartir,
} from './importacion.js';

export interface ImportDataProps {
  /**
   * Qué contenido trae ESTE producto. Mail pide `mail`; Contacts, `contacts`. Se manda
   * tal cual al servidor, que solo acepta contenido personal.
   */
  domains: ImportDomain[];
  /**
   * Endpoint BFF de la app que hace de proxy al gateway. Cada superficie tiene el suyo
   * —la cookie de sesión es host-only— pero todos hablan el mismo contrato.
   */
  endpoint?: string;
  /** Se llama al terminar una importación, para que la vista se recargue. */
  onFinished?: () => void;
}

interface Estado {
  origins: ImportOrigin[];
  jobs: ImportJob[];
}

const VACIO: Estado = { origins: [], jobs: [] };

const CATEGORIA: Record<ImportDomain, DataCategory> = {
  mail: 'mail',
  drive: 'files',
  calendar: 'events',
  contacts: 'contacts',
};

/**
 * Si la persona lo dejó plegado. Una preferencia de este navegador y para todas las apps:
 * quien ya ha decidido no traer nada no quiere el bloque abierto en cada producto. Sin
 * almacenamiento (ventana privada, bloqueado) arranca abierto, como siempre.
 */
const CLAVE_PLEGADO = 'sytue.import.plegado';

function leerPlegado(): boolean {
  try {
    return localStorage.getItem(CLAVE_PLEGADO) === '1';
  } catch {
    return false;
  }
}

function guardarPlegado(plegado: boolean): void {
  try {
    if (plegado) localStorage.setItem(CLAVE_PLEGADO, '1');
    else localStorage.removeItem(CLAVE_PLEGADO);
  } catch {
    // Sin almacenamiento solo se pierde el recuerdo; el pliegue funciona igual.
  }
}

/** El gateway rechazó el lanzamiento con este código (vacío si no dio ninguno). */
class FalloAlLanzar extends Error {
  constructor(readonly code: string) {
    super(code || 'import_failed');
  }
}

export function ImportData({
  domains,
  endpoint = '/api/import',
  onFinished,
}: ImportDataProps): JSX.Element | null {
  const { t, locale } = useI18n();
  const [estado, setEstado] = useState<Estado>(VACIO);
  const [cargando, setCargando] = useState(true);
  const [enviando, setEnviando] = useState(false);
  // El último lanzamiento rechazado, con el código del gateway; `null` si no hubo fallo.
  const [fallo, setFallo] = useState<{ code: string; source: ImportSource } | null>(null);
  // En el lateral el bloque entero (ayuda, DataFlow, botones) se come media barra, así
  // que se puede plegar a su línea de título.
  const [plegado, setPlegado] = useState(leerPlegado);
  // Cada importación lanzada vuelve a arrancar el sondeo: sin esto el bloque se quedaba
  // en «trayendo…» hasta recargar la página, porque solo se sondeaba lo que ya estaba en
  // curso al montar. Igual en la app nativa (`ui.ImportarDatos`).
  const [vuelta, setVuelta] = useState(0);
  // Si en la última lectura había trabajo. En una referencia y no leyendo `estado`
  // dentro del efecto, que ve siempre el del primer render y nunca avisaba al acabar.
  const trabajaba = useRef(false);

  // Sondeo mientras haya trabajo. Una migración de un buzón entero tarda minutos, así
  // que la alternativa —pedir a la persona que recargue— es no contarle nada.
  useEffect(() => {
    let vivo = true;
    let timer: ReturnType<typeof setTimeout> | undefined;

    const cargar = async (): Promise<void> => {
      try {
        const res = await fetch(`${endpoint}?domains=${domains.join(',')}`, {
          credentials: 'include',
          headers: { accept: 'application/json' },
        });
        if (!res.ok) throw new Error(String(res.status));
        const data = (await res.json()) as Estado;
        if (!vivo) return;
        const jobs = data.jobs ?? [];
        const antes = trabajaba.current;
        trabajaba.current = jobs.some(enCurso);
        setEstado({ origins: data.origins ?? [], jobs });
        if (antes && !trabajaba.current) onFinished?.();
        if (trabajaba.current) timer = setTimeout(() => void cargar(), 5000);
      } catch {
        // Degrada a «no hay orígenes»: sin conexión con el gateway no hay nada que
        // ofrecer, y un error aquí solo alarmaría sobre algo que no se pidió.
        if (vivo) setEstado(VACIO);
      } finally {
        if (vivo) setCargando(false);
      }
    };
    void cargar();
    return () => {
      vivo = false;
      if (timer) clearTimeout(timer);
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [endpoint, domains.join(','), vuelta]);

  /**
   * Lanza la importación de `quiero` desde un origen. El servidor es idempotente: si esa
   * cuenta y contenido ya tenían un trabajo, devuelve ese —en marcha, o reanudado donde
   * se quedó— y aquí SUSTITUYE al que había en vez de sumarse a él.
   */
  async function traer(origin: Pick<ImportOrigin, 'instanceId' | 'source'>, quiero = domains) {
    if (enviando) return;
    setEnviando(true);
    setFallo(null);
    try {
      const res = await fetch(endpoint, {
        method: 'POST',
        credentials: 'include',
        headers: { 'content-type': 'application/json', accept: 'application/json' },
        body: JSON.stringify({
          instanceId: origin.instanceId,
          source: origin.source,
          domains: quiero,
        }),
      });
      if (!res.ok) {
        const cuerpo = (await res.json().catch(() => null)) as { code?: unknown } | null;
        throw new FalloAlLanzar(typeof cuerpo?.code === 'string' ? cuerpo.code : '');
      }
      const data = (await res.json()) as { jobs?: ImportJob[] };
      const nuevos = data.jobs ?? [];
      const ids = new Set(nuevos.map((j) => j.id));
      setEstado((e) => ({
        ...e,
        jobs: latestImports([...nuevos, ...e.jobs.filter((j) => !ids.has(j.id))]),
      }));
      trabajaba.current = true;
      setVuelta((v) => v + 1);
    } catch (e) {
      setFallo({ code: e instanceof FalloAlLanzar ? e.code : '', source: origin.source });
    } finally {
      setEnviando(false);
    }
  }

  /**
   * Reintenta lo que falló: un lanzamiento por cuenta con sus contenidos fallidos. Cada
   * trabajo sigue donde se quedó y vuelve a intentar SOLO lo que no entró.
   */
  async function reintentar(fallidos: ImportJob[]): Promise<void> {
    type Lanzamiento = {
      origin: Pick<ImportOrigin, 'instanceId' | 'source'>;
      doms: ImportDomain[];
    };
    const porCuenta = new Map<string, Lanzamiento>();
    for (const j of fallidos) {
      const o = estado.origins.find((x) => esDeOrigen(j, x)) ?? {
        instanceId: j.instanceId,
        source: j.source,
      };
      const clave = `${o.source}|${o.instanceId}`;
      const grupo = porCuenta.get(clave) ?? { origin: o, doms: [] };
      if (!grupo.doms.includes(j.domain)) grupo.doms.push(j.domain);
      porCuenta.set(clave, grupo);
    }
    for (const { origin, doms } of porCuenta.values()) await traer(origin, doms);
  }

  // Sin orígenes conectados no hay nada que ofrecer. Tampoco mientras se averigua: un
  // bloque que aparece y desaparece al cargar da la sensación de que algo se rompió.
  if (cargando || (estado.origins.length === 0 && estado.jobs.length === 0)) return null;

  const { trabajando, hechos, fallidos, libres, variasCuentas } = repartir(
    estado.origins,
    estado.jobs,
    domains,
  );
  const repetidos = proveedoresRepetidos(estado.origins);
  /** «Traerlo de Google», y «· acme.com» detrás si hay más de una cuenta de Google. */
  const rotulo = (o: ImportOrigin): string => {
    const base = t(o.source === 'google' ? 'import.fromGoogle' : 'import.fromMicrosoft');
    return repetidos.has(o.source) ? `${base} · ${cuentaDeOrigen(o)}` : base;
  };
  const marca = (o: ImportOrigin): string =>
    repetidos.has(o.source) ? `${MARCA[o.source]} · ${cuentaDeOrigen(o)}` : MARCA[o.source];
  /** «Correo», y «· acme.com» detrás si el resumen mezcla cuentas. */
  const que = (j: ImportJob): string => {
    const tipo = t(`dataflow.item.${CATEGORIA[j.domain]}`);
    return variasCuentas && j.accountLabel ? `${tipo} · ${j.accountLabel}` : tipo;
  };
  // Las cuentas que no pueden leer se dicen una vez, apagadas y con el motivo.
  const apagados = libres
    .filter((o) => o.state !== 'ready')
    .map((o) => t('import.unavailable', { source: marca(o) }));
  // Un aviso por motivo distinto, no uno por trabajo: el correo y los contactos de la
  // misma cuenta caducada son UN problema, y si ya lo dice el origen apagado, sobra.
  const motivos = [
    ...new Set(fallidos.map((j) => t(claveDeFallo(j.errorCode), { source: MARCA[j.source] }))),
  ].filter((m) => !apagados.includes(m));

  function alternar(): void {
    const siguiente = !plegado;
    setPlegado(siguiente);
    guardarPlegado(siguiente);
  }

  return (
    <section class="border-border bg-surface rounded-md border p-3" aria-label={t('import.title')}>
      <div class="flex items-center justify-between gap-2">
        <h2 class="text-text flex min-w-0 items-center gap-2 text-sm font-semibold">
          <span class="truncate">{t('import.title')}</span>
          {/* Plegado y trabajando: la rueda sigue diciendo que algo pasa. */}
          {plegado && trabajando.length > 0 ? <Spinner size={14} /> : null}
        </h2>
        {/* `data-mantiene-nav`: en el teléfono el lateral es un cajón que se cierra al
            pulsar cualquier botón de dentro; plegar no es elegir nada (ver SidebarSection). */}
        <Button
          size="xs"
          variant="ghost"
          iconOnly
          data-mantiene-nav
          aria-expanded={!plegado}
          aria-label={`${t(plegado ? 'import.expand' : 'import.collapse')}: ${t('import.title')}`}
          title={t(plegado ? 'import.expand' : 'import.collapse')}
          onClick={alternar}
        >
          <Icon name={plegado ? 'chevronRight' : 'chevronDown'} size={14} />
        </Button>
      </div>

      {plegado ? null : (
        <>
          {trabajando.length > 0 ? (
            <div class="mt-2 flex items-center gap-2" aria-live="polite">
              <Spinner size={16} />
              <p class="text-text-muted text-xs">
                {t('import.running', { count: contarTraidos(trabajando) })}
              </p>
            </div>
          ) : (
            <>
              <p class="text-text-muted mt-0.5 text-xs">{t('import.help')}</p>
              <DataFlow compact class="mt-2" incoming={domains.map((d) => CATEGORIA[d])} />
            </>
          )}
          {/* Las cuentas con una importación en marcha no se ofrecen otra vez: el
              servidor devolvería la misma, y un botón que no hace nada confunde. */}
          {libres.length > 0 ? (
            <div class="mt-2 flex flex-wrap gap-2">
              {libres.map((o) => (
                <Button
                  key={o.instanceId}
                  size="sm"
                  variant="secondary"
                  disabled={enviando || o.state !== 'ready'}
                  onClick={() => void traer(o)}
                >
                  {rotulo(o)}
                </Button>
              ))}
            </div>
          ) : null}
          {apagados.map((m) => (
            <p key={m} class="text-text-muted mt-2 text-xs">
              {m}
            </p>
          ))}

          {/* El recuento va SIEMPRE, también cuando termina: «traído» a secas no dice si
          llegó todo, y saber cuántos elementos se quedaron fuera es la mitad honesta
          del informe. Una línea por contenido, con lo acumulado y cuándo terminó. */}
          {hechos.map((j) => (
            <p key={j.id} class="sytue-meta mt-2">
              {t('import.done', {
                what: que(j),
                count: j.migrated,
                skipped: j.skipped,
                date: formatDate(locale, j.finishedAt ?? j.createdAt),
              })}
            </p>
          ))}
          {fallidos.length > 0 ? (
            <div class="mt-2 flex flex-wrap items-center gap-2" role="alert">
              {motivos.map((m) => (
                <p key={m} class="text-danger-text text-xs">
                  {m}
                </p>
              ))}
              {/* Reintentar sigue donde se quedó: solo vuelve a intentar lo que no entró. */}
              <Button
                size="xs"
                variant="ghost"
                disabled={enviando || trabajando.length > 0}
                onClick={() => void reintentar(fallidos)}
              >
                {t('action.retry')}
              </Button>
            </div>
          ) : null}
          {fallo !== null ? (
            <p class="text-danger-text mt-2 text-xs" role="alert">
              {claveDeFallo(fallo.code) === 'import.unavailable'
                ? t('import.unavailable', { source: MARCA[fallo.source] })
                : t('import.failed')}
            </p>
          ) : null}
        </>
      )}
    </section>
  );
}
