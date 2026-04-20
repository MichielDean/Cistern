interface SpecPreviewProps {
  specSnapshot: string;
  title: string;
  description: string;
}

export function SpecPreview({ specSnapshot, title, description }: SpecPreviewProps) {
  return (
    <div className="bg-cistern-surface border border-cistern-border rounded-lg p-4 space-y-3">
      <h3 className="text-xs font-mono font-bold uppercase tracking-wider text-cistern-muted">
        Spec Preview
      </h3>
      {title && (
        <div>
          <span className="text-xs font-mono text-cistern-muted">Title: </span>
          <span className="text-sm text-cistern-fg">{title}</span>
        </div>
      )}
      {description && (
        <div>
          <span className="text-xs font-mono text-cistern-muted">Description: </span>
          <span className="text-sm text-cistern-fg">{description}</span>
        </div>
      )}
      {specSnapshot ? (
        <div className="border-t border-cistern-border pt-3">
          <div className="text-xs font-mono text-cistern-muted mb-1">Refined Spec</div>
          <div className="whitespace-pre-wrap text-sm text-cistern-fg bg-cistern-bg rounded p-3 max-h-[400px] overflow-y-auto">
            {specSnapshot}
          </div>
        </div>
      ) : (
        <div className="border-t border-cistern-border pt-3">
          <div className="text-xs font-mono text-cistern-muted italic">
            Spec will appear here as the conversation progresses...
          </div>
        </div>
      )}
    </div>
  );
}