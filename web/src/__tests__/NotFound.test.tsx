import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { NotFound } from '../pages/NotFound';

function renderNotFound() {
  return render(
    <MemoryRouter>
      <NotFound />
    </MemoryRouter>,
  );
}

describe('NotFound', () => {
  it('renders 404 heading', () => {
    renderNotFound();
    expect(screen.getByText('404')).toBeInTheDocument();
  });

  it('renders "Page not found" message', () => {
    renderNotFound();
    expect(screen.getByText('Page not found')).toBeInTheDocument();
  });

  it('renders a link back to dashboard', () => {
    renderNotFound();
    const link = screen.getByText('Go to Dashboard');
    expect(link).toBeInTheDocument();
    expect(link.getAttribute('href')).toBe('/app/');
  });
});