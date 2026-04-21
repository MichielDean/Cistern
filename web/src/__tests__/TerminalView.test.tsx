import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/react';
import { TerminalView } from '../components/TerminalView';

describe('TerminalView', () => {
  it('renders content in a monospace pre element', () => {
    const { container } = render(<TerminalView content="Hello, terminal!" />);
    const pre = container.querySelector('pre');
    expect(pre).toBeTruthy();
    expect(pre?.textContent).toBe('Hello, terminal!');
    expect(pre?.className).toContain('font-mono');
  });

  it('applies custom className', () => {
    const { container } = render(<TerminalView content="test" className="h-96" />);
    const pre = container.querySelector('pre');
    expect(pre?.className).toContain('h-96');
  });

  it('renders empty content', () => {
    const { container } = render(<TerminalView content="" />);
    const pre = container.querySelector('pre');
    expect(pre?.textContent).toBe('');
  });

  it('has overflow-auto for scrollable content', () => {
    const longContent = 'x'.repeat(10000);
    const { container } = render(<TerminalView content={longContent} />);
    const pre = container.querySelector('pre');
    expect(pre?.className).toContain('overflow-auto');
  });
});