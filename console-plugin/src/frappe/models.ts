import { K8sGroupVersionKind, K8sModel } from '@openshift-console/dynamic-plugin-sdk';

export const FRAPPE_GROUP = 'vyogo.tech';
export const FRAPPE_VERSION = 'v1';

const gvk = (kind: string): K8sGroupVersionKind => ({
  group: FRAPPE_GROUP,
  version: FRAPPE_VERSION,
  kind,
});

const model = (
  kind: string,
  plural: string,
  abbr: string,
  label: string,
  labelPlural: string,
): K8sModel => ({
  apiGroup: FRAPPE_GROUP,
  apiVersion: FRAPPE_VERSION,
  kind,
  plural,
  abbr,
  label,
  labelPlural,
  id: plural,
  namespaced: true,
  crd: true,
});

// ── Group / version / kind references (for useK8sWatchResource, ResourceLink) ──

export const FrappeBenchGVK = gvk('FrappeBench');
export const FrappeSiteGVK = gvk('FrappeSite');
export const SiteMigrationGVK = gvk('SiteMigration');
export const SiteBackupGVK = gvk('SiteBackup');
export const SiteRestoreGVK = gvk('SiteRestore');
export const SiteAppGVK = gvk('SiteApp');

// ── Models (for k8sCreate / k8sDelete) ───────────────────────────────────────
// Declared statically rather than resolved via useK8sModel so that write paths
// never have to wait on model discovery before the user can submit a form.

export const FrappeBenchModel = model('FrappeBench', 'frappebenches', 'FB', 'Frappe Bench', 'Frappe Benches');
export const FrappeSiteModel = model('FrappeSite', 'frappesites', 'FS', 'Frappe Site', 'Frappe Sites');
export const SiteMigrationModel = model('SiteMigration', 'sitemigrations', 'SM', 'Site Migration', 'Site Migrations');
export const SiteBackupModel = model('SiteBackup', 'sitebackups', 'SB', 'Site Backup', 'Site Backups');
export const SiteRestoreModel = model('SiteRestore', 'siterestores', 'SR', 'Site Restore', 'Site Restores');
export const SiteAppModel = model('SiteApp', 'siteapps', 'SA', 'Site App', 'Site Apps');

export const apiVersion = `${FRAPPE_GROUP}/${FRAPPE_VERSION}`;
