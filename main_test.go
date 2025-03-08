package main

import (
	"os"
	"strings"
	"testing"
)

func TestIsTextFile(t *testing.T) {
	textFile := "test_text.txt"
	binaryFile := "test_binary.bin"

	os.WriteFile(textFile, []byte("This is a text file.\nIt has multiple lines."), 0644)
	os.WriteFile(binaryFile, []byte{0x00, 0xFF, 0x00, 0xFF}, 0644)

	defer os.Remove(textFile)
	defer os.Remove(binaryFile)

	if !isTextFile(textFile) {
		t.Errorf("Expected %s to be identified as a text file.", textFile)
	}

	if isTextFile(binaryFile) {
		t.Errorf("Expected %s to be identified as a binary file.", binaryFile)
	}
}

func TestIsHiddenFile(t *testing.T) {
	tests := []struct {
		path     string
		expected bool
	}{
		{".hidden/file.txt", true},
		{"visible/.hiddenfile.txt", true},
		{"visible/file.txt", false},
		{"visible/folder/.hidden", true},
		{"visible/folder/file.txt", false},
	}

	for _, tt := range tests {
		result := isHiddenFile(tt.path)
		if result != tt.expected {
			t.Errorf("isHiddenFile(%q) = %v, want %v", tt.path, result, tt.expected)
		}
	}
}

func TestBuildOutput(t *testing.T) {
	testFiles := []string{"file1.txt", "file2.txt"}
	opts := Options{IncludeLineNumbers: true}

	os.WriteFile(testFiles[0], []byte("Line 1\nLine 2\n"), 0644)
	os.WriteFile(testFiles[1], []byte("Another file.\nLine 2."), 0644)

	output, count, _ := buildOutput(testFiles, opts)
	if cnt := strings.Count(output, "{{File: "); cnt != 3 {
		// one in the format description, one for each file
		t.Errorf("Expected 3 file markers, got %d", cnt)
	}
	if cnt := strings.Count(output, "\n"); cnt < 4 {
		t.Errorf("Expected multiple lines in the output, got %d", cnt)
	}

	for _, f := range testFiles {
		os.Remove(f)
	}
	_ = count
}

func TestWriteOutput(t *testing.T) {
	outputContent := "Test output content"
	outputFile := "output_test.txt"

	opts := Options{OutputFilename: outputFile}
	err := writeOutput(outputContent, opts)
	if err != nil {
		t.Errorf("writeOutput returned error: %v", err)
	}

	data, err := os.ReadFile(outputFile)
	if err != nil {
		t.Errorf("Failed to read written file: %v", err)
	}
	if string(data) != outputContent {
		t.Errorf("File content mismatch, expected %q, got %q", outputContent, string(data))
	}

	os.Remove(outputFile)
}
func TestEstimateTokens(t *testing.T) {
	tests := []struct {
		filename string
		content  []byte
		expected int64
	}{
		{"empty.txt", []byte(""), 0},
		{"small.txt", []byte("This is a small file."), 6},
		{"medium.txt", []byte(strings.Repeat("a", 350)), 100},
		{"large.txt", []byte(strings.Repeat("a", 700)), 200},
	}

	for _, tt := range tests {
		// Write the test file
		err := os.WriteFile(tt.filename, tt.content, 0644)
		if err != nil {
			t.Fatalf("Failed to write test file %s: %v", tt.filename, err)
		}

		// Ensure the file is removed after the test
		defer os.Remove(tt.filename)

		// Estimate tokens
		result := estimateTokens(tt.filename)
		if result != tt.expected {
			t.Errorf("estimateTokens(%q) = %d, want %d", tt.filename, result, tt.expected)
		}
	}
}
