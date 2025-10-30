// Package testutil provides testing utilities for the OCI bundle library.
// This file contains coverage reporting utilities.
package testutil

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// CoverageReporter provides utilities for generating and managing test coverage reports.
type CoverageReporter struct {
	outputDir string
	tempDir   string
}

// NewCoverageReporter creates a new coverage reporter with the specified output directory.
func NewCoverageReporter(outputDir string) (*CoverageReporter, error) {
	// Create output directory if it doesn't exist
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create output directory: %w", err)
	}

	// Create temp directory for coverage files
	tempDir, err := os.MkdirTemp("", "coverage-")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp directory: %w", err)
	}

	return &CoverageReporter{
		outputDir: outputDir,
		tempDir:   tempDir,
	}, nil
}

// Close cleans up temporary files.
func (r *CoverageReporter) Close() error {
	if r.tempDir != "" {
		if err := os.RemoveAll(r.tempDir); err != nil {
			return fmt.Errorf("failed to remove temp directory %s: %w", r.tempDir, err)
		}
	}
	return nil
}

// GenerateCoverage runs tests with coverage and generates reports.
// It returns the coverage percentage and any error.
func (r *CoverageReporter) GenerateCoverage(ctx context.Context, packagePath string) (float64, error) {
	coverageFile := filepath.Join(r.tempDir, "coverage.out")

	// Run tests with coverage
	cmd := exec.CommandContext(ctx, "go", "test", "-cover", "-coverprofile", coverageFile, packagePath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return 0, fmt.Errorf("failed to run tests with coverage: %w", err)
	}

	// Check if coverage file was created
	if _, err := os.Stat(coverageFile); os.IsNotExist(err) {
		return 0, fmt.Errorf("coverage file was not generated")
	}

	// Parse coverage percentage
	percentage, err := r.parseCoveragePercentage(coverageFile)
	if err != nil {
		return 0, fmt.Errorf("failed to parse coverage: %w", err)
	}

	// Generate HTML report
	htmlFile := filepath.Join(r.outputDir, "coverage.html")
	if err := r.generateHTMLReport(coverageFile, htmlFile); err != nil {
		return percentage, fmt.Errorf("failed to generate HTML report: %w", err)
	}

	// Generate function coverage report
	funcFile := filepath.Join(r.outputDir, "coverage-functions.txt")
	if err := r.generateFunctionReport(coverageFile, funcFile); err != nil {
		return percentage, fmt.Errorf("failed to generate function report: %w", err)
	}

	return percentage, nil
}

// parseCoveragePercentage extracts the overall coverage percentage from a coverage file.
func (r *CoverageReporter) parseCoveragePercentage(coverageFile string) (float64, error) {
	cmd := exec.Command("go", "tool", "cover", "-func", coverageFile)
	output, err := cmd.Output()
	if err != nil {
		return 0, fmt.Errorf("failed to run go tool cover: %w", err)
	}

	lines := strings.Split(string(output), "\n")
	if len(lines) == 0 {
		return 0, fmt.Errorf("no output from coverage tool")
	}

	// Find the total line (usually the last meaningful line)
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if strings.HasPrefix(line, "total:") {
			parts := strings.Fields(line)
			if len(parts) >= 3 {
				percentageStr := strings.TrimSuffix(parts[2], "%")
				var percentage float64
				if _, err := fmt.Sscanf(percentageStr, "%f", &percentage); err != nil {
					return 0, fmt.Errorf("failed to parse percentage: %w", err)
				}
				return percentage, nil
			}
		}
	}

	return 0, fmt.Errorf("could not find total coverage in output")
}

// generateHTMLReport generates an HTML coverage report.
func (r *CoverageReporter) generateHTMLReport(coverageFile, outputFile string) error {
	cmd := exec.Command("go", "tool", "cover", "-html", coverageFile, "-o", outputFile)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to generate HTML report: %w", err)
	}

	return nil
}

// generateFunctionReport generates a function-level coverage report.
func (r *CoverageReporter) generateFunctionReport(coverageFile, outputFile string) error {
	cmd := exec.Command("go", "tool", "cover", "-func", coverageFile)
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to generate function report: %w", err)
	}

	if err := os.WriteFile(outputFile, output, 0o644); err != nil {
		return fmt.Errorf("failed to write function report: %w", err)
	}

	return nil
}

// GenerateCombinedCoverage generates coverage reports for multiple packages.
func (r *CoverageReporter) GenerateCombinedCoverage(
	ctx context.Context,
	packagePaths []string,
) (map[string]float64, error) {
	results := make(map[string]float64)

	for _, pkgPath := range packagePaths {
		percentage, err := r.GenerateCoverage(ctx, pkgPath)
		if err != nil {
			return results, fmt.Errorf("failed to generate coverage for %s: %w", pkgPath, err)
		}
		results[pkgPath] = percentage
	}

	// Generate combined summary
	summaryFile := filepath.Join(r.outputDir, "coverage-summary.txt")
	if err := r.generateSummaryReport(results, summaryFile); err != nil {
		return results, fmt.Errorf("failed to generate summary: %w", err)
	}

	return results, nil
}

// generateSummaryReport creates a summary of coverage across multiple packages.
func (r *CoverageReporter) generateSummaryReport(results map[string]float64, outputFile string) error {
	var content strings.Builder
	content.WriteString("Coverage Summary Report\n")
	content.WriteString("=======================\n\n")

	total := 0.0
	count := 0

	for pkg, percentage := range results {
		content.WriteString(fmt.Sprintf("%-50s %.2f%%\n", pkg, percentage))
		total += percentage
		count++
	}

	if count > 0 {
		avg := total / float64(count)
		content.WriteString(fmt.Sprintf("\nAverage Coverage: %.2f%%\n", avg))
	}

	if err := os.WriteFile(outputFile, []byte(content.String()), 0o644); err != nil {
		return fmt.Errorf("failed to write summary: %w", err)
	}

	return nil
}

// GetCoverageFiles returns the paths to generated coverage files.
func (r *CoverageReporter) GetCoverageFiles() map[string]string {
	return map[string]string{
		"html":      filepath.Join(r.outputDir, "coverage.html"),
		"functions": filepath.Join(r.outputDir, "coverage-functions.txt"),
		"summary":   filepath.Join(r.outputDir, "coverage-summary.txt"),
	}
}

// PrintCoverageSummary prints a formatted coverage summary to stdout.
func (r *CoverageReporter) PrintCoverageSummary(results map[string]float64) {
	fmt.Println("Coverage Report")
	fmt.Println("===============")

	total := 0.0
	count := 0

	for pkg, percentage := range results {
		status := "❌"
		if percentage >= 80.0 {
			status = "✅"
		} else if percentage >= 60.0 {
			status = "⚠️"
		}

		fmt.Printf("%s %-50s %.2f%%\n", status, pkg, percentage)
		total += percentage
		count++
	}

	if count > 0 {
		avg := total / float64(count)
		fmt.Printf("\n📊 Average Coverage: %.2f%%\n", avg)

		switch {
		case avg >= 80.0:
			fmt.Println("🎉 Excellent coverage!")
		case avg >= 60.0:
			fmt.Println("👍 Good coverage, could be improved")
		default:
			fmt.Println("⚠️  Coverage needs improvement")
		}
	}
}

// CoverageThreshold represents coverage thresholds for different quality levels.
type CoverageThreshold struct {
	Excellent float64 // >= 80%
	Good      float64 // >= 60%
	Minimum   float64 // >= 40%
}

// DefaultCoverageThresholds returns the default coverage thresholds.
func DefaultCoverageThresholds() CoverageThreshold {
	return CoverageThreshold{
		Excellent: 80.0,
		Good:      60.0,
		Minimum:   40.0,
	}
}

// ValidateCoverage checks if coverage meets the specified thresholds.
func ValidateCoverage(results map[string]float64, thresholds CoverageThreshold) []CoverageIssue {
	var issues []CoverageIssue

	for pkg, percentage := range results {
		switch {
		case percentage < thresholds.Minimum:
			issues = append(issues, CoverageIssue{
				Package:  pkg,
				Coverage: percentage,
				Severity: "critical",
				Message:  fmt.Sprintf("Coverage %.2f%% below minimum threshold %.2f%%", percentage, thresholds.Minimum),
			})
		case percentage < thresholds.Good:
			issues = append(issues, CoverageIssue{
				Package:  pkg,
				Coverage: percentage,
				Severity: "warning",
				Message:  fmt.Sprintf("Coverage %.2f%% below good threshold %.2f%%", percentage, thresholds.Good),
			})
		case percentage < thresholds.Excellent:
			issues = append(issues, CoverageIssue{
				Package:  pkg,
				Coverage: percentage,
				Severity: "info",
				Message: fmt.Sprintf(
					"Coverage %.2f%% could be improved to reach excellent threshold %.2f%%",
					percentage,
					thresholds.Excellent,
				),
			})
		}
	}

	return issues
}

// CoverageIssue represents a coverage validation issue.
type CoverageIssue struct {
	Package  string
	Coverage float64
	Severity string
	Message  string
}

// PrintIssues prints coverage issues in a formatted way.
func PrintIssues(issues []CoverageIssue) {
	if len(issues) == 0 {
		fmt.Println("✅ No coverage issues found")
		return
	}

	fmt.Println("Coverage Issues")
	fmt.Println("===============")

	for _, issue := range issues {
		icon := "ℹ️"
		switch issue.Severity {
		case "critical":
			icon = "❌"
		case "warning":
			icon = "⚠️"
		case "info":
			icon = "ℹ️"
		}

		fmt.Printf("%s %s: %s\n", icon, issue.Package, issue.Message)
	}
}
