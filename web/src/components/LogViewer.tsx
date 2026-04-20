import { useState, useEffect, useRef, useCallback } from 'react';
import type { LogEntry } from '../api/types';

const levelPattern = /\b(INFO|WARN|ERROR|DEBUG)\b/;

function parseLevel(raw: string): LogEntry['level'] {
  const m = raw.match(levelPattern);
  if (m) return m[1] as LogEntry['level'];
  return '';
}

function parseLogLines(lines: string[]): LogEntry[] {
  return lines.map((raw, i) => ({
    line: i + 1,
    level: parseLevel(raw),
    text: raw,
    raw,
  }));
}

const levelColor: Record<string, string> = {
  INFO: 'text-cyan-400',
  WARN: 'text-cistern-yellow',
  ERROR: 'text-cistern-red',
  DEBUG: 'text-cistern-muted/50',
  '': 'text-cistern-fg',
};

interface LogViewerProps {
  entries: LogEntry[];
  autoScroll?: boolean;
  onAutoScrollChange?: (v: boolean) => void;
  maxHeight?: string;
  searchQuery?: string;
}

export function LogViewer({ entries, autoScroll = true, onAutoScrollChange, maxHeight = '100%', searchQuery }: LogViewerProps) {
  const containerRef = useRef<HTMLDivElement>(null);
  const [internalAutoScroll, setInternalAutoScroll] = useState(autoScroll);

  useEffect(() => {
    setInternalAutoScroll(autoScroll);
  }, [autoScroll]);

  const scrollToBottom = useCallback(() => {
    if (containerRef.current) {
      containerRef.current.scrollTop = containerRef.current.scrollHeight;
    }
  }, []);

  useEffect(() => {
    if (internalAutoScroll) {
      scrollToBottom();
    }
  }, [entries, internalAutoScroll, scrollToBottom]);

  const handleScroll = () => {
    if (!containerRef.current) return;
    const { scrollTop, scrollHeight, clientHeight } = containerRef.current;
    if (scrollTop < scrollHeight - clientHeight - 40) {
      setInternalAutoScroll(false);
      onAutoScrollChange?.(false);
    }
  };

  const filtered = searchQuery
    ? entries.filter(e => e.raw.toLowerCase().includes(searchQuery.toLowerCase()))
    : entries;

  const highlightText = (text: string, query: string) => {
    if (!query) return text;
    const idx = text.toLowerCase().indexOf(query.toLowerCase());
    if (idx === -1) return text;
    return (
      <>
        {text.slice(0, idx)}
        <span className="bg-cistern-accent/30">{text.slice(idx, idx + query.length)}</span>
        {text.slice(idx + query.length)}
      </>
    );
  };

  return (
    <div
      ref={containerRef}
      onScroll={handleScroll}
      className="font-mono text-xs bg-cistern-bg text-cistern-fg overflow-y-auto"
      style={{ maxHeight }}
    >
      {filtered.map((entry) => (
        <div key={entry.line} className="flex hover:bg-cistern-surface/50">
          <span className="text-right pr-2 select-none text-cistern-muted w-12 shrink-0">
            {entry.line}
          </span>
          <span className={levelColor[entry.level] || levelColor['']}>
            {searchQuery ? highlightText(entry.text, searchQuery) : entry.text}
          </span>
        </div>
      ))}
    </div>
  );
}

export { parseLogLines };