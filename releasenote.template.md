## Installation

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

### macOS


```bash
# x86_64 or Apple silicon (Automatic switching)
brew install masahide/tap/gopssh
```
