package main

import (
	"os"

	"github.com/cardil/knative-serving-wasm/build/pipelines"
	"github.com/cardil/knative-serving-wasm/build/util/fs"
	"github.com/goyek/goyek/v2"
	"github.com/goyek/x/boot"
)

func main() {
	if err := os.Chdir(fs.RootDir()); err != nil {
		panic(err)
	}
	goyek.DefaultFlow = pipelines.Default()
	boot.Main()
}
