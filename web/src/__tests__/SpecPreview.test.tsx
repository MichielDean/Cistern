import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/react';
import { SpecPreview } from '../components/SpecPreview';

describe('SpecPreview', () => {
  it('renders title and description', () => {
    render(<SpecPreview title="My Idea" description="A great feature" specSnapshot="" />);
    expect(screen.getByText(/My Idea/)).toBeInTheDocument();
    expect(screen.getByText(/A great feature/)).toBeInTheDocument();
  });

  it('shows placeholder when no spec snapshot', () => {
    render(<SpecPreview title="My Idea" description="" specSnapshot="" />);
    expect(screen.getByText(/Spec will appear here/)).toBeInTheDocument();
  });

  it('renders spec snapshot when provided', () => {
    render(<SpecPreview title="My Idea" description="" specSnapshot="1. Add feature X\n2. Write tests" />);
    expect(screen.getByText(/1. Add feature X/)).toBeInTheDocument();
  });
});