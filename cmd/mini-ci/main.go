package main

import (
	"io"
	"os"
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
  mini-ci run --repo https://github.com/vaibhavmahiwal/Prism.git --sha a1b2c3d --keep
`)
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args)==0{
		usage(stderr)
		return 1
	}
	switch args[0]{
	case "help" ,"-h", "--help":
		usage(stdout)
		return 0
	}
    case "run":
		return runCommand(args[1:], stdout, stderr)
	default:
	      fmt.Printf(stderr,"Unknown command:%q", args[0])
          usage(stderr)
		  return 1
	}
	  
