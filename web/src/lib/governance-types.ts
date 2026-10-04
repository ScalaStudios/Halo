export type NamedRef = { id: string; name: string; email?: string };

export type AccessPackage = {
  id: string;
  name: string;
  description: string;
  owner: NamedRef | null;
  groups: NamedRef[];
  approvers: NamedRef[];
  maxDays: number | null;
  requireJustification: boolean;
  archived: boolean;
  pendingRequests: number;
  activeGrants: number;
  createdAt: string;
};

export type RequestStatus = "pending" | "approved" | "denied" | "cancelled";

export type AccessRequest = {
  id: string;
  package: NamedRef;
  requester: NamedRef;
  justification: string;
  durationDays: number | null;
  status: RequestStatus;
  decidedBy: NamedRef | null;
  decidedAt: string | null;
  decisionNote: string;
  expiresAt: string | null;
  endedAt: string | null;
  endReason: "expired" | "revoked" | null;
  groups: NamedRef[];
  approvers: NamedRef[];
  createdAt: string;
  canDecide: boolean;
};

export type ReviewStatus = "in-progress" | "overdue" | "completed";
export type ReviewDecision = "keep" | "remove";
export type ReviewOutcome = "kept" | "removed" | "not-removed" | "no-decision";

export type ReviewItem = {
  user: NamedRef;
  title: string;
  decision: ReviewDecision | null;
  note: string;
  decidedBy: NamedRef | null;
  decidedAt: string | null;
  outcome: ReviewOutcome | null;
};

export type AccessReview = {
  id: string;
  name: string;
  group: NamedRef;
  reviewers: NamedRef[];
  dueAt: string;
  autoApply: boolean;
  status: ReviewStatus;
  total: number;
  decided: number;
  removals: number;
  createdBy: NamedRef | null;
  createdAt: string;
  completedBy: NamedRef | null;
  completedAt: string | null;
  items?: ReviewItem[];
  canDecide: boolean;
  canComplete: boolean;
};

export type LifecycleTrigger = "joiner" | "mover" | "leaver";
export type LifecycleActionType = "add-to-group" | "remove-from-group" | "revoke-sessions" | "suspend";
export type LifecycleAction = { type: LifecycleActionType; groupId?: string };

export type LifecycleRule = {
  id: string;
  name: string;
  trigger: LifecycleTrigger;
  condition: string | null;
  actions: LifecycleAction[];
  enabled: boolean;
  runs: number;
  lastRunAt: string | null;
  createdAt: string;
};

export type LifecycleRun = {
  id: string;
  rule: NamedRef;
  user: NamedRef;
  trigger: LifecycleTrigger;
  change: string;
  steps: string[];
  result: "succeeded" | "failed";
  createdAt: string;
};
