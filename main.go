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
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/log"
	"github.com/dustin/go-humanize"
	"golang.design/x/clipboard"
)

// Options holds the configuration settings used for file concatenation operations.
type Options struct {
	CopyToClipboard    bool
	Extensions         string
	IncludeLineNumbers bool
	OutputFilename     string
}

// ExtInfo holds the file count and total estimated tokens per file extension.
type ExtInfo struct {
	FileCount   int
	TotalTokens int64
}

func main() {
	opts := parseOptions()
	files, err := getTrackedFiles()
	if err != nil {
		log.Fatal("Failed to list Git files", "error", err)
	}
	output, fileCount, totalTokens := buildOutput(files, opts)
	if err := writeOutput(output, opts); err != nil {
		log.Error("Error writing output", "error", err)
	}
	log.Info("Processed files", "count", fileCount, "tokens", humanize.Comma(totalTokens))
}

// estimateTokens estimates the number of tokens in a file based on its size.
func estimateTokens(filename string) int64 {
	fileInfo, err := os.Stat(filename)
	if err != nil {
		return 0
	}
	byteSize := fileInfo.Size()
	return byteSize * 10 / 35 // divide by 3.5 to estimate tokens for code
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

// parseOptions parses command-line flags or runs an interactive prompt.
func parseOptions() Options {
	// Define command-line flags
	copyToClipboard := flag.Bool("c", false, "Copy the concatenated output to the clipboard")
	extensions := flag.String("ext", "", "Comma-separated list of file extensions to include (without leading dot)")
	includeLineNumbers := flag.Bool("n", false, "Include line numbers in the output")
	interactive := flag.Bool("i", false, "Run interactive mode to select file extensions and other flags using huh forms")
	flag.Parse()

	var outputFilename string

	// Handle interactive mode
	if *interactive {
		// Get list of tracked files (e.g., from Git)
		files, err := getTrackedFiles()
		if err != nil {
			log.Fatal("Failed to list Git files", "error", err)
		}

		// Build a map of extension info (file count and token estimate)
		extInfoMap := make(map[string]ExtInfo)
		for _, file := range files {
			if isHiddenFile(file) || !isTextFile(file) {
				continue
			}
			ext := filepath.Ext(file)
			tokens := estimateTokens(file)
			info, ok := extInfoMap[ext]
			if !ok {
				info = ExtInfo{FileCount: 0, TotalTokens: 0}
			}
			info.FileCount++
			info.TotalTokens += tokens
			extInfoMap[ext] = info
		}

		// Create a sorted list of extensions
		var exts []string
		for ext := range extInfoMap {
			exts = append(exts, ext)
		}
		sort.Strings(exts)

		// Build options for the interactive multi-select form
		var options []huh.Option[string]
		for _, ext := range exts {
			info := extInfoMap[ext]
			labelExt := ext
			if ext == "" {
				labelExt = "no extension"
			}
			label := fmt.Sprintf("%s (%d files, ~%s tokens)", labelExt, info.FileCount, humanize.Comma(info.TotalTokens))
			options = append(options, huh.NewOption(label, ext))
		}

		// Run interactive multi-select for extensions
		var selectedExts []string
		if err := huh.NewMultiSelect[string]().
			Title("Select file extensions to include (leave empty for all):").
			Options(options...).
			Value(&selectedExts).
			Run(); err != nil {
			log.Fatal("Interactive selection failed", "error", err)
		}

		// Process selected extensions
		var processedExts []string
		for _, ext := range selectedExts {
			if ext != "" {
				// Remove leading dot if present (e.g., ".md" -> "md")
				processedExts = append(processedExts, strings.TrimPrefix(ext, "."))
			} else {
				// Include an empty string for "no extension"
				processedExts = append(processedExts, "")
			}
		}
		// Join the extensions into a comma-separated string
		*extensions = strings.Join(processedExts, ",")
		// Special case: if only "no extension" was selected, set *extensions to ","
		if len(selectedExts) == 1 && selectedExts[0] == "" {
			*extensions = ","
		}

		// Run interactive confirm for line numbers
		var includeLn bool
		if err := huh.NewConfirm().
			Title("Include line numbers?").
			Value(&includeLn).
			Run(); err != nil {
			log.Fatal("Interactive confirm failed", "error", err)
		}
		*includeLineNumbers = includeLn

		// Run interactive confirm for clipboard option
		var copyClip bool
		if err := huh.NewConfirm().
			Title("Copy output to clipboard?").
			Value(&copyClip).
			Run(); err != nil {
			log.Fatal("Interactive confirm failed", "error", err)
		}
		*copyToClipboard = copyClip

		// If not copying to clipboard, prompt for output filename
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

	// Return the parsed options
	return Options{
		CopyToClipboard:    *copyToClipboard,
		Extensions:         *extensions,
		IncludeLineNumbers: *includeLineNumbers,
		OutputFilename:     outputFilename,
	}
}

// getTrackedFiles retrieves the list of files currently tracked by Git.
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
func buildOutput(files []string, opts Options) (output string, fileCount int, totalTokens int64) {
	var buffer bytes.Buffer
	buffer.WriteString("Format description: The following are files in the Git repository" +
		" of the project. The files are separated using {{File: filename.txt}}.\n\n")

	var extMap map[string]struct{}
	if opts.Extensions != "" {
		extMap = make(map[string]struct{})
		for _, ext := range strings.Split(opts.Extensions, ",") {
			trimmed := strings.TrimSpace(ext)
			if trimmed == "" {
				extMap[""] = struct{}{}
			} else {
				extMap["."+trimmed] = struct{}{}
			}
		}
	}

	for _, file := range files {
		if isHiddenFile(file) || !isTextFile(file) {
			log.Info("Skipping file", "file", file)
			continue
		}
		totalTokens += estimateTokens(file)
		fileExt := filepath.Ext(file)
		if extMap != nil {
			if _, ok := extMap[fileExt]; !ok {
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
	return buffer.String(), fileCount, totalTokens
}

// writeOutput handles the output based on options.
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
