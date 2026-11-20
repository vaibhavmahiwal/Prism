package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func usage(w io.Writer) {
	_, _ = io.WriteString(w, `Usage: mini-ci run [flags]

Flags:
  -f, --file string        Path to pipeline file (default ".mini-ci.yml")
  -w, --workspace string   Local workspace directory (default current directory)
      --repo string        Git repository URL to clone (requires --sha)
      --sha string         Git commit SHA to check out
      --keep               Keep cloned temporary workspace after run

Examples:
  mini-ci run
  mini-ci run -f ci/pipeline.yml -w ./my-app
  mini-ci run --repo https://github.com/org/repo.git --sha a1b2c3d --keep
`)
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		usage(stdout)
		return 0
	}
	switch args[0] {
	case "help", "-h", "--help":
		usage(stdout)
		return 0
	case "run":
		return runCommand(args[1:], stdout, stderr)
	default:
		fmt.Fprintf(stderr, "unknown command %q\n", args[0])
		return 2
	}
}

type runOptions struct {
	file      string
	workspace string
	repo      string
	sha       string
	keep      bool
}

//function to parse the flags for the run command,
//  it takes the args and a writer to write errors to
func parseRunFlags(args []string, _io.Writer)(runOptions,error){
   var opts runOptions
   fs:=flag.NewFlagSet("run",flag.ContinueOnError)
   fs.SetOutput(_io)
   fs.Usage=func(){}

   //this are the flags we want to parse, we can use the flag package to define them
   fs.StringVar(&opts.file,"file",".mini-ci.yml","Path to pipeline file")
   fs.StringVar(&opts.file, "f", ".mini-ci.yml", "")
   fs.StringVar(&opts.workspace, "workspace", "", "")
   fs.StringVar(&opts.workspace, "w", "", "")
   fs.StringVar(&opts.repo, "repo", "", "")
   fs.StringVar(&opts.sha, "sha", "", "")
   fs.BoolVar(&opts.keep, "keep", false, "")

	//parse the flags
	if err:=fs.Parse(args);err!=nil{
		return runOptions{},err
	}
	//check if repo is set but sha is not
	if fs.NArg()>0{
		return runOptions{},fmt.Errorf("unexpected arguments: %v",fs.Args(0))
	}
    //this is a validation step to ensure that the flags are used correctly
		switch {
	case opts.repo != "" && opts.sha == "":
		return runOptions{}, errors.New("--repo requires --sha")
	case opts.sha != "" && opts.repo == "":
		return runOptions{}, errors.New("--sha requires --repo")
	case opts.repo != "" && opts.workspace != "":
		return runOptions{}, errors.New("--repo and --workspace cannot be used together")
	}
	return opts,nil
}
//this dunctions runs the comand
//it takes the args and the stdout and stderr writers
func runCommand(args []string, stdout, stderr io.Writer) int {
	opts,err:=parseRunFlags(args,stderr)

	if errord.Is(err,flag.ErrHelp){
		usage(stdout)
		return 0
	}
	if err!=nil{
		fmt.Fprintf(stderr,"mini-ci:%v\n",err)
		return 2
	}

	ctx,stop:=signal.NotifyContext(context.Background(),os.Interrupt)
	defer stop()

	dir, fileBase, cleanup, err := resolveWorkspace(ctx, opts, stderr)
		if err != nil {
		fmt.Fprintf(stderr, "mini-ci: %v\n", err)
		if ctx.Err() != nil {
			return 130
		}
		return 2
	}
	defer cleanup()
	filePath := opts.file
	if !filepath.IsAbs(filePath) {
		filePath = filepath.Join(fileBase, filePath)
	}
    
	p, err := pipeline.ParseFile(filePath)
	if err != nil {
		fmt.Fprintf(stderr, "mini-ci: %v\n", err)
		if ctx.Err() != nil {
			return 130
		}
		return 2
	}

	ex, err := executor.NewDocker()
	if err != nil {
		fmt.Fprintf(stderr, "mini-ci: %v\n", err)
		return 2
	}
	defer func() { _ = ex.Close() }()

	res, err := runner.Run(ctx, p, ex, runner.Options{
		Workspace: dir,
		Stdout:    stdout,
		Stderr:    stderr,
	})
	if err != nil {
		fmt.Fprintf(stderr, "mini-ci: %v\n", err)
		if ctx.Err() != nil {
			return 130
		}
		return 2
	}

	return reportResult(res, stdout, stderr)
}