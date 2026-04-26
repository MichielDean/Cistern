import { describe, it, expect } from 'vitest';
import { render } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { AqueductArch } from '../components/AqueductArch';
import type { CataractaeInfo, FlowActivity } from '../api/types';

const baseCataractae: CataractaeInfo = {
  name: 'test-aqueduct',
  repo_name: 'test-repo',
  droplet_id: 'droplet-1',
  title: 'Test Droplet',
  step: 'implement',
  steps: ['dispatch', 'implement', 'review'],
  elapsed: 0,
  stage_elapsed: 0,
  cataractae_index: 2,
  total_cataractae: 3,
};

const activityWithNullNotes = {
  droplet_id: 'droplet-1',
  title: 'Test Droplet',
  step: 'implement',
  recent_notes: null as unknown as FlowActivity['recent_notes'],
} satisfies FlowActivity;

describe('AqueductArch null-array regression', () => {
  it('renders without crashing when activity.recent_notes is null', () => {
    expect(() =>
      render(
        <MemoryRouter>
          <AqueductArch
            cataractae={baseCataractae}
            activity={activityWithNullNotes}
            isFlowing={true}
            onPeek={() => {}}
          />
        </MemoryRouter>
      )
    ).not.toThrow();
  });

  it('renders cataractae name when activity.recent_notes is null', () => {
    const { container } = render(
      <MemoryRouter>
        <AqueductArch
          cataractae={baseCataractae}
          activity={activityWithNullNotes}
          isFlowing={true}
          onPeek={() => {}}
        />
      </MemoryRouter>
    );
    expect(container.textContent).toContain('test-aqueduct');
  });
});