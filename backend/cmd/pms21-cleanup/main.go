// pms21-cleanup audits or explicitly applies the PMS 21 destructive cleanup.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	"pms/backend/internal/pms21cleanup"
)

func main() {
	report, ok := run(context.Background(), os.Args[1:], os.Stderr)
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(report); err != nil {
		fmt.Fprintln(os.Stderr, "encode report:", err)
		os.Exit(1)
	}
	if !ok {
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, stderr io.Writer) (pms21cleanup.Report, bool) {
	fs := flag.NewFlagSet("pms21-cleanup", flag.ContinueOnError)
	fs.SetOutput(stderr)
	audit := fs.Bool("audit", false, "audit cleanup readiness using an immutable database and migrated copy")
	apply := fs.Bool("apply", false, "apply all pending migrations, including destructive cleanup")
	db := fs.String("db", "", "absolute path to the quiesced SQLite database")
	dataRoot := fs.String("data-root", "", "absolute application data root used to verify invoice file contents")
	imageDigest := fs.String("image-digest", "", "immutable backend image digest")
	commit := fs.String("commit", "", "backend source commit")
	frontendBuild := fs.String("frontend-build", "", "frontend build identity")
	operator := fs.String("operator", "", "operator identity")
	exceptionRegister := fs.String("exception-register-reference", "", "approved row-level exception register reference")
	approvedExceptions := fs.String("approved-exceptions-reference", "", "approved exceptions decision reference")
	analyticsParity := fs.String("analytics-parity-reference", "", "approved external analytics and availability parity artifact reference")
	remoteVerification := fs.String("remote-verification-reference", "", "approved Google and Nuki remote verification artifact reference")
	callerInventory := fs.String("caller-inventory-reference", "", "approved caller and runtime-independence artifact reference")
	confirm := fs.Bool("confirm-destructive-cleanup", false, "required explicit confirmation for apply")
	preReport := fs.String("pre-report", "", "absolute path to the passing audit report")
	if err := fs.Parse(args); err != nil {
		r := pms21cleanup.Report{Mode: "invalid", Errors: []string{err.Error()}}
		_ = pms21cleanup.SetChecksum(&r)
		return r, false
	}
	opts := pms21cleanup.Options{
		DBPath: *db, DataRoot: *dataRoot, ImageDigest: *imageDigest, Commit: *commit,
		FrontendBuild: *frontendBuild, Operator: *operator,
		ExceptionRegisterReference: *exceptionRegister, ApprovedExceptionsReference: *approvedExceptions,
		AnalyticsParityReference: *analyticsParity, RemoteVerificationReference: *remoteVerification,
		CallerInventoryReference: *callerInventory, Confirm: *confirm, PreReport: *preReport,
	}
	if *audit == *apply {
		r := pms21cleanup.Report{Mode: "invalid", Errors: []string{"select exactly one mode: --audit or --apply"}}
		_ = pms21cleanup.SetChecksum(&r)
		return r, false
	}
	var report pms21cleanup.Report
	if *audit {
		report = pms21cleanup.Audit(ctx, opts)
	} else {
		report = pms21cleanup.Apply(ctx, opts)
	}
	return report, report.Passed
}
