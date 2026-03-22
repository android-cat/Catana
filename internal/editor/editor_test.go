package editor

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMovePathMovesFileAndOpenDocument(t *testing.T) {
	workspace := t.TempDir()
	srcDir := filepath.Join(workspace, "src")
	dstDir := filepath.Join(workspace, "dst")
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		t.Fatalf("MkdirAll(src) error = %v", err)
	}
	if err := os.MkdirAll(dstDir, 0755); err != nil {
		t.Fatalf("MkdirAll(dst) error = %v", err)
	}

	srcFile := filepath.Join(srcDir, "main.go")
	if err := os.WriteFile(srcFile, []byte("package main\n"), 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	state := NewEditorState(workspace)
	if err := state.OpenFile(srcFile); err != nil {
		t.Fatalf("OpenFile() error = %v", err)
	}

	newPath, err := state.MovePath(srcFile, dstDir)
	if err != nil {
		t.Fatalf("MovePath() error = %v", err)
	}
	wantPath := filepath.Join(dstDir, "main.go")
	if newPath != wantPath {
		t.Fatalf("MovePath() path = %q, want %q", newPath, wantPath)
	}
	if _, err := os.Stat(wantPath); err != nil {
		t.Fatalf("moved file stat error = %v", err)
	}
	if _, err := os.Stat(srcFile); !os.IsNotExist(err) {
		t.Fatalf("old file still exists or unexpected error: %v", err)
	}
	doc := state.ActiveDocument()
	if doc == nil {
		t.Fatal("ActiveDocument() = nil")
	}
	if doc.FilePath != wantPath {
		t.Fatalf("doc.FilePath = %q, want %q", doc.FilePath, wantPath)
	}
	if doc.FileName != "main.go" {
		t.Fatalf("doc.FileName = %q, want main.go", doc.FileName)
	}
}

func TestMovePathMovesDirectoryAndNestedDocuments(t *testing.T) {
	workspace := t.TempDir()
	srcDir := filepath.Join(workspace, "pkg")
	nestedDir := filepath.Join(srcDir, "nested")
	dstDir := filepath.Join(workspace, "archive")
	if err := os.MkdirAll(nestedDir, 0755); err != nil {
		t.Fatalf("MkdirAll(nested) error = %v", err)
	}
	if err := os.MkdirAll(dstDir, 0755); err != nil {
		t.Fatalf("MkdirAll(dst) error = %v", err)
	}

	srcFile := filepath.Join(nestedDir, "file.go")
	if err := os.WriteFile(srcFile, []byte("package nested\n"), 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	state := NewEditorState(workspace)
	if err := state.OpenFile(srcFile); err != nil {
		t.Fatalf("OpenFile() error = %v", err)
	}

	newDir, err := state.MovePath(srcDir, dstDir)
	if err != nil {
		t.Fatalf("MovePath() error = %v", err)
	}
	wantDir := filepath.Join(dstDir, "pkg")
	if newDir != wantDir {
		t.Fatalf("MovePath() dir = %q, want %q", newDir, wantDir)
	}
	wantFile := filepath.Join(wantDir, "nested", "file.go")
	if _, err := os.Stat(wantFile); err != nil {
		t.Fatalf("moved nested file stat error = %v", err)
	}
	doc := state.ActiveDocument()
	if doc == nil {
		t.Fatal("ActiveDocument() = nil")
	}
	if doc.FilePath != wantFile {
		t.Fatalf("doc.FilePath = %q, want %q", doc.FilePath, wantFile)
	}
}

func TestMovePathRejectsMoveIntoDescendant(t *testing.T) {
	workspace := t.TempDir()
	srcDir := filepath.Join(workspace, "pkg")
	childDir := filepath.Join(srcDir, "child")
	if err := os.MkdirAll(childDir, 0755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	state := NewEditorState(workspace)
	if _, err := state.MovePath(srcDir, childDir); err == nil {
		t.Fatal("MovePath() error = nil, want descendant move error")
	}
}

func TestRenamePathRenamesOpenFile(t *testing.T) {
	workspace := t.TempDir()
	filePath := filepath.Join(workspace, "old.go")
	if err := os.WriteFile(filePath, []byte("package main\n"), 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	state := NewEditorState(workspace)
	if err := state.OpenFile(filePath); err != nil {
		t.Fatalf("OpenFile() error = %v", err)
	}

	newPath, err := state.RenamePath(filePath, "new.go")
	if err != nil {
		t.Fatalf("RenamePath() error = %v", err)
	}
	want := filepath.Join(workspace, "new.go")
	if newPath != want {
		t.Fatalf("RenamePath() = %q, want %q", newPath, want)
	}
	if _, err := os.Stat(want); err != nil {
		t.Fatalf("renamed file stat error = %v", err)
	}
	if _, err := os.Stat(filePath); !os.IsNotExist(err) {
		t.Fatalf("old file still exists or unexpected error: %v", err)
	}
	doc := state.ActiveDocument()
	if doc == nil || doc.FilePath != want || doc.FileName != "new.go" {
		t.Fatalf("active doc = %+v, want path %q and name new.go", doc, want)
	}
}

func TestDeletePathRemovesNestedOpenDocuments(t *testing.T) {
	workspace := t.TempDir()
	dirPath := filepath.Join(workspace, "pkg")
	nestedDir := filepath.Join(dirPath, "nested")
	if err := os.MkdirAll(nestedDir, 0755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	filePath := filepath.Join(nestedDir, "file.go")
	if err := os.WriteFile(filePath, []byte("package nested\n"), 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	state := NewEditorState(workspace)
	if err := state.OpenFile(filePath); err != nil {
		t.Fatalf("OpenFile() error = %v", err)
	}

	if err := state.DeletePath(dirPath); err != nil {
		t.Fatalf("DeletePath() error = %v", err)
	}
	if _, err := os.Stat(dirPath); !os.IsNotExist(err) {
		t.Fatalf("dir still exists or unexpected error: %v", err)
	}
	if len(state.Documents) != 0 {
		t.Fatalf("len(Documents) = %d, want 0", len(state.Documents))
	}
	if state.ActiveDocument() != nil {
		t.Fatal("ActiveDocument() != nil after delete")
	}
}
