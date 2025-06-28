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
	tasks.Deploy(f)
	tasks.Update(f)
	tasks.Test(f)
	f.SetDefault(f.Define(tasks.Build()))
	return f
}
