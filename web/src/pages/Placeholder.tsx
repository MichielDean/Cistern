export function PlaceholderPage({ title }: { title: string }) {
  return (
    <div className="flex items-center justify-center h-full">
      <div className="text-center">
        <h2 className="text-lg font-mono text-cistern-fg mb-2">{title}</h2>
        <p className="text-sm text-cistern-muted">Coming soon</p>
      </div>
    </div>
  );
}