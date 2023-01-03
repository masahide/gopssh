package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/google/rpmpack"
)

var (
	name    = "app"
	version = "0.0.1"
	hash    = ""
	release = "el7"
	arch    = "x86_64"
	// arch = "noarch"
)

func main() {

	sign := flag.Bool("sign", false, "sign the package with a fake sig")
	flag.Parse()

	if len(hash) > 0 {
		release = hash + "." + release
	}
	r, err := rpmpack.NewRPM(rpmpack.RPMMetaData{
		Name:    name,
		Version: version,
		Release: release,
		Arch:    arch,
	})
	if err != nil {
		log.Fatal(err)
	}
	r.AddFile(
		rpmpack.RPMFile{
			Name:  filepath.Join("/usr/local/bin/", name),
			Mode:  0755,
			Body:  file(filepath.Join(".bin", name)),
			Owner: "root",
			Group: "root",
		})
	if *sign {
		r.SetPGPSigner(func([]byte) ([]byte, error) {
			return []byte(`this is not a signature`), nil
		})
	}
	// http://ftp.rpm.org/max-rpm/ch-rpm-file-format.html
	// ex: name-version-release.architecture.rpm
	filename := name + "-" + version + "-" + release + "." + arch + ".rpm"
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
