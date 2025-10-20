package main

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/content"
	"github.com/containerd/containerd/images"
	"github.com/containerd/containerd/namespaces"
	"github.com/containerd/containerd/snapshots"
	"github.com/gdamore/tcell/v2"
	"github.com/opencontainers/go-digest"
	"github.com/rivo/tview"
)

type ResourceType int

const (
	ResourceImages ResourceType = iota
	ResourceContainers
	ResourceTasks
	ResourceSnapshots
	ResourceContent
)

func (r ResourceType) String() string {
	switch r {
	case ResourceImages:
		return "Images"
	case ResourceContainers:
		return "Containers"
	case ResourceTasks:
		return "Tasks"
	case ResourceSnapshots:
		return "Snapshots"
	case ResourceContent:
		return "Content"
	default:
		return "Unknown"
	}
}

type App struct {
	tviewApp         *tview.Application
	client           *containerd.Client
	namespaceList    *tview.List
	resourceList     *tview.List
	itemTable        *tview.Table
	statusBar        *tview.TextView
	helpText         *tview.TextView
	pages            *tview.Pages
	currentNamespace string
	currentResource  ResourceType
	itemCache        []interface{}
	allItems         []interface{}
	searchQuery      string
	searchInput      *tview.InputField
}

type ImageInfo struct {
	Name      string
	Size      int64
	CreatedAt time.Time
}

type ContainerInfo struct {
	ID        string
	Image     string
	CreatedAt time.Time
	Status    string
}

type TaskInfo struct {
	ID     string
	PID    uint32
	Status string
}

type SnapshotInfo struct {
	Key    string
	Parent string
	Kind   string
}

type ContentInfo struct {
	Digest string
	Size   int64
}

func main() {
	client, err := containerd.New("/run/containerd/containerd.sock")
	if err != nil {
		log.Fatalf("Failed to connect to containerd: %v", err)
	}
	defer client.Close()

	app := &App{
		tviewApp:        tview.NewApplication(),
		client:          client,
		currentResource: ResourceImages,
	}

	if err := app.initUI(); err != nil {
		log.Fatalf("Failed to initialize UI: %v", err)
	}

	if err := app.tviewApp.Run(); err != nil {
		log.Fatalf("Error running application: %v", err)
	}
}

func (app *App) initUI() error {
	// Create namespace list
	app.namespaceList = tview.NewList().
		ShowSecondaryText(false).
		SetHighlightFullLine(true)

	app.namespaceList.SetBorder(true).
		SetTitle(" Namespaces ").
		SetTitleAlign(tview.AlignLeft)

	// Create resource type list
	app.resourceList = tview.NewList().
		ShowSecondaryText(false).
		SetHighlightFullLine(true)

	app.resourceList.SetBorder(true).
		SetTitle(" Resources ").
		SetTitleAlign(tview.AlignLeft)

	// Add all resource types
	resources := []ResourceType{ResourceImages, ResourceContainers, ResourceTasks, ResourceSnapshots, ResourceContent}
	for _, res := range resources {
		resType := res // capture for closure
		app.resourceList.AddItem(resType.String(), "", 0, nil)
	}
	app.resourceList.SetCurrentItem(0)

	// Create item table
	app.itemTable = tview.NewTable().
		SetBorders(false).
		SetSelectable(true, false)

	app.itemTable.SetBorder(true).
		SetTitle(" Items ").
		SetTitleAlign(tview.AlignLeft)

	// Create search input field
	app.searchInput = tview.NewInputField().
		SetLabel("Search: ").
		SetFieldWidth(50).
		SetDoneFunc(func(key tcell.Key) {
			if key == tcell.KeyEnter {
				app.closeSearchBox()
			} else if key == tcell.KeyEscape {
				app.hideSearch()
			}
		})

	app.searchInput.SetChangedFunc(func(text string) {
		app.searchQuery = text
		app.filterItems()
	})

	// Create status bar
	app.statusBar = tview.NewTextView().
		SetDynamicColors(true).
		SetText("[yellow]Loading...[white]")
	app.statusBar.SetBorder(false)

	// Create help text
	app.helpText = tview.NewTextView().
		SetDynamicColors(true).
		SetText("[yellow]q[white]:Quit [yellow]d[white]:Delete [yellow]D[white]:Delete NS [yellow]a[white]:Delete All [yellow]/[white]:Search [yellow]1-5[white]:Jump [yellow]?[white]:Help")
	app.helpText.SetBorder(false)

	// Load namespaces
	if err := app.loadNamespaces(); err != nil {
		return fmt.Errorf("failed to load namespaces: %w", err)
	}

	// Set up namespace selection handler
	app.namespaceList.SetChangedFunc(func(index int, mainText, secondaryText string, shortcut rune) {
		app.currentNamespace = mainText
		app.loadItems()
	})

	// Set up resource selection handler
	app.resourceList.SetChangedFunc(func(index int, mainText, secondaryText string, shortcut rune) {
		app.currentResource = ResourceType(index)
		app.loadItems()
	})

	// Create three-panel layout
	leftPanel := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(app.namespaceList, 0, 1, true)

	middlePanel := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(app.resourceList, 0, 1, false)

	rightPanel := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(app.itemTable, 0, 1, false)

	mainFlex := tview.NewFlex().
		AddItem(leftPanel, 0, 1, true).
		AddItem(middlePanel, 0, 1, false).
		AddItem(rightPanel, 0, 3, false)

	bottomBar := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(app.statusBar, 1, 0, false).
		AddItem(app.helpText, 1, 0, false)

	layout := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(mainFlex, 0, 1, true).
		AddItem(bottomBar, 2, 0, false)

	// Create pages for modal dialogs
	app.pages = tview.NewPages().
		AddPage("main", layout, true, true)

	// Set up keyboard shortcuts
	app.pages.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyRune:
			switch event.Rune() {
			case 'q', 'Q':
				app.tviewApp.Stop()
				return nil
			case 'd':
				if app.itemTable.HasFocus() {
					app.deleteSelectedItem()
				}
				return nil
			case 'D':
				if app.namespaceList.HasFocus() {
					app.deleteSelectedNamespace()
				}
				return nil
			case 'a', 'A':
				if app.itemTable.HasFocus() {
					app.deleteAllItems()
				}
				return nil
			case '/':
				app.showSearch()
				return nil
			case '?':
				app.showHelp()
				return nil
			case '1':
				app.resourceList.SetCurrentItem(0)
				app.tviewApp.SetFocus(app.resourceList)
				return nil
			case '2':
				app.resourceList.SetCurrentItem(1)
				app.tviewApp.SetFocus(app.resourceList)
				return nil
			case '3':
				app.resourceList.SetCurrentItem(2)
				app.tviewApp.SetFocus(app.resourceList)
				return nil
			case '4':
				app.resourceList.SetCurrentItem(3)
				app.tviewApp.SetFocus(app.resourceList)
				return nil
			case '5':
				app.resourceList.SetCurrentItem(4)
				app.tviewApp.SetFocus(app.resourceList)
				return nil
			}
		case tcell.KeyTab:
			if app.namespaceList.HasFocus() {
				app.tviewApp.SetFocus(app.resourceList)
			} else if app.resourceList.HasFocus() {
				app.tviewApp.SetFocus(app.itemTable)
			} else {
				app.tviewApp.SetFocus(app.namespaceList)
			}
			return nil
		case tcell.KeyBacktab:
			if app.itemTable.HasFocus() {
				app.tviewApp.SetFocus(app.resourceList)
			} else if app.resourceList.HasFocus() {
				app.tviewApp.SetFocus(app.namespaceList)
			} else {
				app.tviewApp.SetFocus(app.itemTable)
			}
			return nil
		case tcell.KeyEscape:
			if app.searchQuery != "" {
				app.hideSearch()
				return nil
			}
		}
		return event
	})

	app.tviewApp.SetRoot(app.pages, true)

	return nil
}

func (app *App) loadNamespaces() error {
	ctx := context.Background()

	namespaceSvc := app.client.NamespaceService()
	nsList, err := namespaceSvc.List(ctx)
	if err != nil {
		return fmt.Errorf("failed to list namespaces: %w", err)
	}

	app.namespaceList.Clear()

	for _, ns := range nsList {
		app.namespaceList.AddItem(ns, "", 0, nil)
	}

	if len(nsList) > 0 {
		app.currentNamespace = nsList[0]
		app.namespaceList.SetCurrentItem(0)
		app.loadItems()
	}

	app.updateStatus(fmt.Sprintf("Loaded %d namespaces", len(nsList)))
	return nil
}

func (app *App) loadItems() {
	if app.currentNamespace == "" {
		return
	}

	ctx := namespaces.WithNamespace(context.Background(), app.currentNamespace)

	app.allItems = make([]interface{}, 0)
	app.itemCache = make([]interface{}, 0)

	var err error
	switch app.currentResource {
	case ResourceImages:
		err = app.loadImages(ctx)
	case ResourceContainers:
		err = app.loadContainers(ctx)
	case ResourceTasks:
		err = app.loadTasks(ctx)
	case ResourceSnapshots:
		err = app.loadSnapshots(ctx)
	case ResourceContent:
		err = app.loadContent(ctx)
	}

	if err != nil {
		app.updateStatus(fmt.Sprintf("[red]Error loading %s: %v", app.currentResource, err))
		return
	}

	app.searchQuery = ""
	app.filterItems()
}

func (app *App) loadImages(ctx context.Context) error {
	imageService := app.client.ImageService()
	imageList, err := imageService.List(ctx)
	if err != nil {
		return err
	}

	contentStore := app.client.ContentStore()

	for _, img := range imageList {
		size, err := app.calculateImageSize(ctx, img, contentStore)
		if err != nil {
			size = img.Target.Size
		}

		imgInfo := ImageInfo{
			Name:      img.Name,
			Size:      size,
			CreatedAt: img.CreatedAt,
		}
		app.allItems = append(app.allItems, imgInfo)
	}

	return nil
}

func (app *App) loadContainers(ctx context.Context) error {
	containers, err := app.client.Containers(ctx)
	if err != nil {
		return err
	}

	for _, container := range containers {
		info, err := container.Info(ctx)
		if err != nil {
			continue
		}

		containerInfo := ContainerInfo{
			ID:        container.ID(),
			Image:     info.Image,
			CreatedAt: info.CreatedAt,
			Status:    "Stopped",
		}

		// Check if task exists (running)
		task, err := container.Task(ctx, nil)
		if err == nil {
			status, _ := task.Status(ctx)
			containerInfo.Status = string(status.Status)
		}

		app.allItems = append(app.allItems, containerInfo)
	}

	return nil
}

func (app *App) loadTasks(ctx context.Context) error {
	containers, err := app.client.Containers(ctx)
	if err != nil {
		return err
	}

	for _, container := range containers {
		task, err := container.Task(ctx, nil)
		if err != nil {
			continue // No task for this container
		}

		status, err := task.Status(ctx)
		if err != nil {
			continue
		}

		taskInfo := TaskInfo{
			ID:     container.ID(),
			PID:    task.Pid(),
			Status: string(status.Status),
		}

		app.allItems = append(app.allItems, taskInfo)
	}

	return nil
}

func (app *App) loadSnapshots(ctx context.Context) error {
	snapshotter := app.client.SnapshotService("overlayfs")

	var snapshotList []SnapshotInfo
	err := snapshotter.Walk(ctx, func(ctx context.Context, info snapshots.Info) error {
		snapshotInfo := SnapshotInfo{
			Key:    info.Name,
			Parent: info.Parent,
			Kind:   string(info.Kind),
		}
		snapshotList = append(snapshotList, snapshotInfo)
		return nil
	})

	if err != nil {
		return err
	}

	for _, snap := range snapshotList {
		app.allItems = append(app.allItems, snap)
	}

	return nil
}

func (app *App) loadContent(ctx context.Context) error {
	contentStore := app.client.ContentStore()

	var contentList []ContentInfo
	err := contentStore.Walk(ctx, func(info content.Info) error {
		contentInfo := ContentInfo{
			Digest: info.Digest.String(),
			Size:   info.Size,
		}
		contentList = append(contentList, contentInfo)
		return nil
	})

	if err != nil {
		return err
	}

	for _, c := range contentList {
		app.allItems = append(app.allItems, c)
	}

	return nil
}

func (app *App) calculateImageSize(ctx context.Context, img images.Image, contentStore content.Store) (int64, error) {
	var size int64

	manifest, err := images.Manifest(ctx, contentStore, img.Target, nil)
	if err != nil {
		return 0, err
	}

	size += manifest.Config.Size

	for _, layer := range manifest.Layers {
		size += layer.Size
	}

	return size, nil
}

func (app *App) filterItems() {
	if app.searchQuery == "" {
		app.itemCache = app.allItems
	} else {
		app.itemCache = make([]interface{}, 0)
		query := strings.ToLower(app.searchQuery)

		for _, item := range app.allItems {
			var searchField string
			switch v := item.(type) {
			case ImageInfo:
				searchField = v.Name
			case ContainerInfo:
				searchField = v.ID + " " + v.Image
			case TaskInfo:
				searchField = v.ID
			case SnapshotInfo:
				searchField = v.Key
			case ContentInfo:
				searchField = v.Digest
			}

			if strings.Contains(strings.ToLower(searchField), query) {
				app.itemCache = append(app.itemCache, item)
			}
		}
	}

	app.renderItemTable()
}

func (app *App) renderItemTable() {
	app.itemTable.Clear()

	switch app.currentResource {
	case ResourceImages:
		app.renderImagesTable()
	case ResourceContainers:
		app.renderContainersTable()
	case ResourceTasks:
		app.renderTasksTable()
	case ResourceSnapshots:
		app.renderSnapshotsTable()
	case ResourceContent:
		app.renderContentTable()
	}

	if len(app.itemCache) > 0 {
		app.itemTable.Select(1, 0)
		app.itemTable.SetSelectable(true, false)
	} else {
		app.itemTable.SetCell(1, 0, tview.NewTableCell(fmt.Sprintf("No %s found", strings.ToLower(app.currentResource.String()))).
			SetTextColor(tcell.ColorGray).
			SetAlign(tview.AlignCenter))
		app.itemTable.Select(0, 0)
		app.itemTable.SetSelectable(false, false)
	}

	titleSuffix := ""
	if app.searchQuery != "" {
		titleSuffix = fmt.Sprintf(" (filtered: %s)", app.searchQuery)
	}
	app.itemTable.SetTitle(fmt.Sprintf(" %s [%s]%s ", app.currentResource, app.currentNamespace, titleSuffix))

	app.updateStatus(fmt.Sprintf("Namespace: [cyan]%s[white] | Resource: [yellow]%s[white] | Count: [green]%d[white]/%d",
		app.currentNamespace, app.currentResource, len(app.itemCache), len(app.allItems)))
}

func (app *App) renderImagesTable() {
	headers := []string{"Name", "Size", "Created"}
	for i, header := range headers {
		cell := tview.NewTableCell(header).
			SetTextColor(tcell.ColorYellow).
			SetAlign(tview.AlignLeft).
			SetSelectable(false).
			SetAttributes(tcell.AttrBold)
		app.itemTable.SetCell(0, i, cell)
	}

	for i, item := range app.itemCache {
		img := item.(ImageInfo)
		row := i + 1

		app.itemTable.SetCell(row, 0, tview.NewTableCell(img.Name).SetTextColor(tcell.ColorWhite))
		app.itemTable.SetCell(row, 1, tview.NewTableCell(formatSize(img.Size)).SetTextColor(tcell.ColorGreen))
		app.itemTable.SetCell(row, 2, tview.NewTableCell(img.CreatedAt.Format("2006-01-02 15:04")).SetTextColor(tcell.ColorTeal))
	}
}

func (app *App) renderContainersTable() {
	headers := []string{"ID", "Image", "Status", "Created"}
	for i, header := range headers {
		cell := tview.NewTableCell(header).
			SetTextColor(tcell.ColorYellow).
			SetAlign(tview.AlignLeft).
			SetSelectable(false).
			SetAttributes(tcell.AttrBold)
		app.itemTable.SetCell(0, i, cell)
	}

	for i, item := range app.itemCache {
		container := item.(ContainerInfo)
		row := i + 1

		app.itemTable.SetCell(row, 0, tview.NewTableCell(container.ID).SetTextColor(tcell.ColorWhite))
		app.itemTable.SetCell(row, 1, tview.NewTableCell(container.Image).SetTextColor(tcell.ColorTeal))

		statusColor := tcell.ColorGray
		if container.Status == "running" {
			statusColor = tcell.ColorGreen
		}
		app.itemTable.SetCell(row, 2, tview.NewTableCell(container.Status).SetTextColor(statusColor))
		app.itemTable.SetCell(row, 3, tview.NewTableCell(container.CreatedAt.Format("2006-01-02 15:04")).SetTextColor(tcell.ColorTeal))
	}
}

func (app *App) renderTasksTable() {
	headers := []string{"Container ID", "PID", "Status"}
	for i, header := range headers {
		cell := tview.NewTableCell(header).
			SetTextColor(tcell.ColorYellow).
			SetAlign(tview.AlignLeft).
			SetSelectable(false).
			SetAttributes(tcell.AttrBold)
		app.itemTable.SetCell(0, i, cell)
	}

	for i, item := range app.itemCache {
		task := item.(TaskInfo)
		row := i + 1

		app.itemTable.SetCell(row, 0, tview.NewTableCell(task.ID).SetTextColor(tcell.ColorWhite))
		app.itemTable.SetCell(row, 1, tview.NewTableCell(fmt.Sprintf("%d", task.PID)).SetTextColor(tcell.ColorGreen))
		app.itemTable.SetCell(row, 2, tview.NewTableCell(task.Status).SetTextColor(tcell.ColorTeal))
	}
}

func (app *App) renderSnapshotsTable() {
	headers := []string{"Key", "Parent", "Kind"}
	for i, header := range headers {
		cell := tview.NewTableCell(header).
			SetTextColor(tcell.ColorYellow).
			SetAlign(tview.AlignLeft).
			SetSelectable(false).
			SetAttributes(tcell.AttrBold)
		app.itemTable.SetCell(0, i, cell)
	}

	for i, item := range app.itemCache {
		snapshot := item.(SnapshotInfo)
		row := i + 1

		app.itemTable.SetCell(row, 0, tview.NewTableCell(snapshot.Key).SetTextColor(tcell.ColorWhite))

		parent := snapshot.Parent
		if parent == "" {
			parent = "-"
		}
		app.itemTable.SetCell(row, 1, tview.NewTableCell(parent).SetTextColor(tcell.ColorTeal))
		app.itemTable.SetCell(row, 2, tview.NewTableCell(snapshot.Kind).SetTextColor(tcell.ColorGreen))
	}
}

func (app *App) renderContentTable() {
	headers := []string{"Digest", "Size"}
	for i, header := range headers {
		cell := tview.NewTableCell(header).
			SetTextColor(tcell.ColorYellow).
			SetAlign(tview.AlignLeft).
			SetSelectable(false).
			SetAttributes(tcell.AttrBold)
		app.itemTable.SetCell(0, i, cell)
	}

	for i, item := range app.itemCache {
		c := item.(ContentInfo)
		row := i + 1

		// Truncate digest for display
		digest := c.Digest
		if len(digest) > 60 {
			digest = digest[:60] + "..."
		}
		app.itemTable.SetCell(row, 0, tview.NewTableCell(digest).SetTextColor(tcell.ColorWhite))
		app.itemTable.SetCell(row, 1, tview.NewTableCell(formatSize(c.Size)).SetTextColor(tcell.ColorGreen))
	}
}

func (app *App) showSearch() {
	app.searchInput.SetText("")

	modal := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().
			AddItem(nil, 0, 1, false).
			AddItem(app.searchInput, 60, 1, true).
			AddItem(nil, 0, 1, false), 3, 1, true).
		AddItem(nil, 0, 1, false)

	app.pages.AddPage("search", modal, true, true)
	app.tviewApp.SetFocus(app.searchInput)
}

func (app *App) closeSearchBox() {
	app.pages.RemovePage("search")
	app.tviewApp.SetFocus(app.itemTable)
}

func (app *App) hideSearch() {
	app.searchQuery = ""
	app.searchInput.SetText("")
	app.filterItems()
	app.pages.RemovePage("search")
	app.tviewApp.SetFocus(app.itemTable)
}

func (app *App) deleteSelectedItem() {
	row, _ := app.itemTable.GetSelection()
	if row <= 0 || row > len(app.itemCache) {
		return
	}

	item := app.itemCache[row-1]
	var itemName string

	switch v := item.(type) {
	case ImageInfo:
		itemName = v.Name
	case ContainerInfo:
		itemName = v.ID
	case TaskInfo:
		itemName = v.ID
	case SnapshotInfo:
		itemName = v.Key
	case ContentInfo:
		itemName = v.Digest
	}

	modal := tview.NewModal().
		SetText(fmt.Sprintf("Delete %s?\n\n%s\n\nThis action cannot be undone!", app.currentResource, itemName)).
		AddButtons([]string{"Delete", "Cancel"}).
		SetDoneFunc(func(buttonIndex int, buttonLabel string) {
			if buttonLabel == "Delete" {
				app.performDelete(item)
			}
			app.pages.RemovePage("confirm")
			app.tviewApp.SetFocus(app.itemTable)
		})

	modal.SetBorder(true).SetTitle(" Confirm Delete ")
	modal.SetBackgroundColor(tcell.ColorDefault)

	app.pages.AddPage("confirm", modal, true, true)
}

func (app *App) deleteAllItems() {
	if len(app.itemCache) == 0 {
		return
	}

	filterNote := ""
	if app.searchQuery != "" {
		filterNote = fmt.Sprintf("\n(Filtered results: %d of %d)", len(app.itemCache), len(app.allItems))
	}

	modal := tview.NewModal().
		SetText(fmt.Sprintf("Delete ALL %s in namespace '%s'?%s\n\nThis will delete %d items!\nThis action cannot be undone!",
			app.currentResource, app.currentNamespace, filterNote, len(app.itemCache))).
		AddButtons([]string{"Delete All", "Cancel"}).
		SetDoneFunc(func(buttonIndex int, buttonLabel string) {
			if buttonLabel == "Delete All" {
				app.performDeleteAll()
			}
			app.pages.RemovePage("confirm-all")
			app.tviewApp.SetFocus(app.itemTable)
		})

	modal.SetBorder(true).SetTitle(" ⚠ Confirm Delete All ")
	modal.SetBackgroundColor(tcell.ColorDefault)

	app.pages.AddPage("confirm-all", modal, true, true)
}

func (app *App) performDelete(item interface{}) {
	ctx := namespaces.WithNamespace(context.Background(), app.currentNamespace)

	var err error
	var itemName string

	switch v := item.(type) {
	case ImageInfo:
		itemName = v.Name
		imageService := app.client.ImageService()
		err = imageService.Delete(ctx, v.Name, images.SynchronousDelete())

	case ContainerInfo:
		itemName = v.ID
		container, e := app.client.LoadContainer(ctx, v.ID)
		if e == nil {
			err = container.Delete(ctx)
		} else {
			err = e
		}

	case TaskInfo:
		itemName = v.ID
		container, e := app.client.LoadContainer(ctx, v.ID)
		if e == nil {
			task, e2 := container.Task(ctx, nil)
			if e2 == nil {
				_, err = task.Delete(ctx)
			} else {
				err = e2
			}
		} else {
			err = e
		}

	case SnapshotInfo:
		itemName = v.Key
		snapshotter := app.client.SnapshotService("overlayfs")
		err = snapshotter.Remove(ctx, v.Key)

	case ContentInfo:
		itemName = v.Digest
		contentStore := app.client.ContentStore()
		dgst, parseErr := digest.Parse(v.Digest)
		if parseErr != nil {
			err = parseErr
		} else {
			err = contentStore.Delete(ctx, dgst)
		}
	}

	if err != nil {
		app.showError(fmt.Sprintf("Failed to delete %s: %v", itemName, err))
		return
	}

	app.updateStatus(fmt.Sprintf("[green]Deleted:[white] %s", itemName))
	app.loadItems()
}

func (app *App) performDeleteAll() {
	ctx := namespaces.WithNamespace(context.Background(), app.currentNamespace)

	successCount := 0
	failCount := 0

	for _, item := range app.itemCache {
		var err error

		switch v := item.(type) {
		case ImageInfo:
			imageService := app.client.ImageService()
			err = imageService.Delete(ctx, v.Name, images.SynchronousDelete())

		case ContainerInfo:
			container, e := app.client.LoadContainer(ctx, v.ID)
			if e == nil {
				err = container.Delete(ctx)
			}

		case TaskInfo:
			container, e := app.client.LoadContainer(ctx, v.ID)
			if e == nil {
				task, e2 := container.Task(ctx, nil)
				if e2 == nil {
					_, err = task.Delete(ctx)
				}
			}

		case SnapshotInfo:
			snapshotter := app.client.SnapshotService("overlayfs")
			err = snapshotter.Remove(ctx, v.Key)

		case ContentInfo:
			contentStore := app.client.ContentStore()
			dgst, parseErr := digest.Parse(v.Digest)
			if parseErr != nil {
				err = parseErr
			} else {
				err = contentStore.Delete(ctx, dgst)
			}
		}

		if err == nil {
			successCount++
		} else {
			failCount++
		}
	}

	if failCount > 0 {
		app.updateStatus(fmt.Sprintf("[yellow]Deleted %d items, %d failed", successCount, failCount))
	} else {
		app.updateStatus(fmt.Sprintf("[green]Successfully deleted all %d items", successCount))
	}

	app.loadItems()
}

func (app *App) deleteSelectedNamespace() {
	if app.currentNamespace == "" {
		return
	}

	modal := tview.NewModal().
		SetText(fmt.Sprintf("Delete entire namespace?\n\n%s\n\nWARNING: This will delete ALL resources in this namespace!\nThis action cannot be undone!", app.currentNamespace)).
		AddButtons([]string{"Delete Namespace", "Cancel"}).
		SetDoneFunc(func(buttonIndex int, buttonLabel string) {
			if buttonLabel == "Delete Namespace" {
				app.performDeleteNamespace(app.currentNamespace)
			}
			app.pages.RemovePage("confirm-ns")
			app.tviewApp.SetFocus(app.namespaceList)
		})

	modal.SetBorder(true).SetTitle(" ⚠ Confirm Delete Namespace ")
	modal.SetBackgroundColor(tcell.ColorDefault)

	app.pages.AddPage("confirm-ns", modal, true, true)
}

func (app *App) performDeleteNamespace(namespaceName string) {
	ctx := namespaces.WithNamespace(context.Background(), namespaceName)

	// Delete all images
	imageService := app.client.ImageService()
	imageList, _ := imageService.List(ctx)
	for _, img := range imageList {
		imageService.Delete(ctx, img.Name, images.SynchronousDelete())
	}

	// Delete all containers
	containers, _ := app.client.Containers(ctx)
	for _, container := range containers {
		container.Delete(ctx)
	}

	// Delete namespace
	namespaceSvc := app.client.NamespaceService()
	err := namespaceSvc.Delete(context.Background(), namespaceName)

	if err != nil {
		app.showError(fmt.Sprintf("Failed to delete namespace: %v", err))
		return
	}

	app.updateStatus(fmt.Sprintf("[green]Deleted namespace:[white] %s", namespaceName))
	app.loadNamespaces()
}

func (app *App) showHelp() {
	helpContent := `
[yellow]Keyboard Shortcuts:[white]

  [yellow]q, Q[white]         - Quit application
  [yellow]d[white]            - Delete selected item
  [yellow]D[white]            - Delete entire namespace (when in namespace panel)
  [yellow]a, A[white]         - Delete ALL items in current view
  [yellow]/[white]            - Search/filter items by name
  [yellow]1-5[white]          - Quick jump to resource (1:Images 2:Containers 3:Tasks 4:Snapshots 5:Content)
  [yellow]Tab[white]          - Cycle focus: Namespaces → Resources → Items
  [yellow]Shift+Tab[white]    - Cycle focus backward
  [yellow]?[white]            - Show this help
  [yellow]↑/↓[white]          - Navigate lists
  [yellow]Enter[white]        - Close search box (keep filter active)
  [yellow]Esc[white]          - Clear search filter / Close dialog

[yellow]Resource Types:[white]

  [yellow]1. Images[white]      - Container images (pull, build)
  [yellow]2. Containers[white]  - Container instances (created from images)
  [yellow]3. Tasks[white]       - Running containers (active processes)
  [yellow]4. Snapshots[white]   - Filesystem layers (overlayfs)
  [yellow]5. Content[white]     - Raw blobs in content store

[yellow]Workflow:[white]

  1. Select a namespace (left panel)
  2. Select a resource type (middle panel or press 1-5)
  3. View/manage items (right panel)
  4. Use 'd' to delete single item or 'a' to delete all
  5. Use '/' to search/filter items

[yellow]Note:[white] Requires root/sudo access to containerd socket.
Restart to refresh. Be careful with delete operations!
`

	modal := tview.NewModal().
		SetText(helpContent).
		AddButtons([]string{"Close"}).
		SetDoneFunc(func(buttonIndex int, buttonLabel string) {
			app.pages.RemovePage("help")
			app.tviewApp.SetFocus(app.itemTable)
		})

	modal.SetBorder(true).SetTitle(" Help ")
	app.pages.AddPage("help", modal, true, true)
}

func (app *App) showError(message string) {
	modal := tview.NewModal().
		SetText(fmt.Sprintf("[red]Error[white]\n\n%s", message)).
		AddButtons([]string{"OK"}).
		SetDoneFunc(func(buttonIndex int, buttonLabel string) {
			app.pages.RemovePage("error")
			app.tviewApp.SetFocus(app.itemTable)
		})

	modal.SetBorder(true).SetTitle(" Error ")
	app.pages.AddPage("error", modal, true, true)
}

func (app *App) updateStatus(message string) {
	app.statusBar.SetText(fmt.Sprintf(" %s", message))
}

func formatSize(bytes int64) string {
	const (
		KB = 1024
		MB = 1024 * KB
		GB = 1024 * MB
	)

	switch {
	case bytes >= GB:
		return fmt.Sprintf("%.2f GB", float64(bytes)/float64(GB))
	case bytes >= MB:
		return fmt.Sprintf("%.2f MB", float64(bytes)/float64(MB))
	case bytes >= KB:
		return fmt.Sprintf("%.2f KB", float64(bytes)/float64(KB))
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}
