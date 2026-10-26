import { useCallback, useEffect, useState } from "react";

import { DataTable } from "./components/DataTable";
import { Layout } from "./components/Layout";
import { MetricCard } from "./components/MetricCard";
import { ProcessPanel } from "./components/ProcessPanel";
import type { BadgeTone } from "./components/StatusBadge";
import { api, describeError } from "./services/api";
import type { CalcResult, DataResponse, StatusResponse } from "./services/types";

const POLL_INTERVAL_MS = 15_000;

const numberFormat = new Intl.NumberFormat("de-DE", { maximumFractionDigits: 2 });

export default function App() {
  const [status, setStatus] = useState<StatusResponse | null>(null);
  const [statusError, setStatusError] = useState<string | null>(null);
  const [data, setData] = useState<DataResponse | null>(null);
  const [dataError, setDataError] = useState<string | null>(null);
  const [dataLoading, setDataLoading] = useState<boolean>(false);
  const [result, setResult] = useState<CalcResult | null>(null);
  const [processError, setProcessError] = useState<string | null>(null);
  const [processLoading, setProcessLoading] = useState<boolean>(false);

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

  useEffect(() => {
    void loadStatus();
    void loadData();
    const timer = window.setInterval(() => {
      void loadStatus();
    }, POLL_INTERVAL_MS);
    return () => window.clearInterval(timer);
  }, [loadStatus, loadData]);

  const backendReachable = statusError === null && status !== null;
  const databaseUp = backendReachable && status.database.status === "up";

  const badgeTone: BadgeTone = !backendReachable ? "error" : databaseUp ? "ok" : "warn";
  const badgeLabel = !backendReachable ? "Backend offline" : databaseUp ? "Alle Systeme aktiv" : "DB eingeschränkt";

  return (
    <Layout statusTone={badgeTone} statusLabel={badgeLabel} version={status?.version ?? "—"}>
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
          label="Speicher"
          value={status === null ? "—" : `${numberFormat.format(status.memory_alloc_mb)} MB`}
          hint={status === null ? "—" : `${status.goroutines} Goroutines · ${status.num_cpu} CPUs`}
        />
      </section>

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
