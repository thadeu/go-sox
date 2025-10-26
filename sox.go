package sox

import (
	"fmt"
	"io"
	"os/exec"
)

// Converter handles one-shot audio format conversion using SoX
type Converter struct {
	Input   AudioFormat
	Output  AudioFormat
	Options ConversionOptions
}

// NewConverter creates a new Converter with the specified input and output formats
func NewConverter(input, output AudioFormat) *Converter {
	return &Converter{
		Input:   input,
		Output:  output,
		Options: DefaultOptions(),
	}
}

// WithOptions sets custom conversion options
func (c *Converter) WithOptions(opts ConversionOptions) *Converter {
	c.Options = opts
	return c
}

// Convert performs a one-shot conversion from input reader to output writer
// This uses pipes to stream data through SoX without temporary files
func (c *Converter) Convert(input io.Reader, output io.Writer) error {
	// Validate formats
	if err := c.Input.Validate(); err != nil {
		return fmt.Errorf("invalid input format: %w", err)
	}
	if err := c.Output.Validate(); err != nil {
		return fmt.Errorf("invalid output format: %w", err)
	}

	// Build SoX command arguments
	args := c.buildCommandArgs()

	// Create command
	cmd := exec.Command(c.Options.SoxPath, args...)

	// Set up pipes
	cmd.Stdin = input
	cmd.Stdout = output

	// Capture stderr for debugging
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	// Start the command
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start sox: %w", err)
	}

	// Read stderr in background (to prevent blocking)
	stderrData := make(chan []byte, 1)
	go func() {
		data, _ := io.ReadAll(stderr)
		stderrData <- data
	}()

	// Wait for command to complete
	if err := cmd.Wait(); err != nil {
		errMsg := <-stderrData
		return fmt.Errorf("sox conversion failed: %w\nstderr: %s", err, string(errMsg))
	}

	return nil
}

// ConvertFile converts audio from an input file to an output file
// This is a convenience method for file-based conversions
func (c *Converter) ConvertFile(inputPath, outputPath string) error {
	// Validate formats
	if err := c.Input.Validate(); err != nil {
		return fmt.Errorf("invalid input format: %w", err)
	}
	if err := c.Output.Validate(); err != nil {
		return fmt.Errorf("invalid output format: %w", err)
	}

	// Build SoX command arguments with file paths
	args := c.buildCommandArgs()

	// Replace stdin/stdout placeholders with actual file paths
	for i, arg := range args {
		if arg == "-" {
			if i < len(args)-1 {
				// First "-" is input
				args[i] = inputPath
			} else {
				// Last "-" is output
				args[i] = outputPath
			}
		}
	}

	// Create and run command
	cmd := exec.Command(c.Options.SoxPath, args...)

	// Capture stderr
	stderr, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("sox conversion failed: %w\nstderr: %s", err, string(stderr))
	}

	return nil
}

// buildCommandArgs constructs the complete SoX command arguments
func (c *Converter) buildCommandArgs() []string {
	args := []string{}

	// Global options
	args = append(args, c.Options.buildGlobalArgs()...)

	// Input format arguments
	args = append(args, c.Input.buildArgs(true)...)

	// Input file (stdin)
	args = append(args, "-")

	// Output format arguments
	args = append(args, c.Output.buildArgs(false)...)

	// Format-specific arguments for output
	args = append(args, c.Options.buildFormatArgs(&c.Output)...)

	// Output file (stdout)
	args = append(args, "-")

	// Effects
	if effects := c.Options.buildEffectArgs(); len(effects) > 0 {
		args = append(args, effects...)
	}

	return args
}

// CheckSoxInstalled verifies that SoX is installed and accessible
func CheckSoxInstalled(soxPath string) error {
	if soxPath == "" {
		soxPath = "sox"
	}

	cmd := exec.Command(soxPath, "--version")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("sox not found or not executable: %w", err)
	}

	return nil
}
