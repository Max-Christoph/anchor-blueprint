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

export interface ApiErrorBody {
  error: {
    code: string;
    message: string;
    request_id?: string;
  };
}
