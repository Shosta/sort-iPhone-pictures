package desktop

import (
	"context"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"sort-iphone-pictures/sorter"
)

func TestFileTree(t *testing.T) {
	root := t.TempDir()
	tree := newFileTree()
	photo := filepath.Join(root, "nested", "photo.png")
	tree.add(root, photo, photo)
	tree.add(root, photo, photo)
	tree.add(root, filepath.Join(root, "other.png"), "source")
	tree.add(root, filepath.Join(root, "..", "outside.png"), "")
	if len(tree.roots) != 1 || len(tree.nodes) != 4 || len(tree.nodes[root].children) != 2 {
		t.Fatalf("unexpected tree: %+v", tree)
	}
	if tree.nodes[root].children[0] != filepath.Join(root, "nested") {
		t.Fatal("folders must precede files")
	}
	if tree.nodes[photo].preview != photo || tree.nodes[photo].folder {
		t.Fatal("incorrect leaf")
	}
}

func TestThumbnail(t *testing.T) {
	path := filepath.Join(t.TempDir(), "photo.png")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	source := image.NewNRGBA(image.Rect(0, 0, 200, 100))
	source.Set(0, 0, color.NRGBA{R: 255, A: 255})
	if err := png.Encode(file, source); err != nil {
		t.Fatal(err)
	}
	file.Close()
	result, err := thumbnail(path)
	if err != nil || result.Bounds().Dx() != 96 || result.Bounds().Dy() != 48 {
		t.Fatalf("unexpected thumbnail: %v, %v", result, err)
	}
	rotated := orient(result, 6)
	if rotated.Bounds().Dx() != 48 || rotated.Bounds().Dy() != 96 {
		t.Fatal("incorrect orientation")
	}
}

func TestWindowCopyAndRender(t *testing.T) {
	application := test.NewApp()
	application.Settings().SetTheme(Theme())
	defer application.Quit()
	view := newDesktop(application)
	defer func() {
		view.window.SetCloseIntercept(nil)
		view.window.Close()
	}()
	base := t.TempDir()
	source := os.Getenv("PHOTOSORT_PREVIEW_SOURCE")
	if source == "" {
		source = filepath.Join(base, "Photos")
		if err := os.Mkdir(source, 0755); err != nil {
			t.Fatal(err)
		}
		file, err := os.Create(filepath.Join(source, "photo.png"))
		if err != nil {
			t.Fatal(err)
		}
		if err := png.Encode(file, image.NewNRGBA(image.Rect(0, 0, 120, 80))); err != nil {
			t.Fatal(err)
		}
		file.Close()
		view.engine.ReadMetadata = func(context.Context, string) (sorter.Metadata, error) {
			return sorter.Metadata{TakenAt: time.Date(2026, 10, 21, 0, 0, 0, 0, time.UTC), Camera: "iPhone"}, nil
		}
	}
	destination := filepath.Join(base, "Classement")
	plan, err := view.engine.Plan(context.Background(), []string{source}, destination)
	if err != nil || len(plan.Entries) == 0 {
		t.Fatalf("no entries: %+v, %v", plan, err)
	}
	canonicalSource, err := filepath.EvalSymlinks(source)
	if err != nil {
		t.Fatal(err)
	}
	view.sources = []string{canonicalSource}
	view.destination.OnChanged = nil
	view.destination.SetText(destination)
	view.showPlan(plan)
	if !view.leftWidget.IsBranch("") || len(view.leftWidget.ChildUIDs("")) == 0 {
		t.Fatal("tree root must expose visible branches")
	}
	if view.start.Disabled() || len(view.right.nodes) != 0 || len(view.left.nodes) == 0 {
		t.Fatal("incorrect state before copying")
	}
	for _, entry := range plan.Entries {
		preview, err := thumbnail(entry.Source)
		if err != nil {
			t.Fatal(err)
		}
		view.previews[entry.Source] = preview
	}
	view.leftWidget.Refresh()
	report, err := sorter.CopyWithProgress(context.Background(), plan, func(entry sorter.Entry) {
		view.showCopied(plan, entry)
	})
	if err != nil || report.Copied != len(plan.Entries) || view.copied != report.Copied {
		t.Fatalf("incorrect UI copy result: %+v, %v", report, err)
	}
	for _, entry := range plan.Entries {
		path := filepath.Join(plan.Destination, entry.Target)
		if view.right.nodes[path] == nil {
			t.Fatalf("missing copied leaf %s", path)
		}
		if _, err := os.Stat(path); err != nil {
			t.Fatal(err)
		}
	}
	view.status.SetText("Tri terminé")
	view.window.Show()
	for _, width := range []float32{1180, 900} {
		view.window.Resize(fyne.NewSize(width, 760))
		if view.leftWidget.Size().Width < 200 || view.rightWidget.Size().Width < 200 {
			t.Fatal("panes are too narrow")
		}
		if output := os.Getenv("PHOTOSORT_SCREENSHOT"); output != "" {
			if width == 900 {
				output += ".compact.png"
			}
			file, err := os.Create(output)
			if err != nil {
				t.Fatal(err)
			}
			if err := png.Encode(file, view.window.Canvas().Capture()); err != nil {
				t.Fatal(err)
			}
			file.Close()
		}
	}
}
