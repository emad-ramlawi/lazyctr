# lazyctr - Containerd Terminal UI

A comprehensive terminal user interface (TUI) for managing all containerd resources, similar to k9s and lazydocker.

## Features

- 📋 **Three-Panel Layout** - Namespaces | Resources | Items (inspired by k9s)
- 🎯 **Multiple Resource Types** - Manage Images, Containers, Tasks, Snapshots, and Content
- 🔍 **Search/Filter** - Real-time search across all resource types
- 🗑️ **Flexible Deletion** - Delete individual items, all items, or entire namespaces
- 🏷️ **Image Tagging** - Create new tags/aliases for existing images
- ⌨️ **Intuitive Navigation** - Quick jump with number keys (1-5)
- 🎨 **Clean Interface** - Color-coded, easy-to-read terminal interface
- 📦 **Static Binary** - Single binary with no dependencies
- ⚙️ **Configurable Snapshotter** - Support for overlayfs, native, btrfs, zfs, etc.

## Three-Panel Design

```
┌─ Namespaces ──┐┌─ Resources ──┐┌─ Images [k8s.io] ──────────────┐
│ k8s.io        ││ Images       ││ Name              Size  Created│
│ moby          ││ Containers   ││ nginx:latest      142MB ...    │
│ default       ││ Tasks        ││ redis:alpine      31MB  ...    │
│               ││ Snapshots    │└────────────────────────────────┘
│               ││ Content      │
└───────────────┘└──────────────┘
 Namespace: k8s.io | Resource: Images | Count: 2/2
 q:Quit d:Delete D:Delete NS a:Delete All /:Search 1-5:Jump ?:Help
```

## Resource Types

### 1. Images
View and manage container images with accurate size calculation (including all layers).

**Columns**: Name | Size | Created

### 2. Containers
Manage container instances (both running and stopped).

**Columns**: ID | Image | Status | Created

**Status Colors**:
- 🟢 Green = Running
- ⚪ Gray = Stopped

### 3. Tasks
Monitor and manage active container processes.

**Columns**: Container ID | PID | Status

### 4. Snapshots
Manage filesystem snapshots (overlayfs layers).

**Columns**: Key | Parent | Kind

### 5. Content
Inspect and manage raw content blobs in the content store.

**Columns**: Digest | Size

## Requirements

- Linux system with containerd installed
- Root/sudo access (required to access containerd socket)
- Containerd socket at `/run/containerd/containerd.sock`

## Installation

### Option 1: Build and install

```bash
# Build static binary
go build -ldflags '-s -w' -o build/lazyctr .

# Install system-wide
sudo cp build/lazyctr /usr/local/bin/
```

### Option 2: Quick run

```bash
# Build
go build -o build/lazyctr .

# Run
sudo ./build/lazyctr
```

## Usage

Start the application with sudo:

```bash
sudo lazyctr
```

### Command-line Options

```bash
# Use default snapshotter (overlayfs)
sudo lazyctr

# Specify a different snapshotter
sudo lazyctr --snapshotter native
sudo lazyctr --snapshotter btrfs
sudo lazyctr --snapshotter zfs
```

## Keyboard Shortcuts

| Key | Action |
|-----|--------|
| `q`, `Q` | Quit application |
| `d` | Delete selected item (with confirmation) |
| `D` | Delete entire namespace (when in namespace panel) |
| `a`, `A` | Delete ALL items in current view (with confirmation) |
| `t`, `T` | Tag selected image (only in Images view) |
| `/` | Search/filter items by name |
| `1` | Jump to Images |
| `2` | Jump to Containers |
| `3` | Jump to Tasks |
| `4` | Jump to Snapshots |
| `5` | Jump to Content |
| `Tab` | Cycle focus: Namespaces → Resources → Items |
| `Shift+Tab` | Cycle focus backward |
| `↑`, `↓` | Navigate up/down in lists |
| `Enter` | Close search box (keeps filter active) |
| `?` | Show help |
| `Esc` | Clear search filter / Close dialog |

## Workflow Examples

### Example 1: Delete all stopped containers

```
1. Press '2' to jump to Containers
2. Press '/' and type "Stopped" to filter
3. Press Enter to close search box
4. Press 'a' to delete all filtered containers
5. Confirm deletion
```

### Example 2: Clean up old images

```
1. Press '1' to jump to Images
2. Navigate through namespaces (left panel)
3. Press 'a' to delete all images in namespace
4. Or 'd' to delete specific images one by one
```

### Example 3: Monitor running tasks

```
1. Press '3' to jump to Tasks
2. View all running container processes
3. See PIDs and status in real-time
```

### Example 4: Manage snapshots

```
1. Press '4' to jump to Snapshots
2. View filesystem layers and their relationships
3. Delete orphaned snapshots with 'd'
```

### Example 5: Tag an image

```
1. Press '1' to jump to Images
2. Navigate to the image you want to tag
3. Press 't' to open the tag dialog
4. Enter the new tag name (e.g., myapp:v2.0)
5. Press Enter to create the tag
6. The new tag will appear in the image list
```

## Delete Operations

### Delete Single Item (`d`)
- Deletes the currently selected item
- Requires confirmation
- Works on any resource type

### Delete All (`a`)
- Deletes ALL items in the current view
- Respects active search filters
- Shows count before deletion
- Requires confirmation
- Displays success/failure summary

### Delete Namespace (`D`)
- Only available when namespace panel has focus
- Deletes the entire namespace and ALL its resources
- Requires strong confirmation
- Cannot be undone!

## Search Functionality

1. Press `/` to open search box
2. Type to filter items in real-time (case-insensitive)
3. Press `Enter` to close search box (filter remains active)
4. Perform actions on filtered items
5. Press `Esc` to clear filter and show all items

## Building

### Standard Build

```bash
go build -o build/lazyctr .
```

### Static Binary (Recommended)

```bash
CGO_ENABLED=0 go build -ldflags '-s -w' -o build/lazyctr .
```

This creates a fully static binary with:
- No external dependencies
- CGO disabled
- Stripped symbols for smaller size (~17MB)
- Works on any Linux system

### ARM64 Build

```bash
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags '-s -w' -o build/lazyctr-arm64 .
```

## Architecture

### File Structure

```
.
├── main.go              # Main application (1150+ lines)
├── go.mod               # Go module dependencies
├── go.sum               # Dependency checksums
└── README.md            # This file
```

### Resource Type System

Each resource type implements:
- `load{Resource}()` - Fetch data from containerd
- `render{Resource}Table()` - Display in table format
- Delete operations in `performDelete()` and `performDeleteAll()`
- Tag operations in `tagImage()` and `performTag()` (Images only)

### Dependencies

- `github.com/rivo/tview` - Terminal UI framework
- `github.com/containerd/containerd` - Containerd client library
- `github.com/gdamore/tcell/v2` - Terminal cell library
- `github.com/opencontainers/go-digest` - Content digest handling

## Troubleshooting

### Permission Denied

```
Failed to connect to containerd: permission denied
```

**Solution**: Run with sudo:
```bash
sudo ctr-tui
```

### Socket Not Found

```
Failed to connect to containerd: no such file or directory
```

**Solution**: Ensure containerd is running:
```bash
sudo systemctl status containerd
```

If using a different socket path, modify line 101 in `main.go`:
```go
client, err := containerd.New("/run/containerd/containerd.sock")
```

### No Resources Found

If no resources appear:
- Check that containerd has data: `sudo ctr namespace ls`
- Verify namespace exists: `sudo ctr -n k8s.io images list`
- Restart the app to refresh

### Snapshotter Not Found

If snapshots don't load, you may be using a different snapshotter. Use the `--snapshotter` flag:
```bash
sudo lazyctr --snapshotter native
sudo lazyctr --snapshotter btrfs
```

Or check which snapshotter is configured:
```bash
sudo ctr plugins ls | grep io.containerd.snapshotter
```

## Comparison with ctr

### Traditional ctr commands:

```bash
# List namespaces
sudo ctr namespace ls

# List images
sudo ctr -n k8s.io images list

# List containers
sudo ctr -n k8s.io containers list

# List tasks
sudo ctr -n k8s.io tasks list

# Delete image
sudo ctr -n k8s.io images rm docker.io/library/nginx:latest

# Delete container
sudo ctr -n k8s.io containers rm my-container
```

### With lazyctr:

1. Run `sudo lazyctr`
2. Navigate with arrow keys
3. Press `1-5` to switch between resource types
4. Press `d` to delete, `a` to delete all, or `t` to tag images
5. Everything visible in one interface!

## Performance

- **Binary Size**: ~17MB (static)
- **Startup Time**: <100ms
- **Memory Usage**: ~30-40MB RAM
- **Resource Loading**: Fast (async per resource type)
- **UI Rendering**: Smooth 60fps

## Advanced Features

### Filter + Delete All

Search filters affect "Delete All" operations:

```
1. Press '/' and search for "redis"
2. Press Enter to close search (filter stays active)
3. Press 'a' to delete all filtered items
4. Only redis images/containers will be deleted!
```

### Multi-Namespace Cleanup

Quickly clean up multiple namespaces:

```
1. Select namespace (left panel)
2. Press 'a' to delete all resources
3. Use ↑/↓ to move to next namespace
4. Repeat
```

### Resource Type Jump

Quick navigation with number keys:

- `1` = Images (most common)
- `2` = Containers
- `3` = Tasks (monitor running)
- `4` = Snapshots (advanced)
- `5` = Content (debugging)

## Safety Features

✅ All destructive operations require confirmation
✅ Delete All shows exact count before proceeding
✅ Search filters clearly indicated in title
✅ Cannot delete while confirmation dialog is open
✅ Failed deletions reported with error count

## Known Limitations

- No real-time refresh (restart app to reload)
- Content deletion may fail if blobs are in use
- Task deletion requires container to be stopped first
- Image tagging creates a new reference (doesn't modify original)

## Future Enhancements

Potential features for future versions:

- [ ] Auto-refresh mode
- [ ] Configurable snapshotter selection
- [ ] Export resource list to CSV/JSON
- [ ] Resource usage statistics
- [ ] Color theme customization
- [ ] Multi-select for batch operations
- [ ] Leases management

## Contributing

Feel free to submit issues and enhancement requests!

## License

See LICENSE file for details.

## Similar Tools

- [k9s](https://k9scli.io/) - Kubernetes CLI
- [lazydocker](https://github.com/jesseduffield/lazydocker) - Docker TUI
- [ctop](https://github.com/bcicen/ctop) - Container metrics

## Credits

Built with:
- Go programming language
- tview (Terminal UI framework)
- containerd client libraries
- tcell (Terminal cell library)
