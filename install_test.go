package gopssh_test

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestInstaller(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("installer supports macOS and Linux")
	}
	arch := map[string]string{"amd64": "amd64", "arm64": "arm64"}[runtime.GOARCH]
	if arch == "" {
		t.Skip("installer supports amd64 and arm64")
	}

	tempDir := t.TempDir()
	fixtureDir := filepath.Join(tempDir, "fixtures")
	fakeBin := filepath.Join(tempDir, "bin")
	installDir := filepath.Join(tempDir, "install")
	for _, dir := range []string{fixtureDir, fakeBin} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	osName := map[string]string{"linux": "linux", "darwin": "darwin"}[runtime.GOOS]
	asset := fmt.Sprintf("%s-%s.tar.gz", osName, arch)
	assetPath := filepath.Join(fixtureDir, asset)
	writeTestArchive(t, assetPath)
	data, err := os.ReadFile(assetPath)
	if err != nil {
		t.Fatal(err)
	}
	checksum := fmt.Sprintf("%x  %s\n", sha256.Sum256(data), asset)
	if err := os.WriteFile(filepath.Join(fixtureDir, "checksums.txt"), []byte(checksum), 0o600); err != nil {
		t.Fatal(err)
	}

	fakeCurl := `#!/bin/sh
set -eu
url=""
output=""
while [ "$#" -gt 0 ]; do
	case "$1" in
		-o) shift; output="$1" ;;
		http*) url="$1" ;;
	esac
	shift
done
case "$url" in
	*/checksums.txt) cp "$FIXTURE_DIR/checksums.txt" "$output" ;;
	*) cp "$FIXTURE_DIR/` + asset + `" "$output" ;;
esac
`
	curlPath := filepath.Join(fakeBin, "curl")
	if err := os.WriteFile(curlPath, []byte(fakeCurl), 0o755); err != nil {
		t.Fatal(err)
	}

	command := exec.Command("sh", "install.sh")
	command.Env = append(os.Environ(),
		"FIXTURE_DIR="+fixtureDir,
		"GOPSSH_INSTALL_DIR="+installDir,
		"PATH="+fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"),
	)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("install.sh failed: %v\n%s", err, output)
	}
	installed, err := os.ReadFile(filepath.Join(installDir, "gopssh"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(installed)) != "test gopssh binary" {
		t.Fatalf("installed content=%q", installed)
	}
}

func writeTestArchive(t *testing.T, path string) {
	t.Helper()
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	gzipWriter := gzip.NewWriter(file)
	tarWriter := tar.NewWriter(gzipWriter)

	content := []byte("test gopssh binary\n")
	header := &tar.Header{
		Name: "gopssh",
		Mode: 0o755,
		Size: int64(len(content)),
	}
	if err := tarWriter.WriteHeader(header); err != nil {
		t.Fatal(err)
	}
	if _, err := tarWriter.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}
