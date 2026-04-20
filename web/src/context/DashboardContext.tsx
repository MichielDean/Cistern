import { createContext, useContext, type ReactNode } from 'react';
import { useDashboardEvents } from '../hooks/useDashboardEvents';
import type { DashboardData } from '../api/types';

interface DashboardContextValue {
  data: DashboardData | null;
  connected: boolean;
  error: Error | null;
}

const DashboardContext = createContext<DashboardContextValue>({
  data: null,
  connected: false,
  error: null,
});

export function DashboardProvider({ children }: { children: ReactNode }) {
  const { data, connected, error } = useDashboardEvents();

  return (
    <DashboardContext.Provider value={{ data, connected, error }}>
      {children}
    </DashboardContext.Provider>
  );
}

export function useDashboard() {
  return useContext(DashboardContext);
}