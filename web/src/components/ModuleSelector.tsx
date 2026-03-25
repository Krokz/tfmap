import { useState, useMemo } from "react";
import type { TerraformModule } from "../types";

interface Props {
  modules: TerraformModule[];
  activeModule: string | null;
  onSelect: (modulePath: string | null) => void;
  onBack: () => void;
  canGoBack: boolean;
}

export function ModuleSelector({ modules, activeModule, onSelect, onBack, canGoBack }: Props) {
  const [expanded, setExpanded] = useState(false);

  const sorted = useMemo(() => {
    return [...modules].sort((a, b) => {
      if (a.isRoot !== b.isRoot) return a.isRoot ? -1 : 1;
      return a.path.localeCompare(b.path);
    });
  }, [modules]);

  const totalEntities = (m: TerraformModule) =>
    m.resources + m.dataSources + m.variables + m.outputs + m.modules;

  const activeLabel = activeModule
    ? activeModule === "."
      ? "root"
      : activeModule
    : `All modules (${modules.length})`;

  return (
    <div className="border-b border-gray-800 bg-gray-900/80 relative">
      <div className="flex items-center px-4 py-1.5 gap-3">
        <span className="text-[10px] text-gray-500 uppercase tracking-wider font-semibold shrink-0">
          Module
        </span>
        <button
          onClick={() => setExpanded(!expanded)}
          className="flex items-center gap-2 text-xs text-gray-300 hover:text-white transition-colors bg-gray-800/50 px-2.5 py-1 rounded border border-gray-700 hover:border-gray-600 min-w-0"
        >
          <svg className="w-3 h-3 text-gray-500 shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
            <path d="M3 7v10a2 2 0 002 2h14a2 2 0 002-2V9a2 2 0 00-2-2h-6l-2-2H5a2 2 0 00-2 2z" />
          </svg>
          <span className="truncate font-mono">{activeLabel}</span>
          <svg
            className={`w-3 h-3 text-gray-500 shrink-0 transition-transform ${expanded ? "rotate-180" : ""}`}
            fill="none"
            viewBox="0 0 24 24"
            stroke="currentColor"
            strokeWidth={2}
          >
            <path d="M19 9l-7 7-7-7" />
          </svg>
        </button>
        {canGoBack && (
          <button
            onClick={onBack}
            className="flex items-center gap-1 text-[10px] text-gray-400 hover:text-white bg-gray-800/60 hover:bg-gray-700/80 px-2 py-0.5 rounded border border-gray-700 hover:border-gray-600 transition-colors"
            title="Go back to previous module"
          >
            <svg className="w-3 h-3" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
              <path d="M11 17l-5-5m0 0l5-5m-5 5h12" />
            </svg>
            Back
          </button>
        )}
        {activeModule !== null && (
          <button
            onClick={() => onSelect(null)}
            className="text-[10px] text-gray-500 hover:text-gray-300 transition-colors"
          >
            Clear
          </button>
        )}
      </div>

      {expanded && (
        <div className="absolute top-full left-0 right-0 z-50 bg-gray-900 border-b border-gray-700 shadow-xl max-h-80 overflow-y-auto">
          <button
            onClick={() => {
              onSelect(null);
              setExpanded(false);
            }}
            className={`flex items-center justify-between w-full px-4 py-2 text-xs hover:bg-gray-800/70 transition-colors ${
              activeModule === null ? "bg-indigo-500/10 text-indigo-300" : "text-gray-300"
            }`}
          >
            <span className="font-mono">All modules</span>
            <span className="text-gray-600">{modules.length} modules</span>
          </button>
          {sorted.map((m) => {
            const isActive = activeModule === m.path;
            const label = m.path === "." ? "root (/)" : m.path;
            const count = totalEntities(m);

            return (
              <button
                key={m.path}
                onClick={() => {
                  onSelect(m.path);
                  setExpanded(false);
                }}
                className={`flex items-center justify-between w-full px-4 py-2 text-xs hover:bg-gray-800/70 transition-colors ${
                  isActive ? "bg-indigo-500/10 text-indigo-300" : "text-gray-300"
                }`}
              >
                <div className="flex items-center gap-2 min-w-0">
                  {m.isRoot ? (
                    <span className="text-[9px] font-bold bg-amber-500/20 text-amber-400 px-1 py-0.5 rounded shrink-0">
                      ROOT
                    </span>
                  ) : (
                    <span className="text-[9px] font-bold bg-gray-700 text-gray-400 px-1 py-0.5 rounded shrink-0">
                      MOD
                    </span>
                  )}
                  <span className="font-mono truncate">{label}</span>
                </div>
                <div className="flex items-center gap-3 text-gray-600 shrink-0 ml-4">
                  {m.resources > 0 && <span>{m.resources} res</span>}
                  {m.modules > 0 && <span>{m.modules} mod</span>}
                  {count === 0 && <span>empty</span>}
                </div>
              </button>
            );
          })}
        </div>
      )}
    </div>
  );
}
