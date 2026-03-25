import { useState, useMemo, useEffect, memo, useCallback } from "react";
import type { Project } from "../types";
import type { SelectedItem, CycleHighlightSet } from "../App";
import { resolveModuleByName } from "../utils/module";

interface Props {
  project: Project;
  fullProject: Project;
  selected: SelectedItem | null;
  onSelect: (item: SelectedItem | null) => void;
  onNavigateToModule: (modulePath: string) => void;
  cycleHighlights: CycleHighlightSet;
  moduleStateMap: Map<string, string>;
}

const STATUS_COLORS: Record<string, string> = {
  in_sync: "bg-green-500",
  drifted: "bg-yellow-500",
  not_in_state: "bg-red-500",
  orphaned_in_state: "bg-orange-500",
  unknown: "bg-gray-500",
  partially_drifted: "bg-yellow-300",
  all_drifted: "bg-orange-400",
};

type CategoryKey =
  | "resources"
  | "data"
  | "modules"
  | "variables"
  | "outputs"
  | "locals"
  | "providers";

interface TreeItem {
  kind: SelectedItem["kind"];
  id: string;
  label: string;
  stateStatus?: string;
  inCycle?: boolean;
  sourceDir?: string;
}

const ITEMS_PER_PAGE = 100;

function dirFromFile(file: string): string {
  if (!file.includes("/")) return ".";
  return file.substring(0, file.lastIndexOf("/")) || ".";
}

function shortDir(dir: string): string {
  const parts = dir.split("/");
  return parts.length <= 2 ? dir : parts.slice(-2).join("/");
}

function buildItems(project: Project, moduleStateMap: Map<string, string>): Record<CategoryKey, TreeItem[]> {
  const raw: Record<CategoryKey, TreeItem[]> = {
    resources: project.resources.map((r) => ({
      kind: "resource" as const,
      id: `${r.type}.${r.name}`,
      label: `${r.type}.${r.name}`,
      stateStatus: r.stateStatus,
      sourceDir: dirFromFile(r.source.file),
    })),
    data: project.dataSources.map((d) => ({
      kind: "data" as const,
      id: `${d.type}.${d.name}`,
      label: `data.${d.type}.${d.name}`,
      sourceDir: dirFromFile(d.source.file),
    })),
    modules: project.modules.map((m) => {
      const dir = dirFromFile(m.source.file);
      return {
        kind: "module" as const,
        id: m.name,
        label: `module.${m.name}`,
        stateStatus: moduleStateMap.get(`${dir}:${m.name}`),
        sourceDir: dir,
      };
    }),
    variables: project.variables.map((v) => ({
      kind: "variable" as const,
      id: v.name,
      label: `var.${v.name}`,
      sourceDir: dirFromFile(v.source.file),
    })),
    outputs: project.outputs.map((o) => ({
      kind: "output" as const,
      id: o.name,
      label: `output.${o.name}`,
      sourceDir: dirFromFile(o.source.file),
    })),
    locals: project.locals.map((l) => ({
      kind: "local" as const,
      id: l.name,
      label: `local.${l.name}`,
      sourceDir: dirFromFile(l.source.file),
    })),
    providers: project.providers.map((p) => ({
      kind: "provider" as const,
      id: p.alias ? `${p.name}.${p.alias}` : p.name,
      label: p.alias ? `${p.name}.${p.alias}` : p.name,
      sourceDir: dirFromFile(p.source.file),
    })),
  };

  for (const key of CATEGORIES) {
    const items = raw[key];
    const labelCount = new Map<string, number>();
    for (const item of items) {
      labelCount.set(item.label, (labelCount.get(item.label) ?? 0) + 1);
    }
    let hasDupes = false;
    for (const c of labelCount.values()) {
      if (c > 1) { hasDupes = true; break; }
    }
    if (hasDupes) {
      for (const item of items) {
        if ((labelCount.get(item.label) ?? 0) > 1 && item.sourceDir) {
          item.id = `${item.id}@@${item.sourceDir}`;
        }
      }
    }
  }

  return raw;
}

const CATEGORY_LABELS: Record<CategoryKey, string> = {
  resources: "Resources",
  data: "Data Sources",
  modules: "Modules",
  variables: "Variables",
  outputs: "Outputs",
  locals: "Locals",
  providers: "Providers",
};

const CATEGORIES: CategoryKey[] = [
  "resources",
  "data",
  "modules",
  "variables",
  "outputs",
  "locals",
  "providers",
];

const TreeItemRow = memo(function TreeItemRow({
  item,
  isSelected,
  onClick,
  onDoubleClick,
}: {
  item: TreeItem;
  isSelected: boolean;
  onClick: () => void;
  onDoubleClick?: () => void;
}) {
  return (
    <button
      onClick={onClick}
      onDoubleClick={onDoubleClick}
      className={`flex items-center gap-2 w-full px-4 py-1.5 text-xs text-left transition-colors ${
        item.inCycle
          ? "bg-red-500/10 text-red-300 border-r-2 border-red-400"
          : isSelected
            ? "bg-indigo-500/15 text-indigo-300 border-r-2 border-indigo-400"
            : "text-gray-300 hover:bg-gray-800/70"
      }`}
    >
      {item.inCycle && (
        <span className="text-[8px] font-bold text-red-400 bg-red-500/20 px-1 py-0.5 rounded shrink-0">
          CYCLE
        </span>
      )}
      {!item.inCycle && item.stateStatus !== undefined && (
        <span
          className={`w-1.5 h-1.5 rounded-full shrink-0 ${
            STATUS_COLORS[item.stateStatus] ?? STATUS_COLORS.unknown
          }`}
        />
      )}
      <span className={`truncate font-mono text-[11px] ${item.inCycle ? "text-red-300" : ""}`}>
        {item.label}
      </span>
      {item.id.includes("@@") && item.sourceDir && (
        <span className="text-[9px] text-gray-600 bg-gray-800 px-1.5 py-0.5 rounded truncate max-w-[120px] shrink-0" title={item.sourceDir}>
          {shortDir(item.sourceDir)}
        </span>
      )}
      {item.kind === "module" && (
        <span className="ml-auto text-[9px] text-gray-600 shrink-0" title="Double-click to jump into module">
          &rarr;
        </span>
      )}
    </button>
  );
});

export function TreeExplorer({ project, fullProject, selected, onSelect, onNavigateToModule, cycleHighlights, moduleStateMap }: Props) {
  const [search, setSearch] = useState("");
  const totalEntityCount = useMemo(
    () =>
      project.resources.length +
      project.dataSources.length +
      project.modules.length +
      project.variables.length +
      project.outputs.length +
      project.locals.length +
      project.providers.length,
    [project]
  );

  const [collapsed, setCollapsed] = useState<Record<string, boolean>>(() => {
    if (totalEntityCount > 200) {
      const init: Record<string, boolean> = {};
      for (const key of CATEGORIES) init[key] = true;
      return init;
    }
    return {};
  });

  const [showMore, setShowMore] = useState<Record<string, number>>({});

  const allItems = useMemo(() => {
    const items = buildItems(project, moduleStateMap);
    if (cycleHighlights.size > 0) {
      for (const key of CATEGORIES) {
        items[key] = items[key].map((item) =>
          cycleHighlights.has(item.label) ? { ...item, inCycle: true } : item
        );
      }
    }
    return items;
  }, [project, cycleHighlights, moduleStateMap]);

  const filteredItems = useMemo(() => {
    if (!search) return allItems;
    const lower = search.toLowerCase();
    const result = {} as Record<CategoryKey, TreeItem[]>;
    for (const key of CATEGORIES) {
      result[key] = allItems[key].filter((item) =>
        item.label.toLowerCase().includes(lower)
      );
    }
    return result;
  }, [allItems, search]);

  useEffect(() => {
    if (cycleHighlights.size === 0) return;
    setCollapsed((prev) => {
      const next = { ...prev };
      for (const key of CATEGORIES) {
        if (allItems[key].some((item) => item.inCycle)) {
          next[key] = false;
        }
      }
      return next;
    });
  }, [cycleHighlights, allItems]);

  const toggleCollapse = useCallback((key: string) => {
    setCollapsed((prev) => ({ ...prev, [key]: !prev[key] }));
  }, []);

  const baseId = useCallback((id: string) => {
    const idx = id.indexOf("@@");
    return idx >= 0 ? id.substring(0, idx) : id;
  }, []);

  const isItemSelected = useCallback(
    (item: TreeItem) =>
      selected?.kind === item.kind && selected?.id === baseId(item.id),
    [selected, baseId]
  );

  const handleItemClick = useCallback(
    (item: TreeItem) => {
      onSelect(
        isItemSelected(item) ? null : { kind: item.kind, id: baseId(item.id) }
      );
    },
    [onSelect, isItemSelected]
  );

  const handleModuleDoubleClick = useCallback(
    (item: TreeItem) => {
      if (item.kind !== "module") return;
      const targetPath = resolveModuleByName(baseId(item.id), fullProject);
      if (targetPath) onNavigateToModule(targetPath);
    },
    [fullProject, onNavigateToModule, baseId]
  );

  const handleShowMore = useCallback((key: string) => {
    setShowMore((prev) => ({
      ...prev,
      [key]: (prev[key] ?? ITEMS_PER_PAGE) + ITEMS_PER_PAGE,
    }));
  }, []);

  return (
    <aside className="w-72 border-r border-gray-800 bg-gray-900 flex flex-col shrink-0">
      <div className="p-3 border-b border-gray-800">
        <div className="relative">
          <svg
            className="absolute left-2.5 top-1/2 -translate-y-1/2 w-3.5 h-3.5 text-gray-500"
            fill="none"
            viewBox="0 0 24 24"
            stroke="currentColor"
            strokeWidth={2}
          >
            <circle cx="11" cy="11" r="8" />
            <path d="m21 21-4.35-4.35" />
          </svg>
          <input
            type="text"
            placeholder="Filter..."
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            className="w-full pl-8 pr-3 py-1.5 text-sm bg-gray-800 border border-gray-700 rounded-md text-gray-200 placeholder-gray-500 focus:outline-none focus:border-indigo-500/50 focus:ring-1 focus:ring-indigo-500/30 transition-colors"
          />
        </div>
      </div>

      <div className="flex-1 overflow-y-auto">
        {CATEGORIES.map((key) => {
          const items = filteredItems[key];
          const total = allItems[key].length;
          if (total === 0) return null;
          const isCol = collapsed[key];
          const limit = showMore[key] ?? ITEMS_PER_PAGE;
          const visibleItems = items.slice(0, limit);
          const hasMore = items.length > limit;

          return (
            <div key={key}>
              <button
                onClick={() => toggleCollapse(key)}
                className="flex items-center justify-between w-full px-3 py-2 text-xs font-semibold text-gray-400 uppercase tracking-wider hover:bg-gray-800/50 transition-colors"
              >
                <span className="flex items-center gap-1.5">
                  <svg
                    className={`w-3 h-3 text-gray-500 transition-transform ${isCol ? "" : "rotate-90"}`}
                    fill="currentColor"
                    viewBox="0 0 20 20"
                  >
                    <path d="M6 4l8 6-8 6V4z" />
                  </svg>
                  {CATEGORY_LABELS[key]}
                </span>
                <span className="bg-gray-800 text-gray-500 px-1.5 py-0.5 rounded text-[10px] font-mono">
                  {search ? `${items.length}/${total}` : total}
                </span>
              </button>

              {!isCol && items.length > 0 && (
                <div className="pb-1">
                  {visibleItems.map((item) => (
                    <TreeItemRow
                      key={`${item.kind}-${item.id}`}
                      item={item}
                      isSelected={isItemSelected(item)}
                      onClick={() => handleItemClick(item)}
                      onDoubleClick={item.kind === "module" ? () => handleModuleDoubleClick(item) : undefined}
                    />
                  ))}
                  {hasMore && (
                    <button
                      onClick={() => handleShowMore(key)}
                      className="w-full px-4 py-1.5 text-[10px] text-indigo-400 hover:text-indigo-300 hover:bg-gray-800/50 transition-colors text-left"
                    >
                      Show more ({items.length - limit} remaining)...
                    </button>
                  )}
                </div>
              )}
            </div>
          );
        })}
      </div>
    </aside>
  );
}
