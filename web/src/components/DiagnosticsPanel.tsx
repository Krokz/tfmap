import { useMemo, useState } from "react";
import type { CycleEdge, Diagnostic, DiagSeverity } from "../types";
import type { SelectedItem } from "../App";

interface DiagnosticsBarProps {
  diagnostics: Diagnostic[];
  onToggle: () => void;
  expanded: boolean;
}

const SEVERITY_CONFIG: Record<DiagSeverity, { icon: string; color: string; label: string }> = {
  error: { icon: "E", color: "text-red-400 bg-red-500/15", label: "error" },
  warning: { icon: "W", color: "text-yellow-400 bg-yellow-500/15", label: "warning" },
  info: { icon: "I", color: "text-blue-400 bg-blue-500/15", label: "info" },
};

function entityToSelectedItem(entity: string): SelectedItem | null {
  if (!entity) return null;
  if (entity.startsWith("data.")) {
    const rest = entity.slice(5);
    return { kind: "data", id: rest };
  }
  if (entity.startsWith("module.")) {
    return { kind: "module", id: entity.slice(7) };
  }
  if (entity.startsWith("var.")) {
    return { kind: "variable", id: entity.slice(4) };
  }
  if (entity.startsWith("output.")) {
    return { kind: "output", id: entity.slice(7) };
  }
  if (entity.startsWith("local.")) {
    return { kind: "local", id: entity.slice(6) };
  }
  if (entity.includes(".")) {
    return { kind: "resource", id: entity };
  }
  return null;
}

export function DiagnosticsBar({ diagnostics, onToggle, expanded }: DiagnosticsBarProps) {
  const counts = useMemo(() => {
    const c = { error: 0, warning: 0, info: 0 };
    for (const d of diagnostics) {
      c[d.severity]++;
    }
    return c;
  }, [diagnostics]);

  if (diagnostics.length === 0) return null;

  const hasErrors = counts.error > 0;

  return (
    <button
      onClick={onToggle}
      className={`flex items-center gap-3 px-3 py-1 text-xs transition-colors ${
        hasErrors
          ? "bg-red-500/10 hover:bg-red-500/15 border-l-2 border-red-500"
          : "hover:bg-gray-800/50"
      }`}
    >
      {counts.error > 0 && (
        <span className="flex items-center gap-1 text-red-400">
          <svg className="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
            <circle cx="12" cy="12" r="10" />
            <path d="M12 8v4m0 4h.01" />
          </svg>
          {counts.error}
        </span>
      )}
      {counts.warning > 0 && (
        <span className="flex items-center gap-1 text-yellow-400">
          <svg className="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
            <path d="M12 9v4m0 4h.01M10.29 3.86l-8.6 14.86A1 1 0 002.56 20h18.88a1 1 0 00.87-1.28l-8.6-14.86a1 1 0 00-1.72 0z" />
          </svg>
          {counts.warning}
        </span>
      )}
      {counts.info > 0 && (
        <span className="flex items-center gap-1 text-blue-400">
          <svg className="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
            <circle cx="12" cy="12" r="10" />
            <path d="M12 16v-4m0-4h.01" />
          </svg>
          {counts.info}
        </span>
      )}
      <svg
        className={`w-3 h-3 text-gray-500 transition-transform ${expanded ? "rotate-180" : ""}`}
        fill="none"
        viewBox="0 0 24 24"
        stroke="currentColor"
        strokeWidth={2}
      >
        <path d="M19 9l-7 7-7-7" />
      </svg>
    </button>
  );
}

interface DiagnosticsPanelProps {
  diagnostics: Diagnostic[];
  onSelect: (item: SelectedItem | null) => void;
  onCycleNavigate: (diag: Diagnostic) => void;
  onClose: () => void;
}

export function DiagnosticsPanel({ diagnostics, onSelect, onCycleNavigate, onClose }: DiagnosticsPanelProps) {
  const [filter, setFilter] = useState<DiagSeverity | "all">("all");

  const filtered = useMemo(() => {
    if (filter === "all") return diagnostics;
    return diagnostics.filter((d) => d.severity === filter);
  }, [diagnostics, filter]);

  const counts = useMemo(() => {
    const c = { error: 0, warning: 0, info: 0 };
    for (const d of diagnostics) c[d.severity]++;
    return c;
  }, [diagnostics]);

  return (
    <div className="border-b border-gray-800 bg-gray-900/95 backdrop-blur-sm max-h-[50vh] flex flex-col">
      <div className="flex items-center justify-between px-4 py-2 border-b border-gray-800 shrink-0">
        <div className="flex items-center gap-3">
          <h2 className="text-xs font-semibold text-gray-300 uppercase tracking-wider">
            Diagnostics
          </h2>
          <div className="flex items-center gap-1">
            <FilterButton
              active={filter === "all"}
              onClick={() => setFilter("all")}
              label={`All (${diagnostics.length})`}
            />
            {counts.error > 0 && (
              <FilterButton
                active={filter === "error"}
                onClick={() => setFilter("error")}
                label={`Errors (${counts.error})`}
                severity="error"
              />
            )}
            {counts.warning > 0 && (
              <FilterButton
                active={filter === "warning"}
                onClick={() => setFilter("warning")}
                label={`Warnings (${counts.warning})`}
                severity="warning"
              />
            )}
            {counts.info > 0 && (
              <FilterButton
                active={filter === "info"}
                onClick={() => setFilter("info")}
                label={`Info (${counts.info})`}
                severity="info"
              />
            )}
          </div>
        </div>
        <button
          onClick={onClose}
          className="text-gray-500 hover:text-gray-300 transition-colors p-1 hover:bg-gray-800 rounded"
        >
          <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
            <path strokeLinecap="round" strokeLinejoin="round" d="M6 18L18 6M6 6l12 12" />
          </svg>
        </button>
      </div>

      <div className="overflow-y-auto flex-1">
        {filtered.map((diag, i) => {
          const cfg = SEVERITY_CONFIG[diag.severity];
          const selectable = entityToSelectedItem(diag.entity ?? "");
          const hasCycleEdges = diag.cycleEdges && diag.cycleEdges.length > 0;

          return (
            <div
              key={`${diag.rule}-${i}`}
              className="border-b border-gray-800/50 last:border-0"
            >
              <div
                className={`flex items-start gap-3 px-4 py-2 hover:bg-gray-800/30 transition-colors ${
                  selectable ? "cursor-pointer" : ""
                }`}
                onClick={() => {
                  if (selectable) onSelect(selectable);
                }}
              >
                <span
                  className={`shrink-0 w-5 h-5 rounded flex items-center justify-center text-[10px] font-bold mt-0.5 ${cfg.color}`}
                >
                  {cfg.icon}
                </span>
                <div className="flex-1 min-w-0">
                  <div className="flex items-baseline gap-2">
                    <span className="text-xs text-gray-200">{diag.message}</span>
                    <span className="text-[10px] font-mono text-gray-600">{diag.rule}</span>
                  </div>
                  {!hasCycleEdges && diag.detail && (
                    diag.detail.includes("\n") ? (
                      <pre className="text-[11px] text-gray-500 mt-1 font-mono leading-relaxed whitespace-pre-wrap bg-gray-800/50 rounded px-2 py-1.5 border border-gray-800">
                        {diag.detail}
                      </pre>
                    ) : (
                      <p className="text-[11px] text-gray-500 mt-0.5">{diag.detail}</p>
                    )
                  )}
                  <div className="flex items-center gap-3 mt-1">
                    {diag.entity && (
                      <span className="text-[11px] font-mono text-indigo-400/80">{diag.entity}</span>
                    )}
                    {diag.source && (
                      <span className="text-[11px] font-mono text-gray-600">
                        {diag.source.file}:{diag.source.line}
                      </span>
                    )}
                    {hasCycleEdges && (
                      <button
                        onClick={(e) => {
                          e.stopPropagation();
                          onCycleNavigate(diag);
                        }}
                        className="text-[10px] font-medium text-red-400 hover:text-red-300 bg-red-500/10 hover:bg-red-500/20 px-2 py-0.5 rounded border border-red-500/20 transition-colors whitespace-nowrap ml-auto"
                      >
                        Show in graph &rarr;
                      </button>
                    )}
                  </div>
                </div>
              </div>
              {hasCycleEdges && (
                <CycleEdgesDetail edges={diag.cycleEdges!} onSelect={onSelect} />
              )}
            </div>
          );
        })}
      </div>
    </div>
  );
}

function CycleEdgesDetail({
  edges,
  onSelect,
}: {
  edges: CycleEdge[];
  onSelect: (item: SelectedItem | null) => void;
}) {
  return (
    <div className="px-4 pb-3 ml-8 space-y-2">
      {edges.map((edge, i) => {
        const fromSelectable = entityToSelectedItem(edge.from);
        return (
          <div key={i} className="bg-gray-950/60 rounded-lg border border-gray-800 overflow-hidden">
            <div className="px-3 py-1.5 bg-gray-800/40 border-b border-gray-800 space-y-1">
              <div className="flex items-center gap-2">
                <span className="text-[10px] font-bold text-red-400/70 bg-red-500/10 px-1.5 py-0.5 rounded">
                  {i + 1}
                </span>
                <button
                  className="text-[11px] font-mono text-indigo-400 hover:text-indigo-300 transition-colors"
                  onClick={(e) => {
                    e.stopPropagation();
                    if (fromSelectable) onSelect(fromSelectable);
                  }}
                >
                  {edge.from}
                </button>
                <svg className="w-3 h-3 text-red-400/60 shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                  <path d="M13 7l5 5m0 0l-5 5m5-5H6" />
                </svg>
                <span className="text-[11px] font-mono text-gray-400">{edge.to}</span>
                {edge.source && (
                  <span className="text-[10px] font-mono text-gray-600 ml-auto">
                    {edge.source.file}:{edge.source.line}
                  </span>
                )}
              </div>
              {edge.viaRefs && edge.viaRefs.length > 0 && (
                <div className="flex items-center gap-1.5 ml-7 flex-wrap">
                  <span className="text-[10px] text-gray-500">uses</span>
                  {edge.viaRefs.map((ref, j) => (
                    <span key={j} className="text-[10px] font-mono text-amber-400 bg-amber-500/10 px-1.5 py-0.5 rounded border border-amber-500/20">
                      {ref}
                    </span>
                  ))}
                  <span className="text-[10px] text-gray-500">
                    from <span className="font-mono text-gray-400">{edge.to}</span>
                  </span>
                </div>
              )}
            </div>
            {edge.snippet && edge.snippet.includes(">") && (
              <pre className="text-[11px] font-mono leading-relaxed text-gray-400 px-3 py-2 overflow-x-auto whitespace-pre">
                {edge.snippet.split("\n").map((line, j) => {
                  const isHighlighted = line.includes(">");
                  return (
                    <div
                      key={j}
                      className={isHighlighted ? "text-red-300 bg-red-500/10 -mx-3 px-3" : ""}
                    >
                      {line}
                    </div>
                  );
                })}
              </pre>
            )}
          </div>
        );
      })}
    </div>
  );
}

function FilterButton({
  active,
  onClick,
  label,
  severity,
}: {
  active: boolean;
  onClick: () => void;
  label: string;
  severity?: DiagSeverity;
}) {
  const colors = severity
    ? {
        error: active ? "bg-red-500/20 text-red-300 ring-red-500/30" : "text-red-400/60 hover:text-red-400",
        warning: active ? "bg-yellow-500/20 text-yellow-300 ring-yellow-500/30" : "text-yellow-400/60 hover:text-yellow-400",
        info: active ? "bg-blue-500/20 text-blue-300 ring-blue-500/30" : "text-blue-400/60 hover:text-blue-400",
      }[severity]
    : active
      ? "bg-gray-700 text-gray-200 ring-gray-600"
      : "text-gray-500 hover:text-gray-300";

  return (
    <button
      onClick={onClick}
      className={`text-[10px] px-2 py-0.5 rounded-full transition-colors ${active ? "ring-1" : ""} ${colors}`}
    >
      {label}
    </button>
  );
}
