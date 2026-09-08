import React from 'react';
import ReactDOM from 'react-dom/client';
import StandaloneApp from './StandaloneApp';

const rootElement = document.getElementById('root');

if (rootElement) {
  const root = ReactDOM.createRoot(rootElement);
  root.render(
    <React.StrictMode>
      <StandaloneApp />
    </React.StrictMode>
  );
}

// Export individual modules for OpenShift Dynamic Console Plugin
export { default as BenchesDashboard } from './components/BenchesDashboard';
export { default as SitesDirectory } from './components/SitesDirectory';
export { default as FPMStore } from './components/FPMStore';
export { default as BenchFPMTab } from './components/BenchFPMTab';
export { default as SiteDatabaseTab } from './components/SiteDatabaseTab';
