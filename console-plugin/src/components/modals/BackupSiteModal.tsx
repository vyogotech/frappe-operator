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
import { apiVersion, SiteBackupGVK, SiteBackupModel } from '../../frappe/models';
import { FrappeSite } from '../../frappe/types';
import { isSiteReady, navigateTo, operationName, resourcePath, siteNameOf } from '../../frappe/utils';
import OperationModal from './OperationModal';

export interface BackupSiteModalProps {
  site: FrappeSite;
  closeModal: () => void;
}

/**
 * Creates a SiteBackup. The operator matches `spec.site` against the Frappe
 * site name (FrappeSite `spec.siteName`), not the CR name, and treats an empty
 * schedule as a one-off backup.
 */
export const BackupSiteModal: React.FC<BackupSiteModalProps> = ({ site, closeModal }) => {
  const [withFiles, setWithFiles] = React.useState(true);
  const [compress, setCompress] = React.useState(true);
  const [isScheduled, setIsScheduled] = React.useState(false);
  const [schedule, setSchedule] = React.useState('0 2 * * *');
  const [storageType, setStorageType] = React.useState<'pvc' | 's3'>('pvc');

  const [endpoint, setEndpoint] = React.useState('');
  const [bucket, setBucket] = React.useState('');
  const [region, setRegion] = React.useState('');
  const [credentialsSecret, setCredentialsSecret] = React.useState('');

  const namespace = site.metadata?.namespace;
  const frappeSiteName = siteNameOf(site);

  const s3Incomplete = storageType === 's3' && (!endpoint || !bucket || !credentialsSecret);
  const scheduleInvalid = isScheduled && schedule.trim().split(/\s+/).length !== 5;

  const onSubmit = React.useCallback(async () => {
    const name = operationName(frappeSiteName, isScheduled ? 'backup-cron' : 'backup');
    await k8sCreate({
      model: SiteBackupModel,
      data: {
        apiVersion,
        kind: 'SiteBackup',
        metadata: { name, namespace },
        spec: {
          site: frappeSiteName,
          withFiles,
          compress,
          ...(isScheduled ? { schedule: schedule.trim() } : {}),
          storage:
            storageType === 's3'
              ? {
                  type: 's3',
                  s3: {
                    endpoint,
                    bucket,
                    ...(region ? { region } : {}),
                    useSSL: endpoint.startsWith('https://'),
                    accessKeySecret: { name: credentialsSecret, key: 'accessKeyId' },
                    secretKeySecret: { name: credentialsSecret, key: 'secretAccessKey' },
                  },
                }
              : { type: 'pvc' },
        },
      },
    });
    navigateTo(resourcePath(SiteBackupGVK, name, namespace));
  }, [
    frappeSiteName,
    namespace,
    withFiles,
    compress,
    isScheduled,
    schedule,
    storageType,
    endpoint,
    bucket,
    region,
    credentialsSecret,
  ]);

  return (
    <OperationModal
      title={isScheduled ? 'Schedule backups' : 'Back up site'}
      description={
        <>
          Runs <code>bench backup</code> for <strong>{frappeSiteName}</strong>.
        </>
      }
      submitLabel={isScheduled ? 'Create schedule' : 'Back up'}
      isSubmitDisabled={s3Incomplete || scheduleInvalid}
      onSubmit={onSubmit}
      closeModal={closeModal}
    >
      {!isSiteReady(site) && (
        <Alert variant="warning" isInline title={`Site is ${site.status?.phase ?? 'not ready'}`}>
          The backup stays Pending until the site reaches the Ready phase.
        </Alert>
      )}

      <FormGroup fieldId="backup-with-files">
        <Checkbox
          id="backup-with-files"
          label="Include files"
          description="Adds the site's public and private files to the backup alongside the database."
          isChecked={withFiles}
          onChange={(_e, checked) => setWithFiles(checked)}
        />
      </FormGroup>

      <FormGroup fieldId="backup-compress">
        <Checkbox
          id="backup-compress"
          label="Compress the backup"
          isChecked={compress}
          onChange={(_e, checked) => setCompress(checked)}
        />
      </FormGroup>

      <FormGroup fieldId="backup-scheduled">
        <Checkbox
          id="backup-scheduled"
          label="Repeat on a schedule"
          description="Creates a recurring CronJob instead of a single backup."
          isChecked={isScheduled}
          onChange={(_e, checked) => setIsScheduled(checked)}
        />
      </FormGroup>

      {isScheduled && (
        <FormGroup label="Cron schedule" isRequired fieldId="backup-schedule">
          <TextInput
            id="backup-schedule"
            value={schedule}
            onChange={(_e, value) => setSchedule(value)}
            validated={scheduleInvalid ? 'error' : 'default'}
            aria-label="Cron schedule"
          />
          <FormHelperText>
            <HelperText>
              <HelperTextItem variant={scheduleInvalid ? 'error' : 'default'}>
                {scheduleInvalid
                  ? 'Enter five space-separated cron fields, for example 0 2 * * *'
                  : 'Standard five-field cron expression, in the cluster time zone.'}
              </HelperTextItem>
            </HelperText>
          </FormHelperText>
        </FormGroup>
      )}

      <FormGroup label="Storage" fieldId="backup-storage">
        <FormSelect
          id="backup-storage"
          value={storageType}
          onChange={(_e, value) => setStorageType(value as 'pvc' | 's3')}
          aria-label="Backup storage backend"
        >
          <FormSelectOption value="pvc" label="Bench volume (PVC)" />
          <FormSelectOption value="s3" label="S3-compatible object storage" />
        </FormSelect>
        <FormHelperText>
          <HelperText>
            <HelperTextItem>
              {storageType === 'pvc'
                ? "Backups are written to the site's private backup directory on the bench volume."
                : 'Backups are uploaded off-cluster and the object location is reported in the backup status.'}
            </HelperTextItem>
          </HelperText>
        </FormHelperText>
      </FormGroup>

      {storageType === 's3' && (
        <Grid hasGutter>
          <GridItem md={6}>
            <FormGroup label="Endpoint" isRequired fieldId="backup-s3-endpoint">
              <TextInput
                id="backup-s3-endpoint"
                value={endpoint}
                placeholder="https://s3.amazonaws.com"
                onChange={(_e, value) => setEndpoint(value)}
              />
            </FormGroup>
          </GridItem>
          <GridItem md={6}>
            <FormGroup label="Bucket" isRequired fieldId="backup-s3-bucket">
              <TextInput
                id="backup-s3-bucket"
                value={bucket}
                onChange={(_e, value) => setBucket(value)}
              />
            </FormGroup>
          </GridItem>
          <GridItem md={6}>
            <FormGroup label="Region" fieldId="backup-s3-region">
              <TextInput
                id="backup-s3-region"
                value={region}
                onChange={(_e, value) => setRegion(value)}
              />
            </FormGroup>
          </GridItem>
          <GridItem md={6}>
            <FormGroup label="Credentials secret" isRequired fieldId="backup-s3-secret">
              <TextInput
                id="backup-s3-secret"
                value={credentialsSecret}
                onChange={(_e, value) => setCredentialsSecret(value)}
              />
              <FormHelperText>
                <HelperText>
                  <HelperTextItem>
                    Secret in {namespace ?? 'this namespace'} holding the keys
                    <code> accessKeyId </code> and <code> secretAccessKey</code>.
                  </HelperTextItem>
                </HelperText>
              </FormHelperText>
            </FormGroup>
          </GridItem>
        </Grid>
      )}
    </OperationModal>
  );
};

export default BackupSiteModal;
