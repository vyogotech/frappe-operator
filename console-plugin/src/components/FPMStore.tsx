import React, { useState } from 'react';
import {
  PageSection,
  Title,
  Card,
  CardBody,
  CardTitle,
  CardFooter,
  Label,
  Button,
  Toolbar,
  ToolbarContent,
  ToolbarItem,
  SearchInput,
  Modal,
  ModalVariant,
  Form,
  FormGroup,
  FormSelect,
  FormSelectOption,
  Alert,
} from '@patternfly/react-core';
import { Table, Thead, Tr, Th, Tbody, Td } from '@patternfly/react-table';
import { Package, ShieldCheck, Download, Plus, Server, Check, ArrowRight } from 'lucide-react';

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

export interface FPMRepo {
  name: string;
  url: string;
  priority: number;
  authSecret?: string;
  status: 'Reachable' | 'Syncing' | 'Offline';
}

export const FPMStore: React.FC = () => {
  const [filter, setFilter] = useState('');
  const [selectedPkg, setSelectedPkg] = useState<FPMPackage | null>(null);
  const [targetBench, setTargetBench] = useState('production-bench');
  const [installSuccess, setInstallSuccess] = useState(false);

  const repos: FPMRepo[] = [
    {
      name: 'vyogo-enterprise-local',
      url: 'https://fpm.internal.vyogo.tech',
      priority: 10,
      authSecret: 'fpm-enterprise-token',
      status: 'Reachable',
    },
    {
      name: 'sovereign-defense-mirror',
      url: 'https://nexus.corp.local/repository/fpm-airgapped',
      priority: 20,
      authSecret: 'nexus-basic-auth',
      status: 'Reachable',
    },
  ];

  const packages: FPMPackage[] = [
    {
      name: 'frappe',
      title: 'Frappe Framework Core',
      version: 'v15.22.0',
      description: 'Full-stack web application framework in Python & JavaScript with built-in ORM, REST API, Desk UI.',
      repository: 'vyogo-enterprise-local',
      license: 'MIT',
      airGappedCertified: true,
      sha256: 'a9f8c12...3e1',
    },
    {
      name: 'erpnext',
      title: 'ERPNext Enterprise',
      version: 'v15.20.1',
      description: 'Complete ERP suite: Accounting, HRMS, CRM, Manufacturing, Inventory, and Asset Management.',
      repository: 'vyogo-enterprise-local',
      license: 'GPLv3',
      airGappedCertified: true,
      sha256: 'd41d8cd...9b2',
    },
    {
      name: 'hrms',
      title: 'Frappe HR & Payroll',
      version: 'v15.14.0',
      description: 'Modern human resource management: Payroll calculation, Leave management, Expense claims, and Shift planning.',
      repository: 'sovereign-defense-mirror',
      license: 'GPLv3',
      airGappedCertified: true,
      sha256: '7b52009...c64',
    },
    {
      name: 'payments',
      title: 'Payments Gateway Integrations',
      version: 'v15.3.0',
      description: 'Unified payment gateway adapters for Stripe, PayPal, Razorpay, and sovereign national banking switches.',
      repository: 'sovereign-defense-mirror',
      license: 'MIT',
      airGappedCertified: true,
      sha256: '9a31b40...e81',
    },
  ];

  const filteredPackages = packages.filter(
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
    <PageSection>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 20 }}>
        <div>
          <Title headingLevel="h1" size="2xl" style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
            <Package color="#00BC86" size={28} />
            FPM Air-Gapped Package Store
          </Title>
          <p style={{ color: '#6A6E73', marginTop: 4 }}>
            Zero-egress package distribution for Frappe applications without public PyPI or NPM dependencies.
          </p>
        </div>
        <Button
          variant="secondary"
          icon={<Plus size={16} />}
          style={{ borderColor: '#0050A4', color: '#0050A4' }}
        >
          Add FPM Repository
        </Button>
      </div>

      <Card style={{ marginBottom: 24 }}>
        <CardTitle style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
          <ShieldCheck color="#0050A4" size={20} />
          Configured Sovereign Repositories
        </CardTitle>
        <CardBody>
          <Table aria-label="FPM Repositories" variant="compact">
            <Thead>
              <Tr>
                <Th>Repository Name</Th>
                <Th>Endpoint URL</Th>
                <Th>Priority</Th>
                <Th>Auth Secret</Th>
                <Th>Status</Th>
              </Tr>
            </Thead>
            <Tbody>
              {repos.map((r) => (
                <Tr key={r.name}>
                  <Td dataLabel="Name" style={{ fontWeight: 600 }}>
                    {r.name}
                  </Td>
                  <Td dataLabel="URL">
                    <code>{r.url}</code>
                  </Td>
                  <Td dataLabel="Priority">{r.priority}</Td>
                  <Td dataLabel="Auth Secret">
                    <Label color="blue" isCompact>
                      {r.authSecret || 'None (Public)'}
                    </Label>
                  </Td>
                  <Td dataLabel="Status">
                    <Label color="green">{r.status}</Label>
                  </Td>
                </Tr>
              ))}
            </Tbody>
          </Table>
        </CardBody>
      </Card>

      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 16 }}>
        <Title headingLevel="h2" size="xl">
          Available Application Packages
        </Title>
        <SearchInput
          placeholder="Search package name, title, or description..."
          value={filter}
          onChange={(_e, val) => setFilter(val)}
          onClear={() => setFilter('')}
          style={{ width: 340 }}
        />
      </div>

      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(320px, 1fr))', gap: 16 }}>
        {filteredPackages.map((pkg) => (
          <Card key={pkg.name} isRounded style={{ display: 'flex', flexDirection: 'column' }}>
            <CardTitle style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start' }}>
              <div>
                <div style={{ fontSize: 18, fontWeight: 700, color: '#141917' }}>{pkg.title}</div>
                <code style={{ fontSize: 13, color: '#0050A4' }}>{pkg.name} @ {pkg.version}</code>
              </div>
              <Label color="green" icon={<Check size={12} />} isCompact>
                Air-Gapped
              </Label>
            </CardTitle>
            <CardBody style={{ flexGrow: 1 }}>
              <p style={{ color: '#4A5568', fontSize: 14 }}>{pkg.description}</p>
              <div style={{ marginTop: 12, display: 'flex', gap: 8, flexWrap: 'wrap' }}>
                <Label color="purple" isCompact>
                  Repo: {pkg.repository}
                </Label>
                <Label color="grey" isCompact>
                  License: {pkg.license}
                </Label>
              </div>
            </CardBody>
            <CardFooter>
              <Button
                variant="primary"
                isBlock
                icon={<Download size={16} />}
                onClick={() => setSelectedPkg(pkg)}
                style={{ backgroundColor: '#00BC86', borderColor: '#00BC86', color: '#0B1411', fontWeight: 600 }}
              >
                Install to Bench
              </Button>
            </CardFooter>
          </Card>
        ))}
      </div>

      {selectedPkg && (
        <Modal
          variant={ModalVariant.medium}
          title={`Install ${selectedPkg.title} to Bench`}
          isOpen={true}
          onClose={() => setSelectedPkg(null)}
          actions={[
            <Button
              key="confirm"
              variant="primary"
              onClick={handleInstall}
              style={{ backgroundColor: '#0050A4', borderColor: '#0050A4' }}
            >
              Confirm Installation
            </Button>,
            <Button key="cancel" variant="link" onClick={() => setSelectedPkg(null)}>
              Cancel
            </Button>,
          ]}
        >
          {installSuccess ? (
            <Alert variant="success" title="Installation Dispatched Successfully!">
              Declarative patch applied to FrappeBench <code>{targetBench}</code>. The operator initialization job is
              unpacking the pre-compiled FPM package without egress network access.
            </Alert>
          ) : (
            <Form>
              <p>
                This will declaratively append <strong>{selectedPkg.name} ({selectedPkg.version})</strong> to the target
                FrappeBench. The operator will fetch the artifact from the internal sovereign FPM repository.
              </p>
              <FormGroup label="Target FrappeBench" fieldId="target-bench" isRequired>
                <FormSelect
                  value={targetBench}
                  onChange={(_e, val) => setTargetBench(val)}
                  id="target-bench"
                  aria-label="Target Bench"
                >
                  <FormSelectOption value="production-bench" label="production-bench (frappe-system)" />
                  <FormSelectOption value="staging-bench" label="staging-bench (frappe-staging)" />
                </FormSelect>
              </FormGroup>
            </Form>
          )}
        </Modal>
      )}
    </PageSection>
  );
};

export default FPMStore;
