import { useState, useEffect, useRef } from 'react';
import { fetchRepos, fetchSkills } from '../api/repos';
import type { RepoInfo, SkillInfo } from '../api/types';

export function ReposSkillsPage() {
  const [repos, setRepos] = useState<RepoInfo[]>([]);
  const [skills, setSkills] = useState<SkillInfo[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<Error | null>(null);
  const mountedRef = useRef(true);

  useEffect(() => {
    setLoading(true);
    Promise.all([fetchRepos(), fetchSkills()])
      .then(([r, s]) => {
        if (!mountedRef.current) return;
        setRepos(r);
        setSkills(s);
      })
      .catch(err => {
        if (!mountedRef.current) return;
        setError(err instanceof Error ? err : new Error(String(err)));
      })
      .finally(() => {
        if (mountedRef.current) setLoading(false);
      });
    return () => { mountedRef.current = false; };
  }, []);

  if (error && repos.length === 0 && skills.length === 0) {
    return (
      <div className="flex items-center justify-center h-full">
        <div className="text-center">
          <div className="text-cistern-red text-lg font-mono mb-2">Error</div>
          <div className="text-cistern-muted text-sm">{error.message}</div>
        </div>
      </div>
    );
  }

  if (loading) {
    return (
      <div className="flex items-center justify-center h-full">
        <div className="text-cistern-muted font-mono">Loading…</div>
      </div>
    );
  }

  return (
    <div className="flex-1 overflow-y-auto p-4 md:p-6 space-y-8">
      <section>
        <h2 className="text-sm font-mono text-cistern-muted uppercase tracking-wider mb-3">Repositories</h2>
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
          {repos.map(repo => (
            <div key={repo.name} className="bg-cistern-surface border border-cistern-border rounded-lg p-4">
              <div className="font-mono font-bold text-cistern-fg mb-2">{repo.name}</div>
              <div className="text-xs text-cistern-muted font-mono space-y-1">
                {repo.prefix && <div>Prefix: {repo.prefix}</div>}
                <div>
                  URL:{' '}
                  <a href={repo.url} target="_blank" rel="noopener noreferrer" className="text-cistern-accent hover:underline">
                    {repo.url}
                  </a>
                </div>
              </div>
              {repo.aqueducts.length > 0 && (
                <div className="mt-3 space-y-1">
                  <div className="text-xs text-cistern-muted font-mono uppercase">Aqueducts</div>
                  {repo.aqueducts.map(aq => (
                    <div key={aq.name} className="text-xs font-mono text-cistern-fg">
                      <span className="text-cistern-accent">{aq.name}</span>
                      {aq.steps.length > 0 && (
                        <span className="text-cistern-muted"> · {aq.steps.join(' → ')}</span>
                      )}
                    </div>
                  ))}
                </div>
              )}
            </div>
          ))}
          {repos.length === 0 && (
            <div className="text-cistern-muted text-sm font-mono">No repositories configured</div>
          )}
        </div>
      </section>

      <section>
        <h2 className="text-sm font-mono text-cistern-muted uppercase tracking-wider mb-3">Skills</h2>
        {skills.length === 0 ? (
          <div className="text-cistern-muted text-sm font-mono">No skills installed</div>
        ) : (
          <div className="bg-cistern-surface border border-cistern-border rounded-lg overflow-hidden">
            <table className="w-full text-sm font-mono">
              <thead>
                <tr className="border-b border-cistern-border">
                  <th className="text-left text-cistern-muted px-4 py-2 text-xs uppercase">Name</th>
                  <th className="text-left text-cistern-muted px-4 py-2 text-xs uppercase">Source</th>
                  <th className="text-left text-cistern-muted px-4 py-2 text-xs uppercase">Installed</th>
                </tr>
              </thead>
              <tbody>
                {skills.map((skill, i) => (
                  <tr key={skill.name} className={i % 2 === 1 ? 'bg-cistern-surface/50' : ''}>
                    <td className="px-4 py-2 text-cistern-fg">{skill.name}</td>
                    <td className="px-4 py-2 text-cistern-muted">{skill.source_url}</td>
                    <td className="px-4 py-2 text-cistern-muted">{new Date(skill.installed_at).toLocaleDateString()}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </section>
    </div>
  );
}