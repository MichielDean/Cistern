import { useRef, useEffect, forwardRef, useImperativeHandle } from 'react';

interface TerminalViewProps {
  content: React.ReactNode;
  autoScroll?: boolean;
  className?: string;
  onScroll?: () => void;
}

export const TerminalView = forwardRef<HTMLPreElement, TerminalViewProps>(
  function TerminalView({ content, autoScroll = true, className = '', onScroll }, ref) {
    const containerRef = useRef<HTMLPreElement>(null);

    useImperativeHandle(ref, () => containerRef.current!, []);

    useEffect(() => {
      if (autoScroll && containerRef.current) {
        containerRef.current.scrollTop = containerRef.current.scrollHeight;
      }
    }, [content, autoScroll]);

    return (
      <pre
        ref={containerRef}
        onScroll={onScroll}
        className={`overflow-auto font-mono text-xs text-cistern-green bg-cistern-bg whitespace-pre-wrap break-all ${className}`}
      >
        {content}
      </pre>
    );
  }
);