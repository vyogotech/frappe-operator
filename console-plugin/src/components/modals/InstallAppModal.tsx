import * as React from 'react';
import { k8sCreate } from '@openshift-console/dynamic-plugin-sdk';
import {
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
import { apiVersion, SiteAppGVK, SiteAppModel } from '../../frappe/models';
import { FrappeSite } from '../../frappe/types';
import { navigateTo, operationName, resourcePath, siteNameOf, toDNS1123 } from '../../frappe/utils';
import OperationModal from './OperationModal';

export interface InstallAppModalProps {
  site: FrappeSite;
  closeModal: () => void;
}

type AppSourceKind = 'fpm' | 'git';

/**
 * Installs a Frappe app onto a site by creating a SiteApp. FPM packages are the
 * preferred path — they ship compiled assets and vendored wheels, so nothing is
 * built on the bench at install time; a git clone is the fallback for apps with
 * no published package.
 */
export const InstallAppModal: React.FC<InstallAppModalProps> = ({ site, closeModal }) => {
  const [appName, setAppName] = React.useState('');
  const [sourceKind, setSourceKind] = React.useState<AppSourceKind>('fpm');

  const [fpmOrg, setFpmOrg] = React.useState('frappe');
  const [fpmVersion, setFpmVersion] = React.useState('');
  const [fpmRepo, setFpmRepo] = React.useState('https://fpm.vyogo.tech');
  const [fpmRepoType, setFpmRepoType] = React.useState<'http' | 'oci'>('http');

  const [gitRepo, setGitRepo] = React.useState('');
  const [gitBranch, setGitBranch] = React.useState('');

  const [autoMigrate, setAutoMigrate] = React.useState(true);
  const [backupBeforeInstall, setBackupBeforeInstall] = React.useState(true);

  const namespace = site.metadata?.namespace;
  const siteCRName = site.metadata?.name ?? '';

  const fpmPackage = fpmVersion ? `${fpmOrg}/${appName}==${fpmVersion}` : '';
  const incomplete =
    !appName ||
    (sourceKind === 'fpm' ? !fpmOrg || !fpmVersion || !fpmRepo : !gitRepo);

  const onSubmit = React.useCallback(async () => {
    const name = operationName(`${toDNS1123(siteCRName, 30)}-${toDNS1123(appName, 20)}`, 'app');
    await k8sCreate({
      model: SiteAppModel,
      data: {
        apiVersion,
        kind: 'SiteApp',
        metadata: { name, namespace },
        spec: {
          siteRef: { name: siteCRName, namespace },
          appName,
          autoMigrate,
          backupBeforeInstall,
          ...(sourceKind === 'fpm'
            ? { fpmPackage, fpmRepo, fpmRepoType }
            : { gitRepo, ...(gitBranch ? { gitBranch } : {}) }),
        },
      },
    });
    navigateTo(resourcePath(SiteAppGVK, name, namespace));
  }, [
    siteCRName,
    namespace,
    appName,
    sourceKind,
    fpmPackage,
    fpmRepo,
    fpmRepoType,
    gitRepo,
    gitBranch,
    autoMigrate,
    backupBeforeInstall,
  ]);

  return (
    <OperationModal
      title="Install app"
      description={
        <>
          Installs a Frappe app onto <strong>{siteNameOf(site)}</strong>.
        </>
      }
      submitLabel="Install"
      isSubmitDisabled={incomplete}
      onSubmit={onSubmit}
      closeModal={closeModal}
    >
      <FormGroup label="App name" isRequired fieldId="app-name">
        <TextInput
          id="app-name"
          value={appName}
          placeholder="erpnext"
          onChange={(_e, value) => setAppName(value)}
        />
        <FormHelperText>
          <HelperText>
            <HelperTextItem>
              The Frappe Python module name, as it appears in the bench apps directory.
            </HelperTextItem>
          </HelperText>
        </FormHelperText>
      </FormGroup>

      <FormGroup label="Source" fieldId="app-source">
        <FormSelect
          id="app-source"
          value={sourceKind}
          onChange={(_e, value) => setSourceKind(value as AppSourceKind)}
          aria-label="App source"
        >
          <FormSelectOption value="fpm" label="FPM package (recommended)" />
          <FormSelectOption value="git" label="Git repository" />
        </FormSelect>
        <FormHelperText>
          <HelperText>
            <HelperTextItem>
              {sourceKind === 'fpm'
                ? 'Prebuilt package with compiled assets and vendored wheels — nothing is built on the bench.'
                : 'Cloned and built on the bench at install time. Requires Git access from the cluster.'}
            </HelperTextItem>
          </HelperText>
        </FormHelperText>
      </FormGroup>

      {sourceKind === 'fpm' ? (
        <Grid hasGutter>
          <GridItem md={6}>
            <FormGroup label="Organization" isRequired fieldId="app-fpm-org">
              <TextInput id="app-fpm-org" value={fpmOrg} onChange={(_e, v) => setFpmOrg(v)} />
            </FormGroup>
          </GridItem>
          <GridItem md={6}>
            <FormGroup label="Version" isRequired fieldId="app-fpm-version">
              <TextInput
                id="app-fpm-version"
                value={fpmVersion}
                placeholder="3.0.0"
                onChange={(_e, v) => setFpmVersion(v)}
              />
            </FormGroup>
          </GridItem>
          <GridItem md={8}>
            <FormGroup label="Registry" isRequired fieldId="app-fpm-repo">
              <TextInput id="app-fpm-repo" value={fpmRepo} onChange={(_e, v) => setFpmRepo(v)} />
            </FormGroup>
          </GridItem>
          <GridItem md={4}>
            <FormGroup label="Registry type" fieldId="app-fpm-repo-type">
              <FormSelect
                id="app-fpm-repo-type"
                value={fpmRepoType}
                onChange={(_e, v) => setFpmRepoType(v as 'http' | 'oci')}
                aria-label="Registry type"
              >
                <FormSelectOption value="http" label="HTTP" />
                <FormSelectOption value="oci" label="OCI" />
              </FormSelect>
            </FormGroup>
          </GridItem>
          <GridItem>
            <FormHelperText>
              <HelperText>
                <HelperTextItem>
                  {fpmPackage ? (
                    <>
                      Resolves package <code>{fpmPackage}</code>
                      {fpmRepoType === 'oci'
                        ? '. OCI registries are usually private — credentials come from the fpm-registry-auth secret in this namespace.'
                        : '.'}
                    </>
                  ) : (
                    'Enter an app name and version to see the package that will be resolved.'
                  )}
                </HelperTextItem>
              </HelperText>
            </FormHelperText>
          </GridItem>
        </Grid>
      ) : (
        <Grid hasGutter>
          <GridItem md={8}>
            <FormGroup label="Repository URL" isRequired fieldId="app-git-repo">
              <TextInput
                id="app-git-repo"
                value={gitRepo}
                placeholder="https://github.com/frappe/wiki"
                onChange={(_e, v) => setGitRepo(v)}
              />
            </FormGroup>
          </GridItem>
          <GridItem md={4}>
            <FormGroup label="Branch or tag" fieldId="app-git-branch">
              <TextInput
                id="app-git-branch"
                value={gitBranch}
                placeholder="default branch"
                onChange={(_e, v) => setGitBranch(v)}
              />
            </FormGroup>
          </GridItem>
        </Grid>
      )}

      <FormGroup fieldId="app-backup">
        <Checkbox
          id="app-backup"
          label="Back up the site first"
          description="Creates a rollback point before the app is installed or upgraded."
          isChecked={backupBeforeInstall}
          onChange={(_e, checked) => setBackupBeforeInstall(checked)}
        />
      </FormGroup>

      <FormGroup fieldId="app-auto-migrate">
        <Checkbox
          id="app-auto-migrate"
          label="Migrate after install"
          description="Runs bench migrate once the app is installed so its schema changes are applied."
          isChecked={autoMigrate}
          onChange={(_e, checked) => setAutoMigrate(checked)}
        />
      </FormGroup>
    </OperationModal>
  );
};

export default InstallAppModal;
