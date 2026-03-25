package parser

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
)

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
		t.Fatalf("writing %s: %v", name, err)
	}
}

func assertNoError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func assertError(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func assertStringEqual(t *testing.T, field, got, want string) {
	t.Helper()
	if got != want {
		t.Errorf("%s = %q, want %q", field, got, want)
	}
}

func assertIntEqual(t *testing.T, field string, got, want int) {
	t.Helper()
	if got != want {
		t.Errorf("%s = %d, want %d", field, got, want)
	}
}

func assertBoolEqual(t *testing.T, field string, got, want bool) {
	t.Helper()
	if got != want {
		t.Errorf("%s = %v, want %v", field, got, want)
	}
}

func assertContains(t *testing.T, field string, slice []string, want string) {
	t.Helper()
	for _, s := range slice {
		if s == want {
			return
		}
	}
	t.Errorf("%s %v does not contain %q", field, slice, want)
}

func assertNotContains(t *testing.T, field string, slice []string, notWant string) {
	t.Helper()
	for _, s := range slice {
		if s == notWant {
			t.Errorf("%s %v should not contain %q", field, slice, notWant)
			return
		}
	}
}

func TestParseResources(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "main.tf", `
resource "aws_instance" "web" {
  ami           = "ami-12345"
  instance_type = "t2.micro"
  count         = 2
}

resource "aws_security_group" "allow_ssh" {
  name        = "allow_ssh"
  description = "Allow SSH inbound"
  for_each    = var.sg_set
  depends_on  = [aws_instance.web]

  ingress {
    from_port = 22
    to_port   = 22
    protocol  = "tcp"
  }
}
`)

	proj, err := Parse(dir)
	assertNoError(t, err)
	assertIntEqual(t, "len(Resources)", len(proj.Resources), 2)

	var web, sg *struct {
		idx int
	}
	for i, r := range proj.Resources {
		switch r.Name {
		case "web":
			web = &struct{ idx int }{i}
		case "allow_ssh":
			sg = &struct{ idx int }{i}
		}
	}
	if web == nil || sg == nil {
		t.Fatal("did not find both resources")
	}

	r := proj.Resources[web.idx]
	assertStringEqual(t, "web.Type", r.Type, "aws_instance")
	assertStringEqual(t, "web.Name", r.Name, "web")
	assertStringEqual(t, "web.Attributes[ami]", r.Attributes["ami"].(string), "ami-12345")
	assertStringEqual(t, "web.Attributes[instance_type]", r.Attributes["instance_type"].(string), "t2.micro")
	if ct, ok := r.Count.(int64); !ok || ct != 2 {
		t.Errorf("web.Count = %v (%T), want int64(2)", r.Count, r.Count)
	}
	assertStringEqual(t, "web.Source.File", r.Source.File, "main.tf")
	if r.Source.Line < 1 {
		t.Error("web.Source.Line should be > 0")
	}

	r2 := proj.Resources[sg.idx]
	assertStringEqual(t, "sg.Type", r2.Type, "aws_security_group")
	assertStringEqual(t, "sg.ForEach", r2.ForEach.(string), "var.sg_set")
	if len(r2.DependsOn) != 1 || r2.DependsOn[0] != "aws_instance.web" {
		t.Errorf("sg.DependsOn = %v, want [aws_instance.web]", r2.DependsOn)
	}
	assertStringEqual(t, "sg.Source.File", r2.Source.File, "main.tf")
}

func TestParseDataSources(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "data.tf", `
data "aws_ami" "latest" {
  most_recent = true
  owners      = ["self"]
  name_regex  = var.ami_pattern

  filter {
    name   = "name"
    values = ["myapp-*"]
  }
}
`)

	proj, err := Parse(dir)
	assertNoError(t, err)
	assertIntEqual(t, "len(DataSources)", len(proj.DataSources), 1)

	d := proj.DataSources[0]
	assertStringEqual(t, "Type", d.Type, "aws_ami")
	assertStringEqual(t, "Name", d.Name, "latest")
	assertBoolEqual(t, "Attributes[most_recent]", d.Attributes["most_recent"].(bool), true)
	assertStringEqual(t, "Source.File", d.Source.File, "data.tf")
	assertContains(t, "References", d.References, "var.ami_pattern")
}

func TestParseModules(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "modules.tf", `
module "vpc" {
  source  = "terraform-aws-modules/vpc/aws"
  version = "3.19.0"

  name     = "my-vpc"
  cidr     = var.vpc_cidr

  providers = {
    aws = aws.west
  }
}
`)

	proj, err := Parse(dir)
	assertNoError(t, err)
	assertIntEqual(t, "len(Modules)", len(proj.Modules), 1)

	m := proj.Modules[0]
	assertStringEqual(t, "Name", m.Name, "vpc")
	assertStringEqual(t, "Source", m.Source, "terraform-aws-modules/vpc/aws")
	assertStringEqual(t, "Version", m.Version, "3.19.0")
	assertStringEqual(t, "Inputs[name]", m.Inputs["name"].(string), "my-vpc")
	assertContains(t, "References", m.References, "var.vpc_cidr")

	if m.Providers == nil {
		t.Fatal("Providers map is nil")
	}
	if m.Providers["aws"] != "aws.west" {
		t.Errorf("Providers[aws] = %q, want %q", m.Providers["aws"], "aws.west")
	}

	assertStringEqual(t, "Location.File", m.Location.File, "modules.tf")
	if m.Location.Line < 1 {
		t.Error("Location.Line should be > 0")
	}
}

func TestParseVariables(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "variables.tf", `
variable "instance_type" {
  type        = string
  description = "EC2 instance type"
  default     = "t2.micro"
  sensitive   = true

  validation {
    condition     = length(var.instance_type) > 0
    error_message = "Instance type must not be empty."
  }
}

variable "count_num" {
}
`)

	proj, err := Parse(dir)
	assertNoError(t, err)
	assertIntEqual(t, "len(Variables)", len(proj.Variables), 2)

	var full, minimal *int
	for i, v := range proj.Variables {
		switch v.Name {
		case "instance_type":
			idx := i
			full = &idx
		case "count_num":
			idx := i
			minimal = &idx
		}
	}
	if full == nil || minimal == nil {
		t.Fatal("did not find both variables")
	}

	v := proj.Variables[*full]
	assertStringEqual(t, "Name", v.Name, "instance_type")
	assertStringEqual(t, "Type", v.Type, "string")
	assertStringEqual(t, "Description", v.Description, "EC2 instance type")
	assertStringEqual(t, "Default", v.Default.(string), "t2.micro")
	assertBoolEqual(t, "Sensitive", v.Sensitive, true)
	if v.Validation == nil {
		t.Fatal("Validation is nil")
	}
	assertStringEqual(t, "Validation.ErrorMessage", v.Validation.ErrorMessage, "Instance type must not be empty.")
	if v.Validation.Condition == "" {
		t.Error("Validation.Condition should not be empty")
	}
	assertStringEqual(t, "Source.File", v.Source.File, "variables.tf")

	v2 := proj.Variables[*minimal]
	assertStringEqual(t, "minimal.Name", v2.Name, "count_num")
	assertStringEqual(t, "minimal.Type", v2.Type, "")
	if v2.Validation != nil {
		t.Error("minimal variable should have nil Validation")
	}
}

func TestParseOutputs(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "outputs.tf", `
output "instance_ip" {
  description = "The public IP"
  value       = aws_instance.web.public_ip
  sensitive   = true
  depends_on  = [aws_security_group.allow_ssh]
}
`)

	proj, err := Parse(dir)
	assertNoError(t, err)
	assertIntEqual(t, "len(Outputs)", len(proj.Outputs), 1)

	o := proj.Outputs[0]
	assertStringEqual(t, "Name", o.Name, "instance_ip")
	assertStringEqual(t, "Description", o.Description, "The public IP")
	assertBoolEqual(t, "Sensitive", o.Sensitive, true)
	if len(o.DependsOn) != 1 || o.DependsOn[0] != "aws_security_group.allow_ssh" {
		t.Errorf("DependsOn = %v, want [aws_security_group.allow_ssh]", o.DependsOn)
	}
	assertContains(t, "References", o.References, "aws_instance.web")
	assertStringEqual(t, "Source.File", o.Source.File, "outputs.tf")
}

func TestParseLocals(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "locals.tf", `
locals {
  env    = "production"
  prefix = "myapp-${local.env}"
  count  = var.instance_count
}
`)

	proj, err := Parse(dir)
	assertNoError(t, err)
	assertIntEqual(t, "len(Locals)", len(proj.Locals), 3)

	byName := map[string]int{}
	for i, l := range proj.Locals {
		byName[l.Name] = i
	}

	if _, ok := byName["env"]; !ok {
		t.Fatal("missing local 'env'")
	}
	if _, ok := byName["prefix"]; !ok {
		t.Fatal("missing local 'prefix'")
	}
	if _, ok := byName["count"]; !ok {
		t.Fatal("missing local 'count'")
	}

	envLocal := proj.Locals[byName["env"]]
	assertStringEqual(t, "env.Expression", envLocal.Expression.(string), "production")
	assertStringEqual(t, "env.Source.File", envLocal.Source.File, "locals.tf")

	prefixLocal := proj.Locals[byName["prefix"]]
	assertContains(t, "prefix.References", prefixLocal.References, "local.env")

	countLocal := proj.Locals[byName["count"]]
	assertContains(t, "count.References", countLocal.References, "var.instance_count")
}

func TestParseProviders(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "providers.tf", `
provider "aws" {
  region = "us-east-1"
}

terraform {
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
    }
  }
}
`)

	proj, err := Parse(dir)
	assertNoError(t, err)

	var awsProv *int
	for i, p := range proj.Providers {
		if p.Name == "aws" && p.Alias == "" {
			idx := i
			awsProv = &idx
			break
		}
	}
	if awsProv == nil {
		t.Fatal("no aws provider found")
	}

	p := proj.Providers[*awsProv]
	assertStringEqual(t, "Name", p.Name, "aws")
	assertStringEqual(t, "Version", p.Version, "~> 5.0")
	assertStringEqual(t, "Config[region]", p.Config["region"].(string), "us-east-1")
}

func TestParseBackend(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "backend.tf", `
terraform {
  backend "s3" {
    bucket = "my-state-bucket"
    key    = "terraform.tfstate"
    region = "us-west-2"
  }
}
`)

	proj, err := Parse(dir)
	assertNoError(t, err)

	if proj.Backend == nil {
		t.Fatal("Backend is nil")
	}
	assertStringEqual(t, "Type", proj.Backend.Type, "s3")
	assertStringEqual(t, "Config[bucket]", proj.Backend.Config["bucket"].(string), "my-state-bucket")
	assertStringEqual(t, "Config[key]", proj.Backend.Config["key"].(string), "terraform.tfstate")
	assertStringEqual(t, "Config[region]", proj.Backend.Config["region"].(string), "us-west-2")
	assertStringEqual(t, "Source.File", proj.Backend.Source.File, "backend.tf")
	if proj.Backend.Source.Line < 1 {
		t.Error("Backend.Source.Line should be > 0")
	}
}

func TestParseTFVars(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "variables.tf", `
variable "region" {
  type    = string
  default = "us-east-1"
}

variable "count_num" {
  type    = number
  default = 1
}
`)
	writeFile(t, dir, "terraform.tfvars", `
region    = "eu-west-1"
count_num = 5
`)

	proj, err := Parse(dir)
	assertNoError(t, err)

	byName := map[string]int{}
	for i, v := range proj.Variables {
		byName[v.Name] = i
	}

	regionVar := proj.Variables[byName["region"]]
	assertStringEqual(t, "region.Value", regionVar.Value.(string), "eu-west-1")
	assertStringEqual(t, "region.ValueSource", regionVar.ValueSource, "tfvars")

	countVar := proj.Variables[byName["count_num"]]
	if ct, ok := countVar.Value.(int64); !ok || ct != 5 {
		t.Errorf("count_num.Value = %v (%T), want int64(5)", countVar.Value, countVar.Value)
	}
	assertStringEqual(t, "count_num.ValueSource", countVar.ValueSource, "tfvars")
}

func TestParseAutoTFVars(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "variables.tf", `
variable "env" {
  type = string
}
`)
	writeFile(t, dir, "prod.auto.tfvars", `
env = "production"
`)

	proj, err := Parse(dir)
	assertNoError(t, err)
	assertIntEqual(t, "len(Variables)", len(proj.Variables), 1)

	v := proj.Variables[0]
	assertStringEqual(t, "Value", v.Value.(string), "production")
	assertStringEqual(t, "ValueSource", v.ValueSource, "tfvars")
}

func TestParseReferences(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "refs.tf", `
resource "aws_instance" "app" {
  ami              = var.ami_id
  subnet_id        = local.subnet
  vpc_id           = module.vpc.vpc_id
  security_groups  = data.aws_security_groups.default.ids
  private_ip       = aws_eip.main.private_ip
  instance_type    = count.index
  key_name         = each.key
  user_data        = self.id
  template_file    = path.module
}
`)

	proj, err := Parse(dir)
	assertNoError(t, err)
	assertIntEqual(t, "len(Resources)", len(proj.Resources), 1)

	refs := proj.Resources[0].References
	assertContains(t, "References", refs, "var.ami_id")
	assertContains(t, "References", refs, "local.subnet")
	assertContains(t, "References", refs, "module.vpc.vpc_id")
	assertContains(t, "References", refs, "data.aws_security_groups.default")
	assertContains(t, "References", refs, "aws_eip.main")

	assertNotContains(t, "References", refs, "count.index")
	assertNotContains(t, "References", refs, "each.key")
	assertNotContains(t, "References", refs, "self.id")
	assertNotContains(t, "References", refs, "path.module")
}

func TestParseEmptyDir(t *testing.T) {
	dir := t.TempDir()
	proj, err := Parse(dir)
	assertNoError(t, err)

	assertIntEqual(t, "len(Resources)", len(proj.Resources), 0)
	assertIntEqual(t, "len(DataSources)", len(proj.DataSources), 0)
	assertIntEqual(t, "len(Modules)", len(proj.Modules), 0)
	assertIntEqual(t, "len(Variables)", len(proj.Variables), 0)
	assertIntEqual(t, "len(Outputs)", len(proj.Outputs), 0)
	assertIntEqual(t, "len(Locals)", len(proj.Locals), 0)
	assertIntEqual(t, "len(Providers)", len(proj.Providers), 0)
	if proj.Backend != nil {
		t.Error("Backend should be nil for empty dir")
	}
}

func TestParseInvalidHCL(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "broken.tf", `
resource "aws_instance" "web" {
  ami = "ami-12345"
  THIS IS NOT VALID HCL !!!
`)

	_, err := Parse(dir)
	assertError(t, err)
}

func TestParseSourceLocations(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "locs.tf", `
resource "null_resource" "first" {
  triggers = {}
}

variable "loc_var" {
  type = string
}

output "loc_out" {
  value = "hello"
}
`)

	proj, err := Parse(dir)
	assertNoError(t, err)

	tests := []struct {
		name string
		file string
		line int
		end  int
	}{
		{"resource", proj.Resources[0].Source.File, proj.Resources[0].Source.Line, proj.Resources[0].Source.EndLine},
		{"variable", proj.Variables[0].Source.File, proj.Variables[0].Source.Line, proj.Variables[0].Source.EndLine},
		{"output", proj.Outputs[0].Source.File, proj.Outputs[0].Source.Line, proj.Outputs[0].Source.EndLine},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertStringEqual(t, "File", tt.file, "locs.tf")
			if tt.line < 1 {
				t.Errorf("Line = %d, want > 0", tt.line)
			}
			if tt.end < tt.line {
				t.Errorf("EndLine = %d, should be >= Line = %d", tt.end, tt.line)
			}
		})
	}
}

func TestParseReferences_TableDriven(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    []string
		notWant []string
	}{
		{
			name:  "variable reference",
			input: `resource "null_resource" "a" { triggers = { x = var.foo } }`,
			want:  []string{"var.foo"},
		},
		{
			name:  "local reference",
			input: `resource "null_resource" "a" { triggers = { x = local.bar } }`,
			want:  []string{"local.bar"},
		},
		{
			name:  "module reference",
			input: `resource "null_resource" "a" { triggers = { x = module.net.id } }`,
			want:  []string{"module.net.id"},
		},
		{
			name:  "data source reference",
			input: `resource "null_resource" "a" { triggers = { x = data.aws_ami.ubuntu.id } }`,
			want:  []string{"data.aws_ami.ubuntu"},
		},
		{
			name:  "resource reference",
			input: `resource "null_resource" "a" { triggers = { x = aws_vpc.main.id } }`,
			want:  []string{"aws_vpc.main"},
		},
		{
			name:    "builtins excluded",
			input:   `resource "null_resource" "a" { triggers = { x = count.index, y = each.value, z = self.arn, w = path.root, v = terraform.workspace } }`,
			notWant: []string{"count.index", "each.value", "self.arn", "path.root", "terraform.workspace"},
		},
		{
			name:  "multiple references",
			input: `resource "null_resource" "a" { triggers = { a = var.x, b = local.y, c = aws_s3_bucket.logs.arn } }`,
			want:  []string{"var.x", "local.y", "aws_s3_bucket.logs"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			writeFile(t, dir, "main.tf", tt.input)

			proj, err := Parse(dir)
			assertNoError(t, err)

			if len(proj.Resources) == 0 {
				t.Fatal("no resources parsed")
			}

			refs := proj.Resources[0].References
			sort.Strings(refs)
			for _, w := range tt.want {
				assertContains(t, "References", refs, w)
			}
			for _, nw := range tt.notWant {
				assertNotContains(t, "References", refs, nw)
			}
		})
	}
}
