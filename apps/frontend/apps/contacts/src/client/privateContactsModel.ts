/* SPDX-License-Identifier: Apache-2.0 */
import { isMigrationEpoch } from '@sytue/config/matrix';
import {
  parseVcardDetails,
  type ContactDetails,
  type PostalAddress,
  type TypedValue,
} from '../lib/vcard.js';

/** Go private Contacts projection; these fields are data, never network routes. */
export interface PrivateContactProjection {
  id: string;
  address_book_id: string;
  uid: string;
  etag: string;
  formatted_name: string;
  family_name?: string;
  given_name?: string;
  additional_name?: string;
  honorific_prefix?: string;
  honorific_suffix?: string;
  nickname?: string;
  organization?: string;
  title?: string;
  note?: string;
  birthday?: string | null;
  photo_url?: string;
  has_photo: boolean;
  emails?: Array<{ email: string; type?: string; is_primary?: boolean }> | null;
  phones?: Array<{ phone: string; type?: string; is_primary?: boolean }> | null;
  addresses?: Array<{
    po_box?: string;
    extended?: string;
    street?: string;
    locality?: string;
    region?: string;
    postal_code?: string;
    country?: string;
    type?: string;
    is_primary?: boolean;
  }> | null;
}

/** The future Go bridge must produce all three values from ONE authenticated
 * repository snapshot. Matching the revision here catches inconsistent framing;
 * it cannot prove that two independent reads were atomic. */
export interface PrivateContactSnapshot {
  contact: PrivateContactProjection;
  revision: string;
  vcard: string;
}

export interface PrivateBookProjection {
  id: string;
  name: string;
  display_name: string;
  is_default: boolean;
}

export type PrivateTypedValue = TypedValue & { primary: boolean };
export type PrivateAddress = PostalAddress & {
  poBox: string;
  extended: string;
  street: string;
  city: string;
  region: string;
  postalCode: string;
  country: string;
  primary: boolean;
};

export interface PrivateContactEdit {
  fullName?: string;
  firstName?: string;
  lastName?: string;
  emails?: readonly PrivateTypedValue[];
  phones?: readonly PrivateTypedValue[];
  /** Must preserve the structured fields not shown by the legacy editor. */
  addresses?: readonly PrivateAddress[];
  birthday?: string;
  organization?: string;
  title?: string;
  note?: string;
}

/** Explicit scalar changes only. A future Go operation must apply these changes
 * to the original vCard and projection atomically under ETag; this is not the
 * current full-replacement api.ContactInput wire format. List changes remain
 * disabled until stable per-item identities and unknown metadata are preserved. */
export interface PrivateContactChanges {
  formatted_name?: string;
  family_name?: string;
  given_name?: string;
  organization?: string;
  title?: string;
  note?: string;
  birthday?: string;
}

export interface PrivateContactDetail {
  id: string;
  bookId: string;
  uid: string;
  revision: string;
  fullName: string;
  emails: readonly string[];
  phones: readonly string[];
  addresses: readonly PrivateAddress[];
  organization: string;
  hasPhoto: boolean;
  /** Original content, needed by existing detail/editor and private export. */
  vcard: string;
  /** Original vCard fields may differ from the normalized projection. */
  vcardDetails: ContactDetails;
  /** Editor fields come from the normalized projection, not parsed vCard fields. */
  editor: {
    firstName: string;
    lastName: string;
    additionalName: string;
    honorificPrefix: string;
    honorificSuffix: string;
    nickname: string;
    title: string;
    note: string;
    birthday: string;
    emails: readonly PrivateTypedValue[];
    phones: readonly PrivateTypedValue[];
    addresses: readonly PrivateAddress[];
  };
  planUpdate: (edit: PrivateContactEdit) => {
    id: string;
    etag: string;
    changes: PrivateContactChanges;
  };
}

const invalid = (): never => {
  throw new Error('private_contacts_invalid_snapshot');
};
const value = (v: unknown): string => {
  if (v === undefined || v === null) return '';
  return typeof v === 'string' ? v : invalid();
};
const flag = (v: unknown): boolean => {
  if (v === undefined || v === null) return false;
  return typeof v === 'boolean' ? v : invalid();
};

/** No URL or synthetic href is created: identity stays a private repository ID. */
export function privateBook(book: PrivateBookProjection): {
  id: string;
  name: string;
  displayName: string;
  isDefault: boolean;
} {
  if (
    !isMigrationEpoch(book.id) ||
    typeof book.name !== 'string' ||
    typeof book.display_name !== 'string' ||
    typeof book.is_default !== 'boolean'
  )
    return invalid();
  return {
    id: book.id,
    name: book.name,
    displayName: book.display_name,
    isDefault: book.is_default,
  };
}

/** Snapshot bytes and returned strings can contain personal data. The owning
 * session must drop them and revoke its display resources on close/identity change. */
export function privateContact(snapshot: PrivateContactSnapshot): PrivateContactDetail {
  if (!snapshot || !snapshot.contact) return invalid();
  const c = snapshot.contact;
  if (
    !isMigrationEpoch(c.id) ||
    !isMigrationEpoch(c.address_book_id) ||
    !isMigrationEpoch(c.etag) ||
    snapshot.revision !== c.etag ||
    typeof c.uid !== 'string' ||
    !c.uid ||
    typeof c.formatted_name !== 'string' ||
    typeof c.has_photo !== 'boolean' ||
    typeof snapshot.vcard !== 'string' ||
    !snapshot.vcard
  )
    return invalid();
  const id = c.id,
    bookId = c.address_book_id,
    uid = c.uid,
    etag = c.etag;
  const details = parseVcardDetails(snapshot.vcard);
  const partialBirthday = !details.birthdayManaged;
  const projectedBirthday = c.birthday == null ? '' : value(c.birthday).slice(0, 10);
  if (projectedBirthday && !/^\d{4}-\d{2}-\d{2}$/.test(projectedBirthday)) return invalid();
  // The normalized birthday and the original vCard BDAY can legitimately
  // differ (including a partial BDAY). Retain both without inventing a year.
  if (c.emails != null && !Array.isArray(c.emails)) return invalid();
  if (c.phones != null && !Array.isArray(c.phones)) return invalid();
  if (c.addresses != null && !Array.isArray(c.addresses)) return invalid();
  const addresses: PrivateAddress[] = (c.addresses ?? []).map((item) => ({
    poBox: value(item.po_box),
    extended: value(item.extended),
    street: value(item.street),
    city: value(item.locality),
    region: value(item.region),
    postalCode: value(item.postal_code),
    country: value(item.country),
    type: value(item.type),
    primary: flag(item.is_primary),
  }));
  const editorEmails = (c.emails ?? []).map((item) => ({
    value: value(item.email),
    type: value(item.type),
    primary: flag(item.is_primary),
  }));
  const editorPhones = (c.phones ?? []).map((item) => ({
    value: value(item.phone),
    type: value(item.type),
    primary: flag(item.is_primary),
  }));
  const emails = editorEmails.map((item) => item.value);
  const phones = editorPhones.map((item) => item.value);
  return {
    id,
    bookId,
    uid,
    revision: etag,
    fullName: c.formatted_name,
    emails,
    phones,
    addresses,
    organization: value(c.organization),
    hasPhoto: c.has_photo,
    vcard: snapshot.vcard,
    vcardDetails: details,
    editor: {
      firstName: value(c.given_name),
      lastName: value(c.family_name),
      additionalName: value(c.additional_name),
      honorificPrefix: value(c.honorific_prefix),
      honorificSuffix: value(c.honorific_suffix),
      nickname: value(c.nickname),
      title: value(c.title),
      note: value(c.note),
      birthday: projectedBirthday,
      emails: editorEmails,
      phones: editorPhones,
      addresses,
    },
    planUpdate(edit) {
      // Current Go full-replacement encoder cannot remove a partial BDAY. A
      // future partial-update operation must opt in explicitly before enabling it.
      if (partialBirthday && edit.birthday === '') return invalid();
      const changes: PrivateContactChanges = {};
      if (edit.fullName !== undefined) changes.formatted_name = value(edit.fullName);
      if (edit.firstName !== undefined) changes.given_name = value(edit.firstName);
      if (edit.lastName !== undefined) changes.family_name = value(edit.lastName);
      if (edit.organization !== undefined) changes.organization = value(edit.organization);
      if (edit.title !== undefined) changes.title = value(edit.title);
      if (edit.note !== undefined) changes.note = value(edit.note);
      if (edit.birthday !== undefined) changes.birthday = value(edit.birthday);
      // Projections do not carry stable per-item identities or all vCard
      // parameters. Even equal-looking values can refer to different elements.
      if (edit.emails !== undefined || edit.phones !== undefined || edit.addresses !== undefined)
        throw new Error('private_contacts_item_identity_unavailable');
      return { id, etag, changes };
    },
  };
}

/** There is intentionally no photo fetch, import loop, retry, or object URL in
 * this pure adapter. UID alone is not unique across books. Until ID-bound photo
 * operations and durable caller receipts exist, the host must not use UID photo
 * methods or automatically retry a mutation with an uncertain result. */
