/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * ChannelHeader — la cabecera del canal de una organización: su marca (la misma
 * que en el resto de la suite), su nombre y cuántos vídeos públicos tiene.
 */
import type { JSX } from 'preact';
import { Button, Icon, OrgMark } from '@sytue/ui';
import type { T } from '../i18n/index.js';

export function ChannelHeader({
  t,
  org,
  nombre,
  cuantos,
  denunciar,
}: {
  t: T;
  org: string;
  nombre: string;
  cuantos: number | null;
  /** Abre la denuncia del canal. Cualquiera puede, también sin sesión. */
  denunciar: () => void;
}): JSX.Element {
  return (
    <header class="border-border flex flex-wrap items-center gap-4 border-b pb-6">
      <OrgMark slug={org} size={56} />
      <div class="min-w-0 flex-1 basis-40">
        <h1 class="sytue-h2 text-text break-words">{nombre}</h1>
        {cuantos !== null ? (
          <p class="text-text-muted text-sm">{t('channel.count', { count: cuantos })}</p>
        ) : null}
      </div>
      <Button
        variant="ghost"
        size="sm"
        leadingIcon={<Icon name="danger" size={16} />}
        onClick={denunciar}
      >
        {t('report.action')}
      </Button>
    </header>
  );
}
