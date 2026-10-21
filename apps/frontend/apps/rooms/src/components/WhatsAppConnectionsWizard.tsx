import type { JSX } from 'preact';
import { useEffect, useRef, useState } from 'preact/hooks';
import {
  Button,
  DataFlow,
  FieldGroup,
  Icon,
  InlineHelp,
  Input,
  ReviewSummary,
  Select,
  Spinner,
  Wizard,
  withBase,
} from '@sytue/ui';
import type { WizardStep } from '@sytue/ui';
import type { WhatsAppNumber, WhatsAppSignup, WhatsAppSignupResult } from '@sytue/api-clients';
import { openMetaSignup } from '../lib/metaWhatsAppSignup.js';
import type { T } from '../i18n/index.js';

const API = withBase(import.meta.env.BASE_URL ?? '/rooms', '/api/whatsapp-connection');

interface Props {
  teams: { id: string; name: string }[];
  footerTarget?: Element | null;
  onClose: () => void;
  onConnected: () => void;
  t: T;
}

export function WhatsAppConnectionsWizard({
  teams,
  footerTarget,
  onClose,
  onConnected,
  t,
}: Props): JSX.Element {
  const [available, setAvailable] = useState<boolean | null>(null);
  const [loading, setLoading] = useState(true);
  const [reload, setReload] = useState(0);
  const [teamId, setTeamId] = useState('');
  const [step, setStep] = useState(0);
  const [busy, setBusy] = useState(false);
  const [ready, setReady] = useState(false);
  const [error, setError] = useState<'failed' | 'cancelled' | null>(null);
  const [instanceId, setInstanceId] = useState('');
  const [numbers, setNumbers] = useState<WhatsAppNumber[]>([]);
  const [phoneId, setPhoneId] = useState('');
  const [pin, setPin] = useState('');
  const preparedRef = useRef<WhatsAppSignup | null>(null);
  const pendingRef = useRef<AbortController | null>(null);
  const wizardRef = useRef<HTMLDivElement>(null);
  const team = teams.find((item) => item.id === teamId);
  const number = numbers.find((item) => item.id === phoneId);
  const validPin = !number?.registration_required || /^\d{6}$/.test(pin);

  useEffect(() => {
    const controller = new AbortController();
    setLoading(true);
    setError(null);
    void fetch(API, { credentials: 'include', signal: controller.signal })
      .then(async (response) => {
        if (response.status === 403 || response.status === 503) {
          setAvailable(false);
          return;
        }
        if (!response.ok) throw new Error('unavailable');
        const result = (await response.json()) as { available: boolean };
        if (!controller.signal.aborted) setAvailable(result.available === true);
      })
      .catch(() => {
        if (!controller.signal.aborted) setError('failed');
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoading(false);
      });
    return () => controller.abort();
  }, [reload]);

  useEffect(() => () => pendingRef.current?.abort(), []);
  useEffect(() => {
    if (!loading) wizardRef.current?.querySelector<HTMLElement>('h2')?.focus();
  }, [loading]);

  async function post<TResponse>(body: unknown, signal: AbortSignal): Promise<TResponse> {
    const response = await fetch(API, {
      method: 'POST',
      credentials: 'include',
      signal,
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify(body),
    });
    if (!response.ok) throw new Error('connection');
    return (await response.json()) as TResponse;
  }

  async function prepare(): Promise<void> {
    if (busy || !team) return;
    pendingRef.current?.abort();
    const controller = new AbortController();
    pendingRef.current = controller;
    setStep(1);
    setBusy(true);
    setReady(false);
    setError(null);
    preparedRef.current = null;
    try {
      const signup = await post<WhatsAppSignup>(
        { action: 'start', ...(instanceId ? { instance_id: instanceId } : {}) },
        controller.signal,
      );
      if (
        !/^\d+$/.test(signup.app_id) ||
        !/^\d+$/.test(signup.config_id) ||
        !/^v\d+\.\d+$/.test(signup.graph_version) ||
        !signup.state
      )
        throw new Error('configuration');
      if (controller.signal.aborted) return;
      preparedRef.current = signup;
      setInstanceId(signup.instance_id);
      setReady(true);
    } catch {
      if (!controller.signal.aborted) setError('failed');
    } finally {
      if (!controller.signal.aborted) setBusy(false);
    }
  }

  function authorize(): void {
    const prepared = preparedRef.current;
    if (busy || !ready || !prepared) return;
    const controller = new AbortController();
    pendingRef.current = controller;
    setBusy(true);
    setError(null);
    // Open the isolated consent window synchronously from this button's gesture.
    void openMetaSignup(
      prepared,
      {
        continue: t('appConnection.continue', { provider: 'Meta' }),
        loading: t('state.loading'),
        failed: t('appConnection.failed'),
        done: t('action.done'),
      },
      controller.signal,
    )
      .then((consent) =>
        post<WhatsAppSignupResult>({ action: 'finish', ...consent }, controller.signal),
      )
      .then((result) => {
        if (controller.signal.aborted) return;
        setInstanceId(result.instance_id);
        setNumbers(result.numbers);
        setPhoneId(result.numbers.length === 1 ? result.numbers[0]!.id : '');
        setPin('');
        setStep(2);
      })
      .catch((failure: unknown) => {
        if (!controller.signal.aborted)
          setError(
            failure instanceof Error && failure.message === 'cancelled' ? 'cancelled' : 'failed',
          );
      })
      .finally(() => {
        if (!controller.signal.aborted) {
          setBusy(false);
          setReady(false);
          preparedRef.current = null;
        }
      });
  }

  async function connect(): Promise<void> {
    if (busy || !team || !number || !validPin || !instanceId) return;
    const controller = new AbortController();
    pendingRef.current = controller;
    setBusy(true);
    setError(null);
    try {
      const result = await post<{ account: { id: string } }>(
        {
          action: 'connect',
          team_id: team.id,
          instance_id: instanceId,
          phone_number_id: number.id,
          ...(number.registration_required ? { pin } : {}),
        },
        controller.signal,
      );
      if (!result.account?.id) throw new Error('not_connected');
      if (controller.signal.aborted) return;
      setPin('');
      setStep(4);
      onConnected();
    } catch {
      if (!controller.signal.aborted) setError('failed');
    } finally {
      if (!controller.signal.aborted) setBusy(false);
    }
  }

  if (loading)
    return (
      <p class="text-text-muted flex items-center justify-center gap-3 py-12" role="status">
        <Spinner size={22} />
        {t('state.loading')}
      </p>
    );
  if (available !== true)
    return (
      <InlineHelp
        tone="neutral"
        title={t(error ? 'appConnection.failed' : 'whatsappConnection.unavailable')}
      >
        <Button variant="secondary" onClick={() => setReload((value) => value + 1)}>
          {t('action.retry')}
        </Button>
      </InlineHelp>
    );
  if (!teams.length) return <InlineHelp tone="neutral" title={t('whatsappConnection.noTeams')} />;

  const steps: WizardStep[] = [
    {
      id: 'team',
      title: t('whatsappConnection.team'),
      description: t('whatsappConnection.teamBody'),
      canContinue: Boolean(team),
      onNext: prepare,
      content: (
        <FieldGroup label={t('integrations.whatsappTeam')} htmlFor="wa-signup-team" required>
          <Select
            id="wa-signup-team"
            value={teamId}
            onChange={(event) => setTeamId(event.currentTarget.value)}
          >
            <option value="">{t('whatsappConnection.team')}</option>
            {teams.map((item) => (
              <option key={item.id} value={item.id}>
                {item.name}
              </option>
            ))}
          </Select>
        </FieldGroup>
      ),
    },
    {
      id: 'meta',
      title: t('whatsappConnection.authorize'),
      description: t('whatsappConnection.authorizeBody'),
      nextLabel: ready ? t('appConnection.continue', { provider: 'Meta' }) : t('action.retry'),
      onNext: ready ? authorize : prepare,
      content: (
        <div class="border-border bg-bg-subtle flex items-center gap-3 rounded-lg border p-4">
          <Icon name="lock" size={24} class="text-text-muted" />
          <span class="text-text font-medium">WhatsApp Business</span>
          {busy ? <Spinner size={18} /> : null}
        </div>
      ),
    },
    {
      id: 'number',
      title: t('whatsappConnection.number'),
      description: t('whatsappConnection.numberBody'),
      canContinue: Boolean(number) && validPin,
      content: (
        <div class="flex flex-col gap-5">
          {numbers.length ? (
            <FieldGroup label={t('whatsappConnection.number')} htmlFor="wa-signup-number" required>
              <Select
                id="wa-signup-number"
                value={phoneId}
                onChange={(event) => {
                  setPhoneId(event.currentTarget.value);
                  setPin('');
                }}
              >
                <option value="">{t('whatsappConnection.number')}</option>
                {numbers.map((item) => (
                  <option key={item.id} value={item.id}>
                    {item.display_phone}
                    {item.verified_name ? ` · ${item.verified_name}` : ''}
                  </option>
                ))}
              </Select>
            </FieldGroup>
          ) : (
            <InlineHelp tone="neutral" title={t('whatsappConnection.noNumbers')}>
              <Button variant="secondary" onClick={() => void prepare()}>
                {t('appConnection.continue', { provider: 'Meta' })}
              </Button>
            </InlineHelp>
          )}
          {number?.registration_required ? (
            <FieldGroup label={t('whatsappConnection.pin')} htmlFor="wa-signup-pin" required>
              <Input
                id="wa-signup-pin"
                type="password"
                inputMode="numeric"
                autoComplete="new-password"
                maxLength={6}
                pattern="[0-9]{6}"
                value={pin}
                onInput={(event) => setPin(event.currentTarget.value)}
                aria-describedby="wa-signup-pin-hint"
              />
              <p id="wa-signup-pin-hint" class="text-text-muted text-sm">
                {t('whatsappConnection.pinHint')}
              </p>
            </FieldGroup>
          ) : null}
        </div>
      ),
    },
    {
      id: 'review',
      title: t('appConnection.review'),
      description: t('whatsappConnection.reviewBody', {
        number: number?.display_phone ?? '',
        team: team?.name ?? '',
      }),
      nextLabel: t('integrations.bridgeConnect'),
      canContinue: Boolean(team && number && validPin),
      onNext: connect,
      content: (
        <div class="flex flex-col gap-4">
          <ReviewSummary
            items={[
              { label: t('integrations.whatsappTeam'), value: team?.name ?? '' },
              { label: t('whatsappConnection.number'), value: number?.display_phone ?? '' },
            ]}
          />
          <DataFlow compact incoming={['identity']} outgoing={['messages']} />
          <p class="text-text-muted text-sm">{t('integrations.whatsappWindow')}</p>
        </div>
      ),
    },
    {
      id: 'done',
      title: t('whatsappConnection.done'),
      done: true,
      content: (
        <div class="flex flex-col items-start gap-4">
          <Icon name="check" size={32} class="text-success-text" />
          <p class="text-text text-sm">
            {t('whatsappConnection.doneBody', {
              number: number?.display_phone ?? '',
              team: team?.name ?? '',
            })}
          </p>
          <Button onClick={onClose}>{t('action.done')}</Button>
        </div>
      ),
    },
  ];
  return (
    <div ref={wizardRef} class="flex flex-col gap-4">
      {error ? (
        <p role="alert" class="text-danger-text text-sm">
          {t(error === 'cancelled' ? 'whatsappConnection.cancelled' : 'appConnection.failed')}
        </p>
      ) : null}
      <Wizard
        steps={steps}
        current={step}
        onCurrentChange={(value) => {
          setError(null);
          if (value === 1) void prepare();
          else setStep(value);
        }}
        busy={busy}
        onCancel={onClose}
        {...(footerTarget !== undefined ? { footerTarget } : {})}
      />
    </div>
  );
}
