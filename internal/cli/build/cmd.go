// Package build implements `kigumi build`: compile a module ahead of time
// through LLVM IR, or print the IR.
package build

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"kigumi/internal/cli/shared"
	"kigumi/internal/cliutil"
	"kigumi/internal/driver"
	"kigumi/internal/errors"
)

func Cmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "build [-o <out>] [--emit-llvm] [--target <triple>] [--reproducible] <module-root|entry.kg> [-- <flags for build.kg>]",
		Short: "Compile a module to a native executable, or an archive for a freestanding or `--sys none` target; a build.kg drives the build instead",
		Args:  cobra.MinimumNArgs(1),
	}
	exe := cmd.Flags().StringP("output", "o", "", "output path (default: a.out, or kigumi.a for a freestanding or --sys none target)")
	emit := cmd.Flags().Bool("emit-llvm", false, "print the LLVM IR instead of linking")
	plan := cmd.Flags().Bool("plan", false, "with a build.kg, print the resolved build graph as JSON and stop")
	outDir := cmd.Flags().String("out-dir", ".", "with a build.kg, the directory artifacts and generated files go to")
	secrets := cmd.Flags().StringArray("secret", nil, "with a build.kg, name=file of a secret a tool step may read")
	reproducible := cmd.Flags().Bool("reproducible", false, "build twice, each with a fresh build dir and compiler cache, and fail if the outputs differ")
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		if *reproducible && *emit {
			return &cliutil.UsageError{Err: errors.NewErr("--reproducible and --emit-llvm are incompatible")}
		}
		if !strings.HasSuffix(args[0], ".kg") {
			if _, hasProgram, err := driver.ReadBuildFile(args[0]); err != nil {
				cmd.PrintErr(err.Error())
				return cliutil.Exit(1)
			} else if hasProgram {
				if *exe != "" || *emit {
					return &cliutil.UsageError{Err: errors.NewErr("a module with build.kg takes --out-dir and --plan, not -o or --emit-llvm")}
				}
				return runProgram(cmd, args[0], args[1:], *plan, *outDir, *secrets, *reproducible)
			}
		}
		if cmd.Flags().Changed("plan") || cmd.Flags().Changed("out-dir") || cmd.Flags().Changed("secret") {
			return &cliutil.UsageError{Err: errors.NewErr("a module without build.kg takes -o or --emit-llvm, not --out-dir, --plan, or --secret")}
		}
		if len(args) > 1 {
			return &cliutil.UsageError{Err: errors.NewErrf("extra arguments after %s belong to a build.kg, which this module has none of", args[0])}
		}
		m, err := shared.Load(cmd, args[0], false, cmd.ErrOrStderr())
		if err != nil {
			return err
		}
		if *emit {
			ir, ok, err := driver.Compile(m, cmd.ErrOrStderr())
			if err != nil {
				return errors.WrapErr(err, "compile module")
			}
			if !ok {
				return cliutil.Exit(1)
			}
			fmt.Fprint(cmd.OutOrStdout(), ir)
			return nil
		}
		target, err := shared.Target(cmd)
		if err != nil {
			return &cliutil.UsageError{Err: err}
		}
		out := *exe
		if out == "" {
			out = "a.out"
			if target.Bare() {
				out = "kigumi.a"
			}
		}
		ok, err := driver.BuildWith(m, out, cmd.ErrOrStderr(), driver.BuildOptions{Target: target, CFlags: shared.CFlags(), Reproducible: *reproducible, Env: shared.CCEnv()})
		if err != nil {
			return errors.WrapErr(err, "build module")
		}
		if !ok {
			return cliutil.Exit(1)
		}
		return nil
	}
	return cmd
}
