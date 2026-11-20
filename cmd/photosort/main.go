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
	"path/filepath"
	"strings"
	"syscall"

	"sort-iphone-pictures/internal/remote"
	"sort-iphone-pictures/internal/tagger"
	"sort-iphone-pictures/sorter"
)

type sourcesFlag []string

func (sources *sourcesFlag) String() string { return fmt.Sprint([]string(*sources)) }

func (sources *sourcesFlag) Set(value string) error {
	*sources = append(*sources, value)
	return nil
}

// transport sends sorted trees to a remote destination (replaced in tests).
var transport remote.Transport = remote.Rsync{}

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
	dateFallback := flags.Bool("date-fallback", false, "Date photos without EXIF date from their file name, then their modification time, instead of Photos_Undated")
	screenshots := flags.Bool("screenshots-folder", false, "Put screenshots in a \"Screenshots\" folder")
	blurry := flags.Bool("group-blurry", false, "Put clearly blurry photos in a \"Blurry\" folder for review (nothing is deleted)")
	tagScenes := flags.Bool("tag-scenes", false, "Write XMP keyword sidecars for places (plage, montagne, ville, forêt, intérieur, restaurant, gare...) using local AI models")
	tagMoment := flags.Bool("tag-moment", false, "Write XMP keywords for sunrise, sunset, night, sun and clouds")
	tagPeople := flags.Bool("tag-people", false, "Write XMP keywords People, Portrait and Groupe using local face detection")
	tagMeta := flags.Bool("tag-meta", false, "Write XMP keywords from the metadata: season, year, weekend, selfie, screenshot, panorama, zoom, macro, low light")
	aiDir := flags.String("ai-dir", "", "Directory with the AI models (default: $PHOTOSORT_AI_DIR or ./ai next to the executable)")
	remoteURL := flags.String("remote", "", "Send to a remote library with rsync instead of a local folder: rsync://user@host[:port]/module[/path] (rsync daemon) or ssh://user@host[:port]/path (rsync over SSH)")
	remotePasswordFile := flags.String("remote-password-file", "", "File holding the rsync daemon password (or set PHOTOSORT_REMOTE_PASSWORD); never put it on the command line")
	remoteKey := flags.String("remote-key", "", "SSH private key for ssh:// destinations")
	remoteVerify := flags.Bool("remote-verify", false, "After the upload, compare checksums of the files already on the remote side (nothing is overwritten)")
	remoteDryRun := flags.Bool("remote-dry-run", false, "Simulate the remote upload (rsync --dry-run) and print what would be sent")
	remoteTest := flags.Bool("remote-test", false, "Only test the connection to --remote (rsync --list-only) and exit")
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
	if *remoteURL != "" {
		if *destination != "" {
			return errors.New("use either --destination or --remote, not both")
		}
		if *tagScenes || *tagMoment || *tagPeople || *tagMeta {
			return errors.New("AI tags are written next to local photos and cannot be combined with --remote")
		}
		cfg, err := remote.ParseURL(*remoteURL)
		if err != nil {
			return err
		}
		cfg.KeyFile, cfg.Checksum = *remoteKey, *remoteVerify
		if cfg.Password, err = remotePassword(*remotePasswordFile); err != nil {
			return err
		}
		if *remoteTest {
			res, err := transport.Test(ctx, cfg)
			if err != nil {
				return err
			}
			fmt.Fprintf(output, "OK: %s (%d entries)", cfg.URL(), res.Entries)
			if res.PathMissing {
				fmt.Fprint(output, ", the folder will be created")
			}
			fmt.Fprintln(output)
			return nil
		}
		return runRemote(ctx, cfg, remoteOptions{sources: sources, apply: *apply, dryRun: *remoteDryRun, asJSON: *asJSON, engine: sorter.Engine{ByLocation: *byLocation, Landmarks: *landmarks, SkipDuplicates: *skipDuplicates, GroupSimilar: *groupSimilar, DateFallback: *dateFallback, ScreenshotFolder: *screenshots, GroupBlurry: *blurry}}, output, diagnostics)
	}
	if *remoteTest || *remoteDryRun || *remoteVerify || *remoteKey != "" || *remotePasswordFile != "" {
		return errors.New("--remote-* options need --remote")
	}
	plan, err := (sorter.Engine{ByLocation: *byLocation, Landmarks: *landmarks, SkipDuplicates: *skipDuplicates, GroupSimilar: *groupSimilar, DateFallback: *dateFallback, ScreenshotFolder: *screenshots, GroupBlurry: *blurry}).Plan(ctx, sources, *destination)
	if err != nil {
		return err
	}
	report := sorter.Report{}
	var copyErr error
	if *apply {
		var copied []string
		report, copyErr = sorter.CopyWithProgress(ctx, plan, func(entry sorter.Entry) {
			copied = append(copied, filepath.Join(plan.Destination, entry.Target))
		})
		if (*tagScenes || *tagMoment || *tagPeople || *tagMeta) && len(copied) > 0 {
			tg, err := tagger.New(tagger.FindAssets(*aiDir), tagger.Options{Scenes: *tagScenes, Moment: *tagMoment, People: *tagPeople, Meta: *tagMeta})
			if err != nil {
				fmt.Fprintln(diagnostics, "Tags disabled:", err)
			} else {
				defer tg.Close()
				if err := sorter.TagPhotos(ctx, tg, copied, nil); err != nil {
					copyErr = errors.Join(copyErr, err)
				}
			}
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

func remotePassword(file string) (string, error) {
	if file != "" {
		data, err := os.ReadFile(file)
		if err != nil {
			return "", fmt.Errorf("password file: %w", err)
		}
		return strings.TrimRight(string(data), "\r\n"), nil
	}
	return os.Getenv("PHOTOSORT_REMOTE_PASSWORD"), nil
}

type remoteOptions struct {
	sources []string
	apply   bool
	dryRun  bool
	asJSON  bool
	engine  sorter.Engine
}

// runRemote plans against a scratch folder, then (with --apply or
// --remote-dry-run) stages links and hands them to the transport.
func runRemote(ctx context.Context, cfg remote.Config, opts remoteOptions, output, diagnostics io.Writer) error {
	stage, err := os.MkdirTemp("", "photosort-stage-")
	if err != nil {
		return err
	}
	defer os.Remove(stage)
	plan, err := opts.engine.Plan(ctx, opts.sources, stage)
	if err != nil {
		return err
	}
	var (
		result  remote.Result
		sendErr error
		ran     = opts.apply || opts.dryRun
	)
	if ran {
		items := make([]remote.Item, 0, len(plan.Entries))
		for _, entry := range plan.Entries {
			items = append(items, remote.Item{Source: entry.Source, Target: entry.Target})
		}
		result, sendErr = remote.Send(ctx, transport, cfg, plan.Destination, items, remote.Options{
			DryRun: !opts.apply,
			OnProgress: func(p remote.Progress) {
				if !opts.asJSON {
					fmt.Fprintf(diagnostics, "\r%.0f%% %s %s   ", p.Percent, p.Rate, p.ETA)
				}
			},
		})
	}
	if opts.asJSON {
		out := struct {
			Plan    sorter.Plan    `json:"plan"`
			Remote  string         `json:"remote"`
			Applied bool           `json:"applied"`
			Result  *remote.Result `json:"result,omitempty"`
			Error   string         `json:"error,omitempty"`
		}{Plan: plan, Remote: cfg.URL(), Applied: opts.apply}
		if ran {
			out.Result = &result
		}
		if sendErr != nil {
			out.Error = sendErr.Error()
		}
		encoder := json.NewEncoder(output)
		encoder.SetIndent("", "  ")
		return errors.Join(sendErr, encoder.Encode(out))
	}
	switch {
	case opts.apply:
		fmt.Fprintln(output, "REMOTE COPY")
	case opts.dryRun:
		fmt.Fprintln(output, "REMOTE SIMULATION (nothing is sent)")
	default:
		fmt.Fprintln(output, "PREVIEW (no files written)")
	}
	fmt.Fprintln(output, "Destination:", cfg.URL())
	for _, entry := range plan.Entries {
		fmt.Fprintf(output, "%s -> %s\n", entry.Source, entry.Target)
	}
	for _, issue := range plan.Skipped {
		fmt.Fprintf(diagnostics, "Skipped: %s: %s\n", issue.Path, issue.Reason)
	}
	for _, duplicate := range plan.Duplicates {
		fmt.Fprintf(diagnostics, "Duplicate: %s == %s\n", duplicate.Path, duplicate.Of)
	}
	if sendErr != nil {
		if kind, detail, ok := remote.KindOf(sendErr); ok && kind.Resumable() {
			fmt.Fprintln(diagnostics, "Interrupted; run the same command again to resume.", detail)
		}
		return sendErr
	}
	for _, code := range result.Warnings {
		fmt.Fprintln(diagnostics, "Warning:", code, "(an old rsync is in use; install a recent one, e.g. brew install rsync)")
	}
	for _, name := range result.Mismatched {
		fmt.Fprintln(diagnostics, "Differs on the remote side (left untouched):", name)
	}
	verb := "Sent"
	if !opts.apply {
		verb = "Would send"
	}
	fmt.Fprintf(output, "Planned: %d | Skipped: %d | %s: %d | Already on remote: %d\n", len(plan.Entries), len(plan.Skipped), verb, len(result.Sent), result.Existing)
	return nil
}
