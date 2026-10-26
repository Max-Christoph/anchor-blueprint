const ROLE_PRESETS: ReadonlyArray<{ label: string; roles: string[] }> = [
  { label: "User", roles: ["User"] },
  { label: "Operator", roles: ["Operator"] },
  { label: "Admin", roles: ["Admin"] },
  { label: "Operator + Admin", roles: ["Operator", "Admin"] },
];

interface RoleSwitcherProps {
  roles: string[];
  busy: boolean;
  onChange: (roles: string[]) => void;
  onOperatorAction: () => void;
  onAdminReset: () => void;
}

function isActive(current: string[], candidate: string[]): boolean {
  return current.length === candidate.length && current.every((role, index) => role === candidate[index]);
}

export function RoleSwitcher({ roles, busy, onChange, onOperatorAction, onAdminReset }: RoleSwitcherProps) {
  return (
    <section className="mb-6 rounded-xl border border-slate-200 bg-white shadow-sm">
      <div className="border-b border-slate-200 px-4 py-3">
        <h2 className="text-sm font-semibold text-slate-900">RBAC-Demo</h2>
        <p className="text-xs text-slate-500">
          Rollen werden im Dev-Modus als <code className="font-mono">X-User-Roles</code> gesendet; im JWT-Modus stammen sie aus den Claims.
        </p>
      </div>

      <div className="flex flex-col gap-3 px-4 py-4 sm:flex-row sm:items-center sm:justify-between">
        <div className="flex flex-wrap items-center gap-2">
          <span className="text-xs font-medium uppercase tracking-wide text-slate-500">Aktive Rolle</span>
          {ROLE_PRESETS.map((preset) => (
            <button
              key={preset.label}
              type="button"
              onClick={() => onChange(preset.roles)}
              className={`rounded-lg border px-3 py-1.5 text-xs font-medium ${
                isActive(roles, preset.roles)
                  ? "border-slate-900 bg-slate-900 text-white"
                  : "border-slate-300 text-slate-700 hover:bg-slate-50"
              }`}
            >
              {preset.label}
            </button>
          ))}
        </div>

        <div className="flex flex-wrap gap-2">
          <button
            type="button"
            onClick={onOperatorAction}
            disabled={busy}
            className="rounded-lg border border-slate-300 px-3 py-1.5 text-xs font-medium text-slate-700 hover:bg-slate-50 disabled:opacity-50"
          >
            GET /protected/operator
          </button>
          <button
            type="button"
            onClick={onAdminReset}
            disabled={busy}
            className="rounded-lg bg-rose-600 px-3 py-1.5 text-xs font-medium text-white hover:bg-rose-500 disabled:opacity-50"
          >
            POST /protected/admin/reset
          </button>
        </div>
      </div>
    </section>
  );
}
