import type { ReactNode } from "react";

import { StatusBadge, type BadgeTone } from "./StatusBadge";

interface LayoutProps {
  statusTone: BadgeTone;
  statusLabel: string;
  version: string;
  roles: string[];
  children: ReactNode;
}

const NAV_ITEMS: ReadonlyArray<{ label: string; href: string }> = [
  { label: "Dashboard", href: "#dashboard" },
  { label: "Daten", href: "#daten" },
  { label: "Verarbeitung", href: "#verarbeitung" },
];

export function Layout({ statusTone, statusLabel, version, roles, children }: LayoutProps) {
  return (
    <div className="min-h-screen bg-slate-50 text-slate-900">
      <header className="border-b border-slate-200 bg-white">
        <div className="mx-auto flex max-w-6xl flex-col gap-4 px-6 py-4 sm:flex-row sm:items-center sm:justify-between">
          <div className="flex items-center gap-3">
            <div className="flex h-10 w-10 items-center justify-center rounded-lg bg-slate-900 text-sm font-semibold text-white">
              AB
            </div>
            <div>
              <p className="text-sm font-semibold tracking-tight">Anchor Blueprint</p>
              <p className="text-xs text-slate-500">Interne Plattform · v{version}</p>
            </div>
          </div>

          <div className="flex flex-wrap items-center gap-3">
            <nav className="hidden gap-4 text-sm text-slate-600 sm:flex">
              {NAV_ITEMS.map((item) => (
                <a key={item.href} href={item.href} className="hover:text-slate-900">
                  {item.label}
                </a>
              ))}
            </nav>

            <div className="flex items-center gap-1.5" aria-label="Aktive Rollen">
              {roles.length === 0 ? (
                <span className="rounded bg-slate-100 px-2 py-0.5 text-xs text-slate-600">keine Rolle</span>
              ) : (
                roles.map((role) => (
                  <span key={role} className="rounded bg-slate-100 px-2 py-0.5 text-xs font-medium text-slate-700">
                    {role}
                  </span>
                ))
              )}
            </div>

            <StatusBadge tone={statusTone} label={statusLabel} />
          </div>
        </div>
      </header>

      <main className="mx-auto max-w-6xl px-6 py-8">
        <div className="mb-6">
          <h1 className="text-2xl font-semibold tracking-tight">Systemübersicht</h1>
          <p className="mt-1 text-sm text-slate-600">
            Status des Go-Backends, RBAC-Demo, Beispieldaten aus PostgreSQL und Ausführung eines Python-Worker-Jobs.
          </p>
        </div>
        {children}
      </main>

      <footer className="mx-auto max-w-6xl px-6 pb-8 text-xs text-slate-500">
        Anchor Blueprint · Backend Go (JWT/RBAC) · Worker Python/FastAPI · DB PostgreSQL 16 (golang-migrate)
      </footer>
    </div>
  );
}
