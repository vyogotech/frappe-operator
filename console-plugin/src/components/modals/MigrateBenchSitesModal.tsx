import * as React from 'react';
import { k8sCreate, ResourceLink } from '@openshift-console/dynamic-plugin-sdk';
import {
  Alert,
  Checkbox,
  DataList,
  DataListCell,
  DataListCheck,
  DataListItem,
  DataListItemCells,
  DataListItemRow,
  FormGroup,
  FormHelperText,
  HelperText,
  HelperTextItem,
  Label,
} from '@patternfly/react-core';
import { apiVersion, FrappeSiteGVK, SiteMigrationModel } from '../../frappe/models';
import { FrappeBench, FrappeSite } from '../../frappe/types';
import { operationName, phaseColor, siteNameOf } from '../../frappe/utils';
import OperationModal from './OperationModal';

export interface MigrateBenchSitesModalProps {
  bench: FrappeBench;
  sites: FrappeSite[];
  closeModal: () => void;
}

/**
 * Fans a migration out across the sites on a bench — the usual follow-up after
 * upgrading the apps a bench serves. One SiteMigration is created per selected
 * site; the operator serialises the actual `bench migrate` runs.
 */
export const MigrateBenchSitesModal: React.FC<MigrateBenchSitesModalProps> = ({
  bench,
  sites,
  closeModal,
}) => {
  const [selected, setSelected] = React.useState<string[]>(() =>
    sites.map((s) => s.metadata?.name ?? '').filter(Boolean),
  );
  const [backupBeforeMigrate, setBackupBeforeMigrate] = React.useState(true);
  const [skipFixtures, setSkipFixtures] = React.useState(false);

  const toggle = (name: string, checked: boolean) =>
    setSelected((prev) => (checked ? [...prev, name] : prev.filter((n) => n !== name)));

  const onSubmit = React.useCallback(async () => {
    const targets = sites.filter((s) => selected.includes(s.metadata?.name ?? ''));
    const failures: string[] = [];

    // Sequential so that a partial failure reports exactly which sites were
    // left untouched, rather than an opaque aggregate rejection.
    for (const site of targets) {
      const siteCRName = site.metadata?.name ?? '';
      const namespace = site.metadata?.namespace;
      try {
        await k8sCreate({
          model: SiteMigrationModel,
          data: {
            apiVersion,
            kind: 'SiteMigration',
            metadata: { name: operationName(siteCRName, 'migrate'), namespace },
            spec: {
              siteRef: { name: siteCRName, namespace },
              backupBeforeMigrate,
              skipFixtures,
            },
          },
        });
      } catch (e) {
        failures.push(`${siteCRName}: ${(e as { message?: string })?.message ?? 'rejected'}`);
      }
    }

    if (failures.length > 0) {
      throw new Error(
        `Created ${targets.length - failures.length} of ${targets.length} migrations. ` +
          `Failed: ${failures.join('; ')}`,
      );
    }
  }, [sites, selected, backupBeforeMigrate, skipFixtures]);

  const notReady = sites.filter(
    (s) => selected.includes(s.metadata?.name ?? '') && s.status?.phase !== 'Ready',
  );

  return (
    <OperationModal
      title="Migrate all sites"
      description={
        <>
          Creates one SiteMigration per selected site on{' '}
          <strong>{bench.metadata?.name}</strong>.
        </>
      }
      submitLabel={`Migrate ${selected.length} site${selected.length === 1 ? '' : 's'}`}
      isSubmitDisabled={selected.length === 0}
      onSubmit={onSubmit}
      closeModal={closeModal}
    >
      {notReady.length > 0 && (
        <Alert variant="info" isInline title={`${notReady.length} selected site(s) are not Ready`}>
          Their migrations will be created and stay Pending until each site reaches Ready.
        </Alert>
      )}

      <FormGroup label="Sites" fieldId="migrate-bench-sites" role="group">
        <DataList aria-label="Sites on this bench" isCompact>
          {sites.map((site) => {
            const crName = site.metadata?.name ?? '';
            const phase = site.status?.phase;
            return (
              <DataListItem key={crName} aria-labelledby={`migrate-site-${crName}`}>
                <DataListItemRow>
                  <DataListCheck
                    aria-labelledby={`migrate-site-${crName}`}
                    name={`migrate-site-${crName}`}
                    isChecked={selected.includes(crName)}
                    onChange={(_e, checked) => toggle(crName, checked)}
                  />
                  <DataListItemCells
                    dataListCells={[
                      <DataListCell key="name" id={`migrate-site-${crName}`}>
                        <ResourceLink
                          groupVersionKind={FrappeSiteGVK}
                          name={crName}
                          namespace={site.metadata?.namespace}
                          linkTo={false}
                        />
                      </DataListCell>,
                      <DataListCell key="site-name">
                        <code>{siteNameOf(site)}</code>
                      </DataListCell>,
                      <DataListCell key="phase">
                        <Label isCompact color={phaseColor(phase)}>
                          {phase ?? 'Unknown'}
                        </Label>
                      </DataListCell>,
                    ]}
                  />
                </DataListItemRow>
              </DataListItem>
            );
          })}
        </DataList>
      </FormGroup>

      <FormGroup fieldId="migrate-bench-backup">
        <Checkbox
          id="migrate-bench-backup"
          label="Back up each site first"
          description="Every migration waits for its own backup to succeed before running."
          isChecked={backupBeforeMigrate}
          onChange={(_e, checked) => setBackupBeforeMigrate(checked)}
        />
      </FormGroup>

      <FormGroup fieldId="migrate-bench-fixtures">
        <Checkbox
          id="migrate-bench-fixtures"
          label="Skip fixtures"
          isChecked={skipFixtures}
          onChange={(_e, checked) => setSkipFixtures(checked)}
        />
      </FormGroup>

      <FormHelperText>
        <HelperText>
          <HelperTextItem>
            Migrations are created one at a time. If any is rejected, the ones already created stay
            in place and the failures are listed here.
          </HelperTextItem>
        </HelperText>
      </FormHelperText>
    </OperationModal>
  );
};

export default MigrateBenchSitesModal;
