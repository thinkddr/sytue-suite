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
 */
import type { JSX } from 'preact';
import { useEffect, useRef, useState } from 'preact/hooks';
import { Button, DataFlow, Icon, Spinner } from '@sytue/ui';
import type { DataCategory } from '@sytue/ui';
import { useI18n } from '@sytue/i18n/preact';
import type { ImportDomain, ImportJob, ImportOrigin } from '@sytue/api-clients';

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

/** ¿Sigue trabajando? Mientras lo haga se vuelve a preguntar. */
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

function enCurso(jobs: ImportJob[]): boolean {
  return jobs.some((j) => j.status === 'queued' || j.status === 'running');
}

export function ImportData({
  domains,
  endpoint = '/api/import',
  onFinished,
}: ImportDataProps): JSX.Element | null {
  const { t } = useI18n();
  const [estado, setEstado] = useState<Estado>(VACIO);
  const [cargando, setCargando] = useState(true);
  const [enviando, setEnviando] = useState(false);
  const [error, setError] = useState('');
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
        trabajaba.current = enCurso(jobs);
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

  async function traer(origin: ImportOrigin): Promise<void> {
    if (enviando) return;
    setEnviando(true);
    setError('');
    try {
      const res = await fetch(endpoint, {
        method: 'POST',
        credentials: 'include',
        headers: { 'content-type': 'application/json', accept: 'application/json' },
        body: JSON.stringify({ instanceId: origin.instanceId, source: origin.source, domains }),
      });
      if (!res.ok) throw new Error(String(res.status));
      const data = (await res.json()) as { jobs?: ImportJob[] };
      setEstado((e) => ({ ...e, jobs: [...(data.jobs ?? []), ...e.jobs] }));
      trabajaba.current = true;
      setVuelta((v) => v + 1);
    } catch {
      setError(t('import.failed'));
    } finally {
      setEnviando(false);
    }
  }

  // Sin orígenes conectados no hay nada que ofrecer. Tampoco mientras se averigua: un
  // bloque que aparece y desaparece al cargar da la sensación de que algo se rompió.
  if (cargando || (estado.origins.length === 0 && estado.jobs.length === 0)) return null;

  const trabajando = estado.jobs.filter((j) => j.status === 'queued' || j.status === 'running');
  const hechos = estado.jobs.filter((j) => j.status === 'done');
  const fallidos = estado.jobs.filter((j) => j.status === 'error');

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
                {t('import.running', { count: contar(trabajando) })}
              </p>
            </div>
          ) : (
            <>
              <p class="text-text-muted mt-0.5 text-xs">{t('import.help')}</p>
              <DataFlow compact class="mt-2" incoming={domains.map((d) => CATEGORIA[d])} />
              <div class="mt-2 flex flex-wrap gap-2">
                {estado.origins.map((o) => (
                  <Button
                    key={o.instanceId}
                    size="sm"
                    variant="secondary"
                    disabled={enviando}
                    onClick={() => void traer(o)}
                  >
                    {t(o.source === 'google' ? 'import.fromGoogle' : 'import.fromMicrosoft')}
                  </Button>
                ))}
              </div>
            </>
          )}

          {/* El recuento va SIEMPRE, también cuando termina: «traído» a secas no dice si
          llegó todo, y saber cuántos elementos se quedaron fuera es la mitad honesta
          del informe. */}
          {hechos.length > 0 ? (
            <p class="sytue-meta mt-2">
              {t('import.done', { count: contar(hechos), skipped: contarOmitidos(hechos) })}
            </p>
          ) : null}
          {fallidos.length > 0 ? (
            <p class="text-danger-text mt-2 text-xs" role="alert">
              {t('import.someFailed')}
            </p>
          ) : null}
          {error ? (
            <p class="text-danger-text mt-2 text-xs" role="alert">
              {error}
            </p>
          ) : null}
        </>
      )}
    </section>
  );
}

/** Total de elementos traídos por una tanda de trabajos. */
function contar(jobs: ImportJob[]): number {
  return jobs.reduce((n, j) => n + j.migrated, 0);
}

/** Total de elementos que NO se pudieron traer. */
function contarOmitidos(jobs: ImportJob[]): number {
  return jobs.reduce((n, j) => n + j.skipped, 0);
}
