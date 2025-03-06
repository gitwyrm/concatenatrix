# Concatenatrix

A simple command-line utility written in Go to concatenate all non-binary files in a local Git repository so that a project can be easily dropped as a single file into an AI service like ChatGPT or Grok to ask questions about it.

![Image](https://github.com/user-attachments/assets/6fd6eb5c-a328-4e9d-a795-db6226a6acdb)

Uses a simple heuristic to detect if a file is binary (checking the first 512 bytes for null bytes and UTF-8 validity).

Hidden files (starting with a dot) and those not tracked by Git are ignored.

Files are separated with a file header using the format:

```
{{File: filename.txt}}
```

This allows an AI service to still be able to refer to specific files in your code.

Can be used using simple command line flags or interactively using huh? forms from charm_, which lets you control the app using the cursor keys and enter/return in a nice terminal menu.

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

## Command-Line Flags
`-h`: Print help

```bash
concatenatrix -h
```

`-i`: Run interactive mode to select file extensions and other command line flags in a huh? form

```bash
concatenatrix -i
```

`-c`: Copy the output directly to the clipboard.

```bash
concatenatrix -c
```

`-ext`: Specify file extensions to include in the concatenation, by default all non-binary files tracked by Git are included. For example, to only include `.go` and `.txt` files:

```bash
concatenatrix -ext go,txt
```

`-n`: Include line numbers in the output

```bash
concatenatrix -n
```
