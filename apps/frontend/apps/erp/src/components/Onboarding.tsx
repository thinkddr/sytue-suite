/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Onboarding — el asistente de arranque de ERP: país, forma jurídica, datos del
 * emisor y situación (IVA, empleados, intracomunitario, alquiler). De esas cinco
 * respuestas sale el calendario de obligaciones de la organización y la lista
 * de áreas que conviene encender.
 *
 * Cinco pasos cortos y no un formulario: quien monta un negocio no sabe qué es
 * «régimen de devolución mensual», pero sí sabe si tiene empleados. Cada paso
 * hace UNA pregunta en llano y el asistente traduce.
 *
 * La lista de países y formas la sirve el servicio (`organizacion/paises`), que
 * es quien tiene los ficheros por país; el nombre de cada país lo pone el
 * navegador con `Intl.DisplayNames` en el idioma de quien mira, sin una tabla
 * de veintisiete nombres por idioma en el frontend.
 */
import type { JSX } from 'preact';
import { useEffect, useMemo, useState } from 'preact/hooks';
import {
  Badge,
  Button,
  ChoiceCard,
  FieldGroup,
  Icon,
  Input,
  ReviewSummary,
  Select,
  Spinner,
  Switch,
  Wizard,
  useToast,
  withBase,
} from '@sytue/ui';
import type { WizardStep } from '@sytue/ui';
import type { Locale } from '@sytue/i18n';
import { api, baseDeLaApp, consulta, ErrorApi } from '../lib/api.js';
import type { useT } from '../i18n/index.js';
import type {
  FormaJuridica,
  Modulo,
  Organizacion,
  PaisInfo,
  RespuestaObligaciones,
  RespuestaPaises,
} from './tipos.js';

type Traducir = ReturnType<typeof useT>;

interface Props {
  locale: Locale;
  /** El perfil actual, si ya se hizo el asistente: se abre para editarlo. */
  organizacion: Organizacion | null;
  modulos: Modulo[];
  activos: string[];
  onGuardado: (o: Organizacion) => void;
  onVerCalendario: () => void;
  onActivarModulos: (claves: string[]) => Promise<void>;
  onCancelar: () => void;
  t: Traducir;
}

/** País por defecto según el idioma de la sesión: es lo más probable, no una certeza. */
const PAIS_POR_IDIOMA: Record<string, string> = {
  es: 'ES',
  fr: 'FR',
  de: 'DE',
  it: 'IT',
  en: 'IE',
};

/** Nombre del país en el idioma de quien mira, sin tabla propia. */
export function nombreDePais(codigo: string, locale: string): string {
  try {
    return new Intl.DisplayNames([locale], { type: 'region' }).of(codigo) ?? codigo;
  } catch {
    return codigo;
  }
}

/**
 * Las áreas que un negocio como este enciende el primer día. Es una sugerencia
 * que se puede deshacer con un clic; su valor es que la pantalla de inicio no
 * sea una cuadrícula de treinta y tres tarjetas apagadas.
 */
export function modulosRecomendados(tipo: FormaJuridica['tipo'], empleados: boolean): string[] {
  const base =
    tipo === 'sociedad'
      ? ['contactos', 'ventas', 'compras', 'facturacion', 'contabilidad', 'gastos']
      : ['contactos', 'facturacion', 'contabilidad', 'gastos'];
  return empleados ? [...base, 'rrhh', 'nominas'] : base;
}

const CONDICIONES = ['iva', 'iva_mensual', 'empleados', 'intracomunitario', 'alquiler'] as const;

const PASOS = ['pais', 'forma', 'datos', 'situacion', 'resumen', 'listo'] as const;

export function Onboarding({
  locale,
  organizacion,
  modulos,
  activos,
  onGuardado,
  onVerCalendario,
  onActivarModulos,
  onCancelar,
  t,
}: Props): JSX.Element {
  const aviso = useToast();
  const [paises, setPaises] = useState<PaisInfo[] | null>(null);
  const [paso, setPaso] = useState(0);
  const ir = (id: (typeof PASOS)[number]) => setPaso(PASOS.indexOf(id));

  const [pais, setPais] = useState(organizacion?.pais ?? PAIS_POR_IDIOMA[locale] ?? 'ES');
  const [tipo, setTipo] = useState<FormaJuridica['tipo'] | ''>('');
  const [forma, setForma] = useState(organizacion?.forma ?? '');
  const [nombre, setNombre] = useState(organizacion?.nombre ?? '');
  const [nif, setNif] = useState(organizacion?.nif ?? '');
  const [direccion, setDireccion] = useState(organizacion?.direccion ?? '');
  const [cp, setCp] = useState(organizacion?.cp ?? '');
  const [ciudad, setCiudad] = useState(organizacion?.ciudad ?? '');
  const [cond, setCond] = useState<Record<string, boolean>>({
    iva: true,
    ...organizacion?.condiciones,
  });
  const [tocado, setTocado] = useState(false);
  const [ocupado, setOcupado] = useState(false);
  const [total, setTotal] = useState<number | null>(null);
  const [activando, setActivando] = useState(false);
  const [activados, setActivados] = useState(false);

  useEffect(() => {
    let vivo = true;
    api<RespuestaPaises>('organizacion/paises')
      .then((r) => {
        if (!vivo) return;
        const lista = r.paises ?? [];
        setPaises(lista);
        // Al editar, el tipo sale de la forma ya guardada.
        const actual = lista.find((p) => p.pais === (organizacion?.pais ?? ''));
        const f = actual?.formas.find((x) => x.clave === organizacion?.forma);
        if (f) setTipo(f.tipo);
      })
      .catch(() => vivo && setPaises([]));
    return () => {
      vivo = false;
    };
    // Solo al montar: la lista de países no cambia durante la sesión.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const infoPais = useMemo(() => paises?.find((p) => p.pais === pais) ?? null, [paises, pais]);
  const formasDelTipo = useMemo(
    () => (infoPais?.formas ?? []).filter((f) => f.tipo === tipo),
    [infoPais, tipo],
  );
  const formaElegida = infoPais?.formas.find((f) => f.clave === forma) ?? null;
  const anio = new Date().getFullYear();

  // Cambiar de país o de tipo invalida la forma: una SL no existe en Alemania.
  function elegirPais(codigo: string): void {
    setPais(codigo);
    setForma('');
  }
  function elegirTipo(nuevo: FormaJuridica['tipo']): void {
    setTipo(nuevo);
    const candidatas = (infoPais?.formas ?? []).filter((f) => f.tipo === nuevo);
    setForma(candidatas.length === 1 ? (candidatas[0]?.clave ?? '') : '');
  }

  async function guardar(): Promise<void> {
    if (ocupado) return;
    setOcupado(true);
    try {
      const guardada = await api<Organizacion>('organizacion', {
        method: 'PUT',
        body: JSON.stringify({
          nombre: nombre.trim(),
          nif: nif.trim(),
          direccion: direccion.trim(),
          cp: cp.trim(),
          ciudad: ciudad.trim(),
          pais,
          forma,
          condiciones: cond,
        }),
      });
      onGuardado(guardada);
      const cal = await api<RespuestaObligaciones>(`obligaciones${consulta({ anio })}`).catch(
        () => null,
      );
      setTotal(cal?.obligaciones?.length ?? 0);
      ir('listo');
    } catch (err) {
      aviso.notify({
        tone: 'danger',
        title: err instanceof ErrorApi ? err.message : t('error.generic'),
      });
    } finally {
      setOcupado(false);
    }
  }

  const recomendados = useMemo(
    () =>
      tipo
        ? modulosRecomendados(tipo, cond.empleados === true).filter((c) =>
            modulos.some((m) => m.clave === c),
          )
        : [],
    [tipo, cond.empleados, modulos],
  );
  const pendientes = recomendados.filter((c) => !activos.includes(c));

  async function activarRecomendados(): Promise<void> {
    if (activando) return;
    setActivando(true);
    try {
      await onActivarModulos(pendientes);
      setActivados(true);
    } finally {
      setActivando(false);
    }
  }

  if (paises === null) {
    return (
      <div class="flex h-full items-center justify-center">
        <Spinner size={24} />
      </div>
    );
  }

  const errNombre = nombre.trim() === '' ? t('form.required') : null;

  const stepPais: WizardStep = {
    id: 'pais',
    title: t('onboarding.step.country'),
    heading: t('onboarding.country.heading'),
    canContinue: Boolean(infoPais),
    content: (
      <div class="flex max-w-xl flex-col gap-4">
        <FieldGroup label={t('onboarding.step.country')} htmlFor="ob-pais" required>
          <Select id="ob-pais" value={pais} onChange={(e) => elegirPais(e.currentTarget.value)}>
            {paises
              .map((p) => ({ codigo: p.pais, nombre: nombreDePais(p.pais, locale) }))
              .sort((a, b) => a.nombre.localeCompare(b.nombre, locale))
              .map((p) => (
                <option key={p.codigo} value={p.codigo}>
                  {p.nombre}
                </option>
              ))}
          </Select>
        </FieldGroup>
        {infoPais ? (
          <p class="text-text-muted flex items-start gap-2 text-sm">
            <Badge tone={infoPais.verificado ? 'success' : 'neutral'}>
              {infoPais.verificado ? t('obligations.verified') : t('obligations.indicative')}
            </Badge>
            <span>
              {infoPais.verificado
                ? t('onboarding.country.verified')
                : t('onboarding.country.indicative')}
            </span>
          </p>
        ) : null}
      </div>
    ),
  };

  const stepForma: WizardStep = {
    id: 'forma',
    title: t('onboarding.step.form'),
    heading: t('onboarding.form.heading'),
    canContinue: Boolean(formaElegida),
    content: (
      <div class="flex max-w-xl flex-col gap-4">
        <ul class="grid gap-3 sm:grid-cols-2">
          {(
            [
              [
                'persona_fisica',
                'user',
                'onboarding.form.individual',
                'onboarding.form.individualBody',
              ],
              ['sociedad', 'building', 'onboarding.form.company', 'onboarding.form.companyBody'],
            ] as const
          ).map(([valor, icono, titulo, cuerpo]) => (
            <li key={valor}>
              <ChoiceCard
                name="ob-tipo"
                value={valor}
                icon={icono}
                title={t(titulo)}
                description={t(cuerpo)}
                selected={tipo === valor}
                onSelect={() => elegirTipo(valor)}
              />
            </li>
          ))}
        </ul>
        {tipo ? (
          <FieldGroup label={t('onboarding.form.specific')} htmlFor="ob-forma" required>
            <Select id="ob-forma" value={forma} onChange={(e) => setForma(e.currentTarget.value)}>
              <option value="">{t('relation.choose')}</option>
              {formasDelTipo.map((f) => (
                <option key={f.clave} value={f.clave}>
                  {f.nombre}
                </option>
              ))}
            </Select>
          </FieldGroup>
        ) : null}
      </div>
    ),
  };

  const stepDatos: WizardStep = {
    id: 'datos',
    title: t('onboarding.step.details'),
    heading: t('onboarding.details.heading'),
    onNext: () => {
      if (errNombre) {
        setTocado(true);
        return;
      }
      ir('situacion');
    },
    content: (
      <div class="flex max-w-xl flex-col gap-4">
        <FieldGroup
          label={t('onboarding.details.name')}
          htmlFor="ob-nombre"
          required
          {...(tocado && errNombre ? { error: errNombre } : {})}
        >
          <Input
            id="ob-nombre"
            value={nombre}
            invalid={tocado && Boolean(errNombre)}
            onInput={(e) => setNombre(e.currentTarget.value)}
            onBlur={() => setTocado(true)}
          />
        </FieldGroup>
        <FieldGroup label={t('onboarding.details.taxId')} htmlFor="ob-nif">
          <Input id="ob-nif" value={nif} onInput={(e) => setNif(e.currentTarget.value)} />
        </FieldGroup>
        <FieldGroup label={t('onboarding.details.address')} htmlFor="ob-direccion">
          <Input
            id="ob-direccion"
            value={direccion}
            onInput={(e) => setDireccion(e.currentTarget.value)}
          />
        </FieldGroup>
        <div class="grid gap-4 sm:grid-cols-[1fr_2fr]">
          <FieldGroup label={t('onboarding.details.postcode')} htmlFor="ob-cp">
            <Input id="ob-cp" value={cp} onInput={(e) => setCp(e.currentTarget.value)} />
          </FieldGroup>
          <FieldGroup label={t('onboarding.details.city')} htmlFor="ob-ciudad">
            <Input
              id="ob-ciudad"
              value={ciudad}
              onInput={(e) => setCiudad(e.currentTarget.value)}
            />
          </FieldGroup>
        </div>
      </div>
    ),
  };

  const ROTULO_COND: Record<(typeof CONDICIONES)[number], Parameters<Traducir>[0]> = {
    iva: 'onboarding.situation.iva',
    iva_mensual: 'onboarding.situation.ivaMonthly',
    empleados: 'onboarding.situation.employees',
    intracomunitario: 'onboarding.situation.intra',
    alquiler: 'onboarding.situation.rent',
  };

  const stepSituacion: WizardStep = {
    id: 'situacion',
    title: t('onboarding.step.situation'),
    heading: t('onboarding.situation.heading'),
    content: (
      <div class="flex max-w-xl flex-col gap-3">
        {CONDICIONES.filter((c) => c !== 'iva_mensual' || cond.iva).map((c) => (
          <Switch
            key={c}
            label={t(ROTULO_COND[c])}
            checked={cond[c] === true}
            onChange={(e) => {
              const marcado = e.currentTarget.checked;
              setCond((prev) => ({
                ...prev,
                [c]: marcado,
                // Sin IVA no hay periodicidad de IVA que elegir.
                ...(c === 'iva' && !marcado ? { iva_mensual: false } : {}),
              }));
            }}
          />
        ))}
      </div>
    ),
  };

  const resumen = [
    {
      label: t('onboarding.step.country'),
      value: nombreDePais(pais, locale),
      onEdit: () => ir('pais'),
    },
    {
      label: t('onboarding.form.specific'),
      value: formaElegida?.nombre ?? '—',
      onEdit: () => ir('forma'),
    },
    { label: t('onboarding.details.name'), value: nombre.trim() || '—', onEdit: () => ir('datos') },
    {
      label: t('onboarding.details.taxId'),
      value: nif.trim() || '—',
      mono: true,
      onEdit: () => ir('datos'),
    },
    {
      label: t('onboarding.step.situation'),
      value:
        CONDICIONES.filter((c) => cond[c])
          .map((c) => t(ROTULO_COND[c]))
          .join(' · ') || '—',
      onEdit: () => ir('situacion'),
    },
  ];

  const stepResumen: WizardStep = {
    id: 'resumen',
    title: t('onboarding.step.review'),
    heading: t('onboarding.review.heading'),
    nextLabel: t('onboarding.review.save'),
    onNext: () => void guardar(),
    content: (
      <ReviewSummary items={resumen} editLabel={t('onboarding.review.edit')} class="max-w-xl" />
    ),
  };

  const stepListo: WizardStep = {
    id: 'listo',
    title: t('onboarding.step.done'),
    heading: t('onboarding.done.heading'),
    done: true,
    content: (
      <div class="flex max-w-xl flex-col gap-5">
        <p class="text-text text-base">
          {total !== null ? t('onboarding.done.count', { count: total, year: anio }) : ''}
        </p>
        <div class="flex flex-wrap gap-2">
          <Button variant="primary" onClick={onVerCalendario}>
            <Icon name="calendar" size={14} class="mr-1" />
            {t('onboarding.done.calendar')}
          </Button>
          <Button
            variant="secondary"
            href={withBase(baseDeLaApp(), `/api/obligaciones/ics${consulta({ anio })}`)}
          >
            <Icon name="download" size={14} class="mr-1" />
            {t('onboarding.done.ics')}
          </Button>
        </div>
        {recomendados.length > 0 ? (
          <section class="border-border rounded-card border p-4">
            <h3 class="text-text text-sm font-semibold">{t('onboarding.done.modules')}</h3>
            <p class="text-text-muted mt-1 text-sm">{t('onboarding.done.modulesBody')}</p>
            <ul class="mt-3 flex flex-wrap gap-1.5">
              {recomendados.map((c) => (
                <li key={c}>
                  <Badge tone={activos.includes(c) || activados ? 'success' : 'neutral'}>
                    {modulos.find((m) => m.clave === c)?.etiqueta ?? c}
                  </Badge>
                </li>
              ))}
            </ul>
            {pendientes.length > 0 && !activados ? (
              <Button
                class="mt-3"
                variant="secondary"
                size="sm"
                loading={activando}
                onClick={() => void activarRecomendados()}
              >
                {t('onboarding.done.enableAll')}
              </Button>
            ) : null}
          </section>
        ) : null}
      </div>
    ),
  };

  const porId: Record<(typeof PASOS)[number], WizardStep> = {
    pais: stepPais,
    forma: stepForma,
    datos: stepDatos,
    situacion: stepSituacion,
    resumen: stepResumen,
    listo: stepListo,
  };

  return (
    <div class="mx-auto max-w-4xl px-6 py-6">
      <h1 class="text-text mb-4 text-lg font-semibold tracking-tight">{t('onboarding.title')}</h1>
      <Wizard
        steps={PASOS.map((id) => porId[id])}
        current={paso}
        onCurrentChange={setPaso}
        busy={ocupado}
        onCancel={onCancelar}
        labels={{ cancel: t('onboarding.cancel') }}
      />
    </div>
  );
}
