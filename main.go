package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
)

type Config struct {
	LoginShell  string
	PathType    string
	MsysRoot    string
	WinSymlinks bool
	MSystem     string
	Wd          string
}

var validPathTypes = map[string]bool{
	"minimal": true,
	"strict":  true,
	"inherit": true,
}

func fatalf(format string, a ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", a...)
	os.Exit(1)
}

func getMSystemFromName(name string) string {
	m := map[string]string{
		"MINGW64":    "MINGW64",
		"MINGW32":    "MINGW32",
		"UCRT64":     "UCRT64",
		"CLANG64":    "CLANG64",
		"CLANGARM64": "CLANGARM64",
		"MSYS":       "MSYS",
		"MSYS2":      "MSYS",
	}
	return m[strings.ToUpper(name)]
}

func getMSystemFromExecName(execName string) string {
	base := strings.TrimSuffix(execName, filepath.Ext(execName))
	return getMSystemFromName(base)
}

func loadJSONConfig(path string) (Config, error) {
	cfg := Config{LoginShell: "bash", PathType: "minimal"}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return cfg, fmt.Errorf("read config error: %w", err)
	}
	var tmp struct {
		LoginShell  string `json:"loginShell,omitempty"`
		PathType    string `json:"pathType,omitempty"`
		MsysRoot    string `json:"msysRoot,omitempty"`
		WinSymlinks bool   `json:"winSymlinks,omitempty"`
	}
	if err := json.Unmarshal(data, &tmp); err != nil {
		return cfg, fmt.Errorf("json parse error: %w", err)
	}
	if tmp.LoginShell != "" {
		cfg.LoginShell = tmp.LoginShell
	}
	if tmp.PathType != "" {
		cfg.PathType = tmp.PathType
	}
	cfg.MsysRoot = tmp.MsysRoot
	cfg.WinSymlinks = tmp.WinSymlinks
	return cfg, nil
}

func splitArgs() (launcherArgs, shellArgs []string) {
	args := os.Args[1:]
	for i, a := range args {
		if a == "--" {
			return args[:i], args[i+1:]
		}
	}
	return args, nil
}

func parseLauncherFlags(launcherArgs []string) Config {
	var cfg Config
	fs := flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	fs.StringVar(&cfg.LoginShell, "shell", "", "login shell")
	fs.StringVar(&cfg.PathType, "pathtype", "", "MSYS2_PATH_TYPE")
	fs.StringVar(&cfg.MsysRoot, "msysroot", "", "MSYS2 root path")
	fs.BoolVar(&cfg.WinSymlinks, "winsymlinks", false, "enable winsymlinks")
	fs.StringVar(&cfg.Wd, "wd", "", "working directory")
	fs.StringVar(&cfg.MSystem, "msystem", "", "MSYSTEM (required if exec name cannot infer)")
	fs.Parse(launcherArgs)
	return cfg
}

func mergeConfig(jsonCfg, cliCfg Config) Config {
	if cliCfg.LoginShell != "" {
		jsonCfg.LoginShell = cliCfg.LoginShell
	}
	if cliCfg.PathType != "" {
		jsonCfg.PathType = cliCfg.PathType
	}
	if cliCfg.MsysRoot != "" {
		jsonCfg.MsysRoot = cliCfg.MsysRoot
	}
	if cliCfg.WinSymlinks {
		jsonCfg.WinSymlinks = true
	}
	if cliCfg.Wd != "" {
		jsonCfg.Wd = cliCfg.Wd
	}
	if cliCfg.MSystem != "" {
		jsonCfg.MSystem = cliCfg.MSystem
	}
	return jsonCfg
}

func resolveMSystem(execName, cliValue string) string {
	auto := getMSystemFromExecName(execName)
	if auto != "" && cliValue != "" {
		fatalf("MSYSTEM conflict: exec name implies %s but -msystem provides %s", auto, cliValue)
	}
	if auto == "" && cliValue == "" {
		fatalf("MSYSTEM must be specified (exec name cannot infer)")
	}
	if cliValue != "" {
		v := getMSystemFromName(cliValue)
		if v == "" {
			fatalf("invalid MSYSTEM: %s", cliValue)
		}
		return v
	}
	return auto
}

func validatePathType(pt string) string {
	pt = strings.ToLower(pt)
	if !validPathTypes[pt] {
		fatalf("invalid MSYS2_PATH_TYPE: %s", pt)
	}
	return pt
}

func buildMSYSEnv(cfg Config) string {
	if cfg.WinSymlinks {
		return "winsymlinks:nativestrict"
	}
	return ""
}

func applyEnv(cfg Config) []string {
	env := os.Environ()
	env = append(env, "MSYSTEM="+cfg.MSystem)
	env = append(env, "CHERE_INVOKING=1")
	env = append(env, "MSYS2_PATH_TYPE="+validatePathType(cfg.PathType))
	env = append(env, "MSYS="+buildMSYSEnv(cfg))
	return env
}

func buildShellCommand(cfg Config, shellArgs []string) *exec.Cmd {
	shellPath := filepath.Join(cfg.MsysRoot, "usr", "bin", cfg.LoginShell)
	workDir := cfg.Wd
	if workDir == "" {
		workDir, _ = os.Getwd()
	}
	finalArgs := append([]string{"-l"}, shellArgs...)
	cmd := exec.Command(shellPath, finalArgs...)
	cmd.Dir = workDir
	cmd.Env = applyEnv(cfg)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd
}

func runShell(cfg Config, shellArgs []string) {
	cmd := buildShellCommand(cfg, shellArgs)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan)
	go func() {
		for range sigChan {
		}
	}()

	if err := cmd.Run(); err != nil {
		if e, ok := err.(*exec.ExitError); ok {
			os.Exit(e.ExitCode())
		}
		fatalf("shell error: %v", err)
	}
}

func prepareConfig() (Config, []string) {
	execPath, err := os.Executable()
	if err != nil {
		fatalf("cannot get executable path: %v", err)
	}
	execName := filepath.Base(execPath)

	jsonPath := filepath.Join(filepath.Dir(execPath), "msys2_shell.json")
	jsonCfg, err := loadJSONConfig(jsonPath)
	if err != nil {
		fatalf("config load error: %v", err)
	}

	launcherArgs, shellArgs := splitArgs()
	cliCfg := parseLauncherFlags(launcherArgs)
	cfg := mergeConfig(jsonCfg, cliCfg)
	cfg.MSystem = resolveMSystem(execName, cliCfg.MSystem)

	if cfg.MsysRoot == "" {
		fatalf("msysroot must be specified")
	}

	if shellArgs == nil {
		shellArgs = []string{}
	}

	return cfg, shellArgs
}

func main() {
	cfg, shellArgs := prepareConfig()
	runShell(cfg, shellArgs)
}
