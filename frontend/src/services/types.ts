/** Antworttypen des Go-Backends (Spiegel der JSON-Vertraege). */

export type HealthState = "up" | "down";

export interface HealthResponse {
  status: "ok" | "degraded";
  database: HealthState;
  version: string;
}

export interface DatabaseStatus {
  status: HealthState;
  latency_ms: number;
}

export interface StatusResponse {
  service: string;
  auth_mode: string;
  version: string;
  environment: string;
  uptime_seconds: number;
  go_version: string;
  goroutines: number;
  num_cpu: number;
  memory_alloc_mb: number;
  database: DatabaseStatus;
}

export interface Entry {
  id: number;
  name: string;
  category: string;
  amount_cents: number;
  created_at: string;
}

export interface DataResponse {
  items: Entry[];
  count: number;
}

export interface CalcResult {
  operation: string;
  count: number;
  sum: number;
  mean: number;
  median: number;
  min: number;
  max: number;
  stddev: number;
  p90: number;
  duration_ms: number;
}

export interface ProcessResponse {
  result: CalcResult;
  requested_by: string;
}

/** GET /api/v1/me */
export interface MeResponse {
  subject: string;
  email: string;
  roles: string[];
  groups: string[];
  auth_method: string;
}

/** GET /api/v1/protected/operator */
export interface OperatorResponse {
  message: string;
  subject: string;
  roles: string[];
  required_roles: string[];
}

/** POST /api/v1/protected/admin/reset */
export interface ResetResponse {
  message: string;
  reset: boolean;
  performed_by: string;
  performed_at: string;
}

/** GET /api/v1/protected/admin/audit-logs */
export interface AuditLogEntry {
  id: number;
  timestamp: string;
  request_id: string;
  user_id: string;
  action: string;
  resource: string;
  status_code: number;
  details: Record<string, unknown>;
}

export interface AuditLogsResponse {
  items: AuditLogEntry[];
  count: number;
}

export interface ApiErrorBody {
  error: {
    code: string;
    message: string;
    request_id?: string;
  };
}
