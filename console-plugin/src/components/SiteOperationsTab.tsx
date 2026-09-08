import * as React from 'react';
import {
  ResourceLink,
  Timestamp,
  useK8sWatchResource,
  useModal,
} from '@openshift-console/dynamic-plugin-sdk';
import {
  Alert,
  Button,
  Card,
  CardBody,
  CardTitle,
  EmptyState,
  EmptyStateBody,
  Flex,
  FlexItem,
  Label,
  Split,
  SplitItem,
  Title,
} from '@patternfly/react-core';
import { Table, Tbody, Td, Th, Thead, Tr } from '@patternfly/react-table';
import {
  SiteAppGVK,
  SiteBackupGVK,
  SiteMigrationGVK,
  SiteRestoreGVK,
} from '../frappe/models';
import { FrappeSite, SiteApp, SiteBackup, SiteMigration, SiteRestore } from '../frappe/types';
import { phaseColor, siteNameOf } from '../frappe/utils';
import BackupSiteModal from './modals/BackupSiteModal';
import InstallAppModal from './modals/InstallAppModal';
import MigrateSiteModal from './modals/MigrateSiteModal';
import RestoreSiteModal from './modals/RestoreSiteModal';
import PluginRoot from './PluginRoot';

export interface SiteOperationsTabProps {
  obj?: FrappeSite;
}

const Muted: React.FC<{ children: React.ReactNode }> = ({ children }) => (
  <span style={{ color: 'var(--pf-t--global--text--color--subtle)' }}>{children}</span>
);

const PhaseLabel: React.FC<{ phase?: string }> = ({ phase }) =>
  phase ? (
    <Label isCompact color={phaseColor(phase)}>
      {phase}
    </Label>
  ) : (
    <Muted>Unknown</Muted>
  );

/** Newest first — an operations log is read from the top. */
const byNewest = <T extends { metadata?: { creationTimestamp?: string } }>(a: T, b: T) =>
  (b.metadata?.creationTimestamp ?? '').localeCompare(a.metadata?.creationTimestamp ?? '');

const OperationCard: React.FC<{
  title: string;
  action: React.ReactNode;
  headers: string[];
  isEmpty: boolean;
  emptyText: string;
  children: React.ReactNode;
}> = ({ title, action, headers, isEmpty, emptyText, children }) => (
  <Card style={{ marginBottom: '16px' }}>
    <CardTitle>
      <Split hasGutter>
        <SplitItem isFilled>{title}</SplitItem>
        <SplitItem>{action}</SplitItem>
      </Split>
    </CardTitle>
    <CardBody>
      {isEmpty ? (
        <EmptyState variant="xs">
          <EmptyStateBody>{emptyText}</EmptyStateBody>
        </EmptyState>
      ) : (
        <Table variant="compact" aria-label={title}>
          <Thead>
            <Tr>
              {headers.map((h) => (
                <Th key={h}>{h}</Th>
              ))}
            </Tr>
          </Thead>
          <Tbody>{children}</Tbody>
        </Table>
      )}
    </CardBody>
  </Card>
);

/**
 * The Frappe operations log for one site: every migration, backup, restore and
 * app install the operator has run, plus the buttons to start a new one. These
 * are separate CRDs in separate console list pages, which makes the history of
 * a single tenant hard to follow — this pulls it back together.
 */
export const SiteOperationsTab: React.FC<SiteOperationsTabProps> = ({ obj }) => {
  const launchModal = useModal();
  const namespace = obj?.metadata?.namespace;
  const siteCRName = obj?.metadata?.name;
  const frappeSiteName = siteNameOf(obj);

  const watch = namespace ? { isList: true as const, namespace } : null;

  const [migrations = [], migrationsLoaded] = useK8sWatchResource<SiteMigration[]>(
    watch && { ...watch, groupVersionKind: SiteMigrationGVK },
  );
  const [backups = [], backupsLoaded] = useK8sWatchResource<SiteBackup[]>(
    watch && { ...watch, groupVersionKind: SiteBackupGVK },
  );
  const [restores = [], restoresLoaded] = useK8sWatchResource<SiteRestore[]>(
    watch && { ...watch, groupVersionKind: SiteRestoreGVK },
  );
  const [apps = [], appsLoaded] = useK8sWatchResource<SiteApp[]>(
    watch && { ...watch, groupVersionKind: SiteAppGVK },
  );

  // Migrations and apps reference the FrappeSite CR by name; backups and
  // restores match on the Frappe site name instead.
  const siteMigrations = React.useMemo(
    () => migrations.filter((m) => m.spec?.siteRef?.name === siteCRName).sort(byNewest),
    [migrations, siteCRName],
  );
  const siteApps = React.useMemo(
    () => apps.filter((a) => a.spec?.siteRef?.name === siteCRName).sort(byNewest),
    [apps, siteCRName],
  );
  const siteBackups = React.useMemo(
    () => backups.filter((b) => b.spec?.site === frappeSiteName).sort(byNewest),
    [backups, frappeSiteName],
  );
  const siteRestores = React.useMemo(
    () => restores.filter((r) => r.spec?.site === frappeSiteName).sort(byNewest),
    [restores, frappeSiteName],
  );

  const loaded = migrationsLoaded && backupsLoaded && restoresLoaded && appsLoaded;

  if (!obj) {
    return null;
  }

  const activeCount = [...siteMigrations, ...siteBackups, ...siteRestores, ...siteApps].filter(
    (o) => {
      const phase = (o.status as { phase?: string } | undefined)?.phase;
      return phase && phase !== 'Succeeded' && phase !== 'Failed' && phase !== 'Ready';
    },
  ).length;

  return (
    <div className="co-m-pane__body">
      <Flex
        justifyContent={{ default: 'justifyContentSpaceBetween' }}
        alignItems={{ default: 'alignItemsCenter' }}
        style={{ marginBottom: '16px' }}
      >
        <FlexItem>
          <Title headingLevel="h2" size="xl">
            Operations
          </Title>
        </FlexItem>
        <FlexItem>
          <Flex spaceItems={{ default: 'spaceItemsSm' }}>
            <FlexItem>
              <Button variant="primary" onClick={() => launchModal(MigrateSiteModal, { site: obj })}>
                Migrate
              </Button>
            </FlexItem>
            <FlexItem>
              <Button
                variant="secondary"
                onClick={() => launchModal(InstallAppModal, { site: obj })}
              >
                Install app
              </Button>
            </FlexItem>
            <FlexItem>
              <Button
                variant="secondary"
                onClick={() => launchModal(BackupSiteModal, { site: obj })}
              >
                Back up
              </Button>
            </FlexItem>
            <FlexItem>
              <Button
                variant="secondary"
                onClick={() => launchModal(RestoreSiteModal, { site: obj })}
              >
                Restore
              </Button>
            </FlexItem>
          </Flex>
        </FlexItem>
      </Flex>

      {activeCount > 0 && (
        <Alert
          variant="info"
          isInline
          title={`${activeCount} operation${activeCount === 1 ? '' : 's'} in progress`}
          style={{ marginBottom: '16px' }}
        >
          The operator serialises the underlying Jobs, so queued work starts as earlier operations
          finish.
        </Alert>
      )}

      {/* ── Migrations ────────────────────────────────────────── */}
      <OperationCard
        title="Migrations"
        action={
          <Button
            variant="link"
            isInline
            onClick={() => launchModal(MigrateSiteModal, { site: obj })}
          >
            Migrate site
          </Button>
        }
        headers={['Name', 'Phase', 'Job', 'Pre-migration backup', 'Started', 'Completed']}
        isEmpty={loaded && siteMigrations.length === 0}
        emptyText="This site has never been migrated. Run bench migrate after upgrading its apps."
      >
        {siteMigrations.map((m) => (
          <Tr key={m.metadata?.uid}>
            <Td dataLabel="Name">
              <ResourceLink
                groupVersionKind={SiteMigrationGVK}
                name={m.metadata?.name}
                namespace={m.metadata?.namespace}
                inline
              />
            </Td>
            <Td dataLabel="Phase">
              <PhaseLabel phase={m.status?.phase} />
            </Td>
            <Td dataLabel="Job">
              {m.status?.jobName ? (
                <ResourceLink
                  kind="Job"
                  name={m.status.jobName}
                  namespace={m.metadata?.namespace}
                  inline
                />
              ) : (
                <Muted>—</Muted>
              )}
            </Td>
            <Td dataLabel="Pre-migration backup">
              {m.status?.preBackupRef ? (
                <ResourceLink
                  groupVersionKind={SiteBackupGVK}
                  name={m.status.preBackupRef}
                  namespace={m.metadata?.namespace}
                  inline
                />
              ) : (
                <Muted>{m.spec?.backupBeforeMigrate === false ? 'Skipped' : '—'}</Muted>
              )}
            </Td>
            <Td dataLabel="Started">
              {m.status?.startTime ? <Timestamp timestamp={m.status.startTime} /> : <Muted>—</Muted>}
            </Td>
            <Td dataLabel="Completed">
              {m.status?.completionTime ? (
                <Timestamp timestamp={m.status.completionTime} />
              ) : (
                <Muted>—</Muted>
              )}
            </Td>
          </Tr>
        ))}
      </OperationCard>

      {/* ── Apps ──────────────────────────────────────────────── */}
      <OperationCard
        title="App installs"
        action={
          <Button variant="link" isInline onClick={() => launchModal(InstallAppModal, { site: obj })}>
            Install app
          </Button>
        }
        headers={['Name', 'App', 'Phase', 'Version', 'Source', 'Created']}
        isEmpty={loaded && siteApps.length === 0}
        emptyText="No apps have been installed onto this site through a SiteApp."
      >
        {siteApps.map((a) => (
          <Tr key={a.metadata?.uid}>
            <Td dataLabel="Name">
              <ResourceLink
                groupVersionKind={SiteAppGVK}
                name={a.metadata?.name}
                namespace={a.metadata?.namespace}
                inline
              />
            </Td>
            <Td dataLabel="App">
              <Label isCompact color="blue">
                {a.spec?.appName}
              </Label>
            </Td>
            <Td dataLabel="Phase">
              <PhaseLabel phase={a.status?.phase} />
            </Td>
            <Td dataLabel="Version">
              {a.status?.installedVersion ? (
                <code>{a.status.installedVersion}</code>
              ) : (
                <Muted>—</Muted>
              )}
            </Td>
            <Td dataLabel="Source">
              {a.spec?.fpmPackage ? (
                <code>{a.spec.fpmPackage}</code>
              ) : a.spec?.gitRepo ? (
                <code>
                  {a.spec.gitRepo}
                  {a.spec.gitBranch ? `@${a.spec.gitBranch}` : ''}
                </code>
              ) : (
                <Muted>—</Muted>
              )}
            </Td>
            <Td dataLabel="Created">
              {a.metadata?.creationTimestamp ? (
                <Timestamp timestamp={a.metadata.creationTimestamp} />
              ) : (
                <Muted>—</Muted>
              )}
            </Td>
          </Tr>
        ))}
      </OperationCard>

      {/* ── Backups ───────────────────────────────────────────── */}
      <OperationCard
        title="Backups"
        action={
          <Button variant="link" isInline onClick={() => launchModal(BackupSiteModal, { site: obj })}>
            Back up site
          </Button>
        }
        headers={['Name', 'Phase', 'Schedule', 'Contents', 'Location', 'Last backup']}
        isEmpty={loaded && siteBackups.length === 0}
        emptyText="No backups exist for this site."
      >
        {siteBackups.map((b) => (
          <Tr key={b.metadata?.uid}>
            <Td dataLabel="Name">
              <ResourceLink
                groupVersionKind={SiteBackupGVK}
                name={b.metadata?.name}
                namespace={b.metadata?.namespace}
                inline
              />
            </Td>
            <Td dataLabel="Phase">
              <PhaseLabel phase={b.status?.phase} />
            </Td>
            <Td dataLabel="Schedule">
              {b.spec?.schedule ? <code>{b.spec.schedule}</code> : <Muted>One-off</Muted>}
            </Td>
            <Td dataLabel="Contents">
              <Label isCompact color={b.spec?.withFiles ? 'green' : 'grey'}>
                {b.spec?.withFiles ? 'Database + files' : 'Database only'}
              </Label>
            </Td>
            <Td dataLabel="Location">
              {b.status?.storageLocation ? (
                <code>{b.status.storageLocation}</code>
              ) : (
                <Muted>Bench volume</Muted>
              )}
            </Td>
            <Td dataLabel="Last backup">
              {b.status?.lastBackup ? (
                <Timestamp timestamp={b.status.lastBackup} />
              ) : (
                <Muted>—</Muted>
              )}
            </Td>
          </Tr>
        ))}
      </OperationCard>

      {/* ── Restores ──────────────────────────────────────────── */}
      <OperationCard
        title="Restores"
        action={
          <Button
            variant="link"
            isInline
            onClick={() => launchModal(RestoreSiteModal, { site: obj })}
          >
            Restore site
          </Button>
        }
        headers={['Name', 'Phase', 'Source', 'Message', 'Completed']}
        isEmpty={loaded && siteRestores.length === 0}
        emptyText="This site has never been restored from a backup."
      >
        {siteRestores.map((r) => (
          <Tr key={r.metadata?.uid}>
            <Td dataLabel="Name">
              <ResourceLink
                groupVersionKind={SiteRestoreGVK}
                name={r.metadata?.name}
                namespace={r.metadata?.namespace}
                inline
              />
            </Td>
            <Td dataLabel="Phase">
              <PhaseLabel phase={r.status?.phase} />
            </Td>
            <Td dataLabel="Source">
              {r.spec?.databaseBackupSource?.localPath ? (
                <code>{r.spec.databaseBackupSource.localPath}</code>
              ) : r.spec?.databaseBackupSource?.s3 ? (
                <Label isCompact color="orange">
                  S3
                </Label>
              ) : (
                <Muted>—</Muted>
              )}
            </Td>
            <Td dataLabel="Message">{r.status?.message ?? <Muted>—</Muted>}</Td>
            <Td dataLabel="Completed">
              {r.status?.completionTime ? (
                <Timestamp timestamp={r.status.completionTime} />
              ) : (
                <Muted>—</Muted>
              )}
            </Td>
          </Tr>
        ))}
      </OperationCard>
    </div>
  );
};

/**
 * The console renders this through the extension's `$codeRef`, so the themed
 * wrapper has to be the default export — it is what puts the plugin's bundled
 * PatternFly styles and theme scope around the tree.
 */
const SiteOperationsTabPage: React.FC<React.ComponentProps<typeof SiteOperationsTab>> = (props) => (
  <PluginRoot>
    <SiteOperationsTab {...props} />
  </PluginRoot>
);

export default SiteOperationsTabPage;
