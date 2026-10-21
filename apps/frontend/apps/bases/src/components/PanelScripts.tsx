/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Panel «Scripts» de una base (ADR 0069): lista, código, manifiesto de permisos,
 * versiones y su aprobación, ejecutar con la salida a la vista, la cuota de la hora y el
 * registro de ejecuciones. El código del usuario NO se ejecuta aquí: lo corre el sandbox
 * aislado común (`@sytue/scripts`) con lo que el servidor autoriza, y cada cosa que hace
 * es una petición corriente a la API con la sesión de quien lo lanza. Antes de la primera
 * ejecución —y tras cualquier cambio del manifiesto— se pide el consentimiento, con lo que
 * el manifiesto declara en lenguaje llano.
 *
 * «Ejecutar» corre la versión GUARDADA que está seleccionada: con cambios sin guardar se
 * apaga, para que lo que corre sea lo que se ve.
 */
import type { JSX } from 'preact';
import { useCallback, useEffect, useMemo, useState } from 'preact/hooks';
import { Badge, Button, FieldGroup, Input, Modal, Select, Textarea, useConfirm } from '@sytue/ui';
import { formatDate, type Locale } from '@sytue/i18n';
import type {
  BasesScript,
  BasesScriptQuota,
  BasesScriptRun,
  BasesScriptScope,
  BasesScriptVersion,
  BasesTable,
} from '@sytue/api-clients/bases';
import {
  cargarConsentimientos,
  consentir,
  guardarConsentimientos,
  haConsentido,
} from '@sytue/scripts';
import type { ResultadoScript } from '@sytue/scripts';
import type { T } from '../i18n/index.js';
import { cliente, mensajeDeError } from '../lib/api.js';
import { ejecutarVersion } from '../lib/scripts/cliente.js';
import { canonico, describirManifiesto, validarManifiesto } from '../lib/scripts/manifiesto.js';
import type { LineaConsentimiento } from '../lib/scripts/manifiesto.js';
import { crearSandboxIframe } from '../lib/scripts/sandbox.js';

const MANIFIESTO_INICIAL = JSON.stringify(
  { v: 1, ops: ['tables.list', 'records.list'], tables: [], write: false },
  null,
  2,
);
const CODIGO_INICIAL =
  'const tablas = await sytue.v1.tables.list();\nsytue.v1.log(tablas.map((t) => t.name).join(", "));\n';

const TONO: Record<BasesScriptVersion['status'], 'neutral' | 'success' | 'warning' | 'danger'> = {
  draft: 'neutral',
  pending: 'warning',
  approved: 'success',
  rejected: 'danger',
};

export function PanelScripts(p: {
  t: T;
  locale: Locale;
  baseId: string;
  /** `sub` de quien mira: distingue lo suyo y guarda su consentimiento. */
  sub: string;
  tablas: BasesTable[];
  /** Un script ha escrito en la base: quien abre el panel recarga lo que tenía a la vista. */
  onCambio: () => void;
  onCerrar: () => void;
}): JSX.Element {
  const { t, baseId } = p;
  const { confirmar, dialogo } = useConfirm();
  const [lista, setLista] = useState<BasesScript[]>([]);
  const [revisa, setRevisa] = useState(false);
  const [sel, setSel] = useState<BasesScript | null>(null);
  const [versiones, setVersiones] = useState<BasesScriptVersion[]>([]);
  const [verN, setVerN] = useState(0);
  const [nombre, setNombre] = useState('');
  const [ambito, setAmbito] = useState<BasesScriptScope>('private');
  const [codigo, setCodigo] = useState(CODIGO_INICIAL);
  const [manifiestoTxt, setManifiestoTxt] = useState(MANIFIESTO_INICIAL);
  const [salida, setSalida] = useState<string[] | null>(null);
  const [estado, setEstado] = useState('');
  const [error, setError] = useState('');
  const [ocupado, setOcupado] = useState(false);
  const [cuota, setCuota] = useState<BasesScriptQuota | null>(null);
  const [registro, setRegistro] = useState<BasesScriptRun[] | null>(null);
  const [sigRegistro, setSigRegistro] = useState(0);
  const [aConsentir, setAConsentir] = useState<{
    lineas: LineaConsentimiento[];
    decide: (ok: boolean) => void;
  } | null>(null);

  const manifiesto = useMemo(() => {
    try {
      return validarManifiesto(JSON.parse(manifiestoTxt));
    } catch {
      return { ok: false as const, error: 'json' };
    }
  }, [manifiestoTxt]);
  const version = versiones.find((v) => v.version === verN);
  const mio = sel === null || sel.author_sub === p.sub;
  /** Lo que hay en pantalla es exactamente la versión guardada seleccionada. */
  const guardado =
    version !== undefined &&
    version.code === codigo &&
    manifiesto.ok &&
    (() => {
      const g = validarManifiesto(version.manifest);
      return g.ok && canonico(g.manifiesto) === canonico(manifiesto.manifiesto);
    })();

  const refrescar = useCallback(async (): Promise<void> => {
    try {
      const [l, c] = await Promise.all([cliente().listScripts(baseId), cliente().scriptQuota()]);
      setLista(l.scripts);
      setRevisa(l.can_review);
      setCuota(c);
    } catch (e) {
      setError(mensajeDeError(e, t));
    }
  }, [baseId, t]);
  useEffect(() => {
    void refrescar();
  }, [refrescar]);

  const mostrar = (v: BasesScriptVersion | undefined): void => {
    setVerN(v?.version ?? 0);
    if (v?.code !== undefined) setCodigo(v.code);
    if (v) setManifiestoTxt(JSON.stringify(v.manifest, null, 2));
  };

  async function abrir(s: BasesScript, n?: number): Promise<void> {
    setError('');
    setSalida(null);
    setEstado('');
    try {
      const r = await cliente().getScript(baseId, s.id);
      setSel(r.script);
      setNombre(r.script.name);
      setAmbito(r.script.scope);
      setVersiones(r.versions);
      mostrar(r.versions.find((v) => v.version === n) ?? r.versions[0]);
    } catch (e) {
      setError(mensajeDeError(e, t));
    }
  }

  function nuevo(): void {
    setSel(null);
    setVersiones([]);
    setVerN(0);
    setNombre('');
    setAmbito('private');
    setCodigo(CODIGO_INICIAL);
    setManifiestoTxt(MANIFIESTO_INICIAL);
    setSalida(null);
    setEstado('');
    setError('');
  }

  /** Envuelve una acción: ocupado mientras dura, y el error traducido si falla. */
  async function hacer(fn: () => Promise<void>): Promise<void> {
    setOcupado(true);
    setError('');
    try {
      await fn();
    } catch (e) {
      setError(mensajeDeError(e, t));
    } finally {
      setOcupado(false);
    }
  }

  const guardar = (): Promise<void> =>
    hacer(async () => {
      if (!manifiesto.ok) return;
      const cuerpo = { code: codigo, manifest: manifiesto.manifiesto };
      if (sel) {
        const r = await cliente().addScriptVersion(baseId, sel.id, cuerpo);
        await abrir(sel, r.version.version);
      } else {
        const r = await cliente().createScript(baseId, {
          name: nombre.trim(),
          scope: ambito,
          ...cuerpo,
        });
        await abrir(r.script, 1);
      }
      await refrescar();
    });

  const cambiarAmbito = (a: BasesScriptScope): Promise<void> => {
    setAmbito(a);
    if (!sel) return Promise.resolve();
    return hacer(async () => {
      await cliente().setScriptScope(baseId, sel.id, a);
      await abrir(sel, verN);
      await refrescar();
    });
  };

  const revisar = (decision: 'approve' | 'reject'): Promise<void> =>
    hacer(async () => {
      if (!sel || !version) return;
      await cliente().reviewScriptVersion(
        baseId,
        sel.id,
        version.version,
        decision,
        version.code_hash,
      );
      await abrir(sel, version.version);
      await refrescar();
    });

  const proponer = (): Promise<void> =>
    hacer(async () => {
      if (!sel || !version) return;
      await cliente().proposeScriptVersion(baseId, sel.id, version.version);
      await abrir(sel, version.version);
    });

  async function borrar(): Promise<void> {
    if (!sel) return;
    if (
      !(await confirmar({
        mensaje: t('scripts.deleteConfirm', { name: sel.name }),
        confirmLabel: t('common.delete'),
      }))
    )
      return;
    await hacer(async () => {
      await cliente().deleteScript(baseId, sel.id);
      nuevo();
      await refrescar();
    });
  }

  /** Pregunta por el manifiesto si esta persona no lo ha aceptado ya para ESTE script. */
  const pedirConsentimiento =
    (scriptId: string) =>
    async (m: Parameters<typeof describirManifiesto>[0], hash: string): Promise<boolean> => {
      const dados = cargarConsentimientos('bases', p.sub);
      if (haConsentido(dados, scriptId, hash)) return true;
      const ok = await new Promise<boolean>((decide) =>
        setAConsentir({
          lineas: describirManifiesto(m, (id) => p.tablas.find((x) => x.id === id)?.name),
          decide,
        }),
      );
      setAConsentir(null);
      if (ok) guardarConsentimientos('bases', p.sub, consentir(dados, scriptId, hash));
      return ok;
    };

  const resumen = (r: ResultadoScript): string =>
    r.outcome === 'ok'
      ? t('scripts.result.ok')
      : t(`scripts.result.${r.outcome}`, { reason: r.message || r.errorCode });

  async function ejecutar(): Promise<void> {
    if (!sel || !version) return;
    setOcupado(true);
    setError('');
    setSalida(null);
    setEstado('');
    try {
      const intento = await ejecutarVersion({
        api: cliente(),
        baseId,
        scriptId: sel.id,
        version,
        consentir: pedirConsentimiento(sel.id),
        crearSandbox: () => crearSandboxIframe(),
      });
      if (intento.tipo === 'ejecutado') {
        setSalida(intento.resultado.output);
        setEstado(resumen(intento.resultado));
        // Con cualquier operación hecha puede haber escrito: se recarga lo que hay debajo.
        if (intento.resultado.ops > 0) p.onCambio();
      } else if (intento.motivo === 'sin_consentimiento') {
        setEstado(t('scripts.result.noConsent'));
      } else if (intento.status === 429) {
        setEstado(t('scripts.result.quota'));
      } else {
        setError(
          intento.status === 403
            ? t('err.forbidden')
            : intento.status === 404
              ? t('err.notFound')
              : t('err.generic'),
        );
      }
      await refrescar();
    } finally {
      setOcupado(false);
    }
  }

  const verRegistro = (antes = 0): Promise<void> =>
    hacer(async () => {
      const r = await cliente().listScriptRuns(baseId, antes > 0 ? { before: antes } : {});
      setRegistro(antes > 0 ? [...(registro ?? []), ...r.runs] : r.runs);
      setSigRegistro(r.next_before);
    });

  const puedeGuardar =
    mio && manifiesto.ok && codigo.trim() !== '' && (sel !== null || nombre.trim() !== '');

  return (
    <Modal open onClose={p.onCerrar} title={t('scripts.title')} size="xl">
      <div class="flex flex-col gap-4">
        <p class="text-text-muted text-sm">{t('scripts.help')}</p>
        <div class="flex flex-col gap-4 md:flex-row">
          <div class="flex flex-col gap-2 md:w-56 md:shrink-0">
            <Button variant="secondary" size="sm" onClick={nuevo}>
              {t('scripts.new')}
            </Button>
            {lista.length === 0 ? (
              <p class="text-text-muted text-sm">{t('scripts.list.empty')}</p>
            ) : (
              <ul class="flex flex-col gap-1" aria-label={t('scripts.list.label')}>
                {lista.map((s) => (
                  <li key={s.id}>
                    <Button
                      variant={sel?.id === s.id ? 'secondary' : 'ghost'}
                      size="sm"
                      class="w-full justify-start"
                      aria-current={sel?.id === s.id ? 'true' : undefined}
                      onClick={() => void abrir(s)}
                    >
                      <span class="min-w-0 truncate">{s.name}</span>
                    </Button>
                  </li>
                ))}
              </ul>
            )}
          </div>
          <div class="flex min-w-0 flex-1 flex-col gap-3">
            {/* El nombre con base de 12rem: con `flex-1` a secas encogía a 32 px junto a
                «Visibilidad» en el teléfono y su rótulo se montaba encima; así baja de fila. */}
            <div class="flex flex-wrap items-end gap-3">
              <FieldGroup
                label={t('scripts.name')}
                htmlFor="script-nombre"
                class="min-w-0 flex-[1_1_12rem]"
              >
                <Input
                  id="script-nombre"
                  value={nombre}
                  maxLength={120}
                  readOnly={sel !== null}
                  onInput={(e) => setNombre(e.currentTarget.value)}
                />
              </FieldGroup>
              <FieldGroup label={t('scripts.scope.label')} htmlFor="script-ambito">
                <Select
                  id="script-ambito"
                  value={ambito}
                  disabled={!mio || ocupado}
                  onChange={(e) => void cambiarAmbito(e.currentTarget.value as BasesScriptScope)}
                >
                  <option value="private">{t('scripts.scope.private')}</option>
                  <option value="shared">{t('scripts.scope.shared')}</option>
                </Select>
              </FieldGroup>
              {versiones.length > 0 ? (
                <FieldGroup label={t('scripts.versionLabel')} htmlFor="script-version">
                  <Select
                    id="script-version"
                    value={String(verN)}
                    onChange={(e) =>
                      mostrar(versiones.find((v) => v.version === Number(e.currentTarget.value)))
                    }
                  >
                    {versiones.map((v) => (
                      <option key={v.version} value={String(v.version)}>
                        {t('scripts.version', { n: v.version })} · {t(`scripts.status.${v.status}`)}
                      </option>
                    ))}
                  </Select>
                </FieldGroup>
              ) : null}
              {version ? (
                <Badge tone={TONO[version.status]}>{t(`scripts.status.${version.status}`)}</Badge>
              ) : null}
            </div>
            <FieldGroup label={t('scripts.code')} htmlFor="script-codigo">
              <Textarea
                id="script-codigo"
                class="font-mono text-xs"
                rows={10}
                spellcheck={false}
                wrap="off"
                maxLength={65536}
                readOnly={!mio}
                value={codigo}
                onInput={(e) => setCodigo(e.currentTarget.value)}
              />
            </FieldGroup>
            <FieldGroup
              label={t('scripts.manifest')}
              htmlFor="script-manifiesto"
              {...(manifiesto.ok ? {} : { error: t('scripts.manifest.invalid') })}
            >
              <Textarea
                id="script-manifiesto"
                class="font-mono text-xs"
                rows={7}
                spellcheck={false}
                readOnly={!mio}
                value={manifiestoTxt}
                onInput={(e) => setManifiestoTxt(e.currentTarget.value)}
              />
            </FieldGroup>
            <div class="flex flex-wrap gap-2">
              <Button disabled={ocupado || !guardado} onClick={() => void ejecutar()}>
                {t('scripts.run')}
              </Button>
              {mio ? (
                <Button
                  variant="secondary"
                  disabled={ocupado || !puedeGuardar || guardado}
                  onClick={() => void guardar()}
                >
                  {t('scripts.save')}
                </Button>
              ) : null}
              {mio &&
              sel?.scope === 'shared' &&
              guardado &&
              (version?.status === 'draft' || version?.status === 'rejected') ? (
                <Button variant="secondary" disabled={ocupado} onClick={() => void proponer()}>
                  {t('scripts.propose')}
                </Button>
              ) : null}
              {revisa && sel?.scope === 'shared' && version?.status === 'pending' ? (
                <>
                  <Button
                    variant="secondary"
                    disabled={ocupado}
                    onClick={() => void revisar('approve')}
                  >
                    {t('scripts.approve')}
                  </Button>
                  <Button variant="ghost" disabled={ocupado} onClick={() => void revisar('reject')}>
                    {t('scripts.reject')}
                  </Button>
                </>
              ) : null}
              {sel && (mio || (revisa && sel.scope === 'shared')) ? (
                <Button variant="ghost" disabled={ocupado} onClick={() => void borrar()}>
                  {t('scripts.delete')}
                </Button>
              ) : null}
            </div>
            <div role="status" class="flex flex-col gap-1 text-sm">
              {estado ? <p class="text-text">{estado}</p> : null}
              {salida ? (
                <pre
                  class="border-border bg-surface text-text max-h-40 overflow-auto rounded-sm border p-2 font-mono text-xs"
                  aria-label={t('scripts.output')}
                >
                  {salida.length > 0 ? salida.join('\n') : t('scripts.noOutput')}
                </pre>
              ) : null}
            </div>
            {error ? (
              <p role="alert" class="text-danger-text text-sm">
                {error}
              </p>
            ) : null}
            {cuota ? (
              <p class="text-text-muted text-xs">
                {t('scripts.quota', { runs: cuota.used.user_runs, max: cuota.limits.user_runs })}
              </p>
            ) : null}
            {revisa ? (
              <div class="flex flex-col gap-1">
                <div>
                  <Button variant="ghost" size="sm" onClick={() => void verRegistro()}>
                    {t('scripts.audit')}
                  </Button>
                </div>
                {registro ? (
                  <ul class="max-h-40 overflow-auto text-xs" aria-label={t('scripts.audit')}>
                    {registro.length === 0 ? (
                      <li class="text-text-muted">{t('scripts.audit.empty')}</li>
                    ) : null}
                    {registro.map((r) => (
                      <li key={r.id} class="border-border flex flex-wrap gap-x-2 border-b py-1">
                        <time dateTime={r.started_at}>{formatDate(p.locale, r.started_at)}</time>
                        <span>
                          {r.outcome}
                          {r.error_code ? ` (${r.error_code})` : ''}
                        </span>
                        <span class="text-text-muted">
                          {lista.find((x) => x.id === r.script_id)?.name ?? r.script_id.slice(0, 8)}{' '}
                          · {t('scripts.version', { n: r.version })} · {r.ops} ·{' '}
                          {r.duration_ms ?? 0} ms
                        </span>
                      </li>
                    ))}
                  </ul>
                ) : null}
                {registro && sigRegistro > 0 ? (
                  <div>
                    <Button variant="ghost" size="sm" onClick={() => void verRegistro(sigRegistro)}>
                      {t('grid.loadMore')}
                    </Button>
                  </div>
                ) : null}
              </div>
            ) : null}
          </div>
        </div>
      </div>
      {aConsentir ? (
        <Modal
          open
          onClose={() => aConsentir.decide(false)}
          title={t('scripts.consent.title')}
          size="sm"
          footer={
            <>
              <Button variant="ghost" onClick={() => aConsentir.decide(false)}>
                {t('common.cancel')}
              </Button>
              <Button onClick={() => aConsentir.decide(true)}>{t('scripts.consent.allow')}</Button>
            </>
          }
        >
          <ul class="text-text list-disc pl-4 text-sm">
            {aConsentir.lineas.map((l) => (
              <li key={l.clave}>{t(l.clave, l.params)}</li>
            ))}
          </ul>
          <p class="text-text-muted mt-2 text-xs">{t('scripts.consent.note')}</p>
        </Modal>
      ) : null}
      {dialogo}
    </Modal>
  );
}
