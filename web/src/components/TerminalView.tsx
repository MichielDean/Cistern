import { useRef, useEffect } from 'react';

interface TerminalViewProps {
  content: string;
  autoScroll?: boolean;
  className?: string;
}

export function TerminalView({ content, autoScroll = true, className = '' }: TerminalViewProps) {
  const containerRef = useRef<HTMLPreElement>(null);

  useEffect(() => {
    if (autoScroll && containerRef.current) {
      containerRef.current.scrollTop = containerRef.current.scrollHeight;
    }
  }, [content, autoScroll]);

  return (
    <pre
      ref={containerRef}
      className={`overflow-auto font-mono text-xs text-cistern-green bg-cistern-bg whitespace-pre-wrap break-all ${className}`}
    >
      {content}
    </pre>
  );
}