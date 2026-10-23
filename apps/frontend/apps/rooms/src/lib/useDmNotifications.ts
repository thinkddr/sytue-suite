/* SPDX-License-Identifier: Apache-2.0 */
import { useCallback, useEffect, useRef, useState } from 'preact/hooks';
import { registerLogoutCleanup } from '@sytue/auth/client';
import { useUltimo } from '@sytue/ui';
import { dmNotificationAPI } from '../client/dmNotifications.js';
import {
  DmNotificationPreference,
  LEGACY_DM_NOTIFY_KEY,
  type DmNotificationState,
  type NotificationOwner,
} from './dmNotifications.js';

function legacyChoice(): string | null {
  try {
    const value = localStorage.getItem(LEGACY_DM_NOTIFY_KEY);
    return value === '0' || value === '1' ? value : null;
  } catch {
    return null;
  }
}

export function useDmNotifications(owner: NotificationOwner) {
  const { id, tenant } = owner;
  const scope = JSON.stringify([id, tenant]);
  const [value, setValue] = useState<{ scope: string; state: DmNotificationState } | null>(null);
  const [legacy, setLegacy] = useState(legacyChoice);
  const controllerRef = useRef<DmNotificationPreference | null>(null);
  const liveScopeRef = useUltimo(scope);
  useEffect(() => {
    const instance = new DmNotificationPreference(dmNotificationAPI({ id, tenant }), (state) => {
      if (liveScopeRef.current === scope && controllerRef.current === instance)
        setValue({ scope, state });
    });
    controllerRef.current = instance;
    void instance.refresh();
    const refresh = () => {
      if (document.visibilityState === 'visible') void instance.refresh();
    };
    const storage = (event: StorageEvent) => {
      if (event.key === LEGACY_DM_NOTIFY_KEY || event.key === null) setLegacy(legacyChoice());
    };
    const unregisterLogout = registerLogoutCleanup(() => {
      instance.dispose();
      setValue(null);
      return Promise.resolve();
    });
    window.addEventListener('focus', refresh);
    window.addEventListener('pageshow', refresh);
    document.addEventListener('visibilitychange', refresh);
    window.addEventListener('storage', storage);
    return () => {
      instance.dispose();
      if (controllerRef.current === instance) controllerRef.current = null;
      unregisterLogout();
      window.removeEventListener('focus', refresh);
      window.removeEventListener('pageshow', refresh);
      document.removeEventListener('visibilitychange', refresh);
      window.removeEventListener('storage', storage);
    };
    // Primitive identity fields own the lifetime; a parent can recreate the object.
  }, [id, tenant, scope, liveScopeRef]);

  const refresh = useCallback(() => {
    void controllerRef.current?.refresh();
  }, []);
  const toggle = useCallback(() => {
    void controllerRef.current?.toggle();
  }, []);
  const discardLegacy = useCallback(() => {
    try {
      // Never import an unscoped preference into a server account. The user only
      // discards this browser's filter; a newly read server rule stays untouched.
      if (localStorage.getItem(LEGACY_DM_NOTIFY_KEY) === legacy)
        localStorage.removeItem(LEGACY_DM_NOTIFY_KEY);
      setLegacy(legacyChoice());
    } catch {
      /* Keep the old local silence until storage becomes writable. */
    }
  }, [legacy]);
  const state =
    value?.scope === scope
      ? value.state
      : { enabled: false, loading: true, saving: false, error: false };
  return {
    ...state,
    refresh,
    toggle,
    discardLegacy,
    hasLegacy: legacy !== null,
    browserEnabled:
      state.enabled && !state.loading && !state.saving && !state.error && legacy !== '0',
  };
}
