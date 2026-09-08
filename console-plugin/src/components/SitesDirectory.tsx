import * as React from 'react';
import {
  ListPageHeader,
  ListPageCreateDropdown,
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
  useActiveNamespace,
} from '@openshift-console/dynamic-plugin-sdk';
import { Label } from '@patternfly/react-core';
import { FrappeBenchGVK, FrappeSiteGVK } from '../frappe/models';
import { FrappeBench, FrappeSite } from '../frappe/types';
import {
  benchRefOf,
  createYAMLPath,
  navigateTo,
  phaseColor,
  siteNameOf,
} from '../frappe/utils';
import { DatabaseLabel } from './DataServiceLabels';
import PluginRoot from './PluginRoot';

// ── Columns ──────────────────────────────────────────────────────────────────

const columns: TableColumn<FrappeSite>[] = [
  { title: 'Name',         id: 'name' },
  { title: 'Namespace',    id: 'namespace' },
  { title: 'Parent Bench', id: 'bench' },
  { title: 'Site / URL',   id: 'domain' },
  { title: 'Phase',        id: 'phase' },
  { title: 'Database',     id: 'db' },
  { title: 'Apps',         id: 'apps' },
  { title: 'Route',        id: 'route' },
  { title: 'Created',      id: 'created' },
];

type SiteRowData = { benchesByName: Record<string, FrappeBench> };

// ── Row ───────────────────────────────────────────────────────────────────────

const SiteRow: React.FC<RowProps<FrappeSite, SiteRowData>> = ({
  obj,
  activeColumnIDs,
  rowData,
}) => {
  const phase = obj.status?.phase;
  const benchRef = benchRefOf(obj);
  const bench = benchRef ? rowData?.benchesByName?.[benchRef.name] : undefined;
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

      <TableData id="bench" activeColumnIDs={activeColumnIDs}>
        {benchRef ? (
          <ResourceLink
            groupVersionKind={FrappeBenchGVK}
            name={benchRef.name}
            namespace={benchRef.namespace}
          />
        ) : '—'}
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
        ) : '—'}
      </TableData>

      <TableData id="db" activeColumnIDs={activeColumnIDs}>
        {/* A site without its own dbConfig runs on whatever the bench declares,
            so resolve through the bench and mark the value as inherited. */}
        <DatabaseLabel
          db={obj.spec?.dbConfig?.provider ? obj.spec.dbConfig : bench?.spec?.dbConfig}
          inheritedFrom={obj.spec?.dbConfig?.provider ? undefined : benchRef?.name}
        />
      </TableData>

      <TableData id="apps" activeColumnIDs={activeColumnIDs}>
        <div style={{ display: 'flex', gap: '4px', flexWrap: 'wrap' }}>
          {apps.map((app) => (
            <Label key={app} isCompact color="blue">{app}</Label>
          ))}
          {apps.length === 0 && '—'}
        </div>
      </TableData>

      <TableData id="route" activeColumnIDs={activeColumnIDs}>
        {obj.spec?.routeConfig?.enabled === false ? (
          <Label isCompact color="grey">Disabled</Label>
        ) : (
          <Label isCompact color="green">
            {obj.spec?.routeConfig?.tlsTermination ?? 'edge'}
          </Label>
        )}
      </TableData>

      <TableData id="created" activeColumnIDs={activeColumnIDs}>
        <Timestamp timestamp={obj.metadata!.creationTimestamp!} />
      </TableData>
    </>
  );
};

// ── Page ─────────────────────────────────────────────────────────────────────

export const SitesDirectory: React.FC = () => {
  const [activeNamespace] = useActiveNamespace();

  const [sites, loaded, loadError] = useK8sWatchResource<FrappeSite[]>({
    groupVersionKind: FrappeSiteGVK,
    isList: true,
    namespaced: false,
  });

  // Benches are watched so a site that inherits its database configuration can
  // still show what it actually runs on.
  const [benches] = useK8sWatchResource<FrappeBench[]>({
    groupVersionKind: FrappeBenchGVK,
    isList: true,
    namespaced: false,
  });

  const benchesByName = React.useMemo(
    () =>
      (benches ?? []).reduce<Record<string, FrappeBench>>((acc, b) => {
        if (b.metadata?.name) {
          acc[b.metadata.name] = b;
        }
        return acc;
      }, {}),
    [benches],
  );

  const [staticData, filteredData, onFilterChange] = useListPageFilter(sites ?? []);

  const namespace = activeNamespace !== '#ALL_NS#' ? activeNamespace : undefined;

  const onCreate = React.useCallback(
    (item: string) => {
      navigateTo(
        item === 'form'
          ? `/frappe/sites/~new${namespace ? `?namespace=${encodeURIComponent(namespace)}` : ''}`
          : createYAMLPath(FrappeSiteGVK, namespace),
      );
    },
    [namespace],
  );

  return (
    <>
      <ListPageHeader title="Tenant Sites">
        <ListPageCreateDropdown
          items={{ form: 'With Form', yaml: 'With YAML' }}
          onClick={onCreate}
          createAccessReview={{ groupVersionKind: FrappeSiteGVK, namespace }}
        >
          Create Site
        </ListPageCreateDropdown>
      </ListPageHeader>

      <ListPageBody>
        <ListPageFilter
          data={staticData}
          loaded={loaded}
          onFilterChange={onFilterChange}
        />
        <VirtualizedTable<FrappeSite, SiteRowData>
          data={filteredData}
          unfilteredData={sites ?? []}
          loaded={loaded}
          loadError={loadError}
          columns={columns}
          Row={SiteRow}
          rowData={{ benchesByName }}
        />
      </ListPageBody>
    </>
  );
};

/**
 * The console renders this through the extension's `$codeRef`, so the themed
 * wrapper has to be the default export — it is what puts the plugin's bundled
 * PatternFly styles and theme scope around the tree.
 */
const SitesDirectoryPage: React.FC<React.ComponentProps<typeof SitesDirectory>> = (props) => (
  <PluginRoot>
    <SitesDirectory {...props} />
  </PluginRoot>
);

export default SitesDirectoryPage;
