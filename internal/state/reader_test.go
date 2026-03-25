package state

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/Krokz/tfmap/internal/model"
)

func makeSnapshot() model.StateSnapshot {
	return model.StateSnapshot{
		Serial:  3,
		Version: 4,
		Lineage: "abc-123",
		Resources: []model.StateResource{
			{
				Mode:     "managed",
				Type:     "aws_instance",
				Name:     "web",
				Provider: "provider[\"registry.terraform.io/hashicorp/aws\"]",
				Instances: []model.StateResourceInstance{
					{Attributes: map[string]interface{}{"id": "i-abc123", "ami": "ami-xyz"}},
				},
			},
			{
				Mode:     "managed",
				Type:     "aws_s3_bucket",
				Name:     "data",
				Provider: "provider[\"registry.terraform.io/hashicorp/aws\"]",
				Instances: []model.StateResourceInstance{
					{Attributes: map[string]interface{}{"id": "my-bucket"}},
				},
			},
		},
		Outputs: map[string]model.StateOutput{
			"ip": {Value: "10.0.0.1", Type: "string"},
		},
	}
}

func writeTFState(t *testing.T, dir string, snap model.StateSnapshot) {
	t.Helper()
	data, err := json.Marshal(snap)
	if err != nil {
		t.Fatalf("marshalling state: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "terraform.tfstate"), data, 0644); err != nil {
		t.Fatalf("writing state file: %v", err)
	}
}

func TestReadLocal(t *testing.T) {
	dir := t.TempDir()
	want := makeSnapshot()
	writeTFState(t, dir, want)

	r := NewReader()
	got, err := r.ReadLocal(dir)
	if err != nil {
		t.Fatalf("ReadLocal returned error: %v", err)
	}

	if got.Serial != want.Serial {
		t.Errorf("Serial = %d, want %d", got.Serial, want.Serial)
	}
	if got.Version != want.Version {
		t.Errorf("Version = %d, want %d", got.Version, want.Version)
	}
	if got.Lineage != want.Lineage {
		t.Errorf("Lineage = %q, want %q", got.Lineage, want.Lineage)
	}
	if len(got.Resources) != len(want.Resources) {
		t.Fatalf("Resources count = %d, want %d", len(got.Resources), len(want.Resources))
	}
	for i, r := range got.Resources {
		if r.Type != want.Resources[i].Type || r.Name != want.Resources[i].Name {
			t.Errorf("Resource[%d] = %s.%s, want %s.%s", i, r.Type, r.Name, want.Resources[i].Type, want.Resources[i].Name)
		}
	}
	if len(got.Outputs) != len(want.Outputs) {
		t.Errorf("Outputs count = %d, want %d", len(got.Outputs), len(want.Outputs))
	}
}

func TestReadLocalMissing(t *testing.T) {
	dir := t.TempDir()
	r := NewReader()
	snap, err := r.ReadLocal(dir)
	if err == nil {
		t.Fatal("expected error for missing terraform.tfstate")
	}
	if snap != nil {
		t.Error("expected nil snapshot")
	}
}

func TestReadLocalInvalidJSON(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "terraform.tfstate"), []byte("{not valid json!!!"), 0644); err != nil {
		t.Fatal(err)
	}

	r := NewReader()
	snap, err := r.ReadLocal(dir)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
	if snap != nil {
		t.Error("expected nil snapshot")
	}
}

func TestReadDispatchLocal(t *testing.T) {
	dir := t.TempDir()
	want := makeSnapshot()
	writeTFState(t, dir, want)

	r := NewReader()
	backend := &model.Backend{Type: "local"}

	got, err := r.Read(backend, dir, "")
	if err != nil {
		t.Fatalf("Read(local) returned error: %v", err)
	}
	if got.Serial != want.Serial {
		t.Errorf("Serial = %d, want %d", got.Serial, want.Serial)
	}
	if len(got.Resources) != len(want.Resources) {
		t.Errorf("Resources count = %d, want %d", len(got.Resources), len(want.Resources))
	}
}

func TestReadDispatchS3(t *testing.T) {
	r := NewReader()
	backend := &model.Backend{
		Type:   "s3",
		Config: map[string]interface{}{},
	}
	snap, err := r.Read(backend, "", "")
	if snap != nil {
		t.Error("expected nil snapshot for S3 without bucket config")
	}
	if err == nil {
		t.Error("expected error for S3 without bucket config")
	}
}

func TestReadDispatchAzure(t *testing.T) {
	r := NewReader()
	backend := &model.Backend{
		Type:   "azurerm",
		Config: map[string]interface{}{},
	}
	snap, err := r.Read(backend, "", "")
	if snap != nil {
		t.Error("expected nil snapshot for Azure without storage_account_name config")
	}
	if err == nil {
		t.Error("expected error for Azure without storage_account_name config")
	}
}

func TestReadDispatchAzureMissingContainer(t *testing.T) {
	r := NewReader()
	backend := &model.Backend{
		Type: "azurerm",
		Config: map[string]interface{}{
			"storage_account_name": "myaccount",
		},
	}
	snap, err := r.Read(backend, "", "")
	if snap != nil {
		t.Error("expected nil snapshot for Azure without container_name config")
	}
	if err == nil {
		t.Error("expected error for Azure without container_name config")
	}
}

func TestReadDispatchGCS(t *testing.T) {
	r := NewReader()
	backend := &model.Backend{
		Type:   "gcs",
		Config: map[string]interface{}{},
	}
	snap, err := r.Read(backend, "", "")
	if snap != nil {
		t.Error("expected nil snapshot for GCS without bucket config")
	}
	if err == nil {
		t.Error("expected error for GCS without bucket config")
	}
}

func TestReadDispatchUnknown(t *testing.T) {
	r := NewReader()
	backend := &model.Backend{Type: "consul"}

	snap, err := r.Read(backend, "", "")
	if snap != nil {
		t.Error("expected nil snapshot for unknown backend")
	}
	if err != nil {
		t.Errorf("expected nil error for unknown backend, got: %v", err)
	}
}

func TestCompareWithState(t *testing.T) {
	project := &model.Project{
		Path: "/test",
		Resources: []model.Resource{
			{Type: "aws_instance", Name: "web"},
			{Type: "aws_s3_bucket", Name: "data"},
			{Type: "aws_lambda_function", Name: "handler"},
		},
		State: &model.StateSnapshot{
			Resources: []model.StateResource{
				{
					Mode: "managed",
					Type: "aws_instance",
					Name: "web",
					Instances: []model.StateResourceInstance{
						{Attributes: map[string]interface{}{"id": "i-abc"}},
					},
				},
				{
					Mode: "managed",
					Type: "aws_s3_bucket",
					Name: "data",
					Instances: []model.StateResourceInstance{
						{Attributes: map[string]interface{}{"id": "my-bucket"}},
					},
				},
			},
		},
	}

	CompareWithState(project)

	if project.Resources[0].StateStatus != model.StateStatusInSync {
		t.Errorf("Resources[0].StateStatus = %q, want %q", project.Resources[0].StateStatus, model.StateStatusInSync)
	}
	if project.Resources[0].StateAttrs["id"] != "i-abc" {
		t.Errorf("Resources[0].StateAttrs[id] = %v, want i-abc", project.Resources[0].StateAttrs["id"])
	}
	if project.Resources[1].StateStatus != model.StateStatusInSync {
		t.Errorf("Resources[1].StateStatus = %q, want %q", project.Resources[1].StateStatus, model.StateStatusInSync)
	}
	if project.Resources[1].StateAttrs["id"] != "my-bucket" {
		t.Errorf("Resources[1].StateAttrs[id] = %v, want my-bucket", project.Resources[1].StateAttrs["id"])
	}
	if project.Resources[2].StateStatus != model.StateStatusNotInState {
		t.Errorf("Resources[2].StateStatus = %q, want %q", project.Resources[2].StateStatus, model.StateStatusNotInState)
	}
}

func TestCompareWithStateNilState(t *testing.T) {
	project := &model.Project{
		Path: "/test",
		Resources: []model.Resource{
			{Type: "aws_instance", Name: "web"},
		},
		State: nil,
	}

	CompareWithState(project)

	if project.Resources[0].StateStatus != "" {
		t.Errorf("StateStatus should be unchanged (empty), got %q", project.Resources[0].StateStatus)
	}
}

func TestCompareWithStateModuleResources(t *testing.T) {
	project := &model.Project{
		Path: "/test",
		Resources: []model.Resource{
			{Type: "aws_instance", Name: "web"},
		},
		State: &model.StateSnapshot{
			Resources: []model.StateResource{
				{
					Module: "module.vpc",
					Mode:   "managed",
					Type:   "aws_instance",
					Name:   "web",
					Instances: []model.StateResourceInstance{
						{Attributes: map[string]interface{}{"id": "i-mod"}},
					},
				},
			},
		},
	}

	CompareWithState(project)

	if project.Resources[0].StateStatus != model.StateStatusNotInState {
		t.Errorf("StateStatus = %q, want %q (module resource should not match root)", project.Resources[0].StateStatus, model.StateStatusNotInState)
	}
}

func TestCompareWithStateDrift(t *testing.T) {
	project := &model.Project{
		Path: "/test",
		Resources: []model.Resource{
			{
				Type: "aws_instance", Name: "web",
				Attributes: map[string]interface{}{"instance_type": "t2.micro"},
				Source:     model.SourceLocation{File: "/test/main.tf"},
			},
		},
		State: &model.StateSnapshot{
			Resources: []model.StateResource{
				{
					Mode: "managed", Type: "aws_instance", Name: "web",
					Instances: []model.StateResourceInstance{
						{Attributes: map[string]interface{}{"instance_type": "t3.large", "id": "i-abc"}},
					},
				},
			},
		},
	}

	CompareWithState(project)

	if project.Resources[0].StateStatus != model.StateStatusDrifted {
		t.Errorf("StateStatus = %q, want %q", project.Resources[0].StateStatus, model.StateStatusDrifted)
	}
}

func TestCompareWithStateNoDriftForExpressions(t *testing.T) {
	project := &model.Project{
		Path: "/test",
		Resources: []model.Resource{
			{
				Type: "aws_instance", Name: "web",
				Attributes: map[string]interface{}{"ami": "var.ami_id", "instance_type": "t2.micro"},
				Source:     model.SourceLocation{File: "/test/main.tf"},
			},
		},
		State: &model.StateSnapshot{
			Resources: []model.StateResource{
				{
					Mode: "managed", Type: "aws_instance", Name: "web",
					Instances: []model.StateResourceInstance{
						{Attributes: map[string]interface{}{"ami": "ami-abc123", "instance_type": "t2.micro"}},
					},
				},
			},
		},
	}

	CompareWithState(project)

	if project.Resources[0].StateStatus != model.StateStatusInSync {
		t.Errorf("StateStatus = %q, want %q (expression attrs should be skipped)", project.Resources[0].StateStatus, model.StateStatusInSync)
	}
}

func TestCompareWithStateOrphans(t *testing.T) {
	project := &model.Project{
		Path: "/test",
		Resources: []model.Resource{
			{Type: "aws_instance", Name: "web", Source: model.SourceLocation{File: "/test/main.tf"}},
		},
		State: &model.StateSnapshot{
			Resources: []model.StateResource{
				{
					Mode: "managed", Type: "aws_instance", Name: "web",
					Instances: []model.StateResourceInstance{
						{Attributes: map[string]interface{}{"id": "i-abc"}},
					},
				},
				{
					Mode: "managed", Type: "aws_s3_bucket", Name: "old_bucket",
					Provider: "provider[\"registry.terraform.io/hashicorp/aws\"]",
					Instances: []model.StateResourceInstance{
						{Attributes: map[string]interface{}{"id": "old-bucket"}},
					},
				},
			},
		},
	}

	CompareWithState(project)

	if len(project.OrphanedResources) != 1 {
		t.Fatalf("OrphanedResources count = %d, want 1", len(project.OrphanedResources))
	}
	o := project.OrphanedResources[0]
	if o.Type != "aws_s3_bucket" || o.Name != "old_bucket" {
		t.Errorf("Orphan = %s.%s, want aws_s3_bucket.old_bucket", o.Type, o.Name)
	}
	if o.Attributes["id"] != "old-bucket" {
		t.Errorf("Orphan attributes[id] = %v, want old-bucket", o.Attributes["id"])
	}
}

func TestCompareWithStateModuleMatch(t *testing.T) {
	projectPath := "/test"
	project := &model.Project{
		Path: projectPath,
		Modules: []model.ModuleCall{
			{Name: "vpc", Source: "./modules/vpc"},
		},
		Resources: []model.Resource{
			{
				Type:   "aws_subnet", Name: "main",
				Source: model.SourceLocation{File: filepath.Join(projectPath, "modules", "vpc", "main.tf")},
			},
		},
		State: &model.StateSnapshot{
			Resources: []model.StateResource{
				{
					Module: "module.vpc", Mode: "managed",
					Type: "aws_subnet", Name: "main",
					Instances: []model.StateResourceInstance{
						{Attributes: map[string]interface{}{"id": "subnet-abc"}},
					},
				},
			},
		},
	}

	CompareWithState(project)

	if project.Resources[0].StateStatus != model.StateStatusInSync {
		t.Errorf("StateStatus = %q, want %q (module resource should match via module prefix)", project.Resources[0].StateStatus, model.StateStatusInSync)
	}
	if project.Resources[0].StateAttrs["id"] != "subnet-abc" {
		t.Errorf("StateAttrs[id] = %v, want subnet-abc", project.Resources[0].StateAttrs["id"])
	}
	if len(project.OrphanedResources) != 0 {
		t.Errorf("OrphanedResources count = %d, want 0", len(project.OrphanedResources))
	}
}

func TestCompareWithStateSharedModule(t *testing.T) {
	projectPath := "/project"
	project := &model.Project{
		Path: projectPath,
		Modules: []model.ModuleCall{
			{
				Name:     "vpc",
				Source:   "../../../modules/vpc",
				Location: model.SourceLocation{File: "envs/prod/emea/main.tf"},
			},
		},
		Resources: []model.Resource{
			{
				Type:   "aws_vpc", Name: "main",
				Source: model.SourceLocation{File: "modules/vpc/main.tf"},
			},
			{
				Type:   "null_resource", Name: "trigger",
				Source: model.SourceLocation{File: "envs/prod/emea/main.tf"},
			},
		},
		ModuleStates: map[string]*model.StateSnapshot{
			"envs/prod/emea": {
				Resources: []model.StateResource{
					{
						Module: "module.vpc", Mode: "managed",
						Type: "aws_vpc", Name: "main",
						Instances: []model.StateResourceInstance{
							{Attributes: map[string]interface{}{"id": "vpc-123"}},
						},
					},
					{
						Mode: "managed",
						Type: "null_resource", Name: "trigger",
						Instances: []model.StateResourceInstance{
							{Attributes: map[string]interface{}{"id": "abc"}},
						},
					},
				},
			},
		},
		DiscoveredModules: []model.TerraformModule{
			{Path: "envs/prod/emea", IsRoot: true, HasBackend: true, Backend: &model.Backend{Type: "s3"}},
		},
	}

	CompareWithState(project)

	if project.Resources[0].StateStatus != model.StateStatusInSync {
		t.Errorf("shared module resource: StateStatus = %q, want %q", project.Resources[0].StateStatus, model.StateStatusInSync)
	}
	if project.Resources[0].StateAttrs["id"] != "vpc-123" {
		t.Errorf("shared module resource: StateAttrs[id] = %v, want vpc-123", project.Resources[0].StateAttrs["id"])
	}
	if project.Resources[1].StateStatus != model.StateStatusInSync {
		t.Errorf("root resource: StateStatus = %q, want %q", project.Resources[1].StateStatus, model.StateStatusInSync)
	}
	if len(project.OrphanedResources) != 0 {
		t.Errorf("OrphanedResources count = %d, want 0", len(project.OrphanedResources))
	}
}

func TestStripModuleIndices(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		{"", ""},
		{"module.vpc", "module.vpc"},
		{"module.foo[0]", "module.foo"},
		{`module.foo["key"]`, "module.foo"},
		{"module.foo[0].module.bar", "module.foo.module.bar"},
		{`module.foo["k1"].module.bar["k2"]`, "module.foo.module.bar"},
		{`module.svc[0].module.inner["x"].module.deep[1]`, "module.svc.module.inner.module.deep"},
	}
	for _, tt := range tests {
		got := stripModuleIndices(tt.input)
		if got != tt.want {
			t.Errorf("stripModuleIndices(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestTopLevelModule(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		{"", ""},
		{"module.vpc", "module.vpc"},
		{"module.vpc.module.sub", "module.vpc"},
		{`module.foo[0]`, "module.foo"},
		{`module.foo["key"].module.bar`, "module.foo"},
	}
	for _, tt := range tests {
		got := topLevelModule(tt.input)
		if got != tt.want {
			t.Errorf("topLevelModule(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestIsLiteralValue(t *testing.T) {
	literals := []interface{}{
		"t2.micro",
		"us-east-1",
		"my-bucket-name",
		"true",
		float64(443),
		true,
		false,
	}
	for _, v := range literals {
		if !isLiteralValue(v) {
			t.Errorf("isLiteralValue(%v) = false, want true", v)
		}
	}

	nonLiterals := []interface{}{
		"var.instance_type",
		"module.vpc.vpc_id",
		"local.common_tags",
		"data.aws_ami.latest.id",
		"each.value",
		"self.id",
		"terraform.workspace",
		"aws_lb.main.arn",
		"azurerm_resource_group.rg.name",
		"google_compute_instance.vm.id",
		"null_resource.trigger.id",
		"random_id.suffix.hex",
		"tls_private_key.ssh.public_key_openssh",
		"time_rotating.week.id",
		"merge({ Env = var.env }, local.tags)",
		`format("prefix-%s", var.name)`,
		"[aws_security_group.sg.id]",
		`{"key": "value"}`,
		"${var.name}-suffix",
		"line1\nline2",
		nil,
		[]string{"a"},
		map[string]string{"k": "v"},
	}
	for _, v := range nonLiterals {
		if isLiteralValue(v) {
			t.Errorf("isLiteralValue(%v) = true, want false", v)
		}
	}
}

func TestCompareWithStateCountModule(t *testing.T) {
	projectPath := "/test"
	project := &model.Project{
		Path: projectPath,
		Modules: []model.ModuleCall{
			{
				Name:     "svc",
				Source:   "./modules/svc",
				Location: model.SourceLocation{File: filepath.Join(projectPath, "main.tf")},
			},
		},
		Resources: []model.Resource{
			{
				Type:   "aws_ecs_service", Name: "main",
				Source: model.SourceLocation{File: filepath.Join(projectPath, "modules", "svc", "main.tf")},
			},
		},
		State: &model.StateSnapshot{
			Resources: []model.StateResource{
				{
					Module: "module.svc[0]", Mode: "managed",
					Type: "aws_ecs_service", Name: "main",
					Instances: []model.StateResourceInstance{
						{Attributes: map[string]interface{}{"id": "svc-abc"}},
					},
				},
			},
		},
	}

	CompareWithState(project)

	if project.Resources[0].StateStatus != model.StateStatusInSync {
		t.Errorf("StateStatus = %q, want %q (count module should match after stripping index)", project.Resources[0].StateStatus, model.StateStatusInSync)
	}
	if len(project.OrphanedResources) != 0 {
		t.Errorf("OrphanedResources = %d, want 0", len(project.OrphanedResources))
	}
}

func TestCompareWithStateRegistryModuleOrphans(t *testing.T) {
	project := &model.Project{
		Path: "/test",
		Modules: []model.ModuleCall{
			{Name: "vpc", Source: "terraform-aws-modules/vpc/aws", Location: model.SourceLocation{File: "/test/main.tf"}},
			{Name: "s3", Source: "./modules/s3", Location: model.SourceLocation{File: "/test/main.tf"}},
		},
		Resources: []model.Resource{},
		State: &model.StateSnapshot{
			Resources: []model.StateResource{
				{
					Module: "module.vpc", Mode: "managed",
					Type: "aws_vpc", Name: "this",
					Provider: "provider[\"registry.terraform.io/hashicorp/aws\"]",
					Instances: []model.StateResourceInstance{
						{Attributes: map[string]interface{}{"id": "vpc-123"}},
					},
				},
				{
					Module: "module.vpc", Mode: "managed",
					Type: "aws_subnet", Name: "private",
					Provider: "provider[\"registry.terraform.io/hashicorp/aws\"]",
					Instances: []model.StateResourceInstance{
						{Attributes: map[string]interface{}{"id": "subnet-456"}},
					},
				},
				{
					Module: "module.s3", Mode: "managed",
					Type: "aws_s3_bucket", Name: "real_orphan",
					Provider: "provider[\"registry.terraform.io/hashicorp/aws\"]",
					Instances: []model.StateResourceInstance{
						{Attributes: map[string]interface{}{"id": "orphan-bucket"}},
					},
				},
			},
		},
	}

	CompareWithState(project)

	if len(project.OrphanedResources) != 1 {
		t.Fatalf("OrphanedResources = %d, want 1 (registry module resources should be excluded)", len(project.OrphanedResources))
	}
	o := project.OrphanedResources[0]
	if o.Type != "aws_s3_bucket" || o.Name != "real_orphan" {
		t.Errorf("Orphan = %s.%s, want aws_s3_bucket.real_orphan", o.Type, o.Name)
	}
}

func TestCompareWithStateNoDriftForNewExpressionPatterns(t *testing.T) {
	project := &model.Project{
		Path: "/test",
		Resources: []model.Resource{
			{
				Type: "aws_ssm_parameter", Name: "key",
				Attributes: map[string]interface{}{
					"name":        `"/app/${var.env}/key"`,
					"tags":        "merge({ Env = var.env }, local.tags)",
					"value":       "tls_private_key.ssh.private_key_pem",
					"description": "literal description",
				},
				Source: model.SourceLocation{File: "/test/main.tf"},
			},
		},
		State: &model.StateSnapshot{
			Resources: []model.StateResource{
				{
					Mode: "managed", Type: "aws_ssm_parameter", Name: "key",
					Instances: []model.StateResourceInstance{
						{Attributes: map[string]interface{}{
							"name":        "/app/prod/key",
							"tags":        map[string]interface{}{"Env": "prod", "terraform": "true"},
							"value":       "-----BEGIN RSA PRIVATE KEY-----\nMIIE...",
							"description": "literal description",
						}},
					},
				},
			},
		},
	}

	CompareWithState(project)

	if project.Resources[0].StateStatus != model.StateStatusInSync {
		t.Errorf("StateStatus = %q, want %q (expression attributes should not trigger drift)", project.Resources[0].StateStatus, model.StateStatusInSync)
	}
}
