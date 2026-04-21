import { useState, useEffect, useRef, useCallback } from 'react';
import { useNavigate } from 'react-router-dom';

interface CommandPaletteProps {
  open: boolean;
  onClose: () => void;
}

interface Command {
  id: string;
  label: string;
  path: string;
  section: string;
}

const commands: Command[] = [
  { id: 'dashboard', label: 'Go to Dashboard', path: '/app/', section: 'Navigation' },
  { id: 'droplets', label: 'Go to Droplets', path: '/app/droplets', section: 'Navigation' },
  { id: 'create-droplet', label: 'Create Droplet', path: '/app/droplets/new', section: 'Actions' },
  { id: 'filter', label: 'Go to Filter / Refine', path: '/app/filter', section: 'Navigation' },
  { id: 'import', label: 'Go to Import', path: '/app/import', section: 'Navigation' },
  { id: 'castellarius', label: 'Go to Castellarius', path: '/app/castellarius', section: 'Navigation' },
  { id: 'doctor', label: 'Go to Doctor', path: '/app/doctor', section: 'Navigation' },
  { id: 'logs', label: 'Go to Logs', path: '/app/logs', section: 'Navigation' },
  { id: 'repos', label: 'Go to Repos & Skills', path: '/app/repos', section: 'Navigation' },
  { id: 'classic', label: 'Switch to Classic Dashboard', path: '/', section: 'Navigation' },
];

export function CommandPalette({ open, onClose }: CommandPaletteProps) {
  const [query, setQuery] = useState('');
  const [selectedIndex, setSelectedIndex] = useState(0);
  const inputRef = useRef<HTMLInputElement>(null);
  const navigate = useNavigate();

  const filtered = commands.filter((c) =>
    c.label.toLowerCase().includes(query.toLowerCase())
  );

  useEffect(() => {
    if (open) {
      setQuery('');
      setSelectedIndex(0);
      requestAnimationFrame(() => inputRef.current?.focus());
    }
  }, [open]);

  const execute = useCallback((command: Command) => {
    navigate(command.path);
    onClose();
  }, [navigate, onClose]);

  useEffect(() => {
    if (!open) return;
    const handleKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        e.preventDefault();
        onClose();
      } else if (e.key === 'ArrowDown') {
        e.preventDefault();
        setSelectedIndex((i) => Math.min(i + 1, filtered.length - 1));
      } else if (e.key === 'ArrowUp') {
        e.preventDefault();
        setSelectedIndex((i) => Math.max(i - 1, 0));
      } else if (e.key === 'Enter' && filtered[selectedIndex]) {
        e.preventDefault();
        execute(filtered[selectedIndex]);
      }
    };
    window.addEventListener('keydown', handleKey);
    return () => window.removeEventListener('keydown', handleKey);
  }, [open, filtered, selectedIndex, execute, onClose]);

  useEffect(() => {
    setSelectedIndex(0);
  }, [query]);

  if (!open) return null;

  return (
    <div className="fixed inset-0 z-[60] flex items-start justify-center pt-[20vh]" role="dialog" aria-label="Command palette">
      <div className="fixed inset-0 bg-black/50" onClick={onClose} />
      <div className="relative bg-cistern-surface border border-cistern-border rounded-lg shadow-2xl w-full max-w-lg overflow-hidden">
        <input
          ref={inputRef}
          type="text"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          placeholder="Type a command…"
          className="w-full bg-transparent px-4 py-3 text-sm text-cistern-fg placeholder-cistern-muted border-b border-cistern-border outline-none"
          aria-label="Search commands"
        />
        <ul className="max-h-64 overflow-y-auto py-1" role="listbox">
          {filtered.length === 0 && (
            <li className="px-4 py-2 text-sm text-cistern-muted">No commands found</li>
          )}
          {filtered.map((cmd, i) => (
            <li key={cmd.id}>
              <button
                className={`w-full text-left px-4 py-2 text-sm flex items-center gap-2 ${
                  i === selectedIndex
                    ? 'bg-cistern-accent/10 text-cistern-accent'
                    : 'text-cistern-fg hover:bg-cistern-border/30'
                }`}
                onClick={() => execute(cmd)}
                role="option"
                aria-selected={i === selectedIndex}
              >
                <span className="text-cistern-muted text-xs">{cmd.section}</span>
                <span>{cmd.label}</span>
              </button>
            </li>
          ))}
        </ul>
        <div className="px-4 py-2 border-t border-cistern-border text-xs text-cistern-muted">
          ↑↓ navigate · Enter select · Esc close
        </div>
      </div>
    </div>
  );
}