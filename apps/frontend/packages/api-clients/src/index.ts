/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * @sytue/api-clients — SDKs tipados a los SERVICIOS GO propios de Sytue One.
 *
 * Un SDK por servicio (id, drive, docs, mail, calendar, contacts, chat, meet,
 * hub, cloud), cada uno hablando el estandar/protocolo abierto del servicio Go
 * correspondiente (OIDC, WebDAV, JMAP, CalDAV, CardDAV, WebSocket, REST + LiveKit
 * para Meet). Autorizacion Bearer con el access token de Sytue ID (validado por
 * cada servicio via JWKS). Implementacion clean-room: SIN motores AGPL de
 * terceros (Keycloak/Nextcloud/Stalwart/Mattermost quedan fuera). Las superficies
 * Astro+Preact y @sytue/shell construyen sobre estos SDKs.
 */

// Nucleo
export {
  HttpClient,
  HttpError,
  ResponseTooLargeError,
  bearer,
  cuerpoDeError,
  upstreamStatus,
} from './core/http.js';
export type { AuthProvider, HttpClientOptions, RequestOptions } from './core/http.js';
export { parseXml, firstText, setXmlParser } from './core/xml.js';
export type { XmlParser } from './core/xml.js';

// Asistente de importacion: traerse uno sus propios datos de Google / Microsoft 365.
// Lo consume el BFF de cada producto (Mail, Drive, Calendar, Contacts); el token de
// sesion no sale del servidor.
export { listImportOrigins, startImport, listMyImports } from './core/importData.js';
export type { ImportDomain, ImportSource, ImportOrigin, ImportJob } from './core/importData.js';
export {
  STORAGE_PROVIDERS,
  isStorageOAuthKind,
  startStorageConsent,
} from './core/storageConsent.js';
export type { StorageOAuthKind } from './core/storageConsent.js';
export {
  ROOMS_PROVIDERS,
  isRoomsProvider,
  startAppConsent,
  finishAppConsent,
  listRoomsConnections,
  disconnectRoomsConnection,
} from './core/appConsent.js';
export type { ConsentProvider, RoomsProvider } from './core/appConsent.js';
export {
  whatsAppSignupStatus,
  startWhatsAppSignup,
  finishWhatsAppSignup,
  listWhatsAppNumbers,
} from './core/whatsAppConsent.js';
export type {
  WhatsAppSignup,
  WhatsAppSignupMetadata,
  WhatsAppNumber,
  WhatsAppSignupResult,
  WhatsAppSignupFinish,
} from './core/whatsAppConsent.js';

// Sytue ID (id.sytue.eu — OIDC/OAuth2/JWKS/SCIM/WebAuthn)
export { IdClient } from './id/index.js';
export type {
  IdOptions,
  OidcDiscovery,
  Jwks,
  UserInfo,
  OrgRole,
  OrgMembership,
  WebAuthnCredential,
} from './id/index.js';

// Sytue Drive (drive.sytue.eu — REST + WebDAV)
export { DriveClient, nodeResource } from './drive/index.js';
export type {
  DriveOptions,
  DriveResource,
  RawNode,
  ShareLink,
  CreateShareInput,
  TrashItem,
} from './drive/index.js';

// Sytue Docs (docs.sytue.eu — modelo documental + colab CRDT/OT WS)
export { DocsClient, puedeEscribir } from './docs/index.js';
export type {
  DocsOptions,
  DocKind,
  DocumentMeta,
  DocumentRole,
  CreateDocumentInput,
  DocTemplate,
  DocTemplateVersion,
  DocTemplatesResponse,
  DocTheme,
  DocBrandVersion,
  DocThemesResponse,
} from './docs/index.js';

// Sytue Notepads (notes.sytue.eu — REST propio; cuadernos, notas y referencias cruzadas)
export { NotesClient, noteWritePath } from './notes/index.js';
export type {
  NotesOptions,
  Notebook,
  NotebookInput,
  NotebookLayout,
  Note,
  NoteInput,
  NoteCipherMigration,
  NoteKind,
  NoteVisibility,
  NoteLink,
  NoteLinkTarget,
  NoteState,
  NoteHit,
  Hashtag,
  SharedNote,
  DailyDay,
  Task,
  TaskState,
  NotesCrypto,
  Reminder,
  ReminderInput,
  Tag,
  TagState,
} from './notes/index.js';

// Sytue Mail (mail.sytue.eu — JMAP)
export {
  MailClient,
  JmapClient,
  parseAttachments,
  parseDraftContent,
  parseMessageBody,
} from './mail/index.js';
export type {
  MailOptions,
  JmapOptions,
  JmapSession,
  Mailbox,
  MailboxSetError,
  EmailSummary,
  EmailAddress,
  OutgoingEmail,
  OutgoingAttachment,
  ParsedAttachment,
  DraftContent,
  MessageBody,
} from './mail/index.js';

// Sytue Calendar (calendar.sytue.eu — REST + CalDAV)
export { CalendarClient, CalDavClient, parseVevent } from './calendar/index.js';
export type {
  CalendarOptions,
  CalendarInfo,
  EventInput,
  Event,
  CalDavOptions,
  Calendar,
  CalendarEvent,
} from './calendar/index.js';

// Sytue Contacts (contacts.sytue.eu — REST + CardDAV)
export { ContactsClient, CardDavClient, parseVcard } from './contacts/index.js';
export type {
  ContactsOptions,
  AddressBookInfo,
  ContactInput,
  ContactRecord,
  CardDavOptions,
  AddressBook,
  Contact,
} from './contacts/index.js';

// Sytue Chat (rooms.sytue.eu — REST + WS)
export { ChatClient } from './chat/index.js';
export type {
  ChatOptions,
  MatrixBootstrapMetadata,
  FreeGif,
  FreeGifPage,
  Channel,
  ChannelType,
  Message,
  SendMessageInput,
  ChatEvent,
} from './chat/index.js';

// Sytue Meet (rooms.sytue.eu — REST salas/senalizacion + LiveKit web SDK)
export { MeetClient } from './meet/index.js';
export type { MeetOptions, Room, CreateRoomInput, LiveKitConnection } from './meet/index.js';

// Sytue Hub (hub.sytue.eu — busqueda federada, notificaciones, presencia, intents)
export { HubClient } from './hub/index.js';
export type {
  HubOptions,
  SearchResult,
  SearchResponse,
  Interpretado,
  Presence,
  PresenceState,
  Notification,
  NewNotification,
  NotificationPref,
  Intent,
} from './hub/index.js';

// Sytue DevX (cloud.sytue.eu — PaaS control plane)
export { CloudClient } from './cloud/index.js';
export type {
  CloudOptions,
  Project,
  Deployment,
  DeploymentStatus,
  DeploymentEnv,
  ManagedService,
  ManagedServiceKind,
  EnvVar,
  EnvScope,
  CloudDomain,
  DomainDnsInstructions,
  AddDomainResult,
  CloudOrganization,
  UsageAggregate,
  UsageRecord,
  UptimeCheck,
  UptimeStatus,
  AlertPolicy,
  AlertEvent,
  InstanceSize,
  InstanceSizeCatalog,
  SlugLanguageOptions,
  SlugSuggestion,
  SlugCheck,
  TemplateApplied,
  SizingAdvice,
  SizingAction,
  SizingReason,
  CohortStat,
  CatalogApp,
  AppCatalog,
  RepoInspection,
} from './cloud/index.js';

// Sytue Bases (bases.sytue.eu — tablas relacionales, REST)
export { BasesClient } from './bases/index.js';
export type {
  BasesOptions,
  BasesRole,
  BasesFieldKind,
  BasesViewKind,
  BasesBase,
  BasesField,
  BasesTable,
  BasesValue,
  BasesRecord,
  BasesRecordPage,
  BasesFilter,
  BasesViewConfig,
  BasesView,
  BasesPermission,
  BasesAuditEntry,
  BasesNewField,
  BasesFormLink,
  BasesConnection,
} from './bases/index.js';

// Sytue Photos (photos.sytue.eu — REST)
export { PhotosClient } from './photos/index.js';
export {
  ExternalConversationsClient,
  EXTERNAL_CHAT_PROVIDERS,
  teamsMeetingAppURL,
} from './core/externalConversations.js';
export type {
  ExternalChatProvider,
  ExternalChatCapabilities,
  ExternalChatAccount,
  ExternalChatAccounts,
  ExternalMeetingAccount,
  ExternalMeetingResult,
  ExternalChatChannel,
  ExternalChatMessage,
  ExternalChatPage,
  ExternalChatMessages,
} from './core/externalConversations.js';
export type { PhotosOptions, Foto, PaginaFotos } from './photos/index.js';
export { ExternalMailClient, MAIL_PROVIDERS } from './core/externalMail.js';
export { LinkedServicesClient } from './core/linkedServices.js';
export type {
  LinkedService,
  LinkedServiceCapability,
  RemoteCollection,
  RemotePage,
  RemoteResource,
  RemoteEvent,
  RemoteEventPatch,
  RemoteContact,
  RemoteContactPatch,
  RemoteNote,
} from './core/linkedServices.js';
export type {
  MailProvider,
  ExternalMailAddress,
  ExternalMailServer,
  ExternalMailConfiguration,
  ExternalMailAccount,
  ExternalMailAccounts,
  ExternalMailFolder,
  ExternalMailAttachment,
  ExternalMailMessage,
  ExternalMailOutgoing,
  ExternalMailPage,
} from './core/externalMail.js';
