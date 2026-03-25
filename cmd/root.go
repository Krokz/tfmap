package cmd

import (
	"bufio"
	"fmt"
	"io/fs"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/Krokz/tfmap/internal/diagnostics"
	"github.com/Krokz/tfmap/internal/model"
	"github.com/Krokz/tfmap/internal/parser"
	"github.com/Krokz/tfmap/internal/server"
	"github.com/Krokz/tfmap/internal/state"
	"github.com/Krokz/tfmap/internal/watcher"
	"github.com/spf13/cobra"
)

var (
	port       int
	noBrowser  bool
	noState    bool
	awsProfile string
)

// WebDistFS is set by main() with the embedded web/dist filesystem.
var WebDistFS fs.FS

var rootCmd = &cobra.Command{
	Use:   "tfmap [path]",
	Short: "Visualize and explore Terraform projects",
	Long:  "tfmap watches a Terraform project directory, parses all HCL files, reads state if accessible, and serves an interactive UI for exploring resources, modules, variables, and state drift.",
	Args:  cobra.MaximumNArgs(1),
	RunE:  run,
}

func init() {
	rootCmd.Flags().IntVarP(&port, "port", "p", 0, "port to serve the UI on (default: random available port)")
	rootCmd.Flags().BoolVar(&noBrowser, "no-browser", false, "don't open the browser automatically")
	rootCmd.Flags().BoolVar(&noState, "no-state", false, "skip state reading entirely")
	rootCmd.Flags().StringVar(&awsProfile, "aws-profile", "", "AWS profile to use for S3 state reading")
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func run(cmd *cobra.Command, args []string) error {
	var tfPath string
	switch {
	case len(args) > 0:
		tfPath = args[0]
	case isInteractive():
		fmt.Println()
		picked, err := browseForDirectory()
		if err != nil {
			if err == errBrowseCancelled {
				fmt.Println("No directory selected.")
				return nil
			}
			return err
		}
		tfPath = picked
		fmt.Printf("\nSelected: %s\n", tfPath)
	default:
		return fmt.Errorf("no path provided — run interactively or pass a directory:\n  tfmap /path/to/terraform/project")
	}

	absPath, err := filepath.Abs(tfPath)
	if err != nil {
		return fmt.Errorf("resolving path: %w", err)
	}

	info, err := os.Stat(absPath)
	if err != nil {
		return fmt.Errorf("path %s: %w", absPath, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("path %s is not a directory", absPath)
	}

	project, err := parser.Parse(absPath)
	if err != nil {
		return fmt.Errorf("parsing terraform project: %w", err)
	}

	var selectedPaths map[string]bool
	var profileMap map[string]string
	if !noState {
		selectedPaths, profileMap = selectBackends(project, awsProfile)
	}

	stateReader := state.NewReader()
	if !noState {
		loadState(project, stateReader, absPath, selectedPaths, profileMap)
	}

	project.Diagnostics = diagnostics.Analyze(project)
	printDiagSummary(project)

	srv := server.New(project, WebDistFS)

	w, err := watcher.New(absPath, func() {
		newProject, pErr := parser.Parse(absPath)
		if pErr != nil {
			log.Printf("Re-parse error: %v", pErr)
			return
		}
		if !noState {
			loadState(newProject, stateReader, absPath, selectedPaths, profileMap)
		}
		newProject.Diagnostics = diagnostics.Analyze(newProject)
		srv.UpdateProject(newProject)
	})
	if err != nil {
		return fmt.Errorf("setting up watcher: %w", err)
	}
	defer w.Close()

	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return fmt.Errorf("binding port: %w", err)
	}
	actualPort := listener.Addr().(*net.TCPAddr).Port
	url := fmt.Sprintf("http://127.0.0.1:%d", actualPort)

	fmt.Printf("\ntfmap serving at %s\n", url)
	fmt.Printf("Watching: %s\n", absPath)

	if !noBrowser {
		openBrowser(url)
	}

	return srv.Serve(listener)
}

type backendChoice struct {
	path    string
	backend *model.Backend
}

func collectBackends(project *model.Project) []backendChoice {
	var choices []backendChoice
	seen := make(map[string]bool)

	if project.Backend != nil {
		choices = append(choices, backendChoice{path: ".", backend: project.Backend})
		seen["."] = true
	}

	for _, dm := range project.DiscoveredModules {
		if !dm.IsRoot || !dm.HasBackend || dm.Backend == nil {
			continue
		}
		if seen[dm.Path] {
			continue
		}
		choices = append(choices, backendChoice{path: dm.Path, backend: dm.Backend})
		seen[dm.Path] = true
	}

	return choices
}

func backendSummary(b *model.Backend) string {
	switch b.Type {
	case "s3":
		bucket, _ := b.Config["bucket"].(string)
		key, _ := b.Config["key"].(string)
		region, _ := b.Config["region"].(string)
		if key == "" {
			key = "terraform.tfstate"
		}
		if region == "" {
			region = "us-east-1"
		}
		return fmt.Sprintf("s3://%s/%s (%s)", bucket, key, region)
	case "local", "":
		return "local"
	default:
		return b.Type
	}
}

func selectBackends(project *model.Project, globalProfile string) (map[string]bool, map[string]string) {
	backends := collectBackends(project)
	profileMap := make(map[string]string)

	if len(backends) == 0 {
		return nil, profileMap
	}

	if len(backends) == 1 {
		b := backends[0]
		label := pathLabel(b.path)
		fmt.Printf("\nDetected backend: %s — %s\n", label, backendSummary(b.backend))
		if b.backend.Type == "s3" {
			profileMap[b.path] = promptS3Profile(globalProfile, label)
		}
		return map[string]bool{b.path: true}, profileMap
	}

	fmt.Printf("\nDetected %d backends:\n", len(backends))
	for i, b := range backends {
		fmt.Printf("  [%d] %-30s — %s\n", i+1, pathLabel(b.path), backendSummary(b.backend))
	}
	fmt.Println()
	fmt.Println("  [a] All backends")
	fmt.Println("  [n] None (skip state watching)")
	fmt.Println()

	choice := promptString("Select backends to load state for [a]")
	if choice == "" {
		choice = "a"
	}

	selected := make(map[string]bool)

	switch strings.ToLower(choice) {
	case "n", "none":
		fmt.Println("Skipping state reading.")
		return nil, profileMap
	case "a", "all":
		for _, b := range backends {
			selected[b.path] = true
		}
	default:
		parts := strings.Split(choice, ",")
		for _, p := range parts {
			p = strings.TrimSpace(p)
			idx, err := strconv.Atoi(p)
			if err != nil || idx < 1 || idx > len(backends) {
				fmt.Printf("  Ignoring invalid choice: %s\n", p)
				continue
			}
			selected[backends[idx-1].path] = true
		}
		if len(selected) == 0 {
			fmt.Println("No valid backends selected. Skipping state reading.")
			return nil, profileMap
		}
	}

	var s3Backends []backendChoice
	for _, b := range backends {
		if selected[b.path] && b.backend.Type == "s3" {
			s3Backends = append(s3Backends, b)
		}
	}

	if len(s3Backends) > 0 {
		if globalProfile != "" {
			fmt.Printf("\nUsing AWS profile for all S3 backends: %s (from --aws-profile)\n", globalProfile)
			for _, b := range s3Backends {
				profileMap[b.path] = globalProfile
			}
		} else if len(s3Backends) == 1 {
			profileMap[s3Backends[0].path] = promptS3Profile("", pathLabel(s3Backends[0].path))
		} else {
			if !promptYesNo("\nWould you like to connect to the remote S3 state backends?", true) {
				fmt.Println("Skipping S3 state reading.")
				for _, b := range s3Backends {
					delete(selected, b.path)
				}
			} else {
				checkAWSCLI()
				if promptYesNo("Use the same AWS profile for all S3 backends?", true) {
					profile := promptString("AWS Profile (leave empty for default)")
					for _, b := range s3Backends {
						profileMap[b.path] = profile
					}
				} else {
					for _, b := range s3Backends {
						label := pathLabel(b.path)
						profile := promptString(fmt.Sprintf("AWS Profile for %s (%s)", label, backendSummary(b.backend)))
						profileMap[b.path] = profile
					}
				}
			}
		}
	}

	selectedNames := make([]string, 0, len(selected))
	for _, b := range backends {
		if selected[b.path] {
			selectedNames = append(selectedNames, pathLabel(b.path))
		}
	}
	if len(selectedNames) > 0 {
		fmt.Printf("Loading state for: %s\n", strings.Join(selectedNames, ", "))
	}

	return selected, profileMap
}

func pathLabel(p string) string {
	if p == "." {
		return "(root)"
	}
	return p
}

func promptS3Profile(globalProfile string, label string) string {
	if globalProfile != "" {
		fmt.Printf("Using AWS profile: %s (from --aws-profile)\n", globalProfile)
		return globalProfile
	}

	if !promptYesNo(fmt.Sprintf("Connect to S3 state for %s?", label), true) {
		return ""
	}

	checkAWSCLI()
	return promptString("AWS Profile (leave empty for default)")
}

func loadState(project *model.Project, stateReader *state.Reader, absPath string, selectedPaths map[string]bool, profileMap map[string]string) {
	if selectedPaths == nil {
		return
	}

	if selectedPaths["."] {
		if project.Backend != nil {
			snap, sErr := stateReader.Read(project.Backend, absPath, profileMap["."])
			if sErr != nil {
				log.Printf("Warning: could not read state: %v", sErr)
			} else if snap != nil {
				project.State = snap
				project.Backend.Accessible = true
			}
		} else {
			snap, sErr := stateReader.ReadLocal(absPath)
			if sErr == nil {
				project.State = snap
			}
		}
	}

	project.ModuleStates = make(map[string]*model.StateSnapshot)
	for i, dm := range project.DiscoveredModules {
		if !dm.IsRoot || !dm.HasBackend || dm.Backend == nil || dm.Path == "." {
			continue
		}
		if !selectedPaths[dm.Path] {
			continue
		}
		modulePath := filepath.Join(absPath, dm.Path)
		snap, sErr := stateReader.Read(dm.Backend, modulePath, profileMap[dm.Path])
		if sErr != nil {
			log.Printf("Warning: could not read state for %s: %v", dm.Path, sErr)
		} else if snap != nil {
			project.ModuleStates[dm.Path] = snap
			project.DiscoveredModules[i].Backend.Accessible = true
		}
	}

	state.CompareWithState(project)
}

func checkAWSCLI() {
	out, err := exec.Command("aws", "--version").Output()
	if err != nil {
		fmt.Println("  AWS CLI not found. Will attempt using default credential chain (env vars, instance profile, etc.)")
	} else {
		version := strings.TrimSpace(string(out))
		if idx := strings.Index(version, "\n"); idx > 0 {
			version = version[:idx]
		}
		fmt.Printf("  AWS CLI found: %s\n", version)
	}
}

func promptYesNo(question string, defaultYes bool) bool {
	reader := bufio.NewReader(os.Stdin)
	hint := "Y/n"
	if !defaultYes {
		hint = "y/N"
	}
	fmt.Printf("%s [%s]: ", question, hint)
	text, _ := reader.ReadString('\n')
	text = strings.TrimSpace(strings.ToLower(text))
	if text == "" {
		return defaultYes
	}
	return text == "y" || text == "yes"
}

func promptString(question string) string {
	reader := bufio.NewReader(os.Stdin)
	fmt.Printf("%s: ", question)
	text, _ := reader.ReadString('\n')
	return strings.TrimSpace(text)
}

func printDiagSummary(project *model.Project) {
	if len(project.Diagnostics) == 0 {
		return
	}
	errors, warnings, infos := 0, 0, 0
	for _, d := range project.Diagnostics {
		switch d.Severity {
		case model.DiagError:
			errors++
		case model.DiagWarning:
			warnings++
		case model.DiagInfo:
			infos++
		}
	}
	fmt.Printf("\nDiagnostics: ")
	parts := []string{}
	if errors > 0 {
		parts = append(parts, fmt.Sprintf("%d error(s)", errors))
	}
	if warnings > 0 {
		parts = append(parts, fmt.Sprintf("%d warning(s)", warnings))
	}
	if infos > 0 {
		parts = append(parts, fmt.Sprintf("%d suggestion(s)", infos))
	}
	fmt.Println(strings.Join(parts, ", "))
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		return
	}
	_ = cmd.Start()
}
