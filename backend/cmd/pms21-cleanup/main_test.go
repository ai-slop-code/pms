package main

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"

	"pms/backend/internal/pms21cleanup"
)

func TestRunRequiresExactlyOneMode(t *testing.T) {
	for name, args := range map[string][]string{
		"neither": nil,
		"both":    {"--audit", "--apply"},
	} {
		t.Run(name, func(t *testing.T) {
			report, ok := run(context.Background(), args, &bytes.Buffer{})
			if ok || report.Mode != "invalid" {
				t.Fatalf("ok=%t mode=%q", ok, report.Mode)
			}
			if err := pms21cleanup.VerifyChecksum(report); err != nil {
				t.Fatalf("invalid-mode report checksum: %v", err)
			}
		})
	}
}

func TestRunRejectsNonAbsoluteDatabaseAsJSONReport(t *testing.T) {
	report, ok := run(context.Background(), []string{
		"--audit", "--db", "relative.db", "--image-digest", "sha256:test",
		"--commit", "test", "--frontend-build", "test",
	}, &bytes.Buffer{})
	if ok || report.Passed || len(report.Errors) == 0 {
		t.Fatalf("ok=%t report=%+v", ok, report)
	}
	if err := pms21cleanup.VerifyChecksum(report); err != nil {
		t.Fatalf("validation report checksum: %v", err)
	}
}

func TestRunRejectsMutableImageDigest(t *testing.T) {
	args := validAuditArgs(t)
	for i := range args {
		if args[i] == "--image-digest" {
			args[i+1] = "ghcr.io/example/pms:latest"
		}
	}
	report, ok := run(context.Background(), args, &bytes.Buffer{})
	if ok || report.Passed || !strings.Contains(strings.Join(report.Errors, " "), "tags are not accepted") {
		t.Fatalf("mutable digest report=%+v", report)
	}
}

func TestRunPassesRequiredOperationalEvidenceToReport(t *testing.T) {
	report, ok := run(context.Background(), validAuditArgs(t), &bytes.Buffer{})
	if ok || report.Passed {
		t.Fatalf("missing test database unexpectedly passed: %+v", report)
	}
	if report.Operator != "operator@example.invalid" || report.EffectiveFlags != "none" {
		t.Fatalf("operator/flags missing: %+v", report)
	}
	if report.ExceptionRegisterReference == "" || report.ApprovedExceptionsReference == "" || report.ExternalEvidence.AnalyticsParity == "" || report.ExternalEvidence.RemoteVerification == "" || report.ExternalEvidence.CallerInventory == "" {
		t.Fatalf("evidence references missing: %+v", report)
	}
	if err := pms21cleanup.VerifyChecksum(report); err != nil {
		t.Fatalf("failed report checksum: %v", err)
	}
}

func validAuditArgs(t *testing.T) []string {
	t.Helper()
	return []string{
		"--audit",
		"--db", filepath.Join(t.TempDir(), "missing.db"),
		"--image-digest", "ghcr.io/example/pms@sha256:" + strings.Repeat("a", 64),
		"--commit", "test-commit",
		"--frontend-build", "test-frontend",
		"--operator", "operator@example.invalid",
		"--exception-register-reference", "restricted://exceptions#sha256=" + strings.Repeat("b", 64),
		"--approved-exceptions-reference", "restricted://approval#sha256=" + strings.Repeat("c", 64),
		"--analytics-parity-reference", "restricted://analytics#sha256=" + strings.Repeat("d", 64),
		"--remote-verification-reference", "restricted://remote#sha256=" + strings.Repeat("e", 64),
		"--caller-inventory-reference", "restricted://callers#sha256=" + strings.Repeat("f", 64),
	}
}
