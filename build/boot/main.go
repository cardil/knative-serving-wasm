package boot

import (
	"fmt"
	"strings"
	"time"

	"github.com/goyek/goyek/v2"
	"github.com/goyek/goyek/v2/middleware"
	"github.com/goyek/x/color"
	flag "github.com/spf13/pflag"
)

// Params are reusable flags used by the build pipeline.
type Params struct {
	V       bool
	DryRun  bool
	LongRun time.Duration
	NoDeps  bool
	Skip    string
	NoColor bool
	Options []goyek.Option
}

type Option func(*Params)

// Main is an extension of goyek.Main which additionally
// defines flags and uses the most useful middlewares.
func Main(opts ...Option) {
	p := Params{
		LongRun: time.Minute,
	}
	for _, opt := range opts {
		opt(&p)
	}
	flag.BoolVarP(&p.V, "verbose", "v", p.V, "print all tasks as they are run")
	flag.BoolVar(&p.DryRun, "dry-run", p.DryRun, "print all tasks without executing actions")
	flag.DurationVar(&p.LongRun, "long-run", p.LongRun, "print when a task takes longer")
	flag.BoolVar(&p.NoDeps, "no-deps", p.NoDeps, "do not process dependencies")
	flag.StringVarP(&p.Skip, "skip", "s", p.Skip, "skip processing the `comma-separated tasks`")
	flag.BoolVar(&p.NoColor, "no-color", p.NoColor, "disable colorizing output")
	list := flag.Bool("list", false, "just list all targets")
	flag.CommandLine.SetOutput(goyek.Output())
	flag.Usage = usage
	flag.Parse()

	if *list {
		for _, task := range goyek.Tasks() {
			fmt.Println(task.Name())
		}
		return
	}

	if p.DryRun {
		p.V = true // needed to report the task status
	}

	goyek.UseExecutor(color.ReportFlow)

	if p.DryRun {
		goyek.Use(middleware.DryRun)
	}
	goyek.Use(color.ReportStatus)
	if p.V {
		goyek.Use(middleware.BufferParallel)
	} else {
		goyek.Use(middleware.SilentNonFailed)
	}
	if p.LongRun > 0 {
		goyek.Use(middleware.ReportLongRun(p.LongRun))
	}
	if p.NoColor {
		color.NoColor()
	}

	var gopts []goyek.Option
	if p.NoDeps {
		gopts = append(gopts, goyek.NoDeps())
	}
	if p.Skip != "" {
		skippedTasks := strings.Split(p.Skip, ",")
		gopts = append(gopts, goyek.Skip(skippedTasks...))
	}
	for _, option := range p.Options {
		gopts = append(gopts, option)
	}

	goyek.SetUsage(usage)
	goyek.SetLogger(&color.CodeLineLogger{})
	goyek.Main(flag.Args(), gopts...)
}

func usage() {
	fmt.Println("Usage of build: [flags] [--] [tasks]")
	goyek.Print()
	fmt.Println("Flags:")
	flag.PrintDefaults()
}
