package ui

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"io.github.sandwichdoge.vibesandfolders/internal/app"
)

type IndexDetailsWindow struct {
	app          fyne.App
	window       fyne.Window
	orchestrator *app.Orchestrator
	logger       *app.Logger
	dirPath      string

	listContainer *fyne.Container
	scrollContent *container.Scroll
	statusLabel   *widget.Label
	statsLabel    *widget.Label
	searchEntry   *widget.Entry

	allFiles      []app.IndexedFile
	filteredFiles []app.IndexedFile
}

func NewIndexDetailsWindow(fyneApp fyne.App, orchestrator *app.Orchestrator, logger *app.Logger, dirPath string) *IndexDetailsWindow {
	idw := &IndexDetailsWindow{
		app:          fyneApp,
		window:       fyneApp.NewWindow("Index Details - " + filepath.Base(dirPath)),
		orchestrator: orchestrator,
		logger:       logger,
		dirPath:      dirPath,
	}

	idw.initializeComponents()
	idw.setupLayout()
	idw.loadData()

	return idw
}

func (idw *IndexDetailsWindow) initializeComponents() {
	idw.statusLabel = widget.NewLabel("Loading...")
	idw.statsLabel = widget.NewLabel("Loading statistics...")

	idw.searchEntry = widget.NewEntry()
	idw.searchEntry.SetPlaceHolder("Search filenames, paths, or descriptions...")
	idw.searchEntry.OnChanged = func(query string) {
		idw.filterData(query)
	}

	idw.listContainer = container.NewVBox()
	idw.scrollContent = container.NewScroll(idw.listContainer)
}

func (idw *IndexDetailsWindow) setupLayout() {
	content := container.NewBorder(
		container.NewVBox(
			widget.NewLabel("Indexed Files for: " + idw.dirPath),
			idw.statsLabel,
			idw.searchEntry,
			widget.NewSeparator(),
		),
		container.NewVBox(
			widget.NewSeparator(),
			idw.statusLabel,
		),
		nil, nil,
		idw.scrollContent,
	)

	idw.window.SetContent(container.NewPadded(content))
	idw.window.Resize(fyne.NewSize(1000, 600))
}

func (idw *IndexDetailsWindow) loadData() {
	idw.statusLabel.SetText("Loading indexed files...")
	idw.statsLabel.SetText("Loading statistics...")

	go func() {
		files, err := idw.orchestrator.GetIndexedFiles(idw.dirPath)

		fyne.Do(func() {
			if err != nil {
				idw.logger.Error("Failed to load indexed files: %v", err)
				dialog.ShowError(fmt.Errorf("failed to load indexed files: %w", err), idw.window)
				idw.statusLabel.SetText("Error loading data")
				idw.statsLabel.SetText("Error loading statistics")
				return
			}

			idw.allFiles = files
			idw.filteredFiles = files
			idw.updateStats()
			idw.renderFiles()

			if len(files) == 0 {
				idw.statusLabel.SetText("No indexed files found")
			} else {
				idw.statusLabel.SetText(fmt.Sprintf("Showing %d indexed files", len(files)))
			}
		})
	}()
}

func (idw *IndexDetailsWindow) filterData(query string) {
	if query == "" {
		idw.filteredFiles = idw.allFiles
	} else {
		query = strings.ToLower(query)
		idw.filteredFiles = []app.IndexedFile{}

		for _, file := range idw.allFiles {
			// Search in full path
			if strings.Contains(strings.ToLower(file.FilePath), query) {
				idw.filteredFiles = append(idw.filteredFiles, file)
				continue
			}

			// Search in just the filename (basename)
			filename := filepath.Base(file.FilePath)
			if strings.Contains(strings.ToLower(filename), query) {
				idw.filteredFiles = append(idw.filteredFiles, file)
				continue
			}

			// Search in description
			if strings.Contains(strings.ToLower(file.Description), query) {
				idw.filteredFiles = append(idw.filteredFiles, file)
				continue
			}
		}
	}

	idw.renderFiles()
	idw.statusLabel.SetText(fmt.Sprintf("Showing %d of %d indexed files", len(idw.filteredFiles), len(idw.allFiles)))
}

func (idw *IndexDetailsWindow) renderFiles() {
	idw.listContainer.Objects = nil

	if len(idw.filteredFiles) == 0 {
		emptyLabel := widget.NewLabel("No files to display")
		emptyLabel.Alignment = fyne.TextAlignCenter
		idw.listContainer.Add(emptyLabel)
		idw.listContainer.Refresh()
		return
	}

	for _, file := range idw.filteredFiles {
		card := idw.createFileCard(file)
		idw.listContainer.Add(card)
	}

	idw.listContainer.Refresh()
}

func (idw *IndexDetailsWindow) createFileCard(file app.IndexedFile) fyne.CanvasObject {
	// Get relative path
	relPath, err := filepath.Rel(idw.dirPath, file.FilePath)
	if err != nil {
		relPath = file.FilePath
	}

	// File path label (bold and larger)
	pathLabel := widget.NewLabel(relPath)
	pathLabel.TextStyle = fyne.TextStyle{Bold: true}
	pathLabel.Wrapping = fyne.TextWrapWord

	// Description label (with wrapping)
	descLabel := widget.NewLabel(file.Description)
	descLabel.Wrapping = fyne.TextWrapWord

	// Create metadata line
	metaText := fmt.Sprintf("Type: %s  |  Size: %s  |  Modified: %s  |  Indexed: %s",
		file.FileType,
		formatFileSize(file.FileSize),
		formatTimestamp(file.LastModified),
		formatTimestamp(file.IndexedAt),
	)
	metaLabel := widget.NewLabel(metaText)
	metaLabel.TextStyle = fyne.TextStyle{Italic: true}

	// Create delete button
	deleteBtn := widget.NewButton("Delete", func() {
		idw.deleteEntry(file)
	})
	deleteBtn.Importance = widget.DangerImportance

	// Create a separator line
	separator := canvas.NewLine(theme.ShadowColor())
	separator.StrokeWidth = 1

	// Assemble the card with delete button at the right
	topRow := container.NewBorder(nil, nil, nil, deleteBtn, pathLabel)

	cardContent := container.NewVBox(
		topRow,
		descLabel,
		layout.NewSpacer(),
		metaLabel,
		separator,
	)

	return cardContent
}

func (idw *IndexDetailsWindow) deleteEntry(file app.IndexedFile) {
	// Get relative path for display
	relPath, err := filepath.Rel(idw.dirPath, file.FilePath)
	if err != nil {
		relPath = file.FilePath
	}

	// Show confirmation dialog
	dialog.ShowConfirm(
		"Delete Index Entry",
		fmt.Sprintf("Are you sure you want to delete the index entry for:\n\n%s\n\nThis will only remove the entry from the index, not the actual file.", relPath),
		func(confirmed bool) {
			if !confirmed {
				return
			}

			// Perform deletion in a goroutine
			go func() {
				err := idw.orchestrator.DeleteIndexEntry(file.FilePath)

				fyne.Do(func() {
					if err != nil {
						idw.logger.Error("Failed to delete index entry: %v", err)
						dialog.ShowError(fmt.Errorf("failed to delete index entry: %w", err), idw.window)
						return
					}

					idw.logger.Info("Deleted index entry for: %s", file.FilePath)

					// Refresh the data to show updated list
					idw.loadData()
				})
			}()
		},
		idw.window,
	)
}

func (idw *IndexDetailsWindow) updateStats() {
	if len(idw.allFiles) == 0 {
		idw.statsLabel.SetText("No indexed files")
		return
	}

	// Count by file type
	typeCount := make(map[string]int)
	for _, file := range idw.allFiles {
		typeCount[file.FileType]++
	}

	// Format the statistics text
	statsText := fmt.Sprintf("Total: %d files", len(idw.allFiles))

	// Add breakdown by type if there are multiple types
	if len(typeCount) > 1 {
		statsText += " ("
		first := true
		for fileType, count := range typeCount {
			if !first {
				statsText += ", "
			}
			statsText += fmt.Sprintf("%s: %d", fileType, count)
			first = false
		}
		statsText += ")"
	}

	idw.statsLabel.SetText(statsText)
}

func (idw *IndexDetailsWindow) Show() {
	idw.window.Show()
}

// formatFileSize formats bytes into human-readable format
func formatFileSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	units := []string{"KB", "MB", "GB", "TB"}
	return fmt.Sprintf("%.1f %s", float64(bytes)/float64(div), units[exp])
}

// formatTimestamp formats time into a readable format
func formatTimestamp(t time.Time) string {
	return t.Format("2006-01-02 15:04:05")
}
