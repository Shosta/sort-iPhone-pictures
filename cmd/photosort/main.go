package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"sort-iphone-pictures/sorter"
)

type sourcesFlag []string

func (sources *sourcesFlag) String() string { return fmt.Sprint([]string(*sources)) }

func (sources *sourcesFlag) Set(value string) error {
	*sources = append(*sources, value)
	return nil
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, output, diagnostics io.Writer) error {
	flags := flag.NewFlagSet("photosort", flag.ContinueOnError)
	flags.SetOutput(diagnostics)
	var sources sourcesFlag
	flags.Var(&sources, "source", "Source directory (repeat for multiple directories)")
	destination := flags.String("destination", "", "Destination directory")
	apply := flags.Bool("apply", false, "Copy photos; without this flag, only preview the plan")
	landmarks := flags.Bool("landmarks", false, "With --by-location, use the name of a nearby landmark (museum, castle, stadium) instead of the city")
	byLocation := flags.Bool("by-location", false, "Add a city folder inside each month when GPS data is available")
	asJSON := flags.Bool("json", false, "Print the plan and result as JSON")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("unexpected positional arguments; use --source and --destination")
	}
	plan, err := (sorter.Engine{ByLocation: *byLocation, Landmarks: *landmarks}).Plan(ctx, sources, *destination)
	if err != nil {
		return err
	}
	report := sorter.Report{}
	var copyErr error
	if *apply {
		report, copyErr = sorter.Copy(ctx, plan)
	}
	if *asJSON {
		result := struct {
			Plan    sorter.Plan `json:"plan"`
			Applied bool        `json:"applied"`
			Copied  int         `json:"copied"`
			Error   string      `json:"error,omitempty"`
		}{Plan: plan, Applied: *apply, Copied: report.Copied}
		if copyErr != nil {
			result.Error = copyErr.Error()
		}
		encoder := json.NewEncoder(output)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(result); err != nil {
			return errors.Join(copyErr, err)
		}
	} else {
		mode := "PREVIEW (no files written)"
		if *apply {
			mode = "COPY"
		}
		fmt.Fprintln(output, mode)
		fmt.Fprintln(output, "Destination:", plan.Destination)
		for _, entry := range plan.Entries {
			fmt.Fprintf(output, "%s -> %s\n", entry.Source, entry.Target)
		}
		for _, issue := range plan.Skipped {
			fmt.Fprintf(diagnostics, "Skipped: %s: %s\n", issue.Path, issue.Reason)
		}
		fmt.Fprintf(output, "Planned: %d | Skipped: %d | Copied: %d\n", len(plan.Entries), len(plan.Skipped), report.Copied)
	}
	return copyErr
}
