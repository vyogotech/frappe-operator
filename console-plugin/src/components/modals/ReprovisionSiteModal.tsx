import * as React from 'react';
import { k8sPatch } from '@openshift-console/dynamic-plugin-sdk';
import { Alert, List, ListItem } from '@patternfly/react-core';
import { FrappeSiteModel } from '../../frappe/models';
import { FrappeSite } from '../../frappe/types';
import { siteNameOf } from '../../frappe/utils';
import OperationModal from './OperationModal';

export interface ReprovisionSiteModalProps {
  site: FrappeSite;
  closeModal: () => void;
}

const SITE_VERSION_ANNOTATION = 'frappe.io/site-version';

/**
 * Re-runs the site initialization job by bumping the `frappe.io/site-version`
 * annotation. The controller compares the annotation against
 * `status.observedSiteVersion`, deletes the stale `<site>-init` Job and lets it
 * be recreated — the supported way to retry a provision that failed part way
 * through, or to pick up newly requested apps.
 */
export const ReprovisionSiteModal: React.FC<ReprovisionSiteModalProps> = ({ site, closeModal }) => {
  const current = site.metadata?.annotations?.[SITE_VERSION_ANNOTATION];
  const next = String(Date.now());

  const onSubmit = React.useCallback(async () => {
    await k8sPatch({
      model: FrappeSiteModel,
      resource: site,
      data: [
        {
          // The annotations map may not exist yet; `add` on the map itself
          // replaces it, so seed it in that case and patch the key otherwise.
          op: 'add',
          path: site.metadata?.annotations
            ? `/metadata/annotations/${SITE_VERSION_ANNOTATION.replace(/~/g, '~0').replace(/\//g, '~1')}`
            : '/metadata/annotations',
          value: site.metadata?.annotations ? next : { [SITE_VERSION_ANNOTATION]: next },
        },
      ],
    });
  }, [site, next]);

  return (
    <OperationModal
      title="Re-run site initialization"
      description={
        <>
          Restarts the initialization job for <strong>{siteNameOf(site)}</strong>.
        </>
      }
      submitLabel="Re-run initialization"
      submitVariant="danger"
      onSubmit={onSubmit}
      closeModal={closeModal}
    >
      <Alert variant="warning" isInline title="What this does">
        <List>
          <ListItem>
            Bumps the <code>{SITE_VERSION_ANNOTATION}</code> annotation
            {current ? (
              <>
                {' '}
                from <code>{current}</code>
              </>
            ) : null}{' '}
            to <code>{next}</code>.
          </ListItem>
          <ListItem>
            Deletes the existing <code>{site.metadata?.name}-init</code> Job so the operator recreates
            it.
          </ListItem>
          <ListItem>
            Re-runs site creation and app installation. On a site that already has data, the job
            performs migrations and configuration updates rather than recreating the database.
          </ListItem>
        </List>
      </Alert>
      <Alert variant="info" isInline title="Use this to retry a stuck provision">
        For routine schema updates on a healthy site, use <strong>Migrate site</strong> instead — it
        runs <code>bench migrate</code> and can take a backup first.
      </Alert>
    </OperationModal>
  );
};

export default ReprovisionSiteModal;
