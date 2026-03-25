import { useProject } from "./hooks/useProject";
import { TreeExplorer } from "./components/TreeExplorer";
import { GraphView } from "./components/GraphView";
import { DetailPanel } from "./components/DetailPanel";
import { DiagnosticsBar, DiagnosticsPanel } from "./components/DiagnosticsPanel";
import { StatePanel } from "./components/StatePanel";
import { ModuleSelector } from "./components/ModuleSelector";
import { useState, useMemo, useCallback } from "react";
import type { Backend, Diagnostic, OrphanedResource, Project, StateSnapshot } from "./types";
import { resolveModuleCallToPath } from "./utils/module";

export type CycleHighlightSet = Set<string>;

export type SelectedItem = {
  kind: "resource" | "data" | "module" | "variable" | "output" | "local" | "provider";
  id: string;
};

function getModulePath(file: string): string {
  const dir = file.includes("/") ? file.substring(0, file.lastIndexOf("/")) : ".";
  return dir || ".";
}

function filterByModule(project: Project, modulePath: string | null): Project {
  if (!modulePath) return project;
  const match = (file: string) => getModulePath(file) === modulePath;
  return {
    ...project,
    resources: project.resources.filter((r) => match(r.source.file)),
    dataSources: project.dataSources.filter((d) => match(d.source.file)),
    modules: project.modules.filter((m) => match(m.source.file)),
    variables: project.variables.filter((v) => match(v.source.file)),
    outputs: project.outputs.filter((o) => match(o.source.file)),
    locals: project.locals.filter((l) => match(l.source.file)),
    providers: project.providers.filter((p) => match(p.source.file)),
    diagnostics: project.diagnostics.filter(
      (d) => !d.source || match(d.source.file)
    ),
  };
}

export default function App() {
  const { project, connected: _connected } = useProject();
  const [selected, setSelected] = useState<SelectedItem | null>(null);
  const [diagOpen, setDiagOpen] = useState(false);
  const [stateOpen, setStateOpen] = useState(false);
  const [activeModule, setActiveModule] = useState<string | null>(null);
  const [moduleHistory, setModuleHistory] = useState<(string | null)[]>([]);

  const pushModule = useCallback((next: string | null) => {
    setActiveModule((prev) => {
      if (prev === next) return prev;
      setModuleHistory((h) => [prev, ...h].slice(0, 2));
      return next;
    });
  }, []);

  const goBack = useCallback(() => {
    setModuleHistory((h) => {
      if (h.length === 0) return h;
      const [prev, ...rest] = h;
      setActiveModule(prev);
      setSelected(null);
      return rest;
    });
  }, []);

  const filteredProject = useMemo(
    () => (project ? filterByModule(project, activeModule) : null),
    [project, activeModule]
  );

  const cycleHighlights = useMemo<CycleHighlightSet>(() => {
    const set = new Set<string>();
    if (!filteredProject || !project) return set;
    for (const diag of filteredProject.diagnostics) {
      if (diag.rule !== "dependency-cycle" || !diag.cycleEdges?.length) continue;
      for (const edge of diag.cycleEdges) {
        set.add(edge.from);
        set.add(edge.to);
      }
    }

    if (activeModule && filteredProject) {
      const discovered = project.discoveredModules ?? [];
      const boundaryOutputs = new Set<string>();
      const boundaryVars = new Set<string>();

      for (const m of project.modules) {
        const resolvedPath = resolveModuleCallToPath(m, discovered);
        if (resolvedPath !== activeModule) continue;
        const moduleKey = `module.${m.name}`;

        for (const diag of project.diagnostics) {
          if (diag.rule !== "dependency-cycle" || !diag.cycleEdges?.length) continue;
          for (const edge of diag.cycleEdges) {
            if (edge.to === moduleKey && edge.viaRefs) {
              for (const ref of edge.viaRefs) {
                if (ref.startsWith(moduleKey + ".")) {
                  const outputName = ref.slice(moduleKey.length + 1).split(".")[0];
                  boundaryOutputs.add(`output.${outputName}`);
                }
              }
            }
            if (edge.from === moduleKey && edge.viaRefs) {
              for (const ref of edge.viaRefs) {
                for (const [inputName, inputValue] of Object.entries(m.inputs)) {
                  const valStr = typeof inputValue === "string"
                    ? inputValue
                    : JSON.stringify(inputValue);
                  if (valStr.includes(ref)) {
                    boundaryVars.add(`var.${inputName}`);
                  }
                }
              }
            }
          }
        }
      }

      for (const key of boundaryOutputs) set.add(key);
      for (const key of boundaryVars) set.add(key);

      if (boundaryOutputs.size > 0 || boundaryVars.size > 0) {
        const refToEntityKey = (ref: string): string | null => {
          const p = ref.split(".");
          if (p[0] === "var" && p.length >= 2) return `var.${p[1]}`;
          if (p[0] === "local" && p.length >= 2) return `local.${p[1]}`;
          if (p[0] === "output" && p.length >= 2) return `output.${p[1]}`;
          if (p[0] === "data" && p.length >= 3) return `data.${p[1]}.${p[2]}`;
          if (p[0] === "module" && p.length >= 2) return `module.${p[1]}`;
          if (p.length >= 2) return `${p[0]}.${p[1]}`;
          return null;
        };

        const entityRefs = new Map<string, string[]>();
        const reverseRefs = new Map<string, Set<string>>();

        const addEntity = (key: string, refs: string[] | undefined) => {
          const normalized = (refs ?? []).map(refToEntityKey).filter(Boolean) as string[];
          entityRefs.set(key, normalized);
          for (const target of normalized) {
            if (!reverseRefs.has(target)) reverseRefs.set(target, new Set());
            reverseRefs.get(target)!.add(key);
          }
        };

        for (const r of filteredProject.resources)
          addEntity(`${r.type}.${r.name}`, r.references);
        for (const d of filteredProject.dataSources)
          addEntity(`data.${d.type}.${d.name}`, d.references);
        for (const o of filteredProject.outputs)
          addEntity(`output.${o.name}`, o.references);
        for (const l of filteredProject.locals)
          addEntity(`local.${l.name}`, l.references);

        const visited = new Set<string>();
        const queue: string[] = [];

        for (const output of boundaryOutputs) {
          queue.push(output);
          visited.add(output);
        }
        while (queue.length > 0) {
          const entity = queue.pop()!;
          const deps = entityRefs.get(entity);
          if (!deps) continue;
          for (const dep of deps) {
            if (!visited.has(dep) && entityRefs.has(dep)) {
              visited.add(dep);
              set.add(dep);
              queue.push(dep);
            }
          }
        }

        for (const v of boundaryVars) {
          const dependents = reverseRefs.get(v);
          if (!dependents) continue;
          for (const dep of dependents) {
            if (!visited.has(dep)) {
              visited.add(dep);
              set.add(dep);
            }
          }
        }
      }
    }

    return set;
  }, [filteredProject, activeModule, project]);

  const activeBackend = useMemo<Backend | undefined>(() => {
    if (!project) return undefined;
    if (!activeModule || activeModule === ".") return project.backend;
    const dm = project.discoveredModules?.find((m) => m.path === activeModule);
    return dm?.backend ?? project.backend;
  }, [activeModule, project]);

  const activeState = useMemo<StateSnapshot | null>(() => {
    if (!project) return null;
    if (!activeModule || activeModule === ".") return project.state ?? null;
    return project.moduleStates?.[activeModule] ?? null;
  }, [activeModule, project]);

  const moduleStateMap = useMemo(() => {
    const map = new Map<string, string>();
    if (!project) return map;

    const discovered = project.discoveredModules ?? [];

    for (const m of project.modules) {
      const targetPath = resolveModuleCallToPath(m, discovered);
      if (!targetPath) continue;

      const childResources = project.resources.filter((r) => {
        const dir = getModulePath(r.source.file);
        return dir === targetPath || dir.startsWith(targetPath + "/");
      });

      if (childResources.length === 0) continue;

      const statuses = childResources
        .map((r) => r.stateStatus)
        .filter((s) => s && s !== "unknown");

      if (statuses.length === 0) continue;

      const allInSync = statuses.every((s) => s === "in_sync");
      const allDrifted = statuses.every((s) => s === "drifted");
      const hasDrifted = statuses.some((s) => s === "drifted");
      const hasNotInState = statuses.some((s) => s === "not_in_state");

      let status: string;
      if (hasNotInState) {
        status = "not_in_state";
      } else if (allInSync) {
        status = "in_sync";
      } else if (allDrifted) {
        status = "all_drifted";
      } else if (hasDrifted) {
        status = "partially_drifted";
      } else {
        status = "in_sync";
      }

      const callerDir = getModulePath(m.source.file);
      map.set(`${callerDir}:${m.name}`, status);
    }

    return map;
  }, [project]);

  const stateResources = useMemo(() => {
    if (!project || !activeModule) return filteredProject?.resources ?? [];
    const rootDir = activeModule === "." ? "." : activeModule;
    const reachableDirs = new Set<string>([rootDir]);
    const discovered = project.discoveredModules ?? [];
    let changed = true;
    while (changed) {
      changed = false;
      for (const m of project.modules) {
        const callerDir = getModulePath(m.source.file);
        if (!reachableDirs.has(callerDir)) continue;
        const targetPath = resolveModuleCallToPath(m, discovered);
        if (targetPath && !reachableDirs.has(targetPath)) {
          reachableDirs.add(targetPath);
          changed = true;
        }
      }
    }
    return project.resources.filter((r) => reachableDirs.has(getModulePath(r.source.file)));
  }, [project, activeModule, filteredProject]);

  const activeOrphans = useMemo<OrphanedResource[]>(() => {
    if (!project?.orphanedResources) return [];
    const rootKey = !activeModule || activeModule === "." ? "" : activeModule;
    return project.orphanedResources.filter((o) => (o.rootModule ?? "") === rootKey);
  }, [activeModule, project]);

  const navigateToModule = useCallback((modulePath: string) => {
    pushModule(modulePath);
    setSelected(null);
  }, [pushModule]);

  const handleCycleNavigate = useCallback((diag: Diagnostic) => {
    if (!diag.cycleEdges?.length) return;
    const sourceFile = diag.source?.file ?? diag.cycleEdges[0]?.source?.file;
    if (sourceFile) {
      const dir = sourceFile.includes("/")
        ? sourceFile.substring(0, sourceFile.lastIndexOf("/"))
        : ".";
      pushModule(dir || ".");
    }
    setSelected(null);
    setDiagOpen(false);
  }, [pushModule]);

  if (!project || !filteredProject) {
    return (
      <div className="flex items-center justify-center h-screen">
        <div className="text-gray-400 text-lg">Connecting to tfmap...</div>
      </div>
    );
  }

  const diagnostics: Diagnostic[] = filteredProject.diagnostics ?? [];
  const hasErrors = diagnostics.some((d) => d.severity === "error");
  const hasMultipleModules = (project.discoveredModules?.length ?? 0) > 1;

  return (
    <div className="flex flex-col h-screen">
      <header className="flex items-center justify-between px-4 py-2 border-b border-gray-800 bg-gray-900 shrink-0">
        <div className="flex items-center gap-3">
          <h1 className="text-base font-semibold text-white tracking-tight">tfmap</h1>
          <span className="text-xs text-gray-500 font-mono truncate max-w-xs" title={project.path}>
            {project.path}
          </span>
        </div>
        <div className="flex items-center gap-4">
          <DiagnosticsBar
            diagnostics={diagnostics}
            onToggle={() => { setDiagOpen((v) => !v); if (!diagOpen) setStateOpen(false); }}
            expanded={diagOpen}
          />
          <button
            onClick={() => { setStateOpen((v) => !v); if (!stateOpen) setDiagOpen(false); }}
            className="flex items-center gap-2 hover:bg-gray-800/50 px-2 py-1 rounded transition-colors"
          >
            <span
              className={`inline-block w-2 h-2 rounded-full ${activeBackend?.accessible ? "bg-green-500" : "bg-red-500"}`}
            />
            <span className="text-xs text-gray-400">
              {activeBackend?.accessible ? `State: ${activeBackend.type}` : "State: unavailable"}
            </span>
            <svg
              className={`w-3 h-3 text-gray-500 transition-transform ${stateOpen ? "rotate-180" : ""}`}
              fill="none"
              viewBox="0 0 24 24"
              stroke="currentColor"
              strokeWidth={2}
            >
              <path d="M19 9l-7 7-7-7" />
            </svg>
          </button>
        </div>
      </header>

      {hasMultipleModules && (
        <ModuleSelector
          modules={project.discoveredModules ?? []}
          activeModule={activeModule}
          onSelect={pushModule}
          onBack={goBack}
          canGoBack={moduleHistory.length > 0}
        />
      )}

      {diagOpen && diagnostics.length > 0 && (
        <DiagnosticsPanel
          diagnostics={diagnostics}
          onSelect={(item) => {
            setSelected(item);
            setDiagOpen(false);
          }}
          onCycleNavigate={handleCycleNavigate}
          onClose={() => setDiagOpen(false)}
        />
      )}

      {stateOpen && (
        <StatePanel
          state={activeState}
          backend={activeBackend}
          resources={stateResources}
          orphanedResources={activeOrphans}
          onClose={() => setStateOpen(false)}
        />
      )}

      {hasErrors && !diagOpen && (
        <div className="bg-red-500/10 border-b border-red-500/30 px-4 py-1.5 flex items-center gap-2 shrink-0">
          <svg className="w-4 h-4 text-red-400 shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
            <circle cx="12" cy="12" r="10" />
            <path d="M12 8v4m0 4h.01" />
          </svg>
          <span className="text-xs text-red-300">
            {diagnostics.filter((d) => d.severity === "error").length} error(s) detected.{" "}
            <button onClick={() => setDiagOpen(true)} className="underline hover:text-red-200">
              View diagnostics
            </button>
          </span>
        </div>
      )}

      <div className="flex flex-1 min-h-0">
        <TreeExplorer project={filteredProject} fullProject={project} selected={selected} onSelect={setSelected} onNavigateToModule={navigateToModule} cycleHighlights={cycleHighlights} moduleStateMap={moduleStateMap} />
        <div className="flex flex-col flex-1 min-w-0">
          <div className="flex-1 min-h-0">
            <GraphView project={filteredProject} fullProject={project} selected={selected} onSelect={setSelected} onNavigateToModule={navigateToModule} cycleHighlights={cycleHighlights} moduleStateMap={moduleStateMap} />
          </div>
          {selected && (
            <DetailPanel project={filteredProject} fullProject={project} selected={selected} onClose={() => setSelected(null)} onNavigateToModule={navigateToModule} cycleHighlights={cycleHighlights} />
          )}
        </div>
      </div>
    </div>
  );
}
