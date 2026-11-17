/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 */
import type { JSX } from 'preact';
import { appForKey, Button, Icon } from '@sytue/ui';
import { useI18n } from '@sytue/i18n/preact';
import type { Interpretado } from '@sytue/api-clients';
import { fichasDe } from './interpretacion.js';

/**
 * La banda de «entendido como», con una ficha por filtro aplicado y la salida a la
 * búsqueda literal. La pintan el buscador de la barra y la paleta de la suite.
 *
 * ES OBLIGATORIA, no un adorno. Una búsqueda que ha acotado a un producto o a un rango
 * de fechas que nadie ha visto devuelve pocos resultados —o ninguno— sin explicar por
 * qué, y quien busca concluye que lo que busca no existe. Enseñarlo Y poder deshacerlo
 * en un clic es lo que convierte una deducción en una ayuda en vez de en una jugarreta.
 *
 * Sin fichas (la IA no acotó nada que no se vea ya en el campo) no se pinta.
 */
export function BandaInterpretacion({
  interpretado,
  onLiteral,
}: {
  interpretado: Interpretado | null;
  /** «Buscar las palabras exactas», para la consulta de ahora. */
  onLiteral: () => void;
}): JSX.Element | null {
  const { t } = useI18n();
  if (!interpretado) return null;
  const fichas = fichasDe(
    interpretado,
    (k) => appForKey(k)?.name.replace('Sytue ', '') ?? k,
    (kind) => t(`search.kind.${kind}` as Parameters<typeof t>[0]),
    (ms) => new Date(ms).toLocaleDateString(undefined, { day: 'numeric', month: 'short' }),
    {
      fuente: (nombre) => t('search.ai.source', { name: nombre }),
      tipo: (kind) => t('search.ai.kind', { kind }),
      rango: (desde, hasta) => t('search.ai.range', { from: desde, to: hasta }),
      desde: (d) => t('search.ai.since', { from: d }),
      hasta: (h) => t('search.ai.until', { to: h }),
    },
  );
  if (fichas.length === 0) return null;
  return (
    <div
      role="status"
      class="border-border flex flex-wrap items-center gap-1.5 border-b px-3 py-2 text-xs"
    >
      {/* Sin `label`: es decorativo, el texto de al lado ya lo dice. */}
      <Icon name="ai" size={14} class="text-text-muted shrink-0" />
      <span class="text-text-muted">{t('search.ai.applied')}</span>
      {fichas.map((f) => (
        <span key={f.clave} class="bg-bg-subtle text-text rounded px-1.5 py-0.5">
          {f.texto}
        </span>
      ))}
      <Button size="sm" variant="ghost" class="ml-auto" onClick={onLiteral}>
        {t('search.ai.literal')}
      </Button>
    </div>
  );
}
