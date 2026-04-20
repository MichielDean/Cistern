import { useNavigate } from 'react-router-dom';
import { ImportForm } from '../components/ImportForm';
import { JiraSetupNote } from '../components/JiraSetupNote';
import type { Droplet } from '../api/types';

export function ImportPage() {
  const navigate = useNavigate();

  const handleSuccess = (droplet: Droplet) => {
    navigate(`/app/droplets/${droplet.id}`);
  };

  return (
    <div className="flex-1 overflow-y-auto p-4 md:p-6 max-w-2xl mx-auto">
      <h1 className="text-xl font-mono font-bold text-cistern-fg mb-6">Import from Tracker</h1>
      <div className="bg-cistern-surface border border-cistern-border rounded-lg p-6">
        <ImportForm onSuccess={handleSuccess} />
      </div>
      <div className="mt-4">
        <JiraSetupNote />
      </div>
    </div>
  );
}