export interface Droplet {
  id: string;
  repo: string;
  title: string;
  description: string;
  priority: number;
  complexity: number;
  status: string;
  assignee: string;
  current_cataractae: string;
  outcome?: string;
  assigned_aqueduct?: string;
  last_reviewed_commit?: string;
  external_ref?: string;
  last_heartbeat_at?: string;
  created_at: string;
  updated_at: string;
  stage_dispatched_at?: string;
}

export interface CataractaeNote {
  id: number;
  droplet_id: string;
  cataractae_name: string;
  content: string;
  created_at: string;
}

export interface DropletIssue {
  id: string;
  droplet_id: string;
  flagged_by: string;
  flagged_at: string;
  description: string;
  status: 'open' | 'resolved' | 'unresolved';
  evidence?: string;
  resolved_at?: string;
}

export interface FlowActivity {
  droplet_id: string;
  title: string;
  step: string;
  recent_notes: CataractaeNote[];
}

export interface CataractaeInfo {
  name: string;
  repo_name: string;
  droplet_id: string;
  title: string;
  step: string;
  steps: string[];
  elapsed: number;
  stage_elapsed: number;
  cataractae_index: number;
  total_cataractae: number;
}

export interface DropletListResponse {
  droplets: Droplet[];
  total: number;
  page: number;
  per_page: number;
}

export interface DropletSearchResponse {
  droplets: Droplet[];
  total: number;
  page: number;
  per_page: number;
}

export interface DropletDependency {
  depends_on: string;
  type: 'blocked_by' | 'resolves' | 'blocks';
}

export interface CreateDropletRequest {
  repo: string;
  title: string;
  description?: string;
  priority?: number;
  complexity?: number;
  depends_on?: string[];
}

export interface EditDropletRequest {
  title?: string;
  description?: string;
  complexity?: number;
  priority?: number;
}

export interface ActionRequest {
  notes?: string;
  to?: string;
  cataractae?: string;
}

export interface AddNoteRequest {
  cataractae?: string;
  content: string;
}

export interface AddIssueRequest {
  flagged_by?: string;
  description: string;
}

export interface ResolveIssueRequest {
  evidence: string;
}

export interface DashboardData {
  cataractae_count: number;
  flowing_count: number;
  queued_count: number;
  done_count: number;
  cataractae: CataractaeInfo[];
  unassigned_items: Droplet[];
  cistern_items: Droplet[];
  pooled_items: Droplet[];
  recent_items: Droplet[];
  blocked_by_map: Record<string, string>;
  flow_activities: FlowActivity[];
  castellarius_running: boolean;
  fetched_at: string;
}

export interface AqueductBrief {
  name: string;
  steps: string[];
}

export interface AqueductStatus {
  name: string;
  status: 'idle' | 'flowing';
  droplet_id: string | null;
  droplet_title: string | null;
  current_step: string | null;
  elapsed: number;
}

export interface CastellariusStatus {
  running: boolean;
  pid: number | null;
  uptime_seconds: number | null;
  aqueducts: AqueductStatus[];
  castellarius_running: boolean;
}

export type DoctorCheckStatus = 'pass' | 'fail' | 'warn';

export interface DoctorCheck {
  name: string;
  status: DoctorCheckStatus;
  message: string;
  category: string;
}

export interface DoctorResult {
  checks: DoctorCheck[];
  summary: { total: number; passed: number };
  timestamp: string;
}

export interface LogEntry {
  line: number;
  level: 'INFO' | 'WARN' | 'ERROR' | 'DEBUG' | '';
  text: string;
  raw: string;
}

export interface LogSourceInfo {
  name: string;
  size_bytes: number;
  last_modified: string;
}

export interface RepoInfo {
  name: string;
  prefix: string;
  url: string;
  aqueduct_config: string | null;
  aqueducts: AqueductBrief[];
}

export interface SkillInfo {
  name: string;
  source_url: string;
  installed_at: string;
}

export interface FilterSession {
  id: string;
  title: string;
  description: string;
  messages: string;
  spec_snapshot: string;
  llm_session_id: string;
  created_at: string;
  updated_at: string;
}

export interface FilterMessage {
  role: 'user' | 'assistant';
  content: string;
}

export interface FilterNewResponse {
  session_id: string;
  llm_session_id: string;
  session: FilterSession;
  assistant_message: string;
}

export interface FilterResumeResponse {
  session_id: string;
  llm_session_id: string;
  assistant_message: string;
  spec_snapshot: string;
}

export interface ImportRequest {
  provider: string;
  key: string;
  repo: string;
  complexity?: number;
  priority?: number;
}