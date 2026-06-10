package viewmodel

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wagoodman/dive/dive/filetree"
)

// --- helpers ---

// setupVM creates a viewmodel with the given viewport height and returns it ready for testing.
func setupVM(t *testing.T, height int) *FileTreeViewModel {
	t.Helper()
	vm := initializeTestViewModel(t)
	vm.Setup(0, height)
	vm.ShowAttributes = false
	return vm
}

// updateVM calls Update+Render with the given params.
func updateVM(t *testing.T, vm *FileTreeViewModel, filter *regexp.Regexp, width, height int) {
	t.Helper()
	require.NoError(t, vm.Update(filter, width, height))
	require.NoError(t, vm.Render())
}

// cursorPath returns the path of the currently selected node (nil-safe).
func cursorPath(vm *FileTreeViewModel, filter *regexp.Regexp) string {
	n := vm.getAbsPositionNode(filter)
	if n == nil {
		return "<nil>"
	}
	return n.Path()
}

// assertCursorNotNil verifies the cursor points to a real node.
func assertCursorNotNil(t *testing.T, vm *FileTreeViewModel, filter *regexp.Regexp, msg string) {
	t.Helper()
	n := vm.getAbsPositionNode(filter)
	assert.NotNil(t, n, "cursor should not be nil: %s", msg)
}

// assertCursorBounds verifies TreeIndex and bufferIndex are within valid ranges.
func assertCursorBounds(t *testing.T, vm *FileTreeViewModel, msg string) {
	t.Helper()
	assert.GreaterOrEqual(t, vm.TreeIndex, 0, "%s: TreeIndex should be >= 0", msg)
	assert.GreaterOrEqual(t, vm.bufferIndex, 0, "%s: bufferIndex should be >= 0", msg)
	assert.GreaterOrEqual(t, vm.bufferIndexLowerBound, 0, "%s: bufferIndexLowerBound should be >= 0", msg)
}

// --- Collapse + PageDown boundary ---

func TestRegression_CollapseAllThenPageDown(t *testing.T) {
	vm := setupVM(t, 15)
	updateVM(t, vm, nil, 100, 15)

	// collapse all directories — this drastically reduces visible size
	require.NoError(t, vm.ToggleCollapseAll())
	updateVM(t, vm, nil, 100, 15)

	visibleBefore := vm.ModelTree.VisibleSize()

	// page down multiple times — must not go out of bounds
	for i := 0; i < 10; i++ {
		require.NoError(t, vm.PageDown())
		assertCursorBounds(t, vm, "after PageDown iteration")
	}

	// cursor should never exceed visible size
	assert.LessOrEqual(t, vm.TreeIndex, visibleBefore,
		"TreeIndex should not exceed visible size after repeated PageDown")
	assertCursorNotNil(t, vm, nil, "after collapse-all + page-down")
}

// --- Collapse + PageUp boundary ---

func TestRegression_CollapseAllThenPageUp(t *testing.T) {
	vm := setupVM(t, 15)
	updateVM(t, vm, nil, 100, 15)

	require.NoError(t, vm.ToggleCollapseAll())
	updateVM(t, vm, nil, 100, 15)

	// go down first, then up
	for i := 0; i < 3; i++ {
		require.NoError(t, vm.PageDown())
	}
	for i := 0; i < 10; i++ {
		require.NoError(t, vm.PageUp())
		assertCursorBounds(t, vm, "after PageUp iteration")
	}
	assertCursorNotNil(t, vm, nil, "after collapse-all + page-up")
}

// --- Repeated PageDown at the bottom ---

func TestRegression_RepeatedPageDownAtBottom(t *testing.T) {
	vm := setupVM(t, 5)
	updateVM(t, vm, nil, 100, 5)

	// page down until we can't anymore (way past the tree size)
	for i := 0; i < 100; i++ {
		require.NoError(t, vm.PageDown())
	}

	assertCursorBounds(t, vm, "after exhaustive PageDown")
	assertCursorNotNil(t, vm, nil, "cursor should still be valid at bottom")
}

// --- Repeated PageUp at the top ---

func TestRegression_RepeatedPageUpAtTop(t *testing.T) {
	vm := setupVM(t, 5)
	updateVM(t, vm, nil, 100, 5)

	// page up from the very top
	for i := 0; i < 10; i++ {
		require.NoError(t, vm.PageUp())
		assertCursorBounds(t, vm, "after PageUp at top")
	}

	assertCursorNotNil(t, vm, nil, "cursor should still be valid at top")
}

// --- Filter + Collapse interaction ---

func TestRegression_FilterThenCollapse(t *testing.T) {
	vm := setupVM(t, 50)

	regex, err := regexp.Compile("etc")
	require.NoError(t, err)

	updateVM(t, vm, regex, 100, 50)

	// collapse the first visible node
	require.NoError(t, vm.ToggleCollapse(regex))
	updateVM(t, vm, regex, 100, 50)

	assertCursorNotNil(t, vm, nil, "after filter + collapse")
	assertCursorBounds(t, vm, "after filter + collapse")
}

func TestRegression_CollapseThenFilter(t *testing.T) {
	vm := setupVM(t, 50)

	// first collapse /bin at position 0
	require.NoError(t, vm.ToggleCollapse(nil))
	updateVM(t, vm, nil, 100, 50)

	// now apply a filter that matches inside the collapsed dir
	regex, err := regexp.Compile("etc")
	require.NoError(t, err)

	updateVM(t, vm, regex, 100, 50)
	assertCursorNotNil(t, vm, nil, "after collapse + filter")
	assertCursorBounds(t, vm, "after collapse + filter")
}

// --- CollapseAll + Filter ---

func TestRegression_CollapseAllWithFilter(t *testing.T) {
	vm := setupVM(t, 30)

	require.NoError(t, vm.ToggleCollapseAll())

	regex, err := regexp.Compile("bin")
	require.NoError(t, err)

	updateVM(t, vm, regex, 100, 30)

	assertCursorBounds(t, vm, "collapse-all + filter")
	assertCursorNotNil(t, vm, nil, "collapse-all + filter")

	// navigating should not panic
	vm.CursorDown()
	vm.CursorDown()
	assertCursorBounds(t, vm, "collapse-all + filter + cursor-down")
}

// --- SetTreeByLayer preserves collapse state ---

func TestRegression_SetTreeByLayerPreservesCollapse(t *testing.T) {
	vm := setupVM(t, 100)
	vm.ShowAttributes = true
	updateVM(t, vm, nil, 100, 100)

	// collapse /bin (first node at TreeIndex 0)
	require.NoError(t, vm.ToggleCollapse(nil))
	collapsedPath := cursorPath(vm, nil)

	// move cursor down so we have a non-zero TreeIndex
	vm.CursorDown()
	vm.CursorDown()
	pathBeforeSwitch := cursorPath(vm, nil)
	_ = pathBeforeSwitch

	// switch to layer 1
	err := vm.SetTreeByLayer(0, 0, 1, 1)
	require.NoError(t, err)

	updateVM(t, vm, nil, 100, 100)

	// the collapsed path should still be collapsed in the new tree
	node, err := vm.ModelTree.GetNode(collapsedPath)
	if err == nil && node != nil && node.Data.FileInfo.IsDir {
		assert.True(t, node.Data.ViewInfo.Collapsed,
			"collapse state for %s should be preserved after layer switch", collapsedPath)
	}
}

// --- SetTreeByLayer + Filter + Cursor ---

func TestRegression_SwitchLayerWithFilter(t *testing.T) {
	vm := setupVM(t, 50)

	regex, err := regexp.Compile("usr")
	require.NoError(t, err)
	updateVM(t, vm, regex, 100, 50)

	// switch layers while filter is active
	err = vm.SetTreeByLayer(0, 0, 1, 1)
	require.NoError(t, err)
	updateVM(t, vm, regex, 100, 50)

	assertCursorBounds(t, vm, "after layer switch with filter")
	assertCursorNotNil(t, vm, nil, "after layer switch with filter")
}

// --- CursorRight with filter ---

func TestRegression_CursorRightWithFilter(t *testing.T) {
	vm := setupVM(t, 50)

	// collapse root-level dirs first
	require.NoError(t, vm.ToggleCollapseAll())
	updateVM(t, vm, nil, 100, 50)

	regex, err := regexp.Compile("etc")
	require.NoError(t, err)
	updateVM(t, vm, regex, 100, 50)

	// cursor right should expand the current collapsed dir
	err = vm.CursorRight(regex)
	require.NoError(t, err)

	assertCursorBounds(t, vm, "after CursorRight with filter")
}

// --- CursorLeft after collapse ---

func TestRegression_CursorLeftAfterCollapse(t *testing.T) {
	vm := setupVM(t, 100)
	updateVM(t, vm, nil, 100, 100)

	// navigate down into a directory structure
	for i := 0; i < 5; i++ {
		vm.CursorDown()
	}
	deepPath := cursorPath(vm, nil)

	// CursorLeft should go to parent
	err := vm.CursorLeft(nil)
	require.NoError(t, err)

	parentPath := cursorPath(vm, nil)
	assert.NotEqual(t, deepPath, parentPath, "CursorLeft should move to a different node")
	assertCursorBounds(t, vm, "after CursorLeft")
}

func TestRegression_CursorLeftAtRoot(t *testing.T) {
	vm := setupVM(t, 100)
	updateVM(t, vm, nil, 100, 100)

	// CursorLeft at the very first node (no parent to go to)
	err := vm.CursorLeft(nil)
	require.NoError(t, err)

	assertCursorBounds(t, vm, "CursorLeft at root level")
}

// --- HideDiffType + Collapse + PageDown triple combo ---

func TestRegression_HideDiffTypeCollapsePageDown(t *testing.T) {
	vm := setupVM(t, 10)
	vm.ShowAttributes = true

	// switch to a layer with changes
	err := vm.SetTreeByLayer(0, 0, 1, 7)
	require.NoError(t, err)

	// hide added and removed — only show modified and unmodified
	vm.ToggleShowDiffType(filetree.Added)
	vm.ToggleShowDiffType(filetree.Removed)
	updateVM(t, vm, nil, 100, 10)

	// collapse the first visible dir
	require.NoError(t, vm.ToggleCollapse(nil))
	updateVM(t, vm, nil, 100, 10)

	// page through
	for i := 0; i < 20; i++ {
		require.NoError(t, vm.PageDown())
		assertCursorBounds(t, vm, "hide-diff + collapse + pagedown")
	}

	assertCursorNotNil(t, vm, nil, "after hide-diff + collapse + pagedown")
}

// --- HideDiffType + Filter combined ---

func TestRegression_HideDiffTypeWithFilter(t *testing.T) {
	vm := setupVM(t, 30)

	err := vm.SetTreeByLayer(0, 0, 1, 7)
	require.NoError(t, err)

	// hide added files (not unmodified — hiding unmodified with a narrow filter
	// can leave zero visible nodes, which is a valid but different edge case)
	vm.ToggleShowDiffType(filetree.Added)

	regex, err := regexp.Compile("etc")
	require.NoError(t, err)

	updateVM(t, vm, regex, 100, 30)

	assertCursorBounds(t, vm, "hide-added + filter")
	// only assert cursor not nil if there are visible nodes
	if vm.ModelTree.VisibleSize() > 0 {
		assertCursorNotNil(t, vm, nil, "hide-added + filter")
	}

	// navigate
	for i := 0; i < 10; i++ {
		vm.CursorDown()
	}
	assertCursorBounds(t, vm, "hide-added + filter + cursor-down")
}

// --- CursorDown at VisibleSize boundary ---

func TestRegression_CursorDownAtVisibleBoundary(t *testing.T) {
	vm := setupVM(t, 1000)
	updateVM(t, vm, nil, 100, 1000)

	visibleSize := vm.ModelTree.VisibleSize()

	// move cursor to the last visible node
	for i := 0; i < visibleSize+5; i++ {
		vm.CursorDown()
	}

	// TreeIndex should be clamped to VisibleSize
	assert.LessOrEqual(t, vm.TreeIndex, visibleSize,
		"TreeIndex should not exceed VisibleSize")
	assertCursorBounds(t, vm, "at visible boundary")
}

// --- Filter that matches nothing ---

func TestRegression_FilterMatchesNothing(t *testing.T) {
	vm := setupVM(t, 20)

	// a regex that won't match any path in the test image
	regex, err := regexp.Compile("zzzzz_nonexistent_path_zzzzz")
	require.NoError(t, err)

	updateVM(t, vm, regex, 100, 20)

	assertCursorBounds(t, vm, "empty filter result")

	// operations on an empty tree should not panic
	vm.CursorDown()
	vm.CursorDown()
	require.NoError(t, vm.PageDown())
	require.NoError(t, vm.PageUp())
	require.NoError(t, vm.CursorRight(regex))
	require.NoError(t, vm.CursorLeft(regex))
	require.NoError(t, vm.ToggleCollapse(regex))

	assertCursorBounds(t, vm, "operations on empty filtered tree")
}

// --- Toggle CollapseAll back and forth ---

func TestRegression_CollapseAllToggleRoundtrip(t *testing.T) {
	vm := setupVM(t, 100)
	updateVM(t, vm, nil, 100, 100)

	sizeExpanded := vm.ModelTree.VisibleSize()

	// collapse all
	require.NoError(t, vm.ToggleCollapseAll())
	updateVM(t, vm, nil, 100, 100)
	sizeCollapsed := vm.ModelTree.VisibleSize()
	assert.Less(t, sizeCollapsed, sizeExpanded, "collapsed size should be smaller")

	// expand all
	require.NoError(t, vm.ToggleCollapseAll())
	updateVM(t, vm, nil, 100, 100)
	sizeReexpanded := vm.ModelTree.VisibleSize()
	assert.Equal(t, sizeExpanded, sizeReexpanded, "re-expanded size should match original")
}

// --- Layer switch with CollapseAll + PageDown ---

func TestRegression_LayerSwitchCollapseAllPageDown(t *testing.T) {
	vm := setupVM(t, 10)

	// collapse all, navigate, then switch layer
	require.NoError(t, vm.ToggleCollapseAll())
	updateVM(t, vm, nil, 100, 10)

	vm.CursorDown()
	vm.CursorDown()

	err := vm.SetTreeByLayer(0, 0, 1, 1)
	require.NoError(t, err)
	updateVM(t, vm, nil, 100, 10)

	// page down — should not panic or corrupt state
	for i := 0; i < 5; i++ {
		require.NoError(t, vm.PageDown())
		assertCursorBounds(t, vm, "layer-switch + collapse-all + page-down")
	}
}

// --- Aggregate mode (CompareAllLayers) + filter + collapse ---

func TestRegression_AggregateLayerFilterCollapse(t *testing.T) {
	vm := setupVM(t, 30)

	// aggregate all layers
	err := vm.SetTreeByLayer(0, 0, 1, len(vm.RefTrees)-1)
	require.NoError(t, err)

	regex, err := regexp.Compile("var")
	require.NoError(t, err)
	updateVM(t, vm, regex, 100, 30)

	// collapse current
	require.NoError(t, vm.ToggleCollapse(regex))
	updateVM(t, vm, regex, 100, 30)

	assertCursorBounds(t, vm, "aggregate + filter + collapse")
	assertCursorNotNil(t, vm, nil, "aggregate + filter + collapse")
}

// --- CursorRight then CursorLeft roundtrip ---

func TestRegression_CursorRightLeftRoundtrip(t *testing.T) {
	vm := setupVM(t, 100)
	updateVM(t, vm, nil, 100, 100)

	// collapse first dir
	require.NoError(t, vm.ToggleCollapse(nil))
	startPath := cursorPath(vm, nil)

	// CursorRight expands and enters the dir
	require.NoError(t, vm.CursorRight(nil))
	innerPath := cursorPath(vm, nil)
	assert.NotEqual(t, startPath, innerPath, "CursorRight should move into the dir")

	// CursorLeft should go back to parent
	require.NoError(t, vm.CursorLeft(nil))
	backPath := cursorPath(vm, nil)
	assert.Equal(t, startPath, backPath, "CursorLeft should return to the parent dir")
}

// --- PageDown then PageUp roundtrip stability ---

func TestRegression_PageDownUpRoundtripStability(t *testing.T) {
	vm := setupVM(t, 10)
	updateVM(t, vm, nil, 100, 10)

	startIndex := vm.TreeIndex
	startLower := vm.bufferIndexLowerBound

	// down 3 pages, then up 3 pages
	for i := 0; i < 3; i++ {
		require.NoError(t, vm.PageDown())
	}
	for i := 0; i < 3; i++ {
		require.NoError(t, vm.PageUp())
	}

	// should be back where we started (or very close)
	assert.Equal(t, startIndex, vm.TreeIndex,
		"TreeIndex should be restored after symmetric page-down/up")
	assert.Equal(t, startLower, vm.bufferIndexLowerBound,
		"bufferIndexLowerBound should be restored after symmetric page-down/up")
}

// --- Mixed navigation stress test ---

func TestRegression_MixedNavigationStress(t *testing.T) {
	vm := setupVM(t, 8)
	updateVM(t, vm, nil, 80, 8)

	// simulate a realistic user session mixing all operations
	ops := []func(){
		func() { vm.CursorDown() },
		func() { vm.CursorDown() },
		func() { vm.CursorDown() },
		func() { _ = vm.ToggleCollapse(nil) },
		func() { vm.CursorDown() },
		func() { _ = vm.PageDown() },
		func() { _ = vm.CursorRight(nil) },
		func() { vm.CursorDown() },
		func() { _ = vm.PageDown() },
		func() { _ = vm.PageDown() },
		func() { _ = vm.CursorLeft(nil) },
		func() { _ = vm.PageUp() },
		func() { vm.CursorUp() },
		func() { vm.CursorUp() },
		func() { _ = vm.ToggleCollapse(nil) },
		func() { _ = vm.PageDown() },
		func() { vm.CursorDown() },
		func() { _ = vm.ToggleCollapseAll() },
		func() { _ = vm.PageUp() },
		func() { _ = vm.PageUp() },
		func() { _ = vm.ToggleCollapseAll() }, // expand all
		func() { _ = vm.PageDown() },
		func() { vm.CursorDown() },
		func() { vm.CursorDown() },
	}

	for i, op := range ops {
		op()
		assertCursorBounds(t, vm, "stress op %d")
		_ = i
	}

	// final render must not panic
	updateVM(t, vm, nil, 80, 8)
	assertCursorBounds(t, vm, "after stress test")
}
