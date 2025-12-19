# MSYS2 Launcher (Go)

An MSYS2 environment launcher written in Go.

---

## Features

* Detects `MSYSTEM` from the executable name
* Reads configuration from `msys2_shell.json`
* Allows command-line options to override configuration
* Passes arguments to the shell via `--`

---

## Build

Requires Go.

```bash
go build -o msys2_launcher.exe
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

The launcher reads `msys2_shell.json` from the same directory as the executable.

### JSON fields

| Key           | Type   | Description                       | Default   |
| ------------- | ------ | --------------------------------- | --------- |
| `msysRoot`    | string | Path to MSYS2 installation        | (empty)   |
| `loginShell`  | string | Shell under `/usr/bin`            | `bash`    |
| `pathType`    | string | `minimal`, `strict`, `inherit`    | `minimal` |
| `winSymlinks` | bool   | Enable `winsymlinks:nativestrict` | `false`   |

Example:

```json
{
  "msysRoot": "C:\\msys64",
  "loginShell": "bash",
  "pathType": "minimal",
  "winSymlinks": false
}
```

---

## Command-line options

Command-line flags override JSON configuration.

```
-msysroot string
        MSYS2 root path

-shell string
        login shell

-pathtype string
        MSYS2_PATH_TYPE (minimal, strict, inherit)

-msystem string
        MSYSTEM (if not inferred from executable name)

-wd string
        working directory; not with -home

-home
        start in home directory; not with -wd
```

Arguments after `--` are passed to the shell.

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
.\msys2_launcher.exe -msystem CLANG64
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
