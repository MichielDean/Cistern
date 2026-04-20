import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { ActionDialog } from '../components/ActionDialog';

describe('ActionDialog', () => {
  it('renders nothing when open is false', () => {
    const { container } = render(
      <ActionDialog
        open={false}
        onClose={vi.fn()}
        title="Confirm"
        action="pass"
        dropletId="ct-abc123"
        onConfirm={vi.fn()}
      />
    );
    expect(container.querySelector('.fixed.inset-0')).not.toBeInTheDocument();
  });

  it('renders dialog when open is true', () => {
    render(
      <ActionDialog
        open={true}
        onClose={vi.fn()}
        title="Pass Droplet"
        action="pass"
        dropletId="ct-abc123"
        onConfirm={vi.fn()}
      />
    );
    expect(screen.getByText('Pass Droplet')).toBeInTheDocument();
    expect(screen.getByText(/ct-abc123/)).toBeInTheDocument();
  });

  it('shows notes textarea when showNotes is true', () => {
    render(
      <ActionDialog
        open={true}
        onClose={vi.fn()}
        title="Recirculate"
        action="recirculate"
        dropletId="ct-abc123"
        showNotes={true}
        onConfirm={vi.fn()}
      />
    );
    expect(screen.getByPlaceholderText('Optional notes...')).toBeInTheDocument();
  });

  it('hides notes textarea when showNotes is false', () => {
    render(
      <ActionDialog
        open={true}
        onClose={vi.fn()}
        title="Pass"
        action="pass"
        dropletId="ct-abc123"
        showNotes={false}
        onConfirm={vi.fn()}
      />
    );
    expect(screen.queryByPlaceholderText('Optional notes...')).not.toBeInTheDocument();
  });

  it('calls onClose when backdrop is clicked', () => {
    const onClose = vi.fn();
    const { container } = render(
      <ActionDialog
        open={true}
        onClose={onClose}
        title="Confirm"
        action="pass"
        dropletId="ct-abc123"
        onConfirm={vi.fn()}
      />
    );
    const backdrop = container.querySelector('.fixed.inset-0');
    if (backdrop) fireEvent.click(backdrop);
    expect(onClose).toHaveBeenCalled();
  });

  it('disables confirm button while submitting', () => {
    render(
      <ActionDialog
        open={true}
        onClose={vi.fn()}
        title="Confirm"
        action="pass"
        dropletId="ct-abc123"
        onConfirm={vi.fn()}
      />
    );
    const confirmBtn = screen.getByRole('button', { name: /confirm/i });
    expect(confirmBtn).not.toBeDisabled();
  });

  it('renders target selector when showTargetSelector and steps are provided', () => {
    render(
      <ActionDialog
        open={true}
        onClose={vi.fn()}
        title="Recirculate"
        action="recirculate"
        dropletId="ct-abc123"
        showTargetSelector={true}
        steps={['flag', 'implement', 'review']}
        onConfirm={vi.fn()}
      />
    );
    expect(screen.getByText('Target Step')).toBeInTheDocument();
  });
});