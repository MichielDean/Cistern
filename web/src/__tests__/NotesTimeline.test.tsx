import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/react';
import { NotesTimeline } from '../components/NotesTimeline';
import type { CataractaeNote } from '../api/types';

const mockNotes: CataractaeNote[] = [
  {
    id: 1,
    droplet_id: 'ct-abc123',
    cataractae_name: 'implement',
    content: 'Short note',
    created_at: '2026-04-19T00:00:00Z',
  },
  {
    id: 2,
    droplet_id: 'ct-abc123',
    cataractae_name: 'review',
    content: 'A'.repeat(250) + ' rest of the content',
    created_at: '2026-04-19T01:00:00Z',
  },
];

describe('NotesTimeline', () => {
  it('shows loading state', () => {
    render(<NotesTimeline notes={[]} loading={true} />);
    expect(screen.getByText('Loading notes…')).toBeInTheDocument();
  });

  it('shows empty state when no notes', () => {
    render(<NotesTimeline notes={[]} loading={false} />);
    expect(screen.getByText('No notes yet')).toBeInTheDocument();
  });

  it('renders notes with cataractae name', () => {
    render(<NotesTimeline notes={mockNotes} loading={false} />);
    expect(screen.getByText('implement')).toBeInTheDocument();
    expect(screen.getByText('review')).toBeInTheDocument();
  });

  it('truncates long notes with show more button', () => {
    render(<NotesTimeline notes={mockNotes} loading={false} />);
    expect(screen.getByText('Show more')).toBeInTheDocument();
  });

  it('shows full short notes without truncation', () => {
    const shortNotes: CataractaeNote[] = [
      { id: 1, droplet_id: 'ct-abc123', cataractae_name: 'implement', content: 'Short note', created_at: '2026-04-19T00:00:00Z' },
    ];
    render(<NotesTimeline notes={shortNotes} loading={false} />);
    expect(screen.getByText('Short note')).toBeInTheDocument();
    expect(screen.queryByText('Show more')).not.toBeInTheDocument();
  });
});