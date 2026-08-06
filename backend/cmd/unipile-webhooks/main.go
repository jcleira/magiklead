// Command unipile-webhooks manages the app's webhook registrations at
// Unipile: the three sources the LinkedIn rail listens on, all pointing at
// this deployment's public /api/v1/webhooks/unipile endpoint.
//
// Registrations are pinned to an absolute public URL, so every time the
// tunnel hostname changes the old ones go dead silently — Unipile keeps
// POSTing into the void and the app simply stops seeing connects, accepts
// and replies. This command makes recovery one idempotent run instead of
// dashboard archaeology.
//
// Run it inside the devpod, where UNIPILE_API_KEY, UNIPILE_DSN and
// UNIPILE_WEBHOOK_SECRET are already in the environment:
//
//	devpods exec api go run ./cmd/unipile-webhooks list
//	devpods exec api go run ./cmd/unipile-webhooks register --base-url https://magiklead-smoke.magikshot.com
//	devpods exec api go run ./cmd/unipile-webhooks register --base-url https://… --dry-run
//	devpods exec api go run ./cmd/unipile-webhooks prune --base-url https://…
//
// --base-url is the PUBLIC origin Unipile must reach (the tunnel hostname,
// i.e. MAGIKLEAD_PUBLIC_API_URL), never the .localhost devpod URL.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/joho/godotenv"

	"github.com/jcleira/magiklead/backend/internal/linkedin/unipile"
)

// webhookPath is the app's inbound endpoint, appended to --base-url to form
// the request_url Unipile delivers to.
const webhookPath = "/api/v1/webhooks/unipile"

// defaultNamePrefix marks a registration as ours. Prune only ever deletes
// registrations whose name carries this prefix, so a webhook belonging to
// something else sharing the Unipile workspace is never touched.
const defaultNamePrefix = "magiklead-"

// authHeaderKey is the static header Unipile echoes on every delivery. It is
// the inbound endpoint's ONLY authentication — Unipile has no body signing —
// and its value must equal UNIPILE_WEBHOOK_SECRET, which
// unipile.Module.VerifyAuthToken constant-time compares.
const authHeaderKey = "Unipile-Auth"

// webhookSource is one registration the LinkedIn rail needs.
type webhookSource struct {
	source string
	events []string
}

// webhookSources are the three sources the rail listens on. The source names
// and event selectors are Unipile's own, confirmed against a live
// GET /api/v1/webhooks on the workspace:
//
//   - account_status — hosted-auth connect completion and later account
//     status changes. It takes no event selector.
//   - users/new_relation — a prospect accepted our connection request.
//   - messaging/message_received — a reply landed in a chat.
var webhookSources = []webhookSource{
	{source: "account_status"},
	{source: "users", events: []string{"new_relation"}},
	{source: "messaging", events: []string{"message_received"}},
}

// options is a parsed command line.
type options struct {
	// mode is list, register, or prune.
	mode string
	// baseURL is the public origin for register; on prune it is the origin
	// to KEEP (ours pointing anywhere else are stale and get deleted).
	baseURL string
	// dryRun prints the plan and issues no POST/DELETE.
	dryRun bool
	// namePrefix is the ownership marker (see defaultNamePrefix).
	namePrefix string
	// authToken is UNIPILE_WEBHOOK_SECRET — the static Unipile-Auth header
	// value stamped into each registration, which the inbound endpoint
	// constant-time compares.
	authToken string
}

func main() {
	_ = godotenv.Load()

	opts, err := parseArgs(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		printUsage()
		os.Exit(2)
	}
	opts.authToken = os.Getenv("UNIPILE_WEBHOOK_SECRET")

	apiKey := os.Getenv("UNIPILE_API_KEY")
	if apiKey == "" {
		log.Fatal("UNIPILE_API_KEY is not set")
	}
	dsn := os.Getenv("UNIPILE_DSN")
	if dsn == "" {
		log.Fatal("UNIPILE_DSN is not set")
	}

	m := unipile.New(apiKey, dsn, nil, nil)
	if err := run(context.Background(), os.Stdout, m, opts); err != nil {
		log.Fatalf("%s: %v", opts.mode, err)
	}
}

func parseArgs(args []string) (options, error) {
	if len(args) == 0 {
		return options{}, fmt.Errorf("missing mode")
	}
	opts := options{mode: args[0]}
	switch opts.mode {
	case "list", "register", "prune":
	default:
		return options{}, fmt.Errorf("unknown mode %q", opts.mode)
	}

	fs := flag.NewFlagSet("unipile-webhooks "+opts.mode, flag.ContinueOnError)
	fs.StringVar(&opts.baseURL, "base-url", "", "public origin Unipile delivers to (register: the URL to register; prune: the URL to keep)")
	fs.BoolVar(&opts.dryRun, "dry-run", false, "print the plan without creating or deleting anything")
	fs.StringVar(&opts.namePrefix, "name-prefix", defaultNamePrefix, "ownership marker written into each name; prune only deletes names carrying it")
	if err := fs.Parse(args[1:]); err != nil {
		return options{}, err
	}
	if opts.namePrefix == "" {
		return options{}, fmt.Errorf("--name-prefix must not be empty: prune would match every registration on the workspace")
	}
	return opts, nil
}

func run(ctx context.Context, out io.Writer, m *unipile.Module, opts options) error {
	switch opts.mode {
	case "list":
		return runList(ctx, out, m)
	case "register":
		return runRegister(ctx, out, m, opts)
	case "prune":
		return runPrune(ctx, out, m, opts)
	default:
		return fmt.Errorf("unknown mode %q", opts.mode)
	}
}

// runRegister brings the workspace up to the three sources the rail needs,
// pointed at --base-url. It is idempotent: it lists first and creates only
// what is missing, matching on source + request_url — so re-running after a
// partial failure finishes the job, and re-running after success is a no-op.
// A registration for the same source at a DIFFERENT url is left alone (it is
// stale, and removing it is prune's job, not register's).
func runRegister(ctx context.Context, out io.Writer, m *unipile.Module, opts options) error {
	if opts.baseURL == "" {
		return fmt.Errorf("--base-url is required (the public origin Unipile must reach)")
	}
	if opts.authToken == "" {
		return fmt.Errorf("UNIPILE_WEBHOOK_SECRET is not set — a registration without it would deliver payloads the endpoint rejects 401")
	}
	requestURL := strings.TrimRight(opts.baseURL, "/") + webhookPath

	existing, err := m.ListWebhooks(ctx)
	if err != nil {
		return fmt.Errorf("list webhooks: %w", err)
	}

	fmt.Fprintf(out, "register → %s%s\n", requestURL, dryRunBanner(opts.dryRun))
	var created, kept int
	for _, src := range webhookSources {
		if h, ok := findWebhook(existing, src.source, requestURL); ok {
			fmt.Fprintf(out, "  kept     %-14s %s (%s)\n", src.source, h.Name, h.ID)
			kept++
			continue
		}
		spec := unipile.WebhookSpec{
			Source:     src.source,
			Name:       opts.namePrefix + src.source,
			RequestURL: requestURL,
			Events:     src.events,
			Headers:    []unipile.WebhookHeader{{Key: authHeaderKey, Value: opts.authToken}},
		}
		created++
		if opts.dryRun {
			fmt.Fprintf(out, "  would create  %-14s %s  events=%s  → %s\n",
				spec.Source, spec.Name, eventsLabel(spec.Events), spec.RequestURL)
			continue
		}
		h, err := m.CreateWebhook(ctx, spec)
		if err != nil {
			return fmt.Errorf("create %s webhook: %w", src.source, err)
		}
		fmt.Fprintf(out, "  created  %-14s %s (%s)\n", src.source, spec.Name, h.ID)
	}
	fmt.Fprintf(out, "%s=%d kept=%d\n", countLabel("create", opts.dryRun), created, kept)
	return nil
}

// dryRunBanner marks a run that will not touch Unipile.
func dryRunBanner(dryRun bool) string {
	if dryRun {
		return "  [dry-run: no changes will be made]"
	}
	return ""
}

// countLabel names the tally so a dry run never reads like it did the work:
// "created=3" on a run that created nothing is the one misreading that could
// leave the smoke pod silently unregistered.
func countLabel(verb string, dryRun bool) string {
	if dryRun {
		return "would-" + verb
	}
	return verb + "d"
}

// eventsLabel renders a source's event selector for the plan.
func eventsLabel(events []string) string {
	if len(events) == 0 {
		return "-"
	}
	return strings.Join(events, ",")
}

// runPrune deletes our registrations — and only ours. Ownership is the name
// prefix: the Unipile workspace is shared, so anything without our marker
// belongs to someone else and is never touched, however dead it looks.
//
// With --base-url it deletes only the ones pointing somewhere ELSE, which is
// the tunnel-rotation shape: sweep what the old hostname left behind and keep
// what already points at the new one. Without it, every registration of ours
// goes (the teardown shape, e.g. the end-of-smoke disconnect).
func runPrune(ctx context.Context, out io.Writer, m *unipile.Module, opts options) error {
	keepURL := ""
	if opts.baseURL != "" {
		keepURL = strings.TrimRight(opts.baseURL, "/") + webhookPath
	}

	existing, err := m.ListWebhooks(ctx)
	if err != nil {
		return fmt.Errorf("list webhooks: %w", err)
	}

	banner := dryRunBanner(opts.dryRun)
	if keepURL == "" {
		fmt.Fprintf(out, "prune → every registration named %s*%s\n", opts.namePrefix, banner)
	} else {
		fmt.Fprintf(out, "prune → registrations named %s* not pointing at %s%s\n", opts.namePrefix, keepURL, banner)
	}

	var deleted, kept, foreign int
	for _, h := range existing {
		if !strings.HasPrefix(h.Name, opts.namePrefix) {
			foreign++
			continue
		}
		if keepURL != "" && h.RequestURL == keepURL {
			fmt.Fprintf(out, "  kept     %-14s %s (%s)\n", h.Source, h.Name, h.ID)
			kept++
			continue
		}
		deleted++
		if opts.dryRun {
			fmt.Fprintf(out, "  would delete  %-14s %s (%s)  → %s\n", h.Source, h.Name, h.ID, h.RequestURL)
			continue
		}
		if err := m.DeleteWebhook(ctx, h.ID); err != nil {
			return fmt.Errorf("delete webhook %s (%s): %w", h.ID, h.Name, err)
		}
		fmt.Fprintf(out, "  deleted  %-14s %s (%s) → %s\n", h.Source, h.Name, h.ID, h.RequestURL)
	}
	fmt.Fprintf(out, "%s=%d kept=%d foreign-untouched=%d\n",
		countLabel("delete", opts.dryRun), deleted, kept, foreign)
	return nil
}

// findWebhook looks for an existing registration of a source already pointing
// at requestURL — the idempotency key.
func findWebhook(hooks []unipile.Webhook, source, requestURL string) (unipile.Webhook, bool) {
	for _, h := range hooks {
		if h.Source == source && h.RequestURL == requestURL {
			return h, true
		}
	}
	return unipile.Webhook{}, false
}

// runList prints every registration on the workspace, ours and foreign.
func runList(ctx context.Context, out io.Writer, m *unipile.Module) error {
	hooks, err := m.ListWebhooks(ctx)
	if err != nil {
		return fmt.Errorf("list webhooks: %w", err)
	}
	fmt.Fprintf(out, "%d registration(s)\n", len(hooks))
	printTable(out, hooks)
	return nil
}

// printTable renders registrations one per line, aligned.
func printTable(out io.Writer, hooks []unipile.Webhook) {
	if len(hooks) == 0 {
		return
	}
	tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "  ID\tSOURCE\tNAME\tREQUEST_URL\tEVENTS\tENABLED")
	for _, h := range hooks {
		events := strings.Join(h.Events, ",")
		if events == "" {
			events = "-"
		}
		fmt.Fprintf(tw, "  %s\t%s\t%s\t%s\t%s\t%t\n",
			h.ID, h.Source, h.Name, h.RequestURL, events, h.Enabled)
	}
	_ = tw.Flush()
}

func printUsage() {
	fmt.Fprintln(os.Stderr, "usage: unipile-webhooks <list|register|prune> [--base-url URL] [--dry-run] [--name-prefix PREFIX]")
}
