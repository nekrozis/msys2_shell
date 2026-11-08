# MSYS2 Launcher (Rust)

An MSYS2 environment launcher written in Rust.

---

## Features

* Detects `MSYSTEM` from the executable name
* Reads configuration from `msys2_shell.conf`
* Allows command-line options to override configuration
* Passes arguments to the shell via `--`

---

## Build

Requires Rust.

```bash
cargo build --release
````

Rename or copy the executable to select the environment:

```
mingw64.exe   → MSYSTEM=MINGW64
ucrt64.exe    → MSYSTEM=UCRT64
clang64.exe   → MSYSTEM=CLANG64
msys2.exe     → MSYSTEM=MSYS
```

---

## Configuration

Optional. The launcher reads `msys2_shell.conf` from the same directory as the
executable.

### Fields

| Key            | Type   | Description                       | Default   |
| -------------- | ------ | --------------------------------- | --------- |
| `msys_root`    | string | Path to MSYS2 installation        | (empty)   |
| `login_shell`  | string | Shell file name under `/usr/bin`  | `bash`    |
| `path_type`    | string | `minimal`, `strict`, `inherit`    | `minimal` |
| `win_symlinks` | bool   | Enable `winsymlinks:nativestrict` | `false`   |

Example:

```ini
# msys2_shell.conf
msys_root = C:\msys64
login_shell = bash
path_type = minimal
win_symlinks = false
```

Lines starting with `#` or `;` are comments. Values may be quoted when they
contain spaces. Unknown keys are rejected. The old `msys2_shell.json` is no
longer read; migrate it to `msys2_shell.conf`.

---

## Command-line options

Command-line flags override the config file.

```
--msysroot string
        MSYS2 root path

--shell string
        login shell

--pathtype string
        MSYS2_PATH_TYPE (minimal, strict, inherit)

--msystem string
        MSYSTEM (if not inferred from executable name)

--wd string
        working directory; not with --home

--home
        start in home directory; not with --wd
```

Positional arguments are passed to the shell. Use `--` before arguments that start with `-`, for example `-c`.

---

## Usage examples

Start an interactive shell:

```powershell
.\mingw64.exe
```

Run a command:

```powershell
.\ucrt64.exe -- -c "pacman -Syu"
```

Specify environment explicitly:

```powershell
.\msys2_launcher.exe --msystem CLANG64
```

---

## Environment behavior

The launcher sets:

* `MSYSTEM`
* `MSYS2_PATH_TYPE`
* `MSYS`
* `CHERE_INVOKING=1` unless `-home` is used

---

## License

MIT
