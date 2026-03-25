package parser

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Krokz/tfmap/internal/model"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"
)

var builtinPrefixes = map[string]bool{
	"each": true, "count": true, "self": true, "terraform": true,
	"path": true, "null": true,
}

func Parse(projectPath string) (*model.Project, error) {
	project := &model.Project{
		Path:        projectPath,
		Providers:   []model.Provider{},
		Resources:   []model.Resource{},
		DataSources: []model.DataSource{},
		Modules:     []model.ModuleCall{},
		Variables:   []model.Variable{},
		Outputs:     []model.Output{},
		Locals:      []model.Local{},
	}

	moduleDirs, err := discoverModuleDirs(projectPath)
	if err != nil {
		return nil, fmt.Errorf("discovering modules: %w", err)
	}

	for _, dir := range moduleDirs {
		if err := parseModuleDir(dir, projectPath, project); err != nil {
			relDir, _ := filepath.Rel(projectPath, dir)
			if relDir == "" {
				relDir = "."
			}
			return nil, fmt.Errorf("parsing %s: %w", relDir, err)
		}
	}

	project.TFVars = make(map[string]interface{})
	for _, dir := range moduleDirs {
		if err := parseTFVarsInDir(dir, projectPath, project); err != nil {
			return nil, err
		}
	}

	for i, v := range project.Variables {
		if val, ok := project.TFVars[v.Name]; ok {
			project.Variables[i].Value = val
			project.Variables[i].ValueSource = "tfvars"
		}
	}

	project.DiscoveredModules = buildModuleList(project, moduleDirs, projectPath)

	return project, nil
}

// discoverModuleDirs finds all directories containing .tf files, skipping .terraform/ dirs.
func discoverModuleDirs(root string) ([]string, error) {
	dirSet := make(map[string]bool)

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			base := filepath.Base(path)
			if base == ".terraform" || base == ".terragrunt-cache" {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) == ".tf" {
			dirSet[filepath.Dir(path)] = true
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	dirs := make([]string, 0, len(dirSet))
	for d := range dirSet {
		dirs = append(dirs, d)
	}
	sort.Strings(dirs)
	return dirs, nil
}

func parseModuleDir(dir, projectRoot string, project *model.Project) error {
	tfFiles, err := filepath.Glob(filepath.Join(dir, "*.tf"))
	if err != nil {
		return fmt.Errorf("globbing .tf files: %w", err)
	}

	for _, f := range tfFiles {
		if err := parseTFFile(f, projectRoot, project); err != nil {
			return fmt.Errorf("parsing %s: %w", filepath.Base(f), err)
		}
	}
	return nil
}

func parseTFVarsInDir(dir, projectRoot string, project *model.Project) error {
	varFiles := map[string]bool{}
	tfvarsFiles, _ := filepath.Glob(filepath.Join(dir, "*.tfvars"))
	for _, f := range tfvarsFiles {
		varFiles[f] = true
	}
	autoFiles, _ := filepath.Glob(filepath.Join(dir, "*.auto.tfvars"))
	for _, f := range autoFiles {
		varFiles[f] = true
	}

	for f := range varFiles {
		if err := parseTFVarsFile(f, project); err != nil {
			relFile, _ := filepath.Rel(projectRoot, f)
			return fmt.Errorf("parsing %s: %w", relFile, err)
		}
	}
	return nil
}

func buildModuleList(project *model.Project, dirs []string, root string) []model.TerraformModule {
	type modStats struct {
		resources   int
		dataSources int
		variables   int
		outputs     int
		modules     int
		hasBackend  bool
		backend     *model.Backend
	}

	stats := make(map[string]*modStats)
	for _, dir := range dirs {
		rel, _ := filepath.Rel(root, dir)
		if rel == "" || rel == "." {
			rel = "."
		}
		stats[rel] = &modStats{}
	}

	getDirKey := func(file string) string {
		dir := filepath.Dir(file)
		if dir == "." || dir == "" {
			return "."
		}
		return dir
	}

	for _, r := range project.Resources {
		if s, ok := stats[getDirKey(r.Source.File)]; ok {
			s.resources++
		}
	}
	for _, d := range project.DataSources {
		if s, ok := stats[getDirKey(d.Source.File)]; ok {
			s.dataSources++
		}
	}
	for _, v := range project.Variables {
		if s, ok := stats[getDirKey(v.Source.File)]; ok {
			s.variables++
		}
	}
	for _, o := range project.Outputs {
		if s, ok := stats[getDirKey(o.Source.File)]; ok {
			s.outputs++
		}
	}
	for _, m := range project.Modules {
		if s, ok := stats[getDirKey(m.Location.File)]; ok {
			s.modules++
		}
	}

	for i := range project.Backends {
		b := &project.Backends[i]
		key := getDirKey(b.Source.File)
		if s, ok := stats[key]; ok {
			s.hasBackend = true
			s.backend = b
		}
	}

	// Build set of directories that are referenced as module sources.
	// These are child/reusable modules. Everything else is a root module.
	childModulePaths := make(map[string]bool)
	for _, m := range project.Modules {
		src := m.Source
		if src == "" || (!strings.HasPrefix(src, "./") && !strings.HasPrefix(src, "../")) {
			continue
		}
		callerDir := getDirKey(m.Location.File)
		resolved := resolveRelativePath(callerDir, src)
		if resolved != "" {
			childModulePaths[resolved] = true
		}
	}

	result := make([]model.TerraformModule, 0, len(stats))
	keys := make([]string, 0, len(stats))
	for k := range stats {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		s := stats[k]
		isRoot := s.hasBackend || k == "." || (!childModulePaths[k] && s.modules > 0)
		result = append(result, model.TerraformModule{
			Path:        k,
			IsRoot:      isRoot,
			HasBackend:  s.hasBackend,
			Backend:     s.backend,
			Resources:   s.resources,
			DataSources: s.dataSources,
			Variables:   s.variables,
			Outputs:     s.outputs,
			Modules:     s.modules,
		})
	}
	return result
}

// resolveRelativePath resolves a relative source path against a caller directory.
func resolveRelativePath(callerDir, source string) string {
	var parts []string
	if callerDir != "." && callerDir != "" {
		parts = strings.Split(callerDir, "/")
	}
	for _, seg := range strings.Split(source, "/") {
		switch seg {
		case ".", "":
			continue
		case "..":
			if len(parts) > 0 {
				parts = parts[:len(parts)-1]
			}
		default:
			parts = append(parts, seg)
		}
	}
	if len(parts) == 0 {
		return "."
	}
	return strings.Join(parts, "/")
}

func parseTFFile(filename string, projectRoot string, project *model.Project) error {
	src, err := os.ReadFile(filename)
	if err != nil {
		return err
	}

	file, diags := hclsyntax.ParseConfig(src, filename, hcl.Pos{Line: 1, Column: 1})
	if diags.HasErrors() {
		return fmt.Errorf("HCL parse error: %s", diags.Error())
	}

	body := file.Body.(*hclsyntax.Body)
	relFile, _ := filepath.Rel(projectRoot, filename)
	if relFile == "" {
		relFile = filepath.Base(filename)
	}

	for _, block := range body.Blocks {
		switch block.Type {
		case "resource":
			if len(block.Labels) >= 2 {
				project.Resources = append(project.Resources, parseResource(block, src, relFile))
			}
		case "data":
			if len(block.Labels) >= 2 {
				project.DataSources = append(project.DataSources, parseDataSource(block, src, relFile))
			}
		case "module":
			if len(block.Labels) >= 1 {
				project.Modules = append(project.Modules, parseModuleCall(block, src, relFile))
			}
		case "variable":
			if len(block.Labels) >= 1 {
				project.Variables = append(project.Variables, parseVariable(block, src, relFile))
			}
		case "output":
			if len(block.Labels) >= 1 {
				project.Outputs = append(project.Outputs, parseOutput(block, src, relFile))
			}
		case "locals":
			project.Locals = append(project.Locals, parseLocals(block, src, relFile)...)
		case "provider":
			if len(block.Labels) >= 1 {
				mergeProvider(project, parseProvider(block, src, relFile))
			}
		case "terraform":
			parseTerraformBlock(block, src, relFile, project)
		}
	}

	return nil
}

func sourceLocation(rng hcl.Range, relFile string) model.SourceLocation {
	return model.SourceLocation{
		File:      relFile,
		Line:      rng.Start.Line,
		EndLine:   rng.End.Line,
		Column:    rng.Start.Column,
		EndColumn: rng.End.Column,
	}
}

func blockRange(block *hclsyntax.Block) hcl.Range {
	return hcl.Range{
		Filename: block.TypeRange.Filename,
		Start:    block.TypeRange.Start,
		End:      block.CloseBraceRange.End,
	}
}

func exprSrc(src []byte, expr hclsyntax.Expression) string {
	rng := expr.Range()
	return string(src[rng.Start.Byte:rng.End.Byte])
}

func evalExpr(src []byte, expr hclsyntax.Expression) interface{} {
	val, diags := expr.Value(nil)
	if !diags.HasErrors() {
		return ctyToGo(val)
	}
	return exprSrc(src, expr)
}

func ctyToGo(val cty.Value) interface{} {
	if !val.IsKnown() || val.IsNull() {
		return nil
	}
	ty := val.Type()
	switch {
	case ty == cty.String:
		return val.AsString()
	case ty == cty.Number:
		bf := val.AsBigFloat()
		if bf.IsInt() {
			i, _ := bf.Int64()
			return i
		}
		f, _ := bf.Float64()
		return f
	case ty == cty.Bool:
		return val.True()
	case ty.IsListType() || ty.IsTupleType() || ty.IsSetType():
		var result []interface{}
		for it := val.ElementIterator(); it.Next(); {
			_, v := it.Element()
			result = append(result, ctyToGo(v))
		}
		return result
	case ty.IsMapType() || ty.IsObjectType():
		result := make(map[string]interface{})
		for it := val.ElementIterator(); it.Next(); {
			k, v := it.Element()
			result[k.AsString()] = ctyToGo(v)
		}
		return result
	default:
		return nil
	}
}

func bodyAttrs(src []byte, body *hclsyntax.Body, skip map[string]bool) map[string]interface{} {
	attrs := make(map[string]interface{})
	for name, attr := range body.Attributes {
		if skip != nil && skip[name] {
			continue
		}
		attrs[name] = evalExpr(src, attr.Expr)
	}
	return attrs
}

func extractDependsOn(src []byte, body *hclsyntax.Body) []string {
	attr, ok := body.Attributes["depends_on"]
	if !ok {
		return nil
	}
	if tuple, ok := attr.Expr.(*hclsyntax.TupleConsExpr); ok {
		var deps []string
		for _, elem := range tuple.Exprs {
			deps = append(deps, exprSrc(src, elem))
		}
		return deps
	}
	return []string{exprSrc(src, attr.Expr)}
}

func collectReferences(body *hclsyntax.Body) []string {
	refs := make(map[string]bool)
	collectRefsFromBody(body, refs)
	result := make([]string, 0, len(refs))
	for ref := range refs {
		result = append(result, ref)
	}
	sort.Strings(result)
	return result
}

func collectRefsFromBody(body *hclsyntax.Body, refs map[string]bool) {
	for _, attr := range body.Attributes {
		for _, traversal := range attr.Expr.Variables() {
			if ref := traversalToRef(traversal); ref != "" {
				refs[ref] = true
			}
		}
	}
	for _, block := range body.Blocks {
		collectRefsFromBody(block.Body, refs)
	}
}

func traversalToRef(t hcl.Traversal) string {
	if len(t) < 2 {
		return ""
	}
	root, ok := t[0].(hcl.TraverseRoot)
	if !ok {
		return ""
	}
	attr1, ok := t[1].(hcl.TraverseAttr)
	if !ok {
		return ""
	}
	switch root.Name {
	case "var":
		return "var." + attr1.Name
	case "local":
		return "local." + attr1.Name
	case "module":
		ref := "module." + attr1.Name
		if len(t) >= 3 {
			if attr2, ok := t[2].(hcl.TraverseAttr); ok {
				ref += "." + attr2.Name
			}
		}
		return ref
	case "data":
		if len(t) >= 3 {
			if attr2, ok := t[2].(hcl.TraverseAttr); ok {
				return "data." + attr1.Name + "." + attr2.Name
			}
		}
		return ""
	default:
		if builtinPrefixes[root.Name] {
			return ""
		}
		return root.Name + "." + attr1.Name
	}
}


func parseResource(block *hclsyntax.Block, src []byte, relFile string) model.Resource {
	skip := map[string]bool{"depends_on": true, "provider": true, "count": true, "for_each": true}

	r := model.Resource{
		Type:        block.Labels[0],
		Name:        block.Labels[1],
		Attributes:  bodyAttrs(src, block.Body, skip),
		Source:      sourceLocation(blockRange(block), relFile),
		StateStatus: model.StateStatusUnknown,
		DependsOn:   extractDependsOn(src, block.Body),
		References:  collectReferences(block.Body),
	}

	if attr, ok := block.Body.Attributes["provider"]; ok {
		r.Provider = exprSrc(src, attr.Expr)
	}
	if attr, ok := block.Body.Attributes["count"]; ok {
		r.Count = evalExpr(src, attr.Expr)
	}
	if attr, ok := block.Body.Attributes["for_each"]; ok {
		r.ForEach = evalExpr(src, attr.Expr)
	}

	return r
}

func parseDataSource(block *hclsyntax.Block, src []byte, relFile string) model.DataSource {
	skip := map[string]bool{"provider": true}

	d := model.DataSource{
		Type:       block.Labels[0],
		Name:       block.Labels[1],
		Attributes: bodyAttrs(src, block.Body, skip),
		Source:     sourceLocation(blockRange(block), relFile),
		References: collectReferences(block.Body),
	}

	if attr, ok := block.Body.Attributes["provider"]; ok {
		d.Provider = exprSrc(src, attr.Expr)
	}

	return d
}

func parseModuleCall(block *hclsyntax.Block, src []byte, relFile string) model.ModuleCall {
	skip := map[string]bool{"source": true, "version": true, "depends_on": true, "providers": true}

	m := model.ModuleCall{
		Name:       block.Labels[0],
		Inputs:     bodyAttrs(src, block.Body, skip),
		Location:   sourceLocation(blockRange(block), relFile),
		DependsOn:  extractDependsOn(src, block.Body),
		References: collectReferences(block.Body),
	}

	if attr, ok := block.Body.Attributes["source"]; ok {
		if v, ok := evalExpr(src, attr.Expr).(string); ok {
			m.Source = v
		}
	}
	if attr, ok := block.Body.Attributes["version"]; ok {
		if v, ok := evalExpr(src, attr.Expr).(string); ok {
			m.Version = v
		}
	}
	if attr, ok := block.Body.Attributes["providers"]; ok {
		if obj, ok := attr.Expr.(*hclsyntax.ObjectConsExpr); ok {
			m.Providers = make(map[string]string)
			for _, item := range obj.Items {
				key := exprSrc(src, item.KeyExpr)
				val := exprSrc(src, item.ValueExpr)
				m.Providers[key] = val
			}
		}
	}

	return m
}

func parseVariable(block *hclsyntax.Block, src []byte, relFile string) model.Variable {
	v := model.Variable{
		Name:   block.Labels[0],
		Source: sourceLocation(blockRange(block), relFile),
	}

	if attr, ok := block.Body.Attributes["type"]; ok {
		v.Type = exprSrc(src, attr.Expr)
	}
	if attr, ok := block.Body.Attributes["description"]; ok {
		if s, ok := evalExpr(src, attr.Expr).(string); ok {
			v.Description = s
		}
	}
	if attr, ok := block.Body.Attributes["default"]; ok {
		v.Default = evalExpr(src, attr.Expr)
	}
	if attr, ok := block.Body.Attributes["sensitive"]; ok {
		if b, ok := evalExpr(src, attr.Expr).(bool); ok {
			v.Sensitive = b
		}
	}

	for _, sub := range block.Body.Blocks {
		if sub.Type == "validation" {
			val := &model.Validation{}
			if attr, ok := sub.Body.Attributes["condition"]; ok {
				val.Condition = exprSrc(src, attr.Expr)
			}
			if attr, ok := sub.Body.Attributes["error_message"]; ok {
				if s, ok := evalExpr(src, attr.Expr).(string); ok {
					val.ErrorMessage = s
				}
			}
			v.Validation = val
		}
	}

	return v
}

func parseOutput(block *hclsyntax.Block, src []byte, relFile string) model.Output {
	o := model.Output{
		Name:       block.Labels[0],
		Source:     sourceLocation(blockRange(block), relFile),
		DependsOn:  extractDependsOn(src, block.Body),
		References: collectReferences(block.Body),
	}

	if attr, ok := block.Body.Attributes["description"]; ok {
		if s, ok := evalExpr(src, attr.Expr).(string); ok {
			o.Description = s
		}
	}
	if attr, ok := block.Body.Attributes["value"]; ok {
		o.Value = evalExpr(src, attr.Expr)
	}
	if attr, ok := block.Body.Attributes["sensitive"]; ok {
		if b, ok := evalExpr(src, attr.Expr).(bool); ok {
			o.Sensitive = b
		}
	}

	return o
}

func parseLocals(block *hclsyntax.Block, src []byte, relFile string) []model.Local {
	var locals []model.Local
	for name, attr := range block.Body.Attributes {
		refs := make(map[string]bool)
		for _, traversal := range attr.Expr.Variables() {
			if ref := traversalToRef(traversal); ref != "" {
				refs[ref] = true
			}
		}
		refList := make([]string, 0, len(refs))
		for ref := range refs {
			refList = append(refList, ref)
		}
		sort.Strings(refList)

		locals = append(locals, model.Local{
			Name:       name,
			Expression: evalExpr(src, attr.Expr),
			Source:     sourceLocation(attr.SrcRange, relFile),
			References: refList,
		})
	}
	return locals
}

func parseProvider(block *hclsyntax.Block, src []byte, relFile string) model.Provider {
	skip := map[string]bool{"alias": true, "version": true}

	p := model.Provider{
		Name:   block.Labels[0],
		Config: bodyAttrs(src, block.Body, skip),
		Source: sourceLocation(blockRange(block), relFile),
	}

	if attr, ok := block.Body.Attributes["alias"]; ok {
		if s, ok := evalExpr(src, attr.Expr).(string); ok {
			p.Alias = s
		}
	}
	if attr, ok := block.Body.Attributes["version"]; ok {
		if s, ok := evalExpr(src, attr.Expr).(string); ok {
			p.Version = s
		}
	}

	return p
}

func mergeProvider(project *model.Project, p model.Provider) {
	for i, existing := range project.Providers {
		if existing.Name == p.Name && existing.Alias == p.Alias {
			if p.Version != "" {
				project.Providers[i].Version = p.Version
			}
			for k, v := range p.Config {
				project.Providers[i].Config[k] = v
			}
			if p.Source.File != "" {
				project.Providers[i].Source = p.Source
			}
			return
		}
	}
	project.Providers = append(project.Providers, p)
}

func parseTerraformBlock(block *hclsyntax.Block, src []byte, relFile string, project *model.Project) {
	for _, sub := range block.Body.Blocks {
		switch sub.Type {
		case "backend":
			if len(sub.Labels) >= 1 {
				b := model.Backend{
					Type:   sub.Labels[0],
					Config: bodyAttrs(src, sub.Body, nil),
					Source: sourceLocation(blockRange(sub), relFile),
				}
				project.Backends = append(project.Backends, b)
				project.Backend = &project.Backends[len(project.Backends)-1]
			}
		case "required_providers":
			for name, attr := range sub.Body.Attributes {
				provVersion := ""
				val, diags := attr.Expr.Value(nil)
				if !diags.HasErrors() {
					switch {
					case val.Type() == cty.String:
						provVersion = val.AsString()
					case val.Type().IsObjectType():
						if val.Type().HasAttribute("version") {
							if v := val.GetAttr("version"); v.IsKnown() && v.Type() == cty.String {
								provVersion = v.AsString()
							}
						}
					}
				}

				found := false
				for i, p := range project.Providers {
					if p.Name == name {
						if provVersion != "" && project.Providers[i].Version == "" {
							project.Providers[i].Version = provVersion
						}
						found = true
						break
					}
				}
				if !found {
					p := model.Provider{
						Name:    name,
						Version: provVersion,
						Config:  map[string]interface{}{},
						Source:  sourceLocation(attr.SrcRange, relFile),
					}
					if !diags.HasErrors() && val.Type().IsObjectType() && val.Type().HasAttribute("source") {
						if v := val.GetAttr("source"); v.IsKnown() && v.Type() == cty.String {
							p.Config["source"] = v.AsString()
						}
					}
					project.Providers = append(project.Providers, p)
				}
			}
		}
	}
}

func parseTFVarsFile(filename string, project *model.Project) error {
	src, err := os.ReadFile(filename)
	if err != nil {
		return err
	}

	file, diags := hclsyntax.ParseConfig(src, filename, hcl.Pos{Line: 1, Column: 1})
	if diags.HasErrors() {
		return fmt.Errorf("HCL parse error: %s", diags.Error())
	}

	body := file.Body.(*hclsyntax.Body)
	for name, attr := range body.Attributes {
		project.TFVars[name] = evalExpr(src, attr.Expr)
	}

	return nil
}
