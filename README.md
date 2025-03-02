# Concatenatrix

A simple command-line utility written in Go to concatenate all non-binary files in a local Git repository so that a project can be easily dropped as a single file into an AI service like ChatGPT or Grok to ask questions about it.

Uses a simple heuristic to detect if a file is binary (checking the first 512 bytes for null bytes and UTF-8 validity).

Hidden files (starting with a dot) and those not tracked by Git are ignored.

Files are separated with a file header using the format:

```
{{File: filename.txt}}
```

This allows an AI service to still be able to refer to specific files in your code.

## Installation

You need to have Go and Git installed, then you can install the app directly from this Github repository:

```bash
go install github.com/gitwyrm/concatenatrix@latest
```

## Usage

The app outputs the concatenated files to stdout, so you can pipe the output to a file

```bash
cd path/to/your/git/repo
concatenatrix > output.txt
```

You can also specify the `-c` flag to copy the output directly to the clipboard.

```bash
cd path/to/your/git/repo
concatenatrix -c
```