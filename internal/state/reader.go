package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Krokz/tfmap/internal/model"
)

type Reader struct{}

func NewReader() *Reader {
	return &Reader{}
}

func (r *Reader) Read(backend *model.Backend, projectPath string, profile string) (*model.StateSnapshot, error) {
	switch backend.Type {
	case "local", "":
		return r.ReadLocal(projectPath)
	case "s3":
		return r.ReadS3(backend, profile)
	default:
		return nil, nil
	}
}

func (r *Reader) ReadLocal(projectPath string) (*model.StateSnapshot, error) {
	statePath := filepath.Join(projectPath, "terraform.tfstate")
	data, err := os.ReadFile(statePath)
	if err != nil {
		return nil, err
	}

	var snap model.StateSnapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return nil, fmt.Errorf("parsing terraform.tfstate: %w", err)
	}

	return &snap, nil
}

type dirMapping struct {
	rootPath     string
	modulePrefix string
}

func CompareWithState(project *model.Project) {
	allStates := make(map[string]*model.StateSnapshot)
	if project.State != nil {
		allStates["."] = project.State
	}
	for path, snap := range project.ModuleStates {
		allStates[path] = snap
	}
	if len(allStates) == 0 {
		return
	}

	type stateEntry struct {
		resource *model.StateResource
		matched  bool
	}

	stateMaps := make(map[string]map[string]*stateEntry)
	for rootPath, snap := range allStates {
		m := make(map[string]*stateEntry)
		for i, sr := range snap.Resources {
			if sr.Mode != "managed" {
				continue
			}
			prefix := ""
			if sr.Module != "" {
				prefix = sr.Module + "."
			}
			key := prefix + sr.Type + "." + sr.Name
			m[key] = &stateEntry{resource: &snap.Resources[i]}
		}
		stateMaps[rootPath] = m
	}

	dirMappings := buildDirMappings(project)

	for i, res := range project.Resources {
		dir := filepath.Dir(res.Source.File)
		if !filepath.IsAbs(dir) {
			dir = filepath.Join(project.Path, dir)
		}

		mappings := lookupDirMappings(dir, dirMappings)
		if len(mappings) == 0 {
			continue
		}

		matched := false
		for _, mapping := range mappings {
			stateMap, ok := stateMaps[mapping.rootPath]
			if !ok {
				continue
			}
			key := mapping.modulePrefix + res.Type + "." + res.Name
			if entry, ok := stateMap[key]; ok {
				entry.matched = true
				if len(entry.resource.Instances) > 0 {
					project.Resources[i].StateAttrs = entry.resource.Instances[0].Attributes
				}
				if detectDrift(res.Attributes, project.Resources[i].StateAttrs) {
					project.Resources[i].StateStatus = model.StateStatusDrifted
				} else {
					project.Resources[i].StateStatus = model.StateStatusInSync
				}
				matched = true
				break
			}
		}

		if !matched {
			for _, mapping := range mappings {
				if _, ok := stateMaps[mapping.rootPath]; ok {
					project.Resources[i].StateStatus = model.StateStatusNotInState
					break
				}
			}
		}
	}

	var orphans []model.OrphanedResource
	for rootPath, entries := range stateMaps {
		for _, entry := range entries {
			if !entry.matched {
				sr := entry.resource
				attrs := map[string]interface{}{}
				if len(sr.Instances) > 0 {
					attrs = sr.Instances[0].Attributes
				}
				rm := rootPath
				if rm == "." {
					rm = ""
				}
				orphans = append(orphans, model.OrphanedResource{
					RootModule: rm,
					Module:     sr.Module,
					Type:       sr.Type,
					Name:       sr.Name,
					Provider:   sr.Provider,
					Attributes: attrs,
				})
			}
		}
	}
	project.OrphanedResources = orphans
}

// buildDirMappings creates a mapping from absolute directory paths to
// (rootModulePath, stateModulePrefix) pairs. It starts with root module
// directories, then iteratively follows module call source paths — including
// those that leave the root's own tree via "../" — so shared modules are
// correctly associated with every root that uses them.
func buildDirMappings(project *model.Project) map[string][]dirMapping {
	result := make(map[string][]dirMapping)

	if project.State != nil {
		result[project.Path] = append(result[project.Path], dirMapping{
			rootPath:     ".",
			modulePrefix: "",
		})
	}
	for path := range project.ModuleStates {
		absDir := filepath.Join(project.Path, path)
		result[absDir] = append(result[absDir], dirMapping{
			rootPath:     path,
			modulePrefix: "",
		})
	}

	for round := 0; round < 20; round++ {
		changed := false
		for _, mc := range project.Modules {
			src := mc.Source
			if !strings.HasPrefix(src, "./") && !strings.HasPrefix(src, "../") {
				continue
			}

			callerDir := filepath.Dir(mc.Location.File)
			if !filepath.IsAbs(callerDir) {
				callerDir = filepath.Join(project.Path, callerDir)
			}

			callerMappings := lookupDirMappings(callerDir, result)
			if len(callerMappings) == 0 {
				continue
			}

			absSource := filepath.Clean(filepath.Join(callerDir, src))

			for _, cm := range callerMappings {
				newMapping := dirMapping{
					rootPath:     cm.rootPath,
					modulePrefix: cm.modulePrefix + "module." + mc.Name + ".",
				}

				existing := result[absSource]
				found := false
				for _, e := range existing {
					if e.rootPath == newMapping.rootPath && e.modulePrefix == newMapping.modulePrefix {
						found = true
						break
					}
				}
				if !found {
					result[absSource] = append(result[absSource], newMapping)
					changed = true
				}
			}
		}
		if !changed {
			break
		}
	}

	return result
}

func lookupDirMappings(dir string, mappings map[string][]dirMapping) []dirMapping {
	var best []dirMapping
	bestLen := -1
	for mappedDir, ms := range mappings {
		if dir == mappedDir || strings.HasPrefix(dir, mappedDir+string(filepath.Separator)) {
			if len(mappedDir) > bestLen {
				best = ms
				bestLen = len(mappedDir)
			}
		}
	}
	return best
}

func isLiteralValue(val interface{}) bool {
	switch val.(type) {
	case string:
		s := val.(string)
		if strings.Contains(s, "${") || strings.HasPrefix(s, "var.") ||
			strings.HasPrefix(s, "module.") || strings.HasPrefix(s, "local.") ||
			strings.HasPrefix(s, "data.") || strings.HasPrefix(s, "each.") {
			return false
		}
		return true
	case float64, bool, json.Number:
		return true
	default:
		return false
	}
}

func detectDrift(hclAttrs, stateAttrs map[string]interface{}) bool {
	if hclAttrs == nil || stateAttrs == nil {
		return false
	}
	for key, hclVal := range hclAttrs {
		if !isLiteralValue(hclVal) {
			continue
		}
		stateVal, exists := stateAttrs[key]
		if !exists {
			continue
		}
		if fmt.Sprintf("%v", hclVal) != fmt.Sprintf("%v", stateVal) {
			return true
		}
	}
	return false
}
