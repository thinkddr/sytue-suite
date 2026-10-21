/**
 * VersionsPanel — qué versión corre cada pieza de la suite.
 *
 * Antes eran dos filas dentro de la pestaña de uso: esta superficie y Sytue ID.
 * Con eso se responde «¿el frontend y el IdP van a la par?», que es media
 * pregunta: la plataforma son catorce servicios Go y once superficies web, y un
 * despliegue se queda a medias por cualquiera de ellos. Dos filas que coinciden
 * no dicen nada de las otras veintitrés.
 *
 * LO PRIMERO QUE SE LEE NO ES LA TABLA, ES EL VEREDICTO. Veinticinco versiones
 * en columna obligan a compararlas a ojo, que es justo lo que a las tres de la
 * mañana sale mal. Arriba se dice cuántas van a la par, cuántas se han quedado
 * atrás y cuántas no contestan; la tabla está debajo para saber CUÁLES.
 *
 * Se refresca solo porque esta pantalla se mira MIENTRAS se despliega: tener que
 * recargar a mano para ver si el rollout avanzó es exactamente lo que aquí no
 * puede pasar.
 */
import { useCallback, useEffect, useRef, useState } from 'preact/hooks';
import {
  Badge,
  Button,
  Card,
  CardBody,
  CardHeader,
  CardTitle,
  EmptyState,
  Table,
  useSondeo,
} from '@sytue/ui';
import type { T } from '../i18n/index.js';
import type { ComponentVersion, PlatformVersions } from '../lib/platform.js';

/** Cada cuánto se vuelve a preguntar. Un rollout tarda minutos, no segundos:
 *  quince es suficiente para verlo avanzar sin machacar a veinticinco pods. */
const REFRESCO_MS = 15_000;

/** Una sola forma de pedir los datos: la usan el refresco automático y el botón.
 *  Dos copias del mismo `fetch` acaban divergiendo justo cuando cambia la ruta. */
async function traerVersiones(): Promise<PlatformVersions | null> {
  const res = await fetch('/api/platform/versions');
  // 403 con la pantalla abierta = la elevación ha caducado. Se recarga para
  // volver al gate en vez de sondear en balde cada quince segundos (ver
  // LogsPanel, que lo explica entero).
  if (res.status === 403) window.location.reload();
  if (!res.ok) return null;
  return (await res.json()) as PlatformVersions;
}

/** Estado de una pieza respecto a lo que corre la mayoría. */
type Estado = 'ok' | 'desviada' | 'muda';

function estadoDe(c: ComponentVersion, esperada: string): Estado {
  if (!c.ok) return 'muda';
  // Sin mayoría clara no se señala a nadie: ver `versionMayoritaria` en el
  // backend. Marcar desviados sin patrón sería inventarse cuál es el bando bueno.
  if (!esperada || !c.release) return 'ok';
  return c.release === esperada ? 'ok' : 'desviada';
}

/** Cifra de cabecera; solo se enciende de color si hay algo que mirar. */
function Kpi({
  label,
  value,
  tone,
}: {
  label: string;
  value: number | string;
  tone?: string | undefined;
}) {
  return (
    <Card raised={false}>
      <CardBody>
        <p class="text-text-muted truncate text-sm">{label}</p>
        {/* El que se recortaba era el rótulo, y el que se sale es el VALOR: la
            primera tarjeta enseña la release esperada («2026.08.09-a1b2c3d»), no
            una cifra, y a `sytue-h3` no cabe en una columna de cuatro. Con `title`
            se sigue pudiendo leer entera. */}
        <p
          class={`sytue-h3 mt-1 truncate tabular-nums ${tone ?? 'text-text'}`}
          title={String(value)}
        >
          {value}
        </p>
      </CardBody>
    </Card>
  );
}

export default function VersionsPanel({ t }: { t: T }) {
  const [datos, setDatos] = useState<PlatformVersions | null>(null);
  const [fallo, setFallo] = useState(false);
  // SOLO la recarga a mano. El refresco automático no toca este estado: si lo
  // tocara, el botón se desactivaría solo cada quince segundos y, con el foco de
  // teclado encima, el navegador lo suelta —un control que se apaga sin que nadie
  // lo haya pulsado—.
  const [cargando, setCargando] = useState(false);

  const vivoRef = useRef(true);
  const traer = useCallback(async () => {
    try {
      const d = await traerVersiones();
      if (!vivoRef.current) return;
      if (!d) {
        setFallo(true);
        return;
      }
      setFallo(false);
      setDatos(d);
    } catch {
      if (vivoRef.current) setFallo(true);
    }
  }, []);

  useEffect(() => {
    vivoRef.current = true;
    void traer();
    return () => {
      vivoRef.current = false;
    };
  }, [traer]);

  // Las versiones desplegadas cambian con un despliegue, no cada quince segundos:
  // con la pestaña de fondo eran 240 consultas a la hora para enseñar el mismo
  // número. Al volver a primer plano se consulta de inmediato.
  useSondeo(() => void traer(), REFRESCO_MS);

  if (fallo) {
    return <EmptyState title={t('platform.versions.unavailable')} />;
  }
  if (!datos) {
    return <p class="sy-hint p-4">{t('common.loading')}</p>;
  }

  const esperada = datos.expected;
  const items = datos.items ?? [];
  const desviadas = items.filter((c) => estadoDe(c, esperada) === 'desviada');
  const mudas = items.filter((c) => estadoDe(c, esperada) === 'muda');
  const alDia = items.length - desviadas.length - mudas.length;

  const columnas = [
    {
      key: 'name',
      header: t('platform.versions.col.component'),
      cell: (c: ComponentVersion) => (
        <span class="flex items-center gap-2">
          <span class="text-text font-medium">{c.name}</span>
          <span class="sytue-meta">
            {c.kind === 'surface'
              ? t('platform.versions.kind.surface')
              : t('platform.versions.kind.service')}
          </span>
        </span>
      ),
    },
    {
      key: 'release',
      header: t('platform.versions.col.release'),
      cell: (c: ComponentVersion) =>
        !c.ok ? (
          <span class="text-text-muted">—</span>
        ) : (
          // Sin release: la imagen se construyó a mano, y decirlo es más útil
          // que enseñar un hueco que parece un fallo de carga.
          <span class="tabular-nums">{c.release || t('platform.version.unbuilt')}</span>
        ),
    },
    {
      key: 'commit',
      header: t('platform.versions.col.commit'),
      // Corto para reconocerlo de un vistazo; entero en el title, que es lo que
      // hace falta para buscarlo en la forja.
      cell: (c: ComponentVersion) =>
        c.commit ? (
          <code class="font-mono text-xs" title={c.commit}>
            {c.commit.slice(0, 7)}
          </code>
        ) : (
          <span class="text-text-muted">—</span>
        ),
    },
    {
      key: 'estado',
      header: t('platform.versions.col.status'),
      align: 'end' as const,
      cell: (c: ComponentVersion) => {
        const e = estadoDe(c, esperada);
        if (e === 'muda') {
          return <Badge tone="danger">{c.error || t('platform.version.unreachable')}</Badge>;
        }
        if (e === 'desviada') {
          return <Badge tone="warning">{t('platform.versions.behind')}</Badge>;
        }
        return <Badge tone="success">{t('platform.versions.upToDate')}</Badge>;
      },
    },
  ];

  return (
    <div class="flex flex-col gap-4">
      <div class="grid grid-cols-2 gap-4 lg:grid-cols-4">
        <Kpi label={t('platform.versions.kpi.expected')} value={esperada || '—'} />
        <Kpi label={t('platform.versions.kpi.upToDate')} value={`${alDia}/${items.length}`} />
        <Kpi
          label={t('platform.versions.kpi.behind')}
          value={desviadas.length}
          tone={desviadas.length > 0 ? 'text-warning-text' : undefined}
        />
        <Kpi
          label={t('platform.versions.kpi.silent')}
          value={mudas.length}
          tone={mudas.length > 0 ? 'text-danger-text' : undefined}
        />
      </div>

      <Card>
        <CardHeader>
          <CardTitle>{t('platform.versions.title')}</CardTitle>
          <Button
            size="sm"
            variant="ghost"
            disabled={cargando}
            onClick={() => {
              // Recarga inmediata: al terminar un despliegue no se espera quince
              // segundos para saber si cuadró.
              setCargando(true);
              void traerVersiones()
                .then((d) => {
                  if (d) setDatos(d);
                })
                .finally(() => setCargando(false));
            }}
          >
            {t('platform.versions.refresh')}
          </Button>
        </CardHeader>
        <CardBody class="p-0">
          {items.length === 0 ? (
            // Catálogo vacío ⇒ el chart no pasó `global.components`. Se dice, en
            // vez de enseñar una tabla vacía que parece que todo va bien.
            <EmptyState
              title={t('platform.versions.empty')}
              description={t('platform.versions.emptyHint')}
            />
          ) : (
            <Table
              columns={columnas}
              rows={items}
              rowKey={(c) => `${c.kind}:${c.name}`}
              caption={t('platform.versions.title')}
            />
          )}
        </CardBody>
      </Card>
    </div>
  );
}
