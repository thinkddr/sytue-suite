/**
 * Lista de casillas de permisos, compartida por la edición del rol y su asistente de
 * alta. El Checkbox aporta su propia etiqueta accesible; el matiz va debajo, no anidado.
 */
import { Checkbox } from '@sytue/ui';
import type { T } from '../i18n/index.js';
import { PERMISSION_CATALOG } from '../lib/roles.js';

export function PermissionChecklist({
  selected,
  onToggle,
  t,
}: {
  selected: string[];
  onToggle: (key: string) => void;
  t: T;
}) {
  return (
    <fieldset class="border-border flex flex-col gap-2.5 rounded-md border p-3">
      <legend class="text-text-subtle sytue-overline px-1">{t('roles.permissions')}</legend>
      {PERMISSION_CATALOG.map((p) => (
        <div key={p.key}>
          <Checkbox
            label={t(p.label)}
            checked={selected.includes(p.key)}
            onChange={() => onToggle(p.key)}
          />
          <p class="sytue-meta ml-6">{t(p.hint)}</p>
        </div>
      ))}
      <p class="sytue-meta mt-1">{t('roles.notDelegable')}</p>
    </fieldset>
  );
}
