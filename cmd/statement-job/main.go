// Command statement-job is a one-shot batch job: it generates last month's
// PDF statement for every holder account in every active household, saves it
// to the in-app Inbox, and — only if SMTP is configured — emails it to the
// household's operator(s). It's meant to be invoked by an external scheduler
// (cron, Windows Task Scheduler, or a hosting provider's scheduled job); see
// `make statement-job`.
package main

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"cpal/internal/config"
	"cpal/internal/domain"
	"cpal/internal/ledger"
	"cpal/internal/mail"
	"cpal/internal/statement"
	"cpal/internal/store"
)

func main() {
	if err := run(); err != nil {
		log.Fatalf("fatal: %v", err)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	ctx := context.Background()
	st, err := store.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer st.Close()
	if err := st.Migrate(ctx); err != nil {
		return err
	}

	lg, err := ledger.Connect(cfg.TBClusterID, []string{cfg.TBAddress})
	if err != nil {
		return err
	}
	defer lg.Close()

	sender, emailEnabled := mail.New(cfg.SMTP)
	if !emailEnabled {
		log.Println("SMTP not configured — statements will be generated to the Inbox only")
	}

	period := time.Now().UTC().AddDate(0, -1, 0) // the just-completed calendar month
	deps := statement.Deps{Store: st, Cfg: cfg}

	tenants, err := st.ListActiveTenants(ctx)
	if err != nil {
		return err
	}

	var generated, emailed, failed int
	for _, tenant := range tenants {
		accts, err := st.ListCustomerAccounts(ctx, tenant.ID)
		if err != nil {
			log.Printf("tenant %s: list accounts: %v", tenant.ID, err)
			continue
		}
		recipient := ""
		if emailEnabled {
			recipient = operatorEmail(ctx, st, tenant.ID)
		}
		for _, acct := range accts {
			holder := holderName(ctx, st, tenant.ID, acct)
			pdf, err := statement.Generate(ctx, deps, tenant, acct, holder, period)
			if err != nil {
				log.Printf("tenant %s account %s: generate: %v", tenant.ID, acct.ID, err)
				failed++
				continue
			}
			from, _ := statement.PeriodBounds(period)
			id, err := st.SaveStatement(ctx, tenant.ID, acct.ID, from, pdf)
			if err != nil {
				log.Printf("tenant %s account %s: save: %v", tenant.ID, acct.ID, err)
				failed++
				continue
			}
			generated++

			if recipient == "" {
				continue
			}
			month := period.Format("January 2006")
			subject := fmt.Sprintf("%s's %s statement — %s", holder, tenant.Name, month)
			body := fmt.Sprintf("Attached is %s's statement for %s.\n\nIt's also available any time in the app under Inbox.", holder, month)
			filename := fmt.Sprintf("statement-%s-%s.pdf", slug(holder), period.Format("2006-01"))
			if err := sender.SendStatement(recipient, subject, body, filename, pdf); err != nil {
				// Inbox delivery already succeeded above — a failed send here is
				// logged, not fatal, and never blocks other holders' statements.
				log.Printf("tenant %s account %s: email: %v", tenant.ID, acct.ID, err)
				continue
			}
			_ = st.MarkStatementEmailed(ctx, tenant.ID, id)
			emailed++
		}
	}
	log.Printf("statement job done: %d generated, %d emailed, %d failed", generated, emailed, failed)
	return nil
}

// operatorEmail finds the household's first operator identity whose username
// is email-shaped — the only reliably-emailable address in this data model,
// since holders' usernames are plain, operator-chosen strings.
func operatorEmail(ctx context.Context, st *store.Store, tenantID string) string {
	ids, err := st.ListIdentitiesByRole(ctx, tenantID, domain.RoleOperator)
	if err != nil {
		return ""
	}
	for _, id := range ids {
		if mail.LooksLikeAddress(id.Username) {
			return id.Username
		}
	}
	return ""
}

func holderName(ctx context.Context, st *store.Store, tenantID string, acct domain.Account) string {
	if acct.CustomerID == nil {
		return acct.Name
	}
	cust, err := st.GetCustomer(ctx, tenantID, *acct.CustomerID)
	if err != nil {
		return acct.Name
	}
	return cust.DisplayName
}

func slug(s string) string {
	return strings.ToLower(strings.ReplaceAll(strings.TrimSpace(s), " ", "-"))
}
