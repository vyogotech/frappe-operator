import { K8sGroupVersionKind } from '@openshift-console/dynamic-plugin-sdk';
import { AppSource, FrappeBench, FrappeSite, NamespacedName } from './types';

// Mirrors PatternFly 6's Label colour union; `cyan` and `gold` were dropped in
// favour of `teal`, `orangered` and `yellow`.
export type LabelColor =
  | 'blue'
  | 'teal'
  | 'green'
  | 'orange'
  | 'orangered'
  | 'purple'
  | 'red'
  | 'grey'
  | 'yellow';

/**
 * Maps an operator phase onto a PatternFly label colour. Covers the phase
 * vocabularies of every Frappe CRD: FrappeBench/FrappeSite (Pending,
 * Provisioning, Ready, Failed), SiteMigration (Pending, BackingUp, Migrating,
 * Succeeded, Failed), SiteBackup (Pending, Scheduled, Running, Succeeded,
 * Failed) and SiteApp (Pending, BackingUp, Installing, Ready, Uninstalling,
 * Failed).
 */
export const phaseColor = (phase?: string): LabelColor => {
  switch (phase) {
    case 'Ready':
    case 'Succeeded':
      return 'green';
    case 'Failed':
      return 'red';
    case 'Provisioning':
    case 'Migrating':
    case 'Installing':
    case 'Running':
    case 'BackingUp':
      return 'blue';
    case 'Scheduled':
      return 'purple';
    case 'Uninstalling':
      return 'orange';
    case 'Pending':
      return 'orange';
    default:
      return 'grey';
  }
};

/** Phases past which an operation object no longer changes. */
export const isTerminalPhase = (phase?: string): boolean =>
  phase === 'Succeeded' || phase === 'Failed';

// ── Naming ───────────────────────────────────────────────────────────────────

/** Coerces arbitrary text into a RFC 1123 label usable as a resource name. */
export const toDNS1123 = (value: string, maxLength = 63): string =>
  value
    .toLowerCase()
    .replace(/[^a-z0-9-]+/g, '-')
    .replace(/^-+|-+$/g, '')
    .slice(0, maxLength)
    .replace(/-+$/g, '');

/**
 * Builds a unique, readable name for a one-shot operation resource, e.g.
 * `erp-acme-com-migrate-20260908-141530`. Operation CRs are immutable records
 * of a single run, so every trigger needs a fresh name.
 */
export const operationName = (base: string, suffix: string): string => {
  const now = new Date();
  const pad = (n: number) => String(n).padStart(2, '0');
  const stamp =
    `${now.getFullYear()}${pad(now.getMonth() + 1)}${pad(now.getDate())}` +
    `-${pad(now.getHours())}${pad(now.getMinutes())}${pad(now.getSeconds())}`;
  // Reserve room for "-<suffix>-<stamp>" inside the 63 character limit.
  const room = 63 - (suffix.length + stamp.length + 2);
  // A base that sanitises away entirely would leave a leading dash, which is
  // not a valid resource name.
  const prefix = toDNS1123(base, Math.max(room, 1)) || 'site';
  return `${prefix}-${suffix}-${stamp}`;
};

// ── Resource accessors ───────────────────────────────────────────────────────

/**
 * The Frappe site name — the domain Frappe routes on, and the value that
 * SiteBackup.spec.site / SiteRestore.spec.site are matched against by the
 * operator. This is NOT the FrappeSite CR name.
 */
export const siteNameOf = (site?: FrappeSite): string =>
  site?.spec?.siteName ?? site?.metadata?.name ?? '';

/** Normalises the bench reference, tolerating the legacy `benchName` field. */
export const benchRefOf = (site?: FrappeSite): NamespacedName | undefined => {
  const name = site?.spec?.benchRef?.name ?? site?.spec?.benchName;
  if (!name) {
    return undefined;
  }
  return {
    name,
    namespace: site?.spec?.benchRef?.namespace ?? site?.metadata?.namespace,
  };
};

/** Normalises `spec.apps`, which may hold bare strings on older benches. */
export const benchAppSources = (bench?: FrappeBench): AppSource[] =>
  (bench?.spec?.apps ?? []).map((app) =>
    typeof app === 'string' ? { name: app, source: 'image' as const } : app,
  );

/**
 * App names installable on a site: what the bench actually finished installing,
 * falling back to what its spec asked for while the bench is still building.
 */
export const benchAppNames = (bench?: FrappeBench): string[] => {
  const installed = bench?.status?.installedApps ?? [];
  return installed.length > 0 ? installed : benchAppSources(bench).map((a) => a.name);
};

/** True when the site is in a phase where operations against it will be accepted. */
export const isSiteReady = (site?: FrappeSite): boolean => site?.status?.phase === 'Ready';

// ── Console navigation ───────────────────────────────────────────────────────

export const resourcePath = (
  gvk: K8sGroupVersionKind,
  name: string,
  namespace?: string,
): string => {
  const ref = `${gvk.group}~${gvk.version}~${gvk.kind}`;
  return namespace
    ? `/k8s/ns/${namespace}/${ref}/${encodeURIComponent(name)}`
    : `/k8s/cluster/${ref}/${encodeURIComponent(name)}`;
};

export const createYAMLPath = (gvk: K8sGroupVersionKind, namespace?: string): string => {
  const ref = `${gvk.group}~${gvk.version}~${gvk.kind}`;
  return namespace ? `/k8s/ns/${namespace}/${ref}/~new` : `/k8s/all-namespaces/${ref}/~new`;
};

/**
 * Navigates the console to `path`. The plugin deliberately avoids importing
 * react-router: the console shares it as a non-fallback singleton and its major
 * version differs across OpenShift releases, so a hard navigation is the one
 * form that works everywhere.
 */
export const navigateTo = (path: string): void => {
  window.location.href = path;
};

/** Reads a query string parameter off the current console URL. */
export const queryParam = (key: string): string | undefined =>
  new URLSearchParams(window.location.search).get(key) ?? undefined;
