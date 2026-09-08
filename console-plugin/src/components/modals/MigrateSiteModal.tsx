import * as React from 'react';
import { k8sCreate } from '@openshift-console/dynamic-plugin-sdk';
import {
  Alert,
  Checkbox,
  FormGroup,
  FormHelperText,
  HelperText,
  HelperTextItem,
} from '@patternfly/react-core';
import { apiVersion, SiteMigrationGVK, SiteMigrationModel } from '../../frappe/models';
import { FrappeSite } from '../../frappe/types';
import { isSiteReady, navigateTo, operationName, resourcePath, siteNameOf } from '../../frappe/utils';
import OperationModal from './OperationModal';

export interface MigrateSiteModalProps {
  site: FrappeSite;
  closeModal: () => void;
}

/**
 * Runs `bench migrate` against a site by creating a SiteMigration. The
 * operator resolves `siteRef` against the FrappeSite CR name (not the Frappe
 * site name) and refuses to start until the site reports Ready.
 */
export const MigrateSiteModal: React.FC<MigrateSiteModalProps> = ({ site, closeModal }) => {
  const [backupBeforeMigrate, setBackupBeforeMigrate] = React.useState(true);
  const [skipFixtures, setSkipFixtures] = React.useState(false);
  const [force, setForce] = React.useState(false);

  const namespace = site.metadata?.namespace;
  const siteCRName = site.metadata?.name ?? '';
  const ready = isSiteReady(site);

  const onSubmit = React.useCallback(async () => {
    const name = operationName(siteCRName, 'migrate');
    await k8sCreate({
      model: SiteMigrationModel,
      data: {
        apiVersion,
        kind: 'SiteMigration',
        metadata: { name, namespace },
        spec: {
          siteRef: { name: siteCRName, namespace },
          backupBeforeMigrate,
          skipFixtures,
          force,
        },
      },
    });
    navigateTo(resourcePath(SiteMigrationGVK, name, namespace));
  }, [siteCRName, namespace, backupBeforeMigrate, skipFixtures, force]);

  return (
    <OperationModal
      title="Migrate site"
      description={
        <>
          Runs <code>bench migrate</code> on <strong>{siteNameOf(site)}</strong> to apply pending
          schema changes and patches from its installed apps.
        </>
      }
      submitLabel="Migrate"
      onSubmit={onSubmit}
      closeModal={closeModal}
    >
      {!ready && (
        <Alert
          variant="warning"
          isInline
          title={`Site is ${site.status?.phase ?? 'not ready'}`}
        >
          The migration will be created but stays Pending until the site reaches the Ready phase.
        </Alert>
      )}

      <FormGroup fieldId="migrate-backup">
        <Checkbox
          id="migrate-backup"
          label="Back up the site first"
          description="Takes a full backup and waits for it to succeed before migrating, giving you a rollback point."
          isChecked={backupBeforeMigrate}
          onChange={(_e, checked) => setBackupBeforeMigrate(checked)}
        />
      </FormGroup>

      <FormGroup fieldId="migrate-skip-fixtures">
        <Checkbox
          id="migrate-skip-fixtures"
          label="Skip fixtures"
          description="Skips fixture synchronisation during the migration."
          isChecked={skipFixtures}
          onChange={(_e, checked) => setSkipFixtures(checked)}
        />
      </FormGroup>

      <FormGroup fieldId="migrate-force">
        <Checkbox
          id="migrate-force"
          label="Force migration"
          description="Migrates even when the schema is unchanged."
          isChecked={force}
          onChange={(_e, checked) => setForce(checked)}
        />
      </FormGroup>

      {!backupBeforeMigrate && (
        <FormHelperText>
          <HelperText>
            <HelperTextItem variant="warning">
              Without a pre-migration backup there is no automatic rollback point if a patch fails
              part way through.
            </HelperTextItem>
          </HelperText>
        </FormHelperText>
      )}
    </OperationModal>
  );
};

export default MigrateSiteModal;
