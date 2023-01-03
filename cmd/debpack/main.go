package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/dzeromsk/debpack"
)

var (
	name       = "app"
	version    = "0.0.1"
	arch       = "amd64"
	maintainer = "unknown"
	desc       = ""
	// arch = "arm64"
)

func main() {

	r, err := debpack.NewDEB(debpack.DEBMetaData{
		Name:        name,
		Version:     version,
		Arch:        arch,
		Maintainer:  maintainer,
		Description: desc,
	})
	if err != nil {
		log.Fatal(err)
	}
	r.AddFile(
		debpack.DEBFile{
			Name:  filepath.Join("/usr/local/bin/", name),
			Mode:  0755,
			Body:  file(filepath.Join(".bin", name)),
			Owner: "root",
			Group: "root",
		})
	// https://unix.stackexchange.com/questions/97289/debian-package-naming-convention
	// package_version_architecture.package-type
	filename := name + "-" + version + "." + arch + ".deb"
	file, err := os.OpenFile(filename, os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Fatalf("OpenFile(%s) err: %v", filename, err)
	}
	defer file.Close()
	if err := r.Write(file); err != nil {
		log.Fatalf("write failed: %v", err)
	}
	fmt.Println(filename)
}

func file(f string) []byte {
	b, err := os.ReadFile(f)
	if err != nil {
		log.Fatalf("ReadFile(%s) err:%s", f, err)
	}
	return b
}
