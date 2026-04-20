import { useState, useEffect, useCallback } from 'react';
import { createFilterSession, resumeFilterSession, listFilterSessions, parseFilterMessages } from '../api/filter';
import { ChatWindow } from '../components/ChatWindow';
import { SpecPreview } from '../components/SpecPreview';
import type { FilterSession, FilterMessage } from '../api/types';

export function FilterPage() {
  const [sessions, setSessions] = useState<FilterSession[]>([]);
  const [currentSession, setCurrentSession] = useState<FilterSession | null>(null);
  const [messages, setMessages] = useState<FilterMessage[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [showNewSession, setShowNewSession] = useState(false);
  const [newTitle, setNewTitle] = useState('');
  const [newDescription, setNewDescription] = useState('');
  const [showSessions, setShowSessions] = useState(false);

  useEffect(() => {
    listFilterSessions()
      .then((s) => setSessions(s || []))
      .catch(() => {});
  }, []);

  const handleNewSession = useCallback(async () => {
    if (!newTitle.trim()) return;
    setLoading(true);
    setError(null);
    try {
      const result = await createFilterSession(newTitle.trim(), newDescription.trim());
      const session = result.session;
      setCurrentSession(session);
      const initialMessages: FilterMessage[] = parseFilterMessages(session.messages);
      setMessages([
        ...initialMessages,
      ]);
      setShowNewSession(false);
      setNewTitle('');
      setNewDescription('');
      listFilterSessions().then((s) => setSessions(s || [])).catch(() => {});
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to create session');
    } finally {
      setLoading(false);
    }
  }, [newTitle, newDescription]);

  const handleResumeSession = useCallback(async (session: FilterSession) => {
    setCurrentSession(session);
    setMessages(parseFilterMessages(session.messages));
    setShowSessions(false);
  }, []);

  const handleSendMessage = useCallback(async (message: string) => {
    if (!currentSession) return;
    setMessages((prev) => [...prev, { role: 'user', content: message }]);
    setLoading(true);
    setError(null);
    try {
      const result = await resumeFilterSession(currentSession.id, message);
      setMessages((prev) => [...prev, { role: 'assistant', content: result.assistant_message }]);
      listFilterSessions().then((s) => setSessions(s || [])).catch(() => {});
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to send message');
    } finally {
      setLoading(false);
    }
  }, [currentSession]);

  const handleAccept = useCallback(() => {
    if (!currentSession) return;
    window.location.href = `/app/droplets/new?title=${encodeURIComponent(currentSession.title)}&description=${encodeURIComponent(currentSession.description)}`;
  }, [currentSession]);

  return (
    <div className="flex-1 overflow-y-auto p-4 md:p-6">
      <div className="flex items-center justify-between mb-4">
        <h1 className="text-xl font-mono font-bold text-cistern-fg">Filter &amp; Refine</h1>
        <div className="flex gap-2">
          <button
            type="button"
            onClick={() => setShowSessions(!showSessions)}
            className="px-3 py-1.5 text-sm rounded border border-cistern-border text-cistern-muted hover:text-cistern-fg transition-colors"
          >
            Past Sessions
          </button>
          <button
            type="button"
            onClick={() => setShowNewSession(true)}
            className="px-3 py-1.5 text-sm rounded bg-cistern-accent text-cistern-bg font-medium hover:bg-cistern-accent/90 transition-colors"
          >
            + New Session
          </button>
        </div>
      </div>

      {showSessions && sessions.length > 0 && (
        <div className="mb-4 bg-cistern-surface border border-cistern-border rounded-lg p-3">
          <div className="text-xs font-mono text-cistern-muted uppercase tracking-wider mb-2">Previous Sessions</div>
          <div className="space-y-1 max-h-40 overflow-y-auto">
            {sessions.map((s) => (
              <button
                key={s.id}
                type="button"
                onClick={() => handleResumeSession(s)}
                className="w-full text-left px-3 py-2 rounded hover:bg-cistern-accent/10 transition-colors"
              >
                <div className="text-sm text-cistern-fg font-mono">{s.title}</div>
                <div className="text-xs text-cistern-muted">{new Date(s.updated_at).toLocaleString()}</div>
              </button>
            ))}
          </div>
        </div>
      )}

      {showNewSession && (
        <div className="mb-4 bg-cistern-surface border border-cistern-border rounded-lg p-4">
          <div className="text-xs font-mono font-bold text-cistern-muted uppercase tracking-wider mb-3">New Filter Session</div>
          <div className="space-y-3">
            <div>
              <label className="block text-xs font-mono text-cistern-muted mb-1">Title *</label>
              <input
                type="text"
                value={newTitle}
                onChange={(e) => setNewTitle(e.target.value)}
                placeholder="What do you want to build?"
                className="w-full bg-cistern-bg border border-cistern-border rounded px-3 py-1.5 text-sm text-cistern-fg"
                autoFocus
              />
            </div>
            <div>
              <label className="block text-xs font-mono text-cistern-muted mb-1">Description</label>
              <textarea
                value={newDescription}
                onChange={(e) => setNewDescription(e.target.value)}
                placeholder="Optional: more details about your idea..."
                className="w-full bg-cistern-bg border border-cistern-border rounded p-2 text-sm text-cistern-fg resize-y min-h-[60px]"
              />
            </div>
            <div className="flex gap-2 justify-end">
              <button
                type="button"
                onClick={() => setShowNewSession(false)}
                className="px-3 py-1.5 text-sm rounded border border-cistern-border text-cistern-muted hover:text-cistern-fg"
              >
                Cancel
              </button>
              <button
                type="button"
                onClick={handleNewSession}
                disabled={loading || !newTitle.trim()}
                className="px-3 py-1.5 text-sm rounded bg-cistern-accent text-cistern-bg font-medium disabled:opacity-50"
              >
                {loading ? 'Starting…' : 'Start Session'}
              </button>
            </div>
          </div>
        </div>
      )}

      {error && (
        <div className="mb-4 bg-cistern-red/10 border border-cistern-red/30 rounded p-3 text-sm text-cistern-red font-mono">
          {error}
        </div>
      )}

      {currentSession ? (
        <div className="flex gap-4 h-[calc(100vh-200px)]">
          <div className="flex-1 bg-cistern-surface border border-cistern-border rounded-lg overflow-hidden flex flex-col">
            <div className="px-4 py-2 border-b border-cistern-border">
              <span className="text-sm font-mono text-cistern-fg">{currentSession.title}</span>
            </div>
            <ChatWindow
              messages={messages}
              onSendMessage={handleSendMessage}
              loading={loading}
              placeholder={messages.length === 0 ? 'Describe your idea or respond to the assistant...' : 'Type your reply...'}
            />
          </div>
          <div className="w-80 shrink-0 space-y-3">
            <SpecPreview
              specSnapshot={currentSession.spec_snapshot || ''}
              title={currentSession.title}
              description={currentSession.description}
            />
            <button
              type="button"
              onClick={handleAccept}
              className="w-full px-4 py-2 text-sm rounded bg-cistern-green text-cistern-bg font-medium hover:bg-cistern-green/90 transition-colors"
            >
              Accept &amp; File Droplet
            </button>
          </div>
        </div>
      ) : (
        <div className="flex items-center justify-center h-[calc(100vh-200px)] bg-cistern-surface border border-cistern-border rounded-lg">
          <div className="text-center">
            <div className="text-cistern-muted text-4xl mb-3">💡</div>
            <div className="text-cistern-muted font-mono text-sm">
              Start a new session to refine your idea into a detailed droplet spec.
            </div>
          </div>
        </div>
      )}
    </div>
  );
}