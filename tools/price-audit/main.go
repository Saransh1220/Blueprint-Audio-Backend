package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"time"

	"github.com/google/uuid"
	"github.com/saransh1220/blueprint-audio/internal/modules/catalog/application/priceaudit"
	"github.com/saransh1220/blueprint-audio/internal/modules/catalog/infrastructure/persistence/postgres"
	"github.com/saransh1220/blueprint-audio/internal/shared/infrastructure/config"
	"github.com/saransh1220/blueprint-audio/internal/shared/infrastructure/database"
)

// main is only the composition root. SQL lives in the Postgres repository;
// review/apply policy lives in the priceaudit application package.
func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if err := run(ctx, os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(parent context.Context, args []string, out io.Writer) error {
	flags := flag.NewFlagSet("price-audit", flag.ContinueOnError)
	dry := flags.Bool("dry-run", true, "Read and report only")
	apply := flags.Bool("apply", false, "Apply an explicitly reviewed manifest")
	manifestPath := flags.String("manifest", "", "Reviewed manifest to apply")
	manifestOut := flags.String("manifest-out", "", "Write a proposal for later review; does not apply it")
	reportPath := flags.String("out", "", "Audit report output path")
	rollback := flags.String("rollback-batch", "", "Conditionally undo a previously applied batch")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *apply && (*dry || *manifestPath == "" || *rollback != "") {
		return fmt.Errorf("apply requires --dry-run=false --manifest=<reviewed-file> and no rollback")
	}
	if *rollback != "" && (*dry || *apply) {
		return fmt.Errorf("rollback requires --dry-run=false and no --apply")
	}
	if !*dry && !*apply && *rollback == "" {
		return fmt.Errorf("choose --apply or --rollback-batch, or use dry-run")
	}
	cfg := config.Load()
	db, err := database.NewPostgresDB(cfg.Database)
	if err != nil {
		return fmt.Errorf("connect to audit database: %w", err)
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(parent, 2*time.Minute)
	defer cancel()
	service := priceaudit.NewService(postgres.NewPriceAuditRepository(db))
	if *rollback != "" {
		batch, err := uuid.Parse(*rollback)
		if err != nil {
			return err
		}
		n, err := service.Rollback(ctx, batch)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(out, "Rolled back %d prices from batch %s\n", n, batch)
		return err
	}
	if *apply {
		file, err := os.Open(*manifestPath)
		if err != nil {
			return err
		}
		defer file.Close()
		var manifest priceaudit.Manifest
		decoder := json.NewDecoder(io.LimitReader(file, 8<<20))
		decoder.DisallowUnknownFields()
		if err = decoder.Decode(&manifest); err != nil {
			return err
		}
		var extra any
		if decoder.Decode(&extra) != io.EOF {
			return fmt.Errorf("manifest must contain one JSON document")
		}
		n, err := service.Apply(ctx, manifest)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(out, "Applied %d prices from reviewed batch %s\n", n, manifest.BatchID)
		return err
	}
	report, manifest, err := service.Audit(ctx)
	if err != nil {
		return err
	}
	body, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	if *reportPath != "" {
		if err = os.WriteFile(*reportPath, body, 0600); err != nil {
			return err
		}
	} else {
		if _, err = fmt.Fprintln(out, string(body)); err != nil {
			return err
		}
	}
	if *manifestOut != "" {
		body, err = json.MarshalIndent(manifest, "", "  ")
		if err != nil {
			return err
		}
		return os.WriteFile(*manifestOut, body, 0600)
	}
	return nil
}
