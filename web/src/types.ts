export interface SourceLocation {
  file: string;
  line: number;
  endLine: number;
  column: number;
  endColumn: number;
}

export type StateStatus =
  | "unknown"
  | "in_sync"
  | "drifted"
  | "orphaned_in_state"
  | "not_in_state";

export interface Backend {
  type: string;
  config: Record<string, unknown>;
  accessible: boolean;
  source: SourceLocation;
}

export interface Provider {
  name: string;
  alias?: string;
  version?: string;
  config: Record<string, unknown>;
  source: SourceLocation;
}

export interface Resource {
  type: string;
  name: string;
  provider?: string;
  count?: unknown;
  forEach?: unknown;
  dependsOn?: string[];
  attributes: Record<string, unknown>;
  source: SourceLocation;
  stateStatus: StateStatus;
  stateAttrs?: Record<string, unknown>;
  references?: string[];
}

export interface DataSource {
  type: string;
  name: string;
  provider?: string;
  attributes: Record<string, unknown>;
  source: SourceLocation;
  references?: string[];
}

export interface ModuleCall {
  name: string;
  moduleSource: string;
  version?: string;
  inputs: Record<string, unknown>;
  dependsOn?: string[];
  providers?: Record<string, string>;
  source: SourceLocation;
  references?: string[];
}

export interface Variable {
  name: string;
  type?: string;
  description?: string;
  default?: unknown;
  value?: unknown;
  sensitive?: boolean;
  validation?: {
    condition: string;
    errorMessage: string;
  };
  source: SourceLocation;
  valueSource?: string;
}

export interface Output {
  name: string;
  description?: string;
  value?: unknown;
  sensitive?: boolean;
  dependsOn?: string[];
  source: SourceLocation;
  references?: string[];
}

export interface Local {
  name: string;
  expression: unknown;
  source: SourceLocation;
  references?: string[];
}

export interface StateSnapshot {
  serial: number;
  version: number;
  lineage: string;
  resources: StateResource[];
  outputs: Record<string, StateOutput>;
}

export interface StateResource {
  module?: string;
  mode: string;
  type: string;
  name: string;
  provider: string;
  instances: StateResourceInstance[];
}

export interface StateResourceInstance {
  index_key?: unknown;
  attributes: Record<string, unknown>;
}

export interface StateOutput {
  value: unknown;
  type: unknown;
  sensitive: boolean;
}

export type DiagSeverity = "error" | "warning" | "info";

export interface CycleEdge {
  from: string;
  to: string;
  source?: SourceLocation;
  viaRefs?: string[];
  snippet?: string;
}

export interface Diagnostic {
  severity: DiagSeverity;
  rule: string;
  message: string;
  detail?: string;
  source?: SourceLocation;
  entity?: string;
  cycleEdges?: CycleEdge[];
}

export interface TerraformModule {
  path: string;
  isRoot: boolean;
  hasBackend: boolean;
  backend?: Backend;
  resources: number;
  dataSources: number;
  variables: number;
  outputs: number;
  modules: number;
}

export interface OrphanedResource {
  rootModule?: string;
  module?: string;
  type: string;
  name: string;
  provider: string;
  attributes?: Record<string, unknown>;
}

export interface Project {
  path: string;
  backend?: Backend;
  providers: Provider[];
  resources: Resource[];
  dataSources: DataSource[];
  modules: ModuleCall[];
  variables: Variable[];
  outputs: Output[];
  locals: Local[];
  tfvars?: Record<string, unknown>;
  state?: StateSnapshot;
  moduleStates?: Record<string, StateSnapshot>;
  diagnostics: Diagnostic[];
  discoveredModules: TerraformModule[];
  orphanedResources?: OrphanedResource[];
}
