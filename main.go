package main

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"golang.design/x/clipboard"
)

// isTextFile checks if a file is likely a text file by sampling its initial bytes.
func isTextFile(filename string) bool {
	// Open the file
	file, err := os.Open(filename)
	if err != nil {
		return false // If we can't open it, assume it's not text to skip it
	}
	defer file.Close()

	// Read up to 512 bytes (a reasonable sample size)
	buf := make([]byte, 512)
	n, err := file.Read(buf)
	if err != nil && err != io.EOF {
		return false
	}
	if n == 0 {
		return true // Empty files can be considered text
	}

	// Trim the buffer to the actual bytes read
	buf = buf[:n]

	// Check if the content is valid UTF-8 and mostly printable
	if !utf8.Valid(buf) {
		return false // Invalid UTF-8 suggests binary data
	}

	// Count non-printable characters (ASCII control codes < 32, except \n, \r, \t)
	nonPrintable := 0
	for _, b := range buf {
		if b < 32 && b != '\n' && b != '\r' && b != '\t' {
			nonPrintable++
		}
	}

	// If more than 10% of the sample is non-printable, assume it's binary
	return float64(nonPrintable)/float64(n) < 0.1
}

// checks if any component of the path starts with a dot, indicating a hidden file or directory.
func isHiddenFile(filePath string) bool {
	parts := strings.Split(filepath.ToSlash(filePath), "/") // Normalize path and split by /
	for _, part := range parts {
		if strings.HasPrefix(part, ".") {
			return true
		}
	}
	return false
}

// toClipboard copies the given string to the clipboard.
func toClipboard(s string) {
	// Initialize clipboard
	err := clipboard.Init()
	if err != nil {
		log.Fatal(err)
	}

	// Copy text to clipboard
	text := []byte(s)
	clipboard.Write(clipboard.FmtText, text)
}

func main() {
	// Define the -c flag
	copyToClipboard := flag.Bool("c", false, "Copy the concatenated output to the clipboard")
	flag.Parse()

	// Execute 'git ls-files --cached' to get the list of tracked files
	cmd := exec.Command("git", "ls-files", "--cached")
	output, err := cmd.Output()
	if err != nil {
		log.Fatal("Failed to list Git files; possibly not a Git repository or Git is not installed")
	}

	var buffer bytes.Buffer

	// Write the description to the buffer
	buffer.WriteString(
		"Format description: The following are files in the Git repository" +
			" of the project. The files are separated using {{File: filename.txt}}.\n\n",
	)

	// Use a scanner to read the output line by line
	scanner := bufio.NewScanner(bytes.NewReader(output))
	for scanner.Scan() {
		file := scanner.Text()
		// Skip empty lines
		if file == "" {
			continue
		}

		// Skip files starting with a dot (hidden files)
		if isHiddenFile(file) {
			log.Printf("Skipping hidden file: %s", file)
			continue
		}

		// Check if the file is a text file
		if !isTextFile(file) {
			log.Printf("Skipping non-text file: %s", file)
			continue
		}

		// Read the file content
		content, err := os.ReadFile(file)
		if err != nil {
			log.Printf("Failed to read file %s: %v", file, err)
			continue
		}

		// Write a file header
		buffer.WriteString(fmt.Sprintf("{{File: %s}}\n", file))

		// Write the content to the buffer
		buffer.Write(content)

		// Add a newline after each file
		buffer.WriteString("\n")
	}

	// Check for scanning errors
	if err := scanner.Err(); err != nil {
		log.Printf("Error reading git ls-files output: %v", err)
	}

	// Output to clipboard or stdout based on the flag
	if *copyToClipboard {
		toClipboard(buffer.String())
		log.Println("Output copied to clipboard")
	} else {
		os.Stdout.Write(buffer.Bytes())
	}
}
