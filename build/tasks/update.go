package tasks

import (
	"fmt"
	"slices"

	"github.com/goyek/goyek/v2"
)

func Update(flow *goyek.Flow) goyek.Task {
	deps := goyek.Deps{}
	names := []string{
		"update-deps",
		"update-codegen",
	}
	for _, task := range flow.Tasks() {
		if slices.Contains(names, task.Name()) {
			deps = append(deps, task)
		}
	}
	if len(deps) != len(names) {
		panic(fmt.Errorf("couldn't find all the tasks"))
	}
	return goyek.Task{
		Name:  "update",
		Usage: "Update project",
		Deps:  deps,
	}
}
