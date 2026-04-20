import { NavLink, useLocation } from 'react-router-dom';

const navItems = [
  { to: '/app/', label: 'Dashboard', icon: DashboardIcon },
  { to: '/app/droplets', label: 'Droplets', icon: DropletsIcon },
  { to: '/app/filter', label: 'Filter', icon: FilterIcon },
  { to: '/app/import', label: 'Import', icon: ImportIcon },
  { to: '/app/castellarius', label: 'Castellarius', icon: CastellariusIcon },
  { to: '/app/doctor', label: 'Doctor', icon: DoctorIcon },
  { to: '/app/logs', label: 'Logs', icon: LogsIcon },
  { to: '/app/repos', label: 'Repos/Skills', icon: ReposIcon },
];

export function Sidebar({ open, onToggle }: { open: boolean; onToggle: () => void }) {
  const location = useLocation();

  return (
    <>
      {open && (
        <div className="fixed inset-0 bg-black/50 z-20 md:hidden" onClick={onToggle} />
      )}
      <aside
        className={`fixed md:relative z-30 h-full bg-cistern-surface border-r border-cistern-border flex flex-col transition-all duration-200 ${
          open ? 'w-56' : 'w-0 md:w-14'
        } overflow-hidden`}
      >
        <div className="flex items-center gap-3 px-4 py-4 border-b border-cistern-border min-h-[60px]">
          <button
            onClick={onToggle}
            className="text-cistern-muted hover:text-cistern-fg transition-colors"
            aria-label="Toggle sidebar"
          >
            <svg width="20" height="20" viewBox="0 0 20 20" fill="none" stroke="currentColor" strokeWidth="2">
              {open ? (
                <path d="M6 6l8 8M14 6l-8 8" />
              ) : (
                <path d="M3 5h14M3 10h14M3 15h14" />
              )}
            </svg>
          </button>
          {open && (
            <span className="text-cistern-accent font-mono font-bold text-lg whitespace-nowrap">
              Cistern
            </span>
          )}
        </div>
        <nav className="flex-1 py-2 space-y-1 overflow-y-auto">
          {navItems.map((item) => (
            <NavLink
              key={item.to}
              to={item.to}
              className={() => {
                const isActive = location.pathname === item.to ||
                  (item.to === '/app/' && location.pathname === '/app');
                return `flex items-center gap-3 px-4 py-2.5 mx-2 rounded-md transition-colors text-sm whitespace-nowrap ${
                  isActive
                    ? 'bg-cistern-accent/10 text-cistern-accent'
                    : 'text-cistern-muted hover:text-cistern-fg hover:bg-cistern-border/30'
                }`;
              }}
            >
              <item.icon className="w-5 h-5 shrink-0" />
              {open && <span>{item.label}</span>}
            </NavLink>
          ))}
        </nav>
      </aside>
    </>
  );
}

function DashboardIcon({ className }: { className?: string }) {
  return (
    <svg className={className} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <rect x="3" y="3" width="7" height="7" /><rect x="14" y="3" width="7" height="7" /><rect x="14" y="14" width="7" height="7" /><rect x="3" y="14" width="7" height="7" />
    </svg>
  );
}

function DropletsIcon({ className }: { className?: string }) {
  return (
    <svg className={className} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <path d="M12 2.69l5.66 5.66a8 8 0 1 1-11.31 0z" />
    </svg>
  );
}

function CastellariusIcon({ className }: { className?: string }) {
  return (
    <svg className={className} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <circle cx="12" cy="12" r="3" /><path d="M12 1v4M12 19v4M4.22 4.22l2.83 2.83M16.95 16.95l2.83 2.83M1 12h4M19 12h4M4.22 19.78l2.83-2.83M16.95 7.05l2.83-2.83" />
    </svg>
  );
}

function DoctorIcon({ className }: { className?: string }) {
  return (
    <svg className={className} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <path d="M22 12h-4l-3 9L9 3l-3 9H2" />
    </svg>
  );
}

function LogsIcon({ className }: { className?: string }) {
  return (
    <svg className={className} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z" /><polyline points="14 2 14 8 20 8" /><line x1="16" y1="13" x2="8" y2="13" /><line x1="16" y1="17" x2="8" y2="17" /><polyline points="10 9 9 9 8 9" />
    </svg>
  );
}

function ReposIcon({ className }: { className?: string }) {
  return (
    <svg className={className} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <path d="M9 19c-5 1.5-5-2.5-7-3m14 6v-3.87a3.37 3.37 0 0 0-.94-2.61c3.14-.35 6.44-1.54 6.44-7A5.44 5.44 0 0 0 20 4.77 5.07 5.07 0 0 0 19.91 1S18.73.65 16 2.48a13.38 13.38 0 0 0-7 0C6.27.65 5.09 1 5.09 1A5.07 5.07 0 0 0 5 4.77a5.44 5.44 0 0 0-1.5 3.35c0 5.46 3.3 6.65 6.44 7A3.37 3.37 0 0 0 9 18.13V22" />
    </svg>
  );
}

function FilterIcon({ className }: { className?: string }) {
  return (
    <svg className={className} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <polygon points="22 3 2 3 10 12.46 10 19 14 21 14 12.46 22 3" />
    </svg>
  );
}

function ImportIcon({ className }: { className?: string }) {
  return (
    <svg className={className} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4" /><polyline points="7 10 12 15 17 10" /><line x1="12" y1="15" x2="12" y2="3" />
    </svg>
  );
}