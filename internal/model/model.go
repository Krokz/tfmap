package model

type SourceLocation struct {
	File      string `json:"file"`
	Line      int    `json:"line"`
	EndLine   int    `json:"endLine"`
	Column    int    `json:"column"`
	EndColumn int    `json:"endColumn"`
}

type StateStatus string

const (
	StateStatusUnknown        StateStatus = "unknown"
	StateStatusInSync         StateStatus = "in_sync"
	StateStatusDrifted        StateStatus = "drifted"
	StateStatusOrphanedState  StateStatus = "orphaned_in_state"
	StateStatusNotInState     StateStatus = "not_in_state"
)

type Backend struct {
	Type       string                 `json:"type"`
	Config     map[string]interface{} `json:"config"`
	Accessible bool                   `json:"accessible"`
	Source     SourceLocation         `json:"source"`
}

type Provider struct {
	Name    string                 `json:"name"`
	Alias   string                 `json:"alias,omitempty"`
	Version string                 `json:"version,omitempty"`
	Config  map[string]interface{} `json:"config"`
	Source  SourceLocation         `json:"source"`
}

type Resource struct {
	Type        string                 `json:"type"`
	Name        string                 `json:"name"`
	Provider    string                 `json:"provider,omitempty"`
	Count       interface{}            `json:"count,omitempty"`
	ForEach     interface{}            `json:"forEach,omitempty"`
	DependsOn   []string               `json:"dependsOn,omitempty"`
	Attributes  map[string]interface{} `json:"attributes"`
	Source      SourceLocation         `json:"source"`
	StateStatus StateStatus            `json:"stateStatus"`
	StateAttrs  map[string]interface{} `json:"stateAttrs,omitempty"`
	References  []string               `json:"references,omitempty"`
}

type DataSource struct {
	Type       string                 `json:"type"`
	Name       string                 `json:"name"`
	Provider   string                 `json:"provider,omitempty"`
	Attributes map[string]interface{} `json:"attributes"`
	Source     SourceLocation         `json:"source"`
	References []string               `json:"references,omitempty"`
}

type ModuleCall struct {
	Name       string                 `json:"name"`
	Source     string                 `json:"moduleSource"`
	Version    string                 `json:"version,omitempty"`
	Inputs     map[string]interface{} `json:"inputs"`
	DependsOn  []string               `json:"dependsOn,omitempty"`
	Providers  map[string]string      `json:"providers,omitempty"`
	Location   SourceLocation         `json:"source"`
	References []string               `json:"references,omitempty"`
}

type Variable struct {
	Name         string         `json:"name"`
	Type         string         `json:"type,omitempty"`
	Description  string         `json:"description,omitempty"`
	Default      interface{}    `json:"default,omitempty"`
	Value        interface{}    `json:"value,omitempty"`
	Sensitive    bool           `json:"sensitive,omitempty"`
	Validation   *Validation    `json:"validation,omitempty"`
	Source       SourceLocation `json:"source"`
	ValueSource  string         `json:"valueSource,omitempty"`
}

type Validation struct {
	Condition    string `json:"condition"`
	ErrorMessage string `json:"errorMessage"`
}

type Output struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Value       interface{}    `json:"value,omitempty"`
	Sensitive   bool           `json:"sensitive,omitempty"`
	DependsOn   []string       `json:"dependsOn,omitempty"`
	Source      SourceLocation `json:"source"`
	References  []string       `json:"references,omitempty"`
}

type Local struct {
	Name       string         `json:"name"`
	Expression interface{}    `json:"expression"`
	Source     SourceLocation `json:"source"`
	References []string       `json:"references,omitempty"`
}

type StateSnapshot struct {
	Serial    int              `json:"serial"`
	Version   int              `json:"version"`
	Lineage   string           `json:"lineage"`
	Resources []StateResource  `json:"resources"`
	Outputs   map[string]StateOutput `json:"outputs"`
}

type StateResource struct {
	Module    string                   `json:"module,omitempty"`
	Mode      string                   `json:"mode"`
	Type      string                   `json:"type"`
	Name      string                   `json:"name"`
	Provider  string                   `json:"provider"`
	Instances []StateResourceInstance   `json:"instances"`
}

type StateResourceInstance struct {
	IndexKey   interface{}            `json:"index_key,omitempty"`
	Attributes map[string]interface{} `json:"attributes"`
}

type StateOutput struct {
	Value     interface{} `json:"value"`
	Type      interface{} `json:"type"`
	Sensitive bool        `json:"sensitive"`
}

type DiagSeverity string

const (
	DiagError   DiagSeverity = "error"
	DiagWarning DiagSeverity = "warning"
	DiagInfo    DiagSeverity = "info"
)

type CycleEdge struct {
	From       string          `json:"from"`
	To         string          `json:"to"`
	Source     *SourceLocation `json:"source,omitempty"`
	ViaRefs    []string        `json:"viaRefs,omitempty"`
	Snippet    string          `json:"snippet,omitempty"`
}

type Diagnostic struct {
	Severity   DiagSeverity    `json:"severity"`
	Rule       string          `json:"rule"`
	Message    string          `json:"message"`
	Detail     string          `json:"detail,omitempty"`
	Source     *SourceLocation `json:"source,omitempty"`
	Entity     string          `json:"entity,omitempty"`
	CycleEdges []CycleEdge    `json:"cycleEdges,omitempty"`
}

type TerraformModule struct {
	Path        string       `json:"path"`
	IsRoot      bool         `json:"isRoot"`
	HasBackend  bool         `json:"hasBackend"`
	Backend     *Backend     `json:"backend,omitempty"`
	Resources   int          `json:"resources"`
	DataSources int          `json:"dataSources"`
	Variables   int          `json:"variables"`
	Outputs     int          `json:"outputs"`
	Modules     int          `json:"modules"`
}

type OrphanedResource struct {
	RootModule string                 `json:"rootModule,omitempty"`
	Module     string                 `json:"module,omitempty"`
	Type       string                 `json:"type"`
	Name       string                 `json:"name"`
	Provider   string                 `json:"provider"`
	Attributes map[string]interface{} `json:"attributes,omitempty"`
}

type Project struct {
	Path              string                            `json:"path"`
	Backend           *Backend                          `json:"backend,omitempty"`
	Backends          []Backend                         `json:"-"`
	Providers         []Provider                        `json:"providers"`
	Resources         []Resource                        `json:"resources"`
	DataSources       []DataSource                      `json:"dataSources"`
	Modules           []ModuleCall                      `json:"modules"`
	Variables         []Variable                        `json:"variables"`
	Outputs           []Output                          `json:"outputs"`
	Locals            []Local                           `json:"locals"`
	TFVars            map[string]interface{}             `json:"tfvars,omitempty"`
	State             *StateSnapshot                    `json:"state,omitempty"`
	ModuleStates      map[string]*StateSnapshot         `json:"moduleStates,omitempty"`
	Diagnostics       []Diagnostic                      `json:"diagnostics"`
	DiscoveredModules []TerraformModule                 `json:"discoveredModules"`
	OrphanedResources []OrphanedResource                `json:"orphanedResources,omitempty"`
}
