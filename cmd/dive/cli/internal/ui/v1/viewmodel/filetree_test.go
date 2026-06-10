package viewmodel

import (
	"flag"
	"fmt"
	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wagoodman/dive/cmd/dive/cli/internal/ui/v1"
	"go.uber.org/atomic"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/wagoodman/dive/dive/filetree"
	"github.com/wagoodman/dive/dive/image/docker"
)

var repoRootCache atomic.String

var updateSnapshot = flag.Bool("update", false, "update any test snapshots")

func TestUpdateSnapshotDisabled(t *testing.T) {
	require.False(t, *updateSnapshot, "update snapshot flag should be disabled")
}

func fileExists(filename string) bool {
	info, err := os.Stat(filename)
	if os.IsNotExist(err) {
		return false
	}
	return !info.IsDir()
}

func testCaseDataFilePath(name string) string {
	return filepath.Join("testdata", name+".txt")
}

func helperLoadBytes(t *testing.T) []byte {
	t.Helper()
	path := testCaseDataFilePath(t.Name())
	theBytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("unable to load test data ('%s'): %+v", t.Name(), err)
	}
	return theBytes
}

func helperCaptureBytes(t *testing.T, data []byte) {
	// TODO: switch to https://github.com/gkampitakis/go-snaps
	t.Helper()
	if *updateSnapshot {
		t.Fatalf("cannot capture data in test mode: %s", t.Name())
	}

	path := testCaseDataFilePath(t.Name())
	err := os.WriteFile(path, data, 0644)

	if err != nil {
		t.Fatalf("unable to save test data ('%s'): %+v", t.Name(), err)
	}
}

func initializeTestViewModel(t *testing.T) *FileTreeViewModel {
	t.Helper()

	result := docker.TestAnalysisFromArchive(t, repoPath(t, ".data/test-docker-image.tar"))
	require.NotNil(t, result, "unable to load test data")

	vm, err := NewFileTreeViewModel(v1.Config{
		Analysis:    *result,
		Preferences: v1.DefaultPreferences(),
	}, 0)

	require.NoError(t, err, "unable to create viewmodel")
	return vm
}

func runTestCase(t *testing.T, vm *FileTreeViewModel, width, height int, filterRegex *regexp.Regexp) {
	t.Helper()
	err := vm.Update(filterRegex, width, height)
	if err != nil {
		t.Errorf("failed to update viewmodel: %v", err)
	}

	err = vm.Render()
	if err != nil {
		t.Errorf("failed to render viewmodel: %v", err)
	}

	actualBytes := vm.Buffer.Bytes()
	path := testCaseDataFilePath(t.Name())
	if !fileExists(path) {
		if *updateSnapshot {
			helperCaptureBytes(t, actualBytes)
		} else {
			t.Fatalf("missing test data: %s", path)
		}
	}
	expectedBytes := helperLoadBytes(t)
	if d := cmp.Diff(string(expectedBytes), string(actualBytes)); d != "" {
		t.Errorf("bytes mismatch (-want +got):\n%s", d)
	}
}

func checkError(t *testing.T, err error, message string) {
	if err != nil {
		t.Errorf(message+": %+v", err)
	}
}

func TestFileTreeGoCase(t *testing.T) {
	vm := initializeTestViewModel(t)

	width, height := 100, 1000
	vm.Setup(0, height)
	vm.ShowAttributes = true

	runTestCase(t, vm, width, height, nil)
}

func TestFileTreeNoAttributes(t *testing.T) {
	vm := initializeTestViewModel(t)

	width, height := 100, 1000
	vm.Setup(0, height)
	vm.ShowAttributes = false

	runTestCase(t, vm, width, height, nil)
}

func TestFileTreeRestrictedHeight(t *testing.T) {
	vm := initializeTestViewModel(t)

	width, height := 100, 20
	vm.Setup(0, height)
	vm.ShowAttributes = false

	runTestCase(t, vm, width, height, nil)
}

func TestFileTreeDirCollapse(t *testing.T) {
	vm := initializeTestViewModel(t)

	width, height := 100, 100
	vm.Setup(0, height)
	vm.ShowAttributes = true

	assertPath(t, vm, "/bin", "before toggle of bin")

	// collapse /bin
	err := vm.ToggleCollapse(nil)
	checkError(t, err, "unable to collapse /bin")
	assertPath(t, vm, "/bin", "after toggle of bin")

	moved := vm.CursorDown() // select /dev
	require.True(t, moved, "unable to cursor down")
	assertPath(t, vm, "/dev", "down to dev")

	moved = vm.CursorDown() // select /etc
	require.True(t, moved, "unable to cursor down")
	assertPath(t, vm, "/etc", "down to etc")

	// collapse /etc
	err = vm.ToggleCollapse(nil)
	checkError(t, err, "unable to collapse /etc")
	assertPath(t, vm, "/etc", "after toggle of etc")

	runTestCase(t, vm, width, height, nil)
}

func assertPath(t *testing.T, vm *FileTreeViewModel, expected string, msg string) {
	t.Helper()
	n := vm.CurrentNode(nil)
	require.NotNil(t, n, "unable to get current node")
	assert.Equal(t, expected, n.Path(), msg)
}

func TestFileTreeDirCollapseAll(t *testing.T) {
	vm := initializeTestViewModel(t)

	width, height := 100, 100
	vm.Setup(0, height)
	vm.ShowAttributes = true

	err := vm.ToggleCollapseAll()
	checkError(t, err, "unable to collapse all dir")

	runTestCase(t, vm, width, height, nil)
}

func TestFileTreeSelectLayer(t *testing.T) {
	vm := initializeTestViewModel(t)

	width, height := 100, 100
	vm.Setup(0, height)
	vm.ShowAttributes = true

	// collapse /bin
	err := vm.ToggleCollapse(nil)
	checkError(t, err, "unable to collapse /bin")

	// select the next layer, compareMode = layer
	err = vm.SetTreeByLayer(0, 0, 1, 1)
	if err != nil {
		t.Errorf("unable to SetTreeByLayer: %v", err)
	}
	runTestCase(t, vm, width, height, nil)
}

func TestFileShowAggregateChanges(t *testing.T) {
	vm := initializeTestViewModel(t)

	width, height := 100, 100
	vm.Setup(0, height)
	vm.ShowAttributes = true

	// collapse /bin
	err := vm.ToggleCollapse(nil)
	checkError(t, err, "unable to collapse /bin")

	// select the next layer, compareMode = layer
	err = vm.SetTreeByLayer(0, 0, 1, 13)
	checkError(t, err, "unable to SetTreeByLayer")

	runTestCase(t, vm, width, height, nil)
}

func TestFileTreePageDown(t *testing.T) {
	vm := initializeTestViewModel(t)

	width, height := 100, 10
	vm.Setup(0, height)
	vm.ShowAttributes = true
	err := vm.Update(nil, width, height)
	checkError(t, err, "unable to update")

	err = vm.PageDown()
	checkError(t, err, "unable to page down")

	err = vm.PageDown()
	checkError(t, err, "unable to page down")

	err = vm.PageDown()
	checkError(t, err, "unable to page down")

	runTestCase(t, vm, width, height, nil)
}

func TestFileTreePageUp(t *testing.T) {
	vm := initializeTestViewModel(t)

	width, height := 100, 10
	vm.Setup(0, height)
	vm.ShowAttributes = true

	// these operations have a render step for intermediate results, which require at least one update to be done first
	err := vm.Update(nil, width, height)
	checkError(t, err, "unable to update")

	err = vm.PageDown()
	checkError(t, err, "unable to page down")

	err = vm.PageDown()
	checkError(t, err, "unable to page down")

	err = vm.PageUp()
	checkError(t, err, "unable to page up")

	runTestCase(t, vm, width, height, nil)
}

func TestFileTreeDirCursorRight(t *testing.T) {
	vm := initializeTestViewModel(t)

	width, height := 100, 100
	vm.Setup(0, height)
	vm.ShowAttributes = true

	// collapse /bin
	err := vm.ToggleCollapse(nil)
	checkError(t, err, "unable to collapse /bin")

	moved := vm.CursorDown()
	if !moved {
		t.Error("unable to cursor down")
	}

	moved = vm.CursorDown()
	if !moved {
		t.Error("unable to cursor down")
	}

	// collapse /etc
	err = vm.ToggleCollapse(nil)
	checkError(t, err, "unable to collapse /etc")

	// expand /etc
	err = vm.CursorRight(nil)
	checkError(t, err, "unable to cursor right")

	runTestCase(t, vm, width, height, nil)
}

func TestFileTreeFilterTree(t *testing.T) {
	vm := initializeTestViewModel(t)

	width, height := 100, 1000
	vm.Setup(0, height)
	vm.ShowAttributes = true

	regex, err := regexp.Compile("network")
	if err != nil {
		t.Errorf("could not create filter regex: %+v", err)
	}

	runTestCase(t, vm, width, height, regex)
}

func TestFileTreeHideAddedRemovedModified(t *testing.T) {
	vm := initializeTestViewModel(t)

	width, height := 100, 100
	vm.Setup(0, height)
	vm.ShowAttributes = true

	// collapse /bin
	err := vm.ToggleCollapse(nil)
	checkError(t, err, "unable to collapse /bin")

	// select the 7th layer, compareMode = layer
	err = vm.SetTreeByLayer(0, 0, 1, 7)
	if err != nil {
		t.Errorf("unable to SetTreeByLayer: %v", err)
	}

	// hide added files
	vm.ToggleShowDiffType(filetree.Added)

	// hide modified files
	vm.ToggleShowDiffType(filetree.Modified)

	// hide removed files
	vm.ToggleShowDiffType(filetree.Removed)

	runTestCase(t, vm, width, height, nil)
}

func TestFileTreeHideUnmodified(t *testing.T) {
	vm := initializeTestViewModel(t)

	width, height := 100, 100
	vm.Setup(0, height)
	vm.ShowAttributes = true

	// collapse /bin
	err := vm.ToggleCollapse(nil)
	checkError(t, err, "unable to collapse /bin")

	// select the 7th layer, compareMode = layer
	err = vm.SetTreeByLayer(0, 0, 1, 7)
	if err != nil {
		t.Errorf("unable to SetTreeByLayer: %v", err)
	}

	// hide unmodified files
	vm.ToggleShowDiffType(filetree.Unmodified)

	runTestCase(t, vm, width, height, nil)
}

func TestFileTreeHideTypeWithFilter(t *testing.T) {
	vm := initializeTestViewModel(t)

	width, height := 100, 100
	vm.Setup(0, height)
	vm.ShowAttributes = true

	// collapse /bin
	err := vm.ToggleCollapse(nil)
	checkError(t, err, "unable to collapse /bin")

	// select the 7th layer, compareMode = layer
	err = vm.SetTreeByLayer(0, 0, 1, 7)
	if err != nil {
		t.Errorf("unable to SetTreeByLayer: %v", err)
	}

	// hide added files
	vm.ToggleShowDiffType(filetree.Added)

	regex, err := regexp.Compile("saved")
	if err != nil {
		t.Errorf("could not create filter regex: %+v", err)
	}

	runTestCase(t, vm, width, height, regex)
}

// assertCursorInvariants verifies all cursor/viewport invariants hold.
func assertCursorInvariants(t *testing.T, vm *FileTreeViewModel, context string) {
	t.Helper()
	visibleSize := vm.ModelTree.VisibleSize()

	assert.GreaterOrEqual(t, vm.TreeIndex, 0, "%s: TreeIndex negative", context)
	if visibleSize > 0 {
		assert.LessOrEqual(t, vm.TreeIndex, visibleSize-1, "%s: TreeIndex exceeds visible size", context)
	}
	assert.GreaterOrEqual(t, vm.bufferIndexLowerBound, 0, "%s: bufferIndexLowerBound negative", context)
	assert.LessOrEqual(t, vm.bufferIndexLowerBound, vm.TreeIndex, "%s: bufferIndexLowerBound > TreeIndex", context)
	assert.GreaterOrEqual(t, vm.bufferIndex, 0, "%s: bufferIndex negative", context)
	assert.LessOrEqual(t, vm.bufferIndex, vm.height(), "%s: bufferIndex > height", context)
	assert.Equal(t, vm.TreeIndex-vm.bufferIndexLowerBound, vm.bufferIndex, "%s: bufferIndex inconsistent", context)

	node := vm.CurrentNode(nil)
	if visibleSize > 0 {
		assert.NotNil(t, node, "%s: CurrentNode returned nil with visible nodes", context)
	}
}

func TestCursorClampOnHideDiffType(t *testing.T) {
	vm := initializeTestViewModel(t)

	width, height := 100, 20
	vm.Setup(0, height)

	// move cursor deep into the tree
	err := vm.Update(nil, width, height)
	require.NoError(t, err)
	for i := 0; i < 15; i++ {
		vm.CursorDown()
	}
	require.Equal(t, 15, vm.TreeIndex)
	assertCursorInvariants(t, vm, "before hide")

	// hide unmodified files — this can drastically reduce visible count
	vm.ToggleShowDiffType(filetree.Unmodified)
	err = vm.Update(nil, width, height)
	require.NoError(t, err)

	assertCursorInvariants(t, vm, "after hiding unmodified")

	err = vm.Render()
	require.NoError(t, err, "render should not fail after hiding diff type")
}

func TestCursorClampOnLayerSwitch(t *testing.T) {
	vm := initializeTestViewModel(t)

	width, height := 100, 100
	vm.Setup(0, height)
	err := vm.Update(nil, width, height)
	require.NoError(t, err)

	// move cursor far down
	for i := 0; i < 30; i++ {
		vm.CursorDown()
	}
	assertCursorInvariants(t, vm, "before layer switch")

	// switch to a different layer which may have a smaller visible tree
	err = vm.SetTreeByLayer(0, 0, 1, 1)
	require.NoError(t, err)

	err = vm.Update(nil, width, height)
	require.NoError(t, err)

	assertCursorInvariants(t, vm, "after layer switch")

	err = vm.Render()
	require.NoError(t, err, "render should not fail after layer switch")
}

func TestCursorClampOnFilterApply(t *testing.T) {
	vm := initializeTestViewModel(t)

	width, height := 100, 100
	vm.Setup(0, height)
	err := vm.Update(nil, width, height)
	require.NoError(t, err)

	// move cursor far down
	for i := 0; i < 25; i++ {
		vm.CursorDown()
	}
	require.Equal(t, 25, vm.TreeIndex)
	assertCursorInvariants(t, vm, "before filter")

	// apply a very restrictive filter that matches very few nodes
	regex, err := regexp.Compile("network")
	require.NoError(t, err)

	err = vm.Update(regex, width, height)
	require.NoError(t, err)

	assertCursorInvariants(t, vm, "after restrictive filter")

	err = vm.Render()
	require.NoError(t, err, "render should not fail after filter")
}

func TestPageUpAtTop(t *testing.T) {
	vm := initializeTestViewModel(t)

	width, height := 100, 10
	vm.Setup(0, height)
	err := vm.Update(nil, width, height)
	require.NoError(t, err)

	// page up from the very top — should be a no-op, not produce negative indices
	err = vm.PageUp()
	require.NoError(t, err)
	assertCursorInvariants(t, vm, "page up from top")

	// move down a few lines, then page up repeatedly
	for i := 0; i < 5; i++ {
		vm.CursorDown()
	}
	err = vm.PageUp()
	require.NoError(t, err)
	assertCursorInvariants(t, vm, "page up after small movement")

	err = vm.Render()
	require.NoError(t, err)
}

func TestPageDownAtBottom(t *testing.T) {
	vm := initializeTestViewModel(t)

	width, height := 100, 10
	vm.Setup(0, height)
	err := vm.Update(nil, width, height)
	require.NoError(t, err)

	visibleSize := vm.ModelTree.VisibleSize()

	// page down repeatedly until well past the end
	for i := 0; i < (visibleSize/height)+5; i++ {
		err = vm.PageDown()
		require.NoError(t, err, "page down iteration %d", i)
		assertCursorInvariants(t, vm, fmt.Sprintf("page down iteration %d", i))
	}

	err = vm.Render()
	require.NoError(t, err)
}

func repoPath(t testing.TB, path string) string {
	t.Helper()
	root := repoRoot(t)
	return filepath.Join(root, path)
}

func repoRoot(t testing.TB) string {
	val := repoRootCache.Load()
	if val != "" {
		return val
	}
	t.Helper()
	// use git to find the root of the repo
	out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		t.Fatalf("failed to get repo root: %v", err)
	}
	val = strings.TrimSpace(string(out))
	repoRootCache.Store(val)
	return val
}
