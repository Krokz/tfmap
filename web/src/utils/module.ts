import type { Project } from "../types";

export function resolveModuleCallToPath(
  moduleCall: { moduleSource: string; source: { file: string } },
  discoveredModules: { path: string }[]
): string | null {
  const src = moduleCall.moduleSource;
  if (!src || (!src.startsWith("./") && !src.startsWith("../"))) return null;
  const callerDir = moduleCall.source.file.includes("/")
    ? moduleCall.source.file.substring(0, moduleCall.source.file.lastIndexOf("/"))
    : ".";
  const parts = [...(callerDir === "." ? [] : callerDir.split("/")), ...src.split("/")];
  const resolved: string[] = [];
  for (const p of parts) {
    if (p === "." || p === "") continue;
    if (p === "..") { resolved.pop(); continue; }
    resolved.push(p);
  }
  const resolvedPath = resolved.length === 0 ? "." : resolved.join("/");
  return discoveredModules.some((m) => m.path === resolvedPath) ? resolvedPath : null;
}

export function resolveModuleByName(
  moduleName: string,
  project: Project
): string | null {
  const call = project.modules.find((m) => m.name === moduleName);
  if (!call) return null;
  return resolveModuleCallToPath(call, project.discoveredModules ?? []);
}
