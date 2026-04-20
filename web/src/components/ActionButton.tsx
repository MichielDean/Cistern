import { useState, type ReactNode } from 'react';

interface ActionButtonProps {
  label: string;
  onClick: () => Promise<void>;
  variant?: 'primary' | 'danger' | 'default';
  disabled?: boolean;
  icon?: ReactNode;
  confirm?: string;
}

const variantClasses: Record<string, string> = {
  primary: 'bg-cistern-accent text-cistern-bg hover:bg-cistern-accent/80',
  danger: 'bg-cistern-red text-cistern-bg hover:bg-cistern-red/80',
  default: 'border border-cistern-border text-cistern-fg hover:bg-cistern-border/30',
};

export function ActionButton({ label, onClick, variant = 'default', disabled = false, icon, confirm }: ActionButtonProps) {
  const [loading, setLoading] = useState(false);

  const handleClick = async () => {
    if (confirm) {
      if (!window.confirm(confirm)) return;
    }
    setLoading(true);
    try {
      await onClick();
    } finally {
      setLoading(false);
    }
  };

  return (
    <button
      onClick={handleClick}
      disabled={disabled || loading}
      className={`inline-flex items-center gap-1.5 font-mono text-sm px-3 py-1.5 rounded-md transition-colors ${variantClasses[variant]} ${disabled || loading ? 'opacity-50 cursor-not-allowed' : ''}`}
    >
      {loading ? (
        <svg className="animate-spin h-4 w-4" viewBox="0 0 24 24" fill="none">
          <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" />
          <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z" />
        </svg>
      ) : icon}
      {label}
    </button>
  );
}