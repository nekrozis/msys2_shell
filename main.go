package main

import (
	"encoding/json"
	"errors"
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
	UseHome     bool
}

type Spec struct {
	Cfg       Config
	ShellArgs []string
}

var validPathTypes = map[string]bool{
	"minimal": true,
	"strict":  true,
	"inherit": true,
}

func fatal(err error) {
	_, _ = fmt.Fprintln(os.Stderr, err)
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

func loadJSONConfig(path string) Config {
	cfg := Config{
		LoginShell: "bash",
		PathType:   "minimal",
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg
		}
		fatal(fmt.Errorf("read config file failed: %w", err))
	}

	var tmp struct {
		LoginShell  string `json:"loginShell,omitempty"`
		PathType    string `json:"pathType,omitempty"`
		MsysRoot    string `json:"msysRoot,omitempty"`
		WinSymlinks bool   `json:"winSymlinks,omitempty"`
	}

	if err := json.Unmarshal(data, &tmp); err != nil {
		fatal(fmt.Errorf("parse json config failed: %w", err))
	}

	if tmp.LoginShell != "" {
		cfg.LoginShell = tmp.LoginShell
	}
	if tmp.PathType != "" {
		cfg.PathType = tmp.PathType
	}
	cfg.MsysRoot = tmp.MsysRoot
	cfg.WinSymlinks = tmp.WinSymlinks
	return cfg
}

func splitOSArgs() ([]string, []string) {
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

	fs.StringVar(&cfg.MsysRoot, "msysroot", "", "MSYS2 root path")
	fs.StringVar(&cfg.LoginShell, "shell", "", "login shell")
	fs.StringVar(&cfg.PathType, "pathtype", "", "MSYS2_PATH_TYPE (minimal, strict, inherit)")
	fs.StringVar(&cfg.MSystem, "msystem", "", "MSYSTEM (if not inferred from executable name)")
	fs.StringVar(&cfg.Wd, "wd", "", "working directory; not with -home")
	fs.BoolVar(&cfg.UseHome, "home", false, "start in home directory; not with -wd")
	fs.BoolVar(&cfg.WinSymlinks, "winsymlinks", false, "enable winsymlinks")

	if err := fs.Parse(launcherArgs); err != nil {
		fatal(err)
	}

	if fs.NArg() > 0 {
		fs.Usage()
		os.Exit(1)
	}

	return cfg
}

func mergeConfig(base, cli Config) Config {
	if cli.LoginShell != "" {
		base.LoginShell = cli.LoginShell
	}
	if cli.PathType != "" {
		base.PathType = cli.PathType
	}
	if cli.MsysRoot != "" {
		base.MsysRoot = cli.MsysRoot
	}
	if cli.WinSymlinks {
		base.WinSymlinks = true
	}
	if cli.Wd != "" {
		base.Wd = cli.Wd
	}
	if cli.MSystem != "" {
		base.MSystem = cli.MSystem
	}
	if cli.UseHome {
		base.UseHome = true
	}
	return base
}

func resolveMSystem(execName, cli string) string {
	auto := getMSystemFromExecName(execName)
	if auto != "" && cli != "" {
		fatal(fmt.Errorf("conflict: exec name implies %s but -msystem flag provides %s", auto, cli))
	}
	if auto == "" && cli == "" {
		fatal(errors.New("MSYSTEM not specified: rename exe or use -msystem flag"))
	}
	if cli != "" {
		v := getMSystemFromName(cli)
		if v == "" {
			fatal(fmt.Errorf("unsupported MSYSTEM: %s", cli))
		}
		return v
	}
	return auto
}

func validatePathType(pt string) string {
	lower := strings.ToLower(pt)
	if !validPathTypes[lower] {
		fatal(fmt.Errorf("invalid path type '%s'", pt))
	}
	return lower
}

func applyEnv(cfg Config) []string {
	pt := validatePathType(cfg.PathType)
	env := os.Environ()
	env = append(env, "MSYSTEM="+cfg.MSystem)
	if !cfg.UseHome {
		env = append(env, "CHERE_INVOKING=1")
	}
	env = append(env, "MSYS2_PATH_TYPE="+pt)

	msysVal := ""
	if cfg.WinSymlinks {
		msysVal = "winsymlinks:nativestrict"
	}
	env = append(env, "MSYS="+msysVal)
	return env
}

func resolveSpec() Spec {
	execPath, err := os.Executable()
	if err != nil {
		fatal(fmt.Errorf("failed to get launcher path: %w", err))
	}
	execName := filepath.Base(execPath)

	cfg := loadJSONConfig(filepath.Join(filepath.Dir(execPath), "msys2_shell.json"))
	flags, rest := splitOSArgs()
	cli := parseLauncherFlags(flags)
	cfg = mergeConfig(cfg, cli)

	if cfg.UseHome && cfg.Wd != "" {
		fatal(errors.New("exclusive options: -home and -wd cannot be used together"))
	}

	cfg.MSystem = resolveMSystem(execName, cli.MSystem)
	if cfg.MsysRoot == "" {
		fatal(errors.New("missing configuration: msysRoot not specified"))
	}

	if rest == nil {
		rest = []string{}
	}
	return Spec{Cfg: cfg, ShellArgs: rest}
}

func buildCmd(s Spec) *exec.Cmd {
	shellExe := s.Cfg.LoginShell
	if !strings.HasSuffix(strings.ToLower(shellExe), ".exe") {
		shellExe += ".exe"
	}

	shellPath := filepath.Join(s.Cfg.MsysRoot, "usr", "bin", shellExe)
	if _, err := os.Stat(shellPath); err != nil {
		fatal(fmt.Errorf("shell not found at %s: %w", shellPath, err))
	}

	dir := s.Cfg.Wd
	if dir == "" {
		dir, _ = os.Getwd()
	}

	cmd := exec.Command(shellPath, append([]string{"-l"}, s.ShellArgs...)...)
	cmd.Dir = dir
	cmd.Env = applyEnv(s.Cfg)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd
}

func runCmd(cmd *exec.Cmd) {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan)
	go func() {
		for range sigChan {
		}
	}()

	err := cmd.Run()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			os.Exit(exitErr.ExitCode())
		}
		fatal(fmt.Errorf("shell execution failed: %w", err))
	}
}

func main() {
	runCmd(buildCmd(resolveSpec()))
}
