import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/react';
import { JiraSetupNote } from '../components/JiraSetupNote';

describe('JiraSetupNote', () => {
  it('renders credential instructions', () => {
    render(<JiraSetupNote />);
    expect(screen.getByText(/Jira Credentials Required/)).toBeInTheDocument();
    expect(screen.getByText(/JIRA_API_TOKEN/)).toBeInTheDocument();
  });

  it('links to doctor page', () => {
    render(<JiraSetupNote />);
    const link = screen.getByText(/Check credential status/);
    expect(link).toBeInTheDocument();
    expect(link.closest('a')?.getAttribute('href')).toBe('/app/doctor');
  });
});