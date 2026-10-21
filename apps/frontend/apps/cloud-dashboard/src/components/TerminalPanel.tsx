/**
 * TerminalPanel — consola real contra el `/bin/sh` del pod de producción.
 *
 * Cada comando viaja al BFF (POST /api/exec), que abre el WebSocket de exec del
 * control-plane con el Bearer de la sesión: ni el WebSocket del navegador puede
 * mandar cabeceras ni el adaptador SSR de Astro ve las peticiones `Upgrade`, así
 * que el WS lo abre SIEMPRE el servidor (ver el comentario de src/pages/api/exec.ts).
 *
 * Terminal de LÍNEA, no un emulador: entrada + salida acumulada monoespaciada,
 * historial con ↑/↓. Sin secuencias ANSI, sin TTY y sin programas interactivos —
 * y cada comando corre en una sesión nueva de shell (lo dice la propia UI), que es
 * lo que permite servir esto desde N réplicas del dashboard sin estado pegajoso.
 * Sin dependencias nuevas (nada de xterm.js): solo Preact + @sytue/ui.
 */
import { useEffect, useRef, useState } from 'preact/hooks';
import { Badge, Button, Card, CardBody, Icon, Input, SectionHeader, Spinner } from '@sytue/ui';
import type { Locale } from '@sytue/i18n';
import type { T } from '../i18n/index.js';
import { useT } from '../i18n/index.js';

/** Bloque de la transcripción: el comando escrito o la respuesta del contenedor. */
interface Entry {
  kind: 'command' | 'output' | 'error' | 'note';
  text: string;
}

/** Mensaje honesto para cada fallo REAL del control-plane (nunca «modo demostración»). */
function failureText(t: T, status: number, code: string): string {
  if (status === 401) return t('error.unauthorized');
  if (status === 403) return t('terminal.forbidden');
  if (status === 404) return t('terminal.notFound');
  if (status === 503) return t('terminal.unavailable');
  return t('error.unknown', { status, code });
}

export default function TerminalPanel({
  project,
  deployment,
  locale,
}: {
  project: string;
  /** Despliegue de producción sobre el que abrir el shell; ausente si no hay ninguno. */
  deployment?: { id: string; label: string } | undefined;
  /** Idioma ya negociado en el servidor (llega por props, nunca se detecta aquí). */
  locale: Locale;
}) {
  const t = useT(locale);
  const [entries, setEntries] = useState<Entry[]>([]);
  const [command, setCommand] = useState('');
  const [busy, setBusy] = useState(false);
  const [history, setHistory] = useState<string[]>([]);
  // Posición en el historial: -1 = escribiendo algo nuevo.
  const [histPos, setHistPos] = useState(-1);
  const outputRef = useRef<HTMLDivElement>(null);

  // Auto-scroll al final tras cada bloque nuevo (el foco se queda en la entrada).
  useEffect(() => {
    const el = outputRef.current;
    if (el) el.scrollTop = el.scrollHeight;
  }, [entries]);

  async function run() {
    const cmd = command.trim();
    if (!cmd || busy || !deployment) return;
    setEntries((prev) => [...prev, { kind: 'command', text: cmd }]);
    setHistory((prev) => (prev[prev.length - 1] === cmd ? prev : [...prev, cmd]));
    setHistPos(-1);
    setCommand('');
    setBusy(true);
    try {
      const res = await fetch('/api/exec', {
        method: 'POST',
        headers: { 'content-type': 'application/json' },
        body: JSON.stringify({ project, deployment: deployment.id, command: cmd }),
      });
      const body = (await res.json()) as {
        output?: string;
        truncated?: boolean;
        timedOut?: boolean;
        error?: string;
      };
      if (!res.ok) {
        setEntries((prev) => [
          ...prev,
          { kind: 'error', text: failureText(t, res.status, body.error ?? 'backend_error') },
        ]);
        return;
      }
      const out = body.output ?? '';
      setEntries((prev) => [
        ...prev,
        // Un comando sin salida (p. ej. `cd /app`) no es un error: se dice.
        { kind: 'output', text: out.trimEnd() || t('terminal.noOutput') },
        ...(body.truncated ? [{ kind: 'note' as const, text: t('terminal.truncated') }] : []),
        ...(body.timedOut ? [{ kind: 'note' as const, text: t('terminal.timedOut') }] : []),
      ]);
    } catch {
      setEntries((prev) => [...prev, { kind: 'error', text: t('terminal.netFail') }]);
    } finally {
      setBusy(false);
    }
  }

  /** ↑/↓ recorren el historial de comandos, como en cualquier shell. */
  function onKeyDown(e: KeyboardEvent) {
    if (e.key === 'Enter') {
      e.preventDefault();
      void run();
      return;
    }
    if (e.key !== 'ArrowUp' && e.key !== 'ArrowDown') return;
    if (history.length === 0) return;
    e.preventDefault();
    const next =
      e.key === 'ArrowUp'
        ? histPos < 0
          ? history.length - 1
          : Math.max(0, histPos - 1)
        : histPos < 0
          ? -1
          : histPos + 1 >= history.length
            ? -1
            : histPos + 1;
    setHistPos(next);
    setCommand(next < 0 ? '' : (history[next] ?? ''));
  }

  return (
    <>
      <SectionHeader
        level={1}
        icon={<Icon name="code" size={20} />}
        title={t('nav.terminal')}
        subtitle={
          deployment
            ? t('terminal.subtitle', { project, deployment: deployment.label })
            : t('terminal.subtitle.none')
        }
        actions={
          entries.length > 0 ? (
            <Button variant="secondary" size="sm" onClick={() => setEntries([])}>
              {t('common.clear')}
            </Button>
          ) : undefined
        }
      />
      <div class="cloud-content">
        {!deployment ? (
          <Card>
            <CardBody>
              <p class="text-text-muted text-sm">{t('terminal.noDeployment')}</p>
            </CardBody>
          </Card>
        ) : (
          <Card>
            <CardBody class="p-0">
              <div class="border-border flex flex-wrap items-center gap-2 border-b px-4 py-2.5">
                <Badge tone="warning">{t('env.scope.production')}</Badge>
                <p class="text-text-muted text-sm">{t('terminal.sessionNote')}</p>
              </div>

              {/* Transcripción: role="log" + aria-live para que un lector de pantalla
                  anuncie la salida sin robar el foco de la entrada. Sin tabIndex
                  explícito: los navegadores actuales ya hacen focusable un
                  contenedor con scroll, y añadirlo mete una parada de tabulación
                  extra entre el comando y su salida. */}
              <div
                ref={outputRef}
                role="log"
                aria-live="polite"
                aria-label={t('terminal.outputLabel')}
                class="h-[420px] overflow-y-auto bg-[var(--sytue-neutral-950)] p-3 font-mono text-sm leading-relaxed"
              >
                {entries.length === 0 ? (
                  <p class="text-white/50">{t('terminal.hint')}</p>
                ) : (
                  entries.map((e, i) => (
                    <pre
                      // Índice como clave: la transcripción solo crece por el final.
                      key={i}
                      class={
                        'break-words whitespace-pre-wrap ' +
                        (e.kind === 'command'
                          ? 'text-primary-300'
                          : e.kind === 'error'
                            ? 'font-sans text-[color-mix(in_srgb,var(--sytue-danger)_60%,white)]'
                            : e.kind === 'note'
                              ? 'font-sans text-[color-mix(in_srgb,var(--sytue-warning)_60%,white)]'
                              : 'text-white/85')
                      }
                    >
                      {e.kind === 'command' ? `$ ${e.text}` : e.text}
                    </pre>
                  ))
                )}
                {busy ? (
                  <p class="flex items-center gap-2 text-white/60">
                    <Spinner size={16} /> {t('terminal.running')}
                  </p>
                ) : null}
              </div>

              <div class="border-border flex items-center gap-2 border-t px-3 py-2.5">
                <span class="text-text-subtle font-mono text-sm" aria-hidden="true">
                  $
                </span>
                <Input
                  value={command}
                  disabled={busy}
                  spellcheck={false}
                  autocomplete="off"
                  autocapitalize="off"
                  aria-label={t('terminal.commandLabel')}
                  placeholder="ls -la /app"
                  class="flex-1 font-mono"
                  onInput={(e) => setCommand(e.currentTarget.value)}
                  onKeyDown={onKeyDown}
                />
                <Button
                  variant="primary"
                  loading={busy}
                  disabled={command.trim() === ''}
                  onClick={() => void run()}
                >
                  {t('common.run')}
                </Button>
              </div>
            </CardBody>
          </Card>
        )}
      </div>
    </>
  );
}
