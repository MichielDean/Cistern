import { useState, useRef, useEffect, useCallback } from 'react';
import type { FilterMessage } from '../api/types';
import { MessageBubble } from './MessageBubble';

interface ChatWindowProps {
  messages: FilterMessage[];
  onSendMessage: (message: string) => void;
  loading: boolean;
  placeholder?: string;
}

export function ChatWindow({ messages, onSendMessage, loading, placeholder }: ChatWindowProps) {
  const [input, setInput] = useState('');
  const bottomRef = useRef<HTMLDivElement>(null);
  const inputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    bottomRef.current?.scrollIntoView({ behavior: 'smooth' });
  }, [messages, loading]);

  const handleSubmit = useCallback((e: React.FormEvent) => {
    e.preventDefault();
    const trimmed = input.trim();
    if (!trimmed || loading) return;
    onSendMessage(trimmed);
    setInput('');
  }, [input, loading, onSendMessage]);

  return (
    <div className="flex flex-col h-full">
      <div className="flex-1 overflow-y-auto p-4 space-y-4">
        {messages.length === 0 && (
          <div className="text-cistern-muted text-sm font-mono text-center py-8">
            Start by entering a title for your droplet idea.
          </div>
        )}
        {messages.map((msg, i) => (
          <MessageBubble key={i} message={msg} />
        ))}
        {loading && (
          <div className="flex gap-3">
            <div className="w-8 h-8 rounded-full bg-cistern-accent/20 flex items-center justify-center shrink-0 text-xs font-mono text-cistern-accent">
              CT
            </div>
            <div className="rounded-lg px-4 py-3 bg-cistern-surface border border-cistern-border">
              <div className="flex gap-1">
                <span className="w-2 h-2 bg-cistern-muted rounded-full animate-bounce" style={{ animationDelay: '0ms' }} />
                <span className="w-2 h-2 bg-cistern-muted rounded-full animate-bounce" style={{ animationDelay: '150ms' }} />
                <span className="w-2 h-2 bg-cistern-muted rounded-full animate-bounce" style={{ animationDelay: '300ms' }} />
              </div>
            </div>
          </div>
        )}
        <div ref={bottomRef} />
      </div>

      <form onSubmit={handleSubmit} className="border-t border-cistern-border p-3 flex gap-2">
        <input
          ref={inputRef}
          type="text"
          value={input}
          onChange={(e) => setInput(e.target.value)}
          placeholder={placeholder || 'Type your message...'}
          disabled={loading}
          className="flex-1 bg-cistern-bg border border-cistern-border rounded px-3 py-2 text-sm text-cistern-fg placeholder-cistern-muted disabled:opacity-50"
        />
        <button
          type="submit"
          disabled={loading || !input.trim()}
          className="px-4 py-2 text-sm rounded bg-cistern-accent text-cistern-bg font-medium disabled:opacity-50 hover:bg-cistern-accent/90 transition-colors"
        >
          Send
        </button>
      </form>
    </div>
  );
}