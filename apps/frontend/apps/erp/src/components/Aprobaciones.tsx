/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Aprobaciones — los documentos que pasan del umbral de su módulo y esperan una
 * segunda persona, y los umbrales mismos para quien los administra.
 *
 * Los cuatro ojos los impone el servicio (403 `cuatro_ojos`); aquí solo se evita
 * ofrecer «Aprobar» a quien lo pidió o creó el documento, para que el botón no
 * acabe siempre en el mismo error.
 */
import type { JSX } from 'preact';
import { useCallback, useEffect, useState } from 'preact/hooks';
import {
  Badge,
  Button,
  EmptyState,
  Icon,
  Input,
  Spinner,
  cx,
  usePrompt,
  useToast,
} from '@sytue/ui';
import type { Locale } from '@sytue/i18n';
import { api, consulta, mensajeDeError } from '../lib/api.js';
import type { useT } from '../i18n/index.js';
import { aCentimos, deCentimos, formatearDinero, formatearFecha } from './formato.js';
import type { Aprobacion, EstadoAprobacion, Umbral } from './tipos.js';

type Traducir = ReturnType<typeof useT>;

/** Los módulos tal como los rotula el servicio para la administración. */
interface ModuloRotulado {
  clave: string;
  rotulo: string;
}

/** Lo que devuelve `GET admin/umbrales`. */
interface RespuestaUmbrales {
  umbrales: Umbral[] | null;
  moneda_por_defecto: string;
  modulos: ModuloRotulado[];
}

/** Quien lo pidió o lo creó no lo puede aprobar (cuatro ojos). */
export function puedeResolver(a: Aprobacion, yo: string): boolean {
  return a.estado === 'pendiente' && a.solicitada_por !== yo && a.creada_por !== yo;
}

export function Aprobaciones({
  locale,
  t,
  yo,
  puedeAprobar,
  puedeAdministrar,
  rotuloModelo,
  onAbrir,
  onCambio,
}: {
  locale: Locale;
  t: Traducir;
  /** El `sub` de quien mira: para no ofrecerle aprobar lo suyo. */
  yo: string;
  /** Tiene el verbo `aprobar` en contabilidad (lo que exige la ruta). */
  puedeAprobar: boolean;
  /** Tiene el verbo `administrar`: ve y cambia los umbrales. */
  puedeAdministrar: boolean;
  rotuloModelo: (clave: string) => string;
  onAbrir: (modelo: string, id: string) => void;
  /** Algo cambió (el contador de la barra lateral). */
  onCambio: () => void;
}): JSX.Element {
  const aviso = useToast();
  const { preguntar, dialogo } = usePrompt();
  const [estado, setEstado] = useState<EstadoAprobacion>('pendiente');
  const [lista, setLista] = useState<Aprobacion[] | null>(null);
  const [ocupada, setOcupada] = useState('');

  // Cambiar de filtro deprisa deja respuestas en vuelo: solo pinta la del filtro que
  // sigue puesto, o «Pendientes» podría acabar con la lista de «Rechazadas».
  const cargar = useCallback(
    (vigente: () => boolean = () => true) => {
      setLista(null);
      api<{ aprobaciones: Aprobacion[] | null }>(`aprobaciones${consulta({ estado })}`)
        .then((r) => vigente() && setLista(r.aprobaciones ?? []))
        .catch(() => vigente() && setLista([]));
    },
    [estado],
  );
  useEffect(() => {
    let vigente = true;
    cargar(() => vigente);
    return () => {
      vigente = false;
    };
  }, [cargar]);

  async function resolver(a: Aprobacion, como: 'aprobar' | 'rechazar'): Promise<void> {
    let motivo = '';
    if (como === 'rechazar') {
      // El servicio exige el motivo: sin él, quien lo pidió no sabe qué arreglar.
      const m = await preguntar({
        titulo: t('approvals.rejectTitle'),
        mensaje: t('approvals.rejectReason'),
        confirmLabel: t('approvals.reject'),
      });
      if (!m) return;
      motivo = m;
    }
    setOcupada(a.id);
    try {
      await api(`aprobaciones/${a.id}/${como}`, {
        method: 'POST',
        body: JSON.stringify({ motivo }),
      });
      aviso.notify({
        tone: 'success',
        title: t(como === 'aprobar' ? 'approvals.approvedToast' : 'approvals.rejectedToast'),
      });
      onCambio();
    } catch (err) {
      aviso.notify({ tone: 'danger', title: mensajeDeError(err, t) });
    } finally {
      setOcupada('');
      cargar();
    }
  }

  const estados: { valor: EstadoAprobacion; rotulo: string }[] = [
    { valor: 'pendiente', rotulo: t('approvals.pending') },
    { valor: 'aprobada', rotulo: t('approvals.approved') },
    { valor: 'rechazada', rotulo: t('approvals.rejected') },
  ];

  return (
    <div class="mx-auto max-w-5xl space-y-6 px-6 py-6">
      {dialogo}
      <header class="border-border flex flex-col gap-3 border-b pb-4 sm:flex-row sm:items-start sm:justify-between">
        <div class="min-w-0">
          <h1 class="text-text text-lg font-semibold tracking-tight">{t('approvals.title')}</h1>
          <p class="text-text-muted mt-1 max-w-prose text-sm">{t('approvals.subtitle')}</p>
        </div>
        <div
          class="border-border inline-flex shrink-0 items-center overflow-hidden rounded-md border"
          role="group"
          aria-label={t('approvals.title')}
        >
          {estados.map(({ valor, rotulo }) => (
            <button
              key={valor}
              type="button"
              aria-pressed={estado === valor}
              onClick={() => setEstado(valor)}
              class={cx(
                'sytue-focus-ring px-3 py-1 text-sm pointer-coarse:min-h-[max(44px,2.75rem)]',
                estado === valor
                  ? 'bg-selected text-text font-semibold'
                  : 'text-text-muted hover:text-text',
              )}
            >
              {rotulo}
            </button>
          ))}
        </div>
      </header>

      {lista === null ? (
        <div class="flex justify-center py-12">
          <Spinner size={24} />
        </div>
      ) : lista.length === 0 ? (
        <EmptyState
          title={t(estado === 'pendiente' ? 'approvals.empty' : 'approvals.emptyResolved')}
          {...(estado === 'pendiente' ? { description: t('approvals.emptyHint') } : {})}
          spot="emptyCalendar"
        />
      ) : (
        <ul class="border-border divide-border rounded-card divide-y border">
          {lista.map((a) => (
            <li
              key={a.id}
              class="flex flex-col gap-2 px-4 py-3 sm:flex-row sm:items-center sm:justify-between"
            >
              <div class="min-w-0">
                <p class="text-text text-sm font-medium">
                  <button
                    type="button"
                    onClick={() => onAbrir(a.modelo, a.registro_id)}
                    class="sytue-focus-ring hover:text-text rounded-sm underline"
                  >
                    {rotuloModelo(a.modelo)}
                  </button>
                  {' · '}
                  <span class="font-mono tabular-nums">
                    {formatearDinero(a.importe, locale, a.moneda)}
                  </span>
                </p>
                <p class="text-text-subtle text-xs">
                  {t('approvals.requestedOn', {
                    date: formatearFecha(a.solicitada_en, locale, true),
                  })}
                  {a.solicitada_por === yo ? ` · ${t('approvals.byYou')}` : ''}
                  {a.motivo ? ` · ${t('approvals.reason', { reason: a.motivo })}` : ''}
                </p>
              </div>
              <div class="flex shrink-0 flex-wrap items-center gap-1.5">
                {a.estado !== 'pendiente' ? (
                  <Badge tone={a.estado === 'aprobada' ? 'success' : 'danger'}>
                    {t(a.estado === 'aprobada' ? 'approvals.approved' : 'approvals.rejected')}
                  </Badge>
                ) : puedeAprobar && puedeResolver(a, yo) ? (
                  <>
                    <Button
                      size="xs"
                      variant="primary"
                      loading={ocupada === a.id}
                      onClick={() => void resolver(a, 'aprobar')}
                    >
                      <Icon name="check" size={12} class="mr-1" />
                      {t('approvals.approve')}
                    </Button>
                    <Button
                      size="xs"
                      variant="ghost"
                      disabled={ocupada === a.id}
                      onClick={() => void resolver(a, 'rechazar')}
                    >
                      {t('approvals.reject')}
                    </Button>
                  </>
                ) : (
                  <Badge tone="warning">{t('approvals.waiting')}</Badge>
                )}
              </div>
            </li>
          ))}
        </ul>
      )}

      {puedeAdministrar ? <Umbrales locale={locale} t={t} /> : null}
    </div>
  );
}

/** Los umbrales por módulo: en blanco, sin umbral. */
function Umbrales({ locale, t }: { locale: Locale; t: Traducir }): JSX.Element {
  const aviso = useToast();
  const [datos, setDatos] = useState<RespuestaUmbrales | null>(null);
  const [escrito, setEscrito] = useState<Record<string, string>>({});
  const [guardando, setGuardando] = useState('');

  const cargar = useCallback(() => {
    api<RespuestaUmbrales>('admin/umbrales')
      .then((r) => {
        setDatos(r);
        const inicial: Record<string, string> = {};
        for (const u of r.umbrales ?? []) inicial[u.modulo] = deCentimos(u.importe);
        setEscrito(inicial);
      })
      .catch(() => setDatos({ umbrales: [], moneda_por_defecto: 'EUR', modulos: [] }));
  }, []);
  useEffect(cargar, [cargar]);

  async function guardar(modulo: string, moneda: string): Promise<void> {
    const texto = (escrito[modulo] ?? '').trim();
    // En blanco = quitarlo: el servicio lo entiende como un importe negativo.
    const importe = texto === '' ? -1 : aCentimos(texto);
    if (importe === null || (texto !== '' && importe <= 0)) {
      aviso.notify({ tone: 'danger', title: t('approvals.thresholdInvalid') });
      return;
    }
    setGuardando(modulo);
    try {
      await api(`admin/umbrales/${encodeURIComponent(modulo)}`, {
        method: 'PUT',
        body: JSON.stringify({ importe, moneda }),
      });
      aviso.notify({ tone: 'success', title: t('approvals.thresholdSaved') });
      cargar();
    } catch (err) {
      aviso.notify({ tone: 'danger', title: mensajeDeError(err, t) });
    } finally {
      setGuardando('');
    }
  }

  // «Obligaciones» no es un módulo del catálogo: el servicio no le acepta umbral.
  const modulos = (datos?.modulos ?? []).filter((m) => m.clave !== 'obligaciones');
  return (
    <section aria-labelledby="umbrales-titulo" class="space-y-2">
      <h2 id="umbrales-titulo" class="sytue-overline text-text-subtle">
        {t('approvals.thresholds')}
      </h2>
      <p class="text-text-muted max-w-prose text-sm">{t('approvals.thresholdsHint')}</p>
      {datos === null ? (
        <div class="flex justify-center py-6">
          <Spinner size={20} />
        </div>
      ) : (
        <ul class="border-border divide-border rounded-card divide-y border">
          {modulos.map((m) => {
            const puesto = (datos.umbrales ?? []).find((u) => u.modulo === m.clave);
            const moneda = puesto?.moneda ?? datos.moneda_por_defecto;
            const id = `umbral-${m.clave}`;
            return (
              <li
                key={m.clave}
                class="flex flex-col gap-2 px-4 py-2 sm:flex-row sm:items-center sm:justify-between"
              >
                <label for={id} class="text-text text-sm">
                  {m.rotulo}
                  <span class="text-text-subtle block text-xs">
                    {puesto
                      ? t('approvals.thresholdFrom', {
                          amount: formatearDinero(puesto.importe, locale, puesto.moneda),
                        })
                      : t('approvals.thresholdNone')}
                  </span>
                </label>
                <div class="flex shrink-0 items-center gap-1.5">
                  <Input
                    id={id}
                    size="sm"
                    inputMode="decimal"
                    class="w-32 text-right tabular-nums"
                    value={escrito[m.clave] ?? ''}
                    placeholder={t('approvals.thresholdNone')}
                    onInput={(e) => {
                      const v = (e.target as HTMLInputElement).value;
                      setEscrito((x) => ({ ...x, [m.clave]: v }));
                    }}
                  />
                  <span class="text-text-subtle text-xs">{moneda}</span>
                  <Button
                    size="xs"
                    variant="secondary"
                    loading={guardando === m.clave}
                    onClick={() => void guardar(m.clave, moneda)}
                  >
                    {t('action.save')}
                  </Button>
                </div>
              </li>
            );
          })}
        </ul>
      )}
    </section>
  );
}
