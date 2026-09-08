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
import { Globe, Database, ExternalLink, Plus, CheckCircle2, Clock, AlertTriangle, ShieldCheck } from 'lucide-react';

export interface FrappeSiteResource {
  name: string;
  namespace: string;
  benchName: string;
  domain: string;
  siteURL: string;
  phase: 'Ready' | 'Provisioning' | 'Failed';
  dbProvider: 'postgres' | 'mariadb' | 'external';
  dbEngine?: 'stackgres' | 'percona';
  dbMode: 'dedicated' | 'shared';
  tlsEnabled: boolean;
}

export const SitesDirectory: React.FC = () => {
  const [filter, setFilter] = useState('');

  const sites: FrappeSiteResource[] = [
    {
      name: 'prod-customer1',
      namespace: 'frappe-system',
      benchName: 'production-bench',
      domain: 'customer1.myplatform.com',
      siteURL: 'https://customer1.myplatform.com',
      phase: 'Ready',
      dbProvider: 'postgres',
      dbEngine: 'stackgres',
      dbMode: 'dedicated',
      tlsEnabled: true,
    },
    {
      name: 'prod-customer2',
      namespace: 'frappe-system',
      benchName: 'production-bench',
      domain: 'erp.customer2.corp',
      siteURL: 'https://erp.customer2.corp',
      phase: 'Ready',
      dbProvider: 'postgres',
      dbEngine: 'percona',
      dbMode: 'dedicated',
      tlsEnabled: true,
    },
    {
      name: 'staging-tenant',
      namespace: 'frappe-staging',
      benchName: 'staging-bench',
      domain: 'stage.myplatform.com',
      siteURL: 'https://stage.myplatform.com',
      phase: 'Provisioning',
      dbProvider: 'postgres',
      dbEngine: 'stackgres',
      dbMode: 'shared',
      tlsEnabled: true,
    },
  ];

  const filtered = sites.filter(
    (s) =>
      s.name.toLowerCase().includes(filter.toLowerCase()) ||
      s.domain.toLowerCase().includes(filter.toLowerCase()) ||
      s.benchName.toLowerCase().includes(filter.toLowerCase())
  );

  return (
    <PageSection className="frappe-plugin-page">
      {/* Header Container */}
      <div className="frappe-header-container">
        <div>
          <Title headingLevel="h1" size="2xl" className="frappe-header-title">
            <Globe color="#0066CC" size={30} />
            Tenant Sites Directory
          </Title>
          <div className="frappe-header-subtitle">
            Enterprise multi-tenant Frappe & ERPNext tenant sites with automated database provisioning and TLS routing.
          </div>
        </div>
        <a
          href="/k8s/all-namespaces/vyogo.tech~v1~FrappeSite/~new"
          className="frappe-btn-primary"
        >
          <Plus size={16} />
          Create FrappeSite
        </a>
      </div>

      {/* KPI Cards Grid */}
      <div className="frappe-metrics-grid">
        <Card className="frappe-stat-card">
          <div className="frappe-stat-title">
            <Globe size={18} color="#0066CC" />
            Total Tenant Sites
          </div>
          <div className="frappe-stat-number primary">{sites.length}</div>
        </Card>

        <Card className="frappe-stat-card">
          <div className="frappe-stat-title">
            <Database size={18} color="#3E8635" />
            Dedicated Postgres
          </div>
          <div className="frappe-stat-number success">
            {sites.filter((s) => s.dbMode === 'dedicated').length}
          </div>
        </Card>

        <Card className="frappe-stat-card">
          <div className="frappe-stat-title">
            <ShieldCheck size={18} color="#6A27B8" />
            TLS Encrypted Routes
          </div>
          <div className="frappe-stat-number purple">
            {sites.filter((s) => s.tlsEnabled).length}
          </div>
        </Card>
      </div>

      {/* Table Card */}
      <Card className="frappe-table-card">
        <div className="frappe-toolbar-bar">
          <SearchInput
            placeholder="Filter sites by name, domain, or bench..."
            value={filter}
            onChange={(_e, val) => setFilter(val)}
            onClear={() => setFilter('')}
            style={{ minWidth: 340 }}
          />
          <div style={{ fontSize: 13, color: '#6A6E73' }}>
            Showing {filtered.length} of {sites.length} sites
          </div>
        </div>

        <CardBody style={{ padding: 0 }}>
          <Table aria-label="Frappe Sites Table" variant="compact">
            <Thead>
              <Tr>
                <Th>Site Name</Th>
                <Th>Namespace</Th>
                <Th>Parent Bench</Th>
                <Th>Domain & Ingress</Th>
                <Th>Phase</Th>
                <Th>Database Engine</Th>
                <Th>DB Mode</Th>
                <Th>TLS</Th>
              </Tr>
            </Thead>
            <Tbody>
              {filtered.map((site) => (
                <Tr key={site.name}>
                  <Td dataLabel="Site Name" style={{ fontWeight: 600 }}>
                    <a
                      href={`/k8s/ns/${site.namespace}/vyogo.tech~v1~FrappeSite/${site.name}`}
                      style={{ color: '#0066CC', textDecoration: 'none', fontWeight: 600 }}
                    >
                      {site.name}
                    </a>
                  </Td>
                  <Td dataLabel="Namespace">
                    <span className="frappe-tag frappe-tag-blue">{site.namespace}</span>
                  </Td>
                  <Td dataLabel="Parent Bench">
                    <a
                      href={`/k8s/ns/${site.namespace}/vyogo.tech~v1~FrappeBench/${site.benchName}`}
                      style={{ color: '#0066CC', textDecoration: 'none' }}
                    >
                      {site.benchName}
                    </a>
                  </Td>
                  <Td dataLabel="Domain & Ingress">
                    <a
                      href={site.siteURL}
                      target="_blank"
                      rel="noopener noreferrer"
                      style={{ display: 'inline-flex', alignItems: 'center', gap: 4, color: '#0066CC', textDecoration: 'none' }}
                    >
                      {site.domain}
                      <ExternalLink size={12} />
                    </a>
                  </Td>
                  <Td dataLabel="Phase">
                    {site.phase === 'Ready' && (
                      <span className="frappe-tag frappe-tag-green">
                        <CheckCircle2 size={13} />
                        Ready
                      </span>
                    )}
                    {site.phase === 'Provisioning' && (
                      <span className="frappe-tag frappe-tag-orange">
                        <Clock size={13} />
                        Provisioning
                      </span>
                    )}
                    {site.phase === 'Failed' && (
                      <span className="frappe-tag" style={{ background: '#fdf2f2', color: '#c9190b', border: '1px solid #f9c6c6' }}>
                        <AlertTriangle size={13} />
                        Failed
                      </span>
                    )}
                  </Td>
                  <Td dataLabel="Database Engine">
                    <span className="frappe-tag frappe-tag-gray">
                      {site.dbProvider} ({site.dbEngine})
                    </span>
                  </Td>
                  <Td dataLabel="DB Mode">
                    <span className={site.dbMode === 'dedicated' ? 'frappe-tag frappe-tag-blue' : 'frappe-tag frappe-tag-gray'}>
                      {site.dbMode}
                    </span>
                  </Td>
                  <Td dataLabel="TLS">
                    {site.tlsEnabled ? (
                      <span className="frappe-tag frappe-tag-green">
                        <ShieldCheck size={13} />
                        Active
                      </span>
                    ) : (
                      <span className="frappe-tag frappe-tag-gray">Disabled</span>
                    )}
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

export default SitesDirectory;
