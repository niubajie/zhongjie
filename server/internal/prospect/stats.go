package prospect

import (
	"context"
	"fmt"
	"math"
)

// 批次10：获客效果统计。只读聚合，不产生副作用。

type statsRaw struct {
	Total         int64
	ReviewPending int64
	Contacted     int64
	Converted     int64
	Stale         int64
	OpenReminders int64
	ByStatus      []StatusCount
	ByIntent      []IntentCount
}

// LoadStats 聚合获客统计：总量、待复核、已联系、已转化、超期与提醒，
// 以及按状态、按意向的分布。
func (r *Repository) LoadStats(ctx context.Context) (statsRaw, error) {
	var s statsRaw
	err := r.pool.QueryRow(ctx, `
		SELECT
			(SELECT count(*) FROM prospect_leads WHERE deleted_at IS NULL),
			(SELECT count(*) FROM prospect_leads
			 WHERE deleted_at IS NULL AND status = 'new'
			   AND intent_status <> 'unknown'
			   AND COALESCE(intent_score, 0) >= $1),
			(SELECT count(DISTINCT f.lead_id) FROM follow_ups f
			 JOIN prospect_leads l ON l.id = f.lead_id
			 WHERE f.direction = 'outbound' AND l.deleted_at IS NULL),
			(SELECT count(*) FROM prospect_leads
			 WHERE deleted_at IS NULL AND status = 'converted'),
			(SELECT count(*) FROM prospect_leads
			 WHERE deleted_at IS NULL AND stale_at IS NOT NULL
			   AND status IN ('contacted', 'following_up')),
			(SELECT count(*) FROM prospect_reminders WHERE status = 'open')
	`, ReviewScoreThreshold).Scan(&s.Total, &s.ReviewPending, &s.Contacted, &s.Converted, &s.Stale, &s.OpenReminders)
	if err != nil {
		return statsRaw{}, fmt.Errorf("load prospect stats: %w", err)
	}

	statusRows, err := r.pool.Query(ctx, `
		SELECT status, count(*) FROM prospect_leads
		WHERE deleted_at IS NULL
		GROUP BY status
	`)
	if err != nil {
		return statsRaw{}, fmt.Errorf("load prospect status stats: %w", err)
	}
	defer statusRows.Close()
	s.ByStatus = []StatusCount{}
	for statusRows.Next() {
		var item StatusCount
		if err := statusRows.Scan(&item.Status, &item.Count); err != nil {
			return statsRaw{}, err
		}
		s.ByStatus = append(s.ByStatus, item)
	}
	if err := statusRows.Err(); err != nil {
		return statsRaw{}, err
	}

	intentRows, err := r.pool.Query(ctx, `
		SELECT intent_status, count(*) FROM prospect_leads
		WHERE deleted_at IS NULL
		GROUP BY intent_status
	`)
	if err != nil {
		return statsRaw{}, fmt.Errorf("load prospect intent stats: %w", err)
	}
	defer intentRows.Close()
	s.ByIntent = []IntentCount{}
	for intentRows.Next() {
		var item IntentCount
		if err := intentRows.Scan(&item.IntentStatus, &item.Count); err != nil {
			return statsRaw{}, err
		}
		s.ByIntent = append(s.ByIntent, item)
	}
	return s, intentRows.Err()
}

// GetStats 获客效果统计。
func (s *Service) GetStats(ctx context.Context) (ProspectStatsVO, error) {
	raw, err := s.repo.LoadStats(ctx)
	if err != nil {
		return ProspectStatsVO{}, err
	}
	return ProspectStatsVO{
		TotalLeads:     raw.Total,
		ReviewPending:  raw.ReviewPending,
		ContactedLeads: raw.Contacted,
		ConvertedLeads: raw.Converted,
		ConversionRate: conversionRate(raw.Converted, raw.Contacted),
		StaleLeads:     raw.Stale,
		OpenReminders:  raw.OpenReminders,
		ByStatus:       raw.ByStatus,
		ByIntent:       raw.ByIntent,
	}, nil
}

// conversionRate 计算转化率并保留四位小数；没有联系记录时返回 0。
func conversionRate(converted, contacted int64) float64 {
	if contacted <= 0 {
		return 0
	}
	return math.Round(float64(converted)/float64(contacted)*10000) / 10000
}
