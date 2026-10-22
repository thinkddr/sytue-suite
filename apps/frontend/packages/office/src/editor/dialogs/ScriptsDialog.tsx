/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Panel «Scripts» (ADR 0069): lista de scripts, editor de código con numeración, el
 * manifiesto de permisos, versiones y su aprobación, ejecutar con la salida y los errores a
 * la vista, la cuota de la hora y el registro de ejecuciones. El código del usuario NO se
 * ejecuta aquí: lo corre el sandbox aislado (editor/scripts/sandbox.ts) con lo que el
 * servidor autoriza. Antes de la primera ejecución —y tras cualquier cambio del
 * manifiesto— se pide el consentimiento, con lo que el manifiesto declara en lenguaje llano.
 */
import type { JSX } from 'preact';
import { useCallback, useEffect, useMemo, useRef, useState } from 'preact/hooks';
import { Button, Modal, useToast } from '@sytue/ui';
import { fetchSession } from '@sytue/auth/client';
import { appDomains } from '@sytue/config';
import type { DocsT } from '../../i18n';
import type { EditorContext } from '../types';
import { api, ejecutarVersion } from '../scripts/cliente';
import type { Cuota, EjecucionAuditada, ScriptResumen, VersionScript } from '../scripts/cliente';
import {
  cargar,
  consentir,
  describirManifiesto,
  guardar,
  haConsentido,
} from '../scripts/consentimiento';
import type { LineaConsentimiento } from '../scripts/consentimiento';
import { anfitrionDeEditor } from '../scripts/editor';
import { validarManifiesto } from '../scripts/manifiesto';
import type { ManifiestoScript } from '../scripts/manifiesto';
import { crearSandboxIframe } from '../scripts/sandbox';
import type { ResultadoScript } from '../scripts/anfitrion';

const FIELD = 'border-border bg-surface text-text rounded-sm border px-1 text-xs sytue-focus-ring';
const AREA = `${FIELD} w-full resize-none font-mono leading-5`;

const MANIFIESTO_INICIAL: ManifiestoScript = {
  v: 1,
  commands: [],
  read: [],
  write: false,
  scope: {},
  max_ops: 1000,
  max_ms: 5000,
  unattended: false,
};
const CODIGO_INICIAL = "await sytue.v1.run('bold');\n";

/** Editor de texto con numeración de líneas (un textarea y una columna que scrollea con él). */
function EditorCodigo(p: {
  valor: string;
  onInput: (v: string) => void;
  etiqueta: string;
  filas: number;
}): JSX.Element {
  const gutter = useRef<HTMLPreElement>(null);
  const lineas = Math.max(1, p.valor.split('\n').length);
  const numeros = useMemo(
    () => Array.from({ length: lineas }, (_, i) => i + 1).join('\n'),
    [lineas],
  );
  return (
    <div class="flex gap-1">
      <pre
        ref={gutter}
        aria-hidden="true"
        class="text-text-muted w-8 overflow-hidden text-right font-mono text-xs leading-5 select-none"
        style={{ height: `${p.filas * 1.25}rem` }}
      >
        {numeros}
      </pre>
      <textarea
        class={AREA}
        aria-label={p.etiqueta}
        spellcheck={false}
        rows={p.filas}
        value={p.valor}
        wrap="off"
        onInput={(e) => p.onInput(e.currentTarget.value)}
        onScroll={(e) => {
          if (gutter.current) gutter.current.scrollTop = e.currentTarget.scrollTop;
        }}
      />
    </div>
  );
}

const textoLinea = (t: DocsT, l: LineaConsentimiento): string => t(l.clave, l.params);

/** Por qué terminó una ejecución, en la lengua de quien la lanzó. */
const resumenResultado = (t: DocsT, r: ResultadoScript): string =>
  r.outcome === 'ok'
    ? t('scripts.result.ok')
    : t(`scripts.result.${r.outcome}` as 'scripts.result.error', {
        reason: r.message || r.errorCode,
      });

export function ScriptsDialog({
  ctx,
  onClose,
}: {
  ctx: EditorContext;
  onClose: () => void;
}): JSX.Element {
  const t = ctx.t;
  const toast = useToast();
  const cliente = useMemo(() => api((u, i) => fetch(u, i)), []);
  const [sub, setSub] = useState<string | undefined>();
  const [lista, setLista] = useState<ScriptResumen[]>([]);
  const [puedeRevisar, setPuedeRevisar] = useState(false);
  const [sel, setSel] = useState<ScriptResumen | null>(null);
  const [versiones, setVersiones] = useState<VersionScript[]>([]);
  const [verN, setVerN] = useState(0);
  const [nombre, setNombre] = useState('');
  const [codigo, setCodigo] = useState(CODIGO_INICIAL);
  const [manifiestoTxt, setManifiestoTxt] = useState(JSON.stringify(MANIFIESTO_INICIAL, null, 2));
  const [manifiestoGuardado, setManifiestoGuardado] = useState('');
  const [salida, setSalida] = useState<string[]>([]);
  const [ocupado, setOcupado] = useState(false);
  const [cuota, setCuota] = useState<Cuota | null>(null);
  const [registro, setRegistro] = useState<EjecucionAuditada[] | null>(null);
  const [sigRegistro, setSigRegistro] = useState(0);
  const [errorCarga, setErrorCarga] = useState(false);
  const [aConsentir, setAConsentir] = useState<{
    lineas: LineaConsentimiento[];
    decide: (ok: boolean) => void;
  } | null>(null);

  const manifiesto = useMemo(() => {
    try {
      return validarManifiesto(JSON.parse(manifiestoTxt));
    } catch {
      return { ok: false as const, error: t('scripts.manifest.invalid') };
    }
  }, [manifiestoTxt, t]);

  const refrescar = useCallback(async (): Promise<void> => {
    try {
      const [l, c] = await Promise.all([cliente.listar(), cliente.cuota()]);
      setLista(l.scripts.filter((s) => s.kind === ctx.kind));
      setPuedeRevisar(l.can_review);
      setCuota(c);
      setErrorCarga(false);
    } catch {
      setErrorCarga(true);
    }
  }, [cliente, ctx.kind]);

  useEffect(() => {
    void fetchSession().then((u) => u && setSub(u.id));
    void refrescar();
  }, [refrescar]);

  const abrir = async (s: ScriptResumen, version?: number): Promise<void> => {
    const d = await cliente.ver(s.id).catch(() => null);
    if (!d) return;
    setSel(d.script);
    setVersiones(d.versions);
    const v = d.versions.find((x) => x.version === version) ?? d.versions[0];
    if (v) cargarVersion(d.script, v);
  };
  const cargarVersion = (s: ScriptResumen, v: VersionScript): void => {
    setNombre(s.name);
    setVerN(v.version);
    setCodigo(v.code ?? '');
    const m = validarManifiesto(v.manifest);
    const texto = JSON.stringify(m.ok ? m.manifiesto : v.manifest, null, 2);
    setManifiestoTxt(texto);
    setManifiestoGuardado(texto);
    setSalida([]);
  };
  const nuevo = (): void => {
    setSel(null);
    setVersiones([]);
    setVerN(0);
    setNombre('');
    setCodigo(CODIGO_INICIAL);
    setManifiestoTxt(JSON.stringify(MANIFIESTO_INICIAL, null, 2));
    setSalida([]);
  };

  const esAutor = !sel || sel.author_sub === sub;
  const version = versiones.find((v) => v.version === verN);
  // Se ejecuta la versión GUARDADA: con cambios sin guardar, «Ejecutar» se apaga para que
  // nadie crea que ha corrido lo que tiene en pantalla.
  const sucio =
    !!version && (codigo !== (version.code ?? '') || manifiestoTxt !== manifiestoGuardado);

  const guardarVersion = async (): Promise<void> => {
    if (!manifiesto.ok || !nombre.trim()) return;
    setOcupado(true);
    try {
      if (!sel) {
        const r = await cliente.crear({
          name: nombre.trim(),
          kind: ctx.kind,
          scope: 'private',
          code: codigo,
          manifest: manifiesto.manifiesto,
        });
        await refrescar();
        await abrir(r.script);
      } else {
        const r = await cliente.nuevaVersion(sel.id, {
          code: codigo,
          manifest: manifiesto.manifiesto,
        });
        await refrescar();
        await abrir(sel, r.version.version);
      }
    } catch {
      toast.notify({ tone: 'danger', title: t('scripts.err.save') });
    } finally {
      setOcupado(false);
    }
  };

  const ejecutar = async (): Promise<void> => {
    if (!sel || !version || ocupado) return;
    setOcupado(true);
    setSalida([]);
    try {
      const intento = await ejecutarVersion({
        fetch: (u, i) => fetch(u, i),
        sid: sel.id,
        version,
        documentId: ctx.doc.id,
        anfitrion: anfitrionDeEditor(ctx),
        entorno: {
          kind: ctx.kind,
          soloLectura: ctx.soloLectura,
          origenes: { propio: window.location.origin, drive: [new URL(appDomains.drive).origin] },
        },
        crearSandbox: () => crearSandboxIframe(document),
        consentir: async (m, hash) => {
          const previos = cargar(sub ?? '');
          if (sub && haConsentido(previos, sel.id, hash)) return true;
          const ok = await new Promise<boolean>((resolver) =>
            setAConsentir({ lineas: describirManifiesto(m), decide: resolver }),
          );
          setAConsentir(null);
          if (ok && sub) guardar(sub, consentir(previos, sel.id, hash));
          return ok;
        },
      });
      if (intento.tipo === 'rechazado') {
        const motivo =
          intento.motivo === 'sin_consentimiento'
            ? t('scripts.result.noConsent')
            : intento.code === 'quota_exceeded'
              ? t('scripts.result.quota')
              : t('scripts.result.denied', { reason: intento.code ?? intento.motivo });
        setSalida([motivo]);
      } else {
        const r = intento.resultado;
        setSalida([...r.output, resumenResultado(t, r)]);
      }
    } finally {
      setOcupado(false);
      void cliente
        .cuota()
        .then(setCuota)
        .catch(() => undefined);
    }
  };

  const accion = async (f: () => Promise<unknown>, falla = 'scripts.err.action'): Promise<void> => {
    try {
      await f();
      await refrescar();
      if (sel) await abrir(sel, verN);
    } catch {
      toast.notify({ tone: 'danger', title: t(falla as 'scripts.err.action') });
    }
  };

  // Paginado: `antes` = 0 abre el registro; después, cada «Más» pide la página siguiente.
  const verRegistro = async (antes = 0): Promise<void> => {
    try {
      const desde = antes > 0 ? antes : undefined;
      const r = puedeRevisar
        ? await cliente.registroOrg(desde)
        : await cliente.registroDoc(ctx.doc.id, desde);
      setRegistro(antes > 0 ? [...(registro ?? []), ...r.runs] : r.runs);
      setSigRegistro(r.next_before_id);
    } catch {
      setRegistro([]);
      toast.notify({ tone: 'warning', title: t('scripts.audit.denied') });
    }
  };

  return (
    <Modal
      open
      onClose={onClose}
      title={t('scripts.title')}
      size="xl"
      footer={
        <Button variant="ghost" onClick={onClose}>
          {t('action.close')}
        </Button>
      }
    >
      <div class="flex flex-col gap-3 sm:flex-row">
        <nav class="flex w-full flex-col gap-1 sm:w-48" aria-label={t('scripts.list.label')}>
          <Button onClick={nuevo}>{t('scripts.new')}</Button>
          {errorCarga && (
            <p class="text-danger text-xs" role="alert">
              {t('scripts.err.load')}
            </p>
          )}
          {lista.length === 0 && !errorCarga && (
            <p class="text-text-muted text-xs">{t('scripts.list.empty')}</p>
          )}
          {lista.map((s) => (
            <button
              key={s.id}
              type="button"
              class={`hover:bg-bg-subtle sytue-focus-ring rounded-sm px-2 py-1 text-left text-sm ${sel?.id === s.id ? 'bg-selected' : ''}`}
              aria-current={sel?.id === s.id}
              onClick={() => void abrir(s)}
            >
              <span class="block truncate">{s.name}</span>
              <span class="sytue-meta-dense">
                {t(s.scope === 'org' ? 'scripts.scope.org' : 'scripts.scope.private')}
              </span>
            </button>
          ))}
        </nav>

        <div class="flex min-w-0 flex-1 flex-col gap-2">
          <label class="text-text-muted flex items-center gap-2 text-xs">
            {t('scripts.name')}
            <input
              class={`${FIELD} h-6 flex-1`}
              value={nombre}
              maxLength={120}
              disabled={!!sel}
              onInput={(e) => setNombre(e.currentTarget.value)}
            />
          </label>
          {sel && (
            <div class="text-text-muted flex flex-wrap items-center gap-2 text-xs">
              <label class="flex items-center gap-1">
                {t('scripts.versionLabel')}
                <select
                  class={`${FIELD} h-6`}
                  value={verN}
                  onChange={(e) => {
                    const v = versiones.find((x) => x.version === Number(e.currentTarget.value));
                    if (v) cargarVersion(sel, v);
                  }}
                >
                  {versiones.map((v) => (
                    <option key={v.version} value={v.version}>
                      {t('scripts.version', { n: v.version })} ·{' '}
                      {t(`scripts.status.${v.status}` as 'scripts.status.draft')}
                    </option>
                  ))}
                </select>
              </label>
              {esAutor && (
                <label class="flex items-center gap-1">
                  {t('scripts.scope.label')}
                  <select
                    class={`${FIELD} h-6`}
                    value={sel.scope}
                    onChange={(e) =>
                      void accion(() =>
                        cliente.ambito(sel.id, e.currentTarget.value === 'org' ? 'org' : 'private'),
                      )
                    }
                  >
                    <option value="private">{t('scripts.scope.private')}</option>
                    <option value="org">{t('scripts.scope.org')}</option>
                  </select>
                </label>
              )}
            </div>
          )}
          <EditorCodigo
            valor={codigo}
            onInput={setCodigo}
            etiqueta={t('scripts.code')}
            filas={12}
          />
          <label class="text-text-muted text-xs" for="scripts-manifiesto">
            {t('scripts.manifest')}
          </label>
          <textarea
            id="scripts-manifiesto"
            class={AREA}
            rows={8}
            spellcheck={false}
            value={manifiestoTxt}
            aria-invalid={!manifiesto.ok}
            aria-describedby="scripts-manifiesto-error"
            onInput={(e) => setManifiestoTxt(e.currentTarget.value)}
          />
          <p
            id="scripts-manifiesto-error"
            class="text-danger text-xs"
            role={manifiesto.ok ? undefined : 'alert'}
          >
            {manifiesto.ok ? '' : manifiesto.error}
          </p>

          <div class="flex flex-wrap items-center gap-2">
            <Button
              onClick={() => void ejecutar()}
              disabled={
                ocupado ||
                !sel ||
                !version ||
                sucio ||
                (!!sel && sel.author_sub !== sub && version.status !== 'approved')
              }
            >
              {t('scripts.run')}
            </Button>
            {esAutor && (
              <Button
                variant="ghost"
                onClick={() => void guardarVersion()}
                disabled={ocupado || !manifiesto.ok || !nombre.trim()}
              >
                {t('scripts.save')}
              </Button>
            )}
            {sel &&
              esAutor &&
              sel.scope === 'org' &&
              version &&
              (version.status === 'draft' || version.status === 'rejected') && (
                <Button
                  variant="ghost"
                  onClick={() => void accion(() => cliente.proponer(sel.id, version.version))}
                >
                  {t('scripts.propose')}
                </Button>
              )}
            {sel && puedeRevisar && sel.scope === 'org' && version?.status === 'pending' && (
              <>
                <Button
                  variant="ghost"
                  onClick={() =>
                    void accion(
                      () => cliente.revisar(sel.id, version.version, 'approve', version.code_hash),
                      'scripts.err.review',
                    )
                  }
                >
                  {t('scripts.approve')}
                </Button>
                <Button
                  variant="ghost"
                  onClick={() =>
                    void accion(
                      () => cliente.revisar(sel.id, version.version, 'reject', version.code_hash),
                      'scripts.err.review',
                    )
                  }
                >
                  {t('scripts.reject')}
                </Button>
              </>
            )}
            {sel && (esAutor || (puedeRevisar && sel.scope === 'org')) && (
              <Button
                variant="ghost"
                onClick={() =>
                  void accion(async () => {
                    await cliente.borrar(sel.id);
                    nuevo();
                  })
                }
              >
                {t('scripts.delete')}
              </Button>
            )}
          </div>

          <section
            aria-label={t('scripts.output')}
            class="border-border bg-bg-subtle rounded-sm border p-2"
          >
            <h3 class="text-text-muted text-xs">{t('scripts.output')}</h3>
            <pre
              role="log"
              aria-live="polite"
              class="text-text max-h-40 overflow-auto font-mono text-xs whitespace-pre-wrap"
            >
              {salida.length === 0 ? t('scripts.noOutput') : salida.join('\n')}
            </pre>
          </section>

          {cuota && (
            <p class="text-text-muted text-xs">
              {t('scripts.quota', { runs: cuota.used.user_runs, max: cuota.limits.user_runs })}
            </p>
          )}
          <div>
            <Button variant="ghost" onClick={() => void verRegistro()}>
              {t('scripts.audit')}
            </Button>
            {registro && (
              <ul class="mt-1 max-h-40 overflow-auto text-xs" aria-label={t('scripts.audit')}>
                {registro.length === 0 && (
                  <li class="text-text-muted">{t('scripts.audit.empty')}</li>
                )}
                {registro.map((r) => (
                  <li key={r.id} class="border-border flex flex-wrap gap-x-2 border-b py-0.5">
                    <time dateTime={r.started_at}>
                      {new Date(r.started_at).toLocaleString(ctx.locale)}
                    </time>
                    <span>
                      {r.outcome}
                      {r.error_code ? ` (${r.error_code})` : ''}
                    </span>
                    <span class="text-text-muted">
                      {lista.find((x) => x.id === r.script_id)?.name ?? r.script_id.slice(0, 8)} · v
                      {r.version} · {r.sub} · {r.ops} · {r.duration_ms ?? 0} ms
                    </span>
                  </li>
                ))}
              </ul>
            )}
            {registro && sigRegistro > 0 && (
              <Button variant="ghost" onClick={() => void verRegistro(sigRegistro)}>
                {t('ribbon.more')}
              </Button>
            )}
          </div>
        </div>
      </div>

      {aConsentir && (
        <Modal
          open
          onClose={() => aConsentir.decide(false)}
          title={t('scripts.consent.title')}
          size="sm"
          footer={
            <>
              <Button variant="ghost" onClick={() => aConsentir.decide(false)}>
                {t('action.cancel')}
              </Button>
              <Button onClick={() => aConsentir.decide(true)}>{t('scripts.consent.allow')}</Button>
            </>
          }
        >
          <ul class="text-text list-disc pl-4 text-sm">
            {aConsentir.lineas.map((l) => (
              <li key={l.clave}>{textoLinea(t, l)}</li>
            ))}
          </ul>
          <p class="text-text-muted mt-2 text-xs">{t('scripts.consent.note')}</p>
        </Modal>
      )}
    </Modal>
  );
}
