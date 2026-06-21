use anyhow::{Context, Result, bail};
use lexopt::prelude::*;
use serde::Deserialize;
use std::env;
use std::fs;
use std::path::{Path, PathBuf};
use std::process::{Command, exit};

#[derive(Debug, Default)]
struct Config {
    login_shell: String,
    path_type: String,
    msys_root: String,
    msystem: String,
    wd: String,
    win_symlinks: bool,
    use_home: bool,
}

struct Spec {
    cfg: Config,
    shell_args: Vec<String>,
}

#[derive(Deserialize)]
#[serde(rename_all = "camelCase")]
struct JsonConfig {
    login_shell: Option<String>,
    path_type: Option<String>,
    msys_root: Option<String>,
    win_symlinks: Option<bool>,
}

#[derive(Debug)]
struct CliArgs {
    msys_root: Option<String>,
    login_shell: Option<String>,
    path_type: Option<String>,
    msystem: Option<String>,
    wd: Option<String>,
    use_home: bool,
    win_symlinks: bool,
    shell_args: Vec<String>,
}

impl CliArgs {
    fn parse() -> Result<Self> {
        let mut args = CliArgs {
            msys_root: None,
            login_shell: None,
            path_type: None,
            msystem: None,
            wd: None,
            use_home: false,
            win_symlinks: false,
            shell_args: Vec::new(),
        };

        let mut parser = lexopt::Parser::from_env();
        while let Some(arg) = parser.next()? {
            match arg {
                Long("msysroot") => {
                    args.msys_root = Some(parser.value()?.string()?);
                }
                Long("shell") => {
                    args.login_shell = Some(parser.value()?.string()?);
                }
                Long("pathtype") => {
                    args.path_type = Some(parser.value()?.string()?);
                }
                Long("msystem") => {
                    args.msystem = Some(parser.value()?.string()?);
                }
                Long("wd") => {
                    args.wd = Some(parser.value()?.string()?);
                }
                Long("home") => {
                    args.use_home = true;
                }
                Long("winsymlinks") => {
                    args.win_symlinks = true;
                }
                Long("help") | Short('h') => {
                    println!("Usage: msys2_shell [OPTIONS] [--] [SHELL_ARGS]...");
                    println!();
                    println!("Options:");
                    println!("  --msysroot <DIR>   MSYS2 root path");
                    println!("  --shell <SHELL>    login shell");
                    println!("  --pathtype <TYPE>  MSYS2_PATH_TYPE (minimal, strict, inherit)");
                    println!("  --msystem <SYS>    MSYSTEM (if not inferred from executable name)");
                    println!("  --wd <DIR>         working directory; not with --home");
                    println!("  --home             start in home directory; not with --wd");
                    println!("  --winsymlinks      enable winsymlinks");
                    println!("  -h, --help         Print help");
                    exit(0);
                }
                Value(val) => {
                    args.shell_args.push(val.string()?);
                }
                _ => return Err(anyhow::Error::from(arg.unexpected())),
            }
        }
        Ok(args)
    }
}

fn get_msystem_from_name(name: &str) -> Option<String> {
    match name.to_uppercase().as_str() {
        "MINGW64" => Some("MINGW64".to_string()),
        "MINGW32" => Some("MINGW32".to_string()),
        "UCRT64" => Some("UCRT64".to_string()),
        "CLANG64" => Some("CLANG64".to_string()),
        "CLANGARM64" => Some("CLANGARM64".to_string()),
        "MSYS" | "MSYS2" => Some("MSYS".to_string()),
        _ => None,
    }
}

fn get_msystem_from_exec_name(exec_name: &str) -> Option<String> {
    let base = Path::new(exec_name)
        .file_stem()
        .and_then(|s| s.to_str())
        .unwrap_or(exec_name);
    get_msystem_from_name(base)
}

fn load_json_config<P: AsRef<Path>>(path: P) -> Result<Config> {
    let mut cfg = Config {
        login_shell: "bash".to_string(),
        path_type: "minimal".to_string(),
        ..Default::default()
    };

    let data = match fs::read_to_string(&path) {
        Ok(data) => data,
        Err(e) if e.kind() == std::io::ErrorKind::NotFound => return Ok(cfg),
        Err(e) => return Err(e).context("read config file failed"),
    };

    let tmp: JsonConfig = serde_json::from_str(&data).context("parse json config failed")?;

    if let Some(shell) = tmp.login_shell
        && !shell.is_empty() {
            cfg.login_shell = shell;
        }
    if let Some(pt) = tmp.path_type
        && !pt.is_empty() {
            cfg.path_type = pt;
        }
    if let Some(root) = tmp.msys_root {
        cfg.msys_root = root;
    }
    if let Some(ws) = tmp.win_symlinks {
        cfg.win_symlinks = ws;
    }
    Ok(cfg)
}

fn merge_config(mut base: Config, cli: &CliArgs) -> Config {
    if let Some(shell) = &cli.login_shell
        && !shell.is_empty() {
            base.login_shell = shell.clone();
        }
    if let Some(pt) = &cli.path_type
        && !pt.is_empty() {
            base.path_type = pt.clone();
        }
    if let Some(root) = &cli.msys_root
        && !root.is_empty() {
            base.msys_root = root.clone();
        }
    if cli.win_symlinks {
        base.win_symlinks = true;
    }
    if let Some(wd) = &cli.wd
        && !wd.is_empty() {
            base.wd = wd.clone();
        }
    if let Some(msys) = &cli.msystem
        && !msys.is_empty() {
            base.msystem = msys.clone();
        }
    if cli.use_home {
        base.use_home = true;
    }
    base
}

fn resolve_msystem(exec_name: &str, cli: &str) -> Result<String> {
    let auto = get_msystem_from_exec_name(exec_name);

    if let Some(ref a) = auto
        && !cli.is_empty() {
            bail!(
                "conflict: exec name implies {} but --msystem flag provides {}",
                a,
                cli
            );
        }
    if auto.is_none() && cli.is_empty() {
        bail!("MSYSTEM not specified: rename exe or use --msystem flag");
    }
    if !cli.is_empty() {
        match get_msystem_from_name(cli) {
            Some(v) => return Ok(v),
            None => bail!("unsupported MSYSTEM: {}", cli),
        }
    }
    Ok(auto.unwrap())
}

fn validate_path_type(pt: &str) -> Result<String> {
    let lower = pt.to_lowercase();
    match lower.as_str() {
        "minimal" | "strict" | "inherit" => Ok(lower),
        _ => bail!("invalid path type '{}'", pt),
    }
}

fn resolve_spec() -> Result<Spec> {
    let exec_path = env::current_exe().context("failed to get launcher path")?;
    let exec_name = exec_path
        .file_name()
        .unwrap_or_default()
        .to_string_lossy()
        .into_owned();

    let json_path = exec_path
        .parent()
        .unwrap_or_else(|| Path::new(""))
        .join("msys2_shell.json");
    let mut cfg = load_json_config(&json_path)?;

    let cli = CliArgs::parse()?;
    cfg = merge_config(cfg, &cli);

    if cfg.use_home && !cfg.wd.is_empty() {
        bail!("exclusive options: --home and --wd cannot be used together");
    }

    if cfg.use_home {
        let username = env::var("USERNAME").context("USERNAME not set")?;
        cfg.wd = Path::new(&cfg.msys_root)
            .join("home")
            .join(username)
            .to_string_lossy()
            .into_owned();
    }

    cfg.msystem = resolve_msystem(&exec_name, &cfg.msystem)?;
    if cfg.msys_root.is_empty() {
        bail!("missing configuration: msysRoot not specified");
    }

    Ok(Spec {
        cfg,
        shell_args: cli.shell_args,
    })
}

fn build_cmd(s: &Spec) -> Result<Command> {
    let mut shell_exe = s.cfg.login_shell.clone();
    if !shell_exe.to_lowercase().ends_with(".exe") {
        shell_exe.push_str(".exe");
    }

    let shell_path = Path::new(&s.cfg.msys_root)
        .join("usr")
        .join("bin")
        .join(&shell_exe);
    if !shell_path.exists() {
        bail!("shell not found at {}", shell_path.display());
    }

    let dir = if s.cfg.wd.is_empty() {
        env::current_dir().unwrap_or_else(|_| PathBuf::from("."))
    } else {
        PathBuf::from(&s.cfg.wd)
    };

    let mut cmd = Command::new(&shell_path);
    cmd.arg("-l").args(&s.shell_args);
    cmd.current_dir(dir);

    cmd.env("MSYSTEM", &s.cfg.msystem);
    cmd.env("CHERE_INVOKING", "1");
    cmd.env("MSYS2_PATH_TYPE", validate_path_type(&s.cfg.path_type)?);

    let msys_val = if s.cfg.win_symlinks {
        "winsymlinks:nativestrict"
    } else {
        ""
    };
    cmd.env("MSYS", msys_val);

    Ok(cmd)
}

fn run_cmd(mut cmd: Command) -> Result<()> {
    let status = cmd.status().context("shell execution failed")?;
    if let Some(code) = status.code() {
        exit(code);
    }
    exit(1);
}

fn main() -> Result<()> {
    if let Err(e) = ctrlc::set_handler(|| {}) {
        eprintln!("Warning: Failed to set Ctrl-C handler: {}", e);
    }
    run_cmd(build_cmd(&resolve_spec()?)?)
}
