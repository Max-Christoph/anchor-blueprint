import { useState } from "react";

import type { CalcResult } from "../services/types";

interface ProcessPanelProps {
  loading: boolean;
  error: string | null;
  result: CalcResult | null;
  onRun: (values: number[]) => void;
}

const DEFAULT_INPUT = "120, 340, 95, 480, 210, 365, 155, 290";

const numberFormat = new Intl.NumberFormat("de-DE", { maximumFractionDigits: 3 });

function parseValues(raw: string): number[] {
  return raw
    .split(/[\s,;]+/)
    .map((token) => token.replace(",", "."))
    .filter((token) => token.length > 0)
    .map((token) => Number(token))
    .filter((value) => Number.isFinite(value));
}

export function ProcessPanel({ loading, error, result, onRun }: ProcessPanelProps) {
  const [input, setInput] = useState<string>(DEFAULT_INPUT);
  const parsed = parseValues(input);

  function handleSubmit(event: React.FormEvent<HTMLFormElement>): void {
    event.preventDefault();
    if (parsed.length === 0) {
      return;
    }
    onRun(parsed);
  }

  return (
    <section id="verarbeitung" className="rounded-xl border border-slate-200 bg-white shadow-sm">
      <div className="border-b border-slate-200 px-4 py-3">
        <h2 className="text-sm font-semibold text-slate-900">Worker-Job ausführen</h2>
        <p className="text-xs text-slate-500">POST /api/v1/process → Python-Worker /process/calc</p>
      </div>

      <form onSubmit={handleSubmit} className="space-y-3 px-4 py-4">
        <label htmlFor="values" className="block text-xs font-medium text-slate-600">
          Werte (Komma- oder Leerzeichen-getrennt)
        </label>
        <textarea
          id="values"
          value={input}
          onChange={(event) => setInput(event.target.value)}
          rows={2}
          className="w-full rounded-lg border border-slate-300 px-3 py-2 font-mono text-sm shadow-sm focus:border-slate-500 focus:outline-none"
        />
        <div className="flex items-center gap-3">
          <button
            type="submit"
            disabled={loading || parsed.length === 0}
            className="rounded-lg bg-slate-900 px-3 py-2 text-sm font-medium text-white hover:bg-slate-800 disabled:opacity-50"
          >
            {loading ? "Verarbeite…" : "Berechnung starten"}
          </button>
          <span className="text-xs text-slate-500">{parsed.length} gültige Werte erkannt</span>
        </div>
      </form>

      {error !== null && <p className="px-4 pb-4 text-sm text-rose-700">{error}</p>}

      {result !== null && (
        <dl className="grid grid-cols-2 gap-3 border-t border-slate-200 px-4 py-4 sm:grid-cols-4">
          <ResultItem label="Anzahl" value={numberFormat.format(result.count)} />
          <ResultItem label="Summe" value={numberFormat.format(result.sum)} />
          <ResultItem label="Mittelwert" value={numberFormat.format(result.mean)} />
          <ResultItem label="Median" value={numberFormat.format(result.median)} />
          <ResultItem label="Min" value={numberFormat.format(result.min)} />
          <ResultItem label="Max" value={numberFormat.format(result.max)} />
          <ResultItem label="Std-Abw." value={numberFormat.format(result.stddev)} />
          <ResultItem label="P90" value={numberFormat.format(result.p90)} />
        </dl>
      )}
    </section>
  );
}

function ResultItem({ label, value }: { label: string; value: string }) {
  return (
    <div>
      <dt className="text-xs uppercase tracking-wide text-slate-500">{label}</dt>
      <dd className="mt-1 text-lg font-semibold tabular-nums text-slate-900">{value}</dd>
    </div>
  );
}
