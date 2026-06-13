package formatter

import (
	"strings"
	"testing"

	"nl2sql-executor-go-prod/internal/dbresult"
)

func TestNaturalLanguageSummaryPaymentKPI(t *testing.T) {
	res := &dbresult.Result{
		Columns: []string{"target_name", "total_success_recharge_base_amount", "total_success_withdraw_base_amount"},
		Rows: [][]string{
			{"昨日美国VPBET", "1000000", "500000"},
			{"新加坡", "5000000", "1000000"},
		},
		RowCount: 2,
	}
	got := NaturalLanguageSummary("搜索美国VPBET的昨日总成功充值金额和总提现金额", res)
	if !strings.Contains(got, "昨日美国VPBET：总成功充值金额：100万；总提现金额：50万") {
		t.Fatalf("unexpected summary: %s", got)
	}
	if !strings.Contains(got, "新加坡：总成功充值金额：500万；总提现金额：100万") {
		t.Fatalf("unexpected summary: %s", got)
	}
}
