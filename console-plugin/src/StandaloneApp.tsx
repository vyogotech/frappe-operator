import React, { useState } from 'react';
import {
  Page,
  Masthead,
  MastheadMain,
  MastheadBrand,
  MastheadContent,
  Toolbar,
  ToolbarContent,
  ToolbarItem,
  Tabs,
  Tab,
  TabTitleText,
  TabTitleIcon,
  Label,
  Badge,
} from '@patternfly/react-core';
import { Server, Globe, Package, ShieldCheck, Layers, Terminal } from 'lucide-react';
import BenchesDashboard from './components/BenchesDashboard';
import SitesDirectory from './components/SitesDirectory';
import FPMStore from './components/FPMStore';
import PluginRoot from './components/PluginRoot';

export const StandaloneApp: React.FC = () => {
  const [activeTab, setActiveTab] = useState<string | number>('benches');

  return (
    <Page
      masthead={
        <Masthead
          style={{
            background: '#0B1411',
            borderBottom: '2px solid #00BC86',
            padding: '8px 24px',
          }}
        >
          <MastheadMain>
            <MastheadBrand style={{ textDecoration: 'none', display: 'flex', alignItems: 'center', gap: 12 }}>
              <div
                style={{
                  width: 32,
                  height: 32,
                  borderRadius: 6,
                  backgroundColor: '#00BC86',
                  display: 'flex',
                  alignItems: 'center',
                  justifyContent: 'center',
                  color: '#0B1411',
                  fontWeight: 900,
                  fontSize: 18,
                }}
              >
                V
              </div>
              <span style={{ fontSize: 18, fontWeight: 700, color: '#FFFFFF', letterSpacing: -0.3 }}>
                Frappe Platform Console
              </span>
              <Badge style={{ backgroundColor: '#0050A4', color: '#FFFFFF', marginLeft: 4 }}>
                v5.2.0
              </Badge>
            </MastheadBrand>
          </MastheadMain>

          <MastheadContent>
            <Toolbar id="toolbar" isFullHeight isStatic>
              <ToolbarContent>
                <ToolbarItem align={{ default: 'alignEnd' }}>
                  <div style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
                    <Label color="teal" icon={<ShieldCheck size={14} />}>
                      Air-Gapped Ready
                    </Label>
                    <Label color="blue" icon={<Terminal size={14} />}>
                      Kubernetes & OpenShift
                    </Label>
                  </div>
                </ToolbarItem>
              </ToolbarContent>
            </Toolbar>
          </MastheadContent>
        </Masthead>
      }
    >
      <div style={{ background: '#FFFFFF', borderBottom: '1px solid #D2D2D2', padding: '0 24px' }}>
        <Tabs
          activeKey={activeTab}
          onSelect={(_e, tabIndex) => setActiveTab(tabIndex)}
          aria-label="Frappe Console Navigation Tabs"
          usePageInsets
        >
          <Tab
            eventKey="benches"
            title={
              <>
                <TabTitleIcon>
                  <Server size={16} color={activeTab === 'benches' ? '#00BC86' : '#6A6E73'} />
                </TabTitleIcon>
                <TabTitleText>Frappe Benches</TabTitleText>
              </>
            }
          />
          <Tab
            eventKey="sites"
            title={
              <>
                <TabTitleIcon>
                  <Globe size={16} color={activeTab === 'sites' ? '#0050A4' : '#6A6E73'} />
                </TabTitleIcon>
                <TabTitleText>Tenant Sites</TabTitleText>
              </>
            }
          />
          <Tab
            eventKey="fpm"
            title={
              <>
                <TabTitleIcon>
                  <Package size={16} color={activeTab === 'fpm' ? '#00BC86' : '#6A6E73'} />
                </TabTitleIcon>
                <TabTitleText>FPM Air-Gapped Store</TabTitleText>
              </>
            }
          />
        </Tabs>
      </div>

      <main style={{ backgroundColor: '#F0F2F5', minHeight: 'calc(100vh - 120px)' }}>
        {activeTab === 'benches' && <BenchesDashboard />}
        {activeTab === 'sites' && <SitesDirectory />}
        {activeTab === 'fpm' && <FPMStore />}
      </main>
    </Page>
  );
};

const StandaloneAppRoot: React.FC = () => (
  <PluginRoot>
    <StandaloneApp />
  </PluginRoot>
);

export default StandaloneAppRoot;
