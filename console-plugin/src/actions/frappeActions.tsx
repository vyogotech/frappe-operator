import * as React from 'react';
import {
  Action,
  ExtensionHook,
  K8sVerb,
  useK8sWatchResource,
  useModal,
} from '@openshift-console/dynamic-plugin-sdk';
import { FrappeSiteGVK } from '../frappe/models';
import { FrappeBench, FrappeSite } from '../frappe/types';
import { benchAppNames, benchRefOf } from '../frappe/utils';
import BackupSiteModal from '../components/modals/BackupSiteModal';
import InstallAppModal from '../components/modals/InstallAppModal';
import MigrateBenchSitesModal from '../components/modals/MigrateBenchSitesModal';
import MigrateSiteModal from '../components/modals/MigrateSiteModal';
import ReprovisionSiteModal from '../components/modals/ReprovisionSiteModal';
import RestoreSiteModal from '../components/modals/RestoreSiteModal';

/**
 * Access review for creating an operation CR in the resource's own namespace.
 * The console greys out actions the user lacks RBAC for instead of letting them
 * fill in a form the API server will reject on submit.
 */
const createReview = (namespace: string | undefined, resource: string) => ({
  group: 'vyogo.tech',
  resource,
  namespace,
  verb: 'create' as K8sVerb,
});

// ── FrappeSite ───────────────────────────────────────────────────────────────

/**
 * Frappe-native lifecycle verbs for a site, contributed to the console Actions
 * menu on both the FrappeSite list and details pages.
 *
 * The operation actions stay enabled while a site is still provisioning: the
 * operator accepts the CR and holds it Pending until the site reports Ready,
 * which is more useful than refusing the click.
 */
export const useFrappeSiteActions: ExtensionHook<Action[], FrappeSite> = (site) => {
  const launchModal = useModal();
  const namespace = site?.metadata?.namespace;
  const siteURL = site?.status?.siteURL;
  const isReady = site?.status?.phase === 'Ready';

  const actions = React.useMemo<Action[]>(() => {
    if (!site?.metadata?.name) {
      return [];
    }

    return [
      {
        id: 'frappe-site-open',
        label: 'Open site',
        description: 'Open the tenant site in a new tab',
        disabled: !siteURL,
        disabledTooltip: 'The operator has not resolved a site URL yet.',
        cta: { href: siteURL ?? '', external: true },
      },
      {
        id: 'frappe-site-migrate',
        label: 'Migrate site',
        description: 'Run bench migrate to apply pending schema changes',
        cta: () => launchModal(MigrateSiteModal, { site }),
        accessReview: createReview(namespace, 'sitemigrations'),
      },
      {
        id: 'frappe-site-install-app',
        label: 'Install app',
        description: 'Install a Frappe app from FPM or Git',
        cta: () => launchModal(InstallAppModal, { site }),
        accessReview: createReview(namespace, 'siteapps'),
      },
      {
        id: 'frappe-site-backup',
        label: 'Back up site',
        description: 'Take a backup now, or set up a recurring schedule',
        cta: () => launchModal(BackupSiteModal, { site }),
        accessReview: createReview(namespace, 'sitebackups'),
      },
      {
        id: 'frappe-site-restore',
        label: 'Restore site',
        description: 'Replace the site database from a backup',
        cta: () => launchModal(RestoreSiteModal, { site }),
        accessReview: createReview(namespace, 'siterestores'),
      },
      {
        id: 'frappe-site-reprovision',
        label: 'Re-run site initialization',
        description: 'Restart the init job — use this to retry a failed provision',
        disabled: isReady,
        disabledTooltip:
          'The site is Ready. Use Migrate site for routine schema updates on a healthy site.',
        cta: () => launchModal(ReprovisionSiteModal, { site }),
        accessReview: {
          group: 'vyogo.tech',
          resource: 'frappesites',
          namespace,
          name: site.metadata.name,
          verb: 'patch' as K8sVerb,
        },
      },
    ];
  }, [site, namespace, siteURL, isReady, launchModal]);

  return [actions, true, undefined];
};

// ── FrappeBench ──────────────────────────────────────────────────────────────

/** Bench-level operations: provisioning a tenant and fleet-wide migration. */
export const useFrappeBenchActions: ExtensionHook<Action[], FrappeBench> = (bench) => {
  const launchModal = useModal();
  const name = bench?.metadata?.name;
  const namespace = bench?.metadata?.namespace;

  const [sites] = useK8sWatchResource<FrappeSite[]>({
    groupVersionKind: FrappeSiteGVK,
    isList: true,
    namespace,
  });

  const benchSites = React.useMemo(
    () => (sites ?? []).filter((s) => benchRefOf(s)?.name === name),
    [sites, name],
  );

  const actions = React.useMemo<Action[]>(() => {
    if (!name) {
      return [];
    }

    const appCount = benchAppNames(bench).length;

    return [
      {
        id: 'frappe-bench-create-site',
        label: 'Create site',
        description: appCount
          ? `Provision a tenant site with any of the ${appCount} app(s) on this bench`
          : 'Provision a tenant site on this bench',
        cta: {
          href: `/frappe/sites/~new?bench=${encodeURIComponent(name)}&namespace=${encodeURIComponent(
            namespace ?? '',
          )}`,
        },
        accessReview: createReview(namespace, 'frappesites'),
      },
      {
        id: 'frappe-bench-migrate-sites',
        label: 'Migrate all sites',
        description: benchSites.length
          ? `Run bench migrate across the ${benchSites.length} site(s) on this bench`
          : 'No sites reference this bench yet',
        disabled: benchSites.length === 0,
        disabledTooltip: 'No FrappeSite references this bench.',
        cta: () => launchModal(MigrateBenchSitesModal, { bench, sites: benchSites }),
        accessReview: createReview(namespace, 'sitemigrations'),
      },
    ];
  }, [bench, name, namespace, benchSites, launchModal]);

  return [actions, true, undefined];
};
