package filetree

import (
	"archive/tar"
	"os"
	"testing"
)

func TestSummarize(t *testing.T) {
	// Build 3 layers:
	// Layer 0 (base): /etc/nginx.conf (2000 bytes), /etc/data.txt (3000 bytes)
	// Layer 1: adds /usr/app (500 bytes), modifies /etc/nginx.conf (mode change)
	// Layer 2: removes /etc/data.txt via whiteout

	trees := make([]*FileTree, 3)
	for i := range trees {
		trees[i] = NewFileTree()
	}

	dirInfo := func(p string) FileInfo {
		return FileInfo{Path: p, TypeFlag: tar.TypeDir, IsDir: true, Mode: os.ModeDir | 0755}
	}
	fileInfo := func(p string, size int64, mode os.FileMode) FileInfo {
		return FileInfo{Path: p, TypeFlag: tar.TypeReg, Size: size, Mode: mode}
	}

	// Layer 0: base
	trees[0].AddPath("/etc", dirInfo("/etc"))
	trees[0].AddPath("/etc/nginx.conf", fileInfo("/etc/nginx.conf", 2000, 0644))
	trees[0].AddPath("/etc/data.txt", fileInfo("/etc/data.txt", 3000, 0644))

	// Layer 1: add new file, modify existing (mode change triggers Modified)
	trees[1].AddPath("/usr", dirInfo("/usr"))
	trees[1].AddPath("/usr/app", fileInfo("/usr/app", 500, 0755))
	trees[1].AddPath("/etc", dirInfo("/etc"))
	trees[1].AddPath("/etc/nginx.conf", fileInfo("/etc/nginx.conf", 2500, 0600))

	// Layer 2: remove /etc/data.txt via whiteout
	trees[2].AddPath("/etc", dirInfo("/etc"))
	trees[2].AddPath("/etc/.wh.data.txt", fileInfo("/etc/.wh.data.txt", 0, 0))

	summaries, err := Summarize(trees)
	if err != nil {
		t.Fatal(err)
	}

	if len(summaries) != 3 {
		t.Fatalf("expected 3 summaries, got %d", len(summaries))
	}

	// Layer 0: all non-dir files are added
	s0 := summaries[0]
	if s0.AddedCount != 2 {
		t.Errorf("layer 0 AddedCount: expected 2, got %d", s0.AddedCount)
	}
	if s0.AddedBytes != 5000 {
		t.Errorf("layer 0 AddedBytes: expected 5000, got %d", s0.AddedBytes)
	}
	if s0.ModifiedCount != 0 || s0.RemovedCount != 0 {
		t.Errorf("layer 0: unexpected modified=%d removed=%d", s0.ModifiedCount, s0.RemovedCount)
	}

	// Layer 1: 1 added (/usr/app), 1 modified (/etc/nginx.conf mode change)
	s1 := summaries[1]
	if s1.AddedCount != 1 {
		t.Errorf("layer 1 AddedCount: expected 1, got %d", s1.AddedCount)
	}
	if s1.AddedBytes != 500 {
		t.Errorf("layer 1 AddedBytes: expected 500, got %d", s1.AddedBytes)
	}
	if s1.ModifiedCount != 1 {
		t.Errorf("layer 1 ModifiedCount: expected 1, got %d", s1.ModifiedCount)
	}
	if s1.ModifiedBytes != 2500 {
		t.Errorf("layer 1 ModifiedBytes: expected 2500, got %d", s1.ModifiedBytes)
	}

	// Layer 2: 1 removed (/etc/data.txt, 3000 bytes from base)
	s2 := summaries[2]
	if s2.RemovedCount != 1 {
		t.Errorf("layer 2 RemovedCount: expected 1, got %d", s2.RemovedCount)
	}
	if s2.RemovedBytes != 3000 {
		t.Errorf("layer 2 RemovedBytes: expected 3000, got %d", s2.RemovedBytes)
	}
	if s2.AddedCount != 0 || s2.ModifiedCount != 0 {
		t.Errorf("layer 2: unexpected added=%d modified=%d", s2.AddedCount, s2.ModifiedCount)
	}
}

func TestSummarize_empty(t *testing.T) {
	summaries, err := Summarize(nil)
	if err != nil {
		t.Fatal(err)
	}
	if summaries != nil {
		t.Errorf("expected nil, got %v", summaries)
	}
}
