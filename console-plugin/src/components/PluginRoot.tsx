import * as React from 'react';
import '../patternfly.css';

export interface PluginRootProps {
  children: React.ReactNode;
  className?: string;
}

/**
 * Wraps every page and tab the plugin renders.
 *
 * PatternFly styling is inherited from the host console rather than bundled —
 * the plugin targets PatternFly 6, the version the console ships from OpenShift
 * 4.19 — so this only establishes the class the plugin's own scoped layout rules
 * hang off. Theming needs no work here: the console sets its theme globally and
 * PatternFly 6's tokens resolve from the document root.
 */
export const PluginRoot: React.FC<PluginRootProps> = ({ children, className }) => (
  <div
    className={`frappe-plugin-root${className ? ` ${className}` : ''}`}
    data-test="frappe-plugin-root"
  >
    {children}
  </div>
);

/**
 * Class names for content rendered outside the `PluginRoot` subtree.
 * PatternFly modals portal to `document.body`, so they need the plugin's scope
 * applied directly.
 */
export const useThemeClass = (): string => 'frappe-plugin-root';

export default PluginRoot;
