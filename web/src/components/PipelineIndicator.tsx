interface PipelineIndicatorProps {
  steps: string[];
  currentIndex: number;
  isFlowing: boolean;
  onStepClick?: (step: string) => void;
}

export function PipelineIndicator({ steps, currentIndex, isFlowing, onStepClick }: PipelineIndicatorProps) {
  const progressPercent = steps.length > 1
    ? (currentIndex / (steps.length - 1)) * 100
    : 0;

  return (
    <div className="space-y-2">
      <div className="flex items-center gap-0 overflow-x-auto pb-1">
        {steps.map((step, i) => {
          const isCurrent = i === currentIndex && isFlowing;
          const isCompleted = i < currentIndex;
          const isFuture = i > currentIndex;

          return (
            <div key={step} className="flex items-center shrink-0">
              {i > 0 && (
                <div className={`h-0.5 w-3 ${
                  isCompleted ? 'bg-cistern-green' : isCurrent ? 'bg-cistern-accent' : 'bg-cistern-border'
                }`} />
              )}
              <button
                type="button"
                onClick={() => onStepClick?.(step)}
                className={`relative px-2 py-1 rounded text-xs font-mono whitespace-nowrap transition-colors ${
                  isCurrent
                    ? 'water-flow-active text-cistern-bg font-bold'
                    : isCompleted
                    ? 'bg-cistern-green/20 text-cistern-green hover:bg-cistern-green/30'
                    : isFuture && isFlowing
                    ? 'bg-cistern-accent/10 text-cistern-accent/50 hover:bg-cistern-accent/20'
                    : 'bg-cistern-border/30 text-cistern-muted hover:bg-cistern-border/50'
                }`}
              >
                {step}
                {isCurrent && (
                  <span className="absolute -top-1 -right-1 w-2 h-2 bg-cistern-accent rounded-full animate-ping" />
                )}
              </button>
            </div>
          );
        })}
      </div>
      {isFlowing && progressPercent > 0 && (
        <div className="h-1.5 bg-cistern-border rounded-full overflow-hidden">
          <div
            className="h-full rounded-full bg-cistern-accent transition-all duration-500"
            style={{ width: `${progressPercent}%` }}
          />
        </div>
      )}
    </div>
  );
}