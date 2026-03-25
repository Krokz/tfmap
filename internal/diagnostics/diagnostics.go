package diagnostics

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Krokz/tfmap/internal/model"
)

// referencedFilterExempt lists rules that should always appear regardless
// of whether the entity is referenced (e.g. orphaned-state-resource
// specifically targets unreferenced entities).
var referencedFilterExempt = map[string]bool{
	"orphaned-state-resource": true,
}

// shouldFilterUnreferenced returns true for entity types where we want to
// suppress diagnostics when the entity is not referenced by anything.
// Variables, data sources, outputs, and locals are always kept (they help
// the user identify optimization opportunities). Only modules and plain
// resources are filtered when unreferenced.
func shouldFilterUnreferenced(entity string) bool {
	if entity == "" {
		return false
	}
	for _, prefix := range []string{"var.", "data.", "output.", "local."} {
		if strings.HasPrefix(entity, prefix) {
			return false
		}
	}
	if !strings.Contains(entity, ".") {
		return false
	}
	return true
}

func Analyze(project *model.Project) []model.Diagnostic {
	var diags []model.Diagnostic
	diags = append(diags, checkDependencyCycles(project)...)
	diags = append(diags, checkVariableNoType(project)...)
	diags = append(diags, checkVariableNoDescription(project)...)
	diags = append(diags, checkOutputNoDescription(project)...)
	diags = append(diags, checkResourceNoTags(project)...)
	diags = append(diags, checkNoBackend(project)...)
	diags = append(diags, checkProviderNoVersion(project)...)
	diags = append(diags, checkUnusedVariables(project)...)
	diags = append(diags, checkOrphanedStateResources(project)...)
	diags = append(diags, checkSensitiveNaming(project)...)
	diags = append(diags, checkHighBlastRadius(project)...)

	referenced := buildReferencedSet(project)
	filtered := diags[:0]
	for _, d := range diags {
		if !shouldFilterUnreferenced(d.Entity) || referencedFilterExempt[d.Rule] || referenced[d.Entity] {
			filtered = append(filtered, d)
		}
	}
	diags = filtered

	sortDiagnostics(diags)
	return diags
}

func buildReferencedSet(project *model.Project) map[string]bool {
	set := make(map[string]bool)
	collect := func(refs []string) {
		for _, ref := range refs {
			if key := normalizeRef(ref); key != "" {
				set[key] = true
			}
			// normalizeRef excludes var.* for cycle detection, but we
			// need them here to know which variables are in use.
			parts := strings.SplitN(ref, ".", 3)
			if len(parts) >= 2 && parts[0] == "var" {
				set["var."+parts[1]] = true
			}
		}
	}
	for i := range project.Resources {
		collect(project.Resources[i].References)
	}
	for i := range project.DataSources {
		collect(project.DataSources[i].References)
	}
	for i := range project.Modules {
		collect(project.Modules[i].References)
	}
	for i := range project.Outputs {
		collect(project.Outputs[i].References)
	}
	for i := range project.Locals {
		collect(project.Locals[i].References)
	}
	return set
}

func severityOrder(s model.DiagSeverity) int {
	switch s {
	case model.DiagError:
		return 0
	case model.DiagWarning:
		return 1
	case model.DiagInfo:
		return 2
	default:
		return 3
	}
}

func sortDiagnostics(diags []model.Diagnostic) {
	sort.SliceStable(diags, func(i, j int) bool {
		si, sj := severityOrder(diags[i].Severity), severityOrder(diags[j].Severity)
		if si != sj {
			return si < sj
		}
		fi, li := "", 0
		fj, lj := "", 0
		if diags[i].Source != nil {
			fi = diags[i].Source.File
			li = diags[i].Source.Line
		}
		if diags[j].Source != nil {
			fj = diags[j].Source.File
			lj = diags[j].Source.Line
		}
		if fi != fj {
			return fi < fj
		}
		return li < lj
	})
}

// --- Rule 1: dependency-cycle ---

type entityEntry struct {
	key  string
	refs []string
	src  model.SourceLocation
}

func checkDependencyCycles(project *model.Project) []model.Diagnostic {
	// In Terraform, each module directory is an isolated scope.
	// Build a separate dependency graph per directory so that entities
	// with the same name in different modules don't create false cycles.
	dirEntities := make(map[string][]entityEntry)

	addEntity := func(key string, refs []string, src model.SourceLocation) {
		dir := filepath.Dir(src.File)
		dirEntities[dir] = append(dirEntities[dir], entityEntry{key, refs, src})
	}

	for i := range project.Resources {
		r := &project.Resources[i]
		addEntity(r.Type+"."+r.Name, r.References, r.Source)
	}
	for i := range project.DataSources {
		d := &project.DataSources[i]
		addEntity("data."+d.Type+"."+d.Name, d.References, d.Source)
	}
	for i := range project.Modules {
		m := &project.Modules[i]
		addEntity("module."+m.Name, m.References, m.Location)
	}
	for i := range project.Outputs {
		o := &project.Outputs[i]
		addEntity("output."+o.Name, o.References, o.Source)
	}
	for i := range project.Locals {
		l := &project.Locals[i]
		addEntity("local."+l.Name, l.References, l.Source)
	}

	// Build module call lookup: dir -> name -> *ModuleCall
	moduleCallsByDir := make(map[string]map[string]*model.ModuleCall)
	for i := range project.Modules {
		m := &project.Modules[i]
		dir := filepath.Dir(m.Location.File)
		if moduleCallsByDir[dir] == nil {
			moduleCallsByDir[dir] = make(map[string]*model.ModuleCall)
		}
		moduleCallsByDir[dir][m.Name] = m
	}

	var diags []model.Diagnostic

	for dir, entities := range dirEntities {
		graph := make(map[string][]string)
		sourceMap := make(map[string]*model.SourceLocation)
		rawRefs := make(map[string]map[string][]string)

		for _, e := range entities {
			src := e.src
			sourceMap[e.key] = &src
			for _, ref := range e.refs {
				if target := normalizeRef(ref); target != "" {
					graph[e.key] = append(graph[e.key], target)
					if rawRefs[e.key] == nil {
						rawRefs[e.key] = make(map[string][]string)
					}
					rawRefs[e.key][target] = append(rawRefs[e.key][target], ref)
				}
			}
		}

		nodeSet := make(map[string]bool)
		for k, neighbors := range graph {
			nodeSet[k] = true
			for _, n := range neighbors {
				nodeSet[n] = true
			}
		}

		cycles := detectCycles(graph, nodeSet)

		for _, cycle := range cycles {
			if isModuleOnlyCycle(cycle) &&
				!verifyModuleCycle(cycle, rawRefs, dir, moduleCallsByDir, dirEntities) {
				continue
			}

			entity := cycle[0]

			var detail strings.Builder
			var cycleEdges []model.CycleEdge
			for i := 0; i < len(cycle)-1; i++ {
				from := cycle[i]
				to := cycle[i+1]
				if i > 0 {
					detail.WriteString("\n")
				}
				loc := ""
				src := sourceMap[from]
				if src != nil {
					loc = fmt.Sprintf(" (%s:%d)", src.File, src.Line)
				}

				refs := rawRefs[from][to]
				refStr := ""
				if len(refs) > 0 {
					refStr = strings.Join(refs, ", ")
					detail.WriteString(fmt.Sprintf("%s%s -> %s (via %s)", from, loc, to, refStr))
				} else {
					detail.WriteString(fmt.Sprintf("%s%s -> %s", from, loc, to))
				}

				edge := model.CycleEdge{
					From:    from,
					To:      to,
					Source:  src,
					ViaRefs: refs,
				}
				if src != nil {
					edge.Snippet = extractSnippetWithRefs(project.Path, src, to, refs)
				}
				cycleEdges = append(cycleEdges, edge)
			}

			diags = append(diags, model.Diagnostic{
				Severity:   model.DiagError,
				Rule:       "dependency-cycle",
				Message:    fmt.Sprintf("Dependency cycle detected (%d entities)", len(cycle)-1),
				Detail:     detail.String(),
				Source:     sourceMap[entity],
				Entity:     entity,
				CycleEdges: cycleEdges,
			})
		}
	}
	return diags
}

func isModuleOnlyCycle(cycle []string) bool {
	for i := 0; i < len(cycle)-1; i++ {
		if !strings.HasPrefix(cycle[i], "module.") {
			return false
		}
	}
	return true
}

func isLocalModuleSource(source string) bool {
	return strings.HasPrefix(source, "./") || strings.HasPrefix(source, "../")
}

// verifyModuleCycle checks whether a cycle consisting entirely of module.*
// nodes is a real cycle by tracing through each module's internal dependency
// graph. Terraform resolves dependencies at the individual resource/output
// level inside modules, so a module-level cycle is only real if each
// referenced output transitively depends on the variable that creates the
// return edge. Returns false if any edge is proven to be a false positive.
func verifyModuleCycle(
	cycle []string,
	rawRefs map[string]map[string][]string,
	dir string,
	moduleCallsByDir map[string]map[string]*model.ModuleCall,
	dirEntities map[string][]entityEntry,
) bool {
	ring := cycle[:len(cycle)-1]
	ringLen := len(ring)

	for j := 0; j < ringLen; j++ {
		fromNode := ring[j]
		targetNode := ring[(j+1)%ringLen]
		targetName := strings.TrimPrefix(targetNode, "module.")

		dirModules := moduleCallsByDir[dir]
		if dirModules == nil {
			continue
		}
		targetCall := dirModules[targetName]
		if targetCall == nil {
			continue
		}

		if !isLocalModuleSource(targetCall.Source) {
			if fromNode == targetNode {
				return false
			}
			continue
		}

		targetDir := filepath.Clean(filepath.Join(dir, targetCall.Source))

		targetEntities := dirEntities[targetDir]
		if len(targetEntities) == 0 {
			continue
		}

		incomingOutputs := extractModuleOutputNames(rawRefs[fromNode][targetNode])
		if len(incomingOutputs) == 0 {
			continue
		}

		nextNode := ring[(j+2)%ringLen]
		nextName := strings.TrimPrefix(nextNode, "module.")

		var cyclingVars []string
		searchStr := "module." + nextName + "."
		for attrName, attrVal := range targetCall.Inputs {
			if strings.Contains(fmt.Sprintf("%v", attrVal), searchStr) {
				cyclingVars = append(cyclingVars, "var."+attrName)
			}
		}

		if len(cyclingVars) == 0 {
			continue
		}

		edgeReal := false
		for _, outputName := range incomingOutputs {
			reachable := transitiveVarDeps("output."+outputName, targetEntities)
			for _, v := range cyclingVars {
				if reachable[v] {
					edgeReal = true
					break
				}
			}
			if edgeReal {
				break
			}
		}

		if !edgeReal {
			return false
		}
	}

	return true
}

// extractModuleOutputNames extracts output names from raw reference strings
// like "module.foo.output_name".
func extractModuleOutputNames(refs []string) []string {
	seen := make(map[string]bool)
	var names []string
	for _, ref := range refs {
		parts := strings.SplitN(ref, ".", 3)
		if len(parts) >= 3 && parts[0] == "module" {
			if !seen[parts[2]] {
				seen[parts[2]] = true
				names = append(names, parts[2])
			}
		}
	}
	return names
}

// transitiveVarDeps does a BFS from startKey through the entities in a
// directory, following normalized references, and returns all var.* names
// reachable from the starting entity.
func transitiveVarDeps(startKey string, entities []entityEntry) map[string]bool {
	entityRefs := make(map[string][]string)
	for _, e := range entities {
		entityRefs[e.key] = e.refs
	}

	vars := make(map[string]bool)
	visited := make(map[string]bool)
	queue := []string{startKey}

	for len(queue) > 0 {
		key := queue[0]
		queue = queue[1:]
		if visited[key] {
			continue
		}
		visited[key] = true

		for _, ref := range entityRefs[key] {
			parts := strings.SplitN(ref, ".", 3)
			if len(parts) < 2 {
				continue
			}
			if parts[0] == "var" {
				vars["var."+parts[1]] = true
				continue
			}
			target := normalizeRef(ref)
			if target != "" && !visited[target] {
				queue = append(queue, target)
			}
		}
	}

	return vars
}

// extractSnippetWithRefs reads the source file and returns lines around the entity definition,
// highlighting lines that reference the target. Uses raw reference strings for precise search.
func extractSnippetWithRefs(projectPath string, src *model.SourceLocation, target string, viaRefs []string) string {
	filePath := src.File
	if projectPath != "" && !strings.HasPrefix(filePath, "/") {
		filePath = projectPath + "/" + filePath
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		return ""
	}

	lines := strings.Split(string(data), "\n")
	startLine := src.Line - 1
	endLine := src.EndLine
	if startLine < 0 {
		startLine = 0
	}
	if endLine > len(lines) {
		endLine = len(lines)
	}

	searchTerms := make(map[string]bool)
	for _, ref := range viaRefs {
		searchTerms[ref] = true
		parts := strings.SplitN(ref, ".", 3)
		if len(parts) >= 2 {
			searchTerms[parts[0]+"."+parts[1]] = true
		}
	}
	searchTerms[target] = true
	targetParts := strings.SplitN(target, ".", 3)
	if len(targetParts) >= 2 {
		searchTerms[targetParts[0]+"."+targetParts[1]] = true
	}

	const contextLines = 1
	refLineIndices := make(map[int]bool)
	matchedLines := make(map[int]bool)

	for i := startLine; i < endLine; i++ {
		code := stripHCLComment(lines[i])
		for term := range searchTerms {
			if strings.Contains(code, term) {
				matchedLines[i] = true
				for c := i - contextLines; c <= i+contextLines; c++ {
					if c >= startLine && c < endLine {
						refLineIndices[c] = true
					}
				}
			}
		}
	}

	// No highlighted lines found — snippet would be unhelpful
	if len(matchedLines) == 0 {
		return ""
	}

	refLineIndices[startLine] = true

	sorted := make([]int, 0, len(refLineIndices))
	for idx := range refLineIndices {
		sorted = append(sorted, idx)
	}
	sort.Ints(sorted)

	var snippetLines []string
	prev := -1
	for _, idx := range sorted {
		if prev >= 0 && idx > prev+1 {
			snippetLines = append(snippetLines, "     | ...")
		}
		marker := " "
		if matchedLines[idx] {
			marker = ">"
		}
		snippetLines = append(snippetLines, fmt.Sprintf("%4d %s %s", idx+1, marker, lines[idx]))
		prev = idx
	}

	return strings.Join(snippetLines, "\n")
}

// stripHCLComment returns the portion of a line before any HCL comment
// (# or //), ignoring comment characters inside quoted strings.
func stripHCLComment(line string) string {
	inQuote := false
	for i := 0; i < len(line); i++ {
		switch {
		case line[i] == '"' && (i == 0 || line[i-1] != '\\'):
			inQuote = !inQuote
		case !inQuote && line[i] == '#':
			return line[:i]
		case !inQuote && i+1 < len(line) && line[i] == '/' && line[i+1] == '/':
			return line[:i]
		}
	}
	return line
}

// normalizeRef converts a reference string to a graph node key.
// Returns "" for var.* references (variables are excluded from the graph).
func normalizeRef(ref string) string {
	parts := strings.SplitN(ref, ".", -1)
	if len(parts) < 2 {
		return ""
	}
	switch parts[0] {
	case "var":
		return ""
	case "data":
		if len(parts) >= 3 {
			return "data." + parts[1] + "." + parts[2]
		}
		return ""
	case "module":
		return "module." + parts[1]
	case "local":
		return "local." + parts[1]
	case "output":
		return "output." + parts[1]
	default:
		// resource: type.name
		if len(parts) >= 2 {
			return parts[0] + "." + parts[1]
		}
		return ""
	}
}

func detectCycles(graph map[string][]string, nodeSet map[string]bool) [][]string {
	const (
		white = 0
		gray  = 1
		black = 2
	)
	color := make(map[string]int)
	parent := make(map[string]string)
	var cycles [][]string
	seen := make(map[string]bool) // track cycle signatures to avoid duplicates

	var dfs func(node string)
	dfs = func(node string) {
		color[node] = gray
		for _, neighbor := range graph[node] {
			if color[neighbor] == gray {
				// Found a cycle - reconstruct it
				cycle := []string{neighbor}
				cur := node
				for cur != neighbor {
					cycle = append(cycle, cur)
					cur = parent[cur]
				}
				// Reverse to get the correct order
				for i, j := 0, len(cycle)-1; i < j; i, j = i+1, j-1 {
					cycle[i], cycle[j] = cycle[j], cycle[i]
				}
				cycle = append(cycle, cycle[0]) // close the cycle

				sig := canonicalCycleSig(cycle)
				if !seen[sig] {
					seen[sig] = true
					cycles = append(cycles, cycle)
				}
			} else if color[neighbor] == white {
				parent[neighbor] = node
				dfs(neighbor)
			}
		}
		color[node] = black
	}

	// Sort nodes for deterministic output
	nodes := make([]string, 0, len(nodeSet))
	for n := range nodeSet {
		nodes = append(nodes, n)
	}
	sort.Strings(nodes)

	for _, node := range nodes {
		if color[node] == white {
			dfs(node)
		}
	}
	return cycles
}

// canonicalCycleSig produces a canonical key for a cycle to deduplicate rotations.
func canonicalCycleSig(cycle []string) string {
	if len(cycle) <= 1 {
		return strings.Join(cycle, ",")
	}
	// The cycle is [a, b, ..., a]. Strip the closing element for rotation comparison.
	ring := cycle[:len(cycle)-1]
	min := 0
	for i := 1; i < len(ring); i++ {
		if ring[i] < ring[min] {
			min = i
		}
	}
	rotated := make([]string, len(ring))
	for i := range ring {
		rotated[i] = ring[(i+min)%len(ring)]
	}
	return strings.Join(rotated, ",")
}

// --- Rule 2: variable-no-type ---

func checkVariableNoType(project *model.Project) []model.Diagnostic {
	var diags []model.Diagnostic
	for i := range project.Variables {
		v := &project.Variables[i]
		if v.Type == "" {
			src := v.Source
			diags = append(diags, model.Diagnostic{
				Severity: model.DiagWarning,
				Rule:     "variable-no-type",
				Message:  fmt.Sprintf("Variable %q has no type constraint", v.Name),
				Source:   &src,
				Entity:   "var." + v.Name,
			})
		}
	}
	return diags
}

// --- Rule 3: variable-no-description ---

func checkVariableNoDescription(project *model.Project) []model.Diagnostic {
	var diags []model.Diagnostic
	for i := range project.Variables {
		v := &project.Variables[i]
		if v.Description == "" {
			src := v.Source
			diags = append(diags, model.Diagnostic{
				Severity: model.DiagWarning,
				Rule:     "variable-no-description",
				Message:  fmt.Sprintf("Variable %q has no description", v.Name),
				Source:   &src,
				Entity:   "var." + v.Name,
			})
		}
	}
	return diags
}

// --- Rule 4: output-no-description ---

func checkOutputNoDescription(project *model.Project) []model.Diagnostic {
	var diags []model.Diagnostic
	for i := range project.Outputs {
		o := &project.Outputs[i]
		if o.Description == "" {
			src := o.Source
			diags = append(diags, model.Diagnostic{
				Severity: model.DiagWarning,
				Rule:     "output-no-description",
				Message:  fmt.Sprintf("Output %q has no description", o.Name),
				Source:   &src,
				Entity:   "output." + o.Name,
			})
		}
	}
	return diags
}

// --- Rule 5: resource-no-tags ---

var awsTaggableTypes = map[string]bool{
	"aws_s3_bucket":             true,
	"aws_instance":              true,
	"aws_vpc":                   true,
	"aws_subnet":                true,
	"aws_security_group":        true,
	"aws_lb":                    true,
	"aws_ecs_cluster":           true,
	"aws_ecs_service":           true,
	"aws_lambda_function":       true,
	"aws_dynamodb_table":        true,
	"aws_rds_cluster":           true,
	"aws_rds_instance":          true,
	"aws_elasticache_cluster":   true,
	"aws_sqs_queue":             true,
	"aws_sns_topic":             true,
	"aws_cloudwatch_log_group":  true,
	"aws_iam_role":              true,
	"aws_iam_user":              true,
	"aws_eip":                   true,
	"aws_nat_gateway":           true,
	"aws_internet_gateway":      true,
	"aws_route_table":           true,
	"aws_kms_key":               true,
}

func checkResourceNoTags(project *model.Project) []model.Diagnostic {
	var diags []model.Diagnostic
	for i := range project.Resources {
		r := &project.Resources[i]
		if !awsTaggableTypes[r.Type] {
			continue
		}
		if _, ok := r.Attributes["tags"]; !ok {
			src := r.Source
			diags = append(diags, model.Diagnostic{
				Severity: model.DiagWarning,
				Rule:     "resource-no-tags",
				Message:  fmt.Sprintf("Resource %s.%s is missing tags", r.Type, r.Name),
				Source:   &src,
				Entity:   r.Type + "." + r.Name,
			})
		}
	}
	return diags
}

// --- Rule 6: no-backend ---

func checkNoBackend(project *model.Project) []model.Diagnostic {
	if project.Backend != nil {
		return nil
	}
	return []model.Diagnostic{{
		Severity: model.DiagWarning,
		Rule:     "no-backend",
		Message:  "No backend configuration found",
	}}
}

// --- Rule 7: provider-no-version ---

func checkProviderNoVersion(project *model.Project) []model.Diagnostic {
	var diags []model.Diagnostic
	for i := range project.Providers {
		p := &project.Providers[i]
		if p.Version != "" {
			continue
		}
		// Skip providers with no source location (likely implicit or from required_providers with source only)
		if p.Source.File == "" {
			continue
		}
		src := p.Source
		diags = append(diags, model.Diagnostic{
			Severity: model.DiagWarning,
			Rule:     "provider-no-version",
			Message:  fmt.Sprintf("Provider %q has no version constraint", p.Name),
			Source:   &src,
			Entity:   p.Name,
		})
	}
	return diags
}

// --- Rule 8: unused-variable ---

func checkUnusedVariables(project *model.Project) []model.Diagnostic {
	referenced := make(map[string]bool)

	collectVarRefs := func(refs []string) {
		for _, ref := range refs {
			parts := strings.SplitN(ref, ".", 3)
			if len(parts) >= 2 && parts[0] == "var" {
				referenced[parts[1]] = true
			}
		}
	}

	for i := range project.Resources {
		collectVarRefs(project.Resources[i].References)
	}
	for i := range project.DataSources {
		collectVarRefs(project.DataSources[i].References)
	}
	for i := range project.Modules {
		collectVarRefs(project.Modules[i].References)
	}
	for i := range project.Outputs {
		collectVarRefs(project.Outputs[i].References)
	}
	for i := range project.Locals {
		collectVarRefs(project.Locals[i].References)
	}

	// Also scan provider config values and module inputs for var.* references
	for i := range project.Providers {
		for _, v := range project.Providers[i].Config {
			if s, ok := v.(string); ok {
				if strings.HasPrefix(s, "var.") {
					referenced[s[4:]] = true
				} else if strings.Contains(s, "var.") {
					for _, part := range strings.Split(s, "var.") {
						if len(part) > 0 {
							name := strings.FieldsFunc(part, func(r rune) bool {
								return !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_')
							})
							if len(name) > 0 {
								referenced[name[0]] = true
							}
						}
					}
				}
			}
		}
	}

	var diags []model.Diagnostic
	for i := range project.Variables {
		v := &project.Variables[i]
		if !referenced[v.Name] {
			src := v.Source
			diags = append(diags, model.Diagnostic{
				Severity: model.DiagWarning,
				Rule:     "unused-variable",
				Message:  fmt.Sprintf("Variable %q is declared but never referenced", v.Name),
				Source:   &src,
				Entity:   "var." + v.Name,
			})
		}
	}
	return diags
}

// --- Rule 9: orphaned-state-resource ---

func checkOrphanedStateResources(project *model.Project) []model.Diagnostic {
	if project.State == nil {
		return nil
	}

	declared := make(map[string]bool)
	for i := range project.Resources {
		r := &project.Resources[i]
		declared[r.Type+"."+r.Name] = true
	}

	var diags []model.Diagnostic
	for i := range project.State.Resources {
		sr := &project.State.Resources[i]
		if sr.Mode != "managed" || sr.Module != "" {
			continue
		}
		key := sr.Type + "." + sr.Name
		if !declared[key] {
			diags = append(diags, model.Diagnostic{
				Severity: model.DiagWarning,
				Rule:     "orphaned-state-resource",
				Message:  fmt.Sprintf("State resource %s exists in state but not in configuration", key),
				Entity:   key,
			})
		}
	}
	return diags
}

// --- Rule 10: sensitive-naming ---

var sensitivePatterns = []string{
	"password", "secret", "token", "key", "api_key",
	"access_key", "private_key",
}

func checkSensitiveNaming(project *model.Project) []model.Diagnostic {
	var diags []model.Diagnostic
	for i := range project.Variables {
		v := &project.Variables[i]
		if v.Sensitive {
			continue
		}
		lower := strings.ToLower(v.Name)
		for _, pat := range sensitivePatterns {
			if strings.Contains(lower, pat) {
				src := v.Source
				diags = append(diags, model.Diagnostic{
					Severity: model.DiagInfo,
					Rule:     "sensitive-naming",
					Message:  fmt.Sprintf("Variable %q looks sensitive but is not marked as sensitive", v.Name),
					Source:   &src,
					Entity:   "var." + v.Name,
				})
				break
			}
		}
	}
	return diags
}

// --- Rule 11: high-blast-radius ---

func checkHighBlastRadius(project *model.Project) []model.Diagnostic {
	refCount := make(map[string]int)

	countRefs := func(refs []string) {
		seen := make(map[string]bool)
		for _, ref := range refs {
			target := normalizeRef(ref)
			if target == "" {
				continue
			}
			if seen[target] {
				continue
			}
			seen[target] = true
			refCount[target]++
		}
	}

	for i := range project.Resources {
		countRefs(project.Resources[i].References)
	}
	for i := range project.DataSources {
		countRefs(project.DataSources[i].References)
	}
	for i := range project.Modules {
		countRefs(project.Modules[i].References)
	}
	for i := range project.Outputs {
		countRefs(project.Outputs[i].References)
	}
	for i := range project.Locals {
		countRefs(project.Locals[i].References)
	}

	sourceMap := make(map[string]*model.SourceLocation)
	for i := range project.Resources {
		r := &project.Resources[i]
		src := r.Source
		sourceMap[r.Type+"."+r.Name] = &src
	}
	for i := range project.DataSources {
		d := &project.DataSources[i]
		src := d.Source
		sourceMap["data."+d.Type+"."+d.Name] = &src
	}
	for i := range project.Modules {
		m := &project.Modules[i]
		src := m.Location
		sourceMap["module."+m.Name] = &src
	}

	var diags []model.Diagnostic
	// Sort keys for deterministic output
	keys := make([]string, 0, len(refCount))
	for k := range refCount {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, entity := range keys {
		count := refCount[entity]
		if count >= 5 {
			diags = append(diags, model.Diagnostic{
				Severity: model.DiagInfo,
				Rule:     "high-blast-radius",
				Message:  fmt.Sprintf("%s is referenced by %d other entities", entity, count),
				Source:   sourceMap[entity],
				Entity:   entity,
			})
		}
	}
	return diags
}
