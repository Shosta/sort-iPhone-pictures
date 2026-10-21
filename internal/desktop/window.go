package desktop

import (
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"sort-iphone-pictures/sorter"
)

type desktop struct {
	app         fyne.App
	window      fyne.Window
	engine      sorter.Engine
	sources     []string
	plan        sorter.Plan
	left        *fileTree
	right       *fileTree
	leftWidget  *widget.Tree
	rightWidget *widget.Tree
	previews    map[string]image.Image
	destination *widget.Entry
	add         *widget.Button
	clear       *widget.Button
	choose      *widget.Button
	start       *widget.Button
	cancel      *widget.Button
	warnings    *widget.Button
	sourceCount *widget.Label
	targetCount *widget.Label
	status      *widget.Label
	selection   *widget.Label
	progress    *widget.ProgressBar
	loading     *widget.ProgressBarInfinite
	issues      []sorter.Issue
	generation  int
	copied      int
	copying     bool
	analyzing   bool
	completed   bool
	cancelPlan  context.CancelFunc
	cancelCopy  context.CancelFunc
}

func NewWindow(application fyne.App) fyne.Window {
	return newDesktop(application).window
}

func newDesktop(application fyne.App) *desktop {
	view := &desktop{app: application, left: newFileTree(), right: newFileTree(), previews: make(map[string]image.Image)}
	view.window = application.NewWindow("Photosort")
	view.window.Resize(fyne.NewSize(1180, 760))
	view.leftWidget = view.newTree(true)
	view.rightWidget = view.newTree(false)
	view.sourceCount = widget.NewLabel("0 photo")
	view.targetCount = widget.NewLabel("0 copie")
	view.status = widget.NewLabel("Prêt")
	view.selection = widget.NewLabel("")
	view.selection.Truncation = fyne.TextTruncateEllipsis
	view.progress = widget.NewProgressBar()
	view.loading = widget.NewProgressBarInfinite()
	view.loading.Hide()
	view.add = widget.NewButtonWithIcon("Ajouter", theme.FolderNewIcon(), view.chooseSource)
	view.clear = widget.NewButtonWithIcon("Vider", theme.DeleteIcon(), func() {
		view.sources = nil
		view.analyze()
	})
	view.start = widget.NewButtonWithIcon("Trier", theme.NavigateNextIcon(), view.copy)
	view.start.Importance = widget.HighImportance
	view.cancel = widget.NewButtonWithIcon("Annuler", theme.CancelIcon(), view.stop)
	view.warnings = widget.NewButtonWithIcon("Signalements", theme.WarningIcon(), view.showIssues)
	view.warnings.Disable()
	view.destination = widget.NewEntry()
	view.destination.PlaceHolder = "Dossier de destination"
	home, _ := os.UserHomeDir()
	view.destination.SetText(application.Preferences().StringWithFallback("destination", filepath.Join(home, "Pictures", "Photos classees")))
	view.destination.OnChanged = func(string) {
		view.right = newFileTree()
		view.rightWidget.Refresh()
		view.targetCount.SetText("0 copie")
		view.analyze()
	}
	view.choose = widget.NewButtonWithIcon("Choisir", theme.FolderOpenIcon(), view.chooseDestination)
	leftHeader := container.NewBorder(nil, nil, widget.NewLabelWithStyle("Sources", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}), view.sourceCount)
	rightHeader := container.NewBorder(nil, nil, widget.NewLabelWithStyle("Destination", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}), view.targetCount)
	destinationBar := container.NewBorder(nil, nil, nil, view.choose, view.destination)
	left := container.NewBorder(container.NewVBox(leftHeader, container.NewHBox(view.add, view.clear), widget.NewSeparator()), nil, nil, nil, view.leftWidget)
	right := container.NewBorder(container.NewVBox(rightHeader, destinationBar, widget.NewSeparator()), nil, nil, nil, view.rightWidget)
	center := container.NewVBox(layout.NewSpacer(), view.start, view.cancel, layout.NewSpacer())
	panes := container.New(dualPaneLayout{}, left, center, right)
	header := container.NewBorder(nil, nil, widget.NewLabelWithStyle("Photosort", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}), view.warnings)
	footer := container.NewVBox(container.NewStack(view.progress, view.loading), container.NewBorder(nil, nil, view.status, nil, view.selection))
	view.window.SetContent(container.NewPadded(container.NewBorder(container.NewVBox(header, widget.NewSeparator()), footer, nil, nil, panes)))
	view.window.SetOnDropped(func(_ fyne.Position, items []fyne.URI) {
		if view.copying {
			return
		}
		var paths []string
		for _, item := range items {
			if item.Scheme() == "file" {
				paths = append(paths, item.Path())
			}
		}
		view.addSources(paths)
	})
	view.window.SetCloseIntercept(func() {
		closeWindow := func() {
			view.stop()
			view.window.SetCloseIntercept(nil)
			view.window.Close()
		}
		if view.copying {
			dialog.ShowConfirm("Copie en cours", "Arrêter la copie et fermer la fenêtre ?", func(confirmed bool) {
				if confirmed {
					closeWindow()
				}
			}, view.window)
		} else {
			closeWindow()
		}
	})
	view.updateControls()
	return view
}

func (view *desktop) chooseSource() {
	dialog.ShowFolderOpen(func(folder fyne.ListableURI, err error) {
		if err != nil {
			dialog.ShowError(err, view.window)
		} else if folder != nil {
			view.addSources([]string{folder.Path()})
		}
	}, view.window)
}

func (view *desktop) chooseDestination() {
	dialog.ShowFolderOpen(func(folder fyne.ListableURI, err error) {
		if err != nil {
			dialog.ShowError(err, view.window)
		} else if folder != nil {
			view.destination.SetText(folder.Path())
		}
	}, view.window)
}

func (view *desktop) addSources(paths []string) {
	var failures []string
	for _, path := range paths {
		resolved, err := filepath.EvalSymlinks(path)
		if err == nil {
			resolved, err = filepath.Abs(resolved)
		}
		if err != nil {
			failures = append(failures, path)
			continue
		}
		info, err := os.Stat(resolved)
		if err != nil || !info.IsDir() {
			failures = append(failures, path)
			continue
		}
		covered := false
		for _, source := range view.sources {
			if localTo(source, resolved) {
				covered = true
			}
		}
		if covered {
			continue
		}
		var remaining []string
		for _, source := range view.sources {
			if !localTo(resolved, source) {
				remaining = append(remaining, source)
			}
		}
		view.sources = append(remaining, resolved)
	}
	sort.Strings(view.sources)
	view.analyze()
	if len(failures) > 0 {
		dialog.ShowError(fmt.Errorf("dossiers non accessibles :\n%s", strings.Join(failures, "\n")), view.window)
	}
}

func localTo(parent, path string) bool {
	relative, err := filepath.Rel(parent, path)
	return err == nil && (relative == "." || filepath.IsLocal(relative))
}

func (view *desktop) analyze() {
	if view.cancelPlan != nil {
		view.cancelPlan()
	}
	view.generation++
	generation := view.generation
	view.plan = sorter.Plan{}
	view.completed = false
	view.left = newFileTree()
	view.leftWidget.Refresh()
	preserved := make(map[string]image.Image)
	for _, node := range view.right.nodes {
		if preview := view.previews[node.preview]; preview != nil {
			preserved[node.preview] = preview
		}
	}
	view.previews = preserved
	view.issues = nil
	view.updateWarnings()
	view.sourceCount.SetText("0 photo")
	view.analyzing = len(view.sources) > 0 && strings.TrimSpace(view.destination.Text) != ""
	view.updateControls()
	if !view.analyzing {
		view.loading.Hide()
		view.status.SetText("Prêt")
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	view.cancelPlan = cancel
	sources := append([]string(nil), view.sources...)
	destination := view.destination.Text
	view.loading.Show()
	view.status.SetText("Analyse en cours…")
	go func() {
		timer := time.NewTimer(300 * time.Millisecond)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
		plan, err := view.engine.Plan(ctx, sources, destination)
		fyne.Do(func() {
			if generation != view.generation || ctx.Err() != nil {
				return
			}
			view.analyzing = false
			view.loading.Hide()
			if err != nil {
				view.status.SetText("Analyse impossible")
				view.issues = []sorter.Issue{{Path: destination, Reason: err.Error()}}
				view.updateWarnings()
				view.updateControls()
				return
			}
			view.showPlan(plan)
			view.app.Preferences().SetString("destination", destination)
			go view.loadPreviews(ctx, generation, plan)
		})
	}()
}

func (view *desktop) showPlan(plan sorter.Plan) {
	view.plan = plan
	view.left = newFileTree()
	view.issues = append([]sorter.Issue(nil), plan.Skipped...)
	for _, entry := range plan.Entries {
		for _, source := range view.sources {
			if localTo(source, entry.Source) {
				view.left.add(source, entry.Source, entry.Source)
				break
			}
		}
	}
	view.leftWidget.Refresh()
	view.leftWidget.OpenAllBranches()
	view.sourceCount.SetText(fmt.Sprintf("%d photos", len(plan.Entries)))
	view.status.SetText(fmt.Sprintf("%d photos prêtes", len(plan.Entries)))
	view.updateWarnings()
	view.updateControls()
}

func (view *desktop) loadPreviews(ctx context.Context, generation int, plan sorter.Plan) {
	jobs := make(chan string)
	var workers sync.WaitGroup
	for worker := 0; worker < 2; worker++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for path := range jobs {
				if ctx.Err() != nil {
					return
				}
				preview, err := thumbnail(path)
				fyne.Do(func() {
					if generation != view.generation || ctx.Err() != nil {
						return
					}
					if err != nil {
						view.issues = append(view.issues, sorter.Issue{Path: path, Reason: err.Error()})
						view.updateWarnings()
					} else {
						view.previews[path] = preview
						view.leftWidget.RefreshItem(path)
						for id, node := range view.right.nodes {
							if node.preview == path {
								view.rightWidget.RefreshItem(id)
							}
						}
					}
				})
			}
		}()
	}
	for _, entry := range plan.Entries {
		select {
		case jobs <- entry.Source:
		case <-ctx.Done():
			close(jobs)
			workers.Wait()
			return
		}
	}
	close(jobs)
	workers.Wait()
}

func (view *desktop) copy() {
	if view.copying || view.analyzing || view.completed || len(view.plan.Entries) == 0 {
		return
	}
	plan := view.plan
	view.copying = true
	view.copied = 0
	view.progress.SetValue(0)
	view.right = newFileTree()
	view.rightWidget.Refresh()
	view.targetCount.SetText("0 copie")
	view.status.SetText("Copie en cours…")
	view.updateControls()
	ctx, cancel := context.WithCancel(context.Background())
	view.cancelCopy = cancel
	go func() {
		report, err := sorter.CopyWithProgress(ctx, plan, func(entry sorter.Entry) {
			fyne.Do(func() { view.showCopied(plan, entry) })
		})
		cancel()
		fyne.Do(func() {
			view.copying = false
			view.completed = true
			view.updateControls()
			if errors.Is(err, context.Canceled) {
				view.status.SetText(fmt.Sprintf("Copie arrêtée · %d fichiers copiés", report.Copied))
			} else if err != nil {
				view.status.SetText(fmt.Sprintf("Copie interrompue · %d fichiers copiés", report.Copied))
				dialog.ShowError(err, view.window)
			} else {
				view.status.SetText(fmt.Sprintf("Tri terminé · %d fichiers copiés", report.Copied))
			}
		})
	}()
}

func (view *desktop) showCopied(plan sorter.Plan, entry sorter.Entry) {
	view.copied++
	path := filepath.Join(plan.Destination, entry.Target)
	view.right.add(plan.Destination, path, entry.Source)
	view.rightWidget.Refresh()
	view.rightWidget.OpenBranch(plan.Destination)
	for directory := filepath.Dir(path); directory != plan.Destination && localTo(plan.Destination, directory); directory = filepath.Dir(directory) {
		view.rightWidget.OpenBranch(directory)
	}
	view.targetCount.SetText(fmt.Sprintf("%d copies", view.copied))
	view.progress.SetValue(float64(view.copied) / float64(len(plan.Entries)))
	view.status.SetText(fmt.Sprintf("%d / %d copies", view.copied, len(plan.Entries)))
}

func (view *desktop) stop() {
	if view.copying {
		if view.cancelCopy != nil {
			view.cancelCopy()
		}
		return
	}
	if view.cancelPlan != nil {
		view.cancelPlan()
	}
	view.generation++
	view.analyzing = false
	view.loading.Hide()
	view.status.SetText("Analyse arrêtée")
	view.updateControls()
}

func (view *desktop) updateControls() {
	for _, control := range []interface {
		Enable()
		Disable()
	}{view.add, view.clear, view.choose, view.destination} {
		if view.copying {
			control.Disable()
		} else {
			control.Enable()
		}
	}
	view.start.Disable()
	if !view.copying && !view.analyzing && !view.completed && len(view.plan.Entries) > 0 {
		view.start.Enable()
	}
	view.cancel.Disable()
	if view.copying || view.analyzing {
		view.cancel.Enable()
	}
}

func (view *desktop) updateWarnings() {
	view.warnings.SetText(fmt.Sprintf("%d signalements", len(view.issues)))
	if len(view.issues) == 0 {
		view.warnings.Disable()
	} else {
		view.warnings.Enable()
	}
}

func (view *desktop) showIssues() {
	var text strings.Builder
	for _, issue := range view.issues {
		fmt.Fprintf(&text, "%s\n%s\n\n", issue.Path, issue.Reason)
	}
	label := widget.NewLabel(text.String())
	label.Wrapping = fyne.TextWrapBreak
	popup := dialog.NewCustom("Fichiers signalés", "Fermer", container.NewVScroll(label), view.window)
	popup.Resize(fyne.NewSize(720, 440))
	popup.Show()
}

func (view *desktop) newTree(source bool) *widget.Tree {
	model := func() *fileTree {
		if source {
			return view.left
		}
		return view.right
	}
	tree := widget.NewTree(func(id string) []string { return model().children(id) }, func(id string) bool {
		if id == "" {
			return true
		}
		node := model().nodes[id]
		return node != nil && node.folder
	}, func(bool) fyne.CanvasObject {
		preview := canvas.NewImageFromResource(theme.FileImageIcon())
		preview.FillMode = canvas.ImageFillContain
		label := widget.NewLabel("")
		label.Truncation = fyne.TextTruncateEllipsis
		imageBox := container.NewGridWrap(fyne.NewSize(48, 48), preview)
		return container.New(layout.NewBorderLayout(nil, nil, imageBox, nil), label, imageBox)
	}, func(id string, _ bool, object fyne.CanvasObject) {
		node := model().nodes[id]
		if node == nil {
			return
		}
		row := object.(*fyne.Container)
		label := row.Objects[0].(*widget.Label)
		preview := row.Objects[1].(*fyne.Container).Objects[0].(*canvas.Image)
		label.SetText(node.name)
		label.TextStyle.Bold = node.folder
		label.Refresh()
		preview.Image = nil
		preview.Resource = theme.FileImageIcon()
		if node.folder {
			preview.Resource = theme.NewColoredResource(theme.FolderIcon(), "folderBlue")
		} else if thumbnail := view.previews[node.preview]; thumbnail != nil {
			preview.Resource = nil
			preview.Image = thumbnail
		}
		preview.Refresh()
	})
	tree.HideSeparators = true
	tree.OnSelected = func(id string) {
		if node := model().nodes[id]; node != nil {
			view.selection.SetText(node.path)
		}
	}
	return tree
}

type dualPaneLayout struct{}

func (dualPaneLayout) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	centerWidth := objects[1].MinSize().Width + 24
	paneWidth := max(0, (size.Width-centerWidth)/2)
	objects[0].Move(fyne.NewPos(0, 0))
	objects[0].Resize(fyne.NewSize(paneWidth, size.Height))
	objects[1].Move(fyne.NewPos(paneWidth+12, 0))
	objects[1].Resize(fyne.NewSize(centerWidth-24, size.Height))
	objects[2].Move(fyne.NewPos(paneWidth+centerWidth, 0))
	objects[2].Resize(fyne.NewSize(paneWidth, size.Height))
}

func (dualPaneLayout) MinSize(objects []fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(2*max(objects[0].MinSize().Width, objects[2].MinSize().Width)+objects[1].MinSize().Width+24, 400)
}

type lightTheme struct{}

func (lightTheme) Color(name fyne.ThemeColorName, _ fyne.ThemeVariant) color.Color {
	switch name {
	case "folderBlue":
		return color.NRGBA{R: 57, G: 158, B: 225, A: 255}
	case theme.ColorNamePrimary:
		return color.NRGBA{R: 23, G: 112, B: 87, A: 255}
	case theme.ColorNameBackground:
		return color.NRGBA{R: 249, G: 250, B: 251, A: 255}
	}
	return theme.DefaultTheme().Color(name, theme.VariantLight)
}

func (lightTheme) Font(style fyne.TextStyle) fyne.Resource    { return theme.DefaultTheme().Font(style) }
func (lightTheme) Icon(name fyne.ThemeIconName) fyne.Resource { return theme.DefaultTheme().Icon(name) }
func (lightTheme) Size(name fyne.ThemeSizeName) float32       { return theme.DefaultTheme().Size(name) }

func Theme() fyne.Theme { return lightTheme{} }
