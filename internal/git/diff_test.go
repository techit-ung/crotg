package git

import "testing"

func TestParseUnifiedDiff_whenValidDiff_shouldParseFilesAndLines(t *testing.T) {
	// arrange
	diff := `diff --git a/example.txt b/example.txt
index 0000000..1111111 100644
--- a/example.txt
+++ b/example.txt
@@ -0,0 +1,2 @@
+hello
+world
`

	// act
	files, err := ParseUnifiedDiff(diff)

	// assert
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(files))
	}
	if files[0].Path != "example.txt" {
		t.Fatalf("expected file path example.txt, got %q", files[0].Path)
	}
	if len(files[0].Hunks) != 1 {
		t.Fatalf("expected 1 hunk, got %d", len(files[0].Hunks))
	}
	if len(files[0].Hunks[0].Lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(files[0].Hunks[0].Lines))
	}
}

func TestParseUnifiedDiff_whenEmptyDiff_shouldReturnError(t *testing.T) {
	// arrange
	diff := "   "

	// act
	_, err := ParseUnifiedDiff(diff)

	// assert
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
}
