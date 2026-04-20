import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { PipelineIndicator } from '../components/PipelineIndicator';

describe('PipelineIndicator', () => {
  it('renders all steps', () => {
    const steps = ['flag', 'implement', 'review', 'qa'];
    render(<PipelineIndicator steps={steps} currentIndex={1} isFlowing={true} />);

    expect(screen.getByText('flag')).toBeInTheDocument();
    expect(screen.getByText('implement')).toBeInTheDocument();
    expect(screen.getByText('review')).toBeInTheDocument();
    expect(screen.getByText('qa')).toBeInTheDocument();
  });

  it('renders nothing when steps is empty', () => {
    const { container } = render(<PipelineIndicator steps={[]} currentIndex={0} isFlowing={false} />);
    expect(container.querySelectorAll('button').length).toBe(0);
  });

  it('calls onStepClick when a step is clicked', () => {
    const steps = ['flag', 'implement', 'review'];
    const onStepClick = vi.fn();
    render(<PipelineIndicator steps={steps} currentIndex={1} isFlowing={true} onStepClick={onStepClick} />);

    fireEvent.click(screen.getByText('review'));
    expect(onStepClick).toHaveBeenCalledWith('review');
  });

  it('renders progress bar when flowing with progress', () => {
    const steps = ['flag', 'implement', 'review', 'qa'];
    const { container } = render(<PipelineIndicator steps={steps} currentIndex={2} isFlowing={true} />);

    const progressBar = container.querySelector('.bg-cistern-accent.transition-all');
    expect(progressBar).toBeInTheDocument();
  });

  it('does not render progress bar when not flowing', () => {
    const steps = ['flag', 'implement', 'review', 'qa'];
    render(<PipelineIndicator steps={steps} currentIndex={2} isFlowing={false} />);

    const progressContainer = document.querySelector('.h-1\\.5.bg-cistern-border');
    expect(progressContainer).not.toBeInTheDocument();
  });

  it('marks completed steps before current index', () => {
    const steps = ['flag', 'implement', 'review'];
    render(<PipelineIndicator steps={steps} currentIndex={2} isFlowing={true} />);

    const flagBtn = screen.getByText('flag').closest('button');
    const implementBtn = screen.getByText('implement').closest('button');

    expect(flagBtn?.className).toContain('bg-cistern-green');
    expect(implementBtn?.className).toContain('bg-cistern-green');
  });

  it('marks current step with active indicator', () => {
    const steps = ['flag', 'implement', 'review'];
    render(<PipelineIndicator steps={steps} currentIndex={1} isFlowing={true} />);

    const currentBtn = screen.getByText('implement').closest('button');
    expect(currentBtn?.className).toContain('water-flow-active');
  });
});