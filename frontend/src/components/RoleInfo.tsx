interface RoleInfoProps {
  roles: string[];
  authMode: string;
  authenticated: boolean;
}

/** Read-only-Darstellung der Rollen, wenn der Dev-Modus aus ist. */
export function RoleInfo({ roles, authMode, authenticated }: RoleInfoProps) {
  return (
    <section className="mb-6 rounded-xl border border-slate-200 bg-white shadow-sm">
      <div className="border-b border-slate-200 px-4 py-3">
        <h2 className="text-sm font-semibold text-slate-900">Rollen (read-only)</h2>
        <p className="text-xs text-slate-500">
          Rollen werden über OIDC-Token gesteuert (auth_mode: <code className="font-mono">{authMode || "—"}</code>).
        </p>
      </div>

      <div className="flex flex-wrap items-center gap-2 px-4 py-4">
        {!authenticated ? (
          <span className="text-sm text-slate-600">
            Nicht angemeldet — Rollen werden über OIDC-Token gesteuert, ein Rollenwechsel im UI ist nicht möglich.
          </span>
        ) : roles.length === 0 ? (
          <span className="text-sm text-slate-600">Im Token sind keine Rollen enthalten.</span>
        ) : (
          roles.map((role) => (
            <span key={role} className="rounded-lg border border-slate-200 bg-slate-50 px-3 py-1.5 text-xs font-medium text-slate-700">
              {role}
            </span>
          ))
        )}
      </div>
    </section>
  );
}
