import { useState } from 'react';

interface LoginPageProps {
  onLogin: (key: string) => void;
  error?: boolean;
}

export function LoginPage({ onLogin, error }: LoginPageProps) {
  const [key, setKey] = useState('');

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    if (key.trim()) {
      onLogin(key.trim());
    }
  };

  return (
    <div className="min-h-screen bg-cistern-bg flex items-center justify-center">
      <div className="w-full max-w-sm p-8">
        <h1 className="text-2xl font-bold text-cistern-fg mb-6 text-center">Cistern</h1>
        <form onSubmit={handleSubmit} className="space-y-4">
          <div>
            <label htmlFor="api-key" className="block text-sm text-cistern-muted mb-1">
              API Key
            </label>
            <input
              id="api-key"
              type="password"
              value={key}
              onChange={(e) => setKey(e.target.value)}
              className="w-full px-3 py-2 bg-cistern-bg border border-cistern-border rounded-md text-cistern-fg focus:outline-none focus:ring-2 focus:ring-cistern-accent"
              placeholder="Enter dashboard API key"
              autoFocus
            />
          </div>
          {error && (
            <p className="text-cistern-red text-sm">Invalid API key. Please try again.</p>
          )}
          <button
            type="submit"
            disabled={!key.trim()}
            className="w-full px-4 py-2 bg-cistern-accent text-cistern-bg font-medium rounded-md hover:opacity-90 transition-opacity disabled:opacity-50 disabled:cursor-not-allowed"
          >
            Connect
          </button>
        </form>
      </div>
    </div>
  );
}