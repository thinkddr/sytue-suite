/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Grupo «Scripts» de la cinta, junto a «Macros» en la pestaña Vista (los tres tipos de
 * documento): un botón que abre el panel de scripts. El panel y la lógica viven en
 * dialogs/ScriptsDialog.tsx y en editor/scripts/; aquí solo el pegamento de cinta.
 */
import type { JSX } from 'preact';
import { useState } from 'preact/hooks';
import type { EditorContext } from '../types';
import { RibbonButton, RibbonGroup } from './primitives';
import { ScriptsDialog } from '../dialogs/ScriptsDialog';

export function ScriptsGroup({ ctx }: { ctx: EditorContext }): JSX.Element {
  const [abierto, setAbierto] = useState(false);
  return (
    <RibbonGroup label={ctx.t('group.scripts')}>
      <RibbonButton
        commandId="scriptsManage"
        icon="shield"
        label={ctx.t('scripts.open')}
        active={abierto}
        onAction={() => setAbierto(true)}
      />
      {abierto && <ScriptsDialog ctx={ctx} onClose={() => setAbierto(false)} />}
    </RibbonGroup>
  );
}
