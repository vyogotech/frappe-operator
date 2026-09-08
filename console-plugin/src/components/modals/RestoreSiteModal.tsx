import * as React from 'react';
import { k8sCreate } from '@openshift-console/dynamic-plugin-sdk';
import {
  Alert,
  Checkbox,
  FormGroup,
  FormHelperText,
  FormSelect,
  FormSelectOption,
  Grid,
  GridItem,
  HelperText,
  HelperTextItem,
  TextInput,
} from '@patternfly/react-core';
import { apiVersion, SiteRestoreGVK, SiteRestoreModel } from '../../frappe/models';
import { FrappeSite } from '../../frappe/types';
import { benchRefOf, navigateTo, operationName, resourcePath, siteNameOf } from '../../frappe/utils';
import OperationModal from './OperationModal';

export interface RestoreSiteModalProps {
  site: FrappeSite;
  closeModal: () => void;
}

type SourceKind = 'localPath' | 's3';

/**
 * Creates a SiteRestore. Like SiteBackup, `spec.site` is matched against the
 * Frappe site name; `benchRef` points at the bench whose volume and image the
 * restore job runs against.
 */
export const RestoreSiteModal: React.FC<RestoreSiteModalProps> = ({ site, closeModal }) => {
  const [sourceKind, setSourceKind] = React.useState<SourceKind>('localPath');
  const [databasePath, setDatabasePath] = React.useState('');
  const [publicFilesPath, setPublicFilesPath] = React.useState('');
  const [privateFilesPath, setPrivateFilesPath] = React.useState('');
  const [force, setForce] = React.useState(false);

  const [endpoint, setEndpoint] = React.useState('');
  const [bucket, setBucket] = React.useState('');
  const [region, setRegion] = React.useState('');
  const [objectKey, setObjectKey] = React.useState('');
  const [credentialsSecret, setCredentialsSecret] = React.useState('');

  const namespace = site.metadata?.namespace;
  const frappeSiteName = siteNameOf(site);
  const benchRef = benchRefOf(site);

  const incomplete =
    !benchRef ||
    (sourceKind === 'localPath'
      ? !databasePath
      : !endpoint || !bucket || !objectKey || !credentialsSecret);

  const onSubmit = React.useCallback(async () => {
    const name = operationName(frappeSiteName, 'restore');
    const s3Base = {
      endpoint,
      bucket,
      ...(region ? { region } : {}),
      useSSL: endpoint.startsWith('https://'),
      accessKeySecret: { name: credentialsSecret, key: 'accessKeyId' },
      secretKeySecret: { name: credentialsSecret, key: 'secretAccessKey' },
    };

    await k8sCreate({
      model: SiteRestoreModel,
      data: {
        apiVersion,
        kind: 'SiteRestore',
        metadata: { name, namespace },
        spec: {
          site: frappeSiteName,
          benchRef: { name: benchRef!.name, namespace: benchRef!.namespace },
          force,
          databaseBackupSource:
            sourceKind === 'localPath'
              ? { localPath: databasePath }
              : { s3: { ...s3Base, key: objectKey } },
          ...(sourceKind === 'localPath' && publicFilesPath
            ? { publicFilesSource: { localPath: publicFilesPath } }
            : {}),
          ...(sourceKind === 'localPath' && privateFilesPath
            ? { privateFilesSource: { localPath: privateFilesPath } }
            : {}),
        },
      },
    });
    navigateTo(resourcePath(SiteRestoreGVK, name, namespace));
  }, [
    frappeSiteName,
    namespace,
    benchRef,
    force,
    sourceKind,
    databasePath,
    publicFilesPath,
    privateFilesPath,
    endpoint,
    bucket,
    region,
    objectKey,
    credentialsSecret,
  ]);

  return (
    <OperationModal
      title="Restore site"
      description={
        <>
          Restores <strong>{frappeSiteName}</strong> from a backup, replacing the current database.
        </>
      }
      submitLabel="Restore"
      submitVariant="danger"
      isSubmitDisabled={incomplete}
      onSubmit={onSubmit}
      closeModal={closeModal}
    >
      <Alert variant="warning" isInline title="This overwrites the live site">
        The restore job drops and reloads the site database. Anything written since the backup was
        taken is lost.
      </Alert>

      {!benchRef && (
        <Alert variant="danger" isInline title="No bench reference">
          This site has no benchRef, so the operator cannot tell which bench to run the restore on.
        </Alert>
      )}

      <FormGroup label="Backup location" fieldId="restore-source">
        <FormSelect
          id="restore-source"
          value={sourceKind}
          onChange={(_e, value) => setSourceKind(value as SourceKind)}
          aria-label="Backup location"
        >
          <FormSelectOption value="localPath" label="Bench volume" />
          <FormSelectOption value="s3" label="S3-compatible object storage" />
        </FormSelect>
      </FormGroup>

      {sourceKind === 'localPath' ? (
        <>
          <FormGroup label="Database backup path" isRequired fieldId="restore-db-path">
            <TextInput
              id="restore-db-path"
              value={databasePath}
              placeholder={`sites/${frappeSiteName}/private/backups/20260908_020000-database.sql.gz`}
              onChange={(_e, v) => setDatabasePath(v)}
            />
            <FormHelperText>
              <HelperText>
                <HelperTextItem>Path relative to the bench root on the shared volume.</HelperTextItem>
              </HelperText>
            </FormHelperText>
          </FormGroup>

          <FormGroup label="Public files archive" fieldId="restore-public-path">
            <TextInput
              id="restore-public-path"
              value={publicFilesPath}
              placeholder="Optional"
              onChange={(_e, v) => setPublicFilesPath(v)}
            />
          </FormGroup>

          <FormGroup label="Private files archive" fieldId="restore-private-path">
            <TextInput
              id="restore-private-path"
              value={privateFilesPath}
              placeholder="Optional"
              onChange={(_e, v) => setPrivateFilesPath(v)}
            />
          </FormGroup>
        </>
      ) : (
        <Grid hasGutter>
          <GridItem md={6}>
            <FormGroup label="Endpoint" isRequired fieldId="restore-s3-endpoint">
              <TextInput
                id="restore-s3-endpoint"
                value={endpoint}
                placeholder="https://s3.amazonaws.com"
                onChange={(_e, v) => setEndpoint(v)}
              />
            </FormGroup>
          </GridItem>
          <GridItem md={6}>
            <FormGroup label="Bucket" isRequired fieldId="restore-s3-bucket">
              <TextInput id="restore-s3-bucket" value={bucket} onChange={(_e, v) => setBucket(v)} />
            </FormGroup>
          </GridItem>
          <GridItem md={6}>
            <FormGroup label="Object key" isRequired fieldId="restore-s3-key">
              <TextInput
                id="restore-s3-key"
                value={objectKey}
                placeholder="backups/site/20260908-database.sql.gz"
                onChange={(_e, v) => setObjectKey(v)}
              />
            </FormGroup>
          </GridItem>
          <GridItem md={6}>
            <FormGroup label="Region" fieldId="restore-s3-region">
              <TextInput id="restore-s3-region" value={region} onChange={(_e, v) => setRegion(v)} />
            </FormGroup>
          </GridItem>
          <GridItem md={6}>
            <FormGroup label="Credentials secret" isRequired fieldId="restore-s3-secret">
              <TextInput
                id="restore-s3-secret"
                value={credentialsSecret}
                onChange={(_e, v) => setCredentialsSecret(v)}
              />
              <FormHelperText>
                <HelperText>
                  <HelperTextItem>
                    Secret holding <code>accessKeyId</code> and <code>secretAccessKey</code>.
                  </HelperTextItem>
                </HelperText>
              </FormHelperText>
            </FormGroup>
          </GridItem>
        </Grid>
      )}

      <FormGroup fieldId="restore-force">
        <Checkbox
          id="restore-force"
          label="Force"
          description="Proceeds even when the operator detects the backup would downgrade the site."
          isChecked={force}
          onChange={(_e, checked) => setForce(checked)}
        />
      </FormGroup>
    </OperationModal>
  );
};

export default RestoreSiteModal;
