package sqlguard

import (
	"testing"

	"nl2sql-executor-go-prod/internal/config"
	"nl2sql-executor-go-prod/internal/schema"
)

func TestValidateSelectAndAppendLimit(t *testing.T) {
	g := config.GuardConfig{RequireSchemaQualifiedTables: true, AppendLimitIfMissing: true, RequireLimit: true, AllowCrossSchemaJoin: true}
	checked, err := ValidateAndRewrite("SELECT day, SUM(x) FROM `global_dm`.`t1` WHERE day >= '2026-01-01' GROUP BY day", g, schema.NewEmptyCatalog(), 1000, 100)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if checked.SQL == "" || len(checked.Tables) != 1 {
		t.Fatalf("bad checked: %+v", checked)
	}
}

func TestRejectDelete(t *testing.T) {
	g := config.GuardConfig{RequireSchemaQualifiedTables: true}
	_, err := ValidateAndRewrite("DELETE FROM `global_dm`.`t1`", g, schema.NewEmptyCatalog(), 1000, 100)
	if err == nil {
		t.Fatal("expected reject")
	}
}

func TestLargeTableRequiresTime(t *testing.T) {
	g := config.GuardConfig{RequireSchemaQualifiedTables: true, AppendLimitIfMissing: true, LargeTables: []string{"global_dw.dw_coins_trans"}, TimeColumnHints: []string{"day"}}
	_, err := ValidateAndRewrite("SELECT * FROM `global_dw`.`dw_coins_trans` WHERE user_id = 1", g, schema.NewEmptyCatalog(), 1000, 100)
	if err == nil {
		t.Fatal("expected reject")
	}
}
