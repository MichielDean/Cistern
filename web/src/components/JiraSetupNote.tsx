export function JiraSetupNote() {
  return (
    <div className="bg-cistern-yellow/10 border border-cistern-yellow/30 rounded-lg p-4 text-sm">
      <h4 className="font-mono font-bold text-cistern-yellow mb-2">Jira Credentials Required</h4>
      <p className="text-cistern-fg mb-2">
        To import from Jira, configure these environment variables:
      </p>
      <ul className="list-disc list-inside space-y-1 text-cistern-muted font-mono text-xs">
        <li>JIRA_API_TOKEN — Your Jira API token</li>
        <li>JIRA_USER_EMAIL — Your Jira account email (for Cloud Basic Auth)</li>
      </ul>
      <p className="text-cistern-muted mt-3 text-xs">
        Set them in your shell profile or in <code className="text-cistern-accent">~/.cistern/env</code>.
      </p>
      <a
        href="/app/doctor"
        className="inline-block mt-2 text-xs font-mono text-cistern-accent hover:underline"
      >
        Check credential status on the Doctor page →
      </a>
    </div>
  );
}