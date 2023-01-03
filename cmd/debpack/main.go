package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/dzeromsk/debpack"
	"github.com/kelseyhightower/envconfig"
)

type specification struct {
	BinPath    string `required:"true"`
	Name       string `default:"app"`
	Version    string `default:"0.0.1"`
	Arch       string `default:"amd64"`
	Maintainer string `default:"unknown"`
	Desc       string `default:""`
}

func main() {
	var s specification
	err := envconfig.Process("", &s)
	if err != nil {
		log.Fatal(err.Error())
	}
	r, err := debpack.NewDEB(debpack.DEBMetaData{
		Name:        s.Name,
		Version:     s.Version,
		Arch:        s.Arch,
		Maintainer:  s.Maintainer,
		Description: s.Desc,
	})
	if err != nil {
		log.Fatal(err)
	}
	r.AddFile(
		debpack.DEBFile{
			Name:  filepath.Join("/usr/local/bin/", s.Name),
			Mode:  0755,
			Body:  file(s.BinPath),
			Owner: "root",
			Group: "root",
		})
	// https://unix.stackexchange.com/questions/97289/debian-package-naming-convention
	// package_version_architecture.package-type
	filename := s.Name + "-" + s.Version + "." + s.Arch + ".deb"
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
