import type { ReactNode } from 'react';
import { Header } from './Header';
import { ToastHub } from '../../lib/toast';

export function AppShell({ children }: { children: ReactNode }) {
  return (
    <div className="app-shell">
      <Header />
      <main className="app-shell__main">{children}</main>
      <ToastHub />
    </div>
  );
}
