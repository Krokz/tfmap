import { useMemo } from "react";
import type { Backend, Resource, OrphanedResource, StateSnapshot, StateOutput } from "../types";

interface StatePanelProps {
  state: StateSnapshot | null;
  backend: Backend | undefined;
  resources: Resource[];
  orphanedResources: OrphanedResource[];
  onClose: () => void;
}

const STATUS_COLORS: Record<string, { bg: string; text: string; label: string }> = {
  in_sync: { bg: "bg-green-500/15", text: "text-green-400", label: "In sync" },
  drifted: { bg: "bg-orange-500/15", text: "text-orange-400", label: "Drifted" },
  not_in_state: { bg: "bg-gray-500/15", text: "text-gray-400", label: "Not in state" },
  orphaned_in_state: { bg: "bg-red-500/15", text: "text-red-400", label: "Orphaned" },
};

function formatOutputValue(val: unknown, sensitive: boolean): string {
  if (sensitive) return "(sensitive)";
  if (val === null || val === undefined) return "null";
  if (typeof val === "string") return val;
  return JSON.stringify(val);
}

function formatOutputType(t: unknown): string {
  if (typeof t === "string") return t;
  if (Array.isArray(t)) return t[0] as string;
  return "unknown";
}

export function StatePanel({ state, backend, resources, orphanedResources, onClose }: StatePanelProps) {
  const counts = useMemo(() => {
    const c: Record<string, number> = {
      in_sync: 0,
      drifted: 0,
      not_in_state: 0,
    };
    for (const r of resources) {
      const s = r.stateStatus || "unknown";
      if (s in c) c[s]++;
    }
    c.orphaned_in_state = orphanedResources.length;
    return c;
  }, [resources, orphanedResources]);

  const outputs: [string, StateOutput][] = useMemo(() => {
    if (!state?.outputs) return [];
    return Object.entries(state.outputs).sort(([a], [b]) => a.localeCompare(b));
  }, [state]);

  return (
    <div className="border-b border-gray-800 bg-gray-900/95 backdrop-blur-sm max-h-[50vh] flex flex-col">
      <div className="flex items-center justify-between px-4 py-2 border-b border-gray-800 shrink-0">
        <h2 className="text-xs font-semibold text-gray-300 uppercase tracking-wider">
          State Summary
        </h2>
        <button
          onClick={onClose}
          className="text-gray-500 hover:text-gray-300 transition-colors p-1 hover:bg-gray-800 rounded"
        >
          <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
            <path strokeLinecap="round" strokeLinejoin="round" d="M6 18L18 6M6 6l12 12" />
          </svg>
        </button>
      </div>

      <div className="overflow-y-auto flex-1 divide-y divide-gray-800/50">
        <div className="px-4 py-3">
          <h3 className="text-[10px] uppercase tracking-wider text-gray-500 mb-2 font-semibold">State info</h3>
          <div className="grid grid-cols-2 sm:grid-cols-4 gap-3">
            <InfoItem label="Backend" value={backend?.type ?? "unknown"} />
            <InfoItem label="Serial" value={state ? String(state.serial) : "–"} />
            <InfoItem label="Lineage" value={state?.lineage ? state.lineage.slice(0, 12) + "…" : "–"} title={state?.lineage} />
            <InfoItem label="TF Version" value={state ? `v${state.version}` : "–"} />
          </div>
        </div>

        <div className="px-4 py-3">
          <h3 className="text-[10px] uppercase tracking-wider text-gray-500 mb-2 font-semibold">Resource status</h3>
          <div className="flex flex-wrap gap-2">
            {Object.entries(STATUS_COLORS).map(([key, cfg]) => (
              <span
                key={key}
                className={`${cfg.bg} ${cfg.text} text-xs font-medium px-2.5 py-1 rounded-full`}
              >
                {cfg.label}: {counts[key] ?? 0}
              </span>
            ))}
          </div>
        </div>

        {orphanedResources.length > 0 && (
          <div className="px-4 py-3">
            <h3 className="text-[10px] uppercase tracking-wider text-gray-500 mb-2 font-semibold">
              Orphaned resources
              <span className="ml-1.5 text-red-400 bg-red-500/10 px-1.5 py-0.5 rounded-full text-[10px]">
                {orphanedResources.length}
              </span>
            </h3>
            <div className="overflow-x-auto">
              <table className="w-full text-xs">
                <thead>
                  <tr className="text-left text-gray-500 border-b border-gray-800">
                    <th className="pb-1 pr-4 font-medium">Resource</th>
                    <th className="pb-1 pr-4 font-medium">Module</th>
                    <th className="pb-1 font-medium">Provider</th>
                  </tr>
                </thead>
                <tbody>
                  {orphanedResources.map((o, i) => (
                    <tr key={i} className="border-b border-gray-800/30 last:border-0">
                      <td className="py-1.5 pr-4 font-mono text-red-300">{o.type}.{o.name}</td>
                      <td className="py-1.5 pr-4 text-gray-400">{o.module || "(root)"}</td>
                      <td className="py-1.5 text-gray-500 font-mono text-[11px] truncate max-w-[200px]" title={o.provider}>
                        {extractProviderShort(o.provider)}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </div>
        )}

        {outputs.length > 0 && (
          <div className="px-4 py-3">
            <h3 className="text-[10px] uppercase tracking-wider text-gray-500 mb-2 font-semibold">
              State outputs
              <span className="ml-1.5 text-blue-400 bg-blue-500/10 px-1.5 py-0.5 rounded-full text-[10px]">
                {outputs.length}
              </span>
            </h3>
            <div className="overflow-x-auto">
              <table className="w-full text-xs">
                <thead>
                  <tr className="text-left text-gray-500 border-b border-gray-800">
                    <th className="pb-1 pr-4 font-medium">Name</th>
                    <th className="pb-1 pr-4 font-medium">Type</th>
                    <th className="pb-1 font-medium">Value</th>
                  </tr>
                </thead>
                <tbody>
                  {outputs.map(([name, out]) => (
                    <tr key={name} className="border-b border-gray-800/30 last:border-0">
                      <td className="py-1.5 pr-4 font-mono text-indigo-300">{name}</td>
                      <td className="py-1.5 pr-4 text-gray-500">{formatOutputType(out.type)}</td>
                      <td className="py-1.5 font-mono text-gray-300 truncate max-w-[300px]" title={formatOutputValue(out.value, out.sensitive)}>
                        {out.sensitive ? (
                          <span className="text-amber-400 bg-amber-500/10 px-1.5 py-0.5 rounded text-[10px]">sensitive</span>
                        ) : (
                          formatOutputValue(out.value, false)
                        )}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </div>
        )}

        {!state && (
          <div className="px-4 py-6 text-center text-gray-500 text-xs">
            No state data available. Make sure the backend is accessible.
          </div>
        )}
      </div>
    </div>
  );
}

function InfoItem({ label, value, title }: { label: string; value: string; title?: string }) {
  return (
    <div>
      <div className="text-[10px] text-gray-500 mb-0.5">{label}</div>
      <div className="text-xs text-gray-200 font-mono truncate" title={title ?? value}>{value}</div>
    </div>
  );
}

function extractProviderShort(provider: string): string {
  const match = provider.match(/\/([^/"]+)"\]?$/);
  return match ? match[1] : provider;
}
