/**
 * OrgDomainCard — LOS dominios de correo de la organización.
 *
 * Conectar un dominio propio solo se podía hacer dentro del asistente de alta, y
 * el asistente se pasa una vez: quien lo saltó, o quien compró el dominio meses
 * después —lo normal—, no tenía ninguna pantalla desde la que hacerlo. Y cuando
 * la hubo, cabía UNO: una empresa con dos marcas, o con el .es y el .com de la
 * misma, tenía que elegir cuál de sus dominios recibía correo.
 *
 * LOS DOMINIOS SON UNA LISTA, y uno de ellos es el PRINCIPAL. Esa es la única
 * distinción que hay que entender aquí, así que se dice en la propia lista con
 * una etiqueta y no en un texto de ayuda: el principal es donde nacen las
 * direcciones nuevas; los demás reciben y sirven para enviar-como. Cambiarlo muda
 * a toda la plantilla y por eso se confirma.
 *
 * EL SUBDOMINIO DE LA SUITE, siempre arriba y fuera de la lista. No es un dominio
 * conectado: existe pase lo que pase, no se verifica y no se puede quitar. Va
 * separado para que no parezca una fila más que alguien pueda borrar, y porque la
 * duda de quien conecta su dominio es siempre «¿y entonces las direcciones de
 * antes dejan de funcionar?» — la respuesta tiene que estar en la pantalla.
 *
 * Solo @sytue/ui. docs/CONVENTIONS.md §8.
 */
import { useState } from 'preact/hooks';
import {
  Badge,
  Button,
  Card,
  CardBody,
  CardHeader,
  CardTitle,
  InlineHelp,
  Input,
  ToastProvider,
  useToast,
  ConfirmDialog,
} from '@sytue/ui';
import type { Locale } from '@sytue/i18n';
import { useT, type T } from '../i18n/index.js';
import type { OrgMailDomain, OrgMailDomainsState } from '../lib/admin.js';

/**
 * Una fila «etiqueta · valor · copiar» del registro DNS.
 *
 * ponytail: el asistente de alta tiene su propio CopyRow en la app del portal. Dos
 * copias de quince líneas de presentación no justifican mover un componente a
 * @sytue/ui y tocar tres paquetes; a la tercera, se extrae.
 */
function FilaCopiable({
  etiqueta,
  valor,
  copiar,
  onCopiar,
}: {
  etiqueta: string;
  valor: string;
  copiar: string;
  onCopiar: () => void;
}) {
  return (
    <div class="border-border bg-bg-subtle flex items-center gap-3 rounded-md border px-3 py-2">
      <span class="text-text-muted w-20 shrink-0 text-xs">{etiqueta}</span>
      <code class="text-text min-w-0 flex-1 font-mono text-xs break-all">{valor}</code>
      <Button size="sm" variant="ghost" onClick={onCopiar}>
        {copiar}
      </Button>
    </div>
  );
}

/** El estado del DNS, en una frase. Texto además de color, nunca solo color. */
function EstadoDNS({ d, t }: { d: OrgMailDomain; t: T }) {
  if (d.verified) {
    return (
      <InlineHelp tone="success" title={t('orgDomain.verified')}>
        {t('orgDomain.verifiedHint', { host: d.host })}
      </InlineHelp>
    );
  }
  switch (d.status) {
    case 'mismatch':
      return (
        <InlineHelp tone="warning" title={t('orgDomain.mismatch')}>
          {t('orgDomain.mismatchHint', { name: d.record_name, found: d.found?.[0] ?? '' })}
        </InlineHelp>
      );
    case 'unreachable':
      return (
        <InlineHelp tone="danger" title={t('orgDomain.unreachable')}>
          {t('orgDomain.unreachableHint')}
        </InlineHelp>
      );
    default:
      return (
        <InlineHelp tone="neutral" title={t('orgDomain.pending')}>
          {t('orgDomain.pendingHint')}
        </InlineHelp>
      );
  }
}

/** Una fila de la lista: el dominio, en qué estado está y qué se puede hacer. */
function FilaDominio({
  d,
  live,
  ocupado,
  t,
  onComprobar,
  onPrincipal,
  onQuitar,
  onCopiar,
}: {
  d: OrgMailDomain;
  live: boolean;
  ocupado: boolean;
  t: T;
  onComprobar: () => void;
  onPrincipal: () => void;
  onQuitar: () => void;
  onCopiar: (texto: string) => void;
}) {
  return (
    <div class="border-border flex flex-col gap-3 rounded-md border p-3">
      <div class="flex flex-wrap items-center gap-2">
        <p class="text-text min-w-0 flex-1 font-mono text-sm break-all">@{d.host}</p>
        {d.primary ? <Badge tone="success">{t('orgDomain.primary')}</Badge> : null}
        {!d.primary && d.verified ? <Badge tone="neutral">{t('orgDomain.badgeOwn')}</Badge> : null}
        {!d.verified ? <Badge tone="warning">{t('orgDomain.badgePending')}</Badge> : null}
      </div>

      {/* El registro TXT solo mientras haga falta: en cuanto está verificado, esos
          tres campos son ruido que empuja el resto de la pantalla hacia abajo. */}
      {!d.verified ? (
        <div class="flex flex-col gap-2">
          <p class="text-text-muted text-sm leading-relaxed">{t('orgDomain.recordHint')}</p>
          <FilaCopiable
            etiqueta={t('orgDomain.recordType')}
            valor="TXT"
            copiar={t('orgDomain.copy')}
            onCopiar={() => onCopiar('TXT')}
          />
          <FilaCopiable
            etiqueta={t('orgDomain.recordName')}
            valor={d.record_name}
            copiar={t('orgDomain.copy')}
            onCopiar={() => onCopiar(d.record_name)}
          />
          <FilaCopiable
            etiqueta={t('orgDomain.recordValue')}
            valor={d.record_value}
            copiar={t('orgDomain.copy')}
            onCopiar={() => onCopiar(d.record_value)}
          />
        </div>
      ) : null}

      <EstadoDNS d={d} t={t} />

      <div class="flex flex-wrap gap-2">
        <Button
          size="sm"
          variant={d.verified ? 'secondary' : 'primary'}
          loading={ocupado}
          disabled={!live}
          onClick={onComprobar}
        >
          {d.verified ? t('orgDomain.recheck') : t('orgDomain.check')}
        </Button>
        {/* Estrenar solo tiene sentido en un dominio verificado que no sea ya el
            principal: en cualquier otro caso el botón no haría nada. */}
        {d.verified && !d.primary ? (
          <Button size="sm" variant="secondary" disabled={!live || ocupado} onClick={onPrincipal}>
            {t('orgDomain.makePrimary')}
          </Button>
        ) : null}
        <Button size="sm" variant="ghost" disabled={!live || ocupado} onClick={onQuitar}>
          {t('orgDomain.disconnect')}
        </Button>
      </div>
    </div>
  );
}

function Inner({ state, live, t }: { state: OrgMailDomainsState | null; live: boolean; t: T }) {
  const toast = useToast();
  const [actual, setActual] = useState<OrgMailDomainsState | null>(state);
  const [host, setHost] = useState('');
  const [ocupado, setOcupado] = useState(false);
  const dominios = actual?.items ?? [];

  async function copiar(texto: string) {
    try {
      await navigator.clipboard.writeText(texto);
      toast.notify({ tone: 'success', title: t('orgDomain.copied') });
    } catch {
      // Sin permiso de portapapeles no se finge que se copió: el valor sigue
      // visible y seleccionable, que es lo que de verdad hace falta.
      toast.notify({ tone: 'danger', title: t('orgDomain.copyFailed') });
    }
  }

  function fallo(e?: string) {
    toast.notify({
      tone: 'danger',
      title: t('orgDomain.saveFailed'),
      description: e ?? t('common.tryAgain'),
    });
  }

  /**
   * Añade el dominio o lo vuelve a comprobar: es la misma llamada.
   *
   * La respuesta trae la fila entera —incluido si acaba de quedar verificada y si
   * ha pasado a principal por ser la primera—, así que se sustituye en la lista en
   * vez de recargar: quien acaba de publicar su TXT está mirando esta pantalla.
   */
  async function comprobar(dominio: string) {
    const limpio = dominio.trim();
    if (!limpio || ocupado) return;
    setOcupado(true);
    try {
      const res = await fetch('/api/admin/org/mail-domains', {
        method: 'POST',
        headers: { 'content-type': 'application/json' },
        body: JSON.stringify({ domain: limpio }),
      });
      const body = (await res.json().catch(() => ({}))) as OrgMailDomain & { message?: string };
      if (!res.ok) {
        fallo(body.message);
        return;
      }
      setActual((prev) => (prev ? conDominio(prev, body) : prev));
      setHost('');
      if (body.verified) {
        toast.notify({
          tone: 'success',
          title: t('orgDomain.verified'),
          description: body.primary ? t('orgDomain.movedHint') : t('orgDomain.addedHint'),
        });
      }
    } catch {
      fallo();
    } finally {
      setOcupado(false);
    }
  }

  /**
   * Estrenar un dominio muda la dirección de TODA la plantilla, así que se
   * pregunta antes. La confirmación dice lo que de verdad importa —que las
   * direcciones anteriores se conservan como alias—, porque el miedo de quien
   * pulsa aquí es dejar sin correo a su gente.
   */
  // Las dos acciones que cambian el correo de TODA la plantilla se confirman con un
  // diálogo de la casa, no con el del navegador: aquel pinta sus botones en el
  // idioma del sistema operativo, no deja explicar qué pasa con las direcciones
  // anteriores, y en una tarjeta que habla de no dejar a nadie sin correo eso es
  // justo lo que hay que poder contar.
  const [confirmando, setConfirmando] = useState<{
    accion: 'principal' | 'quitar';
    dominio: OrgMailDomain;
  } | null>(null);

  async function hacerPrincipal(d: OrgMailDomain) {
    setConfirmando(null);
    if (ocupado) return;
    setOcupado(true);
    try {
      const res = await fetch(`/api/admin/org/mail-domains/${d.id}/primary`, { method: 'POST' });
      const body = (await res.json().catch(() => ({}))) as OrgMailDomain & { message?: string };
      if (!res.ok) {
        fallo(body.message);
        return;
      }
      setActual((prev) =>
        prev
          ? {
              ...prev,
              mail_domain: body.host,
              items: prev.items.map((x) => ({ ...x, primary: x.id === body.id })),
            }
          : prev,
      );
      toast.notify({
        tone: 'success',
        title: t('orgDomain.primaryChanged'),
        description: t('orgDomain.movedHint'),
      });
    } catch {
      fallo();
    } finally {
      setOcupado(false);
    }
  }

  async function quitar(d: OrgMailDomain) {
    setConfirmando(null);
    if (ocupado) return;
    setOcupado(true);
    try {
      const res = await fetch(`/api/admin/org/mail-domains/${d.id}`, { method: 'DELETE' });
      if (!res.ok && res.status !== 204) throw new Error(String(res.status));
      // Quitar el principal asciende a otro verificado (lo decide el servicio), y
      // adivinar aquí a cuál sería contar algo que puede no ser cierto: se recarga.
      if (d.primary) {
        window.location.reload();
        return;
      }
      setActual((prev) =>
        prev ? { ...prev, items: prev.items.filter((x) => x.id !== d.id) } : prev,
      );
      toast.notify({
        tone: 'success',
        title: t('orgDomain.disconnected'),
        description: t('orgDomain.disconnectedHint'),
      });
    } catch {
      fallo();
    } finally {
      setOcupado(false);
    }
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('orgDomain.title')}</CardTitle>
        {dominios.some((d) => d.verified) ? (
          <Badge tone="success">{t('orgDomain.badgeOwn')}</Badge>
        ) : null}
      </CardHeader>
      <CardBody class="flex flex-col gap-4">
        <p class="text-text-muted text-sm">{t('orgDomain.description')}</p>
        {!live ? <p class="text-warning-text text-xs">{t('orgDomain.offline')}</p> : null}

        {/* El cimiento: existe siempre y no se puede quitar, así que no se enseña
            como un campo editable que invite a intentarlo. */}
        <div class="border-border rounded-md border p-3">
          <p class="text-text text-sm font-medium">{t('orgDomain.base')}</p>
          <p class="text-text mt-1 font-mono text-sm">@{actual?.base_domain ?? '—'}</p>
          <p class="text-text-muted mt-1 text-xs leading-relaxed">{t('orgDomain.baseHint')}</p>
        </div>

        {dominios.length > 0 ? (
          <div class="flex flex-col gap-3">
            {dominios.map((d) => (
              <FilaDominio
                key={d.id}
                d={d}
                live={live}
                ocupado={ocupado}
                t={t}
                onComprobar={() => void comprobar(d.host)}
                onPrincipal={() => setConfirmando({ accion: 'principal', dominio: d })}
                onQuitar={() => setConfirmando({ accion: 'quitar', dominio: d })}
                onCopiar={(texto) => void copiar(texto)}
              />
            ))}
          </div>
        ) : null}

        {/* Añadir está SIEMPRE disponible, también con dominios ya conectados: que
            el formulario desapareciera al conectar el primero es justo lo que
            obligaba a desconectarlo para poder poner otro. */}
        <div class="flex flex-col gap-2">
          <label class="text-text text-sm font-medium" for="org-domain">
            {dominios.length > 0 ? t('orgDomain.addAnother') : t('orgDomain.own')}
          </label>
          <div class="flex flex-wrap items-start gap-2">
            <Input
              id="org-domain"
              class="max-w-xs flex-1"
              placeholder="acme.com"
              value={host}
              disabled={!live}
              onInput={(e) => setHost(e.currentTarget.value)}
            />
            <Button
              variant="primary"
              size="sm"
              loading={ocupado}
              disabled={!live || host.trim() === ''}
              onClick={() => void comprobar(host)}
            >
              {t('orgDomain.connect')}
            </Button>
          </div>
          <p class="text-text-muted text-xs leading-relaxed">{t('orgDomain.connectHint')}</p>
        </div>
      </CardBody>
      <ConfirmDialog
        open={confirmando !== null}
        onClose={() => setConfirmando(null)}
        onConfirm={() => {
          if (!confirmando) return;
          if (confirmando.accion === 'principal') void hacerPrincipal(confirmando.dominio);
          else void quitar(confirmando.dominio);
        }}
        title={t(
          confirmando?.accion === 'quitar' ? 'orgDomain.disconnect' : 'orgDomain.makePrimary',
        )}
        confirmLabel={t(
          confirmando?.accion === 'quitar' ? 'orgDomain.disconnect' : 'orgDomain.makePrimary',
        )}
        tone={confirmando?.accion === 'quitar' ? 'danger' : 'primary'}
        busy={ocupado}
      >
        <p class="text-text text-sm leading-relaxed">
          {confirmando
            ? t(
                confirmando.accion === 'quitar'
                  ? 'orgDomain.disconnectConfirm'
                  : 'orgDomain.makePrimaryConfirm',
                { host: confirmando.dominio.host },
              )
            : ''}
        </p>
      </ConfirmDialog>
    </Card>
  );
}

/** Sustituye (o añade) un dominio en la lista conservando el orden de llegada. */
function conDominio(estado: OrgMailDomainsState, d: OrgMailDomain): OrgMailDomainsState {
  const conocido = estado.items.some((x) => x.id === d.id);
  const items = conocido
    ? estado.items.map((x) => (x.id === d.id ? d : { ...x, primary: x.primary && !d.primary }))
    : [...estado.items, d];
  return { ...estado, items, mail_domain: d.primary ? d.host : estado.mail_domain };
}

export default function OrgDomainCard({
  locale,
  ...props
}: {
  state: OrgMailDomainsState | null;
  live: boolean;
  locale: Locale;
}) {
  return (
    <ToastProvider>
      <Inner {...props} t={useT(locale)} />
    </ToastProvider>
  );
}
