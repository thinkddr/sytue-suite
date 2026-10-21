/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Elegir con qué editor se abren los documentos de la organización.
 *
 * Esto vivía en una variable de entorno del servicio: conectar el Collabora de una
 * empresa obligaba a editar el Helm y reiniciar Drive, y en una instalación con varias
 * organizaciones todas compartían editor lo quisieran o no. Quién edita los documentos de
 * una empresa es decisión de esa empresa, y se toma aquí.
 *
 * Configuración guiada (docs/design.md §3): se pide UNA cosa —la dirección del editor—,
 * se comprueba antes de guardar y se enseña qué tipos de fichero abre. Nadie tiene que
 * saber qué es WOPI ni leer documentación para dejarlo funcionando.
 */
import type { JSX } from 'preact';
import { useEffect, useState } from 'preact/hooks';
import {
  SettingsShell,
  SettingsSection,
  Wizard,
  ChoiceCard,
  FieldHint,
  InlineHelp,
  ReviewSummary,
  EmptyState,
  Button,
  Input,
  Badge,
  Spinner,
  useToast,
} from '@sytue/ui';
import type { Locale } from '@sytue/i18n';
import { useT, type T } from '../i18n/index.js';

/** Editor conectado, tal como lo devuelve el servicio. */
interface Editor {
  discovery_url: string;
  label: string;
  enabled: boolean;
  check_ok: boolean;
  check_error: string;
  extensions: string[];
  checked_at?: string;
  updated_at: string;
}

/**
 * Los dos editores que la gente conecta en la práctica, con su dirección ya compuesta.
 *
 * Elegir de una lista y escribir solo el host evita el error más común: teclear la
 * dirección del editor en vez de la de su catálogo, que falla con un mensaje que no dice
 * qué se hizo mal. `manual` queda para cualquier otro.
 */
const KNOWN = [
  { id: 'collabora', suffix: '/hosting/discovery', example: 'https://office.tuempresa.com' },
  { id: 'onlyoffice', suffix: '/hosting/discovery', example: 'https://docs.tuempresa.com' },
] as const;

type Kind = (typeof KNOWN)[number]['id'] | 'manual';

export default function DocumentEditorPanel({ locale }: { locale: Locale }): JSX.Element {
  const t = useT(locale);
  const toast = useToast();
  const [editor, setEditor] = useState<Editor | null>(null);
  /*
   * Si ESTE despliegue puede acuñar tokens WOPI. Sin `WOPI_KEY`, Sytue Drive ni siquiera
   * monta el host: se podía conectar un editor, ver «editor conectado» y que nadie de la
   * organización viera jamás la acción, sin un solo error. `null` mientras carga —el aviso
   * solo se pinta cuando el servicio ha dicho que no—, para no parpadear una alarma falsa.
   */
  const [hostEnabled, setHostEnabled] = useState<boolean | null>(null);
  const [loading, setLoading] = useState(true);
  const [wizard, setWizard] = useState(false);
  const [confirmando, setConfirmando] = useState(false);
  const [desconectando, setDesconectando] = useState(false);

  useEffect(() => {
    void fetch('/api/document-editor', { credentials: 'include' })
      .then((r) => (r.ok ? r.json() : { editor: null, host_enabled: null }))
      .then((d: { editor: Editor | null; host_enabled?: boolean | null }) => {
        setEditor(d.editor);
        setHostEnabled(typeof d.host_enabled === 'boolean' ? d.host_enabled : null);
      })
      .catch(() => setEditor(null))
      .finally(() => setLoading(false));
  }, []);

  /*
   * La confirmación es del sistema de diseño, no un `confirm()` del navegador:
   * el nativo se pinta pegado a la barra de direcciones —lejos del botón que se
   * acaba de pulsar—, en móvil llega como una alerta del sistema que no se sabe
   * de dónde sale, y no admite ni el tipo de letra ni los tonos de la casa. Va
   * en línea, donde estaba el botón, que es donde se está mirando.
   */
  async function desconectar(): Promise<void> {
    setDesconectando(true);
    const res = await fetch('/api/document-editor', {
      method: 'POST',
      credentials: 'include',
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify({ remove: true }),
    });
    setDesconectando(false);
    if (res.ok) {
      setConfirmando(false);
      setEditor(null);
      toast.notify({ tone: 'success', title: t('editor.disconnected') });
    } else {
      toast.notify({ tone: 'danger', title: t('editor.disconnectFailed') });
    }
  }

  if (wizard) {
    return (
      <SettingsShell title={t('editor.connect')} description={t('editor.wizard.description')}>
        <ConnectEditorWizard
          t={t}
          hostEnabled={hostEnabled}
          onCancel={() => setWizard(false)}
          onConnected={(e) => {
            setEditor(e);
            setWizard(false);
          }}
        />
      </SettingsShell>
    );
  }

  return (
    <SettingsShell title={t('editor.title')} description={t('editor.description')}>
      <SettingsSection title={t('editor.current')}>
        {/*
          Antes que nada: si la instalación no puede acuñar tokens WOPI, conectar un editor
          no sirve de nada y hay que decirlo AQUÍ, no dejar que se descubra al abrir un
          .docx que nunca ofrece la acción. Es el mismo criterio que la casilla del alta de
          Sytue DevX: se explica el motivo en vez de esconderlo.
        */}
        {hostEnabled === false ? (
          <div class="mb-3">
            <InlineHelp tone="warning" title={t('editor.hostDisabled')}>
              {t('editor.hostDisabledHelp')}
            </InlineHelp>
          </div>
        ) : null}
        {loading ? (
          <div class="flex justify-center py-8" aria-busy="true">
            <Spinner size={22} />
          </div>
        ) : editor === null ? (
          // Sin editor no hay nada roto: los documentos de Sytue se editan en Sytue. Un
          // editor externo es para quien YA tiene uno y quiere seguir usándolo.
          <EmptyState
            title={t('editor.none')}
            description={t('editor.noneHelp')}
            action={
              <Button variant="primary" onClick={() => setWizard(true)}>
                {t('editor.connect')}
              </Button>
            }
          />
        ) : (
          <div class="flex flex-col gap-3">
            <div class="border-border bg-surface flex flex-wrap items-center justify-between gap-3 rounded-md border p-3">
              <div class="min-w-0">
                <p class="text-text flex items-center gap-2 text-sm font-medium">
                  {editor.label || t('editor.unnamed')}
                  {/* El estado va con TEXTO, nunca solo con color. */}
                  <Badge tone={editor.check_ok ? 'success' : 'warning'}>
                    {editor.check_ok ? t('editor.responding') : t('editor.notResponding')}
                  </Badge>
                  {!editor.enabled ? <Badge tone="neutral">{t('editor.paused')}</Badge> : null}
                </p>
                <p class="sytue-meta mt-0.5 truncate">{editor.discovery_url}</p>
              </div>
              {confirmando ? (
                <div class="flex shrink-0 flex-wrap items-center gap-2">
                  <p class="text-text-muted text-sm">{t('editor.disconnectConfirm')}</p>
                  <Button
                    size="sm"
                    variant="danger"
                    loading={desconectando}
                    onClick={() => void desconectar()}
                  >
                    {t('editor.disconnect')}
                  </Button>
                  <Button size="sm" variant="ghost" onClick={() => setConfirmando(false)}>
                    {t('common.cancel')}
                  </Button>
                </div>
              ) : (
                <div class="flex shrink-0 gap-2">
                  <Button size="sm" variant="secondary" onClick={() => setWizard(true)}>
                    {t('editor.change')}
                  </Button>
                  <Button
                    size="sm"
                    variant="ghost"
                    aria-expanded={confirmando}
                    onClick={() => setConfirmando(true)}
                  >
                    {t('editor.disconnect')}
                  </Button>
                </div>
              )}
            </div>

            {editor.check_ok ? (
              <FieldHint>
                {editor.extensions.length > 0
                  ? t('editor.opens', { list: editor.extensions.join(', ') })
                  : t('editor.opensNothing')}
              </FieldHint>
            ) : (
              // Lo que falla y por qué, con las palabras del editor. Un «error» a secas
              // deja a quien administra sin nada que mirar.
              <InlineHelp tone="warning" title={t('editor.notRespondingTitle')}>
                {editor.check_error || t('editor.notRespondingUnknown')}
              </InlineHelp>
            )}
          </div>
        )}
      </SettingsSection>
    </SettingsShell>
  );
}

/** Asistente de tres pasos: cuál, dónde y confirmar. */
function ConnectEditorWizard({
  t,
  hostEnabled,
  onCancel,
  onConnected,
}: {
  t: T;
  /** `false` ⇒ el despliegue no puede acuñar tokens WOPI (ver panel). */
  hostEnabled: boolean | null;
  onCancel: () => void;
  onConnected: (e: Editor) => void;
}): JSX.Element {
  const toast = useToast();
  const [step, setStep] = useState(0);
  const [kind, setKind] = useState<Kind>('collabora');
  const [host, setHost] = useState('');
  const [busy, setBusy] = useState(false);
  const [probe, setProbe] = useState<{
    ok: boolean;
    extensions?: string[];
    reason?: string;
  } | null>(null);

  const known = KNOWN.find((k) => k.id === kind);
  // La dirección completa se compone: quien administra escribe el host de su editor, que
  // es lo que sabe, y no una ruta que tendría que buscar en la documentación del producto.
  const discoveryUrl = known ? `${host.trim().replace(/\/+$/, '')}${known.suffix}` : host.trim();
  const urlValida = /^https?:\/\/[^\s/]+/.test(discoveryUrl);

  async function comprobar(): Promise<void> {
    setBusy(true);
    setProbe(null);
    try {
      const res = await fetch('/api/document-editor', {
        method: 'POST',
        credentials: 'include',
        headers: { 'content-type': 'application/json' },
        body: JSON.stringify({ check: true, discoveryUrl }),
      });
      setProbe((await res.json()) as { ok: boolean; extensions?: string[]; reason?: string });
      setStep(2);
    } catch {
      setProbe({ ok: false, reason: t('editor.checkFailed') });
      setStep(2);
    } finally {
      setBusy(false);
    }
  }

  async function guardar(): Promise<void> {
    setBusy(true);
    try {
      const res = await fetch('/api/document-editor', {
        method: 'POST',
        credentials: 'include',
        headers: { 'content-type': 'application/json' },
        body: JSON.stringify({
          discoveryUrl,
          label: known ? t(`editor.kind.${kind}`) : '',
        }),
      });
      if (!res.ok) throw new Error(String(res.status));
      const { editor } = (await res.json()) as { editor: Editor };
      // El editor SE HA GUARDADO —eso es verdad—, pero si la instalación no puede acuñar
      // tokens WOPI nadie verá la acción en Sytue Drive. Cantar «conectado» ahí era el
      // simulacro: se dice lo que pasa de verdad, y el aviso sigue en el panel.
      if (hostEnabled === false) {
        toast.notify({
          tone: 'warning',
          title: t('editor.hostDisabled'),
          description: t('editor.hostDisabledHelp'),
        });
      } else {
        toast.notify({ tone: 'success', title: t('editor.connected') });
      }
      onConnected(editor);
    } catch {
      toast.notify({ tone: 'danger', title: t('editor.saveFailed') });
    } finally {
      setBusy(false);
    }
  }

  return (
    <Wizard
      current={step}
      onCurrentChange={setStep}
      busy={busy}
      onCancel={onCancel}
      steps={[
        {
          id: 'cual',
          title: t('editor.step1'),
          heading: t('editor.step1.heading'),
          description: t('editor.step1.description'),
          content: (
            <div class="grid max-w-3xl gap-3 sm:grid-cols-2">
              {KNOWN.map((k) => (
                <ChoiceCard
                  key={k.id}
                  name="editor-kind"
                  value={k.id}
                  title={t(`editor.kind.${k.id}`)}
                  description={t(`editor.kind.${k.id}.desc`)}
                  selected={kind === k.id}
                  onSelect={() => setKind(k.id)}
                />
              ))}
              <ChoiceCard
                name="editor-kind"
                value="manual"
                title={t('editor.kind.manual')}
                description={t('editor.kind.manual.desc')}
                selected={kind === 'manual'}
                onSelect={() => setKind('manual')}
              />
            </div>
          ),
        },
        {
          id: 'donde',
          title: t('editor.step2'),
          heading: t('editor.step2.heading'),
          description: t('editor.step2.description'),
          canContinue: urlValida,
          nextLabel: t('editor.check'),
          onNext: () => void comprobar(),
          content: (
            <div class="flex max-w-2xl flex-col gap-2">
              <label class="sytue-label font-medium" for="editor-host">
                {known ? t('editor.hostLabel') : t('editor.urlLabel')}
              </label>
              <Input
                id="editor-host"
                type="url"
                value={host}
                placeholder={
                  known ? known.example : 'https://office.tuempresa.com/hosting/discovery'
                }
                onInput={(e) => setHost(e.currentTarget.value)}
              />
              {/* Se enseña la dirección que se va a usar, montada: así se ve el error de
                  un host mal escrito antes de comprobar nada. */}
              {urlValida ? (
                <FieldHint>{t('editor.willUse', { url: discoveryUrl })}</FieldHint>
              ) : (
                <FieldHint>{known ? t('editor.hostHint') : t('editor.urlHint')}</FieldHint>
              )}
            </div>
          ),
        },
        {
          id: 'confirmar',
          title: t('editor.step3'),
          heading: probe?.ok ? t('editor.step3.heading') : t('editor.step3.headingFailed'),
          description: probe?.ok
            ? t('editor.step3.description')
            : t('editor.step3.descriptionFailed'),
          nextLabel: t('editor.save'),
          canContinue: probe?.ok === true,
          onNext: () => void guardar(),
          content: probe?.ok ? (
            <ReviewSummary
              items={[
                {
                  label: t('editor.summary.kind'),
                  value: t(`editor.kind.${kind}`),
                },
                { label: t('editor.summary.url'), value: discoveryUrl, mono: true },
                {
                  label: t('editor.summary.opens'),
                  value: (probe.extensions ?? []).join(', ') || t('editor.opensNothing'),
                },
              ]}
            />
          ) : (
            // No se deja continuar: guardar un editor que no responde deja a la
            // organización creyendo que puede abrir documentos cuando no puede.
            <InlineHelp tone="danger" title={t('editor.checkFailedTitle')}>
              {probe?.reason || t('editor.checkFailed')}
            </InlineHelp>
          ),
        },
      ]}
    />
  );
}
