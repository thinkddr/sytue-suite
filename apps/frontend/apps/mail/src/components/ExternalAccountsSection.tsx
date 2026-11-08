/* SPDX-License-Identifier: Apache-2.0 */
import type { JSX } from 'preact';
import { useCallback, useEffect, useRef, useState } from 'preact/hooks';
import {
  servesMail,
  type ExternalMailAccount,
  type ExternalMailAccountSummary,
} from '@sytue/api-clients';
import { Button, Icon, SidebarItem, SidebarSection } from '@sytue/ui';
import type { MailMessageKey, T } from '../i18n/index.js';
import { externalMailClient } from '../client/externalMail.js';
import {
  CUENTA_SYTUE,
  type BuzonUnificado,
  type CarpetaSytue,
  type CuentaDeBuzones,
} from './buzones.js';
import { Glyph } from './glyphs.js';
import { mailboxGlyph, mailboxLabel } from './utils.js';
import { esMezclado, type RolMezclado, type VistaExterna } from './vistaExterna.js';

/** Whether this folder of this account is the one in the main panel. */
function inView(view: VistaExterna | null, instanceId: string, folderId: string): boolean {
  return view?.tipo === 'carpeta' && view.cuenta === instanceId && view.carpeta === folderId;
}

/**
 * What the sidebar must ask the summary for again when the dialog's list
 * changes: an account it does not have yet, or one that was reconnected or
 * reconfigured (connected or address changed). Only accounts with a mailbox,
 * the same ones the summary lists.
 */
function signature(item: Pick<ExternalMailAccount, 'instance_id' | 'connected' | 'address'>) {
  return `${item.instance_id}\n${String(item.connected)}\n${item.address}`;
}
function staleIn(
  summary: readonly ExternalMailAccountSummary[],
  items: readonly ExternalMailAccount[],
): boolean {
  const known = new Set(summary.map(signature));
  return items.some((item) => servesMail(item) && !known.has(signature(item)));
}

/**
 * The sidebar's mailboxes, from `GET /external-mail/summary`: each account with
 * its state and its folders, which is what the Apple Mail–style sidebar paints
 * (`arbolDeBuzones`). They used to load only when the dialog closed: back from
 * the provider's sign-in, or after connecting or disconnecting one, the sidebar
 * still showed the old list behind the dialog, and a failed load left the
 * section empty with no word of why.
 *
 * The dialog reports each list it loads (`update`): an account that went away
 * leaves at once, and one that is new or changed asks for the summary again,
 * unless the load in flight already brings it (the return from the provider).
 */
export function useExternalAccounts(): {
  summary: ExternalMailAccountSummary[];
  error: boolean;
  retry: () => void;
  /** The dialog's list, each time it loads. */
  update: (items: readonly ExternalMailAccount[]) => void;
  /**
   * Ask again: what the main panel read or moved in an external folder changes the
   * unread counts the sidebar paints, and nothing else would refresh them.
   */
  settle: () => void;
} {
  const [summary, setSummary] = useState<ExternalMailAccountSummary[]>([]);
  const [error, setError] = useState(false);
  const [attempt, setAttempt] = useState(0);
  // The summary painted, for `update` to compare without waiting for a render.
  const summaryRef = useRef<ExternalMailAccountSummary[]>([]);
  const put = (items: ExternalMailAccountSummary[]) => {
    summaryRef.current = items;
    setSummary(items);
  };
  const loadingRef = useRef(false);
  // The dialog's latest list while a load is in flight: when it lands, it is
  // asked again only if it still does not match that list.
  const pendingRef = useRef<readonly ExternalMailAccount[] | null>(null);
  useEffect(() => {
    const controller = new AbortController();
    loadingRef.current = true;
    setError(false);
    externalMailClient()
      .summary(controller.signal)
      .then((result) => {
        if (controller.signal.aborted) return;
        loadingRef.current = false;
        const pending = pendingRef.current;
        pendingRef.current = null;
        // An account the dialog removed while this load was out does not come back.
        const ids = pending && new Set(pending.map((item) => item.instance_id));
        put(ids ? result.items.filter((item) => ids.has(item.instance_id)) : result.items);
        if (pending && staleIn(result.items, pending)) setAttempt((n) => n + 1);
      })
      .catch(() => {
        if (controller.signal.aborted) return;
        loadingRef.current = false;
        setError(true);
      });
    return () => {
      controller.abort();
      loadingRef.current = false;
    };
  }, [attempt]);
  return {
    summary,
    error,
    retry: () => setAttempt((n) => n + 1),
    update: (items) => {
      const ids = new Set(items.map((item) => item.instance_id));
      put(summaryRef.current.filter((item) => ids.has(item.instance_id)));
      if (loadingRef.current) pendingRef.current = items;
      else if (staleIn(summaryRef.current, items)) setAttempt((n) => n + 1);
    },
    // Always, not only when no summary ever arrived: with the counts in the sidebar,
    // marking an account's inbox as read left «4» there until the page reloaded. A
    // load in flight is replaced (the effect aborts it).
    settle: () => setAttempt((n) => n + 1),
  };
}

/** «All Inboxes», «All Sent»…: the name of each unified mailbox, by its role. */
export const UNIFIED_KEYS = {
  inbox: 'nav.allInboxes',
  drafts: 'nav.allDrafts',
  sent: 'nav.allSent',
  archive: 'nav.allArchive',
  junk: 'nav.allJunk',
  trash: 'nav.allTrash',
} as const satisfies Record<BuzonUnificado['rol'], MailMessageKey>;

/** The branches flipped from their default, with `id` flipped too. */
function flipped(previous: ReadonlySet<string>, id: string): ReadonlySet<string> {
  const next = new Set(previous);
  if (!next.delete(id)) next.add(id);
  return next;
}

/**
 * Which branches are open. Each one starts as `byDefault` says and the person's
 * choice is kept while the page lives (an account that reloads does not fold back).
 * The third function opens one whatever it was: a branch whose folder is shown.
 */
function useBranches(
  byDefault: boolean,
): [(id: string) => boolean, (id: string) => void, (id: string) => void] {
  const [toggled, setToggled] = useState<ReadonlySet<string>>(() => new Set());
  const toggle = useCallback((id: string) => setToggled((previous) => flipped(previous, id)), []);
  const reveal = useCallback(
    (id: string) =>
      setToggled((previous) => (previous.has(id) !== byDefault ? previous : flipped(previous, id))),
    [byDefault],
  );
  return [(id) => toggled.has(id) !== byDefault, toggle, reveal];
}

/**
 * «Mailboxes», as at the top of Apple Mail's sidebar: «All Inboxes», «All Sent»…
 * of every account, Sytue included, with the sum of their unread messages. Only
 * when some external account brings a folder to join: with Sytue alone (no
 * accounts, or all of them asking to sign in again) it would be «Folders» twice.
 * Each one opens (the triangle of Apple Mail) onto the folder of each account.
 * «All Inboxes», «All Sent» and «All Drafts» are also mailboxes of their own: the
 * row opens that folder of every account mixed by date in the main panel, and its
 * triangle, apart, folds (`ROLES_MEZCLADOS`). Archive, Spam and Trash only fold:
 * emptying one or deleting for good belongs to a folder, not to a mix. Fyne's
 * `seccionBuzones` (apps/fyne/apps/mail/menu.go) is this same section.
 */
export function MailboxesSection({
  t,
  mailboxes,
  accounts,
  sytueFolders,
  sytueTitle,
  selectedMailbox,
  selectedExternal,
  onSelectSytue,
  onOpenFolder,
  onOpenUnified,
}: {
  t: T;
  /** `arbolDeBuzones(...).unificados`. */
  mailboxes: readonly BuzonUnificado[];
  /** `arbolDeBuzones(...).cuentas`, to name and mark each source. */
  accounts: readonly CuentaDeBuzones[];
  /** The Sytue folders, for the unread count of each Sytue source. */
  sytueFolders: readonly CarpetaSytue[];
  /** The name of the Sytue mailbox among the sources: its address. */
  sytueTitle: string;
  /** The Sytue folder on screen, marked where it shows up. */
  selectedMailbox: string | undefined;
  /** The external folder, or the unified mailbox, in the main panel. */
  selectedExternal: VistaExterna | null;
  onSelectSytue: (mailboxId: string) => void;
  onOpenFolder: (instanceId: string, folderId: string) => void;
  /** «All Inboxes», «All Sent» or «All Drafts» itself: that folder of every account, mixed by date. */
  onOpenUnified: (role: RolMezclado) => void;
}): JSX.Element | null {
  const [isOpen, toggle] = useBranches(false);
  if (!mailboxes.some((mailbox) => mailbox.fuentes.some((f) => f.cuenta !== CUENTA_SYTUE)))
    return null;
  return (
    <SidebarSection title={t('nav.mailboxes')}>
      {mailboxes.map((mailbox) => {
        const mixed = esMezclado(mailbox.rol) ? mailbox.rol : null;
        return (
          <SidebarItem
            key={mailbox.rol}
            label={t(UNIFIED_KEYS[mailbox.rol])}
            icon={<Glyph name={mailboxGlyph(mailbox.rol)} size={18} />}
            {...(mailbox.noLeidos ? { count: mailbox.noLeidos } : {})}
            expanded={isOpen(mailbox.rol)}
            {...(mixed
              ? {
                  active: selectedExternal?.tipo === 'unificado' && selectedExternal.rol === mixed,
                  onClick: () => onOpenUnified(mixed),
                  onToggle: () => toggle(mailbox.rol),
                  // Con el buzón: «Cuentas», a secas, no dice qué despliega.
                  toggleLabel: t('nav.accountsIn', { mailbox: t(UNIFIED_KEYS[mailbox.rol]) }),
                }
              : { onClick: () => toggle(mailbox.rol) })}
          >
            {mailbox.fuentes.map((source) => {
              if (source.cuenta === CUENTA_SYTUE) {
                // Its count too, or the sum on the branch does not add up below it.
                const unread = sytueFolders.find((f) => f.id === source.carpeta)?.unreadEmails;
                return (
                  <SidebarItem
                    key="sytue"
                    label={sytueTitle}
                    icon={<Icon name="mail" size={18} />}
                    {...(unread ? { count: unread } : {})}
                    active={selectedMailbox === source.carpeta}
                    // «Folders» already has the same folder as the current page.
                    ariaCurrent={false}
                    onClick={() => onSelectSytue(source.carpeta)}
                  />
                );
              }
              const account = accounts.find((item) => item.id === source.cuenta);
              const folder = account?.carpetas.find((item) => item.id === source.carpeta);
              if (!account) return null;
              return (
                <SidebarItem
                  key={account.id}
                  label={account.titulo}
                  icon={<Icon name={account.icloud ? 'cloud' : 'user'} size={18} />}
                  {...(folder?.unread ? { count: folder.unread } : {})}
                  active={inView(selectedExternal, account.id, source.carpeta)}
                  // «Accounts» below has the same folder as the current page.
                  ariaCurrent={false}
                  onClick={() => onOpenFolder(account.id, source.carpeta)}
                />
              );
            })}
          </SidebarItem>
        );
      })}
    </SidebarSection>
  );
}

/**
 * «External email accounts»: always there and above «Folders», with «Add email
 * account» as its last row. Each account is a branch that folds, as an account in
 * Apple Mail: its icon (the cloud for iCloud), its address and, folded, the unread
 * messages of its inbox; open, its folders with their translated names, glyph and
 * count. An account that lost its sign-in says «Sign in again» in place of its
 * folders, and one the provider did not answer offers to retry. Fyne's
 * `seccionExternas` (apps/fyne/apps/mail/menu.go) is this same section.
 */
export function ExternalAccountsSection({
  t,
  accounts,
  error,
  selectedExternal,
  onRetry,
  onOpen,
  onOpenFolder,
  onAdd,
}: {
  t: T;
  /** The accounts of `arbolDeBuzones`, in its order. */
  accounts: readonly CuentaDeBuzones[];
  error: boolean;
  /** The external folder in the main panel, marked as the current page. */
  selectedExternal: VistaExterna | null;
  onRetry: () => void;
  /** The account itself, in the dialog: to sign in again. */
  onOpen: (instanceId: string) => void;
  onOpenFolder: (instanceId: string, folderId: string) => void;
  onAdd: () => void;
}): JSX.Element {
  const [isOpen, toggle, reveal] = useBranches(true);
  // An account opened from the dialog —just connected, or «Open mailbox»— comes
  // without a folder until its inbox is found: its branch opens, so the inbox
  // that is about to be selected can be seen, also if it had been folded.
  const revealed =
    selectedExternal?.tipo === 'carpeta' && !selectedExternal.carpeta
      ? selectedExternal.cuenta
      : '';
  useEffect(() => {
    if (revealed) reveal(revealed);
  }, [revealed, reveal]);
  return (
    <SidebarSection title={t('externalMail.title')}>
      {error ? (
        <div class="flex flex-col items-start gap-3 px-2.5 py-3" role="alert">
          <p class="text-sidebar-fg flex items-start gap-2 text-sm">
            <Icon name="warning" size={16} class="mt-0.5 shrink-0" />
            {t('externalMail.accountsError')}
          </p>
          <Button
            size="sm"
            variant="secondary"
            // Sobre la tinta del menú, el aro del cromo: el azul se queda en 3,4:1.
            class="sytue-focus-ring-chrome"
            onClick={onRetry}
          >
            {t('common.retry')}
          </Button>
        </div>
      ) : null}
      {accounts.map((account) => {
        const open = isOpen(account.id);
        return (
          <SidebarItem
            key={account.id}
            label={account.titulo}
            // An iCloud mailbox: a cloud instead of the person, as in Fyne.
            icon={<Icon name={account.icloud ? 'cloud' : 'user'} />}
            // Open, the inbox below already says it.
            {...(!open && account.noLeidos ? { count: account.noLeidos } : {})}
            expanded={open}
            onClick={() => toggle(account.id)}
          >
            {account.estado === 'reconnect' ? (
              <SidebarItem
                label={t('externalMail.reconnectNeeded')}
                icon={<Icon name="warning" size={18} />}
                onClick={() => onOpen(account.id)}
              />
            ) : account.estado === 'unavailable' ? (
              <SidebarItem
                label={t('externalMail.accountUnavailable')}
                icon={<Icon name="rotate" size={18} />}
                onClick={onRetry}
              />
            ) : (
              account.carpetas.map((folder) => (
                <SidebarItem
                  key={folder.id}
                  label={mailboxLabel(t, folder.name, folder.role)}
                  icon={<Glyph name={mailboxGlyph(folder.role)} size={18} />}
                  {...(folder.unread ? { count: folder.unread } : {})}
                  active={inView(selectedExternal, account.id, folder.id)}
                  onClick={() => onOpenFolder(account.id, folder.id)}
                />
              ))
            )}
          </SidebarItem>
        );
      })}
      <SidebarItem label={t('externalMail.add')} icon={<Icon name="plus" />} onClick={onAdd} />
    </SidebarSection>
  );
}
