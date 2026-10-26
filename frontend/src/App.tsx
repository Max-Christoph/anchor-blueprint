import { useCallback, useEffect, useState } from "react";

import { DataTable } from "./components/DataTable";
import { Layout } from "./components/Layout";
import { MetricCard } from "./components/MetricCard";
import { Notice, type NoticeState } from "./components/Notice";
import { ProcessPanel } from "./components/ProcessPanel";
import { RoleSwitcher } from "./components/RoleSwitcher";
import type { BadgeTone } from "./components/StatusBadge";
import { ApiError, api, describeError, setActiveRoles } from "./services/api";
import type { CalcResult, DataResponse, MeResponse, StatusResponse } from "./services/types";

const POLL_INTERVAL_MS = 15_000;

const numberFormat = new Intl.NumberFormat("de-DE", { maximumFractionDigits: 2 });

export default function App() {
  const [roles, setRoles] = useState<string[]>(["User"]);
  const [status, setStatus] = useState<StatusResponse | null>(null);
  const [statusError, setStatusError] = useState<string | null>(null);
  const [me, setMe] = useState<MeResponse | null>(null);
  const [data, setData] = useState<DataResponse | null>(null);
  const [dataError, setDataError] = useState<string | null>(null);
  const [dataLoading, setDataLoading] = useState<boolean>(false);
  const [result, setResult] = useState<CalcResult | null>(null);
  const [processError, setProcessError] = useState<string | null>(null);
  const [processLoading, setProcessLoading] = useState<boolean>(false);
  const [notice, setNotice] = useState<NoticeState | null>(null);
  const [actionBusy, setActionBusy] = useState<boolean>(false);

  const loadStatus = useCallback(async (): Promise<void> => {
    try {
      const next = await api.status();
      setStatus(next);
      setStatusError(null);
    } catch (error) {
      setStatus(null);
      setStatusError(describeError(error));
    }
  }, []);

  const loadMe = useCallback(async (): Promise<void> => {
    try {
      setMe(await api.me());
    } catch {
      setMe(null);
    }
  }, []);

  const loadData = useCallback(async (): Promise<void> => {
    setDataLoading(true);
    try {
      const next = await api.data();
      setData(next);
      setDataError(null);
    } catch (error) {
      setData(null);
      setDataError(describeError(error));
    } finally {
      setDataLoading(false);
    }
  }, []);

  const runProcess = useCallback(async (values: number[]): Promise<void> => {
    setProcessLoading(true);
    setProcessError(null);
    try {
      const response = await api.process(values);
      setResult(response.result);
    } catch (error) {
      setResult(null);
      setProcessError(describeError(error));
    } finally {
      setProcessLoading(false);
    }
  }, []);

  const runOperatorAction = useCallback(async (): Promise<void> => {
    setActionBusy(true);
    try {
      const response = await api.operatorAction();
      setNotice({ tone: "ok", text: `200 OK — ${response.message} (Rollen: ${response.roles.join(", ")})` });
    } catch (error) {
      setNotice({ tone: "error", text: describeError(error) });
    } finally {
      setActionBusy(false);
    }
  }, []);

  const runAdminReset = useCallback(async (): Promise<void> => {
    setActionBusy(true);
    try {
      const response = await api.adminReset();
      setNotice({ tone: "ok", text: `200 OK — ${response.message} (${response.performed_by})` });
    } catch (error) {
      if (error instanceof ApiError && error.status === 403) {
        setNotice({
          tone: "error",
          text: `403 Forbidden — Rolle "Admin" fehlt. Aktiv: ${roles.join(", ") || "keine"}.`,
        });
      } else {
        setNotice({ tone: "error", text: describeError(error) });
      }
    } finally {
      setActionBusy(false);
    }
  }, [roles]);

  // Rollenwechsel wirkt auf alle folgenden Requests.
  useEffect(() => {
    setActiveRoles(roles);
    void loadStatus();
    void loadMe();
    void loadData();
  }, [roles, loadStatus, loadMe, loadData]);

  useEffect(() => {
    const timer = window.setInterval(() => {
      void loadStatus();
    }, POLL_INTERVAL_MS);
    return () => window.clearInterval(timer);
  }, [loadStatus]);

  const backendReachable = statusError === null && status !== null;
  const databaseUp = backendReachable && status.database.status === "up";

  const badgeTone: BadgeTone = !backendReachable ? "error" : databaseUp ? "ok" : "warn";
  const badgeLabel = !backendReachable ? "Backend offline" : databaseUp ? "Alle Systeme aktiv" : "DB eingeschränkt";

  return (
    <Layout
      statusTone={badgeTone}
      statusLabel={badgeLabel}
      version={status?.version ?? "—"}
      roles={me?.roles ?? roles}
    >
      {notice !== null && <Notice notice={notice} onDismiss={() => setNotice(null)} />}

      <section id="dashboard" className="mb-6 grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
        <MetricCard
          label="Backend"
          value={backendReachable ? "online" : "offline"}
          hint={backendReachable ? status.environment : (statusError ?? "Keine Antwort")}
        />
        <MetricCard
          label="Datenbank"
          value={databaseUp ? "up" : "down"}
          hint={databaseUp ? `${numberFormat.format(status.database.latency_ms)} ms Ping` : "Verbindung gestört"}
        />
        <MetricCard
          label="Uptime"
          value={status === null ? "—" : `${numberFormat.format(status.uptime_seconds)} s`}
          hint={status === null ? "—" : `Go ${status.go_version}`}
        />
        <MetricCard
          label="Auth"
          value={status?.auth_mode ?? "—"}
          hint={me === null ? "Identität wird geladen" : `${me.subject} · Rollen: ${me.roles.join(", ") || "keine"}`}
        />
      </section>

      <RoleSwitcher
        roles={roles}
        busy={actionBusy}
        onChange={(next) => {
          setNotice(null);
          setRoles(next);
        }}
        onOperatorAction={() => void runOperatorAction()}
        onAdminReset={() => void runAdminReset()}
      />

      <div className="grid gap-6 lg:grid-cols-2">
        <DataTable
          items={data?.items ?? []}
          loading={dataLoading}
          error={dataError}
          onReload={() => {
            void loadData();
          }}
        />
        <ProcessPanel loading={processLoading} error={processError} result={result} onRun={(values) => void runProcess(values)} />
      </div>
    </Layout>
  );
}
