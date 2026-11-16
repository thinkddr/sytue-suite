/**
 * Las reglas de silencio de la bandeja de One: un interruptor por cada origen que
 * puede avisar (`ORIGENES_DE_AVISOS` de @sytue/shell), agrupados por familia.
 *
 * Aparte de la bandeja porque tiene su propio ciclo —carga, fallo, guardado— y la
 * bandeja sigue siendo útil aunque el Hub no sirva las preferencias. Antes vivía
 * dentro con dos fallos mudos: si la carga fallaba, todo salía ENCENDIDO aunque
 * hubiera cosas calladas; y si el guardado fallaba, el interruptor se quedaba en la
 * posición nueva como si se hubiera guardado.
 *
 * Bajo cada origen, «Por tipo» despliega sus tipos de aviso (`TIPOS_DE_AVISOS`): se
 * puede callar cada nuevo correo y seguir recibiendo las peticiones de acceso, o callar
 * todo Rooms salvo las menciones. Cada tipo enseña lo que de verdad decide el Hub (la
 * regla más específica gana) y los críticos —un cobro duplicado, un desistimiento, una
 * denuncia— salen fijos: el Hub los entrega aunque se calle el origen entero.
 */
import type { JSX } from 'preact';
import { useEffect, useState } from 'preact/hooks';
import type { HubClient, NotificationPref, NotificationSource } from '@sytue/api-clients';
import { esSoloDeOrganizaciones } from '@sytue/config';
import {
  appForKey,
  Button,
  FieldHint,
  Icon,
  InlineHelp,
  SettingsSection,
  Skeleton,
  Switch,
} from '@sytue/ui';
import type { Locale } from '@sytue/i18n';
import { ORIGENES_DE_AVISOS, TIPOS_DE_AVISOS, type TipoDeAviso } from '@sytue/shell';
import { useT } from '../i18n/index.js';
import { filaSilenciada, ORIGENES_FIJOS, silenciado } from '../lib/reglasAvisos.js';

/** Lo que se usa del cliente del Hub: un subconjunto, para probarlo con un doble. */
export type FuentePreferencias = Pick<HubClient, 'notificationPrefs' | 'setNotificationPref'>;

export interface PreferenciasAvisosProps {
  locale: Locale;
  hub: FuentePreferencias;
  /**
   * Organización activa (claim `tenant`). Las reglas del Hub son por persona Y
   * organización (`notification_prefs` lleva las dos), y la pantalla tiene que decirlo:
   * callar Mail aquí no lo calla en otra organización.
   */
  organizacion?: string | undefined;
  /** Cuenta personal: ni ámbito de organización ni los productos solo de organizaciones. */
  personal?: boolean;
}

type Carga = 'cargando' | 'fallo' | 'lista';

/** Lo que se guarda o falla: un origen entero (`mail`) o una fila de tipos (`mail:messages`). */
const claveDeFila = (source: NotificationSource, fila: TipoDeAviso): string =>
  `${source}:${fila.id}`;

/** Las reglas con la de `source`+`tipo` cambiada por `regla` (o quitada, sin ella). */
const conReglaDeTipo = (
  lista: NotificationPref[],
  source: NotificationSource,
  tipo: string,
  regla: NotificationPref | undefined,
): NotificationPref[] => [
  ...lista.filter((p) => !(p.source === source && p.kind === tipo)),
  ...(regla ? [regla] : []),
];

export function PreferenciasAvisos({
  locale,
  hub,
  organizacion,
  personal = false,
}: PreferenciasAvisosProps): JSX.Element {
  const t = useT(locale);
  const [carga, setCarga] = useState<Carga>('cargando');
  const [prefs, setPrefs] = useState<NotificationPref[]>([]);
  const [intento, setIntento] = useState(0);
  // Por origen: el que se está guardando no admite otro clic (dos PUT cruzados
  // dejarían en el Hub el que llegue último, no el último que se pulsó)...
  const [guardando, setGuardando] = useState<ReadonlySet<string>>(new Set());
  // ...y el que falló dice por qué ha vuelto a su sitio, en su fila.
  const [fallidos, setFallidos] = useState<ReadonlySet<string>>(new Set());
  // Los orígenes con «Por tipo» desplegado. Cerrado de entrada: lo normal es callar un
  // producto entero, y treinta interruptores más de golpe esconderían los de arriba.
  const [abiertos, setAbiertos] = useState<ReadonlySet<NotificationSource>>(new Set());

  useEffect(() => {
    let cancelado = false;
    setCarga('cargando');
    hub.notificationPrefs().then(
      (p) => {
        if (cancelado) return;
        setPrefs(p);
        setCarga('lista');
      },
      () => {
        if (!cancelado) setCarga('fallo');
      },
    );
    return () => {
      cancelado = true;
    };
  }, [hub, intento]);

  const conRegla = (
    lista: NotificationPref[],
    source: NotificationSource,
    muted: boolean,
  ): NotificationPref[] => [
    ...lista.filter((p) => !(p.source === source && !p.kind)),
    { source, muted },
  ];

  const con = <T,>(s: ReadonlySet<T>, x: T, dentro: boolean): ReadonlySet<T> => {
    const n = new Set(s);
    if (dentro) n.add(x);
    else n.delete(x);
    return n;
  };

  const cambiar = async (source: NotificationSource, muted: boolean): Promise<void> => {
    const antes = silenciado(prefs, source);
    // Volver a permitir el origen se lleva sus contra-reglas de tipo, como el Hub: ya
    // no contradicen nada, y si quedaran, callarlo más tarde dejaría pasar esos tipos.
    const contra = muted ? [] : prefs.filter((p) => p.source === source && p.kind && !p.muted);
    const sinContra = (l: NotificationPref[]): NotificationPref[] =>
      l.filter((p) => !contra.includes(p));
    setPrefs((p) => conRegla(sinContra(p), source, muted));
    setGuardando((s) => con(s, source, true));
    setFallidos((s) => con(s, source, false));
    try {
      await hub.setNotificationPref({ source, muted });
    } catch {
      // Se deshace SOLO la regla de este origen: los demás pueden haberse guardado
      // entretanto y no se pisan con una foto vieja de la lista.
      setPrefs((p) => [...conRegla(p, source, antes), ...contra]);
      setFallidos((s) => con(s, source, true));
    } finally {
      setGuardando((s) => con(s, source, false));
    }
  };

  const cambiarFila = async (
    source: NotificationSource,
    fila: TipoDeAviso,
    muted: boolean,
  ): Promise<void> => {
    const clave = claveDeFila(source, fila);
    // La regla exacta que tenía cada tipo, para devolverla tal cual si falla.
    const antes = new Map(
      fila.tipos.map((t) => [t, prefs.find((p) => p.source === source && p.kind === t)]),
    );
    // Con el origen sin callar, permitir un tipo es quitar su regla (el Hub la borra):
    // una contra-regla muda haría que callar luego el origen no callara este tipo.
    const origenCallado = silenciado(prefs, source);
    setPrefs((p) =>
      fila.tipos.reduce(
        (l, kind) =>
          conReglaDeTipo(
            l,
            source,
            kind,
            muted || origenCallado ? { source, kind, muted } : undefined,
          ),
        p,
      ),
    );
    setGuardando((s) => con(s, clave, true));
    setFallidos((s) => con(s, clave, false));
    // Una fila puede reunir varios tipos del Hub, y cada uno es su PUT: se esperan
    // todos y se deshacen solo los que fallaron, para que la fila diga lo que quedó.
    const r = await Promise.allSettled(
      fila.tipos.map((kind) => hub.setNotificationPref({ source, kind, muted })),
    );
    const fallaron = fila.tipos.filter((_, i) => r[i]?.status === 'rejected');
    if (fallaron.length > 0) {
      setPrefs((p) => fallaron.reduce((l, t) => conReglaDeTipo(l, source, t, antes.get(t)), p));
      setFallidos((s) => con(s, clave, true));
    }
    setGuardando((s) => con(s, clave, false));
  };

  const nombre = (source: NotificationSource): string =>
    source === 'integrations'
      ? t('inbox.prefsSource.integrations')
      : (appForKey(source)?.name ?? source);

  const grupos = ORIGENES_DE_AVISOS.map((g) => ({
    ...g,
    // Una cuenta personal no abre Sytue ERP, Bases ni Web: ofrecer callarlos es ruido.
    origenes: g.origenes.filter((o) => !(personal && esSoloDeOrganizaciones(o))),
  })).filter((g) => g.origenes.length > 0);

  /** «Por tipo» de un origen: el botón que lo despliega y, abierto, sus tipos. */
  const tiposDe = (source: NotificationSource): JSX.Element | null => {
    const filas = TIPOS_DE_AVISOS[source];
    if (!filas?.length) return null;
    const abierto = abiertos.has(source);
    const lista = `pref-tipos-${source}`;
    return (
      <>
        <Button
          variant="ghost"
          size="xs"
          // Bajo el texto del interruptor, no bajo el carril: se lee como parte de él.
          class="ms-9 self-start"
          aria-expanded={abierto}
          aria-controls={lista}
          data-tipos={source}
          trailingIcon={<Icon name={abierto ? 'chevronUp' : 'chevronDown'} size={14} />}
          onClick={() => setAbiertos((s) => con(s, source, !abierto))}
        >
          {t('inbox.prefsKindsToggle')}
        </Button>
        {abierto ? (
          <div
            id={lista}
            role="group"
            aria-label={t('inbox.prefsKindsOf', { app: nombre(source) })}
            class="border-border ms-9 flex flex-col gap-2 border-s ps-3"
          >
            {filas.map((fila) => {
              const clave = claveDeFila(source, fila);
              const rotulo = t(`inbox.prefsKind.${fila.id}`);
              if (fila.critico) {
                // Como Sytue ID: encendido, sin poder tocarse y con el porqué debajo.
                const nota = `pref-fijo-${clave}`;
                return (
                  <div key={fila.id} class="flex min-w-0 flex-col gap-0.5">
                    <Switch
                      label={rotulo}
                      checked
                      disabled
                      aria-describedby={nota}
                      data-tipo-fijo={clave}
                    />
                    <span id={nota} class="text-text-muted ps-11.5 text-sm">
                      {t('inbox.prefsKindCritical')}
                    </span>
                  </div>
                );
              }
              const error = `pref-err-${clave}`;
              return (
                <div key={fila.id} class="flex min-w-0 flex-col gap-1">
                  <Switch
                    label={rotulo}
                    checked={!filaSilenciada(prefs, source, fila.tipos)}
                    disabled={guardando.has(clave)}
                    aria-describedby={fallidos.has(clave) ? error : undefined}
                    data-tipo={clave}
                    onChange={(e) => void cambiarFila(source, fila, !e.currentTarget.checked)}
                  />
                  {fallidos.has(clave) ? (
                    <FieldHint id={error} tone="danger">
                      {t('inbox.prefsSaveError')}
                    </FieldHint>
                  ) : null}
                </div>
              );
            })}
          </div>
        ) : null}
      </>
    );
  };

  const ambito =
    !personal && organizacion
      ? t('inbox.prefsScopeOrg', { org: organizacion })
      : t('inbox.prefsScopePersonal');

  return (
    <SettingsSection
      title={t('inbox.prefsTitle')}
      description={`${t('inbox.prefsBody')} ${ambito}`}
    >
      {carga === 'fallo' ? (
        // Sin las reglas no se sabe qué está callado: se dice, y NO se enseñan
        // interruptores, que saldrían todos encendidos.
        <InlineHelp
          tone="danger"
          title={t('inbox.prefsLoadError')}
          action={
            <Button variant="secondary" size="sm" onClick={() => setIntento((n) => n + 1)}>
              {t('inbox.retry')}
            </Button>
          }
        >
          {t('inbox.prefsLoadErrorBody')}
        </InlineHelp>
      ) : (
        <div class="flex flex-col gap-6" aria-busy={carga === 'cargando'}>
          {carga === 'cargando' ? (
            <p class="sr-only" role="status">
              {t('inbox.prefsLoading')}
            </p>
          ) : null}
          {grupos.map((g) => (
            <fieldset key={g.grupo} class="flex min-w-0 flex-col gap-3">
              <legend class="text-text-subtle mb-3 text-xs font-semibold tracking-wide uppercase">
                {t(`inbox.prefsGroup.${g.grupo}`)}
              </legend>
              <div class="grid gap-x-6 gap-y-3 sm:grid-cols-2">
                {g.origenes.map((source) =>
                  carga === 'cargando' ? (
                    // Fantasma con la forma del interruptor: nada que pulsar mientras
                    // no se sabe en qué posición está.
                    <div key={source} class="flex items-center gap-2.5" data-fantasma={source}>
                      <Skeleton width="36px" height="20px" />
                      <Skeleton width="45%" height="14px" />
                    </div>
                  ) : (
                    <div key={source} class="flex min-w-0 flex-col gap-1">
                      <Switch
                        label={nombre(source)}
                        // El interruptor dice «recibir»; el Hub guarda «silenciado».
                        checked={!silenciado(prefs, source)}
                        disabled={guardando.has(source)}
                        aria-describedby={fallidos.has(source) ? `pref-err-${source}` : undefined}
                        data-origen={source}
                        onChange={(e) => void cambiar(source, !e.currentTarget.checked)}
                      />
                      {fallidos.has(source) ? (
                        <FieldHint id={`pref-err-${source}`} tone="danger">
                          {t('inbox.prefsSaveError')}
                        </FieldHint>
                      ) : null}
                      {tiposDe(source)}
                    </div>
                  ),
                )}
              </div>
            </fieldset>
          ))}
          {/* Los avisos de seguridad, encendidos y deshabilitados, con el porqué debajo:
              esconderlos dejaría creer que no llegan, y un interruptor que no responde,
              que está roto. El Hub rechazaría callarlos igual (`domain.Silenciable`). */}
          {carga === 'lista'
            ? ORIGENES_FIJOS.map((source) => {
                const nota = `pref-fijo-${source}`;
                return (
                  <div key={source} class="flex min-w-0 flex-col gap-0.5">
                    <Switch
                      label={nombre(source)}
                      checked
                      disabled
                      aria-describedby={nota}
                      data-fijo={source}
                    />
                    <span id={nota} class="text-text-muted ps-11.5 text-sm">
                      {t('inbox.prefsAlwaysOn')}
                    </span>
                  </div>
                );
              })
            : null}
        </div>
      )}
    </SettingsSection>
  );
}
