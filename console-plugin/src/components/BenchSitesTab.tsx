import * as React from 'react';
import {
  K8sResourceCommon,
  ResourceLink,
  Timestamp,
  useK8sWatchResource,
  VirtualizedTable,
  TableData,
  RowProps,
  TableColumn,
  useListPageFilter,
  ListPageFilter,
  ListPageCreateLink,
} from '@openshift-console/dynamic-plugin-sdk';
import { Label } from '@patternfly/react-core';
import { DatabaseConfig, FrappeBench, FrappeSite } from '../frappe/types';
import { FrappeSiteGVK } from '../frappe/models';
import { benchRefOf, phaseColor, siteNameOf } from '../frappe/utils';
import { DatabaseLabel } from './DataServiceLabels';
import PluginRoot from './PluginRoot';

// ── Types ────────────────────────────────────────────────────────────────────

export interface BenchSitesTabProps {
  obj?: FrappeBench;
}

// ── Columns ──────────────────────────────────────────────────────────────────

const columns: TableColumn<FrappeSite>[] = [
  { title: 'Name',        id: 'name' },
  { title: 'Namespace',   id: 'namespace' },
  { title: 'Domain / URL', id: 'domain' },
  { title: 'Phase',       id: 'phase' },
  { title: 'Database',    id: 'db' },
  { title: 'Installed Apps', id: 'apps' },
  { title: 'Created',     id: 'created' },
];

// ── Row ───────────────────────────────────────────────────────────────────────

/** Bench context, so a site that inherits its database can still show it. */
type BenchRowData = { benchDb?: DatabaseConfig; benchName?: string };

const SiteRow: React.FC<RowProps<FrappeSite, BenchRowData>> = ({
  obj,
  activeColumnIDs,
  rowData,
}) => {
  const { benchDb, benchName } = rowData ?? {};
  const phase = obj.status?.phase;
  const apps = obj.status?.installedApps ?? obj.spec?.apps ?? [];

  return (
    <>
      <TableData id="name" activeColumnIDs={activeColumnIDs}>
        <ResourceLink
          groupVersionKind={FrappeSiteGVK}
          name={obj.metadata!.name}
          namespace={obj.metadata!.namespace}
        />
      </TableData>

      <TableData id="namespace" activeColumnIDs={activeColumnIDs}>
        <ResourceLink kind="Namespace" name={obj.metadata!.namespace} />
      </TableData>

      <TableData id="domain" activeColumnIDs={activeColumnIDs}>
        {obj.status?.siteURL ? (
          <a href={obj.status.siteURL} target="_blank" rel="noopener noreferrer">
            {siteNameOf(obj)}
          </a>
        ) : (
          <code>{siteNameOf(obj) || '—'}</code>
        )}
      </TableData>

      <TableData id="phase" activeColumnIDs={activeColumnIDs}>
        {phase ? (
          <Label isCompact color={phaseColor(phase)}>{phase}</Label>
        ) : (
          '—'
        )}
      </TableData>

      <TableData id="db" activeColumnIDs={activeColumnIDs}>
        <DatabaseLabel
          db={obj.spec?.dbConfig?.provider ? obj.spec.dbConfig : benchDb}
          inheritedFrom={obj.spec?.dbConfig?.provider ? undefined : benchName}
        />
      </TableData>

      <TableData id="apps" activeColumnIDs={activeColumnIDs}>
        {apps.length > 0 ? (
          <div style={{ display: 'flex', gap: '4px', flexWrap: 'wrap' }}>
            {apps.map((app) => (
              <Label key={app} isCompact color="blue">
                {app}
              </Label>
            ))}
          </div>
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

// ── Component ────────────────────────────────────────────────────────────────

export const BenchSitesTab: React.FC<BenchSitesTabProps> = ({ obj }) => {
  const benchName = obj?.metadata?.name;
  const benchNamespace = obj?.metadata?.namespace;

  const [sites, loaded, loadError] = useK8sWatchResource<FrappeSite[]>({
    groupVersionKind: { group: 'vyogo.tech', version: 'v1', kind: 'FrappeSite' },
    isList: true,
    namespace: benchNamespace,
  });

  const matchingSites = React.useMemo(() => {
    if (!sites || !benchName) return [];
    return sites.filter((s) => {
      return benchRefOf(s)?.name === benchName;
    });
  }, [sites, benchName]);

  const [staticData, filteredData, onFilterChange] = useListPageFilter(matchingSites);

  // Route to the plugin's site form with the bench pre-selected, rather than
  // the raw YAML editor.
  const createSiteUrl = `/frappe/sites/~new?bench=${encodeURIComponent(
    benchName ?? '',
  )}&namespace=${encodeURIComponent(benchNamespace ?? '')}`;

  return (
    <div className="co-m-pane__body">
      <div
        style={{
          display: 'flex',
          justifyContent: 'space-between',
          alignItems: 'center',
          marginBottom: '16px',
        }}
      >
        <ListPageCreateLink to={createSiteUrl}>
          Create Site
        </ListPageCreateLink>
      </div>

      <ListPageFilter
        data={staticData}
        loaded={loaded}
        onFilterChange={onFilterChange}
      />

      <VirtualizedTable<FrappeSite, BenchRowData>
        data={filteredData}
        unfilteredData={matchingSites}
        loaded={loaded}
        loadError={loadError}
        columns={columns}
        Row={SiteRow}
        rowData={{ benchDb: obj?.spec?.dbConfig, benchName }}
      />
    </div>
  );
};

/**
 * The console renders this through the extension's `$codeRef`, so the themed
 * wrapper has to be the default export — it is what puts the plugin's bundled
 * PatternFly styles and theme scope around the tree.
 */
const BenchSitesTabPage: React.FC<React.ComponentProps<typeof BenchSitesTab>> = (props) => (
  <PluginRoot>
    <BenchSitesTab {...props} />
  </PluginRoot>
);

export default BenchSitesTabPage;
