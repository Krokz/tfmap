import { memo, useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  ReactFlow,
  Background,
  Controls,
  Handle,
  Position,
  applyNodeChanges,
  type Node,
  type Edge,
  type NodeProps,
  type OnNodesChange,
} from "@xyflow/react";
import "@xyflow/react/dist/style.css";
import * as dagre from "@dagrejs/dagre";
import type { Project } from "../types";
import type { SelectedItem, CycleHighlightSet } from "../App";
import { resolveModuleCallToPath, resolveModuleByName } from "../utils/module";

const MAX_GRAPH_NODES = 200;
const NODE_WIDTH = 220;
const NODE_HEIGHT = 44;

type NodeData = {
  label: string;
  nodeKind: "resource" | "data" | "module";
  stateStatus?: string;
  itemKind: SelectedItem["kind"];
  itemId: string;
  inCycle?: boolean;
  originDir?: string;
  expandedFrom?: string;
  isExpandable?: boolean;
  isExpanded?: boolean;
};

type TerraformNode = Node<NodeData, "terraform">;

function nodeDirFromFile(file: string): string {
  if (!file.includes("/")) return ".";
  return file.substring(0, file.lastIndexOf("/")) || ".";
}

function refToNodeIdInDir(ref: string, dir: string): string | null {
  if (ref.startsWith("var.") || ref.startsWith("local.")) return null;
  if (ref.startsWith("data.")) return `data:${dir}:${ref.slice(5)}`;
  if (ref.startsWith("module.")) {
    const moduleName = ref.slice(7).split(".")[0];
    return `module:${dir}:${moduleName}`;
  }
  if (ref.includes(".")) {
    const parts = ref.split(".");
    return `resource:${dir}:${parts[0]}.${parts[1]}`;
  }
  return null;
}

interface GraphResult {
  nodes: TerraformNode[];
  edges: Edge[];
  totalCount: number;
  truncated: boolean;
}

interface BuildGraphOpts {
  project: Project;
  fullProject: Project;
  moduleStateMap: Map<string, string>;
  expandedModules: Set<string>;
}

function buildGraph({ project, fullProject, moduleStateMap, expandedModules }: BuildGraphOpts): GraphResult {
  const nodeMap = new Map<string, TerraformNode>();
  const nodeDirMap = new Map<string, string>();

  for (const r of project.resources) {
    const dir = nodeDirFromFile(r.source.file);
    const id = `resource:${dir}:${r.type}.${r.name}`;
    if (!nodeMap.has(id)) {
      nodeMap.set(id, {
        id,
        type: "terraform",
        position: { x: 0, y: 0 },
        data: {
          label: `${r.type}.${r.name}`,
          nodeKind: "resource",
          stateStatus: r.stateStatus,
          itemKind: "resource",
          itemId: `${r.type}.${r.name}`,
        },
      });
      nodeDirMap.set(id, dir);
    }
  }

  for (const d of project.dataSources) {
    const dir = nodeDirFromFile(d.source.file);
    const id = `data:${dir}:${d.type}.${d.name}`;
    if (!nodeMap.has(id)) {
      nodeMap.set(id, {
        id,
        type: "terraform",
        position: { x: 0, y: 0 },
        data: {
          label: `data.${d.type}.${d.name}`,
          nodeKind: "data",
          itemKind: "data",
          itemId: `${d.type}.${d.name}`,
        },
      });
      nodeDirMap.set(id, dir);
    }
  }

  for (const m of project.modules) {
    const dir = nodeDirFromFile(m.source.file);
    const id = `module:${dir}:${m.name}`;
    const stateKey = `${dir}:${m.name}`;
    if (!nodeMap.has(id)) {
      nodeMap.set(id, {
        id,
        type: "terraform",
        position: { x: 0, y: 0 },
        data: {
          label: `module.${m.name}`,
          nodeKind: "module",
          stateStatus: moduleStateMap.get(stateKey),
          itemKind: "module",
          itemId: m.name,
          isExpandable: true,
          isExpanded: expandedModules.has(id),
        },
      });
      nodeDirMap.set(id, dir);
    }
  }

  // Expanded modules: add child entities from fullProject
  const expandedTargetPaths: string[] = [];
  const discovered = fullProject.discoveredModules ?? [];
  const expansionEdges: Edge[] = [];

  for (const m of project.modules) {
    const dir = nodeDirFromFile(m.source.file);
    const moduleId = `module:${dir}:${m.name}`;
    if (!expandedModules.has(moduleId)) continue;

    const targetPath = resolveModuleCallToPath(m, discovered);
    if (!targetPath) continue;
    expandedTargetPaths.push(targetPath);

    const addExpansionChild = (childId: string) => {
      expansionEdges.push({
        id: `expand:${moduleId}->${childId}`,
        source: moduleId,
        target: childId,
        data: { isExpansion: true },
      });
    };

    for (const r of fullProject.resources) {
      const rDir = nodeDirFromFile(r.source.file);
      if (rDir !== targetPath && !rDir.startsWith(targetPath + "/")) continue;
      const childId = `resource:${rDir}:${r.type}.${r.name}`;
      if (!nodeMap.has(childId)) {
        nodeMap.set(childId, {
          id: childId,
          type: "terraform",
          position: { x: 0, y: 0 },
          data: {
            label: `${r.type}.${r.name}`,
            nodeKind: "resource",
            stateStatus: r.stateStatus,
            itemKind: "resource",
            itemId: `${r.type}.${r.name}`,
            expandedFrom: `module.${m.name}`,
          },
        });
        nodeDirMap.set(childId, rDir);
      }
      addExpansionChild(childId);
    }

    for (const d of fullProject.dataSources) {
      const dDir = nodeDirFromFile(d.source.file);
      if (dDir !== targetPath && !dDir.startsWith(targetPath + "/")) continue;
      const childId = `data:${dDir}:${d.type}.${d.name}`;
      if (!nodeMap.has(childId)) {
        nodeMap.set(childId, {
          id: childId,
          type: "terraform",
          position: { x: 0, y: 0 },
          data: {
            label: `data.${d.type}.${d.name}`,
            nodeKind: "data",
            itemKind: "data",
            itemId: `${d.type}.${d.name}`,
            expandedFrom: `module.${m.name}`,
          },
        });
        nodeDirMap.set(childId, dDir);
      }
      addExpansionChild(childId);
    }

    for (const cm of fullProject.modules) {
      const cmDir = nodeDirFromFile(cm.source.file);
      if (cmDir !== targetPath && !cmDir.startsWith(targetPath + "/")) continue;
      const childId = `module:${cmDir}:${cm.name}`;
      if (!nodeMap.has(childId)) {
        const childStateKey = `${cmDir}:${cm.name}`;
        nodeMap.set(childId, {
          id: childId,
          type: "terraform",
          position: { x: 0, y: 0 },
          data: {
            label: `module.${cm.name}`,
            nodeKind: "module",
            stateStatus: moduleStateMap.get(childStateKey),
            itemKind: "module",
            itemId: cm.name,
            expandedFrom: `module.${m.name}`,
          },
        });
        nodeDirMap.set(childId, cmDir);
      }
      addExpansionChild(childId);
    }
  }

  const allNodes = Array.from(nodeMap.values());
  const totalCount = allNodes.length;
  const truncated = totalCount > MAX_GRAPH_NODES;
  const nodes = truncated ? allNodes.slice(0, MAX_GRAPH_NODES) : allNodes;
  const visibleIds = new Set(nodes.map((n) => n.id));

  // Collect reference sources from all entities that have nodes
  interface RefSource { dir: string; id: string; references?: string[] }
  const allRefSources: RefSource[] = [];

  for (const r of project.resources) {
    const dir = nodeDirFromFile(r.source.file);
    allRefSources.push({ dir, id: `resource:${dir}:${r.type}.${r.name}`, references: r.references });
  }
  for (const d of project.dataSources) {
    const dir = nodeDirFromFile(d.source.file);
    allRefSources.push({ dir, id: `data:${dir}:${d.type}.${d.name}`, references: d.references });
  }
  for (const m of project.modules) {
    const dir = nodeDirFromFile(m.source.file);
    allRefSources.push({ dir, id: `module:${dir}:${m.name}`, references: m.references });
  }

  for (const targetDir of expandedTargetPaths) {
    const matchDir = (d: string) => d === targetDir || d.startsWith(targetDir + "/");
    for (const r of fullProject.resources) {
      const rDir = nodeDirFromFile(r.source.file);
      if (matchDir(rDir)) allRefSources.push({ dir: rDir, id: `resource:${rDir}:${r.type}.${r.name}`, references: r.references });
    }
    for (const d of fullProject.dataSources) {
      const dDir = nodeDirFromFile(d.source.file);
      if (matchDir(dDir)) allRefSources.push({ dir: dDir, id: `data:${dDir}:${d.type}.${d.name}`, references: d.references });
    }
    for (const cm of fullProject.modules) {
      const cmDir = nodeDirFromFile(cm.source.file);
      if (matchDir(cmDir)) allRefSources.push({ dir: cmDir, id: `module:${cmDir}:${cm.name}`, references: cm.references });
    }
  }

  const refsBySource = new Map<string, Set<string>>();
  for (const src of allRefSources) {
    if (src.references?.length && visibleIds.has(src.id)) {
      const existing = refsBySource.get(src.id) ?? new Set();
      for (const ref of src.references) {
        const tid = refToNodeIdInDir(ref, src.dir);
        if (tid) existing.add(tid);
      }
      refsBySource.set(src.id, existing);
    }
  }

  const edgeIds = new Set<string>();
  const edges: Edge[] = [];
  for (const [sourceId, targetIds] of refsBySource) {
    for (const targetId of targetIds) {
      if (visibleIds.has(targetId) && targetId !== sourceId) {
        const edgeId = `${sourceId}->${targetId}`;
        if (!edgeIds.has(edgeId)) {
          edgeIds.add(edgeId);
          edges.push({ id: edgeId, source: sourceId, target: targetId });
        }
      }
    }
  }

  // Add expansion edges (parent module → expanded children) for layout grouping
  for (const ee of expansionEdges) {
    if (visibleIds.has(ee.source) && visibleIds.has(ee.target) && !edgeIds.has(ee.id)) {
      edgeIds.add(ee.id);
      edges.push(ee);
    }
  }

  layoutWithDagre(nodes, edges);

  const uniqueDirs = new Set(nodeDirMap.values());
  if (uniqueDirs.size > 1) {
    for (const node of nodes) {
      if (node.data.expandedFrom) continue;
      const dir = nodeDirMap.get(node.id);
      if (dir) {
        const short = dir.split("/").slice(-2).join("/");
        node.data = { ...node.data, originDir: dir === "." ? "root" : short };
      }
    }
  }

  return { nodes, edges, totalCount, truncated };
}

function layoutWithDagre(nodes: TerraformNode[], edges: Edge[]) {
  if (nodes.length === 0) return;

  const g = new dagre.graphlib.Graph();
  g.setGraph({
    rankdir: "LR",
    nodesep: 40,
    ranksep: 100,
    edgesep: 20,
    marginx: 40,
    marginy: 40,
  });
  g.setDefaultEdgeLabel(() => ({}));

  for (const node of nodes) {
    g.setNode(node.id, { width: NODE_WIDTH, height: NODE_HEIGHT });
  }
  for (const edge of edges) {
    g.setEdge(edge.source, edge.target);
  }

  dagre.layout(g);

  const nodeById = new Map(nodes.map((n) => [n.id, n]));
  for (const id of g.nodes()) {
    const dagreNode = g.node(id);
    const node = nodeById.get(id);
    if (node && dagreNode) {
      node.position = {
        x: dagreNode.x - NODE_WIDTH / 2,
        y: dagreNode.y - NODE_HEIGHT / 2,
      };
    }
  }
}

const KIND_STYLES = {
  resource: {
    border: "border-l-indigo-500",
    badge: "bg-indigo-500/20 text-indigo-300",
    label: "RES",
  },
  data: {
    border: "border-l-teal-500",
    badge: "bg-teal-500/20 text-teal-300",
    label: "DATA",
  },
  module: {
    border: "border-l-purple-500",
    badge: "bg-purple-500/20 text-purple-300",
    label: "MOD",
  },
};

const STATUS_DOT: Record<string, string> = {
  in_sync: "bg-green-500",
  drifted: "bg-yellow-500",
  not_in_state: "bg-red-500",
  orphaned_in_state: "bg-orange-500",
  unknown: "bg-gray-500",
  partially_drifted: "bg-yellow-300",
  all_drifted: "bg-orange-400",
};

const TerraformNodeComponent = memo(function TerraformNodeComponent({
  data,
  selected,
}: NodeProps<TerraformNode>) {
  const style = KIND_STYLES[data.nodeKind];
  const isCycle = data.inCycle;
  const isExpanded = data.isExpanded;
  const hasTag = data.originDir || data.expandedFrom || isExpanded;
  return (
    <div
      className={`rounded-lg border-l-4 ${
        isCycle
          ? "border-l-red-500"
          : isExpanded
            ? "border-l-purple-500"
            : style.border
      } bg-gray-800/90 px-3 min-w-[180px] max-w-[280px] shadow-lg ${
        hasTag ? "pt-1 pb-2" : "py-2.5"
      } ${
        isCycle
          ? "ring-2 ring-red-500/60 shadow-red-500/30 shadow-xl"
          : isExpanded
            ? "ring-2 ring-purple-500/40 shadow-purple-500/20 shadow-xl"
            : selected
              ? "ring-2 ring-indigo-400/70 shadow-indigo-500/20 shadow-xl"
              : "ring-1 ring-gray-700/50"
      }`}
    >
      <Handle
        type="target"
        position={Position.Left}
        className="!bg-gray-500 !border-gray-600 !w-2 !h-2"
      />
      {data.expandedFrom && (
        <div className="text-[8px] font-mono text-purple-400/60 truncate mb-0.5">
          ↳ {data.expandedFrom}
        </div>
      )}
      {isExpanded && (
        <div className="text-[8px] font-mono text-purple-400/60 truncate mb-0.5">
          expanded
        </div>
      )}
      {!data.expandedFrom && !isExpanded && data.originDir && (
        <div className="text-[8px] font-mono text-gray-500 truncate mb-0.5">
          {data.originDir}
        </div>
      )}
      <div className="flex items-center gap-2">
        <span
          className={`text-[9px] font-bold tracking-wider ${
            isCycle ? "bg-red-500/25 text-red-300" : style.badge
          } px-1.5 py-0.5 rounded`}
        >
          {isCycle ? "CYCLE" : style.label}
        </span>
        <span className={`text-[11px] font-mono truncate flex-1 ${isCycle ? "text-red-200" : "text-gray-200"}`}>
          {data.label}
        </span>
        {data.stateStatus && (
          <span
            className={`w-2 h-2 rounded-full shrink-0 ${STATUS_DOT[data.stateStatus] ?? STATUS_DOT.unknown}`}
            title={data.stateStatus.replace(/_/g, " ")}
          />
        )}
        {data.isExpandable && (
          <button
            data-action="expand"
            className={`w-5 h-5 flex items-center justify-center rounded transition-colors shrink-0 ${
              isExpanded
                ? "bg-purple-500/25 text-purple-300 hover:bg-purple-500/40"
                : "bg-gray-700/50 text-gray-400 hover:bg-gray-600/50 hover:text-gray-200"
            }`}
            title={isExpanded ? "Collapse module" : "Expand module resources"}
          >
            <svg className="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
              {isExpanded ? (
                <path d="M5 12h14" />
              ) : (
                <path d="M12 5v14M5 12h14" />
              )}
            </svg>
          </button>
        )}
      </div>
      <Handle
        type="source"
        position={Position.Right}
        className="!bg-gray-500 !border-gray-600 !w-2 !h-2"
      />
    </div>
  );
});

const nodeTypes = { terraform: TerraformNodeComponent };

interface Props {
  project: Project;
  fullProject: Project;
  selected: SelectedItem | null;
  onSelect: (item: SelectedItem | null) => void;
  onNavigateToModule: (modulePath: string) => void;
  cycleHighlights: CycleHighlightSet;
  moduleStateMap: Map<string, string>;
}

type EdgeMode = "off" | "selected" | "all";

function getActiveNodeIds(
  nodes: TerraformNode[],
  selected: SelectedItem | null
): Set<string> {
  if (!selected) return new Set();
  const set = new Set<string>();
  for (const n of nodes) {
    if (n.data.itemKind === selected.kind && n.data.itemId === selected.id) {
      set.add(n.id);
    }
  }
  return set;
}

const DIM_STYLE = { stroke: "#374151", strokeWidth: 1, opacity: 0.15 };
const NORMAL_STYLE = { stroke: "#4b5563", strokeWidth: 1, opacity: 0.35 };
const ACTIVE_STYLE = { stroke: "#818cf8", strokeWidth: 2, opacity: 1 };
const EXPANSION_STYLE = { stroke: "#a78bfa", strokeWidth: 1, opacity: 0.35, strokeDasharray: "4 3" };
const EXPANSION_ACTIVE_STYLE = { stroke: "#a78bfa", strokeWidth: 1.5, opacity: 0.7, strokeDasharray: "4 3" };

function isExpansionEdge(e: Edge): boolean {
  return !!(e.data as { isExpansion?: boolean })?.isExpansion;
}

function computeVisibleEdges(
  allEdges: Edge[],
  nodePositions: Map<string, { x: number; y: number }>,
  mode: EdgeMode,
  activeNodeIds: Set<string>
): Edge[] {
  if (mode === "off") {
    // Still show expansion edges when edges are off — they show structure
    return allEdges
      .filter(isExpansionEdge)
      .map((e) => ({ ...e, type: "default", style: EXPANSION_STYLE }));
  }

  const edges = allEdges.map((e) => {
    const srcPos = nodePositions.get(e.source);
    const tgtPos = nodePositions.get(e.target);
    const isBackward = srcPos && tgtPos && srcPos.x > tgtPos.x;
    return {
      ...e,
      type: "default",
      ...(isBackward
        ? { sourceHandle: null, targetHandle: null, sourcePosition: Position.Left, targetPosition: Position.Right }
        : {}),
    } as Edge;
  });

  if (mode === "selected") {
    if (activeNodeIds.size === 0) {
      return edges
        .filter(isExpansionEdge)
        .map((e) => ({ ...e, style: EXPANSION_STYLE }));
    }
    const filtered = edges.filter(
      (e) => activeNodeIds.has(e.source) || activeNodeIds.has(e.target)
    );
    return filtered.map((e) =>
      isExpansionEdge(e)
        ? { ...e, style: EXPANSION_ACTIVE_STYLE }
        : { ...e, style: ACTIVE_STYLE, animated: true }
    );
  }

  // mode === "all"
  if (activeNodeIds.size > 0) {
    return edges.map((e) => {
      if (isExpansionEdge(e)) {
        const connected = activeNodeIds.has(e.source) || activeNodeIds.has(e.target);
        return { ...e, style: connected ? EXPANSION_ACTIVE_STYLE : EXPANSION_STYLE };
      }
      const connected = activeNodeIds.has(e.source) || activeNodeIds.has(e.target);
      return {
        ...e,
        style: connected ? ACTIVE_STYLE : DIM_STYLE,
        animated: connected,
        zIndex: connected ? 10 : 0,
      };
    });
  }

  return edges.map((e) =>
    isExpansionEdge(e)
      ? { ...e, style: EXPANSION_STYLE }
      : { ...e, style: NORMAL_STYLE }
  );
}

const EDGE_MODE_LABELS: Record<EdgeMode, string> = {
  off: "Edges off",
  selected: "Selected only",
  all: "All edges",
};

const EDGE_MODES: EdgeMode[] = ["off", "selected", "all"];

export function GraphView({ project, fullProject, selected, onSelect, onNavigateToModule, cycleHighlights, moduleStateMap }: Props) {
  const [expandedModules, setExpandedModules] = useState<Set<string>>(new Set());
  const graphData = useMemo(
    () => buildGraph({ project, fullProject, moduleStateMap, expandedModules }),
    [project, fullProject, moduleStateMap, expandedModules]
  );
  const [tfNodes, setTfNodes] = useState(graphData.nodes);
  const [edgeMode, setEdgeMode] = useState<EdgeMode>("all");
  const prevGraphRef = useRef(graphData);

  const activeNodeIds = useMemo(
    () => getActiveNodeIds(graphData.nodes, selected),
    [graphData.nodes, selected]
  );

  const nodePositions = useMemo(() => {
    const map = new Map<string, { x: number; y: number }>();
    for (const n of graphData.nodes) {
      map.set(n.id, n.position);
    }
    return map;
  }, [graphData.nodes]);

  const visibleEdges = useMemo(
    () => computeVisibleEdges(graphData.edges, nodePositions, edgeMode, activeNodeIds),
    [graphData.edges, nodePositions, edgeMode, activeNodeIds]
  );

  useEffect(() => {
    if (prevGraphRef.current !== graphData) {
      prevGraphRef.current = graphData;
      setTfNodes(graphData.nodes);
    }
  }, [graphData]);

  useEffect(() => {
    if (!selected) return;
    setTfNodes((nds) =>
      nds.map((n) => {
        const shouldBeSelected =
          selected.kind === n.data.itemKind && selected.id === n.data.itemId;
        if (n.selected === shouldBeSelected) return n;
        return { ...n, selected: shouldBeSelected };
      })
    );
  }, [selected]);

  useEffect(() => {
    setTfNodes((nds) =>
      nds.map((n) => {
        const inCycle = cycleHighlights.size > 0 && cycleHighlights.has(n.data.label);
        if (n.data.inCycle === inCycle) return n;
        return { ...n, data: { ...n.data, inCycle } };
      })
    );
  }, [cycleHighlights]);

  const onNodesChange: OnNodesChange<TerraformNode> = useCallback(
    (changes) => setTfNodes((nds) => applyNodeChanges(changes, nds)),
    []
  );

  const handleNodeClick = useCallback(
    (_event: unknown, node: TerraformNode) => {
      const target = (_event as { target: EventTarget | null }).target as HTMLElement | null;
      if (target?.closest?.("[data-action=\"expand\"]")) {
        setExpandedModules((prev) => {
          const next = new Set(prev);
          if (next.has(node.id)) next.delete(node.id);
          else next.add(node.id);
          return next;
        });
        return;
      }
      onSelect({ kind: node.data.itemKind, id: node.data.itemId });
    },
    [onSelect]
  );

  const handleNodeDoubleClick = useCallback(
    (_event: unknown, node: TerraformNode) => {
      if (node.data.nodeKind !== "module") return;
      const targetPath = resolveModuleByName(node.data.itemId, fullProject);
      if (targetPath) onNavigateToModule(targetPath);
    },
    [fullProject, onNavigateToModule]
  );

  const cycleEdgeMode = useCallback(() => {
    setEdgeMode((m) => EDGE_MODES[(EDGE_MODES.indexOf(m) + 1) % EDGE_MODES.length]);
  }, []);

  if (graphData.totalCount === 0) {
    return (
      <div className="w-full h-full flex items-center justify-center text-gray-600 text-sm">
        No resources to display
      </div>
    );
  }

  return (
    <div className="w-full h-full relative">
      {graphData.truncated && (
        <div className="absolute top-3 left-1/2 -translate-x-1/2 z-10 bg-gray-800/90 border border-gray-700 rounded-lg px-4 py-2 text-xs text-gray-400 flex items-center gap-2">
          <svg className="w-4 h-4 text-amber-500 shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
            <path d="M12 9v4m0 4h.01M10.29 3.86l-8.6 14.86A1 1 0 002.56 20h18.88a1 1 0 00.87-1.28l-8.6-14.86a1 1 0 00-1.72 0z" />
          </svg>
          Showing {MAX_GRAPH_NODES} of {graphData.totalCount} nodes. Select a specific module for the full graph.
        </div>
      )}

      <div className="absolute top-3 right-3 z-10">
        <button
          onClick={cycleEdgeMode}
          className="flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-xs font-medium border bg-gray-800/90 border-gray-700 text-gray-300 hover:bg-gray-700/90 transition-colors"
          title="Cycle edge display mode"
        >
          <svg className="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
            <path d="M13 10V3L4 14h7v7l9-11h-7z" />
          </svg>
          {EDGE_MODE_LABELS[edgeMode]}
          {edgeMode !== "off" && (
            <span className="text-gray-500 ml-1">({graphData.edges.length})</span>
          )}
        </button>
      </div>

      <ReactFlow
        nodes={tfNodes}
        edges={visibleEdges}
        onNodesChange={onNodesChange}
        onNodeClick={handleNodeClick}
        onNodeDoubleClick={handleNodeDoubleClick}
        onPaneClick={() => onSelect(null)}
        nodeTypes={nodeTypes}
        colorMode="dark"
        fitView
        fitViewOptions={{ padding: 0.15 }}
        minZoom={0.05}
        maxZoom={2}
      >
        <Background color="#1f2937" gap={24} size={1} />
        <Controls showInteractive={false} />
      </ReactFlow>
    </div>
  );
}
