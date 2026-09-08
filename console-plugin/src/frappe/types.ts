import { K8sResourceCommon } from '@openshift-console/dynamic-plugin-sdk';

// ── Shared building blocks ───────────────────────────────────────────────────

export type NamespacedName = {
  name: string;
  namespace?: string;
};

/** spec.apps entry on a FrappeBench. Legacy benches store a bare string. */
export type AppSource = {
  name: string;
  source: 'fpm' | 'git' | 'image';
  version?: string;
  org?: string;
  gitUrl?: string;
  gitBranch?: string;
};

export type DatabaseProvider = 'mariadb' | 'postgres' | 'sqlite' | 'external';
export type DatabaseMode = 'shared' | 'dedicated';

export type DatabaseConfig = {
  provider?: DatabaseProvider;
  postgresEngine?: 'stackgres' | 'percona';
  mode?: DatabaseMode;
  /** Existing MariaDB CR to attach to instead of provisioning one. */
  mariadbRef?: NamespacedName;
  /** Existing PostgreSQL cluster to attach to instead of provisioning one. */
  postgresRef?: NamespacedName;
  /** Hostname of an out-of-cluster database. Required for the external provider. */
  host?: string;
  port?: string;
  /** Image used when the operator provisions a dedicated instance. */
  image?: string;
  storageSize?: string;
  /**
   * Secret holding `username`, `password` and optionally `database`. Required
   * for the external provider.
   */
  connectionSecretRef?: { name: string; namespace?: string };
  maxStatementTimeSeconds?: number;
};

export type RedisConfig = {
  type?: 'redis' | 'dragonfly';
  /** When true the operator provisions nothing and uses host/port below. */
  external?: boolean;
  host?: string;
  /** Integer in the CRD, unlike DatabaseConfig.port which is a string. */
  port?: number;
  connectionSecretRef?: { name: string; namespace?: string };
  image?: string;
  maxMemory?: string;
  storageSize?: string;
};

export type FPMRepository = {
  name: string;
  url?: string;
  priority?: number;
  authSecretRef?: { name: string };
};

// ── FrappeBench ──────────────────────────────────────────────────────────────

export type FrappeBench = K8sResourceCommon & {
  spec?: {
    frappeVersion?: string;
    apps?: (string | AppSource)[];
    storageSize?: string;
    storageClassName?: string;
    imageConfig?: { repository?: string; tag?: string; pullPolicy?: string };
    dbConfig?: DatabaseConfig;
    redisConfig?: RedisConfig;
    domainConfig?: { suffix?: string; autoDetect?: boolean };
    fpmConfig?: { repositories?: FPMRepository[]; defaultRepo?: string };
    gitConfig?: { enabled?: boolean };
  };
  status?: {
    phase?: string;
    installedApps?: string[];
    activeSites?: number;
    fpmRepositories?: string[];
    conditions?: { type: string; status: string; message?: string }[];
  };
};

// ── FrappeSite ───────────────────────────────────────────────────────────────

export type FrappeSite = K8sResourceCommon & {
  spec?: {
    benchRef?: NamespacedName;
    /** Legacy field superseded by benchRef. */
    benchName?: string;
    siteName?: string;
    domain?: string;
    apps?: string[];
    dbConfig?: DatabaseConfig;
    tls?: { enabled?: boolean; secretName?: string; issuer?: string };
    routeConfig?: {
      enabled?: boolean;
      host?: string;
      tlsTermination?: string;
      wildcardPolicy?: string;
    };
    adminPasswordSecretRef?: { name: string; namespace?: string };
    deletionPolicy?: 'Retain' | 'Delete';
    skipInit?: boolean;
  };
  status?: {
    phase?: string;
    siteURL?: string;
    databaseName?: string;
    databaseCredentialsSecret?: string;
    resolvedDomain?: string;
    installedApps?: string[];
    failedApps?: Record<string, string>;
    appInstallationStatus?: string;
    benchReady?: boolean;
    databaseReady?: boolean;
  };
};

// ── Site operations ──────────────────────────────────────────────────────────

export type SiteMigration = K8sResourceCommon & {
  spec?: {
    siteRef?: NamespacedName;
    skipFixtures?: boolean;
    force?: boolean;
    backupBeforeMigrate?: boolean;
  };
  status?: {
    phase?: string;
    jobName?: string;
    preBackupRef?: string;
    startTime?: string;
    completionTime?: string;
    conditions?: { type: string; status: string; message?: string }[];
  };
};

export type SiteBackup = K8sResourceCommon & {
  spec?: {
    site?: string;
    schedule?: string;
    withFiles?: boolean;
    compress?: boolean;
    storage?: { type?: 'pvc' | 's3'; s3?: Record<string, unknown> };
  };
  status?: {
    phase?: string;
    lastBackup?: string;
    lastBackupJob?: string;
    message?: string;
    storageLocation?: string;
  };
};

export type SiteRestore = K8sResourceCommon & {
  spec?: {
    site?: string;
    benchRef?: NamespacedName;
    databaseBackupSource?: { localPath?: string; s3?: Record<string, unknown> };
    publicFilesSource?: { localPath?: string };
    privateFilesSource?: { localPath?: string };
    force?: boolean;
  };
  status?: {
    phase?: string;
    restoreJob?: string;
    message?: string;
    completionTime?: string;
  };
};

export type SiteApp = K8sResourceCommon & {
  spec?: {
    siteRef?: NamespacedName;
    appName?: string;
    gitRepo?: string;
    gitBranch?: string;
    fpmPackage?: string;
    fpmRepo?: string;
    fpmRepoType?: 'http' | 'oci';
    autoMigrate?: boolean;
    backupBeforeInstall?: boolean;
  };
  status?: {
    phase?: string;
    installedVersion?: string;
    preBackupRef?: string;
  };
};
