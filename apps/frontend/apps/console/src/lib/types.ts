/**
 * Modelos de administracion de Sytue Admin. Reflejan los recursos de la API de
 * administración de Sytue ID (identidad/SCIM) y de Sytue Hub: organizaciones
 * (tenants), usuarios, grupos y roles, más auditoría. docs/CONVENTIONS.md §8, §9, §11.
 */

export type SytueRole = 'owner' | 'admin' | 'member' | 'billing';

/** Estado de aprovisionamiento del usuario en los servicios Sytue (SCIM). */
export type ProvisionState = 'synced' | 'pending' | 'error';

/**
 * Servicio Go de la suite al que se aprovisiona al usuario (un alta en Sytue ID =
 * acceso a toda la suite): Sytue Drive, Sytue Chat y Sytue Mail.
 */
export type SytueService = 'drive' | 'chat' | 'mail';

export interface Tenant {
  /** Slug de la organización -> grupo Sytue ID org/<slug>. */
  slug: string;
  name: string;
  /** Dominio de correo asociado (Sytue Mail). */
  mailDomain: string;
  plan: 'starter' | 'business' | 'enterprise';
  userCount: number;
  /** Namespace Kapsule tenant-<slug>. */
  namespace: string;
  createdAt: string;
  status: 'active' | 'suspended';
  /**
   * Rol de QUIEN MIRA en esta organización, no el de nadie más. El claim `roles`
   * del token habla solo de la org activa, así que sin este campo la Console no
   * sabe si en una segunda organización es owner o admin — y esa diferencia
   * decide si puede ofrecer el rol owner al añadir a alguien. Fail-closed: si el
   * servicio no lo dice, se asume 'admin' (el permiso menor de los dos).
   */
  role: 'owner' | 'admin';
}

export interface DirectoryUser {
  id: string;
  username: string;
  email: string;
  fullName: string;
  enabled: boolean;
  emailVerified: boolean;
  /** Roles de la suite (Sytue ID). */
  roles: SytueRole[];
  /** Grupos de Sytue ID (org/<slug>). */
  groups: string[];
  tenant: string;
  /** MFA configurado (obligatorio para admin/owner). */
  mfaEnabled: boolean;
  lastLogin?: string;
  createdAt: string;
  /** Estado de aprovisionamiento por servicio Sytue. */
  provisioning: Record<SytueService, ProvisionState>;
  /** Atributos que evalúan las reglas de los grupos dinámicos ('' = sin fijar). */
  department: string;
  jobTitle: string;
  location: string;
}

export interface Group {
  /** Ruta del grupo en Sytue ID, p.ej. org/acme o org/acme/ingenieria. */
  path: string;
  name: string;
  memberCount: number;
  /** Roles asignados al grupo. */
  roles: SytueRole[];
  /** Mapeos del grupo a recursos de los servicios Sytue. */
  mappings: {
    chatWorkspace?: string;
    driveGroupFolder?: string;
    mailDomain?: string;
    namespace?: string;
  };
}

/**
 * Grupo gestionable de Sytue ID (directorio de grupos admin, contrato CRUD
 * /admin/groups). Distinto de {@link Group} (jerarquía org/<slug> con mapeos):
 * aquí son grupos con nombre/descripción y miembros arbitrarios, la base del
 * share por-grupo de Sytue Drive (claim `groups` del JWT).
 */
/** Operadores de una condición de regla de grupo dinámico (paridad Sytue ID). */
export type RuleOp = 'eq' | 'ne' | 'contains' | 'starts_with' | 'in';

/** Atributos de usuario sobre los que una regla puede condicionar. */
export type RuleAttribute = 'email' | 'name' | 'department' | 'title' | 'location';

/** Condición atómica «atributo op valor(es)». `values` solo para `in`. */
export interface RuleCondition {
  attribute: RuleAttribute;
  op: RuleOp;
  value?: string;
  values?: string[];
}

/** Regla de membresía de un grupo dinámico: condiciones combinadas AND ("all") u OR ("any"). */
export interface MembershipRule {
  match: 'all' | 'any';
  conditions: RuleCondition[];
}

export interface AdminGroup {
  id: string;
  name: string;
  description: string;
  memberCount: number;
  /** true ⇒ grupo dinámico: la membresía la calcula {@link membershipRule}, sin alta manual. */
  dynamic: boolean;
  /** Regla de membresía cuando {@link dynamic} es true. */
  membershipRule?: MembershipRule;
}

/** Miembro de un {@link AdminGroup} tal como lo expone Sytue ID. */
export interface AdminGroupMember {
  userId: string;
  email: string;
  name: string;
}

export interface AuditEvent {
  id: string;
  /** Origen del evento (Sytue ID / Hub / servicio Sytue). */
  source: 'id' | 'hub' | 'drive' | 'chat' | 'mail';
  type: string;
  actor: string;
  target?: string;
  tenant?: string;
  at: string;
  ok: boolean;
}

/**
 * Métrica del panel. `labelKey`/`hintKey` son CLAVES de catálogo, no texto: la
 * métrica se calcula en el servidor y se traduce al pintarla, que es lo único
 * que permite servirla en los cinco idiomas (docs/CONVENTIONS.md §13.1).
 */
export interface ConsoleMetric {
  labelKey: string;
  value: string;
  hintKey?: string;
}

/** Política de contraseñas de la organización (Sytue ID la guarda y aplica). */
export interface PasswordPolicy {
  min_length: number;
  require_complexity: boolean;
}

/**
 * Proveedor de identidad OIDC upstream (SSO / federación) de la organización, tal
 * como lo expone la API admin de Sytue ID. El client_secret NUNCA viaja al cliente
 * (`hasSecret` indica si hay uno configurado). `redirectUri` es el que el admin
 * debe registrar en su IdP corporativo. `configured` = discovery resuelto.
 */
export interface SsoProvider {
  id: string;
  /** 'oidc' = federación OIDC (broker/RP); 'saml' = federación SAML 2.0 (SP). */
  type: 'oidc' | 'saml';
  name: string;
  slug: string;
  hasSecret: boolean;
  attributeMapping: Record<string, string>;
  allowedEmailDomains: string[];
  enabled: boolean;
  configured: boolean;
  createdAt: string;
  updatedAt: string;
  // OIDC
  issuer: string;
  clientId: string;
  scopes: string;
  authorizationEndpoint: string;
  tokenEndpoint: string;
  userinfoEndpoint: string;
  jwksUri: string;
  pkce: boolean;
  redirectUri: string;
  // SAML: idpCertificate es PÚBLICO (cert de firma del IdP). acsUrl/metadataUrl los
  // registra el admin en su IdP; spEntityId es la audiencia efectiva.
  idpEntityId: string;
  idpSsoUrl: string;
  idpCertificate: string;
  spEntityId: string;
  samlBinding: 'redirect' | 'post';
  nameIdFormat: string;
  acsUrl: string;
  metadataUrl: string;
}

/** Acción de una política de Acceso Condicional (evaluada en el login por Sytue ID). */
export type AccessAction = 'allow' | 'require_mfa' | 'block';

/**
 * Política de Acceso Condicional (paridad Entra Conditional Access) de la organización,
 * tal como la expone la API admin de Sytue ID. Sus condiciones (a quién aplica, IP,
 * app, franja horaria) se combinan en AND; las exclusiones eximen a un usuario
 * (break-glass). El enforcement vive en el login del servicio, no en el cliente.
 */
export interface AccessPolicy {
  id: string;
  name: string;
  enabled: boolean;
  /** Orden de listado/evaluación (menor = antes). No cambia el resultado. */
  priority: number;
  action: AccessAction;
  /** A quién aplica (UUIDs). include vacío (ambos) = todos los usuarios. */
  includeUsers: string[];
  includeGroups: string[];
  /** Exentos break-glass (UUIDs): no sufren la política aunque la cumplan. */
  excludeUsers: string[];
  excludeGroups: string[];
  /** Rangos CIDR. Vacío = cualquier IP. */
  ipRanges: string[];
  /** true = la política aplica cuando la IP está FUERA de los rangos. */
  ipNegate: boolean;
  /** client_ids OAuth a los que aplica. Vacío = cualquier app. */
  clientIds: string[];
  /** true = la política aplica cuando el login NO viene de un dispositivo de confianza. */
  requireTrustedDevice: boolean;
  /** Días de la semana (bitmask, bit i = día i con Domingo=0). 0 = todos. */
  daysMask: number;
  /** Franja horaria en minutos-del-día [start,end); -1 = sin restricción horaria. */
  startMinute: number;
  endMinute: number;
  createdAt: string;
  updatedAt: string;
}

/** Payload de alta/edición de una política de Acceso Condicional (snake_case del BFF). */
export interface AccessPolicyInput {
  name: string;
  action: AccessAction;
  enabled?: boolean;
  priority?: number;
  include_users?: string[];
  include_groups?: string[];
  exclude_users?: string[];
  exclude_groups?: string[];
  ip_ranges?: string[];
  ip_negate?: boolean;
  client_ids?: string[];
  require_trusted_device?: boolean;
  days_mask?: number;
  start_minute?: number;
  end_minute?: number;
}

/**
 * Etiqueta de sensibilidad de la organización (paridad Purview / Google Drive labels).
 * La taxonomía vive en Sytue ID; los productos que etiquetan contenido (Drive) guardan
 * un SNAPSHOT sobre el propio contenido, así que cambiar una etiqueta aquí NO reescribe
 * lo ya etiquetado.
 */
export interface SensitivityLabel {
  id: string;
  name: string;
  /** Nivel de sensibilidad relativo (mayor = más sensible). Lo fija el admin. */
  rank: number;
  /** Color de presentación (#rrggbb). Vacío = neutro. */
  color: string;
  /** Para qué es, en lenguaje llano. Lo ve quien etiqueta, no solo el admin. */
  description: string;
  /** true = el contenido con esta etiqueta no admite enlaces públicos. */
  blockExternalShare: boolean;
  createdAt: string;
  updatedAt: string;
}

/** Payload de alta/edición de una etiqueta de sensibilidad (snake_case del BFF). */
export interface SensitivityLabelInput {
  name: string;
  rank?: number;
  color?: string;
  description?: string;
  block_external_share?: boolean;
}

/**
 * Dispositivo (navegador) registrado por Sytue ID al iniciar sesión (dispositivos de
 * confianza, paridad Entra device-based CA). `trusted` exime al login de una política
 * de Acceso Condicional con `require_trusted_device`. userEmail/userName solo llegan en
 * la vista admin (dueño del dispositivo).
 */
export interface Device {
  id: string;
  name: string;
  userAgent: string;
  trusted: boolean;
  firstSeen: string;
  lastSeen: string;
  lastIp: string;
  /** true si es el dispositivo desde el que se hace la petición (self-service). */
  current: boolean;
  userId?: string;
  userEmail?: string;
  userName?: string;
}

/**
 * Permiso GRANULAR del catálogo FIJO de Sytue ID (domain/roles.go). owner/admin son
 * superusuarios built-in y NO dependen de estos permisos; un rol personalizado concede
 * un subconjunto. `roles.manage` NO es delegable (reservado a owner/admin): nunca se
 * ofrece al definir un rol personalizado.
 */
export type PermissionKey =
  | 'users.manage'
  | 'users.read'
  | 'sso.manage'
  | 'apps.manage'
  | 'access_policies.manage'
  | 'groups.manage'
  | 'audit.view'
  | 'billing.manage'
  | 'roles.manage';

/**
 * Rol personalizado / admin delegada (paridad Entra roles / Workspace) de la
 * organización, tal como lo expone la API admin de Sytue ID. Concede sus `permissions`
 * a los usuarios asignados; el enforcement vive en el servicio (requirePermission).
 */
export interface CustomRole {
  id: string;
  name: string;
  permissions: string[];
  memberCount: number;
}

/** Usuario asignado a un {@link CustomRole} tal como lo expone Sytue ID. */
export interface CustomRoleMember {
  userId: string;
  email: string;
  name: string;
}

/** Payload de alta/edición de un rol personalizado (snake_case del BFF). */
export interface CustomRoleInput {
  name?: string;
  permissions?: string[];
}

/** Payload de alta/edición de un proveedor SSO (el secreto solo se envía al fijarlo). */
export interface SsoProviderInput {
  type?: 'oidc' | 'saml';
  name: string;
  slug: string;
  attribute_mapping?: Record<string, string>;
  allowed_email_domains?: string[];
  enabled?: boolean;
  // OIDC
  issuer?: string;
  client_id?: string;
  client_secret?: string;
  scopes?: string;
  // SAML: o metadata_xml (se parsea), o los tres campos sueltos.
  metadata_xml?: string;
  idp_entity_id?: string;
  idp_sso_url?: string;
  idp_certificate?: string;
  sp_entity_id?: string;
  saml_binding?: 'redirect' | 'post';
  name_id_format?: string;
}

/**
 * Aplicación de terceros que inicia sesión CONTRA Sytue (Sytue ID como proveedor OIDC):
 * un client OAuth2/OIDC registrado por la organización. El secreto solo se ve al crearlo
 * o al rotarlo (`secret`); `hasSecret` dice si la aplicación usa secreto. Los endpoints
 * son los que el admin pega en la aplicación de terceros.
 */
export interface OidcApp {
  clientId: string;
  name: string;
  /** 'public' = sin secreto (navegador/móvil, PKCE puro). */
  type: 'confidential' | 'public';
  redirectUris: string[];
  postLogoutRedirectUris: string[];
  scopes: string[];
  hasSecret: boolean;
  createdAt: string;
  /** Solo en el alta y en la rotación: no vuelve nunca más. */
  secret?: string;
  // Datos de conexión (los mismos para toda la organización).
  issuer: string;
  discoveryUrl: string;
  authorizationEndpoint: string;
  tokenEndpoint: string;
  userinfoEndpoint: string;
  jwksUri: string;
  endSessionEndpoint: string;
}

/**
 * Proveedor de servicio SAML registrado por la organización: una app de terceros a la
 * que Sytue ID hace de IdP. Lo crea el catálogo de plantillas (servicio `integrations`)
 * al activar una app, y desde la Console se ve, se apaga y se borra. Los campos `idp*`
 * son los datos que hay que pegar EN la app de terceros; son iguales para toda la org.
 */
export interface SamlSp {
  id: string;
  name: string;
  entityId: string;
  acsUrl: string;
  nameIdFormat: string;
  idpInitiated: boolean;
  enabled: boolean;
  /** false = le falta algún dato obligatorio y no puede federar todavía. */
  configured: boolean;
  createdAt: string;
  // Datos del IdP (Sytue ID) para dárselos al SP.
  idpEntityId: string;
  idpMetadataUrl: string;
  idpSsoUrl: string;
  idpSloUrl: string;
  /** URL que arranca el SSO iniciado por el IdP (solo útil si `idpInitiated`). */
  initiateUrl: string;
}

/** Payload de alta de una aplicación OIDC (snake_case del BFF). */
export interface OidcAppInput {
  name: string;
  redirect_uris: string[];
  post_logout_redirect_uris?: string[];
  scopes?: string[];
  public?: boolean;
}

/**
 * PIM (Privileged Identity Management, paridad Entra PIM). Una ELEGIBILIDAD declara para
 * qué rol puede elevar un usuario y con qué política (aprobación, ventana máxima); la
 * elevación real es una ACTIVACIÓN por una ventana acotada. El rol es personalizado
 * (roleId) o built-in (builtinRole: owner/admin); roleName es el nombre a mostrar.
 */
export interface PimEligibility {
  id: string;
  userId: string;
  userEmail: string;
  userName: string;
  roleId: string; // '' si built-in
  builtinRole: string; // '' si personalizado
  roleName: string;
  requiresApproval: boolean;
  maxDurationMinutes: number;
}

/**
 * Activación PIM (solicitud/elevación) tal como la expone Sytue ID. `status` es el de la
 * BD; `effectiveStatus` aplica la expiración perezosa (una 'active' caducada se ve
 * 'expired'). `remainingSeconds` solo viene si sigue vigente.
 */
export interface PimActivation {
  id: string;
  userId: string;
  userEmail: string;
  userName: string;
  roleId: string;
  builtinRole: string;
  roleName: string;
  status: string;
  effectiveStatus: string;
  justification: string;
  durationMinutes: number;
  requestedAt: string;
  activatedAt?: string;
  expiresAt?: string;
  remainingSeconds?: number;
  approvedByEmail?: string;
}

/** Payload de alta de una elegibilidad PIM (snake_case del BFF). */
export interface PimEligibilityInput {
  user_id: string;
  role_id?: string;
  builtin_role?: string;
  requires_approval: boolean;
  max_duration_minutes: number;
}

/** Payload de activación self-service (role_id XOR builtin_role). */
export interface PimActivateInput {
  role_id?: string;
  builtin_role?: string;
  justification: string;
  duration_minutes: number;
}
