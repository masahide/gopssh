package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/google/rpmpack"
	"github.com/kelseyhightower/envconfig"
)

type specification struct {
	BinPath string `required:"true"`
	Name    string `default:"app"`
	Version string `default:"0.0.1"`
	Hash    string `default:""`
	Release string `default:"el7"`
	Arch    string `default:"x86_64"`
	Sign    string
}

func main() {
	var s specification
	err := envconfig.Process("", &s)
	if err != nil {
		log.Fatal(err.Error())
	}
	if len(s.Hash) > 0 {
		s.Release = s.Hash + "." + s.Release
	}
	r, err := rpmpack.NewRPM(rpmpack.RPMMetaData{
		Name:    s.Name,
		Version: s.Version,
		Release: s.Release,
		Arch:    s.Arch,
	})
	if err != nil {
		log.Fatal(err)
	}
	r.AddFile(
		rpmpack.RPMFile{
			Name:  filepath.Join("/usr/local/bin/", s.Name),
			Mode:  0755,
			Body:  file(s.BinPath),
			Owner: "root",
			Group: "root",
		})
	if len(s.Sign) > 0 {
		r.SetPGPSigner(func([]byte) ([]byte, error) {
			return []byte(s.Sign), nil
		})
	}
	// http://ftp.rpm.org/max-rpm/ch-rpm-file-format.html
	// ex: name-version-release.architecture.rpm
	filename := s.Name + "-" + s.Version + "-" + s.Release + "." + s.Arch + ".rpm"
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
