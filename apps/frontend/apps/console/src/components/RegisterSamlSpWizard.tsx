/**
 * RegisterSamlSpWizard — asistente «Añadir una aplicación SAML a mano».
 *
 * Para la app de terceros que habla SAML y NO está en el catálogo de plantillas: Sytue
 * ID hace de proveedor de identidad y la app es el proveedor de servicio (SP). Cuatro
 * pasos: qué app es y a dónde manda Sytue la respuesta → qué identidad le llega
 * (NameID, nombres de los atributos, certificado de la app) → repaso → los datos del
 * IdP que hay que pegar en la app.
 *
 * No inventa API: POST /api/admin/apps/saml-sps de la BFF. Los fallos se dicen en el
 * campo que los tiene, ANTES de enviar (`lib/saml-sp.ts`, las mismas reglas que Sytue
 * ID); si aun así Sytue ID dice que no, se vuelve al paso que lo corrige.
 *
 * Los textos vienen del catálogo (`wizard.saml.*`): la `t` llega ya atada al idioma
 * desde SsoPanel.
 */
import { useState } from 'preact/hooks';
import {
  Button,
  Checkbox,
  CopyField,
  DataFlow,
  FieldGroup,
  FieldHint,
  InlineHelp,
  Input,
  ReviewSummary,
  Select,
  Textarea,
  Wizard,
  useToast,
} from '@sytue/ui';
import type { ReviewItem, WizardStep } from '@sytue/ui';
import { NextStep } from './ConnectIdpWizard.js';
import type { ConsoleMessageKey, T } from '../i18n/index.js';
import type { SamlSp } from '../lib/types.js';
import {
  NAMEID_FORMATS,
  SAML_ATTRS,
  altaSamlSp,
  fallosSamlSp,
  type BorradorSamlSp,
  type NameIdFormat,
  type SamlAttr,
  type SamlSpFallo,
} from '../lib/saml-sp.js';

interface RegisterSamlSpWizardProps {
  /** `t` ya atada al locale; llega del panel, que es quien negocia el idioma. */
  t: T;
  onCreated: (sp: SamlSp) => void;
  onCancel: () => void;
  onDone: () => void;
}

const PASOS = ['app', 'identity', 'review', 'done'] as const;
type Paso = (typeof PASOS)[number];

/** Nombre llano de cada formato de NameID, que la URN no le dice nada a nadie. */
const NAMEID_LABEL: Record<NameIdFormat, ConsoleMessageKey> = {
  'urn:oasis:names:tc:SAML:1.1:nameid-format:emailAddress': 'wizard.saml.nameId.email',
  'urn:oasis:names:tc:SAML:2.0:nameid-format:persistent': 'wizard.saml.nameId.persistent',
  'urn:oasis:names:tc:SAML:2.0:nameid-format:transient': 'wizard.saml.nameId.transient',
  'urn:oasis:names:tc:SAML:1.1:nameid-format:unspecified': 'wizard.saml.nameId.unspecified',
};

const ATTR_LABEL: Record<SamlAttr, ConsoleMessageKey> = {
  email: 'wizard.saml.attr.email',
  name: 'wizard.saml.attr.name',
  groups: 'wizard.saml.attr.groups',
};

const FALLO: Record<SamlSpFallo, ConsoleMessageKey> = {
  nameRequired: 'wizard.saml.err.nameRequired',
  entityIdRequired: 'wizard.saml.err.entityIdRequired',
  entityIdSpaces: 'wizard.saml.err.entityIdSpaces',
  acsRequired: 'wizard.saml.err.acsRequired',
  acsNotUrl: 'wizard.saml.err.acsNotUrl',
  acsInsecure: 'wizard.saml.err.acsInsecure',
  certPrivateKey: 'wizard.saml.err.certPrivateKey',
  certNotPem: 'wizard.saml.err.certNotPem',
  certTooLarge: 'wizard.saml.err.certTooLarge',
  nameIdUnknown: 'wizard.saml.err.nameIdUnknown',
  attrSpaces: 'wizard.saml.err.attrSpaces',
};

/** El paso al que lleva un fallo que devuelve la BFF o Sytue ID tras enviar. */
export function pasoDelRechazo(status: number, code: string | undefined): Paso | null {
  if (status === 409) return 'app';
  if (status !== 400) return null;
  switch (code) {
    case 'nameRequired':
    case 'entityIdRequired':
    case 'entityIdSpaces':
    case 'acsRequired':
    case 'acsNotUrl':
    case 'acsInsecure':
      return 'app';
    default:
      // El certificado es lo único que Sytue ID mira más a fondo que el navegador.
      return 'identity';
  }
}

function arreglo(status: number, t: T): string {
  switch (status) {
    case 400:
      return t('wizard.saml.fix.badRequest');
    case 409:
      return t('wizard.saml.fix.conflict');
    case 403:
      return t('wizard.saml.fix.forbidden');
    case 502:
    case 503:
    case 504:
      return t('wizard.saml.fix.idpDown');
    default:
      return t('wizard.saml.fix.generic');
  }
}

export default function RegisterSamlSpWizard({
  t,
  onCreated,
  onCancel,
  onDone,
}: RegisterSamlSpWizardProps) {
  const toast = useToast();
  const [borrador, setBorrador] = useState<BorradorSamlSp>({
    name: '',
    entityId: '',
    acsUrl: '',
    certPem: '',
    nameIdFormat: NAMEID_FORMATS[0],
    attrs: {},
    idpInitiated: false,
    relayState: '',
  });
  const [tocados, setTocados] = useState<Partial<Record<keyof BorradorSamlSp, boolean>>>({});
  const [busy, setBusy] = useState(false);
  const [creado, setCreado] = useState<SamlSp | null>(null);
  /** Lo que Sytue ID rechazó tras enviar (409 o un certificado que no es X.509). */
  const [rechazo, setRechazo] = useState<{ campo: keyof BorradorSamlSp; texto: string } | null>(
    null,
  );
  const [paso, setPaso] = useState(0);
  const ir = (id: Paso) => setPaso(PASOS.indexOf(id));

  const fallos = fallosSamlSp(borrador);
  const poner = <K extends keyof BorradorSamlSp>(k: K, v: BorradorSamlSp[K]) => {
    setBorrador((prev) => ({ ...prev, [k]: v }));
    if (rechazo?.campo === k) setRechazo(null);
  };
  const tocar = (...ks: (keyof BorradorSamlSp)[]) =>
    setTocados((prev) => ({ ...prev, ...Object.fromEntries(ks.map((k) => [k, true])) }));
  /** El texto de error de un campo, solo si ya se tocó (o lo rechazó Sytue ID). */
  const error = (k: keyof BorradorSamlSp): string | null => {
    if (rechazo?.campo === k) return rechazo.texto;
    const f = fallos[k];
    return f && tocados[k] ? t(FALLO[f]) : null;
  };

  async function enviar() {
    if (busy) return;
    setBusy(true);
    try {
      const res = await fetch('/api/admin/apps/saml-sps', {
        method: 'POST',
        headers: { 'content-type': 'application/json' },
        body: JSON.stringify(altaSamlSp(borrador)),
      });
      if (!res.ok) {
        const cuerpo = (await res.json().catch(() => ({}))) as { code?: string; message?: string };
        throw new Rechazo(res.status, cuerpo.code);
      }
      const sp = (await res.json()) as SamlSp;
      setCreado(sp);
      onCreated(sp);
      ir('done');
    } catch (e) {
      const status = e instanceof Rechazo ? e.status : 0;
      const volver = pasoDelRechazo(status, e instanceof Rechazo ? e.code : undefined);
      if (volver) {
        // El fallo se queda pegado al campo que lo corrige, no solo en un aviso que se va.
        const campo: keyof BorradorSamlSp = volver === 'app' ? 'entityId' : 'certPem';
        setRechazo({ campo, texto: arreglo(status, t) });
        ir(volver);
      }
      toast.notify({
        tone: 'danger',
        title: t('wizard.saml.toast.saveFailed'),
        description: arreglo(status, t),
      });
    } finally {
      setBusy(false);
    }
  }

  // ---- Pasos ----

  const errName = error('name');
  const errEntity = error('entityId');
  const errAcs = error('acsUrl');
  const pasoApp: WizardStep = {
    id: 'app',
    title: t('wizard.saml.step.app'),
    heading: t('wizard.saml.app.heading'),
    description: t('wizard.saml.app.description'),
    onNext: () => {
      tocar('name', 'entityId', 'acsUrl');
      if (fallos.name || fallos.entityId || fallos.acsUrl || rechazo?.campo === 'entityId') return;
      ir('identity');
    },
    content: (
      <div class="flex max-w-xl flex-col gap-5">
        <FieldGroup label={t('wizard.saml.app.nameLabel')} htmlFor="saml-name" required>
          <Input
            id="saml-name"
            value={borrador.name}
            placeholder={t('wizard.saml.app.namePlaceholder')}
            aria-describedby="saml-name-hint"
            invalid={Boolean(errName)}
            onInput={(e) => poner('name', e.currentTarget.value)}
            onBlur={() => tocar('name')}
          />
          <FieldHint id="saml-name-hint" tone={errName ? 'danger' : 'neutral'}>
            {errName ?? t('wizard.saml.app.nameHint')}
          </FieldHint>
        </FieldGroup>
        <FieldGroup label={t('wizard.saml.app.entityIdLabel')} htmlFor="saml-entity" required>
          <Input
            id="saml-entity"
            value={borrador.entityId}
            class="font-mono text-xs"
            spellcheck={false}
            placeholder="https://app.acme.com/saml/metadata"
            aria-describedby="saml-entity-hint"
            invalid={Boolean(errEntity)}
            onInput={(e) => poner('entityId', e.currentTarget.value)}
            onBlur={() => tocar('entityId')}
          />
          <FieldHint id="saml-entity-hint" tone={errEntity ? 'danger' : 'neutral'}>
            {errEntity ?? t('wizard.saml.app.entityIdHint')}
          </FieldHint>
        </FieldGroup>
        <FieldGroup label={t('wizard.saml.app.acsLabel')} htmlFor="saml-acs" required>
          <Input
            id="saml-acs"
            value={borrador.acsUrl}
            class="font-mono text-xs"
            spellcheck={false}
            placeholder="https://app.acme.com/saml/acs"
            aria-describedby="saml-acs-hint"
            invalid={Boolean(errAcs)}
            onInput={(e) => poner('acsUrl', e.currentTarget.value)}
            onBlur={() => tocar('acsUrl')}
          />
          <FieldHint id="saml-acs-hint" tone={errAcs ? 'danger' : 'neutral'}>
            {errAcs ?? t('wizard.saml.app.acsHint')}
          </FieldHint>
        </FieldGroup>
        <InlineHelp tone="info" title={t('wizard.saml.app.whereTitle')}>
          {t('wizard.saml.app.whereBody')}
        </InlineHelp>
      </div>
    ),
  };

  const errCert = error('certPem');
  const errAttrs = error('attrs');
  const pasoIdentidad: WizardStep = {
    id: 'identity',
    title: t('wizard.saml.step.identity'),
    heading: t('wizard.saml.identity.heading'),
    description: t('wizard.saml.identity.description'),
    onNext: () => {
      tocar('certPem', 'attrs', 'nameIdFormat');
      if (fallos.certPem || fallos.attrs || fallos.nameIdFormat || rechazo?.campo === 'certPem')
        return;
      ir('review');
    },
    content: (
      <div class="flex max-w-xl flex-col gap-5">
        <FieldGroup label={t('wizard.saml.identity.nameIdLabel')} htmlFor="saml-nameid">
          <Select
            id="saml-nameid"
            value={borrador.nameIdFormat}
            aria-describedby="saml-nameid-hint"
            onChange={(e) => poner('nameIdFormat', e.currentTarget.value)}
          >
            {NAMEID_FORMATS.map((f) => (
              <option key={f} value={f}>
                {t(NAMEID_LABEL[f])}
              </option>
            ))}
          </Select>
          <FieldHint id="saml-nameid-hint">{t('wizard.saml.identity.nameIdHint')}</FieldHint>
        </FieldGroup>

        <fieldset class="flex flex-col gap-3">
          <legend class="text-text text-sm font-semibold">
            {t('wizard.saml.identity.attrsLegend')}
          </legend>
          <p class="sytue-meta">{t('wizard.saml.identity.attrsHint')}</p>
          <div class="grid gap-3 sm:grid-cols-3">
            {SAML_ATTRS.map((k) => (
              <FieldGroup key={k} label={t(ATTR_LABEL[k])} htmlFor={`saml-attr-${k}`}>
                <Input
                  id={`saml-attr-${k}`}
                  value={borrador.attrs[k] ?? ''}
                  class="font-mono text-xs"
                  spellcheck={false}
                  placeholder={k}
                  invalid={Boolean(errAttrs) && /\s/.test((borrador.attrs[k] ?? '').trim())}
                  onInput={(e) => poner('attrs', { ...borrador.attrs, [k]: e.currentTarget.value })}
                  onBlur={() => tocar('attrs')}
                />
              </FieldGroup>
            ))}
          </div>
          {errAttrs ? <FieldHint tone="danger">{errAttrs}</FieldHint> : null}
        </fieldset>

        <FieldGroup label={t('wizard.saml.identity.certLabel')} htmlFor="saml-cert">
          <Textarea
            id="saml-cert"
            rows={6}
            value={borrador.certPem}
            class="font-mono text-xs"
            spellcheck={false}
            placeholder={'-----BEGIN CERTIFICATE-----\n…\n-----END CERTIFICATE-----'}
            aria-describedby="saml-cert-hint"
            invalid={Boolean(errCert)}
            onInput={(e) => poner('certPem', e.currentTarget.value)}
            onBlur={() => tocar('certPem')}
          />
          <FieldHint id="saml-cert-hint" tone={errCert ? 'danger' : 'neutral'}>
            {errCert ?? t('wizard.saml.identity.certHint')}
          </FieldHint>
        </FieldGroup>

        <details class="border-border-subtle border-t pt-3" open={borrador.idpInitiated}>
          <summary class="text-text-muted hover:text-text sytue-focus-ring-inset cursor-pointer rounded text-sm font-medium">
            {t('wizard.saml.identity.advanced')}
          </summary>
          <div class="mt-3 flex flex-col gap-3">
            <Checkbox
              label={t('wizard.saml.identity.idpInitiatedLabel')}
              checked={borrador.idpInitiated}
              onChange={(e) => poner('idpInitiated', e.currentTarget.checked)}
            />
            <FieldHint>{t('wizard.saml.identity.idpInitiatedHint')}</FieldHint>
            {borrador.idpInitiated ? (
              <FieldGroup label={t('wizard.saml.identity.relayStateLabel')} htmlFor="saml-relay">
                <Input
                  id="saml-relay"
                  value={borrador.relayState}
                  class="font-mono text-xs"
                  spellcheck={false}
                  aria-describedby="saml-relay-hint"
                  onInput={(e) => poner('relayState', e.currentTarget.value)}
                />
                <FieldHint id="saml-relay-hint">
                  {t('wizard.saml.identity.relayStateHint')}
                </FieldHint>
              </FieldGroup>
            ) : null}
          </div>
        </details>
      </div>
    ),
  };

  const atributos = SAML_ATTRS.map(
    (k) => `${t(ATTR_LABEL[k])} → ${(borrador.attrs[k] ?? '').trim() || k}`,
  ).join('\n');
  const resumen: ReviewItem[] = [
    {
      label: t('wizard.saml.summary.app'),
      value: borrador.name.trim() || '—',
      onEdit: () => ir('app'),
    },
    {
      label: t('wizard.saml.summary.entityId'),
      value: borrador.entityId.trim() || '—',
      mono: true,
      onEdit: () => ir('app'),
    },
    {
      label: t('wizard.saml.summary.acs'),
      value: borrador.acsUrl.trim() || '—',
      mono: true,
      onEdit: () => ir('app'),
    },
    {
      label: t('wizard.saml.summary.nameId'),
      value: t(NAMEID_LABEL[borrador.nameIdFormat as NameIdFormat] ?? 'wizard.saml.nameId.email'),
      onEdit: () => ir('identity'),
    },
    {
      label: t('wizard.saml.summary.attrs'),
      value: atributos,
      mono: true,
      onEdit: () => ir('identity'),
    },
    {
      label: t('wizard.saml.summary.cert'),
      value: t(
        borrador.certPem.trim() ? 'wizard.saml.summary.certYes' : 'wizard.saml.summary.certNo',
      ),
      onEdit: () => ir('identity'),
    },
    {
      label: t('wizard.saml.summary.idpInitiated'),
      value: t(borrador.idpInitiated ? 'wizard.saml.summary.yes' : 'wizard.saml.summary.no'),
      onEdit: () => ir('identity'),
    },
  ];

  const pasoRepaso: WizardStep = {
    id: 'review',
    title: t('wizard.saml.step.review'),
    heading: t('wizard.saml.review.heading'),
    description: t('wizard.saml.review.description'),
    nextLabel: t('wizard.saml.review.submit'),
    onNext: () => void enviar(),
    content: (
      <div class="flex flex-col gap-4">
        <DataFlow
          partner={borrador.name.trim()}
          incoming={[]}
          outgoing={['identity', 'directory']}
        />
        <ReviewSummary items={resumen} />
        <InlineHelp tone="info" title={t('wizard.saml.review.enabledTitle')}>
          {t('wizard.saml.review.enabledBody')}
        </InlineHelp>
      </div>
    ),
  };

  const pasoHecho: WizardStep = {
    id: 'done',
    title: t('wizard.saml.step.done'),
    heading: t('wizard.saml.done.heading', {
      name: creado?.name || t('wizard.saml.done.fallbackName'),
    }),
    done: true,
    content: creado ? (
      <div class="flex flex-col gap-4">
        <section class="border-border bg-surface-raised rounded-lg border p-4">
          <h3 class="text-text text-sm font-semibold">{t('wizard.saml.done.pasteTitle')}</h3>
          <p class="text-text-muted mt-1 text-sm leading-relaxed">
            {t('wizard.saml.done.pasteBody')}
          </p>
          <div class="mt-3 flex flex-col gap-2">
            <CopyField label={t('sso.saml.metadataUrl')} value={creado.idpMetadataUrl} />
            <CopyField label={t('sso.saml.entityId')} value={creado.idpEntityId} />
            <CopyField label={t('sso.saml.ssoUrl')} value={creado.idpSsoUrl} />
            <CopyField label={t('sso.saml.sloUrl')} value={creado.idpSloUrl} />
            {creado.idpInitiated ? (
              <CopyField label={t('sso.saml.initiateUrl')} value={creado.initiateUrl} />
            ) : null}
          </div>
        </section>
        <section class="border-border bg-surface-raised rounded-lg border p-4">
          <h3 class="text-text text-sm font-semibold">{t('wizard.saml.done.nextTitle')}</h3>
          <ol class="mt-3 flex flex-col gap-3">
            <NextStep n={1} title={t('wizard.saml.done.step1Title')}>
              {t('wizard.saml.done.step1Body')}
            </NextStep>
            <NextStep n={2} title={t('wizard.saml.done.step2Title')}>
              {t('wizard.saml.done.step2Body')}
            </NextStep>
          </ol>
        </section>
        <div class="border-border-subtle flex flex-wrap items-center gap-2 border-t pt-4">
          <Button variant="primary" onClick={onDone}>
            {t('wizard.saml.done.back')}
          </Button>
        </div>
      </div>
    ) : null,
  };

  const porId: Record<Paso, WizardStep> = {
    app: pasoApp,
    identity: pasoIdentidad,
    review: pasoRepaso,
    done: pasoHecho,
  };

  return (
    <Wizard
      steps={PASOS.map((id) => porId[id])}
      current={paso}
      onCurrentChange={setPaso}
      busy={busy}
      onCancel={onCancel}
    />
  );
}

class Rechazo extends Error {
  constructor(
    readonly status: number,
    readonly code: string | undefined,
  ) {
    super(String(status));
  }
}
