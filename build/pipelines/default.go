package pipelines

import (
	"github.com/cardil/knative-serving-wasm/build/tasks"
	"github.com/goyek/goyek/v2"
)

func Default() *goyek.Flow {
	f := &goyek.Flow{}
	f.Define(tasks.Clean())
	f.Define(tasks.Deploy())
	f.Define(tasks.Undeploy())
	f.Define(tasks.UpdateDeps())
	f.Define(tasks.UpdateCodegen())
	// needs to be after codegen and deps
	f.Define(tasks.Update(f))
	f.SetDefault(f.Define(tasks.Build()))
	return f
}
