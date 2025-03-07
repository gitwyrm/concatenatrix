package main

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/log"
	"golang.design/x/clipboard"
)

// Options holds the configuration settings used for file concatenation operations.
type Options struct {
	CopyToClipboard    bool
	Extensions         string
	IncludeLineNumbers bool
	OutputFilename     string
}

func main() {
	opts := parseOptions()
	files, err := getTrackedFiles()
	if err != nil {
		log.Fatal("Failed to list Git files", "error", err)
	}
	output, fileCount := buildOutput(files, opts)
	if err := writeOutput(output, opts); err != nil {
		log.Error("Error writing output", "error", err)
	}
	log.Info("Processed files", "count", fileCount)
}

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

// parseOptions parses command-line flags or runs an interactive prompt to configure concatenation options.
func parseOptions() Options {
	copyToClipboard := flag.Bool("c", false, "Copy the concatenated output to the clipboard")
	extensions := flag.String("ext", "", "Comma-separated list of file extensions to include (without leading dot)")
	includeLineNumbers := flag.Bool("n", false, "Include line numbers in the output")
	interactive := flag.Bool("i", false, "Run interactive mode to select file extensions and other flags using huh forms")
	flag.Parse()

	var outputFilename string

	if *interactive {
		files, err := getTrackedFiles()
		if err != nil {
			log.Fatal("Failed to list Git files", "error", err)
		}
		extSet := make(map[string]struct{})
		for _, file := range files {
			ext := filepath.Ext(file)
			if ext != "" && ext != ".gitignore" {
				extSet[ext] = struct{}{}
			}
		}

		var extOptions []string
		for ext := range extSet {
			extOptions = append(extOptions, ext)
		}

		var options []huh.Option[string]
		for _, ext := range extOptions {
			options = append(options, huh.NewOption(ext, ext))
		}

		var selectedExts []string
		if err := huh.NewMultiSelect[string]().
			Title("Select file extensions to include (leave empty for all):").
			Options(options...).
			Value(&selectedExts).
			Run(); err != nil {
			log.Fatal("Interactive selection failed", "error", err)
		}

		for i, ext := range selectedExts {
			selectedExts[i] = strings.TrimPrefix(ext, ".")
		}
		*extensions = strings.Join(selectedExts, ",")

		var includeLn bool
		if err := huh.NewConfirm().
			Title("Include line numbers?").
			Value(&includeLn).
			Run(); err != nil {
			log.Fatal("Interactive confirm failed", "error", err)
		}
		*includeLineNumbers = includeLn

		var copyClip bool
		if err := huh.NewConfirm().
			Title("Copy output to clipboard?").
			Value(&copyClip).
			Run(); err != nil {
			log.Fatal("Interactive confirm failed", "error", err)
		}
		*copyToClipboard = copyClip

		if !*copyToClipboard {
			if err := huh.NewInput().
				Title("Enter output filename (default: output.txt):").
				Value(&outputFilename).
				Run(); err != nil {
				log.Fatal("Interactive input for filename failed", "error", err)
			}
			if strings.TrimSpace(outputFilename) == "" {
				outputFilename = "output.txt"
			}
		}
	}

	return Options{
		CopyToClipboard:    *copyToClipboard,
		Extensions:         *extensions,
		IncludeLineNumbers: *includeLineNumbers,
		OutputFilename:     outputFilename,
	}
}

// getTrackedFiles retrieves the list of files currently tracked by Git in the repository.
func getTrackedFiles() ([]string, error) {
	cmd := exec.Command("git", "ls-files", "--cached")
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	var files []string
	scanner := bufio.NewScanner(bytes.NewReader(output))
	for scanner.Scan() {
		file := scanner.Text()
		if file != "" {
			files = append(files, file)
		}
	}
	return files, nil
}

// buildOutput generates a concatenated string of file contents based on the provided options.
func buildOutput(files []string, opts Options) (string, int) {
	var buffer bytes.Buffer
	fileCount := 0
	buffer.WriteString("Format description: The following are files in the Git repository" +
		" of the project. The files are separated using {{File: filename.txt}}.\n\n")

	var extMap map[string]struct{}
	if opts.Extensions != "" {
		extMap = make(map[string]struct{})
		for _, ext := range strings.Split(opts.Extensions, ",") {
			extMap["."+strings.TrimSpace(ext)] = struct{}{}
		}
	}

	for _, file := range files {
		if isHiddenFile(file) {
			log.Info("Skipping hidden file", "file", file)
			continue
		}
		if !isTextFile(file) {
			log.Info("Skipping non-text file", "file", file)
			continue
		}
		if extMap != nil {
			if _, ok := extMap[filepath.Ext(file)]; !ok {
				log.Info("Skipping file with excluded extension", "file", file)
				continue
			}
		}
		content, err := os.ReadFile(file)
		if err != nil {
			log.Error("Failed to read file", "file", file, "error", err)
			continue
		}
		buffer.WriteString(fmt.Sprintf("{{File: %s}}\n", file))
		if opts.IncludeLineNumbers {
			lines := strings.Split(string(content), "\n")
			for i, line := range lines {
				buffer.WriteString(fmt.Sprintf("%d: %s\n", i+1, line))
			}
		} else {
			buffer.Write(content)
		}
		buffer.WriteString("\n")
		fileCount++
	}
	return buffer.String(), fileCount
}

// writeOutput either writes the concatenated content to a file, copies it to the clipboard,
// or outputs to stdout based on the provided options.
func writeOutput(output string, opts Options) error {
	if opts.CopyToClipboard {
		toClipboard(output)
		log.Info("Output copied to clipboard")
	} else if opts.OutputFilename != "" {
		err := os.WriteFile(opts.OutputFilename, []byte(output), 0644)
		if err != nil {
			log.Error("Failed to write output file", "error", err)
		} else {
			log.Info("Output written to file", "file", opts.OutputFilename)
		}
	} else {
		os.Stdout.Write([]byte(output))
	}
	return nil
}
