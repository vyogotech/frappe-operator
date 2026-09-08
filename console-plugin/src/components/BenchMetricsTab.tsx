import * as React from 'react';
import {
  K8sResourceCommon,
  ResourceLink,
  QueryBrowser,
  usePrometheusPoll,
  PrometheusEndpoint,
} from '@openshift-console/dynamic-plugin-sdk';
import { Spinner, Button } from '@patternfly/react-core';
import PluginRoot from './PluginRoot';

// ── Types ────────────────────────────────────────────────────────────────────

export interface BenchMetricsTabProps {
  obj?: K8sResourceCommon & {
    spec?: {
      frappeVersion?: string;
    };
  };
}

interface MetricItem {
  pod: string;
  value: number;
}

// ── Helpers ──────────────────────────────────────────────────────────────────

function formatBytes(bytes: number): string {
  if (isNaN(bytes) || bytes === 0) return '0 B';
  const units = ['B', 'KiB', 'MiB', 'GiB', 'TiB'];
  const i = Math.min(Math.floor(Math.log(bytes) / Math.log(1024)), units.length - 1);
  return `${(bytes / Math.pow(1024, i)).toFixed(2)} ${units[i]}`;
}

function formatCpu(cores: number): string {
  if (isNaN(cores) || cores === 0) return '0 m';
  if (cores < 1) return `${(cores * 1000).toFixed(2)} m`;
  return `${cores.toFixed(2)} cores`;
}

// ── Main Component ───────────────────────────────────────────────────────────

export const BenchMetricsTab: React.FC<BenchMetricsTabProps> = ({ obj }) => {
  const benchName = obj?.metadata?.name;
  const namespace = obj?.metadata?.namespace;

  const [topMetricType, setTopMetricType] = React.useState<'memory' | 'cpu'>('memory');

  // PromQL Queries (sum by pod for native multi-line graph with legends)
  const cpuQuery = namespace && benchName
    ? `sum by (pod) (rate(container_cpu_usage_seconds_total{namespace="${namespace}",pod=~"${benchName}-.*",container!=""}[5m]))`
    : '';

  const memQuery = namespace && benchName
    ? `sum by (pod) (container_memory_working_set_bytes{namespace="${namespace}",pod=~"${benchName}-.*",container!=""})`
    : '';

  // Top 10 Memory Pods
  const [topMemRes, topMemLoaded] = usePrometheusPoll({
    endpoint: PrometheusEndpoint.QUERY,
    query: namespace && benchName
      ? `topk(10, sum by (pod) (container_memory_working_set_bytes{namespace="${namespace}",pod=~"${benchName}-.*",container!=""}))`
      : undefined,
  });

  // Top 10 CPU Pods
  const [topCpuRes, topCpuLoaded] = usePrometheusPoll({
    endpoint: PrometheusEndpoint.QUERY,
    query: namespace && benchName
      ? `topk(10, sum by (pod) (rate(container_cpu_usage_seconds_total{namespace="${namespace}",pod=~"${benchName}-.*",container!=""}[5m])))`
      : undefined,
  });

  const topMemItems: MetricItem[] = React.useMemo(() => {
    if (!topMemRes?.data?.result) return [];
    return topMemRes.data.result
      .map((item) => ({
        pod: item.metric?.pod ?? 'unknown',
        value: parseFloat(item.value?.[1] ?? '0'),
      }))
      .sort((a, b) => b.value - a.value);
  }, [topMemRes]);

  const topCpuItems: MetricItem[] = React.useMemo(() => {
    if (!topCpuRes?.data?.result) return [];
    return topCpuRes.data.result
      .map((item) => ({
        pod: item.metric?.pod ?? 'unknown',
        value: parseFloat(item.value?.[1] ?? '0'),
      }))
      .sort((a, b) => b.value - a.value);
  }, [topCpuRes]);

  const activeItems = topMetricType === 'memory' ? topMemItems : topCpuItems;
  const maxVal = React.useMemo(() => {
    if (activeItems.length === 0) return 1;
    return Math.max(...activeItems.map((i) => i.value), 1);
  }, [activeItems]);

  const cardStyle: React.CSSProperties = {
    backgroundColor: '#ffffff',
    border: '1px solid #d2d2d2',
    borderRadius: '4px',
    padding: '20px 24px',
    marginBottom: '24px',
  };

  const cardHeaderStyle: React.CSSProperties = {
    display: 'flex',
    justifyContent: 'space-between',
    alignItems: 'center',
    marginBottom: '16px',
  };

  const titleStyle: React.CSSProperties = {
    fontSize: '16px',
    fontWeight: 600,
    margin: 0,
    color: '#151515',
  };

  const inspectLinkStyle: React.CSSProperties = {
    fontSize: '13px',
    color: '#0066cc',
    textDecoration: 'none',
    fontWeight: 500,
  };

  return (
    <div className="co-m-pane__body" style={{ padding: '24px 30px' }}>
      {/* ── Section Header ──────────────────────────────────────────────── */}
      <h2
        style={{
          fontSize: '20px',
          fontWeight: 600,
          margin: '0 0 20px 0',
          color: 'var(--pf-global--Color--100, #151515)',
        }}
      >
        Resource usage
      </h2>

      {/* ── 1. CPU Usage Full-Width Native Chart ─────────────────────────── */}
      <div style={cardStyle}>
        <div style={cardHeaderStyle}>
          <h3 style={titleStyle}>CPU Usage</h3>
          {cpuQuery && (
            <a
              href={`/monitoring/query-browser?query0=${encodeURIComponent(cpuQuery)}`}
              target="_blank"
              rel="noopener noreferrer"
              style={inspectLinkStyle}
            >
              Inspect
            </a>
          )}
        </div>

        {cpuQuery ? (
          <QueryBrowser
            queries={[cpuQuery]}
            defaultTimespan={30 * 60 * 1000}
            namespace={namespace}
            pollInterval={15 * 1000}
            showLegend={true}
            hideControls={true}
            isStack={false}
          />
        ) : (
          <div style={{ padding: '32px', textAlign: 'center' }}>
            <Spinner size="md" />
          </div>
        )}
      </div>

      {/* ── 2. Memory Usage Full-Width Native Chart ──────────────────────── */}
      <div style={cardStyle}>
        <div style={cardHeaderStyle}>
          <h3 style={titleStyle}>Memory Usage</h3>
          {memQuery && (
            <a
              href={`/monitoring/query-browser?query0=${encodeURIComponent(memQuery)}`}
              target="_blank"
              rel="noopener noreferrer"
              style={inspectLinkStyle}
            >
              Inspect
            </a>
          )}
        </div>

        {memQuery ? (
          <QueryBrowser
            queries={[memQuery]}
            defaultTimespan={30 * 60 * 1000}
            namespace={namespace}
            pollInterval={15 * 1000}
            showLegend={true}
            hideControls={true}
            isStack={false}
          />
        ) : (
          <div style={{ padding: '32px', textAlign: 'center' }}>
            <Spinner size="md" />
          </div>
        )}
      </div>

      {/* ── 3. Top 10 Pod Consumption Section ───────────────────────────── */}
      <div style={{ ...cardStyle, marginBottom: '16px' }}>
        <div
          style={{
            display: 'flex',
            justifyContent: 'space-between',
            alignItems: 'center',
            marginBottom: '20px',
          }}
        >
          <h3 style={titleStyle}>
            {topMetricType === 'memory' ? 'Memory usage by pod (top 10)' : 'CPU usage by pod (top 10)'}
          </h3>

          <div style={{ display: 'flex', gap: '8px' }}>
            <Button
              variant={topMetricType === 'memory' ? 'primary' : 'secondary'}
              size="sm"
              onClick={() => setTopMetricType('memory')}
            >
              Memory
            </Button>
            <Button
              variant={topMetricType === 'cpu' ? 'primary' : 'secondary'}
              size="sm"
              onClick={() => setTopMetricType('cpu')}
            >
              CPU
            </Button>
          </div>
        </div>

        {/* Horizontal Bars */}
        {(!topMemLoaded && topMetricType === 'memory') || (!topCpuLoaded && topMetricType === 'cpu') ? (
          <div style={{ padding: '32px', textAlign: 'center' }}>
            <Spinner size="lg" aria-label="Loading top consumers" />
          </div>
        ) : activeItems.length === 0 ? (
          <div style={{ padding: '24px', color: '#6a6e73' }}>
            No pod metrics found matching this bench.
          </div>
        ) : (
          <div style={{ display: 'flex', flexDirection: 'column', gap: '16px' }}>
            {activeItems.map((item) => {
              const pct = maxVal > 0 ? (item.value / maxVal) * 100 : 0;
              const formattedVal =
                topMetricType === 'memory' ? formatBytes(item.value) : formatCpu(item.value);

              return (
                <div key={item.pod}>
                  {/* Pod Name with ResourceLink */}
                  <div style={{ marginBottom: '6px', fontSize: '13px' }}>
                    <ResourceLink
                      kind="Pod"
                      name={item.pod}
                      namespace={namespace}
                    />
                  </div>

                  {/* Horizontal Bar + Value */}
                  <div style={{ display: 'flex', alignItems: 'center', gap: '16px' }}>
                    <div
                      style={{
                        flex: 1,
                        height: '6px',
                        backgroundColor: '#e0e0e0',
                        borderRadius: '3px',
                        overflow: 'hidden',
                      }}
                    >
                      <div
                        style={{
                          width: `${Math.max(pct, 1)}%`,
                          height: '100%',
                          backgroundColor: '#0066cc',
                          borderRadius: '3px',
                          transition: 'width 0.4s ease',
                        }}
                      />
                    </div>
                    <div
                      style={{
                        fontSize: '13px',
                        color: '#151515',
                        minWidth: '85px',
                        textAlign: 'right',
                        fontVariantNumeric: 'tabular-nums',
                      }}
                    >
                      {formattedVal}
                    </div>
                  </div>
                </div>
              );
            })}
          </div>
        )}
      </div>
    </div>
  );
};

/**
 * The console renders this through the extension's `$codeRef`, so the themed
 * wrapper has to be the default export — it is what puts the plugin's bundled
 * PatternFly styles and theme scope around the tree.
 */
const BenchMetricsTabPage: React.FC<React.ComponentProps<typeof BenchMetricsTab>> = (props) => (
  <PluginRoot>
    <BenchMetricsTab {...props} />
  </PluginRoot>
);

export default BenchMetricsTabPage;
