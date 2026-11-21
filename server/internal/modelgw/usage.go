package modelgw

import (
	"context"
)

func (g *Gateway) record(ctx context.Context, kind, model string, inputUnits, outputUnits int64, failed bool) {
	if g.pool == nil {
		return
	}
	status := "ok"
	if failed {
		status = "failed"
	}
	if _, err := g.pool.Exec(ctx, `
		INSERT INTO usage_records (provider, model, kind, input_units, output_units, status)
		VALUES ('openai_compatible', $1, $2, $3, $4, $5)
	`, model, kind, inputUnits, outputUnits, status); err != nil {
		g.log.Warn("write usage record failed", "kind", kind, "error", err)
	}
}
