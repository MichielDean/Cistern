import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { getStoredKey } from '../hooks/useAuth';
import { truncateBuffer, isAuthCloseCode } from '../utils/buffer';
import { TerminalView } from './TerminalView';

export const MAX_HIGHLIGHTS = 200;

interface PeekPanelProps {
  aqueductName: string;
  onClose: () => void;
}

export function PeekPanel({ aqueductName, onClose }: PeekPanelProps) {
  const [output, setOutput] = useState<string>('');
  const [connected, setConnected] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [reconnectKey, setReconnectKey] = useState(0);
  const [autoScroll, setAutoScroll] = useState(true);
  const [searchQuery, setSearchQuery] = useState('');
  const [searchVisible, setSearchVisible] = useState(false);
  const terminalRef = useRef<HTMLPreElement>(null);
  const searchInputRef = useRef<HTMLInputElement>(null);
  const wsRef = useRef<WebSocket | null>(null);
  const mountedRef = useRef(true);
  const userScrollingRef = useRef(false);

  const appendOutput = useCallback((chunk: string) => {
    setOutput((prev) => truncateBuffer(prev, chunk));
  }, []);

  useEffect(() => {
    mountedRef.current = true;
    setOutput('');
    setConnected(false);
    setError(null);
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const wsUrl = `${protocol}//${window.location.host}/ws/aqueducts/${encodeURIComponent(aqueductName)}/peek`;
    const ws = new WebSocket(wsUrl);
    wsRef.current = ws;

    ws.onopen = () => {
      if (mountedRef.current) {
        setConnected(true);
        setError(null);
        const apiKey = getStoredKey();
        if (apiKey) {
          ws.send(JSON.stringify({ type: 'auth', token: apiKey }));
        }
      }
    };
    ws.onmessage = (e) => {
      if (mountedRef.current) appendOutput(e.data as string);
    };
    ws.onclose = (e) => {
      if (!mountedRef.current) return;
      setConnected(false);
      if (isAuthCloseCode(e.code)) {
        setError('Authentication failed. Please check your API key and try again.');
      }
    };
    ws.onerror = () => {
      if (mountedRef.current) {
        setConnected(false);
        setError('Connection failed. The server may be unreachable.');
      }
    };

    return () => {
      mountedRef.current = false;
      ws.close();
      wsRef.current = null;
    };
  }, [aqueductName, appendOutput, reconnectKey]);

  useEffect(() => {
    if (autoScroll && terminalRef.current && !userScrollingRef.current) {
      terminalRef.current.scrollTop = terminalRef.current.scrollHeight;
    }
  }, [output, autoScroll]);

  const handleScroll = useCallback(() => {
    const el = terminalRef.current;
    if (!el) return;
    const atBottom = el.scrollHeight - el.scrollTop - el.clientHeight < 30;
    if (atBottom) {
      userScrollingRef.current = false;
      setAutoScroll(true);
    } else {
      userScrollingRef.current = true;
      setAutoScroll(false);
    }
  }, []);

  useEffect(() => {
    const handleKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        e.preventDefault();
        e.stopPropagation();
        if (searchVisible) {
          setSearchVisible(false);
          setSearchQuery('');
        } else {
          onClose();
        }
      }
      if (e.ctrlKey && e.key === 'f') {
        e.preventDefault();
        setSearchVisible(true);
      }
    };
    window.addEventListener('keydown', handleKey, true);
    return () => window.removeEventListener('keydown', handleKey, true);
  }, [onClose, searchVisible]);

  useEffect(() => {
    if (searchVisible && searchInputRef.current) {
      searchInputRef.current.focus();
    }
  }, [searchVisible]);

  const highlightedOutput = useMemo(() => {
    if (!searchQuery) return output;
    const parts = output.split(searchQuery);
    const limited = parts.slice(0, MAX_HIGHLIGHTS);
    const result: React.ReactNode[] = [];
    for (let i = 0; i < limited.length; i++) {
      result.push(limited[i]);
      if (i < limited.length - 1) {
        result.push(<mark key={`m-${i}`} className="bg-cistern-yellow/40 text-cistern-fg">{searchQuery}</mark>);
      }
    }
    if (parts.length > MAX_HIGHLIGHTS) {
      result.push(<span key="truncated" className="text-cistern-muted">… ({parts.length - MAX_HIGHLIGHTS} more matches)</span>);
      result.push(parts[parts.length - 1]);
    }
    return result;
  }, [output, searchQuery]);

  return (
    <div className="fixed inset-y-0 right-0 w-full md:w-[600px] bg-cistern-bg border-l border-cistern-border shadow-2xl z-50 flex flex-col" role="dialog" aria-label={`Peek: ${aqueductName}`}>
      <div className="flex items-center justify-between px-4 py-3 border-b border-cistern-border">
        <div className="flex items-center gap-3">
          <h3 className="font-mono text-cistern-accent">{aqueductName}</h3>
          <span className="text-xs text-cistern-muted">Peek</span>
          <div className={`w-2 h-2 rounded-full ${connected ? 'bg-cistern-green' : 'bg-cistern-red'}`} />
        </div>
        <div className="flex items-center gap-2">
          <button
            onClick={() => setSearchVisible(!searchVisible)}
            className="text-xs px-2 py-1 rounded border border-cistern-border text-cistern-muted hover:text-cistern-fg transition-colors"
            title="Search (Ctrl+F)"
            aria-label="Toggle search"
          >
            🔍
          </button>
          <button
            onClick={() => setAutoScroll(!autoScroll)}
            className={`text-xs px-2 py-1 rounded border transition-colors ${
              autoScroll
                ? 'border-cistern-accent text-cistern-accent'
                : 'border-cistern-border text-cistern-muted hover:text-cistern-fg'
            }`}
            title={autoScroll ? 'Auto-scroll enabled' : 'Auto-scroll disabled'}
            aria-label={autoScroll ? 'Disable auto-scroll' : 'Enable auto-scroll'}
          >
            {autoScroll ? '⬇ Auto' : '⬇ Manual'}
          </button>
          <button
            onClick={onClose}
            className="text-cistern-muted hover:text-cistern-fg transition-colors text-lg leading-none"
            aria-label="Close peek"
          >
            ×
          </button>
        </div>
      </div>
      {searchVisible && (
        <div className="flex items-center gap-2 px-4 py-2 border-b border-cistern-border bg-cistern-surface">
          <input
            ref={searchInputRef}
            type="text"
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
            placeholder="Search output..."
            className="flex-1 bg-cistern-bg border border-cistern-border rounded px-2 py-1 text-sm text-cistern-fg placeholder-cistern-muted"
            aria-label="Search peek output"
          />
          {searchQuery && (
            <span className="text-xs text-cistern-muted font-mono">
              {Math.min(output.split(searchQuery).length - 1, MAX_HIGHLIGHTS)}{output.split(searchQuery).length - 1 > MAX_HIGHLIGHTS ? '+' : ''} match{output.split(searchQuery).length - 1 !== 1 ? 'es' : ''}
            </span>
          )}
        </div>
      )}
      {error && !connected && (
        <div className="flex-1 flex items-center justify-center p-4">
          <div className="text-center">
            <div className="text-cistern-red text-sm font-mono mb-2">{error}</div>
            <button
              onClick={() => setReconnectKey((k) => k + 1)}
              className="text-xs px-3 py-1 rounded border border-cistern-border text-cistern-muted hover:text-cistern-fg transition-colors"
            >
              Retry
            </button>
          </div>
        </div>
      )}
      {!error && (
        <TerminalView
          ref={terminalRef}
          content={highlightedOutput || 'Connecting\u2026'}
          autoScroll={false}
          className="flex-1 p-4"
          onScroll={handleScroll}
        />
      )}
    </div>
  );
}