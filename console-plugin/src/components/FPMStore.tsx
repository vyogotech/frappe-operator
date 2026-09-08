import * as React from 'react';
import {
  ListPageHeader,
  ListPageCreate,
  ListPageBody,
  ListPageFilter,
  VirtualizedTable,
  TableData,
  useListPageFilter,
  useK8sWatchResource,
  ResourceLink,
  Timestamp,
  RowProps,
  TableColumn,
  K8sResourceCommon,
} from '@openshift-console/dynamic-plugin-sdk';
import {
  Label,
  Tabs,
  Tab,
  TabTitleText,
  Alert,
} from '@patternfly/react-core';
import PluginRoot from './PluginRoot';

// ── Types ────────────────────────────────────────────────────────────────────

type FrappeBench = K8sResourceCommon & {
  spec?: {
    fpmConfig?: {
      repositories?: {
        name: string;
        url?: string;
        priority?: number;
        authSecretRef?: { name: string };
      }[];
      defaultRepo?: string;
    };
  };
  status?: {
    fpmRepositories?: string[];
  };
};

type SiteApp = K8sResourceCommon & {
  spec?: {
    appName: string;
    siteRef: { name: string; namespace?: string };
    fpmPackage?: string;
    fpmRepo?: string;
    fpmRepoType?: string;
    gitRepo?: string;
    gitBranch?: string;
  };
  status?: {
    phase?: string;
    installedVersion?: string;
  };
};

interface AggregatedFPMRepo {
  name: string;
  url: string;
  priority: number;
  scope: string;
  authSecret?: string;
}

// ── FPM Repositories Table Columns ──────────────────────────────────────────

const repoColumns: TableColumn<AggregatedFPMRepo>[] = [
  { title: 'Repository Name', id: 'name' },
  { title: 'Endpoint URL',    id: 'url' },
  { title: 'Priority',        id: 'priority' },
  { title: 'Scope',           id: 'scope' },
  { title: 'Auth Secret',     id: 'authSecret' },
  { title: 'Status',          id: 'status' },
];

const AggregatedRepoRow: React.FC<RowProps<AggregatedFPMRepo>> = ({ obj, activeColumnIDs }) => (
  <>
    <TableData id="name" activeColumnIDs={activeColumnIDs}>
      <strong>{obj.name}</strong>
    </TableData>
    <TableData id="url" activeColumnIDs={activeColumnIDs}>
      <code>{obj.url}</code>
    </TableData>
    <TableData id="priority" activeColumnIDs={activeColumnIDs}>
      <Label isCompact color="grey">{obj.priority}</Label>
    </TableData>
    <TableData id="scope" activeColumnIDs={activeColumnIDs}>
      <Label isCompact color={obj.scope.startsWith('Operator') ? 'blue' : 'purple'}>
        {obj.scope}
      </Label>
    </TableData>
    <TableData id="authSecret" activeColumnIDs={activeColumnIDs}>
      {obj.authSecret ? (
        <ResourceLink kind="Secret" name={obj.authSecret} />
      ) : (
        <Label isCompact color="grey">Public</Label>
      )}
    </TableData>
    <TableData id="status" activeColumnIDs={activeColumnIDs}>
      <Label isCompact color="green">Configured</Label>
    </TableData>
  </>
);

// ── SiteApp Table Columns ───────────────────────────────────────────────────

const appColumns: TableColumn<SiteApp>[] = [
  { title: 'Name',             id: 'name' },
  { title: 'Namespace',        id: 'namespace' },
  { title: 'Application',      id: 'appName' },
  { title: 'Target Site',      id: 'siteRef' },
  { title: 'Package / Source', id: 'source' },
  { title: 'Phase',            id: 'phase' },
  { title: 'Version',          id: 'version' },
  { title: 'Created',          id: 'created' },
];

const SiteAppRow: React.FC<RowProps<SiteApp>> = ({ obj, activeColumnIDs }) => {
  const phase = obj.status?.phase;
  const phaseColor =
    phase === 'Ready'
      ? 'green'
      : phase === 'Failed'
      ? 'red'
      : phase === 'Installing' || phase === 'Pending'
      ? 'blue'
      : 'orange';

  const sourceDesc = obj.spec?.fpmPackage
    ? `${obj.spec.fpmPackage} (FPM)`
    : obj.spec?.gitRepo
    ? `${obj.spec.gitRepo} (Git)`
    : '—';

  return (
    <>
      <TableData id="name" activeColumnIDs={activeColumnIDs}>
        <ResourceLink
          groupVersionKind={{ group: 'vyogo.tech', version: 'v1', kind: 'SiteApp' }}
          name={obj.metadata!.name}
          namespace={obj.metadata!.namespace}
        />
      </TableData>

      <TableData id="namespace" activeColumnIDs={activeColumnIDs}>
        <ResourceLink kind="Namespace" name={obj.metadata!.namespace} />
      </TableData>

      <TableData id="appName" activeColumnIDs={activeColumnIDs}>
        <strong>{obj.spec?.appName ?? '—'}</strong>
      </TableData>

      <TableData id="siteRef" activeColumnIDs={activeColumnIDs}>
        {obj.spec?.siteRef?.name ? (
          <ResourceLink
            groupVersionKind={{ group: 'vyogo.tech', version: 'v1', kind: 'FrappeSite' }}
            name={obj.spec.siteRef.name}
            namespace={obj.spec.siteRef.namespace ?? obj.metadata!.namespace}
          />
        ) : (
          '—'
        )}
      </TableData>

      <TableData id="source" activeColumnIDs={activeColumnIDs}>
        <code>{sourceDesc}</code>
      </TableData>

      <TableData id="phase" activeColumnIDs={activeColumnIDs}>
        {phase ? (
          <Label isCompact color={phaseColor as 'green' | 'red' | 'blue' | 'orange'}>
            {phase}
          </Label>
        ) : (
          '—'
        )}
      </TableData>

      <TableData id="version" activeColumnIDs={activeColumnIDs}>
        {obj.status?.installedVersion ? (
          <Label isCompact color="purple">{obj.status.installedVersion}</Label>
        ) : (
          '—'
        )}
      </TableData>

      <TableData id="created" activeColumnIDs={activeColumnIDs}>
        <Timestamp timestamp={obj.metadata!.creationTimestamp!} />
      </TableData>
    </>
  );
};

// ── Main Page ────────────────────────────────────────────────────────────────

export const FPMStore: React.FC = () => {
  const [activeTab, setActiveTab] = React.useState<string | number>(0);

  // 1. Watch FrappeBench to aggregate FPM repositories
  const [benches, benchesLoaded, benchesLoadError] = useK8sWatchResource<FrappeBench[]>({
    groupVersionKind: { group: 'vyogo.tech', version: 'v1', kind: 'FrappeBench' },
    isList: true,
  });

  // 2. Watch SiteApp to list installed applications
  const [apps, appsLoaded, appsLoadError] = useK8sWatchResource<SiteApp[]>({
    groupVersionKind: { group: 'vyogo.tech', version: 'v1', kind: 'SiteApp' },
    isList: true,
  });

  const [staticData, filteredData, onFilterChange] = useListPageFilter(apps ?? []);

  // Aggregate repositories from operator defaults and bench specs
  const aggregatedRepos = React.useMemo<AggregatedFPMRepo[]>(() => {
    const list: AggregatedFPMRepo[] = [
      {
        name: 'vyogo-official',
        url: 'https://fpm.vyogo.tech',
        priority: 100,
        scope: 'Operator Global',
      },
    ];

    if (benches) {
      for (const bench of benches) {
        const benchRepos = bench.spec?.fpmConfig?.repositories ?? [];
        for (const r of benchRepos) {
          list.push({
            name: r.name,
            url: r.url ?? '—',
            priority: r.priority ?? 50,
            scope: `Bench: ${bench.metadata?.name ?? 'unknown'}`,
            authSecret: r.authSecretRef?.name,
          });
        }
      }
    }
    return list;
  }, [benches]);

  return (
    <>
      <ListPageHeader title="FPM Package Store & Repositories">
        <ListPageCreate groupVersionKind={{ group: 'vyogo.tech', version: 'v1', kind: 'SiteApp' }}>
          Install Application
        </ListPageCreate>
      </ListPageHeader>

      <ListPageBody>
        <Tabs
          activeKey={activeTab}
          onSelect={(_e, tabIndex) => setActiveTab(tabIndex)}
          usePageInsets
          style={{ marginBottom: '16px' }}
        >
          <Tab eventKey={0} title={<TabTitleText>Configured Repositories</TabTitleText>} />
          <Tab eventKey={1} title={<TabTitleText>Site Applications ({apps?.length ?? 0})</TabTitleText>} />
        </Tabs>

        {activeTab === 0 && (
          <div>
            <Alert
              variant="info"
              isInline
              title="About Frappe Package Manager (FPM) Repositories"
              style={{ marginBottom: '16px' }}
            >
              FPM repositories function similar to Helm Chart Repositories or OCI registries. They host
              pre-compiled Frappe applications and wheels for air-gapped environments without egress network
              requirements. Repositories can be registered globally via operator configuration or scoped
              directly inside a <code>FrappeBench</code> custom resource (<code>spec.fpmConfig.repositories</code>).
            </Alert>

            <VirtualizedTable<AggregatedFPMRepo>
              data={aggregatedRepos}
              unfilteredData={aggregatedRepos}
              loaded={benchesLoaded}
              loadError={benchesLoadError}
              columns={repoColumns}
              Row={AggregatedRepoRow}
            />
          </div>
        )}

        {activeTab === 1 && (
          <div>
            <ListPageFilter
              data={staticData}
              loaded={appsLoaded}
              onFilterChange={onFilterChange}
            />
            <VirtualizedTable<SiteApp>
              data={filteredData}
              unfilteredData={apps ?? []}
              loaded={appsLoaded}
              loadError={appsLoadError}
              columns={appColumns}
              Row={SiteAppRow}
            />
          </div>
        )}
      </ListPageBody>
    </>
  );
};

/**
 * The console renders this through the extension's `$codeRef`, so the themed
 * wrapper has to be the default export — it is what puts the plugin's bundled
 * PatternFly styles and theme scope around the tree.
 */
const FPMStorePage: React.FC<React.ComponentProps<typeof FPMStore>> = (props) => (
  <PluginRoot>
    <FPMStore {...props} />
  </PluginRoot>
);

export default FPMStorePage;
