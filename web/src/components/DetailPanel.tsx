import { useState, useMemo } from "react";
import type { Project, ModuleCall, StateStatus, CycleEdge } from "../types";
import type { SelectedItem, CycleHighlightSet } from "../App";
import { resolveModuleCallToPath } from "../utils/module";

interface Props {
  project: Project;
  fullProject: Project;
  selected: SelectedItem;
  onClose: () => void;
  onNavigateToModule: (modulePath: string) => void;
  cycleHighlights: CycleHighlightSet;
}

const STATUS_BADGES: Record<
  StateStatus,
  { text: string; className: string }
> = {
  in_sync: {
    text: "In Sync",
    className: "bg-green-500/20 text-green-400 ring-green-500/30",
  },
  drifted: {
    text: "Drifted",
    className: "bg-yellow-500/20 text-yellow-400 ring-yellow-500/30",
  },
  not_in_state: {
    text: "Not in State",
    className: "bg-red-500/20 text-red-400 ring-red-500/30",
  },
  orphaned_in_state: {
    text: "Orphaned",
    className: "bg-orange-500/20 text-orange-400 ring-orange-500/30",
  },
  unknown: {
    text: "Unknown",
    className: "bg-gray-500/20 text-gray-400 ring-gray-500/30",
  },
};

const KIND_ICONS: Record<SelectedItem["kind"], string> = {
  resource: "R",
  data: "D",
  module: "M",
  variable: "V",
  output: "O",
  local: "L",
  provider: "P",
};

const KIND_COLORS: Record<SelectedItem["kind"], string> = {
  resource: "bg-indigo-500/20 text-indigo-300",
  data: "bg-teal-500/20 text-teal-300",
  module: "bg-purple-500/20 text-purple-300",
  variable: "bg-amber-500/20 text-amber-300",
  output: "bg-emerald-500/20 text-emerald-300",
  local: "bg-pink-500/20 text-pink-300",
  provider: "bg-blue-500/20 text-blue-300",
};

interface ItemInfo {
  label: string;
  source: { file: string; line: number };
  attributes: Record<string, unknown>;
  references?: string[];
  stateStatus?: StateStatus;
  stateAttrs?: Record<string, unknown>;
}

function findItem(project: Project, selected: SelectedItem): ItemInfo | null {
  switch (selected.kind) {
    case "resource": {
      const r = project.resources.find(
        (res) => `${res.type}.${res.name}` === selected.id
      );
      if (!r) return null;
      return {
        label: `${r.type}.${r.name}`,
        source: { file: r.source.file, line: r.source.line },
        attributes: r.attributes,
        references: r.references,
        stateStatus: r.stateStatus,
        stateAttrs: r.stateAttrs,
      };
    }
    case "data": {
      const d = project.dataSources.find(
        (ds) => `${ds.type}.${ds.name}` === selected.id
      );
      if (!d) return null;
      return {
        label: `data.${d.type}.${d.name}`,
        source: { file: d.source.file, line: d.source.line },
        attributes: d.attributes,
        references: d.references,
      };
    }
    case "module": {
      const m = project.modules.find((mod) => mod.name === selected.id);
      if (!m) return null;
      return {
        label: `module.${m.name}`,
        source: { file: m.source.file, line: m.source.line },
        attributes: m.inputs,
        references: m.references,
      };
    }
    case "variable": {
      const v = project.variables.find((va) => va.name === selected.id);
      if (!v) return null;
      const attrs: Record<string, unknown> = {};
      if (v.type) attrs.type = v.type;
      if (v.description) attrs.description = v.description;
      if (v.default !== undefined) attrs.default = v.default;
      if (v.value !== undefined) attrs.value = v.value;
      if (v.sensitive) attrs.sensitive = v.sensitive;
      if (v.validation) attrs.validation = v.validation;
      if (v.valueSource) attrs.valueSource = v.valueSource;
      return {
        label: `var.${v.name}`,
        source: { file: v.source.file, line: v.source.line },
        attributes: attrs,
      };
    }
    case "output": {
      const o = project.outputs.find((out) => out.name === selected.id);
      if (!o) return null;
      const attrs: Record<string, unknown> = {};
      if (o.description) attrs.description = o.description;
      if (o.value !== undefined) attrs.value = o.value;
      if (o.sensitive) attrs.sensitive = o.sensitive;
      return {
        label: `output.${o.name}`,
        source: { file: o.source.file, line: o.source.line },
        attributes: attrs,
        references: o.references,
      };
    }
    case "local": {
      const l = project.locals.find((loc) => loc.name === selected.id);
      if (!l) return null;
      return {
        label: `local.${l.name}`,
        source: { file: l.source.file, line: l.source.line },
        attributes: { expression: l.expression },
        references: l.references,
      };
    }
    case "provider": {
      const p = project.providers.find((prov) => {
        const pid = prov.alias
          ? `${prov.name}.${prov.alias}`
          : prov.name;
        return pid === selected.id;
      });
      if (!p) return null;
      const attrs: Record<string, unknown> = { ...p.config };
      if (p.version) attrs.version = p.version;
      return {
        label: p.alias ? `${p.name}.${p.alias}` : p.name,
        source: { file: p.source.file, line: p.source.line },
        attributes: attrs,
      };
    }
  }
}

function classifyModuleSource(source: string): {
  kind: "local" | "registry" | "git" | "s3" | "gcs" | "http" | "unknown";
  label: string;
  color: string;
} {
  if (!source) return { kind: "unknown", label: "Unknown", color: "text-gray-400 bg-gray-500/15" };
  if (source.startsWith("./") || source.startsWith("../"))
    return { kind: "local", label: "Local", color: "text-green-400 bg-green-500/15" };
  if (source.startsWith("git::") || source.startsWith("git@") || source.includes(".git"))
    return { kind: "git", label: "Git", color: "text-orange-400 bg-orange-500/15" };
  if (source.startsWith("s3::"))
    return { kind: "s3", label: "S3", color: "text-amber-400 bg-amber-500/15" };
  if (source.startsWith("gcs::"))
    return { kind: "gcs", label: "GCS", color: "text-blue-400 bg-blue-500/15" };
  if (source.startsWith("http://") || source.startsWith("https://"))
    return { kind: "http", label: "HTTP", color: "text-cyan-400 bg-cyan-500/15" };
  if (source.includes("/") && !source.startsWith("/"))
    return { kind: "registry", label: "Registry", color: "text-purple-400 bg-purple-500/15" };
  return { kind: "unknown", label: "Unknown", color: "text-gray-400 bg-gray-500/15" };
}

function resolveModulePath(moduleCall: ModuleCall, fullProject: Project) {
  const path = resolveModuleCallToPath(moduleCall, fullProject.discoveredModules ?? []);
  if (!path) return null;
  return fullProject.discoveredModules?.find((m) => m.path === path) ?? null;
}

interface ModuleContents {
  resources: string[];
  dataSources: string[];
  modules: string[];
  variables: string[];
  outputs: string[];
}

function getModuleContents(
  modulePath: string,
  fullProject: Project
): ModuleContents {
  const match = (file: string) => {
    const dir = file.includes("/") ? file.substring(0, file.lastIndexOf("/")) : ".";
    return dir === modulePath;
  };

  return {
    resources: fullProject.resources
      .filter((r) => match(r.source.file))
      .map((r) => `${r.type}.${r.name}`),
    dataSources: fullProject.dataSources
      .filter((d) => match(d.source.file))
      .map((d) => `data.${d.type}.${d.name}`),
    modules: fullProject.modules
      .filter((m) => match(m.source.file))
      .map((m) => `module.${m.name}`),
    variables: fullProject.variables
      .filter((v) => match(v.source.file))
      .map((v) => `var.${v.name}`),
    outputs: fullProject.outputs
      .filter((o) => match(o.source.file))
      .map((o) => `output.${o.name}`),
  };
}

function JsonValue({ value, depth = 0 }: { value: unknown; depth?: number }) {
  const [expanded, setExpanded] = useState(depth < 1);

  if (value === null || value === undefined) {
    return <span className="text-gray-500 italic">null</span>;
  }
  if (typeof value === "boolean") {
    return (
      <span className="text-amber-400">{value ? "true" : "false"}</span>
    );
  }
  if (typeof value === "number") {
    return <span className="text-cyan-400">{value}</span>;
  }
  if (typeof value === "string") {
    if (value.length > 120) {
      return (
        <span className="text-green-400 break-all">
          &quot;{value.slice(0, 120)}...&quot;
        </span>
      );
    }
    return <span className="text-green-400">&quot;{value}&quot;</span>;
  }

  if (Array.isArray(value)) {
    if (value.length === 0)
      return <span className="text-gray-500">[]</span>;
    return (
      <span>
        <button
          onClick={() => setExpanded(!expanded)}
          className="text-gray-500 hover:text-gray-300 text-[10px]"
        >
          {expanded ? "▼" : "▶"} [{value.length}]
        </button>
        {expanded && (
          <div className="ml-4 border-l border-gray-700 pl-2">
            {value.map((item, i) => (
              <div key={i} className="py-0.5">
                <JsonValue value={item} depth={depth + 1} />
              </div>
            ))}
          </div>
        )}
      </span>
    );
  }

  if (typeof value === "object") {
    const entries = Object.entries(value as Record<string, unknown>);
    if (entries.length === 0)
      return <span className="text-gray-500">{"{}"}</span>;
    return (
      <span>
        <button
          onClick={() => setExpanded(!expanded)}
          className="text-gray-500 hover:text-gray-300 text-[10px]"
        >
          {expanded ? "▼" : "▶"} {`{${entries.length}}`}
        </button>
        {expanded && (
          <div className="ml-4 border-l border-gray-700 pl-2">
            {entries.map(([k, v]) => (
              <div key={k} className="py-0.5 flex gap-2">
                <span className="text-purple-400 shrink-0">{k}:</span>
                <JsonValue value={v} depth={depth + 1} />
              </div>
            ))}
          </div>
        )}
      </span>
    );
  }

  return <span className="text-gray-400">{String(value)}</span>;
}

function StateDiff({
  declared,
  state,
}: {
  declared: Record<string, unknown>;
  state: Record<string, unknown>;
}) {
  const allKeys = useMemo(() => {
    const keys = new Set([
      ...Object.keys(declared),
      ...Object.keys(state),
    ]);
    return Array.from(keys).sort();
  }, [declared, state]);

  return (
    <div>
      <h4 className="text-xs font-semibold text-gray-400 uppercase tracking-wider mb-2">
        State Comparison
      </h4>
      <div className="bg-gray-950/50 rounded-lg border border-gray-800 overflow-x-auto">
        <table className="w-full text-xs">
          <thead>
            <tr className="border-b border-gray-700">
              <th className="text-left py-2 px-3 text-gray-500 font-medium">
                Key
              </th>
              <th className="text-left py-2 px-3 text-gray-500 font-medium">
                Declared
              </th>
              <th className="text-left py-2 px-3 text-gray-500 font-medium">
                State
              </th>
            </tr>
          </thead>
          <tbody>
            {allKeys.map((key) => {
              const dVal = declared[key];
              const sVal = state[key];
              const isDiff =
                JSON.stringify(dVal) !== JSON.stringify(sVal);
              return (
                <tr
                  key={key}
                  className={`border-b border-gray-800/50 last:border-0 ${isDiff ? "bg-yellow-500/5" : ""}`}
                >
                  <td className="py-1.5 px-3 font-mono text-purple-400 whitespace-nowrap">
                    {key}
                  </td>
                  <td
                    className={`py-1.5 px-3 font-mono ${isDiff ? "text-red-400" : "text-gray-300"}`}
                  >
                    {dVal !== undefined ? (
                      typeof dVal === "object" ? (
                        <JsonValue value={dVal} depth={1} />
                      ) : (
                        String(dVal)
                      )
                    ) : (
                      <span className="text-gray-600 italic">—</span>
                    )}
                  </td>
                  <td
                    className={`py-1.5 px-3 font-mono ${isDiff ? "text-green-400" : "text-gray-300"}`}
                  >
                    {sVal !== undefined ? (
                      typeof sVal === "object" ? (
                        <JsonValue value={sVal} depth={1} />
                      ) : (
                        String(sVal)
                      )
                    ) : (
                      <span className="text-gray-600 italic">—</span>
                    )}
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>
    </div>
  );
}

function ContentsList({ label, items, color }: { label: string; items: string[]; color: string }) {
  const [expanded, setExpanded] = useState(false);
  if (items.length === 0) return null;

  const visible = expanded ? items : items.slice(0, 5);
  const hasMore = items.length > 5;

  return (
    <div>
      <div className="flex items-center gap-2 mb-1">
        <span className="text-[10px] font-semibold text-gray-500 uppercase tracking-wider">{label}</span>
        <span className={`text-[10px] font-mono px-1.5 py-0.5 rounded ${color}`}>{items.length}</span>
      </div>
      <div className="flex flex-wrap gap-1">
        {visible.map((item) => (
          <span
            key={item}
            className="text-[11px] font-mono bg-gray-800 text-gray-300 px-2 py-0.5 rounded border border-gray-700"
          >
            {item}
          </span>
        ))}
        {hasMore && !expanded && (
          <button
            onClick={() => setExpanded(true)}
            className="text-[10px] text-indigo-400 hover:text-indigo-300 px-2 py-0.5"
          >
            +{items.length - 5} more
          </button>
        )}
        {expanded && hasMore && (
          <button
            onClick={() => setExpanded(false)}
            className="text-[10px] text-indigo-400 hover:text-indigo-300 px-2 py-0.5"
          >
            show less
          </button>
        )}
      </div>
    </div>
  );
}

function ModuleDetailSection({
  moduleCall,
  fullProject,
  onNavigateToModule,
}: {
  moduleCall: ModuleCall;
  fullProject: Project;
  onNavigateToModule: (modulePath: string) => void;
}) {
  const sourceInfo = classifyModuleSource(moduleCall.moduleSource);
  const resolvedModule = useMemo(
    () => resolveModulePath(moduleCall, fullProject),
    [moduleCall, fullProject]
  );
  const contents = useMemo(
    () => resolvedModule ? getModuleContents(resolvedModule.path, fullProject) : null,
    [resolvedModule, fullProject]
  );

  return (
    <div className="space-y-3">
      <div>
        <h4 className="text-xs font-semibold text-gray-400 uppercase tracking-wider mb-1.5">
          Module Info
        </h4>
        <div className="bg-gray-950/50 rounded-lg border border-gray-800 p-2.5 space-y-2">
          <div className="flex items-center gap-2">
            <span className="text-[10px] text-gray-500 w-14 shrink-0">Source</span>
            <span className={`text-[10px] font-bold px-1.5 py-0.5 rounded ${sourceInfo.color}`}>
              {sourceInfo.label}
            </span>
            <span className="text-[11px] font-mono text-gray-300 break-all">
              {moduleCall.moduleSource}
            </span>
          </div>
          {moduleCall.version && (
            <div className="flex items-center gap-2">
              <span className="text-[10px] text-gray-500 w-14 shrink-0">Version</span>
              <span className="text-[11px] font-mono text-cyan-400">{moduleCall.version}</span>
            </div>
          )}
          {resolvedModule && (
            <div className="flex items-center gap-2">
              <span className="text-[10px] text-gray-500 w-14 shrink-0">Path</span>
              <span className="text-[11px] font-mono text-blue-400">{resolvedModule.path}/</span>
              <button
                onClick={() => onNavigateToModule(resolvedModule.path)}
                className="text-[10px] font-medium text-cyan-400 hover:text-cyan-300 bg-cyan-500/10 hover:bg-cyan-500/20 px-2 py-0.5 rounded border border-cyan-500/20 transition-colors whitespace-nowrap"
                title={`Jump to ${resolvedModule.path}`}
              >
                Jump to module &rarr;
              </button>
            </div>
          )}
          {moduleCall.providers && Object.keys(moduleCall.providers).length > 0 && (
            <div className="flex items-start gap-2">
              <span className="text-[10px] text-gray-500 w-14 shrink-0 pt-0.5">Providers</span>
              <div className="flex flex-wrap gap-1">
                {Object.entries(moduleCall.providers).map(([alias, provider]) => (
                  <span
                    key={alias}
                    className="text-[11px] font-mono bg-blue-500/10 text-blue-300 px-2 py-0.5 rounded border border-blue-500/20"
                  >
                    {alias} = {provider}
                  </span>
                ))}
              </div>
            </div>
          )}
        </div>
      </div>

      {contents && (
        <div>
          <h4 className="text-xs font-semibold text-gray-400 uppercase tracking-wider mb-1.5">
            Module Contents
          </h4>
          <div className="bg-gray-950/50 rounded-lg border border-gray-800 p-2.5 space-y-2">
            <ContentsList label="Resources" items={contents.resources} color="bg-indigo-500/20 text-indigo-300" />
            <ContentsList label="Data Sources" items={contents.dataSources} color="bg-teal-500/20 text-teal-300" />
            <ContentsList label="Sub-Modules" items={contents.modules} color="bg-purple-500/20 text-purple-300" />
            <ContentsList label="Variables" items={contents.variables} color="bg-amber-500/20 text-amber-300" />
            <ContentsList label="Outputs" items={contents.outputs} color="bg-emerald-500/20 text-emerald-300" />
            {contents.resources.length === 0 &&
             contents.dataSources.length === 0 &&
             contents.modules.length === 0 &&
             contents.variables.length === 0 &&
             contents.outputs.length === 0 && (
              <span className="text-[11px] text-gray-600 italic">No entities found in module directory</span>
            )}
          </div>
        </div>
      )}

      {!resolvedModule && sourceInfo.kind !== "local" && (
        <div className="bg-gray-950/50 rounded-lg border border-gray-800 p-2.5">
          <span className="text-[11px] text-gray-500 italic">
            External module — contents not available locally
          </span>
        </div>
      )}
    </div>
  );
}

function CycleInfoSection({
  entityLabel,
  allEdges,
}: {
  entityLabel: string;
  allEdges: CycleEdge[];
}) {
  const cyclePath = allEdges.map((e) => e.from);

  return (
    <div>
      <h4 className="text-xs font-semibold text-red-400 uppercase tracking-wider mb-1.5 flex items-center gap-2">
        <svg className="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
          <path d="M12 9v4m0 4h.01M10.29 3.86l-8.6 14.86A1 1 0 002.56 20h18.88a1 1 0 00.87-1.28l-8.6-14.86a1 1 0 00-1.72 0z" />
        </svg>
        Dependency Cycle
      </h4>
      <div className="bg-red-500/5 rounded-lg border border-red-500/20 overflow-hidden">
        <div className="px-3 py-2 border-b border-red-500/10">
          <div className="flex items-center gap-1.5 flex-wrap text-[11px] font-mono">
            {cyclePath.map((name, i) => (
              <span key={i} className="flex items-center gap-1.5">
                <span className={name === entityLabel ? "text-red-300 font-bold" : "text-gray-400"}>
                  {name}
                </span>
                <svg className="w-3 h-3 text-red-400/50 shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                  <path d="M13 7l5 5m0 0l-5 5m5-5H6" />
                </svg>
              </span>
            ))}
            <span className="text-red-300 font-bold">{cyclePath[0]}</span>
          </div>
        </div>

        {allEdges.map((edge, i) => {
          const isFromThis = edge.from === entityLabel;
          const isToThis = edge.to === entityLabel;
          return (
            <div
              key={i}
              className={`border-b border-red-500/10 last:border-0 ${
                isFromThis ? "bg-red-500/10" : ""
              }`}
            >
              <div className="px-3 py-1.5 flex items-center gap-2">
                <span className="text-[10px] font-bold text-red-400/60 bg-red-500/10 w-5 h-5 rounded flex items-center justify-center shrink-0">
                  {i + 1}
                </span>
                <span className={`text-[11px] font-mono ${isFromThis ? "text-red-300 font-bold" : "text-gray-400"}`}>
                  {edge.from}
                </span>
                <svg className="w-3 h-3 text-red-400/50 shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                  <path d="M13 7l5 5m0 0l-5 5m5-5H6" />
                </svg>
                <span className={`text-[11px] font-mono ${isToThis ? "text-red-300 font-bold" : "text-gray-400"}`}>
                  {edge.to}
                </span>
                {edge.viaRefs && edge.viaRefs.length > 0 && (
                  <div className="flex items-center gap-1 ml-auto flex-wrap justify-end">
                    <span className="text-[9px] text-gray-600">uses</span>
                    {edge.viaRefs.map((ref, j) => (
                      <span key={j} className="text-[10px] font-mono text-amber-400 bg-amber-500/10 px-1.5 py-0.5 rounded border border-amber-500/20">
                        {ref}
                      </span>
                    ))}
                  </div>
                )}
              </div>
              {isFromThis && edge.snippet && edge.snippet.includes(">") && (
                <pre className="text-[10px] font-mono leading-relaxed text-gray-400 px-3 py-1.5 bg-gray-950/40 overflow-x-auto whitespace-pre border-t border-red-500/10">
                  {edge.snippet.split("\n").map((line, j) => {
                    const isHighlighted = line.trimStart().startsWith(">");
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
    </div>
  );
}

export function DetailPanel({ project, fullProject, selected, onClose, onNavigateToModule, cycleHighlights }: Props) {
  const item = useMemo(
    () => findItem(project, selected),
    [project, selected]
  );

  const moduleCall = useMemo(() => {
    if (selected.kind !== "module") return null;
    return project.modules.find((m) => m.name === selected.id) ?? null;
  }, [project, selected]);

  const cycleEdgesForItem = useMemo(() => {
    if (!item || cycleHighlights.size === 0 || !cycleHighlights.has(item.label)) return null;
    for (const diag of project.diagnostics) {
      if (diag.rule !== "dependency-cycle" || !diag.cycleEdges?.length) continue;
      const relevant = diag.cycleEdges.filter(
        (e) => e.from === item.label || e.to === item.label
      );
      if (relevant.length > 0) return { allEdges: diag.cycleEdges, relevant };
    }
    return null;
  }, [item, cycleHighlights, project.diagnostics]);

  if (!item) {
    return (
      <div className="h-64 border-t border-gray-800 bg-gray-900 overflow-y-auto shrink-0 p-4">
        <div className="flex justify-between items-center">
          <span className="text-sm text-gray-500">
            Item not found: {selected.id}
          </span>
          <button
            onClick={onClose}
            className="text-gray-500 hover:text-gray-300 transition-colors p-1 hover:bg-gray-800 rounded"
          >
            <svg
              className="w-4 h-4"
              fill="none"
              viewBox="0 0 24 24"
              stroke="currentColor"
              strokeWidth={2}
            >
              <path
                strokeLinecap="round"
                strokeLinejoin="round"
                d="M6 18L18 6M6 6l12 12"
              />
            </svg>
          </button>
        </div>
      </div>
    );
  }

  const attrEntries = Object.entries(item.attributes).filter(
    ([, v]) => v !== undefined
  );

  return (
    <div className="h-72 border-t border-gray-800 bg-gray-900 overflow-y-auto shrink-0">
      <div className="sticky top-0 bg-gray-900 border-b border-gray-800 px-4 py-2.5 flex items-center justify-between z-10">
        <div className="flex items-center gap-3">
          <span
            className={`w-6 h-6 rounded flex items-center justify-center text-[10px] font-bold ${KIND_COLORS[selected.kind]}`}
          >
            {KIND_ICONS[selected.kind]}
          </span>
          <span className="text-sm font-mono text-gray-200 font-medium">
            {item.label}
          </span>
          {item.stateStatus && (
            <span
              className={`text-[10px] font-medium px-2 py-0.5 rounded-full ring-1 ${STATUS_BADGES[item.stateStatus].className}`}
            >
              {STATUS_BADGES[item.stateStatus].text}
            </span>
          )}
        </div>
        <button
          onClick={onClose}
          className="text-gray-500 hover:text-gray-300 transition-colors p-1 hover:bg-gray-800 rounded"
        >
          <svg
            className="w-4 h-4"
            fill="none"
            viewBox="0 0 24 24"
            stroke="currentColor"
            strokeWidth={2}
          >
            <path
              strokeLinecap="round"
              strokeLinejoin="round"
              d="M6 18L18 6M6 6l12 12"
            />
          </svg>
        </button>
      </div>

      <div className="px-4 py-3 space-y-4">
        <div>
          <h4 className="text-xs font-semibold text-gray-400 uppercase tracking-wider mb-1.5">
            Source
          </h4>
          <span className="text-xs font-mono text-blue-400">
            {item.source.file}:{item.source.line}
          </span>
        </div>

        {cycleEdgesForItem && (
          <CycleInfoSection
            entityLabel={item.label}
            allEdges={cycleEdgesForItem.allEdges}
          />
        )}

        {moduleCall && (
          <ModuleDetailSection moduleCall={moduleCall} fullProject={fullProject} onNavigateToModule={onNavigateToModule} />
        )}

        {attrEntries.length > 0 && (
          <div>
            <h4 className="text-xs font-semibold text-gray-400 uppercase tracking-wider mb-1.5">
              {selected.kind === "module" ? "Inputs" : "Attributes"}
            </h4>
            <div className="bg-gray-950/50 rounded-lg border border-gray-800 p-2.5">
              <table className="w-full text-xs">
                <tbody>
                  {attrEntries.map(([key, value]) => (
                    <tr
                      key={key}
                      className="border-b border-gray-800/50 last:border-0"
                    >
                      <td className="py-1.5 pr-4 text-purple-400 font-mono whitespace-nowrap align-top">
                        {key}
                      </td>
                      <td className="py-1.5 font-mono text-gray-300">
                        <JsonValue value={value} />
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </div>
        )}

        {item.references && item.references.length > 0 && (
          <div>
            <h4 className="text-xs font-semibold text-gray-400 uppercase tracking-wider mb-1.5">
              References
            </h4>
            <div className="flex flex-wrap gap-1.5">
              {item.references.map((ref) => (
                <span
                  key={ref}
                  className="text-[11px] font-mono bg-gray-800 text-gray-300 px-2 py-0.5 rounded border border-gray-700"
                >
                  {ref}
                </span>
              ))}
            </div>
          </div>
        )}

        {item.stateStatus === "drifted" && item.stateAttrs && (
          <StateDiff
            declared={item.attributes}
            state={item.stateAttrs}
          />
        )}

        {item.stateAttrs && item.stateStatus !== "drifted" && (
          <div>
            <h4 className="text-xs font-semibold text-gray-400 uppercase tracking-wider mb-1.5">
              State Attributes
            </h4>
            <div className="bg-gray-950/50 rounded-lg border border-gray-800 p-2.5">
              <table className="w-full text-xs">
                <tbody>
                  {Object.entries(item.stateAttrs).map(
                    ([key, value]) => (
                      <tr
                        key={key}
                        className="border-b border-gray-800/50 last:border-0"
                      >
                        <td className="py-1.5 pr-4 text-purple-400 font-mono whitespace-nowrap align-top">
                          {key}
                        </td>
                        <td className="py-1.5 font-mono text-gray-300">
                          <JsonValue value={value} />
                        </td>
                      </tr>
                    )
                  )}
                </tbody>
              </table>
            </div>
          </div>
        )}
      </div>
    </div>
  );
}
