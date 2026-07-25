---
name: gopssh
description: Safely inspect and run commands on multiple SSH targets with gopssh.
---

# gopssh safe workflow

1. Confirm installation with `command -v gopssh`.
2. Run `gopssh --json doctor` first and inspect every failed check.
3. Validate the target file with `gopssh hosts validate --file <path>`.
4. Preview the exact plan with `gopssh run --dry-run ...`.
5. Prefer JSON for automation and parse stdout separately from stderr.
6. Do not run restart, delete, update, shutdown, or other destructive remote
   commands unless the user explicitly requested that operation and target scope.

Read-only examples:

```bash
gopssh --json doctor --hosts-file hosts.txt
```

```bash
gopssh hosts validate --strict --file hosts.txt
```

```bash
gopssh run --dry-run --hosts-file hosts.txt -- uname -a
```

```bash
gopssh run --json --hosts-file hosts.txt -- df -h
```
