import 'react';

/**
 * The console dynamic plugin SDK (and parts of PatternFly 5) are authored
 * against React 17, where `React.FC<P>` accepted `children` implicitly.
 * @types/react 18 removed that, so every `<TableData>…</TableData>` and
 * `<ListPageHeader>…</ListPageHeader>` in this plugin fails to typecheck even
 * though it is correct at runtime.
 *
 * Declaration merging adds a second call signature that accepts children,
 * restoring the React 17 behaviour for third-party components without loosening
 * anything about our own props.
 */
declare module 'react' {
  interface FunctionComponent<P = {}> {
    (props: P & { children?: ReactNode }, context?: unknown): ReactElement<
      unknown,
      string | JSXElementConstructor<unknown>
    > | null;
  }
}
