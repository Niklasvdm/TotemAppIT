// Command totem-admin reviews community feedback (new-animal suggestions and
// reports on existing animals) from the encrypted database. Review-only tool for
// the SysAdmin over SSH; it adds no public/HTTP surface.
//
//	totem-admin [--db PATH] [--status pending|accepted|rejected|all] suggestions
//	totem-admin [--db PATH] [--status ...]                            reports
//	totem-admin accept-suggestion <id> | reject-suggestion <id>
//	totem-admin accept-report <id>     | reject-report <id>
//
// The key comes from TOTEM_DB_KEY / TOTEM_DB_KEY_FILE, as with totemd.
package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"log"
	"strconv"
	"text/tabwriter"

	"github.com/Niklasvdm/TotemAppIT/internal/config"
	"github.com/Niklasvdm/TotemAppIT/internal/store"
)

func main() {
	log.SetFlags(0)

	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}
	dbPath := flag.String("db", cfg.DBPath, "path to the encrypted database")
	status := flag.String("status", "pending", "status filter: pending|accepted|rejected|all")
	flag.Parse()

	cmd := flag.Arg(0)
	if cmd == "" {
		log.Fatal("usage: totem-admin [--db PATH] [--status ...] <suggestions|reports|accept-*|reject-*> [id]")
	}

	st, err := store.Open(*dbPath, cfg.DBKey)
	if err != nil {
		log.Fatalf("open store: %v", err)
	}
	defer st.Close()

	ctx := context.Background()
	if err := st.Migrate(ctx); err != nil {
		log.Fatalf("migrate: %v", err)
	}

	filter := *status
	if filter == "all" {
		filter = ""
	}

	switch cmd {
	case "suggestions":
		listSuggestions(ctx, st, filter)
	case "reports":
		listReports(ctx, st, filter)
	case "accept-suggestion", "reject-suggestion":
		setSuggestion(ctx, st, cmd, flag.Arg(1))
	case "accept-report", "reject-report":
		setReport(ctx, st, cmd, flag.Arg(1))
	default:
		log.Fatalf("unknown command %q", cmd)
	}
}

func listSuggestions(ctx context.Context, st *store.Store, status string) {
	items, err := st.ListSuggestions(ctx, status)
	if err != nil {
		log.Fatal(err)
	}
	if len(items) == 0 {
		fmt.Println("no suggestions")
		return
	}
	var buf bytes.Buffer
	tw := tabwriter.NewWriter(&buf, 0, 2, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tCOUNT\tSTATUS\tNAME\tNOTE")
	for _, s := range items {
		fmt.Fprintf(tw, "%d\t%d\t%s\t%s\t%s\n", s.ID, s.Count, s.Status, s.Name, s.Note)
	}
	tw.Flush()
	fmt.Print(buf.String())
}

func listReports(ctx context.Context, st *store.Store, status string) {
	items, err := st.ListReports(ctx, status)
	if err != nil {
		log.Fatal(err)
	}
	if len(items) == 0 {
		fmt.Println("no reports")
		return
	}
	var buf bytes.Buffer
	tw := tabwriter.NewWriter(&buf, 0, 2, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tCOUNT\tSTATUS\tSLUG\tREASON\tNOTE")
	for _, r := range items {
		fmt.Fprintf(tw, "%d\t%d\t%s\t%s\t%s\t%s\n", r.ID, r.Count, r.Status, r.AnimalSlug, r.Reason, r.Note)
	}
	tw.Flush()
	fmt.Print(buf.String())
}

// statusFor maps an accept-*/reject-* command to the status it sets.
func statusFor(cmd string) string {
	if cmd[:6] == "accept" {
		return "accepted"
	}
	return "rejected"
}

func setSuggestion(ctx context.Context, st *store.Store, cmd, idArg string) {
	id := parseID(idArg)
	ok, err := st.SetSuggestionStatus(ctx, id, statusFor(cmd))
	if err != nil {
		log.Fatal(err)
	}
	if !ok {
		log.Fatalf("no suggestion with id %d", id)
	}
	fmt.Printf("suggestion %d -> %s\n", id, statusFor(cmd))
}

func setReport(ctx context.Context, st *store.Store, cmd, idArg string) {
	id := parseID(idArg)
	ok, err := st.SetReportStatus(ctx, id, statusFor(cmd))
	if err != nil {
		log.Fatal(err)
	}
	if !ok {
		log.Fatalf("no report with id %d", id)
	}
	fmt.Printf("report %d -> %s\n", id, statusFor(cmd))
}

func parseID(arg string) int64 {
	id, err := strconv.ParseInt(arg, 10, 64)
	if err != nil || id <= 0 {
		log.Fatalf("expected a positive numeric id, got %q", arg)
	}
	return id
}
