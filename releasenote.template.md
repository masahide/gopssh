## Installation

### macOS / Linux

```bash
curl -fsSL https://raw.githubusercontent.com/masahide/gopssh/main/install.sh | sh
```

To uninstall:

```bash
curl -fsSL https://raw.githubusercontent.com/masahide/gopssh/main/uninstall.sh | sh
```

### Linux

For RHEL/CentOS:


```bash
# x86_64
sudo yum install https://github.com/masahide/gopssh/releases/latest/download/__amd64rpm__

# ARM
sudo yum install https://github.com/masahide/gopssh/releases/latest/download/__arm64rpm__
```


For Ubuntu/Debian:

```bash
# x86_64
wget -qO /tmp/gopssh.deb https://github.com/masahide/gopssh/releases/latest/download/__amd64deb__
sudo dpkg -i /tmp/gopssh.deb

# ARM
wget -qO /tmp/gopssh.deb https://github.com/masahide/gopssh/releases/latest/download/__arm64deb__
sudo dpkg -i /tmp/gopssh.deb
```
