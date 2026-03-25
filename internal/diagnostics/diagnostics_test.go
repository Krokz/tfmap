package diagnostics

import (
	"fmt"
	"strings"
	"testing"

	"github.com/Krokz/tfmap/internal/model"
)

func filterByRule(diags []model.Diagnostic, rule string) []model.Diagnostic {
	var out []model.Diagnostic
	for _, d := range diags {
		if d.Rule == rule {
			out = append(out, d)
		}
	}
	return out
}

func baseProject() *model.Project {
	return &model.Project{
		Path:    "/test",
		Backend: &model.Backend{Type: "local"},
	}
}

func TestDependencyCycle(t *testing.T) {
	p := baseProject()
	p.Resources = []model.Resource{
		{Type: "null_resource", Name: "a", Attributes: map[string]interface{}{}, References: []string{"null_resource.b"}},
		{Type: "null_resource", Name: "b", Attributes: map[string]interface{}{}, References: []string{"null_resource.c"}},
		{Type: "null_resource", Name: "c", Attributes: map[string]interface{}{}, References: []string{"null_resource.a"}},
	}
	diags := Analyze(p)
	cycles := filterByRule(diags, "dependency-cycle")
	if len(cycles) != 1 {
		t.Fatalf("expected 1 dependency-cycle diagnostic, got %d", len(cycles))
	}
	if cycles[0].Severity != model.DiagError {
		t.Errorf("expected error severity, got %s", cycles[0].Severity)
	}
	for _, node := range []string{"null_resource.a", "null_resource.b", "null_resource.c"} {
		if !strings.Contains(cycles[0].Detail, node) {
			t.Errorf("cycle detail missing %s: %q", node, cycles[0].Detail)
		}
	}
}

func TestNoDependencyCycle(t *testing.T) {
	p := baseProject()
	p.Resources = []model.Resource{
		{Type: "null_resource", Name: "a", Attributes: map[string]interface{}{}, References: []string{"null_resource.b"}},
		{Type: "null_resource", Name: "b", Attributes: map[string]interface{}{}, References: []string{"null_resource.c"}},
		{Type: "null_resource", Name: "c", Attributes: map[string]interface{}{}},
	}
	diags := Analyze(p)
	cycles := filterByRule(diags, "dependency-cycle")
	if len(cycles) != 0 {
		t.Errorf("expected 0 dependency-cycle diagnostics, got %d", len(cycles))
	}
}

func TestVariableNoType(t *testing.T) {
	p := baseProject()
	p.Variables = []model.Variable{
		{Name: "untyped", Description: "has description"},
	}
	p.Resources = []model.Resource{
		{Type: "null_resource", Name: "a", Attributes: map[string]interface{}{}, References: []string{"var.untyped"}},
	}
	diags := Analyze(p)
	found := filterByRule(diags, "variable-no-type")
	if len(found) != 1 {
		t.Fatalf("expected 1 variable-no-type diagnostic, got %d", len(found))
	}
	if found[0].Severity != model.DiagWarning {
		t.Errorf("expected warning, got %s", found[0].Severity)
	}
}

func TestVariableWithType(t *testing.T) {
	p := baseProject()
	p.Variables = []model.Variable{
		{Name: "typed", Type: "string", Description: "has type"},
	}
	p.Resources = []model.Resource{
		{Type: "null_resource", Name: "a", Attributes: map[string]interface{}{}, References: []string{"var.typed"}},
	}
	diags := Analyze(p)
	found := filterByRule(diags, "variable-no-type")
	if len(found) != 0 {
		t.Errorf("expected 0 variable-no-type diagnostics, got %d", len(found))
	}
}

func TestVariableNoDescription(t *testing.T) {
	p := baseProject()
	p.Variables = []model.Variable{
		{Name: "nodesc", Type: "string"},
	}
	p.Resources = []model.Resource{
		{Type: "null_resource", Name: "a", Attributes: map[string]interface{}{}, References: []string{"var.nodesc"}},
	}
	diags := Analyze(p)
	found := filterByRule(diags, "variable-no-description")
	if len(found) != 1 {
		t.Fatalf("expected 1 variable-no-description diagnostic, got %d", len(found))
	}
	if found[0].Severity != model.DiagWarning {
		t.Errorf("expected warning, got %s", found[0].Severity)
	}
}

func TestOutputNoDescription(t *testing.T) {
	p := baseProject()
	p.Outputs = []model.Output{
		{Name: "out1"},
	}
	diags := Analyze(p)
	found := filterByRule(diags, "output-no-description")
	if len(found) != 1 {
		t.Fatalf("expected 1 output-no-description diagnostic, got %d", len(found))
	}
	if found[0].Severity != model.DiagWarning {
		t.Errorf("expected warning, got %s", found[0].Severity)
	}
}

func TestResourceNoTags(t *testing.T) {
	p := baseProject()
	p.Resources = []model.Resource{
		{Type: "aws_s3_bucket", Name: "mybucket", Attributes: map[string]interface{}{"bucket": "test"}},
		{Type: "null_resource", Name: "helper", Attributes: map[string]interface{}{}, References: []string{"aws_s3_bucket.mybucket"}},
	}
	diags := Analyze(p)
	found := filterByRule(diags, "resource-no-tags")
	if len(found) != 1 {
		t.Fatalf("expected 1 resource-no-tags diagnostic (only for aws_s3_bucket), got %d", len(found))
	}
	if found[0].Entity != "aws_s3_bucket.mybucket" {
		t.Errorf("expected entity aws_s3_bucket.mybucket, got %s", found[0].Entity)
	}
	if found[0].Severity != model.DiagWarning {
		t.Errorf("expected warning, got %s", found[0].Severity)
	}
}

func TestResourceWithTags(t *testing.T) {
	p := baseProject()
	p.Resources = []model.Resource{
		{Type: "aws_instance", Name: "web", Attributes: map[string]interface{}{
			"tags": map[string]interface{}{"Name": "web"},
		}},
	}
	diags := Analyze(p)
	found := filterByRule(diags, "resource-no-tags")
	if len(found) != 0 {
		t.Errorf("expected 0 resource-no-tags diagnostics, got %d", len(found))
	}
}

func TestNoBackend(t *testing.T) {
	p := &model.Project{Path: "/test"}
	diags := Analyze(p)
	found := filterByRule(diags, "no-backend")
	if len(found) != 1 {
		t.Fatalf("expected 1 no-backend diagnostic, got %d", len(found))
	}
	if found[0].Severity != model.DiagWarning {
		t.Errorf("expected warning, got %s", found[0].Severity)
	}
}

func TestWithBackend(t *testing.T) {
	p := baseProject()
	diags := Analyze(p)
	found := filterByRule(diags, "no-backend")
	if len(found) != 0 {
		t.Errorf("expected 0 no-backend diagnostics, got %d", len(found))
	}
}

func TestProviderNoVersion(t *testing.T) {
	p := baseProject()
	p.Providers = []model.Provider{
		{Name: "aws", Source: model.SourceLocation{File: "main.tf", Line: 1}},
	}
	diags := Analyze(p)
	found := filterByRule(diags, "provider-no-version")
	if len(found) != 1 {
		t.Fatalf("expected 1 provider-no-version diagnostic, got %d", len(found))
	}
	if found[0].Severity != model.DiagWarning {
		t.Errorf("expected warning, got %s", found[0].Severity)
	}
}

func TestUnusedVariable(t *testing.T) {
	p := baseProject()
	p.Variables = []model.Variable{
		{Name: "unused", Type: "string", Description: "not used anywhere"},
		{Name: "used", Type: "string", Description: "used in resource"},
	}
	p.Resources = []model.Resource{
		{Type: "null_resource", Name: "a", Attributes: map[string]interface{}{}, References: []string{"var.used"}},
	}
	diags := Analyze(p)
	found := filterByRule(diags, "unused-variable")
	if len(found) != 1 {
		t.Fatalf("expected 1 unused-variable diagnostic, got %d", len(found))
	}
	if found[0].Entity != "var.unused" {
		t.Errorf("expected entity var.unused, got %s", found[0].Entity)
	}
	if found[0].Severity != model.DiagWarning {
		t.Errorf("expected warning, got %s", found[0].Severity)
	}
}

func TestUnusedVariableInProviderConfig(t *testing.T) {
	p := baseProject()
	p.Variables = []model.Variable{
		{Name: "myvar", Type: "string", Description: "used in provider config"},
	}
	p.Providers = []model.Provider{
		{
			Name:    "aws",
			Version: "5.0",
			Config:  map[string]interface{}{"region": "var.myvar"},
			Source:  model.SourceLocation{File: "main.tf", Line: 1},
		},
	}
	diags := Analyze(p)
	found := filterByRule(diags, "unused-variable")
	if len(found) != 0 {
		t.Errorf("expected 0 unused-variable diagnostics for provider-config ref, got %d: %+v", len(found), found)
	}
}

func TestOrphanedStateResource(t *testing.T) {
	p := baseProject()
	p.State = &model.StateSnapshot{
		Resources: []model.StateResource{
			{Type: "aws_instance", Name: "ghost", Mode: "managed"},
		},
	}
	diags := Analyze(p)
	found := filterByRule(diags, "orphaned-state-resource")
	if len(found) != 1 {
		t.Fatalf("expected 1 orphaned-state-resource diagnostic, got %d", len(found))
	}
	if found[0].Entity != "aws_instance.ghost" {
		t.Errorf("expected entity aws_instance.ghost, got %s", found[0].Entity)
	}
	if found[0].Severity != model.DiagWarning {
		t.Errorf("expected warning, got %s", found[0].Severity)
	}

	p2 := baseProject()
	diags2 := Analyze(p2)
	found2 := filterByRule(diags2, "orphaned-state-resource")
	if len(found2) != 0 {
		t.Errorf("expected 0 orphaned-state-resource with nil state, got %d", len(found2))
	}
}

func TestSensitiveNaming(t *testing.T) {
	p := baseProject()
	p.Variables = []model.Variable{
		{Name: "db_password", Type: "string", Description: "the password", Sensitive: false},
	}
	p.Resources = []model.Resource{
		{Type: "null_resource", Name: "a", Attributes: map[string]interface{}{}, References: []string{"var.db_password"}},
	}
	diags := Analyze(p)
	found := filterByRule(diags, "sensitive-naming")
	if len(found) != 1 {
		t.Fatalf("expected 1 sensitive-naming diagnostic, got %d", len(found))
	}
	if found[0].Severity != model.DiagInfo {
		t.Errorf("expected info severity, got %s", found[0].Severity)
	}

	p2 := baseProject()
	p2.Variables = []model.Variable{
		{Name: "db_password", Type: "string", Description: "the password", Sensitive: true},
	}
	p2.Resources = []model.Resource{
		{Type: "null_resource", Name: "a", Attributes: map[string]interface{}{}, References: []string{"var.db_password"}},
	}
	diags2 := Analyze(p2)
	found2 := filterByRule(diags2, "sensitive-naming")
	if len(found2) != 0 {
		t.Errorf("expected 0 sensitive-naming diagnostics with Sensitive=true, got %d", len(found2))
	}
}

func TestHighBlastRadius(t *testing.T) {
	p := baseProject()
	p.Resources = []model.Resource{
		{Type: "null_resource", Name: "target", Attributes: map[string]interface{}{}},
	}
	for i := 0; i < 6; i++ {
		p.Resources = append(p.Resources, model.Resource{
			Type:       "null_resource",
			Name:       fmt.Sprintf("r%d", i),
			Attributes: map[string]interface{}{},
			References: []string{"null_resource.target"},
		})
	}
	diags := Analyze(p)
	found := filterByRule(diags, "high-blast-radius")
	if len(found) != 1 {
		t.Fatalf("expected 1 high-blast-radius diagnostic, got %d", len(found))
	}
	if found[0].Entity != "null_resource.target" {
		t.Errorf("expected entity null_resource.target, got %s", found[0].Entity)
	}
	if found[0].Severity != model.DiagInfo {
		t.Errorf("expected info severity, got %s", found[0].Severity)
	}

	p2 := baseProject()
	p2.Resources = []model.Resource{
		{Type: "null_resource", Name: "target", Attributes: map[string]interface{}{}},
	}
	for i := 0; i < 3; i++ {
		p2.Resources = append(p2.Resources, model.Resource{
			Type:       "null_resource",
			Name:       fmt.Sprintf("r%d", i),
			Attributes: map[string]interface{}{},
			References: []string{"null_resource.target"},
		})
	}
	diags2 := Analyze(p2)
	found2 := filterByRule(diags2, "high-blast-radius")
	if len(found2) != 0 {
		t.Errorf("expected 0 high-blast-radius diagnostics with 3 refs, got %d", len(found2))
	}
}

func TestDiagnosticsSorting(t *testing.T) {
	p := &model.Project{
		Path:    "/test",
		Backend: &model.Backend{Type: "local"},
		Variables: []model.Variable{
			{Name: "untyped", Description: "no type set"},
			{Name: "db_password", Type: "string", Description: "pw"},
		},
		Resources: []model.Resource{
			{Type: "null_resource", Name: "a", Attributes: map[string]interface{}{}, References: []string{"null_resource.b", "var.untyped", "var.db_password"}},
			{Type: "null_resource", Name: "b", Attributes: map[string]interface{}{}, References: []string{"null_resource.a"}},
		},
	}
	diags := Analyze(p)

	hasError, hasWarning, hasInfo := false, false, false
	for _, d := range diags {
		switch d.Severity {
		case model.DiagError:
			hasError = true
		case model.DiagWarning:
			hasWarning = true
		case model.DiagInfo:
			hasInfo = true
		}
	}
	if !hasError || !hasWarning || !hasInfo {
		t.Fatalf("expected at least one error, warning, and info; error=%v warning=%v info=%v\ndiags: %+v", hasError, hasWarning, hasInfo, diags)
	}

	for i := 1; i < len(diags); i++ {
		prev := severityOrder(diags[i-1].Severity)
		curr := severityOrder(diags[i].Severity)
		if curr < prev {
			t.Errorf("diagnostics not sorted at index %d: %s (%d) followed by %s (%d)",
				i, diags[i-1].Severity, prev, diags[i].Severity, curr)
		}
	}
}

func TestUnreferencedEntityFilteredOut(t *testing.T) {
	p := baseProject()
	p.Resources = []model.Resource{
		{Type: "aws_s3_bucket", Name: "orphan", Attributes: map[string]interface{}{"bucket": "test"}},
	}
	diags := Analyze(p)
	found := filterByRule(diags, "resource-no-tags")
	if len(found) != 0 {
		t.Errorf("expected 0 resource-no-tags for unreferenced resource, got %d", len(found))
	}
}

func TestUnreferencedVarAndDataStillDiagnosed(t *testing.T) {
	p := baseProject()
	p.Variables = []model.Variable{
		{Name: "orphan_var", Source: model.SourceLocation{File: "vars.tf", Line: 1}},
	}
	p.DataSources = []model.DataSource{
		{Type: "aws_caller_identity", Name: "orphan_data", Source: model.SourceLocation{File: "data.tf", Line: 1}},
	}
	p.Outputs = []model.Output{
		{Name: "orphan_out", Source: model.SourceLocation{File: "outputs.tf", Line: 1}},
	}
	diags := Analyze(p)

	varDiags := filterByRule(diags, "variable-no-type")
	if len(varDiags) == 0 {
		t.Error("expected variable-no-type diagnostic for unreferenced variable, got none")
	}
	outDiags := filterByRule(diags, "output-no-description")
	if len(outDiags) == 0 {
		t.Error("expected output-no-description diagnostic for unreferenced output, got none")
	}
}

func TestCycleEdgesCorrectlyReconstructed(t *testing.T) {
	p := baseProject()
	p.Modules = []model.ModuleCall{
		{Name: "a", Inputs: map[string]interface{}{}, References: []string{"module.b"}},
		{Name: "b", Inputs: map[string]interface{}{}, References: []string{"module.a"}},
	}
	diags := Analyze(p)
	cycles := filterByRule(diags, "dependency-cycle")
	if len(cycles) != 1 {
		t.Fatalf("expected 1 cycle diagnostic, got %d", len(cycles))
	}
	edges := cycles[0].CycleEdges
	if len(edges) != 2 {
		t.Fatalf("expected 2 cycle edges, got %d", len(edges))
	}
	for _, e := range edges {
		if e.From == e.To {
			t.Errorf("cycle edge has self-reference: %s -> %s", e.From, e.To)
		}
	}
	fromSet := map[string]bool{edges[0].From: true, edges[1].From: true}
	toSet := map[string]bool{edges[0].To: true, edges[1].To: true}
	if !fromSet["module.a"] || !fromSet["module.b"] {
		t.Errorf("expected both module.a and module.b as edge sources, got %s and %s", edges[0].From, edges[1].From)
	}
	if !toSet["module.a"] || !toSet["module.b"] {
		t.Errorf("expected both module.a and module.b as edge targets, got %s and %s", edges[0].To, edges[1].To)
	}
}

func TestStripHCLComment(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		{"  vpc_id = module.vpc.vpc_id", "  vpc_id = module.vpc.vpc_id"},
		{"  udi_queue_arn = {} # module.glue.udi_queue_arn", "  udi_queue_arn = {} "},
		{"  # module.glue.arn - disabled", "  "},
		{`  name = "test#value"`, `  name = "test#value"`},
		{"  foo = bar // comment", "  foo = bar "},
	}
	for _, tt := range tests {
		got := stripHCLComment(tt.input)
		if got != tt.want {
			t.Errorf("stripHCLComment(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestNoIssues(t *testing.T) {
	p := &model.Project{
		Path:    "/clean",
		Backend: &model.Backend{Type: "s3"},
		Providers: []model.Provider{
			{Name: "aws", Version: "~> 5.0", Source: model.SourceLocation{File: "main.tf", Line: 1}},
		},
		Variables: []model.Variable{
			{Name: "region", Type: "string", Description: "AWS region"},
		},
		Resources: []model.Resource{
			{
				Type:       "aws_instance",
				Name:       "web",
				Attributes: map[string]interface{}{"tags": map[string]interface{}{"Name": "web"}},
				References: []string{"var.region"},
			},
		},
		Outputs: []model.Output{
			{Name: "instance_id", Description: "The instance ID", References: []string{"aws_instance.web.id"}},
		},
	}
	diags := Analyze(p)
	if len(diags) != 0 {
		t.Errorf("expected 0 diagnostics for well-formed project, got %d:", len(diags))
		for _, d := range diags {
			t.Errorf("  [%s] %s: %s (entity=%s)", d.Severity, d.Rule, d.Message, d.Entity)
		}
	}
}
