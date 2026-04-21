import { useState, useCallback, useEffect } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import {
  useDroplet,
  useDropletNotes,
  useDropletIssues,
  useDropletDependencies,
  useDropletMutation,
  useRepoSteps,
  resolveIssue,
  rejectIssue,
  renameDroplet,
} from '../hooks/useApi';
import { StatusBadge } from '../components/StatusBadge';
import { PipelineIndicator } from '../components/PipelineIndicator';
import { NotesTimeline } from '../components/NotesTimeline';
import { IssuesList } from '../components/IssuesList';
import { DependenciesList } from '../components/DependenciesList';
import { ActionDialog } from '../components/ActionDialog';
import { AddNoteModal } from '../components/AddNoteModal';
import { EditMetadataModal } from '../components/EditMetadataModal';
import { RestartModal } from '../components/RestartModal';
import { RenameInput } from '../components/RenameInput';
import { FileIssueModal } from '../components/FileIssueModal';
import { CloseModal } from '../components/CloseModal';
import { ReopenModal } from '../components/ReopenModal';
import { SkeletonCard, SkeletonLine } from '../components/LoadingSkeleton';
import { useToast } from '../components/Toast';
import { formatAge } from '../utils/formatAge';
import type { ActionRequest, Droplet, DropletIssue } from '../api/types';

export function DropletDetail() {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const { droplet, loading, error, refetch: refetchDroplet } = useDroplet(id ?? null);
  const { notes, loading: notesLoading, refetch: refetchNotes } = useDropletNotes(id ?? null, useCallback((d: Droplet) => {
    setSseDroplet(d);
  }, []));
  const { issues, loading: issuesLoading, refetch: refetchIssues } = useDropletIssues(id ?? null);
  const { dependencies, loading: depsLoading, refetch: refetchDeps } = useDropletDependencies(id ?? null);
  const { mutate } = useDropletMutation();
  const { addToast } = useToast();

  const [sseDroplet, setSseDroplet] = useState<Droplet | null>(null);
  const currentDroplet = sseDroplet ?? droplet;

  useEffect(() => {
    setSseDroplet(null);
  }, [id]);
  const { steps: pipelineSteps } = useRepoSteps(currentDroplet?.repo ?? null);

  const [actionDialog, setActionDialog] = useState<{
    open: boolean;
    title: string;
    action: string;
  }>({ open: false, title: '', action: '' });
  const [showNoteModal, setShowNoteModal] = useState(false);
  const [showEditModal, setShowEditModal] = useState(false);
  const [showRestartModal, setShowRestartModal] = useState(false);
  const [showFileIssueModal, setShowFileIssueModal] = useState(false);
  const [showCloseModal, setShowCloseModal] = useState(false);
  const [showReopenModal, setShowReopenModal] = useState(false);
  const [copied, setCopied] = useState(false);
  const [submitting, setSubmitting] = useState<string | null>(null);

  const refetchAll = useCallback(() => {
    refetchDroplet();
    refetchNotes();
    refetchIssues();
    refetchDeps();
  }, [refetchDroplet, refetchNotes, refetchIssues, refetchDeps]);

  const handleAction = useCallback(async (
    dropletId: string,
    action: string,
    body?: ActionRequest
  ) => {
    setSubmitting(action);
    try {
      await mutate(dropletId, action, body);
      refetchAll();
    } catch (err) {
      addToast(err instanceof Error ? err.message : `${action} failed`, 'error');
    } finally {
      setSubmitting(null);
    }
  }, [mutate, refetchAll, addToast]);

  const handleRename = useCallback(async (newTitle: string) => {
    try {
      await renameDroplet(id!, newTitle);
      refetchAll();
    } catch (err) {
      addToast(err instanceof Error ? err.message : 'Rename failed', 'error');
    }
  }, [id, refetchAll, addToast]);

  const handleCopyId = useCallback(() => {
    if (currentDroplet?.id) {
      navigator.clipboard.writeText(currentDroplet.id).then(() => {
        setCopied(true);
        setTimeout(() => setCopied(false), 2000);
      }).catch(() => {
        addToast('Failed to copy ID', 'error');
      });
    }
  }, [currentDroplet?.id, addToast]);

  if (error) {
    return (
      <div className="flex items-center justify-center h-full">
        <div className="text-center">
          <div className="text-cistern-red text-lg font-mono mb-2">Error loading droplet</div>
          <div className="text-cistern-muted text-sm">{error.message}</div>
        </div>
      </div>
    );
  }

  if (loading || !currentDroplet) {
    return (
      <div className="flex-1 p-4 md:p-6 space-y-6">
        <SkeletonLine width="48%" />
        <SkeletonLine width="64%" />
        <div className="grid grid-cols-1 lg:grid-cols-2 gap-4 mt-4">
          <SkeletonCard lines={5} />
          <SkeletonCard lines={5} />
        </div>
      </div>
    );
  }

  const d = currentDroplet;
  const isInProgress = d.status === 'in_progress';
  const isOpen = d.status === 'open';
  const isPooled = d.status === 'pooled';
  const isDelivered = d.status === 'delivered';
  const isCancelled = d.status === 'cancelled';

  const steps = pipelineSteps.length > 0 ? pipelineSteps : (d.current_cataractae ? [d.current_cataractae] : []);
  const currentStepIndex = steps.indexOf(d.current_cataractae);

  return (
    <div className="flex-1 overflow-y-auto p-4 md:p-6 space-y-6">
      <div className="flex items-start justify-between gap-4 flex-wrap">
        <div className="space-y-1 min-w-0">
          <div className="flex items-center gap-2 flex-wrap">
            <RenameInput value={d.title} onSave={handleRename} />
            <StatusBadge status={d.status} size="md" />
          </div>
          <div className="flex items-center gap-2 text-xs text-cistern-muted font-mono">
            <button
              type="button"
              onClick={handleCopyId}
              className="flex items-center gap-1 hover:text-cistern-accent transition-colors"
              title="Copy ID"
            >
              {d.id}
              <span className="text-[10px]">{copied ? '✓' : '📋'}</span>
            </button>
            <span className="text-cistern-border">|</span>
            <span className="bg-cistern-border/30 px-1.5 py-0.5 rounded text-cistern-accent">{d.repo}</span>
            <span className="text-cistern-border">|</span>
            <span>Priority {d.priority}</span>
            {d.complexity > 0 && <><span className="text-cistern-border">|</span><span>Complexity {d.complexity}</span></>}
            <span className="text-cistern-border">|</span><span>Created {formatAge(d.created_at)}</span>
            {d.stage_dispatched_at && <><span className="text-cistern-border">|</span><span>Stage {formatAge(d.stage_dispatched_at)}</span></>}
          </div>
        </div>
        <div className="flex items-center gap-2 flex-wrap">
          {isInProgress && (
            <>
              <button type="button" disabled={!!submitting} onClick={() => setActionDialog({ open: true, title: 'Pass Droplet', action: 'pass' })} className="px-3 py-1.5 text-sm rounded bg-cistern-green/20 text-cistern-green border border-cistern-green/40 hover:bg-cistern-green/30 transition-colors disabled:opacity-50">Pass</button>
              <button type="button" disabled={!!submitting} onClick={() => setActionDialog({ open: true, title: 'Recirculate Droplet', action: 'recirculate' })} className="px-3 py-1.5 text-sm rounded bg-cistern-yellow/20 text-cistern-yellow border border-cistern-yellow/40 hover:bg-cistern-yellow/30 transition-colors disabled:opacity-50">Recirculate</button>
              <button type="button" disabled={!!submitting} onClick={() => setActionDialog({ open: true, title: 'Pool Droplet', action: 'pool' })} className="px-3 py-1.5 text-sm rounded bg-cistern-red/20 text-cistern-red border border-cistern-red/40 hover:bg-cistern-red/30 transition-colors disabled:opacity-50">Pool</button>
            </>
          )}
          {isOpen && (
            <>
              <button type="button" disabled={!!submitting} onClick={() => setActionDialog({ open: true, title: 'Cancel Droplet', action: 'cancel' })} className="px-3 py-1.5 text-sm rounded bg-cistern-red/20 text-cistern-red border border-cistern-red/40 hover:bg-cistern-red/30 transition-colors disabled:opacity-50">Cancel</button>
              <button type="button" disabled={!!submitting} onClick={() => setActionDialog({ open: true, title: 'Approve Droplet', action: 'approve' })} className="px-3 py-1.5 text-sm rounded bg-cistern-green/20 text-cistern-green border border-cistern-green/40 hover:bg-cistern-green/30 transition-colors disabled:opacity-50">Approve</button>
            </>
          )}
          {(isPooled || isDelivered || isCancelled) && (
            <button type="button" disabled={!!submitting} onClick={() => setShowReopenModal(true)} className="px-3 py-1.5 text-sm rounded bg-cistern-accent/20 text-cistern-accent border border-cistern-accent/40 hover:bg-cistern-accent/30 transition-colors disabled:opacity-50">{submitting === 'reopen' ? '⟳' : 'Reopen'}</button>
          )}
          {isInProgress && (
            <button type="button" disabled={!!submitting} onClick={() => setShowCloseModal(true)} className="px-3 py-1.5 text-sm rounded bg-cistern-accent/20 text-cistern-accent border border-cistern-accent/40 hover:bg-cistern-accent/30 transition-colors disabled:opacity-50">{submitting === 'close' ? '⟳' : 'Deliver'}</button>
          )}
          <button type="button" onClick={() => navigate('/app/droplets')} className="px-3 py-1.5 text-sm rounded border border-cistern-border text-cistern-muted hover:text-cistern-fg transition-colors">Back</button>
        </div>
      </div>

      {d.current_cataractae && (
        <section className="bg-cistern-surface border border-cistern-border rounded-lg p-4">
          <h2 className="text-sm font-mono text-cistern-muted uppercase tracking-wider mb-3">Pipeline Position</h2>
          <PipelineIndicator
            steps={steps}
            currentIndex={currentStepIndex}
            isFlowing={isInProgress}
          />
          <div className="mt-2 text-xs text-cistern-muted font-mono">
            Current step: <span className="text-cistern-accent">{d.current_cataractae}</span>
          </div>
        </section>
      )}

      {d.description && (
        <section className="bg-cistern-surface border border-cistern-border rounded-lg p-4">
          <h2 className="text-sm font-mono text-cistern-muted uppercase tracking-wider mb-2">Description</h2>
          <div className="text-sm text-cistern-fg whitespace-pre-wrap">{d.description}</div>
        </section>
      )}

      <div className="grid grid-cols-1 lg:grid-cols-2 gap-4">
        <section className="bg-cistern-surface border border-cistern-border rounded-lg p-4">
          <div className="flex items-center justify-between mb-3">
            <h2 className="text-sm font-mono text-cistern-muted uppercase tracking-wider">Notes</h2>
            <button type="button" onClick={() => setShowNoteModal(true)} className="text-xs px-2 py-1 rounded border border-cistern-border text-cistern-muted hover:text-cistern-fg transition-colors">+ Add Note</button>
          </div>
          <NotesTimeline notes={notes} loading={notesLoading} />
        </section>

        <section className="bg-cistern-surface border border-cistern-border rounded-lg p-4">
          <div className="flex items-center justify-between mb-3">
            <h2 className="text-sm font-mono text-cistern-muted uppercase tracking-wider">Issues</h2>
            <button type="button" onClick={() => setShowFileIssueModal(true)} className="text-xs px-2 py-1 rounded border border-cistern-border text-cistern-muted hover:text-cistern-fg transition-colors">+ File Issue</button>
          </div>
          <IssuesList
            issues={issues}
            loading={issuesLoading}
            onResolve={async (issueId, ev) => {
              try {
                await resolveIssue(issueId, { evidence: ev });
                refetchAll();
              } catch (err) {
                addToast(err instanceof Error ? err.message : 'Resolve issue failed', 'error');
              }
            }}
            onReject={async (issueId, ev) => {
              try {
                await rejectIssue(issueId, { evidence: ev });
                refetchAll();
              } catch (err) {
                addToast(err instanceof Error ? err.message : 'Reject issue failed', 'error');
              }
            }}
          />
        </section>
      </div>

      <section className="bg-cistern-surface border border-cistern-border rounded-lg p-4">
        <div className="flex items-center justify-between mb-3">
          <h2 className="text-sm font-mono text-cistern-muted uppercase tracking-wider">Dependencies</h2>
        </div>
        <DependenciesList
          dropletId={d.id}
          dependencies={dependencies}
          loading={depsLoading}
          onChange={() => refetchAll()}
        />
      </section>

      <div className="flex items-center gap-2 flex-wrap">
        <button type="button" onClick={() => setShowRestartModal(true)} className="px-3 py-1.5 text-sm rounded border border-cistern-border text-cistern-muted hover:text-cistern-fg transition-colors">Restart</button>
        <button type="button" onClick={() => setShowEditModal(true)} className="px-3 py-1.5 text-sm rounded border border-cistern-border text-cistern-muted hover:text-cistern-fg transition-colors">Edit Metadata</button>
      </div>

      <ActionDialog
        open={actionDialog.open}
        onClose={() => setActionDialog((prev) => ({ ...prev, open: false }))}
        title={actionDialog.title}
        action={actionDialog.action}
        dropletId={d.id}
        showNotes={actionDialog.action === 'recirculate' || actionDialog.action === 'pool'}
        showTargetSelector={actionDialog.action === 'recirculate'}
        steps={steps}
        onConfirm={handleAction}
      />

      <AddNoteModal
        open={showNoteModal}
        onClose={() => setShowNoteModal(false)}
        dropletId={d.id}
        onSaved={() => refetchAll()}
      />

      <EditMetadataModal
        open={showEditModal}
        onClose={() => setShowEditModal(false)}
        droplet={d}
        onSaved={() => refetchAll()}
      />

      <RestartModal
        open={showRestartModal}
        onClose={() => setShowRestartModal(false)}
        dropletId={d.id}
        steps={steps}
        onRestarted={() => refetchAll()}
      />

      <FileIssueModal
        open={showFileIssueModal}
        onClose={() => setShowFileIssueModal(false)}
        dropletId={d.id}
        onFiled={(_issue: DropletIssue) => refetchAll()}
      />

      <CloseModal
        open={showCloseModal}
        onClose={() => setShowCloseModal(false)}
        dropletId={d.id}
        onConfirm={async (dropletId: string) => {
          try {
            await mutate(dropletId, 'close');
            refetchAll();
          } catch (err) {
            addToast(err instanceof Error ? err.message : 'Close failed', 'error');
          }
        }}
      />

      <ReopenModal
        open={showReopenModal}
        onClose={() => setShowReopenModal(false)}
        dropletId={d.id}
        onConfirm={async (dropletId: string) => {
          try {
            await mutate(dropletId, 'reopen');
            refetchAll();
          } catch (err) {
            addToast(err instanceof Error ? err.message : 'Reopen failed', 'error');
          }
        }}
      />
    </div>
  );
}