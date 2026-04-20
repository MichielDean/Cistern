import type { FilterMessage } from '../api/types';

interface MessageBubbleProps {
  message: FilterMessage;
}

export function MessageBubble({ message }: MessageBubbleProps) {
  const isAssistant = message.role === 'assistant';

  return (
    <div className={`flex gap-3 ${isAssistant ? '' : 'flex-row-reverse'}`}>
      {isAssistant && (
        <div className="w-8 h-8 rounded-full bg-cistern-accent/20 flex items-center justify-center shrink-0 text-xs font-mono text-cistern-accent">
          CT
        </div>
      )}
      <div
        className={`max-w-[80%] rounded-lg px-4 py-3 text-sm ${
          isAssistant
            ? 'bg-cistern-surface border border-cistern-border text-cistern-fg'
            : 'bg-cistern-accent/20 text-cistern-fg'
        }`}
      >
        {isAssistant ? (
          <div className="whitespace-pre-wrap break-words">
            {message.content}
          </div>
        ) : (
          <div className="whitespace-pre-wrap break-words">
            {message.content}
          </div>
        )}
      </div>
      {!isAssistant && (
        <div className="w-8 h-8 rounded-full bg-cistern-border flex items-center justify-center shrink-0 text-xs font-mono text-cistern-muted">
          U
        </div>
      )}
    </div>
  );
}