// Command seed creates a demo household with several months of realistic,
// backdated transaction history — earn/redeem activity spread across weeks,
// varied hours and weekdays, and a claimed bounty — so every chart (balance
// trend, earn-vs-redeem, chore leaderboard, redemption frequency, household
// overview) has enough data to actually look like something. Run it with
// `make seed`; each run creates a brand-new demo household (use `make reset`
// to start clean).
package main

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"sort"
	"strings"
	"time"

	"cpal/internal/auth"
	"cpal/internal/config"
	"cpal/internal/domain"
	"cpal/internal/ledger"
	"cpal/internal/money"
	"cpal/internal/store"

	"github.com/google/uuid"
)

const seedPassword = "demopass123"
const months = 6

type taskDef struct {
	name   string
	value  string
	bounty bool
}

var catalog = []taskDef{
	{name: "Take out trash", value: "0.15"},
	{name: "Wash dishes", value: "0.20"},
	{name: "Mow the lawn", value: "1.00"},
	{name: "Vacuum living room", value: "0.30"},
	{name: "Walk the dog", value: "0.10"},
	{name: "Fold laundry", value: "0.25"},
}

var bountyDef = taskDef{name: "Organize the garage", value: "2.00", bounty: true}

var holderNames = []string{"Sam", "Riley", "Jordan"}

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

	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	suffix := fmt.Sprintf("%d", time.Now().Unix()%100000)

	tenant, opUsername, err := seedHousehold(ctx, st, lg, suffix)
	if err != nil {
		return fmt.Errorf("seed household: %w", err)
	}
	opIdentity, err := st.GetIdentityByUsername(ctx, opUsername)
	if err != nil {
		return fmt.Errorf("load operator identity: %w", err)
	}

	tasks, bounty, err := seedCatalog(ctx, st, tenant.ID)
	if err != nil {
		return fmt.Errorf("seed catalog: %w", err)
	}

	to := time.Now().UTC()
	from := to.AddDate(0, -months, 0)

	fmt.Printf("\nDemo household: %s\n", tenant.Name)
	fmt.Printf("  operator  username=%-24s password=%s\n", opUsername, seedPassword)

	for i, name := range holderNames {
		acct, holderIdentity, err := seedHolder(ctx, st, lg, tenant, name, suffix)
		if err != nil {
			return fmt.Errorf("seed holder %s: %w", name, err)
		}
		includeBounty := i == 0 // first holder claims the one-time bounty
		events := genEvents(rng, tasks, bounty, includeBounty, from, to)
		for _, ev := range events {
			if err := applyEvent(ctx, st, lg, tenant, acct, holderIdentity.ID, opIdentity.ID, ev); err != nil {
				return fmt.Errorf("apply event for %s: %w", name, err)
			}
		}
		// Leave one fresh, un-backdated request pending so Approvals isn't empty.
		if err := seedPendingRequest(ctx, st, lg, tenant, acct, tasks[rng.Intn(len(tasks))], holderIdentity.ID); err != nil {
			return fmt.Errorf("seed pending request for %s: %w", name, err)
		}
		fmt.Printf("  holder    username=%-24s password=%s  (%d settled transactions)\n",
			holderIdentity.Username, seedPassword, len(events))
	}
	fmt.Println("\nLog in at the web app with any of the above.")
	return nil
}

// seedHousehold provisions a tenant plus its GL accounts and first operator,
// mirroring handleSignup.
func seedHousehold(ctx context.Context, st *store.Store, lg *ledger.Ledger, suffix string) (domain.Tenant, string, error) {
	issuanceTB, err := lg.OpenGL()
	if err != nil {
		return domain.Tenant{}, "", err
	}
	redemptionTB, err := lg.OpenGL()
	if err != nil {
		return domain.Tenant{}, "", err
	}
	tenant := &domain.Tenant{
		ID: uuid.NewString(), Name: "Demo Household " + suffix,
		IssuanceAccountID: uuid.NewString(), RedemptionAccountID: uuid.NewString(),
		IssuanceTBID: issuanceTB, RedemptionTBID: redemptionTB,
	}
	if err := st.CreateTenant(ctx, tenant); err != nil {
		return domain.Tenant{}, "", err
	}
	glAccounts := []domain.Account{
		{ID: tenant.IssuanceAccountID, TenantID: tenant.ID, TBAccountID: issuanceTB, Currency: money.Currency, Name: "Issuance GL"},
		{ID: tenant.RedemptionAccountID, TenantID: tenant.ID, TBAccountID: redemptionTB, Currency: money.Currency, Name: "Redemption GL"},
	}
	for i := range glAccounts {
		if err := st.CreateInternalAccount(ctx, &glAccounts[i]); err != nil {
			return domain.Tenant{}, "", err
		}
	}

	username := fmt.Sprintf("demo-parent-%s@benefitcoins.app", suffix)
	hash, err := auth.HashPassword(seedPassword)
	if err != nil {
		return domain.Tenant{}, "", err
	}
	cust := &domain.Customer{ID: uuid.NewString(), TenantID: tenant.ID, Type: domain.CustomerOperator, DisplayName: "Demo Parent"}
	if err := st.CreateCustomer(ctx, cust); err != nil {
		return domain.Tenant{}, "", err
	}
	identity := &domain.Identity{ID: uuid.NewString(), TenantID: tenant.ID, CustomerID: cust.ID, Username: username, PasswordHash: hash, Role: domain.RoleOperator}
	if err := st.CreateIdentity(ctx, identity); err != nil {
		return domain.Tenant{}, "", err
	}
	return *tenant, username, nil
}

func seedCatalog(ctx context.Context, st *store.Store, tenantID string) ([]domain.Task, domain.Task, error) {
	var tasks []domain.Task
	for _, t := range catalog {
		valueMinor, err := money.ParseCoins(t.value)
		if err != nil {
			return nil, domain.Task{}, err
		}
		task := domain.Task{ID: uuid.NewString(), TenantID: tenantID, Name: t.name, ValueMinor: valueMinor, Active: true}
		if err := st.CreateTask(ctx, &task); err != nil {
			return nil, domain.Task{}, err
		}
		tasks = append(tasks, task)
	}
	bValueMinor, err := money.ParseCoins(bountyDef.value)
	if err != nil {
		return nil, domain.Task{}, err
	}
	bounty := domain.Task{ID: uuid.NewString(), TenantID: tenantID, Name: bountyDef.name, ValueMinor: bValueMinor, Active: true, IsBounty: true}
	if err := st.CreateTask(ctx, &bounty); err != nil {
		return nil, domain.Task{}, err
	}
	return tasks, bounty, nil
}

func seedHolder(ctx context.Context, st *store.Store, lg *ledger.Ledger, tenant domain.Tenant, name, suffix string) (domain.Account, domain.Identity, error) {
	username := fmt.Sprintf("%s-%s", strings.ToLower(name), suffix)
	hash, err := auth.HashPassword(seedPassword)
	if err != nil {
		return domain.Account{}, domain.Identity{}, err
	}
	cust := &domain.Customer{ID: uuid.NewString(), TenantID: tenant.ID, Type: domain.CustomerHolder, DisplayName: name}
	if err := st.CreateCustomer(ctx, cust); err != nil {
		return domain.Account{}, domain.Identity{}, err
	}
	identity := domain.Identity{ID: uuid.NewString(), TenantID: tenant.ID, CustomerID: cust.ID, Username: username, PasswordHash: hash, Role: domain.RoleHolder}
	if err := st.CreateIdentity(ctx, &identity); err != nil {
		return domain.Account{}, domain.Identity{}, err
	}
	tbID, err := lg.OpenAccount()
	if err != nil {
		return domain.Account{}, domain.Identity{}, err
	}
	custID := cust.ID
	acct := domain.Account{ID: uuid.NewString(), TenantID: tenant.ID, CustomerID: &custID, Kind: domain.AccountCustomer, TBAccountID: tbID, Name: name + " Wallet"}
	if err := st.CreateAccount(ctx, &acct); err != nil {
		return domain.Account{}, domain.Identity{}, err
	}
	return acct, identity, nil
}

type event struct {
	kind string // "earn" or "redeem"
	when time.Time
	task *domain.Task // nil for redeem
}

// genEvents builds a chronological, balance-feasible history: a few chores a
// week, an occasional redemption once a coin is available, spread across
// varied hours/weekdays so the frequency charts have real shape.
func genEvents(rng *rand.Rand, tasks []domain.Task, bounty domain.Task, includeBounty bool, from, to time.Time) []event {
	var events []event
	balance := int64(0)
	cursor := from
	first := true
	for cursor.Before(to) {
		weekEnd := cursor.AddDate(0, 0, 7)
		if weekEnd.After(to) {
			weekEnd = to
		}
		if includeBounty && first {
			events = append(events, event{"earn", randomTimeIn(rng, cursor, weekEnd), &bounty})
			balance += bounty.ValueMinor
			first = false
		}
		for i, n := 0, 1+rng.Intn(3); i < n; i++ {
			t := &tasks[rng.Intn(len(tasks))]
			events = append(events, event{"earn", randomTimeIn(rng, cursor, weekEnd), t})
			balance += t.ValueMinor
		}
		if balance >= money.Coin(1) && rng.Float64() < 0.5 {
			events = append(events, event{"redeem", randomTimeIn(rng, cursor, weekEnd), nil})
			balance -= money.Coin(1)
		}
		cursor = weekEnd
	}
	sort.Slice(events, func(i, j int) bool { return events[i].when.Before(events[j].when) })
	return events
}

// randomTimeIn picks a random day in [start, end) and biases the hour toward
// after-school/evening (chores get logged, rewards get redeemed) so the
// hour-of-day chart isn't flat.
func randomTimeIn(rng *rand.Rand, start, end time.Time) time.Time {
	span := end.Sub(start)
	if span <= 0 {
		return start
	}
	day := start.Add(time.Duration(rng.Int63n(int64(span))))
	hour := 14 + rng.Intn(8) // 2pm-9pm
	return time.Date(day.Year(), day.Month(), day.Day(), hour, rng.Intn(60), 0, 0, time.UTC)
}

// applyEvent posts a real (settled) two-phase ledger transfer, then backdates
// the Postgres row's timestamps to `when` — TigerBeetle only knows "now," so
// the historical value date lives in Postgres, same as any other value-dated
// posting in this system (see Transaction.EffectiveAt).
func applyEvent(ctx context.Context, st *store.Store, lg *ledger.Ledger, tenant domain.Tenant, acct domain.Account, holderID, operatorID string, ev event) error {
	var tx *domain.Transaction
	switch ev.kind {
	case "earn":
		pendingID, err := lg.EarnHold(tenant.IssuanceTBID, acct.TBAccountID, ev.task.ValueMinor)
		if err != nil {
			return err
		}
		postID, err := lg.Settle(pendingID, domain.TxEarn)
		if err != nil {
			return err
		}
		taskID := ev.task.ID
		tx = &domain.Transaction{
			ID: uuid.NewString(), TenantID: tenant.ID, Type: domain.TxEarn, Status: domain.TxSettled,
			AccountID: acct.ID, GLAccountID: tenant.IssuanceAccountID, AmountMinor: ev.task.ValueMinor,
			TaskID: &taskID, Memo: ev.task.Name, TBPendingTransferID: pendingID, TBPostTransferID: &postID,
			CreatedBy: holderID, DecidedBy: &operatorID,
		}
	case "redeem":
		amount := money.Coin(1)
		pendingID, err := lg.RedeemHold(acct.TBAccountID, tenant.RedemptionTBID, amount)
		if err != nil {
			return err
		}
		postID, err := lg.Settle(pendingID, domain.TxRedeem)
		if err != nil {
			return err
		}
		tx = &domain.Transaction{
			ID: uuid.NewString(), TenantID: tenant.ID, Type: domain.TxRedeem, Status: domain.TxSettled,
			AccountID: acct.ID, GLAccountID: tenant.RedemptionAccountID, AmountMinor: amount,
			Memo: "Reward redemption", TBPendingTransferID: pendingID, TBPostTransferID: &postID,
			CreatedBy: holderID, DecidedBy: &operatorID,
		}
	}
	if err := st.CreateTransaction(ctx, tx); err != nil {
		return err
	}
	_, err := st.Pool().Exec(ctx,
		`UPDATE transactions SET created_at=$2, effective_at=$2, decided_at=$2 WHERE id=$1`,
		tx.ID, ev.when)
	return err
}

// seedPendingRequest leaves one un-backdated, undecided earn hold so the
// operator's Approvals queue has something in it too.
func seedPendingRequest(ctx context.Context, st *store.Store, lg *ledger.Ledger, tenant domain.Tenant, acct domain.Account, task domain.Task, holderID string) error {
	pendingID, err := lg.EarnHold(tenant.IssuanceTBID, acct.TBAccountID, task.ValueMinor)
	if err != nil {
		return err
	}
	taskID := task.ID
	tx := &domain.Transaction{
		ID: uuid.NewString(), TenantID: tenant.ID, Type: domain.TxEarn, Status: domain.TxPending,
		AccountID: acct.ID, GLAccountID: tenant.IssuanceAccountID, AmountMinor: task.ValueMinor,
		TaskID: &taskID, Memo: task.Name, TBPendingTransferID: pendingID, CreatedBy: holderID,
	}
	return st.CreateTransaction(ctx, tx)
}
