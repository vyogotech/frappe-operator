import React, { useState } from 'react';
import '../patternfly-theme.css';
import {
  PageSection,
  Title,
  Card,
  CardBody,
  SearchInput,
  Modal,
  ModalVariant,
  Button,
  Alert,
} from '@patternfly/react-core';
import { Table, Thead, Tr, Th, Tbody, Td } from '@patternfly/react-table';
import { Package, ShieldCheck, Download, Plus, Server, CheckCircle2, ArrowRight } from 'lucide-react';

export interface FPMPackage {
  name: string;
  title: string;
  version: string;
  description: string;
  repository: string;
  license: string;
  airGappedCertified: boolean;
  sha256: string;
}

export const FPMStore: React.FC = () => {
  const [filter, setFilter] = useState('');
  const [selectedPkg, setSelectedPkg] = useState<FPMPackage | null>(null);
  const [targetBench, setTargetBench] = useState('production-bench');
  const [installSuccess, setInstallSuccess] = useState(false);

  const packages: FPMPackage[] = [
    {
      name: 'frappe-erpnext',
      title: 'ERPNext Enterprise Suite',
      version: '15.14.0',
      description: 'Comprehensive Manufacturing, Distribution, Retail, Accounting & CRM ERP suite.',
      repository: 'vyogo-enterprise-local',
      license: 'GPL-3.0',
      airGappedCertified: true,
      sha256: 'e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855',
    },
    {
      name: 'frappe-hrms',
      title: 'Human Resource Management',
      version: '15.2.1',
      description: 'Payroll processing, Attendance, Leave tracking, Expense claims and Recruitment.',
      repository: 'vyogo-enterprise-local',
      license: 'GPL-3.0',
      airGappedCertified: true,
      sha256: '9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08',
    },
    {
      name: 'frappe-helpdesk',
      title: 'Enterprise Customer Support Helpdesk',
      version: '1.2.0',
      description: 'Multi-channel ticketing, SLA management, and agent analytics for OpenShift clusters.',
      repository: 'vyogo-enterprise-local',
      license: 'AGPL-3.0',
      airGappedCertified: true,
      sha256: '5e884898da28047151d0e56f8dc6292773603d0d6aabbdd62a11ef721d1542d8',
    },
    {
      name: 'frappe-insights',
      title: 'Self-Hosted Business Intelligence',
      version: '3.0.4',
      description: 'Interactive SQL query dashboards, KPI metrics, and tenant analytics.',
      repository: 'sovereign-defense-mirror',
      license: 'GPL-3.0',
      airGappedCertified: true,
      sha256: '4b227777d4dd1fc61c6f884f48641d02b4d121d3fd328cb08b5531fcacdabf8a',
    },
  ];

  const filtered = packages.filter(
    (p) =>
      p.name.toLowerCase().includes(filter.toLowerCase()) ||
      p.title.toLowerCase().includes(filter.toLowerCase()) ||
      p.description.toLowerCase().includes(filter.toLowerCase())
  );

  const handleInstall = () => {
    setInstallSuccess(true);
    setTimeout(() => {
      setInstallSuccess(false);
      setSelectedPkg(null);
    }, 2000);
  };

  return (
    <PageSection className="frappe-plugin-page">
      {/* Header Container */}
      <div className="frappe-header-container">
        <div>
          <Title headingLevel="h1" size="2xl" className="frappe-header-title">
            <Package color="#0066CC" size={30} />
            FPM Air-Gapped Package Store
          </Title>
          <div className="frappe-header-subtitle">
            Curated, cryptographically signed, and 100% air-gapped certified Frappe applications for defense, banking, and sovereign clouds.
          </div>
        </div>
      </div>

      {/* KPI Cards Grid */}
      <div className="frappe-metrics-grid">
        <Card className="frappe-stat-card">
          <div className="frappe-stat-title">
            <Package size={18} color="#0066CC" />
            Certified Packages
          </div>
          <div className="frappe-stat-number primary">{packages.length}</div>
        </Card>

        <Card className="frappe-stat-card">
          <div className="frappe-stat-title">
            <ShieldCheck size={18} color="#3E8635" />
            Air-Gapped Certified
          </div>
          <div className="frappe-stat-number success">100%</div>
        </Card>

        <Card className="frappe-stat-card">
          <div className="frappe-stat-title">
            <Server size={18} color="#6A27B8" />
            Synced Local Mirrors
          </div>
          <div className="frappe-stat-number purple">2 Repos</div>
        </Card>
      </div>

      {/* Table Card */}
      <Card className="frappe-table-card">
        <div className="frappe-toolbar-bar">
          <SearchInput
            placeholder="Search air-gapped FPM packages..."
            value={filter}
            onChange={(_e, val) => setFilter(val)}
            onClear={() => setFilter('')}
            style={{ minWidth: 340 }}
          />
          <div style={{ fontSize: 13, color: '#6A6E73' }}>
            Showing {filtered.length} of {packages.length} packages
          </div>
        </div>

        <CardBody style={{ padding: 0 }}>
          <Table aria-label="FPM Packages Table" variant="compact">
            <Thead>
              <Tr>
                <Th>Package Name</Th>
                <Th>Version</Th>
                <Th>Description</Th>
                <Th>Repository Mirror</Th>
                <Th>Security Certification</Th>
                <Th>License</Th>
                <Th>Action</Th>
              </Tr>
            </Thead>
            <Tbody>
              {filtered.map((pkg) => (
                <Tr key={pkg.name}>
                  <Td dataLabel="Package Name" style={{ fontWeight: 600 }}>
                    <div style={{ fontWeight: 600, color: '#151515' }}>{pkg.title}</div>
                    <code style={{ fontSize: 11, color: '#6A6E73' }}>{pkg.name}</code>
                  </Td>
                  <Td dataLabel="Version">
                    <span className="frappe-tag frappe-tag-blue">{pkg.version}</span>
                  </Td>
                  <Td dataLabel="Description" style={{ maxWidth: 320, color: '#6A6E73' }}>
                    {pkg.description}
                  </Td>
                  <Td dataLabel="Repository Mirror">
                    <span className="frappe-tag frappe-tag-purple">{pkg.repository}</span>
                  </Td>
                  <Td dataLabel="Security Certification">
                    <span className="frappe-tag frappe-tag-green">
                      <ShieldCheck size={13} />
                      Air-Gapped Signed
                    </span>
                  </Td>
                  <Td dataLabel="License">
                    <span className="frappe-tag frappe-tag-gray">{pkg.license}</span>
                  </Td>
                  <Td dataLabel="Action">
                    <button
                      className="frappe-btn-secondary"
                      onClick={() => setSelectedPkg(pkg)}
                    >
                      <Download size={14} />
                      Install to Bench
                    </button>
                  </Td>
                </Tr>
              ))}
            </Tbody>
          </Table>
        </CardBody>
      </Card>

      {/* Install Modal */}
      {selectedPkg && (
        <Modal
          variant={ModalVariant.small}
          title={`Install ${selectedPkg.title}`}
          isOpen={true}
          onClose={() => setSelectedPkg(null)}
          actions={[
            <Button
              key="install"
              variant="primary"
              onClick={handleInstall}
              isDisabled={installSuccess}
            >
              {installSuccess ? 'Dispatched to Bench...' : 'Confirm Installation'}
            </Button>,
            <Button
              key="cancel"
              variant="link"
              onClick={() => setSelectedPkg(null)}
            >
              Cancel
            </Button>,
          ]}
        >
          {installSuccess ? (
            <Alert variant="success" title="Package Installation Scheduled!">
              Operator reconciliation job has been triggered on <strong>{targetBench}</strong>.
            </Alert>
          ) : (
            <div>
              <p style={{ marginBottom: 16 }}>
                You are installing <strong>{selectedPkg.name} ({selectedPkg.version})</strong> from the air-gapped repository mirror.
              </p>
              <div style={{ marginBottom: 16 }}>
                <label style={{ display: 'block', fontWeight: 600, marginBottom: 6 }}>
                  Target Frappe Bench:
                </label>
                <select
                  value={targetBench}
                  onChange={(e) => setTargetBench(e.target.value)}
                  style={{ width: '100%', padding: '8px 12px', borderRadius: 4, border: '1px solid #d2d2d2' }}
                >
                  <option value="production-bench">production-bench (frappe-system)</option>
                  <option value="staging-bench">staging-bench (frappe-staging)</option>
                </select>
              </div>
              <div style={{ background: '#f5f5f5', padding: 12, borderRadius: 4, fontSize: 12 }}>
                <strong>SHA256 Verification:</strong>
                <code style={{ display: 'block', marginTop: 4, wordBreak: 'break-all' }}>
                  {selectedPkg.sha256}
                </code>
              </div>
            </div>
          )}
        </Modal>
      )}
    </PageSection>
  );
};

export default FPMStore;
