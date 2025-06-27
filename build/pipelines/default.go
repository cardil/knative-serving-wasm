package pipelines

import (
	"github.com/cardil/knative-serving-wasm/build/tasks"
	"github.com/cardil/knative-serving-wasm/build/util/dotenv"
	"github.com/goyek/goyek/v2"
)

func Default() *goyek.Flow {
	f := &goyek.Flow{}
	f.UseExecutor(dotenv.Load)
	f.Define(tasks.Clean())
	f.Define(tasks.Deploy())
	f.Define(tasks.Undeploy())
	tasks.Update(f)
	tasks.Test(f)
	f.SetDefault(f.Define(tasks.Build()))
	return f
}
