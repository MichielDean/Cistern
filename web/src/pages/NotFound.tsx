export function NotFound() {
  return (
    <div className="flex items-center justify-center h-full p-4">
      <div className="text-center">
        <div className="text-6xl font-mono font-bold text-cistern-accent mb-4">404</div>
        <div className="text-lg text-cistern-fg font-mono mb-2">Page not found</div>
        <div className="text-sm text-cistern-muted mb-6">
          The page you are looking for does not exist.
        </div>
        <a
          href="/app/"
          className="px-4 py-2 rounded bg-cistern-accent text-cistern-bg text-sm font-medium hover:bg-cistern-accent/90 transition-colors"
        >
          Go to Dashboard
        </a>
      </div>
    </div>
  );
}