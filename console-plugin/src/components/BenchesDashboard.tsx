import React, { useState } from 'react';
import '../patternfly-theme.css';
import {
  PageSection,
  Title,
  Card,
  CardBody,
  SearchInput,
} from '@patternfly/react-core';
import { Table, Thead, Tr, Th, Tbody, Td } from '@patternfly/react-table';
import { Server, Layers, Plus, ShieldCheck, CheckCircle2, AlertCircle } from 'lucide-react';

export interface FrappeBenchResource {
  name: string;
  namespace: string;
  frappeVersion: string;
  apps: string[];
  storageSize: string;
  ready: boolean;
  activeSites: number;
  fpmReposCount: number;
}

export const BenchesDashboard: React.FC = () => {
  const [filter, setFilter] = useState('');

  // Sample data reflecting active FrappeBench custom resources
  const benches: FrappeBenchResource[] = [
    {
      name: 'production-bench',
      namespace: 'frappe-system',
      frappeVersion: 'version-15',
      apps: ['frappe', 'erpnext', 'hrms'],
      storageSize: '50Gi',
      ready: true,
      activeSites: 12,
      fpmReposCount: 2,
    },
    {
      name: 'staging-bench',
      namespace: 'frappe-staging',
      frappeVersion: 'version-15',
      apps: ['frappe', 'erpnext'],
      storageSize: '20Gi',
      ready: true,
      activeSites: 3,
      fpmReposCount: 1,
    },
  ];

  const filtered = benches.filter(
    (b) =>
      b.name.toLowerCase().includes(filter.toLowerCase()) ||
      b.namespace.toLowerCase().includes(filter.toLowerCase())
  );

  return (
    <PageSection className="frappe-plugin-page">
      {/* Header Container */}
      <div className="frappe-header-container">
        <div>
          <Title headingLevel="h1" size="2xl" className="frappe-header-title">
            <Server color="#0066CC" size={30} />
            Frappe Benches
          </Title>
          <div className="frappe-header-subtitle">
            Enterprise multi-tenant infrastructure, runtime worker pools, and automated bench lifecycle management for OpenShift.
          </div>
        </div>
        <a
          href="/k8s/all-namespaces/vyogo.tech~v1~FrappeBench/~new"
          className="frappe-btn-primary"
        >
          <Plus size={16} />
          Create FrappeBench
        </a>
      </div>

      {/* KPI Cards Grid */}
      <div className="frappe-metrics-grid">
        <Card className="frappe-stat-card">
          <div className="frappe-stat-title">
            <Layers size={18} color="#0066CC" />
            Total Benches
          </div>
          <div className="frappe-stat-number primary">{benches.length}</div>
        </Card>

        <Card className="frappe-stat-card">
          <div className="frappe-stat-title">
            <Server size={18} color="#3E8635" />
            Active Tenant Sites
          </div>
          <div className="frappe-stat-number success">
            {benches.reduce((acc, b) => acc + b.activeSites, 0)}
          </div>
        </Card>

        <Card className="frappe-stat-card">
          <div className="frappe-stat-title">
            <ShieldCheck size={18} color="#6A27B8" />
            FPM Air-Gapped Repos
          </div>
          <div className="frappe-stat-number purple">
            {benches.reduce((acc, b) => acc + b.fpmReposCount, 0)}
          </div>
        </Card>
      </div>

      {/* Table Card */}
      <Card className="frappe-table-card">
        <div className="frappe-toolbar-bar">
          <SearchInput
            placeholder="Filter benches by name or namespace..."
            value={filter}
            onChange={(_e, val) => setFilter(val)}
            onClear={() => setFilter('')}
            style={{ minWidth: 340 }}
          />
          <div style={{ fontSize: 13, color: '#6A6E73' }}>
            Showing {filtered.length} of {benches.length} benches
          </div>
        </div>

        <CardBody style={{ padding: 0 }}>
          <Table aria-label="Frappe Benches Table" variant="compact">
            <Thead>
              <Tr>
                <Th>Name</Th>
                <Th>Namespace</Th>
                <Th>Version</Th>
                <Th>Status</Th>
                <Th>Apps</Th>
                <Th>Tenant Sites</Th>
                <Th>FPM Repos</Th>
                <Th>Storage</Th>
              </Tr>
            </Thead>
            <Tbody>
              {filtered.map((bench) => (
                <Tr key={bench.name}>
                  <Td dataLabel="Name" style={{ fontWeight: 600 }}>
                    <a
                      href={`/k8s/ns/${bench.namespace}/vyogo.tech~v1~FrappeBench/${bench.name}`}
                      style={{ color: '#0066CC', textDecoration: 'none', fontWeight: 600 }}
                    >
                      {bench.name}
                    </a>
                  </Td>
                  <Td dataLabel="Namespace">
                    <span className="frappe-tag frappe-tag-blue">{bench.namespace}</span>
                  </Td>
                  <Td dataLabel="Version">
                    <code style={{ background: '#f5f5f5', padding: '2px 6px', borderRadius: 4, fontSize: 12 }}>
                      {bench.frappeVersion}
                    </code>
                  </Td>
                  <Td dataLabel="Status">
                    {bench.ready ? (
                      <span className="frappe-tag frappe-tag-green">
                        <CheckCircle2 size={13} />
                        Ready
                      </span>
                    ) : (
                      <span className="frappe-tag frappe-tag-orange">
                        <AlertCircle size={13} />
                        Configuring
                      </span>
                    )}
                  </Td>
                  <Td dataLabel="Apps">
                    <div style={{ display: 'flex', gap: 6, flexWrap: 'wrap' }}>
                      {bench.apps.map((app) => (
                        <span key={app} className="frappe-tag frappe-tag-gray">
                          {app}
                        </span>
                      ))}
                    </div>
                  </Td>
                  <Td dataLabel="Tenant Sites" style={{ fontWeight: 600 }}>
                    {bench.activeSites} sites
                  </Td>
                  <Td dataLabel="FPM Repos">
                    <span className="frappe-tag frappe-tag-purple">
                      {bench.fpmReposCount} Repos
                    </span>
                  </Td>
                  <Td dataLabel="Storage" style={{ color: '#6A6E73' }}>
                    {bench.storageSize}
                  </Td>
                </Tr>
              ))}
            </Tbody>
          </Table>
        </CardBody>
      </Card>
    </PageSection>
  );
};

export default BenchesDashboard;
