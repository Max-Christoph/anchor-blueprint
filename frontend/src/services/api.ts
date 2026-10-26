import type {
  ApiErrorBody,
  DataResponse,
  HealthResponse,
  ProcessResponse,
  StatusResponse,
} from "./types";

/** Basis-URL: leer = relative Pfade (Nginx proxied /api an das Backend). */
const BASE_URL: string = (import.meta.env.VITE_API_BASE_URL ?? "").replace(/\/+$/, "");

/** Identitaet fuer den Dev-Header-Modus (von der API im JWT-Modus ignoriert). */
const DEMO_USER_EMAIL: string = import.meta.env.VITE_DEMO_USER_EMAIL ?? "demo.user@example.com";

/** Optionaler Bearer-Token (Dev-Modus oder kurzlebiges Test-Token). */
const BEARER_TOKEN: string = import.meta.env.VITE_BEARER_TOKEN ?? "";

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
  health: (): Promise<HealthResponse> => request<HealthResponse>("/healthz"),

  status: (): Promise<StatusResponse> => request<StatusResponse>("/api/v1/status"),

  data: (): Promise<DataResponse> => request<DataResponse>("/api/v1/data"),

  process: (values: number[]): Promise<ProcessResponse> =>
    request<ProcessResponse>("/api/v1/process", {
      method: "POST",
      body: JSON.stringify({ values, operation: "summary" }),
    }),
};

/** Wandelt unbekannte Fehler in eine anzeigbare Meldung. */
export function describeError(error: unknown): string {
  if (error instanceof ApiError) {
    return `${error.code}: ${error.message}`;
  }
  if (error instanceof Error) {
    return error.message;
  }
  return "Unbekannter Fehler";
}
