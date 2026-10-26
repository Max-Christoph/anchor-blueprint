import type { Entry } from "../services/types";

interface DataTableProps {
  items: Entry[];
  loading: boolean;
  error: string | null;
  onReload: () => void;
}

const currency = new Intl.NumberFormat("de-DE", { style: "currency", currency: "EUR" });
const dateTime = new Intl.DateTimeFormat("de-DE", { dateStyle: "medium", timeStyle: "short" });

function formatAmount(cents: number): string {
  return currency.format(cents / 100);
}

function formatDate(value: string): string {
  const parsed = new Date(value);
  return Number.isNaN(parsed.getTime()) ? "—" : dateTime.format(parsed);
}

export function DataTable({ items, loading, error, onReload }: DataTableProps) {
  return (
    <section id="daten" className="rounded-xl border border-slate-200 bg-white shadow-sm">
      <div className="flex items-center justify-between border-b border-slate-200 px-4 py-3">
        <div>
          <h2 className="text-sm font-semibold text-slate-900">Beispieldaten</h2>
          <p className="text-xs text-slate-500">GET /api/v1/data · geschützt durch Auth-Middleware</p>
        </div>
        <button
          type="button"
          onClick={onReload}
          disabled={loading}
          className="rounded-lg border border-slate-300 px-3 py-1.5 text-xs font-medium text-slate-700 hover:bg-slate-50 disabled:opacity-50"
        >
          {loading ? "Lädt…" : "Neu laden"}
        </button>
      </div>

      {error !== null ? (
        <p className="px-4 py-6 text-sm text-rose-700">{error}</p>
      ) : items.length === 0 ? (
        <p className="px-4 py-6 text-sm text-slate-500">
          {loading ? "Daten werden geladen…" : "Keine Datensätze vorhanden."}
        </p>
      ) : (
        <div className="overflow-x-auto">
          <table className="min-w-full divide-y divide-slate-200 text-sm">
            <thead className="bg-slate-50 text-xs uppercase tracking-wide text-slate-500">
              <tr>
                <th className="px-4 py-2 text-left font-medium">ID</th>
                <th className="px-4 py-2 text-left font-medium">Name</th>
                <th className="px-4 py-2 text-left font-medium">Kategorie</th>
                <th className="px-4 py-2 text-right font-medium">Betrag</th>
                <th className="px-4 py-2 text-left font-medium">Angelegt</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-slate-100">
              {items.map((item) => (
                <tr key={item.id} className="hover:bg-slate-50">
                  <td className="px-4 py-2 tabular-nums text-slate-500">{item.id}</td>
                  <td className="px-4 py-2 font-medium text-slate-900">{item.name}</td>
                  <td className="px-4 py-2">
                    <span className="rounded bg-slate-100 px-2 py-0.5 text-xs text-slate-600">{item.category}</span>
                  </td>
                  <td className="px-4 py-2 text-right tabular-nums">{formatAmount(item.amount_cents)}</td>
                  <td className="px-4 py-2 text-slate-500">{formatDate(item.created_at)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </section>
  );
}
