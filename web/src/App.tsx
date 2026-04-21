import { useState, useEffect } from 'react';
import { Outlet } from 'react-router-dom';
import { Sidebar } from './components/Sidebar';
import { Header } from './components/Header';
import { ToastProvider, ToastOutlet } from './components/Toast';
import { DashboardProvider, useDashboard } from './context/DashboardContext';
import { CommandPalette } from './components/CommandPalette';
import { useAuth } from './hooks/useAuth';
import { LoginPage } from './components/LoginPage';

export function AppLayout() {
  const { required, authenticated, authError, login } = useAuth();

  if (required && !authenticated) {
    return <LoginPage onLogin={login} error={authError} />;
  }

  return (
    <DashboardProvider>
      <ToastProvider>
        <AppLayoutInner />
        <ToastOutlet />
      </ToastProvider>
    </DashboardProvider>
  );
}

function AppLayoutInner() {
  const [sidebarOpen, setSidebarOpen] = useState(true);
  const [commandPaletteOpen, setCommandPaletteOpen] = useState(false);
  const { data, connected } = useDashboard();

  useEffect(() => {
    const handleKey = (e: KeyboardEvent) => {
      if (e.ctrlKey && e.key === 'k') {
        e.preventDefault();
        setCommandPaletteOpen((prev) => !prev);
      }
    };
    window.addEventListener('keydown', handleKey);
    return () => window.removeEventListener('keydown', handleKey);
  }, []);

  return (
    <div className="h-screen flex overflow-hidden bg-cistern-bg">
      <Sidebar open={sidebarOpen} onToggle={() => setSidebarOpen(!sidebarOpen)} />
      <div className="flex-1 flex flex-col min-w-0 overflow-hidden">
        <Header data={data} connected={connected} onMenuClick={() => setSidebarOpen(!sidebarOpen)} />
        <main className="flex-1 overflow-y-auto">
          <Outlet />
        </main>
      </div>
      <CommandPalette open={commandPaletteOpen} onClose={() => setCommandPaletteOpen(false)} />
    </div>
  );
}