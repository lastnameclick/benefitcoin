package store

import (
	"context"
	"time"

	"cpal/internal/domain"
)

// signedAmount is the SQL expression for a transaction's signed impact on an
// account's balance: positive for earn/adjust_credit, negative for
// redeem/adjust_debit.
const signedAmount = `CASE WHEN type IN ('earn','adjust_credit') THEN amount_minor ELSE -amount_minor END`

// bucketTrunc maps a client-facing bucket name to a safe date_trunc field,
// defaulting to "day" for anything unrecognized.
func bucketTrunc(bucket string) string {
	switch bucket {
	case "week", "month":
		return bucket
	default:
		return "day"
	}
}

// BalanceHistory returns a cumulative, opening-balance-seeded balance series
// for an account, bucketed by day/week/month, over settled transactions whose
// value date falls in [from, to). The live ledger only exposes the current
// balance, so history is derived from the Postgres transaction log.
func (s *Store) BalanceHistory(ctx context.Context, tenantID, accountID string, from, to time.Time, bucket string) ([]domain.BalancePoint, error) {
	var opening int64
	if err := s.pool.QueryRow(ctx, `
		SELECT COALESCE(SUM(`+signedAmount+`), 0)
		FROM transactions
		WHERE tenant_id=$1 AND account_id=$2 AND status='settled'
		  AND COALESCE(effective_at, created_at) < $3`,
		tenantID, accountID, from,
	).Scan(&opening); err != nil {
		return nil, err
	}

	rows, err := s.pool.Query(ctx, `
		SELECT date_trunc($5, COALESCE(effective_at, created_at)) AS bucket,
		       SUM(`+signedAmount+`)
		FROM transactions
		WHERE tenant_id=$1 AND account_id=$2 AND status='settled'
		  AND COALESCE(effective_at, created_at) >= $3 AND COALESCE(effective_at, created_at) < $4
		GROUP BY bucket ORDER BY bucket`,
		tenantID, accountID, from, to, bucketTrunc(bucket))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	running := opening
	out := []domain.BalancePoint{{Bucket: from, BalanceMinor: opening}}
	for rows.Next() {
		var b time.Time
		var delta int64
		if err := rows.Scan(&b, &delta); err != nil {
			return nil, err
		}
		running += delta
		out = append(out, domain.BalancePoint{Bucket: b, BalanceMinor: running})
	}
	return out, rows.Err()
}

// EarnRedeemSummary sums settled earn-side vs redeem-side amounts per bucket.
func (s *Store) EarnRedeemSummary(ctx context.Context, tenantID, accountID string, from, to time.Time, bucket string) ([]domain.EarnRedeemBucket, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT date_trunc($5, COALESCE(effective_at, created_at)) AS bucket,
		       COALESCE(SUM(amount_minor) FILTER (WHERE type IN ('earn','adjust_credit')), 0),
		       COALESCE(SUM(amount_minor) FILTER (WHERE type IN ('redeem','adjust_debit')), 0)
		FROM transactions
		WHERE tenant_id=$1 AND account_id=$2 AND status='settled'
		  AND COALESCE(effective_at, created_at) >= $3 AND COALESCE(effective_at, created_at) < $4
		GROUP BY bucket ORDER BY bucket`,
		tenantID, accountID, from, to, bucketTrunc(bucket))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.EarnRedeemBucket
	for rows.Next() {
		var eb domain.EarnRedeemBucket
		if err := rows.Scan(&eb.Bucket, &eb.EarnedMinor, &eb.RedeemedMinor); err != nil {
			return nil, err
		}
		out = append(out, eb)
	}
	return out, rows.Err()
}

// TaskLeaderboard ranks catalog tasks (including bounties) by settled earnings
// in [from, to). accountID nil scopes it tenant-wide (the operator view).
func (s *Store) TaskLeaderboard(ctx context.Context, tenantID string, accountID *string, from, to time.Time) ([]domain.TaskLeaderboardEntry, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT t.id, t.name, t.is_bounty, COUNT(*), SUM(tx.amount_minor)
		FROM transactions tx
		JOIN tasks t ON t.id = tx.task_id
		WHERE tx.tenant_id=$1 AND tx.status='settled' AND tx.type='earn'
		  AND COALESCE(tx.effective_at, tx.created_at) >= $2 AND COALESCE(tx.effective_at, tx.created_at) < $3
		  AND ($4::uuid IS NULL OR tx.account_id=$4)
		GROUP BY t.id, t.name, t.is_bounty
		ORDER BY SUM(tx.amount_minor) DESC`,
		tenantID, from, to, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.TaskLeaderboardEntry
	for rows.Next() {
		var e domain.TaskLeaderboardEntry
		if err := rows.Scan(&e.TaskID, &e.TaskName, &e.IsBounty, &e.Count, &e.TotalMinor); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// RedemptionFrequency buckets settled redemptions by hour-of-day, day-of-week,
// and month-of-year within [from, to).
func (s *Store) RedemptionFrequency(ctx context.Context, tenantID, accountID string, from, to time.Time) (domain.RedemptionFrequency, error) {
	var out domain.RedemptionFrequency
	for _, part := range []struct {
		field string
		dst   *[]domain.FrequencyBucket
	}{
		{"hour", &out.ByHour},
		{"dow", &out.ByWeekday},
		{"month", &out.ByMonth},
	} {
		rows, err := s.pool.Query(ctx, `
			SELECT EXTRACT(`+part.field+` FROM COALESCE(effective_at, created_at))::int, COUNT(*)
			FROM transactions
			WHERE tenant_id=$1 AND account_id=$2 AND status='settled' AND type='redeem'
			  AND COALESCE(effective_at, created_at) >= $3 AND COALESCE(effective_at, created_at) < $4
			GROUP BY 1 ORDER BY 1`,
			tenantID, accountID, from, to)
		if err != nil {
			return out, err
		}
		var buckets []domain.FrequencyBucket
		for rows.Next() {
			var b domain.FrequencyBucket
			if err := rows.Scan(&b.Bucket, &b.Count); err != nil {
				rows.Close()
				return out, err
			}
			buckets = append(buckets, b)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return out, err
		}
		rows.Close()
		*part.dst = buckets
	}
	return out, nil
}

// CountTransactionsSince returns a per-account count of transactions created
// on or after `since`, for the operator's household-overview "recent activity"
// column.
func (s *Store) CountTransactionsSince(ctx context.Context, tenantID string, since time.Time) (map[string]int64, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT account_id, COUNT(*) FROM transactions
		WHERE tenant_id=$1 AND created_at >= $2
		GROUP BY account_id`, tenantID, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int64{}
	for rows.Next() {
		var id string
		var n int64
		if err := rows.Scan(&id, &n); err != nil {
			return nil, err
		}
		out[id] = n
	}
	return out, rows.Err()
}
