package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/TimLai666/coimnet/bench"
)

type stringList []string

func (l *stringList) String() string     { return strings.Join(*l, ",") }
func (l *stringList) Set(v string) error { *l = append(*l, v); return nil }

func runBenchCompare(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	var candidate, metric, out string
	var baselines stringList
	fs := flag.NewFlagSet("bench compare", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.StringVar(&candidate, "candidate", "", "bench result JSON of the model being judged (required)")
	fs.Var(&baselines, "baseline", "bench result JSON of a baseline; repeat for several (at least one)")
	fs.StringVar(&metric, "metric", "accuracy", "quality metric the verdict uses")
	fs.StringVar(&out, "json", "", "optional new path for the comparison JSON; an existing path is refused")
	usageOutput := &outputCapture{writer: stdout}
	fs.Usage = func() {
		fmt.Fprintln(usageOutput, "Usage: coimnet bench compare --candidate FILE --baseline FILE [--baseline FILE ...] [--metric accuracy] [--json FILE]")
		fmt.Fprintln(usageOutput, "Reads "+bench.SchemaVersion+" results, refuses any baseline measured on another machine, CPU, device, thread count, task or evaluation split, and prints a Markdown table. A candidate wins against a baseline only when its metric is better and its parameter count, whole-process peak RSS and median latency are each no larger.")
		fmt.Fprintln(usageOutput, "Example: coimnet bench compare --candidate coimnet.json --baseline torch-mlp.json")
		fmt.Fprintln(usageOutput, "Errors: a missing --candidate or --baseline, an unreadable or invalid result, mismatched settings, an existing --json path or an output failure.")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); errors.Is(err, flag.ErrHelp) {
		return usageOutput.Err()
	} else if err != nil {
		return err
	}
	if fs.NArg() != 0 || candidate == "" || len(baselines) == 0 {
		fs.Usage()
		return errors.New("bench compare: --candidate and at least one --baseline are required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	c, err := readBenchResult(candidate)
	if err != nil {
		return err
	}
	var bs []bench.Result
	for _, path := range baselines {
		b, err := readBenchResult(path)
		if err != nil {
			return err
		}
		bs = append(bs, b)
	}
	cmp, err := bench.Compare(c, bs, metric)
	if err != nil {
		return fmt.Errorf("bench compare: %w", err)
	}
	if out != "" {
		data, err := json.MarshalIndent(cmp, "", "  ")
		if err != nil {
			return err
		}
		f, err := os.OpenFile(out, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if err != nil {
			return fmt.Errorf("bench compare: %w", err)
		}
		if _, err := f.Write(append(data, '\n')); err != nil {
			f.Close()
			return err
		}
		if err := f.Close(); err != nil {
			return err
		}
	}
	if _, err := io.WriteString(stdout, cmp.Markdown()); err != nil {
		return err
	}
	return usageOutput.Err()
}

func readBenchResult(path string) (bench.Result, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return bench.Result{}, fmt.Errorf("bench compare: %w", err)
	}
	var r bench.Result
	dec := json.NewDecoder(strings.NewReader(string(data)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&r); err != nil {
		return bench.Result{}, fmt.Errorf("bench compare: %s: %w", path, err)
	}
	return r, nil
}
