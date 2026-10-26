import type {
  ApiErrorBody,
  AuditLogsResponse,
  ReadinessResponse,
  DataResponse,
  HealthResponse,
  MeResponse,
  OperatorResponse,
  ProcessResponse,
  ResetResponse,
  StatusResponse,
} from "./types";

/** Basis-URL: leer = relative Pfade (Nginx proxied /api an das Backend). */
const BASE_URL: string = (import.meta.env.VITE_API_BASE_URL ?? "").replace(/\/+$/, "");

/** Identitaet fuer den Dev-Header-Modus (von der API im JWT-Modus ignoriert). */
const DEMO_USER_EMAIL: string = import.meta.env.VITE_DEMO_USER_EMAIL ?? "demo.user@example.com";

/** Optionaler Bearer-Token (Dev-Modus oder kurzlebiges Test-Token). */
const BEARER_TOKEN: string = import.meta.env.VITE_BEARER_TOKEN ?? "";

/** Aktive Rollen fuer den Dev-Modus (RBAC-Demonstration im Browser). */
let activeRoles: string[] = [];

/** Nur im Dev-Modus sendet der Client X-User-Roles (im JWT-Modus sinnlos). */
let roleHeaderEnabled = true;

/** Schaltet den X-User-Roles-Header (im JWT-Modus: aus). */
export function setRoleHeaderEnabled(enabled: boolean): void {
  roleHeaderEnabled = enabled;
}

/** Setzt die Rollen, die als X-User-Roles mitgesendet werden. */
export function setActiveRoles(roles: string[]): void {
  activeRoles = [...roles];
}

/** Liest die aktuell aktiven Rollen. */
export function getActiveRoles(): string[] {
  return [...activeRoles];
}

/** Typisierter Fehler der API-Schicht. */
export class ApiError extends Error {
  readonly status: number;
  readonly code: string;

  constructor(status: number, code: string, message: string) {
    super(message);
    this.name = "ApiError";
    this.status = status;
    this.code = code;
  }
}

async function request<T>(path: string, init: RequestInit = {}): Promise<T> {
  const headers = new Headers(init.headers);
  headers.set("Accept", "application/json");
  if (BEARER_TOKEN !== "") {
    headers.set("Authorization", `Bearer ${BEARER_TOKEN}`);
  } else {
    headers.set("X-User-Email", DEMO_USER_EMAIL);
  }
  if (roleHeaderEnabled && activeRoles.length > 0) {
    headers.set("X-User-Roles", activeRoles.join(","));
  }
  if (init.body !== undefined) {
    headers.set("Content-Type", "application/json");
  }

  const response = await fetch(`${BASE_URL}${path}`, { ...init, headers });

  if (!response.ok) {
    let code = `http_${response.status}`;
    let message = response.statusText || "Request fehlgeschlagen";
    try {
      const body = (await response.json()) as ApiErrorBody;
      code = body.error.code;
      message = body.error.message;
    } catch {
      // Kein JSON-Fehlerkoerper (z. B. Proxy-Fehler) - Fallback bleibt aktiv.
    }
    throw new ApiError(response.status, code, message);
  }

  if (response.status === 204) {
    return undefined as unknown as T;
  }
  return (await response.json()) as T;
}

export const api = {
  livez: (): Promise<HealthResponse> => request<HealthResponse>("/livez"),

  readyz: (): Promise<ReadinessResponse> => request<ReadinessResponse>("/readyz"),

  status: (): Promise<StatusResponse> => request<StatusResponse>("/api/v1/status"),

  me: (): Promise<MeResponse> => request<MeResponse>("/api/v1/me"),

  data: (): Promise<DataResponse> => request<DataResponse>("/api/v1/data"),

  process: (values: number[]): Promise<ProcessResponse> =>
    request<ProcessResponse>("/api/v1/process", {
      method: "POST",
      body: JSON.stringify({ values, operation: "summary" }),
    }),

  operatorAction: (): Promise<OperatorResponse> => request<OperatorResponse>("/api/v1/protected/operator"),

  adminReset: (): Promise<ResetResponse> =>
    request<ResetResponse>("/api/v1/protected/admin/reset", { method: "POST" }),

  auditLogs: (limit = 10): Promise<AuditLogsResponse> =>
    request<AuditLogsResponse>(`/api/v1/protected/admin/audit-logs?limit=${limit}`),
};

/** Wandelt unbekannte Fehler in eine anzeigbare Meldung. */
export function describeError(error: unknown): string {
  if (error instanceof ApiError) {
    return `${error.status} ${error.code}: ${error.message}`;
  }
  if (error instanceof Error) {
    return error.message;
  }
  return "Unbekannter Fehler";
}
