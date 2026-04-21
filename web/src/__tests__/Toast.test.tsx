import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, fireEvent, act } from '@testing-library/react';
import { ToastProvider, useToast } from '../components/Toast';

function ToastTrigger({ message, type, duration }: { message: string; type?: 'success' | 'error' | 'info'; duration?: number }) {
  const { addToast } = useToast();
  return (
    <button
      onClick={() => addToast(message, type, duration)}
      data-testid="trigger"
    >
      Show Toast
    </button>
  );
}

describe('Toast', () => {
  beforeEach(() => {
    vi.useFakeTimers();
  });
  afterEach(() => {
    vi.useRealTimers();
  });

  it('shows a toast when addToast is called', () => {
    render(
      <ToastProvider>
        <ToastTrigger message="Hello world" type="info" duration={0} />
      </ToastProvider>,
    );
    fireEvent.click(screen.getByTestId('trigger'));
    expect(screen.getByText('Hello world')).toBeInTheDocument();
  });

  it('renders different toast types with correct styles', () => {
    render(
      <ToastProvider>
        <ToastTrigger message="Error occurred" type="error" duration={0} />
      </ToastProvider>,
    );
    fireEvent.click(screen.getByTestId('trigger'));
    const toast = screen.getByText('Error occurred').closest('div');
    expect(toast?.className).toContain('bg-cistern-red');
  });

  it('removes toast when dismiss button is clicked', () => {
    render(
      <ToastProvider>
        <ToastTrigger message="Dismissible" type="info" duration={0} />
      </ToastProvider>,
    );
    fireEvent.click(screen.getByTestId('trigger'));
    expect(screen.getByText('Dismissible')).toBeInTheDocument();
    fireEvent.click(screen.getByLabelText('Dismiss'));
    expect(screen.queryByText('Dismissible')).not.toBeInTheDocument();
  });

  it('auto-removes toast after duration', () => {
    render(
      <ToastProvider>
        <ToastTrigger message="Timed" type="info" duration={1000} />
      </ToastProvider>,
    );
    fireEvent.click(screen.getByTestId('trigger'));
    expect(screen.getByText('Timed')).toBeInTheDocument();
    act(() => {
      vi.advanceTimersByTime(1000);
    });
    expect(screen.queryByText('Timed')).not.toBeInTheDocument();
  });

  it('renders no toasts initially', () => {
    const { container } = render(
      <ToastProvider>
        <div>No toasts</div>
      </ToastProvider>,
    );
    expect(container.querySelectorAll('[role="status"]').length).toBe(0);
  });
});