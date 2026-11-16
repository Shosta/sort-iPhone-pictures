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

	"sort-iphone-pictures/internal/tagger"
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
	skipDuplicates := flags.Bool("skip-duplicates", false, "Leave out true duplicates (identical bytes or pixels), also compared to files already in the destination")
	groupSimilar := flags.Bool("group-similar", false, "Put near-identical shots taken close in time into an \"Almost Duplicates\" folder")
	tagScenes := flags.Bool("tag-scenes", false, "Write XMP keyword sidecars for places (plage, montagne, ville, campagne, château, église) using local AI models")
	tagMoment := flags.Bool("tag-moment", false, "Write XMP keywords for sunrise, sunset, night, sun and clouds")
	tagPeople := flags.Bool("tag-people", false, "Write XMP keywords People and Portrait using local face detection")
	aiDir := flags.String("ai-dir", "", "Directory with the AI models (default: $PHOTOSORT_AI_DIR or ./ai next to the executable)")
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
	plan, err := (sorter.Engine{ByLocation: *byLocation, Landmarks: *landmarks, SkipDuplicates: *skipDuplicates, GroupSimilar: *groupSimilar}).Plan(ctx, sources, *destination)
	if err != nil {
		return err
	}
	report := sorter.Report{}
	var copyErr error
	if *apply {
		var afterCopy func(sorter.Entry)
		var runner *sorter.TagRunner
		if *tagScenes || *tagMoment || *tagPeople {
			tg, err := tagger.New(tagger.FindAssets(*aiDir), tagger.Options{Scenes: *tagScenes, Moment: *tagMoment, People: *tagPeople})
			if err != nil {
				fmt.Fprintln(diagnostics, "Tags disabled:", err)
			} else {
				defer tg.Close()
				runner = sorter.NewTagRunner(ctx, tg, plan.Destination, nil)
				afterCopy = runner.Submit
			}
		}
		report, copyErr = sorter.CopyWithHook(ctx, plan, afterCopy, nil)
		if runner != nil {
			runner.Wait()
		}
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
		for _, duplicate := range plan.Duplicates {
			fmt.Fprintf(diagnostics, "Duplicate: %s == %s\n", duplicate.Path, duplicate.Of)
		}
		fmt.Fprintf(output, "Planned: %d | Skipped: %d | Copied: %d\n", len(plan.Entries), len(plan.Skipped), report.Copied)
	}
	return copyErr
}
