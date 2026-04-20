import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/react';
import { MessageBubble } from '../components/MessageBubble';
import type { FilterMessage } from '../api/types';

describe('MessageBubble', () => {
  const assistantMessage: FilterMessage = { role: 'assistant', content: 'Hello! How can I help?' };
  const userMessage: FilterMessage = { role: 'user', content: 'I need help with filtering' };

  it('renders assistant message with CT avatar', () => {
    render(<MessageBubble message={assistantMessage} />);
    expect(screen.getByText('CT')).toBeInTheDocument();
    expect(screen.getByText('Hello! How can I help?')).toBeInTheDocument();
  });

  it('renders user message with U avatar', () => {
    render(<MessageBubble message={userMessage} />);
    expect(screen.getByText('U')).toBeInTheDocument();
    expect(screen.getByText('I need help with filtering')).toBeInTheDocument();
  });
});