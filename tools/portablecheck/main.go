// Command portablecheck runs the existing test suite against a private bundled
// PostgreSQL instance, without installing a database or touching development data.
package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/Autumn-27/artex/internal/localpostgres"
)

func run() int {
	runtimeDir := flag.String("runtime", "", "unpacked PostgreSQL runtime directory")
	packages := flag.String("packages", "./...", "comma-separated packages to test")
	workdir := flag.String("workdir", "", "checkout to test (defaults to current directory)")
	filter := flag.String("run", "", "optional Go test name filter")
	flag.Parse()
	data, err := os.MkdirTemp("", "artex-test-db-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer os.RemoveAll(data)
	s, err := localpostgres.Start(data, *runtimeDir)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer s.Close()
	goCommand := filepath.Join(runtime.GOROOT(), "bin", "go")
	if os.PathSeparator == '\\' {
		goCommand += ".exe"
	}
	// The legacy DB suites share mutable fixtures. Keep package execution serial
	// in addition to their advisory locks, including on fresh Windows clusters.
	args := append([]string{"test", "-p", "1"}, strings.Split(*packages, ",")...)
	args = append(args, "-count=1", "-timeout=15m")
	if *filter != "" {
		args = append(args, "-run", *filter)
	}
	cmd := exec.Command(goCommand, args...)
	cmd.Dir = *workdir
	cmd.Env = append(os.Environ(), "ARTEX_PG_DSN="+s.DSN(), "ARTEX_TEST_POSTGRES_DIR="+*runtimeDir)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}

func main() { os.Exit(run()) }
